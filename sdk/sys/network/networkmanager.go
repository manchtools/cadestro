package network

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/manchtools/cadestro/sdk/sys/exec"
)

type networkManager struct {
	r exec.Runner
}

func (m *networkManager) nmcliRead(ctx context.Context, args ...string) (string, error) {
	res, err := m.r.Run(ctx, exec.Command{Name: "nmcli", Args: args})
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return "", &exec.CommandError{Name: "nmcli", ExitCode: res.ExitCode, Stderr: res.Stderr}
	}
	return res.Stdout, nil
}

func (m *networkManager) nmcliWrite(ctx context.Context, args ...string) error {
	res, err := m.r.Run(ctx, exec.Command{Name: "nmcli", Args: args, Escalate: true})
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return &exec.CommandError{Name: "nmcli", ExitCode: res.ExitCode, Stderr: res.Stderr}
	}
	return nil
}

// ConnectionExists lists all connection names and searches for an exact match so
// that real failures (NetworkManager not running, nmcli missing, ctx cancelled)
// propagate as errors instead of collapsing into "not found".
func (m *networkManager) ConnectionExists(ctx context.Context, name string) (bool, error) {
	if err := validateConnName(name); err != nil {
		return false, err
	}
	out, err := m.nmcliRead(ctx, "-t", "-f", "NAME", "con", "show")
	if err != nil {
		return false, fmt.Errorf("list connections: %w", err)
	}
	for _, line := range strings.Split(out, "\n") {

		if unescapeNmcli(strings.TrimRight(line, "\r")) == name {
			return true, nil
		}
	}
	return false, nil
}

// Settings retrieves a connection's current settings as a key-value map. Values
// are unescaped from nmcli terse-mode encoding (\: -> :, \\ -> \).
func (m *networkManager) Settings(ctx context.Context, name string) (map[string]string, error) {
	if err := validateConnName(name); err != nil {
		return nil, err
	}
	out, err := m.nmcliRead(ctx, "-t", "-f", "all", "con", "show", name)
	if err != nil {
		return nil, fmt.Errorf("get settings for %s: %w", name, err)
	}
	settings := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {

			settings[parts[0]] = unescapeNmcli(parts[1])
		}
	}
	return settings, nil
}

// Apply creates or updates a WiFi connection profile, returning whether a change
// was made.
func (m *networkManager) Apply(ctx context.Context, p Profile) (bool, error) {
	if err := validateProfile(p); err != nil {
		return false, err
	}
	exists, err := m.ConnectionExists(ctx, p.Name)
	if err != nil {
		return false, err
	}
	if exists {
		return m.update(ctx, p)
	}
	return m.create(ctx, p)
}

func (m *networkManager) create(ctx context.Context, p Profile) (bool, error) {
	if p.AuthType == AuthPSK {
		if err := m.provisionPSK(ctx, p); err != nil {
			return false, fmt.Errorf("create PSK connection: %w", err)
		}
		return true, nil
	}

	if err := writeCerts(p); err != nil {
		return false, fmt.Errorf("write certificates: %w", err)
	}
	if err := m.nmcliWrite(ctx, buildAddArgs(p)...); err != nil {
		if rmErr := removeCerts(p.CertDir); rmErr != nil {
			return false, fmt.Errorf("create connection: %w (cert cleanup failed, private key may remain on disk: %v)", err, rmErr)
		}
		return false, fmt.Errorf("create connection: %w", err)
	}
	return true, nil
}

func (m *networkManager) update(ctx context.Context, p Profile) (bool, error) {
	if p.AuthType == AuthPSK {
		if err := m.provisionPSK(ctx, p); err != nil {
			return false, fmt.Errorf("update PSK connection: %w", err)
		}
		return true, nil
	}

	current, err := m.Settings(ctx, p.Name)
	if err != nil {

		return m.stagedModify(ctx, p, nil)
	}
	if !needsModify(current, p) {
		return false, nil
	}
	return m.stagedModify(ctx, p, current)
}

func (m *networkManager) provisionPSK(ctx context.Context, p Profile) error {
	if err := writeKeyfile(keyfilePath(p.Name), buildPSKKeyfile(p)); err != nil {
		return err
	}
	if err := m.nmcliWrite(ctx, "connection", "reload"); err != nil {
		return fmt.Errorf("nmcli connection reload: %w", err)
	}
	return nil
}

func (m *networkManager) stagedModify(ctx context.Context, p Profile, current map[string]string) (bool, error) {
	tmpDir := p.CertDir + ".tmp"
	staged := p
	staged.CertDir = tmpDir
	defer func() { _ = removeAll(tmpDir) }()

	if err := writeCerts(staged); err != nil {
		return false, fmt.Errorf("write staged certificates: %w", err)
	}

	if err := m.nmcliWrite(ctx, buildModifyArgs(p, current)...); err != nil {
		return false, fmt.Errorf("modify connection: %w", err)
	}

	oldDir := p.CertDir + ".old"
	liveExists := false
	if _, err := statFile(p.CertDir); err == nil {
		if err := renameFile(p.CertDir, oldDir); err != nil {
			return true, fmt.Errorf("backup old cert directory: %w", err)
		}
		liveExists = true
	}
	if err := renameFile(tmpDir, p.CertDir); err != nil {
		if liveExists {
			if rerr := renameFile(oldDir, p.CertDir); rerr != nil {
				return true, fmt.Errorf("install staged certs: %w (rollback also failed: %v)", err, rerr)
			}
		}
		return true, fmt.Errorf("install staged certs: %w", err)
	}
	if liveExists {
		if err := removeAll(oldDir); err != nil {

			return true, fmt.Errorf("certs updated but failed to remove old cert directory %s (stale private key may remain): %w", oldDir, err)
		}
	}
	return true, nil
}

// Delete removes a WiFi connection by name and, if opts.CertDir is set, cleans up
// its cert directory.
func (m *networkManager) Delete(ctx context.Context, name string, opts DeleteOptions) error {
	if err := validateConnName(name); err != nil {
		return err
	}
	exists, err := m.ConnectionExists(ctx, name)
	if err != nil {
		return err
	}
	if exists {
		if err := m.nmcliWrite(ctx, "con", "delete", name); err != nil {
			return fmt.Errorf("delete connection %s: %w", name, err)
		}
	}
	if opts.CertDir != "" {
		if err := safeRemoveCertDir(opts.CertDir); err != nil {
			return err
		}
	}
	return nil
}

func needsModify(current map[string]string, p Profile) bool {
	desired := buildDesiredSettings(p)
	for key, want := range desired {
		if current[key] != want {
			return true
		}
	}
	for _, key := range allManagedKeys() {
		if _, inCurrent := current[key]; !inCurrent {
			continue
		}
		if _, inDesired := desired[key]; !inDesired {
			return true
		}
	}
	return certsChanged(p)
}

func buildAddArgs(p Profile) []string {
	args := []string{
		"con", "add",
		"con-name", p.Name,
		"type", "wifi",
		"ssid", p.SSID,
	}
	args = appendEAPAuthArgs(args, p)
	return appendCommonArgs(args, p)
}

func buildModifyArgs(p Profile, current map[string]string) []string {
	args := []string{
		"con", "mod", p.Name,
		"wifi.ssid", p.SSID,
	}
	args = appendEAPAuthArgs(args, p)
	args = appendCommonArgs(args, p)

	if current != nil {
		desired := buildDesiredSettings(p)
		for _, key := range allManagedKeys() {
			if _, inCurrent := current[key]; !inCurrent {
				continue
			}
			if _, inDesired := desired[key]; !inDesired {
				args = append(args, key, "")
			}
		}
	}
	return args
}

func appendEAPAuthArgs(args []string, p Profile) []string {
	args = append(args,
		"wifi-sec.key-mgmt", "wpa-eap",
		"802-1x.eap", "tls",
		"802-1x.identity", p.Identity,
	)
	if p.CACert != "" {
		args = append(args, "802-1x.ca-cert", filepath.Join(p.CertDir, "ca.pem"))
	}
	if p.ClientCert != "" {
		args = append(args, "802-1x.client-cert", filepath.Join(p.CertDir, "client.pem"))
	}
	if !p.ClientKey.IsZero() {
		args = append(args, "802-1x.private-key", filepath.Join(p.CertDir, "client-key.pem"))
	}
	return args
}

func appendCommonArgs(args []string, p Profile) []string {
	if p.AutoConnect {
		args = append(args, "connection.autoconnect", "yes")
	} else {
		args = append(args, "connection.autoconnect", "no")
	}
	args = append(args, "connection.autoconnect-priority", fmt.Sprintf("%d", p.Priority))
	if p.Hidden {
		args = append(args, "wifi.hidden", "yes")
	} else {
		args = append(args, "wifi.hidden", "no")
	}
	return args
}

func allManagedKeys() []string {
	return []string{
		"wifi.ssid",
		"connection.autoconnect",
		"connection.autoconnect-priority",
		"wifi.hidden",
		"wifi-sec.key-mgmt",
		"wifi-sec.psk",
		"802-1x.eap",
		"802-1x.identity",
		"802-1x.ca-cert",
		"802-1x.client-cert",
		"802-1x.private-key",
	}
}

func buildDesiredSettings(p Profile) map[string]string {
	desired := map[string]string{
		"wifi.ssid":         p.SSID,
		"wifi-sec.key-mgmt": "wpa-eap",
		"802-1x.eap":        "tls",
		"802-1x.identity":   p.Identity,
	}
	if p.CACert != "" {
		desired["802-1x.ca-cert"] = filepath.Join(p.CertDir, "ca.pem")
	}
	if p.ClientCert != "" {
		desired["802-1x.client-cert"] = filepath.Join(p.CertDir, "client.pem")
	}
	if !p.ClientKey.IsZero() {
		desired["802-1x.private-key"] = filepath.Join(p.CertDir, "client-key.pem")
	}
	if p.AutoConnect {
		desired["connection.autoconnect"] = "yes"
	} else {
		desired["connection.autoconnect"] = "no"
	}
	desired["connection.autoconnect-priority"] = fmt.Sprintf("%d", p.Priority)
	if p.Hidden {
		desired["wifi.hidden"] = "yes"
	} else {
		desired["wifi.hidden"] = "no"
	}
	return desired
}

func unescapeNmcli(s string) string {
	s = strings.ReplaceAll(s, `\\`, "\x00")
	s = strings.ReplaceAll(s, `\:`, ":")
	s = strings.ReplaceAll(s, "\x00", `\`)
	return s
}

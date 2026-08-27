package dns

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/manchtools/cadestro/sdk/sys/exec"
)

var etcResolvConfPath = "/etc/resolv.conf"

type nmManager struct {
	r exec.Runner
}

func errInterfaceRequired() error {
	return fmt.Errorf("%w: the NetworkManager backend is connection-scoped and requires Config.Interface (use the Resolved backend for host-global DNS)", ErrInvalidConfig)
}

// Get reads the host's effective resolver configuration from /etc/resolv.conf
// (which NetworkManager writes/manages). This is the effective view across all
// connections — symmetric with the Resolved backend's Get — rather than a
// per-connection read (the Manager interface's Get carries no interface arg).
func (m *nmManager) Get(ctx context.Context) (State, error) {
	data, err := os.ReadFile(etcResolvConfPath)
	if err != nil {
		return State{}, fmt.Errorf("read %s: %w", etcResolvConfPath, err)
	}
	return parseResolvConf(data), nil
}

// Apply configures DNS on the active connection of cfg.Interface and reactivates
// it so the change takes effect.
func (m *nmManager) Apply(ctx context.Context, cfg Config) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}
	if cfg.Interface == "" {
		return errInterfaceRequired()
	}

	conn, err := m.activeConnection(ctx, cfg.Interface)
	if err != nil {
		return err
	}

	args := []string{"connection", "modify", conn}
	v4, v6 := partitionByFamily(cfg.Nameservers)
	if len(v4) > 0 {
		args = append(args, "ipv4.dns", strings.Join(v4, ","))
	}
	if len(v6) > 0 {
		args = append(args, "ipv6.dns", strings.Join(v6, ","))
	}
	if len(cfg.SearchDomains) > 0 {
		domains := strings.Join(cfg.SearchDomains, ",")
		args = append(args, "ipv4.dns-search", domains, "ipv6.dns-search", domains)
	}

	if err := runPriv(ctx, m.r, "nmcli", args...); err != nil {
		return fmt.Errorf("nmcli connection modify %s: %w", conn, err)
	}
	if err := runPriv(ctx, m.r, "nmcli", "connection", "up", conn); err != nil {

		_ = runPriv(ctx, m.r, "nmcli", "connection", "modify", conn,
			"ipv4.dns", "", "ipv6.dns", "", "ipv4.dns-search", "", "ipv6.dns-search", "")
		return fmt.Errorf("nmcli connection up %s: %w", conn, err)
	}
	return nil
}

func (m *nmManager) activeConnection(ctx context.Context, iface string) (string, error) {
	out, err := runRead(ctx, m.r, "nmcli", "-g", "GENERAL.CONNECTION", "device", "show", iface)
	if err != nil {
		return "", fmt.Errorf("resolve connection for %s: %w", iface, err)
	}
	conn := strings.TrimSpace(out)
	if conn == "" || conn == "--" {
		return "", fmt.Errorf("%w: interface %q has no active NetworkManager connection to configure", ErrInvalidConfig, iface)
	}
	if err := validateConnName(conn); err != nil {
		return "", err
	}
	return conn, nil
}

func validateConnName(name string) error {
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("%w: connection name %q from host output contains a control character", ErrInvalidConfig, name)
		}
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("%w: connection name %q from host output is flag-shaped (leading '-')", ErrInvalidConfig, name)
	}
	return nil
}

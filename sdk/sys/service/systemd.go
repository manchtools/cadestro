package service

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/fs"
)

var validSystemdUnitName = regexp.MustCompile(`^(?:[a-zA-Z0-9@_:-]|\\x[0-9A-Fa-f]{2})(?:[a-zA-Z0-9@._:-]|\\x[0-9A-Fa-f]{2})*\.(service|socket|device|timer|mount|automount|swap|target|path|slice|scope)$`)

// ValidateUnitName reports whether unit is a safe, well-formed systemd unit name.
func ValidateUnitName(unit string) error {
	if !validSystemdUnitName.MatchString(unit) {
		return fmt.Errorf("invalid systemd unit name %q: must not start with '.' and must match <name>.<type> where type is one of service, socket, device, timer, mount, automount, swap, target, path, slice, scope", unit)
	}
	return nil
}

var validSystemctlOutputs = map[string]map[string]struct{}{
	"is-enabled": {
		"enabled": {}, "enabled-runtime": {}, "linked": {}, "linked-runtime": {},
		"alias": {}, "masked": {}, "masked-runtime": {}, "static": {},
		"indirect": {}, "disabled": {}, "generated": {}, "transient": {},
	},
	"is-active": {
		"active": {}, "reloading": {}, "inactive": {},
		"failed": {}, "activating": {}, "deactivating": {},
	},
}

func (s *Manager) query(ctx context.Context, unit, verb string) (string, error) {
	ctx, cancel := ensureCtx(ctx)
	defer cancel()
	res, err := s.r.Run(ctx, exec.Command{Name: "systemctl", Args: []string{verb, "--", unit}})
	if err != nil {
		return "", fmt.Errorf("systemctl %s %s: %w", verb, unit, err)
	}
	trimmed := strings.TrimSpace(res.Stdout)
	allowed, known := validSystemctlOutputs[verb]
	if !known {
		return "", fmt.Errorf("systemctl %s: unsupported query verb", verb)
	}
	if _, ok := allowed[trimmed]; !ok {
		return "", fmt.Errorf("systemctl %s %s: unrecognised output %q (exit %d)", verb, unit, trimmed, res.ExitCode)
	}
	return trimmed, nil
}

func (s *Manager) mutate(ctx context.Context, args ...string) error {
	res, err := s.r.Run(ctx, exec.Command{Name: "systemctl", Args: args, Escalate: true})
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return &exec.CommandError{Name: "systemctl", ExitCode: res.ExitCode, Stderr: res.Stderr}
	}
	return nil
}

// Status reports the current unit state.
func (s *Manager) Status(ctx context.Context, unit string) (UnitStatus, error) {
	if err := ValidateUnitName(unit); err != nil {
		return UnitStatus{}, err
	}
	var status UnitStatus
	enabled, err := s.query(ctx, unit, "is-enabled")
	if err != nil {
		return UnitStatus{}, err
	}
	switch enabled {
	case "enabled", "enabled-runtime":
		status.Enabled = true
	case "static", "indirect", "generated":
		status.Static = true
	case "masked", "masked-runtime":
		status.Masked = true
	}
	active, err := s.query(ctx, unit, "is-active")
	if err != nil {
		return UnitStatus{}, err
	}
	status.Active = active == "active"
	return status, nil
}

// IsEnabled reports whether a unit is enabled.
func (s *Manager) IsEnabled(ctx context.Context, unit string) (bool, error) {
	if err := ValidateUnitName(unit); err != nil {
		return false, err
	}

	out, err := s.query(ctx, unit, "is-enabled")
	if err != nil {
		return false, err
	}
	return out == "enabled" || out == "enabled-runtime", nil
}

// IsMasked reports whether a unit is masked.
func (s *Manager) IsMasked(ctx context.Context, unit string) (bool, error) {
	if err := ValidateUnitName(unit); err != nil {
		return false, err
	}
	out, err := s.query(ctx, unit, "is-enabled")
	if err != nil {
		return false, err
	}
	return out == "masked" || out == "masked-runtime", nil
}

// IsActive reports whether a unit is active.
func (s *Manager) IsActive(ctx context.Context, unit string) (bool, error) {
	if err := ValidateUnitName(unit); err != nil {
		return false, err
	}
	out, err := s.query(ctx, unit, "is-active")
	if err != nil {
		return false, err
	}
	return out == "active", nil
}

// Enable enables a unit.
func (s *Manager) Enable(ctx context.Context, unit string) error {
	if err := ValidateUnitName(unit); err != nil {
		return err
	}
	return s.mutate(ctx, "enable", "--", unit)
}

// Disable disables a unit.
func (s *Manager) Disable(ctx context.Context, unit string) error {
	if err := ValidateUnitName(unit); err != nil {
		return err
	}
	return s.mutate(ctx, "disable", "--", unit)
}

// EnableNow enables and starts a unit.
func (s *Manager) EnableNow(ctx context.Context, unit string) error {
	if err := ValidateUnitName(unit); err != nil {
		return err
	}
	return s.mutate(ctx, "enable", "--now", "--", unit)
}

// DisableNow disables and stops a unit.
func (s *Manager) DisableNow(ctx context.Context, unit string) error {
	if err := ValidateUnitName(unit); err != nil {
		return err
	}
	return s.mutate(ctx, "disable", "--now", "--", unit)
}

// Start starts a unit.
func (s *Manager) Start(ctx context.Context, unit string) error {
	if err := ValidateUnitName(unit); err != nil {
		return err
	}
	return s.mutate(ctx, "start", "--", unit)
}

// Stop stops a unit.
func (s *Manager) Stop(ctx context.Context, unit string) error {
	if err := ValidateUnitName(unit); err != nil {
		return err
	}
	return s.mutate(ctx, "stop", "--", unit)
}

// Restart restarts a unit.
func (s *Manager) Restart(ctx context.Context, unit string) error {
	if err := ValidateUnitName(unit); err != nil {
		return err
	}
	return s.mutate(ctx, "restart", "--", unit)
}

// Reload asks a running unit to re-read its configuration.
func (s *Manager) Reload(ctx context.Context, unit string) error {
	if err := ValidateUnitName(unit); err != nil {
		return err
	}
	return s.mutate(ctx, "reload", "--", unit)
}

// Mask masks a unit.
func (s *Manager) Mask(ctx context.Context, unit string) error {
	if err := ValidateUnitName(unit); err != nil {
		return err
	}
	return s.mutate(ctx, "mask", "--", unit)
}

// Unmask removes a unit mask.
func (s *Manager) Unmask(ctx context.Context, unit string) error {
	if err := ValidateUnitName(unit); err != nil {
		return err
	}
	return s.mutate(ctx, "unmask", "--", unit)
}

// DaemonReload asks systemd to reload unit files.
func (s *Manager) DaemonReload(ctx context.Context) error {
	return s.mutate(ctx, "daemon-reload")
}

// WriteUnit writes a unit file.
func (s *Manager) WriteUnit(ctx context.Context, unit, content string) error {
	if err := ValidateUnitName(unit); err != nil {
		return err
	}
	if err := validateUnitContent(content); err != nil {
		return err
	}
	return s.fsm.WriteFile(ctx, "/etc/systemd/system/"+unit, []byte(content), fs.WriteOptions{Mode: 0o644, Owner: "root", Group: "root"})
}

// ReadUnit returns the on-disk content of a unit.
func (s *Manager) ReadUnit(ctx context.Context, unit string) (string, error) {
	if err := ValidateUnitName(unit); err != nil {
		return "", err
	}

	content, err := s.fsm.ReadFile(ctx, "/etc/systemd/system/"+unit)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// RemoveUnit removes a unit file.
func (s *Manager) RemoveUnit(ctx context.Context, unit string) error {
	if err := ValidateUnitName(unit); err != nil {
		return err
	}
	path := "/etc/systemd/system/" + unit
	if err := s.fsm.Remove(ctx, path); err != nil {
		return fmt.Errorf("remove systemd unit %s: %w", path, err)
	}
	return nil
}

// NeedsReload reports whether systemd's loaded configuration for unit is
// stale relative to the on-disk unit file, via the NeedDaemonReload unit
// property. `systemctl show` is lenient (exit 0 even for unknown units), so
// correctness rides on the strict yes/no output parse: anything else is an
// error, never a guessed false.
func (s *Manager) NeedsReload(ctx context.Context, unit string) (bool, error) {
	if err := ValidateUnitName(unit); err != nil {
		return false, err
	}
	ctx, cancel := ensureCtx(ctx)
	defer cancel()
	res, err := s.r.Run(ctx, exec.Command{Name: "systemctl", Args: []string{"show", "--property=NeedDaemonReload", "--", unit}})
	if err != nil {
		return false, fmt.Errorf("systemctl show %s: %w", unit, err)
	}
	out := strings.TrimSpace(res.Stdout)
	switch out {
	case "NeedDaemonReload=yes":
		return true, nil
	case "NeedDaemonReload=no":
		return false, nil
	}
	return false, fmt.Errorf("systemctl show %s: unexpected NeedDaemonReload output %q", unit, out)
}

// Version reports systemd's major version: the first integer token on the
// first line of `systemctl --version` ("systemd 257 (257.7-1)" → 257),
// mirroring the parse the agent install script used. Anything else — empty
// output, no numeric token, an exec failure — is an error; the caller owns
// its fail-safe.
func (s *Manager) Version(ctx context.Context) (int, error) {
	ctx, cancel := ensureCtx(ctx)
	defer cancel()
	res, err := s.r.Run(ctx, exec.Command{Name: "systemctl", Args: []string{"--version"}})
	if err != nil {
		return 0, fmt.Errorf("systemctl --version: %w", err)
	}
	firstLine, _, _ := strings.Cut(res.Stdout, "\n")
	for _, tok := range strings.Fields(firstLine) {
		if v, convErr := strconv.Atoi(tok); convErr == nil {
			return v, nil
		}
	}
	return 0, fmt.Errorf("systemctl --version: no version token in %q", firstLine)
}

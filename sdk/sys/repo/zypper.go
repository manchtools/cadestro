package repo

import (
	"context"
	"fmt"
	"strings"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

func (m *manager) applyZypper(ctx context.Context, name string, c *ZypperConfig) (Outcome, error) {
	var log strings.Builder

	args := []string{"--non-interactive", "addrepo"}
	if c.Autorefresh {
		args = append(args, "--refresh")
	}
	if !c.GPGCheck {
		args = append(args, "--no-gpgcheck")
	}
	if c.Type != "" {
		args = append(args, "--type", c.Type)
	}

	if _, err := m.runPriv(ctx, "zypper", "--non-interactive", "removerepo", name); err != nil {
		fmt.Fprintf(&log, "note: pre-add removerepo failed (expected if the repo is absent): %v\n", err)
	}

	args = append(args, c.URL, name)
	res, err := m.runPriv(ctx, "zypper", args...)
	if res.Stdout != "" {
		log.WriteString(res.Stdout)
	}
	if err != nil {
		if res.Stderr != "" {
			log.WriteString(res.Stderr)
		}
		return Outcome{
			Result:  sysexec.Result{ExitCode: 1, Stdout: log.String(), Stderr: res.Stderr},
			Changed: false,
		}, fmt.Errorf("add repository: %w", err)
	}
	fmt.Fprintf(&log, "configured repository: %s\n", name)

	if c.Description != "" {
		if _, err := m.runPriv(ctx, "zypper", "--non-interactive", "modifyrepo", "--name="+c.Description, name); err != nil {
			return Outcome{}, fmt.Errorf("set repo description: %w", err)
		}
	}
	if c.Enabled {
		if _, err := m.runPriv(ctx, "zypper", "--non-interactive", "modifyrepo", "--enable", name); err != nil {
			return Outcome{}, fmt.Errorf("enable repo: %w", err)
		}
	} else {
		if _, err := m.runPriv(ctx, "zypper", "--non-interactive", "modifyrepo", "--disable", name); err != nil {
			return Outcome{}, fmt.Errorf("disable repo: %w", err)
		}
	}

	if c.GPGCheck && c.GPGKey != "" {
		m.runNonFatal(ctx, &log, "warning: failed to import GPG key", "rpm", sysexec.SeparatePositionals([]string{"--import"}, c.GPGKey)...)
	}
	m.runNonFatal(ctx, &log, "warning: failed to refresh repo", "zypper", "--non-interactive", "refresh", name)

	return out(log.String(), true), nil
}

func (m *manager) removeZypper(ctx context.Context, name string) (Outcome, error) {
	var log strings.Builder
	res, err := m.runPriv(ctx, "zypper", "--non-interactive", "removerepo", name)
	if err != nil {
		return Outcome{}, fmt.Errorf("remove repository: %w", err)
	}
	if strings.Contains(res.Stdout+res.Stderr, "not found") {
		fmt.Fprintf(&log, "repository %s not found, nothing to remove\n", name)
		return out(log.String(), false), nil
	}
	fmt.Fprintf(&log, "removed repository: %s\n", name)
	return out(log.String(), true), nil
}

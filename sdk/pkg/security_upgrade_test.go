package pkg

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

func stubUnattendedUpgradePaths(t *testing.T, paths ...string) {
	t.Helper()
	orig := unattendedUpgradeBinPaths
	unattendedUpgradeBinPaths = paths
	t.Cleanup(func() { unattendedUpgradeBinPaths = orig })
}

func TestUpgradeAll_SecurityOnly(t *testing.T) {
	ctx := context.Background()

	t.Run("dnf upgrade --security", func(t *testing.T) {
		m, f := dnfM(t)
		ok(f, "")
		if _, err := m.UpgradeSecurity(ctx); err != nil {
			t.Fatal(err)
		}
		if got := argv(f.Calls()[0]); got != "dnf upgrade -y --security" {
			t.Errorf("argv = %q, want `dnf upgrade -y --security`", got)
		}
	})

	t.Run("zypper patch --category security", func(t *testing.T) {
		m, f := zypperM(t)
		ok(f, "")
		if _, err := m.UpgradeSecurity(ctx); err != nil {
			t.Fatal(err)
		}
		if got := argv(f.Calls()[0]); got != "zypper --non-interactive patch --category security" {
			t.Errorf("argv = %q, want `zypper --non-interactive patch --category security`", got)
		}
	})

	t.Run("apt via unattended-upgrade (escalated, absolute path)", func(t *testing.T) {
		stubUnattendedUpgradePaths(t)
		stubLookPath(t, "apt", "apt-get", "unattended-upgrade")
		m, f := mustNew(t, Apt)
		ok(f, "")
		if _, err := m.UpgradeSecurity(ctx); err != nil {
			t.Fatal(err)
		}
		c := f.Calls()[0]

		if argv(c) != "/usr/bin/unattended-upgrade -v" || !c.Escalate {
			t.Errorf("argv = %q (escalate=%v), want escalated `/usr/bin/unattended-upgrade -v`", argv(c), c.Escalate)
		}
	})

	t.Run("apt without unattended-upgrade fails closed (ErrBackendUnavailable, no command run)", func(t *testing.T) {
		stubUnattendedUpgradePaths(t)
		m, f := aptM(t)
		_, err := m.UpgradeSecurity(ctx)
		if !errors.Is(err, sysexec.ErrBackendUnavailable) {
			t.Fatalf("err = %v, want ErrBackendUnavailable", err)
		}
		if len(f.Calls()) != 0 {
			t.Errorf("nothing must run when unattended-upgrade is absent; got %d calls", len(f.Calls()))
		}
	})

	t.Run("pacman security-only unsupported", func(t *testing.T) {
		m, _ := pacmanM(t)
		if _, err := m.UpgradeSecurity(ctx); !errors.Is(err, ErrUnsupported) {
			t.Errorf("err = %v, want ErrUnsupported", err)
		}
	})

}

func TestApt_HasSecurityUpdates(t *testing.T) {
	ctx := context.Background()
	t.Run("packages", func(t *testing.T) {
		stubUnattendedUpgradePaths(t)
		stubLookPath(t, "unattended-upgrade")
		m, f := mustNew(t, Apt)
		ok(f, "Packages that will be upgraded: vim\n")
		got, err := m.HasSecurityUpdates(ctx)
		if err != nil || !got || argv(f.Calls()[0]) != "/usr/bin/unattended-upgrade --dry-run --verbose" {
			t.Fatalf("got=%v err=%v argv=%q", got, err, argv(f.Calls()[0]))
		}
	})
	t.Run("no packages", func(t *testing.T) {
		stubUnattendedUpgradePaths(t)
		stubLookPath(t, "unattended-upgrade")
		m, f := mustNew(t, Apt)
		ok(f, "No packages will be upgraded.\n")
		got, err := m.HasSecurityUpdates(ctx)
		if err != nil || got {
			t.Fatalf("got=%v err=%v", got, err)
		}
	})
	t.Run("nonzero", func(t *testing.T) {
		stubUnattendedUpgradePaths(t)
		stubLookPath(t, "unattended-upgrade")
		m, f := mustNew(t, Apt)
		f.Push(sysexec.Result{ExitCode: 2}, nil)
		if _, err := m.HasSecurityUpdates(ctx); err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("runner error", func(t *testing.T) {
		stubUnattendedUpgradePaths(t)
		stubLookPath(t, "unattended-upgrade")
		m, f := mustNew(t, Apt)
		runnerErr := errors.New("boom")
		f.Push(sysexec.Result{}, runnerErr)
		if _, err := m.HasSecurityUpdates(ctx); !errors.Is(err, runnerErr) {
			t.Fatalf("err=%v, want runner error", err)
		}
	})
}

func TestResolveUnattendedUpgrade_PrefersAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "unattended-upgrade")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	stubUnattendedUpgradePaths(t, filepath.Join(dir, "missing"), bin)
	stubLookPath(t)

	got, err := resolveUnattendedUpgrade()
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != bin {
		t.Fatalf("got %q, want the known absolute path %q (must not depend on $PATH)", got, bin)
	}
}

func TestResolveUnattendedUpgrade_FallsBackToPath(t *testing.T) {
	stubUnattendedUpgradePaths(t, filepath.Join(t.TempDir(), "missing"))

	stubLookPath(t, "unattended-upgrade")
	if got, err := resolveUnattendedUpgrade(); err != nil || got != "/usr/bin/unattended-upgrade" {
		t.Fatalf("got (%q, %v), want (/usr/bin/unattended-upgrade, nil)", got, err)
	}

	stubLookPath(t)
	if _, err := resolveUnattendedUpgrade(); !errors.Is(err, sysexec.ErrBackendUnavailable) {
		t.Fatalf("err = %v, want ErrBackendUnavailable", err)
	}
}

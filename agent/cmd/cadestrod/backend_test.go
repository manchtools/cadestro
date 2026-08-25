package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

func TestApplyBackendOverrides_PrivilegeBackend(t *testing.T) {

	origEuid := geteuidFn
	t.Cleanup(func() { geteuidFn = origEuid })
	geteuidFn = func() int { return 1000 }

	fakeBin := fakePathWith(t, "sudo", "doas", "systemctl", "cryptsetup")

	cases := []struct {
		name    string
		cfg     *Config
		wantErr bool
		want    sysexec.PrivilegeBackend
	}{
		{name: "empty defaults to sudo", cfg: &Config{}, want: sysexec.Sudo},
		{name: "explicit sudo", cfg: &Config{PrivilegeBackend: "sudo"}, want: sysexec.Sudo},
		{name: "explicit doas", cfg: &Config{PrivilegeBackend: "doas"}, want: sysexec.Doas},
		{name: "unknown value fails", cfg: &Config{PrivilegeBackend: "typo"}, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("PATH", fakeBin)
			got, err := applyBackendOverrides(tc.cfg, discardLogger())
			if err != nil {
				if !tc.wantErr {
					t.Fatalf("applyBackendOverrides: %v", err)
				}
				return
			}
			if tc.wantErr {
				t.Fatal("expected error, got nil")
			}
			if got != tc.want {
				t.Errorf("privilege backend = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestApplyBackendOverrides_MissingPrivilegeBinaryIsFatal(t *testing.T) {

	fakeBin := fakePathWith(t, "sudo", "systemctl", "cryptsetup")
	t.Setenv("PATH", fakeBin)

	_, err := applyBackendOverrides(&Config{PrivilegeBackend: "doas"}, discardLogger())
	if err == nil {
		t.Fatal("expected error when doas binary is missing, got nil")
	}
	if !strings.Contains(err.Error(), "doas") {
		t.Errorf("error should mention doas, got: %v", err)
	}
}

func TestSetPrivilegeBackend_EmptyDefault_BranchesOnEuid(t *testing.T) {
	origEuid := geteuidFn
	t.Cleanup(func() { geteuidFn = origEuid })

	t.Setenv("PATH", fakePathWith(t, "sudo"))

	t.Run("euid 0 selects the root backend (no escalation tool)", func(t *testing.T) {
		geteuidFn = func() int { return 0 }
		got, err := setPrivilegeBackend("", discardLogger())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != sysexec.Direct {
			t.Errorf("empty default at euid 0 = %v, want Direct (root)", got)
		}
	})

	t.Run("euid 1000 selects the sudo backend", func(t *testing.T) {
		geteuidFn = func() int { return 1000 }
		got, err := setPrivilegeBackend("", discardLogger())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != sysexec.Sudo {
			t.Errorf("empty default at euid 1000 = %v, want Sudo", got)
		}
	})
}

func TestSetPrivilegeBackend_RootBackend_RequiresRootEuid(t *testing.T) {
	origEuid := geteuidFn
	t.Cleanup(func() { geteuidFn = origEuid })

	t.Run("euid 1000 refuses the explicit root backend", func(t *testing.T) {
		geteuidFn = func() int { return 1000 }
		if _, err := setPrivilegeBackend("root", discardLogger()); err == nil {
			t.Fatal("root backend on a non-root process must error, not build a Direct runner")
		}
	})

	t.Run("euid 0 accepts the explicit root backend", func(t *testing.T) {
		geteuidFn = func() int { return 0 }
		got, err := setPrivilegeBackend("root", discardLogger())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != sysexec.Direct {
			t.Errorf("root backend at euid 0 = %v, want Direct", got)
		}
	})
}

func TestApplyBackendOverrides_RequiresSystemd(t *testing.T) {

	origEuid := geteuidFn
	t.Cleanup(func() { geteuidFn = origEuid })
	geteuidFn = func() int { return 1000 }

	t.Run("missing systemctl is fatal", func(t *testing.T) {
		t.Setenv("PATH", fakePathWith(t, "sudo", "cryptsetup"))
		_, err := applyBackendOverrides(&Config{}, discardLogger())
		if err == nil {
			t.Fatal("expected a fatal error when systemctl is missing")
		}
		if !strings.Contains(err.Error(), "systemctl") {
			t.Errorf("error should mention systemctl, got: %v", err)
		}
	})

	t.Run("systemctl present", func(t *testing.T) {
		t.Setenv("PATH", fakePathWith(t, "sudo", "cryptsetup", "systemctl"))
		if _, err := applyBackendOverrides(&Config{}, discardLogger()); err != nil {
			t.Fatalf("applyBackendOverrides: %v", err)
		}
	})
}

func fakePathWith(t *testing.T, tools ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, tool := range tools {
		path := filepath.Join(dir, tool)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatalf("write fake %s: %v", tool, err)
		}
	}
	return dir
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

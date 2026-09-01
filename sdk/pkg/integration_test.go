package pkg

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/manchtools/cadestro/sdk/sys/exec"
)

func realManager(t *testing.T, b Backend) Manager {
	t.Helper()

	if testing.Short() {
		t.Skip("-short: skipping real package-manager integration read")
	}
	if !slices.Contains(Detect(), b) {
		t.Skipf("%s not available on this host", b)
	}
	r, err := exec.NewRunner(exec.Direct)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	m, err := New(b, r)
	if err != nil {
		t.Fatalf("New(%s): %v", b, err)
	}
	return m
}

func readIntegration(t *testing.T, m Manager, knownPkg string) {
	t.Helper()
	ctx := context.Background()

	if v, err := m.Version(ctx); err != nil {
		t.Errorf("Version: %v", err)
	} else if v == "" {
		t.Error("Version returned empty string")
	}

	pkgs, err := m.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(pkgs) == 0 {
		t.Error("List returned no installed packages")
	}
	for _, p := range pkgs[:min(5, len(pkgs))] {
		if p.Name == "" {
			t.Error("List returned a package with an empty name")
		}
		if p.Status != PackageStatusInstalled {
			t.Errorf("List package %q status = %q, want installed", p.Name, p.Status)
		}
	}

	if n, err := m.InstalledCount(ctx); err != nil {
		t.Errorf("InstalledCount: %v", err)
	} else if n <= 0 {
		t.Errorf("InstalledCount = %d, want > 0", n)
	}

	if installed, err := m.IsInstalled(ctx, knownPkg); err != nil {
		t.Errorf("IsInstalled(%s): %v", knownPkg, err)
	} else if !installed {
		t.Errorf("IsInstalled(%s) = false, want true", knownPkg)
	}
	if installed, err := m.IsInstalled(ctx, "nonexistent-package-xyz-123456"); err != nil {
		t.Errorf("IsInstalled(ghost): %v", err)
	} else if installed {
		t.Error("IsInstalled(ghost) = true, want false")
	}

	if v, err := m.InstalledVersion(ctx, knownPkg); err != nil {
		t.Errorf("InstalledVersion(%s): %v", knownPkg, err)
	} else if v == "" {
		t.Errorf("InstalledVersion(%s) returned empty", knownPkg)
	}

	if p, err := m.Show(ctx, knownPkg); err != nil {
		t.Errorf("Show(%s): %v", knownPkg, err)
	} else if p == nil {
		t.Errorf("Show(%s) returned nil package without error", knownPkg)
	} else if p.Name != knownPkg {
		t.Errorf("Show(%s).Name = %q", knownPkg, p.Name)
	}

	if _, err := m.Search(ctx, knownPkg); err != nil {
		t.Errorf("Search(%s): %v", knownPkg, err)
	}
	if info, err := m.ListVersions(ctx, knownPkg); err != nil {
		t.Errorf("ListVersions(%s): %v", knownPkg, err)
	} else if info == nil {
		t.Errorf("ListVersions(%s) returned nil info without error", knownPkg)
	} else if info.Name != knownPkg {
		t.Errorf("ListVersions(%s).Name = %q", knownPkg, info.Name)
	}
	if ups, err := m.ListUpgradable(ctx); err != nil {
		t.Errorf("ListUpgradable: %v", err)
	} else {
		for _, u := range ups {
			if u.Name == "" {
				t.Error("ListUpgradable returned an update with an empty name")
			}
		}
	}
	if _, err := m.HasUpdates(ctx); err != nil {
		t.Errorf("HasUpdates: %v", err)
	}

	if _, err := m.IsPinned(ctx, knownPkg); err != nil && !errors.Is(err, ErrUnsupported) {
		t.Errorf("IsPinned(%s): %v", knownPkg, err)
	}

}

func TestIntegration_Apt(t *testing.T)    { readIntegration(t, realManager(t, Apt), "bash") }
func TestIntegration_Dnf(t *testing.T)    { readIntegration(t, realManager(t, Dnf), "bash") }
func TestIntegration_Dnf5(t *testing.T)   { readIntegration(t, realManager(t, Dnf5), "bash") }
func TestIntegration_Pacman(t *testing.T) { readIntegration(t, realManager(t, Pacman), "bash") }
func TestIntegration_Zypper(t *testing.T) { readIntegration(t, realManager(t, Zypper), "bash") }

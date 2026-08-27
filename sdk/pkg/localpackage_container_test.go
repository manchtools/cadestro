//go:build container

package pkg

import (
	"context"
	"os"
	osexec "os/exec"
	"path/filepath"
	"testing"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

func TestLocalPackageInfo_AptRealDeb_Container(t *testing.T) {
	if _, err := osexec.LookPath("dpkg-deb"); err != nil {
		t.Skip("dpkg-deb not on PATH")
	}
	dir := t.TempDir()
	pkgRoot := filepath.Join(dir, "cadestro-testpkg")
	if err := os.MkdirAll(filepath.Join(pkgRoot, "DEBIAN"), 0o755); err != nil {
		t.Fatal(err)
	}
	control := "Package: cadestro-testpkg\n" +
		"Version: 1.2.3\n" +
		"Architecture: all\n" +
		"Maintainer: Cadestro Test <test@cadestro.invalid>\n" +
		"Description: Cadestro LocalPackageInfo real-execution fixture\n"
	if err := os.WriteFile(filepath.Join(pkgRoot, "DEBIAN", "control"), []byte(control), 0o644); err != nil {
		t.Fatal(err)
	}
	debPath := filepath.Join(dir, "cadestro-testpkg.deb")
	if out, err := osexec.Command("dpkg-deb", "--build", pkgRoot, debPath).CombinedOutput(); err != nil {
		t.Fatalf("dpkg-deb --build: %v\n%s", err, out)
	}

	r, err := sysexec.NewRunner(sysexec.Direct)
	if err != nil {
		t.Fatalf("NewRunner(Direct): %v", err)
	}
	m, err := New(Apt, r)
	if err != nil {
		t.Fatalf("New(Apt): %v", err)
	}
	info, err := m.LocalPackageInfo(context.Background(), debPath)
	if err != nil {
		t.Fatalf("LocalPackageInfo on a real .deb: %v", err)
	}
	if info.Name != "cadestro-testpkg" {
		t.Errorf("Name = %q, want cadestro-testpkg (the VALUE, not the 'Package:' label)", info.Name)
	}
	if info.Version != "1.2.3" || info.Arch != "all" {
		t.Errorf("info = %+v, want version=1.2.3 arch=all", info)
	}
}

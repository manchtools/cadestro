//go:build container

package pkg

import (
	"context"
	"os"
	osexec "os/exec"
	"path/filepath"
	"testing"
	"time"

	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

const mutTestPackage = "hello"

func aptMutManager(t *testing.T) Manager {
	t.Helper()
	if _, err := osexec.LookPath("apt-get"); err != nil {
		t.Skip("apt-get not on PATH; apt mutation tests not exercisable")
	}
	r, err := sysexec.NewRunner(sysexec.Direct)
	if err != nil {
		t.Fatalf("NewRunner(Direct): %v", err)
	}
	m, err := New(Apt, r)
	if err != nil {
		t.Fatalf("New(Apt): %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := m.Update(ctx); err != nil {
		t.Skipf("apt-get update failed (no network/mirror?): %v", err)
	}
	return m
}

func mutCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)
	return ctx
}

func cleanupPkg(t *testing.T, m Manager, unpin bool) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if unpin {
			_, _ = m.Unpin(ctx, mutTestPackage)
		}
		_, _ = m.Remove(ctx, RemoveOptions{}, mutTestPackage)
	})
}

func TestApt_InstallRemove_Container(t *testing.T) {
	m := aptMutManager(t)
	ctx := mutCtx(t)
	cleanupPkg(t, m, false)

	if _, err := m.Install(ctx, InstallOptions{}, InstallSpec{Name: mutTestPackage}); err != nil {
		t.Fatalf("Install(%s): %v", mutTestPackage, err)
	}
	if ok, err := m.IsInstalled(ctx, mutTestPackage); err != nil || !ok {
		t.Fatalf("after Install, IsInstalled(%s) = (%v, %v), want (true, nil)", mutTestPackage, ok, err)
	}

	if _, err := m.Remove(ctx, RemoveOptions{}, mutTestPackage); err != nil {
		t.Fatalf("Remove(%s): %v", mutTestPackage, err)
	}
	if ok, err := m.IsInstalled(ctx, mutTestPackage); err != nil || ok {
		t.Fatalf("after Remove, IsInstalled(%s) = (%v, %v), want (false, nil)", mutTestPackage, ok, err)
	}
}

func TestApt_PinUnpin_Container(t *testing.T) {
	m := aptMutManager(t)
	ctx := mutCtx(t)
	cleanupPkg(t, m, true)
	if _, err := m.Install(ctx, InstallOptions{}, InstallSpec{Name: mutTestPackage}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	if _, err := m.Pin(ctx, mutTestPackage); err != nil {
		t.Fatalf("Pin: %v", err)
	}
	if ok, err := m.IsPinned(ctx, mutTestPackage); err != nil || !ok {
		t.Fatalf("after Pin, IsPinned = (%v, %v), want (true, nil)", ok, err)
	}
	if _, err := m.Unpin(ctx, mutTestPackage); err != nil {
		t.Fatalf("Unpin: %v", err)
	}
	if ok, err := m.IsPinned(ctx, mutTestPackage); err != nil || ok {
		t.Fatalf("after Unpin, IsPinned = (%v, %v), want (false, nil)", ok, err)
	}
}

func TestApt_InstallLocal_Container(t *testing.T) {
	m := aptMutManager(t)
	ctx := mutCtx(t)
	cleanupPkg(t, m, false)

	dir := t.TempDir()
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatalf("chmod tempdir: %v", err)
	}
	dl := osexec.CommandContext(ctx, "apt-get", "download", mutTestPackage)
	dl.Dir = dir
	if out, err := dl.CombinedOutput(); err != nil {
		t.Skipf("apt-get download %s failed (no network/mirror?): %v\n%s", mutTestPackage, err, out)
	}
	debs, err := filepath.Glob(filepath.Join(dir, mutTestPackage+"_*.deb"))
	if err != nil {
		t.Fatalf("glob for downloaded .deb: %v", err)
	}
	if len(debs) == 0 {
		t.Fatalf("apt-get download produced no .deb in %s", dir)
	}

	if _, err := m.InstallLocal(ctx, debs[0], InstallLocalOptions{}); err != nil {
		t.Fatalf("InstallLocal(%s): %v", debs[0], err)
	}
	if ok, err := m.IsInstalled(ctx, mutTestPackage); err != nil || !ok {
		t.Fatalf("after InstallLocal, IsInstalled(%s) = (%v, %v), want (true, nil)", mutTestPackage, ok, err)
	}
}

func TestApt_UpgradeAll_Container(t *testing.T) {
	m := aptMutManager(t)
	ctx := mutCtx(t)
	if _, err := m.UpgradeAll(ctx); err != nil {
		t.Fatalf("UpgradeAll: %v", err)
	}
}

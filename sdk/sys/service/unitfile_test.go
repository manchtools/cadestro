package service

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/exec/exectest"
	"github.com/manchtools/cadestro/sdk/sys/fs"
)

type fakeFS struct {
	wrotePath, wroteContent string
	wroteOpts               fs.WriteOptions
	removedPath             string
	readPath, readContent   string
	writeErr, removeErr     error
	readErr                 error
}

func (f *fakeFS) WriteFile(_ context.Context, path string, data []byte, opts fs.WriteOptions) error {
	f.wrotePath, f.wroteContent, f.wroteOpts = path, string(data), opts
	return f.writeErr
}

func (f *fakeFS) ReadFile(_ context.Context, path string) ([]byte, error) {
	f.readPath = path
	if f.readErr != nil {
		return nil, f.readErr
	}
	return []byte(f.readContent), nil
}

func (f *fakeFS) Remove(_ context.Context, path string) error {
	f.removedPath = path
	return f.removeErr
}

func (f *fakeFS) install(t *testing.T) {
	t.Helper()
	prev := newFS
	newFS = func(exec.Runner) (fsManager, error) { return f, nil }
	t.Cleanup(func() { newFS = prev })
}

func TestWriteUnit(t *testing.T) {
	f := &fakeFS{}
	f.install(t)

	content := "[Unit]\nDescription=demo\n[Service]\nExecStart=/usr/bin/true\n"
	if err := mgr(t, exectest.New(exec.Sudo)).WriteUnit(context.Background(), "demo.service", content); err != nil {
		t.Fatal(err)
	}
	if f.wrotePath != "/etc/systemd/system/demo.service" || f.wroteContent != content ||
		f.wroteOpts.Mode != 0o644 || f.wroteOpts.Owner != "root" || f.wroteOpts.Group != "root" {
		t.Errorf("WriteUnit wrote path=%q content=%q opts=%+v", f.wrotePath, f.wroteContent, f.wroteOpts)
	}
}

func TestWriteUnit_RejectsInvalidNameBeforeFS(t *testing.T) {
	f := &fakeFS{}
	f.install(t)

	if err := mgr(t, exectest.New(exec.Sudo)).WriteUnit(context.Background(), "evil", "x"); err == nil {
		t.Error("WriteUnit accepted a unit name without a valid type suffix")
	}
	if f.wrotePath != "" {
		t.Error("WriteUnit touched the filesystem for an invalid unit name")
	}
}

func TestRemoveUnit(t *testing.T) {
	f := &fakeFS{}
	f.install(t)

	if err := mgr(t, exectest.New(exec.Sudo)).RemoveUnit(context.Background(), "demo.service"); err != nil {
		t.Fatal(err)
	}
	if f.removedPath != "/etc/systemd/system/demo.service" {
		t.Errorf("RemoveUnit removed %q", f.removedPath)
	}
}

func TestRemoveUnit_WrapsFSError(t *testing.T) {
	f := &fakeFS{removeErr: errors.New("read-only file system")}
	f.install(t)

	err := mgr(t, exectest.New(exec.Sudo)).RemoveUnit(context.Background(), "demo.service")
	if err == nil {
		t.Fatal("RemoveUnit returned nil on an fs error, want a wrapped error")
	}
	if !strings.Contains(err.Error(), "demo.service") {
		t.Errorf("error %q should name the unit path", err.Error())
	}
}

func TestAvailable(t *testing.T) {
	lp, marker := lookPath, systemdRunMarker
	defer func() { lookPath, systemdRunMarker = lp, marker }()

	existingDir := t.TempDir()

	t.Run("systemd present", func(t *testing.T) {
		lookPath = func(string) (string, error) { return "/usr/bin/systemctl", nil }
		systemdRunMarker = existingDir
		if !Available() {
			t.Error("Available = false, want true")
		}
	})
	t.Run("systemctl missing", func(t *testing.T) {
		lookPath = func(string) (string, error) { return "", errors.New("not found") }
		systemdRunMarker = existingDir
		if Available() {
			t.Error("Available = true, want false when systemctl is missing")
		}
	})
	t.Run("not pid 1 (marker absent)", func(t *testing.T) {
		lookPath = func(string) (string, error) { return "/usr/bin/systemctl", nil }
		systemdRunMarker = filepath.Join(existingDir, "does-not-exist")
		if Available() {
			t.Error("Available = true, want false when /run/systemd/system is absent")
		}
	})
}

func TestValidateUnitName(t *testing.T) {
	valid := []string{
		"nginx.service", "dbus.socket", "fstrim.timer", "tmp.mount",
		"-.mount",
		"getty@tty1.service",
		"home-user.mount",
		`dev-disk-by\x2duuid.device`,
	}
	for _, n := range valid {
		if err := ValidateUnitName(n); err != nil {
			t.Errorf("ValidateUnitName(%q) = %v, want nil", n, err)
		}
	}
	invalid := []string{
		"",
		"nginx",
		"nginx.txt",
		".hidden.service",
		"-rf",
		"nginx.service\n",
		"a b.service",
		"unit.SERVICE",
	}
	for _, n := range invalid {
		if err := ValidateUnitName(n); err == nil {
			t.Errorf("ValidateUnitName(%q) = nil, want an error", n)
		}
	}
}

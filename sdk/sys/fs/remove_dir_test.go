//go:build unix

package fs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestIsUnderProtectedPrefix(t *testing.T) {
	refuse := []string{
		"/",
		"/etc", "/etc/sudoers.d", "/etc/sudoers.d/cadestro",
		"/etc/cron.d", "/etc/cron.d/job", "/etc/systemd/system",
		"/boot", "/boot/efi", "/boot/efi/EFI",
		"/var", "/var/lib", "/var/lib/anything", "/var/lib/postgresql/data",
		"/home", "/home/alice", "/home/alice/.ssh",
		"/root", "/root/.ssh",
		"/usr", "/usr/bin", "/usr/lib/systemd",
		"/bin", "/sbin", "/lib", "/lib64",
		"/proc", "/sys", "/dev", "/run",

		"/etc/../etc/sudoers.d", "/home/./bob",
	}
	for _, p := range refuse {
		if !IsUnderProtectedPrefix(p) {
			t.Errorf("IsUnderProtectedPrefix(%q) = false, want true (protected)", p)
		}
	}

	allow := []string{
		"/tmp/managed", "/tmp/foo/bar",
		"/srv/app/data",
		"/opt/myapp/cache",
		"/var/log/myapp",
		"/data/managed",
	}
	for _, p := range allow {
		if IsUnderProtectedPrefix(p) {
			t.Errorf("IsUnderProtectedPrefix(%q) = true, want false (deletable)", p)
		}
	}

	_ = IsUnderProtectedPrefix("some/relative/path")
}

func TestRemoveDir_RefusesProtectedPrefixes(t *testing.T) {
	m := directManager(t)
	for _, p := range []string{
		"/etc/sudoers.d/cadestro",
		"/etc/cron.d",
		"/boot/efi",
		"/var/lib/anything",
		"/home/alice",
		"/root/.ssh",
		"/usr/local",
	} {
		err := m.RemoveDir(context.Background(), p)
		if err == nil {
			t.Errorf("RemoveDir(%q) = nil, want refusal", p)
		}
	}
}

func TestRemoveDir_DeletesManagedTree(t *testing.T) {
	m := directManager(t)
	root := t.TempDir()
	target := filepath.Join(root, "managed")
	sub := filepath.Join(target, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdirall: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sub, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := m.RemoveDir(context.Background(), target); err != nil {
		t.Fatalf("RemoveDir: %v", err)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Errorf("target still exists after RemoveDir: %v", err)
	}

	if _, err := os.Stat(root); err != nil {
		t.Errorf("RemoveDir removed more than its target: %v", err)
	}
}

func TestRemoveDir_RefusesSymlinkedComponent(t *testing.T) {
	m := directManager(t)
	root := t.TempDir()

	victim := filepath.Join(root, "victim")
	if err := os.MkdirAll(filepath.Join(victim, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir victim: %v", err)
	}
	victimFile := filepath.Join(victim, "sub", "keep.txt")
	if err := os.WriteFile(victimFile, []byte("KEEP"), 0o644); err != nil {
		t.Fatalf("seed victim: %v", err)
	}

	link := filepath.Join(root, "link")
	if err := os.Symlink(victim, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	err := m.RemoveDir(context.Background(), filepath.Join(link, "sub"))
	if err == nil {
		t.Fatalf("RemoveDir through a symlinked component: want error, got nil")
	}
	if _, statErr := os.Stat(victimFile); statErr != nil {
		t.Errorf("victim was deleted through a symlinked component: %v", statErr)
	}
}

func TestRemoveDir_RefusesSymlinkTarget(t *testing.T) {
	m := directManager(t)
	root := t.TempDir()
	victim := filepath.Join(root, "victim")
	if err := os.MkdirAll(victim, 0o755); err != nil {
		t.Fatalf("mkdir victim: %v", err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(victim, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if err := m.RemoveDir(context.Background(), link); err == nil {
		t.Fatalf("RemoveDir on a symlink target: want error, got nil")
	}
	if _, err := os.Stat(victim); err != nil {
		t.Errorf("victim removed via symlink leaf: %v", err)
	}
}

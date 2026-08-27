//go:build unix

package fs

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"testing"
)

func TestSetDirPermissionsNoFollow_RealDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "managed")
	if err := os.Mkdir(dir, 0o777); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := SetDirPermissionsNoFollow(dir, 0o750, -1, -1); err != nil {
		t.Fatalf("SetDirPermissionsNoFollow: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o750 {
		t.Errorf("mode = %v, want 0750", perm)
	}
}

func TestSetDirPermissionsNoFollow_AppliesOwnershipToSelf(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "owned")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	u, err := user.Current()
	if err != nil {
		t.Fatalf("current user: %v", err)
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)

	if err := SetDirPermissionsNoFollow(dir, 0o700, uid, gid); err != nil {
		t.Fatalf("SetDirPermissionsNoFollow: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := fileUID(t, info); got != uid {
		t.Errorf("uid = %d, want %d", got, uid)
	}
}

func TestSetDirPermissionsNoFollow_RefusesSymlink(t *testing.T) {
	root := t.TempDir()

	victim := filepath.Join(root, "victim")
	if err := os.Mkdir(victim, 0o700); err != nil {
		t.Fatalf("mkdir victim: %v", err)
	}
	link := filepath.Join(root, "managed")
	if err := os.Symlink(victim, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	err := SetDirPermissionsNoFollow(link, 0o777, -1, -1)
	if err == nil {
		t.Fatalf("SetDirPermissionsNoFollow on a symlink: want error, got nil")
	}

	info, statErr := os.Stat(victim)
	if statErr != nil {
		t.Fatalf("stat victim: %v", statErr)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("victim mode = %v, want 0700 (unchanged) — symlink was dereferenced", perm)
	}
}

func TestSetDirPermissionsNoFollow_RefusesNonDir(t *testing.T) {

	f := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := SetDirPermissionsNoFollow(f, 0o700, -1, -1); err == nil {
		t.Fatalf("SetDirPermissionsNoFollow on a regular file: want error, got nil")
	}
}

func TestSetDirPermissionsNoFollow_Missing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	if err := SetDirPermissionsNoFollow(missing, 0o700, -1, -1); err == nil {
		t.Fatalf("SetDirPermissionsNoFollow on a missing path: want error, got nil")
	}
}

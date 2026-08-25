//go:build unix

package fs

import (
	"context"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"testing"
)

func TestWriteFileAtomic_RefusesSymlinkPlantedTempTarget(t *testing.T) {
	m := directManager(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "managed.conf")

	sentinelDir := t.TempDir()
	sentinel := filepath.Join(sentinelDir, "sentinel")
	if err := os.WriteFile(sentinel, []byte("ORIGINAL"), 0o644); err != nil {
		t.Fatalf("seed sentinel: %v", err)
	}
	planted := target + ".cadestro-tmp"
	if err := os.Symlink(sentinel, planted); err != nil {
		t.Fatalf("plant symlink: %v", err)
	}

	const content = "managed content\n"
	if err := m.WriteFile(context.Background(), target, []byte(content), WriteOptions{Mode: 0o644}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("read sentinel: %v", err)
	}
	if string(got) != "ORIGINAL" {
		t.Fatalf("sentinel was modified through the planted .cadestro-tmp symlink: %q", string(got))
	}

	info, err := os.Lstat(target)
	if err != nil {
		t.Fatalf("lstat target: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("target is a symlink, want a regular file")
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("target is not a regular file: %v", info.Mode())
	}
	gotTarget, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(gotTarget) != content {
		t.Errorf("target content = %q, want %q", string(gotTarget), content)
	}
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Errorf("target mode = %v, want 0644", perm)
	}

	matches, _ := filepath.Glob(filepath.Join(dir, ".managed.conf.tmp-*"))
	if len(matches) != 0 {
		t.Errorf("leftover temp files: %v", matches)
	}
}

func TestWriteFileAtomic_TargetSymlinkNotDereferenced(t *testing.T) {
	m := directManager(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "managed.conf")

	victimDir := t.TempDir()
	victim := filepath.Join(victimDir, "victim")
	if err := os.WriteFile(victim, []byte("VICTIM"), 0o644); err != nil {
		t.Fatalf("seed victim: %v", err)
	}
	if err := os.Symlink(victim, target); err != nil {
		t.Fatalf("plant target symlink: %v", err)
	}

	if err := m.WriteFile(context.Background(), target, []byte("new\n"), WriteOptions{Mode: 0o644}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if got, _ := os.ReadFile(victim); string(got) != "VICTIM" {
		t.Fatalf("victim modified through target symlink deref: %q", string(got))
	}
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatalf("lstat target: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("target still a symlink after write")
	}
}

func TestWriteFileAtomic_AppliesOwnershipToSelf(t *testing.T) {
	m := directManager(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "owned.conf")

	u, err := user.Current()
	if err != nil {
		t.Fatalf("current user: %v", err)
	}
	g, err := user.LookupGroupId(u.Gid)
	if err != nil {
		t.Skipf("cannot resolve current group: %v", err)
	}

	if err := m.WriteFile(context.Background(), target, []byte("x\n"), WriteOptions{Mode: 0o640, Owner: u.Username, Group: g.Name}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o640 {
		t.Errorf("mode = %v, want 0640", perm)
	}
	wantUID, _ := strconv.Atoi(u.Uid)
	if uid := fileUID(t, info); uid != wantUID {
		t.Errorf("uid = %d, want %d", uid, wantUID)
	}
}

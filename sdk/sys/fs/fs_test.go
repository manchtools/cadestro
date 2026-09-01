//go:build integration

package fs_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/fs"
)

func foreignLocale(t *testing.T) string {
	t.Helper()
	out, err := osexec.Command("locale", "-a").Output()
	if err != nil {
		return ""
	}
	installed := strings.Split(string(out), "\n")
	for canonical, variants := range map[string][]string{
		"ja_JP.UTF-8": {"ja_JP.utf8", "ja_JP.UTF-8"},
		"zh_CN.UTF-8": {"zh_CN.utf8", "zh_CN.UTF-8"},
	} {
		for _, v := range variants {
			for _, have := range installed {
				if strings.EqualFold(strings.TrimSpace(have), v) {
					return canonical
				}
			}
		}
	}
	return ""
}

func missingPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "definitely-missing")
}

func intManager(t *testing.T) *fs.Manager {
	t.Helper()
	b := exec.Sudo
	if os.Geteuid() == 0 {
		b = exec.Direct
	}
	r, err := exec.NewRunner(b)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	m, err := fs.New(r)
	if err != nil {
		t.Fatalf("fs.New: %v", err)
	}
	return m
}

func tmpPath(t *testing.T, name string) string {
	t.Helper()
	return fmt.Sprintf("/tmp/cadestro-fs-test-%s-%d", name, os.Getpid())
}

func cleanup(t *testing.T, m *fs.Manager, path string) {
	t.Helper()
	_ = m.Remove(context.Background(), path)
}

func statMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Mode().Perm()
}

func TestWriteAndReadRoundTrip(t *testing.T) {
	ctx := context.Background()
	m := intManager(t)
	path := tmpPath(t, "write")
	defer cleanup(t, m, path)

	content := []byte("hello world\n")
	if err := m.WriteFile(ctx, path, content, fs.WriteOptions{}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if ok, err := m.Exists(ctx, path); err != nil || !ok {
		t.Fatalf("Exists = (%v,%v), want (true,nil)", ok, err)
	}
	got, err := m.ReadFile(ctx, path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("ReadFile = %q, want %q", got, content)
	}
}

func TestReadFileNotFound(t *testing.T) {
	got, err := intManager(t).ReadFile(context.Background(), missingPath(t))

	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ReadFile(missing) err = %v, want errors.Is(..., os.ErrNotExist)", err)
	}
	if got != nil {
		t.Errorf("ReadFile(missing) = %q, want nil", got)
	}
}

func TestWriteFileWithModeAndOwnership(t *testing.T) {
	ctx := context.Background()
	m := intManager(t)
	path := tmpPath(t, "atomic")
	defer cleanup(t, m, path)

	content := []byte("# SSH Config\nPort 22\nPermitRootLogin no\n")
	if err := m.WriteFile(ctx, path, content, fs.WriteOptions{Mode: 0o644, Owner: "root", Group: "root"}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := m.ReadFile(ctx, path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content mismatch:\n  expected: %q\n  got:      %q", content, got)
	}
	if mode := statMode(t, path); mode != 0o644 {
		t.Errorf("mode = %v, want 0644", mode)
	}
	if owner, group := fs.GetOwnership(path); owner != "root" || group != "root" {
		t.Errorf("ownership = %s:%s, want root:root", owner, group)
	}
}

func TestWriteFile_ReplacesSymlinkTargetNotFollowed(t *testing.T) {
	ctx := context.Background()
	m := intManager(t)

	sentinel := tmpPath(t, "sym-sentinel")
	link := tmpPath(t, "sym-link")
	defer cleanup(t, m, sentinel)
	defer cleanup(t, m, link)

	if err := m.WriteFile(ctx, sentinel, []byte("SENTINEL\n"), fs.WriteOptions{}); err != nil {
		t.Fatalf("seed sentinel: %v", err)
	}
	_ = os.Remove(link)
	if err := os.Symlink(sentinel, link); err != nil {
		t.Fatalf("plant symlink: %v", err)
	}

	if err := m.WriteFile(ctx, link, []byte("newcontent\n"), fs.WriteOptions{}); err != nil {
		t.Fatalf("WriteFile(symlinked target): %v", err)
	}

	if got, _ := m.ReadFile(ctx, sentinel); string(got) != "SENTINEL\n" {
		t.Errorf("sentinel was clobbered through the symlink: %q", got)
	}

	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Errorf("target is still a symlink (err=%v, mode=%v); the symlink was followed, not replaced", err, fi.Mode())
	}
	if got, _ := m.ReadFile(ctx, link); string(got) != "newcontent\n" {
		t.Errorf("target content = %q, want the new content", got)
	}
}

func TestSetModeAndOwnership(t *testing.T) {
	ctx := context.Background()
	m := intManager(t)
	path := tmpPath(t, "perms")
	defer cleanup(t, m, path)

	if err := m.WriteFile(ctx, path, []byte("x"), fs.WriteOptions{}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := m.SetMode(ctx, path, 0o600); err != nil {
		t.Fatalf("SetMode: %v", err)
	}
	if mode := statMode(t, path); mode != 0o600 {
		t.Errorf("mode = %v, want 0600", mode)
	}
	if err := m.SetOwnership(ctx, path, "root", "root"); err != nil {
		t.Fatalf("SetOwnership: %v", err)
	}
	if owner, group := fs.GetOwnership(path); owner != "root" || group != "root" {
		t.Errorf("ownership = %s:%s, want root:root", owner, group)
	}
}

func TestExistsRestrictedDir(t *testing.T) {

	var path string
	for _, c := range []string{"/etc/sudoers.d", "/etc/ssl/private", "/root"} {
		if _, err := os.Stat(c); err == nil {
			path = c
			break
		}
	}
	if path == "" {
		t.Skip("no restricted path available on this host image")
	}
	ok, err := intManager(t).Exists(context.Background(), path)
	if err != nil {
		t.Fatalf("Exists(%s): %v", path, err)
	}
	if !ok {
		t.Errorf("expected %s to exist (via the privilege backend)", path)
	}
}

func TestRemove(t *testing.T) {
	ctx := context.Background()
	m := intManager(t)
	path := tmpPath(t, "remove")

	if err := m.WriteFile(ctx, path, []byte("x"), fs.WriteOptions{}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := m.Remove(ctx, path); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if ok, _ := m.Exists(ctx, path); ok {
		t.Error("file should be removed")
	}

	if err := m.Remove(ctx, path); err != nil {
		t.Errorf("Remove of an absent file = %v, want nil (rm -f)", err)
	}
}

func TestCopy(t *testing.T) {
	ctx := context.Background()
	m := intManager(t)
	src := tmpPath(t, "copysrc")
	dst := tmpPath(t, "copydst")
	defer cleanup(t, m, src)
	defer cleanup(t, m, dst)

	content := []byte("copy me\n")
	if err := m.WriteFile(ctx, src, content, fs.WriteOptions{}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := m.Copy(ctx, src, dst, fs.WriteOptions{Mode: 0o600}); err != nil {
		t.Fatalf("Copy: %v", err)
	}
	got, err := m.ReadFile(ctx, dst)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("copied content = %q, want %q", got, content)
	}
	if mode := statMode(t, dst); mode != 0o600 {
		t.Errorf("dst mode = %v, want 0600", mode)
	}
}

func TestCopyTree(t *testing.T) {
	ctx := context.Background()
	m := intManager(t)
	src := tmpPath(t, "treesrc")
	dst := tmpPath(t, "treedst")
	defer func() { _ = m.RemoveDir(ctx, src); _ = m.RemoveDir(ctx, dst) }()

	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{
		filepath.Join(src, ".bashrc"):      "export X=1\n",
		filepath.Join(src, "sub", "f.txt"): "nested\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := m.CopyTree(ctx, src, dst, fs.WriteOptions{}); err != nil {
		t.Fatalf("CopyTree: %v", err)
	}
	for _, rel := range []string{".bashrc", "sub/f.txt"} {
		if ok, err := m.Exists(ctx, filepath.Join(dst, rel)); err != nil || !ok {
			t.Errorf("dst is missing %q after CopyTree (Exists=%v, err=%v)", rel, ok, err)
		}
	}
	if ok, _ := m.Exists(ctx, filepath.Join(dst, filepath.Base(src))); ok {
		t.Error("CopyTree nested the source under dst (dst/<src> exists) — -T should merge into dst")
	}

	if err := m.CopyTree(ctx, src, dst, fs.WriteOptions{Mode: 0o700}); err != nil {
		t.Fatalf("CopyTree (re-run/merge): %v", err)
	}
	if mode := statMode(t, dst); mode != 0o700 {
		t.Errorf("dst root mode = %v, want 0700 (Mode applies to the root)", mode)
	}
}

func TestListMounts_Integration(t *testing.T) {
	mounts, err := intManager(t).ListMounts(context.Background())
	if err != nil {
		t.Fatalf("ListMounts: %v", err)
	}
	if len(mounts) == 0 {
		t.Fatal("ListMounts returned nothing; at least / must be mounted")
	}
	var root *fs.MountInfo
	for i := range mounts {
		if mounts[i].Target == "/" {
			root = &mounts[i]
		}
	}
	if root == nil {
		t.Fatalf("/ not present in enumerated mounts: %+v", mounts)
	}
	if root.Source == "" || root.FSType == "" {
		t.Errorf("root mount under-populated: %+v", *root)
	}
}

func TestMkdirAndRemoveDir(t *testing.T) {
	ctx := context.Background()
	m := intManager(t)
	base := tmpPath(t, "mkdir")
	defer func() { _ = m.RemoveDir(ctx, base) }()

	leaf := base + "/a/b"
	if err := m.Mkdir(ctx, leaf, fs.MkdirOptions{Mode: 0o750, Owner: "root", Group: "root", Recursive: true}); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if ok, _ := m.Exists(ctx, leaf); !ok {
		t.Fatal("nested directory should exist")
	}

	if mode := statMode(t, leaf); mode != 0o750 {
		t.Errorf("leaf dir mode = %v, want 0750", mode)
	}
	if err := m.WriteFile(ctx, base+"/a/b/file.txt", []byte("x"), fs.WriteOptions{}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := m.RemoveDir(ctx, base); err != nil {
		t.Fatalf("RemoveDir: %v", err)
	}
	if ok, _ := m.Exists(ctx, base); ok {
		t.Error("directory should be removed")
	}
}

func TestOwnershipHelper(t *testing.T) {
	for _, tt := range []struct{ owner, group, want string }{
		{"root", "root", "root:root"},
		{"root", "", "root"},
		{"", "root", ":root"},
		{"", "", ""},
		{"user", "group", "user:group"},
	} {
		if got := fs.Ownership(tt.owner, tt.group); got != tt.want {
			t.Errorf("Ownership(%q,%q) = %q, want %q", tt.owner, tt.group, got, tt.want)
		}
	}
}

func TestGetOwnershipMissing(t *testing.T) {
	if owner, group := fs.GetOwnership(missingPath(t)); owner != "" || group != "" {
		t.Errorf("GetOwnership(missing) = %q:%q, want empties", owner, group)
	}
}

func TestReadFile_MissingUnderForeignLocale(t *testing.T) {
	loc := foreignLocale(t)
	if loc == "" {
		t.Skip("no ja/zh locale installed to exercise the locale guard")
	}
	t.Setenv("LANG", loc)
	t.Setenv("LC_ALL", loc)
	got, err := intManager(t).ReadFile(context.Background(), missingPath(t))

	if !errors.Is(err, os.ErrNotExist) || got != nil {
		t.Fatalf("ReadFile(missing) under %s = (%q,%v), want (nil, ErrNotExist) — the Runner must force LC_ALL=C", loc, got, err)
	}
}

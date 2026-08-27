package fs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveAndValidatePath_AbsolutePath(t *testing.T) {

	tmp := t.TempDir()

	path := filepath.Join(tmp, "somefile.txt")
	resolved, err := ResolveAndValidatePath(path)
	if err != nil {
		t.Fatalf("ResolveAndValidatePath: %v", err)
	}

	if resolved != path {
		t.Fatalf("resolved = %q, want %q", resolved, path)
	}
}

func TestResolveAndValidatePath_RelativePath(t *testing.T) {
	_, err := ResolveAndValidatePath("relative/path/file.txt")
	if err == nil {
		t.Fatal("expected error for relative path")
	}
	if !strings.Contains(err.Error(), "path must be absolute") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveAndValidatePath_SymlinkInParent(t *testing.T) {
	tmp := t.TempDir()

	realDir := filepath.Join(tmp, "real_dir")
	if err := os.Mkdir(realDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	linkDir := filepath.Join(tmp, "link_dir")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	path := filepath.Join(linkDir, "file.txt")
	resolved, err := ResolveAndValidatePath(path)
	if err != nil {
		t.Fatalf("ResolveAndValidatePath: %v", err)
	}

	expected := filepath.Join(realDir, "file.txt")
	if resolved != expected {
		t.Fatalf("resolved = %q, want %q", resolved, expected)
	}
}

func TestResolveAndValidatePath_SymlinkToSensitiveLocation(t *testing.T) {
	tmp := t.TempDir()

	evilLink := filepath.Join(tmp, "evil_link")
	if err := os.Symlink("/etc", evilLink); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	path := filepath.Join(evilLink, "passwd")
	resolved, err := ResolveAndValidatePath(path)
	if err != nil {
		t.Fatalf("ResolveAndValidatePath: %v", err)
	}

	if resolved != "/etc/passwd" {
		t.Fatalf("resolved = %q, want /etc/passwd", resolved)
	}
}

func TestResolveAndValidatePath_NonExistentIntermediateDirectories(t *testing.T) {
	tmp := t.TempDir()

	path := filepath.Join(tmp, "a", "b", "c", "file.txt")
	resolved, err := ResolveAndValidatePath(path)
	if err != nil {
		t.Fatalf("ResolveAndValidatePath: %v", err)
	}

	if resolved != path {
		t.Fatalf("resolved = %q, want %q", resolved, path)
	}
}

func TestResolveAndValidatePath_RootPath(t *testing.T) {
	resolved, err := ResolveAndValidatePath("/tmp/testfile")
	if err != nil {
		t.Fatalf("ResolveAndValidatePath: %v", err)
	}

	if resolved != "/tmp/testfile" {
		t.Fatalf("resolved = %q, want /tmp/testfile", resolved)
	}
}

func TestResolveAndValidatePath_CleansDotDot(t *testing.T) {
	tmp := t.TempDir()

	subDir := filepath.Join(tmp, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	path := filepath.Join(tmp, "subdir", "..", "file.txt")
	resolved, err := ResolveAndValidatePath(path)
	if err != nil {
		t.Fatalf("ResolveAndValidatePath: %v", err)
	}

	expected := filepath.Join(tmp, "file.txt")
	if resolved != expected {
		t.Fatalf("resolved = %q, want %q", resolved, expected)
	}
}

func TestResolveAndValidatePath_NestedSymlinks(t *testing.T) {
	tmp := t.TempDir()

	realDir := filepath.Join(tmp, "real")
	if err := os.Mkdir(realDir, 0755); err != nil {
		t.Fatalf("mkdir real: %v", err)
	}

	link1 := filepath.Join(tmp, "link1")
	if err := os.Symlink(realDir, link1); err != nil {
		t.Fatalf("symlink link1: %v", err)
	}

	link2 := filepath.Join(tmp, "link2")
	if err := os.Symlink(link1, link2); err != nil {
		t.Fatalf("symlink link2: %v", err)
	}

	path := filepath.Join(link2, "secret.txt")
	resolved, err := ResolveAndValidatePath(path)
	if err != nil {
		t.Fatalf("ResolveAndValidatePath: %v", err)
	}

	expected := filepath.Join(realDir, "secret.txt")
	if resolved != expected {
		t.Fatalf("resolved = %q, want %q", resolved, expected)
	}
}

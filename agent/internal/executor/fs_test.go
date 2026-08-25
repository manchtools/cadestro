package executor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestStatFile_RejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if _, err := statFile(context.Background(), link); err == nil {
		t.Fatal("statFile must reject a symlink (fail closed), got nil error")
	}

	if _, err := statFile(context.Background(), target); err != nil {
		t.Fatalf("statFile on a regular file must succeed, got %v", err)
	}
}

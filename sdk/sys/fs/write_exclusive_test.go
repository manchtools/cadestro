package fs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileExclusive_CreatesThenRefuses(t *testing.T) {
	m := directManager(t)
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "svc.xml")

	if err := m.WriteFileExclusive(ctx, path, []byte("mine"), WriteOptions{Mode: 0o644}); err != nil {
		t.Fatalf("first WriteFileExclusive: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "mine" {
		t.Fatalf("after create: content = %q, err = %v; want %q", got, err, "mine")
	}

	err = m.WriteFileExclusive(ctx, path, []byte("theirs"), WriteOptions{Mode: 0o644})
	if !errors.Is(err, ErrExists) {
		t.Fatalf("second WriteFileExclusive err = %v, want ErrExists (errors.Is-matchable)", err)
	}

	got, err = os.ReadFile(path)
	if err != nil || string(got) != "mine" {
		t.Errorf("a refused exclusive write modified the file: content = %q, err = %v; want %q untouched", got, err, "mine")
	}
}

func TestWriteFileExclusive_RefusesForeignFile(t *testing.T) {
	m := directManager(t)
	path := filepath.Join(t.TempDir(), "svc.xml")
	if err := os.WriteFile(path, []byte("foreign"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := m.WriteFileExclusive(context.Background(), path, []byte("ours"), WriteOptions{Mode: 0o644})
	if !errors.Is(err, ErrExists) {
		t.Fatalf("err = %v, want ErrExists for a pre-existing foreign file", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "foreign" {
		t.Errorf("foreign content was clobbered: %q", got)
	}
}

func TestWriteFileExclusive_RejectsBackup(t *testing.T) {
	m := directManager(t)
	dir := t.TempDir()
	err := m.WriteFileExclusive(context.Background(), filepath.Join(dir, "a.xml"), []byte("x"),
		WriteOptions{Mode: 0o644, Backup: filepath.Join(dir, "a.bak")})
	if !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("err = %v, want ErrInvalidPath for a Backup on an exclusive write", err)
	}
}

package remote

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
)

func TestMatchRef(t *testing.T) {
	h := func(s string) plumbing.Hash { return plumbing.NewHash(s) }
	hMain := h("1111111111111111111111111111111111111111")
	hTag := h("2222222222222222222222222222222222222222")
	hAnnot := h("3333333333333333333333333333333333333333")
	refs := []*plumbing.Reference{
		plumbing.NewHashReference("refs/heads/main", hMain),
		plumbing.NewHashReference("refs/tags/v1", hTag),
		plumbing.NewHashReference("refs/tags/v2^{}", hAnnot),
	}
	cases := []struct {
		ref     string
		want    plumbing.Hash
		wantHit bool
	}{
		{"main", hMain, true},
		{"v1", hTag, true},
		{"v2", hAnnot, true},
		{"nonexistent", plumbing.ZeroHash, false},
	}
	for _, c := range cases {
		got, ok := matchRef(refs, c.ref)
		if ok != c.wantHit || got != c.want {
			t.Errorf("matchRef(%q) = (%v,%v), want (%v,%v)", c.ref, got, ok, c.want, c.wantHit)
		}
	}
}

func TestCountTreeFiles(t *testing.T) {
	dest := t.TempDir()
	if err := os.WriteFile(filepath.Join(dest, "a"), []byte("123"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dest, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "sub", "b"), []byte("45"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(dest, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, ".git", "HEAD"), []byte("ref: x"), 0o600); err != nil {
		t.Fatal(err)
	}

	n, bytes, err := countTreeFiles(dest)
	if err != nil {
		t.Fatalf("countTreeFiles: %v", err)
	}
	if n != 2 {
		t.Errorf("file count = %d, want 2 (.git skipped)", n)
	}
	if bytes != 5 {
		t.Errorf("total bytes = %d, want 5 (3 + 2)", bytes)
	}
}

func TestRelPathForKey(t *testing.T) {

	if rel, err := relPathForKey("p/", "p/sub/file"); err != nil || rel != "sub/file" {
		t.Errorf("relPathForKey valid = (%q,%v), want (sub/file, nil)", rel, err)
	}
	bad := []struct {
		prefix, key string
		sentinel    error
	}{
		{"p/", "other/file", ErrInvalidConfig},
		{"p/", "p/", ErrInvalidConfig},
		{"p/", "p/a\x00b", ErrUnsafeDestination},
		{"p/", "p/../escape", ErrUnsafeDestination},
		{"p/", "p/sub/../../escape", ErrUnsafeDestination},
	}
	for _, c := range bad {
		if _, err := relPathForKey(c.prefix, c.key); !errors.Is(err, c.sentinel) {
			t.Errorf("relPathForKey(%q,%q) err = %v; want %v", c.prefix, c.key, err, c.sentinel)
		}
	}
}

func TestAssertWithinDest(t *testing.T) {
	dest := t.TempDir()
	if err := assertWithinDest(dest, filepath.Join(dest, "sub", "file")); err != nil {
		t.Errorf("assertWithinDest(inside) = %v, want nil", err)
	}
	if err := assertWithinDest(dest, dest); err != nil {
		t.Errorf("assertWithinDest(dest itself) = %v, want nil", err)
	}
	if err := assertWithinDest(dest, filepath.Dir(dest)+"/escape"); !errors.Is(err, ErrUnsafeDestination) {
		t.Errorf("assertWithinDest(outside) = %v, want ErrUnsafeDestination", err)
	}
}

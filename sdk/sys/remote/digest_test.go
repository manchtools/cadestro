package remote

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestSHA256File_KnownHash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture")
	payload := []byte("the quick brown fox jumps over the lazy dog")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	want := sha256.Sum256(payload)
	wantHex := hex.EncodeToString(want[:])

	got, err := sha256File(path)
	if err != nil {
		t.Fatalf("sha256File: %v", err)
	}
	if got != wantHex {
		t.Fatalf("sha256File = %q; want %q", got, wantHex)
	}
}

func TestSHA256File_LargeFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping multi-MiB hash in -short mode")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "big")

	const size = 4 * 1024 * 1024
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	chunk := make([]byte, 64*1024)
	for i := range chunk {
		chunk[i] = byte(i & 0xff)
	}
	h := sha256.New()
	for written := 0; written < size; written += len(chunk) {
		n := len(chunk)
		if written+n > size {
			n = size - written
		}
		if _, werr := f.Write(chunk[:n]); werr != nil {
			t.Fatalf("write: %v", werr)
		}
		h.Write(chunk[:n])
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	wantHex := hex.EncodeToString(h.Sum(nil))

	got, err := sha256File(path)
	if err != nil {
		t.Fatalf("sha256File: %v", err)
	}
	if got != wantHex {
		t.Fatalf("sha256File = %q; want %q", got, wantHex)
	}
}

func TestSHA256File_MissingPath(t *testing.T) {
	if _, err := sha256File("/this/does/not/exist/at/all"); err == nil {
		t.Fatal("sha256File on missing path returned nil error")
	}
}

func TestSHA256Tree_StableForIdenticalContent(t *testing.T) {
	t1 := buildTree(t, []treeFile{
		{path: "a.txt", body: "alpha"},
		{path: "b.txt", body: "bravo"},
		{path: "sub/c.txt", body: "charlie"},
	})
	t2 := buildTree(t, []treeFile{

		{path: "sub/c.txt", body: "charlie"},
		{path: "b.txt", body: "bravo"},
		{path: "a.txt", body: "alpha"},
	})
	d1, err := sha256Tree(t1)
	if err != nil {
		t.Fatalf("sha256Tree t1: %v", err)
	}
	d2, err := sha256Tree(t2)
	if err != nil {
		t.Fatalf("sha256Tree t2: %v", err)
	}
	if d1 != d2 {
		t.Fatalf("sha256Tree differs across identical content:\n  t1=%s\n  t2=%s", d1, d2)
	}
}

func TestSHA256Tree_DifferentContent_DifferentDigest(t *testing.T) {
	t1 := buildTree(t, []treeFile{{path: "a", body: "x"}})
	t2 := buildTree(t, []treeFile{{path: "a", body: "y"}})
	d1, _ := sha256Tree(t1)
	d2, _ := sha256Tree(t2)
	if d1 == d2 {
		t.Fatalf("sha256Tree collided on different bodies: %s == %s", d1, d2)
	}
}

func TestSHA256Tree_PathMatters(t *testing.T) {
	t1 := buildTree(t, []treeFile{{path: "a", body: "x"}})
	t2 := buildTree(t, []treeFile{{path: "b", body: "x"}})
	d1, _ := sha256Tree(t1)
	d2, _ := sha256Tree(t2)
	if d1 == d2 {
		t.Fatalf("sha256Tree collided on rename: %s == %s", d1, d2)
	}
}

type treeFile struct {
	path string
	body string
}

func buildTree(t *testing.T, files []treeFile) string {
	t.Helper()
	root := t.TempDir()
	for _, f := range files {
		full := filepath.Join(root, f.path)
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatalf("mkdirall: %v", err)
		}
		if err := os.WriteFile(full, []byte(f.body), 0o600); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
	return root
}

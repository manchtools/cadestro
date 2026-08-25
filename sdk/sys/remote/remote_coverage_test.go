package remote

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectArchiveKind_Matrix(t *testing.T) {
	cases := []struct {
		ct, url string
		want    archiveKind
	}{
		{"application/gzip", "", archiveTarGz},
		{"application/x-gzip", "", archiveTarGz},
		{"application/x-tar+gzip", "", archiveTarGz},
		{"application/x-tgz", "", archiveTarGz},
		{"APPLICATION/GZIP", "", archiveTarGz},
		{"application/zip", "", archiveZip},
		{"application/x-zip", "", archiveZip},
		{"application/x-zip-compressed", "", archiveZip},
		{"application/x-xz", "", archiveTarXz},
		{"application/x-tar+xz", "", archiveTarXz},

		{"", "https://h/f.tar.gz", archiveTarGz},
		{"", "https://h/f.tgz", archiveTarGz},
		{"", "https://h/f.zip", archiveZip},
		{"", "https://h/f.tar.xz", archiveTarXz},
		{"", "https://h/f.txz", archiveTarXz},
		{"", "https://h/f.tar.gz?sig=abc#frag", archiveTarGz},
		{"application/octet-stream", "https://h/f.zip", archiveZip},
		{"", "https://h/f.bin", archiveUnknown},
		{"text/plain", "https://h/nope", archiveUnknown},
	}
	for _, c := range cases {
		if got := detectArchiveKind(c.ct, c.url); got != c.want {
			t.Errorf("detectArchiveKind(%q,%q) = %v, want %v", c.ct, c.url, got, c.want)
		}
	}
}

func TestSafeJoinDest_RefusesEscape(t *testing.T) {
	staging := t.TempDir()
	bad := []string{
		"",
		".",
		"/",
		"/etc/passwd",
		"../escape",
		"a/../../escape",
		"../../etc/passwd",
		"sub/../../escape",
		"foo\x00bar",
		"./",
	}
	for _, e := range bad {
		if _, err := safeJoinDest(staging, e); !errors.Is(err, ErrUnsafeDestination) {
			t.Errorf("safeJoinDest(%q) err = %v; want ErrUnsafeDestination", e, err)
		}
	}

	for _, e := range []string{"file", "sub/file", "a/b/c/file", "dir/"} {
		full, err := safeJoinDest(staging, e)
		if err != nil {
			t.Errorf("safeJoinDest(%q) unexpected err: %v", e, err)
			continue
		}
		if !strings.HasPrefix(full, staging) {
			t.Errorf("safeJoinDest(%q) = %q escaped staging %q", e, full, staging)
		}
	}
}

func TestSha256File(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f")
	if err := os.WriteFile(p, []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}

	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	got, err := sha256File(p)
	if err != nil {
		t.Fatalf("sha256File: %v", err)
	}
	if got != want {
		t.Errorf("sha256File = %q, want %q", got, want)
	}

	if _, err := sha256File(filepath.Join(dir, "missing")); err == nil {
		t.Error("sha256File(missing) returned nil error")
	}
}

func TestSha256Tree(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "b"), []byte("2"), 0o600); err != nil {
		t.Fatal(err)
	}
	h1, err := sha256Tree(dir)
	if err != nil {
		t.Fatalf("sha256Tree: %v", err)
	}
	if h1 == "" {
		t.Error("sha256Tree returned empty hash")
	}

	if h2, _ := sha256Tree(dir); h2 != h1 {
		t.Errorf("sha256Tree not deterministic: %q vs %q", h1, h2)
	}

	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if h3, _ := sha256Tree(dir); h3 == h1 {
		t.Error("sha256Tree digest unchanged after a file edit")
	}

	if _, err := sha256Tree(filepath.Join(dir, "no-such-dir")); err == nil {
		t.Error("sha256Tree(missing) returned nil error")
	}
}

func TestGitSource_StringAndWipe(t *testing.T) {
	src, err := NewGit(GitConfig{URL: "https://example.test/repo.git", Ref: "main"})
	if err != nil {
		t.Fatalf("NewGit: %v", err)
	}
	if s := src.String(); !strings.Contains(s, "example.test") {
		t.Errorf("String() = %q, want it to mention the URL host", s)
	}

	if err := src.Wipe(context.Background(), "/etc/cron.d/x"); !errors.Is(err, ErrUnsafeDestination) {
		t.Errorf("Wipe(unmanaged) err = %v; want ErrUnsafeDestination", err)
	}
}

func TestS3Source_StringAndWipe(t *testing.T) {
	src, err := NewS3(S3Config{Endpoint: "https://s3.example.test", Bucket: "b", Key: "k"})
	if err != nil {
		t.Fatalf("NewS3: %v", err)
	}
	if s := src.String(); !strings.Contains(s, "b") || !strings.Contains(s, "k") {
		t.Errorf("String() = %q, want bucket/key", s)
	}
	if err := src.Wipe(context.Background(), "/usr/bin/x"); !errors.Is(err, ErrUnsafeDestination) {
		t.Errorf("Wipe(unmanaged) err = %v; want ErrUnsafeDestination", err)
	}
}

func TestNewGit_RejectsConfig(t *testing.T) {
	bad := []GitConfig{
		{URL: ""},
		{URL: "http://h/r.git"},
		{URL: "ftp://h/r.git"},
		{URL: "https://user:pw@h/r.git"},
		{URL: "https:///r.git"},
		{URL: "https://h/r.git", Ref: "bad ref with space"},
		{URL: "https://h/r.git", Ref: "ref;rm -rf"},
	}
	for _, c := range bad {
		if _, err := NewGit(c); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("NewGit(%+v) err = %v; want ErrInvalidConfig", c, err)
		}
	}
}

func TestNewS3_RejectsConfig(t *testing.T) {
	bad := []S3Config{
		{Endpoint: "", Bucket: "b", Key: "k"},
		{Endpoint: "https://h\x01", Bucket: "b", Key: "k"},
		{Endpoint: "not-absolute", Bucket: "b", Key: "k"},
		{Endpoint: "ftp://h", Bucket: "b", Key: "k"},
		{Endpoint: "https://u:p@h", Bucket: "b", Key: "k"},
		{Endpoint: "https://", Bucket: "b", Key: "k"},
		{Endpoint: "https://h", Bucket: "", Key: "k"},
		{Endpoint: "https://h", Bucket: strings.Repeat("a", 64), Key: "k"},
		{Endpoint: "https://h", Bucket: "b", Key: ""},
		{Endpoint: "https://h", Bucket: "b", Key: strings.Repeat("a", 1025)},
	}
	for _, c := range bad {
		if _, err := NewS3(c); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("NewS3(endpoint=%q bucket=%q keyLen=%d) err = %v; want ErrInvalidConfig",
				c.Endpoint, c.Bucket, len(c.Key), err)
		}
	}
}

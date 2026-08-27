package remote

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type archiveFixture struct {
	srv         *httptest.Server
	body        []byte
	contentType string
	urlPath     string
	etag        string
}

func newArchiveFixture(t *testing.T, body []byte, contentType, urlPath, etag string) *archiveFixture {
	t.Helper()
	if urlPath == "" {
		urlPath = "/archive"
	}
	a := &archiveFixture{body: body, contentType: contentType, urlPath: urlPath, etag: etag}
	a.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", a.etag)
		if a.contentType != "" {
			w.Header().Set("Content-Type", a.contentType)
		}
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = w.Write(a.body)
	}))
	t.Cleanup(a.srv.Close)
	return a
}

func buildTarGz(t *testing.T, files []archiveEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range files {
		hdr := &tar.Header{Name: e.name, Mode: 0o644, Size: int64(len(e.body))}
		switch {
		case e.linkname != "":
			hdr.Typeflag = tar.TypeSymlink
			hdr.Linkname = e.linkname
			hdr.Size = 0
		case e.isDir:
			hdr.Typeflag = tar.TypeDir
			hdr.Mode = 0o755
			hdr.Size = 0
		default:
			hdr.Typeflag = tar.TypeReg
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("tar header: %v", err)
		}
		if e.body != "" {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatalf("tar body: %v", err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gz close: %v", err)
	}
	return buf.Bytes()
}

func buildZip(t *testing.T, files []archiveEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range files {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatalf("zip create: %v", err)
		}
		if _, err := w.Write([]byte(e.body)); err != nil {
			t.Fatalf("zip write: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

type archiveEntry struct {
	name     string
	body     string
	linkname string
	isDir    bool
}

func TestHTTPFetch_ExtractsTarGz(t *testing.T) {
	body := buildTarGz(t, []archiveEntry{
		{name: "a.txt", body: "alpha"},
		{name: "sub/", isDir: true},
		{name: "sub/b.txt", body: "bravo"},
	})
	fix := newArchiveFixture(t, body, "application/gzip", "/x.tar.gz", `"v1"`)
	dest := filepath.Join(t.TempDir(), "tree")
	recordDestUnder(t, dest)

	src, err := NewHTTP(HTTPConfig{URL: fix.srv.URL + fix.urlPath, Extract: true})
	if err != nil {
		t.Fatalf("NewHTTP: %v", err)
	}
	res, err := src.Fetch(context.Background(), dest)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !res.Changed || res.FilesTouched < 2 {
		t.Fatalf("Result = %+v; want Changed=true and FilesTouched≥2", res)
	}

	assertFile(t, filepath.Join(dest, "a.txt"), "alpha")
	assertFile(t, filepath.Join(dest, "sub", "b.txt"), "bravo")
}

func TestHTTPFetch_ExtractsZip(t *testing.T) {
	body := buildZip(t, []archiveEntry{
		{name: "a.txt", body: "alpha"},
		{name: "sub/b.txt", body: "bravo"},
	})
	fix := newArchiveFixture(t, body, "application/zip", "/x.zip", `"v1"`)
	dest := filepath.Join(t.TempDir(), "tree")
	recordDestUnder(t, dest)

	src, _ := NewHTTP(HTTPConfig{URL: fix.srv.URL + fix.urlPath, Extract: true})
	if _, err := src.Fetch(context.Background(), dest); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	assertFile(t, filepath.Join(dest, "a.txt"), "alpha")
	assertFile(t, filepath.Join(dest, "sub", "b.txt"), "bravo")
}

func TestHTTPFetch_RefusesTarXz(t *testing.T) {
	body := []byte("xz body — content irrelevant; type detection happens first")
	for _, ct := range []string{"application/x-xz", "application/x-tar+xz"} {
		t.Run("contentType="+ct, func(t *testing.T) {
			fix := newArchiveFixture(t, body, ct, "/x.tar.xz", `"v1"`)
			dest := filepath.Join(t.TempDir(), "tree")
			recordDestUnder(t, dest)
			src, _ := NewHTTP(HTTPConfig{URL: fix.srv.URL + fix.urlPath, Extract: true})
			_, err := src.Fetch(context.Background(), dest)
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("Fetch err = %v; want errors.Is(..., ErrInvalidConfig) for tar.xz", err)
			}
		})
	}
}

func TestHTTPFetch_RejectsTraversalEntries(t *testing.T) {
	body := buildTarGz(t, []archiveEntry{
		{name: "ok.txt", body: "innocuous"},
		{name: "../../etc/passwd", body: "evil"},
	})
	fix := newArchiveFixture(t, body, "application/gzip", "/x.tar.gz", `"v1"`)
	dest := filepath.Join(t.TempDir(), "tree")
	recordDestUnder(t, dest)
	src, _ := NewHTTP(HTTPConfig{URL: fix.srv.URL + fix.urlPath, Extract: true})

	_, err := src.Fetch(context.Background(), dest)
	if !errors.Is(err, ErrUnsafeDestination) {
		t.Fatalf("Fetch err = %v; want errors.Is(..., ErrUnsafeDestination)", err)
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Fatalf("dest should not exist after traversal rejection: %v", statErr)
	}
}

func TestHTTPFetch_RejectsAbsoluteEntries(t *testing.T) {
	body := buildTarGz(t, []archiveEntry{
		{name: "/etc/passwd", body: "x"},
	})
	fix := newArchiveFixture(t, body, "application/gzip", "/x.tar.gz", `"v1"`)
	dest := filepath.Join(t.TempDir(), "tree")
	recordDestUnder(t, dest)
	src, _ := NewHTTP(HTTPConfig{URL: fix.srv.URL + fix.urlPath, Extract: true})

	_, err := src.Fetch(context.Background(), dest)
	if !errors.Is(err, ErrUnsafeDestination) {
		t.Fatalf("Fetch err = %v; want errors.Is(..., ErrUnsafeDestination)", err)
	}
}

func TestHTTPFetch_RejectsSymlinkEntries(t *testing.T) {
	body := buildTarGz(t, []archiveEntry{
		{name: "link", linkname: "/tmp"},
	})
	fix := newArchiveFixture(t, body, "application/gzip", "/x.tar.gz", `"v1"`)
	dest := filepath.Join(t.TempDir(), "tree")
	recordDestUnder(t, dest)
	src, _ := NewHTTP(HTTPConfig{URL: fix.srv.URL + fix.urlPath, Extract: true})

	_, err := src.Fetch(context.Background(), dest)
	if !errors.Is(err, ErrUnsafeDestination) {
		t.Fatalf("Fetch err = %v; want errors.Is(..., ErrUnsafeDestination)", err)
	}
}

func TestHTTPFetch_ArchiveSizeCap(t *testing.T) {
	body := buildTarGz(t, []archiveEntry{
		{name: "big.bin", body: strings.Repeat("x", 4*1024)},
	})
	fix := newArchiveFixture(t, body, "application/gzip", "/x.tar.gz", `"v1"`)
	dest := filepath.Join(t.TempDir(), "tree")
	recordDestUnder(t, dest)
	src, _ := NewHTTP(HTTPConfig{URL: fix.srv.URL + fix.urlPath, Extract: true, MaxBytes: 1024})
	_, err := src.Fetch(context.Background(), dest)
	if !errors.Is(err, ErrIntegrity) {
		t.Fatalf("Fetch err = %v; want errors.Is(..., ErrIntegrity)", err)
	}
}

func FuzzHTTPArchive_Tar(f *testing.F) {

	f.Add([]byte{0x1f, 0x8b, 0x08, 0x00})
	f.Add(buildTarGz(&testing.T{}, []archiveEntry{{name: "x", body: "y"}}))

	f.Fuzz(func(t *testing.T, body []byte) {

		parent := t.TempDir()
		dest := filepath.Join(parent, "dest")
		if err := os.MkdirAll(dest, 0o700); err != nil {
			t.Fatalf("mkdir dest: %v", err)
		}

		_ = extractTarGzBytes(body, dest, 1<<20)

		ents := dirEntries(t, parent)
		if len(ents) != 1 || ents[0] != "dest" {
			t.Fatalf("extractor wrote outside dest: parent now contains %v", ents)
		}
	})
}

func dirEntries(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(ents))
	for _, e := range ents {
		out = append(out, e.Name())
	}
	return out
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("%s content = %q; want %q", path, got, want)
	}
}

var _ = io.EOF

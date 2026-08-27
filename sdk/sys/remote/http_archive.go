package remote

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type archiveKind int

const (
	archiveUnknown archiveKind = iota
	archiveTarGz
	archiveZip
	archiveTarXz
)

func detectArchiveKind(contentType, rawURL string) archiveKind {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	switch ct {
	case "application/gzip", "application/x-gzip", "application/x-tar+gzip", "application/x-tgz":
		return archiveTarGz
	case "application/zip", "application/x-zip", "application/x-zip-compressed":
		return archiveZip
	case "application/x-xz", "application/x-tar+xz":
		return archiveTarXz
	}

	lowerURL := strings.ToLower(rawURL)

	if i := strings.IndexAny(lowerURL, "?#"); i >= 0 {
		lowerURL = lowerURL[:i]
	}
	switch {
	case strings.HasSuffix(lowerURL, ".tar.gz"), strings.HasSuffix(lowerURL, ".tgz"):
		return archiveTarGz
	case strings.HasSuffix(lowerURL, ".zip"):
		return archiveZip
	case strings.HasSuffix(lowerURL, ".tar.xz"), strings.HasSuffix(lowerURL, ".txz"):
		return archiveTarXz
	}
	return archiveUnknown
}

func (h *httpSource) fetchArchive(ctx context.Context, dest string) (Result, error) {
	if err := validateDestination(dest); err != nil {
		return Result{}, err
	}

	h.mu.Lock()
	cachedRevision := h.revision
	h.mu.Unlock()

	if cachedRevision != "" {
		notModified, err := h.checkNotModified(ctx, cachedRevision)
		if err != nil {
			return Result{}, err
		}
		if notModified {
			return Result{Changed: false, Revision: cachedRevision}, nil
		}
	}

	body, etag, contentType, notModified, err := h.openArchiveBody(ctx, cachedRevision)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = body.Close() }()
	if notModified {

		return Result{Changed: false, Revision: cachedRevision}, nil
	}

	kind := detectArchiveKind(contentType, h.cfg.URL)
	if kind == archiveTarXz {
		return Result{}, fmt.Errorf("%w: tar.xz archives are not supported in v1", ErrInvalidConfig)
	}
	if kind == archiveUnknown {
		return Result{}, fmt.Errorf("%w: unable to detect archive type for %s (content-type=%q)", ErrInvalidConfig, h.cfg.URL, contentType)
	}

	tmp, written, sum, err := streamToTmp(dest+".dl", body, h.cfg.MaxBytes)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = os.Remove(tmp) }()

	staging, err := tmpPathFor(dest + ".staging")
	if err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return Result{}, fmt.Errorf("mkdir staging %s: %w", staging, err)
	}
	cleanupStaging := func() { _ = os.RemoveAll(staging) }

	var filesTouched int
	var extractErr error
	switch kind {
	case archiveTarGz:
		filesTouched, extractErr = extractTarGzFile(tmp, staging, h.cfg.MaxBytes)
	case archiveZip:
		filesTouched, extractErr = extractZipFile(tmp, staging, h.cfg.MaxBytes)
	}
	if extractErr != nil {
		cleanupStaging()
		return Result{}, extractErr
	}

	if _, statErr := os.Stat(dest); statErr == nil {
		if err := os.RemoveAll(dest); err != nil {
			cleanupStaging()
			return Result{}, fmt.Errorf("remove existing dest %s: %w", dest, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		cleanupStaging()
		return Result{}, fmt.Errorf("mkdir %s: %w", filepath.Dir(dest), err)
	}
	if err := os.Rename(staging, dest); err != nil {
		cleanupStaging()
		return Result{}, fmt.Errorf("swap staging → dest: %w", err)
	}

	if err := applyMode(dest, h.cfg.Mode, h.cfg.Owner, h.cfg.Group); err != nil {
		return Result{}, err
	}

	revision := etag
	if revision == "" {
		revision = fmt.Sprintf("sha256:%x", sum)
	}

	h.mu.Lock()
	h.revision = revision
	h.mu.Unlock()

	RecordDest(dest)

	digest, _ := sha256Tree(dest)

	return Result{
		Changed:      true,
		BytesWritten: written,
		FilesTouched: filesTouched,
		Digest:       digest,
		Revision:     revision,
	}, nil
}

func (h *httpSource) openArchiveBody(ctx context.Context, etag string) (io.ReadCloser, string, string, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.cfg.URL, nil)
	if err != nil {
		return nil, "", "", false, fmt.Errorf("GET %s: %w", h.cfg.URL, err)
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, "", "", false, fmt.Errorf("GET %s: %w", h.cfg.URL, err)
	}
	if resp.StatusCode == http.StatusNotModified {
		_ = resp.Body.Close()
		return io.NopCloser(strings.NewReader("")), resp.Header.Get("ETag"), resp.Header.Get("Content-Type"), true, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		return nil, "", "", false, fmt.Errorf("GET %s: status %d", h.cfg.URL, resp.StatusCode)
	}
	return resp.Body, resp.Header.Get("ETag"), resp.Header.Get("Content-Type"), false, nil
}

func extractTarGzFile(tmpPath, staging string, maxBytes int64) (int, error) {
	f, err := os.Open(tmpPath)
	if err != nil {
		return 0, fmt.Errorf("open archive: %w", err)
	}
	defer func() { _ = f.Close() }()
	return extractTarGz(f, staging, maxBytes)
}

func extractTarGzBytes(body []byte, staging string, maxBytes int64) error {
	_, err := extractTarGz(strings.NewReader(string(body)), staging, maxBytes)
	return err
}

func extractTarGz(body io.Reader, staging string, maxBytes int64) (int, error) {
	gz, err := gzip.NewReader(body)
	if err != nil {
		return 0, fmt.Errorf("gzip header: %w", err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)

	var totalBytes int64
	var files int
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return files, fmt.Errorf("tar next: %w", err)
		}

		switch hdr.Typeflag {
		case tar.TypeReg:

		case tar.TypeDir:
			out, perr := safeJoinDest(staging, hdr.Name)
			if perr != nil {
				return files, perr
			}
			if err := os.MkdirAll(out, 0o755); err != nil {
				return files, fmt.Errorf("mkdir %s: %w", out, err)
			}
			continue
		default:
			return files, fmt.Errorf("%w: tar entry %q has disallowed type %v", ErrUnsafeDestination, hdr.Name, hdr.Typeflag)
		}

		out, perr := safeJoinDest(staging, hdr.Name)
		if perr != nil {
			return files, perr
		}

		if hdr.Size < 0 {
			return files, fmt.Errorf("%w: negative size in entry %q", ErrIntegrity, hdr.Name)
		}
		if totalBytes+hdr.Size > maxBytes {
			return files, fmt.Errorf("%w: cumulative size exceeds %d bytes", ErrIntegrity, maxBytes)
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return files, fmt.Errorf("mkdir %s: %w", filepath.Dir(out), err)
		}
		written, werr := writeTarEntry(out, tr, hdr.FileInfo().Mode().Perm())
		if werr != nil {
			return files, werr
		}
		totalBytes += written
		if totalBytes > maxBytes {
			return files, fmt.Errorf("%w: cumulative size exceeds %d bytes", ErrIntegrity, maxBytes)
		}
		files++
	}
	return files, nil
}

func writeTarEntry(out string, r io.Reader, mode os.FileMode) (int64, error) {
	f, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", out, err)
	}
	defer func() { _ = f.Close() }()
	n, err := io.Copy(f, r)
	if err != nil {
		return n, fmt.Errorf("copy %s: %w", out, err)
	}
	return n, nil
}

func extractZipFile(tmpPath, staging string, maxBytes int64) (int, error) {
	zr, err := zip.OpenReader(tmpPath)
	if err != nil {
		return 0, fmt.Errorf("open zip: %w", err)
	}
	defer func() { _ = zr.Close() }()

	var totalBytes int64
	var files int
	for _, ze := range zr.File {
		out, perr := safeJoinDest(staging, ze.Name)
		if perr != nil {
			return files, perr
		}
		if strings.HasSuffix(ze.Name, "/") || ze.FileInfo().IsDir() {
			if err := os.MkdirAll(out, 0o755); err != nil {
				return files, fmt.Errorf("mkdir %s: %w", out, err)
			}
			continue
		}

		declared := int64(ze.UncompressedSize64)
		if declared < 0 {
			return files, fmt.Errorf("%w: negative uncompressed size", ErrIntegrity)
		}
		if totalBytes+declared > maxBytes {
			return files, fmt.Errorf("%w: cumulative size exceeds %d bytes", ErrIntegrity, maxBytes)
		}
		rc, err := ze.Open()
		if err != nil {
			return files, fmt.Errorf("open zip entry %q: %w", ze.Name, err)
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			_ = rc.Close()
			return files, fmt.Errorf("mkdir %s: %w", filepath.Dir(out), err)
		}
		limited := io.LimitReader(rc, maxBytes-totalBytes+1)
		written, werr := writeTarEntry(out, limited, ze.FileInfo().Mode().Perm())
		_ = rc.Close()
		if werr != nil {
			return files, werr
		}
		totalBytes += written
		if totalBytes > maxBytes {
			return files, fmt.Errorf("%w: cumulative size exceeds %d bytes", ErrIntegrity, maxBytes)
		}
		files++
	}
	return files, nil
}

func safeJoinDest(staging, entry string) (string, error) {
	if entry == "" || entry == "." {
		return "", fmt.Errorf("%w: empty or '.' entry name", ErrUnsafeDestination)
	}
	if strings.ContainsRune(entry, 0) {
		return "", fmt.Errorf("%w: entry %q contains NUL", ErrUnsafeDestination, entry)
	}

	trimmed := strings.TrimRight(entry, "/\\")
	if trimmed == "" || trimmed == "." {
		return "", fmt.Errorf("%w: entry %q normalises to empty", ErrUnsafeDestination, entry)
	}

	hostEntry := filepath.FromSlash(trimmed)
	if !filepath.IsLocal(hostEntry) {
		return "", fmt.Errorf("%w: entry %q is not a local path", ErrUnsafeDestination, entry)
	}
	localized, err := filepath.Localize(hostEntry)
	if err != nil {
		return "", fmt.Errorf("%w: entry %q: %v", ErrUnsafeDestination, entry, err)
	}
	full := filepath.Join(staging, localized)

	stagingAbs, err := filepath.Abs(staging)
	if err != nil {
		return "", fmt.Errorf("%w: staging abs: %v", ErrUnsafeDestination, err)
	}
	fullAbs, err := filepath.Abs(full)
	if err != nil {
		return "", fmt.Errorf("%w: full abs: %v", ErrUnsafeDestination, err)
	}
	if !strings.HasPrefix(fullAbs, stagingAbs+string(filepath.Separator)) && fullAbs != stagingAbs {
		return "", fmt.Errorf("%w: entry %q resolves outside staging", ErrUnsafeDestination, entry)
	}
	return full, nil
}

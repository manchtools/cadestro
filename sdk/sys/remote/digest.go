package remote

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const digestBufSize = 64 * 1024

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("sha256File %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	buf := make([]byte, digestBufSize)
	if _, err := io.CopyBuffer(h, f, buf); err != nil {
		return "", fmt.Errorf("sha256File %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func sha256Tree(root string) (string, error) {
	rootClean := filepath.Clean(root)

	type entry struct {
		rel  string
		full string
		mode os.FileMode
		size int64
	}
	var files []entry

	walkErr := filepath.WalkDir(rootClean, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, rerr := filepath.Rel(rootClean, path)
		if rerr != nil {
			return fmt.Errorf("rel %s: %w", path, rerr)
		}

		rel = filepath.ToSlash(rel)
		info, ierr := d.Info()
		if ierr != nil {
			return fmt.Errorf("stat %s: %w", path, ierr)
		}
		files = append(files, entry{rel: rel, full: path, mode: info.Mode().Perm(), size: info.Size()})
		return nil
	})
	if walkErr != nil {
		return "", walkErr
	}

	sort.Slice(files, func(i, j int) bool { return strings.Compare(files[i].rel, files[j].rel) < 0 })

	h := sha256.New()
	hdr := make([]byte, 0, 16)
	buf := make([]byte, digestBufSize)
	for _, e := range files {

		hdr = hdr[:0]
		hdr = binary.BigEndian.AppendUint32(hdr, uint32(len(e.rel)))
		h.Write(hdr)
		h.Write([]byte(e.rel))

		hdr = hdr[:0]
		hdr = binary.BigEndian.AppendUint32(hdr, uint32(e.mode))
		hdr = binary.BigEndian.AppendUint64(hdr, uint64(e.size))
		h.Write(hdr)

		if e.size > 0 {
			f, oerr := os.Open(e.full)
			if oerr != nil {
				return "", fmt.Errorf("open %s: %w", e.full, oerr)
			}
			if _, cerr := io.CopyBuffer(h, f, buf); cerr != nil {
				_ = f.Close()
				return "", fmt.Errorf("read %s: %w", e.full, cerr)
			}
			_ = f.Close()
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

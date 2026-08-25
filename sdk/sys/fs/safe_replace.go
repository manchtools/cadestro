//go:build unix

package fs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

func safeReplaceFile(path string, data []byte, perm os.FileMode, removeExisting bool) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	tmp, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	_ = tmp.Close()

	f, err := os.OpenFile(tmpPath, os.O_RDWR|syscall.O_NOFOLLOW, perm)
	if err != nil {
		cleanup()
		return fmt.Errorf("reopen temp with O_NOFOLLOW: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		cleanup()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := f.Chmod(perm); err != nil {
		_ = f.Close()
		cleanup()
		return fmt.Errorf("chmod temp: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		cleanup()
		return fmt.Errorf("fsync temp: %w", err)
	}
	if err := f.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temp: %w", err)
	}

	if err := safeRename(tmpPath, path, removeExisting); err != nil {
		cleanup()
		return err
	}
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

func safeBackupAndReplace(path, backupPath string, newContent []byte, perm os.FileMode, removeExistingBackup bool) error {

	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		if os.IsNotExist(err) {

			return safeReplaceFile(path, newContent, perm, true)
		}
		return fmt.Errorf("open current %s for backup: %w", path, err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("stat current %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		_ = f.Close()
		return fmt.Errorf("refusing to back up non-regular file %s", path)
	}
	current, err := io.ReadAll(f)
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("read current %s for backup: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close current %s: %w", path, err)
	}
	if err := safeReplaceFile(backupPath, current, perm, removeExistingBackup); err != nil {
		return fmt.Errorf("backup current to %s: %w", backupPath, err)
	}
	return safeReplaceFile(path, newContent, perm, true)
}

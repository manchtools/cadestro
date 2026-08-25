//go:build linux

package fs

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func safeRename(oldPath, newPath string, removeExisting bool) error {
	if removeExisting {
		if err := os.Rename(oldPath, newPath); err != nil {
			return fmt.Errorf("rename %s -> %s: %w", oldPath, newPath, err)
		}
		return nil
	}

	err := unix.Renameat2(unix.AT_FDCWD, oldPath, unix.AT_FDCWD, newPath, unix.RENAME_NOREPLACE)
	switch err {
	case nil:
		return nil
	case unix.EEXIST:
		return fmt.Errorf("rename %s -> %s: %w", oldPath, newPath, ErrExists)
	case unix.ENOSYS, unix.EINVAL:

	default:
		return fmt.Errorf("renameat2 %s -> %s: %w", oldPath, newPath, err)
	}

	if _, statErr := os.Lstat(newPath); statErr == nil {
		return fmt.Errorf("rename %s -> %s: %w", oldPath, newPath, ErrExists)
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("lstat %s: %w", newPath, statErr)
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", oldPath, newPath, err)
	}
	return nil
}

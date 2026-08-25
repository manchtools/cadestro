//go:build unix && !linux

package fs

import (
	"fmt"
	"os"
)

func safeRename(oldPath, newPath string, removeExisting bool) error {
	if !removeExisting {
		if _, err := os.Lstat(newPath); err == nil {
			return fmt.Errorf("rename %s -> %s: %w", oldPath, newPath, ErrExists)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("lstat %s: %w", newPath, err)
		}
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", oldPath, newPath, err)
	}
	return nil
}

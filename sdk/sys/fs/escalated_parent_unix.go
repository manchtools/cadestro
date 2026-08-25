//go:build unix

package fs

import (
	"fmt"
	"os"
	"syscall"
)

func escalatedParentSafe(dir string) error {
	fi, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("escalated write: stat parent %s: %w", dir, err)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%w: %s: cannot determine ownership", ErrUnsafeParentDir, dir)
	}
	groupOrOtherWritable := fi.Mode().Perm()&0o022 != 0
	sticky := fi.Mode()&os.ModeSticky != 0
	if st.Uid != 0 || (groupOrOtherWritable && !sticky) {
		return fmt.Errorf("%w: %s (uid=%d, mode=%#o, sticky=%v) — a non-root user could plant a symlink here",
			ErrUnsafeParentDir, dir, st.Uid, fi.Mode().Perm(), sticky)
	}
	return nil
}

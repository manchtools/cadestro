//go:build !unix

package fs

import "fmt"

func escalatedParentSafe(dir string) error {
	return fmt.Errorf("%w: %s: escalated writes are unsupported on this platform", ErrUnsafeParentDir, dir)
}

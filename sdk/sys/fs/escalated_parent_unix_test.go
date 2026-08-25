//go:build unix

package fs

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

func TestEscalatedParentSafe(t *testing.T) {

	if err := escalatedParentSafe("/usr"); err != nil {
		t.Errorf("escalatedParentSafe(/usr) = %v, want nil (root-owned, 0755)", err)
	}

	dir := t.TempDir()
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := escalatedParentSafe(dir); !errors.Is(err, ErrUnsafeParentDir) {
		t.Errorf("escalatedParentSafe(0777 non-sticky) = %v, want ErrUnsafeParentDir", err)
	}

	if err := escalatedParentSafe(dir + "/does-not-exist"); err == nil {
		t.Error("escalatedParentSafe(missing) = nil, want an error")
	}

	if fi, err := os.Stat("/tmp"); err == nil {
		st, ok := fi.Sys().(*syscall.Stat_t)
		if ok && st.Uid == 0 && fi.Mode()&os.ModeSticky != 0 {
			if err := escalatedParentSafe("/tmp"); err != nil {
				t.Errorf("escalatedParentSafe(/tmp, root-owned sticky) = %v, want nil (sticky exception)", err)
			}
		}
	}
}

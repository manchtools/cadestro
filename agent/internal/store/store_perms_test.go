package store

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyRestrictiveDirMode_FailsClosedOnWideDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file modes are not meaningful on Windows")
	}
	dir := t.TempDir()

	require.NoError(t, os.Chmod(dir, 0o777))
	require.Error(t, verifyRestrictiveDirMode(dir),
		"a group/world-accessible data dir must be rejected, not accepted")

	require.NoError(t, os.Chmod(dir, 0o750))
	require.Error(t, verifyRestrictiveDirMode(dir),
		"any group/world permission bit must be rejected")

	require.NoError(t, os.Chmod(dir, 0o700))
	require.NoError(t, verifyRestrictiveDirMode(dir), "a 0700 dir must be accepted")
}

func TestStoreNew_TightensDataDirAndDBFileModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file modes are not meaningful on Windows")
	}
	dir := t.TempDir()

	require.NoError(t, os.Chmod(dir, 0o777))

	st, err := New(dir)
	require.NoError(t, err)
	defer st.Close()

	di, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), di.Mode().Perm(), "store.New must tighten the data dir to 0700")

	dbInfo, err := os.Stat(filepath.Join(dir, "agent.db"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), dbInfo.Mode().Perm(), "agent.db must be 0600 (it holds action secrets)")

	for _, sidecar := range []string{"agent.db-wal", "agent.db-shm"} {
		if info, serr := os.Stat(filepath.Join(dir, sidecar)); serr == nil {
			assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "%s must be 0600", sidecar)
		}
	}
}

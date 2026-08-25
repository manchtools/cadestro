package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLuksState_RoundTrip(t *testing.T) {
	st, err := New(t.TempDir())
	require.NoError(t, err)
	defer st.Close()

	const actionID = "01HXLUKSSTATE00000000000000"
	const devicePath = "/dev/mapper/luks-test"

	got, err := st.GetLuksState(context.Background(), actionID)
	require.NoError(t, err)
	require.Nil(t, got, "no state before any write")

	require.NoError(t, st.SetLuksOwnershipTaken(context.Background(), actionID, devicePath))
	got, err = st.GetLuksState(context.Background(), actionID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.OwnershipTaken)
	assert.Equal(t, devicePath, got.DevicePath)
	assert.Equal(t, "none", got.DeviceKeyType)

	require.NoError(t, st.SetLuksDeviceKeyType(context.Background(), actionID, "user_passphrase"))
	got, err = st.GetLuksState(context.Background(), actionID)
	require.NoError(t, err)
	assert.Equal(t, "user_passphrase", got.DeviceKeyType)

	rotAt := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	require.NoError(t, st.SetLuksLastRotatedAt(context.Background(), actionID, rotAt))
	got, err = st.GetLuksState(context.Background(), actionID)
	require.NoError(t, err)
	assert.True(t, got.LastRotatedAt.Equal(rotAt), "last_rotated_at round-trips: got %v want %v", got.LastRotatedAt, rotAt)

	require.NoError(t, st.DeleteLuksState(context.Background(), actionID))
	got, err = st.GetLuksState(context.Background(), actionID)
	require.NoError(t, err)
	assert.Nil(t, got, "state gone after delete")
}

func TestLuksPassphraseHistory_KeepsThreeMostRecent(t *testing.T) {
	st, err := New(t.TempDir())
	require.NoError(t, err)
	defer st.Close()

	const actionID = "01HXLUKSHIST00000000000000"
	inserted := []string{"h1", "h2", "h3", "h4", "h5"}
	for _, h := range inserted {
		require.NoError(t, st.AddLuksPassphraseHash(context.Background(), actionID, h))
	}

	got, err := st.GetLuksPassphraseHashes(context.Background(), actionID)
	require.NoError(t, err)
	assert.Len(t, got, 3, "exactly the 3 most recent hashes are retained")
	for _, h := range got {
		assert.Contains(t, inserted, h, "retained hash must be one that was inserted")
	}
}

func TestLpsState_RoundTrip(t *testing.T) {
	st, err := New(t.TempDir())
	require.NoError(t, err)
	defer st.Close()

	const actionID = "01HXLPSSTATE000000000000000"
	rotAt := time.Date(2026, 6, 2, 9, 30, 0, 0, time.UTC)

	require.NoError(t, st.SetLpsUserState(context.Background(), actionID, "alice", rotAt, "hashA"))
	require.NoError(t, st.SetLpsUserState(context.Background(), actionID, "bob", rotAt, "hashB"))

	states, err := st.GetLpsState(context.Background(), actionID)
	require.NoError(t, err)
	require.Len(t, states, 2)
	require.Contains(t, states, "alice")
	assert.Equal(t, "hashA", states["alice"].PasswordHash)
	assert.True(t, states["alice"].LastRotatedAt.Equal(rotAt))

	require.NoError(t, st.SetLpsUserState(context.Background(), actionID, "alice", rotAt, "hashA2"))
	states, err = st.GetLpsState(context.Background(), actionID)
	require.NoError(t, err)
	assert.Equal(t, "hashA2", states["alice"].PasswordHash)

	require.NoError(t, st.DeleteLpsState(context.Background(), actionID))
	states, err = st.GetLpsState(context.Background(), actionID)
	require.NoError(t, err)
	assert.Empty(t, states)
}

func TestAgentDB_FileModeIs0600(t *testing.T) {

	dir := filepath.Join(t.TempDir(), "data")
	require.NoError(t, os.Mkdir(dir, 0o755))
	st, err := New(dir)
	require.NoError(t, err)
	defer st.Close()

	require.NoError(t, st.SetLuksOwnershipTaken(context.Background(), "01HXMODECHECK0000000000000", "/dev/mapper/x"))

	for _, name := range []string{"agent.db", "agent.db-wal", "agent.db-shm"} {
		path := filepath.Join(dir, name)
		info, statErr := os.Stat(path)
		if os.IsNotExist(statErr) {
			continue
		}
		require.NoError(t, statErr)
		assert.Equalf(t, os.FileMode(0o600), info.Mode().Perm(),
			"%s must be 0600 (holds action secrets), got %v", name, info.Mode().Perm())
	}

	dirInfo, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm(), "data dir must be 0700")
}

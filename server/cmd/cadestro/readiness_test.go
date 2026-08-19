package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type readinessStoreStub struct{ err error }

func (s readinessStoreStub) Ping(context.Context) error { return s.err }

func TestCheckReadinessChecksDatabaseAndArtifactPath(t *testing.T) {
	artifactPath := t.TempDir()
	require.NoError(t, checkReadiness(context.Background(), readinessStoreStub{}, artifactPath, "", 0))

	assert.ErrorContains(t,
		checkReadiness(context.Background(), readinessStoreStub{err: errors.New("down")}, artifactPath, "", 0),
		"database")

	file := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
	assert.ErrorContains(t,
		checkReadiness(context.Background(), readinessStoreStub{}, file, "", 0),
		"artifact path")
}

func TestCheckReadinessBackupPolicy(t *testing.T) {
	artifactPath := t.TempDir()
	base := time.Now().UTC()

	writeStatus := func(t *testing.T, directory string, completedAt time.Time) {
		t.Helper()
		artifact := []byte("backup")
		name := "sqlite-backup.db"
		require.NoError(t, os.WriteFile(filepath.Join(directory, name), artifact, 0o600))
		marker := map[string]any{
			"version": 1, "completed_at": completedAt, "artifact": name,
			"size_bytes": len(artifact), "sha256": "0000000000000000000000000000000000000000000000000000000000000000",
		}
		contents, err := json.Marshal(marker)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(directory, "backup-status.json"), contents, 0o600))
	}

	check := func(directory string, maxLag time.Duration) error {
		return checkReadiness(context.Background(), readinessStoreStub{}, artifactPath, directory, maxLag)
	}

	recent := t.TempDir()
	writeStatus(t, recent, base.Add(-time.Minute))
	require.NoError(t, check(recent, time.Hour))

	stale := t.TempDir()
	writeStatus(t, stale, base.Add(-2*time.Hour))
	assert.ErrorContains(t, check(stale, time.Hour), "stale")

	invalid := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(invalid, "backup-status.json"), []byte(`{}`), 0o600))
	assert.ErrorContains(t, check(invalid, time.Hour), "backup status")

	failed := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(failed, "backup-status.json"), []byte(`{"status":"failed"}`), 0o600))
	assert.ErrorContains(t, check(failed, time.Hour), "backup status")

	assert.ErrorContains(t, check(t.TempDir(), time.Hour), "stale")

	assert.NoError(t, check("", 0), "an explicitly disabled backup policy is not a failure")
	assert.NoError(t, check(t.TempDir(), 0), "an unconfigured backup policy is not a failure")

	storageErrorPath := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(storageErrorPath, []byte("x"), 0o600))
	err := check(storageErrorPath, time.Hour)
	assert.ErrorContains(t, err, "backup status")
}

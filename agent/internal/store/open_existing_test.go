package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenExisting_RequiresInitialisedDB(t *testing.T) {
	dir := t.TempDir()
	_, err := OpenExisting(dir)
	require.Error(t, err, "OpenExisting on a non-existent DB must error, not create an empty one")
	assert.Contains(t, err.Error(), "does not exist",
		"the error must be the missing-DB rejection, not an incidental failure (#174)")
	assert.NoFileExists(t, filepath.Join(dir, "agent.db"), "OpenExisting must not create the database")
}

func TestOpenExisting_OpensWithoutMigrating(t *testing.T) {
	dir := t.TempDir()

	svc, err := New(dir)
	require.NoError(t, err)
	require.NoError(t, svc.SetTTYEnabled(context.Background(), true))
	require.NoError(t, svc.Close())

	cli, err := OpenExisting(dir)
	require.NoError(t, err)
	defer cli.Close()

	enabled, err := cli.IsTTYEnabled(context.Background())
	require.NoError(t, err)
	assert.True(t, enabled, "OpenExisting must read the setting the service wrote")
}

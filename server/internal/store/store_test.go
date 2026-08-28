package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewAppliesCoreSchema(t *testing.T) {
	store, err := New(context.Background(), filepath.Join(t.TempDir(), "control.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	count, err := store.Queries().CountDevices(context.Background())
	require.NoError(t, err)
	require.Zero(t, count)
}

package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	generated "github.com/manchtools/cadestro/server/internal/store/generated"
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

func TestDatabaseOwnsLifecycleTimestamps(t *testing.T) {
	ctx := context.Background()
	database, err := New(ctx, filepath.Join(t.TempDir(), "control.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	role, err := database.Queries().CreateRole(ctx, generated.CreateRoleParams{ID: "01K00000000000000000000001", Name: "Timestamp test", Description: ""})
	require.NoError(t, err)
	require.WithinDuration(t, time.Now().UTC(), role.CreatedAt, 10*time.Second)
	require.Equal(t, role.CreatedAt, role.UpdatedAt)
	time.Sleep(1100 * time.Millisecond)
	role, err = database.Queries().SetRoleDescription(ctx, generated.SetRoleDescriptionParams{ID: role.ID, Description: role.Description})
	require.NoError(t, err)
	require.True(t, role.UpdatedAt.After(role.CreatedAt))
}

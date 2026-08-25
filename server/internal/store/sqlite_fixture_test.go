package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/oklog/ulid/v2"

	"github.com/manchtools/cadestro/server/internal/store"
	"github.com/manchtools/cadestro/server/internal/testdb"
)

func setupSQLitePool(t *testing.T, _ int) (*store.Store, *testdb.DB) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "cadestro.db")
	st, err := store.New(ctx, path)
	if err != nil {
		t.Fatalf("store test fixture: initialize SQLite: %v", err)
	}
	t.Cleanup(st.Close)
	raw, err := testdb.Open(ctx, path)
	if err != nil {
		t.Fatalf("store test fixture: open raw SQLite handle: %v", err)
	}
	t.Cleanup(raw.Close)
	return st, raw
}

func setupSQLite(t *testing.T) (*store.Store, *testdb.DB) {
	t.Helper()
	return setupSQLitePool(t, 0)
}

func newID() string { return ulid.Make().String() }

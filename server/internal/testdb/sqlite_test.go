package testdb_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/manchtools/cadestro/server/internal/testdb"
)

func TestOpenPragmas(t *testing.T) {
	ctx := context.Background()
	db, err := testdb.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)

	var busyTimeout, foreignKeys, synchronous int
	var journalMode string
	if err := db.QueryRow(ctx, "PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, "PRAGMA synchronous").Scan(&synchronous); err != nil {
		t.Fatal(err)
	}
	if busyTimeout != 5000 || foreignKeys != 1 || !strings.EqualFold(journalMode, "wal") || synchronous != 2 {
		t.Fatalf("unexpected SQLite pragmas: busy_timeout=%d foreign_keys=%d journal_mode=%q synchronous=%d", busyTimeout, foreignKeys, journalMode, synchronous)
	}
}

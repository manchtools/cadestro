package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/manchtools/cadestro/agent/internal/store/migrations"
)

func TestBaselineMigrationUsesScheduledWork(t *testing.T) {
	dir := t.TempDir()
	st, err := New(dir)
	require.NoError(t, err)
	require.NoError(t, st.Close())
	db, err := sql.Open("sqlite", filepath.Join(dir, "agent.db"))
	require.NoError(t, err)
	defer db.Close()
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'scheduled_work'`).Scan(&count))
	require.Equal(t, 1, count)
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('scheduled_work') WHERE name = 'kind'`).Scan(&count))
	require.Zero(t, count)
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'scheduled_work_occurrences'`).Scan(&count))
	require.Zero(t, count)
}

func TestBaselineMigrationSupportsRollback(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "agent.db"))
	require.NoError(t, err)
	defer db.Close()
	goose.SetBaseFS(migrations.FS)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.Up(db, "."))
	require.NoError(t, goose.Down(db, "."))
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'scheduled_work'`).Scan(&count))
	require.Zero(t, count)
	require.NoError(t, goose.Up(db, "."))
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'scheduled_work'`).Scan(&count))
	require.Equal(t, 1, count)
}

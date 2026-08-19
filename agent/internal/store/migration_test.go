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

func TestScheduledWorkMigrationUpgradesExistingOneShotStore(t *testing.T) {
	dir := t.TempDir()
	st, err := New(dir)
	require.NoError(t, err)
	require.NoError(t, st.Close())

	db, err := sql.Open("sqlite", filepath.Join(dir, "agent.db"))
	require.NoError(t, err)
	defer db.Close()
	goose.SetBaseFS(migrations.FS)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.Down(db, ".")) // remove the new 003 from the released 002 schema

	var policyColumn int
	err = db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('manifest_deliveries') WHERE name = 'policy'`).Scan(&policyColumn)
	require.NoError(t, err)
	require.Zero(t, policyColumn, "the released transport schema has no policy discriminator")
	require.NoError(t, goose.Up(db, "."))
	err = db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('scheduled_work') WHERE name = 'kind'`).Scan(&policyColumn)
	require.NoError(t, err)
	require.Equal(t, 1, policyColumn, "003 must create the single scheduled-work discriminator")
	err = db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('scheduled_work') WHERE name = 'policy'`).Scan(&policyColumn)
	require.NoError(t, err)
	require.Zero(t, policyColumn, "transport policy state must not survive the cutover")
}

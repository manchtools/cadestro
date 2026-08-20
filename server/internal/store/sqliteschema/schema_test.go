package sqliteschema_test

import (
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/manchtools/cadestro/server/internal/store/sqliteschema"
)

func TestBaselineEnablesRequiredSQLitePosture(t *testing.T) {
	db := openBaseline(t)
	assertPragma(t, db, "foreign_keys", "1")
	assertPragma(t, db, "journal_mode", "wal")
	assertPragma(t, db, "synchronous", "2")
	assertPragma(t, db, "user_version", "1")
	assertPragma(t, db, "integrity_check", "ok")
	var tables int
	require.NoError(t, db.QueryRowContext(t.Context(), `
		SELECT count(*) FROM sqlite_schema
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
		  AND name NOT LIKE 'search_fts%' AND name NOT LIKE 'search_trigram%'`).Scan(&tables))
	assert.Equal(t, 51, tables)
}

func TestBaselineEnforcesForeignKeys(t *testing.T) {
	db := openBaseline(t)
	_, err := db.ExecContext(t.Context(), `
		INSERT INTO device_labels (device_id, key, value)
		VALUES ('00000000000000000000000009', 'site', 'berlin')`)
	assert.ErrorContains(t, err, "FOREIGN KEY constraint failed")
}

func TestBaselineEnforcesCaseInsensitiveActiveEmailUniqueness(t *testing.T) {
	db := openBaseline(t)
	_, err := db.ExecContext(t.Context(), `
		INSERT INTO users (id, email) VALUES ('00000000000000000000000009', 'operator@example.test')`)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `
		INSERT INTO users (id, email) VALUES ('00000000000000000000000008', 'OPERATOR@example.test')`)
	assert.ErrorContains(t, err, "UNIQUE constraint failed")
}

func TestBaselineAuditRowsAreAppendOnly(t *testing.T) {
	db := openBaseline(t)
	_, err := db.ExecContext(t.Context(), `
		INSERT INTO audit_operations (
			operation_id, chain_seq, operation_class, actor_type, origin,
			request_descriptor, authorization_outcome, result, occurred_at)
		VALUES ('00000000000000000000000009', 1, 'MUTATION', 'user', 'rpc',
			'test', 'ALLOWED', 'SUCCESS', CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `DELETE FROM audit_operations`)
	assert.ErrorContains(t, err, "append-only")
	_, err = db.ExecContext(t.Context(), `UPDATE audit_operations SET result = 'FAILURE'`)
	assert.ErrorContains(t, err, "append-only")
}

func TestBaselineRejectsInvalidAuditFieldNames(t *testing.T) {
	db := openBaseline(t)
	_, err := db.ExecContext(t.Context(), `
		INSERT INTO audit_operations (
			operation_id, chain_seq, operation_class, actor_type, origin,
			request_descriptor, authorization_outcome, result, occurred_at)
		VALUES ('00000000000000000000000009', 1, 'MUTATION', 'user', 'rpc',
			'test', 'ALLOWED', 'SUCCESS', CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `
		INSERT INTO audit_effects (
			effect_id, operation_id, chain_seq, effect_seq, resource_type,
			resource_id, action, outcome, changed_fields, occurred_at)
		VALUES ('00000000000000000000000008', '00000000000000000000000009', 2, 0,
			'device', '00000000000000000000000007', 'CREATE', 'APPLIED',
			'[{"bad":1}]', CURRENT_TIMESTAMP)`)
	assert.ErrorContains(t, err, "audit changed_fields contains an invalid field name")
}

func TestBaselineFTS5ProvidesPrefixAndTrigramCandidates(t *testing.T) {
	db := openBaseline(t)
	_, err := db.ExecContext(t.Context(), `
		INSERT INTO search_documents (scope, entity_id, primary_text, description)
		VALUES ('devices', '00000000000000000000000009', 'straße-host-01', 'Berlin workstation')`)
	require.NoError(t, err)
	for _, query := range []struct{ name, table, term string }{
		{"prefix", "search_fts", "straße*"}, {"trigram candidate", "search_trigram", "stra"},
	} {
		t.Run(query.name, func(t *testing.T) {
			var id string
			require.NoError(t, db.QueryRowContext(t.Context(), fmt.Sprintf(`
				SELECT d.entity_id FROM %s AS f JOIN search_documents AS d ON d.rowid = f.rowid
				WHERE %s MATCH ?`, query.table, query.table), query.term).Scan(&id))
			assert.Equal(t, "00000000000000000000000009", id)
		})
	}
}

func openBaseline(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "control.sqlite")
	dsn := (&url.URL{Scheme: "file", Path: path}).String() +
		"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)" +
		"&_pragma=busy_timeout(5000)&_time_format=sqlite"
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	db.SetMaxOpenConns(4)
	t.Cleanup(func() { _ = db.Close() })
	schema, err := sqliteschema.FS.ReadFile("schema.sql")
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), string(schema))
	require.NoError(t, err)
	return db
}
func assertPragma(t *testing.T, db *sql.DB, pragma, want string) {
	t.Helper()
	var got string
	require.NoError(t, db.QueryRowContext(t.Context(), "PRAGMA "+pragma).Scan(&got))
	assert.Equal(t, want, got)
}

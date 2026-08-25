package store_test

import (
	"context"
	"testing"

	"github.com/manchtools/cadestro/server/internal/store"
	"github.com/manchtools/cadestro/server/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func scanStrings(t *testing.T, raw *testdb.DB, statement string, args ...any) []string {
	t.Helper()
	rows, err := raw.Query(context.Background(), statement, args...)
	require.NoError(t, err)
	defer rows.Close()

	out := make([]string, 0)
	for rows.Next() {
		var value string
		require.NoError(t, rows.Scan(&value))
		out = append(out, value)
	}
	require.NoError(t, rows.Err())
	return out
}

func applicationTables(t *testing.T, raw *testdb.DB) []string {
	t.Helper()
	tables := scanStrings(t, raw, `
		SELECT name FROM sqlite_schema
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
		ORDER BY name`)
	require.NotEmpty(t, tables, "matches-zero guard: SQLite returned no tables")
	return tables
}

func TestSchema_HoldsNoProjectionArtifacts(t *testing.T) {
	_, raw := setupSQLite(t)
	tables := applicationTables(t, raw)
	for _, name := range tables {
		assert.NotContains(t, name, "_projection")
	}
	assert.NotContains(t, tables, "events")

	versioned := scanStrings(t, raw, `
		SELECT m.name || '.' || p.name
		FROM sqlite_schema AS m, pragma_table_xinfo(m.name) AS p
		WHERE m.type = 'table' AND p.name = 'projection_version'`)
	assert.Empty(t, versioned)
}

func TestSchema_HoldsNoLocalAuthenticationSecrets(t *testing.T) {
	_, raw := setupSQLite(t)
	found := scanStrings(t, raw, `
		SELECT m.name || '.' || p.name
		FROM sqlite_schema AS m, pragma_table_xinfo(m.name) AS p
		WHERE m.type = 'table' AND (
			p.name LIKE '%password_hash%' OR p.name LIKE '%totp%' OR
			p.name LIKE '%backup_code%' OR p.name LIKE '%mfa%' OR
			p.name LIKE '%webauthn%' OR p.name = 'has_password')`)
	assert.Empty(t, found)

	tables := applicationTables(t, raw)
	for _, forbidden := range []string{"totp", "totp_projection", "webauthn_credentials"} {
		assert.NotContains(t, tables, forbidden)
	}
}

func TestSchema_UsesTextIdentifiersAndNoUUIDs(t *testing.T) {
	_, raw := setupSQLite(t)
	bad := scanStrings(t, raw, `
		SELECT m.name || '.' || p.name || ' ' || p.type
		FROM sqlite_schema AS m, pragma_table_xinfo(m.name) AS p
		WHERE m.type = 'table'
		  AND p.name IN ('id', 'operation_id', 'effect_id', 'job_id')
		  AND NOT (m.name = 'linux_uid_sequence' AND p.name = 'id')
		  AND m.name <> 'goose_db_version'
		  AND m.name NOT GLOB 'search_fts_*'
		  AND m.name NOT GLOB 'search_trigram_*'
		  AND upper(p.type) <> 'TEXT'`)
	assert.Empty(t, bad, "identifier columns must use SQLite TEXT: %v", bad)
}

func TestSchema_UsesCentralFTS5Documents(t *testing.T) {
	_, raw := setupSQLite(t)
	tables := applicationTables(t, raw)
	for _, required := range []string{"search_documents", "search_fts", "search_trigram"} {
		assert.Contains(t, tables, required)
	}

	triggers := scanStrings(t, raw, `
		SELECT name FROM sqlite_schema
		WHERE type = 'trigger' AND tbl_name = 'search_documents'
		ORDER BY name`)
	assert.ElementsMatch(t, []string{
		"search_documents_insert", "search_documents_delete", "search_documents_update",
	}, triggers)
}

func TestSchema_SearchDocumentsCannotStoreClassifiedAuditFields(t *testing.T) {
	_, raw := setupSQLite(t)
	columns := scanStrings(t, raw, `SELECT name FROM pragma_table_xinfo('search_documents') ORDER BY cid`)
	require.NotEmpty(t, columns)
	assert.Contains(t, columns, "primary_text")
	assert.Contains(t, columns, "fields")
	assert.NotContains(t, columns, "evidence_fingerprint")
	assert.NotContains(t, columns, "sealed_detail")
}

func TestSchema_EveryAuditEvidenceTableHasAppendOnlyGuards(t *testing.T) {
	_, raw := setupSQLite(t)
	for _, table := range []string{"audit_operations", "audit_effects"} {
		triggers := scanStrings(t, raw, `
			SELECT name FROM sqlite_schema
			WHERE type = 'trigger' AND tbl_name = ? AND name LIKE '%block_%'
			ORDER BY name`, table)
		assert.Len(t, triggers, 2, "%s needs UPDATE and DELETE guards", table)
	}
}

func TestSchema_EnforcesCaseInsensitiveActiveEmailUniqueness(t *testing.T) {
	_, raw := setupSQLite(t)
	ctx := context.Background()
	_, err := raw.Exec(ctx, `INSERT INTO users (id, email) VALUES ($1, $2)`, newID(), "operator@example.test")
	require.NoError(t, err)
	_, err = raw.Exec(ctx, `INSERT INTO users (id, email) VALUES ($1, $2)`, newID(), "OPERATOR@example.test")
	assert.ErrorContains(t, err, "UNIQUE constraint failed")
}

func TestSchema_AuditTablesRejectUpdateAndDelete(t *testing.T) {
	st, raw := setupSQLite(t)
	ctx := context.Background()
	id := newID()
	_, err := st.WithAudit(ctx, mutationOp(), func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		if err := insertDevice(ctx, tx, id, "schema-append-only.example.test"); err != nil {
			return err
		}
		rec.Effect(deviceEffect(id))
		return nil
	})
	require.NoError(t, err)

	_, err = raw.Exec(ctx, `UPDATE audit_operations SET result = 'FAILURE'`)
	assert.ErrorContains(t, err, "append-only")
	_, err = raw.Exec(ctx, `DELETE FROM audit_operations`)
	assert.ErrorContains(t, err, "append-only")
	_, err = raw.Exec(ctx, `UPDATE audit_effects SET outcome = 'REJECTED'`)
	assert.ErrorContains(t, err, "append-only")
	_, err = raw.Exec(ctx, `DELETE FROM audit_effects`)
	assert.ErrorContains(t, err, "append-only")
}

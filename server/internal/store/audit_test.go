package store_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manchtools/cadestro/server/internal/store"
	"github.com/manchtools/cadestro/server/internal/store/generated"
	"github.com/manchtools/cadestro/server/internal/testdb"
)

func sha256hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func mutationOp() store.AuditOperation {
	return store.AuditOperation{
		Class: store.ClassMutation, ActorType: "user", ActorID: newID(), Origin: "rpc",
		OriginFingerprint:    sha256hex("198.51.100.7"),
		RequestDescriptor:    "cadestro.v1.ControlService/RegisterDevice",
		AuthorizationOutcome: store.AuthorizationAllowed, AuthorizationDetail: "RegisterDevice",
		Result: store.ResultSuccess, ResultCode: "OK",
	}
}

func insertDevice(ctx context.Context, tx *store.Tx, id, hostname string) error {
	at := time.Now().UTC()
	_, err := tx.InsertDevice(ctx, generated.InsertDeviceParams{
		ID: id, Hostname: hostname, AgentVersion: "1.0.0",
		RegisteredAt: &at, LastSeenAt: &at,
	})
	return err
}

func deviceEffect(id string) store.AuditEffect {
	return store.AuditEffect{ResourceType: "device", ResourceID: id, Action: "CREATE",
		Outcome: store.EffectApplied, ChangedFields: []string{"hostname", "agent_version"}}
}

func countRows(t *testing.T, pool *testdb.DB, table string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, pool.QueryRow(context.Background(), fmt.Sprintf("SELECT count(*) FROM %s", table)).Scan(&n))
	return n
}

func TestWithAudit_CommitsStateAndEvidenceAtomically(t *testing.T) {
	st, raw := setupSQLite(t)
	id := newID()
	record, err := st.WithAudit(context.Background(), mutationOp(), func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		if err := insertDevice(ctx, tx, id, "alpha.example.test"); err != nil {
			return err
		}
		rec.Effect(deviceEffect(id))
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), record.OperationSeq)
	assert.Equal(t, []int64{2}, record.EffectSeqs)
	assert.Equal(t, int64(2), record.HeadSeq)
	assert.Equal(t, int64(1), countRows(t, raw, "audit_operations"))
	assert.Equal(t, int64(1), countRows(t, raw, "audit_effects"))
}

func TestWithAudit_RollsBackStateWhenAuditFails(t *testing.T) {
	st, raw := setupSQLite(t)
	id := newID()
	op := mutationOp()
	_, err := st.WithAudit(context.Background(), op, func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		if err := insertDevice(ctx, tx, id, "alpha.example.test"); err != nil {
			return err
		}
		rec.Effect(store.AuditEffect{ResourceType: "device", ResourceID: id, Action: "invalid action", Outcome: store.EffectApplied})
		return nil
	})
	require.Error(t, err)
	assert.Zero(t, countRows(t, raw, "devices"))
	assert.Zero(t, countRows(t, raw, "audit_operations"))
	assert.Zero(t, countRows(t, raw, "audit_effects"))
}

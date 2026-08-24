package store_test

import (
	"context"
	"testing"

	"github.com/manchtools/cadestro/server/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/actionparams"
	"github.com/manchtools/cadestro/server/internal/crypto"
	"github.com/manchtools/cadestro/server/internal/manifest"
	"google.golang.org/protobuf/proto"
)

type manifestFixture struct {
	compiler         *manifest.Compiler
	raw              *testdb.DB
	action1, action2 string
	set1, set2       string
	definition       string
}

func newManifestFixture(t *testing.T) *manifestFixture {
	t.Helper()
	st, raw := setupSQLite(t)
	ctx := context.Background()
	action1, action2 := newID(), newID()
	_, err := raw.Exec(ctx, `
		INSERT INTO actions
			(id, name, action_type, desired_state, params, timeout_seconds, schedule, created_at)
		VALUES
			($1, 'first', $3, 1, '{}', 30, '{"runOnAssign":true}', CURRENT_TIMESTAMP),
			($2, 'second', $4, 2, '{}', 60, '{"cron":"0 3 * * *"}', CURRENT_TIMESTAMP)
	`, action1, action2, int32(cadestrov1.ActionType_ACTION_TYPE_UPDATE), int32(cadestrov1.ActionType_ACTION_TYPE_UPDATE))
	require.NoError(t, err)
	set1, set2 := newID(), newID()
	_, err = raw.Exec(ctx, `
		INSERT INTO action_sets (id, name, schedule, on_failure, created_at) VALUES
			($1, 'daily', '{"cron":"0 4 * * *"}', $3, CURRENT_TIMESTAMP),
			($2, 'on assign', '{"runOnAssign":true}', $4, CURRENT_TIMESTAMP)
	`, set1, set2, int32(cadestrov1.OnFailure_ON_FAILURE_STOP), int32(cadestrov1.OnFailure_ON_FAILURE_CONTINUE))
	require.NoError(t, err)
	_, err = raw.Exec(ctx, `
		INSERT INTO action_set_members (set_id, action_id, sort_order, added_at) VALUES
			($1, $4, 20, CURRENT_TIMESTAMP), ($1, $3, 10, CURRENT_TIMESTAMP), ($2, $3, 0, CURRENT_TIMESTAMP)
	`, set1, set2, action1, action2)
	require.NoError(t, err)
	definition := newID()
	_, err = raw.Exec(ctx, `
		INSERT INTO definitions (id, name, schedule, created_at)
		VALUES ($1, 'workstation', '{"cron":"0 1 * * *"}', CURRENT_TIMESTAMP)
	`, definition)
	require.NoError(t, err)
	_, err = raw.Exec(ctx, `
		INSERT INTO definition_members (definition_id, action_set_id, sort_order, added_at)
		VALUES ($1, $3, 10, CURRENT_TIMESTAMP), ($1, $2, 20, CURRENT_TIMESTAMP)
	`, definition, set1, set2)
	require.NoError(t, err)
	return &manifestFixture{
		compiler: manifest.New(st), raw: raw, action1: action1, action2: action2,
		set1: set1, set2: set2, definition: definition,
	}
}

func TestManifestCompiler_ActionCreatesSingleton(t *testing.T) {
	f := newManifestFixture(t)
	got, err := f.compiler.Action(context.Background(), f.action1)
	require.NoError(t, err)
	require.Len(t, got.Occurrences, 1)
	assert.Equal(t, f.action1, got.Provenance.ActionId)
	assert.Empty(t, got.Provenance.ActionSetId)
	assert.True(t, got.Schedule.RunOnAssign)
	assert.Equal(t, f.action1, got.Occurrences[0].Action.Id.Value)
	assert.Equal(t, cadestrov1.OnFailure_ON_FAILURE_CONTINUE, got.Occurrences[0].OnFailure)
}

func TestManifestCompiler_ActionSetFlattensInAuthoredOrder(t *testing.T) {
	f := newManifestFixture(t)
	got, err := f.compiler.ActionSet(context.Background(), f.set1)
	require.NoError(t, err)
	assert.Equal(t, f.set1, got.Provenance.ActionSetId)
	assert.Equal(t, "0 4 * * *", got.Schedule.Cron)
	assert.Equal(t, cadestrov1.OnFailure_ON_FAILURE_STOP, got.DefaultOnFailure)
	require.Len(t, got.Occurrences, 2)
	assert.Equal(t, f.action1, got.Occurrences[0].Action.Id.Value)
	assert.Equal(t, f.action2, got.Occurrences[1].Action.Id.Value)
	assert.Equal(t, cadestrov1.OnFailure_ON_FAILURE_STOP, got.Occurrences[0].OnFailure)
	assert.Equal(t, cadestrov1.OnFailure_ON_FAILURE_STOP, got.Occurrences[1].OnFailure)
	assert.NotEqual(t, got.Occurrences[0].OccurrenceId, got.Occurrences[1].OccurrenceId)
}

func TestManifestCompiler_DefinitionFlattensGlobalOrderAndPolicies(t *testing.T) {
	f := newManifestFixture(t)
	got, err := f.compiler.Definition(context.Background(), f.definition)
	require.NoError(t, err)
	assert.Equal(t, f.definition, got.Provenance.DefinitionId)
	assert.Empty(t, got.Provenance.ActionSetId)
	assert.Equal(t, "0 1 * * *", got.Schedule.Cron)
	assert.False(t, got.Schedule.RunOnAssign)
	require.Len(t, got.Occurrences, 3)
	assert.Equal(t, []string{f.action1, f.action1, f.action2}, []string{
		got.Occurrences[0].Action.Id.Value, got.Occurrences[1].Action.Id.Value,
		got.Occurrences[2].Action.Id.Value,
	}, "definition member order precedes each set's authored action order")
	assert.Equal(t, []cadestrov1.OnFailure{
		cadestrov1.OnFailure_ON_FAILURE_CONTINUE, cadestrov1.OnFailure_ON_FAILURE_STOP,
		cadestrov1.OnFailure_ON_FAILURE_STOP,
	}, []cadestrov1.OnFailure{
		got.Occurrences[0].OnFailure, got.Occurrences[1].OnFailure,
		got.Occurrences[2].OnFailure,
	})
	assert.NotEqual(t, got.Occurrences[0].OccurrenceId, got.Occurrences[1].OccurrenceId,
		"the same authored action reached through two sets remains two occurrences")

	var unchanged bool
	require.NoError(t, f.raw.QueryRow(context.Background(), `
		SELECT schedule = '{"cron":"0 4 * * *"}'
		FROM action_sets WHERE id = $1`, f.set1).Scan(&unchanged))
	assert.True(t, unchanged, "compilation must not rewrite the ActionSet schedule")

	standalone, err := f.compiler.ActionSet(context.Background(), f.set1)
	require.NoError(t, err)
	assert.Equal(t, "0 4 * * *", standalone.Schedule.Cron,
		"independent ActionSet compilation still uses its own schedule")
}

func TestManifestCompiler_DefinitionSkipsEmptyAndDeletedMembers(t *testing.T) {
	f := newManifestFixture(t)
	ctx := context.Background()
	_, err := f.raw.Exec(ctx, `DELETE FROM action_set_members WHERE set_id = $1`, f.set2)
	require.NoError(t, err)
	_, err = f.raw.Exec(ctx, `UPDATE action_sets SET is_deleted = TRUE WHERE id = $1`, f.set1)
	require.NoError(t, err)

	_, err = f.compiler.Definition(ctx, f.definition)
	assert.ErrorIs(t, err, manifest.ErrEmptyManifest,
		"a definition with only empty or deleted members has no executable runbook")
}

func TestManifestCompiler_RejectsMalformedStoredParams(t *testing.T) {
	f := newManifestFixture(t)
	_, err := f.raw.Exec(context.Background(), `
		UPDATE actions SET action_type = $2, params = '{"unexpected":true}' WHERE id = $1
	`, f.action2, int32(cadestrov1.ActionType_ACTION_TYPE_SHELL))
	require.NoError(t, err)
	_, err = f.compiler.Action(context.Background(), f.action2)
	require.Error(t, err)
}

func TestManifestCompiler_EncryptsActionCredentialBeforeDeliveryPersistence(t *testing.T) {
	st, raw := setupSQLite(t)
	ctx := context.Background()
	seedDevice(t, raw)
	atRest, err := crypto.NewEncryptor("0303030303030303030303030303030303030303030303030303030303030303")
	require.NoError(t, err)
	actionID := newID()
	const plaintext = "initial-volume-secret"
	ciphertext, err := atRest.EncryptWithContext(plaintext,
		crypto.RowAAD(actionID, crypto.PurposeActionEncryptionPresharedKey))
	require.NoError(t, err)
	stored, err := actionparams.MarshalActionParams(&cadestrov1.EncryptionAuthoringParams{
		PresharedKey: &ciphertext, RotationIntervalDays: 30, MinWords: 5,
	})
	require.NoError(t, err)
	_, err = raw.Exec(ctx, `
		INSERT INTO actions
			(id, name, action_type, desired_state, params, timeout_seconds, schedule, created_at)
		VALUES ($1, 'encrypted disk', $2, $3, $4, 300, '{}', CURRENT_TIMESTAMP)`,
		actionID, int32(cadestrov1.ActionType_ACTION_TYPE_ENCRYPTION),
		int32(cadestrov1.DesiredState_DESIRED_STATE_PRESENT), stored)
	require.NoError(t, err)

	compiled, err := manifest.New(st).Action(ctx, actionID)
	require.NoError(t, err)
	catalogCiphertext := compiled.Occurrences[0].Action.GetEncryption().PresharedKey
	require.NotEmpty(t, catalogCiphertext)

	outbound := proto.Clone(compiled).(*cadestrov1.Manifest)
	require.NoError(t, manifest.MaterializeSecrets(outbound, atRest))
	assert.Equal(t, []byte(plaintext), outbound.Occurrences[0].Action.GetEncryption().PresharedKey)
	assert.Equal(t, catalogCiphertext, compiled.Occurrences[0].Action.GetEncryption().PresharedKey,
		"materialization must not mutate the durable compiler output")
	tampered := proto.Clone(compiled).(*cadestrov1.Manifest)
	tampered.Occurrences[0].Action.Id.Value = newID()
	assert.Error(t, manifest.MaterializeSecrets(tampered, atRest), "the action id is part of the at-rest AAD")
}

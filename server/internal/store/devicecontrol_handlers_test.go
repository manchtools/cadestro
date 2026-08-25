package store_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/manchtools/cadestro/server/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/manchtools/cadestro/server/internal/agentsync"
	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/connection"
	"github.com/manchtools/cadestro/server/internal/crypto"
	"github.com/manchtools/cadestro/server/internal/devicecontrol"
	"github.com/manchtools/cadestro/server/internal/store"
)

type deviceControlFixture struct {
	t           *testing.T
	store       *store.Store
	raw         *testdb.DB
	handlers    *devicecontrol.Handlers
	now         time.Time
	actorID     string
	deviceID    string
	otherDevice string
	groupID     string
	actionID    string
	set1        string
	set2        string
	definition  string
	atRest      *crypto.Encryptor
}

func newDeviceControlFixture(t *testing.T) *deviceControlFixture {
	return newDeviceControlFixtureWithSender(t, nil)
}

func newDeviceControlFixtureWithSender(t *testing.T, sender func(string, *cadestrov1.ServerMessage) error) *deviceControlFixture {
	t.Helper()
	st, raw := setupSQLite(t)
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	f := &deviceControlFixture{
		t: t, store: st, raw: raw, now: now, actorID: newID(),
		deviceID: seedDevice(t, raw), otherDevice: seedDevice(t, raw),
		groupID: newID(), actionID: newID(),
	}
	atRest, err := crypto.NewEncryptor("0101010101010101010101010101010101010101010101010101010101010101")
	require.NoError(t, err)
	f.atRest = atRest
	_, err = raw.Exec(context.Background(), `
		INSERT INTO device_groups (id, name, created_at) VALUES ($1, 'fanout', $2)`,
		f.groupID, now)
	require.NoError(t, err)
	_, err = raw.Exec(context.Background(), `
		INSERT INTO device_group_members (group_id, device_id, added_at) VALUES
			($1, $2, $4), ($1, $3, $4)`, f.groupID, f.deviceID, f.otherDevice, now)
	require.NoError(t, err)
	_, err = raw.Exec(context.Background(), `
		INSERT INTO actions
			(id, name, action_type, desired_state, params, timeout_seconds, schedule, created_at)
		VALUES ($1, 'catalog shell', $2, $3, $4, 90, $5, $6)`,
		f.actionID, int32(cadestrov1.ActionType_ACTION_TYPE_SHELL),
		int32(cadestrov1.DesiredState_DESIRED_STATE_ABSENT),
		`{"script":"printf catalog","interpreter":"/bin/sh"}`,
		`{"cron":"0 4 * * *"}`, now)
	require.NoError(t, err)
	f.set1, f.set2, f.definition = newID(), newID(), newID()
	_, err = raw.Exec(context.Background(), `
		INSERT INTO action_sets (id, name, schedule, on_failure, created_at) VALUES
			($1, 'first set', '{"cron":"0 2 * * *"}', $3, $5),
			($2, 'second set', '{"runOnAssign":true}', $4, $5)`,
		f.set1, f.set2,
		int32(cadestrov1.OnFailure_ON_FAILURE_STOP), int32(cadestrov1.OnFailure_ON_FAILURE_CONTINUE),
		now)
	require.NoError(t, err)
	_, err = raw.Exec(context.Background(), `
		INSERT INTO action_set_members (set_id, action_id, sort_order, added_at) VALUES
			($1, $3, 0, $4), ($2, $3, 0, $4)`, f.set1, f.set2, f.actionID, now)
	require.NoError(t, err)
	_, err = raw.Exec(context.Background(), `
		INSERT INTO definitions (id, name, schedule, created_at)
			VALUES ($1, 'two sets', '{"cron":"0 1 * * *"}', $2)`, f.definition, now)
	require.NoError(t, err)
	_, err = raw.Exec(context.Background(), `
		INSERT INTO definition_members (definition_id, action_set_id, sort_order, added_at) VALUES
			($1, $2, 0, $4), ($1, $3, 1, $4)`, f.definition, f.set1, f.set2, now)
	require.NoError(t, err)
	f.handlers = devicecontrol.NewHandlers(devicecontrol.HandlersConfig{
		Store:  st,
		Sender: sender,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:    func() time.Time { return now },
	})
	return f
}

func (f *deviceControlFixture) actor(perms ...string) context.Context {
	return auth.WithUser(context.Background(), &auth.UserContext{
		ID: f.actorID, Kind: auth.PrincipalUser, Permissions: perms,
	})
}

func (f *deviceControlFixture) assign(sourceType, sourceID, targetType, targetID string, mode cadestrov1.AssignmentMode) {
	f.t.Helper()
	_, err := f.raw.Exec(context.Background(), `
		INSERT INTO assignments
			(id, source_type, source_id, target_type, target_id, mode, created_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		newID(), sourceType, sourceID, targetType, targetID, int32(mode), f.now, f.actorID)
	require.NoError(f.t, err)
}

func TestAgentSync_PullsAssignedDefinitionAsOneOrderedPolicy(t *testing.T) {
	f := newDeviceControlFixture(t)
	f.assign("definition", f.definition, "device", f.deviceID, cadestrov1.AssignmentMode_ASSIGNMENT_MODE_REQUIRED)

	manager := connection.NewManager()
	agent := manager.Register(context.Background(), f.deviceID, "device", "v1", nil)
	t.Cleanup(agent.Close)
	other := manager.Register(context.Background(), f.otherDevice, "other", "v1", nil)
	t.Cleanup(other.Close)
	syncer := agentsync.New(agentsync.Config{
		Store: f.store, Manager: manager,
		Assignments: f.handlers,
		Now:         func() time.Time { return f.now },
		AtRest:      f.atRest,
	})
	otherState, err := syncer.Sync(context.Background(), f.otherDevice)
	require.NoError(t, err)
	require.NotNil(t, otherState.DesiredPolicy)
	require.Empty(t, otherState.DesiredPolicy.Manifests, "one device cannot pull another device's assignment")

	state, err := syncer.Sync(context.Background(), f.deviceID)
	require.NoError(t, err)
	require.NotNil(t, state.DesiredPolicy)
	require.Len(t, state.DesiredPolicy.Manifests, 1, "a Definition is one globally ordered runbook")
	require.Len(t, state.DesiredPolicy.Manifests[0].Occurrences, 2)
	require.Equal(t, cadestrov1.OnFailure_ON_FAILURE_STOP, state.DesiredPolicy.Manifests[0].Occurrences[0].OnFailure)
	require.Equal(t, cadestrov1.OnFailure_ON_FAILURE_CONTINUE, state.DesiredPolicy.Manifests[0].Occurrences[1].OnFailure)
	firstRevision := state.DesiredPolicy.Revision
	firstManifestID := state.DesiredPolicy.Manifests[0].GetManifestId().GetValue()

	repeated, err := syncer.Sync(context.Background(), f.deviceID)
	require.NoError(t, err)
	assert.Equal(t, firstRevision, repeated.DesiredPolicy.Revision)
	assert.Equal(t, firstManifestID, repeated.DesiredPolicy.Manifests[0].GetManifestId().GetValue())

	_, err = f.raw.Exec(context.Background(), `UPDATE actions SET params = $1, params_canonical = $1 WHERE id = $2`,
		`{"interpreter":"/bin/sh","script":"printf changed"}`, f.actionID)
	require.NoError(t, err)
	changed, err := syncer.Sync(context.Background(), f.deviceID)
	require.NoError(t, err)
	assert.NotEqual(t, firstRevision, changed.DesiredPolicy.Revision)
	assert.NotEqual(t, firstManifestID, changed.DesiredPolicy.Manifests[0].GetManifestId().GetValue())

	_, err = f.raw.Exec(context.Background(), `UPDATE definition_members
		SET sort_order = CASE WHEN sort_order = 0 THEN 1 ELSE 0 END
		WHERE definition_id = $1`, f.definition)
	require.NoError(t, err)
	reordered, err := syncer.Sync(context.Background(), f.deviceID)
	require.NoError(t, err)
	assert.NotEqual(t, changed.DesiredPolicy.Revision, reordered.DesiredPolicy.Revision)
	assert.NotEqual(t, changed.DesiredPolicy.Manifests[0].GetManifestId().GetValue(), reordered.DesiredPolicy.Manifests[0].GetManifestId().GetValue())

	_, err = f.raw.Exec(context.Background(), `DELETE FROM assignments WHERE source_id = $1`, f.definition)
	require.NoError(t, err)
	f.assign("definition", f.definition, "device", f.deviceID, cadestrov1.AssignmentMode_ASSIGNMENT_MODE_UNINSTALL)
	forced, err := syncer.Sync(context.Background(), f.deviceID)
	require.NoError(t, err)
	assert.NotEqual(t, reordered.DesiredPolicy.Revision, forced.DesiredPolicy.Revision)
	assert.NotEqual(t, reordered.DesiredPolicy.Manifests[0].GetManifestId().GetValue(), forced.DesiredPolicy.Manifests[0].GetManifestId().GetValue())

	_, err = f.raw.Exec(context.Background(), `DELETE FROM assignments WHERE source_id IN ($1, $2)`, f.definition, f.actionID)
	require.NoError(t, err)
	state, err = syncer.Sync(context.Background(), f.deviceID)
	require.NoError(t, err)
	require.Empty(t, state.DesiredPolicy.Manifests, "unassignment must be observable on the next pull")
}

func TestDeviceControlHandlers_LiveOperationsUseTypedStreamWithoutPolicyWork(t *testing.T) {
	var f *deviceControlFixture
	sender := func(deviceID string, message *cadestrov1.ServerMessage) error {
		assert.Equal(t, f.deviceID, deviceID)
		switch message.GetPayload().(type) {
		case *cadestrov1.ServerMessage_SyncDevice:
			go func() {
				_ = f.handlers.CompleteSyncDevice(context.Background(), deviceID, message.GetId().GetValue(), &cadestrov1.SyncDeviceResult{Success: true})
			}()
		case *cadestrov1.ServerMessage_RebootDevice:
			go func() {
				_ = f.handlers.CompleteRebootDevice(context.Background(), deviceID, message.GetId().GetValue(), &cadestrov1.RebootDeviceResult{Success: true})
			}()
		default:
			t.Fatalf("unexpected live payload %T", message.GetPayload())
		}
		return nil
	}
	f = newDeviceControlFixtureWithSender(t, sender)
	_, err := f.handlers.SyncDevice(f.actor("SyncDevice"), connect.NewRequest(&cadestrov1.SyncDeviceRequest{DeviceId: &cadestrov1.DeviceId{Value: f.deviceID}}))
	require.NoError(t, err)
	_, err = f.handlers.RebootDevice(f.actor("RebootDevice"), connect.NewRequest(&cadestrov1.RebootDeviceRequest{DeviceId: &cadestrov1.DeviceId{Value: f.deviceID}}))
	require.NoError(t, err)
}

func TestDeviceControlHandlers_LiveOperationRequiresConnection(t *testing.T) {
	f := newDeviceControlFixture(t)
	_, err := f.handlers.SyncDevice(f.actor("SyncDevice"),
		connect.NewRequest(&cadestrov1.SyncDeviceRequest{DeviceId: &cadestrov1.DeviceId{Value: f.deviceID}}))
	assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))

	operation, err := latestOperationFor(t, f.store, f.raw, cadestrov1connect.ControlServiceSyncDeviceProcedure)
	require.NoError(t, err)
	effects, err := f.store.ListAuditEffects(context.Background(), operation.OperationID)
	require.NoError(t, err)
	require.Len(t, effects, 2)
	assert.Equal(t, string(store.EffectApplied), effects[0].Outcome)
	assert.Equal(t, string(store.EffectFailed), effects[1].Outcome)
}

func TestDeviceControlHandlers_MountsExactInitialSurface(t *testing.T) {
	f := newDeviceControlFixture(t)
	assert.ElementsMatch(t, []string{
		cadestrov1connect.ControlServiceSyncDeviceProcedure,
		cadestrov1connect.ControlServiceRebootDeviceProcedure,
	}, f.handlers.MountLiveControl(http.NewServeMux()))
	assert.ElementsMatch(t, f.handlers.MountLiveControl(http.NewServeMux()), devicecontrol.LiveControlProcedures())
}

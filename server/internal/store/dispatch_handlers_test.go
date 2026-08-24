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
	"google.golang.org/protobuf/encoding/protojson"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/manchtools/cadestro/server/internal/agentsync"
	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/connection"
	"github.com/manchtools/cadestro/server/internal/crypto"
	"github.com/manchtools/cadestro/server/internal/dispatch"
	"github.com/manchtools/cadestro/server/internal/store"
)

type dispatchHandlerFixture struct {
	t           *testing.T
	store       *store.Store
	raw         *testdb.DB
	handlers    *dispatch.Handlers
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

func newDispatchHandlerFixture(t *testing.T) *dispatchHandlerFixture {
	return newDispatchHandlerFixtureWithSender(t, nil)
}

func newDispatchHandlerFixtureWithSender(t *testing.T, sender func(string, *cadestrov1.ServerMessage) error) *dispatchHandlerFixture {
	t.Helper()
	st, raw := setupSQLite(t)
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	f := &dispatchHandlerFixture{
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
	f.handlers = dispatch.NewHandlers(dispatch.HandlersConfig{
		Store:  st,
		Sender: sender,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:    func() time.Time { return now },
	})
	return f
}

func (f *dispatchHandlerFixture) actor(perms ...string) context.Context {
	return auth.WithUser(context.Background(), &auth.UserContext{
		ID: f.actorID, Kind: auth.PrincipalUser, Permissions: perms,
	})
}

func (f *dispatchHandlerFixture) manifest(deliveryID string) *cadestrov1.Manifest {
	f.t.Helper()
	row, err := f.store.GetDelivery(context.Background(), deliveryID)
	require.NoError(f.t, err)
	var result cadestrov1.Manifest
	require.NoError(f.t, protojson.Unmarshal(row.Manifest, &result))
	return &result
}

func (f *dispatchHandlerFixture) deliveryIDs() []string {
	f.t.Helper()
	rows, err := f.raw.Query(context.Background(), `SELECT delivery_id FROM deliveries ORDER BY rowid`)
	require.NoError(f.t, err)
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		require.NoError(f.t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(f.t, rows.Err())
	return ids
}

func (f *dispatchHandlerFixture) assign(sourceType, sourceID, targetType, targetID string, mode cadestrov1.AssignmentMode) {
	f.t.Helper()
	_, err := f.raw.Exec(context.Background(), `
		INSERT INTO assignments
			(id, source_type, source_id, target_type, target_id, mode, created_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		newID(), sourceType, sourceID, targetType, targetID, int32(mode), f.now, f.actorID)
	require.NoError(f.t, err)
}

func TestAgentSync_PullsAssignedDefinitionAsOneOrderedPolicy(t *testing.T) {
	f := newDispatchHandlerFixture(t)
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
	rows, err := f.store.ListDueDeviceDeliveries(context.Background(), f.deviceID, f.now, 10)
	require.NoError(t, err)
	require.Empty(t, rows, "policy must not be materialized before an agent pulls it")
	otherState, err := syncer.Sync(context.Background(), f.otherDevice)
	require.NoError(t, err)
	require.NotNil(t, otherState.DesiredPolicy)
	require.Empty(t, otherState.DesiredPolicy.Manifests, "one device cannot pull another device's assignment")

	state, err := syncer.Sync(context.Background(), f.deviceID)
	require.NoError(t, err)
	require.Empty(t, state.Deliveries, "assignment policy is returned only in the explicit Sync snapshot")
	require.NotNil(t, state.DesiredPolicy)
	require.Len(t, state.DesiredPolicy.Manifests, 1, "a Definition is one globally ordered runbook")
	require.Len(t, state.DesiredPolicy.Manifests[0].Occurrences, 2)
	require.Equal(t, cadestrov1.OnFailure_ON_FAILURE_STOP, state.DesiredPolicy.Manifests[0].Occurrences[0].OnFailure)
	require.Equal(t, cadestrov1.OnFailure_ON_FAILURE_CONTINUE, state.DesiredPolicy.Manifests[0].Occurrences[1].OnFailure)
	firstRevision := state.DesiredPolicy.Revision
	firstManifestID := state.DesiredPolicy.Manifests[0].ManifestId
	// Repeated compilation must keep the authored revision and manifest
	// identity stable even when the device-facing payload is rebuilt.
	repeated, err := syncer.Sync(context.Background(), f.deviceID)
	require.NoError(t, err)
	assert.Equal(t, firstRevision, repeated.DesiredPolicy.Revision)
	assert.Equal(t, firstManifestID, repeated.DesiredPolicy.Manifests[0].ManifestId)

	// Semantic authoring changes feed the stable identity seed rather than
	// being hidden by outbound secret materialization.
	_, err = f.raw.Exec(context.Background(), `UPDATE actions SET params = $1, params_canonical = $1 WHERE id = $2`,
		`{"interpreter":"/bin/sh","script":"printf changed"}`, f.actionID)
	require.NoError(t, err)
	changed, err := syncer.Sync(context.Background(), f.deviceID)
	require.NoError(t, err)
	assert.NotEqual(t, firstRevision, changed.DesiredPolicy.Revision)
	assert.NotEqual(t, firstManifestID, changed.DesiredPolicy.Manifests[0].ManifestId)

	// Authored container order is semantic even when the contained action is
	// currently the same, so moving the sets changes the policy identity.
	_, err = f.raw.Exec(context.Background(), `UPDATE definition_members
		SET sort_order = CASE WHEN sort_order = 0 THEN 1 ELSE 0 END
		WHERE definition_id = $1`, f.definition)
	require.NoError(t, err)
	reordered, err := syncer.Sync(context.Background(), f.deviceID)
	require.NoError(t, err)
	assert.NotEqual(t, changed.DesiredPolicy.Revision, reordered.DesiredPolicy.Revision)
	assert.NotEqual(t, changed.DesiredPolicy.Manifests[0].ManifestId, reordered.DesiredPolicy.Manifests[0].ManifestId)

	// Assignment mode is also semantic: force-absent must not reuse the
	// required-mode identity when the source is toggled.
	_, err = f.raw.Exec(context.Background(), `DELETE FROM assignments WHERE source_id = $1`, f.definition)
	require.NoError(t, err)
	f.assign("definition", f.definition, "device", f.deviceID, cadestrov1.AssignmentMode_ASSIGNMENT_MODE_UNINSTALL)
	forced, err := syncer.Sync(context.Background(), f.deviceID)
	require.NoError(t, err)
	assert.NotEqual(t, reordered.DesiredPolicy.Revision, forced.DesiredPolicy.Revision)
	assert.NotEqual(t, reordered.DesiredPolicy.Manifests[0].ManifestId, forced.DesiredPolicy.Manifests[0].ManifestId)

	rows, err = f.store.ListDueDeviceDeliveries(context.Background(), f.deviceID, f.now, 10)
	require.NoError(t, err)
	require.Empty(t, rows, "Sync policy is not a server delivery")

	_, err = f.raw.Exec(context.Background(), `DELETE FROM assignments WHERE source_id IN ($1, $2)`, f.definition, f.actionID)
	require.NoError(t, err)
	state, err = syncer.Sync(context.Background(), f.deviceID)
	require.NoError(t, err)
	require.Empty(t, state.DesiredPolicy.Manifests, "unassignment must be observable on the next pull")
	rows, err = f.store.ListDueDeviceDeliveries(context.Background(), f.deviceID, f.now, 10)
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestDispatchHandlers_CatalogAndInlineActionsUseDurableOneShotManifests(t *testing.T) {
	f := newDispatchHandlerFixture(t)
	ctx := f.actor("DispatchAction")

	catalog, err := f.handlers.DispatchAction(ctx, connect.NewRequest(&cadestrov1.DispatchActionRequest{
		DeviceId: f.deviceID,
		ActionSource: &cadestrov1.DispatchActionRequest_ActionId{
			ActionId: f.actionID,
		},
	}))
	require.NoError(t, err)
	ids := f.deliveryIDs()
	require.Len(t, ids, 1)
	assert.Equal(t, f.actionID, catalog.Msg.Execution.ActionId)
	assert.Equal(t, cadestrov1.ExecutionStatus_EXECUTION_STATUS_PENDING, catalog.Msg.Execution.Status)
	catalogManifest := f.manifest(ids[0])
	require.NotNil(t, catalogManifest.Schedule)
	assert.Empty(t, catalogManifest.Schedule.Cron,
		"an explicit dispatch runs once instead of adopting the authored manifest schedule")
	assert.Zero(t, catalogManifest.Schedule.IntervalHours)
	assert.False(t, catalogManifest.Schedule.RunOnAssign)
	assert.False(t, catalogManifest.Schedule.SkipIfUnchanged)
	require.Len(t, catalogManifest.Occurrences, 1)
	assert.Equal(t, "0 4 * * *", catalogManifest.Occurrences[0].Action.Schedule.Cron,
		"the nested Action keeps its authoring/display schedule")

	inlineID := newID()
	inline, err := f.handlers.DispatchAction(ctx, connect.NewRequest(&cadestrov1.DispatchActionRequest{
		DeviceId: f.deviceID,
		ActionSource: &cadestrov1.DispatchActionRequest_InlineAction{InlineAction: &cadestrov1.Action{
			Id: &cadestrov1.ActionId{Value: inlineID}, Type: cadestrov1.ActionType_ACTION_TYPE_SHELL,
			DesiredState: cadestrov1.DesiredState_DESIRED_STATE_PRESENT,
			Params:       &cadestrov1.Action_Shell{Shell: &cadestrov1.ShellParams{Script: "printf inline"}},
		}},
	}))
	require.NoError(t, err)
	ids = f.deliveryIDs()
	require.Len(t, ids, 2)
	assert.Empty(t, inline.Msg.Execution.ActionId, "an inline action is not a catalog reference")
	inlineManifest := f.manifest(ids[1])
	assert.Equal(t, inlineID, inlineManifest.Provenance.ActionId)
	assert.Equal(t, "printf inline", inlineManifest.Occurrences[0].Action.GetShell().Script)
	assert.Equal(t, int32(300), inlineManifest.Occurrences[0].Action.TimeoutSeconds)

	operation, err := latestOperationFor(t, f.store, f.raw,
		cadestrov1connect.ControlServiceDispatchActionProcedure)
	require.NoError(t, err)
	effects, err := f.store.ListAuditEffects(context.Background(), operation.OperationID)
	require.NoError(t, err)
	require.Len(t, effects, 2)
	assert.ElementsMatch(t, []string{"delivery", "execution"},
		[]string{effects[0].ResourceType, effects[1].ResourceType})
}

func TestDispatchHandlers_LiveOperationsUseTypedStreamWithoutDelivery(t *testing.T) {
	var f *dispatchHandlerFixture
	sender := func(deviceID string, message *cadestrov1.ServerMessage) error {
		assert.Equal(t, f.deviceID, deviceID)
		switch message.GetPayload().(type) {
		case *cadestrov1.ServerMessage_SyncDevice:
			go func() {
				_ = f.handlers.CompleteSyncDevice(context.Background(), deviceID, message.Id, &cadestrov1.SyncDeviceResult{Success: true})
			}()
		case *cadestrov1.ServerMessage_RebootDevice:
			go func() {
				_ = f.handlers.CompleteRebootDevice(context.Background(), deviceID, message.Id, &cadestrov1.RebootDeviceResult{Success: true})
			}()
		default:
			t.Fatalf("unexpected live payload %T", message.GetPayload())
		}
		return nil
	}
	f = newDispatchHandlerFixtureWithSender(t, sender)
	_, err := f.handlers.SyncDevice(f.actor("SyncDevice"), connect.NewRequest(&cadestrov1.SyncDeviceRequest{DeviceId: f.deviceID}))
	require.NoError(t, err)
	_, err = f.handlers.RebootDevice(f.actor("RebootDevice"), connect.NewRequest(&cadestrov1.RebootDeviceRequest{DeviceId: f.deviceID}))
	require.NoError(t, err)
	assert.Empty(t, f.deliveryIDs())

	_, err = f.handlers.DispatchAction(f.actor("DispatchAction"), connect.NewRequest(&cadestrov1.DispatchActionRequest{
		DeviceId: f.deviceID,
		ActionSource: &cadestrov1.DispatchActionRequest_InlineAction{InlineAction: &cadestrov1.Action{
			Id: &cadestrov1.ActionId{Value: newID()}, Type: cadestrov1.ActionType_ACTION_TYPE_USER,
			Params: &cadestrov1.Action_Shell{Shell: &cadestrov1.ShellParams{Script: "mismatch"}},
		}},
	}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestDispatchHandlers_LiveOperationRequiresConnection(t *testing.T) {
	f := newDispatchHandlerFixture(t)
	_, err := f.handlers.SyncDevice(f.actor("SyncDevice"),
		connect.NewRequest(&cadestrov1.SyncDeviceRequest{DeviceId: f.deviceID}))
	assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))

	operation, err := latestOperationFor(t, f.store, f.raw, cadestrov1connect.ControlServiceSyncDeviceProcedure)
	require.NoError(t, err)
	effects, err := f.store.ListAuditEffects(context.Background(), operation.OperationID)
	require.NoError(t, err)
	require.Len(t, effects, 2)
	assert.Equal(t, string(store.EffectApplied), effects[0].Outcome)
	assert.Equal(t, string(store.EffectFailed), effects[1].Outcome)
}

func TestDispatchHandlers_ActionSetAndDefinitionPreserveComposition(t *testing.T) {
	f := newDispatchHandlerFixture(t)
	setResponse, err := f.handlers.DispatchActionSet(f.actor("DispatchActionSet"),
		connect.NewRequest(&cadestrov1.DispatchActionSetRequest{
			DeviceId: f.deviceID, ActionSetId: f.set1,
		}))
	require.NoError(t, err)
	require.Len(t, setResponse.Msg.Executions, 1)
	ids := f.deliveryIDs()
	require.Len(t, ids, 1)
	setManifest := f.manifest(ids[0])
	assert.Equal(t, f.set1, setManifest.Provenance.ActionSetId)
	assert.Empty(t, setManifest.Schedule.Cron)
	assert.Equal(t, cadestrov1.OnFailure_ON_FAILURE_STOP, setManifest.DefaultOnFailure)
	assert.Equal(t, cadestrov1.OnFailure_ON_FAILURE_STOP, setManifest.Occurrences[0].OnFailure)

	definitionResponse, err := f.handlers.DispatchDefinition(f.actor("DispatchDefinition"),
		connect.NewRequest(&cadestrov1.DispatchDefinitionRequest{
			DeviceId: f.deviceID, DefinitionId: f.definition,
		}))
	require.NoError(t, err)
	require.Len(t, definitionResponse.Msg.Executions, 2, "one execution is created per ordered occurrence")
	ids = f.deliveryIDs()
	require.Len(t, ids, 2)
	definitionManifest := f.manifest(ids[1])
	assert.Equal(t, f.definition, definitionManifest.Provenance.DefinitionId)
	assert.Empty(t, definitionManifest.Provenance.ActionSetId)
	require.Len(t, definitionManifest.Occurrences, 2)
	assert.Equal(t, []string{f.actionID, f.actionID}, []string{
		definitionManifest.Occurrences[0].Action.Id.Value,
		definitionManifest.Occurrences[1].Action.Id.Value,
	}, "definition set order is preserved")
	assert.Equal(t, []cadestrov1.OnFailure{
		cadestrov1.OnFailure_ON_FAILURE_STOP, cadestrov1.OnFailure_ON_FAILURE_CONTINUE,
	}, []cadestrov1.OnFailure{
		definitionManifest.Occurrences[0].OnFailure,
		definitionManifest.Occurrences[1].OnFailure,
	})
	operation, err := latestOperationFor(t, f.store, f.raw,
		cadestrov1connect.ControlServiceDispatchDefinitionProcedure)
	require.NoError(t, err)
	effects, err := f.store.ListAuditEffects(context.Background(), operation.OperationID)
	require.NoError(t, err)
	require.Len(t, effects, 3, "one delivery and two occurrence executions share one initiating operation")
	row, err := f.store.GetDelivery(context.Background(), ids[1])
	require.NoError(t, err)
	require.NotNil(t, row.OperationID)
	assert.Equal(t, operation.OperationID, *row.OperationID)

	var set1Schedule, set2Schedule string
	require.NoError(t, f.raw.QueryRow(context.Background(), `
		SELECT (SELECT schedule FROM action_sets WHERE id = $1),
		       (SELECT schedule FROM action_sets WHERE id = $2)`, f.set1, f.set2).
		Scan(&set1Schedule, &set2Schedule))
	assert.JSONEq(t, `{"cron":"0 2 * * *"}`, set1Schedule)
	assert.JSONEq(t, `{"runOnAssign":true}`, set2Schedule)
}

func TestDispatchHandlers_ExplicitDispatchMarksEveryManifestOneShot(t *testing.T) {
	f := newDispatchHandlerFixture(t)

	_, err := f.handlers.DispatchAction(f.actor("DispatchAction"),
		connect.NewRequest(&cadestrov1.DispatchActionRequest{
			DeviceId:     f.deviceID,
			ActionSource: &cadestrov1.DispatchActionRequest_ActionId{ActionId: f.actionID},
		}))
	require.NoError(t, err)
	ids := f.deliveryIDs()
	require.Len(t, ids, 1)
	catalog := f.manifest(ids[0])
	assert.True(t, catalog.GetOneShot(),
		"an explicitly dispatched catalog action executes exactly once")
	assert.Empty(t, catalog.Schedule.Cron)

	_, err = f.handlers.DispatchActionSet(f.actor("DispatchActionSet"),
		connect.NewRequest(&cadestrov1.DispatchActionSetRequest{
			DeviceId: f.deviceID, ActionSetId: f.set1,
		}))
	require.NoError(t, err)
	ids = f.deliveryIDs()
	require.Len(t, ids, 2)
	set := f.manifest(ids[1])
	assert.True(t, set.GetOneShot(),
		"an explicitly dispatched ActionSet executes exactly once")
	assert.Empty(t, set.Schedule.Cron)

	_, err = f.handlers.DispatchDefinition(f.actor("DispatchDefinition"),
		connect.NewRequest(&cadestrov1.DispatchDefinitionRequest{
			DeviceId: f.deviceID, DefinitionId: f.definition,
		}))
	require.NoError(t, err)
	ids = f.deliveryIDs()
	require.Len(t, ids, 3)
	for _, deliveryID := range ids[2:] {
		compiled := f.manifest(deliveryID)
		assert.True(t, compiled.GetOneShot(),
			"an explicitly dispatched Definition runbook executes exactly once")
		assert.Empty(t, compiled.Schedule.Cron)
	}
}

func TestDispatchHandlers_MultiDeviceAndGroupFanoutAreSingleOperations(t *testing.T) {
	f := newDispatchHandlerFixture(t)
	multiple, err := f.handlers.DispatchToMultiple(f.actor("DispatchToMultiple"),
		connect.NewRequest(&cadestrov1.DispatchToMultipleRequest{
			DeviceIds: []string{f.deviceID, f.otherDevice},
			ActionSource: &cadestrov1.DispatchToMultipleRequest_ActionId{
				ActionId: f.actionID,
			},
		}))
	require.NoError(t, err)
	require.Len(t, multiple.Msg.Executions, 2)
	assert.Equal(t, []string{f.deviceID, f.otherDevice}, []string{
		multiple.Msg.Executions[0].DeviceId, multiple.Msg.Executions[1].DeviceId,
	})
	ids := f.deliveryIDs()
	require.Len(t, ids, 2)
	firstManifest, secondManifest := f.manifest(ids[0]), f.manifest(ids[1])
	assert.NotEqual(t, firstManifest.ManifestId, secondManifest.ManifestId)
	assert.NotEqual(t, firstManifest.Occurrences[0].OccurrenceId, secondManifest.Occurrences[0].OccurrenceId)
	operation, err := latestOperationFor(t, f.store, f.raw,
		cadestrov1connect.ControlServiceDispatchToMultipleProcedure)
	require.NoError(t, err)
	effects, err := f.store.ListAuditEffects(context.Background(), operation.OperationID)
	require.NoError(t, err)
	require.Len(t, effects, 4)

	group, err := f.handlers.DispatchToGroup(f.actor("DispatchToGroup"),
		connect.NewRequest(&cadestrov1.DispatchToGroupRequest{
			GroupId: f.groupID,
			ActionSource: &cadestrov1.DispatchToGroupRequest_DefinitionId{
				DefinitionId: f.definition,
			},
		}))
	require.NoError(t, err)
	require.Len(t, group.Msg.Executions, 4, "one runbook is copied to each of two devices")
	require.Len(t, f.deliveryIDs(), 4)
	counts := map[string]int{}
	for _, execution := range group.Msg.Executions {
		counts[execution.DeviceId]++
	}
	assert.Equal(t, map[string]int{f.deviceID: 2, f.otherDevice: 2}, counts)
	operation, err = latestOperationFor(t, f.store, f.raw,
		cadestrov1connect.ControlServiceDispatchToGroupProcedure)
	require.NoError(t, err)
	effects, err = f.store.ListAuditEffects(context.Background(), operation.OperationID)
	require.NoError(t, err)
	require.Len(t, effects, 6, "two deliveries and four executions share the group operation")
}

func TestDispatchHandlers_RefuseUnauthorizedAndMissingTargetsWithoutWork(t *testing.T) {
	f := newDispatchHandlerFixture(t)
	request := connect.NewRequest(&cadestrov1.DispatchActionRequest{
		DeviceId: f.deviceID,
		ActionSource: &cadestrov1.DispatchActionRequest_ActionId{
			ActionId: f.actionID,
		},
	})
	_, err := f.handlers.DispatchAction(f.actor(), request)
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))

	request.Msg.DeviceId = newID()
	_, err = f.handlers.DispatchAction(f.actor("DispatchAction"), request)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	_, err = f.handlers.DispatchToMultiple(f.actor("DispatchToMultiple"),
		connect.NewRequest(&cadestrov1.DispatchToMultipleRequest{
			DeviceIds: []string{f.deviceID, f.deviceID},
			ActionSource: &cadestrov1.DispatchToMultipleRequest_ActionId{
				ActionId: f.actionID,
			},
		}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	emptyGroup := newID()
	_, err = f.raw.Exec(context.Background(), `
		INSERT INTO device_groups (id, name, created_at) VALUES ($1, 'empty', $2)`, emptyGroup, f.now)
	require.NoError(t, err)
	_, err = f.handlers.DispatchToGroup(f.actor("DispatchToGroup"),
		connect.NewRequest(&cadestrov1.DispatchToGroupRequest{GroupId: emptyGroup}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err),
		"an empty group must not make a missing action source look successful")
	assert.Empty(t, f.deliveryIDs())
}

func TestDispatchHandlers_MountsExactInitialSurface(t *testing.T) {
	f := newDispatchHandlerFixture(t)
	assert.ElementsMatch(t, []string{
		cadestrov1connect.ControlServiceDispatchActionProcedure,
		cadestrov1connect.ControlServiceSyncDeviceProcedure,
		cadestrov1connect.ControlServiceRebootDeviceProcedure,
		cadestrov1connect.ControlServiceDispatchActionSetProcedure,
		cadestrov1connect.ControlServiceDispatchDefinitionProcedure,
		cadestrov1connect.ControlServiceDispatchToMultipleProcedure,
		cadestrov1connect.ControlServiceDispatchToGroupProcedure,
	}, f.handlers.MountActions(http.NewServeMux()))
	assert.ElementsMatch(t, f.handlers.MountActions(http.NewServeMux()), dispatch.MutationProcedures())
}

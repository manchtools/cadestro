package store_test

import (
	"context"
	"net/http"
	"sort"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/authoring"
)

func definitionCreate(name string) *cadestrov1.CreateDefinitionRequest {
	return &cadestrov1.CreateDefinitionRequest{
		Name: name, Schedule: &cadestrov1.ActionSchedule{Cron: "0 1 * * *"},
	}
}

func createDefinitionSet(t *testing.T, state *authoring.Service, name string) string {
	t.Helper()
	op := actionOperation()
	set, err := state.CreateActionSet(context.Background(), op, authoring.CreateActionSetParams{
		Name: name, CreatedBy: op.ActorID, Schedule: &cadestrov1.ActionSchedule{Cron: "0 4 * * *"},
	})
	require.NoError(t, err)
	return set.ID
}

func TestDefinitionHandlers_ValidateBeforeAuthentication(t *testing.T) {
	f := newActionHandlerFixture(t)
	_, err := validated(f.handlers.GetDefinition)(context.Background(), connect.NewRequest(&cadestrov1.GetDefinitionRequest{Id: &cadestrov1.DefinitionId{Value: "bad"}}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	_, err = validated(f.handlers.GetDefinition)(context.Background(), connect.NewRequest(&cadestrov1.GetDefinitionRequest{Id: &cadestrov1.DefinitionId{Value: newID()}}))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

func TestDefinitionHandlers_CRUDMembershipAndAudit(t *testing.T) {
	f := newActionHandlerFixture(t)
	ctx := f.actor(
		"CreateDefinition", "GetDefinition", "ListDefinitions", "RenameDefinition",
		"UpdateDefinitionDescription", "UpdateDefinitionSchedule", "DeleteDefinition",
		"AddActionSetToDefinition", "RemoveActionSetFromDefinition", "ReorderActionSetInDefinition",
	)
	state := authoring.New(authoring.Config{Store: f.store, Now: func() time.Time { return f.now }})
	set1 := createDefinitionSet(t, state, "one")
	set2 := createDefinitionSet(t, state, "two")

	created, err := f.handlers.CreateDefinition(ctx, connect.NewRequest(definitionCreate("baseline")))
	require.NoError(t, err)
	definitionID := created.Msg.Definition.GetId().GetValue()
	assert.Equal(t, int32(0), created.Msg.Definition.MemberCount)
	assert.Equal(t, "0 1 * * *", created.Msg.Definition.Schedule.Cron)

	added, err := f.handlers.AddActionSetToDefinition(ctx, connect.NewRequest(&cadestrov1.AddActionSetToDefinitionRequest{
		DefinitionId: &cadestrov1.DefinitionId{Value: definitionID}, ActionSetId: &cadestrov1.ActionSetId{Value: set2}, SortOrder: 20,
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(1), added.Msg.Definition.MemberCount)
	_, err = f.handlers.AddActionSetToDefinition(ctx, connect.NewRequest(&cadestrov1.AddActionSetToDefinitionRequest{
		DefinitionId: &cadestrov1.DefinitionId{Value: definitionID}, ActionSetId: &cadestrov1.ActionSetId{Value: set1}, SortOrder: 10,
	}))
	require.NoError(t, err)
	_, err = f.handlers.AddActionSetToDefinition(ctx, connect.NewRequest(&cadestrov1.AddActionSetToDefinitionRequest{
		DefinitionId: &cadestrov1.DefinitionId{Value: definitionID}, ActionSetId: &cadestrov1.ActionSetId{Value: set1}, SortOrder: 30,
	}))
	assert.Equal(t, connect.CodeAlreadyExists, connect.CodeOf(err))

	got, err := f.handlers.GetDefinition(ctx, connect.NewRequest(&cadestrov1.GetDefinitionRequest{Id: &cadestrov1.DefinitionId{Value: definitionID}}))
	require.NoError(t, err)
	require.Len(t, got.Msg.Members, 2)
	assert.Equal(t, set1, got.Msg.Members[0].GetActionSetId().GetValue())
	assert.Equal(t, "one", got.Msg.Members[0].ActionSetName)
	assert.Equal(t, set2, got.Msg.Members[1].GetActionSetId().GetValue())
	assert.Equal(t, int32(2), got.Msg.Definition.MemberCount)

	renamed, err := f.handlers.RenameDefinition(ctx, connect.NewRequest(&cadestrov1.RenameDefinitionRequest{Id: &cadestrov1.DefinitionId{Value: definitionID}, Name: "renamed"}))
	require.NoError(t, err)
	assert.Equal(t, "renamed", renamed.Msg.Definition.Name)
	described, err := f.handlers.UpdateDefinitionDescription(ctx, connect.NewRequest(&cadestrov1.UpdateDefinitionDescriptionRequest{
		Id: &cadestrov1.DefinitionId{Value: definitionID}, Description: "direct state",
	}))
	require.NoError(t, err)
	assert.Equal(t, "direct state", described.Msg.Definition.Description)
	scheduled, err := f.handlers.UpdateDefinitionSchedule(ctx, connect.NewRequest(&cadestrov1.UpdateDefinitionScheduleRequest{
		Id: &cadestrov1.DefinitionId{Value: definitionID}, Schedule: &cadestrov1.ActionSchedule{IntervalHours: 12},
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(12), scheduled.Msg.Definition.Schedule.IntervalHours)

	reordered, err := f.handlers.ReorderActionSetInDefinition(ctx, connect.NewRequest(&cadestrov1.ReorderActionSetInDefinitionRequest{
		DefinitionId: &cadestrov1.DefinitionId{Value: definitionID}, ActionSetId: &cadestrov1.ActionSetId{Value: set2}, NewOrder: 0,
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(2), reordered.Msg.Definition.MemberCount)
	got, err = f.handlers.GetDefinition(ctx, connect.NewRequest(&cadestrov1.GetDefinitionRequest{Id: &cadestrov1.DefinitionId{Value: definitionID}}))
	require.NoError(t, err)
	assert.Equal(t, set2, got.Msg.Members[0].GetActionSetId().GetValue())

	removed, err := f.handlers.RemoveActionSetFromDefinition(ctx, connect.NewRequest(&cadestrov1.RemoveActionSetFromDefinitionRequest{
		DefinitionId: &cadestrov1.DefinitionId{Value: definitionID}, ActionSetId: &cadestrov1.ActionSetId{Value: set1},
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(1), removed.Msg.Definition.MemberCount)
	_, err = f.handlers.RemoveActionSetFromDefinition(ctx, connect.NewRequest(&cadestrov1.RemoveActionSetFromDefinitionRequest{
		DefinitionId: &cadestrov1.DefinitionId{Value: definitionID}, ActionSetId: &cadestrov1.ActionSetId{Value: set1},
	}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))

	_, err = f.handlers.DeleteDefinition(ctx, connect.NewRequest(&cadestrov1.DeleteDefinitionRequest{Id: &cadestrov1.DefinitionId{Value: definitionID}}))
	require.NoError(t, err)
	_, err = f.handlers.GetDefinition(ctx, connect.NewRequest(&cadestrov1.GetDefinitionRequest{Id: &cadestrov1.DefinitionId{Value: definitionID}}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))

	for _, procedure := range authoring.DefinitionMutationProcedures() {
		operation, err := latestOperationFor(t, f.store, f.raw, procedure)
		require.NoError(t, err, procedure)
		effects, err := f.store.ListAuditEffects(context.Background(), operation.OperationID)
		require.NoError(t, err, procedure)
		assert.NotEmpty(t, effects, procedure)
	}
}

func TestDefinitionHandlers_KeysetAndDirectScope(t *testing.T) {
	f := newActionHandlerFixture(t)
	state := authoring.New(authoring.Config{Store: f.store, Now: func() time.Time { return f.now }})
	create := func(name string) string {
		op := actionOperation()
		row, err := state.CreateDefinition(context.Background(), op, authoring.CreateDefinitionParams{
			Name: name, CreatedBy: op.ActorID, Schedule: &cadestrov1.ActionSchedule{RunOnAssign: true},
		})
		require.NoError(t, err)
		return row.ID
	}
	directID, outsideID, unassignedID := create("direct"), create("outside"), create("unassigned")
	groupA, groupB := newID(), newID()
	_, err := f.raw.Exec(context.Background(),
		`INSERT INTO device_groups (id, name) VALUES ($1, 'A'), ($2, 'B')`, groupA, groupB)
	require.NoError(t, err)
	_, err = f.raw.Exec(context.Background(), `
		INSERT INTO assignments (id, source_type, source_id, target_type, target_id, created_at, created_by)
		VALUES ($1, 'definition', $2, 'device_group', $3, $4, $5),
		       ($6, 'definition', $7, 'device_group', $8, $4, $5)`,
		newID(), directID, groupA, f.now, f.actorID,
		newID(), outsideID, groupB)
	require.NoError(t, err)

	scoped := auth.WithUser(context.Background(), &auth.UserContext{
		ID: f.actorID, Kind: auth.PrincipalUser,
		Permissions: []string{"GetDefinition", "ListDefinitions", "RenameDefinition"},
		ScopedGrants: []auth.ScopedGrant{{
			Permission: "ListDevices", ScopeKind: auth.ScopeKindDeviceGroup, ScopeID: groupA,
		}},
	})
	_, err = f.handlers.GetDefinition(scoped, connect.NewRequest(&cadestrov1.GetDefinitionRequest{Id: &cadestrov1.DefinitionId{Value: directID}}))
	require.NoError(t, err)
	for _, id := range []string{outsideID, unassignedID} {
		_, err = f.handlers.GetDefinition(scoped, connect.NewRequest(&cadestrov1.GetDefinitionRequest{Id: &cadestrov1.DefinitionId{Value: id}}))
		assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), id)
	}
	_, err = f.handlers.RenameDefinition(scoped, connect.NewRequest(&cadestrov1.RenameDefinitionRequest{Id: &cadestrov1.DefinitionId{Value: outsideID}, Name: "denied"}))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	_, err = f.handlers.RenameDefinition(scoped, connect.NewRequest(&cadestrov1.RenameDefinitionRequest{Id: &cadestrov1.DefinitionId{Value: directID}, Name: "allowed"}))
	require.NoError(t, err)

	list, err := f.handlers.ListDefinitions(scoped, connect.NewRequest(&cadestrov1.ListDefinitionsRequest{}))
	require.NoError(t, err)
	require.Len(t, list.Msg.Definitions, 1)
	assert.Equal(t, directID, list.Msg.Definitions[0].GetId().GetValue())
	assert.Equal(t, int32(1), list.Msg.TotalCount)

	global := f.actor("ListDefinitions")
	page, err := f.handlers.ListDefinitions(global, connect.NewRequest(&cadestrov1.ListDefinitionsRequest{PageSize: 1}))
	require.NoError(t, err)
	require.Len(t, page.Msg.Definitions, 1)
	assert.NotEmpty(t, page.Msg.NextPageToken)
	assert.Equal(t, int32(3), page.Msg.TotalCount)
	all, err := f.handlers.ListDefinitions(global, connect.NewRequest(&cadestrov1.ListDefinitionsRequest{}))
	require.NoError(t, err)
	ids := []string{all.Msg.Definitions[0].GetId().GetValue(), all.Msg.Definitions[1].GetId().GetValue(), all.Msg.Definitions[2].GetId().GetValue()}
	sort.Strings(ids)
	want := []string{directID, outsideID, unassignedID}
	sort.Strings(want)
	assert.Equal(t, want, ids)
}

func TestDefinitionHandlers_AddRequiresVisibleActionSet(t *testing.T) {
	f := newActionHandlerFixture(t)
	state := authoring.New(authoring.Config{Store: f.store})
	definitionOp := actionOperation()
	definition, err := state.CreateDefinition(context.Background(), definitionOp, authoring.CreateDefinitionParams{
		Name: "target", CreatedBy: definitionOp.ActorID, Schedule: &cadestrov1.ActionSchedule{RunOnAssign: true},
	})
	require.NoError(t, err)
	inScope := createDefinitionSet(t, state, "in")
	outOfScope := createDefinitionSet(t, state, "out")
	groupA, groupB := newID(), newID()
	_, err = f.raw.Exec(context.Background(),
		`INSERT INTO device_groups (id, name) VALUES ($1, 'A'), ($2, 'B')`, groupA, groupB)
	require.NoError(t, err)
	_, err = f.raw.Exec(context.Background(), `
		INSERT INTO assignments (id, source_type, source_id, target_type, target_id, created_at, created_by)
		VALUES ($1, 'definition', $2, 'device_group', $3, CURRENT_TIMESTAMP, $4),
		       ($5, 'action_set', $6, 'device_group', $3, CURRENT_TIMESTAMP, $4),
		       ($7, 'action_set', $8, 'device_group', $9, CURRENT_TIMESTAMP, $4)`,
		newID(), definition.ID, groupA, f.actorID,
		newID(), inScope, newID(), outOfScope, groupB)
	require.NoError(t, err)
	scoped := auth.WithUser(context.Background(), &auth.UserContext{
		ID: f.actorID, Kind: auth.PrincipalUser, Permissions: []string{"AddActionSetToDefinition"},
		ScopedGrants: []auth.ScopedGrant{{
			Permission: "ListDevices", ScopeKind: auth.ScopeKindDeviceGroup, ScopeID: groupA,
		}},
	})
	_, err = f.handlers.AddActionSetToDefinition(scoped, connect.NewRequest(&cadestrov1.AddActionSetToDefinitionRequest{
		DefinitionId: &cadestrov1.DefinitionId{Value: definition.ID}, ActionSetId: &cadestrov1.ActionSetId{Value: outOfScope},
	}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	_, err = f.handlers.AddActionSetToDefinition(scoped, connect.NewRequest(&cadestrov1.AddActionSetToDefinitionRequest{
		DefinitionId: &cadestrov1.DefinitionId{Value: definition.ID}, ActionSetId: &cadestrov1.ActionSetId{Value: inScope},
	}))
	require.NoError(t, err)
}

func TestDefinitionHandlers_CorruptStoredScheduleFailsClosed(t *testing.T) {
	f := newActionHandlerFixture(t)
	state := authoring.New(authoring.Config{Store: f.store})
	op := actionOperation()
	definition, err := state.CreateDefinition(context.Background(), op, authoring.CreateDefinitionParams{
		Name: "safe", CreatedBy: op.ActorID, Schedule: &cadestrov1.ActionSchedule{RunOnAssign: true},
	})
	require.NoError(t, err)
	_, err = f.raw.Exec(context.Background(), `UPDATE definitions SET schedule = '{}' WHERE id = $1`, definition.ID)
	require.NoError(t, err)

	_, err = f.handlers.GetDefinition(f.actor("GetDefinition"), connect.NewRequest(&cadestrov1.GetDefinitionRequest{Id: &cadestrov1.DefinitionId{Value: definition.ID}}))
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
}

func TestDefinitionHandlers_MountsExactSurface(t *testing.T) {
	f := newActionHandlerFixture(t)
	assert.Equal(t, []string{
		cadestrov1connect.ControlServiceCreateDefinitionProcedure,
		cadestrov1connect.ControlServiceGetDefinitionProcedure,
		cadestrov1connect.ControlServiceListDefinitionsProcedure,
		cadestrov1connect.ControlServiceRenameDefinitionProcedure,
		cadestrov1connect.ControlServiceUpdateDefinitionDescriptionProcedure,
		cadestrov1connect.ControlServiceUpdateDefinitionScheduleProcedure,
		cadestrov1connect.ControlServiceDeleteDefinitionProcedure,
		cadestrov1connect.ControlServiceAddActionSetToDefinitionProcedure,
		cadestrov1connect.ControlServiceRemoveActionSetFromDefinitionProcedure,
		cadestrov1connect.ControlServiceReorderActionSetInDefinitionProcedure,
	}, f.handlers.MountDefinitions(http.NewServeMux()))
}

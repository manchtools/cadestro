package store_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/devicegroup"
)

type deviceGroupHandlerFixture struct {
	*deviceHandlerFixture
	handlers *devicegroup.Handlers
}

func newDeviceGroupHandlerFixture(t *testing.T) *deviceGroupHandlerFixture {
	t.Helper()
	devices := newDeviceHandlerFixture(t)
	return &deviceGroupHandlerFixture{
		deviceHandlerFixture: devices,
		handlers: devicegroup.NewHandlers(devicegroup.HandlersConfig{
			Store: devices.store, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			Now: func() time.Time { return devices.now },
		}),
	}
}

func TestDeviceGroupHandlers_ValidateBeforeAuthentication(t *testing.T) {
	f := newDeviceGroupHandlerFixture(t)
	_, err := validated(f.handlers.GetDeviceGroup)(context.Background(), connect.NewRequest(&cadestrov1.GetDeviceGroupRequest{Id: "bad"}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	_, err = validated(f.handlers.GetDeviceGroup)(context.Background(), connect.NewRequest(&cadestrov1.GetDeviceGroupRequest{Id: newID()}))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

func TestDeviceGroupHandlers_CRUDMembershipAndAudit(t *testing.T) {
	f := newDeviceGroupHandlerFixture(t)
	ctx := f.actor(
		"CreateStaticDeviceGroup", "CreateDynamicDeviceGroup", "GetDeviceGroup", "ListDeviceGroups",
		"ListDeviceGroupsForDevice", "RenameDeviceGroup", "UpdateDeviceGroupDescription",
		"UpdateDynamicDeviceGroupQuery", "DeleteDeviceGroup", "AddDeviceToGroup", "RemoveDeviceFromGroup",
		"ValidateDynamicQuery", "EvaluateDynamicGroup", "SetDeviceGroupSyncInterval",
		"SetDeviceGroupInventoryInterval", "SetDeviceGroupMaintenanceWindow",
	)

	created, err := f.handlers.CreateDeviceGroup(ctx, connect.NewRequest(&cadestrov1.CreateDeviceGroupRequest{
		Name: "workstations", Description: "static fleet",
	}))
	require.NoError(t, err)
	id := created.Msg.Group.Id

	added, err := f.handlers.AddDeviceToGroup(ctx, connect.NewRequest(&cadestrov1.AddDeviceToGroupRequest{
		GroupId: id, DeviceId: f.directID, DeviceIds: []string{f.groupID, f.directID},
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(2), added.Msg.Group.MemberCount)

	got, err := f.handlers.GetDeviceGroup(ctx, connect.NewRequest(&cadestrov1.GetDeviceGroupRequest{Id: id}))
	require.NoError(t, err)
	require.Len(t, got.Msg.Devices, 2)
	assert.Len(t, got.Msg.DeviceIds, 2)

	removed, err := f.handlers.RemoveDeviceFromGroup(ctx, connect.NewRequest(&cadestrov1.RemoveDeviceFromGroupRequest{
		GroupId: id, DeviceId: f.directID,
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(1), removed.Msg.Group.MemberCount)

	renamed, err := f.handlers.RenameDeviceGroup(ctx, connect.NewRequest(&cadestrov1.RenameDeviceGroupRequest{Id: id, Name: "renamed"}))
	require.NoError(t, err)
	assert.Equal(t, "renamed", renamed.Msg.Group.Name)
	described, err := f.handlers.UpdateDeviceGroupDescription(ctx, connect.NewRequest(&cadestrov1.UpdateDeviceGroupDescriptionRequest{
		Id: id, Description: "direct state",
	}))
	require.NoError(t, err)
	assert.Equal(t, "direct state", described.Msg.Group.Description)

	synced, err := f.handlers.SetDeviceGroupSyncInterval(ctx, connect.NewRequest(&cadestrov1.SetDeviceGroupSyncIntervalRequest{
		Id: id, SyncIntervalMinutes: 30,
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(30), synced.Msg.Group.SyncIntervalMinutes)
	inventoried, err := f.handlers.SetDeviceGroupInventoryInterval(ctx, connect.NewRequest(&cadestrov1.SetDeviceGroupInventoryIntervalRequest{
		Id: id, InventoryIntervalMinutes: 120,
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(120), inventoried.Msg.Group.InventoryIntervalMinutes)
	windowed, err := f.handlers.SetDeviceGroupMaintenanceWindow(ctx, connect.NewRequest(&cadestrov1.SetDeviceGroupMaintenanceWindowRequest{
		Id: id, MaintenanceWindow: &cadestrov1.MaintenanceWindow{Schedule: []*cadestrov1.MaintenanceWindowEntry{{
			Days: []string{"mon"}, Allow: "09:00-17:00",
		}}},
	}))
	require.NoError(t, err)
	require.Len(t, windowed.Msg.Group.MaintenanceWindow.Schedule, 1)

	// The mode is a property the owner may change in either direction (target
	// design §5.1). This group still has one hand-picked member; converting it
	// hands membership to the rule, so that member does not survive the call.
	converted, err := f.handlers.UpdateDeviceGroupQuery(ctx, connect.NewRequest(&cadestrov1.UpdateDeviceGroupQueryRequest{
		Id: id, IsDynamic: true, DynamicQuery: `device.labels.env equals prod`,
	}))
	require.NoError(t, err, "a curated group is convertible to a rule")
	assert.True(t, converted.Msg.Group.IsDynamic)
	assert.Equal(t, `device.labels.env equals prod`, converted.Msg.Group.DynamicQuery)
	assert.Zero(t, converted.Msg.Group.MemberCount, "the curated membership does not survive the rule")
	convertedGroup, err := f.handlers.GetDeviceGroup(ctx, connect.NewRequest(&cadestrov1.GetDeviceGroupRequest{Id: id}))
	require.NoError(t, err)
	assert.Empty(t, convertedGroup.Msg.Devices)
	assert.Empty(t, convertedGroup.Msg.DeviceIds)
	// An invalid query is still refused, and refusing it changes nothing.
	_, err = f.handlers.UpdateDeviceGroupQuery(ctx, connect.NewRequest(&cadestrov1.UpdateDeviceGroupQueryRequest{
		Id: id, IsDynamic: true, DynamicQuery: "(",
	}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	dynamic, err := f.handlers.CreateDeviceGroup(ctx, connect.NewRequest(&cadestrov1.CreateDeviceGroupRequest{
		Name: "dynamic workstations", IsDynamic: true, DynamicQuery: `device.labels.env equals prod`,
	}))
	require.NoError(t, err)
	invalid, err := f.handlers.ValidateDynamicQuery(ctx, connect.NewRequest(&cadestrov1.ValidateDynamicQueryRequest{Query: "("}))
	require.NoError(t, err)
	assert.False(t, invalid.Msg.Valid)
	_, err = f.raw.Exec(ctx, `INSERT INTO device_labels (device_id, key, value) VALUES ($1, 'env', 'prod')`, f.outsideID)
	require.NoError(t, err)
	_, err = f.raw.Exec(ctx, `UPDATE device_inventory SET rows = '[{"physical_memory":2048}]' WHERE device_id = $1 AND table_name = 'system_info'`, f.groupID)
	require.NoError(t, err)
	for _, query := range []string{
		`device.hostname equals group`,
		`device.labels.env equals prod`,
		`device.memory_total greaterThan 1024`,
		`device.group equals scope`,
	} {
		preview, err := f.handlers.ValidateDynamicQuery(ctx, connect.NewRequest(&cadestrov1.ValidateDynamicQueryRequest{Query: query}))
		require.NoError(t, err, query)
		assert.True(t, preview.Msg.Valid, query)
		assert.Equal(t, int32(1), preview.Msg.MatchingDeviceCount, query)
	}
	updatedQuery, err := f.handlers.UpdateDeviceGroupQuery(ctx, connect.NewRequest(&cadestrov1.UpdateDeviceGroupQueryRequest{
		Id: dynamic.Msg.Group.Id, IsDynamic: true, DynamicQuery: `device.hostname equals group`,
	}))
	require.NoError(t, err)
	assert.Equal(t, `device.hostname equals group`, updatedQuery.Msg.Group.DynamicQuery)
	evaluated, err := f.handlers.EvaluateDynamicGroup(ctx, connect.NewRequest(&cadestrov1.EvaluateDynamicGroupRequest{Id: dynamic.Msg.Group.Id}))
	require.NoError(t, err)
	assert.Equal(t, int32(1), evaluated.Msg.DevicesAdded)
	assert.Zero(t, evaluated.Msg.DevicesRemoved)
	assert.Equal(t, int32(1), evaluated.Msg.Group.MemberCount)
	_, err = f.handlers.UpdateDeviceGroupQuery(ctx, connect.NewRequest(&cadestrov1.UpdateDeviceGroupQueryRequest{
		Id: dynamic.Msg.Group.Id, IsDynamic: true, DynamicQuery: `device.hostname equals outside`,
	}))
	require.NoError(t, err)
	evaluated, err = f.handlers.EvaluateDynamicGroup(ctx, connect.NewRequest(&cadestrov1.EvaluateDynamicGroupRequest{Id: dynamic.Msg.Group.Id}))
	require.NoError(t, err)
	assert.Equal(t, int32(1), evaluated.Msg.DevicesAdded)
	assert.Equal(t, int32(1), evaluated.Msg.DevicesRemoved)
	_, err = f.handlers.AddDeviceToGroup(ctx, connect.NewRequest(&cadestrov1.AddDeviceToGroupRequest{
		GroupId: dynamic.Msg.Group.Id, DeviceId: f.outsideID,
	}))
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))

	listed, err := f.handlers.ListDeviceGroups(ctx, connect.NewRequest(&cadestrov1.ListDeviceGroupsRequest{}))
	require.NoError(t, err)
	assert.NotEmpty(t, listed.Msg.Groups)
	assert.GreaterOrEqual(t, listed.Msg.TotalCount, int32(2))
	forDevice, err := f.handlers.ListDeviceGroupsForDevice(ctx, connect.NewRequest(&cadestrov1.ListDeviceGroupsForDeviceRequest{
		DeviceId: f.groupID,
	}))
	require.NoError(t, err)
	assert.NotEmpty(t, forDevice.Msg.Groups)

	_, err = f.handlers.DeleteDeviceGroup(ctx, connect.NewRequest(&cadestrov1.DeleteDeviceGroupRequest{Id: id}))
	require.NoError(t, err)
	_, err = f.handlers.GetDeviceGroup(ctx, connect.NewRequest(&cadestrov1.GetDeviceGroupRequest{Id: id}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))

	for _, procedure := range devicegroup.MutationProcedures() {
		operation, err := latestOperationFor(t, f.store, f.raw, procedure)
		require.NoError(t, err, procedure)
		effects, err := f.store.ListAuditEffects(context.Background(), operation.OperationID)
		require.NoError(t, err, procedure)
		assert.NotEmpty(t, effects, procedure)
	}
}

func TestDeviceGroupHandlers_ShapeSpecificCreatePermissionAndScope(t *testing.T) {
	f := newDeviceGroupHandlerFixture(t)
	staticOnly := f.actor("CreateStaticDeviceGroup")
	_, err := f.handlers.CreateDeviceGroup(staticOnly, connect.NewRequest(&cadestrov1.CreateDeviceGroupRequest{
		Name: "denied", IsDynamic: true, DynamicQuery: `device.labels.env equals prod`,
	}))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))

	dynamicOnly := f.actor("CreateDynamicDeviceGroup")
	created, err := f.handlers.CreateDeviceGroup(dynamicOnly, connect.NewRequest(&cadestrov1.CreateDeviceGroupRequest{
		Name: "dynamic", IsDynamic: true, DynamicQuery: `device.labels.env equals prod`,
	}))
	require.NoError(t, err)

	scoped := auth.WithUser(context.Background(), &auth.UserContext{
		ID: f.actorID, Kind: auth.PrincipalUser,
		Permissions: []string{"GetDeviceGroup", "ListDeviceGroups", "RenameDeviceGroup"},
		ScopedGrants: []auth.ScopedGrant{{
			Permission: "GetDeviceGroup", ScopeKind: auth.ScopeKindDeviceGroup, ScopeID: f.scopeGroup,
		}, {
			Permission: "ListDeviceGroups", ScopeKind: auth.ScopeKindDeviceGroup, ScopeID: f.scopeGroup,
		}, {
			Permission: "RenameDeviceGroup", ScopeKind: auth.ScopeKindDeviceGroup, ScopeID: f.scopeGroup,
		}},
	})
	_, err = f.handlers.GetDeviceGroup(scoped, connect.NewRequest(&cadestrov1.GetDeviceGroupRequest{Id: created.Msg.Group.Id}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	_, err = f.handlers.RenameDeviceGroup(scoped, connect.NewRequest(&cadestrov1.RenameDeviceGroupRequest{
		Id: created.Msg.Group.Id, Name: "denied",
	}))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	list, err := f.handlers.ListDeviceGroups(scoped, connect.NewRequest(&cadestrov1.ListDeviceGroupsRequest{}))
	require.NoError(t, err)
	require.Len(t, list.Msg.Groups, 1)
	assert.Equal(t, f.scopeGroup, list.Msg.Groups[0].Id)

	membershipScoped := auth.WithUser(context.Background(), &auth.UserContext{
		ID: f.actorID, Kind: auth.PrincipalUser, Permissions: []string{"AddDeviceToGroup"},
		ScopedGrants: []auth.ScopedGrant{{
			Permission: "AddDeviceToGroup", ScopeKind: auth.ScopeKindDeviceGroup, ScopeID: f.scopeGroup,
		}},
	})
	_, err = f.handlers.AddDeviceToGroup(membershipScoped, connect.NewRequest(&cadestrov1.AddDeviceToGroupRequest{
		GroupId: f.scopeGroup, DeviceId: f.groupID,
	}))
	require.NoError(t, err)
	_, err = f.handlers.AddDeviceToGroup(membershipScoped, connect.NewRequest(&cadestrov1.AddDeviceToGroupRequest{
		GroupId: f.scopeGroup, DeviceId: f.outsideID,
	}))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err), "membership writes must not widen the caller's device scope")
}

func TestDeviceGroupHandlers_MountsCompleteDirectSurface(t *testing.T) {
	f := newDeviceGroupHandlerFixture(t)
	mounted := f.handlers.Mount(http.NewServeMux())
	assert.Equal(t, []string{
		cadestrov1connect.ControlServiceCreateDeviceGroupProcedure,
		cadestrov1connect.ControlServiceGetDeviceGroupProcedure,
		cadestrov1connect.ControlServiceListDeviceGroupsProcedure,
		cadestrov1connect.ControlServiceListDeviceGroupsForDeviceProcedure,
		cadestrov1connect.ControlServiceRenameDeviceGroupProcedure,
		cadestrov1connect.ControlServiceUpdateDeviceGroupDescriptionProcedure,
		cadestrov1connect.ControlServiceUpdateDeviceGroupQueryProcedure,
		cadestrov1connect.ControlServiceDeleteDeviceGroupProcedure,
		cadestrov1connect.ControlServiceAddDeviceToGroupProcedure,
		cadestrov1connect.ControlServiceRemoveDeviceFromGroupProcedure,
		cadestrov1connect.ControlServiceValidateDynamicQueryProcedure,
		cadestrov1connect.ControlServiceEvaluateDynamicGroupProcedure,
		cadestrov1connect.ControlServiceSetDeviceGroupSyncIntervalProcedure,
		cadestrov1connect.ControlServiceSetDeviceGroupInventoryIntervalProcedure,
		cadestrov1connect.ControlServiceSetDeviceGroupMaintenanceWindowProcedure,
	}, mounted)
}

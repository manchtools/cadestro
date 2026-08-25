package devicegroup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/oklog/ulid/v2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/manchtools/cadestro/contract/maintenance"
	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/middleware"
	"github.com/manchtools/cadestro/server/internal/store"
)

const defaultPageSize = int32(50)

type HandlersConfig struct {
	Store  *store.Store
	Logger *slog.Logger
	Now    func() time.Time
}

type Handlers struct {
	store  *store.Store
	state  *State
	logger *slog.Logger
}

func NewHandlers(cfg HandlersConfig) *Handlers {
	if cfg.Store == nil {
		panic("device group: handler store is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Handlers{
		store: cfg.Store, state: NewState(Config{Store: cfg.Store, Now: cfg.Now}),
		logger: cfg.Logger,
	}
}

func (h *Handlers) actor(ctx context.Context) (*auth.UserContext, error) {
	actor, ok := auth.UserFromContext(ctx)
	if !ok {
		return nil, rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_NOT_AUTHENTICATED, connect.CodeUnauthenticated, "not authenticated")
	}
	return actor, nil
}

func (h *Handlers) authorize(ctx context.Context, permission, resourceID string) error {
	if !auth.AuthorizeContext(ctx, permission, resourceID) {
		return rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, connect.CodePermissionDenied, "permission denied")
	}
	return nil
}

func (h *Handlers) readScope(ctx context.Context, permission, id string) error {
	if err := h.authorize(ctx, permission, id); err != nil {
		return err
	}
	groups, restricted := auth.DeviceScopeListFilter(ctx, permission)
	if restricted && !contains(groups, id) {
		return rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_DEVICE_GROUP_NOT_FOUND, connect.CodeNotFound, "device group not found")
	}
	return nil
}

func (h *Handlers) writeScope(ctx context.Context, permission, id string) error {
	if err := h.authorize(ctx, permission, id); err != nil {
		return err
	}
	groups, restricted := auth.DeviceScopeListFilter(ctx, permission)
	if restricted && !contains(groups, id) {
		return rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, connect.CodePermissionDenied, "permission denied")
	}
	return nil
}

func (h *Handlers) operation(req connect.AnyRequest, actor *auth.UserContext, procedure, permission string) store.AuditOperation {
	op := store.AuditOperation{
		Class: store.ClassMutation, ActorType: string(actor.Kind), Origin: auth.ControlRPCOrigin,
		RequestDescriptor: procedure, AuthorizationOutcome: store.AuthorizationAllowed,
		AuthorizationDetail: permission, Result: store.ResultSuccess, ResultCode: "OK",
	}
	if actor.CanOwnResources() {
		op.ActorID = actor.ID
	}
	if ip := auth.ClientIP(req); ip != "" {
		op.OriginFingerprint = auth.Fingerprint(ip)
	}
	return op
}

func (h *Handlers) CreateDeviceGroup(ctx context.Context, req *connect.Request[cadestrov1.CreateDeviceGroupRequest]) (*connect.Response[cadestrov1.CreateDeviceGroupResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	permission := "CreateStaticDeviceGroup"
	if req.Msg.DynamicQuery != nil {
		permission = "CreateDynamicDeviceGroup"
	}
	if !auth.HasPermission(ctx, permission) {
		return nil, rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, connect.CodePermissionDenied, "permission denied")
	}
	row, err := h.state.Create(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceCreateDeviceGroupProcedure, permission), CreateParams{
		Name: req.Msg.Name, Description: req.Msg.Description, CreatedBy: actor.ID,
		Query: req.Msg.DynamicQuery,
	})
	if err != nil {
		return nil, h.mapError(ctx, "create device group", err)
	}
	group, err := h.groupProto(row)
	if err != nil {
		return nil, h.internal(ctx, "encode created device group", err)
	}
	return connect.NewResponse(&cadestrov1.CreateDeviceGroupResponse{Group: group}), nil
}

func (h *Handlers) GetDeviceGroup(ctx context.Context, req *connect.Request[cadestrov1.GetDeviceGroupRequest]) (*connect.Response[cadestrov1.GetDeviceGroupResponse], error) {
	if _, err := h.actor(ctx); err != nil {
		return nil, err
	}
	if err := h.readScope(ctx, "GetDeviceGroup", req.Msg.GetId().GetValue()); err != nil {
		return nil, err
	}
	row, err := h.store.GetDeviceGroup(ctx, req.Msg.GetId().GetValue())
	if err != nil {
		return nil, h.mapError(ctx, "get device group", err)
	}
	members, err := h.store.ListDeviceGroupMembers(ctx, req.Msg.GetId().GetValue())
	if err != nil {
		return nil, h.internal(ctx, "list device group members", err)
	}
	group, err := h.groupProto(row)
	if err != nil {
		return nil, h.internal(ctx, "decode device group", err)
	}
	ids := make([]string, len(members))
	devices := make([]*cadestrov1.DeviceGroupMember, len(members))
	for i, member := range members {
		ids[i] = member.DeviceID
		devices[i] = &cadestrov1.DeviceGroupMember{
			DeviceId: &cadestrov1.DeviceId{Value: member.DeviceID}, Hostname: member.Hostname, AgentVersion: member.AgentVersion,
		}
		if member.LastSeenAt != nil {
			devices[i].LastSeenAt = timestamppb.New(*member.LastSeenAt)
		}
	}
	return connect.NewResponse(&cadestrov1.GetDeviceGroupResponse{
		Group: group, DeviceIds: func() []*cadestrov1.DeviceId {
			out := make([]*cadestrov1.DeviceId, len(ids))
			for i, id := range ids {
				out[i] = &cadestrov1.DeviceId{Value: id}
			}
			return out
		}(), Devices: devices,
	}), nil
}

func (h *Handlers) ListDeviceGroups(ctx context.Context, req *connect.Request[cadestrov1.ListDeviceGroupsRequest]) (*connect.Response[cadestrov1.ListDeviceGroupsResponse], error) {
	if _, err := h.actor(ctx); err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, "ListDeviceGroups", ""); err != nil {
		return nil, err
	}
	if !validPageToken(req.Msg.PageToken) {
		return nil, rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_INVALID_PAGE_TOKEN, connect.CodeInvalidArgument, "invalid page token")
	}
	limit := req.Msg.PageSize
	if limit == 0 {
		limit = defaultPageSize
	}
	groups, restricted := auth.DeviceScopeListFilter(ctx, "ListDeviceGroups")
	filter := store.DeviceGroupListFilter{
		AfterID: req.Msg.PageToken, Limit: limit + 1,
		ScopeRestricted: restricted, ScopeGroupIDs: groups,
	}
	rows, err := h.store.ListDeviceGroups(ctx, filter)
	if err != nil {
		return nil, h.internal(ctx, "list device groups", err)
	}
	hasMore := len(rows) > int(limit)
	if hasMore {
		rows = rows[:limit]
	}
	count, err := h.store.CountDeviceGroups(ctx, filter)
	if err != nil {
		return nil, h.internal(ctx, "count device groups", err)
	}
	out := make([]*cadestrov1.DeviceGroup, len(rows))
	for i, row := range rows {
		out[i], err = h.groupProto(row)
		if err != nil {
			return nil, h.internal(ctx, "decode listed device group", err)
		}
	}
	next := ""
	if hasMore {
		next = rows[len(rows)-1].ID
	}
	return connect.NewResponse(&cadestrov1.ListDeviceGroupsResponse{
		Groups: out, NextPageToken: next, TotalCount: boundedCount(count),
	}), nil
}

func (h *Handlers) ListDeviceGroupsForDevice(ctx context.Context, req *connect.Request[cadestrov1.ListDeviceGroupsForDeviceRequest]) (*connect.Response[cadestrov1.ListDeviceGroupsForDeviceResponse], error) {
	deviceID := req.Msg.GetDeviceId().GetValue()
	if _, err := h.actor(ctx); err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, "ListDeviceGroupsForDevice", deviceID); err != nil {
		return nil, err
	}
	if _, err := h.store.GetDevice(ctx, deviceID); err != nil {
		if store.IsNotFound(err) {
			return nil, rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_DEVICE_NOT_FOUND, connect.CodeNotFound, "device not found")
		}
		return nil, h.internal(ctx, "read device for groups", err)
	}

	if err := h.enforceDeviceReadScope(ctx, "ListDeviceGroupsForDevice", deviceID); err != nil {
		return nil, err
	}
	groups, restricted := auth.DeviceScopeListFilter(ctx, "ListDeviceGroupsForDevice")
	rows, err := h.store.ListDeviceGroupsForDevice(ctx, deviceID, store.DeviceGroupListFilter{
		ScopeRestricted: restricted, ScopeGroupIDs: groups,
	})
	if err != nil {
		return nil, h.internal(ctx, "list groups for device", err)
	}
	out := make([]*cadestrov1.DeviceGroup, len(rows))
	for i, row := range rows {
		out[i], err = h.groupProto(row)
		if err != nil {
			return nil, h.internal(ctx, "decode device group for device", err)
		}
	}
	return connect.NewResponse(&cadestrov1.ListDeviceGroupsForDeviceResponse{Groups: out}), nil
}

func (h *Handlers) RenameDeviceGroup(ctx context.Context, req *connect.Request[cadestrov1.RenameDeviceGroupRequest]) (*connect.Response[cadestrov1.UpdateDeviceGroupResponse], error) {
	actor, err := h.mutationActor(ctx, req.Msg.GetId().GetValue(), "RenameDeviceGroup")
	if err != nil {
		return nil, err
	}
	row, err := h.state.Rename(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceRenameDeviceGroupProcedure, "RenameDeviceGroup"), req.Msg.GetId().GetValue(), req.Msg.Name)
	return h.updated(ctx, "rename device group", row, err)
}

func (h *Handlers) UpdateDeviceGroupDescription(ctx context.Context, req *connect.Request[cadestrov1.UpdateDeviceGroupDescriptionRequest]) (*connect.Response[cadestrov1.UpdateDeviceGroupResponse], error) {
	actor, err := h.mutationActor(ctx, req.Msg.GetId().GetValue(), "UpdateDeviceGroupDescription")
	if err != nil {
		return nil, err
	}
	row, err := h.state.UpdateDescription(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceUpdateDeviceGroupDescriptionProcedure, "UpdateDeviceGroupDescription"),
		req.Msg.GetId().GetValue(), req.Msg.Description)
	return h.updated(ctx, "update device group description", row, err)
}

func (h *Handlers) UpdateDeviceGroupQuery(ctx context.Context, req *connect.Request[cadestrov1.UpdateDeviceGroupQueryRequest]) (*connect.Response[cadestrov1.UpdateDeviceGroupQueryResponse], error) {
	const permission = "UpdateDynamicDeviceGroupQuery"
	actor, err := h.mutationActor(ctx, req.Msg.GetId().GetValue(), permission)
	if err != nil {
		return nil, err
	}
	row, err := h.state.UpdateQuery(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceUpdateDeviceGroupQueryProcedure, permission),
		req.Msg.GetId().GetValue(), req.Msg.DynamicQuery)
	if err != nil {
		return nil, h.mapError(ctx, "update device group query", err)
	}
	group, err := h.groupProto(row)
	if err != nil {
		return nil, h.internal(ctx, "decode updated device group query", err)
	}
	return connect.NewResponse(&cadestrov1.UpdateDeviceGroupQueryResponse{Group: group}), nil
}

func (h *Handlers) DeleteDeviceGroup(ctx context.Context, req *connect.Request[cadestrov1.DeleteDeviceGroupRequest]) (*connect.Response[cadestrov1.DeleteDeviceGroupResponse], error) {
	actor, err := h.mutationActor(ctx, req.Msg.GetId().GetValue(), "DeleteDeviceGroup")
	if err != nil {
		return nil, err
	}
	if err := h.state.Delete(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceDeleteDeviceGroupProcedure, "DeleteDeviceGroup"), req.Msg.GetId().GetValue()); err != nil {
		return nil, h.mapError(ctx, "delete device group", err)
	}
	return connect.NewResponse(&cadestrov1.DeleteDeviceGroupResponse{}), nil
}

func (h *Handlers) AddDeviceToGroup(ctx context.Context, req *connect.Request[cadestrov1.AddDeviceToGroupRequest]) (*connect.Response[cadestrov1.AddDeviceToGroupResponse], error) {
	deviceID := req.Msg.GetDeviceId().GetValue()
	ids := make([]string, 0, len(req.Msg.GetDeviceIds()))
	for _, id := range req.Msg.GetDeviceIds() {
		ids = append(ids, id.GetValue())
	}
	if deviceID != "" {
		ids = append(ids, deviceID)
	}
	if len(ids) == 0 || len(ids) > maxBatchDevices {
		return nil, rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_VALIDATION_FAILED, connect.CodeInvalidArgument, "at least one device is required")
	}
	actor, err := h.mutationActor(ctx, req.Msg.GetGroupId().GetValue(), "AddDeviceToGroup")
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if err := h.enforceDeviceScope(ctx, "AddDeviceToGroup", id); err != nil {
			return nil, err
		}
		if _, err := h.store.GetDevice(ctx, id); err != nil {
			if store.IsNotFound(err) {
				return nil, rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_DEVICE_NOT_FOUND, connect.CodeNotFound, "device not found")
			}
			return nil, h.internal(ctx, "read device membership target", err)
		}
	}
	if _, err := h.state.AddDevices(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceAddDeviceToGroupProcedure, "AddDeviceToGroup"), req.Msg.GetGroupId().GetValue(), ids); err != nil {
		return nil, h.mapError(ctx, "add devices to group", err)
	}
	group, err := h.groupResponse(ctx, req.Msg.GetGroupId().GetValue())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&cadestrov1.AddDeviceToGroupResponse{Group: group}), nil
}

func (h *Handlers) RemoveDeviceFromGroup(ctx context.Context, req *connect.Request[cadestrov1.RemoveDeviceFromGroupRequest]) (*connect.Response[cadestrov1.RemoveDeviceFromGroupResponse], error) {
	deviceID := req.Msg.GetDeviceId().GetValue()
	actor, err := h.mutationActor(ctx, req.Msg.GetGroupId().GetValue(), "RemoveDeviceFromGroup")
	if err != nil {
		return nil, err
	}
	if err := h.state.RemoveDevice(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceRemoveDeviceFromGroupProcedure, "RemoveDeviceFromGroup"),
		req.Msg.GetGroupId().GetValue(), deviceID); err != nil {
		return nil, h.mapError(ctx, "remove device from group", err)
	}
	group, err := h.groupResponse(ctx, req.Msg.GetGroupId().GetValue())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&cadestrov1.RemoveDeviceFromGroupResponse{Group: group}), nil
}

func (h *Handlers) ValidateDynamicQuery(ctx context.Context, req *connect.Request[cadestrov1.ValidateDynamicQueryRequest]) (*connect.Response[cadestrov1.ValidateDynamicQueryResponse], error) {
	if _, err := h.actor(ctx); err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, "ValidateDynamicQuery", ""); err != nil {
		return nil, err
	}
	count, err := h.state.CountMatchingDevices(ctx, req.Msg.Query)
	if err != nil {
		if errors.Is(err, ErrInvalidQuery) {
			return connect.NewResponse(&cadestrov1.ValidateDynamicQueryResponse{Valid: false, Error: err.Error()}), nil
		}
		return nil, h.mapError(ctx, "count dynamic query matches", err)
	}
	return connect.NewResponse(&cadestrov1.ValidateDynamicQueryResponse{
		Valid: true, MatchingDeviceCount: boundedCount(count),
	}), nil
}

func (h *Handlers) EvaluateDynamicGroup(ctx context.Context, req *connect.Request[cadestrov1.EvaluateDynamicGroupRequest]) (*connect.Response[cadestrov1.EvaluateDynamicGroupResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, "EvaluateDynamicGroup", req.Msg.GetId().GetValue()); err != nil {
		return nil, err
	}
	result, err := h.state.EvaluateDynamicGroup(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceEvaluateDynamicGroupProcedure, "EvaluateDynamicGroup"), req.Msg.GetId().GetValue())
	if err != nil {
		return nil, h.mapError(ctx, "evaluate dynamic group", err)
	}
	group, err := h.groupProto(result.Group)
	if err != nil {
		return nil, h.internal(ctx, "decode evaluated device group", err)
	}
	return connect.NewResponse(&cadestrov1.EvaluateDynamicGroupResponse{
		Group: group, DevicesAdded: boundedCount(result.Added), DevicesRemoved: boundedCount(result.Removed),
	}), nil
}

func (h *Handlers) SetDeviceGroupSyncInterval(ctx context.Context, req *connect.Request[cadestrov1.SetDeviceGroupSyncIntervalRequest]) (*connect.Response[cadestrov1.UpdateDeviceGroupResponse], error) {
	actor, err := h.mutationActor(ctx, req.Msg.GetId().GetValue(), "SetDeviceGroupSyncInterval")
	if err != nil {
		return nil, err
	}
	row, err := h.state.SetSyncInterval(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceSetDeviceGroupSyncIntervalProcedure, "SetDeviceGroupSyncInterval"),
		req.Msg.GetId().GetValue(), req.Msg.SyncIntervalMinutes)
	return h.updated(ctx, "set device group sync interval", row, err)
}

func (h *Handlers) SetDeviceGroupInventoryInterval(ctx context.Context, req *connect.Request[cadestrov1.SetDeviceGroupInventoryIntervalRequest]) (*connect.Response[cadestrov1.UpdateDeviceGroupResponse], error) {
	actor, err := h.mutationActor(ctx, req.Msg.GetId().GetValue(), "SetDeviceGroupInventoryInterval")
	if err != nil {
		return nil, err
	}
	row, err := h.state.SetInventoryInterval(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceSetDeviceGroupInventoryIntervalProcedure, "SetDeviceGroupInventoryInterval"),
		req.Msg.GetId().GetValue(), req.Msg.InventoryIntervalMinutes)
	return h.updated(ctx, "set device group inventory interval", row, err)
}

func (h *Handlers) SetDeviceGroupMaintenanceWindow(ctx context.Context, req *connect.Request[cadestrov1.SetDeviceGroupMaintenanceWindowRequest]) (*connect.Response[cadestrov1.UpdateDeviceGroupResponse], error) {
	if err := maintenance.Validate(req.Msg.MaintenanceWindow); err != nil {
		return nil, rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_VALIDATION_FAILED, connect.CodeInvalidArgument, err.Error())
	}
	actor, err := h.mutationActor(ctx, req.Msg.GetId().GetValue(), "SetDeviceGroupMaintenanceWindow")
	if err != nil {
		return nil, err
	}
	raw := []byte("{}")
	if req.Msg.MaintenanceWindow != nil && len(req.Msg.MaintenanceWindow.Schedule) > 0 {
		raw, err = protojson.Marshal(req.Msg.MaintenanceWindow)
		if err != nil {
			return nil, h.internal(ctx, "encode maintenance window", err)
		}
	}
	row, err := h.state.SetMaintenanceWindow(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceSetDeviceGroupMaintenanceWindowProcedure, "SetDeviceGroupMaintenanceWindow"),
		req.Msg.GetId().GetValue(), raw)
	return h.updated(ctx, "set device group maintenance window", row, err)
}

func (h *Handlers) mutationActor(ctx context.Context, id, permission string) (*auth.UserContext, error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.writeScope(ctx, permission, id); err != nil {
		return nil, err
	}
	if _, err := h.store.GetDeviceGroup(ctx, id); err != nil {
		return nil, h.mapError(ctx, "read mutation target", err)
	}
	return actor, nil
}

type scopeResolver struct{ store *store.Store }

func (r scopeResolver) DeviceGroupsForDevice(ctx context.Context, deviceID string) ([]string, error) {
	ids, err := r.store.ListDeviceGroupIDs(ctx, deviceID)
	if store.IsNotFound(err) {
		return nil, nil
	}
	return ids, err
}

func (scopeResolver) UserGroupsForUser(context.Context, string) ([]string, error) {
	return nil, fmt.Errorf("user scope resolution is unavailable in the device-group domain")
}

func (h *Handlers) enforceDeviceScope(ctx context.Context, permission, deviceID string) error {
	err := auth.EnforceDeviceScopeOnBaseTier(ctx, scopeResolver{h.store}, permission, deviceID)
	if err == nil {
		return nil
	}
	if connect.CodeOf(err) == connect.CodeInternal {
		return h.internal(ctx, "resolve device scope", err)
	}
	return rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, connect.CodePermissionDenied, "permission denied")
}

func (h *Handlers) enforceDeviceReadScope(ctx context.Context, permission, deviceID string) error {
	err := auth.EnforceDeviceScopeOnBaseTier(ctx, scopeResolver{h.store}, permission, deviceID)
	if err == nil {
		return nil
	}
	if connect.CodeOf(err) == connect.CodeInternal {
		return h.internal(ctx, "resolve device scope", err)
	}
	return rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_DEVICE_NOT_FOUND, connect.CodeNotFound, "device not found")
}

func (h *Handlers) updated(ctx context.Context, operation string, row store.DeviceGroupView, err error) (*connect.Response[cadestrov1.UpdateDeviceGroupResponse], error) {
	if err != nil {
		return nil, h.mapError(ctx, operation, err)
	}
	group, err := h.groupProto(row)
	if err != nil {
		return nil, h.internal(ctx, "decode updated device group", err)
	}
	return connect.NewResponse(&cadestrov1.UpdateDeviceGroupResponse{Group: group}), nil
}

func (h *Handlers) groupResponse(ctx context.Context, id string) (*cadestrov1.DeviceGroup, error) {
	row, err := h.store.GetDeviceGroup(ctx, id)
	if err != nil {
		return nil, h.mapError(ctx, "read changed device group", err)
	}
	group, err := h.groupProto(row)
	if err != nil {
		return nil, h.internal(ctx, "decode changed device group", err)
	}
	return group, nil
}

func (h *Handlers) groupProto(row store.DeviceGroupView) (*cadestrov1.DeviceGroup, error) {
	group := &cadestrov1.DeviceGroup{
		Id: &cadestrov1.DeviceGroupId{Value: row.ID}, Name: row.Name, Description: row.Description,
		MemberCount: boundedCount(row.LiveMemberCount), CreatedBy: row.CreatedBy,
		SyncIntervalMinutes: row.SyncIntervalMinutes, InventoryIntervalMinutes: row.InventoryIntervalMinutes,
	}
	if row.DynamicQuery != nil {
		group.DynamicQuery = row.DynamicQuery
	}
	if row.CreatedAt != nil {
		group.CreatedAt = timestamppb.New(*row.CreatedAt)
	}
	if len(row.MaintenanceWindow) > 0 && string(row.MaintenanceWindow) != "{}" {
		window := &cadestrov1.MaintenanceWindow{}
		if err := protojson.Unmarshal(row.MaintenanceWindow, window); err != nil {
			return nil, err
		}
		if len(window.Schedule) > 0 {
			group.MaintenanceWindow = window
		}
	}
	return group, nil
}

func (h *Handlers) mapError(ctx context.Context, operation string, err error) error {
	switch {
	case errors.Is(err, ErrInvalidInput):
		return rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_VALIDATION_FAILED, connect.CodeInvalidArgument, "invalid device group")
	case errors.Is(err, ErrInvalidQuery):
		return rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_INVALID_DYNAMIC_QUERY, connect.CodeInvalidArgument, "invalid dynamic query")
	case errors.Is(err, ErrStaticGroup):
		return rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_GROUP_NOT_DYNAMIC, connect.CodeFailedPrecondition, "group is not dynamic")
	case errors.Is(err, ErrDynamicGroup):
		return rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_DYNAMIC_GROUP_MEMBERSHIP_MANAGED, connect.CodeFailedPrecondition, "dynamic group membership is evaluator-managed")
	case errors.Is(err, ErrMemberNotFound):
		return rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_DEVICE_GROUP_MEMBER_NOT_FOUND, connect.CodeNotFound, "device group member not found")
	case store.IsNotFound(err):
		return rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_DEVICE_GROUP_NOT_FOUND, connect.CodeNotFound, "device group not found")
	default:
		return h.internal(ctx, operation, err)
	}
}

func (h *Handlers) internal(ctx context.Context, operation string, err error) *connect.Error {
	h.logger.Error("device-group RPC failed", "operation", operation, "error", err)
	return rpcError(ctx, cadestrov1.ErrorCode_ERROR_CODE_INTERNAL_ERROR, connect.CodeInternal, "internal error")
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func validPageToken(token string) bool {
	if token == "" {
		return true
	}
	_, err := ulid.ParseStrict(token)
	return err == nil
}

func boundedCount(value int64) int32 {
	if value > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(value)
}

func rpcError(ctx context.Context, code cadestrov1.ErrorCode, connectCode connect.Code, message string) *connect.Error {
	err := connect.NewError(connectCode, errors.New(message))
	detail, detailErr := connect.NewErrorDetail(&cadestrov1.ErrorDetail{
		Code: code, RequestId: &cadestrov1.RequestId{Value: middleware.RequestIDFromContext(ctx)},
	})
	if detailErr == nil {
		err.AddDetail(detail)
	}
	return err
}

func (h *Handlers) Mount(mux *http.ServeMux, opts ...connect.HandlerOption) []string {
	if mux == nil {
		panic("device group: mux is required")
	}
	mounted := make([]string, 0, 15)
	register := func(procedure string, handler http.Handler) {
		mux.Handle(procedure, handler)
		mounted = append(mounted, procedure)
	}
	register(cadestrov1connect.ControlServiceCreateDeviceGroupProcedure, connect.NewUnaryHandler(cadestrov1connect.ControlServiceCreateDeviceGroupProcedure, h.CreateDeviceGroup, opts...))
	register(cadestrov1connect.ControlServiceGetDeviceGroupProcedure, connect.NewUnaryHandler(cadestrov1connect.ControlServiceGetDeviceGroupProcedure, h.GetDeviceGroup, opts...))
	register(cadestrov1connect.ControlServiceListDeviceGroupsProcedure, connect.NewUnaryHandler(cadestrov1connect.ControlServiceListDeviceGroupsProcedure, h.ListDeviceGroups, opts...))
	register(cadestrov1connect.ControlServiceListDeviceGroupsForDeviceProcedure, connect.NewUnaryHandler(cadestrov1connect.ControlServiceListDeviceGroupsForDeviceProcedure, h.ListDeviceGroupsForDevice, opts...))
	register(cadestrov1connect.ControlServiceRenameDeviceGroupProcedure, connect.NewUnaryHandler(cadestrov1connect.ControlServiceRenameDeviceGroupProcedure, h.RenameDeviceGroup, opts...))
	register(cadestrov1connect.ControlServiceUpdateDeviceGroupDescriptionProcedure, connect.NewUnaryHandler(cadestrov1connect.ControlServiceUpdateDeviceGroupDescriptionProcedure, h.UpdateDeviceGroupDescription, opts...))
	register(cadestrov1connect.ControlServiceUpdateDeviceGroupQueryProcedure, connect.NewUnaryHandler(cadestrov1connect.ControlServiceUpdateDeviceGroupQueryProcedure, h.UpdateDeviceGroupQuery, opts...))
	register(cadestrov1connect.ControlServiceDeleteDeviceGroupProcedure, connect.NewUnaryHandler(cadestrov1connect.ControlServiceDeleteDeviceGroupProcedure, h.DeleteDeviceGroup, opts...))
	register(cadestrov1connect.ControlServiceAddDeviceToGroupProcedure, connect.NewUnaryHandler(cadestrov1connect.ControlServiceAddDeviceToGroupProcedure, h.AddDeviceToGroup, opts...))
	register(cadestrov1connect.ControlServiceRemoveDeviceFromGroupProcedure, connect.NewUnaryHandler(cadestrov1connect.ControlServiceRemoveDeviceFromGroupProcedure, h.RemoveDeviceFromGroup, opts...))
	register(cadestrov1connect.ControlServiceValidateDynamicQueryProcedure, connect.NewUnaryHandler(cadestrov1connect.ControlServiceValidateDynamicQueryProcedure, h.ValidateDynamicQuery, opts...))
	register(cadestrov1connect.ControlServiceEvaluateDynamicGroupProcedure, connect.NewUnaryHandler(cadestrov1connect.ControlServiceEvaluateDynamicGroupProcedure, h.EvaluateDynamicGroup, opts...))
	register(cadestrov1connect.ControlServiceSetDeviceGroupSyncIntervalProcedure, connect.NewUnaryHandler(cadestrov1connect.ControlServiceSetDeviceGroupSyncIntervalProcedure, h.SetDeviceGroupSyncInterval, opts...))
	register(cadestrov1connect.ControlServiceSetDeviceGroupInventoryIntervalProcedure, connect.NewUnaryHandler(cadestrov1connect.ControlServiceSetDeviceGroupInventoryIntervalProcedure, h.SetDeviceGroupInventoryInterval, opts...))
	register(cadestrov1connect.ControlServiceSetDeviceGroupMaintenanceWindowProcedure, connect.NewUnaryHandler(cadestrov1connect.ControlServiceSetDeviceGroupMaintenanceWindowProcedure, h.SetDeviceGroupMaintenanceWindow, opts...))
	return mounted
}

func MutationProcedures() []string {
	return []string{
		cadestrov1connect.ControlServiceCreateDeviceGroupProcedure,
		cadestrov1connect.ControlServiceRenameDeviceGroupProcedure,
		cadestrov1connect.ControlServiceUpdateDeviceGroupDescriptionProcedure,
		cadestrov1connect.ControlServiceUpdateDeviceGroupQueryProcedure,
		cadestrov1connect.ControlServiceDeleteDeviceGroupProcedure,
		cadestrov1connect.ControlServiceAddDeviceToGroupProcedure,
		cadestrov1connect.ControlServiceRemoveDeviceFromGroupProcedure,
		cadestrov1connect.ControlServiceEvaluateDynamicGroupProcedure,
		cadestrov1connect.ControlServiceSetDeviceGroupSyncIntervalProcedure,
		cadestrov1connect.ControlServiceSetDeviceGroupInventoryIntervalProcedure,
		cadestrov1connect.ControlServiceSetDeviceGroupMaintenanceWindowProcedure,
	}
}

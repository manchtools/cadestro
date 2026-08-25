package authoring

import (
	"context"

	"connectrpc.com/connect"
	"github.com/oklog/ulid/v2"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/store"
)

func (h *Handlers) CreateAction(ctx context.Context, req *connect.Request[cadestrov1.CreateActionRequest]) (*connect.Response[cadestrov1.CreateActionResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, "CreateAction", ""); err != nil {
		return nil, err
	}
	actionID := ulid.Make().String()
	params, err := h.requestParams(req.Msg, req.Msg.Type, actionID, nil)
	if err != nil {
		return nil, h.actionError(ctx, "validate create action params", err)
	}
	row, err := h.state.CreateAction(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceCreateActionProcedure, "CreateAction"), CreateActionParams{
		Name: req.Msg.Name, Description: req.Msg.Description, CreatedBy: actor.ID,
		ID:   actionID,
		Type: req.Msg.Type, DesiredState: req.Msg.DesiredState, Params: params,
		TimeoutSeconds: req.Msg.TimeoutSeconds, Schedule: req.Msg.Schedule,
	})
	if err != nil {
		return nil, h.actionError(ctx, "create action", err)
	}
	action, err := ActionToProto(row)
	if err != nil {
		return nil, h.internal(ctx, "encode created action", err)
	}
	return connect.NewResponse(&cadestrov1.CreateActionResponse{Action: action}), nil
}

func (h *Handlers) GetAction(ctx context.Context, req *connect.Request[cadestrov1.GetActionRequest]) (*connect.Response[cadestrov1.GetActionResponse], error) {
	if _, err := h.actor(ctx); err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, "GetAction", req.Msg.GetId().GetValue()); err != nil {
		return nil, err
	}
	row, err := h.operatorAction(ctx, req.Msg.GetId().GetValue())
	if err != nil {
		return nil, err
	}
	if err := h.enforceActionReadScope(ctx, req.Msg.GetId().GetValue()); err != nil {
		return nil, err
	}
	action, err := ActionToProto(row)
	if err != nil {
		return nil, h.internal(ctx, "encode action", err)
	}
	return connect.NewResponse(&cadestrov1.GetActionResponse{Action: action}), nil
}

func (h *Handlers) ListActions(ctx context.Context, req *connect.Request[cadestrov1.ListActionsRequest]) (*connect.Response[cadestrov1.ListActionsResponse], error) {
	if _, err := h.actor(ctx); err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, "ListActions", ""); err != nil {
		return nil, err
	}
	if !validPageToken(req.Msg.PageToken) {
		return nil, authoringRPCError(ctx, errInvalidPageToken, connect.CodeInvalidArgument, "invalid page token")
	}
	if _, ok := cadestrov1.ActionType_name[int32(req.Msg.TypeFilter)]; !ok {
		return nil, authoringRPCError(ctx, errValidationFailed, connect.CodeInvalidArgument, "invalid action type filter")
	}
	limit := req.Msg.PageSize
	if limit == 0 {
		limit = defaultAuthoringPageSize
	}
	groupIDs, restricted := auth.ObjectScopeListFilter(ctx)
	filter := store.ActionListFilter{
		AfterID: req.Msg.PageToken, Limit: limit + 1, Type: int32(req.Msg.TypeFilter),
		UnassignedOnly: req.Msg.UnassignedOnly, ScopeRestricted: restricted,
		ScopeGroupIDs: groupIDs,
	}
	rows, err := h.store.ListAuthoringActions(ctx, filter)
	if err != nil {
		return nil, h.internal(ctx, "list actions", err)
	}
	hasMore := len(rows) > int(limit)
	if hasMore {
		rows = rows[:limit]
	}
	countFilter := filter
	countFilter.AfterID = ""
	countFilter.Limit = 0
	total, err := h.store.CountAuthoringActions(ctx, countFilter)
	if err != nil {
		return nil, h.internal(ctx, "count actions", err)
	}
	actions := make([]*cadestrov1.ManagedAction, len(rows))
	for i := range rows {
		actions[i], err = ActionToProto(rows[i])
		if err != nil {
			return nil, h.internal(ctx, "encode listed action", err)
		}
	}
	next := ""
	if hasMore {
		next = rows[len(rows)-1].ID
	}
	return connect.NewResponse(&cadestrov1.ListActionsResponse{
		Actions: actions, NextPageToken: next, TotalCount: boundedCount(total),
	}), nil
}

func (h *Handlers) RenameAction(ctx context.Context, req *connect.Request[cadestrov1.RenameActionRequest]) (*connect.Response[cadestrov1.UpdateActionResponse], error) {
	actor, err := h.mutationActor(ctx, req.Msg.GetId().GetValue(), "RenameAction")
	if err != nil {
		return nil, err
	}
	row, err := h.state.RenameAction(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceRenameActionProcedure, "RenameAction"), req.Msg.GetId().GetValue(), req.Msg.Name, false)
	return h.updatedAction(ctx, "rename action", row, err)
}

func (h *Handlers) UpdateActionDescription(ctx context.Context, req *connect.Request[cadestrov1.UpdateActionDescriptionRequest]) (*connect.Response[cadestrov1.UpdateActionResponse], error) {
	actor, err := h.mutationActor(ctx, req.Msg.GetId().GetValue(), "UpdateActionDescription")
	if err != nil {
		return nil, err
	}
	row, err := h.state.UpdateActionDescription(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceUpdateActionDescriptionProcedure, "UpdateActionDescription"),
		req.Msg.GetId().GetValue(), req.Msg.Description, false)
	return h.updatedAction(ctx, "update action description", row, err)
}

func (h *Handlers) UpdateActionParams(ctx context.Context, req *connect.Request[cadestrov1.UpdateActionParamsRequest]) (*connect.Response[cadestrov1.UpdateActionResponse], error) {
	actor, row, err := h.mutationAction(ctx, req.Msg.GetId().GetValue(), "UpdateActionParams")
	if err != nil {
		return nil, err
	}
	params, err := h.requestParams(req.Msg, cadestrov1.ActionType(row.ActionType), req.Msg.GetId().GetValue(), row.Params)
	if err != nil {
		return nil, h.actionError(ctx, "validate updated action params", err)
	}
	updated, err := h.state.UpdateActionParams(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceUpdateActionParamsProcedure, "UpdateActionParams"), UpdateActionParams{
		ID: req.Msg.GetId().GetValue(), DesiredState: req.Msg.DesiredState, Params: params,
		TimeoutSeconds: req.Msg.TimeoutSeconds, Schedule: req.Msg.Schedule,
	})
	return h.updatedAction(ctx, "update action params", updated, err)
}

func (h *Handlers) DeleteAction(ctx context.Context, req *connect.Request[cadestrov1.DeleteActionRequest]) (*connect.Response[cadestrov1.DeleteActionResponse], error) {
	actor, err := h.mutationActor(ctx, req.Msg.GetId().GetValue(), "DeleteAction")
	if err != nil {
		return nil, err
	}
	if err := h.state.DeleteAction(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceDeleteActionProcedure, "DeleteAction"), req.Msg.GetId().GetValue(), false); err != nil {
		return nil, h.actionError(ctx, "delete action", err)
	}
	return connect.NewResponse(&cadestrov1.DeleteActionResponse{}), nil
}

func (h *Handlers) operatorAction(ctx context.Context, id string) (store.ActionRow, error) {
	row, err := h.store.GetManifestAction(ctx, id)
	if err != nil {
		if store.IsNotFound(err) {
			return store.ActionRow{}, authoringNotFound(ctx, errActionNotFound, "action not found")
		}
		return store.ActionRow{}, h.internal(ctx, "read action", err)
	}
	if row.IsSystem {
		return store.ActionRow{}, authoringNotFound(ctx, errActionNotFound, "action not found")
	}
	return row, nil
}

func (h *Handlers) enforceActionReadScope(ctx context.Context, id string) error {
	visible, err := ActionVisibleToCaller(ctx, h.store, id)
	if err != nil {
		return h.internal(ctx, "resolve action read scope", err)
	}
	if !visible {
		h.logger.Warn("out-of-scope action read denied", "action_id", id)
		return authoringNotFound(ctx, errActionNotFound, "action not found")
	}
	return nil
}

func (h *Handlers) enforceActionWriteScope(ctx context.Context, id string) error {
	callerGroups, restricted := auth.ObjectScopeListFilter(ctx)
	if !restricted {
		return nil
	}
	objectGroups, err := h.directScopeGroups(ctx, "action", id)
	if err != nil {
		return h.internal(ctx, "resolve action write scope", err)
	}
	if !groupsOverlap(callerGroups, objectGroups) {
		h.logger.Warn("out-of-scope action mutation denied", "action_id", id)
		return authoringRPCError(ctx, errPermissionDenied, connect.CodePermissionDenied, "permission denied")
	}
	return nil
}

func (h *Handlers) mutationAction(ctx context.Context, id, permission string) (*auth.UserContext, store.ActionRow, error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, store.ActionRow{}, err
	}
	if err := h.authorize(ctx, permission, id); err != nil {
		return nil, store.ActionRow{}, err
	}
	if err := h.enforceActionWriteScope(ctx, id); err != nil {
		return nil, store.ActionRow{}, err
	}
	row, err := h.store.GetManifestAction(ctx, id)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, store.ActionRow{}, authoringNotFound(ctx, errActionNotFound, "action not found")
		}
		return nil, store.ActionRow{}, h.internal(ctx, "read action mutation target", err)
	}
	if row.IsSystem {
		return nil, store.ActionRow{}, h.actionError(ctx, "reject system action mutation", ErrSystemAction)
	}
	return actor, row, nil
}

func (h *Handlers) mutationActor(ctx context.Context, id, permission string) (*auth.UserContext, error) {
	actor, _, err := h.mutationAction(ctx, id, permission)
	return actor, err
}

func (h *Handlers) updatedAction(ctx context.Context, operation string, row store.ActionRow, err error) (*connect.Response[cadestrov1.UpdateActionResponse], error) {
	if err != nil {
		return nil, h.actionError(ctx, operation, err)
	}
	action, err := ActionToProto(row)
	if err != nil {
		return nil, h.internal(ctx, "encode updated action", err)
	}
	return connect.NewResponse(&cadestrov1.UpdateActionResponse{Action: action}), nil
}

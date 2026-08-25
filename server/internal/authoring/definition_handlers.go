package authoring

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/store"
)

func (h *Handlers) CreateDefinition(ctx context.Context, req *connect.Request[cadestrov1.CreateDefinitionRequest]) (*connect.Response[cadestrov1.CreateDefinitionResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, "CreateDefinition", ""); err != nil {
		return nil, err
	}
	row, err := h.state.CreateDefinition(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceCreateDefinitionProcedure, "CreateDefinition"), CreateDefinitionParams{
		Name: req.Msg.Name, Description: req.Msg.Description,
		CreatedBy: actor.ID, Schedule: req.Msg.Schedule,
	})
	if err != nil {
		return nil, h.definitionError(ctx, "create definition", err)
	}
	definition, err := DefinitionToProto(row, 0)
	if err != nil {
		return nil, h.internal(ctx, "encode created definition", err)
	}
	return connect.NewResponse(&cadestrov1.CreateDefinitionResponse{Definition: definition}), nil
}

func (h *Handlers) GetDefinition(ctx context.Context, req *connect.Request[cadestrov1.GetDefinitionRequest]) (*connect.Response[cadestrov1.GetDefinitionResponse], error) {
	if _, err := h.actor(ctx); err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, "GetDefinition", req.Msg.GetId().GetValue()); err != nil {
		return nil, err
	}
	row, err := h.operatorDefinition(ctx, req.Msg.GetId().GetValue())
	if err != nil {
		return nil, err
	}
	if err := h.enforceDefinitionReadScope(ctx, req.Msg.GetId().GetValue()); err != nil {
		return nil, err
	}
	members, err := h.store.ListDefinitionMembers(ctx, req.Msg.GetId().GetValue())
	if err != nil {
		return nil, h.internal(ctx, "list definition members", err)
	}
	definition, err := DefinitionToProto(row, int64(len(members)))
	if err != nil {
		return nil, h.internal(ctx, "encode definition", err)
	}
	return connect.NewResponse(&cadestrov1.GetDefinitionResponse{
		Definition: definition, Members: DefinitionMembersToProto(members),
	}), nil
}

func (h *Handlers) ListDefinitions(ctx context.Context, req *connect.Request[cadestrov1.ListDefinitionsRequest]) (*connect.Response[cadestrov1.ListDefinitionsResponse], error) {
	if _, err := h.actor(ctx); err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, "ListDefinitions", ""); err != nil {
		return nil, err
	}
	if !validPageToken(req.Msg.PageToken) {
		return nil, authoringRPCError(ctx, errInvalidPageToken, connect.CodeInvalidArgument, "invalid page token")
	}
	limit := req.Msg.PageSize
	if limit == 0 {
		limit = defaultAuthoringPageSize
	}
	groupIDs, restricted := auth.ObjectScopeListFilter(ctx)
	filter := store.DefinitionListFilter{
		AfterID: req.Msg.PageToken, Limit: limit + 1,
		ScopeRestricted: restricted, ScopeGroupIDs: groupIDs,
	}
	views, err := h.store.ListAuthoringDefinitions(ctx, filter)
	if err != nil {
		return nil, h.internal(ctx, "list definitions", err)
	}
	hasMore := len(views) > int(limit)
	if hasMore {
		views = views[:limit]
	}
	total, err := h.store.CountAuthoringDefinitions(ctx, filter)
	if err != nil {
		return nil, h.internal(ctx, "count definitions", err)
	}
	definitions := make([]*cadestrov1.Definition, len(views))
	for i, view := range views {
		definitions[i], err = DefinitionToProto(view.DefinitionRow, view.LiveMemberCount)
		if err != nil {
			return nil, h.internal(ctx, "encode listed definition", err)
		}
	}
	next := ""
	if hasMore {
		next = views[len(views)-1].ID
	}
	return connect.NewResponse(&cadestrov1.ListDefinitionsResponse{
		Definitions: definitions, NextPageToken: next, TotalCount: boundedCount(total),
	}), nil
}

func (h *Handlers) RenameDefinition(ctx context.Context, req *connect.Request[cadestrov1.RenameDefinitionRequest]) (*connect.Response[cadestrov1.UpdateDefinitionResponse], error) {
	actor, err := h.mutationDefinitionActor(ctx, req.Msg.GetId().GetValue(), "RenameDefinition")
	if err != nil {
		return nil, err
	}
	row, err := h.state.RenameDefinition(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceRenameDefinitionProcedure, "RenameDefinition"), req.Msg.GetId().GetValue(), req.Msg.Name)
	return h.updatedDefinition(ctx, "rename definition", row, err)
}

func (h *Handlers) UpdateDefinitionDescription(ctx context.Context, req *connect.Request[cadestrov1.UpdateDefinitionDescriptionRequest]) (*connect.Response[cadestrov1.UpdateDefinitionResponse], error) {
	actor, err := h.mutationDefinitionActor(ctx, req.Msg.GetId().GetValue(), "UpdateDefinitionDescription")
	if err != nil {
		return nil, err
	}
	row, err := h.state.UpdateDefinitionDescription(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceUpdateDefinitionDescriptionProcedure, "UpdateDefinitionDescription"),
		req.Msg.GetId().GetValue(), req.Msg.Description)
	return h.updatedDefinition(ctx, "update definition description", row, err)
}

func (h *Handlers) UpdateDefinitionSchedule(ctx context.Context, req *connect.Request[cadestrov1.UpdateDefinitionScheduleRequest]) (*connect.Response[cadestrov1.UpdateDefinitionResponse], error) {
	actor, err := h.mutationDefinitionActor(ctx, req.Msg.GetId().GetValue(), "UpdateDefinitionSchedule")
	if err != nil {
		return nil, err
	}
	row, err := h.state.UpdateDefinitionSchedule(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceUpdateDefinitionScheduleProcedure, "UpdateDefinitionSchedule"),
		req.Msg.GetId().GetValue(), req.Msg.Schedule)
	return h.updatedDefinition(ctx, "update definition schedule", row, err)
}

func (h *Handlers) DeleteDefinition(ctx context.Context, req *connect.Request[cadestrov1.DeleteDefinitionRequest]) (*connect.Response[cadestrov1.DeleteDefinitionResponse], error) {
	actor, err := h.mutationDefinitionActor(ctx, req.Msg.GetId().GetValue(), "DeleteDefinition")
	if err != nil {
		return nil, err
	}
	if err := h.state.DeleteDefinition(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceDeleteDefinitionProcedure, "DeleteDefinition"), req.Msg.GetId().GetValue()); err != nil {
		return nil, h.definitionError(ctx, "delete definition", err)
	}
	return connect.NewResponse(&cadestrov1.DeleteDefinitionResponse{}), nil
}

func (h *Handlers) AddActionSetToDefinition(ctx context.Context, req *connect.Request[cadestrov1.AddActionSetToDefinitionRequest]) (*connect.Response[cadestrov1.AddActionSetToDefinitionResponse], error) {
	definitionID, actionSetID := req.Msg.GetDefinitionId().GetValue(), req.Msg.GetActionSetId().GetValue()
	actor, err := h.mutationDefinitionActor(ctx, definitionID, "AddActionSetToDefinition")
	if err != nil {
		return nil, err
	}
	if err := h.enforceActionSetReadScope(ctx, actionSetID); err != nil {
		return nil, err
	}
	if _, err := h.operatorActionSet(ctx, actionSetID); err != nil {
		return nil, err
	}
	if err := h.state.AddActionSetToDefinition(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceAddActionSetToDefinitionProcedure, "AddActionSetToDefinition"),
		definitionID, actionSetID, req.Msg.SortOrder); err != nil {
		return nil, h.definitionError(ctx, "add action set to definition", err)
	}
	definition, err := h.definitionResponse(ctx, definitionID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&cadestrov1.AddActionSetToDefinitionResponse{Definition: definition}), nil
}

func (h *Handlers) RemoveActionSetFromDefinition(ctx context.Context, req *connect.Request[cadestrov1.RemoveActionSetFromDefinitionRequest]) (*connect.Response[cadestrov1.RemoveActionSetFromDefinitionResponse], error) {
	definitionID, actionSetID := req.Msg.GetDefinitionId().GetValue(), req.Msg.GetActionSetId().GetValue()
	actor, err := h.mutationDefinitionActor(ctx, definitionID, "RemoveActionSetFromDefinition")
	if err != nil {
		return nil, err
	}
	if err := h.state.RemoveActionSetFromDefinition(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceRemoveActionSetFromDefinitionProcedure, "RemoveActionSetFromDefinition"),
		definitionID, actionSetID); err != nil {
		return nil, h.definitionError(ctx, "remove action set from definition", err)
	}
	definition, err := h.definitionResponse(ctx, definitionID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&cadestrov1.RemoveActionSetFromDefinitionResponse{Definition: definition}), nil
}

func (h *Handlers) ReorderActionSetInDefinition(ctx context.Context, req *connect.Request[cadestrov1.ReorderActionSetInDefinitionRequest]) (*connect.Response[cadestrov1.ReorderActionSetInDefinitionResponse], error) {
	definitionID, actionSetID := req.Msg.GetDefinitionId().GetValue(), req.Msg.GetActionSetId().GetValue()
	actor, err := h.mutationDefinitionActor(ctx, definitionID, "ReorderActionSetInDefinition")
	if err != nil {
		return nil, err
	}
	if err := h.state.ReorderActionSetInDefinition(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceReorderActionSetInDefinitionProcedure, "ReorderActionSetInDefinition"),
		definitionID, actionSetID, req.Msg.NewOrder); err != nil {
		return nil, h.definitionError(ctx, "reorder action set in definition", err)
	}
	definition, err := h.definitionResponse(ctx, definitionID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&cadestrov1.ReorderActionSetInDefinitionResponse{Definition: definition}), nil
}

func (h *Handlers) definitionError(ctx context.Context, operation string, err error) error {
	switch {
	case errors.Is(err, ErrInvalidInput):
		return authoringRPCError(ctx, errValidationFailed, connect.CodeInvalidArgument, "invalid definition")
	case errors.Is(err, ErrDefinitionAlreadyMember):
		return authoringRPCError(ctx, errActionSetAlreadyInDef, connect.CodeAlreadyExists, "action set is already in the definition")
	case errors.Is(err, ErrDefinitionMemberMissing):
		return authoringNotFound(ctx, errDefinitionMemberNotFound, "definition member not found")
	case store.IsNotFound(err):
		return authoringNotFound(ctx, errDefinitionNotFound, "definition not found")
	default:
		return h.internal(ctx, operation, err)
	}
}

func (h *Handlers) operatorDefinition(ctx context.Context, id string) (store.DefinitionRow, error) {
	row, err := h.store.GetManifestDefinition(ctx, id)
	if err != nil {
		if store.IsNotFound(err) {
			return store.DefinitionRow{}, authoringNotFound(ctx, errDefinitionNotFound, "definition not found")
		}
		return store.DefinitionRow{}, h.internal(ctx, "read definition", err)
	}
	return row, nil
}

func (h *Handlers) enforceDefinitionReadScope(ctx context.Context, id string) error {
	callerGroups, restricted := auth.ObjectScopeListFilter(ctx)
	if !restricted {
		return nil
	}
	objectGroups, err := h.directScopeGroups(ctx, "definition", id)
	if err != nil {
		return h.internal(ctx, "resolve definition read scope", err)
	}
	if !groupsOverlap(callerGroups, objectGroups) {
		h.logger.Warn("out-of-scope definition read denied", "definition_id", id)
		return authoringNotFound(ctx, errDefinitionNotFound, "definition not found")
	}
	return nil
}

func (h *Handlers) enforceDefinitionWriteScope(ctx context.Context, id string) error {
	callerGroups, restricted := auth.ObjectScopeListFilter(ctx)
	if !restricted {
		return nil
	}
	objectGroups, err := h.directScopeGroups(ctx, "definition", id)
	if err != nil {
		return h.internal(ctx, "resolve definition write scope", err)
	}
	if !groupsOverlap(callerGroups, objectGroups) {
		h.logger.Warn("out-of-scope definition mutation denied", "definition_id", id)
		return authoringRPCError(ctx, errPermissionDenied, connect.CodePermissionDenied, "permission denied")
	}
	return nil
}

func (h *Handlers) mutationDefinitionActor(ctx context.Context, id, permission string) (*auth.UserContext, error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, permission, id); err != nil {
		return nil, err
	}
	if err := h.enforceDefinitionWriteScope(ctx, id); err != nil {
		return nil, err
	}
	if _, err := h.operatorDefinition(ctx, id); err != nil {
		return nil, err
	}
	return actor, nil
}

func (h *Handlers) updatedDefinition(ctx context.Context, operation string, row store.DefinitionRow, err error) (*connect.Response[cadestrov1.UpdateDefinitionResponse], error) {
	if err != nil {
		return nil, h.definitionError(ctx, operation, err)
	}
	members, err := h.store.ListDefinitionMembers(ctx, row.ID)
	if err != nil {
		return nil, h.internal(ctx, "count updated definition members", err)
	}
	definition, err := DefinitionToProto(row, int64(len(members)))
	if err != nil {
		return nil, h.internal(ctx, "encode updated definition", err)
	}
	return connect.NewResponse(&cadestrov1.UpdateDefinitionResponse{Definition: definition}), nil
}

func (h *Handlers) definitionResponse(ctx context.Context, id string) (*cadestrov1.Definition, error) {
	row, err := h.store.GetManifestDefinition(ctx, id)
	if err != nil {
		return nil, h.definitionError(ctx, "read changed definition", err)
	}
	members, err := h.store.ListDefinitionMembers(ctx, id)
	if err != nil {
		return nil, h.internal(ctx, "count changed definition members", err)
	}
	definition, err := DefinitionToProto(row, int64(len(members)))
	if err != nil {
		return nil, h.internal(ctx, "encode changed definition", err)
	}
	return definition, nil
}

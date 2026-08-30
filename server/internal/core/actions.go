package core

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"
	"github.com/oklog/ulid/v2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

func validateAction(actionType cadestrov1.ActionType, desiredState cadestrov1.DesiredState, timeoutSeconds int32, schedule *cadestrov1.ActionSchedule, packageParams *cadestrov1.PackageParams, updateParams *cadestrov1.UpdateParams, shellParams *cadestrov1.ShellParams) (*cadestrov1.Action, error) {
	action := &cadestrov1.Action{Type: actionType, DesiredState: desiredState, TimeoutSeconds: timeoutSeconds, Schedule: schedule}
	switch actionType {
	case cadestrov1.ActionType_ACTION_TYPE_PACKAGE:
		if packageParams == nil || updateParams != nil || shellParams != nil {
			return nil, errors.New("package action requires package parameters")
		}
		if desiredState != cadestrov1.DesiredState_DESIRED_STATE_PRESENT && desiredState != cadestrov1.DesiredState_DESIRED_STATE_ABSENT {
			return nil, errors.New("package action requires present or absent desired state")
		}
		action.Params = &cadestrov1.Action_Package{Package: packageParams}
	case cadestrov1.ActionType_ACTION_TYPE_UPDATE:
		if updateParams == nil || packageParams != nil || shellParams != nil {
			return nil, errors.New("update action requires update parameters")
		}
		if desiredState != cadestrov1.DesiredState_DESIRED_STATE_PRESENT {
			return nil, errors.New("update action requires present desired state")
		}
		action.Params = &cadestrov1.Action_Update{Update: updateParams}
	case cadestrov1.ActionType_ACTION_TYPE_SHELL:
		if shellParams == nil || packageParams != nil || updateParams != nil {
			return nil, errors.New("shell action requires shell parameters")
		}
		if desiredState != cadestrov1.DesiredState_DESIRED_STATE_PRESENT {
			return nil, errors.New("shell action requires present desired state")
		}
		if shellParams.GetIsCompliance() && shellParams.GetDetectionScript() == "" {
			return nil, errors.New("compliance action requires a detection script")
		}
		if shellParams.GetScript() == "" && shellParams.GetDetectionScript() == "" {
			return nil, errors.New("shell action requires a script or detection script")
		}
		action.Params = &cadestrov1.Action_Shell{Shell: shellParams}
	default:
		return nil, errors.New("unsupported action type")
	}
	return action, nil
}

func actionProto(action *db.Action) (*cadestrov1.ManagedAction, error) {
	executable, err := executableAction(action)
	if err != nil {
		return nil, err
	}
	mapped := &cadestrov1.ManagedAction{
		Id: executable.Id, Name: action.Name, Description: action.Description,
		Type: executable.Type, DesiredState: executable.DesiredState, TimeoutSeconds: executable.TimeoutSeconds,
		Schedule:  executable.Schedule,
		CreatedAt: timestamppb.New(action.CreatedAt), UpdatedAt: timestamppb.New(action.UpdatedAt),
	}
	switch params := executable.Params.(type) {
	case *cadestrov1.Action_Package:
		mapped.Params = &cadestrov1.ManagedAction_Package{Package: params.Package}
	case *cadestrov1.Action_Update:
		mapped.Params = &cadestrov1.ManagedAction_Update{Update: params.Update}
	case *cadestrov1.Action_Shell:
		mapped.Params = &cadestrov1.ManagedAction_Shell{Shell: params.Shell}
	default:
		return nil, errors.New("stored action parameters are missing")
	}
	return mapped, nil
}

func executableAction(action *db.Action) (*cadestrov1.Action, error) {
	result := &cadestrov1.Action{}
	if err := proto.Unmarshal(action.ActionBlob, result); err != nil {
		return nil, err
	}
	if result.GetId().GetValue() != action.ID || result.Type != action.Type {
		return nil, errors.New("stored action metadata does not match action blob")
	}
	return result, nil
}

func createActionParams(request *cadestrov1.CreateActionRequest, action *cadestrov1.Action, now time.Time) (db.CreateActionParams, error) {
	action.Id = &cadestrov1.ActionId{Value: ulid.Make().String()}
	blob, err := proto.Marshal(action)
	if err != nil {
		return db.CreateActionParams{}, err
	}
	return db.CreateActionParams{ID: action.Id.GetValue(), Name: request.GetName(), Description: request.GetDescription(), Type: action.Type, ActionBlob: blob, CreatedAt: now, UpdatedAt: now}, nil
}

func (service *Service) CreateAction(ctx context.Context, request *connect.Request[cadestrov1.CreateActionRequest]) (*connect.Response[cadestrov1.CreateActionResponse], error) {
	actionValue, err := validateAction(request.Msg.GetType(), request.Msg.GetDesiredState(), request.Msg.GetTimeoutSeconds(), request.Msg.GetSchedule(), request.Msg.GetPackage(), request.Msg.GetUpdate(), request.Msg.GetShell())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	params, err := createActionParams(request.Msg, actionValue, service.now().UTC())
	if err != nil {
		return nil, service.internal("encode action", err)
	}
	action, err := service.store.Queries().CreateAction(ctx, params)
	if err != nil {
		if store.IsConflict(err) {
			return nil, rpcConflict("action")
		}
		return nil, service.internal("create action", err)
	}
	if err := service.audit(ctx, "action.created", "action", action.ID, "user", ""); err != nil {
		return nil, service.internal("audit action creation", err)
	}
	mapped, err := actionProto(action)
	if err != nil {
		return nil, service.internal("map action", err)
	}
	return connect.NewResponse(&cadestrov1.CreateActionResponse{Action: mapped}), nil
}

func (service *Service) GetAction(ctx context.Context, request *connect.Request[cadestrov1.GetActionRequest]) (*connect.Response[cadestrov1.GetActionResponse], error) {
	action, err := service.store.Queries().GetAction(ctx, request.Msg.GetId().GetValue())
	if err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("action")
		}
		return nil, service.internal("get action", err)
	}
	mapped, err := actionProto(action)
	if err != nil {
		return nil, service.internal("map action", err)
	}
	return connect.NewResponse(&cadestrov1.GetActionResponse{Action: mapped}), nil
}

func (service *Service) ListActions(ctx context.Context, request *connect.Request[cadestrov1.ListActionsRequest]) (*connect.Response[cadestrov1.ListActionsResponse], error) {
	limit := pageSize(request.Msg.GetPageSize())
	actions, err := service.store.Queries().ListActions(ctx, db.ListActionsParams{AfterID: request.Msg.GetPageToken(), TypeFilter: int64(request.Msg.GetTypeFilter()), PageLimit: limit})
	if err != nil {
		return nil, service.internal("list actions", err)
	}
	total, err := service.store.Queries().CountActions(ctx, int64(request.Msg.GetTypeFilter()))
	if err != nil {
		return nil, service.internal("count actions", err)
	}
	response := &cadestrov1.ListActionsResponse{TotalCount: int32(total), NextPageToken: nextPageToken(actions, limit, func(action *db.Action) string { return action.ID })}
	for _, action := range actions {
		mapped, err := actionProto(action)
		if err != nil {
			return nil, service.internal("map action", err)
		}
		response.Actions = append(response.Actions, mapped)
	}
	return connect.NewResponse(response), nil
}

func (service *Service) RenameAction(ctx context.Context, request *connect.Request[cadestrov1.RenameActionRequest]) (*connect.Response[cadestrov1.UpdateActionResponse], error) {
	action, err := service.store.Queries().RenameAction(ctx, db.RenameActionParams{Name: request.Msg.GetName(), UpdatedAt: service.now().UTC(), ID: request.Msg.GetId().GetValue()})
	return service.actionUpdateResponse(ctx, "rename action", action, err)
}

func (service *Service) UpdateActionDescription(ctx context.Context, request *connect.Request[cadestrov1.UpdateActionDescriptionRequest]) (*connect.Response[cadestrov1.UpdateActionResponse], error) {
	action, err := service.store.Queries().UpdateActionDescription(ctx, db.UpdateActionDescriptionParams{Description: request.Msg.GetDescription(), UpdatedAt: service.now().UTC(), ID: request.Msg.GetId().GetValue()})
	return service.actionUpdateResponse(ctx, "update action description", action, err)
}

func (service *Service) UpdateActionParams(ctx context.Context, request *connect.Request[cadestrov1.UpdateActionParamsRequest]) (*connect.Response[cadestrov1.UpdateActionResponse], error) {
	current, err := service.store.Queries().GetAction(ctx, request.Msg.GetId().GetValue())
	if err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("action")
		}
		return nil, service.internal("get action for update", err)
	}
	actionValue, err := validateAction(current.Type, request.Msg.GetDesiredState(), request.Msg.GetTimeoutSeconds(), request.Msg.GetSchedule(), request.Msg.GetPackage(), request.Msg.GetUpdate(), request.Msg.GetShell())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	actionValue.Id = &cadestrov1.ActionId{Value: current.ID}
	blob, err := proto.Marshal(actionValue)
	if err != nil {
		return nil, service.internal("encode action parameters", err)
	}
	action, err := service.store.Queries().UpdateActionParams(ctx, db.UpdateActionParamsParams{
		ActionBlob: blob, UpdatedAt: service.now().UTC(), ID: current.ID,
	})
	return service.actionUpdateResponse(ctx, "update action parameters", action, err)
}

func (service *Service) actionUpdateResponse(ctx context.Context, operation string, action *db.Action, err error) (*connect.Response[cadestrov1.UpdateActionResponse], error) {
	if err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("action")
		}
		if store.IsConflict(err) {
			return nil, rpcConflict("action")
		}
		return nil, service.internal(operation, err)
	}
	if err := service.audit(ctx, "action.updated", "action", action.ID, "user", ""); err != nil {
		return nil, service.internal("audit action update", err)
	}
	mapped, err := actionProto(action)
	if err != nil {
		return nil, service.internal("map action", err)
	}
	return connect.NewResponse(&cadestrov1.UpdateActionResponse{Action: mapped}), nil
}

func (service *Service) DeleteAction(ctx context.Context, request *connect.Request[cadestrov1.DeleteActionRequest]) (*connect.Response[cadestrov1.DeleteActionResponse], error) {
	id := request.Msg.GetId().GetValue()
	rows, err := service.store.Queries().DeleteAction(ctx, id)
	if err != nil {
		return nil, service.internal("delete action", err)
	}
	if rows == 0 {
		return nil, rpcNotFound("action")
	}
	if err := service.audit(ctx, "action.deleted", "action", id, "user", ""); err != nil {
		return nil, service.internal("audit action deletion", err)
	}
	return connect.NewResponse(&cadestrov1.DeleteActionResponse{}), nil
}

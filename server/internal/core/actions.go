package core

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/oklog/ulid/v2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

func validateAction(desiredState cadestrov1.DesiredState, timeoutSeconds int32, schedule *cadestrov1.ActionSchedule, packageParams *cadestrov1.PackageActionParams, updateParams *cadestrov1.UpdateActionParams, shellParams *cadestrov1.ShellActionParams) (*cadestrov1.Action, error) {
	action := &cadestrov1.Action{DesiredState: desiredState, TimeoutSeconds: timeoutSeconds, Schedule: schedule}
	switch {
	case packageParams != nil:
		if updateParams != nil || shellParams != nil {
			return nil, errors.New("package action requires package parameters")
		}
		if desiredState != cadestrov1.DesiredState_DESIRED_STATE_PRESENT && desiredState != cadestrov1.DesiredState_DESIRED_STATE_ABSENT {
			return nil, errors.New("package action requires present or absent desired state")
		}
		action.Params = &cadestrov1.Action_Package{Package: packageParams}
	case updateParams != nil:
		if packageParams != nil || shellParams != nil {
			return nil, errors.New("update action requires update parameters")
		}
		if desiredState != cadestrov1.DesiredState_DESIRED_STATE_PRESENT {
			return nil, errors.New("update action requires present desired state")
		}
		action.Params = &cadestrov1.Action_Update{Update: updateParams}
	case shellParams != nil:
		if packageParams != nil || updateParams != nil {
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
		return nil, errors.New("action parameters are required")
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
		DesiredState: executable.DesiredState, TimeoutSeconds: executable.TimeoutSeconds,
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
	if result.GetId().GetValue() != action.ID {
		return nil, errors.New("stored action metadata does not match action blob")
	}
	return result, nil
}

func sameActionKind(left, right *cadestrov1.Action) bool {
	switch left.GetParams().(type) {
	case *cadestrov1.Action_Package:
		_, ok := right.GetParams().(*cadestrov1.Action_Package)
		return ok
	case *cadestrov1.Action_Update:
		_, ok := right.GetParams().(*cadestrov1.Action_Update)
		return ok
	case *cadestrov1.Action_Shell:
		_, ok := right.GetParams().(*cadestrov1.Action_Shell)
		return ok
	default:
		return false
	}
}

func createActionParams(request *cadestrov1.CreateActionRequest, action *cadestrov1.Action) (db.CreateActionParams, error) {
	action.Id = &cadestrov1.ActionId{Value: ulid.Make().String()}
	blob, err := proto.Marshal(action)
	if err != nil {
		return db.CreateActionParams{}, err
	}
	return db.CreateActionParams{ID: action.Id.GetValue(), Name: request.GetName(), Description: request.GetDescription(), ActionBlob: blob}, nil
}

func (service *Service) CreateAction(ctx context.Context, request *connect.Request[cadestrov1.CreateActionRequest]) (*connect.Response[cadestrov1.CreateActionResponse], error) {
	actionValue, err := validateAction(request.Msg.GetDesiredState(), request.Msg.GetTimeoutSeconds(), request.Msg.GetSchedule(), request.Msg.GetPackage(), request.Msg.GetUpdate(), request.Msg.GetShell())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	params, err := createActionParams(request.Msg, actionValue)
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
	if err := service.audit(ctx, cadestrov1.AuditEventType_AUDIT_EVENT_TYPE_ACTION_CREATED, cadestrov1.AuditStreamType_AUDIT_STREAM_TYPE_ACTION, action.ID, cadestrov1.AuditActorType_AUDIT_ACTOR_TYPE_USER, ""); err != nil {
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
	actions, err := service.store.Queries().ListActions(ctx, db.ListActionsParams{AfterID: request.Msg.GetPageToken(), PageLimit: limit})
	if err != nil {
		return nil, service.internal("list actions", err)
	}
	total, err := service.store.Queries().CountActions(ctx)
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

func (service *Service) RenameAction(ctx context.Context, request *connect.Request[cadestrov1.RenameActionRequest]) (*connect.Response[cadestrov1.RenameActionResponse], error) {
	action, err := service.store.Queries().RenameAction(ctx, db.RenameActionParams{Name: request.Msg.GetName(), ID: request.Msg.GetId().GetValue()})
	mapped, err := service.actionUpdateResponse(ctx, "rename action", action, err)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&cadestrov1.RenameActionResponse{Action: mapped}), nil
}

func (service *Service) SetActionDescription(ctx context.Context, request *connect.Request[cadestrov1.SetActionDescriptionRequest]) (*connect.Response[cadestrov1.SetActionDescriptionResponse], error) {
	action, err := service.store.Queries().SetActionDescription(ctx, db.SetActionDescriptionParams{Description: request.Msg.GetDescription(), ID: request.Msg.GetId().GetValue()})
	mapped, err := service.actionUpdateResponse(ctx, "set action description", action, err)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&cadestrov1.SetActionDescriptionResponse{Action: mapped}), nil
}

func (service *Service) ConfigureAction(ctx context.Context, request *connect.Request[cadestrov1.ConfigureActionRequest]) (*connect.Response[cadestrov1.ConfigureActionResponse], error) {
	current, err := service.store.Queries().GetAction(ctx, request.Msg.GetId().GetValue())
	if err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("action")
		}
		return nil, service.internal("get action for update", err)
	}
	stored, err := executableAction(current)
	if err != nil {
		return nil, service.internal("decode action for update", err)
	}
	actionValue, err := validateAction(request.Msg.GetDesiredState(), request.Msg.GetTimeoutSeconds(), request.Msg.GetSchedule(), request.Msg.GetPackage(), request.Msg.GetUpdate(), request.Msg.GetShell())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if !sameActionKind(stored, actionValue) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("action kind cannot change"))
	}
	actionValue.Id = &cadestrov1.ActionId{Value: current.ID}
	blob, err := proto.Marshal(actionValue)
	if err != nil {
		return nil, service.internal("encode action parameters", err)
	}
	action, err := service.store.Queries().ConfigureAction(ctx, db.ConfigureActionParams{
		ActionBlob: blob, ID: current.ID,
	})
	mapped, err := service.actionUpdateResponse(ctx, "configure action", action, err)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&cadestrov1.ConfigureActionResponse{Action: mapped}), nil
}

func (service *Service) actionUpdateResponse(ctx context.Context, operation string, action *db.Action, err error) (*cadestrov1.ManagedAction, error) {
	if err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("action")
		}
		if store.IsConflict(err) {
			return nil, rpcConflict("action")
		}
		return nil, service.internal(operation, err)
	}
	if err := service.audit(ctx, cadestrov1.AuditEventType_AUDIT_EVENT_TYPE_ACTION_UPDATED, cadestrov1.AuditStreamType_AUDIT_STREAM_TYPE_ACTION, action.ID, cadestrov1.AuditActorType_AUDIT_ACTOR_TYPE_USER, ""); err != nil {
		return nil, service.internal("audit action update", err)
	}
	mapped, err := actionProto(action)
	if err != nil {
		return nil, service.internal("map action", err)
	}
	return mapped, nil
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
	if err := service.audit(ctx, cadestrov1.AuditEventType_AUDIT_EVENT_TYPE_ACTION_DELETED, cadestrov1.AuditStreamType_AUDIT_STREAM_TYPE_ACTION, id, cadestrov1.AuditActorType_AUDIT_ACTOR_TYPE_USER, ""); err != nil {
		return nil, service.internal("audit action deletion", err)
	}
	return connect.NewResponse(&cadestrov1.DeleteActionResponse{}), nil
}

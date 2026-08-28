package core

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"connectrpc.com/connect"
	"github.com/oklog/ulid/v2"
	"google.golang.org/protobuf/types/known/timestamppb"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

type actionValues struct {
	desiredState          int64
	timeoutSeconds        int64
	intervalHours         int64
	runOnAssign           bool
	skipIfUnchanged       bool
	packageName           string
	packageVersion        string
	shellScript           string
	shellInterpreter      string
	shellWorkingDirectory string
	shellEnvironmentJSON  string
	shellDetectionScript  string
	shellIsCompliance     bool
}

func validateAction(actionType cadestrov1.ActionType, desiredState cadestrov1.DesiredState, timeoutSeconds int32, schedule *cadestrov1.ActionSchedule, packageParams *cadestrov1.PackageParams, updateParams *cadestrov1.UpdateParams, shellParams *cadestrov1.ShellParams) (actionValues, error) {
	values := actionValues{desiredState: int64(desiredState), timeoutSeconds: int64(timeoutSeconds), shellEnvironmentJSON: "{}"}
	if schedule != nil {
		values.intervalHours = int64(schedule.GetIntervalHours())
		values.runOnAssign = schedule.GetRunOnAssign()
		values.skipIfUnchanged = schedule.GetSkipIfUnchanged()
	}
	switch actionType {
	case cadestrov1.ActionType_ACTION_TYPE_PACKAGE:
		if packageParams == nil || updateParams != nil || shellParams != nil {
			return actionValues{}, errors.New("package action requires package parameters")
		}
		if desiredState != cadestrov1.DesiredState_DESIRED_STATE_PRESENT && desiredState != cadestrov1.DesiredState_DESIRED_STATE_ABSENT {
			return actionValues{}, errors.New("package action requires present or absent desired state")
		}
		values.packageName = packageParams.GetName()
		values.packageVersion = packageParams.GetVersion()
	case cadestrov1.ActionType_ACTION_TYPE_UPDATE:
		if updateParams == nil || packageParams != nil || shellParams != nil {
			return actionValues{}, errors.New("update action requires update parameters")
		}
		if desiredState != cadestrov1.DesiredState_DESIRED_STATE_PRESENT {
			return actionValues{}, errors.New("update action requires present desired state")
		}
	case cadestrov1.ActionType_ACTION_TYPE_SHELL:
		if shellParams == nil || packageParams != nil || updateParams != nil {
			return actionValues{}, errors.New("shell action requires shell parameters")
		}
		if desiredState != cadestrov1.DesiredState_DESIRED_STATE_PRESENT {
			return actionValues{}, errors.New("shell action requires present desired state")
		}
		if shellParams.GetIsCompliance() && shellParams.GetDetectionScript() == "" {
			return actionValues{}, errors.New("compliance action requires a detection script")
		}
		if shellParams.GetScript() == "" && shellParams.GetDetectionScript() == "" {
			return actionValues{}, errors.New("shell action requires a script or detection script")
		}
		environment, err := json.Marshal(shellParams.GetEnvironment())
		if err != nil {
			return actionValues{}, errors.New("encode shell environment")
		}
		values.shellScript = shellParams.GetScript()
		values.shellInterpreter = shellParams.GetInterpreter()
		values.shellWorkingDirectory = shellParams.GetWorkingDirectory()
		values.shellEnvironmentJSON = string(environment)
		values.shellDetectionScript = shellParams.GetDetectionScript()
		values.shellIsCompliance = shellParams.GetIsCompliance()
	default:
		return actionValues{}, errors.New("unsupported action type")
	}
	return values, nil
}

func actionProto(action *db.Action) (*cadestrov1.ManagedAction, error) {
	mapped := &cadestrov1.ManagedAction{
		Id: &cadestrov1.ActionId{Value: action.ID}, Name: action.Name, Description: action.Description,
		Type: cadestrov1.ActionType(action.Type), DesiredState: cadestrov1.DesiredState(action.DesiredState), TimeoutSeconds: int32(action.TimeoutSeconds),
		Schedule:  &cadestrov1.ActionSchedule{IntervalHours: int32(action.IntervalHours), RunOnAssign: action.RunOnAssign, SkipIfUnchanged: action.SkipIfUnchanged},
		CreatedAt: timestamppb.New(action.CreatedAt), UpdatedAt: timestamppb.New(action.UpdatedAt),
	}
	switch mapped.Type {
	case cadestrov1.ActionType_ACTION_TYPE_PACKAGE:
		mapped.Params = &cadestrov1.ManagedAction_Package{Package: &cadestrov1.PackageParams{Name: action.PackageName, Version: action.PackageVersion}}
	case cadestrov1.ActionType_ACTION_TYPE_UPDATE:
		mapped.Params = &cadestrov1.ManagedAction_Update{Update: &cadestrov1.UpdateParams{}}
	case cadestrov1.ActionType_ACTION_TYPE_SHELL:
		var environment map[string]string
		if err := json.Unmarshal([]byte(action.ShellEnvironmentJson), &environment); err != nil {
			return nil, err
		}
		mapped.Params = &cadestrov1.ManagedAction_Shell{Shell: &cadestrov1.ShellParams{
			Script: action.ShellScript, Interpreter: action.ShellInterpreter, WorkingDirectory: action.ShellWorkingDirectory,
			Environment: environment, DetectionScript: action.ShellDetectionScript, IsCompliance: action.ShellIsCompliance,
		}}
	default:
		return nil, errors.New("unsupported stored action type")
	}
	return mapped, nil
}

func executableAction(action *db.Action) (*cadestrov1.Action, error) {
	managed, err := actionProto(action)
	if err != nil {
		return nil, err
	}
	result := &cadestrov1.Action{
		Id: managed.Id, Type: managed.Type, DesiredState: managed.DesiredState,
		TimeoutSeconds: managed.TimeoutSeconds, Schedule: managed.Schedule,
	}
	switch params := managed.Params.(type) {
	case *cadestrov1.ManagedAction_Package:
		result.Params = &cadestrov1.Action_Package{Package: params.Package}
	case *cadestrov1.ManagedAction_Update:
		result.Params = &cadestrov1.Action_Update{Update: params.Update}
	case *cadestrov1.ManagedAction_Shell:
		result.Params = &cadestrov1.Action_Shell{Shell: params.Shell}
	}
	return result, nil
}

func createActionParams(request *cadestrov1.CreateActionRequest, values actionValues, now time.Time) db.CreateActionParams {
	return db.CreateActionParams{
		ID: ulid.Make().String(), Name: request.GetName(), Description: request.GetDescription(), Type: int64(request.GetType()),
		DesiredState: values.desiredState, TimeoutSeconds: values.timeoutSeconds, IntervalHours: values.intervalHours,
		RunOnAssign: values.runOnAssign, SkipIfUnchanged: values.skipIfUnchanged, PackageName: values.packageName,
		PackageVersion: values.packageVersion, ShellScript: values.shellScript, ShellInterpreter: values.shellInterpreter,
		ShellWorkingDirectory: values.shellWorkingDirectory, ShellEnvironmentJson: values.shellEnvironmentJSON,
		ShellDetectionScript: values.shellDetectionScript, ShellIsCompliance: values.shellIsCompliance, CreatedAt: now, UpdatedAt: now,
	}
}

func (service *Service) CreateAction(ctx context.Context, request *connect.Request[cadestrov1.CreateActionRequest]) (*connect.Response[cadestrov1.CreateActionResponse], error) {
	values, err := validateAction(request.Msg.GetType(), request.Msg.GetDesiredState(), request.Msg.GetTimeoutSeconds(), request.Msg.GetSchedule(), request.Msg.GetPackage(), request.Msg.GetUpdate(), request.Msg.GetShell())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	params := createActionParams(request.Msg, values, service.now().UTC())
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
	values, err := validateAction(cadestrov1.ActionType(current.Type), request.Msg.GetDesiredState(), request.Msg.GetTimeoutSeconds(), request.Msg.GetSchedule(), request.Msg.GetPackage(), request.Msg.GetUpdate(), request.Msg.GetShell())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	action, err := service.store.Queries().UpdateActionParams(ctx, db.UpdateActionParamsParams{
		DesiredState: values.desiredState, TimeoutSeconds: values.timeoutSeconds, IntervalHours: values.intervalHours,
		RunOnAssign: values.runOnAssign, SkipIfUnchanged: values.skipIfUnchanged, PackageName: values.packageName,
		PackageVersion: values.packageVersion, ShellScript: values.shellScript, ShellInterpreter: values.shellInterpreter,
		ShellWorkingDirectory: values.shellWorkingDirectory, ShellEnvironmentJson: values.shellEnvironmentJSON,
		ShellDetectionScript: values.shellDetectionScript, ShellIsCompliance: values.shellIsCompliance,
		UpdatedAt: service.now().UTC(), ID: current.ID,
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

package dispatch

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/oklog/ulid/v2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	pmv1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/authoring"
	"github.com/manchtools/cadestro/server/internal/manifest"
	"github.com/manchtools/cadestro/server/internal/store"
)

// HandlersConfig supplies the durable store and live-control sender.
type HandlersConfig struct {
	Store  *store.Store
	Sender func(deviceID string, message *pmv1.ServerMessage) error
	Logger *slog.Logger
	Now    func() time.Time
}

// Handlers implements direct manifest dispatch RPCs.
type Handlers struct {
	store     *store.Store
	compiler  *manifest.Compiler
	submitter *Service
	logger    *slog.Logger
	sender    func(deviceID string, message *pmv1.ServerMessage) error
	liveMu    sync.Mutex
	live      map[string]pendingLiveOperation
}

type pendingLiveOperation struct {
	deviceID string
	action   string
	result   chan liveOperationResult
}

type liveOperationResult struct {
	success bool
	err     error
}

const liveOperationTimeout = 20 * time.Second

// NewHandlers constructs direct dispatch handlers.
func NewHandlers(cfg HandlersConfig) *Handlers {
	if cfg.Store == nil {
		panic("dispatch: handler store is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Handlers{
		store: cfg.Store, compiler: manifest.New(cfg.Store),
		submitter: New(Config{Store: cfg.Store, Now: cfg.Now}),
		logger:    cfg.Logger, sender: cfg.Sender,
		live: make(map[string]pendingLiveOperation),
	}
}

func (h *Handlers) actor(ctx context.Context) (*auth.UserContext, error) {
	actor, ok := auth.UserFromContext(ctx)
	if !ok {
		return nil, rpcError(ctx, errNotAuthenticated, connect.CodeUnauthenticated, "not authenticated")
	}
	return actor, nil
}

type deviceScopeResolver struct{ store *store.Store }

func (r deviceScopeResolver) DeviceGroupsForDevice(ctx context.Context, deviceID string) ([]string, error) {
	return r.store.ListDeviceGroupIDs(ctx, deviceID)
}

func (deviceScopeResolver) UserGroupsForUser(context.Context, string) ([]string, error) {
	return nil, errors.New("dispatch: user scope resolution is unavailable")
}

func (h *Handlers) target(ctx context.Context, actor *auth.UserContext, permission, deviceID string) error {
	if !auth.AuthorizeContext(ctx, permission, deviceID) {
		return rpcError(ctx, errPermissionDenied, connect.CodePermissionDenied, "permission denied")
	}
	if _, err := h.store.GetDeviceView(ctx, deviceID); err != nil {
		if store.IsNotFound(err) {
			return notFound(ctx, errDeviceNotFound, "device not found")
		}
		return h.internal(ctx, "read dispatch device", err)
	}
	if auth.HasPermission(ctx, permission) {
		if err := auth.EnforceDeviceScopeOnBaseTier(ctx, deviceScopeResolver{h.store}, permission, deviceID); err != nil {
			if connect.CodeOf(err) == connect.CodeInternal {
				return h.internal(ctx, "resolve dispatch device scope", err)
			}
			return notFound(ctx, errDeviceNotFound, "device not found")
		}
		return nil
	}
	assigned, err := h.store.IsDeviceAssignedToUser(ctx, deviceID, actor.ID)
	if err != nil {
		return h.internal(ctx, "check dispatch device assignment", err)
	}
	if !assigned {
		return notFound(ctx, errDeviceNotFound, "device not found")
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

func (h *Handlers) internal(ctx context.Context, operation string, err error) *connect.Error {
	h.logger.Error("dispatch RPC failed", "operation", operation, "error", err)
	return rpcError(ctx, errInternal, connect.CodeInternal, "internal error")
}

// DispatchAction compiles one catalog or inline Action and durably submits it.
func (h *Handlers) DispatchAction(ctx context.Context, req *connect.Request[pmv1.DispatchActionRequest]) (*connect.Response[pmv1.DispatchActionResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.target(ctx, actor, "DispatchAction", req.Msg.DeviceId); err != nil {
		return nil, err
	}

	var input ManifestInput
	switch source := req.Msg.ActionSource.(type) {
	case *pmv1.DispatchActionRequest_ActionId:
		compiled, err := h.catalogActionTemplate(ctx, source.ActionId)
		if err != nil {
			return nil, err
		}
		input = ManifestInput{Manifest: compiled, PersistActionIDs: true}
	case *pmv1.DispatchActionRequest_InlineAction:
		compiled, err := h.inlineTemplate(ctx, source.InlineAction)
		if err != nil {
			return nil, err
		}
		input = ManifestInput{Manifest: compiled}
	default:
		return nil, rpcError(ctx, errValidationFailed, connect.CodeInvalidArgument,
			"either action_id or inline_action is required")
	}

	scheduledFor, err := futureTime(req.Msg.RunAt)
	if err != nil {
		return nil, rpcError(ctx, errValidationFailed, connect.CodeInvalidArgument, "run_at must be a valid future timestamp")
	}
	result, err := h.submitter.Submit(ctx, SubmitParams{
		Operation: h.operation(req, actor, cadestrov1connect.ControlServiceDispatchActionProcedure, "DispatchAction"),
		DeviceID:  req.Msg.DeviceId, Manifests: []ManifestInput{input}, ScheduledFor: scheduledFor,
	})
	if err != nil {
		if errors.Is(err, ErrInvalidInput) {
			return nil, rpcError(ctx, errValidationFailed, connect.CodeInvalidArgument, "invalid dispatch")
		}
		return nil, h.internal(ctx, "submit action dispatch", err)
	}
	return connect.NewResponse(&pmv1.DispatchActionResponse{
		Execution: createdExecutionToProto(result.Executions[0]),
	}), nil
}

// DispatchActionSet compiles the set once and submits its ordered occurrences
// as one complete delivery.
func (h *Handlers) DispatchActionSet(ctx context.Context, req *connect.Request[pmv1.DispatchActionSetRequest]) (*connect.Response[pmv1.DispatchActionSetResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.target(ctx, actor, "DispatchActionSet", req.Msg.DeviceId); err != nil {
		return nil, err
	}
	compiled, err := h.catalogActionSetTemplate(ctx, req.Msg.ActionSetId)
	if err != nil {
		return nil, err
	}
	result, err := h.submitter.Submit(ctx, SubmitParams{
		Operation: h.operation(req, actor, cadestrov1connect.ControlServiceDispatchActionSetProcedure, "DispatchActionSet"),
		DeviceID:  req.Msg.DeviceId, Manifests: catalogManifests(compiled),
	})
	if err != nil {
		return nil, h.submitError(ctx, "submit action set dispatch", err)
	}
	return connect.NewResponse(&pmv1.DispatchActionSetResponse{
		Executions: createdExecutionsToProto(result.Executions),
	}), nil
}

// DispatchDefinition compiles one globally ordered runbook and commits it
// atomically without mutating any authored schedule.
func (h *Handlers) DispatchDefinition(ctx context.Context, req *connect.Request[pmv1.DispatchDefinitionRequest]) (*connect.Response[pmv1.DispatchDefinitionResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.target(ctx, actor, "DispatchDefinition", req.Msg.DeviceId); err != nil {
		return nil, err
	}
	compiled, err := h.catalogDefinitionTemplate(ctx, req.Msg.DefinitionId)
	if err != nil {
		return nil, err
	}
	result, err := h.submitter.Submit(ctx, SubmitParams{
		Operation: h.operation(req, actor, cadestrov1connect.ControlServiceDispatchDefinitionProcedure, "DispatchDefinition"),
		DeviceID:  req.Msg.DeviceId, Manifests: catalogManifests(compiled),
	})
	if err != nil {
		return nil, h.submitError(ctx, "submit definition dispatch", err)
	}
	return connect.NewResponse(&pmv1.DispatchDefinitionResponse{
		Executions: createdExecutionsToProto(result.Executions),
	}), nil
}

// DispatchToMultiple atomically submits one Action to every named device.
func (h *Handlers) DispatchToMultiple(ctx context.Context, req *connect.Request[pmv1.DispatchToMultipleRequest]) (*connect.Response[pmv1.DispatchToMultipleResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if hasDuplicateIDs(req.Msg.DeviceIds) {
		return nil, rpcError(ctx, errValidationFailed, connect.CodeInvalidArgument, "device_ids contains duplicates")
	}
	for _, deviceID := range req.Msg.DeviceIds {
		if err := h.target(ctx, actor, "DispatchToMultiple", deviceID); err != nil {
			return nil, err
		}
	}

	var targets []TargetInput
	switch source := req.Msg.ActionSource.(type) {
	case *pmv1.DispatchToMultipleRequest_ActionId:
		compiled, err := h.catalogActionTemplate(ctx, source.ActionId)
		if err != nil {
			return nil, err
		}
		targets, err = freshTargets(req.Msg.DeviceIds, []*pmv1.Manifest{compiled}, true)
		if err != nil {
			return nil, h.internal(ctx, "prepare multi-device dispatch", err)
		}
	case *pmv1.DispatchToMultipleRequest_InlineAction:
		compiled, err := h.inlineTemplate(ctx, source.InlineAction)
		if err != nil {
			return nil, err
		}
		targets, err = freshTargets(req.Msg.DeviceIds, []*pmv1.Manifest{compiled}, false)
		if err != nil {
			return nil, h.internal(ctx, "prepare multi-device dispatch", err)
		}
	default:
		return nil, rpcError(ctx, errValidationFailed, connect.CodeInvalidArgument,
			"either action_id or inline_action is required")
	}
	result, err := h.submitter.SubmitBatch(ctx, SubmitBatchParams{
		Operation: h.operation(req, actor, cadestrov1connect.ControlServiceDispatchToMultipleProcedure, "DispatchToMultiple"),
		Targets:   targets,
	})
	if err != nil {
		return nil, h.submitError(ctx, "submit multi-device dispatch", err)
	}
	return connect.NewResponse(&pmv1.DispatchToMultipleResponse{
		Executions: createdExecutionsToProto(result.Executions),
	}), nil
}

// DispatchToGroup snapshots the group's live members, then atomically submits
// one freshly identified copy of the selected source to every member.
func (h *Handlers) DispatchToGroup(ctx context.Context, req *connect.Request[pmv1.DispatchToGroupRequest]) (*connect.Response[pmv1.DispatchToGroupResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if !auth.AuthorizeContext(ctx, "DispatchToGroup", req.Msg.GroupId) {
		return nil, rpcError(ctx, errPermissionDenied, connect.CodePermissionDenied, "permission denied")
	}
	if req.Msg.ActionSource == nil {
		return nil, rpcError(ctx, errValidationFailed, connect.CodeInvalidArgument, "an action source is required")
	}
	if _, err := h.store.GetDeviceGroup(ctx, req.Msg.GroupId); err != nil {
		if store.IsNotFound(err) {
			return nil, notFound(ctx, errDeviceGroupMissing, "device group not found")
		}
		return nil, h.internal(ctx, "read dispatch device group", err)
	}
	if groups, restricted := auth.DeviceScopeListFilter(ctx, "DispatchToGroup"); restricted && !containsID(groups, req.Msg.GroupId) {
		return nil, notFound(ctx, errDeviceGroupMissing, "device group not found")
	}
	members, err := h.store.ListDeviceGroupMembers(ctx, req.Msg.GroupId)
	if err != nil {
		return nil, h.internal(ctx, "list dispatch group members", err)
	}
	deviceIDs := make([]string, len(members))
	if len(members) == 0 && !auth.HasPermission(ctx, "DispatchToGroup") {
		return nil, notFound(ctx, errDeviceGroupMissing, "device group not found")
	}
	for i, member := range members {
		deviceIDs[i] = member.DeviceID
		if err := h.target(ctx, actor, "DispatchToGroup", member.DeviceID); err != nil {
			return nil, err
		}
	}

	op := h.operation(req, actor, cadestrov1connect.ControlServiceDispatchToGroupProcedure, "DispatchToGroup")
	if len(deviceIDs) == 0 {
		_, err := h.store.RecordOperation(ctx, op, store.AuditEffect{
			ResourceType: "device_group", ResourceID: req.Msg.GroupId,
			Action: "DISPATCH", Outcome: store.EffectApplied,
		})
		if err != nil {
			return nil, h.internal(ctx, "audit empty group dispatch", err)
		}
		return connect.NewResponse(&pmv1.DispatchToGroupResponse{}), nil
	}
	var templates []*pmv1.Manifest
	persistActionIDs := true
	switch source := req.Msg.ActionSource.(type) {
	case *pmv1.DispatchToGroupRequest_ActionId:
		compiled, err := h.catalogActionTemplate(ctx, source.ActionId)
		if err != nil {
			return nil, err
		}
		templates = []*pmv1.Manifest{compiled}
	case *pmv1.DispatchToGroupRequest_ActionSetId:
		compiled, err := h.catalogActionSetTemplate(ctx, source.ActionSetId)
		if err != nil {
			return nil, err
		}
		templates = []*pmv1.Manifest{compiled}
	case *pmv1.DispatchToGroupRequest_DefinitionId:
		compiled, err := h.catalogDefinitionTemplate(ctx, source.DefinitionId)
		if err != nil {
			return nil, err
		}
		templates = []*pmv1.Manifest{compiled}
	case *pmv1.DispatchToGroupRequest_InlineAction:
		compiled, err := h.inlineTemplate(ctx, source.InlineAction)
		if err != nil {
			return nil, err
		}
		templates = []*pmv1.Manifest{compiled}
		persistActionIDs = false
	default:
		return nil, rpcError(ctx, errValidationFailed, connect.CodeInvalidArgument, "an action source is required")
	}
	targets, err := freshTargets(deviceIDs, templates, persistActionIDs)
	if err != nil {
		return nil, h.internal(ctx, "prepare group dispatch", err)
	}
	result, err := h.submitter.SubmitBatch(ctx, SubmitBatchParams{Operation: op, Targets: targets})
	if err != nil {
		return nil, h.submitError(ctx, "submit group dispatch", err)
	}
	return connect.NewResponse(&pmv1.DispatchToGroupResponse{
		Executions: createdExecutionsToProto(result.Executions),
	}), nil
}

// SyncDevice asks a connected agent to run its normal full Sync.
func (h *Handlers) SyncDevice(ctx context.Context, req *connect.Request[pmv1.SyncDeviceRequest]) (*connect.Response[pmv1.SyncDeviceResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.target(ctx, actor, "SyncDevice", req.Msg.DeviceId); err != nil {
		return nil, err
	}
	if err := h.dispatchLiveOperation(ctx, req, actor, req.Msg.DeviceId, "SYNC",
		cadestrov1connect.ControlServiceSyncDeviceProcedure, "SyncDevice",
		&pmv1.ServerMessage{Payload: &pmv1.ServerMessage_SyncDevice{SyncDevice: &pmv1.SyncDeviceCommand{}}}); err != nil {
		return nil, err
	}
	return connect.NewResponse(&pmv1.SyncDeviceResponse{}), nil
}

// RebootDevice asks a connected agent to schedule its safe delayed reboot.
func (h *Handlers) RebootDevice(ctx context.Context, req *connect.Request[pmv1.RebootDeviceRequest]) (*connect.Response[pmv1.RebootDeviceResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.target(ctx, actor, "RebootDevice", req.Msg.DeviceId); err != nil {
		return nil, err
	}
	if err := h.dispatchLiveOperation(ctx, req, actor, req.Msg.DeviceId, "REBOOT",
		cadestrov1connect.ControlServiceRebootDeviceProcedure, "RebootDevice",
		&pmv1.ServerMessage{Payload: &pmv1.ServerMessage_RebootDevice{RebootDevice: &pmv1.RebootDeviceCommand{}}}); err != nil {
		return nil, err
	}
	return connect.NewResponse(&pmv1.RebootDeviceResponse{}), nil
}

func (h *Handlers) dispatchLiveOperation(ctx context.Context, req connect.AnyRequest, actor *auth.UserContext,
	deviceID, action, procedure, permission string, message *pmv1.ServerMessage) error {
	op := h.operation(req, actor, procedure, permission)
	op.OperationID = ulid.Make().String()
	if _, err := h.store.RecordOperation(ctx, op, store.AuditEffect{
		ResourceType: "device", ResourceID: deviceID, Action: action + "_REQUEST", Outcome: store.EffectApplied,
	}); err != nil {
		return h.internal(ctx, "audit live operation request", err)
	}

	wait := make(chan liveOperationResult, 1)
	h.liveMu.Lock()
	h.live[op.OperationID] = pendingLiveOperation{deviceID: deviceID, action: action, result: wait}
	h.liveMu.Unlock()
	defer h.removeLiveOperation(op.OperationID)

	message.Id = op.OperationID
	if h.sender == nil || h.sender(deviceID, message) != nil {
		h.removeLiveOperation(op.OperationID)
		if _, err := h.store.WithAuditEffects(ctx, op.OperationID, func(_ context.Context, _ *store.Tx, rec *store.AuditRecorder) error {
			rec.Effect(store.AuditEffect{ResourceType: "device", ResourceID: deviceID, Action: action, Outcome: store.EffectFailed})
			return nil
		}); err != nil {
			return h.internal(ctx, "audit live operation send failure", err)
		}
		return rpcError(ctx, errDeviceUnavailable, connect.CodeUnavailable, "device is unavailable")
	}

	timer := time.NewTimer(liveOperationTimeout)
	defer timer.Stop()
	select {
	case result := <-wait:
		if result.err != nil {
			return h.internal(ctx, "complete live operation", result.err)
		}
		if !result.success {
			return rpcError(ctx, errValidationFailed, connect.CodeFailedPrecondition, "device rejected live operation")
		}
		return nil
	case <-timer.C:
		if h.takeLiveOperation(op.OperationID) {
			auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			if _, err := h.store.WithAuditEffects(auditCtx, op.OperationID, func(_ context.Context, _ *store.Tx, rec *store.AuditRecorder) error {
				rec.Effect(store.AuditEffect{ResourceType: "device", ResourceID: deviceID, Action: action, Outcome: store.EffectFailed})
				return nil
			}); err != nil {
				return h.internal(ctx, "audit live operation timeout", err)
			}
			return rpcError(ctx, errDeviceUnavailable, connect.CodeDeadlineExceeded, "device did not answer live operation")
		}
		return rpcError(ctx, errDeviceUnavailable, connect.CodeDeadlineExceeded, "device did not answer live operation")
	case <-ctx.Done():
		if h.takeLiveOperation(op.OperationID) {
			auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_, _ = h.store.WithAuditEffects(auditCtx, op.OperationID, func(_ context.Context, _ *store.Tx, rec *store.AuditRecorder) error {
				rec.Effect(store.AuditEffect{ResourceType: "device", ResourceID: deviceID, Action: action, Outcome: store.EffectFailed})
				return nil
			})
		}
		return connect.NewError(connect.CodeOf(ctx.Err()), ctx.Err())
	}
}

func (h *Handlers) removeLiveOperation(operationID string) {
	h.liveMu.Lock()
	delete(h.live, operationID)
	h.liveMu.Unlock()
}

func (h *Handlers) takeLiveOperation(operationID string) bool {
	h.liveMu.Lock()
	defer h.liveMu.Unlock()
	if _, ok := h.live[operationID]; !ok {
		return false
	}
	delete(h.live, operationID)
	return true
}

func (h *Handlers) completeLiveOperation(ctx context.Context, deviceID, operationID, action string, success bool) error {
	h.liveMu.Lock()
	pending, ok := h.live[operationID]
	if ok && (pending.deviceID != deviceID || pending.action != action) {
		h.liveMu.Unlock()
		return connect.NewError(connect.CodePermissionDenied, errors.New("live operation belongs to another device"))
	}
	if ok {
		delete(h.live, operationID)
	}
	h.liveMu.Unlock()
	if !ok {
		return nil
	}

	outcome := store.EffectApplied
	if !success {
		outcome = store.EffectFailed
	}
	_, err := h.store.WithAuditEffects(ctx, operationID, func(_ context.Context, _ *store.Tx, rec *store.AuditRecorder) error {
		rec.Effect(store.AuditEffect{ResourceType: "device", ResourceID: deviceID, Action: action, Outcome: outcome})
		return nil
	})
	pending.result <- liveOperationResult{success: success, err: err}
	return err
}

func (h *Handlers) CompleteSyncDevice(ctx context.Context, deviceID, operationID string, result *pmv1.SyncDeviceResult) error {
	if result == nil {
		return errors.New("sync device result is required")
	}
	return h.completeLiveOperation(ctx, deviceID, operationID, "SYNC", result.GetSuccess())
}

func (h *Handlers) CompleteRebootDevice(ctx context.Context, deviceID, operationID string, result *pmv1.RebootDeviceResult) error {
	if result == nil {
		return errors.New("reboot device result is required")
	}
	return h.completeLiveOperation(ctx, deviceID, operationID, "REBOOT", result.GetSuccess())
}

func (h *Handlers) compileError(ctx context.Context, operation string, err error) error {
	switch {
	case store.IsNotFound(err):
		return notFound(ctx, errActionNotFound, "action not found")
	case errors.Is(err, manifest.ErrInvalidInput), errors.Is(err, manifest.ErrEmptyManifest):
		return rpcError(ctx, errValidationFailed, connect.CodeInvalidArgument, "invalid action")
	default:
		return h.internal(ctx, operation, err)
	}
}

func (h *Handlers) collectionCompileError(ctx context.Context, resource, code, operation string, err error) error {
	switch {
	case store.IsNotFound(err):
		return notFound(ctx, code, resource+" not found")
	case errors.Is(err, manifest.ErrInvalidInput):
		return rpcError(ctx, errValidationFailed, connect.CodeInvalidArgument, "invalid "+resource)
	case errors.Is(err, manifest.ErrEmptyManifest):
		return rpcError(ctx, errValidationFailed, connect.CodeFailedPrecondition, resource+" contains no executable actions")
	default:
		return h.internal(ctx, operation, err)
	}
}

func (h *Handlers) submitError(ctx context.Context, operation string, err error) error {
	if errors.Is(err, ErrInvalidInput) {
		return rpcError(ctx, errValidationFailed, connect.CodeInvalidArgument, "invalid dispatch")
	}
	return h.internal(ctx, operation, err)
}

func catalogManifests(manifests ...*pmv1.Manifest) []ManifestInput {
	inputs := make([]ManifestInput, len(manifests))
	for i, compiled := range manifests {
		inputs[i] = ManifestInput{Manifest: compiled, PersistActionIDs: true}
	}
	return inputs
}

func (h *Handlers) catalogActionTemplate(ctx context.Context, actionID string) (*pmv1.Manifest, error) {
	compiled, err := h.compiler.Action(ctx, actionID)
	if err != nil {
		return nil, h.compileError(ctx, "compile dispatched action", err)
	}
	visible, err := authoring.ActionVisibleToCaller(ctx, h.store, actionID)
	if err != nil {
		return nil, h.internal(ctx, "resolve dispatched action scope", err)
	}
	if !visible {
		return nil, notFound(ctx, errActionNotFound, "action not found")
	}
	return manifest.AsOneShot(compiled), nil
}

func (h *Handlers) catalogActionSetTemplate(ctx context.Context, setID string) (*pmv1.Manifest, error) {
	compiled, err := h.compiler.ActionSet(ctx, setID)
	if err != nil {
		return nil, h.collectionCompileError(ctx, "action set", errActionSetMissing, "compile dispatched action set", err)
	}
	visible, err := authoring.ActionSetVisibleToCaller(ctx, h.store, setID)
	if err != nil {
		return nil, h.internal(ctx, "resolve dispatched action set scope", err)
	}
	if !visible {
		return nil, notFound(ctx, errActionSetMissing, "action set not found")
	}
	return manifest.AsOneShot(compiled), nil
}

func (h *Handlers) catalogDefinitionTemplate(ctx context.Context, definitionID string) (*pmv1.Manifest, error) {
	compiled, err := h.compiler.Definition(ctx, definitionID)
	if err != nil {
		return nil, h.collectionCompileError(ctx, "definition", errDefinitionMissing, "compile dispatched definition", err)
	}
	visible, err := authoring.DefinitionVisibleToCaller(ctx, h.store, definitionID)
	if err != nil {
		return nil, h.internal(ctx, "resolve dispatched definition scope", err)
	}
	if !visible {
		return nil, notFound(ctx, errDefinitionMissing, "definition not found")
	}
	return manifest.AsOneShot(compiled), nil
}

func (h *Handlers) inlineTemplate(ctx context.Context, action *pmv1.Action) (*pmv1.Manifest, error) {
	if action == nil {
		return nil, rpcError(ctx, errValidationFailed, connect.CodeInvalidArgument, "invalid inline action")
	}
	if action.Type == pmv1.ActionType_ACTION_TYPE_ENCRYPTION || action.Type == pmv1.ActionType_ACTION_TYPE_WIFI {
		return nil, rpcError(ctx, errValidationFailed, connect.CodeInvalidArgument,
			"credential-bearing actions must be authored before dispatch")
	}
	inline := proto.Clone(action).(*pmv1.Action)
	if inline.TimeoutSeconds == 0 {
		inline.TimeoutSeconds = 300
	}
	if err := authoring.ValidateExecutableAction(inline); err != nil {
		return nil, rpcError(ctx, errValidationFailed, connect.CodeInvalidArgument, "invalid inline action")
	}
	compiled, err := manifest.OneShotAction(inline)
	if err != nil {
		return nil, rpcError(ctx, errValidationFailed, connect.CodeInvalidArgument, "invalid inline action")
	}
	return compiled, nil
}

func freshTargets(deviceIDs []string, templates []*pmv1.Manifest, persistActionIDs bool) ([]TargetInput, error) {
	targets := make([]TargetInput, len(deviceIDs))
	for deviceIndex, deviceID := range deviceIDs {
		inputs := make([]ManifestInput, len(templates))
		for manifestIndex, template := range templates {
			compiled, err := manifest.FreshCopy(template)
			if err != nil {
				return nil, err
			}
			inputs[manifestIndex] = ManifestInput{Manifest: compiled, PersistActionIDs: persistActionIDs}
		}
		targets[deviceIndex] = TargetInput{DeviceID: deviceID, Manifests: inputs}
	}
	return targets, nil
}

func hasDuplicateIDs(ids []string) bool {
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, duplicate := seen[id]; duplicate {
			return true
		}
		seen[id] = struct{}{}
	}
	return false
}

func containsID(ids []string, wanted string) bool {
	for _, id := range ids {
		if id == wanted {
			return true
		}
	}
	return false
}

func futureTime(value *timestamppb.Timestamp) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	if err := value.CheckValid(); err != nil {
		return nil, err
	}
	result := value.AsTime().UTC()
	return &result, nil
}

func createdExecutionToProto(row store.ExecutionView) *pmv1.ActionExecution {
	status := pmv1.ExecutionStatus_EXECUTION_STATUS_PENDING
	if row.Status == "scheduled" {
		status = pmv1.ExecutionStatus_EXECUTION_STATUS_SCHEDULED
	}
	result := &pmv1.ActionExecution{
		Id: row.ID, DeviceId: row.DeviceID, Type: pmv1.ActionType(row.ActionType),
		DesiredState: pmv1.DesiredState(row.DesiredState), Status: status,
		CreatedBy: row.CreatedByID,
	}
	if row.ActionID != nil {
		result.ActionId = *row.ActionID
	}
	if row.CreatedAt != nil {
		result.CreatedAt = timestamppb.New(*row.CreatedAt)
	}
	if row.ScheduledFor != nil {
		result.ScheduledFor = timestamppb.New(*row.ScheduledFor)
	}
	return result
}

func createdExecutionsToProto(rows []store.ExecutionView) []*pmv1.ActionExecution {
	result := make([]*pmv1.ActionExecution, len(rows))
	for i := range rows {
		result[i] = createdExecutionToProto(rows[i])
	}
	return result
}

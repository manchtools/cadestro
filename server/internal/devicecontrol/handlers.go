package devicecontrol

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/oklog/ulid/v2"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/manifest"
	"github.com/manchtools/cadestro/server/internal/store"
)

// HandlersConfig supplies the durable store and live-control sender.
type HandlersConfig struct {
	Store  *store.Store
	Sender func(deviceID string, message *cadestrov1.ServerMessage) error
	Logger *slog.Logger
	Now    func() time.Time
}

// Handlers implements live device-control RPCs.
type Handlers struct {
	store    *store.Store
	compiler *manifest.Compiler
	logger   *slog.Logger
	sender   func(deviceID string, message *cadestrov1.ServerMessage) error
	liveMu   sync.Mutex
	live     map[string]pendingLiveOperation
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

// NewHandlers constructs live device-control handlers.
func NewHandlers(cfg HandlersConfig) *Handlers {
	if cfg.Store == nil {
		panic("devicecontrol: handler store is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Handlers{
		store: cfg.Store, compiler: manifest.New(cfg.Store),
		logger: cfg.Logger, sender: cfg.Sender,
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
	return nil, errors.New("devicecontrol: user scope resolution is unavailable")
}

func (h *Handlers) target(ctx context.Context, actor *auth.UserContext, permission, deviceID string) error {
	if !auth.AuthorizeContext(ctx, permission, deviceID) {
		return rpcError(ctx, errPermissionDenied, connect.CodePermissionDenied, "permission denied")
	}
	if _, err := h.store.GetDeviceView(ctx, deviceID); err != nil {
		if store.IsNotFound(err) {
			return notFound(ctx, errDeviceNotFound, "device not found")
		}
		return h.internal(ctx, "read device control target", err)
	}
	if auth.HasPermission(ctx, permission) {
		if err := auth.EnforceDeviceScopeOnBaseTier(ctx, deviceScopeResolver{h.store}, permission, deviceID); err != nil {
			if connect.CodeOf(err) == connect.CodeInternal {
				return h.internal(ctx, "resolve device control scope", err)
			}
			return notFound(ctx, errDeviceNotFound, "device not found")
		}
		return nil
	}
	assigned, err := h.store.IsDeviceAssignedToUser(ctx, deviceID, actor.ID)
	if err != nil {
		return h.internal(ctx, "check device control assignment", err)
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
	h.logger.Error("device control RPC failed", "operation", operation, "error", err)
	return rpcError(ctx, errInternal, connect.CodeInternal, "internal error")
}

// SyncDevice asks a connected agent to run its normal full Sync.
func (h *Handlers) SyncDevice(ctx context.Context, req *connect.Request[cadestrov1.SyncDeviceRequest]) (*connect.Response[cadestrov1.SyncDeviceResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.target(ctx, actor, "SyncDevice", req.Msg.DeviceId); err != nil {
		return nil, err
	}
	if err := h.liveControlOperation(ctx, req, actor, req.Msg.DeviceId, "SYNC",
		cadestrov1connect.ControlServiceSyncDeviceProcedure, "SyncDevice",
		&cadestrov1.ServerMessage{Payload: &cadestrov1.ServerMessage_SyncDevice{SyncDevice: &cadestrov1.SyncDeviceCommand{}}}); err != nil {
		return nil, err
	}
	return connect.NewResponse(&cadestrov1.SyncDeviceResponse{}), nil
}

// RebootDevice asks a connected agent to schedule its safe delayed reboot.
func (h *Handlers) RebootDevice(ctx context.Context, req *connect.Request[cadestrov1.RebootDeviceRequest]) (*connect.Response[cadestrov1.RebootDeviceResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.target(ctx, actor, "RebootDevice", req.Msg.DeviceId); err != nil {
		return nil, err
	}
	if err := h.liveControlOperation(ctx, req, actor, req.Msg.DeviceId, "REBOOT",
		cadestrov1connect.ControlServiceRebootDeviceProcedure, "RebootDevice",
		&cadestrov1.ServerMessage{Payload: &cadestrov1.ServerMessage_RebootDevice{RebootDevice: &cadestrov1.RebootDeviceCommand{}}}); err != nil {
		return nil, err
	}
	return connect.NewResponse(&cadestrov1.RebootDeviceResponse{}), nil
}

func (h *Handlers) liveControlOperation(ctx context.Context, req connect.AnyRequest, actor *auth.UserContext,
	deviceID, action, procedure, permission string, message *cadestrov1.ServerMessage) error {
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

func (h *Handlers) CompleteSyncDevice(ctx context.Context, deviceID, operationID string, result *cadestrov1.SyncDeviceResult) error {
	if result == nil {
		return errors.New("sync device result is required")
	}
	return h.completeLiveOperation(ctx, deviceID, operationID, "SYNC", result.GetSuccess())
}

func (h *Handlers) CompleteRebootDevice(ctx context.Context, deviceID, operationID string, result *cadestrov1.RebootDeviceResult) error {
	if result == nil {
		return errors.New("reboot device result is required")
	}
	return h.completeLiveOperation(ctx, deviceID, operationID, "REBOOT", result.GetSuccess())
}

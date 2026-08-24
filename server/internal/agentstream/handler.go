// Package agentstream terminates the authenticated device connection directly
// in control. Frames are applied to SQLite-backed services without a relay,
// broker, or application-signature layer.
package agentstream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"github.com/oklog/ulid/v2"
	"google.golang.org/protobuf/types/known/durationpb"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/connection"
	"github.com/manchtools/cadestro/server/internal/mtls"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

type frameClass string

const (
	frameState     frameClass = "state"
	frameHello     frameClass = "hello"
	frameTelemetry frameClass = "telemetry"
	frameAudit     frameClass = "audit"
	frameBulk      frameClass = "bulk"
	frameTerminal  frameClass = "terminal"
)

const frameRateWindow = time.Minute

var errCertificateNotActive = errors.New("certificate is not active for device")

// DeviceResults is the direct sink for device-owned result frames.
type DeviceResults interface {
	CompleteOSQueryResult(context.Context, string, *cadestrov1.OSQueryResult) error
	CompleteLogQueryResult(context.Context, string, *cadestrov1.LogQueryResult) error
	StoreDeviceInventory(context.Context, string, *cadestrov1.DeviceInventory) error
	CompleteLuksKeyRevocation(context.Context, string, *cadestrov1.RevokeLuksDeviceKeyResult) error
}

// PolicyResults records assignment manifest results.
type PolicyResults interface {
	RecordPolicyManifestResult(context.Context, string, string, string, string, string) error
}

// ExecutionResults commits per-occurrence results and streamed output.
type ExecutionResults interface {
	ApplyActionResult(context.Context, string, *cadestrov1.ActionResult) error
}

// Secrets owns the narrow feature sinks for LUKS and LPS fields.
type Secrets interface {
	ValidateLuksToken(context.Context, string, *cadestrov1.ValidateLuksTokenRequest) (*cadestrov1.ValidateLuksTokenResponse, error)
	GetLuksKey(context.Context, string, *cadestrov1.GetLuksKeyRequest) (*cadestrov1.GetLuksKeyResponse, error)
	StoreLuksKey(context.Context, string, *cadestrov1.StoreLuksKeyRequest) (*cadestrov1.StoreLuksKeyResponse, error)
	StoreLpsPasswords(context.Context, string, *cadestrov1.StoreLpsPasswordsRequest) (*cadestrov1.StoreLpsPasswordsResponse, error)
}

// SyncSource returns the current assignment policy and scheduling state
// for the authenticated device.
type SyncSource interface {
	Sync(context.Context, string) (*cadestrov1.SyncState, error)
}

type LiveOperationResults interface {
	CompleteSyncDevice(context.Context, string, string, *cadestrov1.SyncDeviceResult) error
	CompleteRebootDevice(context.Context, string, string, *cadestrov1.RebootDeviceResult) error
}

// Config supplies the direct services used by AgentService.
type Config struct {
	Store             *store.Store
	Manager           *connection.Manager
	PolicyResults     PolicyResults
	Executions        ExecutionResults
	DeviceResults     DeviceResults
	Secrets           Secrets
	Sync              SyncSource
	LiveOperations    LiveOperationResults
	TerminalSessions  *connection.TerminalSessionRegistry
	Logger            *slog.Logger
	ServerVersion     string
	DeviceLoginURL    string
	HeartbeatInterval time.Duration
	Now               func() time.Time
}

// Handler implements the target AgentService without legacy transport paths.
type Handler struct {
	cadestrov1connect.UnimplementedAgentServiceHandler

	store             *store.Store
	manager           *connection.Manager
	policyResults     PolicyResults
	executions        ExecutionResults
	deviceResults     DeviceResults
	secrets           Secrets
	sync              SyncSource
	liveOperations    LiveOperationResults
	terminalSessions  *connection.TerminalSessionRegistry
	logger            *slog.Logger
	serverVersion     string
	deviceLoginURL    string
	heartbeatInterval time.Duration
	now               func() time.Time
	validator         protovalidate.Validator
	frameLimiters     map[frameClass]*auth.RateLimiter
	frameDropAudits   *auth.RateLimiter
}

// New constructs the direct AgentService handler.
func New(cfg Config) *Handler {
	if cfg.Store == nil || cfg.Manager == nil || cfg.PolicyResults == nil || cfg.Executions == nil ||
		cfg.DeviceResults == nil || cfg.Secrets == nil || cfg.Sync == nil || cfg.LiveOperations == nil || cfg.TerminalSessions == nil {
		panic("agentstream: complete direct service wiring is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Handler{
		store: cfg.Store, manager: cfg.Manager, policyResults: cfg.PolicyResults, executions: cfg.Executions,
		deviceResults: cfg.DeviceResults, secrets: cfg.Secrets, sync: cfg.Sync, liveOperations: cfg.LiveOperations,
		terminalSessions: cfg.TerminalSessions, logger: cfg.Logger,
		serverVersion: cfg.ServerVersion, deviceLoginURL: cfg.DeviceLoginURL,
		heartbeatInterval: cfg.HeartbeatInterval, now: cfg.Now,
		validator: protovalidate.GlobalValidator,
		// These are deliberately generous ingestion ceilings, not ordinary
		// operating rates. A healthy agent stays far below every budget.
		frameLimiters: map[frameClass]*auth.RateLimiter{
			frameState:     auth.NewRateLimiter(600, frameRateWindow),
			frameHello:     auth.NewRateLimiter(10, frameRateWindow),
			frameTelemetry: auth.NewRateLimiter(12, frameRateWindow),
			frameAudit:     auth.NewRateLimiter(30, frameRateWindow),
			frameBulk:      auth.NewRateLimiter(4097, frameRateWindow),
			frameTerminal:  auth.NewRateLimiter(6000, frameRateWindow),
		},
		frameDropAudits: auth.NewRateLimiter(1, frameRateWindow),
	}
}

// Close stops the process-local frame-budget cleanup loops.
func (h *Handler) Close() {
	if h == nil {
		return
	}
	for _, limiter := range h.frameLimiters {
		limiter.Stop()
	}
	if h.frameDropAudits != nil {
		h.frameDropAudits.Stop()
	}
}

// Stream owns one authenticated device connection.
func (h *Handler) Stream(ctx context.Context, stream *connect.BidiStream[cadestrov1.AgentMessage, cadestrov1.ServerMessage]) error {
	deviceID, ok := DeviceIDFromContext(ctx)
	if !ok || !validID(deviceID) {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("authenticated device identity required"))
	}
	first, err := stream.Receive()
	if err != nil {
		return normalizeStreamClose(err)
	}
	if err := h.validator.Validate(first); err != nil {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("invalid hello frame"))
	}
	hello := first.GetHello()
	if hello == nil {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("first frame must be hello"))
	}
	if hello.GetDeviceId().GetValue() != deviceID {
		return connect.NewError(connect.CodePermissionDenied, errors.New("device identity mismatch"))
	}
	if !h.allowFrame(deviceID, first) {
		h.recordFrameDrop(ctx, deviceID, first)
		return connect.NewError(connect.CodeResourceExhausted, errors.New("agent connection rate limit exceeded"))
	}
	if err := h.recordHello(ctx, deviceID, hello); err != nil {
		if store.IsNotFound(err) || errors.Is(err, errCertificateNotActive) {
			return connect.NewError(connect.CodePermissionDenied, errors.New("device is not registered"))
		}
		h.logger.Error("record agent hello", "device_id", deviceID, "error", err)
		return connect.NewError(connect.CodeInternal, errors.New("could not establish device session"))
	}

	agent := h.manager.Register(ctx, deviceID, hello.Hostname, hello.AgentVersion, stream)
	if deadliner := writeDeadlinerFrom(ctx); deadliner != nil {
		agent.SetWriteDeadlineFunc(deadliner.SetWriteDeadline)
	}
	defer func() {
		h.manager.UnregisterIfCurrent(deviceID, agent)
		agent.Close()
		agent.WaitForInFlightSend()
	}()

	welcome := &cadestrov1.Welcome{
		ServerVersion: h.serverVersion, DeviceLoginUrl: h.deviceLoginURL,
	}
	if h.heartbeatInterval > 0 {
		welcome.HeartbeatInterval = durationpb.New(h.heartbeatInterval)
	}
	if err := agent.Send(&cadestrov1.ServerMessage{
		Id: ulid.Make().String(), Payload: &cadestrov1.ServerMessage_Welcome{Welcome: welcome},
	}); err != nil {
		return fmt.Errorf("send welcome: %w", err)
	}
	type received struct {
		message *cadestrov1.AgentMessage
		err     error
	}
	receivedCh := make(chan received, 1)
	go func() {
		for {
			message, err := stream.Receive()
			select {
			case receivedCh <- received{message: message, err: err}:
			case <-agent.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-agent.Done():
			return nil
		case received := <-receivedCh:
			if received.err != nil {
				return normalizeStreamClose(received.err)
			}
			if agent.Terminated() {
				return nil
			}
			if !h.peerCertificateActive(ctx, deviceID) {
				return connect.NewError(connect.CodePermissionDenied, errors.New("certificate is no longer active"))
			}
			if err := h.validator.Validate(received.message); err != nil {
				return connect.NewError(connect.CodeInvalidArgument, errors.New("invalid agent frame"))
			}
			if !h.allowFrame(deviceID, received.message) {
				h.recordFrameDrop(ctx, deviceID, received.message)
				continue
			}
			h.manager.UpdateLastSeen(deviceID)
			if err := h.handleAgentMessage(ctx, agent, received.message); err != nil {
				h.logger.Warn("apply agent frame", "device_id", deviceID,
					"frame", fmt.Sprintf("%T", received.message.Payload), "error", err)
				if frameNotAuthorized(err) {
					return connect.NewError(connect.CodePermissionDenied,
						errors.New("agent frame claimed a resource it does not own"))
				}
				continue
			}
		}
	}
}

func (h *Handler) recordFrameDrop(ctx context.Context, deviceID string, message *cadestrov1.AgentMessage) {
	class := frameClassOf(message)
	if h.frameDropAudits != nil && !h.frameDropAudits.Allow(deviceID) {
		return
	}
	op := agentOperation(deviceID, "FrameRateLimit/"+string(class))
	op.AuthorizationOutcome = store.AuthorizationDenied
	op.AuthorizationDetail = "device_frame_budget"
	op.Result = store.ResultRejected
	// The dropped frame's class rides the result code. It is not a row
	// reference, so it must not go in an effect's after_ref: that column
	// only accepts a ULID and the rejected INSERT rolls the operation row
	// back with it, leaving an abusive device no durable trace at all.
	op.ResultCode = "RATE_LIMITED." + string(class)
	_, err := h.store.RecordOperation(ctx, op, store.AuditEffect{
		ResourceType: "device", ResourceID: deviceID, Action: "FRAME_RATE_LIMIT",
		Outcome: store.EffectRejected,
	})
	if err != nil {
		h.logger.Error("record agent frame rate limit", "device_id", deviceID, "class", class, "error", err)
	}
	h.logger.Warn("agent frame rate limit exceeded", "device_id", deviceID, "class", class)
}

func (h *Handler) handleAgentMessage(ctx context.Context, agent *connection.Agent, message *cadestrov1.AgentMessage) error {
	deviceID := agent.DeviceID
	switch payload := message.Payload.(type) {
	case *cadestrov1.AgentMessage_Heartbeat:
		return nil
	case *cadestrov1.AgentMessage_SyncRequest:
		response, err := h.sync.Sync(ctx, deviceID)
		return h.sendResponse(agent, message.Id, response, err)
	case *cadestrov1.AgentMessage_SyncDeviceResult:
		if payload.SyncDeviceResult == nil {
			return errors.New("sync device result is required")
		}
		return h.liveOperations.CompleteSyncDevice(ctx, deviceID, message.Id, payload.SyncDeviceResult)
	case *cadestrov1.AgentMessage_RebootDeviceResult:
		if payload.RebootDeviceResult == nil {
			return errors.New("reboot device result is required")
		}
		return h.liveOperations.CompleteRebootDevice(ctx, deviceID, message.Id, payload.RebootDeviceResult)
	case *cadestrov1.AgentMessage_ManifestResult:
		state, code, err := manifestResultState(payload.ManifestResult)
		if err != nil {
			_ = h.sendResultAck(agent, message.Id, err)
			return err
		}
		err = h.policyResults.RecordPolicyManifestResult(ctx, deviceID, payload.ManifestResult.RunId,
			payload.ManifestResult.ManifestId, state, code)
		if ackErr := h.sendResultAck(agent, message.Id, err); ackErr != nil && err == nil {
			return ackErr
		}
		return err
	case *cadestrov1.AgentMessage_ActionResult:
		err := h.executions.ApplyActionResult(ctx, deviceID, payload.ActionResult)
		if ackErr := h.sendResultAck(agent, message.Id, err); ackErr != nil && err == nil {
			return ackErr
		}
		return err
	case *cadestrov1.AgentMessage_QueryResult:
		return h.deviceResults.CompleteOSQueryResult(ctx, deviceID, payload.QueryResult)
	case *cadestrov1.AgentMessage_LogQueryResult:
		return h.deviceResults.CompleteLogQueryResult(ctx, deviceID, payload.LogQueryResult)
	case *cadestrov1.AgentMessage_Inventory:
		return h.deviceResults.StoreDeviceInventory(ctx, deviceID, payload.Inventory)
	case *cadestrov1.AgentMessage_RevokeLuksDeviceKeyResult:
		return h.deviceResults.CompleteLuksKeyRevocation(ctx, deviceID, payload.RevokeLuksDeviceKeyResult)
	case *cadestrov1.AgentMessage_SecurityAlert:
		return h.recordSecurityAlert(ctx, deviceID, payload.SecurityAlert)
	case *cadestrov1.AgentMessage_GetLuksKey:
		response, err := h.secrets.GetLuksKey(ctx, deviceID, payload.GetLuksKey)
		return h.sendResponse(agent, message.Id, response, err)
	case *cadestrov1.AgentMessage_StoreLuksKey:
		response, err := h.secrets.StoreLuksKey(ctx, deviceID, payload.StoreLuksKey)
		return h.sendResponse(agent, message.Id, response, err)
	case *cadestrov1.AgentMessage_StoreLpsPasswords:
		response, err := h.secrets.StoreLpsPasswords(ctx, deviceID, payload.StoreLpsPasswords)
		return h.sendResponse(agent, message.Id, response, err)
	case *cadestrov1.AgentMessage_ValidateLuksToken:
		response, err := h.secrets.ValidateLuksToken(ctx, deviceID, payload.ValidateLuksToken)
		return h.sendResponse(agent, message.Id, response, err)
	case *cadestrov1.AgentMessage_TerminalOutput:
		return h.routeTerminal(deviceID, payload.TerminalOutput.SessionId, message)
	case *cadestrov1.AgentMessage_TerminalStateChange:
		return h.routeTerminal(deviceID, payload.TerminalStateChange.SessionId, message)
	case *cadestrov1.AgentMessage_Hello:
		return errors.New("hello is only valid as the first frame")
	default:
		return errors.New("unsupported agent frame")
	}
}

func (h *Handler) sendResultAck(agent *connection.Agent, messageID string, resultErr error) error {
	if agent == nil || messageID == "" {
		return nil
	}
	ack := &cadestrov1.ResultAck{Accepted: resultErr == nil}
	if resultErr == nil {
		ack.Code = cadestrov1.ResultAckCode_RESULT_ACK_CODE_ACCEPTED
	} else {
		ack.Code = cadestrov1.ResultAckCode_RESULT_ACK_CODE_REJECTED
	}
	return agent.Send(&cadestrov1.ServerMessage{Id: messageID, Payload: &cadestrov1.ServerMessage_ResultAck{ResultAck: ack}})
}

func (h *Handler) allowFrame(deviceID string, message *cadestrov1.AgentMessage) bool {
	if h == nil || h.frameLimiters == nil {
		return true
	}
	limiter := h.frameLimiters[frameClassOf(message)]
	return limiter == nil || limiter.Allow(deviceID)
}

func frameClassOf(message *cadestrov1.AgentMessage) frameClass {
	if message == nil {
		return frameState
	}
	switch message.Payload.(type) {
	case *cadestrov1.AgentMessage_Hello:
		return frameHello
	case *cadestrov1.AgentMessage_Heartbeat:
		return frameTelemetry
	case *cadestrov1.AgentMessage_SecurityAlert:
		return frameAudit
	case *cadestrov1.AgentMessage_OutputChunk:
		return frameBulk
	case *cadestrov1.AgentMessage_TerminalOutput, *cadestrov1.AgentMessage_TerminalStateChange:
		return frameTerminal
	default:
		return frameState
	}
}

func (h *Handler) sendResponse(agent *connection.Agent, messageID string, response any, operationErr error) error {
	if operationErr != nil {
		h.logger.Warn("agent request failed", "device_id", agent.DeviceID, "error", operationErr)
		return agent.Send(&cadestrov1.ServerMessage{
			Id: messageID,
			Payload: &cadestrov1.ServerMessage_Error{Error: &cadestrov1.Error{
				Code: connect.CodeFailedPrecondition.String(), Message: "secret operation failed",
			}},
		})
	}
	message := &cadestrov1.ServerMessage{Id: messageID}
	switch response := response.(type) {
	case *cadestrov1.GetLuksKeyResponse:
		message.Payload = &cadestrov1.ServerMessage_GetLuksKey{GetLuksKey: response}
	case *cadestrov1.StoreLuksKeyResponse:
		message.Payload = &cadestrov1.ServerMessage_StoreLuksKey{StoreLuksKey: response}
	case *cadestrov1.StoreLpsPasswordsResponse:
		message.Payload = &cadestrov1.ServerMessage_StoreLpsPasswords{StoreLpsPasswords: response}
	case *cadestrov1.ValidateLuksTokenResponse:
		message.Payload = &cadestrov1.ServerMessage_ValidateLuksToken{ValidateLuksToken: response}
	case *cadestrov1.SyncState:
		message.Payload = &cadestrov1.ServerMessage_SyncState{SyncState: response}
	default:
		return errors.New("unsupported agent response")
	}
	return agent.Send(message)
}

// errForeignTerminalSession is the terminal path's cross-device claim. It is
// a sentinel rather than an inline error so frameNotAuthorized can recognise
// it without matching on message text.
var errForeignTerminalSession = errors.New("terminal session belongs to another device")

// frameNotAuthorized reports whether a per-frame application error is the
// device claiming a resource that is not its own. Only those end the
// connection.
//
// Everything else — malformed input, a stale transition, an already-applied
// replay — is dropped and the stream continues. The agent's outbox is
// durable: a frame control refuses is re-sent on every reconnect, so ending
// the connection turns one bad frame into a permanent reconnect loop and
// discards every other frame the device was about to report. Defaulting to
// "keep the stream" is what makes a new sink's rejection safe by
// construction; a new cross-actor sentinel must be added here, and
// TestFrameAuthorizationClassificationCoversEveryCrossActorSentinel fails
// until it is.
func frameNotAuthorized(err error) bool {
	switch {
	case errors.Is(err, errForeignTerminalSession):
		return true
	}
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		switch connectErr.Code() {
		case connect.CodeUnauthenticated, connect.CodePermissionDenied:
			return true
		}
	}
	return false
}

func (h *Handler) routeTerminal(deviceID, sessionID string, message *cadestrov1.AgentMessage) error {
	session := h.terminalSessions.Get(sessionID)
	if session == nil {
		return nil
	}
	if session.DeviceID != deviceID {
		return errForeignTerminalSession
	}
	h.terminalSessions.RouteAgentMessage(sessionID, message)
	return nil
}

func (h *Handler) recordHello(ctx context.Context, deviceID string, hello *cadestrov1.Hello) error {
	now := h.now().UTC().Truncate(time.Microsecond)
	_, err := h.store.WithAudit(ctx, agentOperation(deviceID, "Hello"),
		func(ctx context.Context, tx *store.Tx, recorder *store.AuditRecorder) error {
			current, err := tx.GetDevice(ctx, deviceID)
			if err != nil {
				return err
			}
			_, peerOK := mtls.PeerCertificateFromContext(ctx)
			serial, serialOK := mtls.PeerSerialFromContext(ctx)
			if !peerOK || !serialOK {
				return errCertificateNotActive
			}
			if current.ActiveCertSerial == nil || (*current.ActiveCertSerial != serial && (current.PendingCertSerial == nil || *current.PendingCertSerial != serial)) {
				return errCertificateNotActive
			}
			promoted := current.PendingCertSerial != nil && *current.PendingCertSerial == serial
			if promoted {
				current, err = tx.PromotePendingDeviceCertificate(ctx, db.PromotePendingDeviceCertificateParams{ID: deviceID, PendingSerial: &serial})
				if err != nil {
					return err
				}
			}
			if current.ActiveCertSerial == nil || *current.ActiveCertSerial != serial {
				return errCertificateNotActive
			}
			rows, err := tx.RecordDeviceHello(ctx, db.RecordDeviceHelloParams{
				Hostname: hello.Hostname, AgentVersion: hello.AgentVersion, LastSeenAt: &now, ID: deviceID,
			})
			if err != nil {
				return err
			}
			if rows != 1 {
				return store.ErrNotFound
			}
			changed := []string{"agent_version", "hostname", "last_seen_at"}
			if promoted {
				changed = append(changed, "active_cert_serial", "pending_cert_serial")
			}
			recorder.Effect(store.AuditEffect{
				ResourceType: "device", ResourceID: deviceID, Action: "CONNECT", Outcome: store.EffectApplied,
				ChangedFields: changed,
			})
			return nil
		})
	return err
}

// peerCertificateActive enforces the active serial before every privileged
// frame. Pending certificates are handled only by recordHello's promotion
// transaction and never authorize a later frame by themselves.
func (h *Handler) peerCertificateActive(ctx context.Context, deviceID string) bool {
	_, ok := mtls.PeerCertificateFromContext(ctx)
	serial, serialOK := mtls.PeerSerialFromContext(ctx)
	if !ok || !serialOK {
		return false
	}
	device, err := h.store.GetDevice(ctx, deviceID)
	if err != nil {
		return false
	}
	if device.ActiveCertSerial != nil && *device.ActiveCertSerial == serial {
		return true
	}
	return false
}

func (h *Handler) recordSecurityAlert(ctx context.Context, deviceID string, alert *cadestrov1.SecurityAlert) error {
	if alert == nil || alert.Type == cadestrov1.SecurityAlertType_SECURITY_ALERT_TYPE_UNSPECIFIED {
		return errors.New("invalid security alert")
	}
	op := agentOperation(deviceID, "SecurityAlert")
	// The alert type is the record's whole content. It rides the result
	// code because that column takes 64 characters of the shape an enum
	// name has; the effect's action column stops at 32 and the longest
	// alert name is 47, and a reference column takes a ULID or nothing.
	op.ResultCode = alert.Type.String()
	_, err := h.store.RecordOperation(ctx, op, store.AuditEffect{
		ResourceType: "device", ResourceID: deviceID, Action: "SECURITY_ALERT",
		Outcome: store.EffectApplied,
	})
	return err
}

func manifestResultState(result *cadestrov1.ManifestResult) (state, code string, err error) {
	if result == nil {
		return "", "", errors.New("manifest result is required")
	}
	switch result.Status {
	case cadestrov1.ExecutionStatus_EXECUTION_STATUS_SUCCESS:
		return "SUCCEEDED", "SUCCESS", nil
	case cadestrov1.ExecutionStatus_EXECUTION_STATUS_FAILED:
		return "FAILED", "FAILED", nil
	case cadestrov1.ExecutionStatus_EXECUTION_STATUS_INDETERMINATE:
		return "PARTIAL", "INDETERMINATE", nil
	default:
		return "", "", errors.New("invalid manifest result status")
	}
}

func validID(value string) bool {
	_, err := ulid.ParseStrict(value)
	return err == nil
}

func agentOperation(deviceID, descriptor string) store.AuditOperation {
	return store.AuditOperation{
		Class: store.ClassMutation, ActorType: "agent", ActorID: deviceID, Origin: "agent_stream",
		RequestDescriptor:    "cadestro.v1.AgentService.Stream/" + descriptor,
		AuthorizationOutcome: store.AuthorizationAllowed, AuthorizationDetail: "device_mtls",
		Result: store.ResultSuccess, ResultCode: "OK",
	}
}

func normalizeStreamClose(err error) error {
	if err == nil || errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
		return nil
	}
	var connectErr *connect.Error
	if errors.As(err, &connectErr) && (connectErr.Code() == connect.CodeCanceled ||
		(connectErr.Code() == connect.CodeUnknown && strings.Contains(connectErr.Message(), "EOF"))) {
		return nil
	}
	return err
}

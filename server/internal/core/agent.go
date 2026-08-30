package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"github.com/oklog/ulid/v2"
	"google.golang.org/protobuf/types/known/durationpb"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/mtls"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

func deterministicULID(at time.Time, values ...string) (string, error) {
	hash := sha256.New()
	for _, value := range values {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		if _, err := hash.Write(length[:]); err != nil {
			return "", err
		}
		if _, err := hash.Write([]byte(value)); err != nil {
			return "", err
		}
	}
	id, err := ulid.New(ulid.Timestamp(at), bytes.NewReader(hash.Sum(nil)))
	if err != nil {
		return "", fmt.Errorf("create deterministic ULID: %w", err)
	}
	return id.String(), nil
}

func AgentMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" || request.URL.Path == "/ready" {
			next.ServeHTTP(response, request)
			return
		}
		deviceID, err := mtls.DeviceIDFromRequest(request)
		if err != nil {
			http.Error(response, "client certificate required", http.StatusUnauthorized)
			return
		}
		peerClass, err := mtls.PeerClassFromTLS(request.TLS)
		if err != nil || peerClass != mtls.PeerClassAgent {
			http.Error(response, "agent certificate required", http.StatusForbidden)
			return
		}
		ctx := mtls.WithDeviceID(request.Context(), deviceID)
		ctx = mtls.WithPeerCertificate(ctx, request.TLS.PeerCertificates[0])
		next.ServeHTTP(response, request.WithContext(ctx))
	})
}

func (service *Service) authenticateAgent(ctx context.Context, hello *cadestrov1.Hello) (string, error) {
	deviceID, idOK := mtls.DeviceIDFromContext(ctx)
	serial, serialOK := mtls.PeerSerialFromContext(ctx)
	if !idOK || !serialOK || hello.GetDeviceId().GetValue() != deviceID {
		return "", connect.NewError(connect.CodeUnauthenticated, errors.New("authenticated device identity required"))
	}
	device, err := service.store.Queries().GetDevice(ctx, deviceID)
	if err != nil {
		if store.IsNotFound(err) {
			return "", connect.NewError(connect.CodePermissionDenied, errors.New("device is not registered"))
		}
		return "", service.internal("authenticate agent", err)
	}
	switch {
	case device.ActiveCertSerial == serial:
	case device.PendingCertSerial != nil && *device.PendingCertSerial == serial:
		rows, err := service.store.Queries().PromotePendingDeviceCertificate(ctx, db.PromotePendingDeviceCertificateParams{ID: deviceID, PendingCertSerial: &serial})
		if err != nil {
			return "", service.internal("promote agent certificate", err)
		}
		if rows != 1 {
			return "", connect.NewError(connect.CodePermissionDenied, errors.New("device certificate is not current"))
		}
	default:
		return "", connect.NewError(connect.CodePermissionDenied, errors.New("device certificate is not current"))
	}
	return deviceID, nil
}

func (service *Service) Stream(ctx context.Context, stream *connect.BidiStream[cadestrov1.AgentMessage, cadestrov1.ServerMessage]) error {
	first, err := stream.Receive()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return connect.NewError(connect.CodeUnavailable, errors.New("agent stream closed"))
	}
	if err := protovalidate.GlobalValidator.Validate(first); err != nil || first.GetHello() == nil {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("first frame must be a valid hello"))
	}
	hello := first.GetHello()
	deviceID, err := service.authenticateAgent(ctx, hello)
	if err != nil {
		return err
	}
	if err := service.touchAgent(ctx, deviceID, hello); err != nil {
		return err
	}
	if err := stream.Send(&cadestrov1.ServerMessage{
		Id: &cadestrov1.MessageId{Value: ulid.Make().String()},
		Payload: &cadestrov1.ServerMessage_Welcome{Welcome: &cadestrov1.Welcome{
			ServerVersion: service.version, HeartbeatInterval: durationpb.New(service.heartbeatInterval),
		}},
	}); err != nil {
		return fmt.Errorf("send welcome: %w", err)
	}
	for {
		message, err := stream.Receive()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return connect.NewError(connect.CodeUnavailable, errors.New("agent stream closed"))
		}
		if err := protovalidate.GlobalValidator.Validate(message); err != nil {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("invalid agent frame"))
		}
		if err := service.handleAgentMessage(ctx, stream, deviceID, hello, message); err != nil {
			return err
		}
	}
}

func (service *Service) touchAgent(ctx context.Context, deviceID string, hello *cadestrov1.Hello) error {
	now := service.now().UTC()
	if err := service.store.Queries().TouchDevice(ctx, db.TouchDeviceParams{Hostname: hello.GetHostname(), AgentVersion: hello.GetAgentVersion(), LastSeenAt: &now, ID: deviceID}); err != nil {
		return service.internal("record agent heartbeat", err)
	}
	return nil
}

func (service *Service) handleAgentMessage(ctx context.Context, stream *connect.BidiStream[cadestrov1.AgentMessage, cadestrov1.ServerMessage], deviceID string, hello *cadestrov1.Hello, message *cadestrov1.AgentMessage) error {
	switch payload := message.Payload.(type) {
	case *cadestrov1.AgentMessage_Heartbeat:
		return service.touchAgent(ctx, deviceID, hello)
	case *cadestrov1.AgentMessage_SyncRequest:
		state, err := service.syncState(ctx, deviceID)
		if err != nil {
			return err
		}
		return stream.Send(&cadestrov1.ServerMessage{Id: message.Id, Payload: &cadestrov1.ServerMessage_SyncState{SyncState: state}})
	case *cadestrov1.AgentMessage_ActionResult:
		err := service.storeActionResult(ctx, deviceID, payload.ActionResult)
		code := cadestrov1.ResultAckCode_RESULT_ACK_CODE_ACCEPTED
		if err != nil {
			service.logger.Warn("reject agent result", "device_id", deviceID, "error", err)
			code = cadestrov1.ResultAckCode_RESULT_ACK_CODE_REJECTED
		}
		return stream.Send(&cadestrov1.ServerMessage{Id: message.Id, Payload: &cadestrov1.ServerMessage_ResultAck{ResultAck: &cadestrov1.ResultAck{Code: code}}})
	case *cadestrov1.AgentMessage_ManifestResult:
		return stream.Send(&cadestrov1.ServerMessage{Id: message.Id, Payload: &cadestrov1.ServerMessage_ResultAck{ResultAck: &cadestrov1.ResultAck{Code: cadestrov1.ResultAckCode_RESULT_ACK_CODE_ACCEPTED}}})
	case *cadestrov1.AgentMessage_Hello:
		return connect.NewError(connect.CodeInvalidArgument, errors.New("hello is only valid as the first frame"))
	default:
		return connect.NewError(connect.CodeInvalidArgument, errors.New("unsupported agent frame"))
	}
}

func (service *Service) syncState(ctx context.Context, deviceID string) (*cadestrov1.SyncState, error) {
	actions, err := service.store.Queries().ListActionsForDevice(ctx, db.ListActionsForDeviceParams{DeviceID: deviceID, TargetID: deviceID})
	if err != nil {
		return nil, service.internal("compile desired policy", err)
	}
	latest := time.UnixMilli(0).UTC()
	parts := make([]string, 0, len(actions)*2)
	for _, action := range actions {
		parts = append(parts, action.ID, action.UpdatedAt.UTC().Format(time.RFC3339Nano))
		if action.UpdatedAt.After(latest) {
			latest = action.UpdatedAt
		}
	}
	revision, err := deterministicULID(latest, parts...)
	if err != nil {
		return nil, service.internal("create policy revision", err)
	}
	policy := &cadestrov1.DesiredPolicy{Revision: &cadestrov1.PolicyRevisionId{Value: revision}}
	for _, action := range actions {
		executable, err := executableAction(action)
		if err != nil {
			return nil, service.internal("map desired action", err)
		}
		manifestID, err := deterministicULID(action.UpdatedAt, action.ID, "manifest")
		if err != nil {
			return nil, service.internal("create manifest ID", err)
		}
		occurrenceID, err := deterministicULID(action.UpdatedAt, action.ID, "occurrence")
		if err != nil {
			return nil, service.internal("create occurrence ID", err)
		}
		policy.Manifests = append(policy.Manifests, &cadestrov1.Manifest{
			ManifestId: &cadestrov1.ManifestId{Value: manifestID}, OccurrenceId: &cadestrov1.OccurrenceId{Value: occurrenceID},
			Action: executable, Schedule: executable.Schedule,
		})
	}
	return &cadestrov1.SyncState{SyncIntervalMinutes: 5, DesiredPolicy: policy}, nil
}

func (service *Service) storeActionResult(ctx context.Context, deviceID string, result *cadestrov1.ActionResult) error {
	if result == nil {
		return errors.New("action result is required")
	}
	actionID := result.GetActionId().GetValue()
	actions, err := service.store.Queries().ListActionsForDevice(ctx, db.ListActionsForDeviceParams{DeviceID: deviceID, TargetID: deviceID})
	if err != nil {
		return fmt.Errorf("list assigned actions: %w", err)
	}
	var assigned *db.Action
	for _, action := range actions {
		if action.ID == actionID {
			assigned = action
			break
		}
	}
	if assigned == nil {
		return errors.New("action is not assigned to device")
	}
	completedAt := service.now().UTC()
	if result.GetCompletedAt() != nil {
		completedAt = result.GetCompletedAt().AsTime()
	}
	output := result.GetOutput()
	if output == nil {
		output = &cadestrov1.CommandOutput{}
	}
	detection := result.GetDetectionOutput()
	if detection == nil {
		detection = &cadestrov1.CommandOutput{}
	}
	executable, err := executableAction(assigned)
	if err != nil {
		return fmt.Errorf("decode assigned action: %w", err)
	}
	if err := service.store.Queries().CreateExecutionResult(ctx, db.CreateExecutionResultParams{
		RunID: result.GetRunId().GetValue(), DeviceID: deviceID, ActionID: actionID, Status: int64(result.GetStatus()), Error: result.GetError(),
		OutputExitCode: int64(output.GetExitCode()), OutputStdout: output.GetStdout(), OutputStderr: output.GetStderr(), CompletedAt: completedAt,
		Compliant: result.GetCompliant(), DetectionExitCode: int64(detection.GetExitCode()), DetectionStdout: detection.GetStdout(),
		DetectionStderr: detection.GetStderr(), IsCompliance: executable.GetShell().GetIsCompliance(),
	}); err != nil {
		return fmt.Errorf("store action result: %w", err)
	}
	return service.audit(ctx, "execution_result.received", "action", actionID, "device", deviceID)
}

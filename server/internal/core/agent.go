package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"connectrpc.com/connect"
	"github.com/oklog/ulid/v2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/mtls"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

var errResultRejected = errors.New("action result rejected")

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
		return err
	}
	if first.GetHello() == nil {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("first frame must be hello"))
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
		Id:      &cadestrov1.MessageId{Value: ulid.Make().String()},
		Payload: &cadestrov1.ServerMessage_Welcome{Welcome: &cadestrov1.Welcome{HeartbeatInterval: durationpb.New(service.heartbeatInterval)}},
	}); err != nil {
		return fmt.Errorf("send welcome: %w", err)
	}
	for {
		message, err := stream.Receive()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
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
	case *cadestrov1.AgentMessage_DesiredPolicyRequest:
		policy, err := service.desiredPolicy(ctx, deviceID)
		if err != nil {
			return err
		}
		return stream.Send(&cadestrov1.ServerMessage{Id: message.Id, Payload: &cadestrov1.ServerMessage_DesiredPolicy{DesiredPolicy: policy}})
	case *cadestrov1.AgentMessage_ActionResult:
		err := service.storeActionResult(ctx, deviceID, payload.ActionResult)
		if err != nil {
			if !errors.Is(err, errResultRejected) {
				return service.internal("store agent result", err)
			}
			service.logger.Warn("reject agent result", "device_id", deviceID, "error", err)
			return stream.Send(&cadestrov1.ServerMessage{Id: message.Id, Payload: &cadestrov1.ServerMessage_ResultAck{ResultAck: &cadestrov1.ResultAck{Code: cadestrov1.ResultAckCode_RESULT_ACK_CODE_REJECTED}}})
		}
		return stream.Send(&cadestrov1.ServerMessage{Id: message.Id, Payload: &cadestrov1.ServerMessage_ResultAck{ResultAck: &cadestrov1.ResultAck{Code: cadestrov1.ResultAckCode_RESULT_ACK_CODE_ACCEPTED}}})
	case *cadestrov1.AgentMessage_Hello:
		return connect.NewError(connect.CodeInvalidArgument, errors.New("hello is only valid as the first frame"))
	default:
		return connect.NewError(connect.CodeInvalidArgument, errors.New("unsupported agent frame"))
	}
}

func (service *Service) desiredPolicy(ctx context.Context, deviceID string) (*cadestrov1.DesiredPolicy, error) {
	actions, err := service.store.Queries().ListActionsForDevice(ctx, db.ListActionsForDeviceParams{DeviceID: deviceID, TargetID: deviceID})
	if err != nil {
		return nil, service.internal("compile desired policy", err)
	}
	policy := &cadestrov1.DesiredPolicy{}
	for _, action := range actions {
		executable, err := executableAction(action)
		if err != nil {
			return nil, service.internal("map desired action", err)
		}
		policy.Actions = append(policy.Actions, executable)
	}
	policy.RefreshIntervalMinutes = 5
	return policy, nil
}

func (service *Service) storeActionResult(ctx context.Context, deviceID string, result *cadestrov1.ActionResult) error {
	if result == nil || result.GetCompletedAt() == nil || len(result.GetActionDigest()) != 32 {
		return fmt.Errorf("%w: malformed action result", errResultRejected)
	}
	actionID := result.GetActionId().GetValue()
	completedAt := result.GetCompletedAt().AsTime()
	if err := result.GetCompletedAt().CheckValid(); err != nil {
		return fmt.Errorf("%w: invalid completion time", errResultRejected)
	}
	resultBlob, err := proto.Marshal(result)
	if err != nil {
		return fmt.Errorf("encode action result: %w", err)
	}
	return service.store.Transaction(ctx, func(queries *db.Queries) error {
		existing, err := queries.GetExecutionResult(ctx, result.GetRunId().GetValue())
		if err == nil {
			payload, decodeErr := executionResultProto(existing.RunID, existing.ActionID, existing.CompletedAt, existing.ResultBlob)
			if decodeErr != nil {
				return fmt.Errorf("decode stored duplicate result: %w", decodeErr)
			}
			if existing.DeviceID == deviceID && proto.Equal(payload, result) {
				return nil
			}
			return fmt.Errorf("%w: conflicting run id", errResultRejected)
		}
		if !store.IsNotFound(err) {
			return fmt.Errorf("load existing result: %w", err)
		}
		actions, err := queries.ListActionsForDevice(ctx, db.ListActionsForDeviceParams{DeviceID: deviceID, TargetID: deviceID})
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
			return fmt.Errorf("%w: action is not assigned to device", errResultRejected)
		}
		if _, err := executableAction(assigned); err != nil {
			return fmt.Errorf("decode assigned action: %w", err)
		}
		inserted, err := queries.CreateExecutionResult(ctx, db.CreateExecutionResultParams{
			RunID: result.GetRunId().GetValue(), DeviceID: deviceID, ActionID: actionID, CompletedAt: completedAt, ResultBlob: resultBlob,
		})
		if err != nil {
			return fmt.Errorf("store action result: %w", err)
		}
		if inserted != 1 {
			return fmt.Errorf("%w: conflicting run id", errResultRejected)
		}
		return queries.CreateAuditEvent(ctx, db.CreateAuditEventParams{
			ID: ulid.Make().String(), EventType: cadestrov1.AuditEventType_AUDIT_EVENT_TYPE_EXECUTION_RESULT_RECEIVED,
			StreamType: cadestrov1.AuditStreamType_AUDIT_STREAM_TYPE_ACTION, StreamID: actionID,
			ActorType: cadestrov1.AuditActorType_AUDIT_ACTOR_TYPE_DEVICE, ActorID: deviceID, OccurredAt: service.now().UTC(),
		})
	})
}

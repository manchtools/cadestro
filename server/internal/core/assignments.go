package core

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/oklog/ulid/v2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	contract "github.com/manchtools/cadestro/contract"
	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

func assignmentProto(assignment *db.ListAssignmentsRow) *cadestrov1.Assignment {
	return &cadestrov1.Assignment{
		Id: &cadestrov1.AssignmentId{Value: assignment.ID}, ActionId: &cadestrov1.ActionId{Value: assignment.ActionID},
		ActionName: assignment.ActionName, TargetType: assignment.TargetType,
		TargetId: &cadestrov1.AssignmentTargetId{Value: assignment.TargetID}, TargetName: assignment.TargetName,
		CreatedAt: timestamppb.New(assignment.CreatedAt),
	}
}

func (service *Service) assignmentTargetName(ctx context.Context, queries *db.Queries, targetType cadestrov1.AssignmentTargetType, targetID string) (string, error) {
	switch targetType {
	case cadestrov1.AssignmentTargetType_ASSIGNMENT_TARGET_TYPE_DEVICE:
		device, err := queries.GetDevice(ctx, targetID)
		if err != nil {
			if store.IsNotFound(err) {
				return "", rpcNotFound("device")
			}
			return "", service.internal("get assignment device", err)
		}
		return device.Hostname, nil
	case cadestrov1.AssignmentTargetType_ASSIGNMENT_TARGET_TYPE_DEVICE_GROUP:
		group, err := queries.GetDeviceGroup(ctx, targetID)
		if err != nil {
			if store.IsNotFound(err) {
				return "", rpcNotFound("device group")
			}
			return "", service.internal("get assignment device group", err)
		}
		return group.Name, nil
	default:
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("unsupported assignment target type"))
	}
}

func (service *Service) CreateAssignment(ctx context.Context, request *connect.Request[cadestrov1.CreateAssignmentRequest]) (*connect.Response[cadestrov1.CreateAssignmentResponse], error) {
	actionID := request.Msg.GetActionId().GetValue()
	targetID := request.Msg.GetTargetId().GetValue()
	var mapped *cadestrov1.Assignment
	err := service.store.Transaction(ctx, func(queries *db.Queries) error {
		action, err := queries.GetAction(ctx, actionID)
		if err != nil {
			if store.IsNotFound(err) {
				return rpcNotFound("action")
			}
			return fmt.Errorf("get assignment action: %w", err)
		}
		targetName, err := service.assignmentTargetName(ctx, queries, request.Msg.GetTargetType(), targetID)
		if err != nil {
			return err
		}
		assignment, err := queries.CreateAssignment(ctx, db.CreateAssignmentParams{
			ID: ulid.Make().String(), ActionID: actionID, TargetType: request.Msg.GetTargetType(), TargetID: targetID,
		})
		if err != nil {
			return err
		}
		if err := service.audit(ctx, queries, cadestrov1.AuditEventType_AUDIT_EVENT_TYPE_ASSIGNMENT_CREATED, cadestrov1.AuditStreamType_AUDIT_STREAM_TYPE_ASSIGNMENT, assignment.ID, cadestrov1.AuditActorType_AUDIT_ACTOR_TYPE_USER, ""); err != nil {
			return err
		}
		mapped = assignmentProto(&db.ListAssignmentsRow{
			ID: assignment.ID, ActionID: assignment.ActionID, TargetType: assignment.TargetType, TargetID: assignment.TargetID,
			CreatedAt: assignment.CreatedAt, ActionName: action.Name, TargetName: targetName,
		})
		return nil
	})
	if err != nil {
		var connectErr *connect.Error
		if errors.As(err, &connectErr) {
			return nil, connectErr
		}
		if store.IsConflict(err) {
			return nil, rpcConflict("assignment")
		}
		return nil, service.internal("create assignment", err)
	}
	return connect.NewResponse(&cadestrov1.CreateAssignmentResponse{Assignment: mapped}), nil
}

func (service *Service) DeleteAssignment(ctx context.Context, request *connect.Request[cadestrov1.DeleteAssignmentRequest]) (*connect.Response[cadestrov1.DeleteAssignmentResponse], error) {
	id := request.Msg.GetId().GetValue()
	err := service.store.Transaction(ctx, func(queries *db.Queries) error {
		rows, err := queries.DeleteAssignment(ctx, id)
		if err != nil {
			return err
		}
		if rows != 1 {
			return sql.ErrNoRows
		}
		return service.audit(ctx, queries, cadestrov1.AuditEventType_AUDIT_EVENT_TYPE_ASSIGNMENT_DELETED, cadestrov1.AuditStreamType_AUDIT_STREAM_TYPE_ASSIGNMENT, id, cadestrov1.AuditActorType_AUDIT_ACTOR_TYPE_USER, "")
	})
	if err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("assignment")
		}
		return nil, service.internal("delete assignment", err)
	}
	return connect.NewResponse(&cadestrov1.DeleteAssignmentResponse{}), nil
}

func (service *Service) ListAssignments(ctx context.Context, request *connect.Request[cadestrov1.ListAssignmentsRequest]) (*connect.Response[cadestrov1.ListAssignmentsResponse], error) {
	assignments, err := service.store.Queries().ListAssignments(ctx, db.ListAssignmentsParams{
		ActionFilter: request.Msg.GetActionId().GetValue(), TargetTypeFilter: int64(request.Msg.GetTargetType()), TargetFilter: request.Msg.GetTargetId().GetValue(),
	})
	if err != nil {
		return nil, service.internal("list assignments", err)
	}
	response := &cadestrov1.ListAssignmentsResponse{}
	for _, assignment := range assignments {
		response.Assignments = append(response.Assignments, assignmentProto(assignment))
	}
	return connect.NewResponse(response), nil
}

func (service *Service) GetDeviceAssignments(ctx context.Context, request *connect.Request[cadestrov1.GetDeviceAssignmentsRequest]) (*connect.Response[cadestrov1.GetDeviceAssignmentsResponse], error) {
	deviceID := request.Msg.GetDeviceId().GetValue()
	if _, err := service.store.Queries().GetDevice(ctx, deviceID); err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("device")
		}
		return nil, service.internal("get assignment device", err)
	}
	actions, err := service.store.Queries().ListActionsForDevice(ctx, db.ListActionsForDeviceParams{DeviceID: deviceID, TargetID: deviceID})
	if err != nil {
		return nil, service.internal("list device actions", err)
	}
	response := &cadestrov1.GetDeviceAssignmentsResponse{}
	for _, action := range actions {
		mapped, err := actionProto(action)
		if err != nil {
			return nil, service.internal("map assigned action", err)
		}
		response.Actions = append(response.Actions, mapped)
	}
	return connect.NewResponse(response), nil
}

func (service *Service) GetDeviceCompliance(ctx context.Context, request *connect.Request[cadestrov1.GetDeviceComplianceRequest]) (*connect.Response[cadestrov1.GetDeviceComplianceResponse], error) {
	deviceID := request.Msg.GetDeviceId().GetValue()
	if _, err := service.store.Queries().GetDevice(ctx, deviceID); err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("device")
		}
		return nil, service.internal("get compliance device", err)
	}
	compliance, err := service.deviceCompliance(ctx, deviceID)
	if err != nil {
		return nil, service.internal("compute device compliance", err)
	}
	return connect.NewResponse(&cadestrov1.GetDeviceComplianceResponse{Status: compliance.status, Checks: compliance.checks}), nil
}

func (service *Service) ListExecutionResults(ctx context.Context, request *connect.Request[cadestrov1.ListExecutionResultsRequest]) (*connect.Response[cadestrov1.ListExecutionResultsResponse], error) {
	deviceID := request.Msg.GetDeviceId().GetValue()
	if _, err := service.store.Queries().GetDevice(ctx, deviceID); err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("device")
		}
		return nil, service.internal("get execution result device", err)
	}
	completedBefore, runBefore, err := parseExecutionPageToken(request.Msg.GetPageToken())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid execution results page token"))
	}
	var hasCursor int64
	if request.Msg.GetPageToken() != "" {
		hasCursor = 1
	}
	limit := pageSize(request.Msg.GetPageSize())
	results, err := service.store.Queries().ListExecutionResults(ctx, db.ListExecutionResultsParams{
		DeviceID: deviceID, HasCursor: hasCursor, BeforeCompletedAt: completedBefore, BeforeRunID: runBefore, PageLimit: limit + 1,
	})
	if err != nil {
		return nil, service.internal("list execution results", err)
	}
	results, next := paginate(results, limit, func(result *db.ListExecutionResultsRow) string {
		return executionPageToken(result.CompletedAt, result.RunID)
	})
	response := &cadestrov1.ListExecutionResultsResponse{NextPageToken: next}
	for _, result := range results {
		payload, err := executionResultProto(result.RunID, result.ActionID, result.CompletedAt, result.ResultBlob)
		if err != nil {
			return nil, service.internal("decode execution result", err)
		}
		action, err := executableAction(&db.Action{ID: result.ActionID, ActionBlob: result.ActionBlob})
		if err != nil {
			return nil, service.internal("decode execution action", err)
		}
		complianceStatus, err := complianceStatus(action, payload)
		if err != nil {
			return nil, service.internal("classify execution result", err)
		}
		response.Results = append(response.Results, &cadestrov1.ExecutionResult{
			RunId: payload.GetRunId(), ActionId: payload.GetActionId(), ActionName: result.ActionName,
			Status: payload.GetStatus(), Output: payload.GetOutput(), CompletedAt: payload.GetCompletedAt(),
			ComplianceStatus: complianceStatus, DetectionOutput: payload.GetDetectionOutput(),
		})
	}
	return connect.NewResponse(response), nil
}

func executionPageToken(completedAt time.Time, runID string) string {
	value := completedAt.UTC().Format(time.RFC3339Nano) + "\n" + runID
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func parseExecutionPageToken(value string) (time.Time, string, error) {
	if value == "" {
		return time.Time{}, "", nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return time.Time{}, "", err
	}
	parts := strings.SplitN(string(decoded), "\n", 2)
	if len(parts) != 2 {
		return time.Time{}, "", errors.New("page token fields are missing")
	}
	completedAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", err
	}
	if _, err := ulid.ParseStrict(parts[1]); err != nil {
		return time.Time{}, "", err
	}
	return completedAt.UTC(), parts[1], nil
}

func executionResultProto(runID, actionID string, completedAt time.Time, resultBlob []byte) (*cadestrov1.ActionResult, error) {
	result := &cadestrov1.ActionResult{}
	if err := proto.Unmarshal(resultBlob, result); err != nil {
		return nil, err
	}
	if result.GetRunId().GetValue() != runID || result.GetActionId().GetValue() != actionID || result.GetCompletedAt() == nil || !result.GetCompletedAt().AsTime().Equal(completedAt) {
		return nil, errors.New("stored execution result metadata does not match result blob")
	}
	return result, nil
}

func isComplianceAction(action *cadestrov1.Action) bool {
	shell, ok := action.GetParams().(*cadestrov1.Action_Shell)
	return ok && shell.Shell.GetIsCompliance()
}

func complianceStatus(action *cadestrov1.Action, result *cadestrov1.ActionResult) (cadestrov1.ComplianceStatus, error) {
	if !isComplianceAction(action) {
		return cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_UNSPECIFIED, nil
	}
	if result == nil {
		return cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_PENDING, nil
	}
	digest, err := contract.ActionDigest(action)
	if err != nil {
		return cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_UNSPECIFIED, err
	}
	if !bytes.Equal(digest, result.GetActionDigest()) {
		return cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_PENDING, nil
	}
	if result.GetStatus() == cadestrov1.ExecutionStatus_EXECUTION_STATUS_SUCCESS && result.GetDetectionOutput() != nil && result.GetDetectionOutput().GetExitCode() == 0 {
		return cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_COMPLIANT, nil
	}
	return cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_NON_COMPLIANT, nil
}

type deviceComplianceSummary struct {
	status  cadestrov1.ComplianceStatus
	passing int32
	checks  []*cadestrov1.ComplianceCheckResult
}

func (service *Service) deviceCompliance(ctx context.Context, deviceID string) (deviceComplianceSummary, error) {
	rows, err := service.store.Queries().ListComplianceResults(ctx, deviceID)
	if err != nil {
		return deviceComplianceSummary{}, err
	}
	status := cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_UNSPECIFIED
	var passing int32
	checks := make([]*cadestrov1.ComplianceCheckResult, 0, len(rows))
	for _, row := range rows {
		action, err := executableAction(&db.Action{ID: row.ActionID, ActionBlob: row.ActionBlob})
		if err != nil {
			return deviceComplianceSummary{}, err
		}
		if !isComplianceAction(action) {
			continue
		}
		var result *cadestrov1.ActionResult
		if row.RunID != nil {
			if row.CompletedAt == nil {
				return deviceComplianceSummary{}, errors.New("stored execution result is missing completion time")
			}
			result, err = executionResultProto(*row.RunID, row.ActionID, *row.CompletedAt, row.ResultBlob)
			if err != nil {
				return deviceComplianceSummary{}, err
			}
		}
		checkStatus, err := complianceStatus(action, result)
		if err != nil {
			return deviceComplianceSummary{}, err
		}
		check := &cadestrov1.ComplianceCheckResult{ActionId: &cadestrov1.ActionId{Value: row.ActionID}, ActionName: row.ActionName, Status: checkStatus}
		if result != nil {
			check.DetectionOutput = result.GetDetectionOutput()
			check.CheckedAt = result.GetCompletedAt()
		}
		checks = append(checks, check)
		if checkStatus == cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_COMPLIANT {
			passing++
		}
		switch {
		case checkStatus == cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_NON_COMPLIANT:
			status = checkStatus
		case checkStatus == cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_PENDING && status != cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_NON_COMPLIANT:
			status = checkStatus
		case checkStatus == cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_COMPLIANT && status == cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_UNSPECIFIED:
			status = checkStatus
		}
	}
	return deviceComplianceSummary{status: status, passing: passing, checks: checks}, nil
}

func (service *Service) ListAuditEvents(ctx context.Context, request *connect.Request[cadestrov1.ListAuditEventsRequest]) (*connect.Response[cadestrov1.ListAuditEventsResponse], error) {
	before := request.Msg.GetPageToken()
	if before == "" {
		before = "~"
	}
	limit := pageSize(request.Msg.GetPageSize())
	events, err := service.store.Queries().ListAuditEvents(ctx, db.ListAuditEventsParams{ID: before, Limit: limit + 1})
	if err != nil {
		return nil, service.internal("list audit events", err)
	}
	events, next := paginate(events, limit, func(event *db.AuditEvent) string { return event.ID })
	response := &cadestrov1.ListAuditEventsResponse{NextPageToken: next}
	for _, event := range events {
		response.Events = append(response.Events, &cadestrov1.AuditEvent{
			Id: &cadestrov1.AuditEventId{Value: event.ID}, EventType: event.EventType, StreamType: event.StreamType,
			StreamId: &cadestrov1.AuditStreamId{Value: event.StreamID}, ActorType: event.ActorType,
			ActorId: &cadestrov1.AuditActorId{Value: event.ActorID}, OccurredAt: timestamppb.New(event.OccurredAt),
		})
	}
	return connect.NewResponse(response), nil
}

package core

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	contract "github.com/manchtools/cadestro/contract"
	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func createResultTestDevice(t *testing.T, service *Service, deviceID string, now time.Time) {
	t.Helper()
	_, err := service.store.Queries().CreateDevice(t.Context(), db.CreateDeviceParams{ID: deviceID, Hostname: "host", AgentVersion: "test", IdentityPublicKey: []byte(deviceID), ActiveCertificatePem: []byte{1}, ActiveCertSerial: "serial", CertExpiresAt: now.Add(time.Hour), RegisteredAt: now})
	require.NoError(t, err)
}

func createResultTestAction(t *testing.T, service *Service, actionID, name, script string, compliance bool) *cadestrov1.Action {
	t.Helper()
	action := &cadestrov1.Action{Id: &cadestrov1.ActionId{Value: actionID}, Params: &cadestrov1.Action_Shell{Shell: &cadestrov1.ShellActionParams{Script: script, DetectionScript: "check", IsCompliance: compliance}}}
	blob, err := proto.Marshal(action)
	require.NoError(t, err)
	_, err = service.store.Queries().CreateAction(t.Context(), db.CreateActionParams{ID: actionID, Name: name, ActionBlob: blob})
	require.NoError(t, err)
	return action
}

func assignResultTestAction(t *testing.T, service *Service, assignmentID, actionID, deviceID string) {
	t.Helper()
	_, err := service.store.Queries().CreateAssignment(t.Context(), db.CreateAssignmentParams{ID: assignmentID, ActionID: actionID, TargetType: 1, TargetID: deviceID})
	require.NoError(t, err)
}

func resultForAction(t *testing.T, action *cadestrov1.Action, runID string, completed time.Time, exitCode int32) *cadestrov1.ActionResult {
	t.Helper()
	digest, err := contract.ActionDigest(action)
	require.NoError(t, err)
	return &cadestrov1.ActionResult{ActionId: action.Id, RunId: &cadestrov1.RunId{Value: runID}, Status: cadestrov1.ExecutionStatus_EXECUTION_STATUS_SUCCESS, DetectionOutput: &cadestrov1.CommandOutput{ExitCode: exitCode}, CompletedAt: timestamppb.New(completed), ActionDigest: digest}
}

func insertResult(t *testing.T, service *Service, deviceID string, result *cadestrov1.ActionResult) {
	t.Helper()
	blob, err := proto.Marshal(result)
	require.NoError(t, err)
	inserted, err := service.store.Queries().CreateExecutionResult(t.Context(), db.CreateExecutionResultParams{RunID: result.GetRunId().GetValue(), DeviceID: deviceID, ActionID: result.GetActionId().GetValue(), CompletedAt: result.GetCompletedAt().AsTime(), ResultBlob: blob})
	require.NoError(t, err)
	require.EqualValues(t, 1, inserted)
}

func TestExecutionResultPersistsPayloadAndDuplicateIsIdempotent(t *testing.T) {
	service, ctx, now, _ := testService(t)
	actionID := "01K00000000000000000000001"
	deviceID := "01K00000000000000000000002"
	action := createResultTestAction(t, service, actionID, "shell", "true", false)
	createResultTestDevice(t, service, deviceID, now)
	assignmentID := "01K00000000000000000000003"
	assignResultTestAction(t, service, assignmentID, actionID, deviceID)
	result := resultForAction(t, action, "01K00000000000000000000004", now.Add(-time.Minute), 0)
	require.NoError(t, service.storeActionResult(ctx, deviceID, result))
	_, err := service.store.Queries().DeleteAssignment(ctx, assignmentID)
	require.NoError(t, err)
	require.NoError(t, service.storeActionResult(ctx, deviceID, proto.CloneOf(result)))
	require.ErrorIs(t, service.storeActionResult(ctx, deviceID, resultForAction(t, action, "01K00000000000000000000005", now, 0)), errResultRejected)
	rows, err := service.store.Queries().ListExecutionResults(ctx, db.ListExecutionResultsParams{DeviceID: deviceID, Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	stored := &cadestrov1.ActionResult{}
	require.NoError(t, proto.Unmarshal(rows[0].ResultBlob, stored))
	require.True(t, proto.Equal(result, stored))
	events, err := service.store.Queries().ListAuditEvents(ctx, db.ListAuditEventsParams{ID: "~", Limit: 10})
	require.NoError(t, err)
	require.Len(t, events, 1)
}

func TestExecutionResultRejectsMissingCompletionAndConflicts(t *testing.T) {
	service, ctx, now, _ := testService(t)
	actionID := "01K00000000000000000000011"
	deviceID := "01K00000000000000000000012"
	action := createResultTestAction(t, service, actionID, "shell", "true", false)
	createResultTestDevice(t, service, deviceID, now)
	assignResultTestAction(t, service, "01K00000000000000000000013", actionID, deviceID)
	missing := resultForAction(t, action, "01K00000000000000000000014", now, 0)
	missing.CompletedAt = nil
	require.ErrorIs(t, service.storeActionResult(ctx, deviceID, missing), errResultRejected)
	original := resultForAction(t, action, "01K00000000000000000000015", now, 0)
	require.NoError(t, service.storeActionResult(ctx, deviceID, original))
	conflict := proto.CloneOf(original)
	conflict.Status = cadestrov1.ExecutionStatus_EXECUTION_STATUS_FAILED
	require.ErrorIs(t, service.storeActionResult(ctx, deviceID, conflict), errResultRejected)
	require.NoError(t, service.storeActionResult(ctx, deviceID, resultForAction(t, action, "01K00000000000000000000016", now, 0)))
}

func TestDeviceComplianceTracksCurrentAssignmentsAndLatestDigest(t *testing.T) {
	service, ctx, now, _ := testService(t)
	deviceID := "01K00000000000000000000021"
	actionID := "01K00000000000000000000022"
	createResultTestDevice(t, service, deviceID, now)
	action := createResultTestAction(t, service, actionID, "compliance", "true", true)
	assignmentID := "01K00000000000000000000023"
	assignResultTestAction(t, service, assignmentID, actionID, deviceID)
	response, err := service.GetDeviceCompliance(ctx, connect.NewRequest(&cadestrov1.GetDeviceComplianceRequest{DeviceId: &cadestrov1.DeviceId{Value: deviceID}}))
	require.NoError(t, err)
	require.Equal(t, cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_PENDING, response.Msg.GetStatus())
	require.Len(t, response.Msg.GetChecks(), 1)
	require.Nil(t, response.Msg.GetChecks()[0].GetCheckedAt())
	insertResult(t, service, deviceID, resultForAction(t, action, "01K00000000000000000000024", now, 0))
	stale := resultForAction(t, createResultTestActionValue(actionID, "false"), "01K00000000000000000000025", now, 0)
	insertResult(t, service, deviceID, stale)
	rows, err := service.store.Queries().ListComplianceResults(ctx, deviceID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, stale.GetRunId().GetValue(), *rows[0].RunID)
	response, err = service.GetDeviceCompliance(ctx, connect.NewRequest(&cadestrov1.GetDeviceComplianceRequest{DeviceId: &cadestrov1.DeviceId{Value: deviceID}}))
	require.NoError(t, err)
	require.Equal(t, cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_PENDING, response.Msg.GetStatus())
	_, err = service.store.Queries().DeleteAssignment(ctx, assignmentID)
	require.NoError(t, err)
	response, err = service.GetDeviceCompliance(ctx, connect.NewRequest(&cadestrov1.GetDeviceComplianceRequest{DeviceId: &cadestrov1.DeviceId{Value: deviceID}}))
	require.NoError(t, err)
	require.Equal(t, cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_UNSPECIFIED, response.Msg.GetStatus())
	require.Empty(t, response.Msg.GetChecks())
}

func createResultTestActionValue(actionID, script string) *cadestrov1.Action {
	return &cadestrov1.Action{Id: &cadestrov1.ActionId{Value: actionID}, Params: &cadestrov1.Action_Shell{Shell: &cadestrov1.ShellActionParams{Script: script, DetectionScript: "check", IsCompliance: true}}}
}

func TestDeviceComplianceAggregatePrioritizesFailureThenPending(t *testing.T) {
	service, ctx, now, _ := testService(t)
	deviceID := "01K00000000000000000000031"
	createResultTestDevice(t, service, deviceID, now)
	passing := createResultTestAction(t, service, "01K00000000000000000000032", "passing", "true", true)
	pending := createResultTestAction(t, service, "01K00000000000000000000033", "pending", "true", true)
	failing := createResultTestAction(t, service, "01K00000000000000000000034", "failing", "true", true)
	for index, action := range []*cadestrov1.Action{passing, pending, failing} {
		assignResultTestAction(t, service, []string{"01K00000000000000000000035", "01K00000000000000000000036", "01K00000000000000000000037"}[index], action.GetId().GetValue(), deviceID)
	}
	insertResult(t, service, deviceID, resultForAction(t, passing, "01K00000000000000000000038", now, 0))
	response, err := service.GetDeviceCompliance(ctx, connect.NewRequest(&cadestrov1.GetDeviceComplianceRequest{DeviceId: &cadestrov1.DeviceId{Value: deviceID}}))
	require.NoError(t, err)
	require.Equal(t, cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_PENDING, response.Msg.GetStatus())
	insertResult(t, service, deviceID, resultForAction(t, failing, "01K00000000000000000000039", now, 1))
	response, err = service.GetDeviceCompliance(ctx, connect.NewRequest(&cadestrov1.GetDeviceComplianceRequest{DeviceId: &cadestrov1.DeviceId{Value: deviceID}}))
	require.NoError(t, err)
	require.Equal(t, cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_NON_COMPLIANT, response.Msg.GetStatus())
	device, err := service.store.Queries().GetDevice(ctx, deviceID)
	require.NoError(t, err)
	mapped, err := service.deviceProto(ctx, device)
	require.NoError(t, err)
	require.EqualValues(t, 3, mapped.GetComplianceTotal())
	require.EqualValues(t, 1, mapped.GetCompliancePassing())
}

func TestDeviceComplianceUsesEffectiveGroupAssignmentsOnce(t *testing.T) {
	service, ctx, now, _ := testService(t)
	deviceID := "01K00000000000000000000071"
	groupID := "01K00000000000000000000072"
	compliance := createResultTestAction(t, service, "01K00000000000000000000073", "compliance", "true", true)
	ordinary := createResultTestAction(t, service, "01K00000000000000000000074", "ordinary", "true", false)
	createResultTestDevice(t, service, deviceID, now)
	_, err := service.store.Queries().CreateDeviceGroup(ctx, db.CreateDeviceGroupParams{ID: groupID, Name: "group"})
	require.NoError(t, err)
	require.NoError(t, service.store.Queries().AddDeviceToGroup(ctx, db.AddDeviceToGroupParams{GroupID: groupID, DeviceID: deviceID}))
	directAssignment := "01K00000000000000000000075"
	assignResultTestAction(t, service, directAssignment, compliance.GetId().GetValue(), deviceID)
	_, err = service.store.Queries().CreateAssignment(ctx, db.CreateAssignmentParams{ID: "01K00000000000000000000076", ActionID: compliance.GetId().GetValue(), TargetType: cadestrov1.AssignmentTargetType_ASSIGNMENT_TARGET_TYPE_DEVICE_GROUP, TargetID: groupID})
	require.NoError(t, err)
	_, err = service.store.Queries().CreateAssignment(ctx, db.CreateAssignmentParams{ID: "01K00000000000000000000077", ActionID: ordinary.GetId().GetValue(), TargetType: cadestrov1.AssignmentTargetType_ASSIGNMENT_TARGET_TYPE_DEVICE_GROUP, TargetID: groupID})
	require.NoError(t, err)
	response, err := service.GetDeviceCompliance(ctx, connect.NewRequest(&cadestrov1.GetDeviceComplianceRequest{DeviceId: &cadestrov1.DeviceId{Value: deviceID}}))
	require.NoError(t, err)
	require.Equal(t, cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_PENDING, response.Msg.GetStatus())
	require.Len(t, response.Msg.GetChecks(), 1)
	_, err = service.store.Queries().DeleteAssignment(ctx, directAssignment)
	require.NoError(t, err)
	response, err = service.GetDeviceCompliance(ctx, connect.NewRequest(&cadestrov1.GetDeviceComplianceRequest{DeviceId: &cadestrov1.DeviceId{Value: deviceID}}))
	require.NoError(t, err)
	require.Len(t, response.Msg.GetChecks(), 1)
	_, err = service.store.Queries().RemoveDeviceFromGroup(ctx, db.RemoveDeviceFromGroupParams{GroupID: groupID, DeviceID: deviceID})
	require.NoError(t, err)
	response, err = service.GetDeviceCompliance(ctx, connect.NewRequest(&cadestrov1.GetDeviceComplianceRequest{DeviceId: &cadestrov1.DeviceId{Value: deviceID}}))
	require.NoError(t, err)
	require.Equal(t, cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_UNSPECIFIED, response.Msg.GetStatus())
	require.Empty(t, response.Msg.GetChecks())
}

func TestListExecutionResultsUsesComplianceStatus(t *testing.T) {
	service, ctx, now, _ := testService(t)
	deviceID := "01K00000000000000000000041"
	action := createResultTestAction(t, service, "01K00000000000000000000042", "compliance", "true", true)
	createResultTestDevice(t, service, deviceID, now)
	insertResult(t, service, deviceID, resultForAction(t, action, "01K00000000000000000000043", now, 0))
	response, err := service.ListExecutionResults(ctx, connect.NewRequest(&cadestrov1.ListExecutionResultsRequest{DeviceId: &cadestrov1.DeviceId{Value: deviceID}, PageSize: 10}))
	require.NoError(t, err)
	require.Len(t, response.Msg.GetResults(), 1)
	require.Equal(t, cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_COMPLIANT, response.Msg.GetResults()[0].GetComplianceStatus())
}

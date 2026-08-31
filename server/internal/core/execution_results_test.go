package core

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestExecutionResultPersistsSerializedPayload(t *testing.T) {
	service, ctx, now, _ := testService(t)
	actionID := "01K00000000000000000000001"
	deviceID := "01K00000000000000000000002"
	action := &cadestrov1.Action{
		Id:     &cadestrov1.ActionId{Value: actionID},
		Params: &cadestrov1.Action_Shell{Shell: &cadestrov1.ShellActionParams{Script: "true"}},
	}
	blob, err := proto.Marshal(action)
	require.NoError(t, err)
	_, err = service.store.Queries().CreateAction(ctx, db.CreateActionParams{ID: actionID, Name: "shell", ActionBlob: blob})
	require.NoError(t, err)
	_, err = service.store.Queries().CreateDevice(ctx, db.CreateDeviceParams{ID: deviceID, Hostname: "host", AgentVersion: "test", IdentityPublicKey: []byte{1}, ActiveCertificatePem: []byte{1}, ActiveCertSerial: "serial", CertExpiresAt: now.Add(time.Hour), RegisteredAt: now})
	require.NoError(t, err)
	_, err = service.store.Queries().CreateAssignment(ctx, db.CreateAssignmentParams{ID: "01K00000000000000000000003", ActionID: actionID, TargetType: 1, TargetID: deviceID})
	require.NoError(t, err)
	result := &cadestrov1.ActionResult{
		ActionId: &cadestrov1.ActionId{Value: actionID}, Status: cadestrov1.ExecutionStatus_EXECUTION_STATUS_SUCCESS,
		Output:   &cadestrov1.CommandOutput{ExitCode: 7, Stdout: "out", Stderr: "err"},
		DetectionOutput: &cadestrov1.CommandOutput{ExitCode: 2, Stdout: "detect", Stderr: "detect err"},
		RunId:           &cadestrov1.RunId{Value: "01K00000000000000000000004"},
	}
	expected := proto.CloneOf(result)
	expected.CompletedAt = timestamppb.New(now)
	require.NoError(t, service.storeActionResult(ctx, deviceID, result))
	rows, err := service.store.Queries().ListExecutionResults(ctx, db.ListExecutionResultsParams{DeviceID: deviceID, Limit: 1})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	stored := &cadestrov1.ActionResult{}
	require.NoError(t, proto.Unmarshal(rows[0].ResultBlob, stored))
	require.True(t, proto.Equal(expected, stored))
	response, err := service.ListExecutionResults(ctx, connect.NewRequest(&cadestrov1.ListExecutionResultsRequest{DeviceId: &cadestrov1.DeviceId{Value: deviceID}, PageSize: 1}))
	require.NoError(t, err)
	require.Len(t, response.Msg.Results, 1)
	require.False(t, response.Msg.Results[0].GetCompliant())
}

func TestDeviceComplianceUsesLinkedShellAction(t *testing.T) {
	service, ctx, now, _ := testService(t)
	deviceID := "01K00000000000000000000011"
	complianceID := "01K00000000000000000000012"
	normalID := "01K00000000000000000000013"
	_, err := service.store.Queries().CreateDevice(ctx, db.CreateDeviceParams{ID: deviceID, Hostname: "host", AgentVersion: "test", IdentityPublicKey: []byte{1}, ActiveCertificatePem: []byte{1}, ActiveCertSerial: "serial", CertExpiresAt: now.Add(time.Hour), RegisteredAt: now})
	require.NoError(t, err)
	for _, action := range []*cadestrov1.Action{
		{Id: &cadestrov1.ActionId{Value: complianceID}, Params: &cadestrov1.Action_Shell{Shell: &cadestrov1.ShellActionParams{DetectionScript: "check", IsCompliance: true}}},
		{Id: &cadestrov1.ActionId{Value: normalID}, Params: &cadestrov1.Action_Shell{Shell: &cadestrov1.ShellActionParams{Script: "true"}}},
	} {
		blob, err := proto.Marshal(action)
		require.NoError(t, err)
		_, err = service.store.Queries().CreateAction(ctx, db.CreateActionParams{ID: action.GetId().GetValue(), Name: action.GetId().GetValue(), ActionBlob: blob})
		require.NoError(t, err)
	}
	for _, result := range []*cadestrov1.ActionResult{
		{RunId: &cadestrov1.RunId{Value: "01K00000000000000000000014"}, ActionId: &cadestrov1.ActionId{Value: complianceID}, CompletedAt: timestamppb.New(now), Status: cadestrov1.ExecutionStatus_EXECUTION_STATUS_SUCCESS, DetectionOutput: &cadestrov1.CommandOutput{ExitCode: 1}},
		{RunId: &cadestrov1.RunId{Value: "01K00000000000000000000015"}, ActionId: &cadestrov1.ActionId{Value: normalID}, CompletedAt: timestamppb.New(now), Status: cadestrov1.ExecutionStatus_EXECUTION_STATUS_SUCCESS},
	} {
		blob, err := proto.Marshal(result)
		require.NoError(t, err)
		require.NoError(t, service.store.Queries().CreateExecutionResult(ctx, db.CreateExecutionResultParams{RunID: result.GetRunId().GetValue(), DeviceID: deviceID, ActionID: result.GetActionId().GetValue(), CompletedAt: now, ResultBlob: blob}))
	}
	checks, err := service.store.Queries().ListComplianceResults(ctx, deviceID)
	require.NoError(t, err)
	require.Len(t, checks, 2)
	response, err := service.GetDeviceCompliance(ctx, connect.NewRequest(&cadestrov1.GetDeviceComplianceRequest{DeviceId: &cadestrov1.DeviceId{Value: deviceID}}))
	require.NoError(t, err)
	require.Len(t, response.Msg.Checks, 1)
	require.Equal(t, complianceID, response.Msg.Checks[0].GetActionId().GetValue())
	require.Equal(t, cadestrov1.ComplianceStatus_COMPLIANCE_STATUS_NON_COMPLIANT, response.Msg.Status)
	device, err := service.store.Queries().GetDevice(ctx, deviceID)
	require.NoError(t, err)
	mapped, err := service.deviceProto(ctx, device)
	require.NoError(t, err)
	require.EqualValues(t, 1, mapped.ComplianceTotal)
	require.EqualValues(t, 0, mapped.CompliancePassing)
}

func TestListExecutionResultsRejectsCorruptBlobAndMetadata(t *testing.T) {
	metadataBlob, err := proto.Marshal(&cadestrov1.ActionResult{
		RunId: &cadestrov1.RunId{Value: "blob-run"}, ActionId: &cadestrov1.ActionId{Value: "01K00000000000000000000021"}, CompletedAt: timestamppb.New(time.Unix(0, 0)),
	})
	require.NoError(t, err)
	for _, test := range []struct {
		name string
		blob []byte
	}{
		{name: "malformed", blob: []byte("bad")},
		{name: "metadata", blob: metadataBlob},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, ctx, now, _ := testService(t)
			actionID := "01K00000000000000000000021"
			deviceID := "01K00000000000000000000022"
			actionBlob, err := proto.Marshal(&cadestrov1.Action{Id: &cadestrov1.ActionId{Value: actionID}, Params: &cadestrov1.Action_Shell{Shell: &cadestrov1.ShellActionParams{Script: "true"}}})
			require.NoError(t, err)
			_, err = service.store.Queries().CreateAction(ctx, db.CreateActionParams{ID: actionID, Name: "shell", ActionBlob: actionBlob})
			require.NoError(t, err)
			_, err = service.store.Queries().CreateDevice(ctx, db.CreateDeviceParams{ID: deviceID, Hostname: "host", AgentVersion: "test", IdentityPublicKey: []byte{1}, ActiveCertificatePem: []byte{1}, ActiveCertSerial: "serial", CertExpiresAt: now.Add(time.Hour), RegisteredAt: now})
			require.NoError(t, err)
			err = service.store.Queries().CreateExecutionResult(ctx, db.CreateExecutionResultParams{RunID: "row-run", DeviceID: deviceID, ActionID: actionID, CompletedAt: now, ResultBlob: test.blob})
			require.NoError(t, err)
			_, err = service.ListExecutionResults(ctx, connect.NewRequest(&cadestrov1.ListExecutionResultsRequest{DeviceId: &cadestrov1.DeviceId{Value: deviceID}, PageSize: 1}))
			require.Equal(t, connect.CodeInternal, connect.CodeOf(err))
		})
	}
}

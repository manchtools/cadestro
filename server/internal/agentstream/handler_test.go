package agentstream

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/connection"
)

func TestFrameBudgetsArePerDeviceAndClass(t *testing.T) {
	heartbeats := auth.NewRateLimiter(2, time.Minute)
	alerts := auth.NewRateLimiter(1, time.Minute)
	hellos := auth.NewRateLimiter(1, time.Minute)
	defer heartbeats.Stop()
	defer alerts.Stop()
	defer hellos.Stop()
	h := &Handler{frameLimiters: map[frameClass]*auth.RateLimiter{
		frameTelemetry: heartbeats,
		frameAudit:     alerts,
		frameHello:     hellos,
	}}
	heartbeat := &cadestrov1.AgentMessage{Payload: &cadestrov1.AgentMessage_Heartbeat{Heartbeat: &cadestrov1.Heartbeat{}}}
	alert := &cadestrov1.AgentMessage{Payload: &cadestrov1.AgentMessage_SecurityAlert{SecurityAlert: &cadestrov1.SecurityAlert{
		Type: cadestrov1.SecurityAlertType_SECURITY_ALERT_TYPE_CREDENTIAL_TAMPERING,
	}}}
	hello := &cadestrov1.AgentMessage{Payload: &cadestrov1.AgentMessage_Hello{Hello: &cadestrov1.Hello{}}}

	assert.True(t, h.allowFrame("device-1", heartbeat))
	assert.True(t, h.allowFrame("device-1", heartbeat))
	assert.False(t, h.allowFrame("device-1", heartbeat))
	assert.True(t, h.allowFrame("device-2", heartbeat), "devices must not share a budget")
	assert.True(t, h.allowFrame("device-1", alert), "frame classes must not share a budget")
	assert.False(t, h.allowFrame("device-1", alert))
	assert.True(t, h.allowFrame("device-1", hello))
	assert.False(t, h.allowFrame("device-1", hello))
}

type fakeExecutionResults struct {
	resultDevice string
	result       *cadestrov1.ActionResult
}

func (f *fakeExecutionResults) ApplyActionResult(_ context.Context, deviceID string, result *cadestrov1.ActionResult) error {
	f.resultDevice, f.result = deviceID, result
	return nil
}

type fakeDeviceResults struct {
	queryDevice, logDevice, inventoryDevice, revocationDevice string
}

type recordingLiveOperations struct {
	syncDevice, rebootDevice       string
	syncOperation, rebootOperation string
}

func (f *recordingLiveOperations) CompleteSyncDevice(_ context.Context, deviceID, operationID string, _ *cadestrov1.SyncDeviceResult) error {
	f.syncDevice, f.syncOperation = deviceID, operationID
	return nil
}

func (f *recordingLiveOperations) CompleteRebootDevice(_ context.Context, deviceID, operationID string, _ *cadestrov1.RebootDeviceResult) error {
	f.rebootDevice, f.rebootOperation = deviceID, operationID
	return nil
}

func (f *fakeDeviceResults) CompleteOSQueryResult(_ context.Context, deviceID string, _ *cadestrov1.OSQueryResult) error {
	f.queryDevice = deviceID
	return nil
}

func (f *fakeDeviceResults) CompleteLogQueryResult(_ context.Context, deviceID string, _ *cadestrov1.LogQueryResult) error {
	f.logDevice = deviceID
	return nil
}

func (f *fakeDeviceResults) StoreDeviceInventory(_ context.Context, deviceID string, _ *cadestrov1.DeviceInventory) error {
	f.inventoryDevice = deviceID
	return nil
}

func (f *fakeDeviceResults) CompleteLuksKeyRevocation(_ context.Context, deviceID string, _ *cadestrov1.RevokeLuksDeviceKeyResult) error {
	f.revocationDevice = deviceID
	return nil
}

func TestHandleAgentMessageRoutesRetainedFrames(t *testing.T) {
	deviceID := "device"
	executionResults := &fakeExecutionResults{}
	deviceResults := &fakeDeviceResults{}
	liveOperations := &recordingLiveOperations{}
	handler := &Handler{
		executions: executionResults, deviceResults: deviceResults,
		liveOperations: liveOperations, terminalSessions: connection.NewTerminalSessionRegistry(),
	}
	agent := &connection.Agent{DeviceID: deviceID}

	frames := []*cadestrov1.AgentMessage{
		{Payload: &cadestrov1.AgentMessage_Heartbeat{Heartbeat: &cadestrov1.Heartbeat{}}},
		{Id: "sync-operation", Payload: &cadestrov1.AgentMessage_SyncDeviceResult{SyncDeviceResult: &cadestrov1.SyncDeviceResult{Success: true}}},
		{Id: "reboot-operation", Payload: &cadestrov1.AgentMessage_RebootDeviceResult{RebootDeviceResult: &cadestrov1.RebootDeviceResult{Success: true}}},
		{Payload: &cadestrov1.AgentMessage_ActionResult{ActionResult: &cadestrov1.ActionResult{OccurrenceId: "occurrence"}}},
		{Payload: &cadestrov1.AgentMessage_QueryResult{QueryResult: &cadestrov1.OSQueryResult{QueryId: "query"}}},
		{Payload: &cadestrov1.AgentMessage_LogQueryResult{LogQueryResult: &cadestrov1.LogQueryResult{QueryId: "log"}}},
		{Payload: &cadestrov1.AgentMessage_Inventory{Inventory: &cadestrov1.DeviceInventory{}}},
		{Payload: &cadestrov1.AgentMessage_RevokeLuksDeviceKeyResult{RevokeLuksDeviceKeyResult: &cadestrov1.RevokeLuksDeviceKeyResult{ActionId: "action"}}},
	}
	for _, frame := range frames {
		require.NoError(t, handler.handleAgentMessage(context.Background(), agent, frame))
	}

	assert.Equal(t, deviceID, executionResults.resultDevice)
	assert.Equal(t, deviceID, deviceResults.queryDevice)
	assert.Equal(t, deviceID, deviceResults.logDevice)
	assert.Equal(t, deviceID, deviceResults.inventoryDevice)
	assert.Equal(t, deviceID, deviceResults.revocationDevice)
	assert.Equal(t, deviceID, liveOperations.syncDevice)
	assert.Equal(t, "sync-operation", liveOperations.syncOperation)
	assert.Equal(t, deviceID, liveOperations.rebootDevice)
	assert.Equal(t, "reboot-operation", liveOperations.rebootOperation)
}

func TestHandleAgentMessageEnforcesTerminalDeviceBinding(t *testing.T) {
	registry := connection.NewTerminalSessionRegistry()
	registry.Register(connection.NewTerminalSession("session", "right-device", "user", "root", 80, 24))
	handler := &Handler{terminalSessions: registry}
	message := &cadestrov1.AgentMessage{Payload: &cadestrov1.AgentMessage_TerminalOutput{
		TerminalOutput: &cadestrov1.TerminalOutput{SessionId: "session", Data: []byte("output")},
	}}

	err := handler.handleAgentMessage(context.Background(), &connection.Agent{DeviceID: "wrong-device"}, message)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "another device")

	require.NoError(t, handler.handleAgentMessage(context.Background(), &connection.Agent{DeviceID: "right-device"}, message))
	select {
	case routed := <-registry.Get("session").OutputCh:
		assert.Same(t, message, routed)
	default:
		t.Fatal("terminal frame was not routed")
	}
}

func TestManifestResultStateAcceptsOnlyAggregateOutcomes(t *testing.T) {
	tests := []struct {
		status cadestrov1.ExecutionStatus
		state  string
		code   string
	}{
		{cadestrov1.ExecutionStatus_EXECUTION_STATUS_SUCCESS, "SUCCEEDED", "SUCCESS"},
		{cadestrov1.ExecutionStatus_EXECUTION_STATUS_FAILED, "FAILED", "FAILED"},
		{cadestrov1.ExecutionStatus_EXECUTION_STATUS_INDETERMINATE, "PARTIAL", "INDETERMINATE"},
	}
	for _, test := range tests {
		state, code, err := manifestResultState(&cadestrov1.ManifestResult{Status: test.status})
		require.NoError(t, err)
		assert.Equal(t, test.state, state)
		assert.Equal(t, test.code, code)
	}
	_, _, err := manifestResultState(&cadestrov1.ManifestResult{Status: cadestrov1.ExecutionStatus_EXECUTION_STATUS_RUNNING})
	require.Error(t, err)
}

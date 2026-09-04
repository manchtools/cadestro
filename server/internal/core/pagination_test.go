package core

import (
	"encoding/base64"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

func TestPaginatedListsOmitCursorAtExactBoundary(t *testing.T) {
	service, ctx, now, _ := testService(t)
	createResultTestDevice(t, service, "01K00000000000000000000101", now)
	createResultTestAction(t, service, "01K00000000000000000000102", "page", "true", false)
	_, err := service.store.Queries().CreateRegistrationToken(ctx, db.CreateRegistrationTokenParams{
		ID: "01K00000000000000000000103", ValueHash: "page", Name: "page", ExpiresAt: now.Add(time.Hour),
	})
	require.NoError(t, err)
	_, err = service.store.Queries().CreateDeviceGroup(ctx, db.CreateDeviceGroupParams{ID: "01K00000000000000000000104", Name: "page"})
	require.NoError(t, err)
	testUser(t, service, ctx, "01K00000000000000000000105")
	err = service.store.Queries().CreateAuditEvent(ctx, db.CreateAuditEventParams{
		ID: "01K00000000000000000000106", EventType: cadestrov1.AuditEventType_AUDIT_EVENT_TYPE_DEVICE_DELETED,
		StreamType: cadestrov1.AuditStreamType_AUDIT_STREAM_TYPE_DEVICE, StreamID: "01K00000000000000000000101",
		ActorType: cadestrov1.AuditActorType_AUDIT_ACTOR_TYPE_USER, ActorID: "01K00000000000000000000105", OccurredAt: now,
	})
	require.NoError(t, err)

	devices, err := service.ListDevices(ctx, connect.NewRequest(&cadestrov1.ListDevicesRequest{PageSize: 1}))
	require.NoError(t, err)
	require.Len(t, devices.Msg.GetDevices(), 1)
	require.Empty(t, devices.Msg.GetNextPageToken())
	actions, err := service.ListActions(ctx, connect.NewRequest(&cadestrov1.ListActionsRequest{PageSize: 1}))
	require.NoError(t, err)
	require.Len(t, actions.Msg.GetActions(), 1)
	require.Empty(t, actions.Msg.GetNextPageToken())
	tokens, err := service.ListTokens(ctx, connect.NewRequest(&cadestrov1.ListTokensRequest{PageSize: 1}))
	require.NoError(t, err)
	require.Len(t, tokens.Msg.GetTokens(), 1)
	require.Empty(t, tokens.Msg.GetNextPageToken())
	groups, err := service.ListDeviceGroups(ctx, connect.NewRequest(&cadestrov1.ListDeviceGroupsRequest{PageSize: 1}))
	require.NoError(t, err)
	require.Len(t, groups.Msg.GetGroups(), 1)
	require.Empty(t, groups.Msg.GetNextPageToken())
	roles, err := service.ListRoles(ctx, connect.NewRequest(&cadestrov1.ListRolesRequest{PageSize: 1, PageToken: administratorsRoleID}))
	require.NoError(t, err)
	require.Len(t, roles.Msg.GetRoles(), 1)
	require.Empty(t, roles.Msg.GetNextPageToken())
	users, err := service.ListUsers(ctx, connect.NewRequest(&cadestrov1.ListUsersRequest{PageSize: 1}))
	require.NoError(t, err)
	require.Len(t, users.Msg.GetUsers(), 1)
	require.Empty(t, users.Msg.GetNextPageToken())
	events, err := service.ListAuditEvents(ctx, connect.NewRequest(&cadestrov1.ListAuditEventsRequest{PageSize: 1}))
	require.NoError(t, err)
	require.Len(t, events.Msg.GetEvents(), 1)
	require.Empty(t, events.Msg.GetNextPageToken())
}

func TestDeviceScopedListsRequireExistingDevice(t *testing.T) {
	service, ctx, _, _ := testService(t)
	deviceID := &cadestrov1.DeviceId{Value: "01K00000000000000000000107"}
	_, err := service.ListExecutionResults(ctx, connect.NewRequest(&cadestrov1.ListExecutionResultsRequest{DeviceId: deviceID}))
	require.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	_, err = service.ListDeviceGroupsForDevice(ctx, connect.NewRequest(&cadestrov1.ListDeviceGroupsForDeviceRequest{DeviceId: deviceID}))
	require.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestExecutionResultPaginationUsesCompletedTimeAndRunID(t *testing.T) {
	service, ctx, now, _ := testService(t)
	deviceID := "01K00000000000000000000108"
	action := createResultTestAction(t, service, "01K00000000000000000000109", "history", "true", false)
	createResultTestDevice(t, service, deviceID, now)
	completedAt := now.Add(time.Minute)
	for _, runID := range []string{
		"01K00000000000000000000201",
		"01K00000000000000000000202",
		"01K00000000000000000000203",
	} {
		insertResult(t, service, deviceID, resultForAction(t, action, runID, completedAt, 0))
	}
	olderCompletedAt := completedAt.Add(-time.Minute)
	insertResult(t, service, deviceID, resultForAction(t, action, "01K00000000000000000000299", olderCompletedAt, 0))

	first, err := service.ListExecutionResults(ctx, connect.NewRequest(&cadestrov1.ListExecutionResultsRequest{
		DeviceId: &cadestrov1.DeviceId{Value: deviceID}, PageSize: 2,
	}))
	require.NoError(t, err)
	require.Equal(t, []string{"01K00000000000000000000203", "01K00000000000000000000202"}, executionRunIDs(first.Msg.GetResults()))
	require.NotEmpty(t, first.Msg.GetNextPageToken())

	second, err := service.ListExecutionResults(ctx, connect.NewRequest(&cadestrov1.ListExecutionResultsRequest{
		DeviceId: &cadestrov1.DeviceId{Value: deviceID}, PageSize: 2, PageToken: first.Msg.GetNextPageToken(),
	}))
	require.NoError(t, err)
	require.Equal(t, []string{"01K00000000000000000000201", "01K00000000000000000000299"}, executionRunIDs(second.Msg.GetResults()))
	require.Empty(t, second.Msg.GetNextPageToken())

	empty, err := service.ListExecutionResults(ctx, connect.NewRequest(&cadestrov1.ListExecutionResultsRequest{
		DeviceId: &cadestrov1.DeviceId{Value: deviceID}, PageSize: 2,
		PageToken: executionPageToken(olderCompletedAt, "01K00000000000000000000299"),
	}))
	require.NoError(t, err)
	require.Empty(t, empty.Msg.GetResults())
	require.Empty(t, empty.Msg.GetNextPageToken())
}

func TestExecutionResultPaginationRejectsMalformedCursor(t *testing.T) {
	service, ctx, now, _ := testService(t)
	deviceID := "01K00000000000000000000110"
	createResultTestDevice(t, service, deviceID, now)
	invalid := []string{
		"!",
		base64.RawURLEncoding.EncodeToString([]byte("missing fields")),
		base64.RawURLEncoding.EncodeToString([]byte("bad time\n01K00000000000000000000204")),
		base64.RawURLEncoding.EncodeToString([]byte(now.Format(time.RFC3339Nano) + "\nnot a run id")),
	}
	for _, token := range invalid {
		_, err := service.ListExecutionResults(ctx, connect.NewRequest(&cadestrov1.ListExecutionResultsRequest{
			DeviceId: &cadestrov1.DeviceId{Value: deviceID}, PageToken: token,
		}))
		require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	}
}

func TestExecutionPageTokenNormalizesTimestampOffset(t *testing.T) {
	want := time.Date(2026, time.January, 1, 12, 0, 0, 123, time.UTC)
	runID := "01K00000000000000000000205"
	offset := want.In(time.FixedZone("test", 2*60*60)).Format(time.RFC3339Nano)
	token := base64.RawURLEncoding.EncodeToString([]byte(offset + "\n" + runID))
	completedAt, parsedRunID, err := parseExecutionPageToken(token)
	require.NoError(t, err)
	require.Equal(t, want, completedAt)
	require.Equal(t, runID, parsedRunID)
}

func executionRunIDs(results []*cadestrov1.ExecutionResult) []string {
	ids := make([]string, 0, len(results))
	for _, result := range results {
		ids = append(ids, result.GetRunId().GetValue())
	}
	return ids
}

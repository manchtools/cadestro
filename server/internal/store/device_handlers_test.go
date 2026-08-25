package store_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/manchtools/cadestro/server/internal/testdb"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/connection"
	"github.com/manchtools/cadestro/server/internal/crypto"
	"github.com/manchtools/cadestro/server/internal/device"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
	"github.com/manchtools/cadestro/server/internal/terminal"
)

type deviceHandlerFixture struct {
	t          *testing.T
	store      *store.Store
	raw        *testdb.DB
	handlers   *device.Handlers
	now        time.Time
	actorID    string
	directID   string
	groupID    string
	outsideID  string
	userID     string
	userGroup  string
	scopeGroup string
	closed     []string
	encryptor  *crypto.Encryptor
	sender     *fakeAgentSender
	tokens     *terminal.TokenStore
	sessions   *connection.TerminalSessionRegistry
	connected  map[string]bool
}

type fakeAgentSender struct {
	messages []*cadestrov1.ServerMessage
	err      error
}

func (s *fakeAgentSender) Send(_ string, message *cadestrov1.ServerMessage) error {
	s.messages = append(s.messages, message)
	return s.err
}

func newDeviceHandlerFixture(t *testing.T) *deviceHandlerFixture {
	t.Helper()
	st, raw := setupSQLite(t)
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	encryptor, err := crypto.NewEncryptor(strings.Repeat("01", 32))
	require.NoError(t, err)
	f := &deviceHandlerFixture{
		t: t, store: st, raw: raw, now: now,
		actorID: newID(), directID: newID(), groupID: newID(), outsideID: newID(),
		userID: newID(), userGroup: newID(), scopeGroup: newID(),
		encryptor: encryptor, sender: &fakeAgentSender{},
		sessions: connection.NewTerminalSessionRegistry(), connected: make(map[string]bool),
	}
	f.tokens = terminal.NewTokenStore(terminal.NewMemoryBackend(func() time.Time { return now }),
		terminal.WithClock(func() time.Time { return now }))
	f.connected[f.directID], f.connected[f.groupID], f.connected[f.outsideID] = true, true, true
	_, err = st.WithAudit(context.Background(), mutationOp(), func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		for id, email := range map[string]string{
			f.actorID: "actor@example.test",
			f.userID:  "subject@example.test",
		} {
			if _, err := tx.InsertUser(ctx, db.InsertUserParams{
				ProvisioningSource: store.UserProvisioningSourceSCIM,
				ID:                 id, Email: email, DisplayName: email, LinuxUsername: "test",
				LinuxUid: 200001, CreatedAt: &now,
			}); err != nil {
				return err
			}
		}
		if _, err := tx.InsertUserGroup(ctx, db.InsertUserGroupParams{
			ID: f.userGroup, Name: "operators", CreatedAt: now, CreatedBy: f.actorID,
		}); err != nil {
			return err
		}
		if _, err := tx.InsertUserGroupMember(ctx, db.InsertUserGroupMemberParams{
			GroupID: f.userGroup, UserID: f.actorID, AddedAt: now, AddedBy: f.actorID,
		}); err != nil {
			return err
		}
		for _, d := range []db.InsertDeviceParams{
			{
				ID: f.directID, Hostname: "direct", AgentVersion: "1.0.0",
				RegisteredAt: &now, LastSeenAt: &now,
			},
			{
				ID: f.groupID, Hostname: "group", AgentVersion: "1.0.0",
				RegisteredAt: &now, LastSeenAt: &now,
			},
			{
				ID: f.outsideID, Hostname: "outside", AgentVersion: "1.0.0",
				RegisteredAt: &now, LastSeenAt: &now,
			},
		} {
			if _, err := tx.InsertDevice(ctx, d); err != nil {
				return err
			}
			rec.Effect(deviceEffect(d.ID))
		}
		if _, err := tx.AssignDeviceUser(ctx, db.AssignDeviceUserParams{
			DeviceID: f.directID, UserID: f.actorID, AssignedAt: now, AssignedBy: f.actorID,
		}); err != nil {
			return err
		}
		if _, err := tx.AssignDeviceGroup(ctx, db.AssignDeviceGroupParams{
			DeviceID: f.groupID, GroupID: f.userGroup, AssignedAt: now, AssignedBy: f.actorID,
		}); err != nil {
			return err
		}
		return nil
	})
	require.NoError(t, err)

	_, err = raw.Exec(context.Background(), `
		INSERT INTO device_groups (id, name, inventory_interval_minutes)
		VALUES ($1, 'scope', 720)`, f.scopeGroup)
	require.NoError(t, err)
	_, err = raw.Exec(context.Background(), `
		INSERT INTO device_group_members (group_id, device_id) VALUES ($1, $2)`, f.scopeGroup, f.groupID)
	require.NoError(t, err)
	_, err = raw.Exec(context.Background(), `
		INSERT INTO device_inventory (device_id, table_name, rows, collected_at)
		VALUES
			($1, 'os_version', '[{"name":"Debian"}]', $2),
			($1, 'system_info', '[{"hostname":"group"}]', $2)`, f.groupID, now.Add(-time.Hour))
	require.NoError(t, err)

	f.handlers = device.New(device.Config{
		Store:            st,
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:              func() time.Time { return now },
		Decryptor:        encryptor,
		AgentSender:      f.sender,
		TerminalTokens:   f.tokens,
		TerminalSessions: f.sessions,
		TerminalURL:      "wss://control.example.test/terminal",
		IsConnected:      func(id string) bool { return f.connected[id] },
		CloseStream: func(id string) {
			f.closed = append(f.closed, id)
		},
	})
	return f
}

func (f *deviceHandlerFixture) actor(perms ...string) context.Context {
	f.t.Helper()
	return auth.WithUser(context.Background(), &auth.UserContext{
		ID: f.actorID, Kind: auth.PrincipalUser, Email: "actor@example.test", Permissions: perms,
	})
}

func TestDeviceHandlers_ValidateBeforeAuthentication(t *testing.T) {
	f := newDeviceHandlerFixture(t)
	_, err := validated(f.handlers.GetDevice)(context.Background(), connect.NewRequest(&cadestrov1.GetDeviceRequest{Id: &cadestrov1.DeviceId{Value: "bad"}}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, err = validated(f.handlers.GetDeviceInventory)(context.Background(),
		connect.NewRequest(&cadestrov1.GetDeviceInventoryRequest{DeviceId: &cadestrov1.DeviceId{Value: "bad"}}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, err = validated(f.handlers.GetOSQueryResult)(context.Background(),
		connect.NewRequest(&cadestrov1.GetOSQueryResultRequest{QueryId: &cadestrov1.QueryId{Value: "bad"}}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, err = validated(f.handlers.GetDeviceLogResult)(context.Background(),
		connect.NewRequest(&cadestrov1.GetDeviceLogResultRequest{QueryId: &cadestrov1.QueryId{Value: "bad"}}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, err = validated(f.handlers.GetDeviceCompliance)(context.Background(),
		connect.NewRequest(&cadestrov1.GetDeviceComplianceRequest{DeviceId: &cadestrov1.DeviceId{Value: "bad"}}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, err = validated(f.handlers.GetDeviceCompliancePolicyStatus)(context.Background(),
		connect.NewRequest(&cadestrov1.GetDeviceCompliancePolicyStatusRequest{DeviceId: &cadestrov1.DeviceId{Value: "bad"}}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, err = validated(f.handlers.ListLpsPasswords)(context.Background(),
		connect.NewRequest(&cadestrov1.ListLpsPasswordsRequest{DeviceId: &cadestrov1.DeviceId{Value: "bad"}}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, err = validated(f.handlers.RevealLpsPassword)(context.Background(),
		connect.NewRequest(&cadestrov1.RevealLpsPasswordRequest{Id: &cadestrov1.LpsPasswordId{Value: "bad"}}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, err = validated(f.handlers.ListLuksKeys)(context.Background(),
		connect.NewRequest(&cadestrov1.ListLuksKeysRequest{DeviceId: &cadestrov1.DeviceId{Value: "bad"}}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, err = validated(f.handlers.RevealLuksKey)(context.Background(),
		connect.NewRequest(&cadestrov1.RevealLuksKeyRequest{Id: &cadestrov1.LuksKeyId{Value: "bad"}}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, err = validated(f.handlers.CreateLuksToken)(context.Background(),
		connect.NewRequest(&cadestrov1.CreateLuksTokenRequest{DeviceId: &cadestrov1.DeviceId{Value: "bad"}, ActionId: &cadestrov1.ActionId{Value: "bad"}}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, err = validated(f.handlers.RevokeLuksDeviceKey)(context.Background(),
		connect.NewRequest(&cadestrov1.RevokeLuksDeviceKeyRequest{DeviceId: &cadestrov1.DeviceId{Value: "bad"}, ActionId: &cadestrov1.ActionId{Value: "bad"}}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, err = validated(f.handlers.DispatchOSQuery)(context.Background(),
		connect.NewRequest(&cadestrov1.DispatchOSQueryRequest{DeviceId: &cadestrov1.DeviceId{Value: "bad"}, Table: "packages"}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, err = validated(f.handlers.QueryDeviceLogs)(context.Background(),
		connect.NewRequest(&cadestrov1.QueryDeviceLogsRequest{DeviceId: &cadestrov1.DeviceId{Value: "bad"}}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, err = validated(f.handlers.RefreshDeviceInventory)(context.Background(),
		connect.NewRequest(&cadestrov1.RefreshDeviceInventoryRequest{DeviceId: &cadestrov1.DeviceId{Value: "bad"}}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, err = validated(f.handlers.StartTerminal)(context.Background(),
		connect.NewRequest(&cadestrov1.StartTerminalRequest{DeviceId: &cadestrov1.DeviceId{Value: "bad"}}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, err = validated(f.handlers.StopTerminal)(context.Background(),
		connect.NewRequest(&cadestrov1.StopTerminalRequest{SessionId: &cadestrov1.SessionId{Value: "bad"}}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, err = validated(f.handlers.TerminateTerminalSession)(context.Background(),
		connect.NewRequest(&cadestrov1.TerminateTerminalSessionRequest{SessionId: &cadestrov1.SessionId{Value: "bad"}}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, err = validated(f.handlers.ListActiveTerminalSessions)(context.Background(),
		connect.NewRequest(&cadestrov1.ListActiveTerminalSessionsRequest{PageToken: "bad"}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, err = validated(f.handlers.DispatchOSQuery)(context.Background(),
		connect.NewRequest(&cadestrov1.DispatchOSQueryRequest{DeviceId: &cadestrov1.DeviceId{Value: f.directID}}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), "custom shape validation must precede authentication")

	_, err = validated(f.handlers.GetDevice)(context.Background(), connect.NewRequest(&cadestrov1.GetDeviceRequest{Id: &cadestrov1.DeviceId{Value: f.directID}}))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	_, err = validated(f.handlers.GetDeviceInventory)(context.Background(),
		connect.NewRequest(&cadestrov1.GetDeviceInventoryRequest{DeviceId: &cadestrov1.DeviceId{Value: f.directID}}))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	_, err = validated(f.handlers.GetOSQueryResult)(context.Background(),
		connect.NewRequest(&cadestrov1.GetOSQueryResultRequest{QueryId: &cadestrov1.QueryId{Value: newID()}}))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	_, err = validated(f.handlers.GetDeviceLogResult)(context.Background(),
		connect.NewRequest(&cadestrov1.GetDeviceLogResultRequest{QueryId: &cadestrov1.QueryId{Value: newID()}}))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	_, err = validated(f.handlers.GetDeviceCompliance)(context.Background(),
		connect.NewRequest(&cadestrov1.GetDeviceComplianceRequest{DeviceId: &cadestrov1.DeviceId{Value: f.directID}}))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	_, err = validated(f.handlers.GetDeviceCompliancePolicyStatus)(context.Background(),
		connect.NewRequest(&cadestrov1.GetDeviceCompliancePolicyStatusRequest{DeviceId: &cadestrov1.DeviceId{Value: f.directID}}))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	_, err = validated(f.handlers.ListLpsPasswords)(context.Background(),
		connect.NewRequest(&cadestrov1.ListLpsPasswordsRequest{DeviceId: &cadestrov1.DeviceId{Value: f.directID}}))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	_, err = validated(f.handlers.RevealLpsPassword)(context.Background(),
		connect.NewRequest(&cadestrov1.RevealLpsPasswordRequest{Id: &cadestrov1.LpsPasswordId{Value: newID()}}))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	_, err = validated(f.handlers.ListLuksKeys)(context.Background(),
		connect.NewRequest(&cadestrov1.ListLuksKeysRequest{DeviceId: &cadestrov1.DeviceId{Value: f.directID}}))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	_, err = validated(f.handlers.RevealLuksKey)(context.Background(),
		connect.NewRequest(&cadestrov1.RevealLuksKeyRequest{Id: &cadestrov1.LuksKeyId{Value: newID()}}))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	_, err = validated(f.handlers.CreateLuksToken)(context.Background(),
		connect.NewRequest(&cadestrov1.CreateLuksTokenRequest{DeviceId: &cadestrov1.DeviceId{Value: f.directID}, ActionId: &cadestrov1.ActionId{Value: newID()}}))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	_, err = validated(f.handlers.RevokeLuksDeviceKey)(context.Background(),
		connect.NewRequest(&cadestrov1.RevokeLuksDeviceKeyRequest{DeviceId: &cadestrov1.DeviceId{Value: f.directID}, ActionId: &cadestrov1.ActionId{Value: newID()}}))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	_, err = validated(f.handlers.DispatchOSQuery)(context.Background(),
		connect.NewRequest(&cadestrov1.DispatchOSQueryRequest{DeviceId: &cadestrov1.DeviceId{Value: f.directID}, Table: "packages"}))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	_, err = validated(f.handlers.QueryDeviceLogs)(context.Background(),
		connect.NewRequest(&cadestrov1.QueryDeviceLogsRequest{DeviceId: &cadestrov1.DeviceId{Value: f.directID}}))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	_, err = validated(f.handlers.RefreshDeviceInventory)(context.Background(),
		connect.NewRequest(&cadestrov1.RefreshDeviceInventoryRequest{DeviceId: &cadestrov1.DeviceId{Value: f.directID}}))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	_, err = validated(f.handlers.StartTerminal)(context.Background(),
		connect.NewRequest(&cadestrov1.StartTerminalRequest{DeviceId: &cadestrov1.DeviceId{Value: f.directID}}))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	_, err = validated(f.handlers.StopTerminal)(context.Background(),
		connect.NewRequest(&cadestrov1.StopTerminalRequest{SessionId: &cadestrov1.SessionId{Value: newID()}}))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	_, err = validated(f.handlers.TerminateTerminalSession)(context.Background(),
		connect.NewRequest(&cadestrov1.TerminateTerminalSessionRequest{SessionId: &cadestrov1.SessionId{Value: newID()}}))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	_, err = validated(f.handlers.ListActiveTerminalSessions)(context.Background(),
		connect.NewRequest(&cadestrov1.ListActiveTerminalSessionsRequest{}))
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

func TestDeviceHandlers_AssignedAndScopedReads(t *testing.T) {
	f := newDeviceHandlerFixture(t)
	assignedCtx := f.actor("GetDevice:assigned", "ListDevices:assigned")

	for _, id := range []string{f.directID, f.groupID} {
		resp, err := f.handlers.GetDevice(assignedCtx, connect.NewRequest(&cadestrov1.GetDeviceRequest{Id: &cadestrov1.DeviceId{Value: id}}))
		require.NoError(t, err)
		assert.Equal(t, id, resp.Msg.Device.GetId().GetValue())
	}
	_, err := f.handlers.GetDevice(assignedCtx, connect.NewRequest(&cadestrov1.GetDeviceRequest{Id: &cadestrov1.DeviceId{Value: f.outsideID}}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "assigned-only reads must not reveal other devices")

	list, err := f.handlers.ListDevices(assignedCtx, connect.NewRequest(&cadestrov1.ListDevicesRequest{}))
	require.NoError(t, err)
	require.Len(t, list.Msg.Devices, 2)
	ids := []string{list.Msg.Devices[0].GetId().GetValue(), list.Msg.Devices[1].GetId().GetValue()}
	sort.Strings(ids)
	want := []string{f.directID, f.groupID}
	sort.Strings(want)
	assert.Equal(t, want, ids)
	assert.Equal(t, int32(2), list.Msg.TotalCount)
	firstPage, err := f.handlers.ListDevices(assignedCtx, connect.NewRequest(&cadestrov1.ListDevicesRequest{PageSize: 1}))
	require.NoError(t, err)
	require.Len(t, firstPage.Msg.Devices, 1)
	require.NotEmpty(t, firstPage.Msg.NextPageToken)
	secondPage, err := f.handlers.ListDevices(assignedCtx, connect.NewRequest(&cadestrov1.ListDevicesRequest{
		PageSize: 1, PageToken: firstPage.Msg.NextPageToken,
	}))
	require.NoError(t, err)
	require.Len(t, secondPage.Msg.Devices, 1)
	assert.Empty(t, secondPage.Msg.NextPageToken)
	assert.NotEqual(t, firstPage.Msg.Devices[0].GetId().GetValue(), secondPage.Msg.Devices[0].GetId().GetValue())
	assert.Equal(t, int32(2), secondPage.Msg.TotalCount)

	complianceCtx := f.actor("GetDeviceCompliance:assigned", "GetDeviceCompliancePolicyStatus:assigned")
	for _, id := range []string{f.directID, f.groupID} {
		_, err = f.handlers.GetDeviceCompliance(complianceCtx,
			connect.NewRequest(&cadestrov1.GetDeviceComplianceRequest{DeviceId: &cadestrov1.DeviceId{Value: id}}))
		require.NoError(t, err)
		_, err = f.handlers.GetDeviceCompliancePolicyStatus(complianceCtx,
			connect.NewRequest(&cadestrov1.GetDeviceCompliancePolicyStatusRequest{DeviceId: &cadestrov1.DeviceId{Value: id}}))
		require.NoError(t, err)
	}
	_, err = f.handlers.GetDeviceCompliance(complianceCtx,
		connect.NewRequest(&cadestrov1.GetDeviceComplianceRequest{DeviceId: &cadestrov1.DeviceId{Value: f.outsideID}}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "assigned compliance must not reveal other devices")
	_, err = f.handlers.GetDeviceCompliancePolicyStatus(complianceCtx,
		connect.NewRequest(&cadestrov1.GetDeviceCompliancePolicyStatusRequest{DeviceId: &cadestrov1.DeviceId{Value: f.outsideID}}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "assigned policy status must not reveal other devices")

	scopedCtx := auth.WithUser(context.Background(), &auth.UserContext{
		ID: f.actorID, Kind: auth.PrincipalUser,
		Permissions: []string{"GetDevice", "ListDevices"},
		ScopedGrants: []auth.ScopedGrant{
			{Permission: "GetDevice", ScopeKind: auth.ScopeKindDeviceGroup, ScopeID: f.scopeGroup},
			{Permission: "ListDevices", ScopeKind: auth.ScopeKindDeviceGroup, ScopeID: f.scopeGroup},
		},
	})
	_, err = f.handlers.GetDevice(scopedCtx, connect.NewRequest(&cadestrov1.GetDeviceRequest{Id: &cadestrov1.DeviceId{Value: f.outsideID}}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "scope misses must not reveal existence")
	list, err = f.handlers.ListDevices(scopedCtx, connect.NewRequest(&cadestrov1.ListDevicesRequest{}))
	require.NoError(t, err)
	require.Len(t, list.Msg.Devices, 1)
	assert.Equal(t, f.groupID, list.Msg.Devices[0].GetId().GetValue())
	assert.NotNil(t, list.Msg.Devices[0].LastInventoryAt)
	assert.False(t, list.Msg.Devices[0].InventoryOverdue)
}

func TestDeviceHandlers_GetDeviceInventoryReadsDirectTables(t *testing.T) {
	f := newDeviceHandlerFixture(t)
	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		ID: f.actorID, Kind: auth.PrincipalUser,
		Permissions: []string{"GetDeviceInventory"},
		ScopedGrants: []auth.ScopedGrant{{
			Permission: "GetDeviceInventory", ScopeKind: auth.ScopeKindDeviceGroup, ScopeID: f.scopeGroup,
		}},
	})

	response, err := f.handlers.GetDeviceInventory(ctx,
		connect.NewRequest(&cadestrov1.GetDeviceInventoryRequest{DeviceId: &cadestrov1.DeviceId{Value: f.groupID}}))
	require.NoError(t, err)
	require.Len(t, response.Msg.Tables, 2)
	assert.Equal(t, "os_version", response.Msg.Tables[0].TableName)
	require.Len(t, response.Msg.Tables[0].Rows, 1)
	assert.Equal(t, "Debian", response.Msg.Tables[0].Rows[0].Data["name"])
	assert.True(t, response.Msg.Tables[0].CollectedAt.AsTime().Equal(f.now.Add(-time.Hour)))
	assert.Equal(t, "system_info", response.Msg.Tables[1].TableName)

	filtered, err := f.handlers.GetDeviceInventory(ctx,
		connect.NewRequest(&cadestrov1.GetDeviceInventoryRequest{
			DeviceId: &cadestrov1.DeviceId{Value: f.groupID}, TableNames: []string{"system_info"},
		}))
	require.NoError(t, err)
	require.Len(t, filtered.Msg.Tables, 1)
	assert.Equal(t, "group", filtered.Msg.Tables[0].Rows[0].Data["hostname"])

	_, err = f.handlers.GetDeviceInventory(ctx,
		connect.NewRequest(&cadestrov1.GetDeviceInventoryRequest{DeviceId: &cadestrov1.DeviceId{Value: f.outsideID}}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "scope must not disclose the device")

	_, err = f.handlers.GetDeviceInventory(ctx,
		connect.NewRequest(&cadestrov1.GetDeviceInventoryRequest{
			DeviceId: &cadestrov1.DeviceId{Value: f.groupID}, TableNames: make([]string, 129),
		}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assertSensitiveDeviceRead(t, f, cadestrov1connect.ControlServiceGetDeviceInventoryProcedure,
		"device_inventory", f.groupID)
}

func TestDeviceHandlers_GetDeviceInventoryRejectsInvalidStoredShape(t *testing.T) {
	f := newDeviceHandlerFixture(t)
	_, err := f.raw.Exec(context.Background(),
		`UPDATE device_inventory SET rows = '{"not":"rows"}' WHERE device_id = $1`, f.groupID)
	require.NoError(t, err)

	_, err = f.handlers.GetDeviceInventory(f.actor("GetDeviceInventory"),
		connect.NewRequest(&cadestrov1.GetDeviceInventoryRequest{DeviceId: &cadestrov1.DeviceId{Value: f.groupID}}))
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err), "corrupt inventory must not look like an empty table")
}

func TestDeviceHandlers_GetOSQueryResultReadsDirectState(t *testing.T) {
	f := newDeviceHandlerFixture(t)
	completedID, pendingID, staleID, outsideID := newID(), newID(), newID(), newID()
	for _, row := range []struct {
		id, deviceID string
		completed    bool
		createdAt    time.Time
		rows         string
	}{
		{completedID, f.groupID, true, f.now.Add(-time.Minute), `[{"package":"bash"}]`},
		{pendingID, f.groupID, false, f.now.Add(-time.Minute), `[]`},
		{staleID, f.groupID, false, f.now.Add(-6 * time.Minute), `[]`},
		{outsideID, f.outsideID, true, f.now.Add(-time.Minute), `[]`},
	} {
		_, err := f.raw.Exec(context.Background(), `
			INSERT INTO osquery_results
				(query_id, device_id, table_name, completed, success, rows, created_at)
			VALUES ($1, $2, 'packages', $3, $3, $4, $5)`,
			row.id, row.deviceID, row.completed, row.rows, row.createdAt)
		require.NoError(t, err)
	}
	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		ID: f.actorID, Kind: auth.PrincipalUser,
		Permissions: []string{"GetOSQueryResult"},
		ScopedGrants: []auth.ScopedGrant{{
			Permission: "GetOSQueryResult", ScopeKind: auth.ScopeKindDeviceGroup, ScopeID: f.scopeGroup,
		}},
	})

	completed, err := f.handlers.GetOSQueryResult(ctx,
		connect.NewRequest(&cadestrov1.GetOSQueryResultRequest{QueryId: &cadestrov1.QueryId{Value: completedID}}))
	require.NoError(t, err)
	assert.True(t, completed.Msg.Completed)
	assert.True(t, completed.Msg.Success)
	require.Len(t, completed.Msg.Rows, 1)
	assert.Equal(t, "bash", completed.Msg.Rows[0].Data["package"])

	pending, err := f.handlers.GetOSQueryResult(ctx,
		connect.NewRequest(&cadestrov1.GetOSQueryResultRequest{QueryId: &cadestrov1.QueryId{Value: pendingID}}))
	require.NoError(t, err)
	assert.False(t, pending.Msg.Completed)

	stale, err := f.handlers.GetOSQueryResult(ctx,
		connect.NewRequest(&cadestrov1.GetOSQueryResultRequest{QueryId: &cadestrov1.QueryId{Value: staleID}}))
	require.NoError(t, err)
	assert.True(t, stale.Msg.Completed)
	assert.False(t, stale.Msg.Success)
	assert.Contains(t, stale.Msg.Error, "timed out")
	var storedCompleted bool
	require.NoError(t, f.raw.QueryRow(context.Background(),
		`SELECT completed FROM osquery_results WHERE query_id = $1`, staleID).Scan(&storedCompleted))
	assert.False(t, storedCompleted, "a read must not smuggle in an unaudited expiry mutation")

	_, err = f.handlers.GetOSQueryResult(ctx,
		connect.NewRequest(&cadestrov1.GetOSQueryResultRequest{QueryId: &cadestrov1.QueryId{Value: outsideID}}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "scope must not disclose the result")
	_, err = f.handlers.GetOSQueryResult(ctx,
		connect.NewRequest(&cadestrov1.GetOSQueryResultRequest{QueryId: &cadestrov1.QueryId{Value: newID()}}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	assertSensitiveDeviceRead(t, f, cadestrov1connect.ControlServiceGetOSQueryResultProcedure,
		"osquery_result", staleID)
}

func TestDeviceHandlers_GetOSQueryResultRejectsInvalidStoredShape(t *testing.T) {
	f := newDeviceHandlerFixture(t)
	queryID := newID()
	_, err := f.raw.Exec(context.Background(), `
		INSERT INTO osquery_results
			(query_id, device_id, table_name, completed, success, rows, created_at)
		VALUES ($1, $2, 'packages', TRUE, TRUE, '{"not":"rows"}', $3)`, queryID, f.groupID, f.now)
	require.NoError(t, err)

	_, err = f.handlers.GetOSQueryResult(f.actor("GetOSQueryResult"),
		connect.NewRequest(&cadestrov1.GetOSQueryResultRequest{QueryId: &cadestrov1.QueryId{Value: queryID}}))
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err), "corrupt rows must not look like an empty result")
}

func TestDeviceHandlers_GetDeviceLogResultReadsDirectState(t *testing.T) {
	f := newDeviceHandlerFixture(t)
	completedID, staleID, outsideID := newID(), newID(), newID()
	for _, row := range []struct {
		id, deviceID string
		completed    bool
		createdAt    time.Time
		logs         string
	}{
		{completedID, f.groupID, true, f.now.Add(-time.Minute), "service started\n"},
		{staleID, f.groupID, false, f.now.Add(-6 * time.Minute), ""},
		{outsideID, f.outsideID, true, f.now.Add(-time.Minute), "hidden\n"},
	} {
		_, err := f.raw.Exec(context.Background(), `
			INSERT INTO log_query_results
				(query_id, device_id, completed, success, logs, created_at)
			VALUES ($1, $2, $3, $3, $4, $5)`,
			row.id, row.deviceID, row.completed, row.logs, row.createdAt)
		require.NoError(t, err)
	}
	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		ID: f.actorID, Kind: auth.PrincipalUser,
		Permissions: []string{"GetDeviceLogResult"},
		ScopedGrants: []auth.ScopedGrant{{
			Permission: "GetDeviceLogResult", ScopeKind: auth.ScopeKindDeviceGroup, ScopeID: f.scopeGroup,
		}},
	})

	completed, err := f.handlers.GetDeviceLogResult(ctx,
		connect.NewRequest(&cadestrov1.GetDeviceLogResultRequest{QueryId: &cadestrov1.QueryId{Value: completedID}}))
	require.NoError(t, err)
	assert.True(t, completed.Msg.Completed)
	assert.True(t, completed.Msg.Success)
	assert.Equal(t, "service started\n", completed.Msg.Logs)

	stale, err := f.handlers.GetDeviceLogResult(ctx,
		connect.NewRequest(&cadestrov1.GetDeviceLogResultRequest{QueryId: &cadestrov1.QueryId{Value: staleID}}))
	require.NoError(t, err)
	assert.True(t, stale.Msg.Completed)
	assert.False(t, stale.Msg.Success)
	assert.Empty(t, stale.Msg.Logs)
	assert.Contains(t, stale.Msg.Error, "timed out")
	var storedCompleted bool
	require.NoError(t, f.raw.QueryRow(context.Background(),
		`SELECT completed FROM log_query_results WHERE query_id = $1`, staleID).Scan(&storedCompleted))
	assert.False(t, storedCompleted, "a read must not smuggle in an unaudited expiry mutation")

	_, err = f.handlers.GetDeviceLogResult(ctx,
		connect.NewRequest(&cadestrov1.GetDeviceLogResultRequest{QueryId: &cadestrov1.QueryId{Value: outsideID}}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "scope must not disclose the result")
	_, err = f.handlers.GetDeviceLogResult(ctx,
		connect.NewRequest(&cadestrov1.GetDeviceLogResultRequest{QueryId: &cadestrov1.QueryId{Value: newID()}}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	assertSensitiveDeviceRead(t, f, cadestrov1connect.ControlServiceGetDeviceLogResultProcedure,
		"device_log_result", staleID)
}

func TestDeviceHandlers_SecretListsAreMetadataAndRevealsAreIndividuallyAudited(t *testing.T) {
	f := newDeviceHandlerFixture(t)
	lpsActionID, luksActionID := newID(), newID()
	_, err := f.raw.Exec(context.Background(), `
		INSERT INTO actions (id, name, action_type, params, created_by) VALUES
			($1, 'Local admin', $2, '{}', $3),
			($4, 'Root disk', $5, '{}', $3)`,
		lpsActionID, int32(cadestrov1.ActionType_ACTION_TYPE_LPS), f.actorID,
		luksActionID, int32(cadestrov1.ActionType_ACTION_TYPE_ENCRYPTION))
	require.NoError(t, err)
	// The reveal handlers compute the at-rest AAD from the row's immutable id
	// (secret.ID), not from the username/device_path shared by every rotation
	// row. Generate the row ids first, then seal the CURRENT row's ciphertext
	// under its own id: only the revealed (current) row need open.
	lpsIDs := make([]string, 5)
	luksIDs := make([]string, 5)
	for i := range lpsIDs {
		lpsIDs[i], luksIDs[i] = newID(), newID()
	}
	for i := 0; i < 5; i++ {
		current := i == 0
		rotatedAt := f.now.Add(-time.Duration(i) * time.Hour)
		password, err := f.encryptor.EncryptWithContext("local-secret",
			crypto.DeviceSecretAAD(lpsIDs[i], f.directID, "lps", lpsActionID, 1))
		require.NoError(t, err)
		passphrase, err := f.encryptor.EncryptWithContext("disk-secret",
			crypto.DeviceSecretAAD(luksIDs[i], f.directID, "luks", luksActionID, 1))
		require.NoError(t, err)
		_, err = f.raw.Exec(context.Background(), `INSERT INTO device_secrets (id, device_id, kind, subject, version, ciphertext) VALUES ($1, $2, 'lps', $3, 1, $4)`, lpsIDs[i], f.directID, lpsActionID, password)
		require.NoError(t, err)
		_, err = f.raw.Exec(context.Background(), `INSERT INTO device_secrets (id, device_id, kind, subject, version, ciphertext) VALUES ($1, $2, 'luks', $3, 1, $4)`, luksIDs[i], f.directID, luksActionID, passphrase)
		require.NoError(t, err)
		_, err = f.raw.Exec(context.Background(), `
			INSERT INTO lps_passwords
				(id, username, rotated_at, rotation_reason, is_current)
			VALUES ($1, 'localadmin', $2, 'scheduled', $3)`, lpsIDs[i], rotatedAt, current)
		require.NoError(t, err)
		_, err = f.raw.Exec(context.Background(), `
			INSERT INTO luks_keys
				(id, device_path, rotated_at, rotation_reason, is_current, revocation_status)
			VALUES ($1, '/dev/vda', $2, 'initial', $3, 'dispatched')`, luksIDs[i], rotatedAt, current)
		require.NoError(t, err)
	}

	lps, err := f.handlers.ListLpsPasswords(f.actor("ListLpsPasswords"),
		connect.NewRequest(&cadestrov1.ListLpsPasswordsRequest{DeviceId: &cadestrov1.DeviceId{Value: f.directID}}))
	require.NoError(t, err)
	require.Len(t, lps.Msg.Current, 1)
	require.Len(t, lps.Msg.History, 3)
	assert.Equal(t, lpsIDs[0], lps.Msg.Current[0].GetId().GetValue())
	assert.Equal(t, "direct", lps.Msg.History[0].DeviceHostname)
	assert.Equal(t, "Local admin", lps.Msg.Current[0].ActionName)

	luks, err := f.handlers.ListLuksKeys(f.actor("ListLuksKeys"),
		connect.NewRequest(&cadestrov1.ListLuksKeysRequest{DeviceId: &cadestrov1.DeviceId{Value: f.directID}}))
	require.NoError(t, err)
	require.Len(t, luks.Msg.Current, 1)
	require.Len(t, luks.Msg.History, 3)
	assert.Equal(t, luksIDs[0], luks.Msg.Current[0].GetId().GetValue())
	assert.Equal(t, "Root disk", luks.Msg.Current[0].ActionName)
	assert.Equal(t, cadestrov1.LuksRevocationStatus_LUKS_REVOCATION_STATUS_DISPATCHED,
		luks.Msg.Current[0].RevocationStatus)

	assertSensitiveDeviceRead(t, f,
		cadestrov1connect.ControlServiceListLpsPasswordsProcedure,
		"device_lps_passwords", f.directID)
	assertSensitiveDeviceRead(t, f,
		cadestrov1connect.ControlServiceListLuksKeysProcedure,
		"device_luks_keys", f.directID)

	_, err = f.handlers.RevealLpsPassword(f.actor("ListLpsPasswords"),
		connect.NewRequest(&cadestrov1.RevealLpsPasswordRequest{Id: &cadestrov1.LpsPasswordId{Value: lpsIDs[0]}}))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err),
		"metadata access must not imply plaintext access")
	lpsReveal, err := f.handlers.RevealLpsPassword(f.actor("RevealLpsPassword"),
		connect.NewRequest(&cadestrov1.RevealLpsPasswordRequest{Id: &cadestrov1.LpsPasswordId{Value: lpsIDs[0]}}))
	require.NoError(t, err)
	assert.Equal(t, "local-secret", lpsReveal.Msg.Password)
	assertSecretReveal(t, f, cadestrov1connect.ControlServiceRevealLpsPasswordProcedure,
		"lps_password", lpsIDs[0], f.directID, lpsActionID)

	luksReveal, err := f.handlers.RevealLuksKey(f.actor("RevealLuksKey"),
		connect.NewRequest(&cadestrov1.RevealLuksKeyRequest{Id: &cadestrov1.LuksKeyId{Value: luksIDs[0]}}))
	require.NoError(t, err)
	assert.Equal(t, "disk-secret", luksReveal.Msg.Passphrase)
	assertSecretReveal(t, f, cadestrov1connect.ControlServiceRevealLuksKeyProcedure,
		"luks_key", luksIDs[0], f.directID, luksActionID)

	_, err = f.raw.Exec(context.Background(), `
		UPDATE device_secrets SET ciphertext = 'enc:v1:not-base64'
		WHERE id = $1`, lpsIDs[0])
	require.NoError(t, err)
	_, err = f.handlers.ListLpsPasswords(f.actor("ListLpsPasswords"),
		connect.NewRequest(&cadestrov1.ListLpsPasswordsRequest{DeviceId: &cadestrov1.DeviceId{Value: f.directID}}))
	require.NoError(t, err, "metadata listing must not open ciphertext")
	_, err = f.handlers.RevealLpsPassword(f.actor("RevealLpsPassword"),
		connect.NewRequest(&cadestrov1.RevealLpsPasswordRequest{Id: &cadestrov1.LpsPasswordId{Value: lpsIDs[0]}}))
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err), "corrupt ciphertext must fail closed")
	_, err = f.raw.Exec(context.Background(), `
		UPDATE device_secrets SET ciphertext = 'enc:v1:not-base64'
		WHERE id = $1`, lpsIDs[0])
	require.NoError(t, err)
	_, err = f.handlers.RevealLpsPassword(f.actor("RevealLpsPassword"),
		connect.NewRequest(&cadestrov1.RevealLpsPasswordRequest{Id: &cadestrov1.LpsPasswordId{Value: lpsIDs[0]}}))
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err), "plaintext storage must not get a compatibility path")

	rejectAuditOperation(t, f.raw, "/cadestro.v1.ControlService/RevealLuksKey")
	blocked, err := f.handlers.RevealLuksKey(f.actor("RevealLuksKey"),
		connect.NewRequest(&cadestrov1.RevealLuksKeyRequest{Id: &cadestrov1.LuksKeyId{Value: luksIDs[1]}}))
	assert.Nil(t, blocked)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err),
		"audit persistence failure must prevent the plaintext response")
}

func TestDeviceHandlers_CreateLuksTokenIsOwnerOnlyHashedAndAudited(t *testing.T) {
	f := newDeviceHandlerFixture(t)
	actionID := newID()
	_, err := f.raw.Exec(context.Background(), `
		INSERT INTO actions (id, name, action_type, params, created_by)
		VALUES ($1, 'Encryption', $2,
			'{"presharedKey":"enc:v1:stored","userPassphraseMinLength":24,"userPassphraseComplexity":"LPS_PASSWORD_COMPLEXITY_COMPLEX"}', $3)`,
		actionID, int32(cadestrov1.ActionType_ACTION_TYPE_ENCRYPTION), f.actorID)
	require.NoError(t, err)
	ctx := f.actor("CreateLuksToken")

	issued, err := f.handlers.CreateLuksToken(ctx, connect.NewRequest(&cadestrov1.CreateLuksTokenRequest{
		DeviceId: &cadestrov1.DeviceId{Value: f.directID}, ActionId: &cadestrov1.ActionId{Value: actionID},
	}))
	require.NoError(t, err)
	_, err = ulid.ParseStrict(issued.Msg.Token)
	require.NoError(t, err)
	assert.Contains(t, issued.Msg.Uri, issued.Msg.Token)
	// The agent's URI handler rewrites the scheme by STRICT prefix and exits
	// non-zero on anything else (agent cmd/cadestrod/cmd_luks.go runLuksURI),
	// and the operator pastes CliCommand into a shell. Both values therefore
	// name device-side artifacts that live in the agent module, so a rename on
	// only one side of that boundary produces a link the agent refuses and a
	// command the host cannot find — with no compile error anywhere. These two
	// assertions are the only thing that fails when that happens.
	assert.True(t, strings.HasPrefix(issued.Msg.Uri, "cadestro://"),
		"the issued URI must use the scheme the agent's handler accepts; got %q", issued.Msg.Uri)
	assert.True(t, strings.HasPrefix(issued.Msg.CliCommand, "cadestrod "),
		"CliCommand must invoke the installed agent daemon by its real name; got %q", issued.Msg.CliCommand)
	// F2: the advertised command must NOT carry the token on argv —
	// /proc/<pid>/cmdline is world-readable and the client reads the passphrase
	// before it dials, so an argv token is exposed for the whole typing window
	// while being the sole authorization for a root daemon that writes LUKS
	// keyslots. It must not advertise sudo either: the sudoers rule was removed
	// precisely so this client is unprivileged, and an operator copying the
	// string back would reinstate the escalation.
	assert.NotContains(t, issued.Msg.CliCommand, issued.Msg.Token,
		"CliCommand must not put the one-time LUKS token on argv")
	assert.NotContains(t, issued.Msg.CliCommand, "sudo",
		"the LUKS passphrase client is unprivileged; advertising sudo reinstates the removed escalation")
	assert.Contains(t, issued.Msg.CliCommand, "luks set-passphrase",
		"CliCommand must still name the command the operator runs")
	hash := sha256.Sum256([]byte(issued.Msg.Token))
	var storedHash string
	var minLength, complexity int32
	var expiresAt time.Time
	err = f.raw.QueryRow(context.Background(), `
		SELECT token, min_length, complexity, expires_at
		FROM luks_tokens WHERE device_id = $1 AND action_id = $2`,
		f.directID, actionID).Scan(&storedHash, &minLength, &complexity, &expiresAt)
	require.NoError(t, err)
	assert.Equal(t, hex.EncodeToString(hash[:]), storedHash)
	assert.NotEqual(t, issued.Msg.Token, storedHash)
	assert.Equal(t, int32(24), minLength)
	assert.Equal(t, int32(cadestrov1.LpsPasswordComplexity_LPS_PASSWORD_COMPLEXITY_COMPLEX), complexity)
	assert.True(t, expiresAt.Equal(f.now.Add(24*time.Hour)))
	operation, err := latestOperationFor(t, f.store, f.raw,
		cadestrov1connect.ControlServiceCreateLuksTokenProcedure)
	require.NoError(t, err)
	effects, err := f.store.ListAuditEffects(context.Background(), operation.OperationID)
	require.NoError(t, err)
	require.Len(t, effects, 1)
	assert.Equal(t, "luks_token", effects[0].ResourceType)
	assert.NotContains(t, strings.Join(effects[0].ChangedFields, ","), "token")

	_, err = f.handlers.CreateLuksToken(ctx, connect.NewRequest(&cadestrov1.CreateLuksTokenRequest{
		DeviceId: &cadestrov1.DeviceId{Value: f.groupID}, ActionId: &cadestrov1.ActionId{Value: actionID},
	}))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err),
		"group-derived visibility is not direct device ownership")

	_, err = f.raw.Exec(context.Background(), `PRAGMA ignore_check_constraints = ON`)
	require.NoError(t, err)
	_, err = f.raw.Exec(context.Background(), `UPDATE actions SET params = '"corrupt"' WHERE id = $1`, actionID)
	require.NoError(t, err)
	_, err = f.handlers.CreateLuksToken(ctx, connect.NewRequest(&cadestrov1.CreateLuksTokenRequest{
		DeviceId: &cadestrov1.DeviceId{Value: f.directID}, ActionId: &cadestrov1.ActionId{Value: actionID},
	}))
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err), "corrupt policy must not fall back")
	_, err = f.raw.Exec(context.Background(), `UPDATE actions SET params = '{}' WHERE id = $1`, actionID)
	require.NoError(t, err)
	_, err = f.raw.Exec(context.Background(), `PRAGMA ignore_check_constraints = OFF`)
	require.NoError(t, err)

	rejectAuditOperation(t, f.raw, "/cadestro.v1.ControlService/CreateLuksToken")
	_, err = f.handlers.CreateLuksToken(ctx, connect.NewRequest(&cadestrov1.CreateLuksTokenRequest{
		DeviceId: &cadestrov1.DeviceId{Value: f.directID}, ActionId: &cadestrov1.ActionId{Value: actionID},
	}))
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	var tokenCount int
	err = f.raw.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM luks_tokens WHERE device_id = $1 AND action_id = $2`,
		f.directID, actionID).Scan(&tokenCount)
	require.NoError(t, err)
	assert.Equal(t, 1, tokenCount, "audit failure must roll the token insert back")
}

func TestDeviceHandlers_RevokeLuksDeviceKeyUsesDirectMTLSStream(t *testing.T) {
	f := newDeviceHandlerFixture(t)
	actionID := newID()
	ctx := f.actor("RevokeLuksDeviceKey")

	_, err := f.handlers.RevokeLuksDeviceKey(f.actor(), connect.NewRequest(&cadestrov1.RevokeLuksDeviceKeyRequest{
		DeviceId: &cadestrov1.DeviceId{Value: f.directID}, ActionId: &cadestrov1.ActionId{Value: actionID},
	}))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	_, err = f.handlers.RevokeLuksDeviceKey(ctx, connect.NewRequest(&cadestrov1.RevokeLuksDeviceKeyRequest{
		DeviceId: &cadestrov1.DeviceId{Value: f.directID}, ActionId: &cadestrov1.ActionId{Value: actionID},
	}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))

	seedCurrentLuksKeys(t, f, actionID, 2)
	_, err = f.handlers.RevokeLuksDeviceKey(ctx, connect.NewRequest(&cadestrov1.RevokeLuksDeviceKeyRequest{
		DeviceId: &cadestrov1.DeviceId{Value: f.directID}, ActionId: &cadestrov1.ActionId{Value: actionID},
	}))
	require.NoError(t, err)
	require.Len(t, f.sender.messages, 1)
	message := f.sender.messages[0]
	_, err = ulid.ParseStrict(message.GetId().GetValue())
	require.NoError(t, err)
	require.NotNil(t, message.GetRevokeLuksDeviceKey())
	assert.Equal(t, actionID, message.GetRevokeLuksDeviceKey().GetActionId().GetValue())

	var dispatched int
	err = f.raw.QueryRow(context.Background(), `
		SELECT count(*) FROM luks_keys k JOIN device_secrets ds ON ds.id = k.id
		WHERE ds.device_id = $1 AND ds.subject = $2 AND k.revocation_status = 'dispatched'`,
		f.directID, actionID).Scan(&dispatched)
	require.NoError(t, err)
	assert.Equal(t, 2, dispatched)
	operation, err := latestOperationFor(t, f.store, f.raw,
		cadestrov1connect.ControlServiceRevokeLuksDeviceKeyProcedure)
	require.NoError(t, err)
	effects, err := f.store.ListAuditEffects(context.Background(), operation.OperationID)
	require.NoError(t, err)
	require.Len(t, effects, 1)
	assert.Equal(t, "luks_key_action", effects[0].ResourceType)
	assert.Equal(t, actionID, effects[0].ResourceID)
	assert.Equal(t, int64(2), *effects[0].AfterCount)

	_, err = f.handlers.RevokeLuksDeviceKey(ctx, connect.NewRequest(&cadestrov1.RevokeLuksDeviceKeyRequest{
		DeviceId: &cadestrov1.DeviceId{Value: f.directID}, ActionId: &cadestrov1.ActionId{Value: actionID},
	}))
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Len(t, f.sender.messages, 1, "a pending revocation must not be sent twice")

	require.NoError(t, f.handlers.CompleteLuksKeyRevocation(context.Background(), f.directID,
		&cadestrov1.RevokeLuksDeviceKeyResult{ActionId: &cadestrov1.ActionId{Value: actionID}, Success: true}))
	var succeeded int
	err = f.raw.QueryRow(context.Background(), `
		SELECT count(*) FROM luks_keys k JOIN device_secrets ds ON ds.id = k.id
		WHERE ds.device_id = $1 AND ds.subject = $2 AND k.revocation_status = 'success'
		  AND revocation_error IS NULL`, f.directID, actionID).Scan(&succeeded)
	require.NoError(t, err)
	assert.Equal(t, 2, succeeded)

	// A replay is absorbed by the conditional update and preserved as rejected
	// evidence instead of changing the terminal state again.
	require.NoError(t, f.handlers.CompleteLuksKeyRevocation(context.Background(), f.directID,
		&cadestrov1.RevokeLuksDeviceKeyResult{ActionId: &cadestrov1.ActionId{Value: actionID}, Success: false, Error: "stale"}))
	resultOperation, err := latestOperationFor(t, f.store, f.raw,
		"cadestro.v1.AgentService.Stream/RevokeLuksDeviceKeyResult")
	require.NoError(t, err)
	resultEffects, err := f.store.ListAuditEffects(context.Background(), resultOperation.OperationID)
	require.NoError(t, err)
	require.Len(t, resultEffects, 1)
	assert.Equal(t, string(store.EffectRejected), resultEffects[0].Outcome)
}

func TestDeviceHandlers_RevokeLuksDeviceKeyRecordsUnavailableDevice(t *testing.T) {
	f := newDeviceHandlerFixture(t)
	actionID := newID()
	seedCurrentLuksKeys(t, f, actionID, 1)
	f.sender.err = errors.New("agent not connected")

	_, err := f.handlers.RevokeLuksDeviceKey(f.actor("RevokeLuksDeviceKey"),
		connect.NewRequest(&cadestrov1.RevokeLuksDeviceKeyRequest{DeviceId: &cadestrov1.DeviceId{Value: f.directID}, ActionId: &cadestrov1.ActionId{Value: actionID}}))
	assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
	var status string
	var detail *string
	err = f.raw.QueryRow(context.Background(), `
		SELECT k.revocation_status, k.revocation_error FROM luks_keys k JOIN device_secrets ds ON ds.id = k.id
		WHERE ds.device_id = $1 AND ds.subject = $2 AND k.is_current = TRUE`,
		f.directID, actionID).Scan(&status, &detail)
	require.NoError(t, err)
	assert.Equal(t, "failed", status)
	require.NotNil(t, detail)
	assert.Equal(t, "device unavailable", *detail, "transport internals must not enter durable state")
	operation, err := latestOperationFor(t, f.store, f.raw,
		cadestrov1connect.ControlServiceRevokeLuksDeviceKeyProcedure)
	require.NoError(t, err)
	effects, err := f.store.ListAuditEffects(context.Background(), operation.OperationID)
	require.NoError(t, err)
	require.Len(t, effects, 2)
	assert.Equal(t, string(store.EffectApplied), effects[0].Outcome)
	assert.Equal(t, string(store.EffectFailed), effects[1].Outcome)
}

func TestDeviceHandlers_RevokeLuksDeviceKeyAuditFailurePreventsSend(t *testing.T) {
	f := newDeviceHandlerFixture(t)
	actionID := newID()
	seedCurrentLuksKeys(t, f, actionID, 1)
	rejectAuditOperation(t, f.raw, "/cadestro.v1.ControlService/RevokeLuksDeviceKey")

	_, err := f.handlers.RevokeLuksDeviceKey(f.actor("RevokeLuksDeviceKey"),
		connect.NewRequest(&cadestrov1.RevokeLuksDeviceKeyRequest{DeviceId: &cadestrov1.DeviceId{Value: f.directID}, ActionId: &cadestrov1.ActionId{Value: actionID}}))
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	assert.Empty(t, f.sender.messages, "the irreversible command must not leave before its audit commits")
	var status *string
	err = f.raw.QueryRow(context.Background(), `
		SELECT k.revocation_status FROM luks_keys k JOIN device_secrets ds ON ds.id = k.id
		WHERE ds.device_id = $1 AND ds.subject = $2 AND k.is_current = TRUE`,
		f.directID, actionID).Scan(&status)
	require.NoError(t, err)
	assert.Nil(t, status, "the state mutation must roll back with failed audit evidence")
}

func seedCurrentLuksKeys(t *testing.T, f *deviceHandlerFixture, actionID string, count int) {
	t.Helper()
	_, err := f.raw.Exec(context.Background(), `
		INSERT INTO actions (id, name, action_type, params, created_by)
		VALUES ($1, 'Encryption', $2, '{}', $3)
		ON CONFLICT (id) DO NOTHING`,
		actionID, int32(cadestrov1.ActionType_ACTION_TYPE_ENCRYPTION), f.actorID)
	require.NoError(t, err)
	for i := 0; i < count; i++ {
		id := newID()
		_, err = f.raw.Exec(context.Background(), `INSERT INTO device_secrets (id, device_id, kind, subject, version, ciphertext) VALUES ($1, $2, 'luks', $3, 1, 'enc:v1:test')`, id, f.directID, actionID)
		require.NoError(t, err)
		_, err = f.raw.Exec(context.Background(), `
			INSERT INTO luks_keys
				(id, device_path, rotated_at, rotation_reason, is_current)
			VALUES ($1, $2, $3, 'scheduled', TRUE)`, id, fmt.Sprintf("/dev/test%d", i), f.now)
		require.NoError(t, err)
	}
}

func TestDeviceHandlers_InstantQueriesUseDirectStreamAndSQLiteResults(t *testing.T) {
	f := newDeviceHandlerFixture(t)
	ctx := f.actor("DispatchOSQuery", "QueryDeviceLogs", "RefreshDeviceInventory")

	osquery, err := f.handlers.DispatchOSQuery(ctx, connect.NewRequest(&cadestrov1.DispatchOSQueryRequest{
		DeviceId: &cadestrov1.DeviceId{Value: f.directID}, Table: "packages", Columns: []string{"name"}, Limit: 25,
	}))
	require.NoError(t, err)
	require.Len(t, f.sender.messages, 1)
	queryFrame := f.sender.messages[0]
	assert.Equal(t, osquery.Msg.GetQueryId().GetValue(), queryFrame.GetId().GetValue())
	require.NotNil(t, queryFrame.GetQuery())
	assert.Equal(t, "packages", queryFrame.GetQuery().Table)
	assert.Equal(t, []string{"name"}, queryFrame.GetQuery().Columns)
	var osCompleted, osSuccess bool
	var osTable string
	err = f.raw.QueryRow(context.Background(), `
		SELECT table_name, completed, success FROM osquery_results WHERE query_id = $1`,
		osquery.Msg.GetQueryId().GetValue()).Scan(&osTable, &osCompleted, &osSuccess)
	require.NoError(t, err)
	assert.Equal(t, "packages", osTable)
	assert.False(t, osCompleted)
	assert.False(t, osSuccess)

	logs, err := f.handlers.QueryDeviceLogs(ctx, connect.NewRequest(&cadestrov1.QueryDeviceLogsRequest{
		DeviceId: &cadestrov1.DeviceId{Value: f.directID}, Lines: 100, Unit: "sshd.service", Priority: "warning",
	}))
	require.NoError(t, err)
	require.Len(t, f.sender.messages, 2)
	logFrame := f.sender.messages[1]
	assert.Equal(t, logs.Msg.GetQueryId().GetValue(), logFrame.GetId().GetValue())
	require.NotNil(t, logFrame.GetLogQuery())
	assert.Equal(t, "sshd.service", logFrame.GetLogQuery().Unit)
	var logCompleted bool
	require.NoError(t, f.raw.QueryRow(context.Background(), `
		SELECT completed FROM log_query_results WHERE query_id = $1`, logs.Msg.GetQueryId().GetValue()).Scan(&logCompleted))
	assert.False(t, logCompleted)

	_, err = f.handlers.RefreshDeviceInventory(ctx,
		connect.NewRequest(&cadestrov1.RefreshDeviceInventoryRequest{DeviceId: &cadestrov1.DeviceId{Value: f.directID}}))
	require.NoError(t, err)
	require.Len(t, f.sender.messages, 3)
	refreshFrame := f.sender.messages[2]
	require.NotNil(t, refreshFrame.GetRequestInventory())
	assert.Equal(t, refreshFrame.GetId().GetValue(), refreshFrame.GetRequestInventory().GetQueryId().GetValue())

	for _, procedure := range []string{
		cadestrov1connect.ControlServiceDispatchOSQueryProcedure,
		cadestrov1connect.ControlServiceQueryDeviceLogsProcedure,
		cadestrov1connect.ControlServiceRefreshDeviceInventoryProcedure,
	} {
		operation, err := latestOperationFor(t, f.store, f.raw, procedure)
		require.NoError(t, err, procedure)
		effects, err := f.store.ListAuditEffects(context.Background(), operation.OperationID)
		require.NoError(t, err, procedure)
		assert.NotEmpty(t, effects, procedure)
	}

	_, err = f.handlers.DispatchOSQuery(ctx, connect.NewRequest(&cadestrov1.DispatchOSQueryRequest{
		DeviceId: &cadestrov1.DeviceId{Value: f.directID}, Table: "packages", RawSql: "select 1",
	}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestDeviceHandlers_InstantQuerySendFailureIsTerminalAndGeneric(t *testing.T) {
	f := newDeviceHandlerFixture(t)
	f.sender.err = errors.New("write tcp 10.0.0.1: secret transport detail")
	ctx := f.actor("DispatchOSQuery", "QueryDeviceLogs", "RefreshDeviceInventory")

	_, err := f.handlers.DispatchOSQuery(ctx, connect.NewRequest(&cadestrov1.DispatchOSQueryRequest{
		DeviceId: &cadestrov1.DeviceId{Value: f.directID}, RawSql: "select version from os_version",
	}))
	assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
	require.Len(t, f.sender.messages, 1)
	osID := f.sender.messages[0].GetQuery().GetQueryId().GetValue()
	var completed, success bool
	var storedError, tableName string
	err = f.raw.QueryRow(context.Background(), `
		SELECT completed, success, error, table_name FROM osquery_results WHERE query_id = $1`, osID).
		Scan(&completed, &success, &storedError, &tableName)
	require.NoError(t, err)
	assert.True(t, completed)
	assert.False(t, success)
	assert.Equal(t, "device unavailable", storedError)
	assert.Equal(t, "raw_sql", tableName)

	_, err = f.handlers.QueryDeviceLogs(ctx, connect.NewRequest(&cadestrov1.QueryDeviceLogsRequest{
		DeviceId: &cadestrov1.DeviceId{Value: f.directID},
	}))
	assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
	require.Len(t, f.sender.messages, 2)
	logID := f.sender.messages[1].GetLogQuery().GetQueryId().GetValue()
	err = f.raw.QueryRow(context.Background(), `
		SELECT completed, success, error FROM log_query_results WHERE query_id = $1`, logID).
		Scan(&completed, &success, &storedError)
	require.NoError(t, err)
	assert.True(t, completed)
	assert.False(t, success)
	assert.Equal(t, "device unavailable", storedError)

	_, err = f.handlers.RefreshDeviceInventory(ctx,
		connect.NewRequest(&cadestrov1.RefreshDeviceInventoryRequest{DeviceId: &cadestrov1.DeviceId{Value: f.directID}}))
	assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
	refreshOperation, err := latestOperationFor(t, f.store, f.raw,
		cadestrov1connect.ControlServiceRefreshDeviceInventoryProcedure)
	require.NoError(t, err)
	refreshEffects, err := f.store.ListAuditEffects(context.Background(), refreshOperation.OperationID)
	require.NoError(t, err)
	require.Len(t, refreshEffects, 2)
	assert.Equal(t, string(store.EffectFailed), refreshEffects[1].Outcome)
}

func TestDeviceHandlers_AgentQueryResultsAndInventoryCommitDirectly(t *testing.T) {
	f := newDeviceHandlerFixture(t)
	ctx := f.actor("DispatchOSQuery", "QueryDeviceLogs")
	osquery, err := f.handlers.DispatchOSQuery(ctx, connect.NewRequest(&cadestrov1.DispatchOSQueryRequest{
		DeviceId: &cadestrov1.DeviceId{Value: f.directID}, Table: "packages",
	}))
	require.NoError(t, err)
	logs, err := f.handlers.QueryDeviceLogs(ctx, connect.NewRequest(&cadestrov1.QueryDeviceLogsRequest{
		DeviceId: &cadestrov1.DeviceId{Value: f.directID}, Lines: 10,
	}))
	require.NoError(t, err)

	// A result from another authenticated device cannot claim this query.
	require.NoError(t, f.handlers.CompleteOSQueryResult(context.Background(), f.outsideID,
		&cadestrov1.OSQueryResult{QueryId: osquery.Msg.GetQueryId(), Success: true}))
	var completed bool
	require.NoError(t, f.raw.QueryRow(context.Background(), `
		SELECT completed FROM osquery_results WHERE query_id = $1`, osquery.Msg.GetQueryId().GetValue()).Scan(&completed))
	assert.False(t, completed)

	require.NoError(t, f.handlers.CompleteOSQueryResult(context.Background(), f.directID,
		&cadestrov1.OSQueryResult{
			QueryId: osquery.Msg.GetQueryId(), Success: true,
			Rows: []*cadestrov1.OSQueryRow{{Data: map[string]string{"name": "bash"}}},
		}))
	var rowsJSON []byte
	var success bool
	require.NoError(t, f.raw.QueryRow(context.Background(), `
		SELECT completed, success, rows FROM osquery_results WHERE query_id = $1`, osquery.Msg.GetQueryId().GetValue()).
		Scan(&completed, &success, &rowsJSON))
	assert.True(t, completed)
	assert.True(t, success)
	assert.JSONEq(t, `[{"name":"bash"}]`, string(rowsJSON))

	require.NoError(t, f.handlers.CompleteLogQueryResult(context.Background(), f.directID,
		&cadestrov1.LogQueryResult{QueryId: logs.Msg.GetQueryId(), Success: true, Logs: "service started\n"}))
	var storedLogs string
	require.NoError(t, f.raw.QueryRow(context.Background(), `
		SELECT completed, success, logs FROM log_query_results WHERE query_id = $1`, logs.Msg.GetQueryId().GetValue()).
		Scan(&completed, &success, &storedLogs))
	assert.True(t, completed)
	assert.True(t, success)
	assert.Equal(t, "service started\n", storedLogs)

	require.NoError(t, f.handlers.StoreDeviceInventory(context.Background(), f.directID,
		&cadestrov1.DeviceInventory{Tables: []*cadestrov1.InventoryTable{
			{TableName: "os_version", Rows: []*cadestrov1.OSQueryRow{{Data: map[string]string{"name": "Debian"}}}},
			{TableName: "system_info", Rows: []*cadestrov1.OSQueryRow{{Data: map[string]string{"hostname": "direct"}}}},
		}}))
	var inventoryCount int
	require.NoError(t, f.raw.QueryRow(context.Background(), `
		SELECT count(*) FROM device_inventory WHERE device_id = $1 AND collected_at = $2`,
		f.directID, f.now).Scan(&inventoryCount))
	assert.Equal(t, 2, inventoryCount)

	err = f.handlers.StoreDeviceInventory(context.Background(), f.directID,
		&cadestrov1.DeviceInventory{Tables: []*cadestrov1.InventoryTable{
			{TableName: "os_version"}, {TableName: "os_version"},
		}})
	assert.Error(t, err, "duplicate table names must not create order-dependent state")
}

func TestDeviceHandlers_OSQueryAuditFailurePreventsSendAndPendingRow(t *testing.T) {
	f := newDeviceHandlerFixture(t)
	rejectAuditOperation(t, f.raw, "/cadestro.v1.ControlService/DispatchOSQuery")
	_, err := f.handlers.DispatchOSQuery(f.actor("DispatchOSQuery"),
		connect.NewRequest(&cadestrov1.DispatchOSQueryRequest{DeviceId: &cadestrov1.DeviceId{Value: f.directID}, Table: "packages"}))
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	assert.Empty(t, f.sender.messages)
	var count int
	require.NoError(t, f.raw.QueryRow(context.Background(), `SELECT count(*) FROM osquery_results`).Scan(&count))
	assert.Zero(t, count)
}

func TestDeviceHandlers_TerminalLifecycleUsesInProcessSessionTruth(t *testing.T) {
	f := newDeviceHandlerFixture(t)
	startCtx := f.actor("StartTerminal")

	_, err := f.handlers.StartTerminal(f.actor(), connect.NewRequest(&cadestrov1.StartTerminalRequest{
		DeviceId: &cadestrov1.DeviceId{Value: f.directID},
	}))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	f.connected[f.directID] = false
	_, err = f.handlers.StartTerminal(startCtx, connect.NewRequest(&cadestrov1.StartTerminalRequest{
		DeviceId: &cadestrov1.DeviceId{Value: f.directID},
	}))
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	f.connected[f.directID] = true

	started, err := f.handlers.StartTerminal(startCtx, connect.NewRequest(&cadestrov1.StartTerminalRequest{
		DeviceId: &cadestrov1.DeviceId{Value: f.directID},
	}))
	require.NoError(t, err)
	assert.Equal(t, "wss://control.example.test/terminal", started.Msg.TerminalUrl)
	assert.Equal(t, "cadestro-tty-test", started.Msg.TtyUser)
	assert.Empty(t, f.sender.messages, "the PTY starts only after the browser redeems its token")
	stored, err := f.store.GetOpenTerminalSession(context.Background(), started.Msg.GetSessionId().GetValue())
	require.NoError(t, err)
	assert.Equal(t, int32(80), stored.Cols)
	assert.Equal(t, int32(24), stored.Rows)

	validated, err := f.tokens.Validate(context.Background(), started.Msg.GetSessionId().GetValue(), started.Msg.SessionToken)
	require.NoError(t, err)
	assert.Equal(t, f.directID, validated.DeviceID)
	_, err = f.tokens.Validate(context.Background(), started.Msg.GetSessionId().GetValue(), started.Msg.SessionToken)
	assert.ErrorIs(t, err, terminal.ErrTokenNotFound, "the browser bearer must be single-use")
	f.sessions.Register(connection.NewTerminalSession(
		started.Msg.GetSessionId().GetValue(), validated.DeviceID, validated.UserID, validated.TtyUser,
		validated.Cols, validated.Rows,
	))

	listed, err := f.handlers.ListActiveTerminalSessions(f.actor("ListActiveTerminalSessions"),
		connect.NewRequest(&cadestrov1.ListActiveTerminalSessionsRequest{}))
	require.NoError(t, err)
	require.Len(t, listed.Msg.Sessions, 1)
	assert.Equal(t, started.Msg.GetSessionId().GetValue(), listed.Msg.Sessions[0].GetSessionId().GetValue())
	assert.Equal(t, "actor@example.test", listed.Msg.Sessions[0].UserEmail)
	assert.Equal(t, "direct", listed.Msg.Sessions[0].DeviceHostname)
	assert.Equal(t, int32(1), listed.Msg.TotalCount)
	operation, err := latestOperationFor(t, f.store, f.raw,
		cadestrov1connect.ControlServiceListActiveTerminalSessionsProcedure)
	require.NoError(t, err)
	assert.Equal(t, string(store.ClassSensitiveRead), operation.OperationClass)

	nonOwner := auth.WithUser(context.Background(), &auth.UserContext{
		ID: f.userID, Kind: auth.PrincipalUser, Permissions: []string{"StopTerminal"},
	})
	_, err = f.handlers.StopTerminal(nonOwner,
		connect.NewRequest(&cadestrov1.StopTerminalRequest{SessionId: &cadestrov1.SessionId{Value: started.Msg.GetSessionId().GetValue()}}))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))

	_, err = f.handlers.StopTerminal(f.actor("StopTerminal"),
		connect.NewRequest(&cadestrov1.StopTerminalRequest{SessionId: &cadestrov1.SessionId{Value: started.Msg.GetSessionId().GetValue()}}))
	require.NoError(t, err)
	require.Len(t, f.sender.messages, 1)
	require.NotNil(t, f.sender.messages[0].GetTerminalStop())
	assert.Equal(t, started.Msg.GetSessionId().GetValue(), f.sender.messages[0].GetTerminalStop().GetSessionId().GetValue())
	assert.Zero(t, f.sessions.Count())
	_, err = f.store.GetOpenTerminalSession(context.Background(), started.Msg.GetSessionId().GetValue())
	assert.True(t, store.IsNotFound(err))

	_, err = f.handlers.StopTerminal(f.actor("StopTerminal"),
		connect.NewRequest(&cadestrov1.StopTerminalRequest{SessionId: &cadestrov1.SessionId{Value: started.Msg.GetSessionId().GetValue()}}))
	require.NoError(t, err)
	assert.Len(t, f.sender.messages, 1, "the idempotent replay must not send another frame")
}

func TestDeviceHandlers_TerminalListFiltersScopesAndPagesLiveRegistry(t *testing.T) {
	f := newDeviceHandlerFixture(t)
	startCtx := f.actor("StartTerminal")
	ids := make([]string, 0, 3)
	for _, deviceID := range []string{f.directID, f.groupID, f.outsideID} {
		started, err := f.handlers.StartTerminal(startCtx,
			connect.NewRequest(&cadestrov1.StartTerminalRequest{DeviceId: &cadestrov1.DeviceId{Value: deviceID}, Cols: 120, Rows: 40}))
		require.NoError(t, err)
		ids = append(ids, started.Msg.GetSessionId().GetValue())
		f.sessions.Register(connection.NewTerminalSession(
			started.Msg.GetSessionId().GetValue(), deviceID, f.actorID, started.Msg.TtyUser, 120, 40,
		))
	}

	ctx := f.actor("ListActiveTerminalSessions")
	page1, err := f.handlers.ListActiveTerminalSessions(ctx,
		connect.NewRequest(&cadestrov1.ListActiveTerminalSessionsRequest{PageSize: 1}))
	require.NoError(t, err)
	require.Len(t, page1.Msg.Sessions, 1)
	assert.Equal(t, int32(3), page1.Msg.TotalCount)
	assert.NotEmpty(t, page1.Msg.NextPageToken)
	page2, err := f.handlers.ListActiveTerminalSessions(ctx,
		connect.NewRequest(&cadestrov1.ListActiveTerminalSessionsRequest{
			PageSize: 1, PageToken: page1.Msg.NextPageToken,
		}))
	require.NoError(t, err)
	require.Len(t, page2.Msg.Sessions, 1)
	assert.NotEqual(t, page1.Msg.Sessions[0].GetSessionId().GetValue(), page2.Msg.Sessions[0].GetSessionId().GetValue())

	filtered, err := f.handlers.ListActiveTerminalSessions(ctx,
		connect.NewRequest(&cadestrov1.ListActiveTerminalSessionsRequest{DeviceId: &cadestrov1.DeviceId{Value: f.directID}}))
	require.NoError(t, err)
	require.Len(t, filtered.Msg.Sessions, 1)
	assert.Equal(t, f.directID, filtered.Msg.Sessions[0].GetDeviceId().GetValue())

	scoped := auth.WithUser(context.Background(), &auth.UserContext{
		ID: f.actorID, Kind: auth.PrincipalUser,
		Permissions: []string{"ListActiveTerminalSessions"},
		ScopedGrants: []auth.ScopedGrant{{
			Permission: "ListActiveTerminalSessions", ScopeKind: auth.ScopeKindDeviceGroup, ScopeID: f.scopeGroup,
		}},
	})
	scopedList, err := f.handlers.ListActiveTerminalSessions(scoped,
		connect.NewRequest(&cadestrov1.ListActiveTerminalSessionsRequest{}))
	require.NoError(t, err)
	require.Len(t, scopedList.Msg.Sessions, 1)
	assert.Equal(t, f.groupID, scopedList.Msg.Sessions[0].GetDeviceId().GetValue())
	assert.Equal(t, int32(1), scopedList.Msg.TotalCount)
	assert.Len(t, ids, 3)
}

func TestDeviceHandlers_StartTerminalAppliesScopeBeforeExistence(t *testing.T) {
	f := newDeviceHandlerFixture(t)
	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		ID: f.actorID, Kind: auth.PrincipalUser,
		Permissions: []string{"StartTerminal"},
		ScopedGrants: []auth.ScopedGrant{{
			Permission: "StartTerminal", ScopeKind: auth.ScopeKindDeviceGroup, ScopeID: f.scopeGroup,
		}},
	})
	_, err := f.handlers.StartTerminal(ctx,
		connect.NewRequest(&cadestrov1.StartTerminalRequest{DeviceId: &cadestrov1.DeviceId{Value: f.groupID}}))
	require.NoError(t, err)
	_, err = f.handlers.StartTerminal(ctx,
		connect.NewRequest(&cadestrov1.StartTerminalRequest{DeviceId: &cadestrov1.DeviceId{Value: f.outsideID}}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "scope misses must not disclose device existence")
	_, err = f.handlers.StartTerminal(ctx,
		connect.NewRequest(&cadestrov1.StartTerminalRequest{DeviceId: &cadestrov1.DeviceId{Value: newID()}}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err), "unknown and out-of-scope devices must look alike")
}

func TestDeviceHandlers_TerminateTerminalSurfacesSendFailureThenCommitsRetry(t *testing.T) {
	f := newDeviceHandlerFixture(t)
	started, err := f.handlers.StartTerminal(f.actor("StartTerminal"),
		connect.NewRequest(&cadestrov1.StartTerminalRequest{DeviceId: &cadestrov1.DeviceId{Value: f.directID}}))
	require.NoError(t, err)
	_, err = f.tokens.Validate(context.Background(), started.Msg.GetSessionId().GetValue(), started.Msg.SessionToken)
	require.NoError(t, err)
	f.sessions.Register(connection.NewTerminalSession(
		started.Msg.GetSessionId().GetValue(), f.directID, f.actorID, started.Msg.TtyUser, 80, 24,
	))
	f.sender.err = errors.New("agent disconnected")
	_, err = f.handlers.TerminateTerminalSession(f.actor("TerminateTerminalSession"),
		connect.NewRequest(&cadestrov1.TerminateTerminalSessionRequest{
			SessionId: &cadestrov1.SessionId{Value: started.Msg.GetSessionId().GetValue()}, Reason: "incident response",
		}))
	assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
	_, err = f.store.GetOpenTerminalSession(context.Background(), started.Msg.GetSessionId().GetValue())
	require.NoError(t, err, "failed delivery must not claim the privileged shell is closed")
	assert.Equal(t, 1, f.sessions.Count())

	f.sender.err = nil
	_, err = f.handlers.TerminateTerminalSession(f.actor("TerminateTerminalSession"),
		connect.NewRequest(&cadestrov1.TerminateTerminalSessionRequest{
			SessionId: &cadestrov1.SessionId{Value: started.Msg.GetSessionId().GetValue()}, Reason: "incident response",
		}))
	require.NoError(t, err)
	assert.Zero(t, f.sessions.Count())
	var reason string
	var terminatedBy *string
	require.NoError(t, f.raw.QueryRow(context.Background(), `
		SELECT exit_reason, terminated_by FROM terminal_sessions WHERE session_id = $1`,
		started.Msg.GetSessionId().GetValue()).Scan(&reason, &terminatedBy))
	assert.Equal(t, "incident response", reason)
	require.NotNil(t, terminatedBy)
	assert.Equal(t, f.actorID, *terminatedBy)

	_, err = f.handlers.TerminateTerminalSession(f.actor("TerminateTerminalSession"),
		connect.NewRequest(&cadestrov1.TerminateTerminalSessionRequest{SessionId: &cadestrov1.SessionId{Value: newID()}}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestDeviceHandlers_SensitiveReadFailsClosedWhenEvidenceFails(t *testing.T) {
	f := newDeviceHandlerFixture(t)
	rejectAuditOperation(t, f.raw, "/cadestro.v1.ControlService/GetDeviceInventory")

	_, err := f.handlers.GetDeviceInventory(f.actor("GetDeviceInventory"),
		connect.NewRequest(&cadestrov1.GetDeviceInventoryRequest{DeviceId: &cadestrov1.DeviceId{Value: f.groupID}}))
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
}

func TestDeviceHandlers_MutationsAreAuditedCRUD(t *testing.T) {
	f := newDeviceHandlerFixture(t)
	ctx := f.actor(
		"SetDeviceLabel", "RemoveDeviceLabel", "AssignDevice", "UnassignDevice",
		"ListDeviceAssignees", "SetDeviceSyncInterval", "SetDeviceInventoryInterval", "DeleteDevice",
		"CreateLuksToken", "RevokeLuksDeviceKey", "DispatchOSQuery",
		"QueryDeviceLogs", "RefreshDeviceInventory", "StartTerminal", "StopTerminal",
		"TerminateTerminalSession",
	)

	setLabel, err := f.handlers.SetDeviceLabel(ctx, connect.NewRequest(&cadestrov1.SetDeviceLabelRequest{
		Id: &cadestrov1.DeviceId{Value: f.directID}, Key: "env", Value: "prod",
	}))
	require.NoError(t, err)
	assert.Equal(t, "prod", setLabel.Msg.Device.Labels["env"])
	removedLabel, err := f.handlers.RemoveDeviceLabel(ctx, connect.NewRequest(&cadestrov1.RemoveDeviceLabelRequest{
		Id: &cadestrov1.DeviceId{Value: f.directID}, Key: "env",
	}))
	require.NoError(t, err)
	assert.NotContains(t, removedLabel.Msg.Device.Labels, "env")

	assigned, err := f.handlers.AssignDevice(ctx, connect.NewRequest(&cadestrov1.AssignDeviceRequest{
		DeviceId: &cadestrov1.DeviceId{Value: f.directID},
		UserIds:  []*cadestrov1.UserId{&cadestrov1.UserId{Value: f.userID}, &cadestrov1.UserId{Value: f.userID}},
		GroupIds: []*cadestrov1.UserGroupId{&cadestrov1.UserGroupId{Value: f.userGroup}, &cadestrov1.UserGroupId{Value: f.userGroup}},
	}))
	require.NoError(t, err)
	assignedUserIDs := make([]string, len(assigned.Msg.Device.AssignedUserIds))
	for i, id := range assigned.Msg.Device.AssignedUserIds {
		assignedUserIDs[i] = id.GetValue()
	}
	assignedGroupIDs := make([]string, len(assigned.Msg.Device.AssignedGroupIds))
	for i, id := range assigned.Msg.Device.AssignedGroupIds {
		assignedGroupIDs[i] = id.GetValue()
	}
	assert.ElementsMatch(t, []string{f.actorID, f.userID}, assignedUserIDs)
	assert.Equal(t, []string{f.userGroup}, assignedGroupIDs)

	assignees, err := f.handlers.ListDeviceAssignees(ctx, connect.NewRequest(&cadestrov1.ListDeviceAssigneesRequest{DeviceId: &cadestrov1.DeviceId{Value: f.directID}}))
	require.NoError(t, err)
	require.Len(t, assignees.Msg.Assignees, 3)

	_, err = f.handlers.UnassignDevice(ctx, connect.NewRequest(&cadestrov1.UnassignDeviceRequest{
		DeviceId: &cadestrov1.DeviceId{Value: f.directID}, UserId: &cadestrov1.UserId{Value: f.userID}, GroupId: &cadestrov1.UserGroupId{Value: f.userGroup},
	}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, err = f.handlers.UnassignDevice(ctx, connect.NewRequest(&cadestrov1.UnassignDeviceRequest{
		DeviceId: &cadestrov1.DeviceId{Value: f.directID}, UserId: &cadestrov1.UserId{Value: f.userID},
	}))
	require.NoError(t, err)

	updated, err := f.handlers.SetDeviceSyncInterval(ctx, connect.NewRequest(&cadestrov1.SetDeviceSyncIntervalRequest{
		Id: &cadestrov1.DeviceId{Value: f.directID}, SyncIntervalMinutes: 60,
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(60), updated.Msg.Device.SyncIntervalMinutes)
	updated, err = f.handlers.SetDeviceInventoryInterval(ctx, connect.NewRequest(&cadestrov1.SetDeviceInventoryIntervalRequest{
		Id: &cadestrov1.DeviceId{Value: f.directID}, InventoryIntervalMinutes: 1440,
	}))
	require.NoError(t, err)
	assert.Equal(t, int32(1440), updated.Msg.Device.InventoryIntervalMinutes)
	encryptionActionID := newID()
	_, err = f.raw.Exec(context.Background(), `
		INSERT INTO actions (id, name, action_type, params, created_by)
		VALUES ($1, 'Encryption', $2, '{}', $3)`,
		encryptionActionID, int32(cadestrov1.ActionType_ACTION_TYPE_ENCRYPTION), f.actorID)
	require.NoError(t, err)
	_, err = f.handlers.CreateLuksToken(ctx,
		connect.NewRequest(&cadestrov1.CreateLuksTokenRequest{
			DeviceId: &cadestrov1.DeviceId{Value: f.directID}, ActionId: &cadestrov1.ActionId{Value: encryptionActionID},
		}))
	require.NoError(t, err)
	seedCurrentLuksKeys(t, f, encryptionActionID, 1)
	_, err = f.handlers.RevokeLuksDeviceKey(ctx,
		connect.NewRequest(&cadestrov1.RevokeLuksDeviceKeyRequest{
			DeviceId: &cadestrov1.DeviceId{Value: f.directID}, ActionId: &cadestrov1.ActionId{Value: encryptionActionID},
		}))
	require.NoError(t, err)
	_, err = f.handlers.DispatchOSQuery(ctx,
		connect.NewRequest(&cadestrov1.DispatchOSQueryRequest{DeviceId: &cadestrov1.DeviceId{Value: f.directID}, Table: "packages"}))
	require.NoError(t, err)
	_, err = f.handlers.QueryDeviceLogs(ctx,
		connect.NewRequest(&cadestrov1.QueryDeviceLogsRequest{DeviceId: &cadestrov1.DeviceId{Value: f.directID}, Lines: 10}))
	require.NoError(t, err)
	_, err = f.handlers.RefreshDeviceInventory(ctx,
		connect.NewRequest(&cadestrov1.RefreshDeviceInventoryRequest{DeviceId: &cadestrov1.DeviceId{Value: f.directID}}))
	require.NoError(t, err)
	graceful, err := f.handlers.StartTerminal(ctx,
		connect.NewRequest(&cadestrov1.StartTerminalRequest{DeviceId: &cadestrov1.DeviceId{Value: f.directID}}))
	require.NoError(t, err)
	_, err = f.handlers.StopTerminal(ctx,
		connect.NewRequest(&cadestrov1.StopTerminalRequest{SessionId: &cadestrov1.SessionId{Value: graceful.Msg.GetSessionId().GetValue()}}))
	require.NoError(t, err)
	forced, err := f.handlers.StartTerminal(ctx,
		connect.NewRequest(&cadestrov1.StartTerminalRequest{DeviceId: &cadestrov1.DeviceId{Value: f.directID}}))
	require.NoError(t, err)
	_, err = f.handlers.TerminateTerminalSession(ctx,
		connect.NewRequest(&cadestrov1.TerminateTerminalSessionRequest{SessionId: &cadestrov1.SessionId{Value: forced.Msg.GetSessionId().GetValue()}}))
	require.NoError(t, err)

	_, err = f.handlers.DeleteDevice(ctx, connect.NewRequest(&cadestrov1.DeleteDeviceRequest{Id: &cadestrov1.DeviceId{Value: f.directID}}))
	require.NoError(t, err)
	assert.Equal(t, []string{f.directID}, f.closed)
	_, err = f.store.GetDevice(context.Background(), f.directID)
	assert.True(t, store.IsNotFound(err))

	for _, procedure := range device.MutationProcedures() {
		operation, err := latestOperationFor(t, f.store, f.raw, procedure)
		require.NoError(t, err, procedure)
		effects, err := f.store.ListAuditEffects(context.Background(), operation.OperationID)
		require.NoError(t, err, procedure)
		assert.NotEmpty(t, effects, procedure)
	}

	operation, err := latestOperationFor(t, f.store, f.raw, cadestrov1connect.ControlServiceDeleteDeviceProcedure)
	require.NoError(t, err)
	assert.Equal(t, "DeleteDevice", operation.AuthorizationDetail)
	effects, err := f.store.ListAuditEffects(context.Background(), operation.OperationID)
	require.NoError(t, err)
	require.Len(t, effects, 1)
	assert.Equal(t, f.directID, effects[0].ResourceID)
	assert.Equal(t, "DELETE", effects[0].Action)
}

func TestDeviceHandlers_MountsExactSurface(t *testing.T) {
	f := newDeviceHandlerFixture(t)
	mounted := f.handlers.Mount(http.NewServeMux())
	want := []string{
		cadestrov1connect.ControlServiceListDevicesProcedure,
		cadestrov1connect.ControlServiceGetDeviceProcedure,
		cadestrov1connect.ControlServiceGetDeviceInventoryProcedure,
		cadestrov1connect.ControlServiceGetOSQueryResultProcedure,
		cadestrov1connect.ControlServiceGetDeviceLogResultProcedure,
		cadestrov1connect.ControlServiceGetDeviceComplianceProcedure,
		cadestrov1connect.ControlServiceGetDeviceCompliancePolicyStatusProcedure,
		cadestrov1connect.ControlServiceListLpsPasswordsProcedure,
		cadestrov1connect.ControlServiceRevealLpsPasswordProcedure,
		cadestrov1connect.ControlServiceListLuksKeysProcedure,
		cadestrov1connect.ControlServiceRevealLuksKeyProcedure,
		cadestrov1connect.ControlServiceCreateLuksTokenProcedure,
		cadestrov1connect.ControlServiceRevokeLuksDeviceKeyProcedure,
		cadestrov1connect.ControlServiceDispatchOSQueryProcedure,
		cadestrov1connect.ControlServiceRefreshDeviceInventoryProcedure,
		cadestrov1connect.ControlServiceQueryDeviceLogsProcedure,
		cadestrov1connect.ControlServiceStartTerminalProcedure,
		cadestrov1connect.ControlServiceStopTerminalProcedure,
		cadestrov1connect.ControlServiceListActiveTerminalSessionsProcedure,
		cadestrov1connect.ControlServiceTerminateTerminalSessionProcedure,
		cadestrov1connect.ControlServiceSetDeviceLabelProcedure,
		cadestrov1connect.ControlServiceRemoveDeviceLabelProcedure,
		cadestrov1connect.ControlServiceAssignDeviceProcedure,
		cadestrov1connect.ControlServiceUnassignDeviceProcedure,
		cadestrov1connect.ControlServiceListDeviceAssigneesProcedure,
		cadestrov1connect.ControlServiceSetDeviceSyncIntervalProcedure,
		cadestrov1connect.ControlServiceSetDeviceInventoryIntervalProcedure,
		cadestrov1connect.ControlServiceDeleteDeviceProcedure,
	}
	assert.Equal(t, want, mounted)
	assert.Equal(t, []string{
		cadestrov1connect.ControlServiceGetDeviceInventoryProcedure,
		cadestrov1connect.ControlServiceGetOSQueryResultProcedure,
		cadestrov1connect.ControlServiceGetDeviceLogResultProcedure,
		cadestrov1connect.ControlServiceGetDeviceComplianceProcedure,
		cadestrov1connect.ControlServiceGetDeviceCompliancePolicyStatusProcedure,
		cadestrov1connect.ControlServiceListLpsPasswordsProcedure,
		cadestrov1connect.ControlServiceRevealLpsPasswordProcedure,
		cadestrov1connect.ControlServiceListLuksKeysProcedure,
		cadestrov1connect.ControlServiceRevealLuksKeyProcedure,
		cadestrov1connect.ControlServiceListActiveTerminalSessionsProcedure,
	}, device.SensitiveReadProcedures())
	classified := append(device.MutationProcedures(), device.ReadProcedures()...)
	classified = append(classified, device.SensitiveReadProcedures()...)
	assert.ElementsMatch(t, want, classified, "every mounted procedure must have exactly one audit class")
}

func assertSensitiveDeviceRead(
	t *testing.T,
	f *deviceHandlerFixture,
	procedure, resourceType, resourceID string,
) {
	t.Helper()
	operation, err := latestOperationFor(t, f.store, f.raw, procedure)
	require.NoError(t, err)
	assert.Equal(t, string(store.ClassSensitiveRead), operation.OperationClass)
	effects, err := f.store.ListAuditEffects(context.Background(), operation.OperationID)
	require.NoError(t, err)
	require.Len(t, effects, 1)
	assert.Equal(t, resourceType, effects[0].ResourceType)
	assert.Equal(t, resourceID, effects[0].ResourceID)
	assert.Equal(t, "READ", effects[0].Action)
}

func assertSecretReveal(
	t *testing.T,
	f *deviceHandlerFixture,
	procedure, secretType, secretID, deviceID, actionID string,
) {
	t.Helper()
	operation, err := latestOperationFor(t, f.store, f.raw, procedure)
	require.NoError(t, err)
	assert.Equal(t, string(store.ClassSensitiveRead), operation.OperationClass)
	effects, err := f.store.ListAuditEffects(context.Background(), operation.OperationID)
	require.NoError(t, err)
	require.Len(t, effects, 3)
	want := map[string]string{secretType: secretID, "device": deviceID, "action": actionID}
	for _, effect := range effects {
		assert.Equal(t, want[effect.ResourceType], effect.ResourceID)
		assert.Equal(t, "REVEAL", effect.Action)
		delete(want, effect.ResourceType)
	}
	assert.Empty(t, want)
}

func latestOperationFor(t *testing.T, st *store.Store, raw *testdb.DB, procedure string) (store.AuditOperationRow, error) {
	t.Helper()
	var operationID string
	if err := raw.QueryRow(context.Background(), `
		SELECT operation_id
		FROM audit_operations
		WHERE request_descriptor = $1
		ORDER BY chain_seq DESC
		LIMIT 1`, procedure).Scan(&operationID); err != nil {
		return store.AuditOperationRow{}, err
	}
	return st.GetAuditOperation(context.Background(), operationID)
}

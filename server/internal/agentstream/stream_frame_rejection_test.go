package agentstream

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
	"google.golang.org/protobuf/encoding/protojson"

	pmv1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/manchtools/cadestro/server/internal/connection"
	"github.com/manchtools/cadestro/server/internal/delivery"
	"github.com/manchtools/cadestro/server/internal/execution"
	"github.com/manchtools/cadestro/server/internal/mtls"
	"github.com/manchtools/cadestro/server/internal/store"
	"github.com/manchtools/cadestro/server/internal/testdb"
)

// CHARTER — one rejected frame must not cost the connection.
//
// The agent's outbox is durable: a frame control refuses is re-sent on every
// reconnect. Ending the stream on an application-level rejection therefore
// turns one bad frame into a permanent reconnect loop, and every other frame
// the agent was about to report is lost with it. Only a claim the device is
// not entitled to make ends the connection.

type fakeSecrets struct{}

func (fakeSecrets) ValidateLuksToken(context.Context, string, *pmv1.ValidateLuksTokenRequest) (*pmv1.ValidateLuksTokenResponse, error) {
	return &pmv1.ValidateLuksTokenResponse{}, nil
}

func (fakeSecrets) GetLuksKey(context.Context, string, *pmv1.GetLuksKeyRequest) (*pmv1.GetLuksKeyResponse, error) {
	return &pmv1.GetLuksKeyResponse{}, nil
}

func (fakeSecrets) StoreLuksKey(context.Context, string, *pmv1.StoreLuksKeyRequest) (*pmv1.StoreLuksKeyResponse, error) {
	return &pmv1.StoreLuksKeyResponse{}, nil
}

func (fakeSecrets) StoreLpsPasswords(context.Context, string, *pmv1.StoreLpsPasswordsRequest) (*pmv1.StoreLpsPasswordsResponse, error) {
	return &pmv1.StoreLpsPasswordsResponse{}, nil
}

type fakeSync struct{}

func (fakeSync) Sync(context.Context, string) (*pmv1.SyncState, error) {
	return &pmv1.SyncState{SyncIntervalMinutes: 30}, nil
}

type fakeWaker struct{}

func (fakeWaker) WakeDevice(context.Context, string) error { return nil }

// seededExecution is one device with one acknowledged delivery holding one
// pending occurrence — the state an agent is in when it reports a result.
type seededExecution struct {
	deviceID   string
	deliveryID string
	occurrence string
	actionID   string
}

func seedExecution(t *testing.T, raw *testdb.DB, at time.Time) seededExecution {
	t.Helper()
	ctx := context.Background()
	seeded := seededExecution{
		deviceID: ulid.Make().String(), deliveryID: ulid.Make().String(),
		occurrence: ulid.Make().String(), actionID: ulid.Make().String(),
	}
	_, err := raw.Exec(ctx, `
		INSERT INTO devices (id, hostname, agent_version, certificate_pem, active_cert_serial, registered_at)
		VALUES ($1, $2, 'v1', X'01', '1', $3)`, seeded.deviceID, "host-"+seeded.deviceID, at)
	require.NoError(t, err)
	manifest, err := protojson.Marshal(&pmv1.Manifest{
		ManifestId: ulid.Make().String(),
		Occurrences: []*pmv1.ManifestOccurrence{{
			OccurrenceId: seeded.occurrence,
			Action: &pmv1.Action{
				Id: &pmv1.ActionId{Value: seeded.actionID}, Type: pmv1.ActionType_ACTION_TYPE_ENCRYPTION,
			},
		}},
	})
	require.NoError(t, err)
	_, err = raw.Exec(ctx, `
		INSERT INTO deliveries (
			delivery_id, device_id, manifest_id, manifest, state, pushed_at, acked_receipt_at
		) VALUES ($1, $2, $3, $4, $5, $6, $6)`,
		seeded.deliveryID, seeded.deviceID, ulid.Make().String(), manifest, delivery.StateAckedReceipt, at)
	require.NoError(t, err)
	_, err = raw.Exec(ctx, `
		INSERT INTO executions (
			id, delivery_id, device_id, action_type, desired_state, params,
			timeout_seconds, status, created_at, created_by_type, created_by_id
		) VALUES ($1, $2, $3, 1, 0, '{}', 300, 'pending', $4, 'user', $5)`,
		seeded.occurrence, seeded.deliveryID, seeded.deviceID, at, ulid.Make().String())
	require.NoError(t, err)
	return seeded
}

// streamFixture is one live AgentService stream over h2c, terminated by the
// real handler with the real execution sink behind it. The fake sink used by
// the routing tests never errors, so it cannot show what an error costs.
type streamFixture struct {
	store      *store.Store
	raw        *testdb.DB
	client     cadestrov1connect.AgentServiceClient
	own        seededExecution
	foreign    seededExecution
	peerSerial *big.Int
}

func newStreamFixture(t *testing.T) *streamFixture {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "cadestro.db")
	st, err := store.New(ctx, path)
	require.NoError(t, err)
	t.Cleanup(st.Close)
	raw, err := testdb.Open(ctx, path)
	require.NoError(t, err)
	t.Cleanup(raw.Close)

	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	f := &streamFixture{store: st, raw: raw, own: seedExecution(t, raw, now), foreign: seedExecution(t, raw, now), peerSerial: big.NewInt(1)}

	handler := New(Config{
		Store: st, Manager: connection.NewManager(),
		Deliveries:    &fakeDeliveryState{},
		Executions:    execution.New(execution.Config{Store: st, Now: func() time.Time { return now }}),
		DeviceResults: &fakeDeviceResults{},
		Secrets:       fakeSecrets{}, Sync: fakeSync{}, Waker: fakeWaker{},
		TerminalSessions: connection.NewTerminalSessionRegistry(),
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:              func() time.Time { return now },
	})
	t.Cleanup(handler.Close)

	procedure, connectHandler := cadestrov1connect.NewAgentServiceHandler(handler)
	mux := http.NewServeMux()
	// Stands in for MTLSMiddleware: the transport is already authenticated by
	// the time a frame reaches Stream, so the identity is bound here directly.
	mux.Handle(procedure, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := WithDeviceID(r.Context(), f.own.deviceID)
		ctx = mtls.WithPeerCertificate(ctx, &x509.Certificate{SerialNumber: new(big.Int).Set(f.peerSerial)})
		connectHandler.ServeHTTP(w, r.WithContext(ctx))
	}))
	server := httptest.NewUnstartedServer(mux)
	server.Config.Protocols = new(http.Protocols)
	server.Config.Protocols.SetUnencryptedHTTP2(true)
	server.Start()
	t.Cleanup(server.Close)

	httpClient := &http.Client{Transport: &http2.Transport{
		AllowHTTP: true,
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
	}}
	f.client = cadestrov1connect.NewAgentServiceClient(httpClient, server.URL, connect.WithGRPC())
	return f
}

func TestPendingHelloPromotesAndOldActiveIsRejected(t *testing.T) {
	f := newStreamFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := f.raw.Exec(ctx, `UPDATE devices SET pending_certificate_pem = X'02', pending_cert_serial = '2' WHERE id = ?`, f.own.deviceID)
	require.NoError(t, err)
	f.peerSerial = big.NewInt(2)
	f.open(t, ctx)
	var active, pending any
	require.NoError(t, f.raw.QueryRow(ctx, `SELECT active_cert_serial, pending_cert_serial FROM devices WHERE id = ?`, f.own.deviceID).Scan(&active, &pending))
	require.Equal(t, "2", active)
	require.Nil(t, pending)

	f.peerSerial = big.NewInt(1)
	stream := f.client.Stream(ctx)
	require.NoError(t, stream.Send(&pmv1.AgentMessage{Id: ulid.Make().String(), Payload: &pmv1.AgentMessage_Hello{Hello: &pmv1.Hello{
		DeviceId: &pmv1.DeviceId{Value: f.own.deviceID}, AgentVersion: "v1", Hostname: "device",
	}}}))
	_, err = stream.Receive()
	require.Error(t, err)
	var connectErr *connect.Error
	if assert.ErrorAs(t, err, &connectErr) {
		assert.Equal(t, connect.CodePermissionDenied, connectErr.Code())
	}
}

func TestLegacyFingerprintBridgeRecordsAuthenticatedSerial(t *testing.T) {
	f := newStreamFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	digest := sha256.Sum256(nil)
	_, err := f.raw.Exec(ctx, `UPDATE devices SET active_cert_serial = NULL, certificate_pem = X'01', cert_fingerprint = ?, cert_not_after = CURRENT_TIMESTAMP WHERE id = ?`, hex.EncodeToString(digest[:]), f.own.deviceID)
	require.NoError(t, err)
	f.open(t, ctx)
	var serial, fingerprint any
	require.NoError(t, f.raw.QueryRow(ctx, `SELECT active_cert_serial, cert_fingerprint FROM devices WHERE id = ?`, f.own.deviceID).Scan(&serial, &fingerprint))
	require.Equal(t, "1", serial)
	require.Nil(t, fingerprint)
}

func TestStreamRejectsAlreadyOpenPeerAfterSerialPromotion(t *testing.T) {
	f := newStreamFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stream := f.open(t, ctx)
	_, err := f.raw.Exec(ctx, `UPDATE devices SET certificate_pem = X'02', active_cert_serial = '2' WHERE id = ?`, f.own.deviceID)
	require.NoError(t, err)
	require.NoError(t, stream.Send(syncRequest()))
	_, err = stream.Receive()
	require.Error(t, err)
	var connectErr *connect.Error
	if assert.ErrorAs(t, err, &connectErr) {
		assert.Equal(t, connect.CodePermissionDenied, connectErr.Code())
	}
}

// open completes the handshake and returns a stream ready for result frames.
func (f *streamFixture) open(t *testing.T, ctx context.Context) *connect.BidiStreamForClient[pmv1.AgentMessage, pmv1.ServerMessage] {
	t.Helper()
	stream := f.client.Stream(ctx)
	t.Cleanup(func() { _ = stream.CloseRequest() })
	require.NoError(t, stream.Send(&pmv1.AgentMessage{
		Id: ulid.Make().String(),
		Payload: &pmv1.AgentMessage_Hello{Hello: &pmv1.Hello{
			DeviceId: &pmv1.DeviceId{Value: f.own.deviceID}, AgentVersion: "v1", Hostname: "device",
		}},
	}))
	welcome, err := stream.Receive()
	require.NoError(t, err)
	require.NotNil(t, welcome.GetWelcome())
	return stream
}

// luksResult is the frame the agent's LUKS success path actually produces.
func luksResult(seeded seededExecution, metadata map[string]string) *pmv1.AgentMessage {
	return &pmv1.AgentMessage{
		Id: ulid.Make().String(),
		Payload: &pmv1.AgentMessage_ActionResult{ActionResult: &pmv1.ActionResult{
			ActionId: &pmv1.ActionId{Value: seeded.actionID}, Status: pmv1.ExecutionStatus_EXECUTION_STATUS_SUCCESS,
			DeliveryId: seeded.deliveryID, OccurrenceId: seeded.occurrence, Changed: true,
			Output:   &pmv1.CommandOutput{Stdout: "LUKS: ownership taken, managed passphrase set\n"},
			Metadata: metadata,
		}},
	}
}

func syncRequest() *pmv1.AgentMessage {
	return &pmv1.AgentMessage{
		Id: ulid.Make().String(), Payload: &pmv1.AgentMessage_SyncRequest{SyncRequest: &pmv1.SyncRequest{}},
	}
}

// A rejected LUKS result is the concrete case: the agent stamps
// luks.device_path onto every setup success and the server refuses any
// non-empty metadata. Before the fix this one frame closed the stream, so the
// result was either lost outright or replayed on every reconnect forever.
func TestStreamSurvivesRejectedActionResult(t *testing.T) {
	f := newStreamFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stream := f.open(t, ctx)

	result := luksResult(f.own, map[string]string{"luks.device_path": "/dev/sda2"})
	require.NoError(t, stream.Send(result))
	ack, err := stream.Receive()
	require.NoError(t, err)
	require.NotNil(t, ack.GetResultAck())
	assert.Equal(t, result.Id, ack.Id)
	assert.False(t, ack.GetResultAck().Accepted)

	// The connection must still be usable: a request frame sent after the
	// refused one still gets its answer.
	require.NoError(t, stream.Send(syncRequest()))
	response, err := stream.Receive()
	require.NoError(t, err, "a rejected application frame must not end the stream")
	require.NotNil(t, response.GetSyncState())

	// The rejection is real — the result did not land.
	row, err := f.store.GetExecution(ctx, f.own.occurrence)
	require.NoError(t, err)
	assert.Equal(t, "pending", row.Status, "a frame the server refused must not be recorded as applied")
}

// The counterpart: a frame claiming another device's execution is an
// authorization failure, and that still ends the connection.
func TestStreamTerminatesOnCrossDeviceClaim(t *testing.T) {
	f := newStreamFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stream := f.open(t, ctx)

	// No follow-up frame here: the connection is expected to be gone, so a
	// second Send would race the teardown. Receive is the deterministic
	// observation point — it carries the code the handler returned.
	result := luksResult(f.foreign, nil)
	require.NoError(t, stream.Send(result))
	ack, err := stream.Receive()
	require.NoError(t, err)
	require.NotNil(t, ack.GetResultAck())
	assert.Equal(t, result.Id, ack.Id)
	assert.False(t, ack.GetResultAck().Accepted)

	_, err = stream.Receive()
	require.Error(t, err, "a cross-device claim must end the connection")
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))

	row, err := f.store.GetExecution(context.Background(), f.foreign.occurrence)
	require.NoError(t, err)
	assert.Equal(t, "pending", row.Status, "another device's execution must be untouched")
}

// And the frame the server does accept still commits, so the two branches
// above distinguish rejection from acceptance, not from a broken sink.
func TestStreamAppliesCleanActionResult(t *testing.T) {
	f := newStreamFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stream := f.open(t, ctx)

	result := luksResult(f.own, nil)
	require.NoError(t, stream.Send(result))
	ack, err := stream.Receive()
	require.NoError(t, err)
	require.NotNil(t, ack.GetResultAck())
	assert.Equal(t, result.Id, ack.Id)
	assert.True(t, ack.GetResultAck().Accepted)
	require.NoError(t, stream.Send(syncRequest()))
	response, err := stream.Receive()
	require.NoError(t, err)
	require.NotNil(t, response.GetSyncState())

	row, err := f.store.GetExecution(ctx, f.own.occurrence)
	require.NoError(t, err)
	assert.Equal(t, "success", row.Status)
}

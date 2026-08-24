package agentstream

import (
	"context"
	"crypto/tls"
	"crypto/x509"
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

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/manchtools/cadestro/server/internal/connection"
	"github.com/manchtools/cadestro/server/internal/mtls"
	"github.com/manchtools/cadestro/server/internal/store"
	"github.com/manchtools/cadestro/server/internal/testdb"
)

type streamTestSecrets struct{}

func (streamTestSecrets) ValidateLuksToken(context.Context, string, *cadestrov1.ValidateLuksTokenRequest) (*cadestrov1.ValidateLuksTokenResponse, error) {
	return &cadestrov1.ValidateLuksTokenResponse{}, nil
}
func (streamTestSecrets) GetLuksKey(context.Context, string, *cadestrov1.GetLuksKeyRequest) (*cadestrov1.GetLuksKeyResponse, error) {
	return &cadestrov1.GetLuksKeyResponse{}, nil
}
func (streamTestSecrets) StoreLuksKey(context.Context, string, *cadestrov1.StoreLuksKeyRequest) (*cadestrov1.StoreLuksKeyResponse, error) {
	return &cadestrov1.StoreLuksKeyResponse{}, nil
}
func (streamTestSecrets) StoreLpsPasswords(context.Context, string, *cadestrov1.StoreLpsPasswordsRequest) (*cadestrov1.StoreLpsPasswordsResponse, error) {
	return &cadestrov1.StoreLpsPasswordsResponse{}, nil
}

type streamTestPolicyResults struct{}

func (streamTestPolicyResults) RecordPolicyManifestResult(context.Context, string, string, string, string, string) error {
	return nil
}

type streamTestSync struct{}

func (streamTestSync) Sync(context.Context, string) (*cadestrov1.SyncState, error) {
	return &cadestrov1.SyncState{SyncIntervalMinutes: 30}, nil
}

type streamTestLiveOperations struct{}

func (streamTestLiveOperations) CompleteSyncDevice(context.Context, string, string, *cadestrov1.SyncDeviceResult) error {
	return nil
}
func (streamTestLiveOperations) CompleteRebootDevice(context.Context, string, string, *cadestrov1.RebootDeviceResult) error {
	return nil
}

type streamTestFixture struct {
	client     cadestrov1connect.AgentServiceClient
	raw        *testdb.DB
	deviceID   string
	peerSerial *big.Int
}

func newStreamTestFixture(t *testing.T) *streamTestFixture {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "cadestro.db")
	st, err := store.New(ctx, path)
	require.NoError(t, err)
	t.Cleanup(st.Close)
	raw, err := testdb.Open(ctx, path)
	require.NoError(t, err)
	t.Cleanup(raw.Close)
	deviceID := ulid.Make().String()
	_, err = raw.Exec(ctx, `
		INSERT INTO devices (id, hostname, agent_version, certificate_pem, active_cert_serial, registered_at)
		VALUES ($1, $2, 'v1', X'01', '1', $3)`, deviceID, "host-"+deviceID, time.Now().UTC())
	require.NoError(t, err)
	serial := big.NewInt(1)
	handler := New(Config{
		Store: st, Manager: connection.NewManager(), PolicyResults: streamTestPolicyResults{},
		Executions: &fakeExecutionResults{}, DeviceResults: &fakeDeviceResults{}, Secrets: streamTestSecrets{},
		Sync: streamTestSync{}, LiveOperations: streamTestLiveOperations{},
		TerminalSessions: connection.NewTerminalSessionRegistry(), Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	t.Cleanup(handler.Close)
	procedure, connectHandler := cadestrov1connect.NewAgentServiceHandler(handler)
	mux := http.NewServeMux()
	mux.Handle(procedure, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestContext := WithDeviceID(r.Context(), deviceID)
		requestContext = mtls.WithPeerCertificate(requestContext, &x509.Certificate{SerialNumber: new(big.Int).Set(serial)})
		connectHandler.ServeHTTP(w, r.WithContext(requestContext))
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
	return &streamTestFixture{
		client: cadestrov1connect.NewAgentServiceClient(httpClient, server.URL, connect.WithGRPC()),
		raw:    raw, deviceID: deviceID, peerSerial: serial,
	}
}

func (f *streamTestFixture) open(t *testing.T, ctx context.Context) *connect.BidiStreamForClient[cadestrov1.AgentMessage, cadestrov1.ServerMessage] {
	t.Helper()
	stream := f.client.Stream(ctx)
	t.Cleanup(func() { _ = stream.CloseRequest() })
	require.NoError(t, stream.Send(&cadestrov1.AgentMessage{Id: ulid.Make().String(), Payload: &cadestrov1.AgentMessage_Hello{Hello: &cadestrov1.Hello{
		DeviceId: &cadestrov1.DeviceId{Value: f.deviceID}, AgentVersion: "v1", Hostname: "device",
	}}}))
	welcome, err := stream.Receive()
	require.NoError(t, err)
	require.NotNil(t, welcome.GetWelcome())
	return stream
}

func TestPendingHelloPromotesAndOldActiveIsRejected(t *testing.T) {
	f := newStreamTestFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := f.raw.Exec(ctx, `UPDATE devices SET pending_certificate_pem = X'02', pending_cert_serial = '2' WHERE id = ?`, f.deviceID)
	require.NoError(t, err)
	f.peerSerial.SetInt64(2)
	stream := f.open(t, ctx)
	var active, pending any
	require.NoError(t, f.raw.QueryRow(ctx, `SELECT active_cert_serial, pending_cert_serial FROM devices WHERE id = ?`, f.deviceID).Scan(&active, &pending))
	require.Equal(t, "2", active)
	require.Nil(t, pending)

	f.peerSerial.SetInt64(1)
	stream = f.client.Stream(ctx)
	require.NoError(t, stream.Send(&cadestrov1.AgentMessage{Id: ulid.Make().String(), Payload: &cadestrov1.AgentMessage_Hello{Hello: &cadestrov1.Hello{
		DeviceId: &cadestrov1.DeviceId{Value: f.deviceID}, AgentVersion: "v1", Hostname: "device",
	}}}))
	_, err = stream.Receive()
	require.Error(t, err)
	var connectErr *connect.Error
	if assert.ErrorAs(t, err, &connectErr) {
		assert.Equal(t, connect.CodePermissionDenied, connectErr.Code())
	}
}

func TestStreamRejectsAlreadyOpenPeerAfterSerialPromotion(t *testing.T) {
	f := newStreamTestFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stream := f.open(t, ctx)
	_, err := f.raw.Exec(ctx, `UPDATE devices SET certificate_pem = X'02', active_cert_serial = '2' WHERE id = ?`, f.deviceID)
	require.NoError(t, err)
	require.NoError(t, stream.Send(&cadestrov1.AgentMessage{Id: ulid.Make().String(), Payload: &cadestrov1.AgentMessage_SyncRequest{SyncRequest: &cadestrov1.SyncRequest{}}}))
	_, err = stream.Receive()
	require.Error(t, err)
	var connectErr *connect.Error
	if assert.ErrorAs(t, err, &connectErr) {
		assert.Equal(t, connect.CodePermissionDenied, connectErr.Code())
	}
}

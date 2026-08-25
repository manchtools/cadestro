package deviceauth

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"

	"github.com/manchtools/cadestro/agent/internal/credentials"
	sdk "github.com/manchtools/cadestro/contract"
)

var testCAPin = strings.Repeat("0", 64)

// mockRegisterService implements the Register RPC of ControlServiceHandler.
type mockRegisterService struct {
	cadestrov1connect.UnimplementedControlServiceHandler

	registerFunc func(context.Context, *connect.Request[cadestrov1.RegisterRequest]) (*connect.Response[cadestrov1.RegisterResponse], error)
}

func (m *mockRegisterService) Register(ctx context.Context, req *connect.Request[cadestrov1.RegisterRequest]) (*connect.Response[cadestrov1.RegisterResponse], error) {
	if m.registerFunc != nil {
		return m.registerFunc(ctx, req)
	}
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

// startMockControlServer starts an httptest TLS control server (the
// agent enforces https-only enrollment, so a plain-http test server
// would be rejected by the gate before RegisterAgent runs).
func startMockControlServer(t *testing.T, mock *mockRegisterService) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	path, handler := cadestrov1connect.NewControlServiceHandler(mock)
	mux.Handle(path, handler)
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// trustServer returns the registerOpts that make sdk.RegisterAgent trust
// the httptest TLS server's self-signed certificate.
func trustServer(srv *httptest.Server) []sdk.ClientOption {
	return []sdk.ClientOption{sdk.WithHTTPClient(srv.Client())}
}

func TestEnroll_Success(t *testing.T) {
	caPEM := genTestCAPEM(t)
	mock := &mockRegisterService{
		registerFunc: func(_ context.Context, req *connect.Request[cadestrov1.RegisterRequest]) (*connect.Response[cadestrov1.RegisterResponse], error) {
			return connect.NewResponse(&cadestrov1.RegisterResponse{
				DeviceId:    &cadestrov1.DeviceId{Value: "dev-123"},
				CaCert:      caPEM,
				Certificate: []byte("-----BEGIN CERTIFICATE-----\nfake-cert\n-----END CERTIFICATE-----\n"),
				ControlUrl:  "https://gw.example.com:8443",
			}), nil
		},
	}
	srv := startMockControlServer(t, mock)

	credStore := credentials.NewStore(t.TempDir())
	logger := slog.Default()

	var enrolledCreds *credentials.Credentials
	handler := NewEnrollHandler("test-host", "dev", credStore, logger, func(creds *credentials.Credentials) {
		enrolledCreds = creds
	})
	handler.registerOpts = trustServer(srv)

	resp, err := handler.Enroll(context.Background(), connect.NewRequest(&cadestrov1.EnrollRequest{
		ServerUrl: srv.URL, Token: "test-token", CaFingerprintPin: caPin(t, caPEM),
	}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.Success)
	assert.Equal(t, "dev-123", resp.Msg.GetDeviceId().GetValue())
	assert.Empty(t, resp.Msg.Error)

	// Callback was called
	require.NotNil(t, enrolledCreds)
	assert.Equal(t, "dev-123", enrolledCreds.DeviceID)
	assert.Equal(t, "https://gw.example.com:8443", enrolledCreds.AgentAddr)
	assert.Equal(t, srv.URL, enrolledCreds.ControlAddr)

	// Credentials saved to store
	assert.True(t, credStore.Exists())
	loaded, err := credStore.Load()
	require.NoError(t, err)
	assert.Equal(t, "dev-123", loaded.DeviceID)
}

func TestEnroll_MissingFields(t *testing.T) {
	credStore := credentials.NewStore(t.TempDir())
	logger := slog.Default()
	handler := NewEnrollHandler("test-host", "dev", credStore, logger, nil)

	resp, err := handler.Enroll(context.Background(), connect.NewRequest(&cadestrov1.EnrollRequest{
		ServerUrl: "",
		Token:     "",
	}))
	require.NoError(t, err)
	assert.False(t, resp.Msg.Success)
	assert.Contains(t, resp.Msg.Error, "required")
}

func TestEnroll_AlreadyEnrolled(t *testing.T) {
	// Pre-populate credentials
	credStore := credentials.NewStore(t.TempDir())
	credStore.Save(&credentials.Credentials{
		DeviceID:    "existing-device",
		CACert:      []byte("ca"),
		Certificate: []byte("cert"),
		PrivateKey:  []byte("key"),
		AgentAddr:   "https://gw.example.com",
	})

	logger := slog.Default()
	handler := NewEnrollHandler("test-host", "dev", credStore, logger, nil)

	resp, err := handler.Enroll(context.Background(), connect.NewRequest(&cadestrov1.EnrollRequest{
		ServerUrl:        "https://example.com",
		Token:            "token",
		CaFingerprintPin: testCAPin,
	}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.Success) // Returns success with existing device ID
	assert.Equal(t, "existing-device", resp.Msg.GetDeviceId().GetValue())
	assert.Contains(t, resp.Msg.Error, "already enrolled")
}

func TestEnroll_RegistrationFails(t *testing.T) {
	mock := &mockRegisterService{
		registerFunc: func(_ context.Context, _ *connect.Request[cadestrov1.RegisterRequest]) (*connect.Response[cadestrov1.RegisterResponse], error) {
			return nil, connect.NewError(connect.CodePermissionDenied, nil)
		},
	}
	srv := startMockControlServer(t, mock)

	credStore := credentials.NewStore(t.TempDir())
	logger := slog.Default()
	handler := NewEnrollHandler("test-host", "dev", credStore, logger, nil)
	handler.registerOpts = trustServer(srv)

	resp, err := handler.Enroll(context.Background(), connect.NewRequest(&cadestrov1.EnrollRequest{
		ServerUrl: srv.URL, Token: "bad-token", CaFingerprintPin: testCAPin,
	}))
	require.NoError(t, err)
	assert.False(t, resp.Msg.Success)
	assert.Contains(t, resp.Msg.Error, "registration failed")
}

func TestGetEnrollmentStatus_NotEnrolled(t *testing.T) {
	credStore := credentials.NewStore(t.TempDir())
	logger := slog.Default()
	handler := NewEnrollHandler("test-host", "dev", credStore, logger, nil)

	resp, err := handler.GetEnrollmentStatus(context.Background(), connect.NewRequest(&cadestrov1.GetEnrollmentStatusRequest{}))
	require.NoError(t, err)
	assert.False(t, resp.Msg.Enrolled)
	assert.Empty(t, resp.Msg.GetDeviceId().GetValue())
}

func TestGetEnrollmentStatus_Enrolled(t *testing.T) {
	credStore := credentials.NewStore(t.TempDir())
	credStore.Save(&credentials.Credentials{
		DeviceID:    "dev-abc",
		CACert:      []byte("ca"),
		Certificate: []byte("cert"),
		PrivateKey:  []byte("key"),
		AgentAddr:   "https://gw.example.com",
	})

	logger := slog.Default()
	handler := NewEnrollHandler("test-host", "dev", credStore, logger, nil)

	resp, err := handler.GetEnrollmentStatus(context.Background(), connect.NewRequest(&cadestrov1.GetEnrollmentStatusRequest{}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.Enrolled)
	assert.Equal(t, "dev-abc", resp.Msg.GetDeviceId().GetValue())
}

func TestEnrollServer_EndToEnd(t *testing.T) {
	caPEM := genTestCAPEM(t)
	mock := &mockRegisterService{
		registerFunc: func(_ context.Context, _ *connect.Request[cadestrov1.RegisterRequest]) (*connect.Response[cadestrov1.RegisterResponse], error) {
			return connect.NewResponse(&cadestrov1.RegisterResponse{
				DeviceId:    &cadestrov1.DeviceId{Value: "dev-e2e"},
				CaCert:      caPEM,
				Certificate: []byte("-----BEGIN CERTIFICATE-----\ncert\n-----END CERTIFICATE-----\n"),
				ControlUrl:  "https://gw.example.com",
			}), nil
		},
	}
	controlSrv := startMockControlServer(t, mock)

	credStore := credentials.NewStore(t.TempDir())
	logger := slog.Default()

	enrollCh := make(chan *credentials.Credentials, 1)
	enrollHandler := NewEnrollHandler("test-host", "dev", credStore, logger, func(creds *credentials.Credentials) {
		enrollCh <- creds
	})
	enrollHandler.registerOpts = trustServer(controlSrv)

	socketPath := filepath.Join(t.TempDir(), "enroll.sock")
	enrollServer := NewEnrollServer(enrollHandler, socketPath, logger)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	go enrollServer.Start(ctx)

	// Wait for socket to be ready
	require.Eventually(t, func() bool {
		conn, err := net.Dial("unix", socketPath)
		if err != nil {
			return false
		}
		conn.Close()
		return true
	}, 2*time.Second, 10*time.Millisecond)

	// Create client over unix socket
	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
			},
		},
	}
	client := cadestrov1connect.NewDeviceAuthServiceClient(httpClient, "http://localhost")

	// Check status: not enrolled
	status, err := client.GetEnrollmentStatus(context.Background(), connect.NewRequest(&cadestrov1.GetEnrollmentStatusRequest{}))
	require.NoError(t, err)
	assert.False(t, status.Msg.Enrolled)

	// Enroll
	resp, err := client.Enroll(context.Background(), connect.NewRequest(&cadestrov1.EnrollRequest{
		ServerUrl: controlSrv.URL, Token: "test-token", CaFingerprintPin: caPin(t, caPEM),
	}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.Success)
	assert.Equal(t, "dev-e2e", resp.Msg.GetDeviceId().GetValue())

	// Callback received
	select {
	case creds := <-enrollCh:
		assert.Equal(t, "dev-e2e", creds.DeviceID)
	case <-time.After(2 * time.Second):
		t.Fatal("enrollment callback not received")
	}

	// Check status again: enrolled
	status, err = client.GetEnrollmentStatus(context.Background(), connect.NewRequest(&cadestrov1.GetEnrollmentStatusRequest{}))
	require.NoError(t, err)
	assert.True(t, status.Msg.Enrolled)
	assert.Equal(t, "dev-e2e", status.Msg.GetDeviceId().GetValue())
}

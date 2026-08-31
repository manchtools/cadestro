package deviceauth

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"

	"github.com/manchtools/cadestro/agent/internal/credentials"
)

type failingStore struct {
	credentialStore
	saveErr error
}

func (f *failingStore) Save(context.Context, *credentials.Credentials) error { return f.saveErr }

type failFinalSaveStore struct {
	credentialStore
	finalSaveFailed bool
}

func (f *failFinalSaveStore) Save(ctx context.Context, creds *credentials.Credentials) error {
	if len(creds.PendingCSR) == 0 && !f.finalSaveFailed {
		f.finalSaveFailed = true
		return errors.New("response lost after server commit")
	}
	return f.credentialStore.Save(ctx, creds)
}

func TestEnroll_RetryReusesPendingIdentityAfterResponseLoss(t *testing.T) {
	caPEM := genTestCAPEM(t)
	var csrs [][]byte
	mock := &mockRegisterService{
		registerFunc: func(_ context.Context, req *connect.Request[cadestrov1.RegisterRequest]) (*connect.Response[cadestrov1.RegisterResponse], error) {
			csrs = append(csrs, append([]byte(nil), req.Msg.Csr...))
			return connect.NewResponse(&cadestrov1.RegisterResponse{
				DeviceId:    &cadestrov1.DeviceId{Value: "retry-device"},
				CaCert:      caPEM,
				Certificate: []byte(fakeLeafPEM),
				ControlUrl:  "https://gw.example.com",
			}), nil
		},
	}
	srv := startMockControlServer(t, mock)
	store := credentials.NewStore(t.TempDir())
	h := NewEnrollHandler("test-host", "dev", store, slog.Default(), nil)
	h.registerOpts = trustServer(srv)
	h.credStore = &failFinalSaveStore{credentialStore: store}
	req := func() *connect.Request[cadestrov1.EnrollRequest] {
		return connect.NewRequest(&cadestrov1.EnrollRequest{ServerUrl: srv.URL, Token: "reusable", CaFingerprintPin: caPin(t, caPEM)})
	}
	first, err := h.Enroll(context.Background(), req())
	require.NoError(t, err)
	assert.False(t, first.Msg.Success)
	assert.Contains(t, first.Msg.Error, "save credentials")
	second, err := h.Enroll(context.Background(), req())
	require.NoError(t, err)
	assert.True(t, second.Msg.Success, "%s", second.Msg.Error)
	assert.Len(t, csrs, 2)
	assert.Equal(t, csrs[0], csrs[1], "retry must submit the same CSR identity")
	loaded, err := store.Load()
	require.NoError(t, err)
	assert.Empty(t, loaded.PendingCSR)
	assert.Empty(t, loaded.PendingPrivateKey)
}

func TestEnroll_RateLimitRejectsSixthInWindow(t *testing.T) {
	var registerCalls int32
	mock := &mockRegisterService{
		registerFunc: func(_ context.Context, _ *connect.Request[cadestrov1.RegisterRequest]) (*connect.Response[cadestrov1.RegisterResponse], error) {
			atomic.AddInt32(&registerCalls, 1)
			return nil, connect.NewError(connect.CodePermissionDenied, nil)
		},
	}
	srv := startMockControlServer(t, mock)

	credStore := credentials.NewStore(t.TempDir())
	h := NewEnrollHandler("test-host", "dev", credStore, slog.Default(), nil)
	h.registerOpts = trustServer(srv)
	fixed := time.Now()
	h.now = func() time.Time { return fixed }

	for i := 0; i < 5; i++ {
		resp, err := h.Enroll(context.Background(), connect.NewRequest(&cadestrov1.EnrollRequest{
			ServerUrl: srv.URL, Token: "tok", CaFingerprintPin: testCAPin,
		}))
		require.NoError(t, err)
		assert.Contains(t, resp.Msg.Error, "registration failed", "attempt %d should reach (and fail) registration", i+1)
	}

	resp, err := h.Enroll(context.Background(), connect.NewRequest(&cadestrov1.EnrollRequest{
		ServerUrl: srv.URL, Token: "tok", CaFingerprintPin: testCAPin,
	}))
	require.NoError(t, err)
	assert.False(t, resp.Msg.Success)
	assert.Contains(t, resp.Msg.Error, "rate limit")
	assert.EqualValues(t, 5, atomic.LoadInt32(&registerCalls), "the 6th attempt must not reach the network")
}

func TestEnroll_RateLimitSlidingWindowEviction(t *testing.T) {
	var registerCalls int32
	mock := &mockRegisterService{
		registerFunc: func(_ context.Context, _ *connect.Request[cadestrov1.RegisterRequest]) (*connect.Response[cadestrov1.RegisterResponse], error) {
			atomic.AddInt32(&registerCalls, 1)
			return nil, connect.NewError(connect.CodePermissionDenied, nil)
		},
	}
	srv := startMockControlServer(t, mock)
	credStore := credentials.NewStore(t.TempDir())
	h := NewEnrollHandler("test-host", "dev", credStore, slog.Default(), nil)
	h.registerOpts = trustServer(srv)
	now := time.Now()
	h.now = func() time.Time { return now }

	for i := 0; i < 6; i++ {
		_, err := h.Enroll(context.Background(), connect.NewRequest(&cadestrov1.EnrollRequest{ServerUrl: srv.URL, Token: "tok", CaFingerprintPin: testCAPin}))
		require.NoError(t, err)
	}
	require.EqualValues(t, 5, atomic.LoadInt32(&registerCalls), "only 5 attempts may reach the network within one window")

	now = now.Add(61 * time.Second)
	resp, err := h.Enroll(context.Background(), connect.NewRequest(&cadestrov1.EnrollRequest{ServerUrl: srv.URL, Token: "tok", CaFingerprintPin: testCAPin}))
	require.NoError(t, err)
	assert.Contains(t, resp.Msg.Error, "registration failed", "after the window resets, enrollment is allowed through again")
	assert.EqualValues(t, 6, atomic.LoadInt32(&registerCalls), "a fresh attempt after the window must reach registration")
}

func TestEnroll_ConcurrentSerializesToOneRegistration(t *testing.T) {
	var registerCalls int32
	caPEM := genTestCAPEM(t)
	pin := caPin(t, caPEM)
	mock := &mockRegisterService{
		registerFunc: func(_ context.Context, _ *connect.Request[cadestrov1.RegisterRequest]) (*connect.Response[cadestrov1.RegisterResponse], error) {
			atomic.AddInt32(&registerCalls, 1)
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
	h := NewEnrollHandler("test-host", "dev", credStore, slog.Default(), nil)
	h.registerOpts = trustServer(srv)

	const n = 5
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := h.Enroll(context.Background(), connect.NewRequest(&cadestrov1.EnrollRequest{
				ServerUrl: srv.URL, Token: "tok", CaFingerprintPin: pin,
			})); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("Enroll: %v", err)
	}

	assert.EqualValues(t, 1, atomic.LoadInt32(&registerCalls),
		"enrollMu must serialize concurrent enrollments so exactly one device registers; the rest short-circuit on Exists()")
	assert.True(t, credStore.Exists(), "the single enrollment must have saved credentials")
}

func TestEnroll_RejectsMissingMTLSCerts(t *testing.T) {
	cases := []struct {
		name     string
		ca, cert []byte
	}{
		{"ca only", []byte("ca"), nil},
		{"cert only", nil, []byte("cert")},
		{"both empty", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockRegisterService{
				registerFunc: func(_ context.Context, _ *connect.Request[cadestrov1.RegisterRequest]) (*connect.Response[cadestrov1.RegisterResponse], error) {
					return connect.NewResponse(&cadestrov1.RegisterResponse{
						DeviceId:    &cadestrov1.DeviceId{Value: "01HZZZZZZZZZZZZZZZZZZZZZZZZ"},
						CaCert:      tc.ca,
						Certificate: tc.cert,
						ControlUrl:  "https://gw.example.com",
					}), nil
				},
			}
			srv := startMockControlServer(t, mock)
			credStore := credentials.NewStore(t.TempDir())
			h := NewEnrollHandler("test-host", "dev", credStore, slog.Default(), nil)
			h.registerOpts = trustServer(srv)

			resp, err := h.Enroll(context.Background(), connect.NewRequest(&cadestrov1.EnrollRequest{
				ServerUrl: srv.URL, Token: "tok", CaFingerprintPin: testCAPin,
			}))
			require.NoError(t, err)
			assert.False(t, resp.Msg.Success)
			assert.Contains(t, resp.Msg.Error, "mTLS certificates")
			pending, loadErr := credStore.Load()
			require.NoError(t, loadErr)
			assert.NotEmpty(t, pending.PendingCSR, "failed completion must retain the retry identity")
			assert.Empty(t, pending.DeviceID, "failed completion must not create active credentials")
		})
	}
}

func TestEnroll_BindsOutboundRegisterRequest(t *testing.T) {
	var captured *cadestrov1.RegisterRequest
	caPEM := genTestCAPEM(t)
	mock := &mockRegisterService{
		registerFunc: func(_ context.Context, req *connect.Request[cadestrov1.RegisterRequest]) (*connect.Response[cadestrov1.RegisterResponse], error) {
			captured = req.Msg
			return connect.NewResponse(&cadestrov1.RegisterResponse{
				DeviceId:    &cadestrov1.DeviceId{Value: "01HZZZZZZZZZZZZZZZZZZZZZZZZ"},
				CaCert:      caPEM,
				Certificate: []byte(fakeLeafPEM),
				ControlUrl:  "https://gw.example.com",
			}), nil
		},
	}
	srv := startMockControlServer(t, mock)
	credStore := credentials.NewStore(t.TempDir())
	h := NewEnrollHandler("test-host", "dev", credStore, slog.Default(), nil)
	h.registerOpts = trustServer(srv)

	resp, err := h.Enroll(context.Background(), connect.NewRequest(&cadestrov1.EnrollRequest{
		ServerUrl: srv.URL, Token: "test-token", CaFingerprintPin: caPin(t, caPEM),
	}))
	require.NoError(t, err)
	require.True(t, resp.Msg.Success, "%s", resp.Msg.Error)

	require.NotNil(t, captured)
	assert.Equal(t, "test-token", captured.Token)
	assert.Equal(t, "test-host", captured.Hostname)
	assert.Equal(t, "dev", captured.AgentVersion)

	block, _ := pem.Decode(captured.Csr)
	require.NotNil(t, block, "CSR must be PEM")
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	require.NoError(t, err)
	require.NoError(t, csr.CheckSignature(), "CSR signature must verify")
}

func TestEnroll_SaveFailureFailsClosed(t *testing.T) {
	caPEM := genTestCAPEM(t)
	srv := startMockControlServer(t, caReturningMock(caPEM))
	called := false
	h := NewEnrollHandler("test-host", "dev", credentials.NewStore(t.TempDir()), slog.Default(), func(*credentials.Credentials) { called = true })
	h.registerOpts = trustServer(srv)
	h.credStore = &failingStore{credentialStore: h.credStore, saveErr: errors.New("disk full")}

	resp, err := h.Enroll(context.Background(), connect.NewRequest(&cadestrov1.EnrollRequest{
		ServerUrl: srv.URL, Token: "tok", CaFingerprintPin: caPin(t, caPEM),
	}))
	require.NoError(t, err)
	assert.False(t, resp.Msg.Success)
	assert.Contains(t, resp.Msg.Error, "save credentials")
	assert.False(t, called, "onEnrolled must not fire when Save fails")

	st, err := h.GetEnrollmentStatus(context.Background(), connect.NewRequest(&cadestrov1.GetEnrollmentStatusRequest{}))
	require.NoError(t, err)
	assert.False(t, st.Msg.Enrolled, "status cache must not be primed on Save failure")
}

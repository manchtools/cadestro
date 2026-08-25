package contract

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
)

type agentLoopback struct {
	srv        *httptest.Server
	serverURL  string
	handler    *recordingAgentHandler
	httpClient *http.Client
}

type recordingAgentHandler struct {
	mu       sync.Mutex
	received []*cadestrov1.AgentMessage

	onStream func(ctx context.Context, stream *connect.BidiStream[cadestrov1.AgentMessage, cadestrov1.ServerMessage]) error

	syncState *cadestrov1.SyncState
}

func (h *recordingAgentHandler) Stream(ctx context.Context, s *connect.BidiStream[cadestrov1.AgentMessage, cadestrov1.ServerMessage]) error {
	if h.onStream != nil {
		return h.onStream(ctx, s)
	}
	for {
		msg, err := s.Receive()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		h.mu.Lock()
		h.received = append(h.received, msg)
		h.mu.Unlock()
		if msg.GetSyncRequest() != nil {
			state := h.syncState
			if state == nil {
				state = &cadestrov1.SyncState{}
			}
			if err := s.Send(&cadestrov1.ServerMessage{
				Id:      &cadestrov1.MessageId{Value: msg.GetId().GetValue()},
				Payload: &cadestrov1.ServerMessage_SyncState{SyncState: state},
			}); err != nil {
				return err
			}
		}
	}
}

func (h *recordingAgentHandler) snapshot() []*cadestrov1.AgentMessage {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]*cadestrov1.AgentMessage, len(h.received))
	copy(out, h.received)
	return out
}

func newAgentLoopback(t *testing.T) *agentLoopback {
	t.Helper()

	handler := &recordingAgentHandler{}
	path, h := cadestrov1connect.NewAgentServiceHandler(handler)
	mux := http.NewServeMux()
	mux.Handle(path, h)

	proto := new(http.Protocols)
	proto.SetUnencryptedHTTP2(true)

	srv := httptest.NewUnstartedServer(mux)
	srv.Config.Protocols = proto
	srv.Start()
	t.Cleanup(srv.Close)

	hc := &http.Client{
		Transport: &http.Transport{
			Protocols: proto,
		},
	}

	return &agentLoopback{
		srv:        srv,
		serverURL:  srv.URL,
		handler:    handler,
		httpClient: hc,
	}
}

func (l *agentLoopback) newClient(extra ...ClientOption) *Client {
	opts := append([]ClientOption{WithHTTPClient(l.httpClient)}, extra...)
	return NewClient(l.serverURL, opts...)
}

type controlLoopback struct {
	srv       *httptest.Server
	serverURL string
	handler   *recordingControlHandler
}

type recordingControlHandler struct {
	cadestrov1connect.UnimplementedControlServiceHandler

	registerFn         func(*connect.Request[cadestrov1.RegisterRequest]) (*connect.Response[cadestrov1.RegisterResponse], error)
	renewCertificateFn func(*connect.Request[cadestrov1.RenewCertificateRequest]) (*connect.Response[cadestrov1.RenewCertificateResponse], error)
}

func (h *recordingControlHandler) Register(ctx context.Context, req *connect.Request[cadestrov1.RegisterRequest]) (*connect.Response[cadestrov1.RegisterResponse], error) {
	if h.registerFn != nil {
		return h.registerFn(req)
	}
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("Register not stubbed"))
}

func (h *recordingControlHandler) RenewCertificate(ctx context.Context, req *connect.Request[cadestrov1.RenewCertificateRequest]) (*connect.Response[cadestrov1.RenewCertificateResponse], error) {
	if h.renewCertificateFn != nil {
		return h.renewCertificateFn(req)
	}
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("RenewCertificate not stubbed"))
}

func newControlLoopback(t *testing.T) *controlLoopback {
	t.Helper()

	handler := &recordingControlHandler{}
	path, h := cadestrov1connect.NewControlServiceHandler(handler)
	mux := http.NewServeMux()
	mux.Handle(path, h)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &controlLoopback{
		srv:       srv,
		serverURL: srv.URL,
		handler:   handler,
	}
}

func TestRegisterAgent_HappyPath(t *testing.T) {
	cl := newControlLoopback(t)

	var observed *cadestrov1.RegisterRequest
	cl.handler.registerFn = func(req *connect.Request[cadestrov1.RegisterRequest]) (*connect.Response[cadestrov1.RegisterResponse], error) {
		observed = req.Msg
		return connect.NewResponse(&cadestrov1.RegisterResponse{
			DeviceId:    &cadestrov1.DeviceId{Value: "01HXXXXXXXXXXXXXXXXXXXXXX0"},
			CaCert:      []byte("ca"),
			Certificate: []byte("cert"),
			ControlUrl:  "https://control.example",
		}), nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	got, err := RegisterAgent(ctx, cl.serverURL, "token-x", "host-1", "v1.2.3", []byte("csr-bytes"))
	if err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	if got.DeviceID != "01HXXXXXXXXXXXXXXXXXXXXXX0" {
		t.Errorf("DeviceID = %q", got.DeviceID)
	}
	if string(got.Certificate) != "cert" || string(got.CACert) != "ca" {
		t.Errorf("certs not threaded through")
	}
	if got.ControlURL != "https://control.example" {
		t.Errorf("ControlURL = %q", got.ControlURL)
	}
	if observed == nil {
		t.Fatal("server never observed the Register request")
	}
	if observed.Token != "token-x" || observed.Hostname != "host-1" ||
		observed.AgentVersion != "v1.2.3" || string(observed.Csr) != "csr-bytes" {
		t.Errorf("request fields lost in transit: %+v", observed)
	}
}

func TestRegisterAgent_ServerErrorPropagates(t *testing.T) {
	cl := newControlLoopback(t)
	cl.handler.registerFn = func(_ *connect.Request[cadestrov1.RegisterRequest]) (*connect.Response[cadestrov1.RegisterResponse], error) {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("bad token"))
	}

	_, err := RegisterAgent(context.Background(), cl.serverURL, "wrong", "host", "v0", []byte("csr"))
	if err == nil {
		t.Fatal("expected error from server-side PermissionDenied")
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("want *connect.Error, got %T: %v", err, err)
	}
	if connectErr.Code() != connect.CodePermissionDenied {
		t.Errorf("code = %v", connectErr.Code())
	}
}

func TestRenewCertificate_HappyPath(t *testing.T) {
	cl := newControlLoopback(t)

	notAfter := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	var observed *cadestrov1.RenewCertificateRequest
	cl.handler.renewCertificateFn = func(req *connect.Request[cadestrov1.RenewCertificateRequest]) (*connect.Response[cadestrov1.RenewCertificateResponse], error) {
		observed = req.Msg
		return connect.NewResponse(&cadestrov1.RenewCertificateResponse{
			Certificate: []byte("renewed-cert"),
			NotAfter:    timestamppb.New(notAfter),
		}), nil
	}

	got, err := RenewCertificate(context.Background(), cl.serverURL, []byte("new-csr"))
	if err != nil {
		t.Fatalf("RenewCertificate: %v", err)
	}
	if string(got.Certificate) != "renewed-cert" {
		t.Errorf("Certificate = %q", got.Certificate)
	}
	if !got.NotAfter.Equal(notAfter) {
		t.Errorf("NotAfter = %v want %v", got.NotAfter, notAfter)
	}
	if observed == nil || string(observed.Csr) != "new-csr" {
		t.Errorf("request lost fields: %+v", observed)
	}
}

func TestRenewCertificate_ServerErrorPropagates(t *testing.T) {
	cl := newControlLoopback(t)
	cl.handler.renewCertificateFn = func(_ *connect.Request[cadestrov1.RenewCertificateRequest]) (*connect.Response[cadestrov1.RenewCertificateResponse], error) {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("expired cert"))
	}

	_, err := RenewCertificate(context.Background(), cl.serverURL, []byte("csr"))
	if err == nil {
		t.Fatal("expected error")
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("want *connect.Error, got %T", err)
	}
	if connectErr.Code() != connect.CodeUnauthenticated {
		t.Errorf("code = %v", connectErr.Code())
	}
}

func TestConnect_DoubleConnectErrors(t *testing.T) {
	l := newAgentLoopback(t)
	c := l.newClient()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("first Connect: %v", err)
	}
	defer c.Close()

	if err := c.Connect(ctx); err == nil {
		t.Fatal("second Connect should error")
	}
}

func TestSync_MapsMessageFieldsThroughFacade(t *testing.T) {
	l := newAgentLoopback(t)
	l.handler.syncState = &cadestrov1.SyncState{
		SyncIntervalMinutes: 42,
		MaintenanceWindow: &cadestrov1.MaintenanceWindow{
			Schedule: []*cadestrov1.MaintenanceWindowEntry{
				{Days: []string{"sat", "sun"}, Allow: "22:00-06:00"},
			},
		},
	}
	c := l.newClient(WithAuth("01HKDEVICE0000000000000000", "tok"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()
	stopReceiver := c.StartReceiver(ctx)
	defer stopReceiver()

	res, err := c.Sync(ctx)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if res.MaintenanceWindow == nil {
		t.Fatal("MaintenanceWindow dropped by the SyncStateResult facade")
	}
	if len(res.MaintenanceWindow.Schedule) != 1 ||
		res.MaintenanceWindow.Schedule[0].Allow != "22:00-06:00" ||
		len(res.MaintenanceWindow.Schedule[0].Days) != 2 ||
		res.MaintenanceWindow.Schedule[0].Days[0] != "sat" ||
		res.MaintenanceWindow.Schedule[0].Days[1] != "sun" {
		t.Errorf("MaintenanceWindow mismatch: %+v", res.MaintenanceWindow)
	}

	if res.SyncIntervalMinutes != 42 {
		t.Errorf("SyncIntervalMinutes = %d, want 42", res.SyncIntervalMinutes)
	}
}

func TestSend_BeforeConnect_ReturnsError(t *testing.T) {
	l := newAgentLoopback(t)
	c := l.newClient()

	if err := c.SendHeartbeat(context.Background(), &cadestrov1.Heartbeat{}); err == nil {
		t.Fatal("SendHeartbeat without Connect should error")
	}
}

func TestSend_AfterClose_ReturnsError(t *testing.T) {
	l := newAgentLoopback(t)
	c := l.newClient()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := c.SendHeartbeat(context.Background(), &cadestrov1.Heartbeat{}); err == nil {
		t.Fatal("SendHeartbeat after Close should error")
	}
}

func TestClose_IsIdempotent(t *testing.T) {
	l := newAgentLoopback(t)
	c := l.newClient()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second Close should be no-op, got: %v", err)
	}
}

func TestConcurrentSend_PreservesEveryMessage(t *testing.T) {
	l := newAgentLoopback(t)
	c := l.newClient(WithAuth("device-x", "tok"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()

	if err := c.SendHello(ctx, "h", "v"); err != nil {
		t.Fatalf("SendHello: %v", err)
	}

	const (
		goroutines = 20
		perG       = 25
	)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perG; i++ {

				ar := &cadestrov1.ActionResult{
					ActionId: &cadestrov1.ActionId{Value: fmt.Sprintf("g%d-i%d", g, i)},
				}
				if err := c.SendActionResult(ctx, ar); err != nil {
					t.Errorf("send g=%d i=%d: %v", g, i, err)
					return
				}
			}
		}(g)
	}
	wg.Wait()

	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	var got []*cadestrov1.AgentMessage
	want := 1 + goroutines*perG
	for {
		got = l.handler.snapshot()
		if len(got) >= want || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(got) != want {
		t.Fatalf("received %d messages, want %d (drops or dupes break serialisation guarantee)", len(got), want)
	}

	if got[0].GetHello() == nil {
		t.Errorf("first message = %T, want Hello", got[0].Payload)
	}

	seen := make(map[string]int)
	for _, m := range got[1:] {
		ar := m.GetActionResult()
		if ar == nil {
			t.Errorf("non-action-result observed: %T", m.Payload)
			continue
		}
		seen[ar.ActionId.GetValue()]++
	}
	for g := 0; g < goroutines; g++ {
		for i := 0; i < perG; i++ {
			key := fmt.Sprintf("g%d-i%d", g, i)
			if seen[key] != 1 {
				t.Errorf("ActionId %q seen %d times, want 1", key, seen[key])
			}
		}
	}
}

func TestDispatch_NilPayload_IsDropped(t *testing.T) {
	c := NewClient("http://localhost:0")
	handler := &fakeTerminalHandler{}
	msg := &cadestrov1.ServerMessage{Id: &cadestrov1.MessageId{Value: NewULID()}}
	if err := c.dispatchServerMessage(context.Background(), msg, handler); err != nil {
		t.Fatalf("nil payload should not error: %v", err)
	}

	if len(handler.startCalls)+len(handler.inputCalls)+len(handler.resizeCalls)+len(handler.stopCalls) != 0 {
		t.Error("nil payload still reached a handler method")
	}
}

func TestRun_UnknownServerMessage_DoesNotTerminate(t *testing.T) {
	l := newAgentLoopback(t)

	var welcomed atomic.Bool

	l.handler.onStream = func(ctx context.Context, s *connect.BidiStream[cadestrov1.AgentMessage, cadestrov1.ServerMessage]) error {

		if _, err := s.Receive(); err != nil {
			return err
		}

		if err := s.Send(&cadestrov1.ServerMessage{
			Id:      &cadestrov1.MessageId{Value: NewULID()},
			Payload: &cadestrov1.ServerMessage_Welcome{Welcome: &cadestrov1.Welcome{ServerVersion: "test"}},
		}); err != nil {
			return err
		}

		if err := s.Send(&cadestrov1.ServerMessage{Id: &cadestrov1.MessageId{Value: NewULID()}}); err != nil {
			return err
		}

		for {
			if _, err := s.Receive(); err != nil {
				return nil
			}
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := l.newClient(WithAuth("device", "tok"))

	handler := &welcomeRecordingHandler{
		welcomed: &welcomed,
	}

	done := make(chan error, 1)
	go func() {
		done <- c.Run(ctx, "host", "v1", 50*time.Millisecond, handler)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for !welcomed.Load() {
		if time.Now().After(deadline) {
			t.Fatal("Welcome never reached the handler — receive loop died?")
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-done:

		if err != nil && !errors.Is(err, context.Canceled) {

			t.Logf("Run returned: %v (after cancel)", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after ctx cancel")
	}
}

type welcomeRecordingHandler struct {
	welcomed *atomic.Bool
}

func (h *welcomeRecordingHandler) OnWelcome(ctx context.Context, w *cadestrov1.Welcome) error {
	h.welcomed.Store(true)
	return nil
}
func (h *welcomeRecordingHandler) OnQuery(ctx context.Context, q *cadestrov1.OSQuery) (*cadestrov1.OSQueryResult, error) {
	return nil, nil
}
func (h *welcomeRecordingHandler) OnError(ctx context.Context, e *cadestrov1.Error) error { return nil }

func TestWithMTLSFromPEM_ClientPresentsCertificate(t *testing.T) {
	caPEM, caKey, caCert := genCA(t, "test-ca")
	serverCertPEM, serverKeyPEM := genLeaf(t, caCert, caKey, "127.0.0.1", true)
	clientCertPEM, clientKeyPEM := genLeaf(t, caCert, caKey, "device-client", false)

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caPEM) {
		t.Fatal("AppendCertsFromPEM(ca)")
	}
	srvURL := startMTLSTestServer(t, serverCertPEM, serverKeyPEM, caPool)

	t.Run("with cert succeeds", func(t *testing.T) {
		opt, err := WithMTLSFromPEM(clientCertPEM, clientKeyPEM, caPEM)
		if err != nil {
			t.Fatalf("WithMTLSFromPEM: %v", err)
		}
		got, err := RegisterAgent(context.Background(), srvURL,
			"tok", "host", "v0", []byte("csr"), opt)
		if err != nil {
			t.Fatalf("RegisterAgent: %v", err)
		}
		if got.DeviceID != "ok" {
			t.Errorf("DeviceID = %q", got.DeviceID)
		}
	})

	t.Run("without cert handshake fails", func(t *testing.T) {

		tlsConfig := &tls.Config{
			RootCAs:    caPool,
			MinVersion: tls.VersionTLS13,
		}
		hc := newHTTPClientWithTLS(tlsConfig)
		_, err := RegisterAgent(context.Background(), srvURL,
			"tok", "host", "v0", []byte("csr"), WithHTTPClient(hc))
		if err == nil {
			t.Fatal("expected handshake failure without client cert")
		}
	})
}

func startMTLSTestServer(t *testing.T, serverCertPEM, serverKeyPEM []byte, clientCAPool *x509.CertPool) string {
	t.Helper()
	serverCert, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	if err != nil {
		t.Fatalf("server keypair: %v", err)
	}
	handler := &recordingControlHandler{}
	handler.registerFn = func(*connect.Request[cadestrov1.RegisterRequest]) (*connect.Response[cadestrov1.RegisterResponse], error) {
		return connect.NewResponse(&cadestrov1.RegisterResponse{DeviceId: &cadestrov1.DeviceId{Value: "ok"}}), nil
	}
	path, h := cadestrov1connect.NewControlServiceHandler(handler)
	mux := http.NewServeMux()
	mux.Handle(path, h)
	srv := httptest.NewUnstartedServer(mux)
	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    clientCAPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestWithMTLSFromPEM_RejectsServerSignedByForeignCA(t *testing.T) {
	internalCAPEM, _, _ := genCA(t, "internal-ca")

	internalCAForClientPEM, internalKey, internalCert := genCA(t, "internal-ca-2")
	clientCertPEM, clientKeyPEM := genLeaf(t, internalCert, internalKey, "device-client", false)
	clientCAPool := x509.NewCertPool()
	if !clientCAPool.AppendCertsFromPEM(internalCAForClientPEM) {
		t.Fatal("AppendCertsFromPEM(client ca)")
	}

	foreignCAPEM, foreignKey, foreignCert := genCA(t, "foreign-public-ca")
	_ = foreignCAPEM
	serverCertPEM, serverKeyPEM := genLeaf(t, foreignCert, foreignKey, "127.0.0.1", true)
	srvURL := startMTLSTestServer(t, serverCertPEM, serverKeyPEM, clientCAPool)

	opt, err := WithMTLSFromPEM(clientCertPEM, clientKeyPEM, internalCAPEM)
	if err != nil {
		t.Fatalf("WithMTLSFromPEM: %v", err)
	}
	_, err = RegisterAgent(context.Background(), srvURL,
		"tok", "host", "v0", []byte("csr"), opt)
	if err == nil {
		t.Fatal("strict mTLS must reject a server signed by a CA other than the pinned internal CA")
	}
}

func TestWithMTLSFromPEMAndSystemRoots_TrustsInternalCA(t *testing.T) {
	caPEM, caKey, caCert := genCA(t, "internal-ca")
	serverCertPEM, serverKeyPEM := genLeaf(t, caCert, caKey, "127.0.0.1", true)
	clientCertPEM, clientKeyPEM := genLeaf(t, caCert, caKey, "device-client", false)
	clientCAPool := x509.NewCertPool()
	if !clientCAPool.AppendCertsFromPEM(caPEM) {
		t.Fatal("AppendCertsFromPEM(ca)")
	}
	srvURL := startMTLSTestServer(t, serverCertPEM, serverKeyPEM, clientCAPool)

	opt, err := WithMTLSFromPEMAndSystemRoots(clientCertPEM, clientKeyPEM, caPEM)
	if err != nil {
		t.Fatalf("WithMTLSFromPEMAndSystemRoots: %v", err)
	}
	got, err := RegisterAgent(context.Background(), srvURL,
		"tok", "host", "v0", []byte("csr"), opt)
	if err != nil {
		t.Fatalf("system-roots variant must trust a server signed by the internal CA: %v", err)
	}
	if got.DeviceID != "ok" {
		t.Errorf("DeviceID = %q", got.DeviceID)
	}
}

func TestWithMTLSFromPEMAndSystemRoots_RejectsUntrustedServer(t *testing.T) {
	internalCAPEM, _, _ := genCA(t, "internal-ca")
	clientCAPEM, clientKey, clientCACert := genCA(t, "internal-ca-2")
	clientCertPEM, clientKeyPEM := genLeaf(t, clientCACert, clientKey, "device-client", false)
	clientCAPool := x509.NewCertPool()
	if !clientCAPool.AppendCertsFromPEM(clientCAPEM) {
		t.Fatal("AppendCertsFromPEM(client ca)")
	}

	_, foreignKey, foreignCert := genCA(t, "untrusted-ca")
	serverCertPEM, serverKeyPEM := genLeaf(t, foreignCert, foreignKey, "127.0.0.1", true)
	srvURL := startMTLSTestServer(t, serverCertPEM, serverKeyPEM, clientCAPool)

	opt, err := WithMTLSFromPEMAndSystemRoots(clientCertPEM, clientKeyPEM, internalCAPEM)
	if err != nil {
		t.Fatalf("WithMTLSFromPEMAndSystemRoots: %v", err)
	}
	_, err = RegisterAgent(context.Background(), srvURL,
		"tok", "host", "v0", []byte("csr"), opt)
	if err == nil {
		t.Fatal("system-roots variant must still reject a server signed by an untrusted CA")
	}
}

func TestWithHTTPClient_AppliedToControlCalls(t *testing.T) {
	cl := newControlLoopback(t)
	cl.handler.registerFn = func(_ *connect.Request[cadestrov1.RegisterRequest]) (*connect.Response[cadestrov1.RegisterResponse], error) {
		return connect.NewResponse(&cadestrov1.RegisterResponse{DeviceId: &cadestrov1.DeviceId{Value: "id"}}), nil
	}

	var called atomic.Int32
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		called.Add(1)
		return http.DefaultTransport.RoundTrip(req)
	})
	hc := &http.Client{Transport: rt}

	if _, err := RegisterAgent(context.Background(), cl.serverURL,
		"tok", "host", "v0", []byte("csr"), WithHTTPClient(hc)); err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}
	if called.Load() == 0 {
		t.Fatal("WithHTTPClient was ignored by RegisterAgent")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Package sdk provides a client library for communicating with the cadestro server.
package contract

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"reflect"
	"runtime"
	"sync"
	"time"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"github.com/oklog/ulid/v2"
	"golang.org/x/net/http2"
	"google.golang.org/protobuf/proto"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
)

// Heartbeat interval bounds. The SDK clamps server-supplied values from
// Welcome.heartbeat_interval into this range before applying them, so a
// misconfigured or malicious server can never push the cadence outside
// what's safe for both sides (too fast = stream spam, too slow = agent
// looks dead to control's liveness tracking).
const (
	MinHeartbeatInterval = 5 * time.Second
	MaxHeartbeatInterval = 5 * time.Minute
)

// Client provides methods to communicate with the cadestro server.
type Client struct {
	client    cadestrov1connect.AgentServiceClient
	deviceID  string
	authToken string
	logger    *slog.Logger

	httpClient *http.Client

	mu     sync.RWMutex
	stream *connect.BidiStreamForClient[cadestrov1.AgentMessage, cadestrov1.ServerMessage]

	sendSem chan struct{}

	pendingMu       sync.Mutex
	pendingRequests map[string]chan *cadestrov1.ServerMessage

	heartbeatUpdate chan time.Duration

	requireWelcome bool
	welcomed       bool

	invSem         chan struct{}
	luksRevokeSem  chan struct{}
	liveControlSem chan struct{}
}

const (
	inventoryDispatchConcurrency = 2

	luksRevokeDispatchConcurrency  = 2
	liveControlDispatchConcurrency = 1

	maxInboundMessageBytes = 16 << 20
)

// NewClient creates a new SDK client.
func NewClient(serverURL string, opts ...ClientOption) *Client {
	c := &Client{
		logger:         slog.Default(),
		sendSem:        make(chan struct{}, 1),
		invSem:         make(chan struct{}, inventoryDispatchConcurrency),
		luksRevokeSem:  make(chan struct{}, luksRevokeDispatchConcurrency),
		liveControlSem: make(chan struct{}, liveControlDispatchConcurrency),
	}

	httpClient := http.DefaultClient
	for _, opt := range opts {
		opt.apply(c, &httpClient)
	}
	c.httpClient = httpClient

	c.client = cadestrov1connect.NewAgentServiceClient(httpClient, serverURL,
		connect.WithReadMaxBytes(maxInboundMessageBytes))
	return c
}

// CloseIdleConnections releases idle keep-alive connections held by this
// client's transport. The agent calls it when tearing down a connection session
// before reconnecting (WS13 #8): without it, each reconnect builds a fresh
// client whose mTLS transport keeps its own idle-connection pool, leaking
// sockets/file-descriptors across a long-lived reconnect loop. Safe to call on a
// client with no custom transport (http.DefaultClient.Transport) or a nil
// client.
func (c *Client) CloseIdleConnections() {
	if c == nil || c.httpClient == nil {
		return
	}
	c.httpClient.CloseIdleConnections()
}

// ClientOption configures the client.
type ClientOption interface {
	apply(*Client, **http.Client)
}

type funcOption struct {
	f func(*Client, **http.Client)
}

func (fo *funcOption) apply(c *Client, hc **http.Client) {
	fo.f(c, hc)
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(hc *http.Client) ClientOption {
	return &funcOption{func(c *Client, httpClient **http.Client) {
		*httpClient = hc
	}}
}

// WithAuth sets the device ID and auth token.
func WithAuth(deviceID, authToken string) ClientOption {
	return &funcOption{func(c *Client, _ **http.Client) {
		c.deviceID = deviceID
		c.authToken = authToken
	}}
}

// WithLogger sets a custom structured logger for the client.
func WithLogger(l *slog.Logger) ClientOption {
	return &funcOption{func(c *Client, _ **http.Client) {
		c.logger = l
	}}
}

// WithMTLS configures the client to use mTLS authentication.
// certFile and keyFile are the paths to the client certificate and key.
// caFile is the path to the CA certificate for server verification.
func WithMTLS(certFile, keyFile, caFile string) (ClientOption, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load client certificate: %w", err)
	}

	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read CA certificate: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCert) {
		return nil, errors.New("failed to parse CA certificate")
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS13,
	}

	return &funcOption{func(c *Client, httpClient **http.Client) {
		*httpClient = newHTTPClientWithTLS(tlsConfig)
	}}, nil
}

// WithTLSConfig configures the client with a custom TLS configuration.
func WithTLSConfig(tlsConfig *tls.Config) ClientOption {
	return &funcOption{func(c *Client, httpClient **http.Client) {
		*httpClient = newHTTPClientWithTLS(tlsConfig)
	}}
}

// WithMTLSFromPEM configures mTLS using PEM-encoded certificate data.
//
// Trust is strict: the returned TLS config verifies the server ONLY
// against caPEM. This is the correct setup for talking to the
// internal-CA-signed control agent listener over mTLS — system roots
// are NOT consulted, so a cert signed by any public CA cannot
// impersonate control even if its SNI matches.
//
// Callers that deliberately reach a public-CA endpoint may use the separate
// system-roots variant below; the agent's control stream must use this strict
// option.
func WithMTLSFromPEM(certPEM, keyPEM, caPEM []byte) (ClientOption, error) {
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse client certificate: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("failed to parse CA certificate")
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS13,
	}

	return &funcOption{func(c *Client, httpClient **http.Client) {
		*httpClient = newHTTPClientWithTLS(tlsConfig)
	}}, nil
}

// WithMTLSFromPEMAndSystemRoots is like WithMTLSFromPEM but the
// server-verification root pool contains caPEM PLUS the host's
// system roots. Use this when the server sits behind a public CA
// (e.g. a Traefik reverse proxy terminating TLS with Let's Encrypt)
// and the client cert must still authenticate the agent's identity
// at the application layer — for example the
// reusable SDK clients that deliberately reach a public-CA-fronted endpoint.
//
// Do NOT use this for the agent's mTLS stream: control's agent
// listener is internal-CA only, and broadening its trust to system
// roots lets any publicly-trusted cert with a matching SNI
// impersonate it.
func WithMTLSFromPEMAndSystemRoots(certPEM, keyPEM, caPEM []byte) (ClientOption, error) {
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse client certificate: %w", err)
	}

	caPool, err := x509.SystemCertPool()
	if err != nil || caPool == nil {
		caPool = x509.NewCertPool()
	}
	if !caPool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("failed to parse CA certificate")
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS13,
	}

	return &funcOption{func(c *Client, httpClient **http.Client) {
		*httpClient = newHTTPClientWithTLS(tlsConfig)
	}}, nil
}

func newHTTPClientWithTLS(tlsConfig *tls.Config) *http.Client {
	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}
	if err := http2.ConfigureTransport(transport); err != nil {
		slog.Default().Warn("failed to configure HTTP/2 transport; falling back to HTTP/1.1 (bidirectional streaming will not work)", "error", err)
	}
	return &http.Client{Transport: transport}
}

func bootstrapHTTPClient() *http.Client {
	transport := &http.Transport{
		Proxy:           http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13},
	}

	if err := http2.ConfigureTransport(transport); err != nil {
		slog.Default().Warn("bootstrap: failed to configure HTTP/2 transport; falling back to HTTP/1.1", "error", err)
	}
	return &http.Client{
		Timeout:   60 * time.Second,
		Transport: transport,
	}
}

// RegisterAgentResult contains the result of agent registration.
type RegisterAgentResult struct {
	DeviceID    string
	CACert      []byte
	Certificate []byte
	// ControlURL is where the agent dials its AgentService stream — control's
	// agent listener, normally a different host from the API URL registration
	// went to.

	ControlURL string
}

// RegisterAgent registers an agent with the control server.
// This is a standalone function that uses ControlServiceClient (not AgentServiceClient).
// The controlURL is the control server's public API URL (where the web UI
// connects). The result's ControlURL is a DIFFERENT host — control's agent
// listener, which the agent dials for its stream.
func RegisterAgent(ctx context.Context, controlURL string, token, hostname, agentVersion string, csr []byte, opts ...ClientOption) (*RegisterAgentResult, error) {
	c := &Client{}
	httpClient := bootstrapHTTPClient()
	for _, opt := range opts {
		opt.apply(c, &httpClient)
	}

	controlClient := cadestrov1connect.NewControlServiceClient(httpClient, controlURL)

	req := connect.NewRequest(&cadestrov1.RegisterRequest{
		Token:        token,
		Hostname:     hostname,
		AgentVersion: agentVersion,
		Csr:          csr,
	})

	resp, err := controlClient.Register(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("register: %w", err)
	}

	return &RegisterAgentResult{
		DeviceID:    resp.Msg.DeviceId.GetValue(),
		CACert:      resp.Msg.CaCert,
		Certificate: resp.Msg.Certificate,
		ControlURL:  resp.Msg.ControlUrl,
	}, nil
}

// RenewCertificateResult contains the result of certificate renewal.
type RenewCertificateResult struct {
	Certificate []byte
	NotAfter    time.Time
}

// RenewCertificate renews a device certificate via the control server.
// The mTLS transport presents the authenticated certificate identity.
func RenewCertificate(ctx context.Context, controlURL string, csr []byte, opts ...ClientOption) (*RenewCertificateResult, error) {
	c := &Client{}
	httpClient := bootstrapHTTPClient()
	for _, opt := range opts {
		opt.apply(c, &httpClient)
	}

	controlClient := cadestrov1connect.NewControlServiceClient(httpClient, controlURL)

	req := connect.NewRequest(&cadestrov1.RenewCertificateRequest{
		Csr: csr,
	})

	resp, err := controlClient.RenewCertificate(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("renew certificate: %w", err)
	}

	return &RenewCertificateResult{
		Certificate: resp.Msg.Certificate,
		NotAfter:    resp.Msg.NotAfter.AsTime(),
	}, nil
}

// StreamHandler handles messages received from the server.
type StreamHandler interface {
	// OnWelcome is called when the server sends a welcome message.

	OnWelcome(ctx context.Context, welcome *cadestrov1.Welcome) error
	// OnQuery is called when the server sends an OS query.

	OnQuery(ctx context.Context, query *cadestrov1.OSQuery) (*cadestrov1.OSQueryResult, error)
	// OnError is called when the server sends an error.

	OnError(ctx context.Context, err *cadestrov1.Error) error
}

// LiveControlHandler handles correlated control operations over the agent stream.
type LiveControlHandler interface {
	OnSyncDevice(context.Context, *cadestrov1.SyncDeviceCommand) error
	OnRebootDevice(context.Context, *cadestrov1.RebootDeviceCommand) error
}

// LuksHandler extends StreamHandler with LUKS device-key revocation support.
// Handlers that implement this interface will receive revoke requests from the server.
type LuksHandler interface {
	StreamHandler
	// OnRevokeLuksDeviceKey is called when control requests revocation of a
	// LUKS device-bound key. The full message is delivered rather than the
	// bare action_id so the handler keeps whatever context later fields add.
	// Returns (success, errorMessage).

	OnRevokeLuksDeviceKey(ctx context.Context, req *cadestrov1.RevokeLuksDeviceKey) (bool, string)
}

// LogQueryHandler extends StreamHandler with remote log query support.
// Handlers that implement this interface can execute journalctl queries on the device.
type LogQueryHandler interface {
	StreamHandler
	// OnLogQuery is called when the server sends a log query request.

	OnLogQuery(ctx context.Context, query *cadestrov1.LogQuery) (*cadestrov1.LogQueryResult, error)
}

// InventoryHandler extends StreamHandler with device inventory collection support.
// Handlers that implement this interface can collect and send hardware/software inventory.
type InventoryHandler interface {
	StreamHandler
	// CollectInventory gathers hardware/software inventory from the device on
	// the agent's OWN schedule (on connect + every 24h). Returns nil if
	// collection is unavailable (e.g. osquery not installed).

	CollectInventory(ctx context.Context) *cadestrov1.DeviceInventory
	// OnRequestInventory handles a control-originated RequestInventory,
	// collecting the same inventory on demand and correlating it with the
	// request's query_id. Returns nil when collection is unavailable.

	OnRequestInventory(ctx context.Context, req *cadestrov1.RequestInventory) *cadestrov1.DeviceInventory
}

// TerminalHandler extends StreamHandler with remote terminal (PTY) session
// support. Handlers that implement this interface receive the four
// server-initiated session control messages from archived sdk#16
// and are responsible for allocating PTYs, relaying I/O, and reporting
// state back via Client.SendTerminalOutput / Client.SendTerminalStateChange.
//
// All four methods MUST return promptly: the SDK invokes them on the
// receive loop, so a slow handler will stall delivery of every other
// ServerMessage variant. Implementations should hand off to a per-session
// goroutine for any blocking I/O.
//
// A nil error from these methods means the request was accepted; the
// handler is expected to surface terminal-level failures via
// SendTerminalStateChange with a TERMINAL_SESSION_STATE_ERROR payload.
// Returning a non-nil error from OnTerminalStart/Input/Resize/Stop is
// treated as a fatal stream error and tears down the agent connection.
type TerminalHandler interface {
	StreamHandler
	// OnTerminalStart is called when the server requests a new PTY.
	// The handler should validate tty_user, allocate the PTY, kick off
	// I/O goroutines, and send a TERMINAL_SESSION_STATE_STARTED state
	// change. If allocation fails, it MUST send a STATE_ERROR instead.

	OnTerminalStart(ctx context.Context, req *cadestrov1.TerminalStart) error
	// OnTerminalInput is called for every stdin frame from the server.
	// The handler should write the bytes to the PTY of the matching
	// session_id and ignore (with a debug log) frames for unknown
	// sessions.

	OnTerminalInput(ctx context.Context, req *cadestrov1.TerminalInput) error
	// OnTerminalResize forwards a TIOCSWINSZ to the session's PTY.
	// Unknown sessions are ignored.

	OnTerminalResize(ctx context.Context, req *cadestrov1.TerminalResize) error
	// OnTerminalStop terminates the session and reverts any side effects
	// (shell unmask, temp home cleanup, etc.). Unknown sessions are
	// idempotent no-ops so the server can fire and forget on disconnect.

	OnTerminalStop(ctx context.Context, req *cadestrov1.TerminalStop) error
}

// Connect establishes a bidirectional stream with the server.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	if c.stream != nil {
		c.mu.Unlock()
		return errors.New("already connected")
	}

	stream := c.client.Stream(ctx)
	c.stream = stream
	c.mu.Unlock()

	return nil
}

func (c *Client) send(ctx context.Context, msg *cadestrov1.AgentMessage) error {
	c.mu.RLock()
	stream := c.stream
	c.mu.RUnlock()

	if stream == nil {
		return errors.New("not connected")
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	select {
	case c.sendSem <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}

	errCh := make(chan error, 1)
	go func() {
		err := stream.Send(msg)
		<-c.sendSem
		errCh <- err
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SendHello sends a hello message to the server.
func (c *Client) SendHello(ctx context.Context, hostname, agentVersion string) error {
	c.mu.RLock()
	deviceID := c.deviceID
	authToken := c.authToken
	c.mu.RUnlock()

	return c.send(ctx, &cadestrov1.AgentMessage{
		Id: &cadestrov1.MessageId{Value: NewULID()},
		Payload: &cadestrov1.AgentMessage_Hello{
			Hello: &cadestrov1.Hello{
				DeviceId:     &cadestrov1.DeviceId{Value: deviceID},
				AgentVersion: agentVersion,
				Hostname:     hostname,
				AuthToken:    authToken,
				Arch:         runtime.GOARCH,
			},
		},
	})
}

// SendHeartbeat sends a heartbeat message to the server.
func (c *Client) SendHeartbeat(ctx context.Context, hb *cadestrov1.Heartbeat) error {
	return c.send(ctx, &cadestrov1.AgentMessage{
		Id: &cadestrov1.MessageId{Value: NewULID()},
		Payload: &cadestrov1.AgentMessage_Heartbeat{
			Heartbeat: hb,
		},
	})
}

// SendActionResult reports the outcome of one occurrence. The result must
// carry the run_id and occurrence_id it descends from; control keys
// ingestion on that pair, so a result replayed after a reconnect updates the
// same row instead of creating a second one.
func (c *Client) SendActionResult(ctx context.Context, result *cadestrov1.ActionResult) error {
	message := &cadestrov1.AgentMessage{Id: &cadestrov1.MessageId{Value: NewULID()}, Payload: &cadestrov1.AgentMessage_ActionResult{ActionResult: result}}
	if result == nil || result.GetRunId().GetValue() == "" || result.GetOccurrenceId().GetValue() == "" {
		return c.send(ctx, message)
	}
	return c.sendResultAwaitAck(ctx, message)
}

// SendManifestResult reports the outcome of a complete manifest, once, after
// its occurrences have reported individually.
func (c *Client) SendManifestResult(ctx context.Context, result *cadestrov1.ManifestResult) error {
	message := &cadestrov1.AgentMessage{Id: &cadestrov1.MessageId{Value: NewULID()}, Payload: &cadestrov1.AgentMessage_ManifestResult{ManifestResult: result}}
	if result == nil || result.GetRunId().GetValue() == "" || result.GetManifestId().GetValue() == "" {
		return c.send(ctx, message)
	}
	return c.sendResultAwaitAck(ctx, message)
}

func (c *Client) sendResultAwaitAck(ctx context.Context, message *cadestrov1.AgentMessage) error {
	id := message.GetId().GetValue()
	ch := c.registerPending(id)
	defer c.unregisterPending(id)
	if err := c.send(ctx, message); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case response, ok := <-ch:
		if !ok || response == nil {
			return errors.New("stream closed while waiting for result acknowledgement")
		}
		ack := response.GetResultAck()
		if ack == nil || !ack.GetAccepted() {
			if ack == nil {
				return errors.New("server returned no result acknowledgement")
			}
			return fmt.Errorf("result rejected: %s", ack.GetCode())
		}
		return nil
	}
}

// SendQueryResult sends an OS query result to the server.
func (c *Client) SendQueryResult(ctx context.Context, result *cadestrov1.OSQueryResult) error {
	return c.send(ctx, &cadestrov1.AgentMessage{
		Id: &cadestrov1.MessageId{Value: NewULID()},
		Payload: &cadestrov1.AgentMessage_QueryResult{
			QueryResult: result,
		},
	})
}

// SendLogQueryResult sends a log query result to the server.
func (c *Client) SendLogQueryResult(ctx context.Context, result *cadestrov1.LogQueryResult) error {
	return c.send(ctx, &cadestrov1.AgentMessage{
		Id: &cadestrov1.MessageId{Value: NewULID()},
		Payload: &cadestrov1.AgentMessage_LogQueryResult{
			LogQueryResult: result,
		},
	})
}

// SendSecurityAlert sends a security alert to the server for audit logging.
func (c *Client) SendSecurityAlert(ctx context.Context, alert *cadestrov1.SecurityAlert) error {
	return c.send(ctx, &cadestrov1.AgentMessage{
		Id: &cadestrov1.MessageId{Value: NewULID()},
		Payload: &cadestrov1.AgentMessage_SecurityAlert{
			SecurityAlert: alert,
		},
	})
}

// SendInventory sends device inventory to the server.
func (c *Client) SendInventory(ctx context.Context, inventory *cadestrov1.DeviceInventory) error {
	if inventory == nil {
		return nil
	}

	return c.send(ctx, &cadestrov1.AgentMessage{
		Id: &cadestrov1.MessageId{Value: NewULID()},
		Payload: &cadestrov1.AgentMessage_Inventory{
			Inventory: inventory,
		},
	})
}

func (c *Client) sendSyncDeviceResult(ctx context.Context, id string, operationErr error) error {
	result := &cadestrov1.SyncDeviceResult{Success: operationErr == nil}
	return c.send(ctx, &cadestrov1.AgentMessage{Id: &cadestrov1.MessageId{Value: id}, Payload: &cadestrov1.AgentMessage_SyncDeviceResult{SyncDeviceResult: result}})
}

func (c *Client) sendRebootDeviceResult(ctx context.Context, id string, operationErr error) error {
	result := &cadestrov1.RebootDeviceResult{Success: operationErr == nil}
	return c.send(ctx, &cadestrov1.AgentMessage{Id: &cadestrov1.MessageId{Value: id}, Payload: &cadestrov1.AgentMessage_RebootDeviceResult{RebootDeviceResult: result}})
}

// SendTerminalOutput sends a stdout/stderr chunk from a remote terminal
// session back to the server. The TerminalHandler is responsible for
// chunking PTY reads to fit the proto's 64KB max data size.
func (c *Client) SendTerminalOutput(ctx context.Context, out *cadestrov1.TerminalOutput) error {
	return c.send(ctx, &cadestrov1.AgentMessage{
		Id: &cadestrov1.MessageId{Value: NewULID()},
		Payload: &cadestrov1.AgentMessage_TerminalOutput{
			TerminalOutput: out,
		},
	})
}

// SendTerminalStateChange reports a terminal session lifecycle event
// (started, exited with code, error). Send STARTED immediately after
// the PTY is allocated, EXITED when the shell process exits cleanly,
// and ERROR for any failure that ends the session before STARTED or
// in flight.
func (c *Client) SendTerminalStateChange(ctx context.Context, change *cadestrov1.TerminalStateChange) error {
	return c.send(ctx, &cadestrov1.AgentMessage{
		Id: &cadestrov1.MessageId{Value: NewULID()},
		Payload: &cadestrov1.AgentMessage_TerminalStateChange{
			TerminalStateChange: change,
		},
	})
}

// GetLuksKey sends a GetLuksKeyRequest on the stream and waits for the
// correlated response, matched by message ID.
//
// The returned passphrase is plaintext inside the authenticated mTLS stream.
// The caller should keep its lifetime narrow and clear its copy after use.
func (c *Client) GetLuksKey(ctx context.Context, actionID string) ([]byte, error) {
	id := NewULID()
	ch := c.registerPending(id)
	defer c.unregisterPending(id)

	if err := c.send(ctx, &cadestrov1.AgentMessage{
		Id: &cadestrov1.MessageId{Value: id},
		Payload: &cadestrov1.AgentMessage_GetLuksKey{
			GetLuksKey: &cadestrov1.GetLuksKeyRequest{
				ActionId: &cadestrov1.ActionId{Value: actionID},
			},
		},
	}); err != nil {
		return nil, fmt.Errorf("send get luks key request: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp, ok := <-ch:
		if !ok || resp == nil {
			return nil, errors.New("stream closed while waiting for GetLuksKey response")
		}
		if errMsg := resp.GetError(); errMsg != nil {
			return nil, fmt.Errorf("server error: %s", errMsg.Message)
		}
		luksResp := resp.GetGetLuksKey()
		if luksResp == nil {
			return nil, errors.New("unexpected response type")
		}

		if err := c.validateInbound(luksResp); err != nil {
			return nil, fmt.Errorf("invalid GetLuksKey response: %w", err)
		}
		return luksResp.Passphrase, nil
	}
}

// StoreLuksKey sends a StoreLuksKeyRequest on the stream and waits for the
// server confirmation.
//
// passphrase is plaintext inside the authenticated mTLS stream. Control derives
// the device identity from that stream and encrypts the value before storage.
func (c *Client) StoreLuksKey(ctx context.Context, actionID, devicePath string, passphrase []byte, reason cadestrov1.RotationReason) error {
	id := NewULID()
	ch := c.registerPending(id)
	defer c.unregisterPending(id)

	if err := c.send(ctx, &cadestrov1.AgentMessage{
		Id: &cadestrov1.MessageId{Value: id},
		Payload: &cadestrov1.AgentMessage_StoreLuksKey{
			StoreLuksKey: &cadestrov1.StoreLuksKeyRequest{
				ActionId:       &cadestrov1.ActionId{Value: actionID},
				DevicePath:     devicePath,
				Passphrase:     passphrase,
				RotationReason: reason,
			},
		},
	}); err != nil {
		return fmt.Errorf("send store luks key request: %w", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case resp, ok := <-ch:
		if !ok || resp == nil {
			return errors.New("stream closed while waiting for StoreLuksKey response")
		}
		if errMsg := resp.GetError(); errMsg != nil {
			return fmt.Errorf("server error: %s", errMsg.Message)
		}
		storeResp := resp.GetStoreLuksKey()
		if storeResp == nil {
			return errors.New("unexpected response type")
		}
		if !storeResp.Success {
			return errors.New("server rejected key storage")
		}
		return nil
	}
}

// StoreLpsPasswords reports one LPS execution's password rotations and waits for
// the server confirmation.
//
// Each rotation's password is plaintext inside the authenticated mTLS stream.
// Control derives the device identity from the stream and binds the stored
// ciphertext to its row, device, kind, subject and version.
//
// Request/response are correlated by message id like every other stream call, so
// a failed batch is reported rather than silently dropped: LPS rotations are
// unrecoverable if lost — the agent has already changed the local password.
func (c *Client) StoreLpsPasswords(ctx context.Context, actionID string, rotations []*cadestrov1.LpsPasswordRotation) error {
	if len(rotations) == 0 {
		return errors.New("refusing to send an empty LPS rotation batch")
	}

	id := NewULID()
	ch := c.registerPending(id)
	defer c.unregisterPending(id)

	if err := c.send(ctx, &cadestrov1.AgentMessage{
		Id: &cadestrov1.MessageId{Value: id},
		Payload: &cadestrov1.AgentMessage_StoreLpsPasswords{
			StoreLpsPasswords: &cadestrov1.StoreLpsPasswordsRequest{
				ActionId:  &cadestrov1.ActionId{Value: actionID},
				Rotations: rotations,
			},
		},
	}); err != nil {
		return fmt.Errorf("send store lps passwords request: %w", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case resp, ok := <-ch:
		if !ok || resp == nil {
			return errors.New("stream closed while waiting for StoreLpsPasswords response")
		}
		if errMsg := resp.GetError(); errMsg != nil {
			return fmt.Errorf("server error: %s", errMsg.Message)
		}
		storeResp := resp.GetStoreLpsPasswords()
		if storeResp == nil {
			return errors.New("unexpected response type")
		}
		if !storeResp.Success {
			return errors.New("server rejected LPS password storage")
		}
		return nil
	}
}

// SendRevokeLuksDeviceKeyResult sends the result of a LUKS device key revocation back to the server.
func (c *Client) SendRevokeLuksDeviceKeyResult(ctx context.Context, actionID string, success bool, errMsg string) error {
	return c.send(ctx, &cadestrov1.AgentMessage{
		Id: &cadestrov1.MessageId{Value: NewULID()},
		Payload: &cadestrov1.AgentMessage_RevokeLuksDeviceKeyResult{
			RevokeLuksDeviceKeyResult: &cadestrov1.RevokeLuksDeviceKeyResult{
				ActionId: &cadestrov1.ActionId{Value: actionID},
				Success:  success,
				Error:    errMsg,
			},
		},
	})
}

func (c *Client) registerPending(id string) chan *cadestrov1.ServerMessage {
	ch := make(chan *cadestrov1.ServerMessage, 1)
	c.pendingMu.Lock()
	if c.pendingRequests == nil {
		c.pendingRequests = make(map[string]chan *cadestrov1.ServerMessage)
	}
	c.pendingRequests[id] = ch
	c.pendingMu.Unlock()
	return ch
}

func (c *Client) unregisterPending(id string) {
	c.pendingMu.Lock()
	delete(c.pendingRequests, id)
	c.pendingMu.Unlock()
}

func (c *Client) deliverPending(msg *cadestrov1.ServerMessage) bool {
	c.pendingMu.Lock()
	ch, ok := c.pendingRequests[msg.GetId().GetValue()]
	c.pendingMu.Unlock()
	if ok {
		select {
		case ch <- msg:
		default:
			c.logger.Warn("deliverPending: dropping duplicate response", "id", msg.GetId().GetValue())
		}
	}
	return ok
}

// Receive receives the next message from the server.
func (c *Client) Receive(ctx context.Context) (*cadestrov1.ServerMessage, error) {
	c.mu.RLock()
	stream := c.stream
	c.mu.RUnlock()

	if stream == nil {
		return nil, errors.New("not connected")
	}

	msg, err := stream.Receive()
	if err != nil {
		return nil, err
	}

	return msg, nil
}

// Close closes the stream connection and cancels every pending request.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stream == nil {
		return nil
	}

	c.pendingMu.Lock()
	for id, ch := range c.pendingRequests {
		close(ch)
		delete(c.pendingRequests, id)
	}
	c.pendingMu.Unlock()

	_ = c.stream.CloseRequest()
	_ = c.stream.CloseResponse()
	c.stream = nil
	c.requireWelcome = false
	c.welcomed = false
	return nil
}

// StartReceiver starts a background goroutine that receives stream messages and
// delivers them to pending correlated request channels.
// Returns a cancel function to stop the receiver. This is useful for CLI tools that
// need request-response correlation without the full Run() loop.
// The caller must call Connect() and SendHello() before calling this.
func (c *Client) StartReceiver(ctx context.Context) context.CancelFunc {
	rctx, cancel := context.WithCancel(ctx)
	go func() {
		for {
			msg, err := c.Receive(rctx)
			if err != nil {
				return
			}
			c.deliverPending(msg)
		}
	}()
	return cancel
}

// Run connects to the server and processes messages using the provided handler.
//
// heartbeatInterval is the initial cadence used until the server's
// Welcome message arrives. If Welcome.heartbeat_interval is set and
// falls within [MinHeartbeatInterval, MaxHeartbeatInterval], the SDK
// resets the heartbeat ticker to that value — both on the initial
// connect and on every subsequent reconnect (each reconnect is a fresh
// Run() call that receives a fresh Welcome). Out-of-range values are
// clamped; zero / unset keeps the caller-supplied interval.
func (c *Client) Run(ctx context.Context, hostname, agentVersion string, heartbeatInterval time.Duration, handler StreamHandler) error {
	if err := c.Connect(ctx); err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	heartbeatInterval = normalizeHeartbeatInterval(heartbeatInterval)
	c.mu.Lock()
	c.requireWelcome = true
	c.welcomed = false
	c.mu.Unlock()

	if err := c.SendHello(ctx, hostname, agentVersion); err != nil {
		return fmt.Errorf("send hello: %w", err)
	}

	heartbeatCtx, cancelHeartbeat := context.WithCancel(ctx)
	defer cancelHeartbeat()

	hbUpdate := make(chan time.Duration, 1)
	c.mu.Lock()
	c.heartbeatUpdate = hbUpdate
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.heartbeatUpdate = nil
		c.mu.Unlock()
	}()

	heartbeatErr := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()

		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case d := <-hbUpdate:
				ticker.Reset(d)
			case <-ticker.C:
				hb := &cadestrov1.Heartbeat{}

				if err := c.SendHeartbeat(heartbeatCtx, hb); err != nil {
					heartbeatErr <- err
					return
				}
			}
		}
	}()

	if invHandler, ok := handler.(InventoryHandler); ok {
		c.safeGo("inventory-ticker", func() {

			sendWithRetry := func(inv *cadestrov1.DeviceInventory) {
				const maxAttempts = 3
				delay := time.Second
				for attempt := 1; attempt <= maxAttempts; attempt++ {
					err := c.SendInventory(heartbeatCtx, inv)
					if err == nil {
						return
					}
					if attempt == maxAttempts || heartbeatCtx.Err() != nil {
						c.logger.Warn("failed to send inventory", "error", err, "attempts", attempt)
						return
					}
					select {
					case <-heartbeatCtx.Done():
						return
					case <-time.After(delay):
					}
					delay *= 3
				}
			}

			if inv := invHandler.CollectInventory(heartbeatCtx); inv != nil {
				sendWithRetry(inv)
			}

			ticker := time.NewTicker(24 * time.Hour)
			defer ticker.Stop()

			for {
				select {
				case <-heartbeatCtx.Done():
					return
				case <-ticker.C:
					if inv := invHandler.CollectInventory(heartbeatCtx); inv != nil {
						sendWithRetry(inv)
					}
				}
			}
		})
	}

	type receiveResult struct {
		msg *cadestrov1.ServerMessage
		err error
	}
	msgCh := make(chan receiveResult, 1)

	go func() {
		for {
			msg, err := c.Receive(ctx)
			select {
			case msgCh <- receiveResult{msg, err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-heartbeatErr:
			return fmt.Errorf("heartbeat: %w", err)
		case result := <-msgCh:
			if result.err != nil {
				return fmt.Errorf("receive: %w", result.err)
			}
			if err := c.dispatchServerMessage(ctx, result.msg, handler); err != nil {
				return err
			}
		}
	}
}

func normalizeHeartbeatInterval(interval time.Duration) time.Duration {
	if interval <= 0 {
		return MinHeartbeatInterval
	}
	return interval
}

func (c *Client) applyWelcomeHeartbeat(w *cadestrov1.Welcome) {
	if w == nil || w.HeartbeatInterval == nil {
		return
	}
	d := w.HeartbeatInterval.AsDuration()
	if d <= 0 {
		return
	}
	if d < MinHeartbeatInterval {
		d = MinHeartbeatInterval
	}
	if d > MaxHeartbeatInterval {
		d = MaxHeartbeatInterval
	}
	c.mu.RLock()
	ch := c.heartbeatUpdate
	c.mu.RUnlock()
	if ch == nil {
		return
	}

	select {
	case <-ch:
	default:
	}
	select {
	case ch <- d:
	default:
	}
}

func (c *Client) safeGo(label string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				c.logger.Error("recovered panic in stream dispatch goroutine",
					"leg", label, "panic", fmt.Sprintf("%v", r))
			}
		}()
		fn()
	}()
}

func (c *Client) validateInbound(payload any) error {
	msg, ok := payload.(proto.Message)
	if !ok {
		return fmt.Errorf("inbound payload is not a proto.Message: %T", payload)
	}
	return protovalidate.Validate(msg)
}

func (c *Client) validateServerMessage(msg *cadestrov1.ServerMessage) error {
	c.mu.RLock()
	strict := c.requireWelcome
	c.mu.RUnlock()
	if !strict {
		return nil
	}
	if msg == nil {
		return errors.New("ServerMessage is required")
	}
	if msg.Payload == nil {
		return errors.New("ServerMessage payload is required")
	}
	payload := reflect.ValueOf(msg.Payload)
	if payload.Kind() == reflect.Ptr && payload.IsNil() {
		return errors.New("ServerMessage payload is required")
	}
	if payload.Kind() == reflect.Ptr && payload.Elem().Kind() == reflect.Struct {
		field := payload.Elem().Field(0)
		if field.Kind() == reflect.Ptr && field.IsNil() {
			return errors.New("ServerMessage payload is required")
		}
	}
	return c.validateInbound(msg)
}

func correlatedResponsePayload(msg *cadestrov1.ServerMessage) any {
	switch p := msg.Payload.(type) {
	case *cadestrov1.ServerMessage_SyncState:
		return p.SyncState
	case *cadestrov1.ServerMessage_GetLuksKey:
		return p.GetLuksKey
	case *cadestrov1.ServerMessage_StoreLuksKey:
		return p.StoreLuksKey
	case *cadestrov1.ServerMessage_StoreLpsPasswords:
		return p.StoreLpsPasswords
	case *cadestrov1.ServerMessage_ValidateLuksToken:
		return p.ValidateLuksToken
	case *cadestrov1.ServerMessage_ResultAck:
		return p.ResultAck
	default:
		return nil
	}
}

func (c *Client) dispatchServerMessage(ctx context.Context, msg *cadestrov1.ServerMessage, handler StreamHandler) (retErr error) {
	defer func() {
		if r := recover(); r != nil {
			var payloadType string
			if msg != nil {
				payloadType = fmt.Sprintf("%T", msg.Payload)
			}
			var msgID string
			if msg != nil {
				msgID = msg.GetId().GetValue()
			}
			c.logger.Error("recovered panic while dispatching ServerMessage; dropping frame (non-fatal)",
				"message_id", msgID, "payload_type", payloadType, "panic", fmt.Sprintf("%v", r))

			retErr = nil
		}
	}()
	if err := c.validateServerMessage(msg); err != nil {
		return fmt.Errorf("invalid ServerMessage: %w", err)
	}
	c.mu.Lock()
	if c.requireWelcome && !c.welcomed {
		if _, ok := msg.Payload.(*cadestrov1.ServerMessage_Welcome); !ok {
			c.mu.Unlock()
			return errors.New("first server message must be Welcome")
		}
	}
	c.mu.Unlock()
	switch p := msg.Payload.(type) {
	case *cadestrov1.ServerMessage_SyncDevice:
		if p.SyncDevice == nil {
			return nil
		}
		select {
		case c.liveControlSem <- struct{}{}:
			c.safeGo("live-sync", func() {
				defer func() { <-c.liveControlSem }()
				operationErr := errors.New("live sync is unsupported")
				if h, ok := handler.(LiveControlHandler); ok {
					operationErr = h.OnSyncDevice(ctx, p.SyncDevice)
				}
				if err := c.sendSyncDeviceResult(ctx, msg.GetId().GetValue(), operationErr); err != nil {
					c.logger.Warn("failed to send live sync result", "error", err)
				}
			})
			return nil
		default:
			return c.sendSyncDeviceResult(ctx, msg.GetId().GetValue(), errors.New("another live operation is already running"))
		}
	case *cadestrov1.ServerMessage_RebootDevice:
		if p.RebootDevice == nil {
			return nil
		}
		select {
		case c.liveControlSem <- struct{}{}:
			c.safeGo("live-reboot", func() {
				defer func() { <-c.liveControlSem }()
				operationErr := errors.New("live reboot is unsupported")
				if h, ok := handler.(LiveControlHandler); ok {
					operationErr = h.OnRebootDevice(ctx, p.RebootDevice)
				}
				if err := c.sendRebootDeviceResult(ctx, msg.GetId().GetValue(), operationErr); err != nil {
					c.logger.Warn("failed to send live reboot result", "error", err)
				}
			})
			return nil
		default:
			return c.sendRebootDeviceResult(ctx, msg.GetId().GetValue(), errors.New("another live operation is already running"))
		}
	case *cadestrov1.ServerMessage_Welcome:
		if p.Welcome == nil {
			c.logger.Warn("dropping Welcome with nil payload", "message_id", msg.GetId().GetValue())
			return nil
		}
		c.mu.Lock()
		c.welcomed = true
		c.mu.Unlock()
		c.applyWelcomeHeartbeat(p.Welcome)
		if err := handler.OnWelcome(ctx, p.Welcome); err != nil {
			return fmt.Errorf("handle welcome: %w", err)
		}

	case *cadestrov1.ServerMessage_Query:
		if p.Query == nil {
			c.logger.Warn("dropping Query with nil payload", "message_id", msg.GetId().GetValue())
			return nil
		}
		if err := c.validateInbound(p.Query); err != nil {
			c.logger.Warn("dropping invalid Query", "message_id", msg.GetId().GetValue(), "error", err)
			return nil
		}
		queryResult, err := handler.OnQuery(ctx, p.Query)
		if err != nil {
			return fmt.Errorf("handle query: %w", err)
		}
		if queryResult != nil {
			if err := c.SendQueryResult(ctx, queryResult); err != nil {
				return fmt.Errorf("send query result: %w", err)
			}
		}

	case *cadestrov1.ServerMessage_Error:
		if p.Error == nil {
			c.logger.Warn("dropping Error with nil payload", "message_id", msg.GetId().GetValue())
			return nil
		}
		if err := c.validateInbound(p.Error); err != nil {
			return fmt.Errorf("invalid Error response: %w", err)
		}

		if c.deliverPending(msg) {
			return nil
		}

		if err := handler.OnError(ctx, p.Error); err != nil {
			return fmt.Errorf("handle error: %w", err)
		}

	case *cadestrov1.ServerMessage_SyncState,
		*cadestrov1.ServerMessage_GetLuksKey,
		*cadestrov1.ServerMessage_StoreLuksKey,
		*cadestrov1.ServerMessage_StoreLpsPasswords,
		*cadestrov1.ServerMessage_ValidateLuksToken,
		*cadestrov1.ServerMessage_ResultAck:
		if err := c.validateInbound(correlatedResponsePayload(msg)); err != nil {
			return fmt.Errorf("invalid correlated response: %w", err)
		}

		if !c.deliverPending(msg) {
			c.logger.Debug("dropping correlated response without waiter", "message_id", msg.GetId().GetValue())
		}

	case *cadestrov1.ServerMessage_RequestInventory:
		if p.RequestInventory == nil {
			c.logger.Warn("dropping RequestInventory with nil payload", "message_id", msg.GetId().GetValue())
			return nil
		}
		if err := c.validateInbound(p.RequestInventory); err != nil {
			c.logger.Warn("dropping invalid RequestInventory", "message_id", msg.GetId().GetValue(), "error", err)
			return nil
		}
		if invHandler, ok := handler.(InventoryHandler); ok {
			req := p.RequestInventory

			select {
			case c.invSem <- struct{}{}:

				c.safeGo("inventory", func() {
					defer func() { <-c.invSem }()

					if inv := invHandler.OnRequestInventory(ctx, req); inv != nil {
						if err := c.SendInventory(ctx, inv); err != nil {
							c.logger.Warn("failed to send inventory", "error", err)
						}
					}
				})
			default:
				c.logger.Warn("dropping RequestInventory: inventory collection already at capacity",
					"message_id", msg.GetId().GetValue(), "limit", inventoryDispatchConcurrency)
			}
		}

	case *cadestrov1.ServerMessage_LogQuery:
		if p.LogQuery == nil {
			c.logger.Warn("dropping LogQuery with nil payload", "message_id", msg.GetId().GetValue())
			return nil
		}
		if err := c.validateInbound(p.LogQuery); err != nil {
			c.logger.Warn("dropping invalid LogQuery", "message_id", msg.GetId().GetValue(), "error", err)
			return nil
		}
		if lqHandler, ok := handler.(LogQueryHandler); ok {
			result, err := lqHandler.OnLogQuery(ctx, p.LogQuery)
			if err != nil {
				return fmt.Errorf("handle log query: %w", err)
			}
			if result != nil {
				if err := c.SendLogQueryResult(ctx, result); err != nil {
					return fmt.Errorf("send log query result: %w", err)
				}
			}
		}

	case *cadestrov1.ServerMessage_RevokeLuksDeviceKey:
		if p.RevokeLuksDeviceKey == nil {

			c.logger.Warn("dropping RevokeLuksDeviceKey with nil payload", "message_id", msg.GetId().GetValue())
			return nil
		}

		if err := c.validateInbound(p.RevokeLuksDeviceKey); err != nil {
			c.logger.Warn("dropping invalid RevokeLuksDeviceKey", "message_id", msg.GetId().GetValue(), "error", err)
			return nil
		}
		if luksHandler, ok := handler.(LuksHandler); ok {
			req := p.RevokeLuksDeviceKey
			actionID := req.GetActionId().GetValue()

			select {
			case c.luksRevokeSem <- struct{}{}:

				c.safeGo("luks-revoke", func() {
					defer func() { <-c.luksRevokeSem }()

					success, errMsg := luksHandler.OnRevokeLuksDeviceKey(ctx, req)
					if err := c.SendRevokeLuksDeviceKeyResult(ctx, actionID, success, errMsg); err != nil {
						c.logger.Warn("failed to send LUKS revocation result", "action_id", actionID, "error", err)
					}
				})
			default:
				c.logger.Warn("dropping RevokeLuksDeviceKey: revocation already at capacity",
					"message_id", msg.GetId().GetValue(), "action_id", actionID, "limit", luksRevokeDispatchConcurrency)
			}
		}

	case *cadestrov1.ServerMessage_TerminalStart:
		if p.TerminalStart == nil {
			c.logger.Warn("dropping TerminalStart with nil payload", "message_id", msg.GetId().GetValue())
			return nil
		}
		if err := c.validateInbound(p.TerminalStart); err != nil {
			c.logger.Warn("dropping invalid TerminalStart", "message_id", msg.GetId().GetValue(), "error", err)
			return nil
		}
		if termHandler, ok := handler.(TerminalHandler); ok {
			if err := termHandler.OnTerminalStart(ctx, p.TerminalStart); err != nil {
				return fmt.Errorf("handle terminal start: %w", err)
			}
		} else {
			c.logger.Debug("dropping TerminalStart: handler does not implement TerminalHandler",
				"session_id", p.TerminalStart.GetSessionId().GetValue())
		}

	case *cadestrov1.ServerMessage_TerminalInput:
		if p.TerminalInput == nil {
			c.logger.Warn("dropping TerminalInput with nil payload", "message_id", msg.GetId().GetValue())
			return nil
		}
		if err := c.validateInbound(p.TerminalInput); err != nil {
			c.logger.Warn("dropping invalid TerminalInput", "message_id", msg.GetId().GetValue(), "error", err)
			return nil
		}
		if termHandler, ok := handler.(TerminalHandler); ok {
			if err := termHandler.OnTerminalInput(ctx, p.TerminalInput); err != nil {
				return fmt.Errorf("handle terminal input: %w", err)
			}
		}

	case *cadestrov1.ServerMessage_TerminalResize:
		if p.TerminalResize == nil {
			c.logger.Warn("dropping TerminalResize with nil payload", "message_id", msg.GetId().GetValue())
			return nil
		}
		if err := c.validateInbound(p.TerminalResize); err != nil {
			c.logger.Warn("dropping invalid TerminalResize", "message_id", msg.GetId().GetValue(), "error", err)
			return nil
		}
		if termHandler, ok := handler.(TerminalHandler); ok {
			if err := termHandler.OnTerminalResize(ctx, p.TerminalResize); err != nil {
				return fmt.Errorf("handle terminal resize: %w", err)
			}
		}

	case *cadestrov1.ServerMessage_TerminalStop:
		if p.TerminalStop == nil {
			c.logger.Warn("dropping TerminalStop with nil payload", "message_id", msg.GetId().GetValue())
			return nil
		}
		if err := c.validateInbound(p.TerminalStop); err != nil {
			c.logger.Warn("dropping invalid TerminalStop", "message_id", msg.GetId().GetValue(), "error", err)
			return nil
		}
		if termHandler, ok := handler.(TerminalHandler); ok {
			if err := termHandler.OnTerminalStop(ctx, p.TerminalStop); err != nil {
				return fmt.Errorf("handle terminal stop: %w", err)
			}
		}

	default:

		c.logger.Debug("dropping unknown ServerMessage payload",
			"message_id", msg.GetId().GetValue(), "type", fmt.Sprintf("%T", msg.Payload))
	}
	return nil
}

// NewULID generates a new ULID string.
func NewULID() string {
	return ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader).String()
}

// DeviceID returns the current device ID.
func (c *Client) DeviceID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.deviceID
}

// AuthToken returns the current auth token.
func (c *Client) AuthToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.authToken
}

// ValidateLuksTokenResult contains the result of a LUKS token validation.
type ValidateLuksTokenResult struct {
	ActionID   string
	DevicePath string
	MinLength  int32
	Complexity cadestrov1.LpsPasswordComplexity
}

// ValidateLuksToken validates and atomically consumes a one-time LUKS token on
// the existing authenticated agent stream.
func (c *Client) ValidateLuksToken(ctx context.Context, token string) (*ValidateLuksTokenResult, error) {
	id := NewULID()
	ch := c.registerPending(id)
	defer c.unregisterPending(id)
	if err := c.send(ctx, &cadestrov1.AgentMessage{
		Id: &cadestrov1.MessageId{Value: id},
		Payload: &cadestrov1.AgentMessage_ValidateLuksToken{
			ValidateLuksToken: &cadestrov1.ValidateLuksTokenRequest{Token: token},
		},
	}); err != nil {
		return nil, fmt.Errorf("send validate luks token request: %w", err)
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case response, ok := <-ch:
		if !ok || response == nil {
			return nil, errors.New("stream closed while waiting for ValidateLuksToken response")
		}
		if errorMessage := response.GetError(); errorMessage != nil {
			return nil, fmt.Errorf("server error: %s", errorMessage.Message)
		}
		validated := response.GetValidateLuksToken()
		if validated == nil {
			return nil, errors.New("unexpected response type")
		}
		if err := c.validateInbound(validated); err != nil {
			return nil, fmt.Errorf("invalid ValidateLuksToken response: %w", err)
		}
		return &ValidateLuksTokenResult{
			ActionID:   validated.GetActionId().GetValue(),
			DevicePath: validated.DevicePath,
			MinLength:  validated.MinLength,
			Complexity: validated.Complexity,
		}, nil
	}
}

// SyncStateResult contains the current device state returned over the stream.
type SyncStateResult struct {
	// SyncIntervalMinutes is the effective sync interval for this device.
	// 0 means use the default (30 minutes).

	SyncIntervalMinutes int32
	// MaintenanceWindow is the server-resolved union of every reaching
	// group's window (device groups + user groups assigned to the
	// device). nil means "no constraint" — the agent dispatches at any
	// time. The agent evaluates this against time.Now().Local() before
	// firing scheduler-driven dispatches.

	MaintenanceWindow *cadestrov1.MaintenanceWindow
	// DesiredPolicy is the authenticated assignment snapshot reconciled locally.

	DesiredPolicy *cadestrov1.DesiredPolicy
}

// Sync requests the current desired policy on the existing authenticated stream.
func (c *Client) Sync(ctx context.Context) (*SyncStateResult, error) {
	id := NewULID()
	ch := c.registerPending(id)
	defer c.unregisterPending(id)
	if err := c.send(ctx, &cadestrov1.AgentMessage{
		Id:      &cadestrov1.MessageId{Value: id},
		Payload: &cadestrov1.AgentMessage_SyncRequest{SyncRequest: &cadestrov1.SyncRequest{}},
	}); err != nil {
		return nil, fmt.Errorf("send sync request: %w", err)
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case response, ok := <-ch:
		if !ok || response == nil {
			return nil, errors.New("stream closed while waiting for SyncState response")
		}
		if errorMessage := response.GetError(); errorMessage != nil {
			return nil, fmt.Errorf("server error: %s", errorMessage.Message)
		}
		state := response.GetSyncState()
		if state == nil {
			return nil, errors.New("unexpected response type")
		}
		if err := c.validateInbound(state); err != nil {
			return nil, fmt.Errorf("invalid SyncState response: %w", err)
		}
		return &SyncStateResult{
			SyncIntervalMinutes: state.SyncIntervalMinutes,
			MaintenanceWindow:   state.MaintenanceWindow,
			DesiredPolicy:       state.DesiredPolicy,
		}, nil
	}
}

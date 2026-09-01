// Package contract provides the Cadestro agent protocol client.
package contract

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"github.com/oklog/ulid/v2"
	"golang.org/x/net/http2"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
)

const maxInboundMessageBytes = 16 << 20

// Client maintains one authenticated bidirectional agent stream.
type Client struct {
	client     cadestrov1connect.AgentServiceClient
	deviceID   string
	logger     *slog.Logger
	httpClient *http.Client

	mu     sync.RWMutex
	stream *connect.BidiStreamForClient[cadestrov1.AgentMessage, cadestrov1.ServerMessage]
	sendMu sync.Mutex

	pendingMu sync.Mutex
	pending   map[string]chan *cadestrov1.ServerMessage
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// NewClient creates an agent protocol client.
func NewClient(serverURL string, options ...ClientOption) *Client {
	client := &Client{logger: slog.Default(), pending: make(map[string]chan *cadestrov1.ServerMessage), httpClient: http.DefaultClient}
	for _, option := range options {
		option(client)
	}
	client.client = cadestrov1connect.NewAgentServiceClient(client.httpClient, serverURL, connect.WithReadMaxBytes(maxInboundMessageBytes))
	return client
}

// WithHTTPClient supplies the HTTP client used for RPCs.
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(client *Client) { client.httpClient = httpClient }
}

// WithDeviceID supplies the device identity.
func WithDeviceID(deviceID string) ClientOption {
	return func(client *Client) { client.deviceID = deviceID }
}

// WithLogger supplies the structured logger.
func WithLogger(logger *slog.Logger) ClientOption {
	return func(client *Client) { client.logger = logger }
}

// WithTLSConfig supplies a TLS configuration.
func WithTLSConfig(config *tls.Config) ClientOption {
	return func(client *Client) { client.httpClient = newHTTPClientWithTLS(config) }
}

// WithMTLSFromPEM configures TLS 1.3 client authentication and trusts only the supplied CA.
func WithMTLSFromPEM(certPEM, keyPEM, caPEM []byte) (ClientOption, error) {
	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse client certificate: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("parse CA certificate")
	}
	return WithTLSConfig(&tls.Config{
		Certificates: []tls.Certificate{certificate}, RootCAs: pool, MinVersion: tls.VersionTLS13,
	}), nil
}

func newHTTPClientWithTLS(config *tls.Config) *http.Client {
	transport := &http.Transport{TLSClientConfig: config}
	if err := http2.ConfigureTransport(transport); err != nil {
		slog.Warn("configure HTTP/2 transport", "error", err)
	}
	return &http.Client{Transport: transport}
}

func bootstrapHTTPClient() *http.Client {
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13}}
	if err := http2.ConfigureTransport(transport); err != nil {
		slog.Warn("configure bootstrap HTTP/2 transport", "error", err)
	}
	return &http.Client{Timeout: time.Minute, Transport: transport}
}

// RegisterAgentResult contains the certificate bundle and stream endpoint returned by enrollment.
type RegisterAgentResult struct {
	DeviceID    string
	CACert      []byte
	Certificate []byte
	ControlURL  string
}

// RegisterAgent enrolls a device through the public control endpoint.
func RegisterAgent(ctx context.Context, controlURL, token, hostname, agentVersion string, csr []byte, options ...ClientOption) (*RegisterAgentResult, error) {
	client := &Client{httpClient: bootstrapHTTPClient()}
	for _, option := range options {
		option(client)
	}
	control := cadestrov1connect.NewControlServiceClient(client.httpClient, controlURL)
	response, err := control.Register(ctx, connect.NewRequest(&cadestrov1.RegisterRequest{
		Token: token, Hostname: hostname, AgentVersion: agentVersion, Csr: csr,
	}))
	if err != nil {
		return nil, fmt.Errorf("register agent: %w", err)
	}
	return &RegisterAgentResult{
		DeviceID: response.Msg.GetDeviceId().GetValue(), CACert: response.Msg.GetCaCert(),
		Certificate: response.Msg.GetCertificate(), ControlURL: response.Msg.GetControlUrl(),
	}, nil
}

// RenewCertificateResult contains the renewed device certificate.
type RenewCertificateResult struct {
	Certificate []byte
	NotAfter    time.Time
}

// RenewCertificate renews the certificate presented by the authenticated device.
func RenewCertificate(ctx context.Context, controlURL string, csr []byte, options ...ClientOption) (*RenewCertificateResult, error) {
	client := &Client{httpClient: bootstrapHTTPClient()}
	for _, option := range options {
		option(client)
	}
	control := cadestrov1connect.NewControlServiceClient(client.httpClient, controlURL)
	response, err := control.RenewCertificate(ctx, connect.NewRequest(&cadestrov1.RenewCertificateRequest{Csr: csr}))
	if err != nil {
		return nil, fmt.Errorf("renew certificate: %w", err)
	}
	return &RenewCertificateResult{Certificate: response.Msg.GetCertificate(), NotAfter: response.Msg.GetNotAfter().AsTime()}, nil
}

// Run connects and owns the stream until it closes or ctx is cancelled.
func (client *Client) Run(ctx context.Context, hostname, agentVersion string, welcome chan<- *cadestrov1.Welcome) error {
	if welcome == nil {
		return errors.New("welcome channel is required")
	}
	client.mu.Lock()
	if client.stream != nil {
		client.mu.Unlock()
		return errors.New("agent stream is already connected")
	}
	stream := client.client.Stream(ctx)
	client.stream = stream
	client.mu.Unlock()
	defer client.closeStream(stream)

	if err := client.send(ctx, &cadestrov1.AgentMessage{
		Id: &cadestrov1.MessageId{Value: NewULID()}, Payload: &cadestrov1.AgentMessage_Hello{Hello: &cadestrov1.Hello{
			DeviceId: &cadestrov1.DeviceId{Value: client.deviceID}, AgentVersion: agentVersion, Hostname: hostname,
		}},
	}); err != nil {
		return fmt.Errorf("send hello: %w", err)
	}
	heartbeatStarted := false

	for {
		message, err := stream.Receive()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
				return ctx.Err()
			}
			return fmt.Errorf("receive agent stream: %w", err)
		}
		if err := protovalidate.Validate(message); err != nil {
			return fmt.Errorf("validate server message: %w", err)
		}
		if client.deliverPending(message) {
			continue
		}
		switch payload := message.GetPayload().(type) {
		case *cadestrov1.ServerMessage_Welcome:
			select {
			case welcome <- payload.Welcome:
			default:
			}
			if !heartbeatStarted {
				interval := payload.Welcome.GetHeartbeatInterval().AsDuration()
				if interval <= 0 {
					return errors.New("welcome heartbeat interval must be positive")
				}
				heartbeatCtx, cancelHeartbeat := context.WithCancel(ctx)
				defer cancelHeartbeat()
				go client.sendHeartbeats(heartbeatCtx, interval)
				heartbeatStarted = true
			}
		default:
			return fmt.Errorf("unexpected uncorrelated server message %T", payload)
		}
	}
}

func (client *Client) sendHeartbeats(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := client.send(ctx, &cadestrov1.AgentMessage{
				Id: &cadestrov1.MessageId{Value: NewULID()}, Payload: &cadestrov1.AgentMessage_Heartbeat{Heartbeat: &cadestrov1.Heartbeat{}},
			}); err != nil {
				client.logger.Debug("send heartbeat", "error", err)
				return
			}
		}
	}
}

func (client *Client) closeStream(stream *connect.BidiStreamForClient[cadestrov1.AgentMessage, cadestrov1.ServerMessage]) {
	client.mu.Lock()
	if client.stream == stream {
		client.stream = nil
	}
	client.mu.Unlock()
	client.pendingMu.Lock()
	for id, pending := range client.pending {
		close(pending)
		delete(client.pending, id)
	}
	client.pendingMu.Unlock()
}

func (client *Client) send(ctx context.Context, message *cadestrov1.AgentMessage) error {
	if err := protovalidate.Validate(message); err != nil {
		return fmt.Errorf("validate agent message: %w", err)
	}
	client.mu.RLock()
	stream := client.stream
	client.mu.RUnlock()
	if stream == nil {
		return errors.New("agent stream is not connected")
	}
	client.sendMu.Lock()
	defer client.sendMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := stream.Send(message); err != nil {
		return fmt.Errorf("send agent message: %w", err)
	}
	return nil
}

func (client *Client) registerPending(id string) chan *cadestrov1.ServerMessage {
	pending := make(chan *cadestrov1.ServerMessage, 1)
	client.pendingMu.Lock()
	client.pending[id] = pending
	client.pendingMu.Unlock()
	return pending
}

func (client *Client) unregisterPending(id string) {
	client.pendingMu.Lock()
	delete(client.pending, id)
	client.pendingMu.Unlock()
}

func (client *Client) deliverPending(message *cadestrov1.ServerMessage) bool {
	client.pendingMu.Lock()
	defer client.pendingMu.Unlock()
	pending := client.pending[message.GetId().GetValue()]
	if pending == nil {
		return false
	}
	pending <- message
	return true
}

// SendActionResult durably acknowledges delivery with the server before returning.
func (client *Client) SendActionResult(ctx context.Context, result *cadestrov1.ActionResult) error {
	return client.sendResult(ctx, &cadestrov1.AgentMessage{Payload: &cadestrov1.AgentMessage_ActionResult{ActionResult: result}})
}

func (client *Client) sendResult(ctx context.Context, message *cadestrov1.AgentMessage) error {
	id := NewULID()
	pending := client.registerPending(id)
	defer client.unregisterPending(id)
	message.Id = &cadestrov1.MessageId{Value: id}
	if err := client.send(ctx, message); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case response, ok := <-pending:
		if !ok {
			return errors.New("agent stream closed before result acknowledgement")
		}
		if response.GetResultAck().GetCode() != cadestrov1.ResultAckCode_RESULT_ACK_CODE_ACCEPTED {
			return errors.New("server rejected result")
		}
		return nil
	}
}

// PullDesiredPolicy pulls the desired policy over the authenticated stream.
func (client *Client) PullDesiredPolicy(ctx context.Context) (*cadestrov1.DesiredPolicy, error) {
	id := NewULID()
	pending := client.registerPending(id)
	defer client.unregisterPending(id)
	if err := client.send(ctx, &cadestrov1.AgentMessage{
		Id: &cadestrov1.MessageId{Value: id}, Payload: &cadestrov1.AgentMessage_DesiredPolicyRequest{DesiredPolicyRequest: &cadestrov1.DesiredPolicyRequest{}},
	}); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case response, ok := <-pending:
		if !ok {
			return nil, errors.New("agent stream closed before desired policy response")
		}
		policy := response.GetDesiredPolicy()
		if policy == nil {
			return nil, errors.New("unexpected desired policy response")
		}
		return policy, nil
	}
}

// CloseIdleConnections releases transport keep-alive resources.
func (client *Client) CloseIdleConnections() {
	if client != nil && client.httpClient != nil {
		client.httpClient.CloseIdleConnections()
	}
}

// DeviceID returns the configured device identity.
func (client *Client) DeviceID() string {
	client.mu.RLock()
	defer client.mu.RUnlock()
	return client.deviceID
}

// NewULID returns a cryptographically random ULID.
func NewULID() string {
	return ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader).String()
}

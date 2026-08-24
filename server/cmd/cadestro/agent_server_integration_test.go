package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"os"
	"syscall"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	sdk "github.com/manchtools/cadestro/contract"
	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/manchtools/cadestro/sdk/crypto"
	"github.com/manchtools/cadestro/server/internal/agentstream"
	"github.com/manchtools/cadestro/server/internal/ca"
)

type integrationAgentService struct {
	cadestrov1connect.UnimplementedAgentServiceHandler
	hello chan string
}

func (s *integrationAgentService) Stream(ctx context.Context, stream *connect.BidiStream[cadestrov1.AgentMessage, cadestrov1.ServerMessage]) error {
	first, err := stream.Receive()
	if err != nil {
		return err
	}
	if first.GetHello() == nil {
		return errors.New("first frame was not Hello")
	}
	if got, ok := agentstream.DeviceIDFromContext(ctx); !ok || got == "" {
		return errors.New("mTLS device identity was not bound")
	} else {
		s.hello <- got
	}
	if err := stream.Send(&cadestrov1.ServerMessage{Id: first.Id, Payload: &cadestrov1.ServerMessage_Welcome{Welcome: &cadestrov1.Welcome{}}}); err != nil {
		return err
	}
	<-ctx.Done()
	return ctx.Err()
}

func TestBuildAgentServerTLSHTTP2AndMTLS(t *testing.T) {
	caPEM, caKeyPEM, caAuthority := integrationCA(t)
	caAuthorityFromPEM, err := ca.NewFromPEM(caPEM, caKeyPEM, time.Hour)
	require.NoError(t, err)
	serverCSR, serverKeyPEM, err := crypto.GenerateCSR("control")
	require.NoError(t, err)
	serverCert, err := caAuthorityFromPEM.IssueServerCertificateFromCSR("control", serverCSR, "localhost")
	require.NoError(t, err)
	serverCertFile := writeTemp(t, "server.crt", serverCert.CertPEM)
	serverKeyFile := writeTemp(t, "server.key", serverKeyPEM)
	clientCSR, clientKeyPEM, err := crypto.GenerateCSR("agent")
	require.NoError(t, err)
	clientCert, err := caAuthorityFromPEM.IssueCertificateFromCSR("01K00000000000000000000001", clientCSR)
	require.NoError(t, err)

	service := &integrationAgentService{hello: make(chan string, 1)}
	path, serviceHandler := cadestrov1connect.NewAgentServiceHandler(service)
	mux := http.NewServeMux()
	protocols := make(chan int, 1)
	mtlsHandler := agentstream.MTLSMiddleware(serviceHandler)
	mux.Handle(path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		protocols <- r.ProtoMajor
		mtlsHandler.ServeHTTP(w, r)
	}))

	listener := testTCPListener(t)
	server, err := buildAgentServer(&Config{AgentListen: listener.Addr().String(), AgentTLSCertFile: serverCertFile, AgentTLSKeyFile: serverKeyFile}, caAuthority, mux)
	require.NoError(t, err)
	serverErr := make(chan error, 1)
	go func() { serverErr <- server.Serve(tls.NewListener(listener, server.TLSConfig)) }()
	t.Cleanup(func() {
		require.NoError(t, server.Shutdown(context.Background()))
		select {
		case err := <-serverErr:
			require.ErrorIs(t, err, http.ErrServerClosed)
		default:
		}
	})

	clientTLS, err := tls.X509KeyPair(clientCert.CertPEM, clientKeyPEM)
	require.NoError(t, err)
	roots := x509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM(caPEM))
	client := sdk.NewClient("https://"+listener.Addr().String(), sdk.WithTLSConfig(&tls.Config{
		Certificates: []tls.Certificate{clientTLS}, RootCAs: roots, ServerName: "localhost", MinVersion: tls.VersionTLS13,
	}), sdk.WithAuth("01K00000000000000000000001", ""))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := make(chan error, 1)
	go func() { runErr <- client.Run(ctx, "agent-host", "test", time.Hour, integrationStreamHandler{}) }()
	select {
	case got := <-service.hello:
		require.Equal(t, "01K00000000000000000000001", got)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for authenticated Hello")
	}
	select {
	case got := <-protocols:
		require.Equal(t, 2, got, "AgentService.Stream must use HTTP/2")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out observing AgentService protocol")
	}
	cancel()
	select {
	case <-runErr:
	case <-time.After(5 * time.Second):
		t.Fatal("client did not stop")
	}
}

func TestBuildAgentServerRejectsMissingAndWrongClientIdentity(t *testing.T) {
	caPEM, caKeyPEM, caAuthority := integrationCA(t)
	caAuthorityFromPEM, err := ca.NewFromPEM(caPEM, caKeyPEM, time.Hour)
	require.NoError(t, err)
	serverCSR, serverKeyPEM, err := crypto.GenerateCSR("control")
	require.NoError(t, err)
	serverCert, err := caAuthorityFromPEM.IssueServerCertificateFromCSR("control", serverCSR, "localhost")
	require.NoError(t, err)
	service := &integrationAgentService{hello: make(chan string, 1)}
	path, serviceHandler := cadestrov1connect.NewAgentServiceHandler(service)
	mux := http.NewServeMux()
	mux.Handle(path, agentstream.MTLSMiddleware(serviceHandler))
	listener := testTCPListener(t)
	server, err := buildAgentServer(&Config{AgentListen: listener.Addr().String(), AgentTLSCertFile: writeTemp(t, "server.crt", serverCert.CertPEM), AgentTLSKeyFile: writeTemp(t, "server.key", serverKeyPEM)}, caAuthority, mux)
	require.NoError(t, err)
	go func() { _ = server.Serve(tls.NewListener(listener, server.TLSConfig)) }()
	t.Cleanup(func() { _ = server.Shutdown(context.Background()) })
	roots := x509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM(caPEM))

	for _, tc := range []struct {
		name string
		cert []byte
		key  []byte
	}{
		{name: "missing client certificate"},
		func() struct {
			name string
			cert []byte
			key  []byte
		} {
			csr, key, err := crypto.GenerateCSR("control")
			require.NoError(t, err)
			cert, err := caAuthorityFromPEM.IssueServerCertificateFromCSR("control", csr, "localhost")
			require.NoError(t, err)
			return struct {
				name string
				cert []byte
				key  []byte
			}{name: "wrong client identity", cert: cert.CertPEM, key: key}
		}(),
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := &tls.Config{RootCAs: roots, ServerName: "localhost", MinVersion: tls.VersionTLS13}
			if len(tc.cert) != 0 {
				pair, err := tls.X509KeyPair(tc.cert, tc.key)
				require.NoError(t, err)
				config.Certificates = []tls.Certificate{pair}
			}
			client := sdk.NewClient("https://"+listener.Addr().String(), sdk.WithTLSConfig(config), sdk.WithAuth("01K00000000000000000000001", ""))
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			err := client.Run(ctx, "agent-host", "test", time.Hour, integrationStreamHandler{})
			require.Error(t, err)
		})
	}
}

type integrationStreamHandler struct{}

func (integrationStreamHandler) OnWelcome(context.Context, *cadestrov1.Welcome) error { return nil }
func (integrationStreamHandler) OnQuery(context.Context, *cadestrov1.OSQuery) (*cadestrov1.OSQueryResult, error) {
	return nil, nil
}
func (integrationStreamHandler) OnError(context.Context, *cadestrov1.Error) error { return nil }

func integrationCA(t *testing.T) ([]byte, []byte, *ca.CA) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "integration CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, template, template, public, private)
	require.NoError(t, err)
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalPKCS8PrivateKey(private)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	authority, err := ca.NewFromPEM(caPEM, keyPEM, time.Hour)
	require.NoError(t, err)
	return caPEM, keyPEM, authority
}

func writeTemp(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := t.TempDir() + "/" + name
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func testTCPListener(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if errors.Is(err, syscall.EPERM) {
		t.Skip("sandbox forbids TCP sockets")
	}
	require.NoError(t, err)
	return listener
}

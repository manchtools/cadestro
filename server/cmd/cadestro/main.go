package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"connectrpc.com/connect"
	connectvalidate "connectrpc.com/validate"

	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/manchtools/cadestro/sdk/crypto"
	"github.com/manchtools/cadestro/sdk/logging"
	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/ca"
	"github.com/manchtools/cadestro/server/internal/core"
	"github.com/manchtools/cadestro/server/internal/middleware"
	"github.com/manchtools/cadestro/server/internal/store"
)

var version = "dev"

func main() {
	config, err := loadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cadestro: invalid configuration:", err)
		os.Exit(2)
	}
	logger := logging.SetupLogger(config.LogLevel, config.LogFormat, os.Stderr)
	slog.SetDefault(logger)
	if err := run(config, logger); err != nil {
		logger.Error("control stopped", "error", err)
		os.Exit(1)
	}
}

func run(config *Config, logger *slog.Logger) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	storage, err := store.New(ctx, config.DatabasePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer storage.Close()
	certificateAuthority, err := ca.New(config.CACertFile, config.CAKeyFile, config.CertificateValidity)
	if err != nil {
		return fmt.Errorf("load certificate authority: %w", err)
	}
	jwt, err := auth.NewJWTManager(auth.JWTConfig{PrivateKey: config.SessionSigningKey})
	if err != nil {
		return fmt.Errorf("load session signer: %w", err)
	}
	fingerprint, err := crypto.CAFingerprintFromPEM(certificateAuthority.CACertPEM())
	if err != nil {
		return fmt.Errorf("fingerprint certificate authority: %w", err)
	}
	service := core.New(core.Config{
		Store: storage, CA: certificateAuthority, JWT: jwt, Logger: logger,
		PublicBaseURL: config.PublicBaseURL, AgentURL: config.AgentURL, CAFingerprint: fingerprint,
		Version: version, HeartbeatInterval: config.HeartbeatInterval,
	})
	if err := service.EnsureBootstrapProvider(ctx, core.BootstrapProvider{
		Name: config.BootstrapOIDCName, Slug: config.BootstrapOIDCSlug, ClientID: config.BootstrapOIDCClientID,
		IssuerURL: config.BootstrapOIDCIssuer, Scopes: config.BootstrapOIDCScopes,
	}); err != nil {
		return fmt.Errorf("bootstrap identity provider: %w", err)
	}
	publicServer, agentServer, err := buildServers(config, service, jwt, logger, certificateAuthority)
	if err != nil {
		return err
	}
	errorsChannel := make(chan error, 2)
	go serve(publicServer, "public", logger, errorsChannel)
	go serve(agentServer, "agent", logger, errorsChannel)
	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-errorsChannel:
		cancel()
	}
	shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	publicErr := publicServer.Shutdown(shutdownContext)
	agentErr := agentServer.Shutdown(shutdownContext)
	if serveErr != nil {
		return serveErr
	}
	if publicErr != nil {
		return fmt.Errorf("shut down public listener: %w", publicErr)
	}
	if agentErr != nil {
		return fmt.Errorf("shut down agent listener: %w", agentErr)
	}
	return nil
}

func buildServers(config *Config, service *core.Service, jwt *auth.JWTManager, logger *slog.Logger, certificateAuthority *ca.CA) (*http.Server, *http.Server, error) {
	if _, err := readPrivateFile(config.PublicTLSKeyFile); err != nil {
		return nil, nil, fmt.Errorf("public TLS key: %w", err)
	}
	publicCertificate, err := tls.LoadX509KeyPair(config.PublicTLSCertFile, config.PublicTLSKeyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("load public TLS certificate: %w", err)
	}
	if _, err := readPrivateFile(config.AgentTLSKeyFile); err != nil {
		return nil, nil, fmt.Errorf("agent TLS key: %w", err)
	}
	agentCertificate, err := tls.LoadX509KeyPair(config.AgentTLSCertFile, config.AgentTLSKeyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("load agent TLS certificate: %w", err)
	}
	validator := connectvalidate.NewInterceptor()
	authenticator := auth.NewInterceptor(jwt)
	publicMux := http.NewServeMux()
	publicPath, publicHandler := cadestrov1connect.NewControlServiceHandler(service, connect.WithInterceptors(validator, connect.UnaryInterceptorFunc(authenticator.WrapUnary)))
	publicMux.Handle(publicPath, publicHandler)
	publicMux.HandleFunc("/health", health)
	publicMux.HandleFunc("/ready", health)
	publicRoot := middleware.RequestID(middleware.SecurityHeaders(middleware.CORS(config.CORSOrigins, false, logger)(publicMux)))
	agentMux := http.NewServeMux()
	agentPath, agentHandler := cadestrov1connect.NewAgentServiceHandler(service, connect.WithInterceptors(validator))
	agentMux.Handle(agentPath, agentHandler)
	agentMux.Handle(cadestrov1connect.ControlServiceRenewCertificateProcedure, connect.NewUnaryHandler(cadestrov1connect.ControlServiceRenewCertificateProcedure, service.RenewCertificate, connect.WithInterceptors(validator)))
	agentMux.HandleFunc("/health", health)
	agentMux.HandleFunc("/ready", health)
	agentRoot := core.AgentMiddleware(agentMux)
	publicServer := &http.Server{
		Addr: config.PublicListen, Handler: publicRoot, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second,
		MaxHeaderBytes: 1 << 20, TLSConfig: &tls.Config{Certificates: []tls.Certificate{publicCertificate}, MinVersion: tls.VersionTLS13},
	}
	agentServer := &http.Server{
		Addr: config.AgentListen, Handler: agentRoot, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second,
		MaxHeaderBytes: 1 << 20, TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{agentCertificate}, ClientAuth: tls.RequireAndVerifyClientCert,
			ClientCAs: certificateAuthority.TrustPool(), MinVersion: tls.VersionTLS13,
		},
	}
	return publicServer, agentServer, nil
}

func serve(server *http.Server, name string, logger *slog.Logger, errorsChannel chan<- error) {
	logger.Info(name+" listener ready", "address", server.Addr)
	if err := server.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
		errorsChannel <- fmt.Errorf("%s listener: %w", name, err)
	}
}

func health(response http.ResponseWriter, _ *http.Request) {
	response.Header().Set("Content-Type", "text/plain; charset=utf-8")
	response.WriteHeader(http.StatusOK)
	_, _ = response.Write([]byte("ok\n"))
}

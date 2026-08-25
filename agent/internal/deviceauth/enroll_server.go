package deviceauth

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
)

const (
	EnrollSocketPath = "/run/cadestro/enroll.sock"
)

type EnrollServer struct {
	handler      *EnrollHandler
	socketPath   string
	logger       *slog.Logger
	httpServer   *http.Server
	shutdownOnce sync.Once
}

func NewEnrollServer(handler *EnrollHandler, socketPath string, logger *slog.Logger) *EnrollServer {
	if socketPath == "" {
		socketPath = EnrollSocketPath
	}
	return &EnrollServer{
		handler:    handler,
		socketPath: socketPath,
		logger:     logger,
	}
}

func (s *EnrollServer) Start(ctx context.Context) error {

	dir := filepath.Dir(s.socketPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create socket directory %s: %w", dir, err)
	}

	os.Remove(s.socketPath)

	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.socketPath, err)
	}

	if err := os.Chmod(s.socketPath, 0600); err != nil {
		_ = listener.Close()
		return fmt.Errorf("chmod socket: %w", err)
	}

	listener = newPeerCredListener(listener, s.logger)

	mux := http.NewServeMux()
	path, handler := cadestrov1connect.NewDeviceAuthServiceHandler(s.handler)
	mux.Handle(path, handler)

	s.httpServer = &http.Server{
		Handler: mux,

		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	s.logger.Info("enrollment service listening", "socket", s.socketPath)

	go func() {
		<-ctx.Done()
		s.Shutdown()
	}()

	if err := s.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

func (s *EnrollServer) Shutdown() {
	s.shutdownOnce.Do(func() {
		if s.httpServer != nil {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			s.httpServer.Shutdown(shutdownCtx)
			os.Remove(s.socketPath)
		}
	})
}

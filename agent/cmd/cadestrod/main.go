package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/manchtools/cadestro/agent/internal/credentials"
	"github.com/manchtools/cadestro/agent/internal/deviceauth"
	"github.com/manchtools/cadestro/agent/internal/executor"
	"github.com/manchtools/cadestro/agent/internal/handler"
	"github.com/manchtools/cadestro/agent/internal/scheduler"
	"github.com/manchtools/cadestro/agent/internal/store"
	"github.com/manchtools/cadestro/sdk/logging"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

var version = "dev"

const (
	defaultHeartbeatInterval = 30 * time.Second
	defaultSyncInterval      = 30 * time.Minute
	minInitialBackoff        = 5 * time.Second
	maxInitialBackoff        = 10 * time.Second
	maxBackoff               = 5 * time.Minute
)

type Config struct {
	DataDir   string
	LogLevel  string
	LogFormat string
}

func main() {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "version", "--version", "-v":
			fmt.Printf("cadestrod %s\n", version)
			return
		case "enroll":
			runEnroll(os.Args[2:])
			return
		case "install-unit":
			os.Exit(runInstallUnit(os.Args[2:]))
		}
	}

	cfg := parseFlags()
	logger := logging.SetupLogger(cfg.LogLevel, cfg.LogFormat, os.Stdout)
	slog.SetDefault(logger)
	backend, err := rootBackend(os.Geteuid())
	if err != nil {
		logger.Error("cadestrod must run as root", "error", err)
		os.Exit(1)
	}
	runner, err := sysexec.NewRunner(backend)
	if err != nil {
		logger.Error("create process runner", "error", err)
		os.Exit(1)
	}
	hostname, err := os.Hostname()
	if err != nil {
		logger.Error("read hostname", "error", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	reconcileUnitAtStartup(ctx, runner, logger, cfg.DataDir)
	credentialStore := credentials.NewStore(cfg.DataDir)
	creds, err := loadOrEnroll(ctx, credentialStore, hostname, logger)
	if err != nil {
		logger.Error("initialize enrollment", "error", err)
		os.Exit(1)
	}
	actionStore, err := store.New(cfg.DataDir)
	if err != nil {
		logger.Error("open action store", "error", err)
		os.Exit(1)
	}
	defer actionStore.Close()

	actionExecutor := executor.NewExecutor(runner)
	actionScheduler := scheduler.New(actionStore, actionExecutor, logger)
	streamHandler := handler.NewHandler(logger)
	go actionScheduler.Start(ctx)
	runAgent(ctx, credentialStore, creds, hostname, streamHandler, actionScheduler, logger, time.Now)
	actionScheduler.Stop()
}

func loadOrEnroll(ctx context.Context, credentialStore *credentials.Store, hostname string, logger *slog.Logger) (*credentials.Credentials, error) {
	if credentialStore.Exists() {
		return credentialStore.Load()
	}
	enrolled := make(chan *credentials.Credentials, 1)
	enrollHandler := deviceauth.NewEnrollHandler(hostname, version, credentialStore, logger, func(creds *credentials.Credentials) { enrolled <- creds })
	enrollServer := deviceauth.NewEnrollServer(enrollHandler, deviceauth.EnrollSocketPath, logger)
	errors := make(chan error, 1)
	go func() { errors <- enrollServer.Start(ctx) }()
	defer enrollServer.Shutdown()
	select {
	case creds := <-enrolled:
		return creds, nil
	case err := <-errors:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func parseFlags() *Config {
	cfg := &Config{}
	flag.StringVar(&cfg.DataDir, "data-dir", credentials.DefaultDataDir, "Data directory")
	flag.StringVar(&cfg.LogLevel, "log-level", "info", "Log level")
	flag.StringVar(&cfg.LogFormat, "log-format", "text", "Log format")
	flag.Parse()
	if uri := flag.Arg(0); registrationURIRefusedByHandler(uri) {
		fmt.Fprintf(os.Stderr, "refusing implicit enrollment; run cadestrod enroll %q\n", uri)
		os.Exit(1)
	}
	if value := os.Getenv("CADESTRO_DATA_DIR"); value != "" {
		cfg.DataDir = value
	}
	return cfg
}

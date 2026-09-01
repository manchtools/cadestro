package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/manchtools/cadestro/agent/internal/credentials"
	"github.com/manchtools/cadestro/agent/internal/executor"
	"github.com/manchtools/cadestro/agent/internal/scheduler"
	"github.com/manchtools/cadestro/agent/internal/store"
	"github.com/manchtools/cadestro/sdk/logging"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

var version = "dev"

const (
	defaultPolicyRefreshInterval = 30 * time.Minute
	minInitialBackoff            = 5 * time.Second
	maxInitialBackoff            = 10 * time.Second
	maxBackoff                   = 5 * time.Minute
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
			os.Exit(runEnroll(os.Args[2:], os.Geteuid()))
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
	creds, err := loadCredentials(credentialStore)
	if err != nil {
		logger.Error("initialize enrollment", "error", err)
		os.Exit(1)
	}
	actionStore, err := store.New(cfg.DataDir)
	if err != nil {
		logger.Error("open action store", "error", err)
		os.Exit(1)
	}

	actionExecutor, err := executor.NewExecutor(runner)
	if err != nil {
		logger.Error("create action executor", "error", err)
		os.Exit(1)
	}
	actionScheduler := scheduler.New(actionStore, actionExecutor, logger)
	schedulerDone := make(chan struct{})
	go func() {
		defer close(schedulerDone)
		actionScheduler.Run(ctx)
	}()
	runAgent(ctx, credentialStore, creds, hostname, actionScheduler, logger, time.Now)
	cancel()
	<-schedulerDone
	if err := actionStore.Close(); err != nil {
		logger.Error("close action store", "error", err)
	}
}

func loadCredentials(credentialStore *credentials.Store) (*credentials.Credentials, error) {
	creds, err := credentialStore.Load()
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("agent is not enrolled; run cadestrod enroll")
	}
	if err != nil {
		return nil, fmt.Errorf("load credentials: %w", err)
	}
	if !creds.Ready() {
		return nil, fmt.Errorf("agent is not enrolled; run cadestrod enroll")
	}
	return creds, nil
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

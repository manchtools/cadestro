package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/manchtools/cadestro/agent/internal/credentials"
	"github.com/manchtools/cadestro/agent/internal/deviceauth"
	"github.com/manchtools/cadestro/agent/internal/executor"
	"github.com/manchtools/cadestro/agent/internal/handler"
	"github.com/manchtools/cadestro/agent/internal/luksd"
	"github.com/manchtools/cadestro/agent/internal/scheduler"
	"github.com/manchtools/cadestro/agent/internal/store"
	"github.com/manchtools/cadestro/sdk/logging"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
)

var version = "dev"

const (
	defaultHeartbeatInterval = 30 * time.Second
	defaultSyncInterval      = 30 * time.Minute

	minInitialBackoff = 5 * time.Second
	maxInitialBackoff = 10 * time.Second
	maxBackoff        = 5 * time.Minute
	backoffFactor     = 2.0
)

type Config struct {
	DataDir string

	LogLevel  string
	LogFormat string

	PrivilegeBackend string

	pendingSecurityAlert *pendingSecurityAlert
}

type pendingSecurityAlert struct {
	alertType        string
	message          string
	requestedServer  string
	registeredServer string
}

func main() {

	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "version", "--version", "-v":
			fmt.Printf("cadestrod %s\n", version)
			return
		case "query", "--query", "-query":
			runQuery(os.Args[2:])
			return
		case "luks":
			runLuks(os.Args[2:])
			return
		case "enroll":
			runEnroll(os.Args[2:])
			return
		case "self-test":
			os.Exit(runSelfTest(os.Args[2:]))
		case "tty":
			os.Exit(runTTY(os.Args[2:]))
		case "install-unit":
			os.Exit(runInstallUnit(os.Args[2:]))
		}
	}

	cfg := parseFlags()

	logger := logging.SetupLogger(cfg.LogLevel, cfg.LogFormat, os.Stdout)
	slog.SetDefault(logger)
	logger.Info("logger initialized", "level", cfg.LogLevel, "format", cfg.LogFormat)

	resolvedBackend, err := applyBackendOverrides(cfg, logger)
	if err != nil {
		logger.Error("backend validation failed", "error", err)
		os.Exit(1)
	}

	runner, err := sysexec.NewRunner(resolvedBackend)
	if err != nil {
		logger.Error("failed to build privilege runner", "error", err)
		os.Exit(1)
	}

	executor.CheckStartupUpdateState(cfg.DataDir, logger, time.Now)

	hostname, err := os.Hostname()
	if err != nil {
		logger.Error("failed to get hostname", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Info("received signal, shutting down", "signal", sig)
		cancel()
	}()

	reconcileUnitAtStartup(ctx, runner, logger, cfg.DataDir)

	credStore := credentials.NewStore(cfg.DataDir)

	var creds *credentials.Credentials
	if credStore.Exists() {
		logger.Info("loading stored credentials", "data_dir", credStore.DataDir())
		creds, err = credStore.Load()
		if err != nil {
			logger.Error("failed to load credentials", "error", err)
			logger.Info("hint: delete stored credentials to re-register",
				"path", credStore.DataDir())
			os.Exit(1)
		}
		logger.Info("credentials loaded",
			"device_id", creds.DeviceID,
			"control", creds.AgentAddr,
		)

	} else {

		logger.Info("agent not enrolled, waiting for enrollment via socket",
			"socket", deviceauth.EnrollSocketPath)

		enrollCh := make(chan *credentials.Credentials, 1)
		enrollHandler := deviceauth.NewEnrollHandler(hostname, version, credStore, logger, func(c *credentials.Credentials) {
			enrollCh <- c
		})
		enrollServer := deviceauth.NewEnrollServer(enrollHandler, deviceauth.EnrollSocketPath, logger)

		enrollErrCh := make(chan error, 1)
		go func() {
			if err := enrollServer.Start(ctx); err != nil {
				logger.Error("enrollment server failed", "error", err)
				enrollErrCh <- err
			}
		}()

		select {
		case creds = <-enrollCh:
			logger.Info("enrollment complete", "device_id", creds.DeviceID)
			enrollServer.Shutdown()
		case err := <-enrollErrCh:
			logger.Error("cannot accept enrollments; exiting", "error", err)
			os.Exit(1)
		case <-ctx.Done():
			logger.Info("agent stopped while waiting for enrollment")
			return
		}
	}

	actionStore, err := store.New(cfg.DataDir)
	if err != nil {
		logger.Error("failed to initialize action store", "error", err)
		os.Exit(1)
	}
	defer actionStore.Close()

	luksDaemon := luksd.NewDaemon(luksd.DefaultSocketPath, actionStore, luksd.NewSysencEnroller(), logger)
	go func() {
		if err := luksDaemon.Start(ctx); err != nil {
			logger.Error("LUKS passphrase daemon failed", "error", err)
		}
	}()

	exec := executor.NewExecutor(runner)
	exec.SetStore(actionStore)
	sched := scheduler.New(ctx, actionStore, exec, logger)
	exec.SetActionStore(sched)

	go sched.Start(ctx)

	syncTrigger := make(chan struct{}, 1)

	h := handler.NewHandler(logger, exec, actionStore, syncTrigger)

	binaryPath, err := os.Executable()
	if err != nil {

		logger.Error("os.Executable failed; self-update DISABLED for this process",
			"error", err,
			"remediation", "run from a path where os.Executable can resolve /proc/self/exe, or disable self-update upstream")
	} else {
		exec.SetUpdateConfig(&executor.AgentUpdateConfig{
			Version:    version,
			DataDir:    cfg.DataDir,
			BinaryPath: binaryPath,
			Shutdown:   cancel,
		})
	}

	if creds.ControlAddr != "" {
	}

	logger.Info("starting agent",
		"control", creds.AgentAddr,
		"device_id", creds.DeviceID,
		"hostname", hostname,
		"version", version,
	)

	runAgent(ctx, credStore, creds, hostname, h, sched, syncTrigger, cfg.pendingSecurityAlert, luksDaemon, logger, time.Now)

	sched.Stop()

	teardownCtx, cancelTeardown := context.WithTimeout(context.Background(), 30*time.Second)
	h.CloseAllTerminals(teardownCtx)
	cancelTeardown()

	h.StopTerminalSweeper()
}

func parseFlags() *Config {
	cfg := &Config{}

	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintln(out, "cadestrod — Cadestro device agent")
		fmt.Fprintln(out, "\nUsage:")
		fmt.Fprintln(out, "  cadestrod [flags]           run the agent (default)")
		fmt.Fprintln(out, "  cadestrod <command> [args]  run a subcommand")
		fmt.Fprintln(out, "\nSubcommands:")
		fmt.Fprintln(out, "  enroll        enroll this device with a control server (token or cadestro:// URI)")
		fmt.Fprintln(out, "  tty           toggle the device-local remote-terminal gate (enable|disable|status)")
		fmt.Fprintln(out, "  luks          LUKS passphrase operations")
		fmt.Fprintln(out, "  query         run a local osquery query")
		fmt.Fprintln(out, "  self-test     run agent self-diagnostics")
		fmt.Fprintln(out, "  install-unit  install/refresh the agent's systemd unit from this binary (root)")
		fmt.Fprintln(out, "  version       print the agent version")
		fmt.Fprintln(out, "\nFlags (default run mode):")
		flag.PrintDefaults()
	}

	var uri string
	flag.StringVar(&uri, "uri", "", "Registration URI (accepted only by the explicit enroll subcommand)")
	flag.StringVar(&cfg.DataDir, "data-dir", credentials.DefaultDataDir, "Data directory for credentials")
	flag.StringVar(&cfg.LogLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	flag.StringVar(&cfg.LogFormat, "log-format", "text", "Log format (text, json)")
	flag.Parse()

	if uri == "" && flag.NArg() > 0 {
		arg := flag.Arg(0)
		if strings.HasPrefix(arg, "cadestro://") {
			uri = arg
		}
	}

	if uri != "" && strings.HasPrefix(uri, "cadestro://luks/") {
		runLuksURI(uri)
	}

	if registrationURIRefusedByHandler(uri) {
		fmt.Fprintln(os.Stderr, "refusing to enroll from a URI handler: enrollment must be explicit. Run:")
		fmt.Fprintf(os.Stderr, "  cadestrod enroll '%s'\n", uri)
		os.Exit(1)
	}

	if v := os.Getenv("CADESTRO_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}

	cfg.PrivilegeBackend = strings.ToLower(os.Getenv("CADESTRO_PRIVILEGE_BACKEND"))

	return cfg
}

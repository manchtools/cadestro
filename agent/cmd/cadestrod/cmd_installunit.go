package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/manchtools/cadestro/agent/internal/credentials"
	"github.com/manchtools/cadestro/agent/internal/unit"
	"github.com/manchtools/cadestro/sdk/logging"
	sysexec "github.com/manchtools/cadestro/sdk/sys/exec"
	"github.com/manchtools/cadestro/sdk/sys/service"
)

func runInstallUnit(args []string) int {
	flags := flag.NewFlagSet("install-unit", flag.ContinueOnError)
	dataDir := flags.String("data-dir", credentials.DefaultDataDir, "Data directory the unit passes to the agent")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	logger := logging.SetupLogger("info", "text", os.Stderr)

	if os.Geteuid() != 0 {
		logger.Error("install-unit must run as root (it writes /etc/systemd/system and reloads systemd)")
		return 1
	}

	ctx := context.Background()
	if !service.Available() {

		logger.Error("no usable systemd detected on this host; the agent's unit cannot be installed")
		return 1
	}

	runner, err := sysexec.NewRunner(sysexec.Direct)
	if err != nil {
		logger.Error("failed to build runner", "error", err)
		return 1
	}
	mgr, err := service.New(runner)
	if err != nil {
		logger.Error("failed to build service manager", "error", err)
		return 1
	}

	binaryPath, err := os.Executable()
	if err != nil {
		logger.Error("failed to resolve own executable path", "error", err)
		return 1
	}

	if err := unit.EnsureInstalled(ctx, mgr, logger, unit.Params{BinaryPath: binaryPath, DataDir: *dataDir}); err != nil {
		logger.Error("unit install failed", "unit", unit.UnitName, "error", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "unit %s installed (data-dir=%s)\n", unit.UnitName, *dataDir)
	return 0
}

func reconcileUnitAtStartup(ctx context.Context, runner sysexec.Runner, logger *slog.Logger, dataDir string) {
	if os.Geteuid() != 0 {
		logger.Debug("skipping unit reconcile: not running as root")
		return
	}
	if !service.Available() {
		logger.Debug("skipping unit reconcile: no usable systemd detected")
		return
	}
	mgr, err := service.New(runner)
	if err != nil {
		logger.Error("unit reconcile: failed to build service manager; agent continues with the on-disk unit", "error", err)
		return
	}
	binaryPath, err := os.Executable()
	if err != nil {
		logger.Error("unit reconcile: failed to resolve own executable path; agent continues with the on-disk unit", "error", err)
		return
	}
	drifted, err := unit.Reconcile(ctx, mgr, logger, unit.Params{BinaryPath: binaryPath, DataDir: dataDir})
	if err != nil {
		logger.Error("unit reconcile failed; agent continues with the on-disk unit", "unit", unit.UnitName, "error", err)
		return
	}
	if drifted {
		logger.Error("systemd unit was STALE and has been rewritten from the embedded template; the new settings apply at the next service restart or reboot",
			"unit", unit.UnitName)
	}
}

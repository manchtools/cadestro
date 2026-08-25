package main

import (
	"context"
	"flag"
	"os"
	"strings"
	"time"

	"github.com/manchtools/cadestro/agent/internal/credentials"
	sdk "github.com/manchtools/cadestro/contract"
	"github.com/manchtools/cadestro/sdk/logging"
)

func runSelfTest(args []string) int {
	fs := flag.NewFlagSet("self-test", flag.ExitOnError)
	dataDir := fs.String("data-dir", credentials.DefaultDataDir, "Agent data directory")
	timeout := fs.Duration("timeout", 60*time.Second, "Self-test timeout")
	fs.Parse(args)

	logger := logging.SetupLogger("info", "text", os.Stderr)

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	credStore := credentials.NewStore(*dataDir)
	if !credStore.Exists() {
		logger.Error("self-test: no credentials found", "data_dir", *dataDir)
		return 1
	}
	creds, err := credStore.Load()
	if err != nil {
		logger.Error("self-test: failed to load credentials", "error", err)
		return 1
	}
	logger.Info("self-test: credentials loaded", "device_id", creds.DeviceID)

	controlAddr := strings.TrimSpace(creds.AgentAddr)
	if err := requireHTTPSAgentAddr(creds.AgentAddr); err != nil {
		logger.Error("self-test: refusing control URL", "control", creds.AgentAddr, "error", err)
		return 1
	}
	mtlsOpt, err := sdk.WithMTLSFromPEM(creds.Certificate, creds.PrivateKey, creds.CACert)
	if err != nil {
		logger.Error("self-test: failed to configure mTLS", "error", err)
		return 1
	}
	client := sdk.NewClient(controlAddr,
		mtlsOpt,
		sdk.WithAuth(creds.DeviceID, ""),
	)

	if err := client.Connect(ctx); err != nil {
		logger.Error("self-test: failed to connect to control", "error", err)
		return 1
	}
	defer client.Close()

	hostname, _ := os.Hostname()
	if err := client.SendHello(ctx, hostname, version); err != nil {
		logger.Error("self-test: failed to send hello", "error", err)
		return 1
	}

	msg, err := client.Receive(ctx)
	if err != nil {
		logger.Error("self-test: failed to receive welcome", "error", err)
		return 1
	}
	if msg.GetWelcome() == nil {
		logger.Error("self-test: expected welcome message, got something else")
		return 1
	}
	logger.Info("self-test: stream connected, welcome received",
		"server_version", msg.GetWelcome().ServerVersion)

	stopReceiver := client.StartReceiver(ctx)
	defer stopReceiver()
	_, err = client.Sync(ctx)
	if err != nil {
		logger.Error("self-test: stream synchronization failed", "error", err)
		return 1
	}
	logger.Info("self-test: stream synchronization succeeded")

	logger.Info("self-test: all checks passed")
	return 0
}

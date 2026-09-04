package main

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/manchtools/cadestro/agent/internal/credentials"
	"github.com/manchtools/cadestro/agent/internal/scheduler"
	sdk "github.com/manchtools/cadestro/contract"
)

func reloadCredsForReconnect(credStore *credentials.Store, current *credentials.Credentials, logger *slog.Logger) *credentials.Credentials {
	reloaded, err := credStore.Load()
	if err != nil {
		logger.Warn("cert reload: failed to reload credentials before reconnect; using in-memory copy", "error", err)
		return current
	}
	return reloaded
}

func waitForReadiness(ctx context.Context, cancel context.CancelFunc, readiness <-chan struct{}, timeout time.Duration) bool {
	readinessCtx, cancelReadiness := context.WithTimeout(ctx, timeout)
	defer cancelReadiness()
	select {
	case <-readiness:
		return true
	case <-readinessCtx.Done():
		cancel()
		return false
	}
}

func runAgent(ctx context.Context, credStore *credentials.Store, creds *credentials.Credentials, hostname string, scheduler *scheduler.Scheduler, logger *slog.Logger, now func() time.Time) {
	backoff := randomBackoff()
	fallbackActive := false
	firstConnect := true

	for ctx.Err() == nil {
		if !firstConnect {
			creds = reloadCredsForReconnect(credStore, creds, logger)
		}
		firstConnect = false
		readiness := make(chan struct{}, 1)

		if err := requireHTTPSAgentAddr(creds.AgentAddr); err != nil {
			logger.Error("refusing invalid control URL", "control", creds.AgentAddr, "error", err)
			return
		}
		mtlsOption, usingPending, pendingInvalid, err := configureAgentMTLS(creds, fallbackActive)
		if err != nil {
			if pendingInvalid {
				fallbackActive = true
				logger.Warn("pending certificate is unusable; falling back to the active certificate", "error", err)
				continue
			}
			logger.Error("configure mTLS", "error", err)
			return
		}

		sessionCtx, cancelSession := context.WithCancel(ctx)
		client := sdk.NewClient(strings.TrimSpace(creds.AgentAddr), mtlsOption, sdk.WithDeviceID(creds.DeviceID), sdk.WithLogger(logger))
		streamDone := make(chan error, 1)
		go func() { streamDone <- client.Run(sessionCtx, hostname, version, readiness) }()

		connected := waitForReadiness(sessionCtx, cancelSession, readiness, 30*time.Second)
		staged := false
		if connected && usingPending {
			creds.Certificate = append([]byte(nil), creds.PendingCertificate...)
			creds.PendingCertificate = nil
			if err := credStore.Save(sessionCtx, creds); err != nil {
				logger.Warn("persist promoted certificate", "error", err)
			}
			fallbackActive = false
		}
		if connected && !usingPending {
			staged, err = renewCertificateIfDue(sessionCtx, credStore, creds, hostname, logger, now, len(creds.PendingCertificate) > 0)
			if err != nil {
				logger.Warn("certificate renewal", "error", err)
			}
			if staged {
				cancelSession()
			}
		}

		var workers sync.WaitGroup
		if connected && !staged {
			interval := pullDesiredPolicyFromControl(sessionCtx, client, scheduler, logger)
			workers.Go(func() { periodicSync(sessionCtx, client, scheduler, interval, logger) })
			workers.Go(func() { sendScheduledResults(sessionCtx, client, scheduler, logger) })
		}

		started := now()
		streamErr := <-streamDone
		fallbackActive = fallbackAfterConnection(len(creds.PendingCertificate) > 0, usingPending, connected)
		cancelSession()
		workers.Wait()
		client.CloseIdleConnections()
		if ctx.Err() != nil {
			return
		}
		if now().Sub(started) > backoff {
			backoff = randomBackoff()
		}
		logger.Warn("connection lost; scheduled actions remain active", "error", streamErr, "backoff", backoff)
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		backoff = min(backoff*2, maxBackoff)
	}
}

func periodicSync(ctx context.Context, client *sdk.Client, scheduler *scheduler.Scheduler, interval time.Duration, logger *slog.Logger) {
	if interval <= 0 {
		interval = defaultPolicyRefreshInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if updated := pullDesiredPolicyFromControl(ctx, client, scheduler, logger); updated > 0 && updated != interval {
				interval = updated
				ticker.Reset(interval)
			}
		}
	}
}

func sendScheduledResults(ctx context.Context, client *sdk.Client, scheduler *scheduler.Scheduler, logger *slog.Logger) {
	syncPendingResults(ctx, scheduler, client, logger)
	for {
		select {
		case <-ctx.Done():
			return
		case <-scheduler.ResultsReady():
			syncPendingResults(ctx, scheduler, client, logger)
		}
	}
}

func pullDesiredPolicyFromControl(ctx context.Context, client *sdk.Client, scheduler *scheduler.Scheduler, logger *slog.Logger) time.Duration {
	policy, err := client.PullDesiredPolicy(ctx)
	if err != nil {
		logger.Warn("pull desired state", "error", err)
		return 0
	}
	if err := scheduler.ReconcilePolicy(ctx, policy); err != nil {
		logger.Warn("reconcile desired state", "error", err)
	}
	return time.Duration(policy.GetRefreshIntervalMinutes()) * time.Minute
}

func syncPendingResults(ctx context.Context, scheduler *scheduler.Scheduler, client *sdk.Client, logger *slog.Logger) {
	results, err := scheduler.GetPendingResults(ctx)
	if err != nil {
		logger.Warn("load pending results", "error", err)
		return
	}
	for _, result := range results {
		if err := client.SendActionResult(ctx, result.ActionResult); err != nil {
			if errors.Is(err, sdk.ErrResultRejected) {
				logger.Warn("drop rejected result", "sequence", result.Sequence, "error", err)
				if err := scheduler.DeletePendingResult(ctx, result.Sequence); err != nil {
					logger.Warn("delete rejected result", "sequence", result.Sequence, "error", err)
					return
				}
				continue
			}
			logger.Warn("send pending result", "sequence", result.Sequence, "error", err)
			return
		}
		if err := scheduler.DeletePendingResult(ctx, result.Sequence); err != nil {
			logger.Warn("delete pending result", "sequence", result.Sequence, "error", err)
			return
		}
	}
}

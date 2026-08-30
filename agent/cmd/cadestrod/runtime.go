package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/manchtools/cadestro/agent/internal/credentials"
	"github.com/manchtools/cadestro/agent/internal/handler"
	"github.com/manchtools/cadestro/agent/internal/scheduler"
	sdk "github.com/manchtools/cadestro/contract"
	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

func reloadCredsForReconnect(credStore *credentials.Store, current *credentials.Credentials, logger *slog.Logger) *credentials.Credentials {
	reloaded, err := credStore.Load()
	if err != nil {
		logger.Warn("cert reload: failed to reload credentials before reconnect; using in-memory copy", "error", err)
		return current
	}
	return reloaded
}

func waitForWelcome(ctx context.Context, cancel context.CancelFunc, wait func(context.Context) error, timeout time.Duration) error {
	welcomeCtx, cancelWelcome := context.WithTimeout(ctx, timeout)
	defer cancelWelcome()
	err := wait(welcomeCtx)
	if err != nil {
		cancel()
	}
	return err
}

func runAgent(ctx context.Context, credStore *credentials.Store, creds *credentials.Credentials, hostname string, handler *handler.Handler, scheduler *scheduler.Scheduler, logger *slog.Logger, now func() time.Time) {
	backoff := randomBackoff()
	fallbackActive := false
	firstConnect := true

	for ctx.Err() == nil {
		if !firstConnect {
			creds = reloadCredsForReconnect(credStore, creds, logger)
		}
		firstConnect = false
		handler.ResetConnection()

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
		client := sdk.NewClient(strings.TrimSpace(creds.AgentAddr), mtlsOption, sdk.WithAuth(creds.DeviceID, ""), sdk.WithLogger(logger))
		streamDone := make(chan error, 1)
		go func() { streamDone <- client.Run(sessionCtx, hostname, version, defaultHeartbeatInterval, handler) }()

		connected := waitForWelcome(sessionCtx, cancelSession, handler.WaitConnected, defaultHeartbeatInterval) == nil
		staged := false
		if connected && usingPending {
			creds.Certificate = append([]byte(nil), creds.PendingCertificate...)
			creds.PendingCertificate = nil
			if err := credStore.Save(creds); err != nil {
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

		var workers syncWorkers
		if connected && !staged {
			interval := pullDesiredPolicyFromControl(sessionCtx, client, scheduler, logger)
			syncPendingResults(sessionCtx, scheduler, client, logger)
			workers.start(sessionCtx, client, scheduler, interval, logger)
		}

		started := now()
		streamErr := <-streamDone
		fallbackActive = fallbackAfterConnection(len(creds.PendingCertificate) > 0, usingPending, connected)
		cancelSession()
		workers.wait()
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

type syncWorkers struct {
	done []<-chan struct{}
}

func (workers *syncWorkers) start(ctx context.Context, client *sdk.Client, scheduler *scheduler.Scheduler, interval time.Duration, logger *slog.Logger) {
	for _, run := range []func(){
		func() { periodicSync(ctx, client, scheduler, interval, logger) },
		func() { sendScheduledResults(ctx, client, scheduler, logger) },
	} {
		done := make(chan struct{})
		workers.done = append(workers.done, done)
		go func() {
			defer close(done)
			run()
		}()
	}
}

func (workers *syncWorkers) wait() {
	for _, done := range workers.done {
		<-done
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
	for {
		select {
		case <-ctx.Done():
			return
		case result := <-scheduler.Results():
			if err := sendResult(ctx, client, result.ActionResult, result.ManifestResult); err != nil {
				logger.Warn("send scheduled result", "result_id", result.ResultID, "error", err)
				continue
			}
			if err := scheduler.MarkPendingResultSynced(ctx, result.ResultID); err != nil {
				logger.Warn("mark result synced", "result_id", result.ResultID, "error", err)
			}
		}
	}
}

func pullDesiredPolicyFromControl(ctx context.Context, client *sdk.Client, scheduler *scheduler.Scheduler, logger *slog.Logger) time.Duration {
	policy, err := client.PullDesiredPolicy(ctx)
	if err != nil {
		logger.Warn("pull desired state", "error", err)
		return 0
	}
	if policy != nil {
		if err := scheduler.ReconcilePolicy(ctx, policy); err != nil {
			logger.Warn("reconcile desired state", "error", err)
		}
	}
	if policy.GetRefreshIntervalMinutes() <= 0 {
		return defaultPolicyRefreshInterval
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
		if err := sendResult(ctx, client, result.ActionResult, result.ManifestResult); err != nil {
			logger.Warn("send pending result", "result_id", result.ID, "error", err)
			return
		}
		if err := scheduler.MarkPendingResultSynced(ctx, result.ID); err != nil {
			logger.Warn("mark pending result synced", "result_id", result.ID, "error", err)
			return
		}
	}
}

func sendResult(ctx context.Context, client *sdk.Client, action *cadestrov1.ActionResult, manifest *cadestrov1.ManifestResult) error {
	switch {
	case action != nil && manifest == nil:
		return client.SendActionResult(ctx, action)
	case manifest != nil && action == nil:
		return client.SendManifestResult(ctx, manifest)
	default:
		return fmt.Errorf("result outbox entry must contain exactly one payload")
	}
}

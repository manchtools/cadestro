package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/manchtools/cadestro/agent/internal/credentials"
	"github.com/manchtools/cadestro/agent/internal/handler"
	"github.com/manchtools/cadestro/agent/internal/luksd"
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

func runAgent(ctx context.Context, credStore *credentials.Store, creds *credentials.Credentials, hostname string, h *handler.Handler, sched *scheduler.Scheduler, syncTrigger <-chan struct{}, securityAlert *pendingSecurityAlert, luksDaemon *luksd.Daemon, logger *slog.Logger, now func() time.Time) {

	syncInterval := defaultSyncInterval

	currentBackoff := randomBackoff()

	firstConnect := true
	fallbackActive := false

	for {
		if !firstConnect {
			creds = reloadCredsForReconnect(credStore, creds, logger)
		}
		firstConnect = false

		h.ResetConnection()

		if err := requireHTTPSAgentAddr(creds.AgentAddr); err != nil {
			logger.Error("refusing stream URL — re-enrol against an https:// control server or delete the cached credentials",
				"agent_addr", creds.AgentAddr, "error", err)
			os.Exit(1)
		}

		sessionCtx, cancelSession := context.WithCancel(ctx)

		mtlsOpt, usingPending, pendingConfigFailed, err := configureAgentMTLS(creds, fallbackActive)
		if err != nil {
			if pendingConfigFailed {

				logger.Warn("pending certificate unusable; falling back to active certificate", "error", err)
				fallbackActive = true
				cancelSession()
				timer := time.NewTimer(currentBackoff)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
				continue
			}
			logger.Error("failed to configure mTLS", "error", err)
			os.Exit(1)
		}
		client := sdk.NewClient(strings.TrimSpace(creds.AgentAddr),
			mtlsOpt,
			sdk.WithAuth(creds.DeviceID, ""),
		)

		luksStore := &clientLuksKeyStore{client: client, executor: h.Executor()}

		h.SetTerminalSender(client)

		streamDone := make(chan error, 1)
		go func() {
			defer cancelSession()
			streamDone <- client.Run(sessionCtx, hostname, version, defaultHeartbeatInterval, h)
		}()

		connected := waitForWelcome(sessionCtx, cancelSession, h.WaitConnected, defaultHeartbeatInterval) == nil
		staged := false
		if connected {
			if !usingPending {

				fallbackActive = false
				var renewErr error
				staged, renewErr = renewCertificateIfDue(sessionCtx, credStore, creds, hostname, logger, now, len(creds.PendingCertificate) > 0)
				if renewErr != nil {
					logger.Warn("certificate renewal check failed", "error", renewErr)
				} else if staged {
					cancelSession()
				}
			}
			if usingPending {

				creds.Certificate = append([]byte(nil), creds.PendingCertificate...)
				creds.PendingCertificate = nil
				if err := credStore.Save(creds); err != nil {
					logger.Warn("certificate promotion: failed to persist active bundle", "error", err)
				}
			}
			if staged {

			} else {
				h.Executor().SetLuksKeyStore(luksStore)
				h.Executor().SetLpsPasswordStore(&clientLpsPasswordStore{client: client})
				if luksDaemon != nil {
					luksDaemon.SetSession(luksStore)
				}

				if newInterval := syncStateFromControl(sessionCtx, client, sched, logger); newInterval > 0 {
					syncInterval = newInterval
				}
				syncPendingResults(sessionCtx, sched, client, logger)

				if securityAlert != nil {
					go sendSecurityAlert(sessionCtx, client, securityAlert, logger)
					securityAlert = nil
				}
			}
		}

		intervalUpdatesOut := make(chan time.Duration, 1)
		syncDone := make(chan struct{})
		go func() {
			defer close(syncDone)
			var beforeSync func() bool
			if !usingPending && !staged {
				beforeSync = func() bool {
					staged, renewErr := renewCertificateIfDue(sessionCtx, credStore, creds, hostname, logger, now, len(creds.PendingCertificate) > 0)
					if renewErr != nil {
						logger.Warn("certificate renewal check failed", "error", renewErr)
						return false
					}
					if staged {
						cancelSession()
						return true
					}
					return false
				}
			}
			periodicSync(sessionCtx, client, sched, syncInterval, intervalUpdatesOut, syncTrigger, logger, beforeSync)
		}()

		resultsDone := make(chan struct{})
		go func() {
			defer close(resultsDone)
			sendScheduledResults(sessionCtx, client, sched, logger)
		}()

		connStart := now()
		streamErr := waitForStreamEnd(streamDone, intervalUpdatesOut, &syncInterval)
		err = streamErr

		fallbackActive = fallbackAfterConnection(len(creds.PendingCertificate) > 0, usingPending, connected)

		cancelSession()
		h.Executor().SetLuksKeyStore(nil)

		h.Executor().SetLpsPasswordStore(nil)
		if luksDaemon != nil {
			luksDaemon.ClearSession()
		}
		<-syncDone
		<-resultsDone

		client.CloseIdleConnections()

		select {
		case updated := <-intervalUpdatesOut:
			syncInterval = updated
		default:
		}

		if ctx.Err() != nil {
			logger.Info("agent stopped")
			return
		}

		if now().Sub(connStart) > currentBackoff {
			currentBackoff = randomBackoff()
		}

		logger.Error("connection lost, continuing with scheduled actions",
			"error", err,
			"backoff", currentBackoff.String(),
		)

		select {
		case <-ctx.Done():
			logger.Info("agent stopped during backoff")
			return
		case <-time.After(currentBackoff):
		}

		currentBackoff = time.Duration(float64(currentBackoff) * backoffFactor)
		if currentBackoff > maxBackoff {
			currentBackoff = maxBackoff
		}
	}
}

func waitForStreamEnd(streamDone <-chan error, intervalUpdatesOut <-chan time.Duration, interval *time.Duration) error {
	for {
		select {
		case err := <-streamDone:
			return err
		case updated := <-intervalUpdatesOut:
			*interval = updated
		}
	}
}

func periodicSync(
	ctx context.Context,
	client *sdk.Client,
	sched *scheduler.Scheduler,
	initialInterval time.Duration,
	intervalUpdatesOut chan<- time.Duration,
	syncTrigger <-chan struct{},
	logger *slog.Logger,
	beforeSync func() bool,
) {
	syncInterval := initialInterval
	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()

	logger.Info("periodic sync started", "interval", syncInterval.String())

	doSync := func(reason string) {
		if beforeSync != nil && beforeSync() {
			return
		}
		logger.Info("synchronizing stream state", "reason", reason)
		newInterval := syncStateFromControl(ctx, client, sched, logger)
		if newInterval > 0 && newInterval != syncInterval {
			syncInterval = newInterval
			ticker.Reset(syncInterval)
			logger.Info("sync interval updated", "new_interval", syncInterval.String())

			select {
			case intervalUpdatesOut <- syncInterval:
			default:
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			logger.Debug("periodic sync stopped")
			return
		case <-ticker.C:
			doSync("periodic")
		case <-syncTrigger:
			doSync("live sync trigger")
		}
	}
}

func sendScheduledResults(ctx context.Context, client *sdk.Client, sched *scheduler.Scheduler, logger *slog.Logger) {
	for {
		select {
		case <-ctx.Done():
			return
		case result, ok := <-sched.Results():
			if !ok {
				return
			}

			if err := sendResult(ctx, client, result.ActionResult, result.ManifestResult); err != nil {
				logger.Warn("failed to send scheduled result", "result_id", result.ResultID, "error", err)
				continue
			}
			if err := sched.MarkPendingResultSynced(ctx, result.ResultID); err != nil {
				logger.Warn("failed to mark result synced", "result_id", result.ResultID, "error", err)
			}
		}
	}
}

func syncStateFromControl(ctx context.Context, client *sdk.Client, sched *scheduler.Scheduler, logger *slog.Logger) time.Duration {
	logger.Info("synchronizing state from control")

	result, err := client.Sync(ctx)
	if err != nil {
		logger.Warn("failed to synchronize state from control", "error", err)
		return 0
	}

	sched.SetMaintenanceWindow(ctx, result.MaintenanceWindow)
	if result.DesiredPolicy != nil {
		if err := sched.ReconcilePolicy(ctx, result.DesiredPolicy); err != nil {
			logger.Warn("failed to reconcile assigned policy", "error", err)
		}
	}

	var syncInterval time.Duration
	if result.SyncIntervalMinutes > 0 {
		syncInterval = time.Duration(result.SyncIntervalMinutes) * time.Minute
	} else {
		syncInterval = defaultSyncInterval
	}

	logger.Info("manifests synced from server", "sync_interval", syncInterval.String())

	return syncInterval
}

func syncPendingResults(ctx context.Context, sched *scheduler.Scheduler, client *sdk.Client, logger *slog.Logger) {
	results, err := sched.GetPendingResults(ctx)
	if err != nil {
		logger.Warn("failed to get unsynced results", "error", err)
		return
	}

	if len(results) == 0 {
		return
	}

	logger.Info("syncing pending results", "count", len(results))

	for _, r := range results {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := sendResult(ctx, client, r.ActionResult, r.ManifestResult); err != nil {
			logger.Warn("failed to send pending result", "result_id", r.ID, "error", err)
			continue
		}
		if err := sched.MarkPendingResultSynced(ctx, r.ID); err != nil {
			logger.Warn("failed to mark result synced", "result_id", r.ID, "error", err)
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

func sendSecurityAlert(ctx context.Context, client *sdk.Client, alert *pendingSecurityAlert, logger *slog.Logger) {

	select {
	case <-ctx.Done():
		return
	case <-time.After(2 * time.Second):
	}

	logger.Info("sending security alert to server",
		"type", alert.alertType,
		"message", alert.message,
	)

	var alertType cadestrov1.SecurityAlertType
	switch alert.alertType {
	case "server_reassignment_attempt":
		alertType = cadestrov1.SecurityAlertType_SECURITY_ALERT_TYPE_SERVER_REASSIGNMENT_ATTEMPT
	default:
		alertType = cadestrov1.SecurityAlertType_SECURITY_ALERT_TYPE_UNSPECIFIED
	}

	protoAlert := &cadestrov1.SecurityAlert{
		Type:    alertType,
		Message: alert.message,
		Details: map[string]string{
			"requested_server":  alert.requestedServer,
			"registered_server": alert.registeredServer,
		},
	}

	if err := client.SendSecurityAlert(ctx, protoAlert); err != nil {
		logger.Warn("failed to send security alert", "error", err)
	} else {
		logger.Debug("security alert sent successfully")
	}
}

package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/manchtools/cadestro/server/internal/backupstatus"
	"github.com/manchtools/cadestro/server/internal/jobs"
	"github.com/manchtools/cadestro/server/internal/store"
	"github.com/manchtools/cadestro/server/internal/webhook"
)

const (
	KindAuthStateCleanup = "identity.auth_state_cleanup"
	KindBackupInspect    = "storage.backup_inspect"
	KindSecurityInspect  = "security.inspect"

	authStateCleanupInterval = time.Hour
	securityInspectInterval  = 15 * time.Minute
	backupInspectInterval    = 15 * time.Minute
	maintenanceMaxAttempts   = int32(100)
)

var recurring = map[string]time.Duration{
	KindAuthStateCleanup: authStateCleanupInterval,
	KindBackupInspect:    backupInspectInterval,
	KindSecurityInspect:  securityInspectInterval,
}

type Notifier interface {
	Send(context.Context, webhook.Event) error
}

type Config struct {
	Store        *store.Store
	Now          func() time.Time
	Notifier     Notifier
	BackupPath   string
	BackupMaxLag time.Duration
}

type Service struct {
	store        *store.Store
	now          func() time.Time
	notifier     Notifier
	backupPath   string
	backupMaxLag time.Duration
}

func New(cfg Config) *Service {
	if cfg.Store == nil || cfg.BackupPath == "" || cfg.BackupMaxLag <= 0 {
		panic("maintenance: store and backup policy are required")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Service{
		store: cfg.Store,
		now:   cfg.Now, notifier: cfg.Notifier,
		backupPath: cfg.BackupPath, backupMaxLag: cfg.BackupMaxLag,
	}
}

func (s *Service) Handlers() map[string]jobs.Handler {
	return map[string]jobs.Handler{
		KindAuthStateCleanup: s.CleanupAuthStates,
		KindBackupInspect:    s.InspectBackup,
		KindSecurityInspect:  s.InspectSecurity,
	}
}

func (s *Service) Recurring() map[string]time.Duration {
	out := make(map[string]time.Duration, len(recurring))
	for kind, interval := range recurring {
		out[kind] = interval
	}
	return out
}

func (s *Service) EnsureScheduled(ctx context.Context) error {
	if ctx == nil {
		return errors.New("maintenance scheduling requires a context")
	}
	kinds := make([]string, 0, len(recurring))
	for kind := range recurring {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		if _, err := s.store.GetLiveJobByDedupe(ctx, kind); err == nil {
			continue
		} else if !store.IsNotFound(err) {
			return err
		}
		opID := ulid.Make().String()
		op := backgroundOperation(opID, "maintenance.schedule."+kind)
		_, err := s.store.WithAudit(ctx, op, func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
			_, err := jobs.InsertInTx(ctx, tx, rec, jobs.InsertParams{
				OperationID: opID, Kind: kind, Payload: json.RawMessage(`{}`), DueAt: s.now().UTC(),
				MaxAttempts: maintenanceMaxAttempts, DedupeKey: kind,
			})
			return err
		})
		if err != nil {
			return fmt.Errorf("schedule %s: %w", kind, err)
		}
	}
	return nil
}

func (s *Service) CleanupAuthStates(ctx context.Context, _ jobs.Job) error {
	_, err := s.store.CleanupExpiredAuthStates(ctx)
	return err
}

func (s *Service) InspectSecurity(ctx context.Context, _ jobs.Job) error {
	if ctx == nil {
		return errors.New("security inspection requires a context")
	}
	if s.notifier == nil {
		return nil
	}
	var count int64
	opID := ulid.Make().String()
	_, err := s.store.WithAudit(ctx, backgroundOperation(opID, "maintenance.security.inspect"),
		func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
			var err error
			count, err = tx.CountEnabledUnscopedAdmins(ctx)
			if err != nil {
				return err
			}
			rec.Effect(store.AuditEffect{
				ResourceType: "security_posture", ResourceID: opID,
				Action: "INSPECT", Outcome: store.EffectApplied, AfterCount: &count,
			})
			if count == 0 {
				rec.Effect(store.AuditEffect{
					ResourceType: "webhook", ResourceID: opID,
					Action: "NOTIFY_INTENT", Outcome: store.EffectApplied,
				})
			}
			return nil
		})
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return s.notifier.Send(ctx, webhook.Event{
		Name: webhook.EventZeroEnabledAdministrators, OccurredAt: s.now().UTC(),
	})
}

func (s *Service) InspectBackup(ctx context.Context, _ jobs.Job) error {
	if ctx == nil {
		return errors.New("backup inspection requires a context")
	}
	status, readErr := backupstatus.Read(s.backupPath, s.now().UTC(), s.backupMaxLag)
	stale := readErr != nil || status.Stale
	opID := ulid.Make().String()
	_, err := s.store.WithAudit(ctx, backgroundOperation(opID, "maintenance.backup.inspect"),
		func(_ context.Context, _ *store.Tx, rec *store.AuditRecorder) error {
			rec.Effect(store.AuditEffect{
				ResourceType: "backup_posture", ResourceID: opID,
				Action: "INSPECT", Outcome: store.EffectApplied,
				AfterFlag: &stale, AfterCount: status.LagSeconds,
			})
			if stale && s.notifier != nil {
				rec.Effect(store.AuditEffect{
					ResourceType: "webhook", ResourceID: opID,
					Action: "NOTIFY_INTENT", Outcome: store.EffectApplied,
				})
			}
			return nil
		})
	if err != nil {
		return err
	}
	if !stale || s.notifier == nil {
		return nil
	}
	return s.notifier.Send(ctx, webhook.Event{Name: webhook.EventBackupLag, OccurredAt: s.now().UTC()})
}

func backgroundOperation(id, descriptor string) store.AuditOperation {
	return store.AuditOperation{
		OperationID: id, Class: store.ClassBackgroundWriter, ActorType: "control_worker",
		Origin: "in_process", RequestDescriptor: descriptor,
		AuthorizationOutcome: store.AuthorizationNotApplicable,
		Result:               store.ResultSuccess, ResultCode: "OK",
	}
}

// Package scheduler executes durably received manifests on the agent.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/manchtools/cadestro/agent/internal/store"
	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/maintenance"
)

const DefaultCheckInterval = time.Minute

const maintenanceWindowSettingKey = "maintenance_window"

type ActionExecutor interface {
	ExecuteAction(context.Context, *pb.Action) *pb.ActionResult
	ResetUpdateCycle()
}

type ExecutionResult struct {
	ResultID       string
	ActionResult   *pb.ActionResult
	ManifestResult *pb.ManifestResult
}

type Scheduler struct {
	store    *store.Store
	executor ActionExecutor
	logger   *slog.Logger
	now      func() time.Time
	wakeCh   chan struct{}
	results  chan *ExecutionResult

	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
	done    chan struct{}

	windowMu           sync.RWMutex
	window             *pb.MaintenanceWindow
	windowDecodeFailed bool
}

func New(ctx context.Context, st *store.Store, executor ActionExecutor, logger *slog.Logger) *Scheduler {
	s := &Scheduler{
		store:    st,
		executor: executor,
		logger:   logger,
		now:      time.Now,
		wakeCh:   make(chan struct{}, 1),
		results:  make(chan *ExecutionResult, 100),
	}
	if window, err := loadMaintenanceWindow(ctx, st); err != nil {
		logger.Error("persisted maintenance window is unreadable; denying scheduled policy runs until sync", "error", err)
		s.windowDecodeFailed = true
	} else {
		s.window = window
	}
	return s
}

func (s *Scheduler) Results() <-chan *ExecutionResult { return s.results }

// ReconcilePolicy replaces assignment-derived work. Policy snapshots arrive
// through authenticated Sync.
func (s *Scheduler) ReconcilePolicy(ctx context.Context, policy *pb.DesiredPolicy) error {
	if err := s.store.ReconcilePolicy(ctx, policy); err != nil {
		return err
	}
	s.Wake()
	return nil
}

func (s *Scheduler) GetPendingResults(ctx context.Context) ([]store.PendingResult, error) {
	return s.store.GetPendingResults(ctx)
}

func (s *Scheduler) MarkPendingResultSynced(ctx context.Context, id string) error {
	return s.store.MarkPendingResultSynced(ctx, id)
}

func (s *Scheduler) Wake() {
	select {
	case s.wakeCh <- struct{}{}:
	default:
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	if prior := s.done; prior != nil {
		s.mu.Unlock()
		<-prior
		s.mu.Lock()
		if s.running {
			s.mu.Unlock()
			return
		}
	}
	s.running = true
	s.stopCh = make(chan struct{})
	s.done = make(chan struct{})
	stopCh, done := s.stopCh, s.done
	s.mu.Unlock()
	defer close(done)

	if err := s.recoverInterruptedOccurrences(ctx); err != nil {
		s.logger.Error("failed to recover interrupted occurrences; refusing to schedule", "error", err)
		return
	}
	ticker := time.NewTicker(DefaultCheckInterval)
	defer ticker.Stop()
	s.runDue(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-stopCh:
			return
		case <-ticker.C:
			s.runDue(ctx)
		case <-s.wakeCh:
			s.runDue(ctx)
		}
	}
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	close(s.stopCh)
	s.running = false
	done := s.done
	s.mu.Unlock()
	if done != nil {
		<-done
	}
}

func (s *Scheduler) runDue(ctx context.Context) {
	if err := s.recoverInterruptedOccurrences(ctx); err != nil {
		s.logger.Error("recover interrupted occurrences", "error", err)
		return
	}
	workItems, err := s.store.GetDueScheduledWork(ctx)
	if err != nil {
		s.logger.Error("load due manifests", "error", err)
		return
	}
	allowed := s.runAllowed(s.now().Local())
	for _, stored := range workItems {
		if ctx.Err() != nil {
			return
		}
		if !allowed {
			continue
		}
		s.executeManifest(ctx, stored)
	}
}

func (s *Scheduler) recoverInterruptedOccurrences(ctx context.Context) error {
	recovered, recoverErr := s.store.RecoverInterruptedOccurrences(ctx)
	if recoverErr != nil {
		return recoverErr
	}
	for _, result := range recovered {
		s.publish(&ExecutionResult{ResultID: result.ID, ActionResult: result.ActionResult})
	}
	return nil
}

func (s *Scheduler) executeManifest(ctx context.Context, work store.ScheduledWork) {
	manifest := work.Manifest
	started, err := s.store.BeginManifestRun(ctx, &work, s.now().UTC())
	if err != nil {
		s.logger.Error("begin manifest run", "work_id", work.RunID, "error", err)
		return
	}
	states, err := s.store.GetManifestOccurrenceStates(ctx, work.RunID)
	if err != nil {
		s.logger.Error("load occurrence states", "work_id", work.RunID, "error", err)
		return
	}
	s.executor.ResetUpdateCycle()
	aggregate := pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS
	var aggregateError string
	stop := false
	for _, occurrence := range manifest.GetOccurrences() {
		if ctx.Err() != nil {
			return
		}
		action := occurrence.GetAction()
		if action == nil || action.GetId() == nil {
			aggregate = pb.ExecutionStatus_EXECUTION_STATUS_FAILED
			aggregateError = "manifest contains a malformed occurrence"
			break
		}
		if prior, exists := states[occurrence.GetOccurrenceId()]; exists && prior.State != store.OccurrencePending {
			if prior.State == store.OccurrenceStarted {
				return // a scheduled reboot is still waiting for its boot marker
			}
			aggregate, aggregateError = aggregateStatus(aggregate, aggregateError, prior.ResultStatus, prior.ResultError)
			if prior.ResultStatus != pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS &&
				prior.ResultStatus != pb.ExecutionStatus_EXECUTION_STATUS_NOT_APPLICABLE &&
				prior.ResultStatus != pb.ExecutionStatus_EXECUTION_STATUS_SKIPPED &&
				occurrence.GetOnFailure() == pb.OnFailure_ON_FAILURE_STOP {
				stop = true
			}
			continue
		}

		if err := s.store.MarkOccurrenceStarted(ctx, work.RunID, occurrence.GetOccurrenceId(), s.now()); err != nil {
			s.logger.Error("mark occurrence started", "work_id", work.RunID, "occurrence_id", occurrence.GetOccurrenceId(), "error", err)
			aggregate = pb.ExecutionStatus_EXECUTION_STATUS_FAILED
			aggregateError = "failed to durably mark occurrence STARTED"
			break
		}

		var result *pb.ActionResult
		if stop {
			result = &pb.ActionResult{
				ActionId:    action.GetId(),
				Status:      pb.ExecutionStatus_EXECUTION_STATUS_SKIPPED,
				Error:       "skipped after an earlier occurrence failed with STOP policy",
				CompletedAt: timestamppb.New(s.now()),
			}
		} else {
			result = s.executor.ExecuteAction(ctx, action)
		}
		if ctx.Err() != nil {
			// Leave STARTED durable. Startup recovery will report INDETERMINATE
			// and the next run cannot silently repeat the effect.
			return
		}
		result.RunId = &pb.RunId{Value: work.RunID}
		result.OccurrenceId = &pb.OccurrenceId{Value: occurrence.GetOccurrenceId()}
		if result.CompletedAt == nil {
			result.CompletedAt = timestamppb.New(s.now())
		}
		suppressUnchanged := manifest.GetSchedule().GetSkipIfUnchanged() &&
			result.GetStatus() == pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS &&
			!result.GetChanged()
		resultID, suppressed, err := s.store.RecordOccurrenceResult(ctx, result, suppressUnchanged)
		if err != nil {
			s.logger.Error("record occurrence result", "work_id", work.RunID, "occurrence_id", occurrence.GetOccurrenceId(), "error", err)
			return
		}
		if !suppressed {
			s.publish(&ExecutionResult{ResultID: resultID, ActionResult: result})
		}
		aggregate, aggregateError = aggregateStatus(aggregate, aggregateError, result.GetStatus(), result.GetError())
		if result.GetStatus() != pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS && result.GetStatus() != pb.ExecutionStatus_EXECUTION_STATUS_NOT_APPLICABLE && result.GetStatus() != pb.ExecutionStatus_EXECUTION_STATUS_SKIPPED {
			stop = occurrence.GetOnFailure() == pb.OnFailure_ON_FAILURE_STOP
		}
	}

	finished := s.now().UTC()
	manifestResult := &pb.ManifestResult{
		RunId:       work.RunID,
		ManifestId:  manifest.GetManifestId(),
		Status:      aggregate,
		CompletedAt: timestamppb.New(finished),
		Duration:    durationpb.New(finished.Sub(started)),
		Error:       aggregateError,
	}
	resultID, err := s.store.RecordManifestResult(ctx, manifestResult)
	if err != nil {
		s.logger.Error("record manifest result", "work_id", work.RunID, "error", err)
		return
	}
	s.publish(&ExecutionResult{ResultID: resultID, ManifestResult: manifestResult})
}

func aggregateStatus(current pb.ExecutionStatus, currentError string, status pb.ExecutionStatus, resultError string) (pb.ExecutionStatus, string) {
	if status == pb.ExecutionStatus_EXECUTION_STATUS_INDETERMINATE {
		return pb.ExecutionStatus_EXECUTION_STATUS_INDETERMINATE, resultError
	}
	if status != pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS &&
		status != pb.ExecutionStatus_EXECUTION_STATUS_NOT_APPLICABLE &&
		status != pb.ExecutionStatus_EXECUTION_STATUS_SKIPPED &&
		current != pb.ExecutionStatus_EXECUTION_STATUS_INDETERMINATE {
		return pb.ExecutionStatus_EXECUTION_STATUS_FAILED, resultError
	}
	return current, currentError
}

func (s *Scheduler) publish(result *ExecutionResult) {
	select {
	case s.results <- result:
	default:
		s.logger.Warn("result notification queue full; durable outbox will retry", "result_id", result.ResultID)
	}
}

// GetStoredActions supplies the LUKS conflict check from the manifest store.
func (s *Scheduler) GetStoredActions(ctx context.Context) ([]*store.StoredAction, error) {
	return s.store.GetManifestActions(ctx)
}

func (s *Scheduler) SetMaintenanceWindow(ctx context.Context, window *pb.MaintenanceWindow) {
	var normalized *pb.MaintenanceWindow
	if window != nil && len(window.GetSchedule()) != 0 {
		normalized = proto.Clone(window).(*pb.MaintenanceWindow)
	}
	s.windowMu.Lock()
	s.window = normalized
	s.windowDecodeFailed = false
	s.windowMu.Unlock()
	if err := storeMaintenanceWindow(ctx, s.store, normalized); err != nil {
		s.logger.Warn("persist maintenance window", "error", err)
	}
}

func (s *Scheduler) runAllowed(at time.Time) bool {
	s.windowMu.RLock()
	defer s.windowMu.RUnlock()
	return !s.windowDecodeFailed && maintenance.IsAllowed(s.window, at)
}

func loadMaintenanceWindow(ctx context.Context, st *store.Store) (*pb.MaintenanceWindow, error) {
	raw, err := st.GetSetting(ctx, maintenanceWindowSettingKey)
	if err != nil || raw == "" {
		return nil, err
	}
	window := &pb.MaintenanceWindow{}
	if err := proto.Unmarshal([]byte(raw), window); err != nil {
		return nil, fmt.Errorf("decode maintenance window: %w", err)
	}
	if len(window.GetSchedule()) == 0 {
		return nil, nil
	}
	return window, nil
}

func storeMaintenanceWindow(ctx context.Context, st *store.Store, window *pb.MaintenanceWindow) error {
	if window == nil || len(window.GetSchedule()) == 0 {
		return st.DeleteSetting(ctx, maintenanceWindowSettingKey)
	}
	raw, err := proto.Marshal(window)
	if err != nil {
		return err
	}
	return st.SetSetting(ctx, maintenanceWindowSettingKey, string(raw))
}

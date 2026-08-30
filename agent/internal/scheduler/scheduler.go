package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/manchtools/cadestro/agent/internal/store"
	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

const DefaultCheckInterval = time.Minute

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
}

func New(st *store.Store, executor ActionExecutor, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		store: st, executor: executor, logger: logger, now: time.Now,
		wakeCh: make(chan struct{}, 1), results: make(chan *ExecutionResult, 100),
	}
}

func (s *Scheduler) Results() <-chan *ExecutionResult { return s.results }

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
	s.running = true
	s.stopCh = make(chan struct{})
	s.done = make(chan struct{})
	stopCh, done := s.stopCh, s.done
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		close(done)
	}()

	if !s.recoverInterrupted(ctx) {
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
	done := s.done
	s.mu.Unlock()
	<-done
}

func (s *Scheduler) runDue(ctx context.Context) {
	if !s.recoverInterrupted(ctx) {
		return
	}
	workItems, err := s.store.GetDueScheduledWork(ctx)
	if err != nil {
		s.logger.Error("load due manifests", "error", err)
		return
	}
	for _, work := range workItems {
		if ctx.Err() != nil {
			return
		}
		s.executeManifest(ctx, work)
	}
}

func (s *Scheduler) recoverInterrupted(ctx context.Context) bool {
	recovered, err := s.store.RecoverInterruptedOccurrences(ctx)
	if err != nil {
		s.logger.Error("recover interrupted action", "error", err)
		return false
	}
	for _, result := range recovered {
		s.publish(&ExecutionResult{ResultID: result.ID, ActionResult: result.ActionResult})
	}
	return true
}

func (s *Scheduler) executeManifest(ctx context.Context, work store.ScheduledWork) {
	manifest := work.Manifest
	action := manifest.GetAction()
	occurrenceID := manifest.GetOccurrenceId().GetValue()
	started, err := s.store.BeginManifestRun(ctx, &work, s.now().UTC())
	if err != nil {
		s.logger.Error("begin action run", "manifest_id", manifest.GetManifestId().GetValue(), "error", err)
		return
	}
	if err := s.store.MarkOccurrenceStarted(ctx, work.RunID, occurrenceID, started); err != nil {
		s.logger.Error("mark action started", "run_id", work.RunID, "error", err)
		return
	}

	s.executor.ResetUpdateCycle()
	result := s.executor.ExecuteAction(ctx, action)
	if ctx.Err() != nil {
		return
	}
	finished := s.now().UTC()
	result.RunId = &pb.RunId{Value: work.RunID}
	result.OccurrenceId = manifest.GetOccurrenceId()
	if result.CompletedAt == nil {
		result.CompletedAt = timestamppb.New(finished)
	}
	suppress := manifest.GetSchedule().GetSkipIfUnchanged() && result.GetStatus() == pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS && !result.GetChanged()
	resultID, suppressed, err := s.store.RecordOccurrenceResult(ctx, result, suppress)
	if err != nil {
		s.logger.Error("record action result", "run_id", work.RunID, "error", err)
		return
	}
	if !suppressed {
		s.publish(&ExecutionResult{ResultID: resultID, ActionResult: result})
	}

	manifestResult := &pb.ManifestResult{
		RunId: &pb.RunId{Value: work.RunID}, ManifestId: manifest.GetManifestId(), Status: result.GetStatus(),
		CompletedAt: timestamppb.New(finished), Duration: durationpb.New(finished.Sub(started)),
	}
	manifestResultID, err := s.store.RecordManifestResult(ctx, manifestResult)
	if err != nil {
		s.logger.Error("record manifest result", "run_id", work.RunID, "error", err)
		return
	}
	s.publish(&ExecutionResult{ResultID: manifestResultID, ManifestResult: manifestResult})
}

func (s *Scheduler) publish(result *ExecutionResult) {
	select {
	case s.results <- result:
	default:
		s.logger.Warn("result queue full; durable outbox will retry", "result_id", result.ResultID)
	}
}

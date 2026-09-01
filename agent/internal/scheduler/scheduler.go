package scheduler

import (
	"context"
	"github.com/manchtools/cadestro/agent/internal/store"
	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
	"log/slog"
	"sync"
	"time"
)

const DefaultCheckInterval = time.Minute

type ActionExecutor interface {
	ExecuteAction(context.Context, *pb.Action) *pb.ActionResult
}
type Scheduler struct {
	store          *store.Store
	executor       ActionExecutor
	logger         *slog.Logger
	now            func() time.Time
	scheduleWakeCh chan struct{}
	resultReadyCh  chan struct{}
	mu             sync.Mutex
	running        bool
	stopCh         chan struct{}
	done           chan struct{}
}

func New(st *store.Store, e ActionExecutor, l *slog.Logger) *Scheduler {
	return &Scheduler{store: st, executor: e, logger: l, now: time.Now, scheduleWakeCh: make(chan struct{}, 1), resultReadyCh: make(chan struct{}, 1)}
}
func (s *Scheduler) ResultsReady() <-chan struct{} { return s.resultReadyCh }
func (s *Scheduler) ReconcilePolicy(ctx context.Context, p *pb.DesiredPolicy) error {
	if err := s.store.ReconcilePolicy(ctx, p); err != nil {
		return err
	}
	s.wakeSchedule()
	return nil
}
func (s *Scheduler) GetPendingResults(ctx context.Context) ([]store.PendingResult, error) {
	return s.store.GetPendingResults(ctx)
}
func (s *Scheduler) DeletePendingResult(ctx context.Context, n int64) error {
	return s.store.DeletePendingResult(ctx, n)
}
func (s *Scheduler) wakeSchedule() {
	select {
	case s.scheduleWakeCh <- struct{}{}:
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
	stop, done := s.stopCh, s.done
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.running = false; s.mu.Unlock(); close(done) }()
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
		case <-stop:
			return
		case <-ticker.C:
			s.runDue(ctx)
		case <-s.scheduleWakeCh:
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
	items, err := s.store.GetDueScheduledWork(ctx)
	if err != nil {
		s.logger.Error("load due actions", "error", err)
		return
	}
	for _, w := range items {
		if ctx.Err() != nil {
			return
		}
		s.executeAction(ctx, w)
	}
}
func (s *Scheduler) recoverInterrupted(ctx context.Context) bool {
	if err := s.store.RecoverInterruptedActions(ctx); err != nil {
		s.logger.Error("recover interrupted action", "error", err)
		return false
	}
	return true
}
func (s *Scheduler) executeAction(ctx context.Context, w store.ScheduledWork) {
	if err := s.store.BeginActionRun(ctx, &w, s.now().UTC()); err != nil {
		s.logger.Error("begin action run", "action_id", w.Action.GetId().GetValue(), "error", err)
		return
	}
	r := s.executor.ExecuteAction(ctx, w.Action)
	if ctx.Err() != nil {
		return
	}
	r.RunId = &pb.RunId{Value: w.RunID}
	if r.CompletedAt == nil {
		r.CompletedAt = timestamppb.New(s.now().UTC())
	}
	_, err := s.store.RecordActionResult(ctx, r)
	if err != nil {
		s.logger.Error("record action result", "run_id", w.RunID, "error", err)
		return
	}
	select {
	case s.resultReadyCh <- struct{}{}:
	default:
	}
}

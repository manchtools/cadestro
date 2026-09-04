package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/manchtools/cadestro/agent/internal/store"
	contract "github.com/manchtools/cadestro/contract"
	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const checkInterval = time.Minute

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
}

func New(st *store.Store, e ActionExecutor, l *slog.Logger) *Scheduler {
	return &Scheduler{store: st, executor: e, logger: l, now: time.Now, scheduleWakeCh: make(chan struct{}, 1), resultReadyCh: make(chan struct{}, 1)}
}
func (s *Scheduler) ResultsReady() <-chan struct{} { return s.resultReadyCh }
func (s *Scheduler) ReconcilePolicy(ctx context.Context, p *pb.DesiredPolicy) error {
	if err := s.store.ReconcilePolicy(ctx, p); err != nil {
		return err
	}
	select {
	case s.scheduleWakeCh <- struct{}{}:
	default:
	}
	return nil
}
func (s *Scheduler) GetPendingResults(ctx context.Context) ([]store.PendingResult, error) {
	return s.store.GetPendingResults(ctx)
}
func (s *Scheduler) DeletePendingResult(ctx context.Context, n int64) error {
	return s.store.DeletePendingResult(ctx, n)
}
func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	s.runDue(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runDue(ctx)
		case <-s.scheduleWakeCh:
			s.runDue(ctx)
		}
	}
}
func (s *Scheduler) runDue(ctx context.Context) {
	recovered, err := s.store.RecoverInterruptedActions(ctx)
	if err != nil {
		s.logger.Error("recover interrupted action", "error", err)
		return
	}
	if recovered > 0 {
		s.signalResultsReady()
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
	digest, err := contract.ActionDigest(w.Action)
	if err != nil {
		s.logger.Error("digest action result", "run_id", w.RunID, "error", err)
		return
	}
	r.ActionDigest = digest
	if r.CompletedAt == nil {
		r.CompletedAt = timestamppb.New(s.now().UTC())
	}
	_, err = s.store.RecordActionResult(ctx, r)
	if err != nil {
		s.logger.Error("record action result", "run_id", w.RunID, "error", err)
		return
	}
	s.signalResultsReady()
}

func (s *Scheduler) signalResultsReady() {
	select {
	case s.resultReadyCh <- struct{}{}:
	default:
	}
}

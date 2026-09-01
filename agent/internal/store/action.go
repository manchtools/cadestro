package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/manchtools/cadestro/agent/internal/store/generated"
	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/oklog/ulid/v2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"time"
)

type ScheduledWork struct {
	Action        *pb.Action
	WorkID, RunID string
	ReceivedAt    time.Time
	LastExecuted  *time.Time
	NextExecuteAt time.Time
	RunStartedAt  *time.Time
	RunInProgress bool
}
type PendingResult struct {
	Sequence     int64
	ActionResult *pb.ActionResult
}

func (s *Store) ReconcilePolicy(ctx context.Context, policy *pb.DesiredPolicy) error {
	if policy == nil || policy.GetRevision().GetValue() == "" {
		return errors.New("reconcile policy: malformed snapshot")
	}
	current := make(map[string]*pb.Action, len(policy.GetActions()))
	for _, a := range policy.GetActions() {
		if a == nil || a.GetId().GetValue() == "" {
			return errors.New("reconcile policy: malformed action")
		}
		if _, ok := current[a.GetId().GetValue()]; ok {
			return fmt.Errorf("reconcile policy: duplicate action %s", a.GetId().GetValue())
		}
		current[a.GetId().GetValue()] = a
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	rev, err := q.GetAssignedPolicyRevision(ctx)
	if err == nil && rev == policy.GetRevision().GetValue() {
		return tx.Commit()
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	rows, err := q.ListAllWork(ctx)
	if err != nil {
		return err
	}
	inventory := make(map[string]generated.ListAllWorkRow, len(rows))
	for _, row := range rows {
		inventory[row.WorkID] = row
	}
	now := s.now().UTC()
	for _, row := range rows {
		if _, ok := current[row.WorkID]; !ok {
			if row.RunInProgress {
				if err := q.RetireWork(ctx, row.WorkID); err != nil {
					return err
				}
			} else if err := q.DeleteWork(ctx, row.WorkID); err != nil {
				return err
			}
		} else if !row.Retired {
			stored, decodeErr := decodeAction(row.ActionBlob)
			if decodeErr != nil {
				return fmt.Errorf("reconcile policy: decode %s: %w", row.WorkID, decodeErr)
			}
			if !proto.Equal(stored, current[row.WorkID]) {
				blob, marshalErr := proto.Marshal(current[row.WorkID])
				if marshalErr != nil {
					return fmt.Errorf("reconcile policy: marshal %s: %w", row.WorkID, marshalErr)
				}
				if err := q.UpdateScheduledWork(ctx, generated.UpdateScheduledWorkParams{ActionBlob: blob, NextExecuteAt: now, WorkID: row.WorkID}); err != nil {
					return fmt.Errorf("reconcile policy: update %s: %w", row.WorkID, err)
				}
			}
		} else {
			blob, marshalErr := proto.Marshal(current[row.WorkID])
			if marshalErr != nil {
				return fmt.Errorf("reconcile policy: marshal %s: %w", row.WorkID, marshalErr)
			}
			if err := q.UpdateScheduledWork(ctx, generated.UpdateScheduledWorkParams{ActionBlob: blob, NextExecuteAt: now, WorkID: row.WorkID}); err != nil {
				return fmt.Errorf("reconcile policy: revive %s: %w", row.WorkID, err)
			}
		}
	}
	for id, a := range current {
		if _, found := inventory[id]; found {
			continue
		}
		blob, marshalErr := proto.Marshal(a)
		if marshalErr != nil {
			return fmt.Errorf("reconcile policy: marshal %s: %w", id, marshalErr)
		}
		if err := q.InsertScheduledWork(ctx, generated.InsertScheduledWorkParams{WorkID: id, ActionBlob: blob, ReceivedAt: now, NextExecuteAt: calculateNextExecuteFromSchedule(a.GetSchedule(), nil, now)}); err != nil {
			return fmt.Errorf("reconcile policy: insert %s: %w", id, err)
		}
	}
	if err := q.SetAssignedPolicyRevision(ctx, policy.GetRevision().GetValue()); err != nil {
		return err
	}
	return tx.Commit()
}
func decodeAction(blob []byte) (*pb.Action, error) {
	action := &pb.Action{}
	if err := proto.Unmarshal(blob, action); err != nil {
		return nil, err
	}
	return action, nil
}
func (s *Store) GetDueScheduledWork(ctx context.Context) ([]ScheduledWork, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.queries.GetDueScheduledWork(ctx, s.now().UTC())
	if err != nil {
		return nil, err
	}
	out := make([]ScheduledWork, 0, len(rows))
	for _, row := range rows {
		a := &pb.Action{}
		if err := proto.Unmarshal(row.ActionBlob, a); err != nil {
			return nil, fmt.Errorf("decode action %s: %w", row.WorkID, err)
		}
		out = append(out, ScheduledWork{Action: a, WorkID: row.WorkID, RunID: row.RunID, ReceivedAt: row.ReceivedAt, LastExecuted: row.LastExecutedAt, NextExecuteAt: row.NextExecuteAt, RunStartedAt: row.RunStartedAt, RunInProgress: row.RunInProgress})
	}
	return out, nil
}
func (s *Store) BeginActionRun(ctx context.Context, w *ScheduledWork, started time.Time) error {
	if w == nil || w.Action == nil || w.WorkID == "" {
		return errors.New("begin action run: missing work")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	run, err := q.GetScheduledRun(ctx, generated.GetScheduledRunParams{WorkID: w.WorkID, RunID: &w.RunID})
	if err != nil {
		return err
	}
	if run.RunInProgress {
		w.RunID = run.RunID
		return tx.Commit()
	}
	started = started.UTC()
	id := ulid.Make().String()
	n, err := q.BeginScheduledRun(ctx, generated.BeginScheduledRunParams{RunID: &id, LastExecutedAt: &started, NextExecuteAt: calculateNextExecuteFromSchedule(w.Action.GetSchedule(), &started, s.now()), RunStartedAt: &started, WorkID: w.WorkID})
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("begin action run: already running")
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	w.RunID = id
	w.RunStartedAt = &started
	return nil
}
func (s *Store) RecordActionResult(ctx context.Context, r *pb.ActionResult) (int64, error) {
	if r == nil || r.GetActionId().GetValue() == "" || r.GetRunId().GetValue() == "" {
		return 0, errors.New("record action result: malformed result")
	}
	payload, err := proto.Marshal(r)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	retired, err := q.FinishScheduledRun(ctx, generated.FinishScheduledRunParams{WorkID: r.GetActionId().GetValue(), RunID: &r.RunId.Value})
	if err != nil {
		return 0, err
	}
	sequence, err := q.InsertResultOutbox(ctx, payload)
	if err != nil {
		return 0, err
	}
	if retired {
		if err := q.DeleteRetiredWork(ctx, generated.DeleteRetiredWorkParams{WorkID: r.GetActionId().GetValue(), RunID: &r.RunId.Value}); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return sequence, nil
}
func (s *Store) GetPendingResults(ctx context.Context) ([]PendingResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.queries.GetPendingResults(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PendingResult, 0, len(rows))
	for _, row := range rows {
		r := &pb.ActionResult{}
		if err := proto.Unmarshal(row.Payload, r); err != nil {
			return nil, err
		}
		out = append(out, PendingResult{Sequence: row.Sequence, ActionResult: r})
	}
	return out, nil
}
func (s *Store) DeletePendingResult(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queries.DeletePendingResult(ctx, id)
}
func (s *Store) RecoverInterruptedActions(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	rows, err := q.ListInterruptedWork(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		a := &pb.Action{}
		if err := proto.Unmarshal(row.ActionBlob, a); err != nil {
			return err
		}
		r := &pb.ActionResult{ActionId: a.GetId(), RunId: &pb.RunId{Value: row.RunID}, Status: pb.ExecutionStatus_EXECUTION_STATUS_INDETERMINATE, Output: &pb.CommandOutput{Stderr: "interrupted action run"}, CompletedAt: timestamppb.New(s.now().UTC())}
		payload, err := proto.Marshal(r)
		if err != nil {
			return err
		}
		retired, err := q.FinishScheduledRun(ctx, generated.FinishScheduledRunParams{WorkID: row.WorkID, RunID: &row.RunID})
		if err != nil {
			return err
		}
		if _, err := q.InsertResultOutbox(ctx, payload); err != nil {
			return err
		}
		if retired {
			if err := q.DeleteRetiredWork(ctx, generated.DeleteRetiredWorkParams{WorkID: row.WorkID, RunID: &row.RunID}); err != nil {
				return fmt.Errorf("recover interrupted action: delete retired work: %w", err)
			}
		}
	}
	return tx.Commit()
}

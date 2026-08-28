package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/manchtools/cadestro/agent/internal/store/generated"
	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

const (
	OccurrencePending       = "PENDING"
	OccurrenceStarted       = "STARTED"
	OccurrenceSuccess       = "SUCCESS"
	OccurrenceFailed        = "FAILED"
	OccurrenceIndeterminate = "INDETERMINATE"
)

func timePtr(value time.Time) *time.Time { return &value }

func stringPtr(value string) *string { return &value }

type ScheduledWork struct {
	Manifest      *pb.Manifest
	WorkID        string
	RunID         string
	ReceivedAt    time.Time
	LastExecuted  *time.Time
	NextExecuteAt time.Time
	RunStartedAt  *time.Time
	RunInProgress bool
}

type PendingResult struct {
	ID             string
	ActionResult   *pb.ActionResult
	ManifestResult *pb.ManifestResult
}

func (s *Store) resolveWorkID(ctx context.Context, id string) (string, error) {
	return s.queries.ResolveWorkID(ctx, generated.ResolveWorkIDParams{WorkID: id, RunID: stringPtr(id)})
}

func (s *Store) ReconcilePolicy(ctx context.Context, policy *pb.DesiredPolicy) error {
	if policy == nil {
		return errors.New("reconcile policy: missing snapshot")
	}
	if policy.GetRevision().GetValue() == "" {
		return errors.New("reconcile policy: missing revision")
	}
	current := make(map[string]*pb.Manifest, len(policy.Manifests))
	for _, manifest := range policy.Manifests {
		if manifest == nil || manifest.GetManifestId().GetValue() == "" || manifest.GetOccurrenceId().GetValue() == "" || manifest.GetAction().GetId().GetValue() == "" {
			return errors.New("reconcile policy: malformed manifest")
		}
		if _, exists := current[manifest.GetManifestId().GetValue()]; exists {
			return fmt.Errorf("reconcile policy: duplicate manifest identity %s", manifest.GetManifestId().GetValue())
		}
		current[manifest.GetManifestId().GetValue()] = manifest
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("reconcile policy: begin: %w", err)
	}
	defer tx.Rollback()
	var appliedRevision string
	queries := s.queries.WithTx(tx)
	appliedRevision, err = queries.GetAssignedPolicyRevision(ctx)
	if err == nil && appliedRevision == policy.GetRevision().GetValue() {
		return tx.Commit()
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("reconcile policy: read revision: %w", err)
	}
	rows, err := queries.ListActiveWork(ctx)
	if err != nil {
		return fmt.Errorf("reconcile policy: list: %w", err)
	}
	type staleWork struct {
		id     string
		active bool
	}
	var stale []staleWork
	for _, row := range rows {
		if _, keep := current[row.WorkID]; !keep {
			stale = append(stale, staleWork{id: row.WorkID, active: row.RunInProgress})
		}
	}
	for _, work := range stale {
		if work.active {
			if err := queries.RetireWork(ctx, work.id); err != nil {
				return fmt.Errorf("reconcile policy: retire %s: %w", work.id, err)
			}
		} else if err := queries.DeleteWork(ctx, work.id); err != nil {
			return fmt.Errorf("reconcile policy: remove %s: %w", work.id, err)
		}
	}
	now := s.now().UTC()
	for id, manifest := range current {
		blob, err := marshalStoredProto(manifest)
		if err != nil {
			return fmt.Errorf("reconcile policy: marshal %s: %w", id, err)
		}
		_, err = queries.ScheduledWorkExists(ctx, id)
		if err == nil {
			if err := queries.ReviveWork(ctx, id); err != nil {
				return fmt.Errorf("reconcile policy: revive %s: %w", id, err)
			}
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err := queries.InsertScheduledWork(ctx, generated.InsertScheduledWorkParams{
			WorkID: id, RunID: &id, ManifestBlob: blob, ReceivedAt: now,
			NextExecuteAt: calculateNextExecuteFromSchedule(manifest.GetSchedule(), nil, false, now),
		}); err != nil {
			return fmt.Errorf("reconcile policy: insert %s: %w", id, err)
		}
		if err := queries.InsertOccurrence(ctx, generated.InsertOccurrenceParams{
			WorkID: id, OccurrenceID: manifest.GetOccurrenceId().GetValue(), Position: 0, ActionID: manifest.GetAction().GetId().GetValue(),
		}); err != nil {
			return fmt.Errorf("reconcile policy: insert occurrence: %w", err)
		}
	}
	if err := queries.SetAssignedPolicyRevision(ctx, policy.GetRevision().GetValue()); err != nil {
		return fmt.Errorf("reconcile policy: store revision: %w", err)
	}
	return tx.Commit()
}

func (s *Store) GetDueScheduledWork(ctx context.Context) ([]ScheduledWork, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.queries.GetDueScheduledWork(ctx, s.now().UTC())
	if err != nil {
		return nil, err
	}
	var workItems []ScheduledWork
	for _, row := range rows {
		workID, runID, blob := row.WorkID, row.RunID, row.ManifestBlob
		stored := ScheduledWork{ReceivedAt: row.ReceivedAt, NextExecuteAt: row.NextExecuteAt, RunInProgress: row.RunInProgress}
		manifest := &pb.Manifest{}
		if err := unmarshalStoredProto(blob, manifest); err != nil {
			return nil, fmt.Errorf("decode manifest work %s: %w", workID, err)
		}
		if runID == "" {
			runID = workID
		}
		stored.WorkID, stored.RunID = workID, runID
		stored.Manifest = manifest
		if row.LastExecutedAt != nil {
			lastTime := *row.LastExecutedAt
			stored.LastExecuted = &lastTime
		}
		if row.RunStartedAt != nil {
			runStartedTime := *row.RunStartedAt
			stored.RunStartedAt = &runStartedTime
		}
		workItems = append(workItems, stored)
	}
	return workItems, nil
}

func (s *Store) GetManifestActions(ctx context.Context) ([]*StoredAction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.queries.GetManifestActions(ctx)
	if err != nil {
		return nil, err
	}
	var actions []*StoredAction
	for _, row := range rows {
		workID, blob, received, next := row.WorkID, row.ManifestBlob, row.ReceivedAt, row.NextExecuteAt
		manifest := &pb.Manifest{}
		if err := unmarshalStoredProto(blob, manifest); err != nil {
			return nil, fmt.Errorf("decode manifest work %s: %w", workID, err)
		}
		stored := &StoredAction{
			ID: manifest.GetAction().GetId().GetValue(), Action: manifest.GetAction(), AssignedAt: received, NextExecuteAt: next,
		}
		if row.LastExecutedAt != nil {
			lastTime := *row.LastExecutedAt
			stored.LastExecutedAt = &lastTime
		}
		actions = append(actions, stored)
	}
	return actions, nil
}

func (s *Store) BeginManifestRun(ctx context.Context, work *ScheduledWork, startedAt time.Time) (time.Time, error) {
	if work == nil || work.Manifest == nil || work.WorkID == "" {
		return time.Time{}, errors.New("begin manifest run: missing work")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return time.Time{}, err
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	run, err := queries.GetScheduledRun(ctx, generated.GetScheduledRunParams{WorkID: work.WorkID, RunID: stringPtr(work.RunID)})
	if err != nil {
		return time.Time{}, err
	}
	workID, runID, inProgress, priorStarted := run.WorkID, run.RunID, run.RunInProgress, run.RunStartedAt
	if runID == "" {
		runID = workID
	}
	if inProgress {
		if priorStarted == nil {
			return time.Time{}, errors.New("begin manifest run: active run has no start time")
		}
		if err := tx.Commit(); err != nil {
			return time.Time{}, err
		}
		return priorStarted.UTC(), nil
	}
	started, err := queries.CountStartedOccurrences(ctx, generated.CountStartedOccurrencesParams{WorkID: workID, State: OccurrenceStarted})
	if err != nil {
		return time.Time{}, err
	}
	if started != 0 {
		return time.Time{}, fmt.Errorf("begin manifest run: work %s has interrupted occurrences", work.WorkID)
	}
	if err := queries.ResetOccurrences(ctx, generated.ResetOccurrencesParams{State: OccurrencePending, WorkID: workID}); err != nil {
		return time.Time{}, err
	}
	startedAt = startedAt.UTC()
	next := calculateNextExecuteFromSchedule(work.Manifest.GetSchedule(), &startedAt, false, s.now())
	if runID == "" || runID == workID {
		runID = ulid.Make().String()
	}
	if err := queries.BeginScheduledRun(ctx, generated.BeginScheduledRunParams{
		RunID: &runID, LastExecutedAt: &startedAt, NextExecuteAt: next, RunStartedAt: &startedAt, WorkID: workID,
	}); err != nil {
		return time.Time{}, err
	}
	if err := tx.Commit(); err != nil {
		return time.Time{}, err
	}
	work.RunID = runID
	return startedAt, nil
}

func (s *Store) MarkOccurrenceStarted(ctx context.Context, workID, occurrenceID string, startedAt time.Time) error {
	workID, err := s.resolveWorkID(ctx, workID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	changed, err := s.queries.MarkOccurrenceStarted(ctx, generated.MarkOccurrenceStartedParams{
		State: OccurrenceStarted, StartedAt: timePtr(startedAt.UTC()), WorkID: workID, OccurrenceID: occurrenceID, State_2: OccurrencePending,
	})
	if err != nil {
		return err
	}
	if changed != 1 {
		return fmt.Errorf("mark occurrence started: invalid state for %s/%s", workID, occurrenceID)
	}
	return nil
}

func (s *Store) RecordOccurrenceResult(ctx context.Context, result *pb.ActionResult, suppressUnchanged bool) (string, bool, error) {
	if result == nil || result.GetRunId().GetValue() == "" || result.GetOccurrenceId().GetValue() == "" {
		return "", false, errors.New("record occurrence result: missing work or occurrence identity")
	}
	state, err := occurrenceState(result.GetStatus())
	if err != nil {
		return "", false, err
	}
	payload, err := marshalStoredProto(result)
	if err != nil {
		return "", false, fmt.Errorf("record occurrence result: marshal: %w", err)
	}
	resultHash, err := actionResultHash(result)
	if err != nil {
		return "", false, err
	}
	workID, err := s.resolveWorkID(ctx, result.GetRunId().GetValue())
	if err != nil {
		return "", false, err
	}
	now := s.now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	previousHash, err := queries.GetStartedOccurrenceHash(ctx, generated.GetStartedOccurrenceHashParams{
		WorkID: workID, OccurrenceID: result.GetOccurrenceId().GetValue(), State: OccurrenceStarted,
	})
	if err != nil {
		return "", false, err
	}
	status := int32(result.GetStatus())
	updated, err := queries.RecordOccurrence(ctx, generated.RecordOccurrenceParams{
		State: state, CompletedAt: timePtr(now), ResultStatus: &status, ResultError: result.GetError(), LastResultHash: resultHash,
		WorkID: workID, OccurrenceID: result.GetOccurrenceId().GetValue(), State_2: OccurrenceStarted,
	})
	if err != nil {
		return "", false, err
	}
	if updated != 1 {
		return "", false, errors.New("record occurrence result: occurrence was not STARTED")
	}
	if suppressUnchanged && previousHash != "" && previousHash == resultHash {
		if err := tx.Commit(); err != nil {
			return "", false, err
		}
		return "", true, nil
	}
	id, err := randomResultID("ACTION", now)
	if err != nil {
		return "", false, err
	}
	if err := queries.InsertResultOutbox(ctx, generated.InsertResultOutboxParams{ID: id, Kind: "ACTION", Payload: payload, CreatedAt: now}); err != nil {
		return "", false, err
	}
	if err := tx.Commit(); err != nil {
		return "", false, err
	}
	return id, false, nil
}

func actionResultHash(result *pb.ActionResult) (string, error) {
	stable := proto.Clone(result).(*pb.ActionResult)
	stable.CompletedAt = nil
	stable.Duration = nil
	stable.RunId = nil
	stable.OccurrenceId = nil
	encoded, err := canonicalProtoBytes(stable)
	if err != nil {
		return "", fmt.Errorf("hash action result: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func (s *Store) RecordManifestResult(ctx context.Context, result *pb.ManifestResult) (string, error) {
	if result == nil || result.GetRunId().GetValue() == "" || result.GetManifestId().GetValue() == "" {
		return "", errors.New("record manifest result: missing identity")
	}
	return s.recordResult(ctx, "MANIFEST", result, func(ctx context.Context, queries *generated.Queries, _ time.Time) error {
		runID := result.GetRunId().GetValue()
		rows, err := queries.FinishManifestRun(ctx, generated.FinishManifestRunParams{WorkID: runID, RunID: stringPtr(runID)})
		if err != nil {
			return err
		}
		if rows != 1 {
			return errors.New("record manifest result: manifest run is not active")
		}
		if err := queries.DeleteRetiredWork(ctx, generated.DeleteRetiredWorkParams{WorkID: runID, RunID: stringPtr(runID)}); err != nil {
			return err
		}
		return queries.ClearRunID(ctx, generated.ClearRunIDParams{WorkID: runID, RunID: stringPtr(runID)})
	})
}

func (s *Store) recordResult(ctx context.Context, kind string, message proto.Message, update func(context.Context, *generated.Queries, time.Time) error) (string, error) {
	payload, err := marshalStoredProto(message)
	if err != nil {
		return "", fmt.Errorf("record result: marshal: %w", err)
	}
	now := s.now().UTC()
	id, err := randomResultID(kind, now)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	if update != nil {
		if err := update(ctx, queries, now); err != nil {
			return "", err
		}
	}
	if err := queries.InsertResultOutbox(ctx, generated.InsertResultOutboxParams{ID: id, Kind: kind, Payload: payload, CreatedAt: now}); err != nil {
		return "", err
	}
	return id, tx.Commit()
}

func (s *Store) GetPendingResults(ctx context.Context) ([]PendingResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.queries.GetPendingResults(ctx)
	if err != nil {
		return nil, err
	}
	var pending []PendingResult
	for _, row := range rows {
		var item PendingResult
		item.ID = row.ID
		switch row.Kind {
		case "ACTION":
			item.ActionResult = &pb.ActionResult{}
			if err := unmarshalStoredProto(row.Payload, item.ActionResult); err != nil {
				return nil, fmt.Errorf("decode action result %s: %w", item.ID, err)
			}
		case "MANIFEST":
			item.ManifestResult = &pb.ManifestResult{}
			if err := unmarshalStoredProto(row.Payload, item.ManifestResult); err != nil {
				return nil, fmt.Errorf("decode manifest result %s: %w", item.ID, err)
			}
		default:
			return nil, fmt.Errorf("unknown result outbox kind %q", row.Kind)
		}
		pending = append(pending, item)
	}
	return pending, nil
}

func (s *Store) MarkPendingResultSynced(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queries.MarkPendingResultSynced(ctx, id)
}

func (s *Store) RecoverInterruptedOccurrences(ctx context.Context) ([]PendingResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	rows, err := queries.ListInterruptedOccurrences(ctx, OccurrenceStarted)
	if err != nil {
		return nil, err
	}
	type interrupted struct {
		runID, workID, occurrenceID, actionID string
	}
	var interruptedRows []interrupted
	for _, row := range rows {
		interruptedRows = append(interruptedRows, interrupted{runID: row.RunID, workID: row.WorkID, occurrenceID: row.OccurrenceID, actionID: row.ActionID})
	}
	now := s.now().UTC()
	var recovered []PendingResult
	for _, item := range interruptedRows {
		status := pb.ExecutionStatus_EXECUTION_STATUS_INDETERMINATE
		message := "agent restarted after STARTED; effect is unknown and was not repeated"
		result := &pb.ActionResult{
			ActionId:     &pb.ActionId{Value: item.actionID},
			RunId:        &pb.RunId{Value: item.runID},
			OccurrenceId: &pb.OccurrenceId{Value: item.occurrenceID},
			Status:       status,
			Error:        message,
			CompletedAt:  timestamppb.New(now),
		}
		payload, err := marshalStoredProto(result)
		if err != nil {
			return nil, err
		}
		id, err := randomResultID("ACTION", now)
		if err != nil {
			return nil, err
		}
		if err := queries.InsertRecoveredResult(ctx, generated.InsertRecoveredResultParams{ID: id, Payload: payload, CreatedAt: now}); err != nil {
			return nil, err
		}
		state, err := occurrenceState(status)
		if err != nil {
			return nil, err
		}
		statusValue := int32(status)
		if err := queries.RecoverOccurrence(ctx, generated.RecoverOccurrenceParams{
			State: state, CompletedAt: timePtr(now), ResultStatus: &statusValue, ResultError: message,
			WorkID: item.workID, OccurrenceID: item.occurrenceID, State_2: OccurrenceStarted,
		}); err != nil {
			return nil, err
		}
		recovered = append(recovered, PendingResult{ID: id, ActionResult: result})
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return recovered, nil
}

func occurrenceState(status pb.ExecutionStatus) (string, error) {
	switch status {
	case pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS:
		return OccurrenceSuccess, nil
	case pb.ExecutionStatus_EXECUTION_STATUS_INDETERMINATE:
		return OccurrenceIndeterminate, nil
	case pb.ExecutionStatus_EXECUTION_STATUS_FAILED,
		pb.ExecutionStatus_EXECUTION_STATUS_TIMEOUT,
		pb.ExecutionStatus_EXECUTION_STATUS_SKIPPED:
		return OccurrenceFailed, nil
	default:
		return "", fmt.Errorf("record occurrence result: non-terminal status %s", status)
	}
}

func randomResultID(kind string, now time.Time) (string, error) {
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", fmt.Errorf("generate result ID: %w", err)
	}
	return fmt.Sprintf("%s-%d-%s", kind, now.UnixNano(), hex.EncodeToString(suffix[:])), nil
}

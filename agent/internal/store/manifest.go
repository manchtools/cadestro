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

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

const (
	OccurrencePending       = "PENDING"
	OccurrenceStarted       = "STARTED"
	OccurrenceSuccess       = "SUCCESS"
	OccurrenceFailed        = "FAILED"
	OccurrenceIndeterminate = "INDETERMINATE"
)

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

type ManifestOccurrenceState struct {
	State        string
	ResultStatus pb.ExecutionStatus
	ResultError  string
}

type PendingResult struct {
	ID             string
	ActionResult   *pb.ActionResult
	ManifestResult *pb.ManifestResult
}

func (s *Store) resolveWorkID(id string) (string, error) {
	var workID string
	if err := s.db.QueryRow(`SELECT work_id FROM scheduled_work WHERE work_id = ? OR run_id = ?`, id, id).Scan(&workID); err != nil {
		return "", err
	}
	return workID, nil
}

// ReconcilePolicy replaces assignment-derived manifests from authenticated Sync.
func (s *Store) ReconcilePolicy(ctx context.Context, policy *pb.DesiredPolicy) error {
	if policy == nil {
		return errors.New("reconcile policy: missing snapshot")
	}
	if policy.GetRevision() == "" {
		return errors.New("reconcile policy: missing revision")
	}
	current := make(map[string]*pb.Manifest, len(policy.Manifests))
	for _, manifest := range policy.Manifests {
		if manifest == nil || manifest.GetManifestId() == "" || len(manifest.GetOccurrences()) == 0 {
			return errors.New("reconcile policy: malformed manifest")
		}
		if _, exists := current[manifest.GetManifestId()]; exists {
			return fmt.Errorf("reconcile policy: duplicate manifest identity %s", manifest.GetManifestId())
		}
		current[manifest.GetManifestId()] = manifest
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("reconcile policy: begin: %w", err)
	}
	defer tx.Rollback()
	var appliedRevision string
	err = tx.QueryRow("SELECT value FROM settings WHERE key = 'assigned_policy_revision'").Scan(&appliedRevision)
	if err == nil && appliedRevision == policy.GetRevision() {
		return tx.Commit()
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("reconcile policy: read revision: %w", err)
	}
	rows, err := tx.Query("SELECT work_id, run_in_progress FROM scheduled_work WHERE retired = FALSE")
	if err != nil {
		return fmt.Errorf("reconcile policy: list: %w", err)
	}
	type staleWork struct {
		id     string
		active bool
	}
	var stale []staleWork
	for rows.Next() {
		var id string
		var active bool
		if err := rows.Scan(&id, &active); err != nil {
			_ = rows.Close()
			return err
		}
		if _, keep := current[id]; !keep {
			stale = append(stale, staleWork{id: id, active: active})
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, work := range stale {
		if work.active {
			if _, err := tx.Exec("UPDATE scheduled_work SET retired = TRUE WHERE work_id = ?", work.id); err != nil {
				return fmt.Errorf("reconcile policy: retire %s: %w", work.id, err)
			}
		} else if _, err := tx.Exec("DELETE FROM scheduled_work WHERE work_id = ?", work.id); err != nil {
			return fmt.Errorf("reconcile policy: remove %s: %w", work.id, err)
		}
	}
	now := s.now().UTC()
	for id, manifest := range current {
		blob, err := marshalStoredProto(manifest)
		if err != nil {
			return fmt.Errorf("reconcile policy: marshal %s: %w", id, err)
		}
		var existing int
		err = tx.QueryRow("SELECT 1 FROM scheduled_work WHERE work_id = ?", id).Scan(&existing)
		if err == nil {
			if _, err := tx.Exec("UPDATE scheduled_work SET retired = FALSE WHERE work_id = ?", id); err != nil {
				return fmt.Errorf("reconcile policy: revive %s: %w", id, err)
			}
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO scheduled_work
			(work_id, run_id, manifest_blob, retired, received_at, next_execute_at)
			VALUES (?, ?, ?, FALSE, ?, ?)`, id, id, blob, now, calculateNextExecuteFromSchedule(manifest.GetSchedule(), nil, false, now)); err != nil {
			return fmt.Errorf("reconcile policy: insert %s: %w", id, err)
		}
		for position, occurrence := range manifest.GetOccurrences() {
			if occurrence == nil || occurrence.GetOccurrenceId() == "" || occurrence.GetAction().GetId().GetValue() == "" {
				return errors.New("reconcile policy: malformed occurrence")
			}
			if _, err := tx.Exec(`INSERT INTO scheduled_work_occurrences
				(work_id, occurrence_id, position, action_id) VALUES (?, ?, ?, ?)`,
				id, occurrence.GetOccurrenceId(), position, occurrence.GetAction().GetId().GetValue()); err != nil {
				return fmt.Errorf("reconcile policy: insert occurrence: %w", err)
			}
		}
	}
	if _, err := tx.Exec(`INSERT INTO settings (key, value) VALUES ('assigned_policy_revision', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, policy.GetRevision()); err != nil {
		return fmt.Errorf("reconcile policy: store revision: %w", err)
	}
	return tx.Commit()
}

func (s *Store) GetDueScheduledWork(ctx context.Context) ([]ScheduledWork, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.QueryContext(ctx, `
		SELECT work_id, COALESCE(run_id, ''), manifest_blob,
		       received_at, last_executed_at, next_execute_at,
		       run_started_at, run_in_progress
		FROM scheduled_work
		WHERE (retired = FALSE OR run_in_progress = TRUE)
		  AND (run_in_progress = TRUE OR next_execute_at <= ?)
		ORDER BY run_in_progress DESC, next_execute_at, work_id
	`, s.now().UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workItems []ScheduledWork
	for rows.Next() {
		var workID, runID string
		var blob []byte
		var stored ScheduledWork
		var last, runStarted sql.NullTime
		if err := rows.Scan(
			&workID, &runID, &blob, &stored.ReceivedAt, &last,
			&stored.NextExecuteAt, &runStarted, &stored.RunInProgress,
		); err != nil {
			return nil, err
		}
		manifest := &pb.Manifest{}
		if err := unmarshalStoredProto(blob, manifest); err != nil {
			return nil, fmt.Errorf("decode manifest work %s: %w", workID, err)
		}
		if runID == "" {
			runID = workID
		}
		stored.WorkID, stored.RunID = workID, runID
		stored.Manifest = manifest
		if last.Valid {
			lastTime := last.Time
			stored.LastExecuted = &lastTime
		}
		if runStarted.Valid {
			runStartedTime := runStarted.Time
			stored.RunStartedAt = &runStartedTime
		}
		workItems = append(workItems, stored)
	}
	return workItems, rows.Err()
}

func (s *Store) GetManifestActions() ([]*StoredAction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query(`
		SELECT work_id, manifest_blob, received_at, last_executed_at, next_execute_at
		FROM scheduled_work WHERE retired = FALSE ORDER BY received_at, work_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var actions []*StoredAction
	for rows.Next() {
		var workID string
		var blob []byte
		var received, next time.Time
		var last sql.NullTime
		if err := rows.Scan(&workID, &blob, &received, &last, &next); err != nil {
			return nil, err
		}
		manifest := &pb.Manifest{}
		if err := unmarshalStoredProto(blob, manifest); err != nil {
			return nil, fmt.Errorf("decode manifest work %s: %w", workID, err)
		}
		for _, occurrence := range manifest.GetOccurrences() {
			stored := &StoredAction{
				ID:            occurrence.GetAction().GetId().GetValue(),
				Action:        occurrence.GetAction(),
				AssignedAt:    received,
				NextExecuteAt: next,
			}
			if last.Valid {
				lastTime := last.Time
				stored.LastExecutedAt = &lastTime
			}
			actions = append(actions, stored)
		}
	}
	return actions, rows.Err()
}

// BeginManifestRun advances the manifest cursor before any side effect. An
// interrupted run stays active and resumes from its durable occurrence states.
func (s *Store) BeginManifestRun(work *ScheduledWork, startedAt time.Time) (time.Time, error) {
	if work == nil || work.Manifest == nil || work.WorkID == "" {
		return time.Time{}, errors.New("begin manifest run: missing work")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return time.Time{}, err
	}
	defer tx.Rollback()
	var inProgress bool
	var runID, workID string
	var priorStarted sql.NullTime
	if err := tx.QueryRow(`
		SELECT work_id, COALESCE(run_id, ''), run_in_progress, run_started_at
		FROM scheduled_work
		WHERE (work_id = ? OR run_id = ?)
		  AND (retired = FALSE OR run_in_progress = TRUE)
	`, work.WorkID, work.RunID).Scan(&workID, &runID, &inProgress, &priorStarted); err != nil {
		return time.Time{}, err
	}
	if runID == "" {
		runID = workID
	}
	if inProgress {
		if !priorStarted.Valid {
			return time.Time{}, errors.New("begin manifest run: active run has no start time")
		}
		if err := tx.Commit(); err != nil {
			return time.Time{}, err
		}
		return priorStarted.Time.UTC(), nil
	}
	var started int
	if err := tx.QueryRow(`
		SELECT COUNT(*) FROM scheduled_work_occurrences
		WHERE work_id = ? AND state = ?
	`, workID, OccurrenceStarted).Scan(&started); err != nil {
		return time.Time{}, err
	}
	if started != 0 {
		return time.Time{}, fmt.Errorf("begin manifest run: work %s has interrupted occurrences", work.WorkID)
	}
	if _, err := tx.Exec(`
		UPDATE scheduled_work_occurrences
		SET state = ?, started_at = NULL, completed_at = NULL,
		    result_status = NULL, result_error = ''
		WHERE work_id = ?
	`, OccurrencePending, workID); err != nil {
		return time.Time{}, err
	}
	startedAt = startedAt.UTC()
	next := calculateNextExecuteFromSchedule(work.Manifest.GetSchedule(), &startedAt, false, s.now())
	if runID == "" || runID == workID {
		runID = ulid.Make().String()
	}
	if _, err := tx.Exec(`
		UPDATE scheduled_work
		SET run_id = ?, last_executed_at = ?, next_execute_at = ?,
		    run_started_at = ?, run_in_progress = TRUE
		WHERE work_id = ?
	`, runID, startedAt, next, startedAt, workID); err != nil {
		return time.Time{}, err
	}
	if err := tx.Commit(); err != nil {
		return time.Time{}, err
	}
	work.RunID = runID
	return startedAt, nil
}

func (s *Store) MarkOccurrenceStarted(workID, occurrenceID string, startedAt time.Time) error {
	workID, err := s.resolveWorkID(workID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := s.db.Exec(`
		UPDATE scheduled_work_occurrences SET state = ?, started_at = ?, completed_at = NULL
		WHERE work_id = ? AND occurrence_id = ? AND state = ?
	`, OccurrenceStarted, startedAt.UTC(), workID, occurrenceID, OccurrencePending)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return fmt.Errorf("mark occurrence started: invalid state for %s/%s", workID, occurrenceID)
	}
	return nil
}

func (s *Store) GetManifestOccurrenceStates(workID string) (map[string]ManifestOccurrenceState, error) {
	workID, err := s.resolveWorkID(workID)
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query(`
		SELECT occurrence_id, state, result_status, result_error
		FROM scheduled_work_occurrences WHERE work_id = ? ORDER BY position
	`, workID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	states := make(map[string]ManifestOccurrenceState)
	for rows.Next() {
		var occurrenceID, state, resultError string
		var resultStatus sql.NullInt64
		if err := rows.Scan(&occurrenceID, &state, &resultStatus, &resultError); err != nil {
			return nil, err
		}
		item := ManifestOccurrenceState{State: state, ResultError: resultError}
		if resultStatus.Valid {
			item.ResultStatus = pb.ExecutionStatus(resultStatus.Int64)
		}
		states[occurrenceID] = item
	}
	return states, rows.Err()
}

func (s *Store) RecordOccurrenceResult(result *pb.ActionResult, suppressUnchanged bool) (string, bool, error) {
	if result == nil || result.GetRunId() == "" || result.GetOccurrenceId() == "" {
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
	workID, err := s.resolveWorkID(result.GetRunId())
	if err != nil {
		return "", false, err
	}
	now := s.now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback()
	var previousHash string
	if err := tx.QueryRow(`
		SELECT last_result_hash FROM scheduled_work_occurrences
		WHERE work_id = ? AND occurrence_id = ? AND state = ?
	`, workID, result.GetOccurrenceId(), OccurrenceStarted).Scan(&previousHash); err != nil {
		return "", false, err
	}
	updated, err := tx.Exec(`
		UPDATE scheduled_work_occurrences
		SET state = ?, completed_at = ?, result_status = ?, result_error = ?, last_result_hash = ?
		WHERE work_id = ? AND occurrence_id = ? AND state = ?
	`, state, now, result.GetStatus(), result.GetError(), resultHash,
		workID, result.GetOccurrenceId(), OccurrenceStarted)
	if err != nil {
		return "", false, err
	}
	rows, err := updated.RowsAffected()
	if err != nil {
		return "", false, err
	}
	if rows != 1 {
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
	if _, err := tx.Exec(`
		INSERT INTO result_outbox (id, kind, payload, created_at)
		VALUES (?, 'ACTION', ?, ?)
	`, id, payload, now); err != nil {
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
	stable.DurationMs = 0
	stable.RunId = ""
	stable.OccurrenceId = ""
	encoded, err := canonicalProtoBytes(stable)
	if err != nil {
		return "", fmt.Errorf("hash action result: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func (s *Store) RecordManifestResult(result *pb.ManifestResult) (string, error) {
	if result == nil || result.GetRunId() == "" || result.GetManifestId() == "" {
		return "", errors.New("record manifest result: missing identity")
	}
	return s.recordResult("MANIFEST", result, func(tx *sql.Tx, _ time.Time) error {
		updated, err := tx.Exec(`
			UPDATE scheduled_work
			SET run_in_progress = FALSE, run_started_at = NULL
			WHERE (work_id = ? OR run_id = ?) AND run_in_progress = TRUE
		`, result.GetRunId(), result.GetRunId())
		if err != nil {
			return err
		}
		rows, err := updated.RowsAffected()
		if err != nil {
			return err
		}
		if rows != 1 {
			return errors.New("record manifest result: manifest run is not active")
		}
		_, err = tx.Exec(`DELETE FROM scheduled_work WHERE (work_id = ? OR run_id = ?) AND retired = TRUE`, result.GetRunId(), result.GetRunId())
		if err != nil {
			return err
		}
		_, err = tx.Exec(`UPDATE scheduled_work SET run_id = NULL WHERE (work_id = ? OR run_id = ?)`, result.GetRunId(), result.GetRunId())
		return err
	})
}

func (s *Store) recordResult(kind string, message proto.Message, update func(*sql.Tx, time.Time) error) (string, error) {
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
	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if update != nil {
		if err := update(tx, now); err != nil {
			return "", err
		}
	}
	if _, err := tx.Exec(`
		INSERT INTO result_outbox (id, kind, payload, created_at)
		VALUES (?, ?, ?, ?)
	`, id, kind, payload, now); err != nil {
		return "", err
	}
	return id, tx.Commit()
}

func (s *Store) GetPendingResults() ([]PendingResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query(`
		SELECT id, kind, payload FROM result_outbox
		WHERE synced = FALSE ORDER BY sequence
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pending []PendingResult
	for rows.Next() {
		var item PendingResult
		var kind string
		var payload []byte
		if err := rows.Scan(&item.ID, &kind, &payload); err != nil {
			return nil, err
		}
		switch kind {
		case "ACTION":
			item.ActionResult = &pb.ActionResult{}
			if err := unmarshalStoredProto(payload, item.ActionResult); err != nil {
				return nil, fmt.Errorf("decode action result %s: %w", item.ID, err)
			}
		case "MANIFEST":
			item.ManifestResult = &pb.ManifestResult{}
			if err := unmarshalStoredProto(payload, item.ManifestResult); err != nil {
				return nil, fmt.Errorf("decode manifest result %s: %w", item.ID, err)
			}
		default:
			return nil, fmt.Errorf("unknown result outbox kind %q", kind)
		}
		pending = append(pending, item)
	}
	return pending, rows.Err()
}

func (s *Store) MarkPendingResultSynced(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec("UPDATE result_outbox SET synced = TRUE WHERE id = ?", id)
	return err
}

// RecoverInterruptedOccurrences resolves durable STARTED rows without ever
// repeating their side effects.
func (s *Store) RecoverInterruptedOccurrences() ([]PendingResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`
		SELECT COALESCE(sw.run_id, o.work_id), o.work_id, o.occurrence_id, o.action_id
		FROM scheduled_work_occurrences o
		JOIN scheduled_work sw ON sw.work_id = o.work_id
		WHERE o.state = ?
		ORDER BY o.work_id, o.position
	`, OccurrenceStarted)
	if err != nil {
		return nil, err
	}
	type interrupted struct {
		runID, workID, occurrenceID, actionID string
	}
	var interruptedRows []interrupted
	for rows.Next() {
		var item interrupted
		if err := rows.Scan(&item.runID, &item.workID, &item.occurrenceID, &item.actionID); err != nil {
			rows.Close()
			return nil, err
		}
		interruptedRows = append(interruptedRows, item)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	var recovered []PendingResult
	for _, item := range interruptedRows {
		status := pb.ExecutionStatus_EXECUTION_STATUS_INDETERMINATE
		message := "agent restarted after STARTED; effect is unknown and was not repeated"
		result := &pb.ActionResult{
			ActionId:     &pb.ActionId{Value: item.actionID},
			RunId:        item.runID,
			OccurrenceId: item.occurrenceID,
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
		if _, err := tx.Exec(`INSERT INTO result_outbox (id, kind, payload, created_at) VALUES (?, 'ACTION', ?, ?)`, id, payload, now); err != nil {
			return nil, err
		}
		state, err := occurrenceState(status)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`
			UPDATE scheduled_work_occurrences
			SET state = ?, completed_at = ?, result_status = ?, result_error = ?
			WHERE work_id = ? AND occurrence_id = ? AND state = ?
		`, state, now, status, message, item.workID, item.occurrenceID, OccurrenceStarted); err != nil {
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
		pb.ExecutionStatus_EXECUTION_STATUS_CANCELLED,
		pb.ExecutionStatus_EXECUTION_STATUS_SKIPPED,
		pb.ExecutionStatus_EXECUTION_STATUS_NOT_APPLICABLE:
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

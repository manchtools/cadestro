package store

import (
	"bytes"
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
	Delivery      *pb.ManifestDelivery
	WorkID        string
	RunID         string
	Kind          string
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
	if err := s.db.QueryRow(`SELECT delivery_id FROM scheduled_work WHERE delivery_id = ? OR run_id = ?`, id, id).Scan(&workID); err != nil {
		return "", err
	}
	return workID, nil
}

// ReconcilePolicy replaces only assignment-derived manifests. Explicit
// deliveries remain untouched; authenticated Sync is the policy boundary.
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
	rows, err := tx.Query("SELECT delivery_id, run_in_progress FROM scheduled_work WHERE kind = 'policy' AND retired = FALSE")
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
			if _, err := tx.Exec("UPDATE scheduled_work SET retired = TRUE WHERE delivery_id = ? AND kind = 'policy'", work.id); err != nil {
				return fmt.Errorf("reconcile policy: retire %s: %w", work.id, err)
			}
		} else if _, err := tx.Exec("DELETE FROM scheduled_work WHERE delivery_id = ? AND kind = 'policy'", work.id); err != nil {
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
		err = tx.QueryRow("SELECT 1 FROM scheduled_work WHERE delivery_id = ? AND kind = 'policy'", id).Scan(&existing)
		if err == nil {
			if _, err := tx.Exec("UPDATE scheduled_work SET retired = FALSE WHERE delivery_id = ?", id); err != nil {
				return fmt.Errorf("reconcile policy: revive %s: %w", id, err)
			}
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO scheduled_work
			(delivery_id, run_id, manifest_blob, kind, retired, received_at, next_execute_at)
			VALUES (?, ?, ?, 'policy', FALSE, ?, ?)`, id, id, blob, now, calculateNextExecuteFromSchedule(manifest.GetSchedule(), nil, false, now)); err != nil {
			return fmt.Errorf("reconcile policy: insert %s: %w", id, err)
		}
		for position, occurrence := range manifest.GetOccurrences() {
			if occurrence == nil || occurrence.GetOccurrenceId() == "" || occurrence.GetAction().GetId().GetValue() == "" {
				return errors.New("reconcile policy: malformed occurrence")
			}
			if _, err := tx.Exec(`INSERT INTO scheduled_work_occurrences
				(delivery_id, occurrence_id, position, action_id) VALUES (?, ?, ?, ?)`,
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

// RecordManifestDelivery commits a delivery and every authored occurrence in
// one transaction. A replay with identical bytes is accepted without changing
// its schedule or execution state; the same delivery ID with different bytes
// is rejected.
func (s *Store) RecordManifestDelivery(ctx context.Context, delivery *pb.ManifestDelivery) (bool, error) {
	if delivery == nil || delivery.GetDeliveryId() == "" || delivery.GetManifest() == nil {
		return false, errors.New("record manifest delivery: missing delivery identity or manifest")
	}
	manifest := delivery.GetManifest()
	if len(manifest.GetOccurrences()) == 0 {
		return false, errors.New("record manifest delivery: manifest has no occurrences")
	}
	seen := make(map[string]struct{}, len(manifest.GetOccurrences()))
	for _, occurrence := range manifest.GetOccurrences() {
		if occurrence == nil || occurrence.GetOccurrenceId() == "" || occurrence.GetAction().GetId().GetValue() == "" {
			return false, errors.New("record manifest delivery: malformed occurrence")
		}
		if _, exists := seen[occurrence.GetOccurrenceId()]; exists {
			return false, fmt.Errorf("record manifest delivery: duplicate occurrence %s", occurrence.GetOccurrenceId())
		}
		seen[occurrence.GetOccurrenceId()] = struct{}{}
	}

	blob, err := marshalStoredProto(manifest)
	if err != nil {
		return false, fmt.Errorf("record manifest delivery: marshal manifest: %w", err)
	}
	now := s.now().UTC()
	next := calculateNextExecuteFromSchedule(manifest.GetSchedule(), nil, false, now)

	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("record manifest delivery: begin: %w", err)
	}
	defer tx.Rollback()

	var existing []byte
	err = tx.QueryRow("SELECT manifest_blob FROM scheduled_work WHERE delivery_id = ?", delivery.GetDeliveryId()).Scan(&existing)
	if err == nil {
		if !bytes.Equal(existing, blob) {
			return false, errors.New("record manifest delivery: delivery ID replayed with different manifest")
		}
		return false, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("record manifest delivery: lookup: %w", err)
	}

	if _, err := tx.Exec(`
		INSERT INTO scheduled_work
		    (delivery_id, run_id, manifest_blob, kind, received_at, next_execute_at)
		VALUES (?, ?, ?, 'delivery', ?, ?)
	`, delivery.GetDeliveryId(), delivery.GetDeliveryId(), blob, now, next); err != nil {
		return false, fmt.Errorf("record manifest delivery: insert: %w", err)
	}
	for position, occurrence := range manifest.GetOccurrences() {
		if _, err := tx.Exec(`
			INSERT INTO scheduled_work_occurrences
			    (delivery_id, occurrence_id, position, action_id)
			VALUES (?, ?, ?, ?)
		`, delivery.GetDeliveryId(), occurrence.GetOccurrenceId(), position, occurrence.GetAction().GetId().GetValue()); err != nil {
			return false, fmt.Errorf("record manifest occurrence: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("record manifest delivery: commit: %w", err)
	}
	return true, nil
}

func (s *Store) GetDueScheduledWork(ctx context.Context) ([]ScheduledWork, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.QueryContext(ctx, `
		SELECT delivery_id, kind, COALESCE(run_id, ''), manifest_blob,
		       received_at, last_executed_at, next_execute_at,
		       run_started_at, run_in_progress
		FROM scheduled_work
		WHERE (retired = FALSE OR run_in_progress = TRUE)
		  AND (run_in_progress = TRUE
		       OR (next_execute_at <= ? AND one_shot_run_at IS NULL))
		ORDER BY run_in_progress DESC, next_execute_at, delivery_id
	`, s.now().UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deliveries []ScheduledWork
	for rows.Next() {
		var deliveryID, kind, runID string
		var blob []byte
		var stored ScheduledWork
		var last, runStarted sql.NullTime
		if err := rows.Scan(
			&deliveryID, &kind, &runID, &blob, &stored.ReceivedAt, &last,
			&stored.NextExecuteAt, &runStarted, &stored.RunInProgress,
		); err != nil {
			return nil, err
		}
		manifest := &pb.Manifest{}
		if err := unmarshalStoredProto(blob, manifest); err != nil {
			return nil, fmt.Errorf("decode manifest delivery %s: %w", deliveryID, err)
		}
		if runID == "" {
			runID = deliveryID
		}
		stored.WorkID, stored.RunID, stored.Kind = deliveryID, runID, kind
		stored.Delivery = &pb.ManifestDelivery{DeliveryId: runID, Manifest: manifest}
		if last.Valid {
			lastTime := last.Time
			stored.LastExecuted = &lastTime
		}
		if runStarted.Valid {
			runStartedTime := runStarted.Time
			stored.RunStartedAt = &runStartedTime
		}
		deliveries = append(deliveries, stored)
	}
	return deliveries, rows.Err()
}

func (s *Store) GetManifestActions() ([]*StoredAction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query(`
		SELECT delivery_id, manifest_blob, received_at, last_executed_at, next_execute_at
		FROM scheduled_work WHERE retired = FALSE ORDER BY received_at, delivery_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var actions []*StoredAction
	for rows.Next() {
		var deliveryID string
		var blob []byte
		var received, next time.Time
		var last sql.NullTime
		if err := rows.Scan(&deliveryID, &blob, &received, &last, &next); err != nil {
			return nil, err
		}
		manifest := &pb.Manifest{}
		if err := unmarshalStoredProto(blob, manifest); err != nil {
			return nil, fmt.Errorf("decode manifest delivery %s: %w", deliveryID, err)
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
func (s *Store) BeginManifestRun(delivery *pb.ManifestDelivery, startedAt time.Time) (time.Time, error) {
	if delivery == nil || delivery.GetManifest() == nil {
		return time.Time{}, errors.New("begin manifest run: missing delivery")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return time.Time{}, err
	}
	defer tx.Rollback()
	var inProgress bool
	var kind, runID, workID string
	var priorStarted sql.NullTime
	if err := tx.QueryRow(`
		SELECT delivery_id, kind, COALESCE(run_id, ''), run_in_progress, run_started_at
		FROM scheduled_work
		WHERE (delivery_id = ? OR run_id = ?)
		  AND (retired = FALSE OR run_in_progress = TRUE)
	`, delivery.GetDeliveryId(), delivery.GetDeliveryId()).Scan(&workID, &kind, &runID, &inProgress, &priorStarted); err != nil {
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
		WHERE delivery_id = ? AND state = ?
	`, workID, OccurrenceStarted).Scan(&started); err != nil {
		return time.Time{}, err
	}
	if started != 0 {
		return time.Time{}, fmt.Errorf("begin manifest run: delivery %s has interrupted occurrences", delivery.GetDeliveryId())
	}
	if _, err := tx.Exec(`
		UPDATE scheduled_work_occurrences
		SET state = ?, started_at = NULL, completed_at = NULL,
		    result_status = NULL, result_error = ''
		WHERE delivery_id = ?
	`, OccurrencePending, workID); err != nil {
		return time.Time{}, err
	}
	startedAt = startedAt.UTC()
	next := calculateNextExecuteFromSchedule(delivery.GetManifest().GetSchedule(), &startedAt, false, s.now())
	// Starting a one-shot run is what makes it terminal, and it is recorded in
	// the same transaction as the cursor it overrides. An interrupted run
	// returned above without reaching here, so a crash still resumes from
	// run_in_progress rather than being written off as already run.
	oneShotRunAt := sql.NullTime{Time: startedAt, Valid: delivery.GetManifest().GetOneShot()}
	if kind == "policy" && (runID == "" || runID == workID) {
		runID = ulid.Make().String()
	}
	if _, err := tx.Exec(`
		UPDATE scheduled_work
		SET run_id = ?, last_executed_at = ?, next_execute_at = ?,
		    run_started_at = ?, run_in_progress = TRUE,
		    one_shot_run_at = ?
		WHERE delivery_id = ?
	`, runID, startedAt, next, startedAt, oneShotRunAt, workID); err != nil {
		return time.Time{}, err
	}
	if err := tx.Commit(); err != nil {
		return time.Time{}, err
	}
	delivery.DeliveryId = runID
	return startedAt, nil
}

func (s *Store) MarkOccurrenceStarted(deliveryID, occurrenceID string, startedAt time.Time) error {
	workID, err := s.resolveWorkID(deliveryID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := s.db.Exec(`
		UPDATE scheduled_work_occurrences SET state = ?, started_at = ?, completed_at = NULL
		WHERE delivery_id = ? AND occurrence_id = ? AND state = ?
	`, OccurrenceStarted, startedAt.UTC(), workID, occurrenceID, OccurrencePending)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return fmt.Errorf("mark occurrence started: invalid state for %s/%s", deliveryID, occurrenceID)
	}
	return nil
}

func (s *Store) GetManifestOccurrenceStates(deliveryID string) (map[string]ManifestOccurrenceState, error) {
	workID, err := s.resolveWorkID(deliveryID)
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query(`
		SELECT occurrence_id, state, result_status, result_error
		FROM scheduled_work_occurrences WHERE delivery_id = ? ORDER BY position
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
	if result == nil || result.GetDeliveryId() == "" || result.GetOccurrenceId() == "" {
		return "", false, errors.New("record occurrence result: missing delivery or occurrence identity")
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
	workID, err := s.resolveWorkID(result.GetDeliveryId())
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
		WHERE delivery_id = ? AND occurrence_id = ? AND state = ?
	`, workID, result.GetOccurrenceId(), OccurrenceStarted).Scan(&previousHash); err != nil {
		return "", false, err
	}
	updated, err := tx.Exec(`
		UPDATE scheduled_work_occurrences
		SET state = ?, completed_at = ?, result_status = ?, result_error = ?, last_result_hash = ?
		WHERE delivery_id = ? AND occurrence_id = ? AND state = ?
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
	stable.DeliveryId = ""
	stable.OccurrenceId = ""
	encoded, err := canonicalProtoBytes(stable)
	if err != nil {
		return "", fmt.Errorf("hash action result: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func (s *Store) RecordManifestResult(result *pb.ManifestResult) (string, error) {
	if result == nil || result.GetDeliveryId() == "" || result.GetManifestId() == "" {
		return "", errors.New("record manifest result: missing identity")
	}
	return s.recordResult("MANIFEST", result, func(tx *sql.Tx, _ time.Time) error {
		updated, err := tx.Exec(`
			UPDATE scheduled_work
			SET run_in_progress = FALSE, run_started_at = NULL
			WHERE (delivery_id = ? OR run_id = ?) AND run_in_progress = TRUE
		`, result.GetDeliveryId(), result.GetDeliveryId())
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
		_, err = tx.Exec(`DELETE FROM scheduled_work WHERE (delivery_id = ? OR run_id = ?) AND kind = 'policy' AND retired = TRUE`, result.GetDeliveryId(), result.GetDeliveryId())
		if err != nil {
			return err
		}
		_, err = tx.Exec(`UPDATE scheduled_work SET run_id = NULL WHERE (delivery_id = ? OR run_id = ?) AND kind = 'policy'`, result.GetDeliveryId(), result.GetDeliveryId())
		return nil
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
		SELECT COALESCE(sw.run_id, o.delivery_id), o.delivery_id, o.occurrence_id, o.action_id
		FROM scheduled_work_occurrences o
		JOIN scheduled_work sw ON sw.delivery_id = o.delivery_id
		WHERE o.state = ?
		ORDER BY o.delivery_id, o.position
	`, OccurrenceStarted)
	if err != nil {
		return nil, err
	}
	type interrupted struct {
		runID, deliveryID, occurrenceID, actionID string
	}
	var interruptedRows []interrupted
	for rows.Next() {
		var item interrupted
		if err := rows.Scan(&item.runID, &item.deliveryID, &item.occurrenceID, &item.actionID); err != nil {
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
			DeliveryId:   item.runID,
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
			WHERE delivery_id = ? AND occurrence_id = ? AND state = ?
		`, state, now, status, message, item.deliveryID, item.occurrenceID, OccurrenceStarted); err != nil {
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

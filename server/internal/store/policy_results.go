package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/oklog/ulid/v2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

var ErrPolicyResultConflict = errors.New("policy result conflicts with stored replay")

func (s *Store) RecordPolicyActionResult(ctx context.Context, deviceID string, result *pb.ActionResult) error {
	if result == nil || deviceID == "" || result.GetDeliveryId() == "" || result.GetOccurrenceId() == "" {
		return errors.New("policy action result: missing identity")
	}
	payload, err := protojson.Marshal(result)
	if err != nil {
		return fmt.Errorf("policy action result: marshal: %w", err)
	}
	hashBytes, err := proto.MarshalOptions{Deterministic: true}.Marshal(result)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(hashBytes)
	hash := hex.EncodeToString(digest[:])
	now := s.now().UTC()
	return s.recordPolicyResult(ctx, deviceID, func(tx *sql.Tx) error {
		var existing, existingDevice string
		err := tx.QueryRow(`SELECT result_hash, device_id FROM policy_action_results WHERE run_id = ? AND occurrence_id = ?`, result.GetDeliveryId(), result.GetOccurrenceId()).Scan(&existing, &existingDevice)
		if err == nil {
			if existingDevice != deviceID || existing != hash {
				return ErrPolicyResultConflict
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		_, err = tx.Exec(`INSERT INTO policy_action_results
			(run_id, occurrence_id, device_id, action_id, result_hash, payload, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, result.GetDeliveryId(), result.GetOccurrenceId(), deviceID,
			result.GetActionId().GetValue(), hash, payload, now)
		return err
	})
}

func (s *Store) RecordPolicyManifestResult(ctx context.Context, deviceID, runID, manifestID, state, code string) error {
	if deviceID == "" || runID == "" || manifestID == "" || state == "" || code == "" {
		return errors.New("policy manifest result: missing identity")
	}
	now := s.now().UTC()
	return s.recordPolicyResult(ctx, deviceID, func(tx *sql.Tx) error {
		var existingState, existingCode, existingDevice, existingManifest string
		err := tx.QueryRow(`SELECT state, result_code, device_id, manifest_id FROM policy_manifest_results WHERE run_id = ?`, runID).Scan(&existingState, &existingCode, &existingDevice, &existingManifest)
		if err == nil {
			if existingDevice != deviceID || existingManifest != manifestID || existingState != state || existingCode != code {
				return ErrPolicyResultConflict
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		_, err = tx.Exec(`INSERT INTO policy_manifest_results
			(run_id, device_id, manifest_id, state, result_code, created_at)
			VALUES (?, ?, ?, ?, ?, ?)`, runID, deviceID, manifestID, state, code, now)
		return err
	})
}

func (s *Store) recordPolicyResult(ctx context.Context, actorID string, write func(*sql.Tx) error) error {
	operationID := ulid.Make().String()
	_, err := s.WithAudit(ctx, AuditOperation{
		OperationID: operationID, Class: ClassMutation, ActorType: "agent", ActorID: actorID,
		Origin: "agent_stream", RequestDescriptor: "agent.policy_result",
		AuthorizationOutcome: AuthorizationAllowed, AuthorizationDetail: "device_mtls",
		Result: ResultSuccess, ResultCode: "OK",
	}, func(_ context.Context, tx *Tx, _ *AuditRecorder) error {
		return write(tx.raw)
	})
	return err
}

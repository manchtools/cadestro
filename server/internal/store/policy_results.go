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

	"github.com/manchtools/cadestro/server/internal/store/generated"
	"github.com/manchtools/cadestro/server/internal/store/sqlitetype"
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
	return s.recordPolicyResult(ctx, deviceID, func(ctx context.Context, tx *Tx) error {
		existing, err := tx.GetPolicyActionResult(ctx, generated.GetPolicyActionResultParams{
			RunID: result.GetDeliveryId(), OccurrenceID: result.GetOccurrenceId(),
		})
		if err == nil {
			if existing.DeviceID != deviceID || existing.ResultHash != hash {
				return ErrPolicyResultConflict
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return tx.InsertPolicyActionResult(ctx, generated.InsertPolicyActionResultParams{
			RunID: result.GetDeliveryId(), OccurrenceID: result.GetOccurrenceId(), DeviceID: deviceID,
			ActionID: result.GetActionId().GetValue(), ResultHash: hash,
			Payload: sqlitetype.JSON(payload), CreatedAt: now,
		})
	})
}

func (s *Store) RecordPolicyManifestResult(ctx context.Context, deviceID, runID, manifestID, state, code string) error {
	if deviceID == "" || runID == "" || manifestID == "" || state == "" || code == "" {
		return errors.New("policy manifest result: missing identity")
	}
	now := s.now().UTC()
	return s.recordPolicyResult(ctx, deviceID, func(ctx context.Context, tx *Tx) error {
		existing, err := tx.GetPolicyManifestResult(ctx, runID)
		if err == nil {
			if existing.DeviceID != deviceID || existing.ManifestID != manifestID ||
				existing.State != state || existing.ResultCode != code {
				return ErrPolicyResultConflict
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return tx.InsertPolicyManifestResult(ctx, generated.InsertPolicyManifestResultParams{
			RunID: runID, DeviceID: deviceID, ManifestID: manifestID, State: state,
			ResultCode: code, CreatedAt: now,
		})
	})
}

func (s *Store) recordPolicyResult(ctx context.Context, actorID string, write func(context.Context, *Tx) error) error {
	operationID := ulid.Make().String()
	_, err := s.WithAudit(ctx, AuditOperation{
		OperationID: operationID, Class: ClassMutation, ActorType: "agent", ActorID: actorID,
		Origin: "agent_stream", RequestDescriptor: "agent.policy_result",
		AuthorizationOutcome: AuthorizationAllowed, AuthorizationDetail: "device_mtls",
		Result: ResultSuccess, ResultCode: "OK",
	}, func(ctx context.Context, tx *Tx, _ *AuditRecorder) error {
		return write(ctx, tx)
	})
	return err
}

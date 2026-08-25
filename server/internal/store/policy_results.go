package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	pb "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/oklog/ulid/v2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/manchtools/cadestro/server/internal/actionparams"
	"github.com/manchtools/cadestro/server/internal/store/generated"
	"github.com/manchtools/cadestro/server/internal/store/sqlitetype"
)

var ErrPolicyResultConflict = errors.New("policy result conflicts with stored replay")

func (s *Store) RecordPolicyActionResult(ctx context.Context, deviceID string, result *pb.ActionResult) error {
	if result == nil || deviceID == "" || result.GetRunId().GetValue() == "" || result.GetOccurrenceId().GetValue() == "" {
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
	checkedAt := now
	if result.CompletedAt != nil {
		if err := result.CompletedAt.CheckValid(); err == nil {
			checkedAt = result.CompletedAt.AsTime().UTC()
		}
	}
	return s.recordPolicyResult(ctx, deviceID, func(ctx context.Context, tx *Tx, rec *AuditRecorder) error {
		existing, err := tx.GetPolicyActionResult(ctx, generated.GetPolicyActionResultParams{
			RunID: result.GetRunId().GetValue(), OccurrenceID: result.GetOccurrenceId().GetValue(),
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
		if err := tx.InsertPolicyActionResult(ctx, generated.InsertPolicyActionResultParams{
			RunID: result.GetRunId().GetValue(), OccurrenceID: result.GetOccurrenceId().GetValue(), DeviceID: deviceID,
			ActionID: result.GetActionId().GetValue(), ResultHash: hash,
			Payload: sqlitetype.JSON(payload), CreatedAt: now,
		}); err != nil {
			return err
		}
		return recordPolicyCompliance(ctx, tx, rec, deviceID, result, checkedAt)
	})
}
func (s *Store) RecordPolicyManifestResult(ctx context.Context, deviceID, runID, manifestID, state, code string) error {
	if deviceID == "" || runID == "" || manifestID == "" || state == "" || code == "" {
		return errors.New("policy manifest result: missing identity")
	}
	now := s.now().UTC()
	return s.recordPolicyResult(ctx, deviceID, func(ctx context.Context, tx *Tx, _ *AuditRecorder) error {
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

func (s *Store) recordPolicyResult(ctx context.Context, actorID string, write func(context.Context, *Tx, *AuditRecorder) error) error {
	operationID := ulid.Make().String()
	_, err := s.WithAudit(ctx, AuditOperation{
		OperationID: operationID, Class: ClassMutation, ActorType: "agent", ActorID: actorID,
		Origin: "agent_stream", RequestDescriptor: "agent.policy_result",
		AuthorizationOutcome: AuthorizationAllowed, AuthorizationDetail: "device_mtls",
		Result: ResultSuccess, ResultCode: "OK",
	}, func(ctx context.Context, tx *Tx, rec *AuditRecorder) error {
		return write(ctx, tx, rec)
	})
	return err
}

func recordPolicyCompliance(ctx context.Context, tx *Tx, rec *AuditRecorder, deviceID string, result *pb.ActionResult, checkedAt time.Time) error {
	action, err := tx.GetManifestAction(ctx, result.GetActionId().GetValue())
	if err != nil {
		if IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("read compliance action: %w", err)
	}
	if pb.ActionType(action.ActionType) != pb.ActionType_ACTION_TYPE_SHELL {
		return nil
	}
	managed := &pb.ManagedAction{Type: pb.ActionType(action.ActionType)}
	if err := actionparams.PopulateManagedAction(managed, managed.Type, action.Params); err != nil {
		return fmt.Errorf("decode compliance action: %w", err)
	}
	shell := managed.GetShell()
	if shell == nil || !shell.IsCompliance || strings.TrimSpace(shell.DetectionScript) == "" {
		return nil
	}
	compliant := result.Compliant && result.Status == pb.ExecutionStatus_EXECUTION_STATUS_SUCCESS
	detection, err := protojson.Marshal(result.DetectionOutput)
	if result.DetectionOutput == nil {
		detection = nil
	}
	if err != nil {
		return fmt.Errorf("marshal detection output: %w", err)
	}
	if n, err := tx.UpsertDeviceComplianceResult(ctx, generated.UpsertDeviceComplianceResultParams{
		DeviceID: deviceID, ActionID: action.ID, Compliant: compliant,
		DetectionOutput: sqlitetype.JSON(detection), CheckedAt: checkedAt,
	}); err != nil {
		return fmt.Errorf("record compliance result: %w", err)
	} else if n != 1 {
		return fmt.Errorf("record compliance result: action %s is not live", action.ID)
	}
	targets, err := tx.ListComplianceRuleEvaluationTargets(ctx, generated.ListComplianceRuleEvaluationTargetsParams{DeviceID: deviceID, ActionID: action.ID})
	if err != nil {
		return fmt.Errorf("list compliance rules: %w", err)
	}
	for _, target := range targets {
		firstFailedAt := target.FirstFailedAt
		switch {
		case compliant:
			firstFailedAt = nil
		case firstFailedAt == nil:
			firstFailedAt = &checkedAt
		}
		status := int32(pb.ComplianceStatus_COMPLIANCE_STATUS_NON_COMPLIANT)
		if compliant {
			status = int32(pb.ComplianceStatus_COMPLIANCE_STATUS_COMPLIANT)
		} else if firstFailedAt != nil && target.GracePeriodHours > 0 && checkedAt.Before(firstFailedAt.Add(time.Duration(target.GracePeriodHours)*time.Hour)) {
			status = int32(pb.ComplianceStatus_COMPLIANCE_STATUS_IN_GRACE_PERIOD)
		}
		if err := tx.UpsertCompliancePolicyEvaluation(ctx, generated.UpsertCompliancePolicyEvaluationParams{
			DeviceID: deviceID, PolicyID: target.PolicyID, ActionID: action.ID, Compliant: compliant,
			FirstFailedAt: firstFailedAt, Status: status, CheckedAt: &checkedAt,
		}); err != nil {
			return fmt.Errorf("record compliance evaluation: %w", err)
		}
	}
	if _, err := tx.RefreshDeviceComplianceStatus(ctx, generated.RefreshDeviceComplianceStatusParams{DeviceID: deviceID, CheckedAt: &checkedAt}); err != nil {
		return fmt.Errorf("refresh device compliance status: %w", err)
	}
	rec.Effect(AuditEffect{ResourceType: "device", ResourceID: deviceID, Action: "COMPLIANCE", Outcome: EffectApplied, AfterRef: &action.ID, AfterFlag: &compliant, ChangedFields: []string{"compliance_status", "compliance_checked_at", "compliance_total", "compliance_passing"}})
	return nil
}

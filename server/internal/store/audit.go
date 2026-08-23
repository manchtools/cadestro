package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/manchtools/cadestro/server/internal/store/generated"
	"github.com/manchtools/cadestro/server/internal/store/sqlitetype"
)

const DefaultAuditStream = "control"

type OperationClass string

const (
	ClassMutation               OperationClass = "MUTATION"
	ClassSensitiveRead          OperationClass = "SENSITIVE_READ"
	ClassRejectedAuthentication OperationClass = "REJECTED_AUTHENTICATION"
	ClassBackgroundWriter       OperationClass = "BACKGROUND_WRITER"
)

type AuthorizationOutcome string

const (
	AuthorizationAllowed       AuthorizationOutcome = "ALLOWED"
	AuthorizationDenied        AuthorizationOutcome = "DENIED"
	AuthorizationNotApplicable AuthorizationOutcome = "NOT_APPLICABLE"
)

type OperationResult string

const (
	ResultSuccess  OperationResult = "SUCCESS"
	ResultFailure  OperationResult = "FAILURE"
	ResultRejected OperationResult = "REJECTED"
)

type EffectOutcome string

const (
	EffectApplied  EffectOutcome = "APPLIED"
	EffectRejected EffectOutcome = "REJECTED"
	EffectFailed   EffectOutcome = "FAILED"
)

var (
	ErrAuditOperationRequired = errors.New("audit operation required")
	ErrAuditEffectRequired    = errors.New("audit effect required")
	ErrAuditEffectInvalid     = errors.New("audit effect is invalid")
)

type AuditOperation struct {
	OperationID          string
	Stream               string
	Class                OperationClass
	ActorType            string
	ActorID              string
	ActorFingerprint     string
	Origin               string
	OriginFingerprint    string
	RequestDescriptor    string
	AuthorizationOutcome AuthorizationOutcome
	AuthorizationDetail  string
	Result               OperationResult
	ResultCode           string
	SealedDetail         []byte
	SealedDetailSubject  string
}

type AuditEffect struct {
	ResourceType        string
	ResourceID          string
	Action              string
	Outcome             EffectOutcome
	ChangedFields       []string
	BeforeRef           *string
	AfterRef            *string
	BeforeFlag          *bool
	AfterFlag           *bool
	BeforeCount         *int64
	AfterCount          *int64
	EvidenceKind        string
	EvidenceFingerprint string
	SealedDetail        []byte
	SealedDetailSubject string
}

type AuditRecorder struct {
	effects       []AuditEffect
	searchTouches []searchTouch
}

func (r *AuditRecorder) Effect(e AuditEffect) { r.effects = append(r.effects, e) }
func (r *AuditRecorder) RefreshSearch(resourceType, resourceID string) {
	r.searchTouches = append(r.searchTouches, searchTouch{resourceType: resourceType, resourceID: resourceID})
}
func (r *AuditRecorder) Len() int { return len(r.effects) }

type AuditRecord struct {
	OperationID  string
	Stream       string
	OperationSeq int64
	EffectSeqs   []int64
	HeadSeq      int64
}

func (s *Store) WithAudit(ctx context.Context, op AuditOperation, mutate func(context.Context, *Tx, *AuditRecorder) error) (AuditRecord, error) {
	if err := op.validate(); err != nil {
		return AuditRecord{}, err
	}
	if op.OperationID == "" {
		op.OperationID = ulid.Make().String()
	}
	if op.Stream == "" {
		op.Stream = DefaultAuditStream
	}

	var rec AuditRecorder
	var out AuditRecord
	err := s.withTx(ctx, func(raw *sql.Tx, q *generated.Queries) error {
		tx := q
		if mutate != nil {
			if err := mutate(ctx, &Tx{Queries: tx, raw: raw}, &rec); err != nil {
				return err
			}
		}
		if err := refreshSearchDocumentsForEffects(ctx, q, rec.effects, rec.searchTouches); err != nil {
			return err
		}
		seq, err := tx.NextAuditEventSeq(ctx, op.Stream)
		if err != nil {
			return fmt.Errorf("audit: next event sequence: %w", err)
		}
		at := s.auditNow()
		row, err := tx.InsertAuditOperation(ctx, op.insertParams(seq, at))
		if err != nil {
			return fmt.Errorf("audit: insert operation: %w", err)
		}
		out.OperationSeq = row.ChainSeq
		for i, e := range rec.effects {
			if err := e.validate(); err != nil {
				return fmt.Errorf("audit: effect %d: %w", i, err)
			}
			seq, err = tx.NextAuditEventSeq(ctx, op.Stream)
			if err != nil {
				return fmt.Errorf("audit: next event sequence: %w", err)
			}
			effectID := ulid.Make().String()
			_, err = tx.InsertAuditEffect(ctx, e.insertParams(op.Stream, op.OperationID, effectID, seq, int64(i), at))
			if err != nil {
				return fmt.Errorf("audit: insert effect %d: %w", i, err)
			}
			out.EffectSeqs = append(out.EffectSeqs, seq)
		}
		if err := refreshSearchDocument(ctx, q, "audit_events", op.OperationID); err != nil {
			return err
		}
		out.OperationID, out.Stream, out.HeadSeq = op.OperationID, op.Stream, seq
		return nil
	})
	if err != nil {
		return AuditRecord{}, err
	}
	return out, nil
}

func (s *Store) RecordOperation(ctx context.Context, op AuditOperation, effects ...AuditEffect) (AuditRecord, error) {
	for i, e := range effects {
		if err := e.validate(); err != nil {
			return AuditRecord{}, fmt.Errorf("audit: effect %d: %w", i, err)
		}
	}
	return s.WithAudit(ctx, op, func(_ context.Context, _ *Tx, rec *AuditRecorder) error {
		for _, e := range effects {
			rec.Effect(e)
		}
		return nil
	})
}

func (s *Store) WithAuditEffects(ctx context.Context, operationID string, mutate func(context.Context, *Tx, *AuditRecorder) error) (AuditRecord, error) {
	if operationID == "" {
		return AuditRecord{}, fmt.Errorf("%w: continuation needs the operation it belongs to", ErrAuditOperationRequired)
	}
	var rec AuditRecorder
	var out AuditRecord
	err := s.withTx(ctx, func(raw *sql.Tx, q *generated.Queries) error {
		parent, err := q.GetAuditOperation(ctx, operationID)
		if err != nil {
			return fmt.Errorf("%w: operation not found", ErrAuditOperationRequired)
		}
		if mutate != nil {
			if err := mutate(ctx, &Tx{Queries: q, raw: raw}, &rec); err != nil {
				return err
			}
		}
		if len(rec.effects) == 0 {
			return fmt.Errorf("%w: a continuation of %s recorded nothing", ErrAuditEffectRequired, operationID)
		}
		if err := refreshSearchDocumentsForEffects(ctx, q, rec.effects, rec.searchTouches); err != nil {
			return err
		}
		nextEffect, err := q.NextAuditEffectSeq(ctx, operationID)
		if err != nil {
			return fmt.Errorf("audit: next effect sequence: %w", err)
		}
		at := s.auditNow()
		var seq int64
		for i, e := range rec.effects {
			if err := e.validate(); err != nil {
				return fmt.Errorf("audit: effect %d: %w", i, err)
			}
			seq, err = q.NextAuditEventSeq(ctx, parent.Stream)
			if err != nil {
				return fmt.Errorf("audit: next event sequence: %w", err)
			}
			_, err = q.InsertAuditEffect(ctx, e.insertParams(parent.Stream, operationID, ulid.Make().String(), seq, nextEffect+int64(i), at))
			if err != nil {
				return fmt.Errorf("audit: insert effect %d: %w", i, err)
			}
			out.EffectSeqs = append(out.EffectSeqs, seq)
		}
		if err := refreshSearchDocument(ctx, q, "audit_events", operationID); err != nil {
			return err
		}
		out.OperationID, out.Stream, out.HeadSeq = operationID, parent.Stream, seq
		return nil
	})
	if err != nil {
		return AuditRecord{}, err
	}
	return out, nil
}

func (op AuditOperation) validate() error {
	switch {
	case op.Class == "":
		return fmt.Errorf("%w: operation class is unset", ErrAuditOperationRequired)
	case op.RequestDescriptor == "":
		return fmt.Errorf("%w: request descriptor is unset", ErrAuditOperationRequired)
	case op.ActorType == "":
		return fmt.Errorf("%w: actor type is unset", ErrAuditOperationRequired)
	case op.Origin == "":
		return fmt.Errorf("%w: origin is unset", ErrAuditOperationRequired)
	case op.AuthorizationOutcome == "":
		return fmt.Errorf("%w: authorization outcome is unset", ErrAuditOperationRequired)
	case op.Result == "":
		return fmt.Errorf("%w: result is unset", ErrAuditOperationRequired)
	}
	return nil
}

func (e AuditEffect) validate() error {
	for _, ref := range []*string{e.BeforeRef, e.AfterRef} {
		if ref == nil {
			continue
		}
		if _, err := ulid.ParseStrict(*ref); err != nil {
			return fmt.Errorf("%w: reference must be a ULID", ErrAuditEffectInvalid)
		}
	}
	return nil
}

func (s *Store) auditNow() time.Time { return s.clock().UTC().Truncate(time.Microsecond) }

func nilIfEmpty(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func (op AuditOperation) insertParams(seq int64, at time.Time) generated.InsertAuditOperationParams {
	return generated.InsertAuditOperationParams{
		OperationID: op.OperationID, Stream: op.Stream, ChainSeq: seq,
		OperationClass: string(op.Class), ActorType: op.ActorType, ActorID: op.ActorID,
		ActorFingerprint: op.ActorFingerprint, Origin: op.Origin, OriginFingerprint: op.OriginFingerprint,
		RequestDescriptor: op.RequestDescriptor, AuthorizationOutcome: string(op.AuthorizationOutcome),
		AuthorizationDetail: op.AuthorizationDetail, Result: string(op.Result), ResultCode: op.ResultCode,
		OccurredAt: at, SealedDetail: op.SealedDetail, SealedDetailSubject: nilIfEmpty(op.SealedDetailSubject),
	}
}

func (e AuditEffect) insertParams(stream, operationID, effectID string, seq, effectSeq int64, at time.Time) generated.InsertAuditEffectParams {
	changed := e.ChangedFields
	if changed == nil {
		changed = []string{}
	}
	return generated.InsertAuditEffectParams{
		EffectID: effectID, OperationID: operationID, Stream: stream, ChainSeq: seq, EffectSeq: effectSeq,
		ResourceType: e.ResourceType, ResourceID: e.ResourceID, Action: e.Action, Outcome: string(e.Outcome),
		ChangedFields: sqlitetype.StringList(changed), BeforeRef: e.BeforeRef, AfterRef: e.AfterRef,
		BeforeFlag: e.BeforeFlag, AfterFlag: e.AfterFlag, BeforeCount: e.BeforeCount, AfterCount: e.AfterCount,
		EvidenceKind: e.EvidenceKind, EvidenceFingerprint: e.EvidenceFingerprint,
		SealedDetail: e.SealedDetail, SealedDetailSubject: nilIfEmpty(e.SealedDetailSubject), OccurredAt: at,
	}
}

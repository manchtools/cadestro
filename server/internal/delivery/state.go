// Package delivery owns durable one-shot manifests and their terminal results.
package delivery

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"buf.build/go/protovalidate"
	"github.com/oklog/ulid/v2"
	"google.golang.org/protobuf/encoding/protojson"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

const (
	StateSucceeded = "SUCCEEDED"
	StatePartial   = "PARTIAL"
	StateFailed    = "FAILED"
)

var (
	ErrInvalidInput      = errors.New("invalid delivery input")
	ErrWrongDevice       = errors.New("delivery belongs to another device")
	ErrWrongManifest     = errors.New("delivery carries another manifest")
	ErrInvalidTransition = errors.New("invalid delivery transition")

	resultCodePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
)

// InsertParams is the complete durable input for one device delivery.
type InsertParams struct {
	OperationID string
	DeviceID    string
	Manifest    *cadestrov1.Manifest
	AvailableAt time.Time
}

// InsertInTx commits a complete manifest through the initiating operation's
// audited transaction. The device retrieves it through Sync.
func InsertInTx(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder, p InsertParams) (string, error) {
	if ctx == nil || tx == nil || rec == nil || p.Manifest == nil || p.AvailableAt.IsZero() {
		return "", ErrInvalidInput
	}
	if !validID(p.OperationID) || !validID(p.DeviceID) || !validManifest(p.Manifest) {
		return "", ErrInvalidInput
	}
	payload, err := protojson.Marshal(p.Manifest)
	if err != nil {
		return "", fmt.Errorf("marshal delivery manifest: %w", err)
	}
	deliveryID := ulid.Make().String()
	operationID := p.OperationID
	if _, err := tx.InsertDelivery(ctx, db.InsertDeliveryParams{
		DeliveryID: deliveryID, DeviceID: p.DeviceID, ManifestID: p.Manifest.ManifestId,
		Manifest: payload, OperationID: &operationID, AvailableAt: p.AvailableAt,
	}); err != nil {
		return "", fmt.Errorf("insert delivery: %w", err)
	}
	manifestID := p.Manifest.ManifestId
	rec.Effect(store.AuditEffect{
		ResourceType: "delivery", ResourceID: deliveryID, Action: "CREATE",
		Outcome: store.EffectApplied, ChangedFields: []string{"manifest", "state"}, AfterRef: &manifestID,
	})
	return deliveryID, nil
}

func validManifest(manifest *cadestrov1.Manifest) bool {
	if protovalidate.Validate(manifest) != nil {
		return false
	}
	p := manifest.GetProvenance()
	if p == nil {
		return false
	}
	validPath := (p.DefinitionId != "" && p.ActionSetId == "" && p.ActionId == "") ||
		(p.DefinitionId != "" && p.ActionSetId != "" && p.ActionId == "") ||
		(p.DefinitionId == "" && p.ActionSetId != "" && p.ActionId == "") ||
		(p.DefinitionId == "" && p.ActionSetId == "" && p.ActionId != "")
	if !validPath {
		return false
	}
	seen := make(map[string]struct{}, len(manifest.Occurrences))
	for _, occurrence := range manifest.Occurrences {
		if occurrence == nil {
			return false
		}
		if _, duplicate := seen[occurrence.OccurrenceId]; duplicate {
			return false
		}
		seen[occurrence.OccurrenceId] = struct{}{}
	}
	return true
}

func validID(id string) bool {
	_, err := ulid.ParseStrict(id)
	return err == nil
}

// Config supplies the durable store and clock used by delivery transitions.
type Config struct {
	Store *store.Store
	Now   func() time.Time
}

// Service advances delivery rows in audited transactions.
type Service struct {
	store *store.Store
	now   func() time.Time
}

// New constructs a delivery service. A missing store is a boot-time wiring
// defect and is rejected immediately.
func New(cfg Config) *Service {
	if cfg.Store == nil {
		panic("delivery: store is required")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Service{store: cfg.Store, now: cfg.Now}
}

// Complete records one manifest's aggregate terminal result. A replay must
// agree with the already committed state and fixed result code.
func (s *Service) Complete(ctx context.Context, deliveryID, deviceID, manifestID, state, resultCode string) (bool, error) {
	if ctx == nil || !validID(deliveryID) || !validID(deviceID) || !validID(manifestID) || !resultCodePattern.MatchString(resultCode) {
		return false, ErrInvalidInput
	}
	if state != StateSucceeded && state != StatePartial && state != StateFailed {
		return false, ErrInvalidInput
	}
	row, err := s.store.GetDelivery(ctx, deliveryID)
	if err != nil {
		if store.IsNotFound(err) {
			return true, s.store.RecordPolicyManifestResult(ctx, deviceID, deliveryID, manifestID, state, resultCode)
		}
		return false, err
	}
	if err := resultAllowed(row, deviceID, manifestID, state, resultCode); err != nil {
		return false, err
	}
	if terminal(row.State) {
		return false, nil
	}

	now := s.now().UTC()
	_, err = s.store.WithAudit(ctx, agentOperation(deviceID, "delivery.result"), func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		n, err := tx.MarkDeliveryResult(ctx, db.MarkDeliveryResultParams{
			DeliveryID: deliveryID, State: state, TerminalAt: &now, ResultCode: resultCode,
		})
		if err != nil {
			return fmt.Errorf("complete delivery: %w", err)
		}
		if n != 1 {
			return store.ErrConflict
		}
		rec.Effect(deliveryEffect(deliveryID, "RESULT", "state", "result_code", "terminal_at"))
		return nil
	})
	if err == nil {
		return true, nil
	}
	if !store.IsConflict(err) {
		return false, err
	}
	current, readErr := s.store.GetDelivery(ctx, deliveryID)
	if readErr != nil {
		return false, readErr
	}
	if allowedErr := resultAllowed(current, deviceID, manifestID, state, resultCode); allowedErr != nil {
		return false, allowedErr
	}
	if terminal(current.State) {
		return false, nil
	}
	return false, store.ErrConflict
}

func resultAllowed(row store.DeliveryRow, deviceID, manifestID, state, resultCode string) error {
	if row.DeviceID != deviceID {
		return ErrWrongDevice
	}
	if row.ManifestID != manifestID {
		return ErrWrongManifest
	}
	if terminal(row.State) {
		if row.State == state && row.ResultCode == resultCode {
			return nil
		}
		return ErrInvalidTransition
	}
	return nil
}

func terminal(state string) bool {
	switch state {
	case StateSucceeded, StatePartial, StateFailed:
		return true
	default:
		return false
	}
}

func deliveryEffect(deliveryID, action string, fields ...string) store.AuditEffect {
	return store.AuditEffect{
		ResourceType: "delivery", ResourceID: deliveryID, Action: action,
		Outcome: store.EffectApplied, ChangedFields: fields,
	}
}

func agentOperation(deviceID, descriptor string) store.AuditOperation {
	return store.AuditOperation{
		Class: store.ClassMutation, ActorType: "agent", ActorID: deviceID, Origin: "agent_stream",
		RequestDescriptor: descriptor, AuthorizationOutcome: store.AuthorizationAllowed,
		AuthorizationDetail: "device_mtls", Result: store.ResultSuccess, ResultCode: "OK",
	}
}

package agentsecrets

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"buf.build/go/protovalidate"
	"github.com/oklog/ulid/v2"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/crypto"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

var (
	ErrInvalidInput      = errors.New("invalid agent secret input")
	ErrWrongActionType   = errors.New("action has the wrong secret type")
	ErrDuplicateUsername = errors.New("LPS batch repeats a username")
)

type Config struct {
	Store  *store.Store
	AtRest *crypto.Encryptor
	Now    func() time.Time
}

type Service struct {
	store     *store.Store
	atRest    *crypto.Encryptor
	now       func() time.Time
	validator protovalidate.Validator
}

func New(cfg Config) *Service {
	if cfg.Store == nil || cfg.AtRest == nil {
		panic("agentsecrets: store and at-rest cipher are required")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Service{
		store: cfg.Store, atRest: cfg.AtRest,
		now: cfg.Now, validator: protovalidate.GlobalValidator,
	}
}

func (s *Service) ValidateLuksToken(ctx context.Context, deviceID string, request *cadestrov1.ValidateLuksTokenRequest) (*cadestrov1.ValidateLuksTokenResponse, error) {
	if ctx == nil || !validID(deviceID) || request == nil || s.validator.Validate(request) != nil {
		return nil, ErrInvalidInput
	}
	hash := sha256.Sum256([]byte(request.Token))
	now := s.now().UTC().Truncate(time.Microsecond)
	var token db.LuksToken
	_, err := s.store.WithAudit(ctx, agentOperation(deviceID, "ValidateLuksToken"),
		func(ctx context.Context, tx *store.Tx, recorder *store.AuditRecorder) error {
			var err error
			token, err = tx.ConsumeLuksToken(ctx, db.ConsumeLuksTokenParams{
				Token: hex.EncodeToString(hash[:]), DeviceID: deviceID, Now: now,
			})
			if store.IsNotFound(err) {
				return store.ErrNotFound
			}
			if err != nil {
				return fmt.Errorf("consume LUKS token: %w", err)
			}
			recorder.Effect(store.AuditEffect{
				ResourceType: "luks_token", ResourceID: token.ID, Action: "CONSUME",
				Outcome: store.EffectApplied, ChangedFields: []string{"used"},
			})
			return nil
		})
	if err != nil {
		return nil, err
	}
	devicePath := ""
	key, err := s.store.GetCurrentLuksKeyForAgent(ctx, deviceID, token.ActionID)
	if err == nil {
		devicePath = key.DevicePath
	} else if !store.IsNotFound(err) {
		return nil, err
	}
	return &cadestrov1.ValidateLuksTokenResponse{
		ActionId: &cadestrov1.ActionId{Value: token.ActionID}, DevicePath: devicePath, MinLength: token.MinLength,
		Complexity: cadestrov1.LpsPasswordComplexity(token.Complexity),
	}, nil
}

func (s *Service) GetLuksKey(ctx context.Context, deviceID string, request *cadestrov1.GetLuksKeyRequest) (*cadestrov1.GetLuksKeyResponse, error) {
	if ctx == nil || !validID(deviceID) || request == nil || s.validator.Validate(request) != nil {
		return nil, ErrInvalidInput
	}
	actionID := request.GetActionId().GetValue()
	key, err := s.store.GetCurrentLuksKeyForAgent(ctx, deviceID, actionID)
	if err != nil {
		return nil, err
	}
	stored, err := s.store.GetDeviceSecret(ctx, key.ID)
	if err != nil || stored.DeviceID != deviceID || stored.Kind != "luks" {
		if err == nil {
			err = errors.New("generic LUKS secret owner mismatch")
		}
		return nil, err
	}
	plaintext, err := s.atRest.DecryptWithContext(stored.Ciphertext,
		crypto.DeviceSecretAAD(stored.ID, stored.DeviceID, stored.Kind, stored.Subject, uint32(stored.Version)))
	if err != nil {
		return nil, fmt.Errorf("open LUKS passphrase at rest: %w", err)
	}
	if _, err := s.store.RecordOperation(ctx, agentSensitiveReadOperation(deviceID, "GetLuksKey"), store.AuditEffect{
		ResourceType: "luks_key", ResourceID: key.ID, Action: "READ", Outcome: store.EffectApplied,
	}); err != nil {
		return nil, err
	}
	return &cadestrov1.GetLuksKeyResponse{Passphrase: []byte(plaintext)}, nil
}

func (s *Service) StoreLuksKey(ctx context.Context, deviceID string, request *cadestrov1.StoreLuksKeyRequest) (*cadestrov1.StoreLuksKeyResponse, error) {
	if ctx == nil || !validID(deviceID) || request == nil || s.validator.Validate(request) != nil {
		return nil, ErrInvalidInput
	}
	actionID := request.GetActionId().GetValue()
	if err := s.requireActionType(ctx, actionID, cadestrov1.ActionType_ACTION_TYPE_ENCRYPTION); err != nil {
		return nil, err
	}
	plaintext, err := copySecret(request.Passphrase)
	if err != nil {
		return nil, err
	}
	defer clear(plaintext)

	rowID := ulid.Make().String()
	genericCiphertext, err := s.atRest.EncryptWithContext(string(plaintext),
		crypto.DeviceSecretAAD(rowID, deviceID, "luks", actionID, 1))
	if err != nil {
		return nil, fmt.Errorf("encrypt generic LUKS passphrase at rest: %w", err)
	}
	reason, ok := rotationReason(request.RotationReason)
	if !ok || request.RotationReason == cadestrov1.RotationReason_ROTATION_REASON_AUTH_GRACE {
		return nil, ErrInvalidInput
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	_, err = s.store.WithAudit(ctx, agentOperation(deviceID, "StoreLuksKey"),
		func(ctx context.Context, tx *store.Tx, recorder *store.AuditRecorder) error {
			if _, err := tx.RetireCurrentLuksKeys(ctx, db.RetireCurrentLuksKeysParams{
				DeviceID: deviceID, ActionID: actionID,
			}); err != nil {
				return fmt.Errorf("retire current LUKS key: %w", err)
			}
			if err := tx.InsertDeviceSecret(ctx, db.InsertDeviceSecretParams{ID: rowID, DeviceID: deviceID, Kind: "luks", Subject: actionID, Version: 1, Ciphertext: genericCiphertext}); err != nil {
				return fmt.Errorf("insert device secret: %w", err)
			}
			if _, err := tx.InsertLuksKey(ctx, db.InsertLuksKeyParams{
				ID: rowID, DevicePath: request.DevicePath,
				RotatedAt: now, RotationReason: reason, CreatedAt: now,
			}); err != nil {
				return fmt.Errorf("insert LUKS key: %w", err)
			}
			recorder.Effect(secretEffect("luks_key", rowID, "ROTATE",
				"device_path", "is_current", "passphrase", "rotated_at", "rotation_reason"))
			return nil
		})
	if err != nil {
		return nil, err
	}
	return &cadestrov1.StoreLuksKeyResponse{Success: true}, nil
}

func (s *Service) StoreLpsPasswords(ctx context.Context, deviceID string, request *cadestrov1.StoreLpsPasswordsRequest) (*cadestrov1.StoreLpsPasswordsResponse, error) {
	if ctx == nil || !validID(deviceID) || request == nil || s.validator.Validate(request) != nil {
		return nil, ErrInvalidInput
	}
	actionID := request.GetActionId().GetValue()
	if err := s.requireActionType(ctx, actionID, cadestrov1.ActionType_ACTION_TYPE_LPS); err != nil {
		return nil, err
	}
	type preparedRotation struct {
		id, username, genericCiphertext, reason string
		rotatedAt                               time.Time
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	prepared := make([]preparedRotation, 0, len(request.Rotations))
	seen := make(map[string]struct{}, len(request.Rotations))
	for _, rotation := range request.Rotations {
		if rotation == nil {
			return nil, ErrInvalidInput
		}
		if _, duplicate := seen[rotation.Username]; duplicate {
			return nil, ErrDuplicateUsername
		}
		seen[rotation.Username] = struct{}{}
		plaintext, err := copySecret(rotation.Password)
		if err != nil {
			return nil, err
		}

		rowID := ulid.Make().String()
		genericCiphertext, genericEncryptErr := s.atRest.EncryptWithContext(string(plaintext),
			crypto.DeviceSecretAAD(rowID, deviceID, "lps", actionID, 1))
		clear(plaintext)
		if genericEncryptErr != nil {
			return nil, fmt.Errorf("encrypt generic LPS password at rest: %w", genericEncryptErr)
		}
		rotatedAt, err := time.Parse(time.RFC3339Nano, rotation.RotatedAt)
		if err != nil {
			rotatedAt = now
		}
		reason, ok := rotationReason(rotation.Reason)
		if !ok {
			return nil, ErrInvalidInput
		}
		prepared = append(prepared, preparedRotation{
			id: rowID, username: rotation.Username, genericCiphertext: genericCiphertext,
			rotatedAt: rotatedAt.UTC().Truncate(time.Microsecond), reason: reason,
		})
	}
	_, err := s.store.WithAudit(ctx, agentOperation(deviceID, "StoreLpsPasswords"),
		func(ctx context.Context, tx *store.Tx, recorder *store.AuditRecorder) error {
			for _, rotation := range prepared {
				if _, err := tx.RetireCurrentLpsPassword(ctx, db.RetireCurrentLpsPasswordParams{
					DeviceID: deviceID, ActionID: actionID, Username: rotation.username,
				}); err != nil {
					return fmt.Errorf("retire current LPS password: %w", err)
				}
				if err := tx.InsertDeviceSecret(ctx, db.InsertDeviceSecretParams{ID: rotation.id, DeviceID: deviceID, Kind: "lps", Subject: actionID, Version: 1, Ciphertext: rotation.genericCiphertext}); err != nil {
					return fmt.Errorf("insert device secret: %w", err)
				}
				if _, err := tx.InsertLpsPassword(ctx, db.InsertLpsPasswordParams{
					ID: rotation.id, Username: rotation.username, RotatedAt: rotation.rotatedAt,
					RotationReason: rotation.reason, CreatedAt: now,
				}); err != nil {
					return fmt.Errorf("insert LPS password: %w", err)
				}
				recorder.Effect(secretEffect("lps_password", rotation.id, "ROTATE",
					"is_current", "password", "rotated_at", "rotation_reason", "username"))
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	return &cadestrov1.StoreLpsPasswordsResponse{Success: true}, nil
}

func copySecret(value []byte) ([]byte, error) {
	if len(value) == 0 {
		return nil, ErrInvalidInput
	}

	return append([]byte(nil), value...), nil
}

func (s *Service) requireActionType(ctx context.Context, actionID string, expected cadestrov1.ActionType) error {
	action, err := s.store.GetManifestAction(ctx, actionID)
	if err != nil {
		return err
	}
	if cadestrov1.ActionType(action.ActionType) != expected {
		return ErrWrongActionType
	}
	return nil
}

func rotationReason(reason cadestrov1.RotationReason) (string, bool) {
	switch reason {
	case cadestrov1.RotationReason_ROTATION_REASON_INITIAL:
		return "initial", true
	case cadestrov1.RotationReason_ROTATION_REASON_SCHEDULED:
		return "scheduled", true
	case cadestrov1.RotationReason_ROTATION_REASON_AUTH_GRACE:
		return "auth_grace", true
	default:
		return "", false
	}
}

func validID(id string) bool {
	_, err := ulid.ParseStrict(id)
	return err == nil
}

func agentOperation(deviceID, descriptor string) store.AuditOperation {
	return store.AuditOperation{
		Class: store.ClassMutation, ActorType: "agent", ActorID: deviceID, Origin: "agent_stream",
		RequestDescriptor:    "cadestro.v1.AgentService.Stream/" + descriptor,
		AuthorizationOutcome: store.AuthorizationAllowed, AuthorizationDetail: "device_mtls",
		Result: store.ResultSuccess, ResultCode: "OK",
	}
}

func agentSensitiveReadOperation(deviceID, descriptor string) store.AuditOperation {
	operation := agentOperation(deviceID, descriptor)
	operation.Class = store.ClassSensitiveRead
	return operation
}

func secretEffect(resourceType, id, action string, fields ...string) store.AuditEffect {
	return store.AuditEffect{
		ResourceType: resourceType, ResourceID: id, Action: action,
		Outcome: store.EffectApplied, ChangedFields: fields,
	}
}

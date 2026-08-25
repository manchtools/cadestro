package authoring

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"buf.build/go/protovalidate"
	"github.com/oklog/ulid/v2"

	contract "github.com/manchtools/cadestro/contract"
	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/actionparams"
	"github.com/manchtools/cadestro/server/internal/crypto"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
	"github.com/manchtools/cadestro/server/internal/store/sqlitetype"
	"google.golang.org/protobuf/proto"
)

const (
	defaultActionTimeout = int32(300)
	maxParamsBytes       = 2 << 20
)

var (
	ErrInvalidInput = errors.New("invalid authoring input")
	ErrSystemAction = errors.New("system action cannot be changed by an operator")
)

type Config struct {
	Store *store.Store
	Now   func() time.Time
}

type Service struct {
	store *store.Store
	now   func() time.Time
}

func New(cfg Config) *Service {
	if cfg.Store == nil {
		panic("authoring: store is required")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Service{store: cfg.Store, now: cfg.Now}
}

type CreateActionParams struct {
	ID             string
	Name           string
	Description    string
	CreatedBy      string
	Type           cadestrov1.ActionType
	DesiredState   cadestrov1.DesiredState
	Params         []byte
	TimeoutSeconds int32
	Schedule       *cadestrov1.ActionSchedule
	System         bool
}

type UpdateActionParams struct {
	ID             string
	DesiredState   cadestrov1.DesiredState
	Params         []byte
	TimeoutSeconds int32
	Schedule       *cadestrov1.ActionSchedule
	AllowSystem    bool
}

func (s *Service) CreateAction(ctx context.Context, op store.AuditOperation, p CreateActionParams) (store.ActionRow, error) {
	if ctx == nil || !validID(p.CreatedBy) || (op.ActorID != "" && op.ActorID != p.CreatedBy) ||
		p.Name == "" || utf8.RuneCountInString(p.Name) > 255 || utf8.RuneCountInString(p.Description) > 1024 {
		return store.ActionRow{}, ErrInvalidInput
	}
	timeout := p.TimeoutSeconds
	if timeout == 0 {
		timeout = defaultActionTimeout
	}
	id := p.ID
	if id == "" {
		id = ulid.Make().String()
	} else if !validID(id) {
		return store.ActionRow{}, ErrInvalidInput
	}
	params, err := validateActionData(id, p.Type, p.DesiredState, timeout, p.Schedule, p.Params)
	if err != nil {
		return store.ActionRow{}, err
	}
	schedule, err := actionparams.ScheduleToRaw(p.Schedule)
	if err != nil {
		return store.ActionRow{}, fmt.Errorf("authoring: encode action schedule: %w", err)
	}

	now := s.now().UTC()
	var out store.ActionRow
	_, err = s.store.WithAudit(ctx, op, func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		row, err := tx.InsertAuthoringAction(ctx, db.InsertAuthoringActionParams{
			ID: id, Name: p.Name, Description: p.Description,
			ActionType: int32(p.Type), DesiredState: int32(p.DesiredState),
			Params: params, ParamsCanonical: params, TimeoutSeconds: timeout,
			Schedule: sqlitetype.JSON(schedule), IsSystem: p.System, CreatedAt: &now, CreatedBy: p.CreatedBy,
		})
		if err != nil {
			return fmt.Errorf("authoring: insert action: %w", err)
		}
		out = row
		rec.Effect(actionEffect(id, "CREATE",
			"name", "description", "action_type", "desired_state", "params", "timeout_seconds", "schedule"))
		return nil
	})
	if err != nil {
		return store.ActionRow{}, err
	}
	return out, nil
}

func (s *Service) RenameAction(ctx context.Context, op store.AuditOperation, id, name string, allowSystem bool) (store.ActionRow, error) {
	if ctx == nil || !validID(id) || name == "" || utf8.RuneCountInString(name) > 255 {
		return store.ActionRow{}, ErrInvalidInput
	}
	now := s.now().UTC()
	var out store.ActionRow
	_, err := s.store.WithAudit(ctx, op, func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		row, err := tx.RenameAuthoringAction(ctx, db.RenameAuthoringActionParams{
			ID: id, Name: name, UpdatedAt: &now, AllowSystem: allowSystem,
		})
		if err != nil {
			return err
		}
		out = row
		rec.Effect(actionEffect(id, "UPDATE", "name"))
		return refreshActionDependents(ctx, tx, rec, id)
	})
	return out, s.classifyWriteError(ctx, id, allowSystem, err)
}

func (s *Service) UpdateActionDescription(ctx context.Context, op store.AuditOperation, id, description string, allowSystem bool) (store.ActionRow, error) {
	if ctx == nil || !validID(id) || utf8.RuneCountInString(description) > 1024 {
		return store.ActionRow{}, ErrInvalidInput
	}
	now := s.now().UTC()
	var out store.ActionRow
	_, err := s.store.WithAudit(ctx, op, func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		row, err := tx.UpdateAuthoringActionDescription(ctx, db.UpdateAuthoringActionDescriptionParams{
			ID: id, Description: description, UpdatedAt: &now, AllowSystem: allowSystem,
		})
		if err != nil {
			return err
		}
		out = row
		rec.Effect(actionEffect(id, "UPDATE", "description"))
		return refreshActionDependents(ctx, tx, rec, id)
	})
	return out, s.classifyWriteError(ctx, id, allowSystem, err)
}

func (s *Service) UpdateActionParams(ctx context.Context, op store.AuditOperation, p UpdateActionParams) (store.ActionRow, error) {
	if ctx == nil || !validID(p.ID) {
		return store.ActionRow{}, ErrInvalidInput
	}
	existing, err := s.store.GetManifestAction(ctx, p.ID)
	if err != nil {
		return store.ActionRow{}, err
	}
	if existing.IsSystem && !p.AllowSystem {
		return store.ActionRow{}, ErrSystemAction
	}
	timeout := p.TimeoutSeconds
	if timeout == 0 {
		timeout = existing.TimeoutSeconds
	}
	scheduleForValidation := p.Schedule
	if scheduleForValidation == nil {
		scheduleForValidation, err = actionparams.ParseSchedule(existing.Schedule)
		if err != nil {
			return store.ActionRow{}, fmt.Errorf("authoring: stored action schedule: %w", err)
		}
	}
	params, err := validateActionData(p.ID, cadestrov1.ActionType(existing.ActionType), p.DesiredState, timeout, scheduleForValidation, p.Params)
	if err != nil {
		return store.ActionRow{}, err
	}
	var schedule []byte
	if p.Schedule != nil {
		schedule, err = actionparams.ScheduleToRaw(p.Schedule)
		if err != nil {
			return store.ActionRow{}, fmt.Errorf("authoring: encode action schedule: %w", err)
		}
	}

	now := s.now().UTC()
	var out store.ActionRow
	_, err = s.store.WithAudit(ctx, op, func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		row, err := tx.UpdateAuthoringActionParams(ctx, db.UpdateAuthoringActionParamsParams{
			ID: p.ID, DesiredState: int32(p.DesiredState), Params: params, ParamsCanonical: params,
			HasTimeout: boolInt32(p.TimeoutSeconds > 0), TimeoutSeconds: p.TimeoutSeconds,
			HasSchedule: p.Schedule != nil, Schedule: sqlitetype.JSON(schedule),
			UpdatedAt: &now, AllowSystem: p.AllowSystem,
		})
		if err != nil {
			return err
		}
		out = row
		changed := []string{"desired_state", "params"}
		if p.TimeoutSeconds > 0 {
			changed = append(changed, "timeout_seconds")
		}
		if p.Schedule != nil {
			changed = append(changed, "schedule")
		}
		rec.Effect(actionEffect(p.ID, "UPDATE", changed...))
		return nil
	})
	return out, s.classifyWriteError(ctx, p.ID, p.AllowSystem, err)
}

func boolInt32(value bool) int32 {
	if value {
		return 1
	}
	return 0
}

func (s *Service) DeleteAction(ctx context.Context, op store.AuditOperation, id string, allowSystem bool) error {
	if ctx == nil || !validID(id) {
		return ErrInvalidInput
	}
	now := s.now().UTC()
	_, err := s.store.WithAudit(ctx, op, func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		if err := refreshActionDependents(ctx, tx, rec, id); err != nil {
			return err
		}
		if _, err := tx.DeleteActionMemberships(ctx, id); err != nil {
			return fmt.Errorf("authoring: delete action memberships: %w", err)
		}
		if _, err := tx.DeleteCompliancePolicyRulesForAction(ctx, id); err != nil {
			return fmt.Errorf("authoring: delete compliance policy rules: %w", err)
		}
		evaluationDevices, err := tx.DeleteCompliancePolicyEvaluationsForAction(ctx, id)
		if err != nil {
			return fmt.Errorf("authoring: delete compliance policy evaluations: %w", err)
		}
		resultDevices, err := tx.DeleteComplianceResultsForAction(ctx, id)
		if err != nil {
			return fmt.Errorf("authoring: delete compliance results: %w", err)
		}
		if err := store.RefreshDeviceCompliance(ctx, tx, rec, evaluationDevices, resultDevices); err != nil {
			return err
		}
		if _, err := tx.SoftDeleteAuthoringAction(ctx, db.SoftDeleteAuthoringActionParams{
			ID: id, UpdatedAt: &now, AllowSystem: allowSystem,
		}); err != nil {
			return err
		}
		rec.Effect(actionEffect(id, "DELETE", "is_deleted", "memberships"))
		return nil
	})
	return s.classifyWriteError(ctx, id, allowSystem, err)
}

func validateActionData(id string, actionType cadestrov1.ActionType, desired cadestrov1.DesiredState, timeout int32, schedule *cadestrov1.ActionSchedule, raw []byte) ([]byte, error) {
	if _, ok := cadestrov1.ActionType_name[int32(actionType)]; !ok || actionType == cadestrov1.ActionType_ACTION_TYPE_UNSPECIFIED {
		return nil, ErrInvalidInput
	}
	if _, ok := cadestrov1.DesiredState_name[int32(desired)]; !ok || timeout < 0 || timeout > 3600 {
		return nil, ErrInvalidInput
	}
	canonical, err := canonicalJSONObject(raw)
	if err != nil {
		return nil, err
	}
	actionID := id
	if !validID(actionID) {
		actionID = ulid.Make().String()
	}
	request := &cadestrov1.UpdateActionParamsRequest{
		Id: &cadestrov1.ActionId{Value: actionID}, DesiredState: desired, TimeoutSeconds: timeout, Schedule: schedule,
	}
	if err := actionparams.PopulateUpdateActionParams(request, actionType, canonical); err != nil {
		return nil, fmt.Errorf("authoring: validate action params: %w", err)
	}
	params := actionparams.ExtractParamsMsg(request)
	if params == nil && !bytes.Equal(canonical, []byte("{}")) {
		return nil, ErrInvalidInput
	}
	if err := normalizeStoredSecretsForValidation(params); err != nil {
		return nil, err
	}
	if err := protovalidate.Validate(request); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidInput, err)
	}
	if err := validateActionSafety(params); err != nil {
		return nil, err
	}
	return canonical, nil
}

func normalizeStoredSecretsForValidation(params proto.Message) error {
	switch value := params.(type) {
	case *cadestrov1.EncryptionAuthoringParams:
		if value.PresharedKey == nil || !crypto.IsEncryptedValue(value.GetPresharedKey()) {
			return ErrInvalidInput
		}
		value.PresharedKey = stringPointer("configured")
	case *cadestrov1.WifiAuthoringParams:
		switch value.AuthType {
		case cadestrov1.WifiAuthType_WIFI_AUTH_TYPE_PSK:
			if value.Psk == nil || !crypto.IsEncryptedValue(value.GetPsk()) || value.ClientKey != nil {
				return ErrInvalidInput
			}
			value.Psk = stringPointer("configured")
		case cadestrov1.WifiAuthType_WIFI_AUTH_TYPE_EAP_TLS:
			if value.ClientKey == nil || !crypto.IsEncryptedValue(value.GetClientKey()) || value.Psk != nil {
				return ErrInvalidInput
			}
			value.ClientKey = stringPointer("configured")
		default:
			return ErrInvalidInput
		}
	}
	return nil
}

func ValidateExecutableAction(action *cadestrov1.Action) error {
	if action == nil || !validID(action.GetId().GetValue()) {
		return ErrInvalidInput
	}
	params := actionparams.ExtractParamsMsg(action)
	if params == nil {

		if action.Type != cadestrov1.ActionType_ACTION_TYPE_UPDATE {
			return ErrInvalidInput
		}
	} else if !actionparams.ParamsMatchType(action, action.Type) {
		return ErrInvalidInput
	}
	raw := []byte("{}")
	if params != nil {
		var err error
		raw, err = actionparams.MarshalActionParams(params)
		if err != nil {
			return ErrInvalidInput
		}
	}
	_, err := validateActionData(action.Id.Value, action.Type, action.DesiredState,
		action.TimeoutSeconds, action.Schedule, raw)
	return err
}

func validateActionSafety(params proto.Message) error {
	switch p := params.(type) {
	case *cadestrov1.ShellParams:
		if p.Script == "" && p.DetectionScript == "" {
			return fmt.Errorf("%w: shell action needs a script or detection script", ErrInvalidInput)
		}
		if p.IsCompliance && strings.TrimSpace(p.DetectionScript) == "" {
			return fmt.Errorf("%w: compliance shell action is detection-only and needs a detection script", ErrInvalidInput)
		}
	case *cadestrov1.AppInstallParams:
		if !strings.HasPrefix(strings.ToLower(p.Url), "https://") || !isLowerHex64(p.ChecksumSha256) {
			return fmt.Errorf("%w: application install requires HTTPS and a lowercase SHA-256", ErrInvalidInput)
		}
	case *cadestrov1.AgentUpdateParams:
		if p.Amd64 == nil && p.Arm64 == nil {
			return fmt.Errorf("%w: agent update needs at least one architecture", ErrInvalidInput)
		}
		for _, arch := range []*cadestrov1.AgentUpdateArch{p.Amd64, p.Arm64} {
			if arch == nil {
				continue
			}
			if contract.ValidateHTTPSURL(arch.BinaryUrl) != nil || contract.ValidateHTTPSURL(arch.ChecksumUrl) != nil {
				return fmt.Errorf("%w: unsafe agent update source", ErrInvalidInput)
			}
		}
	}
	return nil
}

func isLowerHex64(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, c := range value {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func canonicalJSONObject(raw []byte) ([]byte, error) {
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	if len(raw) > maxParamsBytes {
		return nil, ErrInvalidInput
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var value map[string]json.RawMessage
	if err := decoder.Decode(&value); err != nil || value == nil {
		return nil, ErrInvalidInput
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, ErrInvalidInput
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("authoring: canonicalize params: %w", err)
	}
	return canonical, nil
}

func (s *Service) classifyWriteError(ctx context.Context, id string, allowSystem bool, err error) error {
	if err == nil || !store.IsNotFound(err) {
		return err
	}
	row, readErr := s.store.GetManifestAction(ctx, id)
	if readErr == nil && row.IsSystem && !allowSystem {
		return ErrSystemAction
	}
	return store.ErrNotFound
}

func actionEffect(id, action string, fields ...string) store.AuditEffect {
	return store.AuditEffect{
		ResourceType: "action", ResourceID: id, Action: action,
		Outcome: store.EffectApplied, ChangedFields: fields,
	}
}

func validID(id string) bool {
	_, err := ulid.ParseStrict(id)
	return err == nil
}

// Package manifest compiles the authoring hierarchy into the flat, durable
// unit of work sent to an agent.
package manifest

import (
	"context"
	"errors"
	"fmt"

	"buf.build/go/protovalidate"
	"github.com/oklog/ulid/v2"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/actionparams"
	"github.com/manchtools/cadestro/server/internal/crypto"
	"github.com/manchtools/cadestro/server/internal/store"
	"google.golang.org/protobuf/proto"
)

var (
	// ErrInvalidInput means the requested source identifier is not a ULID or
	// the caller supplied no context.
	ErrInvalidInput = errors.New("invalid manifest compiler input")
	// ErrEmptyManifest means an ActionSet or Definition member contains no
	// live actions and therefore cannot become executable work.
	ErrEmptyManifest = errors.New("manifest contains no actions")
)

// Compiler turns an Action, ActionSet or Definition into complete manifests.
type Compiler struct {
	store *store.Store
}

// New constructs a compiler. A missing store is a boot-time wiring error.
func New(st *store.Store) *Compiler {
	if st == nil {
		panic("manifest: store is required")
	}
	return &Compiler{store: st}
}

// Action creates the singleton manifest for one authored Action.
func (c *Compiler) Action(ctx context.Context, id string) (*cadestrov1.Manifest, error) {
	if !validInput(ctx, id) {
		return nil, ErrInvalidInput
	}
	row, err := c.store.GetManifestAction(ctx, id)
	if err != nil {
		return nil, err
	}
	action, err := c.compileAction(row)
	if err != nil {
		return nil, err
	}
	schedule := action.Schedule
	if schedule == nil {
		schedule = &cadestrov1.ActionSchedule{}
	}
	return finish(&cadestrov1.Manifest{
		ManifestId:       ulid.Make().String(),
		Provenance:       &cadestrov1.ManifestProvenance{ActionId: id},
		Schedule:         schedule,
		DefaultOnFailure: cadestrov1.OnFailure_ON_FAILURE_CONTINUE,
		Occurrences:      []*cadestrov1.ManifestOccurrence{occurrence(action, cadestrov1.OnFailure_ON_FAILURE_CONTINUE)},
	})
}

// ActionSet flattens one set into a manifest in authored member order.
func (c *Compiler) ActionSet(ctx context.Context, id string) (*cadestrov1.Manifest, error) {
	if !validInput(ctx, id) {
		return nil, ErrInvalidInput
	}
	set, err := c.store.GetManifestActionSet(ctx, id)
	if err != nil {
		return nil, err
	}
	rows, err := c.store.ListManifestActionSetActions(ctx, id)
	if err != nil {
		return nil, err
	}
	return c.compileSet(set, rows, &cadestrov1.ManifestProvenance{ActionSetId: id}, nil)
}

// Definition flattens a Definition into one globally ordered runbook.
func (c *Compiler) Definition(ctx context.Context, id string) (*cadestrov1.Manifest, error) {
	if !validInput(ctx, id) {
		return nil, ErrInvalidInput
	}
	definition, err := c.store.GetManifestDefinition(ctx, id)
	if err != nil {
		return nil, err
	}
	sets, err := c.store.ListManifestDefinitionActionSets(ctx, id)
	if err != nil {
		return nil, err
	}
	rows, err := c.store.ListManifestDefinitionActions(ctx, id)
	if err != nil {
		return nil, err
	}
	actionsBySet := make(map[string][]store.ActionRow, len(sets))
	for _, row := range rows {
		actionsBySet[row.ActionSetID] = append(actionsBySet[row.ActionSetID], row.Action)
	}
	schedule, err := requiredSchedule(definition.Schedule)
	if err != nil {
		return nil, fmt.Errorf("manifest definition schedule: %w", err)
	}
	runbook := &cadestrov1.Manifest{
		ManifestId: ulid.Make().String(), Provenance: &cadestrov1.ManifestProvenance{DefinitionId: id},
		Schedule: schedule, DefaultOnFailure: cadestrov1.OnFailure_ON_FAILURE_CONTINUE,
		Occurrences: make([]*cadestrov1.ManifestOccurrence, 0),
	}
	for _, set := range sets {
		setRows := actionsBySet[set.ID]
		if len(setRows) == 0 {
			continue
		}
		policy := cadestrov1.OnFailure(set.OnFailure)
		if policy != cadestrov1.OnFailure_ON_FAILURE_CONTINUE && policy != cadestrov1.OnFailure_ON_FAILURE_STOP {
			return nil, fmt.Errorf("action set %s has invalid failure policy %d", set.ID, set.OnFailure)
		}
		for _, row := range setRows {
			action, err := c.compileAction(row)
			if err != nil {
				return nil, fmt.Errorf("manifest: definition %s set %s: %w", id, set.ID, err)
			}
			runbook.Occurrences = append(runbook.Occurrences, occurrence(action, policy))
		}
	}
	if len(runbook.Occurrences) == 0 {
		return nil, ErrEmptyManifest
	}
	return finish(runbook)
}

// FreshCopy preserves compiled semantics while reminting delivery-local
// manifest and occurrence identities for another target device.
func FreshCopy(compiled *cadestrov1.Manifest) (*cadestrov1.Manifest, error) {
	if compiled == nil {
		return nil, ErrInvalidInput
	}
	cloned, ok := proto.Clone(compiled).(*cadestrov1.Manifest)
	if !ok {
		return nil, ErrInvalidInput
	}
	cloned.ManifestId = ulid.Make().String()
	for _, occurrence := range cloned.Occurrences {
		if occurrence == nil {
			return nil, ErrInvalidInput
		}
		occurrence.OccurrenceId = ulid.Make().String()
	}
	return finish(cloned)
}

func (c *Compiler) compileSet(set store.ActionSetRow, rows []store.ActionRow, provenance *cadestrov1.ManifestProvenance, scheduleOverride []byte) (*cadestrov1.Manifest, error) {
	if len(rows) == 0 {
		return nil, ErrEmptyManifest
	}
	scheduleRaw := set.Schedule
	if scheduleOverride != nil {
		scheduleRaw = scheduleOverride
	}
	schedule, err := requiredSchedule(scheduleRaw)
	if err != nil {
		return nil, fmt.Errorf("manifest schedule: %w", err)
	}
	policy := cadestrov1.OnFailure(set.OnFailure)
	if policy != cadestrov1.OnFailure_ON_FAILURE_CONTINUE && policy != cadestrov1.OnFailure_ON_FAILURE_STOP {
		return nil, fmt.Errorf("action set %s has invalid failure policy %d", set.ID, set.OnFailure)
	}
	manifest := &cadestrov1.Manifest{
		ManifestId:       ulid.Make().String(),
		Provenance:       provenance,
		Schedule:         schedule,
		DefaultOnFailure: policy,
		Occurrences:      make([]*cadestrov1.ManifestOccurrence, 0, len(rows)),
	}
	for _, row := range rows {
		action, err := c.compileAction(row)
		if err != nil {
			return nil, err
		}
		manifest.Occurrences = append(manifest.Occurrences, occurrence(action, policy))
	}
	return finish(manifest)
}

func (c *Compiler) compileAction(row store.ActionRow) (*cadestrov1.Action, error) {
	schedule, err := actionparams.ParseSchedule(row.Schedule)
	if err != nil {
		return nil, fmt.Errorf("manifest: action %s schedule: %w", row.ID, err)
	}
	action := &cadestrov1.Action{
		Id:             &cadestrov1.ActionId{Value: row.ID},
		Type:           cadestrov1.ActionType(row.ActionType),
		DesiredState:   cadestrov1.DesiredState(row.DesiredState),
		TimeoutSeconds: row.TimeoutSeconds,
		Schedule:       schedule,
	}
	switch action.Type {
	case cadestrov1.ActionType_ACTION_TYPE_ENCRYPTION:
		params, err := c.encryptionParams(row)
		if err != nil {
			return nil, fmt.Errorf("manifest: action %s params: %w", row.ID, err)
		}
		action.Params = &cadestrov1.Action_Encryption{Encryption: params}
	case cadestrov1.ActionType_ACTION_TYPE_WIFI:
		params, err := c.wifiParams(row)
		if err != nil {
			return nil, fmt.Errorf("manifest: action %s params: %w", row.ID, err)
		}
		action.Params = &cadestrov1.Action_Wifi{Wifi: params}
	default:
		if err := actionparams.PopulateAction(action, row.ActionType, row.Params); err != nil {
			return nil, fmt.Errorf("manifest: action %s params: %w", row.ID, err)
		}
	}
	if err := protovalidate.Validate(action); err != nil {
		return nil, fmt.Errorf("manifest: action %s invalid: %s", row.ID, err)
	}
	return action, nil
}

func (c *Compiler) encryptionParams(row store.ActionRow) (*cadestrov1.EncryptionParams, error) {
	stored := &cadestrov1.EncryptionAuthoringParams{}
	if err := actionparams.UnmarshalActionParams(row.Params, stored); err != nil {
		return nil, err
	}
	secret, err := secretActionField(stored.GetPresharedKey())
	if err != nil {
		return nil, err
	}
	return &cadestrov1.EncryptionParams{
		PresharedKey: secret, RotationIntervalDays: stored.RotationIntervalDays,
		MinWords: stored.MinWords, DeviceBoundKeyType: stored.DeviceBoundKeyType,
		UserPassphraseMinLength:  stored.UserPassphraseMinLength,
		UserPassphraseComplexity: stored.UserPassphraseComplexity,
	}, nil
}

func (c *Compiler) wifiParams(row store.ActionRow) (*cadestrov1.WifiParams, error) {
	stored := &cadestrov1.WifiAuthoringParams{}
	if err := actionparams.UnmarshalActionParams(row.Params, stored); err != nil {
		return nil, err
	}
	params := &cadestrov1.WifiParams{
		Ssid: stored.Ssid, AuthType: stored.AuthType, CaCert: stored.CaCert,
		ClientCert: stored.ClientCert, Identity: stored.Identity,
		AutoConnect: stored.AutoConnect, Hidden: stored.Hidden, Priority: stored.Priority,
	}
	var err error
	switch stored.AuthType {
	case cadestrov1.WifiAuthType_WIFI_AUTH_TYPE_PSK:
		params.Psk, err = secretActionField(stored.GetPsk())
	case cadestrov1.WifiAuthType_WIFI_AUTH_TYPE_EAP_TLS:
		params.ClientKey, err = secretActionField(stored.GetClientKey())
	default:
		return nil, errors.New("unsupported WiFi authentication type")
	}
	return params, err
}

func secretActionField(ciphertext string) ([]byte, error) {
	if !crypto.IsEncryptedValue(ciphertext) {
		return nil, errors.New("action secret compiler requires encrypted storage")
	}
	return []byte(ciphertext), nil
}

// MaterializeSecrets opens catalog ciphertext only for the authenticated
// device stream. Callers must pass a copy that is never persisted.
func MaterializeSecrets(manifest *cadestrov1.Manifest, atRest *crypto.Encryptor) error {
	if manifest == nil || atRest == nil {
		return errors.New("manifest secret materialization requires manifest and cipher")
	}
	for _, occurrence := range manifest.Occurrences {
		if occurrence == nil || occurrence.Action == nil || occurrence.Action.Id == nil {
			continue
		}
		actionID := occurrence.Action.Id.Value
		open := func(value *[]byte, purpose string) error {
			if value == nil || len(*value) == 0 {
				return nil
			}
			plaintext, err := atRest.DecryptWithContext(string(*value), crypto.RowAAD(actionID, purpose))
			if err != nil {
				return fmt.Errorf("open manifest secret: %w", err)
			}
			*value = []byte(plaintext)
			return nil
		}
		switch params := occurrence.Action.Params.(type) {
		case *cadestrov1.Action_Encryption:
			if err := open(&params.Encryption.PresharedKey, crypto.PurposeActionEncryptionPresharedKey); err != nil {
				return err
			}
		case *cadestrov1.Action_Wifi:
			switch params.Wifi.AuthType {
			case cadestrov1.WifiAuthType_WIFI_AUTH_TYPE_PSK:
				if err := open(&params.Wifi.Psk, crypto.PurposeActionWifiPSK); err != nil {
					return err
				}
			case cadestrov1.WifiAuthType_WIFI_AUTH_TYPE_EAP_TLS:
				if err := open(&params.Wifi.ClientKey, crypto.PurposeActionWifiClientKey); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func requiredSchedule(raw []byte) (*cadestrov1.ActionSchedule, error) {
	schedule, err := actionparams.ParseSchedule(raw)
	if err != nil {
		return nil, err
	}
	if schedule == nil {
		schedule = &cadestrov1.ActionSchedule{}
	}
	return schedule, nil
}

func occurrence(action *cadestrov1.Action, policy cadestrov1.OnFailure) *cadestrov1.ManifestOccurrence {
	return &cadestrov1.ManifestOccurrence{
		OccurrenceId: ulid.Make().String(),
		Action:       action,
		OnFailure:    policy,
	}
}

func finish(manifest *cadestrov1.Manifest) (*cadestrov1.Manifest, error) {
	if err := protovalidate.Validate(manifest); err != nil {
		return nil, fmt.Errorf("manifest: compiled output invalid: %s", err)
	}
	return manifest, nil
}

func validInput(ctx context.Context, id string) bool {
	if ctx == nil {
		return false
	}
	_, err := ulid.ParseStrict(id)
	return err == nil
}

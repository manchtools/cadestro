// Package manifest compiles the authoring hierarchy into the flat, durable
// unit of work sent to an agent.
package manifest

import (
	"context"
	"errors"
	"fmt"

	"github.com/oklog/ulid/v2"

	pmv1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	sdkvalidate "github.com/manchtools/cadestro/contract/validate"
	"github.com/manchtools/cadestro/server/internal/actionparams"
	pmcrypto "github.com/manchtools/cadestro/server/internal/crypto"
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

var validator = sdkvalidate.NewValidator()

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
func (c *Compiler) Action(ctx context.Context, id string) (*pmv1.Manifest, error) {
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
		schedule = &pmv1.ActionSchedule{}
	}
	return finish(&pmv1.Manifest{
		ManifestId:       ulid.Make().String(),
		Provenance:       &pmv1.ManifestProvenance{ActionId: id},
		Schedule:         schedule,
		DefaultOnFailure: pmv1.OnFailure_ON_FAILURE_CONTINUE,
		Occurrences:      []*pmv1.ManifestOccurrence{occurrence(action, pmv1.OnFailure_ON_FAILURE_CONTINUE)},
	})
}

// ActionSet flattens one set into a manifest in authored member order.
func (c *Compiler) ActionSet(ctx context.Context, id string) (*pmv1.Manifest, error) {
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
	return c.compileSet(set, rows, &pmv1.ManifestProvenance{ActionSetId: id}, nil)
}

// Definition flattens a Definition into one globally ordered runbook.
func (c *Compiler) Definition(ctx context.Context, id string) (*pmv1.Manifest, error) {
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
	runbook := &pmv1.Manifest{
		ManifestId: ulid.Make().String(), Provenance: &pmv1.ManifestProvenance{DefinitionId: id},
		Schedule: schedule, DefaultOnFailure: pmv1.OnFailure_ON_FAILURE_CONTINUE,
		Occurrences: make([]*pmv1.ManifestOccurrence, 0),
	}
	for _, set := range sets {
		setRows := actionsBySet[set.ID]
		if len(setRows) == 0 {
			continue
		}
		policy := pmv1.OnFailure(set.OnFailure)
		if policy != pmv1.OnFailure_ON_FAILURE_CONTINUE && policy != pmv1.OnFailure_ON_FAILURE_STOP {
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

// OneShotAction creates the singleton manifest used by an explicit dispatch.
// The Action schedule remains authoring/display data; the manifest carries the
// structural one_shot flag, which is what makes the agent execute the delivery
// exactly once after recording it instead of scheduling it. An empty manifest
// schedule accompanies the flag but never stands in for it.
func OneShotAction(action *pmv1.Action) (*pmv1.Manifest, error) {
	if action == nil {
		return nil, ErrInvalidInput
	}
	cloned, ok := proto.Clone(action).(*pmv1.Action)
	if !ok || cloned.GetId() == nil || !validInput(context.Background(), cloned.Id.Value) {
		return nil, ErrInvalidInput
	}
	return finish(&pmv1.Manifest{
		ManifestId:       ulid.Make().String(),
		Provenance:       &pmv1.ManifestProvenance{ActionId: cloned.Id.Value},
		Schedule:         &pmv1.ActionSchedule{},
		DefaultOnFailure: pmv1.OnFailure_ON_FAILURE_CONTINUE,
		Occurrences:      []*pmv1.ManifestOccurrence{occurrence(cloned, pmv1.OnFailure_ON_FAILURE_CONTINUE)},
		OneShot:          true,
	})
}

// AsOneShot marks a manifest compiled from the catalog as an explicit dispatch.
// The structural one_shot flag is what makes the agent execute the delivery
// exactly once; clearing the compiled schedule stops the
// authored cadence from also being installed. The nested Actions keep their
// authoring/display schedules.
func AsOneShot(compiled *pmv1.Manifest) *pmv1.Manifest {
	if compiled == nil {
		return nil
	}
	compiled.Schedule = &pmv1.ActionSchedule{}
	compiled.OneShot = true
	return compiled
}

// FreshCopy preserves compiled semantics while reminting delivery-local
// manifest and occurrence identities for another target device.
func FreshCopy(compiled *pmv1.Manifest) (*pmv1.Manifest, error) {
	if compiled == nil {
		return nil, ErrInvalidInput
	}
	cloned, ok := proto.Clone(compiled).(*pmv1.Manifest)
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

func (c *Compiler) compileSet(set store.ActionSetRow, rows []store.ActionRow, provenance *pmv1.ManifestProvenance, scheduleOverride []byte) (*pmv1.Manifest, error) {
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
	policy := pmv1.OnFailure(set.OnFailure)
	if policy != pmv1.OnFailure_ON_FAILURE_CONTINUE && policy != pmv1.OnFailure_ON_FAILURE_STOP {
		return nil, fmt.Errorf("action set %s has invalid failure policy %d", set.ID, set.OnFailure)
	}
	manifest := &pmv1.Manifest{
		ManifestId:       ulid.Make().String(),
		Provenance:       provenance,
		Schedule:         schedule,
		DefaultOnFailure: policy,
		Occurrences:      make([]*pmv1.ManifestOccurrence, 0, len(rows)),
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

func (c *Compiler) compileAction(row store.ActionRow) (*pmv1.Action, error) {
	schedule, err := actionparams.ParseSchedule(row.Schedule)
	if err != nil {
		return nil, fmt.Errorf("manifest: action %s schedule: %w", row.ID, err)
	}
	action := &pmv1.Action{
		Id:             &pmv1.ActionId{Value: row.ID},
		Type:           pmv1.ActionType(row.ActionType),
		DesiredState:   pmv1.DesiredState(row.DesiredState),
		TimeoutSeconds: row.TimeoutSeconds,
		Schedule:       schedule,
	}
	switch action.Type {
	case pmv1.ActionType_ACTION_TYPE_ENCRYPTION:
		params, err := c.encryptionParams(row)
		if err != nil {
			return nil, fmt.Errorf("manifest: action %s params: %w", row.ID, err)
		}
		action.Params = &pmv1.Action_Encryption{Encryption: params}
	case pmv1.ActionType_ACTION_TYPE_WIFI:
		params, err := c.wifiParams(row)
		if err != nil {
			return nil, fmt.Errorf("manifest: action %s params: %w", row.ID, err)
		}
		action.Params = &pmv1.Action_Wifi{Wifi: params}
	default:
		if err := actionparams.PopulateAction(action, row.ActionType, row.Params); err != nil {
			return nil, fmt.Errorf("manifest: action %s params: %w", row.ID, err)
		}
	}
	if detail, ok := sdkvalidate.Struct(validator, action); !ok {
		return nil, fmt.Errorf("manifest: action %s invalid: %s", row.ID, detail)
	}
	return action, nil
}

func (c *Compiler) encryptionParams(row store.ActionRow) (*pmv1.EncryptionParams, error) {
	stored := &pmv1.EncryptionAuthoringParams{}
	if err := actionparams.UnmarshalActionParams(row.Params, stored); err != nil {
		return nil, err
	}
	secret, err := secretActionField(stored.GetPresharedKey())
	if err != nil {
		return nil, err
	}
	return &pmv1.EncryptionParams{
		PresharedKey: secret, RotationIntervalDays: stored.RotationIntervalDays,
		MinWords: stored.MinWords, DeviceBoundKeyType: stored.DeviceBoundKeyType,
		UserPassphraseMinLength:  stored.UserPassphraseMinLength,
		UserPassphraseComplexity: stored.UserPassphraseComplexity,
	}, nil
}

func (c *Compiler) wifiParams(row store.ActionRow) (*pmv1.WifiParams, error) {
	stored := &pmv1.WifiAuthoringParams{}
	if err := actionparams.UnmarshalActionParams(row.Params, stored); err != nil {
		return nil, err
	}
	params := &pmv1.WifiParams{
		Ssid: stored.Ssid, AuthType: stored.AuthType, CaCert: stored.CaCert,
		ClientCert: stored.ClientCert, Identity: stored.Identity,
		AutoConnect: stored.AutoConnect, Hidden: stored.Hidden, Priority: stored.Priority,
	}
	var err error
	switch stored.AuthType {
	case pmv1.WifiAuthType_WIFI_AUTH_TYPE_PSK:
		params.Psk, err = secretActionField(stored.GetPsk())
	case pmv1.WifiAuthType_WIFI_AUTH_TYPE_EAP_TLS:
		params.ClientKey, err = secretActionField(stored.GetClientKey())
	default:
		return nil, errors.New("unsupported WiFi authentication type")
	}
	return params, err
}

func secretActionField(ciphertext string) ([]byte, error) {
	if !pmcrypto.IsEncryptedValue(ciphertext) {
		return nil, errors.New("action secret compiler requires encrypted storage")
	}
	return []byte(ciphertext), nil
}

// MaterializeSecrets opens catalog ciphertext only for the authenticated
// device stream. Callers must pass a copy that is never persisted.
func MaterializeSecrets(manifest *pmv1.Manifest, atRest *pmcrypto.Encryptor) error {
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
			plaintext, err := atRest.DecryptWithContext(string(*value), pmcrypto.RowAAD(actionID, purpose))
			if err != nil {
				return fmt.Errorf("open manifest secret: %w", err)
			}
			*value = []byte(plaintext)
			return nil
		}
		switch params := occurrence.Action.Params.(type) {
		case *pmv1.Action_Encryption:
			if err := open(&params.Encryption.PresharedKey, pmcrypto.PurposeActionEncryptionPresharedKey); err != nil {
				return err
			}
		case *pmv1.Action_Wifi:
			switch params.Wifi.AuthType {
			case pmv1.WifiAuthType_WIFI_AUTH_TYPE_PSK:
				if err := open(&params.Wifi.Psk, pmcrypto.PurposeActionWifiPSK); err != nil {
					return err
				}
			case pmv1.WifiAuthType_WIFI_AUTH_TYPE_EAP_TLS:
				if err := open(&params.Wifi.ClientKey, pmcrypto.PurposeActionWifiClientKey); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func requiredSchedule(raw []byte) (*pmv1.ActionSchedule, error) {
	schedule, err := actionparams.ParseSchedule(raw)
	if err != nil {
		return nil, err
	}
	if schedule == nil {
		schedule = &pmv1.ActionSchedule{}
	}
	return schedule, nil
}

func occurrence(action *pmv1.Action, policy pmv1.OnFailure) *pmv1.ManifestOccurrence {
	return &pmv1.ManifestOccurrence{
		OccurrenceId: ulid.Make().String(),
		Action:       action,
		OnFailure:    policy,
	}
}

func finish(manifest *pmv1.Manifest) (*pmv1.Manifest, error) {
	if detail, ok := sdkvalidate.Struct(validator, manifest); !ok {
		return nil, fmt.Errorf("manifest: compiled output invalid: %s", detail)
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

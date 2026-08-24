package devicecontrol

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/assignment"
	"github.com/manchtools/cadestro/server/internal/manifest"
	"github.com/manchtools/cadestro/server/internal/store"
	"github.com/oklog/ulid/v2"
	"google.golang.org/protobuf/proto"
)

// CompileAssigned is the single assignment/compiler path used by both the
// admin RPC and authenticated agent Sync. A nil visibility callback is
// reserved for the mTLS device path; user RPCs provide their scope check.
func CompileAssigned(ctx context.Context, st *store.Store, compiler *manifest.Compiler, deviceID string, visible func(context.Context, string, string) error) ([]*cadestrov1.Manifest, error) {
	paths, err := st.ListResolvedSources(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	sources, err := assignment.ResolveSources(paths)
	if err != nil {
		return nil, err
	}
	byType := make(map[string][]assignment.ResolvedSource)
	for _, source := range sources {
		byType[source.Row.SourceType] = append(byType[source.Row.SourceType], source)
	}

	manifests := make([]*cadestrov1.Manifest, 0)
	absorbedSets := make(map[string]struct{})
	absorbedActions := make(map[string]struct{})
	emittedActions := make(map[string]struct{})

	for _, source := range byType["definition"] {
		if !source.Active && !source.Excluded {
			continue
		}
		sets, err := st.ListManifestDefinitionActionSets(ctx, source.Row.SourceID)
		if err != nil {
			return nil, err
		}
		for _, set := range sets {
			absorbedSets[set.ID] = struct{}{}
		}
		actions, err := st.ListManifestDefinitionActions(ctx, source.Row.SourceID)
		if err != nil {
			return nil, err
		}
		for _, action := range actions {
			absorbedActions[action.Action.ID] = struct{}{}
		}
		if !source.Active {
			continue
		}
		if visible != nil {
			if err := visible(ctx, source.Row.SourceType, source.Row.SourceID); err != nil {
				return nil, err
			}
		}
		compiled, err := compiler.Definition(ctx, source.Row.SourceID)
		if errors.Is(err, manifest.ErrEmptyManifest) {
			continue
		}
		if err != nil {
			return nil, err
		}
		forceAbsent(compiled, source.ForceAbsent)
		if err := stablePolicyIdentityForSource(ctx, st, compiled, source.Row.SourceType, source.Row.SourceID, fmt.Sprintf("force_absent:%t", source.ForceAbsent)); err != nil {
			return nil, err
		}
		manifests = append(manifests, compiled)
		rememberManifestActions(emittedActions, compiled)
	}

	for _, source := range byType["action_set"] {
		if _, absorbed := absorbedSets[source.Row.SourceID]; absorbed || (!source.Active && !source.Excluded) {
			continue
		}
		actions, err := st.ListManifestActionSetActions(ctx, source.Row.SourceID)
		if err != nil {
			return nil, err
		}
		for _, action := range actions {
			absorbedActions[action.ID] = struct{}{}
		}
		if !source.Active {
			continue
		}
		if visible != nil {
			if err := visible(ctx, source.Row.SourceType, source.Row.SourceID); err != nil {
				return nil, err
			}
		}
		compiled, err := compiler.ActionSet(ctx, source.Row.SourceID)
		if errors.Is(err, manifest.ErrEmptyManifest) {
			continue
		}
		if err != nil {
			return nil, err
		}
		forceAbsent(compiled, source.ForceAbsent)
		if err := stablePolicyIdentityForSource(ctx, st, compiled, source.Row.SourceType, source.Row.SourceID, fmt.Sprintf("force_absent:%t", source.ForceAbsent)); err != nil {
			return nil, err
		}
		manifests = append(manifests, compiled)
		rememberManifestActions(emittedActions, compiled)
	}

	blockedActions := make(map[string]struct{})
	for _, source := range byType["action"] {
		if source.Excluded {
			blockedActions[source.Row.SourceID] = struct{}{}
		}
		if !source.Active {
			continue
		}
		if _, absorbed := absorbedActions[source.Row.SourceID]; absorbed {
			continue
		}
		if visible != nil {
			if err := visible(ctx, source.Row.SourceType, source.Row.SourceID); err != nil {
				return nil, err
			}
		}
		compiled, err := compiler.Action(ctx, source.Row.SourceID)
		if err != nil {
			return nil, err
		}
		forceAbsent(compiled, source.ForceAbsent)
		if err := stablePolicyIdentityForSource(ctx, st, compiled, source.Row.SourceType, source.Row.SourceID, fmt.Sprintf("force_absent:%t", source.ForceAbsent)); err != nil {
			return nil, err
		}
		manifests = append(manifests, compiled)
		rememberManifestActions(emittedActions, compiled)
	}

	for _, source := range byType["compliance_policy"] {
		if !source.Active {
			continue
		}
		rules, err := st.ListCompliancePolicyRules(ctx, source.Row.SourceID)
		if err != nil {
			return nil, err
		}
		for _, rule := range rules {
			if _, blocked := blockedActions[rule.ActionID]; blocked {
				continue
			}
			if _, absorbed := absorbedActions[rule.ActionID]; absorbed {
				continue
			}
			if _, emitted := emittedActions[rule.ActionID]; emitted {
				continue
			}
			if visible != nil {
				if err := visible(ctx, "action", rule.ActionID); err != nil {
					return nil, err
				}
			}
			compiled, err := compiler.Action(ctx, rule.ActionID)
			if err != nil {
				return nil, err
			}
			forceAbsent(compiled, source.ForceAbsent)
			if err := stablePolicyIdentityForSource(ctx, st, compiled, source.Row.SourceType, source.Row.SourceID, fmt.Sprintf("force_absent:%t:%s", source.ForceAbsent, rule.ActionID)); err != nil {
				return nil, err
			}
			manifests = append(manifests, compiled)
			rememberManifestActions(emittedActions, compiled)
		}
	}

	for sourceType := range byType {
		switch sourceType {
		case "action", "action_set", "definition", "compliance_policy":
		default:
			return nil, fmt.Errorf("unknown assigned source type %q", sourceType)
		}
	}
	return manifests, nil
}

// AssignedPolicy returns the authenticated device snapshot without applying
// a user caller's authoring visibility filter.
func (h *Handlers) AssignedPolicy(ctx context.Context, deviceID string) ([]*cadestrov1.Manifest, error) {
	return CompileAssigned(ctx, h.store, h.compiler, deviceID, nil)
}

func forceAbsent(compiled *cadestrov1.Manifest, enabled bool) {
	if !enabled {
		return
	}
	for _, occurrence := range compiled.Occurrences {
		occurrence.Action.DesiredState = cadestrov1.DesiredState_DESIRED_STATE_ABSENT
	}
}

func rememberManifestActions(seen map[string]struct{}, compiled *cadestrov1.Manifest) {
	for _, occurrence := range compiled.Occurrences {
		seen[occurrence.Action.Id.Value] = struct{}{}
	}
}

func stablePolicyIdentityForSource(ctx context.Context, st *store.Store, manifest *cadestrov1.Manifest, sourceType, sourceID, extra string) error {
	seed, err := sourceIdentity(ctx, st, sourceType, sourceID, extra)
	if err != nil {
		return err
	}
	manifest.ManifestId = policyULID(seed)
	for index, occurrence := range manifest.Occurrences {
		occurrence.OccurrenceId = policyULID(append(append([]byte(nil), seed...), byte(index>>24), byte(index>>16), byte(index>>8), byte(index)))
	}
	return nil
}

// stablePolicyIdentity remains a small pure helper for callers that already
// hold a fully authored manifest. Assignment pull uses the source canonical
// content above so outbound secret materialization cannot enter the identity.
func stablePolicyIdentity(manifest *cadestrov1.Manifest) {
	clone := proto.Clone(manifest).(*cadestrov1.Manifest)
	clone.ManifestId = ""
	for _, occurrence := range clone.Occurrences {
		occurrence.OccurrenceId = ""
	}
	seed, err := proto.MarshalOptions{Deterministic: true}.Marshal(clone)
	if err != nil {
		return
	}
	manifest.ManifestId = policyULID(seed)
	for index, occurrence := range manifest.Occurrences {
		occurrence.OccurrenceId = policyULID(append(append([]byte(nil), seed...), byte(index>>24), byte(index>>16), byte(index>>8), byte(index)))
	}
}

func sourceIdentity(ctx context.Context, st *store.Store, sourceType, sourceID, extra string) ([]byte, error) {
	seed := make([]byte, 0, 256)
	seed = identityPart(seed, sourceType)
	seed = identityPart(seed, sourceID)
	seed = identityPart(seed, extra)
	addAction := func(row store.ActionRow) {
		seed = identityPart(seed, row.ID)
		seed = identityPart(seed, fmt.Sprintf("%d:%d:%d", row.ActionType, row.DesiredState, row.TimeoutSeconds))
		seed = identityPart(seed, row.ParamsCanonical)
		seed = identityPart(seed, row.Schedule)
	}
	switch sourceType {
	case "action":
		row, err := st.GetManifestAction(ctx, sourceID)
		if err != nil {
			return nil, err
		}
		addAction(row)
	case "action_set":
		set, err := st.GetManifestActionSet(ctx, sourceID)
		if err != nil {
			return nil, err
		}
		seed = identityPart(seed, set.Schedule)
		seed = identityPart(seed, fmt.Sprintf("on_failure:%d", set.OnFailure))
		rows, err := st.ListManifestActionSetActions(ctx, sourceID)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			addAction(row)
		}
	case "definition":
		definition, err := st.GetManifestDefinition(ctx, sourceID)
		if err != nil {
			return nil, err
		}
		seed = identityPart(seed, definition.Schedule)
		sets, err := st.ListManifestDefinitionActionSets(ctx, sourceID)
		if err != nil {
			return nil, err
		}
		for _, set := range sets {
			seed = identityPart(seed, set.ID)
			seed = identityPart(seed, fmt.Sprintf("on_failure:%d", set.OnFailure))
		}
		rows, err := st.ListManifestDefinitionActions(ctx, sourceID)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			seed = identityPart(seed, row.ActionSetID)
			addAction(row.Action)
		}
	case "compliance_policy":
		rules, err := st.ListCompliancePolicyRules(ctx, sourceID)
		if err != nil {
			return nil, err
		}
		for _, rule := range rules {
			seed = identityPart(seed, rule.ActionID)
			row, err := st.GetManifestAction(ctx, rule.ActionID)
			if err != nil {
				return nil, err
			}
			addAction(row)
		}
	default:
		return nil, fmt.Errorf("unknown assigned source type %q", sourceType)
	}
	return seed, nil
}

func identityPart(seed []byte, value any) []byte {
	var raw []byte
	switch v := value.(type) {
	case string:
		raw = []byte(v)
	case []byte:
		raw = v
	default:
		raw = []byte(fmt.Sprint(v))
	}
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(raw)))
	seed = append(seed, length[:]...)
	return append(seed, raw...)
}

func policyULID(seed []byte) string {
	digest := sha256.Sum256(seed)
	digest[0] &= 0x03
	return ulid.ULID(digest[:16]).String()
}

package scim

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

type groupAssertion struct {
	DisplayName string
	Members     []string

	MembersAsserted bool
}

func assertionFromGroup(g SCIMGroup) groupAssertion {
	a := groupAssertion{DisplayName: g.DisplayName}
	if g.Members != nil {
		a.MembersAsserted = true
		a.Members = make([]string, 0, len(g.Members))
		for _, m := range g.Members {
			if m.Value != "" {
				a.Members = append(a.Members, m.Value)
			}
		}
	}
	return a
}

func (h *Handler) createGroup(w http.ResponseWriter, r *http.Request, s *session) {
	ctx := r.Context()
	var resource SCIMGroup
	if !decodeBody(w, r, &resource) {
		return
	}
	if resource.DisplayName == "" {
		writeError(w, http.StatusBadRequest, "displayName is required")
		return
	}
	baseURL := baseURLFromRequest(r, s.provider.Slug)
	assertion := assertionFromGroup(resource)

	scimGroupID := resource.ExternalID
	if scimGroupID == "" {
		scimGroupID = resource.ID
	}
	if scimGroupID == "" {
		scimGroupID = ulid.Make().String()
	}

	existing, err := h.store.GetSCIMGroupMapping(ctx, s.provider.ID, scimGroupID)
	switch {
	case err == nil:
		if _, groupErr := h.store.GetUserGroup(ctx, existing.UserGroupID); groupErr == nil {
			h.syncGroup(w, r, s, existing, assertion, baseURL, http.StatusOK)
			return
		} else if !store.IsNotFound(groupErr) {
			h.logger.Error("scim: failed to resolve the mapped group", "error", groupErr)
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}

		h.provisionGroup(w, r, s, scimGroupID, assertion, baseURL, &existing)
	case store.IsNotFound(err):
		h.provisionGroup(w, r, s, scimGroupID, assertion, baseURL, nil)
	default:
		h.logger.Error("scim: failed to resolve the group mapping", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func (h *Handler) provisionGroup(
	w http.ResponseWriter,
	r *http.Request,
	s *session,
	scimGroupID string,
	a groupAssertion,
	baseURL string,
	stale *store.SCIMGroupMappingRow,
) {
	ctx := r.Context()
	groupID := ulid.Make().String()
	mappingID := ulid.Make().String()
	at := h.now().UTC()

	_, err := h.store.WithAudit(ctx, h.mutationOp(s), func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		if stale != nil {
			removed, err := tx.DeleteSCIMGroupMapping(ctx, stale.ID)
			if err != nil {
				return err
			}
			rec.Effect(store.AuditEffect{
				ResourceType: "scim_group_mapping",
				ResourceID:   removed.ID,
				Action:       "UNMAP",
				Outcome:      store.EffectApplied,
				BeforeRef:    &removed.UserGroupID,
				AfterRef:     &s.provider.ID,
			})

			rec.RefreshSearch("user_group", removed.UserGroupID)
		}

		if _, err := tx.InsertUserGroup(ctx, db.InsertUserGroupParams{
			ID:          groupID,
			Name:        a.DisplayName,
			Description: "provisioned by the " + s.provider.Slug + " directory",
			CreatedAt:   at,
			CreatedBy:   actorSCIM,
		}); err != nil {
			return err
		}
		rec.Effect(store.AuditEffect{
			ResourceType:  "user_group",
			ResourceID:    groupID,
			Action:        "CREATE",
			Outcome:       store.EffectApplied,
			ChangedFields: []string{"name", "description"},
			AfterRef:      &s.provider.ID,
		})

		if _, err := tx.InsertSCIMGroupMapping(ctx, db.InsertSCIMGroupMappingParams{
			ID:              mappingID,
			ProviderID:      s.provider.ID,
			ScimGroupID:     scimGroupID,
			ScimDisplayName: a.DisplayName,
			UserGroupID:     groupID,
			CreatedAt:       at,
		}); err != nil {
			return err
		}
		rec.Effect(store.AuditEffect{
			ResourceType:        "scim_group_mapping",
			ResourceID:          mappingID,
			Action:              "MAP",
			Outcome:             store.EffectApplied,
			ChangedFields:       []string{"scim_group_id", "scim_display_name", "user_group_id"},
			BeforeRef:           &s.provider.ID,
			AfterRef:            &groupID,
			EvidenceKind:        "external_group_sha256",
			EvidenceFingerprint: fingerprint(scimGroupID),
		})

		if !a.MembersAsserted {
			return nil
		}
		return h.reconcileMembers(ctx, tx, rec, s, groupID, a.Members, at)
	})
	if err != nil {
		if store.IsConflict(err) {
			writeError(w, http.StatusConflict, "a group with that name already exists")
			return
		}
		h.logger.Error("scim: failed to provision the group", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create group")
		return
	}

	mapping, err := h.store.GetSCIMGroupMapping(ctx, s.provider.ID, scimGroupID)
	if err != nil {
		h.logger.Error("scim: failed to read back the group mapping", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to read group")
		return
	}
	h.writeGroup(ctx, w, mapping, baseURL, http.StatusCreated)
}

func (h *Handler) replaceGroup(w http.ResponseWriter, r *http.Request, s *session) {
	ctx := r.Context()
	mapping, ok := h.resolveGroup(ctx, w, s, r.PathValue("id"))
	if !ok {
		return
	}
	if _, err := h.store.GetUserGroup(ctx, mapping.UserGroupID); err != nil {
		if store.IsNotFound(err) {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}
		h.logger.Error("scim: failed to load the mapped group", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get group")
		return
	}
	var resource SCIMGroup
	if !decodeBody(w, r, &resource) {
		return
	}
	h.syncGroup(w, r, s, mapping, assertionFromGroup(resource),
		baseURLFromRequest(r, s.provider.Slug), http.StatusOK)
}

func (h *Handler) patchGroup(w http.ResponseWriter, r *http.Request, s *session) {
	ctx := r.Context()
	mapping, ok := h.resolveGroup(ctx, w, s, r.PathValue("id"))
	if !ok {
		return
	}
	group, err := h.store.GetUserGroup(ctx, mapping.UserGroupID)
	if err != nil {
		if store.IsNotFound(err) {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}
		h.logger.Error("scim: failed to load the mapped group", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get group")
		return
	}
	var patch SCIMPatchRequest
	if !decodeBody(w, r, &patch) {
		return
	}
	for _, op := range patch.Operations {

		if !op.Op.IsValid() {
			writeError(w, http.StatusBadRequest, "unsupported patch op")
			return
		}
	}

	baseURL := baseURLFromRequest(r, s.provider.Slug)
	at := h.now().UTC()

	_, err = h.store.WithAudit(ctx, h.mutationOp(s), func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		for _, op := range patch.Operations {
			if err := h.applyGroupPatchOp(ctx, tx, rec, s, mapping, group, op, at); err != nil {
				return err
			}
		}
		return noChangeIfNothingRecorded(rec)
	})
	if err != nil && !errors.Is(err, errNoChange) {
		h.logger.Error("scim: failed to apply the group patch", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to apply patch operation")
		return
	}
	h.writeGroup(ctx, w, mapping, baseURL, http.StatusOK)
}

func (h *Handler) applyGroupPatchOp(
	ctx context.Context,
	tx *store.Tx,
	rec *store.AuditRecorder,
	s *session,
	mapping store.SCIMGroupMappingRow,
	group store.UserGroupRow,
	op SCIMPatchOp,
	at time.Time,
) error {
	path := strings.ToLower(strings.TrimSpace(op.Path))

	switch op.Op.Normalize() {
	case SCIMPatchOpAdd:
		if path != "members" && path != "" {
			return nil
		}
		for _, userID := range patchMemberIDs(op.Value) {
			if err := h.addMember(ctx, tx, rec, s, mapping.UserGroupID, userID, at); err != nil {
				return err
			}
		}

	case SCIMPatchOpRemove:
		if strings.HasPrefix(path, "members[") {
			if userID := memberIDFromFilter(op.Path); userID != "" {
				return h.removeMember(ctx, tx, rec, mapping.UserGroupID, userID, at)
			}
			return nil
		}
		if path != "members" && path != "" {
			return nil
		}
		for _, userID := range patchMemberIDs(op.Value) {
			if err := h.removeMember(ctx, tx, rec, mapping.UserGroupID, userID, at); err != nil {
				return err
			}
		}

	case SCIMPatchOpReplace:
		switch path {
		case "displayname":
			name, ok := patchString(op.Value)
			if !ok || name == "" {
				return nil
			}
			return h.renameGroup(ctx, tx, rec, s, mapping, group, name, at)
		case "members":
			return h.reconcileMembers(ctx, tx, rec, s, mapping.UserGroupID, patchMemberIDs(op.Value), at)
		}
	}
	return nil
}

func (h *Handler) syncGroup(
	w http.ResponseWriter,
	r *http.Request,
	s *session,
	mapping store.SCIMGroupMappingRow,
	a groupAssertion,
	baseURL string,
	okStatus int,
) {
	ctx := r.Context()
	at := h.now().UTC()

	_, err := h.store.WithAudit(ctx, h.mutationOp(s), func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		group, err := tx.GetUserGroup(ctx, mapping.UserGroupID)
		if err != nil {
			return err
		}
		if a.DisplayName != "" && a.DisplayName != mapping.ScimDisplayName {
			if err := h.renameGroup(ctx, tx, rec, s, mapping, group, a.DisplayName, at); err != nil {
				return err
			}
		}
		if a.MembersAsserted {
			if err := h.reconcileMembers(ctx, tx, rec, s, mapping.UserGroupID, a.Members, at); err != nil {
				return err
			}
		}
		return noChangeIfNothingRecorded(rec)
	})
	if err != nil && !errors.Is(err, errNoChange) {
		if store.IsConflict(err) {
			writeError(w, http.StatusConflict, "a group with that name already exists")
			return
		}
		if store.IsNotFound(err) {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}
		h.logger.Error("scim: failed to apply the group assertion", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update group")
		return
	}
	h.writeGroup(ctx, w, mapping, baseURL, okStatus)
}

func (h *Handler) deleteGroup(w http.ResponseWriter, r *http.Request, s *session) {
	ctx := r.Context()
	mapping, ok := h.resolveGroup(ctx, w, s, r.PathValue("id"))
	if !ok {
		return
	}

	_, err := h.store.WithAudit(ctx, h.mutationOp(s), func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		removed, err := tx.DeleteSCIMGroupMapping(ctx, mapping.ID)
		if err != nil {
			return err
		}
		rec.Effect(store.AuditEffect{
			ResourceType:        "scim_group_mapping",
			ResourceID:          removed.ID,
			Action:              "UNMAP",
			Outcome:             store.EffectApplied,
			BeforeRef:           &removed.UserGroupID,
			AfterRef:            &s.provider.ID,
			EvidenceKind:        "external_group_sha256",
			EvidenceFingerprint: fingerprint(removed.ScimGroupID),
		})

		rec.RefreshSearch("user_group", removed.UserGroupID)
		return nil
	})
	if err != nil {
		h.logger.Error("scim: failed to unmap the group", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete group mapping")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) reconcileMembers(
	ctx context.Context,
	tx *store.Tx,
	rec *store.AuditRecorder,
	s *session,
	groupID string,
	requested []string,
	at time.Time,
) error {
	current, err := tx.ListUserGroupMemberIDs(ctx, groupID)
	if err != nil {
		return err
	}
	currentSet := make(map[string]bool, len(current))
	for _, id := range current {
		currentSet[id] = true
	}
	requestedSet := make(map[string]bool, len(requested))
	for _, id := range requested {
		requestedSet[id] = true
	}

	for _, userID := range requested {
		if currentSet[userID] {
			continue
		}
		if err := h.addMember(ctx, tx, rec, s, groupID, userID, at); err != nil {
			return err
		}
	}
	for _, userID := range current {
		if requestedSet[userID] {
			continue
		}
		if err := h.removeMember(ctx, tx, rec, groupID, userID, at); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) addMember(
	ctx context.Context,
	tx *store.Tx,
	rec *store.AuditRecorder,
	s *session,
	groupID, userID string,
	at time.Time,
) error {
	if _, err := tx.GetIdentityLinkByProviderAndUser(ctx, db.GetIdentityLinkByProviderAndUserParams{
		ProviderID: s.provider.ID,
		UserID:     userID,
	}); err != nil {
		if store.IsNotFound(err) {
			h.logger.Warn("scim: skipping a member the directory does not provision", "group_id", groupID)
			return nil
		}
		return err
	}

	n, err := tx.InsertUserGroupMember(ctx, db.InsertUserGroupMemberParams{
		GroupID: groupID,
		UserID:  userID,
		AddedAt: at,
		AddedBy: actorSCIM,
	})
	if err != nil {
		return err
	}
	if n == 0 {

		return nil
	}
	rec.Effect(store.AuditEffect{
		ResourceType: "user_group_member",
		ResourceID:   groupID,
		Action:       "JOIN",
		Outcome:      store.EffectApplied,
		AfterRef:     &userID,
	})
	rec.RefreshSearch("user_group", groupID)
	return h.invalidateSubjectSessions(ctx, tx, rec, userID, at)
}

func (h *Handler) removeMember(
	ctx context.Context,
	tx *store.Tx,
	rec *store.AuditRecorder,
	groupID, userID string,
	at time.Time,
) error {
	n, err := tx.DeleteUserGroupMember(ctx, db.DeleteUserGroupMemberParams{
		GroupID: groupID,
		UserID:  userID,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return nil
	}
	rec.Effect(store.AuditEffect{
		ResourceType: "user_group_member",
		ResourceID:   groupID,
		Action:       "LEAVE",
		Outcome:      store.EffectApplied,
		BeforeRef:    &userID,
	})
	rec.RefreshSearch("user_group", groupID)
	return h.invalidateSubjectSessions(ctx, tx, rec, userID, at)
}

func (h *Handler) invalidateSubjectSessions(
	ctx context.Context,
	tx *store.Tx,
	rec *store.AuditRecorder,
	userID string,
	at time.Time,
) error {
	version, err := tx.BumpUserSessionVersion(ctx, db.BumpUserSessionVersionParams{ID: userID, UpdatedAt: &at})
	if err != nil {
		if store.IsNotFound(err) {
			return nil
		}
		return err
	}
	after := int64(version)
	rec.Effect(store.AuditEffect{
		ResourceType:  "user",
		ResourceID:    userID,
		Action:        "INVALIDATE_SESSIONS",
		Outcome:       store.EffectApplied,
		ChangedFields: []string{"session_version"},
		AfterCount:    &after,
	})
	return nil
}

func (h *Handler) renameGroup(
	ctx context.Context,
	tx *store.Tx,
	rec *store.AuditRecorder,
	s *session,
	mapping store.SCIMGroupMappingRow,
	group store.UserGroupRow,
	name string,
	at time.Time,
) error {
	if name == mapping.ScimDisplayName && name == group.Name {
		return nil
	}
	if _, err := tx.UpdateSCIMGroupMappingDisplayName(ctx, db.UpdateSCIMGroupMappingDisplayNameParams{
		ProviderID:      s.provider.ID,
		ScimGroupID:     mapping.ScimGroupID,
		ScimDisplayName: name,
	}); err != nil {
		return err
	}
	if _, err := tx.UpdateUserGroupName(ctx, db.UpdateUserGroupNameParams{
		ID:        mapping.UserGroupID,
		Name:      name,
		UpdatedAt: at,
	}); err != nil {
		return err
	}
	rec.Effect(store.AuditEffect{
		ResourceType:  "scim_group_mapping",
		ResourceID:    mapping.ID,
		Action:        "RENAME",
		Outcome:       store.EffectApplied,
		ChangedFields: []string{"scim_display_name"},
		BeforeRef:     &mapping.UserGroupID,
		AfterRef:      &s.provider.ID,
	})
	rec.Effect(store.AuditEffect{
		ResourceType:  "user_group",
		ResourceID:    mapping.UserGroupID,
		Action:        "RENAME",
		Outcome:       store.EffectApplied,
		ChangedFields: []string{"name"},
		AfterRef:      &s.provider.ID,
	})
	return nil
}

func (h *Handler) writeGroup(ctx context.Context, w http.ResponseWriter, mapping store.SCIMGroupMappingRow, baseURL string, status int) {
	resource, err := h.groupResource(ctx, mapping, baseURL)
	if err != nil {
		if store.IsNotFound(err) {
			writeError(w, http.StatusNotFound, "group not found")
			return
		}
		h.logger.Error("scim: failed to build the group resource", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to read group")
		return
	}
	writeJSON(w, status, resource)
}

func patchMemberIDs(value json.RawMessage) []string {
	var out []string
	var items []json.RawMessage
	if json.Unmarshal(value, &items) == nil {
		for _, item := range items {
			var member SCIMMember
			if json.Unmarshal(item, &member) == nil && member.Value != "" {
				out = append(out, member.Value)
			}
		}
		return out
	}
	var member SCIMMember
	if json.Unmarshal(value, &member) == nil && member.Value != "" {
		out = append(out, member.Value)
	}
	return out
}

func memberIDFromFilter(path string) string {
	lower := strings.ToLower(path)
	if !strings.HasPrefix(lower, "members[") {
		return ""
	}
	inner := strings.TrimSuffix(path[len("members["):], "]")
	idx := strings.Index(strings.ToLower(inner), " eq ")
	if idx < 0 {
		return ""
	}
	return strings.Trim(strings.TrimSpace(inner[idx+len(" eq "):]), `"`)
}

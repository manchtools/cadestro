package scim

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/manchtools/cadestro/server/internal/crypto"
	"github.com/manchtools/cadestro/server/internal/idp"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

const actorSCIM = "scim"

var errNoChange = errors.New("scim: the assertion changed nothing")

func noChangeIfNothingRecorded(rec *store.AuditRecorder) error {
	if rec.Len() == 0 {
		return errNoChange
	}
	return nil
}

type subjectAssertion struct {
	Email       *string
	Active      *bool
	DisplayName *string
	GivenName   *string
	FamilyName  *string
}

func (a subjectAssertion) assertsProfile() bool {
	return a.DisplayName != nil || a.GivenName != nil || a.FamilyName != nil
}

func assertionFromResource(u SCIMUser) subjectAssertion {
	a := subjectAssertion{Active: u.Active}
	if email := resourceEmail(u); email != "" {
		normalized := normalizeEmail(email)
		a.Email = &normalized
	}
	if u.Name != nil {
		display := formatExternalName(u.Name)
		a.DisplayName = &display
		a.GivenName = &u.Name.GivenName
		a.FamilyName = &u.Name.FamilyName
	}
	return a
}

func resourceEmail(u SCIMUser) string {
	if u.UserName != "" {
		return u.UserName
	}
	if len(u.Emails) > 0 {
		return u.Emails[0].Value
	}
	return ""
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request, s *session) {
	ctx := r.Context()
	var resource SCIMUser
	if !decodeBody(w, r, &resource) {
		return
	}

	email := normalizeEmail(resourceEmail(resource))
	if email == "" {
		writeError(w, http.StatusBadRequest, "userName or emails[0].value is required")
		return
	}
	baseURL := baseURLFromRequest(r, s.provider.Slug)
	assertion := assertionFromResource(resource)

	if resource.ExternalID != "" {
		existing, err := h.store.FindSCIMUserByExternalID(ctx, s.provider.ID, resource.ExternalID)
		switch {
		case err == nil:
			h.syncSubject(w, r, s, existing.User, resource.ExternalID, assertion, baseURL, http.StatusOK)
			return
		case !store.IsNotFound(err):
			h.logger.Error("scim: failed to resolve external identifier", "error", err)
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
	}

	if s.provider.AutoLinkByEmail {
		bound, err := h.store.FindSCIMUserByEmail(ctx, s.provider.ID, email)
		switch {
		case err == nil:
			h.syncSubject(w, r, s, bound.User, resource.ExternalID, assertion, baseURL, http.StatusOK)
			return
		case !store.IsNotFound(err):
			h.logger.Error("scim: failed to resolve address", "error", err)
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}

		existing, err := h.store.GetUserByEmail(ctx, email)
		switch {
		case err == nil:
			if !h.mayBindByAddress(ctx, w, s, existing.ID) {
				return
			}
			h.bindExistingSubject(w, r, s, existing, resource, baseURL)
			return
		case !store.IsNotFound(err):
			h.logger.Error("scim: failed to look up subject by address", "error", err)
			writeError(w, http.StatusInternalServerError, "internal server error")
			return
		}
	}

	h.provisionSubject(w, r, s, resource, email, baseURL)
}

func (h *Handler) mayBindByAddress(ctx context.Context, w http.ResponseWriter, s *session, userID string) bool {
	if s.provider.TrustEmailAssertions {
		return true
	}
	bound, err := h.store.CountIdentityLinksForUser(ctx, userID)
	if err != nil {
		h.logger.Error("scim: failed to count existing bindings", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return false
	}
	if bound > 0 {
		h.logger.Warn("scim: refusing to bind an already-bound account by asserted address")
		writeError(w, http.StatusConflict, "the address already belongs to a bound account; cannot auto-link")
		return false
	}
	return true
}

func (h *Handler) bindExistingSubject(w http.ResponseWriter, r *http.Request, s *session, existing store.UserRow, resource SCIMUser, baseURL string) {
	ctx := r.Context()
	linkID := ulid.Make().String()
	at := h.now().UTC()

	_, err := h.store.WithAudit(ctx, h.mutationOp(s), func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		return h.insertBinding(ctx, tx, rec, s, bindingSpec{
			LinkID:      linkID,
			UserID:      existing.ID,
			ExternalID:  resource.ExternalID,
			Email:       existing.Email,
			DisplayName: formatExternalName(resource.Name),
			At:          at,
		})
	})
	if err != nil {
		if store.IsConflict(err) {
			writeError(w, http.StatusConflict, "the external identifier is already bound")
			return
		}
		h.logger.Error("scim: failed to bind existing subject", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to link user")
		return
	}
	writeJSON(w, http.StatusCreated, userResource(existing, resource.ExternalID, baseURL))
}

func (h *Handler) provisionSubject(w http.ResponseWriter, r *http.Request, s *session, resource SCIMUser, email, baseURL string) {
	ctx := r.Context()
	userID := ulid.Make().String()
	linkID := ulid.Make().String()
	at := h.now().UTC()

	var created store.UserRow
	_, err := h.store.WithAudit(ctx, h.mutationOp(s), func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		wrapped, err := h.mintSubjectDEK(ctx, tx, userID)
		if err != nil {
			return err
		}
		sealed, err := h.sealForSubject(userID, wrapped, "email", email)
		if err != nil {
			return err
		}

		linuxUID, err := tx.GetNextLinuxUID(ctx)
		if err != nil {
			return err
		}
		if linuxUID > 2147483647 {
			return errors.New("assign linux uid: int32 range exhausted")
		}
		linuxUsername := idp.DeriveLinuxUsername(email, resource.UserName)
		if linuxUsername == "" {
			linuxUsername = "user_" + strings.ToLower(userID[:8])
		}

		created, err = tx.InsertUser(ctx, db.InsertUserParams{
			ID:                 userID,
			Email:              email,
			DisplayName:        formatExternalName(resource.Name),
			GivenName:          nameField(resource.Name, func(n *SCIMName) string { return n.GivenName }),
			FamilyName:         nameField(resource.Name, func(n *SCIMName) string { return n.FamilyName }),
			LinuxUsername:      linuxUsername,
			LinuxUid:           int32(linuxUID),
			ProvisioningSource: store.UserProvisioningSourceSCIM,
			CreatedAt:          &at,
		})
		if err != nil {
			return err
		}
		rec.Effect(store.AuditEffect{
			ResourceType:        "user",
			ResourceID:          userID,
			Action:              "CREATE",
			Outcome:             store.EffectApplied,
			ChangedFields:       []string{"email", "display_name", "linux_username", "linux_uid"},
			AfterRef:            &s.provider.ID,
			EvidenceKind:        "email_sha256",
			EvidenceFingerprint: fingerprint(email),
			SealedDetail:        sealed,
			SealedDetailSubject: userID,
		})

		if s.provider.DefaultRoleID != "" {
			grantID := ulid.Make().String()
			if _, err := tx.InsertUserRoleGrant(ctx, db.InsertUserRoleGrantParams{
				GrantID:    grantID,
				UserID:     userID,
				RoleID:     s.provider.DefaultRoleID,
				AssignedAt: at,
				AssignedBy: actorSCIM,
			}); err != nil {
				return err
			}
			rec.Effect(store.AuditEffect{
				ResourceType: "user_role",
				ResourceID:   grantID,
				Action:       "GRANT",
				Outcome:      store.EffectApplied,
				BeforeRef:    &userID,
				AfterRef:     &s.provider.DefaultRoleID,
			})
		}

		if err := h.applyDeploymentDefaults(ctx, tx, rec, userID, at); err != nil {
			return err
		}

		if !resource.IsActive() {
			if _, err := tx.SetUserDisabled(ctx, db.SetUserDisabledParams{
				ID: userID, Disabled: true, UpdatedAt: &at,
			}); err != nil {
				return err
			}
			no, yes := false, true
			rec.Effect(store.AuditEffect{
				ResourceType:  "user",
				ResourceID:    userID,
				Action:        "SET_DISABLED",
				Outcome:       store.EffectApplied,
				ChangedFields: []string{"disabled", "session_version"},
				BeforeFlag:    &no,
				AfterFlag:     &yes,
			})
			created.Disabled = true
		}

		return h.insertBinding(ctx, tx, rec, s, bindingSpec{
			LinkID:      linkID,
			UserID:      userID,
			ExternalID:  resource.ExternalID,
			Email:       email,
			DisplayName: formatExternalName(resource.Name),
			At:          at,
		})
	})
	if err != nil {
		if store.IsConflict(err) {
			writeError(w, http.StatusConflict, "a user with that address already exists")
			return
		}
		h.logger.Error("scim: failed to provision subject", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	writeJSON(w, http.StatusCreated, userResource(created, resource.ExternalID, baseURL))
}

func (h *Handler) applyDeploymentDefaults(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder, userID string, at time.Time) error {
	settings, err := tx.GetServerSettings(ctx)
	if err != nil {
		return err
	}
	yes := true
	if settings.UserProvisioningEnabled {
		if _, err := tx.SetUserProvisioningEnabled(ctx, db.SetUserProvisioningEnabledParams{
			ID:                      userID,
			UserProvisioningEnabled: true,
			UpdatedAt:               &at,
		}); err != nil {
			return err
		}
		rec.Effect(store.AuditEffect{
			ResourceType:  "user",
			ResourceID:    userID,
			Action:        "SET_PROVISIONING",
			Outcome:       store.EffectApplied,
			ChangedFields: []string{"user_provisioning_enabled"},
			AfterFlag:     &yes,
		})
	}
	if settings.SshAccessForAll {
		if _, err := tx.UpdateUserSshSettings(ctx, db.UpdateUserSshSettingsParams{
			ID:               userID,
			SshAccessEnabled: true,
			SshAllowPubkey:   true,
			SshAllowPassword: false,
			UpdatedAt:        &at,
		}); err != nil {
			return err
		}
		rec.Effect(store.AuditEffect{
			ResourceType:  "user",
			ResourceID:    userID,
			Action:        "SET_SSH_SETTINGS",
			Outcome:       store.EffectApplied,
			ChangedFields: []string{"ssh_access_enabled", "ssh_allow_pubkey", "ssh_allow_password"},
			AfterFlag:     &yes,
		})
	}
	return nil
}

type bindingSpec struct {
	LinkID      string
	UserID      string
	ExternalID  string
	Email       string
	DisplayName string
	At          time.Time
}

func (h *Handler) insertBinding(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder, s *session, spec bindingSpec) error {
	if _, err := tx.InsertIdentityLink(ctx, db.InsertIdentityLinkParams{
		ID:            spec.LinkID,
		UserID:        spec.UserID,
		ProviderID:    s.provider.ID,
		ExternalID:    spec.ExternalID,
		ExternalEmail: spec.Email,
		ExternalName:  spec.DisplayName,
		LinkedAt:      spec.At,
	}); err != nil {
		return err
	}
	rec.Effect(store.AuditEffect{
		ResourceType:        "identity_link",
		ResourceID:          spec.LinkID,
		Action:              "LINK",
		Outcome:             store.EffectApplied,
		BeforeRef:           &s.provider.ID,
		AfterRef:            &spec.UserID,
		EvidenceKind:        "external_subject_sha256",
		EvidenceFingerprint: fingerprint(spec.ExternalID),
	})
	return nil
}

func (h *Handler) replaceUser(w http.ResponseWriter, r *http.Request, s *session) {
	ctx := r.Context()
	_, before, ok := h.resolveSubject(ctx, w, s, r.PathValue("id"))
	if !ok {
		return
	}
	var resource SCIMUser
	if !decodeBody(w, r, &resource) {
		return
	}
	h.syncSubject(w, r, s, before, resource.ExternalID, assertionFromResource(resource),
		baseURLFromRequest(r, s.provider.Slug), http.StatusOK)
}

func (h *Handler) patchUser(w http.ResponseWriter, r *http.Request, s *session) {
	ctx := r.Context()
	_, before, ok := h.resolveSubject(ctx, w, s, r.PathValue("id"))
	if !ok {
		return
	}
	var patch SCIMPatchRequest
	if !decodeBody(w, r, &patch) {
		return
	}

	var assertion subjectAssertion
	for _, op := range patch.Operations {

		if !op.Op.IsValid() {
			writeError(w, http.StatusBadRequest, "unsupported patch op")
			return
		}
		if op.Op.Normalize() != SCIMPatchOpReplace {
			writeError(w, http.StatusBadRequest, "only the replace op is supported on a user")
			return
		}
		if err := applyUserPatchOp(&assertion, op); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	h.syncSubject(w, r, s, before, "", assertion, baseURLFromRequest(r, s.provider.Slug), http.StatusOK)
}

func (h *Handler) syncSubject(
	w http.ResponseWriter,
	r *http.Request,
	s *session,
	before store.UserRow,
	externalID string,
	assertion subjectAssertion,
	baseURL string,
	okStatus int,
) {
	ctx := r.Context()
	at := h.now().UTC()

	_, err := h.store.WithAudit(ctx, h.mutationOp(s), func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		if err := h.applyAssertion(ctx, tx, rec, before, assertion, at); err != nil {
			return err
		}
		if err := h.refreshBinding(ctx, tx, rec, s, before.ID, externalID, assertion); err != nil {
			return err
		}
		return noChangeIfNothingRecorded(rec)
	})
	if err != nil && !errors.Is(err, errNoChange) {
		if store.IsConflict(err) {
			writeError(w, http.StatusConflict, "a user with that address already exists")
			return
		}
		if store.IsNotFound(err) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		h.logger.Error("scim: failed to apply the directory assertion", "route", s.descriptor, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update user")
		return
	}

	after, err := h.store.GetUser(ctx, before.ID)
	if err != nil {
		h.logger.Error("scim: failed to read back the subject", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to read user")
		return
	}
	link, err := h.store.GetIdentityLinkByProviderAndUser(ctx, s.provider.ID, before.ID)
	if err != nil {
		h.logger.Error("scim: failed to read back the binding", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to read user")
		return
	}
	writeJSON(w, okStatus, userResource(after, link.ExternalID, baseURL))
}

func (h *Handler) applyAssertion(
	ctx context.Context,
	tx *store.Tx,
	rec *store.AuditRecorder,
	before store.UserRow,
	a subjectAssertion,
	at time.Time,
) error {
	if a.Email != nil && *a.Email != "" && *a.Email != before.Email {
		n, err := tx.UpdateUserEmail(ctx, db.UpdateUserEmailParams{
			ID: before.ID, Email: *a.Email, UpdatedAt: &at,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return store.ErrNotFound
		}

		sealed, err := h.sealTransition(ctx, tx, before.ID, "email", before.Email, *a.Email)
		if err != nil {
			return err
		}
		rec.Effect(store.AuditEffect{
			ResourceType:        "user",
			ResourceID:          before.ID,
			Action:              "UPDATE_EMAIL",
			Outcome:             store.EffectApplied,
			ChangedFields:       []string{"email"},
			EvidenceKind:        "email_sha256",
			EvidenceFingerprint: fingerprint(*a.Email),
			SealedDetail:        sealed,
			SealedDetailSubject: before.ID,
		})
	}

	if a.Active != nil {
		disabled := !*a.Active
		if disabled != before.Disabled {

			if _, err := tx.SetUserDisabled(ctx, db.SetUserDisabledParams{
				ID: before.ID, Disabled: disabled, UpdatedAt: &at,
			}); err != nil {
				return err
			}
			wasDisabled := before.Disabled
			rec.Effect(store.AuditEffect{
				ResourceType:  "user",
				ResourceID:    before.ID,
				Action:        "SET_DISABLED",
				Outcome:       store.EffectApplied,
				ChangedFields: []string{"disabled", "session_version"},
				BeforeFlag:    &wasDisabled,
				AfterFlag:     &disabled,
			})
		}
	}

	if a.assertsProfile() {

		if _, err := tx.UpdateUserProfile(ctx, db.UpdateUserProfileParams{
			ID:                before.ID,
			DisplayName:       valueOr(a.DisplayName, before.DisplayName),
			GivenName:         valueOr(a.GivenName, before.GivenName),
			FamilyName:        valueOr(a.FamilyName, before.FamilyName),
			PreferredUsername: before.PreferredUsername,
			Picture:           before.Picture,
			Locale:            before.Locale,
			UpdatedAt:         &at,
		}); err != nil {
			return err
		}
		rec.Effect(store.AuditEffect{
			ResourceType:  "user",
			ResourceID:    before.ID,
			Action:        "UPDATE_PROFILE",
			Outcome:       store.EffectApplied,
			ChangedFields: []string{"display_name", "given_name", "family_name"},
		})
	}
	return nil
}

func (h *Handler) refreshBinding(
	ctx context.Context,
	tx *store.Tx,
	rec *store.AuditRecorder,
	s *session,
	userID, externalID string,
	a subjectAssertion,
) error {
	link, err := tx.GetIdentityLinkByProviderAndUser(ctx, db.GetIdentityLinkByProviderAndUserParams{
		ProviderID: s.provider.ID,
		UserID:     userID,
	})
	if err != nil {
		return err
	}
	if externalID == "" {
		externalID = link.ExternalID
	}
	email := link.ExternalEmail
	if a.Email != nil && *a.Email != "" {
		email = *a.Email
	}
	name := link.ExternalName
	if a.assertsProfile() {
		name = valueOr(a.DisplayName, "")
	}

	if externalID == link.ExternalID && email == link.ExternalEmail && name == link.ExternalName {
		return nil
	}

	updated, err := tx.UpdateIdentityLinkExternalIdentity(ctx, db.UpdateIdentityLinkExternalIdentityParams{
		ID:            link.ID,
		ExternalID:    externalID,
		ExternalEmail: email,
		ExternalName:  name,
	})
	if err != nil {
		return err
	}
	rec.Effect(store.AuditEffect{
		ResourceType:        "identity_link",
		ResourceID:          updated.ID,
		Action:              "SYNC_LINK",
		Outcome:             store.EffectApplied,
		ChangedFields:       []string{"external_id", "external_email", "external_name"},
		BeforeRef:           &s.provider.ID,
		AfterRef:            &userID,
		EvidenceKind:        "external_subject_sha256",
		EvidenceFingerprint: fingerprint(updated.ExternalID),
	})
	return nil
}

func (h *Handler) deleteUser(w http.ResponseWriter, r *http.Request, s *session) {
	ctx := r.Context()
	link, before, ok := h.resolveSubject(ctx, w, s, r.PathValue("id"))
	if !ok {
		return
	}

	_, err := h.store.WithAudit(ctx, h.mutationOp(s), func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		removed, err := tx.DeleteIdentityLink(ctx, link.ID)
		if err != nil {
			return err
		}
		rec.Effect(store.AuditEffect{
			ResourceType:        "identity_link",
			ResourceID:          removed.ID,
			Action:              "UNLINK",
			Outcome:             store.EffectApplied,
			BeforeRef:           &removed.UserID,
			AfterRef:            &removed.ProviderID,
			EvidenceKind:        "external_subject_sha256",
			EvidenceFingerprint: fingerprint(removed.ExternalID),
		})

		remaining, err := tx.CountIdentityLinksForUser(ctx, before.ID)
		if err != nil {
			return err
		}
		if remaining > 0 || before.ProvisioningSource != store.UserProvisioningSourceSCIM {
			return nil
		}
		return store.EraseUser(ctx, tx, rec, before)
	})
	if err != nil {
		h.logger.Error("scim: failed to remove the subject binding", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) mintSubjectDEK(ctx context.Context, tx *store.Tx, subjectID string) (string, error) {
	wrapped, err := crypto.GenerateWrappedDEK(h.kek, subjectID)
	if err != nil {
		return "", err
	}
	if _, err := tx.InsertUserEncryptionKey(ctx, db.InsertUserEncryptionKeyParams{
		UserID:     subjectID,
		WrappedDek: wrapped,
	}); err != nil {
		return "", err
	}
	row, err := tx.GetUserEncryptionKey(ctx, subjectID)
	if err != nil {
		return "", err
	}
	return row.WrappedDek, nil
}

func (h *Handler) sealForSubject(subjectID, wrappedDEK, field, value string) ([]byte, error) {
	dek, err := crypto.UnwrapDEK(h.kek, subjectID, wrappedDEK)
	if err != nil {
		return nil, err
	}
	sealed, err := dek.SealField(value, field)
	if err != nil {
		return nil, err
	}
	return []byte(sealed), nil
}

func (h *Handler) sealTransition(ctx context.Context, tx *store.Tx, subjectID, field, before, after string) ([]byte, error) {
	key, err := tx.GetUserEncryptionKey(ctx, subjectID)
	if err != nil {
		return nil, err
	}
	return h.sealForSubject(subjectID, key.WrappedDek, field, before+" -> "+after)
}

func decodeBody(w http.ResponseWriter, r *http.Request, into any) bool {
	limitBody(w, r)
	if err := json.NewDecoder(r.Body).Decode(into); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

func valueOr(v *string, fallback string) string {
	if v == nil {
		return fallback
	}
	return *v
}

func nameField(name *SCIMName, pick func(*SCIMName) string) string {
	if name == nil {
		return ""
	}
	return pick(name)
}

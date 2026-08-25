package identity

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"

	"connectrpc.com/connect"
	"github.com/oklog/ulid/v2"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/crypto"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

const scimTokenBytes = 32

func (h *Handlers) CreateIdentityProvider(ctx context.Context, req *connect.Request[cadestrov1.CreateIdentityProviderRequest]) (*connect.Response[cadestrov1.CreateIdentityProviderResponse], error) {
	actor, err := h.requireActor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, PermCreateIdentityProvider, ""); err != nil {
		return nil, err
	}
	if err := validateProviderClientID(ctx, req.Msg.GetClientId().GetValue()); err != nil {
		return nil, err
	}
	providerType, ok := providerTypeFromProto(req.Msg.ProviderType)
	if !ok {
		return nil, rpcError(ctx, ErrValidationFailed, connect.CodeInvalidArgument, "unsupported provider_type")
	}
	if req.Msg.GetDefaultRoleId().GetValue() != "" {
		if _, err := h.store.GetRole(ctx, req.Msg.GetDefaultRoleId().GetValue()); err != nil {
			if store.IsNotFound(err) {
				return nil, notFound(ctx, ErrRoleNotFound, "default role not found")
			}
			return nil, internalError(ctx, "failed to resolve default role")
		}
	}
	mapping, err := encodeGroupMapping(ctx, req.Msg.GroupMapping)
	if err != nil {
		return nil, err
	}

	scopes := nonNilStrings(req.Msg.Scopes)

	providerID := ulid.Make().String()
	sealed, err := h.kek.EncryptWithContext(req.Msg.ClientSecret, crypto.RowAAD(providerID, crypto.PurposeIdPClientSecret))
	if err != nil {
		return nil, internalError(ctx, "failed to protect the client secret")
	}

	at := h.now().UTC()
	var created store.IdentityProviderRow
	_, err = h.store.WithAudit(ctx, h.mutationOp(req, actor, PermCreateIdentityProvider),
		func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
			var err error
			created, err = tx.InsertIdentityProvider(ctx, db.InsertIdentityProviderParams{
				ID:                    providerID,
				Name:                  req.Msg.Name,
				Slug:                  req.Msg.Slug,
				ProviderType:          providerType,
				Enabled:               true,
				ClientID:              req.Msg.GetClientId().GetValue(),
				ClientSecretEncrypted: sealed,
				IssuerUrl:             req.Msg.IssuerUrl,
				AuthorizationUrl:      req.Msg.AuthorizationUrl,
				TokenUrl:              req.Msg.TokenUrl,
				UserinfoUrl:           req.Msg.UserinfoUrl,
				Scopes:                scopes,
				AutoCreateUsers:       req.Msg.AutoCreateUsers,
				AutoLinkByEmail:       req.Msg.AutoLinkByEmail,
				TrustEmailAssertions:  req.Msg.TrustEmailAssertions,
				DefaultRoleID:         req.Msg.GetDefaultRoleId().GetValue(),
				GroupClaim:            req.Msg.GroupClaim,
				GroupMapping:          mapping,
				CreatedAt:             at,
				CreatedBy:             actor.ID,
			})
			if err != nil {
				return err
			}
			trust := req.Msg.TrustEmailAssertions
			effect := store.AuditEffect{
				ResourceType: "identity_provider",
				ResourceID:   providerID,
				Action:       "CREATE",
				Outcome:      store.EffectApplied,
				ChangedFields: []string{
					"name", "slug", "client_id", "issuer_url",
					"auto_create_users", "auto_link_by_email", "trust_email_assertions",
				},

				AfterFlag: &trust,
			}
			if req.Msg.ClientSecret != "" {
				effect.ChangedFields = append(effect.ChangedFields, "client_secret_encrypted")
				effect.EvidenceKind = "idp_client_secret_sha256"
				effect.EvidenceFingerprint = fingerprint(req.Msg.ClientSecret)
			}
			rec.Effect(effect)
			return nil
		})
	if err != nil {
		if store.IsConflict(err) {
			return nil, rpcError(ctx, ErrProviderSlugExists, connect.CodeAlreadyExists, "a provider with that slug already exists")
		}
		h.logger.Error("failed to create identity provider", "error", err)
		return nil, internalError(ctx, "failed to create identity provider")
	}
	return connect.NewResponse(&cadestrov1.CreateIdentityProviderResponse{Provider: h.providerToProto(created)}), nil
}

func (h *Handlers) GetIdentityProvider(ctx context.Context, req *connect.Request[cadestrov1.GetIdentityProviderRequest]) (*connect.Response[cadestrov1.GetIdentityProviderResponse], error) {
	if _, err := h.requireActor(ctx); err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, PermGetIdentityProvider, req.Msg.GetId().GetValue()); err != nil {
		return nil, err
	}
	row, err := h.store.GetIdentityProvider(ctx, req.Msg.GetId().GetValue())
	if err != nil {
		if store.IsNotFound(err) {
			return nil, notFound(ctx, ErrProviderNotFound, "identity provider not found")
		}
		return nil, internalError(ctx, "failed to load identity provider")
	}
	return connect.NewResponse(&cadestrov1.GetIdentityProviderResponse{Provider: h.providerToProto(row)}), nil
}

func (h *Handlers) ListIdentityProviders(ctx context.Context, req *connect.Request[cadestrov1.ListIdentityProvidersRequest]) (*connect.Response[cadestrov1.ListIdentityProvidersResponse], error) {
	if _, err := h.requireActor(ctx); err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, PermListIdentityProviders, ""); err != nil {
		return nil, err
	}
	limit := pageLimit(req.Msg.PageSize)
	rows, err := h.store.ListIdentityProviders(ctx, req.Msg.PageToken, limit)
	if err != nil {
		return nil, internalError(ctx, "failed to list identity providers")
	}
	total, err := h.store.CountIdentityProviders(ctx)
	if err != nil {
		return nil, internalError(ctx, "failed to count identity providers")
	}
	resp := &cadestrov1.ListIdentityProvidersResponse{TotalCount: int32(total)}
	for _, r := range rows {
		resp.Providers = append(resp.Providers, h.providerToProto(r))
	}
	if len(rows) == int(limit) {
		resp.NextPageToken = rows[len(rows)-1].ID
	}
	return connect.NewResponse(resp), nil
}

func (h *Handlers) UpdateIdentityProvider(ctx context.Context, req *connect.Request[cadestrov1.UpdateIdentityProviderRequest]) (*connect.Response[cadestrov1.UpdateIdentityProviderResponse], error) {
	actor, err := h.requireActor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, PermUpdateIdentityProvider, req.Msg.GetId().GetValue()); err != nil {
		return nil, err
	}
	before, err := h.store.GetIdentityProvider(ctx, req.Msg.GetId().GetValue())
	if err != nil {
		if store.IsNotFound(err) {
			return nil, notFound(ctx, ErrProviderNotFound, "identity provider not found")
		}
		return nil, internalError(ctx, "failed to load identity provider")
	}
	if err := validateProviderClientID(ctx, req.Msg.GetClientId().GetValue()); err != nil {
		return nil, err
	}
	if req.Msg.GetDefaultRoleId().GetValue() != "" {
		if _, err := h.store.GetRole(ctx, req.Msg.GetDefaultRoleId().GetValue()); err != nil {
			if store.IsNotFound(err) {
				return nil, notFound(ctx, ErrRoleNotFound, "default role not found")
			}
			return nil, internalError(ctx, "failed to resolve default role")
		}
	}
	mapping, err := encodeGroupMapping(ctx, req.Msg.GroupMapping)
	if err != nil {
		return nil, err
	}

	secret := before.ClientSecretEncrypted
	secretProvided := req.Msg.ClientSecret != ""
	if secretProvided {
		secret, err = h.kek.EncryptWithContext(req.Msg.ClientSecret, crypto.RowAAD(before.ID, crypto.PurposeIdPClientSecret))
		if err != nil {
			return nil, internalError(ctx, "failed to protect the client secret")
		}
	}

	at := h.now().UTC()
	var updated store.IdentityProviderRow
	_, err = h.store.WithAudit(ctx, h.mutationOp(req, actor, PermUpdateIdentityProvider),
		func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
			var err error
			updated, err = tx.UpdateIdentityProvider(ctx, db.UpdateIdentityProviderParams{
				ID:                    before.ID,
				Name:                  req.Msg.Name,
				Enabled:               req.Msg.Enabled,
				ClientID:              req.Msg.GetClientId().GetValue(),
				ClientSecretEncrypted: secret,
				IssuerUrl:             req.Msg.IssuerUrl,
				AuthorizationUrl:      req.Msg.AuthorizationUrl,
				TokenUrl:              req.Msg.TokenUrl,
				UserinfoUrl:           req.Msg.UserinfoUrl,
				Scopes:                nonNilStrings(req.Msg.Scopes),
				AutoCreateUsers:       req.Msg.AutoCreateUsers,
				AutoLinkByEmail:       req.Msg.AutoLinkByEmail,
				TrustEmailAssertions:  req.Msg.TrustEmailAssertions,
				DefaultRoleID:         req.Msg.GetDefaultRoleId().GetValue(),
				GroupClaim:            req.Msg.GroupClaim,
				GroupMapping:          mapping,
				UpdatedAt:             at,
			})
			if err != nil {
				return err
			}
			changed := []string{"name", "enabled", "client_id", "issuer_url", "auto_create_users", "auto_link_by_email", "trust_email_assertions"}
			effect := store.AuditEffect{
				ResourceType:  "identity_provider",
				ResourceID:    before.ID,
				Action:        "UPDATE",
				Outcome:       store.EffectApplied,
				ChangedFields: changed,
				BeforeFlag:    &before.TrustEmailAssertions,
				AfterFlag:     &req.Msg.TrustEmailAssertions,
			}
			if secretProvided {
				effect.ChangedFields = append(changed, "client_secret_encrypted")
				effect.EvidenceKind = "idp_client_secret_sha256"
				effect.EvidenceFingerprint = fingerprint(req.Msg.ClientSecret)
			}
			rec.Effect(effect)
			return nil
		})
	if err != nil {
		if store.IsNotFound(err) {
			return nil, notFound(ctx, ErrProviderNotFound, "identity provider not found")
		}
		return nil, internalError(ctx, "failed to update identity provider")
	}
	return connect.NewResponse(&cadestrov1.UpdateIdentityProviderResponse{Provider: h.providerToProto(updated)}), nil
}

func validateProviderClientID(ctx context.Context, clientID string) error {
	if clientID == "" {
		return rpcError(ctx, ErrValidationFailed, connect.CodeInvalidArgument,
			"client_id is required")
	}
	return nil
}

func (h *Handlers) DeleteIdentityProvider(ctx context.Context, req *connect.Request[cadestrov1.DeleteIdentityProviderRequest]) (*connect.Response[cadestrov1.DeleteIdentityProviderResponse], error) {
	actor, err := h.requireActor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, PermDeleteIdentityProvider, req.Msg.GetId().GetValue()); err != nil {
		return nil, err
	}
	before, err := h.store.GetIdentityProvider(ctx, req.Msg.GetId().GetValue())
	if err != nil {
		if store.IsNotFound(err) {
			return nil, notFound(ctx, ErrProviderNotFound, "identity provider not found")
		}
		return nil, internalError(ctx, "failed to load identity provider")
	}

	at := h.now().UTC()
	_, err = h.store.WithAudit(ctx, h.mutationOp(req, actor, PermDeleteIdentityProvider),
		func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
			n, err := tx.SoftDeleteIdentityProvider(ctx, db.SoftDeleteIdentityProviderParams{ID: before.ID, UpdatedAt: at})
			if err != nil {
				return err
			}
			if n == 0 {
				return store.ErrNotFound
			}
			yes := true
			rec.Effect(store.AuditEffect{
				ResourceType:  "identity_provider",
				ResourceID:    before.ID,
				Action:        "DELETE",
				Outcome:       store.EffectApplied,
				ChangedFields: []string{"is_deleted"},
				AfterFlag:     &yes,
			})
			return nil
		})
	if err != nil {
		if store.IsNotFound(err) {
			return nil, notFound(ctx, ErrProviderNotFound, "identity provider not found")
		}
		return nil, internalError(ctx, "failed to delete identity provider")
	}
	return connect.NewResponse(&cadestrov1.DeleteIdentityProviderResponse{}), nil
}

func (h *Handlers) EnableSCIM(ctx context.Context, req *connect.Request[cadestrov1.EnableSCIMRequest]) (*connect.Response[cadestrov1.EnableSCIMResponse], error) {
	token, provider, err := h.setSCIM(ctx, req, req.Msg.GetId().GetValue(), PermEnableSCIM, true, "ENABLE_SCIM")
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&cadestrov1.EnableSCIMResponse{
		Token:       token,
		EndpointUrl: h.scimEndpointURL(provider.ID),
	}), nil
}

func (h *Handlers) RotateSCIMToken(ctx context.Context, req *connect.Request[cadestrov1.RotateSCIMTokenRequest]) (*connect.Response[cadestrov1.RotateSCIMTokenResponse], error) {
	provider, err := h.store.GetIdentityProvider(ctx, req.Msg.GetId().GetValue())
	if err == nil && !provider.ScimEnabled {
		return nil, rpcError(ctx, ErrSCIMNotEnabled, connect.CodeFailedPrecondition, "SCIM is not enabled for this provider")
	}
	token, _, err := h.setSCIM(ctx, req, req.Msg.GetId().GetValue(), PermRotateSCIMToken, true, "ROTATE_SCIM_TOKEN")
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&cadestrov1.RotateSCIMTokenResponse{Token: token}), nil
}

func (h *Handlers) DisableSCIM(ctx context.Context, req *connect.Request[cadestrov1.DisableSCIMRequest]) (*connect.Response[cadestrov1.DisableSCIMResponse], error) {
	if _, _, err := h.setSCIM(ctx, req, req.Msg.GetId().GetValue(), PermDisableSCIM, false, "DISABLE_SCIM"); err != nil {
		return nil, err
	}
	return connect.NewResponse(&cadestrov1.DisableSCIMResponse{}), nil
}

func (h *Handlers) setSCIM(
	ctx context.Context,
	req connect.AnyRequest,
	providerID, permission string,
	enable bool,
	action string,
) (string, store.IdentityProviderRow, error) {
	actor, err := h.requireActor(ctx)
	if err != nil {
		return "", store.IdentityProviderRow{}, err
	}
	if err := h.authorize(ctx, permission, providerID); err != nil {
		return "", store.IdentityProviderRow{}, err
	}
	before, err := h.store.GetIdentityProvider(ctx, providerID)
	if err != nil {
		if store.IsNotFound(err) {
			return "", store.IdentityProviderRow{}, notFound(ctx, ErrProviderNotFound, "identity provider not found")
		}
		return "", store.IdentityProviderRow{}, internalError(ctx, "failed to load identity provider")
	}

	var token, tokenHash string
	if enable {
		token, err = newSCIMToken()
		if err != nil {
			return "", store.IdentityProviderRow{}, internalError(ctx, "failed to mint a SCIM token")
		}
		tokenHash = fingerprint(token)
	}

	at := h.now().UTC()
	var updated store.IdentityProviderRow
	_, err = h.store.WithAudit(ctx, h.mutationOp(req, actor, permission),
		func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
			var err error
			updated, err = tx.SetIdentityProviderSCIM(ctx, db.SetIdentityProviderSCIMParams{
				ID:            before.ID,
				ScimEnabled:   enable,
				ScimTokenHash: tokenHash,
				UpdatedAt:     at,
			})
			if err != nil {
				return err
			}
			effect := store.AuditEffect{
				ResourceType:  "identity_provider",
				ResourceID:    before.ID,
				Action:        action,
				Outcome:       store.EffectApplied,
				ChangedFields: []string{"scim_enabled", "scim_token_hash"},
				BeforeFlag:    &before.ScimEnabled,
				AfterFlag:     &enable,
			}
			if tokenHash != "" {

				effect.EvidenceKind = "scim_token_sha256"
				effect.EvidenceFingerprint = tokenHash
			}
			rec.Effect(effect)
			return nil
		})
	if err != nil {
		if store.IsNotFound(err) {
			return "", store.IdentityProviderRow{}, notFound(ctx, ErrProviderNotFound, "identity provider not found")
		}
		return "", store.IdentityProviderRow{}, internalError(ctx, "failed to update SCIM configuration")
	}
	return token, updated, nil
}

func newSCIMToken() (string, error) {
	raw := make([]byte, scimTokenBytes)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

func encodeGroupMapping(ctx context.Context, mapping map[string]string) ([]byte, error) {
	if mapping == nil {
		mapping = map[string]string{}
	}
	raw, err := json.Marshal(mapping)
	if err != nil {
		return nil, rpcError(ctx, ErrValidationFailed, connect.CodeInvalidArgument, "group_mapping is not serialisable")
	}
	return raw, nil
}

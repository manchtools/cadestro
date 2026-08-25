package identity

import (
	"context"
	"errors"
	"strings"
	"time"

	"connectrpc.com/connect"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/crypto"
	"github.com/manchtools/cadestro/server/internal/idp"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

const authStateTTL = 10 * time.Minute

const authFlowBrowser = "browser"

func (h *Handlers) ListAuthMethods(ctx context.Context, req *connect.Request[cadestrov1.ListAuthMethodsRequest]) (*connect.Response[cadestrov1.ListAuthMethodsResponse], error) {
	providers, err := h.store.ListEnabledIdentityProviders(ctx)
	if err != nil {
		return nil, internalError(ctx, "failed to list authentication methods")
	}
	resp := &cadestrov1.ListAuthMethodsResponse{}
	for _, p := range providers {
		resp.Providers = append(resp.Providers, &cadestrov1.AuthMethodProvider{
			Slug:         p.Slug,
			Name:         p.Name,
			ProviderType: providerTypeToProto(p.ProviderType),
			BrowserLogin: p.ClientID != "",
		})
	}
	return connect.NewResponse(resp), nil
}

func (h *Handlers) GetSSOLoginURL(ctx context.Context, req *connect.Request[cadestrov1.GetSSOLoginURLRequest]) (*connect.Response[cadestrov1.GetSSOLoginURLResponse], error) {
	provider, err := h.store.GetIdentityProviderBySlug(ctx, req.Msg.Slug)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, notFound(ctx, ErrProviderNotFound, "identity provider not found")
		}
		return nil, internalError(ctx, "failed to load identity provider")
	}
	if !provider.Enabled {

		return nil, notFound(ctx, ErrProviderNotFound, "identity provider not found")
	}
	if provider.ClientID == "" {
		return nil, rpcError(ctx, ErrValidationFailed, connect.CodeFailedPrecondition,
			"browser login is not configured for this identity provider")
	}

	oidcClient, err := h.oidcClientFor(ctx, provider, req.Msg.RedirectUrl)
	if err != nil {
		h.logger.Error("failed to build the OIDC client", "provider_id", provider.ID, "error", err)
		return nil, internalError(ctx, "failed to reach the identity provider")
	}

	state, err := idp.GenerateState()
	if err != nil {
		return nil, internalError(ctx, "failed to start the login flow")
	}
	nonce, err := idp.GenerateNonce()
	if err != nil {
		return nil, internalError(ctx, "failed to start the login flow")
	}
	verifier, err := idp.GenerateCodeVerifier()
	if err != nil {
		return nil, internalError(ctx, "failed to start the login flow")
	}

	expires := h.now().UTC().Add(authStateTTL)

	op := store.AuditOperation{
		Class:                store.ClassBackgroundWriter,
		ActorType:            auth.AnonymousActorType,
		Origin:               auth.ControlRPCOrigin,
		RequestDescriptor:    req.Spec().Procedure,
		AuthorizationOutcome: store.AuthorizationNotApplicable,
		Result:               store.ResultSuccess,
	}
	if ip := auth.ClientIP(req); ip != "" {
		op.OriginFingerprint = auth.Fingerprint(ip)
	}
	_, err = h.store.WithAudit(ctx, op, func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		if err := tx.CreateAuthState(ctx, db.CreateAuthStateParams{
			State:        state,
			ProviderID:   provider.ID,
			FlowKind:     authFlowBrowser,
			Nonce:        nonce,
			CodeVerifier: verifier,
			RedirectUri:  req.Msg.RedirectUrl,
			ExpiresAt:    expires,
		}); err != nil {
			return err
		}
		rec.Effect(store.AuditEffect{
			ResourceType: "auth_state",
			ResourceID:   provider.ID,
			Action:       "START_LOGIN",
			Outcome:      store.EffectApplied,

			EvidenceKind:        "auth_state_sha256",
			EvidenceFingerprint: fingerprint(state),
		})
		return nil
	})
	if err != nil {
		return nil, internalError(ctx, "failed to start the login flow")
	}

	return connect.NewResponse(&cadestrov1.GetSSOLoginURLResponse{
		LoginUrl: oidcClient.AuthCodeURL(state, nonce, verifier),
	}), nil
}

func (h *Handlers) SSOCallback(ctx context.Context, req *connect.Request[cadestrov1.SSOCallbackRequest]) (*connect.Response[cadestrov1.SSOCallbackResponse], error) {
	provider, err := h.store.GetIdentityProviderBySlug(ctx, req.Msg.Slug)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, notFound(ctx, ErrProviderNotFound, "identity provider not found")
		}
		return nil, internalError(ctx, "failed to load identity provider")
	}
	if !provider.Enabled {
		return nil, notFound(ctx, ErrProviderNotFound, "identity provider not found")
	}

	state, err := h.consumeAuthState(ctx, req, provider.ID, req.Msg.State, authFlowBrowser)
	if err != nil {
		return nil, err
	}

	oidcClient, err := h.oidcClientFor(ctx, provider, state.RedirectUri)
	if err != nil {
		h.logger.Error("failed to build the OIDC client", "provider_id", provider.ID, "error", err)
		return nil, internalError(ctx, "failed to reach the identity provider")
	}
	token, err := oidcClient.ExchangeCode(ctx, req.Msg.Code, state.CodeVerifier)
	if err != nil {
		h.logger.Warn("SSO code exchange failed", "provider_id", provider.ID)
		return nil, h.rejectSSO(ctx, req, provider.ID, "code exchange failed")
	}
	claims, err := oidcClient.VerifyAndExtractClaims(ctx, token, state.Nonce)
	if err != nil {
		h.logger.Warn("SSO identity token verification failed", "provider_id", provider.ID)
		return nil, h.rejectSSO(ctx, req, provider.ID, "identity token verification failed")
	}

	completed, err := h.completeLogin(ctx, req, provider, claims)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&cadestrov1.SSOCallbackResponse{
		AccessToken: completed.AccessToken, RefreshToken: completed.RefreshToken,
		ExpiresAt: timestampValue(completed.ExpiresAt), User: completed.User,
	}), nil
}

type completedLogin struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	User         *cadestrov1.User
}

func (h *Handlers) completeLogin(ctx context.Context, req connect.AnyRequest, provider store.IdentityProviderRow, claims *idp.UserClaims) (*completedLogin, error) {
	actor := &auth.UserContext{Kind: auth.PrincipalUser}
	op := h.mutationOp(req, actor, "")
	op.ActorType = auth.AnonymousActorType
	op.ActorID = ""
	op.AuthorizationOutcome = store.AuthorizationNotApplicable

	var (
		result   *idp.LinkResult
		linkErr  error
		sessions store.UserSessionStateRow
	)
	_, err := h.store.WithAudit(ctx, op, func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		result, linkErr = h.linker.LinkOrCreate(ctx, tx, rec, provider, claims)
		if linkErr != nil {
			return linkErr
		}
		if provider.GroupClaim != "" {
			mapping := idp.ParseGroupMapping(provider.GroupMapping)
			if err := h.linker.SyncGroupMemberships(ctx, tx, rec, result.UserID, claims.Groups, mapping); err != nil {
				return err
			}
		}
		at := h.now().UTC()
		if _, err := tx.TouchUserLastLogin(ctx, db.TouchUserLastLoginParams{ID: result.UserID, LastLoginAt: &at}); err != nil {
			return err
		}

		rec.RefreshSearch("user", result.UserID)
		state, err := tx.GetUserSessionState(ctx, result.UserID)
		if err != nil {
			return err
		}
		if state.Disabled || state.IsDeleted {

			return errSubjectNotEligible
		}
		sessions = state
		rec.Effect(store.AuditEffect{
			ResourceType: "session",
			ResourceID:   result.UserID,
			Action:       "ISSUE",
			Outcome:      store.EffectApplied,
			AfterRef:     &provider.ID,
		})
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, idp.ErrNoMatchingAccount), errors.Is(err, errSubjectNotEligible):
			return nil, h.rejectSSO(ctx, req, provider.ID, "no matching account")
		default:
			h.logger.Error("SSO callback failed", "provider_id", provider.ID, "error", err)
			return nil, internalError(ctx, "failed to complete the login")
		}
	}

	view, err := h.loadUserView(ctx, result.UserID)
	if err != nil {
		return nil, internalError(ctx, "failed to load the signed-in user")
	}
	tokens, err := h.mintSession(ctx, result.UserID, view.Row.Email, sessions.SessionVersion)
	if err != nil {
		return nil, internalError(ctx, "failed to issue session")
	}
	return &completedLogin{
		AccessToken: tokens.AccessToken, RefreshToken: tokens.RefreshToken,
		ExpiresAt: tokens.ExpiresAt, User: userToProto(view),
	}, nil
}

var errSubjectNotEligible = errors.New("identity: subject is not eligible for a session")

func (h *Handlers) consumeAuthState(ctx context.Context, req connect.AnyRequest, providerID, state, flowKind string) (store.AuthStateRow, error) {
	var consumed store.AuthStateRow
	op := store.AuditOperation{
		Class:                store.ClassBackgroundWriter,
		ActorType:            auth.AnonymousActorType,
		Origin:               auth.ControlRPCOrigin,
		RequestDescriptor:    req.Spec().Procedure,
		AuthorizationOutcome: store.AuthorizationNotApplicable,
		Result:               store.ResultSuccess,
	}
	if ip := auth.ClientIP(req); ip != "" {
		op.OriginFingerprint = auth.Fingerprint(ip)
	}
	_, err := h.store.WithAudit(ctx, op, func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		row, err := tx.ConsumeAuthState(ctx, state)
		if err != nil {
			return err
		}
		consumed = row
		rec.Effect(store.AuditEffect{
			ResourceType:        "auth_state",
			ResourceID:          row.ProviderID,
			Action:              "CONSUME",
			Outcome:             store.EffectApplied,
			EvidenceKind:        "auth_state_sha256",
			EvidenceFingerprint: fingerprint(state),
		})
		return nil
	})
	if err != nil {
		if store.IsNotFound(err) {
			return store.AuthStateRow{}, h.rejectSSO(ctx, req, providerID, "the login attempt has expired or was already used")
		}
		return store.AuthStateRow{}, internalError(ctx, "failed to complete the login")
	}
	if consumed.ProviderID != providerID || consumed.FlowKind != flowKind {
		return store.AuthStateRow{}, h.rejectSSO(ctx, req, providerID, "the login attempt does not belong to this flow")
	}
	return consumed, nil
}

func (h *Handlers) rejectSSO(ctx context.Context, req connect.AnyRequest, providerID, reason string) error {
	op := store.AuditOperation{
		Class:                store.ClassRejectedAuthentication,
		ActorType:            auth.AnonymousActorType,
		Origin:               auth.ControlRPCOrigin,
		RequestDescriptor:    req.Spec().Procedure,
		AuthorizationOutcome: store.AuthorizationDenied,
		Result:               store.ResultRejected,
		ResultCode:           auditResultCode(ErrSSONoMatchingAccount),
		AuthorizationDetail:  providerID,
	}
	if ip := auth.ClientIP(req); ip != "" {
		op.OriginFingerprint = auth.Fingerprint(ip)
	}
	if _, err := h.store.RecordOperation(ctx, op); err != nil {
		h.logger.Error("failed to record a rejected SSO attempt", "error", err)
	}
	h.logger.Warn("SSO login refused", "provider_id", providerID, "reason", reason)
	return rpcError(ctx, ErrSSONoMatchingAccount, connect.CodeUnauthenticated,
		"could not sign you in; contact an administrator to link your identity")
}

func (h *Handlers) oidcClientFor(ctx context.Context, provider store.IdentityProviderRow, redirectURL string) (*idp.OIDCProvider, error) {
	secret, err := h.kek.DecryptWithContext(provider.ClientSecretEncrypted, crypto.RowAAD(provider.ID, crypto.PurposeIdPClientSecret))
	if err != nil {
		return nil, err
	}
	return h.newOIDC(ctx, idp.ProviderConfig{
		IssuerURL:        provider.IssuerUrl,
		AuthorizationURL: provider.AuthorizationUrl,
		TokenURL:         provider.TokenUrl,
		UserinfoURL:      provider.UserinfoUrl,
		ClientID:         provider.ClientID,
		ClientSecret:     secret,
		Scopes:           provider.Scopes,
		RedirectURL:      strings.TrimSpace(redirectURL),
		GroupClaim:       provider.GroupClaim,
	})
}

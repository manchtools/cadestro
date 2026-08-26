package identity

import (
	"context"
	"time"

	"connectrpc.com/connect"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

func (h *Handlers) mintSession(ctx context.Context, userID, email string, sessionVersion int32) (*auth.TokenPair, error) {
	permissions, grants, err := h.userAuthority(ctx, userID)
	if err != nil {
		return nil, err
	}
	return h.jwt.GenerateTokens(userID, email, permissions, grants, sessionVersion)
}

func (h *Handlers) userAuthority(ctx context.Context, userID string) ([]string, []auth.ScopedGrant, error) {
	permissions, err := h.store.ListUserPermissions(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	grantRows, err := h.store.ListUserScopedGrants(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	grants := make([]auth.ScopedGrant, 0, len(grantRows))
	for _, g := range grantRows {
		sg := auth.ScopedGrant{Permission: g.Permission}
		if g.ScopeKind != nil {
			sg.ScopeKind = *g.ScopeKind
		}
		if g.ScopeID != nil {
			sg.ScopeID = *g.ScopeID
		}
		grants = append(grants, sg)
	}
	return permissions, grants, nil
}

func (h *Handlers) RefreshToken(ctx context.Context, req *connect.Request[cadestrov1.RefreshTokenRequest]) (*connect.Response[cadestrov1.RefreshTokenResponse], error) {

	result, err := h.jwt.ValidateRefreshToken(req.Msg.RefreshToken, func(jti string) (bool, error) {
		return h.store.IsTokenRevoked(ctx, jti)
	})
	if err != nil {
		return nil, h.rejectSession(ctx, req, "invalid or expired refresh token")
	}

	state, err := h.store.GetUserSessionState(ctx, result.Claims.UserID)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, h.rejectSession(ctx, req, "invalid or expired refresh token")
		}
		return nil, internalError(ctx, "failed to resolve session state")
	}

	if state.IsDeleted || state.Disabled || state.SessionVersion != result.Claims.SessionVersion {
		return nil, h.rejectSession(ctx, req, "session invalidated, please log in again")
	}

	rotated, err := h.revokeRefreshToken(ctx, req, result.OldJTI, result.OldExp, result.Claims.UserID, "ROTATE")
	if err != nil {
		return nil, err
	}
	if !rotated {

		return nil, h.rejectSession(ctx, req, "refresh token already used")
	}

	tokens, err := h.mintSession(ctx, result.Claims.UserID, result.Claims.Email, state.SessionVersion)
	if err != nil {
		return nil, internalError(ctx, "failed to issue session")
	}
	return connect.NewResponse(&cadestrov1.RefreshTokenResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    timestampValue(tokens.ExpiresAt),
	}), nil
}

func (h *Handlers) Logout(ctx context.Context, req *connect.Request[cadestrov1.LogoutRequest]) (*connect.Response[cadestrov1.LogoutResponse], error) {
	claims, err := h.jwt.ValidateToken(req.Msg.RefreshToken, auth.TokenTypeRefresh)
	if err != nil {
		return connect.NewResponse(&cadestrov1.LogoutResponse{}), nil
	}
	var exp time.Time
	if claims.ExpiresAt != nil {
		exp = claims.ExpiresAt.Time
	}
	if _, err := h.revokeRefreshToken(ctx, req, claims.ID, exp, claims.UserID, "LOGOUT"); err != nil {
		return nil, err
	}
	return connect.NewResponse(&cadestrov1.LogoutResponse{}), nil
}

func (h *Handlers) revokeRefreshToken(
	ctx context.Context,
	req connect.AnyRequest,
	jti string,
	expiresAt time.Time,
	subjectID string,
	action string,
) (bool, error) {
	if jti == "" {
		return false, nil
	}
	if expiresAt.IsZero() {

		expiresAt = h.now().Add(h.jwt.AccessTokenTTL())
	}

	actor := &auth.UserContext{ID: subjectID, Kind: auth.PrincipalUser}
	op := h.mutationOp(req, actor, "")
	op.AuthorizationOutcome = store.AuthorizationNotApplicable

	revoked := false
	_, err := h.store.WithAudit(ctx, op, func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		_, err := tx.RevokeToken(ctx, db.RevokeTokenParams{Jti: jti, ExpiresAt: expiresAt.UTC()})
		switch {
		case err == nil:
			revoked = true
		case store.IsNotFound(err):

			revoked = false
		default:
			return err
		}

		outcome := store.EffectApplied
		if !revoked {
			outcome = store.EffectRejected
		}
		rec.Effect(store.AuditEffect{
			ResourceType: "session",
			ResourceID:   subjectID,
			Action:       action,
			Outcome:      outcome,

			EvidenceKind:        "session_token_id_sha256",
			EvidenceFingerprint: auth.Fingerprint(jti),
		})
		return nil
	})
	if err != nil {
		h.logger.Error("failed to revoke session token", "action", action, "error", err)
		return false, internalError(ctx, "failed to end session")
	}
	return revoked, nil
}

func (h *Handlers) rejectSession(ctx context.Context, req connect.AnyRequest, msg string) error {
	op := store.AuditOperation{
		Class:                store.ClassRejectedAuthentication,
		ActorType:            auth.AnonymousActorType,
		Origin:               auth.ControlRPCOrigin,
		RequestDescriptor:    req.Spec().Procedure,
		AuthorizationOutcome: store.AuthorizationDenied,
		Result:               store.ResultRejected,
		ResultCode:           auditResultCode(ErrTokenExpired),
	}
	if ip := auth.ClientIP(req); ip != "" {
		op.OriginFingerprint = auth.Fingerprint(ip)
	}
	if _, err := h.store.RecordOperation(ctx, op); err != nil {
		h.logger.Error("failed to record rejected session operation", "error", err)
	}
	return rpcError(ctx, ErrTokenExpired, connect.CodeUnauthenticated, msg)
}

func (h *Handlers) GetCurrentUser(ctx context.Context, req *connect.Request[cadestrov1.GetCurrentUserRequest]) (*connect.Response[cadestrov1.GetCurrentUserResponse], error) {
	actor, err := h.requireActor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, PermGetCurrentUser, actor.ID); err != nil {
		return nil, err
	}
	if !actor.CanOwnResources() {
		return nil, notFound(ctx, ErrUserNotFound, "user not found")
	}

	view, err := h.loadUserView(ctx, actor.ID)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, notFound(ctx, ErrUserNotFound, "user not found")
		}
		return nil, internalError(ctx, "failed to load user")
	}
	return connect.NewResponse(&cadestrov1.GetCurrentUserResponse{User: userToProto(view)}), nil
}

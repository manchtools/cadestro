package identity

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/oklog/ulid/v2"
	"google.golang.org/protobuf/types/known/timestamppb"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

func (h *Handlers) CreateApiToken(ctx context.Context, req *connect.Request[cadestrov1.CreateApiTokenRequest]) (*connect.Response[cadestrov1.CreateApiTokenResponse], error) {
	if req.Msg.ExpiresAt == nil || req.Msg.ExpiresAt.CheckValid() != nil || !req.Msg.ExpiresAt.AsTime().After(h.now().UTC()) {
		return nil, rpcError(ctx, ErrValidationFailed, connect.CodeInvalidArgument, "token expiry must be in the future")
	}
	actor, err := h.requireActor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, PermCreateApiToken, actor.ID); err != nil {
		return nil, err
	}
	state, err := h.store.GetUserSessionState(ctx, actor.ID)
	if err != nil {
		return nil, internalError(ctx, "failed to load user session")
	}
	if state.IsDeleted || state.Disabled || state.SessionVersion != actor.SessionVersion {
		return nil, rpcError(ctx, ErrNotAuthenticated, connect.CodeUnauthenticated, "session is no longer active")
	}
	permissions, grants, err := h.userAuthority(ctx, actor.ID)
	if err != nil {
		return nil, internalError(ctx, "failed to load user authority")
	}
	id, value, err := h.jwt.GenerateAPIToken(actor.ID, actor.Email, permissions, grants, state.SessionVersion, req.Msg.ExpiresAt.AsTime())
	if err != nil {
		return nil, internalError(ctx, "failed to issue API token")
	}
	createdAt := h.now().UTC()
	expiresAt := req.Msg.ExpiresAt.AsTime().UTC()
	var row store.ApiTokenRow
	_, err = h.store.WithAudit(ctx, h.mutationOp(req, actor, PermCreateApiToken), func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		inserted, err := tx.InsertApiToken(ctx, db.InsertApiTokenParams{ID: id, UserID: actor.ID, Name: req.Msg.Name, ExpiresAt: expiresAt, CreatedAt: createdAt})
		if err != nil {
			return fmt.Errorf("insert API token: %w", err)
		}
		row = inserted
		rec.Effect(store.AuditEffect{ResourceType: "api_token", ResourceID: id, Action: "CREATE", Outcome: store.EffectApplied, ChangedFields: []string{"name", "expires_at"}})
		return nil
	})
	if err != nil {
		return nil, internalError(ctx, "failed to create API token")
	}
	return connect.NewResponse(&cadestrov1.CreateApiTokenResponse{Token: apiTokenToProto(row), Value: value}), nil
}

func (h *Handlers) ListApiTokens(ctx context.Context, req *connect.Request[cadestrov1.ListApiTokensRequest]) (*connect.Response[cadestrov1.ListApiTokensResponse], error) {
	actor, err := h.requireActor(ctx)
	if err != nil {
		return nil, err
	}
	if req.Msg.PageToken != "" {
		if _, err := ulid.ParseStrict(req.Msg.PageToken); err != nil {
			return nil, rpcError(ctx, ErrInvalidPageToken, connect.CodeInvalidArgument, "invalid page token")
		}
	}
	if err := h.authorize(ctx, PermListApiTokens, actor.ID); err != nil {
		return nil, err
	}
	limit := pageLimit(req.Msg.PageSize)
	rows, err := h.store.ListApiTokensForUser(ctx, actor.ID, req.Msg.PageToken, limit+1)
	if err != nil {
		return nil, internalError(ctx, "failed to list API tokens")
	}
	total, err := h.store.CountApiTokensForUser(ctx, actor.ID)
	if err != nil {
		return nil, internalError(ctx, "failed to count API tokens")
	}
	next := ""
	if len(rows) > int(limit) {
		rows = rows[:limit]
		next = rows[len(rows)-1].ID
	}
	tokens := make([]*cadestrov1.ApiToken, len(rows))
	for i, row := range rows {
		tokens[i] = apiTokenToProto(row)
	}
	return connect.NewResponse(&cadestrov1.ListApiTokensResponse{Tokens: tokens, NextPageToken: next, TotalCount: boundedIdentityCount(total)}), nil
}

func (h *Handlers) RevokeApiToken(ctx context.Context, req *connect.Request[cadestrov1.RevokeApiTokenRequest]) (*connect.Response[cadestrov1.RevokeApiTokenResponse], error) {
	actor, err := h.requireActor(ctx)
	if err != nil {
		return nil, err
	}
	id := req.Msg.GetId().GetValue()
	if err := h.authorize(ctx, PermRevokeApiToken, actor.ID); err != nil {
		return nil, err
	}
	revokedAt := h.now().UTC()
	_, err = h.store.WithAudit(ctx, h.mutationOp(req, actor, PermRevokeApiToken), func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		if _, err := tx.RevokeApiToken(ctx, db.RevokeApiTokenParams{ID: id, UserID: actor.ID, RevokedAt: &revokedAt}); err != nil {
			return err
		}
		rec.Effect(store.AuditEffect{ResourceType: "api_token", ResourceID: id, Action: "REVOKE", Outcome: store.EffectApplied, ChangedFields: []string{"revoked_at"}})
		return nil
	})
	if err != nil {
		if store.IsNotFound(err) {
			return nil, notFound(ctx, ErrTokenNotFound, "API token not found")
		}
		return nil, internalError(ctx, "failed to revoke API token")
	}
	return connect.NewResponse(&cadestrov1.RevokeApiTokenResponse{}), nil
}

func apiTokenToProto(row store.ApiTokenRow) *cadestrov1.ApiToken {
	token := &cadestrov1.ApiToken{Id: &cadestrov1.ApiTokenId{Value: row.ID}, Name: row.Name, ExpiresAt: timestamppb.New(row.ExpiresAt), CreatedAt: timestamppb.New(row.CreatedAt)}
	if row.RevokedAt != nil {
		token.RevokedAt = timestamppb.New(*row.RevokedAt)
	}
	return token
}

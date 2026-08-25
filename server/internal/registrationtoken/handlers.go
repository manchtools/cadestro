package registrationtoken

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	"github.com/oklog/ulid/v2"
	"google.golang.org/protobuf/types/known/timestamppb"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

const defaultPageSize = int32(50)

type Config struct {
	Store         *store.Store
	Logger        *slog.Logger
	Now           func() time.Time
	CAFingerprint string
}

type Handlers struct {
	store  *store.Store
	logger *slog.Logger
	now    func() time.Time
	caPin  string
}

func New(cfg Config) *Handlers {
	if cfg.Store == nil {
		panic("registrationtoken: store is required")
	}
	decodedPin, err := hex.DecodeString(cfg.CAFingerprint)
	if err != nil || len(decodedPin) != sha256.Size {
		panic("registrationtoken: a SHA-256 CA fingerprint is required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Handlers{
		store: cfg.Store, logger: cfg.Logger, now: cfg.Now,
		caPin: cfg.CAFingerprint,
	}
}

func (h *Handlers) actor(ctx context.Context) (*auth.UserContext, error) {
	actor, ok := auth.UserFromContext(ctx)
	if !ok {
		return nil, rpcError(ctx, errNotAuthenticated, connect.CodeUnauthenticated, "not authenticated")
	}
	return actor, nil
}

func (h *Handlers) authorize(ctx context.Context, permission, resourceID string) error {
	if !auth.AuthorizeContext(ctx, permission, resourceID) {
		return rpcError(ctx, errPermissionDenied, connect.CodePermissionDenied, "permission denied")
	}
	return nil
}

func (h *Handlers) internal(ctx context.Context, operation string, err error) *connect.Error {
	h.logger.Error("registration-token RPC failed", "operation", operation, "error", err)
	return rpcError(ctx, errInternal, connect.CodeInternal, "internal error")
}

func (h *Handlers) operation(req connect.AnyRequest, actor *auth.UserContext, procedure, permission string) store.AuditOperation {
	op := store.AuditOperation{
		Class: store.ClassMutation, ActorType: string(actor.Kind), Origin: auth.ControlRPCOrigin,
		RequestDescriptor: procedure, AuthorizationOutcome: store.AuthorizationAllowed,
		AuthorizationDetail: permission, Result: store.ResultSuccess, ResultCode: "OK",
	}
	if actor.CanOwnResources() {
		op.ActorID = actor.ID
	}
	if ip := auth.ClientIP(req); ip != "" {
		op.OriginFingerprint = auth.Fingerprint(ip)
	}
	return op
}

func tokenEffect(id, action string, fields ...string) store.AuditEffect {
	return store.AuditEffect{
		ResourceType: "registration_token", ResourceID: id, Action: action,
		Outcome: store.EffectApplied, ChangedFields: fields,
	}
}

func (h *Handlers) CreateToken(ctx context.Context, req *connect.Request[cadestrov1.CreateTokenRequest]) (*connect.Response[cadestrov1.CreateTokenResponse], error) {
	if req.Msg.Name == store.BootstrapAdminTokenName {
		return nil, rpcError(ctx, errValidationFailed, connect.CodeInvalidArgument, "token name is reserved")
	}
	if req.Msg.ExpiresAt != nil {
		if err := req.Msg.ExpiresAt.CheckValid(); err != nil {
			return nil, rpcError(ctx, errValidationFailed, connect.CodeInvalidArgument, "invalid token expiry")
		}
	} else {
		return nil, rpcError(ctx, errValidationFailed, connect.CodeInvalidArgument, "token expiry is required")
	}
	expiresAt := req.Msg.ExpiresAt.AsTime().UTC()
	if !expiresAt.After(h.now().UTC()) {
		return nil, rpcError(ctx, errValidationFailed, connect.CodeInvalidArgument, "token expiry must be in the future")
	}
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}

	if !auth.HasPermission(ctx, "CreateToken") {
		return nil, rpcError(ctx, errPermissionDenied, connect.CodePermissionDenied, "permission denied")
	}
	if err := h.authorize(ctx, "CreateToken", ""); err != nil {
		return nil, err
	}

	maxUses := req.Msg.MaxUses

	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, h.internal(ctx, "generate token", err)
	}
	plaintext := base64.RawURLEncoding.EncodeToString(secret)
	digest := sha256.Sum256([]byte(plaintext))
	digestHex := hex.EncodeToString(digest[:])
	id, createdAt := ulid.Make().String(), h.now().UTC()
	var row store.RegistrationTokenRow
	_, err = h.store.WithAudit(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceCreateTokenProcedure, "CreateToken"),
		func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
			inserted, err := tx.InsertRegistrationToken(ctx, db.InsertRegistrationTokenParams{
				ID: id, ValueHash: digestHex, Name: req.Msg.Name,
				MaxUses: maxUses, ExpiresAt: expiresAt, CreatedAt: &createdAt,
				CreatedBy: actor.ID,
			})
			if err != nil {
				return fmt.Errorf("registration token: insert: %w", err)
			}
			row = store.RegistrationTokenRow{Token: inserted}
			row.CurrentUses = 0
			effect := tokenEffect(id, "CREATE", "name", "max_uses", "expires_at")
			effect.EvidenceKind = "registration_token"
			effect.EvidenceFingerprint = digestHex
			rec.Effect(effect)
			return nil
		})
	if err != nil {
		return nil, h.internal(ctx, "create token", err)
	}
	out := tokenToProto(row)
	out.Value = plaintext
	return connect.NewResponse(&cadestrov1.CreateTokenResponse{Token: out, CaFingerprintPin: h.caPin}), nil
}

func (h *Handlers) ListTokens(ctx context.Context, req *connect.Request[cadestrov1.ListTokensRequest]) (*connect.Response[cadestrov1.ListTokensResponse], error) {
	if req.Msg.PageToken != "" {
		if _, err := ulid.ParseStrict(req.Msg.PageToken); err != nil {
			return nil, rpcError(ctx, errInvalidPageToken, connect.CodeInvalidArgument, "invalid page token")
		}
	}
	if _, err := h.actor(ctx); err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, "ListTokens", ""); err != nil {
		return nil, err
	}
	pageSize := req.Msg.PageSize
	if pageSize == 0 {
		pageSize = defaultPageSize
	}
	rows, err := h.store.ListRegistrationTokens(ctx, store.RegistrationTokenListFilter{
		AfterID: req.Msg.PageToken, Limit: pageSize + 1, IncludeDisabled: req.Msg.IncludeDisabled,
	})
	if err != nil {
		return nil, h.internal(ctx, "list tokens", err)
	}
	count, err := h.store.CountRegistrationTokens(ctx, req.Msg.IncludeDisabled)
	if err != nil {
		return nil, h.internal(ctx, "count tokens", err)
	}
	next := ""
	if len(rows) > int(pageSize) {
		rows = rows[:pageSize]
		next = rows[len(rows)-1].ID
	}
	out := make([]*cadestrov1.RegistrationToken, len(rows))
	for i, row := range rows {
		out[i] = tokenToProto(row)
	}
	return connect.NewResponse(&cadestrov1.ListTokensResponse{
		Tokens: out, NextPageToken: next, TotalCount: boundedCount(count),
	}), nil
}

func (h *Handlers) RenameToken(ctx context.Context, req *connect.Request[cadestrov1.RenameTokenRequest]) (*connect.Response[cadestrov1.UpdateTokenResponse], error) {
	if req.Msg.Name == store.BootstrapAdminTokenName {
		return nil, rpcError(ctx, errValidationFailed, connect.CodeInvalidArgument, "token name is reserved")
	}
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, "RenameToken", req.Msg.GetId().GetValue()); err != nil {
		return nil, err
	}
	var row store.RegistrationTokenRow
	_, err = h.store.WithAudit(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceRenameTokenProcedure, "RenameToken"),
		func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
			updated, err := tx.RenameRegistrationToken(ctx, db.RenameRegistrationTokenParams{
				ID: req.Msg.GetId().GetValue(), Name: req.Msg.Name, ReservedName: store.BootstrapAdminTokenName,
			})
			if err != nil {
				return err
			}
			row = store.RegistrationTokenRow{Token: updated}
			tokenID := updated.ID
			uses, countErr := tx.CountRegistrationTokenUses(ctx, &tokenID)
			if countErr != nil {
				return fmt.Errorf("registration token: count uses: %w", countErr)
			}
			row.CurrentUses = int32(uses)
			rec.Effect(tokenEffect(req.Msg.GetId().GetValue(), "UPDATE", "name"))
			return nil
		})
	if err != nil {
		return nil, h.writeError(ctx, "rename token", err)
	}
	return connect.NewResponse(&cadestrov1.UpdateTokenResponse{Token: tokenToProto(row)}), nil
}

func (h *Handlers) SetTokenDisabled(ctx context.Context, req *connect.Request[cadestrov1.SetTokenDisabledRequest]) (*connect.Response[cadestrov1.UpdateTokenResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, "SetTokenDisabled", req.Msg.GetId().GetValue()); err != nil {
		return nil, err
	}
	var row store.RegistrationTokenRow
	_, err = h.store.WithAudit(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceSetTokenDisabledProcedure, "SetTokenDisabled"),
		func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
			updated, err := tx.SetRegistrationTokenDisabled(ctx, db.SetRegistrationTokenDisabledParams{
				ID: req.Msg.GetId().GetValue(), Disabled: req.Msg.Disabled, ReservedName: store.BootstrapAdminTokenName,
			})
			if err != nil {
				return err
			}
			row = store.RegistrationTokenRow{Token: updated}
			tokenID := updated.ID
			uses, countErr := tx.CountRegistrationTokenUses(ctx, &tokenID)
			if countErr != nil {
				return fmt.Errorf("registration token: count uses: %w", countErr)
			}
			row.CurrentUses = int32(uses)
			effect := tokenEffect(req.Msg.GetId().GetValue(), "UPDATE", "disabled")
			effect.AfterFlag = &req.Msg.Disabled
			rec.Effect(effect)
			return nil
		})
	if err != nil {
		return nil, h.writeError(ctx, "set token disabled", err)
	}
	return connect.NewResponse(&cadestrov1.UpdateTokenResponse{Token: tokenToProto(row)}), nil
}

func (h *Handlers) DeleteToken(ctx context.Context, req *connect.Request[cadestrov1.DeleteTokenRequest]) (*connect.Response[cadestrov1.DeleteTokenResponse], error) {
	actor, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.authorize(ctx, "DeleteToken", req.Msg.GetId().GetValue()); err != nil {
		return nil, err
	}
	_, err = h.store.WithAudit(ctx, h.operation(req, actor,
		cadestrov1connect.ControlServiceDeleteTokenProcedure, "DeleteToken"),
		func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
			if _, err := tx.SoftDeleteRegistrationToken(ctx, db.SoftDeleteRegistrationTokenParams{
				ID: req.Msg.GetId().GetValue(), ReservedName: store.BootstrapAdminTokenName,
			}); err != nil {
				return err
			}
			rec.Effect(tokenEffect(req.Msg.GetId().GetValue(), "DELETE", "is_deleted"))
			return nil
		})
	if err != nil {
		return nil, h.writeError(ctx, "delete token", err)
	}
	return connect.NewResponse(&cadestrov1.DeleteTokenResponse{}), nil
}

func (h *Handlers) writeError(ctx context.Context, operation string, err error) error {
	if store.IsNotFound(err) {
		return notFound(ctx, errTokenNotFound, "token not found")
	}
	return h.internal(ctx, operation, err)
}

func tokenToProto(row store.RegistrationTokenRow) *cadestrov1.RegistrationToken {
	out := &cadestrov1.RegistrationToken{
		Id: &cadestrov1.RegistrationTokenId{Value: row.ID}, Name: row.Name, MaxUses: row.MaxUses, CurrentUses: row.CurrentUses,
		CreatedBy: row.CreatedBy, Disabled: row.Disabled,
	}
	out.ExpiresAt = timestamppb.New(row.ExpiresAt)
	if row.CreatedAt != nil {
		out.CreatedAt = timestamppb.New(*row.CreatedAt)
	}
	return out
}

func boundedCount(n int64) int32 {
	const maxInt32 = int64(^uint32(0) >> 1)
	if n > maxInt32 {
		return int32(maxInt32)
	}
	return int32(n)
}

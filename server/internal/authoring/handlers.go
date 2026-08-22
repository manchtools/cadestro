package authoring

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"time"

	"connectrpc.com/connect"
	"github.com/oklog/ulid/v2"

	"github.com/manchtools/cadestro/server/internal/auth"
	pmcrypto "github.com/manchtools/cadestro/server/internal/crypto"
	"github.com/manchtools/cadestro/server/internal/store"
)

const defaultAuthoringPageSize = int32(50)

// HandlersConfig supplies the direct SQLite store and process-local seams
// used by the authoring RPC handlers.
type HandlersConfig struct {
	Store  *store.Store
	AtRest *pmcrypto.Encryptor
	Logger *slog.Logger
	Now    func() time.Time
}

// Handlers implements the explicit Action, ActionSet and Definition authoring
// RPCs.
type Handlers struct {
	store  *store.Store
	state  *Service
	logger *slog.Logger
	atRest *pmcrypto.Encryptor
}

// NewHandlers constructs the explicit authoring RPC handlers.
func NewHandlers(cfg HandlersConfig) *Handlers {
	if cfg.Store == nil || cfg.AtRest == nil {
		panic("authoring: handler store and at-rest cipher are required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Handlers{
		store: cfg.Store, state: New(Config{Store: cfg.Store, Now: cfg.Now}),
		logger: cfg.Logger, atRest: cfg.AtRest,
	}
}

func (h *Handlers) actor(ctx context.Context) (*auth.UserContext, error) {
	actor, ok := auth.UserFromContext(ctx)
	if !ok {
		return nil, authoringRPCError(ctx, errNotAuthenticated, connect.CodeUnauthenticated, "not authenticated")
	}
	return actor, nil
}

func (h *Handlers) authorize(ctx context.Context, permission, resourceID string) error {
	if !auth.AuthorizeContext(ctx, permission, resourceID) {
		return authoringRPCError(ctx, errPermissionDenied, connect.CodePermissionDenied, "permission denied")
	}
	return nil
}

func (h *Handlers) internal(ctx context.Context, operation string, err error) *connect.Error {
	h.logger.Error("authoring RPC failed", "operation", operation, "error", err)
	return authoringRPCError(ctx, errInternal, connect.CodeInternal, "internal error")
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

func (h *Handlers) actionError(ctx context.Context, operation string, err error) error {
	switch {
	case errors.Is(err, ErrInvalidInput):
		return authoringRPCError(ctx, errValidationFailed, connect.CodeInvalidArgument, "invalid action")
	case errors.Is(err, ErrSystemAction):
		return authoringRPCError(ctx, errCannotModifySystemAction, connect.CodeFailedPrecondition, "system-managed action cannot be modified")
	case store.IsNotFound(err):
		return authoringNotFound(ctx, errActionNotFound, "action not found")
	default:
		return h.internal(ctx, operation, err)
	}
}

func validPageToken(token string) bool {
	if token == "" {
		return true
	}
	_, err := ulid.ParseStrict(token)
	return err == nil
}

func boundedCount(n int64) int32 {
	if n > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(n)
}

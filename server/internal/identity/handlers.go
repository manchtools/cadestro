package identity

import (
	"context"
	"log/slog"
	"time"

	"connectrpc.com/connect"

	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/crypto"
	"github.com/manchtools/cadestro/server/internal/idp"
	"github.com/manchtools/cadestro/server/internal/store"
)

type Store interface {
	WithAudit(ctx context.Context, op store.AuditOperation, mutate func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error) (store.AuditRecord, error)
	RecordOperation(ctx context.Context, op store.AuditOperation, effects ...store.AuditEffect) (store.AuditRecord, error)

	GetUser(ctx context.Context, id string) (store.UserRow, error)
	GetUserByEmail(ctx context.Context, email string) (store.UserRow, error)
	GetUserSessionState(ctx context.Context, id string) (store.UserSessionStateRow, error)
	ListUsers(ctx context.Context, after string, limit int32) ([]store.UserRow, error)
	CountUsers(ctx context.Context) (int64, error)
	ListUserPermissions(ctx context.Context, userID string) ([]string, error)
	ListUserScopedGrants(ctx context.Context, userID string) ([]store.ScopedGrantRow, error)
	ListUserRoleGrants(ctx context.Context, userID string) ([]store.RoleGrantRow, error)
	ListInheritedRolesForUser(ctx context.Context, userID string) ([]store.InheritedRoleRow, error)
	ListUserGroupIDsForUser(ctx context.Context, userID string) ([]string, error)
	GetUserGroupView(ctx context.Context, id string) (store.UserGroupView, error)
	ListUserGroups(ctx context.Context, filter store.UserGroupListFilter) ([]store.UserGroupView, error)
	CountUserGroups(ctx context.Context, filter store.UserGroupListFilter) (int64, error)
	ListUserGroupsForUser(ctx context.Context, userID string, filter store.UserGroupListFilter) ([]store.UserGroupView, error)
	ListUserGroupMembers(ctx context.Context, groupID string) ([]store.UserGroupMemberView, error)
	ListUserGroupRoleGrants(ctx context.Context, groupID string) ([]store.GroupRoleGrantRow, error)
	ListUsersForDynamicUserGroupEvaluation(ctx context.Context) ([]store.UserDynamicEvaluationRow, error)
	ListUserSSHKeys(ctx context.Context, userID string) ([]store.UserSSHKeyRow, error)
	ListIdentityLinksForUser(ctx context.Context, userID string) ([]store.IdentityLinkWithProviderRow, error)
	GetIdentityLink(ctx context.Context, id string) (store.IdentityLinkRow, error)

	GetRole(ctx context.Context, id string) (store.RoleRow, error)
	GetRoleByName(ctx context.Context, name string) (store.RoleRow, error)
	ListRoles(ctx context.Context, after string, limit int32) ([]store.RoleRow, error)
	CountRoles(ctx context.Context) (int64, error)
	CountRoleHolders(ctx context.Context, roleID string) (int64, error)
	GetServerSettings(ctx context.Context) (store.ServerSettingsRow, error)
	ListAuditEventRows(ctx context.Context, filter store.AuditEventFilter) ([]store.AuditEventRow, error)
	CountAuditEventRows(ctx context.Context, filter store.AuditEventFilter) (int64, error)

	GetIdentityProvider(ctx context.Context, id string) (store.IdentityProviderRow, error)
	GetIdentityProviderBySlug(ctx context.Context, slug string) (store.IdentityProviderRow, error)
	ListIdentityProviders(ctx context.Context, after string, limit int32) ([]store.IdentityProviderRow, error)
	ListEnabledIdentityProviders(ctx context.Context) ([]store.IdentityProviderRow, error)
	CountIdentityProviders(ctx context.Context) (int64, error)

	IsTokenRevoked(ctx context.Context, jti string) (bool, error)
	ListApiTokensForUser(ctx context.Context, userID, afterID string, limit int32) ([]store.ApiTokenRow, error)
	CountApiTokensForUser(ctx context.Context, userID string) (int64, error)
	GetApiTokenForAuth(ctx context.Context, id, userID string) (store.ApiTokenRow, error)
}

type ProviderFactory func(ctx context.Context, cfg idp.ProviderConfig) (*idp.OIDCProvider, error)

type Config struct {
	Store  Store
	Logger *slog.Logger

	JWT *auth.JWTManager

	KEK *crypto.Encryptor

	PublicBaseURL string

	NewProvider ProviderFactory

	Now func() time.Time
}

type Handlers struct {
	store   Store
	logger  *slog.Logger
	jwt     *auth.JWTManager
	kek     *crypto.Encryptor
	baseURL string
	newOIDC ProviderFactory
	linker  *idp.Linker
	now     func() time.Time
}

func New(cfg Config) *Handlers {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	newOIDC := cfg.NewProvider
	if newOIDC == nil {
		newOIDC = idp.NewOIDCProvider
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Handlers{
		store:   cfg.Store,
		logger:  logger,
		jwt:     cfg.JWT,
		kek:     cfg.KEK,
		baseURL: cfg.PublicBaseURL,
		newOIDC: newOIDC,
		linker:  idp.NewLinker(cfg.KEK, now),
		now:     now,
	}
}

func (h *Handlers) requireActor(ctx context.Context) (*auth.UserContext, error) {
	actor, ok := auth.UserFromContext(ctx)
	if !ok {
		return nil, rpcError(ctx, ErrNotAuthenticated, connect.CodeUnauthenticated, "not authenticated")
	}
	return actor, nil
}

func (h *Handlers) authorize(ctx context.Context, permission, resourceID string) error {
	if !auth.AuthorizeContext(ctx, permission, resourceID) {
		return rpcError(ctx, ErrPermissionDenied, connect.CodePermissionDenied, "permission denied")
	}
	return nil
}

func (h *Handlers) mutationOp(req connect.AnyRequest, actor *auth.UserContext, permission string) store.AuditOperation {
	return h.operation(req, actor, store.ClassMutation, permission, store.AuthorizationAllowed, store.ResultSuccess, "")
}

func (h *Handlers) operation(
	req connect.AnyRequest,
	actor *auth.UserContext,
	class store.OperationClass,
	permission string,
	outcome store.AuthorizationOutcome,
	result store.OperationResult,
	resultCode string,
) store.AuditOperation {
	op := store.AuditOperation{
		Class:                class,
		ActorType:            string(auth.PrincipalUser),
		Origin:               auth.ControlRPCOrigin,
		RequestDescriptor:    req.Spec().Procedure,
		AuthorizationOutcome: outcome,
		AuthorizationDetail:  permission,
		Result:               result,
		ResultCode:           resultCode,
	}
	if actor != nil {
		op.ActorType = string(actor.Kind)
		if actor.CanOwnResources() {
			op.ActorID = actor.ID
		}
	}
	if ip := auth.ClientIP(req); ip != "" {
		op.OriginFingerprint = auth.Fingerprint(ip)
	}
	return op
}

func (h *Handlers) sealForSubject(subjectID, wrappedDEK, field, value string) ([]byte, error) {
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

const (
	defaultPageSize = 50
	maxPageSize     = 200
)

func pageLimit(requested int32) int32 {
	switch {
	case requested <= 0:
		return defaultPageSize
	case requested > maxPageSize:
		return maxPageSize
	default:
		return requested
	}
}

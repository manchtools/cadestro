package scim

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/crypto"
	"github.com/manchtools/cadestro/server/internal/store"
)

type Store interface {
	WithAudit(ctx context.Context, op store.AuditOperation, mutate func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error) (store.AuditRecord, error)
	RecordOperation(ctx context.Context, op store.AuditOperation, effects ...store.AuditEffect) (store.AuditRecord, error)

	GetIdentityProviderBySlug(ctx context.Context, slug string) (store.IdentityProviderRow, error)
	GetServerSettings(ctx context.Context) (store.ServerSettingsRow, error)

	GetUser(ctx context.Context, id string) (store.UserRow, error)
	GetUserByEmail(ctx context.Context, email string) (store.UserRow, error)
	ListSCIMUsers(ctx context.Context, providerID string, limit, offset int32) ([]store.SCIMUserRow, error)
	CountSCIMUsers(ctx context.Context, providerID string) (int64, error)
	FindSCIMUserByEmail(ctx context.Context, providerID, email string) (store.SCIMUserRow, error)
	FindSCIMUserByExternalID(ctx context.Context, providerID, externalID string) (store.SCIMUserRow, error)
	GetIdentityLinkByProviderAndUser(ctx context.Context, providerID, userID string) (store.IdentityLinkRow, error)
	CountIdentityLinksForUser(ctx context.Context, userID string) (int64, error)

	GetUserGroup(ctx context.Context, id string) (store.UserGroupRow, error)
	ListUserGroupMemberIDs(ctx context.Context, groupID string) ([]string, error)
	GetSCIMGroupMapping(ctx context.Context, providerID, scimGroupID string) (store.SCIMGroupMappingRow, error)
	GetSCIMGroupMappingByUserGroup(ctx context.Context, providerID, userGroupID string) (store.SCIMGroupMappingRow, error)
	ListSCIMGroupMappings(ctx context.Context, providerID string) ([]store.SCIMGroupMappingRow, error)
}

const (
	Origin = "scim"

	ActorTypeProvider = "scim_provider"

	AuthorizationDetail = "scim_bearer_token"
)

const (
	reasonMissingCredentials = "missing_credentials"
	reasonUnknownProvider    = "unknown_provider"
	reasonProviderDisabled   = "provider_disabled"
	reasonSCIMDisabled       = "scim_disabled"
	reasonNoTokenConfigured  = "no_token_configured"
	reasonInvalidToken       = "invalid_token"
)

const (
	providerRequestsPerWindow = 100

	providerIPRequestsPerWindow = 20

	rejectedPerWindow = 20
	rateLimitWindow   = time.Minute
)

type Config struct {
	Store  Store
	Logger *slog.Logger

	KEK *crypto.Encryptor

	Now func() time.Time
}

type Handler struct {
	store  Store
	logger *slog.Logger
	kek    *crypto.Encryptor
	now    func() time.Time

	providerLimiter  *auth.RateLimiter
	providerIPLimit  *auth.RateLimiter
	rejectionLimiter *auth.RateLimiter
}

func New(cfg Config) *Handler {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		store:            cfg.Store,
		logger:           logger,
		kek:              cfg.KEK,
		now:              now,
		providerLimiter:  auth.NewRateLimiter(providerRequestsPerWindow, rateLimitWindow),
		providerIPLimit:  auth.NewRateLimiter(providerIPRequestsPerWindow, rateLimitWindow),
		rejectionLimiter: auth.NewRateLimiter(rejectedPerWindow, rateLimitWindow),
	}
}

func (h *Handler) Close() {
	h.providerLimiter.Stop()
	h.providerIPLimit.Stop()
	h.rejectionLimiter.Stop()
}

type session struct {
	provider store.IdentityProviderRow

	descriptor string

	tokenFingerprint string

	originFingerprint string
}

type routeHandler func(w http.ResponseWriter, r *http.Request, s *session)

const (
	DescUsersList     = "scim.v2.Users.List"
	DescUsersGet      = "scim.v2.Users.Get"
	DescUsersCreate   = "scim.v2.Users.Create"
	DescUsersReplace  = "scim.v2.Users.Replace"
	DescUsersPatch    = "scim.v2.Users.Patch"
	DescUsersDelete   = "scim.v2.Users.Delete"
	DescGroupsList    = "scim.v2.Groups.List"
	DescGroupsGet     = "scim.v2.Groups.Get"
	DescGroupsCreate  = "scim.v2.Groups.Create"
	DescGroupsReplace = "scim.v2.Groups.Replace"
	DescGroupsPatch   = "scim.v2.Groups.Patch"
	DescGroupsDelete  = "scim.v2.Groups.Delete"

	DescServiceProviderConfig = "scim.v2.ServiceProviderConfig"
	DescSchemas               = "scim.v2.Schemas"
	DescResourceTypes         = "scim.v2.ResourceTypes"
)

func MutationRoutes() []string {
	return []string{
		DescUsersCreate,
		DescUsersReplace,
		DescUsersPatch,
		DescUsersDelete,
		DescGroupsCreate,
		DescGroupsReplace,
		DescGroupsPatch,
		DescGroupsDelete,
	}
}

func SensitiveReadRoutes() []string {
	return []string{
		DescUsersList,
		DescUsersGet,
		DescGroupsList,
		DescGroupsGet,
	}
}

func DiscoveryRoutes() []string {
	return []string{
		DescServiceProviderConfig,
		DescSchemas,
		DescResourceTypes,
	}
}

func (h *Handler) Mount(mux *http.ServeMux) []string {
	var mounted []string
	register := func(method, path, descriptor string, handle routeHandler) {
		mux.HandleFunc(method+" "+path, h.withAuth(descriptor, handle))
		mounted = append(mounted, descriptor)
	}

	const base = "/scim/v2/{slug}"

	register(http.MethodGet, base+"/ServiceProviderConfig", DescServiceProviderConfig, h.serviceProviderConfig)
	register(http.MethodGet, base+"/Schemas", DescSchemas, h.schemas)
	register(http.MethodGet, base+"/ResourceTypes", DescResourceTypes, h.resourceTypes)

	register(http.MethodGet, base+"/Users", DescUsersList, h.listUsers)
	register(http.MethodPost, base+"/Users", DescUsersCreate, h.createUser)
	register(http.MethodGet, base+"/Users/{id}", DescUsersGet, h.getUser)
	register(http.MethodPut, base+"/Users/{id}", DescUsersReplace, h.replaceUser)
	register(http.MethodPatch, base+"/Users/{id}", DescUsersPatch, h.patchUser)
	register(http.MethodDelete, base+"/Users/{id}", DescUsersDelete, h.deleteUser)

	register(http.MethodGet, base+"/Groups", DescGroupsList, h.listGroups)
	register(http.MethodPost, base+"/Groups", DescGroupsCreate, h.createGroup)
	register(http.MethodGet, base+"/Groups/{id}", DescGroupsGet, h.getGroup)
	register(http.MethodPut, base+"/Groups/{id}", DescGroupsReplace, h.replaceGroup)
	register(http.MethodPatch, base+"/Groups/{id}", DescGroupsPatch, h.patchGroup)
	register(http.MethodDelete, base+"/Groups/{id}", DescGroupsDelete, h.deleteGroup)

	return mounted
}

func (h *Handler) mutationOp(s *session) store.AuditOperation {
	return h.operation(s, store.ClassBackgroundWriter)
}

func (h *Handler) sensitiveReadOp(s *session) store.AuditOperation {
	return h.operation(s, store.ClassSensitiveRead)
}

func (h *Handler) operation(s *session, class store.OperationClass) store.AuditOperation {
	return store.AuditOperation{
		Class:     class,
		ActorType: ActorTypeProvider,

		ActorID:              s.provider.ID,
		ActorFingerprint:     s.tokenFingerprint,
		Origin:               Origin,
		OriginFingerprint:    s.originFingerprint,
		RequestDescriptor:    s.descriptor,
		AuthorizationOutcome: store.AuthorizationAllowed,
		AuthorizationDetail:  AuthorizationDetail,
		Result:               store.ResultSuccess,
	}
}

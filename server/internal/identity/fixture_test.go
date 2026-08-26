package identity_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	connectvalidate "connectrpc.com/validate"
	"github.com/manchtools/cadestro/server/internal/testdb"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"

	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/crypto"
	"github.com/manchtools/cadestro/server/internal/identity"
	"github.com/manchtools/cadestro/server/internal/idp"
	"github.com/manchtools/cadestro/server/internal/store"
)

const testKEK = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

const testBaseURL = "https://control.test.example"

type fixture struct {
	t        *testing.T
	store    *store.Store
	raw      *testdb.DB
	handlers *identity.Handlers
	boot     *identity.Bootstrapper
	jwt      *auth.JWTManager
	kek      *crypto.Encryptor
	server   *httptest.Server
	client   cadestrov1connect.ControlServiceClient
	mounted  []string
	now      time.Time

	clock *time.Time

	sessionKey ed25519.PrivateKey
}

func newFixture(t *testing.T, opts ...fixtureOption) *fixture {
	t.Helper()
	st, raw := setupSQLite(t)

	cfg := &fixtureConfig{now: time.Now().UTC().Truncate(time.Microsecond)}
	for _, o := range opts {
		o(cfg)
	}

	clock := new(time.Time)
	*clock = cfg.now
	tick := func() time.Time { return *clock }

	kek, err := crypto.NewEncryptor(testKEK)
	require.NoError(t, err)

	_, priv, err := auth.GenerateSessionKey()
	require.NoError(t, err)
	jwt, err := auth.NewJWTManager(auth.JWTConfig{
		PrivateKey: priv,
		Now:        tick,
	})
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	_, err = raw.Exec(t.Context(), `UPDATE roles SET permissions = $1 WHERE id = $2`, auth.AdminPermissions(), auth.AdminRoleID)
	require.NoError(t, err)
	_, err = raw.Exec(t.Context(), `UPDATE roles SET permissions = $1 WHERE id = $2`, auth.DefaultUserPermissions(), auth.UserRoleID)
	require.NoError(t, err)
	handlers := identity.New(identity.Config{
		Store:         st,
		Logger:        logger,
		JWT:           jwt,
		KEK:           kek,
		PublicBaseURL: testBaseURL,
		NewProvider:   cfg.newProvider,
		Now:           tick,
	})
	boot := identity.NewBootstrapper(st, testBaseURL, cfg.bootstrapTTL, tick)

	chain := connect.WithInterceptors(
		connectvalidate.NewInterceptor(),
		auth.NewAuthInterceptor(logger, jwt, auth.RateLimiters{}, auth.NewRejectionRecorder(st)).
			WithBootstrapAuthenticator(boot).WithAPITokenAuthenticator(handlers),
		auth.NewAuthzInterceptor(),
	)

	mux := http.NewServeMux()
	mounted := handlers.Mount(mux, chain)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &fixture{
		t:          t,
		store:      st,
		raw:        raw,
		handlers:   handlers,
		boot:       boot,
		jwt:        jwt,
		kek:        kek,
		server:     srv,
		client:     cadestrov1connect.NewControlServiceClient(srv.Client(), srv.URL),
		mounted:    mounted,
		now:        cfg.now,
		clock:      clock,
		sessionKey: priv,
	}
}

func (f *fixture) advance(d time.Duration) {
	*f.clock = f.clock.Add(d)
}

func (f *fixture) rebuildSearch() {
	f.t.Helper()
	require.NoError(f.t, f.store.RebuildSearchIndexes(f.ctx(), store.AuditOperation{
		Class: store.ClassBackgroundWriter, ActorType: "system", Origin: "test_fixture",
		RequestDescriptor: "search.fixture/rebuild", AuthorizationOutcome: store.AuthorizationNotApplicable,
		Result: store.ResultSuccess, ResultCode: "OK",
	}))
}

type fixtureConfig struct {
	now          time.Time
	newProvider  identity.ProviderFactory
	bootstrapTTL time.Duration
}

type fixtureOption func(*fixtureConfig)

func withProviderFactory(f identity.ProviderFactory) fixtureOption {
	return func(c *fixtureConfig) { c.newProvider = f }
}

func withBootstrapTTL(d time.Duration) fixtureOption {
	return func(c *fixtureConfig) { c.bootstrapTTL = d }
}

type grant struct {
	Permissions []string

	ScopeKind string
	ScopeID   string
}

type actor struct {
	ID     string
	Email  string
	Token  string
	RoleID string
}

func (f *fixture) seedActor(grants ...grant) *actor {
	f.t.Helper()
	userID := newULID()

	email := "actor-" + strings.ToLower(userID[20:]) + "@test.example"
	f.insertUser(userID, email)

	a := &actor{ID: userID, Email: email}
	for _, g := range grants {
		roleID := f.insertRole(g.Permissions)
		a.RoleID = roleID
		f.insertUserRoleGrant(userID, roleID, g.ScopeKind, g.ScopeID)
	}
	a.Token = f.mintToken(userID, email)
	return a
}

func (f *fixture) seedSubject() *actor {
	f.t.Helper()
	userID := newULID()
	email := "subject-" + strings.ToLower(userID[20:]) + "@test.example"
	f.insertUser(userID, email)
	return &actor{ID: userID, Email: email}
}

func (f *fixture) seedJITSubject() *actor {
	f.t.Helper()
	userID := newULID()
	email := "jit-" + strings.ToLower(userID[20:]) + "@test.example"
	f.insertUserWithSource(userID, email, "oidc_jit")
	return &actor{ID: userID, Email: email}
}

func (f *fixture) mintToken(userID, email string) string {
	f.t.Helper()
	ctx := f.ctx()
	perms, err := f.store.ListUserPermissions(ctx, userID)
	require.NoError(f.t, err)
	grantRows, err := f.store.ListUserScopedGrants(ctx, userID)
	require.NoError(f.t, err)
	scoped := make([]auth.ScopedGrant, 0, len(grantRows))
	for _, g := range grantRows {
		sg := auth.ScopedGrant{Permission: g.Permission}
		if g.ScopeKind != nil {
			sg.ScopeKind = *g.ScopeKind
		}
		if g.ScopeID != nil {
			sg.ScopeID = *g.ScopeID
		}
		scoped = append(scoped, sg)
	}
	state, err := f.store.GetUserSessionState(ctx, userID)
	require.NoError(f.t, err)
	pair, err := f.jwt.GenerateTokens(userID, email, perms, scoped, state.SessionVersion)
	require.NoError(f.t, err)
	return pair.AccessToken
}

func (f *fixture) insertUser(id, email string) {
	f.insertUserWithSource(id, email, "scim")
}

func (f *fixture) insertUserWithSource(id, email, source string) {
	f.t.Helper()
	_, err := f.raw.Exec(f.ctx(),
		`INSERT INTO users (id, email, provisioning_source, created_at, updated_at) VALUES ($1, $2, $3, $4, $4)`,
		id, email, source, f.now)
	require.NoError(f.t, err)

	wrapped, err := crypto.GenerateWrappedDEK(f.kek, id)
	require.NoError(f.t, err)
	_, err = f.raw.Exec(f.ctx(),
		`INSERT INTO user_encryption_keys (user_id, wrapped_dek) VALUES ($1, $2)`, id, wrapped)
	require.NoError(f.t, err)
}

func (f *fixture) insertRole(perms []string) string {
	f.t.Helper()
	roleID := newULID()
	_, err := f.raw.Exec(f.ctx(),
		`INSERT INTO roles (id, name, description, permissions, is_system, created_at, updated_at)
		 VALUES ($1, $2, '', $3, FALSE, $4, $4)`,
		roleID, "role-"+roleID[18:], perms, f.now)
	require.NoError(f.t, err)
	return roleID
}

func (f *fixture) insertUserRoleGrant(userID, roleID, scopeKind, scopeID string) string {
	f.t.Helper()
	grantID := newULID()
	var kind, id any
	if scopeKind != "" {
		kind, id = scopeKind, scopeID
	}
	_, err := f.raw.Exec(f.ctx(),
		`INSERT INTO user_roles (grant_id, user_id, role_id, assigned_at, assigned_by, scope_kind, scope_id)
		 VALUES ($1, $2, $3, $4, '', $5, $6)`,
		grantID, userID, roleID, f.now, kind, id)
	require.NoError(f.t, err)
	return grantID
}

func (f *fixture) insertUserGroup() string {
	f.t.Helper()
	groupID := newULID()
	_, err := f.raw.Exec(f.ctx(),
		`INSERT INTO user_groups (id, name, created_at, updated_at) VALUES ($1, $2, $3, $3)`,
		groupID, "group-"+groupID[18:], f.now)
	require.NoError(f.t, err)
	return groupID
}

func (f *fixture) addUserToGroup(groupID, userID string) {
	f.t.Helper()
	_, err := f.raw.Exec(f.ctx(),
		`INSERT INTO user_group_members (group_id, user_id, added_at, added_by) VALUES ($1, $2, $3, '')`,
		groupID, userID, f.now)
	require.NoError(f.t, err)
}

func (f *fixture) insertDeviceGroup() string {
	f.t.Helper()
	groupID := newULID()
	_, err := f.raw.Exec(f.ctx(),
		`INSERT INTO device_groups (id, name, created_at) VALUES ($1, $2, $3)`,
		groupID, "dgroup-"+groupID[18:], f.now)
	require.NoError(f.t, err)
	return groupID
}

func (f *fixture) insertProvider(slug string, mutate func(*providerSeed)) string {
	f.t.Helper()
	seed := providerSeed{Enabled: true, ClientID: "client-id", Secret: "client-secret", GroupMapping: "{}"}
	if mutate != nil {
		mutate(&seed)
	}
	providerID := newULID()
	sealed, err := f.kek.EncryptWithContext(seed.Secret, crypto.RowAAD(providerID, crypto.PurposeIdPClientSecret))
	require.NoError(f.t, err)
	_, err = f.raw.Exec(f.ctx(),
		`INSERT INTO identity_providers
		   (id, name, slug, provider_type, enabled, client_id, client_secret_encrypted, issuer_url,
		    auto_create_users, auto_link_by_email, trust_email_assertions, default_role_id,
		    group_claim, group_mapping, created_at, updated_at)
		 VALUES ($1, $2, $3, 'oidc', $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $14)`,
		providerID, "provider-"+slug, slug, seed.Enabled, seed.ClientID, sealed, seed.IssuerURL,
		seed.AutoCreateUsers, seed.AutoLinkByEmail, seed.TrustEmailAssertions, seed.DefaultRoleID,
		seed.GroupClaim, seed.GroupMapping, f.now)
	require.NoError(f.t, err)
	return providerID
}

type providerSeed struct {
	Enabled              bool
	ClientID             string
	Secret               string
	IssuerURL            string
	AutoCreateUsers      bool
	AutoLinkByEmail      bool
	TrustEmailAssertions bool
	DefaultRoleID        string
	GroupClaim           string
	GroupMapping         string
}

func (f *fixture) insertIdentityLink(userID, providerID, externalID string) string {
	f.t.Helper()
	linkID := newULID()
	_, err := f.raw.Exec(f.ctx(),
		`INSERT INTO identity_links (id, user_id, provider_id, external_id, external_email, external_name, linked_at)
		 VALUES ($1, $2, $3, $4, '', '', $5)`,
		linkID, userID, providerID, externalID, f.now)
	require.NoError(f.t, err)
	return linkID
}

func authed[T any](msg *T, token string) *connect.Request[T] {
	req := connect.NewRequest(msg)
	if token != "" {
		req.Header().Set("Authorization", "Bearer "+token)
	}
	return req
}

func bootstrapAuthed[T any](msg *T, token string) *connect.Request[T] {
	req := connect.NewRequest(msg)
	req.Header().Set("Authorization", auth.BootstrapTokenScheme+" "+token)
	return req
}

func (f *fixture) ctx() context.Context { return f.t.Context() }

func (f *fixture) expiredToken(userID, email string) string {
	f.t.Helper()
	past := f.now.Add(-24 * time.Hour)
	m, err := auth.NewJWTManager(auth.JWTConfig{
		PrivateKey: f.signingKey(),
		Now:        func() time.Time { return past },
	})
	require.NoError(f.t, err)
	pair, err := m.GenerateTokens(userID, email, nil, nil, 0)
	require.NoError(f.t, err)
	return pair.AccessToken
}

func (f *fixture) forgedToken(userID, email string, perms []string) string {
	f.t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(f.t, err)
	m, err := auth.NewJWTManager(auth.JWTConfig{
		PrivateKey: priv,
		Now:        func() time.Time { return f.now },
	})
	require.NoError(f.t, err)
	pair, err := m.GenerateTokens(userID, email, perms, nil, 0)
	require.NoError(f.t, err)
	return pair.AccessToken
}

func (f *fixture) signingKey() ed25519.PrivateKey {
	f.t.Helper()
	if f.sessionKey == nil {
		f.t.Fatal("fixture has no session signing key")
	}
	return f.sessionKey
}

type auditOperation struct {
	OperationID          string
	Class                string
	ActorType            string
	ActorID              string
	ActorFingerprint     string
	Origin               string
	OriginFingerprint    string
	RequestDescriptor    string
	AuthorizationOutcome string
	AuthorizationDetail  string
	Result               string
	ResultCode           string
}

type auditEffect struct {
	ResourceType        string
	ResourceID          string
	Action              string
	Outcome             string
	ChangedFields       []string
	BeforeRef           *string
	AfterRef            *string
	BeforeFlag          *bool
	AfterFlag           *bool
	BeforeCount         *int64
	AfterCount          *int64
	EvidenceKind        string
	EvidenceFingerprint string
	SealedDetail        []byte
	SealedDetailSubject *string
}

func (f *fixture) operationsFor(descriptor string) []auditOperation {
	f.t.Helper()
	rows, err := f.raw.Query(f.ctx(),
		`SELECT operation_id, operation_class, actor_type, actor_id, actor_fingerprint,
		        origin, origin_fingerprint, request_descriptor, authorization_outcome,
		        authorization_detail, result, result_code
		   FROM audit_operations WHERE request_descriptor = $1 ORDER BY chain_seq`, descriptor)
	require.NoError(f.t, err)
	defer rows.Close()

	var out []auditOperation
	for rows.Next() {
		var op auditOperation
		require.NoError(f.t, rows.Scan(
			&op.OperationID, &op.Class, &op.ActorType, &op.ActorID, &op.ActorFingerprint,
			&op.Origin, &op.OriginFingerprint, &op.RequestDescriptor, &op.AuthorizationOutcome,
			&op.AuthorizationDetail, &op.Result, &op.ResultCode))
		out = append(out, op)
	}
	require.NoError(f.t, rows.Err())
	return out
}

func (f *fixture) onlyOperationFor(descriptor string) auditOperation {
	f.t.Helper()
	ops := f.operationsFor(descriptor)
	require.Len(f.t, ops, 1, "expected exactly one audit operation for %s", descriptor)
	return ops[0]
}

func (f *fixture) operationOfClass(descriptor, class string) auditOperation {
	f.t.Helper()
	var found []auditOperation
	for _, op := range f.operationsFor(descriptor) {
		if op.Class == class {
			found = append(found, op)
		}
	}
	require.Len(f.t, found, 1, "expected exactly one %s operation for %s", class, descriptor)
	return found[0]
}

func (f *fixture) effectsOf(operationID string) []auditEffect {
	f.t.Helper()
	rows, err := f.raw.Query(f.ctx(),
		`SELECT resource_type, resource_id, action, outcome, changed_fields,
		        before_ref, after_ref, before_flag, after_flag, before_count, after_count,
		        evidence_kind, evidence_fingerprint, sealed_detail, sealed_detail_subject
		   FROM audit_effects WHERE operation_id = $1 ORDER BY effect_seq`, operationID)
	require.NoError(f.t, err)
	defer rows.Close()

	var out []auditEffect
	for rows.Next() {
		var e auditEffect
		require.NoError(f.t, rows.Scan(
			&e.ResourceType, &e.ResourceID, &e.Action, &e.Outcome, &e.ChangedFields,
			&e.BeforeRef, &e.AfterRef, &e.BeforeFlag, &e.AfterFlag, &e.BeforeCount, &e.AfterCount,
			&e.EvidenceKind, &e.EvidenceFingerprint, &e.SealedDetail, &e.SealedDetailSubject))
		out = append(out, e)
	}
	require.NoError(f.t, rows.Err())
	return out
}

func (f *fixture) effectWithAction(effects []auditEffect, action string) auditEffect {
	f.t.Helper()
	var found []auditEffect
	for _, e := range effects {
		if e.Action == action {
			found = append(found, e)
		}
	}
	require.Len(f.t, found, 1, "expected exactly one %s effect, got %d in %+v", action, len(found), effects)
	return found[0]
}

func (f *fixture) countAuditOperations() int64 {
	f.t.Helper()
	var n int64
	require.NoError(f.t, f.raw.QueryRow(f.ctx(), `SELECT COUNT(*) FROM audit_operations`).Scan(&n))
	return n
}

func (f *fixture) openSealedDetail(subjectID string, sealed []byte, field string) (string, error) {
	f.t.Helper()
	var wrapped string
	if err := f.raw.QueryRow(f.ctx(),
		`SELECT wrapped_dek FROM user_encryption_keys WHERE user_id = $1`, subjectID).Scan(&wrapped); err != nil {
		return "", err
	}
	dek, err := crypto.UnwrapDEK(f.kek, subjectID, wrapped)
	if err != nil {
		return "", err
	}
	return dek.OpenField(string(sealed), field)
}

func newULID() string { return ulid.Make().String() }

func sha256Hex(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}

func connectCodeOf(t *testing.T, err error) connect.Code {
	t.Helper()
	require.Error(t, err)
	var cerr *connect.Error
	require.ErrorAs(t, err, &cerr, "expected a connect error, got %v", err)
	return cerr.Code()
}

func discardProviderFactory(context.Context, idp.ProviderConfig) (*idp.OIDCProvider, error) {
	return nil, errNoProviderInTest
}

var errNoProviderInTest = errNoProvider{}

type errNoProvider struct{}

func (errNoProvider) Error() string {
	return "identity test: no OIDC provider is wired in this fixture"
}

func (f *fixture) mintPair(userID, email string) *auth.TokenPair {
	f.t.Helper()
	perms, err := f.store.ListUserPermissions(f.ctx(), userID)
	require.NoError(f.t, err)
	state, err := f.store.GetUserSessionState(f.ctx(), userID)
	require.NoError(f.t, err)
	pair, err := f.jwt.GenerateTokens(userID, email, perms, nil, state.SessionVersion)
	require.NoError(f.t, err)
	return pair
}

func allPermissionKeys() []string {
	all := auth.AllPermissions()
	keys := make([]string, len(all))
	for i, p := range all {
		keys[i] = p.Key
	}
	return keys
}

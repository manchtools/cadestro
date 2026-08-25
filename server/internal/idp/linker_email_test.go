package idp

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manchtools/cadestro/server/internal/crypto"
	"github.com/manchtools/cadestro/server/internal/store"
	"github.com/manchtools/cadestro/server/internal/testdb"
)

const linkerTestKEK = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"

func newLinkerStore(t *testing.T) (*store.Store, *testdb.DB) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "cadestro.db")
	st, err := store.New(ctx, path)
	require.NoError(t, err)
	t.Cleanup(st.Close)
	raw, err := testdb.Open(ctx, path)
	require.NoError(t, err)
	t.Cleanup(raw.Close)
	return st, raw
}

func linkerOp() store.AuditOperation {
	return store.AuditOperation{
		Class:                store.ClassMutation,
		ActorType:            SystemActorSSO,
		Origin:               "rpc",
		RequestDescriptor:    "cadestro.v1.ControlService/CompleteOIDCLogin",
		AuthorizationOutcome: store.AuthorizationAllowed,
		AuthorizationDetail:  "oidc",
		Result:               store.ResultSuccess,
		ResultCode:           "OK",
	}
}

func newTestLinker(t *testing.T, at time.Time) *Linker {
	t.Helper()
	kek, err := crypto.NewEncryptor(linkerTestKEK)
	require.NoError(t, err)
	return NewLinker(kek, func() time.Time { return at })
}

func seedProvider(t *testing.T, raw *testdb.DB, autoCreate, autoLink bool) store.IdentityProviderRow {
	t.Helper()
	id := ulid.Make().String()
	_, err := raw.Exec(context.Background(), `
		INSERT INTO identity_providers (id, name, slug, client_id, issuer_url, auto_create_users, auto_link_by_email)
		VALUES ($1, 'Corp', 'corp', 'client', 'https://issuer.example', $2, $3)`,
		id, autoCreate, autoLink)
	require.NoError(t, err)
	return store.IdentityProviderRow{
		ID: id, Slug: "corp", AutoCreateUsers: autoCreate, AutoLinkByEmail: autoLink,
	}
}

func TestLinker_JITStoresNormalizedEmail(t *testing.T) {
	ctx := context.Background()
	st, raw := newLinkerStore(t)
	at := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	linker := newTestLinker(t, at)

	provider := seedProvider(t, raw, true, false)
	claims := &UserClaims{
		Subject: "ext-jit-1", Email: "First.Last@Company.com",
		Name: "First Last", GivenName: "First", FamilyName: "Last",
	}

	var result *LinkResult
	_, err := st.WithAudit(ctx, linkerOp(), func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		var err error
		result, err = linker.LinkOrCreate(ctx, tx, rec, provider, claims)
		return err
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsNew, "an absent subject is provisioned by JIT")

	user, err := st.GetUserByEmail(ctx, "first.last@company.com")
	require.NoError(t, err, "a normalized lookup must find the JIT-provisioned subject")
	assert.Equal(t, result.UserID, user.ID)
	assert.Equal(t, "first.last@company.com", user.Email, "the JIT write must store the normalized email")
}

func TestLinker_PaddedEmailDerivesUsernameFromNormalizedForm(t *testing.T) {
	ctx := context.Background()
	st, raw := newLinkerStore(t)
	at := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	linker := newTestLinker(t, at)

	provider := seedProvider(t, raw, true, false)
	claims := &UserClaims{Subject: "ext-padded-1", Email: "  Padded.User@Corp.com  ", Name: "Padded User"}

	var result *LinkResult
	_, err := st.WithAudit(ctx, linkerOp(), func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		var e error
		result, e = linker.LinkOrCreate(ctx, tx, rec, provider, claims)
		return e
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	user, err := st.GetUserByEmail(ctx, "padded.user@corp.com")
	require.NoError(t, err, "the padded email must be stored and found in normalized form")
	assert.Equal(t, "padded.user@corp.com", user.Email)
	assert.Equal(t, DeriveLinuxUsername("padded.user@corp.com", ""), user.LinuxUsername,
		"the Linux username must derive from the normalized email, not the padded raw claim")
}

func TestLinker_AutoLinkFindsNormalizedUser(t *testing.T) {
	ctx := context.Background()
	st, raw := newLinkerStore(t)
	at := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	linker := newTestLinker(t, at)

	existingID := ulid.Make().String()
	_, err := raw.Exec(ctx, `INSERT INTO users (id, email) VALUES ($1, $2)`,
		existingID, "first.last@company.com")
	require.NoError(t, err)

	provider := seedProvider(t, raw, false, true)
	claims := &UserClaims{Subject: "ext-link-1", Email: "First.Last@Company.com", Name: "First Last"}

	var result *LinkResult
	_, err = st.WithAudit(ctx, linkerOp(), func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		var err error
		result, err = linker.LinkOrCreate(ctx, tx, rec, provider, claims)
		return err
	})
	require.NoError(t, err, "a case-only-different email must resolve the existing subject, not fail to match")
	require.NotNil(t, result)
	assert.Equal(t, existingID, result.UserID, "auto-link must bind the existing normalized subject")
	assert.False(t, result.IsNew, "auto-link must not provision a duplicate subject")
}

func TestLinker_WhitespaceEmailIsRefused(t *testing.T) {
	ctx := context.Background()
	st, raw := newLinkerStore(t)
	at := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	linker := newTestLinker(t, at)

	provider := seedProvider(t, raw, true, true)
	claims := &UserClaims{Subject: "ext-blank-1", Email: "   ", Name: "Blank"}

	_, err := st.WithAudit(ctx, linkerOp(), func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		_, e := linker.LinkOrCreate(ctx, tx, rec, provider, claims)
		return e
	})
	require.Error(t, err, "a whitespace-only email must not provision or link a subject")

	assert.ErrorIs(t, err, ErrNoMatchingAccount)

	var count int
	require.NoError(t, raw.QueryRow(ctx,
		`SELECT count(*) FROM users WHERE email = ''`).Scan(&count))
	assert.Zero(t, count, "no empty-email subject may be created")

	var links int
	require.NoError(t, raw.QueryRow(ctx,
		`SELECT count(*) FROM identity_links`).Scan(&links))
	assert.Zero(t, links, "no identity link may be created on an empty email")
}

func TestLinker_RefusalNamesTheDisabledAutoCreateFlag(t *testing.T) {
	ctx := context.Background()
	st, raw := newLinkerStore(t)
	linker := newTestLinker(t, time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC))

	disabled := seedProvider(t, raw, false, false)
	_, err := st.WithAudit(ctx, linkerOp(), func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		_, e := linker.LinkOrCreate(ctx, tx, rec, disabled,
			&UserClaims{Subject: "ext-gate-1", Email: "one@example.com", Name: "One"})
		return e
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNoMatchingAccount, "the sentinel must survive wrapping")
	assert.Contains(t, err.Error(), "auto_create_users",
		"a disabled-flag refusal must name the flag")
}

func TestLinker_RefusalNamesTheMissingTrustedEmail(t *testing.T) {
	ctx := context.Background()
	st, raw := newLinkerStore(t)
	linker := newTestLinker(t, time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC))

	enabled := seedProvider(t, raw, true, false)
	_, err := st.WithAudit(ctx, linkerOp(), func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		_, e := linker.LinkOrCreate(ctx, tx, rec, enabled,
			&UserClaims{Subject: "ext-gate-2", Email: "", Name: "Two"})
		return e
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNoMatchingAccount, "the sentinel must survive wrapping")
	assert.Contains(t, err.Error(), "trusted email",
		"a missing-claim refusal must name the claim")
}

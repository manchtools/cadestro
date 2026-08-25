package identity

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

const (
	BootstrapTokenBytes = 32

	DefaultBootstrapTokenTTL = 15 * time.Minute
)

type BootstrapToken struct {
	Token     string
	URL       string
	ExpiresAt time.Time
}

type BootstrapStore interface {
	WithAudit(ctx context.Context, op store.AuditOperation, mutate func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error) (store.AuditRecord, error)
}

type Bootstrapper struct {
	store   BootstrapStore
	baseURL string
	ttl     time.Duration
	now     func() time.Time
}

func NewBootstrapper(st BootstrapStore, baseURL string, ttl time.Duration, now func() time.Time) *Bootstrapper {
	if ttl <= 0 {
		ttl = DefaultBootstrapTokenTTL
	}
	if now == nil {
		now = time.Now
	}
	return &Bootstrapper{store: st, baseURL: baseURL, ttl: ttl, now: now}
}

var ErrBootstrapTokenRejected = errors.New("identity: bootstrap token rejected")

func (b *Bootstrapper) Issue(ctx context.Context) (BootstrapToken, error) {
	raw := make([]byte, BootstrapTokenBytes)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return BootstrapToken{}, fmt.Errorf("generate bootstrap token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	digest := fingerprint(token)

	tokenID := ulid.Make().String()
	issuedAt := b.now().UTC()
	expiresAt := issuedAt.Add(b.ttl)

	_, err := b.store.WithAudit(ctx, store.AuditOperation{
		Class:                store.ClassBackgroundWriter,
		ActorType:            string(auth.PrincipalBootstrapAdmin),
		Origin:               "host_command",
		RequestDescriptor:    "control.bootstrap-admin/Issue",
		AuthorizationOutcome: store.AuthorizationNotApplicable,
		Result:               store.ResultSuccess,
	}, func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		retired, err := tx.RetireBootstrapAdminTokens(ctx, store.BootstrapAdminTokenName)
		if err != nil {
			return err
		}
		if _, err := tx.InsertBootstrapAdminToken(ctx, db.InsertBootstrapAdminTokenParams{
			ID:           tokenID,
			ValueHash:    digest,
			ReservedName: store.BootstrapAdminTokenName,
			ExpiresAt:    expiresAt,
			CreatedAt:    &issuedAt,
			CreatedBy:    auth.BootstrapPrincipalID,
		}); err != nil {
			return err
		}
		if retired > 0 {
			rec.Effect(store.AuditEffect{
				ResourceType: "bootstrap_token",
				ResourceID:   tokenID,
				Action:       "RETIRE_OUTSTANDING",
				Outcome:      store.EffectApplied,
				BeforeCount:  &retired,
			})
		}
		rec.Effect(store.AuditEffect{
			ResourceType:        "bootstrap_token",
			ResourceID:          tokenID,
			Action:              "ISSUE",
			Outcome:             store.EffectApplied,
			ChangedFields:       []string{"value_hash", "expires_at"},
			EvidenceKind:        "bootstrap_token_sha256",
			EvidenceFingerprint: digest,
		})
		return nil
	})
	if err != nil {
		return BootstrapToken{}, fmt.Errorf("issue bootstrap token: %w", err)
	}

	return BootstrapToken{
		Token:     token,
		URL:       b.setupURL(token),
		ExpiresAt: expiresAt,
	}, nil
}

func (b *Bootstrapper) AuthenticateBootstrapToken(ctx context.Context, token string) (*auth.UserContext, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrBootstrapTokenRejected
	}
	digest := fingerprint(token)

	now := b.now().UTC()

	var spent bool
	_, err := b.store.WithAudit(ctx, store.AuditOperation{
		Class:                store.ClassMutation,
		ActorType:            string(auth.PrincipalBootstrapAdmin),
		Origin:               auth.ControlRPCOrigin,
		RequestDescriptor:    "control.bootstrap-admin/Consume",
		AuthorizationOutcome: store.AuthorizationNotApplicable,
		Result:               store.ResultSuccess,
	}, func(ctx context.Context, tx *store.Tx, rec *store.AuditRecorder) error {
		row, err := tx.ConsumeBootstrapAdminToken(ctx, db.ConsumeBootstrapAdminTokenParams{
			ValueHash:    digest,
			ReservedName: store.BootstrapAdminTokenName,
			Now:          now,
		})
		if err != nil {
			if store.IsNotFound(err) {
				return ErrBootstrapTokenRejected
			}
			return err
		}
		spent = true
		rec.Effect(store.AuditEffect{
			ResourceType:        "bootstrap_token",
			ResourceID:          row.ID,
			Action:              "CONSUME",
			Outcome:             store.EffectApplied,
			ChangedFields:       []string{"is_deleted"},
			EvidenceKind:        "bootstrap_token_sha256",
			EvidenceFingerprint: digest,
		})
		return nil
	})
	switch {
	case errors.Is(err, ErrBootstrapTokenRejected):
		return nil, ErrBootstrapTokenRejected
	case err != nil:

		return nil, fmt.Errorf("consume bootstrap token: %w", err)
	case !spent:
		return nil, ErrBootstrapTokenRejected
	}

	return &auth.UserContext{
		ID:   auth.BootstrapPrincipalID,
		Kind: auth.PrincipalBootstrapAdmin,

		Permissions: BootstrapPermissions(),
	}, nil
}

func BootstrapPermissions() []string {
	return []string{
		PermCreateIdentityProvider,
		PermGetIdentityProvider,
		PermListIdentityProviders,
		PermUpdateIdentityProvider,
		PermCreateRole,
		PermGetRole,
		PermListRoles,
		PermListPermissions,
		PermGetUser,
		PermListUsers,
		PermAssignRoleToUser,
	}
}

func (b *Bootstrapper) setupURL(token string) string {
	base := strings.TrimSuffix(b.baseURL, "/")
	fragment := url.Values{"bootstrap_token": {token}}.Encode()
	return base + "/setup#" + fragment
}

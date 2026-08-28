package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	"github.com/oklog/ulid/v2"
	"google.golang.org/protobuf/types/known/timestamppb"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/ca"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

type Config struct {
	Store             *store.Store
	CA                *ca.CA
	JWT               *auth.JWTManager
	Logger            *slog.Logger
	Now               func() time.Time
	PublicBaseURL     string
	AgentURL          string
	CAFingerprint     string
	Version           string
	HeartbeatInterval time.Duration
}

type Service struct {
	cadestrov1connect.UnimplementedControlServiceHandler
	cadestrov1connect.UnimplementedAgentServiceHandler
	store             *store.Store
	ca                *ca.CA
	jwt               *auth.JWTManager
	logger            *slog.Logger
	now               func() time.Time
	publicBaseURL     string
	agentURL          string
	caFingerprint     string
	version           string
	heartbeatInterval time.Duration
}

func New(config Config) *Service {
	if config.Store == nil || config.CA == nil || config.JWT == nil {
		panic("core: store, CA, and JWT manager are required")
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.HeartbeatInterval <= 0 {
		config.HeartbeatInterval = 30 * time.Second
	}
	return &Service{
		store: config.Store, ca: config.CA, jwt: config.JWT,
		logger: config.Logger, now: config.Now, publicBaseURL: config.PublicBaseURL,
		agentURL: config.AgentURL, caFingerprint: config.CAFingerprint, version: config.Version,
		heartbeatInterval: config.HeartbeatInterval,
	}
}

func (service *Service) LookupUser(ctx context.Context, id string) (string, int32, error) {
	user, err := service.store.Queries().GetUser(ctx, id)
	if err != nil {
		return "", 0, err
	}
	return user.Email, int32(user.SessionVersion), nil
}

func (service *Service) internal(operation string, err error) error {
	service.logger.Error("core operation failed", "operation", operation, "error", err)
	return connect.NewError(connect.CodeInternal, errors.New("internal error"))
}

func rpcNotFound(name string) error {
	return connect.NewError(connect.CodeNotFound, fmt.Errorf("%s not found", name))
}

func rpcConflict(name string) error {
	return connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("%s already exists", name))
}

func pageSize(requested int32) int64 {
	if requested <= 0 {
		return 50
	}
	return int64(requested)
}

func nextPageToken[T any](rows []T, limit int64, id func(T) string) string {
	if int64(len(rows)) < limit || len(rows) == 0 {
		return ""
	}
	return id(rows[len(rows)-1])
}

func auditActor(ctx context.Context) string {
	if user, ok := auth.UserFromContext(ctx); ok {
		return user.ID
	}
	return ulid.Make().String()
}

func (service *Service) audit(ctx context.Context, eventType, streamType, streamID, actorType, actorID string) error {
	if actorID == "" {
		actorID = auditActor(ctx)
	}
	return service.store.Queries().CreateAuditEvent(ctx, db.CreateAuditEventParams{
		ID: ulid.Make().String(), EventType: eventType, StreamType: streamType,
		StreamID: streamID, ActorType: actorType, ActorID: actorID, OccurredAt: service.now().UTC(),
	})
}

func userProto(user *db.User) *cadestrov1.User {
	return &cadestrov1.User{
		Id: &cadestrov1.UserId{Value: user.ID}, Email: user.Email, DisplayName: user.DisplayName,
		Picture: user.Picture, CreatedAt: timestamppb.New(user.CreatedAt), LastLoginAt: timestamppb.New(user.LastLoginAt),
	}
}

func providerProto(provider *db.IdentityProvider) (*cadestrov1.IdentityProvider, error) {
	var scopes []string
	if err := json.Unmarshal([]byte(provider.ScopesJson), &scopes); err != nil {
		return nil, fmt.Errorf("decode provider scopes: %w", err)
	}
	return &cadestrov1.IdentityProvider{
		Id: &cadestrov1.IdentityProviderId{Value: provider.ID}, Name: provider.Name, Slug: provider.Slug,
		Enabled: provider.Enabled, ClientId: &cadestrov1.OidcClientId{Value: provider.ClientID},
		IssuerUrl: provider.IssuerUrl, Scopes: scopes, CreatedAt: timestamppb.New(provider.CreatedAt), UpdatedAt: timestamppb.New(provider.UpdatedAt),
	}, nil
}

func registrationTokenProto(token *db.RegistrationToken) *cadestrov1.RegistrationToken {
	return &cadestrov1.RegistrationToken{
		Id: &cadestrov1.RegistrationTokenId{Value: token.ID}, Name: token.Name,
		MaxUses: int32(token.MaxUses), CurrentUses: int32(token.CurrentUses),
		ExpiresAt: timestamppb.New(token.ExpiresAt), CreatedAt: timestamppb.New(token.CreatedAt), Disabled: token.Disabled,
	}
}

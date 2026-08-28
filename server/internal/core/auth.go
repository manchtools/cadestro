package core

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/oklog/ulid/v2"
	"google.golang.org/protobuf/types/known/timestamppb"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/server/internal/auth"
	"github.com/manchtools/cadestro/server/internal/idp"
	"github.com/manchtools/cadestro/server/internal/store"
	db "github.com/manchtools/cadestro/server/internal/store/generated"
)

const authStateTTL = 10 * time.Minute

type BootstrapProvider struct {
	Name      string
	Slug      string
	ClientID  string
	IssuerURL string
	Scopes    []string
}

func (service *Service) EnsureBootstrapProvider(ctx context.Context, config BootstrapProvider) error {
	count, err := service.store.Queries().CountIdentityProviders(ctx)
	if err != nil || count > 0 {
		return err
	}
	if config.IssuerURL == "" || config.ClientID == "" {
		return errors.New("bootstrap OIDC issuer and client ID are required for a new database")
	}
	if config.Name == "" {
		config.Name = "Company SSO"
	}
	if config.Slug == "" {
		config.Slug = "sso"
	}
	id := ulid.Make().String()
	if _, err := service.newOIDCProvider(ctx, config.ClientID, config.IssuerURL, config.Scopes, service.publicBaseURL); err != nil {
		return err
	}
	scopes, err := json.Marshal(config.Scopes)
	if err != nil {
		return err
	}
	now := service.now().UTC()
	_, err = service.store.Queries().CreateIdentityProvider(ctx, db.CreateIdentityProviderParams{
		ID: id, Name: config.Name, Slug: config.Slug, Enabled: true, ClientID: config.ClientID,
		IssuerUrl: config.IssuerURL, ScopesJson: string(scopes), CreatedAt: now, UpdatedAt: now,
	})
	return err
}

func (service *Service) RefreshToken(ctx context.Context, request *connect.Request[cadestrov1.RefreshTokenRequest]) (*connect.Response[cadestrov1.RefreshTokenResponse], error) {
	claims, err := service.jwt.ValidateToken(request.Msg.GetRefreshToken(), auth.TokenTypeRefresh)
	if err != nil || claims.ID == "" || claims.ExpiresAt == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid refresh token"))
	}
	user, err := service.store.Queries().GetUser(ctx, claims.UserID)
	if err != nil || user.SessionVersion != int64(claims.SessionVersion) {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid refresh token"))
	}
	rows, err := service.store.Queries().CreateRevokedToken(ctx, db.CreateRevokedTokenParams{ID: claims.ID, ExpiresAt: claims.ExpiresAt.Time})
	if err != nil {
		return nil, service.internal("revoke refresh token", err)
	}
	if rows != 1 {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("refresh token already used"))
	}
	pair, err := service.jwt.GenerateTokens(user.ID, user.Email, int32(user.SessionVersion))
	if err != nil {
		return nil, service.internal("mint session", err)
	}
	return connect.NewResponse(&cadestrov1.RefreshTokenResponse{
		AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken, ExpiresAt: timestamppb.New(pair.ExpiresAt),
	}), nil
}

func (service *Service) Logout(ctx context.Context, request *connect.Request[cadestrov1.LogoutRequest]) (*connect.Response[cadestrov1.LogoutResponse], error) {
	claims, err := service.jwt.ValidateToken(request.Msg.GetRefreshToken(), auth.TokenTypeRefresh)
	if err != nil || claims.ID == "" || claims.ExpiresAt == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid refresh token"))
	}
	if _, err := service.store.Queries().CreateRevokedToken(ctx, db.CreateRevokedTokenParams{ID: claims.ID, ExpiresAt: claims.ExpiresAt.Time}); err != nil {
		return nil, service.internal("revoke session", err)
	}
	return connect.NewResponse(&cadestrov1.LogoutResponse{}), nil
}

func (service *Service) GetCurrentUser(ctx context.Context, _ *connect.Request[cadestrov1.GetCurrentUserRequest]) (*connect.Response[cadestrov1.GetCurrentUserResponse], error) {
	principal, ok := auth.UserFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	user, err := service.store.Queries().GetUser(ctx, principal.ID)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("user")
		}
		return nil, service.internal("get current user", err)
	}
	return connect.NewResponse(&cadestrov1.GetCurrentUserResponse{User: userProto(user)}), nil
}

func (service *Service) ListAuthMethods(ctx context.Context, _ *connect.Request[cadestrov1.ListAuthMethodsRequest]) (*connect.Response[cadestrov1.ListAuthMethodsResponse], error) {
	providers, err := service.store.Queries().ListEnabledIdentityProviders(ctx)
	if err != nil {
		return nil, service.internal("list auth methods", err)
	}
	response := &cadestrov1.ListAuthMethodsResponse{}
	for _, provider := range providers {
		response.Providers = append(response.Providers, &cadestrov1.AuthMethodProvider{Slug: provider.Slug, Name: provider.Name})
	}
	return connect.NewResponse(response), nil
}

func (service *Service) GetSSOLoginURL(ctx context.Context, request *connect.Request[cadestrov1.GetSSOLoginURLRequest]) (*connect.Response[cadestrov1.GetSSOLoginURLResponse], error) {
	if err := service.validateRedirect(request.Msg.GetRedirectUrl()); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	provider, err := service.store.Queries().GetIdentityProviderBySlug(ctx, request.Msg.GetSlug())
	if err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("identity provider")
		}
		return nil, service.internal("get login provider", err)
	}
	if !provider.Enabled {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("identity provider is disabled"))
	}
	oidcProvider, err := service.providerClient(ctx, provider, request.Msg.GetRedirectUrl())
	if err != nil {
		return nil, service.internal("initialize OIDC provider", err)
	}
	state, err := idp.GenerateState()
	if err != nil {
		return nil, service.internal("generate OIDC state", err)
	}
	nonce, err := idp.GenerateNonce()
	if err != nil {
		return nil, service.internal("generate OIDC nonce", err)
	}
	verifier, err := idp.GenerateCodeVerifier()
	if err != nil {
		return nil, service.internal("generate PKCE verifier", err)
	}
	now := service.now().UTC()
	if err := service.store.Queries().DeleteExpiredAuthStates(ctx, now); err != nil {
		return nil, service.internal("delete expired auth states", err)
	}
	if err := service.store.Queries().CreateAuthState(ctx, db.CreateAuthStateParams{
		State: state, ProviderID: provider.ID, Nonce: nonce, CodeVerifier: verifier,
		RedirectUrl: request.Msg.GetRedirectUrl(), ExpiresAt: now.Add(authStateTTL),
	}); err != nil {
		return nil, service.internal("store auth state", err)
	}
	return connect.NewResponse(&cadestrov1.GetSSOLoginURLResponse{LoginUrl: oidcProvider.AuthCodeURL(state, nonce, verifier)}), nil
}

func (service *Service) SSOCallback(ctx context.Context, request *connect.Request[cadestrov1.SSOCallbackRequest]) (*connect.Response[cadestrov1.SSOCallbackResponse], error) {
	state, err := service.store.Queries().GetAuthState(ctx, request.Msg.GetState())
	if err != nil || !state.ExpiresAt.After(service.now()) {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("SSO state is invalid or expired"))
	}
	provider, err := service.store.Queries().GetIdentityProvider(ctx, state.ProviderID)
	if err != nil || provider.Slug != request.Msg.GetSlug() || !provider.Enabled {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("SSO state is invalid or expired"))
	}
	deleted, err := service.store.Queries().DeleteAuthState(ctx, state.State)
	if err != nil {
		return nil, service.internal("consume auth state", err)
	}
	if deleted != 1 {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("SSO state is invalid or expired"))
	}
	oidcProvider, err := service.providerClient(ctx, provider, state.RedirectUrl)
	if err != nil {
		return nil, service.internal("initialize callback provider", err)
	}
	oauthToken, err := oidcProvider.ExchangeCode(ctx, request.Msg.GetCode(), state.CodeVerifier)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("SSO exchange failed"))
	}
	claims, err := oidcProvider.VerifyAndExtractClaims(ctx, oauthToken, state.Nonce)
	if err != nil || claims.Subject == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("SSO identity verification failed"))
	}
	user, err := service.linkIdentity(ctx, provider.ID, claims)
	if err != nil {
		return nil, service.internal("link OIDC identity", err)
	}
	pair, err := service.jwt.GenerateTokens(user.ID, user.Email, int32(user.SessionVersion))
	if err != nil {
		return nil, service.internal("mint OIDC session", err)
	}
	return connect.NewResponse(&cadestrov1.SSOCallbackResponse{
		AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken,
		ExpiresAt: timestamppb.New(pair.ExpiresAt), User: userProto(user),
	}), nil
}

func (service *Service) linkIdentity(ctx context.Context, providerID string, claims *idp.UserClaims) (*db.User, error) {
	var linked *db.User
	err := service.store.Transaction(ctx, func(queries *db.Queries) error {
		user, err := queries.GetIdentityUser(ctx, db.GetIdentityUserParams{ProviderID: providerID, Subject: claims.Subject})
		if err == nil {
			linked, err = queries.UpdateUserLogin(ctx, db.UpdateUserLoginParams{
				Email: claims.Email, DisplayName: claims.Name, Picture: claims.Picture, LastLoginAt: service.now().UTC(), ID: user.ID,
			})
			return err
		}
		if !store.IsNotFound(err) {
			return err
		}
		now := service.now().UTC()
		displayName := claims.Name
		if displayName == "" {
			displayName = claims.Email
		}
		linked, err = queries.CreateUser(ctx, db.CreateUserParams{
			ID: ulid.Make().String(), Email: claims.Email, DisplayName: displayName,
			Picture: claims.Picture, CreatedAt: now, LastLoginAt: now,
		})
		if err != nil {
			return err
		}
		return queries.LinkIdentity(ctx, db.LinkIdentityParams{ProviderID: providerID, Subject: claims.Subject, UserID: linked.ID})
	})
	return linked, err
}

func (service *Service) CreateIdentityProvider(ctx context.Context, request *connect.Request[cadestrov1.CreateIdentityProviderRequest]) (*connect.Response[cadestrov1.CreateIdentityProviderResponse], error) {
	id := ulid.Make().String()
	if _, err := service.newOIDCProvider(ctx, request.Msg.GetClientId().GetValue(), request.Msg.GetIssuerUrl(), request.Msg.GetScopes(), service.publicBaseURL); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("identity provider discovery failed"))
	}
	scopes, err := json.Marshal(request.Msg.GetScopes())
	if err != nil {
		return nil, service.internal("encode provider scopes", err)
	}
	now := service.now().UTC()
	provider, err := service.store.Queries().CreateIdentityProvider(ctx, db.CreateIdentityProviderParams{
		ID: id, Name: request.Msg.GetName(), Slug: request.Msg.GetSlug(), Enabled: true,
		ClientID: request.Msg.GetClientId().GetValue(), IssuerUrl: request.Msg.GetIssuerUrl(),
		ScopesJson: string(scopes), CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		if store.IsConflict(err) {
			return nil, rpcConflict("identity provider slug")
		}
		return nil, service.internal("create identity provider", err)
	}
	if err := service.audit(ctx, "identity_provider.created", "identity_provider", id, "user", ""); err != nil {
		return nil, service.internal("audit provider creation", err)
	}
	result, err := providerProto(provider)
	if err != nil {
		return nil, service.internal("map identity provider", err)
	}
	return connect.NewResponse(&cadestrov1.CreateIdentityProviderResponse{Provider: result}), nil
}

func (service *Service) GetIdentityProvider(ctx context.Context, request *connect.Request[cadestrov1.GetIdentityProviderRequest]) (*connect.Response[cadestrov1.GetIdentityProviderResponse], error) {
	provider, err := service.store.Queries().GetIdentityProvider(ctx, request.Msg.GetId().GetValue())
	if err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("identity provider")
		}
		return nil, service.internal("get identity provider", err)
	}
	result, err := providerProto(provider)
	if err != nil {
		return nil, service.internal("map identity provider", err)
	}
	return connect.NewResponse(&cadestrov1.GetIdentityProviderResponse{Provider: result}), nil
}

func (service *Service) ListIdentityProviders(ctx context.Context, _ *connect.Request[cadestrov1.ListIdentityProvidersRequest]) (*connect.Response[cadestrov1.ListIdentityProvidersResponse], error) {
	providers, err := service.store.Queries().ListIdentityProviders(ctx)
	if err != nil {
		return nil, service.internal("list identity providers", err)
	}
	response := &cadestrov1.ListIdentityProvidersResponse{}
	for _, provider := range providers {
		mapped, err := providerProto(provider)
		if err != nil {
			return nil, service.internal("map identity provider", err)
		}
		response.Providers = append(response.Providers, mapped)
	}
	return connect.NewResponse(response), nil
}

func (service *Service) UpdateIdentityProvider(ctx context.Context, request *connect.Request[cadestrov1.UpdateIdentityProviderRequest]) (*connect.Response[cadestrov1.UpdateIdentityProviderResponse], error) {
	id := request.Msg.GetId().GetValue()
	_, err := service.store.Queries().GetIdentityProvider(ctx, id)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("identity provider")
		}
		return nil, service.internal("get provider for update", err)
	}
	if _, err := service.newOIDCProvider(ctx, request.Msg.GetClientId().GetValue(), request.Msg.GetIssuerUrl(), request.Msg.GetScopes(), service.publicBaseURL); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("identity provider discovery failed"))
	}
	scopes, err := json.Marshal(request.Msg.GetScopes())
	if err != nil {
		return nil, service.internal("encode provider scopes", err)
	}
	provider, err := service.store.Queries().UpdateIdentityProvider(ctx, db.UpdateIdentityProviderParams{
		Name: request.Msg.GetName(), Enabled: request.Msg.GetEnabled(), ClientID: request.Msg.GetClientId().GetValue(),
		IssuerUrl: request.Msg.GetIssuerUrl(), ScopesJson: string(scopes),
		UpdatedAt: service.now().UTC(), ID: id,
	})
	if err != nil {
		return nil, service.internal("update identity provider", err)
	}
	mapped, err := providerProto(provider)
	if err != nil {
		return nil, service.internal("map identity provider", err)
	}
	return connect.NewResponse(&cadestrov1.UpdateIdentityProviderResponse{Provider: mapped}), nil
}

func (service *Service) DeleteIdentityProvider(ctx context.Context, request *connect.Request[cadestrov1.DeleteIdentityProviderRequest]) (*connect.Response[cadestrov1.DeleteIdentityProviderResponse], error) {
	count, err := service.store.Queries().CountIdentityProviders(ctx)
	if err != nil {
		return nil, service.internal("count identity providers", err)
	}
	if count <= 1 {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("cannot delete the last identity provider"))
	}
	rows, err := service.store.Queries().DeleteIdentityProvider(ctx, request.Msg.GetId().GetValue())
	if err != nil {
		return nil, service.internal("delete identity provider", err)
	}
	if rows != 1 {
		return nil, rpcNotFound("identity provider")
	}
	return connect.NewResponse(&cadestrov1.DeleteIdentityProviderResponse{}), nil
}

func (service *Service) providerClient(ctx context.Context, provider *db.IdentityProvider, redirectURL string) (*idp.OIDCProvider, error) {
	var scopes []string
	if err := json.Unmarshal([]byte(provider.ScopesJson), &scopes); err != nil {
		return nil, err
	}
	return service.newOIDCProvider(ctx, provider.ClientID, provider.IssuerUrl, scopes, redirectURL)
}

func (service *Service) newOIDCProvider(ctx context.Context, clientID, issuerURL string, scopes []string, redirectURL string) (*idp.OIDCProvider, error) {
	return idp.NewOIDCProvider(ctx, idp.ProviderConfig{
		IssuerURL: issuerURL, ClientID: clientID, Scopes: scopes, RedirectURL: redirectURL,
	})
}

func (service *Service) validateRedirect(raw string) error {
	base, err := url.Parse(service.publicBaseURL)
	if err != nil {
		return errors.New("server public URL is invalid")
	}
	redirect, err := url.Parse(raw)
	if err != nil || redirect.Scheme != base.Scheme || !strings.EqualFold(redirect.Host, base.Host) || redirect.User != nil || redirect.Fragment != "" {
		return errors.New("redirect URL must use the configured Cadestro origin")
	}
	return nil
}

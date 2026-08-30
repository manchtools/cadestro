package core

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
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
const oidcTransactionCookieName = "__Host-cadestro-oidc"

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
	_, err = service.store.Queries().CreateIdentityProvider(ctx, db.CreateIdentityProviderParams{
		ID: id, Name: config.Name, Slug: config.Slug, Enabled: true, ClientID: config.ClientID,
		IssuerUrl: config.IssuerURL, ScopesJson: string(scopes),
	})
	return err
}

func (service *Service) RefreshToken(ctx context.Context, request *connect.Request[cadestrov1.RefreshTokenRequest]) (*connect.Response[cadestrov1.RefreshTokenResponse], error) {
	claims, err := service.jwt.ValidateToken(request.Msg.GetRefreshToken(), auth.TokenTypeRefresh)
	if err != nil || claims.ID == "" || claims.ExpiresAt == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid refresh token"))
	}
	user, err := service.store.Queries().RotateUserSession(ctx, db.RotateUserSessionParams{ID: claims.UserID, SessionVersion: claims.SessionVersion})
	if err != nil {
		if store.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid refresh token"))
		}
		return nil, service.internal("rotate refresh session", err)
	}
	permissions, err := service.store.Queries().ListUserPermissions(ctx, user.ID)
	if err != nil {
		return nil, service.internal("load session permissions", err)
	}
	pair, err := service.jwt.GenerateTokens(user.ID, user.Email, user.SessionVersion, permissions)
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
	if _, err := service.store.Queries().RotateUserSession(ctx, db.RotateUserSessionParams{ID: claims.UserID, SessionVersion: claims.SessionVersion}); err != nil {
		if store.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid refresh token"))
		}
		return nil, service.internal("rotate logout session", err)
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
	value, err := service.userProto(ctx, user)
	if err != nil {
		return nil, service.internal("read current user roles", err)
	}
	return connect.NewResponse(&cadestrov1.GetCurrentUserResponse{User: value}), nil
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
	transaction, err := service.jwt.SignOIDCTransaction(state, provider.ID, nonce, verifier, request.Msg.GetRedirectUrl(), now.Add(authStateTTL))
	if err != nil {
		return nil, service.internal("sign OIDC transaction", err)
	}
	response := connect.NewResponse(&cadestrov1.GetSSOLoginURLResponse{LoginUrl: oidcProvider.AuthCodeURL(state, nonce, verifier)})
	response.Header().Add("Set-Cookie", oidcTransactionCookie(transaction, authStateTTL).String())
	return response, nil
}

func (service *Service) SSOCallback(ctx context.Context, request *connect.Request[cadestrov1.SSOCallbackRequest]) (*connect.Response[cadestrov1.SSOCallbackResponse], error) {
	cookie, err := (&http.Request{Header: request.Header()}).Cookie(oidcTransactionCookieName)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("SSO state is invalid or expired"))
	}
	transaction, err := service.jwt.ValidateOIDCTransaction(cookie.Value)
	if err != nil || transaction.State != request.Msg.GetState() {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("SSO state is invalid or expired"))
	}
	provider, err := service.store.Queries().GetIdentityProvider(ctx, transaction.ProviderID)
	if err != nil || provider.Slug != request.Msg.GetSlug() || !provider.Enabled {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("SSO state is invalid or expired"))
	}
	clearOIDCTransactionCookie(ctx)
	oidcProvider, err := service.providerClient(ctx, provider, transaction.RedirectURL)
	if err != nil {
		return nil, service.internal("initialize callback provider", err)
	}
	oauthToken, err := oidcProvider.ExchangeCode(ctx, request.Msg.GetCode(), transaction.CodeVerifier)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("SSO exchange failed"))
	}
	claims, err := oidcProvider.VerifyAndExtractClaims(ctx, oauthToken, transaction.Nonce)
	if err != nil || claims.Subject == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("SSO identity verification failed"))
	}
	user, err := service.linkIdentity(ctx, provider.ID, claims)
	if err != nil {
		return nil, service.internal("link OIDC identity", err)
	}
	permissions, err := service.store.Queries().ListUserPermissions(ctx, user.ID)
	if err != nil {
		return nil, service.internal("load OIDC permissions", err)
	}
	pair, err := service.jwt.GenerateTokens(user.ID, user.Email, user.SessionVersion, permissions)
	if err != nil {
		return nil, service.internal("mint OIDC session", err)
	}
	value, err := service.userProto(ctx, user)
	if err != nil {
		return nil, service.internal("read OIDC user roles", err)
	}
	return connect.NewResponse(&cadestrov1.SSOCallbackResponse{
		AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken,
		ExpiresAt: timestamppb.New(pair.ExpiresAt), User: value,
	}), nil
}

func oidcTransactionCookie(value string, maxAge time.Duration) *http.Cookie {
	seconds := int(maxAge.Seconds())
	if maxAge < 0 {
		seconds = -1
	}
	return &http.Cookie{Name: oidcTransactionCookieName, Value: value, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteNoneMode, MaxAge: seconds}
}

func clearOIDCTransactionCookie(ctx context.Context) {
	if callInfo, ok := connect.CallInfoForHandlerContext(ctx); ok {
		callInfo.ResponseHeader().Add("Set-Cookie", oidcTransactionCookie("", -1).String())
	}
}

func (service *Service) linkIdentity(ctx context.Context, providerID string, claims *idp.UserClaims) (*db.User, error) {
	var linked *db.User
	err := service.store.Transaction(ctx, func(queries *db.Queries) error {
		user, err := queries.GetIdentityUser(ctx, db.GetIdentityUserParams{ProviderID: providerID, Subject: claims.Subject})
		if err == nil {
			linked, err = queries.UpdateUserLogin(ctx, db.UpdateUserLoginParams{
				Email: claims.Email, DisplayName: claims.Name, LastLoginAt: service.now().UTC(), ID: user.ID,
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
			LastLoginAt: now,
		})
		if err != nil {
			return err
		}
		if err := queries.LinkIdentity(ctx, db.LinkIdentityParams{ProviderID: providerID, Subject: claims.Subject, UserID: linked.ID}); err != nil {
			return err
		}
		count, err := queries.CountUsers(ctx)
		if err != nil {
			return err
		}
		roleID := usersRoleID
		if count == 1 {
			roleID = administratorsRoleID
		}
		if _, err := queries.GetRole(ctx, roleID); store.IsNotFound(err) {
			return nil
		} else if err != nil {
			return err
		}
		if err := queries.AssignRoleToUser(ctx, db.AssignRoleToUserParams{UserID: linked.ID, RoleID: roleID}); err != nil {
			return err
		}
		return nil
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
	provider, err := service.store.Queries().CreateIdentityProvider(ctx, db.CreateIdentityProviderParams{
		ID: id, Name: request.Msg.GetName(), Slug: request.Msg.GetSlug(), Enabled: true,
		ClientID: request.Msg.GetClientId().GetValue(), IssuerUrl: request.Msg.GetIssuerUrl(),
		ScopesJson: string(scopes),
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

func (service *Service) RenameIdentityProvider(ctx context.Context, request *connect.Request[cadestrov1.RenameIdentityProviderRequest]) (*connect.Response[cadestrov1.RenameIdentityProviderResponse], error) {
	id := request.Msg.GetId().GetValue()
	provider, err := service.store.Queries().RenameIdentityProvider(ctx, db.RenameIdentityProviderParams{Name: request.Msg.GetName(), ID: id})
	if err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("identity provider")
		}
		return nil, service.internal("rename identity provider", err)
	}
	mapped, err := providerProto(provider)
	if err != nil {
		return nil, service.internal("map identity provider", err)
	}
	return connect.NewResponse(&cadestrov1.RenameIdentityProviderResponse{Provider: mapped}), nil
}

func (service *Service) ConfigureIdentityProvider(ctx context.Context, request *connect.Request[cadestrov1.ConfigureIdentityProviderRequest]) (*connect.Response[cadestrov1.ConfigureIdentityProviderResponse], error) {
	id := request.Msg.GetId().GetValue()
	_, err := service.store.Queries().GetIdentityProvider(ctx, id)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("identity provider")
		}
		return nil, service.internal("get provider for configure", err)
	}
	if _, err := service.newOIDCProvider(ctx, request.Msg.GetClientId().GetValue(), request.Msg.GetIssuerUrl(), request.Msg.GetScopes(), service.publicBaseURL); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("identity provider discovery failed"))
	}
	scopes, err := json.Marshal(request.Msg.GetScopes())
	if err != nil {
		return nil, service.internal("encode provider scopes", err)
	}
	provider, err := service.store.Queries().ConfigureIdentityProvider(ctx, db.ConfigureIdentityProviderParams{
		ClientID:  request.Msg.GetClientId().GetValue(),
		IssuerUrl: request.Msg.GetIssuerUrl(), ScopesJson: string(scopes),
		ID: id,
	})
	if err != nil {
		return nil, service.internal("configure identity provider", err)
	}
	mapped, err := providerProto(provider)
	if err != nil {
		return nil, service.internal("map identity provider", err)
	}
	return connect.NewResponse(&cadestrov1.ConfigureIdentityProviderResponse{Provider: mapped}), nil
}

func (service *Service) EnableIdentityProvider(ctx context.Context, request *connect.Request[cadestrov1.EnableIdentityProviderRequest]) (*connect.Response[cadestrov1.EnableIdentityProviderResponse], error) {
	id := request.Msg.GetId().GetValue()
	current, err := service.store.Queries().GetIdentityProvider(ctx, id)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("identity provider")
		}
		return nil, service.internal("get provider for enable", err)
	}
	var scopes []string
	if err := json.Unmarshal([]byte(current.ScopesJson), &scopes); err != nil {
		return nil, service.internal("decode provider scopes", err)
	}
	if _, err := service.newOIDCProvider(ctx, current.ClientID, current.IssuerUrl, scopes, service.publicBaseURL); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("identity provider discovery failed"))
	}
	provider, err := service.store.Queries().EnableIdentityProvider(ctx, id)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("identity provider")
		}
		return nil, service.internal("enable identity provider", err)
	}
	mapped, err := providerProto(provider)
	if err != nil {
		return nil, service.internal("map identity provider", err)
	}
	return connect.NewResponse(&cadestrov1.EnableIdentityProviderResponse{Provider: mapped}), nil
}

func (service *Service) DisableIdentityProvider(ctx context.Context, request *connect.Request[cadestrov1.DisableIdentityProviderRequest]) (*connect.Response[cadestrov1.DisableIdentityProviderResponse], error) {
	provider, err := service.store.Queries().DisableIdentityProvider(ctx, request.Msg.GetId().GetValue())
	if err != nil {
		if store.IsNotFound(err) {
			return nil, rpcNotFound("identity provider")
		}
		return nil, service.internal("disable identity provider", err)
	}
	mapped, err := providerProto(provider)
	if err != nil {
		return nil, service.internal("map identity provider", err)
	}
	return connect.NewResponse(&cadestrov1.DisableIdentityProviderResponse{Provider: mapped}), nil
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

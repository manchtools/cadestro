package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
	"github.com/manchtools/cadestro/server/internal/middleware"
	"github.com/manchtools/cadestro/server/internal/store"
)

const (
	errRateLimited      = cadestrov1.ErrorCode_ERROR_CODE_RATE_LIMITED
	errNotAuthenticated = cadestrov1.ErrorCode_ERROR_CODE_NOT_AUTHENTICATED
	errTokenExpired     = cadestrov1.ErrorCode_ERROR_CODE_TOKEN_EXPIRED
	errPermissionDenied = cadestrov1.ErrorCode_ERROR_CODE_PERMISSION_DENIED
)

const ControlProcedurePrefix = "/" + cadestrov1connect.ControlServiceName + "/"

var PublicProcedures = map[string]bool{
	cadestrov1connect.ControlServiceRefreshTokenProcedure:     true,
	cadestrov1connect.ControlServiceLogoutProcedure:           true,
	cadestrov1connect.ControlServiceRegisterProcedure:         true,
	cadestrov1connect.ControlServiceRenewCertificateProcedure: true,
	cadestrov1connect.ControlServiceListAuthMethodsProcedure:  true,
	cadestrov1connect.ControlServiceGetSSOLoginURLProcedure:   true,
	cadestrov1connect.ControlServiceSSOCallbackProcedure:      true,
}

var procedureAlternatives = map[string][]string{
	cadestrov1connect.ControlServiceCreateDeviceGroupProcedure: {
		"CreateStaticDeviceGroup",
		"CreateDynamicDeviceGroup",
	},
	cadestrov1connect.ControlServiceCreateUserGroupProcedure: {
		"CreateStaticUserGroup",
		"CreateDynamicUserGroup",
	},

	cadestrov1connect.ControlServiceUpdateDeviceGroupQueryProcedure: {
		"UpdateDynamicDeviceGroupQuery",
	},
	cadestrov1connect.ControlServiceUpdateUserGroupQueryProcedure: {
		"UpdateDynamicUserGroupQuery",
	},

	cadestrov1connect.ControlServiceExportAuditEventsProcedure: {
		"ListAuditEvents",
	},
}

func ProcedureAlternativesSnapshot() map[string][]string {
	out := make(map[string][]string, len(procedureAlternatives))
	for k, v := range procedureAlternatives {
		out[k] = append([]string(nil), v...)
	}
	return out
}

func PermissionIsAlternative(permKey string) bool {
	for _, alts := range procedureAlternatives {
		for _, perm := range alts {
			if perm == permKey {
				return true
			}
		}
	}
	return false
}

var TrustedProxies []*net.IPNet

func SetTrustedProxies(cidrs []string) {
	var nets []*net.IPNet
	for _, cidr := range cidrs {
		if !strings.Contains(cidr, "/") {
			ip := net.ParseIP(cidr)
			if ip == nil {
				continue
			}
			bits := 32
			if ip.To4() == nil {
				bits = 128
			}
			nets = append(nets, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			continue
		}
		if _, ipNet, err := net.ParseCIDR(cidr); err == nil {
			nets = append(nets, ipNet)
		}
	}
	TrustedProxies = nets
}

func isTrustedProxy(addr string) bool {
	if len(TrustedProxies) == 0 {
		return false
	}
	ip := net.ParseIP(addr)
	if ip == nil {
		return false
	}
	for _, n := range TrustedProxies {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func resolveClientIP(peerIP, xff, xri string) string {
	if !isTrustedProxy(peerIP) {
		return peerIP
	}
	if xff != "" {
		hops := strings.Split(xff, ",")
		for i := len(hops) - 1; i >= 0; i-- {
			hop := strings.TrimSpace(hops[i])
			if net.ParseIP(hop) == nil {
				return peerIP
			}
			if isTrustedProxy(hop) {
				continue
			}
			return hop
		}
		return peerIP
	}
	if xri != "" {
		if ip := strings.TrimSpace(xri); net.ParseIP(ip) != nil {
			return ip
		}
	}
	return peerIP
}

func ClientIPFromHTTP(r *http.Request) string {
	peerIP := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		peerIP = host
	}
	resolved := resolveClientIP(peerIP, r.Header.Get("X-Forwarded-For"), r.Header.Get("X-Real-IP"))
	if net.ParseIP(resolved) != nil {
		return resolved
	}
	return ""
}

func clientIP(req connect.AnyRequest) string {
	peerAddr := req.Peer().Addr
	peerIP := peerAddr
	if host, _, err := net.SplitHostPort(peerAddr); err == nil {
		peerIP = host
	}
	resolved := resolveClientIP(peerIP, req.Header().Get("X-Forwarded-For"), req.Header().Get("X-Real-IP"))
	if net.ParseIP(resolved) != nil {
		return resolved
	}
	return ""
}

func ClientIP(req connect.AnyRequest) string { return clientIP(req) }

type RateLimiters struct {
	SSOCallback *RateLimiter

	Refresh *RateLimiter

	Register  *RateLimiter
	RenewCert *RateLimiter

	Logout *RateLimiter

	AuthMethods *RateLimiter

	SSO *RateLimiter

	Authenticated *RateLimiter

	Expensive *RateLimiter

	Rejected *RateLimiter
}

var expensiveProcedureActions = map[string]bool{
	"DispatchOSQuery":          true,
	"EvaluateDynamicGroup":     true,
	"EvaluateDynamicUserGroup": true,
	"ExportAuditEvents":        true,
	"QueryDeviceLogs":          true,
	"RebuildSearchIndex":       true,
	"Search":                   true,
	"UpdateDeviceGroupQuery":   true,
	"UpdateUserGroupQuery":     true,
	"ValidateDynamicQuery":     true,
	"ValidateUserGroupQuery":   true,
}

func isExpensiveProcedure(action string) bool {
	return expensiveProcedureActions[action]
}

func IsExpensiveProcedure(action string) bool { return isExpensiveProcedure(action) }

func ProcedureAction(procedure string) string {
	parts := strings.Split(procedure, "/")
	return parts[len(parts)-1]
}

type RejectionRecorder interface {
	RecordRejectedAuthentication(ctx context.Context, att RejectedAuthentication) error
}

type RejectedAuthentication struct {
	Procedure string

	Reason string

	CredentialFingerprint string

	OriginFingerprint string
}

func Fingerprint(v string) string {
	if v == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}

type AuthInterceptor struct {
	logger     *slog.Logger
	jwtManager *JWTManager
	limiters   RateLimiters
	rejections RejectionRecorder

	bootstrap BootstrapAuthenticator
	apiTokens *store.Store
}

type BootstrapAuthenticator interface {
	AuthenticateBootstrapToken(ctx context.Context, token string) (*UserContext, error)
}

func NewAuthInterceptor(logger *slog.Logger, jwtManager *JWTManager, limiters RateLimiters, rejections RejectionRecorder) *AuthInterceptor {
	return &AuthInterceptor{logger: logger, jwtManager: jwtManager, limiters: limiters, rejections: rejections}
}

func (i *AuthInterceptor) WithBootstrapAuthenticator(b BootstrapAuthenticator) *AuthInterceptor {
	i.bootstrap = b
	return i
}

func (i *AuthInterceptor) WithAPITokenStore(st *store.Store) *AuthInterceptor {
	i.apiTokens = st
	return i
}

const BootstrapTokenScheme = "Cadestro-Bootstrap"

func (i *AuthInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		procedure := req.Spec().Procedure

		if err := i.applyPublicLimiters(ctx, procedure, req); err != nil {
			return nil, err
		}

		if PublicProcedures[procedure] {
			return next(ctx, req)
		}

		scheme, credential, err := parseAuthorization(req.Header().Get("Authorization"))
		if err != nil {
			return nil, i.rejectAuthentication(ctx, req, procedure, errNotAuthenticated, "",
				connect.CodeUnauthenticated, err.Error())
		}

		if strings.EqualFold(scheme, BootstrapTokenScheme) {
			return i.authenticateBootstrap(ctx, next, req, procedure, credential)
		}
		if !strings.EqualFold(scheme, "Bearer") {
			return nil, i.rejectAuthentication(ctx, req, procedure, errNotAuthenticated, credential,
				connect.CodeUnauthenticated, "invalid authorization header format")
		}

		claims, err := i.jwtManager.ValidateBearerToken(credential)
		if err != nil {
			code, msg := errNotAuthenticated, "invalid token"
			if errors.Is(err, jwt.ErrTokenExpired) {
				code, msg = errTokenExpired, "token expired"
			}
			return nil, i.rejectAuthentication(ctx, req, procedure, code, credential, connect.CodeUnauthenticated, msg)
		}
		principal, err := i.authenticateBearer(ctx, claims)
		if err != nil {
			return nil, i.rejectAuthentication(ctx, req, procedure, errNotAuthenticated, credential, connect.CodeUnauthenticated, "invalid token")
		}
		return i.continueAuthenticated(ctx, next, req, procedure, principal)
	}
}

func (i *AuthInterceptor) authenticateBearer(ctx context.Context, claims *Claims) (*UserContext, error) {
	if claims.TokenType == TokenTypeAPIToken {
		if i.apiTokens == nil || claims.Subject == "" || claims.Subject != claims.UserID || claims.ID == "" {
			return nil, errors.New("invalid API token claims")
		}
		row, err := i.apiTokens.GetApiTokenForAuth(ctx, claims.ID, claims.UserID)
		if err != nil {
			return nil, fmt.Errorf("resolve API token: %w", err)
		}
		if !row.ExpiresAt.After(i.jwtManager.config.Now().UTC()) {
			return nil, errors.New("API token expired")
		}
		state, err := i.apiTokens.GetUserSessionState(ctx, claims.UserID)
		if err != nil {
			return nil, fmt.Errorf("resolve API token user: %w", err)
		}
		if state.IsDeleted || state.Disabled || state.SessionVersion != claims.SessionVersion {
			return nil, errors.New("API token user is not active")
		}
	}
	return &UserContext{
		ID:             claims.UserID,
		Kind:           PrincipalUser,
		Email:          claims.Email,
		Permissions:    claims.Permissions,
		ScopedGrants:   claims.ScopedGrants,
		SessionVersion: claims.SessionVersion,
	}, nil
}

func (i *AuthInterceptor) continueAuthenticated(ctx context.Context, next connect.UnaryFunc, req connect.AnyRequest, procedure string, principal *UserContext) (connect.AnyResponse, error) {
	if i.limiters.Authenticated != nil && !i.limiters.Authenticated.Allow("uid:"+principal.ID) {
		i.logger.Warn("rate limit exceeded", "limiter", "authenticated", "procedure", procedure)
		return nil, authErrorCtx(ctx, errRateLimited, connect.CodeResourceExhausted, "too many requests, try again later")
	}
	if i.limiters.Expensive != nil && isExpensiveProcedure(ProcedureAction(procedure)) {
		if !i.limiters.Expensive.Allow("uid:" + principal.ID) {
			i.logger.Warn("rate limit exceeded", "limiter", "expensive", "procedure", procedure)
			return nil, authErrorCtx(ctx, errRateLimited, connect.CodeResourceExhausted, "too many expensive requests, try again later")
		}
	}
	return next(WithUser(ctx, principal), req)
}

func (i *AuthInterceptor) authenticateBootstrap(
	ctx context.Context,
	next connect.UnaryFunc,
	req connect.AnyRequest,
	procedure, credential string,
) (connect.AnyResponse, error) {
	if i.bootstrap == nil {
		return nil, i.rejectAuthentication(ctx, req, procedure, errNotAuthenticated, credential,
			connect.CodeUnauthenticated, "invalid token")
	}
	principal, err := i.bootstrap.AuthenticateBootstrapToken(ctx, credential)
	if err != nil {
		return nil, i.rejectAuthentication(ctx, req, procedure, errNotAuthenticated, credential,
			connect.CodeUnauthenticated, "invalid token")
	}
	return next(WithUser(ctx, principal), req)
}

func (i *AuthInterceptor) applyPublicLimiters(ctx context.Context, procedure string, req connect.AnyRequest) error {
	type gate struct {
		limiter *RateLimiter
		name    string
		message string
	}
	var g gate
	switch procedure {
	case cadestrov1connect.ControlServiceSSOCallbackProcedure:
		g = gate{i.limiters.SSOCallback, "sso_callback", "too many login attempts, try again later"}
	case cadestrov1connect.ControlServiceRefreshTokenProcedure:
		g = gate{i.limiters.Refresh, "refresh", "too many refresh attempts, try again later"}
	case cadestrov1connect.ControlServiceRegisterProcedure:
		g = gate{i.limiters.Register, "register", "too many registration attempts, try again later"}
	case cadestrov1connect.ControlServiceLogoutProcedure:
		g = gate{i.limiters.Logout, "logout", "too many logout attempts, try again later"}
	case cadestrov1connect.ControlServiceRenewCertificateProcedure:
		g = gate{i.limiters.RenewCert, "renew_cert", "too many certificate renewal attempts, try again later"}
	case cadestrov1connect.ControlServiceListAuthMethodsProcedure:
		g = gate{i.limiters.AuthMethods, "auth_methods", "too many requests, try again later"}
	case cadestrov1connect.ControlServiceGetSSOLoginURLProcedure:
		g = gate{i.limiters.SSO, "sso", "too many requests, try again later"}
	default:
		return nil
	}
	if g.limiter == nil {
		return nil
	}
	if !g.limiter.Allow(clientIP(req)) {
		i.logger.Warn("rate limit exceeded", "limiter", g.name, "procedure", procedure)
		return authErrorCtx(ctx, errRateLimited, connect.CodeResourceExhausted, g.message)
	}
	return nil
}

func (i *AuthInterceptor) rejectAuthentication(
	ctx context.Context,
	req connect.AnyRequest,
	procedure string,
	reason cadestrov1.ErrorCode,
	credential string,
	code connect.Code,
	message string,
) error {
	ip := clientIP(req)
	if i.rejections != nil && (i.limiters.Rejected == nil || i.limiters.Rejected.Allow("rej:"+ip)) {
		att := RejectedAuthentication{
			Procedure:             procedure,
			Reason:                auditReasonString(reason),
			CredentialFingerprint: Fingerprint(credential),
			OriginFingerprint:     Fingerprint(ip),
		}
		if err := i.rejections.RecordRejectedAuthentication(ctx, att); err != nil {
			i.logger.Error("failed to record rejected authentication",
				"procedure", procedure, "reason", reason, "error", err)
		}
	}
	return authErrorCtx(ctx, reason, code, message)
}

func parseAuthorization(header string) (scheme, credential string, err error) {
	if header == "" {
		return "", "", errors.New("missing authentication credentials")
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 {
		return "", "", errors.New("invalid authorization header format")
	}
	scheme, credential = parts[0], strings.TrimSpace(parts[1])
	if credential == "" {
		return "", "", errors.New("missing authentication credentials")
	}
	return scheme, credential, nil
}

func (i *AuthInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *AuthInterceptor) WrapStreamingHandler(connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(context.Context, connect.StreamingHandlerConn) error {
		return connect.NewError(connect.CodeUnimplemented, errors.New("streaming RPCs are not supported on the control server"))
	}
}

type AuthzInterceptor struct{}

func NewAuthzInterceptor() *AuthzInterceptor { return &AuthzInterceptor{} }

func (i *AuthzInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		procedure := req.Spec().Procedure
		if PublicProcedures[procedure] {
			return next(ctx, req)
		}

		userCtx, ok := UserFromContext(ctx)
		if !ok {
			return nil, authErrorCtx(ctx, errNotAuthenticated, connect.CodeUnauthenticated, "not authenticated")
		}

		if alts, hasAlt := procedureAlternatives[procedure]; hasAlt {
			for _, alt := range alts {
				for _, perm := range userCtx.Permissions {
					if perm == alt {
						return next(ctx, req)
					}
				}
			}
			return nil, authErrorCtx(ctx, errPermissionDenied, connect.CodePermissionDenied, "permission denied")
		}

		if !Authorize(AuthzInput{
			Permissions:  userCtx.Permissions,
			SubjectID:    userCtx.ID,
			SelfEligible: userCtx.CanOwnResources(),
			Action:       ProcedureAction(procedure),
		}) {
			return nil, authErrorCtx(ctx, errPermissionDenied, connect.CodePermissionDenied, "permission denied")
		}
		return next(ctx, req)
	}
}

func (i *AuthzInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *AuthzInterceptor) WrapStreamingHandler(connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(context.Context, connect.StreamingHandlerConn) error {
		return connect.NewError(connect.CodeUnimplemented, errors.New("streaming RPCs are not supported on the control server"))
	}
}

func authErrorCtx(ctx context.Context, code cadestrov1.ErrorCode, connectCode connect.Code, msg string) *connect.Error {
	e := connect.NewError(connectCode, errors.New(msg))
	detail := &cadestrov1.ErrorDetail{Code: code, RequestId: &cadestrov1.RequestId{Value: middleware.RequestIDFromContext(ctx)}}
	if d, err := connect.NewErrorDetail(detail); err == nil {
		e.AddDetail(d)
	}
	return e
}

func auditReasonString(code cadestrov1.ErrorCode) string {
	return strings.ToLower(strings.TrimPrefix(code.String(), "ERROR_CODE_"))
}

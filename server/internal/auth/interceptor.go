package auth

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"

	"github.com/manchtools/cadestro/contract/gen/go/cadestro/v1/cadestrov1connect"
)

type UserLookup func(context.Context, string) (string, int32, error)

type Interceptor struct {
	jwt    *JWTManager
	lookup UserLookup
}

func NewInterceptor(jwt *JWTManager, lookup UserLookup) *Interceptor {
	return &Interceptor{jwt: jwt, lookup: lookup}
}

func (interceptor *Interceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, request connect.AnyRequest) (connect.AnyResponse, error) {
		if publicProcedure(request.Spec().Procedure) {
			return next(ctx, request)
		}
		authorization := request.Header().Get("Authorization")
		prefix, token, found := strings.Cut(authorization, " ")
		if !found || !strings.EqualFold(prefix, "Bearer") || token == "" {
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
		}
		claims, err := interceptor.jwt.ValidateToken(token, TokenTypeAccess)
		if err != nil {
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid access token"))
		}
		email, sessionVersion, err := interceptor.lookup(ctx, claims.UserID)
		if err != nil || sessionVersion != claims.SessionVersion {
			return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid access token"))
		}
		return next(WithUser(ctx, &UserContext{ID: claims.UserID, Email: email, SessionVersion: sessionVersion}), request)
	}
}

func publicProcedure(procedure string) bool {
	switch procedure {
	case cadestrov1connect.ControlServiceRefreshTokenProcedure,
		cadestrov1connect.ControlServiceLogoutProcedure,
		cadestrov1connect.ControlServiceListAuthMethodsProcedure,
		cadestrov1connect.ControlServiceGetSSOLoginURLProcedure,
		cadestrov1connect.ControlServiceSSOCallbackProcedure,
		cadestrov1connect.ControlServiceRegisterProcedure,
		cadestrov1connect.ControlServiceRenewCertificateProcedure:
		return true
	default:
		return false
	}
}

package auth

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/oklog/ulid/v2"
)

type contextKey string

const (
	userContextKey contextKey = "user"
)

type PrincipalKind string

const (
	PrincipalUser PrincipalKind = "user"

	PrincipalBootstrapAdmin PrincipalKind = "bootstrap_admin"
)

const BootstrapPrincipalID = "bootstrap-admin"

type UserContext struct {
	ID             string
	Kind           PrincipalKind
	Email          string
	Permissions    []string
	ScopedGrants   []ScopedGrant
	SessionVersion int32
}

func (u *UserContext) CanOwnResources() bool {
	if u == nil || u.Kind != PrincipalUser {
		return false
	}
	_, err := ulid.ParseStrict(u.ID)
	return err == nil
}

func WithUser(ctx context.Context, user *UserContext) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}

func UserFromContext(ctx context.Context) (*UserContext, bool) {
	user, ok := ctx.Value(userContextKey).(*UserContext)
	if !ok || user == nil {
		return nil, false
	}
	return user, true
}

func HasPermission(ctx context.Context, perm string) bool {
	user, ok := UserFromContext(ctx)
	if !ok {
		return false
	}
	for _, p := range user.Permissions {
		if p == perm {
			return true
		}
	}
	return false
}

func EnforceSelfScope(ctx context.Context, action, resourceID string) error {
	user, ok := UserFromContext(ctx)
	if !ok {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("not authenticated"))
	}
	if HasPermission(ctx, action) {
		return nil
	}
	if HasPermission(ctx, action+":self") && user.CanOwnResources() && resourceID == user.ID {
		return nil
	}
	return connect.NewError(connect.CodePermissionDenied, errors.New("permission denied"))
}

package auth

import (
	"context"

	cadestrov1 "github.com/manchtools/cadestro/contract/gen/go/cadestro/v1"
)

type contextKey struct{}

type UserContext struct {
	ID             string
	Email          string
	SessionVersion int32
	Permissions    []cadestrov1.Permission
}

func WithUser(ctx context.Context, user *UserContext) context.Context {
	return context.WithValue(ctx, contextKey{}, user)
}

func UserFromContext(ctx context.Context) (*UserContext, bool) {
	user, ok := ctx.Value(contextKey{}).(*UserContext)
	return user, ok && user != nil
}

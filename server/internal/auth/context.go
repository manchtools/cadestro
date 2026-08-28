package auth

import "context"

type contextKey struct{}

type UserContext struct {
	ID             string
	Email          string
	SessionVersion int32
}

func WithUser(ctx context.Context, user *UserContext) context.Context {
	return context.WithValue(ctx, contextKey{}, user)
}

func UserFromContext(ctx context.Context) (*UserContext, bool) {
	user, ok := ctx.Value(contextKey{}).(*UserContext)
	return user, ok && user != nil
}

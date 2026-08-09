package scopedtoken

import (
	"context"

	"github.com/your-org/ai-runtime-gateway/internal/core"
)

type contextKey struct{}

func WithClaims(ctx context.Context, claims Claims) context.Context {
	return context.WithValue(ctx, contextKey{}, claims)
}

func FromContext(ctx context.Context) (Claims, bool) {
	claims, ok := ctx.Value(contextKey{}).(Claims)
	return claims, ok
}

func RequireScope(ctx context.Context, scope string) error {
	claims, ok := FromContext(ctx)
	if !ok {
		return core.ErrUnauthorized
	}
	if !claims.HasScope(scope) {
		return core.ErrForbidden
	}
	return nil
}

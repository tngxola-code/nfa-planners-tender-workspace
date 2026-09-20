package auth

import "context"

type ctxKey struct{}

// WithClaims returns a context carrying the verified claims. Only the
// authentication middleware calls this.
func WithClaims(ctx context.Context, c *Claims) context.Context {
	return context.WithValue(ctx, ctxKey{}, c)
}

// FromContext returns the verified claims, if the request was authenticated.
func FromContext(ctx context.Context) (*Claims, bool) {
	c, ok := ctx.Value(ctxKey{}).(*Claims)
	return c, ok
}

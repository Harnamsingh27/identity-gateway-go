package auth

import (
	"context"

	"github.com/golang-jwt/jwt/v5"
)

// Claims is the JWT claims struct shared across all go-servicekit services.
// It extends RegisteredClaims with a Role field extracted from the "role" JSON key.
type Claims struct {
	jwt.RegisteredClaims
	Role string `json:"role"`
}

type contextKey struct{}

// WithClaims stores c in ctx. Retrieve with ClaimsFromContext.
func WithClaims(ctx context.Context, c *Claims) context.Context {
	return context.WithValue(ctx, contextKey{}, c)
}

// ClaimsFromContext returns the Claims stored in ctx by HTTPMiddleware or
// UnaryInterceptor, and a boolean indicating whether they were present.
func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	c, ok := ctx.Value(contextKey{}).(*Claims)
	return c, ok
}

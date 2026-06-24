package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	gjwt "github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	appErrors "github.com/harnamsingh/go-servicekit/errors"
)

// VerifierOption configures a Verifier.
type VerifierOption func(*verifierConfig)

type verifierConfig struct {
	issuer   string
	audience []string
}

// WithIssuer restricts accepted tokens to those with the given iss claim.
func WithIssuer(iss string) VerifierOption {
	return func(c *verifierConfig) { c.issuer = iss }
}

// WithAudience restricts accepted tokens to those containing one of the given
// audience values.
func WithAudience(aud ...string) VerifierOption {
	return func(c *verifierConfig) { c.audience = aud }
}

// Verifier parses and validates a raw JWT string.
type Verifier interface {
	Verify(tokenString string) (*Claims, error)
}

// HMACVerifier verifies tokens signed with HMAC (HS256/HS384/HS512).
type HMACVerifier struct {
	secret []byte
	cfg    verifierConfig
}

// NewHMACVerifier returns a Verifier that accepts HMAC-signed JWTs.
func NewHMACVerifier(secret []byte, opts ...VerifierOption) *HMACVerifier {
	v := &HMACVerifier{secret: secret}
	for _, o := range opts {
		o(&v.cfg)
	}
	return v
}

// Verify implements Verifier.
func (v *HMACVerifier) Verify(tokenString string) (*Claims, error) {
	return parseToken(tokenString, func(t *gjwt.Token) (any, error) {
		if _, ok := t.Method.(*gjwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidSignature
		}
		return v.secret, nil
	}, v.cfg)
}

// parseToken is the shared token parsing logic used by all verifier types.
func parseToken(tokenString string, keyFn gjwt.Keyfunc, cfg verifierConfig) (*Claims, error) {
	parserOpts := []gjwt.ParserOption{gjwt.WithExpirationRequired()}
	if cfg.issuer != "" {
		parserOpts = append(parserOpts, gjwt.WithIssuer(cfg.issuer))
	}
	if len(cfg.audience) > 0 {
		parserOpts = append(parserOpts, gjwt.WithAudience(cfg.audience[0]))
	}

	claims := &Claims{}
	token, err := gjwt.ParseWithClaims(tokenString, claims, keyFn, parserOpts...)
	if err != nil {
		switch {
		case errors.Is(err, gjwt.ErrTokenExpired):
			return nil, ErrExpiredToken
		case errors.Is(err, gjwt.ErrSignatureInvalid):
			return nil, ErrInvalidSignature
		default:
			return nil, appErrors.Wrap(appErrors.CodeUnauthorized, "invalid token", err)
		}
	}
	c, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}
	return c, nil
}

// extractBearerToken pulls the token from an "Authorization: Bearer <token>" header.
func extractBearerToken(authHeader string) (string, error) {
	if authHeader == "" {
		return "", ErrMissingToken
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return "", ErrInvalidToken
	}
	return strings.TrimSpace(parts[1]), nil
}

// JWTMiddleware returns an HTTP middleware that validates a Bearer JWT in the
// Authorization header and stores the parsed Claims in the request context
// via WithClaims.
func JWTMiddleware(v Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, err := extractBearerToken(r.Header.Get("Authorization"))
			if err != nil {
				ae := new(appErrors.AppError)
				if errors.As(err, &ae) {
					http.Error(w, ae.Message, appErrors.ToHTTPStatus(err))
				} else {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
				}
				return
			}
			claims, err := v.Verify(raw)
			if err != nil {
				ae := new(appErrors.AppError)
				if errors.As(err, &ae) {
					http.Error(w, ae.Message, appErrors.ToHTTPStatus(err))
				} else {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
				}
				return
			}
			ctx := WithClaims(r.Context(), claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// JWTUnaryInterceptor returns a gRPC unary server interceptor that validates
// the bearer token found in the incoming metadata "authorization" key.
func JWTUnaryInterceptor(v Verifier) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx, err := jwtFromMetadata(ctx, v)
		if err != nil {
			return nil, appErrors.ToGRPCStatus(err).Err()
		}
		return handler(ctx, req)
	}
}

// JWTStreamInterceptor returns a gRPC stream server interceptor that validates
// the bearer token found in the incoming metadata "authorization" key.
func JWTStreamInterceptor(v Verifier) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx, err := jwtFromMetadata(ss.Context(), v)
		if err != nil {
			return appErrors.ToGRPCStatus(err).Err()
		}
		return handler(srv, &wrappedStream{ServerStream: ss, ctx: ctx})
	}
}

func jwtFromMetadata(ctx context.Context, v Verifier) (context.Context, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx, ErrMissingToken
	}
	vals := md.Get("authorization")
	if len(vals) == 0 {
		return ctx, ErrMissingToken
	}
	raw, err := extractBearerToken(vals[0])
	if err != nil {
		return ctx, err
	}
	claims, err := v.Verify(raw)
	if err != nil {
		return ctx, err
	}
	return WithClaims(ctx, claims), nil
}

// wrappedStream overrides the context carried by a grpc.ServerStream.
type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

// Context returns the overridden context.
func (w *wrappedStream) Context() context.Context { return w.ctx }

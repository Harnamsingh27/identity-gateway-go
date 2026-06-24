
package observability

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"google.golang.org/grpc/metadata"
)

// RequestIDHeader is the canonical HTTP header name for request IDs.
const RequestIDHeader = "X-Request-ID"

// RequestIDMetadataKey is the gRPC metadata key for request IDs.
const RequestIDMetadataKey = "x-request-id"

type reqIDKey struct{}

// NewRequestID generates a new random UUID request ID.
func NewRequestID() string {
	return uuid.New().String()
}

// WithRequestID returns a context carrying the given request ID.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, reqIDKey{}, id)
}

// RequestIDFromContext extracts the request ID from ctx. Returns "" if absent.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(reqIDKey{}).(string)
	return id
}

// RequestIDMiddleware is an HTTP middleware that reads X-Request-ID from the
// incoming request (or generates one) and injects it into the request context
// and response header.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(RequestIDHeader)
		if id == "" {
			id = NewRequestID()
		}
		ctx := WithRequestID(r.Context(), id)
		w.Header().Set(RequestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFromMetadata extracts the request ID from gRPC incoming metadata.
func RequestIDFromMetadata(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	vals := md.Get(RequestIDMetadataKey)
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

// WithRequestIDFromMetadata extracts the request ID from gRPC incoming metadata
// and stores it in the context.
func WithRequestIDFromMetadata(ctx context.Context) context.Context {
	id := RequestIDFromMetadata(ctx)
	if id == "" {
		id = NewRequestID()
	}
	return WithRequestID(ctx, id)
}

// InjectRequestIDToMetadata adds the request ID from context to outgoing gRPC metadata.
func InjectRequestIDToMetadata(ctx context.Context) context.Context {
	id := RequestIDFromContext(ctx)
	if id == "" {
		return ctx
	}
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		md = metadata.New(nil)
	}
	md.Set(RequestIDMetadataKey, id)
	return metadata.NewOutgoingContext(ctx, md)
}

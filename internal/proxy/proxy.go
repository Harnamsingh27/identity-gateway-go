// Package proxy implements the HTTP reverse-proxy and gRPC pass-through for
// the identity gateway.
package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/harnamsingh/go-servicekit/observability"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

// HTTPProxy is a configurable reverse proxy that forwards allowed requests to
// a set of named backend services.
type HTTPProxy struct {
	backends map[string]*httputil.ReverseProxy
	logger   *slog.Logger
	tracer   interface{ Start(context.Context, string) }
}

// NewHTTPProxy creates an HTTPProxy from a map of backend-key → base URL strings.
func NewHTTPProxy(backends map[string]string, logger *slog.Logger) (*HTTPProxy, error) {
	rps := make(map[string]*httputil.ReverseProxy, len(backends))
	for key, rawURL := range backends {
		u, err := url.Parse(rawURL)
		if err != nil {
			return nil, fmt.Errorf("proxy: invalid backend URL for %q: %w", key, err)
		}
		rp := httputil.NewSingleHostReverseProxy(u)
		rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			logger.ErrorContext(r.Context(), "proxy error",
				slog.String("backend", key),
				slog.String("error", err.Error()),
			)
			http.Error(w, "bad gateway", http.StatusBadGateway)
		}
		// Rewrite the request so the backend sees the correct Host header.
		origDirector := rp.Director
		rp.Director = func(req *http.Request) {
			origDirector(req)
			// Propagate request ID to backend.
			if rid := observability.RequestIDFromContext(req.Context()); rid != "" {
				req.Header.Set("X-Request-ID", rid)
			}
		}
		rps[key] = rp
	}
	return &HTTPProxy{backends: rps, logger: logger}, nil
}

// Forward proxies the request to the backend identified by backendKey. If
// backendKey is empty or unknown, it falls back to the first registered backend.
// Returns an HTTP status code and error.
func (p *HTTPProxy) Forward(w http.ResponseWriter, r *http.Request, backendKey string) error {
	tr := otel.Tracer("proxy")
	ctx, span := tr.Start(r.Context(), "proxy.forward")
	defer span.End()

	span.SetAttributes(attribute.String("proxy.backend", backendKey))

	rp, ok := p.backends[backendKey]
	if !ok {
		// Try any available backend if the key is unknown.
		for _, rp = range p.backends {
			ok = true
			break
		}
	}
	if !ok {
		http.Error(w, "no backend available", http.StatusBadGateway)
		return fmt.Errorf("proxy: no backend registered for key %q", backendKey)
	}

	start := time.Now()
	rp.ServeHTTP(w, r.WithContext(ctx))
	p.logger.DebugContext(ctx, "forwarded request",
		slog.String("backend", backendKey),
		slog.Duration("elapsed", time.Since(start)),
	)
	return nil
}

// Backends returns the set of registered backend keys.
func (p *HTTPProxy) Backends() []string {
	keys := make([]string, 0, len(p.backends))
	for k := range p.backends {
		keys = append(keys, k)
	}
	return keys
}

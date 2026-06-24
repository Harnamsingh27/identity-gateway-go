package proxy

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/harnamsingh/go-servicekit/auth"
	"github.com/harnamsingh/go-servicekit/observability"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"identity-gateway-go/internal/audit"
	"identity-gateway-go/internal/policy"
	"identity-gateway-go/internal/ratelimit"
)

// GatewayHandler is an http.Handler that chains:
//
//	request-ID → JWT auth → rate-limit → policy → audit → proxy
type GatewayHandler struct {
	proxy     *HTTPProxy
	pol       *policy.Policy
	rateLim   *ratelimit.Limiter
	auditLog  *audit.Logger
	logger    *slog.Logger
}

// NewGatewayHandler creates a GatewayHandler wired to the provided components.
func NewGatewayHandler(
	p *HTTPProxy,
	pol *policy.Policy,
	rl *ratelimit.Limiter,
	al *audit.Logger,
	logger *slog.Logger,
) *GatewayHandler {
	return &GatewayHandler{
		proxy:    p,
		pol:      pol,
		rateLim:  rl,
		auditLog: al,
		logger:   logger,
	}
}

func (h *GatewayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx := r.Context()

	tr := otel.Tracer("gateway")
	ctx, span := tr.Start(ctx, "gateway.handle")
	defer span.End()
	r = r.WithContext(ctx)

	rid := observability.RequestIDFromContext(ctx)
	span.SetAttributes(
		attribute.String("http.method", r.Method),
		attribute.String("http.path", r.URL.Path),
		attribute.String("request_id", rid),
	)

	// Extract claims already set in context by the JWT middleware.
	claims, hasClaims := auth.ClaimsFromContext(ctx)
	identity := remoteIP(r)
	role := ""
	if hasClaims {
		identity = claims.Subject
		role = claims.Role
	}

	// Rate limit check.
	ok, retryAfter := h.rateLim.Allow(identity)
	if !ok {
		h.auditLog.Log(ctx, audit.Entry{
			RequestID: rid,
			Identity:  identity,
			Role:      role,
			Method:    r.Method,
			Path:      r.URL.Path,
			Decision:  audit.Deny,
			Reason:    "rate_limited",
			Latency:   time.Since(start),
			StatusCode: http.StatusTooManyRequests,
		})
		secs := int(retryAfter.Seconds()) + 1
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	// Policy decision.
	result := h.pol.Decide(role, r.Method, r.URL.Path)
	if !result.Allow {
		statusCode := http.StatusForbidden
		h.auditLog.Log(ctx, audit.Entry{
			RequestID:    rid,
			Identity:     identity,
			Role:         role,
			Method:       r.Method,
			Path:         r.URL.Path,
			MatchedRoute: result.MatchedRoute,
			Decision:     audit.Deny,
			Reason:       result.Reason,
			Latency:      time.Since(start),
			StatusCode:   statusCode,
		})
		http.Error(w, result.Reason, statusCode)
		return
	}

	// Forward to backend.
	sw := &statusCapture{ResponseWriter: w, code: http.StatusOK}
	if err := h.proxy.Forward(sw, r, result.Backend); err != nil {
		h.logger.ErrorContext(ctx, "proxy forward error", slog.String("error", err.Error()))
	}

	h.auditLog.Log(ctx, audit.Entry{
		RequestID:    rid,
		Identity:     identity,
		Role:         role,
		Method:       r.Method,
		Path:         r.URL.Path,
		MatchedRoute: result.MatchedRoute,
		Decision:     audit.Allow,
		Reason:       result.Reason,
		Latency:      time.Since(start),
		Backend:      result.Backend,
		StatusCode:   sw.code,
	})
}

// statusCapture captures the HTTP status code written by the downstream handler.
type statusCapture struct {
	http.ResponseWriter
	code    int
	written bool
}

func (sc *statusCapture) WriteHeader(code int) {
	if !sc.written {
		sc.code = code
		sc.written = true
	}
	sc.ResponseWriter.WriteHeader(code)
}

func remoteIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return strings.Split(ip, ",")[0]
	}
	return r.RemoteAddr
}


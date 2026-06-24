// Package audit emits one structured JSON log line per gateway decision.
package audit

import (
	"context"
	"log/slog"
	"time"

	"github.com/harnamsingh/go-servicekit/observability"
)

// Decision represents whether a request was allowed or denied.
type Decision string

const (
	// Allow means the gateway forwarded the request.
	Allow Decision = "allow"
	// Deny means the gateway rejected the request.
	Deny Decision = "deny"
)

// Entry holds all the fields written to the audit log for a single request.
type Entry struct {
	Timestamp  time.Time
	RequestID  string
	Identity   string // subject claim from JWT, or IP if no JWT
	Role       string
	Method     string
	Path       string
	MatchedRoute string
	Decision   Decision
	Reason     string
	Latency    time.Duration
	Backend    string // forwarded-to backend key (empty on deny)
	StatusCode int
}

// Logger wraps a slog.Logger and writes structured audit entries.
type Logger struct {
	l *slog.Logger
}

// New creates a Logger that writes to the provided slog.Logger.
func New(l *slog.Logger) *Logger {
	return &Logger{l: l}
}

// Log writes one audit log line for the given entry. The request ID is
// extracted from ctx if present.
func (al *Logger) Log(ctx context.Context, e Entry) {
	if e.RequestID == "" {
		e.RequestID = observability.RequestIDFromContext(ctx)
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}

	al.l.LogAttrs(ctx, slog.LevelInfo, "audit",
		slog.Time("timestamp", e.Timestamp),
		slog.String("request_id", e.RequestID),
		slog.String("identity", e.Identity),
		slog.String("role", e.Role),
		slog.String("method", e.Method),
		slog.String("path", e.Path),
		slog.String("matched_route", e.MatchedRoute),
		slog.String("decision", string(e.Decision)),
		slog.String("reason", e.Reason),
		slog.Duration("latency", e.Latency),
		slog.String("backend", e.Backend),
		slog.Int("status_code", e.StatusCode),
	)
}


package observability

import (
	"context"
	"io"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel/trace"
)

// LoggerOption configures NewLogger.
type LoggerOption func(*loggerConfig)

type loggerConfig struct {
	level     slog.Level
	output    io.Writer
	addSource bool
}

// WithLogLevel sets the minimum log level.
func WithLogLevel(l slog.Level) LoggerOption {
	return func(c *loggerConfig) { c.level = l }
}

// WithLogOutput sets the writer for log output. Defaults to os.Stderr.
func WithLogOutput(w io.Writer) LoggerOption {
	return func(c *loggerConfig) { c.output = w }
}

// WithLogSource includes the source file and line in every log record.
func WithLogSource() LoggerOption {
	return func(c *loggerConfig) { c.addSource = true }
}

// NewLogger constructs a JSON slog.Logger that automatically attaches
// request_id and trace_id from context when present.
func NewLogger(opts ...LoggerOption) *slog.Logger {
	cfg := &loggerConfig{level: slog.LevelInfo, output: os.Stderr}
	for _, o := range opts {
		o(cfg)
	}
	base := slog.NewJSONHandler(cfg.output, &slog.HandlerOptions{
		Level:     cfg.level,
		AddSource: cfg.addSource,
	})
	return slog.New(&contextHandler{Handler: base})
}

// contextHandler wraps a slog.Handler and injects request_id and trace_id
// from context on every log record.
type contextHandler struct {
	slog.Handler
}

// Handle adds context attributes before delegating to the inner handler.
func (h *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := RequestIDFromContext(ctx); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		sc := span.SpanContext()
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.Handler.Handle(ctx, r)
}

// WithAttrs returns a new contextHandler with the given attrs pre-attached.
func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithAttrs(attrs)}
}

// WithGroup returns a new contextHandler scoped to the given group.
func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{Handler: h.Handler.WithGroup(name)}
}

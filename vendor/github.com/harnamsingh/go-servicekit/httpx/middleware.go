package httpx

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/harnamsingh/go-servicekit/observability"
)

// PanicRecoveryMiddleware catches panics in downstream handlers, logs the stack
// trace, and returns 500 Internal Server Error without crashing the server.
func PanicRecoveryMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					stack := string(debug.Stack())
					logger.ErrorContext(r.Context(), "panic recovered",
						slog.Any("panic", rec),
						slog.String("stack", stack),
					)
					http.Error(w, "internal server error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// statusWriter wraps ResponseWriter to capture the HTTP status code.
type statusWriter struct {
	http.ResponseWriter
	status  int
	written int64
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

func (sw *statusWriter) Write(b []byte) (int, error) {
	n, err := sw.ResponseWriter.Write(b)
	sw.written += int64(n)
	return n, err
}

// AccessLogMiddleware records one structured log line per request with method,
// path, status, latency, and request ID.
func AccessLogMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()
			next.ServeHTTP(sw, r)
			logger.InfoContext(r.Context(), "http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", sw.status),
				slog.String("latency", time.Since(start).String()),
				slog.String("remote_addr", r.RemoteAddr),
			)
		})
	}
}

// TimeoutMiddleware cancels the request context after d and returns 503 if the
// handler has not responded in time.
func TimeoutMiddleware(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.TimeoutHandler(next, d, fmt.Sprintf("request timed out after %s", d))
	}
}

// DefaultMiddleware returns the standard ordered middleware chain:
//
//	PanicRecovery → RequestID → AccessLog → Timeout
func DefaultMiddleware(logger *slog.Logger, timeout time.Duration) []func(http.Handler) http.Handler {
	return []func(http.Handler) http.Handler{
		PanicRecoveryMiddleware(logger),
		observability.RequestIDMiddleware,
		AccessLogMiddleware(logger),
		TimeoutMiddleware(timeout),
	}
}

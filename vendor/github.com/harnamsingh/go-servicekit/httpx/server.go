package httpx

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

const (
	defaultAddr           = ":8080"
	defaultReadTimeout    = 5 * time.Second
	defaultWriteTimeout   = 10 * time.Second
	defaultIdleTimeout    = 120 * time.Second
	defaultMaxHeaderBytes = 1 << 20 // 1 MB
)

// ServerOption configures a Server.
type ServerOption func(*serverConfig)

type serverConfig struct {
	addr           string
	readTimeout    time.Duration
	writeTimeout   time.Duration
	idleTimeout    time.Duration
	maxHeaderBytes int
	middlewares    []func(http.Handler) http.Handler
}

// WithAddr sets the listen address (default ":8080").
func WithAddr(addr string) ServerOption {
	return func(c *serverConfig) { c.addr = addr }
}

// WithReadTimeout sets the maximum duration for reading the entire request.
func WithReadTimeout(d time.Duration) ServerOption {
	return func(c *serverConfig) { c.readTimeout = d }
}

// WithWriteTimeout sets the maximum duration before timing out writes of the response.
func WithWriteTimeout(d time.Duration) ServerOption {
	return func(c *serverConfig) { c.writeTimeout = d }
}

// WithIdleTimeout sets the maximum duration to wait for the next request on keep-alive.
func WithIdleTimeout(d time.Duration) ServerOption {
	return func(c *serverConfig) { c.idleTimeout = d }
}

// WithServerMiddleware appends middlewares to the global chain applied to every route.
// The chain is applied in the order provided (first = outermost wrapper).
func WithServerMiddleware(mw ...func(http.Handler) http.Handler) ServerOption {
	return func(c *serverConfig) { c.middlewares = append(c.middlewares, mw...) }
}

// Server wraps net/http.Server with production defaults and graceful shutdown.
type Server struct {
	inner  *http.Server
	router *Router
}

// NewServer creates a Server with sane production defaults. Configure via options.
func NewServer(opts ...ServerOption) *Server {
	cfg := &serverConfig{
		addr:           defaultAddr,
		readTimeout:    defaultReadTimeout,
		writeTimeout:   defaultWriteTimeout,
		idleTimeout:    defaultIdleTimeout,
		maxHeaderBytes: defaultMaxHeaderBytes,
	}
	for _, o := range opts {
		o(cfg)
	}

	r := NewRouter()
	var handler http.Handler = r

	// Apply middlewares in reverse so the first option is the outermost wrapper.
	for i := len(cfg.middlewares) - 1; i >= 0; i-- {
		handler = cfg.middlewares[i](handler)
	}

	s := &Server{
		router: r,
		inner: &http.Server{
			Addr:           cfg.addr,
			Handler:        handler,
			ReadTimeout:    cfg.readTimeout,
			WriteTimeout:   cfg.writeTimeout,
			IdleTimeout:    cfg.idleTimeout,
			MaxHeaderBytes: cfg.maxHeaderBytes,
		},
	}
	return s
}

// Router returns the underlying Router so routes can be registered.
func (s *Server) Router() *Router { return s.router }

// ListenAndServe starts the server. It returns only on error or after Shutdown.
func (s *Server) ListenAndServe() error {
	if err := s.inner.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("httpx: listen: %w", err)
	}
	return nil
}

// Serve accepts connections on the given listener.
func (s *Server) Serve(l net.Listener) error {
	if err := s.inner.Serve(l); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("httpx: serve: %w", err)
	}
	return nil
}

// Shutdown gracefully stops the server, waiting for in-flight requests to complete.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.inner.Shutdown(ctx)
}

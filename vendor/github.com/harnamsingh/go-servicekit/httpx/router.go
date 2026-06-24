package httpx

import (
	"net/http"
	"strings"
)

// Router is a thin wrapper around http.ServeMux that adds method-based routing
// and group/prefix support.
type Router struct {
	mux    *http.ServeMux
	prefix string
}

// NewRouter creates a new Router backed by http.ServeMux.
func NewRouter() *Router {
	return &Router{mux: http.NewServeMux()}
}

// Handle registers handler for the given HTTP method and path pattern.
// The pattern is "{METHOD} {prefix}{path}".
func (r *Router) Handle(method, path string, handler http.Handler) {
	pattern := strings.ToUpper(method) + " " + r.prefix + path
	r.mux.Handle(pattern, handler)
}

// GET registers a GET handler.
func (r *Router) GET(path string, h http.HandlerFunc) { r.Handle(http.MethodGet, path, h) }

// POST registers a POST handler.
func (r *Router) POST(path string, h http.HandlerFunc) { r.Handle(http.MethodPost, path, h) }

// PUT registers a PUT handler.
func (r *Router) PUT(path string, h http.HandlerFunc) { r.Handle(http.MethodPut, path, h) }

// PATCH registers a PATCH handler.
func (r *Router) PATCH(path string, h http.HandlerFunc) { r.Handle(http.MethodPatch, path, h) }

// DELETE registers a DELETE handler.
func (r *Router) DELETE(path string, h http.HandlerFunc) { r.Handle(http.MethodDelete, path, h) }

// HandleFunc registers a handler function for the given method and path.
func (r *Router) HandleFunc(method, path string, h http.HandlerFunc) {
	r.Handle(method, path, h)
}

// Group returns a new Router that shares the underlying mux but prepends prefix
// to every registered path.
func (r *Router) Group(prefix string) *Router {
	return &Router{mux: r.mux, prefix: r.prefix + prefix}
}

// ServeHTTP implements http.Handler so the Router can be used directly.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mux.ServeHTTP(w, req)
}

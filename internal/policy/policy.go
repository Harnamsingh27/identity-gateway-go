// Package policy loads a YAML route-policy file and evaluates RBAC decisions.
package policy

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Route describes a single policy entry: which paths + methods are allowed for
// which caller roles.
type Route struct {
	// Path is a path pattern, e.g. "/admin", "/users/:id", "/api/*".
	Path string `yaml:"path"`
	// Methods is the list of HTTP methods covered by this route (e.g. GET, POST).
	// An empty list matches all methods.
	Methods []string `yaml:"methods"`
	// Roles is the list of caller roles that are allowed to access this route.
	Roles []string `yaml:"roles"`
	// Backend is an optional target key that the proxy uses to look up the
	// backend base URL. When empty, a global or default backend is used.
	Backend string `yaml:"backend"`
}

// File is the top-level structure of a policy YAML file.
type File struct {
	Routes []Route `yaml:"routes"`
}

// Policy holds a compiled set of routes and provides RBAC decisions.
type Policy struct {
	mu     sync.RWMutex
	routes []Route
}

// Load reads and parses the YAML file at path and returns a ready-to-use Policy.
func Load(path string) (*Policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("policy: read %q: %w", path, err)
	}
	return parse(data)
}

// parse unmarshals raw YAML bytes into a Policy.
func parse(data []byte) (*Policy, error) {
	var f File
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("policy: parse YAML: %w", err)
	}
	// Normalise methods to upper-case.
	for i := range f.Routes {
		for j, m := range f.Routes[i].Methods {
			f.Routes[i].Methods[j] = strings.ToUpper(m)
		}
	}
	return &Policy{routes: f.Routes}, nil
}

// Reload atomically replaces the loaded routes with a freshly parsed file.
// It is safe to call concurrently with Decide.
func (p *Policy) Reload(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("policy: reload %q: %w", path, err)
	}
	next, err := parse(data)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.routes = next.routes
	p.mu.Unlock()
	return nil
}

// DecisionResult is returned by Decide and carries all the information the
// gateway needs to act on the RBAC verdict.
type DecisionResult struct {
	// Allow is true when the request is permitted.
	Allow bool
	// MatchedRoute is the Path of the first route that matched the request.
	// Empty when no route matched (default-deny case).
	MatchedRoute string
	// Backend is the backend key from the matched route, if any.
	Backend string
	// Reason is a short human-readable explanation of the verdict (used in audit logs).
	Reason string
}

// Decide evaluates whether a caller with the given role may execute method on
// path. It returns a DecisionResult. The default when no route matches is deny.
func (p *Policy) Decide(role, method, path string) DecisionResult {
	p.mu.RLock()
	routes := p.routes
	p.mu.RUnlock()

	method = strings.ToUpper(method)

	for _, r := range routes {
		if !matchPath(r.Path, path) {
			continue
		}
		// A route with no methods configured matches all methods.
		if len(r.Methods) > 0 && !containsMethod(r.Methods, method) {
			return DecisionResult{
				Allow:        false,
				MatchedRoute: r.Path,
				Reason:       fmt.Sprintf("method %q not allowed on route %q", method, r.Path),
			}
		}
		if !containsRole(r.Roles, role) {
			return DecisionResult{
				Allow:        false,
				MatchedRoute: r.Path,
				Reason:       fmt.Sprintf("role %q not in allowed roles for route %q", role, r.Path),
			}
		}
		return DecisionResult{
			Allow:        true,
			MatchedRoute: r.Path,
			Backend:      r.Backend,
			Reason:       "allowed",
		}
	}

	return DecisionResult{
		Allow:  false,
		Reason: "no route matched (default deny)",
	}
}

// matchPath reports whether the request path matches the route pattern.
//
// Rules:
//   - Exact match: "/admin" matches only "/admin"
//   - Wildcard segment: "/users/:id" matches "/users/123" and "/users/abc"
//   - Trailing wildcard: "/api/*" matches "/api/", "/api/v1/", "/api/v1/foo"
func matchPath(pattern, path string) bool {
	if pattern == path {
		return true
	}

	// Trailing wildcard: strip the "*" and check prefix.
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(path, prefix)
	}

	// Segment-by-segment matching for patterns with ":" parameters.
	patternSegs := strings.Split(strings.TrimPrefix(pattern, "/"), "/")
	pathSegs := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(patternSegs) != len(pathSegs) {
		return false
	}
	for i, ps := range patternSegs {
		if strings.HasPrefix(ps, ":") {
			continue // wildcard segment — matches any value
		}
		if ps != pathSegs[i] {
			return false
		}
	}
	return true
}

func containsMethod(methods []string, method string) bool {
	for _, m := range methods {
		if m == method {
			return true
		}
	}
	return false
}

func containsRole(roles []string, role string) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}

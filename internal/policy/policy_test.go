package policy_test

import (
	"os"
	"path/filepath"
	"testing"

	"identity-gateway-go/internal/policy"
)

const testYAML = `
routes:
  - path: /admin
    methods: [GET, POST]
    roles: [admin]
    backend: backend-a
  - path: /profile
    methods: [GET]
    roles: [user, admin]
    backend: backend-b
  - path: /users/:id
    methods: [GET, PUT, DELETE]
    roles: [admin]
    backend: backend-c
  - path: /public/*
    methods: [GET]
    roles: [user, admin, guest]
`

func mustLoad(t *testing.T, yaml string) *policy.Policy {
	t.Helper()
	f := filepath.Join(t.TempDir(), "policy.yaml")
	if err := os.WriteFile(f, []byte(yaml), 0o600); err != nil {
		t.Fatalf("writeFile: %v", err)
	}
	p, err := policy.Load(f)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return p
}

// TestAllowed verifies a matching role on a matching route returns allow=true.
func TestAllowed(t *testing.T) {
	p := mustLoad(t, testYAML)
	d := p.Decide("admin", "GET", "/admin")
	if !d.Allow {
		t.Errorf("expected allow, got deny: %s", d.Reason)
	}
	if d.MatchedRoute != "/admin" {
		t.Errorf("MatchedRoute = %q, want /admin", d.MatchedRoute)
	}
	if d.Backend != "backend-a" {
		t.Errorf("Backend = %q, want backend-a", d.Backend)
	}
}

// TestDeniedRole verifies the right route matches but the wrong role is denied.
func TestDeniedRole(t *testing.T) {
	p := mustLoad(t, testYAML)
	d := p.Decide("user", "GET", "/admin")
	if d.Allow {
		t.Error("expected deny for user on /admin, got allow")
	}
	if d.MatchedRoute != "/admin" {
		t.Errorf("MatchedRoute = %q, want /admin", d.MatchedRoute)
	}
}

// TestDefaultDeny verifies that an unmatched path produces a deny with no matched route.
func TestDefaultDeny(t *testing.T) {
	p := mustLoad(t, testYAML)
	d := p.Decide("admin", "GET", "/no-such-path")
	if d.Allow {
		t.Error("expected default deny for unmatched path, got allow")
	}
	if d.MatchedRoute != "" {
		t.Errorf("MatchedRoute = %q, want empty for default deny", d.MatchedRoute)
	}
}

// TestWrongMethod verifies that the right path + role but wrong method is denied.
func TestWrongMethod(t *testing.T) {
	p := mustLoad(t, testYAML)
	d := p.Decide("admin", "DELETE", "/admin") // only GET and POST allowed
	if d.Allow {
		t.Error("expected deny for DELETE on /admin (only GET/POST allowed), got allow")
	}
	if d.MatchedRoute != "/admin" {
		t.Errorf("MatchedRoute = %q, want /admin", d.MatchedRoute)
	}
}

// TestMultipleRoles verifies a route that lists several roles allows any of them.
func TestMultipleRoles(t *testing.T) {
	p := mustLoad(t, testYAML)
	for _, role := range []string{"user", "admin"} {
		d := p.Decide(role, "GET", "/profile")
		if !d.Allow {
			t.Errorf("role %q: expected allow on /profile, got deny: %s", role, d.Reason)
		}
	}
}

// TestParameterPath verifies segment wildcard (:id) matching.
func TestParameterPath(t *testing.T) {
	p := mustLoad(t, testYAML)
	d := p.Decide("admin", "GET", "/users/42")
	if !d.Allow {
		t.Errorf("expected allow on /users/42 for admin, got deny: %s", d.Reason)
	}
}

// TestParameterPathMismatchDepth verifies /users/42/extra does NOT match /users/:id.
func TestParameterPathMismatchDepth(t *testing.T) {
	p := mustLoad(t, testYAML)
	d := p.Decide("admin", "GET", "/users/42/extra")
	if d.Allow {
		t.Error("expected deny for /users/42/extra against /users/:id (segment count mismatch)")
	}
}

// TestWildcardPath verifies trailing "*" prefix matching.
func TestWildcardPath(t *testing.T) {
	p := mustLoad(t, testYAML)
	for _, path := range []string{"/public/", "/public/css/main.css", "/public/images/logo.png"} {
		d := p.Decide("guest", "GET", path)
		if !d.Allow {
			t.Errorf("path %q: expected allow for guest, got deny: %s", path, d.Reason)
		}
	}
}

// TestCaseInsensitiveMethod verifies that method matching is case-insensitive.
func TestCaseInsensitiveMethod(t *testing.T) {
	p := mustLoad(t, testYAML)
	d := p.Decide("admin", "get", "/admin")
	if !d.Allow {
		t.Errorf("expected allow for lowercase method 'get', got deny: %s", d.Reason)
	}
}

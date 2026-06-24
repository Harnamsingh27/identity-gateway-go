package integration_test

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/harnamsingh/go-servicekit/auth"
	"github.com/harnamsingh/go-servicekit/httpx"
	"identity-gateway-go/internal/audit"
	"identity-gateway-go/internal/policy"
	"identity-gateway-go/internal/proxy"
	"identity-gateway-go/internal/ratelimit"
)

// testGateway bundles a running gateway httptest.Server, its echo backend, and
// the audit log output buffer so tests can assert on all three.
type testGateway struct {
	Server   *httptest.Server
	Backend  *httptest.Server
	AuditLog *bytes.Buffer
}

// testPolicy is the YAML used for all integration tests.
// It covers an admin-only route, a shared route, and a guest-accessible wildcard.
const testPolicy = `
routes:
  - path: /admin
    methods: [GET, POST]
    roles: [admin]
    backend: test-backend
  - path: /profile
    methods: [GET]
    roles: [user, admin]
    backend: test-backend
  - path: /public/*
    methods: [GET]
    roles: [user, admin, guest]
    backend: test-backend
`

// setupGateway starts a gateway test server in-process. rps and burst configure
// the rate limiter. Both servers are shut down when t finishes.
func setupGateway(t *testing.T, rps float64, burst int) *testGateway {
	t.Helper()

	// In-process echo backend.
	backend := echoBackend("test-backend")
	t.Cleanup(backend.Close)

	// Write policy to a temp file.
	policyFile := filepath.Join(t.TempDir(), "policy.yaml")
	if err := os.WriteFile(policyFile, []byte(fmt.Sprintf(testPolicy)), 0600); err != nil {
		t.Fatalf("write policy file: %v", err)
	}

	pol, err := policy.Load(policyFile)
	if err != nil {
		t.Fatalf("load policy: %v", err)
	}

	// Discard all non-audit logging so test output stays clean.
	mainLogger := slog.New(slog.NewTextHandler(io.Discard, nil))

	prx, err := proxy.NewHTTPProxy(map[string]string{"test-backend": backend.URL}, mainLogger)
	if err != nil {
		t.Fatalf("new proxy: %v", err)
	}

	var auditBuf bytes.Buffer
	auditLog := audit.New(slog.New(slog.NewJSONHandler(&auditBuf, nil)))

	rl := ratelimit.New(rps, burst)
	gh := proxy.NewGatewayHandler(prx, pol, rl, auditLog, mainLogger)

	verifier := auth.NewHMACVerifier([]byte(testSecret))
	// Chain: RequestID (outermost) → JWT auth → GatewayHandler
	handler := httpx.Chain(gh, httpx.RequestIDMiddleware, auth.JWTMiddleware(verifier))

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &testGateway{
		Server:   srv,
		Backend:  backend,
		AuditLog: &auditBuf,
	}
}

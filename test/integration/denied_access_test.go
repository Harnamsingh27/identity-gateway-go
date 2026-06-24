package integration_test

import (
	"net/http"
	"testing"
	"time"
)

func TestDeniedAccess_WrongRole_Returns403(t *testing.T) {
	tg := setupGateway(t, 100, 200)
	// "user" role is not permitted on /admin.
	token := mintToken("bob", "user", 5*time.Minute)

	resp := doRequest(t, http.MethodGet, tg.Server.URL+"/admin", token)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
	assertAudit(t, tg.AuditLog, "deny", http.StatusForbidden, "bob")
}

func TestDeniedAccess_MissingToken_Returns401(t *testing.T) {
	tg := setupGateway(t, 100, 200)

	// No Authorization header — JWT middleware returns 401 before reaching
	// GatewayHandler, so no audit entry is written.
	resp := doRequest(t, http.MethodGet, tg.Server.URL+"/admin", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	// Audit log must be empty: the JWT middleware short-circuits before the handler.
	if tg.AuditLog.Len() != 0 {
		t.Errorf("expected empty audit log for unauthenticated request, got: %s", tg.AuditLog.String())
	}
}

func TestDeniedAccess_ExpiredToken_Returns401(t *testing.T) {
	tg := setupGateway(t, 100, 200)

	resp := doRequest(t, http.MethodGet, tg.Server.URL+"/admin", expiredToken())
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	// Again, JWT middleware handles this; no audit entry expected.
	if tg.AuditLog.Len() != 0 {
		t.Errorf("expected empty audit log for expired-token request, got: %s", tg.AuditLog.String())
	}
}

func TestDeniedAccess_UnmatchedRoute_Returns403(t *testing.T) {
	tg := setupGateway(t, 100, 200)
	token := mintToken("alice", "admin", 5*time.Minute)

	// /secret is not defined in the test policy — default deny.
	resp := doRequest(t, http.MethodGet, tg.Server.URL+"/secret", token)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
	assertAudit(t, tg.AuditLog, "deny", http.StatusForbidden, "alice")
}

func TestDeniedAccess_WrongMethod_Returns403(t *testing.T) {
	tg := setupGateway(t, 100, 200)
	// /profile only allows GET; a POST should be forbidden.
	token := mintToken("alice", "admin", 5*time.Minute)

	resp := doRequest(t, http.MethodPost, tg.Server.URL+"/profile", token)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
	assertAudit(t, tg.AuditLog, "deny", http.StatusForbidden, "alice")
}

func TestDeniedAccess_RateLimitExceeded_Returns429(t *testing.T) {
	// burst=1 means one token is available immediately; the second request
	// will be denied until the bucket refills (rps=0.0001 → ~2.7 hours away).
	tg := setupGateway(t, 0.0001, 1)
	token := mintToken("alice", "admin", 5*time.Minute)

	// First request: consumes the single burst token, should be allowed.
	resp1 := doRequest(t, http.MethodGet, tg.Server.URL+"/admin", token)
	resp1.Body.Close()
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", resp1.StatusCode)
	}

	// Second request: bucket is empty, must be rate-limited.
	resp2 := doRequest(t, http.MethodGet, tg.Server.URL+"/admin", token)
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("second request: expected 429, got %d", resp2.StatusCode)
	}
	if resp2.Header.Get("Retry-After") == "" {
		t.Error("expected Retry-After header on 429 response")
	}

	// Audit log: the first request produced an "allow" entry, the second a "deny".
	// assertAudit checks the LAST line — the rate-limit deny.
	assertAudit(t, tg.AuditLog, "deny", http.StatusTooManyRequests, "alice")
}

package integration_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// doRequest is a convenience wrapper that builds and fires an HTTP request
// against the gateway and returns the response.
func doRequest(t *testing.T, method, url, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func TestAllowedAccess_AdminCanAccessAdminRoute(t *testing.T) {
	tg := setupGateway(t, 100, 200)
	token := mintToken("alice", "admin", 5*time.Minute)

	resp := doRequest(t, http.MethodGet, tg.Server.URL+"/admin", token)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// The echo backend encodes request details as JSON; verify the backend name
	// and that the gateway propagated a request ID.
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if body["backend"] != "test-backend" {
		t.Errorf("backend echo: want %q, got %v", "test-backend", body["backend"])
	}
	if rid, _ := body["request_id"].(string); rid == "" {
		t.Error("expected non-empty request_id in backend echo response")
	}

	// Audit log must record an allow decision for "alice".
	assertAudit(t, tg.AuditLog, "allow", http.StatusOK, "alice")
}

func TestAllowedAccess_UserCanAccessProfileRoute(t *testing.T) {
	tg := setupGateway(t, 100, 200)
	token := mintToken("bob", "user", 5*time.Minute)

	resp := doRequest(t, http.MethodGet, tg.Server.URL+"/profile", token)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	assertAudit(t, tg.AuditLog, "allow", http.StatusOK, "bob")
}

func TestAllowedAccess_AdminCanAccessProfileRoute(t *testing.T) {
	tg := setupGateway(t, 100, 200)
	token := mintToken("carol", "admin", 5*time.Minute)

	resp := doRequest(t, http.MethodGet, tg.Server.URL+"/profile", token)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	assertAudit(t, tg.AuditLog, "allow", http.StatusOK, "carol")
}

func TestAllowedAccess_GuestCanAccessPublicRoute(t *testing.T) {
	tg := setupGateway(t, 100, 200)
	token := mintToken("guest1", "guest", 5*time.Minute)

	resp := doRequest(t, http.MethodGet, tg.Server.URL+"/public/docs", token)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	assertAudit(t, tg.AuditLog, "allow", http.StatusOK, "guest1")
}

func TestAllowedAccess_RequestIDPropagated(t *testing.T) {
	tg := setupGateway(t, 100, 200)
	token := mintToken("alice", "admin", 5*time.Minute)

	// Provide an explicit request ID; the gateway must forward it to the backend.
	req, _ := http.NewRequest(http.MethodGet, tg.Server.URL+"/admin", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Request-ID", "test-rid-12345")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["request_id"] != "test-rid-12345" {
		t.Errorf("request_id: want %q, got %v", "test-rid-12345", body["request_id"])
	}

	// Response header must echo it back too (set by RequestIDMiddleware).
	if got := resp.Header.Get("X-Request-ID"); got != "test-rid-12345" {
		t.Errorf("X-Request-ID response header: want %q, got %q", "test-rid-12345", got)
	}
}

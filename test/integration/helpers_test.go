package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/harnamsingh/go-servicekit/auth"
)

const testSecret = "integration-test-secret"

// mintToken creates a signed HS256 JWT with the given subject and role.
func mintToken(subject, role string, ttl time.Duration) string {
	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
		},
		Role: role,
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := t.SignedString([]byte(testSecret))
	if err != nil {
		panic(fmt.Sprintf("mintToken: %v", err))
	}
	return s
}

// expiredToken creates a JWT that is already expired.
func expiredToken() string {
	return mintToken("user1", "user", -1*time.Minute)
}

// echoBackend starts an httptest.Server that echoes back request details as JSON.
// The caller is responsible for calling Close().
func echoBackend(name string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"backend":    name,
			"path":       r.URL.Path,
			"request_id": r.Header.Get("X-Request-ID"),
		})
	}))
}

// assertAudit parses the last JSON line in buf and checks the audit fields.
// wantIdentity may be empty to skip the identity check.
func assertAudit(t *testing.T, buf *bytes.Buffer, wantDecision string, wantStatus int, wantIdentity string) {
	t.Helper()

	raw := bytes.TrimSpace(buf.Bytes())
	if len(raw) == 0 {
		t.Fatal("audit log is empty — expected at least one entry")
	}

	lines := bytes.Split(raw, []byte("\n"))
	lastLine := lines[len(lines)-1]

	var entry map[string]any
	if err := json.Unmarshal(lastLine, &entry); err != nil {
		t.Fatalf("parse audit log: %v (raw: %s)", err, lastLine)
	}

	if got := entry["decision"]; got != wantDecision {
		t.Errorf("audit decision: want %q, got %v", wantDecision, got)
	}
	if got := entry["status_code"]; got != float64(wantStatus) {
		t.Errorf("audit status_code: want %d, got %v", wantStatus, got)
	}
	if wantIdentity != "" {
		if got := entry["identity"]; got != wantIdentity {
			t.Errorf("audit identity: want %q, got %v", wantIdentity, got)
		}
	}
}

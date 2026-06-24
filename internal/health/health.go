// Package health exposes /healthz and /readyz HTTP handlers.
package health

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
)

// Handler provides /healthz (liveness) and /readyz (readiness) endpoints.
type Handler struct {
	ready atomic.Bool
}

// SetReady marks the gateway as ready to serve traffic. Call this once the
// policy file has been loaded and the backends are configured.
func (h *Handler) SetReady(ready bool) {
	h.ready.Store(ready)
}

// Liveness returns 200 OK once the process is running. It never fails.
func (h *Handler) Liveness(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "alive"})
}

// Readiness returns 200 once the gateway has finished initialising, 503 otherwise.
func (h *Handler) Readiness(w http.ResponseWriter, _ *http.Request) {
	if h.ready.Load() {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
		return
	}
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

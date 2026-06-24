// Command fake-backend-b is a minimal echo HTTP service for integration testing.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
)

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8082"
	}
	http.HandleFunc("/", handler("backend-b"))
	log.Printf("fake-backend-b listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func handler(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"backend":    name,
			"path":       r.URL.Path,
			"method":     r.Method,
			"request_id": r.Header.Get("X-Request-ID"),
			"headers": map[string]string{
				"X-Request-ID": r.Header.Get("X-Request-ID"),
				"User-Agent":   r.Header.Get("User-Agent"),
			},
		})
	}
}

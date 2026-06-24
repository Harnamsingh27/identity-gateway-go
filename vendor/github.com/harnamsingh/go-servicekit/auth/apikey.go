package auth

import (
	"context"
	"net/http"

	appErrors "github.com/harnamsingh/go-servicekit/errors"
)

// APIKeyHeader is the HTTP header used for API key authentication.
const APIKeyHeader = "X-API-Key" //nolint:gosec

// KeyStore validates API keys. Implementations may check env vars, a database,
// or any other backing store.
type KeyStore interface {
	// Valid returns true if the given key is authorised.
	Valid(ctx context.Context, key string) (bool, error)
}

// MemoryKeyStore is an in-memory KeyStore backed by a fixed set of keys.
// Intended for testing and demo purposes.
type MemoryKeyStore struct {
	keys map[string]struct{}
}

// NewMemoryKeyStore creates a MemoryKeyStore pre-loaded with the given keys.
func NewMemoryKeyStore(keys ...string) *MemoryKeyStore {
	m := &MemoryKeyStore{keys: make(map[string]struct{}, len(keys))}
	for _, k := range keys {
		m.keys[k] = struct{}{}
	}
	return m
}

// Valid reports whether key is in the store.
func (s *MemoryKeyStore) Valid(_ context.Context, key string) (bool, error) {
	_, ok := s.keys[key]
	return ok, nil
}

// AddKey inserts a key into the store.
func (s *MemoryKeyStore) AddKey(key string) { s.keys[key] = struct{}{} }

// RemoveKey deletes a key from the store.
func (s *MemoryKeyStore) RemoveKey(key string) { delete(s.keys, key) }

// APIKeyMiddleware returns an HTTP middleware that validates the X-API-Key header
// against the provided KeyStore. On success the handler is called; on failure a
// 401 response is written.
func APIKeyMiddleware(store KeyStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get(APIKeyHeader)
			if key == "" {
				http.Error(w, ErrMissingAPIKey.Message, appErrors.ToHTTPStatus(ErrMissingAPIKey))
				return
			}
			ok, err := store.Valid(r.Context(), key)
			if err != nil {
				http.Error(w, "key store error", http.StatusInternalServerError)
				return
			}
			if !ok {
				http.Error(w, ErrInvalidAPIKey.Message, appErrors.ToHTTPStatus(ErrInvalidAPIKey))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

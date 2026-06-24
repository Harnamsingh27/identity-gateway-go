// Package ratelimit implements a per-identity token-bucket rate limiter.
package ratelimit

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Limiter wraps per-identity token buckets and provides an HTTP middleware.
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]*rate.Limiter
	rps     rate.Limit
	burst   int
}

// New creates a Limiter with the given steady-state rate (requests per second)
// and burst capacity.
func New(requestsPerSecond float64, burst int) *Limiter {
	return &Limiter{
		buckets: make(map[string]*rate.Limiter),
		rps:     rate.Limit(requestsPerSecond),
		burst:   burst,
	}
}

// Allow reports whether the named identity should be allowed to proceed.
// It returns the time after which the caller may retry when not allowed.
func (l *Limiter) Allow(identity string) (ok bool, retryAfter time.Duration) {
	lim := l.getLimiter(identity)
	r := lim.Reserve()
	if !r.OK() {
		return false, 0
	}
	delay := r.Delay()
	if delay > 0 {
		r.Cancel()
		return false, delay
	}
	return true, 0
}

func (l *Limiter) getLimiter(identity string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	if lim, ok := l.buckets[identity]; ok {
		return lim
	}
	lim := rate.NewLimiter(l.rps, l.burst)
	l.buckets[identity] = lim
	return lim
}

// Middleware returns an HTTP middleware that enforces the rate limit. The
// identity key is derived from the caller — prefer the JWT subject claim when
// available; fall back to remote IP.
func (l *Limiter) Middleware(identityFn func(r *http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity := identityFn(r)
			ok, retryAfter := l.Allow(identity)
			if !ok {
				secs := int(retryAfter.Seconds()) + 1
				w.Header().Set("Retry-After", fmt.Sprintf("%d", secs))
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

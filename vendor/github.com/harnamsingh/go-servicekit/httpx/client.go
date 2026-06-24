package httpx

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/harnamsingh/go-servicekit/observability"
)

const (
	defaultClientTimeout    = 30 * time.Second
	defaultRetryMax         = 3
	defaultRetryBaseBackoff = 100 * time.Millisecond
)

// ClientOption configures a Client.
type ClientOption func(*clientConfig)

type clientConfig struct {
	timeout     time.Duration
	retryMax    int
	baseBackoff time.Duration
	transport   http.RoundTripper
}

// WithClientTimeout sets the per-request timeout.
func WithClientTimeout(d time.Duration) ClientOption {
	return func(c *clientConfig) { c.timeout = d }
}

// WithRetry sets the maximum number of retry attempts and the initial backoff.
// Backoff doubles on each retry (exponential).
func WithRetry(maxAttempts int, baseBackoff time.Duration) ClientOption {
	return func(c *clientConfig) { c.retryMax = maxAttempts; c.baseBackoff = baseBackoff }
}

// WithTransport replaces the underlying http.RoundTripper.
func WithTransport(t http.RoundTripper) ClientOption {
	return func(c *clientConfig) { c.transport = t }
}

// Client is an HTTP client with retries, timeout, and automatic X-Request-ID
// header propagation from context.
type Client struct {
	inner       *http.Client
	retryMax    int
	baseBackoff time.Duration
}

// NewClient creates a Client with sensible defaults.
func NewClient(opts ...ClientOption) *Client {
	cfg := &clientConfig{
		timeout:     defaultClientTimeout,
		retryMax:    defaultRetryMax,
		baseBackoff: defaultRetryBaseBackoff,
		transport:   http.DefaultTransport,
	}
	for _, o := range opts {
		o(cfg)
	}
	return &Client{
		inner:       &http.Client{Timeout: cfg.timeout, Transport: cfg.transport},
		retryMax:    cfg.retryMax,
		baseBackoff: cfg.baseBackoff,
	}
}

// Do executes req with retry on transient errors. The request ID from ctx is
// propagated via the X-Request-ID header.
func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if id := observability.RequestIDFromContext(ctx); id != "" {
		req.Header.Set(observability.RequestIDHeader, id)
	}

	var (
		resp    *http.Response
		lastErr error
		backoff = c.baseBackoff
	)
	for attempt := 0; attempt <= c.retryMax; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("httpx: client: context cancelled: %w", ctx.Err())
			case <-time.After(backoff):
			}
			backoff *= 2
		}

		resp, lastErr = c.inner.Do(req.WithContext(ctx))
		if lastErr != nil {
			// Network error — retry.
			continue
		}
		if resp.StatusCode < http.StatusInternalServerError {
			return resp, nil
		}
		// 5xx — close body and retry.
		resp.Body.Close() //nolint:errcheck,gosec
		lastErr = fmt.Errorf("httpx: server returned %d", resp.StatusCode)
	}
	return nil, fmt.Errorf("httpx: all %d attempts failed: %w", c.retryMax+1, lastErr)
}

// Get is a convenience wrapper for GET requests.
func (c *Client) Get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("httpx: build request: %w", err)
	}
	return c.Do(ctx, req)
}

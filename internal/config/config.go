// Package config defines the gateway configuration struct and loader.
package config

import (
	"time"

	"github.com/harnamsingh/go-servicekit/config"
)

// BackendTarget maps a backend key (used in policy.yaml) to a base URL.
type BackendTarget struct {
	Key     string `yaml:"key"     env:""`
	BaseURL string `yaml:"base_url" env:""`
}

// RateLimitConfig controls token-bucket parameters.
type RateLimitConfig struct {
	// RequestsPerSecond is the steady-state refill rate per identity.
	RequestsPerSecond float64 `yaml:"requests_per_second" env:"GATEWAY_RL_RPS"    default:"10"`
	// Burst is the maximum number of requests that can be served in a burst.
	Burst int `yaml:"burst"              env:"GATEWAY_RL_BURST"  default:"20"`
}

// JWTConfig holds the JWT verification settings.
type JWTConfig struct {
	// Secret is the HMAC-SHA256 key used for HS256 tokens.
	Secret string `yaml:"secret" env:"GATEWAY_JWT_SECRET" validate:"required"`
}

// OTelConfig holds OpenTelemetry exporter settings.
type OTelConfig struct {
	// OTLPEndpoint is the gRPC endpoint for OTLP export (e.g. "localhost:4317").
	// Empty = no-op tracer.
	OTLPEndpoint string `yaml:"otlp_endpoint" env:"GATEWAY_OTLP_ENDPOINT"`
	// ServiceName is the OTel service name.
	ServiceName string `yaml:"service_name" env:"GATEWAY_SERVICE_NAME" default:"identity-gateway"`
	// ServiceVersion is included in OTel resource attributes.
	ServiceVersion string `yaml:"service_version" env:"GATEWAY_SERVICE_VERSION" default:"v0.1.0"`
}

// GRPCConfig holds settings for the gRPC pass-through listener.
type GRPCConfig struct {
	// Addr is the listen address for the gRPC server (e.g. ":9090").
	Addr string `yaml:"addr" env:"GATEWAY_GRPC_ADDR" default:":9090"`
}

// Gateway is the top-level gateway configuration.
type Gateway struct {
	// Addr is the HTTP listen address (e.g. ":8080").
	Addr string `yaml:"addr" env:"GATEWAY_ADDR" default:":8080"`
	// PolicyFile is the path to the YAML policy file.
	PolicyFile string `yaml:"policy_file" env:"GATEWAY_POLICY_FILE" default:"policy.example.yaml"`
	// Backends maps backend keys to base URLs.
	// The map keys correspond to the "backend" field in policy routes.
	Backends map[string]string `yaml:"backends"`
	// RateLimit configures the per-identity token-bucket limiter.
	RateLimit RateLimitConfig `yaml:"rate_limit"`
	// JWT holds JWT verification settings.
	JWT JWTConfig `yaml:"jwt"`
	// OTel holds observability settings.
	OTel OTelConfig `yaml:"otel"`
	// GRPC holds gRPC listener settings.
	GRPC GRPCConfig `yaml:"grpc"`
	// ShutdownTimeout is how long to wait for in-flight requests on shutdown.
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout" env:"GATEWAY_SHUTDOWN_TIMEOUT" default:"10s"`
}

// Load reads the YAML file at path and returns a Gateway config, applying
// environment variable overrides on top.
func Load(path string) (Gateway, error) {
	return config.Load[Gateway](
		config.WithYAMLFile(path),
	)
}

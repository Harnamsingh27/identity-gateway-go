
package observability

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// MetricsOption configures InitMetrics.
type MetricsOption func(*metricsConfig)

type metricsConfig struct {
	otlpEndpoint string
	stdout       bool
}

// WithOTLPMetricsEndpoint sets the OTLP gRPC endpoint for metric export.
func WithOTLPMetricsEndpoint(endpoint string) MetricsOption {
	return func(c *metricsConfig) { c.otlpEndpoint = endpoint }
}

// WithStdoutMetrics writes metrics to stdout. Intended for local development.
func WithStdoutMetrics() MetricsOption {
	return func(c *metricsConfig) { c.stdout = true }
}

// InitMetrics initialises and globally registers an OpenTelemetry MeterProvider.
// The returned shutdown function must be called before the process exits.
func InitMetrics(serviceName string, opts ...MetricsOption) (shutdown func(context.Context) error, err error) {
	cfg := &metricsConfig{}
	for _, o := range opts {
		o(cfg)
	}

	res, err := resource.New(context.Background(),
		resource.WithAttributes(semconv.ServiceName(serviceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("observability: build metric resource: %w", err)
	}

	var reader sdkmetric.Reader
	switch {
	case cfg.stdout:
		exp, expErr := stdoutmetric.New()
		if expErr != nil {
			return nil, fmt.Errorf("observability: stdout metric exporter: %w", expErr)
		}
		reader = sdkmetric.NewPeriodicReader(exp)
	case cfg.otlpEndpoint != "":
		exp, expErr := otlpmetricgrpc.New(context.Background(),
			otlpmetricgrpc.WithEndpoint(cfg.otlpEndpoint),
			otlpmetricgrpc.WithInsecure(),
		)
		if expErr != nil {
			return nil, fmt.Errorf("observability: otlp metric exporter: %w", expErr)
		}
		reader = sdkmetric.NewPeriodicReader(exp)
	default:
		return nil, fmt.Errorf("observability: no metric exporter configured")
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	return func(ctx context.Context) error {
		if err := mp.Shutdown(ctx); err != nil {
			return fmt.Errorf("observability: metrics shutdown: %w", err)
		}
		return nil
	}, nil
}

// Meter returns a named meter from the globally registered MeterProvider.
func Meter(name string) metric.Meter {
	return otel.Meter(name)
}

// HTTPMetrics holds OTel instruments for HTTP servers.
type HTTPMetrics struct {
	RequestCount    metric.Int64Counter
	RequestDuration metric.Float64Histogram
	InFlight        metric.Int64UpDownCounter
}

// NewHTTPMetrics creates the standard set of HTTP server instruments.
func NewHTTPMetrics(meter metric.Meter) (HTTPMetrics, error) {
	count, err := meter.Int64Counter("http.server.request.count",
		metric.WithDescription("Total number of HTTP requests received"))
	if err != nil {
		return HTTPMetrics{}, err
	}
	dur, err := meter.Float64Histogram("http.server.request.duration",
		metric.WithDescription("HTTP request duration in seconds"),
		metric.WithUnit("s"))
	if err != nil {
		return HTTPMetrics{}, err
	}
	inflight, err := meter.Int64UpDownCounter("http.server.active_requests",
		metric.WithDescription("Number of in-flight HTTP requests"))
	if err != nil {
		return HTTPMetrics{}, err
	}
	return HTTPMetrics{RequestCount: count, RequestDuration: dur, InFlight: inflight}, nil
}

// statusRecorder wraps http.ResponseWriter to capture the status code.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// HTTPMetricsMiddleware returns a middleware that records request count,
// duration, and in-flight count using the provided instruments.
func HTTPMetricsMiddleware(m HTTPMetrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attrs := []attribute.KeyValue{
				attribute.String("http.method", r.Method),
				attribute.String("http.route", r.URL.Path),
			}
			m.InFlight.Add(r.Context(), 1, metric.WithAttributes(attrs...))
			defer m.InFlight.Add(r.Context(), -1, metric.WithAttributes(attrs...))

			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()
			next.ServeHTTP(rec, r)
			dur := time.Since(start).Seconds()

			statusAttrs := append(attrs, attribute.Int("http.status_code", rec.status))
			m.RequestCount.Add(r.Context(), 1, metric.WithAttributes(statusAttrs...))
			m.RequestDuration.Record(r.Context(), dur, metric.WithAttributes(statusAttrs...))
		})
	}
}

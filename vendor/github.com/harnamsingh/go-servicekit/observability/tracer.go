
package observability

import (
	"context"
	"fmt"
	"io"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// TracerOption configures InitTracer.
type TracerOption func(*tracerConfig)

type tracerConfig struct {
	otlpEndpoint string
	stdout       bool
	stdoutWriter io.Writer
	noop         bool
}

// WithOTLPEndpoint sets the OTLP gRPC endpoint (e.g. "localhost:4317").
func WithOTLPEndpoint(endpoint string) TracerOption {
	return func(c *tracerConfig) { c.otlpEndpoint = endpoint }
}

// WithStdoutTracer writes spans to the provided writer (default os.Stdout).
// Intended for local development. Pass nil to use os.Stdout.
func WithStdoutTracer(w io.Writer) TracerOption {
	return func(c *tracerConfig) { c.stdout = true; c.stdoutWriter = w }
}

// WithNoopTracer installs a no-op tracer. Useful in tests that do not want
// real spans but still exercise instrumented code paths.
func WithNoopTracer() TracerOption {
	return func(c *tracerConfig) { c.noop = true }
}

// InitTracer initialises and globally registers an OpenTelemetry TracerProvider.
// It returns a shutdown function that must be called before the process exits
// to flush buffered spans.
//
//	shutdown, err := observability.InitTracer("my-service",
//	    observability.WithOTLPEndpoint("otelcol:4317"),
//	)
//	if err != nil { ... }
//	defer shutdown(ctx)
func InitTracer(serviceName string, opts ...TracerOption) (shutdown func(context.Context) error, err error) {
	cfg := &tracerConfig{}
	for _, o := range opts {
		o(cfg)
	}

	if cfg.noop {
		otel.SetTracerProvider(noop.NewTracerProvider())
		return func(context.Context) error { return nil }, nil
	}

	res, err := resource.New(context.Background(),
		resource.WithAttributes(semconv.ServiceName(serviceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("observability: build resource: %w", err)
	}

	var exporter sdktrace.SpanExporter
	switch {
	case cfg.stdout:
		stdoutOpts := []stdouttrace.Option{stdouttrace.WithPrettyPrint()}
		if cfg.stdoutWriter != nil {
			stdoutOpts = append(stdoutOpts, stdouttrace.WithWriter(cfg.stdoutWriter))
		}
		exporter, err = stdouttrace.New(stdoutOpts...)
	case cfg.otlpEndpoint != "":
		exporter, err = otlptracegrpc.New(context.Background(),
			otlptracegrpc.WithEndpoint(cfg.otlpEndpoint),
			otlptracegrpc.WithInsecure(),
		)
	default:
		return nil, fmt.Errorf("observability: no trace exporter configured; use WithOTLPEndpoint or WithStdoutTracer")
	}
	if err != nil {
		return nil, fmt.Errorf("observability: build trace exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(sdktrace.NewBatchSpanProcessor(exporter)),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	return func(ctx context.Context) error {
		if err := tp.Shutdown(ctx); err != nil {
			return fmt.Errorf("observability: tracer shutdown: %w", err)
		}
		return nil
	}, nil
}

// Tracer returns a named tracer from the globally registered TracerProvider.
func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}

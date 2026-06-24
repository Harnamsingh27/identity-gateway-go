// Command gateway is the identity-gateway process.
package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/harnamsingh/go-servicekit/auth"
	"github.com/harnamsingh/go-servicekit/httpx"
	"github.com/harnamsingh/go-servicekit/observability"

	"identity-gateway-go/internal/audit"
	gatewayconfig "identity-gateway-go/internal/config"
	"identity-gateway-go/internal/health"
	"identity-gateway-go/internal/policy"
	"identity-gateway-go/internal/proxy"
	"identity-gateway-go/internal/ratelimit"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "path to gateway config YAML")
	flag.Parse()

	logger := observability.NewLogger(observability.WithLogLevel(slog.LevelInfo))

	cfg, err := gatewayconfig.Load(*cfgPath)
	if err != nil {
		logger.Error("failed to load config", slog.String("error", err.Error()))
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Set up OTel tracer (no-op when no OTLP endpoint is configured).
	tracerOpts := []observability.TracerOption{observability.WithNoopTracer()}
	if cfg.OTel.OTLPEndpoint != "" {
		tracerOpts = []observability.TracerOption{
			observability.WithOTLPEndpoint(cfg.OTel.OTLPEndpoint),
		}
	}
	shutdownTrace, err := observability.InitTracer(cfg.OTel.ServiceName, tracerOpts...)
	if err != nil {
		logger.Error("tracer setup failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer func() { _ = shutdownTrace(context.Background()) }()

	// Set up OTel metrics only when an OTLP endpoint is configured.
	shutdownMetrics := func(context.Context) error { return nil }
	if cfg.OTel.OTLPEndpoint != "" {
		shutdownMetrics, err = observability.InitMetrics(cfg.OTel.ServiceName,
			observability.WithOTLPMetricsEndpoint(cfg.OTel.OTLPEndpoint))
		if err != nil {
			logger.Error("metrics setup failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}
	defer func() { _ = shutdownMetrics(context.Background()) }()

	pol, err := policy.Load(cfg.PolicyFile)
	if err != nil {
		logger.Error("failed to load policy",
			slog.String("file", cfg.PolicyFile), slog.String("error", err.Error()))
		os.Exit(1)
	}

	httpProxy, err := proxy.NewHTTPProxy(cfg.Backends, logger)
	if err != nil {
		logger.Error("proxy init failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	auditLog := audit.New(logger.With(slog.String("component", "audit")))
	rl := ratelimit.New(cfg.RateLimit.RequestsPerSecond, cfg.RateLimit.Burst)
	verifier := auth.NewHMACVerifier([]byte(cfg.JWT.Secret))
	gwHandler := proxy.NewGatewayHandler(httpProxy, pol, rl, auditLog, logger)
	hh := &health.Handler{}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", hh.Liveness)
	mux.HandleFunc("/readyz", hh.Readiness)
	mux.Handle("/", auth.JWTMiddleware(verifier)(gwHandler))

	// Chain: request-ID → access-log → mux.
	// observability.RequestIDMiddleware is outermost; AccessLogMiddleware sits inside it.
	handler := observability.RequestIDMiddleware(httpx.AccessLogMiddleware(logger)(mux))

	hh.SetReady(true)

	srv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown: wait for signal, then drain in-flight requests.
	go func() {
		<-ctx.Done()
		shutCtx, shutCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer shutCancel()
		_ = srv.Shutdown(shutCtx)
	}()

	logger.Info("gateway listening", slog.String("addr", cfg.Addr))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

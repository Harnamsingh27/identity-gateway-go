# Changelog

All notable changes to this project will be documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
This project uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] — 2026-06-24

### Added

- **JWT authentication** — HS256 Bearer token validation via
  `go-servicekit/auth.HMACVerifier`; missing or invalid tokens return 401.
- **YAML RBAC policy engine** — `internal/policy` loads a YAML file and
  evaluates `(role, method, path)` triples; supports exact paths, `:param`
  wildcards, and trailing `*` wildcards; default-deny.
- **Per-identity rate limiter** — `internal/ratelimit` implements a
  token-bucket (`golang.org/x/time/rate`) keyed on JWT subject; returns 429
  with a `Retry-After` header.
- **Structured audit log** — `internal/audit` writes one JSON line per
  gateway decision (allow or deny) including identity, role, method, path,
  matched route, decision, reason, latency, backend, and status code.
- **HTTP reverse proxy** — `internal/proxy` uses `net/http/httputil.ReverseProxy`
  to forward allowed requests to named backend services; propagates
  `X-Request-ID`; falls back to any registered backend when the policy key
  is unknown.
- **GatewayHandler** — wires rate-limiter → policy → audit → proxy in a
  single `http.Handler`.
- **Health endpoints** — `GET /healthz` (liveness, always 200) and
  `GET /readyz` (readiness, 200 or 503).
- **`cmd/gateway`** — entry point with `--config` flag; graceful shutdown via
  `SIGINT`/`SIGTERM`; OTel tracer (no-op when no OTLP endpoint configured).
- **Fake echo backends** — three small HTTP servers (`services/fake-backend-{a,b,c}`)
  that echo request details as JSON; used for local development and Compose.
- **Docker Compose stack** — multi-stage Dockerfile (distroless runtime) plus
  `docker-compose.yml` for the gateway and all three fake backends.
- **Integration tests** — 11 in-process end-to-end tests covering allowed
  access (admin/user/guest roles, request-ID propagation) and all denial
  paths (wrong role, missing token, expired token, unmatched route, wrong
  method, rate-limit exceeded); each denial asserts the audit log entry.
- **CI workflow** — GitHub Actions pipeline: lint → unit tests → integration
  tests → binary build → Docker Compose smoke test.

[0.1.0]: https://github.com/harnamsingh/identity-gateway-go/releases/tag/v0.1.0

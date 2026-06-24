# identity-gateway-go

A zero-trust HTTP gateway written in Go. Every inbound request is authenticated
via JWT, evaluated against a YAML RBAC policy, rate-limited per caller identity,
and audit-logged as structured JSON before being forwarded to a backend service.

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                        identity-gateway                       │
│                                                              │
│  client ──► RequestIDMiddleware                              │
│                   │                                          │
│                   ▼                                          │
│             JWTMiddleware  (401 if missing/invalid)          │
│                   │                                          │
│                   ▼                                          │
│            RateLimiter     (429 + Retry-After if exceeded)   │
│                   │                                          │
│                   ▼                                          │
│          Policy.Decide()   (403 if role/route denied)        │
│                   │                                          │
│                   ▼                                          │
│          AuditLogger.Log() (structured JSON, every decision) │
│                   │                                          │
│                   ▼                                          │
│         ReverseProxy.Forward() ──────────────────────────────┼──► backend-a
│                                                              │        backend-b
│  /healthz  /readyz  (unauthenticated)                        │        backend-c
└──────────────────────────────────────────────────────────────┘
```

### Request flow

| Step | Component | On failure |
|------|-----------|-----------|
| 1 | `RequestIDMiddleware` — reuses `X-Request-ID` header or generates one | — |
| 2 | `JWTMiddleware` — validates HS256 Bearer token, stores claims in ctx | `401` |
| 3 | `RateLimiter.Allow(identity)` — per-subject token bucket | `429 + Retry-After` |
| 4 | `Policy.Decide(role, method, path)` — YAML RBAC evaluation | `403` |
| 5 | `AuditLogger.Log(...)` — one JSON line per decision (allow or deny) | — |
| 6 | `ReverseProxy.Forward(backend)` — strips hop-by-hop headers, propagates request ID | `502` |

## Prerequisites

- Go 1.23+
- [go-servicekit](../Go%20ServiceKit) checked out as a sibling directory (required for local builds via `go.work`)
- Docker + Docker Compose (for the containerised stack)

## Local run

```bash
# 1. Confirm the workspace can see go-servicekit
go env GOWORK   # should print the path to go.work at the Desktop level

# 2. Copy and edit config
cp config.example.yaml config.yaml
# Set at minimum: jwt.secret and backends.*

# 3. Copy and edit policy
cp policy.example.yaml policy.yaml

# 4. Build and run
make build
./bin/gateway --config config.yaml
```

The gateway listens on `:8080` (HTTP) and `:9090` (gRPC) by default.

```
GET  /healthz   → 200 {"status":"alive"}
GET  /readyz    → 200 {"status":"ready"} | 503 {"status":"not_ready"}
```

## Docker Compose

The Compose stack builds and starts all four services from source:

```bash
# Start
docker compose up --build -d

# Stop
docker compose down
```

| Service | Port | Description |
|---------|------|-------------|
| `gateway` | 8080 (HTTP), 9090 (gRPC) | Identity gateway |
| `fake-backend-a` | 8081 | Echo backend for `/admin` and `/public/*` |
| `fake-backend-b` | 8082 | Echo backend for `/profile` |
| `fake-backend-c` | 8083 | Echo backend for `/users/:id` |

The Compose file sets `GATEWAY_JWT_SECRET=integration-test-secret` so you can
use the JWT examples below immediately.

## Policy file

`policy.example.yaml` shows the full syntax:

```yaml
routes:
  - path: /admin          # exact match
    methods: [GET, POST]
    roles: [admin]
    backend: backend-a

  - path: /users/:id      # :param matches any single path segment
    methods: [GET, PUT, DELETE]
    roles: [admin]
    backend: backend-c

  - path: /public/*       # * matches any suffix (one or more segments)
    methods: [GET]
    roles: [user, admin, guest]
    backend: backend-a
```

**Matching rules:**

| Pattern | Matches | Does not match |
|---------|---------|---------------|
| `/admin` | `/admin` | `/admin/` `/admin/foo` |
| `/users/:id` | `/users/123` `/users/abc` | `/users/` `/users/a/b` |
| `/public/*` | `/public/` `/public/docs/api` | `/public` |

The default action is **deny** — any request that matches no route returns 403.

The gateway reads the policy file at startup. To reload without restarting,
replace `policy.yaml` on disk and send `SIGHUP` (hot-reload is wired via
`Policy.Reload`).

## Generating JWTs for testing

The gateway verifies HS256 tokens. The claims struct expects a top-level
`"role"` field alongside the standard JWT fields.

**Using the Go playground / a small script:**

```go
package main

import (
    "fmt"
    "time"
    "github.com/golang-jwt/jwt/v5"
)

func main() {
    secret := []byte("integration-test-secret")
    claims := jwt.MapClaims{
        "sub":  "alice",
        "role": "admin",
        "exp":  time.Now().Add(24 * time.Hour).Unix(),
    }
    t, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
    fmt.Println(t)
}
```

**Using the gateway against the Compose stack:**

```bash
TOKEN=$(go run ./scripts/mintjwt/main.go)   # if you add such a helper
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/admin
```

**Using jwt.io:** paste the secret `integration-test-secret`, select HS256,
and add `"role": "admin"` to the payload.

## Configuration reference

| YAML key | Env var | Default | Description |
|----------|---------|---------|-------------|
| `addr` | `GATEWAY_ADDR` | `:8080` | HTTP listen address |
| `policy_file` | `GATEWAY_POLICY_FILE` | `policy.example.yaml` | Path to RBAC policy |
| `jwt.secret` | `GATEWAY_JWT_SECRET` | *(required)* | HMAC-SHA256 signing secret |
| `rate_limit.requests_per_second` | `GATEWAY_RL_RPS` | `10` | Steady-state refill rate per identity |
| `rate_limit.burst` | `GATEWAY_RL_BURST` | `20` | Burst capacity per identity |
| `otel.otlp_endpoint` | `GATEWAY_OTLP_ENDPOINT` | *(empty = no-op)* | OTLP gRPC endpoint |
| `otel.service_name` | `GATEWAY_SERVICE_NAME` | `identity-gateway` | OTel resource attribute |
| `grpc.addr` | `GATEWAY_GRPC_ADDR` | `:9090` | gRPC listen address |
| `shutdown_timeout` | `GATEWAY_SHUTDOWN_TIMEOUT` | `10s` | Graceful shutdown window |

## Audit log

Every request produces one JSON line on stdout, whether allowed or denied:

```json
{
  "time": "2025-01-15T12:00:00Z",
  "level": "INFO",
  "msg": "audit",
  "request_id": "a3f8e2b1c4d5",
  "identity": "alice",
  "role": "admin",
  "method": "GET",
  "path": "/admin",
  "matched_route": "/admin",
  "decision": "allow",
  "reason": "allowed",
  "latency": 1234567,
  "backend": "backend-a",
  "status_code": 200
}
```

`decision` is always `"allow"` or `"deny"`. Denied requests have an empty
`backend` field and a human-readable `reason`.

## Make targets

| Target | Description |
|--------|-------------|
| `make build` | Compile `bin/gateway` |
| `make test` | Run all unit tests |
| `make integration` | Run integration tests |
| `make lint` | Run golangci-lint |
| `make cover` | Generate HTML coverage report |
| `make docker-up` | Build and start the Compose stack |
| `make docker-down` | Tear down the Compose stack |
| `make clean` | Remove build artifacts |

## Project layout

```
cmd/gateway/          — main entry point
internal/
  config/             — YAML + env config loader
  policy/             — YAML RBAC engine (Load, Decide, hot-reload)
  audit/              — structured JSON audit logger
  ratelimit/          — per-identity token-bucket limiter
  proxy/              — HTTP reverse proxy + GatewayHandler
  health/             — /healthz and /readyz handlers
services/
  fake-backend-{a,b,c}/ — echo servers for local development
test/integration/     — in-process end-to-end tests
```

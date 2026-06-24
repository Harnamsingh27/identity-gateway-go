# syntax=docker/dockerfile:1

# ── Builder ──────────────────────────────────────────────────────────────────
FROM golang:1.23-alpine AS builder

WORKDIR /workspace

# Copy the go-servicekit source so the replace directive resolves inside Docker.
COPY ../Go\ ServiceKit/ /go-servicekit/

# Copy gateway source.
COPY . .

# Write a go.work that maps the workspace inside the container.
RUN printf 'go 1.26\nuse (\n\t.\n\t/go-servicekit\n)\n' > /workspace/go.work && \
    go mod download

# Build each binary with a distinct name.
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/gateway          ./cmd/gateway/...
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/fake-backend-a   ./services/fake-backend-a/...
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/fake-backend-b   ./services/fake-backend-b/...
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/fake-backend-c   ./services/fake-backend-c/...

# ── Gateway runtime ───────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12 AS gateway
COPY --from=builder /out/gateway /app/gateway
COPY policy.example.yaml          /app/policy.example.yaml
COPY config.example.yaml          /app/config.yaml
WORKDIR /app
ENTRYPOINT ["/app/gateway", "--config", "/app/config.yaml"]

# ── Fake Backend A ────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12 AS fake-backend-a
COPY --from=builder /out/fake-backend-a /fake-backend-a
ENTRYPOINT ["/fake-backend-a"]

# ── Fake Backend B ────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12 AS fake-backend-b
COPY --from=builder /out/fake-backend-b /fake-backend-b
ENTRYPOINT ["/fake-backend-b"]

# ── Fake Backend C ────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12 AS fake-backend-c
COPY --from=builder /out/fake-backend-c /fake-backend-c
ENTRYPOINT ["/fake-backend-c"]

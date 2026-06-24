.PHONY: all lint test test-race build run clean docker-up docker-down

GO      := go
GOFLAGS :=
LINT    := golangci-lint
BINARY  := bin/gateway

all: lint test build

lint:
	$(LINT) run ./...

test:
	$(GO) test $(GOFLAGS) ./...

test-race:
	$(GO) test -race $(GOFLAGS) ./...

cover:
	$(GO) test -coverprofile=coverage.out -covermode=atomic ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

build:
	$(GO) build -o $(BINARY) ./cmd/gateway/...

run: build
	./$(BINARY) --config policy.example.yaml

integration:
	$(GO) test $(GOFLAGS) ./test/integration/...

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down

clean:
	rm -f $(BINARY) coverage.out coverage.html

.DEFAULT_GOAL := all

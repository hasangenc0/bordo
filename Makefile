.PHONY: all build build-cp build-cli build-fleet test lint fmt clean board help

# ── Variables ────────────────────────────────────────────────────────────────
GOFLAGS   ?= -trimpath
OUT       ?= $(CURDIR)/bin

# Version metadata injected into binaries at build time.
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BTIME     ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
CP_PKG    := github.com/bordo-io/bordo/control-plane/internal/version
CLI_PKG   := github.com/bordo-io/bordo/cli/cmd

LDFLAGS   ?= -s -w \
	-X $(CP_PKG).Version=$(VERSION) \
	-X $(CP_PKG).Commit=$(COMMIT) \
	-X $(CP_PKG).BuildTime=$(BTIME) \
	-X $(CLI_PKG).Version=$(VERSION) \
	-X $(CLI_PKG).Commit=$(COMMIT) \
	-X $(CLI_PKG).BuildTime=$(BTIME)

# ── Default ──────────────────────────────────────────────────────────────────
all: build

## build: Build all Go binaries into ./bin/
build: build-cp build-cli build-fleet

build-cp:
	@echo "→ building bordod (control-plane)"
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(OUT)/bordod ./control-plane

build-cli:
	@echo "→ building bordo (CLI)"
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(OUT)/bordo ./cli

build-fleet:
	@echo "→ building bordo-fleet (fleet agent)"
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(OUT)/bordo-fleet ./fleet

## test: Run all Go tests
test:
	go test ./...

## lint: Run golangci-lint
lint:
	golangci-lint run ./...

## fmt: Format all Go and TypeScript code
fmt:
	gofmt -w .
	goimports -w .

## board: Regenerate tracker/BOARD.md from issue files
board:
	@go run ./scripts/gen-board.go --root .

## clean: Remove build artifacts
clean:
	rm -rf $(OUT)

## help: Show this help
help:
	@grep -E '^##' Makefile | sed 's/^## //'

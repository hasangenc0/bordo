.PHONY: all build build-cp build-cli build-fleet test lint fmt clean board help

# ── Variables ────────────────────────────────────────────────────────────────
GOFLAGS   ?= -trimpath
LDFLAGS   ?= -s -w
OUT       ?= $(CURDIR)/bin

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
## (placeholder — BRD-091 implements the real generator)
board:
	@echo "board generator not yet implemented (see BRD-091)"
	@echo "edit tracker/BOARD.md manually for now"

## clean: Remove build artifacts
clean:
	rm -rf $(OUT)

## help: Show this help
help:
	@grep -E '^##' Makefile | sed 's/^## //'

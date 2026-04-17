.PHONY: help build test test-unit test-integration lint fmt vet clean install tidy tools release-dry all

BINARY_NAME := wrtbox
MODULE := github.com/itslavrov/wrtbox
CMD_DIR := ./cmd/wrtbox
BIN_DIR := ./bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -s -w \
	-X '$(MODULE)/internal/version.Version=$(VERSION)' \
	-X '$(MODULE)/internal/version.Commit=$(COMMIT)' \
	-X '$(MODULE)/internal/version.BuildDate=$(BUILD_DATE)'

GO := go
GOFLAGS := -trimpath

help: ## Show this help message
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

all: fmt vet lint test build ## Run full local pipeline

build: ## Build wrtbox binary for current platform
	@mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME) $(CMD_DIR)

install: ## Install wrtbox to $GOBIN or $GOPATH/bin
	$(GO) install $(GOFLAGS) -ldflags="$(LDFLAGS)" $(CMD_DIR)

test: test-unit ## Run all non-integration tests

test-unit: ## Run unit tests with race detector
	$(GO) test -race -count=1 -covermode=atomic -coverprofile=coverage.out ./internal/... ./cmd/...

test-integration: ## Run integration tests against a running emu VM (requires EMU_* env + openrc)
	$(GO) test -count=1 -tags=integration -timeout=30m ./internal/apply/...

coverage: test-unit ## Show coverage summary
	$(GO) tool cover -func=coverage.out | tail -1
	@echo "HTML report: $(GO) tool cover -html=coverage.out"

lint: ## Run golangci-lint (install: brew install golangci-lint)
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed: brew install golangci-lint"; exit 1; }
	golangci-lint run ./...

fmt: ## Format code with gofmt
	$(GO) fmt ./...

vet: ## Run go vet
	$(GO) vet ./...

tidy: ## go mod tidy
	$(GO) mod tidy

tools: ## Install dev tools
	@echo "Install golangci-lint: brew install golangci-lint"
	@echo "Install goreleaser:    brew install goreleaser"

release-dry: ## Dry-run a release (no publish)
	@command -v goreleaser >/dev/null 2>&1 || { echo "goreleaser not installed: brew install goreleaser"; exit 1; }
	goreleaser release --snapshot --clean --skip=publish

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR) dist coverage.out

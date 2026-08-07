# Makefile for MarkGo

# Variables
BINARY_NAME=markgo
MAIN_PATH=./cmd/markgo
BUILD_DIR=./build
DOCKER_IMAGE=markgo
DOCKER_TAG=latest

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod
GOFMT=gofmt
GOBIN=$(shell $(GOCMD) env GOBIN)

# golangci-lint is derived from the CI pin in .github/workflows/ci.yml, never hardcoded
# here — a second copy of the version is what lets local lint drift from CI lint. A
# newer binary than the pin reports issues CI does not (goconst floods untouched files).
# The version is baked into the binary path so bumping the CI pin forces a reinstall.
GOLANGCI_VERSION=$(shell sed -n 's/.*golangci-lint-version:[[:space:]]*"\(v[0-9][0-9.]*\)".*/\1/p' .github/workflows/ci.yml)
TOOLS_DIR=.tools
GOLINT=$(TOOLS_DIR)/golangci-lint-$(GOLANGCI_VERSION)

# Build flags
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "v2.0.0")
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-s -w \
	-X 'github.com/1mb-dev/markgo/internal/constants.AppVersion=$(VERSION)' \
	-X 'github.com/1mb-dev/markgo/internal/constants.GitCommit=$(GIT_COMMIT)' \
	-X 'github.com/1mb-dev/markgo/internal/constants.BuildTime=$(BUILD_TIME)'"
BUILD_FLAGS=-trimpath

.PHONY: help build build-release clean test test-race coverage lint fmt run dev docker install tidy

# Default target
help: ## Show this help message
	@echo "MarkGo - Available targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Build targets
build: ## Build unified CLI binary (includes all commands)
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 $(GOBUILD) $(BUILD_FLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "✓ Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

build-release: ## Build for all platforms (Linux, macOS, Windows)
	@echo "Building release binaries..."
	@mkdir -p $(BUILD_DIR)
	@echo "  Linux amd64..."
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(BUILD_FLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PATH)
	@echo "  macOS amd64..."
	@CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) $(BUILD_FLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_PATH)
	@echo "  macOS arm64..."
	@CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GOBUILD) $(BUILD_FLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_PATH)
	@echo "  Windows amd64..."
	@CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) $(BUILD_FLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PATH)
	@echo "✓ Release builds complete"

# Development targets
run: ## Run the server
	@echo "Running $(BINARY_NAME)..."
	$(GOCMD) run $(MAIN_PATH)

dev: ## Run in development mode with live reload (requires: go install github.com/air-verse/air@latest)
	@echo "Starting development server with live reload..."
	ENVIRONMENT=development air -c .air.toml

install: ## Install server binary to GOPATH/bin
	@echo "Installing $(BINARY_NAME)..."
	$(GOCMD) install $(LDFLAGS) $(MAIN_PATH)

# Testing targets
test: ## Run all tests
	@echo "Running tests..."
	$(GOTEST) -v ./...

test-race: ## Run tests with race detector
	@echo "Running tests with race detection..."
	$(GOTEST) -race -v ./...

coverage: ## Generate test coverage report
	@echo "Generating coverage report..."
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "✓ Coverage report: coverage.html"
	@$(GOCMD) tool cover -func=coverage.out | grep total

# Code quality targets
fmt: ## Format code with gofmt
	@echo "Formatting code..."
	$(GOFMT) -s -w .
	$(GOCMD) fmt ./...
	@echo "✓ Code formatted"

lint: $(GOLINT) ## Run linter (golangci-lint, version pinned to CI)
	@echo "Running linter ($(GOLANGCI_VERSION))..."
	@$(GOLINT) run
	@echo "✓ Lint complete"

$(GOLINT):
	@if [ -z "$(GOLANGCI_VERSION)" ]; then \
		echo "Error: no golangci-lint-version found in .github/workflows/ci.yml"; \
		echo "The lint target reads the CI pin from that file — check its format."; \
		exit 1; \
	fi
	@echo "Installing golangci-lint $(GOLANGCI_VERSION) (CI pin)..."
	@mkdir -p $(TOOLS_DIR)
	@GOBIN=$(abspath $(TOOLS_DIR)) $(GOCMD) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)
	@mv $(TOOLS_DIR)/golangci-lint $(GOLINT)

# Dependency management
tidy: ## Tidy and verify dependencies
	@echo "Tidying dependencies..."
	$(GOMOD) tidy
	$(GOMOD) verify
	@echo "✓ Dependencies tidied"

# Docker targets
docker: ## Build and run Docker container
	@echo "Building Docker image..."
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .
	@echo "Running Docker container..."
	docker run -p 3000:3000 $(DOCKER_IMAGE):$(DOCKER_TAG)

# Utility targets
clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html
	@echo "✓ Clean complete"

# Note: For benchmarks, profiling, and other advanced tasks, use go test directly:
#   Benchmarks:  go test -bench=. -benchmem ./...
#   CPU Profile: go test -cpuprofile=cpu.prof ./...
#   Mem Profile: go test -memprofile=mem.prof ./...
#   Profiling:   go tool pprof cpu.prof

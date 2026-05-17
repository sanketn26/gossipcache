.PHONY: help all build build-all run test test-short coverage coverage-report bench benchmark profile-cpu profile-mem profile-block profile-trace profile-all profile-clean lint fmt vet tidy update deps verify clean install install-lint install-tools docker-build docker-run watch version mod-graph list-deps check

# Variables
BINARY_NAME=gossipcache
GO=go
GOFLAGS=-v
LDFLAGS=-ldflags "-s -w"
COVERAGE_FILE=coverage.out
COVERAGE_HTML=coverage.html
CPU_PROFILE=cpu.prof
MEM_PROFILE=mem.prof
BLOCK_PROFILE=block.prof
TRACE_FILE=trace.out

# Directories
EXAMPLE_DIR=./examples/server
PKG_DIR=./pkg
INTERNAL_DIR=./internal
BUILD_DIR=./build
BIN_DIR=./bin
EXAMPLE_TAGS=example

# Colors for output
CYAN=\033[0;36m
GREEN=\033[0;32m
YELLOW=\033[0;33m
RED=\033[0;31m
NC=\033[0m # No Color

## help: Display this help message
help:
	@echo "$(CYAN)Available targets:$(NC)"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/##/  /' | column -t -s ':'

## all: Run fmt, vet, lint, test and build
all: fmt vet lint test build

## build: Build the example binary (library has no default binary)
build:
	@echo "$(CYAN)Building example binary $(BINARY_NAME)...$(NC)"
	@mkdir -p $(BIN_DIR)
	@$(GO) build $(GOFLAGS) -tags $(EXAMPLE_TAGS) $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME) $(EXAMPLE_DIR)
	@echo "$(GREEN)✓ Build complete: $(BIN_DIR)/$(BINARY_NAME)$(NC)"

## build-all: Build the example binary for multiple platforms
build-all:
	@echo "$(CYAN)Building example for multiple platforms...$(NC)"
	@mkdir -p $(BIN_DIR)
	@GOOS=linux GOARCH=amd64 $(GO) build -tags $(EXAMPLE_TAGS) $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME)-linux-amd64 $(EXAMPLE_DIR)
	@GOOS=darwin GOARCH=amd64 $(GO) build -tags $(EXAMPLE_TAGS) $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME)-darwin-amd64 $(EXAMPLE_DIR)
	@GOOS=darwin GOARCH=arm64 $(GO) build -tags $(EXAMPLE_TAGS) $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME)-darwin-arm64 $(EXAMPLE_DIR)
	@GOOS=windows GOARCH=amd64 $(GO) build -tags $(EXAMPLE_TAGS) $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME)-windows-amd64.exe $(EXAMPLE_DIR)
	@echo "$(GREEN)✓ Cross-platform build complete$(NC)"

## run: Run the example binary
run:
	@echo "$(CYAN)Running example $(BINARY_NAME)...$(NC)"
	@$(GO) run -tags $(EXAMPLE_TAGS) $(EXAMPLE_DIR)

## test: Run all tests
test:
	@echo "$(CYAN)Running tests...$(NC)"
	@$(GO) test -v -race ./...
	@echo "$(GREEN)✓ Tests passed$(NC)"

## test-short: Run short tests only
test-short:
	@echo "$(CYAN)Running short tests...$(NC)"
	@$(GO) test -v -short ./...

## coverage: Run tests with coverage
coverage:
	@echo "$(CYAN)Running tests with coverage...$(NC)"
	@$(GO) test -v -race -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./...
	@$(GO) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@echo "$(GREEN)✓ Coverage report generated: $(COVERAGE_HTML)$(NC)"

## coverage-report: Display coverage summary
coverage-report: coverage
	@$(GO) tool cover -func=$(COVERAGE_FILE)

## bench: Run benchmarks
bench:
	@echo "$(CYAN)Running benchmarks...$(NC)"
	@$(GO) test -bench=. -benchmem ./...

## benchmark: Run benchmark tests
benchmark: bench

## profile-cpu: Run CPU profiling
profile-cpu:
	@echo "$(CYAN)Running CPU profiling...$(NC)"
	@$(GO) test -cpuprofile=$(CPU_PROFILE) -bench=. ./...
	@echo "$(GREEN)✓ CPU profile generated: $(CPU_PROFILE)$(NC)"
	@echo "$(YELLOW)View with: go tool pprof -http=:8080 $(CPU_PROFILE)$(NC)"

## profile-mem: Run memory profiling
profile-mem:
	@echo "$(CYAN)Running memory profiling...$(NC)"
	@$(GO) test -memprofile=$(MEM_PROFILE) -bench=. ./...
	@echo "$(GREEN)✓ Memory profile generated: $(MEM_PROFILE)$(NC)"
	@echo "$(YELLOW)View with: go tool pprof -http=:8080 $(MEM_PROFILE)$(NC)"

## profile-block: Run blocking profiling
profile-block:
	@echo "$(CYAN)Running blocking profiling...$(NC)"
	@$(GO) test -blockprofile=$(BLOCK_PROFILE) -bench=. ./...
	@echo "$(GREEN)✓ Block profile generated: $(BLOCK_PROFILE)$(NC)"
	@echo "$(YELLOW)View with: go tool pprof -http=:8080 $(BLOCK_PROFILE)$(NC)"

## profile-trace: Run execution trace
profile-trace:
	@echo "$(CYAN)Running execution trace...$(NC)"
	@$(GO) test -trace=$(TRACE_FILE) -bench=. ./...
	@echo "$(GREEN)✓ Trace file generated: $(TRACE_FILE)$(NC)"
	@echo "$(YELLOW)View with: go tool trace $(TRACE_FILE)$(NC)"

## profile-all: Run all profiling (CPU, memory, blocking, trace)
profile-all: profile-cpu profile-mem profile-block profile-trace
	@echo "$(GREEN)✓ All profiling complete$(NC)"

## profile-clean: Clean profiling files
profile-clean:
	@echo "$(CYAN)Cleaning profiling files...$(NC)"
	@rm -f $(CPU_PROFILE) $(MEM_PROFILE) $(BLOCK_PROFILE) $(TRACE_FILE)
	@rm -f *.test
	@echo "$(GREEN)✓ Profiling files cleaned$(NC)"

## lint: Run linters (requires golangci-lint)
lint:
	@echo "$(CYAN)Running linters...$(NC)"
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run --timeout=5m ./...; \
		echo "$(GREEN)✓ Linting complete$(NC)"; \
	else \
		echo "$(YELLOW)⚠ golangci-lint not found. Install it with: make install-lint$(NC)"; \
	fi

## fmt: Format code with gofmt
fmt:
	@echo "$(CYAN)Formatting code...$(NC)"
	@$(GO) fmt ./...
	@echo "$(GREEN)✓ Code formatted$(NC)"

## vet: Run go vet
vet:
	@echo "$(CYAN)Running go vet...$(NC)"
	@$(GO) vet ./...
	@echo "$(GREEN)✓ Vet complete$(NC)"

## tidy: Run go mod tidy
tidy:
	@echo "$(CYAN)Tidying dependencies...$(NC)"
	@$(GO) mod tidy
	@echo "$(GREEN)✓ Dependencies tidied$(NC)"

## update: Update all dependencies
update:
	@echo "$(CYAN)Updating dependencies...$(NC)"
	@$(GO) get -u ./...
	@$(GO) mod tidy
	@echo "$(GREEN)✓ Dependencies updated$(NC)"

## deps: Download dependencies
deps:
	@echo "$(CYAN)Downloading dependencies...$(NC)"
	@$(GO) mod download
	@echo "$(GREEN)✓ Dependencies downloaded$(NC)"

## verify: Verify dependencies
verify:
	@echo "$(CYAN)Verifying dependencies...$(NC)"
	@$(GO) mod verify
	@echo "$(GREEN)✓ Dependencies verified$(NC)"

## clean: Clean build artifacts
clean:
	@echo "$(CYAN)Cleaning build artifacts...$(NC)"
	@rm -rf $(BIN_DIR) $(BUILD_DIR)
	@rm -f $(COVERAGE_FILE) $(COVERAGE_HTML)
	@rm -f $(CPU_PROFILE) $(MEM_PROFILE) $(BLOCK_PROFILE) $(TRACE_FILE)
	@rm -f *.test
	@$(GO) clean
	@echo "$(GREEN)✓ Clean complete$(NC)"

## install: Install the example binary to $GOPATH/bin
install:
	@echo "$(CYAN)Installing example binary $(BINARY_NAME)...$(NC)"
	@$(GO) install -tags $(EXAMPLE_TAGS) $(LDFLAGS) $(EXAMPLE_DIR)
	@echo "$(GREEN)✓ Installed to $(GOPATH)/bin/$(BINARY_NAME)$(NC)"

## install-lint: Install golangci-lint
install-lint:
	@echo "$(CYAN)Installing golangci-lint...$(NC)"
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell go env GOPATH)/bin latest
	@echo "$(GREEN)✓ golangci-lint installed$(NC)"

## install-tools: Install development tools
install-tools: install-lint
	@echo "$(CYAN)Installing development tools...$(NC)"
	@go install golang.org/x/tools/cmd/goimports@latest
	@go install github.com/swaggo/swag/cmd/swag@latest
	@echo "$(GREEN)✓ Development tools installed$(NC)"

## docker-build: Build Docker image
docker-build:
	@echo "$(CYAN)Building Docker image...$(NC)"
	@docker build -t $(BINARY_NAME):latest .
	@echo "$(GREEN)✓ Docker image built$(NC)"

## docker-run: Run Docker container
docker-run:
	@echo "$(CYAN)Running Docker container...$(NC)"
	@docker run --rm -it $(BINARY_NAME):latest

## watch: Watch for changes and rebuild (requires entr)
watch:
	@if command -v entr >/dev/null 2>&1; then \
		find . -name '*.go' | entr -r make run; \
	else \
		echo "$(YELLOW)⚠ entr not found. Install it with: brew install entr (macOS)$(NC)"; \
	fi

## version: Display Go version
version:
	@$(GO) version

## mod-graph: Display module dependency graph
mod-graph:
	@$(GO) mod graph

## list-deps: List all dependencies
list-deps:
	@$(GO) list -m all

## check: Run all checks (fmt, vet, lint, test)
check: fmt vet lint test
	@echo "$(GREEN)✓ All checks passed$(NC)"

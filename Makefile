# Makefile for Lux EVM (Subnet EVM)

# Variables
REGISTRY_TMPDIR := $(shell pwd)/build/tmp
# Use local tmp directory for Go builds to avoid macOS permission issues
override TMPDIR := $(REGISTRY_TMPDIR)
REGISTRY_GOCACHE := $(shell pwd)/build/go-cache
# Use local Go build cache to avoid permission issues on macOS
override GOCACHE := $(REGISTRY_GOCACHE)
export TMPDIR GOCACHE
BINARY_NAME := evm
PLUGIN_PATH := ~/.luxd/plugins/$(BINARY_NAME)
VERSION := $(shell git describe --tags --always --dirty="-dev" 2>/dev/null || echo "unknown")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date +%FT%T%z)

# Go build flags
GOARCH := $(shell go env GOARCH)
GOOS := $(shell go env GOOS)
CGO_CFLAGS := -O -D__BLST_PORTABLE__
LDFLAGS := -X github.com/luxfi/evm/plugin/evm.Version=$(VERSION)
# Build tags - only include necessary tags
EVM_TAGS_DEFAULT := sqlite
EVM_TAGS_PEBBLE := sqlite,pebbledb
EVM_TAGS_ROCKSDB := sqlite,rocksdb

# Default target
.PHONY: all
all: evm

# Build the EVM binary with BadgerDB (default)
.PHONY: evm
evm:
	@echo "Building Lux EVM with BadgerDB..."
	@mkdir -p build
	@mkdir -p $(TMPDIR) $(GOCACHE)
	TMPDIR=$(TMPDIR) GOCACHE=$(GOCACHE) CGO_CFLAGS="$(CGO_CFLAGS)" go build -tags="$(EVM_TAGS_DEFAULT)" -ldflags "$(LDFLAGS)" -o build/$(BINARY_NAME) ./plugin

# Build the EVM binary with PebbleDB
.PHONY: evm-pebble
evm-pebble:
	@echo "Building Lux EVM with PebbleDB..."
	@mkdir -p build
	@mkdir -p $(TMPDIR) $(GOCACHE)
	TMPDIR=$(TMPDIR) GOCACHE=$(GOCACHE) CGO_CFLAGS="$(CGO_CFLAGS)" go build -tags="$(EVM_TAGS_PEBBLE)" -ldflags "$(LDFLAGS)" -o build/$(BINARY_NAME) ./plugin

# Build the EVM binary with RocksDB only
.PHONY: evm-rocksdb
evm-rocksdb:
	@echo "Building Lux EVM with RocksDB..."
	@mkdir -p build
	@mkdir -p $(TMPDIR) $(GOCACHE)
	TMPDIR=$(TMPDIR) GOCACHE=$(GOCACHE) CGO_CFLAGS="$(CGO_CFLAGS)" go build -tags="$(EVM_TAGS_ROCKSDB)" -ldflags "$(LDFLAGS)" -o build/$(BINARY_NAME) ./plugin

# Build the EVM binary (alias for backward compatibility)
.PHONY: build
build: evm

# Build and install as plugin
.PHONY: install
install: build
	@echo "Installing Lux EVM plugin to $(PLUGIN_PATH)..."
	@mkdir -p $(dir $(PLUGIN_PATH))
	@cp build/$(BINARY_NAME) $(PLUGIN_PATH)
	@echo "Successfully installed to $(PLUGIN_PATH)"

# Run tests
.PHONY: test
test:
	@echo "Running tests..."
	@mkdir -p $(TMPDIR) $(GOCACHE)
	TMPDIR=$(TMPDIR) GOCACHE=$(GOCACHE) go test -v ./...

# Run tests with filter
.PHONY: test-filter
test-filter:
	@echo "Running tests with filter: $(filter)"
	@mkdir -p $(TMPDIR) $(GOCACHE)
	TMPDIR=$(TMPDIR) GOCACHE=$(GOCACHE) go test -v -run="$(filter)" ./...

# Run tests with ginkgo
.PHONY: test-ginkgo
test-ginkgo:
	@echo "Running tests with ginkgo..."
	@mkdir -p $(TMPDIR) $(GOCACHE)
	@if command -v ginkgo >/dev/null 2>&1; then \
		TMPDIR=$(TMPDIR) GOCACHE=$(GOCACHE) ginkgo -v -r --race --fail-fast --randomize-all --randomize-suites; \
	else \
		echo "ginkgo not installed. Install with: go install github.com/onsi/ginkgo/ginkgo@latest"; \
		exit 1; \
	fi

# Run tests with ginkgo and filter
.PHONY: test-ginkgo-filter
test-ginkgo-filter:
	@echo "Running tests with ginkgo filter: $(filter)"
	@mkdir -p $(TMPDIR) $(GOCACHE)
	@if command -v ginkgo >/dev/null 2>&1; then \
		TMPDIR=$(TMPDIR) GOCACHE=$(GOCACHE) ginkgo -v -r --race --fail-fast --randomize-all --randomize-suites --focus="$(filter)"; \
	else \
		echo "ginkgo not installed. Install with: go install github.com/onsi/ginkgo/ginkgo@latest"; \
		exit 1; \
	fi

# Run tests with coverage
.PHONY: test-coverage
test-coverage:
	@echo "Running tests with coverage..."
	@mkdir -p $(TMPDIR) $(GOCACHE)
	TMPDIR=$(TMPDIR) GOCACHE=$(GOCACHE) go test -v -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run tests with coverage using ginkgo
.PHONY: test-coverage-ginkgo
test-coverage-ginkgo:
	@echo "Running tests with coverage using ginkgo..."
	@mkdir -p $(TMPDIR) $(GOCACHE)
	@if command -v ginkgo >/dev/null 2>&1; then \
		TMPDIR=$(TMPDIR) GOCACHE=$(GOCACHE) ginkgo -v -r --race --fail-fast --randomize-all --randomize-suites --cover --coverprofile=coverage.out --covermode=atomic; \
		go tool cover -html=coverage.out -o coverage.html; \
		echo "Coverage report generated: coverage.html"; \
	else \
		echo "ginkgo not installed. Install with: go install github.com/onsi/ginkgo/ginkgo@latest"; \
		exit 1; \
	fi

# Run benchmarks
.PHONY: bench
bench:
	@echo "Running benchmarks..."
	go test -bench=. -benchmem ./...

# Format code
.PHONY: fmt
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Run linter
.PHONY: lint
lint:
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		exit 1; \
	fi

# Generate mocks
.PHONY: mocks
mocks:
	@echo "Generating ethapi mocks..."
	@go generate ./internal/ethapi
	@echo "Generating precompileconfig mocks..."
	@go generate ./precompile/precompileconfig
	@echo "Generating contract mocks..."
	@go generate ./precompile/contract

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf build/
	@rm -f coverage.out coverage.html
	@echo "Clean complete"

# Run security checks
.PHONY: security
security:
	@echo "Running security checks..."
	@if command -v gosec >/dev/null 2>&1; then \
		gosec ./...; \
	else \
		echo "gosec not installed. Install with: go install github.com/securego/gosec/cmd/gosec@latest"; \
		exit 1; \
	fi

# Run static analysis
.PHONY: staticcheck
staticcheck:
	@echo "Running static analysis..."
	@if command -v staticcheck >/dev/null 2>&1; then \
		staticcheck ./...; \
	else \
		echo "staticcheck not installed. Install with: go install honnef.co/go/tools/cmd/staticcheck@latest"; \
		exit 1; \
	fi

# Update dependencies
.PHONY: deps
deps:
	@echo "Updating dependencies..."
	go mod download
	go mod tidy

# Verify dependencies
.PHONY: verify
verify:
	@echo "Verifying dependencies..."
	go mod verify

# Run all checks (fmt, lint, test)
.PHONY: check
check: fmt lint test

# Display version information
.PHONY: version
version:
	@echo "Lux EVM"
	@echo "Version: $(VERSION)"
	@echo "Commit: $(COMMIT)"
	@echo "Build Date: $(BUILD_DATE)"
	@echo "Go Version: $(shell go version)"
	@echo "OS/Arch: $(GOOS)/$(GOARCH)"

# Help message
.PHONY: help
help:
	@echo "Lux EVM Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  all          - Build the EVM binary with BadgerDB (default)"
	@echo "  evm          - Build the EVM binary with BadgerDB"
	@echo "  evm-pebble   - Build the EVM binary with PebbleDB"
	@echo "  evm-rocksdb  - Build the EVM binary with RocksDB"
	@echo "  build        - Build the EVM binary (alias for evm)"
	@echo "  install      - Build and install as Lux plugin"
	@echo "  test         - Run tests"
	@echo "  test-filter  - Run tests with filter (usage: make test-filter filter=TestName)"
	@echo "  test-ginkgo  - Run tests with ginkgo"
	@echo "  test-ginkgo-filter - Run tests with ginkgo filter (usage: make test-ginkgo-filter filter=TestName)"
	@echo "  test-coverage - Run tests with coverage report"
	@echo "  test-coverage-ginkgo - Run tests with coverage using ginkgo"
	@echo "  bench        - Run benchmarks"
	@echo "  fmt          - Format code"
	@echo "  lint         - Run linter"
	@echo "  mocks        - Generate mocks"
	@echo "  clean        - Clean build artifacts"
	@echo "  security     - Run security checks"
	@echo "  staticcheck  - Run static analysis"
	@echo "  deps         - Update dependencies"
	@echo "  verify       - Verify dependencies"
	@echo "  check        - Run fmt, lint, and test"
	@echo "  version      - Display version information"
	@echo "  help         - Display this help message"
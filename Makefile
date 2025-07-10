# NoiseFS Makefile

# Configuration
SHELL := /bin/bash
PROJECT_NAME := noisefs
BUILD_DIR := bin
DIST_DIR := dist

# Go configuration
GO := go
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)
CGO_ENABLED := 0

# Version information
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Linker flags
LDFLAGS := -s -w
LDFLAGS += -X 'main.Version=$(VERSION)'
LDFLAGS += -X 'main.GitCommit=$(GIT_COMMIT)'
LDFLAGS += -X 'main.BuildDate=$(BUILD_DATE)'

# Build tags
BUILD_TAGS ?=

# Binaries to build
BINARIES := noisefs noisefs-mount noisefs-config noisefs-security webui legal-review simulation demo

# Sub-tools under noisefs-tools (built separately)
TOOLS := noisefs-bootstrap inspect-index benchmark docker-benchmark enterprise-benchmark impact-demo

# Docker configuration
DOCKER_IMAGE := $(PROJECT_NAME)
DOCKER_TAG := latest

# Colors for output
RED := \033[0;31m
GREEN := \033[0;32m
YELLOW := \033[1;33m
BLUE := \033[0;34m
NC := \033[0m # No Color

.PHONY: help build build-all tools clean test bench lint fmt vet deps docker docker-build docker-push install dist dev check all demo demo-reuse impact-demo benchmark simulation

# Default target
all: clean build test

# Show help
help:
	@echo -e "$(BLUE)NoiseFS Build System$(NC)"
	@echo ""
	@echo -e "$(YELLOW)Available targets:$(NC)"
	@echo -e "  $(GREEN)build$(NC)         Build all binaries"
	@echo -e "  $(GREEN)tools$(NC)         Build all sub-tools"
	@echo -e "  $(GREEN)build-all$(NC)     Build binaries and tools"
	@echo -e "  $(GREEN)clean$(NC)         Clean build artifacts"
	@echo ""
	@echo -e "$(YELLOW)Testing:$(NC)"
	@echo -e "  $(GREEN)test$(NC)          Run all tests"
	@echo -e "  $(GREEN)test-unit$(NC)     Run unit tests only"
	@echo -e "  $(GREEN)test-integration$(NC) Run integration tests (with mocks)"
	@echo -e "  $(GREEN)test-real$(NC)     Run real end-to-end tests with IPFS"
	@echo -e "  $(GREEN)test-evolution$(NC) Run comprehensive evolution analysis"
	@echo -e "  $(GREEN)test-evolution-impact$(NC) Show evolution impact summary"
	@echo -e "  $(GREEN)quick-test$(NC)    Run quick unit tests"
	@echo -e "  $(GREEN)real-quick$(NC)    Run quick real IPFS test"
	@echo -e "  $(GREEN)perf-test$(NC)     Run performance tests with real IPFS"
	@echo ""
	@echo -e "$(YELLOW)Real IPFS Testing:$(NC)"
	@echo -e "  $(GREEN)start-ipfs$(NC)    Start real IPFS test network"
	@echo -e "  $(GREEN)stop-ipfs$(NC)     Stop IPFS test network"
	@echo -e "  $(GREEN)ipfs-status$(NC)   Show IPFS network status"
	@echo ""
	@echo -e "$(YELLOW)Demos & Simulations:$(NC)"
	@echo -e "  $(GREEN)demo$(NC)          Run NoiseFS core functionality demo"
	@echo -e "  $(GREEN)demo-reuse$(NC)    Run NoiseFS block reuse demo"
	@echo -e "  $(GREEN)impact-demo$(NC)   Run NoiseFS impact analysis demo"
	@echo -e "  $(GREEN)evolution-demo$(NC) Show comprehensive evolution impact"
	@echo -e "  $(GREEN)evolution-demo-detailed$(NC) Show detailed optimization breakdown"
	@echo -e "  $(GREEN)benchmark$(NC)     Run NoiseFS benchmarks"
	@echo -e "  $(GREEN)simulation$(NC)    Run medium-scale simulation"
	@echo -e "  $(GREEN)simulation-large$(NC) Run large-scale simulation"
	@echo ""
	@echo -e "$(YELLOW)Development:$(NC)"
	@echo -e "  $(GREEN)bench$(NC)         Run benchmarks"
	@echo -e "  $(GREEN)lint$(NC)          Run linters"
	@echo -e "  $(GREEN)fmt$(NC)           Format code"
	@echo -e "  $(GREEN)vet$(NC)           Run go vet"
	@echo -e "  $(GREEN)deps$(NC)          Download dependencies"
	@echo -e "  $(GREEN)dev$(NC)           Development build with race detection"
	@echo -e "  $(GREEN)check$(NC)         Run all checks (test, lint, vet)"
	@echo -e "  $(GREEN)all$(NC)           Clean, build, and test"
	@echo ""
	@echo -e "$(YELLOW)Docker & Deployment:$(NC)"
	@echo -e "  $(GREEN)docker$(NC)        Build Docker image"
	@echo -e "  $(GREEN)docker-push$(NC)   Push Docker image"
	@echo -e "  $(GREEN)install$(NC)       Install binaries to system"
	@echo -e "  $(GREEN)dist$(NC)          Create distribution packages"
	@echo ""
	@echo -e "$(YELLOW)Variables:$(NC)"
	@echo "  VERSION=$(VERSION)"
	@echo "  GOOS=$(GOOS)"
	@echo "  GOARCH=$(GOARCH)"
	@echo "  BUILD_TAGS=$(BUILD_TAGS)"

# Build all binaries
build: $(BUILD_DIR) $(addprefix $(BUILD_DIR)/,$(BINARIES))
	@echo -e "$(GREEN)✓ Build completed$(NC)"

# Build all tools (binaries + sub-tools)
build-all: build tools
	@echo -e "$(GREEN)✓ All binaries and tools built$(NC)"

# Build sub-tools
tools: $(BUILD_DIR) $(addprefix $(BUILD_DIR)/,$(TOOLS))
	@echo -e "$(GREEN)✓ Tools completed$(NC)"

# Build individual binaries
$(BUILD_DIR)/%: cmd/%
	@echo -e "$(BLUE)Building $*...$(NC)"
	@CGO_ENABLED=$(CGO_ENABLED) $(GO) build \
		$(if $(BUILD_TAGS),-tags $(BUILD_TAGS)) \
		-ldflags "$(LDFLAGS)" \
		-o $@ \
		./cmd/$*

# Build sub-tools under noisefs-tools
$(BUILD_DIR)/noisefs-bootstrap:
	@echo -e "$(BLUE)Building noisefs-bootstrap...$(NC)"
	@CGO_ENABLED=$(CGO_ENABLED) $(GO) build \
		$(if $(BUILD_TAGS),-tags $(BUILD_TAGS)) \
		-ldflags "$(LDFLAGS)" \
		-o $@ \
		./cmd/noisefs-tools/bootstrap/noisefs-bootstrap

$(BUILD_DIR)/inspect-index:
	@echo -e "$(BLUE)Building inspect-index...$(NC)"
	@CGO_ENABLED=$(CGO_ENABLED) $(GO) build \
		$(if $(BUILD_TAGS),-tags $(BUILD_TAGS)) \
		-ldflags "$(LDFLAGS)" \
		-o $@ \
		./cmd/noisefs-tools/inspect/inspect-index

$(BUILD_DIR)/benchmark:
	@echo -e "$(BLUE)Building benchmark...$(NC)"
	@CGO_ENABLED=$(CGO_ENABLED) $(GO) build \
		$(if $(BUILD_TAGS),-tags $(BUILD_TAGS)) \
		-ldflags "$(LDFLAGS)" \
		-o $@ \
		./cmd/noisefs-tools/benchmark/benchmark

$(BUILD_DIR)/docker-benchmark:
	@echo -e "$(BLUE)Building docker-benchmark...$(NC)"
	@CGO_ENABLED=$(CGO_ENABLED) $(GO) build \
		$(if $(BUILD_TAGS),-tags $(BUILD_TAGS)) \
		-ldflags "$(LDFLAGS)" \
		-o $@ \
		./cmd/noisefs-tools/benchmark/docker-benchmark

$(BUILD_DIR)/enterprise-benchmark:
	@echo -e "$(BLUE)Building enterprise-benchmark...$(NC)"
	@CGO_ENABLED=$(CGO_ENABLED) $(GO) build \
		$(if $(BUILD_TAGS),-tags $(BUILD_TAGS)) \
		-ldflags "$(LDFLAGS)" \
		-o $@ \
		./cmd/noisefs-tools/benchmark/enterprise-benchmark

$(BUILD_DIR)/impact-demo:
	@echo -e "$(BLUE)Building impact-demo...$(NC)"
	@CGO_ENABLED=$(CGO_ENABLED) $(GO) build \
		$(if $(BUILD_TAGS),-tags $(BUILD_TAGS)) \
		-ldflags "$(LDFLAGS)" \
		-o $@ \
		./cmd/noisefs-tools/benchmark/impact-demo

# Create build directory
$(BUILD_DIR):
	@mkdir -p $(BUILD_DIR)

# Development build with race detection
dev: CGO_ENABLED := 1
dev: LDFLAGS += -race
dev: build
	@echo -e "$(GREEN)✓ Development build completed$(NC)"

# Clean build artifacts
clean:
	@echo -e "$(BLUE)Cleaning build artifacts...$(NC)"
	@rm -rf $(BUILD_DIR) $(DIST_DIR)
	@$(GO) clean -cache -testcache
	@echo -e "$(GREEN)✓ Clean completed$(NC)"

# Download dependencies
deps:
	@echo -e "$(BLUE)Downloading dependencies...$(NC)"
	@$(GO) mod download
	@$(GO) mod tidy
	@echo -e "$(GREEN)✓ Dependencies updated$(NC)"

# Format code
fmt:
	@echo -e "$(BLUE)Formatting code...$(NC)"
	@$(GO) fmt ./...
	@echo -e "$(GREEN)✓ Code formatted$(NC)"

# Run go vet
vet:
	@echo -e "$(BLUE)Running go vet...$(NC)"
	@$(GO) vet $(if $(BUILD_TAGS),-tags $(BUILD_TAGS)) ./...
	@echo -e "$(GREEN)✓ Vet completed$(NC)"

# Run linters
lint:
	@echo -e "$(BLUE)Running linters...$(NC)"
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run $(if $(BUILD_TAGS),--build-tags $(BUILD_TAGS)); \
		echo "$(GREEN)✓ Linting completed$(NC)"; \
	else \
		echo "$(YELLOW)⚠ golangci-lint not found, running basic checks$(NC)"; \
		$(MAKE) fmt vet; \
	fi

# Run tests
test:
	@echo -e "$(BLUE)Running tests...$(NC)"
	@$(GO) test $(if $(BUILD_TAGS),-tags $(BUILD_TAGS)) -v ./...
	@echo -e "$(GREEN)✓ Tests completed$(NC)"

# Run unit tests only
test-unit:
	@echo -e "$(BLUE)Running unit tests...$(NC)"
	@$(GO) test -short $(if $(BUILD_TAGS),-tags $(BUILD_TAGS)) -v ./pkg/core/blocks ./pkg/storage/cache ./pkg/core/client ./pkg/storage/ipfs
	@echo -e "$(GREEN)✓ Unit tests completed$(NC)"

# Run integration tests (with mocks)
test-integration:
	@echo -e "$(BLUE)Running integration tests...$(NC)"
	@$(GO) test $(if $(BUILD_TAGS),-tags $(BUILD_TAGS)) -v ./tests/integration/ -run "TestEvolution"
	@echo -e "$(GREEN)✓ Integration tests completed$(NC)"

# Run real end-to-end tests with IPFS
test-real: docker-check start-ipfs
	@echo -e "$(BLUE)Running real end-to-end tests...$(NC)"
	@echo -e "$(YELLOW)Waiting for IPFS network to stabilize...$(NC)"
	@sleep 60
	@$(GO) test $(if $(BUILD_TAGS),-tags $(BUILD_TAGS)) -v ./tests/system/ -timeout=10m || ($(MAKE) stop-ipfs && exit 1)
	@$(MAKE) stop-ipfs
	@echo -e "$(GREEN)✓ Real end-to-end tests completed$(NC)"

# Legacy milestone4 testing has been replaced by comprehensive evolution analysis
# Use: make test-evolution or make evolution-demo instead

# Run tests with coverage
test-coverage:
	@echo -e "$(BLUE)Running tests with coverage...$(NC)"
	@$(GO) test $(if $(BUILD_TAGS),-tags $(BUILD_TAGS)) -v -coverprofile=coverage.out ./...
	@$(GO) tool cover -html=coverage.out -o coverage.html
	@echo -e "$(GREEN)✓ Coverage report generated: coverage.html$(NC)"

# Run benchmarks
bench:
	@echo -e "$(BLUE)Running benchmarks...$(NC)"
	@$(GO) test $(if $(BUILD_TAGS),-tags $(BUILD_TAGS)) -bench=. -benchmem ./...
	@echo -e "$(GREEN)✓ Benchmarks completed$(NC)"

# Run all checks
check: deps fmt vet lint test
	@echo -e "$(GREEN)✓ All checks passed$(NC)"

# Docker and IPFS management
docker-check:
	@command -v docker >/dev/null 2>&1 || { echo "$(RED)Docker is required but not installed$(NC)"; exit 1; }
	@command -v docker-compose >/dev/null 2>&1 || { echo "$(RED)docker-compose is required but not installed$(NC)"; exit 1; }
	@docker info >/dev/null 2>&1 || { echo "$(RED)Docker daemon is not running$(NC)"; exit 1; }

# Start real IPFS test network
start-ipfs: docker-check
	@echo -e "$(BLUE)Starting real IPFS test network...$(NC)"
	@docker-compose -f docker-compose.test.yml up -d
	@echo -e "$(YELLOW)Waiting for IPFS nodes to initialize...$(NC)"
	@sleep 20
	@echo -e "$(GREEN)✓ IPFS network started$(NC)"
	@echo "Nodes available at:"
	@echo "  Node 1: http://localhost:5001"
	@echo "  Node 2: http://localhost:5002"
	@echo "  Node 3: http://localhost:5003"
	@echo "  Node 4: http://localhost:5004"
	@echo "  Node 5: http://localhost:5005"

# Stop IPFS test network
stop-ipfs:
	@echo -e "$(BLUE)Stopping IPFS test network...$(NC)"
	@docker-compose -f docker-compose.test.yml down -v >/dev/null 2>&1 || true
	@echo -e "$(GREEN)✓ IPFS network stopped and volumes cleaned$(NC)"

# Show IPFS network status
ipfs-status:
	@echo -e "$(BLUE)IPFS Network Status:$(NC)"
	@echo "==================="
	@for port in 5001 5002 5003 5004 5005; do \
		echo -n "Node $$port: "; \
		curl -s http://localhost:$$port/api/v0/version 2>/dev/null | grep -o '"Version":"[^"]*"' | cut -d'"' -f4 || echo "$(RED)Not responding$(NC)"; \
	done

# Demo and simulation targets
demo: bin/demo
	@echo -e "$(BLUE)Running NoiseFS core functionality demo...$(NC)"
	@./bin/demo

benchmark:
	@echo -e "$(BLUE)Running NoiseFS benchmarks...$(NC)"
	@$(GO) run cmd/noisefs-tools/benchmark/benchmark/main.go

# Additional demo targets
demo-reuse: bin/demo
	@echo -e "$(BLUE)Running NoiseFS block reuse demo...$(NC)"
	@./bin/demo -reuse

impact-demo:
	@echo -e "$(BLUE)Running NoiseFS impact demo...$(NC)"
	@$(GO) run cmd/noisefs-tools/benchmark/impact-demo/main.go

evolution-demo:
	@echo -e "$(BLUE)🎯 Running comprehensive NoiseFS evolution demo...$(NC)"
	@$(GO) run cmd/evolution-demo/main.go

evolution-demo-detailed:
	@echo -e "$(BLUE)🎯 Running detailed NoiseFS evolution analysis...$(NC)"
	@$(GO) run cmd/evolution-demo/main.go --detailed

simulation:
	@echo -e "$(BLUE)Running medium-scale simulation...$(NC)"
	@$(GO) run cmd/simulation/main.go -scenario=medium -duration=60s

simulation-large:
	@echo -e "$(BLUE)Running large-scale simulation...$(NC)"
	@$(GO) run cmd/simulation/main.go -scenario=large -duration=120s

# Quick testing shortcuts
quick-test:
	@echo -e "$(BLUE)Running quick unit tests...$(NC)"
	@$(GO) test ./pkg/core/client/ ./pkg/core/blocks/ ./pkg/storage/cache/ -v

real-quick: start-ipfs
	@echo -e "$(BLUE)Running quick real test...$(NC)"
	@sleep 25
	@$(GO) test ./tests/system/ -run TestRealSingleNode -v -timeout=5m || ($(MAKE) stop-ipfs && exit 1)
	@$(MAKE) stop-ipfs

# Performance testing
perf-test: start-ipfs
	@echo -e "$(BLUE)Running performance tests...$(NC)"
	@sleep 30
	@$(GO) test ./tests/benchmarks/ -bench=. -benchtime=30s -timeout=15m || ($(MAKE) stop-ipfs && exit 1)
	@$(MAKE) stop-ipfs

# Build Docker image
docker: docker-build

docker-build:
	@echo -e "$(BLUE)Building Docker image...$(NC)"
	@cd deployments && docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) -f Dockerfile ..
	@echo -e "$(GREEN)✓ Docker image built: $(DOCKER_IMAGE):$(DOCKER_TAG)$(NC)"

# Push Docker image
docker-push:
	@echo -e "$(BLUE)Pushing Docker image...$(NC)"
	@docker push $(DOCKER_IMAGE):$(DOCKER_TAG)
	@echo -e "$(GREEN)✓ Docker image pushed$(NC)"

# Install binaries to system
install: build
	@echo -e "$(BLUE)Installing binaries...$(NC)"
	@for binary in $(BINARIES); do \
		echo "Installing $$binary to /usr/local/bin/"; \
		sudo cp $(BUILD_DIR)/$$binary /usr/local/bin/; \
	done
	@echo -e "$(GREEN)✓ Installation completed$(NC)"

# Create distribution packages
dist: $(DIST_DIR)
	@echo -e "$(BLUE)Creating distribution packages...$(NC)"
	@./scripts/build.sh dist
	@echo -e "$(GREEN)✓ Distribution packages created in $(DIST_DIR)$(NC)"

# Create dist directory
$(DIST_DIR):
	@mkdir -p $(DIST_DIR)

# Cross-compilation targets
build-linux: GOOS := linux
build-linux: build

build-darwin: GOOS := darwin
build-darwin: build

build-windows: GOOS := windows
build-windows: build

build-all-platforms:
	@$(MAKE) build-linux GOOS=linux GOARCH=amd64
	@$(MAKE) build-linux GOOS=linux GOARCH=arm64
	@$(MAKE) build-darwin GOOS=darwin GOARCH=amd64
	@$(MAKE) build-darwin GOOS=darwin GOARCH=arm64
	@$(MAKE) build-windows GOOS=windows GOARCH=amd64

# FUSE build
build-fuse: BUILD_TAGS := fuse
build-fuse: build
	@echo -e "$(GREEN)✓ FUSE build completed$(NC)"

# Development workflow
dev-setup: deps
	@echo -e "$(BLUE)Setting up development environment...$(NC)"
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "$(YELLOW)Installing golangci-lint...$(NC)"; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell go env GOPATH)/bin; \
	fi
	@echo -e "$(GREEN)✓ Development environment ready$(NC)"

# Watch for changes and rebuild (requires entr)
watch:
	@if command -v entr >/dev/null 2>&1; then \
		echo "$(BLUE)Watching for changes... (Ctrl+C to stop)$(NC)"; \
		find . -name "*.go" | entr -c $(MAKE) build; \
	else \
		echo "$(RED)Error: entr is required for watch mode$(NC)"; \
		echo "Install with: brew install entr (macOS) or apt-get install entr (Ubuntu)"; \
		exit 1; \
	fi

# Start development server
dev-server: build
	@echo -e "$(BLUE)Starting development server...$(NC)"
	@./$(BUILD_DIR)/noisefs daemon --config configs/config.example.json --log-level debug

# Quick deployment
deploy: docker
	@echo -e "$(BLUE)Starting deployment...$(NC)"
	@cd deployments && docker-compose up -d
	@echo -e "$(GREEN)✓ Deployment started$(NC)"
	@echo "Web UI: http://localhost:8080"

# Stop deployment
stop:
	@echo -e "$(BLUE)Stopping deployment...$(NC)"
	@cd deployments && docker-compose down
	@echo -e "$(GREEN)✓ Deployment stopped$(NC)"

# List all available binaries and tools
list-targets:
	@echo -e "$(BLUE)Available Binaries:$(NC)"
	@for binary in $(BINARIES); do \
		echo "  $(GREEN)$$binary$(NC) -> cmd/$$binary/"; \
	done
	@echo ""
	@echo -e "$(BLUE)Available Tools:$(NC)"
	@echo "  $(GREEN)noisefs-bootstrap$(NC) -> cmd/noisefs-tools/bootstrap/noisefs-bootstrap/"
	@echo "  $(GREEN)inspect-index$(NC) -> cmd/noisefs-tools/inspect/inspect-index/"
	@echo "  $(GREEN)benchmark$(NC) -> cmd/noisefs-tools/benchmark/benchmark/"
	@echo "  $(GREEN)docker-benchmark$(NC) -> cmd/noisefs-tools/benchmark/docker-benchmark/"
	@echo "  $(GREEN)enterprise-benchmark$(NC) -> cmd/noisefs-tools/benchmark/enterprise-benchmark/"
	@echo "  $(GREEN)impact-demo$(NC) -> cmd/noisefs-tools/benchmark/impact-demo/"

# Show project status
status:
	@echo -e "$(BLUE)Project Status:$(NC)"
	@echo "  Version: $(VERSION)"
	@echo "  Commit: $(GIT_COMMIT)"
	@echo "  Build Date: $(BUILD_DATE)"
	@echo "  Go Version: $(shell go version)"
	@echo "  Platform: $(GOOS)/$(GOARCH)"
	@if [ -d $(BUILD_DIR) ]; then \
		echo "  Binaries:"; \
		ls -la $(BUILD_DIR)/ | grep -v "^d" | awk '{print "    " $$9 " (" $$5 " bytes)"}'; \
	fi

# Evolution Analysis - Comprehensive impact testing of ALL NoiseFS optimizations
test-evolution:
	@echo -e "$(BLUE)🎯 Running comprehensive NoiseFS evolution analysis...$(NC)"
	@echo -e "$(YELLOW)This analyzes the impact of ALL optimizations made throughout the project$(NC)"
	cd tests/integration && go test -run TestEvolutionAnalyzer -v
	@echo -e "$(GREEN)✅ Evolution analysis completed$(NC)"

test-evolution-impact:
	@echo -e "$(BLUE)📊 Running evolution impact analysis...$(NC)"
	@echo -e "$(YELLOW)Shows cumulative impact of all NoiseFS improvements$(NC)"
	cd tests/integration && go test -run TestEvolutionImpactAnalysis -v
	@echo -e "$(GREEN)📈 Evolution impact analysis completed$(NC)"

test-ipfs-optimization:
	@echo -e "$(BLUE)🔧 Testing IPFS endpoint optimization impact...$(NC)"
	cd tests/integration && go test -run TestIPFSOptimizationImpact -v
	@echo -e "$(GREEN)🚀 IPFS optimization test completed$(NC)"
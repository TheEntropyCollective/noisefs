# NoiseFS Testing Framework Makefile

# Configuration
SHELL := /bin/bash
TEST_DIR := $(PWD)
ROOT_DIR := $(TEST_DIR)/..
GO := go
TIMEOUT := 30m
VERBOSE := -v

# Colors for output
RED := \033[0;31m
GREEN := \033[0;32m
YELLOW := \033[1;33m
BLUE := \033[0;34m
NC := \033[0m

# Docker configuration for test infrastructure
DOCKER_COMPOSE := docker-compose
IPFS_COMPOSE_FILE := docker-compose.ipfs.yml
TEST_NETWORK := noisefs-test-network

.PHONY: help test-all test-unit test-integration test-system test-performance test-compliance test-privacy

# Default target
help:
	@echo "$(BLUE)NoiseFS Testing Framework$(NC)"
	@echo ""
	@echo "$(YELLOW)Quick Testing:$(NC)"
	@echo "  $(GREEN)test-unit$(NC)          Run fast unit tests"
	@echo "  $(GREEN)test-integration$(NC)   Run integration tests with mocks"
	@echo "  $(GREEN)test-quick$(NC)         Run quick validation tests"
	@echo ""
	@echo "$(YELLOW)Comprehensive Testing:$(NC)"
	@echo "  $(GREEN)test-all$(NC)           Run complete test suite"
	@echo "  $(GREEN)test-system$(NC)        Run system tests with real infrastructure"
	@echo "  $(GREEN)test-performance$(NC)   Run performance benchmarks"
	@echo "  $(GREEN)test-compliance$(NC)    Run legal compliance tests"
	@echo "  $(GREEN)test-privacy$(NC)       Run privacy validation tests"
	@echo ""
	@echo "$(YELLOW)Real Infrastructure:$(NC)"
	@echo "  $(GREEN)test-real-ipfs$(NC)     Test with multi-node IPFS network"
	@echo "  $(GREEN)test-scenarios$(NC)     Run realistic usage scenarios"
	@echo "  $(GREEN)test-load$(NC)          Run load and stress tests"
	@echo "  $(GREEN)test-stability$(NC)     Run long-term stability tests"
	@echo ""
	@echo "$(YELLOW)Infrastructure Management:$(NC)"
	@echo "  $(GREEN)setup-test-env$(NC)     Set up complete test environment"
	@echo "  $(GREEN)cleanup-test-env$(NC)   Clean up test environment"
	@echo "  $(GREEN)ipfs-network-start$(NC) Start multi-node IPFS network"
	@echo "  $(GREEN)ipfs-network-stop$(NC)  Stop IPFS network"
	@echo "  $(GREEN)ipfs-network-status$(NC) Check IPFS network status"
	@echo ""
	@echo "$(YELLOW)Specialized Testing:$(NC)"
	@echo "  $(GREEN)test-dmca$(NC)          Test DMCA workflow"
	@echo "  $(GREEN)test-audit$(NC)         Test audit trail validation"
	@echo "  $(GREEN)test-anonymization$(NC) Test block anonymization"
	@echo "  $(GREEN)benchmark-storage$(NC)  Test storage efficiency"
	@echo "  $(GREEN)benchmark-comparative$(NC) Compare performance improvements"

# Main test targets
test-all: test-unit test-integration test-system test-performance
	@echo "$(GREEN)✓ All tests completed$(NC)"

test-unit:
	@echo "$(BLUE)Running unit tests...$(NC)"
	@cd $(ROOT_DIR) && $(GO) test $(VERBOSE) -short ./tests/unit/...
	@echo "$(GREEN)✓ Unit tests completed$(NC)"

test-integration:
	@echo "$(BLUE)Running integration tests...$(NC)"
	@cd $(ROOT_DIR) && $(GO) test $(VERBOSE) -timeout=$(TIMEOUT) ./tests/integration/...
	@echo "$(GREEN)✓ Integration tests completed$(NC)"

test-system: setup-test-env
	@echo "$(BLUE)Running system tests...$(NC)"
	@cd $(ROOT_DIR) && $(GO) test $(VERBOSE) -timeout=$(TIMEOUT) ./tests/system/...
	@echo "$(GREEN)✓ System tests completed$(NC)"

test-performance:
	@echo "$(BLUE)Running performance tests...$(NC)"
	@cd $(ROOT_DIR) && $(GO) test $(VERBOSE) -timeout=$(TIMEOUT) ./tests/benchmarks/...
	@echo "$(GREEN)✓ Performance tests completed$(NC)"

test-compliance:
	@echo "$(BLUE)Running compliance tests...$(NC)"
	@cd $(ROOT_DIR) && $(GO) test $(VERBOSE) -timeout=$(TIMEOUT) ./tests/compliance/...
	@echo "$(GREEN)✓ Compliance tests completed$(NC)"

test-privacy:
	@echo "$(BLUE)Running privacy tests...$(NC)"
	@cd $(ROOT_DIR) && $(GO) test $(VERBOSE) -timeout=$(TIMEOUT) ./tests/privacy/...
	@echo "$(GREEN)✓ Privacy tests completed$(NC)"

# Quick testing shortcuts
test-quick:
	@echo "$(BLUE)Running quick validation tests...$(NC)"
	@cd $(ROOT_DIR) && $(GO) test -short ./tests/unit/blocks ./tests/unit/cache ./tests/unit/noisefs
	@echo "$(GREEN)✓ Quick tests completed$(NC)"

test-unit-package:
	@if [ -z "$(PKG)" ]; then echo "$(RED)Error: PKG variable required$(NC)"; exit 1; fi
	@echo "$(BLUE)Running unit tests for package: $(PKG)$(NC)"
	@cd $(ROOT_DIR) && $(GO) test $(VERBOSE) -short ./tests/unit/$(PKG)

# Real infrastructure testing
test-real-ipfs: ipfs-network-start
	@echo "$(BLUE)Running real IPFS tests...$(NC)"
	@echo "$(YELLOW)Waiting for IPFS network to stabilize...$(NC)"
	@sleep 60
	@cd $(ROOT_DIR) && $(GO) test $(VERBOSE) -timeout=$(TIMEOUT) ./tests/system/ -tags=real_ipfs || ($(MAKE) ipfs-network-stop && exit 1)
	@$(MAKE) ipfs-network-stop
	@echo "$(GREEN)✓ Real IPFS tests completed$(NC)"

test-scenarios:
	@echo "$(BLUE)Running realistic usage scenarios...$(NC)"
	@cd $(ROOT_DIR) && $(GO) test $(VERBOSE) -timeout=$(TIMEOUT) ./tests/system/scenarios/...
	@echo "$(GREEN)✓ Scenario tests completed$(NC)"

test-load: setup-test-env
	@echo "$(BLUE)Running load tests...$(NC)"
	@cd $(ROOT_DIR) && $(GO) test $(VERBOSE) -timeout=60m ./tests/system/ -tags=load_test
	@echo "$(GREEN)✓ Load tests completed$(NC)"

test-stability: setup-test-env
	@echo "$(BLUE)Running stability tests (this may take several hours)...$(NC)"
	@cd $(ROOT_DIR) && $(GO) test $(VERBOSE) -timeout=24h ./tests/system/ -tags=stability_test
	@echo "$(GREEN)✓ Stability tests completed$(NC)"

# Infrastructure management
setup-test-env: docker-check
	@echo "$(BLUE)Setting up test environment...$(NC)"
	@$(MAKE) ipfs-network-start
	@echo "$(YELLOW)Waiting for services to initialize...$(NC)"
	@sleep 30
	@echo "$(GREEN)✓ Test environment ready$(NC)"

cleanup-test-env:
	@echo "$(BLUE)Cleaning up test environment...$(NC)"
	@$(MAKE) ipfs-network-stop
	@docker system prune -f >/dev/null 2>&1 || true
	@echo "$(GREEN)✓ Test environment cleaned$(NC)"

ipfs-network-start: docker-check
	@echo "$(BLUE)Starting multi-node IPFS network...$(NC)"
	@$(DOCKER_COMPOSE) -f $(IPFS_COMPOSE_FILE) up -d
	@echo "$(YELLOW)Waiting for IPFS nodes to connect...$(NC)"
	@sleep 30
	@echo "$(GREEN)✓ IPFS network started$(NC)"
	@echo "Available nodes:"
	@echo "  Node 1: http://localhost:5001"
	@echo "  Node 2: http://localhost:5002"
	@echo "  Node 3: http://localhost:5003"
	@echo "  Node 4: http://localhost:5004"
	@echo "  Node 5: http://localhost:5005"

ipfs-network-stop:
	@echo "$(BLUE)Stopping IPFS network...$(NC)"
	@$(DOCKER_COMPOSE) -f $(IPFS_COMPOSE_FILE) down -v >/dev/null 2>&1 || true
	@echo "$(GREEN)✓ IPFS network stopped$(NC)"

ipfs-network-status:
	@echo "$(BLUE)IPFS Network Status:$(NC)"
	@echo "==================="
	@for port in 5001 5002 5003 5004 5005; do \
		echo -n "Node $$port: "; \
		curl -s http://localhost:$$port/api/v0/version 2>/dev/null | jq -r '.Version' 2>/dev/null || echo "$(RED)Not responding$(NC)"; \
	done

# Specialized testing
test-dmca:
	@echo "$(BLUE)Running DMCA workflow tests...$(NC)"
	@cd $(ROOT_DIR) && $(GO) test $(VERBOSE) ./tests/compliance/dmca_simulation/...
	@echo "$(GREEN)✓ DMCA tests completed$(NC)"

test-audit:
	@echo "$(BLUE)Running audit trail tests...$(NC)"
	@cd $(ROOT_DIR) && $(GO) test $(VERBOSE) ./tests/compliance/audit_verification/...
	@echo "$(GREEN)✓ Audit tests completed$(NC)"

test-anonymization:
	@echo "$(BLUE)Running anonymization tests...$(NC)"
	@cd $(ROOT_DIR) && $(GO) test $(VERBOSE) ./tests/privacy/ -run TestAnonymization
	@echo "$(GREEN)✓ Anonymization tests completed$(NC)"

# Performance benchmarking
benchmark-storage:
	@echo "$(BLUE)Running storage efficiency benchmarks...$(NC)"
	@cd $(ROOT_DIR) && $(GO) test -bench=BenchmarkStorage -benchmem ./tests/benchmarks/storage_efficiency/...
	@echo "$(GREEN)✓ Storage benchmarks completed$(NC)"

benchmark-comparative:
	@echo "$(BLUE)Running comparative performance analysis...$(NC)"
	@cd $(ROOT_DIR) && $(GO) test -bench=. -benchmem ./tests/benchmarks/comparative_analysis/...
	@echo "$(GREEN)✓ Comparative benchmarks completed$(NC)"

benchmark-all:
	@echo "$(BLUE)Running all performance benchmarks...$(NC)"
	@cd $(ROOT_DIR) && $(GO) test -bench=. -benchmem ./tests/benchmarks/...
	@echo "$(GREEN)✓ All benchmarks completed$(NC)"

# Coverage and reporting
test-coverage:
	@echo "$(BLUE)Running tests with coverage...$(NC)"
	@cd $(ROOT_DIR) && $(GO) test -coverprofile=tests/coverage.out ./tests/unit/... ./tests/integration/...
	@cd $(ROOT_DIR) && $(GO) tool cover -html=tests/coverage.out -o tests/coverage.html
	@echo "$(GREEN)✓ Coverage report generated: tests/coverage.html$(NC)"

test-report:
	@echo "$(BLUE)Generating test report...$(NC)"
	@cd $(ROOT_DIR) && $(GO) test -json ./tests/... > tests/test-results.json
	@cd tools/report-generator && $(GO) run . ../../test-results.json ../../test-report.html
	@echo "$(GREEN)✓ Test report generated: test-report.html$(NC)"

# Debug and development
test-debug:
	@if [ -z "$(PKG)" ]; then echo "$(RED)Error: PKG variable required$(NC)"; exit 1; fi
	@echo "$(BLUE)Running debug tests for: $(PKG)$(NC)"
	@cd $(ROOT_DIR) && $(GO) test -v -run $(RUN) ./tests/$(PKG)

test-profile:
	@if [ -z "$(PKG)" ]; then echo "$(RED)Error: PKG variable required$(NC)"; exit 1; fi
	@echo "$(BLUE)Running profiled tests for: $(PKG)$(NC)"
	@cd $(ROOT_DIR) && $(GO) test -cpuprofile=tests/cpu.prof -memprofile=tests/mem.prof ./tests/$(PKG)
	@echo "$(GREEN)✓ Profiles generated: tests/cpu.prof, tests/mem.prof$(NC)"

# Continuous integration
test-ci: test-unit test-integration
	@echo "$(GREEN)✓ CI tests completed$(NC)"

test-ci-full: test-all
	@echo "$(GREEN)✓ Full CI tests completed$(NC)"

# Utilities
docker-check:
	@command -v docker >/dev/null 2>&1 || { echo "$(RED)Docker is required but not installed$(NC)"; exit 1; }
	@command -v docker-compose >/dev/null 2>&1 || { echo "$(RED)docker-compose is required but not installed$(NC)"; exit 1; }
	@docker info >/dev/null 2>&1 || { echo "$(RED)Docker daemon is not running$(NC)"; exit 1; }

clean:
	@echo "$(BLUE)Cleaning test artifacts...$(NC)"
	@rm -f tests/coverage.out tests/coverage.html tests/test-results.json tests/test-report.html
	@rm -f tests/cpu.prof tests/mem.prof
	@echo "$(GREEN)✓ Test artifacts cleaned$(NC)"

# Help with specific test patterns
test-help:
	@echo "$(YELLOW)Test Pattern Examples:$(NC)"
	@echo "  make test-unit-package PKG=cache"
	@echo "  make test-debug PKG=integration RUN=TestE2E"
	@echo "  make test-profile PKG=benchmarks"
	@echo ""
	@echo "$(YELLOW)Environment Variables:$(NC)"
	@echo "  VERBOSE=-v (default) or VERBOSE=-q for quiet"
	@echo "  TIMEOUT=30m (default) or custom timeout"
	@echo "  PKG=package_name for specific package testing"
	@echo "  RUN=test_pattern for specific test filtering"
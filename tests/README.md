# NoiseFS Testing Framework

This directory contains the comprehensive testing infrastructure for NoiseFS, providing validation for all system components and their interactions.

## Directory Structure

### Core Testing
- **`integration/`** - Integration tests between components
- **`system/`** - System-level tests with real infrastructure
- **Note:** Unit tests remain in their respective pkg/ directories following Go conventions

### Specialized Testing
- **`benchmarks/`** - Performance benchmarking and comparative analysis
- **`compliance/`** - Legal compliance and DMCA workflow testing
- **`privacy/`** - Privacy validation and anonymization verification

### Infrastructure
- **`infrastructure/`** - Test infrastructure, mocks, and utilities
- **`tools/`** - Testing tools and analysis utilities
- **`configs/`** - Test configurations and environment settings

## Test Categories

### 1. Unit Tests (in `pkg/*/`)
Fast, isolated tests for individual packages (located with the code):
```bash
go test ./pkg/...                 # Run all unit tests
go test ./pkg/storage/cache      # Run specific package tests
```

### 2. Integration Tests (`tests/integration/`)
Tests for component interactions:
```bash
make test-integration             # Mock-based integration tests
make test-integration-real        # Real infrastructure integration
```

### 3. System Tests (`tests/system/`)
Full system validation with real infrastructure:
```bash
make test-system                  # Complete system testing
make test-real-ipfs              # Multi-node IPFS testing
make test-scenarios              # Realistic usage scenarios
```

### 4. Performance Tests (`tests/benchmarks/`)
Performance validation and comparative analysis:
```bash
make test-performance            # Full performance suite
make benchmark-storage           # Storage efficiency tests
make benchmark-comparative       # Before/after analysis
```

### 5. Compliance Tests (`tests/compliance/`)
Legal compliance and DMCA workflow validation:
```bash
make test-compliance             # Full compliance testing
make test-dmca                   # DMCA workflow simulation
make test-audit                  # Audit trail verification
```

### 6. Privacy Tests (`tests/privacy/`)
Privacy and security validation:
```bash
make test-privacy                # Privacy protection tests
make test-anonymization          # Block anonymization verification
make test-traffic-analysis       # Traffic analysis resistance
```

## Test Infrastructure

### Multi-Node IPFS Environment
Real IPFS network with 5-10 nodes for authentic testing:
```bash
make ipfs-network-start          # Start test IPFS network
make ipfs-network-stop           # Stop and cleanup
make ipfs-network-status         # Check network health
```

### Mock Framework
Comprehensive mocks for fast testing:
- Mock IPFS clients
- Mock storage backends  
- Mock legal APIs
- Configurable failure injection

### Performance Monitoring
Real-time metrics collection during testing:
- Latency measurements
- Throughput analysis
- Storage efficiency tracking
- Cache hit rate monitoring

## Usage Examples

### Quick Testing
```bash
# Fast unit tests only
make test-unit

# Integration tests with mocks
make test-integration

# Quick system validation
make test-quick
```

### Comprehensive Testing
```bash
# Full test suite (may take 30+ minutes)
make test-all

# Performance validation
make test-performance

# Real-world scenario testing
make test-scenarios
```

### CI/CD Testing
```bash
# Continuous integration testing
make test-ci

# Performance regression testing
make test-performance-regression

# Security and compliance validation
make test-security-compliance
```

## Test Data

### Realistic Workloads
- Document management scenarios
- Media distribution patterns
- Backup and archival workflows
- Multi-user concurrent access

### Performance Baselines
- Pre-Milestone 4 performance data
- Storage efficiency benchmarks
- Latency and throughput baselines
- Cache effectiveness measurements

## Contributing

### Adding New Tests
1. Place unit tests in appropriate `tests/unit/{package}/` directory
2. Add integration tests to `tests/integration/`
3. Create system tests in `tests/system/` for end-to-end validation
4. Update this README with new test categories

### Test Requirements
- All tests must be deterministic and reproducible
- Use mocks for external dependencies in unit/integration tests
- Real infrastructure tests should clean up resources
- Performance tests should include baseline comparisons

### Test Standards
- Follow Go testing conventions
- Use table-driven tests where appropriate
- Include both positive and negative test cases
- Add proper error handling and cleanup

## Troubleshooting

### Common Issues
- **IPFS network failures**: Check Docker daemon and network connectivity
- **Performance test variations**: Ensure consistent test environment
- **Resource cleanup**: Use `make cleanup-test-env` to reset environment

### Debug Mode
```bash
# Run tests with verbose output
make test-debug PKG=integration

# Enable performance profiling
make test-profile PKG=system

# Generate detailed test reports
make test-report-detailed
```

This testing framework ensures comprehensive validation of all NoiseFS components and their complex interactions.
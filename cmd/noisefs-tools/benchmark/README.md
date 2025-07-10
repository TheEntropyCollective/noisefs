# NoiseFS Benchmarking Tools

This directory contains all NoiseFS performance testing and benchmarking tools, organized by use case.

## 🚀 Primary Tool (Use This 95% of the Time)

### `benchmark/` - Unified Performance Testing
**What it does:** Complete single-node and multi-node performance testing with automatic setup/cleanup

```bash
# Quick single-node test
go run cmd/benchmarks/benchmark/main.go -files 10 -verbose

# Multi-node cluster test (auto-manages IPFS nodes)
go run cmd/benchmarks/benchmark/main.go -nodes 3 -files 15 -verbose

# Stress test
go run cmd/benchmarks/benchmark/main.go -files 50 -file-size 262144
```

**Features:**
- ✅ Single-node and multi-node testing
- ✅ Automatic IPFS node management
- ✅ Cross-node replication testing
- ✅ Concurrent operations testing
- ✅ Clean setup/teardown
- ✅ Performance assessment

---

## 🔧 Specialized Tools (For Specific Scenarios)

### `docker-benchmark/` - Production-Like Testing
**What it does:** Tests against Docker containerized IPFS cluster for production validation

```bash
# Start Docker cluster first
docker-compose -f docker-compose.test.yml up -d

# Run production-like benchmark
go run cmd/benchmarks/docker-benchmark/main.go -nodes 5 -files 20 -verbose

# Cleanup
docker-compose -f docker-compose.test.yml down -v
```

**Use when:** 
- Testing production deployment scenarios
- Validating containerized environments
- Need real IPFS infrastructure testing

### `enterprise-benchmark/` - Enterprise Framework
**What it does:** Professional-grade benchmarking with FUSE filesystem testing and structured reporting

```bash
go run cmd/benchmarks/enterprise-benchmark/main.go
```

**Use when:**
- Enterprise/professional environments
- Need FUSE filesystem performance testing
- Require structured JSON/text reporting
- Configuration-driven test suites

### `impact-demo/` - Educational Tool
**What it does:** Demonstrates performance impact of specific NoiseFS features for presentations

```bash
go run cmd/benchmarks/impact-demo/main.go
```

**Use when:**
- Educational presentations
- Demonstrating feature impacts
- Stakeholder communications
- Algorithm comparison analysis

---

## 📊 Quick Decision Guide

| I want to... | Use this tool |
|--------------|---------------|
| Test performance quickly | `benchmark/` |
| Test multi-node cluster | `benchmark/` with `-nodes N` |
| Validate production setup | `docker-benchmark/` |
| Professional reporting | `enterprise-benchmark/` |
| Show feature impact | `impact-demo/` |

## 🎯 Most Common Usage

```bash
# For daily performance testing
go run cmd/benchmarks/benchmark/main.go -nodes 2 -files 10 -verbose
```

This covers 95% of all benchmarking needs with automatic setup, comprehensive testing, and clean results.
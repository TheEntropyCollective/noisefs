# NoiseFS Benchmarking Tools

This directory contains the unified NoiseFS performance testing and benchmarking tool.

## 🚀 Unified Benchmark Tool

### `unified/` - Complete Performance Testing Suite
**What it does:** All-in-one benchmark tool combining basic, Docker, enterprise, and demo modes

```bash
# Quick single-node test (basic mode)
go run cmd/noisefs-tools/benchmark/unified/main.go -files 10 -verbose

# Multi-node cluster test
go run cmd/noisefs-tools/benchmark/unified/main.go -nodes 3 -files 15 -verbose

# Docker multi-node testing
go run cmd/noisefs-tools/benchmark/unified/main.go -docker -nodes 5 -files 20 -verbose

# Enterprise-grade benchmarks
go run cmd/noisefs-tools/benchmark/unified/main.go -enterprise -type all -format json

# Feature demonstration mode
go run cmd/noisefs-tools/benchmark/unified/main.go -demo
```

**Features:**
- ✅ **Basic Mode**: Single-node and multi-node testing with automatic IPFS node management
- ✅ **Docker Mode**: Production-like testing against containerized IPFS clusters
- ✅ **Enterprise Mode**: Professional-grade benchmarks with FUSE testing and structured reporting  
- ✅ **Demo Mode**: Educational tool demonstrating NoiseFS feature performance impacts
- ✅ Unified command-line interface with mode flags
- ✅ Cross-node replication testing
- ✅ Concurrent operations testing
- ✅ Clean setup/teardown
- ✅ Comprehensive performance assessment

---

## 📊 Mode Selection Guide

| I want to... | Use this mode |
|--------------|---------------|
| Test performance quickly | Basic mode (default) |
| Test multi-node cluster | Basic mode with `-nodes N` |
| Validate production setup | Docker mode `-docker` |
| Professional reporting | Enterprise mode `-enterprise` |
| Show feature impact | Demo mode `-demo` |

## 🎯 Most Common Usage Examples

```bash
# Daily performance testing (basic mode)
go run cmd/noisefs-tools/benchmark/unified/main.go -nodes 2 -files 10 -verbose

# Production validation (docker mode)  
go run cmd/noisefs-tools/benchmark/unified/main.go -docker -nodes 3 -duration 5m

# Enterprise reporting (enterprise mode)
go run cmd/noisefs-tools/benchmark/unified/main.go -enterprise -format json -output results.json

# Educational demonstration (demo mode)
go run cmd/noisefs-tools/benchmark/unified/main.go -demo
```

## 🔄 Migration from Old Tools

The unified tool replaces the previous separate benchmark tools:
- `benchmark/main.go` → `unified/main.go` (basic mode)
- `docker-benchmark/main.go` → `unified/main.go -docker`
- `enterprise-benchmark/main.go` → `unified/main.go -enterprise`
- `impact-demo/main.go` → `unified/main.go -demo`

All functionality has been preserved and consolidated into a single, easy-to-use interface.
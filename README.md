# NoiseFS - Anonymous Distributed File Storage

NoiseFS is a privacy-focused distributed file system that implements the OFFSystem architecture on top of IPFS. It provides anonymous, efficient file storage with plausible deniability for all participants.

## Key Features

- **Anonymous Storage**: All blocks appear as random data through XOR anonymization
- **Efficient Block Reuse**: Smart caching minimizes storage overhead (~200% vs 900-2900% for traditional systems)
- **Plausible Deniability**: No original file content is ever stored
- **IPFS Integration**: Leverages IPFS's distributed network for resilience
- **Real-time Metrics**: Track efficiency and performance statistics

## Quick Start

### Prerequisites

1. Install [Go](https://golang.org/dl/) (1.21 or later)
2. Install [IPFS](https://docs.ipfs.tech/install/)
3. Start IPFS daemon:
   ```bash
   ipfs daemon
   ```

### Option 1: Using Make (Recommended)

1. **Build all binaries:**
   ```bash
   make build
   ```

2. **Start the web UI:**
   ```bash
   make dev-server
   ```

3. **Open your browser:**
   ```
   http://localhost:8080
   ```

### Option 2: Using Docker

1. **Quick deployment:**
   ```bash
   make deploy
   ```

2. **Access services:**
   - Web UI: http://localhost:8080
   - IPFS API: http://localhost:5001

### Option 3: Manual Build

1. **Build individual binaries:**
   ```bash
   # CLI application
   go build -o bin/noisefs ./cmd/noisefs
   
   # Web interface
   go build -o bin/webui ./cmd/webui
   
   # FUSE filesystem (requires FUSE libraries)
   go build -tags fuse -o bin/noisefs-mount ./cmd/noisefs-mount
   
   # Configuration tool
   go build -o bin/noisefs-config ./cmd/noisefs-config
   
   # Benchmarking tool
   go build -o bin/noisefs-benchmark ./cmd/noisefs-benchmark
   ```

2. **Use the binaries:**
   ```bash
   # Upload a file
   ./bin/noisefs upload myfile.txt
   
   # Download a file
   ./bin/noisefs download <descriptor_cid> -output downloaded_file.txt
   
   # Mount filesystem (FUSE)
   ./bin/noisefs-mount /mnt/noisefs
   ```

## How It Works

NoiseFS implements the OFFSystem architecture:

1. **File Splitting**: Files are divided into 128KB blocks
2. **Anonymization**: Each block is XORed with a randomizer block
3. **Storage**: Only anonymized blocks are stored in IPFS
4. **Reconstruction**: Files are rebuilt by XORing stored blocks with randomizers
5. **Efficiency**: Popular randomizer blocks are reused across multiple files

## Performance Metrics

The system tracks several efficiency metrics:

- **Block Reuse Rate**: Percentage of randomizers reused from cache
- **Cache Hit Rate**: Efficiency of block caching
- **Storage Efficiency**: Overhead compared to original file size
- **Network Activity**: Upload/download operations

## Security Properties

- **Content Privacy**: Stored blocks appear as random data
- **Metadata Privacy**: No direct mapping between blocks and files
- **Plausible Deniability**: Hosts cannot prove what content they're storing
- **Distributed Risk**: No single point of failure or control

## Development

### Project Structure

```
├── cmd/                    # Applications and tools
│   ├── noisefs/           # Main CLI application
│   ├── noisefs-mount/     # FUSE filesystem
│   ├── noisefs-benchmark/ # Performance benchmarking
│   ├── noisefs-config/    # Configuration management
│   └── webui/             # Web interface
├── pkg/                    # Go packages
│   ├── blocks/            # File splitting and assembly
│   ├── cache/             # Advanced caching system
│   ├── config/            # Configuration management
│   ├── descriptors/       # File metadata management
│   ├── fuse/              # FUSE filesystem integration
│   ├── ipfs/              # IPFS integration
│   ├── logging/           # Structured logging
│   ├── noisefs/           # High-level client API
│   └── benchmarks/        # Performance testing
├── deployments/           # Docker and Kubernetes configs
│   ├── docker/            # Docker configurations
│   ├── kubernetes/        # Kubernetes manifests
│   ├── Dockerfile         # Container build
│   └── docker-compose.yml # Service orchestration
├── configs/               # Configuration examples
├── scripts/               # Build and deployment scripts
├── docs/                  # Documentation
├── bin/                   # Built binaries (gitignored)
└── dist/                  # Distribution packages (gitignored)
```

### Development Commands

```bash
# Build everything
make build

# Run tests
make test

# Run with coverage
make test-coverage

# Run benchmarks
make bench

# Run linters
make lint

# Format code
make fmt

# Development build with race detection
make dev

# Build with FUSE support
make build-fuse

# Create distribution packages
make dist

# Install to system
make install

# Docker deployment
make deploy

# Clean build artifacts
make clean
```

### Running Tests

```bash
# All tests
make test

# With coverage report
make test-coverage

# Benchmarks
make bench

# FUSE tests (requires FUSE)
make test BUILD_TAGS=fuse
```

### Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

[Add your license here]

## Roadmap

- [x] Unit test coverage
- [x] FUSE filesystem integration
- [ ] Performance optimizations
- [ ] Privacy analysis and documentation
- [ ] Mobile applications
- [ ] Federation between IPFS networks

## Support

For questions, issues, or contributions, please [open an issue](https://github.com/your-repo/noisefs/issues) on GitHub.
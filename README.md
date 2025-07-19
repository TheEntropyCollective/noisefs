# NoiseFS

NoiseFS is a privacy-preserving distributed filesystem built on IPFS. It uses 3-tuple XOR anonymization to make file blocks indistinguishable from random data while maintaining efficient storage overhead.

## Key Features

- **Strong Privacy**: XOR-based block anonymization provides plausible deniability
- **Efficient Storage**: ~30% measured overhead vs 900-2900% for other anonymous systems
- **IPFS Integration**: Built on mature, decentralized storage infrastructure
- **FUSE Support**: Mount as a regular filesystem on Linux/macOS
- **Direct Access**: No routing through intermediate nodes

## How It Works

NoiseFS splits files into 128 KiB blocks and XORs each with two randomizer blocks:

```
Original Block ⊕ Randomizer1 ⊕ Randomizer2 = Anonymous Block
```

The resulting blocks appear as random data. Randomizer blocks are reused across multiple files for plausible deniability. Only the encrypted descriptor needed for reconstruction is stored separately.

## Quick Start

### Prerequisites

- Go 1.19+
- IPFS daemon running
- FUSE support (Linux/macOS)

### Install and Run

```bash
# Clone and build
git clone https://github.com/TheEntropyCollective/noisefs.git
cd noisefs
go build -o bin/noisefs ./cmd/noisefs

# Upload a file
echo "Hello, NoiseFS!" > test.txt
./bin/noisefs upload test.txt

# Download back
./bin/noisefs download <descriptor-cid> recovered.txt

# Mount as filesystem (optional)
mkdir /tmp/noisefs-mount
./bin/noisefs mount /tmp/noisefs-mount
```

## Architecture

NoiseFS implements a layered architecture:

- **Block Layer**: File splitting and XOR anonymization
- **Storage Layer**: IPFS integration with multiple backend support  
- **Cache Layer**: Smart block reuse and performance optimization
- **CLI/FUSE**: User interfaces for file operations

## Performance

Based on corrected storage overhead benchmarks (July 2025):

- **System Maturity Effect**: 200% overhead for first file → 0% overhead in mature systems
- **Randomizer Reuse**: Effective block reuse eliminates overhead after initial randomizer creation
- **Cold vs Warm**: Cold systems show 200% overhead, mature systems approach 0% overhead
- **Block Size**: Fixed 128 KiB for privacy uniformity  
- **Retrieval**: 3-block overhead (1 anonymous + 2 randomizers) vs direct IPFS

## Legal Considerations

NoiseFS implements technical privacy measures but users must understand their legal obligations. The system provides:

- Technical anonymization of stored content
- Plausible deniability through randomizer reuse
- No knowledge of original content by storage nodes

Users remain responsible for compliance with applicable laws.

## Documentation

- [Installation Guide](docs/INSTALL.md) - Detailed setup instructions
- [API Reference](docs/API.md) - Command-line interface reference
- [Architecture Overview](docs/ARCHITECTURE.md) - Technical design details
- [Troubleshooting](docs/TROUBLESHOOTING.md) - Common issues and solutions

## Contributing

NoiseFS is experimental software. Contributions welcome:

1. Fork the repository
2. Create a feature branch
3. Submit a pull request

## License

[License information needed]

## Disclaimer

NoiseFS is experimental software focused on technical privacy research. Users must evaluate legal compliance in their jurisdiction.
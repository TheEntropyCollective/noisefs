# Installation Guide

## System Requirements

- **Go**: Version 1.19 or higher
- **IPFS**: Running IPFS daemon (version 0.15+)
- **FUSE**: Kernel support for FUSE (Linux/macOS)
- **Memory**: Minimum 2GB RAM (4GB recommended)
- **Disk**: At least 10GB free space

## Installation

### From Source

1. **Clone the repository**
   ```bash
   git clone https://github.com/TheEntropyCollective/noisefs.git
   cd noisefs
   ```

2. **Build all components**
   ```bash
   # Build main CLI
   go build -o bin/noisefs ./cmd/noisefs
   
   # Build additional tools (optional)
   go build -o bin/noisefs-mount ./cmd/noisefs-mount
   go build -o bin/noisefs-webui ./cmd/noisefs-webui
   ```

3. **Install to system PATH (optional)**
   ```bash
   sudo cp bin/* /usr/local/bin/
   ```

## Setup

### 1. Install and Start IPFS

```bash
# Download IPFS (if not already installed)
wget https://dist.ipfs.io/go-ipfs/v0.15.0/go-ipfs_v0.15.0_linux-amd64.tar.gz
tar -xzf go-ipfs_v0.15.0_linux-amd64.tar.gz
sudo mv go-ipfs/ipfs /usr/local/bin/

# Initialize and start IPFS
ipfs init
ipfs daemon
```

### 2. Verify Installation

```bash
# Check NoiseFS version
./bin/noisefs version

# Test basic operation
echo "Hello NoiseFS" > test.txt
./bin/noisefs upload test.txt
```

## Configuration

NoiseFS uses IPFS defaults but can be configured:

```bash
# Set custom IPFS endpoint
export NOISEFS_IPFS_ENDPOINT="127.0.0.1:5001"

# Set block size (default 131072 = 128 KiB)
export NOISEFS_BLOCK_SIZE=131072

# Enable debug logging
export NOISEFS_LOG_LEVEL=debug
```

## Troubleshooting

### IPFS Connection Issues

If you see "failed to connect to IPFS":

1. Ensure IPFS daemon is running: `ipfs daemon`
2. Check IPFS API endpoint: `curl http://127.0.0.1:5001/api/v0/version`
3. Verify firewall settings allow port 5001

### FUSE Mount Issues

If mounting fails:

1. Install FUSE: `sudo apt install fuse` (Ubuntu) or `brew install macfuse` (macOS)
2. Check permissions: User must be in `fuse` group on Linux
3. Create mount directory: `mkdir /tmp/noisefs-mount`

### Build Issues

If build fails:

1. Update Go: NoiseFS requires Go 1.19+
2. Clear module cache: `go clean -modcache`
3. Retry build: `go build -v ./cmd/noisefs`

## Next Steps

- Read the [API Reference](API.md) for command details
- Check [Architecture Overview](ARCHITECTURE.md) for technical details
- See [Troubleshooting](TROUBLESHOOTING.md) for common issues
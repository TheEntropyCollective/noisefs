# NoiseFS Installation Guide

## System Requirements

- **Go**: Version 1.19 or higher
- **IPFS**: Running IPFS daemon (version 0.15+)
- **FUSE**: Kernel support for FUSE (Linux/macOS)
- **Memory**: Minimum 2GB RAM (4GB recommended)
- **Disk**: At least 10GB free space

## Installation Methods

### Installing from Source

1. **Clone the repository**
   ```bash
   git clone https://github.com/TheEntropyCollective/noisefs.git
   cd noisefs
   ```

2. **Build all components**
   ```bash
   make build
   ```

   This creates binaries in the `bin/` directory:
   - `noisefs` - Main CLI tool
   - `noisefs-mount` - FUSE mounting tool
   - `noisefs-webui` - Unified web interface server (file management + announcements)
   - `noisefs-config` - Configuration tool
   - `noisefs-security` - Security utilities

3. **Install to system (optional)**
   ```bash
   sudo make install
   ```

### Using Docker

1. **Pull the Docker image**
   ```bash
   docker pull theentropycollective/noisefs:latest
   ```

2. **Run with FUSE support**
   ```bash
   docker run -it --privileged \
     --device /dev/fuse \
     --cap-add SYS_ADMIN \
     -v /tmp/noisefs:/mnt/noisefs \
     theentropycollective/noisefs
   ```

### Platform-Specific Instructions

#### macOS
```bash
# Install FUSE for macOS
brew install --cask macfuse

# Build NoiseFS
make build
```

#### Ubuntu/Debian
```bash
# Install dependencies
sudo apt-get update
sudo apt-get install -y golang fuse libfuse-dev

# Build NoiseFS
make build
```

#### Fedora/RHEL
```bash
# Install dependencies
sudo dnf install -y golang fuse fuse-devel

# Build NoiseFS
make build
```

## Post-Installation Setup

### 1. Verify IPFS is Running

```bash
# Check IPFS daemon status
ipfs id

# If not running, start it
ipfs daemon &
```

### 2. Initialize NoiseFS Configuration

```bash
# Create default configuration
noisefs-config init

# This creates ~/.noisefs/config.json
```

### 3. Test Installation

```bash
# Upload a test file
echo "Hello NoiseFS" > test.txt
noisefs upload test.txt

# List uploaded files
noisefs list

# Mount filesystem (requires FUSE)
mkdir -p ~/noisefs-mount
noisefs-mount ~/noisefs-mount
```

## Configuration

### Basic Configuration

The default configuration file is located at `~/.noisefs/config.json`:

```json
{
  "ipfs": {
    "api_endpoint": "http://localhost:5001"
  },
  "cache": {
    "enabled": true,
    "max_size": 1000,
    "memory_limit": 268435456
  },
  "fuse": {
    "allow_other": false,
    "debug": false
  }
}
```

### Environment Variables

You can override configuration with environment variables:

```bash
export NOISEFS_IPFS_ENDPOINT="http://localhost:5001"
export NOISEFS_CACHE_SIZE="2000"
export NOISEFS_DEBUG="true"
```

## Verifying Installation

Run the following commands to verify everything is working:

```bash
# Check version
noisefs version

# Test IPFS connectivity
noisefs test-ipfs

# Run built-in demo
noisefs demo
```

## Troubleshooting

### IPFS Connection Failed

```bash
# Ensure IPFS is running
ipfs daemon --enable-pubsub-experiment &

# Check IPFS API endpoint
curl http://localhost:5001/api/v0/id
```

### FUSE Mount Failed

```bash
# Check FUSE is installed
ls /dev/fuse

# On Linux, ensure user is in fuse group
sudo usermod -a -G fuse $USER

# Log out and back in for group changes
```

### Permission Denied

```bash
# Ensure proper permissions on config directory
chmod 700 ~/.noisefs
chmod 600 ~/.noisefs/config.json
```

## Next Steps

- Read the [CLI Usage Guide](cli-usage.md) to learn basic commands
- See [Configuration Reference](configuration.md) for advanced options
- Try the [Quick Start Tutorial](quickstart.md) for a hands-on introduction

## Uninstallation

```bash
# Remove installed binaries
sudo make uninstall

# Remove configuration and data
rm -rf ~/.noisefs

# Remove Docker image
docker rmi theentropycollective/noisefs
```
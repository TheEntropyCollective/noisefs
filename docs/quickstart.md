# NoiseFS Quick Start Guide

This guide will get you using NoiseFS in 5 minutes. For detailed instructions, see the [Installation Guide](installation.md).

## Prerequisites

- Go 1.19+ installed
- IPFS daemon running (`ipfs daemon`)
- FUSE support (Linux/macOS)

## 1. Install NoiseFS

```bash
# Clone and build
git clone https://github.com/TheEntropyCollective/noisefs.git
cd noisefs
make build

# Add to PATH (optional)
export PATH=$PATH:$(pwd)/bin
```

## 2. Start Using NoiseFS

### Upload Your First File

```bash
# Create a test file
echo "Hello, NoiseFS!" > hello.txt

# Upload it
noisefs upload hello.txt
```

Output:
```
Uploading: hello.txt (16 bytes)
✓ File split into 1 blocks
✓ Uploaded to IPFS
✓ Descriptor created: QmXoypizjW3WknFiJnKLwHCnL72vedxjQkDDP1mXWo6uco
✓ Added to index: hello.txt
```

### List Your Files

```bash
noisefs list
```

Output:
```
Files in NoiseFS:
  hello.txt (16 bytes) - uploaded 2 minutes ago
```

### Download a File

```bash
# Download by name
noisefs download hello.txt -o downloaded.txt

# Or download by descriptor CID
noisefs download --cid QmXoypizjW3WknFiJnKLwHCnL72vedxjQkDDP1mXWo6uco
```

## 3. Mount as Filesystem (Optional)

```bash
# Create mount point
mkdir ~/noisefs-mount

# Mount NoiseFS
noisefs-mount ~/noisefs-mount

# Use like a regular filesystem
cp myfile.pdf ~/noisefs-mount/files/
ls ~/noisefs-mount/files/

# Unmount when done
umount ~/noisefs-mount
```

## 4. Use the Web Interface (Optional)

```bash
# Start web UI
noisefs-webui

# Open in browser
open http://localhost:8080
```

## Common Operations

### Upload Multiple Files

```bash
# Upload all PDFs in a directory
noisefs upload *.pdf

# Upload with high privacy
noisefs upload sensitive.doc --privacy high
```

### Share Files

```bash
# Get shareable descriptor
noisefs info myfile.pdf | grep "Descriptor CID"

# Others can download with:
noisefs download --cid <descriptor-cid>
```

### Manage Cache

```bash
# View cache stats
noisefs cache stats

# Clear cache if needed
noisefs cache clear
```

## What's Next?

- Read the [CLI Usage Guide](cli-usage.md) for all commands
- Configure NoiseFS with the [Configuration Guide](configuration.md)
- Learn about [Privacy Features](privacy-infrastructure.md)
- Explore [Advanced Features](block-management.md)

## Getting Help

```bash
# Show help
noisefs --help

# Get command help
noisefs upload --help

# Check system status
noisefs status
```

## Troubleshooting

If you encounter issues:

1. **Check IPFS is running**: `ipfs id`
2. **Verify installation**: `noisefs version`
3. **Enable debug mode**: `noisefs --debug upload file.txt`
4. **See [Troubleshooting Guide](troubleshooting.md)**

## Example: Secure Document Storage

```bash
# 1. Upload important documents with high privacy
noisefs upload tax-returns.pdf --privacy high
noisefs upload passport-scan.jpg --privacy high

# 2. List your secure files
noisefs list --filter "*.pdf"

# 3. Create encrypted backup
noisefs export --all -o backup-$(date +%Y%m%d).noisefs

# 4. Share specific file (provide descriptor CID)
noisefs info tax-returns.pdf
```

That's it! You're now using distributed, privacy-preserving storage with NoiseFS.
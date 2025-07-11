# NoiseFS CLI Usage Guide

## Overview

The `noisefs` command-line tool is the primary interface for interacting with NoiseFS. It provides commands for uploading, downloading, listing, and managing files in the distributed, privacy-preserving filesystem.

## Basic Usage

```bash
noisefs [global options] command [command options] [arguments...]
```

## Global Options

- `--config PATH` - Specify custom config file (default: `~/.noisefs/config.json`)
- `--ipfs-endpoint URL` - Override IPFS API endpoint (default: `http://localhost:5001`)
- `--debug` - Enable debug logging
- `--quiet` - Suppress non-error output
- `--help, -h` - Show help
- `--version, -v` - Print version

## Commands

### File Operations

#### `upload` - Upload a file to NoiseFS

```bash
# Upload a single file
noisefs upload document.pdf

# Upload with custom name
noisefs upload document.pdf --name "My Document.pdf"

# Upload multiple files
noisefs upload file1.txt file2.txt file3.txt

# Upload with privacy level
noisefs upload sensitive.doc --privacy high
```

Options:
- `--name, -n` - Specify filename in NoiseFS
- `--privacy` - Privacy level: low, medium, high (default: medium)
- `--no-cache` - Skip local caching

#### `download` - Download a file from NoiseFS

```bash
# Download by filename
noisefs download "My Document.pdf"

# Download to specific location
noisefs download "My Document.pdf" -o ~/Downloads/

# Download by descriptor CID
noisefs download --cid QmXoypizjW3WknFiJnKLwHCnL72vedxjQkDDP1mXWo6uco
```

Options:
- `--output, -o` - Output directory or filename
- `--cid` - Download by descriptor CID instead of name
- `--force` - Overwrite existing files

#### `list` - List files in NoiseFS

```bash
# List all files
noisefs list

# List with details
noisefs list --detailed

# List files in directory
noisefs list --dir documents/

# Filter by pattern
noisefs list --filter "*.pdf"
```

Options:
- `--detailed, -d` - Show file sizes and dates
- `--dir` - List files in specific directory
- `--filter` - Filter by glob pattern
- `--json` - Output in JSON format

#### `delete` - Remove files from NoiseFS

```bash
# Delete a file
noisefs delete "old-document.pdf"

# Delete multiple files
noisefs delete file1.txt file2.txt

# Delete with confirmation
noisefs delete important.doc --confirm
```

Options:
- `--confirm` - Skip confirmation prompt
- `--recursive, -r` - Delete directories recursively

### File Information

#### `info` - Show file information

```bash
# Get file info
noisefs info "My Document.pdf"

# Show extended information
noisefs info "My Document.pdf" --extended

# Output as JSON
noisefs info "My Document.pdf" --json
```

Output includes:
- Filename and size
- Descriptor CID
- Upload date
- Block count
- Privacy level

#### `verify` - Verify file integrity

```bash
# Verify a file
noisefs verify "important.doc"

# Verify all files
noisefs verify --all

# Verify and repair
noisefs verify "corrupted.file" --repair
```

### System Commands

#### `status` - Show system status

```bash
# Basic status
noisefs status

# Detailed status with metrics
noisefs status --detailed
```

Shows:
- IPFS connection status
- Cache statistics
- Active operations
- System health

#### `cache` - Manage local cache

```bash
# Show cache statistics
noisefs cache stats

# Clear cache
noisefs cache clear

# Set cache size
noisefs cache resize 2000

# List cached blocks
noisefs cache list
```

#### `config` - View/edit configuration

```bash
# Show current config
noisefs config show

# Set configuration value
noisefs config set cache.max_size 2000

# Reset to defaults
noisefs config reset
```

### Advanced Commands

#### `export` - Export file with descriptor

```bash
# Export to portable format
noisefs export "document.pdf" -o document.noisefs

# Export multiple files
noisefs export --all -o backup.tar
```

#### `import` - Import file with descriptor

```bash
# Import from export
noisefs import document.noisefs

# Import from descriptor CID
noisefs import --descriptor QmXoyp... --name "Restored.pdf"
```

#### `mount` - Mount as FUSE filesystem

```bash
# Mount to directory
noisefs mount ~/noisefs-files

# Mount read-only
noisefs mount ~/noisefs-files --read-only

# Mount with debug output
noisefs mount ~/noisefs-files --debug
```

## Common Workflows

### Uploading and Sharing Files

```bash
# 1. Upload a file
$ noisefs upload presentation.pptx
Uploaded: presentation.pptx
Descriptor CID: QmXoypizjW3WknFiJnKLwHCnL72vedxjQkDDP1mXWo6uco

# 2. Share the descriptor CID with others
# They can download using:
$ noisefs download --cid QmXoypizjW3WknFiJnKLwHCnL72vedxjQkDDP1mXWo6uco
```

### Backing Up Files

```bash
# 1. Export all files
$ noisefs export --all -o noisefs-backup-$(date +%Y%m%d).tar

# 2. Later, restore from backup
$ noisefs import noisefs-backup-20240115.tar
```

### Working with Mounted Filesystem

```bash
# 1. Mount NoiseFS
$ noisefs mount ~/secure-files

# 2. Use regular file operations
$ cp important.doc ~/secure-files/files/
$ ls ~/secure-files/files/

# 3. Unmount when done
$ umount ~/secure-files
```

### Privacy Levels

NoiseFS supports three privacy levels:

- **Low**: Basic anonymization, optimized for performance
- **Medium**: Balanced privacy and performance (default)
- **High**: Maximum privacy with additional randomization

```bash
# Upload with high privacy
$ noisefs upload sensitive-data.csv --privacy high

# Configure default privacy level
$ noisefs config set privacy.default_level high
```

## Error Handling

Common errors and solutions:

### IPFS Not Connected
```bash
Error: cannot connect to IPFS API

# Solution: Start IPFS daemon
$ ipfs daemon &
```

### File Not Found
```bash
Error: file not found in index

# Solution: List available files
$ noisefs list
```

### Insufficient Cache
```bash
Warning: cache full, performance may degrade

# Solution: Increase cache size
$ noisefs cache resize 5000
```

## Performance Tips

1. **Use local IPFS node** for best performance
2. **Enable caching** for frequently accessed files
3. **Batch operations** when uploading multiple files
4. **Use appropriate privacy levels** - high privacy impacts performance
5. **Monitor cache hit rate** with `noisefs cache stats`

## Getting Help

```bash
# General help
noisefs help

# Command-specific help
noisefs upload --help

# View documentation
noisefs docs
```

## See Also

- [Configuration Reference](configuration.md) - Detailed configuration options
- [Web UI Guide](webui-guide.md) - Using the web interface
- [Troubleshooting](troubleshooting.md) - Common issues and solutions
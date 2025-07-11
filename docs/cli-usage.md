# NoiseFS CLI Usage Guide

## Overview

The `noisefs` command-line tool is the primary interface for interacting with NoiseFS. It provides functionality for uploading and downloading files with 3-tuple XOR anonymization, viewing system statistics, and managing the distributed, privacy-preserving filesystem.

## Basic Usage

```bash
noisefs [options]
```

## Global Options

- `-config PATH` - Specify custom config file (default: `~/.noisefs/config.json`)
- `-api URL` - Override IPFS API endpoint (default: `http://localhost:5001`)
- `-quiet` - Minimal output (only show errors and results)
- `-json` - Output results in JSON format
- `-block-size SIZE` - Block size in bytes (overrides config)
- `-cache-size SIZE` - Number of blocks to cache in memory (overrides config)

## Commands

### Upload a File

```bash
# Basic upload
noisefs -upload file.txt

# Upload with custom block size
noisefs -upload large-file.bin -block-size 262144

# Upload with quiet mode (only shows descriptor CID)
noisefs -upload document.pdf -quiet

# Upload with JSON output
noisefs -upload data.csv -json
```

The upload command:
- Splits the file into blocks
- Selects randomizer blocks for 3-tuple XOR anonymization
- Stores anonymized blocks in IPFS
- Returns a descriptor CID for later retrieval
- Shows progress bars and metrics (unless -quiet is used)

### Download a File

```bash
# Basic download
noisefs -download <descriptor-cid> -output recovered-file.txt

# Download with quiet mode
noisefs -download <descriptor-cid> -output file.pdf -quiet

# Download with JSON output
noisefs -download <descriptor-cid> -output data.csv -json
```

The download command:
- Retrieves the descriptor from IPFS
- Downloads anonymized blocks and randomizers
- Reconstructs the original file using XOR operations
- Saves to the specified output path
- Shows progress bars (unless -quiet is used)

### View System Statistics

```bash
# Show system statistics
noisefs -stats

# Statistics with JSON output
noisefs -stats -json
```

The stats command displays:
- IPFS connection status and peer count
- Cache statistics (hits, misses, hit rate)
- Block management metrics (reuse rate)
- Storage efficiency
- Upload/download history

## Output Formats

### Standard Output

By default, NoiseFS provides human-readable output with progress bars and formatted results:

```bash
$ noisefs -upload document.pdf
Splitting file [████████████████████] 100.0% 1.23 MB/1.23 MB
Uploading blocks [████████████████████] 100.0% 10/10

Upload complete!
Descriptor CID: QmXoypizjW3WknFiJnKLwHCnL72vedxjQkDDP1mXWo6uco

--- NoiseFS Metrics ---
Block Reuse Rate: 30.0% (3 reused, 7 generated)
Cache Hit Rate: 45.0% (9 hits, 11 misses)
Storage Efficiency: 180.0% overhead
```

### Quiet Mode

Use `-quiet` for minimal output, ideal for scripting:

```bash
$ noisefs -upload document.pdf -quiet
QmXoypizjW3WknFiJnKLwHCnL72vedxjQkDDP1mXWo6uco
```

### JSON Output

Use `-json` for machine-readable output:

```bash
$ noisefs -upload document.pdf -json
{
  "success": true,
  "data": {
    "result": {
      "descriptor_cid": "QmXoypizjW3WknFiJnKLwHCnL72vedxjQkDDP1mXWo6uco",
      "filename": "document.pdf",
      "file_size": 1234567,
      "block_count": 10,
      "block_size": 131072
    }
  }
}
```

## Error Handling

NoiseFS provides helpful error messages with suggestions:

```bash
$ noisefs -upload missing-file.txt
Error: failed to open file: open missing-file.txt: no such file or directory
💡 Suggestion: Check the file path and ensure the file exists

$ noisefs -download QmInvalidCID -output file.txt
Error: failed to load descriptor: ipfs get: invalid CID
💡 Suggestion: The descriptor CID may be invalid or the descriptor is not available in IPFS
```

## Common Use Cases

### Backup Important Files

```bash
# Upload a file
noisefs -upload important-document.pdf

# Save the descriptor CID
echo "QmXoypizjW3WknFiJnKLwHCnL72vedxjQkDDP1mXWo6uco" > backup-cids.txt

# Later, recover the file
noisefs -download QmXoypizjW3WknFiJnKLwHCnL72vedxjQkDDP1mXWo6uco -output recovered-document.pdf
```

### Scripting with NoiseFS

```bash
#!/bin/bash
# Backup script using NoiseFS

# Upload file and capture CID
CID=$(noisefs -upload "$1" -quiet)

# Save CID with metadata
echo "$(date): $1 -> $CID" >> noisefs-backups.log

# Verify upload
noisefs -download "$CID" -output "/tmp/verify-$$" -quiet
if cmp -s "$1" "/tmp/verify-$$"; then
    echo "Backup verified: $1"
    rm "/tmp/verify-$$"
else
    echo "Backup verification failed!"
    exit 1
fi
```

### Monitoring System Health

```bash
# Check system status as JSON
noisefs -stats -json | jq '.data.result.ipfs.connected'

# Monitor cache performance
watch -n 5 'noisefs -stats | grep "Cache Hit Rate"'
```

## Configuration

NoiseFS uses a JSON configuration file located at `~/.noisefs/config.json`. You can override settings using command-line flags or environment variables.

### Example Configuration

```json
{
  "ipfs": {
    "api_endpoint": "http://localhost:5001"
  },
  "cache": {
    "max_size": 1000
  },
  "performance": {
    "block_size": 131072
  }
}
```

### Environment Variables

All configuration options can be overridden using environment variables:

```bash
export NOISEFS_IPFS_API_ENDPOINT="http://192.168.1.100:5001"
export NOISEFS_CACHE_MAX_SIZE="5000"
export NOISEFS_PERFORMANCE_BLOCK_SIZE="262144"
```

## Troubleshooting

### IPFS Connection Issues

If you see "Failed to connect to IPFS":
1. Ensure IPFS daemon is running: `ipfs daemon`
2. Check the API endpoint: `noisefs -api http://localhost:5001 -stats`
3. Verify IPFS is accessible: `curl http://localhost:5001/api/v0/id`

### Performance Issues

For large files:
- Increase block size: `-block-size 1048576` (1MB blocks)
- Increase cache size: `-cache-size 5000`
- Use quiet mode to reduce output overhead: `-quiet`

### Storage Issues

If uploads fail with storage errors:
1. Check IPFS repo size: `ipfs repo stat`
2. Run garbage collection: `ipfs repo gc`
3. Ensure adequate disk space

## See Also

- [Installation Guide](installation.md) - How to install NoiseFS
- [Configuration Reference](configuration.md) - Detailed configuration options
- [Architecture Overview](../README.md) - Understanding NoiseFS internals
- [Troubleshooting Guide](troubleshooting.md) - Common issues and solutions
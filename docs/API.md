# API Reference

## Command Line Interface

### Basic Commands

#### Upload Files

```bash
# Upload a single file
noisefs upload <file>
noisefs upload document.pdf

# Upload with custom encryption
noisefs upload --encrypt <file>

# Upload directory (creates descriptor for each file)
noisefs upload --recursive <directory>
```

**Output**: Returns a descriptor CID for file reconstruction.

#### Download Files

```bash
# Download using descriptor
noisefs download <descriptor-cid> <output-file>
noisefs download QmExampleDescriptor output.pdf

# Download to stdout
noisefs download <descriptor-cid> -
```

#### Status and Information

```bash
# Show version
noisefs version

# Check IPFS connection
noisefs status

# Display statistics
noisefs stats
```

### FUSE Filesystem

#### Mount/Unmount

```bash
# Mount NoiseFS as filesystem
noisefs mount <mount-point>
noisefs mount /tmp/noisefs

# Unmount
fusermount -u /tmp/noisefs  # Linux
umount /tmp/noisefs         # macOS
```

#### Using Mounted Filesystem

Once mounted, use like any filesystem:

```bash
# Copy files
cp document.pdf /tmp/noisefs/

# View files
ls /tmp/noisefs/
cat /tmp/noisefs/document.pdf

# Remove files
rm /tmp/noisefs/document.pdf
```

### Sync Commands

Sync enables real-time directory synchronization:

```bash
# Start sync session
noisefs sync start <sync-id> <local-path> <remote-path>
noisefs sync start project /home/user/project /remote/project

# Monitor sync status
noisefs sync status project

# List all active syncs
noisefs sync list

# Stop sync
noisefs sync stop project
```

## Configuration

### Environment Variables

- `NOISEFS_IPFS_ENDPOINT`: IPFS API endpoint (default: 127.0.0.1:5001)
- `NOISEFS_BLOCK_SIZE`: Block size in bytes (default: 131072)
- `NOISEFS_LOG_LEVEL`: Logging level (debug, info, warn, error)
- `NOISEFS_CACHE_SIZE`: Cache size in MB (default: 100)

### Configuration File

Create `~/.noisefs/config.json`:

```json
{
  "ipfs_endpoint": "127.0.0.1:5001",
  "block_size": 131072,
  "cache_size_mb": 100,
  "privacy": {
    "enable_cover_traffic": true,
    "randomizer_pool_size": 1000
  }
}
```

## Go API

### Basic Usage

```go
package main

import (
    "github.com/TheEntropyCollective/noisefs/pkg/core/client"
    "github.com/TheEntropyCollective/noisefs/pkg/storage"
    "github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
)

func main() {
    // Create storage manager
    config := storage.DefaultConfig()
    manager, err := storage.NewManager(config)
    if err != nil {
        panic(err)
    }
    
    // Create cache
    cache := cache.NewMemoryCache(100) // 100MB
    
    // Create client
    client, err := client.NewClient(manager, cache)
    if err != nil {
        panic(err)
    }
    
    // Upload file
    data := []byte("Hello, NoiseFS!")
    descriptor, err := client.Upload(data)
    if err != nil {
        panic(err)
    }
    
    // Download file
    recovered, err := client.Download(descriptor)
    if err != nil {
        panic(err)
    }
    
    // Data should match
    println(string(recovered)) // "Hello, NoiseFS!"
}
```

### Advanced Usage

#### Custom Storage Backend

```go
config := storage.DefaultConfig()
config.Backends["s3"] = &storage.BackendConfig{
    Type:    storage.BackendTypeS3,
    Enabled: true,
    Connection: &storage.ConnectionConfig{
        Endpoint: "s3.amazonaws.com",
        Region:   "us-east-1",
    },
}
```

#### Block Management

```go
import "github.com/TheEntropyCollective/noisefs/pkg/core/blocks"

// Create block with custom size
block, err := blocks.NewBlockWithSize(data, 64*1024) // 64KB

// Anonymize block
randomizers := []*blocks.Block{rand1, rand2}
anonBlock := blocks.AnonymizeBlock(block, randomizers)

// Recover block
recovered := blocks.RecoverBlock(anonBlock, randomizers)
```

## Error Handling

### Common Errors

- `IPFS connection failed`: IPFS daemon not running
- `Block not found`: Descriptor CID invalid or content not available
- `Permission denied`: Insufficient permissions for FUSE mount
- `Invalid descriptor`: Malformed or corrupted descriptor

### Error Codes

- `ERR_IPFS_UNAVAILABLE`: IPFS daemon unreachable
- `ERR_BLOCK_NOT_FOUND`: Required block missing from network
- `ERR_INVALID_DESCRIPTOR`: Descriptor format invalid
- `ERR_MOUNT_FAILED`: FUSE mount operation failed

## Performance Tips

1. **Use larger files**: Fixed 128 KiB blocks work better with larger files
2. **Enable caching**: Set appropriate cache size for your workload
3. **Local IPFS**: Run IPFS daemon locally for best performance
4. **Batch operations**: Upload multiple files in one session when possible

## Security Considerations

1. **Descriptor security**: Treat descriptors like sensitive keys
2. **Local storage**: Ensure local IPFS repo is encrypted
3. **Network privacy**: Consider using Tor for IPFS connections
4. **Metadata leakage**: File sizes and access patterns may leak information
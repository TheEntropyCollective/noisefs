# Storage Package

## Overview

The storage package provides a unified interface for block storage backends, supporting multiple storage systems including IPFS, local storage, and cloud providers.

## Core Concepts

- **Backend Abstraction**: Common interface for diverse storage systems
- **Router**: Intelligent routing between storage backends
- **Health Monitoring**: Track backend availability and performance
- **Compatibility Layer**: Adapt various storage APIs to NoiseFS needs

## Implementation Details

### Key Components

- **Interface**: Common storage operations API
- **Manager**: Lifecycle and configuration management
- **Router**: Request routing and failover
- **Registry**: Backend registration and discovery

### Storage Interface

```go
type Storage interface {
    Store(ctx context.Context, data []byte) (string, error)
    Retrieve(ctx context.Context, id string) ([]byte, error)
    Delete(ctx context.Context, id string) error
    Has(ctx context.Context, id string) (bool, error)
}
```

### Backend Types

1. **IPFS Backend** (`backends/ipfs.go`)
   - Primary distributed storage
   - Content addressing
   - DHT integration

2. **Local Backend**
   - Development and testing
   - Fast local cache
   - Offline operation

3. **Cloud Backends** (future)
   - S3-compatible storage
   - Backup and archival

## Performance Features

- Parallel operations across backends
- Automatic retry with backoff
- Connection pooling
- Request coalescing

## Health and Monitoring

- Backend health checks
- Performance metrics
- Automatic failover
- Storage usage tracking

## Integration Points

- Used by [Blocks](../core/blocks/CLAUDE.md) for data storage
- Managed by [Cache](cache/CLAUDE.md) for optimization
- Configured via [Infrastructure](../infrastructure/CLAUDE.md)

## References

See [Global CLAUDE.md](/CLAUDE.md) for system-wide principles.
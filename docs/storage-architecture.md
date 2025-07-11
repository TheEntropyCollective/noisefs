# NoiseFS Storage Architecture

## Overview

The Storage Architecture in NoiseFS provides a flexible, high-performance abstraction layer that seamlessly integrates with IPFS while maintaining the privacy guarantees of the OFFSystem. This architecture enables distributed, content-addressed storage with strong anonymity properties.

## Core Design Principles

### Abstraction Layer Philosophy

NoiseFS implements a storage abstraction layer that:

1. **Backend Agnostic**: Supports multiple storage backends (primarily IPFS)
2. **Privacy First**: All operations maintain anonymity guarantees
3. **Performance Optimized**: Intelligent routing and load balancing
4. **Fault Tolerant**: Retry mechanisms and error aggregation

## Current Implementation

### Storage Manager

The core storage manager (`pkg/storage/manager.go`) provides:

```go
type Manager struct {
    backends   map[string]Backend
    config     *Config
    router     *Router
    cache      Cache
    metadata   MetadataStore
}
```

Key features:
- Multi-backend support with dynamic registration
- Health monitoring and automatic failover
- Batch operations for efficiency
- Integrated caching layer

### Storage Router

The router (`pkg/storage/router.go`) handles intelligent backend selection:

```go
type Router struct {
    manager      *Manager
    config       *DistributionConfig
    strategies   map[string]DistributionStrategy
    loadBalancer *LoadBalancer
}
```

Supported distribution strategies:
- **SingleBackendStrategy**: Simple single backend storage
- **ReplicationStrategy**: Multi-backend replication
- **SmartDistributionStrategy**: Automatic strategy selection

### Load Balancing

The load balancer provides multiple algorithms:

```go
type LoadBalancer struct {
    config    *LoadBalancingConfig
    metrics   map[string]*BackendMetrics
}
```

Algorithms:
- Round-robin
- Weighted (based on performance)
- Least connections
- Performance-based (default)

## IPFS Integration

### IPFS Client

The IPFS client (`pkg/storage/ipfs/client.go`) provides:

```go
type Client struct {
    shell          *shell.Shell
    peerManager    *p2p.PeerManager
    requestMetrics map[peer.ID]*RequestMetrics
}
```

Key capabilities:
- Peer-aware operations for optimized retrieval
- Performance metrics tracking
- Parallel retrieval from multiple peers
- Broadcast support for redundancy

### Peer Selection Integration

When a peer manager is available, the IPFS client uses intelligent peer selection:

```go
func (c *Client) RetrieveBlockWithPeerHint(cid string, preferredPeers []peer.ID) (*blocks.Block, error) {
    // Try preferred peers first
    // Use peer selection strategies
    // Fall back to standard IPFS retrieval
}
```

### Content Addressing

All blocks are content-addressed using SHA-256:
- CIDs are deterministic based on content
- Integrity verification is automatic
- Deduplication happens at the network level

## Backend Interface

All storage backends implement a common interface:

```go
type Backend interface {
    // Core operations
    Put(ctx context.Context, block *blocks.Block) (*BlockAddress, error)
    Get(ctx context.Context, address *BlockAddress) (*blocks.Block, error)
    Has(ctx context.Context, address *BlockAddress) (bool, error)
    Delete(ctx context.Context, address *BlockAddress) error
    
    // Batch operations
    PutMany(ctx context.Context, blocks []*blocks.Block) ([]*BlockAddress, error)
    GetMany(ctx context.Context, addresses []*BlockAddress) ([]*blocks.Block, error)
    
    // Management operations
    Pin(ctx context.Context, address *BlockAddress) error
    Unpin(ctx context.Context, address *BlockAddress) error
    
    // Health and info
    HealthCheck(ctx context.Context) *HealthStatus
    GetBackendInfo() *BackendInfo
    IsConnected() bool
}
```

### IPFS Backend Implementation

The IPFS backend (`pkg/storage/backends/ipfs.go`) provides:

```go
type IPFSBackend struct {
    name     string
    client   *ipfs.Client
    info     *BackendInfo
}
```

Features:
- Direct IPFS API integration
- Content-addressed storage
- Pinning support for persistence
- Health monitoring

## Performance Optimizations

### Parallel Operations

The router supports efficient batch operations:

```go
func (r *Router) GetMany(ctx context.Context, addresses []*BlockAddress) ([]*blocks.Block, error) {
    // Groups addresses by backend
    // Retrieves in parallel
    // Handles failures gracefully
}
```

### Retry Mechanisms

Failed operations are retried with appropriate fallbacks:

```go
func (r *Router) Get(ctx context.Context, address *BlockAddress) (*blocks.Block, error) {
    // Try specified backend first
    // Fall back to other available backends
    // Return aggregated errors if all fail
}
```

### Performance Metrics

The load balancer tracks backend performance:

```go
type BackendMetrics struct {
    RequestCount   int64
    SuccessRate    float64
    AverageLatency time.Duration
    LastUsed       time.Time
}
```

## Error Handling

### Error Aggregation

Multiple errors are collected and reported:

```go
type ErrorAggregator struct {
    errors []error
}

func (ea *ErrorAggregator) CreateAggregateError() error {
    // Returns a combined error message
}
```

### Not Found Errors

Special handling for missing blocks:

```go
type NotFoundError struct {
    Backend string
    Address *BlockAddress
}
```

## Caching Integration

The storage manager integrates with the cache system:
- Frequently accessed blocks are cached locally
- Cache checks happen before backend retrieval
- Write-through caching for new blocks

## Security Considerations

1. **Content Verification**: All blocks are verified by content hash
2. **No Semantic Leakage**: CIDs reveal nothing about content
3. **Anonymized Storage**: All stored blocks are XOR'd
4. **Distribution Privacy**: Routing decisions don't leak information

## Configuration

### Storage Configuration

```go
type Config struct {
    DefaultBackend string
    Backends       map[string]BackendConfig
    Distribution   *DistributionConfig
    Cache          *CacheConfig
}
```

### Distribution Configuration

```go
type DistributionConfig struct {
    Strategy      string            // "single", "replicate", "smart"
    Replication   *ReplicationConfig
    LoadBalancing *LoadBalancingConfig
}
```

## Future Enhancements

The following features are planned but not yet implemented:

1. **IPFS Cluster Integration**: Coordinated pinning across nodes
2. **Advanced Metrics**: ML-based predictive prefetching
3. **Circuit Breaker Pattern**: Prevent cascading failures
4. **Storage Proofs**: Cryptographic proof of storage
5. **GraphSync Integration**: Efficient block synchronization

## Testing

The storage system includes comprehensive tests:
- Unit tests for all components
- Integration tests with mock backends
- Performance benchmarks
- Failure scenario testing

## Conclusion

The Storage Architecture provides a robust foundation for NoiseFS's distributed storage needs. By abstracting storage operations and providing intelligent routing, it achieves excellent performance while maintaining strong privacy guarantees. The IPFS integration leverages the benefits of content-addressed storage while adding NoiseFS-specific optimizations for anonymity and performance.
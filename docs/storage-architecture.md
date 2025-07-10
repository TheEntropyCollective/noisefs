# NoiseFS Storage Architecture

## Overview

The Storage Architecture in NoiseFS provides a flexible, high-performance abstraction layer that seamlessly integrates with IPFS while maintaining the privacy guarantees of the OFFSystem. This architecture enables distributed, content-addressed storage with strong anonymity properties.

## Core Design Principles

### Abstraction Layer Philosophy

NoiseFS implements a storage abstraction layer that:

1. **Backend Agnostic**: Supports multiple storage backends (IPFS, S3, local)
2. **Privacy First**: All operations maintain anonymity guarantees
3. **Performance Optimized**: Intelligent routing and caching
4. **Fault Tolerant**: Automatic failover and retry mechanisms

### IPFS Integration Architecture

```go
type StorageManager struct {
    router      *StorageRouter
    backends    map[string]StorageBackend
    cache       *AdaptiveCache
    metrics     *StorageMetrics
    healthCheck *HealthMonitor
}
```

## IPFS as Primary Backend

### Why IPFS?

IPFS provides ideal properties for NoiseFS:

1. **Content Addressing**: Deduplication happens automatically
2. **Distributed Network**: No single point of failure
3. **Peer-to-Peer**: Direct block retrieval without intermediaries
4. **Existing Infrastructure**: Leverages mature P2P network

### Integration Design Decisions

#### Direct API Integration

NoiseFS uses the IPFS HTTP API for maximum compatibility:

```go
type IPFSBackend struct {
    client      *ipfs.Shell
    peerManager *PeerManager
    config      *IPFSConfig
    metrics     *IPFSMetrics
}

func (b *IPFSBackend) StoreBlock(ctx context.Context, block *Block) (string, error) {
    // Add to IPFS with optimal parameters
    cid, err := b.client.Add(
        bytes.NewReader(block.Data),
        ipfs.Pin(true),                    // Pin important blocks
        ipfs.OnlyHash(false),             // Store, don't just hash
        ipfs.RawLeaves(true),             // Store as raw blocks
        ipfs.Chunker("size-131072"),      // 128KB chunks
    )
    
    // Track metrics
    b.metrics.RecordStore(len(block.Data), err == nil)
    
    return cid, err
}
```

#### Peer-Aware Operations

NoiseFS extends IPFS with peer-aware operations for performance:

```go
func (b *IPFSBackend) RetrieveBlockWithPeerHint(
    ctx context.Context, 
    cid string, 
    preferredPeers []peer.ID,
) (*Block, error) {
    // Try preferred peers first
    for _, peerID := range preferredPeers {
        if block, err := b.retrieveFromPeer(ctx, cid, peerID); err == nil {
            return block, nil
        }
    }
    
    // Fall back to DHT lookup
    return b.retrieveFromDHT(ctx, cid)
}
```

### Content Addressing Implications

#### CID Management

Content IDs (CIDs) in IPFS provide:

1. **Integrity Verification**: Content is self-verifying
2. **Global Deduplication**: Identical blocks share CIDs
3. **Immutability**: Content cannot be modified without changing CID

#### Privacy Considerations

While CIDs are public, NoiseFS maintains privacy through:

1. **Anonymized Content**: CIDs point to XOR'd blocks
2. **No Semantic Information**: CIDs reveal nothing about content
3. **Unlinkability**: CIDs cannot be connected to original files

## Storage Router Architecture

### Dynamic Backend Selection

The Storage Router intelligently selects backends based on:

```go
type RoutingDecision struct {
    Primary    StorageBackend
    Fallbacks  []StorageBackend
    Strategy   RoutingStrategy
    Timeout    time.Duration
}

func (r *StorageRouter) Route(operation StorageOp) *RoutingDecision {
    switch operation.Type {
    case OpTypeStore:
        return r.routeStore(operation)
    case OpTypeRetrieve:
        return r.routeRetrieve(operation)
    case OpTypeDelete:
        return r.routeDelete(operation)
    }
}
```

### Routing Strategies

1. **Performance First**: Route to fastest available backend
2. **Reliability First**: Route to most reliable backend
3. **Cost Optimized**: Minimize bandwidth/storage costs
4. **Privacy Enhanced**: Route through privacy-preserving paths

## Multi-Backend Support

### Backend Interface

All storage backends implement a common interface:

```go
type StorageBackend interface {
    // Basic Operations
    Store(ctx context.Context, block *Block) (string, error)
    Retrieve(ctx context.Context, id string) (*Block, error)
    Delete(ctx context.Context, id string) error
    
    // Advanced Operations
    BatchStore(ctx context.Context, blocks []*Block) ([]string, error)
    GetMetadata(ctx context.Context, id string) (*BlockMetadata, error)
    
    // Health & Metrics
    HealthCheck() error
    GetMetrics() *BackendMetrics
}
```

### Supported Backends

#### IPFS Backend (Primary)
- **Strengths**: Decentralized, content-addressed, peer-to-peer
- **Use Cases**: General storage, public datasets
- **Configuration**: Peer selection, pinning strategies

#### S3-Compatible Backend
- **Strengths**: High availability, scalability
- **Use Cases**: Enterprise deployments, backups
- **Configuration**: Bucket policies, encryption settings

#### Local Filesystem Backend
- **Strengths**: Low latency, full control
- **Use Cases**: Development, edge caching
- **Configuration**: Directory structure, permissions

### Backend Selection Logic

```go
func (r *StorageRouter) selectBackend(criteria SelectionCriteria) StorageBackend {
    scores := make(map[string]float64)
    
    for name, backend := range r.backends {
        score := 0.0
        metrics := backend.GetMetrics()
        
        // Latency score (lower is better)
        score += (1.0 / metrics.AvgLatency.Seconds()) * criteria.LatencyWeight
        
        // Reliability score
        score += metrics.SuccessRate * criteria.ReliabilityWeight
        
        // Cost score
        score += (1.0 / metrics.CostPerGB) * criteria.CostWeight
        
        // Availability score
        if backend.HealthCheck() == nil {
            score += criteria.AvailabilityWeight
        }
        
        scores[name] = score
    }
    
    return r.backends[r.highestScore(scores)]
}
```

## Performance Optimizations

### Parallel Operations

NoiseFS implements aggressive parallelization:

```go
func (s *StorageManager) ParallelRetrieve(
    ctx context.Context, 
    cids []string,
) ([]*Block, error) {
    results := make(chan *blockResult, len(cids))
    
    for _, cid := range cids {
        go func(id string) {
            block, err := s.Retrieve(ctx, id)
            results <- &blockResult{block: block, err: err}
        }(cid)
    }
    
    // Collect results
    blocks := make([]*Block, 0, len(cids))
    for i := 0; i < len(cids); i++ {
        result := <-results
        if result.err != nil {
            return nil, result.err
        }
        blocks = append(blocks, result.block)
    }
    
    return blocks, nil
}
```

### Connection Pooling

Efficient connection management for IPFS:

```go
type ConnectionPool struct {
    connections sync.Map
    maxPerPeer  int
    idleTimeout time.Duration
}

func (p *ConnectionPool) GetConnection(peerID peer.ID) (net.Conn, error) {
    // Reuse existing connections
    if conn, ok := p.connections.Load(peerID); ok {
        return conn.(net.Conn), nil
    }
    
    // Create new connection
    conn, err := p.dial(peerID)
    if err != nil {
        return nil, err
    }
    
    p.connections.Store(peerID, conn)
    return conn, nil
}
```

### Predictive Prefetching

ML-based prefetching for improved performance:

```go
func (s *StorageManager) PrefetchPredicted(ctx context.Context) {
    predictions := s.cache.GetAccessPredictor().PredictNext(10)
    
    for _, prediction := range predictions {
        if prediction.Confidence > 0.8 {
            go s.prefetchBlock(ctx, prediction.CID)
        }
    }
}
```

## IPFS-Specific Optimizations

### DHT Optimization

Custom DHT parameters for NoiseFS use case:

```go
dhtConfig := dht.Config{
    Mode:                 dht.ModeServer,
    BucketSize:          20,
    Concurrency:         10,
    MaxRecordAge:        24 * time.Hour,
    EnableProviderStore: true,
    EnableValueStore:    false, // Only use for routing
}
```

### Pinning Strategy

Intelligent pinning for block persistence:

```go
type PinningStrategy struct {
    PopularityThreshold float64
    RetentionPeriod     time.Duration
    MaxPinnedBlocks     int
}

func (s *PinningStrategy) ShouldPin(block *Block, metrics *BlockMetrics) bool {
    // Pin if block is popular
    if metrics.AccessCount > s.PopularityThreshold {
        return true
    }
    
    // Pin if block is a critical randomizer
    if block.IsRandomizer && metrics.ReuseCount > 10 {
        return true
    }
    
    return false
}
```

### Swarm Optimization

Optimize IPFS swarm connections:

```go
swarmConfig := config.SwarmConfig{
    ConnMgr: config.ConnMgr{
        Type:        "basic",
        LowWater:    100,
        HighWater:   400,
        GracePeriod: time.Minute,
    },
    EnableRelayHop:  false, // Disable relay for privacy
    EnableAutoRelay: false,
}
```

## Fault Tolerance

### Retry Mechanisms

Exponential backoff with jitter:

```go
func (s *StorageManager) RetryWithBackoff(
    operation func() error,
    maxAttempts int,
) error {
    backoff := 100 * time.Millisecond
    
    for attempt := 0; attempt < maxAttempts; attempt++ {
        err := operation()
        if err == nil {
            return nil
        }
        
        if attempt < maxAttempts-1 {
            jitter := time.Duration(rand.Float64() * float64(backoff))
            time.Sleep(backoff + jitter)
            backoff *= 2
        }
    }
    
    return fmt.Errorf("operation failed after %d attempts", maxAttempts)
}
```

### Circuit Breaker Pattern

Prevent cascading failures:

```go
type CircuitBreaker struct {
    failures      int
    lastFailure   time.Time
    state         State
    threshold     int
    timeout       time.Duration
}

func (cb *CircuitBreaker) Call(fn func() error) error {
    if cb.state == StateOpen {
        if time.Since(cb.lastFailure) > cb.timeout {
            cb.state = StateHalfOpen
        } else {
            return ErrCircuitOpen
        }
    }
    
    err := fn()
    if err != nil {
        cb.recordFailure()
    } else {
        cb.reset()
    }
    
    return err
}
```

## Performance Metrics

### Key Metrics Tracked

1. **Storage Latency**: Time to store blocks
2. **Retrieval Latency**: Time to retrieve blocks
3. **Success Rate**: Percentage of successful operations
4. **Throughput**: MB/s for storage and retrieval
5. **Network Efficiency**: Bytes transferred vs stored

### Monitoring Implementation

```go
type StorageMetrics struct {
    StoreLatency    *LatencyTracker
    RetrieveLatency *LatencyTracker
    SuccessRate     *RateTracker
    Throughput      *ThroughputTracker
    NetworkOverhead *OverheadTracker
}

func (m *StorageMetrics) RecordOperation(
    op OperationType,
    size int64,
    duration time.Duration,
    success bool,
) {
    switch op {
    case OpStore:
        m.StoreLatency.Record(duration)
        m.Throughput.RecordStore(size, duration)
    case OpRetrieve:
        m.RetrieveLatency.Record(duration)
        m.Throughput.RecordRetrieve(size, duration)
    }
    
    m.SuccessRate.Record(success)
}
```

## Future Enhancements

### Planned Improvements

1. **IPFS Cluster Integration**: Coordinated pinning across nodes
2. **Bitswap Optimization**: Custom Bitswap strategies
3. **GraphSync Integration**: Efficient block synchronization
4. **Storage Proofs**: Cryptographic proof of storage

### Research Directions

1. **Decentralized Indexing**: Privacy-preserving block discovery
2. **Incentive Mechanisms**: Token economics for storage
3. **Cross-Chain Integration**: Ethereum/Filecoin anchoring
4. **Zero-Knowledge Storage**: Prove storage without revealing content

## Conclusion

The Storage Architecture provides a robust, flexible foundation for NoiseFS's distributed storage needs. By leveraging IPFS's content-addressed network while adding privacy-aware optimizations, it achieves excellent performance without compromising anonymity. The multi-backend support ensures deployability across diverse environments while maintaining consistent behavior and strong privacy guarantees.
# NoiseFS Development Todo

## Current Milestone: Milestone 4 - Scalability & Performance

### Overview: Intelligent Peer Selection Algorithms

This milestone focuses on implementing sophisticated peer selection algorithms that optimize NoiseFS performance while maintaining privacy guarantees. The implementation will address four key areas:

1. **Randomizer-Aware Peer Selection**: Peers are selected based on their availability of popular randomizer blocks, maximizing block reuse and reducing storage overhead.

2. **Performance-Based Scoring**: Real-time tracking of peer latency, bandwidth, and reliability to optimize block retrieval speed.

3. **Privacy-Preserving Load Distribution**: Request routing and timing randomization to prevent traffic analysis while maintaining plausible deniability.

4. **Adaptive Caching Strategy**: Machine learning-based cache management that predicts block access patterns and coordinates caching across peers.

### Key Data Structures

```go
// PeerInfo tracks comprehensive peer metadata
type PeerInfo struct {
    ID              peer.ID
    LastSeen        time.Time
    Latency         time.Duration
    Bandwidth       float64 // MB/s
    SuccessRate     float64 // 0.0-1.0
    BlockInventory  *BloomFilter
    RandomizerScore float64
    Reputation      float64
}

// BlockAvailability tracks which peers have which blocks
type BlockAvailability struct {
    BlockCID    string
    Peers       []peer.ID
    Popularity  int64
    LastAccess  time.Time
}

// PeerSelectionStrategy defines selection algorithms
type PeerSelectionStrategy interface {
    SelectPeers(blockCID string, count int) []peer.ID
    UpdateMetrics(peer peer.ID, success bool, latency time.Duration)
}
```

### Integration Points

1. **IPFS Client** (`pkg/ipfs/client.go`): Add peer selection hooks to RetrieveBlock
2. **NoiseFS Client** (`pkg/noisefs/client.go`): Integrate peer metrics into randomizer selection
3. **Cache Manager** (`pkg/cache/cache.go`): Coordinate cache state across peers
4. **Descriptor Store** (`pkg/descriptors/store.go`): Distribute descriptors across selected peers

### Core Algorithms

#### 1. Randomizer-Aware Selection Algorithm
```
function selectPeersForRandomizer(blockSize, count):
    candidates = getAllHealthyPeers()
    for each peer in candidates:
        peer.score = calculateRandomizerScore(peer, blockSize)
    
    sortByScore(candidates)
    return topK(candidates, count)

function calculateRandomizerScore(peer, blockSize):
    inventoryMatch = peer.blockInventory.estimateMatches(blockSize)
    popularityScore = peer.getAverageBlockPopularity()
    diversityScore = peer.getBlockDiversity()
    
    return inventoryMatch * 0.5 + popularityScore * 0.3 + diversityScore * 0.2
```

#### 2. Performance-Based Selection Algorithm
```
function selectPeersByPerformance(requiredBandwidth, maxLatency):
    candidates = getAllPeers()
    filtered = []
    
    for each peer in candidates:
        if peer.bandwidth >= requiredBandwidth AND peer.latency <= maxLatency:
            peer.performanceScore = calculatePerformanceScore(peer)
            filtered.append(peer)
    
    return sortByPerformanceScore(filtered)

function calculatePerformanceScore(peer):
    latencyScore = 1.0 / (1.0 + peer.latency.Seconds())
    bandwidthScore = min(peer.bandwidth / 10.0, 1.0) // Normalize to 10MB/s
    reliabilityScore = peer.successRate
    
    return latencyScore * 0.4 + bandwidthScore * 0.3 + reliabilityScore * 0.3
```

#### 3. Privacy Mixer Algorithm
```
function routeRequestWithPrivacy(request, targetPeer):
    if shouldUseDirectRoute():
        return sendDirect(request, targetPeer)
    
    numHops = randomInt(1, 3)
    intermediatePeers = selectRandomPeers(numHops)
    
    // Add temporal delay
    delay = randomDuration(0, 500ms)
    sleep(delay)
    
    // Route through intermediates
    return routeThroughPeers(request, intermediatePeers, targetPeer)
```

#### 4. Adaptive Cache Prediction Algorithm
```
function predictBlockAccess(blockCID, accessHistory):
    features = extractFeatures(blockCID, accessHistory)
    
    // Simple ML model using exponential weighted moving average
    recentAccesses = getRecentAccesses(blockCID, 24h)
    trend = calculateAccessTrend(recentAccesses)
    
    predictedAccess = baselineAccess * trend * seasonalityFactor(time.Now())
    
    return predictedAccess

function shouldCache(block, predictedAccess, cacheSpace):
    benefit = predictedAccess * block.popularity * block.reuseCount
    cost = block.size / cacheSpace.available
    
    return benefit/cost > CACHE_THRESHOLD
```

### 🎯 Sprint 1: Intelligent Peer Selection Implementation

**Goal**: Implement sophisticated peer selection algorithms that optimize for randomizer block reuse, performance, privacy, and caching efficiency.

#### Task 1: Core Peer Selection Infrastructure
- [ ] Create `pkg/p2p/peer_manager.go` with PeerManager interface and implementation
- [ ] Implement peer metadata tracking (latency, bandwidth, block availability, reputation)
- [ ] Create peer discovery mechanisms integrated with IPFS DHT
- [ ] Build peer connection pool with configurable limits
- [ ] Add peer health monitoring and automatic pruning of unresponsive peers

#### Task 2: Randomizer-Aware Peer Selection
- [ ] Create `pkg/p2p/randomizer_index.go` for tracking randomizer block distribution
- [ ] Implement bloom filter-based block availability announcements
- [ ] Build randomizer popularity tracking across peer network
- [ ] Create peer ranking algorithm based on randomizer availability score
- [ ] Implement preferential peer selection for nodes with high randomizer overlap

#### Task 3: Performance-Based Peer Scoring
- [ ] Create `pkg/p2p/peer_metrics.go` for real-time performance tracking
- [ ] Implement latency measurement using periodic ping/pong
- [ ] Build bandwidth estimation through transfer sampling
- [ ] Create composite performance score (latency * 0.4 + bandwidth * 0.3 + success_rate * 0.3)
- [ ] Implement adaptive timeout adjustments based on peer performance history

#### Task 4: Privacy-Preserving Load Distribution
- [ ] Create `pkg/p2p/privacy_mixer.go` for anonymizing peer requests
- [ ] Implement request routing through random intermediate peers
- [ ] Build query batching to obscure individual file access patterns
- [ ] Create decoy traffic generation for plausible deniability
- [ ] Implement temporal randomization for request timing

### 🎯 Sprint 2: Adaptive Caching & Integration

**Goal**: Implement intelligent caching strategies and integrate peer selection with NoiseFS core.

#### Task 5: Adaptive Caching Strategy
- [ ] Create `pkg/cache/adaptive_cache.go` with ML-based eviction policies
- [ ] Implement cache preloading based on access patterns and peer availability
- [ ] Build collaborative caching protocol for peer cache coordination
- [ ] Create cache exchange protocol for popular randomizer blocks
- [ ] Implement tiered caching with hot/warm/cold block segregation

#### Task 6: IPFS Client Enhancement
- [ ] Modify `pkg/ipfs/client.go` to use PeerManager for block retrieval
- [ ] Implement parallel block fetching from multiple peers
- [ ] Add peer selection hooks for StoreBlock and RetrieveBlock operations
- [ ] Create fallback mechanisms for peer failures
- [ ] Implement request routing through selected peers

#### Task 7: NoiseFS Client Integration
- [ ] Update `pkg/noisefs/client.go` to leverage peer selection for randomizer selection
- [ ] Integrate peer metrics into SelectRandomizer and SelectTwoRandomizers
- [ ] Add peer-aware block retrieval with automatic failover
- [ ] Implement distributed randomizer discovery across peer network
- [ ] Create peer coordination for 3-tuple block assembly

#### Task 8: Testing & Benchmarking
- [ ] Create comprehensive unit tests for peer selection algorithms
- [ ] Build integration tests simulating various network conditions
- [ ] Implement benchmarks comparing peer selection strategies
- [ ] Create chaos testing framework for peer failures
- [ ] Document performance improvements and trade-offs

## Completed Major Milestones

### ✅ Milestone 1 - Core Implementation
- Core OFFSystem architecture with 3-tuple anonymization
- IPFS integration and distributed storage
- FUSE filesystem with complete POSIX operations
- Block splitting, assembly, and caching systems
- Web UI and CLI interfaces

### ✅ Milestone 2 - Performance & Production
- Configuration management system
- Structured logging with metrics
- Performance benchmarking suite
- Advanced caching optimizations
- Docker containerization and Kubernetes deployment
- Project organization and build system

### ✅ Milestone 3 - Security & Privacy Analysis
- Production-grade security with HTTPS/TLS enforcement
- AES-256-GCM encryption for file descriptors and local indexes
- Streaming media support with progressive download
- Comprehensive input validation and rate limiting
- Anti-forensics features and secure memory handling
- Security management tooling and CLI utilities
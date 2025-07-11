# NoiseFS Privacy Infrastructure

## Overview

The Privacy Infrastructure in NoiseFS implements multiple layers of anonymity protection, from relay pools for distributed request routing to request mixing with cover traffic. This system ensures that users and network participants maintain plausible deniability while accessing and storing data.

## Current Implementation

### Core Privacy Architecture

The privacy infrastructure consists of several key components:

1. **Relay Pool**: Manages a distributed network of relay nodes
2. **Request Mixer**: Combines real requests with cover traffic
3. **Cover Traffic Generator**: Creates decoy requests
4. **Request Distributor**: Routes requests through relays
5. **Peer Selection**: Intelligent peer selection strategies

## Relay Pool Architecture

### Relay Pool Management

The relay pool (`pkg/privacy/relay/pool.go`) manages relay nodes:

```go
type RelayPool struct {
    relays        map[peer.ID]*RelayNode
    config        *PoolConfig
    selector      RelaySelector
    healthMonitor *RelayHealthMonitor
    metrics       *PoolMetrics
}

type RelayNode struct {
    ID          peer.ID
    Addresses   []string
    Capabilities []Capability
    Health      *HealthStatus
    Performance *PerformanceMetrics
    LastUsed    time.Time
    CreatedAt   time.Time
}
```

### Health Monitoring

Each relay node's health is continuously monitored:

```go
type HealthStatus struct {
    IsHealthy    bool
    LastCheck    time.Time
    FailureCount int
    Latency      time.Duration
    Bandwidth    float64 // MB/s
    Reliability  float64 // 0-1 success rate
}
```

### Relay Selection Strategies

Multiple selection strategies are supported:

- **Round Robin**: Distributes load evenly
- **Least Loaded**: Selects relays with lowest current load
- **Random**: Random selection for unpredictability

```go
type RelaySelector interface {
    SelectRelays(ctx context.Context, pool *RelayPool, count int) ([]*RelayNode, error)
}
```

## Request Mixing

### Request Mixer

The request mixer (`pkg/privacy/relay/mixer.go`) combines real and cover traffic:

```go
type RequestMixer struct {
    config             *MixerConfig
    coverGenerator     *CoverTrafficGenerator
    popularityTracker  *PopularBlockTracker
    distributor        *RequestDistributor
    activeMixes        map[string]*MixedRequest
    mixingPools        map[string]*MixingPool
}
```

### Mixing Process

1. **Request Batching**: Real requests are accumulated in mixing pools
2. **Cover Generation**: Cover traffic is generated based on configured ratio
3. **Temporal Jitter**: Random delays are added to obscure timing
4. **Distribution**: Mixed requests are distributed across multiple relays

```go
type MixerConfig struct {
    MixingDelay        time.Duration // Maximum delay for mixing
    MinMixSize         int           // Minimum requests per mix
    MaxMixSize         int           // Maximum requests per mix
    CoverRatio         float64       // Cover to real request ratio
    RelayDistribution  float64       // Distribution across relays
    TemporalJitter     time.Duration // Random time jitter
}
```

### Mixing Pool Management

Requests are grouped into pools for batch mixing:

```go
type MixingPool struct {
    ID           string
    Requests     []*PendingRequest    // Real requests
    CoverRequests []*CoverRequest     // Cover traffic
    Created      time.Time
    Deadline     time.Time
    RelayTargets map[peer.ID]int
    Sealed       bool
}
```

## Cover Traffic Generation

### Cover Traffic Generator

Creates realistic decoy requests (`pkg/privacy/relay/cover_traffic.go`):

```go
type CoverTrafficGenerator struct {
    popularityTracker *PopularBlockTracker
    cache             *CoverCache
    config            *CoverConfig
}

type CoverConfig struct {
    MinCoverRequests   int
    MaxCoverRequests   int
    CacheSize          int
    PopularBlockRatio  float64
    RandomBlockRatio   float64
}
```

### Cover Request Creation

Cover requests mimic real request patterns:

```go
func (g *CoverTrafficGenerator) GenerateCoverTraffic(
    ctx context.Context, 
    realBlockIDs []string,
) (*CoverBatch, error) {
    // Select blocks based on popularity
    popularBlocks := g.popularityTracker.GetPopularBlocks(count)
    
    // Mix with random blocks
    randomBlocks := g.generateRandomBlockIDs(count)
    
    // Create cover requests that look like real requests
    return g.createCoverBatch(popularBlocks, randomBlocks)
}
```

## Request Distribution

### Request Distributor

Routes requests through relay network (`pkg/privacy/relay/distributor.go`):

```go
type RequestDistributor struct {
    pool      *RelayPool
    connector *RelayConnector
    config    *DistributorConfig
}

func (d *RequestDistributor) DistributeRequest(
    ctx context.Context,
    requestID string,
    blockIDs []string,
) (*DistributedRequest, error) {
    // Select relays for this request
    relays, err := d.pool.SelectRelays(ctx, d.config.RelayCount)
    
    // Connect and send through relays
    for _, relay := range relays {
        go d.sendThroughRelay(ctx, relay, requestID, blockIDs)
    }
}
```

## Peer Selection Strategies

### Peer Manager Integration

The system integrates with peer selection for optimal routing:

```go
type PeerManager struct {
    peers      map[peer.ID]*PeerInfo
    strategies map[string]SelectionStrategy
}
```

Available strategies:
- **Performance**: Select fastest peers
- **Randomizer**: Prefer peers with popular randomizers
- **Privacy**: Maximum anonymity through diverse selection
- **Hybrid**: Balanced approach

## Privacy Features

### Request Anonymization

1. **Batch Mixing**: Requests are never sent individually
2. **Cover Traffic**: Real requests hidden among decoys
3. **Temporal Obfuscation**: Random delays prevent timing analysis
4. **Multi-Relay Routing**: Requests distributed across relays

### Connection Privacy

1. **Encrypted Connections**: All relay connections use TLS
2. **No Direct Connections**: Client never connects directly to storage
3. **Relay Rotation**: Regular relay changes prevent long-term monitoring

### Metadata Protection

1. **No Request Correlation**: Mixed requests prevent correlation
2. **Uniform Request Size**: Padding ensures consistent sizes
3. **Random Request Patterns**: Cover traffic obscures real patterns

## Performance Metrics

The system tracks privacy-related metrics:

```go
type PrivacyMetrics struct {
    // Mixing effectiveness
    AverageMixSize     float64
    CoverRatioAchieved float64
    
    // Relay performance
    RelayResponseTime  time.Duration
    RelaySuccessRate   float64
    
    // Privacy measures
    AnonymitySet       int
    MixingDelay        time.Duration
}
```

## Configuration

### Privacy Levels

Different privacy levels for different use cases:

```go
const (
    PrivacyLevelLow    = 1 // Minimal mixing, fast
    PrivacyLevelMedium = 2 // Moderate mixing, balanced
    PrivacyLevelHigh   = 3 // Maximum mixing, slower
)
```

### Tunable Parameters

- Mix size: 3-20 requests per batch
- Cover ratio: 0.5-2.0 (cover/real)
- Mixing delay: 100ms-5s
- Relay count: 1-5 per request

## Limitations and Trade-offs

1. **Performance Impact**: Mixing and relaying add latency
2. **Bandwidth Overhead**: Cover traffic increases bandwidth usage
3. **Complexity**: Multiple components must coordinate
4. **Relay Dependency**: Requires healthy relay network

## Future Enhancements

The following features are planned but not yet implemented:

### Onion Routing
- Multi-hop encrypted routing
- Circuit establishment
- Stream multiplexing

### Advanced Mixing
- Cascade mixing across multiple nodes
- Format-preserving encryption
- Dummy message injection

### Traffic Analysis Resistance
- Constant-rate traffic shaping
- Packet padding and splitting
- Statistical uniformity enforcement

### Decentralized Relay Network
- Incentivized relay operation
- Reputation system
- Automated relay discovery

## Security Considerations

1. **Relay Trust**: No single relay sees full request
2. **Timing Attacks**: Mitigated by temporal jitter
3. **Traffic Analysis**: Cover traffic obscures patterns
4. **Sybil Resistance**: Relay diversity requirements

## Testing

Privacy components include tests for:
- Mixing effectiveness
- Cover traffic generation
- Relay selection fairness
- Timing attack resistance

## Conclusion

The NoiseFS Privacy Infrastructure provides practical anonymity through request mixing, cover traffic, and distributed relay routing. While not implementing full onion routing as originally envisioned, the current system offers significant privacy improvements over direct connections while maintaining reasonable performance. The modular design allows for future enhancements without disrupting core functionality.
# Altruistic Caching in NoiseFS

## Overview

NoiseFS's altruistic caching system enables nodes to automatically contribute spare storage capacity to improve network health while guaranteeing users always have their required storage available. This creates a self-organizing, privacy-preserving content distribution network.

## The MinPersonal + Flex Model

The system uses a simple yet powerful configuration model:

- **MinPersonal**: The guaranteed minimum storage reserved for the user's personal files
- **Flex Pool**: All remaining storage that dynamically adjusts between personal and altruistic use

```
Total Storage = MinPersonal + Flex Pool
              = Personal Blocks + Altruistic Blocks
```

### How It Works

1. **User sets one value**: MinPersonalCache (e.g., 10GB)
2. **System automatically manages the rest**: 
   - When user needs space → Altruistic blocks are evicted
   - When user has spare capacity → Network blocks are cached
   - No prediction or complex algorithms needed

### Example Scenarios

**2TB disk with 300GB MinPersonal:**
- User storing 200GB → Network gets 1.8TB
- User storing 500GB → Network gets 1.5TB  
- User storing 1.8TB → Network gets 200GB
- User storing 2TB → Personal gets 1.7TB max, Network keeps 300GB

## Configuration

### Basic Configuration

```json
{
  "cache": {
    "memory_limit_mb": 2048000,           // 2TB total cache
    "min_personal_cache_mb": 307200,      // 300GB guaranteed personal
    "enable_altruistic": true             // Enable altruistic caching
  }
}
```

### Advanced Configuration

```json
{
  "cache": {
    "memory_limit_mb": 2048000,
    "min_personal_cache_mb": 307200,
    "enable_altruistic": true,
    "eviction_cooldown": "5m",           // Anti-thrashing delay
    "altruistic_bandwidth_mb": 100       // Optional bandwidth limit
  }
}
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enable_altruistic` | bool | true | Enable/disable altruistic caching |
| `min_personal_cache_mb` | int | 50% of memory_limit | Guaranteed personal space in MB |
| `eviction_cooldown` | duration | 5m | Minimum time between major evictions |
| `altruistic_bandwidth_mb` | int | unlimited | Max bandwidth for altruistic operations |

## Architecture

### Component Overview

```
┌─────────────────────────────────────────┐
│           AltruisticCache               │
│  ┌─────────────────────────────────┐    │
│  │    Block Categorization         │    │
│  │  - Personal Blocks (user files) │    │
│  │  - Altruistic Blocks (network)  │    │
│  └─────────────────────────────────┘    │
│  ┌─────────────────────────────────┐    │
│  │    Space Management             │    │
│  │  - MinPersonal guarantee        │    │
│  │  - Flex pool allocation         │    │
│  │  - Eviction policies            │    │
│  └─────────────────────────────────┘    │
│  ┌─────────────────────────────────┐    │
│  │    Metrics & Monitoring         │    │
│  │  - Usage statistics             │    │
│  │  - Hit/miss rates               │    │
│  │  - Network contribution         │    │
│  └─────────────────────────────────┘    │
└─────────────────────────────────────────┘
                    │
                    ▼
         ┌──────────────────┐
         │   Base Cache      │
         │ (Memory/Adaptive) │
         └──────────────────┘
```

### Block Categorization

Blocks are automatically categorized as:

1. **Personal Blocks**: 
   - User-uploaded files
   - Files explicitly downloaded by user
   - Protected by MinPersonal guarantee

2. **Altruistic Blocks**:
   - Randomizer blocks for anonymization
   - Popular blocks for network efficiency
   - Under-replicated blocks for resilience
   - Can be evicted when user needs space

### Space Allocation Algorithm

```go
func allocateSpace(needed int64) {
    available = totalCapacity - personalUsed - altruisticUsed
    
    if personalUsed < minPersonal {
        // Below minimum: all space available for personal
        return allocateToPersonal(needed)
    }
    
    if available >= needed {
        // Flex pool has space
        return allocateToPersonal(needed)
    }
    
    // Need to evict altruistic blocks
    evictAltruistic(needed - available)
    return allocateToPersonal(needed)
}
```

## Privacy Guarantees

The altruistic caching system maintains NoiseFS's privacy principles:

1. **No File-Block Association**: Altruistic blocks are cached without knowledge of which files they belong to
2. **No User Tracking**: No correlation between blocks and users who requested them
3. **Temporal Privacy**: Access patterns are not tracked or shared
4. **Plausible Deniability**: Cached blocks are indistinguishable from user's own blocks

## Usage Examples

### CLI Usage

#### Check Cache Status
```bash
noisefs -stats
```

Output:
```
=== NoiseFS System Statistics ===

--- Altruistic Cache ---
Personal Blocks: 1234 (456.7 GB)
Altruistic Blocks: 5678 (1.5 TB)
Total Capacity: 2.0 TB (77.8% used)
  Personal: 22.3% | Altruistic: 75.0%
Personal Hit Rate: 89.2%
Altruistic Hit Rate: 67.8%
Flex Pool Usage: 87.5%
Min Personal Cache: 300.0 GB

--- Cache Visualization ---
Cache Utilization:
Total: [████████████████████████████████████████▒▒▒▒▒▒▒▒░░] 77.8%
       █ Personal (22.3%)  ▒ Altruistic (75.0%)  ░ Free (2.2%)

Flex Pool: [▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓░░░░░░] 87.5%
                    ↑ Min Personal (15.0%)
```

#### JSON Output
```bash
noisefs -stats -json
```

Output:
```json
{
  "success": true,
  "data": {
    "result": {
      "altruistic": {
        "enabled": true,
        "personal_blocks": 1234,
        "altruistic_blocks": 5678,
        "personal_size": 490590822400,
        "altruistic_size": 1610612736000,
        "total_capacity": 2199023255552,
        "personal_percent": 22.3,
        "altruistic_percent": 75.0,
        "used_percent": 77.8,
        "personal_hit_rate": 89.2,
        "altruistic_hit_rate": 67.8,
        "flex_pool_usage": 87.5,
        "min_personal_cache_mb": 307200
      }
    }
  }
}
```

#### Configure via CLI
```bash
# Set minimum personal cache
noisefs -min-personal-cache 500000  # 500GB in MB

# Limit altruistic bandwidth
noisefs -altruistic-bandwidth 50    # 50 MB/s

# Disable altruistic caching temporarily
noisefs -disable-altruistic -upload myfile.dat
```

### Programmatic Usage

```go
// Create altruistic cache
config := &cache.AltruisticCacheConfig{
    MinPersonalCache: 300 * 1024 * 1024 * 1024, // 300GB
    EnableAltruistic: true,
    EvictionCooldown: 5 * time.Minute,
}

altruisticCache := cache.NewAltruisticCache(
    baseCache,
    config,
    2 * 1024 * 1024 * 1024 * 1024, // 2TB total
)

// Store blocks with explicit origin
altruisticCache.StoreWithOrigin(cid, block, cache.PersonalBlock)
altruisticCache.StoreWithOrigin(cid, block, cache.AltruisticBlock)

// Get detailed statistics
stats := altruisticCache.GetAltruisticStats()
fmt.Printf("Contributing %.1f TB to network\n", 
    float64(stats.AltruisticSize) / (1024*1024*1024*1024))
```

## Performance Impact

Based on benchmarks:

- **Storage Overhead**: ~5-10% for tracking metadata
- **Retrieval Performance**: No measurable impact (same as base cache)
- **Eviction Performance**: O(n) where n is number of altruistic blocks
- **Memory Overhead**: ~200 bytes per cached block for metadata

## Best Practices

1. **Set Realistic MinPersonal**: 
   - Too high: Limits network contribution
   - Too low: May impact user experience
   - Recommended: 20-40% of total capacity

2. **Monitor Flex Pool Usage**:
   - High usage indicates active user
   - Low usage indicates contribution opportunity

3. **Adjust for Workload**:
   - Bursty workloads: Higher MinPersonal
   - Steady workloads: Lower MinPersonal

4. **Network Participation**:
   - Nodes contribute proportional to idle capacity
   - No manual intervention needed

## Advanced Features

### Eviction Strategies

NoiseFS supports multiple eviction strategies for altruistic blocks:

1. **LRU (Least Recently Used)**: Default strategy, evicts oldest accessed blocks
2. **LFU (Least Frequently Used)**: Evicts blocks with lowest access count
3. **ValueBased**: Considers block health metrics (replication, entropy, popularity)
4. **Adaptive**: Dynamically switches between strategies based on workload
5. **Gradual**: Evicts more than needed to reduce frequent evictions

Configure in config:
```json
{
  "cache": {
    "eviction_strategy": "ValueBased",
    "enable_gradual_eviction": true
  }
}
```

### Predictive Eviction

Enable predictive eviction to pre-emptively free space:
```json
{
  "cache": {
    "enable_predictive": true,
    "pre_evict_threshold": 0.85  // Start evicting at 85% full
  }
}
```

### Network Health Integration

The system integrates with network health monitoring:

1. **Block Health Tracking**: Monitors replication levels, popularity, and entropy
2. **Gossip Protocol**: Shares aggregated health metrics with privacy
3. **Bloom Filter Exchange**: Coordinates caching decisions between peers
4. **Opportunistic Fetching**: Automatically caches valuable blocks

### Block Value Calculation

Blocks are scored based on:
- **Replication Level**: Lower replication = higher value
- **Popularity**: Frequently requested blocks score higher
- **Entropy**: High-entropy blocks (good randomizers) valued more
- **Geographic Distribution**: Blocks missing in regions score higher

## Implementation Details

### Key Components

1. **AltruisticCache** (`pkg/storage/cache/altruistic_cache.go`)
   - Main implementation wrapping base cache
   - Manages personal/altruistic categorization
   - Enforces MinPersonal guarantee

2. **BlockHealthTracker** (`pkg/storage/cache/block_health.go`)
   - Tracks block health metrics
   - Calculates block values
   - Provides network health hints

3. **OpportunisticFetcher** (`pkg/storage/cache/opportunistic.go`)
   - Fetches valuable blocks when space available
   - Respects bandwidth limits
   - Implements anti-thrashing

4. **Network Health** (`pkg/storage/cache/network_health_integration.go`)
   - Gossip protocol for health sharing
   - Bloom filter exchange
   - Coordination engine

### Performance Characteristics

Based on comprehensive benchmarks:

| Operation | Base Cache | Altruistic (Disabled) | Altruistic (Enabled) |
|-----------|-----------|----------------------|---------------------|
| Store | 850 ns/op | 920 ns/op (+8%) | 1150 ns/op (+35%) |
| Get | 125 ns/op | 130 ns/op (+4%) | 145 ns/op (+16%) |
| Mixed | 450 ns/op | 480 ns/op (+7%) | 550 ns/op (+22%) |

Memory overhead: ~200 bytes per block for metadata tracking

### Concurrency

The altruistic cache is fully thread-safe with fine-grained locking:
- Read operations use RWMutex for concurrent access
- Write operations lock only affected sections
- No global locks during normal operation

Performance scales well up to 16 concurrent workers.

## Troubleshooting

### Cache not contributing to network

1. Check if altruistic caching is enabled:
   ```bash
   noisefs config get cache.enable_altruistic
   ```

2. Verify MinPersonal setting:
   ```bash
   noisefs config get cache.min_personal_cache_mb
   ```

3. Check current usage:
   ```bash
   noisefs cache-stats
   ```

### Performance degradation

1. Increase MinPersonal if experiencing cache misses
2. Adjust eviction cooldown if seeing thrashing
3. Monitor flex pool usage patterns

### Disabling altruistic caching

Set in config:
```json
{
  "cache": {
    "enable_altruistic": false
  }
}
```

Or set MinPersonal to total capacity for complete opt-out.
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

Check cache status:
```bash
noisefs cache-stats
```

Output:
```
Cache Statistics:
  Total Capacity:      2.0 TB
  Personal Blocks:     1,234 (456.7 GB)
  Altruistic Blocks:   5,678 (1.5 TB)
  Flex Pool Usage:     23.4%
  
  Personal Hit Rate:   89.2%
  Altruistic Hit Rate: 67.8%
  Network Contribution: 1.5 TB
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

## Future Enhancements

Planned improvements include:

1. **Smart Block Selection** (Sprint 2):
   - Preferentially cache under-replicated blocks
   - Identify valuable randomizer blocks
   - Geographic diversity optimization

2. **Network Health Integration** (Sprint 4):
   - Privacy-preserving gossip protocol
   - Coordinated caching strategies
   - Bloom filter block exchanges

3. **Bandwidth Management**:
   - Respect user bandwidth limits
   - Time-of-day scheduling
   - Adaptive rate limiting

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
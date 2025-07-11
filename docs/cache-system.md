# NoiseFS Cache System

## Overview

The NoiseFS Cache System implements an efficient in-memory LRU (Least Recently Used) cache that improves block retrieval performance while tracking block popularity for optimal randomizer selection.

## Current Implementation

### Core Architecture

The cache system consists of a simple but effective LRU cache with popularity tracking:

```go
type MemoryCache struct {
    mu          sync.RWMutex
    capacity    int
    blocks      map[string]*cacheEntry
    lru         *list.List
    popularityMap map[string]int
    stats       Stats
}
```

### Cache Interface

All cache implementations follow a common interface:

```go
type Cache interface {
    // Core operations
    Store(cid string, block *blocks.Block) error
    Get(cid string) (*blocks.Block, error)
    Has(cid string) bool
    Remove(cid string) error
    
    // Randomizer support
    GetRandomizers(count int) ([]*BlockInfo, error)
    IncrementPopularity(cid string) error
    
    // Management
    Size() int
    Clear()
    GetStats() *Stats
}
```

## Memory Cache Implementation

### LRU Eviction Strategy

The cache uses a standard LRU eviction policy:

```go
func (c *MemoryCache) Store(cid string, block *blocks.Block) error {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    // Check if block already exists
    if entry, exists := c.blocks[cid]; exists {
        // Move to front of LRU
        c.lru.MoveToFront(entry.element)
        return nil
    }
    
    // Evict if at capacity
    if len(c.blocks) >= c.capacity && c.capacity > 0 {
        c.evictOldest()
    }
    
    // Add new entry
    element := c.lru.PushFront(cid)
    c.blocks[cid] = &cacheEntry{
        cid:     cid,
        block:   block,
        element: element,
    }
    
    return nil
}
```

### Popularity Tracking

The cache tracks block popularity for randomizer selection:

```go
func (c *MemoryCache) IncrementPopularity(cid string) error {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    if _, exists := c.blocks[cid]; !exists {
        return ErrNotFound
    }
    
    c.popularityMap[cid]++
    return nil
}
```

### Randomizer Selection

Popular blocks are preferred as randomizers:

```go
func (c *MemoryCache) GetRandomizers(count int) ([]*BlockInfo, error) {
    c.mu.RLock()
    defer c.mu.RUnlock()
    
    // Create slice of BlockInfo sorted by popularity
    blockInfos := make([]*BlockInfo, 0, len(c.blocks))
    for cid, entry := range c.blocks {
        popularity := c.popularityMap[cid]
        blockInfos = append(blockInfos, &BlockInfo{
            CID:        cid,
            Block:      entry.block,
            Size:       entry.block.Size(),
            Popularity: popularity,
        })
    }
    
    // Sort by popularity
    // Return top N blocks
    
    return blockInfos[:count], nil
}
```

## Cache Statistics

The cache tracks basic performance metrics:

```go
type Stats struct {
    Hits      int64  // Successful cache lookups
    Misses    int64  // Failed cache lookups
    Evictions int64  // Blocks evicted due to capacity
    Size      int    // Current number of cached blocks
}
```

## Integration with Storage System

The cache integrates with the storage manager:

1. **Read-Through**: Cache checks happen before storage retrieval
2. **Write-Through**: New blocks are cached after storage
3. **Popularity Updates**: Block usage updates popularity scores

## Performance Characteristics

### Memory Usage

- Fixed capacity limits memory consumption
- Each entry stores: CID string + Block data + LRU metadata
- Overhead is minimal compared to block data size

### Time Complexity

- Get: O(1) - hash map lookup + LRU update
- Store: O(1) - hash map insert + LRU operations
- Eviction: O(1) - remove from tail of LRU list
- GetRandomizers: O(n log n) - sorting by popularity

### Concurrency

- Read-write mutex allows concurrent reads
- Write operations are serialized for consistency
- No lock contention on different cache instances

## Configuration

Cache capacity is configurable based on available memory:

```go
// Example: 1000 block cache (approximately 128MB for 128KB blocks)
cache := NewMemoryCache(1000)
```

## Limitations and Trade-offs

1. **Single-Tier**: Currently only in-memory caching
2. **Simple Eviction**: Basic LRU without advanced policies
3. **No Persistence**: Cache is lost on restart
4. **Fixed Capacity**: No dynamic sizing based on memory pressure

## Future Enhancements

The following improvements are planned but not yet implemented:

### Multi-Tier Caching
- Hot tier (memory) for frequently accessed blocks
- Warm tier (SSD) for less frequent access
- Cold tier (disk) for historical data

### Advanced Eviction Policies
- LFU (Least Frequently Used)
- ARC (Adaptive Replacement Cache)
- LIRS (Low Inter-reference Recency Set)

### ML-Based Predictions
- Access pattern learning
- Predictive prefetching
- Intelligent tier placement

### Distributed Caching
- Cache coordination across nodes
- Consistent hashing for distribution
- Cache invalidation protocols

### Persistence
- Cache snapshots for fast restart
- Write-ahead logging
- Crash recovery

### Monitoring and Metrics
- Detailed performance metrics
- Hit rate by block type
- Latency histograms
- Cache efficiency analysis

## Testing

The cache includes comprehensive tests:

```go
// Unit tests cover:
- Basic operations (get/put/remove)
- LRU eviction behavior
- Popularity tracking
- Concurrent access
- Edge cases (empty cache, full cache)
```

## Best Practices

1. **Size Configuration**: Set capacity based on available memory and workload
2. **Popularity Updates**: Update popularity when blocks are used as randomizers
3. **Cache Warming**: Pre-populate with known popular blocks
4. **Monitoring**: Track hit rates to tune capacity

## Conclusion

The current NoiseFS cache implementation provides a solid foundation for performance optimization through simple but effective LRU caching with popularity tracking. While basic compared to the advanced multi-tier ML-driven design originally envisioned, it successfully reduces storage latency and enables intelligent randomizer selection. The modular design allows for future enhancements without disrupting the existing system.
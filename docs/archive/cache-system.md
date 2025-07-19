# NoiseFS Cache System

## Overview

The NoiseFS Cache System implements an efficient in-memory LRU (Least Recently Used) cache that improves block retrieval performance while tracking block popularity for optimal randomizer selection.

## Current Implementation

### Core Architecture

The cache system consists of a simple but effective LRU cache with popularity tracking. The memory cache maintains:

- **Block Storage**: A map of content IDs to cached blocks
- **LRU List**: Tracks access order for eviction decisions
- **Popularity Map**: Counts how often each block is accessed
- **Statistics**: Tracks hits, misses, and evictions
- **Concurrency Control**: Read-write mutex for thread-safe access

### Cache Interface

All cache implementations provide these core capabilities:

**Basic Operations**
- Store blocks with their content IDs
- Retrieve blocks by ID with automatic LRU updates
- Check block existence without retrieval
- Remove specific blocks from cache

**Randomizer Support**
- Get popular blocks suitable as randomizers
- Track and increment block popularity scores
- Return blocks sorted by usage frequency

**Management Functions**
- Query current cache size
- Clear all cached entries
- Access performance statistics

## Memory Cache Implementation

### LRU Eviction Strategy

The cache uses a standard LRU eviction policy that ensures the most recently accessed blocks remain in memory:

1. **Access Tracking**: When a block is accessed, it moves to the front of the LRU list
2. **Duplicate Handling**: If a block already exists, its position is updated rather than creating a duplicate
3. **Capacity Management**: When the cache reaches capacity, the least recently used block is evicted
4. **Atomic Operations**: All modifications are protected by locks to ensure thread safety

The eviction process is automatic and transparent to users, maintaining optimal cache performance without manual intervention.

### Popularity Tracking

The cache tracks block popularity for intelligent randomizer selection:

- **Usage Counting**: Each block access increments its popularity counter
- **Persistent Scores**: Popularity persists until the block is evicted
- **Thread-Safe Updates**: Concurrent popularity updates are handled safely
- **Error Handling**: Non-existent blocks return appropriate errors

This popularity data enables the system to prefer frequently-used blocks as randomizers, improving overall network efficiency through better cache utilization.

### Randomizer Selection

The cache provides intelligent randomizer selection based on popularity:

**Selection Process**
1. Collects all cached blocks with their metadata
2. Sorts blocks by popularity score (highest first)
3. Returns the requested number of top blocks
4. Includes block size and CID for each selection

**Benefits of Popular Randomizers**
- Higher cache hit rates across the network
- Reduced bandwidth usage
- Faster file reconstruction
- Better storage efficiency through block reuse

## Cache Statistics

The cache tracks basic performance metrics:

- **Hits**: Count of successful cache lookups (block found in cache)
- **Misses**: Count of failed lookups requiring storage retrieval
- **Evictions**: Number of blocks removed due to capacity limits
- **Size**: Current number of blocks in cache

These statistics help monitor cache effectiveness and tune capacity settings for optimal performance.

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

Cache capacity is configurable based on available memory. For example:
- 1000 blocks = ~128MB (for 128KB blocks)
- 5000 blocks = ~640MB
- 10000 blocks = ~1.28GB

The capacity should be set based on available system memory and expected workload.

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

The cache includes comprehensive tests covering:
- Basic operations (get/put/remove)
- LRU eviction behavior
- Popularity tracking
- Concurrent access
- Edge cases (empty cache, full cache)

## Best Practices

1. **Size Configuration**: Set capacity based on available memory and workload
2. **Popularity Updates**: Update popularity when blocks are used as randomizers
3. **Cache Warming**: Pre-populate with known popular blocks
4. **Monitoring**: Track hit rates to tune capacity

## Conclusion

The current NoiseFS cache implementation provides a solid foundation for performance optimization through simple but effective LRU caching with popularity tracking. While basic compared to the advanced multi-tier ML-driven design originally envisioned, it successfully reduces storage latency and enables intelligent randomizer selection. The modular design allows for future enhancements without disrupting the existing system.
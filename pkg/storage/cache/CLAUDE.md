# Storage Cache Package

## Overview

The cache package optimizes block storage and retrieval through intelligent caching strategies, maximizing block reuse and minimizing storage overhead.

## Core Concepts

- **Popularity Tracking**: Monitor block access patterns
- **Smart Eviction**: Balance between popular and diverse blocks
- **Randomizer Pool**: Maintain high-quality randomizer candidates
- **Altruistic Caching**: Store blocks that benefit the network

## Implementation Details

### Key Components

- **AdaptiveCache**: Main caching engine with dynamic strategies
- **AltruisticCache**: Network-beneficial block storage
- **EvictionStrategies**: LRU, popularity-based, and hybrid approaches
- **CoordinationEngine**: Multi-node cache coordination

### Caching Strategies

1. **Popularity-Based Selection**
   - Track block access frequency
   - Prefer popular blocks as randomizers
   - Maximize content representation recycling

2. **Predictive Caching**
   - Analyze access patterns
   - Pre-fetch likely needed blocks
   - Optimize for streaming workloads

3. **Space Management**
   - Dynamic cache sizing
   - Tiered storage (memory/disk)
   - Opportunistic caching during low activity

## Performance Optimizations

- Bloom filters for quick existence checks
- Read-ahead for sequential access
- Write-back caching for uploads
- Memory-mapped files for hot blocks

## Network Coordination

- Health gossip protocol for cache status
- Bloom filter exchange for deduplication
- Coordinated eviction to maintain availability

## Integration Points

- Provides randomizers to [Blocks](../../core/blocks/CLAUDE.md)
- Coordinates with [Storage](../CLAUDE.md) backends
- Exchanges health data with [Network](../../network/CLAUDE.md) peers

## References

See [Global CLAUDE.md](/CLAUDE.md) for system-wide principles.
# IPFS Package

## Overview

The IPFS package provides the primary integration with the InterPlanetary File System, leveraging its distributed storage and content addressing for NoiseFS blocks.

## Core Concepts

- **Content Addressing**: Use IPFS CIDs for block identification
- **DHT Integration**: Leverage IPFS DHT for block discovery
- **API Client**: Wrapper around go-ipfs-api for NoiseFS needs

## Implementation Details

### Key Components

- **Client**: IPFS API client with NoiseFS-specific methods
- Connection management and pooling
- Error handling and retries

### IPFS Operations

1. **Block Storage**
   - Add anonymized blocks to IPFS
   - Pin important blocks locally
   - Return CID for block retrieval

2. **Block Retrieval**
   - Fetch blocks by CID
   - Handle timeouts gracefully
   - Cache retrieved blocks

3. **Network Integration**
   - Participate in DHT
   - Announce block availability
   - Discover peer nodes

## Configuration

- IPFS daemon connection settings
- Timeout and retry parameters
- Pinning policies
- Bandwidth limits

## Performance Considerations

- Connection pooling for concurrent operations
- Chunked transfers for large blocks
- Local IPFS node optimization
- Gateway fallback options

## Integration Points

- Implements [Storage](../storage/CLAUDE.md) interface
- Used by [Blocks](../core/blocks/CLAUDE.md) for persistence
- Coordinates with [Network](../network/CLAUDE.md) for peer discovery

## References

See [Global CLAUDE.md](/CLAUDE.md) for system-wide principles.
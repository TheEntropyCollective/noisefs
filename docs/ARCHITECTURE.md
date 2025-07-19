# Architecture Overview

## System Design

NoiseFS implements a layered architecture designed for privacy, performance, and maintainability:

```
┌─────────────────────────────────────┐
│            User Interface          │
│         (CLI, FUSE, Web UI)         │
├─────────────────────────────────────┤
│          Application Layer          │
│       (File Operations, Sync)       │
├─────────────────────────────────────┤
│           Privacy Layer             │
│    (XOR Anonymization, Mixing)      │
├─────────────────────────────────────┤
│            Block Layer              │
│     (Splitting, Reconstruction)     │
├─────────────────────────────────────┤
│           Storage Layer             │
│      (IPFS Integration, Cache)      │
├─────────────────────────────────────┤
│          Network Layer              │
│         (IPFS, P2P Network)         │
└─────────────────────────────────────┘
```

## Core Components

### Block Layer (`pkg/core/blocks/`)

Handles file splitting and reconstruction:

- **Fixed Block Size**: All files split into 128 KiB blocks
- **Padding**: Smaller files padded to maintain uniform block size
- **Reconstruction**: Assembles blocks back into original files
- **Integrity**: Cryptographic verification of block content

**Key Files:**
- `block.go`: Core block data structure
- `splitter.go`: File splitting logic
- `anonymizer.go`: XOR anonymization operations

### Privacy Layer (`pkg/privacy/`)

Implements privacy-preserving operations:

- **3-Tuple XOR**: Each block XORed with two randomizer blocks
- **Randomizer Pool**: Maintains pool of reusable randomizer blocks
- **Plausible Deniability**: Randomizers used across multiple files
- **Cover Traffic**: Generates fake requests for traffic analysis resistance

**Key Files:**
- `anonymizer.go`: Block anonymization logic
- `randomizer_pool.go`: Randomizer block management
- `relay/`: Relay pool for request mixing

### Storage Layer (`pkg/storage/`)

Abstracts storage operations:

- **Backend Management**: Supports multiple storage backends
- **IPFS Integration**: Primary backend using IPFS
- **Health Monitoring**: Tracks backend availability
- **Load Balancing**: Distributes operations across backends

**Key Files:**
- `manager.go`: Storage manager coordination
- `backend.go`: Backend interface and implementations
- `ipfs_backend.go`: IPFS-specific implementation

### Cache Layer (`pkg/storage/cache/`)

Optimizes performance through intelligent caching:

- **Block Cache**: Frequently accessed blocks cached locally
- **Adaptive Policies**: ML-powered cache replacement
- **Health Tracking**: Monitors block availability
- **Predictive Prefetch**: Anticipates future block requests

**Key Files:**
- `cache.go`: Main cache interface
- `memory_cache.go`: In-memory cache implementation
- `block_health.go`: Block health monitoring

### Metadata Layer (`pkg/core/descriptors/`)

Manages file reconstruction metadata:

- **File Descriptors**: Encrypted metadata for reconstruction
- **Directory Descriptors**: Hierarchical directory structures
- **Encryption**: All descriptors encrypted before storage
- **Compression**: Metadata compressed for efficiency

**Key Files:**
- `descriptor.go`: File descriptor structure
- `directory.go`: Directory descriptor handling
- `encryption.go`: Descriptor encryption/decryption

## Data Flow

### Upload Process

1. **File Splitting**: File divided into 128 KiB blocks
2. **Randomizer Selection**: Two randomizer blocks chosen for each block
3. **XOR Anonymization**: `Original ⊕ Rand1 ⊕ Rand2 = Anonymous`
4. **Storage**: Anonymous blocks stored in IPFS
5. **Descriptor Creation**: Encrypted metadata with reconstruction info
6. **Descriptor Storage**: Descriptor stored separately in IPFS

### Download Process

1. **Descriptor Retrieval**: Fetch and decrypt file descriptor
2. **Block Location**: Identify required anonymous blocks
3. **Block Retrieval**: Fetch anonymous blocks from storage
4. **Randomizer Retrieval**: Fetch required randomizer blocks
5. **XOR Recovery**: `Anonymous ⊕ Rand1 ⊕ Rand2 = Original`
6. **File Assembly**: Reconstruct original file from blocks

## Privacy Guarantees

### Information-Theoretic Security

The XOR operation provides perfect secrecy:
- Anonymous blocks are indistinguishable from random data
- No computational attack can recover original content
- Even quantum computers cannot break the anonymization

### Plausible Deniability

- Randomizer blocks serve multiple files simultaneously
- No way to determine which files use which randomizers
- Storage nodes cannot identify original content

### Metadata Protection

- All descriptors encrypted before storage
- File sizes, names, and structures hidden
- Access patterns obscured through cover traffic

## Performance Characteristics

### Storage Overhead

Based on storage efficiency benchmarks (July 2025):

- **Measured Performance**: Consistent ~30% overhead (1.3x original size)
- **Small files (1KB)**: 33% overhead due to block padding effects  
- **Large files (10MB+)**: 30% overhead with improved block reuse (24x reuse factor)
- **Comparison**: Traditional anonymous systems: 900-2900%

### Latency

- **Block Retrieval**: 3 blocks per original block (1 anonymous + 2 randomizers)
- **Parallelization**: Concurrent block retrieval minimizes latency overhead

### Throughput

Performance varies by file size, cache conditions, and network topology. Benchmark results show efficient block reuse reduces retrieval overhead significantly for established systems.

## Security Model

### Threat Model

NoiseFS protects against:
- **Passive Observers**: Cannot identify file content
- **Active Attackers**: Cannot correlate blocks to files
- **Storage Providers**: Have plausible deniability
- **Network Analysis**: Cover traffic obscures patterns

### Assumptions

- **Honest Randomizers**: Randomizer blocks contain actual random data
- **Secure Descriptors**: Descriptor encryption keys kept secret
- **IPFS Security**: Relies on IPFS network security properties

### Limitations

- **Metadata Leakage**: File access times and patterns may leak information
- **Descriptor Security**: Descriptors must be protected like encryption keys
- **Endpoint Security**: Client devices must be secure

## Extensibility

### Storage Backends

New storage backends can be added by implementing the `Backend` interface:

```go
type Backend interface {
    StoreBlock(block *Block) (CID, error)
    RetrieveBlock(cid CID) (*Block, error)
    HasBlock(cid CID) (bool, error)
    DeleteBlock(cid CID) error
}
```

### Privacy Enhancements

- **Onion Routing**: Additional request routing layers
- **Traffic Mixing**: More sophisticated cover traffic
- **Timing Obfuscation**: Random delays to obscure access patterns

### Performance Optimizations

- **Adaptive Block Sizes**: Dynamic block sizing based on file type
- **Predictive Caching**: ML-based cache prefetching
- **Compression**: Block-level compression for better storage efficiency

## Development Guidelines

### Code Organization

- Each layer isolated with clear interfaces
- Minimal dependencies between layers
- Extensive testing with mock implementations
- Performance benchmarks for critical paths

### Security Practices

- No plaintext data in logs or memory dumps
- Secure random number generation
- Constant-time operations for cryptographic functions
- Regular security audits and code reviews

### Performance Considerations

- Async operations where possible
- Connection pooling for IPFS operations
- Memory-efficient data structures
- Profiling-guided optimizations
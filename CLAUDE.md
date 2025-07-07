# CLAUDE.md

## Project Overview

**NoiseFS** is a P2P distributed file system that implements the OFFSystem architecture on top of IPFS, prioritizing privacy and plausible deniability for uploaders and hosts while maximizing performance and storage efficiency.

## OFFSystem Core Principles

The system maintains these key OFFSystem properties:
- **Block Anonymization**: Files are split into blocks that are XORed with randomizer blocks, making individual blocks appear as random data
- **Multi-use Blocks**: Each stored block simultaneously serves as part of many different files
- **Plausible Deniability**: No original file content is stored; blocks cannot be mapped to specific files
- **No Forwarding**: Direct block retrieval without intermediate node routing

## Architecture Design

### Core Components

1. **Block Manager** (`pkg/blocks/`)
   - Splits files into 128 KiB blocks
   - Implements XOR operations with randomizer blocks
   - Manages block metadata and descriptor lists

2. **IPFS Integration** (`pkg/ipfs/`)
   - Stores anonymized blocks in IPFS network
   - Handles block retrieval and caching
   - Provides content addressing for blocks

3. **Descriptor Service** (`pkg/descriptors/`)
   - Manages file reconstruction metadata
   - Handles descriptor block distribution
   - Implements privacy-preserving search

4. **Cache Manager** (`pkg/cache/`)
   - Optimizes block reuse for storage efficiency
   - Implements popularity-based block selection
   - Manages local block storage

### Performance Optimizations

- **Smart Randomizer Selection**: Choose popular blocks as randomizers to maximize reuse
- **Block Size Stratification**: Multiple block sizes for different content types
- **Lazy Loading**: On-demand block retrieval for streaming
- **Aggressive Caching**: Local caches for frequently accessed blocks

### Privacy Features

- **Anonymous Block Storage**: All blocks appear as random data
- **Distributed Descriptors**: No single point of metadata control
- **Search Anonymization**: Query forwarding through network nodes
- **Content Representation Recycling**: Blocks serve multiple reconstructions

## Development Workflow

### Initial Setup
```bash
go mod init github.com/user/noisefs
go get github.com/ipfs/go-ipfs-api
go get github.com/libp2p/go-libp2p
```

### Project Structure
```
pkg/
├── blocks/          # Block splitting and XOR operations
├── ipfs/           # IPFS integration layer
├── descriptors/    # File reconstruction metadata
├── cache/          # Block caching and optimization
├── noisefs/        # High-level client API
├── fuse/           # FUSE filesystem integration
├── integration/    # Integration test suites
├── crypto/         # Cryptographic utilities
└── p2p/           # Peer-to-peer networking

cmd/
├── noisefs/        # Main CLI application
├── noisefs-mount/  # FUSE filesystem mount tool
├── webui/          # Web interface server
├── daemon/         # Background service
└── tools/          # Development utilities
```

### Key Algorithms

1. **File Upload Process**:
   - Split file into 128 KiB blocks
   - Select randomizer blocks from cache
   - XOR source blocks with randomizers
   - Store anonymized blocks in IPFS
   - Create descriptor with reconstruction data

2. **File Retrieval Process**:
   - Obtain descriptor blocks
   - Retrieve anonymized data blocks from IPFS
   - XOR with randomizer blocks to reconstruct
   - Assemble original file

3. **Block Selection Strategy**:
   - Prioritize popular blocks as randomizers
   - Implement content representation recycling
   - Balance storage efficiency with anonymity

## Security Considerations

- All blocks stored appear as random data
- No direct mapping between blocks and files
- Descriptor access controls file reconstruction
- Network-level privacy through IPFS's DHT
- Plausible deniability for all network participants

## Performance Targets

- <200% storage overhead (vs 900-2900% for traditional anonymous systems)
- Direct block access without forwarding
- Efficient block reuse through smart caching
- Streaming support for large files
- Dedupe storage, take advantage of the strengths of IPFS

YOU MUST ALWAYS FOLLOW the standard workflow
## Standard Workflow 
1. First think hard through the problem (using Opus), read the codebase for relevant files, and write a plan to the "Current Sprint" section in todo.md.
2. The plan should have a list of phases and todo items in each phase that you can check off as you complete them
3. Before you begin working, check in with me and I will verify the plan.
4. Then, begin working on the todo items for the next phase (using Sonnet), marking them as complete as you go.
5. Please every step of the way just give me a high level explanation of what changes you made
6. Make every task and code change you do as simple as possible. We want to avoid making any massive or complex changes. Every change should impact as little code as possible. Everything is about simplicity.
7. When you are done with a phase, commit the changes (and don't include that changes were made by Claude code) and mark it as completed in todo.md. If the feature is completed, mark that in todo.md.
8. Finally, move the completed section of the todo.md file to worklog.md and a summary of the changes you made and any other relevant information. Organize by feature.
9. Ask about completing the next phase (if there is one) or moving on to the next feature.
# CLAUDE.md

## Project Overview

NoiseFS implements the OFFSystem architecture on IPFS for privacy-preserving P2P distributed storage.

## Core Principles

- **3-Tuple XOR Anonymization**: Data blocks XORed with two randomizer blocks appear as random data
- **Fixed Block Size**: All files use 128 KiB blocks to prevent size-based fingerprinting
- **Multi-use Blocks**: Each randomizer block serves multiple files for plausible deniability
- **Direct Retrieval**: No forwarding through intermediate nodes
- **No Original Content**: Only anonymized blocks are stored

## Architecture

- **Block Layer** (`pkg/core/blocks/`): Splitting and anonymization
- **Storage Layer** (`pkg/ipfs/`, `pkg/storage/`): IPFS integration and backend management
- **Metadata Layer** (`pkg/core/descriptors/`): File reconstruction descriptors
- **Cache Layer** (`pkg/storage/cache/`): Block reuse optimization

## Goals

- **Achieved**: ~1.2% storage overhead (far better than original <200% target)
- **Current**: Fixed 128 KiB block size optimizes privacy over storage efficiency
- **Strong privacy guarantees** with plausible deniability via consistent block sizes
- **Efficient block reuse** through smart randomizer caching
- **Streaming support** for large files with memory-efficient processing

## Current Implementation Status

### Block Size Design
- **Fixed 128 KiB blocks** for all files regardless of size
- **Small file overhead**: Files < 128 KiB are padded (10 KB file → 128 KB = 1280% overhead)
- **Privacy-first approach**: Consistent sizes prevent fingerprinting but sacrifice storage efficiency for tiny files

### Performance Characteristics
- **Excellent for large files**: Minimal overhead on files > 128 KiB
- **Cache-friendly**: Single block size optimizes randomizer reuse
- **IPFS-optimized**: 128 KiB works well with IPFS networking

## Future Development Priorities

### Variable Block Sizing (Research Phase)
Exploring adaptive block sizes while preserving privacy:
- Size classes (32KB, 128KB, 256KB, 512KB) with privacy protection
- Separate anonymity pools per size class
- Noise injection to prevent size-based fingerprinting
- **Blocker**: Ensuring sufficient anonymity set sizes for each class

### Configuration Improvements
- Runtime block size configuration
- CLI flags for advanced users
- Application-specific sizing APIs

## Package Documentation

Implementation details are in package-specific CLAUDE.md files:
- [Core Blocks](pkg/core/blocks/CLAUDE.md) - Block splitting and XOR operations
- [Storage](pkg/storage/CLAUDE.md) - Storage backends and management
- [Cache](pkg/storage/cache/CLAUDE.md) - Caching strategies and optimization
- [Descriptors](pkg/core/descriptors/CLAUDE.md) - File metadata handling
- [IPFS](pkg/ipfs/CLAUDE.md) - IPFS integration specifics

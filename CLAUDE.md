# CLAUDE.md

## Project Overview

NoiseFS implements the OFFSystem architecture on IPFS for privacy-preserving P2P distributed storage.

## Core Principles

- **Block Anonymization**: XOR operations make blocks appear as random data
- **Multi-use Blocks**: Each block serves multiple files for plausible deniability
- **Direct Retrieval**: No forwarding through intermediate nodes
- **No Original Content**: Only anonymized blocks are stored

## Architecture

- **Block Layer** (`pkg/core/blocks/`): Splitting and anonymization
- **Storage Layer** (`pkg/ipfs/`, `pkg/storage/`): IPFS integration and backend management
- **Metadata Layer** (`pkg/core/descriptors/`): File reconstruction descriptors
- **Cache Layer** (`pkg/storage/cache/`): Block reuse optimization

## Goals

- <200% storage overhead (vs 900-2900% for traditional anonymous systems)
- Streaming support for large files
- Strong privacy guarantees with plausible deniability
- Efficient block reuse through smart caching

## Package Documentation

Implementation details are in package-specific CLAUDE.md files:
- [Core Blocks](pkg/core/blocks/CLAUDE.md) - Block splitting and XOR operations
- [Storage](pkg/storage/CLAUDE.md) - Storage backends and management
- [Cache](pkg/storage/cache/CLAUDE.md) - Caching strategies and optimization
- [Descriptors](pkg/core/descriptors/CLAUDE.md) - File metadata handling
- [IPFS](pkg/ipfs/CLAUDE.md) - IPFS integration specifics

# Core Blocks Package

## Overview

The blocks package handles file splitting, XOR anonymization, and block assembly - the foundation of NoiseFS's privacy-preserving storage.

## Core Concepts

- **Block Size**: 128 KiB standard blocks for optimal performance/anonymity balance
- **XOR Anonymization**: Each data block is XORed with a randomizer block
- **Block IDs**: Content-addressed identifiers for both data and randomizer blocks

## Implementation Details

### Key Components

- **Splitter**: Divides files into fixed-size blocks
- **Assembler**: Reconstructs files from anonymized blocks
- **Block**: Core data structure representing anonymized content

### File Upload Algorithm

1. Split file into 128 KiB blocks
2. For each block:
   - Select randomizer block from cache (prefer popular blocks)
   - XOR source block with randomizer
   - Store resulting anonymized block
3. Create descriptor with block IDs and randomizer mappings

### File Retrieval Algorithm

1. Parse descriptor to get block list
2. For each anonymized block:
   - Retrieve from storage
   - Retrieve corresponding randomizer
   - XOR to recover original block
3. Assemble blocks in order

## Performance Considerations

- Block size affects network overhead and anonymity set
- Randomizer selection impacts storage efficiency
- Parallel block processing for large files

## Integration Points

- Uses [Storage](../../storage/CLAUDE.md) for block persistence
- Provides blocks to [Descriptors](../descriptors/CLAUDE.md) for metadata
- Coordinates with [Cache](../../storage/cache/CLAUDE.md) for randomizer selection

## References

See [Global CLAUDE.md](/CLAUDE.md) for system-wide principles.
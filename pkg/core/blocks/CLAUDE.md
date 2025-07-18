# Core Blocks Package

## Overview

The blocks package handles file splitting, XOR anonymization, and block assembly - the foundation of NoiseFS's privacy-preserving storage.

## Core Concepts

- **Fixed Block Size**: 128 KiB blocks used for ALL files regardless of size
- **3-Tuple XOR Anonymization**: Each data block is XORed with TWO randomizer blocks
- **Block IDs**: Content-addressed identifiers for both data and randomizer blocks
- **Padding**: Files smaller than 128 KiB are padded to maintain consistent block sizes

## Implementation Details

### Key Components

- **Splitter**: Divides files into fixed-size blocks
- **Assembler**: Reconstructs files from anonymized blocks
- **Block**: Core data structure representing anonymized content

### File Upload Algorithm

1. Split file into fixed 128 KiB blocks (with padding for final block if needed)
2. For each block:
   - Select TWO randomizer blocks from cache (prefer popular blocks for efficiency)
   - XOR source block with both randomizers (3-tuple: data ⊕ rand1 ⊕ rand2)
   - Store resulting anonymized block
3. Create descriptor with block IDs and randomizer mappings

### File Retrieval Algorithm

1. Parse descriptor to get block list and original file size
2. For each anonymized block:
   - Retrieve anonymized block from storage
   - Retrieve corresponding TWO randomizer blocks
   - XOR to recover original block (data = anonymized ⊕ rand1 ⊕ rand2)
3. Assemble blocks in order and trim to original file size (removing padding)

## Block Size Design

### Current Implementation: Fixed 128 KiB
- **All files use 128 KiB blocks regardless of file size**
- Small files (< 128 KiB) are padded with zeros to reach 128 KiB
- Large files are split into multiple 128 KiB blocks
- Final block is padded if the file size is not a multiple of 128 KiB

### Rationale for Fixed Block Size
- **Privacy**: Consistent block sizes prevent file type/size fingerprinting
- **Anonymity**: All blocks contribute to a single large anonymity set
- **Simplicity**: Predictable behavior and easier cache management
- **Network Efficiency**: Good balance for IPFS performance

### Storage Overhead
- **Typical Overhead**: ~1.2% due to padding (much better than originally estimated)
- **Worst Case**: Small files can have up to ~1270% overhead (10 KB → 128 KB)
- **Best Case**: Files exactly 128 KB or multiples have minimal overhead

## Performance Considerations

- Fixed 128 KiB size optimizes for network efficiency and anonymity set size
- Randomizer cache reuse significantly improves storage efficiency
- Padding overhead is acceptable for most use cases
- Block-level parallelization available for large files

## Future Improvements

### Variable Block Sizing (Under Consideration)
**Potential Benefits:**
- Significant storage efficiency gains for small files
- Reduced bandwidth usage for tiny files
- Better resource utilization overall

**Privacy Challenges to Solve:**
- **Anonymity Set Fragmentation**: Different block sizes create separate anonymity pools
- **Information Leakage**: Block size patterns could reveal file types or user behavior
- **Cache Complexity**: Need separate randomizer pools for each block size
- **Attack Vectors**: File type fingerprinting and traffic analysis risks

**Proposed Solution Approach:**
- Size classes with privacy protection (32KB, 128KB, 256KB, 512KB)
- Maintain large randomizer pools for each size class
- Add noise to size-based decisions to prevent fingerprinting
- Ensure minimum anonymity set sizes (>1000 randomizers per size class)

**Implementation Status**: Research phase - privacy implications need thorough analysis

### Configuration Options
- Runtime block size configuration through config files
- CLI flags for block size override in advanced use cases
- API improvements for application-specific sizing

## Integration Points

- Uses [Storage](../../storage/CLAUDE.md) for block persistence
- Provides blocks to [Descriptors](../descriptors/CLAUDE.md) for metadata
- Coordinates with [Cache](../../storage/cache/CLAUDE.md) for randomizer selection

## References

See [Global CLAUDE.md](/CLAUDE.md) for system-wide principles.
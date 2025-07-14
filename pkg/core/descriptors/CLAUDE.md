# Core Descriptors Package

## Overview

The descriptors package manages file reconstruction metadata, enabling files to be reassembled from anonymized blocks while preserving privacy.

## Core Concepts

- **Descriptor**: Metadata structure containing block IDs and randomizer mappings
- **Encrypted Storage**: Optional encryption for sensitive descriptors
- **Versioning**: Support for descriptor format evolution

## Implementation Details

### Key Components

- **Descriptor**: Core metadata structure
- **Store**: Descriptor persistence and retrieval
- **EncryptedStore**: Password-protected descriptor storage

### Descriptor Structure

```
Descriptor {
    Version: Format version
    FileSize: Original file size
    BlockSize: Size of each block
    Blocks: [
        {
            Index: Block position
            DataID: Anonymized block ID
            RandomizerID: Randomizer block ID
        }
    ]
    Metadata: Optional file metadata
}
```

### Privacy Considerations

- Descriptors reveal file structure but not content
- Can be shared publicly or kept private
- Support for encrypted descriptors
- Minimal metadata to reduce fingerprinting

## Search and Discovery

- Privacy-preserving search through DHT
- Optional public announcement
- Tag-based discovery system

## Integration Points

- Receives block lists from [Blocks](../blocks/CLAUDE.md)
- Stores descriptors via [Storage](../../storage/CLAUDE.md)
- Announces via [Network](../../network/CLAUDE.md) protocols

## References

See [Global CLAUDE.md](/CLAUDE.md) for system-wide principles.
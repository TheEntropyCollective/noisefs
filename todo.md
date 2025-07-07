# NoiseFS Implementation Plan

## Current Status
-  Basic project structure created
-  Block management (splitting, XOR, assembly)
-  IPFS integration for block storage
-  CLI with upload functionality

## Next Implementation Steps

### 1. Descriptor Management
- [x] Create descriptor data structure in `pkg/descriptors/descriptor.go`
- [x] Implement descriptor serialization/deserialization (JSON format)
- [x] Add descriptor storage to IPFS
- [x] Create descriptor retrieval functionality

### 2. File Download Implementation
- [x] Implement descriptor parsing in CLI
- [x] Add download command to CLI
- [x] Implement file reconstruction from descriptor
- [x] Test round-trip (upload then download)

### 3. Cache Management Foundation
- [x] Create cache interface in `pkg/cache/cache.go`
- [x] Implement basic in-memory cache
- [x] Add cache for popular randomizer blocks
- [x] Integrate cache into upload process

### 4. Improve Block Selection
- [x] Replace random randomizer selection with cache-based selection
- [x] Implement popularity tracking for blocks
- [x] Add block reuse metrics

### 5. Basic Testing
- [ ] Add unit tests for block operations
- [ ] Add integration test for upload/download
- [ ] Create test helper utilities

## Design Decisions
- Start with JSON descriptors for simplicity
- Use in-memory cache initially (can add persistence later)
- Focus on correctness over optimization initially
- Keep changes small and incremental
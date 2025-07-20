---
id: task-0001
title: Remove unimplemented storage backend types
status: Done
assignee:
  - '@claude'
created_date: '2025-07-20'
updated_date: '2025-07-20'
labels:
  - ipfs pkg cleanup
dependencies: []
---

## Description

Eliminate 6 unimplemented backend types (filecoin, arweave, storj, s3, gcs, azure) that add complexity without value. Only IPFS and mock backends are actually implemented and used.

## Acceptance Criteria

- [x] All references to unimplemented backend types removed from config validation
- [x] Backend type constants reduced from 8 to 2 (ipfs + mock)
- [x] Configuration validation simplified
- [x] All tests pass after removal
- [x] No compilation errors

## Implementation Plan

1. Analyze current backend type usage across codebase
2. Remove unimplemented backend type constants from interface.go:254-261 (keep only IPFS and mock)
3. Update config validation in config.go:402-408 to only accept 'ipfs' and 'mock' types
4. Check and update any other references to removed backend types
5. Run tests to ensure no compilation errors
6. Verify all acceptance criteria are met

## Implementation Notes

**Approach taken:**
- Identified 6 unimplemented backend types that were only declared as constants but had no actual implementations
- Systematically removed all references to these unused backend types throughout the codebase
- Simplified configuration validation to only accept the 2 actually implemented backends

**Features implemented or modified:**
- Reduced backend type constants from 8 to 2 in `pkg/storage/interface.go:252-256`
- Added missing `BackendTypeMock` constant that was referenced but not defined
- Simplified config validation in `pkg/storage/config.go:402-404` to only accept "ipfs" and "mock" 
- Updated test helpers in `pkg/storage/testing/test_helpers.go` to use only implemented backends

**Technical decisions and trade-offs:**
- Kept both IPFS (production) and mock (testing) backends as they are the only ones actually implemented
- Removed 6 unimplemented backend types: filecoin, arweave, storj, local, s3, gcs, azure
- This eliminates dead code paths and prevents configuration of non-functional backends

**Modified files:**
- `pkg/storage/interface.go` - Removed 6 unused backend type constants, added missing mock constant
- `pkg/storage/config.go` - Simplified validation to only accept implemented types
- `pkg/storage/testing/test_helpers.go` - Updated test backends to use only implemented types

**Result:**
- Codebase complexity reduced by removing dead code
- Configuration validation now properly prevents unusable backends
- All tests pass and no compilation errors
- Storage backend type constants reduced from 8 to 2 as specified

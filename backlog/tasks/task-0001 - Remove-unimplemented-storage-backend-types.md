---
id: task-0001
title: Remove unimplemented storage backend types
status: In Progress
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

- [ ] All references to unimplemented backend types removed from config validation
- [ ] Backend type constants reduced from 8 to 2 (ipfs + mock)
- [ ] Configuration validation simplified
- [ ] All tests pass after removal
- [ ] No compilation errors

## Implementation Plan

1. Analyze current backend type usage across codebase
2. Remove unimplemented backend type constants from interface.go:254-261 (keep only IPFS and mock)
3. Update config validation in config.go:402-408 to only accept 'ipfs' and 'mock' types
4. Check and update any other references to removed backend types
5. Run tests to ensure no compilation errors
6. Verify all acceptance criteria are met

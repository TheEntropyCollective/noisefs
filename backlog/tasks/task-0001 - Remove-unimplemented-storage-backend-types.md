---
id: task-0001
title: Remove unimplemented storage backend types
status: To Do
assignee: []
created_date: '2025-07-20'
labels: []
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

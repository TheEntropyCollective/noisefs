---
id: task-0005
title: Consolidate storage interfaces
status: To Do
assignee: []
created_date: '2025-07-20'
updated_date: '2025-07-20'
labels:
  - ipfs pkg cleanup
dependencies: []
---

## Description

Simplify the 3 backend interfaces (Backend StreamingBackend PeerAwareBackend) and complex BlockAddress structure. Keep PeerAwareBackend for NoiseFS randomizer functionality but eliminate unused complexity.

## Acceptance Criteria

- [ ] StreamingBackend interface evaluation completed (keep or remove based on future value)
- [ ] BlockAddress simplified to essential fields only
- [ ] Unused interface methods removed
- [ ] Error codes reduced from 13 to 5 essential ones
- [ ] Interface documentation clarified
- [ ] All existing peer-aware functionality preserved
- [ ] All tests pass

---
id: task-0007
title: Remove unused distribution and replication strategies
status: To Do
assignee: []
created_date: '2025-07-20'
updated_date: '2025-07-20'
labels:
  - ipfs pkg cleanup
dependencies: []
---

## Description

Eliminate complex distribution strategies, replication configs, and load balancing algorithms that are not used. Only 'single' strategy is used in practice.

## Acceptance Criteria

- [ ] Unused distribution strategies removed from configuration
- [ ] Replication and geographic diversity configs eliminated
- [ ] Load balancing algorithm complexity reduced
- [ ] Distribution configuration simplified to essential options
- [ ] All existing file operations continue to work
- [ ] All tests pass

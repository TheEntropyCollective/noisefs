---
id: task-0007
title: Remove unused distribution and replication strategies
status: In Progress
assignee:
  - '@jconnuck'
created_date: '2025-07-20'
updated_date: '2025-07-21'
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

## Implementation Plan

1. Remove unused distribution strategy implementations (replicate, stripe, smart) from pkg/storage/router.go
2. Simplify DistributionConfig struct in pkg/storage/config.go - remove ReplicationConfig and simplify LoadBalancingConfig
3. Remove distribution strategy registrations from NewRouter() except for 'single'
4. Update DefaultConfig() to remove unnecessary complexity in distribution settings
5. Simplify or remove unused SelectionConfig and PerformanceCriteria
6. Update validation methods to match simplified structures
7. Run tests to ensure file operations still work correctly
8. Update any configuration examples or documentation

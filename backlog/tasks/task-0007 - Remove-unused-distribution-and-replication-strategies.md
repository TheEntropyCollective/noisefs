---
id: task-0007
title: Remove unused distribution and replication strategies
status: Done
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

- [x] Unused distribution strategies removed from configuration
- [x] Replication and geographic diversity configs eliminated
- [x] Load balancing algorithm complexity reduced
- [x] Distribution configuration simplified to essential options
- [x] All existing file operations continue to work
- [x] All tests pass

## Implementation Plan

1. Remove unused distribution strategy implementations (replicate, stripe, smart) from pkg/storage/router.go
2. Simplify DistributionConfig struct in pkg/storage/config.go - remove ReplicationConfig and simplify LoadBalancingConfig
3. Remove distribution strategy registrations from NewRouter() except for 'single'
4. Update DefaultConfig() to remove unnecessary complexity in distribution settings
5. Simplify or remove unused SelectionConfig and PerformanceCriteria
6. Update validation methods to match simplified structures
7. Run tests to ensure file operations still work correctly
8. Update any configuration examples or documentation

## Implementation Notes

- Removed ReplicationStrategy, StripingStrategy, and SmartDistributionStrategy from router.go
- Updated NewRouter() to only register the 'single' strategy
- Removed ReplicationConfig struct and all references to replication settings
- Simplified DistributionConfig by removing the replication field
- Simplified LoadBalancingConfig by removing StickyBlocks field and limiting algorithm to "performance" only
- Updated DefaultConfig() to remove performance criteria from selection config
- Updated validation methods to only accept 'single' strategy and 'performance' algorithm
- All tests pass - storage package tests run successfully, main binary builds without errors
- File operations continue to work as verified by integration tests

Successfully removed unused distribution strategies (replicate, stripe, smart) from the codebase. Simplified configuration structures by removing ReplicationConfig and simplifying LoadBalancingConfig. The system now only supports the 'single' distribution strategy which is the only one used in practice. All tests pass and file operations continue to work correctly.

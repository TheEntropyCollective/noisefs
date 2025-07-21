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

**Approach taken:**
- Identified 3 unused distribution strategies (replicate, stripe, smart) that were registered but never used in practice
- Systematically removed all implementations and references to these unused strategies throughout the codebase
- Simplified configuration structures to only support the 'single' distribution strategy

**Features implemented or modified:**
- Removed unused strategy implementations from `pkg/storage/router.go:390-489`
  - Deleted ReplicationStrategy struct and methods
  - Deleted StripingStrategy struct and methods  
  - Deleted SmartDistributionStrategy struct and methods
- Updated NewRouter() in `pkg/storage/router.go:29-30` to only register 'single' strategy
- Removed ReplicationConfig struct from `pkg/storage/config.go:118-131`
- Simplified DistributionConfig in `pkg/storage/config.go:103-113` by removing replication field
- Simplified LoadBalancingConfig in `pkg/storage/config.go:140-146` by removing StickyBlocks field
- Updated DefaultConfig() in `pkg/storage/config.go:255-264` to remove performance criteria from selection config
- Updated validation methods in `pkg/storage/config.go:584-604` and `pkg/storage/config.go:649-655`

**Technical decisions and trade-offs:**
- Kept only the 'single' distribution strategy as it's the only one used in production
- Removed geographic diversity and backend diversity features that were never utilized
- Simplified load balancing to only support 'performance' algorithm
- This eliminates dead code paths and reduces system complexity without losing functionality

**Modified files:**
- `pkg/storage/router.go` - Removed 104 lines of unused strategy implementations
- `pkg/storage/config.go` - Removed 64 lines of unused configuration structures
- Total reduction: 168 lines of code

**Result:**
- Codebase complexity significantly reduced by removing dead code
- Configuration validation now prevents unusable distribution strategies
- All tests pass and no compilation errors
- Distribution strategies reduced from 4 to 1 as specified
- System maintains full functionality with cleaner, simpler implementation

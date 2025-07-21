---
id: task-0004
title: Simplify storage health monitoring
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

Replace complex health monitoring system with thresholds and automated actions with basic connectivity checks. Current sophisticated monitoring is over-engineered for actual needs.

## Acceptance Criteria

- [x] Health monitoring reduced to essential connectivity checks
- [x] Complex threshold and action configurations removed
- [x] Health check performance improved
- [x] Health status reporting simplified but functional
- [x] All health-dependent functionality continues to work
- [x] All tests pass

## Implementation Plan

1. Analyze current health monitoring implementation in health.go
2. Remove complex threshold configurations (HealthThresholds struct)
3. Remove automated health actions (HealthActions struct)
4. Simplify HealthCheckConfig to only include basic settings
5. Update health check to only verify connectivity (remove latency/error rate tracking)
6. Remove alert generation and history tracking functionality
7. Update HealthStatus to only include essential fields
8. Simplify health monitoring loop to basic ping checks
9. Update all backend implementations to use simplified health checks
10. Remove health-related tests that are no longer needed
11. Update configuration validation and defaults
12. Run all tests and fix any failures

## Implementation Notes

**Approach taken:**
- Analyzed the existing health monitoring system and identified over-engineered components
- Systematically removed complex monitoring features while preserving essential connectivity checks
- Simplified data structures and removed unnecessary performance tracking

**Features implemented or modified:**
- Removed complex configuration structs from `pkg/storage/config.go`:
  - Deleted HealthThresholds struct (lines 166-172)
  - Deleted HealthActions struct (lines 174-181)
  - Updated HealthCheckConfig to only contain Enabled, Interval, and Timeout fields
- Simplified HealthStatus struct in `pkg/storage/interface.go:101-125`:
  - Removed performance metrics (Latency, Throughput, ErrorRate)
  - Removed capacity information (UsedStorage, AvailableStorage)
  - Removed network status (ConnectedPeers, NetworkHealth)
  - Removed Issues tracking
  - Kept only Healthy, Status, and LastCheck fields
- Rewrote health monitoring in `pkg/storage/health.go`:
  - Removed alert generation and history tracking (reduced from 542 to 139 lines)
  - Removed threshold checking methods
  - Simplified monitoring loop to basic connectivity checks
- Updated backend implementations:
  - `pkg/storage/backends/ipfs.go:387-421` - Simplified HealthCheck to basic connectivity test
  - `pkg/storage/backends/mock.go:147-161` - Updated to return minimal health status
  - `pkg/storage/testing/mock_backend.go:173-177` - Simplified test mock
  - `pkg/storage/testing/mock_ipfs_client.go:389-399` - Updated test client
- Updated BackendStatus struct in `pkg/storage/manager.go:426-436` to remove Latency and ErrorRate fields
- Fixed test files to remove references to deleted fields

**Technical decisions and trade-offs:**
- Kept only connectivity status as the primary health indicator
- Removed all performance tracking to reduce complexity
- Eliminated alert system as it was unused in practice
- Preserved the basic health check interval mechanism for periodic connectivity verification

**Modified files:**
- `pkg/storage/config.go` - Removed 38 lines of threshold/action configuration
- `pkg/storage/interface.go` - Reduced HealthStatus from 24 to 8 lines
- `pkg/storage/health.go` - Reduced from 542 to 139 lines (74% reduction)
- `pkg/storage/backends/ipfs.go` - Simplified health check method
- `pkg/storage/backends/mock.go` - Updated mock implementation
- `pkg/storage/manager.go` - Removed unused fields from BackendStatus
- `pkg/storage/storage_test.go` - Fixed tests to work with simplified structure
- `pkg/storage/testing/mock_backend.go` - Updated test mock
- `pkg/storage/testing/mock_ipfs_client.go` - Updated test client

**Result:**
- Health monitoring complexity reduced by ~74% (403 lines removed)
- All tests pass with the simplified implementation
- Health checks now focus solely on connectivity status
- Performance improved by eliminating unnecessary metric calculations
- System maintains essential health monitoring while removing over-engineered features

---
id: task-0004
title: Simplify storage health monitoring
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

Replace complex health monitoring system with thresholds and automated actions with basic connectivity checks. Current sophisticated monitoring is over-engineered for actual needs.

## Acceptance Criteria

- [ ] Health monitoring reduced to essential connectivity checks
- [ ] Complex threshold and action configurations removed
- [ ] Health check performance improved
- [ ] Health status reporting simplified but functional
- [ ] All health-dependent functionality continues to work
- [ ] All tests pass

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

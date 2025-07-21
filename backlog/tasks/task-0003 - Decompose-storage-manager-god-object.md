---
id: task-0003
title: Decompose storage manager god object
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

Split the 602-line manager.go with 35+ public methods into focused services following single responsibility principle. Current manager handles backends, routing, health monitoring, and error reporting.

## Acceptance Criteria

- [ ] Manager split into focused services (BackendManager HealthManager etc)
- [ ] Each service has single clear responsibility
- [ ] Public API surface reduced and simplified
- [ ] Weird Backend interface delegation removed
- [ ] Manager lifecycle methods remain functional
- [ ] All existing functionality preserved
- [ ] All tests pass

## Implementation Plan

1. Analyze manager.go to identify all responsibilities (COMPLETED)
2. Design service decomposition:
   - BackendRegistry: Manage backend instances and lookups
   - BackendLifecycle: Handle connect/disconnect operations  
   - BackendSelector: Backend selection by criteria (priority, capability, health)
   - StatusAggregator: Collect and aggregate status from all backends
   - ManagerFacade: Thin orchestration layer implementing Backend interface
3. Create new service interfaces and structs
4. Extract BackendRegistry service (store/retrieve backends)
5. Extract BackendLifecycle service (connect/disconnect logic)
6. Extract BackendSelector service (selection algorithms)
7. Extract StatusAggregator service (status collection)
8. Refactor Manager to use new services (facade pattern)
9. Remove weird Backend interface delegation - Manager should not implement Backend
10. Update tests to work with new structure
11. Verify all existing functionality preserved

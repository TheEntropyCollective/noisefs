---
id: task-0003
title: Decompose storage manager god object
status: To Do
assignee: []
created_date: '2025-07-20'
updated_date: '2025-07-20'
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

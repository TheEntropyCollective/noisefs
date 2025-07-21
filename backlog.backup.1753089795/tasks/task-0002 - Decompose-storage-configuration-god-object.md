---
id: task-0002
title: Decompose storage configuration god object
status: In Progress
assignee:
  - '@agent1'
created_date: '2025-07-20'
updated_date: '2025-07-21'
labels:
  - ipfs pkg cleanup
dependencies: []
---

## Description

Split the 811-line config.go file with 20+ struct types into focused, maintainable modules. Current god object handles auth, TLS, distribution, health, performance, caching, and compression in one file.

## Acceptance Criteria

- [ ] config.go split into 3-4 focused files (basic_config.go connection_config.go health_config.go)
- [ ] Each new file has single responsibility
- [ ] Total lines reduced by removing unused configurations
- [ ] All configuration validation consolidated and simplified
- [ ] Existing configuration loading continues to work
- [ ] All tests pass

## Implementation Plan

1. Analyze current config.go structure and identify logical groupings\n2. Create new config files: basic_config.go, connection_config.go, health_config.go\n3. Move related structs and methods to appropriate files\n4. Remove unused configuration options\n5. Consolidate and simplify validation logic\n6. Update imports in dependent files\n7. Run and fix tests

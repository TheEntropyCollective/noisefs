---
id: task-0002
title: Decompose storage configuration god object
status: To Do
assignee: []
created_date: '2025-07-20'
labels: []
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

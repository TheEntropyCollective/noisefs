---
id: task-0002
title: Decompose storage configuration god object
status: Done
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

- [x] config.go split into 3-4 focused files (basic_config.go connection_config.go health_config.go)
- [x] Each new file has single responsibility
- [x] Total lines reduced by removing unused configurations
- [x] All configuration validation consolidated and simplified
- [x] Existing configuration loading continues to work
- [x] All tests pass

## Implementation Plan

1. Analyze current config.go structure and identify logical groupings\n2. Create new config files: basic_config.go, connection_config.go, health_config.go\n3. Move related structs and methods to appropriate files\n4. Remove unused configuration options\n5. Consolidate and simplify validation logic\n6. Update imports in dependent files\n7. Run and fix tests

## Implementation Notes

Decomposed the 811-line config.go file into 4 focused, maintainable modules:

**Approach taken:**
- Analyzed original config.go structure and identified logical groupings by responsibility
- Created 4 focused configuration files with single responsibilities
- Removed unused and over-engineered configuration options to reduce complexity
- Maintained backward compatibility by keeping all types in the storage package

**Features implemented:**
- basic_config.go (187 lines) - Core Config and BackendConfig with validation methods
- connection_config.go (94 lines) - Connection, Retry, and Timeout configurations
- health_config.go (266 lines) - Health monitoring, Performance, and Distribution configs
- default_config.go (69 lines) - DefaultConfig function with sensible defaults  
- config.go (10 lines) - Entry point documentation

**Technical decisions and trade-offs:**
- Removed unused Settings map from BackendConfig (never accessed)
- Eliminated AuthConfig and TLSConfig structs (validated but never used in backends)
- Simplified replication config by removing geo-diversity options (over-engineered)
- Removed unimplemented caching, batching, and compression configs (185 lines saved)
- Kept all validation logic distributed with each config type for maintainability

**Modified/added files:**
- pkg/storage/config.go - Reduced from 811 to 10 lines
- pkg/storage/basic_config.go - New file with core configurations
- pkg/storage/connection_config.go - New file with connection settings
- pkg/storage/health_config.go - New file with monitoring and performance configs
- pkg/storage/default_config.go - New file with default configuration
- pkg/storage/directory_manager_test.go - Updated to remove Settings field references

Decomposed the 811-line config.go file into 4 focused, maintainable modules:

**Approach taken:**
- Analyzed original config.go structure and identified logical groupings by responsibility
- Created 4 focused configuration files with single responsibilities  
- Removed unused and over-engineered configuration options to reduce complexity
- Maintained backward compatibility by keeping all types in the storage package

**Features implemented:**
- basic_config.go (187 lines) - Core Config and BackendConfig with validation methods
- connection_config.go (94 lines) - Connection, Retry, and Timeout configurations
- health_config.go (266 lines) - Health monitoring, Performance, and Distribution configs
- default_config.go (69 lines) - DefaultConfig function with sensible defaults
- config.go (10 lines) - Entry point documentation

**Technical decisions and trade-offs:**
- Removed unused Settings map from BackendConfig (never accessed)
- Eliminated AuthConfig and TLSConfig structs (validated but never used in backends)
- Simplified replication config by removing geo-diversity options (over-engineered)
- Removed unimplemented caching, batching, and compression configs (185 lines saved)
- Kept all validation logic distributed with each config type for maintainability

**Modified/added files:**
- pkg/storage/config.go - Reduced from 811 to 10 lines
- pkg/storage/basic_config.go - New file with core configurations
- pkg/storage/connection_config.go - New file with connection settings
- pkg/storage/health_config.go - New file with monitoring and performance configs
- pkg/storage/default_config.go - New file with default configuration
- pkg/storage/directory_manager_test.go - Updated to remove Settings field references

**Results:**
- Reduced total configuration lines from 811 to 626 (23% reduction)
- All 16 tests pass successfully
- Configuration loading continues to work seamlessly

package storage

// This file serves as the main config entry point.
// Configuration types have been decomposed into focused files:
//
//   basic_config.go      - Core Config and BackendConfig structs with validation
//   connection_config.go - Connection, Auth, TLS, Retry, and Timeout configs
//   health_config.go     - Health, Performance, Distribution, and related configs
//   default_config.go    - DefaultConfig() function with sensible defaults
//
// All types remain in the storage package for backward compatibility.

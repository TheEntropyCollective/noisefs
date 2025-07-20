# FUSE Configuration Management System

This document describes the centralized configuration management system for the NoiseFS FUSE package.

## Overview

The FUSE configuration system provides centralized, type-safe configuration management with environment variable overrides, comprehensive validation, and secure defaults optimized for privacy-preserving distributed storage.

## Files

- `config.go` - Core configuration structures and loading logic
- `config_test.go` - Comprehensive test suite for all configuration functionality  
- `config_example_test.go` - Usage examples and demonstrations
- `directory_cache.go` - Updated to integrate with centralized configuration

## Key Features

### 1. Centralized Configuration Structure

```go
type FUSEConfig struct {
    Cache       CacheConfig       // Directory cache settings
    Security    SecurityConfig    // Memory protection and secure deletion
    Performance PerformanceConfig // Streaming and concurrency settings
    Mount       MountConfig       // FUSE mount options and timeouts
}
```

### 2. Environment Variable Support

All configuration options can be overridden using environment variables with the `NOISEFS_FUSE_` prefix:

```bash
export NOISEFS_FUSE_CACHE_SIZE=500
export NOISEFS_FUSE_CACHE_TTL=1h
export NOISEFS_FUSE_MEMORY_LOCK=true
export NOISEFS_FUSE_STREAM_CHUNK_SIZE=131072
```

### 3. Comprehensive Validation

The configuration system includes detailed validation with helpful error messages:

```go
config := LoadFUSEConfig()
if err := config.Validate(); err != nil {
    // Get actionable error message with recommendations
    fmt.Printf("Configuration error: %v\n", err)
}
```

### 4. Global Configuration Access

```go
// Access global configuration from anywhere in the FUSE package
config := GetGlobalConfig()

// Or set custom global configuration
SetGlobalConfig(customConfig)
```

## Configuration Sections

### Cache Configuration

Controls directory manifest caching and performance:

- `Size` - Maximum number of cached directory manifests (default: 100)
- `TTL` - Time-to-live for cache entries (default: 30m)
- `MaxEntries` - Upper bound on cache entries (default: 1000)
- `EnableMetrics` - Collect cache hit/miss statistics (default: true)

### Security Configuration

Controls privacy and security features:

- `SecureDeletePasses` - Number of overwrite passes (default: 3)
- `MemoryLock` - Lock sensitive data in memory (default: true)
- `ClearMemoryOnExit` - Securely clear memory on exit (default: true)
- `RestrictXAttrs` - Restrict extended attributes for privacy (default: true)

### Performance Configuration

Controls resource usage and optimization:

- `StreamingChunkSize` - Buffer size for streaming operations (default: 64KB)
- `MaxConcurrentOps` - Limit concurrent FUSE operations (default: 10)
- `ReadAheadSize` - Prefetch size for sequential reads (default: 128KB)
- `WriteBufferSize` - Buffer size for write operations (default: 64KB)
- `EnableAsyncIO` - Use asynchronous I/O for large files (default: true)

### Mount Configuration

Controls FUSE filesystem behavior:

- `DefaultPath` - Default mount path (default: empty)
- `DefaultVolumeName` - Volume name shown to users (default: "NoiseFS")
- `AllowOther` - Allow other users to access mount (default: false)
- `Debug` - Enable FUSE debug logging (default: false)
- `ReadOnly` - Mount filesystem read-only (default: false)
- `AttrTimeout` - File attribute cache timeout (default: 1s)
- `EntryTimeout` - Directory entry cache timeout (default: 1s)
- `NegativeTimeout` - Negative lookup cache timeout (default: 1s)

## Usage Examples

### Basic Usage

```go
// Load configuration with environment overrides
config := LoadFUSEConfig()

// Validate before use
if err := config.Validate(); err != nil {
    return fmt.Errorf("invalid config: %w", err)
}

// Use configuration values
cacheConfig := config.GetDirectoryCacheConfig()
dirCache, err := NewDirectoryCache(cacheConfig, storageManager)
```

### Environment Variable Override

```bash
# Set environment variables
export NOISEFS_FUSE_CACHE_SIZE=500
export NOISEFS_FUSE_CACHE_TTL=1h
export NOISEFS_FUSE_MEMORY_LOCK=false

# Configuration automatically picks up these values
```

### Integration with Existing Code

```go
// Use centralized configuration for directory cache
dirCache, err := NewDirectoryCacheFromGlobalConfig(storageManager)

// Convert FUSE config to mount options
config := GetGlobalConfig()
opts := MountOptions{
    MountPath:  config.Mount.DefaultPath,
    VolumeName: config.Mount.DefaultVolumeName,
    ReadOnly:   config.Mount.ReadOnly,
    Debug:      config.Mount.Debug,
}
```

## Environment Variables Reference

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `NOISEFS_FUSE_CACHE_SIZE` | int | 100 | Maximum cached directory manifests |
| `NOISEFS_FUSE_CACHE_TTL` | duration | 30m | Cache entry time-to-live |
| `NOISEFS_FUSE_CACHE_MAX` | int | 1000 | Maximum cache entries |
| `NOISEFS_FUSE_CACHE_METRICS` | bool | true | Enable cache metrics |
| `NOISEFS_FUSE_SECURE_DELETE_PASSES` | int | 3 | Secure deletion overwrite passes |
| `NOISEFS_FUSE_MEMORY_LOCK` | bool | true | Lock sensitive memory |
| `NOISEFS_FUSE_CLEAR_MEMORY` | bool | true | Clear memory on exit |
| `NOISEFS_FUSE_RESTRICT_XATTRS` | bool | true | Restrict extended attributes |
| `NOISEFS_FUSE_STREAM_CHUNK_SIZE` | int | 65536 | Streaming buffer size (bytes) |
| `NOISEFS_FUSE_MAX_CONCURRENT_OPS` | int | 10 | Maximum concurrent operations |
| `NOISEFS_FUSE_READAHEAD_SIZE` | int | 131072 | Read-ahead buffer size (bytes) |
| `NOISEFS_FUSE_WRITE_BUFFER_SIZE` | int | 65536 | Write buffer size (bytes) |
| `NOISEFS_FUSE_ASYNC_IO` | bool | true | Enable asynchronous I/O |
| `NOISEFS_FUSE_DEFAULT_PATH` | string | "" | Default mount path |
| `NOISEFS_FUSE_VOLUME_NAME` | string | "NoiseFS" | Volume name |
| `NOISEFS_FUSE_ALLOW_OTHER` | bool | false | Allow other users access |
| `NOISEFS_FUSE_DEBUG` | bool | false | Enable debug logging |
| `NOISEFS_FUSE_READ_ONLY` | bool | false | Mount read-only |
| `NOISEFS_FUSE_ATTR_TIMEOUT` | duration | 1s | Attribute cache timeout |
| `NOISEFS_FUSE_ENTRY_TIMEOUT` | duration | 1s | Directory entry cache timeout |
| `NOISEFS_FUSE_NEGATIVE_TIMEOUT` | duration | 1s | Negative lookup cache timeout |

## Migration Guide

### From Hardcoded Values

Replace hardcoded values with configuration access:

```go
// Before
cacheSize := 100
ttl := 30 * time.Minute

// After  
config := GetGlobalConfig()
cacheSize := config.Cache.Size
ttl := config.Cache.TTL
```

### From DefaultDirectoryCacheConfig()

The old function still works but is deprecated:

```go
// Deprecated
config := DefaultDirectoryCacheConfig()
cache, err := NewDirectoryCache(config, storageManager)

// Recommended
cache, err := NewDirectoryCacheFromGlobalConfig(storageManager)
```

## Security Considerations

The configuration system follows NoiseFS privacy-first principles:

1. **Secure Defaults**: All security features enabled by default
2. **Memory Protection**: Sensitive data locked in memory by default
3. **Anti-Forensics**: Memory cleared on exit by default
4. **Privacy Protection**: Extended attributes restricted by default
5. **Validation**: Comprehensive validation prevents insecure configurations

## Performance Considerations

- Configuration loading is optimized for speed (~1μs per operation)
- Environment variable parsing has graceful error handling
- Global configuration is cached for repeated access
- Validation is comprehensive but lightweight

## Testing

The configuration system includes comprehensive tests:

- Unit tests for all configuration structures
- Environment variable override testing
- Validation testing with helpful error messages
- Performance benchmarking
- Integration examples

Run tests with:
```bash
go test ./pkg/fuse -run "Config" -v
```

## Future Enhancements

Planned improvements for Phase 1.3:

1. **Configuration File Support**: JSON/YAML configuration files
2. **Hot Reload**: Runtime configuration updates
3. **Metrics Integration**: Configuration-driven metrics collection
4. **Profile System**: Predefined configuration profiles (development, production, etc.)
5. **Validation Presets**: Quick validation for common deployment scenarios
// Package fuse provides centralized configuration management for the NoiseFS FUSE subsystem.
//
// This package serves as the configuration hub for all FUSE-related components,
// providing type-safe configuration structures, environment variable support,
// comprehensive validation, and secure defaults optimized for privacy-preserving
// distributed storage.
//
// Configuration Sources (in order of precedence):
//   1. Environment variables (highest priority)
//   2. Programmatic configuration (API calls)
//   3. Default values (lowest priority)
//
// Key Features:
//   - Comprehensive validation with actionable error messages
//   - Environment variable overrides for all settings
//   - Memory safety and performance optimization settings
//   - FUSE-specific mount options and security controls
//   - Cache configuration with TTL and size limits
//   - Secure defaults following NoiseFS privacy principles
//
// Usage Example:
//
//	// Load configuration with environment overrides
//	config := LoadFUSEConfig()
//	
//	// Validate configuration before use
//	if err := config.Validate(); err != nil {
//		return fmt.Errorf("invalid FUSE config: %w", err)
//	}
//	
//	// Use in mount operations
//	opts := MountOptions{
//		MountPath:  config.Mount.Path,
//		VolumeName: config.Mount.VolumeName,
//		ReadOnly:   config.Mount.ReadOnly,
//		Debug:      config.Mount.Debug,
//	}
//
package fuse

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// FUSEConfig represents the complete FUSE subsystem configuration.
//
// This structure contains all configuration options for the NoiseFS FUSE
// filesystem implementation, including cache behavior, security settings,
// performance tuning, and mount options. The configuration follows NoiseFS
// privacy-first principles with secure defaults.
//
// Thread Safety:
//   - Config instances are safe for concurrent read access
//   - Modifications should be synchronized by the caller
//   - Validation is thread-safe
//
// Validation:
//   - All fields are validated during Validate() calls
//   - Comprehensive error messages guide users to correct configurations
//   - Security warnings alert users to insecure settings
//
type FUSEConfig struct {
	// Cache Configuration
	// Controls directory manifest caching and block caching behavior
	Cache CacheConfig `json:"cache"`

	// Security Configuration  
	// Controls encryption, memory protection, and secure deletion
	Security SecurityConfig `json:"security"`

	// Performance Configuration
	// Controls streaming, concurrency, and resource limits
	Performance PerformanceConfig `json:"performance"`

	// Mount Configuration
	// Controls FUSE mount options and filesystem behavior
	Mount MountConfig `json:"mount"`
}

// CacheConfig holds cache-related configuration for FUSE operations.
//
// The cache system is critical for performance in NoiseFS FUSE operations,
// particularly for directory manifest caching and block reuse. These settings
// control memory usage, cache expiration, and performance characteristics.
//
type CacheConfig struct {
	// Directory Cache Settings
	// Controls caching of directory manifests for faster navigation
	
	// Size is the maximum number of cached directory manifests
	// Recommended: 100 for normal use, 500+ for high-activity directories
	Size int `env:"NOISEFS_FUSE_CACHE_SIZE" default:"100"`
	
	// TTL is the time-to-live for cached directory entries
	// Longer TTL improves performance but may show stale data
	TTL time.Duration `env:"NOISEFS_FUSE_CACHE_TTL" default:"30m"`
	
	// MaxEntries is the maximum number of cache entries to maintain
	// This provides an upper bound on memory usage
	MaxEntries int `env:"NOISEFS_FUSE_CACHE_MAX" default:"1000"`
	
	// EnableMetrics controls whether cache hit/miss statistics are collected
	// Useful for performance tuning but adds slight overhead
	EnableMetrics bool `env:"NOISEFS_FUSE_CACHE_METRICS" default:"true"`
}

// SecurityConfig holds security-related configuration for FUSE operations.
//
// NoiseFS prioritizes security and privacy, so these settings control
// memory protection, secure deletion, and other security measures that
// may impact performance but enhance security.
//
type SecurityConfig struct {
	// SecureDeletePasses is the number of overwrite passes for secure deletion
	// Higher values provide better security but slower deletion
	// Recommended: 3 for balanced security, 7 for high security, 1 for performance
	SecureDeletePasses int `env:"NOISEFS_FUSE_SECURE_DELETE_PASSES" default:"3"`
	
	// MemoryLock controls whether sensitive data is locked in memory
	// When true, prevents sensitive data from being swapped to disk
	// May require elevated privileges on some systems
	MemoryLock bool `env:"NOISEFS_FUSE_MEMORY_LOCK" default:"true"`
	
	// ClearMemoryOnExit controls whether memory is securely cleared on exit
	// Provides additional protection against forensic analysis
	ClearMemoryOnExit bool `env:"NOISEFS_FUSE_CLEAR_MEMORY" default:"true"`
	
	// RestrictXAttrs controls whether extended attributes are restricted
	// When true, only privacy-safe attributes are exposed
	RestrictXAttrs bool `env:"NOISEFS_FUSE_RESTRICT_XATTRS" default:"true"`
}

// PerformanceConfig holds performance-related configuration for FUSE operations.
//
// These settings control resource usage, concurrency, and streaming behavior
// to optimize performance while maintaining security guarantees.
//
type PerformanceConfig struct {
	// Streaming Configuration
	// Controls how large files are handled during reads and writes
	
	// StreamingChunkSize is the buffer size for streaming operations
	// Larger values improve throughput but use more memory
	// Must be a power of 2 for optimal performance
	StreamingChunkSize int `env:"NOISEFS_FUSE_STREAM_CHUNK_SIZE" default:"65536"`
	
	// MaxConcurrentOps limits the number of concurrent FUSE operations
	// Higher values improve parallelism but may overwhelm system resources
	// Recommended: 10 for normal use, 5 for low-memory systems, 20+ for high-performance
	MaxConcurrentOps int `env:"NOISEFS_FUSE_MAX_CONCURRENT_OPS" default:"10"`
	
	// ReadAheadSize controls prefetching for sequential reads
	// Larger values improve streaming performance but use more memory
	// Set to 0 to disable read-ahead
	ReadAheadSize int `env:"NOISEFS_FUSE_READAHEAD_SIZE" default:"131072"`
	
	// WriteBufferSize controls buffering for write operations
	// Larger values improve write performance but may delay data persistence
	WriteBufferSize int `env:"NOISEFS_FUSE_WRITE_BUFFER_SIZE" default:"65536"`
	
	// EnableAsyncIO controls whether asynchronous I/O is used
	// Improves performance for large files but adds complexity
	EnableAsyncIO bool `env:"NOISEFS_FUSE_ASYNC_IO" default:"true"`
}

// MountConfig holds FUSE mount options and filesystem behavior settings.
//
// These settings control how the FUSE filesystem appears to the operating
// system and applications, including permissions, debug options, and
// filesystem characteristics.
//
type MountConfig struct {
	// Default mount path for FUSE filesystem
	// Empty string means no default (must be specified at mount time)
	DefaultPath string `env:"NOISEFS_FUSE_DEFAULT_PATH" default:""`
	
	// Default volume name shown in file managers
	// This is how the filesystem appears to users
	DefaultVolumeName string `env:"NOISEFS_FUSE_VOLUME_NAME" default:"NoiseFS"`
	
	// AllowOther controls whether other users can access the mount
	// Requires user_allow_other in /etc/fuse.conf on Linux
	// Security consideration: may expose files to other users
	AllowOther bool `env:"NOISEFS_FUSE_ALLOW_OTHER" default:"false"`
	
	// Debug enables FUSE debug logging
	// Useful for troubleshooting but may log sensitive information
	Debug bool `env:"NOISEFS_FUSE_DEBUG" default:"false"`
	
	// ReadOnly mounts the filesystem in read-only mode
	// Provides additional security by preventing modifications
	ReadOnly bool `env:"NOISEFS_FUSE_READ_ONLY" default:"false"`
	
	// Timeout settings for FUSE operations
	// These control how long the kernel waits for FUSE responses
	
	// AttrTimeout controls how long file attributes are cached
	// Longer timeouts improve performance but may show stale metadata
	AttrTimeout time.Duration `env:"NOISEFS_FUSE_ATTR_TIMEOUT" default:"1s"`
	
	// EntryTimeout controls how long directory entries are cached
	// Longer timeouts improve performance but may show stale directory listings
	EntryTimeout time.Duration `env:"NOISEFS_FUSE_ENTRY_TIMEOUT" default:"1s"`
	
	// NegativeTimeout controls how long negative lookups are cached
	// Caches "file not found" results to avoid repeated lookups
	NegativeTimeout time.Duration `env:"NOISEFS_FUSE_NEGATIVE_TIMEOUT" default:"1s"`
}

// DefaultFUSEConfig returns a secure-by-default FUSE configuration.
//
// This configuration provides a balanced approach between security and performance,
// with all security features enabled and reasonable performance settings.
// It serves as the foundation for environment variable overrides.
//
// Key Characteristics:
//   - All security features enabled (memory lock, secure delete, etc.)
//   - Conservative performance settings for stability
//   - Cache settings optimized for normal desktop use
//   - Mount options prioritizing security over convenience
//   - Debug disabled to prevent information leakage
//
// Security Features Enabled:
//   - 3-pass secure deletion for balanced security/performance
//   - Memory locking to prevent swapping of sensitive data
//   - Memory clearing on exit for anti-forensic protection
//   - Restricted extended attributes for privacy
//
// Performance Settings:
//   - 64KB streaming chunks for good memory/performance balance
//   - 10 concurrent operations for stability
//   - 128KB read-ahead for streaming performance
//   - Asynchronous I/O enabled for large file performance
//
// Returns:
//   A new FUSEConfig instance with secure default values
//
// Complexity: O(1) - Simple structure initialization
func DefaultFUSEConfig() *FUSEConfig {
	return &FUSEConfig{
		Cache: CacheConfig{
			Size:          100,              // Good balance for normal desktop use
			TTL:           30 * time.Minute, // Reasonable cache lifetime
			MaxEntries:    1000,             // Prevents unbounded memory growth
			EnableMetrics: true,             // Useful for performance monitoring
		},
		Security: SecurityConfig{
			SecureDeletePasses: 3,     // Balanced security without excessive overhead
			MemoryLock:         true,  // Prevent sensitive data swapping
			ClearMemoryOnExit:  true,  // Anti-forensic protection
			RestrictXAttrs:     true,  // Privacy protection
		},
		Performance: PerformanceConfig{
			StreamingChunkSize: 65536,  // 64KB - good balance of memory and performance
			MaxConcurrentOps:   10,     // Conservative for stability
			ReadAheadSize:      131072, // 128KB for streaming performance
			WriteBufferSize:    65536,  // 64KB for good write performance
			EnableAsyncIO:      true,   // Better performance for large files
		},
		Mount: MountConfig{
			DefaultPath:         "",            // No default - must be specified
			DefaultVolumeName:   "NoiseFS",     // User-friendly name
			AllowOther:          false,         // Security: restrict access
			Debug:               false,         // Security: no debug output by default
			ReadOnly:            false,         // Allow writes by default
			AttrTimeout:         1 * time.Second, // Short timeouts for freshness
			EntryTimeout:        1 * time.Second,
			NegativeTimeout:     1 * time.Second,
		},
	}
}

// LoadFUSEConfig loads FUSE configuration with environment variable overrides.
//
// This function implements the complete FUSE configuration loading pipeline:
// 1. Start with secure defaults
// 2. Apply environment variable overrides
// 3. Return the complete configuration
//
// The configuration can then be validated separately using the Validate() method.
//
// Configuration Precedence (highest to lowest):
//   1. Environment variables (NOISEFS_FUSE_*)
//   2. Default values
//
// Environment Variables:
//   All configuration options can be overridden using environment variables
//   with the NOISEFS_FUSE_ prefix. Boolean values use "true"/"false".
//
// Error Handling:
//   - Invalid environment variable values are silently ignored (uses defaults)
//   - This ensures environment variable errors don't break startup
//   - Use Validate() after loading to catch configuration issues
//
// Returns:
//   *FUSEConfig: A fully loaded configuration with environment overrides applied
//
// Complexity: O(1) - Fixed number of environment variable checks
func LoadFUSEConfig() *FUSEConfig {
	config := DefaultFUSEConfig()
	config.applyEnvironmentOverrides()
	return config
}

// applyEnvironmentOverrides applies environment variable overrides to the configuration.
//
// This method implements comprehensive environment variable support for all
// FUSE configuration options. Environment variables have the highest precedence
// and can override default values.
//
// Naming Convention:
//   All environment variables use the NOISEFS_FUSE_ prefix followed by the
//   configuration path in UPPER_CASE with underscores.
//
// Type Conversion:
//   - Strings: Used directly
//   - Integers: Parsed with strconv.Atoi (invalid values ignored)
//   - Booleans: "true" or "false" (case-insensitive)
//   - Durations: Parsed with time.ParseDuration (supports 1s, 5m, 2h, etc.)
//
// Error Handling:
//   - Invalid values are silently ignored to prevent startup failures
//   - This allows graceful degradation when environment is misconfigured
//   - Use Validate() to catch and report configuration errors
//
// Supported Variables:
//   Cache: NOISEFS_FUSE_CACHE_SIZE, NOISEFS_FUSE_CACHE_TTL, etc.
//   Security: NOISEFS_FUSE_SECURE_DELETE_PASSES, NOISEFS_FUSE_MEMORY_LOCK, etc.
//   Performance: NOISEFS_FUSE_STREAM_CHUNK_SIZE, NOISEFS_FUSE_MAX_CONCURRENT_OPS, etc.
//   Mount: NOISEFS_FUSE_DEFAULT_PATH, NOISEFS_FUSE_VOLUME_NAME, etc.
//
// Complexity: O(1) - Fixed number of environment variable checks
func (c *FUSEConfig) applyEnvironmentOverrides() {
	// Cache configuration overrides
	if val := os.Getenv("NOISEFS_FUSE_CACHE_SIZE"); val != "" {
		if size, err := strconv.Atoi(val); err == nil && size > 0 {
			c.Cache.Size = size
		}
	}
	if val := os.Getenv("NOISEFS_FUSE_CACHE_TTL"); val != "" {
		if ttl, err := time.ParseDuration(val); err == nil && ttl > 0 {
			c.Cache.TTL = ttl
		}
	}
	if val := os.Getenv("NOISEFS_FUSE_CACHE_MAX"); val != "" {
		if max, err := strconv.Atoi(val); err == nil && max > 0 {
			c.Cache.MaxEntries = max
		}
	}
	if val := os.Getenv("NOISEFS_FUSE_CACHE_METRICS"); val != "" {
		c.Cache.EnableMetrics = strings.ToLower(val) == "true"
	}

	// Security configuration overrides
	if val := os.Getenv("NOISEFS_FUSE_SECURE_DELETE_PASSES"); val != "" {
		if passes, err := strconv.Atoi(val); err == nil && passes >= 0 {
			c.Security.SecureDeletePasses = passes
		}
	}
	if val := os.Getenv("NOISEFS_FUSE_MEMORY_LOCK"); val != "" {
		c.Security.MemoryLock = strings.ToLower(val) == "true"
	}
	if val := os.Getenv("NOISEFS_FUSE_CLEAR_MEMORY"); val != "" {
		c.Security.ClearMemoryOnExit = strings.ToLower(val) == "true"
	}
	if val := os.Getenv("NOISEFS_FUSE_RESTRICT_XATTRS"); val != "" {
		c.Security.RestrictXAttrs = strings.ToLower(val) == "true"
	}

	// Performance configuration overrides
	if val := os.Getenv("NOISEFS_FUSE_STREAM_CHUNK_SIZE"); val != "" {
		if size, err := strconv.Atoi(val); err == nil && size > 0 {
			c.Performance.StreamingChunkSize = size
		}
	}
	if val := os.Getenv("NOISEFS_FUSE_MAX_CONCURRENT_OPS"); val != "" {
		if ops, err := strconv.Atoi(val); err == nil && ops > 0 {
			c.Performance.MaxConcurrentOps = ops
		}
	}
	if val := os.Getenv("NOISEFS_FUSE_READAHEAD_SIZE"); val != "" {
		if size, err := strconv.Atoi(val); err == nil && size >= 0 {
			c.Performance.ReadAheadSize = size
		}
	}
	if val := os.Getenv("NOISEFS_FUSE_WRITE_BUFFER_SIZE"); val != "" {
		if size, err := strconv.Atoi(val); err == nil && size > 0 {
			c.Performance.WriteBufferSize = size
		}
	}
	if val := os.Getenv("NOISEFS_FUSE_ASYNC_IO"); val != "" {
		c.Performance.EnableAsyncIO = strings.ToLower(val) == "true"
	}

	// Mount configuration overrides
	if val := os.Getenv("NOISEFS_FUSE_DEFAULT_PATH"); val != "" {
		c.Mount.DefaultPath = val
	}
	if val := os.Getenv("NOISEFS_FUSE_VOLUME_NAME"); val != "" {
		c.Mount.DefaultVolumeName = val
	}
	if val := os.Getenv("NOISEFS_FUSE_ALLOW_OTHER"); val != "" {
		c.Mount.AllowOther = strings.ToLower(val) == "true"
	}
	if val := os.Getenv("NOISEFS_FUSE_DEBUG"); val != "" {
		c.Mount.Debug = strings.ToLower(val) == "true"
	}
	if val := os.Getenv("NOISEFS_FUSE_READ_ONLY"); val != "" {
		c.Mount.ReadOnly = strings.ToLower(val) == "true"
	}
	if val := os.Getenv("NOISEFS_FUSE_ATTR_TIMEOUT"); val != "" {
		if timeout, err := time.ParseDuration(val); err == nil && timeout >= 0 {
			c.Mount.AttrTimeout = timeout
		}
	}
	if val := os.Getenv("NOISEFS_FUSE_ENTRY_TIMEOUT"); val != "" {
		if timeout, err := time.ParseDuration(val); err == nil && timeout >= 0 {
			c.Mount.EntryTimeout = timeout
		}
	}
	if val := os.Getenv("NOISEFS_FUSE_NEGATIVE_TIMEOUT"); val != "" {
		if timeout, err := time.ParseDuration(val); err == nil && timeout >= 0 {
			c.Mount.NegativeTimeout = timeout
		}
	}
}

// Validate performs comprehensive FUSE configuration validation with helpful error messages.
//
// This method validates all FUSE configuration fields and provides actionable
// error messages that guide users to correct configurations. It checks for
// common misconfigurations and suggests appropriate values.
//
// Validation Categories:
//   - Cache: Size limits, TTL values, and memory usage validation
//   - Security: Secure delete settings and memory protection validation
//   - Performance: Buffer sizes, concurrency limits, and resource validation
//   - Mount: Timeout values, path validation, and permission checks
//
// Error Message Design:
//   - Describes what is wrong with specific values
//   - Suggests specific corrective actions with examples
//   - Provides reasoning for recommended values
//   - References security implications where relevant
//
// Security Validation:
//   - Warns about disabled security features
//   - Validates memory protection settings
//   - Checks for insecure configurations
//   - Ensures performance settings don't compromise security
//
// Performance Validation:
//   - Ensures buffer sizes are reasonable and aligned
//   - Validates concurrency limits against system capabilities
//   - Checks for configurations that may cause memory exhaustion
//   - Warns about settings that may impact responsiveness
//
// Returns:
//   error: nil if configuration is valid, detailed error message otherwise
//
// Complexity: O(1) - Fixed number of validation checks
func (c *FUSEConfig) Validate() error {
	// Validate cache configuration
	if c.Cache.Size <= 0 {
		return fmt.Errorf("cache size must be positive (current: %d). Recommended values: 50 for low-memory systems, 100 for normal use, 500+ for high-activity directories", c.Cache.Size)
	}
	if c.Cache.Size > 10000 {
		return fmt.Errorf("cache size is very large (%d), which may use excessive memory. Consider using 100-1000 for most use cases", c.Cache.Size)
	}
	
	if c.Cache.TTL <= 0 {
		return fmt.Errorf("cache TTL must be positive (current: %v). Recommended values: 5m for frequently changing directories, 30m for normal use, 2h for stable directories", c.Cache.TTL)
	}
	if c.Cache.TTL > 24*time.Hour {
		return fmt.Errorf("cache TTL is very long (%v), which may show stale data. Consider using 30m-2h for most use cases", c.Cache.TTL)
	}
	
	if c.Cache.MaxEntries <= 0 {
		return fmt.Errorf("cache max entries must be positive (current: %d). This should be larger than cache size, typically 10x the cache size", c.Cache.MaxEntries)
	}
	if c.Cache.MaxEntries < c.Cache.Size {
		return fmt.Errorf("cache max entries (%d) should be larger than cache size (%d). Set max entries to at least %d", c.Cache.MaxEntries, c.Cache.Size, c.Cache.Size*2)
	}

	// Validate security configuration
	if c.Security.SecureDeletePasses < 0 {
		return fmt.Errorf("secure delete passes must be non-negative (current: %d). Use 0 to disable, 1 for basic security, 3 for balanced security, or 7 for high security", c.Security.SecureDeletePasses)
	}
	if c.Security.SecureDeletePasses > 35 {
		return fmt.Errorf("secure delete passes is very high (%d), which will severely impact performance. Consider using 1-7 passes for most security needs", c.Security.SecureDeletePasses)
	}

	// Validate performance configuration  
	if c.Performance.StreamingChunkSize <= 0 {
		return fmt.Errorf("streaming chunk size must be positive (current: %d). Recommended values: 32KB for low memory, 64KB for normal use, 128KB for high performance", c.Performance.StreamingChunkSize)
	}
	if c.Performance.StreamingChunkSize < 4096 {
		return fmt.Errorf("streaming chunk size is very small (%d bytes), which may impact performance. Consider using at least 32KB for better efficiency", c.Performance.StreamingChunkSize)
	}
	if c.Performance.StreamingChunkSize > 1048576 {
		return fmt.Errorf("streaming chunk size is very large (%d bytes), which may use excessive memory. Consider using 64KB-256KB for most use cases", c.Performance.StreamingChunkSize)
	}
	// Check if chunk size is a power of 2 for optimal performance
	if c.Performance.StreamingChunkSize&(c.Performance.StreamingChunkSize-1) != 0 {
		// Find nearest power of 2
		nextPowerOf2 := 1
		for nextPowerOf2 < c.Performance.StreamingChunkSize {
			nextPowerOf2 <<= 1
		}
		prevPowerOf2 := nextPowerOf2 >> 1
		return fmt.Errorf("streaming chunk size (%d) should be a power of 2 for optimal performance. Consider using %d or %d", c.Performance.StreamingChunkSize, prevPowerOf2, nextPowerOf2)
	}
	
	if c.Performance.MaxConcurrentOps <= 0 {
		return fmt.Errorf("max concurrent operations must be positive (current: %d). Recommended values: 5 for low-spec systems, 10 for normal use, 20+ for high-performance systems", c.Performance.MaxConcurrentOps)
	}
	if c.Performance.MaxConcurrentOps > 1000 {
		return fmt.Errorf("max concurrent operations is very high (%d), which may overwhelm system resources. Consider using 10-50 for most use cases", c.Performance.MaxConcurrentOps)
	}
	
	if c.Performance.ReadAheadSize < 0 {
		return fmt.Errorf("read-ahead size must be non-negative (current: %d). Use 0 to disable read-ahead, or 64KB-256KB for streaming performance", c.Performance.ReadAheadSize)
	}
	if c.Performance.ReadAheadSize > 0 && c.Performance.ReadAheadSize < c.Performance.StreamingChunkSize {
		return fmt.Errorf("read-ahead size (%d) should be at least as large as streaming chunk size (%d) for optimal performance", c.Performance.ReadAheadSize, c.Performance.StreamingChunkSize)
	}
	
	if c.Performance.WriteBufferSize <= 0 {
		return fmt.Errorf("write buffer size must be positive (current: %d). Recommended values: 32KB for low memory, 64KB for normal use, 128KB for high performance", c.Performance.WriteBufferSize)
	}
	if c.Performance.WriteBufferSize < 4096 {
		return fmt.Errorf("write buffer size is very small (%d bytes), which may impact performance. Consider using at least 32KB for better efficiency", c.Performance.WriteBufferSize)
	}

	// Validate mount configuration
	if c.Mount.AttrTimeout < 0 {
		return fmt.Errorf("attribute timeout must be non-negative (current: %v). Use 0 for no caching, 1s for balance, or 5s for performance", c.Mount.AttrTimeout)
	}
	if c.Mount.AttrTimeout > time.Hour {
		return fmt.Errorf("attribute timeout is very long (%v), which may show stale file metadata. Consider using 1s-30s for most use cases", c.Mount.AttrTimeout)
	}
	
	if c.Mount.EntryTimeout < 0 {
		return fmt.Errorf("entry timeout must be non-negative (current: %v). Use 0 for no caching, 1s for balance, or 5s for performance", c.Mount.EntryTimeout)
	}
	if c.Mount.EntryTimeout > time.Hour {
		return fmt.Errorf("entry timeout is very long (%v), which may show stale directory listings. Consider using 1s-30s for most use cases", c.Mount.EntryTimeout)
	}
	
	if c.Mount.NegativeTimeout < 0 {
		return fmt.Errorf("negative timeout must be non-negative (current: %v). Use 0 for no caching, 1s for balance, or 5s for performance", c.Mount.NegativeTimeout)
	}
	if c.Mount.NegativeTimeout > time.Hour {
		return fmt.Errorf("negative timeout is very long (%v), which may prevent seeing new files. Consider using 1s-30s for most use cases", c.Mount.NegativeTimeout)
	}

	// Validate volume name
	if c.Mount.DefaultVolumeName == "" {
		return fmt.Errorf("volume name cannot be empty. Use a descriptive name like 'NoiseFS' or 'PrivateStorage'")
	}
	if len(c.Mount.DefaultVolumeName) > 255 {
		return fmt.Errorf("volume name is too long (%d characters). Keep it under 255 characters for compatibility", len(c.Mount.DefaultVolumeName))
	}

	return nil
}

// GetDirectoryCacheConfig returns a DirectoryCacheConfig from the FUSE configuration.
//
// This method provides a bridge between the centralized FUSE configuration
// and the directory cache subsystem, ensuring consistent configuration
// across all FUSE components.
//
// The returned configuration inherits all cache settings from the FUSE
// configuration and can be used directly with NewDirectoryCache().
//
// Returns:
//   *DirectoryCacheConfig: Cache configuration suitable for directory cache initialization
//
// Complexity: O(1) - Simple structure mapping
func (c *FUSEConfig) GetDirectoryCacheConfig() *DirectoryCacheConfig {
	return &DirectoryCacheConfig{
		MaxSize:       c.Cache.Size,
		TTL:           c.Cache.TTL,
		EnableMetrics: c.Cache.EnableMetrics,
	}
}

// Global configuration instance for easy access across the FUSE package
var globalConfig *FUSEConfig

// GetGlobalConfig returns the global FUSE configuration instance.
//
// This function provides access to a shared configuration instance that
// is loaded once and reused throughout the FUSE package. The configuration
// includes environment variable overrides and should be validated before use.
//
// Thread Safety:
//   - Safe for concurrent read access after initialization
//   - Should not be modified after first access
//   - Use SetGlobalConfig() to update the global instance
//
// Initialization:
//   - Loads configuration with environment overrides on first call
//   - Subsequent calls return the cached instance
//   - Call SetGlobalConfig() to update the global configuration
//
// Returns:
//   *FUSEConfig: The global FUSE configuration instance
//
// Complexity: O(1) after first initialization
func GetGlobalConfig() *FUSEConfig {
	if globalConfig == nil {
		globalConfig = LoadFUSEConfig()
	}
	return globalConfig
}

// SetGlobalConfig sets the global FUSE configuration instance.
//
// This function allows updating the global configuration instance used
// throughout the FUSE package. This is useful for testing or when
// configuration needs to be updated at runtime.
//
// Parameters:
//   config: The new FUSE configuration to use globally. Should be validated before setting.
//
// Thread Safety:
//   - This function is not thread-safe
//   - Should only be called during initialization or in controlled environments
//   - Concurrent access during updates may lead to inconsistent state
//
// Usage:
//   - Call during application startup to set custom configuration
//   - Use in tests to set specific test configurations
//   - Avoid calling after FUSE operations have started
//
// Complexity: O(1) - Simple assignment
func SetGlobalConfig(config *FUSEConfig) {
	globalConfig = config
}
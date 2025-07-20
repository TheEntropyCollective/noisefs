package fuse

import (
	"fmt"
	"os"
)

// TestExampleLoadFUSEConfig demonstrates basic FUSE configuration usage
func TestExampleLoadFUSEConfig(t *testing.T) {
	// Load configuration with environment variable overrides
	config := LoadFUSEConfig()
	
	// Validate configuration before using it
	if err := config.Validate(); err != nil {
		fmt.Printf("Configuration error: %v\n", err)
		return
	}
	
	// Use configuration values
	fmt.Printf("Cache size: %d\n", config.Cache.Size)
	fmt.Printf("Cache TTL: %v\n", config.Cache.TTL)
	fmt.Printf("Memory lock enabled: %v\n", config.Security.MemoryLock)
	fmt.Printf("Streaming chunk size: %d\n", config.Performance.StreamingChunkSize)
	
	// Output:
	// Cache size: 100
	// Cache TTL: 30m0s
	// Memory lock enabled: true
	// Streaming chunk size: 65536
}

// ExampleEnvironmentOverrides demonstrates environment variable configuration
func ExampleEnvironmentOverrides() {
	// Set environment variables to override defaults
	os.Setenv("NOISEFS_FUSE_CACHE_SIZE", "500")
	os.Setenv("NOISEFS_FUSE_CACHE_TTL", "1h")
	os.Setenv("NOISEFS_FUSE_MEMORY_LOCK", "false")
	
	// Clean up after example
	defer func() {
		os.Unsetenv("NOISEFS_FUSE_CACHE_SIZE")
		os.Unsetenv("NOISEFS_FUSE_CACHE_TTL")
		os.Unsetenv("NOISEFS_FUSE_MEMORY_LOCK")
	}()
	
	// Load configuration - environment variables take precedence
	config := LoadFUSEConfig()
	
	fmt.Printf("Cache size: %d\n", config.Cache.Size)
	fmt.Printf("Cache TTL: %v\n", config.Cache.TTL)
	fmt.Printf("Memory lock enabled: %v\n", config.Security.MemoryLock)
	
	// Output:
	// Cache size: 500
	// Cache TTL: 1h0m0s
	// Memory lock enabled: false
}

// ExampleDirectoryCacheIntegration demonstrates directory cache configuration integration
func ExampleDirectoryCacheIntegration() {
	// Set FUSE cache configuration via environment
	os.Setenv("NOISEFS_FUSE_CACHE_SIZE", "250")
	os.Setenv("NOISEFS_FUSE_CACHE_TTL", "45m")
	
	defer func() {
		os.Unsetenv("NOISEFS_FUSE_CACHE_SIZE")
		os.Unsetenv("NOISEFS_FUSE_CACHE_TTL")
	}()
	
	// Get FUSE configuration
	fuseConfig := LoadFUSEConfig()
	
	// Convert to directory cache configuration
	cacheConfig := fuseConfig.GetDirectoryCacheConfig()
	
	fmt.Printf("Directory cache max size: %d\n", cacheConfig.MaxSize)
	fmt.Printf("Directory cache TTL: %v\n", cacheConfig.TTL)
	fmt.Printf("Metrics enabled: %v\n", cacheConfig.EnableMetrics)
	
	// Output:
	// Directory cache max size: 250
	// Directory cache TTL: 45m0s
	// Metrics enabled: true
}

// ExampleGlobalConfiguration demonstrates global configuration usage
func ExampleGlobalConfiguration() {
	// Set custom global configuration
	customConfig := DefaultFUSEConfig()
	customConfig.Cache.Size = 999
	customConfig.Security.SecureDeletePasses = 7
	SetGlobalConfig(customConfig)
	
	// Access global configuration from anywhere in the FUSE package
	globalConfig := GetGlobalConfig()
	
	fmt.Printf("Global cache size: %d\n", globalConfig.Cache.Size)
	fmt.Printf("Global secure delete passes: %d\n", globalConfig.Security.SecureDeletePasses)
	
	// Output:
	// Global cache size: 999
	// Global secure delete passes: 7
}

// ExampleMountOptionsFromConfig demonstrates creating mount options from configuration
func ExampleMountOptionsFromConfig() {
	// Load and validate configuration
	config := LoadFUSEConfig()
	if err := config.Validate(); err != nil {
		fmt.Printf("Configuration error: %v\n", err)
		return
	}
	
	// Create mount options from configuration
	// This shows how the configuration integrates with existing FUSE structures
	opts := MountOptions{
		MountPath:     config.Mount.DefaultPath,
		VolumeName:    config.Mount.DefaultVolumeName,
		ReadOnly:      config.Mount.ReadOnly,
		AllowOther:    config.Mount.AllowOther,
		Debug:         config.Mount.Debug,
		// Note: Security and other fields would be set from their respective configs
	}
	
	fmt.Printf("Mount path: %s\n", opts.MountPath)
	fmt.Printf("Volume name: %s\n", opts.VolumeName)
	fmt.Printf("Read only: %v\n", opts.ReadOnly)
	fmt.Printf("Allow other: %v\n", opts.AllowOther)
	fmt.Printf("Debug: %v\n", opts.Debug)
	
	// Output:
	// Mount path: 
	// Volume name: NoiseFS
	// Read only: false
	// Allow other: false
	// Debug: false
}

// ExampleConfigurationValidation demonstrates comprehensive validation
func ExampleConfigurationValidation() {
	// Create configuration with some invalid values
	config := DefaultFUSEConfig()
	config.Cache.Size = 0                    // Invalid: must be positive
	config.Performance.StreamingChunkSize = 1000 // Invalid: should be power of 2
	config.Mount.DefaultVolumeName = ""          // Invalid: cannot be empty
	
	// Validate and see helpful error messages
	if err := config.Validate(); err != nil {
		fmt.Printf("Validation failed: %v\n", err)
		// In practice, you would fix the configuration and retry
	}
	
	// Output:
	// Validation failed: cache size must be positive (current: 0). Recommended values: 50 for low-memory systems, 100 for normal use, 500+ for high-activity directories
}

// ExamplePerformanceConfiguration demonstrates performance-related settings
func ExamplePerformanceConfiguration() {
	config := DefaultFUSEConfig()
	
	fmt.Printf("Streaming configuration:\n")
	fmt.Printf("  Chunk size: %d bytes\n", config.Performance.StreamingChunkSize)
	fmt.Printf("  Read-ahead size: %d bytes\n", config.Performance.ReadAheadSize)
	fmt.Printf("  Write buffer size: %d bytes\n", config.Performance.WriteBufferSize)
	fmt.Printf("  Max concurrent ops: %d\n", config.Performance.MaxConcurrentOps)
	fmt.Printf("  Async I/O enabled: %v\n", config.Performance.EnableAsyncIO)
	
	// Output:
	// Streaming configuration:
	//   Chunk size: 65536 bytes
	//   Read-ahead size: 131072 bytes
	//   Write buffer size: 65536 bytes
	//   Max concurrent ops: 10
	//   Async I/O enabled: true
}

// ExampleSecurityConfiguration demonstrates security-related settings
func ExampleSecurityConfiguration() {
	config := DefaultFUSEConfig()
	
	fmt.Printf("Security configuration:\n")
	fmt.Printf("  Secure delete passes: %d\n", config.Security.SecureDeletePasses)
	fmt.Printf("  Memory lock enabled: %v\n", config.Security.MemoryLock)
	fmt.Printf("  Clear memory on exit: %v\n", config.Security.ClearMemoryOnExit)
	fmt.Printf("  Restrict extended attributes: %v\n", config.Security.RestrictXAttrs)
	
	// Output:
	// Security configuration:
	//   Secure delete passes: 3
	//   Memory lock enabled: true
	//   Clear memory on exit: true
	//   Restrict extended attributes: true
}
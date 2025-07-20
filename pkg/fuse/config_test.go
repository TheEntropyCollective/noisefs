package fuse

import (
	"os"
	"testing"
	"time"
)

func TestDefaultFUSEConfig(t *testing.T) {
	config := DefaultFUSEConfig()

	// Test cache defaults
	if config.Cache.Size != 100 {
		t.Errorf("Expected default cache size 100, got %d", config.Cache.Size)
	}
	if config.Cache.TTL != 30*time.Minute {
		t.Errorf("Expected default TTL 30m, got %v", config.Cache.TTL)
	}
	if config.Cache.MaxEntries != 1000 {
		t.Errorf("Expected default max entries 1000, got %d", config.Cache.MaxEntries)
	}
	if !config.Cache.EnableMetrics {
		t.Error("Expected metrics to be enabled by default")
	}

	// Test security defaults
	if config.Security.SecureDeletePasses != 3 {
		t.Errorf("Expected default secure delete passes 3, got %d", config.Security.SecureDeletePasses)
	}
	if !config.Security.MemoryLock {
		t.Error("Expected memory lock to be enabled by default")
	}
	if !config.Security.ClearMemoryOnExit {
		t.Error("Expected clear memory on exit to be enabled by default")
	}
	if !config.Security.RestrictXAttrs {
		t.Error("Expected restrict xattrs to be enabled by default")
	}

	// Test performance defaults
	if config.Performance.StreamingChunkSize != 65536 {
		t.Errorf("Expected default streaming chunk size 65536, got %d", config.Performance.StreamingChunkSize)
	}
	if config.Performance.MaxConcurrentOps != 10 {
		t.Errorf("Expected default max concurrent ops 10, got %d", config.Performance.MaxConcurrentOps)
	}
	if config.Performance.ReadAheadSize != 131072 {
		t.Errorf("Expected default read ahead size 131072, got %d", config.Performance.ReadAheadSize)
	}
	if config.Performance.WriteBufferSize != 65536 {
		t.Errorf("Expected default write buffer size 65536, got %d", config.Performance.WriteBufferSize)
	}
	if !config.Performance.EnableAsyncIO {
		t.Error("Expected async IO to be enabled by default")
	}

	// Test mount defaults
	if config.Mount.DefaultPath != "" {
		t.Errorf("Expected empty default path, got %s", config.Mount.DefaultPath)
	}
	if config.Mount.DefaultVolumeName != "NoiseFS" {
		t.Errorf("Expected default volume name 'NoiseFS', got %s", config.Mount.DefaultVolumeName)
	}
	if config.Mount.AllowOther {
		t.Error("Expected allow other to be disabled by default")
	}
	if config.Mount.Debug {
		t.Error("Expected debug to be disabled by default")
	}
	if config.Mount.ReadOnly {
		t.Error("Expected read only to be disabled by default")
	}
	if config.Mount.AttrTimeout != time.Second {
		t.Errorf("Expected default attr timeout 1s, got %v", config.Mount.AttrTimeout)
	}
	if config.Mount.EntryTimeout != time.Second {
		t.Errorf("Expected default entry timeout 1s, got %v", config.Mount.EntryTimeout)
	}
	if config.Mount.NegativeTimeout != time.Second {
		t.Errorf("Expected default negative timeout 1s, got %v", config.Mount.NegativeTimeout)
	}
}

func TestLoadFUSEConfig(t *testing.T) {
	config := LoadFUSEConfig()

	// Should return a valid configuration with defaults
	if err := config.Validate(); err != nil {
		t.Errorf("Default configuration should be valid: %v", err)
	}

	// Should be the same as DefaultFUSEConfig initially
	defaultConfig := DefaultFUSEConfig()
	if config.Cache.Size != defaultConfig.Cache.Size {
		t.Errorf("Loaded config should match default config")
	}
}

func TestEnvironmentOverrides(t *testing.T) {
	// Store original environment
	originalEnv := make(map[string]string)
	envVars := []string{
		"NOISEFS_FUSE_CACHE_SIZE",
		"NOISEFS_FUSE_CACHE_TTL",
		"NOISEFS_FUSE_CACHE_MAX",
		"NOISEFS_FUSE_CACHE_METRICS",
		"NOISEFS_FUSE_SECURE_DELETE_PASSES",
		"NOISEFS_FUSE_MEMORY_LOCK",
		"NOISEFS_FUSE_CLEAR_MEMORY",
		"NOISEFS_FUSE_RESTRICT_XATTRS",
		"NOISEFS_FUSE_STREAM_CHUNK_SIZE",
		"NOISEFS_FUSE_MAX_CONCURRENT_OPS",
		"NOISEFS_FUSE_READAHEAD_SIZE",
		"NOISEFS_FUSE_WRITE_BUFFER_SIZE",
		"NOISEFS_FUSE_ASYNC_IO",
		"NOISEFS_FUSE_DEFAULT_PATH",
		"NOISEFS_FUSE_VOLUME_NAME",
		"NOISEFS_FUSE_ALLOW_OTHER",
		"NOISEFS_FUSE_DEBUG",
		"NOISEFS_FUSE_READ_ONLY",
		"NOISEFS_FUSE_ATTR_TIMEOUT",
		"NOISEFS_FUSE_ENTRY_TIMEOUT",
		"NOISEFS_FUSE_NEGATIVE_TIMEOUT",
	}

	for _, envVar := range envVars {
		originalEnv[envVar] = os.Getenv(envVar)
	}

	// Clean up environment after test
	defer func() {
		for _, envVar := range envVars {
			if val, exists := originalEnv[envVar]; exists && val != "" {
				os.Setenv(envVar, val)
			} else {
				os.Unsetenv(envVar)
			}
		}
	}()

	// Test cache overrides
	os.Setenv("NOISEFS_FUSE_CACHE_SIZE", "200")
	os.Setenv("NOISEFS_FUSE_CACHE_TTL", "1h")
	os.Setenv("NOISEFS_FUSE_CACHE_MAX", "2000")
	os.Setenv("NOISEFS_FUSE_CACHE_METRICS", "false")

	// Test security overrides
	os.Setenv("NOISEFS_FUSE_SECURE_DELETE_PASSES", "7")
	os.Setenv("NOISEFS_FUSE_MEMORY_LOCK", "false")
	os.Setenv("NOISEFS_FUSE_CLEAR_MEMORY", "false")
	os.Setenv("NOISEFS_FUSE_RESTRICT_XATTRS", "false")

	// Test performance overrides
	os.Setenv("NOISEFS_FUSE_STREAM_CHUNK_SIZE", "131072")
	os.Setenv("NOISEFS_FUSE_MAX_CONCURRENT_OPS", "20")
	os.Setenv("NOISEFS_FUSE_READAHEAD_SIZE", "262144")
	os.Setenv("NOISEFS_FUSE_WRITE_BUFFER_SIZE", "131072")
	os.Setenv("NOISEFS_FUSE_ASYNC_IO", "false")

	// Test mount overrides
	os.Setenv("NOISEFS_FUSE_DEFAULT_PATH", "/tmp/noisefs")
	os.Setenv("NOISEFS_FUSE_VOLUME_NAME", "TestFS")
	os.Setenv("NOISEFS_FUSE_ALLOW_OTHER", "true")
	os.Setenv("NOISEFS_FUSE_DEBUG", "true")
	os.Setenv("NOISEFS_FUSE_READ_ONLY", "true")
	os.Setenv("NOISEFS_FUSE_ATTR_TIMEOUT", "5s")
	os.Setenv("NOISEFS_FUSE_ENTRY_TIMEOUT", "10s")
	os.Setenv("NOISEFS_FUSE_NEGATIVE_TIMEOUT", "2s")

	config := LoadFUSEConfig()

	// Verify cache overrides
	if config.Cache.Size != 200 {
		t.Errorf("Expected cache size 200, got %d", config.Cache.Size)
	}
	if config.Cache.TTL != time.Hour {
		t.Errorf("Expected TTL 1h, got %v", config.Cache.TTL)
	}
	if config.Cache.MaxEntries != 2000 {
		t.Errorf("Expected max entries 2000, got %d", config.Cache.MaxEntries)
	}
	if config.Cache.EnableMetrics {
		t.Error("Expected metrics to be disabled")
	}

	// Verify security overrides
	if config.Security.SecureDeletePasses != 7 {
		t.Errorf("Expected secure delete passes 7, got %d", config.Security.SecureDeletePasses)
	}
	if config.Security.MemoryLock {
		t.Error("Expected memory lock to be disabled")
	}
	if config.Security.ClearMemoryOnExit {
		t.Error("Expected clear memory on exit to be disabled")
	}
	if config.Security.RestrictXAttrs {
		t.Error("Expected restrict xattrs to be disabled")
	}

	// Verify performance overrides
	if config.Performance.StreamingChunkSize != 131072 {
		t.Errorf("Expected streaming chunk size 131072, got %d", config.Performance.StreamingChunkSize)
	}
	if config.Performance.MaxConcurrentOps != 20 {
		t.Errorf("Expected max concurrent ops 20, got %d", config.Performance.MaxConcurrentOps)
	}
	if config.Performance.ReadAheadSize != 262144 {
		t.Errorf("Expected read ahead size 262144, got %d", config.Performance.ReadAheadSize)
	}
	if config.Performance.WriteBufferSize != 131072 {
		t.Errorf("Expected write buffer size 131072, got %d", config.Performance.WriteBufferSize)
	}
	if config.Performance.EnableAsyncIO {
		t.Error("Expected async IO to be disabled")
	}

	// Verify mount overrides
	if config.Mount.DefaultPath != "/tmp/noisefs" {
		t.Errorf("Expected default path '/tmp/noisefs', got %s", config.Mount.DefaultPath)
	}
	if config.Mount.DefaultVolumeName != "TestFS" {
		t.Errorf("Expected volume name 'TestFS', got %s", config.Mount.DefaultVolumeName)
	}
	if !config.Mount.AllowOther {
		t.Error("Expected allow other to be enabled")
	}
	if !config.Mount.Debug {
		t.Error("Expected debug to be enabled")
	}
	if !config.Mount.ReadOnly {
		t.Error("Expected read only to be enabled")
	}
	if config.Mount.AttrTimeout != 5*time.Second {
		t.Errorf("Expected attr timeout 5s, got %v", config.Mount.AttrTimeout)
	}
	if config.Mount.EntryTimeout != 10*time.Second {
		t.Errorf("Expected entry timeout 10s, got %v", config.Mount.EntryTimeout)
	}
	if config.Mount.NegativeTimeout != 2*time.Second {
		t.Errorf("Expected negative timeout 2s, got %v", config.Mount.NegativeTimeout)
	}
}

func TestValidation(t *testing.T) {
	// Test valid configuration
	config := DefaultFUSEConfig()
	if err := config.Validate(); err != nil {
		t.Errorf("Default configuration should be valid: %v", err)
	}

	// Test invalid cache size
	config.Cache.Size = 0
	if err := config.Validate(); err == nil {
		t.Error("Expected validation error for zero cache size")
	}
	config.Cache.Size = 100 // reset

	// Test invalid TTL
	config.Cache.TTL = 0
	if err := config.Validate(); err == nil {
		t.Error("Expected validation error for zero TTL")
	}
	config.Cache.TTL = 30 * time.Minute // reset

	// Test invalid max entries
	config.Cache.MaxEntries = 0
	if err := config.Validate(); err == nil {
		t.Error("Expected validation error for zero max entries")
	}
	config.Cache.MaxEntries = 50 // smaller than cache size
	if err := config.Validate(); err == nil {
		t.Error("Expected validation error for max entries smaller than cache size")
	}
	config.Cache.MaxEntries = 1000 // reset

	// Test invalid secure delete passes
	config.Security.SecureDeletePasses = -1
	if err := config.Validate(); err == nil {
		t.Error("Expected validation error for negative secure delete passes")
	}
	config.Security.SecureDeletePasses = 3 // reset

	// Test invalid streaming chunk size
	config.Performance.StreamingChunkSize = 0
	if err := config.Validate(); err == nil {
		t.Error("Expected validation error for zero streaming chunk size")
	}
	config.Performance.StreamingChunkSize = 1000 // not power of 2
	if err := config.Validate(); err == nil {
		t.Error("Expected validation error for non-power-of-2 streaming chunk size")
	}
	config.Performance.StreamingChunkSize = 65536 // reset

	// Test invalid max concurrent ops
	config.Performance.MaxConcurrentOps = 0
	if err := config.Validate(); err == nil {
		t.Error("Expected validation error for zero max concurrent ops")
	}
	config.Performance.MaxConcurrentOps = 10 // reset

	// Test invalid read ahead size
	config.Performance.ReadAheadSize = -1
	if err := config.Validate(); err == nil {
		t.Error("Expected validation error for negative read ahead size")
	}
	config.Performance.ReadAheadSize = 1000 // smaller than chunk size
	if err := config.Validate(); err == nil {
		t.Error("Expected validation error for read ahead size smaller than chunk size")
	}
	config.Performance.ReadAheadSize = 131072 // reset

	// Test invalid write buffer size
	config.Performance.WriteBufferSize = 0
	if err := config.Validate(); err == nil {
		t.Error("Expected validation error for zero write buffer size")
	}
	config.Performance.WriteBufferSize = 65536 // reset

	// Test invalid timeouts
	config.Mount.AttrTimeout = -1 * time.Second
	if err := config.Validate(); err == nil {
		t.Error("Expected validation error for negative attr timeout")
	}
	config.Mount.AttrTimeout = time.Second // reset

	config.Mount.EntryTimeout = -1 * time.Second
	if err := config.Validate(); err == nil {
		t.Error("Expected validation error for negative entry timeout")
	}
	config.Mount.EntryTimeout = time.Second // reset

	config.Mount.NegativeTimeout = -1 * time.Second
	if err := config.Validate(); err == nil {
		t.Error("Expected validation error for negative negative timeout")
	}
	config.Mount.NegativeTimeout = time.Second // reset

	// Test empty volume name
	config.Mount.DefaultVolumeName = ""
	if err := config.Validate(); err == nil {
		t.Error("Expected validation error for empty volume name")
	}
}

func TestGetDirectoryCacheConfig(t *testing.T) {
	config := DefaultFUSEConfig()
	config.Cache.Size = 250
	config.Cache.TTL = 45 * time.Minute
	config.Cache.EnableMetrics = false

	cacheConfig := config.GetDirectoryCacheConfig()

	if cacheConfig.MaxSize != 250 {
		t.Errorf("Expected cache config max size 250, got %d", cacheConfig.MaxSize)
	}
	if cacheConfig.TTL != 45*time.Minute {
		t.Errorf("Expected cache config TTL 45m, got %v", cacheConfig.TTL)
	}
	if cacheConfig.EnableMetrics {
		t.Error("Expected cache config metrics to be disabled")
	}
}

func TestGlobalConfig(t *testing.T) {
	// Reset global config
	globalConfig = nil

	// First call should initialize
	config1 := GetGlobalConfig()
	if config1 == nil {
		t.Error("Expected non-nil global config")
	}

	// Second call should return same instance
	config2 := GetGlobalConfig()
	if config1 != config2 {
		t.Error("Expected same global config instance")
	}

	// Test setting global config
	customConfig := DefaultFUSEConfig()
	customConfig.Cache.Size = 999
	SetGlobalConfig(customConfig)

	config3 := GetGlobalConfig()
	if config3.Cache.Size != 999 {
		t.Errorf("Expected custom cache size 999, got %d", config3.Cache.Size)
	}
}

func TestEnvironmentErrorHandling(t *testing.T) {
	// Store original environment
	originalCacheSize := os.Getenv("NOISEFS_FUSE_CACHE_SIZE")
	originalTTL := os.Getenv("NOISEFS_FUSE_CACHE_TTL")
	originalMemoryLock := os.Getenv("NOISEFS_FUSE_MEMORY_LOCK")

	defer func() {
		// Clean up
		if originalCacheSize != "" {
			os.Setenv("NOISEFS_FUSE_CACHE_SIZE", originalCacheSize)
		} else {
			os.Unsetenv("NOISEFS_FUSE_CACHE_SIZE")
		}
		if originalTTL != "" {
			os.Setenv("NOISEFS_FUSE_CACHE_TTL", originalTTL)
		} else {
			os.Unsetenv("NOISEFS_FUSE_CACHE_TTL")
		}
		if originalMemoryLock != "" {
			os.Setenv("NOISEFS_FUSE_MEMORY_LOCK", originalMemoryLock)
		} else {
			os.Unsetenv("NOISEFS_FUSE_MEMORY_LOCK")
		}
	}()

	// Test invalid integer
	os.Setenv("NOISEFS_FUSE_CACHE_SIZE", "invalid")
	config := LoadFUSEConfig()
	// Should use default value when invalid
	if config.Cache.Size != 100 {
		t.Errorf("Expected default cache size when invalid env var, got %d", config.Cache.Size)
	}

	// Test invalid duration
	os.Setenv("NOISEFS_FUSE_CACHE_TTL", "invalid")
	config = LoadFUSEConfig()
	// Should use default value when invalid
	if config.Cache.TTL != 30*time.Minute {
		t.Errorf("Expected default TTL when invalid env var, got %v", config.Cache.TTL)
	}

	// Test invalid boolean - should treat invalid as false
	os.Setenv("NOISEFS_FUSE_MEMORY_LOCK", "invalid")
	config = LoadFUSEConfig()
	// Should treat invalid boolean as false
	if config.Security.MemoryLock {
		t.Error("Expected memory lock to be false when invalid boolean env var")
	}

	// Test negative values are ignored
	os.Setenv("NOISEFS_FUSE_CACHE_SIZE", "-100")
	config = LoadFUSEConfig()
	if config.Cache.Size != 100 {
		t.Errorf("Expected default cache size when negative env var, got %d", config.Cache.Size)
	}
}

func TestValidationErrorMessages(t *testing.T) {
	config := DefaultFUSEConfig()

	// Test cache size validation messages
	config.Cache.Size = -5
	err := config.Validate()
	if err == nil || !contains(err.Error(), "must be positive") {
		t.Errorf("Expected helpful error message for negative cache size, got: %v", err)
	}

	config.Cache.Size = 15000
	err = config.Validate()
	if err == nil || !contains(err.Error(), "very large") {
		t.Errorf("Expected helpful error message for large cache size, got: %v", err)
	}

	// Test chunk size power of 2 validation
	config.Cache.Size = 100 // reset
	config.Performance.StreamingChunkSize = 100000 // Large enough to pass size check but not power of 2
	err = config.Validate()
	if err == nil || !contains(err.Error(), "power of 2") {
		t.Errorf("Expected helpful error message for non-power-of-2 chunk size, got: %v", err)
	}

	// Test read ahead size validation
	config.Performance.StreamingChunkSize = 65536 // reset
	config.Performance.ReadAheadSize = 1000
	err = config.Validate()
	if err == nil || !contains(err.Error(), "at least as large") {
		t.Errorf("Expected helpful error message for small read ahead size, got: %v", err)
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || 
		(len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || 
		containsHelper(s, substr))))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestBenchmarkConfiguration(t *testing.T) {
	// Benchmark configuration loading
	start := time.Now()
	iterations := 1000
	
	for i := 0; i < iterations; i++ {
		config := LoadFUSEConfig()
		_ = config.Validate()
	}
	
	elapsed := time.Since(start)
	avgTime := elapsed / time.Duration(iterations)
	
	// Configuration loading should be fast (< 1ms per operation)
	if avgTime > time.Millisecond {
		t.Errorf("Configuration loading too slow: %v per operation", avgTime)
	}
	
	t.Logf("Configuration loading performance: %v per operation (%d iterations)", avgTime, iterations)
}
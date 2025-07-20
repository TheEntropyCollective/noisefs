package fuse

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultFuseConfig(t *testing.T) {
	config := DefaultFuseConfig()
	
	// Test cache configuration
	if config.Cache.DirectoryMaxSize != 100 {
		t.Errorf("Expected DirectoryMaxSize to be 100, got %d", config.Cache.DirectoryMaxSize)
	}
	if config.Cache.DirectoryTTL != 30*time.Minute {
		t.Errorf("Expected DirectoryTTL to be 30 minutes, got %v", config.Cache.DirectoryTTL)
	}
	
	// Test security configuration
	if config.Security.DefaultFileMode != 0644 {
		t.Errorf("Expected DefaultFileMode to be 0644, got %o", config.Security.DefaultFileMode)
	}
	if config.Security.DefaultDirMode != 0755 {
		t.Errorf("Expected DefaultDirMode to be 0755, got %o", config.Security.DefaultDirMode)
	}
	
	// Test mount configuration
	if config.Mount.DefaultVolumeName != "noisefs" {
		t.Errorf("Expected DefaultVolumeName to be 'noisefs', got '%s'", config.Mount.DefaultVolumeName)
	}
	if config.Mount.FilesSubdirectory != "files" {
		t.Errorf("Expected FilesSubdirectory to be 'files', got '%s'", config.Mount.FilesSubdirectory)
	}
	
	// Test validation
	if err := ValidateConfig(config); err != nil {
		t.Errorf("Default config should be valid, got error: %v", err)
	}
}

func TestPerformanceFuseConfig(t *testing.T) {
	config := PerformanceFuseConfig()
	
	// Performance config should have larger values
	if config.Cache.DirectoryMaxSize <= 100 {
		t.Errorf("Performance config should have larger DirectoryMaxSize, got %d", config.Cache.DirectoryMaxSize)
	}
	if config.Performance.MaxConcurrentOperations <= 10 {
		t.Errorf("Performance config should have more concurrent operations, got %d", config.Performance.MaxConcurrentOperations)
	}
	
	// Test validation
	if err := ValidateConfig(config); err != nil {
		t.Errorf("Performance config should be valid, got error: %v", err)
	}
}

func TestSecureFuseConfig(t *testing.T) {
	config := SecureFuseConfig()
	
	// Security config should enable security features
	if !config.Security.EnableEncryption {
		t.Error("Secure config should enable encryption")
	}
	if !config.Security.SecureMemoryLocking {
		t.Error("Secure config should enable secure memory locking")
	}
	if !config.Security.SecureDeletion {
		t.Error("Secure config should enable secure deletion")
	}
	
	// Should have more restrictive permissions
	if config.Security.DefaultFileMode != 0600 {
		t.Errorf("Secure config should have restrictive file mode, got %o", config.Security.DefaultFileMode)
	}
	
	// Test validation
	if err := ValidateConfig(config); err != nil {
		t.Errorf("Secure config should be valid, got error: %v", err)
	}
}

func TestConfigValidation(t *testing.T) {
	// Test invalid configurations
	config := DefaultFuseConfig()
	
	// Invalid cache size
	config.Cache.DirectoryMaxSize = -1
	if err := ValidateConfig(config); err == nil {
		t.Error("Should reject negative cache size")
	}
	
	// Reset and test invalid TTL
	config = DefaultFuseConfig()
	config.Cache.DirectoryTTL = -1 * time.Second
	if err := ValidateConfig(config); err == nil {
		t.Error("Should reject negative TTL")
	}
	
	// Reset and test empty files subdirectory
	config = DefaultFuseConfig()
	config.Mount.FilesSubdirectory = ""
	if err := ValidateConfig(config); err == nil {
		t.Error("Should reject empty files subdirectory")
	}
}

func TestConfigFileOperations(t *testing.T) {
	// Create temporary directory for test
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.json")
	
	// Test saving config
	config := DefaultFuseConfig()
	config.Cache.DirectoryMaxSize = 200 // Change a value to test persistence
	
	if err := SaveConfigToFile(config, configPath); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}
	
	// Test loading config
	loadedConfig, err := LoadConfigFromFile(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	
	// Verify the changed value persisted
	if loadedConfig.Cache.DirectoryMaxSize != 200 {
		t.Errorf("Expected loaded DirectoryMaxSize to be 200, got %d", loadedConfig.Cache.DirectoryMaxSize)
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	// Save original env vars
	originalVars := map[string]string{
		"NOISEFS_CACHE_DIR_MAX_SIZE": os.Getenv("NOISEFS_CACHE_DIR_MAX_SIZE"),
		"NOISEFS_ENABLE_ENCRYPTION":  os.Getenv("NOISEFS_ENABLE_ENCRYPTION"),
		"NOISEFS_VOLUME_NAME":        os.Getenv("NOISEFS_VOLUME_NAME"),
	}
	
	// Clean up at the end
	defer func() {
		for key, value := range originalVars {
			if value == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, value)
			}
		}
	}()
	
	// Set test env vars
	os.Setenv("NOISEFS_CACHE_DIR_MAX_SIZE", "500")
	os.Setenv("NOISEFS_ENABLE_ENCRYPTION", "true")
	os.Setenv("NOISEFS_VOLUME_NAME", "test-volume")
	
	config := LoadConfigFromEnv()
	
	// Verify env var values were applied
	if config.Cache.DirectoryMaxSize != 500 {
		t.Errorf("Expected DirectoryMaxSize from env to be 500, got %d", config.Cache.DirectoryMaxSize)
	}
	if !config.Security.EnableEncryption {
		t.Error("Expected encryption to be enabled from env")
	}
	if config.Mount.DefaultVolumeName != "test-volume" {
		t.Errorf("Expected volume name from env to be 'test-volume', got '%s'", config.Mount.DefaultVolumeName)
	}
}

func TestNewDirectoryCacheFromFuseConfig(t *testing.T) {
	config := DefaultFuseConfig()
	config.Cache.DirectoryMaxSize = 50
	config.Cache.DirectoryTTL = 15 * time.Minute
	
	// Create a mock storage manager (nil for this test)
	_, err := NewDirectoryCacheFromFuseConfig(config, nil)
	if err == nil {
		t.Error("Should fail with nil storage manager")
	}
	
	// Test with nil config (should use default)
	_, err = NewDirectoryCacheFromFuseConfig(nil, nil)
	if err == nil {
		t.Error("Should fail with nil storage manager even with nil config")
	}
}
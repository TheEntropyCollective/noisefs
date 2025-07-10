package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	// Test defaults
	if config.IPFS.APIEndpoint != "127.0.0.1:5001" {
		t.Errorf("Expected default IPFS endpoint 127.0.0.1:5001, got %s", config.IPFS.APIEndpoint)
	}

	if config.Cache.BlockCacheSize != 1000 {
		t.Errorf("Expected default cache size 1000, got %d", config.Cache.BlockCacheSize)
	}

	if config.Logging.Level != "info" {
		t.Errorf("Expected default log level info, got %s", config.Logging.Level)
	}
}

func TestConfigValidation(t *testing.T) {
	config := DefaultConfig()

	// Test valid config
	if err := config.Validate(); err != nil {
		t.Errorf("Valid config failed validation: %v", err)
	}

	// Test invalid IPFS endpoint
	config.IPFS.APIEndpoint = ""
	if err := config.Validate(); err == nil {
		t.Error("Empty IPFS endpoint should fail validation")
	}

	// Reset and test invalid log level
	config = DefaultConfig()
	config.Logging.Level = "invalid"
	if err := config.Validate(); err == nil {
		t.Error("Invalid log level should fail validation")
	}
}

func TestEnvironmentOverrides(t *testing.T) {
	// Set environment variables
	os.Setenv("NOISEFS_IPFS_API", "test.example.com:5001")
	os.Setenv("NOISEFS_LOG_LEVEL", "debug")
	os.Setenv("NOISEFS_READ_ONLY", "true")
	defer func() {
		os.Unsetenv("NOISEFS_IPFS_API")
		os.Unsetenv("NOISEFS_LOG_LEVEL")
		os.Unsetenv("NOISEFS_READ_ONLY")
	}()

	config := DefaultConfig()
	config.applyEnvironmentOverrides()

	if config.IPFS.APIEndpoint != "test.example.com:5001" {
		t.Errorf("Environment override failed for IPFS API, got %s", config.IPFS.APIEndpoint)
	}

	if config.Logging.Level != "debug" {
		t.Errorf("Environment override failed for log level, got %s", config.Logging.Level)
	}

	if !config.FUSE.ReadOnly {
		t.Error("Environment override failed for read-only flag")
	}
}

func TestConfigFileOperations(t *testing.T) {
	// Create temporary config file
	tmpDir, err := os.MkdirTemp("", "noisefs_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.json")

	// Test saving config
	config := DefaultConfig()
	config.IPFS.APIEndpoint = "custom.example.com:5001"

	if err := config.SaveToFile(configPath); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Test loading config
	loadedConfig, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if loadedConfig.IPFS.APIEndpoint != "custom.example.com:5001" {
		t.Errorf("Config not loaded correctly, got %s", loadedConfig.IPFS.APIEndpoint)
	}
}

func TestLoadNonexistentConfig(t *testing.T) {
	// Test loading non-existent config should use defaults
	config, err := LoadConfig("/nonexistent/path/config.json")
	if err != nil {
		t.Fatalf("Loading non-existent config should not error: %v", err)
	}

	// Should have default values
	if config.IPFS.APIEndpoint != "127.0.0.1:5001" {
		t.Errorf("Non-existent config should use defaults, got %s", config.IPFS.APIEndpoint)
	}
}

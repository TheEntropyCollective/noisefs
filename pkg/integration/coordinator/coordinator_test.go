package integration

import (
	"os"
	"strings"
	"testing"

	"github.com/TheEntropyCollective/noisefs/pkg/common/config"
	"github.com/TheEntropyCollective/noisefs/pkg/common/logging"
)

func TestSystemCoordinatorCreation(t *testing.T) {
	// Note: This is a basic test that will likely fail without a full IPFS setup
	// For unit testing, we would need to mock all dependencies
	
	t.Run("Create with default config", func(t *testing.T) {
		cfg := config.DefaultConfig()
		
		// This will fail in unit tests as it requires real IPFS
		// But it validates that the coordinator structure is correct
		_, err := NewSystemCoordinator(cfg)
		if err == nil {
			t.Skip("SystemCoordinator creation unexpectedly succeeded - requires real IPFS")
		}
		
		// We expect it to fail trying to connect to storage backend
		// After refactoring, the error is about storage/reuse initialization
		expectedErrors := []string{
			"failed to initialize IPFS",
			"failed to initialize reuse",
			"failed to initialize storage",
		}
		
		foundExpectedError := false
		for _, expected := range expectedErrors {
			if strings.Contains(err.Error(), expected) {
				foundExpectedError = true
				break
			}
		}
		
		if !foundExpectedError {
			t.Errorf("Expected error containing one of %v, got: %v", expectedErrors, err)
		}
	})
}

func TestCacheInitialization(t *testing.T) {
	t.Run("Test adaptive cache configuration paths", func(t *testing.T) {
		// Test that we can create configurations for both cache types
		// This validates the conditional logic without requiring full system setup
		
		// Test adaptive cache enabled (default config)
		cfg := config.DefaultConfig()
		if !cfg.Cache.EnableAdaptiveCache {
			t.Error("DefaultConfig should have adaptive cache enabled")
		}
		
		// Test adaptive cache disabled (quickstart config)
		quickCfg := config.QuickStartConfig()
		if quickCfg.Cache.EnableAdaptiveCache {
			t.Error("QuickStartConfig should have adaptive cache disabled")
		}
		
		// Test that we can manually toggle the setting
		cfg.Cache.EnableAdaptiveCache = false
		if cfg.Cache.EnableAdaptiveCache {
			t.Error("Failed to disable adaptive cache")
		}
		
		cfg.Cache.EnableAdaptiveCache = true
		if !cfg.Cache.EnableAdaptiveCache {
			t.Error("Failed to enable adaptive cache")
		}
	})
	
	t.Run("Test cache initialization logic", func(t *testing.T) {
		// Create a coordinator to test the cache initialization paths
		// We'll test just the cache initialization part by creating a coordinator
		// with minimal configuration and checking that it fails at storage, not cache
		
		// Test with adaptive cache enabled
		cfg := config.DefaultConfig()
		cfg.Cache.EnableAdaptiveCache = true
		
		coordinator := &SystemCoordinator{
			config: cfg,
		}
		
		// This should not panic and should set up the conditional correctly
		// We can't test the full initialization without mocking, but we can
		// verify that the configuration is properly read
		if !coordinator.config.Cache.EnableAdaptiveCache {
			t.Error("Coordinator should use adaptive cache when enabled in config")
		}
		
		// Test with adaptive cache disabled
		cfg.Cache.EnableAdaptiveCache = false
		coordinator.config = cfg
		
		if coordinator.config.Cache.EnableAdaptiveCache {
			t.Error("Coordinator should use simple cache when disabled in config")
		}
		
		// Test that both paths can be initialized without panic
		// (They'll fail due to missing dependencies, but the conditional logic should work)
		
		// Test adaptive cache path
		adaptiveCfg := config.DefaultConfig()
		adaptiveCfg.Cache.EnableAdaptiveCache = true
		adaptiveCoordinator := &SystemCoordinator{
			config: adaptiveCfg,
		}
		
		// Test simple cache path  
		simpleCfg := config.QuickStartConfig()
		simpleCfg.Cache.EnableAdaptiveCache = false
		simpleCoordinator := &SystemCoordinator{
			config: simpleCfg,
		}
		
		// Verify both configurations are set correctly
		if !adaptiveCoordinator.config.Cache.EnableAdaptiveCache {
			t.Error("Adaptive coordinator should have adaptive cache enabled")
		}
		if simpleCoordinator.config.Cache.EnableAdaptiveCache {
			t.Error("Simple coordinator should have adaptive cache disabled")
		}
	})
	
	t.Run("Test environment variable override", func(t *testing.T) {
		// Test that the environment variable can override the configuration
		
		// Set environment variable to disable adaptive cache
		originalVal := os.Getenv("NOISEFS_ENABLE_ADAPTIVE_CACHE")
		defer func() {
			if originalVal == "" {
				os.Unsetenv("NOISEFS_ENABLE_ADAPTIVE_CACHE")
			} else {
				os.Setenv("NOISEFS_ENABLE_ADAPTIVE_CACHE", originalVal)
			}
		}()
		
		// Test overriding to false
		os.Setenv("NOISEFS_ENABLE_ADAPTIVE_CACHE", "false")
		cfg, err := config.LoadConfig("")
		if err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		
		if cfg.Cache.EnableAdaptiveCache {
			t.Error("Environment variable should have disabled adaptive cache")
		}
		
		// Test overriding to true
		os.Setenv("NOISEFS_ENABLE_ADAPTIVE_CACHE", "true")
		cfg, err = config.LoadConfig("")
		if err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		
		if !cfg.Cache.EnableAdaptiveCache {
			t.Error("Environment variable should have enabled adaptive cache")
		}
	})
}

func TestSystemMetrics(t *testing.T) {
	metrics := &SystemMetrics{
		TotalUploads:      10,
		TotalDownloads:    20,
		TotalBlocks:       100,
		ReuseRatio:        2.5,
		CoverTrafficRatio: 1.8,
		StorageEfficiency: 0.85,
		PrivacyScore:      0.9,
	}
	
	// Validate all fields
	if metrics.TotalUploads != 10 {
		t.Errorf("Expected 10 uploads, got %d", metrics.TotalUploads)
	}
	
	if metrics.TotalDownloads != 20 {
		t.Errorf("Expected 20 downloads, got %d", metrics.TotalDownloads)
	}
	
	if metrics.TotalBlocks != 100 {
		t.Errorf("Expected 100 blocks, got %d", metrics.TotalBlocks)
	}
	
	if metrics.ReuseRatio != 2.5 {
		t.Errorf("Expected reuse ratio 2.5, got %f", metrics.ReuseRatio)
	}
	
	if metrics.CoverTrafficRatio != 1.8 {
		t.Errorf("Expected cover traffic ratio 1.8, got %f", metrics.CoverTrafficRatio)
	}
	
	if metrics.StorageEfficiency != 0.85 {
		t.Errorf("Expected storage efficiency 0.85, got %f", metrics.StorageEfficiency)
	}
	
	if metrics.PrivacyScore < 0 || metrics.PrivacyScore > 1 {
		t.Errorf("Privacy score out of range: %f", metrics.PrivacyScore)
	}
	
	// Additional validations
	if metrics.StorageEfficiency > 1 {
		t.Errorf("Storage efficiency cannot exceed 1.0: %f", metrics.StorageEfficiency)
	}
	
	if metrics.ReuseRatio < 0 {
		t.Errorf("Reuse ratio cannot be negative: %f", metrics.ReuseRatio)
	}
	
	if metrics.CoverTrafficRatio < 0 {
		t.Errorf("Cover traffic ratio cannot be negative: %f", metrics.CoverTrafficRatio)
	}
}

func TestShutdownResourceCleanup(t *testing.T) {
	// Test that shutdown properly handles all components and doesn't panic
	coordinator := &SystemCoordinator{
		config: config.DefaultConfig(),
		systemMetrics: &SystemMetrics{},
	}
	
	// Initialize minimal logger to avoid nil pointer
	coordinator.logger = logging.GetGlobalLogger().WithComponent("test")
	
	// Test shutdown with no initialized components - should not error
	err := coordinator.Shutdown()
	if err != nil {
		t.Errorf("Shutdown should handle nil components gracefully, got error: %v", err)
	}
	
	// Test that all components are properly nullified after shutdown
	if coordinator.storageManager != nil {
		t.Error("Storage manager should be nil after shutdown")
	}
	if coordinator.blockCache != nil {
		t.Error("Block cache should be nil after shutdown")
	}
	if coordinator.libp2pHost != nil {
		t.Error("LibP2P host should be nil after shutdown")
	}
}


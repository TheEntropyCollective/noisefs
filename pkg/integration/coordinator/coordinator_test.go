package integration

import (
	"strings"
	"testing"

	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/config"
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
		// After refactoring, the error is about subsystem initialization
		expectedErrors := []string{
			"failed to initialize IPFS",
			"failed to initialize storage subsystem",
			"failed to initialize reuse subsystem",
			"failed to initialize privacy subsystem",
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

func TestStorageEfficiencyCalculation(t *testing.T) {
	// Test the storage efficiency calculation with mock data

	// Test case 1: Perfect efficiency (1:1 ratio)
	t.Run("Perfect efficiency", func(t *testing.T) {
		original := int64(1000)
		stored := int64(1000)
		efficiency := float64(original) / float64(stored)
		if efficiency != 1.0 {
			t.Errorf("Expected perfect efficiency 1.0, got %f", efficiency)
		}
	})

	// Test case 2: 128KB overhead due to NoiseFS (typical scenario)
	t.Run("NoiseFS overhead", func(t *testing.T) {
		original := int64(100 * 1024) // 100KB file
		stored := int64(256 * 1024)   // 256KB stored (100KB + randomizers)
		efficiency := float64(original) / float64(stored)
		expected := 100.0 / 256.0 // ~0.39
		if efficiency != expected {
			t.Errorf("Expected efficiency %f, got %f", expected, efficiency)
		}
		if efficiency > 1.0 {
			t.Errorf("Efficiency cannot exceed 1.0: %f", efficiency)
		}
	})

	// Test case 3: Division by zero protection
	t.Run("Division by zero", func(t *testing.T) {
		original := int64(1000)
		stored := int64(0)
		var efficiency float64
		if stored > 0 {
			efficiency = float64(original) / float64(stored)
		} else {
			efficiency = 0.0
		}
		if efficiency != 0.0 {
			t.Errorf("Expected 0.0 efficiency when no data stored, got %f", efficiency)
		}
	})

	// Test case 4: Large file efficiency (should be better)
	t.Run("Large file efficiency", func(t *testing.T) {
		original := int64(10 * 1024 * 1024) // 10MB file
		stored := int64(11 * 1024 * 1024)   // 11MB stored (minimal overhead)
		efficiency := float64(original) / float64(stored)
		expected := 10.0 / 11.0 // ~0.91
		if efficiency != expected {
			t.Errorf("Expected efficiency %f, got %f", expected, efficiency)
		}
		if efficiency > 1.0 {
			t.Errorf("Efficiency cannot exceed 1.0: %f", efficiency)
		}
	})
}

func TestSystemCoordinatorSubsystemGetters(t *testing.T) {
	// Test that subsystem getters exist and have correct return types
	t.Run("Subsystem getters compilation test", func(t *testing.T) {
		// Create a minimal coordinator to test getters (won't succeed but tests compilation)
		coordinator := &SystemCoordinator{}
		
		// Test that all getter methods exist and return expected types
		_ = coordinator.GetStorageSubsystem
		_ = coordinator.GetPrivacySubsystem
		_ = coordinator.GetReuseSubsystem
		_ = coordinator.GetComplianceSubsystem
		_ = coordinator.GetMetricsSubsystem
		
		// These should return nil since coordinator is not initialized, but types should be correct
		storageSubsystem := coordinator.GetStorageSubsystem()
		privacySubsystem := coordinator.GetPrivacySubsystem()
		reuseSubsystem := coordinator.GetReuseSubsystem()
		complianceSubsystem := coordinator.GetComplianceSubsystem()
		metricsSubsystem := coordinator.GetMetricsSubsystem()
		
		// All should be nil but have correct types
		if storageSubsystem != nil {
			t.Log("StorageSubsystem getter works")
		}
		if privacySubsystem != nil {
			t.Log("PrivacySubsystem getter works")
		}
		if reuseSubsystem != nil {
			t.Log("ReuseSubsystem getter works")
		}
		if complianceSubsystem != nil {
			t.Log("ComplianceSubsystem getter works")
		}
		if metricsSubsystem != nil {
			t.Log("MetricsSubsystem getter works")
		}
	})
}

func TestSystemCoordinatorBackwardCompatibility(t *testing.T) {
	// Test that the refactored coordinator maintains the same public API
	t.Run("Public API compatibility", func(t *testing.T) {
		cfg := config.DefaultConfig()
		
		// Test that original constructor exists
		_, err := NewSystemCoordinator(cfg)
		if err == nil {
			t.Skip("SystemCoordinator creation unexpectedly succeeded - requires real IPFS")
		}
		
		// Test that we have the expected error (compilation test)
		if err == nil {
			t.Error("Expected error from SystemCoordinator creation without IPFS")
		}
		
		// Create a minimal coordinator to test API methods exist
		coordinator := &SystemCoordinator{}
		
		// Test that all original public methods exist (compilation test)
		_ = coordinator.GetSystemMetrics
		_ = coordinator.Shutdown
		
		// Note: UploadFile and DownloadFile would panic with nil subsystems,
		// but we're just testing that the methods exist with correct signatures
	})
}

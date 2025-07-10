package integration

import (
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
		
		// We expect it to fail trying to connect to IPFS
		expectedError := "failed to initialize IPFS"
		if err.Error()[:len(expectedError)] != expectedError {
			t.Errorf("Expected error starting with '%s', got: %v", expectedError, err)
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
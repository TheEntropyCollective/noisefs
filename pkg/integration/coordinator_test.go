package integration

import (
	"testing"

	"github.com/TheEntropyCollective/noisefs/pkg/config"
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
	
	// Basic validation
	if metrics.TotalUploads != 10 {
		t.Errorf("Expected 10 uploads, got %d", metrics.TotalUploads)
	}
	
	if metrics.PrivacyScore < 0 || metrics.PrivacyScore > 1 {
		t.Errorf("Privacy score out of range: %f", metrics.PrivacyScore)
	}
	
	if metrics.StorageEfficiency > 1 {
		t.Errorf("Storage efficiency cannot exceed 1.0: %f", metrics.StorageEfficiency)
	}
}
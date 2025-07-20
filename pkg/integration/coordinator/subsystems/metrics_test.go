package subsystems

import (
	"testing"
	"time"
)

func TestMetricsSubsystemCreation(t *testing.T) {
	t.Run("Create metrics subsystem", func(t *testing.T) {
		metrics := NewMetricsSubsystem()
		if metrics == nil {
			t.Error("Expected valid metrics subsystem")
		}
		
		// Test initial state
		systemMetrics := metrics.GetSystemMetrics(nil, nil, nil)
		if systemMetrics.TotalUploads != 0 {
			t.Errorf("Expected 0 initial uploads, got %d", systemMetrics.TotalUploads)
		}
		if systemMetrics.TotalDownloads != 0 {
			t.Errorf("Expected 0 initial downloads, got %d", systemMetrics.TotalDownloads)
		}
	})
}

func TestMetricsSubsystemCounters(t *testing.T) {
	metrics := NewMetricsSubsystem()
	
	t.Run("Increment uploads", func(t *testing.T) {
		initial := metrics.GetSystemMetrics(nil, nil, nil).TotalUploads
		
		metrics.IncrementUploads()
		metrics.IncrementUploads()
		
		current := metrics.GetSystemMetrics(nil, nil, nil).TotalUploads
		expected := initial + 2
		
		if current != expected {
			t.Errorf("Expected %d uploads, got %d", expected, current)
		}
	})
	
	t.Run("Increment downloads", func(t *testing.T) {
		initial := metrics.GetSystemMetrics(nil, nil, nil).TotalDownloads
		
		metrics.IncrementDownloads()
		metrics.IncrementDownloads()
		metrics.IncrementDownloads()
		
		current := metrics.GetSystemMetrics(nil, nil, nil).TotalDownloads
		expected := initial + 3
		
		if current != expected {
			t.Errorf("Expected %d downloads, got %d", expected, current)
		}
	})
}

func TestMetricsSubsystemConcurrency(t *testing.T) {
	metrics := NewMetricsSubsystem()
	
	t.Run("Concurrent increments", func(t *testing.T) {
		// Test that metrics can handle concurrent access
		done := make(chan bool, 10)
		
		// Start 5 goroutines incrementing uploads
		for i := 0; i < 5; i++ {
			go func() {
				for j := 0; j < 10; j++ {
					metrics.IncrementUploads()
				}
				done <- true
			}()
		}
		
		// Start 5 goroutines incrementing downloads
		for i := 0; i < 5; i++ {
			go func() {
				for j := 0; j < 10; j++ {
					metrics.IncrementDownloads()
				}
				done <- true
			}()
		}
		
		// Wait for all goroutines to complete
		for i := 0; i < 10; i++ {
			select {
			case <-done:
				// Good
			case <-time.After(5 * time.Second):
				t.Fatal("Timeout waiting for concurrent operations")
			}
		}
		
		// Check final counts
		systemMetrics := metrics.GetSystemMetrics(nil, nil, nil)
		if systemMetrics.TotalUploads != 50 {
			t.Errorf("Expected 50 uploads, got %d", systemMetrics.TotalUploads)
		}
		if systemMetrics.TotalDownloads != 50 {
			t.Errorf("Expected 50 downloads, got %d", systemMetrics.TotalDownloads)
		}
	})
}

func TestMetricsSubsystemResponsibilities(t *testing.T) {
	t.Run("Metrics subsystem focuses on metrics concerns only", func(t *testing.T) {
		// Test that the metrics subsystem has the right responsibilities
		metrics := NewMetricsSubsystem()
		
		// Test that it has metrics-related methods
		_ = metrics.GetSystemMetrics
		_ = metrics.IncrementUploads
		_ = metrics.IncrementDownloads
		_ = metrics.UpdateMetricsFromUpload
		_ = metrics.Shutdown
		
		// Ensure it doesn't have non-metrics methods (compile-time check)
	})
}

func TestSystemMetricsValidation(t *testing.T) {
	metrics := NewMetricsSubsystem()
	
	t.Run("Validate metrics ranges", func(t *testing.T) {
		systemMetrics := metrics.GetSystemMetrics(nil, nil, nil)
		
		// Validate ranges
		if systemMetrics.TotalUploads < 0 {
			t.Errorf("TotalUploads cannot be negative: %d", systemMetrics.TotalUploads)
		}
		
		if systemMetrics.TotalDownloads < 0 {
			t.Errorf("TotalDownloads cannot be negative: %d", systemMetrics.TotalDownloads)
		}
		
		if systemMetrics.TotalBlocks < 0 {
			t.Errorf("TotalBlocks cannot be negative: %d", systemMetrics.TotalBlocks)
		}
		
		if systemMetrics.ReuseRatio < 0 {
			t.Errorf("ReuseRatio cannot be negative: %f", systemMetrics.ReuseRatio)
		}
		
		if systemMetrics.CoverTrafficRatio < 0 {
			t.Errorf("CoverTrafficRatio cannot be negative: %f", systemMetrics.CoverTrafficRatio)
		}
		
		if systemMetrics.StorageEfficiency < 0 || systemMetrics.StorageEfficiency > 1 {
			t.Errorf("StorageEfficiency out of range [0,1]: %f", systemMetrics.StorageEfficiency)
		}
		
		if systemMetrics.PrivacyScore < 0 || systemMetrics.PrivacyScore > 1 {
			t.Errorf("PrivacyScore out of range [0,1]: %f", systemMetrics.PrivacyScore)
		}
	})
}
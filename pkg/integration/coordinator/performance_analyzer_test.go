package integration

import (
	"testing"
	"time"
)

func TestMetricsConfigDefaults(t *testing.T) {
	config := DefaultMetricsConfig()
	
	if config.MaxOperations != 10000 {
		t.Errorf("Expected MaxOperations to be 10000, got %d", config.MaxOperations)
	}
	
	if config.MaxCacheMetrics != 1000 {
		t.Errorf("Expected MaxCacheMetrics to be 1000, got %d", config.MaxCacheMetrics)
	}
	
	if config.MaxPeerMetrics != 5000 {
		t.Errorf("Expected MaxPeerMetrics to be 5000, got %d", config.MaxPeerMetrics)
	}
	
	if config.RetentionPeriod != 24*time.Hour {
		t.Errorf("Expected RetentionPeriod to be 24h, got %v", config.RetentionPeriod)
	}
}

func TestPerformanceAnalyzerBoundedOperations(t *testing.T) {
	// Create analyzer with small bounds for testing
	config := MetricsConfig{
		MaxOperations:   3,
		MaxCacheMetrics: 2,
		MaxPeerMetrics:  2,
		RetentionPeriod: time.Hour,
	}
	
	pa := NewPerformanceAnalyzerWithConfig(config)
	
	// Add more operations than the limit
	for i := 0; i < 5; i++ {
		pa.RecordOperation("store", time.Millisecond*100, 1024, true, "test", false)
	}
	
	opCount, _, _ := pa.GetCurrentMetrics()
	if opCount != 3 {
		t.Errorf("Expected operations count to be limited to 3, got %d", opCount)
	}
}

func TestPerformanceAnalyzerBoundedCacheMetrics(t *testing.T) {
	// Create analyzer with small bounds for testing
	config := MetricsConfig{
		MaxOperations:   10,
		MaxCacheMetrics: 2,
		MaxPeerMetrics:  10,
		RetentionPeriod: time.Hour,
	}
	
	pa := NewPerformanceAnalyzerWithConfig(config)
	
	// Add more cache metrics than the limit
	for i := 0; i < 4; i++ {
		metric := CacheMetric{
			Timestamp:   time.Now(),
			HitRate:     0.5,
			TotalBlocks: 100,
			HotTier:     10,
			WarmTier:    20,
			ColdTier:    70,
			Evictions:   int64(i),
		}
		pa.addCacheMetric(metric)
	}
	
	_, cacheCount, _ := pa.GetCurrentMetrics()
	if cacheCount != 2 {
		t.Errorf("Expected cache metrics count to be limited to 2, got %d", cacheCount)
	}
}

func TestPerformanceAnalyzerConfigUpdate(t *testing.T) {
	pa := NewPerformanceAnalyzer()
	
	originalConfig := pa.GetMetricsConfig()
	if originalConfig.MaxOperations != 10000 {
		t.Errorf("Expected default MaxOperations to be 10000, got %d", originalConfig.MaxOperations)
	}
	
	// Update configuration
	newConfig := MetricsConfig{
		MaxOperations:   5000,
		MaxCacheMetrics: 500,
		MaxPeerMetrics:  2500,
		RetentionPeriod: 12 * time.Hour,
	}
	
	pa.UpdateMetricsConfig(newConfig)
	
	updatedConfig := pa.GetMetricsConfig()
	if updatedConfig.MaxOperations != 5000 {
		t.Errorf("Expected updated MaxOperations to be 5000, got %d", updatedConfig.MaxOperations)
	}
	
	if updatedConfig.RetentionPeriod != 12*time.Hour {
		t.Errorf("Expected updated RetentionPeriod to be 12h, got %v", updatedConfig.RetentionPeriod)
	}
}

func TestPerformanceAnalyzerConcurrency(t *testing.T) {
	pa := NewPerformanceAnalyzer()
	
	// Test concurrent access
	done := make(chan bool)
	
	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			pa.RecordOperation("store", time.Millisecond*10, 1024, true, "test", false)
		}
		done <- true
	}()
	
	// Reader goroutine
	go func() {
		for i := 0; i < 10; i++ {
			pa.GetCurrentMetrics()
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()
	
	// Wait for both goroutines
	<-done
	<-done
	
	opCount, _, _ := pa.GetCurrentMetrics()
	if opCount != 100 {
		t.Errorf("Expected 100 operations, got %d", opCount)
	}
}

func TestPerformanceAnalyzerRetentionCleanup(t *testing.T) {
	// Create analyzer with very short retention period
	config := MetricsConfig{
		MaxOperations:   1000,
		MaxCacheMetrics: 100,
		MaxPeerMetrics:  100,
		RetentionPeriod: time.Millisecond * 10, // Very short for testing
	}
	
	pa := NewPerformanceAnalyzerWithConfig(config)
	
	// Add some operations
	pa.RecordOperation("store", time.Millisecond*10, 1024, true, "test", false)
	pa.RecordOperation("retrieve", time.Millisecond*20, 1024, true, "test", true)
	
	opCount, _, _ := pa.GetCurrentMetrics()
	if opCount != 2 {
		t.Errorf("Expected 2 operations before cleanup, got %d", opCount)
	}
	
	// Wait for retention period to pass
	time.Sleep(time.Millisecond * 15)
	
	// Trigger cleanup by collecting metrics
	pa.CollectMetrics()
	
	opCount, _, _ = pa.GetCurrentMetrics()
	if opCount != 0 {
		t.Errorf("Expected 0 operations after cleanup, got %d", opCount)
	}
}
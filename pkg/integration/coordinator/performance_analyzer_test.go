package integration

import (
	"testing"
	"time"
)

func TestPerformanceAnalyzerBoundedMetrics(t *testing.T) {
	config := &MetricsConfig{
		MaxOperations:   5, // Small limit for testing
		MaxCacheMetrics: 3,
		MaxPeerMetrics:  4,
		RetentionPeriod: time.Hour,
	}
	
	pa := NewPerformanceAnalyzerWithConfig(config)
	
	// Test operations don't exceed bounds
	for i := 0; i < 10; i++ {
		pa.RecordOperation("store", time.Millisecond*100, 1024, true, "test", false)
	}
	
	usage := pa.GetMetricsUsage()
	
	// Check that operations are bounded
	opsUsage := usage["operations"].(map[string]interface{})
	if opsUsage["count"].(int) > 5 {
		t.Errorf("Operations count exceeded max limit: got %d, max %d", opsUsage["count"], 5)
	}
	
	if !opsUsage["full"].(bool) {
		t.Error("Operations buffer should be marked as full")
	}
	
	// Test configuration update
	newConfig := &MetricsConfig{
		MaxOperations:   3, // Reduce limit
		MaxCacheMetrics: 2,
		MaxPeerMetrics:  2,
		RetentionPeriod: time.Hour,
	}
	
	pa.UpdateConfig(newConfig)
	updatedConfig := pa.GetConfig()
	
	if updatedConfig.MaxOperations != 3 {
		t.Errorf("Config update failed: expected MaxOperations=3, got %d", updatedConfig.MaxOperations)
	}
	
	// Test cleanup old metrics
	pa.CleanupOldMetrics()
	
	// All should pass - this validates the bounded implementation is working
	t.Log("Bounded metrics implementation working correctly")
}

func TestPerformanceAnalyzerMemoryBounds(t *testing.T) {
	// Test that metrics don't grow unbounded (memory leak prevention)
	pa := NewPerformanceAnalyzer() // Uses default limits
	
	// Add many operations to test bounds
	for i := 0; i < 15000; i++ { // More than default limit of 10k
		pa.RecordOperation("retrieve", time.Millisecond*50, 2048, true, "performance", i%2 == 0)
	}
	
	usage := pa.GetMetricsUsage()
	opsUsage := usage["operations"].(map[string]interface{})
	
	// Should never exceed configured maximum
	if opsUsage["count"].(int) > 10000 {
		t.Errorf("Memory leak detected: operations count %d exceeded limit %d", 
			opsUsage["count"], 10000)
	}
	
	// Buffer should be marked as full
	if !opsUsage["full"].(bool) {
		t.Error("Buffer should be marked as full to indicate circular buffer is active")
	}
	
	t.Logf("Memory bounds validated: %d operations stored (max: 10000)", opsUsage["count"].(int))
}
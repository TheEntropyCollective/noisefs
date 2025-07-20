package integration

import (
	"testing"
	"time"
)

func TestPerformanceAnalyzerBoundedMetrics(t *testing.T) {
	config := &MetricsConfig{
		MaxOperations:   3, // Small limit for testing
		MaxCacheMetrics: 2,
		MaxPeerMetrics:  2,
		RetentionPeriod: time.Hour,
	}
	
	pa := NewPerformanceAnalyzerWithConfig(config)
	
	// Test operations don't exceed bounds
	for i := 0; i < 8; i++ {
		pa.RecordOperation("retrieve", time.Millisecond*50, 512, true, "test", false)
	}
	
	usage := pa.GetMetricsUsage()
	
	// Check that operations are bounded
	opsUsage := usage["operations"].(map[string]interface{})
	if opsUsage["count"].(int) > 3 {
		t.Errorf("Operations count exceeded max limit: got %d, max %d", opsUsage["count"], 3)
	}
	
	if !opsUsage["full"].(bool) {
		t.Error("Operations buffer should be marked as full")
	}
	
	// Verify memory bounds are working
	t.Logf("Memory bounds working: %d operations stored (max: %d)", 
		opsUsage["count"].(int), 3)
}
package integration

import (
	"testing"
	"time"
)

func TestMetricsConfig(t *testing.T) {
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

func TestMetricsBoundedCollection(t *testing.T) {
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
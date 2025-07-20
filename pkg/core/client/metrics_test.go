package noisefs

import (
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
)

func TestMetrics_HealthMonitoring(t *testing.T) {
	metrics := NewMetrics()

	// Test initial health stats
	stats := metrics.GetStats()

	// Health fields should exist and be valid
	if stats.CacheHealthScore < 0 || stats.CacheHealthScore > 1 {
		t.Errorf("CacheHealthScore should be between 0 and 1, got %f", stats.CacheHealthScore)
	}

	if stats.RandomizerDiversity < 0 || stats.RandomizerDiversity > 1 {
		t.Errorf("RandomizerDiversity should be between 0 and 1, got %f", stats.RandomizerDiversity)
	}

	if stats.MemoryPressure < 0 || stats.MemoryPressure > 1 {
		t.Errorf("MemoryPressure should be between 0 and 1, got %f", stats.MemoryPressure)
	}

	if stats.CoordinationHealth < 0 || stats.CoordinationHealth > 1 {
		t.Errorf("CoordinationHealth should be between 0 and 1, got %f", stats.CoordinationHealth)
	}
}

func TestMetrics_HealthMetricsUpdate(t *testing.T) {
	metrics := NewMetrics()

	// Update with some health metrics
	healthMetrics := &cache.HealthMetrics{
		TotalBlocks:       100,
		EvictionCount:     5,
		LastEvictionTime:  time.Now(),
		MemoryUsageBytes:  1024 * 1024 * 50,  // 50MB
		TotalMemoryBytes:  1024 * 1024 * 100, // 100MB
		BlockAgeSum:       200,               // 2 hours average
		RandomizerCount:   50,
		UniqueRandomizers: 45,
	}

	metrics.UpdateHealthMetrics(healthMetrics)

	// Get updated stats
	stats := metrics.GetStats()

	// Verify the health metrics are reflected
	expectedAvgAge := 2.0 // 200/100 = 2 hours
	if stats.AverageBlockAge != expectedAvgAge {
		t.Errorf("Expected average block age of %f, got %f", expectedAvgAge, stats.AverageBlockAge)
	}

	expectedMemoryPressure := 0.5 // 50MB/100MB
	if stats.MemoryPressure != expectedMemoryPressure {
		t.Errorf("Expected memory pressure of %f, got %f", expectedMemoryPressure, stats.MemoryPressure)
	}

	expectedDiversity := 0.9 // 45/50
	if stats.RandomizerDiversity != expectedDiversity {
		t.Errorf("Expected randomizer diversity of %f, got %f", expectedDiversity, stats.RandomizerDiversity)
	}
}

func TestMetrics_EvictionRecording(t *testing.T) {
	metrics := NewMetrics()

	// Record baseline stats
	initialStats := metrics.GetStats()
	initialEvictionRate := initialStats.EvictionRate

	// Record an eviction
	metrics.RecordEviction()

	// Wait a small amount to ensure time calculation
	time.Sleep(1 * time.Millisecond)

	// Get updated stats
	newStats := metrics.GetStats()

	// Eviction rate should have changed (increased or at least not decreased)
	if newStats.EvictionRate < initialEvictionRate {
		t.Error("Eviction rate should not decrease after recording eviction")
	}
}

func TestMetrics_HealthSummary(t *testing.T) {
	metrics := NewMetrics()

	// Test initial summary
	summary := metrics.GetHealthSummary()
	validSummaries := []string{"Excellent", "Good", "Fair", "Poor", "Critical"}

	found := false
	for _, valid := range validSummaries {
		if summary == valid {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Health summary should be one of %v, got %s", validSummaries, summary)
	}

	// Test with good health metrics
	goodMetrics := &cache.HealthMetrics{
		TotalBlocks:       100,
		EvictionCount:     0,
		MemoryUsageBytes:  1024 * 1024 * 20,  // 20MB
		TotalMemoryBytes:  1024 * 1024 * 100, // 100MB
		BlockAgeSum:       50,                // 0.5 hours average
		RandomizerCount:   50,
		UniqueRandomizers: 50, // Perfect diversity
	}

	metrics.UpdateHealthMetrics(goodMetrics)
	goodSummary := metrics.GetHealthSummary()

	if goodSummary != "Excellent" && goodSummary != "Good" {
		t.Errorf("Expected good health summary with good metrics, got %s", goodSummary)
	}
}

func TestMetrics_ComponentIntegration(t *testing.T) {
	metrics := NewMetrics()

	// Create mock components
	config := &cache.BloomExchangeConfig{
		MinPeersForCoordination: 3,
	}
	coordEngine := cache.NewCoordinationEngine(config)
	healthTracker := cache.NewBlockHealthTracker(nil)

	// Set components
	metrics.SetHealthMonitorComponents(coordEngine, healthTracker)

	// Verify integration works
	stats := metrics.GetStats()

	// Should have coordination health (even if 0)
	if stats.CoordinationHealth < 0 || stats.CoordinationHealth > 1 {
		t.Errorf("CoordinationHealth should be between 0 and 1 with components set, got %f", stats.CoordinationHealth)
	}
}

func TestMetrics_ConcurrentAccess(t *testing.T) {
	metrics := NewMetrics()

	// Test concurrent access doesn't panic
	done := make(chan bool, 3)

	// Goroutine 1: Record metrics
	go func() {
		for i := 0; i < 100; i++ {
			metrics.RecordBlockReuse()
			metrics.RecordCacheHit()
		}
		done <- true
	}()

	// Goroutine 2: Record evictions
	go func() {
		for i := 0; i < 50; i++ {
			metrics.RecordEviction()
		}
		done <- true
	}()

	// Goroutine 3: Get stats
	go func() {
		for i := 0; i < 100; i++ {
			stats := metrics.GetStats()
			_ = stats // Prevent optimization
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 3; i++ {
		<-done
	}

	// Verify final state is consistent
	finalStats := metrics.GetStats()
	if finalStats.BlocksReused != 100 {
		t.Errorf("Expected 100 blocks reused, got %d", finalStats.BlocksReused)
	}

	if finalStats.CacheHits != 100 {
		t.Errorf("Expected 100 cache hits, got %d", finalStats.CacheHits)
	}
}

func BenchmarkMetrics_GetStatsWithHealth(b *testing.B) {
	metrics := NewMetrics()

	// Set up realistic health metrics
	healthMetrics := &cache.HealthMetrics{
		TotalBlocks:       1000,
		EvictionCount:     25,
		MemoryUsageBytes:  1024 * 1024 * 512,
		TotalMemoryBytes:  1024 * 1024 * 1024,
		BlockAgeSum:       5000,
		RandomizerCount:   500,
		UniqueRandomizers: 450,
	}

	metrics.UpdateHealthMetrics(healthMetrics)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		stats := metrics.GetStats()
		_ = stats // Prevent optimization
	}
}

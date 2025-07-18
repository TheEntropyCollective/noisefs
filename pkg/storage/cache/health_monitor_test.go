package cache

import (
	"testing"
	"time"
)

func TestCacheHealthMonitor_BasicFunctionality(t *testing.T) {
	monitor := NewCacheHealthMonitor()
	
	// Test initial state
	score := monitor.CalculateHealthScore()
	if score.Overall < 0 || score.Overall > 1 {
		t.Errorf("Overall score should be between 0 and 1, got %f", score.Overall)
	}
	
	// Test metrics update
	metrics := &HealthMetrics{
		TotalBlocks:       100,
		EvictionCount:     5,
		LastEvictionTime:  time.Now(),
		MemoryUsageBytes:  1024 * 1024 * 50, // 50MB
		TotalMemoryBytes:  1024 * 1024 * 100, // 100MB
		BlockAgeSum:       300, // 300 hours total
		RandomizerCount:   50,
		UniqueRandomizers: 45,
	}
	
	monitor.UpdateMetrics(metrics)
	
	// Recalculate score
	score = monitor.CalculateHealthScore()
	
	// Test individual components
	if score.RandomizerDiversity <= 0 || score.RandomizerDiversity > 1 {
		t.Errorf("Randomizer diversity should be between 0 and 1, got %f", score.RandomizerDiversity)
	}
	
	if score.AverageBlockAge != 3.0 { // 300/100 = 3 hours
		t.Errorf("Expected average block age of 3.0, got %f", score.AverageBlockAge)
	}
	
	if score.MemoryPressure != 0.5 { // 50MB/100MB = 0.5
		t.Errorf("Expected memory pressure of 0.5, got %f", score.MemoryPressure)
	}
	
	// Test eviction recording
	initialEvictionCount := metrics.EvictionCount
	monitor.RecordEviction()
	
	// Wait a small amount of time to ensure elapsed time calculation
	time.Sleep(1 * time.Millisecond)
	
	newScore := monitor.CalculateHealthScore()
	
	// Check that eviction was recorded (the rate depends on elapsed time)
	if newScore.EvictionRate < score.EvictionRate {
		t.Error("Eviction rate should not decrease after recording eviction")
	}
	
	// Verify the eviction count increased internally
	monitor.mu.RLock()
	currentCount := monitor.metrics.EvictionCount
	monitor.mu.RUnlock()
	
	if currentCount != initialEvictionCount+1 {
		t.Errorf("Expected eviction count to increase by 1, got %d -> %d", initialEvictionCount, currentCount)
	}
}

func TestCacheHealthMonitor_HealthSummary(t *testing.T) {
	monitor := NewCacheHealthMonitor()
	
	tests := []struct {
		name     string
		metrics  *HealthMetrics
		expected string
	}{
		{
			name: "excellent_health",
			metrics: &HealthMetrics{
				TotalBlocks:       100,
				EvictionCount:     0,
				MemoryUsageBytes:  1024 * 1024 * 20, // 20MB
				TotalMemoryBytes:  1024 * 1024 * 100, // 100MB
				BlockAgeSum:       50, // 0.5 hours average
				RandomizerCount:   50,
				UniqueRandomizers: 50, // Perfect diversity
			},
			expected: "Excellent",
		},
		{
			name: "poor_health",
			metrics: &HealthMetrics{
				TotalBlocks:       100,
				EvictionCount:     50, // High eviction rate
				MemoryUsageBytes:  1024 * 1024 * 95, // 95MB
				TotalMemoryBytes:  1024 * 1024 * 100, // 100MB
				BlockAgeSum:       2400, // 24 hours average (very old)
				RandomizerCount:   50,
				UniqueRandomizers: 5, // Poor diversity
			},
			expected: "Poor",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			monitor.UpdateMetrics(tt.metrics)
			summary := monitor.GetHealthSummary()
			
			if summary != tt.expected {
				t.Errorf("Expected health summary %s, got %s", tt.expected, summary)
			}
		})
	}
}

func TestCacheHealthMonitor_RandomizerDiversity(t *testing.T) {
	monitor := NewCacheHealthMonitor()
	
	tests := []struct {
		name              string
		randomizerCount   int64
		uniqueRandomizers int64
		expectedDiversity float64
	}{
		{"perfect_diversity", 10, 10, 1.0},
		{"no_diversity", 10, 1, 0.1},
		{"moderate_diversity", 10, 5, 0.5},
		{"no_randomizers", 0, 0, 0.0},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := &HealthMetrics{
				RandomizerCount:   tt.randomizerCount,
				UniqueRandomizers: tt.uniqueRandomizers,
			}
			
			monitor.UpdateMetrics(metrics)
			score := monitor.CalculateHealthScore()
			
			if score.RandomizerDiversity != tt.expectedDiversity {
				t.Errorf("Expected diversity %f, got %f", tt.expectedDiversity, score.RandomizerDiversity)
			}
		})
	}
}

func TestCacheHealthMonitor_EvictionRateCalculation(t *testing.T) {
	// Create monitor with known start time
	monitor := &CacheHealthMonitor{
		metrics:   &HealthMetrics{},
		startTime: time.Now().Add(-2 * time.Hour), // 2 hours ago
	}
	
	// Record 10 evictions over 2 hours = 5 evictions/hour
	metrics := &HealthMetrics{
		EvictionCount: 10,
	}
	
	monitor.UpdateMetrics(metrics)
	score := monitor.CalculateHealthScore()
	
	expectedRate := 5.0 // 10 evictions / 2 hours
	tolerance := 0.1
	
	if score.EvictionRate < expectedRate-tolerance || score.EvictionRate > expectedRate+tolerance {
		t.Errorf("Expected eviction rate around %f, got %f", expectedRate, score.EvictionRate)
	}
}

func TestCacheHealthMonitor_WithCoordination(t *testing.T) {
	monitor := NewCacheHealthMonitor()
	
	// Create mock coordination engine
	config := &BloomExchangeConfig{
		MinPeersForCoordination: 3,
	}
	coordEngine := NewCoordinationEngine(config)
	monitor.SetCoordinationEngine(coordEngine)
	
	// Test coordination health score
	score := monitor.CalculateHealthScore()
	
	// Should have coordination health score
	if score.CoordinationHealth < 0 || score.CoordinationHealth > 1 {
		t.Errorf("Coordination health should be between 0 and 1, got %f", score.CoordinationHealth)
	}
}
package cache

import (
	"sync"
	"testing"
	"time"
)

func BenchmarkCacheHealthMonitor_CalculateHealthScore(b *testing.B) {
	monitor := NewCacheHealthMonitor()
	
	// Set up realistic metrics
	metrics := &HealthMetrics{
		TotalBlocks:       1000,
		EvictionCount:     25,
		LastEvictionTime:  time.Now(),
		MemoryUsageBytes:  1024 * 1024 * 512, // 512MB
		TotalMemoryBytes:  1024 * 1024 * 1024, // 1GB
		BlockAgeSum:       5000, // 5 hours average
		RandomizerCount:   500,
		UniqueRandomizers: 450,
	}
	
	monitor.UpdateMetrics(metrics)
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		monitor.CalculateHealthScore()
	}
}

func BenchmarkCacheHealthMonitor_ConcurrentAccess(b *testing.B) {
	monitor := NewCacheHealthMonitor()
	
	metrics := &HealthMetrics{
		TotalBlocks:       1000,
		EvictionCount:     25,
		RandomizerCount:   500,
		UniqueRandomizers: 450,
	}
	
	monitor.UpdateMetrics(metrics)
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Simulate concurrent reads and writes
			if b.N%2 == 0 {
				monitor.CalculateHealthScore()
			} else {
				monitor.RecordEviction()
			}
		}
	})
}

func BenchmarkCacheHealthMonitor_MetricsUpdate(b *testing.B) {
	monitor := NewCacheHealthMonitor()
	
	metrics := &HealthMetrics{
		TotalBlocks:       1000,
		EvictionCount:     25,
		MemoryUsageBytes:  1024 * 1024 * 512,
		TotalMemoryBytes:  1024 * 1024 * 1024,
		BlockAgeSum:       5000,
		RandomizerCount:   500,
		UniqueRandomizers: 450,
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		monitor.UpdateMetrics(metrics)
	}
}

func BenchmarkCacheHealthMonitor_HealthSummary(b *testing.B) {
	monitor := NewCacheHealthMonitor()
	
	metrics := &HealthMetrics{
		TotalBlocks:       1000,
		EvictionCount:     25,
		RandomizerCount:   500,
		UniqueRandomizers: 450,
	}
	
	monitor.UpdateMetrics(metrics)
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		monitor.GetHealthSummary()
	}
}

func BenchmarkCacheHealthMonitor_ParallelEvictionRecording(b *testing.B) {
	monitor := NewCacheHealthMonitor()
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			monitor.RecordEviction()
		}
	})
}

// Benchmark the impact on metrics snapshot creation
func BenchmarkMetricsSnapshot_WithHealthMonitoring(b *testing.B) {
	// Create metrics without health monitoring
	metricsWithoutHealth := &struct {
		BlocksReused    int64
		BlocksGenerated int64
		CacheHits       int64
		CacheMisses     int64
		mu              sync.RWMutex
	}{
		BlocksReused:    100,
		BlocksGenerated: 50,
		CacheHits:       300,
		CacheMisses:     50,
	}
	
	// Create metrics with health monitoring
	metricsWithHealth := NewCacheHealthMonitor()
	healthMetrics := &HealthMetrics{
		TotalBlocks:       150,
		RandomizerCount:   75,
		UniqueRandomizers: 70,
	}
	metricsWithHealth.UpdateMetrics(healthMetrics)
	
	b.Run("WithoutHealthMonitoring", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			metricsWithoutHealth.mu.RLock()
			_ = metricsWithoutHealth.BlocksReused + metricsWithoutHealth.BlocksGenerated
			metricsWithoutHealth.mu.RUnlock()
		}
	})
	
	b.Run("WithHealthMonitoring", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			metricsWithHealth.CalculateHealthScore()
		}
	})
}

// Test memory allocation during health score calculation
func BenchmarkCacheHealthMonitor_MemoryAllocations(b *testing.B) {
	monitor := NewCacheHealthMonitor()
	
	metrics := &HealthMetrics{
		TotalBlocks:       1000,
		RandomizerCount:   500,
		UniqueRandomizers: 450,
	}
	
	monitor.UpdateMetrics(metrics)
	
	b.ReportAllocs()
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		score := monitor.CalculateHealthScore()
		_ = score // Prevent optimization
	}
}
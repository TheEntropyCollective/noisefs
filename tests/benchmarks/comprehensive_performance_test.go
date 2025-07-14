package main

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/logging"
)

// BenchmarkConcurrentLoad tests cache performance under concurrent load
func BenchmarkConcurrentLoad(b *testing.B) {
	concurrencyLevels := []int{1, 10, 50, 100, 500, 1000}
	
	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("Concurrency-%d", concurrency), func(b *testing.B) {
			baseCache := cache.NewMemoryCache(10000)
			perfMonitor := cache.NewPerformanceMonitor(baseCache, logging.NewLogger(nil))
			
			// Pre-populate cache
			blockData := make([]byte, 4096)
			testBlock, _ := blocks.NewBlock(blockData)
			for i := 0; i < 1000; i++ {
				perfMonitor.Store(fmt.Sprintf("block-%d", i), testBlock)
			}
			
			// Set performance baseline
			perfMonitor.SetBaseline()
			
			b.ResetTimer()
			b.SetParallelism(concurrency)
			
			b.RunParallel(func(pb *testing.PB) {
				blockID := 0
				for pb.Next() {
					cid := fmt.Sprintf("block-%d", blockID%1000)
					
					// Mix of operations
					switch blockID % 4 {
					case 0, 1: // 50% reads
						perfMonitor.Get(cid)
					case 2: // 25% writes
						perfMonitor.Store(cid, testBlock)
					case 3: // 25% popularity updates
						perfMonitor.IncrementPopularity(cid)
					}
					
					blockID++
				}
			})
			
			// Report performance metrics
			snapshot := perfMonitor.GetPerformanceSnapshot()
			alert, ratio, _ := perfMonitor.GetRegressionStatus()
			
			b.Logf("Concurrency %d: %.0f ops/sec, %.2fμs avg latency, hit rate %.1f%%, regression: %s (%.2fx)",
				concurrency, snapshot.OperationsPerSecond, 
				float64(snapshot.AvgGetLatency.Nanoseconds())/1000.0,
				snapshot.HitRate*100, 
				map[bool]string{true: "YES", false: "NO"}[alert], ratio)
		})
	}
}

// BenchmarkEvictionStrategiesUnderLoad tests eviction performance under load
func BenchmarkEvictionStrategiesUnderLoad(b *testing.B) {
	strategies := []string{"LRU", "LFU", "ValueBased", "Adaptive"}
	
	for _, strategy := range strategies {
		b.Run(strategy, func(b *testing.B) {
			// Create small cache to force evictions
			baseCache := cache.NewMemoryCache(100)
			config := &cache.AltruisticCacheConfig{
				MinPersonalCache:  10 * 1024 * 1024,
				EnableAltruistic:  true,
				EvictionStrategy:  strategy,
				EvictionCooldown:  100 * time.Millisecond,
			}
			altruisticCache := cache.NewAltruisticCache(baseCache, config, 20*1024*1024)
			perfMonitor := cache.NewPerformanceMonitor(altruisticCache, logging.NewLogger(nil))
			
			blockData := make([]byte, 4096)
			testBlock, _ := blocks.NewBlock(blockData)
			
			perfMonitor.SetBaseline()
			
			b.ResetTimer()
			
			for i := 0; i < b.N; i++ {
				cid := fmt.Sprintf("block-%d", i)
				perfMonitor.Store(cid, testBlock)
				
				// Periodically access old blocks to create patterns
				if i%10 == 0 && i > 0 {
					oldCid := fmt.Sprintf("block-%d", i-10)
					perfMonitor.Get(oldCid)
				}
			}
			
			snapshot := perfMonitor.GetPerformanceSnapshot()
			b.Logf("%s: %.2fμs store, %.2fμs get, %.2fμs evict",
				strategy,
				float64(snapshot.AvgStoreLatency.Nanoseconds())/1000.0,
				float64(snapshot.AvgGetLatency.Nanoseconds())/1000.0,
				float64(snapshot.AvgEvictLatency.Nanoseconds())/1000.0)
		})
	}
}

// BenchmarkMetricsScaling tests how metrics overhead scales with cache size
func BenchmarkMetricsScaling(b *testing.B) {
	cacheSizes := []int{100, 1000, 10000, 100000}
	
	for _, size := range cacheSizes {
		b.Run(fmt.Sprintf("CacheSize-%d", size), func(b *testing.B) {
			// Test with sampled metrics
			baseCache := cache.NewMemoryCache(size)
			config := cache.DefaultSampledStatsConfig()
			statsCache := cache.NewSampledStatisticsCache(baseCache, config, logging.NewLogger(nil))
			
			blockData := make([]byte, 4096)
			testBlock, _ := blocks.NewBlock(blockData)
			
			// Pre-populate cache
			for i := 0; i < size/2; i++ {
				statsCache.Store(fmt.Sprintf("block-%d", i), testBlock)
			}
			
			b.ResetTimer()
			
			for i := 0; i < b.N; i++ {
				cid := fmt.Sprintf("block-%d", i%(size/2))
				
				// Mix of operations
				if i%3 == 0 {
					statsCache.Store(cid, testBlock)
				} else {
					statsCache.Get(cid)
				}
			}
			
			efficiency := statsCache.GetEfficiencyStats()
			b.Logf("Cache size %d: %.2f%% sampling efficiency, %d popular blocks tracked",
				size, efficiency["sampling_efficiency"], efficiency["popular_blocks_tracked"])
		})
	}
}

// BenchmarkMemoryPressure tests cache behavior under memory pressure
func BenchmarkMemoryPressure(b *testing.B) {
	baseCache := cache.NewMemoryCache(10000)
	perfMonitor := cache.NewPerformanceMonitor(baseCache, logging.NewLogger(nil))
	
	// Create blocks of different sizes to simulate memory pressure
	blockSizes := []int{1024, 4096, 16384, 65536} // 1KB to 64KB
	
	perfMonitor.SetBaseline()
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		size := blockSizes[i%len(blockSizes)]
		blockData := make([]byte, size)
		testBlock, _ := blocks.NewBlock(blockData)
		
		cid := fmt.Sprintf("block-%d-%d", i, size)
		perfMonitor.Store(cid, testBlock)
		
		// Trigger some GC pressure
		if i%100 == 0 {
			_ = make([]byte, 1024*1024) // Allocate 1MB to trigger GC
		}
	}
	
	snapshot := perfMonitor.GetPerformanceSnapshot()
	alert, ratio, _ := perfMonitor.GetRegressionStatus()
	
	b.Logf("Memory pressure test: %.1fMB heap, %d goroutines, regression: %t (%.2fx)",
		snapshot.MemoryUsageMB, snapshot.NumGoroutines, alert, ratio)
}

// BenchmarkCacheHitRateOptimization validates hit rate remains high under optimization
func BenchmarkCacheHitRateOptimization(b *testing.B) {
	// Test optimized cache with score caching
	baseCache := cache.NewMemoryCache(1000)
	config := &cache.AltruisticCacheConfig{
		MinPersonalCache: 50 * 1024 * 1024,
		EnableAltruistic: true,
		EvictionStrategy: "ValueBased", // Most optimized strategy
	}
	altruisticCache := cache.NewAltruisticCache(baseCache, config, 100*1024*1024)
	perfMonitor := cache.NewPerformanceMonitor(altruisticCache, logging.NewLogger(nil))
	
	blockData := make([]byte, 4096)
	testBlock, _ := blocks.NewBlock(blockData)
	
	// Pre-populate with working set
	workingSetSize := 500
	for i := 0; i < workingSetSize; i++ {
		perfMonitor.Store(fmt.Sprintf("block-%d", i), testBlock)
	}
	
	perfMonitor.SetBaseline()
	
	b.ResetTimer()
	
	hitCount := 0
	totalCount := 0
	
	for i := 0; i < b.N; i++ {
		totalCount++
		
		// 80% access to working set (should be hits), 20% new blocks
		var cid string
		if i%5 < 4 {
			cid = fmt.Sprintf("block-%d", i%workingSetSize)
		} else {
			cid = fmt.Sprintf("new-block-%d", i)
		}
		
		block, err := perfMonitor.Get(cid)
		if err == nil && block != nil {
			hitCount++
		}
		
		// Store new blocks
		if i%5 == 4 {
			perfMonitor.Store(cid, testBlock)
		}
	}
	
	hitRate := float64(hitCount) / float64(totalCount) * 100
	snapshot := perfMonitor.GetPerformanceSnapshot()
	
	b.Logf("Optimized cache hit rate: %.1f%% (target: ≥80%%), avg latency: %.2fμs",
		hitRate, float64(snapshot.AvgGetLatency.Nanoseconds())/1000.0)
	
	if hitRate < 80.0 {
		b.Errorf("Hit rate %.1f%% below target 80%%", hitRate)
	}
}

// BenchmarkStressTest runs a comprehensive stress test
func BenchmarkStressTest(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping stress test in short mode")
	}
	
	// Create complex cache setup
	baseCache := cache.NewMemoryCache(5000)
	config := &cache.AltruisticCacheConfig{
		MinPersonalCache:      100 * 1024 * 1024,
		EnableAltruistic:      true,
		EvictionStrategy:      "Adaptive",
		EnableGradualEviction: true,
		EnablePredictive:      true,
	}
	altruisticCache := cache.NewAltruisticCache(baseCache, config, 200*1024*1024)
	
	// Layer with sampled statistics
	sampledConfig := &cache.SampledStatsConfig{
		SampleRate:           0.05, // 5% sampling for stress test
		PopularitySampleRate: 0.02, // 2% popularity sampling
		LatencySampleRate:    0.05, // 5% latency sampling
		UseApproximation:     true,
		MaxPopularBlocks:     500,
	}
	statsCache := cache.NewSampledStatisticsCache(altruisticCache, sampledConfig, logging.NewLogger(nil))
	
	// Top layer with performance monitoring
	perfMonitor := cache.NewPerformanceMonitor(statsCache, logging.NewLogger(nil))
	
	blockData := make([]byte, 4096)
	testBlock, _ := blocks.NewBlock(blockData)
	
	perfMonitor.SetBaseline()
	
	// Run stress test with multiple concurrent workers
	numWorkers := 100
	operationsPerWorker := b.N / numWorkers
	if operationsPerWorker < 1 {
		operationsPerWorker = 1
	}
	
	b.ResetTimer()
	
	var wg sync.WaitGroup
	start := time.Now()
	
	for worker := 0; worker < numWorkers; worker++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			for i := 0; i < operationsPerWorker; i++ {
				opID := workerID*operationsPerWorker + i
				cid := fmt.Sprintf("stress-block-%d", opID%2000)
				
				switch opID % 10 {
				case 0, 1, 2, 3: // 40% reads
					perfMonitor.Get(cid)
				case 4, 5: // 20% writes
					perfMonitor.Store(cid, testBlock)
				case 6: // 10% popularity updates
					perfMonitor.IncrementPopularity(cid)
				case 7: // 10% has checks
					perfMonitor.Has(cid)
				case 8: // 10% randomizer requests
					perfMonitor.GetRandomizers(5)
				case 9: // 10% removals
					if opID > 100 { // Don't remove initially
						oldCid := fmt.Sprintf("stress-block-%d", (opID-100)%2000)
						perfMonitor.Remove(oldCid)
					}
				}
			}
		}(worker)
	}
	
	wg.Wait()
	elapsed := time.Since(start)
	
	// Report comprehensive results
	snapshot := perfMonitor.GetPerformanceSnapshot()
	alert, ratio, message := perfMonitor.GetRegressionStatus()
	efficiency := statsCache.GetEfficiencyStats()
	altruisticStats := altruisticCache.GetAltruisticStats()
	
	totalOps := float64(b.N)
	actualThroughput := totalOps / elapsed.Seconds()
	
	b.Logf("STRESS TEST RESULTS:")
	b.Logf("  Duration: %v", elapsed)
	b.Logf("  Throughput: %.0f ops/sec (measured: %.0f)", actualThroughput, snapshot.OperationsPerSecond)
	b.Logf("  Latencies: Get=%.2fμs, Store=%.2fμs, Evict=%.2fμs",
		float64(snapshot.AvgGetLatency.Nanoseconds())/1000.0,
		float64(snapshot.AvgStoreLatency.Nanoseconds())/1000.0,
		float64(snapshot.AvgEvictLatency.Nanoseconds())/1000.0)
	b.Logf("  Hit Rate: %.1f%%", snapshot.HitRate*100)
	b.Logf("  Memory: %.1fMB heap, %d goroutines", snapshot.MemoryUsageMB, snapshot.NumGoroutines)
	b.Logf("  Cache: %.1f%% utilization", snapshot.CacheUtilization*100)
	b.Logf("  Sampling: %.1f%% efficiency", efficiency["sampling_efficiency"])
	b.Logf("  Altruistic: %.1f%% flex pool usage", altruisticStats.FlexPoolUsage*100)
	b.Logf("  Regression: %s (%.2fx)", message, ratio)
	
	// Performance targets
	if snapshot.HitRate < 0.7 {
		b.Errorf("Hit rate %.1f%% below target 70%%", snapshot.HitRate*100)
	}
	if actualThroughput < 10000 {
		b.Errorf("Throughput %.0f ops/sec below target 10,000", actualThroughput)
	}
	if alert && ratio > 2.0 {
		b.Errorf("Severe performance regression: %.2fx worse than baseline", ratio)
	}
}
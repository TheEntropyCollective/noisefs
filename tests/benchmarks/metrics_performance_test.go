package benchmarks

import (
	"fmt"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/logging"
)

// BenchmarkMetricsOverhead compares full vs sampled metrics performance
func BenchmarkMetricsOverhead(b *testing.B) {
	// Create test block data
	blockData := make([]byte, 4096)
	testBlock, _ := blocks.NewBlock(blockData)
	
	logger := logging.NewLogger(nil)
	
	b.Run("FullMetrics", func(b *testing.B) {
		baseCache := cache.NewMemoryCache(1000)
		statsCache := cache.NewStatisticsCache(baseCache, logger)
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cid := fmt.Sprintf("block-%d", i%100) // Reuse some CIDs
			
			// Simulate cache operations
			statsCache.Store(cid, testBlock)
			statsCache.Get(cid)
			if i%10 == 0 {
				statsCache.Remove(cid)
			}
		}
	})
	
	b.Run("SampledMetrics-10%", func(b *testing.B) {
		baseCache := cache.NewMemoryCache(1000)
		config := &cache.SampledStatsConfig{
			SampleRate:           0.1,  // 10% sampling
			PopularitySampleRate: 0.05, // 5% popularity sampling
			LatencySampleRate:    0.1,  // 10% latency sampling
			MinSampleInterval:    time.Microsecond, // Allow frequent sampling for benchmark
			UseApproximation:     true,
			MaxPopularBlocks:     100,
		}
		statsCache := cache.NewSampledStatisticsCache(baseCache, config, logger)
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cid := fmt.Sprintf("block-%d", i%100) // Reuse some CIDs
			
			// Simulate cache operations
			statsCache.Store(cid, testBlock)
			statsCache.Get(cid)
			if i%10 == 0 {
				statsCache.Remove(cid)
			}
		}
	})
	
	b.Run("SampledMetrics-1%", func(b *testing.B) {
		baseCache := cache.NewMemoryCache(1000)
		config := &cache.SampledStatsConfig{
			SampleRate:           0.01, // 1% sampling
			PopularitySampleRate: 0.01, // 1% popularity sampling
			LatencySampleRate:    0.01, // 1% latency sampling
			MinSampleInterval:    time.Microsecond,
			UseApproximation:     true,
			MaxPopularBlocks:     50,
		}
		statsCache := cache.NewSampledStatisticsCache(baseCache, config, logger)
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cid := fmt.Sprintf("block-%d", i%100) // Reuse some CIDs
			
			// Simulate cache operations
			statsCache.Store(cid, testBlock)
			statsCache.Get(cid)
			if i%10 == 0 {
				statsCache.Remove(cid)
			}
		}
	})
	
	b.Run("NoMetrics", func(b *testing.B) {
		baseCache := cache.NewMemoryCache(1000)
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cid := fmt.Sprintf("block-%d", i%100) // Reuse some CIDs
			
			// Simulate cache operations
			baseCache.Store(cid, testBlock)
			baseCache.Get(cid)
			if i%10 == 0 {
				baseCache.Remove(cid)
			}
		}
	})
}

// BenchmarkMetricsAccuracy tests how well sampling preserves accuracy
func BenchmarkMetricsAccuracy(b *testing.B) {
	// This benchmark will help us validate that sampling maintains reasonable accuracy
	blockData := make([]byte, 4096)
	testBlock, _ := blocks.NewBlock(blockData)
	logger := logging.NewLogger(nil)
	
	baseCache1 := cache.NewMemoryCache(1000)
	fullStats := cache.NewStatisticsCache(baseCache1, logger)
	
	baseCache2 := cache.NewMemoryCache(1000)
	config := &cache.SampledStatsConfig{
		SampleRate:           0.1,
		PopularitySampleRate: 0.1,
		LatencySampleRate:    0.1,
		MinSampleInterval:    time.Microsecond,
		UseApproximation:     true,
		MaxPopularBlocks:     100,
	}
	sampledStats := cache.NewSampledStatisticsCache(baseCache2, config, logger)
	
	// Perform the same operations on both
	operations := 10000
	for i := 0; i < operations; i++ {
		cid := fmt.Sprintf("block-%d", i%100)
		
		fullStats.Store(cid, testBlock)
		sampledStats.Store(cid, testBlock)
		
		fullStats.Get(cid)
		sampledStats.Get(cid)
		
		if i%10 == 0 {
			fullStats.Remove(cid)
			sampledStats.Remove(cid)
		}
	}
	
	// Compare results
	fullSnapshot := fullStats.GetStats()
	sampledSnapshot := sampledStats.GetSampledStats()
	efficiency := sampledStats.GetEfficiencyStats()
	
	b.Logf("Full stats - Hits: %d, Misses: %d, Hit Rate: %.2f%%", 
		fullSnapshot.Hits, fullSnapshot.Misses, fullSnapshot.HitRate*100)
	b.Logf("Sampled stats - Hits: %d, Misses: %d, Hit Rate: %.2f%%", 
		sampledSnapshot.Hits, sampledSnapshot.Misses, sampledSnapshot.HitRate*100)
	b.Logf("Sampling efficiency: %.2f%%, Latency samples: %v", 
		efficiency["sampling_efficiency"], efficiency["latency_samples"])
		
	// Calculate accuracy
	hitRateError := abs(fullSnapshot.HitRate - sampledSnapshot.HitRate) / fullSnapshot.HitRate * 100
	b.Logf("Hit rate accuracy: %.2f%% error", hitRateError)
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// BenchmarkPopularityTracking compares full vs sampled popularity tracking
func BenchmarkPopularityTracking(b *testing.B) {
	blockData := make([]byte, 4096)
	testBlock, _ := blocks.NewBlock(blockData)
	logger := logging.NewLogger(nil)
	
	b.Run("FullPopularityTracking", func(b *testing.B) {
		baseCache := cache.NewMemoryCache(1000)
		statsCache := cache.NewStatisticsCache(baseCache, logger)
		
		// Pre-populate cache
		for i := 0; i < 1000; i++ {
			cid := fmt.Sprintf("block-%d", i)
			statsCache.Store(cid, testBlock)
		}
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cid := fmt.Sprintf("block-%d", i%1000)
			statsCache.Get(cid) // This triggers popularity tracking
		}
	})
	
	b.Run("SampledPopularityTracking", func(b *testing.B) {
		baseCache := cache.NewMemoryCache(1000)
		config := cache.DefaultSampledStatsConfig()
		statsCache := cache.NewSampledStatisticsCache(baseCache, config, logger)
		
		// Pre-populate cache
		for i := 0; i < 1000; i++ {
			cid := fmt.Sprintf("block-%d", i)
			statsCache.Store(cid, testBlock)
		}
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cid := fmt.Sprintf("block-%d", i%1000)
			statsCache.Get(cid) // This triggers popularity tracking
		}
	})
}
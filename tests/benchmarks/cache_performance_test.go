package benchmarks

import (
	"fmt"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
)

// BenchmarkCacheEfficiency tests the benefit of score caching
func BenchmarkCacheEfficiency(b *testing.B) {
	// Create test metadata
	testBlocks := make([]*cache.BlockMetadata, 100)
	for i := 0; i < 100; i++ {
		blockInfo := &cache.BlockInfo{
			CID:        fmt.Sprintf("test-block-%d", i),
			Size:       4096,
			Popularity: i % 100,
		}
		testBlocks[i] = cache.NewBlockMetadata(blockInfo, cache.PersonalBlock)
		testBlocks[i].CachedAt = time.Now().Add(-time.Duration(i) * time.Minute)
		testBlocks[i].LastAccessed = time.Now().Add(-time.Duration(i%50) * time.Minute)
	}

	healthTracker := cache.NewBlockHealthTracker(cache.DefaultBlockHealthConfig())
	strategy := cache.NewValueBasedEvictionStrategy()

	b.Run("WithCaching", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Simulate multiple eviction decisions on same blocks
			for _, block := range testBlocks {
				strategy.Score(block, healthTracker)
			}
			// Second pass - should hit cache
			for _, block := range testBlocks {
				strategy.Score(block, healthTracker)
			}
		}
	})

	// Disable caching for comparison
	b.Run("WithoutCaching", func(b *testing.B) {
		for _, block := range testBlocks {
			block.ClearCachedScores() // Clear any existing cache
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Simulate eviction decisions without caching
			for _, block := range testBlocks {
				block.ClearCachedScores() // Force recalculation
				strategy.Score(block, healthTracker)
			}
			for _, block := range testBlocks {
				block.ClearCachedScores() // Force recalculation
				strategy.Score(block, healthTracker)
			}
		}
	})
}

// BenchmarkEvictionFullWorkflow tests complete eviction scenarios
func BenchmarkEvictionFullWorkflow(b *testing.B) {
	strategies := map[string]cache.EvictionStrategy{
		"LRU-Optimized":        &cache.LRUEvictionStrategy{},
		"LFU-Optimized":        &cache.LFUEvictionStrategy{},
		"ValueBased-Optimized": cache.NewValueBasedEvictionStrategy(),
		"Adaptive-Optimized":   cache.NewAdaptiveEvictionStrategy(),
	}

	for name, strategy := range strategies {
		b.Run(name, func(b *testing.B) {
			testBlocks := make(map[string]*cache.BlockMetadata)
			for i := 0; i < 1000; i++ {
				cid := fmt.Sprintf("test-block-%d", i)
				blockInfo := &cache.BlockInfo{
					CID:        cid,
					Size:       4096,
					Popularity: i % 100,
				}
				metadata := cache.NewBlockMetadata(blockInfo, cache.PersonalBlock)
				metadata.CachedAt = time.Now().Add(-time.Duration(i) * time.Minute)
				metadata.LastAccessed = time.Now().Add(-time.Duration(i%50) * time.Minute)
				testBlocks[cid] = metadata
			}

			healthTracker := cache.NewBlockHealthTracker(cache.DefaultBlockHealthConfig())
			spaceNeeded := int64(50 * 4096) // Need to evict ~50 blocks

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				strategy.SelectEvictionCandidates(testBlocks, spaceNeeded, healthTracker)
			}
		})
	}
}
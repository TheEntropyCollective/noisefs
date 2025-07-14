package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
)

// BenchmarkEvictionScoring tests the performance of scoring algorithms
func BenchmarkEvictionScoring(b *testing.B) {
	// Create test metadata using new constructor
	testBlocks := make([]*cache.BlockMetadata, 1000)
	for i := 0; i < 1000; i++ {
		blockInfo := &cache.BlockInfo{
			CID:        fmt.Sprintf("test-block-%d", i),
			Size:       4096,
			Popularity: i % 100, // Varying popularity
		}
		testBlocks[i] = cache.NewBlockMetadata(blockInfo, cache.PersonalBlock)
		testBlocks[i].CachedAt = time.Now().Add(-time.Duration(i) * time.Minute)
		testBlocks[i].LastAccessed = time.Now().Add(-time.Duration(i%50) * time.Minute)
	}

	healthTracker := cache.NewBlockHealthTracker(cache.DefaultBlockHealthConfig())

	strategies := map[string]cache.EvictionStrategy{
		"LRU":        &cache.LRUEvictionStrategy{},
		"LFU":        &cache.LFUEvictionStrategy{},
		"ValueBased": cache.NewValueBasedEvictionStrategy(),
		"Adaptive":   cache.NewAdaptiveEvictionStrategy(),
	}

	for name, strategy := range strategies {
		b.Run(name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Score all blocks (simulating eviction candidate selection)
				for _, block := range testBlocks {
					strategy.Score(block, healthTracker)
				}
			}
		})
	}
}

// BenchmarkEvictionSelection tests full eviction candidate selection
func BenchmarkEvictionSelection(b *testing.B) {
	// Create map of test blocks using new constructor
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

	strategies := map[string]cache.EvictionStrategy{
		"LRU":        &cache.LRUEvictionStrategy{},
		"LFU":        &cache.LFUEvictionStrategy{},
		"ValueBased": cache.NewValueBasedEvictionStrategy(),
		"Adaptive":   cache.NewAdaptiveEvictionStrategy(),
	}

	spaceNeeded := int64(50 * 4096) // Need to evict ~50 blocks

	for name, strategy := range strategies {
		b.Run(name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				strategy.SelectEvictionCandidates(testBlocks, spaceNeeded, healthTracker)
			}
		})
	}
}
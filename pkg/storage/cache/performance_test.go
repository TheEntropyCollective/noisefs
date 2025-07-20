package cache

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
)

// BenchmarkGetRandomizers tests the performance improvement of optimized sorting
func BenchmarkGetRandomizers(b *testing.B) {
	// Test different cache sizes to validate O(n²) -> O(n + k*log(n)) improvement
	sizes := []int{100, 500, 1000, 2000, 5000}
	
	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size%d", size), func(b *testing.B) {
			cache := NewMemoryCache(size)
			
			// Populate cache with test blocks
			for i := 0; i < size; i++ {
				cid := fmt.Sprintf("block_%d", i)
				block, err := blocks.NewBlock([]byte(fmt.Sprintf("data_%d", i)))
				if err != nil {
					b.Fatalf("Failed to create block: %v", err)
				}
				cache.Store(cid, block)
				
				// Set random popularity to create varied sorting scenarios
				for j := 0; j < rand.Intn(100); j++ {
					cache.IncrementPopularity(cid)
				}
			}
			
			b.ResetTimer()
			
			// Benchmark the optimized GetRandomizers operation
			for i := 0; i < b.N; i++ {
				_, err := cache.GetRandomizers(50) // Request top 50 blocks
				if err != nil {
					b.Fatalf("GetRandomizers failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkMemoryOptimizations tests memory management improvements
func BenchmarkMemoryOptimizations(b *testing.B) {
	const memoryLimit = 1024 * 1024 // 1MB limit
	
	b.Run("WithMemoryLimit", func(b *testing.B) {
		cache := NewMemoryCacheWithMemoryLimit(10000, memoryLimit)
		
		b.ResetTimer()
		
		for i := 0; i < b.N; i++ {
			cid := fmt.Sprintf("block_%d", i)
			// Create varying size blocks to test memory eviction
			dataSize := 1024 + rand.Intn(4096) // 1-5KB blocks
			block, err := blocks.NewBlock(make([]byte, dataSize))
			if err != nil {
				b.Fatalf("Failed to create block: %v", err)
			}
			
			cache.Store(cid, block)
		}
		
		// Verify memory limit is respected
		current, limit := cache.GetMemoryUsage()
		if current > limit && limit > 0 {
			b.Errorf("Memory usage %d exceeded limit %d", current, limit)
		}
	})
	
	b.Run("WithoutMemoryLimit", func(b *testing.B) {
		cache := NewMemoryCache(10000)
		
		b.ResetTimer()
		
		for i := 0; i < b.N; i++ {
			cid := fmt.Sprintf("block_%d", i)
			dataSize := 1024 + rand.Intn(4096)
			block, err := blocks.NewBlock(make([]byte, dataSize))
			if err != nil {
				b.Fatalf("Failed to create block: %v", err)
			}
			
			cache.Store(cid, block)
		}
	})
}

// TestPerformanceOptimizer validates the optimization algorithms
func TestPerformanceOptimizer(t *testing.T) {
	optimizer := NewPerformanceOptimizer()
	
	// Create test data with known popularity ordering
	blockInfos := make([]*BlockInfo, 100)
	for i := 0; i < 100; i++ {
		block, err := blocks.NewBlock([]byte(fmt.Sprintf("data_%d", i)))
		if err != nil {
			t.Fatalf("Failed to create block: %v", err)
		}
		blockInfos[i] = &BlockInfo{
			CID:        fmt.Sprintf("block_%d", i),
			Block:      block,
			Size:       len(block.Data),
			Popularity: 100 - i, // Descending popularity: 100, 99, 98, ...
		}
	}
	
	// Test top-K selection
	topN := optimizer.GetTopNBlocks(blockInfos, 10)
	
	if len(topN) != 10 {
		t.Errorf("Expected 10 blocks, got %d", len(topN))
	}
	
	// Verify correct ordering (highest popularity first)
	for i := 0; i < len(topN)-1; i++ {
		if topN[i].Popularity < topN[i+1].Popularity {
			t.Errorf("Incorrect ordering: block %d popularity %d < block %d popularity %d",
				i, topN[i].Popularity, i+1, topN[i+1].Popularity)
		}
	}
	
	// Verify top blocks are correct
	if topN[0].Popularity != 100 {
		t.Errorf("Expected highest popularity 100, got %d", topN[0].Popularity)
	}
	if topN[9].Popularity != 91 {
		t.Errorf("Expected 10th highest popularity 91, got %d", topN[9].Popularity)
	}
}

// TestAdaptiveWorkerCount validates worker count calculation
func TestAdaptiveWorkerCount(t *testing.T) {
	workers := AdaptiveWorkerCount()
	
	if workers < 4 {
		t.Errorf("Worker count too low: %d (minimum should be 4)", workers)
	}
	
	if workers > 64 {
		t.Errorf("Worker count too high: %d (maximum should be 64)", workers)
	}
	
	t.Logf("Adaptive worker count: %d", workers)
}

// TestCacheStatistics validates performance metrics calculation
func TestCacheStatistics(t *testing.T) {
	cache := NewMemoryCacheWithMemoryLimit(100, 1024*1024)
	
	// Perform operations to generate statistics
	for i := 0; i < 50; i++ {
		cid := fmt.Sprintf("block_%d", i)
		block, err := blocks.NewBlock(make([]byte, 1024))
		if err != nil {
			t.Fatalf("Failed to create block: %v", err)
		}
		cache.Store(cid, block)
	}
	
	// Generate some hits and misses
	for i := 0; i < 25; i++ {
		cache.Get(fmt.Sprintf("block_%d", i)) // Hits
	}
	for i := 50; i < 75; i++ {
		cache.Get(fmt.Sprintf("block_%d", i)) // Misses
	}
	
	stats := cache.GetPerformanceStats()
	
	if stats.HitRate <= 0 {
		t.Error("Hit rate should be greater than 0")
	}
	
	if stats.MissRate <= 0 {
		t.Error("Miss rate should be greater than 0")
	}
	
	if stats.MemoryUsage <= 0 {
		t.Error("Memory usage should be greater than 0")
	}
	
	if stats.MemoryLimit != 1024*1024 {
		t.Errorf("Expected memory limit 1048576, got %d", stats.MemoryLimit)
	}
	
	t.Logf("Cache performance: Hit=%.1f%%, Miss=%.1f%%, Memory=%d/%d",
		stats.HitRate, stats.MissRate, stats.MemoryUsage, stats.MemoryLimit)
}

// BenchmarkSortingAlgorithms compares performance of different sorting approaches
func BenchmarkSortingAlgorithms(b *testing.B) {
	// Create realistic test data with varied popularity distribution
	createTestBlocks := func(size int) []*BlockInfo {
		blockInfos := make([]*BlockInfo, size)
		for i := 0; i < size; i++ {
			block, err := blocks.NewBlock(make([]byte, 128)) // Typical small block
			if err != nil {
				b.Fatalf("Failed to create block: %v", err)
			}
			blockInfos[i] = &BlockInfo{
				CID:        fmt.Sprintf("block_%d", i),
				Block:      block,
				Size:       128,
				Popularity: rand.Intn(1000), // Random popularity 0-999
			}
		}
		return blockInfos
	}
	
	sizes := []int{100, 500, 1000, 2000}
	optimizer := NewPerformanceOptimizer()
	
	for _, size := range sizes {
		blockInfos := createTestBlocks(size)
		
		b.Run(fmt.Sprintf("OptimizedSort_Size%d", size), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				// Copy blocks to avoid modifying original
				testBlocks := make([]*BlockInfo, len(blockInfos))
				copy(testBlocks, blockInfos)
				
				optimizer.GetTopNBlocks(testBlocks, 50)
			}
		})
	}
}

// TestMemoryEviction validates memory-based eviction policies
func TestMemoryEviction(t *testing.T) {
	// Create cache with small memory limit
	cache := NewMemoryCacheWithMemoryLimit(100, 5*1024) // 5KB limit
	
	// Add blocks until memory limit is exceeded
	for i := 0; i < 10; i++ {
		cid := fmt.Sprintf("block_%d", i)
		block, err := blocks.NewBlock(make([]byte, 1024)) // 1KB each
		if err != nil {
			t.Fatalf("Failed to create block: %v", err)
		}
		
		err = cache.Store(cid, block)
		if err != nil {
			t.Fatalf("Failed to store block %s: %v", cid, err)
		}
	}
	
	// Verify memory limit is respected
	current, limit := cache.GetMemoryUsage()
	if current > limit {
		t.Errorf("Memory usage %d exceeded limit %d", current, limit)
	}
	
	// Verify some blocks were evicted
	if cache.Size() >= 10 {
		t.Errorf("Expected fewer than 10 blocks due to memory eviction, got %d", cache.Size())
	}
	
	t.Logf("Final cache state: %d blocks, %d/%d memory used", 
		cache.Size(), current, limit)
}

func init() {
	// Seed random number generator for consistent benchmarks
	rand.Seed(time.Now().UnixNano())
}
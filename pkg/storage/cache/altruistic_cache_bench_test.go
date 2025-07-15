package cache

import (
	"fmt"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
)

// BenchmarkAltruisticCache_Store benchmarks storing blocks
func BenchmarkAltruisticCache_Store(b *testing.B) {
	sizes := []int{1024, 4096, 16384, 65536} // 1KB, 4KB, 16KB, 64KB
	
	for _, size := range sizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			baseCache := NewMemoryCache(10000)
			config := &AltruisticCacheConfig{
				MinPersonalCache: 50 * 1024 * 1024, // 50MB
				EnableAltruistic: true,
				EvictionCooldown: 5 * time.Minute,
			}
			cache := NewAltruisticCache(baseCache, config, 100*1024*1024) // 100MB
			
			// Pre-generate blocks
			testBlocks := make([]*blocks.Block, b.N)
			for i := 0; i < b.N; i++ {
				data := make([]byte, size)
				testBlocks[i], _ = blocks.NewBlock(data)
			}
			
			b.ResetTimer()
			b.SetBytes(int64(size))
			
			for i := 0; i < b.N; i++ {
				origin := PersonalBlock
				if i%3 == 0 {
					origin = AltruisticBlock
				}
				cache.StoreWithOrigin(fmt.Sprintf("block-%d", i), testBlocks[i], origin)
			}
		})
	}
}

// BenchmarkAltruisticCache_Get benchmarks retrieving blocks
func BenchmarkAltruisticCache_Get(b *testing.B) {
	baseCache := NewMemoryCache(10000)
	config := &AltruisticCacheConfig{
		MinPersonalCache: 50 * 1024 * 1024,
		EnableAltruistic: true,
		EvictionCooldown: 5 * time.Minute,
	}
	cache := NewAltruisticCache(baseCache, config, 100*1024*1024)
	
	// Pre-populate cache
	numBlocks := 1000
	blockSize := 4096
	for i := 0; i < numBlocks; i++ {
		data := make([]byte, blockSize)
		block := &blocks.Block{Data: data}
		origin := PersonalBlock
		if i%3 == 0 {
			origin = AltruisticBlock
		}
		cache.StoreWithOrigin(fmt.Sprintf("block-%d", i), block, origin)
	}
	
	b.ResetTimer()
	b.SetBytes(int64(blockSize))
	
	for i := 0; i < b.N; i++ {
		cid := fmt.Sprintf("block-%d", i%numBlocks)
		cache.Get(cid)
	}
}

// BenchmarkAltruisticCache_Eviction benchmarks eviction performance
func BenchmarkAltruisticCache_Eviction(b *testing.B) {
	for _, numAltruistic := range []int{100, 1000, 5000} {
		b.Run(fmt.Sprintf("altruistic=%d", numAltruistic), func(b *testing.B) {
			baseCache := NewMemoryCache(10000)
			config := &AltruisticCacheConfig{
				MinPersonalCache: 10 * 1024 * 1024, // 10MB
				EnableAltruistic: true,
				EvictionCooldown: 100 * time.Millisecond,
			}
			cache := NewAltruisticCache(baseCache, config, 20*1024*1024) // 20MB
			
			// Fill with altruistic blocks
			blockSize := 4096
			for i := 0; i < numAltruistic; i++ {
				data := make([]byte, blockSize)
				block := &blocks.Block{Data: data}
				cache.StoreWithOrigin(fmt.Sprintf("alt-%d", i), block, AltruisticBlock)
			}
			
			b.ResetTimer()
			
			// Benchmark eviction by adding personal blocks
			for i := 0; i < b.N; i++ {
				data := make([]byte, blockSize)
				block := &blocks.Block{Data: data}
				cache.StoreWithOrigin(fmt.Sprintf("personal-%d", i), block, PersonalBlock)
				
				// Reset after cooldown
				if i%10 == 0 {
					time.Sleep(150 * time.Millisecond)
				}
			}
		})
	}
}

// BenchmarkAltruisticCache_Stats benchmarks stats calculation
func BenchmarkAltruisticCache_Stats(b *testing.B) {
	baseCache := NewMemoryCache(10000)
	config := &AltruisticCacheConfig{
		MinPersonalCache: 50 * 1024 * 1024,
		EnableAltruistic: true,
		EvictionCooldown: 5 * time.Minute,
	}
	cache := NewAltruisticCache(baseCache, config, 100*1024*1024)
	
	// Pre-populate with mixed blocks
	for i := 0; i < 1000; i++ {
		data := make([]byte, 4096)
		block := &blocks.Block{Data: data}
		origin := PersonalBlock
		if i%3 == 0 {
			origin = AltruisticBlock
		}
		cache.StoreWithOrigin(fmt.Sprintf("block-%d", i), block, origin)
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		stats := cache.GetAltruisticStats()
		_ = stats.FlexPoolUsage // Force calculation
	}
}

// BenchmarkAltruisticCache_MemoryOverhead measures memory overhead
func BenchmarkAltruisticCache_MemoryOverhead(b *testing.B) {
	b.Run("AltruisticCache", func(b *testing.B) {
		baseCache := NewMemoryCache(1000)
		config := &AltruisticCacheConfig{
			MinPersonalCache: 50 * 1024 * 1024,
			EnableAltruistic: true,
			EvictionCooldown: 5 * time.Minute,
		}
		_ = NewAltruisticCache(baseCache, config, 100*1024*1024)
		
		// The memory profiler will show the overhead
		b.ReportAllocs()
	})
	
	b.Run("PlainCache", func(b *testing.B) {
		_ = NewMemoryCache(1000)
		b.ReportAllocs()
	})
}

// BenchmarkAltruisticCache_Comparison compares altruistic vs regular cache
func BenchmarkAltruisticCache_Comparison(b *testing.B) {
	blockSize := 4096
	numOps := 10000
	
	b.Run("AltruisticCache", func(b *testing.B) {
		baseCache := NewMemoryCache(10000)
		config := &AltruisticCacheConfig{
			MinPersonalCache: 50 * 1024 * 1024,
			EnableAltruistic: true,
			EvictionCooldown: 5 * time.Minute,
		}
		cache := NewAltruisticCache(baseCache, config, 100*1024*1024)
		
		b.ResetTimer()
		
		for i := 0; i < b.N; i++ {
			for j := 0; j < numOps; j++ {
				data := make([]byte, blockSize)
				block := &blocks.Block{Data: data}
				cid := fmt.Sprintf("block-%d", j)
				
				// Mix of operations
				switch j % 4 {
				case 0:
					cache.StoreWithOrigin(cid, block, PersonalBlock)
				case 1:
					cache.StoreWithOrigin(cid, block, AltruisticBlock)
				case 2:
					cache.Get(cid)
				default:
					cache.GetAltruisticStats()
				}
			}
		}
	})
	
	b.Run("RegularCache", func(b *testing.B) {
		cache := NewMemoryCache(10000)
		
		b.ResetTimer()
		
		for i := 0; i < b.N; i++ {
			for j := 0; j < numOps; j++ {
				data := make([]byte, blockSize)
				block := &blocks.Block{Data: data}
				cid := fmt.Sprintf("block-%d", j)
				
				// Similar operations
				if j%4 <= 1 {
					cache.Store(cid, block)
				} else if j%4 == 2 {
					cache.Get(cid)
				} else {
					cache.GetStats()
				}
			}
		}
	})
}
package cache

import (
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/bits-and-blooms/bloom/v3"
)

// BenchmarkCache_BaselineMemoryCache provides baseline performance
func BenchmarkCache_BaselineMemoryCache(b *testing.B) {
	cache := NewMemoryCache(1000)
	benchmarkCacheOperations(b, cache, "MemoryCache")
}

// BenchmarkCache_AltruisticDisabled tests altruistic cache with feature disabled
func BenchmarkCache_AltruisticDisabled(b *testing.B) {
	baseCache := NewMemoryCache(1000)
	config := &AltruisticCacheConfig{
		MinPersonalCache: 100 * 1024 * 1024,
		EnableAltruistic: false,
	}
	cache := NewAltruisticCache(baseCache, config, 1*1024*1024*1024)
	benchmarkCacheOperations(b, cache, "AltruisticDisabled")
}

// BenchmarkCache_AltruisticEnabled tests altruistic cache with feature enabled
func BenchmarkCache_AltruisticEnabled(b *testing.B) {
	baseCache := NewMemoryCache(1000)
	config := &AltruisticCacheConfig{
		MinPersonalCache: 100 * 1024 * 1024,
		EnableAltruistic: true,
	}
	cache := NewAltruisticCache(baseCache, config, 1*1024*1024*1024)
	benchmarkCacheOperations(b, cache, "AltruisticEnabled")
}

// benchmarkCacheOperations runs standard cache operations
func benchmarkCacheOperations(b *testing.B, cache Cache, name string) {
	blockSize := 128 * 1024 // 128KB testBlocks

	// Prepare test data
	testBlocks := make([]*blocks.Block, 100)
	cids := make([]string, 100)
	for i := 0; i < 100; i++ {
		data := make([]byte, blockSize)
		rand.Read(data)
		testBlocks[i], _ = blocks.NewBlock(data)
		cids[i] = fmt.Sprintf("%s-block-%d", name, i)
	}

	b.Run(name+"/Store", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			idx := i % len(testBlocks)
			cache.Store(cids[idx], testBlocks[idx])
		}
	})

	b.Run(name+"/Get", func(b *testing.B) {
		// Pre-populate cache
		for i := 0; i < len(testBlocks); i++ {
			cache.Store(cids[i], testBlocks[i])
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			idx := i % len(cids)
			cache.Get(cids[idx])
		}
	})

	b.Run(name+"/Mixed", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			idx := i % len(testBlocks)
			if i%3 == 0 {
				cache.Store(cids[idx], testBlocks[idx])
			} else {
				cache.Get(cids[idx])
			}
		}
	})
}

// BenchmarkEvictionStrategies compares different eviction strategies
func BenchmarkEvictionStrategies(b *testing.B) {
	strategies := []string{"LRU", "LFU", "ValueBased", "Adaptive"}

	for _, strategy := range strategies {
		b.Run(strategy, func(b *testing.B) {
			baseCache := NewMemoryCache(100) // Small cache to force evictions
			config := &AltruisticCacheConfig{
				MinPersonalCache: 10 * 1024 * 1024,
				EnableAltruistic: true,
				EvictionStrategy: strategy,
			}
			cache := NewAltruisticCache(baseCache, config, 50*1024*1024)

			// Create health tracker for value-based strategy
			if strategy == "ValueBased" {
				healthTracker := NewBlockHealthTracker(nil)
				cache.healthTracker = healthTracker
			}

			blockSize := 512 * 1024 // 512KB testBlocks

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				data := make([]byte, blockSize)
				block, _ := blocks.NewBlock(data)

				// Mix of personal and altruistic testBlocks
				origin := AltruisticBlock
				if i%3 == 0 {
					origin = PersonalBlock
				}

				cid := fmt.Sprintf("evict-test-%d", i)
				cache.StoreWithOrigin(cid, block, origin)

				// Update health for value-based strategy
				if strategy == "ValueBased" && origin == AltruisticBlock {
					hint := BlockHint{
						ReplicationBucket: ReplicationBucket(i % 3),
						NoisyRequestRate:  float64(i % 100),
					}
					cache.UpdateBlockHealth(cid, hint)
				}
			}
		})
	}
}

// BenchmarkConcurrentAccess tests performance under concurrent load
func BenchmarkConcurrentAccess(b *testing.B) {
	configs := []struct {
		name    string
		workers int
	}{
		{"Serial", 1},
		{"Concurrent2", 2},
		{"Concurrent4", 4},
		{"Concurrent8", 8},
		{"Concurrent16", 16},
	}

	for _, cfg := range configs {
		b.Run(cfg.name, func(b *testing.B) {
			baseCache := NewMemoryCache(1000)
			config := &AltruisticCacheConfig{
				MinPersonalCache: 100 * 1024 * 1024,
				EnableAltruistic: true,
			}
			cache := NewAltruisticCache(baseCache, config, 1*1024*1024*1024)

			blockSize := 64 * 1024 // 64KB testBlocks

			b.ResetTimer()

			var wg sync.WaitGroup
			workPerWorker := b.N / cfg.workers

			start := time.Now()
			for w := 0; w < cfg.workers; w++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()

					for i := 0; i < workPerWorker; i++ {
						data := make([]byte, blockSize)
						block, _ := blocks.NewBlock(data)

						cid := fmt.Sprintf("worker%d-block%d", workerID, i)

						if i%2 == 0 {
							cache.StoreWithOrigin(cid, block, PersonalBlock)
						} else {
							cache.Get(cid)
						}
					}
				}(w)
			}

			wg.Wait()
			elapsed := time.Since(start)

			opsPerSec := float64(b.N) / elapsed.Seconds()
			b.ReportMetric(opsPerSec, "ops/sec")
		})
	}
}

// BenchmarkMemoryOverhead measures memory overhead of altruistic caching
func BenchmarkMemoryOverhead(b *testing.B) {
	measureMemory := func(name string, setupCache func() Cache) {
		b.Run(name, func(b *testing.B) {
			runtime.GC()
			var m1 runtime.MemStats
			runtime.ReadMemStats(&m1)

			cache := setupCache()
			blockSize := 128 * 1024 // 128KB testBlocks

			// Store many testBlocks
			for i := 0; i < 1000; i++ {
				data := make([]byte, blockSize)
				block, _ := blocks.NewBlock(data)
				cache.Store(fmt.Sprintf("block-%d", i), block)
			}

			runtime.GC()
			var m2 runtime.MemStats
			runtime.ReadMemStats(&m2)

			memUsed := m2.Alloc - m1.Alloc
			memPerBlock := memUsed / 1000

			b.ReportMetric(float64(memPerBlock), "bytes/block")
			b.ReportMetric(float64(memUsed/(1024*1024)), "MB_total")
		})
	}

	measureMemory("BaseMemoryCache", func() Cache {
		return NewMemoryCache(10000)
	})

	measureMemory("AltruisticCache", func() Cache {
		baseCache := NewMemoryCache(10000)
		config := &AltruisticCacheConfig{
			MinPersonalCache: 100 * 1024 * 1024,
			EnableAltruistic: true,
		}
		return NewAltruisticCache(baseCache, config, 1*1024*1024*1024)
	})
}

// BenchmarkNetworkHealth benchmarks network health operations
func BenchmarkNetworkHealth(b *testing.B) {
	b.Run("GossipMessageCreation", func(b *testing.B) {
		config := &HealthGossipConfig{
			EnableDifferentialPrivacy: true,
			PrivacyEpsilon:            1.0,
		}
		healthTracker := NewBlockHealthTracker(nil)
		gossiper, _ := NewHealthGossiper(config, healthTracker, nil)

		// Add test blocks
		for i := 0; i < 100; i++ {
			healthTracker.UpdateBlockHealth(fmt.Sprintf("block-%d", i), BlockHint{
				ReplicationBucket: ReplicationBucket(i % 3),
				NoisyRequestRate:  float64(i),
			})
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			hints := healthTracker.GetAllBlockHints()
			gossiper.calculateAggregateStats(
				hints,
				bloom.NewWithEstimates(10000, 0.01),
				bloom.NewWithEstimates(10000, 0.01),
			)
		}
	})

	b.Run("BloomFilterExchange", func(b *testing.B) {
		baseCache := NewMemoryCache(1000)
		config := &AltruisticCacheConfig{
			MinPersonalCache: 100 * 1024 * 1024,
			EnableAltruistic: true,
		}
		cache := NewAltruisticCache(baseCache, config, 1*1024*1024*1024)

		exchangeConfig := DefaultBloomExchangeConfig()
		exchanger, _ := NewBloomExchanger(exchangeConfig, cache, nil)

		// Add blocks to cache
		for i := 0; i < 100; i++ {
			data := make([]byte, 1024)
			block := &blocks.Block{Data: data}
			cache.StoreWithOrigin(fmt.Sprintf("block-%d", i), block, AltruisticBlock)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			exchanger.UpdateLocalFilters()
		}
	})

	b.Run("CoordinationHints", func(b *testing.B) {
		config := DefaultBloomExchangeConfig()
		engine := NewCoordinationEngine(config)

		// Create test data
		localFilters := map[string]*bloom.BloomFilter{
			"valuable_blocks": bloom.NewWithEstimates(10000, 0.01),
		}

		peerFilters := make(map[string]*PeerFilterSet)
		for i := 0; i < 10; i++ {
			peerFilters[fmt.Sprintf("peer%d", i)] = &PeerFilterSet{
				PeerID: fmt.Sprintf("peer%d", i),
				Filters: map[string]*bloom.BloomFilter{
					"valuable_blocks": bloom.NewWithEstimates(10000, 0.01),
				},
				Hints: &CoordinationHints{
					HighDemandBlocks: []string{"block1", "block2", "block3"},
				},
			}
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			engine.GenerateHints(localFilters, peerFilters)
		}
	})
}

// BenchmarkSpaceManagement benchmarks space management operations
func BenchmarkSpaceManagement(b *testing.B) {
	b.Run("FlexPoolCalculation", func(b *testing.B) {
		baseCache := NewMemoryCache(1000)
		config := &AltruisticCacheConfig{
			MinPersonalCache: 100 * 1024 * 1024,
			EnableAltruistic: true,
		}
		cache := NewAltruisticCache(baseCache, config, 1*1024*1024*1024)

		// Add some blocks
		for i := 0; i < 100; i++ {
			data := make([]byte, 128*1024)
			block := &blocks.Block{Data: data}
			cache.StoreWithOrigin(fmt.Sprintf("block-%d", i), block, PersonalBlock)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cache.getFlexPoolUsage()
		}
	})

	b.Run("EvictionDecision", func(b *testing.B) {
		baseCache := NewMemoryCache(100) // Small to force evictions
		config := &AltruisticCacheConfig{
			MinPersonalCache: 10 * 1024 * 1024,
			EnableAltruistic: true,
		}
		cache := NewAltruisticCache(baseCache, config, 50*1024*1024)

		// Fill cache
		for i := 0; i < 100; i++ {
			data := make([]byte, 512*1024)
			block := &blocks.Block{Data: data}
			cache.StoreWithOrigin(fmt.Sprintf("altruistic-%d", i), block, AltruisticBlock)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Simulate eviction decision
			cache.canAcceptAltruistic(512 * 1024)
		}
	})
}

// BenchmarkScalability tests performance at different scales
func BenchmarkScalability(b *testing.B) {
	scales := []struct {
		name      string
		numBlocks int
		blockSize int
	}{
		{"Small_100x64KB", 100, 64 * 1024},
		{"Medium_1000x128KB", 1000, 128 * 1024},
		{"Large_10000x256KB", 10000, 256 * 1024},
	}

	for _, scale := range scales {
		b.Run(scale.name, func(b *testing.B) {
			baseCache := NewMemoryCache(scale.numBlocks * 2)
			config := &AltruisticCacheConfig{
				MinPersonalCache: int64(scale.numBlocks * scale.blockSize / 4),
				EnableAltruistic: true,
			}
			totalCapacity := int64(scale.numBlocks * scale.blockSize)
			cache := NewAltruisticCache(baseCache, config, totalCapacity)

			// Pre-populate cache
			for i := 0; i < scale.numBlocks/2; i++ {
				data := make([]byte, scale.blockSize)
				block, _ := blocks.NewBlock(data)
				cache.StoreWithOrigin(fmt.Sprintf("existing-%d", i), block, AltruisticBlock)
			}

			b.ResetTimer()

			// Benchmark mixed operations
			for i := 0; i < b.N; i++ {
				op := i % 4
				idx := i % scale.numBlocks

				switch op {
				case 0, 1: // 50% stores
					data := make([]byte, scale.blockSize)
					block, _ := blocks.NewBlock(data)
					origin := PersonalBlock
					if i%3 != 0 {
						origin = AltruisticBlock
					}
					cache.StoreWithOrigin(fmt.Sprintf("new-%d", idx), block, origin)
				case 2: // 25% gets
					cache.Get(fmt.Sprintf("existing-%d", idx/2))
				case 3: // 25% stats
					cache.GetAltruisticStats()
				}
			}

			// Report final stats
			stats := cache.GetAltruisticStats()
			b.ReportMetric(float64(stats.PersonalBlocks+stats.AltruisticBlocks), "total_blocks")
			b.ReportMetric(stats.FlexPoolUsage*100, "flex_usage_%")
		})
	}
}

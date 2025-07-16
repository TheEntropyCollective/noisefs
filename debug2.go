package main

import (
	"fmt"
	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
)

func main() {
	totalCapacity := int64(20 * 1024 * 1024) // 20MB
	minPersonal := int64(5 * 1024 * 1024)    // 5MB
	
	baseCache := cache.NewMemoryCache(200)
	config := &cache.AltruisticCacheConfig{
		MinPersonalCache: minPersonal,
		EnableAltruistic: true,
		EvictionStrategy: "ValueBased",
	}
	
	altruisticCache := cache.NewAltruisticCache(baseCache, config, totalCapacity)
	
	blockSize := 256 * 1024 // 256KB blocks
	
	// Add just a few blocks to test scoring
	for i := 0; i < 8; i++ {
		data := make([]byte, blockSize)
		block := &blocks.Block{Data: data}
		
		cid := fmt.Sprintf("network-block-%d", i)
		err := altruisticCache.StoreWithOrigin(cid, block, cache.AltruisticBlock)
		if err != nil {
			fmt.Printf("Error storing block %d: %v\n", i, err)
		}
		
		// Simulate network health hints
		var hint cache.BlockHint
		switch i % 4 {
		case 0: // Very valuable
			hint = cache.BlockHint{
				ReplicationBucket: cache.ReplicationLow,
				HighEntropy:       true,
				NoisyRequestRate:  100,
				MissingRegions:    5,
			}
		case 1: // Somewhat valuable
			hint = cache.BlockHint{
				ReplicationBucket: cache.ReplicationMedium,
				HighEntropy:       true,
				NoisyRequestRate:  50,
			}
		case 2: // Low value
			hint = cache.BlockHint{
				ReplicationBucket: cache.ReplicationHigh,
				HighEntropy:       false,
				NoisyRequestRate:  10,
			}
		default: // Very low value
			hint = cache.BlockHint{
				ReplicationBucket: cache.ReplicationHigh,
				HighEntropy:       false,
				NoisyRequestRate:  1,
			}
		}
		
		altruisticCache.UpdateBlockHealth(cid, hint)
	}
	
	// Get the health tracker and eviction strategy
	healthTracker := altruisticCache.GetHealthTracker()
	evictionStrategy := &cache.ValueBasedEvictionStrategy{
		AgeWeight:       0.3,
		FrequencyWeight: 0.3,
		HealthWeight:    0.4,
		RandomizerWeight: 0.1,
	}
	
	// Check scores for all blocks
	fmt.Println("Block scores:")
	for i := 0; i < 8; i++ {
		cid := fmt.Sprintf("network-block-%d", i)
		hint := healthTracker.GetBlockHint(cid)
		value := healthTracker.CalculateBlockValue(cid, hint)
		
		// Get the block metadata from the altruistic cache
		stats := altruisticCache.GetAltruisticStats()
		if stats.AltruisticBlocks > 0 {
			// We need to access the internal block metadata somehow
			// This is tricky since it's not exposed publicly
			fmt.Printf("Block %d (case %d): value=%.2f, replication=%d, entropy=%v, rate=%d\n", 
				i, i%4, value, hint.ReplicationBucket, hint.HighEntropy, hint.NoisyRequestRate)
		}
	}
}
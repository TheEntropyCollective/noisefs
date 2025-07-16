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
	
	// Add blocks with varying health scores
	for i := 0; i < 40; i++ {
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
	
	// Check initial state
	stats := altruisticCache.GetAltruisticStats()
	fmt.Printf("Initial state: Personal=%d, Altruistic=%d, Total=%d\n", 
		stats.PersonalSize, stats.AltruisticSize, stats.PersonalSize+stats.AltruisticSize)
	
	// Calculate health values
	healthTracker := altruisticCache.GetHealthTracker()
	for i := 0; i < 4; i++ {
		cid := fmt.Sprintf("network-block-%d", i)
		hint := healthTracker.GetBlockHint(cid)
		value := healthTracker.CalculateBlockValue(cid, hint)
		fmt.Printf("Block %d (case %d): value=%.2f, replication=%d, entropy=%v, rate=%d\n", 
			i, i%4, value, hint.ReplicationBucket, hint.HighEntropy, hint.NoisyRequestRate)
	}
	
	// Add personal blocks to trigger eviction
	personalCount := 0
	for i := 0; i < 50; i++ {
		data := make([]byte, blockSize)
		block := &blocks.Block{Data: data}
		
		err := altruisticCache.StoreWithOrigin(fmt.Sprintf("personal-%d", i), block, cache.PersonalBlock)
		if err != nil {
			fmt.Printf("Error storing personal block %d: %v\n", i, err)
			break
		}
		personalCount++
	}
	
	fmt.Printf("Added %d personal blocks\n", personalCount)
	
	// Check final state
	stats = altruisticCache.GetAltruisticStats()
	fmt.Printf("Final state: Personal=%d, Altruistic=%d, Total=%d\n", 
		stats.PersonalSize, stats.AltruisticSize, stats.PersonalSize+stats.AltruisticSize)
	
	// Check which blocks survived
	valuableCount := 0
	lowValueCount := 0
	
	for i := 0; i < 40; i++ {
		cid := fmt.Sprintf("network-block-%d", i)
		if altruisticCache.Has(cid) {
			switch i % 4 {
			case 0:
				valuableCount++
				fmt.Printf("Valuable block %d survived\n", i)
			case 2, 3:
				lowValueCount++
				fmt.Printf("Low-value block %d survived\n", i)
			}
		}
	}
	
	fmt.Printf("Surviving blocks: %d valuable, %d low-value\n", valuableCount, lowValueCount)
}
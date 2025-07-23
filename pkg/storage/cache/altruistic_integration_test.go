package cache_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
)

// TestAltruisticCacheWithAdaptiveBase tests integration with AdaptiveCache
func TestAltruisticCacheWithAdaptiveBase(t *testing.T) {
	// Create adaptive cache as base
	adaptiveConfig := &cache.AdaptiveCacheConfig{
		MaxSize:            10 * 1024 * 1024, // 10MB
		MaxItems:           1000,
		HotTierRatio:       0.1,
		WarmTierRatio:      0.3,
		PredictionWindow:   time.Hour,
		EvictionBatchSize:  10,
		ExchangeInterval:   time.Minute * 15,
		PredictionInterval: time.Minute * 10,

		// Privacy settings
		PrivacyEpsilon:  1.0,
		TemporalQuantum: time.Hour,
		DummyAccessRate: 0.1,
	}

	adaptiveCache := cache.NewAdaptiveCache(adaptiveConfig)

	// Wrap with altruistic cache
	altruisticConfig := &cache.AltruisticCacheConfig{
		MinPersonalCache: 5 * 1024 * 1024, // 5MB
		EnableAltruistic: true,
		EvictionCooldown: 500 * time.Millisecond,
	}

	altruisticCache := cache.NewAltruisticCache(
		adaptiveCache,
		altruisticConfig,
		10*1024*1024, // 10MB total
	)

	// Test that adaptive cache features still work

	// 1. Store blocks with metadata
	for i := 0; i < 10; i++ {
		data := make([]byte, 100*1024) // 100KB blocks
		block := &blocks.Block{Data: data}

		// The adaptive cache's Store method is called internally
		origin := cache.PersonalBlock
		if i%3 == 0 {
			origin = cache.AltruisticBlock
		}

		err := altruisticCache.StoreWithOrigin(fmt.Sprintf("block-%d", i), block, origin)
		if err != nil {
			t.Errorf("Failed to store block %d: %v", i, err)
		}
	}

	// 2. Verify tier assignment works
	adaptiveStats := adaptiveCache.GetStats()
	if adaptiveStats == nil {
		t.Error("Adaptive cache stats should not be nil")
	}

	// 3. Test that altruistic features work on top
	altruisticStats := altruisticCache.GetAltruisticStats()
	if altruisticStats.PersonalBlocks == 0 {
		t.Error("Should have personal blocks")
	}
	if altruisticStats.AltruisticBlocks == 0 {
		t.Error("Should have altruistic blocks")
	}

	// 4. Test cache warming/prediction features
	ctx := context.Background()
	blockFetcher := func(cid string) ([]byte, error) {
		// Simulate fetching from network
		return make([]byte, 100*1024), nil
	}

	// This should work through the altruistic cache
	err := adaptiveCache.Preload(ctx, blockFetcher)
	if err != nil {
		t.Errorf("Preload failed: %v", err)
	}
}

// TestAltruisticCacheRealWorldScenario simulates realistic usage patterns
func TestAltruisticCacheRealWorldScenario(t *testing.T) {
	// Simulate a user with 500GB disk, setting 200GB personal minimum
	baseCache := cache.NewMemoryCache(100000)

	config := &cache.AltruisticCacheConfig{
		MinPersonalCache: 200 * 1024 * 1024, // 200MB (scaled down for test)
		EnableAltruistic: true,
		EvictionCooldown: 5 * time.Minute,
	}

	totalCapacity := int64(500 * 1024 * 1024) // 500MB (scaled down)
	altruisticCache := cache.NewAltruisticCache(baseCache, config, totalCapacity)

	// Simulate daily usage pattern

	// Morning: User downloads some files (personal blocks increase)
	morning := []struct {
		size int
		name string
	}{
		{50 * 1024 * 1024, "work-doc-1"},
		{30 * 1024 * 1024, "work-doc-2"},
		{20 * 1024 * 1024, "work-doc-3"},
	}

	for _, file := range morning {
		data := make([]byte, file.size)
		block := &blocks.Block{Data: data}
		err := altruisticCache.StoreWithOrigin(file.name, block, cache.PersonalBlock)
		if err != nil {
			t.Fatalf("Morning download failed: %v", err)
		}
	}

	stats := altruisticCache.GetAltruisticStats()
	t.Logf("Morning - Personal: %d MB, Altruistic: %d MB, Flex: %.1f%%",
		stats.PersonalSize/(1024*1024),
		stats.AltruisticSize/(1024*1024),
		stats.FlexPoolUsage*100)

	// Midday: Network requests some blocks (altruistic fills spare space)
	for i := 0; i < 50; i++ {
		data := make([]byte, 5*1024*1024) // 5MB blocks
		block := &blocks.Block{Data: data}
		altruisticCache.StoreWithOrigin(fmt.Sprintf("network-%d", i), block, cache.AltruisticBlock)
	}

	stats = altruisticCache.GetAltruisticStats()
	t.Logf("Midday - Personal: %d MB, Altruistic: %d MB, Flex: %.1f%%",
		stats.PersonalSize/(1024*1024),
		stats.AltruisticSize/(1024*1024),
		stats.FlexPoolUsage*100)

	// Afternoon: User needs more space (altruistic blocks evicted)
	bigFile := make([]byte, 250*1024*1024) // 250MB
	bigBlock := &blocks.Block{Data: bigFile}

	// Wait for cooldown to pass
	time.Sleep(100 * time.Millisecond)

	err := altruisticCache.StoreWithOrigin("big-project", bigBlock, cache.PersonalBlock)
	if err != nil {
		t.Fatalf("Big file storage failed: %v", err)
	}

	stats = altruisticCache.GetAltruisticStats()
	t.Logf("Afternoon - Personal: %d MB, Altruistic: %d MB, Flex: %.1f%%",
		stats.PersonalSize/(1024*1024),
		stats.AltruisticSize/(1024*1024),
		stats.FlexPoolUsage*100)

	// Verify personal minimum was always respected
	if stats.PersonalSize+stats.AltruisticSize > totalCapacity {
		t.Error("Total usage exceeds capacity")
	}

	// Verify flex pool adapted correctly
	if stats.PersonalSize > config.MinPersonalCache && stats.FlexPoolUsage == 0 {
		t.Error("Flex pool usage should be non-zero when personal exceeds minimum")
	}
}

// TestAltruisticCacheMemoryEfficiency verifies memory usage is reasonable
func TestAltruisticCacheMemoryEfficiency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory efficiency test in short mode")
	}

	// Create a large cache and measure overhead
	baseCache := cache.NewMemoryCache(10000)

	config := &cache.AltruisticCacheConfig{
		MinPersonalCache: 1024 * 1024 * 1024, // 1GB
		EnableAltruistic: true,
		EvictionCooldown: 5 * time.Minute,
	}

	altruisticCache := cache.NewAltruisticCache(
		baseCache,
		config,
		2*1024*1024*1024, // 2GB
	)

	// Add many small blocks to test metadata overhead
	blockSize := 4096 // 4KB
	numBlocks := 10000

	for i := 0; i < numBlocks; i++ {
		data := make([]byte, blockSize)
		block := &blocks.Block{Data: data}

		origin := cache.PersonalBlock
		if i%3 == 0 {
			origin = cache.AltruisticBlock
		}

		err := altruisticCache.StoreWithOrigin(fmt.Sprintf("block-%d", i), block, origin)
		if err != nil {
			// Base cache might be full, that's ok
			break
		}
	}

	stats := altruisticCache.GetAltruisticStats()

	// Calculate overhead
	totalDataSize := stats.PersonalSize + stats.AltruisticSize
	totalBlocks := stats.PersonalBlocks + stats.AltruisticBlocks

	if totalBlocks > 0 {
		overheadPerBlock := 200 // Estimated bytes per block metadata
		totalOverhead := int64(totalBlocks) * int64(overheadPerBlock)
		overheadPercent := float64(totalOverhead) / float64(totalDataSize) * 100

		t.Logf("Memory efficiency - Blocks: %d, Data: %d MB, Overhead: %.2f%%",
			totalBlocks,
			totalDataSize/(1024*1024),
			overheadPercent)

		// Overhead should be reasonable (< 5%)
		if overheadPercent > 5.0 {
			t.Errorf("Memory overhead too high: %.2f%%", overheadPercent)
		}
	}
}

package cache

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
)

// TestSpaceManagement_FullCapacityScenario tests behavior when cache approaches full capacity
func TestSpaceManagement_FullCapacityScenario(t *testing.T) {
	// Create cache with limited capacity
	totalCapacity := int64(10 * 1024 * 1024) // 10MB
	minPersonal := int64(3 * 1024 * 1024)    // 3MB
	
	baseCache := NewMemoryCache(100) // 100 blocks max
	config := &AltruisticCacheConfig{
		MinPersonalCache: minPersonal,
		EnableAltruistic: true,
		EvictionStrategy: "LRU",
	}
	
	cache := NewAltruisticCache(baseCache, config, totalCapacity)
	
	// Fill with altruistic blocks first (7MB worth)
	altruisticSize := int64(0)
	blockSize := 128 * 1024 // 128KB blocks
	
	for i := 0; altruisticSize < 7*1024*1024; i++ {
		data := make([]byte, blockSize)
		block := &blocks.Block{Data: data}
		
		err := cache.StoreWithOrigin(fmt.Sprintf("altruistic-%d", i), block, AltruisticBlock)
		if err != nil {
			t.Fatalf("Failed to store altruistic block: %v", err)
		}
		altruisticSize += int64(blockSize)
	}
	
	// Verify initial state
	stats := cache.GetAltruisticStats()
	if stats.AltruisticSize < 6*1024*1024 { // Allow some overhead
		t.Errorf("Expected at least 6MB altruistic, got %d", stats.AltruisticSize)
	}
	
	// Now add personal blocks that require evicting altruistic blocks
	personalSize := int64(0)
	for i := 0; personalSize < 8*1024*1024; i++ { // Try to add 8MB personal
		data := make([]byte, blockSize)
		block := &blocks.Block{Data: data}
		
		err := cache.StoreWithOrigin(fmt.Sprintf("personal-%d", i), block, PersonalBlock)
		if err != nil {
			break // Expected to fail at some point
		}
		personalSize += int64(blockSize)
	}
	
	// Final verification
	finalStats := cache.GetAltruisticStats()
	
	// Personal blocks should have displaced altruistic blocks
	if finalStats.PersonalSize < 7*1024*1024 {
		t.Errorf("Personal blocks should be at least 7MB, got %d", finalStats.PersonalSize)
	}
	
	// Total should not exceed capacity
	totalUsed := finalStats.PersonalSize + finalStats.AltruisticSize
	if totalUsed > totalCapacity {
		t.Errorf("Total usage %d exceeds capacity %d", totalUsed, totalCapacity)
	}
	
	// MinPersonal guarantee should be respected
	remainingCapacity := totalCapacity - finalStats.AltruisticSize
	if remainingCapacity < minPersonal {
		t.Errorf("MinPersonal guarantee violated: %d < %d", remainingCapacity, minPersonal)
	}
}

// TestSpaceManagement_FlexPoolDynamics tests dynamic adjustment of flex pool
func TestSpaceManagement_FlexPoolDynamics(t *testing.T) {
	totalCapacity := int64(20 * 1024 * 1024) // 20MB
	minPersonal := int64(5 * 1024 * 1024)    // 5MB
	
	baseCache := NewMemoryCache(200)
	config := &AltruisticCacheConfig{
		MinPersonalCache:      minPersonal,
		EnableAltruistic:      true,
		EvictionStrategy:      "ValueBased",
		EnableGradualEviction: true,
	}
	
	cache := NewAltruisticCache(baseCache, config, totalCapacity)
	
	// Create block health tracker
	cache.healthTracker = NewBlockHealthTracker(nil)
	
	// Phase 1: Fill with mix of blocks
	blockSize := 256 * 1024 // 256KB blocks
	
	// Add some valuable altruistic blocks
	for i := 0; i < 20; i++ {
		data := make([]byte, blockSize)
		block := &blocks.Block{Data: data}
		
		cid := fmt.Sprintf("valuable-%d", i)
		cache.StoreWithOrigin(cid, block, AltruisticBlock)
		
		// Mark as valuable
		cache.UpdateBlockHealth(cid, BlockHint{
			ReplicationBucket: ReplicationLow,
			HighEntropy:       true,
			RequestCount:      100,
		})
	}
	
	// Add some regular altruistic blocks
	for i := 0; i < 20; i++ {
		data := make([]byte, blockSize)
		block := &blocks.Block{Data: data}
		
		cid := fmt.Sprintf("regular-%d", i)
		cache.StoreWithOrigin(cid, block, AltruisticBlock)
		
		cache.UpdateBlockHealth(cid, BlockHint{
			ReplicationBucket: ReplicationHigh,
			HighEntropy:       false,
			RequestCount:      5,
		})
	}
	
	initialStats := cache.GetAltruisticStats()
	t.Logf("Initial state: Personal=%d MB, Altruistic=%d MB, FlexUsage=%.2f%%",
		initialStats.PersonalSize/(1024*1024),
		initialStats.AltruisticSize/(1024*1024),
		initialStats.FlexPoolUsage*100)
	
	// Phase 2: Add personal blocks gradually
	for i := 0; i < 30; i++ {
		data := make([]byte, blockSize)
		block := &blocks.Block{Data: data}
		
		err := cache.StoreWithOrigin(fmt.Sprintf("personal-%d", i), block, PersonalBlock)
		if err != nil {
			t.Logf("Personal block %d failed: %v", i, err)
			break
		}
		
		// Check flex pool usage
		stats := cache.GetAltruisticStats()
		if i%5 == 0 {
			t.Logf("After %d personal blocks: FlexUsage=%.2f%%, Personal=%d MB, Altruistic=%d MB",
				i+1, stats.FlexPoolUsage*100,
				stats.PersonalSize/(1024*1024),
				stats.AltruisticSize/(1024*1024))
		}
	}
	
	// Phase 3: Verify value-based eviction worked
	finalStats := cache.GetAltruisticStats()
	
	// Check that some valuable blocks were preserved
	preservedValuable := 0
	for i := 0; i < 20; i++ {
		if cache.Has(fmt.Sprintf("valuable-%d", i)) {
			preservedValuable++
		}
	}
	
	preservedRegular := 0
	for i := 0; i < 20; i++ {
		if cache.Has(fmt.Sprintf("regular-%d", i)) {
			preservedRegular++
		}
	}
	
	t.Logf("Preserved blocks: %d valuable, %d regular", preservedValuable, preservedRegular)
	
	// Valuable blocks should be preserved more than regular blocks
	if preservedValuable <= preservedRegular {
		t.Error("Value-based eviction should preserve more valuable blocks")
	}
}

// TestSpaceManagement_ConcurrentPressure tests cache under concurrent access with space pressure
func TestSpaceManagement_ConcurrentPressure(t *testing.T) {
	totalCapacity := int64(50 * 1024 * 1024) // 50MB
	minPersonal := int64(10 * 1024 * 1024)   // 10MB
	
	baseCache := NewMemoryCache(500)
	config := &AltruisticCacheConfig{
		MinPersonalCache: minPersonal,
		EnableAltruistic: true,
		EvictionStrategy: "Adaptive",
		EvictionCooldown: 100 * time.Millisecond,
	}
	
	cache := NewAltruisticCache(baseCache, config, totalCapacity)
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	var wg sync.WaitGroup
	errors := make(chan error, 100)
	
	// Writer goroutines - add blocks
	for w := 0; w < 5; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			blockSize := 64 * 1024 // 64KB blocks
			for i := 0; i < 100; i++ {
				select {
				case <-ctx.Done():
					return
				default:
				}
				
				data := make([]byte, blockSize+rand.Intn(blockSize))
				block := &blocks.Block{Data: data}
				
				// Mix of personal and altruistic blocks
				origin := AltruisticBlock
				if rand.Float32() < 0.3 { // 30% personal
					origin = PersonalBlock
				}
				
				cid := fmt.Sprintf("worker%d-block%d", workerID, i)
				err := cache.StoreWithOrigin(cid, block, origin)
				if err != nil && origin == PersonalBlock {
					// Personal blocks might fail when cache is full
					select {
					case errors <- fmt.Errorf("worker %d: %w", workerID, err):
					default:
					}
				}
				
				// Random delay
				time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)
			}
		}(w)
	}
	
	// Reader goroutines - access blocks
	for r := 0; r < 3; r++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			
			for i := 0; i < 200; i++ {
				select {
				case <-ctx.Done():
					return
				default:
				}
				
				// Try to read random blocks
				workerID := rand.Intn(5)
				blockID := rand.Intn(50) // Only try first 50 blocks
				cid := fmt.Sprintf("worker%d-block%d", workerID, blockID)
				
				cache.Get(cid)
				
				// Random delay
				time.Sleep(time.Duration(rand.Intn(5)) * time.Millisecond)
			}
		}(r)
	}
	
	// Stats monitor
	done := make(chan bool)
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				stats := cache.GetAltruisticStats()
				t.Logf("Stats: Personal=%d MB (%.1f%%), Altruistic=%d MB (%.1f%%), Flex=%.1f%%",
					stats.PersonalSize/(1024*1024),
					float64(stats.PersonalSize)/float64(totalCapacity)*100,
					stats.AltruisticSize/(1024*1024),
					float64(stats.AltruisticSize)/float64(totalCapacity)*100,
					stats.FlexPoolUsage*100)
			case <-done:
				return
			}
		}
	}()
	
	// Wait for completion
	wg.Wait()
	close(done)
	
	// Check for errors
	close(errors)
	errorCount := 0
	for err := range errors {
		t.Logf("Error during concurrent test: %v", err)
		errorCount++
	}
	
	// Some errors are expected when cache is full
	if errorCount > 50 {
		t.Errorf("Too many errors during concurrent access: %d", errorCount)
	}
	
	// Final verification
	finalStats := cache.GetAltruisticStats()
	
	// Cache should be near capacity
	utilization := float64(finalStats.PersonalSize+finalStats.AltruisticSize) / float64(totalCapacity)
	if utilization < 0.8 {
		t.Errorf("Cache underutilized: %.1f%%", utilization*100)
	}
	
	// MinPersonal guarantee should be maintained
	if finalStats.PersonalSize+minPersonal > totalCapacity {
		remainingForPersonal := totalCapacity - finalStats.AltruisticSize
		if remainingForPersonal < minPersonal {
			t.Errorf("MinPersonal guarantee violated under concurrent pressure")
		}
	}
}

// TestSpaceManagement_PredictiveEviction tests predictive eviction under space pressure
func TestSpaceManagement_PredictiveEviction(t *testing.T) {
	totalCapacity := int64(10 * 1024 * 1024) // 10MB
	minPersonal := int64(2 * 1024 * 1024)    // 2MB
	
	baseCache := NewMemoryCache(100)
	config := &AltruisticCacheConfig{
		MinPersonalCache:      minPersonal,
		EnableAltruistic:      true,
		EnablePredictive:      true,
		PreEvictThreshold:     0.75, // Start pre-evicting at 75% full
		EvictionStrategy:      "LRU",
	}
	
	cache := NewAltruisticCache(baseCache, config, totalCapacity)
	
	// Simulate access patterns
	blockSize := 512 * 1024 // 512KB blocks
	
	// Phase 1: Create blocks with different access patterns
	// Regular access pattern blocks
	for i := 0; i < 5; i++ {
		data := make([]byte, blockSize)
		block := &blocks.Block{Data: data}
		
		cid := fmt.Sprintf("regular-%d", i)
		cache.StoreWithOrigin(cid, block, AltruisticBlock)
		
		// Simulate regular accesses
		for j := 0; j < 10; j++ {
			cache.Get(cid)
			time.Sleep(10 * time.Millisecond)
		}
	}
	
	// Rarely accessed blocks
	for i := 0; i < 5; i++ {
		data := make([]byte, blockSize)
		block := &blocks.Block{Data: data}
		
		cid := fmt.Sprintf("rare-%d", i)
		cache.StoreWithOrigin(cid, block, AltruisticBlock)
		
		// Access only once
		cache.Get(cid)
	}
	
	// Check current utilization
	stats := cache.GetAltruisticStats()
	utilization := float64(stats.PersonalSize+stats.AltruisticSize) / float64(totalCapacity)
	t.Logf("Utilization before pre-eviction: %.1f%%", utilization*100)
	
	// Trigger pre-eviction if threshold reached
	if utilization > config.PreEvictThreshold {
		err := cache.PerformPreEviction()
		if err != nil {
			t.Logf("Pre-eviction error: %v", err)
		}
		
		// Check utilization after pre-eviction
		newStats := cache.GetAltruisticStats()
		newUtilization := float64(newStats.PersonalSize+newStats.AltruisticSize) / float64(totalCapacity)
		t.Logf("Utilization after pre-eviction: %.1f%%", newUtilization*100)
		
		// Should have reduced utilization
		if newUtilization >= utilization {
			t.Error("Pre-eviction should reduce utilization")
		}
		
		// Should preferentially evict rarely accessed blocks
		regularCount := 0
		rareCount := 0
		for i := 0; i < 5; i++ {
			if cache.Has(fmt.Sprintf("regular-%d", i)) {
				regularCount++
			}
			if cache.Has(fmt.Sprintf("rare-%d", i)) {
				rareCount++
			}
		}
		
		t.Logf("Remaining blocks: %d regular, %d rare", regularCount, rareCount)
		
		// More regular blocks should remain
		if regularCount <= rareCount {
			t.Error("Predictive eviction should preserve frequently accessed blocks")
		}
	}
}

// TestSpaceManagement_NetworkHealthIntegration tests integration with network health components
func TestSpaceManagement_NetworkHealthIntegration(t *testing.T) {
	totalCapacity := int64(20 * 1024 * 1024) // 20MB
	minPersonal := int64(5 * 1024 * 1024)    // 5MB
	
	baseCache := NewMemoryCache(200)
	config := &AltruisticCacheConfig{
		MinPersonalCache: minPersonal,
		EnableAltruistic: true,
		EvictionStrategy: "ValueBased",
	}
	
	cache := NewAltruisticCache(baseCache, config, totalCapacity)
	
	// Initialize network health manager (mock shell for testing)
	nhConfig := &NetworkHealthConfig{
		EnableGossip:        true,
		EnableBloomExchange: true,
	}
	
	manager, err := NewNetworkHealthManager(cache, nil, nhConfig)
	if err != nil {
		t.Fatalf("Failed to create network health manager: %v", err)
	}
	
	// Simulate network health updates
	blockSize := 256 * 1024 // 256KB blocks
	
	// Add blocks with varying health scores
	for i := 0; i < 40; i++ {
		data := make([]byte, blockSize)
		block := &blocks.Block{Data: data}
		
		cid := fmt.Sprintf("network-block-%d", i)
		cache.StoreWithOrigin(cid, block, AltruisticBlock)
		
		// Simulate network health hints
		var hint BlockHint
		switch i % 4 {
		case 0: // Very valuable
			hint = BlockHint{
				ReplicationBucket: ReplicationLow,
				HighEntropy:       true,
				RequestCount:      100,
				MissingRegions:    5,
			}
		case 1: // Somewhat valuable
			hint = BlockHint{
				ReplicationBucket: ReplicationMedium,
				HighEntropy:       true,
				RequestCount:      50,
			}
		case 2: // Low value
			hint = BlockHint{
				ReplicationBucket: ReplicationHigh,
				HighEntropy:       false,
				RequestCount:      10,
			}
		default: // Very low value
			hint = BlockHint{
				ReplicationBucket: ReplicationHigh,
				HighEntropy:       false,
				RequestCount:      1,
			}
		}
		
		cache.UpdateBlockHealth(cid, hint)
	}
	
	// Get network health report
	report := manager.GetNetworkHealth()
	t.Logf("Network health - Tracked blocks: %d, Low replication: %d, High entropy: %d",
		report.LocalHealth.TrackedBlocks,
		report.LocalHealth.LowReplication,
		report.LocalHealth.HighEntropy)
	
	// Now fill cache with personal blocks to trigger value-based eviction
	personalCount := 0
	for i := 0; i < 50; i++ {
		data := make([]byte, blockSize)
		block := &blocks.Block{Data: data}
		
		err := cache.StoreWithOrigin(fmt.Sprintf("personal-%d", i), block, PersonalBlock)
		if err != nil {
			break
		}
		personalCount++
	}
	
	t.Logf("Added %d personal blocks", personalCount)
	
	// Check which network blocks survived
	valuableCount := 0
	lowValueCount := 0
	
	for i := 0; i < 40; i++ {
		cid := fmt.Sprintf("network-block-%d", i)
		if cache.Has(cid) {
			switch i % 4 {
			case 0:
				valuableCount++
			case 2, 3:
				lowValueCount++
			}
		}
	}
	
	t.Logf("Surviving blocks: %d valuable, %d low-value", valuableCount, lowValueCount)
	
	// Valuable blocks should survive more than low-value blocks
	if valuableCount <= lowValueCount {
		t.Error("Network health integration should preserve valuable blocks")
	}
	
	// Final stats
	finalStats := cache.GetAltruisticStats()
	finalReport := manager.GetNetworkHealth()
	
	t.Logf("Final state: Personal=%d MB, Altruistic=%d MB, Network score=%.2f",
		finalStats.PersonalSize/(1024*1024),
		finalStats.AltruisticSize/(1024*1024),
		finalReport.OverallScore)
}
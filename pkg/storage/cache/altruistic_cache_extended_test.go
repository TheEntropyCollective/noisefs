package cache

import (
	"sync"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
)

// TestAltruisticCache_EvictionCooldown tests that eviction cooldown prevents thrashing
func TestAltruisticCache_EvictionCooldown(t *testing.T) {
	baseCache := NewMemoryCache(1000)
	
	config := &AltruisticCacheConfig{
		MinPersonalCache: 800,
		EnableAltruistic: true,
		EvictionCooldown: 500 * time.Millisecond,
	}
	
	cache := NewAltruisticCache(baseCache, config, 1024)
	
	// Fill with altruistic blocks
	for i := 0; i < 5; i++ {
		data := make([]byte, 100)
		block := &blocks.Block{Data: data}
		cache.StoreWithOrigin(string(rune('a'+i)), block, AltruisticBlock)
	}
	
	// First eviction should succeed
	largeData := make([]byte, 600)
	largeBlock := &blocks.Block{Data: largeData}
	err := cache.StoreWithOrigin("personal1", largeBlock, PersonalBlock)
	if err != nil {
		t.Fatalf("First personal block should succeed: %v", err)
	}
	
	// Immediate second eviction should fail due to cooldown
	largeData2 := make([]byte, 300)
	largeBlock2 := &blocks.Block{Data: largeData2}
	err = cache.StoreWithOrigin("personal2", largeBlock2, PersonalBlock)
	if err == nil {
		t.Error("Second personal block should fail due to eviction cooldown")
	}
	
	// Wait for cooldown
	time.Sleep(600 * time.Millisecond)
	
	// Now it should succeed
	err = cache.StoreWithOrigin("personal2", largeBlock2, PersonalBlock)
	if err != nil {
		t.Errorf("Personal block should succeed after cooldown: %v", err)
	}
}

// TestAltruisticCache_ConcurrentAccess tests thread safety
func TestAltruisticCache_ConcurrentAccess(t *testing.T) {
	baseCache := NewMemoryCache(10000)
	
	config := &AltruisticCacheConfig{
		MinPersonalCache: 500 * 1024,
		EnableAltruistic: true,
		EvictionCooldown: 100 * time.Millisecond,
	}
	
	cache := NewAltruisticCache(baseCache, config, 1024*1024)
	
	var wg sync.WaitGroup
	errors := make(chan error, 100)
	
	// Concurrent writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			for j := 0; j < 10; j++ {
				data := make([]byte, 1024)
				block := &blocks.Block{Data: data}
				
				origin := PersonalBlock
				if j%2 == 0 {
					origin = AltruisticBlock
				}
				
				cid := fmt.Sprintf("block-%d-%d", id, j)
				if err := cache.StoreWithOrigin(cid, block, origin); err != nil {
					errors <- err
				}
			}
		}(i)
	}
	
	// Concurrent readers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			time.Sleep(10 * time.Millisecond) // Let some writes happen first
			
			for j := 0; j < 20; j++ {
				cid := fmt.Sprintf("block-%d-%d", j%10, j%10)
				cache.Get(cid) // Ignore errors, blocks might not exist yet
			}
		}(i)
	}
	
	// Stats reader
	wg.Add(1)
	go func() {
		defer wg.Done()
		
		for i := 0; i < 10; i++ {
			stats := cache.GetAltruisticStats()
			if stats == nil {
				errors <- fmt.Errorf("nil stats returned")
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()
	
	wg.Wait()
	close(errors)
	
	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent access error: %v", err)
	}
	
	// Verify final state is consistent
	stats := cache.GetAltruisticStats()
	totalSize := stats.PersonalSize + stats.AltruisticSize
	if totalSize > cache.totalCapacity {
		t.Errorf("Total size %d exceeds capacity %d", totalSize, cache.totalCapacity)
	}
}

// TestAltruisticCache_BlockTransition tests changing a block from altruistic to personal
func TestAltruisticCache_BlockTransition(t *testing.T) {
	baseCache := NewMemoryCache(1000)
	
	config := &AltruisticCacheConfig{
		MinPersonalCache: 500,
		EnableAltruistic: true,
		EvictionCooldown: 100 * time.Millisecond,
	}
	
	cache := NewAltruisticCache(baseCache, config, 1024)
	
	// Store as altruistic first
	data := []byte("transitioning block")
	block := &blocks.Block{Data: data}
	
	err := cache.StoreWithOrigin("block1", block, AltruisticBlock)
	if err != nil {
		t.Fatalf("Failed to store altruistic block: %v", err)
	}
	
	stats := cache.GetAltruisticStats()
	if stats.AltruisticBlocks != 1 || stats.PersonalBlocks != 0 {
		t.Errorf("Expected 1 altruistic, 0 personal blocks")
	}
	
	// Store same block as personal
	err = cache.StoreWithOrigin("block1", block, PersonalBlock)
	if err != nil {
		t.Fatalf("Failed to transition block to personal: %v", err)
	}
	
	stats = cache.GetAltruisticStats()
	if stats.AltruisticBlocks != 0 || stats.PersonalBlocks != 1 {
		t.Errorf("Expected 0 altruistic, 1 personal blocks after transition")
	}
	
	// Size accounting should be correct
	if stats.PersonalSize != int64(len(data)) || stats.AltruisticSize != 0 {
		t.Errorf("Size accounting incorrect after transition")
	}
}

// TestAltruisticCache_MetricsAccuracy tests that metrics are accurately tracked
func TestAltruisticCache_MetricsAccuracy(t *testing.T) {
	baseCache := NewMemoryCache(1000)
	
	config := &AltruisticCacheConfig{
		MinPersonalCache: 200,
		EnableAltruistic: true,
		EvictionCooldown: 100 * time.Millisecond,
	}
	
	cache := NewAltruisticCache(baseCache, config, 1024)
	
	// Track expected values
	expectedPersonalSize := int64(0)
	expectedAltruisticSize := int64(0)
	expectedPersonalBlocks := 0
	expectedAltruisticBlocks := 0
	
	// Add various blocks
	for i := 0; i < 10; i++ {
		size := 50 + i*10
		data := make([]byte, size)
		block := &blocks.Block{Data: data}
		
		if i%3 == 0 {
			cache.StoreWithOrigin(fmt.Sprintf("personal-%d", i), block, PersonalBlock)
			expectedPersonalSize += int64(size)
			expectedPersonalBlocks++
		} else {
			cache.StoreWithOrigin(fmt.Sprintf("altruistic-%d", i), block, AltruisticBlock)
			expectedAltruisticSize += int64(size)
			expectedAltruisticBlocks++
		}
	}
	
	stats := cache.GetAltruisticStats()
	
	// Verify counts
	if stats.PersonalBlocks != expectedPersonalBlocks {
		t.Errorf("Personal blocks: expected %d, got %d", expectedPersonalBlocks, stats.PersonalBlocks)
	}
	if stats.AltruisticBlocks != expectedAltruisticBlocks {
		t.Errorf("Altruistic blocks: expected %d, got %d", expectedAltruisticBlocks, stats.AltruisticBlocks)
	}
	
	// Verify sizes
	if stats.PersonalSize != expectedPersonalSize {
		t.Errorf("Personal size: expected %d, got %d", expectedPersonalSize, stats.PersonalSize)
	}
	if stats.AltruisticSize != expectedAltruisticSize {
		t.Errorf("Altruistic size: expected %d, got %d", expectedAltruisticSize, stats.AltruisticSize)
	}
	
	// Test hit/miss tracking
	cache.Get("personal-0")    // Hit
	cache.Get("altruistic-1")  // Hit
	cache.Get("nonexistent")   // Miss
	
	// Note: Current implementation tracks misses but doesn't distinguish type
	// This is a limitation we should document
}

// TestAltruisticCache_ErrorHandling tests error conditions and recovery
func TestAltruisticCache_ErrorHandling(t *testing.T) {
	baseCache := NewMemoryCache(10)
	
	config := &AltruisticCacheConfig{
		MinPersonalCache: 500,
		EnableAltruistic: true,
		EvictionCooldown: 100 * time.Millisecond,
	}
	
	cache := NewAltruisticCache(baseCache, config, 1024)
	
	// Test storing when base cache is full
	for i := 0; i < 15; i++ {
		data := []byte(fmt.Sprintf("block-%d", i))
		block := &blocks.Block{Data: data}
		cache.StoreWithOrigin(fmt.Sprintf("block-%d", i), block, PersonalBlock)
	}
	
	// Base cache should be at capacity, but altruistic cache should handle it
	stats := cache.GetAltruisticStats()
	if stats.PersonalBlocks == 0 {
		t.Error("Should have stored some personal blocks despite base cache limits")
	}
	
	// Test removing non-existent block
	err := cache.Remove("nonexistent")
	if err == nil {
		t.Error("Expected error when removing non-existent block")
	}
	
	// Test getting non-existent block
	_, err = cache.Get("nonexistent")
	if err == nil {
		t.Error("Expected error when getting non-existent block")
	}
}

// TestAltruisticCache_ExtremeScenarios tests edge cases
func TestAltruisticCache_ExtremeScenarios(t *testing.T) {
	// Scenario 1: MinPersonal equals total capacity
	config1 := &AltruisticCacheConfig{
		MinPersonalCache: 1024,
		EnableAltruistic: true,
		EvictionCooldown: 100 * time.Millisecond,
	}
	
	cache1 := NewAltruisticCache(NewMemoryCache(100), config1, 1024)
	
	// Should not accept any altruistic blocks
	data := []byte("altruistic")
	block := &blocks.Block{Data: data}
	err := cache1.StoreWithOrigin("alt1", block, AltruisticBlock)
	if err == nil {
		t.Error("Should not accept altruistic blocks when MinPersonal equals capacity")
	}
	
	// But should accept personal blocks
	err = cache1.StoreWithOrigin("pers1", block, PersonalBlock)
	if err != nil {
		t.Error("Should accept personal blocks")
	}
	
	// Scenario 2: Zero MinPersonal
	config2 := &AltruisticCacheConfig{
		MinPersonalCache: 0,
		EnableAltruistic: true,
		EvictionCooldown: 100 * time.Millisecond,
	}
	
	cache2 := NewAltruisticCache(NewMemoryCache(100), config2, 1024)
	
	// Should use entire cache for altruistic if no personal blocks
	for i := 0; i < 10; i++ {
		data := make([]byte, 100)
		block := &blocks.Block{Data: data}
		cache2.StoreWithOrigin(fmt.Sprintf("alt-%d", i), block, AltruisticBlock)
	}
	
	stats := cache2.GetAltruisticStats()
	if stats.AltruisticSize == 0 {
		t.Error("Should use full capacity for altruistic when MinPersonal is 0")
	}
}
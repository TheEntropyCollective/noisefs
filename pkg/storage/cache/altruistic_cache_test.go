package cache

import (
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
)

func TestAltruisticCache_BasicFunctionality(t *testing.T) {
	// Create a memory cache as base
	baseCache := NewMemoryCache(1000)
	
	// Create altruistic cache with 1MB total, 500KB personal minimum
	config := &AltruisticCacheConfig{
		MinPersonalCache: 500 * 1024,
		EnableAltruistic: true,
		EvictionCooldown: 100 * time.Millisecond,
	}
	
	cache := NewAltruisticCache(baseCache, config, 1024*1024)
	
	// Test storing personal blocks
	personalData := []byte("personal block data")
	personalBlock := &blocks.Block{Data: personalData}
	
	err := cache.StoreWithOrigin("personal1", personalBlock, PersonalBlock)
	if err != nil {
		t.Fatalf("Failed to store personal block: %v", err)
	}
	
	// Test storing altruistic blocks
	altruisticData := []byte("altruistic block data")
	altruisticBlock := &blocks.Block{Data: altruisticData}
	
	err = cache.StoreWithOrigin("altruistic1", altruisticBlock, AltruisticBlock)
	if err != nil {
		t.Fatalf("Failed to store altruistic block: %v", err)
	}
	
	// Test retrieval
	retrieved, err := cache.Get("personal1")
	if err != nil {
		t.Fatalf("Failed to get personal block: %v", err)
	}
	if string(retrieved.Data) != string(personalData) {
		t.Errorf("Retrieved data mismatch")
	}
	
	retrieved, err = cache.Get("altruistic1")
	if err != nil {
		t.Fatalf("Failed to get altruistic block: %v", err)
	}
	if string(retrieved.Data) != string(altruisticData) {
		t.Errorf("Retrieved data mismatch")
	}
	
	// Check stats
	stats := cache.GetAltruisticStats()
	if stats.PersonalBlocks != 1 {
		t.Errorf("Expected 1 personal block, got %d", stats.PersonalBlocks)
	}
	if stats.AltruisticBlocks != 1 {
		t.Errorf("Expected 1 altruistic block, got %d", stats.AltruisticBlocks)
	}
}

func TestAltruisticCache_SpaceManagement(t *testing.T) {
	baseCache := NewMemoryCache(1000)
	
	// Small cache: 1KB total, 600B personal minimum
	config := &AltruisticCacheConfig{
		MinPersonalCache: 600,
		EnableAltruistic: true,
		EvictionCooldown: 100 * time.Millisecond,
	}
	
	cache := NewAltruisticCache(baseCache, config, 1024)
	
	// Fill with altruistic blocks
	for i := 0; i < 5; i++ {
		data := make([]byte, 200)
		block := &blocks.Block{Data: data}
		cache.StoreWithOrigin(string(rune('a'+i)), block, AltruisticBlock)
	}
	
	// Now add personal blocks that require evicting altruistic
	largePersonalData := make([]byte, 700)
	largePersonalBlock := &blocks.Block{Data: largePersonalData}
	
	err := cache.StoreWithOrigin("large-personal", largePersonalBlock, PersonalBlock)
	if err != nil {
		t.Fatalf("Failed to store large personal block: %v", err)
	}
	
	// Check that altruistic blocks were evicted
	stats := cache.GetAltruisticStats()
	if stats.PersonalSize < 700 {
		t.Errorf("Personal size should be at least 700, got %d", stats.PersonalSize)
	}
	if stats.AltruisticSize > 324 { // 1024 - 700
		t.Errorf("Altruistic size should be at most 324, got %d", stats.AltruisticSize)
	}
}

func TestAltruisticCache_MinPersonalGuarantee(t *testing.T) {
	baseCache := NewMemoryCache(1000)
	
	// 1KB total, 800B personal minimum
	config := &AltruisticCacheConfig{
		MinPersonalCache: 800,
		EnableAltruistic: true,
		EvictionCooldown: 100 * time.Millisecond,
	}
	
	cache := NewAltruisticCache(baseCache, config, 1024)
	
	// Try to fill with altruistic blocks
	for i := 0; i < 10; i++ {
		data := make([]byte, 200)
		block := &blocks.Block{Data: data}
		cache.StoreWithOrigin(string(rune('a'+i)), block, AltruisticBlock)
	}
	
	// Check that altruistic didn't violate personal minimum
	stats := cache.GetAltruisticStats()
	availableForPersonal := cache.totalCapacity - stats.AltruisticSize
	if availableForPersonal < config.MinPersonalCache {
		t.Errorf("Personal minimum violated: available %d < minimum %d", 
			availableForPersonal, config.MinPersonalCache)
	}
}

func TestAltruisticCache_DisabledAltruistic(t *testing.T) {
	baseCache := NewMemoryCache(1000)
	
	config := &AltruisticCacheConfig{
		MinPersonalCache: 500,
		EnableAltruistic: false, // Disabled
		EvictionCooldown: 100 * time.Millisecond,
	}
	
	cache := NewAltruisticCache(baseCache, config, 1024)
	
	// Try to store altruistic block
	data := []byte("altruistic data")
	block := &blocks.Block{Data: data}
	
	err := cache.StoreWithOrigin("altruistic1", block, AltruisticBlock)
	if err == nil {
		t.Error("Expected error when storing altruistic block with disabled altruistic caching")
	}
	
	// Personal blocks should still work
	err = cache.StoreWithOrigin("personal1", block, PersonalBlock)
	if err != nil {
		t.Errorf("Failed to store personal block: %v", err)
	}
}

func TestAltruisticCache_FlexPoolUsage(t *testing.T) {
	baseCache := NewMemoryCache(1000)
	
	// 1MB total, 400KB personal minimum
	config := &AltruisticCacheConfig{
		MinPersonalCache: 400 * 1024,
		EnableAltruistic: true,
		EvictionCooldown: 100 * time.Millisecond,
	}
	
	cache := NewAltruisticCache(baseCache, config, 1024*1024)
	
	// Initially, flex pool usage should be 0
	stats := cache.GetAltruisticStats()
	if stats.FlexPoolUsage != 0 {
		t.Errorf("Initial flex pool usage should be 0, got %f", stats.FlexPoolUsage)
	}
	
	// Add 200KB personal (within minimum)
	data := make([]byte, 200*1024)
	block := &blocks.Block{Data: data}
	cache.StoreWithOrigin("personal1", block, PersonalBlock)
	
	stats = cache.GetAltruisticStats()
	if stats.FlexPoolUsage != 0 {
		t.Errorf("Flex pool usage should still be 0 when personal < minimum, got %f", stats.FlexPoolUsage)
	}
	
	// Add 300KB more personal (now 500KB total, exceeding minimum)
	data = make([]byte, 300*1024)
	block = &blocks.Block{Data: data}
	cache.StoreWithOrigin("personal2", block, PersonalBlock)
	
	stats = cache.GetAltruisticStats()
	// Flex pool is 600KB (1MB - 400KB min), used 100KB (500KB - 400KB min)
	expectedUsage := 100.0 / 600.0
	if abs(stats.FlexPoolUsage - expectedUsage) > 0.01 {
		t.Errorf("Flex pool usage should be %f, got %f", expectedUsage, stats.FlexPoolUsage)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
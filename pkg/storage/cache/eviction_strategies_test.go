package cache

import (
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
)

func TestLRUEvictionStrategy(t *testing.T) {
	strategy := &LRUEvictionStrategy{}
	
	// Create test blocks with different access times
	now := time.Now()
	blocks := map[string]*BlockMetadata{
		"old": {
			BlockInfo:    &BlockInfo{CID: "old", Size: 1024},
			LastAccessed: now.Add(-2 * time.Hour),
		},
		"recent": {
			BlockInfo:    &BlockInfo{CID: "recent", Size: 1024},
			LastAccessed: now.Add(-5 * time.Minute),
		},
		"veryold": {
			BlockInfo:    &BlockInfo{CID: "veryold", Size: 1024},
			LastAccessed: now.Add(-24 * time.Hour),
		},
	}
	
	// Select candidates
	candidates := strategy.SelectEvictionCandidates(blocks, 2048, nil)
	
	// Should evict oldest first
	if len(candidates) != 2 {
		t.Errorf("Expected 2 candidates, got %d", len(candidates))
	}
	
	if candidates[0].CID != "veryold" {
		t.Errorf("Expected veryold first, got %s", candidates[0].CID)
	}
	
	if candidates[1].CID != "old" {
		t.Errorf("Expected old second, got %s", candidates[1].CID)
	}
}

func TestLFUEvictionStrategy(t *testing.T) {
	strategy := &LFUEvictionStrategy{}
	
	// Create test blocks with different access frequencies
	now := time.Now()
	blocks := map[string]*BlockMetadata{
		"popular": {
			BlockInfo:    &BlockInfo{CID: "popular", Size: 1024, Popularity: 100},
			CachedAt:     now.Add(-10 * time.Hour),
		},
		"unpopular": {
			BlockInfo:    &BlockInfo{CID: "unpopular", Size: 1024, Popularity: 2},
			CachedAt:     now.Add(-10 * time.Hour),
		},
		"medium": {
			BlockInfo:    &BlockInfo{CID: "medium", Size: 1024, Popularity: 20},
			CachedAt:     now.Add(-10 * time.Hour),
		},
	}
	
	// Select candidates
	candidates := strategy.SelectEvictionCandidates(blocks, 2048, nil)
	
	// Should evict least frequently used first
	if len(candidates) != 2 {
		t.Errorf("Expected 2 candidates, got %d", len(candidates))
	}
	
	if candidates[0].CID != "unpopular" {
		t.Errorf("Expected unpopular first, got %s", candidates[0].CID)
	}
}

func TestValueBasedEvictionStrategy(t *testing.T) {
	strategy := NewValueBasedEvictionStrategy()
	healthTracker := NewBlockHealthTracker(nil)
	
	// Add health information
	healthTracker.UpdateBlockHealth("valuable", BlockHint{
		ReplicationBucket: ReplicationLow,
		HighEntropy:       true,
		MissingRegions:    5,
	})
	
	healthTracker.UpdateBlockHealth("common", BlockHint{
		ReplicationBucket: ReplicationHigh,
		HighEntropy:       false,
		MissingRegions:    0,
	})
	
	// Create test blocks
	now := time.Now()
	blocks := map[string]*BlockMetadata{
		"valuable": {
			BlockInfo:    &BlockInfo{CID: "valuable", Size: 1024},
			LastAccessed: now.Add(-1 * time.Hour),
			Origin:       AltruisticBlock,
		},
		"common": {
			BlockInfo:    &BlockInfo{CID: "common", Size: 1024},
			LastAccessed: now.Add(-1 * time.Hour),
			Origin:       AltruisticBlock,
		},
	}
	
	// Calculate scores
	valuableScore := strategy.Score(blocks["valuable"], healthTracker)
	commonScore := strategy.Score(blocks["common"], healthTracker)
	
	// Common block should have higher eviction score (more likely to evict)
	if commonScore <= valuableScore {
		t.Errorf("Common block should have higher eviction score: common=%f, valuable=%f",
			commonScore, valuableScore)
	}
}

func TestAdaptiveEvictionStrategy(t *testing.T) {
	strategy := NewAdaptiveEvictionStrategy()
	
	// Create many blocks to simulate different utilization levels
	blocks := make(map[string]*BlockMetadata)
	now := time.Now()
	
	for i := 0; i < 100; i++ {
		blocks[string(rune('a'+i%26))+string(rune('0'+i/26))] = &BlockMetadata{
			BlockInfo:    &BlockInfo{CID: string(rune('a'+i%26)) + string(rune('0'+i/26)), Size: 1024, Popularity: i % 10},
			LastAccessed: now.Add(-time.Duration(i) * time.Minute),
			CachedAt:     now.Add(-24 * time.Hour),
		}
	}
	
	// Test selection (should adapt based on utilization)
	candidates := strategy.SelectEvictionCandidates(blocks, 10240, nil)
	
	if len(candidates) < 10 {
		t.Errorf("Expected at least 10 candidates, got %d", len(candidates))
	}
}

func TestGradualEvictionStrategy(t *testing.T) {
	base := &LRUEvictionStrategy{}
	strategy := NewGradualEvictionStrategy(base)
	
	// Create test blocks
	blocks := make(map[string]*BlockMetadata)
	now := time.Now()
	totalSize := int64(0)
	
	for i := 0; i < 20; i++ {
		size := 1024 * (i%3 + 1) // Vary sizes
		blocks[string(rune('a'+i))] = &BlockMetadata{
			BlockInfo:    &BlockInfo{CID: string(rune('a' + i)), Size: size},
			LastAccessed: now.Add(-time.Duration(i) * time.Hour),
		}
		totalSize += int64(size)
	}
	
	// Request small eviction
	candidates := strategy.SelectEvictionCandidates(blocks, 1024, nil)
	
	// Should evict more than requested (buffer) but not too much
	evictedSize := int64(0)
	for _, c := range candidates {
		evictedSize += int64(c.Size)
	}
	
	// Should evict at least requested amount
	if evictedSize < 1024 {
		t.Errorf("Evicted too little: %d < 1024", evictedSize)
	}
	
	// But not more than max ratio (10% of total)
	maxEvict := int64(float64(totalSize) * 0.1)
	if evictedSize > maxEvict {
		t.Errorf("Evicted too much: %d > %d (max)", evictedSize, maxEvict)
	}
}

func TestEvictionStrategyIntegration(t *testing.T) {
	// Test with real altruistic cache
	baseCache := NewMemoryCache(100)
	config := &AltruisticCacheConfig{
		MinPersonalCache:      5 * 1024,
		EnableAltruistic:      true,
		EvictionCooldown:      100 * time.Millisecond,
		EvictionStrategy:      "ValueBased",
		EnableGradualEviction: true,
	}
	
	cache := NewAltruisticCache(baseCache, config, 10*1024)
	
	// Add blocks with different characteristics
	for i := 0; i < 8; i++ {
		data := make([]byte, 1024)
		block := &blocks.Block{Data: data}
		
		cid := string(rune('a' + i))
		cache.StoreWithOrigin(cid, block, AltruisticBlock)
		
		// Update health for some blocks
		if i%2 == 0 {
			cache.UpdateBlockHealth(cid, BlockHint{
				ReplicationBucket: ReplicationLow,
				HighEntropy:       true,
			})
		}
	}
	
	// Now add personal block that requires eviction
	personalData := make([]byte, 3*1024)
	personalBlock := &blocks.Block{Data: personalData}
	
	err := cache.StoreWithOrigin("personal", personalBlock, PersonalBlock)
	if err != nil {
		t.Fatalf("Failed to store personal block: %v", err)
	}
	
	// Check that valuable blocks were preserved
	stats := cache.GetAltruisticStats()
	if stats.AltruisticBlocks == 0 {
		t.Error("All altruistic blocks were evicted")
	}
}
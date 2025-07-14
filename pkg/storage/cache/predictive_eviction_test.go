package cache

import (
	"testing"
	"time"
	
	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
)

func TestPredictiveEvictor_AccessPrediction(t *testing.T) {
	config := &PredictiveEvictorConfig{
		PredictionWindow:  24 * time.Hour,
		UpdateInterval:    15 * time.Minute,
		PreEvictThreshold: 0.85,
	}
	
	predictor := NewPredictiveEvictor(config)
	
	// Record regular access pattern
	now := time.Now()
	blockID := "regular-block"
	
	// Simulate accesses every 2 hours
	for i := 0; i < 12; i++ {
		accessTime := now.Add(-time.Duration(i*2) * time.Hour)
		predictor.RecordAccess(blockID, accessTime)
	}
	
	// Predict next access
	nextAccess, confidence := predictor.PredictNextAccess(blockID)
	
	// Should predict around 2 hours from last access
	expectedTime := now.Add(2 * time.Hour)
	timeDiff := nextAccess.Sub(expectedTime)
	if timeDiff < 0 {
		timeDiff = -timeDiff
	}
	
	if timeDiff > 30*time.Minute {
		t.Errorf("Prediction off by %v, expected around %v, got %v",
			timeDiff, expectedTime, nextAccess)
	}
	
	// Should have reasonable confidence with regular pattern
	if confidence < 0.5 {
		t.Errorf("Expected higher confidence for regular pattern, got %f", confidence)
	}
}

func TestPredictiveEvictor_IrregularPattern(t *testing.T) {
	predictor := NewPredictiveEvictor(nil)
	
	// Record irregular access pattern
	now := time.Now()
	blockID := "irregular-block"
	
	// Random intervals
	intervals := []time.Duration{
		1 * time.Hour,
		5 * time.Hour,
		30 * time.Minute,
		12 * time.Hour,
		2 * time.Hour,
	}
	
	accessTime := now
	for _, interval := range intervals {
		accessTime = accessTime.Add(-interval)
		predictor.RecordAccess(blockID, accessTime)
	}
	
	// Predict next access
	_, confidence := predictor.PredictNextAccess(blockID)
	
	// Should have low confidence for irregular pattern
	if confidence > 0.5 {
		t.Errorf("Expected low confidence for irregular pattern, got %f", confidence)
	}
}

func TestPredictiveEvictor_EvictionCandidates(t *testing.T) {
	predictor := NewPredictiveEvictor(nil)
	now := time.Now()
	
	// Create blocks with different access patterns
	blocks := map[string]*BlockMetadata{
		"frequent": {
			BlockInfo: &BlockInfo{CID: "frequent"},
		},
		"rare": {
			BlockInfo: &BlockInfo{CID: "rare"},
		},
		"never": {
			BlockInfo: &BlockInfo{CID: "never"},
		},
	}
	
	// Record access patterns
	// Frequent: accessed every hour
	for i := 0; i < 24; i++ {
		predictor.RecordAccess("frequent", now.Add(-time.Duration(i)*time.Hour))
	}
	
	// Rare: accessed once a day
	for i := 0; i < 7; i++ {
		predictor.RecordAccess("rare", now.Add(-time.Duration(i*24)*time.Hour))
	}
	
	// Never: no access history
	
	// Get eviction candidates
	candidates := predictor.GetEvictionCandidates(blocks, 2)
	
	// Should prioritize blocks with longest time until next access
	if len(candidates) != 2 {
		t.Errorf("Expected 2 candidates, got %d", len(candidates))
	}
	
	// "never" should be first (no predicted access)
	foundNever := false
	foundFrequent := false
	for _, cid := range candidates {
		if cid == "never" {
			foundNever = true
		}
		if cid == "frequent" {
			foundFrequent = true
		}
	}
	
	if !foundNever {
		t.Error("Expected 'never' block in eviction candidates")
	}
	
	if foundFrequent {
		t.Error("'frequent' block should not be in eviction candidates")
	}
}

func TestPredictiveEvictor_PreEviction(t *testing.T) {
	config := &PredictiveEvictorConfig{
		PreEvictThreshold: 0.85,
	}
	
	predictor := NewPredictiveEvictor(config)
	
	// Test threshold
	if predictor.ShouldPreEvict(0.8) {
		t.Error("Should not pre-evict below threshold")
	}
	
	if !predictor.ShouldPreEvict(0.9) {
		t.Error("Should pre-evict above threshold")
	}
	
	// Test pre-eviction size calculation
	totalCapacity := int64(1000 * 1024 * 1024) // 1GB
	utilization := 0.9 // 90% full
	
	evictSize := predictor.GetPreEvictionSize(utilization, totalCapacity)
	
	// Should evict to 75% (15% of total)
	expectedSize := int64(0.15 * float64(totalCapacity))
	
	if evictSize != expectedSize {
		t.Errorf("Expected eviction size %d, got %d", expectedSize, evictSize)
	}
}

func TestPredictiveEvictionIntegration(t *testing.T) {
	// Create cache with predictive eviction
	baseCache := NewMemoryCache(100)
	config := &AltruisticCacheConfig{
		MinPersonalCache:  5 * 1024,
		EnableAltruistic:  true,
		EnablePredictive:  true,
		PreEvictThreshold: 0.8,
		EvictionCooldown:  100 * time.Millisecond,
	}
	
	cache := NewAltruisticCache(baseCache, config, 10*1024)
	
	// Fill cache to 70%
	for i := 0; i < 7; i++ {
		data := make([]byte, 1024)
		block, _ := blocks.NewBlock(data)
		cache.StoreWithOrigin(string(rune('a'+i)), block, AltruisticBlock)
	}
	
	// Simulate access patterns
	for i := 0; i < 7; i++ {
		// Some blocks accessed frequently
		if i%2 == 0 {
			for j := 0; j < 5; j++ {
				cache.Get(string(rune('a' + i)))
				time.Sleep(10 * time.Millisecond)
			}
		}
	}
	
	// Add more blocks to trigger pre-eviction
	data := make([]byte, 2*1024)
	block := &blocks.Block{Data: data}
	cache.StoreWithOrigin("trigger", block, AltruisticBlock)
	
	// Trigger pre-eviction
	err := cache.PerformPreEviction()
	if err != nil {
		t.Logf("Pre-eviction error (might be expected): %v", err)
	}
	
	// Check that frequently accessed blocks were preserved
	for i := 0; i < 7; i += 2 {
		if !cache.Has(string(rune('a' + i))) {
			t.Errorf("Frequently accessed block %c was evicted", rune('a'+i))
		}
	}
}
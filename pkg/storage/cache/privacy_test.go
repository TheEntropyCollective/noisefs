package cache

import (
	"testing"
	"time"
)

func TestDifferentialPrivacy(t *testing.T) {
	config := &AdaptiveCacheConfig{
		MaxSize:            1024 * 1024, // 1MB
		MaxItems:           100,
		PrivacyEpsilon:     1.0, // Strong privacy
		TemporalQuantum:    time.Hour,
		DummyAccessRate:    0.1, // 10% dummy accesses
		PredictionInterval: time.Minute,
		ExchangeInterval:   time.Minute * 5,
	}
	
	cache := NewAdaptiveCache(config)
	
	// Test differential privacy noise addition
	originalValue := 5.0
	noisyValue := cache.addDifferentialPrivacyNoise(originalValue)
	
	// Noise should be added (very unlikely to be exactly the same)
	if noisyValue == originalValue {
		t.Logf("Warning: Noisy value identical to original (possible but unlikely)")
	}
	
	// Value should be non-negative
	if noisyValue < 0 {
		t.Errorf("Noisy value should be non-negative, got %f", noisyValue)
	}
	
	t.Logf("Original: %f, Noisy: %f", originalValue, noisyValue)
}

func TestTemporalQuantization(t *testing.T) {
	config := &AdaptiveCacheConfig{
		MaxSize:         1024 * 1024,
		MaxItems:        100,
		TemporalQuantum: time.Hour, // Quantize to hour boundaries
	}
	
	cache := NewAdaptiveCache(config)
	
	// Test timestamp quantization
	now := time.Now()
	quantized := cache.quantizeTimestamp(now)
	
	// Should be rounded down to the hour
	expectedHour := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location())
	
	if !quantized.Equal(expectedHour) {
		t.Errorf("Expected quantized time %v, got %v", expectedHour, quantized)
	}
	
	// Test with zero quantum (no quantization)
	cache.config.TemporalQuantum = 0
	unquantized := cache.quantizeTimestamp(now)
	if !unquantized.Equal(now) {
		t.Errorf("Expected unquantized time to be unchanged")
	}
}

func TestBloomFilterCacheHint(t *testing.T) {
	// Test Bloom filter creation
	filter := NewBloomFilter(100, 0.01)
	
	// Add some items
	testItems := []string{"block1", "block2", "block3"}
	for _, item := range testItems {
		filter.Add(item)
	}
	
	// Test positive cases
	for _, item := range testItems {
		if !filter.Contains(item) {
			t.Errorf("Bloom filter should contain %s", item)
		}
	}
	
	// Test negative case (might have false positives)
	if filter.Contains("nonexistent") {
		t.Logf("False positive detected (expected behavior)")
	}
	
	// Test cache hint creation
	config := &AdaptiveCacheConfig{
		MaxSize:  1024 * 1024,
		MaxItems: 100,
	}
	cache := NewAdaptiveCache(config)
	
	// Add some items to cache
	for i, item := range testItems {
		data := []byte("test data")
		cache.Put(item, data, map[string]interface{}{
			"test": i,
		})
	}
	
	// Create cache hint
	hint := cache.CreateCacheHint()
	if hint == nil {
		t.Fatal("Cache hint should not be nil")
	}
	
	// Test querying the hint
	for _, item := range testItems {
		if !hint.QueryCacheHint(item) {
			t.Errorf("Cache hint should indicate %s is available", item)
		}
	}
}

func TestDummyAccessInjection(t *testing.T) {
	config := &AdaptiveCacheConfig{
		MaxSize:         1024 * 1024,
		MaxItems:        100,
		DummyAccessRate: 100.0, // Very high rate for testing
	}
	
	cache := NewAdaptiveCache(config)
	
	// Add an item to cache
	testData := []byte("test data")
	cache.Put("test_cid", testData, map[string]interface{}{})
	
	// Get initial access count
	cache.mutex.RLock()
	item := cache.items["test_cid"]
	initialCount := item.AccessCount
	cache.mutex.RUnlock()
	
	// Force dummy access injection
	cache.lastDummyAccess = time.Now().Add(-2 * time.Hour) // Make it seem like long time ago
	cache.injectDummyAccess()
	
	// Check if access count increased
	cache.mutex.RLock()
	newCount := cache.items["test_cid"].AccessCount
	cache.mutex.RUnlock()
	
	if newCount <= initialCount {
		t.Errorf("Expected access count to increase from dummy access, got %d -> %d", initialCount, newCount)
	}
	
	t.Logf("Access count increased from %d to %d due to dummy access", initialCount, newCount)
}

func TestPrivacyConfigDefaults(t *testing.T) {
	config := &AdaptiveCacheConfig{
		MaxSize:  1024 * 1024,
		MaxItems: 100,
		// No privacy settings - should disable privacy features
	}
	
	cache := NewAdaptiveCache(config)
	
	// Test that privacy features are disabled
	originalValue := 5.0
	noisyValue := cache.addDifferentialPrivacyNoise(originalValue)
	if noisyValue != originalValue {
		t.Errorf("Expected no privacy noise when epsilon is 0, got %f != %f", noisyValue, originalValue)
	}
	
	// Test that temporal quantization is disabled
	now := time.Now()
	quantized := cache.quantizeTimestamp(now)
	if !quantized.Equal(now) {
		t.Errorf("Expected no temporal quantization when quantum is 0")
	}
}
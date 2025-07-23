package cache

import (
	"testing"
	"time"
)

func TestBlockHealthTracker_BasicFunctionality(t *testing.T) {
	config := &BlockHealthConfig{
		PrivacyEpsilon:  1.0,
		TemporalQuantum: time.Hour,
		ValueCacheTime:  5 * time.Minute,
		CleanupInterval: time.Hour,
	}

	tracker := NewBlockHealthTracker(config)

	// Test updating block health
	hint1 := BlockHint{
		ReplicationBucket: ReplicationLow,
		NoisyRequestRate:  2.5,
		HighEntropy:       true,
		MissingRegions:    3,
		LastSeen:          time.Now(),
		Size:              4096,
	}

	tracker.UpdateBlockHealth("block1", hint1)

	// Test calculating value
	value := tracker.CalculateBlockValue("block1", hint1)
	if value <= 0 {
		t.Error("Block value should be positive")
	}

	// Test that low replication scores higher
	hint2 := hint1
	hint2.ReplicationBucket = ReplicationHigh
	value2 := tracker.CalculateBlockValue("block2", hint2)

	if value2 >= value {
		t.Error("Low replication blocks should have higher value")
	}
}

func TestBlockHealthTracker_ValueCalculation(t *testing.T) {
	tracker := NewBlockHealthTracker(nil)

	testCases := []struct {
		name     string
		hint     BlockHint
		minValue float64
	}{
		{
			name: "Under-replicated block",
			hint: BlockHint{
				ReplicationBucket: ReplicationLow,
			},
			minValue: 3.0,
		},
		{
			name: "High entropy randomizer",
			hint: BlockHint{
				ReplicationBucket: ReplicationMedium,
				HighEntropy:       true,
			},
			minValue: 3.0,
		},
		{
			name: "Popular block",
			hint: BlockHint{
				ReplicationBucket: ReplicationMedium,
				NoisyRequestRate:  10.0,
			},
			minValue: 3.0,
		},
		{
			name: "Geographically needed",
			hint: BlockHint{
				ReplicationBucket: ReplicationMedium,
				MissingRegions:    5,
			},
			minValue: 2.5,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			value := tracker.CalculateBlockValue("test", tc.hint)
			if value < tc.minValue {
				t.Errorf("Expected value >= %f, got %f", tc.minValue, value)
			}
		})
	}
}

func TestBlockHealthTracker_GetMostValuableBlocks(t *testing.T) {
	tracker := NewBlockHealthTracker(nil)

	// Add blocks with different values
	blocks := []struct {
		cid  string
		hint BlockHint
	}{
		{
			cid: "high-value",
			hint: BlockHint{
				ReplicationBucket: ReplicationLow,
				HighEntropy:       true,
				MissingRegions:    5,
				Size:              1024,
			},
		},
		{
			cid: "medium-value",
			hint: BlockHint{
				ReplicationBucket: ReplicationMedium,
				NoisyRequestRate:  5.0,
				Size:              1024,
			},
		},
		{
			cid: "low-value",
			hint: BlockHint{
				ReplicationBucket: ReplicationHigh,
				Size:              1024,
			},
		},
		{
			cid: "oversized",
			hint: BlockHint{
				ReplicationBucket: ReplicationLow,
				HighEntropy:       true,
				Size:              10240, // Too big
			},
		},
	}

	for _, b := range blocks {
		tracker.UpdateBlockHealth(b.cid, b.hint)
	}

	// Get most valuable with size limit
	valuable := tracker.GetMostValuableBlocks(3, 5000)

	if len(valuable) != 3 {
		t.Errorf("Expected 3 blocks, got %d", len(valuable))
	}

	// Should be ordered by value
	if valuable[0] != "high-value" {
		t.Errorf("Expected high-value block first, got %s", valuable[0])
	}

	// Oversized block should be excluded
	for _, cid := range valuable {
		if cid == "oversized" {
			t.Error("Oversized block should be excluded")
		}
	}
}

func TestBlockHealthTracker_RequestTracking(t *testing.T) {
	config := &BlockHealthConfig{
		PrivacyEpsilon: 0.0, // Disable noise for testing
	}
	tracker := NewBlockHealthTracker(config)

	// Record multiple requests
	for i := 0; i < 10; i++ {
		tracker.RecordRequest("popular-block")
	}

	tracker.RecordRequest("unpopular-block")

	// Get request rates
	popularRate := tracker.GetBlockRequestRate("popular-block")
	unpopularRate := tracker.GetBlockRequestRate("unpopular-block")

	if popularRate <= unpopularRate {
		t.Error("Popular block should have higher request rate")
	}
}

func TestBlockHealthTracker_PrivacyFeatures(t *testing.T) {
	config := &BlockHealthConfig{
		PrivacyEpsilon:  0.5, // Strong privacy
		TemporalQuantum: time.Hour,
	}

	tracker := NewBlockHealthTracker(config)

	// Test temporal quantization
	now := time.Now()
	hint := BlockHint{
		LastSeen: now,
	}

	tracker.UpdateBlockHealth("block1", hint)

	// Retrieve and check quantization
	if health, exists := tracker.blocks["block1"]; exists {
		// Should be rounded to hour
		if health.Hint.LastSeen.Minute() != 0 || health.Hint.LastSeen.Second() != 0 {
			t.Error("Time should be quantized to hour boundaries")
		}
	}

	// Test differential privacy adds noise
	trueValue := 10.0
	noisyValues := make([]float64, 100)

	for i := 0; i < 100; i++ {
		noisyValues[i] = tracker.AddDifferentialPrivacyNoise(trueValue)
	}

	// Check that values vary (noise is added)
	allSame := true
	for i := 1; i < len(noisyValues); i++ {
		if noisyValues[i] != noisyValues[0] {
			allSame = false
			break
		}
	}

	if allSame {
		t.Error("Differential privacy should add varying noise")
	}
}

func TestBlockHealthTracker_ReplicationBuckets(t *testing.T) {
	testCases := []struct {
		count    int
		expected ReplicationBucket
	}{
		{0, ReplicationLow},
		{1, ReplicationLow},
		{3, ReplicationLow},
		{4, ReplicationMedium},
		{10, ReplicationMedium},
		{11, ReplicationHigh},
		{100, ReplicationHigh},
	}

	for _, tc := range testCases {
		bucket := GetReplicationBucket(tc.count)
		if bucket != tc.expected {
			t.Errorf("Count %d: expected %v, got %v", tc.count, tc.expected, bucket)
		}
	}
}

func TestBlockHealthTracker_EntropyAnalysis(t *testing.T) {
	// High entropy data (random)
	randomData := make([]byte, 1024)
	for i := range randomData {
		randomData[i] = byte(i % 256)
	}

	if !AnalyzeBlockEntropy(randomData) {
		t.Error("Random data should have high entropy")
	}

	// Low entropy data (repeated pattern)
	lowEntropyData := make([]byte, 1024)
	for i := range lowEntropyData {
		lowEntropyData[i] = byte(i % 10) // Only 10 different values
	}

	if AnalyzeBlockEntropy(lowEntropyData) {
		t.Error("Repeated pattern should have low entropy")
	}
}

func TestBlockHealthTracker_Cleanup(t *testing.T) {
	config := &BlockHealthConfig{
		CleanupInterval: 100 * time.Millisecond, // Fast cleanup for testing
	}

	tracker := NewBlockHealthTracker(config)

	// Add an old block
	oldHint := BlockHint{
		ReplicationBucket: ReplicationLow,
	}
	tracker.UpdateBlockHealth("old-block", oldHint)

	// Manually set to old timestamp
	tracker.mu.Lock()
	if health, exists := tracker.blocks["old-block"]; exists {
		health.LastUpdated = time.Now().Add(-48 * time.Hour)
		health.LastRequested = time.Now().Add(-48 * time.Hour)
	}
	tracker.mu.Unlock()

	// Add a recent block
	tracker.UpdateBlockHealth("new-block", BlockHint{})

	// Wait for cleanup
	time.Sleep(200 * time.Millisecond)

	// Check that old block was removed
	tracker.mu.RLock()
	_, hasOld := tracker.blocks["old-block"]
	_, hasNew := tracker.blocks["new-block"]
	tracker.mu.RUnlock()

	if hasOld {
		t.Error("Old block should have been cleaned up")
	}
	if !hasNew {
		t.Error("New block should still exist")
	}
}

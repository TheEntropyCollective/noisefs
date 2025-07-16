package cache

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/bits-and-blooms/bloom/v3"
)

func TestHealthGossiper_MessageCreation(t *testing.T) {
	config := &HealthGossipConfig{
		GossipInterval:            1 * time.Minute,
		MinBlocksForGossip:        2,
		EnableDifferentialPrivacy: false, // Disable for deterministic testing
	}
	
	healthTracker := NewBlockHealthTracker(nil)
	gossiper, err := NewHealthGossiper(config, healthTracker, nil)
	if err != nil {
		t.Fatalf("Failed to create gossiper: %v", err)
	}
	
	// Add test blocks
	healthTracker.UpdateBlockHealth("block1", BlockHint{
		ReplicationBucket: ReplicationLow,
		HighEntropy:       true,
		NoisyRequestRate:      10,
		MissingRegions:    3,
	})
	healthTracker.UpdateBlockHealth("block2", BlockHint{
		ReplicationBucket: ReplicationMedium,
		HighEntropy:       false,
		NoisyRequestRate:      20,
		MissingRegions:    1,
	})
	healthTracker.UpdateBlockHealth("block3", BlockHint{
		ReplicationBucket: ReplicationHigh,
		HighEntropy:       true,
		NoisyRequestRate:      5,
	})
	
	// Test aggregate stats calculation
	hints := healthTracker.GetAllBlockHints()
	lowRepFilter := bloom.NewWithEstimates(1000, 0.01)
	highEntropyFilter := bloom.NewWithEstimates(1000, 0.01)
	
	stats := gossiper.calculateAggregateStats(hints, lowRepFilter, highEntropyFilter)
	
	// Verify stats
	if stats.TotalBlocks != 3 {
		t.Errorf("Expected 3 total blocks, got %d", stats.TotalBlocks)
	}
	
	if stats.LowReplicationCount != 1 {
		t.Errorf("Expected 1 low replication block, got %d", stats.LowReplicationCount)
	}
	
	if stats.MediumReplicationCount != 1 {
		t.Errorf("Expected 1 medium replication block, got %d", stats.MediumReplicationCount)
	}
	
	if stats.HighReplicationCount != 1 {
		t.Errorf("Expected 1 high replication block, got %d", stats.HighReplicationCount)
	}
	
	// Check average popularity
	expectedAvgPopularity := (10.0 + 20.0 + 5.0) / 3.0
	if stats.AveragePopularity != expectedAvgPopularity {
		t.Errorf("Expected average popularity %f, got %f", expectedAvgPopularity, stats.AveragePopularity)
	}
	
	// Check bloom filters
	if !lowRepFilter.TestString("block1") {
		t.Error("Low replication filter should contain block1")
	}
	
	if !highEntropyFilter.TestString("block1") || !highEntropyFilter.TestString("block3") {
		t.Error("High entropy filter should contain block1 and block3")
	}
}

func TestHealthGossiper_DifferentialPrivacy(t *testing.T) {
	config := &HealthGossipConfig{
		EnableDifferentialPrivacy: true,
		PrivacyEpsilon:           1.0,
		MinBlocksForGossip:       1,
	}
	
	healthTracker := NewBlockHealthTracker(nil)
	gossiper, err := NewHealthGossiper(config, healthTracker, nil)
	if err != nil {
		t.Fatalf("Failed to create gossiper: %v", err)
	}
	
	// Add test block
	healthTracker.UpdateBlockHealth("block1", BlockHint{
		ReplicationBucket: ReplicationLow,
		NoisyRequestRate:      100,
	})
	
	// Run multiple times to verify noise is added
	originalValue := int64(1) // Only one block with low replication
	variations := make(map[int64]int)
	
	for i := 0; i < 100; i++ {
		hints := healthTracker.GetAllBlockHints()
		stats := gossiper.calculateAggregateStats(
			hints,
			bloom.NewWithEstimates(1000, 0.01),
			bloom.NewWithEstimates(1000, 0.01),
		)
		
		// Track variations
		variations[stats.LowReplicationCount]++
	}
	
	// Should see multiple different values due to noise
	if len(variations) < 5 {
		t.Error("Differential privacy should add more variation to values")
	}
	
	// Most values should be near the original
	nearOriginal := 0
	for value, count := range variations {
		if value >= originalValue-3 && value <= originalValue+3 {
			nearOriginal += count
		}
	}
	
	if nearOriginal < 70 {
		t.Error("Most noisy values should be near the original value")
	}
}

func TestHealthGossiper_MessageProcessing(t *testing.T) {
	config := DefaultHealthGossipConfig()
	healthTracker := NewBlockHealthTracker(nil)
	gossiper, err := NewHealthGossiper(config, healthTracker, nil)
	if err != nil {
		t.Fatalf("Failed to create gossiper: %v", err)
	}
	
	// Create a test message
	testStats := &AggregateHealthStats{
		TotalBlocks:          100,
		LowReplicationCount:  20,
		MediumReplicationCount: 50,
		HighReplicationCount: 30,
		AveragePopularity:    25.5,
		AverageEntropy:       0.6,
		RegionCounts: map[string]int64{
			"americas": 40,
			"europe":   30,
			"asia":     30,
		},
	}
	
	// Create bloom filters
	lowRepFilter := bloom.NewWithEstimates(1000, 0.01)
	lowRepFilter.AddString("test-block-1")
	lowRepFilter.AddString("test-block-2")
	
	highEntropyFilter := bloom.NewWithEstimates(1000, 0.01)
	highEntropyFilter.AddString("test-block-3")
	
	lowRepData, _ := lowRepFilter.MarshalBinary()
	highEntropyData, _ := highEntropyFilter.MarshalBinary()
	
	msg := &HealthGossipMessage{
		Timestamp:            time.Now(),
		LowReplicationFilter: lowRepData,
		HighEntropyFilter:    highEntropyData,
		AggregateStats:       testStats,
		PeerID:              "test-peer-123",
		Version:             1,
	}
	
	// Serialize and process
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal message: %v", err)
	}
	
	err = gossiper.processGossip(data)
	if err != nil {
		t.Fatalf("Failed to process gossip: %v", err)
	}
	
	// Verify peer estimate was stored
	gossiper.mu.RLock()
	estimate, exists := gossiper.peerHealthEstimates[msg.PeerID]
	gossiper.mu.RUnlock()
	
	if !exists {
		t.Fatal("Peer estimate should be stored")
	}
	
	if estimate.AggregateStats.TotalBlocks != testStats.TotalBlocks {
		t.Errorf("Expected total blocks %d, got %d",
			testStats.TotalBlocks,
			estimate.AggregateStats.TotalBlocks)
	}
	
	// Verify bloom filters were unmarshaled
	if !estimate.LowReplicationBlocks.TestString("test-block-1") {
		t.Error("Low replication blocks filter not properly unmarshaled")
	}
	
	if !estimate.HighEntropyBlocks.TestString("test-block-3") {
		t.Error("High entropy blocks filter not properly unmarshaled")
	}
}

func TestHealthGossiper_OldMessageRejection(t *testing.T) {
	config := DefaultHealthGossipConfig()
	healthTracker := NewBlockHealthTracker(nil)
	gossiper, err := NewHealthGossiper(config, healthTracker, nil)
	if err != nil {
		t.Fatalf("Failed to create gossiper: %v", err)
	}
	
	// Create an old message
	msg := &HealthGossipMessage{
		Timestamp: time.Now().Add(-15 * time.Minute), // Too old
		PeerID:    "old-peer",
		Version:   1,
		AggregateStats: &AggregateHealthStats{
			TotalBlocks: 50,
		},
	}
	
	data, _ := json.Marshal(msg)
	err = gossiper.processGossip(data)
	
	// Should reject old message
	if err == nil {
		t.Error("Should reject messages older than 10 minutes")
	}
}

func TestHealthGossiper_NetworkHealthEstimate(t *testing.T) {
	config := &HealthGossipConfig{
		AggregationWindow: 10 * time.Minute,
	}
	
	healthTracker := NewBlockHealthTracker(nil)
	gossiper, err := NewHealthGossiper(config, healthTracker, nil)
	if err != nil {
		t.Fatalf("Failed to create gossiper: %v", err)
	}
	
	// Add peer estimates
	now := time.Now()
	gossiper.mu.Lock()
	gossiper.peerHealthEstimates["peer1"] = &PeerHealthEstimate{
		LastUpdate: now,
		AggregateStats: &AggregateHealthStats{
			TotalBlocks:         100,
			LowReplicationCount: 20,
		},
	}
	gossiper.peerHealthEstimates["peer2"] = &PeerHealthEstimate{
		LastUpdate: now.Add(-5 * time.Minute), // Recent
		AggregateStats: &AggregateHealthStats{
			TotalBlocks:         150,
			LowReplicationCount: 30,
		},
	}
	gossiper.peerHealthEstimates["peer3"] = &PeerHealthEstimate{
		LastUpdate: now.Add(-20 * time.Minute), // Too old
		AggregateStats: &AggregateHealthStats{
			TotalBlocks:         200,
			LowReplicationCount: 40,
		},
	}
	gossiper.mu.Unlock()
	
	// Get network estimate
	estimate := gossiper.GetNetworkHealthEstimate()
	
	// Should only include recent peers
	if estimate.PeerCount != 3 {
		t.Errorf("Expected 3 total peers, got %d", estimate.PeerCount)
	}
	
	// Should aggregate only recent peer stats
	expectedTotal := int64(100 + 150) // peer3 is too old
	if estimate.TotalNetworkBlocks != expectedTotal {
		t.Errorf("Expected total blocks %d, got %d", expectedTotal, estimate.TotalNetworkBlocks)
	}
	
	expectedLowRep := int64(20 + 30)
	if estimate.LowReplicationBlocks != expectedLowRep {
		t.Errorf("Expected low replication blocks %d, got %d", expectedLowRep, estimate.LowReplicationBlocks)
	}
}

func TestHealthGossiper_RegionAnonymization(t *testing.T) {
	config := DefaultHealthGossipConfig()
	healthTracker := NewBlockHealthTracker(nil)
	gossiper, err := NewHealthGossiper(config, healthTracker, nil)
	if err != nil {
		t.Fatalf("Failed to create gossiper: %v", err)
	}
	
	// Test region mapping
	testCases := []struct {
		region   int
		expected string
	}{
		{0, "americas"},
		{1, "americas"},
		{2, "americas"},
		{3, "europe"},
		{4, "europe"},
		{5, "europe"},
		{6, "asia"},
		{7, "asia"},
		{8, "asia"},
		{9, "other"},
		{100, "other"},
	}
	
	for _, tc := range testCases {
		result := gossiper.anonymizeRegion(tc.region)
		if result != tc.expected {
			t.Errorf("Region %d: expected %s, got %s", tc.region, tc.expected, result)
		}
	}
}

func TestHealthGossiper_StartStop(t *testing.T) {
	config := &HealthGossipConfig{
		GossipInterval:     100 * time.Millisecond,
		MinBlocksForGossip: 0, // Allow gossip even with no blocks
	}
	
	healthTracker := NewBlockHealthTracker(nil)
	gossiper, err := NewHealthGossiper(config, healthTracker, nil)
	if err != nil {
		t.Fatalf("Failed to create gossiper: %v", err)
	}
	
	// Start gossiper
	err = gossiper.Start()
	if err != nil {
		t.Fatalf("Failed to start gossiper: %v", err)
	}
	
	// Let it run briefly
	time.Sleep(50 * time.Millisecond)
	
	// Stop gossiper
	gossiper.Stop()
	
	// Verify it stopped
	select {
	case <-gossiper.ctx.Done():
		// Good, context was cancelled
	default:
		t.Error("Gossiper context should be cancelled after Stop()")
	}
}

func TestLaplaceNoise_Properties(t *testing.T) {
	// Test with different epsilon values
	epsilons := []float64{0.1, 1.0, 10.0}
	
	for _, epsilon := range epsilons {
		noise := NewLaplaceNoise(epsilon)
		
		// Generate many samples
		samples := make([]float64, 10000)
		for i := range samples {
			samples[i] = noise.generateLaplace()
		}
		
		// Calculate mean and variance
		var sum, sumSquared float64
		for _, s := range samples {
			sum += s
			sumSquared += s * s
		}
		
		mean := sum / float64(len(samples))
		variance := sumSquared/float64(len(samples)) - mean*mean
		
		// Mean should be near 0
		if mean > 0.1 || mean < -0.1 {
			t.Errorf("Epsilon %f: Mean %f too far from 0", epsilon, mean)
		}
		
		// Variance should be approximately 2 * (scale^2) = 2 * (1/epsilon)^2
		expectedVariance := 2.0 / (epsilon * epsilon)
		relativeError := (variance - expectedVariance) / expectedVariance
		
		if relativeError > 0.1 || relativeError < -0.1 {
			t.Errorf("Epsilon %f: Variance %f differs from expected %f by %f%%",
				epsilon, variance, expectedVariance, relativeError*100)
		}
	}
}
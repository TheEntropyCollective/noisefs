package cache

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/bits-and-blooms/bloom/v3"
)

func TestHealthGossiper_BasicOperation(t *testing.T) {
	// Create mock components
	healthTracker := NewBlockHealthTracker(nil)

	// Add some test data
	healthTracker.UpdateBlockHealth("block1", BlockHint{
		ReplicationBucket: ReplicationLow,
		HighEntropy:       true,
		NoisyRequestRate:  10,
	})
	healthTracker.UpdateBlockHealth("block2", BlockHint{
		ReplicationBucket: ReplicationHigh,
		HighEntropy:       false,
		NoisyRequestRate:  5,
	})

	// Create gossiper
	config := &HealthGossipConfig{
		GossipInterval:            1 * time.Second,
		MinBlocksForGossip:        2,
		EnableDifferentialPrivacy: true,
		PrivacyEpsilon:            1.0,
	}

	gossiper, err := NewHealthGossiper(config, healthTracker, nil)
	if err != nil {
		t.Fatalf("Failed to create gossiper: %v", err)
	}

	// Test message creation
	hints := healthTracker.GetAllBlockHints()
	lowRepFilter := bloom.NewWithEstimates(1000, 0.01)
	highEntropyFilter := bloom.NewWithEstimates(1000, 0.01)

	stats := gossiper.calculateAggregateStats(hints, lowRepFilter, highEntropyFilter)

	// Verify stats (with tolerance for differential privacy noise)
	if stats.TotalBlocks < 1 || stats.TotalBlocks > 4 {
		t.Errorf("Expected 2 total blocks (±noise), got %d", stats.TotalBlocks)
	}

	// With differential privacy, we can't expect exact values
	// Just ensure we have some low replication blocks tracked
	if stats.LowReplicationCount < 0 || stats.LowReplicationCount > 3 {
		t.Errorf("Expected ~1 low replication block (±noise), got %d", stats.LowReplicationCount)
	}

	// Test differential privacy
	if gossiper.config.EnableDifferentialPrivacy {
		// Stats should have noise added
		// We can't test exact values due to randomness
		if stats.AveragePopularity == 7.5 {
			t.Error("Average popularity should have noise added")
		}
	}
}

func TestBloomExchanger_CoordinationHints(t *testing.T) {
	// Create test cache
	baseCache := NewMemoryCache(100)
	config := &AltruisticCacheConfig{
		MinPersonalCache: 50 * 1024,
		EnableAltruistic: true,
	}
	cache := NewAltruisticCache(baseCache, config, 100*1024)

	// Create bloom exchanger
	exchangeConfig := &BloomExchangeConfig{
		ExchangeInterval: 1 * time.Second,
		FilterCategories: map[string]*BloomFilterParams{
			"valuable_blocks": {
				Size:          1000,
				HashFunctions: 5,
				FalsePositive: 0.01,
			},
		},
		MinPeersForCoordination: 2,
		CoordinationThreshold:   0.5,
	}

	exchanger, err := NewBloomExchanger(exchangeConfig, cache, nil)
	if err != nil {
		t.Fatalf("Failed to create exchanger: %v", err)
	}

	// Update local filters
	exchanger.UpdateLocalFilters()

	// Create mock peer filters
	peerFilters := make(map[string]*PeerFilterSet)

	// Add peer 1
	peer1Filter := bloom.NewWithEstimates(1000, 0.01)
	peer1Filter.AddString("block1")
	peer1Filter.AddString("block2")

	peerFilters["peer1"] = &PeerFilterSet{
		PeerID:     "peer1",
		LastUpdate: time.Now(),
		Filters: map[string]*bloom.BloomFilter{
			"valuable_blocks": peer1Filter,
		},
		Hints: &CoordinationHints{
			HighDemandBlocks: []string{"block1", "block2"},
		},
	}

	// Add peer 2
	peer2Filter := bloom.NewWithEstimates(1000, 0.01)
	peer2Filter.AddString("block1")
	peer2Filter.AddString("block3")

	peerFilters["peer2"] = &PeerFilterSet{
		PeerID:     "peer2",
		LastUpdate: time.Now(),
		Filters: map[string]*bloom.BloomFilter{
			"valuable_blocks": peer2Filter,
		},
		Hints: &CoordinationHints{
			HighDemandBlocks: []string{"block1", "block3"},
		},
	}

	// Generate coordination hints
	hints := exchanger.coordinationEngine.GenerateHints(
		exchanger.localFilters,
		peerFilters,
	)

	// Verify high demand blocks
	if len(hints.HighDemandBlocks) == 0 {
		t.Error("Expected high demand blocks")
	}

	// block1 should be identified as high demand (wanted by both peers)
	found := false
	for _, blockID := range hints.HighDemandBlocks {
		if blockID == "block1" {
			found = true
			break
		}
	}

	if !found {
		t.Error("block1 should be identified as high demand")
	}

	// Should have coordination score
	if hints.CoordinationScore == 0 {
		t.Error("Expected non-zero coordination score")
	}
}

func TestCoordinationEngine_BlockAssignment(t *testing.T) {
	config := DefaultBloomExchangeConfig()
	engine := NewCoordinationEngine(config)

	// Test block affinity calculation
	blockID := "test-block"
	peer1ID := "peer1"
	peer2ID := "peer2"

	affinity1 := engine.calculateBlockAffinity(blockID, peer1ID)
	affinity2 := engine.calculateBlockAffinity(blockID, peer2ID)

	// Affinities should be different for different peers
	if affinity1 == affinity2 {
		t.Error("Expected different affinities for different peers")
	}

	// Affinities should be between 0 and 1
	if affinity1 < 0 || affinity1 > 1 {
		t.Errorf("Invalid affinity1: %f", affinity1)
	}
	if affinity2 < 0 || affinity2 > 1 {
		t.Errorf("Invalid affinity2: %f", affinity2)
	}

	// Test assignment tracking
	engine.UpdateAssignments("peer1", []string{"block1", "block2"})
	engine.UpdateAssignments("peer2", []string{"block2", "block3"})

	// Check block assignments
	block2Peers := engine.GetBlockAssignments("block2")
	if len(block2Peers) != 2 {
		t.Errorf("Expected 2 peers for block2, got %d", len(block2Peers))
	}

	// Check peer assignments
	peer1Blocks := engine.GetPeerAssignments("peer1")
	if len(peer1Blocks) != 2 {
		t.Errorf("Expected 2 blocks for peer1, got %d", len(peer1Blocks))
	}
}

func TestNetworkHealthManager_Integration(t *testing.T) {
	// Create test cache
	baseCache := NewMemoryCache(100)
	config := &AltruisticCacheConfig{
		MinPersonalCache: 50 * 1024,
		EnableAltruistic: true,
	}
	cache := NewAltruisticCache(baseCache, config, 100*1024)

	// Add some test blocks
	for i := 0; i < 5; i++ {
		data := make([]byte, 1024)
		block := &blocks.Block{Data: data}
		cache.StoreWithOrigin(string(rune('a'+i)), block, AltruisticBlock)

		// Update health info
		cache.UpdateBlockHealth(string(rune('a'+i)), BlockHint{
			ReplicationBucket: ReplicationBucket(i % 3),
			HighEntropy:       i%2 == 0,
			NoisyRequestRate:  float64(i * 10),
		})
	}

	// Create network health manager
	nhConfig := &NetworkHealthConfig{
		EnableGossip:        true,
		EnableBloomExchange: true,
	}

	manager, err := NewNetworkHealthManager(cache, nil, nhConfig)
	if err != nil {
		t.Fatalf("Failed to create network health manager: %v", err)
	}

	// Get network health report
	report := manager.GetNetworkHealth()

	// Verify report structure
	if report.Timestamp.IsZero() {
		t.Error("Expected timestamp in report")
	}

	if report.LocalHealth == nil {
		t.Error("Expected local health in report")
	}

	if report.LocalHealth.TrackedBlocks != 5 {
		t.Errorf("Expected 5 tracked blocks, got %d", report.LocalHealth.TrackedBlocks)
	}

	// Test overall score calculation
	if report.OverallScore < 0 || report.OverallScore > 1 {
		t.Errorf("Invalid overall score: %f", report.OverallScore)
	}
}

func TestLaplaceNoise_Distribution(t *testing.T) {
	noise := NewLaplaceNoise(1.0) // epsilon = 1.0

	// Generate many samples
	samples := make([]float64, 1000)
	for i := range samples {
		samples[i] = noise.generateLaplace()
	}

	// Calculate mean (should be close to 0)
	var sum float64
	for _, s := range samples {
		sum += s
	}
	mean := sum / float64(len(samples))

	// Mean should be close to 0
	if mean > 1.0 || mean < -1.0 {
		t.Errorf("Mean too far from 0: %f", mean)
	}

	// Test noise addition
	original := int64(100)
	noisy := noise.AddNoiseInt64(original)

	// Value should be different (with high probability)
	if noisy == original {
		// There's a small chance they could be equal
		// Run multiple times to be sure
		allEqual := true
		for i := 0; i < 10; i++ {
			if noise.AddNoiseInt64(original) != original {
				allEqual = false
				break
			}
		}
		if allEqual {
			t.Error("Noise generator not adding noise")
		}
	}
}

func TestHealthGossipMessage_Serialization(t *testing.T) {
	// Create a test message
	msg := &HealthGossipMessage{
		Timestamp: time.Now(),
		PeerID:    "test-peer",
		Version:   1,
		AggregateStats: &AggregateHealthStats{
			TotalBlocks:          100,
			LowReplicationCount:  10,
			HighReplicationCount: 50,
			AveragePopularity:    25.5,
			RegionCounts: map[string]int64{
				"americas": 30,
				"europe":   40,
				"asia":     30,
			},
		},
	}

	// Create bloom filters
	lowRepFilter := bloom.NewWithEstimates(1000, 0.01)
	lowRepFilter.AddString("block1")
	lowRepFilter.AddString("block2")

	highEntropyFilter := bloom.NewWithEstimates(1000, 0.01)
	highEntropyFilter.AddString("block3")

	msg.LowReplicationFilter, _ = lowRepFilter.MarshalBinary()
	msg.HighEntropyFilter, _ = highEntropyFilter.MarshalBinary()

	// Serialize
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal message: %v", err)
	}

	// Deserialize
	var decoded HealthGossipMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal message: %v", err)
	}

	// Verify fields
	if decoded.Version != msg.Version {
		t.Errorf("Version mismatch: got %d, want %d", decoded.Version, msg.Version)
	}

	if decoded.AggregateStats.TotalBlocks != msg.AggregateStats.TotalBlocks {
		t.Errorf("TotalBlocks mismatch: got %d, want %d",
			decoded.AggregateStats.TotalBlocks,
			msg.AggregateStats.TotalBlocks)
	}

	// Verify bloom filters
	decodedLowRep := &bloom.BloomFilter{}
	if err := decodedLowRep.UnmarshalBinary(decoded.LowReplicationFilter); err != nil {
		t.Fatalf("Failed to unmarshal low rep filter: %v", err)
	}

	// Check if filter still contains test data
	if !decodedLowRep.TestString("block1") {
		t.Error("Decoded filter missing block1")
	}
}

func TestNetworkHealthIntegration_FullFlow(t *testing.T) {
	// Skip if no IPFS shell available
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create test environment
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create cache with network health
	baseCache := NewMemoryCache(100)
	config := &AltruisticCacheConfig{
		MinPersonalCache: 50 * 1024,
		EnableAltruistic: true,
	}
	cache := NewAltruisticCache(baseCache, config, 100*1024)

	// Create network health manager
	nhConfig := DefaultNetworkHealthConfig()
	nhConfig.GossipConfig.GossipInterval = 1 * time.Second
	nhConfig.BloomExchangeConfig.ExchangeInterval = 1 * time.Second

	manager, err := NewNetworkHealthManager(cache, nil, nhConfig)
	if err != nil {
		t.Fatalf("Failed to create network health manager: %v", err)
	}

	// Note: Starting would fail without actual IPFS shell
	// This test mainly verifies the integration compiles and basic operations work

	// Test peer discovery integration
	err = manager.IntegrateWithPeerDiscovery(ctx)
	if err != nil {
		t.Errorf("Failed to integrate with peer discovery: %v", err)
	}

	// Add test data
	for i := 0; i < 10; i++ {
		data := make([]byte, 1024)
		block := &blocks.Block{Data: data}
		cache.StoreWithOrigin(string(rune('a'+i)), block, AltruisticBlock)
	}

	// Update network health
	manager.UpdateFromNetworkHealth()

	// Get health report
	report := manager.GetNetworkHealth()
	if report == nil {
		t.Fatal("Expected health report")
	}

	// Verify local health tracking
	if report.LocalHealth.TrackedBlocks != 10 {
		t.Errorf("Expected 10 tracked blocks, got %d", report.LocalHealth.TrackedBlocks)
	}
}

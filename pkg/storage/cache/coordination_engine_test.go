package cache

import (
	"testing"

	"github.com/bits-and-blooms/bloom/v3"
)

func TestCoordinationEngine_BlockAffinity(t *testing.T) {
	config := DefaultBloomExchangeConfig()
	engine := NewCoordinationEngine(config)

	// Test that same block has different affinity with different peers
	blockID := "test-block-123"
	peer1 := "peer-alice"
	peer2 := "peer-bob"

	affinity1 := engine.calculateBlockAffinity(blockID, peer1)
	affinity2 := engine.calculateBlockAffinity(blockID, peer2)

	// Affinities should be different
	if affinity1 == affinity2 {
		t.Error("Same block should have different affinities with different peers")
	}

	// Affinities should be in valid range
	if affinity1 < 0 || affinity1 > 1 {
		t.Errorf("Affinity1 out of range: %f", affinity1)
	}
	if affinity2 < 0 || affinity2 > 1 {
		t.Errorf("Affinity2 out of range: %f", affinity2)
	}

	// Test consistency - same inputs should produce same output
	affinity1Again := engine.calculateBlockAffinity(blockID, peer1)
	if affinity1 != affinity1Again {
		t.Error("Affinity calculation should be deterministic")
	}
}

func TestCoordinationEngine_HighDemandBlocks(t *testing.T) {
	config := DefaultBloomExchangeConfig()
	engine := NewCoordinationEngine(config)

	// Create block demand data
	blockPeers := map[string][]string{
		"popular-block":   {"peer1", "peer2", "peer3", "peer4"},          // 80% want it
		"medium-block":    {"peer1", "peer2"},                            // 40% want it
		"unpopular-block": {"peer1"},                                     // 20% want it
		"very-popular":    {"peer1", "peer2", "peer3", "peer4", "peer5"}, // 100% want it
	}

	totalPeers := 5
	highDemand := engine.findHighDemandBlocks(blockPeers, totalPeers)

	// Should include blocks wanted by >= 30% of peers
	expectedBlocks := map[string]bool{
		"popular-block": true,
		"medium-block":  true,
		"very-popular":  true,
	}

	if len(highDemand) != 3 {
		t.Errorf("Expected 3 high demand blocks, got %d", len(highDemand))
	}

	for _, block := range highDemand {
		if !expectedBlocks[block] {
			t.Errorf("Unexpected block in high demand: %s", block)
		}
		delete(expectedBlocks, block)
	}

	if len(expectedBlocks) > 0 {
		t.Error("Some expected blocks were not found in high demand list")
	}

	// Verify ordering (most popular first)
	if len(highDemand) > 0 && highDemand[0] != "very-popular" {
		t.Error("Most popular block should be first")
	}
}

func TestCoordinationEngine_BlockSuggestions(t *testing.T) {
	config := DefaultBloomExchangeConfig()
	config.MinPeersForCoordination = 2
	engine := NewCoordinationEngine(config)

	// Create local filters
	localFilters := map[string]*bloom.BloomFilter{
		"valuable_blocks": bloom.NewWithEstimates(1000, 0.01),
	}

	// Create peer filters
	peerFilters := map[string]*PeerFilterSet{
		"peer1": {
			PeerID: "peer1",
			Hints: &CoordinationHints{
				HighDemandBlocks: []string{"block1", "block2", "block3"},
			},
		},
		"peer2": {
			PeerID: "peer2",
			Hints: &CoordinationHints{
				HighDemandBlocks: []string{"block2", "block3", "block4"},
			},
		},
	}

	// Create valuable blocks map
	valuableBlocks := map[string][]string{
		"block1": {"peer1"},
		"block2": {"peer1", "peer2"},
		"block3": {"peer1", "peer2"},
		"block4": {"peer2"},
	}

	suggestions := engine.generateBlockSuggestions(localFilters, peerFilters, valuableBlocks)

	// Should return suggestions
	if len(suggestions) == 0 {
		t.Error("Should generate block suggestions")
	}

	// Should not exceed limit
	if len(suggestions) > 20 {
		t.Errorf("Too many suggestions: %d", len(suggestions))
	}

	// Suggestions should be from valuable blocks
	suggestedMap := make(map[string]bool)
	for _, block := range suggestions {
		suggestedMap[block] = true
	}

	for block := range suggestedMap {
		if _, exists := valuableBlocks[block]; !exists {
			t.Errorf("Suggested block %s not in valuable blocks", block)
		}
	}
}

func TestCoordinationEngine_CoordinationScore(t *testing.T) {
	config := DefaultBloomExchangeConfig()
	config.MinPeersForCoordination = 2
	engine := NewCoordinationEngine(config)

	// Test with too few peers
	fewPeers := map[string]*PeerFilterSet{
		"peer1": {
			Filters: map[string]*bloom.BloomFilter{
				"valuable_blocks": bloom.NewWithEstimates(1000, 0.01),
			},
		},
	}

	score := engine.calculateCoordinationScore(fewPeers)
	if score != 0.0 {
		t.Errorf("Expected 0 score with too few peers, got %f", score)
	}

	// Test with good coordination (moderate overlap)
	filter1 := bloom.NewWithEstimates(1000, 0.01)
	filter2 := bloom.NewWithEstimates(1000, 0.01)

	// Add some common elements for moderate overlap
	for i := 0; i < 30; i++ {
		filter1.AddString(string(rune('a' + i)))
		if i < 15 {
			filter2.AddString(string(rune('a' + i)))
		}
	}

	goodPeers := map[string]*PeerFilterSet{
		"peer1": {
			Filters: map[string]*bloom.BloomFilter{
				"valuable_blocks": filter1,
			},
		},
		"peer2": {
			Filters: map[string]*bloom.BloomFilter{
				"valuable_blocks": filter2,
			},
		},
	}

	score = engine.calculateCoordinationScore(goodPeers)

	// Score should be between 0 and 1
	if score < 0 || score > 1 {
		t.Errorf("Score out of range: %f", score)
	}

	// With moderate overlap, score should be reasonable
	if score < 0.3 {
		t.Error("Score too low for moderate overlap")
	}
}

func TestCoordinationEngine_AssignmentTracking(t *testing.T) {
	config := DefaultBloomExchangeConfig()
	engine := NewCoordinationEngine(config)

	// Test assignment updates
	peer1 := "peer1"
	blocks1 := []string{"block1", "block2", "block3"}

	engine.UpdateAssignments(peer1, blocks1)

	// Verify peer assignments
	peerBlocks := engine.GetPeerAssignments(peer1)
	if len(peerBlocks) != len(blocks1) {
		t.Errorf("Expected %d blocks for peer1, got %d", len(blocks1), len(peerBlocks))
	}

	// Verify block assignments
	for _, block := range blocks1 {
		peers := engine.GetBlockAssignments(block)
		if len(peers) != 1 || peers[0] != peer1 {
			t.Errorf("Block %s should be assigned to peer1", block)
		}
	}

	// Update with new assignments (should replace old ones)
	blocks2 := []string{"block3", "block4", "block5"}
	engine.UpdateAssignments(peer1, blocks2)

	// Old blocks should no longer be assigned to peer1
	peers := engine.GetBlockAssignments("block1")
	if len(peers) != 0 {
		t.Error("block1 should have no assignments after update")
	}

	// New blocks should be assigned
	peers = engine.GetBlockAssignments("block4")
	if len(peers) != 1 || peers[0] != peer1 {
		t.Error("block4 should be assigned to peer1")
	}

	// block3 should still be assigned (in both lists)
	peers = engine.GetBlockAssignments("block3")
	if len(peers) != 1 || peers[0] != peer1 {
		t.Error("block3 should still be assigned to peer1")
	}
}

func TestCoordinationEngine_MultiPeerAssignments(t *testing.T) {
	config := DefaultBloomExchangeConfig()
	engine := NewCoordinationEngine(config)

	// Assign same block to multiple peers
	engine.UpdateAssignments("peer1", []string{"blockA", "blockB"})
	engine.UpdateAssignments("peer2", []string{"blockB", "blockC"})
	engine.UpdateAssignments("peer3", []string{"blockA", "blockC"})

	// Check block assignments
	peersA := engine.GetBlockAssignments("blockA")
	if len(peersA) != 2 {
		t.Errorf("blockA should be assigned to 2 peers, got %d", len(peersA))
	}

	peersB := engine.GetBlockAssignments("blockB")
	if len(peersB) != 2 {
		t.Errorf("blockB should be assigned to 2 peers, got %d", len(peersB))
	}

	peersC := engine.GetBlockAssignments("blockC")
	if len(peersC) != 2 {
		t.Errorf("blockC should be assigned to 2 peers, got %d", len(peersC))
	}

	// Get metrics
	metrics := engine.GetCoordinationMetrics()

	if metrics.TotalTrackedBlocks != 3 {
		t.Errorf("Expected 3 tracked blocks, got %d", metrics.TotalTrackedBlocks)
	}

	// Average coverage should be 2.0 (each block has 2 peers)
	if metrics.AverageBlockCoverage != 2.0 {
		t.Errorf("Expected average coverage 2.0, got %f", metrics.AverageBlockCoverage)
	}
}

func TestCoordinationEngine_GenerateHints(t *testing.T) {
	config := DefaultBloomExchangeConfig()
	config.MinPeersForCoordination = 2
	engine := NewCoordinationEngine(config)

	// Create local filters
	localFilters := map[string]*bloom.BloomFilter{
		"valuable_blocks": bloom.NewWithEstimates(1000, 0.01),
	}

	// Test with too few peers
	fewPeers := map[string]*PeerFilterSet{
		"peer1": {
			PeerID: "peer1",
			Filters: map[string]*bloom.BloomFilter{
				"valuable_blocks": bloom.NewWithEstimates(1000, 0.01),
			},
		},
	}

	hints := engine.GenerateHints(localFilters, fewPeers)

	// Should return empty hints with too few peers
	if len(hints.HighDemandBlocks) != 0 || len(hints.SuggestedBlocks) != 0 {
		t.Error("Should not generate hints with too few peers")
	}

	if hints.CoordinationScore != 0.0 {
		t.Error("Coordination score should be 0 with too few peers")
	}

	// Test with enough peers
	filter1 := bloom.NewWithEstimates(1000, 0.01)
	filter2 := bloom.NewWithEstimates(1000, 0.01)
	filter3 := bloom.NewWithEstimates(1000, 0.01)

	// Add some blocks to create overlap
	filter1.AddString("block1")
	filter1.AddString("block2")
	filter1.AddString("common1")

	filter2.AddString("block2")
	filter2.AddString("block3")
	filter2.AddString("common1")

	filter3.AddString("block1")
	filter3.AddString("block3")
	filter3.AddString("common2")

	peerFilters := map[string]*PeerFilterSet{
		"peer1": {
			PeerID: "peer1",
			Filters: map[string]*bloom.BloomFilter{
				"valuable_blocks": filter1,
			},
			Hints: &CoordinationHints{
				HighDemandBlocks: []string{"block1", "block2"},
			},
		},
		"peer2": {
			PeerID: "peer2",
			Filters: map[string]*bloom.BloomFilter{
				"valuable_blocks": filter2,
			},
			Hints: &CoordinationHints{
				HighDemandBlocks: []string{"block2", "block3"},
			},
		},
		"peer3": {
			PeerID: "peer3",
			Filters: map[string]*bloom.BloomFilter{
				"valuable_blocks": filter3,
			},
			Hints: &CoordinationHints{
				HighDemandBlocks: []string{"block1", "block3"},
			},
		},
	}

	hints = engine.GenerateHints(localFilters, peerFilters)

	// Should identify high demand blocks
	if len(hints.HighDemandBlocks) == 0 {
		t.Error("Should identify high demand blocks")
	}

	// Should generate suggestions
	if len(hints.SuggestedBlocks) == 0 {
		t.Error("Should generate block suggestions")
	}

	// Should calculate coordination score
	if hints.CoordinationScore == 0.0 {
		t.Errorf("Should calculate non-zero coordination score, got %f", hints.CoordinationScore)
	}
}

func TestCoordinationEngine_EstimateFilterOverlap(t *testing.T) {
	engine := NewCoordinationEngine(DefaultBloomExchangeConfig())

	// Test with identical filters
	filter1 := bloom.NewWithEstimates(1000, 0.01)
	for i := 0; i < 50; i++ {
		filter1.AddString(string(rune('a' + i)))
	}

	overlap := engine.estimateFilterOverlap(filter1, filter1)

	// Same filter should have high overlap
	if overlap < 0.9 {
		t.Errorf("Identical filters should have high overlap, got %f", overlap)
	}

	// Test with no overlap
	filter2 := bloom.NewWithEstimates(1000, 0.01)
	emptyFilter := bloom.NewWithEstimates(1000, 0.01)

	for i := 0; i < 50; i++ {
		filter2.AddString(string(rune('A' + i)))
	}

	noOverlap := engine.estimateFilterOverlap(filter2, emptyFilter)
	if noOverlap != 0 {
		t.Errorf("Filter and empty filter should have 0 overlap, got %f", noOverlap)
	}

	// Test with different sizes
	smallFilter := bloom.NewWithEstimates(100, 0.01)
	largeFilter := bloom.NewWithEstimates(10000, 0.01)

	for i := 0; i < 10; i++ {
		smallFilter.AddString(string(rune('a' + i)))
		largeFilter.AddString(string(rune('a' + i)))
	}

	sizeOverlap := engine.estimateFilterOverlap(smallFilter, largeFilter)

	// Should account for size difference
	if sizeOverlap > 0.5 {
		t.Error("Overlap should be reduced due to size difference")
	}
}

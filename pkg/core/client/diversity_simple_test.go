package noisefs

import (
	"testing"

	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
)

func TestScoredCandidate_Type(t *testing.T) {
	// Test the scoredCandidate type
	candidate := scoredCandidate{
		block: &cache.BlockInfo{
			CID:        "test-cid",
			Size:       1024,
			Popularity: 5,
		},
		score: 0.75,
	}

	if candidate.block.CID != "test-cid" {
		t.Errorf("Expected CID 'test-cid', got %s", candidate.block.CID)
	}

	if candidate.score != 0.75 {
		t.Errorf("Expected score 0.75, got %f", candidate.score)
	}
}

func TestClient_WeightedRandomSelection_Simple(t *testing.T) {
	// Create a minimal client for testing
	client := &Client{
		diversityControls: cache.NewRandomizerDiversityControls(nil),
	}

	// Test weighted selection with simple candidates
	candidates := []scoredCandidate{
		{
			block: &cache.BlockInfo{CID: "low", Size: 1024, Popularity: 1},
			score: 0.1,
		},
		{
			block: &cache.BlockInfo{CID: "high", Size: 1024, Popularity: 3},
			score: 0.9,
		},
	}

	// Test that selection works
	selected, err := client.weightedRandomSelection(candidates)
	if err != nil {
		t.Fatalf("Failed to perform weighted selection: %v", err)
	}

	if selected == nil {
		t.Fatal("Should have selected a candidate")
	}

	if selected.CID != "low" && selected.CID != "high" {
		t.Errorf("Selected CID should be 'low' or 'high', got %s", selected.CID)
	}
}

func TestClient_WeightedRandomSelection_ZeroScores(t *testing.T) {
	// Create a minimal client for testing
	client := &Client{
		diversityControls: cache.NewRandomizerDiversityControls(nil),
	}

	// Test with zero scores (should use uniform random)
	candidates := []scoredCandidate{
		{
			block: &cache.BlockInfo{CID: "zero1", Size: 1024, Popularity: 1},
			score: 0.0,
		},
		{
			block: &cache.BlockInfo{CID: "zero2", Size: 1024, Popularity: 2},
			score: 0.0,
		},
	}

	// Test that selection works even with zero scores
	selected, err := client.weightedRandomSelection(candidates)
	if err != nil {
		t.Fatalf("Failed to perform weighted selection: %v", err)
	}

	if selected == nil {
		t.Fatal("Should have selected a candidate")
	}

	if selected.CID != "zero1" && selected.CID != "zero2" {
		t.Errorf("Selected CID should be 'zero1' or 'zero2', got %s", selected.CID)
	}
}

func TestClient_WeightedRandomSelection_Empty(t *testing.T) {
	// Create a minimal client for testing
	client := &Client{
		diversityControls: cache.NewRandomizerDiversityControls(nil),
	}

	// Test with empty candidates
	candidates := []scoredCandidate{}

	// Should return error
	_, err := client.weightedRandomSelection(candidates)
	if err == nil {
		t.Error("Should have returned error for empty candidates")
	}
}

func TestClient_SelectRandomizersRandom(t *testing.T) {
	// Create a minimal client for testing
	client := &Client{
		diversityControls: cache.NewRandomizerDiversityControls(nil),
	}

	// Test random selection
	candidates := []*cache.BlockInfo{
		{CID: "cid1", Size: 1024, Popularity: 1},
		{CID: "cid2", Size: 1024, Popularity: 2},
		{CID: "cid3", Size: 1024, Popularity: 3},
	}

	selected1, selected2, err := client.selectRandomizersRandom(candidates)
	if err != nil {
		t.Fatalf("Failed to select randomizers randomly: %v", err)
	}

	if selected1 == nil || selected2 == nil {
		t.Fatal("Should have selected two randomizers")
	}

	if selected1.CID == selected2.CID {
		t.Error("Should have selected different randomizers")
	}

	// Both should be from our candidates
	validCIDs := map[string]bool{"cid1": true, "cid2": true, "cid3": true}
	if !validCIDs[selected1.CID] || !validCIDs[selected2.CID] {
		t.Error("Selected randomizers should be from candidate list")
	}
}

func TestClient_SelectRandomizersRandom_InsufficientCandidates(t *testing.T) {
	// Create a minimal client for testing
	client := &Client{
		diversityControls: cache.NewRandomizerDiversityControls(nil),
	}

	// Test with insufficient candidates
	candidates := []*cache.BlockInfo{
		{CID: "single", Size: 1024, Popularity: 1},
	}

	// Should work fine (will just use the single candidate twice is acceptable)
	// Actually, this should fail because we can't select 2 different from 1
	_, _, err := client.selectRandomizersRandom(candidates)
	if err == nil {
		t.Error("Should have returned error for insufficient candidates")
	}
}

func TestClient_SelectRandomizersWithDiversity_FallbackToRandom(t *testing.T) {
	// Create a minimal client without diversity controls
	client := &Client{
		diversityControls: nil, // No diversity controls
	}

	// Test candidates
	candidates := []*cache.BlockInfo{
		{CID: "cid1", Size: 1024, Popularity: 1},
		{CID: "cid2", Size: 1024, Popularity: 2},
		{CID: "cid3", Size: 1024, Popularity: 3},
	}

	// Should fall back to random selection
	selected1, selected2, err := client.selectRandomizersWithDiversity(candidates)
	if err != nil {
		t.Fatalf("Failed to select randomizers with diversity fallback: %v", err)
	}

	if selected1 == nil || selected2 == nil {
		t.Fatal("Should have selected two randomizers")
	}

	if selected1.CID == selected2.CID {
		t.Error("Should have selected different randomizers")
	}
}

func TestClient_SelectRandomizersWithDiversity_WithControls(t *testing.T) {
	// Create a minimal client with diversity controls
	client := &Client{
		diversityControls: cache.NewRandomizerDiversityControls(nil),
	}

	// Test candidates
	candidates := []*cache.BlockInfo{
		{CID: "cid1", Size: 1024, Popularity: 1},
		{CID: "cid2", Size: 1024, Popularity: 2},
		{CID: "cid3", Size: 1024, Popularity: 3},
	}

	// Should use diversity-aware selection
	selected1, selected2, err := client.selectRandomizersWithDiversity(candidates)
	if err != nil {
		t.Fatalf("Failed to select randomizers with diversity: %v", err)
	}

	if selected1 == nil || selected2 == nil {
		t.Fatal("Should have selected two randomizers")
	}

	if selected1.CID == selected2.CID {
		t.Error("Should have selected different randomizers")
	}
}

func BenchmarkClient_WeightedRandomSelectionSimple(b *testing.B) {
	// Create a minimal client for testing
	client := &Client{
		diversityControls: cache.NewRandomizerDiversityControls(nil),
	}

	// Create test candidates
	candidates := make([]scoredCandidate, 10)
	for i := 0; i < 10; i++ {
		candidates[i] = scoredCandidate{
			block: &cache.BlockInfo{
				CID:        "test-cid-" + string(rune(i)),
				Size:       1024,
				Popularity: i + 1,
			},
			score: float64(i + 1),
		}
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := client.weightedRandomSelection(candidates)
		if err != nil {
			b.Errorf("Failed to perform weighted selection: %v", err)
		}
	}
}

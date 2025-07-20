package noisefs

import (
	"context"
	"testing"

	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
)

func TestClient_DiversityControlsIntegration(t *testing.T) {
	// Create mock storage manager for testing
	config := storage.DefaultConfig()
	config.Backends = make(map[string]*storage.BackendConfig)

	config.Backends["memory"] = &storage.BackendConfig{
		Type:    storage.BackendTypeLocal,
		Enabled: true,
		Connection: &storage.ConnectionConfig{
			Endpoint: "memory://test",
		},
	}
	config.DefaultBackend = "memory"

	manager, err := storage.NewManager(config)
	if err != nil {
		t.Fatalf("Failed to create storage manager: %v", err)
	}
	defer manager.Stop(context.Background())

	// Create client with diversity controls
	blockCache := cache.NewMemoryCache(100)
	client, err := NewClient(manager, blockCache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Test that diversity controls are initialized
	if client.diversityControls == nil {
		t.Error("Diversity controls should be initialized")
	}

	// Test initial diversity metrics
	metrics := client.diversityControls.GetDiversityMetrics()
	if metrics.TotalRandomizers != 0 {
		t.Error("Should start with 0 randomizers")
	}

	if metrics.MaxUsageRatio != 0.0 {
		t.Error("Should start with 0 max usage ratio")
	}
}

func TestClient_DiversityControlsConfiguration(t *testing.T) {
	// Test diversity controls configuration with mock storage
	config := storage.DefaultConfig()
	config.Backends = make(map[string]*storage.BackendConfig)

	config.Backends["memory"] = &storage.BackendConfig{
		Type:    storage.BackendTypeLocal,
		Enabled: true,
		Connection: &storage.ConnectionConfig{
			Endpoint: "memory://test",
		},
	}
	config.DefaultBackend = "memory"

	manager, err := storage.NewManager(config)
	if err != nil {
		t.Fatalf("Failed to create storage manager: %v", err)
	}
	defer manager.Stop(context.Background())

	// Create client with diversity controls
	blockCache := cache.NewMemoryCache(100)
	client, err := NewClient(manager, blockCache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Test diversity controls configuration
	if client.diversityControls == nil {
		t.Fatal("Diversity controls should be configured")
	}

	// Test that we can record randomizer selections
	client.diversityControls.RecordRandomizerSelection("test-cid-1")
	client.diversityControls.RecordRandomizerSelection("test-cid-2")
	client.diversityControls.RecordRandomizerSelection("test-cid-1") // Repeat

	// Check metrics
	metrics := client.diversityControls.GetDiversityMetrics()
	if metrics.TotalRandomizers != 2 {
		t.Errorf("Expected 2 unique randomizers, got %d", metrics.TotalRandomizers)
	}

	// test-cid-1 should have higher usage (2 out of 3 selections = 66.7%)
	expectedRatio := 2.0 / 3.0
	if metrics.MaxUsageRatio < expectedRatio-0.01 || metrics.MaxUsageRatio > expectedRatio+0.01 {
		t.Errorf("Expected max usage ratio near %.2f, got %.2f", expectedRatio, metrics.MaxUsageRatio)
	}
}

func TestClient_SelectRandomizersWithDiversityUnit(t *testing.T) {
	// Create a minimal client for testing
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

func TestClient_WeightedRandomSelectionIntegration(t *testing.T) {
	// Create a minimal client for testing
	client := &Client{
		diversityControls: cache.NewRandomizerDiversityControls(nil),
	}

	// Create test candidates with different scores
	candidates := []scoredCandidate{
		{block: &cache.BlockInfo{CID: "low-score", Size: 1024, Popularity: 1}, score: 0.1},
		{block: &cache.BlockInfo{CID: "high-score", Size: 1024, Popularity: 5}, score: 1.0},
		{block: &cache.BlockInfo{CID: "medium-score", Size: 1024, Popularity: 3}, score: 0.5},
	}

	// Test weighted selection multiple times
	selections := make(map[string]int)
	iterations := 1000

	for i := 0; i < iterations; i++ {
		selected, err := client.weightedRandomSelection(candidates)
		if err != nil {
			t.Fatalf("Failed to perform weighted selection: %v", err)
		}

		selections[selected.CID]++
	}

	// High score should be selected more often than low score
	if selections["high-score"] <= selections["low-score"] {
		t.Errorf("High score randomizer should be selected more often. High: %d, Low: %d",
			selections["high-score"], selections["low-score"])
	}

	// All candidates should be selected at least once over 1000 iterations
	if selections["low-score"] == 0 || selections["high-score"] == 0 || selections["medium-score"] == 0 {
		t.Errorf("All candidates should be selected at least once. Counts: %v", selections)
	}
}

func TestClient_WeightedRandomSelectionZeroScores(t *testing.T) {
	// Create a minimal client for testing
	client := &Client{
		diversityControls: cache.NewRandomizerDiversityControls(nil),
	}

	// Create test candidates with zero scores (should fall back to uniform random)
	candidates := []scoredCandidate{
		{block: &cache.BlockInfo{CID: "zero-1", Size: 1024, Popularity: 1}, score: 0.0},
		{block: &cache.BlockInfo{CID: "zero-2", Size: 1024, Popularity: 2}, score: 0.0},
		{block: &cache.BlockInfo{CID: "zero-3", Size: 1024, Popularity: 3}, score: 0.0},
	}

	// Test that selection works even with zero scores
	selected, err := client.weightedRandomSelection(candidates)
	if err != nil {
		t.Fatalf("Failed to perform weighted selection with zero scores: %v", err)
	}

	if selected == nil {
		t.Fatal("Should have selected a candidate even with zero scores")
	}

	// Verify it's one of our candidates
	found := false
	for _, candidate := range candidates {
		if candidate.block.CID == selected.CID {
			found = true
			break
		}
	}

	if !found {
		t.Error("Selected candidate should be from the candidate list")
	}
}

func TestClient_DiversityControlsRecordingUnit(t *testing.T) {
	// Create a minimal client for testing
	client := &Client{
		diversityControls: cache.NewRandomizerDiversityControls(nil),
	}

	// Test that diversity controls record selections
	if client.diversityControls == nil {
		t.Fatal("Diversity controls should be initialized")
	}

	// Initial metrics
	initialMetrics := client.diversityControls.GetDiversityMetrics()
	if initialMetrics.TotalRandomizers != 0 {
		t.Error("Should start with 0 randomizers")
	}

	// Simulate recording selections
	client.diversityControls.RecordRandomizerSelection("test-cid-1")
	client.diversityControls.RecordRandomizerSelection("test-cid-2")
	client.diversityControls.RecordRandomizerSelection("test-cid-1") // Repeat

	// Check updated metrics
	updatedMetrics := client.diversityControls.GetDiversityMetrics()
	if updatedMetrics.TotalRandomizers != 2 {
		t.Errorf("Expected 2 unique randomizers, got %d", updatedMetrics.TotalRandomizers)
	}

	// test-cid-1 should have higher usage ratio
	if updatedMetrics.MaxUsageRatio != 2.0/3.0 { // 2 out of 3 total selections
		t.Errorf("Expected max usage ratio of 0.67, got %f", updatedMetrics.MaxUsageRatio)
	}
}

func BenchmarkClient_SelectRandomizersWithDiversityIntegration(b *testing.B) {
	// Create a minimal client for testing
	client := &Client{
		diversityControls: cache.NewRandomizerDiversityControls(nil),
	}

	// Create test candidates
	candidates := make([]*cache.BlockInfo, 20)
	for i := 0; i < 20; i++ {
		candidates[i] = &cache.BlockInfo{
			CID:        "test-cid-" + string(rune(i)),
			Size:       1024,
			Popularity: i + 1,
		}
	}

	// Pre-populate diversity controls with some history
	for i := 0; i < 100; i++ {
		cid := "test-cid-" + string(rune(i%10))
		client.diversityControls.RecordRandomizerSelection(cid)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, err := client.selectRandomizersWithDiversity(candidates)
		if err != nil {
			b.Errorf("Failed to select randomizers: %v", err)
		}
	}
}

func BenchmarkClient_WeightedRandomSelectionIntegration(b *testing.B) {
	// Create a minimal client for testing
	client := &Client{
		diversityControls: cache.NewRandomizerDiversityControls(nil),
	}

	// Create test candidates
	candidates := make([]scoredCandidate, 20)
	for i := 0; i < 20; i++ {
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

package noisefs

import (
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
)

func TestClient_AvailabilityIntegration(t *testing.T) {
	// Create a real storage manager interface for testing
	realStorageManager := &storage.Manager{}
	
	// Create basic cache
	blockCache := cache.NewMemoryCache(100)
	
	// Create client config with availability integration
	config := &ClientConfig{
		EnableAdaptiveCache: false,
		AvailabilityConfig: &cache.AvailabilityConfig{
			CacheTTL:                 5 * time.Minute,
			CheckTimeout:             10 * time.Second,
			MaxConcurrentChecks:      10,
			MinAvailabilityThreshold: 0.5,
		},
	}
	
	// Create client - this should not panic even with nil storage manager for this test
	client := &Client{
		storageManager: realStorageManager,
		cache:          blockCache,
		metrics:        NewMetrics(),
	}
	
	// Initialize availability integration manually for testing
	client.availabilityIntegration = cache.NewAvailabilityIntegration(realStorageManager, config.AvailabilityConfig)
	
	// Test that availability integration is enabled
	if !client.IsAvailabilityIntegrationEnabled() {
		t.Error("Expected availability integration to be enabled")
	}
	
	// Test that availability score has a default value
	score := client.GetAvailabilityScore()
	if score < 0 || score > 1 {
		t.Errorf("Expected availability score between 0 and 1, got %f", score)
	}
	
	// Test that availability metrics can be retrieved
	metrics := client.GetAvailabilityMetrics()
	if metrics == nil {
		t.Error("Expected availability metrics to be available")
	}
	
	// Test that metrics include availability health
	clientMetrics := client.GetMetrics()
	if clientMetrics.AvailabilityHealth < 0 || clientMetrics.AvailabilityHealth > 1 {
		t.Errorf("Expected availability health between 0 and 1, got %f", clientMetrics.AvailabilityHealth)
	}
	
}

func TestClient_AvailabilityIntegration_Disabled(t *testing.T) {
	// Create a real storage manager interface for testing
	realStorageManager := &storage.Manager{}
	
	// Create basic cache
	blockCache := cache.NewMemoryCache(100)
	
	// Create client without availability integration
	client := &Client{
		storageManager: realStorageManager,
		cache:          blockCache,
		metrics:        NewMetrics(),
		// availabilityIntegration is nil
	}
	
	// Test that availability integration is disabled
	if client.IsAvailabilityIntegrationEnabled() {
		t.Error("Expected availability integration to be disabled")
	}
	
	// Test that availability score has default value
	score := client.GetAvailabilityScore()
	if score != 1.0 {
		t.Errorf("Expected default availability score of 1.0, got %f", score)
	}
	
	// Test that availability metrics are nil
	metrics := client.GetAvailabilityMetrics()
	if metrics != nil {
		t.Error("Expected availability metrics to be nil when disabled")
	}
	
}

func TestClient_AvailabilityAwareRandomizerSelection(t *testing.T) {
	// Create mock block candidates
	candidates := []*cache.BlockInfo{
		{
			CID:        "block1",
			Size:       1024,
			Popularity: 5,
			Block:      nil, // Mock block
		},
		{
			CID:        "block2", 
			Size:       1024,
			Popularity: 3,
			Block:      nil, // Mock block
		},
		{
			CID:        "block3",
			Size:       1024,
			Popularity: 7,
			Block:      nil, // Mock block
		},
	}
	
	// Create client with availability integration
	client := &Client{
		storageManager: &storage.Manager{},
		cache:          cache.NewMemoryCache(100),
		metrics:        NewMetrics(),
		diversityControls: cache.NewRandomizerDiversityControls(nil),
		// availabilityIntegration is nil for this test
	}
	
	// Test fallback to diversity-only selection when availability integration is disabled
	selected1, selected2, err := client.selectRandomizersWithDiversityAndAvailability(candidates)
	if err != nil {
		t.Fatalf("Expected selection to succeed with fallback, got error: %v", err)
	}
	
	if selected1 == nil || selected2 == nil {
		t.Error("Expected two selected randomizers")
	}
	
	if selected1.CID == selected2.CID {
		t.Error("Expected different randomizers to be selected")
	}
}
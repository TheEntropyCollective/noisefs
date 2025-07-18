package benchmarks

import (
	"context"
	"fmt"
	"strings"
	"testing"

	noisefs "github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/logging"
	
	// Import backends to register them
	_ "github.com/TheEntropyCollective/noisefs/pkg/storage/backends"
)

// TestDebugOverhead helps debug why randomizer reuse isn't working
func TestDebugOverhead(t *testing.T) {
	// Create storage setup with mock backend
	cache := cache.NewMemoryCache(1000)
	
	// Create storage config with mock backend
	config := storage.DefaultConfig()
	config.Backends = map[string]*storage.BackendConfig{
		"mock": {
			Type:     "mock",
			Enabled:  true,
			Priority: 100,
			Connection: &storage.ConnectionConfig{
				Endpoint: "mock://test",
			},
		},
	}
	config.DefaultBackend = "mock"
	
	// Create storage manager
	storageManager, err := storage.NewManager(config)
	if err != nil {
		t.Fatalf("Failed to create storage manager: %v", err)
	}
	
	// Start storage manager
	ctx := context.Background()
	if err := storageManager.Start(ctx); err != nil {
		t.Fatalf("Failed to start storage manager: %v", err)
	}
	defer storageManager.Stop(ctx)
	
	// Create NoiseFS client with the storage manager
	client, err := noisefs.NewClientWithStorageManager(storageManager, cache)
	if err != nil {
		t.Fatalf("Failed to create NoiseFS client: %v", err)
	}

	// Test with 128KB file (should be exactly 1 block)
	testData := make([]byte, 128*1024)
	for i := range testData {
		testData[i] = byte(i % 256)
	}
	
	t.Logf("=== FIRST UPLOAD (should have 0%% reuse) ===")
	cacheStatsBefore := cache.GetStats()
	t.Logf("Cache before: %d blocks", cacheStatsBefore.Size)
	
	initialMetrics := client.GetMetrics()
	initialStored := initialMetrics.BytesStoredIPFS
	
	reader1 := strings.NewReader(string(testData))
	_, err = client.Upload(reader1, "test1.dat")
	if err != nil {
		t.Fatalf("First upload failed: %v", err)
	}
	
	finalMetrics1 := client.GetMetrics()
	finalStored1 := finalMetrics1.BytesStoredIPFS
	bytesStored1 := finalStored1 - initialStored
	overhead1 := (float64(bytesStored1) / float64(len(testData))) * 100.0
	
	cacheStatsAfter1 := cache.GetStats()
	t.Logf("Cache after: %d blocks", cacheStatsAfter1.Size)
	t.Logf("First upload: %d bytes stored, %.1f%% overhead", bytesStored1, overhead1)
	
	// Check what's in the cache
	randomizers, err := cache.GetRandomizers(10)
	if err != nil {
		t.Fatalf("Failed to get randomizers: %v", err)
	}
	t.Logf("Available randomizers: %d", len(randomizers))
	for i, r := range randomizers {
		t.Logf("  Randomizer %d: CID=%s, Size=%d, Popularity=%d", i, r.CID[:8], r.Size, r.Popularity)
	}
	
	t.Logf("\n=== SECOND UPLOAD (should have high reuse) ===")
	
	// Now upload the same size file again - should reuse randomizers
	reader2 := strings.NewReader(string(testData))
	_, err = client.Upload(reader2, "test2.dat") 
	if err != nil {
		t.Fatalf("Second upload failed: %v", err)
	}
	
	finalMetrics2 := client.GetMetrics()
	finalStored2 := finalMetrics2.BytesStoredIPFS
	bytesStored2 := finalStored2 - finalStored1  // Incremental storage for second file
	overhead2 := (float64(bytesStored2) / float64(len(testData))) * 100.0
	
	cacheStatsAfter2 := cache.GetStats()
	t.Logf("Cache after second: %d blocks", cacheStatsAfter2.Size)
	t.Logf("Second upload: %d bytes stored, %.1f%% overhead", bytesStored2, overhead2)
	
	// Check cache hit rate
	cacheStats := cache.GetStats()
	t.Logf("Cache stats: hits=%d, misses=%d, hit_rate=%.1f%%", cacheStats.Hits, cacheStats.Misses, cacheStats.HitRate*100)
	
	// Check what's in cache after second upload
	randomizers2, err := cache.GetRandomizers(10)
	if err != nil {
		t.Fatalf("Failed to get randomizers: %v", err)
	}
	t.Logf("Available randomizers after second upload: %d", len(randomizers2))
	for i, r := range randomizers2 {
		t.Logf("  Randomizer %d: CID=%s, Size=%d, Popularity=%d", i, r.CID[:8], r.Size, r.Popularity)
	}
	
	// Verify expectations
	if overhead1 < 250 || overhead1 > 350 {
		t.Errorf("First upload overhead %.1f%% unexpected (should be ~300%%)", overhead1)
	}
	
	if overhead2 > 150 {  // Second upload should have much lower overhead due to reuse
		t.Errorf("Second upload overhead %.1f%% too high (should be <150%% with reuse)", overhead2)
	}
}
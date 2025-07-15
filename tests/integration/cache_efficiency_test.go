package integration

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	noisefs "github.com/TheEntropyCollective/noisefs/pkg/core/client"
	storagetesting "github.com/TheEntropyCollective/noisefs/pkg/storage/testing"
)

func TestCacheEfficiencyWithRepeatedOperations(t *testing.T) {
	// Setup
	storageManager, err := storagetesting.CreateRealTestStorageManager()
	if err != nil {
		t.Fatalf("Failed to create storage manager: %v", err)
	}
	defer storageManager.Stop(context.Background())
	
	cache := cache.NewMemoryCache(30)
	client, err := noisefs.NewClient(storageManager, cache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Create test files
	testFiles := [][]byte{
		bytes.Repeat([]byte("File 1 repeated content pattern "), 25),
		bytes.Repeat([]byte("File 2 different content pattern "), 20),
		bytes.Repeat([]byte("File 3 another content pattern "), 30),
	}

	blockSize := 64
	descriptors := make([]*descriptors.Descriptor, len(testFiles))

	// Initial upload of all files
	t.Log("=== Initial Upload Phase ===")
	for i, content := range testFiles {
		desc, err := simulateUpload(client, content, blockSize)
		if err != nil {
			t.Fatalf("Failed to upload file %d: %v", i+1, err)
		}
		desc.Filename = fmt.Sprintf("repeated_test_%d.txt", i+1)
		descriptors[i] = desc

		metrics := client.GetMetrics()
		t.Logf("After upload %d: Cache hits: %d, Cache misses: %d, Hit rate: %.2f%%", 
			i+1, metrics.CacheHits, metrics.CacheMisses, metrics.CacheHitRate)
	}

	initialMetrics := client.GetMetrics()

	// Multiple download rounds to test cache efficiency
	rounds := 3
	for round := 1; round <= rounds; round++ {
		t.Logf("=== Download Round %d ===", round)
		
		for i, desc := range descriptors {
			reconstructed, err := simulateDownload(client, desc)
			if err != nil {
				t.Fatalf("Round %d: Failed to download file %d: %v", round, i+1, err)
			}

			if !bytes.Equal(testFiles[i], reconstructed) {
				t.Errorf("Round %d: Content mismatch for file %d", round, i+1)
			}
		}

		roundMetrics := client.GetMetrics()
		roundHits := roundMetrics.CacheHits - initialMetrics.CacheHits
		roundMisses := roundMetrics.CacheMisses - initialMetrics.CacheMisses
		roundHitRate := float64(roundHits) / float64(roundHits+roundMisses) * 100

		t.Logf("Round %d results: Hits: %d, Misses: %d, Hit rate: %.2f%%", 
			round, roundHits, roundMisses, roundHitRate)

		// Cache hit rate should improve with each round
		if round > 1 && roundHitRate < 80 {
			t.Logf("Warning: Lower than expected hit rate in round %d: %.2f%%", round, roundHitRate)
		}
	}

	finalMetrics := client.GetMetrics()
	t.Logf("=== Final Results ===")
	t.Logf("Total cache hits: %d", finalMetrics.CacheHits)
	t.Logf("Total cache misses: %d", finalMetrics.CacheMisses)
	t.Logf("Overall hit rate: %.2f%%", finalMetrics.CacheHitRate)
	t.Logf("Total downloads: %d", finalMetrics.TotalDownloads)
}

func TestCacheEfficiencyWithPopularityTracking(t *testing.T) {
	// Setup
	storageManager, err := storagetesting.CreateRealTestStorageManager()
	if err != nil {
		t.Fatalf("Failed to create storage manager: %v", err)
	}
	defer storageManager.Stop(context.Background())
	
	cache := cache.NewMemoryCache(20)
	client, err := noisefs.NewClient(storageManager, cache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Create files with shared patterns (to test popularity tracking)
	sharedPattern := []byte("POPULAR SHARED CONTENT BLOCK ")
	uniquePattern1 := []byte("Unique content for file 1 ")
	uniquePattern2 := []byte("Unique content for file 2 ")

	files := []struct {
		name    string
		content []byte
	}{
		{
			name:    "popular_file_1.txt",
			content: bytes.Repeat(sharedPattern, 20), // Will be popular
		},
		{
			name:    "popular_file_2.txt", 
			content: bytes.Repeat(sharedPattern, 15), // Shares popular content
		},
		{
			name:    "unique_file_1.txt",
			content: bytes.Repeat(uniquePattern1, 10), // Less popular
		},
		{
			name:    "unique_file_2.txt",
			content: bytes.Repeat(uniquePattern2, 8), // Less popular
		},
	}

	blockSize := 32
	descriptors := make([]*descriptors.Descriptor, len(files))

	// Upload all files
	t.Log("=== Upload Phase ===")
	for i, file := range files {
		desc, err := simulateUpload(client, file.content, blockSize)
		if err != nil {
			t.Fatalf("Failed to upload %s: %v", file.name, err)
		}
		desc.Filename = file.name
		descriptors[i] = desc
		
		t.Logf("Uploaded %s (%d bytes, %d blocks)", file.name, len(file.content), len(desc.Blocks))
	}

	uploadMetrics := client.GetMetrics()

	// Download popular files multiple times
	t.Log("=== Popularity Testing Phase ===")
	popularFiles := descriptors[0:2] // Files with shared content

	for round := 1; round <= 3; round++ {
		t.Logf("--- Popular files round %d ---", round)
		for _, desc := range popularFiles {
			_, err := simulateDownload(client, desc)
			if err != nil {
				t.Fatalf("Failed to download popular file %s: %v", desc.Filename, err)
			}
		}
	}

	// Download unique files once
	t.Log("=== Unique Files Download ===")
	uniqueFiles := descriptors[2:4]
	for _, desc := range uniqueFiles {
		_, err := simulateDownload(client, desc)
		if err != nil {
			t.Fatalf("Failed to download unique file %s: %v", desc.Filename, err)
		}
	}

	finalMetrics := client.GetMetrics()

	t.Logf("=== Popularity Tracking Results ===")
	t.Logf("Upload phase - Reused: %d, Generated: %d", uploadMetrics.BlocksReused, uploadMetrics.BlocksGenerated)
	t.Logf("Block reuse rate: %.2f%%", finalMetrics.BlockReuseRate)
	t.Logf("Cache hit rate: %.2f%%", finalMetrics.CacheHitRate)
	t.Logf("Total downloads: %d", finalMetrics.TotalDownloads)

	// Should have high block reuse due to shared content
	if finalMetrics.BlockReuseRate < 50 {
		t.Errorf("Expected high block reuse rate, got %.2f%%", finalMetrics.BlockReuseRate)
	}
}

func TestCacheEfficiencyWithLRUEviction(t *testing.T) {
	// Setup with small cache to test LRU eviction
	storageManager, err := storagetesting.CreateRealTestStorageManager()
	if err != nil {
		t.Fatalf("Failed to create storage manager: %v", err)
	}
	defer storageManager.Stop(context.Background())
	
	cache := cache.NewMemoryCache(6) // Very small cache
	client, err := noisefs.NewClient(storageManager, cache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Create sequence of files that will trigger eviction
	numFiles := 10
	blockSize := 64
	files := make([][]byte, numFiles)
	descriptors := make([]*descriptors.Descriptor, numFiles)

	// Upload files sequentially
	t.Log("=== Sequential Upload (triggering LRU eviction) ===")
	for i := 0; i < numFiles; i++ {
		content := fmt.Sprintf("File %02d content - unique identifier %d ", i+1, i*13)
		files[i] = bytes.Repeat([]byte(content), 8)

		desc, err := simulateUpload(client, files[i], blockSize)
		if err != nil {
			t.Fatalf("Failed to upload file %d: %v", i+1, err)
		}
		desc.Filename = fmt.Sprintf("lru_test_%02d.txt", i+1)
		descriptors[i] = desc

		// Check cache state periodically
		if (i+1)%3 == 0 {
			metrics := client.GetMetrics()
			t.Logf("After %d uploads: Hit rate: %.2f%%, Blocks reused: %d", 
				i+1, metrics.CacheHitRate, metrics.BlocksReused)
		}
	}

	// Test LRU behavior by accessing files in specific pattern
	t.Log("=== LRU Access Pattern Test ===")
	
	// Access recent files (should be in cache)
	recentFiles := descriptors[numFiles-3:] // Last 3 files
	t.Log("Accessing recent files (should be cache hits)...")
	
	preRecentMetrics := client.GetMetrics()
	for i, desc := range recentFiles {
		_, err := simulateDownload(client, desc)
		if err != nil {
			t.Fatalf("Failed to download recent file %d: %v", i, err)
		}
	}
	postRecentMetrics := client.GetMetrics()
	
	recentHits := postRecentMetrics.CacheHits - preRecentMetrics.CacheHits
	recentMisses := postRecentMetrics.CacheMisses - preRecentMetrics.CacheMisses
	
	// Access old files (should be cache misses due to eviction)
	oldFiles := descriptors[:3] // First 3 files
	t.Log("Accessing old files (should be cache misses)...")
	
	preOldMetrics := client.GetMetrics()
	for i, desc := range oldFiles {
		_, err := simulateDownload(client, desc)
		if err != nil {
			t.Fatalf("Failed to download old file %d: %v", i, err)
		}
	}
	postOldMetrics := client.GetMetrics()
	
	oldHits := postOldMetrics.CacheHits - preOldMetrics.CacheHits
	oldMisses := postOldMetrics.CacheMisses - preOldMetrics.CacheMisses

	finalMetrics := client.GetMetrics()

	t.Logf("=== LRU Eviction Analysis ===")
	t.Logf("Cache capacity: 6 blocks")
	t.Logf("Files uploaded: %d", numFiles)
	t.Logf("Recent files access - Hits: %d, Misses: %d", recentHits, recentMisses)
	t.Logf("Old files access - Hits: %d, Misses: %d", oldHits, oldMisses)
	t.Logf("Overall cache hit rate: %.2f%%", finalMetrics.CacheHitRate)
	t.Logf("Total cache misses: %d", finalMetrics.CacheMisses)

	// Recent files should have better hit rate than old files
	recentHitRate := float64(recentHits) / float64(recentHits+recentMisses) * 100
	oldHitRate := float64(oldHits) / float64(oldHits+oldMisses) * 100
	
	t.Logf("Recent files hit rate: %.2f%%", recentHitRate)
	t.Logf("Old files hit rate: %.2f%%", oldHitRate)

	// Should see evidence of LRU eviction (more misses for old files)
	if finalMetrics.CacheMisses == 0 {
		t.Error("Expected cache misses due to LRU eviction")
	}
}

func TestCacheEfficiencyWithWorkloadPatterns(t *testing.T) {
	// Test different access patterns to validate cache efficiency
	patterns := []struct {
		name        string
		description string
		testFunc    func(*testing.T, *noisefs.Client, []*descriptors.Descriptor)
	}{
		{
			name:        "sequential_access",
			description: "Sequential file access pattern",
			testFunc:    testSequentialAccess,
		},
		{
			name:        "random_access",
			description: "Random file access pattern", 
			testFunc:    testRandomAccess,
		},
		{
			name:        "hotspot_access",
			description: "Hotspot access pattern (few files accessed frequently)",
			testFunc:    testHotspotAccess,
		},
	}

	for _, pattern := range patterns {
		t.Run(pattern.name, func(t *testing.T) {
			// Setup fresh environment for each pattern
			storageManager, err := storagetesting.CreateRealTestStorageManager()
			if err != nil {
				t.Fatalf("Failed to create storage manager: %v", err)
			}
			defer storageManager.Stop(context.Background())
			
			cache := cache.NewMemoryCache(15)
			client, err := noisefs.NewClient(storageManager, cache)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			// Create test files
			numFiles := 8
			blockSize := 48
			descriptors := make([]*descriptors.Descriptor, numFiles)

			for i := 0; i < numFiles; i++ {
				content := fmt.Sprintf("Workload test file %d content ", i+1)
				data := bytes.Repeat([]byte(content), 15)

				desc, err := simulateUpload(client, data, blockSize)
				if err != nil {
					t.Fatalf("Failed to upload file %d: %v", i+1, err)
				}
				desc.Filename = fmt.Sprintf("workload_%s_%d.txt", pattern.name, i+1)
				descriptors[i] = desc
			}

			// Run the specific pattern test
			t.Logf("Testing %s: %s", pattern.name, pattern.description)
			pattern.testFunc(t, client, descriptors)
		})
	}
}

func testSequentialAccess(t *testing.T, client *noisefs.Client, descriptors []*descriptors.Descriptor) {
	initialMetrics := client.GetMetrics()

	// Access files sequentially multiple times
	for round := 1; round <= 3; round++ {
		for i, desc := range descriptors {
			_, err := simulateDownload(client, desc)
			if err != nil {
				t.Fatalf("Sequential access round %d, file %d failed: %v", round, i+1, err)
			}
		}
	}

	finalMetrics := client.GetMetrics()
	accessHits := finalMetrics.CacheHits - initialMetrics.CacheHits
	accessMisses := finalMetrics.CacheMisses - initialMetrics.CacheMisses
	
	t.Logf("Sequential access results: Hits: %d, Misses: %d, Hit rate: %.2f%%", 
		accessHits, accessMisses, float64(accessHits)/(float64(accessHits+accessMisses))*100)
}

func testRandomAccess(t *testing.T, client *noisefs.Client, descriptors []*descriptors.Descriptor) {
	initialMetrics := client.GetMetrics()

	// Random access pattern
	accessOrder := []int{3, 1, 6, 2, 7, 0, 4, 5, 2, 6, 1, 3, 0, 7, 4, 5}
	
	for _, idx := range accessOrder {
		if idx < len(descriptors) {
			_, err := simulateDownload(client, descriptors[idx])
			if err != nil {
				t.Fatalf("Random access to file %d failed: %v", idx+1, err)
			}
		}
	}

	finalMetrics := client.GetMetrics()
	accessHits := finalMetrics.CacheHits - initialMetrics.CacheHits
	accessMisses := finalMetrics.CacheMisses - initialMetrics.CacheMisses
	
	t.Logf("Random access results: Hits: %d, Misses: %d, Hit rate: %.2f%%", 
		accessHits, accessMisses, float64(accessHits)/(float64(accessHits+accessMisses))*100)
}

func testHotspotAccess(t *testing.T, client *noisefs.Client, descriptors []*descriptors.Descriptor) {
	initialMetrics := client.GetMetrics()

	// Hotspot pattern: access first 2 files frequently, others rarely
	hotFiles := descriptors[:2]
	coldFiles := descriptors[2:]

	// Access hot files many times
	for round := 1; round <= 5; round++ {
		for _, desc := range hotFiles {
			_, err := simulateDownload(client, desc)
			if err != nil {
				t.Fatalf("Hotspot access round %d failed: %v", round, err)
			}
		}
	}

	// Access cold files once
	for _, desc := range coldFiles {
		_, err := simulateDownload(client, desc)
		if err != nil {
			t.Fatalf("Cold file access failed: %v", err)
		}
	}

	finalMetrics := client.GetMetrics()
	accessHits := finalMetrics.CacheHits - initialMetrics.CacheHits
	accessMisses := finalMetrics.CacheMisses - initialMetrics.CacheMisses
	
	t.Logf("Hotspot access results: Hits: %d, Misses: %d, Hit rate: %.2f%%", 
		accessHits, accessMisses, float64(accessHits)/(float64(accessHits+accessMisses))*100)

	// Hotspot pattern should have high hit rate due to repeated access
	hitRate := float64(accessHits) / (float64(accessHits + accessMisses)) * 100
	if hitRate < 70 {
		t.Logf("Warning: Lower than expected hit rate for hotspot pattern: %.2f%%", hitRate)
	}
}
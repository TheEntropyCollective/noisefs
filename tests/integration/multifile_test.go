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

func TestMultiFileBlockReuse(t *testing.T) {
	// Setup
	storageManager, err := storagetesting.CreateRealTestStorageManager()
	if err != nil {
		t.Fatalf("Failed to create storage manager: %v", err)
	}
	defer storageManager.Stop(context.Background())
	
	cache := cache.NewMemoryCache(50) // Large cache to see maximum reuse
	client, err := noisefs.NewClient(storageManager, cache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Define test files with overlapping content patterns
	testFiles := []struct {
		name     string
		content  []byte
		expected string
	}{
		{
			name:     "file1.txt",
			content:  bytes.Repeat([]byte("Common pattern A"), 20),
			expected: "Common pattern A",
		},
		{
			name:     "file2.txt", 
			content:  bytes.Repeat([]byte("Common pattern B"), 15),
			expected: "Common pattern B",
		},
		{
			name:     "file3.txt",
			content:  bytes.Repeat([]byte("Common pattern A"), 10), // Reuses pattern from file1
			expected: "Common pattern A",
		},
		{
			name:     "file4.txt",
			content:  bytes.Repeat([]byte("Unique content X"), 12),
			expected: "Unique content X",
		},
		{
			name:     "file5.txt",
			content:  bytes.Repeat([]byte("Common pattern B"), 25), // Reuses pattern from file2
			expected: "Common pattern B",
		},
	}

	blockSize := 64
	descriptors := make([]*descriptors.Descriptor, len(testFiles))

	// Upload all files and track metrics
	for i, file := range testFiles {
		desc, err := simulateUpload(client, file.content, blockSize)
		if err != nil {
			t.Fatalf("Failed to upload %s: %v", file.name, err)
		}
		desc.Filename = file.name // Update filename
		descriptors[i] = desc

		t.Logf("Uploaded %s (%d bytes, %d blocks)", file.name, len(file.content), len(desc.Blocks))
	}

	// Check block reuse metrics
	finalMetrics := client.GetMetrics()
	
	t.Logf("Final metrics after all uploads:")
	t.Logf("  Blocks reused: %d", finalMetrics.BlocksReused)
	t.Logf("  Blocks generated: %d", finalMetrics.BlocksGenerated)
	t.Logf("  Block reuse rate: %.2f%%", finalMetrics.BlockReuseRate)
	t.Logf("  Total uploads: %d", finalMetrics.TotalUploads)

	// Verify significant block reuse occurred
	if finalMetrics.BlockReuseRate < 30 {
		t.Errorf("Expected at least 30%% block reuse, got %.2f%%", finalMetrics.BlockReuseRate)
	}

	// Download and verify all files
	for i, desc := range descriptors {
		reconstructed, err := simulateDownload(client, desc)
		if err != nil {
			t.Fatalf("Failed to download %s: %v", desc.Filename, err)
		}

		if !bytes.Equal(testFiles[i].content, reconstructed) {
			t.Errorf("File %s: content mismatch", desc.Filename)
		}

		// Verify content contains expected pattern
		if !bytes.Contains(reconstructed, []byte(testFiles[i].expected)) {
			t.Errorf("File %s: missing expected pattern %s", desc.Filename, testFiles[i].expected)
		}
	}

	downloadMetrics := client.GetMetrics()
	t.Logf("Download metrics:")
	t.Logf("  Total downloads: %d", downloadMetrics.TotalDownloads)
	t.Logf("  Cache hit rate: %.2f%%", downloadMetrics.CacheHitRate)
}

func TestMultiFileStorageEfficiency(t *testing.T) {
	// Setup
	storageManager, err := storagetesting.CreateRealTestStorageManager()
	if err != nil {
		t.Fatalf("Failed to create storage manager: %v", err)
	}
	defer storageManager.Stop(context.Background())
	
	cache := cache.NewMemoryCache(100)
	client, err := noisefs.NewClient(storageManager, cache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Create files with varying degrees of similarity
	files := [][]byte{
		// File 1: Base pattern
		bytes.Repeat([]byte("AAAABBBBCCCCDDDD"), 32),
		// File 2: 50% overlap with File 1
		append(bytes.Repeat([]byte("AAAABBBBCCCCDDDD"), 16), bytes.Repeat([]byte("EEEEFFFF"), 16)...),
		// File 3: 25% overlap with File 1
		append(bytes.Repeat([]byte("AAAABBBB"), 16), bytes.Repeat([]byte("GGGGHHHHIIIIJJJJ"), 24)...),
		// File 4: No overlap
		bytes.Repeat([]byte("KKKKLLLLMMMMNNN"), 34),
	}

	blockSize := 32
	totalOriginalSize := int64(0)

	// Upload all files
	for i, content := range files {
		_, err := simulateUpload(client, content, blockSize)
		if err != nil {
			t.Fatalf("Failed to upload file %d: %v", i+1, err)
		}
		totalOriginalSize += int64(len(content))
	}

	// Analyze storage efficiency
	metrics := client.GetMetrics()
	storageOverhead := float64(metrics.BytesStoredIPFS) / float64(totalOriginalSize)
	
	t.Logf("Storage efficiency analysis:")
	t.Logf("  Total original size: %d bytes", totalOriginalSize)
	t.Logf("  Total stored size: %d bytes", metrics.BytesStoredIPFS)
	t.Logf("  Storage overhead: %.2fx", storageOverhead)
	t.Logf("  Block reuse rate: %.2f%%", metrics.BlockReuseRate)
	
	// With block reuse, we should see less than 2x storage overhead
	if storageOverhead > 2.5 {
		t.Errorf("Storage overhead too high: %.2fx (expected < 2.5x)", storageOverhead)
	}

	// Should have some meaningful block reuse
	if metrics.BlockReuseRate == 0 {
		t.Error("Expected some block reuse with overlapping content")
	}
}

func TestMultiFileWithDifferentBlockSizes(t *testing.T) {
	// Setup
	storageManager, err := storagetesting.CreateRealTestStorageManager()
	if err != nil {
		t.Fatalf("Failed to create storage manager: %v", err)
	}
	defer storageManager.Stop(context.Background())
	
	cache := cache.NewMemoryCache(200)
	client, err := noisefs.NewClient(storageManager, cache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Test files with different block sizes
	testCases := []struct {
		name      string
		content   []byte
		blockSize int
	}{
		{
			name:      "small_blocks.txt",
			content:   bytes.Repeat([]byte("Small block content "), 20),
			blockSize: 16,
		},
		{
			name:      "medium_blocks.txt", 
			content:   bytes.Repeat([]byte("Medium block content "), 30),
			blockSize: 64,
		},
		{
			name:      "large_blocks.txt",
			content:   bytes.Repeat([]byte("Large block content "), 40),
			blockSize: 256,
		},
		{
			name:      "mixed_content.txt",
			content:   append(bytes.Repeat([]byte("Small block content "), 10), bytes.Repeat([]byte("Large block content "), 20)...),
			blockSize: 128,
		},
	}

	descriptors := make([]*descriptors.Descriptor, len(testCases))

	// Upload files with different block sizes
	for i, tc := range testCases {
		desc, err := simulateUpload(client, tc.content, tc.blockSize)
		if err != nil {
			t.Fatalf("Failed to upload %s: %v", tc.name, err)
		}
		desc.Filename = tc.name
		descriptors[i] = desc

		t.Logf("Uploaded %s: %d bytes, %d blocks of size %d", 
			tc.name, len(tc.content), len(desc.Blocks), tc.blockSize)
	}

	// Verify all files can be downloaded correctly
	for i, desc := range descriptors {
		reconstructed, err := simulateDownload(client, desc)
		if err != nil {
			t.Fatalf("Failed to download %s: %v", desc.Filename, err)
		}

		if !bytes.Equal(testCases[i].content, reconstructed) {
			t.Errorf("Content mismatch for %s", desc.Filename)
		}
	}

	// Check that randomizers with different sizes were managed properly
	metrics := client.GetMetrics()
	t.Logf("Multi-block-size metrics:")
	t.Logf("  Total files: %d", len(testCases))
	t.Logf("  Blocks generated: %d", metrics.BlocksGenerated)
	t.Logf("  Blocks reused: %d", metrics.BlocksReused)
	t.Logf("  Cache hit rate: %.2f%%", metrics.CacheHitRate)

	// Should have generated randomizers for each block size
	if metrics.BlocksGenerated == 0 {
		t.Error("Expected some randomizer blocks to be generated")
	}
}

func TestConcurrentMultiFileOperations(t *testing.T) {
	// Note: This test simulates concurrent operations by interleaving 
	// upload/download operations rather than true concurrency
	// since our mock infrastructure isn't thread-safe

	// Setup
	storageManager, err := storagetesting.CreateRealTestStorageManager()
	if err != nil {
		t.Fatalf("Failed to create storage manager: %v", err)
	}
	defer storageManager.Stop(context.Background())
	
	cache := cache.NewMemoryCache(50)
	client, err := noisefs.NewClient(storageManager, cache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Create several files
	files := make([][]byte, 5)
	for i := range files {
		content := fmt.Sprintf("File %d content - unique data pattern %d", i+1, i*17)
		files[i] = bytes.Repeat([]byte(content), 10)
	}

	blockSize := 64
	descriptors := make([]*descriptors.Descriptor, len(files))

	// Simulate concurrent upload pattern: start all uploads
	for i, content := range files {
		desc, err := simulateUpload(client, content, blockSize)
		if err != nil {
			t.Fatalf("Failed to upload file %d: %v", i+1, err)
		}
		desc.Filename = fmt.Sprintf("concurrent_file_%d.txt", i+1)
		descriptors[i] = desc
	}

	uploadMetrics := client.GetMetrics()

	// Simulate concurrent download pattern: download in different order
	downloadOrder := []int{2, 0, 4, 1, 3} // Mixed order
	for _, i := range downloadOrder {
		reconstructed, err := simulateDownload(client, descriptors[i])
		if err != nil {
			t.Fatalf("Failed to download file %d: %v", i+1, err)
		}

		if !bytes.Equal(files[i], reconstructed) {
			t.Errorf("Content mismatch for file %d", i+1)
		}
	}

	finalMetrics := client.GetMetrics()

	t.Logf("Concurrent operations metrics:")
	t.Logf("  Files processed: %d", len(files))
	t.Logf("  Upload metrics - Reused: %d, Generated: %d", uploadMetrics.BlocksReused, uploadMetrics.BlocksGenerated)
	t.Logf("  Final cache hit rate: %.2f%%", finalMetrics.CacheHitRate)
	t.Logf("  Total downloads: %d", finalMetrics.TotalDownloads)

	// Verify operations completed successfully
	if finalMetrics.TotalUploads != int64(len(files)) {
		t.Errorf("Expected %d uploads, got %d", len(files), finalMetrics.TotalUploads)
	}

	if finalMetrics.TotalDownloads != int64(len(files)) {
		t.Errorf("Expected %d downloads, got %d", len(files), finalMetrics.TotalDownloads)
	}
}

func TestMultiFileWithCacheEviction(t *testing.T) {
	// Setup with very limited cache to force eviction
	storageManager, err := storagetesting.CreateRealTestStorageManager()
	if err != nil {
		t.Fatalf("Failed to create storage manager: %v", err)
	}
	defer storageManager.Stop(context.Background())
	
	cache := cache.NewMemoryCache(8) // Very small cache
	client, err := noisefs.NewClient(storageManager, cache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Create many small files to trigger cache eviction
	numFiles := 15
	blockSize := 32
	files := make([][]byte, numFiles)
	descriptors := make([]*descriptors.Descriptor, numFiles)

	for i := 0; i < numFiles; i++ {
		content := fmt.Sprintf("File %02d content with unique identifier %d", i+1, i*23)
		files[i] = bytes.Repeat([]byte(content), 5)

		desc, err := simulateUpload(client, files[i], blockSize)
		if err != nil {
			t.Fatalf("Failed to upload file %d: %v", i+1, err)
		}
		desc.Filename = fmt.Sprintf("eviction_test_%02d.txt", i+1)
		descriptors[i] = desc

		// Log metrics periodically
		if (i+1)%5 == 0 {
			metrics := client.GetMetrics()
			t.Logf("After %d uploads - Cache hits: %d, Cache misses: %d", 
				i+1, metrics.CacheHits, metrics.CacheMisses)
		}
	}

	uploadMetrics := client.GetMetrics()

	// Download files in reverse order to test cache behavior
	for i := numFiles - 1; i >= 0; i-- {
		reconstructed, err := simulateDownload(client, descriptors[i])
		if err != nil {
			t.Fatalf("Failed to download file %d: %v", i+1, err)
		}

		if !bytes.Equal(files[i], reconstructed) {
			t.Errorf("Content mismatch for file %d", i+1)
		}
	}

	finalMetrics := client.GetMetrics()

	t.Logf("Cache eviction test results:")
	t.Logf("  Files processed: %d", numFiles)
	t.Logf("  Cache size limit: 8 blocks")
	t.Logf("  Upload phase - Hits: %d, Misses: %d", uploadMetrics.CacheHits, uploadMetrics.CacheMisses)
	t.Logf("  Download phase - Additional hits: %d, Additional misses: %d", 
		finalMetrics.CacheHits-uploadMetrics.CacheHits, finalMetrics.CacheMisses-uploadMetrics.CacheMisses)
	t.Logf("  Final cache hit rate: %.2f%%", finalMetrics.CacheHitRate)

	// Should have cache misses due to eviction
	if finalMetrics.CacheMisses == 0 {
		t.Error("Expected some cache misses due to limited cache size")
	}

	// Should still complete all operations successfully
	if finalMetrics.TotalUploads != int64(numFiles) {
		t.Errorf("Expected %d uploads, got %d", numFiles, finalMetrics.TotalUploads)
	}

	if finalMetrics.TotalDownloads != int64(numFiles) {
		t.Errorf("Expected %d downloads, got %d", numFiles, finalMetrics.TotalDownloads)
	}
}
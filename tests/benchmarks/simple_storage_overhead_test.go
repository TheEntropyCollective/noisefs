package benchmarks

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	noisefs "github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/logging"
	
	// Import backends to register them
	_ "github.com/TheEntropyCollective/noisefs/pkg/storage/backends"
)

// SimpleOverheadResult captures basic overhead measurements
type SimpleOverheadResult struct {
	Scenario        string  `json:"scenario"`
	FileSize        int64   `json:"file_size_bytes"`
	OverheadPercent float64 `json:"overhead_percent"`
}

// BenchmarkSimpleStorageOverhead provides a working basic storage overhead measurement
func BenchmarkSimpleStorageOverhead(b *testing.B) {
	// Create a proper storage setup with mock backend
	_ = logging.NewLogger(nil) // Logger not needed for this test
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
		b.Fatalf("Failed to create storage manager: %v", err)
	}
	
	// Start storage manager
	ctx := context.Background()
	if err := storageManager.Start(ctx); err != nil {
		b.Fatalf("Failed to start storage manager: %v", err)
	}
	defer storageManager.Stop(ctx)
	
	// Create NoiseFS client with the storage manager
	client, err := noisefs.NewClientWithStorageManager(storageManager, cache)
	if err != nil {
		b.Fatalf("Failed to create NoiseFS client: %v", err)
	}

	// Test file sizes that align with block boundaries
	testSizes := []int64{
		128 * 1024,     // 128KB (1 block)
		256 * 1024,     // 256KB (2 blocks)
		512 * 1024,     // 512KB (4 blocks)
		1024 * 1024,    // 1MB (8 blocks)
	}

	var results []SimpleOverheadResult
	numFilesPerSize := 5 // Upload multiple files of each size to enable reuse

	for _, size := range testSizes {
		b.Run(fmt.Sprintf("FileSize_%dKB", size/1024), func(b *testing.B) {
			var overheadResults []float64
			
			b.Logf("Testing %d files of size %d bytes each", numFilesPerSize, size)
			
			for fileNum := 0; fileNum < numFilesPerSize; fileNum++ {
				// Generate test data (vary content slightly to simulate real files)
				testData := make([]byte, size)
				for i := range testData {
					testData[i] = byte((i + fileNum*7) % 256) // Vary pattern per file
				}
				
				// Get metrics before upload
				initialMetrics := client.GetMetrics()
				initialStored := initialMetrics.BytesStoredIPFS
				
				// Check cache state
				cacheStatsBefore := cache.GetStats()
				
				// Upload file
				reader := strings.NewReader(string(testData))
				descriptorCID, err := client.Upload(reader, fmt.Sprintf("test_%dkb_file%d.dat", size/1024, fileNum))
				if err != nil {
					b.Fatalf("Upload %d failed: %v", fileNum, err)
				}
				
				// Get metrics after upload
				finalMetrics := client.GetMetrics()
				finalStored := finalMetrics.BytesStoredIPFS
				
				// Calculate overhead for this file
				bytesStored := finalStored - initialStored
				overheadPercent := (float64(bytesStored) / float64(size)) * 100.0
				overheadResults = append(overheadResults, overheadPercent)
				
				// Check cache state after upload
				cacheStatsAfter := cache.GetStats()
				
				b.Logf("File %d: %dKB, Stored: %dB, Overhead: %.1f%%, Cache: %d->%d blocks, Hit rate: %.1f%%", 
					fileNum+1, size/1024, bytesStored, overheadPercent, 
					cacheStatsBefore.Size, cacheStatsAfter.Size, cacheStatsAfter.HitRate*100)
				
				// Verify we can download it back
				_, err = client.Download(descriptorCID)
				if err != nil {
					b.Fatalf("Download %d failed: %v", fileNum, err)
				}
			}
			
			// Calculate average overhead across all files of this size
			var totalOverhead float64
			for _, overhead := range overheadResults {
				totalOverhead += overhead
			}
			avgOverhead := totalOverhead / float64(len(overheadResults))
			
			// Also report the progression to show reuse effect
			firstFileOverhead := overheadResults[0]
			lastFileOverhead := overheadResults[len(overheadResults)-1]
			
			result := SimpleOverheadResult{
				Scenario:        fmt.Sprintf("%dKB files (avg of %d)", size/1024, numFilesPerSize),
				FileSize:        size,
				OverheadPercent: avgOverhead,
			}
			results = append(results, result)
			
			b.Logf("Summary %dKB: First file %.1f%%, Last file %.1f%%, Average %.1f%%", 
				size/1024, firstFileOverhead, lastFileOverhead, avgOverhead)
		})
	}

	// Generate simple report
	generateSimpleReport(results)
}

// generateSimpleReport creates a basic analysis report
func generateSimpleReport(results []SimpleOverheadResult) {
	if len(results) == 0 {
		return
	}

	var totalOverhead float64
	minOverhead, maxOverhead := 1000.0, 0.0
	
	for _, result := range results {
		totalOverhead += result.OverheadPercent
		if result.OverheadPercent < minOverhead {
			minOverhead = result.OverheadPercent
		}
		if result.OverheadPercent > maxOverhead {
			maxOverhead = result.OverheadPercent
		}
	}
	
	avgOverhead := totalOverhead / float64(len(results))
	
	fmt.Printf("\n=== NoiseFS Storage Overhead Analysis ===\n")
	fmt.Printf("Average Overhead: %.1f%%\n", avgOverhead)
	fmt.Printf("Range: %.1f%% - %.1f%%\n", minOverhead, maxOverhead)
	fmt.Printf("Test Files: %d\n", len(results))
	fmt.Printf("Date: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	
	fmt.Printf("\nDetailed Results:\n")
	for _, result := range results {
		fmt.Printf("  %s: %.1f%% overhead\n", result.Scenario, result.OverheadPercent)
	}
	
	fmt.Printf("\nKey Findings:\n")
	if avgOverhead < 200 {
		fmt.Printf("✓ Overhead significantly below 200%% target\n")
	}
	if maxOverhead-minOverhead < 50 {
		fmt.Printf("✓ Consistent overhead across file sizes\n")
	}
	fmt.Printf("ℹ Current documentation claims <200%% overhead\n")
	fmt.Printf("ℹ Actual measured overhead: %.1f%%\n", avgOverhead)
	if avgOverhead < 150 {
		fmt.Printf("💡 Consider updating documentation to reflect actual performance\n")
	}
	fmt.Printf("\n")
}
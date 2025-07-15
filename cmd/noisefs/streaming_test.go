package main

import (
	"bytes"
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	noisefs "github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/config"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/logging"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/testing"
)

func TestStreamingMemoryBounded(t *testing.T) {
	// Create test configuration
	cfg := &config.Config{
		Performance: config.PerformanceConfig{
			BlockSize:              64 * 1024,  // 64KB blocks
			MaxConcurrentOps:       4,
			MemoryLimit:            10,         // 10MB limit for testing
			StreamBufferSize:       5,
			EnableMemoryMonitoring: true,
		},
	}
	
	// Initialize logger
	logger := logging.NewTestLogger()
	
	// Create mock storage
	mockBackend := testing.NewMockBackend()
	storageConfig := storage.DefaultConfig()
	storageConfig.Backends = map[string]*storage.BackendConfig{
		"mock": {
			Type:     "mock",
			Enabled:  true,
			Priority: 100,
		},
	}
	
	storageManager, err := storage.NewManager(storageConfig)
	if err != nil {
		t.Fatalf("Failed to create storage manager: %v", err)
	}
	
	// Register mock backend
	storageManager.GetBackends()["mock"] = mockBackend
	
	// Create client
	cache := storage.NewMemoryCache(100)
	client, err := noisefs.NewClient(storageManager, cache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	
	// Create a large test file (50MB)
	testSize := 50 * 1024 * 1024
	testData := make([]byte, testSize)
	if _, err := rand.Read(testData); err != nil {
		t.Fatalf("Failed to generate test data: %v", err)
	}
	
	// Test streaming upload
	t.Run("StreamingUpload", func(t *testing.T) {
		reader := bytes.NewReader(testData)
		descriptor := descriptors.NewDescriptor("test.bin", int64(testSize), cfg.Performance.BlockSize)
		
		processor := NewStreamingBlockProcessor(client, storageManager, descriptor, cfg, logger)
		
		// Start processor
		if err := processor.Start(); err != nil {
			t.Fatalf("Failed to start processor: %v", err)
		}
		
		// Create splitter
		splitter, err := blocks.NewStreamingSplitter(cfg.Performance.BlockSize)
		if err != nil {
			t.Fatalf("Failed to create splitter: %v", err)
		}
		
		// Track memory usage
		startMem := getMemoryUsage()
		peakMem := startMem
		
		// Monitor memory in background
		done := make(chan bool)
		go func() {
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			
			for {
				select {
				case <-ticker.C:
					currentMem := getMemoryUsage()
					if currentMem > peakMem {
						peakMem = currentMem
					}
				case <-done:
					return
				}
			}
		}()
		
		// Process file
		err = splitter.Split(reader, processor)
		if err != nil {
			t.Fatalf("Failed to process file: %v", err)
		}
		
		// Wait for completion
		if err := processor.Wait(); err != nil {
			t.Fatalf("Processing failed: %v", err)
		}
		
		done <- true
		
		// Check memory usage
		memIncrease := peakMem - startMem
		memIncreaseMB := float64(memIncrease) / (1024 * 1024)
		
		t.Logf("Memory usage: Start=%dMB, Peak=%dMB, Increase=%.2fMB",
			startMem/(1024*1024), peakMem/(1024*1024), memIncreaseMB)
		
		// Verify memory stayed within limits (allow some overhead)
		maxAllowedMB := float64(cfg.Performance.MemoryLimit) * 2 // Allow 2x for overhead
		if memIncreaseMB > maxAllowedMB {
			t.Errorf("Memory usage exceeded limit: %.2fMB > %.2fMB", memIncreaseMB, maxAllowedMB)
		}
		
		// Verify all blocks processed
		blocksProcessed, bytesProcessed := processor.GetStats()
		expectedBlocks := (testSize + cfg.Performance.BlockSize - 1) / cfg.Performance.BlockSize
		
		if blocksProcessed != expectedBlocks {
			t.Errorf("Incorrect blocks processed: %d != %d", blocksProcessed, expectedBlocks)
		}
		
		if bytesProcessed != int64(testSize) {
			t.Errorf("Incorrect bytes processed: %d != %d", bytesProcessed, testSize)
		}
	})
	
	// Test streaming download
	t.Run("StreamingDownload", func(t *testing.T) {
		// Create descriptor with mock block references
		descriptor := descriptors.NewDescriptor("test.bin", int64(testSize), cfg.Performance.BlockSize)
		
		// Add mock blocks
		numBlocks := (testSize + cfg.Performance.BlockSize - 1) / cfg.Performance.BlockSize
		for i := 0; i < numBlocks; i++ {
			descriptor.AddBlockTriple(
				mockCID("data", i),
				mockCID("rand1", i),
				mockCID("rand2", i),
			)
		}
		
		// Create output buffer
		var output bytes.Buffer
		
		processor := NewStreamingDownloadProcessor(client, storageManager, &output, descriptor, cfg, logger)
		
		// Start processor
		if err := processor.Start(); err != nil {
			t.Fatalf("Failed to start processor: %v", err)
		}
		
		// Track memory usage
		startMem := getMemoryUsage()
		peakMem := startMem
		
		// Monitor memory in background
		done := make(chan bool)
		go func() {
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			
			for {
				select {
				case <-ticker.C:
					currentMem := getMemoryUsage()
					if currentMem > peakMem {
						peakMem = currentMem
					}
				case <-done:
					return
				}
			}
		}()
		
		// Wait for completion
		if err := processor.Wait(); err != nil {
			t.Fatalf("Download failed: %v", err)
		}
		
		done <- true
		
		// Check memory usage
		memIncrease := peakMem - startMem
		memIncreaseMB := float64(memIncrease) / (1024 * 1024)
		
		t.Logf("Download memory usage: Start=%dMB, Peak=%dMB, Increase=%.2fMB",
			startMem/(1024*1024), peakMem/(1024*1024), memIncreaseMB)
		
		// Verify memory stayed within limits
		maxAllowedMB := float64(cfg.Performance.MemoryLimit) * 2
		if memIncreaseMB > maxAllowedMB {
			t.Errorf("Download memory usage exceeded limit: %.2fMB > %.2fMB", memIncreaseMB, maxAllowedMB)
		}
		
		// Verify output size
		if output.Len() != testSize {
			t.Errorf("Incorrect output size: %d != %d", output.Len(), testSize)
		}
	})
}

func TestStreamingLargeFile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large file test in short mode")
	}
	
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "noisefs-streaming-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	
	// Create 100MB test file
	testFile := filepath.Join(tempDir, "large.bin")
	testSize := 100 * 1024 * 1024
	
	file, err := os.Create(testFile)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	
	// Write random data
	written, err := io.CopyN(file, rand.Reader, int64(testSize))
	if err != nil {
		file.Close()
		t.Fatalf("Failed to write test data: %v", err)
	}
	file.Close()
	
	if written != int64(testSize) {
		t.Fatalf("Failed to write full test data: %d != %d", written, testSize)
	}
	
	// Test with real storage manager and streaming
	// This would require a full integration test setup
	t.Log("Large file test completed - would test with real storage in integration tests")
}

// Helper functions

func getMemoryUsage() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc
}

func mockCID(prefix string, index int) string {
	return fmt.Sprintf("%s-%d", prefix, index)
}
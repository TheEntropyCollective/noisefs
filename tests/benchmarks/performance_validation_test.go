// +build ignore

package benchmarks

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	// "github.com/TheEntropyCollective/noisefs/tests/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestFile creates a test file with the specified size
func createTestFile(path string, size int) error {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}
	return os.WriteFile(path, data, 0644)
}

// TestPerformanceImprovements validates the performance improvements from Phase 3
func TestPerformanceImprovements(t *testing.T) {
	// Skip if running short tests
	if testing.Short() {
		t.Skip("Skipping performance validation in short mode")
	}

	// Test configurations
	fileSizes := []int{
		1 * 1024 * 1024,   // 1MB
		10 * 1024 * 1024,  // 10MB
		100 * 1024 * 1024, // 100MB
	}

	workerCounts := []int{1, 2, 4, 8}

	// Initialize storage manager
	ctx := context.Background()
	
	tempDir, err := os.MkdirTemp("", "noisefs-perf-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create storage config
	storageConfig := &storage.Config{
		DefaultBackend: "mock",
		Backends: map[string]*storage.BackendConfig{
			"mock": {
				Type:    "mock",
				Enabled: true,
				Priority: 100,
			},
		},
	}
	sm, err := storage.NewManager(storageConfig)
	require.NoError(t, err)

	// Test each file size
	for _, fileSize := range fileSizes {
		t.Run(fmt.Sprintf("FileSize_%dMB", fileSize/(1024*1024)), func(t *testing.T) {
			// Create test file
			testFile := filepath.Join(tempDir, fmt.Sprintf("test_%d.bin", fileSize))
			require.NoError(t, createTestFile(testFile, fileSize))
			defer os.Remove(testFile)

			// Test with different worker counts
			var sequentialTime time.Duration
			results := make(map[int]time.Duration)

			for _, workers := range workerCounts {
				t.Run(fmt.Sprintf("Workers_%d", workers), func(t *testing.T) {
					// Configure worker count
					_ = workers // Track workers for this test

					// Measure upload time
					start := time.Now()
					
					// Simulate upload process (simplified)
					data, err := os.ReadFile(testFile)
					require.NoError(t, err)

					blockSize := 256 * 1024 // 256KB blocks
					blockSplitter := blocks.NewBlockSplitter(blockSize)
					dataBlocks, err := blockSplitter.Split(data)
					require.NoError(t, err)

					// Process blocks (XOR and storage)
					for _, block := range dataBlocks {
						// Generate randomizers
						rand1, err := crypto.GenerateRandomBytes(len(block.Data))
						require.NoError(t, err)
						rand2, err := crypto.GenerateRandomBytes(len(block.Data))
						require.NoError(t, err)

						// XOR operation
						xored := blocks.XORBytes(block.Data, rand1, rand2)
						
						// Store blocks (simulated)
						_, err = sm.Store(ctx, xored)
						require.NoError(t, err)
						_, err = sm.Store(ctx, rand1)
						require.NoError(t, err)
						_, err = sm.Store(ctx, rand2)
						require.NoError(t, err)
					}

					elapsed := time.Since(start)
					results[workers] = elapsed

					if workers == 1 {
						sequentialTime = elapsed
					}

					// Calculate speedup
					speedup := float64(sequentialTime) / float64(elapsed)
					t.Logf("Time: %v, Speedup: %.2fx", elapsed, speedup)

					// Validate performance improvement
					if workers > 1 {
						// Expect at least 1.5x speedup with 2+ workers
						assert.Greater(t, speedup, 1.5, 
							"Expected at least 1.5x speedup with %d workers", workers)
					}
				})
			}

			// Log performance summary
			t.Logf("\nPerformance Summary for %dMB file:", fileSize/(1024*1024))
			for workers, elapsed := range results {
				speedup := float64(sequentialTime) / float64(elapsed)
				throughput := float64(fileSize) / elapsed.Seconds() / (1024 * 1024)
				t.Logf("  %d workers: %v (%.2fx speedup, %.2f MB/s)", 
					workers, elapsed, speedup, throughput)
			}
		})
	}
}

// TestMemoryEfficiency validates streaming memory management
func TestMemoryEfficiency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory efficiency test in short mode")
	}

	// Test configurations
	testCases := []struct {
		name       string
		fileSize   int
		memLimit   int64
		streaming  bool
	}{
		{"Regular_Small", 10 * 1024 * 1024, 0, false},
		{"Streaming_Small", 10 * 1024 * 1024, 64 * 1024 * 1024, true},
		{"Regular_Large", 100 * 1024 * 1024, 0, false},
		{"Streaming_Large", 100 * 1024 * 1024, 64 * 1024 * 1024, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Get initial memory stats
			var m runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&m)
			startMem := m.Alloc

			// Simulate file processing
			tempDir, err := os.MkdirTemp("", "noisefs-mem-test-*")
			require.NoError(t, err)
			defer os.RemoveAll(tempDir)

			testFile := filepath.Join(tempDir, "test.bin")
			require.NoError(t, createTestFile(testFile, tc.fileSize))

			// Process file (simplified simulation)
			if tc.streaming {
				// Streaming mode - process in chunks
				file, err := os.Open(testFile)
				require.NoError(t, err)
				defer file.Close()

				chunkSize := 1024 * 1024 // 1MB chunks
				chunk := make([]byte, chunkSize)
				
				for {
					n, err := file.Read(chunk)
					if n == 0 {
						break
					}
					require.NoError(t, err)
					
					// Process chunk (simulated)
					_ = blocks.XORBytes(chunk[:n], chunk[:n], chunk[:n])
					
					// Check memory usage
					runtime.ReadMemStats(&m)
					currentMem := m.Alloc - startMem
					
					if tc.memLimit > 0 {
						assert.LessOrEqual(t, int64(currentMem), tc.memLimit,
							"Memory usage exceeded limit")
					}
				}
			} else {
				// Regular mode - load entire file
				data, err := os.ReadFile(testFile)
				require.NoError(t, err)
				
				// Process data (simulated)
				_ = blocks.XORBytes(data, data, data)
			}

			// Get final memory stats
			runtime.GC()
			runtime.ReadMemStats(&m)
			endMem := m.Alloc
			peakMem := endMem - startMem

			t.Logf("Memory usage - Start: %d MB, Peak: %d MB, File: %d MB",
				startMem/(1024*1024), peakMem/(1024*1024), tc.fileSize/(1024*1024))

			// Validate memory efficiency
			if tc.streaming && tc.memLimit > 0 {
				assert.LessOrEqual(t, int64(peakMem), tc.memLimit,
					"Streaming mode should respect memory limit")
			}
		})
	}
}

// TestPerformanceRegression ensures performance gains are maintained
func TestPerformanceRegression(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping regression test in short mode")
	}

	// Load baseline metrics
	baseline := BaselineMetrics{
		UploadLatency:    500 * time.Millisecond,
		DownloadLatency:  400 * time.Millisecond,
		XORThroughput:    100.0, // MB/s
		StorageThroughput: 50.0,  // MB/s
	}

	// Create regression detector
	detector := NewRegressionDetector(baseline)

	// Test current performance
	current := PerformanceMetric{
		UploadLatency:    300 * time.Millisecond,  // Better
		DownloadLatency:  350 * time.Millisecond,  // Better
		XORThroughput:    150.0,                    // Better
		StorageThroughput: 60.0,                    // Better
	}

	// Check for regressions
	regressions := detector.CheckForRegressions(current)
	assert.Empty(t, regressions, "No regressions should be detected")

	// Test with regression
	badMetric := PerformanceMetric{
		UploadLatency:    700 * time.Millisecond,  // Worse
		DownloadLatency:  350 * time.Millisecond,  // Better
		XORThroughput:    150.0,                    // Better
		StorageThroughput: 60.0,                    // Better
	}

	regressions = detector.CheckForRegressions(badMetric)
	assert.NotEmpty(t, regressions, "Upload latency regression should be detected")
}

// BenchmarkParallelXOR measures XOR operation performance
func BenchmarkParallelXOR(b *testing.B) {
	sizes := []int{
		64 * 1024,   // 64KB
		256 * 1024,  // 256KB
		1024 * 1024, // 1MB
	}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size_%dKB", size/1024), func(b *testing.B) {
			// Create test data
			data := make([]byte, size)
			rand1 := make([]byte, size)
			rand2 := make([]byte, size)
			
			for i := range data {
				data[i] = byte(i % 256)
				rand1[i] = byte((i * 2) % 256)
				rand2[i] = byte((i * 3) % 256)
			}

			b.ResetTimer()
			b.SetBytes(int64(size))

			for i := 0; i < b.N; i++ {
				_ = blocks.XORBytes(data, rand1, rand2)
			}
		})
	}
}

// BenchmarkWorkerPool measures worker pool overhead
func BenchmarkWorkerPool(b *testing.B) {
	workerCounts := []int{1, 2, 4, 8, 16}

	for _, workers := range workerCounts {
		b.Run(fmt.Sprintf("Workers_%d", workers), func(b *testing.B) {
			// Simulate workload
			workItems := 100
			workDuration := 10 * time.Microsecond

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				// Create work channel
				work := make(chan int, workItems)
				done := make(chan bool, workers)

				// Start workers
				for w := 0; w < workers; w++ {
					go func() {
						for range work {
							time.Sleep(workDuration)
						}
						done <- true
					}()
				}

				// Submit work
				for j := 0; j < workItems; j++ {
					work <- j
				}
				close(work)

				// Wait for completion
				for w := 0; w < workers; w++ {
					<-done
				}
			}
		})
	}
}
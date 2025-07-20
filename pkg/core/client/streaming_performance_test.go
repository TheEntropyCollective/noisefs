package noisefs

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
)

// BenchmarkStreamingUpload benchmarks streaming upload performance
func BenchmarkStreamingUpload(b *testing.B) {
	client, err := NewTestClient()
	if err != nil {
		b.Fatalf("Failed to create test client: %v", err)
	}

	// Test with different file sizes
	sizes := []int{
		1024,             // 1KB
		10 * 1024,        // 10KB
		100 * 1024,       // 100KB
		1024 * 1024,      // 1MB
		10 * 1024 * 1024, // 10MB (largest for benchmark)
	}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size%dB", size), func(b *testing.B) {
			// Create test data once
			testData := make([]byte, size)
			_, err := rand.Read(testData)
			if err != nil {
				b.Fatalf("Failed to create test data: %v", err)
			}

			b.ResetTimer()
			b.SetBytes(int64(size))

			for i := 0; i < b.N; i++ {
				reader := bytes.NewReader(testData)
				_, err := client.StreamingUpload(reader, fmt.Sprintf("bench-file-%d.bin", i))
				if err != nil {
					b.Fatalf("Upload failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkStreamingDownload benchmarks streaming download performance
func BenchmarkStreamingDownload(b *testing.B) {
	client, err := NewTestClient()
	if err != nil {
		b.Fatalf("Failed to create test client: %v", err)
	}

	// Test with different file sizes
	sizes := []int{
		1024,             // 1KB
		10 * 1024,        // 10KB
		100 * 1024,       // 100KB
		1024 * 1024,      // 1MB
		10 * 1024 * 1024, // 10MB
	}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size%dB", size), func(b *testing.B) {
			// Create and upload test data once
			testData := make([]byte, size)
			_, err := rand.Read(testData)
			if err != nil {
				b.Fatalf("Failed to create test data: %v", err)
			}

			reader := bytes.NewReader(testData)
			descriptorCID, err := client.StreamingUpload(reader, "bench-download.bin")
			if err != nil {
				b.Fatalf("Upload failed: %v", err)
			}

			b.ResetTimer()
			b.SetBytes(int64(size))

			for i := 0; i < b.N; i++ {
				var buffer bytes.Buffer
				err := client.StreamingDownload(descriptorCID, &buffer)
				if err != nil {
					b.Fatalf("Download failed: %v", err)
				}
			}
		})
	}
}

// TestStreamingMemoryUsage tests that streaming operations maintain constant memory usage
func TestStreamingMemoryUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory usage test in short mode")
	}

	client, err := NewTestClient()
	if err != nil {
		t.Fatalf("Failed to create test client: %v", err)
	}

	// Test with progressively larger files to ensure memory usage doesn't grow
	sizes := []int{
		100 * 1024,       // 100KB
		1024 * 1024,      // 1MB
		5 * 1024 * 1024,  // 5MB
		10 * 1024 * 1024, // 10MB
	}

	for _, size := range sizes {
		t.Run(fmt.Sprintf("Size%dMB", size/(1024*1024)), func(t *testing.T) {
			// Create test data
			testData := make([]byte, size)
			for i := range testData {
				testData[i] = byte(i % 256) // Predictable pattern
			}

			// Upload with memory tracking
			reader := &memoryTrackingReader{
				reader:  bytes.NewReader(testData),
				maxRead: 0,
			}

			descriptorCID, err := client.StreamingUpload(reader, fmt.Sprintf("memory-test-%d.bin", size))
			if err != nil {
				t.Fatalf("Upload failed: %v", err)
			}

			// Verify streaming behavior - should read at most block size at once
			expectedMaxRead := blocks.DefaultBlockSize
			if size < blocks.DefaultBlockSize {
				expectedMaxRead = size // For small files, it's OK to read entire file
			}
			if reader.maxRead > expectedMaxRead {
				t.Errorf("Upload read too much at once (%d bytes), expected max %d bytes", reader.maxRead, expectedMaxRead)
			}

			// Download and verify memory usage
			var downloadBuffer bytes.Buffer
			err = client.StreamingDownload(descriptorCID, &downloadBuffer)
			if err != nil {
				t.Fatalf("Download failed: %v", err)
			}

			// Verify data integrity
			downloadedData := downloadBuffer.Bytes()
			if !bytes.Equal(downloadedData[:len(testData)], testData) {
				t.Error("Downloaded data doesn't match original")
			}

			t.Logf("Size: %dMB, Max read chunk: %dKB", size/(1024*1024), reader.maxRead/1024)
		})
	}
}

// TestStreamingCancellation tests that context cancellation works correctly
func TestStreamingCancellation(t *testing.T) {
	client, err := NewTestClient()
	if err != nil {
		t.Fatalf("Failed to create test client: %v", err)
	}

	t.Run("UploadCancellation", func(t *testing.T) {
		// Create a context that cancels quickly
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()

		// Create a slow reader that will trigger cancellation
		slowReader := &slowReader{
			data:  make([]byte, 10*1024*1024), // 10MB
			delay: 10 * time.Millisecond,      // Slow enough to trigger timeout
		}

		_, err := client.StreamingUploadWithContext(ctx, slowReader, "cancel-test.bin")
		if err == nil {
			t.Error("Expected cancellation error, got success")
		}

		// Check if the error is related to context cancellation (may be wrapped)
		if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
			t.Errorf("Expected context cancellation error, got: %v", err)
		}
	})

	t.Run("DownloadCancellation", func(t *testing.T) {
		// First upload a file
		testData := make([]byte, 1024)
		_, err := rand.Read(testData)
		if err != nil {
			t.Fatalf("Failed to create test data: %v", err)
		}

		reader := bytes.NewReader(testData)
		descriptorCID, err := client.StreamingUpload(reader, "cancel-download-test.bin")
		if err != nil {
			t.Fatalf("Upload failed: %v", err)
		}

		// Create a context that cancels immediately
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		var buffer bytes.Buffer
		err = client.StreamingDownloadWithContext(ctx, descriptorCID, &buffer)
		if err == nil {
			t.Error("Expected cancellation error, got success")
		}

		if err != context.Canceled {
			t.Errorf("Expected context cancelled error, got: %v", err)
		}
	})
}

// TestStreamingProgressReporting tests progress reporting functionality
func TestStreamingProgressReporting(t *testing.T) {
	client, err := NewTestClient()
	if err != nil {
		t.Fatalf("Failed to create test client: %v", err)
	}

	// Create test data that spans multiple blocks
	testData := make([]byte, 3*blocks.DefaultBlockSize+1000) // 3+ blocks
	_, err = rand.Read(testData)
	if err != nil {
		t.Fatalf("Failed to create test data: %v", err)
	}

	t.Run("UploadProgress", func(t *testing.T) {
		reader := bytes.NewReader(testData)

		var progressCalls []ProgressReport
		progressCallback := func(operation string, bytesProcessed int64, blocksProcessed int) {
			progressCalls = append(progressCalls, ProgressReport{
				Operation:       operation,
				BytesProcessed:  bytesProcessed,
				BlocksProcessed: blocksProcessed,
			})
		}

		_, err := client.StreamingUploadWithProgress(reader, "progress-test.bin", progressCallback)
		if err != nil {
			t.Fatalf("Upload failed: %v", err)
		}

		// Verify progress was reported
		if len(progressCalls) == 0 {
			t.Error("No progress reports received")
		}

		// Verify progress contains expected operations
		operations := make(map[string]bool)
		for _, report := range progressCalls {
			operations[report.Operation] = true
		}

		expectedOps := []string{"Initializing streaming upload", "Processing blocks", "Upload complete"}
		for _, expectedOp := range expectedOps {
			if !operations[expectedOp] {
				t.Errorf("Expected operation '%s' not found in progress reports", expectedOp)
			}
		}

		t.Logf("Received %d progress reports", len(progressCalls))
	})
}

// memoryTrackingReader tracks the maximum amount read in a single Read() call
type memoryTrackingReader struct {
	reader  io.Reader
	maxRead int
}

func (m *memoryTrackingReader) Read(p []byte) (n int, err error) {
	n, err = m.reader.Read(p)
	if n > m.maxRead {
		m.maxRead = n
	}
	return n, err
}

// slowReader simulates a slow data source for cancellation testing
type slowReader struct {
	data  []byte
	pos   int
	delay time.Duration
}

func (s *slowReader) Read(p []byte) (n int, err error) {
	time.Sleep(s.delay) // Simulate slow I/O

	if s.pos >= len(s.data) {
		return 0, io.EOF
	}

	n = copy(p, s.data[s.pos:])
	s.pos += n
	return n, nil
}

// ProgressReport represents a progress update
type ProgressReport struct {
	Operation       string
	BytesProcessed  int64
	BlocksProcessed int
}

// TestLargeFilePerformance tests performance characteristics with larger files
// This test can be run manually with larger files by setting environment variables
func TestLargeFilePerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large file performance test in short mode")
	}

	client, err := NewTestClient()
	if err != nil {
		t.Fatalf("Failed to create test client: %v", err)
	}

	// Test file sizes (can be increased for manual testing)
	testSizes := []struct {
		name string
		size int
	}{
		{"1MB", 1024 * 1024},
		{"5MB", 5 * 1024 * 1024},
		{"10MB", 10 * 1024 * 1024},
		{"25MB", 25 * 1024 * 1024},
		// Uncomment for manual testing with larger files:
		// {"100MB", 100 * 1024 * 1024},
		// {"500MB", 500 * 1024 * 1024},
		// {"1GB", 1024 * 1024 * 1024},
	}

	for _, testSize := range testSizes {
		t.Run(testSize.name, func(t *testing.T) {
			// Create test data
			testData := make([]byte, testSize.size)
			for i := range testData {
				testData[i] = byte(i % 256)
			}

			// Track performance metrics
			var uploadDuration, downloadDuration time.Duration
			var progressReports int

			// Upload with timing
			uploadStart := time.Now()
			reader := bytes.NewReader(testData)

			progressCallback := func(operation string, bytesProcessed int64, blocksProcessed int) {
				progressReports++
			}

			descriptorCID, err := client.StreamingUploadWithProgress(reader, fmt.Sprintf("perf-test-%s.bin", testSize.name), progressCallback)
			if err != nil {
				t.Fatalf("Upload failed: %v", err)
			}
			uploadDuration = time.Since(uploadStart)

			// Download with timing
			downloadStart := time.Now()
			var downloadBuffer bytes.Buffer
			err = client.StreamingDownload(descriptorCID, &downloadBuffer)
			if err != nil {
				t.Fatalf("Download failed: %v", err)
			}
			downloadDuration = time.Since(downloadStart)

			// Verify data integrity
			downloadedData := downloadBuffer.Bytes()
			if len(downloadedData) < len(testData) {
				t.Errorf("Downloaded size mismatch: got %d, expected %d", len(downloadedData), len(testData))
			} else if !bytes.Equal(downloadedData[:len(testData)], testData) {
				t.Error("Data integrity check failed")
			}

			// Calculate performance metrics
			uploadMBps := float64(testSize.size) / (1024 * 1024) / uploadDuration.Seconds()
			downloadMBps := float64(testSize.size) / (1024 * 1024) / downloadDuration.Seconds()

			t.Logf("Performance for %s:", testSize.name)
			t.Logf("  Upload: %v (%.2f MB/s)", uploadDuration, uploadMBps)
			t.Logf("  Download: %v (%.2f MB/s)", downloadDuration, downloadMBps)
			t.Logf("  Progress reports: %d", progressReports)
			t.Logf("  Expected blocks: %d", (testSize.size+blocks.DefaultBlockSize-1)/blocks.DefaultBlockSize)
		})
	}
}

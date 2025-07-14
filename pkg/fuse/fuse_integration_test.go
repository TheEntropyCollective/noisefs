package fuse

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	storagetesting "github.com/TheEntropyCollective/noisefs/pkg/storage/testing"
	noisefs "github.com/TheEntropyCollective/noisefs/pkg/core/client"
)

// TestFuseIntegration tests the FUSE filesystem with real IPFS integration
func TestFuseIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping FUSE integration test in short mode")
	}
	
	// Check if FUSE is available
	if _, err := os.Stat("/dev/fuse"); err != nil {
		t.Skip("Skipping FUSE test: /dev/fuse not available")
	}

	// Create temporary mount point
	mountDir, err := os.MkdirTemp("", "noisefs_test_mount")
	if err != nil {
		t.Fatalf("Failed to create temp mount dir: %v", err)
	}
	defer os.RemoveAll(mountDir)

	// Create temporary index file
	indexFile := filepath.Join(mountDir, "test_index.json")

	// Setup storage manager and NoiseFS client
	storageManager, err := storagetesting.CreateRealTestStorageManager()
	if err != nil {
		t.Skipf("Skipping FUSE test - storage manager setup failed: %v", err)
	}
	defer storageManager.Stop(context.Background())

	// Create cache and NoiseFS client
	blockCache := cache.NewMemoryCache(100)
	client, err := noisefs.NewClient(storageManager, blockCache)
	if err != nil {
		t.Fatalf("Failed to create NoiseFS client: %v", err)
	}

	// Mount options
	opts := MountOptions{
		MountPath:  mountDir,
		VolumeName: "test_noisefs",
		ReadOnly:   false,
		AllowOther: false,
		Debug:      testing.Verbose(),
	}

	// Start mount in background
	mountErr := make(chan error, 1)
	go func() {
		err := MountWithIndex(client, storageManager, opts, indexFile)
		mountErr <- err
	}()

	// Wait for mount to be ready
	time.Sleep(2 * time.Second)

	// Check if mount failed
	select {
	case err := <-mountErr:
		if err != nil {
			t.Fatalf("Mount failed: %v", err)
		}
	default:
		// Mount is still running, continue with tests
	}

	// Run filesystem tests
	t.Run("BasicFileOperations", func(t *testing.T) {
		testBasicFileOperations(t, mountDir)
	})

	t.Run("DirectoryOperations", func(t *testing.T) {
		testDirectoryOperations(t, mountDir)
	})

	t.Run("ConcurrentAccess", func(t *testing.T) {
		testConcurrentAccess(t, mountDir)
	})

	t.Run("LargeFiles", func(t *testing.T) {
		testLargeFiles(t, mountDir)
	})

	t.Run("FileAttributes", func(t *testing.T) {
		testFileAttributes(t, mountDir)
	})

	// Unmount
	if err := Unmount(mountDir); err != nil {
		t.Errorf("Failed to unmount: %v", err)
	}

	// Check if mount process exited
	select {
	case err := <-mountErr:
		if err != nil {
			t.Logf("Mount process exited with error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Log("Mount process still running after unmount")
	}
}

// testBasicFileOperations tests basic file read/write operations
func testBasicFileOperations(t *testing.T, mountDir string) {
	testFile := filepath.Join(mountDir, "test_file.txt")
	testContent := "Hello, NoiseFS! This is a test file."

	// Write file
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Read file
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read test file: %v", err)
	}

	if string(content) != testContent {
		t.Errorf("File content mismatch: expected %q, got %q", testContent, string(content))
	}

	// Check file exists
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Errorf("File should exist after writing")
	}

	// Delete file
	if err := os.Remove(testFile); err != nil {
		t.Fatalf("Failed to delete test file: %v", err)
	}

	// Check file is gone
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Errorf("File should not exist after deletion")
	}
}

// testDirectoryOperations tests directory creation and listing
func testDirectoryOperations(t *testing.T, mountDir string) {
	testDir := filepath.Join(mountDir, "test_directory")

	// Create directory
	if err := os.Mkdir(testDir, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	// Check directory exists
	if info, err := os.Stat(testDir); err != nil {
		t.Fatalf("Directory should exist: %v", err)
	} else if !info.IsDir() {
		t.Errorf("Should be a directory")
	}

	// Create file in directory
	testFile := filepath.Join(testDir, "nested_file.txt")
	testContent := "This is a nested file."

	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to write nested file: %v", err)
	}

	// List directory contents
	entries, err := os.ReadDir(testDir)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("Expected 1 entry in directory, got %d", len(entries))
	}

	if entries[0].Name() != "nested_file.txt" {
		t.Errorf("Expected file name 'nested_file.txt', got %q", entries[0].Name())
	}

	// Remove directory and contents
	if err := os.RemoveAll(testDir); err != nil {
		t.Fatalf("Failed to remove directory: %v", err)
	}
}

// testConcurrentAccess tests concurrent file operations
func testConcurrentAccess(t *testing.T, mountDir string) {
	const numWorkers = 10
	const numFiles = 5

	var wg sync.WaitGroup
	errors := make(chan error, numWorkers)

	// Concurrent file writes
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < numFiles; j++ {
				filename := fmt.Sprintf("worker_%d_file_%d.txt", workerID, j)
				filepath := filepath.Join(mountDir, filename)
				content := fmt.Sprintf("Worker %d, File %d content", workerID, j)

				if err := os.WriteFile(filepath, []byte(content), 0644); err != nil {
					errors <- fmt.Errorf("worker %d failed to write file %d: %w", workerID, j, err)
					return
				}

				// Read back to verify
				readContent, err := os.ReadFile(filepath)
				if err != nil {
					errors <- fmt.Errorf("worker %d failed to read file %d: %w", workerID, j, err)
					return
				}

				if string(readContent) != content {
					errors <- fmt.Errorf("worker %d file %d content mismatch", workerID, j)
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent access error: %v", err)
	}

	// Cleanup
	entries, err := os.ReadDir(mountDir)
	if err != nil {
		t.Fatalf("Failed to read mount directory: %v", err)
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "worker_") {
			if err := os.Remove(filepath.Join(mountDir, entry.Name())); err != nil {
				t.Errorf("Failed to cleanup file %s: %v", entry.Name(), err)
			}
		}
	}
}

// testLargeFiles tests handling of large files
func testLargeFiles(t *testing.T, mountDir string) {
	testFile := filepath.Join(mountDir, "large_file.txt")
	
	// Create a 1MB file
	const fileSize = 1024 * 1024
	content := make([]byte, fileSize)
	for i := range content {
		content[i] = byte(i % 256)
	}

	// Write large file
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("Failed to write large file: %v", err)
	}

	// Check file size
	info, err := os.Stat(testFile)
	if err != nil {
		t.Fatalf("Failed to stat large file: %v", err)
	}

	if info.Size() != fileSize {
		t.Errorf("File size mismatch: expected %d, got %d", fileSize, info.Size())
	}

	// Read and verify content
	readContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read large file: %v", err)
	}

	if len(readContent) != fileSize {
		t.Errorf("Read content size mismatch: expected %d, got %d", fileSize, len(readContent))
	}

	// Verify content integrity
	for i, b := range readContent {
		if b != byte(i%256) {
			t.Errorf("Content mismatch at position %d: expected %d, got %d", i, byte(i%256), b)
			break
		}
	}

	// Test streaming read
	file, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("Failed to open large file for streaming: %v", err)
	}
	defer file.Close()

	buffer := make([]byte, 4096)
	totalRead := 0
	for {
		n, err := file.Read(buffer)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Failed to read chunk: %v", err)
		}
		totalRead += n
	}

	if totalRead != fileSize {
		t.Errorf("Streaming read size mismatch: expected %d, got %d", fileSize, totalRead)
	}

	// Cleanup
	if err := os.Remove(testFile); err != nil {
		t.Errorf("Failed to cleanup large file: %v", err)
	}
}

// testFileAttributes tests file attribute operations
func testFileAttributes(t *testing.T, mountDir string) {
	testFile := filepath.Join(mountDir, "attr_test.txt")
	testContent := "Testing file attributes"

	// Create file
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Get file info
	info, err := os.Stat(testFile)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	// Check basic attributes
	if info.Size() != int64(len(testContent)) {
		t.Errorf("File size mismatch: expected %d, got %d", len(testContent), info.Size())
	}

	if info.IsDir() {
		t.Errorf("File should not be a directory")
	}

	// Check permissions
	expectedMode := os.FileMode(0644)
	if info.Mode().Perm() != expectedMode {
		t.Errorf("File permissions mismatch: expected %o, got %o", expectedMode, info.Mode().Perm())
	}

	// Check timestamps
	now := time.Now()
	if info.ModTime().After(now) {
		t.Errorf("File modification time is in the future")
	}

	if info.ModTime().Before(now.Add(-time.Minute)) {
		t.Errorf("File modification time is too old")
	}

	// Cleanup
	if err := os.Remove(testFile); err != nil {
		t.Errorf("Failed to cleanup test file: %v", err)
	}
}

// BenchmarkFuseFileOperations benchmarks basic file operations
func BenchmarkFuseFileOperations(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping FUSE benchmark in short mode")
	}

	// Create temporary mount point
	mountDir, err := os.MkdirTemp("", "noisefs_bench_mount")
	if err != nil {
		b.Fatalf("Failed to create temp mount dir: %v", err)
	}
	defer os.RemoveAll(mountDir)

	// Setup storage manager
	storageManager, err := storagetesting.CreateRealTestStorageManager()
	if err != nil {
		b.Skipf("Skipping FUSE benchmark - storage manager setup failed: %v", err)
	}
	defer storageManager.Stop(context.Background())

	// Create cache and NoiseFS client
	blockCache := cache.NewMemoryCache(100)
	client, err := noisefs.NewClient(storageManager, blockCache)
	if err != nil {
		b.Fatalf("Failed to create NoiseFS client: %v", err)
	}

	// Mount options
	opts := MountOptions{
		MountPath:  mountDir,
		VolumeName: "bench_noisefs",
		ReadOnly:   false,
		AllowOther: false,
		Debug:      false,
	}

	// Start mount in background
	mountErr := make(chan error, 1)
	go func() {
		err := MountWithIndex(client, storageManager, opts, "")
		mountErr <- err
	}()

	// Wait for mount to be ready
	time.Sleep(2 * time.Second)

	// Run benchmarks
	b.Run("WriteSmallFile", func(b *testing.B) {
		content := []byte("Small file content for benchmarking")
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			filename := fmt.Sprintf("bench_small_%d.txt", i)
			filepath := filepath.Join(mountDir, filename)
			
			if err := os.WriteFile(filepath, content, 0644); err != nil {
				b.Fatalf("Failed to write file: %v", err)
			}
		}
	})

	b.Run("ReadSmallFile", func(b *testing.B) {
		// Setup test file
		testFile := filepath.Join(mountDir, "bench_read_test.txt")
		content := []byte("Content for read benchmarking")
		if err := os.WriteFile(testFile, content, 0644); err != nil {
			b.Fatalf("Failed to create test file: %v", err)
		}

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			_, err := os.ReadFile(testFile)
			if err != nil {
				b.Fatalf("Failed to read file: %v", err)
			}
		}
	})

	// Unmount
	if err := Unmount(mountDir); err != nil {
		b.Errorf("Failed to unmount: %v", err)
	}
}
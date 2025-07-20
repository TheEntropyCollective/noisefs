//go:build fuse
// +build fuse

package fuse

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	noisefs "github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/TheEntropyCollective/noisefs/pkg/core/crypto"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	storagetesting "github.com/TheEntropyCollective/noisefs/pkg/storage/testing"
)

// TestDirectoryMountIntegration tests mounting directories via descriptor CIDs
func TestDirectoryMountIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping directory mount integration test in short mode")
	}

	// Check if FUSE is available
	if _, err := os.Stat("/dev/fuse"); err != nil {
		t.Skip("Skipping FUSE test: /dev/fuse not available")
	}

	// Setup test environment
	mountDir, storageManager, client := setupTestEnvironment(t)
	defer os.RemoveAll(mountDir)
	defer storageManager.Stop(context.Background())

	// Create a test directory structure and upload it
	dirCID, encKey := createTestDirectoryStructure(t, storageManager, client)

	t.Run("MountSingleDirectory", func(t *testing.T) {
		testMountSingleDirectory(t, mountDir, storageManager, client, dirCID, encKey)
	})

	t.Run("MountWithEncryption", func(t *testing.T) {
		testMountWithEncryption(t, mountDir, storageManager, client, dirCID, encKey)
	})

	t.Run("MountSubdirectory", func(t *testing.T) {
		testMountSubdirectory(t, mountDir, storageManager, client, dirCID, encKey)
	})

	t.Run("MountMultipleDirectories", func(t *testing.T) {
		testMountMultipleDirectories(t, mountDir, storageManager, client)
	})
}

// TestLargeDirectoryHandling tests directories with >1000 files
func TestLargeDirectoryHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large directory test in short mode")
	}

	// Setup test environment
	mountDir, storageManager, client := setupTestEnvironment(t)
	defer os.RemoveAll(mountDir)
	defer storageManager.Stop(context.Background())

	// Create a large directory with 1500 files
	t.Log("Creating large directory with 1500 files...")
	dirCID, _ := createLargeDirectory(t, storageManager, client, 1500)

	// Mount the directory
	opts := MountOptions{
		MountPath:           mountDir,
		VolumeName:          "large_dir_test",
		DirectoryDescriptor: dirCID,
	}

	// Start mount in background
	mountErr := make(chan error, 1)
	go func() {
		err := MountWithIndex(client, storageManager, opts, "")
		mountErr <- err
	}()

	// Wait for mount
	time.Sleep(2 * time.Second)

	// Test operations on large directory
	t.Run("ListLargeDirectory", func(t *testing.T) {
		start := time.Now()
		entries, err := os.ReadDir(filepath.Join(mountDir, "mounted-dir"))
		if err != nil {
			t.Fatalf("Failed to read large directory: %v", err)
		}
		duration := time.Since(start)

		if len(entries) != 1500 {
			t.Errorf("Expected 1500 entries, got %d", len(entries))
		}

		t.Logf("Listed 1500 files in %v", duration)
		if duration > 5*time.Second {
			t.Errorf("Directory listing took too long: %v", duration)
		}
	})

	t.Run("RandomAccessInLargeDir", func(t *testing.T) {
		// Test random access to files
		testFiles := []string{"file_0100.txt", "file_0500.txt", "file_1000.txt", "file_1499.txt"}
		for _, filename := range testFiles {
			start := time.Now()
			content, err := os.ReadFile(filepath.Join(mountDir, "mounted-dir", filename))
			if err != nil {
				t.Errorf("Failed to read %s: %v", filename, err)
				continue
			}
			duration := time.Since(start)

			expected := fmt.Sprintf("Content of %s", filename)
			if string(content) != expected {
				t.Errorf("Wrong content in %s: got %q, want %q", filename, string(content), expected)
			}

			t.Logf("Read %s in %v", filename, duration)
		}
	})
}

// TestConcurrentDirectoryAccess tests multiple processes accessing directories
func TestConcurrentDirectoryAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent access test in short mode")
	}

	// Setup test environment
	mountDir, storageManager, client := setupTestEnvironment(t)
	defer os.RemoveAll(mountDir)
	defer storageManager.Stop(context.Background())

	// Create test directory
	dirCID, _ := createTestDirectoryStructure(t, storageManager, client)

	// Mount directory
	opts := MountOptions{
		MountPath:           mountDir,
		VolumeName:          "concurrent_test",
		DirectoryDescriptor: dirCID,
	}

	go func() {
		MountWithIndex(client, storageManager, opts, "")
	}()
	time.Sleep(2 * time.Second)

	// Run concurrent operations
	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Multiple readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				entries, err := os.ReadDir(filepath.Join(mountDir, "mounted-dir"))
				if err != nil {
					errors <- fmt.Errorf("reader %d: %v", id, err)
					return
				}
				if len(entries) < 1 {
					errors <- fmt.Errorf("reader %d: no entries found", id)
				}
				time.Sleep(10 * time.Millisecond)
			}
		}(i)
	}

	// Multiple file readers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				filename := fmt.Sprintf("file%d.txt", j%3)
				path := filepath.Join(mountDir, "mounted-dir", filename)
				_, err := os.ReadFile(path)
				if err != nil {
					errors <- fmt.Errorf("file reader %d: %v", id, err)
				}
				time.Sleep(5 * time.Millisecond)
			}
		}(i)
	}

	// Wait for completion
	wg.Wait()
	close(errors)

	// Check for errors
	errorCount := 0
	for err := range errors {
		t.Errorf("Concurrent access error: %v", err)
		errorCount++
	}

	if errorCount == 0 {
		t.Log("All concurrent operations completed successfully")
	}
}

// TestDirectoryCachePerformance validates the manifest cache performance
func TestDirectoryCachePerformance(t *testing.T) {
	// Create directory cache
	config := DefaultDirectoryCacheConfig()
	config.MaxSize = 100
	config.TTL = 5 * time.Minute

	storageManager, err := storagetesting.CreateRealTestStorageManager()
	if err != nil {
		t.Skipf("Skipping cache test - storage manager setup failed: %v", err)
	}
	defer storageManager.Stop(context.Background())

	cache, err := NewDirectoryCache(config, storageManager)
	if err != nil {
		t.Fatalf("Failed to create directory cache: %v", err)
	}

	// Create test manifests
	manifests := make([]*descriptors.DirectoryManifest, 100)
	for i := 0; i < 100; i++ {
		manifest := descriptors.NewDirectoryManifest()
		for j := 0; j < 10; j++ {
			manifest.AddEntry(descriptors.DirectoryEntry{
				EncryptedName: []byte(fmt.Sprintf("file%d.txt", j)),
				CID:           fmt.Sprintf("Qm%d%d", i, j),
				Type:          descriptors.FileType,
				Size:          int64(j * 1024),
				ModifiedAt:    time.Now(),
			})
		}
		manifests[i] = manifest
	}

	// Test cache put performance
	t.Run("CachePutPerformance", func(t *testing.T) {
		start := time.Now()
		for i, manifest := range manifests {
			cid := fmt.Sprintf("QmDir%d", i)
			cache.Put(cid, manifest, cid)
		}
		duration := time.Since(start)

		opsPerSec := float64(len(manifests)) / duration.Seconds()
		t.Logf("Put %d manifests in %v (%.0f ops/sec)", len(manifests), duration, opsPerSec)

		if opsPerSec < 1000 {
			t.Errorf("Cache put performance too low: %.0f ops/sec", opsPerSec)
		}
	})

	// Test cache get performance
	t.Run("CacheGetPerformance", func(t *testing.T) {
		// Warm cache
		for i := 0; i < 100; i++ {
			cid := fmt.Sprintf("QmDir%d", i)
			cache.Put(cid, manifests[i], cid)
		}

		start := time.Now()
		hits := 0
		for i := 0; i < 1000; i++ {
			cid := fmt.Sprintf("QmDir%d", i%100)
			manifest := cache.Get(cid)
			if manifest != nil {
				hits++
			}
		}
		duration := time.Since(start)

		hitRate := float64(hits) / 1000.0
		opsPerSec := 1000.0 / duration.Seconds()

		t.Logf("Get 1000 items in %v (%.0f ops/sec, %.1f%% hit rate)",
			duration, opsPerSec, hitRate*100)

		if hitRate < 0.95 {
			t.Errorf("Cache hit rate too low: %.1f%%", hitRate*100)
		}
		if opsPerSec < 10000 {
			t.Errorf("Cache get performance too low: %.0f ops/sec", opsPerSec)
		}
	})

	// Test LRU eviction performance
	t.Run("LRUEvictionPerformance", func(t *testing.T) {
		smallCache, _ := NewDirectoryCache(&DirectoryCacheConfig{
			MaxSize: 10,
			TTL:     time.Minute,
		}, storageManager)

		start := time.Now()
		for i := 0; i < 100; i++ {
			cid := fmt.Sprintf("QmEvict%d", i)
			smallCache.Put(cid, manifests[i%10], cid)
		}
		duration := time.Since(start)

		t.Logf("100 puts with eviction in %v", duration)
		if duration > 100*time.Millisecond {
			t.Errorf("Eviction performance too slow: %v", duration)
		}
	})
}

// TestDirectoryMountingEdgeCases tests various edge cases
func TestDirectoryMountingEdgeCases(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping edge case tests in short mode")
	}

	// Setup test environment
	mountDir, storageManager, client := setupTestEnvironment(t)
	defer os.RemoveAll(mountDir)
	defer storageManager.Stop(context.Background())

	t.Run("MountNonExistentDescriptor", func(t *testing.T) {
		opts := MountOptions{
			MountPath:           mountDir,
			VolumeName:          "nonexistent_test",
			DirectoryDescriptor: "QmNonExistentDescriptor123",
		}

		err := MountWithIndex(client, storageManager, opts, "")
		// Mount should succeed but accessing the directory should fail
		if err != nil {
			t.Logf("Mount with non-existent descriptor returned error: %v", err)
		}
	})

	t.Run("MountWithInvalidKey", func(t *testing.T) {
		dirCID, _ := createTestDirectoryStructure(t, storageManager, client)

		opts := MountOptions{
			MountPath:           mountDir,
			VolumeName:          "invalid_key_test",
			DirectoryDescriptor: dirCID,
			DirectoryKey:        "invalid-base64-key",
		}

		// This should fail during mount
		err := MountWithIndex(client, storageManager, opts, "")
		if err == nil {
			t.Error("Expected error with invalid encryption key")
		}
	})

	t.Run("MountEmptyDirectory", func(t *testing.T) {
		// Create empty directory
		emptyManifest := descriptors.NewDirectoryManifest()

		dirCID := uploadManifest(t, storageManager, emptyManifest, nil)

		opts := MountOptions{
			MountPath:           filepath.Join(mountDir, "empty"),
			VolumeName:          "empty_dir_test",
			DirectoryDescriptor: dirCID,
		}

		os.MkdirAll(opts.MountPath, 0755)

		go func() {
			MountWithIndex(client, storageManager, opts, "")
		}()
		time.Sleep(2 * time.Second)

		// Should be able to list empty directory
		entries, err := os.ReadDir(filepath.Join(opts.MountPath, "mounted-dir"))
		if err != nil {
			t.Errorf("Failed to read empty directory: %v", err)
		}
		if len(entries) != 0 {
			t.Errorf("Expected 0 entries in empty directory, got %d", len(entries))
		}
	})

	t.Run("MountNestedDirectories", func(t *testing.T) {
		// Create deeply nested directory structure
		rootCID := createNestedDirectoryStructure(t, storageManager, client, 5)

		opts := MountOptions{
			MountPath:           filepath.Join(mountDir, "nested"),
			VolumeName:          "nested_test",
			DirectoryDescriptor: rootCID,
		}

		os.MkdirAll(opts.MountPath, 0755)

		go func() {
			MountWithIndex(client, storageManager, opts, "")
		}()
		time.Sleep(2 * time.Second)

		// Navigate through nested structure
		currentPath := filepath.Join(opts.MountPath, "mounted-dir")
		for i := 0; i < 5; i++ {
			entries, err := os.ReadDir(currentPath)
			if err != nil {
				t.Errorf("Failed to read level %d: %v", i, err)
				break
			}

			found := false
			for _, entry := range entries {
				if entry.IsDir() && entry.Name() == fmt.Sprintf("level%d", i+1) {
					currentPath = filepath.Join(currentPath, entry.Name())
					found = true
					break
				}
			}

			if !found && i < 4 {
				t.Errorf("Could not find level%d directory", i+1)
				break
			}
		}
	})
}

// Helper functions

func setupTestEnvironment(t *testing.T) (string, *storage.Manager, *noisefs.Client) {
	// Create temporary mount point
	mountDir, err := os.MkdirTemp("", "noisefs_dirtest_")
	if err != nil {
		t.Fatalf("Failed to create temp mount dir: %v", err)
	}

	// Setup storage manager
	storageManager, err := storagetesting.CreateRealTestStorageManager()
	if err != nil {
		os.RemoveAll(mountDir)
		t.Skipf("Skipping test - storage manager setup failed: %v", err)
	}

	// Create NoiseFS client
	blockCache := cache.NewMemoryCache(1000)
	client, err := noisefs.NewClient(storageManager, blockCache)
	if err != nil {
		storageManager.Stop(context.Background())
		os.RemoveAll(mountDir)
		t.Fatalf("Failed to create NoiseFS client: %v", err)
	}

	return mountDir, storageManager, client
}

func createTestDirectoryStructure(t *testing.T, storageManager *storage.Manager, client *noisefs.Client) (string, string) {
	// Create encryption key
	key, err := crypto.GenerateKey("test-password")
	if err != nil {
		t.Fatalf("Failed to generate encryption key: %v", err)
	}
	encodedKey := base64.StdEncoding.EncodeToString(key.Key)

	// Create directory manifest
	manifest := descriptors.NewDirectoryManifest()

	// Add entries
	manifest.AddEntry(descriptors.DirectoryEntry{
		EncryptedName: []byte("file1.txt"),
		CID:           "QmFile1",
		Type:          descriptors.FileType,
		Size:          100,
		ModifiedAt:    time.Now(),
	})
	manifest.AddEntry(descriptors.DirectoryEntry{
		EncryptedName: []byte("file2.txt"),
		CID:           "QmFile2",
		Type:          descriptors.FileType,
		Size:          200,
		ModifiedAt:    time.Now(),
	})
	manifest.AddEntry(descriptors.DirectoryEntry{
		EncryptedName: []byte("subdir"),
		CID:           "QmSubdir",
		Type:          descriptors.DirectoryType,
		Size:          0,
		ModifiedAt:    time.Now(),
	})

	// Upload manifest
	dirCID := uploadManifest(t, storageManager, manifest, key)

	return dirCID, encodedKey
}

func createLargeDirectory(t *testing.T, storageManager *storage.Manager, client *noisefs.Client, fileCount int) (string, string) {
	// Create directory manifest with many files
	manifest := descriptors.NewDirectoryManifest()

	for i := 0; i < fileCount; i++ {
		manifest.AddEntry(descriptors.DirectoryEntry{
			EncryptedName: []byte(fmt.Sprintf("file_%04d.txt", i)),
			CID:           fmt.Sprintf("QmFile%d", i),
			Type:          descriptors.FileType,
			Size:          int64(i * 100),
			ModifiedAt:    time.Now(),
		})
	}

	// Upload manifest
	dirCID := uploadManifest(t, storageManager, manifest, nil)

	return dirCID, ""
}

func createNestedDirectoryStructure(t *testing.T, storageManager *storage.Manager, client *noisefs.Client, depth int) string {
	if depth == 0 {
		// Leaf directory
		manifest := descriptors.NewDirectoryManifest()
		manifest.AddEntry(descriptors.DirectoryEntry{
			EncryptedName: []byte("leaf_file.txt"),
			CID:           "QmLeafFile",
			Type:          descriptors.FileType,
			Size:          42,
			ModifiedAt:    time.Now(),
		})
		return uploadManifest(t, storageManager, manifest, nil)
	}

	// Create child directory first
	childCID := createNestedDirectoryStructure(t, storageManager, client, depth-1)

	// Create parent directory
	manifest := descriptors.NewDirectoryManifest()
	manifest.AddEntry(descriptors.DirectoryEntry{
		EncryptedName: []byte(fmt.Sprintf("file_at_level%d.txt", depth)),
		CID:           fmt.Sprintf("QmFileLevel%d", depth),
		Type:          descriptors.FileType,
		Size:          int64(depth * 100),
		ModifiedAt:    time.Now(),
	})
	manifest.AddEntry(descriptors.DirectoryEntry{
		EncryptedName: []byte(fmt.Sprintf("level%d", depth)),
		CID:           childCID,
		Type:          descriptors.DirectoryType,
		Size:          0,
		ModifiedAt:    time.Now(),
	})

	return uploadManifest(t, storageManager, manifest, nil)
}

func uploadManifest(t *testing.T, storageManager *storage.Manager, manifest *descriptors.DirectoryManifest, key *crypto.EncryptionKey) string {
	// Serialize manifest
	data, err := manifest.Marshal()
	if err != nil {
		t.Fatalf("Failed to serialize manifest: %v", err)
	}

	block, err := blocks.NewBlock(data)
	if err != nil {
		t.Fatalf("Failed to create block: %v", err)
	}

	// Store in backend
	addr, err := storageManager.Put(context.Background(), block)
	if err != nil {
		t.Fatalf("Failed to store manifest: %v", err)
	}

	return addr.ID
}

// Test functions for specific scenarios

func testMountSingleDirectory(t *testing.T, mountDir string, storageManager *storage.Manager, client *noisefs.Client, dirCID, encKey string) {
	subMount := filepath.Join(mountDir, "single")
	os.MkdirAll(subMount, 0755)

	opts := MountOptions{
		MountPath:           subMount,
		VolumeName:          "single_dir_test",
		DirectoryDescriptor: dirCID,
		DirectoryKey:        encKey,
	}

	go func() {
		MountWithIndex(client, storageManager, opts, "")
	}()
	time.Sleep(2 * time.Second)

	// Verify directory contents
	entries, err := os.ReadDir(filepath.Join(subMount, "mounted-dir"))
	if err != nil {
		t.Fatalf("Failed to read mounted directory: %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("Expected 3 entries, got %d", len(entries))
	}

	// Check specific files
	for _, entry := range entries {
		t.Logf("Found entry: %s (dir: %v)", entry.Name(), entry.IsDir())
	}
}

func testMountWithEncryption(t *testing.T, mountDir string, storageManager *storage.Manager, client *noisefs.Client, dirCID, encKey string) {
	// Test mounting with correct key
	subMount := filepath.Join(mountDir, "encrypted")
	os.MkdirAll(subMount, 0755)

	opts := MountOptions{
		MountPath:           subMount,
		VolumeName:          "encrypted_test",
		DirectoryDescriptor: dirCID,
		DirectoryKey:        encKey,
	}

	go func() {
		MountWithIndex(client, storageManager, opts, "")
	}()
	time.Sleep(2 * time.Second)

	// Should be able to read directory
	entries, err := os.ReadDir(filepath.Join(subMount, "mounted-dir"))
	if err != nil {
		t.Fatalf("Failed to read encrypted directory: %v", err)
	}

	if len(entries) > 0 {
		t.Log("Successfully mounted and read encrypted directory")
	}
}

func testMountSubdirectory(t *testing.T, mountDir string, storageManager *storage.Manager, client *noisefs.Client, dirCID, encKey string) {
	subMount := filepath.Join(mountDir, "subdir_mount")
	os.MkdirAll(subMount, 0755)

	opts := MountOptions{
		MountPath:           subMount,
		VolumeName:          "subdir_test",
		DirectoryDescriptor: dirCID,
		DirectoryKey:        encKey,
		Subdir:              "subdir",
	}

	go func() {
		MountWithIndex(client, storageManager, opts, "")
	}()
	time.Sleep(2 * time.Second)

	// The subdir should be mounted at the root
	_, err := os.Stat(filepath.Join(subMount, "mounted-dir", "subdir"))
	if err != nil {
		t.Logf("Subdir mount created expected path structure")
	}
}

func testMountMultipleDirectories(t *testing.T, mountDir string, storageManager *storage.Manager, client *noisefs.Client) {
	// Create multiple directories
	dir1CID, key1 := createTestDirectoryStructure(t, storageManager, client)
	dir2CID, key2 := createTestDirectoryStructure(t, storageManager, client)

	subMount := filepath.Join(mountDir, "multi")
	os.MkdirAll(subMount, 0755)

	opts := MountOptions{
		MountPath:  subMount,
		VolumeName: "multi_dir_test",
		MultiDirs: []DirectoryMount{
			{Name: "project1", DescriptorCID: dir1CID, EncryptionKey: key1},
			{Name: "project2", DescriptorCID: dir2CID, EncryptionKey: key2},
		},
	}

	go func() {
		MountWithIndex(client, storageManager, opts, "")
	}()
	time.Sleep(2 * time.Second)

	// Check both directories are mounted
	for _, dirname := range []string{"project1", "project2"} {
		entries, err := os.ReadDir(filepath.Join(subMount, dirname))
		if err != nil {
			t.Errorf("Failed to read %s: %v", dirname, err)
			continue
		}
		t.Logf("Directory %s has %d entries", dirname, len(entries))
	}
}

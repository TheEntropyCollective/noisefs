//go:build fuse
// +build fuse

package fuse

import (
	"fmt"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

func createTestManifest(name string, entryCount int) *descriptors.DirectoryManifest {
	manifest := descriptors.NewDirectoryManifest()

	for i := 0; i < entryCount; i++ {
		entry := descriptors.DirectoryEntry{
			EncryptedName: []byte(fmt.Sprintf("encrypted-%s-%d", name, i)),
			CID:           fmt.Sprintf("Qm%s%d", name, i),
			Type:          descriptors.FileType,
			Size:          int64(i * 1024),
			ModifiedAt:    time.Now(),
		}
		manifest.AddEntry(entry)
	}

	return manifest
}

func TestDirectoryCacheBasicOperations(t *testing.T) {
	// Create mock storage manager
	storageConfig := storage.DefaultConfig()
	storageManager, err := storage.NewManager(storageConfig)
	if err != nil {
		t.Skip("Storage manager not available for test")
	}

	config := &DirectoryCacheConfig{
		MaxSize:       5,
		TTL:           1 * time.Hour,
		EnableMetrics: true,
	}

	cache, err := NewDirectoryCache(config, storageManager)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Test Put and Get
	manifest1 := createTestManifest("dir1", 10)
	cache.Put("/dir1", manifest1, "QmDir1")

	retrieved := cache.Get("/dir1")
	if retrieved == nil {
		t.Error("Expected to retrieve cached manifest")
	}

	if len(retrieved.Entries) != len(manifest1.Entries) {
		t.Errorf("Expected %d entries, got %d", len(manifest1.Entries), len(retrieved.Entries))
	}

	// Test cache miss
	notFound := cache.Get("/nonexistent")
	if notFound != nil {
		t.Error("Expected nil for non-existent path")
	}

	// Test metrics
	hits, misses, hitRate := cache.GetMetrics()
	if hits != 1 {
		t.Errorf("Expected 1 hit, got %d", hits)
	}
	if misses != 1 {
		t.Errorf("Expected 1 miss, got %d", misses)
	}
	if hitRate != 0.5 {
		t.Errorf("Expected 50%% hit rate, got %.2f", hitRate)
	}
}

func TestDirectoryCacheLRUEviction(t *testing.T) {
	storageConfig := storage.DefaultConfig()
	storageManager, err := storage.NewManager(storageConfig)
	if err != nil {
		t.Skip("Storage manager not available for test")
	}

	config := &DirectoryCacheConfig{
		MaxSize:       3,
		TTL:           1 * time.Hour,
		EnableMetrics: true,
	}

	cache, err := NewDirectoryCache(config, storageManager)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Fill cache beyond capacity
	for i := 0; i < 5; i++ {
		manifest := createTestManifest(fmt.Sprintf("dir%d", i), 5)
		cache.Put(fmt.Sprintf("/dir%d", i), manifest, fmt.Sprintf("QmDir%d", i))
	}

	// Cache should only have 3 entries
	if cache.GetSize() != 3 {
		t.Errorf("Expected cache size of 3, got %d", cache.GetSize())
	}

	// Oldest entries (dir0, dir1) should be evicted
	if cache.Get("/dir0") != nil {
		t.Error("Expected /dir0 to be evicted")
	}

	if cache.Get("/dir1") != nil {
		t.Error("Expected /dir1 to be evicted")
	}

	// Newer entries should still be present
	if cache.Get("/dir3") == nil {
		t.Error("Expected /dir3 to be in cache")
	}
}

func TestDirectoryCacheTTL(t *testing.T) {
	storageConfig := storage.DefaultConfig()
	storageManager, err := storage.NewManager(storageConfig)
	if err != nil {
		t.Skip("Storage manager not available for test")
	}

	config := &DirectoryCacheConfig{
		MaxSize:       10,
		TTL:           100 * time.Millisecond, // Short TTL for testing
		EnableMetrics: true,
	}

	cache, err := NewDirectoryCache(config, storageManager)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Add entry
	manifest := createTestManifest("dir1", 5)
	cache.Put("/dir1", manifest, "QmDir1")

	// Should be retrievable immediately
	if cache.Get("/dir1") == nil {
		t.Error("Expected to retrieve entry immediately after put")
	}

	// Wait for TTL to expire
	time.Sleep(150 * time.Millisecond)

	// Should no longer be retrievable
	if cache.Get("/dir1") != nil {
		t.Error("Expected entry to expire after TTL")
	}
}

func TestDirectoryCacheConcurrency(t *testing.T) {
	storageConfig := storage.DefaultConfig()
	storageManager, err := storage.NewManager(storageConfig)
	if err != nil {
		t.Skip("Storage manager not available for test")
	}

	config := &DirectoryCacheConfig{
		MaxSize:       100,
		TTL:           1 * time.Hour,
		EnableMetrics: true,
	}

	cache, err := NewDirectoryCache(config, storageManager)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Run concurrent operations
	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 50; i++ {
			manifest := createTestManifest(fmt.Sprintf("dir%d", i), 5)
			cache.Put(fmt.Sprintf("/dir%d", i), manifest, fmt.Sprintf("QmDir%d", i))
			time.Sleep(time.Microsecond)
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 50; i++ {
			cache.Get(fmt.Sprintf("/dir%d", i))
			cache.GetMetrics()
			cache.GetSize()
			time.Sleep(time.Microsecond)
		}
		done <- true
	}()

	// Wait for both to complete
	<-done
	<-done

	// Verify cache is in consistent state
	size := cache.GetSize()
	if size > 100 {
		t.Errorf("Cache size exceeded maximum: %d", size)
	}
}

func TestDirectoryCacheClear(t *testing.T) {
	storageConfig := storage.DefaultConfig()
	storageManager, err := storage.NewManager(storageConfig)
	if err != nil {
		t.Skip("Storage manager not available for test")
	}

	config := DefaultDirectoryCacheConfig()
	cache, err := NewDirectoryCache(config, storageManager)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Add entries
	for i := 0; i < 5; i++ {
		manifest := createTestManifest(fmt.Sprintf("dir%d", i), 5)
		cache.Put(fmt.Sprintf("/dir%d", i), manifest, fmt.Sprintf("QmDir%d", i))
	}

	// Verify entries exist
	if cache.GetSize() != 5 {
		t.Errorf("Expected 5 entries, got %d", cache.GetSize())
	}

	// Clear cache
	cache.Clear()

	// Verify cache is empty
	if cache.GetSize() != 0 {
		t.Errorf("Expected empty cache after clear, got %d entries", cache.GetSize())
	}

	// Verify entries are gone
	for i := 0; i < 5; i++ {
		if cache.Get(fmt.Sprintf("/dir%d", i)) != nil {
			t.Errorf("Expected /dir%d to be cleared", i)
		}
	}
}

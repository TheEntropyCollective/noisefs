package storage

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/core/crypto"
)

// Simple mock backend for testing
type testBackend struct {
	data  map[string][]byte
	mutex sync.RWMutex
}

func newTestBackend() *testBackend {
	return &testBackend{
		data: make(map[string][]byte),
	}
}

func (tb *testBackend) Connect(ctx context.Context) error {
	return nil
}

func (tb *testBackend) Disconnect(ctx context.Context) error {
	return nil
}

func (tb *testBackend) IsConnected() bool {
	return true
}

func (tb *testBackend) Put(ctx context.Context, block *blocks.Block) (*BlockAddress, error) {
	tb.mutex.Lock()
	defer tb.mutex.Unlock()
	
	tb.data[block.ID] = block.Data
	return &BlockAddress{
		ID:          block.ID,
		BackendType: "test",
	}, nil
}

func (tb *testBackend) Get(ctx context.Context, address *BlockAddress) (*blocks.Block, error) {
	tb.mutex.RLock()
	defer tb.mutex.RUnlock()
	
	data, exists := tb.data[address.ID]
	if !exists {
		return nil, fmt.Errorf("block not found")
	}
	
	return &blocks.Block{
		ID:   address.ID,
		Data: data,
	}, nil
}

func (tb *testBackend) Has(ctx context.Context, address *BlockAddress) (bool, error) {
	tb.mutex.RLock()
	defer tb.mutex.RUnlock()
	
	_, exists := tb.data[address.ID]
	return exists, nil
}

func (tb *testBackend) Delete(ctx context.Context, address *BlockAddress) error {
	tb.mutex.Lock()
	defer tb.mutex.Unlock()
	
	delete(tb.data, address.ID)
	return nil
}

func (tb *testBackend) Pin(ctx context.Context, address *BlockAddress) error {
	return nil // No-op for test backend
}

func (tb *testBackend) Unpin(ctx context.Context, address *BlockAddress) error {
	return nil // No-op for test backend
}

func (tb *testBackend) GetBackendInfo() *BackendInfo {
	return &BackendInfo{
		Name:    "test-backend",
		Type:    "test",
		Version: "1.0.0",
		Capabilities: []string{
			CapabilityContentAddress,
			CapabilityBatch,
		},
		Config: map[string]interface{}{
			"type": "test",
		},
	}
}

func (tb *testBackend) PutMany(ctx context.Context, blocks []*blocks.Block) ([]*BlockAddress, error) {
	addresses := make([]*BlockAddress, len(blocks))
	for i, block := range blocks {
		address, err := tb.Put(ctx, block)
		if err != nil {
			return nil, err
		}
		addresses[i] = address
	}
	return addresses, nil
}

func (tb *testBackend) GetMany(ctx context.Context, addresses []*BlockAddress) ([]*blocks.Block, error) {
	blocks := make([]*blocks.Block, len(addresses))
	for i, address := range addresses {
		block, err := tb.Get(ctx, address)
		if err != nil {
			return nil, err
		}
		blocks[i] = block
	}
	return blocks, nil
}

func (tb *testBackend) HealthCheck(ctx context.Context) *HealthStatus {
	return &HealthStatus{
		Healthy: true,
		Status:  "healthy",
	}
}

// Test helper functions
func createTestStorageManager(t *testing.T) *Manager {
	// Register test backend
	RegisterBackend("test", func(cfg *BackendConfig) (Backend, error) {
		return newTestBackend(), nil
	})
	
	config := &Config{
		DefaultBackend: "test",
		Backends: map[string]*BackendConfig{
			"test": {
				Type:     "test",
				Enabled:  true,
				Priority: 1,
				Connection: &ConnectionConfig{
					Endpoint: "http://localhost:5001",
				},
				Settings: map[string]interface{}{},
			},
		},
		Distribution: &DistributionConfig{
			Strategy: "single",
		},
		HealthCheck: &HealthCheckConfig{
			Enabled:  false,
			Interval: time.Second,
			Timeout:  time.Second,
		},
	}
	
	manager, err := NewManager(config)
	if err != nil {
		t.Fatalf("Failed to create storage manager: %v", err)
	}
	
	return manager
}

func createTestEncryptionKey(t *testing.T) *crypto.EncryptionKey {
	key, err := crypto.GenerateKey("test-password")
	if err != nil {
		t.Fatalf("Failed to generate encryption key: %v", err)
	}
	return key
}

func createTestDirectoryManifest() *blocks.DirectoryManifest {
	manifest := blocks.NewDirectoryManifest()
	
	// Add some test entries
	entries := []blocks.DirectoryEntry{
		{
			EncryptedName: []byte("encrypted-file1.txt"),
			CID:           "test-cid-1",
			Type:          blocks.FileType,
			Size:          100,
			ModifiedAt:    time.Now(),
		},
		{
			EncryptedName: []byte("encrypted-file2.txt"),
			CID:           "test-cid-2",
			Type:          blocks.FileType,
			Size:          200,
			ModifiedAt:    time.Now(),
		},
		{
			EncryptedName: []byte("encrypted-subdir"),
			CID:           "test-cid-3",
			Type:          blocks.DirectoryType,
			Size:          0,
			ModifiedAt:    time.Now(),
		},
	}
	
	for _, entry := range entries {
		manifest.AddEntry(entry)
	}
	
	return manifest
}

func TestDefaultDirectoryManagerConfig(t *testing.T) {
	config := DefaultDirectoryManagerConfig()
	
	if config.CacheSize <= 0 {
		t.Error("Cache size should be positive")
	}
	
	if config.CacheTTL <= 0 {
		t.Error("Cache TTL should be positive")
	}
	
	if config.MaxManifestSize <= 0 {
		t.Error("Max manifest size should be positive")
	}
	
	if config.ReconstructionTTL <= 0 {
		t.Error("Reconstruction TTL should be positive")
	}
	
	if !config.EnableMetrics {
		t.Error("Metrics should be enabled by default")
	}
}

func TestNewDirectoryManager(t *testing.T) {
	tests := []struct {
		name             string
		storageManager   *Manager
		encryptionKey    *crypto.EncryptionKey
		config           *DirectoryManagerConfig
		expectError      bool
	}{
		{
			name:           "nil storage manager",
			storageManager: nil,
			encryptionKey:  createTestEncryptionKey(t),
			config:         DefaultDirectoryManagerConfig(),
			expectError:    true,
		},
		{
			name:           "nil encryption key",
			storageManager: createTestStorageManager(t),
			encryptionKey:  nil,
			config:         DefaultDirectoryManagerConfig(),
			expectError:    true,
		},
		{
			name:           "valid parameters",
			storageManager: createTestStorageManager(t),
			encryptionKey:  createTestEncryptionKey(t),
			config:         DefaultDirectoryManagerConfig(),
			expectError:    false,
		},
		{
			name:           "nil config uses default",
			storageManager: createTestStorageManager(t),
			encryptionKey:  createTestEncryptionKey(t),
			config:         nil,
			expectError:    false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewDirectoryManager(tt.storageManager, tt.encryptionKey, tt.config)
			
			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got nil")
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			
			if manager == nil {
				t.Error("Expected non-nil manager")
				return
			}
			
			if manager.cache == nil {
				t.Error("Expected non-nil cache")
			}
		})
	}
}

func TestDirectoryManager_StoreAndRetrieveManifest(t *testing.T) {
	storageManager := createTestStorageManager(t)
	ctx := context.Background()
	
	// Start storage manager
	if err := storageManager.Start(ctx); err != nil {
		t.Fatalf("Failed to start storage manager: %v", err)
	}
	defer storageManager.Stop(ctx)
	
	// Create directory manager
	encryptionKey := createTestEncryptionKey(t)
	config := DefaultDirectoryManagerConfig()
	
	manager, err := NewDirectoryManager(storageManager, encryptionKey, config)
	if err != nil {
		t.Fatalf("Failed to create directory manager: %v", err)
	}
	
	// Create test manifest
	manifest := createTestDirectoryManifest()
	dirPath := "/test/directory"
	
	// Store manifest
	manifestCID, err := manager.StoreDirectoryManifest(ctx, dirPath, manifest)
	if err != nil {
		t.Fatalf("Failed to store manifest: %v", err)
	}
	
	if manifestCID == "" {
		t.Error("Expected non-empty manifest CID")
	}
	
	// Retrieve manifest
	retrievedManifest, err := manager.RetrieveDirectoryManifest(ctx, dirPath, manifestCID)
	if err != nil {
		t.Fatalf("Failed to retrieve manifest: %v", err)
	}
	
	if retrievedManifest == nil {
		t.Error("Expected non-nil retrieved manifest")
	}
	
	// Verify manifest contents
	if retrievedManifest.Version != manifest.Version {
		t.Errorf("Version mismatch: expected %s, got %s", manifest.Version, retrievedManifest.Version)
	}
	
	if len(retrievedManifest.Entries) != len(manifest.Entries) {
		t.Errorf("Entry count mismatch: expected %d, got %d", len(manifest.Entries), len(retrievedManifest.Entries))
	}
}

func TestDirectoryManager_CacheHitAndMiss(t *testing.T) {
	storageManager := createTestStorageManager(t)
	ctx := context.Background()
	
	// Start storage manager
	if err := storageManager.Start(ctx); err != nil {
		t.Fatalf("Failed to start storage manager: %v", err)
	}
	defer storageManager.Stop(ctx)
	
	// Create directory manager
	encryptionKey := createTestEncryptionKey(t)
	config := DefaultDirectoryManagerConfig()
	
	manager, err := NewDirectoryManager(storageManager, encryptionKey, config)
	if err != nil {
		t.Fatalf("Failed to create directory manager: %v", err)
	}
	
	// Create test manifest
	manifest := createTestDirectoryManifest()
	dirPath := "/test/directory"
	
	// Store manifest
	manifestCID, err := manager.StoreDirectoryManifest(ctx, dirPath, manifest)
	if err != nil {
		t.Fatalf("Failed to store manifest: %v", err)
	}
	
	// Clear cache to ensure miss
	manager.ClearCache()
	
	// First retrieval should be cache miss
	initialStats := manager.GetCacheStats()
	
	_, err = manager.RetrieveDirectoryManifest(ctx, dirPath, manifestCID)
	if err != nil {
		t.Fatalf("Failed to retrieve manifest: %v", err)
	}
	
	// Second retrieval should be cache hit
	_, err = manager.RetrieveDirectoryManifest(ctx, dirPath, manifestCID)
	if err != nil {
		t.Fatalf("Failed to retrieve manifest: %v", err)
	}
	
	finalStats := manager.GetCacheStats()
	
	if finalStats.CacheMisses != initialStats.CacheMisses + 1 {
		t.Errorf("Expected cache miss count to increase by 1")
	}
	
	if finalStats.CacheHits != initialStats.CacheHits + 1 {
		t.Errorf("Expected cache hit count to increase by 1")
	}
	
	if finalStats.HitRate <= 0 {
		t.Error("Expected positive hit rate")
	}
}

func TestDirectoryManager_ReconstructDirectory(t *testing.T) {
	storageManager := createTestStorageManager(t)
	ctx := context.Background()
	
	// Start storage manager
	if err := storageManager.Start(ctx); err != nil {
		t.Fatalf("Failed to start storage manager: %v", err)
	}
	defer storageManager.Stop(ctx)
	
	// Create directory manager
	encryptionKey := createTestEncryptionKey(t)
	config := DefaultDirectoryManagerConfig()
	
	manager, err := NewDirectoryManager(storageManager, encryptionKey, config)
	if err != nil {
		t.Fatalf("Failed to create directory manager: %v", err)
	}
	
	// Create test manifest with encrypted filenames
	manifest := blocks.NewDirectoryManifest()
	dirPath := "/test/directory"
	
	// Derive directory key
	dirKey, err := crypto.DeriveDirectoryKey(encryptionKey, dirPath)
	if err != nil {
		t.Fatalf("Failed to derive directory key: %v", err)
	}
	
	// Add entries with encrypted names
	filenames := []string{"file1.txt", "file2.txt", "subdir"}
	for i, filename := range filenames {
		encryptedName, err := crypto.EncryptFileName(filename, dirKey)
		if err != nil {
			t.Fatalf("Failed to encrypt filename: %v", err)
		}
		
		entry := blocks.DirectoryEntry{
			EncryptedName: encryptedName,
			CID:           fmt.Sprintf("test-cid-%d", i+1),
			Type:          blocks.FileType,
			Size:          int64((i + 1) * 100),
			ModifiedAt:    time.Now(),
		}
		
		if filename == "subdir" {
			entry.Type = blocks.DirectoryType
			entry.Size = 0
		}
		
		manifest.AddEntry(entry)
	}
	
	// Store manifest
	manifestCID, err := manager.StoreDirectoryManifest(ctx, dirPath, manifest)
	if err != nil {
		t.Fatalf("Failed to store manifest: %v", err)
	}
	
	// Reconstruct directory (use same path as when created to derive correct key)
	targetPath := dirPath
	result, err := manager.ReconstructDirectory(ctx, manifestCID, targetPath)
	if err != nil {
		t.Fatalf("Failed to reconstruct directory: %v", err)
	}
	
	// Verify reconstruction result
	if result.Status != "completed" {
		t.Errorf("Expected status 'completed', got '%s'", result.Status)
	}
	
	if result.TotalEntries != len(filenames) {
		t.Errorf("Expected %d entries, got %d", len(filenames), result.TotalEntries)
	}
	
	if result.ProcessedEntries != len(filenames) {
		t.Errorf("Expected %d processed entries, got %d", len(filenames), result.ProcessedEntries)
	}
	
	// Verify decrypted filenames
	for i, entry := range result.Entries {
		expectedName := filenames[i]
		if entry.DecryptedName != expectedName {
			t.Errorf("Expected filename '%s', got '%s'", expectedName, entry.DecryptedName)
		}
	}
	
	if len(result.Errors) > 0 {
		t.Errorf("Expected no errors, got %d errors", len(result.Errors))
	}
}

func TestDirectoryCache_LRUEviction(t *testing.T) {
	cache, err := NewDirectoryCache(2, time.Hour) // Small cache for testing
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	
	// Create test manifests
	manifest1 := createTestDirectoryManifest()
	manifest2 := createTestDirectoryManifest()
	manifest3 := createTestDirectoryManifest()
	
	// Add entries to cache
	cache.Put("key1", manifest1)
	cache.Put("key2", manifest2)
	
	// Verify cache size
	if cache.Size() != 2 {
		t.Errorf("Expected cache size 2, got %d", cache.Size())
	}
	
	// Access key1 to make it most recently used
	retrieved := cache.Get("key1")
	if retrieved == nil {
		t.Error("Expected to retrieve key1")
	}
	
	// Add key3, should evict key2 (least recently used)
	cache.Put("key3", manifest3)
	
	// Verify cache size is still 2
	if cache.Size() != 2 {
		t.Errorf("Expected cache size 2, got %d", cache.Size())
	}
	
	// key1 should still be in cache
	if cache.Get("key1") == nil {
		t.Error("Expected key1 to still be in cache")
	}
	
	// key2 should have been evicted
	if cache.Get("key2") != nil {
		t.Error("Expected key2 to be evicted")
	}
	
	// key3 should be in cache
	if cache.Get("key3") == nil {
		t.Error("Expected key3 to be in cache")
	}
}

func TestDirectoryCache_TTLExpiration(t *testing.T) {
	cache, err := NewDirectoryCache(10, 10*time.Millisecond) // Short TTL for testing
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	
	manifest := createTestDirectoryManifest()
	
	// Add entry to cache
	cache.Put("test-key", manifest)
	
	// Should be retrievable immediately
	if cache.Get("test-key") == nil {
		t.Error("Expected to retrieve entry immediately")
	}
	
	// Wait for TTL expiration
	time.Sleep(15 * time.Millisecond)
	
	// Should be expired and return nil
	if cache.Get("test-key") != nil {
		t.Error("Expected entry to be expired")
	}
	
	// Cache should be empty after expiration
	if cache.Size() != 0 {
		t.Errorf("Expected cache size 0 after expiration, got %d", cache.Size())
	}
}

func TestDirectoryManager_HealthCheck(t *testing.T) {
	storageManager := createTestStorageManager(t)
	ctx := context.Background()
	
	// Start storage manager
	if err := storageManager.Start(ctx); err != nil {
		t.Fatalf("Failed to start storage manager: %v", err)
	}
	defer storageManager.Stop(ctx)
	
	// Create directory manager
	encryptionKey := createTestEncryptionKey(t)
	config := DefaultDirectoryManagerConfig()
	
	manager, err := NewDirectoryManager(storageManager, encryptionKey, config)
	if err != nil {
		t.Fatalf("Failed to create directory manager: %v", err)
	}
	
	// Check health
	health := manager.HealthCheck()
	
	if health == nil {
		t.Error("Expected non-nil health check result")
	}
	
	if !health.Healthy {
		t.Errorf("Expected healthy status, got issues: %v", health.Issues)
	}
	
	if health.CacheStats == nil {
		t.Error("Expected non-nil cache stats")
	}
	
	if health.LastCheck.IsZero() {
		t.Error("Expected non-zero last check time")
	}
}

func TestDirectoryManager_MaxManifestSize(t *testing.T) {
	storageManager := createTestStorageManager(t)
	ctx := context.Background()
	
	// Start storage manager
	if err := storageManager.Start(ctx); err != nil {
		t.Fatalf("Failed to start storage manager: %v", err)
	}
	defer storageManager.Stop(ctx)
	
	// Create directory manager with small max size
	encryptionKey := createTestEncryptionKey(t)
	config := DefaultDirectoryManagerConfig()
	config.MaxManifestSize = 100 // Very small size for testing
	
	manager, err := NewDirectoryManager(storageManager, encryptionKey, config)
	if err != nil {
		t.Fatalf("Failed to create directory manager: %v", err)
	}
	
	// Create large manifest
	manifest := createTestDirectoryManifest()
	dirPath := "/test/directory"
	
	// Try to store manifest (should fail due to size limit)
	_, err = manager.StoreDirectoryManifest(ctx, dirPath, manifest)
	if err == nil {
		t.Error("Expected error due to manifest size limit")
	}
}

func BenchmarkDirectoryManager_StoreManifest(b *testing.B) {
	storageManager := createTestStorageManagerBench(b)
	ctx := context.Background()
	
	// Start storage manager
	if err := storageManager.Start(ctx); err != nil {
		b.Fatalf("Failed to start storage manager: %v", err)
	}
	defer storageManager.Stop(ctx)
	
	// Create directory manager
	encryptionKey := createTestEncryptionKeyBench(b)
	config := DefaultDirectoryManagerConfig()
	
	manager, err := NewDirectoryManager(storageManager, encryptionKey, config)
	if err != nil {
		b.Fatalf("Failed to create directory manager: %v", err)
	}
	
	// Create test manifest
	manifest := createTestDirectoryManifest()
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		dirPath := fmt.Sprintf("/test/directory-%d", i)
		_, err := manager.StoreDirectoryManifest(ctx, dirPath, manifest)
		if err != nil {
			b.Fatalf("Failed to store manifest: %v", err)
		}
	}
}

func BenchmarkDirectoryManager_RetrieveManifest(b *testing.B) {
	storageManager := createTestStorageManagerBench(b)
	ctx := context.Background()
	
	// Start storage manager
	if err := storageManager.Start(ctx); err != nil {
		b.Fatalf("Failed to start storage manager: %v", err)
	}
	defer storageManager.Stop(ctx)
	
	// Create directory manager
	encryptionKey := createTestEncryptionKeyBench(b)
	config := DefaultDirectoryManagerConfig()
	
	manager, err := NewDirectoryManager(storageManager, encryptionKey, config)
	if err != nil {
		b.Fatalf("Failed to create directory manager: %v", err)
	}
	
	// Store test manifest
	manifest := createTestDirectoryManifest()
	dirPath := "/test/directory"
	
	manifestCID, err := manager.StoreDirectoryManifest(ctx, dirPath, manifest)
	if err != nil {
		b.Fatalf("Failed to store manifest: %v", err)
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_, err := manager.RetrieveDirectoryManifest(ctx, dirPath, manifestCID)
		if err != nil {
			b.Fatalf("Failed to retrieve manifest: %v", err)
		}
	}
}

// Helper functions for benchmarks
func createTestStorageManagerBench(b *testing.B) *Manager {
	// Register test backend
	RegisterBackend("test", func(cfg *BackendConfig) (Backend, error) {
		return newTestBackend(), nil
	})
	
	config := &Config{
		DefaultBackend: "test",
		Backends: map[string]*BackendConfig{
			"test": {
				Type:     "test",
				Enabled:  true,
				Priority: 1,
				Connection: &ConnectionConfig{
					Endpoint: "http://localhost:5001",
				},
				Settings: map[string]interface{}{},
			},
		},
		Distribution: &DistributionConfig{
			Strategy: "single",
		},
		HealthCheck: &HealthCheckConfig{
			Enabled:  false,
			Interval: time.Second,
			Timeout:  time.Second,
		},
	}
	
	manager, err := NewManager(config)
	if err != nil {
		b.Fatalf("Failed to create storage manager: %v", err)
	}
	
	return manager
}

func createTestEncryptionKeyBench(b *testing.B) *crypto.EncryptionKey {
	key, err := crypto.GenerateKey("test-password")
	if err != nil {
		b.Fatalf("Failed to generate encryption key: %v", err)
	}
	return key
}
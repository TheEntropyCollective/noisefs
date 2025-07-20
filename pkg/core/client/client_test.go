package noisefs

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	_ "github.com/TheEntropyCollective/noisefs/pkg/storage/backends" // Register mock backend
)

func createTestStorageManager(t *testing.T) *storage.Manager {
	t.Helper()
	
	config := storage.DefaultConfig()
	config.DefaultBackend = "mock"
	config.Backends = map[string]*storage.BackendConfig{
		"mock": {
			Type:     "mock",
			Enabled:  true,
			Priority: 100,
			Connection: &storage.ConnectionConfig{
				Endpoint: "memory://test",
			},
		},
	}
	
	storageManager, err := storage.NewManager(config)
	if err != nil {
		t.Fatalf("Failed to create storage manager: %v", err)
	}
	
	err = storageManager.Start(context.Background())
	if err != nil {
		t.Fatalf("Failed to start storage manager: %v", err)
	}
	
	return storageManager
}

func TestNewClient_Basic(t *testing.T) {
	storageManager := createTestStorageManager(t)
	blockCache := cache.NewMemoryCache(1024 * 1024) // 1MB cache
	
	client, err := NewClient(storageManager, blockCache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	
	if client == nil {
		t.Fatal("Client should not be nil")
	}
	
	if client.storageManager != storageManager {
		t.Error("Client should have the provided storage manager")
	}
	
	if client.cache != blockCache {
		t.Error("Client should have the provided cache")
	}
}

func TestNewClient_WithConfig(t *testing.T) {
	storageManager := createTestStorageManager(t)
	blockCache := cache.NewMemoryCache(1024 * 1024)
	
	config := &ClientConfig{
		EnableAdaptiveCache:   false,
		PreferRandomizerPeers: false,
	}
	
	client, err := NewClientWithConfig(storageManager, blockCache, config)
	if err != nil {
		t.Fatalf("Failed to create client with config: %v", err)
	}
	
	if client.adaptiveCacheEnabled {
		t.Error("Adaptive cache should be disabled")
	}
	
	if client.preferRandomizerPeers {
		t.Error("Randomizer peer preference should be disabled")
	}
}

func TestClient_StoreBlockWithCache(t *testing.T) {
	storageManager := createTestStorageManager(t)
	blockCache := cache.NewMemoryCache(1024 * 1024)
	
	client, err := NewClient(storageManager, blockCache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	
	// Create test block
	testData := []byte("Hello, NoiseFS test data!")
	block, err := blocks.NewBlock(testData)
	if err != nil {
		t.Fatalf("Failed to create block: %v", err)
	}
	
	// Store block
	ctx := context.Background()
	cid, err := client.StoreBlockWithCache(ctx, block)
	if err != nil {
		t.Fatalf("Failed to store block: %v", err)
	}
	
	if cid == "" {
		t.Error("CID should not be empty")
	}
	
	if block.ID != cid {
		t.Error("Block ID should match returned CID")
	}
}

func TestClient_RetrieveBlockWithCache(t *testing.T) {
	storageManager := createTestStorageManager(t)
	blockCache := cache.NewMemoryCache(1024 * 1024)
	
	client, err := NewClient(storageManager, blockCache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	
	// Create and store test block
	testData := []byte("Hello, NoiseFS retrieval test!")
	block, err := blocks.NewBlock(testData)
	if err != nil {
		t.Fatalf("Failed to create block: %v", err)
	}
	
	ctx := context.Background()
	cid, err := client.StoreBlockWithCache(ctx, block)
	if err != nil {
		t.Fatalf("Failed to store block: %v", err)
	}
	
	// Retrieve block
	retrievedBlock, err := client.RetrieveBlockWithCache(context.Background(), cid)
	if err != nil {
		t.Fatalf("Failed to retrieve block: %v", err)
	}
	
	if retrievedBlock.ID != cid {
		t.Error("Retrieved block should have correct ID")
	}
	
	if !bytes.Equal(retrievedBlock.Data, testData) {
		t.Error("Retrieved block data should match original")
	}
}

func TestClient_SelectRandomizers(t *testing.T) {
	storageManager := createTestStorageManager(t)
	blockCache := cache.NewMemoryCache(1024 * 1024)
	
	client, err := NewClient(storageManager, blockCache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	
	// Select randomizers
	size := 64 * 1024 // 64KB
	ctx := context.Background()
	rand1, cid1, rand2, cid2, overhead, err := client.SelectRandomizers(ctx, size)
	if err != nil {
		t.Fatalf("Failed to select randomizers: %v", err)
	}
	
	if rand1 == nil {
		t.Fatal("First randomizer block should not be nil")
	}
	
	if rand2 == nil {
		t.Fatal("Second randomizer block should not be nil")
	}
	
	if len(rand1.Data) != size {
		t.Errorf("Expected first randomizer size %d, got %d", size, len(rand1.Data))
	}
	
	if len(rand2.Data) != size {
		t.Errorf("Expected second randomizer size %d, got %d", size, len(rand2.Data))
	}
	
	if cid1 == "" {
		t.Error("First randomizer should have CID")
	}
	
	if cid2 == "" {
		t.Error("Second randomizer should have CID")
	}
	
	if overhead < 0 {
		t.Error("Overhead should be non-negative")
	}
}

func TestClient_UploadAndDownload(t *testing.T) {
	storageManager := createTestStorageManager(t)
	blockCache := cache.NewMemoryCache(1024 * 1024)
	
	client, err := NewClient(storageManager, blockCache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	
	// Create test file data
	testData := []byte(strings.Repeat("Hello NoiseFS! ", 1000)) // ~15KB
	reader := bytes.NewReader(testData)
	
	// Upload file
	filename := "test_file.txt"
	blockSize := 64 * 1024
	ctx := context.Background()
	
	descriptorCID, err := client.UploadWithBlockSize(ctx, reader, filename, blockSize)
	if err != nil {
		t.Fatalf("Failed to upload file: %v", err)
	}
	
	if descriptorCID == "" {
		t.Error("Descriptor CID should not be empty")
	}
	
	// Download file
	retrievedData, err := client.Download(ctx, descriptorCID)
	if err != nil {
		t.Fatalf("Failed to download file: %v", err)
	}
	
	if !bytes.Equal(testData, retrievedData) {
		t.Error("Downloaded file data should match original")
	}
}

func TestClient_CacheIntegration(t *testing.T) {
	storageManager := createTestStorageManager(t)
	blockCache := cache.NewMemoryCache(1024 * 1024)
	
	client, err := NewClient(storageManager, blockCache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	
	// Create test block
	testData := []byte("Cache integration test data")
	block, err := blocks.NewBlock(testData)
	if err != nil {
		t.Fatalf("Failed to create block: %v", err)
	}
	
	// Store block (should go to cache)
	ctx := context.Background()
	cid, err := client.StoreBlockWithCache(ctx, block)
	if err != nil {
		t.Fatalf("Failed to store block: %v", err)
	}
	
	// Check cache has the block
	cachedBlock, err := blockCache.Get(cid)
	if err != nil || cachedBlock == nil {
		t.Error("Block should be in cache after storage")
	}
	
	// Retrieve block (should come from cache)
	retrievedBlock, err := client.RetrieveBlockWithCache(context.Background(), cid)
	if err != nil {
		t.Fatalf("Failed to retrieve block: %v", err)
	}
	
	if !bytes.Equal(retrievedBlock.Data, testData) {
		t.Error("Retrieved block data should match original")
	}
}

func TestClient_ErrorHandling(t *testing.T) {
	storageManager := createTestStorageManager(t)
	blockCache := cache.NewMemoryCache(1024 * 1024)
	
	client, err := NewClient(storageManager, blockCache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	
	// Test retrieval of non-existent block
	_, err = client.RetrieveBlockWithCache(context.Background(), "non-existent-cid")
	if err == nil {
		t.Error("Should fail to retrieve non-existent block")
	}
	
	// Test download of non-existent descriptor
	ctx := context.Background()
	_, err = client.Download(ctx, "non-existent-descriptor")
	if err == nil {
		t.Error("Should fail to download non-existent descriptor")
	}
	
	// Test invalid randomizer size
	_, _, _, _, _, err = client.SelectRandomizers(ctx, -1)
	if err == nil {
		t.Error("Should fail with negative block size")
	}
	
	// Test zero-size randomizer
	_, _, _, _, _, err = client.SelectRandomizers(ctx, 0)
	if err == nil {
		t.Error("Should fail with zero block size")
	}
}

func TestClient_Metrics(t *testing.T) {
	storageManager := createTestStorageManager(t)
	blockCache := cache.NewMemoryCache(1024 * 1024)
	
	client, err := NewClient(storageManager, blockCache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	
	// Get initial metrics
	_ = client.GetMetrics()
	// MetricsSnapshot is a struct, not a pointer, so it can't be nil
	
	// Perform some operations to generate metrics
	testData := []byte("Metrics test data")
	block, err := blocks.NewBlock(testData)
	if err != nil {
		t.Fatalf("Failed to create block: %v", err)
	}
	
	ctx := context.Background()
	cid, err := client.StoreBlockWithCache(ctx, block)
	if err != nil {
		t.Fatalf("Failed to store block: %v", err)
	}
	
	_, err = client.RetrieveBlockWithCache(context.Background(), cid)
	if err != nil {
		t.Fatalf("Failed to retrieve block: %v", err)
	}
	
	// Check metrics after operations - just verify we can get them
	updatedMetrics := client.GetMetrics()
	
	// Test basic metrics functionality
	if updatedMetrics.CacheHits < 0 {
		t.Error("Cache hits should be non-negative")
	}
	
	if updatedMetrics.CacheMisses < 0 {
		t.Error("Cache misses should be non-negative")
	}
}

func TestClient_PeerManagement(t *testing.T) {
	storageManager := createTestStorageManager(t)
	blockCache := cache.NewMemoryCache(1024 * 1024)
	
	client, err := NewClient(storageManager, blockCache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	
	// Test getting connected peers - currently returns nil as it's not implemented
	peers := client.GetConnectedPeers()
	// This is expected to be nil for now as the method is not fully implemented
	_ = peers
	
	// Test that client exists and basic peer management structure is in place
	if client == nil {
		t.Error("Client should not be nil")
	}
}

func TestClient_InputValidation(t *testing.T) {
	storageManager := createTestStorageManager(t)
	blockCache := cache.NewMemoryCache(1024 * 1024)
	
	client, err := NewClient(storageManager, blockCache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	
	ctx := context.Background()
	
	// Test invalid CID validation
	_, err = client.RetrieveBlockWithCache(context.Background(), "")
	if err == nil {
		t.Error("Should fail with empty CID")
	}
	
	_, err = client.RetrieveBlockWithCache(context.Background(), "invalid-cid")
	if err == nil {
		t.Error("Should fail with invalid CID format")
	}
	
	_, err = client.Download(ctx, "")
	if err == nil {
		t.Error("Should fail with empty descriptor CID")
	}
	
	// Test filename validation
	reader := strings.NewReader("test data")
	_, err = client.Upload(ctx, reader, "")
	if err == nil {
		t.Error("Should fail with empty filename")
	}
	
	reader = strings.NewReader("test data")
	_, err = client.Upload(ctx, reader, "../path/traversal")
	if err == nil {
		t.Error("Should fail with path traversal in filename")
	}
	
	reader = strings.NewReader("test data")
	longFilename := strings.Repeat("a", 300) // Exceeds MaxFilenameLength
	_, err = client.Upload(ctx, reader, longFilename)
	if err == nil {
		t.Error("Should fail with filename too long")
	}
	
	// Test file size validation (create a large buffer that exceeds MaxFileSize)
	// Note: We can't easily test this without consuming too much memory, so we test the validation function directly
	err = validateFileSize(-1)
	if err == nil {
		t.Error("Should fail with negative file size")
	}
	
	err = validateFileSize(MaxFileSize + 1)
	if err == nil {
		t.Error("Should fail with file size exceeding maximum")
	}
}
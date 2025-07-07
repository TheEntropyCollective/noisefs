package noisefs

import (
	"errors"
	"testing"

	"github.com/TheEntropyCollective/noisefs/pkg/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/ipfs"
)

// mockIPFSClient provides a mock IPFS client for testing
type mockIPFSClient struct {
	storeBlockFunc    func(*blocks.Block) (string, error)
	retrieveBlockFunc func(string) (*blocks.Block, error)
}

func (m *mockIPFSClient) StoreBlock(block *blocks.Block) (string, error) {
	if m.storeBlockFunc != nil {
		return m.storeBlockFunc(block)
	}
	return "mock_cid", nil
}

func (m *mockIPFSClient) RetrieveBlock(cid string) (*blocks.Block, error) {
	if m.retrieveBlockFunc != nil {
		return m.retrieveBlockFunc(cid)
	}
	// Return a mock block
	return blocks.NewBlock([]byte("mock data"))
}

func TestNewClient(t *testing.T) {
	tests := []struct {
		name       string
		ipfsClient ipfs.BlockStore
		cache      cache.Cache
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "valid inputs",
			ipfsClient: &mockIPFSClient{},
			cache:      cache.NewMemoryCache(10),
			wantErr:    false,
		},
		{
			name:       "nil IPFS client",
			ipfsClient: nil,
			cache:      cache.NewMemoryCache(10),
			wantErr:    true,
			errMsg:     "IPFS client is required",
		},
		{
			name:       "nil cache",
			ipfsClient: &mockIPFSClient{},
			cache:      nil,
			wantErr:    true,
			errMsg:     "cache is required",
		},
		{
			name:       "both nil",
			ipfsClient: nil,
			cache:      nil,
			wantErr:    true,
			errMsg:     "IPFS client is required", // Should hit IPFS check first
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.ipfsClient, tt.cache)

			if tt.wantErr && err == nil {
				t.Errorf("NewClient() error = nil, wantErr %v", tt.wantErr)
				return
			}

			if !tt.wantErr && err != nil {
				t.Errorf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err != nil {
				if err.Error() != tt.errMsg {
					t.Errorf("NewClient() error = %v, want %v", err.Error(), tt.errMsg)
				}
				return
			}

			// Valid case checks
			if client == nil {
				t.Error("NewClient() returned nil client for valid inputs")
			}

			// Note: Cannot compare interface values directly, but we can verify it's not nil
			if client.ipfsClient == nil {
				t.Error("NewClient() did not set IPFS client correctly")
			}

			if client.cache != tt.cache {
				t.Error("NewClient() did not set cache correctly")
			}

			if client.metrics == nil {
				t.Error("NewClient() did not initialize metrics")
			}
		})
	}
}

func TestSelectRandomizer(t *testing.T) {
	// Create mock IPFS client that succeeds
	mockIPFS := &mockIPFSClient{
		storeBlockFunc: func(block *blocks.Block) (string, error) {
			return "randomizer_cid", nil
		},
	}

	// Test with empty cache (should generate new randomizer)
	cache := cache.NewMemoryCache(10)
	client, err := NewClient(mockIPFS, cache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Test generating new randomizer
	randBlock, cid, err := client.SelectRandomizer(128)
	if err != nil {
		t.Errorf("SelectRandomizer() error = %v, want nil", err)
	}

	if randBlock == nil {
		t.Error("SelectRandomizer() returned nil block")
	}

	if cid == "" {
		t.Error("SelectRandomizer() returned empty CID")
	}

	if len(randBlock.Data) != 128 {
		t.Errorf("SelectRandomizer() block size = %v, want 128", len(randBlock.Data))
	}

	// Check that block was cached
	cached, err := cache.Get(cid)
	if err != nil {
		t.Errorf("SelectRandomizer() did not cache block: %v", err)
	}

	if cached.ID != randBlock.ID {
		t.Error("SelectRandomizer() cached block differs from returned block")
	}

	// Test with cache containing suitable blocks
	// First, populate cache with a block of the right size
	existingBlock, err := blocks.NewRandomBlock(128)
	if err != nil {
		t.Fatalf("Failed to create test block: %v", err)
	}
	cache.Store("existing_cid", existingBlock)

	// Now SelectRandomizer should reuse from cache
	initialMetrics := client.GetMetrics()
	
	randBlock2, cid2, err := client.SelectRandomizer(128)
	if err != nil {
		t.Errorf("SelectRandomizer() with cache error = %v, want nil", err)
	}

	finalMetrics := client.GetMetrics()
	
	// Should have incremented block reuse metric
	if finalMetrics.BlocksReused <= initialMetrics.BlocksReused {
		t.Error("SelectRandomizer() should have recorded block reuse")
	}

	// Block should be from cache (could be the existing one or the one we just added)
	if randBlock2 == nil {
		t.Error("SelectRandomizer() from cache returned nil block")
	}

	if cid2 == "" {
		t.Error("SelectRandomizer() from cache returned empty CID")
	}
}

func TestSelectRandomizerIPFSError(t *testing.T) {
	// Create mock IPFS client that fails
	mockIPFS := &mockIPFSClient{
		storeBlockFunc: func(block *blocks.Block) (string, error) {
			return "", errors.New("IPFS store failed")
		},
	}

	cache := cache.NewMemoryCache(10)
	client, err := NewClient(mockIPFS, cache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Should fail when trying to store randomizer
	_, _, err = client.SelectRandomizer(128)
	if err == nil {
		t.Error("SelectRandomizer() should fail when IPFS store fails")
	}

	if err.Error() != "failed to store randomizer: IPFS store failed" {
		t.Errorf("SelectRandomizer() error = %v, want specific error message", err)
	}
}

func TestStoreBlockWithCache(t *testing.T) {
	// Test successful storage
	mockIPFS := &mockIPFSClient{
		storeBlockFunc: func(block *blocks.Block) (string, error) {
			return "stored_cid", nil
		},
	}

	cache := cache.NewMemoryCache(10)
	client, err := NewClient(mockIPFS, cache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	testBlock, err := blocks.NewBlock([]byte("test data"))
	if err != nil {
		t.Fatalf("Failed to create test block: %v", err)
	}

	cid, err := client.StoreBlockWithCache(testBlock)
	if err != nil {
		t.Errorf("StoreBlockWithCache() error = %v, want nil", err)
	}

	if cid != "stored_cid" {
		t.Errorf("StoreBlockWithCache() cid = %v, want stored_cid", cid)
	}

	// Check that block was cached
	cached, err := cache.Get(cid)
	if err != nil {
		t.Errorf("StoreBlockWithCache() did not cache block: %v", err)
	}

	if cached.ID != testBlock.ID {
		t.Error("StoreBlockWithCache() cached block differs from original")
	}

	// Test IPFS failure
	mockIPFS.storeBlockFunc = func(block *blocks.Block) (string, error) {
		return "", errors.New("IPFS store failed")
	}

	_, err = client.StoreBlockWithCache(testBlock)
	if err == nil {
		t.Error("StoreBlockWithCache() should fail when IPFS fails")
	}
}

func TestRetrieveBlockWithCache(t *testing.T) {
	mockIPFS := &mockIPFSClient{
		retrieveBlockFunc: func(cid string) (*blocks.Block, error) {
			return blocks.NewBlock([]byte("retrieved data"))
		},
	}

	cache := cache.NewMemoryCache(10)
	client, err := NewClient(mockIPFS, cache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Test cache miss (should retrieve from IPFS)
	initialMetrics := client.GetMetrics()
	
	block, err := client.RetrieveBlockWithCache("test_cid")
	if err != nil {
		t.Errorf("RetrieveBlockWithCache() error = %v, want nil", err)
	}

	finalMetrics := client.GetMetrics()

	if block == nil {
		t.Error("RetrieveBlockWithCache() returned nil block")
	}

	// Should have recorded cache miss
	if finalMetrics.CacheMisses <= initialMetrics.CacheMisses {
		t.Error("RetrieveBlockWithCache() should have recorded cache miss")
	}

	// Block should now be cached
	cached, err := cache.Get("test_cid")
	if err != nil {
		t.Errorf("RetrieveBlockWithCache() did not cache retrieved block: %v", err)
	}

	if cached.ID != block.ID {
		t.Error("RetrieveBlockWithCache() cached block differs from returned block")
	}

	// Test cache hit (should get from cache without IPFS call)
	initialCacheMisses := finalMetrics.CacheMisses
	
	block2, err := client.RetrieveBlockWithCache("test_cid")
	if err != nil {
		t.Errorf("RetrieveBlockWithCache() cache hit error = %v, want nil", err)
	}

	finalMetrics2 := client.GetMetrics()

	// Should have recorded cache hit
	if finalMetrics2.CacheHits <= finalMetrics.CacheHits {
		t.Error("RetrieveBlockWithCache() should have recorded cache hit")
	}

	// Should not have recorded additional cache miss
	if finalMetrics2.CacheMisses != initialCacheMisses {
		t.Error("RetrieveBlockWithCache() should not record cache miss on hit")
	}

	if block2.ID != block.ID {
		t.Error("RetrieveBlockWithCache() cache hit returned different block")
	}

	// Test IPFS failure
	mockIPFS.retrieveBlockFunc = func(cid string) (*blocks.Block, error) {
		return nil, errors.New("IPFS retrieve failed")
	}

	_, err = client.RetrieveBlockWithCache("nonexistent_cid")
	if err == nil {
		t.Error("RetrieveBlockWithCache() should fail when IPFS fails")
	}
}

func TestMetricsIntegration(t *testing.T) {
	mockIPFS := &mockIPFSClient{}
	cache := cache.NewMemoryCache(10)
	client, err := NewClient(mockIPFS, cache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Test initial metrics
	metrics := client.GetMetrics()
	if metrics.BlocksReused != 0 || metrics.BlocksGenerated != 0 {
		t.Error("New client should have zero metrics")
	}

	// Test recording uploads/downloads
	client.RecordUpload(1000, 1200)
	client.RecordDownload()

	metrics = client.GetMetrics()
	if metrics.TotalUploads != 1 {
		t.Errorf("RecordUpload() uploads = %v, want 1", metrics.TotalUploads)
	}

	if metrics.TotalDownloads != 1 {
		t.Errorf("RecordDownload() downloads = %v, want 1", metrics.TotalDownloads)
	}

	if metrics.BytesUploadedOriginal != 1000 {
		t.Errorf("RecordUpload() original bytes = %v, want 1000", metrics.BytesUploadedOriginal)
	}

	if metrics.BytesStoredIPFS != 1200 {
		t.Errorf("RecordUpload() stored bytes = %v, want 1200", metrics.BytesStoredIPFS)
	}

	// Test storage efficiency calculation
	expectedEfficiency := float64(1200) / float64(1000) * 100.0
	if metrics.StorageEfficiency != expectedEfficiency {
		t.Errorf("Storage efficiency = %v, want %v", metrics.StorageEfficiency, expectedEfficiency)
	}
}

func TestClientEdgeCases(t *testing.T) {
	// Test with mock that validates inputs
	mockIPFS := &mockIPFSClient{
		storeBlockFunc: func(block *blocks.Block) (string, error) {
			if block == nil {
				return "", errors.New("block cannot be nil")
			}
			return "test_cid", nil
		},
		retrieveBlockFunc: func(cid string) (*blocks.Block, error) {
			if cid == "" {
				return nil, errors.New("CID cannot be empty")
			}
			return blocks.NewBlock([]byte("test"))
		},
	}
	
	cache := cache.NewMemoryCache(10)
	client, err := NewClient(mockIPFS, cache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Test SelectRandomizer with zero block size (blocks.NewRandomBlock should handle this)
	_, _, err = client.SelectRandomizer(0)
	if err == nil {
		t.Error("SelectRandomizer() should fail with zero block size")
	}

	// Test SelectRandomizer with negative block size
	_, _, err = client.SelectRandomizer(-1)
	if err == nil {
		t.Error("SelectRandomizer() should fail with negative block size")
	}

	// Test StoreBlockWithCache with nil block (mock will catch this)
	_, err = client.StoreBlockWithCache(nil)
	if err == nil {
		t.Error("StoreBlockWithCache() should fail with nil block")
	}

	// Test RetrieveBlockWithCache with empty CID (mock will catch this)
	_, err = client.RetrieveBlockWithCache("")
	if err == nil {
		t.Error("RetrieveBlockWithCache() should fail with empty CID")
	}
}

func TestSelectRandomizerCacheFiltering(t *testing.T) {
	mockIPFS := &mockIPFSClient{
		storeBlockFunc: func(block *blocks.Block) (string, error) {
			return "new_randomizer_cid", nil
		},
	}

	cache := cache.NewMemoryCache(10)
	client, err := NewClient(mockIPFS, cache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Add blocks of different sizes to cache
	smallBlock, err := blocks.NewRandomBlock(64)
	if err != nil {
		t.Fatalf("Failed to create small block: %v", err)
	}
	cache.Store("small_cid", smallBlock)

	largeBlock, err := blocks.NewRandomBlock(256)
	if err != nil {
		t.Fatalf("Failed to create large block: %v", err)
	}
	cache.Store("large_cid", largeBlock)

	// Request 128 byte randomizer - should not find suitable blocks and generate new one
	initialGenerated := client.GetMetrics().BlocksGenerated
	
	_, _, err = client.SelectRandomizer(128)
	if err != nil {
		t.Errorf("SelectRandomizer() error = %v, want nil", err)
	}

	finalGenerated := client.GetMetrics().BlocksGenerated
	
	// Should have generated a new block since cache had no 128-byte blocks
	if finalGenerated <= initialGenerated {
		t.Error("SelectRandomizer() should have generated new block when cache has no suitable blocks")
	}
}
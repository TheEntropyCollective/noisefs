package cache

import (
	"fmt"
	"testing"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
)

func TestNewMemoryCache(t *testing.T) {
	cache := NewMemoryCache(10)

	if cache == nil {
		t.Fatal("NewMemoryCache() returned nil")
	}

	if cache.capacity != 10 {
		t.Errorf("NewMemoryCache() capacity = %v, want 10", cache.capacity)
	}

	if cache.Size() != 0 {
		t.Errorf("NewMemoryCache() initial size = %v, want 0", cache.Size())
	}
}

func TestMemoryCacheStore(t *testing.T) {
	cache := NewMemoryCache(2)

	// Create test blocks
	block1, err := blocks.NewBlock([]byte("test data 1"))
	if err != nil {
		t.Fatalf("Failed to create block1: %v", err)
	}

	block2, err := blocks.NewBlock([]byte("test data 2"))
	if err != nil {
		t.Fatalf("Failed to create block2: %v", err)
	}

	// Test normal store
	err = cache.Store("cid1", block1)
	if err != nil {
		t.Errorf("Store() error = %v, want nil", err)
	}

	if cache.Size() != 1 {
		t.Errorf("After store, cache size = %v, want 1", cache.Size())
	}

	// Test store existing block (should not increase size)
	err = cache.Store("cid1", block1)
	if err != nil {
		t.Errorf("Store() existing block error = %v, want nil", err)
	}

	if cache.Size() != 1 {
		t.Errorf("After storing existing, cache size = %v, want 1", cache.Size())
	}

	// Test invalid inputs
	err = cache.Store("", block1)
	if err != ErrNotFound {
		t.Errorf("Store() with empty CID error = %v, want %v", err, ErrNotFound)
	}

	err = cache.Store("cid2", nil)
	if err != ErrNotFound {
		t.Errorf("Store() with nil block error = %v, want %v", err, ErrNotFound)
	}

	// Test capacity limit and eviction
	err = cache.Store("cid2", block2)
	if err != nil {
		t.Errorf("Store() second block error = %v, want nil", err)
	}

	if cache.Size() != 2 {
		t.Errorf("After storing second block, cache size = %v, want 2", cache.Size())
	}

	// Add third block to trigger eviction
	block3, err := blocks.NewBlock([]byte("test data 3"))
	if err != nil {
		t.Fatalf("Failed to create block3: %v", err)
	}

	err = cache.Store("cid3", block3)
	if err != nil {
		t.Errorf("Store() third block error = %v, want nil", err)
	}

	if cache.Size() != 2 {
		t.Errorf("After eviction, cache size = %v, want 2", cache.Size())
	}

	// First block should be evicted (LRU)
	if cache.Has("cid1") {
		t.Error("Oldest block was not evicted")
	}
}

func TestMemoryCacheGet(t *testing.T) {
	cache := NewMemoryCache(10)

	// Test get non-existent block
	_, err := cache.Get("nonexistent")
	if err != ErrNotFound {
		t.Errorf("Get() non-existent error = %v, want %v", err, ErrNotFound)
	}

	// Store a block and retrieve it
	block, err := blocks.NewBlock([]byte("test data"))
	if err != nil {
		t.Fatalf("Failed to create block: %v", err)
	}

	err = cache.Store("cid1", block)
	if err != nil {
		t.Fatalf("Failed to store block: %v", err)
	}

	retrieved, err := cache.Get("cid1")
	if err != nil {
		t.Errorf("Get() error = %v, want nil", err)
	}

	if retrieved == nil {
		t.Error("Get() returned nil block")
	}

	if retrieved.ID != block.ID {
		t.Errorf("Get() returned block with ID = %v, want %v", retrieved.ID, block.ID)
	}
}

func TestMemoryCacheHas(t *testing.T) {
	cache := NewMemoryCache(10)

	// Test non-existent block
	if cache.Has("nonexistent") {
		t.Error("Has() returned true for non-existent block")
	}

	// Store a block and check
	block, err := blocks.NewBlock([]byte("test data"))
	if err != nil {
		t.Fatalf("Failed to create block: %v", err)
	}

	err = cache.Store("cid1", block)
	if err != nil {
		t.Fatalf("Failed to store block: %v", err)
	}

	if !cache.Has("cid1") {
		t.Error("Has() returned false for existing block")
	}
}

func TestMemoryCacheRemove(t *testing.T) {
	cache := NewMemoryCache(10)

	// Test remove non-existent block
	err := cache.Remove("nonexistent")
	if err != ErrNotFound {
		t.Errorf("Remove() non-existent error = %v, want %v", err, ErrNotFound)
	}

	// Store a block and remove it
	block, err := blocks.NewBlock([]byte("test data"))
	if err != nil {
		t.Fatalf("Failed to create block: %v", err)
	}

	err = cache.Store("cid1", block)
	if err != nil {
		t.Fatalf("Failed to store block: %v", err)
	}

	if cache.Size() != 1 {
		t.Errorf("Before remove, cache size = %v, want 1", cache.Size())
	}

	err = cache.Remove("cid1")
	if err != nil {
		t.Errorf("Remove() error = %v, want nil", err)
	}

	if cache.Size() != 0 {
		t.Errorf("After remove, cache size = %v, want 0", cache.Size())
	}

	if cache.Has("cid1") {
		t.Error("Block still exists after removal")
	}
}

func TestMemoryCacheClear(t *testing.T) {
	cache := NewMemoryCache(10)

	// Store multiple blocks
	for i := 0; i < 5; i++ {
		block, err := blocks.NewBlock([]byte(fmt.Sprintf("test data %d", i)))
		if err != nil {
			t.Fatalf("Failed to create block %d: %v", i, err)
		}

		err = cache.Store(fmt.Sprintf("cid%d", i), block)
		if err != nil {
			t.Fatalf("Failed to store block %d: %v", i, err)
		}
	}

	if cache.Size() != 5 {
		t.Errorf("Before clear, cache size = %v, want 5", cache.Size())
	}

	cache.Clear()

	if cache.Size() != 0 {
		t.Errorf("After clear, cache size = %v, want 0", cache.Size())
	}

	// Check that no blocks exist
	for i := 0; i < 5; i++ {
		if cache.Has(fmt.Sprintf("cid%d", i)) {
			t.Errorf("Block cid%d still exists after clear", i)
		}
	}
}

func TestMemoryCachePopularity(t *testing.T) {
	cache := NewMemoryCache(10)

	// Test increment popularity for non-existent block
	err := cache.IncrementPopularity("nonexistent")
	if err != ErrNotFound {
		t.Errorf("IncrementPopularity() non-existent error = %v, want %v", err, ErrNotFound)
	}

	// Store blocks
	block1, err := blocks.NewBlock([]byte("test data 1"))
	if err != nil {
		t.Fatalf("Failed to create block1: %v", err)
	}

	block2, err := blocks.NewBlock([]byte("test data 2"))
	if err != nil {
		t.Fatalf("Failed to create block2: %v", err)
	}

	err = cache.Store("cid1", block1)
	if err != nil {
		t.Fatalf("Failed to store block1: %v", err)
	}

	err = cache.Store("cid2", block2)
	if err != nil {
		t.Fatalf("Failed to store block2: %v", err)
	}

	// Increment popularity of block1 multiple times
	for i := 0; i < 3; i++ {
		err = cache.IncrementPopularity("cid1")
		if err != nil {
			t.Errorf("IncrementPopularity() error = %v, want nil", err)
		}
	}

	// Increment popularity of block2 once
	err = cache.IncrementPopularity("cid2")
	if err != nil {
		t.Errorf("IncrementPopularity() error = %v, want nil", err)
	}

	// Get randomizers and check ordering
	randomizers, err := cache.GetRandomizers(2)
	if err != nil {
		t.Errorf("GetRandomizers() error = %v, want nil", err)
	}

	if len(randomizers) != 2 {
		t.Errorf("GetRandomizers() returned %d blocks, want 2", len(randomizers))
	}

	// First should be most popular (cid1)
	if randomizers[0].CID != "cid1" {
		t.Errorf("Most popular block CID = %v, want cid1", randomizers[0].CID)
	}

	if randomizers[0].Popularity != 3 {
		t.Errorf("Most popular block popularity = %v, want 3", randomizers[0].Popularity)
	}

	// Second should be less popular (cid2)
	if randomizers[1].CID != "cid2" {
		t.Errorf("Second popular block CID = %v, want cid2", randomizers[1].CID)
	}

	if randomizers[1].Popularity != 1 {
		t.Errorf("Second popular block popularity = %v, want 1", randomizers[1].Popularity)
	}
}

func TestMemoryCacheGetRandomizers(t *testing.T) {
	cache := NewMemoryCache(10)

	// Test empty cache
	randomizers, err := cache.GetRandomizers(5)
	if err != nil {
		t.Errorf("GetRandomizers() on empty cache error = %v, want nil", err)
	}

	if len(randomizers) != 0 {
		t.Errorf("GetRandomizers() on empty cache returned %d blocks, want 0", len(randomizers))
	}

	// Store some blocks
	testBlocks := make([]*blocks.Block, 3)
	for i := 0; i < 3; i++ {
		block, err := blocks.NewBlock([]byte(fmt.Sprintf("test data %d", i)))
		if err != nil {
			t.Fatalf("Failed to create block %d: %v", i, err)
		}
		testBlocks[i] = block

		err = cache.Store(fmt.Sprintf("cid%d", i), block)
		if err != nil {
			t.Fatalf("Failed to store block %d: %v", i, err)
		}
	}

	// Test getting more randomizers than available
	randomizers, err = cache.GetRandomizers(10)
	if err != nil {
		t.Errorf("GetRandomizers() error = %v, want nil", err)
	}

	if len(randomizers) != 3 {
		t.Errorf("GetRandomizers() returned %d blocks, want 3", len(randomizers))
	}

	// Test getting fewer randomizers than available
	randomizers, err = cache.GetRandomizers(2)
	if err != nil {
		t.Errorf("GetRandomizers() error = %v, want nil", err)
	}

	if len(randomizers) != 2 {
		t.Errorf("GetRandomizers() returned %d blocks, want 2", len(randomizers))
	}

	// Verify BlockInfo fields are set correctly
	for _, info := range randomizers {
		if info.CID == "" {
			t.Error("BlockInfo has empty CID")
		}

		if info.Block == nil {
			t.Error("BlockInfo has nil Block")
		}

		if info.Size != info.Block.Size() {
			t.Errorf("BlockInfo Size = %v, want %v", info.Size, info.Block.Size())
		}

		if info.Popularity < 0 {
			t.Errorf("BlockInfo Popularity = %v, want >= 0", info.Popularity)
		}
	}
}

func TestMemoryCacheLRUOrdering(t *testing.T) {
	cache := NewMemoryCache(3)

	// Store blocks
	testBlocks := make([]*blocks.Block, 4)
	for i := 0; i < 4; i++ {
		block, err := blocks.NewBlock([]byte(fmt.Sprintf("test data %d", i)))
		if err != nil {
			t.Fatalf("Failed to create block %d: %v", i, err)
		}
		testBlocks[i] = block
	}

	// Fill cache to capacity
	for i := 0; i < 3; i++ {
		err := cache.Store(fmt.Sprintf("cid%d", i), testBlocks[i])
		if err != nil {
			t.Fatalf("Failed to store block %d: %v", i, err)
		}
	}

	// Access block 0 to make it most recently used
	_, err := cache.Get("cid0")
	if err != nil {
		t.Fatalf("Failed to get cid0: %v", err)
	}

	// Add a new block, should evict cid1 (oldest unaccessed)
	err = cache.Store("cid3", testBlocks[3])
	if err != nil {
		t.Fatalf("Failed to store block 3: %v", err)
	}

	// cid1 should be evicted
	if cache.Has("cid1") {
		t.Error("cid1 was not evicted as expected")
	}

	// cid0, cid2, cid3 should still exist
	for _, cid := range []string{"cid0", "cid2", "cid3"} {
		if !cache.Has(cid) {
			t.Errorf("Block %s was unexpectedly evicted", cid)
		}
	}
}

func TestMemoryCacheZeroCapacity(t *testing.T) {
	cache := NewMemoryCache(0)

	block, err := blocks.NewBlock([]byte("test data"))
	if err != nil {
		t.Fatalf("Failed to create block: %v", err)
	}

	// Should allow unlimited storage with 0 capacity
	for i := 0; i < 100; i++ {
		err = cache.Store(fmt.Sprintf("cid%d", i), block)
		if err != nil {
			t.Errorf("Store() with zero capacity error = %v, want nil", err)
		}
	}

	if cache.Size() != 100 {
		t.Errorf("Zero capacity cache size = %v, want 100", cache.Size())
	}
}

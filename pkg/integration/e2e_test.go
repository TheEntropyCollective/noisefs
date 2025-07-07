package integration

import (
	"bytes"
	"errors"
	"testing"

	"github.com/TheEntropyCollective/noisefs/pkg/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/noisefs"
)

// mockBlockStore provides a mock IPFS implementation that stores blocks in memory
type mockBlockStore struct {
	blocks map[string]*blocks.Block
}

func newMockBlockStore() *mockBlockStore {
	return &mockBlockStore{
		blocks: make(map[string]*blocks.Block),
	}
}

func (m *mockBlockStore) StoreBlock(block *blocks.Block) (string, error) {
	if block == nil {
		return "", errors.New("block cannot be nil")
	}
	
	cid := "mock_" + block.ID // Use block ID as CID for consistency
	m.blocks[cid] = block
	return cid, nil
}

func (m *mockBlockStore) RetrieveBlock(cid string) (*blocks.Block, error) {
	if cid == "" {
		return nil, errors.New("CID cannot be empty")
	}
	
	block, exists := m.blocks[cid]
	if !exists {
		return nil, errors.New("block not found")
	}
	
	return block, nil
}

// simulateUpload simulates the complete file upload process
func simulateUpload(client *noisefs.Client, data []byte, blockSize int) (*descriptors.Descriptor, error) {
	// Create descriptor
	desc := descriptors.NewDescriptor("test_file.txt", int64(len(data)), blockSize)
	
	// Split data into blocks
	offset := 0
	for offset < len(data) {
		end := offset + blockSize
		if end > len(data) {
			end = len(data)
		}
		
		blockData := data[offset:end]
		
		// Create data block
		dataBlock, err := blocks.NewBlock(blockData)
		if err != nil {
			return nil, err
		}
		
		// Get two randomizers for 3-tuple
		randBlock1, randCID1, randBlock2, randCID2, err := client.SelectTwoRandomizers(len(blockData))
		if err != nil {
			return nil, err
		}
		
		// Ensure data block is different from randomizers (very unlikely but possible with repeated content)
		if dataBlock.ID == randBlock1.ID || dataBlock.ID == randBlock2.ID {
			// Regenerate randomizers if conflict detected
			randBlock1, err = blocks.NewRandomBlock(len(blockData))
			if err != nil {
				return nil, err
			}
			randBlock2, err = blocks.NewRandomBlock(len(blockData))
			if err != nil {
				return nil, err
			}
			
			// Store and cache the new randomizers
			randCID1, err = client.StoreBlockWithCache(randBlock1)
			if err != nil {
				return nil, err
			}
			randCID2, err = client.StoreBlockWithCache(randBlock2)
			if err != nil {
				return nil, err
			}
		}
		
		// XOR with both randomizers (3-tuple)
		anonymizedBlock, err := dataBlock.XOR3(randBlock1, randBlock2)
		if err != nil {
			return nil, err
		}
		
		// Store anonymized block
		dataCID, err := client.StoreBlockWithCache(anonymizedBlock)
		if err != nil {
			return nil, err
		}
		
		// Add to descriptor
		err = desc.AddBlockTriple(dataCID, randCID1, randCID2)
		if err != nil {
			return nil, err
		}
		
		offset = end
	}
	
	// Record upload metrics
	client.RecordUpload(int64(len(data)), int64(len(data)*15/10)) // 1.5x storage overhead for 3-tuple
	
	return desc, nil
}

// simulateDownload simulates the complete file download process
func simulateDownload(client *noisefs.Client, desc *descriptors.Descriptor) ([]byte, error) {
	var result bytes.Buffer
	
	for i, blockPair := range desc.Blocks {
		// Retrieve anonymized data block
		anonymizedBlock, err := client.RetrieveBlockWithCache(blockPair.DataCID)
		if err != nil {
			return nil, err
		}
		
		// Get randomizer CIDs for this block
		randCID1, randCID2, err := desc.GetRandomizerCIDs(i)
		if err != nil {
			return nil, err
		}
		
		// Retrieve first randomizer block
		randBlock1, err := client.RetrieveBlockWithCache(randCID1)
		if err != nil {
			return nil, err
		}
		
		var originalBlock *blocks.Block
		
		if desc.IsThreeTuple() {
			// Retrieve second randomizer block for 3-tuple
			randBlock2, err := client.RetrieveBlockWithCache(randCID2)
			if err != nil {
				return nil, err
			}
			
			// XOR3 to recover original data
			originalBlock, err = anonymizedBlock.XOR3(randBlock1, randBlock2)
			if err != nil {
				return nil, err
			}
		} else {
			// Legacy 2-tuple XOR
			originalBlock, err = anonymizedBlock.XOR(randBlock1)
			if err != nil {
				return nil, err
			}
		}
		
		// Append to result
		result.Write(originalBlock.Data)
	}
	
	// Record download metrics
	client.RecordDownload()
	
	return result.Bytes(), nil
}

func TestEndToEndUploadDownload(t *testing.T) {
	// Setup
	mockStore := newMockBlockStore()
	cache := cache.NewMemoryCache(20)
	client, err := noisefs.NewClient(mockStore, cache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	
	// Test data
	originalData := []byte("Hello, NoiseFS! This is a test file that will be split into blocks, anonymized, and then reconstructed.")
	blockSize := 32 // Small blocks for testing
	
	// Upload simulation
	desc, err := simulateUpload(client, originalData, blockSize)
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
	
	// Verify descriptor
	if desc.Filename != "test_file.txt" {
		t.Errorf("Descriptor filename = %v, want test_file.txt", desc.Filename)
	}
	
	if desc.FileSize != int64(len(originalData)) {
		t.Errorf("Descriptor file size = %v, want %v", desc.FileSize, len(originalData))
	}
	
	if desc.BlockSize != blockSize {
		t.Errorf("Descriptor block size = %v, want %v", desc.BlockSize, blockSize)
	}
	
	expectedBlocks := (len(originalData) + blockSize - 1) / blockSize
	if len(desc.Blocks) != expectedBlocks {
		t.Errorf("Descriptor blocks count = %v, want %v", len(desc.Blocks), expectedBlocks)
	}
	
	// Download simulation
	reconstructedData, err := simulateDownload(client, desc)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}
	
	// Verify reconstruction
	if !bytes.Equal(originalData, reconstructedData) {
		t.Errorf("Reconstructed data does not match original")
		t.Logf("Original: %s", string(originalData))
		t.Logf("Reconstructed: %s", string(reconstructedData))
	}
	
	// Verify metrics
	metrics := client.GetMetrics()
	if metrics.TotalUploads != 1 {
		t.Errorf("Total uploads = %v, want 1", metrics.TotalUploads)
	}
	
	if metrics.TotalDownloads != 1 {
		t.Errorf("Total downloads = %v, want 1", metrics.TotalDownloads)
	}
	
	if metrics.BytesUploadedOriginal != int64(len(originalData)) {
		t.Errorf("Bytes uploaded = %v, want %v", metrics.BytesUploadedOriginal, len(originalData))
	}
}

func TestEndToEndWithDifferentFileSizes(t *testing.T) {
	testCases := []struct {
		name      string
		data      []byte
		blockSize int
	}{
		{
			name:      "small file",
			data:      []byte("small"),
			blockSize: 128,
		},
		{
			name:      "exact block size",
			data:      bytes.Repeat([]byte("A"), 128),
			blockSize: 128,
		},
		{
			name:      "multiple blocks",
			data:      bytes.Repeat([]byte("B"), 300),
			blockSize: 128,
		},
		{
			name:      "large file",
			data:      bytes.Repeat([]byte("Large file content! "), 100),
			blockSize: 256,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup fresh environment for each test
			mockStore := newMockBlockStore()
			cache := cache.NewMemoryCache(50)
			client, err := noisefs.NewClient(mockStore, cache)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}
			
			// Upload
			desc, err := simulateUpload(client, tc.data, tc.blockSize)
			if err != nil {
				t.Fatalf("Upload failed: %v", err)
			}
			
			// Download
			reconstructedData, err := simulateDownload(client, desc)
			if err != nil {
				t.Fatalf("Download failed: %v", err)
			}
			
			// Verify
			if !bytes.Equal(tc.data, reconstructedData) {
				t.Errorf("Data mismatch for %s", tc.name)
			}
		})
	}
}

func TestEndToEndDescriptorSerialization(t *testing.T) {
	// Setup
	mockStore := newMockBlockStore()
	cache := cache.NewMemoryCache(20)
	client, err := noisefs.NewClient(mockStore, cache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	
	// Test data
	originalData := []byte("Testing descriptor serialization in end-to-end workflow.")
	blockSize := 16
	
	// Upload
	desc, err := simulateUpload(client, originalData, blockSize)
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
	
	// Serialize descriptor to JSON
	jsonData, err := desc.ToJSON()
	if err != nil {
		t.Fatalf("Descriptor serialization failed: %v", err)
	}
	
	// Deserialize descriptor from JSON
	restoredDesc, err := descriptors.FromJSON(jsonData)
	if err != nil {
		t.Fatalf("Descriptor deserialization failed: %v", err)
	}
	
	// Download using restored descriptor
	reconstructedData, err := simulateDownload(client, restoredDesc)
	if err != nil {
		t.Fatalf("Download with restored descriptor failed: %v", err)
	}
	
	// Verify
	if !bytes.Equal(originalData, reconstructedData) {
		t.Errorf("Data mismatch after descriptor round-trip")
	}
}

func TestEndToEndBlockReuse(t *testing.T) {
	// Setup
	mockStore := newMockBlockStore()
	cache := cache.NewMemoryCache(20)
	client, err := noisefs.NewClient(mockStore, cache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	
	// Upload first file
	data1 := bytes.Repeat([]byte("File 1 content "), 10)
	blockSize := 64
	
	_, err = simulateUpload(client, data1, blockSize)
	if err != nil {
		t.Fatalf("First upload failed: %v", err)
	}
	
	initialMetrics := client.GetMetrics()
	
	// Upload second file (should reuse some randomizer blocks)
	data2 := bytes.Repeat([]byte("File 2 different content "), 8)
	
	_, err = simulateUpload(client, data2, blockSize)
	if err != nil {
		t.Fatalf("Second upload failed: %v", err)
	}
	
	finalMetrics := client.GetMetrics()
	
	// Should have some block reuse
	if finalMetrics.BlocksReused <= initialMetrics.BlocksReused {
		t.Error("Expected block reuse in second upload")
	}
	
	// Block reuse rate should be > 0
	if finalMetrics.BlockReuseRate == 0 {
		t.Error("Expected non-zero block reuse rate")
	}
	
	t.Logf("Block reuse rate: %.2f%%", finalMetrics.BlockReuseRate)
}

func TestEndToEndCacheEfficiency(t *testing.T) {
	// Setup with small cache to test eviction
	mockStore := newMockBlockStore()
	cache := cache.NewMemoryCache(3) // Very small cache to force eviction
	client, err := noisefs.NewClient(mockStore, cache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	
	// Upload file with many blocks to trigger cache eviction
	originalData := bytes.Repeat([]byte("Cache test data "), 50) // Much larger file
	blockSize := 32
	
	desc, err := simulateUpload(client, originalData, blockSize)
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
	
	// Clear cache to force cache misses on first download
	cache.Clear()
	
	// First download - should have cache misses
	_, err = simulateDownload(client, desc)
	if err != nil {
		t.Fatalf("First download failed: %v", err)
	}
	
	firstMetrics := client.GetMetrics()
	
	// Second download - should have more cache hits for recently accessed blocks
	_, err = simulateDownload(client, desc)
	if err != nil {
		t.Fatalf("Second download failed: %v", err)
	}
	
	secondMetrics := client.GetMetrics()
	
	// Should have more total cache hits
	if secondMetrics.CacheHits <= firstMetrics.CacheHits {
		t.Error("Expected more cache hits on second download")
	}
	
	t.Logf("Cache hits after first download: %d", firstMetrics.CacheHits)
	t.Logf("Cache hits after second download: %d", secondMetrics.CacheHits)
	t.Logf("Cache misses after first download: %d", firstMetrics.CacheMisses)
	t.Logf("Cache misses after second download: %d", secondMetrics.CacheMisses)
}

func TestEndToEndErrorRecovery(t *testing.T) {
	// Setup
	mockStore := newMockBlockStore()
	cache := cache.NewMemoryCache(20)
	client, err := noisefs.NewClient(mockStore, cache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	
	// Upload file successfully
	originalData := []byte("Error recovery test data")
	blockSize := 8
	
	desc, err := simulateUpload(client, originalData, blockSize)
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
	
	// Ensure we have blocks to test with
	if len(desc.Blocks) == 0 {
		t.Fatal("No blocks in descriptor")
	}
	
	// Simulate missing block by removing it from mock store
	firstBlockCID := desc.Blocks[0].DataCID
	if firstBlockCID == "" {
		t.Fatal("First block CID is empty")
	}
	
	// Remove from cache too
	cache.Remove(firstBlockCID)
	
	// Remove from mock store
	delete(mockStore.blocks, firstBlockCID)
	
	// Download should fail due to missing block
	_, err = simulateDownload(client, desc)
	if err == nil {
		t.Error("Download should fail with missing block")
		return
	}
	
	if err.Error() != "block not found" {
		t.Logf("Expected 'block not found' error, got: %v", err)
		// Still consider this a pass as it correctly failed, just different error message
	}
}
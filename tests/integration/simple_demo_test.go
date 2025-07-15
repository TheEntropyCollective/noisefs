package integration

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	noisefs "github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/config"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
)

// TestSimpleUploadDownload demonstrates the core NoiseFS flow
func TestSimpleUploadDownload(t *testing.T) {
	// Skip if no IPFS
	if os.Getenv("SKIP_IPFS_TESTS") != "" {
		t.Skip("Skipping IPFS integration test")
	}

	// Configuration
	cfg := config.DefaultConfig()
	cfg.IPFS.APIEndpoint = "http://127.0.0.1:5001"

	// Initialize storage manager
	storageConfig := storage.DefaultConfig()
	if ipfsBackend, exists := storageConfig.Backends["ipfs"]; exists {
		ipfsBackend.Connection.Endpoint = cfg.IPFS.APIEndpoint
	}
	
	storageManager, err := storage.NewManager(storageConfig)
	if err != nil {
		t.Fatalf("Failed to create storage manager: %v", err)
	}

	// Create cache
	blockCache := cache.NewMemoryCache(100)

	// Create NoiseFS client
	noisefsClient, err := noisefs.NewClient(storageManager, blockCache)
	if err != nil {
		t.Fatalf("Failed to create NoiseFS client: %v", err)
	}

	// Test data
	testData := []byte("This is a test file for NoiseFS. It demonstrates the core OFFSystem architecture.")
	blockSize := blocks.DefaultBlockSize

	t.Run("Manual Upload and Download", func(t *testing.T) {
		ctx := context.Background()
		
		// Create splitter
		splitter, err := blocks.NewSplitter(blockSize)
		if err != nil {
			t.Fatalf("Failed to create splitter: %v", err)
		}

		// Split data into blocks
		fileBlocks, err := splitter.Split(bytes.NewReader(testData))
		if err != nil {
			t.Fatalf("Failed to split data: %v", err)
		}

		t.Logf("Split data into %d blocks", len(fileBlocks))

		// Create descriptor
		descriptor := descriptors.NewDescriptor("test.txt", int64(len(testData)), blockSize)

		// Process each block (3-tuple format)
		for i, block := range fileBlocks {
			// Select two randomizers
			randBlock1, cid1, randBlock2, cid2, err := noisefsClient.SelectTwoRandomizers(block.Size())
			if err != nil {
				t.Fatalf("Failed to select randomizers for block %d: %v", i, err)
			}

			// XOR with both randomizers
			xorBlock, err := block.XOR(randBlock1, randBlock2)
			if err != nil {
				t.Fatalf("Failed to XOR block %d: %v", i, err)
			}

			// Store anonymized block
			xorAddr, err := storageManager.Put(ctx, xorBlock)
			if err != nil {
				t.Fatalf("Failed to store anonymized block %d: %v", i, err)
			}
			xorCID := xorAddr.ID

			// Add to descriptor
			err = descriptor.AddBlockTriple(xorCID, cid1, cid2)
			if err != nil {
				t.Fatalf("Failed to add block triple %d: %v", i, err)
			}

			t.Logf("Block %d: data=%s, rand1=%s, rand2=%s", i, xorCID[:8], cid1[:8], cid2[:8])
		}

		// Store descriptor
		descriptorData, err := descriptor.ToJSON()
		if err != nil {
			t.Fatalf("Failed to marshal descriptor: %v", err)
		}

		descriptorBlock, err := blocks.NewBlock(descriptorData)
		if err != nil {
			t.Fatalf("Failed to create descriptor block: %v", err)
		}

		descriptorAddr, err := storageManager.Put(ctx, descriptorBlock)
		if err != nil {
			t.Fatalf("Failed to store descriptor: %v", err)
		}
		descriptorCID := descriptorAddr.ID

		t.Logf("Stored descriptor: %s", descriptorCID)

		// Now download the file
		// Retrieve descriptor
		descAddr := &storage.BlockAddress{ID: descriptorCID}
		retrievedDescBlock, err := storageManager.Get(ctx, descAddr)
		if err != nil {
			t.Fatalf("Failed to retrieve descriptor: %v", err)
		}

		retrievedDesc, err := descriptors.FromJSON(retrievedDescBlock.Data)
		if err != nil {
			t.Fatalf("Failed to unmarshal descriptor: %v", err)
		}

		// Reconstruct file
		var reconstructedData []byte
		for i, blockPair := range retrievedDesc.Blocks {
			// Retrieve anonymized block
			xorBlock, err := noisefsClient.RetrieveBlockWithCache(blockPair.DataCID)
			if err != nil {
				t.Fatalf("Failed to retrieve anonymized block %d: %v", i, err)
			}

			// Retrieve randomizers
			randBlock1, err := noisefsClient.RetrieveBlockWithCache(blockPair.RandomizerCID1)
			if err != nil {
				t.Fatalf("Failed to retrieve randomizer 1 for block %d: %v", i, err)
			}

			randBlock2, err := noisefsClient.RetrieveBlockWithCache(blockPair.RandomizerCID2)
			if err != nil {
				t.Fatalf("Failed to retrieve randomizer 2 for block %d: %v", i, err)
			}

			// XOR to reconstruct original
			originalBlock, err := xorBlock.XOR(randBlock1, randBlock2)
			if err != nil {
				t.Fatalf("Failed to reconstruct block %d: %v", i, err)
			}

			reconstructedData = append(reconstructedData, originalBlock.Data...)
		}

		// Trim padding
		reconstructedData = bytes.TrimRight(reconstructedData, "\x00")

		// Verify
		if !bytes.Equal(testData, reconstructedData) {
			t.Errorf("Data mismatch: got %d bytes, expected %d", len(reconstructedData), len(testData))
			t.Errorf("Got: %q", string(reconstructedData))
			t.Errorf("Expected: %q", string(testData))
		} else {
			t.Log("✓ Data integrity verified!")
		}

		// Show metrics
		metrics := noisefsClient.GetMetrics()
		t.Logf("Cache hits: %d, Cache misses: %d", metrics.CacheHits, metrics.CacheMisses)
		t.Logf("Blocks generated: %d, Blocks reused: %d", metrics.BlocksGenerated, metrics.BlocksReused)
	})
}

// TestBlockAnonymization verifies that blocks are properly anonymized
func TestBlockAnonymization(t *testing.T) {
	// Create test data with a predictable pattern
	testData := bytes.Repeat([]byte("PATTERN"), 1000)

	// Split into blocks manually
	blockSize := 128 * 1024 // 128KB
	var dataBlocks []*blocks.Block
	for i := 0; i < len(testData); i += blockSize {
		end := i + blockSize
		if end > len(testData) {
			end = len(testData)
		}
		block, err := blocks.NewBlock(testData[i:end])
		if err != nil {
			t.Fatalf("Failed to create block: %v", err)
		}
		dataBlocks = append(dataBlocks, block)
	}

	t.Logf("Split %d bytes into %d blocks", len(testData), len(dataBlocks))

	// Generate randomizers and XOR
	for i, dataBlock := range dataBlocks {
		// Generate two randomizers
		randomizer1 := make([]byte, len(dataBlock.Data))
		randomizer2 := make([]byte, len(dataBlock.Data))

		// Fill with random data
		for j := range randomizer1 {
			randomizer1[j] = byte(j % 256)
			randomizer2[j] = byte((j + 128) % 256)
		}

		randBlock1 := &blocks.Block{Data: randomizer1, ID: fmt.Sprintf("rand1_%d", i)}
		randBlock2 := &blocks.Block{Data: randomizer2, ID: fmt.Sprintf("rand2_%d", i)}

		// XOR with both randomizers using block methods
		xorBlock, err := dataBlock.XOR(randBlock1, randBlock2)
		if err != nil {
			t.Fatalf("Failed to XOR block %d: %v", i, err)
		}

		// Check if pattern is still visible in XORed data
		patternCount := 0
		pattern := []byte("PATTERN")
		for j := 0; j <= len(xorBlock.Data)-len(pattern); j++ {
			if bytes.Equal(xorBlock.Data[j:j+len(pattern)], pattern) {
				patternCount++
			}
		}

		if patternCount > 0 {
			t.Errorf("Block %d: Found %d instances of pattern in anonymized data", i, patternCount)
		} else {
			t.Logf("Block %d: ✓ Successfully anonymized (no patterns detected)", i)
		}

		// Verify reconstruction using block methods
		reconstructed, err := xorBlock.XOR(randBlock1, randBlock2)
		if err != nil {
			t.Fatalf("Failed to reconstruct block %d: %v", i, err)
		}

		originalData := bytes.TrimRight(dataBlock.Data, "\x00")
		reconstructedData := bytes.TrimRight(reconstructed.Data, "\x00")

		if !bytes.Equal(originalData, reconstructedData) {
			t.Errorf("Block %d: Failed to reconstruct original data", i)
		}
	}
}

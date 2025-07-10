package integration

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/config"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/ipfs"
	"github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/TheEntropyCollective/noisefs/pkg/privacy/reuse"
)

// TestEndToEndFlow demonstrates the complete upload/download cycle
func TestEndToEndFlow(t *testing.T) {
	// Skip if no IPFS is available
	if !isIPFSAvailable() {
		t.Skip("IPFS not available, skipping end-to-end test")
	}

	// Create test configuration
	cfg := config.DefaultConfig()
	cfg.IPFS.URL = "http://localhost:5001"
	
	// Initialize components
	ipfsClient, err := ipfs.NewClient(cfg.IPFS.URL)
	if err != nil {
		t.Fatalf("Failed to create IPFS client: %v", err)
	}

	// Create cache
	blockCache := cache.NewMemoryCache(100)

	// Create NoiseFS client
	noisefsClient, err := noisefs.NewClient(ipfsClient, blockCache)
	if err != nil {
		t.Fatalf("Failed to create NoiseFS client: %v", err)
	}

	// Test data
	testData := []byte("This is a test file for NoiseFS. It contains enough data to span multiple blocks when split. " +
		"The OFFSystem architecture ensures that no original content is stored directly - everything is XORed with randomizer blocks. " +
		"This provides plausible deniability while maintaining efficient storage through block reuse.")

	t.Run("Basic Upload and Download", func(t *testing.T) {
		// Upload file
		reader := bytes.NewReader(testData)
		descriptor, err := noisefsClient.Upload(reader, "test.txt")
		if err != nil {
			t.Fatalf("Upload failed: %v", err)
		}

		t.Logf("File uploaded successfully. Descriptor CID: %s", descriptor.CID)

		// Verify descriptor structure
		if len(descriptor.Blocks) == 0 {
			t.Fatal("Descriptor has no blocks")
		}

		// Verify blocks are anonymized (should look random)
		for i, block := range descriptor.Blocks {
			if block.DataCID == "" || block.RandomizerCID == "" {
				t.Errorf("Block %d missing CID: data=%s, randomizer=%s", i, block.DataCID, block.RandomizerCID)
			}
			t.Logf("Block %d: data=%s, randomizer=%s", i, block.DataCID, block.RandomizerCID)
		}

		// Download file
		downloadedData, err := noisefsClient.Download(descriptor.CID)
		if err != nil {
			t.Fatalf("Download failed: %v", err)
		}

		// Read downloaded data
		buf := new(bytes.Buffer)
		_, err = io.Copy(buf, downloadedData)
		if err != nil {
			t.Fatalf("Failed to read downloaded data: %v", err)
		}

		// Verify data integrity
		if !bytes.Equal(testData, buf.Bytes()) {
			t.Errorf("Downloaded data doesn't match original. Got %d bytes, expected %d", buf.Len(), len(testData))
		}

		t.Log("✓ File uploaded and downloaded successfully with data integrity verified")
	})

	t.Run("Block Anonymization Verification", func(t *testing.T) {
		// Create known test data
		knownData := bytes.Repeat([]byte("AAAA"), 1024) // Predictable pattern
		
		// Upload file
		reader := bytes.NewReader(knownData)
		descriptor, err := noisefsClient.Upload(reader, "pattern.txt")
		if err != nil {
			t.Fatalf("Upload failed: %v", err)
		}

		// Retrieve and verify anonymized blocks
		for i, block := range descriptor.Blocks {
			// Get the anonymized data block
			dataBlock, err := ipfsClient.GetBlock(block.DataCID)
			if err != nil {
				t.Errorf("Failed to retrieve data block %d: %v", i, err)
				continue
			}

			// Check if data appears random (anonymized)
			if isPatternDetectable(dataBlock) {
				t.Errorf("Block %d appears to contain detectable pattern - anonymization may have failed", i)
			}

			t.Logf("✓ Block %d verified as anonymized (appears random)", i)
		}
	})

	t.Run("Multi-File Block Reuse", func(t *testing.T) {
		// Enable reuse system if available
		reuseClient, err := reuse.NewReuseAwareClient(ipfsClient, blockCache)
		if err == nil {
			// Upload first file
			file1Data := []byte("File 1: This content will be mixed with public domain blocks for plausible deniability.")
			result1, err := reuseClient.UploadFile(bytes.NewReader(file1Data), "file1.txt", blocks.DefaultBlockSize)
			if err != nil {
				t.Logf("Reuse upload not available: %v", err)
				t.Skip("Skipping reuse test - reuse system not fully initialized")
			}

			// Upload second file
			file2Data := []byte("File 2: This content should reuse some blocks from the pool, demonstrating storage efficiency.")
			result2, err := reuseClient.UploadFile(bytes.NewReader(file2Data), "file2.txt", blocks.DefaultBlockSize)
			if err != nil {
				t.Fatalf("Second upload failed: %v", err)
			}

			// Check reuse statistics
			if result1.ReuseProof != nil && result2.ReuseProof != nil {
				t.Logf("File 1 reuse count: %d", result1.ReuseProof.TotalReuses)
				t.Logf("File 2 reuse count: %d", result2.ReuseProof.TotalReuses)
				
				if result2.ReuseProof.TotalReuses > 0 {
					t.Log("✓ Block reuse demonstrated - storage efficiency achieved")
				}
			}
		} else {
			t.Logf("Reuse system not available: %v", err)
		}
	})

	t.Run("Streaming Large File", func(t *testing.T) {
		// Create a larger file (1MB)
		largeData := make([]byte, 1024*1024)
		_, err := rand.Read(largeData)
		if err != nil {
			t.Fatalf("Failed to generate test data: %v", err)
		}

		// Upload large file
		start := time.Now()
		descriptor, err := noisefsClient.Upload(bytes.NewReader(largeData), "large.bin")
		if err != nil {
			t.Fatalf("Large file upload failed: %v", err)
		}
		uploadTime := time.Since(start)

		t.Logf("Large file uploaded in %v. Descriptor: %s", uploadTime, descriptor.CID)
		t.Logf("File split into %d blocks", len(descriptor.Blocks))

		// Download and verify
		start = time.Now()
		downloaded, err := noisefsClient.Download(descriptor.CID)
		if err != nil {
			t.Fatalf("Large file download failed: %v", err)
		}

		// Stream download verification
		buf := make([]byte, 4096)
		totalRead := 0
		for {
			n, err := downloaded.Read(buf)
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("Error reading downloaded stream: %v", err)
			}
			totalRead += n
		}
		downloadTime := time.Since(start)

		t.Logf("Large file downloaded in %v. Total bytes: %d", downloadTime, totalRead)
		
		if totalRead != len(largeData) {
			t.Errorf("Downloaded size mismatch: got %d, expected %d", totalRead, len(largeData))
		}

		t.Log("✓ Large file streaming verified")
	})

	t.Run("Descriptor Storage and Retrieval", func(t *testing.T) {
		// Create descriptor store
		store := descriptors.NewStore()

		// Upload a file
		testData := []byte("Testing descriptor storage")
		descriptor, err := noisefsClient.Upload(bytes.NewReader(testData), "desc_test.txt")
		if err != nil {
			t.Fatalf("Upload failed: %v", err)
		}

		// Store descriptor
		ctx := context.Background()
		err = store.StoreDescriptor(ctx, descriptor)
		if err != nil {
			t.Fatalf("Failed to store descriptor: %v", err)
		}

		// Retrieve descriptor
		retrieved, err := store.GetDescriptor(ctx, descriptor.CID)
		if err != nil {
			t.Fatalf("Failed to retrieve descriptor: %v", err)
		}

		// Verify descriptor integrity
		if retrieved.CID != descriptor.CID {
			t.Errorf("Retrieved descriptor CID mismatch")
		}
		if len(retrieved.Blocks) != len(descriptor.Blocks) {
			t.Errorf("Retrieved descriptor has different block count")
		}

		t.Log("✓ Descriptor storage and retrieval verified")
	})
}

// Helper function to check if IPFS is available
func isIPFSAvailable() bool {
	client, err := ipfs.NewClient("http://localhost:5001")
	if err != nil {
		return false
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	_, err = client.ID(ctx)
	return err == nil
}

// Helper function to detect patterns in data
func isPatternDetectable(data []byte) bool {
	if len(data) < 4 {
		return false
	}

	// Simple pattern detection: check for repeated sequences
	patternCount := 0
	for i := 0; i < len(data)-4; i++ {
		if bytes.Equal(data[i:i+4], data[i+4:i+8]) {
			patternCount++
		}
	}

	// If more than 10% of the data shows patterns, it's detectable
	threshold := len(data) / 40 // 2.5%
	return patternCount > threshold
}

// TestBlockSplittingAndXOR verifies the core XOR anonymization
func TestBlockSplittingAndXOR(t *testing.T) {
	// Create block manager
	blockManager := blocks.NewBlockManager(blocks.DefaultBlockConfig())

	// Test data
	testData := []byte("This is test data that will be split into blocks and XORed")

	// Split into blocks
	dataBlocks, err := blockManager.SplitIntoBlocks(bytes.NewReader(testData))
	if err != nil {
		t.Fatalf("Failed to split data: %v", err)
	}

	t.Logf("Data split into %d blocks", len(dataBlocks))

	// Generate randomizer blocks
	var randomizerBlocks []blocks.Block
	for range dataBlocks {
		randomizer := make([]byte, blocks.DefaultBlockSize)
		_, err := rand.Read(randomizer)
		if err != nil {
			t.Fatalf("Failed to generate randomizer: %v", err)
		}
		randomizerBlocks = append(randomizerBlocks, blocks.Block{
			Data: randomizer,
			ID:   hex.EncodeToString(randomizer[:16]),
		})
	}

	// XOR blocks
	var xoredBlocks []blocks.Block
	for i, dataBlock := range dataBlocks {
		xored := blockManager.XORBlocks(dataBlock, randomizerBlocks[i])
		xoredBlocks = append(xoredBlocks, xored)
		
		// Verify XOR property: data XOR randomizer XOR randomizer = data
		recovered := blockManager.XORBlocks(xored, randomizerBlocks[i])
		if !bytes.Equal(recovered.Data, dataBlock.Data) {
			t.Errorf("XOR recovery failed for block %d", i)
		}
	}

	// Reconstruct data
	var reconstructedData []byte
	for i, xoredBlock := range xoredBlocks {
		recovered := blockManager.XORBlocks(xoredBlock, randomizerBlocks[i])
		reconstructedData = append(reconstructedData, recovered.Data...)
	}

	// Trim padding and verify
	reconstructedData = bytes.TrimRight(reconstructedData, "\x00")
	if !bytes.Equal(reconstructedData, testData) {
		t.Errorf("Reconstructed data doesn't match original")
		t.Logf("Original: %s", testData)
		t.Logf("Reconstructed: %s", reconstructedData)
	}

	t.Log("✓ Block splitting and XOR anonymization verified")
}

// BenchmarkUploadDownload measures performance
func BenchmarkUploadDownload(b *testing.B) {
	if !isIPFSAvailable() {
		b.Skip("IPFS not available")
	}

	// Setup
	ipfsClient, _ := ipfs.NewClient("http://localhost:5001")
	blockCache := cache.NewMemoryCache(100)
	noisefsClient, _ := noisefs.NewClient(ipfsClient, blockCache)

	// Test data (10KB)
	testData := make([]byte, 10*1024)
	rand.Read(testData)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Upload
		descriptor, err := noisefsClient.Upload(bytes.NewReader(testData), fmt.Sprintf("bench_%d.dat", i))
		if err != nil {
			b.Fatalf("Upload failed: %v", err)
		}

		// Download
		reader, err := noisefsClient.Download(descriptor.CID)
		if err != nil {
			b.Fatalf("Download failed: %v", err)
		}

		// Consume data
		io.Copy(io.Discard, reader)
	}
}
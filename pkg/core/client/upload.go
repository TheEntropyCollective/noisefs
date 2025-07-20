// Package noisefs provides file upload functionality.
// This file handles file splitting, 3-tuple XOR anonymization with randomizer selection,
// and both streaming and non-streaming upload modes with progress reporting.
package noisefs

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
)

// ProgressCallback is called during operations to report progress
type ProgressCallback func(stage string, current, total int)

// Upload uploads a file to NoiseFS with full protocol implementation
func (c *Client) Upload(reader io.Reader, filename string) (string, error) {
	return c.UploadWithBlockSize(reader, filename, blocks.DefaultBlockSize)
}

// UploadWithProgress uploads a file with progress reporting
func (c *Client) UploadWithProgress(reader io.Reader, filename string, progress ProgressCallback) (string, error) {
	return c.UploadWithBlockSizeAndProgress(reader, filename, blocks.DefaultBlockSize, progress)
}

// UploadWithBlockSize uploads a file with a specific block size
func (c *Client) UploadWithBlockSize(reader io.Reader, filename string, blockSize int) (string, error) {
	return c.UploadWithBlockSizeAndProgress(reader, filename, blockSize, nil)
}

// UploadWithBlockSizeAndProgress uploads a file with a specific block size and progress reporting
func (c *Client) UploadWithBlockSizeAndProgress(reader io.Reader, filename string, blockSize int, progress ProgressCallback) (string, error) {
	// Read all data to get size
	if progress != nil {
		progress("Reading file", 0, 100)
	}

	if reader == nil {
		return "", fmt.Errorf("reader cannot be nil")
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("failed to read data: %w", err)
	}

	fileSize := int64(len(data))
	if progress != nil {
		progress("Reading file", 100, 100)
	}

	// Create splitter
	splitter, err := blocks.NewSplitter(blockSize)
	if err != nil {
		return "", fmt.Errorf("failed to create splitter: %w", err)
	}

	// Split file into blocks (always padded for cache efficiency)
	if progress != nil {
		progress("Splitting file into blocks", 0, 100)
	}
	fileBlocks, err := splitter.Split(strings.NewReader(string(data)))
	if err != nil {
		return "", fmt.Errorf("failed to split file: %w", err)
	}
	if progress != nil {
		progress("Splitting file into blocks", 100, 100)
	}

	// Calculate padded file size
	paddedFileSize := int64(len(fileBlocks) * blockSize)

	// Create descriptor with padding information
	descriptor := descriptors.NewDescriptor(filename, fileSize, paddedFileSize, blockSize)

	// Process each block with XOR and track actual storage
	totalBlocks := len(fileBlocks)
	var totalStorageUsed int64 = 0 // Track actual bytes stored

	for i, fileBlock := range fileBlocks {
		if progress != nil {
			progress("Anonymizing blocks", i, totalBlocks)
		}
		// Select two randomizer blocks (3-tuple XOR) and track NEW randomizer storage
		randBlock1, cid1, randBlock2, cid2, randomizerBytesStored, err := c.SelectRandomizers(fileBlock.Size())
		if err != nil {
			return "", fmt.Errorf("failed to select randomizers: %w", err)
		}

		// XOR the blocks (3-tuple: data XOR randomizer1 XOR randomizer2)
		xorBlock, err := fileBlock.XOR(randBlock1, randBlock2)
		if err != nil {
			return "", fmt.Errorf("failed to XOR blocks: %w", err)
		}

		// Store anonymized block with tracking
		dataCID, dataBytesStored, err := c.storeBlockWithTracking(context.Background(), xorBlock)
		if err != nil {
			return "", fmt.Errorf("failed to store data block: %w", err)
		}

		// Count both data and NEW randomizer storage
		totalStorageUsed += dataBytesStored + randomizerBytesStored

		// Cache the anonymized block
		c.cacheBlock(dataCID, xorBlock, map[string]interface{}{
			"block_type": "data",
			"strategy":   "performance",
		})

		// Add block triple to descriptor
		if err := descriptor.AddBlockTriple(dataCID, cid1, cid2); err != nil {
			return "", fmt.Errorf("failed to add block triple: %w", err)
		}
	}

	if progress != nil {
		progress("Anonymizing blocks", totalBlocks, totalBlocks)
	}

	// Store descriptor in IPFS
	if progress != nil {
		progress("Saving file descriptor", 0, 100)
	}

	// Create descriptor store with storage manager
	descriptorStore, err := descriptors.NewStoreWithManager(c.storageManager)
	if err != nil {
		return "", fmt.Errorf("failed to create descriptor store: %w", err)
	}

	descriptorCID, err := descriptorStore.Save(descriptor)
	if err != nil {
		return "", fmt.Errorf("failed to save descriptor: %w", err)
	}

	if progress != nil {
		progress("Saving file descriptor", 100, 100)
	}

	// Record metrics with actual storage used
	c.RecordUpload(fileSize, totalStorageUsed)

	return descriptorCID, nil
}

// RecordUpload records upload metrics
func (c *Client) RecordUpload(originalBytes, storedBytes int64) {
	c.metrics.RecordUpload(originalBytes, storedBytes)
}

// Package noisefs provides streaming upload and download functionality.
// This file implements memory-efficient streaming operations with constant memory usage
// regardless of file size, real-time XOR processing, and progress reporting.
package noisefs

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
)

// StreamingProgressCallback is called during streaming operations to report progress
type StreamingProgressCallback func(stage string, bytesProcessed int64, blocksProcessed int)

// StreamingUpload uploads a file using streaming with constant memory usage
func (c *Client) StreamingUpload(reader io.Reader, filename string) (string, error) {
	return c.StreamingUploadWithBlockSize(reader, filename, blocks.DefaultBlockSize)
}

// StreamingUploadWithProgress uploads a file using streaming with progress reporting
func (c *Client) StreamingUploadWithProgress(reader io.Reader, filename string, progress StreamingProgressCallback) (string, error) {
	return c.StreamingUploadWithBlockSizeAndProgress(reader, filename, blocks.DefaultBlockSize, progress)
}

// StreamingUploadWithBlockSize uploads a file using streaming with a specific block size
func (c *Client) StreamingUploadWithBlockSize(reader io.Reader, filename string, blockSize int) (string, error) {
	return c.StreamingUploadWithBlockSizeAndProgress(reader, filename, blockSize, nil)
}

// StreamingUploadWithBlockSizeAndProgress uploads a file using streaming with block size and progress
func (c *Client) StreamingUploadWithBlockSizeAndProgress(reader io.Reader, filename string, blockSize int, progress StreamingProgressCallback) (string, error) {
	if reader == nil {
		return "", errors.New("reader cannot be nil")
	}

	if progress != nil {
		progress("Initializing streaming upload", 0, 0)
	}

	// Create streaming splitter
	splitter, err := blocks.NewStreamingSplitter(blockSize)
	if err != nil {
		return "", fmt.Errorf("failed to create streaming splitter: %w", err)
	}

	// Create descriptor (file size will be updated as we process)
	descriptor := descriptors.NewDescriptor(filename, 0, 0, blockSize)

	// Create context for cancellation support (using background context for backward compatibility)
	ctx := context.Background()

	// Track progress
	var totalBytesProcessed int64
	var totalBlocksProcessed int

	// Create a client block processor that handles XOR anonymization and storage
	clientProcessor := &clientBlockProcessor{
		client:      c,
		descriptor:  descriptor,
		blockSize:   blockSize,
		progress:    progress,
		totalBytes:  &totalBytesProcessed,
		totalBlocks: &totalBlocksProcessed,
		ctx:         ctx,
	}

	// Process file in streaming fashion with progress reporting
	progressCallback := func(bytesProcessed int64, blocksProcessed int) {
		totalBytesProcessed = bytesProcessed
		totalBlocksProcessed = blocksProcessed
		if progress != nil {
			progress("Processing blocks", bytesProcessed, blocksProcessed)
		}
	}

	if progress != nil {
		progress("Streaming file processing", 0, 0)
	}

	// Split and process blocks with progress
	err = splitter.SplitWithProgressAndContext(ctx, reader, clientProcessor, progressCallback)
	if err != nil {
		return "", fmt.Errorf("failed to process file: %w", err)
	}

	// Update descriptor with final file size
	descriptor.FileSize = totalBytesProcessed

	if progress != nil {
		progress("Saving descriptor", totalBytesProcessed, totalBlocksProcessed)
	}

	// Store descriptor
	descriptorStore, err := descriptors.NewStoreWithManager(c.storageManager)
	if err != nil {
		return "", fmt.Errorf("failed to create descriptor store: %w", err)
	}

	descriptorCID, err := descriptorStore.Save(descriptor)
	if err != nil {
		return "", fmt.Errorf("failed to save descriptor: %w", err)
	}

	// Record metrics
	c.RecordUpload(totalBytesProcessed, totalBytesProcessed*3) // *3 for data + 2 randomizer blocks

	if progress != nil {
		progress("Upload complete", totalBytesProcessed, totalBlocksProcessed)
	}

	return descriptorCID, nil
}

// clientBlockProcessor implements blocks.BlockProcessor for streaming upload
type clientBlockProcessor struct {
	client      *Client
	descriptor  *descriptors.Descriptor
	blockSize   int
	progress    StreamingProgressCallback
	totalBytes  *int64
	totalBlocks *int
	ctx         context.Context
}

// ProcessBlock implements the BlockProcessor interface
func (p *clientBlockProcessor) ProcessBlock(blockIndex int, block *blocks.Block) error {
	// Check for context cancellation
	select {
	case <-p.ctx.Done():
		return p.ctx.Err()
	default:
	}

	// Select two randomizer blocks for 3-tuple XOR
	randomizer1, randomizer1CID, randomizer2, randomizer2CID, _, err := p.client.SelectRandomizers(p.blockSize)
	if err != nil {
		return fmt.Errorf("failed to select randomizers for block %d: %w", blockIndex, err)
	}

	// XOR the block with both randomizers (3-tuple anonymization)
	anonymizedBlock, err := block.XOR(randomizer1, randomizer2)
	if err != nil {
		return fmt.Errorf("failed to anonymize block %d: %w", blockIndex, err)
	}

	// Store the anonymized block with context
	address, err := p.client.storageManager.Put(p.ctx, anonymizedBlock)
	if err != nil {
		return fmt.Errorf("failed to store anonymized block %d: %w", blockIndex, err)
	}

	// Add block triple to descriptor
	if err := p.descriptor.AddBlockTriple(address.ID, randomizer1CID, randomizer2CID); err != nil {
		return fmt.Errorf("failed to add block triple for block %d: %w", blockIndex, err)
	}

	return nil
}

// StreamingUploadWithContext uploads a file using streaming with context for cancellation
func (c *Client) StreamingUploadWithContext(ctx context.Context, reader io.Reader, filename string) (string, error) {
	return c.StreamingUploadWithContextAndProgress(ctx, reader, filename, blocks.DefaultBlockSize, nil)
}

// StreamingUploadWithContextAndProgress uploads a file using streaming with context and progress reporting
func (c *Client) StreamingUploadWithContextAndProgress(ctx context.Context, reader io.Reader, filename string, blockSize int, progress StreamingProgressCallback) (string, error) {
	if reader == nil {
		return "", errors.New("reader cannot be nil")
	}

	if progress != nil {
		progress("Initializing streaming upload", 0, 0)
	}

	// Create streaming splitter
	splitter, err := blocks.NewStreamingSplitter(blockSize)
	if err != nil {
		return "", fmt.Errorf("failed to create streaming splitter: %w", err)
	}

	// Create descriptor (file size will be updated as we process)
	descriptor := descriptors.NewDescriptor(filename, 0, 0, blockSize)

	// Track progress
	var totalBytesProcessed int64
	var totalBlocksProcessed int

	// Create a client block processor that handles XOR anonymization and storage
	clientProcessor := &clientBlockProcessor{
		client:      c,
		descriptor:  descriptor,
		blockSize:   blockSize,
		progress:    progress,
		totalBytes:  &totalBytesProcessed,
		totalBlocks: &totalBlocksProcessed,
		ctx:         ctx,
	}

	// Process file in streaming fashion with progress reporting
	progressCallback := func(bytesProcessed int64, blocksProcessed int) {
		totalBytesProcessed = bytesProcessed
		totalBlocksProcessed = blocksProcessed
		if progress != nil {
			progress("Processing blocks", bytesProcessed, blocksProcessed)
		}
	}

	if progress != nil {
		progress("Streaming file processing", 0, 0)
	}

	// Split and process blocks with progress and context
	err = splitter.SplitWithProgressAndContext(ctx, reader, clientProcessor, progressCallback)
	if err != nil {
		return "", fmt.Errorf("failed to process file: %w", err)
	}

	// Update descriptor file size information
	descriptor.FileSize = totalBytesProcessed
	descriptor.PaddedFileSize = int64(totalBlocksProcessed * blockSize)

	if progress != nil {
		progress("Saving descriptor", totalBytesProcessed, totalBlocksProcessed)
	}

	// Save descriptor
	descriptorStore, err := descriptors.NewStoreWithManager(c.storageManager)
	if err != nil {
		return "", fmt.Errorf("failed to create descriptor store: %w", err)
	}

	descriptorCID, err := descriptorStore.Save(descriptor)
	if err != nil {
		return "", fmt.Errorf("failed to save descriptor: %w", err)
	}

	// Record metrics
	c.RecordUpload(totalBytesProcessed, totalBytesProcessed*3) // *3 for data + 2 randomizer blocks

	if progress != nil {
		progress("Upload complete", totalBytesProcessed, totalBlocksProcessed)
	}

	return descriptorCID, nil
}

package noisefs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

// limitingWriter wraps an io.Writer and limits the amount of data written
type limitingWriter struct {
	writer    io.Writer
	remaining int64
}

func (lw *limitingWriter) Write(p []byte) (n int, err error) {
	if lw.remaining <= 0 {
		return 0, nil // Don't write more data
	}
	
	if int64(len(p)) > lw.remaining {
		// Only write up to the remaining limit
		toWrite := p[:lw.remaining]
		n, err = lw.writer.Write(toWrite)
		lw.remaining -= int64(n)
		return n, err
	}
	
	// Write all data if within limit
	n, err = lw.writer.Write(p)
	lw.remaining -= int64(n)
	return n, err
}

// Download downloads a file by descriptor CID and returns data
func (c *Client) Download(descriptorCID string) ([]byte, error) {
	data, _, err := c.DownloadWithMetadata(descriptorCID)
	return data, err
}

// DownloadWithProgress downloads a file with progress reporting
func (c *Client) DownloadWithProgress(descriptorCID string, progress ProgressCallback) ([]byte, error) {
	data, _, err := c.DownloadWithMetadataAndProgress(descriptorCID, progress)
	return data, err
}

// DownloadWithMetadata downloads a file and returns both data and metadata
func (c *Client) DownloadWithMetadata(descriptorCID string) ([]byte, string, error) {
	return c.DownloadWithMetadataAndProgress(descriptorCID, nil)
}

// DownloadWithMetadataAndProgress downloads a file with progress reporting
func (c *Client) DownloadWithMetadataAndProgress(descriptorCID string, progress ProgressCallback) ([]byte, string, error) {
	if progress != nil {
		progress("Loading file descriptor", 0, 100)
	}

	// Create descriptor store with storage manager
	descriptorStore, err := descriptors.NewStoreWithManager(c.storageManager)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create descriptor store: %w", err)
	}

	// Load descriptor
	descriptor, err := descriptorStore.Load(descriptorCID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to load descriptor: %w", err)
	}

	if progress != nil {
		progress("Loading file descriptor", 100, 100)
	}

	// Retrieve and reconstruct blocks
	var originalBlocks []*blocks.Block
	totalBlocks := len(descriptor.Blocks)

	for i, blockInfo := range descriptor.Blocks {
		if progress != nil {
			progress("Downloading blocks", i, totalBlocks)
		}
		// Retrieve anonymized data block
		dataBlock, err := c.retrieveBlock(context.Background(), blockInfo.DataCID)
		if err != nil {
			return nil, "", fmt.Errorf("failed to retrieve data block: %w", err)
		}

		// Retrieve randomizer blocks
		randBlock1, err := c.retrieveBlock(context.Background(), blockInfo.RandomizerCID1)
		if err != nil {
			return nil, "", fmt.Errorf("failed to retrieve randomizer1 block: %w", err)
		}

		// Retrieve second randomizer block (3-tuple XOR)
		randBlock2, err := c.retrieveBlock(context.Background(), blockInfo.RandomizerCID2)
		if err != nil {
			return nil, "", fmt.Errorf("failed to retrieve randomizer2 block: %w", err)
		}

		origBlock, err := dataBlock.XOR(randBlock1, randBlock2)
		if err != nil {
			return nil, "", fmt.Errorf("failed to XOR blocks: %w", err)
		}

		originalBlocks = append(originalBlocks, origBlock)
	}

	if progress != nil {
		progress("Downloading blocks", totalBlocks, totalBlocks)
	}

	// Assemble file
	if progress != nil {
		progress("Assembling file", 0, 100)
	}

	assembler := blocks.NewAssembler()
	var buf strings.Builder
	if err := assembler.AssembleToWriter(originalBlocks, &buf); err != nil {
		return nil, "", fmt.Errorf("failed to assemble file: %w", err)
	}

	if progress != nil {
		progress("Assembling file", 100, 100)
	}

	// Handle padding removal (all files are padded)
	assembledData := []byte(buf.String())

	// Trim to original size (all files have padding)
	originalSize := descriptor.GetOriginalFileSize()
	if int64(len(assembledData)) > originalSize {
		assembledData = assembledData[:originalSize]
	}

	// Record download
	c.RecordDownload()

	return assembledData, descriptor.Filename, nil
}

// StreamingDownload downloads a file using streaming with constant memory usage
func (c *Client) StreamingDownload(descriptorCID string, writer io.Writer) error {
	return c.StreamingDownloadWithProgress(descriptorCID, writer, nil)
}

// StreamingDownloadWithProgress downloads a file using streaming with progress reporting
func (c *Client) StreamingDownloadWithProgress(descriptorCID string, writer io.Writer, progress StreamingProgressCallback) error {
	if writer == nil {
		return errors.New("writer cannot be nil")
	}

	if progress != nil {
		progress("Loading descriptor", 0, 0)
	}

	// Create descriptor store
	descriptorStore, err := descriptors.NewStoreWithManager(c.storageManager)
	if err != nil {
		return fmt.Errorf("failed to create descriptor store: %w", err)
	}

	// Load descriptor
	descriptor, err := descriptorStore.Load(descriptorCID)
	if err != nil {
		return fmt.Errorf("failed to load descriptor: %w", err)
	}

	if progress != nil {
		progress("Descriptor loaded", 0, len(descriptor.Blocks))
	}

	// Create a limiting writer that only writes up to the original file size
	originalSize := descriptor.GetOriginalFileSize()
	limitWriter := &limitingWriter{
		writer:    writer,
		remaining: originalSize,
	}

	// Create streaming assembler with the limiting writer
	assembler, err := blocks.NewStreamingAssembler(limitWriter)
	if err != nil {
		return fmt.Errorf("failed to create streaming assembler: %w", err)
	}

	// Set total blocks for the assembler
	assembler.SetTotalBlocks(len(descriptor.Blocks))

	// Process blocks in streaming fashion
	totalBlocks := len(descriptor.Blocks)
	var totalBytesWritten int64

	for i, blockPair := range descriptor.Blocks {
		if progress != nil {
			progress("Downloading blocks", totalBytesWritten, i)
		}

		// Retrieve anonymized data block
		dataAddress := &storage.BlockAddress{ID: blockPair.DataCID}
		dataBlock, err := c.storageManager.Get(context.Background(), dataAddress)
		if err != nil {
			return fmt.Errorf("failed to retrieve data block %d: %w", i, err)
		}

		// Retrieve randomizer blocks
		rand1Address := &storage.BlockAddress{ID: blockPair.RandomizerCID1}
		randomizer1, err := c.storageManager.Get(context.Background(), rand1Address)
		if err != nil {
			return fmt.Errorf("failed to retrieve randomizer1 block %d: %w", i, err)
		}

		rand2Address := &storage.BlockAddress{ID: blockPair.RandomizerCID2}
		randomizer2, err := c.storageManager.Get(context.Background(), rand2Address)
		if err != nil {
			return fmt.Errorf("failed to retrieve randomizer2 block %d: %w", i, err)
		}

		// De-anonymize: XOR the data block with both randomizers
		originalBlock, err := dataBlock.XOR(randomizer1, randomizer2)
		if err != nil {
			return fmt.Errorf("failed to de-anonymize block %d: %w", i, err)
		}

		// Add block to streaming assembler (using block index i)
		if err := assembler.AddBlock(i, originalBlock); err != nil {
			return fmt.Errorf("failed to add block %d to assembler: %w", i, err)
		}

		totalBytesWritten += int64(originalBlock.Size())
	}

	// Finalize assembly to write any remaining buffered blocks
	if err := assembler.Finalize(); err != nil {
		return fmt.Errorf("failed to finalize assembly: %w", err)
	}

	// Record download metrics
	c.RecordDownload()

	if progress != nil {
		progress("Download complete", totalBytesWritten, totalBlocks)
	}

	return nil
}

// StreamingDownloadWithContext downloads a file using streaming with context for cancellation
func (c *Client) StreamingDownloadWithContext(ctx context.Context, descriptorCID string, writer io.Writer) error {
	return c.StreamingDownloadWithContextAndProgress(ctx, descriptorCID, writer, nil)
}

// StreamingDownloadWithContextAndProgress downloads a file using streaming with context and progress reporting
func (c *Client) StreamingDownloadWithContextAndProgress(ctx context.Context, descriptorCID string, writer io.Writer, progress StreamingProgressCallback) error {
	if writer == nil {
		return errors.New("writer cannot be nil")
	}

	if progress != nil {
		progress("Loading descriptor", 0, 0)
	}

	// Create descriptor store
	descriptorStore, err := descriptors.NewStoreWithManager(c.storageManager)
	if err != nil {
		return fmt.Errorf("failed to create descriptor store: %w", err)
	}

	// Load descriptor
	descriptor, err := descriptorStore.Load(descriptorCID)
	if err != nil {
		return fmt.Errorf("failed to load descriptor: %w", err)
	}

	if progress != nil {
		progress("Creating streaming assembler", 0, 0)
	}

	// Create a limiting writer that only writes up to the original file size
	originalSize := descriptor.GetOriginalFileSize()
	limitWriter := &limitingWriter{
		writer:    writer,
		remaining: originalSize,
	}

	// Create streaming assembler for writing output
	assembler, err := blocks.NewStreamingAssembler(limitWriter)
	if err != nil {
		return fmt.Errorf("failed to create streaming assembler: %w", err)
	}

	totalBlocks := len(descriptor.Blocks)
	var totalBytesWritten int64

	if progress != nil {
		progress("Downloading blocks", 0, 0)
	}

	// Process each block
	for i, blockPair := range descriptor.Blocks {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Get the anonymized data block
		dataBlockAddress := &storage.BlockAddress{
			ID:          blockPair.DataCID,
			BackendType: "", // Let router determine
		}
		dataBlock, err := c.storageManager.Get(ctx, dataBlockAddress)
		if err != nil {
			return fmt.Errorf("failed to retrieve data block %d: %w", i, err)
		}

		// Get the first randomizer block
		randomizer1Address := &storage.BlockAddress{
			ID:          blockPair.RandomizerCID1,
			BackendType: "",
		}
		randomizer1, err := c.storageManager.Get(ctx, randomizer1Address)
		if err != nil {
			return fmt.Errorf("failed to retrieve randomizer1 for block %d: %w", i, err)
		}

		// Get the second randomizer block
		randomizer2Address := &storage.BlockAddress{
			ID:          blockPair.RandomizerCID2,
			BackendType: "",
		}
		randomizer2, err := c.storageManager.Get(ctx, randomizer2Address)
		if err != nil {
			return fmt.Errorf("failed to retrieve randomizer2 for block %d: %w", i, err)
		}

		// De-anonymize: XOR the data block with both randomizers
		originalBlock, err := dataBlock.XOR(randomizer1, randomizer2)
		if err != nil {
			return fmt.Errorf("failed to de-anonymize block %d: %w", i, err)
		}

		// Add block to streaming assembler (using block index i)
		if err := assembler.AddBlock(i, originalBlock); err != nil {
			return fmt.Errorf("failed to add block %d to assembler: %w", i, err)
		}

		totalBytesWritten += int64(originalBlock.Size())

		if progress != nil {
			progress("Processing blocks", totalBytesWritten, i+1)
		}
	}

	// Finalize assembly to write any remaining buffered blocks
	if err := assembler.Finalize(); err != nil {
		return fmt.Errorf("failed to finalize assembly: %w", err)
	}

	// Record download metrics
	c.RecordDownload()

	if progress != nil {
		progress("Download complete", totalBytesWritten, totalBlocks)
	}

	return nil
}

// RecordDownload records download metrics
func (c *Client) RecordDownload() {
	c.metrics.RecordDownload()
}

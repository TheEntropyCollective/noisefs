// Package noisefs provides comprehensive file download functionality for NoiseFS.
// This file handles file retrieval, de-anonymization through 3-tuple XOR operations,
// and both streaming and non-streaming download modes with proper file size trimming
// and padding removal for privacy-preserving distributed storage.
//
// The download system supports multiple operation modes:
//   - Simple downloads with automatic metadata handling
//   - Progress-enabled downloads for user interface integration
//   - Streaming downloads with constant memory usage
//   - Context-aware downloads with cancellation support
//   - Metadata extraction alongside file content
//
// All download operations perform 3-tuple XOR de-anonymization to recover original
// file content from anonymized blocks stored in the distributed network.
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

// limitingWriter wraps an io.Writer and limits the amount of data written to prevent overflow.
// This utility writer ensures that streaming downloads respect original file sizes by
// preventing writes beyond the specified limit, enabling proper padding removal without
// buffering entire files in memory.
//
// The limiting writer is essential for streaming operations where files have been padded
// to fixed block sizes for privacy protection, and the padding must be removed during
// reconstruction to recover the original file content.
//
// Key Features:
//   - Transparent write limiting without client awareness
//   - Automatic truncation of oversized writes
//   - Memory-efficient padding removal for streaming operations
//   - Integration with streaming assemblers for file reconstruction
//
// Use Cases:
//   - Streaming download operations with padding removal
//   - Memory-constrained file reconstruction
//   - Large file processing without memory buffering
//
// Thread Safety:
//   The writer is not inherently thread-safe and should be used from a single goroutine
//   or protected by external synchronization.
//
// Time Complexity: O(1) per write operation
// Space Complexity: O(1) - constant memory overhead
type limitingWriter struct {
	writer    io.Writer // Underlying writer receiving limited output
	remaining int64     // Number of bytes remaining before limit is reached
}

// Write implements io.Writer interface with size limiting for padding removal.
// This method ensures that writes do not exceed the remaining byte limit,
// effectively truncating padded data to recover original file sizes during
// streaming download operations.
//
// Write Behavior:
//   - Returns immediately if limit is exceeded (remaining <= 0)
//   - Truncates writes that would exceed the remaining limit
//   - Passes through writes that fit within the limit
//   - Updates remaining count after each successful write
//
// The method handles padding removal transparently by preventing writes beyond
// the original file size, enabling streaming reconstruction without requiring
// full file buffering in memory.
//
// Parameters:
//   - p: Data to write (may be truncated if exceeding limit)
//
// Returns:
//   - n: Number of bytes actually written (may be less than len(p))
//   - err: Error from underlying writer, or nil if successful/truncated
//
// Call Flow:
//   - Called by: Streaming assemblers during file reconstruction
//   - Calls: Underlying writer.Write for actual data output
//
// Time Complexity: O(n) where n is the number of bytes written
// Space Complexity: O(1) - no additional memory allocation
func (lw *limitingWriter) Write(p []byte) (n int, err error) {
	// Prevent writing beyond the limit (padding removal)
	if lw.remaining <= 0 {
		return 0, nil // Silently discard data beyond original file size
	}
	
	// Truncate write if it would exceed the remaining limit
	if int64(len(p)) > lw.remaining {
		// Only write up to the remaining limit for proper padding removal
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

// Download downloads a file by descriptor CID and returns the reconstructed data.
// This convenience method provides simple file download without progress reporting
// or metadata extraction, suitable for basic use cases where only file content is needed.
//
// The method performs complete file reconstruction including:
//   - Descriptor loading and validation
//   - Block retrieval from distributed storage
//   - 3-tuple XOR de-anonymization (data ⊕ randomizer1 ⊕ randomizer2)
//   - File assembly with proper padding removal
//   - Metrics recording for performance tracking
//
// Parameters:
//   - descriptorCID: Content identifier of the file descriptor containing block references
//
// Returns:
//   - []byte: Complete reconstructed file data with padding removed
//   - error: Non-nil if descriptor loading, block retrieval, or assembly fails
//
// Call Flow:
//   - Called by: Simple download operations, CLI commands, basic file retrieval
//   - Calls: DownloadWithMetadata for actual implementation
//
// Time Complexity: O(n) where n is the number of blocks in the file
// Space Complexity: O(f) where f is the complete file size in memory
func (c *Client) Download(descriptorCID string) ([]byte, error) {
	data, _, err := c.DownloadWithMetadata(descriptorCID)
	return data, err
}

// DownloadWithProgress downloads a file with progress reporting for user interface integration.
// This method provides the same functionality as Download while enabling real-time progress
// updates for long-running downloads, suitable for user interfaces requiring feedback.
//
// Progress callbacks are invoked at key stages:
//   - Descriptor loading completion
//   - Block download progress with current/total counts
//   - File assembly progress
//
// The progress information enables user interfaces to display completion percentages,
// estimated time remaining, and current operation status.
//
// Parameters:
//   - descriptorCID: Content identifier of the file descriptor containing block references
//   - progress: Callback function for progress updates (nil to disable progress reporting)
//
// Returns:
//   - []byte: Complete reconstructed file data with padding removed
//   - error: Non-nil if descriptor loading, block retrieval, or assembly fails
//
// Call Flow:
//   - Called by: User interface operations, progress-enabled downloads
//   - Calls: DownloadWithMetadataAndProgress for actual implementation
//
// Time Complexity: O(n) where n is the number of blocks in the file
// Space Complexity: O(f) where f is the complete file size in memory
func (c *Client) DownloadWithProgress(descriptorCID string, progress ProgressCallback) ([]byte, error) {
	data, _, err := c.DownloadWithMetadataAndProgress(descriptorCID, progress)
	return data, err
}

// DownloadWithMetadata downloads a file and returns both reconstructed data and filename metadata.
// This method provides access to both file content and associated metadata (filename)
// extracted from the descriptor, useful for applications requiring file identification.
//
// The method extracts the original filename from the descriptor while performing
// complete file reconstruction, enabling proper file handling and organization
// without requiring separate metadata queries.
//
// Parameters:
//   - descriptorCID: Content identifier of the file descriptor containing block references and metadata
//
// Returns:
//   - []byte: Complete reconstructed file data with padding removed
//   - string: Original filename extracted from descriptor metadata
//   - error: Non-nil if descriptor loading, block retrieval, or assembly fails
//
// Call Flow:
//   - Called by: Applications requiring both content and metadata, file organization systems
//   - Calls: DownloadWithMetadataAndProgress for actual implementation
//
// Time Complexity: O(n) where n is the number of blocks in the file
// Space Complexity: O(f) where f is the complete file size in memory
func (c *Client) DownloadWithMetadata(descriptorCID string) ([]byte, string, error) {
	return c.DownloadWithMetadataAndProgress(descriptorCID, nil)
}

// DownloadWithMetadataAndProgress downloads a file with comprehensive progress reporting and metadata extraction.
// This is the primary download implementation that provides complete functionality including
// real-time progress updates, metadata extraction, and full file reconstruction with
// 3-tuple XOR de-anonymization.
//
// Download Process:
//   1. Load and validate file descriptor from storage
//   2. Retrieve anonymized data blocks and randomizer pairs
//   3. Perform 3-tuple XOR de-anonymization for each block
//   4. Assemble blocks into complete file with progress tracking
//   5. Remove padding to recover original file size
//   6. Extract filename metadata and record metrics
//
// Progress Reporting:
//   - Descriptor loading phase (0-100%)
//   - Block download phase with current/total block counts
//   - File assembly phase (0-100%)
//
// The method handles all aspects of NoiseFS file reconstruction including privacy
// protection through XOR de-anonymization and proper padding removal.
//
// Parameters:
//   - descriptorCID: Content identifier of the file descriptor containing block references and metadata
//   - progress: Callback function for progress updates (nil to disable progress reporting)
//
// Returns:
//   - []byte: Complete reconstructed file data with padding removed to original size
//   - string: Original filename extracted from descriptor metadata
//   - error: Non-nil if any stage of download or reconstruction fails
//
// Call Flow:
//   - Called by: All other download methods, primary download implementation
//   - Calls: descriptor store operations, block retrieval, XOR operations, assembly
//
// Time Complexity: O(n) where n is the number of blocks in the file
// Space Complexity: O(f) where f is the complete file size for in-memory assembly
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
		// Retrieve anonymized data block from distributed storage
		dataBlock, err := c.retrieveBlock(context.Background(), blockInfo.DataCID)
		if err != nil {
			return nil, "", fmt.Errorf("failed to retrieve data block: %w", err)
		}

		// Retrieve first randomizer block for 3-tuple XOR de-anonymization
		randBlock1, err := c.retrieveBlock(context.Background(), blockInfo.RandomizerCID1)
		if err != nil {
			return nil, "", fmt.Errorf("failed to retrieve randomizer1 block: %w", err)
		}

		// Retrieve second randomizer block to complete 3-tuple XOR system
		randBlock2, err := c.retrieveBlock(context.Background(), blockInfo.RandomizerCID2)
		if err != nil {
			return nil, "", fmt.Errorf("failed to retrieve randomizer2 block: %w", err)
		}

		// Perform 3-tuple XOR de-anonymization: original = data ⊕ randomizer1 ⊕ randomizer2
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

	// Handle padding removal - all NoiseFS files are padded to fixed block sizes for privacy
	assembledData := []byte(buf.String())

	// Trim to original size to remove privacy-preserving padding
	originalSize := descriptor.GetOriginalFileSize()
	if int64(len(assembledData)) > originalSize {
		assembledData = assembledData[:originalSize] // Remove padding to recover original file
	}

	// Record download metrics for performance monitoring
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

// StreamingDownloadWithContext downloads a file using streaming with context cancellation support.
// This method provides memory-efficient streaming download with cancellation capabilities,
// enabling responsive shutdown of long-running downloads and proper resource cleanup.
//
// Context cancellation is checked before each block retrieval, allowing for prompt
// cancellation even during downloads of very large files with minimal latency.
//
// The method maintains the same memory efficiency as other streaming downloads while
// adding cancellation support for better user experience and resource management.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - descriptorCID: Content identifier of the file descriptor containing block references
//   - writer: Output destination for reconstructed file data (must be non-nil)
//
// Returns:
//   - error: Non-nil if context is cancelled, validation fails, descriptor loading fails, block retrieval fails, or assembly fails
//
// Call Flow:
//   - Called by: Cancellable streaming downloads, timeout-aware operations
//   - Calls: StreamingDownloadWithContextAndProgress for actual implementation
//
// Time Complexity: O(n) where n is the number of blocks in the file
// Space Complexity: O(1) - constant memory usage regardless of file size
func (c *Client) StreamingDownloadWithContext(ctx context.Context, descriptorCID string, writer io.Writer) error {
	return c.StreamingDownloadWithContextAndProgress(ctx, descriptorCID, writer, nil)
}

// StreamingDownloadWithContextAndProgress downloads a file using streaming with context cancellation and progress reporting.
// This is the most feature-complete streaming download method, providing memory efficiency,
// cancellation support, and real-time progress updates for comprehensive download management.
//
// Combined Features:
//   - Memory-efficient streaming with constant memory usage
//   - Context cancellation for responsive shutdown
//   - Progress reporting for user interface integration
//   - Proper padding removal through limiting writer
//   - 3-tuple XOR de-anonymization for privacy protection
//
// Cancellation Strategy:
//   - Context cancellation is checked before each block retrieval
//   - Enables prompt shutdown even for very large files
//   - Prevents resource waste on cancelled operations
//   - Maintains consistent error reporting
//
// Memory Management:
//   - Processes blocks individually without full file buffering
//   - Uses limiting writer for transparent padding removal
//   - Maintains constant memory footprint regardless of file size
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - descriptorCID: Content identifier of the file descriptor containing block references
//   - writer: Output destination for reconstructed file data (must be non-nil)
//   - progress: Callback for progress updates including bytes written and block counts (nil to disable)
//
// Returns:
//   - error: Non-nil if context is cancelled, validation fails, descriptor loading fails, block retrieval fails, or assembly fails
//
// Call Flow:
//   - Called by: Full-featured streaming downloads requiring cancellation and progress
//   - Calls: descriptor operations, storage manager with context, streaming assembler, progress callbacks
//
// Time Complexity: O(n) where n is the number of blocks in the file
// Space Complexity: O(1) - constant memory usage regardless of file size
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
		// Check for context cancellation before each block to enable responsive shutdown
		select {
		case <-ctx.Done():
			return ctx.Err() // Return cancellation or timeout error
		default:
			// Continue with block processing
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

// RecordDownload records download completion metrics for performance monitoring and analysis.
// This method updates the client's metrics tracking system to record successful download
// operations, enabling monitoring of download frequency, performance trends, and system usage.
//
// The metrics are used for:
//   - Performance analysis and optimization
//   - System health monitoring and alerting
//   - Usage pattern analysis and capacity planning
//   - Debugging and troubleshooting download issues
//
// Call Flow:
//   - Called by: All download methods upon successful completion
//   - Calls: metrics.RecordDownload for actual metrics recording
//
// Time Complexity: O(1) - simple counter increment with thread safety
// Space Complexity: O(1) - no additional memory allocation
func (c *Client) RecordDownload() {
	c.metrics.RecordDownload()
}

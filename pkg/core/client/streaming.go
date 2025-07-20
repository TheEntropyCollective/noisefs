// Package noisefs provides advanced streaming upload and download functionality for NoiseFS.
// This file implements memory-efficient streaming operations with constant memory usage
// regardless of file size, real-time 3-tuple XOR processing, context cancellation support,
// and comprehensive progress reporting for large file handling.
//
// The streaming system provides:
//   - Constant memory usage regardless of file size
//   - Real-time block processing with immediate storage
//   - Context cancellation for responsive operation control
//   - Progress reporting with bytes and block granularity
//   - 3-tuple XOR anonymization during streaming
//   - Integration with adaptive caching and storage management
//
// Streaming operations are ideal for:
//   - Large files that exceed available memory
//   - Real-time processing requirements
//   - Network streaming scenarios
//   - Resource-constrained environments
//   - Interactive applications requiring cancellation
package noisefs

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
)

// StreamingProgressCallback is called during streaming operations to report real-time progress.
// This callback function enables user interfaces and monitoring systems to provide feedback
// during long-running streaming operations, displaying current processing state, throughput,
// and completion estimates.
//
// The callback provides detailed progress information including:
//   - Current operation stage with human-readable descriptions
//   - Total bytes processed so far for throughput calculation
//   - Number of blocks processed for completion percentage
//
// Progress stages include:
//   - "Initializing streaming upload": Setup and validation phase
//   - "Streaming file processing": Main processing begins
//   - "Processing blocks": Active block processing with count updates
//   - "Saving descriptor": Final descriptor storage phase
//   - "Upload complete": Operation completion
//
// Parameters:
//   - stage: Human-readable description of current processing stage
//   - bytesProcessed: Total bytes processed so far (cumulative)
//   - blocksProcessed: Number of blocks completed (0-based count)
//
// Usage:
//   Progress callbacks should be lightweight and non-blocking to avoid impacting
//   streaming performance. They are called frequently during streaming operations.
//
// Time Complexity: O(1) - callback execution time depends on implementation
type StreamingProgressCallback func(stage string, bytesProcessed int64, blocksProcessed int)

// StreamingUpload uploads a file using memory-efficient streaming with constant memory usage.
// This convenience method provides simple streaming upload with recommended default configuration,
// suitable for large files where memory efficiency is more important than upload speed.
//
// The method uses constant memory regardless of file size by processing blocks individually
// as they are read from the input stream, making it ideal for:
//   - Large files that exceed available memory
//   - Network streaming scenarios
//   - Resource-constrained environments
//   - Continuous data processing pipelines
//
// Streaming Process:
//   1. Initialize streaming splitter with default block size
//   2. Process file blocks individually as they arrive
//   3. Perform 3-tuple XOR anonymization in real-time
//   4. Store blocks immediately without buffering
//   5. Build descriptor incrementally
//   6. Finalize with descriptor storage
//
// Parameters:
//   - reader: Data source for file content (processed as stream, not buffered)
//   - filename: Original filename for metadata (stored in descriptor)
//
// Returns:
//   - string: Content identifier of the stored file descriptor
//   - error: Non-nil if streaming fails, processing fails, or storage fails
//
// Call Flow:
//   - Called by: Memory-efficient upload operations, large file handling
//   - Calls: StreamingUploadWithBlockSize with default block size
//
// Time Complexity: O(n) where n is the number of blocks in the file
// Space Complexity: O(1) - constant memory usage regardless of file size
func (c *Client) StreamingUpload(reader io.Reader, filename string) (string, error) {
	return c.StreamingUploadWithBlockSize(reader, filename, blocks.DefaultBlockSize)
}

// StreamingUploadWithProgress uploads a file using memory-efficient streaming with real-time progress reporting.
// This method combines the memory efficiency of streaming uploads with comprehensive progress updates,
// enabling user interfaces to provide feedback during long-running operations without memory overhead.
//
// Progress Reporting Features:
//   - Real-time updates during block processing
//   - Bytes processed for throughput calculation
//   - Block count for completion percentage
//   - Stage descriptions for user feedback
//   - No impact on memory efficiency
//
// The progress information enables user interfaces to display:
//   - Processing throughput (bytes/second)
//   - Completion percentage (blocks processed)
//   - Current operation stage
//   - Estimated time remaining
//
// Parameters:
//   - reader: Data source for file content (processed as stream, not buffered)
//   - filename: Original filename for metadata (stored in descriptor)
//   - progress: Callback function for progress updates (nil to disable progress reporting)
//
// Returns:
//   - string: Content identifier of the stored file descriptor
//   - error: Non-nil if streaming fails, processing fails, or storage fails
//
// Call Flow:
//   - Called by: User interface operations requiring progress feedback
//   - Calls: StreamingUploadWithBlockSizeAndProgress with default block size
//
// Time Complexity: O(n) where n is the number of blocks in the file
// Space Complexity: O(1) - constant memory usage regardless of file size
func (c *Client) StreamingUploadWithProgress(reader io.Reader, filename string, progress StreamingProgressCallback) (string, error) {
	return c.StreamingUploadWithBlockSizeAndProgress(reader, filename, blocks.DefaultBlockSize, progress)
}

// StreamingUploadWithBlockSize uploads a file using memory-efficient streaming with custom block size.
// This method enables custom block sizing for streaming operations, allowing optimization for
// specific use cases such as network constraints, storage characteristics, or performance requirements.
//
// Custom Block Size Considerations:
//   - Larger blocks: Reduced overhead but higher memory per block
//   - Smaller blocks: Lower memory per block but increased overhead
//   - Network efficiency: Block size affects transfer characteristics
//   - Cache efficiency: Consistent sizing improves randomizer reuse
//
// The streaming approach maintains constant memory usage regardless of total file size,
// with memory usage determined only by the individual block size.
//
// Parameters:
//   - reader: Data source for file content (processed as stream, not buffered)
//   - filename: Original filename for metadata (stored in descriptor)
//   - blockSize: Custom block size in bytes (must be positive, affects memory per block)
//
// Returns:
//   - string: Content identifier of the stored file descriptor
//   - error: Non-nil if validation fails, streaming fails, processing fails, or storage fails
//
// Call Flow:
//   - Called by: Performance-optimized streaming operations, custom configuration scenarios
//   - Calls: StreamingUploadWithBlockSizeAndProgress with specified block size
//
// Time Complexity: O(n) where n is the number of blocks (affected by block size)
// Space Complexity: O(b) where b is the individual block size (constant for file size)
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

// clientBlockProcessor implements blocks.BlockProcessor interface for streaming upload operations.
// This processor handles real-time 3-tuple XOR anonymization and immediate storage of blocks
// during streaming operations, enabling constant memory usage regardless of file size.
//
// The processor coordinates multiple NoiseFS subsystems:
//   - Randomizer selection through client's caching system
//   - 3-tuple XOR anonymization for privacy protection
//   - Immediate block storage without buffering
//   - Incremental descriptor building
//   - Progress tracking for user interface integration
//   - Context cancellation for responsive operation control
//
// Key Features:
//   - Real-time block processing as data arrives
//   - Integration with adaptive caching for randomizer selection
//   - Immediate storage to minimize memory footprint
//   - Thread-safe progress tracking with shared counters
//   - Context-aware cancellation support
//
// Thread Safety:
//   The processor is designed for single-threaded use within the streaming splitter
//   but coordinates with thread-safe client subsystems.
//
// Time Complexity: O(1) per block processed
// Space Complexity: O(1) - constant memory per block regardless of file size
type clientBlockProcessor struct {
	client      *Client                   // Client instance providing storage and randomizer operations
	descriptor  *descriptors.Descriptor   // Descriptor being built incrementally with block references
	blockSize   int                       // Block size for consistent processing and validation
	progress    StreamingProgressCallback // Progress callback for real-time user feedback
	totalBytes  *int64                    // Shared counter for total bytes processed (updated by reference)
	totalBlocks *int                      // Shared counter for total blocks processed (updated by reference)
	ctx         context.Context           // Context for cancellation support during processing
}

// ProcessBlock implements the blocks.BlockProcessor interface for real-time block processing.
// This method performs complete block processing including randomizer selection, 3-tuple XOR
// anonymization, immediate storage, and descriptor updates during streaming operations.
//
// Block Processing Steps:
//   1. Check for context cancellation to enable responsive shutdown
//   2. Select two randomizer blocks using intelligent caching
//   3. Perform 3-tuple XOR anonymization (data ⊕ randomizer1 ⊕ randomizer2)
//   4. Store anonymized block immediately in distributed storage
//   5. Add block triple to descriptor for future reconstruction
//
// Randomizer Selection:
//   The method uses the client's SelectRandomizers functionality which:
//   - Leverages adaptive caching for optimal randomizer reuse
//   - Implements diversity controls to prevent concentration attacks
//   - Ensures security properties while maximizing cache efficiency
//
// Storage Integration:
//   Blocks are stored immediately using the storage manager with context support,
//   enabling cancellation and ensuring consistent backend access patterns.
//
// Parameters:
//   - blockIndex: Sequential index of the block within the file (0-based)
//   - block: Block data to be processed and stored
//
// Returns:
//   - error: Non-nil if context is cancelled, randomizer selection fails, XOR fails, storage fails, or descriptor update fails
//
// Call Flow:
//   - Called by: Streaming splitter during file processing
//   - Calls: Client randomizer selection, XOR operations, storage manager, descriptor updates
//
// Time Complexity: O(1) per block (plus storage backend latency)
// Space Complexity: O(1) - no additional memory allocation beyond block size
func (p *clientBlockProcessor) ProcessBlock(blockIndex int, block *blocks.Block) error {
	// Check for context cancellation
	select {
	case <-p.ctx.Done():
		return p.ctx.Err()
	default:
	}

	// Select two randomizer blocks for 3-tuple XOR anonymization
	// Uses intelligent caching and diversity controls for optimal security and performance
	randomizer1, randomizer1CID, randomizer2, randomizer2CID, _, err := p.client.SelectRandomizers(p.blockSize)
	if err != nil {
		return fmt.Errorf("failed to select randomizers for block %d: %w", blockIndex, err)
	}

	// Perform 3-tuple XOR anonymization: anonymized = data ⊕ randomizer1 ⊕ randomizer2
	// This makes the stored block appear random, providing privacy protection
	anonymizedBlock, err := block.XOR(randomizer1, randomizer2)
	if err != nil {
		return fmt.Errorf("failed to anonymize block %d: %w", blockIndex, err)
	}

	// Store the anonymized block immediately in distributed storage with context support
	// Context enables cancellation and storage manager handles backend selection
	address, err := p.client.storageManager.Put(p.ctx, anonymizedBlock)
	if err != nil {
		return fmt.Errorf("failed to store anonymized block %d: %w", blockIndex, err)
	}

	// Add block triple (data, randomizer1, randomizer2) to descriptor for reconstruction
	// The descriptor incrementally builds the mapping needed to retrieve and de-anonymize blocks
	if err := p.descriptor.AddBlockTriple(address.ID, randomizer1CID, randomizer2CID); err != nil {
		return fmt.Errorf("failed to add block triple for block %d: %w", blockIndex, err)
	}

	return nil
}

// StreamingUploadWithContext uploads a file using memory-efficient streaming with context cancellation support.
// This method provides memory-efficient streaming upload with cancellation capabilities,
// enabling responsive shutdown of long-running uploads and proper resource cleanup.
//
// Context cancellation is checked before each block processing operation, allowing for prompt
// cancellation even during uploads of very large files with minimal latency.
//
// The method maintains the same memory efficiency as other streaming uploads while
// adding cancellation support for better user experience and resource management.
//
// Cancellation Features:
//   - Context cancellation checked before each block
//   - Prompt shutdown even for very large files
//   - Proper resource cleanup on cancellation
//   - Consistent error reporting for cancellation scenarios
//   - Integration with timeout and deadline contexts
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - reader: Data source for file content (processed as stream, not buffered)
//   - filename: Original filename for metadata (stored in descriptor)
//
// Returns:
//   - string: Content identifier of the stored file descriptor
//   - error: Non-nil if context is cancelled, streaming fails, processing fails, or storage fails
//
// Call Flow:
//   - Called by: Cancellable streaming uploads, timeout-aware operations
//   - Calls: StreamingUploadWithContextAndProgress with default block size
//
// Time Complexity: O(n) where n is the number of blocks in the file
// Space Complexity: O(b) where b is the individual block size (constant for file size)
func (c *Client) StreamingUploadWithContext(ctx context.Context, reader io.Reader, filename string) (string, error) {
	return c.StreamingUploadWithContextAndProgress(ctx, reader, filename, blocks.DefaultBlockSize, nil)
}

// StreamingUploadWithContextAndProgress uploads a file using memory-efficient streaming with context cancellation and comprehensive progress reporting.
// This is the most feature-complete streaming upload method, providing memory efficiency, cancellation support,
// and real-time progress updates for comprehensive upload management of large files.
//
// Combined Features:
//   - Memory-efficient streaming with constant memory usage
//   - Context cancellation for responsive shutdown
//   - Progress reporting for user interface integration
//   - Custom block sizing for performance optimization
//   - 3-tuple XOR anonymization for privacy protection
//   - Immediate block storage without buffering
//
// Cancellation Strategy:
//   - Context cancellation is checked during block processing
//   - Enables prompt shutdown even for very large files
//   - Prevents resource waste on cancelled operations
//   - Maintains consistent error reporting
//   - Integrates with timeout and deadline contexts
//
// Memory Management:
//   - Processes blocks individually without full file buffering
//   - Constant memory footprint regardless of file size
//   - Memory usage determined only by individual block size
//   - Immediate storage prevents memory accumulation
//
// Progress Reporting:
//   - Real-time updates during block processing
//   - Bytes and block count information
//   - Stage descriptions for user feedback
//   - No impact on memory efficiency or cancellation responsiveness
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - reader: Data source for file content (processed as stream, not buffered)
//   - filename: Original filename for metadata (stored in descriptor)
//   - blockSize: Custom block size in bytes (must be positive, determines memory per block)
//   - progress: Callback function for progress updates (nil to disable progress reporting)
//
// Returns:
//   - string: Content identifier of the stored file descriptor
//   - error: Non-nil if context is cancelled, validation fails, streaming fails, processing fails, or storage fails
//
// Call Flow:
//   - Called by: Full-featured streaming uploads requiring cancellation and progress
//   - Calls: Streaming splitter with context, block processor, storage manager, descriptor store
//
// Time Complexity: O(n) where n is the number of blocks in the file
// Space Complexity: O(b) where b is the individual block size (constant for file size)
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

	// Update descriptor with final file size information after processing
	// FileSize is the original unpadded size, PaddedFileSize includes block padding
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

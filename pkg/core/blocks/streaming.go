// Package blocks provides streaming file processing functionality for NoiseFS.
// This file implements streaming splitters and assemblers that enable memory-efficient
// processing of large files without buffering entire files in memory, supporting
// real-time XOR anonymization and out-of-order block assembly.
package blocks

import (
	"context"
	"errors"
	"io"
	"sort"
	"sync"
)

// BlockProcessor defines the callback interface for processing blocks during streaming operations.
// This interface enables real-time processing of blocks as they are created from streaming data,
// allowing for immediate XOR anonymization, storage operations, or other transformations
// without requiring the entire file to be loaded into memory.
//
// Implementations of this interface can perform various operations on blocks:
//   - XOR anonymization with randomizer blocks
//   - Direct storage to backend systems
//   - Validation and integrity checking
//   - Real-time compression or encryption
//
// The interface supports streaming workflows where blocks are processed as soon as they
// are created, enabling constant memory usage regardless of file size.
//
// Call Flow:
//   - Implemented by: XOR processors, storage handlers, file processors
//   - Called by: StreamingSplitter during block creation
type BlockProcessor interface {
	// ProcessBlock is called for each block as it's created during streaming operations.
	// This method receives blocks in sequential order as they are split from the source data.
	//
	// Parameters:
	//   - blockIndex: Sequential index of the block within the file (0-based)
	//   - block: The newly created block ready for processing
	//
	// Returns:
	//   - error: Non-nil if block processing fails (will stop streaming operation)
	ProcessBlock(blockIndex int, block *Block) error
}

// ProgressCallback provides real-time progress reporting during streaming operations.
// This callback function is invoked periodically to report processing progress,
// enabling user interfaces to display progress bars, status updates, or performance metrics.
//
// The callback provides both byte-level and block-level progress information,
// allowing implementations to calculate completion percentages, transfer rates,
// and estimated time remaining for large file operations.
//
// Parameters:
//   - bytesProcessed: Total number of bytes processed from the original file
//   - blocksProcessed: Total number of blocks created and processed
//
// Call Flow:
//   - Called by: StreamingSplitter progress-enabled methods
//   - Frequency: Once per block processed
//
// Time Complexity: O(1) - callback execution time depends on implementation
type ProgressCallback func(bytesProcessed int64, blocksProcessed int)

// StreamingSplitter handles memory-efficient file splitting without buffering entire files.
// This splitter processes data from io.Reader sources in a streaming fashion, creating
// fixed-size blocks with automatic padding while maintaining constant memory usage
// regardless of input file size.
//
// The streaming approach provides several key advantages:
//   - Constant memory usage: Only one block worth of data in memory at a time
//   - Real-time processing: Blocks can be processed immediately as they're created
//   - Large file support: No practical limit on input file size
//   - Context support: Cancellation and timeout support for long operations
//
// Key Features:
//   - Memory-efficient streaming processing with fixed memory footprint
//   - Callback-based block processing for real-time operations
//   - Context cancellation support for responsive shutdown
//   - Progress reporting for user interface integration
//   - Fixed-size block creation with automatic zero-padding
//
// Call Flow:
//   - Created by: NewStreamingSplitter factory function
//   - Used by: Client streaming operations, directory processors
//   - Calls: BlockProcessor implementations for each created block
//
// Time Complexity: O(n) where n is the total data size
// Space Complexity: O(1) - constant memory usage regardless of file size
type StreamingSplitter struct {
	blockSize int // Size in bytes for each block (typically DefaultBlockSize = 128 KiB)
}

// NewStreamingSplitter creates a new memory-efficient streaming file splitter.
// This factory function validates the block size parameter and initializes a splitter
// capable of processing files of any size with constant memory usage.
//
// The streaming splitter uses the specified block size to create fixed-size blocks
// with automatic zero-padding, ensuring consistent block sizes for privacy protection
// and optimal cache performance in the NoiseFS system.
//
// Parameters:
//   - blockSize: Size in bytes for each block (must be positive, typically DefaultBlockSize)
//
// Returns:
//   - *StreamingSplitter: New splitter ready for streaming file processing
//   - error: Non-nil if blockSize is invalid (zero or negative)
//
// Call Flow:
//   - Called by: Client initialization, DirectoryProcessor setup, testing code
//   - Calls: None (simple constructor with validation)
//
// Time Complexity: O(1) - constant time initialization
// Space Complexity: O(1) - minimal memory allocation
func NewStreamingSplitter(blockSize int) (*StreamingSplitter, error) {
	if blockSize <= 0 {
		return nil, errors.New("block size must be positive")
	}

	return &StreamingSplitter{
		blockSize: blockSize,
	}, nil
}

// Split splits data from a reader into blocks using callback-based processing.
// This convenience method provides streaming block creation without context cancellation
// support, suitable for simple operations where cancellation is not required.
//
// The method delegates to SplitWithContext using a background context, providing
// the same streaming functionality while simplifying the API for basic use cases.
//
// Parameters:
//   - reader: Data source to read from (must be non-nil)
//   - processor: Block processor to handle each created block (must be non-nil)
//
// Returns:
//   - error: Non-nil if reader/processor validation fails or streaming operation fails
//
// Call Flow:
//   - Called by: Simple streaming operations without cancellation requirements
//   - Calls: SplitWithContext with background context
//
// Time Complexity: O(n) where n is the total data size
// Space Complexity: O(1) - constant memory usage regardless of file size
func (s *StreamingSplitter) Split(reader io.Reader, processor BlockProcessor) error {
	return s.SplitWithContext(context.Background(), reader, processor)
}

// SplitWithContext splits data from a reader into blocks with context cancellation support.
// This method provides the core streaming functionality with cancellation support,
// enabling responsive shutdown of long-running operations and resource cleanup.
//
// The splitting process:
//   1. Read data in chunks up to block size from the reader
//   2. Create full-size block with zero-padding for consistent block sizes
//   3. Generate content-addressed identifier for each block
//   4. Call processor for immediate block processing
//   5. Check context cancellation between blocks for responsive shutdown
//   6. Continue until EOF or error occurs
//
// Context cancellation is checked before each read operation, allowing for
// prompt cancellation even during processing of very large files.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - reader: Data source to read from (must be non-nil)
//   - processor: Block processor to handle each created block (must be non-nil)
//
// Returns:
//   - error: Non-nil if validation fails, context is cancelled, reading fails, or block processing fails
//
// Call Flow:
//   - Called by: Split, SplitWithProgress, cancellation-aware streaming operations
//   - Calls: reader.Read, NewBlock, processor.ProcessBlock, ctx.Done for cancellation
//
// Time Complexity: O(n) where n is the total data size
// Space Complexity: O(1) - constant memory usage regardless of file size
func (s *StreamingSplitter) SplitWithContext(ctx context.Context, reader io.Reader, processor BlockProcessor) error {
	if reader == nil {
		return errors.New("reader cannot be nil")
	}

	if processor == nil {
		return errors.New("processor cannot be nil")
	}

	buffer := make([]byte, s.blockSize)
	blockIndex := 0

	for {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := reader.Read(buffer)
		if n > 0 {
			// Always create full-sized blocks with padding for optimal cache efficiency
			blockData := make([]byte, s.blockSize)
			copy(blockData, buffer[:n])
			// Remaining bytes are zero-padded automatically

			block, blockErr := NewBlock(blockData)
			if blockErr != nil {
				return blockErr
			}

			if procErr := processor.ProcessBlock(blockIndex, block); procErr != nil {
				return procErr
			}

			blockIndex++
		}

		if err == io.EOF {
			break
		}

		if err != nil {
			return err
		}
	}

	return nil
}

// SplitWithProgress splits data with progress reporting for user interface integration.
// This convenience method provides streaming block creation with progress callbacks
// but without context cancellation, suitable for operations where progress visibility
// is needed but cancellation support is not required.
//
// The progress callback receives both byte-level and block-level progress information,
// enabling user interfaces to display completion percentages, transfer rates, and
// estimated time remaining for large file operations.
//
// Parameters:
//   - reader: Data source to read from (must be non-nil)
//   - processor: Block processor to handle each created block (must be non-nil)
//   - progress: Callback for progress updates (nil to disable progress reporting)
//
// Returns:
//   - error: Non-nil if validation fails, reading fails, or block processing fails
//
// Call Flow:
//   - Called by: User interface operations requiring progress without cancellation
//   - Calls: SplitWithProgressAndContext with background context
//
// Time Complexity: O(n) where n is the total data size
// Space Complexity: O(1) - constant memory usage regardless of file size
func (s *StreamingSplitter) SplitWithProgress(reader io.Reader, processor BlockProcessor, progress ProgressCallback) error {
	return s.SplitWithProgressAndContext(context.Background(), reader, processor, progress)
}

// SplitWithProgressAndContext splits data with comprehensive progress reporting and context cancellation.
// This is the most feature-complete streaming method, providing both progress visibility
// for user interfaces and cancellation support for responsive resource management.
//
// The method combines all streaming features:
//   - Memory-efficient processing with constant memory usage
//   - Context cancellation for prompt shutdown of long operations
//   - Progress reporting for user interface integration
//   - Fixed-size block creation with automatic padding
//
// Progress callbacks are invoked after each block is successfully processed,
// providing real-time updates on bytes processed and blocks created. The callback
// receives cumulative totals enabling calculation of completion percentages.
//
// Context cancellation is checked before each read operation, ensuring responsive
// shutdown even during processing of very large files with minimal latency.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - reader: Data source to read from (must be non-nil)
//   - processor: Block processor to handle each created block (must be non-nil)
//   - progress: Callback for progress updates (nil to disable progress reporting)
//
// Returns:
//   - error: Non-nil if validation fails, context is cancelled, reading fails, or block processing fails
//
// Call Flow:
//   - Called by: Full-featured streaming operations requiring both progress and cancellation
//   - Calls: reader.Read, NewBlock, processor.ProcessBlock, progress callback, ctx.Done
//
// Time Complexity: O(n) where n is the total data size
// Space Complexity: O(1) - constant memory usage regardless of file size
func (s *StreamingSplitter) SplitWithProgressAndContext(ctx context.Context, reader io.Reader, processor BlockProcessor, progress ProgressCallback) error {
	if reader == nil {
		return errors.New("reader cannot be nil")
	}

	if processor == nil {
		return errors.New("processor cannot be nil")
	}

	buffer := make([]byte, s.blockSize)
	blockIndex := 0
	bytesProcessed := int64(0)

	for {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := reader.Read(buffer)
		if n > 0 {
			// Always create full-sized blocks with padding for optimal cache efficiency
			blockData := make([]byte, s.blockSize)
			copy(blockData, buffer[:n])
			// Remaining bytes are zero-padded automatically

			block, blockErr := NewBlock(blockData)
			if blockErr != nil {
				return blockErr
			}

			if procErr := processor.ProcessBlock(blockIndex, block); procErr != nil {
				return procErr
			}

			blockIndex++
			bytesProcessed += int64(n)

			if progress != nil {
				progress(bytesProcessed, blockIndex)
			}
		}

		if err == io.EOF {
			break
		}

		if err != nil {
			return err
		}
	}

	return nil
}

// StreamingAssembler handles reconstruction of files from blocks with out-of-order arrival support.
// This assembler enables efficient file reconstruction when blocks arrive asynchronously
// or out-of-order, buffering blocks until they can be written sequentially to maintain
// file integrity while supporting concurrent block processing.
//
// The assembler maintains an internal buffer for out-of-order blocks and writes them
// sequentially as soon as all preceding blocks are available, enabling streaming
// reconstruction with minimal memory overhead and immediate writing when possible.
//
// Key Features:
//   - Out-of-order block handling with sequential writing
//   - Thread-safe concurrent block addition and writing
//   - Streaming output to any io.Writer destination
//   - Automatic completion detection when total blocks known
//   - Memory-efficient buffering of only out-of-order blocks
//
// Use Cases:
//   - Concurrent block retrieval and reconstruction
//   - Network-based file reconstruction with variable latency
//   - Parallel processing with ordered output requirements
//   - Real-time file streaming from distributed storage
//
// Thread Safety:
//   All methods use RWMutex protection to ensure safe concurrent access from
//   multiple goroutines adding blocks while maintaining write ordering.
//
// Call Flow:
//   - Created by: NewStreamingAssembler factory function
//   - Used by: Concurrent download operations, parallel block processors
//   - Writes to: Any io.Writer for output (files, network, memory)
//
// Time Complexity: O(log b) per block where b is the number of buffered out-of-order blocks
// Space Complexity: O(b) where b is the maximum number of out-of-order blocks buffered
type StreamingAssembler struct {
	writer        io.Writer      // Destination for reconstructed file data
	blockBuffer   map[int]*Block // Buffer holding out-of-order blocks indexed by block position
	nextIndex     int            // Index of the next block expected for sequential writing
	totalBlocks   int            // Total number of blocks expected (-1 if unknown)
	writtenBlocks int            // Number of blocks successfully written so far
	mutex         sync.RWMutex   // RWMutex protecting concurrent access to assembler state
	complete      bool           // Flag indicating all blocks have been written and assembly is complete
}

// NewStreamingAssembler creates a new streaming file assembler for out-of-order block reconstruction.
// This factory function validates the output writer and initializes an assembler ready for
// concurrent block addition with sequential writing to maintain file integrity.
//
// The assembler starts with no knowledge of the total expected blocks (unknown total),
// allowing it to work with streams where the total size is not known in advance.
// The total can be set later using SetTotalBlocks() when the information becomes available.
//
// Parameters:
//   - writer: Destination for reconstructed file data (must be non-nil)
//
// Returns:
//   - *StreamingAssembler: New assembler ready for concurrent block processing
//   - error: Non-nil if writer is nil
//
// Call Flow:
//   - Called by: Download operations, parallel block processors, streaming reconstructors
//   - Calls: make for buffer initialization
//
// Time Complexity: O(1) - constant time initialization
// Space Complexity: O(1) - minimal memory allocation for empty assembler
func NewStreamingAssembler(writer io.Writer) (*StreamingAssembler, error) {
	if writer == nil {
		return nil, errors.New("writer cannot be nil")
	}

	return &StreamingAssembler{
		writer:        writer,
		blockBuffer:   make(map[int]*Block),
		nextIndex:     0,
		totalBlocks:   -1, // Unknown total, can be set later via SetTotalBlocks
		writtenBlocks: 0,
		complete:      false,
	}, nil
}

// SetTotalBlocks sets the expected total number of blocks for completion detection.
// This method enables the assembler to automatically detect completion when the
// specified number of blocks have been written, triggering the completion state
// and preventing further block additions.
//
// Setting the total blocks is optional but recommended when the total is known,
// as it enables automatic completion detection and resource cleanup. Without
// a total, completion must be triggered manually using Finalize().
//
// Thread Safety:
//   Uses mutex protection to ensure safe concurrent access when blocks are
//   being added simultaneously from multiple goroutines.
//
// Parameters:
//   - total: Expected total number of blocks in the complete file
//
// Call Flow:
//   - Called by: Download operations when file size/block count is known
//   - Used by: AddBlock method for automatic completion detection
//
// Time Complexity: O(1) - simple mutex-protected assignment
// Space Complexity: O(1) - no memory allocation
func (a *StreamingAssembler) SetTotalBlocks(total int) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.totalBlocks = total
}

// AddBlock adds a block to the assembler and writes any sequential blocks that are ready.
// This method handles both in-order and out-of-order block arrival, immediately writing
// blocks when they are sequential and buffering out-of-order blocks until their
// predecessors arrive. It triggers automatic writing of any sequential blocks.
//
// The method performs comprehensive validation to ensure data integrity:
//   - Validates block is non-nil to prevent invalid data
//   - Ensures assembly is not already complete to prevent corruption
//   - Validates block index is non-negative for proper ordering
//   - Prevents duplicate blocks to maintain data consistency
//
// After successful validation, the block is added to the buffer and the assembler
// attempts to write any sequential blocks starting from the next expected index.
// This enables immediate progress when blocks arrive in order while handling
// out-of-order scenarios gracefully.
//
// Thread Safety:
//   Uses full mutex locking to ensure safe concurrent access when multiple
//   goroutines are adding blocks simultaneously.
//
// Parameters:
//   - blockIndex: Zero-based index indicating the block's position in the file
//   - block: Block data to add (must be non-nil with valid content)
//
// Returns:
//   - error: Non-nil if validation fails, assembly is complete, or writing fails
//
// Call Flow:
//   - Called by: Concurrent download operations, parallel block processors
//   - Calls: writeSequentialBlocks to attempt immediate writing
//
// Time Complexity: O(k) where k is the number of sequential blocks written
// Space Complexity: O(1) for buffering, O(k) for sequential writing
func (a *StreamingAssembler) AddBlock(blockIndex int, block *Block) error {
	if block == nil {
		return errors.New("block cannot be nil")
	}

	a.mutex.Lock()
	defer a.mutex.Unlock()

	if a.complete {
		return errors.New("assembly already complete")
	}

	if blockIndex < 0 {
		return errors.New("block index cannot be negative")
	}

	if _, exists := a.blockBuffer[blockIndex]; exists {
		return errors.New("block already exists")
	}

	// Add block to buffer and attempt to write any sequential blocks
	a.blockBuffer[blockIndex] = block
	return a.writeSequentialBlocks()
}

// writeSequentialBlocks writes blocks starting from nextIndex while they're available.
// This internal method processes blocks in strict sequential order, writing each available
// block immediately and removing it from the buffer to free memory. It continues until
// a gap is encountered or all expected blocks have been written.
//
// The method implements the core streaming logic:
//   1. Check if the next expected block is available in the buffer
//   2. Write the block data to the output writer
//   3. Remove the block from buffer to free memory
//   4. Advance to the next expected block index
//   5. Check for completion based on total blocks if known
//   6. Repeat until gap encountered or completion reached
//
// Automatic completion detection triggers when all expected blocks have been
// written (based on totalBlocks if set), preventing further additions and
// indicating the file is fully reconstructed.
//
// Note: This method assumes the caller holds the mutex lock for thread safety.
//
// Returns:
//   - error: Non-nil if writing to the output writer fails
//
// Call Flow:
//   - Called by: AddBlock after adding a new block to the buffer
//   - Called by: Finalize during manual completion processing
//   - Calls: writer.Write for each sequential block found
//
// Time Complexity: O(k) where k is the number of sequential blocks available
// Space Complexity: O(1) per block processed (memory freed by buffer removal)
func (a *StreamingAssembler) writeSequentialBlocks() error {
	// Process all available sequential blocks starting from nextIndex
	for {
		block, exists := a.blockBuffer[a.nextIndex]
		if !exists {
			// Gap encountered - cannot write more blocks sequentially
			break
		}

		// Write block data to output destination
		if _, err := a.writer.Write(block.Data); err != nil {
			return err
		}

		// Remove block from buffer to free memory and advance counters
		delete(a.blockBuffer, a.nextIndex)
		a.nextIndex++
		a.writtenBlocks++

		// Check for automatic completion if total blocks is known
		if a.totalBlocks > 0 && a.writtenBlocks >= a.totalBlocks {
			a.complete = true
			break
		}
	}

	return nil
}

// IsComplete returns whether all blocks have been written and assembly is finished.
// This method provides thread-safe access to the completion status, indicating
// whether the file reconstruction is complete and no further blocks should be added.
//
// Completion can occur in two ways:
//   1. Automatic: When totalBlocks is set and that many blocks have been written
//   2. Manual: When Finalize() is called to process remaining buffered blocks
//
// Thread Safety:
//   Uses read lock to allow concurrent status checking while preventing
//   race conditions with methods that modify the completion state.
//
// Returns:
//   - bool: True if assembly is complete and no further blocks should be added
//
// Call Flow:
//   - Called by: Client code checking assembly progress, completion detection logic
//   - Used by: Control flow logic to determine when reconstruction is finished
//
// Time Complexity: O(1) - simple mutex-protected field access
// Space Complexity: O(1) - no memory allocation
func (a *StreamingAssembler) IsComplete() bool {
	a.mutex.RLock()
	defer a.mutex.RUnlock()
	return a.complete
}

// Finalize completes the assembly by writing any remaining buffered blocks in sequential order.
// This method forces completion of the file reconstruction by writing all buffered
// blocks in sorted order, regardless of gaps. It should be called when no more
// blocks are expected or when manual completion is desired.
//
// The finalization process:
//   1. Check if assembly is already complete (no-op if true)
//   2. Extract all buffered block indices and sort them numerically
//   3. Write blocks in sequential order to maintain file integrity
//   4. Remove blocks from buffer as they are written to free memory
//   5. Mark assembly as complete to prevent further additions
//
// This method is useful when:
//   - Total block count is unknown but all available blocks should be written
//   - Some blocks may be missing but partial reconstruction is acceptable
//   - Manual completion is preferred over automatic detection
//
// Warning: This method writes blocks in order of their indices regardless of
// gaps, which may result in corrupted output if blocks are missing.
//
// Thread Safety:
//   Uses full mutex locking to ensure exclusive access during the finalization
//   process, preventing concurrent modifications.
//
// Returns:
//   - error: Non-nil if writing to the output writer fails during finalization
//
// Call Flow:
//   - Called by: Client code when forcing completion, cleanup operations
//   - Calls: sort.Ints for ordering, writer.Write for each buffered block
//
// Time Complexity: O(b log b) where b is the number of buffered blocks (due to sorting)
// Space Complexity: O(b) for temporary indices slice
func (a *StreamingAssembler) Finalize() error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	// No-op if already complete
	if a.complete {
		return nil
	}

	// Extract and sort all buffered block indices for sequential writing
	indices := make([]int, 0, len(a.blockBuffer))
	for index := range a.blockBuffer {
		indices = append(indices, index)
	}
	sort.Ints(indices) // Ensure blocks are written in sequential order

	// Write all buffered blocks in order and clean up buffer
	for _, index := range indices {
		block := a.blockBuffer[index]
		if _, err := a.writer.Write(block.Data); err != nil {
			return err
		}
		delete(a.blockBuffer, index) // Free memory as blocks are written
		a.writtenBlocks++
	}

	// Mark assembly as complete to prevent further block additions
	a.complete = true
	return nil
}

// RandomizerProvider provides randomizer blocks for XOR operations during streaming operations.
// This interface abstracts the source of randomizer blocks, enabling different strategies
// for randomizer selection and management while maintaining compatibility with the
// 3-tuple XOR anonymization system used throughout NoiseFS.
//
// The provider supplies pairs of randomizer blocks for each data block, enabling
// real-time XOR operations during streaming without requiring pre-computation or
// large memory buffers. Different implementations can provide:
//   - Fixed randomizer pairs for simple scenarios
//   - Cache-based randomizers for optimal storage efficiency
//   - Dynamic randomizers for enhanced privacy
//   - Network-sourced randomizers for distributed systems
//
// Call Flow:
//   - Implemented by: SimpleRandomizerProvider, cache-based providers, network providers
//   - Used by: StreamingXORProcessor during block anonymization
//   - Integration: Coordinates with storage cache for randomizer selection
//
// Time Complexity: Depends on implementation (O(1) for simple, O(log n) for cache-based)
// Space Complexity: Depends on implementation and caching strategy
type RandomizerProvider interface {
	// GetRandomizers retrieves two randomizer blocks for XOR operations with the specified data block.
	// This method is called during streaming operations to obtain randomizer pairs for
	// 3-tuple XOR anonymization (data ⊕ randomizer1 ⊕ randomizer2).
	//
	// The block index can be used by implementations to:
	//   - Select different randomizers for different blocks
	//   - Implement deterministic randomizer patterns
	//   - Coordinate with caching strategies
	//   - Enable reproducible anonymization
	//
	// Parameters:
	//   - blockIndex: Zero-based index of the data block needing randomizers
	//
	// Returns:
	//   - *Block: First randomizer block for XOR operations
	//   - *Block: Second randomizer block for XOR operations
	//   - error: Non-nil if randomizer retrieval fails
	GetRandomizers(blockIndex int) (*Block, *Block, error)
}

// StreamingXORProcessor processes blocks with XOR operations during streaming upload operations.
// This processor implements the BlockProcessor interface while adding real-time XOR
// anonymization to the streaming pipeline, enabling privacy-preserving block processing
// without requiring pre-computation or large memory buffers.
//
// The processor acts as a middleware component in the streaming pipeline:
//   1. Receives data blocks from StreamingSplitter
//   2. Obtains randomizer pairs from RandomizerProvider
//   3. Performs 3-tuple XOR anonymization (data ⊕ rand1 ⊕ rand2)
//   4. Forwards anonymized blocks to downstream processor
//
// This design enables flexible composition of streaming operations while maintaining
// the privacy guarantees of the NoiseFS anonymization system. The processor can be
// chained with storage processors, validation processors, or other middleware.
//
// Key Features:
//   - Real-time XOR anonymization during streaming
//   - Composable middleware design for flexible pipelines
//   - Integration with RandomizerProvider for flexible randomizer sources
//   - Error propagation from both randomizer retrieval and downstream processing
//
// Call Flow:
//   - Created by: NewStreamingXORProcessor factory function
//   - Used by: StreamingSplitter through BlockProcessor interface
//   - Integrates with: RandomizerProvider for randomizers, downstream BlockProcessor for storage
//
// Time Complexity: O(1) per block plus randomizer provider and downstream processor costs
// Space Complexity: O(1) - minimal overhead beyond randomizer and result blocks
type StreamingXORProcessor struct {
	provider   RandomizerProvider // Source of randomizer block pairs for XOR operations
	downstream BlockProcessor     // Next processor in the pipeline for anonymized blocks
}

// NewStreamingXORProcessor creates a new streaming XOR processor for real-time anonymization.
// This factory function validates the required dependencies and initializes a processor
// ready for composable streaming operations with XOR anonymization middleware.
//
// The processor requires both a randomizer provider for obtaining anonymization keys
// and a downstream processor for handling the anonymized results, ensuring a complete
// processing pipeline from data input to final storage or transmission.
//
// Parameters:
//   - provider: Source of randomizer block pairs for XOR operations (must be non-nil)
//   - downstream: Next processor in pipeline for anonymized blocks (must be non-nil)
//
// Returns:
//   - *StreamingXORProcessor: New processor ready for streaming XOR operations
//   - error: Non-nil if either provider or downstream processor is nil
//
// Call Flow:
//   - Called by: Streaming upload operations, pipeline construction code
//   - Calls: None (simple constructor with validation)
//
// Time Complexity: O(1) - constant time initialization
// Space Complexity: O(1) - minimal memory allocation
func NewStreamingXORProcessor(provider RandomizerProvider, downstream BlockProcessor) (*StreamingXORProcessor, error) {
	if provider == nil {
		return nil, errors.New("randomizer provider cannot be nil")
	}

	if downstream == nil {
		return nil, errors.New("downstream processor cannot be nil")
	}

	return &StreamingXORProcessor{
		provider:   provider,
		downstream: downstream,
	}, nil
}

// ProcessBlock implements BlockProcessor interface with XOR operations for real-time anonymization.
// This method performs the core streaming XOR processing by obtaining randomizer pairs
// and applying 3-tuple XOR anonymization before forwarding to the downstream processor.
//
// The processing pipeline:
//   1. Validate input block for data integrity
//   2. Retrieve randomizer pair from provider for the given block index
//   3. Perform XOR anonymization: result = data ⊕ randomizer1 ⊕ randomizer2
//   4. Forward anonymized block to downstream processor
//   5. Propagate any errors from randomizer retrieval, XOR operations, or downstream processing
//
// The method ensures that each data block is properly anonymized before storage
// or transmission, maintaining the privacy guarantees of the NoiseFS system while
// enabling real-time processing without memory buffering.
//
// Parameters:
//   - blockIndex: Zero-based index of the block for randomizer selection and ordering
//   - block: Data block to anonymize (must be non-nil with valid content)
//
// Returns:
//   - error: Non-nil if validation fails, randomizer retrieval fails, XOR fails, or downstream processing fails
//
// Call Flow:
//   - Called by: StreamingSplitter during streaming file processing
//   - Calls: provider.GetRandomizers, block.XOR, downstream.ProcessBlock
//
// Time Complexity: O(n) where n is block size for XOR operation, plus provider and downstream costs
// Space Complexity: O(1) - minimal overhead for XOR result block
func (p *StreamingXORProcessor) ProcessBlock(blockIndex int, block *Block) error {
	if block == nil {
		return errors.New("block cannot be nil")
	}

	// Obtain randomizer pair for 3-tuple XOR anonymization
	randomizer1, randomizer2, err := p.provider.GetRandomizers(blockIndex)
	if err != nil {
		return err
	}

	// Perform XOR anonymization: data ⊕ randomizer1 ⊕ randomizer2
	xorBlock, err := block.XOR(randomizer1, randomizer2)
	if err != nil {
		return err
	}

	// Forward anonymized block to downstream processor
	return p.downstream.ProcessBlock(blockIndex, xorBlock)
}

// SimpleRandomizerProvider provides the same randomizers for all blocks in a file.
// This provider implements the RandomizerProvider interface with a fixed pair of
// randomizer blocks, suitable for simple scenarios where consistent randomizers
// are acceptable and cache optimization is not required.
//
// The simple provider is useful for:
//   - Testing and development scenarios
//   - Small files where cache optimization is unnecessary
//   - Scenarios where deterministic anonymization is required
//   - Proof-of-concept implementations
//
// While this provider is easy to use and understand, it may not provide optimal
// storage efficiency compared to cache-based providers that reuse popular
// randomizers across multiple files for better space utilization.
//
// Security Considerations:
//   Using the same randomizers for all blocks in a file reduces the anonymity
//   set compared to varying randomizers, but still provides privacy protection
//   through the 3-tuple XOR system.
//
// Call Flow:
//   - Created by: NewSimpleRandomizerProvider factory function
//   - Used by: StreamingXORProcessor for simple randomizer scenarios
//   - Integration: Alternative to cache-based randomizer providers
//
// Time Complexity: O(1) for all operations
// Space Complexity: O(1) - fixed memory usage for two randomizer blocks
type SimpleRandomizerProvider struct {
	randomizer1 *Block // First randomizer block used for all XOR operations
	randomizer2 *Block // Second randomizer block used for all XOR operations
}

// NewSimpleRandomizerProvider creates a simple provider with fixed randomizers for all blocks.
// This factory function validates the randomizer blocks and initializes a provider that
// will return the same randomizer pair for every block index, providing consistent
// but simple anonymization suitable for basic use cases.
//
// The provider requires both randomizers to be valid blocks with proper content-addressed
// identifiers and data. These randomizers will be used for all XOR operations performed
// by the associated StreamingXORProcessor.
//
// Parameters:
//   - randomizer1: First randomizer block for XOR operations (must be non-nil)
//   - randomizer2: Second randomizer block for XOR operations (must be non-nil)
//
// Returns:
//   - *SimpleRandomizerProvider: New provider ready for consistent randomizer operations
//   - error: Non-nil if either randomizer is nil
//
// Call Flow:
//   - Called by: Simple streaming operations, testing code, proof-of-concept implementations
//   - Calls: None (simple constructor with validation)
//
// Time Complexity: O(1) - constant time initialization
// Space Complexity: O(1) - stores references to existing blocks
func NewSimpleRandomizerProvider(randomizer1, randomizer2 *Block) (*SimpleRandomizerProvider, error) {
	if randomizer1 == nil || randomizer2 == nil {
		return nil, errors.New("randomizers cannot be nil")
	}

	return &SimpleRandomizerProvider{
		randomizer1: randomizer1,
		randomizer2: randomizer2,
	}, nil
}

// GetRandomizers returns the fixed randomizers for any block index.
// This method implements the RandomizerProvider interface by returning the same
// randomizer pair regardless of the block index, providing consistent but simple
// anonymization for all blocks in a file.
//
// The method ignores the block index parameter since this provider uses fixed
// randomizers, making it suitable for scenarios where randomizer variation is
// not required or where deterministic anonymization is desired.
//
// Parameters:
//   - blockIndex: Block index (ignored by this simple implementation)
//
// Returns:
//   - *Block: First randomizer block (always the same instance)
//   - *Block: Second randomizer block (always the same instance)
//   - error: Always nil for this simple implementation
//
// Call Flow:
//   - Called by: StreamingXORProcessor.ProcessBlock during XOR operations
//   - Returns: Fixed randomizer blocks set during provider creation
//
// Time Complexity: O(1) - constant time return of stored references
// Space Complexity: O(1) - no memory allocation, returns existing references
func (p *SimpleRandomizerProvider) GetRandomizers(blockIndex int) (*Block, *Block, error) {
	return p.randomizer1, p.randomizer2, nil
}

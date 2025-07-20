// Package streaming provides comprehensive interface specifications for NoiseFS streaming operations.
// This package defines the core abstractions needed for memory-efficient, cancellable, and observable
// streaming upload and download operations in NoiseFS.
//
// The streaming interfaces provide:
//   - Unified streaming abstractions for consistent API surface
//   - Type-safe configuration with validation and defaults
//   - Standardized progress reporting and observability
//   - Composable block processing chains for extensibility
//   - Mockable interfaces for comprehensive testing
//   - Structured error handling with context
//
// Core Design Principles:
//   - Consistency: Standardized method signatures and naming conventions
//   - Testability: Mockable interfaces with dependency injection support
//   - Composability: Modular processor chains and configurable pipelines
//   - Observability: Unified progress reporting and metrics collection
//   - Safety: Context-aware cancellation and timeout support
//   - Performance: Memory-efficient streaming with configurable concurrency
package streaming

import (
	"context"
	"io"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
)

// Streamer provides unified streaming operations for NoiseFS file upload and download.
// This interface abstracts the core streaming functionality with consistent method signatures,
// comprehensive error handling, and configurable operation modes.
//
// The Streamer interface enables:
//   - Memory-efficient streaming regardless of file size
//   - Context-aware cancellation and timeout support
//   - Configurable progress reporting and monitoring
//   - Consistent error handling across all operations
//   - Support for both encrypted and unencrypted operations
//
// Implementation Requirements:
//   - Must respect context cancellation and timeouts
//   - Must provide progress updates when configured
//   - Must handle network failures and storage errors gracefully
//   - Must maintain constant memory usage regardless of file size
//   - Must support concurrent operations safely
//
// Thread Safety:
//   Implementations must be safe for concurrent use across multiple goroutines.
//
// Error Handling:
//   All methods return structured errors that can be unwrapped for specific error types.
//   Context cancellation and timeout errors are properly propagated.
type Streamer interface {
	// StreamUpload performs memory-efficient streaming upload of file data.
	// The operation processes data from the reader in chunks, applying 3-tuple XOR
	// anonymization and storing blocks incrementally without buffering the entire file.
	//
	// Process:
	//   1. Initialize streaming splitter with configured block size
	//   2. Process file blocks individually as they arrive from reader
	//   3. Apply 3-tuple XOR anonymization in real-time
	//   4. Store blocks immediately without buffering
	//   5. Build descriptor incrementally with block references
	//   6. Store final descriptor and return content identifier
	//
	// Parameters:
	//   - ctx: Context for cancellation, timeout, and tracing support
	//   - reader: Data source (processed as stream, not buffered)
	//   - opts: Configuration options for upload behavior
	//
	// Returns:
	//   - string: Content identifier of the stored file descriptor
	//   - error: Structured error with context, nil on success
	//
	// Errors:
	//   - ErrInvalidOptions: Configuration validation failed
	//   - ErrStreamingFailed: Streaming operation failed
	//   - context.Canceled: Operation was cancelled
	//   - context.DeadlineExceeded: Operation timed out
	//
	// Performance:
	//   - Time Complexity: O(n) where n is number of blocks
	//   - Space Complexity: O(1) - constant memory regardless of file size
	StreamUpload(ctx context.Context, reader io.Reader, opts UploadOptions) (string, error)

	// StreamDownload performs memory-efficient streaming download of file data.
	// The operation retrieves blocks using the descriptor, applies XOR de-anonymization,
	// and writes reconstructed data to the writer without buffering the entire file.
	//
	// Process:
	//   1. Retrieve and validate file descriptor
	//   2. Process blocks individually using descriptor metadata
	//   3. Retrieve data block and associated randomizer blocks
	//   4. Apply XOR de-anonymization: original = data ⊕ randomizer1 ⊕ randomizer2
	//   5. Write reconstructed data to writer incrementally
	//   6. Handle padding removal for original file size
	//
	// Parameters:
	//   - ctx: Context for cancellation, timeout, and tracing support
	//   - descriptorCID: Content identifier of the file descriptor
	//   - writer: Output destination for reconstructed file data
	//   - opts: Configuration options for download behavior
	//
	// Returns:
	//   - error: Structured error with context, nil on success
	//
	// Errors:
	//   - ErrDescriptorNotFound: Descriptor CID not found in storage
	//   - ErrBlockRetrievalFailed: Failed to retrieve required blocks
	//   - ErrDeAnonymizationFailed: XOR de-anonymization failed
	//   - context.Canceled: Operation was cancelled
	//   - context.DeadlineExceeded: Operation timed out
	//
	// Performance:
	//   - Time Complexity: O(n) where n is number of blocks
	//   - Space Complexity: O(1) - constant memory regardless of file size
	StreamDownload(ctx context.Context, descriptorCID string, writer io.Writer, opts DownloadOptions) error

	// GetMetrics returns comprehensive streaming operation metrics.
	// Provides insights into performance, throughput, and resource utilization
	// for monitoring and optimization purposes.
	//
	// Metrics include:
	//   - Total operations performed (upload/download)
	//   - Average throughput (bytes/second)
	//   - Memory usage patterns
	//   - Error rates and types
	//   - Cancellation statistics
	//
	// Returns:
	//   - StreamingMetrics: Current metrics snapshot
	//
	// Thread Safety:
	//   Safe to call concurrently from multiple goroutines.
	GetMetrics() StreamingMetrics

	// Close gracefully shuts down the streaming operations and releases resources.
	// This method should be called when the Streamer is no longer needed to ensure
	// proper cleanup of internal resources, connections, and background operations.
	//
	// Behavior:
	//   - Waits for ongoing operations to complete or timeout
	//   - Cancels all pending operations gracefully
	//   - Releases allocated resources and connections
	//   - Flushes any pending metrics or logging data
	//
	// Returns:
	//   - error: Error if cleanup failed, nil on successful shutdown
	//
	// Note:
	//   After Close() is called, further operations will return ErrStreamerClosed.
	Close() error
}

// StreamingConfig provides type-safe configuration for streaming operations.
// This interface enables flexible configuration with validation, defaults, and
// builder pattern support for creating streaming configurations.
//
// Configuration includes:
//   - Block size for file splitting and processing
//   - Concurrency settings for parallel operations
//   - Progress reporting configuration
//   - Timeout and cancellation settings
//   - Buffer sizes and memory constraints
//   - Encryption and security options
//
// The configuration is immutable after creation to ensure thread safety
// and prevent accidental modification during operations.
type StreamingConfig interface {
	// GetBlockSize returns the block size in bytes for file splitting.
	// Block size affects memory usage, network efficiency, and cache performance.
	//
	// Considerations:
	//   - Larger blocks: Reduced overhead, higher memory per block
	//   - Smaller blocks: Lower memory per block, increased overhead
	//   - Default: 128 KiB (optimal for most use cases)
	//
	// Returns:
	//   - int: Block size in bytes (must be positive)
	GetBlockSize() int

	// GetMaxConcurrency returns the maximum number of concurrent operations.
	// This limits parallel block processing to control resource usage and
	// prevent overwhelming storage backends or network connections.
	//
	// Returns:
	//   - int: Maximum concurrent operations (must be positive)
	GetMaxConcurrency() int

	// GetProgressReporter returns the configured progress reporter.
	// The reporter receives real-time updates during streaming operations
	// for user interface integration and monitoring.
	//
	// Returns:
	//   - ProgressReporter: Progress reporting interface (may be nil)
	GetProgressReporter() ProgressReporter

	// GetTimeout returns the operation timeout duration.
	// Operations exceeding this duration will be automatically cancelled
	// with context.DeadlineExceeded error.
	//
	// Returns:
	//   - time.Duration: Maximum operation duration (0 = no timeout)
	GetTimeout() time.Duration

	// GetBufferSize returns the internal buffer size for streaming operations.
	// This affects memory usage and I/O efficiency during data processing.
	//
	// Returns:
	//   - int: Buffer size in bytes
	GetBufferSize() int

	// IsEncryptionEnabled returns whether descriptor encryption is enabled.
	// When enabled, file descriptors are encrypted before storage for
	// enhanced privacy protection.
	//
	// Returns:
	//   - bool: True if encryption is enabled
	IsEncryptionEnabled() bool

	// Validate performs comprehensive configuration validation.
	// Checks all configuration parameters for validity and consistency,
	// returning detailed error information for invalid settings.
	//
	// Validation includes:
	//   - Block size within acceptable range
	//   - Concurrency limits are reasonable
	//   - Timeout values are positive
	//   - Buffer sizes are appropriate
	//
	// Returns:
	//   - error: Detailed validation error, nil if configuration is valid
	Validate() error

	// Clone creates a deep copy of the configuration.
	// This enables safe modification of configuration parameters without
	// affecting the original configuration instance.
	//
	// Returns:
	//   - StreamingConfig: Independent copy of the configuration
	Clone() StreamingConfig
}

// ProgressReporter provides standardized progress reporting for streaming operations.
// This interface enables real-time monitoring, user interface updates, and observability
// for long-running streaming operations with consistent reporting format.
//
// Progress reporting includes:
//   - Current operation stage and description
//   - Bytes and blocks processed with totals
//   - Throughput calculation and time estimates
//   - Error reporting and recovery status
//   - Completion and cancellation notifications
//
// The reporter must be thread-safe as it may receive updates from multiple
// concurrent operations and processing stages.
type ProgressReporter interface {
	// ReportProgress sends a progress update with current operation status.
	// This method is called frequently during streaming operations to provide
	// real-time feedback on processing state and completion estimates.
	//
	// Parameters:
	//   - info: Comprehensive progress information including stage, counts, and timing
	//
	// Implementation Notes:
	//   - Must be non-blocking to avoid impacting streaming performance
	//   - Should handle rapid successive calls efficiently
	//   - Must be thread-safe for concurrent operation reporting
	ReportProgress(info ProgressInfo)

	// ReportError sends error information during streaming operations.
	// Enables error tracking, logging, and user notification without
	// interrupting the progress reporting flow.
	//
	// Parameters:
	//   - err: Error that occurred during streaming operation
	//   - context: Additional context about when/where the error occurred
	//
	// Note:
	//   Errors reported here are typically non-fatal and may be recovered.
	//   Fatal errors are returned directly from streaming methods.
	ReportError(err error, context string)

	// SetTotal sets the total expected size for progress calculation.
	// Called at the beginning of operations when the total size is known,
	// enabling accurate percentage completion and time estimate calculations.
	//
	// Parameters:
	//   - totalBytes: Total expected bytes to process
	//   - totalBlocks: Total expected blocks to process
	SetTotal(totalBytes int64, totalBlocks int)

	// Complete notifies the reporter that the operation has finished successfully.
	// Provides final metrics and enables cleanup of progress tracking resources.
	//
	// Parameters:
	//   - finalInfo: Final progress information with completion metrics
	Complete(finalInfo ProgressInfo)

	// Cancel notifies the reporter that the operation was cancelled.
	// Enables proper cleanup and user notification of cancellation.
	//
	// Parameters:
	//   - reason: Reason for cancellation (timeout, user request, etc.)
	Cancel(reason string)
}

// BlockProcessor defines the interface for processing individual blocks during streaming.
// This interface enables composable processing chains where multiple processors can be
// chained together to perform different operations on blocks (XOR, compression, encryption, etc.).
//
// Processors are designed to be:
//   - Composable: Multiple processors can be chained together
//   - Stateless: Each block is processed independently
//   - Thread-safe: Safe for concurrent use across goroutines
//   - Context-aware: Respect cancellation and timeout requests
//   - Error-resilient: Handle failures gracefully with detailed error reporting
//
// Common use cases:
//   - XOR anonymization and de-anonymization
//   - Block validation and integrity checking
//   - Compression and decompression
//   - Encryption and decryption
//   - Caching and prefetching
//   - Metrics collection and logging
type BlockProcessor interface {
	// ProcessBlock performs processing on a single block during streaming operations.
	// The processor receives a block, applies its transformation or operation,
	// and returns the processed result or an error.
	//
	// Process Requirements:
	//   - Must respect context cancellation and timeouts
	//   - Must not modify the input block (create copies if needed)
	//   - Must handle errors gracefully with detailed context
	//   - Must be thread-safe for concurrent block processing
	//   - Must maintain consistent performance characteristics
	//
	// Parameters:
	//   - ctx: Context for cancellation, timeout, and tracing
	//   - blockIndex: Sequential index of the block within the file (0-based)
	//   - block: Block data to process (must not be modified)
	//
	// Returns:
	//   - *blocks.Block: Processed block result (may be same as input)
	//   - error: Processing error with context, nil on success
	//
	// Errors:
	//   - ErrBlockProcessingFailed: Processing operation failed
	//   - ErrInvalidBlock: Block data is invalid or corrupted
	//   - context.Canceled: Operation was cancelled
	//   - context.DeadlineExceeded: Operation timed out
	//
	// Performance:
	//   - Should maintain O(1) or O(block_size) time complexity
	//   - Should minimize memory allocations for efficiency
	ProcessBlock(ctx context.Context, blockIndex int, block *blocks.Block) (*blocks.Block, error)

	// GetName returns a human-readable name for the processor.
	// Used for logging, debugging, and progress reporting to identify
	// which processor is currently active in processing chains.
	//
	// Returns:
	//   - string: Descriptive name of the processor
	GetName() string

	// CanProcess determines if this processor can handle the given block.
	// Enables conditional processing and processor chain optimization
	// by skipping processors that don't apply to specific block types.
	//
	// Parameters:
	//   - block: Block to evaluate for processing capability
	//
	// Returns:
	//   - bool: True if processor can handle this block type
	CanProcess(block *blocks.Block) bool

	// GetMetrics returns processor-specific performance metrics.
	// Provides insights into processing performance, error rates,
	// and resource utilization for monitoring and optimization.
	//
	// Returns:
	//   - ProcessorMetrics: Current metrics for this processor
	GetMetrics() ProcessorMetrics
}
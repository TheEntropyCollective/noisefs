// Package streaming provides specialized interfaces for streaming operations.
// This file defines storage, randomizer, and assembler interfaces that support
// the core streaming functionality with testable and composable abstractions.
package streaming

import (
	"context"
	"io"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
)

// StreamingStorage provides storage operations optimized for streaming workflows.
// This interface abstracts storage backend interactions to enable testing,
// caching optimizations, and different storage strategies for streaming operations.
//
// The streaming storage interface differs from general storage by providing:
//   - Batch operations for improved performance
//   - Streaming-specific caching hints
//   - Progress callbacks for long-running operations
//   - Context-aware cancellation for all operations
//   - Specialized error handling for streaming scenarios
//
// Implementations must be thread-safe and support concurrent operations
// from multiple streaming operations simultaneously.
type StreamingStorage interface {
	// StoreBlock stores a single block and returns its content identifier.
	// Optimized for streaming operations with progress reporting and cancellation support.
	//
	// Parameters:
	//   - ctx: Context for cancellation, timeout, and tracing
	//   - block: Block data to store (must not be modified)
	//   - hint: Storage hint for optimization (e.g., "randomizer", "data", "descriptor")
	//
	// Returns:
	//   - string: Content identifier of the stored block
	//   - error: Storage error with context, nil on success
	//
	// Performance:
	//   - Should minimize latency for frequent small operations
	//   - May batch operations internally for efficiency
	//   - Should respect context cancellation promptly
	StoreBlock(ctx context.Context, block *blocks.Block, hint string) (string, error)

	// StoreBatch stores multiple blocks in a single operation for improved efficiency.
	// Enables batch optimization for storage backends that support bulk operations.
	//
	// Parameters:
	//   - ctx: Context for cancellation, timeout, and tracing
	//   - blocks: Map of blocks to store (hint -> block)
	//   - progress: Optional progress callback for batch operation updates
	//
	// Returns:
	//   - map[string]string: Map of hints to content identifiers
	//   - error: Storage error with context, nil on success
	//
	// Behavior:
	//   - All blocks are stored atomically or operation fails
	//   - Progress callback receives updates for large batches
	//   - Should be more efficient than individual StoreBlock calls
	StoreBatch(ctx context.Context, blocks map[string]*blocks.Block, progress ProgressReporter) (map[string]string, error)

	// RetrieveBlock retrieves a single block by its content identifier.
	// Optimized for streaming operations with caching hints and progress support.
	//
	// Parameters:
	//   - ctx: Context for cancellation, timeout, and tracing
	//   - cid: Content identifier of the block to retrieve
	//   - hint: Retrieval hint for cache optimization
	//
	// Returns:
	//   - *blocks.Block: Retrieved block data
	//   - error: Retrieval error with context, nil on success
	//
	// Caching:
	//   - Should utilize caching for frequently accessed blocks
	//   - Hint helps optimize cache strategies
	//   - May prefetch related blocks based on streaming patterns
	RetrieveBlock(ctx context.Context, cid string, hint string) (*blocks.Block, error)

	// RetrieveBatch retrieves multiple blocks in a single operation for improved efficiency.
	// Enables batch optimization and parallel retrieval for streaming downloads.
	//
	// Parameters:
	//   - ctx: Context for cancellation, timeout, and tracing
	//   - cids: Map of content identifiers to retrieve (hint -> cid)
	//   - progress: Optional progress callback for batch operation updates
	//
	// Returns:
	//   - map[string]*blocks.Block: Map of hints to retrieved blocks
	//   - error: Retrieval error with context, nil on success
	//
	// Behavior:
	//   - Retrieves blocks in parallel when possible
	//   - Progress callback receives updates for large batches
	//   - Should be more efficient than individual RetrieveBlock calls
	//   - Missing blocks result in an error rather than partial results
	RetrieveBatch(ctx context.Context, cids map[string]string, progress ProgressReporter) (map[string]*blocks.Block, error)

	// StoreDescriptor stores a file descriptor with streaming-specific optimizations.
	// Provides specialized handling for descriptor storage with encryption support.
	//
	// Parameters:
	//   - ctx: Context for cancellation, timeout, and tracing
	//   - descriptor: Descriptor to store
	//   - encrypted: Whether to store the descriptor encrypted
	//   - password: Encryption password (required if encrypted is true)
	//
	// Returns:
	//   - string: Content identifier of the stored descriptor
	//   - error: Storage error with context, nil on success
	StoreDescriptor(ctx context.Context, descriptor *descriptors.Descriptor, encrypted bool, password string) (string, error)

	// RetrieveDescriptor retrieves and validates a file descriptor.
	// Provides specialized handling for descriptor retrieval with decryption support.
	//
	// Parameters:
	//   - ctx: Context for cancellation, timeout, and tracing
	//   - cid: Content identifier of the descriptor
	//   - password: Decryption password (required for encrypted descriptors)
	//
	// Returns:
	//   - *descriptors.Descriptor: Retrieved and validated descriptor
	//   - error: Retrieval error with context, nil on success
	RetrieveDescriptor(ctx context.Context, cid string, password string) (*descriptors.Descriptor, error)

	// GetStorageMetrics returns storage-specific metrics for monitoring and optimization.
	// Provides insights into storage performance, cache hit rates, and error statistics.
	//
	// Returns:
	//   - StorageMetrics: Current storage metrics snapshot
	GetStorageMetrics() StorageMetrics

	// Close gracefully shuts down storage operations and releases resources.
	// Should be called when the storage is no longer needed.
	//
	// Returns:
	//   - error: Cleanup error, nil on successful shutdown
	Close() error
}

// RandomizerProvider supplies randomizer blocks for 3-tuple XOR anonymization.
// This interface abstracts randomizer selection and generation to enable testing,
// different selection strategies, and performance optimizations.
//
// The provider must ensure:
//   - Randomizer blocks are cryptographically random
//   - Same randomizers are not selected for the same data block
//   - Efficient selection with minimal storage overhead
//   - Thread-safe operation for concurrent streaming
//   - Proper caching and reuse strategies
type RandomizerProvider interface {
	// SelectRandomizers selects two randomizer blocks for 3-tuple XOR anonymization.
	// The selection process considers cache efficiency, randomizer diversity,
	// and storage overhead optimization.
	//
	// Selection criteria:
	//   - Randomizers must be different from each other and the data block
	//   - Should maximize cache reuse when possible
	//   - Should maintain statistical anonymity properties
	//   - Should minimize new randomizer generation
	//
	// Parameters:
	//   - ctx: Context for cancellation, timeout, and tracing
	//   - blockSize: Size of the data block being anonymized
	//   - hint: Selection hint for optimization (e.g., "sequential", "random")
	//
	// Returns:
	//   - *blocks.Block: First randomizer block
	//   - string: Content identifier of first randomizer
	//   - *blocks.Block: Second randomizer block
	//   - string: Content identifier of second randomizer
	//   - int64: Total bytes of new randomizer storage used
	//   - error: Selection error with context, nil on success
	SelectRandomizers(ctx context.Context, blockSize int, hint string) (*blocks.Block, string, *blocks.Block, string, int64, error)

	// GenerateRandomizer creates a new randomizer block of the specified size.
	// Used when existing randomizers are not suitable or available.
	//
	// Parameters:
	//   - ctx: Context for cancellation, timeout, and tracing
	//   - blockSize: Size of the randomizer block to generate
	//   - metadata: Optional metadata for randomizer tracking
	//
	// Returns:
	//   - *blocks.Block: Generated randomizer block
	//   - string: Content identifier of the generated randomizer
	//   - error: Generation error with context, nil on success
	GenerateRandomizer(ctx context.Context, blockSize int, metadata map[string]string) (*blocks.Block, string, error)

	// CacheRandomizer adds a randomizer to the cache for future reuse.
	// Enables manual cache management and optimization strategies.
	//
	// Parameters:
	//   - ctx: Context for cancellation, timeout, and tracing
	//   - cid: Content identifier of the randomizer
	//   - block: Randomizer block data
	//   - metadata: Optional metadata for cache optimization
	//
	// Returns:
	//   - error: Caching error with context, nil on success
	CacheRandomizer(ctx context.Context, cid string, block *blocks.Block, metadata map[string]string) error

	// GetRandomizerMetrics returns randomizer-specific metrics for monitoring.
	// Provides insights into cache performance, generation rates, and reuse statistics.
	//
	// Returns:
	//   - RandomizerMetrics: Current randomizer metrics snapshot
	GetRandomizerMetrics() RandomizerMetrics

	// SetStrategy configures the randomizer selection strategy.
	// Enables runtime optimization of randomizer selection behavior.
	//
	// Parameters:
	//   - strategy: Selection strategy ("performance", "privacy", "balanced")
	//
	// Returns:
	//   - error: Configuration error, nil on success
	SetStrategy(strategy string) error
}

// BlockAssembler reconstructs files from out-of-order blocks during streaming downloads.
// This interface enables efficient handling of parallel block retrieval and
// streaming reconstruction without requiring all blocks to be available simultaneously.
//
// The assembler maintains:
//   - Block ordering information for correct reconstruction
//   - Progress tracking for partially assembled files
//   - Memory-efficient storage of blocks until needed
//   - Context-aware cancellation for long-running assemblies
//   - Streaming output generation as blocks become available
type BlockAssembler interface {
	// Initialize prepares the assembler for a new file reconstruction.
	// Sets up the assembler state based on the file descriptor metadata.
	//
	// Parameters:
	//   - ctx: Context for cancellation, timeout, and tracing
	//   - descriptor: File descriptor containing reconstruction metadata
	//   - writer: Output destination for reconstructed file data
	//
	// Returns:
	//   - error: Initialization error with context, nil on success
	Initialize(ctx context.Context, descriptor *descriptors.Descriptor, writer io.Writer) error

	// AddBlock adds a retrieved block to the assembler for reconstruction.
	// Blocks may be added out of order and will be assembled in the correct sequence.
	//
	// Parameters:
	//   - ctx: Context for cancellation, timeout, and tracing
	//   - blockIndex: Index of the block within the file (0-based)
	//   - anonymizedBlock: Anonymized block data as retrieved from storage
	//   - randomizer1: First randomizer block for de-anonymization
	//   - randomizer2: Second randomizer block for de-anonymization
	//
	// Returns:
	//   - bool: True if this block completed the file reconstruction
	//   - error: Assembly error with context, nil on success
	//
	// Behavior:
	//   - Performs XOR de-anonymization: original = anonymized ⊕ randomizer1 ⊕ randomizer2
	//   - Writes sequential blocks to output immediately when available
	//   - Buffers out-of-order blocks until their position can be written
	//   - Returns true when the last block has been processed and written
	AddBlock(ctx context.Context, blockIndex int, anonymizedBlock *blocks.Block, randomizer1 *blocks.Block, randomizer2 *blocks.Block) (bool, error)

	// GetProgress returns the current assembly progress.
	// Provides detailed information about reconstruction status.
	//
	// Returns:
	//   - AssemblyProgress: Current progress information
	GetProgress() AssemblyProgress

	// IsComplete returns whether file reconstruction is complete.
	// True when all blocks have been processed and written to output.
	//
	// Returns:
	//   - bool: True if reconstruction is complete
	IsComplete() bool

	// GetMissingBlocks returns a list of block indices that are still needed.
	// Enables optimized retrieval strategies and progress reporting.
	//
	// Returns:
	//   - []int: List of missing block indices (0-based)
	GetMissingBlocks() []int

	// Cancel cancels the assembly operation and releases resources.
	// Safe to call multiple times and from different goroutines.
	//
	// Returns:
	//   - error: Cancellation error, nil on successful cancellation
	Cancel() error

	// Close finalizes the assembly and releases all resources.
	// Should be called when assembly is complete or cancelled.
	//
	// Returns:
	//   - error: Cleanup error, nil on successful cleanup
	Close() error
}

// ProcessorChain enables composable block processing with multiple processors.
// This interface provides a way to chain multiple block processors together
// for complex processing workflows while maintaining performance and error handling.
type ProcessorChain interface {
	// AddProcessor appends a processor to the end of the processing chain.
	// Processors are executed in the order they are added.
	//
	// Parameters:
	//   - processor: Processor to add to the chain
	//
	// Returns:
	//   - ProcessorChain: Chain instance for method chaining
	AddProcessor(processor BlockProcessor) ProcessorChain

	// InsertProcessor inserts a processor at the specified position in the chain.
	// Enables fine-grained control over processor execution order.
	//
	// Parameters:
	//   - index: Position to insert the processor (0-based)
	//   - processor: Processor to insert
	//
	// Returns:
	//   - ProcessorChain: Chain instance for method chaining
	//   - error: Error if index is invalid
	InsertProcessor(index int, processor BlockProcessor) (ProcessorChain, error)

	// RemoveProcessor removes a processor from the chain by name.
	// Enables dynamic reconfiguration of processing chains.
	//
	// Parameters:
	//   - name: Name of the processor to remove
	//
	// Returns:
	//   - bool: True if processor was found and removed
	RemoveProcessor(name string) bool

	// ProcessBlock executes all processors in the chain on the given block.
	// Each processor receives the output of the previous processor as input.
	//
	// Parameters:
	//   - ctx: Context for cancellation, timeout, and tracing
	//   - blockIndex: Sequential index of the block within the file
	//   - block: Initial block data to process
	//
	// Returns:
	//   - *blocks.Block: Final processed block after all processors
	//   - error: Processing error from any processor in the chain
	//
	// Error Handling:
	//   - Chain execution stops at the first processor error
	//   - Partial processing results are not returned on error
	//   - Context cancellation is checked between processors
	ProcessBlock(ctx context.Context, blockIndex int, block *blocks.Block) (*blocks.Block, error)

	// GetProcessors returns a list of processors in the chain.
	// Enables inspection and debugging of processor configurations.
	//
	// Returns:
	//   - []BlockProcessor: Copy of processors in execution order
	GetProcessors() []BlockProcessor

	// GetMetrics returns aggregated metrics for all processors in the chain.
	// Provides comprehensive performance monitoring for the entire chain.
	//
	// Returns:
	//   - ChainMetrics: Aggregated metrics for the processor chain
	GetMetrics() ChainMetrics

	// Clone creates a copy of the processor chain with the same configuration.
	// Enables reuse of processor chain configurations for multiple operations.
	//
	// Returns:
	//   - ProcessorChain: Independent copy of the processor chain
	Clone() ProcessorChain

	// Clear removes all processors from the chain.
	// Enables reuse of chain instances with different processor configurations.
	//
	// Returns:
	//   - ProcessorChain: Cleared chain instance for method chaining
	Clear() ProcessorChain
}

// Additional metrics types for specialized interfaces

// StorageMetrics provides comprehensive storage performance metrics.
type StorageMetrics struct {
	// Operations
	TotalStoreOperations     int64
	TotalRetrieveOperations  int64
	SuccessfulStoreOps       int64
	SuccessfulRetrieveOps    int64
	FailedStoreOps           int64
	FailedRetrieveOps        int64

	// Performance
	AverageStoreLatency      time.Duration
	AverageRetrieveLatency   time.Duration
	TotalBytesStored         int64
	TotalBytesRetrieved      int64

	// Caching
	CacheHits                int64
	CacheMisses              int64
	CacheHitRate             float64

	// Batch operations
	BatchStoreOperations     int64
	BatchRetrieveOperations  int64
	AverageBatchSize         float64

	// Error rates
	StoreErrorRate           float64
	RetrieveErrorRate        float64
}

// RandomizerMetrics provides randomizer-specific performance metrics.
type RandomizerMetrics struct {
	// Generation
	RandomizersGenerated     int64
	RandomizersReused        int64
	ReuseRate               float64

	// Selection
	SelectionOperations      int64
	AverageSelectionTime     time.Duration
	CacheHits               int64
	CacheMisses             int64

	// Storage overhead
	NewRandomizerBytes       int64
	ReusedRandomizerBytes    int64
	StorageOverheadRatio     float64

	// Strategy effectiveness
	CurrentStrategy          string
	StrategyEffectiveness    float64
}

// AssemblyProgress provides detailed assembly progress information.
type AssemblyProgress struct {
	// Block progress
	TotalBlocks             int
	ProcessedBlocks         int
	RemainingBlocks         int
	NextExpectedBlock       int

	// Data progress
	TotalBytes              int64
	ProcessedBytes          int64
	WrittenBytes            int64

	// Timing
	StartTime               time.Time
	LastBlockTime           time.Time
	EstimatedCompletion     time.Time

	// Performance
	AverageBlockTime        time.Duration
	Throughput              float64

	// Buffering
	BufferedBlocks          int
	MaxBufferedBlocks       int
	MemoryUsage             int64
}

// ChainMetrics provides aggregated metrics for processor chains.
type ChainMetrics struct {
	// Chain composition
	ProcessorCount          int
	ProcessorNames          []string

	// Performance
	TotalProcessingTime     time.Duration
	AverageChainTime        time.Duration
	BlocksProcessed         int64

	// Per-processor metrics
	ProcessorMetrics        map[string]ProcessorMetrics

	// Error statistics
	ChainErrors             int64
	ProcessorErrors         map[string]int64
	ErrorRate               float64
}
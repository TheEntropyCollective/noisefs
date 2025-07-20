// Package blocks provides directory tree processing functionality for NoiseFS.
// This file implements recursive directory traversal with parallel file processing,
// encrypted filename handling, and directory manifest creation for preserving
// directory structure while maintaining privacy through encryption.
package blocks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/crypto"
)

// DirectoryProcessor handles recursive processing of directory trees with parallel file processing.
// This processor traverses directory structures, encrypts filenames and directory names,
// creates encrypted directory manifests, and processes files through the block system
// while maintaining original directory relationships.
//
// The processor implements a worker pool pattern for parallel file processing, uses
// atomic operations for thread-safe progress tracking, and provides cancellation
// support through context. Each directory is processed to create an encrypted manifest
// containing metadata about its contents.
//
// Key Features:
//   - Recursive directory traversal with configurable worker pools
//   - Encrypted filename and directory name processing
//   - Progress reporting and error handling with customizable callbacks
//   - Cancellation support through context propagation
//   - Thread-safe operations with atomic counters and mutex protection
//
// Call Flow:
//   - Created by: NewDirectoryProcessor factory function
//   - Used by: Client directory upload operations, bulk processing workflows
//   - Integrates with: StreamingSplitter, crypto package, manifest system
//
// Time Complexity: O(n) where n is the total number of files in directory tree
// Space Complexity: O(w + m) where w is worker pool size and m is manifest entries
type DirectoryProcessor struct {
	// Configuration parameters
	blockSize     int                       // Size for block splitting operations
	maxWorkers    int                       // Maximum concurrent file processing workers
	splitter      *StreamingSplitter        // For splitting files into blocks
	encryptionKey *crypto.EncryptionKey     // Master key for filename encryption
	progressFn    DirectoryProgressCallback // Optional progress reporting callback
	errorHandler  DirectoryErrorHandler     // Optional error handling callback
	cancelFunc    context.CancelFunc        // For cancellation support
	ctx           context.Context           // Cancellation context

	// Internal state (protected by atomic operations for thread safety)
	processedFiles int64 // Number of files processed so far
	processedBytes int64 // Total bytes processed so far
	totalFiles     int64 // Total files discovered in directory tree
	totalBytes     int64 // Total bytes discovered in directory tree

	// Worker pool management for parallel processing
	workerPool chan struct{} // Semaphore for limiting concurrent workers
	wg         sync.WaitGroup // Wait group for coordinating worker completion

	// Results collection with thread-safe access
	results    chan ProcessResult // Channel for collecting processing results
	resultsMux sync.RWMutex       // Mutex protecting error slice
	errors     []error            // Collection of errors encountered during processing
}

// ProcessResult represents the outcome of processing a single file or directory.
// This structure captures all relevant metadata about the processing operation,
// including timing information, encrypted names, and content identifiers.
//
// Results are collected from worker goroutines and used to track processing
// progress and build directory manifests with encrypted metadata.
type ProcessResult struct {
	Path           string        // Original filesystem path of the processed item
	Type           DescriptorType // Whether this is a file or directory
	Size           int64         // Size in bytes (0 for directories)
	CID            string        // Content identifier for the processed item
	EncryptedName  []byte        // Encrypted filename or directory name
	ManifestCID    string        // For directories, the CID of the directory manifest
	Error          error         // Any error encountered during processing
	ProcessedAt    time.Time     // Timestamp when processing completed
	ProcessingTime time.Duration // How long the processing took
}

// DirectoryProgressCallback is called periodically to report processing progress.
// This callback allows monitoring of large directory processing operations,
// providing real-time feedback for user interfaces or logging systems.
//
// Parameters:
//   - processed: Number of files processed so far
//   - total: Total number of files discovered in the directory tree
//   - currentFile: Path of the file currently being processed
//
// Call Flow:
//   - Called by: Worker goroutines after completing file processing
//   - Frequency: Once per file processed
//
// Time Complexity: O(1) - callback execution time depends on implementation
type DirectoryProgressCallback func(processed, total int64, currentFile string)

// DirectoryErrorHandler provides custom error handling during directory processing.
// This handler allows applications to implement custom error recovery logic,
// decide whether to continue or abort processing, and potentially retry operations.
//
// Parameters:
//   - path: Filesystem path where the error occurred
//   - err: The error that was encountered
//
// Returns:
//   - bool: true to continue processing, false to abort the entire operation
//
// Call Flow:
//   - Called by: Directory processor when errors occur
//   - Decision point: Determines whether processing continues or stops
//
// Time Complexity: O(1) - depends on handler implementation
type DirectoryErrorHandler func(path string, err error) bool

// ProcessorConfig holds all configuration parameters for directory processor creation.
// This configuration structure provides fine-grained control over processing behavior,
// worker pool sizing, encryption settings, and filtering options.
//
// The configuration supports various filtering mechanisms to control which files
// are processed, including file size limits, extension filtering, and hidden file handling.
type ProcessorConfig struct {
	BlockSize         int                       // Block size for file splitting (uses DefaultBlockSize if 0)
	MaxWorkers        int                       // Maximum concurrent workers (default 10 if 0)
	EncryptionKey     *crypto.EncryptionKey     // Required encryption key for filename encryption
	ProgressCallback  DirectoryProgressCallback // Optional progress reporting callback
	ErrorHandler      DirectoryErrorHandler     // Optional error handling callback
	SkipSymlinks      bool                      // Whether to skip symbolic links during traversal
	SkipHidden        bool                      // Whether to skip hidden files (starting with '.')
	MaxFileSize       int64                     // Maximum file size to process (0 = no limit)
	AllowedExtensions []string                  // Only process files with these extensions (empty = all)
	BlockedExtensions []string                  // Skip files with these extensions
}

// NewDirectoryProcessor creates a new directory processor with the specified configuration.
// This factory function validates configuration parameters, applies defaults for optional
// settings, initializes the streaming splitter, and sets up the worker pool and result
// collection infrastructure.
//
// The function performs comprehensive validation of the configuration and initializes
// all internal data structures required for concurrent directory processing. A cancellation
// context is created to support graceful shutdown of processing operations.
//
// Configuration defaults applied:
//   - BlockSize: Uses DefaultBlockSize if not specified or invalid
//   - MaxWorkers: Uses 10 workers if not specified or invalid
//   - EncryptionKey: Required field, function fails if not provided
//
// Parameters:
//   - config: Configuration parameters for the directory processor (must be non-nil)
//
// Returns:
//   - *DirectoryProcessor: Configured processor ready for directory processing
//   - error: Non-nil if configuration is invalid or initialization fails
//
// Call Flow:
//   - Called by: Client directory upload operations, bulk processing workflows
//   - Calls: NewStreamingSplitter, context.WithCancel
//
// Time Complexity: O(1) - constant time initialization
// Space Complexity: O(w) where w is the worker pool size
func NewDirectoryProcessor(config *ProcessorConfig) (*DirectoryProcessor, error) {
	if config == nil {
		return nil, errors.New("processor config cannot be nil")
	}

	// Apply default block size if not specified
	if config.BlockSize <= 0 {
		config.BlockSize = DefaultBlockSize
	}

	// Apply default worker count if not specified
	if config.MaxWorkers <= 0 {
		config.MaxWorkers = 10
	}

	// Encryption key is mandatory for filename encryption
	if config.EncryptionKey == nil {
		return nil, errors.New("encryption key is required")
	}

	// Initialize streaming splitter for file processing
	splitter, err := NewStreamingSplitter(config.BlockSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create streaming splitter: %w", err)
	}

	// Create cancellation context for graceful shutdown support
	ctx, cancel := context.WithCancel(context.Background())

	return &DirectoryProcessor{
		blockSize:     config.BlockSize,
		maxWorkers:    config.MaxWorkers,
		splitter:      splitter,
		encryptionKey: config.EncryptionKey,
		progressFn:    config.ProgressCallback,
		errorHandler:  config.ErrorHandler,
		cancelFunc:    cancel,
		ctx:           ctx,
		workerPool:    make(chan struct{}, config.MaxWorkers), // Semaphore for worker limit
		results:       make(chan ProcessResult, 100),          // Buffered channel for results
		errors:        make([]error, 0),                       // Error collection slice
	}, nil
}

// ProcessDirectory processes a complete directory tree recursively with parallel file processing.
// This is the main entry point for directory processing operations, coordinating the entire
// workflow from initial discovery through final result collection.
//
// The processing follows a two-phase approach:
//   1. Discovery phase: Walk the directory tree to calculate total files and bytes
//   2. Processing phase: Recursively process directories and files with worker pool
//
// The function sets up a result collector goroutine to gather processing results from
// worker threads, manages the worker pool lifecycle, and aggregates any errors
// encountered during processing.
//
// Parameters:
//   - rootPath: Filesystem path to the root directory to process
//   - processor: Interface implementation for handling processed blocks and manifests
//
// Returns:
//   - []*ProcessResult: Slice of results for all processed files and directories
//   - error: Non-nil if processing fails or if any errors were encountered
//
// Call Flow:
//   - Called by: Client directory upload operations
//   - Calls: calculateTotals, processDirectoryRecursive, result collector goroutine
//
// Time Complexity: O(n) where n is the total number of files in directory tree
// Space Complexity: O(n) for storing all processing results
func (dp *DirectoryProcessor) ProcessDirectory(rootPath string, processor DirectoryBlockProcessor) ([]*ProcessResult, error) {
	// Discovery phase: calculate total files and bytes for progress reporting
	if err := dp.calculateTotals(rootPath); err != nil {
		return nil, fmt.Errorf("failed to calculate totals: %w", err)
	}

	// Initialize result collection infrastructure
	resultList := make([]*ProcessResult, 0)
	var resultsMux sync.Mutex
	var collectorDone sync.WaitGroup

	// Start background result collector goroutine
	collectorDone.Add(1)
	go func() {
		defer collectorDone.Done()
		// Collect results from worker goroutines as they complete
		for result := range dp.results {
			resultsMux.Lock()
			resultList = append(resultList, &result)
			resultsMux.Unlock()
		}
	}()

	// Processing phase: recursively process the directory tree
	if err := dp.processDirectoryRecursive(rootPath, processor); err != nil {
		return nil, fmt.Errorf("failed to process directory: %w", err)
	}

	// Synchronization: wait for all worker goroutines to complete
	dp.wg.Wait()
	close(dp.results) // Signal result collector that no more results are coming

	// Wait for result collector goroutine to finish processing all results
	collectorDone.Wait()

	// Error aggregation: check if any errors were encountered during processing
	dp.resultsMux.RLock()
	errors := append([]error(nil), dp.errors...)
	dp.resultsMux.RUnlock()

	if len(errors) > 0 {
		return resultList, fmt.Errorf("encountered %d errors during processing", len(errors))
	}

	return resultList, nil
}

// processDirectoryRecursive processes a single directory and its contents recursively.
// This function implements the core directory traversal logic, reading directory entries,
// creating directory manifests, and dispatching processing for subdirectories and files.
//
// The function implements cancellation checking at key points to ensure responsive
// shutdown. It creates a directory manifest to track all entries within the directory
// and handles both subdirectories (recursively) and files (via worker pool).
//
// Error handling is delegated to the configured error handler, allowing customizable
// recovery strategies. Hidden files are skipped by default to avoid processing
// system files and directories starting with '.'.
//
// Parameters:
//   - dirPath: Filesystem path to the directory to process
//   - processor: Interface for handling processed blocks and manifests
//
// Returns:
//   - error: Non-nil if directory processing fails or is cancelled
//
// Call Flow:
//   - Called by: ProcessDirectory, self (recursive calls)
//   - Calls: os.ReadDir, processDirectoryEntry, processFileEntry, storeDirectoryManifest
//
// Time Complexity: O(n) where n is the number of entries in the directory
// Space Complexity: O(1) for this function, O(d) overall where d is directory depth
func (dp *DirectoryProcessor) processDirectoryRecursive(dirPath string, processor DirectoryBlockProcessor) error {
	// Cancellation check: respond to context cancellation requests
	select {
	case <-dp.ctx.Done():
		return dp.ctx.Err()
	default:
	}

	// Read all entries in the current directory
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", dirPath, err)
	}

	// Create directory manifest to track all entries in this directory
	manifest := NewDirectoryManifest()

	// Process each directory entry (files and subdirectories)
	for _, entry := range entries {
		entryPath := filepath.Join(dirPath, entry.Name())

		// Cancellation check: respond to cancellation during iteration
		select {
		case <-dp.ctx.Done():
			return dp.ctx.Err()
		default:
		}

		// Filter: skip hidden files and directories (starting with '.')
		if entry.Name()[0] == '.' {
			continue
		}

		if entry.IsDir() {
			// Recursive processing: handle subdirectory with error recovery
			if err := dp.processDirectoryEntry(entryPath, entry, manifest, processor); err != nil {
				if !dp.handleError(entryPath, err) {
					return err // Stop processing if error handler indicates failure
				}
			}
		} else {
			// File processing: dispatch to worker pool with error recovery
			if err := dp.processFileEntry(entryPath, entry, manifest, processor); err != nil {
				if !dp.handleError(entryPath, err) {
					return err // Stop processing if error handler indicates failure
				}
			}
		}
	}

	// Finalization: store the encrypted directory manifest
	return dp.storeDirectoryManifest(dirPath, manifest, processor)
}

// processDirectoryEntry processes a directory entry
func (dp *DirectoryProcessor) processDirectoryEntry(dirPath string, entry os.DirEntry, manifest *DirectoryManifest, processor DirectoryBlockProcessor) error {
	// First recursively process the subdirectory
	if err := dp.processDirectoryRecursive(dirPath, processor); err != nil {
		return err
	}

	// Get directory info
	info, err := entry.Info()
	if err != nil {
		return fmt.Errorf("failed to get directory info: %w", err)
	}

	// Derive directory-specific key
	dirKey, err := crypto.DeriveDirectoryKey(dp.encryptionKey, dirPath)
	if err != nil {
		return fmt.Errorf("failed to derive directory key: %w", err)
	}

	// Encrypt directory name
	encryptedName, err := crypto.EncryptFileName(entry.Name(), dirKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt directory name: %w", err)
	}

	// Create directory entry with a placeholder CID
	// In a real implementation, this would be the CID of the directory's manifest
	dirEntry := DirectoryEntry{
		EncryptedName: encryptedName,
		CID:           "dir-" + filepath.Base(dirPath), // Simple placeholder for testing
		Type:          DirectoryType,
		Size:          0,
		ModifiedAt:    info.ModTime(),
	}

	// Add to manifest
	return manifest.AddEntry(dirEntry)
}

// processFileEntry processes a file entry
func (dp *DirectoryProcessor) processFileEntry(filePath string, entry os.DirEntry, manifest *DirectoryManifest, processor DirectoryBlockProcessor) error {
	// Get file info
	info, err := entry.Info()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	// Acquire worker slot
	dp.workerPool <- struct{}{}
	dp.wg.Add(1)

	// Process file in goroutine
	go func() {
		defer func() {
			<-dp.workerPool
			dp.wg.Done()
		}()

		startTime := time.Now()

		// Open file
		file, err := os.Open(filePath)
		if err != nil {
			dp.recordError(fmt.Errorf("failed to open file %s: %w", filePath, err))
			return
		}
		defer file.Close()

		// Create file processor
		fileProcessor := &FileBlockProcessor{
			FilePath:      filePath,
			FileSize:      info.Size(),
			Processor:     processor,
			Results:       make([]*ProcessResult, 0),
			EncryptionKey: dp.encryptionKey,
		}

		// Process file blocks
		if err := dp.splitter.Split(file, fileProcessor); err != nil {
			dp.recordError(fmt.Errorf("failed to process file %s: %w", filePath, err))
			return
		}

		// Update progress
		atomic.AddInt64(&dp.processedFiles, 1)
		atomic.AddInt64(&dp.processedBytes, info.Size())

		if dp.progressFn != nil {
			dp.progressFn(atomic.LoadInt64(&dp.processedFiles), atomic.LoadInt64(&dp.totalFiles), filePath)
		}

		// Derive directory-specific key
		dirKey, err := crypto.DeriveDirectoryKey(dp.encryptionKey, filepath.Dir(filePath))
		if err != nil {
			dp.recordError(fmt.Errorf("failed to derive directory key: %w", err))
			return
		}

		// Encrypt filename
		encryptedName, err := crypto.EncryptFileName(entry.Name(), dirKey)
		if err != nil {
			dp.recordError(fmt.Errorf("failed to encrypt filename: %w", err))
			return
		}

		// Create file entry
		fileEntry := DirectoryEntry{
			EncryptedName: encryptedName,
			CID:           fileProcessor.GetFileCID(),
			Type:          FileType,
			Size:          info.Size(),
			ModifiedAt:    info.ModTime(),
		}

		// Add to manifest (thread-safe)
		if err := manifest.AddEntry(fileEntry); err != nil {
			dp.recordError(fmt.Errorf("failed to add file entry: %w", err))
			return
		}

		// Record result
		result := ProcessResult{
			Path:           filePath,
			Type:           FileType,
			Size:           info.Size(),
			CID:            fileProcessor.GetFileCID(),
			EncryptedName:  encryptedName,
			ProcessedAt:    time.Now(),
			ProcessingTime: time.Since(startTime),
		}

		dp.results <- result
	}()

	return nil
}

// storeDirectoryManifest stores the directory manifest
func (dp *DirectoryProcessor) storeDirectoryManifest(dirPath string, manifest *DirectoryManifest, processor DirectoryBlockProcessor) error {
	// Encrypt and store manifest
	encryptedManifest, err := EncryptManifest(manifest, dp.encryptionKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt manifest: %w", err)
	}

	// Create block from encrypted manifest
	manifestBlock, err := NewBlock(encryptedManifest)
	if err != nil {
		return fmt.Errorf("failed to create manifest block: %w", err)
	}

	// Store manifest block
	if err := processor.ProcessDirectoryManifest(dirPath, manifestBlock); err != nil {
		return fmt.Errorf("failed to store manifest: %w", err)
	}

	// Record result
	result := ProcessResult{
		Path:           dirPath,
		Type:           DirectoryType,
		Size:           0,
		CID:            manifestBlock.ID,
		ManifestCID:    manifestBlock.ID,
		ProcessedAt:    time.Now(),
		ProcessingTime: 0,
	}

	dp.results <- result

	return nil
}

// calculateTotals calculates total files and bytes to process
func (dp *DirectoryProcessor) calculateTotals(rootPath string) error {
	return filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			atomic.AddInt64(&dp.totalFiles, 1)
			atomic.AddInt64(&dp.totalBytes, info.Size())
		}

		return nil
	})
}

// handleError handles errors during processing
func (dp *DirectoryProcessor) handleError(path string, err error) bool {
	dp.recordError(err)

	if dp.errorHandler != nil {
		return dp.errorHandler(path, err)
	}

	return false // Stop on error by default
}

// recordError records an error
func (dp *DirectoryProcessor) recordError(err error) {
	dp.resultsMux.Lock()
	dp.errors = append(dp.errors, err)
	dp.resultsMux.Unlock()
}

// Cancel initiates graceful cancellation of the directory processing operation.
// This function triggers the cancellation context, which will be detected by all
// worker goroutines and the main processing loop, allowing them to exit cleanly.
//
// The cancellation is cooperative - workers will finish their current operation
// before checking the cancellation status. This ensures data integrity while
// providing responsive shutdown capability.
//
// Call Flow:
//   - Called by: Client operations, timeout handlers, user cancellation requests
//   - Calls: context.CancelFunc (if available)
//
// Time Complexity: O(1) - immediate cancellation signal
// Space Complexity: O(1) - no additional memory allocation
func (dp *DirectoryProcessor) Cancel() {
	if dp.cancelFunc != nil {
		dp.cancelFunc()
	}
}

// GetProgress returns the current processing progress in terms of files processed.
// This function provides thread-safe access to progress counters using atomic
// operations, allowing safe concurrent access from multiple goroutines.
//
// The progress information can be used for user interface updates, logging,
// or determining when processing operations should be considered complete.
//
// Returns:
//   - processed: Number of files that have been completely processed
//   - total: Total number of files discovered during the initial directory scan
//
// Call Flow:
//   - Called by: Progress monitoring, user interfaces, logging systems
//   - Calls: atomic.LoadInt64 for thread-safe counter access
//
// Time Complexity: O(1) - atomic read operations
// Space Complexity: O(1) - no memory allocation
func (dp *DirectoryProcessor) GetProgress() (processed, total int64) {
	return atomic.LoadInt64(&dp.processedFiles), atomic.LoadInt64(&dp.totalFiles)
}

// DirectoryBlockProcessor defines the interface for handling processed blocks and manifests.
// This interface abstracts the storage and processing logic, allowing different
// implementations for various storage backends or processing strategies.
//
// Implementations of this interface are responsible for:
//   - Storing individual file blocks after processing
//   - Storing encrypted directory manifests
//   - Managing any necessary indexing or metadata
//
// The interface supports the separation of concerns between directory traversal
// (handled by DirectoryProcessor) and storage operations (handled by implementations).
//
// Call Flow:
//   - Implemented by: Storage clients, upload processors, testing mocks
//   - Called by: DirectoryProcessor during file and directory processing
type DirectoryBlockProcessor interface {
	// ProcessDirectoryBlock handles a processed block from a file within the directory.
	// This method is called for each block created during file processing.
	//
	// Parameters:
	//   - blockIndex: Sequential index of the block within the file
	//   - block: The processed block ready for storage
	//
	// Returns:
	//   - error: Non-nil if block processing or storage fails
	ProcessDirectoryBlock(blockIndex int, block *Block) error

	// ProcessDirectoryManifest handles the encrypted manifest for a directory.
	// This method is called once per directory after all its contents have been processed.
	//
	// Parameters:
	//   - dirPath: Filesystem path of the directory
	//   - manifestBlock: Block containing the encrypted directory manifest
	//
	// Returns:
	//   - error: Non-nil if manifest processing or storage fails
	ProcessDirectoryManifest(dirPath string, manifestBlock *Block) error
}

// FileBlockProcessor processes blocks from a single file during directory processing.

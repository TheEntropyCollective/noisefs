// Package noisefs provides comprehensive file upload functionality for NoiseFS.
// This file handles file splitting, 3-tuple XOR anonymization with intelligent randomizer selection,
// and both regular and encrypted upload modes with progress reporting and storage optimization.
//
// The upload system provides multiple operation modes:
//   - Simple uploads with default settings
//   - Progress-enabled uploads for user interface integration
//   - Custom block size uploads for specific requirements
//   - Encrypted uploads with password-protected descriptors
//   - Combined feature uploads with full customization
//
// All upload operations perform 3-tuple XOR anonymization to provide privacy protection
// while maintaining efficient storage through intelligent randomizer selection and caching.
package noisefs

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
)

// ProgressCallback is called during upload operations to report real-time progress.
// This callback function enables user interfaces to provide feedback during long-running
// upload operations, displaying current stage, completion percentage, and estimated time remaining.
//
// The callback provides detailed progress information including:
//   - Current operation stage (reading, splitting, anonymizing, storing)
//   - Current progress count within the stage
//   - Total count for the current stage
//
// Progress stages include:
//   - "Reading file": Initial file data reading and validation
//   - "Splitting file into blocks": Block creation with padding
//   - "Anonymizing blocks": XOR operations and randomizer selection
//   - "Saving file descriptor": Descriptor storage and finalization
//   - "Saving encrypted file descriptor": Encrypted descriptor storage
//
// Parameters:
//   - stage: Human-readable description of current operation stage
//   - current: Current progress count within the stage (0-based)
//   - total: Total count for completion of the current stage
//
// Usage:
//   Progress callbacks should be lightweight and non-blocking to avoid impacting
//   upload performance. They are called frequently during upload operations.
//
// Time Complexity: O(1) - callback execution time depends on implementation
type ProgressCallback func(stage string, current, total int)

// Upload uploads a file to NoiseFS with full protocol implementation using default settings.
// This convenience method provides simple file upload with recommended default configuration,
// suitable for most use cases where custom block sizing or progress reporting is not required.
//
// The method uses the standard NoiseFS block size (128 KiB) for optimal balance between
// privacy protection, network efficiency, and storage overhead. All files are automatically
// padded to fixed block sizes and anonymized using 3-tuple XOR operations.
//
// Upload Process:
//   1. Read and validate input data
//   2. Split file into fixed-size blocks with padding
//   3. Select randomizer pairs for each block
//   4. Perform 3-tuple XOR anonymization
//   5. Store anonymized blocks and randomizers
//   6. Create and store file descriptor
//   7. Record metrics and return descriptor CID
//
// Parameters:
//   - reader: Data source for file content (must be non-nil)
//   - filename: Original filename for metadata (used in descriptor)
//
// Returns:
//   - string: Content identifier of the stored file descriptor
//   - error: Non-nil if validation fails, reading fails, anonymization fails, or storage fails
//
// Call Flow:
//   - Called by: Simple upload operations, CLI commands, basic file storage
//   - Calls: UploadWithBlockSize with default block size
//
// Time Complexity: O(n) where n is the number of blocks in the file
// Space Complexity: O(f) where f is the complete file size for in-memory processing
func (c *Client) Upload(reader io.Reader, filename string) (string, error) {
	return c.UploadWithBlockSize(reader, filename, blocks.DefaultBlockSize)
}

// UploadWithProgress uploads a file with progress reporting for user interface integration.
// This method provides the same functionality as Upload while enabling real-time progress
// updates for long-running uploads, suitable for user interfaces requiring feedback.
//
// Progress callbacks are invoked at key stages:
//   - File reading and validation
//   - Block splitting and padding
//   - Block anonymization with randomizer selection
//   - Descriptor storage and finalization
//
// The progress information enables user interfaces to display completion percentages,
// current operation status, and estimated time remaining for large file uploads.
//
// Parameters:
//   - reader: Data source for file content (must be non-nil)
//   - filename: Original filename for metadata (used in descriptor)
//   - progress: Callback function for progress updates (nil to disable progress reporting)
//
// Returns:
//   - string: Content identifier of the stored file descriptor
//   - error: Non-nil if validation fails, reading fails, anonymization fails, or storage fails
//
// Call Flow:
//   - Called by: User interface operations, progress-enabled uploads
//   - Calls: UploadWithBlockSizeAndProgress with default block size
//
// Time Complexity: O(n) where n is the number of blocks in the file
// Space Complexity: O(f) where f is the complete file size for in-memory processing
func (c *Client) UploadWithProgress(reader io.Reader, filename string, progress ProgressCallback) (string, error) {
	return c.UploadWithBlockSizeAndProgress(reader, filename, blocks.DefaultBlockSize, progress)
}

// UploadWithBlockSize uploads a file with a custom block size for specialized requirements.
// This method enables custom block sizing for specific use cases such as performance
// optimization, memory constraints, or compatibility with different storage backends.
//
// Custom block sizes affect:
//   - Memory usage during processing
//   - Network transfer characteristics
//   - Storage overhead from padding
//   - Cache efficiency and randomizer reuse
//
// While the default 128 KiB block size is recommended for most use cases,
// custom sizes may be beneficial for specific deployment scenarios or
// performance requirements.
//
// Parameters:
//   - reader: Data source for file content (must be non-nil)
//   - filename: Original filename for metadata (used in descriptor)
//   - blockSize: Custom block size in bytes (must be positive)
//
// Returns:
//   - string: Content identifier of the stored file descriptor
//   - error: Non-nil if validation fails, reading fails, anonymization fails, or storage fails
//
// Call Flow:
//   - Called by: Performance-optimized uploads, custom configuration operations
//   - Calls: UploadWithBlockSizeAndProgress with specified block size
//
// Time Complexity: O(n) where n is the number of blocks (affected by block size)
// Space Complexity: O(f) where f is the complete file size for in-memory processing
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

// EncryptedUpload uploads a file with encrypted descriptor metadata for enhanced privacy.
// This method provides the same file anonymization as regular uploads while adding
// descriptor encryption to protect file metadata from unauthorized access.
//
// Encryption Features:
//   - Descriptor metadata encrypted with AES-256-GCM
//   - Password-based key derivation using Argon2id
//   - File content anonymized with 3-tuple XOR as normal
//   - Secure storage of encrypted descriptors
//
// The encrypted descriptor protects sensitive metadata including filename,
// file size, block structure, and randomizer references while maintaining
// the same storage efficiency and privacy protection of the block content.
//
// Parameters:
//   - reader: Data source for file content (must be non-nil)
//   - filename: Original filename for metadata (encrypted in descriptor)
//   - password: Password for descriptor encryption (must be non-empty)
//
// Returns:
//   - string: Content identifier of the encrypted file descriptor
//   - error: Non-nil if validation fails, reading fails, anonymization fails, encryption fails, or storage fails
//
// Call Flow:
//   - Called by: Privacy-enhanced upload operations, encrypted file storage
//   - Calls: EncryptedUploadWithBlockSize with default block size
//
// Time Complexity: O(n) where n is the number of blocks in the file
// Space Complexity: O(f) where f is the complete file size for in-memory processing
func (c *Client) EncryptedUpload(reader io.Reader, filename string, password string) (string, error) {
	return c.EncryptedUploadWithBlockSize(reader, filename, password, blocks.DefaultBlockSize)
}

// EncryptedUploadWithProgress uploads a file with encrypted descriptor and progress reporting.
// This method combines descriptor encryption with real-time progress updates,
// enabling secure uploads with user interface feedback for enhanced privacy workflows.
//
// Progress stages include all standard upload phases plus descriptor encryption:
//   - File reading and validation
//   - Block splitting and padding
//   - Block anonymization with randomizer selection
//   - Encrypted descriptor storage and finalization
//
// The method provides the same progress granularity as regular uploads while
// adding descriptor encryption for metadata protection.
//
// Parameters:
//   - reader: Data source for file content (must be non-nil)
//   - filename: Original filename for metadata (encrypted in descriptor)
//   - password: Password for descriptor encryption (must be non-empty)
//   - progress: Callback function for progress updates (nil to disable progress reporting)
//
// Returns:
//   - string: Content identifier of the encrypted file descriptor
//   - error: Non-nil if validation fails, reading fails, anonymization fails, encryption fails, or storage fails
//
// Call Flow:
//   - Called by: Privacy-enhanced upload operations with progress tracking
//   - Calls: EncryptedUploadWithBlockSizeAndProgress with default block size
//
// Time Complexity: O(n) where n is the number of blocks in the file
// Space Complexity: O(f) where f is the complete file size for in-memory processing
func (c *Client) EncryptedUploadWithProgress(reader io.Reader, filename string, password string, progress ProgressCallback) (string, error) {
	return c.EncryptedUploadWithBlockSizeAndProgress(reader, filename, password, blocks.DefaultBlockSize, progress)
}

// EncryptedUploadWithBlockSize uploads a file with encrypted descriptor and custom block size.
// This method enables both descriptor encryption and custom block sizing for specialized
// security and performance requirements in privacy-enhanced storage scenarios.
//
// The combination of descriptor encryption and custom block sizing provides:
//   - Enhanced metadata privacy through encryption
//   - Performance optimization through custom block sizes
//   - Flexibility for different deployment requirements
//   - Compatibility with various storage backends
//
// Parameters:
//   - reader: Data source for file content (must be non-nil)
//   - filename: Original filename for metadata (encrypted in descriptor)
//   - password: Password for descriptor encryption (must be non-empty)
//   - blockSize: Custom block size in bytes (must be positive)
//
// Returns:
//   - string: Content identifier of the encrypted file descriptor
//   - error: Non-nil if validation fails, reading fails, anonymization fails, encryption fails, or storage fails
//
// Call Flow:
//   - Called by: Custom-configured privacy-enhanced uploads
//   - Calls: EncryptedUploadWithBlockSizeAndProgress with specified parameters
//
// Time Complexity: O(n) where n is the number of blocks (affected by block size)
// Space Complexity: O(f) where f is the complete file size for in-memory processing
func (c *Client) EncryptedUploadWithBlockSize(reader io.Reader, filename string, password string, blockSize int) (string, error) {
	return c.EncryptedUploadWithBlockSizeAndProgress(reader, filename, password, blockSize, nil)
}

// EncryptedUploadWithBlockSizeAndProgress uploads a file with encrypted descriptor, custom block size, and comprehensive progress reporting.
// This is the primary encrypted upload implementation that provides full functionality including descriptor encryption,
// custom block sizing, real-time progress updates, 3-tuple XOR anonymization, and secure descriptor storage.
//
// Encrypted Upload Implementation Process:
//   1. Read and validate complete file data from input reader
//   2. Create file splitter with specified block size
//   3. Split file into fixed-size blocks with automatic padding
//   4. Create file descriptor with size and block information
//   5. Process each block through 3-tuple XOR anonymization
//   6. Select randomizer pairs for each block using intelligent selection
//   7. Store anonymized blocks and track storage usage
//   8. Cache blocks for performance optimization
//   9. Create encrypted descriptor store with password-based key derivation
//   10. Store encrypted descriptor using AES-256-GCM with Argon2id
//   11. Record metrics and return encrypted descriptor CID
//
// Encryption Features:
//   - Descriptor metadata encrypted with AES-256-GCM for confidentiality and integrity
//   - Password-based key derivation using Argon2id for key security
//   - File content anonymized with 3-tuple XOR as normal for privacy
//   - Secure storage of encrypted descriptors protecting sensitive metadata
//
// Progress Stages:
//   - "Reading file": Initial data reading and validation (0-100%)
//   - "Splitting file into blocks": Block creation with padding (0-100%)
//   - "Anonymizing blocks": XOR operations and storage (per block progress)
//   - "Saving encrypted file descriptor": Encrypted descriptor storage and finalization (0-100%)
//
// The encrypted descriptor protects sensitive metadata including filename, file size,
// block structure, and randomizer references while maintaining the same storage efficiency
// and privacy protection of the block content through XOR anonymization.
//
// Parameters:
//   - reader: Data source for file content (must be non-nil, fully consumed)
//   - filename: Original filename for metadata (encrypted in descriptor)
//   - password: Password for descriptor encryption (must be non-empty, used for Argon2id key derivation)
//   - blockSize: Custom block size in bytes (must be positive, affects memory usage and cache efficiency)
//   - progress: Callback function for progress updates (nil to disable progress reporting)
//
// Returns:
//   - string: Content identifier of the encrypted file descriptor
//   - error: Non-nil if validation fails, reading fails, splitting fails, anonymization fails, encryption fails, or storage fails
//
// Call Flow:
//   - Called by: All encrypted upload methods, primary encrypted upload implementation
//   - Calls: File splitter, randomizer selection, XOR operations, encrypted store creation, storage management
//
// Time Complexity: O(n) where n is the number of blocks in the file
// Space Complexity: O(f) where f is the complete file size for in-memory processing
func (c *Client) EncryptedUploadWithBlockSizeAndProgress(reader io.Reader, filename string, password string, blockSize int, progress ProgressCallback) (string, error) {
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

	// Split into blocks
	if progress != nil {
		progress("Splitting file into blocks", 0, 100)
	}

	splitter, err := blocks.NewSplitter(blockSize)
	if err != nil {
		return "", fmt.Errorf("failed to create splitter: %w", err)
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

	// Process blocks with anonymization
	totalBlocks := len(fileBlocks)
	var totalStorageUsed int64 = 0

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

	// Store descriptor using EncryptedStore
	if progress != nil {
		progress("Saving encrypted file descriptor", 0, 100)
	}

	// Create encrypted descriptor store with password-based key derivation
	// Uses AES-256-GCM encryption with Argon2id key derivation for security
	encryptedStore, err := descriptors.NewEncryptedStoreWithPassword(c.storageManager, password)
	if err != nil {
		return "", fmt.Errorf("failed to create encrypted descriptor store: %w", err)
	}

	// Save descriptor with encryption protecting filename, size, and block references
	descriptorCID, err := encryptedStore.Save(descriptor)
	if err != nil {
		return "", fmt.Errorf("failed to save encrypted descriptor: %w", err)
	}

	if progress != nil {
		progress("Saving encrypted file descriptor", 100, 100)
	}

	// Record metrics with actual storage used
	c.RecordUpload(fileSize, totalStorageUsed)

	return descriptorCID, nil
}

// SmartUpload uploads a file using the client's default encryption configuration.
// This method automatically determines whether to use encrypted or unencrypted upload
// based on the client's encryption configuration, providing a simplified API that
// respects the client's encryption policy settings.
//
// Encryption Decision Logic:
//   - If default encryption is enabled and password provider is available: encrypt
//   - If encryption is required (policy mode): encrypt or fail
//   - If no encryption configuration: use regular unencrypted upload
//   - If encryption fails and fallback is allowed: use unencrypted upload
//
// This method provides the recommended upload API for applications using clients
// with encryption configuration, eliminating the need to manually choose between
// encrypted and unencrypted upload methods.
//
// Parameters:
//   - reader: Data source for file content (must be non-nil)
//   - filename: Original filename for metadata
//
// Returns:
//   - string: Content identifier of the stored file descriptor
//   - error: Non-nil if validation fails, encryption fails (when required), or storage fails
//
// Call Flow:
//   - Called by: Application code using configured clients
//   - Calls: EncryptedUpload or Upload based on configuration
//
// Time Complexity: O(n) where n is the number of blocks in the file
// Space Complexity: O(f) where f is the complete file size for in-memory processing
func (c *Client) SmartUpload(reader io.Reader, filename string) (string, error) {
	return c.SmartUploadWithBlockSizeAndProgress(reader, filename, blocks.DefaultBlockSize, nil)
}

// SmartUploadWithProgress uploads a file using the client's encryption configuration with progress reporting.
// This method combines the encryption decision logic of SmartUpload with progress reporting
// capabilities, providing an intelligent upload API with user interface integration.
//
// Parameters:
//   - reader: Data source for file content (must be non-nil)
//   - filename: Original filename for metadata
//   - progress: Callback function for progress updates (nil to disable progress reporting)
//
// Returns:
//   - string: Content identifier of the stored file descriptor
//   - error: Non-nil if validation fails, encryption fails (when required), or storage fails
//
// Call Flow:
//   - Called by: UI applications using configured clients with progress tracking
//   - Calls: SmartUploadWithBlockSizeAndProgress with default block size
//
// Time Complexity: O(n) where n is the number of blocks in the file
// Space Complexity: O(f) where f is the complete file size for in-memory processing
func (c *Client) SmartUploadWithProgress(reader io.Reader, filename string, progress ProgressCallback) (string, error) {
	return c.SmartUploadWithBlockSizeAndProgress(reader, filename, blocks.DefaultBlockSize, progress)
}

// SmartUploadWithBlockSize uploads a file using encryption configuration with custom block size.
// This method provides intelligent encryption decisions while allowing custom block sizing
// for specialized performance or compatibility requirements.
//
// Parameters:
//   - reader: Data source for file content (must be non-nil)
//   - filename: Original filename for metadata
//   - blockSize: Custom block size in bytes (must be positive)
//
// Returns:
//   - string: Content identifier of the stored file descriptor
//   - error: Non-nil if validation fails, encryption fails (when required), or storage fails
//
// Call Flow:
//   - Called by: Performance-optimized uploads using configured clients
//   - Calls: SmartUploadWithBlockSizeAndProgress with specified block size
//
// Time Complexity: O(n) where n is the number of blocks (affected by block size)
// Space Complexity: O(f) where f is the complete file size for in-memory processing
func (c *Client) SmartUploadWithBlockSize(reader io.Reader, filename string, blockSize int) (string, error) {
	return c.SmartUploadWithBlockSizeAndProgress(reader, filename, blockSize, nil)
}

// SmartUploadWithBlockSizeAndProgress uploads a file using encryption configuration with full customization.
// This is the primary intelligent upload implementation that combines encryption decision logic,
// custom block sizing, and progress reporting for comprehensive upload functionality.
//
// Encryption Decision Implementation:
//   1. Check if client has encryption configuration
//   2. If default encryption enabled, attempt to get password from provider
//   3. If password available, use encrypted upload
//   4. If encryption required and password fails, return error
//   5. If encryption optional and password fails, fallback to unencrypted if allowed
//   6. If no encryption configuration, use standard unencrypted upload
//
// This method implements the complete encryption policy enforcement and provides
// backward compatibility with clients that don't have encryption configuration.
//
// Parameters:
//   - reader: Data source for file content (must be non-nil)
//   - filename: Original filename for metadata
//   - blockSize: Custom block size in bytes (must be positive)
//   - progress: Callback function for progress updates (nil to disable progress reporting)
//
// Returns:
//   - string: Content identifier of the stored file descriptor
//   - error: Non-nil if validation fails, encryption fails (when required), or storage fails
//
// Call Flow:
//   - Called by: All SmartUpload variants, primary implementation
//   - Calls: EncryptedUploadWithBlockSizeAndProgress or UploadWithBlockSizeAndProgress
//
// Time Complexity: O(n) where n is the number of blocks in the file
// Space Complexity: O(f) where f is the complete file size for in-memory processing
func (c *Client) SmartUploadWithBlockSizeAndProgress(reader io.Reader, filename string, blockSize int, progress ProgressCallback) (string, error) {
	// Check if client has encryption configuration
	if c.encryptionConfig == nil || !c.encryptionConfig.EnableDefaultEncryption {
		// No encryption configuration or default encryption disabled - use regular upload
		return c.UploadWithBlockSizeAndProgress(reader, filename, blockSize, progress)
	}

	// Default encryption is enabled - attempt to get password from provider
	if c.encryptionConfig.DefaultPasswordProvider == nil {
		if c.encryptionConfig.RequireEncryption {
			return "", fmt.Errorf("encryption required but no password provider configured")
		}
		// No password provider but encryption not required - fallback to unencrypted if allowed
		if c.encryptionConfig.AllowUnencrypted {
			return c.UploadWithBlockSizeAndProgress(reader, filename, blockSize, progress)
		}
		return "", fmt.Errorf("no password provider available and unencrypted uploads not allowed")
	}

	// Get password from provider
	password, err := c.encryptionConfig.DefaultPasswordProvider()
	if err != nil {
		if c.encryptionConfig.RequireEncryption {
			return "", fmt.Errorf("failed to get password for required encryption: %w", err)
		}
		// Password provider failed but encryption not required - fallback if allowed
		if c.encryptionConfig.AllowUnencrypted {
			return c.UploadWithBlockSizeAndProgress(reader, filename, blockSize, progress)
		}
		return "", fmt.Errorf("failed to get password and unencrypted uploads not allowed: %w", err)
	}

	// Password available - use encrypted upload
	if password == "" {
		if c.encryptionConfig.RequireEncryption {
			return "", fmt.Errorf("empty password provided but encryption is required")
		}
		// Empty password but encryption not required - fallback if allowed
		if c.encryptionConfig.AllowUnencrypted {
			return c.UploadWithBlockSizeAndProgress(reader, filename, blockSize, progress)
		}
		return "", fmt.Errorf("empty password provided and unencrypted uploads not allowed")
	}

	// Use encrypted upload with the provided password
	return c.EncryptedUploadWithBlockSizeAndProgress(reader, filename, password, blockSize, progress)
}

// RecordUpload records comprehensive upload metrics for performance analysis and monitoring.
// This method updates the client's metrics tracking system to record successful upload operations,
// enabling monitoring of storage efficiency, system usage patterns, and performance trends.
//
// Metrics Recorded:
//   - Original file size before processing for efficiency analysis
//   - Actual storage used including data blocks and new randomizers
//   - Storage overhead ratio for optimization insights
//   - Upload frequency and performance trends
//   - System utilization patterns
//
// The metrics are used for:
//   - Storage efficiency analysis and optimization
//   - Performance monitoring and capacity planning
//   - System health monitoring and alerting
//   - Usage pattern analysis and trend identification
//   - Debugging and troubleshooting upload issues
//
// Parameters:
//   - originalBytes: Original file size in bytes before processing
//   - storedBytes: Total bytes actually stored including data blocks and new randomizers
//
// Call Flow:
//   - Called by: All upload methods upon successful completion
//   - Calls: metrics.RecordUpload for actual metrics recording
//
// Time Complexity: O(1) - simple metrics update with thread safety
// Space Complexity: O(1) - no additional memory allocation
func (c *Client) RecordUpload(originalBytes, storedBytes int64) {
	c.metrics.RecordUpload(originalBytes, storedBytes)
}

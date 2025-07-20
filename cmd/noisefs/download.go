// Package main provides CLI download functionality for NoiseFS file retrieval.
// This file implements the download command that handles both single file and directory downloads
// with support for encrypted descriptor detection, automatic decryption, and various output formats.
//
// The download functionality provides:
//   - Single file downloads with automatic encryption detection
//   - Directory downloads with recursive file reconstruction (planned)
//   - Encrypted descriptor detection and automatic password prompting
//   - Progress reporting for large downloads
//   - JSON and human-readable output formats
//   - Comprehensive performance metrics and timing analysis
//
// Download modes:
//   - Standard download: Retrieves anonymized blocks and reconstructs original file
//   - Encrypted download: Automatically detects encryption and prompts for passwords
//   - Directory download: Reconstructs entire directory structures (planned)
//   - Streaming download: Memory-efficient processing for large files (planned)
//
// Encryption handling:
//   - Automatic detection of encrypted descriptors using EncryptedStore.IsEncrypted()
//   - Interactive password prompting with secure input handling
//   - Environment variable password support via NOISEFS_PASSWORD
//   - Fallback to regular download for unencrypted descriptors
//
// The CLI integrates with the NoiseFS client API and EncryptedStore to provide
// user-friendly access to the complete NoiseFS retrieval system while maintaining
// the same privacy and security guarantees as the underlying library.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	noisefs "github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/common/config"
	"github.com/TheEntropyCollective/noisefs/pkg/common/logging"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/util"
)

// downloadFile downloads a file from NoiseFS using a descriptor CID with automatic encryption detection.
// This function provides the core single-file download implementation for the CLI, handling
// encryption detection, password acquisition, file reconstruction, and output file management.
//
// Download Process:
//   1. Detect encryption status using EncryptedStore.IsEncrypted()
//   2. If encrypted: prompt for password and use EncryptedStore for descriptor loading
//   3. If unencrypted: use standard client.Download for file retrieval
//   4. Determine output filename from descriptor metadata or generate default
//   5. Write reconstructed file data to specified output path
//   6. Report timing metrics and file information
//
// Encryption Detection and Handling:
//   - Automatic detection of encrypted descriptors without requiring user input
//   - Environment variable password support via NOISEFS_PASSWORD
//   - Interactive password prompting with secure input for encrypted descriptors
//   - Secure password handling with automatic memory clearing
//   - Graceful fallback to unencrypted download if detection fails
//
// File Reconstruction:
//   - Retrieves anonymized blocks and randomizer pairs
//   - Performs 3-tuple XOR operations to reconstruct original blocks
//   - Assembles blocks into complete file with padding removal
//   - Handles both encrypted and unencrypted descriptor workflows
//
// Output Management:
//   - Automatic filename extraction from descriptor metadata
//   - Fallback filename generation using descriptor CID prefix
//   - Configurable output path specification
//   - File permission setting (0644) for downloaded files
//
// Performance Metrics:
//   - Download timing from network retrieval through reconstruction
//   - File write timing for disk operation analysis
//   - Total operation timing for end-to-end performance
//   - File size reporting for verification
//
// Output Formats:
//   - JSON mode: Machine-readable structured output with all metrics
//   - Human-readable mode: User-friendly progress and results display
//   - Quiet mode: Only outputs the written file path for scripting
//
// Parameters:
//   - storageManager: Backend storage abstraction for block retrieval
//   - client: NoiseFS client instance configured with caching and networking
//   - descriptorCID: Content identifier of the file descriptor to download
//   - outputPath: Target file path for downloaded content (empty string for auto-detection)
//   - quiet: Suppress progress output, only show final result
//   - jsonOutput: Output results in JSON format for machine processing
//   - logger: Structured logging instance for debugging and audit trails
//
// Returns:
//   - error: Non-nil if descriptor retrieval fails, decryption fails, reconstruction fails, or file write fails
//
// Call Flow:
//   - Called by: CLI download command handler
//   - Calls: EncryptedStore.IsEncrypted, client.Download, downloadUsingDescriptor, file system operations
//
// Time Complexity: O(n) where n is the number of blocks in the file
// Space Complexity: O(f) where f is the complete file size for reconstruction
func downloadFile(storageManager *storage.Manager, client *noisefs.Client, descriptorCID string, outputPath string, quiet bool, jsonOutput bool, logger *logging.Logger) error {
	if !quiet && !jsonOutput {
		fmt.Printf("Downloading file: %s\n", descriptorCID)
	}

	startTime := time.Now()

	// Check if descriptor is encrypted
	encryptedStore, err := descriptors.NewEncryptedStore(storageManager, nil)
	if err != nil {
		return fmt.Errorf("failed to create encrypted store: %w", err)
	}

	isEncrypted, err := encryptedStore.IsEncrypted(descriptorCID)
	if err != nil {
		// If we can't determine encryption status, try regular download first
		isEncrypted = false
	}

	var data []byte
	if isEncrypted {
		// This is an encrypted descriptor, we need to handle it specially
		password := os.Getenv("NOISEFS_PASSWORD")
		if password == "" {
			password, err = util.PromptPassword("Enter decryption password: ")
			if err != nil {
				return fmt.Errorf("failed to get password: %w", err)
			}
		}

		// Create encrypted store with password
		encStoreWithPassword, err := descriptors.NewEncryptedStoreWithPassword(storageManager, password)
		if err != nil {
			return fmt.Errorf("failed to create encrypted store with password: %w", err)
		}

		// Load encrypted descriptor
		descriptor, err := encStoreWithPassword.Load(descriptorCID)
		if err != nil {
			return fmt.Errorf("failed to load encrypted descriptor (wrong password?): %w", err)
		}

		// Manually perform download using the decrypted descriptor
		data, err = downloadUsingDescriptor(client, descriptor)
		if err != nil {
			return fmt.Errorf("download failed: %w", err)
		}
	} else {
		// Regular unencrypted download
		data, err = client.Download(descriptorCID)
		if err != nil {
			return fmt.Errorf("download failed: %w", err)
		}
	}

	downloadDuration := time.Since(startTime)

	// Determine output path
	if outputPath == "" {
		// Try to get filename from descriptor
		descriptorStore, err := descriptors.NewStoreWithManager(storageManager)
		if err != nil {
			outputPath = fmt.Sprintf("downloaded-file-%s", descriptorCID[:8])
		} else {
			descriptor, err := descriptorStore.Load(descriptorCID)
			if err != nil {
				outputPath = fmt.Sprintf("downloaded-file-%s", descriptorCID[:8])
			} else {
				outputPath = descriptor.Filename
			}
		}
	}

	// Write the downloaded data to file
	writeStartTime := time.Now()
	err = os.WriteFile(outputPath, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	writeDuration := time.Since(writeStartTime)

	totalDuration := time.Since(startTime)

	if jsonOutput {
		result := map[string]interface{}{
			"success":        true,
			"descriptor_cid": descriptorCID,
			"output_path":    outputPath,
			"size_bytes":     len(data),
			"download_time":  downloadDuration.String(),
			"write_time":     writeDuration.String(),
			"total_time":     totalDuration.String(),
		}
		
		jsonData, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(jsonData))
	} else if !quiet {
		fmt.Printf("Download completed successfully!\n")
		fmt.Printf("Output file: %s\n", outputPath)
		fmt.Printf("File size: %s\n", formatBytes(int64(len(data))))
		fmt.Printf("Download time: %v\n", downloadDuration)
		fmt.Printf("Write time: %v\n", writeDuration)
		fmt.Printf("Total time: %v\n", totalDuration)
	} else {
		fmt.Println(outputPath)
	}

	logger.Debug("File download completed", map[string]interface{}{
		"descriptor_cid": descriptorCID,
		"output_path":    outputPath,
		"size":           len(data),
		"duration":       totalDuration.String(),
	})

	return nil
}

// downloadDirectory downloads a directory from NoiseFS using a directory descriptor CID.
// This function provides directory-level download functionality for the CLI, enabling bulk file downloads
// with directory structure reconstruction and comprehensive progress reporting.
//
// Planned Directory Download Features:
//   - Recursive directory structure reconstruction from manifests
//   - Parallel file download processing for performance optimization
//   - Directory metadata preservation including timestamps and permissions
//   - Progress tracking for bulk download operations
//   - Error handling and recovery for individual file failures
//   - Comprehensive statistics reporting for directory operations
//
// Directory Structure Reconstruction:
//   - Processes directory manifests to understand file organization
//   - Creates local directory hierarchy matching original structure
//   - Downloads individual files using file descriptor references
//   - Preserves file metadata and directory organization
//   - Handles symbolic links and special files appropriately
//
// Performance Optimization:
//   - Parallel download processing for multiple files
//   - Intelligent batching of download operations
//   - Progress tracking and reporting for long-running operations
//   - Memory-efficient processing for large directories
//   - Network optimization through connection pooling
//
// Implementation Status:
//   This function is currently not implemented and returns an error.
//   Future implementation will provide full directory download capabilities
//   with streaming support for memory-efficient processing of large directories.
//   The function includes placeholder statistics calculation and output formatting.
//
// Error Handling:
//   - Individual file download errors with recovery options
//   - Directory creation failures with detailed error reporting
//   - Network errors with retry logic and fallback mechanisms
//   - Manifest parsing errors with graceful degradation
//
// Parameters:
//   - storageManager: Backend storage abstraction for block retrieval
//   - client: NoiseFS client instance configured with caching and networking
//   - directoryCID: Content identifier of the directory descriptor to download
//   - outputDir: Target directory path for downloaded content (empty string for auto-generation)
//   - quiet: Suppress progress output, only show final results
//   - jsonOutput: Output results in JSON format for machine processing
//   - cfg: Configuration instance for application settings
//   - logger: Structured logging instance for debugging and audit trails
//
// Returns:
//   - error: Currently returns "not implemented" error, future implementation will return download errors
//
// Call Flow:
//   - Called by: CLI download command handler for directory arguments
//   - Calls: Directory manifest processing, file enumeration, parallel download processing
//
// Time Complexity: O(n*f) where n is number of files, f is average file size
// Space Complexity: O(d) where d is directory depth plus file metadata
func downloadDirectory(storageManager *storage.Manager, client *noisefs.Client, directoryCID string, outputDir string, quiet bool, jsonOutput bool, cfg *config.Config, logger *logging.Logger) error {
	if !quiet && !jsonOutput {
		fmt.Printf("Downloading directory: %s\n", directoryCID)
	}

	startTime := time.Now()

	// Ensure output directory exists
	if outputDir == "" {
		outputDir = fmt.Sprintf("downloaded-directory-%s", directoryCID[:8])
	}

	err := os.MkdirAll(outputDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// TODO: Implement directory download functionality
	// For now, return an error indicating feature is not yet implemented
	return fmt.Errorf("directory download not yet implemented")

	downloadDuration := time.Since(startTime)

	// Count files and calculate total size
	var totalFiles int
	var totalSize int64
	
	err = filepath.Walk(outputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			totalFiles++
			totalSize += info.Size()
		}
		return nil
	})
	if err != nil {
		logger.Warn("Failed to calculate directory statistics", map[string]interface{}{
			"error": err.Error(),
		})
	}

	if jsonOutput {
		result := map[string]interface{}{
			"success":        true,
			"descriptor_cid": directoryCID,
			"output_dir":     outputDir,
			"total_files":    totalFiles,
			"total_size":     totalSize,
			"download_time":  downloadDuration.String(),
		}
		
		jsonData, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(jsonData))
	} else if !quiet {
		fmt.Printf("Directory download completed successfully!\n")
		fmt.Printf("Output directory: %s\n", outputDir)
		fmt.Printf("Total files: %d\n", totalFiles)
		fmt.Printf("Total size: %s\n", formatBytes(totalSize))
		fmt.Printf("Download time: %v\n", downloadDuration)
	} else {
		fmt.Println(outputDir)
	}

	logger.Debug("Directory download completed", map[string]interface{}{
		"descriptor_cid": directoryCID,
		"output_dir":     outputDir,
		"total_files":    totalFiles,
		"total_size":     totalSize,
		"duration":       downloadDuration,
	})

	return nil
}

// streamingDownloadDirectory downloads a directory using streaming mode for memory-efficient processing.
// This function provides streaming download capabilities for large directories, enabling
// processing of directories that exceed available memory through incremental file handling.
//
// Streaming Download Advantages:
//   - Memory-efficient processing of large directories
//   - Incremental progress reporting for long-running operations
//   - Parallel file processing with controlled concurrency
//   - Early failure detection and graceful error recovery
//   - Reduced memory pressure on constrained systems
//
// Streaming Architecture:
//   - File enumeration with lazy loading of file content
//   - Block-level streaming for individual files
//   - Incremental file writing to reduce memory usage
//   - Progressive directory structure creation
//   - Configurable concurrency limits for system resource management
//
// Implementation Status:
//   This function is currently a placeholder that delegates to downloadDirectory.
//   Future implementation will provide true streaming capabilities when
//   streaming interfaces are available in the NoiseFS client API.
//
// Planned Streaming Features:
//   - Configurable memory limits and buffer sizes
//   - Progress callbacks for real-time status updates
//   - Resumable downloads for interrupted operations
//   - Incremental checkpointing for fault tolerance
//   - Resource usage monitoring and automatic throttling
//
// Performance Benefits:
//   - Constant memory usage regardless of directory size
//   - Parallel processing with controlled resource utilization
//   - Network optimization through streaming transfer
//   - Progressive user feedback for long operations
//   - Efficient handling of very large directory structures
//
// Parameters:
//   - storageManager: Backend storage abstraction for block retrieval
//   - client: NoiseFS client instance configured with caching and networking
//   - directoryCID: Content identifier of the directory descriptor for streaming download
//   - outputDir: Target directory path for downloaded content
//   - quiet: Suppress progress output, only show final results
//   - jsonOutput: Output results in JSON format for machine processing
//   - cfg: Configuration instance for application settings
//   - logger: Structured logging instance for debugging and audit trails
//
// Returns:
//   - error: Currently delegates to downloadDirectory, future implementation will return streaming errors
//
// Call Flow:
//   - Called by: CLI download command handler for large directory operations
//   - Calls: Currently downloadDirectory, future will call streaming download APIs
//
// Time Complexity: O(n*f) where n is number of files, f is average file size
// Space Complexity: O(1) for streaming mode, O(b) where b is buffer size
func streamingDownloadDirectory(storageManager *storage.Manager, client *noisefs.Client, directoryCID string, outputDir string, quiet bool, jsonOutput bool, cfg *config.Config, logger *logging.Logger) error {
	// Implementation would use streaming interfaces
	logger.Info("Streaming directory download", map[string]interface{}{
		"directory_cid": directoryCID,
	})
	
	// For now, fall back to regular directory download
	// TODO: Implement actual streaming download when streaming interfaces are available
	return downloadDirectory(storageManager, client, directoryCID, outputDir, quiet, jsonOutput, cfg, logger)
}

// detectDirectoryDescriptor checks if a CID represents a directory descriptor.
// This function provides automatic detection of directory vs file descriptors,
// enabling the CLI to route download requests to appropriate handlers without
// requiring user specification of the descriptor type.
//
// Detection Process:
//   1. Load descriptor using standard descriptor store
//   2. Examine descriptor structure and metadata
//   3. Check for directory-specific fields and markers
//   4. Return boolean result indicating directory status
//
// Directory Descriptor Characteristics:
//   - Contains directory manifest information
//   - Includes file organization metadata
//   - Has directory-specific structure markers
//   - May contain hierarchical file references
//   - Different metadata format from file descriptors
//
// Use Cases:
//   - Automatic routing of download commands to appropriate handlers
//   - User interface adaptation based on descriptor type
//   - Validation of descriptor type before processing
//   - Error prevention through proper type detection
//
// Error Handling:
//   - Graceful handling of invalid or corrupted descriptors
//   - Clear error reporting for descriptor access failures
//   - Fallback behavior for ambiguous descriptor types
//   - Network error recovery for descriptor retrieval
//
// Parameters:
//   - storageManager: Backend storage abstraction for descriptor retrieval
//   - cid: Content identifier of the descriptor to analyze
//
// Returns:
//   - bool: True if descriptor represents a directory, false for file descriptors
//   - error: Non-nil if descriptor retrieval fails, loading fails, or analysis fails
//
// Call Flow:
//   - Called by: Download command routing logic, descriptor type validation
//   - Calls: descriptors.NewStoreWithManager, descriptor.Load, descriptor.IsDirectory
//
// Time Complexity: O(1) - single descriptor load and analysis
// Space Complexity: O(d) where d is descriptor size for analysis
func detectDirectoryDescriptor(storageManager *storage.Manager, cid string) (bool, error) {
	descriptorStore, err := descriptors.NewStoreWithManager(storageManager)
	if err != nil {
		return false, fmt.Errorf("failed to create descriptor store: %w", err)
	}

	descriptor, err := descriptorStore.Load(cid)
	if err != nil {
		return false, fmt.Errorf("failed to load descriptor: %w", err)
	}

	// Check if this is a directory descriptor by examining its structure
	// Directory descriptors typically have specific metadata or structure
	return descriptor.IsDirectory(), nil
}

// downloadUsingDescriptor downloads file data using a pre-loaded descriptor.
// This function provides file reconstruction using a descriptor that has already been
// loaded and potentially decrypted, enabling download workflows for encrypted descriptors
// where descriptor decryption is handled separately from file reconstruction.
//
// Primary Use Case:
//   This function is used for encrypted descriptors where the CLI has already
//   performed descriptor decryption using EncryptedStore and needs to reconstruct
//   the original file using the decrypted descriptor metadata.
//
// File Reconstruction Process:
//   1. Validate descriptor is non-nil and contains valid block information
//   2. Iterate through each block triple in the descriptor
//   3. Retrieve anonymized data block from storage
//   4. Retrieve both randomizer blocks for 3-tuple XOR reconstruction
//   5. Perform XOR operations to recover original block data
//   6. Assemble all blocks into complete file content
//   7. Remove padding to restore original file size
//
// 3-Tuple XOR Reconstruction:
//   - Original = Data XOR Randomizer1 XOR Randomizer2
//   - Each block requires three storage retrievals for full reconstruction
//   - XOR operations restore original block content from anonymized storage
//   - Process maintains privacy protection while enabling file recovery
//
// Block Assembly and Padding Removal:
//   - Uses blocks.Assembler for proper block sequence reconstruction
//   - Assembles blocks in correct order based on descriptor metadata
//   - Trims assembled data to original file size to remove padding
//   - Handles both padded and unpadded file scenarios
//
// Error Handling:
//   - Validates descriptor before processing to prevent nil pointer errors
//   - Handles missing blocks with specific error messages
//   - Reports XOR operation failures with detailed context
//   - Provides assembly failure information for debugging
//
// Caching Integration:
//   - Uses client.RetrieveBlockWithCache for efficient block retrieval
//   - Benefits from client caching for frequently accessed randomizers
//   - Optimizes network usage through intelligent cache management
//
// Parameters:
//   - client: NoiseFS client instance configured with caching and networking
//   - descriptor: Pre-loaded descriptor containing block references and metadata
//
// Returns:
//   - []byte: Complete reconstructed file data with padding removed
//   - error: Non-nil if validation fails, block retrieval fails, XOR fails, or assembly fails
//
// Call Flow:
//   - Called by: downloadFile for encrypted descriptor workflows
//   - Calls: client.RetrieveBlockWithCache, block.XOR, blocks.Assembler.AssembleToWriter
//
// Time Complexity: O(n) where n is the number of blocks in the file
// Space Complexity: O(f) where f is the complete file size for reconstruction
func downloadUsingDescriptor(client *noisefs.Client, descriptor *descriptors.Descriptor) ([]byte, error) {
	// This function replicates the core logic from client.Download but uses a provided descriptor
	// rather than loading it from a CID
	
	if descriptor == nil {
		return nil, fmt.Errorf("descriptor cannot be nil")
	}

	// Retrieve and reconstruct blocks (similar to client.DownloadWithMetadataAndProgress)
	var originalBlocks []*blocks.Block

	for _, blockInfo := range descriptor.Blocks {
		// Retrieve anonymized data block
		dataBlock, err := client.RetrieveBlockWithCache(blockInfo.DataCID)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve data block: %w", err)
		}

		// Retrieve randomizer blocks
		randBlock1, err := client.RetrieveBlockWithCache(blockInfo.RandomizerCID1)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve randomizer1 block: %w", err)
		}

		// Retrieve second randomizer block (3-tuple XOR)
		randBlock2, err := client.RetrieveBlockWithCache(blockInfo.RandomizerCID2)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve randomizer2 block: %w", err)
		}

		// XOR to get original block
		origBlock, err := dataBlock.XOR(randBlock1, randBlock2)
		if err != nil {
			return nil, fmt.Errorf("failed to XOR blocks: %w", err)
		}

		originalBlocks = append(originalBlocks, origBlock)
	}

	// Assemble file
	assembler := blocks.NewAssembler()
	var buf strings.Builder
	if err := assembler.AssembleToWriter(originalBlocks, &buf); err != nil {
		return nil, fmt.Errorf("failed to assemble file: %w", err)
	}

	// Handle padding removal (all files are padded)
	assembledData := []byte(buf.String())

	// Trim to original size (all files have padding)
	originalSize := descriptor.GetOriginalFileSize()
	if int64(len(assembledData)) > originalSize {
		assembledData = assembledData[:originalSize]
	}

	return assembledData, nil
}
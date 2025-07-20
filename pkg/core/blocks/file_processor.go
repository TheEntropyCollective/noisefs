// Package blocks provides file-specific block processing functionality.
// This file implements the FileBlockProcessor which serves as an adapter between
// individual file processing and the broader directory processing system,
// handling file-specific concerns like CID tracking and result aggregation.
package blocks

import (
	"sync"

	"github.com/TheEntropyCollective/noisefs/pkg/core/crypto"
)

// FileBlockProcessor provides file-specific block processing functionality within directory operations.
// This processor acts as an adapter between the streaming block processing system and the
// directory processing infrastructure, handling file-specific concerns such as CID tracking,
// result aggregation, and thread-safe access to file metadata.
//
// The processor implements the BlockProcessor interface to integrate with StreamingSplitter
// while maintaining compatibility with the DirectoryBlockProcessor interface for storage
// operations. It tracks the file's primary CID (derived from the first block) and provides
// thread-safe access to processing results.
//
// Key Responsibilities:
//   - Adapting block processing calls to directory processing interface
//   - Tracking file content identifier (CID) based on first block
//   - Providing thread-safe access to file processing metadata
//   - Maintaining encryption context for filename operations
//
// Thread Safety:
//   The processor uses mutex protection for all shared state access, ensuring safe
//   concurrent usage within the directory processing worker pool.
//
// Call Flow:
//   - Created by: DirectoryProcessor.processFileEntry for each file
//   - Used by: StreamingSplitter during file block processing
//   - Integrates with: DirectoryBlockProcessor for actual block storage
//
// Time Complexity: O(1) per block processed
// Space Complexity: O(1) - minimal overhead beyond underlying processor
type FileBlockProcessor struct {
	FilePath      string                    // Original filesystem path of the file being processed
	FileSize      int64                     // Size of the file in bytes for validation and metadata
	Processor     DirectoryBlockProcessor   // Underlying processor for actual block storage operations
	Results       []*ProcessResult          // Collection of processing results (currently unused but available for future features)
	EncryptionKey *crypto.EncryptionKey     // Encryption key for filename operations within directory context
	fileCID       string                    // Primary content identifier derived from first block (thread-safe access required)
	mutex         sync.Mutex               // Mutex protecting concurrent access to fileCID and shared state
}

// ProcessBlock implements the BlockProcessor interface for integration with StreamingSplitter.
// This method receives blocks as they are created during file splitting and forwards them
// to the underlying DirectoryBlockProcessor for storage while tracking file metadata.
//
// The method captures the first block's CID as the primary file identifier, which is used
// for creating file entries in directory manifests. All subsequent blocks are processed
// normally through the directory processing system.
//
// Thread Safety:
//   Uses mutex protection to ensure safe concurrent access to the fileCID field,
//   preventing race conditions when multiple blocks are processed simultaneously.
//
// Parameters:
//   - blockIndex: Sequential index of the block within the file (0-based)
//   - block: The processed block ready for storage (contains data and content-addressed ID)
//
// Returns:
//   - error: Non-nil if the underlying directory processor fails to handle the block
//
// Call Flow:
//   - Called by: StreamingSplitter.Split during file processing
//   - Calls: DirectoryBlockProcessor.ProcessDirectoryBlock for actual storage
//
// Time Complexity: O(1) - constant time operations plus underlying processor complexity
// Space Complexity: O(1) - no additional memory allocation beyond mutex overhead
func (fbp *FileBlockProcessor) ProcessBlock(blockIndex int, block *Block) error {
	fbp.mutex.Lock()
	defer fbp.mutex.Unlock()

	// Capture the first block's CID as the primary file identifier
	// This CID will be used in directory manifests to reference the file
	if blockIndex == 0 {
		fbp.fileCID = block.ID
	}

	// Forward the block to the underlying directory processor for storage
	return fbp.Processor.ProcessDirectoryBlock(blockIndex, block)
}

// GetFileCID returns the primary content identifier for the file being processed.
// The file CID is derived from the first block of the file and serves as the
// primary reference for the file within directory manifests and storage systems.
//
// This method provides thread-safe access to the file CID, which may be accessed
// concurrently by directory processing operations that need to create file entries
// in directory manifests.
//
// Returns:
//   - string: Content identifier of the file (empty string if no blocks have been processed)
//
// Call Flow:
//   - Called by: DirectoryProcessor.processFileEntry when creating directory entries
//   - Called by: Directory manifest creation operations
//
// Time Complexity: O(1) - simple mutex-protected field access
// Space Complexity: O(1) - no memory allocation, returns existing string
func (fbp *FileBlockProcessor) GetFileCID() string {
	fbp.mutex.Lock()
	defer fbp.mutex.Unlock()
	return fbp.fileCID
}
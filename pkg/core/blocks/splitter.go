// Package blocks provides file splitting functionality for NoiseFS.
// This file implements the Splitter type which divides files and data into
// fixed-size blocks with automatic padding for privacy protection and
// optimal caching performance in the NoiseFS anonymization system.
package blocks

import (
	"errors"
	"io"
)

// Splitter handles the division of files and data into fixed-size blocks for NoiseFS storage.
// This splitter implements the core block creation functionality that ensures all blocks
// have consistent size for privacy protection and optimal caching performance.
//
// The splitter creates blocks of exactly the configured size, padding smaller chunks
// with zero bytes to maintain consistent block sizes throughout the system. This
// consistency is crucial for:
//   - Privacy protection: All blocks appear the same size preventing fingerprinting
//   - Cache efficiency: Uniform block sizes optimize randomizer block reuse
//   - Network efficiency: Consistent sizes improve IPFS networking performance
//
// Key Features:
//   - Fixed-size block creation with automatic zero-padding
//   - Support for both streaming (io.Reader) and in-memory ([]byte) data sources
//   - Content-addressed block identifiers for each created block
//   - Validation of input parameters and error handling
//
// Call Flow:
//   - Created by: NewSplitter factory function or DefaultSplitter convenience function
//   - Used by: Client upload operations, DirectoryProcessor file processing
//   - Creates: Block instances with content-addressed identifiers
//
// Time Complexity: O(n) where n is the total data size
// Space Complexity: O(b) where b is the number of blocks created
type Splitter struct {
	blockSize int // Size in bytes for each block (typically DefaultBlockSize = 128 KiB)
}

// NewSplitter creates a new file splitter with the specified block size.
// This factory function validates the block size parameter and initializes
// a splitter ready for processing files and data into fixed-size blocks.
//
// The block size determines the size of all blocks created by this splitter.
// Typically this should be DefaultBlockSize (128 KiB) for consistency with
// the NoiseFS system, but custom sizes can be used for specific requirements.
//
// Parameters:
//   - blockSize: Size in bytes for each block (must be positive)
//
// Returns:
//   - *Splitter: New splitter instance ready for block creation
//   - error: Non-nil if blockSize is invalid (zero or negative)
//
// Call Flow:
//   - Called by: Client initialization, DirectoryProcessor setup, testing code
//   - Calls: None (simple constructor with validation)
//
// Time Complexity: O(1) - constant time initialization
// Space Complexity: O(1) - minimal memory allocation
func NewSplitter(blockSize int) (*Splitter, error) {
	if blockSize <= 0 {
		return nil, errors.New("block size must be positive")
	}

	return &Splitter{
		blockSize: blockSize,
	}, nil
}

// Split divides data from a reader into fixed-size blocks with automatic padding.
// This method reads data from the provided io.Reader and creates blocks of exactly
// the configured block size, padding the final block with zeros if necessary.
//
// The splitting process:
//   1. Read data in chunks up to block size
//   2. Create full-size block (pad with zeros if chunk is smaller)
//   3. Generate content-addressed identifier for the block
//   4. Continue until all data is processed
//
// All blocks created will be exactly blockSize bytes, with zero-padding applied
// to maintain consistent block sizes for privacy and caching benefits.
//
// Parameters:
//   - reader: Data source to read from (must be non-nil)
//
// Returns:
//   - []*Block: Slice of blocks containing the split data with content-addressed IDs
//   - error: Non-nil if reader is nil, reading fails, or block creation fails
//
// Call Flow:
//   - Called by: Client upload operations, file processing workflows
//   - Calls: reader.Read, NewBlock for each created block
//
// Time Complexity: O(n) where n is the total data size
// Space Complexity: O(b) where b is the number of blocks (data size / block size)
func (s *Splitter) Split(reader io.Reader) ([]*Block, error) {
	if reader == nil {
		return nil, errors.New("reader cannot be nil")
	}

	var blocks []*Block
	buffer := make([]byte, s.blockSize)

	for {
		// Read up to blockSize bytes from the reader
		n, err := reader.Read(buffer)
		if n > 0 {
			// Create fixed-size block with zero-padding for consistent block sizes
			blockData := make([]byte, s.blockSize) // Always allocate full block size
			copy(blockData, buffer[:n])            // Copy actual data
			// Remaining bytes are automatically zero-padded

			// Create block with content-addressed identifier
			block, err := NewBlock(blockData)
			if err != nil {
				return nil, err
			}

			blocks = append(blocks, block)
		}

		// Handle end of file and other errors
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, err
		}
	}

	return blocks, nil
}

// SplitBytes divides a byte slice into fixed-size blocks with automatic padding.
// This method provides in-memory block splitting for data that is already loaded
// into memory, creating blocks of exactly the configured size with zero-padding
// applied to the final block if necessary.
//
// The splitting process:
//   1. Iterate through data in chunks of block size
//   2. Create full-size block for each chunk (pad with zeros if needed)
//   3. Generate content-addressed identifier for each block
//   4. Continue until all data is processed
//
// This method is more efficient than Split() for in-memory data as it avoids
// the overhead of io.Reader interface and can process data more directly.
//
// Parameters:
//   - data: Byte slice to split into blocks (must be non-empty)
//
// Returns:
//   - []*Block: Slice of blocks containing the split data with content-addressed IDs
//   - error: Non-nil if data is empty or block creation fails
//
// Call Flow:
//   - Called by: Client operations with in-memory data, testing code
//   - Calls: NewBlock for each created block
//
// Time Complexity: O(n) where n is the data size
// Space Complexity: O(b) where b is the number of blocks (data size / block size)
func (s *Splitter) SplitBytes(data []byte) ([]*Block, error) {
	if len(data) == 0 {
		return nil, errors.New("data cannot be empty")
	}

	var blocks []*Block

	// Process data in chunks of blockSize
	for i := 0; i < len(data); i += s.blockSize {
		end := i + s.blockSize
		if end > len(data) {
			end = len(data) // Don't exceed data length for final chunk
		}

		// Create fixed-size block with zero-padding for consistent block sizes
		blockData := make([]byte, s.blockSize) // Always allocate full block size
		copy(blockData, data[i:end])           // Copy actual data chunk
		// Remaining bytes are automatically zero-padded

		// Create block with content-addressed identifier
		block, err := NewBlock(blockData)
		if err != nil {
			return nil, err
		}

		blocks = append(blocks, block)
	}

	return blocks, nil
}

// DefaultSplitter creates a splitter with the standard NoiseFS block size.
// This convenience function provides a pre-configured splitter using DefaultBlockSize
// (128 KiB) which is the standard block size for the NoiseFS system.
//
// The default block size provides optimal balance between privacy protection,
// network efficiency, and storage overhead for most NoiseFS use cases.
//
// Returns:
//   - *Splitter: Splitter configured with DefaultBlockSize (128 KiB)
//
// Call Flow:
//   - Called by: Client operations requiring standard configuration, convenience functions
//   - Calls: NewSplitter with DefaultBlockSize parameter
//
// Time Complexity: O(1) - constant time initialization
// Space Complexity: O(1) - minimal memory allocation
func DefaultSplitter() *Splitter {
	splitter, _ := NewSplitter(DefaultBlockSize) // DefaultBlockSize is guaranteed valid
	return splitter
}

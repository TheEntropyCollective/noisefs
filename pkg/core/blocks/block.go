// Package blocks provides the core Block data structure and operations for NoiseFS.
// This file implements the fundamental building block of the NoiseFS anonymization system,
// supporting content-addressed storage, cryptographically secure randomizer generation,
// and 3-tuple XOR anonymization operations.
package blocks

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
)

const (
	// DefaultBlockSize defines the standard fixed block size used throughout NoiseFS (128 KiB).
	// This size provides optimal balance between network efficiency, anonymity set size,
	// and storage overhead. All files are split into blocks of this size, with smaller files
	// padded to maintain consistent block sizes for privacy protection.
	//
	// The 128 KiB size was chosen because:
	//   - Good performance for IPFS networking protocols
	//   - Creates large anonymity sets for privacy protection
	//   - Reasonable memory usage for streaming operations
	//   - Optimal balance for most file sizes in practice
	DefaultBlockSize = 128 * 1024
)

// Block represents a fundamental data unit in the NoiseFS anonymization system.
// Each block contains a content-addressed identifier (SHA-256 hash) and raw data bytes.
// Blocks can represent original file content, randomizer data for XOR operations,
// or anonymized data resulting from 3-tuple XOR operations.
//
// The Block type is immutable after creation - the ID is computed from the data
// and both fields should not be modified after construction to maintain integrity.
//
// Call Flow:
//   - Created by: NewBlock, NewRandomBlock, XOR operations
//   - Used by: Splitter, Assembler, Client upload/download operations
//   - Stored via: Storage backends through content-addressed identifiers
//
// Security Considerations:
//   - Content-addressed IDs provide integrity verification
//   - Constant-time comparison prevents timing attack vulnerabilities
//   - Cryptographically secure random generation for randomizer blocks
type Block struct {
	// ID is the content-addressed identifier computed as SHA-256 hash of Data
	ID string
	// Data contains the actual block content (original, randomizer, or anonymized)
	Data []byte
}

// NewBlock creates a new Block instance with content-addressed identifier generation.
// The block ID is computed as the SHA-256 hash of the provided data, ensuring
// content-addressable storage properties and data integrity verification.
//
// This function is used for creating blocks from original file content, anonymized
// data after XOR operations, or any other data that needs to be stored as a block.
//
// Parameters:
//   - data: Raw bytes to store in the block (must be non-empty)
//
// Returns:
//   - *Block: New block instance with computed content-addressed ID
//   - error: Non-nil if data is empty or block creation fails
//
// Call Flow:
//   - Called by: File splitters, XOR operations, randomizer creation
//   - Calls: generateBlockID for content-addressed identifier computation
//
// Time Complexity: O(n) where n is the length of data (for SHA-256 computation)
// Space Complexity: O(n) - stores copy of input data
func NewBlock(data []byte) (*Block, error) {
	if len(data) == 0 {
		return nil, errors.New("block data cannot be empty")
	}

	return &Block{
		ID:   generateBlockID(data),
		Data: data,
	}, nil
}

// NewRandomBlock creates a new Block filled with cryptographically secure random data.
// This function is primarily used to generate randomizer blocks for 3-tuple XOR
// anonymization operations. The random data is generated using crypto/rand for
// cryptographic security.
//
// Randomizer blocks are essential for NoiseFS privacy protection, providing the
// XOR masks that make original data appear as random noise during storage and transmission.
//
// Parameters:
//   - size: Number of random bytes to generate (must be positive)
//
// Returns:
//   - *Block: New block containing cryptographically secure random data
//   - error: Non-nil if size is invalid or random generation fails
//
// Call Flow:
//   - Called by: Client randomizer selection, randomizer cache population
//   - Calls: crypto/rand.Read, NewBlock
//
// Time Complexity: O(n) where n is the size parameter
// Space Complexity: O(n) - allocates and stores random data
func NewRandomBlock(size int) (*Block, error) {
	if size <= 0 {
		return nil, errors.New("block size must be positive")
	}

	// Generate cryptographically secure random data
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return nil, fmt.Errorf("failed to generate random data: %w", err)
	}

	return NewBlock(data)
}

// XOR performs 3-tuple XOR anonymization operation between this block and two randomizer blocks.
// This implements the core anonymization algorithm from OFFSystem research, where original data
// is XORed with two independent randomizer blocks to produce anonymized output.
//
// The operation: result = data ⊕ randomizer1 ⊕ randomizer2
// This ensures that:
//   - Anonymized data appears as random noise
//   - Original data can be recovered by XORing with the same randomizers
//   - Two randomizers are required for enhanced security over single-randomizer systems
//
// All three blocks must have identical sizes for the operation to succeed.
// The result is a new block containing the anonymized data with its own content-addressed ID.
//
// Parameters:
//   - randomizer1: First randomizer block for XOR operation
//   - randomizer2: Second randomizer block for XOR operation
//
// Returns:
//   - *Block: New block containing anonymized data
//   - error: Non-nil if block sizes don't match or block creation fails
//
// Call Flow:
//   - Called by: Client upload operations (anonymization), download operations (de-anonymization)
//   - Calls: NewBlock for result block creation
//
// Time Complexity: O(n) where n is the block size
// Space Complexity: O(n) - creates new block with XOR result
func (b *Block) XOR(randomizer1, randomizer2 *Block) (*Block, error) {
	if len(b.Data) != len(randomizer1.Data) {
		return nil, errors.New("data block and randomizer1 must have the same size")
	}
	if len(b.Data) != len(randomizer2.Data) {
		return nil, errors.New("data block and randomizer2 must have the same size")
	}

	// Perform byte-wise XOR operation across all three blocks
	result := make([]byte, len(b.Data))
	for i := range b.Data {
		// 3-tuple XOR: data ⊕ randomizer1 ⊕ randomizer2
		result[i] = b.Data[i] ^ randomizer1.Data[i] ^ randomizer2.Data[i]
	}

	return NewBlock(result)
}

// Size returns the size in bytes of the block's data content.
// This is a simple accessor method for determining block size for
// compatibility checking, storage calculations, and memory management.
//
// Returns:
//   - int: Number of bytes in the block's data
//
// Call Flow:
//   - Called by: Size validation logic, storage calculations, streaming operations
//   - Calls: None (simple accessor)
//
// Time Complexity: O(1)
// Space Complexity: O(1)
func (b *Block) Size() int {
	return len(b.Data)
}

// VerifyIntegrity checks if the block's ID correctly matches its data content hash.
// This method provides data integrity verification by recomputing the content-addressed
// identifier and comparing it with the stored ID using constant-time comparison
// to prevent timing attack vulnerabilities.
//
// The verification process:
//   1. Recompute SHA-256 hash of current data
//   2. Convert both stored and computed IDs to byte arrays
//   3. Perform constant-time comparison to prevent timing attacks
//
// This method should be called when loading blocks from storage or after any
// operation that might have corrupted the block data.
//
// Returns:
//   - bool: true if ID matches data hash, false if corrupted or invalid
//
// Call Flow:
//   - Called by: Storage retrieval operations, data validation workflows
//   - Calls: generateBlockID, hex.DecodeString, subtle.ConstantTimeCompare
//
// Time Complexity: O(n) where n is the data size (for SHA-256 computation)
// Space Complexity: O(1) - only temporary variables for comparison
func (b *Block) VerifyIntegrity() bool {
	expectedID := generateBlockID(b.Data)

	// Convert hex strings to byte slices for constant-time comparison
	expected, err := hex.DecodeString(expectedID)
	if err != nil {
		return false
	}

	actual, err := hex.DecodeString(b.ID)
	if err != nil {
		return false
	}

	// Ensure both hash representations have the same length
	if len(expected) != len(actual) {
		return false
	}

	// Use constant-time comparison to prevent timing attack vectors
	return subtle.ConstantTimeCompare(expected, actual) == 1
}

// generateBlockID computes a content-addressed identifier for block data using SHA-256.
// This function creates deterministic, collision-resistant identifiers that enable
// content-addressable storage, deduplication, and integrity verification.
//
// The SHA-256 hash provides:
//   - Deterministic addressing: Same content always produces same ID
//   - Collision resistance: Extremely unlikely for different content to have same ID
//   - Integrity verification: Changes in data result in different ID
//   - Fixed-length identifiers: Consistent 64-character hex strings
//
// Parameters:
//   - data: Raw bytes to compute hash for
//
// Returns:
//   - string: Hex-encoded SHA-256 hash of the input data
//
// Call Flow:
//   - Called by: NewBlock, VerifyIntegrity
//   - Calls: crypto/sha256.Sum256, hex.EncodeToString
//
// Time Complexity: O(n) where n is the length of data
// Space Complexity: O(1) - fixed-size hash output
func generateBlockID(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

package blocks

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
)

const (
	// DefaultBlockSize is the standard block size (128 KiB)
	DefaultBlockSize = 128 * 1024
)

// Block represents a data block in the NoiseFS system
type Block struct {
	ID   string
	Data []byte
}

// NewBlock creates a new block with the given data
func NewBlock(data []byte) (*Block, error) {
	if len(data) == 0 {
		return nil, errors.New("block data cannot be empty")
	}

	return &Block{
		ID:   generateBlockID(data),
		Data: data,
	}, nil
}

// NewBlockWithHMAC creates a new block with HMAC-based ID generation
// This provides additional security by requiring a secret key for ID generation
func NewBlockWithHMAC(data []byte, key []byte) (*Block, error) {
	if len(data) == 0 {
		return nil, errors.New("block data cannot be empty")
	}

	if len(key) == 0 {
		return nil, errors.New("HMAC key cannot be empty")
	}

	return &Block{
		ID:   generateBlockIDHMAC(data, key),
		Data: data,
	}, nil
}

// NewRandomBlock creates a new block filled with random data
func NewRandomBlock(size int) (*Block, error) {
	if size <= 0 {
		return nil, errors.New("block size must be positive")
	}

	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return nil, fmt.Errorf("failed to generate random data: %w", err)
	}

	return NewBlock(data)
}

// XOR performs XOR operation between three blocks (data XOR randomizer1 XOR randomizer2)
// This implements the 3-tuple anonymization used in OFFSystem for enhanced security
func (b *Block) XOR(randomizer1, randomizer2 *Block) (*Block, error) {
	if len(b.Data) != len(randomizer1.Data) {
		return nil, errors.New("data block and randomizer1 must have the same size")
	}
	if len(b.Data) != len(randomizer2.Data) {
		return nil, errors.New("data block and randomizer2 must have the same size")
	}

	result := make([]byte, len(b.Data))
	for i := range b.Data {
		result[i] = b.Data[i] ^ randomizer1.Data[i] ^ randomizer2.Data[i]
	}

	return NewBlock(result)
}

// Size returns the size of the block data
func (b *Block) Size() int {
	return len(b.Data)
}

// VerifyIntegrity checks if the block ID matches the content hash
// Uses constant-time comparison to prevent timing attacks
func (b *Block) VerifyIntegrity() bool {
	expectedID := generateBlockID(b.Data)

	// Convert strings to byte slices for constant-time comparison
	expected, err := hex.DecodeString(expectedID)
	if err != nil {
		return false
	}

	actual, err := hex.DecodeString(b.ID)
	if err != nil {
		return false
	}

	// Ensure both slices are the same length
	if len(expected) != len(actual) {
		return false
	}

	// Use constant-time comparison to prevent timing attacks
	return subtle.ConstantTimeCompare(expected, actual) == 1
}

// VerifyIntegrityHMAC verifies block integrity using HMAC for additional security
// The key parameter should be a secret key known only to authorized parties
func (b *Block) VerifyIntegrityHMAC(key []byte) bool {
	expectedID := generateBlockIDHMAC(b.Data, key)

	// Convert strings to byte slices for constant-time comparison
	expected, err := hex.DecodeString(expectedID)
	if err != nil {
		return false
	}

	actual, err := hex.DecodeString(b.ID)
	if err != nil {
		return false
	}

	// Use HMAC equal for constant-time comparison
	return hmac.Equal(expected, actual)
}

// generateBlockID generates a content-addressed identifier for a block
func generateBlockID(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// generateBlockIDHMAC generates an HMAC-based identifier for a block
// This provides additional security by requiring a secret key
func generateBlockIDHMAC(data []byte, key []byte) string {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

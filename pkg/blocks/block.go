package blocks

import (
	"crypto/rand"
	"crypto/sha256"
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

// XOR performs XOR operation between two blocks
func (b *Block) XOR(other *Block) (*Block, error) {
	if len(b.Data) != len(other.Data) {
		return nil, errors.New("blocks must have the same size for XOR operation")
	}
	
	result := make([]byte, len(b.Data))
	for i := range b.Data {
		result[i] = b.Data[i] ^ other.Data[i]
	}
	
	return NewBlock(result)
}

// XOR3 performs XOR operation between three blocks (data XOR randomizer1 XOR randomizer2)
// This implements the 3-tuple anonymization used in OFFSystem
func (b *Block) XOR3(randomizer1, randomizer2 *Block) (*Block, error) {
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
func (b *Block) VerifyIntegrity() bool {
	expectedID := generateBlockID(b.Data)
	return b.ID == expectedID
}

// generateBlockID generates a content-addressed identifier for a block
func generateBlockID(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
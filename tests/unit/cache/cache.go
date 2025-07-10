package cache

import (
	"errors"
	
	"github.com/TheEntropyCollective/noisefs/pkg/blocks"
)

// Cache defines the interface for block caching
type Cache interface {
	// Store adds a block to the cache with its CID
	Store(cid string, block *blocks.Block) error
	
	// Get retrieves a block from the cache by its CID
	Get(cid string) (*blocks.Block, error)
	
	// Has checks if a block exists in the cache
	Has(cid string) bool
	
	// Remove removes a block from the cache
	Remove(cid string) error
	
	// GetRandomizers returns a list of popular blocks suitable as randomizers
	GetRandomizers(count int) ([]*BlockInfo, error)
	
	// IncrementPopularity increases the popularity score of a block
	IncrementPopularity(cid string) error
	
	// Size returns the number of blocks in the cache
	Size() int
	
	// Clear removes all blocks from the cache
	Clear()
}

// BlockInfo contains block metadata for cache management
type BlockInfo struct {
	CID        string
	Block      *blocks.Block
	Size       int
	Popularity int
}

// ErrNotFound is returned when a block is not found in the cache
var ErrNotFound = errors.New("block not found in cache")
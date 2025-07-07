package noisefs

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	
	"github.com/TheEntropyCollective/noisefs/pkg/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/ipfs"
)

// Client provides high-level NoiseFS operations with caching
type Client struct {
	ipfsClient ipfs.BlockStore
	cache      cache.Cache
	metrics    *Metrics
}

// NewClient creates a new NoiseFS client
func NewClient(ipfsClient ipfs.BlockStore, blockCache cache.Cache) (*Client, error) {
	if ipfsClient == nil {
		return nil, errors.New("IPFS client is required")
	}
	
	if blockCache == nil {
		return nil, errors.New("cache is required")
	}
	
	return &Client{
		ipfsClient: ipfsClient,
		cache:      blockCache,
		metrics:    NewMetrics(),
	}, nil
}

// SelectRandomizer selects a randomizer block for the given block size
func (c *Client) SelectRandomizer(blockSize int) (*blocks.Block, string, error) {
	// Try to get popular blocks from cache first
	randomizers, err := c.cache.GetRandomizers(10)
	if err == nil && len(randomizers) > 0 {
		// Filter by matching size
		suitableBlocks := make([]*cache.BlockInfo, 0)
		for _, info := range randomizers {
			if info.Size == blockSize {
				suitableBlocks = append(suitableBlocks, info)
			}
		}
		
		// If we have suitable cached blocks, use one
		if len(suitableBlocks) > 0 {
			// Use cryptographically secure random selection
			index, err := rand.Int(rand.Reader, big.NewInt(int64(len(suitableBlocks))))
			if err != nil {
				return nil, "", fmt.Errorf("failed to generate random index: %w", err)
			}
			
			selected := suitableBlocks[index.Int64()]
			c.cache.IncrementPopularity(selected.CID)
			c.metrics.RecordBlockReuse()
			return selected.Block, selected.CID, nil
		}
	}
	
	// No suitable cached blocks, generate new randomizer
	randBlock, err := blocks.NewRandomBlock(blockSize)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create randomizer: %w", err)
	}
	
	// Store in IPFS
	cid, err := c.ipfsClient.StoreBlock(randBlock)
	if err != nil {
		return nil, "", fmt.Errorf("failed to store randomizer: %w", err)
	}
	
	// Cache the new randomizer
	c.cache.Store(cid, randBlock)
	c.metrics.RecordBlockGeneration()
	
	return randBlock, cid, nil
}

// StoreBlockWithCache stores a block in IPFS and caches it
func (c *Client) StoreBlockWithCache(block *blocks.Block) (string, error) {
	// Store in IPFS
	cid, err := c.ipfsClient.StoreBlock(block)
	if err != nil {
		return "", err
	}
	
	// Cache the block
	c.cache.Store(cid, block)
	c.cache.IncrementPopularity(cid)
	
	return cid, nil
}

// RetrieveBlockWithCache retrieves a block, checking cache first
func (c *Client) RetrieveBlockWithCache(cid string) (*blocks.Block, error) {
	// Check cache first
	if block, err := c.cache.Get(cid); err == nil {
		c.cache.IncrementPopularity(cid)
		c.metrics.RecordCacheHit()
		return block, nil
	}
	
	// Not in cache, retrieve from IPFS
	c.metrics.RecordCacheMiss()
	block, err := c.ipfsClient.RetrieveBlock(cid)
	if err != nil {
		return nil, err
	}
	
	// Cache for future use
	c.cache.Store(cid, block)
	
	return block, nil
}

// GetMetrics returns current metrics
func (c *Client) GetMetrics() MetricsSnapshot {
	return c.metrics.GetStats()
}

// RecordUpload records upload metrics
func (c *Client) RecordUpload(originalBytes, storedBytes int64) {
	c.metrics.RecordUpload(originalBytes, storedBytes)
}

// RecordDownload records download metrics
func (c *Client) RecordDownload() {
	c.metrics.RecordDownload()
}
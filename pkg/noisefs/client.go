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

// SelectRandomizer selects a randomizer block for the given block size (legacy 2-tuple support)
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

// SelectTwoRandomizers selects two randomizer blocks for 3-tuple anonymization
func (c *Client) SelectTwoRandomizers(blockSize int) (*blocks.Block, string, *blocks.Block, string, error) {
	// Try to get popular blocks from cache first
	randomizers, err := c.cache.GetRandomizers(20) // Get more blocks for better selection
	if err == nil && len(randomizers) > 0 {
		// Filter by matching size
		suitableBlocks := make([]*cache.BlockInfo, 0)
		for _, info := range randomizers {
			if info.Size == blockSize {
				suitableBlocks = append(suitableBlocks, info)
			}
		}
		
		// If we have at least 2 suitable cached blocks, use them
		if len(suitableBlocks) >= 2 {
			// Select first randomizer
			index1, err := rand.Int(rand.Reader, big.NewInt(int64(len(suitableBlocks))))
			if err != nil {
				return nil, "", nil, "", fmt.Errorf("failed to generate random index for first randomizer: %w", err)
			}
			
			selected1 := suitableBlocks[index1.Int64()]
			
			// Remove selected block from pool and select second randomizer
			remainingBlocks := make([]*cache.BlockInfo, 0, len(suitableBlocks)-1)
			for i, block := range suitableBlocks {
				if i != int(index1.Int64()) {
					remainingBlocks = append(remainingBlocks, block)
				}
			}
			
			index2, err := rand.Int(rand.Reader, big.NewInt(int64(len(remainingBlocks))))
			if err != nil {
				return nil, "", nil, "", fmt.Errorf("failed to generate random index for second randomizer: %w", err)
			}
			
			selected2 := remainingBlocks[index2.Int64()]
			
			// Update popularity and metrics
			c.cache.IncrementPopularity(selected1.CID)
			c.cache.IncrementPopularity(selected2.CID)
			c.metrics.RecordBlockReuse()
			c.metrics.RecordBlockReuse()
			
			return selected1.Block, selected1.CID, selected2.Block, selected2.CID, nil
		}
		
		// If we have exactly 1 suitable cached block, use it and generate another
		if len(suitableBlocks) == 1 {
			selected1 := suitableBlocks[0]
			c.cache.IncrementPopularity(selected1.CID)
			c.metrics.RecordBlockReuse()
			
			// Generate second randomizer
			randBlock2, err := blocks.NewRandomBlock(blockSize)
			if err != nil {
				return nil, "", nil, "", fmt.Errorf("failed to create second randomizer: %w", err)
			}
			
			cid2, err := c.ipfsClient.StoreBlock(randBlock2)
			if err != nil {
				return nil, "", nil, "", fmt.Errorf("failed to store second randomizer: %w", err)
			}
			
			c.cache.Store(cid2, randBlock2)
			c.metrics.RecordBlockGeneration()
			
			return selected1.Block, selected1.CID, randBlock2, cid2, nil
		}
	}
	
	// No suitable cached blocks or insufficient blocks, generate both randomizers
	// Ensure they're different by generating different random data
	randBlock1, err := blocks.NewRandomBlock(blockSize)
	if err != nil {
		return nil, "", nil, "", fmt.Errorf("failed to create first randomizer: %w", err)
	}
	
	// Generate second randomizer, retry if identical to first (extremely unlikely but possible)
	var randBlock2 *blocks.Block
	for attempts := 0; attempts < 10; attempts++ {
		randBlock2, err = blocks.NewRandomBlock(blockSize)
		if err != nil {
			return nil, "", nil, "", fmt.Errorf("failed to create second randomizer: %w", err)
		}
		
		// Check if blocks are different (compare IDs which are content hashes)
		if randBlock1.ID != randBlock2.ID {
			break
		}
		
		// If we reach max attempts, this is extremely unlikely with crypto random
		if attempts == 9 {
			return nil, "", nil, "", fmt.Errorf("failed to generate different randomizer blocks after 10 attempts")
		}
	}
	
	// Store both in IPFS
	cid1, err := c.ipfsClient.StoreBlock(randBlock1)
	if err != nil {
		return nil, "", nil, "", fmt.Errorf("failed to store first randomizer: %w", err)
	}
	
	cid2, err := c.ipfsClient.StoreBlock(randBlock2)
	if err != nil {
		return nil, "", nil, "", fmt.Errorf("failed to store second randomizer: %w", err)
	}
	
	// Ensure CIDs are different (they should be since block content is different)
	if cid1 == cid2 {
		return nil, "", nil, "", fmt.Errorf("generated randomizers have identical CIDs")
	}
	
	// Cache both randomizers
	c.cache.Store(cid1, randBlock1)
	c.cache.Store(cid2, randBlock2)
	c.metrics.RecordBlockGeneration()
	c.metrics.RecordBlockGeneration()
	
	return randBlock1, cid1, randBlock2, cid2, nil
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
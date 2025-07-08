package noisefs

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"
	
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/TheEntropyCollective/noisefs/pkg/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/ipfs"
	"github.com/TheEntropyCollective/noisefs/pkg/p2p"
)

// Client provides high-level NoiseFS operations with caching and peer selection
type Client struct {
	ipfsClient    ipfs.PeerAwareIPFSClient
	cache         cache.Cache
	adaptiveCache *cache.AdaptiveCache
	peerManager   *p2p.PeerManager
	metrics       *Metrics
	
	// Configuration for intelligent operations
	preferRandomizerPeers bool
	adaptiveCacheEnabled  bool
}

// ClientConfig holds configuration for NoiseFS client
type ClientConfig struct {
	EnableAdaptiveCache   bool
	PreferRandomizerPeers bool
	AdaptiveCacheConfig   *cache.AdaptiveCacheConfig
}

// NewClient creates a new NoiseFS client
func NewClient(ipfsClient ipfs.BlockStore, blockCache cache.Cache) (*Client, error) {
	config := &ClientConfig{
		EnableAdaptiveCache:   true,
		PreferRandomizerPeers: true,
		AdaptiveCacheConfig: &cache.AdaptiveCacheConfig{
			MaxSize:            100 * 1024 * 1024, // 100MB
			MaxItems:           10000,
			HotTierRatio:       0.1,  // 10% hot tier
			WarmTierRatio:      0.3,  // 30% warm tier
			PredictionWindow:   time.Hour * 24,
			EvictionBatchSize:  10,
			ExchangeInterval:   time.Minute * 15,
			PredictionInterval: time.Minute * 10,
		},
	}
	
	return NewClientWithConfig(ipfsClient, blockCache, config)
}

// NewClientWithConfig creates a new NoiseFS client with custom configuration
func NewClientWithConfig(ipfsClient ipfs.BlockStore, blockCache cache.Cache, config *ClientConfig) (*Client, error) {
	if ipfsClient == nil {
		return nil, errors.New("IPFS client is required")
	}
	
	if blockCache == nil {
		return nil, errors.New("cache is required")
	}
	
	// Try to cast to peer-aware IPFS client
	peerAwareClient, isPeerAware := ipfsClient.(ipfs.PeerAwareIPFSClient)
	if !isPeerAware {
		return nil, errors.New("IPFS client must implement PeerAwareIPFSClient interface")
	}
	
	client := &Client{
		ipfsClient:            peerAwareClient,
		cache:                 blockCache,
		metrics:               NewMetrics(),
		preferRandomizerPeers: config.PreferRandomizerPeers,
		adaptiveCacheEnabled:  config.EnableAdaptiveCache,
	}
	
	// Initialize adaptive cache if enabled
	if config.EnableAdaptiveCache && config.AdaptiveCacheConfig != nil {
		client.adaptiveCache = cache.NewAdaptiveCache(config.AdaptiveCacheConfig)
	}
	
	return client, nil
}

// SetPeerManager sets the peer manager for intelligent peer selection
func (c *Client) SetPeerManager(manager *p2p.PeerManager) {
	c.peerManager = manager
	c.ipfsClient.SetPeerManager(manager)
}

// SelectRandomizer selects a randomizer block for the given block size (legacy 2-tuple support)
func (c *Client) SelectRandomizer(blockSize int) (*blocks.Block, string, error) {
	// If we have peer selection enabled, use intelligent randomizer selection
	if c.preferRandomizerPeers && c.peerManager != nil {
		return c.selectRandomizerWithPeerSelection(blockSize)
	}
	
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
	
	// Store in IPFS with randomizer strategy if available
	cid, err := c.storeBlockWithStrategy(randBlock, "randomizer")
	if err != nil {
		return nil, "", fmt.Errorf("failed to store randomizer: %w", err)
	}
	
	// Cache the new randomizer
	c.cacheBlock(cid, randBlock, map[string]interface{}{
		"is_randomizer": true,
		"block_type":    "randomizer",
	})
	c.metrics.RecordBlockGeneration()
	
	return randBlock, cid, nil
}

// selectRandomizerWithPeerSelection uses peer selection to find optimal randomizer blocks
func (c *Client) selectRandomizerWithPeerSelection(blockSize int) (*blocks.Block, string, error) {
	ctx := context.Background()
	
	// Get peers with randomizer blocks
	criteria := p2p.SelectionCriteria{
		Count:             5,
		PreferRandomizers: true,
	}
	peers, err := c.peerManager.SelectPeers(ctx, "randomizer", criteria)
	if err != nil || len(peers) == 0 {
		// Fall back to standard selection if no suitable peers
		return c.selectStandardRandomizer(blockSize)
	}
	
	// Try to get randomizer blocks from selected peers
	for _, peerID := range peers {
		// This would require a protocol to query peer for available randomizers
		// For now, we'll use the peer as a hint for block retrieval
		randomizers, err := c.cache.GetRandomizers(10)
		if err == nil && len(randomizers) > 0 {
			for _, info := range randomizers {
				if info.Size == blockSize {
					// Try to retrieve this block with peer hint
					if block, err := c.ipfsClient.RetrieveBlockWithPeerHint(info.CID, []peer.ID{peerID}); err == nil {
						c.cache.IncrementPopularity(info.CID)
						c.metrics.RecordBlockReuse()
						return block, info.CID, nil
					}
				}
			}
		}
	}
	
	// If peer-based selection fails, fall back to standard method
	return c.selectStandardRandomizer(blockSize)
}

// selectStandardRandomizer implements the original randomizer selection logic
func (c *Client) selectStandardRandomizer(blockSize int) (*blocks.Block, string, error) {
	// Generate new randomizer
	randBlock, err := blocks.NewRandomBlock(blockSize)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create randomizer: %w", err)
	}
	
	// Store in IPFS
	cid, err := c.storeBlockWithStrategy(randBlock, "randomizer")
	if err != nil {
		return nil, "", fmt.Errorf("failed to store randomizer: %w", err)
	}
	
	// Cache the new randomizer
	c.cacheBlock(cid, randBlock, map[string]interface{}{
		"is_randomizer": true,
		"block_type":    "randomizer",
	})
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
	return c.storeBlockWithStrategy(block, "performance")
}

// storeBlockWithStrategy stores a block using the specified peer selection strategy
func (c *Client) storeBlockWithStrategy(block *blocks.Block, strategy string) (string, error) {
	// Try to use strategy-aware storage if available
	if strategyStore, ok := c.ipfsClient.(interface {
		StoreBlockWithStrategy(block *blocks.Block, strategy string) (string, error)
	}); ok {
		cid, err := strategyStore.StoreBlockWithStrategy(block, strategy)
		if err != nil {
			return "", err
		}
		
		// Cache the block with metadata
		metadata := map[string]interface{}{
			"block_type": "data",
			"strategy":   strategy,
		}
		if strategy == "randomizer" {
			metadata["is_randomizer"] = true
		}
		
		c.cacheBlock(cid, block, metadata)
		return cid, nil
	}
	
	// Fallback to standard storage
	cid, err := c.ipfsClient.StoreBlock(block)
	if err != nil {
		return "", err
	}
	
	// Cache the block
	c.cache.Store(cid, block)
	c.cache.IncrementPopularity(cid)
	
	return cid, nil
}

// cacheBlock stores a block in both standard and adaptive caches with metadata
func (c *Client) cacheBlock(cid string, block *blocks.Block, metadata map[string]interface{}) {
	// Store in standard cache
	c.cache.Store(cid, block)
	c.cache.IncrementPopularity(cid)
	
	// Store in adaptive cache if enabled
	if c.adaptiveCacheEnabled && c.adaptiveCache != nil {
		c.adaptiveCache.Put(cid, block.Data, metadata)
	}
}

// RetrieveBlockWithCache retrieves a block, checking cache first
func (c *Client) RetrieveBlockWithCache(cid string) (*blocks.Block, error) {
	return c.RetrieveBlockWithCacheAndPeerHint(cid, nil)
}

// RetrieveBlockWithCacheAndPeerHint retrieves a block with cache and peer hints
func (c *Client) RetrieveBlockWithCacheAndPeerHint(cid string, preferredPeers []peer.ID) (*blocks.Block, error) {
	// Check adaptive cache first if enabled
	if c.adaptiveCacheEnabled && c.adaptiveCache != nil {
		if data, found := c.adaptiveCache.Get(cid); found {
			block, err := blocks.NewBlock(data)
			if err == nil {
				c.metrics.RecordCacheHit()
				return block, nil
			}
		}
	}
	
	// Check standard cache
	if block, err := c.cache.Get(cid); err == nil {
		c.cache.IncrementPopularity(cid)
		c.metrics.RecordCacheHit()
		
		// Update adaptive cache with access
		if c.adaptiveCacheEnabled && c.adaptiveCache != nil {
			metadata := map[string]interface{}{
				"block_type": "data",
				"accessed_from_cache": true,
			}
			c.adaptiveCache.Put(cid, block.Data, metadata)
		}
		
		return block, nil
	}
	
	// Not in cache, retrieve from IPFS with peer hints
	c.metrics.RecordCacheMiss()
	
	var block *blocks.Block
	var err error
	
	// Use peer-aware retrieval if available
	if peerAware, ok := c.ipfsClient.(interface {
		RetrieveBlockWithPeerHint(cid string, preferredPeers []peer.ID) (*blocks.Block, error)
	}); ok {
		block, err = peerAware.RetrieveBlockWithPeerHint(cid, preferredPeers)
	} else {
		block, err = c.ipfsClient.RetrieveBlock(cid)
	}
	
	if err != nil {
		return nil, err
	}
	
	// Cache for future use with metadata
	metadata := map[string]interface{}{
		"block_type": "data",
		"retrieved_from_network": true,
	}
	c.cacheBlock(cid, block, metadata)
	
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

// GetAdaptiveCacheStats returns adaptive cache statistics if enabled
func (c *Client) GetAdaptiveCacheStats() *cache.AdaptiveCacheStats {
	if c.adaptiveCacheEnabled && c.adaptiveCache != nil {
		return c.adaptiveCache.GetStats()
	}
	return nil
}

// GetPeerStats returns peer performance statistics if available
func (c *Client) GetPeerStats() map[peer.ID]*ipfs.RequestMetrics {
	if metricsProvider, ok := c.ipfsClient.(interface {
		GetPeerMetrics() map[peer.ID]*ipfs.RequestMetrics
	}); ok {
		return metricsProvider.GetPeerMetrics()
	}
	return nil
}

// PreloadBlocks preloads blocks based on ML predictions
func (c *Client) PreloadBlocks(ctx context.Context) error {
	if !c.adaptiveCacheEnabled || c.adaptiveCache == nil {
		return nil // Adaptive cache not enabled
	}
	
	// Define block fetcher for preloading
	blockFetcher := func(cid string) ([]byte, error) {
		block, err := c.ipfsClient.RetrieveBlock(cid)
		if err != nil {
			return nil, err
		}
		return block.Data, nil
	}
	
	return c.adaptiveCache.Preload(ctx, blockFetcher)
}

// OptimizeForRandomizers adjusts cache and peer selection for randomizer optimization
func (c *Client) OptimizeForRandomizers() {
	c.preferRandomizerPeers = true
	
	// Switch to randomizer-aware eviction policy if adaptive cache is enabled
	if c.adaptiveCacheEnabled && c.adaptiveCache != nil {
		// TODO: Implement NewRandomizerAwareEvictionPolicy
		// For now, keep the existing policy
	}
}

// SetPeerSelectionStrategy sets the default peer selection strategy
func (c *Client) SetPeerSelectionStrategy(strategy string) {
	if c.peerManager != nil {
		// TODO: Implement SetDefaultStrategy method on PeerManager
		// For now, this is a no-op
	}
}

// GetConnectedPeers returns currently connected peers
func (c *Client) GetConnectedPeers() []peer.ID {
	return c.ipfsClient.GetConnectedPeers()
}
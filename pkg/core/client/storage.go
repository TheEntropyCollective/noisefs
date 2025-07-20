// Package noisefs provides storage management and caching functionality.
// This file handles block storage operations, cache integration, and peer-aware
// storage strategies for the NoiseFS distributed storage system.
package noisefs

import (
	"context"
	"fmt"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	"github.com/libp2p/go-libp2p/core/peer"
)

// StoreBlockWithCache stores a block in IPFS and caches it
func (c *Client) StoreBlockWithCache(block *blocks.Block) (string, error) {
	return c.storeBlockWithStrategy(block, "performance")
}

// storeBlockWithStrategy stores a block using the specified peer selection strategy
func (c *Client) storeBlockWithStrategy(block *blocks.Block, strategy string) (string, error) {
	ctx := context.Background() // TODO: Accept context parameter in future version

	// Use storage manager (strategy is handled at backend level)
	cid, err := c.storeBlock(ctx, block)
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

// cacheBlock stores a block in both standard and adaptive caches with metadata
func (c *Client) cacheBlock(cid string, block *blocks.Block, metadata map[string]interface{}) {
	// Determine if this is a personal block (requested by user)
	// or an altruistic block (for network benefit)
	isPersonal := true // Default to personal

	// Check metadata for explicit origin
	if origin, ok := metadata["requested_by_user"]; ok {
		isPersonal = origin.(bool)
	} else if blockType, ok := metadata["block_type"]; ok {
		// Randomizers and other system blocks are not personal
		switch blockType {
		case "randomizer", "public_domain":
			isPersonal = false
		}
	}

	// Store in cache with origin info
	if altruisticCache, ok := c.cache.(*cache.AltruisticCache); ok {
		// Use altruistic cache with explicit origin
		if isPersonal {
			altruisticCache.StoreWithOrigin(cid, block, cache.PersonalBlock)
		} else {
			altruisticCache.StoreWithOrigin(cid, block, cache.AltruisticBlock)
		}
	} else {
		// Fallback to standard cache
		c.cache.Store(cid, block)
	}

	c.cache.IncrementPopularity(cid)

	// Store in adaptive cache if enabled
	if c.adaptiveCacheEnabled && c.adaptiveCache != nil {
		c.adaptiveCache.Store(cid, block)
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
		if block, err := c.adaptiveCache.Get(cid); err == nil {
			c.metrics.RecordCacheHit()
			return block, nil
		}
	}

	// Check standard cache
	if block, err := c.cache.Get(cid); err == nil {
		c.cache.IncrementPopularity(cid)
		c.metrics.RecordCacheHit()

		// Update adaptive cache with access
		if c.adaptiveCacheEnabled && c.adaptiveCache != nil {
			c.adaptiveCache.Store(cid, block)
		}

		return block, nil
	}

	// Not in cache, retrieve from IPFS with peer hints
	c.metrics.RecordCacheMiss()

	var block *blocks.Block
	var err error

	// Use storage manager for retrieval
	// TODO: Implement peer hints in storage manager
	_ = preferredPeers // TODO: Use preferredPeers for peer-aware retrieval
	block, err = c.retrieveBlock(context.Background(), cid)

	if err != nil {
		return nil, err
	}

	// Cache for future use with metadata
	metadata := map[string]interface{}{
		"block_type":             "data",
		"retrieved_from_network": true,
	}
	c.cacheBlock(cid, block, metadata)

	return block, nil
}

// storeBlock stores a block using the storage manager
func (c *Client) storeBlock(ctx context.Context, block *blocks.Block) (string, error) {
	address, err := c.storageManager.Put(ctx, block)
	if err != nil {
		return "", err
	}
	return address.ID, nil
}

// storeBlockWithTracking stores a block and returns bytes stored for metrics
func (c *Client) storeBlockWithTracking(ctx context.Context, block *blocks.Block) (string, int64, error) {
	address, err := c.storageManager.Put(ctx, block)
	if err != nil {
		return "", 0, err
	}

	// For simplicity, assume block size equals bytes stored
	// In a real implementation, this might differ due to compression, overhead, etc.
	bytesStored := int64(block.Size())

	return address.ID, bytesStored, nil
}

// retrieveBlock retrieves a block using the storage manager
func (c *Client) retrieveBlock(ctx context.Context, cid string) (*blocks.Block, error) {
	address := &storage.BlockAddress{
		ID:          cid,
		BackendType: "", // Let router determine
	}
	return c.storageManager.Get(ctx, address)
}

// PreloadBlocks preloads blocks based on ML predictions
func (c *Client) PreloadBlocks(ctx context.Context) error {
	if !c.adaptiveCacheEnabled || c.adaptiveCache == nil {
		return nil // Adaptive cache not enabled
	}

	// Define block fetcher for preloading
	blockFetcher := func(cid string) ([]byte, error) {
		block, err := c.retrieveBlock(ctx, cid)
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
		randomizerPolicy := cache.NewRandomizerAwareEvictionPolicy()
		c.adaptiveCache.SetEvictionPolicy(randomizerPolicy)
	}
}

// SetPeerSelectionStrategy sets the default peer selection strategy
func (c *Client) SetPeerSelectionStrategy(strategy string) error {
	if c.peerManager != nil {
		return c.peerManager.SetDefaultStrategy(strategy)
	}
	return fmt.Errorf("peer manager not initialized")
}

// GetConnectedPeers returns currently connected peers
func (c *Client) GetConnectedPeers() []peer.ID {
	// TODO: Implement through storage manager backend
	// This requires extending the storage interface to support peer information
	return nil
}

// GetPeerStats returns peer performance statistics if available
func (c *Client) GetPeerStats() map[peer.ID]interface{} {
	// TODO: Implement peer stats through storage manager
	// This requires extending the storage interface to support peer metrics
	return nil
}

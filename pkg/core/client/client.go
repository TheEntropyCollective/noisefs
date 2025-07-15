package noisefs

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
	"time"
	
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/privacy/p2p"
)

// Client provides high-level NoiseFS operations with caching and peer selection
type Client struct {
	// Storage abstraction
	storageManager *storage.Manager
	
	// Common components
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

// NewClient creates a new NoiseFS client using storage manager
func NewClient(storageManager *storage.Manager, blockCache cache.Cache) (*Client, error) {
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
	
	return NewClientWithConfig(storageManager, blockCache, config)
}

// NewClientWithStorageManager creates a NoiseFS client using the storage abstraction layer
// This is kept for backward compatibility and delegates to NewClient
func NewClientWithStorageManager(storageManager *storage.Manager, blockCache cache.Cache) (*Client, error) {
	return NewClient(storageManager, blockCache)
}

// NewClientWithStorageManagerAndConfig creates a NoiseFS client with storage manager and custom config
// This is kept for backward compatibility and delegates to NewClientWithConfig
func NewClientWithStorageManagerAndConfig(storageManager *storage.Manager, blockCache cache.Cache, config *ClientConfig) (*Client, error) {
	return NewClientWithConfig(storageManager, blockCache, config)
}

// NewClientWithConfig creates a new NoiseFS client with custom configuration
func NewClientWithConfig(storageManager *storage.Manager, blockCache cache.Cache, config *ClientConfig) (*Client, error) {
	if storageManager == nil {
		return nil, errors.New("storage manager is required")
	}
	
	if blockCache == nil {
		return nil, errors.New("cache is required")
	}
	
	client := &Client{
		storageManager:        storageManager,
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

// NewClientWithDefaultStorageManager creates a client with default storage configuration
// This is a convenience function for common use cases
func NewClientWithDefaultStorageManager(blockCache cache.Cache) (*Client, error) {
	// Create default storage configuration
	config := storage.DefaultConfig()
	
	// Create storage manager
	storageManager, err := storage.NewManager(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage manager: %w", err)
	}
	
	// Start storage manager
	ctx := context.Background()
	if err := storageManager.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start storage manager: %w", err)
	}
	
	// Create client with storage manager
	return NewClientWithStorageManager(storageManager, blockCache)
}

// Storage abstraction methods

// storeBlock stores a block using the storage manager
func (c *Client) storeBlock(ctx context.Context, block *blocks.Block) (string, error) {
	// Use storage manager
	address, err := c.storageManager.Put(ctx, block)
	if err != nil {
		return "", fmt.Errorf("storage manager put failed: %w", err)
	}
	return address.ID, nil
}

// retrieveBlock retrieves a block using the storage manager
func (c *Client) retrieveBlock(ctx context.Context, cid string) (*blocks.Block, error) {
	// Use storage manager
	address := &storage.BlockAddress{ID: cid}
	return c.storageManager.Get(ctx, address)
}

// hasBlock checks if a block exists using the storage manager
func (c *Client) hasBlock(ctx context.Context, cid string) (bool, error) {
	// Use storage manager
	address := &storage.BlockAddress{ID: cid}
	return c.storageManager.Has(ctx, address)
}

// SetPeerManager sets the peer manager for intelligent peer selection
func (c *Client) SetPeerManager(manager *p2p.PeerManager) {
	c.peerManager = manager
	
	// For storage manager mode, peer management is handled at the backend level
	// The storage manager will propagate this to peer-aware backends
	if ipfsBackend, ok := c.storageManager.GetBackend("ipfs"); ok {
		if peerAware, ok := ipfsBackend.(storage.PeerAwareBackend); ok {
			// TODO: Add SetPeerManager method to PeerAwareBackend interface
			_ = peerAware // For now, just acknowledge the interface
		}
	}
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
		_ = peerID // TODO: Use peerID for peer-aware retrieval in storage manager
		randomizers, err := c.cache.GetRandomizers(10)
		if err == nil && len(randomizers) > 0 {
			for _, info := range randomizers {
				if info.Size == blockSize {
					// Try to retrieve this block with peer hint
					if block, err := c.retrieveBlock(context.Background(), info.CID); err == nil {
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

// SelectRandomizers selects two randomizer blocks for 3-tuple anonymization
func (c *Client) SelectRandomizers(blockSize int) (*blocks.Block, string, *blocks.Block, string, error) {
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
			
			cid2, err := c.storeBlock(context.Background(), randBlock2)
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
	
	// Store both randomizers using storage abstraction
	ctx := context.Background() // TODO: Accept context parameter in future version
	cid1, err := c.storeBlock(ctx, randBlock1)
	if err != nil {
		return nil, "", nil, "", fmt.Errorf("failed to store first randomizer: %w", err)
	}
	
	cid2, err := c.storeBlock(ctx, randBlock2)
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
	isPersonal := true // Default to personal for backward compatibility
	
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
func (c *Client) GetAdaptiveCacheStats() *cache.AdaptiveCacheStatsSnapshot {
	if c.adaptiveCacheEnabled && c.adaptiveCache != nil {
		return c.adaptiveCache.GetAdaptiveStats()
	}
	return nil
}

// GetAltruisticCacheStats returns altruistic cache statistics if available
func (c *Client) GetAltruisticCacheStats() *cache.AltruisticStats {
	// Check if the cache is an altruistic cache
	if altruisticCache, ok := c.cache.(*cache.AltruisticCache); ok {
		return altruisticCache.GetAltruisticStats()
	}
	return nil
}

// IsAltruisticCacheEnabled returns whether altruistic caching is enabled
func (c *Client) IsAltruisticCacheEnabled() bool {
	if altruisticCache, ok := c.cache.(*cache.AltruisticCache); ok {
		return altruisticCache.GetConfig().EnableAltruistic
	}
	return false
}

// GetCacheConfig returns the cache configuration
func (c *Client) GetCacheConfig() *cache.AltruisticCacheConfig {
	if altruisticCache, ok := c.cache.(*cache.AltruisticCache); ok {
		return altruisticCache.GetConfig()
	}
	return nil
}

// GetPeerStats returns peer performance statistics if available
func (c *Client) GetPeerStats() map[peer.ID]interface{} {
	// TODO: Implement peer stats through storage manager
	// This requires extending the storage interface to support peer metrics
	return nil
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

// Upload uploads a file and returns descriptor CID
// This is a simplified implementation for testing
// ProgressCallback is called during operations to report progress
type ProgressCallback func(stage string, current, total int)

// Upload uploads a file to NoiseFS with full protocol implementation
func (c *Client) Upload(reader io.Reader, filename string) (string, error) {
	return c.UploadWithBlockSize(reader, filename, blocks.DefaultBlockSize)
}

// UploadWithProgress uploads a file with progress reporting
func (c *Client) UploadWithProgress(reader io.Reader, filename string, progress ProgressCallback) (string, error) {
	return c.UploadWithBlockSizeAndProgress(reader, filename, blocks.DefaultBlockSize, progress)
}

// UploadWithBlockSize uploads a file with a specific block size
func (c *Client) UploadWithBlockSize(reader io.Reader, filename string, blockSize int) (string, error) {
	return c.UploadWithBlockSizeAndProgress(reader, filename, blockSize, nil)
}

// UploadWithBlockSizeAndProgress uploads a file with a specific block size and progress reporting
func (c *Client) UploadWithBlockSizeAndProgress(reader io.Reader, filename string, blockSize int, progress ProgressCallback) (string, error) {
	// Read all data to get size
	if progress != nil {
		progress("Reading file", 0, 100)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("failed to read data: %w", err)
	}
	
	fileSize := int64(len(data))
	if progress != nil {
		progress("Reading file", 100, 100)
	}
	
	// Create splitter
	splitter, err := blocks.NewSplitter(blockSize)
	if err != nil {
		return "", fmt.Errorf("failed to create splitter: %w", err)
	}
	
	// Split file into blocks
	if progress != nil {
		progress("Splitting file into blocks", 0, 100)
	}
	fileBlocks, err := splitter.Split(strings.NewReader(string(data)))
	if err != nil {
		return "", fmt.Errorf("failed to split file: %w", err)
	}
	if progress != nil {
		progress("Splitting file into blocks", 100, 100)
	}
	
	// Create descriptor
	descriptor := descriptors.NewDescriptor(filename, fileSize, blockSize)
	
	// Process each block with XOR
	totalBlocks := len(fileBlocks)
	for i, fileBlock := range fileBlocks {
		if progress != nil {
			progress("Anonymizing blocks", i, totalBlocks)
		}
		// Select two randomizer blocks (3-tuple XOR)
		randBlock1, cid1, randBlock2, cid2, err := c.SelectRandomizers(fileBlock.Size())
		if err != nil {
			return "", fmt.Errorf("failed to select randomizers: %w", err)
		}
		
		// XOR the blocks (3-tuple: data XOR randomizer1 XOR randomizer2)
		xorBlock, err := fileBlock.XOR(randBlock1, randBlock2)
		if err != nil {
			return "", fmt.Errorf("failed to XOR blocks: %w", err)
		}
		
		// Store anonymized block
		dataCID, err := c.StoreBlockWithCache(xorBlock)
		if err != nil {
			return "", fmt.Errorf("failed to store data block: %w", err)
		}
		
		// Add block triple to descriptor
		if err := descriptor.AddBlockTriple(dataCID, cid1, cid2); err != nil {
			return "", fmt.Errorf("failed to add block triple: %w", err)
		}
	}
	
	if progress != nil {
		progress("Anonymizing blocks", totalBlocks, totalBlocks)
	}
	
	// Store descriptor in IPFS
	if progress != nil {
		progress("Saving file descriptor", 0, 100)
	}
	
	// Create descriptor store with storage manager
	descriptorStore, err := descriptors.NewStoreWithManager(c.storageManager)
	if err != nil {
		return "", fmt.Errorf("failed to create descriptor store: %w", err)
	}
	
	descriptorCID, err := descriptorStore.Save(descriptor)
	if err != nil {
		return "", fmt.Errorf("failed to save descriptor: %w", err)
	}
	
	if progress != nil {
		progress("Saving file descriptor", 100, 100)
	}
	
	// Record metrics
	c.RecordUpload(fileSize, fileSize*3) // *3 for data + 2 randomizer blocks
	
	return descriptorCID, nil
}

// Download downloads a file by descriptor CID and returns data
func (c *Client) Download(descriptorCID string) ([]byte, error) {
	data, _, err := c.DownloadWithMetadata(descriptorCID)
	return data, err
}

// DownloadWithProgress downloads a file with progress reporting
func (c *Client) DownloadWithProgress(descriptorCID string, progress ProgressCallback) ([]byte, error) {
	data, _, err := c.DownloadWithMetadataAndProgress(descriptorCID, progress)
	return data, err
}

// DownloadWithMetadata downloads a file and returns both data and metadata
func (c *Client) DownloadWithMetadata(descriptorCID string) ([]byte, string, error) {
	return c.DownloadWithMetadataAndProgress(descriptorCID, nil)
}

// DownloadWithMetadataAndProgress downloads a file with progress reporting
func (c *Client) DownloadWithMetadataAndProgress(descriptorCID string, progress ProgressCallback) ([]byte, string, error) {
	if progress != nil {
		progress("Loading file descriptor", 0, 100)
	}
	
	// Create descriptor store with storage manager
	descriptorStore, err := descriptors.NewStoreWithManager(c.storageManager)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create descriptor store: %w", err)
	}
	
	// Load descriptor
	descriptor, err := descriptorStore.Load(descriptorCID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to load descriptor: %w", err)
	}
	
	if progress != nil {
		progress("Loading file descriptor", 100, 100)
	}
	
	// Retrieve and reconstruct blocks
	var originalBlocks []*blocks.Block
	totalBlocks := len(descriptor.Blocks)
	
	for i, blockInfo := range descriptor.Blocks {
		if progress != nil {
			progress("Downloading blocks", i, totalBlocks)
		}
		// Retrieve anonymized data block
		dataBlock, err := c.retrieveBlock(context.Background(), blockInfo.DataCID)
		if err != nil {
			return nil, "", fmt.Errorf("failed to retrieve data block: %w", err)
		}
		
		// Retrieve randomizer blocks
		randBlock1, err := c.retrieveBlock(context.Background(), blockInfo.RandomizerCID1)
		if err != nil {
			return nil, "", fmt.Errorf("failed to retrieve randomizer1 block: %w", err)
		}
		
		// Retrieve second randomizer block (3-tuple XOR)
		randBlock2, err := c.retrieveBlock(context.Background(), blockInfo.RandomizerCID2)
		if err != nil {
			return nil, "", fmt.Errorf("failed to retrieve randomizer2 block: %w", err)
		}
		
		origBlock, err := dataBlock.XOR(randBlock1, randBlock2)
		if err != nil {
			return nil, "", fmt.Errorf("failed to XOR blocks: %w", err)
		}
		
		originalBlocks = append(originalBlocks, origBlock)
	}
	
	if progress != nil {
		progress("Downloading blocks", totalBlocks, totalBlocks)
	}
	
	// Assemble file
	if progress != nil {
		progress("Assembling file", 0, 100)
	}
	
	assembler := blocks.NewAssembler()
	var buf strings.Builder
	if err := assembler.AssembleToWriter(originalBlocks, &buf); err != nil {
		return nil, "", fmt.Errorf("failed to assemble file: %w", err)
	}
	
	if progress != nil {
		progress("Assembling file", 100, 100)
	}
	
	// Record download
	c.RecordDownload()
	
	return []byte(buf.String()), descriptor.Filename, nil
}
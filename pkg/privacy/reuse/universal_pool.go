package reuse

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/tools/bootstrap"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/ipfs"
)

// UniversalBlockPool manages the mandatory pool of reusable blocks
type UniversalBlockPool struct {
	blocks           map[string]*PoolBlock      // CID -> PoolBlock
	blocksBySize     map[int][]string          // size -> []CID
	publicDomainCIDs map[string]bool           // CID -> is public domain
	metrics          *PoolMetrics
	mutex            sync.RWMutex
	
	// Configuration
	config           *PoolConfig
	ipfsClient       *ipfs.Client
	initialized      bool
}

// PoolBlock represents a block in the universal pool
type PoolBlock struct {
	CID              string            `json:"cid"`
	Block            *blocks.Block     `json:"-"` // Don't serialize the block data
	Size             int               `json:"size"`
	IsPublicDomain   bool              `json:"is_public_domain"`
	Source           string            `json:"source"`           // "bootstrap", "upload", "genesis"
	ContentType      string            `json:"content_type"`     // "text", "image", "audio", etc.
	UsageCount       int64             `json:"usage_count"`
	CreatedAt        time.Time         `json:"created_at"`
	LastUsed         time.Time         `json:"last_used"`
	PopularityScore  float64           `json:"popularity_score"`
	Metadata         map[string]string `json:"metadata"`
}

// PoolConfig defines the universal pool configuration
type PoolConfig struct {
	MinPoolSize          int     `json:"min_pool_size"`           // Minimum blocks per size
	MaxPoolSize          int     `json:"max_pool_size"`           // Maximum blocks per size
	PublicDomainRatio    float64 `json:"public_domain_ratio"`     // Required ratio of public domain blocks
	RefreshInterval      time.Duration `json:"refresh_interval"`   // How often to refresh blocks
	PopularityThreshold  float64 `json:"popularity_threshold"`    // Minimum popularity to keep
	MinReuseCount        int64   `json:"min_reuse_count"`         // Minimum usage before block removal
}

// PoolMetrics tracks pool statistics
type PoolMetrics struct {
	TotalBlocks        int   `json:"total_blocks"`
	PublicDomainBlocks int   `json:"public_domain_blocks"`
	TotalUsages        int64 `json:"total_usages"`
	BlocksGenerated    int64 `json:"blocks_generated"`
	BlocksRefreshed    int64 `json:"blocks_refreshed"`
	mutex              sync.RWMutex
}

// DefaultPoolConfig returns the default pool configuration
func DefaultPoolConfig() *PoolConfig {
	return &PoolConfig{
		MinPoolSize:         100,  // 100 blocks per size minimum
		MaxPoolSize:         1000, // 1000 blocks per size maximum
		PublicDomainRatio:   0.5,  // 50% must be public domain
		RefreshInterval:     time.Hour * 24, // Refresh daily
		PopularityThreshold: 0.1,  // Keep blocks with 0.1+ popularity
		MinReuseCount:       5,    // Must be used 5+ times
	}
}

// NewUniversalBlockPool creates a new universal block pool
func NewUniversalBlockPool(config *PoolConfig, ipfsClient *ipfs.Client) *UniversalBlockPool {
	if config == nil {
		config = DefaultPoolConfig()
	}

	return &UniversalBlockPool{
		blocks:           make(map[string]*PoolBlock),
		blocksBySize:     make(map[int][]string),
		publicDomainCIDs: make(map[string]bool),
		metrics:          &PoolMetrics{},
		config:           config,
		ipfsClient:       ipfsClient,
		initialized:      false,
	}
}

// Initialize populates the pool with public domain content and genesis blocks
func (pool *UniversalBlockPool) Initialize() error {
	pool.mutex.Lock()
	defer pool.mutex.Unlock()

	if pool.initialized {
		return nil
	}

	// Generate genesis blocks for common sizes
	if err := pool.generateGenesisBlocks(); err != nil {
		return fmt.Errorf("failed to generate genesis blocks: %w", err)
	}

	// Load public domain blocks from bootstrap content
	if err := pool.loadPublicDomainBlocks(); err != nil {
		return fmt.Errorf("failed to load public domain blocks: %w", err)
	}

	// Validate pool meets minimum requirements
	if err := pool.validatePool(); err != nil {
		return fmt.Errorf("pool validation failed: %w", err)
	}

	pool.initialized = true
	return nil
}

// generateGenesisBlocks creates deterministic blocks for each standard size
func (pool *UniversalBlockPool) generateGenesisBlocks() error {
	standardSizes := []int{
		64 * 1024,   // 64 KiB
		128 * 1024,  // 128 KiB
		256 * 1024,  // 256 KiB
		512 * 1024,  // 512 KiB
		1024 * 1024, // 1 MiB
	}

	for _, size := range standardSizes {
		// Generate deterministic blocks for this size
		for i := 0; i < pool.config.MinPoolSize/2; i++ {
			block, err := pool.generateDeterministicBlock(size, i)
			if err != nil {
				return fmt.Errorf("failed to generate deterministic block: %w", err)
			}

			cid, err := pool.storeBlock(block)
			if err != nil {
				return fmt.Errorf("failed to store genesis block: %w", err)
			}

			poolBlock := &PoolBlock{
				CID:             cid,
				Block:           block,
				Size:            size,
				IsPublicDomain:  true, // Genesis blocks are considered public domain
				Source:          "genesis",
				ContentType:     "random",
				UsageCount:      0,
				CreatedAt:       time.Now(),
				LastUsed:        time.Now(),
				PopularityScore: 1.0, // High initial popularity
				Metadata: map[string]string{
					"genesis_index": fmt.Sprintf("%d", i),
					"deterministic": "true",
				},
			}

			pool.addBlockToPool(poolBlock)
		}
	}

	return nil
}

// generateDeterministicBlock creates a deterministic block for the given size and index
func (pool *UniversalBlockPool) generateDeterministicBlock(size, index int) (*blocks.Block, error) {
	// Create deterministic content using SHA-256 hash chain
	seed := fmt.Sprintf("noisefs-genesis-%d-%d", size, index)
	hash := sha256.Sum256([]byte(seed))
	
	// Expand hash to fill block size
	data := make([]byte, size)
	for i := 0; i < size; i += 32 {
		copy(data[i:], hash[:])
		// Chain the hash for next 32 bytes
		hash = sha256.Sum256(hash[:])
	}

	return blocks.NewBlock(data)
}

// loadPublicDomainBlocks loads blocks from bootstrap public domain content
func (pool *UniversalBlockPool) loadPublicDomainBlocks() error {
	// Get available public domain datasets
	datasets := bootstrap.GetBuiltinDatasets()
	
	// Create a slice of datasets from the map
	datasetList := make([]*bootstrap.Dataset, 0, len(datasets))
	for _, dataset := range datasets {
		datasetList = append(datasetList, dataset)
	}
	
	// Process each dataset
	for _, dataset := range datasetList {
		// For now, simulate loading public domain blocks
		// In full implementation, this would download and process the content
		if err := pool.simulatePublicDomainBlocks(dataset); err != nil {
			return fmt.Errorf("failed to load dataset %s: %w", dataset.Name, err)
		}
	}

	return nil
}

// simulatePublicDomainBlocks creates simulated public domain blocks
func (pool *UniversalBlockPool) simulatePublicDomainBlocks(dataset *bootstrap.Dataset) error {
	// Generate blocks that simulate public domain content
	sizes := []int{64 * 1024, 128 * 1024, 256 * 1024}
	
	for _, size := range sizes {
		for i := 0; i < 20; i++ { // 20 blocks per size per dataset
			// Create pseudo-content based on dataset
			content := pool.generatePublicDomainContent(dataset, size, i)
			
			block, err := blocks.NewBlock(content)
			if err != nil {
				return fmt.Errorf("failed to create block: %w", err)
			}

			cid, err := pool.storeBlock(block)
			if err != nil {
				return fmt.Errorf("failed to store public domain block: %w", err)
			}

			poolBlock := &PoolBlock{
				CID:             cid,
				Block:           block,
				Size:            size,
				IsPublicDomain:  true,
				Source:          "bootstrap",
				ContentType:     dataset.Directory, // e.g., "books_literature"
				UsageCount:      0,
				CreatedAt:       time.Now(),
				LastUsed:        time.Now(),
				PopularityScore: 0.8, // High popularity for public domain
				Metadata: map[string]string{
					"dataset":     dataset.Name,
					"block_index": fmt.Sprintf("%d", i),
					"license":     "public_domain",
				},
			}

			pool.addBlockToPool(poolBlock)
		}
	}

	return nil
}

// generatePublicDomainContent creates content that simulates public domain material
func (pool *UniversalBlockPool) generatePublicDomainContent(dataset *bootstrap.Dataset, size, index int) []byte {
	// Create repeating pattern based on dataset characteristics
	seed := fmt.Sprintf("%s-%d-%d", dataset.Name, size, index)
	hash := sha256.Sum256([]byte(seed))
	
	data := make([]byte, size)
	
	// Fill with patterns that simulate different content types
	switch dataset.Directory {
	case "books_literature":
		// Simulate text patterns
		pattern := []byte("public domain text content from project gutenberg ")
		for i := 0; i < len(data); i++ {
			data[i] = pattern[i%len(pattern)] ^ hash[i%32]
		}
	case "images_artwork":
		// Simulate image patterns
		for i := 0; i < len(data); i++ {
			data[i] = byte(i%256) ^ hash[i%32]
		}
	default:
		// Default pattern
		for i := 0; i < len(data); i++ {
			data[i] = hash[i%32]
		}
	}

	return data
}

// storeBlock stores a block in IPFS and returns its CID
func (pool *UniversalBlockPool) storeBlock(block *blocks.Block) (string, error) {
	// In full implementation, this would store in IPFS
	// For now, generate a deterministic CID based on content
	hash := sha256.Sum256(block.Data)
	return hex.EncodeToString(hash[:16]), nil // Use first 16 bytes as CID
}

// addBlockToPool adds a block to the pool data structures
func (pool *UniversalBlockPool) addBlockToPool(poolBlock *PoolBlock) {
	pool.blocks[poolBlock.CID] = poolBlock
	
	// Add to size index
	if _, exists := pool.blocksBySize[poolBlock.Size]; !exists {
		pool.blocksBySize[poolBlock.Size] = make([]string, 0)
	}
	pool.blocksBySize[poolBlock.Size] = append(pool.blocksBySize[poolBlock.Size], poolBlock.CID)
	
	// Mark as public domain if applicable
	if poolBlock.IsPublicDomain {
		pool.publicDomainCIDs[poolBlock.CID] = true
	}
	
	// Update metrics
	pool.metrics.mutex.Lock()
	pool.metrics.TotalBlocks++
	if poolBlock.IsPublicDomain {
		pool.metrics.PublicDomainBlocks++
	}
	pool.metrics.BlocksGenerated++
	pool.metrics.mutex.Unlock()
}

// validatePool ensures the pool meets minimum requirements
func (pool *UniversalBlockPool) validatePool() error {
	// Check minimum pool size for each standard size
	standardSizes := []int{64 * 1024, 128 * 1024, 256 * 1024}
	
	for _, size := range standardSizes {
		blocks := pool.blocksBySize[size]
		if len(blocks) < pool.config.MinPoolSize {
			return fmt.Errorf("insufficient blocks for size %d: have %d, need %d", 
				size, len(blocks), pool.config.MinPoolSize)
		}
		
		// Check public domain ratio
		publicCount := 0
		for _, cid := range blocks {
			if pool.publicDomainCIDs[cid] {
				publicCount++
			}
		}
		
		ratio := float64(publicCount) / float64(len(blocks))
		if ratio < pool.config.PublicDomainRatio {
			return fmt.Errorf("insufficient public domain blocks for size %d: ratio %.2f, need %.2f",
				size, ratio, pool.config.PublicDomainRatio)
		}
	}

	return nil
}

// GetRandomizerBlock returns a random block from the pool for the given size
func (pool *UniversalBlockPool) GetRandomizerBlock(size int) (*PoolBlock, error) {
	pool.mutex.RLock()
	defer pool.mutex.RUnlock()

	if !pool.initialized {
		return nil, fmt.Errorf("pool not initialized")
	}

	blocks := pool.blocksBySize[size]
	if len(blocks) == 0 {
		return nil, fmt.Errorf("no blocks available for size %d", size)
	}

	// Select random block
	index := rand.Intn(len(blocks))
	cid := blocks[index]
	
	poolBlock := pool.blocks[cid]
	if poolBlock == nil {
		return nil, fmt.Errorf("block not found: %s", cid)
	}

	// Update usage statistics
	poolBlock.UsageCount++
	poolBlock.LastUsed = time.Now()
	poolBlock.PopularityScore = pool.calculatePopularity(poolBlock)

	// Update metrics
	pool.metrics.mutex.Lock()
	pool.metrics.TotalUsages++
	pool.metrics.mutex.Unlock()

	return poolBlock, nil
}

// GetPublicDomainBlock returns a random public domain block for the given size
func (pool *UniversalBlockPool) GetPublicDomainBlock(size int) (*PoolBlock, error) {
	pool.mutex.RLock()
	defer pool.mutex.RUnlock()

	if !pool.initialized {
		return nil, fmt.Errorf("pool not initialized")
	}

	// Filter to public domain blocks only
	var publicBlocks []string
	for _, cid := range pool.blocksBySize[size] {
		if pool.publicDomainCIDs[cid] {
			publicBlocks = append(publicBlocks, cid)
		}
	}

	if len(publicBlocks) == 0 {
		return nil, fmt.Errorf("no public domain blocks available for size %d", size)
	}

	// Select random public domain block
	index := rand.Intn(len(publicBlocks))
	cid := publicBlocks[index]
	
	poolBlock := pool.blocks[cid]
	if poolBlock == nil {
		return nil, fmt.Errorf("public domain block not found: %s", cid)
	}

	// Update usage statistics
	poolBlock.UsageCount++
	poolBlock.LastUsed = time.Now()
	poolBlock.PopularityScore = pool.calculatePopularity(poolBlock)

	return poolBlock, nil
}

// GetPublicDomainBlocks returns multiple public domain blocks
func (pool *UniversalBlockPool) GetPublicDomainBlocks(count int) ([]*PoolBlock, error) {
	pool.mutex.RLock()
	defer pool.mutex.RUnlock()

	publicBlocks := make([]*PoolBlock, 0)
	for cid, isPublic := range pool.publicDomainCIDs {
		if isPublic {
			if block, exists := pool.blocks[cid]; exists {
				publicBlocks = append(publicBlocks, block)
			}
		}
	}

	if len(publicBlocks) == 0 {
		return nil, fmt.Errorf("no public domain blocks available")
	}

	// If we need more blocks than available, return all
	if count >= len(publicBlocks) {
		return publicBlocks, nil
	}

	// Randomly select the requested number of blocks
	selected := make([]*PoolBlock, count)
	perm := rand.Perm(len(publicBlocks))
	for i := 0; i < count; i++ {
		selected[i] = publicBlocks[perm[i]]
	}

	return selected, nil
}

// calculatePopularity calculates a block's popularity score
func (pool *UniversalBlockPool) calculatePopularity(block *PoolBlock) float64 {
	// Simple popularity calculation based on usage and recency
	usageScore := float64(block.UsageCount) / 100.0 // Normalize usage count
	
	// Recency bonus (blocks used recently get higher scores)
	timeSinceUse := time.Since(block.LastUsed)
	recencyScore := 1.0 / (1.0 + timeSinceUse.Hours()/24.0) // Decay over days
	
	// Public domain bonus
	publicDomainBonus := 1.0
	if block.IsPublicDomain {
		publicDomainBonus = 1.2
	}

	return (usageScore + recencyScore) * publicDomainBonus
}

// GetMetrics returns pool statistics
func (pool *UniversalBlockPool) GetMetrics() *PoolMetrics {
	pool.metrics.mutex.RLock()
	defer pool.metrics.mutex.RUnlock()

	// Return a copy to avoid race conditions
	return &PoolMetrics{
		TotalBlocks:        pool.metrics.TotalBlocks,
		PublicDomainBlocks: pool.metrics.PublicDomainBlocks,
		TotalUsages:        pool.metrics.TotalUsages,
		BlocksGenerated:    pool.metrics.BlocksGenerated,
		BlocksRefreshed:    pool.metrics.BlocksRefreshed,
	}
}

// GetStatus returns detailed pool status
func (pool *UniversalBlockPool) GetStatus() map[string]interface{} {
	pool.mutex.RLock()
	defer pool.mutex.RUnlock()

	status := make(map[string]interface{})
	status["initialized"] = pool.initialized
	status["total_blocks"] = len(pool.blocks)
	status["public_domain_blocks"] = len(pool.publicDomainCIDs)
	
	// Size distribution
	sizeDistribution := make(map[string]int)
	for size, blocks := range pool.blocksBySize {
		sizeDistribution[fmt.Sprintf("%d_KB", size/1024)] = len(blocks)
	}
	status["size_distribution"] = sizeDistribution

	// Public domain ratio
	if len(pool.blocks) > 0 {
		ratio := float64(len(pool.publicDomainCIDs)) / float64(len(pool.blocks))
		status["public_domain_ratio"] = ratio
	}

	return status
}

// IsInitialized returns whether the pool has been initialized
func (pool *UniversalBlockPool) IsInitialized() bool {
	pool.mutex.RLock()
	defer pool.mutex.RUnlock()
	return pool.initialized
}
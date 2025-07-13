// Package cache provides caching implementations for NoiseFS blocks.
// The altruistic cache extends the base cache functionality to support
// network health contributions while guaranteeing user storage needs.
package cache

import (
	"fmt"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
)

// BlockOrigin indicates whether a block is personal or altruistic
type BlockOrigin int

const (
	// PersonalBlock is a block explicitly requested by the user
	PersonalBlock BlockOrigin = iota
	// AltruisticBlock is a block cached for network benefit
	AltruisticBlock
)

// AltruisticCacheConfig holds configuration for altruistic caching
type AltruisticCacheConfig struct {
	// MinPersonalCache is the guaranteed space for personal blocks
	MinPersonalCache int64 `json:"min_personal_cache"`
	
	// EnableAltruistic allows disabling altruistic caching entirely
	EnableAltruistic bool `json:"enable_altruistic"`
	
	// EvictionCooldown prevents thrashing by limiting major evictions
	EvictionCooldown time.Duration `json:"eviction_cooldown"`
	
	// Optional advanced settings
	AltruisticBandwidthMB int `json:"altruistic_bandwidth_mb,omitempty"`
}

// BlockMetadata extends block info with origin tracking
type BlockMetadata struct {
	*BlockInfo
	Origin      BlockOrigin
	CachedAt    time.Time
	LastAccessed time.Time
}

// AltruisticCache wraps an existing cache with altruistic functionality.
// It implements the MinPersonal + Flex model where users set a minimum
// guaranteed personal storage amount, and all remaining capacity flexibly
// adjusts between personal and altruistic (network benefit) use.
//
// The cache ensures:
//   - Users always have MinPersonal space available for their files
//   - Spare capacity automatically benefits the network
//   - No complex configuration or prediction needed
//   - Privacy-preserving operation with no file-block associations
//
// Example usage:
//
//	config := &AltruisticCacheConfig{
//	    MinPersonalCache: 100 * 1024 * 1024 * 1024, // 100GB
//	    EnableAltruistic: true,
//	}
//	cache := NewAltruisticCache(baseCache, config, totalCapacity)
type AltruisticCache struct {
	// Embedded base cache (typically AdaptiveCache)
	baseCache Cache
	
	// Configuration
	config *AltruisticCacheConfig
	
	// Block categorization
	personalBlocks   map[string]*BlockMetadata
	altruisticBlocks map[string]*BlockMetadata
	
	// Space tracking
	personalSize     int64
	altruisticSize   int64
	totalCapacity    int64
	
	// Anti-thrashing
	lastMajorEviction time.Time
	
	// Metrics
	altruisticHits   int64
	altruisticMisses int64
	personalHits     int64
	personalMisses   int64
	
	// Synchronization
	mu sync.RWMutex
}

// NewAltruisticCache creates a new altruistic cache wrapping a base cache
func NewAltruisticCache(baseCache Cache, config *AltruisticCacheConfig, totalCapacity int64) *AltruisticCache {
	if config.EvictionCooldown == 0 {
		config.EvictionCooldown = 5 * time.Minute
	}
	
	return &AltruisticCache{
		baseCache:        baseCache,
		config:           config,
		personalBlocks:   make(map[string]*BlockMetadata),
		altruisticBlocks: make(map[string]*BlockMetadata),
		totalCapacity:    totalCapacity,
	}
}

// Store adds a block to the cache with origin metadata
func (ac *AltruisticCache) Store(cid string, block *blocks.Block) error {
	// Default to personal block for backward compatibility
	return ac.StoreWithOrigin(cid, block, PersonalBlock)
}

// StoreWithOrigin adds a block to the cache with explicit origin tracking.
// Personal blocks are protected by the MinPersonal guarantee and will not
// be evicted to make room for altruistic blocks. Altruistic blocks can be
// evicted when users need space for personal files.
//
// Returns an error if:
//   - Altruistic caching is disabled and origin is AltruisticBlock
//   - There is insufficient space even after eviction attempts
//   - The underlying storage operation fails
func (ac *AltruisticCache) StoreWithOrigin(cid string, block *blocks.Block, origin BlockOrigin) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	
	blockSize := int64(block.Size())
	
	// Check if altruistic caching is disabled
	if origin == AltruisticBlock && !ac.config.EnableAltruistic {
		return fmt.Errorf("altruistic caching is disabled")
	}
	
	// Handle personal blocks
	if origin == PersonalBlock {
		// Check if we need to evict altruistic blocks
		available := ac.getAvailableSpace()
		if ac.personalSize + blockSize > ac.config.MinPersonalCache && available < blockSize {
			// Need to make room by evicting altruistic blocks
			if err := ac.evictAltruisticBlocks(blockSize - available); err != nil {
				return fmt.Errorf("failed to make space for personal block: %w", err)
			}
		}
		
		// Store in base cache
		if err := ac.baseCache.Store(cid, block); err != nil {
			return err
		}
		
		// Track as personal
		metadata := &BlockMetadata{
			BlockInfo: &BlockInfo{
				CID:   cid,
				Block: block,
				Size:  int(blockSize),
			},
			Origin:       PersonalBlock,
			CachedAt:     time.Now(),
			LastAccessed: time.Now(),
		}
		
		// If block was previously altruistic, update tracking
		if _, wasAltruistic := ac.altruisticBlocks[cid]; wasAltruistic {
			ac.altruisticSize -= blockSize
			delete(ac.altruisticBlocks, cid)
		}
		
		ac.personalBlocks[cid] = metadata
		ac.personalSize += blockSize
		
	} else {
		// Handle altruistic blocks
		if !ac.canAcceptAltruistic(blockSize) {
			return fmt.Errorf("insufficient space for altruistic block")
		}
		
		// Store in base cache
		if err := ac.baseCache.Store(cid, block); err != nil {
			return err
		}
		
		// Track as altruistic
		metadata := &BlockMetadata{
			BlockInfo: &BlockInfo{
				CID:   cid,
				Block: block,
				Size:  int(blockSize),
			},
			Origin:       AltruisticBlock,
			CachedAt:     time.Now(),
			LastAccessed: time.Now(),
		}
		
		ac.altruisticBlocks[cid] = metadata
		ac.altruisticSize += blockSize
	}
	
	return nil
}

// Get retrieves a block and updates access tracking
func (ac *AltruisticCache) Get(cid string) (*blocks.Block, error) {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	
	block, err := ac.baseCache.Get(cid)
	if err != nil {
		// Track misses
		if _, isPersonal := ac.personalBlocks[cid]; isPersonal {
			ac.personalMisses++
		} else {
			ac.altruisticMisses++
		}
		return nil, err
	}
	
	// Update access time and track hits
	if metadata, isPersonal := ac.personalBlocks[cid]; isPersonal {
		metadata.LastAccessed = time.Now()
		ac.personalHits++
	} else if metadata, isAltruistic := ac.altruisticBlocks[cid]; isAltruistic {
		metadata.LastAccessed = time.Now()
		ac.altruisticHits++
	}
	
	return block, nil
}

// Has checks if a block exists in the cache
func (ac *AltruisticCache) Has(cid string) bool {
	return ac.baseCache.Has(cid)
}

// Remove removes a block from the cache
func (ac *AltruisticCache) Remove(cid string) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	
	// Remove from base cache
	if err := ac.baseCache.Remove(cid); err != nil {
		return err
	}
	
	// Update tracking
	if metadata, isPersonal := ac.personalBlocks[cid]; isPersonal {
		ac.personalSize -= int64(metadata.Size)
		delete(ac.personalBlocks, cid)
	} else if metadata, isAltruistic := ac.altruisticBlocks[cid]; isAltruistic {
		ac.altruisticSize -= int64(metadata.Size)
		delete(ac.altruisticBlocks, cid)
	}
	
	return nil
}

// GetRandomizers returns popular blocks suitable as randomizers
func (ac *AltruisticCache) GetRandomizers(count int) ([]*BlockInfo, error) {
	return ac.baseCache.GetRandomizers(count)
}

// IncrementPopularity increases the popularity score of a block
func (ac *AltruisticCache) IncrementPopularity(cid string) error {
	return ac.baseCache.IncrementPopularity(cid)
}

// Size returns the total number of blocks in the cache
func (ac *AltruisticCache) Size() int {
	return ac.baseCache.Size()
}

// Clear removes all blocks from the cache
func (ac *AltruisticCache) Clear() {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	
	ac.baseCache.Clear()
	ac.personalBlocks = make(map[string]*BlockMetadata)
	ac.altruisticBlocks = make(map[string]*BlockMetadata)
	ac.personalSize = 0
	ac.altruisticSize = 0
}

// GetStats returns extended cache statistics
func (ac *AltruisticCache) GetStats() *Stats {
	baseStats := ac.baseCache.GetStats()
	
	// Extend with altruistic stats
	return &Stats{
		Hits:      baseStats.Hits,
		Misses:    baseStats.Misses,
		Evictions: baseStats.Evictions,
		Size:      baseStats.Size,
	}
}

// GetAltruisticStats returns detailed statistics about cache usage.
// This includes separate metrics for personal and altruistic blocks,
// current space utilization, hit/miss rates, and flex pool usage.
// The flex pool usage indicates what percentage of the non-guaranteed
// space is currently in use (0.0 = all free, 1.0 = fully utilized).
func (ac *AltruisticCache) GetAltruisticStats() *AltruisticStats {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	
	return &AltruisticStats{
		PersonalBlocks:    len(ac.personalBlocks),
		AltruisticBlocks:  len(ac.altruisticBlocks),
		PersonalSize:      ac.personalSize,
		AltruisticSize:    ac.altruisticSize,
		TotalCapacity:     ac.totalCapacity,
		PersonalHits:      ac.personalHits,
		PersonalMisses:    ac.personalMisses,
		AltruisticHits:    ac.altruisticHits,
		AltruisticMisses:  ac.altruisticMisses,
		FlexPoolUsage:     ac.getFlexPoolUsage(),
	}
}

// AltruisticStats holds detailed cache statistics
type AltruisticStats struct {
	PersonalBlocks   int     `json:"personal_blocks"`
	AltruisticBlocks int     `json:"altruistic_blocks"`
	PersonalSize     int64   `json:"personal_size"`
	AltruisticSize   int64   `json:"altruistic_size"`
	TotalCapacity    int64   `json:"total_capacity"`
	PersonalHits     int64   `json:"personal_hits"`
	PersonalMisses   int64   `json:"personal_misses"`
	AltruisticHits   int64   `json:"altruistic_hits"`
	AltruisticMisses int64   `json:"altruistic_misses"`
	FlexPoolUsage    float64 `json:"flex_pool_usage"`
}

// Helper methods

func (ac *AltruisticCache) getAvailableSpace() int64 {
	return ac.totalCapacity - ac.personalSize - ac.altruisticSize
}

func (ac *AltruisticCache) canAcceptAltruistic(size int64) bool {
	// Ensure personal can always reach minimum
	spaceAfterAdd := ac.totalCapacity - ac.personalSize - ac.altruisticSize - size
	return ac.personalSize + spaceAfterAdd >= ac.config.MinPersonalCache
}

func (ac *AltruisticCache) getFlexPoolUsage() float64 {
	flexPoolSize := ac.totalCapacity - ac.config.MinPersonalCache
	if flexPoolSize <= 0 {
		return 0
	}
	
	flexPoolUsed := ac.personalSize + ac.altruisticSize - ac.config.MinPersonalCache
	if flexPoolUsed < 0 {
		flexPoolUsed = 0
	}
	
	return float64(flexPoolUsed) / float64(flexPoolSize)
}

func (ac *AltruisticCache) evictAltruisticBlocks(needed int64) error {
	// Anti-thrashing check
	if time.Since(ac.lastMajorEviction) < ac.config.EvictionCooldown {
		return fmt.Errorf("eviction cooldown active")
	}
	
	// Get altruistic blocks sorted by last access time (oldest first)
	candidates := ac.getAltruisticBlocksByAge()
	
	freed := int64(0)
	for _, metadata := range candidates {
		if freed >= needed {
			break
		}
		
		if err := ac.baseCache.Remove(metadata.CID); err != nil {
			continue // Skip blocks that can't be removed
		}
		
		freed += int64(metadata.Size)
		ac.altruisticSize -= int64(metadata.Size)
		delete(ac.altruisticBlocks, metadata.CID)
	}
	
	if freed >= needed*2 {
		ac.lastMajorEviction = time.Now()
	}
	
	if freed < needed {
		return fmt.Errorf("insufficient space freed: need %d, freed %d", needed, freed)
	}
	
	return nil
}

func (ac *AltruisticCache) getAltruisticBlocksByAge() []*BlockMetadata {
	blocks := make([]*BlockMetadata, 0, len(ac.altruisticBlocks))
	for _, metadata := range ac.altruisticBlocks {
		blocks = append(blocks, metadata)
	}
	
	// Sort by last access time (oldest first)
	// Simple bubble sort for now - can optimize later
	for i := 0; i < len(blocks); i++ {
		for j := i + 1; j < len(blocks); j++ {
			if blocks[i].LastAccessed.After(blocks[j].LastAccessed) {
				blocks[i], blocks[j] = blocks[j], blocks[i]
			}
		}
	}
	
	return blocks
}
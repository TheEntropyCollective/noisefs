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
	
	// Eviction strategy configuration
	EvictionStrategy      string  `json:"eviction_strategy,omitempty"` // "LRU", "LFU", "ValueBased", "Adaptive"
	EnablePredictive      bool    `json:"enable_predictive,omitempty"`
	EnableGradualEviction bool    `json:"enable_gradual_eviction,omitempty"`
	PreEvictThreshold     float64 `json:"pre_evict_threshold,omitempty"`
}

// BlockMetadata extends block info with origin tracking
type BlockMetadata struct {
	*BlockInfo
	Origin       BlockOrigin
	CachedAt     time.Time
	LastAccessed time.Time
	
	// Performance optimization: cached scores with TTL
	cachedScores    map[string]float64 // strategy name -> score
	scoreExpiry     map[string]time.Time // strategy name -> expiry
	scoresCacheTTL  time.Duration // configurable TTL (default 5min)
	scoreMutex      sync.RWMutex // protect score cache
}

// DefaultScoreCacheTTL is the default time-to-live for cached scores
const DefaultScoreCacheTTL = 5 * time.Minute

// NewBlockMetadata creates a new BlockMetadata with score caching initialized
func NewBlockMetadata(blockInfo *BlockInfo, origin BlockOrigin) *BlockMetadata {
	return &BlockMetadata{
		BlockInfo:      blockInfo,
		Origin:         origin,
		CachedAt:       time.Now(),
		LastAccessed:   time.Now(),
		cachedScores:   make(map[string]float64),
		scoreExpiry:    make(map[string]time.Time),
		scoresCacheTTL: DefaultScoreCacheTTL,
	}
}

// GetCachedScore retrieves a cached score if it's still valid
func (bm *BlockMetadata) GetCachedScore(strategyName string) (float64, bool) {
	bm.scoreMutex.RLock()
	defer bm.scoreMutex.RUnlock()
	
	score, exists := bm.cachedScores[strategyName]
	if !exists {
		return 0, false
	}
	
	expiry, expiryExists := bm.scoreExpiry[strategyName]
	if !expiryExists || time.Now().After(expiry) {
		return 0, false
	}
	
	return score, true
}

// SetCachedScore stores a score with TTL expiry
func (bm *BlockMetadata) SetCachedScore(strategyName string, score float64) {
	bm.scoreMutex.Lock()
	defer bm.scoreMutex.Unlock()
	
	if bm.cachedScores == nil {
		bm.cachedScores = make(map[string]float64)
		bm.scoreExpiry = make(map[string]time.Time)
		bm.scoresCacheTTL = DefaultScoreCacheTTL
	}
	
	bm.cachedScores[strategyName] = score
	bm.scoreExpiry[strategyName] = time.Now().Add(bm.scoresCacheTTL)
}

// ClearCachedScores removes all cached scores (called when metadata changes)
func (bm *BlockMetadata) ClearCachedScores() {
	bm.scoreMutex.Lock()
	defer bm.scoreMutex.Unlock()
	
	bm.cachedScores = make(map[string]float64)
	bm.scoreExpiry = make(map[string]time.Time)
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
	recentlyEvicted   map[string]time.Time // Track recently evicted blocks
	evictionHistory   []string             // Order of evictions
	
	// Eviction strategies
	evictionStrategy EvictionStrategy
	healthTracker    *BlockHealthTracker
	predictiveEvictor *PredictiveEvictionIntegration
	
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
	
	ac := &AltruisticCache{
		baseCache:        baseCache,
		config:           config,
		personalBlocks:   make(map[string]*BlockMetadata),
		altruisticBlocks: make(map[string]*BlockMetadata),
		totalCapacity:    totalCapacity,
		recentlyEvicted:  make(map[string]time.Time),
		evictionHistory:  make([]string, 0, 100),
	}
	
	// Initialize eviction strategy
	ac.evictionStrategy = ac.createEvictionStrategy()
	
	// Initialize health tracker
	ac.healthTracker = NewBlockHealthTracker(nil)
	
	// Initialize predictive eviction if enabled
	if config.EnablePredictive {
		predictiveConfig := &PredictiveEvictorConfig{
			PreEvictThreshold: config.PreEvictThreshold,
		}
		if predictiveConfig.PreEvictThreshold == 0 {
			predictiveConfig.PreEvictThreshold = 0.85
		}
		ac.predictiveEvictor = NewPredictiveEvictionIntegration(ac, predictiveConfig)
	}
	
	return ac
}

// createEvictionStrategy creates the configured eviction strategy
func (ac *AltruisticCache) createEvictionStrategy() EvictionStrategy {
	var base EvictionStrategy
	
	switch ac.config.EvictionStrategy {
	case "LRU":
		base = &LRUEvictionStrategy{}
	case "LFU":
		base = &LFUEvictionStrategy{}
	case "ValueBased":
		base = NewValueBasedEvictionStrategy()
	case "Adaptive":
		base = NewAdaptiveEvictionStrategy()
	default:
		// Default to LRU
		base = &LRUEvictionStrategy{}
	}
	
	// Wrap with gradual eviction if enabled
	if ac.config.EnableGradualEviction {
		base = NewGradualEvictionStrategy(base)
	}
	
	return base
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
		
		// Check anti-thrashing: don't re-add recently evicted blocks
		if evictTime, wasEvicted := ac.recentlyEvicted[cid]; wasEvicted {
			if time.Since(evictTime) < ac.config.EvictionCooldown {
				return fmt.Errorf("block was recently evicted, cooldown active")
			}
			// Clean up old eviction record
			delete(ac.recentlyEvicted, cid)
		}
		
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
		metadata.Popularity++
		ac.personalHits++
	} else if metadata, isAltruistic := ac.altruisticBlocks[cid]; isAltruistic {
		metadata.LastAccessed = time.Now()
		metadata.Popularity++
		ac.altruisticHits++
	}
	
	// Record access for predictive eviction
	if ac.predictiveEvictor != nil {
		ac.predictiveEvictor.RecordBlockAccess(cid)
	}
	
	// Update health tracker
	ac.healthTracker.RecordRequest(cid)
	
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
		HitRate:   baseStats.HitRate,
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
	
	// Use eviction strategy to select candidates
	candidates := ac.evictionStrategy.SelectEvictionCandidates(
		ac.altruisticBlocks,
		needed,
		ac.healthTracker,
	)
	
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
		
		// Track eviction for anti-thrashing
		ac.recentlyEvicted[metadata.CID] = time.Now()
		ac.evictionHistory = append(ac.evictionHistory, metadata.CID)
		
		// Limit history size
		if len(ac.evictionHistory) > 1000 {
			// Remove oldest entries
			oldCID := ac.evictionHistory[0]
			delete(ac.recentlyEvicted, oldCID)
			ac.evictionHistory = ac.evictionHistory[1:]
		}
	}
	
	if freed >= needed*2 {
		ac.lastMajorEviction = time.Now()
	}
	
	if freed < needed {
		return fmt.Errorf("insufficient space freed: need %d, freed %d", needed, freed)
	}
	
	return nil
}


// ShouldCacheAltruistic checks if an altruistic block should be cached
// Returns false if the block was recently evicted or space constraints prevent it
func (ac *AltruisticCache) ShouldCacheAltruistic(cid string, size int64) bool {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	
	// Check if disabled
	if !ac.config.EnableAltruistic {
		return false
	}
	
	// Check anti-thrashing
	if evictTime, wasEvicted := ac.recentlyEvicted[cid]; wasEvicted {
		if time.Since(evictTime) < ac.config.EvictionCooldown {
			return false
		}
	}
	
	// Check space constraints
	return ac.canAcceptAltruistic(size)
}

// GetEvictionHistory returns recent eviction history for debugging
func (ac *AltruisticCache) GetEvictionHistory() []string {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	
	history := make([]string, len(ac.evictionHistory))
	copy(history, ac.evictionHistory)
	return history
}

// UpdateBlockHealth updates health information for a block
func (ac *AltruisticCache) UpdateBlockHealth(cid string, hint BlockHint) {
	ac.healthTracker.UpdateBlockHealth(cid, hint)
}

// SetEvictionStrategy changes the eviction strategy at runtime
func (ac *AltruisticCache) SetEvictionStrategy(strategy string) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	
	ac.config.EvictionStrategy = strategy
	ac.evictionStrategy = ac.createEvictionStrategy()
}

// PerformPreEviction triggers predictive eviction if enabled
func (ac *AltruisticCache) PerformPreEviction() error {
	if ac.predictiveEvictor == nil {
		return nil
	}
	
	return ac.predictiveEvictor.PerformPreEviction()
}

// GetHealthTracker returns the block health tracker for external use
func (ac *AltruisticCache) GetHealthTracker() *BlockHealthTracker {
	return ac.healthTracker
}

// GetConfig returns the altruistic cache configuration
func (ac *AltruisticCache) GetConfig() *AltruisticCacheConfig {
	return ac.config
}
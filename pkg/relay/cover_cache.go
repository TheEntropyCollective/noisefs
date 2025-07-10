package relay

import (
	"context"
	"sync"
	"time"
)

// CoverBlockCache caches cover blocks to avoid redundant requests and reduce waste
type CoverBlockCache struct {
	cache        map[string]*CachedCoverBlock
	config       *CoverCacheConfig
	metrics      *CoverCacheMetrics
	accessOrder  []*CachedCoverBlock // LRU tracking
	totalSize    int64
	mu           sync.RWMutex
	cleanupTicker *time.Ticker
	ctx          context.Context
	cancel       context.CancelFunc
}

// CoverCacheConfig contains configuration for the cover block cache
type CoverCacheConfig struct {
	MaxSize          int64         // Maximum cache size in bytes
	MaxBlocks        int           // Maximum number of blocks to cache
	TTL              time.Duration // Time to live for cached blocks
	CleanupInterval  time.Duration // How often to run cleanup
	PrefetchSize     int           // Number of popular blocks to prefetch
	ReplicationFactor int          // How many copies of popular blocks to keep
	CacheStrategy    string        // "LRU", "LFU", "TTL", "Popular"
}

// CachedCoverBlock represents a cached cover block
type CachedCoverBlock struct {
	BlockID        string
	Data           []byte
	Category       BlockCategory
	PopularityScore float64
	CachedAt       time.Time
	LastAccessed   time.Time
	AccessCount    int64
	Size           int64
	IsPrefetched   bool
	ExpiresAt      time.Time
}

// CoverCacheMetrics tracks cache performance
type CoverCacheMetrics struct {
	TotalRequests    int64
	CacheHits        int64
	CacheMisses      int64
	Evictions        int64
	PrefetchHits     int64
	TotalSize        int64
	BlockCount       int64
	HitRate          float64
	AverageBlockSize float64
	LastUpdate       time.Time
}

// NewCoverBlockCache creates a new cover block cache
func NewCoverBlockCache(config *CoverCacheConfig) *CoverBlockCache {
	ctx, cancel := context.WithCancel(context.Background())
	
	cache := &CoverBlockCache{
		cache:       make(map[string]*CachedCoverBlock),
		config:      config,
		metrics:     &CoverCacheMetrics{},
		accessOrder: make([]*CachedCoverBlock, 0),
		ctx:         ctx,
		cancel:      cancel,
	}
	
	// Start cleanup routine
	cache.cleanupTicker = time.NewTicker(config.CleanupInterval)
	go cache.cleanupLoop()
	
	return cache
}

// Get retrieves a block from the cache
func (cbc *CoverBlockCache) Get(blockID string) (*CachedCoverBlock, bool) {
	cbc.mu.Lock()
	defer cbc.mu.Unlock()
	
	cbc.metrics.TotalRequests++
	
	block, exists := cbc.cache[blockID]
	if !exists {
		cbc.metrics.CacheMisses++
		return nil, false
	}
	
	// Check if block has expired
	if !block.ExpiresAt.IsZero() && time.Now().After(block.ExpiresAt) {
		delete(cbc.cache, blockID)
		cbc.removeFromAccessOrder(block)
		cbc.totalSize -= block.Size
		cbc.metrics.CacheMisses++
		return nil, false
	}
	
	// Update access statistics
	block.LastAccessed = time.Now()
	block.AccessCount++
	
	// Update LRU order
	cbc.updateAccessOrder(block)
	
	cbc.metrics.CacheHits++
	if block.IsPrefetched {
		cbc.metrics.PrefetchHits++
	}
	
	cbc.updateMetrics()
	
	return block, true
}

// Put stores a block in the cache
func (cbc *CoverBlockCache) Put(blockID string, data []byte, category BlockCategory, popularityScore float64) {
	cbc.mu.Lock()
	defer cbc.mu.Unlock()
	
	// Check if block already exists
	if existing, exists := cbc.cache[blockID]; exists {
		// Update existing block
		existing.Data = data
		existing.PopularityScore = popularityScore
		existing.LastAccessed = time.Now()
		existing.AccessCount++
		cbc.updateAccessOrder(existing)
		return
	}
	
	// Create new cached block
	block := &CachedCoverBlock{
		BlockID:         blockID,
		Data:            data,
		Category:        category,
		PopularityScore: popularityScore,
		CachedAt:        time.Now(),
		LastAccessed:    time.Now(),
		AccessCount:     1,
		Size:            int64(len(data)),
		IsPrefetched:    false,
		ExpiresAt:       time.Now().Add(cbc.config.TTL),
	}
	
	// Check cache capacity
	if err := cbc.makeRoom(block.Size); err != nil {
		return // Unable to make room
	}
	
	// Add to cache
	cbc.cache[blockID] = block
	cbc.accessOrder = append(cbc.accessOrder, block)
	cbc.totalSize += block.Size
	
	cbc.updateMetrics()
}

// PrefetchPopular prefetches popular blocks for cover traffic
func (cbc *CoverBlockCache) PrefetchPopular(popularBlocks []*PopularityInfo) {
	cbc.mu.Lock()
	defer cbc.mu.Unlock()
	
	prefetchCount := cbc.config.PrefetchSize
	if prefetchCount > len(popularBlocks) {
		prefetchCount = len(popularBlocks)
	}
	
	for i := 0; i < prefetchCount; i++ {
		block := popularBlocks[i]
		
		// Skip if already cached
		if _, exists := cbc.cache[block.BlockID]; exists {
			continue
		}
		
		// Create prefetched block entry (without actual data for now)
		// In a real implementation, this would fetch the data
		cachedBlock := &CachedCoverBlock{
			BlockID:         block.BlockID,
			Data:            []byte("prefetched_placeholder"), // Would be actual data
			Category:        block.Category,
			PopularityScore: block.PopularityScore,
			CachedAt:        time.Now(),
			LastAccessed:    time.Now(),
			AccessCount:     0,
			Size:            128 * 1024, // Assume 128KB blocks
			IsPrefetched:    true,
			ExpiresAt:       time.Now().Add(cbc.config.TTL),
		}
		
		// Check if we can fit this block
		if err := cbc.makeRoom(cachedBlock.Size); err != nil {
			break // Can't fit more blocks
		}
		
		// Add to cache
		cbc.cache[block.BlockID] = cachedBlock
		cbc.accessOrder = append(cbc.accessOrder, cachedBlock)
		cbc.totalSize += cachedBlock.Size
	}
	
	cbc.updateMetrics()
}

// makeRoom makes room in the cache for a new block
func (cbc *CoverBlockCache) makeRoom(neededSize int64) error {
	// Check if we need to make room
	for (cbc.totalSize+neededSize > cbc.config.MaxSize || len(cbc.cache) >= cbc.config.MaxBlocks) && len(cbc.accessOrder) > 0 {
		// Find block to evict based on strategy
		var toEvict *CachedCoverBlock
		
		switch cbc.config.CacheStrategy {
		case "LRU":
			toEvict = cbc.findLRU()
		case "LFU":
			toEvict = cbc.findLFU()
		case "TTL":
			toEvict = cbc.findExpired()
		case "Popular":
			toEvict = cbc.findLeastPopular()
		default:
			toEvict = cbc.findLRU()
		}
		
		if toEvict == nil {
			break
		}
		
		// Evict the block
		delete(cbc.cache, toEvict.BlockID)
		cbc.removeFromAccessOrder(toEvict)
		cbc.totalSize -= toEvict.Size
		cbc.metrics.Evictions++
	}
	
	return nil
}

// findLRU finds the least recently used block
func (cbc *CoverBlockCache) findLRU() *CachedCoverBlock {
	if len(cbc.accessOrder) == 0 {
		return nil
	}
	return cbc.accessOrder[0]
}

// findLFU finds the least frequently used block
func (cbc *CoverBlockCache) findLFU() *CachedCoverBlock {
	var lfu *CachedCoverBlock
	
	for _, block := range cbc.cache {
		if lfu == nil || block.AccessCount < lfu.AccessCount {
			lfu = block
		}
	}
	
	return lfu
}

// findExpired finds an expired block
func (cbc *CoverBlockCache) findExpired() *CachedCoverBlock {
	now := time.Now()
	
	for _, block := range cbc.cache {
		if !block.ExpiresAt.IsZero() && now.After(block.ExpiresAt) {
			return block
		}
	}
	
	return nil
}

// findLeastPopular finds the least popular block
func (cbc *CoverBlockCache) findLeastPopular() *CachedCoverBlock {
	var leastPopular *CachedCoverBlock
	
	for _, block := range cbc.cache {
		if leastPopular == nil || block.PopularityScore < leastPopular.PopularityScore {
			leastPopular = block
		}
	}
	
	return leastPopular
}

// updateAccessOrder updates the LRU access order
func (cbc *CoverBlockCache) updateAccessOrder(block *CachedCoverBlock) {
	// Remove from current position
	cbc.removeFromAccessOrder(block)
	
	// Add to end (most recently used)
	cbc.accessOrder = append(cbc.accessOrder, block)
}

// removeFromAccessOrder removes a block from the access order
func (cbc *CoverBlockCache) removeFromAccessOrder(block *CachedCoverBlock) {
	for i, b := range cbc.accessOrder {
		if b == block {
			cbc.accessOrder = append(cbc.accessOrder[:i], cbc.accessOrder[i+1:]...)
			break
		}
	}
}

// cleanupLoop periodically cleans up expired blocks
func (cbc *CoverBlockCache) cleanupLoop() {
	defer cbc.cleanupTicker.Stop()
	
	for {
		select {
		case <-cbc.ctx.Done():
			return
		case <-cbc.cleanupTicker.C:
			cbc.cleanup()
		}
	}
}

// cleanup removes expired blocks from the cache
func (cbc *CoverBlockCache) cleanup() {
	cbc.mu.Lock()
	defer cbc.mu.Unlock()
	
	now := time.Now()
	toRemove := make([]*CachedCoverBlock, 0)
	
	// Find expired blocks
	for _, block := range cbc.cache {
		if !block.ExpiresAt.IsZero() && now.After(block.ExpiresAt) {
			toRemove = append(toRemove, block)
		}
	}
	
	// Remove expired blocks
	for _, block := range toRemove {
		delete(cbc.cache, block.BlockID)
		cbc.removeFromAccessOrder(block)
		cbc.totalSize -= block.Size
	}
	
	cbc.updateMetrics()
}

// updateMetrics updates cache metrics
func (cbc *CoverBlockCache) updateMetrics() {
	cbc.metrics.TotalSize = cbc.totalSize
	cbc.metrics.BlockCount = int64(len(cbc.cache))
	
	if cbc.metrics.TotalRequests > 0 {
		cbc.metrics.HitRate = float64(cbc.metrics.CacheHits) / float64(cbc.metrics.TotalRequests)
	}
	
	if cbc.metrics.BlockCount > 0 {
		cbc.metrics.AverageBlockSize = float64(cbc.metrics.TotalSize) / float64(cbc.metrics.BlockCount)
	}
	
	cbc.metrics.LastUpdate = time.Now()
}

// GetCachedBlocks returns a list of all cached blocks
func (cbc *CoverBlockCache) GetCachedBlocks() []*CachedCoverBlock {
	cbc.mu.RLock()
	defer cbc.mu.RUnlock()
	
	blocks := make([]*CachedCoverBlock, 0, len(cbc.cache))
	for _, block := range cbc.cache {
		blocks = append(blocks, block)
	}
	
	return blocks
}

// GetCachedBlocksByCategory returns cached blocks for a specific category
func (cbc *CoverBlockCache) GetCachedBlocksByCategory(category BlockCategory) []*CachedCoverBlock {
	cbc.mu.RLock()
	defer cbc.mu.RUnlock()
	
	blocks := make([]*CachedCoverBlock, 0)
	for _, block := range cbc.cache {
		if block.Category == category {
			blocks = append(blocks, block)
		}
	}
	
	return blocks
}

// InvalidateBlock removes a specific block from the cache
func (cbc *CoverBlockCache) InvalidateBlock(blockID string) {
	cbc.mu.Lock()
	defer cbc.mu.Unlock()
	
	if block, exists := cbc.cache[blockID]; exists {
		delete(cbc.cache, blockID)
		cbc.removeFromAccessOrder(block)
		cbc.totalSize -= block.Size
		cbc.updateMetrics()
	}
}

// Clear removes all blocks from the cache
func (cbc *CoverBlockCache) Clear() {
	cbc.mu.Lock()
	defer cbc.mu.Unlock()
	
	cbc.cache = make(map[string]*CachedCoverBlock)
	cbc.accessOrder = make([]*CachedCoverBlock, 0)
	cbc.totalSize = 0
	cbc.updateMetrics()
}

// GetMetrics returns current cache metrics
func (cbc *CoverBlockCache) GetMetrics() *CoverCacheMetrics {
	cbc.mu.RLock()
	defer cbc.mu.RUnlock()
	return cbc.metrics
}

// GetStats returns detailed cache statistics
func (cbc *CoverBlockCache) GetStats() map[string]interface{} {
	cbc.mu.RLock()
	defer cbc.mu.RUnlock()
	
	stats := make(map[string]interface{})
	stats["total_blocks"] = len(cbc.cache)
	stats["total_size"] = cbc.totalSize
	stats["hit_rate"] = cbc.metrics.HitRate
	stats["cache_hits"] = cbc.metrics.CacheHits
	stats["cache_misses"] = cbc.metrics.CacheMisses
	stats["evictions"] = cbc.metrics.Evictions
	stats["prefetch_hits"] = cbc.metrics.PrefetchHits
	
	// Category distribution
	categoryStats := make(map[string]int)
	for _, block := range cbc.cache {
		categoryStats[string(block.Category)]++
	}
	stats["category_distribution"] = categoryStats
	
	return stats
}

// Stop stops the cover block cache
func (cbc *CoverBlockCache) Stop() {
	cbc.cancel()
}
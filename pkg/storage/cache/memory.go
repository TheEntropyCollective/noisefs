package cache

import (
	"container/list"
	"sync"
	
	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
)

// MemoryCache implements an in-memory LRU cache for blocks with performance optimizations
type MemoryCache struct {
	mu            sync.RWMutex
	capacity      int
	blocks        map[string]*cacheEntry
	lru           *list.List
	popularityMap map[string]int
	stats         Stats
	optimizer     *PerformanceOptimizer // Performance optimization engine
	memoryUsage   int64                // Current memory usage in bytes
	memoryLimit   int64                // Memory limit in bytes (0 = no limit)
}

type cacheEntry struct {
	cid     string
	block   *blocks.Block
	element *list.Element
}

// NewMemoryCache creates a new in-memory cache with specified capacity
func NewMemoryCache(capacity int) *MemoryCache {
	return &MemoryCache{
		capacity:      capacity,
		blocks:        make(map[string]*cacheEntry),
		lru:           list.New(),
		popularityMap: make(map[string]int),
		optimizer:     NewPerformanceOptimizer(),
		memoryLimit:   0, // No memory limit by default
	}
}

// NewMemoryCacheWithMemoryLimit creates a new in-memory cache with capacity and memory limits
func NewMemoryCacheWithMemoryLimit(capacity int, memoryLimitBytes int64) *MemoryCache {
	return &MemoryCache{
		capacity:      capacity,
		blocks:        make(map[string]*cacheEntry),
		lru:           list.New(),
		popularityMap: make(map[string]int),
		optimizer:     NewPerformanceOptimizer(),
		memoryLimit:   memoryLimitBytes,
	}
}

// Store adds a block to the cache
func (c *MemoryCache) Store(cid string, block *blocks.Block) error {
	if cid == "" || block == nil {
		return ErrNotFound
	}
	
	c.mu.Lock()
	defer c.mu.Unlock()
	
	// Check if block already exists
	if entry, exists := c.blocks[cid]; exists {
		// Move to front of LRU
		c.lru.MoveToFront(entry.element)
		return nil
	}
	
	blockSize := int64(block.Size())
	
	// Check memory limit before adding
	if c.memoryLimit > 0 && c.memoryUsage+blockSize > c.memoryLimit {
		c.evictToFitMemory(blockSize)
	}
	
	// Evict if at capacity
	if len(c.blocks) >= c.capacity && c.capacity > 0 {
		c.evictOldest()
	}
	
	// Add new entry
	element := c.lru.PushFront(cid)
	c.blocks[cid] = &cacheEntry{
		cid:     cid,
		block:   block,
		element: element,
	}
	
	// Update memory usage
	c.memoryUsage += blockSize
	
	return nil
}

// Get retrieves a block from the cache
func (c *MemoryCache) Get(cid string) (*blocks.Block, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	entry, exists := c.blocks[cid]
	if !exists {
		c.stats.Misses++
		return nil, ErrNotFound
	}
	
	// Move to front of LRU
	c.lru.MoveToFront(entry.element)
	c.stats.Hits++
	
	return entry.block, nil
}

// Has checks if a block exists in the cache
func (c *MemoryCache) Has(cid string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	_, exists := c.blocks[cid]
	return exists
}

// Remove removes a block from the cache
func (c *MemoryCache) Remove(cid string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	entry, exists := c.blocks[cid]
	if !exists {
		return ErrNotFound
	}
	
	c.lru.Remove(entry.element)
	delete(c.blocks, cid)
	delete(c.popularityMap, cid)
	
	return nil
}

// GetRandomizers returns popular blocks suitable as randomizers
func (c *MemoryCache) GetRandomizers(count int) ([]*BlockInfo, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	// Create slice of BlockInfo sorted by popularity
	blockInfos := make([]*BlockInfo, 0, len(c.blocks))
	for cid, entry := range c.blocks {
		popularity := c.popularityMap[cid]
		blockInfos = append(blockInfos, &BlockInfo{
			CID:        cid,
			Block:      entry.block,
			Size:       entry.block.Size(),
			Popularity: popularity,
		})
	}
	
	// Use optimized sorting algorithm (O(n + k*log(n)) vs O(n²))
	blockInfos = c.optimizer.GetTopNBlocks(blockInfos, count)
	
	// blockInfos already contains exactly the top N blocks from optimization
	return blockInfos, nil
}

// IncrementPopularity increases the popularity score of a block
func (c *MemoryCache) IncrementPopularity(cid string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if _, exists := c.blocks[cid]; !exists {
		return ErrNotFound
	}
	
	c.popularityMap[cid]++
	return nil
}

// Size returns the number of blocks in the cache
func (c *MemoryCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	return len(c.blocks)
}

// Clear removes all blocks from the cache
func (c *MemoryCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.blocks = make(map[string]*cacheEntry)
	c.lru = list.New()
	c.popularityMap = make(map[string]int)
	c.memoryUsage = 0
}

// evictOldest removes the least recently used block
func (c *MemoryCache) evictOldest() {
	oldest := c.lru.Back()
	if oldest != nil {
		cid := oldest.Value.(string)
		if entry, exists := c.blocks[cid]; exists {
			// Update memory usage
			c.memoryUsage -= int64(entry.block.Size())
		}
		c.lru.Remove(oldest)
		delete(c.blocks, cid)
		delete(c.popularityMap, cid)
		c.stats.Evictions++
	}
}

// evictToFitMemory evicts blocks until there's enough memory for a new block
func (c *MemoryCache) evictToFitMemory(requiredBytes int64) {
	for c.memoryUsage+requiredBytes > c.memoryLimit && c.lru.Len() > 0 {
		c.evictOldest()
	}
}

// GetMemoryUsage returns current memory usage statistics
func (c *MemoryCache) GetMemoryUsage() (current, limit int64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.memoryUsage, c.memoryLimit
}

// GetPerformanceStats returns enhanced cache performance metrics
func (c *MemoryCache) GetPerformanceStats() CacheStatistics {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return calculateCacheStatistics(c.stats, c.memoryUsage, c.memoryLimit)
}

// GetStats returns cache statistics
func (c *MemoryCache) GetStats() *Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	// Calculate hit rate
	var hitRate float64
	if c.stats.Hits+c.stats.Misses > 0 {
		hitRate = float64(c.stats.Hits) / float64(c.stats.Hits+c.stats.Misses)
	}
	
	// Create a copy to avoid data races
	return &Stats{
		Hits:      c.stats.Hits,
		Misses:    c.stats.Misses,
		Evictions: c.stats.Evictions,
		Size:      len(c.blocks),
		HitRate:   hitRate,
	}
}
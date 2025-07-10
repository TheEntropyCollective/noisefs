package cache

import (
	"container/list"
	"sync"
	
	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
)

// MemoryCache implements an in-memory LRU cache for blocks
type MemoryCache struct {
	mu          sync.RWMutex
	capacity    int
	blocks      map[string]*cacheEntry
	lru         *list.List
	popularityMap map[string]int
	stats       Stats
}

type cacheEntry struct {
	cid     string
	block   *blocks.Block
	element *list.Element
}

// NewMemoryCache creates a new in-memory cache with specified capacity
func NewMemoryCache(capacity int) *MemoryCache {
	return &MemoryCache{
		capacity:    capacity,
		blocks:      make(map[string]*cacheEntry),
		lru:         list.New(),
		popularityMap: make(map[string]int),
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
	
	// Sort by popularity (simple selection for now)
	for i := 0; i < len(blockInfos); i++ {
		for j := i + 1; j < len(blockInfos); j++ {
			if blockInfos[j].Popularity > blockInfos[i].Popularity {
				blockInfos[i], blockInfos[j] = blockInfos[j], blockInfos[i]
			}
		}
	}
	
	// Return top N blocks
	if count > len(blockInfos) {
		count = len(blockInfos)
	}
	
	return blockInfos[:count], nil
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
}

// evictOldest removes the least recently used block
func (c *MemoryCache) evictOldest() {
	oldest := c.lru.Back()
	if oldest != nil {
		cid := oldest.Value.(string)
		c.lru.Remove(oldest)
		delete(c.blocks, cid)
		delete(c.popularityMap, cid)
		c.stats.Evictions++
	}
}

// GetStats returns cache statistics
func (c *MemoryCache) GetStats() *Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	// Create a copy to avoid data races
	return &Stats{
		Hits:      c.stats.Hits,
		Misses:    c.stats.Misses,
		Evictions: c.stats.Evictions,
		Size:      len(c.blocks),
	}
}
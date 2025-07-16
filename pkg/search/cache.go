package search

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

// CacheEntry represents a cached search result with TTL
type CacheEntry struct {
	Results   *SearchResults
	CreatedAt time.Time
	TTL       time.Duration
}

// IsExpired checks if the cache entry has expired
func (e *CacheEntry) IsExpired() bool {
	return time.Since(e.CreatedAt) > e.TTL
}

// SearchResultCache implements an LRU cache with TTL for search results
type SearchResultCache struct {
	mu       sync.RWMutex
	entries  map[string]*CacheEntry
	order    []string // LRU order (most recent at end)
	maxSize  int
	defaultTTL time.Duration
}

// NewSearchResultCache creates a new search result cache
func NewSearchResultCache(maxSize int, defaultTTL time.Duration) *SearchResultCache {
	return &SearchResultCache{
		entries:    make(map[string]*CacheEntry),
		order:      make([]string, 0),
		maxSize:    maxSize,
		defaultTTL: defaultTTL,
	}
}

// Get retrieves a cached search result
func (c *SearchResultCache) Get(key string) (*SearchResults, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.entries[key]
	if !exists {
		return nil, false
	}

	// Check if expired
	if entry.IsExpired() {
		delete(c.entries, key)
		c.removeFromOrder(key)
		return nil, false
	}

	// Move to end (most recently used)
	c.moveToEnd(key)
	
	return entry.Results, true
}

// Put stores a search result in the cache
func (c *SearchResultCache) Put(key string, results *SearchResults) {
	c.PutWithTTL(key, results, c.defaultTTL)
}

// PutWithTTL stores a search result with custom TTL
func (c *SearchResultCache) PutWithTTL(key string, results *SearchResults, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Create new entry
	entry := &CacheEntry{
		Results:   results,
		CreatedAt: time.Now(),
		TTL:       ttl,
	}

	// If key already exists, update it
	if _, exists := c.entries[key]; exists {
		c.entries[key] = entry
		c.moveToEnd(key)
		return
	}

	// Check if we need to evict
	if len(c.entries) >= c.maxSize {
		c.evictLRU()
	}

	// Add new entry
	c.entries[key] = entry
	c.order = append(c.order, key)
}

// Clear removes all entries from the cache
func (c *SearchResultCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*CacheEntry)
	c.order = c.order[:0]
}

// Size returns the current number of entries
func (c *SearchResultCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// CleanExpired removes all expired entries
func (c *SearchResultCache) CleanExpired() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	removed := 0
	for key, entry := range c.entries {
		if entry.IsExpired() {
			delete(c.entries, key)
			c.removeFromOrder(key)
			removed++
		}
	}

	return removed
}

// GetStats returns cache statistics
func (c *SearchResultCache) GetStats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := CacheStats{
		Size:     len(c.entries),
		MaxSize:  c.maxSize,
		Capacity: float64(len(c.entries)) / float64(c.maxSize) * 100,
	}

	// Count expired entries
	for _, entry := range c.entries {
		if entry.IsExpired() {
			stats.Expired++
		}
	}

	return stats
}

// moveToEnd moves key to end of order slice (most recently used)
func (c *SearchResultCache) moveToEnd(key string) {
	// Remove from current position
	c.removeFromOrder(key)
	// Add to end
	c.order = append(c.order, key)
}

// removeFromOrder removes key from order slice
func (c *SearchResultCache) removeFromOrder(key string) {
	for i, k := range c.order {
		if k == key {
			c.order = append(c.order[:i], c.order[i+1:]...)
			break
		}
	}
}

// evictLRU removes the least recently used entry
func (c *SearchResultCache) evictLRU() {
	if len(c.order) == 0 {
		return
	}

	// Remove oldest (first in order)
	lruKey := c.order[0]
	delete(c.entries, lruKey)
	c.order = c.order[1:]
}

// GenerateCacheKey creates a cache key from search parameters
func GenerateCacheKey(query string, options SearchOptions) string {
	// Create a deterministic key from search parameters
	keyData := struct {
		Query       string        `json:"query"`
		Options     SearchOptions `json:"options"`
	}{
		Query:   query,
		Options: options,
	}

	data, _ := json.Marshal(keyData)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// GenerateMetadataCacheKey creates a cache key from metadata filters
func GenerateMetadataCacheKey(filters MetadataFilters) string {
	data, _ := json.Marshal(filters)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// CacheStats provides cache statistics
type CacheStats struct {
	Size     int     `json:"size"`
	MaxSize  int     `json:"max_size"`
	Capacity float64 `json:"capacity_percent"`
	Expired  int     `json:"expired_entries"`
}

// SearchCacheConfig configures the search result cache
type SearchCacheConfig struct {
	Enabled         bool          `json:"enabled"`
	MaxSize         int           `json:"max_size"`
	DefaultTTL      time.Duration `json:"default_ttl"`
	CleanupInterval time.Duration `json:"cleanup_interval"`
}

// DefaultSearchCacheConfig returns default cache configuration
func DefaultSearchCacheConfig() SearchCacheConfig {
	return SearchCacheConfig{
		Enabled:         true,
		MaxSize:         1000,
		DefaultTTL:      15 * time.Minute,
		CleanupInterval: 5 * time.Minute,
	}
}
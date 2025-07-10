package cache

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/logging"
)

// CacheStats tracks comprehensive cache performance metrics
type CacheStats struct {
	mu                    sync.RWMutex
	
	// Hit/Miss statistics
	Hits                  int64     `json:"hits"`
	Misses                int64     `json:"misses"`
	TotalRequests         int64     `json:"total_requests"`
	HitRate               float64   `json:"hit_rate"`
	
	// Storage statistics
	Stores                int64     `json:"stores"`
	Removals              int64     `json:"removals"`
	Evictions             int64     `json:"evictions"`
	CurrentSize           int64     `json:"current_size"`
	MaxSize               int64     `json:"max_size"`
	
	// Byte statistics
	BytesStored           int64     `json:"bytes_stored"`
	BytesRetrieved        int64     `json:"bytes_retrieved"`
	BytesEvicted          int64     `json:"bytes_evicted"`
	
	// Performance statistics
	AvgGetLatency         time.Duration `json:"avg_get_latency"`
	AvgStoreLatency       time.Duration `json:"avg_store_latency"`
	TotalGetLatency       time.Duration `json:"total_get_latency"`
	TotalStoreLatency     time.Duration `json:"total_store_latency"`
	
	// Time-based statistics
	StartTime             time.Time `json:"start_time"`
	LastReset             time.Time `json:"last_reset"`
	
	// Popular blocks
	PopularBlocks         map[string]int64 `json:"popular_blocks"`
	MostPopularCID        string           `json:"most_popular_cid"`
	MostPopularCount      int64            `json:"most_popular_count"`
}

// NewCacheStats creates a new cache statistics tracker
func NewCacheStats() *CacheStats {
	return &CacheStats{
		StartTime:     time.Now(),
		LastReset:     time.Now(),
		PopularBlocks: make(map[string]int64),
	}
}

// RecordHit records a cache hit
func (s *CacheStats) RecordHit(cid string, latency time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.Hits++
	s.TotalRequests++
	s.TotalGetLatency += latency
	
	// Update hit rate
	if s.TotalRequests > 0 {
		s.HitRate = float64(s.Hits) / float64(s.TotalRequests)
	}
	
	// Update average latency
	if s.Hits > 0 {
		s.AvgGetLatency = s.TotalGetLatency / time.Duration(s.Hits)
	}
	
	// Update popularity
	s.PopularBlocks[cid]++
	if s.PopularBlocks[cid] > s.MostPopularCount {
		s.MostPopularCID = cid
		s.MostPopularCount = s.PopularBlocks[cid]
	}
}

// RecordMiss records a cache miss
func (s *CacheStats) RecordMiss(cid string, latency time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.Misses++
	s.TotalRequests++
	s.TotalGetLatency += latency
	
	// Update hit rate
	if s.TotalRequests > 0 {
		s.HitRate = float64(s.Hits) / float64(s.TotalRequests)
	}
	
	// Update average latency (includes misses)
	if s.TotalRequests > 0 {
		s.AvgGetLatency = s.TotalGetLatency / time.Duration(s.TotalRequests)
	}
}

// RecordStore records a cache store operation
func (s *CacheStats) RecordStore(cid string, block *blocks.Block, latency time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.Stores++
	s.CurrentSize++
	s.BytesStored += int64(block.Size())
	s.TotalStoreLatency += latency
	
	// Update average store latency
	if s.Stores > 0 {
		s.AvgStoreLatency = s.TotalStoreLatency / time.Duration(s.Stores)
	}
}

// RecordRemoval records a cache removal operation
func (s *CacheStats) RecordRemoval(cid string, blockSize int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.Removals++
	s.CurrentSize--
	
	// Remove from popularity tracking
	delete(s.PopularBlocks, cid)
	
	// Recalculate most popular if needed
	if cid == s.MostPopularCID {
		s.recalculateMostPopular()
	}
}

// RecordEviction records a cache eviction operation
func (s *CacheStats) RecordEviction(cid string, blockSize int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.Evictions++
	s.CurrentSize--
	s.BytesEvicted += blockSize
	
	// Remove from popularity tracking
	delete(s.PopularBlocks, cid)
	
	// Recalculate most popular if needed
	if cid == s.MostPopularCID {
		s.recalculateMostPopular()
	}
}

// recalculateMostPopular recalculates the most popular block
func (s *CacheStats) recalculateMostPopular() {
	s.MostPopularCID = ""
	s.MostPopularCount = 0
	
	for cid, count := range s.PopularBlocks {
		if count > s.MostPopularCount {
			s.MostPopularCID = cid
			s.MostPopularCount = count
		}
	}
}

// SetMaxSize sets the maximum cache size
func (s *CacheStats) SetMaxSize(maxSize int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.MaxSize = maxSize
}

// GetSnapshot returns a snapshot of current statistics
func (s *CacheStats) GetSnapshot() CacheStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	// Create a copy of the stats
	snapshot := *s
	
	// Deep copy the popular blocks map
	snapshot.PopularBlocks = make(map[string]int64)
	for cid, count := range s.PopularBlocks {
		snapshot.PopularBlocks[cid] = count
	}
	
	return snapshot
}

// Reset resets all statistics
func (s *CacheStats) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.Hits = 0
	s.Misses = 0
	s.TotalRequests = 0
	s.HitRate = 0
	s.Stores = 0
	s.Removals = 0
	s.Evictions = 0
	s.BytesStored = 0
	s.BytesRetrieved = 0
	s.BytesEvicted = 0
	s.TotalGetLatency = 0
	s.TotalStoreLatency = 0
	s.AvgGetLatency = 0
	s.AvgStoreLatency = 0
	s.PopularBlocks = make(map[string]int64)
	s.MostPopularCID = ""
	s.MostPopularCount = 0
	s.LastReset = time.Now()
}

// PrintStats prints formatted statistics
func (s *CacheStats) PrintStats() {
	snapshot := s.GetSnapshot()
	
	fmt.Println("=== Cache Statistics ===")
	fmt.Printf("Uptime: %v\n", time.Since(snapshot.StartTime))
	fmt.Printf("Last Reset: %v ago\n", time.Since(snapshot.LastReset))
	fmt.Println()
	
	fmt.Printf("Hit Rate: %.2f%% (%d hits, %d misses)\n", 
		snapshot.HitRate*100, snapshot.Hits, snapshot.Misses)
	fmt.Printf("Total Requests: %d\n", snapshot.TotalRequests)
	fmt.Println()
	
	fmt.Printf("Cache Size: %d / %d (%.1f%% full)\n", 
		snapshot.CurrentSize, snapshot.MaxSize, 
		float64(snapshot.CurrentSize)/float64(snapshot.MaxSize)*100)
	fmt.Printf("Stores: %d, Removals: %d, Evictions: %d\n", 
		snapshot.Stores, snapshot.Removals, snapshot.Evictions)
	fmt.Println()
	
	fmt.Printf("Data Transfer:\n")
	fmt.Printf("  Stored: %s\n", formatBytes(snapshot.BytesStored))
	fmt.Printf("  Retrieved: %s\n", formatBytes(snapshot.BytesRetrieved))
	fmt.Printf("  Evicted: %s\n", formatBytes(snapshot.BytesEvicted))
	fmt.Println()
	
	fmt.Printf("Average Latencies:\n")
	fmt.Printf("  Get: %v\n", snapshot.AvgGetLatency)
	fmt.Printf("  Store: %v\n", snapshot.AvgStoreLatency)
	fmt.Println()
	
	if snapshot.MostPopularCID != "" {
		fmt.Printf("Most Popular Block: %s (%d accesses)\n", 
			snapshot.MostPopularCID[:min(16, len(snapshot.MostPopularCID))], 
			snapshot.MostPopularCount)
	}
}

// ToJSON returns statistics as JSON
func (s *CacheStats) ToJSON() ([]byte, error) {
	snapshot := s.GetSnapshot()
	return json.MarshalIndent(snapshot, "", "  ")
}

// StatisticsCache wraps a cache with statistics tracking
type StatisticsCache struct {
	underlying Cache
	stats      *CacheStats
	logger     *logging.Logger
}

// NewStatisticsCache creates a new cache with statistics tracking
func NewStatisticsCache(underlying Cache, logger *logging.Logger) *StatisticsCache {
	return &StatisticsCache{
		underlying: underlying,
		stats:      NewCacheStats(),
		logger:     logger,
	}
}

// Store adds a block to the cache with statistics tracking
func (c *StatisticsCache) Store(cid string, block *blocks.Block) error {
	start := time.Now()
	
	err := c.underlying.Store(cid, block)
	latency := time.Since(start)
	
	if err == nil {
		c.stats.RecordStore(cid, block, latency)
	}
	
	return err
}

// Get retrieves a block from the cache with statistics tracking
func (c *StatisticsCache) Get(cid string) (*blocks.Block, error) {
	start := time.Now()
	
	block, err := c.underlying.Get(cid)
	latency := time.Since(start)
	
	if err == nil {
		c.stats.RecordHit(cid, latency)
		c.stats.BytesRetrieved += int64(block.Size())
	} else {
		c.stats.RecordMiss(cid, latency)
	}
	
	return block, err
}

// Has checks if a block exists in the cache
func (c *StatisticsCache) Has(cid string) bool {
	return c.underlying.Has(cid)
}

// Remove removes a block from the cache with statistics tracking
func (c *StatisticsCache) Remove(cid string) error {
	// Try to get block size before removal
	var blockSize int64
	if block, err := c.underlying.Get(cid); err == nil {
		blockSize = int64(block.Size())
	}
	
	err := c.underlying.Remove(cid)
	if err == nil {
		c.stats.RecordRemoval(cid, blockSize)
	}
	
	return err
}

// GetRandomizers returns popular blocks suitable as randomizers
func (c *StatisticsCache) GetRandomizers(count int) ([]*BlockInfo, error) {
	return c.underlying.GetRandomizers(count)
}

// IncrementPopularity increases the popularity score of a block
func (c *StatisticsCache) IncrementPopularity(cid string) error {
	return c.underlying.IncrementPopularity(cid)
}

// Size returns the number of blocks in the cache
func (c *StatisticsCache) Size() int {
	size := c.underlying.Size()
	c.stats.SetMaxSize(int64(size)) // Update current size
	return size
}

// Clear removes all blocks from the cache
func (c *StatisticsCache) Clear() {
	c.underlying.Clear()
	c.stats.Reset()
}

// GetStats returns the current cache statistics
func (c *StatisticsCache) GetStats() *CacheStats {
	return c.stats
}

// LogStats logs current statistics
func (c *StatisticsCache) LogStats() {
	snapshot := c.stats.GetSnapshot()
	
	c.logger.Info("Cache statistics", map[string]interface{}{
		"hit_rate":          snapshot.HitRate,
		"total_requests":    snapshot.TotalRequests,
		"hits":              snapshot.Hits,
		"misses":            snapshot.Misses,
		"current_size":      snapshot.CurrentSize,
		"max_size":          snapshot.MaxSize,
		"stores":            snapshot.Stores,
		"evictions":         snapshot.Evictions,
		"bytes_stored":      snapshot.BytesStored,
		"bytes_retrieved":   snapshot.BytesRetrieved,
		"avg_get_latency":   snapshot.AvgGetLatency.String(),
		"avg_store_latency": snapshot.AvgStoreLatency.String(),
		"most_popular_cid":  snapshot.MostPopularCID,
		"most_popular_count": snapshot.MostPopularCount,
	})
}

// formatBytes formats byte counts in human-readable format
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
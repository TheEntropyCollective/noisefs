package cache

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/common/logging"
)

// SampledStatsConfig configures the sampling behavior
type SampledStatsConfig struct {
	// SampleRate is the probability of recording a sample (0.0-1.0)
	// Default: 0.1 (10% sampling rate)
	SampleRate float64 `json:"sample_rate"`
	
	// PopularitySampleRate is the rate for popularity tracking
	// Default: 0.05 (5% sampling rate for popularity)
	PopularitySampleRate float64 `json:"popularity_sample_rate"`
	
	// LatencySampleRate is the rate for latency measurements
	// Default: 0.1 (10% sampling rate for latency)
	LatencySampleRate float64 `json:"latency_sample_rate"`
	
	// MinSampleInterval prevents too frequent sampling
	// Default: 100ms
	MinSampleInterval time.Duration `json:"min_sample_interval"`
	
	// UseApproximation enables probabilistic data structures
	UseApproximation bool `json:"use_approximation"`
	
	// MaxPopularBlocks limits the popular blocks map size
	MaxPopularBlocks int `json:"max_popular_blocks"`
}

// DefaultSampledStatsConfig returns sensible defaults
func DefaultSampledStatsConfig() *SampledStatsConfig {
	return &SampledStatsConfig{
		SampleRate:           0.1,  // 10% sampling
		PopularitySampleRate: 0.05, // 5% sampling for popularity
		LatencySampleRate:    0.1,  // 10% sampling for latency
		MinSampleInterval:    100 * time.Millisecond,
		UseApproximation:     true,
		MaxPopularBlocks:     1000, // Cap popular blocks map size
	}
}

// SampledCacheStats provides high-performance statistics with sampling
type SampledCacheStats struct {
	mu     sync.RWMutex
	config *SampledStatsConfig
	
	// Atomic counters for high-frequency operations
	hits          int64
	misses        int64
	stores        int64
	removals      int64
	evictions     int64
	currentSize   int64
	
	// Sampled statistics
	bytesStored      int64
	bytesRetrieved   int64
	bytesEvicted     int64
	maxSize          int64
	
	// Latency tracking (sampled)
	totalGetLatency   time.Duration
	totalStoreLatency time.Duration
	latencySamples    int64
	
	// Popularity tracking (sampled and capped)
	popularBlocks     map[string]int64
	mostPopularCID    string
	mostPopularCount  int64
	
	// Timing
	startTime    time.Time
	lastReset    time.Time
	
	// Sampling state (counter-based for performance)
	// Using global atomic counter instead of per-instance RNG
}

// NewSampledCacheStats creates a new sampled statistics tracker
func NewSampledCacheStats(config *SampledStatsConfig) *SampledCacheStats {
	if config == nil {
		config = DefaultSampledStatsConfig()
	}
	
	return &SampledCacheStats{
		config:        config,
		startTime:     time.Now(),
		lastReset:     time.Now(),
		popularBlocks: make(map[string]int64),
	}
}

// Counter-based sampling for better performance
var samplingCounter int64

// shouldSample determines if we should sample this operation using fast counter-based sampling
func (s *SampledCacheStats) shouldSample(rate float64) bool {
	if rate >= 1.0 {
		return true
	}
	if rate <= 0.0 {
		return false
	}
	
	// Use atomic counter-based sampling instead of RNG for better performance
	counter := atomic.AddInt64(&samplingCounter, 1)
	
	// Convert rate to interval (e.g., 0.1 = every 10th operation)
	interval := int64(1.0 / rate)
	if interval <= 1 {
		return true
	}
	
	return counter%interval == 0
}

// RecordHit records a cache hit with sampling
func (s *SampledCacheStats) RecordHit(cid string, latency time.Duration) {
	// Always increment atomic counters
	atomic.AddInt64(&s.hits, 1)
	
	// Sample latency measurements
	if s.shouldSample(s.config.LatencySampleRate) {
		s.mu.Lock()
		s.totalGetLatency += latency
		s.latencySamples++
		s.mu.Unlock()
	}
	
	// Sample popularity tracking
	if s.shouldSample(s.config.PopularitySampleRate) {
		s.mu.Lock()
		s.updatePopularity(cid)
		s.mu.Unlock()
	}
}

// RecordMiss records a cache miss with sampling
func (s *SampledCacheStats) RecordMiss(cid string, latency time.Duration) {
	// Always increment atomic counters
	atomic.AddInt64(&s.misses, 1)
	
	// Sample latency measurements
	if s.shouldSample(s.config.LatencySampleRate) {
		s.mu.Lock()
		s.totalGetLatency += latency
		s.latencySamples++
		s.mu.Unlock()
	}
}

// RecordStore records a cache store operation with sampling
func (s *SampledCacheStats) RecordStore(cid string, block *blocks.Block, latency time.Duration) {
	// Always increment atomic counters
	atomic.AddInt64(&s.stores, 1)
	atomic.AddInt64(&s.currentSize, 1)
	
	// Sample byte tracking and latency
	if s.shouldSample(s.config.SampleRate) {
		s.mu.Lock()
		s.bytesStored += int64(block.Size())
		s.totalStoreLatency += latency
		s.mu.Unlock()
	}
}

// RecordRemoval records a cache removal operation
func (s *SampledCacheStats) RecordRemoval(cid string, blockSize int64) {
	atomic.AddInt64(&s.removals, 1)
	atomic.AddInt64(&s.currentSize, -1)
	
	// Remove from popularity tracking if present
	s.mu.Lock()
	delete(s.popularBlocks, cid)
	// Note: We don't recalculate most popular on every removal for performance
	s.mu.Unlock()
}

// RecordEviction records a cache eviction operation
func (s *SampledCacheStats) RecordEviction(cid string, blockSize int64) {
	atomic.AddInt64(&s.evictions, 1)
	atomic.AddInt64(&s.currentSize, -1)
	
	// Sample byte tracking
	if s.shouldSample(s.config.SampleRate) {
		s.mu.Lock()
		s.bytesEvicted += blockSize
		s.mu.Unlock()
	}
	
	// Remove from popularity tracking if present
	s.mu.Lock()
	delete(s.popularBlocks, cid)
	s.mu.Unlock()
}

// updatePopularity updates popularity tracking with size limits
func (s *SampledCacheStats) updatePopularity(cid string) {
	// Scale the increment to account for sampling rate
	increment := int64(1.0 / s.config.PopularitySampleRate)
	
	s.popularBlocks[cid] += increment
	
	// Check if this becomes the most popular
	if s.popularBlocks[cid] > s.mostPopularCount {
		s.mostPopularCID = cid
		s.mostPopularCount = s.popularBlocks[cid]
	}
	
	// Limit the size of popular blocks map
	if len(s.popularBlocks) > s.config.MaxPopularBlocks {
		s.trimPopularBlocks()
	}
}

// trimPopularBlocks removes least popular blocks when map gets too large
func (s *SampledCacheStats) trimPopularBlocks() {
	// Find the median popularity
	counts := make([]int64, 0, len(s.popularBlocks))
	for _, count := range s.popularBlocks {
		counts = append(counts, count)
	}
	
	// Simple approach: remove blocks with count <= 1
	for cid, count := range s.popularBlocks {
		if count <= 1 && len(s.popularBlocks) > s.config.MaxPopularBlocks/2 {
			delete(s.popularBlocks, cid)
		}
	}
	
	// If still too large, remove half randomly
	if len(s.popularBlocks) > s.config.MaxPopularBlocks {
		toRemove := len(s.popularBlocks) - s.config.MaxPopularBlocks/2
		removed := 0
		for cid := range s.popularBlocks {
			if removed >= toRemove {
				break
			}
			if cid != s.mostPopularCID { // Don't remove the most popular
				delete(s.popularBlocks, cid)
				removed++
			}
		}
	}
}

// GetSnapshot returns a snapshot of current statistics
func (s *SampledCacheStats) GetSnapshot() CacheStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	// Load atomic values
	hits := atomic.LoadInt64(&s.hits)
	misses := atomic.LoadInt64(&s.misses)
	stores := atomic.LoadInt64(&s.stores)
	removals := atomic.LoadInt64(&s.removals)
	evictions := atomic.LoadInt64(&s.evictions)
	currentSize := atomic.LoadInt64(&s.currentSize)
	
	totalRequests := hits + misses
	var hitRate float64
	if totalRequests > 0 {
		hitRate = float64(hits) / float64(totalRequests)
	}
	
	// Calculate average latencies from samples
	var avgGetLatency, avgStoreLatency time.Duration
	if s.latencySamples > 0 {
		avgGetLatency = s.totalGetLatency / time.Duration(s.latencySamples)
	}
	if stores > 0 && s.totalStoreLatency > 0 {
		avgStoreLatency = s.totalStoreLatency / time.Duration(stores)
	}
	
	// Deep copy popular blocks
	popularBlocks := make(map[string]int64)
	for cid, count := range s.popularBlocks {
		popularBlocks[cid] = count
	}
	
	return CacheStats{
		Hits:                hits,
		Misses:              misses,
		TotalRequests:       totalRequests,
		HitRate:             hitRate,
		Stores:              stores,
		Removals:            removals,
		Evictions:           evictions,
		CurrentSize:         currentSize,
		MaxSize:             s.maxSize,
		BytesStored:         s.bytesStored,
		BytesRetrieved:      s.bytesRetrieved,
		BytesEvicted:        s.bytesEvicted,
		AvgGetLatency:       avgGetLatency,
		AvgStoreLatency:     avgStoreLatency,
		TotalGetLatency:     s.totalGetLatency,
		TotalStoreLatency:   s.totalStoreLatency,
		StartTime:           s.startTime,
		LastReset:           s.lastReset,
		PopularBlocks:       popularBlocks,
		MostPopularCID:      s.mostPopularCID,
		MostPopularCount:    s.mostPopularCount,
	}
}

// SetMaxSize sets the maximum cache size
func (s *SampledCacheStats) SetMaxSize(maxSize int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maxSize = maxSize
}

// Reset resets all statistics
func (s *SampledCacheStats) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Reset atomic counters
	atomic.StoreInt64(&s.hits, 0)
	atomic.StoreInt64(&s.misses, 0)
	atomic.StoreInt64(&s.stores, 0)
	atomic.StoreInt64(&s.removals, 0)
	atomic.StoreInt64(&s.evictions, 0)
	atomic.StoreInt64(&s.currentSize, 0)
	
	// Reset sampled data
	s.bytesStored = 0
	s.bytesRetrieved = 0
	s.bytesEvicted = 0
	s.totalGetLatency = 0
	s.totalStoreLatency = 0
	s.latencySamples = 0
	s.popularBlocks = make(map[string]int64)
	s.mostPopularCID = ""
	s.mostPopularCount = 0
	s.lastReset = time.Now()
}

// GetEfficiency returns sampling efficiency metrics
func (s *SampledCacheStats) GetEfficiency() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	totalOps := atomic.LoadInt64(&s.hits) + atomic.LoadInt64(&s.misses) + atomic.LoadInt64(&s.stores)
	
	return map[string]interface{}{
		"total_operations":       totalOps,
		"latency_samples":        s.latencySamples,
		"popular_blocks_tracked": len(s.popularBlocks),
		"sample_rate":            s.config.SampleRate,
		"popularity_sample_rate": s.config.PopularitySampleRate,
		"latency_sample_rate":    s.config.LatencySampleRate,
		"sampling_efficiency":    float64(s.latencySamples) / float64(totalOps+1) * 100,
	}
}

// SampledStatisticsCache wraps a cache with sampled statistics tracking
type SampledStatisticsCache struct {
	underlying Cache
	stats      *SampledCacheStats
	logger     *logging.Logger
}

// NewSampledStatisticsCache creates a new cache with sampled statistics tracking
func NewSampledStatisticsCache(underlying Cache, config *SampledStatsConfig, logger *logging.Logger) *SampledStatisticsCache {
	return &SampledStatisticsCache{
		underlying: underlying,
		stats:      NewSampledCacheStats(config),
		logger:     logger,
	}
}

// Store adds a block to the cache with sampled statistics tracking
func (c *SampledStatisticsCache) Store(cid string, block *blocks.Block) error {
	start := time.Now()
	
	err := c.underlying.Store(cid, block)
	latency := time.Since(start)
	
	if err == nil {
		c.stats.RecordStore(cid, block, latency)
	}
	
	return err
}

// Get retrieves a block from the cache with sampled statistics tracking
func (c *SampledStatisticsCache) Get(cid string) (*blocks.Block, error) {
	start := time.Now()
	
	block, err := c.underlying.Get(cid)
	latency := time.Since(start)
	
	if err == nil {
		c.stats.RecordHit(cid, latency)
		// Sample bytes retrieved
		if c.stats.shouldSample(c.stats.config.SampleRate) {
			c.stats.mu.Lock()
			c.stats.bytesRetrieved += int64(block.Size())
			c.stats.mu.Unlock()
		}
	} else {
		c.stats.RecordMiss(cid, latency)
	}
	
	return block, err
}

// Has checks if a block exists in the cache
func (c *SampledStatisticsCache) Has(cid string) bool {
	return c.underlying.Has(cid)
}

// Remove removes a block from the cache with sampled statistics tracking
func (c *SampledStatisticsCache) Remove(cid string) error {
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
func (c *SampledStatisticsCache) GetRandomizers(count int) ([]*BlockInfo, error) {
	return c.underlying.GetRandomizers(count)
}

// IncrementPopularity increases the popularity score of a block
func (c *SampledStatisticsCache) IncrementPopularity(cid string) error {
	return c.underlying.IncrementPopularity(cid)
}

// Size returns the number of blocks in the cache
func (c *SampledStatisticsCache) Size() int {
	size := c.underlying.Size()
	c.stats.SetMaxSize(int64(size))
	return size
}

// Clear removes all blocks from the cache
func (c *SampledStatisticsCache) Clear() {
	c.underlying.Clear()
	c.stats.Reset()
}

// GetStats returns the current cache statistics
func (c *SampledStatisticsCache) GetStats() *Stats {
	snapshot := c.stats.GetSnapshot()
	
	// Calculate hit rate
	var hitRate float64
	if snapshot.Hits+snapshot.Misses > 0 {
		hitRate = float64(snapshot.Hits) / float64(snapshot.Hits+snapshot.Misses)
	}
	
	return &Stats{
		Hits:      snapshot.Hits,
		Misses:    snapshot.Misses,
		Evictions: snapshot.Evictions,
		Size:      int(snapshot.CurrentSize),
		HitRate:   hitRate,
	}
}

// GetSampledStats returns the full sampled statistics
func (c *SampledStatisticsCache) GetSampledStats() CacheStats {
	return c.stats.GetSnapshot()
}

// GetEfficiencyStats returns sampling efficiency metrics
func (c *SampledStatisticsCache) GetEfficiencyStats() map[string]interface{} {
	return c.stats.GetEfficiency()
}

// LogStats logs current statistics
func (c *SampledStatisticsCache) LogStats() {
	snapshot := c.stats.GetSnapshot()
	efficiency := c.stats.GetEfficiency()
	
	c.logger.Info("Sampled cache statistics", map[string]interface{}{
		"hit_rate":           snapshot.HitRate,
		"total_requests":     snapshot.TotalRequests,
		"hits":               snapshot.Hits,
		"misses":             snapshot.Misses,
		"current_size":       snapshot.CurrentSize,
		"max_size":           snapshot.MaxSize,
		"stores":             snapshot.Stores,
		"evictions":          snapshot.Evictions,
		"bytes_stored":       snapshot.BytesStored,
		"bytes_retrieved":    snapshot.BytesRetrieved,
		"avg_get_latency":    snapshot.AvgGetLatency.String(),
		"avg_store_latency":  snapshot.AvgStoreLatency.String(),
		"most_popular_cid":   snapshot.MostPopularCID,
		"sampling_efficiency": efficiency["sampling_efficiency"],
		"latency_samples":    efficiency["latency_samples"],
	})
}
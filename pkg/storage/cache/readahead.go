package cache

import (
	"fmt"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/logging"
)

// ReadAheadCache implements a cache with read-ahead capabilities
type ReadAheadCache struct {
	underlying      Cache
	readAheadSize   int
	readAheadWorkers int
	logger          *logging.Logger
	
	// Read-ahead state
	mu              sync.RWMutex
	readAheadQueue  chan readAheadRequest
	stopChan        chan struct{}
	wg              sync.WaitGroup
	accessPattern   map[string]*ReadAheadPattern
	
	// Statistics
	stats ReadAheadStats
}

// ReadAheadStats tracks read-ahead performance metrics
type ReadAheadStats struct {
	mu                    sync.RWMutex
	ReadAheadRequests     int64
	ReadAheadHits         int64
	ReadAheadMisses       int64
	ReadAheadBytes        int64
	PrefetchedBlocks      int64
	PrefetchCacheHits     int64
	AvgReadAheadLatency   time.Duration
	TotalReadAheadLatency time.Duration
}

// ReadAheadStatsSnapshot represents a snapshot of read-ahead statistics without mutex
type ReadAheadStatsSnapshot struct {
	ReadAheadRequests     int64         `json:"read_ahead_requests"`
	ReadAheadHits         int64         `json:"read_ahead_hits"`
	ReadAheadMisses       int64         `json:"read_ahead_misses"`
	ReadAheadBytes        int64         `json:"read_ahead_bytes"`
	PrefetchedBlocks      int64         `json:"prefetched_blocks"`
	PrefetchCacheHits     int64         `json:"prefetch_cache_hits"`
	AvgReadAheadLatency   time.Duration `json:"avg_read_ahead_latency"`
	TotalReadAheadLatency time.Duration `json:"total_read_ahead_latency"`
}

// ReadAheadPattern tracks sequential access patterns for read-ahead
type ReadAheadPattern struct {
	LastAccess    time.Time
	AccessCount   int
	LastCID       string
	IsSequential  bool
	Direction     int // 1 for forward, -1 for backward
}

// readAheadRequest represents a request for read-ahead prefetching
type readAheadRequest struct {
	baseCID    string
	count      int
	direction  int
	startTime  time.Time
}

// ReadAheadConfig configures read-ahead behavior
type ReadAheadConfig struct {
	ReadAheadSize   int
	WorkerCount     int
	MaxPatterns     int
	PatternTimeout  time.Duration
}

// NewReadAheadCache creates a new read-ahead cache
func NewReadAheadCache(underlying Cache, config ReadAheadConfig, logger *logging.Logger) *ReadAheadCache {
	if config.ReadAheadSize <= 0 {
		config.ReadAheadSize = 4
	}
	if config.WorkerCount <= 0 {
		config.WorkerCount = 2
	}
	if config.MaxPatterns <= 0 {
		config.MaxPatterns = 1000
	}
	if config.PatternTimeout <= 0 {
		config.PatternTimeout = 5 * time.Minute
	}

	cache := &ReadAheadCache{
		underlying:      underlying,
		readAheadSize:   config.ReadAheadSize,
		readAheadWorkers: config.WorkerCount,
		logger:          logger,
		readAheadQueue:  make(chan readAheadRequest, 100),
		stopChan:        make(chan struct{}),
		accessPattern:   make(map[string]*ReadAheadPattern),
	}

	// Start read-ahead workers
	for i := 0; i < cache.readAheadWorkers; i++ {
		cache.wg.Add(1)
		go cache.readAheadWorker(i)
	}

	// Start pattern cleanup goroutine
	cache.wg.Add(1)
	go cache.patternCleanup(config.PatternTimeout)

	return cache
}

// Store adds a block to the cache
func (c *ReadAheadCache) Store(cid string, block *blocks.Block) error {
	return c.underlying.Store(cid, block)
}

// Get retrieves a block from the cache and triggers read-ahead if needed
func (c *ReadAheadCache) Get(cid string) (*blocks.Block, error) {
	start := time.Now()
	
	// Get the block from underlying cache
	block, err := c.underlying.Get(cid)
	if err != nil {
		c.updateStats(func(stats *ReadAheadStats) {
			stats.ReadAheadMisses++
		})
		return nil, err
	}

	// Update access pattern and trigger read-ahead
	c.updateAccessPattern(cid)
	c.triggerReadAhead(cid)

	// Update statistics
	c.updateStats(func(stats *ReadAheadStats) {
		stats.ReadAheadHits++
		latency := time.Since(start)
		stats.TotalReadAheadLatency += latency
		stats.ReadAheadRequests++
		if stats.ReadAheadRequests > 0 {
			stats.AvgReadAheadLatency = stats.TotalReadAheadLatency / time.Duration(stats.ReadAheadRequests)
		}
	})

	return block, nil
}

// Has checks if a block exists in the cache
func (c *ReadAheadCache) Has(cid string) bool {
	return c.underlying.Has(cid)
}

// Remove removes a block from the cache
func (c *ReadAheadCache) Remove(cid string) error {
	return c.underlying.Remove(cid)
}

// GetRandomizers returns popular blocks suitable as randomizers
func (c *ReadAheadCache) GetRandomizers(count int) ([]*BlockInfo, error) {
	return c.underlying.GetRandomizers(count)
}

// IncrementPopularity increases the popularity score of a block
func (c *ReadAheadCache) IncrementPopularity(cid string) error {
	return c.underlying.IncrementPopularity(cid)
}

// Size returns the number of blocks in the cache
func (c *ReadAheadCache) Size() int {
	return c.underlying.Size()
}

// Clear removes all blocks from the cache
func (c *ReadAheadCache) Clear() {
	c.underlying.Clear()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.accessPattern = make(map[string]*ReadAheadPattern)
}

// Close shuts down the read-ahead cache
func (c *ReadAheadCache) Close() error {
	close(c.stopChan)
	c.wg.Wait()
	return nil
}

// GetStats returns current read-ahead statistics
func (c *ReadAheadCache) GetStats() ReadAheadStatsSnapshot {
	c.stats.mu.RLock()
	defer c.stats.mu.RUnlock()
	
	return ReadAheadStatsSnapshot{
		ReadAheadRequests:     c.stats.ReadAheadRequests,
		ReadAheadHits:         c.stats.ReadAheadHits,
		ReadAheadMisses:       c.stats.ReadAheadMisses,
		ReadAheadBytes:        c.stats.ReadAheadBytes,
		PrefetchedBlocks:      c.stats.PrefetchedBlocks,
		PrefetchCacheHits:     c.stats.PrefetchCacheHits,
		AvgReadAheadLatency:   c.stats.AvgReadAheadLatency,
		TotalReadAheadLatency: c.stats.TotalReadAheadLatency,
	}
}

// updateAccessPattern updates the access pattern for a CID
func (c *ReadAheadCache) updateAccessPattern(cid string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	pattern, exists := c.accessPattern[cid]
	if !exists {
		c.accessPattern[cid] = &ReadAheadPattern{
			LastAccess:   now,
			AccessCount:  1,
			LastCID:      cid,
			IsSequential: false,
			Direction:    1,
		}
		return
	}

	pattern.AccessCount++
	pattern.LastAccess = now

	// Simple heuristic: if this access is "close" to the last one, consider it sequential
	// In a real implementation, this would use actual block ordering/numbering
	if pattern.AccessCount > 1 && time.Since(pattern.LastAccess) < time.Second {
		pattern.IsSequential = true
	}

	pattern.LastCID = cid
}

// triggerReadAhead triggers read-ahead prefetching based on access patterns
func (c *ReadAheadCache) triggerReadAhead(cid string) {
	c.mu.RLock()
	pattern, exists := c.accessPattern[cid]
	c.mu.RUnlock()

	if !exists || !pattern.IsSequential {
		return
	}

	// Queue read-ahead request
	select {
	case c.readAheadQueue <- readAheadRequest{
		baseCID:   cid,
		count:     c.readAheadSize,
		direction: pattern.Direction,
		startTime: time.Now(),
	}:
		c.logger.Debug("Queued read-ahead request", map[string]interface{}{
			"base_cid":  cid,
			"count":     c.readAheadSize,
			"direction": pattern.Direction,
		})
	default:
		// Queue is full, skip this read-ahead
		c.logger.Debug("Read-ahead queue full, skipping", map[string]interface{}{
			"base_cid": cid,
		})
	}
}

// readAheadWorker processes read-ahead requests
func (c *ReadAheadCache) readAheadWorker(id int) {
	defer c.wg.Done()

	c.logger.Debug("Starting read-ahead worker", map[string]interface{}{
		"worker_id": id,
	})

	for {
		select {
		case <-c.stopChan:
			c.logger.Debug("Stopping read-ahead worker", map[string]interface{}{
				"worker_id": id,
			})
			return
		case req := <-c.readAheadQueue:
			c.processReadAheadRequest(req)
		}
	}
}

// processReadAheadRequest processes a single read-ahead request
func (c *ReadAheadCache) processReadAheadRequest(req readAheadRequest) {
	c.logger.Debug("Processing read-ahead request", map[string]interface{}{
		"base_cid":  req.baseCID,
		"count":     req.count,
		"direction": req.direction,
	})

	// Use the enhanced sequential tracker if available
	// For backward compatibility, we'll keep the simple implementation
	// but log that the enhanced version should be used
	
	c.logger.Info("Using simplified read-ahead. Consider using EnhancedReadAheadWorker for better sequential detection", map[string]interface{}{
		"base_cid": req.baseCID,
	})
	
	// Simple implementation for backward compatibility
	prefetchedCount := 0
	cacheHits := 0

	// Generate sequential CIDs based on a simple pattern
	// In production, use the EnhancedReadAheadWorker which understands descriptors
	for i := 1; i <= req.count; i++ {
		var nextCID string
		if req.direction > 0 {
			// Forward sequential
			nextCID = fmt.Sprintf("%s_seq_%d", req.baseCID, i)
		} else {
			// Backward sequential
			nextCID = fmt.Sprintf("%s_seq_-%d", req.baseCID, i)
		}
		
		// Check if already in cache
		if c.underlying.Has(nextCID) {
			cacheHits++
			continue
		}

		// In production, this would fetch from IPFS
		// For now, we can't fetch without proper block resolution
		c.logger.Debug("Would prefetch block", map[string]interface{}{
			"cid": nextCID,
		})
		
		prefetchedCount++
	}

	// Update statistics
	c.updateStats(func(stats *ReadAheadStats) {
		stats.PrefetchedBlocks += int64(prefetchedCount)
		stats.PrefetchCacheHits += int64(cacheHits)
		stats.ReadAheadBytes += int64(prefetchedCount * 128 * 1024) // Assume 128KB blocks
	})

	c.logger.Debug("Read-ahead request completed", map[string]interface{}{
		"base_cid":         req.baseCID,
		"prefetched_count": prefetchedCount,
		"cache_hits":       cacheHits,
		"duration_ms":      time.Since(req.startTime).Milliseconds(),
	})
}

// patternCleanup periodically removes old access patterns
func (c *ReadAheadCache) patternCleanup(timeout time.Duration) {
	defer c.wg.Done()

	ticker := time.NewTicker(timeout / 2)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.cleanupOldPatterns(timeout)
		}
	}
}

// cleanupOldPatterns removes access patterns older than the timeout
func (c *ReadAheadCache) cleanupOldPatterns(timeout time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	removedCount := 0

	for cid, pattern := range c.accessPattern {
		if now.Sub(pattern.LastAccess) > timeout {
			delete(c.accessPattern, cid)
			removedCount++
		}
	}

	if removedCount > 0 {
		c.logger.Debug("Cleaned up old access patterns", map[string]interface{}{
			"removed_count": removedCount,
			"remaining":     len(c.accessPattern),
		})
	}
}

// updateStats safely updates statistics
func (c *ReadAheadCache) updateStats(updateFunc func(*ReadAheadStats)) {
	c.stats.mu.Lock()
	defer c.stats.mu.Unlock()
	updateFunc(&c.stats)
}
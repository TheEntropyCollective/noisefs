package cache

import (
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/logging"
)

// WriteBackCache implements a cache with write-back capabilities
type WriteBackCache struct {
	underlying     Cache
	logger         *logging.Logger
	
	// Write-back state
	mu             sync.RWMutex
	writeBuffer    map[string]*BufferedWrite
	bufferSize     int
	flushInterval  time.Duration
	flushQueue     chan string
	stopChan       chan struct{}
	wg             sync.WaitGroup
	
	// Statistics
	stats WriteBackStats
}

// WriteBackStats tracks write-back performance metrics
type WriteBackStats struct {
	mu                    sync.RWMutex
	BufferedWrites        int64
	FlushedWrites         int64
	BufferHits            int64
	BufferSize            int64
	FlushLatency          time.Duration
	TotalFlushLatency     time.Duration
	FlushErrors           int64
	CoalescedWrites       int64
}

// BufferedWrite represents a write operation in the buffer
type BufferedWrite struct {
	CID           string
	Block         *blocks.Block
	Timestamp     time.Time
	FlushAttempts int
	Dirty         bool
}

// WriteBackConfig configures write-back behavior
type WriteBackConfig struct {
	BufferSize     int
	FlushInterval  time.Duration
	MaxFlushRetries int
	FlushWorkers   int
}

// NewWriteBackCache creates a new write-back cache
func NewWriteBackCache(underlying Cache, config WriteBackConfig, logger *logging.Logger) *WriteBackCache {
	if config.BufferSize <= 0 {
		config.BufferSize = 1000
	}
	if config.FlushInterval <= 0 {
		config.FlushInterval = 5 * time.Second
	}
	if config.FlushWorkers <= 0 {
		config.FlushWorkers = 2
	}

	cache := &WriteBackCache{
		underlying:    underlying,
		logger:        logger,
		writeBuffer:   make(map[string]*BufferedWrite),
		bufferSize:    config.BufferSize,
		flushInterval: config.FlushInterval,
		flushQueue:    make(chan string, config.BufferSize),
		stopChan:      make(chan struct{}),
	}

	// Start flush workers
	for i := 0; i < config.FlushWorkers; i++ {
		cache.wg.Add(1)
		go cache.flushWorker(i)
	}

	// Start periodic flush
	cache.wg.Add(1)
	go cache.periodicFlush()

	return cache
}

// Store adds a block to the write buffer
func (c *WriteBackCache) Store(cid string, block *blocks.Block) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if we already have this block in buffer
	if existing, exists := c.writeBuffer[cid]; exists {
		// Update existing buffered write
		existing.Block = block
		existing.Timestamp = time.Now()
		existing.Dirty = true
		
		c.updateStats(func(stats *WriteBackStats) {
			stats.CoalescedWrites++
		})
		
		c.logger.Debug("Coalesced write in buffer", map[string]interface{}{
			"cid": cid,
		})
		return nil
	}

	// Check if buffer is full
	if len(c.writeBuffer) >= c.bufferSize {
		// Force flush of oldest entry
		c.flushOldest()
	}

	// Add to buffer
	c.writeBuffer[cid] = &BufferedWrite{
		CID:       cid,
		Block:     block,
		Timestamp: time.Now(),
		Dirty:     true,
	}

	c.updateStats(func(stats *WriteBackStats) {
		stats.BufferedWrites++
		stats.BufferSize = int64(len(c.writeBuffer))
	})

	c.logger.Debug("Buffered write", map[string]interface{}{
		"cid":         cid,
		"buffer_size": len(c.writeBuffer),
	})

	return nil
}

// Get retrieves a block from the cache, checking buffer first
func (c *WriteBackCache) Get(cid string) (*blocks.Block, error) {
	// Check write buffer first
	c.mu.RLock()
	if buffered, exists := c.writeBuffer[cid]; exists {
		c.mu.RUnlock()
		c.updateStats(func(stats *WriteBackStats) {
			stats.BufferHits++
		})
		return buffered.Block, nil
	}
	c.mu.RUnlock()

	// Check underlying cache
	return c.underlying.Get(cid)
}

// Has checks if a block exists in the cache or buffer
func (c *WriteBackCache) Has(cid string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	// Check buffer first
	if _, exists := c.writeBuffer[cid]; exists {
		return true
	}
	
	// Check underlying cache
	return c.underlying.Has(cid)
}

// Remove removes a block from the cache and buffer
func (c *WriteBackCache) Remove(cid string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	// Remove from buffer if present
	delete(c.writeBuffer, cid)
	
	// Remove from underlying cache
	return c.underlying.Remove(cid)
}

// GetRandomizers returns popular blocks suitable as randomizers
func (c *WriteBackCache) GetRandomizers(count int) ([]*BlockInfo, error) {
	return c.underlying.GetRandomizers(count)
}

// IncrementPopularity increases the popularity score of a block
func (c *WriteBackCache) IncrementPopularity(cid string) error {
	return c.underlying.IncrementPopularity(cid)
}

// Size returns the total number of blocks in cache and buffer
func (c *WriteBackCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	return c.underlying.Size() + len(c.writeBuffer)
}

// Clear removes all blocks from the cache and buffer
func (c *WriteBackCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.writeBuffer = make(map[string]*BufferedWrite)
	c.underlying.Clear()
}

// Flush forces all buffered writes to be written to underlying cache
func (c *WriteBackCache) Flush() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var lastError error
	flushedCount := 0

	for cid, buffered := range c.writeBuffer {
		if buffered.Dirty {
			if err := c.underlying.Store(cid, buffered.Block); err != nil {
				lastError = err
				c.logger.Warn("Failed to flush buffered write", map[string]interface{}{
					"cid":   cid,
					"error": err.Error(),
				})
			} else {
				buffered.Dirty = false
				flushedCount++
			}
		}
	}

	c.updateStats(func(stats *WriteBackStats) {
		stats.FlushedWrites += int64(flushedCount)
	})

	c.logger.Debug("Flushed buffered writes", map[string]interface{}{
		"flushed_count": flushedCount,
	})

	return lastError
}

// Close shuts down the write-back cache and flushes all pending writes
func (c *WriteBackCache) Close() error {
	// Signal workers to stop
	close(c.stopChan)
	
	// Flush all pending writes
	if err := c.Flush(); err != nil {
		c.logger.Warn("Error during final flush", map[string]interface{}{
			"error": err.Error(),
		})
	}
	
	// Wait for workers to finish
	c.wg.Wait()
	
	return nil
}

// GetStats returns current write-back statistics
func (c *WriteBackCache) GetStats() WriteBackStats {
	c.stats.mu.RLock()
	defer c.stats.mu.RUnlock()
	return c.stats
}

// flushWorker processes flush requests
func (c *WriteBackCache) flushWorker(id int) {
	defer c.wg.Done()

	c.logger.Debug("Starting flush worker", map[string]interface{}{
		"worker_id": id,
	})

	for {
		select {
		case <-c.stopChan:
			c.logger.Debug("Stopping flush worker", map[string]interface{}{
				"worker_id": id,
			})
			return
		case cid := <-c.flushQueue:
			c.flushSingle(cid)
		}
	}
}

// flushSingle flushes a single buffered write
func (c *WriteBackCache) flushSingle(cid string) {
	start := time.Now()
	
	c.mu.Lock()
	buffered, exists := c.writeBuffer[cid]
	if !exists || !buffered.Dirty {
		c.mu.Unlock()
		return
	}
	
	// Create a copy to avoid holding the lock during flush
	block := buffered.Block
	c.mu.Unlock()

	// Flush to underlying cache
	if err := c.underlying.Store(cid, block); err != nil {
		c.mu.Lock()
		buffered.FlushAttempts++
		c.mu.Unlock()
		
		c.updateStats(func(stats *WriteBackStats) {
			stats.FlushErrors++
		})
		
		c.logger.Warn("Failed to flush buffered write", map[string]interface{}{
			"cid":   cid,
			"error": err.Error(),
			"attempts": buffered.FlushAttempts,
		})
		return
	}

	// Mark as clean and update stats
	c.mu.Lock()
	buffered.Dirty = false
	c.mu.Unlock()

	latency := time.Since(start)
	c.updateStats(func(stats *WriteBackStats) {
		stats.FlushedWrites++
		stats.TotalFlushLatency += latency
		if stats.FlushedWrites > 0 {
			stats.FlushLatency = stats.TotalFlushLatency / time.Duration(stats.FlushedWrites)
		}
	})

	c.logger.Debug("Flushed single write", map[string]interface{}{
		"cid":        cid,
		"latency_ms": latency.Milliseconds(),
	})
}

// periodicFlush periodically flushes dirty writes
func (c *WriteBackCache) periodicFlush() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.flushDirtyWrites()
		}
	}
}

// flushDirtyWrites queues all dirty writes for flushing
func (c *WriteBackCache) flushDirtyWrites() {
	c.mu.RLock()
	dirtyWrites := make([]string, 0, len(c.writeBuffer))
	for cid, buffered := range c.writeBuffer {
		if buffered.Dirty {
			dirtyWrites = append(dirtyWrites, cid)
		}
	}
	c.mu.RUnlock()

	if len(dirtyWrites) == 0 {
		return
	}

	c.logger.Debug("Queueing dirty writes for flush", map[string]interface{}{
		"dirty_count": len(dirtyWrites),
	})

	// Queue dirty writes for flushing
	for _, cid := range dirtyWrites {
		select {
		case c.flushQueue <- cid:
		default:
			// Queue is full, skip this flush cycle
			c.logger.Debug("Flush queue full, skipping write", map[string]interface{}{
				"cid": cid,
			})
		}
	}
}

// flushOldest flushes the oldest buffered write
func (c *WriteBackCache) flushOldest() {
	var oldestCID string
	var oldestTime time.Time

	for cid, buffered := range c.writeBuffer {
		if oldestCID == "" || buffered.Timestamp.Before(oldestTime) {
			oldestCID = cid
			oldestTime = buffered.Timestamp
		}
	}

	if oldestCID != "" {
		// Queue for immediate flush
		select {
		case c.flushQueue <- oldestCID:
		default:
			// Queue is full, force synchronous flush
			c.flushSingle(oldestCID)
		}
	}
}

// updateStats safely updates statistics
func (c *WriteBackCache) updateStats(updateFunc func(*WriteBackStats)) {
	c.stats.mu.Lock()
	defer c.stats.mu.Unlock()
	updateFunc(&c.stats)
}
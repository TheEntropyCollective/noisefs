package cache

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
)

// BlockFetcher is a function that retrieves block data from the network
type BlockFetcher func(ctx context.Context, cid string) ([]byte, error)

// OpportunisticFetcher fetches valuable blocks when spare capacity exists
type OpportunisticFetcher struct {
	// Dependencies
	cache         *AltruisticCache
	healthTracker *BlockHealthTracker
	fetcher       BlockFetcher
	
	// Configuration
	config *OpportunisticConfig
	
	// State
	running       bool
	fetchQueue    chan string
	recentFetches map[string]time.Time // CID -> last fetch time
	fetchErrors   map[string]int       // CID -> error count
	
	// Metrics
	fetchCount    int64
	errorCount    int64
	bytesFlexed   int64
	
	// Control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
}

// OpportunisticConfig configures opportunistic fetching
type OpportunisticConfig struct {
	// When to fetch
	MinFlexPoolFree   float64       // Min free flex pool % to start fetching (0.3 = 30%)
	CheckInterval     time.Duration // How often to check for fetch opportunities
	
	// What to fetch
	MaxBlockSize      int           // Maximum block size to fetch
	ValueThreshold    float64       // Minimum block value to consider
	BatchSize         int           // Blocks to evaluate per check
	
	// Rate limiting
	MaxConcurrent     int           // Max concurrent fetches
	FetchCooldown     time.Duration // Min time between fetching same block
	ErrorBackoff      time.Duration // Backoff after fetch errors
	MaxErrorRetries   int           // Max errors before blacklisting block
	
	// Resource limits
	MaxBandwidthMBps  int           // Max bandwidth for opportunistic fetching
}

// DefaultOpportunisticConfig returns sensible defaults
func DefaultOpportunisticConfig() *OpportunisticConfig {
	return &OpportunisticConfig{
		MinFlexPoolFree:  0.3,           // Start when 30% flex pool free
		CheckInterval:    30 * time.Second,
		MaxBlockSize:     16 * 1024 * 1024, // 16MB max
		ValueThreshold:   2.0,           // Moderate value blocks
		BatchSize:        20,
		MaxConcurrent:    3,
		FetchCooldown:    5 * time.Minute,
		ErrorBackoff:     15 * time.Minute,
		MaxErrorRetries:  3,
		MaxBandwidthMBps: 10,
	}
}

// NewOpportunisticFetcher creates a new opportunistic fetcher
func NewOpportunisticFetcher(
	cache *AltruisticCache,
	healthTracker *BlockHealthTracker,
	fetcher BlockFetcher,
	config *OpportunisticConfig,
) *OpportunisticFetcher {
	if config == nil {
		config = DefaultOpportunisticConfig()
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	of := &OpportunisticFetcher{
		cache:         cache,
		healthTracker: healthTracker,
		fetcher:       fetcher,
		config:        config,
		fetchQueue:    make(chan string, 100),
		recentFetches: make(map[string]time.Time),
		fetchErrors:   make(map[string]int),
		ctx:           ctx,
		cancel:        cancel,
	}
	
	return of
}

// Start begins opportunistic fetching
func (of *OpportunisticFetcher) Start() error {
	of.mu.Lock()
	defer of.mu.Unlock()
	
	if of.running {
		return fmt.Errorf("already running")
	}
	
	of.running = true
	
	// Start checker routine
	of.wg.Add(1)
	go of.checkLoop()
	
	// Start fetcher workers
	for i := 0; i < of.config.MaxConcurrent; i++ {
		of.wg.Add(1)
		go of.fetchWorker(i)
	}
	
	// Start cleanup routine
	of.wg.Add(1)
	go of.cleanupLoop()
	
	return nil
}

// Stop halts opportunistic fetching
func (of *OpportunisticFetcher) Stop() {
	of.mu.Lock()
	if !of.running {
		of.mu.Unlock()
		return
	}
	of.running = false
	of.mu.Unlock()
	
	// Cancel context and wait
	of.cancel()
	close(of.fetchQueue)
	of.wg.Wait()
}

// checkLoop periodically checks if we should fetch blocks
func (of *OpportunisticFetcher) checkLoop() {
	defer of.wg.Done()
	
	ticker := time.NewTicker(of.config.CheckInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-of.ctx.Done():
			return
		case <-ticker.C:
			of.checkAndQueueBlocks()
		}
	}
}

// checkAndQueueBlocks evaluates blocks and queues valuable ones
func (of *OpportunisticFetcher) checkAndQueueBlocks() {
	of.mu.RLock()
	if !of.running {
		of.mu.RUnlock()
		return
	}
	// Check for pause marker
	if pauseTime, exists := of.recentFetches["__PAUSE__"]; exists && time.Now().Before(pauseTime) {
		of.mu.RUnlock()
		return
	}
	of.mu.RUnlock()
	
	// Check if we have spare capacity
	stats := of.cache.GetAltruisticStats()
	flexPoolFree := 1.0 - stats.FlexPoolUsage
	
	if flexPoolFree < of.config.MinFlexPoolFree {
		return // Not enough free space
	}
	
	// Get valuable blocks from health tracker
	valuableBlocks := of.healthTracker.GetMostValuableBlocks(
		of.config.BatchSize,
		of.config.MaxBlockSize,
	)
	
	of.mu.Lock()
	defer of.mu.Unlock()
	
	queued := 0
	for _, cid := range valuableBlocks {
		// Skip if already cached
		if of.cache.Has(cid) {
			continue
		}
		
		// Skip if recently fetched
		if lastFetch, exists := of.recentFetches[cid]; exists {
			if time.Since(lastFetch) < of.config.FetchCooldown {
				continue
			}
		}
		
		// Skip if too many errors
		if errors, exists := of.fetchErrors[cid]; exists {
			if errors >= of.config.MaxErrorRetries {
				continue
			}
		}
		
		// Check block value
		blockValue := of.healthTracker.GetBlockValue(cid)
		if blockValue < of.config.ValueThreshold {
			continue
		}
		
		// Queue for fetching
		select {
		case of.fetchQueue <- cid:
			queued++
		default:
			// Queue full, stop queuing
			return
		}
	}
}

// fetchWorker processes blocks from the fetch queue
func (of *OpportunisticFetcher) fetchWorker(_ int) {
	defer of.wg.Done()
	
	for {
		select {
		case <-of.ctx.Done():
			return
		case cid, ok := <-of.fetchQueue:
			if !ok {
				return
			}
			of.fetchBlock(cid)
		}
	}
}

// fetchBlock retrieves and caches a single block
func (of *OpportunisticFetcher) fetchBlock(cid string) {
	// Create timeout context
	fetchCtx, cancel := context.WithTimeout(of.ctx, 30*time.Second)
	defer cancel()
	
	// Record fetch attempt
	of.mu.Lock()
	of.recentFetches[cid] = time.Now()
	of.mu.Unlock()
	
	// Check if fetcher is nil (for tests)
	if of.fetcher == nil {
		of.handleFetchError(cid, fmt.Errorf("fetcher not configured"))
		return
	}
	
	// Fetch block data
	data, err := of.fetcher(fetchCtx, cid)
	if err != nil {
		of.handleFetchError(cid, err)
		return
	}
	
	// Create block
	block, err := blocks.NewBlock(data)
	if err != nil {
		of.handleFetchError(cid, err)
		return
	}
	
	// Store as altruistic
	err = of.cache.StoreWithOrigin(cid, block, AltruisticBlock)
	if err != nil {
		// Might be out of space or disabled, not a fetch error
		return
	}
	
	// Update metrics
	of.mu.Lock()
	of.fetchCount++
	of.bytesFlexed += int64(len(data))
	delete(of.fetchErrors, cid) // Clear errors on success
	of.mu.Unlock()
	
	// Update health tracker
	of.healthTracker.RecordRequest(cid)
}

// handleFetchError records fetch failures
func (of *OpportunisticFetcher) handleFetchError(cid string, _ error) {
	of.mu.Lock()
	defer of.mu.Unlock()
	
	of.errorCount++
	of.fetchErrors[cid]++
	
	// Apply error backoff
	backoffTime := time.Now().Add(of.config.ErrorBackoff)
	if existing, exists := of.recentFetches[cid]; !exists || existing.Before(backoffTime) {
		of.recentFetches[cid] = backoffTime
	}
}

// cleanupLoop periodically cleans old fetch records
func (of *OpportunisticFetcher) cleanupLoop() {
	defer of.wg.Done()
	
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	
	for {
		select {
		case <-of.ctx.Done():
			return
		case <-ticker.C:
			of.cleanup()
		}
	}
}

// cleanup removes old fetch records
func (of *OpportunisticFetcher) cleanup() {
	of.mu.Lock()
	defer of.mu.Unlock()
	
	cutoff := time.Now().Add(-24 * time.Hour)
	
	// Clean old fetch times
	for cid, fetchTime := range of.recentFetches {
		if fetchTime.Before(cutoff) {
			delete(of.recentFetches, cid)
		}
	}
	
	// Clean error counts for blocks not recently attempted
	for cid := range of.fetchErrors {
		if _, recent := of.recentFetches[cid]; !recent {
			delete(of.fetchErrors, cid)
		}
	}
}

// GetStats returns fetcher statistics
func (of *OpportunisticFetcher) GetStats() map[string]interface{} {
	of.mu.RLock()
	defer of.mu.RUnlock()
	
	return map[string]interface{}{
		"running":         of.running,
		"fetch_count":     of.fetchCount,
		"error_count":     of.errorCount,
		"bytes_fetched":   of.bytesFlexed,
		"queue_length":    len(of.fetchQueue),
		"recent_fetches":  len(of.recentFetches),
		"error_blocks":    len(of.fetchErrors),
	}
}

// SetBandwidthLimit updates the bandwidth limit
func (of *OpportunisticFetcher) SetBandwidthLimit(mbps int) {
	of.mu.Lock()
	defer of.mu.Unlock()
	
	of.config.MaxBandwidthMBps = mbps
}

// PauseForDuration temporarily pauses fetching
func (of *OpportunisticFetcher) PauseForDuration(d time.Duration) {
	of.mu.Lock()
	defer of.mu.Unlock()
	
	// Set a pause marker to prevent all fetching
	pauseEnd := time.Now().Add(d)
	
	// Add a special marker that will be checked in checkAndQueueBlocks
	of.recentFetches["__PAUSE__"] = pauseEnd
}
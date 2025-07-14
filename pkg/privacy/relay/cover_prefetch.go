package relay

import (
	"context"
	"fmt"
	"sync"
	"time"
	
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

// SimpleRateLimiter provides basic rate limiting functionality
type SimpleRateLimiter struct {
	requestsPerSec float64
	burstSize      int
	tokens         chan struct{}
	ticker         *time.Ticker
	stop           chan struct{}
}

// NewSimpleRateLimiter creates a simple rate limiter
func NewSimpleRateLimiter(requestsPerSec float64, burstSize int) *SimpleRateLimiter {
	if requestsPerSec <= 0 {
		requestsPerSec = 10
	}
	if burstSize <= 0 {
		burstSize = int(requestsPerSec) * 2
	}
	
	rl := &SimpleRateLimiter{
		requestsPerSec: requestsPerSec,
		burstSize:      burstSize,
		tokens:         make(chan struct{}, burstSize),
		stop:           make(chan struct{}),
	}
	
	// Fill initial burst
	for i := 0; i < burstSize; i++ {
		rl.tokens <- struct{}{}
	}
	
	// Start refill routine
	interval := time.Duration(float64(time.Second) / requestsPerSec)
	rl.ticker = time.NewTicker(interval)
	
	go func() {
		for {
			select {
			case <-rl.ticker.C:
				select {
				case rl.tokens <- struct{}{}:
				default:
					// Bucket full
				}
			case <-rl.stop:
				rl.ticker.Stop()
				return
			}
		}
	}()
	
	return rl
}

// Wait blocks until a token is available
func (rl *SimpleRateLimiter) Wait(ctx context.Context) error {
	select {
	case <-rl.tokens:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stop stops the rate limiter
func (rl *SimpleRateLimiter) Stop() {
	close(rl.stop)
}

// PrefetchWorker handles concurrent block prefetching with rate limiting
type PrefetchWorker struct {
	id           int
	cache        *CoverBlockCache
	backend      storage.Backend
	rateLimiter  *SimpleRateLimiter
	metrics      *PrefetchMetrics
	errors       chan error
	mu           sync.Mutex
}

// PrefetchMetrics tracks prefetch performance
type PrefetchMetrics struct {
	TotalRequests    int64
	SuccessfulFetches int64
	FailedFetches    int64
	BytesFetched     int64
	AverageFetchTime time.Duration
	LastUpdate       time.Time
	mu               sync.RWMutex
}

// PrefetchManager manages multiple prefetch workers
type PrefetchManager struct {
	workers      []*PrefetchWorker
	workerCount  int
	backend      storage.Backend
	cache        *CoverBlockCache
	metrics      *PrefetchMetrics
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	
	// Configuration
	maxRetries      int
	retryDelay      time.Duration
	requestsPerSec  float64
	burstSize       int
}

// PrefetchConfig contains configuration for prefetching
type PrefetchConfig struct {
	WorkerCount     int
	MaxRetries      int
	RetryDelay      time.Duration
	RequestsPerSec  float64
	BurstSize       int
}

// NewPrefetchManager creates a new prefetch manager
func NewPrefetchManager(cache *CoverBlockCache, backend storage.Backend, config PrefetchConfig) *PrefetchManager {
	ctx, cancel := context.WithCancel(context.Background())
	
	// Set defaults
	if config.WorkerCount <= 0 {
		config.WorkerCount = 4
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = 3
	}
	if config.RetryDelay <= 0 {
		config.RetryDelay = time.Second
	}
	if config.RequestsPerSec <= 0 {
		config.RequestsPerSec = 10
	}
	if config.BurstSize <= 0 {
		config.BurstSize = 20
	}
	
	metrics := &PrefetchMetrics{
		LastUpdate: time.Now(),
	}
	
	manager := &PrefetchManager{
		workerCount:    config.WorkerCount,
		backend:        backend,
		cache:          cache,
		metrics:        metrics,
		ctx:            ctx,
		cancel:         cancel,
		maxRetries:     config.MaxRetries,
		retryDelay:     config.RetryDelay,
		requestsPerSec: config.RequestsPerSec,
		burstSize:      config.BurstSize,
	}
	
	manager.initWorkers()
	return manager
}

// initWorkers initializes the worker pool
func (pm *PrefetchManager) initWorkers() {
	pm.workers = make([]*PrefetchWorker, pm.workerCount)
	
	// Create a shared rate limiter for all workers
	rateLimiter := NewSimpleRateLimiter(pm.requestsPerSec, pm.burstSize)
	
	for i := 0; i < pm.workerCount; i++ {
		worker := &PrefetchWorker{
			id:          i,
			cache:       pm.cache,
			backend:     pm.backend,
			rateLimiter: rateLimiter,
			metrics:     pm.metrics,
			errors:      make(chan error, 10),
		}
		pm.workers[i] = worker
	}
}

// PrefetchPopularBlocks fetches popular blocks concurrently
func (pm *PrefetchManager) PrefetchPopularBlocks(popularBlocks []*PopularityInfo) error {
	if len(popularBlocks) == 0 {
		return nil
	}
	
	// Create work queue
	workQueue := make(chan *PopularityInfo, len(popularBlocks))
	for _, block := range popularBlocks {
		workQueue <- block
	}
	close(workQueue)
	
	// Start workers
	pm.wg.Add(pm.workerCount)
	for _, worker := range pm.workers {
		go pm.workerLoop(worker, workQueue)
	}
	
	// Wait for completion
	pm.wg.Wait()
	
	// Check for errors
	var errors []error
	for _, worker := range pm.workers {
		select {
		case err := <-worker.errors:
			errors = append(errors, err)
		default:
		}
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("prefetch completed with %d errors: %v", len(errors), errors[0])
	}
	
	return nil
}

// workerLoop processes prefetch requests
func (pm *PrefetchManager) workerLoop(worker *PrefetchWorker, workQueue <-chan *PopularityInfo) {
	defer pm.wg.Done()
	
	for block := range workQueue {
		select {
		case <-pm.ctx.Done():
			return
		default:
			if err := worker.prefetchBlock(pm.ctx, block); err != nil {
				select {
				case worker.errors <- err:
				default:
					// Error channel full, drop error
				}
			}
		}
	}
}

// prefetchBlock fetches a single block with retries
func (w *PrefetchWorker) prefetchBlock(ctx context.Context, blockInfo *PopularityInfo) error {
	// Check if already cached
	if cached, exists := w.cache.Get(blockInfo.BlockID); exists && cached != nil {
		return nil // Already have it
	}
	
	// Rate limit
	if err := w.rateLimiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limit error: %w", err)
	}
	
	w.updateMetrics(func(m *PrefetchMetrics) {
		m.TotalRequests++
	})
	
	var lastErr error
	retries := 3 // Hardcoded retry count
	
	for attempt := 0; attempt < retries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			backoff := time.Duration(attempt) * time.Second
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}
		
		start := time.Now()
		
		// Fetch from storage backend
		address := &storage.BlockAddress{
			ID:          blockInfo.BlockID,
			BackendType: storage.BackendTypeIPFS,
		}
		block, err := w.backend.Get(ctx, address)
		if err != nil {
			lastErr = err
			continue
		}
		
		if block == nil || len(block.Data) == 0 {
			lastErr = fmt.Errorf("retrieved empty block for %s", blockInfo.BlockID)
			continue
		}
		
		fetchTime := time.Since(start)
		
		// Store in cache
		w.cache.Put(blockInfo.BlockID, block.Data, blockInfo.Category, blockInfo.PopularityScore)
		
		// Update metrics on success
		w.updateMetrics(func(m *PrefetchMetrics) {
			m.SuccessfulFetches++
			m.BytesFetched += int64(len(block.Data))
			
			// Update average fetch time
			if m.AverageFetchTime == 0 {
				m.AverageFetchTime = fetchTime
			} else {
				// Exponential moving average
				alpha := 0.1
				m.AverageFetchTime = time.Duration(
					float64(m.AverageFetchTime)*(1-alpha) + float64(fetchTime)*alpha,
				)
			}
			m.LastUpdate = time.Now()
		})
		
		return nil
	}
	
	// All retries failed
	w.updateMetrics(func(m *PrefetchMetrics) {
		m.FailedFetches++
	})
	
	return fmt.Errorf("failed to prefetch block %s after %d attempts: %w", 
		blockInfo.BlockID, retries, lastErr)
}

// updateMetrics safely updates metrics
func (w *PrefetchWorker) updateMetrics(updateFunc func(*PrefetchMetrics)) {
	w.metrics.mu.Lock()
	defer w.metrics.mu.Unlock()
	updateFunc(w.metrics)
}

// GetMetrics returns current prefetch metrics
func (pm *PrefetchManager) GetMetrics() PrefetchMetrics {
	pm.metrics.mu.RLock()
	defer pm.metrics.mu.RUnlock()
	return *pm.metrics
}

// Stop stops all prefetch workers
func (pm *PrefetchManager) Stop() {
	pm.cancel()
	pm.wg.Wait()
	
	// Stop rate limiters
	if len(pm.workers) > 0 && pm.workers[0].rateLimiter != nil {
		pm.workers[0].rateLimiter.Stop()
	}
}

// EnhancedPrefetchPopular replaces the simplified PrefetchPopular in CoverBlockCache
func (cbc *CoverBlockCache) EnhancedPrefetchPopular(tracker *PopularBlockTracker, backend storage.Backend) {
	// Get popular blocks to prefetch
	popularBlocks, err := tracker.GetRandomizedBlocks(cbc.config.PrefetchSize * 2)
	if err != nil || len(popularBlocks) == 0 {
		return
	}
	
	// Create prefetch manager
	config := PrefetchConfig{
		WorkerCount:    4,
		MaxRetries:     3,
		RetryDelay:     time.Second,
		RequestsPerSec: 10,
		BurstSize:      20,
	}
	
	manager := NewPrefetchManager(cbc, backend, config)
	defer manager.Stop()
	
	// Filter blocks that need prefetching
	blocksToFetch := make([]*PopularityInfo, 0, cbc.config.PrefetchSize)
	cbc.mu.RLock()
	for _, block := range popularBlocks {
		if _, exists := cbc.cache[block.BlockID]; !exists {
			blocksToFetch = append(blocksToFetch, block)
			if len(blocksToFetch) >= cbc.config.PrefetchSize {
				break
			}
		}
	}
	cbc.mu.RUnlock()
	
	if len(blocksToFetch) == 0 {
		return // Nothing to prefetch
	}
	
	// Perform prefetch
	if err := manager.PrefetchPopularBlocks(blocksToFetch); err != nil {
		// Log error but don't fail - prefetch is best effort
		// In production, this would use proper logging
		fmt.Printf("Prefetch completed with errors: %v\n", err)
	}
	
	// Update metrics
	metrics := manager.GetMetrics()
	cbc.mu.Lock()
	cbc.metrics.PrefetchHits += metrics.SuccessfulFetches
	cbc.updateMetrics()
	cbc.mu.Unlock()
}

// PrefetchResult contains the result of a prefetch operation
type PrefetchResult struct {
	BlockID    string
	Success    bool
	Error      error
	FetchTime  time.Duration
	DataSize   int64
}

// BatchPrefetchWithResults performs batch prefetch and returns detailed results
func (pm *PrefetchManager) BatchPrefetchWithResults(blocks []*PopularityInfo) []PrefetchResult {
	results := make([]PrefetchResult, len(blocks))
	resultChan := make(chan PrefetchResult, len(blocks))
	
	// Create work items with index
	type workItem struct {
		index int
		block *PopularityInfo
	}
	
	workQueue := make(chan workItem, len(blocks))
	for i, block := range blocks {
		workQueue <- workItem{index: i, block: block}
	}
	close(workQueue)
	
	// Start workers
	pm.wg.Add(pm.workerCount)
	for _, worker := range pm.workers {
		go func(w *PrefetchWorker) {
			defer pm.wg.Done()
			
			for item := range workQueue {
				start := time.Now()
				err := w.prefetchBlock(pm.ctx, item.block)
				
				result := PrefetchResult{
					BlockID:   item.block.BlockID,
					Success:   err == nil,
					Error:     err,
					FetchTime: time.Since(start),
				}
				
				if err == nil {
					// Get size from cache
					if cached, exists := w.cache.Get(item.block.BlockID); exists {
						result.DataSize = cached.Size
					}
				}
				
				resultChan <- result
			}
		}(worker)
	}
	
	// Collect results
	go func() {
		pm.wg.Wait()
		close(resultChan)
	}()
	
	for result := range resultChan {
		results = append(results, result)
	}
	
	return results
}
package relay

import (
	"context"
	"crypto/rand"
	"fmt"
	"math"
	"math/big"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// CoverTrafficGenerator generates cover traffic to mask real block requests
type CoverTrafficGenerator struct {
	config            *CoverTrafficConfig
	popularityTracker *PopularBlockTracker
	pool              *RelayPool
	connectionPool    *ConnectionPool
	metrics           *CoverTrafficMetrics
	activeRequests    map[string]*CoverRequest
	bandwidthLimiter  *BandwidthLimiter
	mu                sync.RWMutex
	ctx               context.Context
	cancel            context.CancelFunc
}

// CoverTrafficConfig contains configuration for cover traffic generation
type CoverTrafficConfig struct {
	NoiseRatio          float64       // Ratio of cover traffic to real traffic (e.g., 2.0 = 2x cover)
	MinCoverRequests    int           // Minimum number of cover requests per batch
	MaxCoverRequests    int           // Maximum number of cover requests per batch
	CoverInterval       time.Duration // Interval between cover traffic bursts
	RandomDelay         time.Duration // Maximum random delay for cover requests
	BandwidthLimit      float64       // Bandwidth limit for cover traffic (MB/s)
	CategoryDistribution map[BlockCategory]float64 // Distribution of block categories
	PopularityBias      float64       // Bias towards popular blocks (0-1)
	BatchSize           int           // Size of cover request batches
	MaxConcurrent       int           // Maximum concurrent cover requests
}

// CoverRequest represents a single cover traffic request
type CoverRequest struct {
	ID            string
	BlockID       string
	RelayID       peer.ID
	Category      BlockCategory
	Priority      int
	StartTime     time.Time
	EndTime       time.Time
	Status        CoverRequestStatus
	Size          int64  // Size of requested block
	IsDecoy       bool   // True if this is purely decoy traffic
	RealRequestID string // ID of real request this is covering (if any)
}

// CoverRequestStatus represents the status of a cover request
type CoverRequestStatus int

const (
	CoverStatusPending CoverRequestStatus = iota
	CoverStatusSent
	CoverStatusCompleted
	CoverStatusFailed
	CoverStatusCancelled
)

// CoverTrafficMetrics tracks cover traffic performance
type CoverTrafficMetrics struct {
	TotalCoverRequests   int64
	SuccessfulRequests   int64
	FailedRequests       int64
	TotalBandwidthUsed   float64 // MB
	AverageCoverLatency  time.Duration
	NoiseRatioAchieved   float64
	CoverRequestsPerSecond float64
	CategoryDistribution map[BlockCategory]int64
	LastUpdate           time.Time
}

// BandwidthLimiter controls the rate of cover traffic to avoid overwhelming the network
type BandwidthLimiter struct {
	limit        float64 // MB/s
	used         float64 // MB used in current window
	windowStart  time.Time
	windowSize   time.Duration
	mu           sync.Mutex
}

// CoverTrafficBatch represents a batch of cover requests
type CoverTrafficBatch struct {
	ID       string
	Requests []*CoverRequest
	Created  time.Time
	Mixed    bool // True if mixed with real requests
}

// NewCoverTrafficGenerator creates a new cover traffic generator
func NewCoverTrafficGenerator(config *CoverTrafficConfig, popularityTracker *PopularBlockTracker, pool *RelayPool, connectionPool *ConnectionPool) *CoverTrafficGenerator {
	ctx, cancel := context.WithCancel(context.Background())
	
	generator := &CoverTrafficGenerator{
		config:            config,
		popularityTracker: popularityTracker,
		pool:              pool,
		connectionPool:    connectionPool,
		metrics:           &CoverTrafficMetrics{CategoryDistribution: make(map[BlockCategory]int64)},
		activeRequests:    make(map[string]*CoverRequest),
		bandwidthLimiter:  NewBandwidthLimiter(config.BandwidthLimit, time.Minute),
		ctx:               ctx,
		cancel:            cancel,
	}
	
	// Start cover traffic generation
	go generator.generateCoverTrafficLoop()
	
	return generator
}

// NewBandwidthLimiter creates a new bandwidth limiter
func NewBandwidthLimiter(limitMBps float64, windowSize time.Duration) *BandwidthLimiter {
	return &BandwidthLimiter{
		limit:       limitMBps,
		windowSize:  windowSize,
		windowStart: time.Now(),
	}
}

// GenerateCoverTraffic generates cover traffic to mask a set of real requests
func (ctg *CoverTrafficGenerator) GenerateCoverTraffic(ctx context.Context, realRequests []string) (*CoverTrafficBatch, error) {
	// Calculate number of cover requests needed
	coverCount := int(float64(len(realRequests)) * ctg.config.NoiseRatio)
	if coverCount < ctg.config.MinCoverRequests {
		coverCount = ctg.config.MinCoverRequests
	}
	if coverCount > ctg.config.MaxCoverRequests {
		coverCount = ctg.config.MaxCoverRequests
	}
	
	// Generate cover requests
	coverRequests, err := ctg.generateCoverRequests(ctx, coverCount)
	if err != nil {
		return nil, fmt.Errorf("failed to generate cover requests: %w", err)
	}
	
	batch := &CoverTrafficBatch{
		ID:       ctg.generateBatchID(),
		Requests: coverRequests,
		Created:  time.Now(),
		Mixed:    true,
	}
	
	// Execute cover requests
	go ctg.executeCoverBatch(ctx, batch)
	
	return batch, nil
}

// GenerateBackgroundCoverTraffic generates ongoing background cover traffic
func (ctg *CoverTrafficGenerator) GenerateBackgroundCoverTraffic(ctx context.Context) error {
	// Generate a smaller batch for background noise
	coverCount := ctg.config.MinCoverRequests / 2
	if coverCount < 1 {
		coverCount = 1
	}
	
	coverRequests, err := ctg.generateCoverRequests(ctx, coverCount)
	if err != nil {
		return fmt.Errorf("failed to generate background cover traffic: %w", err)
	}
	
	batch := &CoverTrafficBatch{
		ID:       ctg.generateBatchID(),
		Requests: coverRequests,
		Created:  time.Now(),
		Mixed:    false,
	}
	
	// Execute in background
	go ctg.executeCoverBatch(ctx, batch)
	
	return nil
}

// generateCoverRequests generates a set of cover requests
func (ctg *CoverTrafficGenerator) generateCoverRequests(ctx context.Context, count int) ([]*CoverRequest, error) {
	requests := make([]*CoverRequest, 0, count)
	
	for i := 0; i < count; i++ {
		// Select category based on distribution
		category := ctg.selectRandomCategory()
		
		// Get popular blocks for this category
		popularBlocks, err := ctg.popularityTracker.GetPopularBlocks(50, category)
		if err != nil || len(popularBlocks.Blocks) == 0 {
			// Fall back to any popular blocks
			popularBlocks, err = ctg.popularityTracker.GetPopularBlocks(50, CategoryUnknown)
			if err != nil || len(popularBlocks.Blocks) == 0 {
				continue // Skip this request if no popular blocks available
			}
		}
		
		// Select a block based on popularity bias
		blockInfo := ctg.selectBlockWithBias(popularBlocks.Blocks)
		
		// Select a relay
		relays, err := ctg.pool.SelectRelays(ctx, 1)
		if err != nil || len(relays) == 0 {
			continue // Skip if no relays available
		}
		
		// Create cover request
		request := &CoverRequest{
			ID:        ctg.generateRequestID(),
			BlockID:   blockInfo.BlockID,
			RelayID:   relays[0].ID,
			Category:  blockInfo.Category,
			Priority:  0, // Cover traffic has lowest priority
			StartTime: time.Now(),
			Status:    CoverStatusPending,
			Size:      128 * 1024, // Assume 128KB blocks
			IsDecoy:   true,
		}
		
		requests = append(requests, request)
	}
	
	return requests, nil
}

// selectRandomCategory selects a random block category based on configured distribution
func (ctg *CoverTrafficGenerator) selectRandomCategory() BlockCategory {
	if len(ctg.config.CategoryDistribution) == 0 {
		return CategoryUnknown
	}
	
	// Generate random number
	r, _ := rand.Int(rand.Reader, big.NewInt(1000))
	random := float64(r.Int64()) / 1000.0
	
	// Select category based on cumulative distribution
	cumulative := 0.0
	for category, weight := range ctg.config.CategoryDistribution {
		cumulative += weight
		if random <= cumulative {
			return category
		}
	}
	
	// Default fallback
	return CategoryUnknown
}

// selectBlockWithBias selects a block from the popular list with popularity bias
func (ctg *CoverTrafficGenerator) selectBlockWithBias(blocks []*PopularityInfo) *PopularityInfo {
	if len(blocks) == 0 {
		return nil
	}
	
	if len(blocks) == 1 {
		return blocks[0]
	}
	
	// Generate random number for selection
	r, _ := rand.Int(rand.Reader, big.NewInt(1000))
	random := float64(r.Int64()) / 1000.0
	
	// Apply popularity bias
	if random < ctg.config.PopularityBias {
		// Select from top popular blocks
		topCount := int(math.Max(1, float64(len(blocks))*0.3))
		r2, _ := rand.Int(rand.Reader, big.NewInt(int64(topCount)))
		return blocks[r2.Int64()]
	} else {
		// Select randomly
		r2, _ := rand.Int(rand.Reader, big.NewInt(int64(len(blocks))))
		return blocks[r2.Int64()]
	}
}

// executeCoverBatch executes a batch of cover requests
func (ctg *CoverTrafficGenerator) executeCoverBatch(ctx context.Context, batch *CoverTrafficBatch) {
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, ctg.config.MaxConcurrent)
	
	for _, request := range batch.Requests {
		wg.Add(1)
		semaphore <- struct{}{} // Acquire semaphore
		
		go func(req *CoverRequest) {
			defer wg.Done()
			defer func() { <-semaphore }() // Release semaphore
			
			ctg.executeCoverRequest(ctx, req)
		}(request)
	}
	
	wg.Wait()
	
	// Update metrics
	ctg.updateBatchMetrics(batch)
}

// executeCoverRequest executes a single cover request
func (ctg *CoverTrafficGenerator) executeCoverRequest(ctx context.Context, request *CoverRequest) {
	// Add random delay to avoid timing correlation
	if ctg.config.RandomDelay > 0 {
		delay, _ := rand.Int(rand.Reader, big.NewInt(int64(ctg.config.RandomDelay.Milliseconds())))
		time.Sleep(time.Duration(delay.Int64()) * time.Millisecond)
	}
	
	// Check bandwidth limit
	if !ctg.bandwidthLimiter.Reserve(float64(request.Size) / (1024 * 1024)) {
		request.Status = CoverStatusCancelled
		return
	}
	
	// Store active request
	ctg.mu.Lock()
	ctg.activeRequests[request.ID] = request
	ctg.mu.Unlock()
	
	// Update status
	request.Status = CoverStatusSent
	
	// Get connection to relay
	conn, err := ctg.connectionPool.GetConnection(ctx, request.RelayID)
	if err != nil {
		request.Status = CoverStatusFailed
		ctg.removeActiveRequest(request.ID)
		return
	}
	
	// Create cover request message
	msg, err := conn.Protocol.CreateCoverRequest(ctx, []string{request.BlockID}, 1, request.RelayID)
	if err != nil {
		request.Status = CoverStatusFailed
		ctg.removeActiveRequest(request.ID)
		return
	}
	
	// Send request
	_, err = ctg.connectionPool.SendRequest(ctx, conn, msg)
	request.EndTime = time.Now()
	
	if err != nil {
		request.Status = CoverStatusFailed
	} else {
		request.Status = CoverStatusCompleted
	}
	
	// Remove from active requests
	ctg.removeActiveRequest(request.ID)
}

// generateCoverTrafficLoop generates background cover traffic
func (ctg *CoverTrafficGenerator) generateCoverTrafficLoop() {
	ticker := time.NewTicker(ctg.config.CoverInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctg.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(ctg.ctx, 30*time.Second)
			ctg.GenerateBackgroundCoverTraffic(ctx)
			cancel()
		}
	}
}

// Reserve reserves bandwidth for a request
func (bl *BandwidthLimiter) Reserve(sizeMB float64) bool {
	bl.mu.Lock()
	defer bl.mu.Unlock()
	
	now := time.Now()
	
	// Reset window if expired
	if now.Sub(bl.windowStart) >= bl.windowSize {
		bl.used = 0
		bl.windowStart = now
	}
	
	// Check if request fits within limit
	if bl.used+sizeMB > bl.limit {
		return false
	}
	
	bl.used += sizeMB
	return true
}

// updateBatchMetrics updates metrics after batch completion
func (ctg *CoverTrafficGenerator) updateBatchMetrics(batch *CoverTrafficBatch) {
	ctg.mu.Lock()
	defer ctg.mu.Unlock()
	
	successful := int64(0)
	failed := int64(0)
	totalLatency := time.Duration(0)
	bandwidthUsed := 0.0
	
	for _, request := range batch.Requests {
		ctg.metrics.TotalCoverRequests++
		ctg.metrics.CategoryDistribution[request.Category]++
		
		if request.Status == CoverStatusCompleted {
			successful++
			latency := request.EndTime.Sub(request.StartTime)
			totalLatency += latency
		} else {
			failed++
		}
		
		bandwidthUsed += float64(request.Size) / (1024 * 1024) // Convert to MB
	}
	
	ctg.metrics.SuccessfulRequests += successful
	ctg.metrics.FailedRequests += failed
	ctg.metrics.TotalBandwidthUsed += bandwidthUsed
	
	if successful > 0 {
		avgLatency := totalLatency / time.Duration(successful)
		if ctg.metrics.AverageCoverLatency == 0 {
			ctg.metrics.AverageCoverLatency = avgLatency
		} else {
			ctg.metrics.AverageCoverLatency = (ctg.metrics.AverageCoverLatency + avgLatency) / 2
		}
	}
	
	ctg.metrics.LastUpdate = time.Now()
}

// removeActiveRequest removes a request from active tracking
func (ctg *CoverTrafficGenerator) removeActiveRequest(requestID string) {
	ctg.mu.Lock()
	defer ctg.mu.Unlock()
	delete(ctg.activeRequests, requestID)
}

// generateRequestID generates a unique request ID
func (ctg *CoverTrafficGenerator) generateRequestID() string {
	return fmt.Sprintf("cover_%d", time.Now().UnixNano())
}

// generateBatchID generates a unique batch ID
func (ctg *CoverTrafficGenerator) generateBatchID() string {
	return fmt.Sprintf("batch_%d", time.Now().UnixNano())
}

// GetMetrics returns current cover traffic metrics
func (ctg *CoverTrafficGenerator) GetMetrics() *CoverTrafficMetrics {
	ctg.mu.RLock()
	defer ctg.mu.RUnlock()
	return ctg.metrics
}

// Stop stops the cover traffic generator
func (ctg *CoverTrafficGenerator) Stop() {
	ctg.cancel()
}
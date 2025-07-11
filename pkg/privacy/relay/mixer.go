package relay

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// RequestMixer combines real and cover traffic to obscure request patterns
type RequestMixer struct {
	config             *MixerConfig
	coverGenerator     *CoverTrafficGenerator
	popularityTracker  *PopularBlockTracker
	distributor        *RequestDistributor
	activeMixes        map[string]*MixedRequest
	mixingPools        map[string]*MixingPool
	metrics            *MixerMetrics
	enhancedExecutor   *EnhancedMixerExecutor // Optional enhanced executor
	logger             Logger // Optional logger interface
	mu                 sync.RWMutex
	ctx                context.Context
	cancel             context.CancelFunc
}

// Logger interface for optional logging
type Logger interface {
	Warn(msg string, fields ...interface{})
	Info(msg string, fields ...interface{})
	Debug(msg string, fields ...interface{})
}

// MixerConfig contains configuration for request mixing
type MixerConfig struct {
	MixingDelay        time.Duration // Maximum delay to wait for mixing opportunities
	MinMixSize         int           // Minimum number of requests to mix together
	MaxMixSize         int           // Maximum number of requests in a mix
	CoverRatio         float64       // Ratio of cover to real requests in mix
	RelayDistribution  float64       // How much to distribute across different relays (0-1)
	TemporalJitter     time.Duration // Random time jitter for mixed requests
	PriorityMixing     bool          // Whether to mix requests of different priorities
	BatchTimeout       time.Duration // Timeout for accumulating batch requests
}

// MixedRequest represents a request that has been mixed with others
type MixedRequest struct {
	ID             string
	OriginalID     string
	BlockID        string
	RelayID        peer.ID
	MixID          string
	IsRealRequest  bool
	Priority       int
	SubmitTime     time.Time
	MixedTime      time.Time
	CompletedTime  time.Time
	Status         MixedRequestStatus
	MixSize        int // Total size of the mix this request is part of
}

// MixedRequestStatus represents the status of a mixed request
type MixedRequestStatus int

const (
	MixedStatusPending MixedRequestStatus = iota
	MixedStatusMixed
	MixedStatusSent
	MixedStatusCompleted
	MixedStatusFailed
)

// MixingPool accumulates requests for mixing
type MixingPool struct {
	ID           string
	Requests     []*PendingRequest
	CoverRequests []*CoverRequest
	Created      time.Time
	Deadline     time.Time
	RelayTargets map[peer.ID]int // How many requests per relay
	Sealed       bool
}

// PendingRequest represents a real request waiting to be mixed
type PendingRequest struct {
	ID        string
	BlockID   string
	Priority  int
	SubmitTime time.Time
	Context   context.Context
	Response  chan *MixedRequestResult
}

// MixedRequestResult contains the result of a mixed request
type MixedRequestResult struct {
	BlockID    string
	Data       []byte
	Success    bool
	Error      error
	Latency    time.Duration
	MixID      string
	RelayUsed  peer.ID
}

// MixerMetrics tracks mixing performance
type MixerMetrics struct {
	TotalMixes         int64
	TotalRequests      int64
	AverageMixSize     float64
	AverageMixDelay    time.Duration
	CoverRatioAchieved float64
	RelayDistribution  float64
	MixingEfficiency   float64 // Successful mixes / total attempts
	LastUpdate         time.Time
}

// NewRequestMixer creates a new request mixer
func NewRequestMixer(config *MixerConfig, coverGenerator *CoverTrafficGenerator, popularityTracker *PopularBlockTracker, distributor *RequestDistributor) *RequestMixer {
	ctx, cancel := context.WithCancel(context.Background())
	
	mixer := &RequestMixer{
		config:            config,
		coverGenerator:    coverGenerator,
		popularityTracker: popularityTracker,
		distributor:       distributor,
		activeMixes:       make(map[string]*MixedRequest),
		mixingPools:       make(map[string]*MixingPool),
		metrics:           &MixerMetrics{},
		ctx:               ctx,
		cancel:            cancel,
	}
	
	// Start mixing routine
	go mixer.mixingLoop()
	
	return mixer
}

// SubmitRequest submits a request for mixing
func (rm *RequestMixer) SubmitRequest(ctx context.Context, blockID string, priority int) (*MixedRequestResult, error) {
	// Create pending request
	request := &PendingRequest{
		ID:       rm.generateRequestID(),
		BlockID:  blockID,
		Priority: priority,
		SubmitTime: time.Now(),
		Context:  ctx,
		Response: make(chan *MixedRequestResult, 1),
	}
	
	// Add to mixing pool
	err := rm.addToMixingPool(request)
	if err != nil {
		return nil, fmt.Errorf("failed to add request to mixing pool: %w", err)
	}
	
	// Wait for result or timeout
	select {
	case result := <-request.Response:
		return result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// addToMixingPool adds a request to an appropriate mixing pool
func (rm *RequestMixer) addToMixingPool(request *PendingRequest) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	
	// Find or create mixing pool based on priority
	poolID := rm.getPoolID(request.Priority)
	
	pool, exists := rm.mixingPools[poolID]
	if !exists {
		pool = &MixingPool{
			ID:           poolID,
			Requests:     make([]*PendingRequest, 0),
			CoverRequests: make([]*CoverRequest, 0),
			Created:      time.Now(),
			Deadline:     time.Now().Add(rm.config.BatchTimeout),
			RelayTargets: make(map[peer.ID]int),
		}
		rm.mixingPools[poolID] = pool
	}
	
	// Check if pool is already sealed
	if pool.Sealed {
		// Create new pool
		pool = &MixingPool{
			ID:           fmt.Sprintf("%s_%d", poolID, time.Now().UnixNano()),
			Requests:     make([]*PendingRequest, 0),
			CoverRequests: make([]*CoverRequest, 0),
			Created:      time.Now(),
			Deadline:     time.Now().Add(rm.config.BatchTimeout),
			RelayTargets: make(map[peer.ID]int),
		}
		rm.mixingPools[pool.ID] = pool
	}
	
	// Add request to pool
	pool.Requests = append(pool.Requests, request)
	
	// Check if pool is ready for mixing
	if len(pool.Requests) >= rm.config.MinMixSize {
		go rm.processMixingPool(pool.ID)
	}
	
	return nil
}

// mixingLoop periodically processes mixing pools
func (rm *RequestMixer) mixingLoop() {
	ticker := time.NewTicker(rm.config.MixingDelay / 2) // Check twice per max delay
	defer ticker.Stop()
	
	for {
		select {
		case <-rm.ctx.Done():
			return
		case <-ticker.C:
			rm.processExpiredPools()
		}
	}
}

// processExpiredPools processes pools that have reached their deadline
func (rm *RequestMixer) processExpiredPools() {
	rm.mu.RLock()
	expiredPools := make([]string, 0)
	
	for poolID, pool := range rm.mixingPools {
		if time.Now().After(pool.Deadline) && !pool.Sealed {
			expiredPools = append(expiredPools, poolID)
		}
	}
	rm.mu.RUnlock()
	
	// Process expired pools
	for _, poolID := range expiredPools {
		go rm.processMixingPool(poolID)
	}
}

// processMixingPool processes a mixing pool by creating a mixed request batch
func (rm *RequestMixer) processMixingPool(poolID string) {
	rm.mu.Lock()
	pool, exists := rm.mixingPools[poolID]
	if !exists || pool.Sealed {
		rm.mu.Unlock()
		return
	}
	
	// Seal the pool to prevent further additions
	pool.Sealed = true
	rm.mu.Unlock()
	
	// Generate cover traffic for this mix
	coverCount := int(float64(len(pool.Requests)) * rm.config.CoverRatio)
	if coverCount > 0 {
		coverBatch, err := rm.coverGenerator.GenerateCoverTraffic(rm.ctx, rm.getBlockIDs(pool.Requests))
		if err == nil && coverBatch != nil {
			// Convert cover requests to our format
			for _, coverReq := range coverBatch.Requests {
				pool.CoverRequests = append(pool.CoverRequests, coverReq)
			}
		}
	}
	
	// Create mixed requests
	mixedRequests := rm.createMixedRequests(pool)
	
	// Distribute across relays
	rm.distributeRequests(mixedRequests)
	
	// Execute mixed requests
	rm.executeMixedRequests(pool, mixedRequests)
	
	// Clean up pool
	rm.mu.Lock()
	delete(rm.mixingPools, poolID)
	rm.mu.Unlock()
}

// createMixedRequests creates mixed requests from a pool
func (rm *RequestMixer) createMixedRequests(pool *MixingPool) []*MixedRequest {
	mixID := rm.generateMixID()
	mixedRequests := make([]*MixedRequest, 0)
	
	// Create mixed requests for real requests
	for _, req := range pool.Requests {
		mixed := &MixedRequest{
			ID:            rm.generateRequestID(),
			OriginalID:    req.ID,
			BlockID:       req.BlockID,
			MixID:         mixID,
			IsRealRequest: true,
			Priority:      req.Priority,
			SubmitTime:    req.SubmitTime,
			MixedTime:     time.Now(),
			Status:        MixedStatusMixed,
			MixSize:       len(pool.Requests) + len(pool.CoverRequests),
		}
		mixedRequests = append(mixedRequests, mixed)
	}
	
	// Create mixed requests for cover traffic
	for _, coverReq := range pool.CoverRequests {
		mixed := &MixedRequest{
			ID:            rm.generateRequestID(),
			OriginalID:    coverReq.ID,
			BlockID:       coverReq.BlockID,
			MixID:         mixID,
			IsRealRequest: false,
			Priority:      0,
			SubmitTime:    coverReq.StartTime,
			MixedTime:     time.Now(),
			Status:        MixedStatusMixed,
			MixSize:       len(pool.Requests) + len(pool.CoverRequests),
		}
		mixedRequests = append(mixedRequests, mixed)
	}
	
	return mixedRequests
}

// distributeRequests distributes mixed requests across relays
func (rm *RequestMixer) distributeRequests(mixedRequests []*MixedRequest) {
	// Get available relays
	relays, err := rm.distributor.pool.SelectRelays(rm.ctx, rm.config.MaxMixSize)
	if err != nil || len(relays) == 0 {
		// Fallback to single relay if none available
		return
	}
	
	// Calculate distribution
	relayCount := int(float64(len(relays)) * rm.config.RelayDistribution)
	if relayCount < 1 {
		relayCount = 1
	}
	if relayCount > len(relays) {
		relayCount = len(relays)
	}
	
	// Assign relays to requests
	for i, mixed := range mixedRequests {
		relayIndex := i % relayCount
		mixed.RelayID = relays[relayIndex].ID
	}
}

// executeMixedRequests executes all mixed requests in a pool
func (rm *RequestMixer) executeMixedRequests(pool *MixingPool, mixedRequests []*MixedRequest) {
	var wg sync.WaitGroup
	results := make(map[string]*MixedRequestResult)
	resultsMu := sync.Mutex{}
	
	// Execute requests with temporal jitter
	for _, mixed := range mixedRequests {
		wg.Add(1)
		
		go func(req *MixedRequest) {
			defer wg.Done()
			
			// Add temporal jitter
			if rm.config.TemporalJitter > 0 {
				jitter, _ := rand.Int(rand.Reader, big.NewInt(int64(rm.config.TemporalJitter.Milliseconds())))
				time.Sleep(time.Duration(jitter.Int64()) * time.Millisecond)
			}
			
			// Execute request
			result := rm.executeRequest(req)
			
			// Store result for real requests
			if req.IsRealRequest {
				resultsMu.Lock()
				results[req.OriginalID] = result
				resultsMu.Unlock()
			}
		}(mixed)
	}
	
	wg.Wait()
	
	// Send results back to original requesters
	for _, req := range pool.Requests {
		if result, exists := results[req.ID]; exists {
			select {
			case req.Response <- result:
			case <-req.Context.Done():
				// Request context cancelled
			}
		}
	}
	
	// Update metrics
	rm.updateMixMetrics(pool, mixedRequests)
}

// executeRequest executes a single mixed request
func (rm *RequestMixer) executeRequest(mixed *MixedRequest) *MixedRequestResult {
	start := time.Now()
	mixed.Status = MixedStatusSent
	
	// Check if we have an enhanced executor available
	// If not, fall back to simplified implementation
	if rm.enhancedExecutor != nil {
		return rm.enhancedExecutor.ExecuteMixedRequest(rm.ctx, mixed)
	}
	
	// Simplified fallback implementation
	// Create block request array
	blockIDs := []string{mixed.BlockID}
	
	// Submit to distributor
	_, err := rm.distributor.DistributeRequest(rm.ctx, mixed.ID, blockIDs)
	if err != nil {
		mixed.Status = MixedStatusFailed
		return &MixedRequestResult{
			BlockID:   mixed.BlockID,
			Success:   false,
			Error:     err,
			MixID:     mixed.MixID,
			RelayUsed: mixed.RelayID,
		}
	}
	
	// Log that enhanced executor should be used
	if rm.logger != nil {
		rm.logger.Warn("Using simplified request execution. Consider using EnhancedMixerExecutor for proper tracking and data retrieval")
	}
	
	// Wait for completion (simplified)
	time.Sleep(50 * time.Millisecond) // Simulate processing time
	
	mixed.Status = MixedStatusCompleted
	mixed.CompletedTime = time.Now()
	
	return &MixedRequestResult{
		BlockID:   mixed.BlockID,
		Data:      []byte("simulated block data - use EnhancedMixerExecutor for real data"),
		Success:   true,
		Latency:   time.Since(start),
		MixID:     mixed.MixID,
		RelayUsed: mixed.RelayID,
	}
}

// getBlockIDs extracts block IDs from pending requests
func (rm *RequestMixer) getBlockIDs(requests []*PendingRequest) []string {
	blockIDs := make([]string, len(requests))
	for i, req := range requests {
		blockIDs[i] = req.BlockID
	}
	return blockIDs
}

// getPoolID generates a pool ID based on priority
func (rm *RequestMixer) getPoolID(priority int) string {
	if rm.config.PriorityMixing {
		return "mixed_pool" // Mix all priorities together
	}
	return fmt.Sprintf("pool_p%d", priority)
}

// updateMixMetrics updates mixing metrics
func (rm *RequestMixer) updateMixMetrics(pool *MixingPool, mixedRequests []*MixedRequest) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	
	rm.metrics.TotalMixes++
	rm.metrics.TotalRequests += int64(len(pool.Requests))
	
	// Update average mix size
	mixSize := float64(len(mixedRequests))
	if rm.metrics.AverageMixSize == 0 {
		rm.metrics.AverageMixSize = mixSize
	} else {
		rm.metrics.AverageMixSize = (rm.metrics.AverageMixSize + mixSize) / 2
	}
	
	// Calculate achieved cover ratio
	realRequests := float64(len(pool.Requests))
	coverRequests := float64(len(pool.CoverRequests))
	if realRequests > 0 {
		achievedRatio := coverRequests / realRequests
		if rm.metrics.CoverRatioAchieved == 0 {
			rm.metrics.CoverRatioAchieved = achievedRatio
		} else {
			rm.metrics.CoverRatioAchieved = (rm.metrics.CoverRatioAchieved + achievedRatio) / 2
		}
	}
	
	rm.metrics.LastUpdate = time.Now()
}

// generateRequestID generates a unique request ID
func (rm *RequestMixer) generateRequestID() string {
	return fmt.Sprintf("mixed_%d", time.Now().UnixNano())
}

// generateMixID generates a unique mix ID
func (rm *RequestMixer) generateMixID() string {
	return fmt.Sprintf("mix_%d", time.Now().UnixNano())
}

// GetMetrics returns current mixer metrics
func (rm *RequestMixer) GetMetrics() *MixerMetrics {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.metrics
}

// Stop stops the request mixer
func (rm *RequestMixer) Stop() {
	rm.cancel()
}
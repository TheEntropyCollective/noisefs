package relay

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// RequestDistributor manages the distribution of block requests across relays
type RequestDistributor struct {
	pool         *RelayPool
	config       *DistributorConfig
	metrics      *DistributorMetrics
	activeRequests map[string]*DistributedRequest
	mu           sync.RWMutex
}

// DistributorConfig contains configuration for the request distributor
type DistributorConfig struct {
	MaxConcurrentRequests int           // Maximum concurrent requests per relay
	RequestTimeout        time.Duration // Timeout for individual requests
	RetryAttempts         int           // Number of retry attempts
	LoadBalanceStrategy   string        // Strategy for load balancing
	FailoverEnabled       bool          // Enable automatic failover
}

// DistributedRequest represents a request distributed across multiple relays
type DistributedRequest struct {
	ID            string
	BlockIDs      []string
	RelayRequests []*RelayRequest
	StartTime     time.Time
	Status        RequestStatus
	Results       map[string]*BlockResult
	mu            sync.RWMutex
}

// RelayRequest represents a request to a specific relay
type RelayRequest struct {
	ID       string
	RelayID  peer.ID
	BlockID  string
	Status   RequestStatus
	StartTime time.Time
	EndTime  time.Time
	Error    error
	Attempts int
}

// BlockResult contains the result of a block request
type BlockResult struct {
	BlockID   string
	Data      []byte
	RelayID   peer.ID
	Latency   time.Duration
	Success   bool
	Error     error
	Timestamp time.Time
}

// RequestStatus represents the status of a request
type RequestStatus int

const (
	RequestStatusPending RequestStatus = iota
	RequestStatusInProgress
	RequestStatusCompleted
	RequestStatusFailed
	RequestStatusCancelled
)

// DistributorMetrics tracks distributor performance
type DistributorMetrics struct {
	TotalRequests     int64
	SuccessfulRequests int64
	FailedRequests    int64
	AverageLatency    time.Duration
	RelayUtilization  map[peer.ID]float64
	LastUpdate        time.Time
}

// NewRequestDistributor creates a new request distributor
func NewRequestDistributor(pool *RelayPool, config *DistributorConfig) *RequestDistributor {
	return &RequestDistributor{
		pool:           pool,
		config:         config,
		metrics:        &DistributorMetrics{RelayUtilization: make(map[peer.ID]float64)},
		activeRequests: make(map[string]*DistributedRequest),
	}
}

// DistributeRequest distributes a request for multiple blocks across relays
func (d *RequestDistributor) DistributeRequest(ctx context.Context, requestID string, blockIDs []string) (*DistributedRequest, error) {
	// Create distributed request
	request := &DistributedRequest{
		ID:            requestID,
		BlockIDs:      blockIDs,
		RelayRequests: make([]*RelayRequest, 0, len(blockIDs)),
		StartTime:     time.Now(),
		Status:        RequestStatusPending,
		Results:       make(map[string]*BlockResult),
	}
	
	// Store active request
	d.mu.Lock()
	d.activeRequests[requestID] = request
	d.mu.Unlock()
	
	// Select relays for distribution
	relayCount := len(blockIDs)
	if relayCount > 10 {
		relayCount = 10 // Limit to 10 relays max
	}
	
	relays, err := d.pool.SelectRelays(ctx, relayCount)
	if err != nil {
		return nil, fmt.Errorf("failed to select relays: %w", err)
	}
	
	// Distribute blocks across relays
	request.mu.Lock()
	request.Status = RequestStatusInProgress
	
	for i, blockID := range blockIDs {
		relayIndex := i % len(relays)
		relay := relays[relayIndex]
		
		relayRequest := &RelayRequest{
			ID:        fmt.Sprintf("%s-%d", requestID, i),
			RelayID:   relay.ID,
			BlockID:   blockID,
			Status:    RequestStatusPending,
			StartTime: time.Now(),
		}
		
		request.RelayRequests = append(request.RelayRequests, relayRequest)
	}
	request.mu.Unlock()
	
	// Execute requests concurrently
	go d.executeDistributedRequest(ctx, request)
	
	return request, nil
}

// executeDistributedRequest executes all relay requests for a distributed request
func (d *RequestDistributor) executeDistributedRequest(ctx context.Context, request *DistributedRequest) {
	var wg sync.WaitGroup
	
	// Execute each relay request
	for _, relayRequest := range request.RelayRequests {
		wg.Add(1)
		go func(rr *RelayRequest) {
			defer wg.Done()
			d.executeRelayRequest(ctx, rr, request)
		}(relayRequest)
	}
	
	// Wait for all requests to complete
	wg.Wait()
	
	// Update request status
	request.mu.Lock()
	allSuccessful := true
	for _, result := range request.Results {
		if !result.Success {
			allSuccessful = false
			break
		}
	}
	
	if allSuccessful {
		request.Status = RequestStatusCompleted
	} else {
		request.Status = RequestStatusFailed
	}
	request.mu.Unlock()
	
	// Update metrics
	d.updateMetrics(request)
	
	// Clean up active request
	d.mu.Lock()
	delete(d.activeRequests, request.ID)
	d.mu.Unlock()
}

// executeRelayRequest executes a single relay request
func (d *RequestDistributor) executeRelayRequest(ctx context.Context, relayRequest *RelayRequest, distributedRequest *DistributedRequest) {
	relayRequest.Status = RequestStatusInProgress
	
	// Add timeout to context
	ctx, cancel := context.WithTimeout(ctx, d.config.RequestTimeout)
	defer cancel()
	
	// Simulate block retrieval - in real implementation, this would use the relay
	// to retrieve the block from IPFS or other storage
	result := d.simulateBlockRetrieval(ctx, relayRequest)
	
	// Store result
	distributedRequest.mu.Lock()
	distributedRequest.Results[relayRequest.BlockID] = result
	distributedRequest.mu.Unlock()
	
	// Update relay request status
	relayRequest.EndTime = time.Now()
	if result.Success {
		relayRequest.Status = RequestStatusCompleted
	} else {
		relayRequest.Status = RequestStatusFailed
		relayRequest.Error = result.Error
	}
}

// simulateBlockRetrieval simulates retrieving a block through a relay
func (d *RequestDistributor) simulateBlockRetrieval(ctx context.Context, relayRequest *RelayRequest) *BlockResult {
	start := time.Now()
	
	// Simulate network delay
	select {
	case <-time.After(50 * time.Millisecond):
		// Success
		return &BlockResult{
			BlockID:   relayRequest.BlockID,
			Data:      []byte("simulated block data"),
			RelayID:   relayRequest.RelayID,
			Latency:   time.Since(start),
			Success:   true,
			Timestamp: time.Now(),
		}
	case <-ctx.Done():
		// Timeout or cancellation
		return &BlockResult{
			BlockID:   relayRequest.BlockID,
			RelayID:   relayRequest.RelayID,
			Latency:   time.Since(start),
			Success:   false,
			Error:     ctx.Err(),
			Timestamp: time.Now(),
		}
	}
}

// GetRequestStatus returns the status of a distributed request
func (d *RequestDistributor) GetRequestStatus(requestID string) (*DistributedRequest, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	request, exists := d.activeRequests[requestID]
	return request, exists
}

// CancelRequest cancels a distributed request
func (d *RequestDistributor) CancelRequest(requestID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	request, exists := d.activeRequests[requestID]
	if !exists {
		return fmt.Errorf("request not found: %s", requestID)
	}
	
	request.mu.Lock()
	request.Status = RequestStatusCancelled
	request.mu.Unlock()
	
	return nil
}

// GetMetrics returns current distributor metrics
func (d *RequestDistributor) GetMetrics() *DistributorMetrics {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	return d.metrics
}

// updateMetrics updates the distributor metrics
func (d *RequestDistributor) updateMetrics(request *DistributedRequest) {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	d.metrics.TotalRequests++
	
	if request.Status == RequestStatusCompleted {
		d.metrics.SuccessfulRequests++
	} else {
		d.metrics.FailedRequests++
	}
	
	// Update average latency
	totalLatency := time.Duration(0)
	for _, result := range request.Results {
		totalLatency += result.Latency
	}
	avgLatency := totalLatency / time.Duration(len(request.Results))
	
	// Update running average
	if d.metrics.TotalRequests == 1 {
		d.metrics.AverageLatency = avgLatency
	} else {
		d.metrics.AverageLatency = (d.metrics.AverageLatency + avgLatency) / 2
	}
	
	// Update relay utilization
	for _, result := range request.Results {
		if current, exists := d.metrics.RelayUtilization[result.RelayID]; exists {
			d.metrics.RelayUtilization[result.RelayID] = (current + 1) / 2
		} else {
			d.metrics.RelayUtilization[result.RelayID] = 1.0
		}
	}
	
	d.metrics.LastUpdate = time.Now()
}

// GetActiveRequests returns all active requests
func (d *RequestDistributor) GetActiveRequests() []*DistributedRequest {
	d.mu.RLock()
	defer d.mu.RUnlock()
	
	requests := make([]*DistributedRequest, 0, len(d.activeRequests))
	for _, request := range d.activeRequests {
		requests = append(requests, request)
	}
	
	return requests
}
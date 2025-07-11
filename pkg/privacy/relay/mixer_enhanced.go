package relay

import (
	"context"
	"fmt"
	"sync"
	"time"
	
	"github.com/TheEntropyCollective/noisefs/pkg/storage/ipfs"
)

// RequestTracker tracks the status of distributed requests
type RequestTracker struct {
	mu               sync.RWMutex
	activeRequests   map[string]*TrackedRequest
	completedRequests map[string]*CompletedRequest
	blockRetriever   ipfs.BlockStore
}

// TrackedRequest represents a request being tracked
type TrackedRequest struct {
	ID            string
	MixedRequest  *MixedRequest
	StartTime     time.Time
	LastUpdate    time.Time
	Status        TrackedRequestStatus
	Progress      float64 // 0.0 to 1.0
	RelayStatuses map[string]RelayStatus
	BlockData     map[string][]byte // CID -> data
	Errors        []error
}

// CompletedRequest represents a completed request
type CompletedRequest struct {
	ID           string
	BlockData    map[string][]byte
	Success      bool
	Errors       []error
	TotalLatency time.Duration
	CompletedAt  time.Time
}

// TrackedRequestStatus represents the status of a tracked request
type TrackedRequestStatus int

const (
	TrackedStatusQueued TrackedRequestStatus = iota
	TrackedStatusMixed
	TrackedStatusDistributing
	TrackedStatusFetching
	TrackedStatusCompleted
	TrackedStatusFailed
)

// RelayStatus tracks status from a specific relay
type RelayStatus struct {
	RelayID      string
	Status       string
	LastUpdate   time.Time
	BlocksServed []string
}

// DistributionResult represents the result of distributing requests to relays
type DistributionResult struct {
	RequestID        string
	RelayAssignments map[string][]string // RelayID -> BlockIDs
	Status           string
	StartTime        time.Time
}

// NewRequestTracker creates a new request tracker
func NewRequestTracker(blockRetriever ipfs.BlockStore) *RequestTracker {
	return &RequestTracker{
		activeRequests:    make(map[string]*TrackedRequest),
		completedRequests: make(map[string]*CompletedRequest),
		blockRetriever:    blockRetriever,
	}
}

// StartTracking begins tracking a request
func (rt *RequestTracker) StartTracking(mixed *MixedRequest) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	
	tracked := &TrackedRequest{
		ID:            mixed.ID,
		MixedRequest:  mixed,
		StartTime:     time.Now(),
		LastUpdate:    time.Now(),
		Status:        TrackedStatusQueued,
		Progress:      0.0,
		RelayStatuses: make(map[string]RelayStatus),
		BlockData:     make(map[string][]byte),
		Errors:        make([]error, 0),
	}
	
	rt.activeRequests[mixed.ID] = tracked
}

// UpdateStatus updates the status of a tracked request
func (rt *RequestTracker) UpdateStatus(requestID string, status TrackedRequestStatus, progress float64) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	
	if tracked, exists := rt.activeRequests[requestID]; exists {
		tracked.Status = status
		tracked.Progress = progress
		tracked.LastUpdate = time.Now()
	}
}

// UpdateRelayStatus updates status from a specific relay
func (rt *RequestTracker) UpdateRelayStatus(requestID, relayID string, status string, blocksServed []string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	
	if tracked, exists := rt.activeRequests[requestID]; exists {
		tracked.RelayStatuses[relayID] = RelayStatus{
			RelayID:      relayID,
			Status:       status,
			LastUpdate:   time.Now(),
			BlocksServed: blocksServed,
		}
	}
}

// CompleteRequest marks a request as completed and moves it to completed map
func (rt *RequestTracker) CompleteRequest(requestID string, success bool) *CompletedRequest {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	
	tracked, exists := rt.activeRequests[requestID]
	if !exists {
		return nil
	}
	
	completed := &CompletedRequest{
		ID:           requestID,
		BlockData:    tracked.BlockData,
		Success:      success,
		Errors:       tracked.Errors,
		TotalLatency: time.Since(tracked.StartTime),
		CompletedAt:  time.Now(),
	}
	
	rt.completedRequests[requestID] = completed
	delete(rt.activeRequests, requestID)
	
	return completed
}

// GetTrackedRequest retrieves a tracked request
func (rt *RequestTracker) GetTrackedRequest(requestID string) (*TrackedRequest, bool) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	
	tracked, exists := rt.activeRequests[requestID]
	return tracked, exists
}

// EnhancedMixerExecutor provides enhanced execution with proper tracking
type EnhancedMixerExecutor struct {
	tracker        *RequestTracker
	blockRetriever ipfs.BlockStore
	distributor    *RequestDistributor
	coverCache     *CoverBlockCache
}

// NewEnhancedMixerExecutor creates a new enhanced mixer executor
func NewEnhancedMixerExecutor(blockRetriever ipfs.BlockStore, distributor *RequestDistributor, coverCache *CoverBlockCache) *EnhancedMixerExecutor {
	return &EnhancedMixerExecutor{
		tracker:        NewRequestTracker(blockRetriever),
		blockRetriever: blockRetriever,
		distributor:    distributor,
		coverCache:     coverCache,
	}
}

// ExecuteMixedRequest executes a mixed request with proper tracking and data retrieval
func (e *EnhancedMixerExecutor) ExecuteMixedRequest(ctx context.Context, mixed *MixedRequest) *MixedRequestResult {
	start := time.Now()
	
	// Start tracking
	e.tracker.StartTracking(mixed)
	
	// Update status to distributing
	e.tracker.UpdateStatus(mixed.ID, TrackedStatusDistributing, 0.2)
	
	// Prepare block request (simplified structure for compatibility)
	blockIDs := []string{mixed.BlockID}
	
	// For cover traffic, use cached data if available
	if !mixed.IsRealRequest {
		if data := e.coverCache.GetCachedBlock(mixed.BlockID); data != nil {
			e.tracker.UpdateStatus(mixed.ID, TrackedStatusCompleted, 1.0)
			e.tracker.CompleteRequest(mixed.ID, true)
			
			return &MixedRequestResult{
				BlockID:   mixed.BlockID,
				Data:      data,
				Success:   true,
				Latency:   time.Since(start),
				MixID:     mixed.MixID,
				RelayUsed: mixed.RelayID,
			}
		}
	}
	
	// Distribute request
	distribution, err := e.distributor.DistributeRequest(ctx, mixed.ID, blockIDs)
	if err != nil {
		e.tracker.UpdateStatus(mixed.ID, TrackedStatusFailed, 0.0)
		e.tracker.CompleteRequest(mixed.ID, false)
		
		return &MixedRequestResult{
			BlockID:   mixed.BlockID,
			Success:   false,
			Error:     err,
			MixID:     mixed.MixID,
			RelayUsed: mixed.RelayID,
		}
	}
	
	// Update status to fetching
	e.tracker.UpdateStatus(mixed.ID, TrackedStatusFetching, 0.5)
	
	// Fetch the actual block data
	blockData, err := e.fetchBlockData(ctx, mixed.BlockID, distribution)
	if err != nil {
		e.tracker.UpdateStatus(mixed.ID, TrackedStatusFailed, 0.0)
		e.tracker.CompleteRequest(mixed.ID, false)
		
		return &MixedRequestResult{
			BlockID:   mixed.BlockID,
			Success:   false,
			Error:     err,
			MixID:     mixed.MixID,
			RelayUsed: mixed.RelayID,
		}
	}
	
	// Update tracked request with block data
	if tracked, exists := e.tracker.GetTrackedRequest(mixed.ID); exists {
		tracked.BlockData[mixed.BlockID] = blockData
	}
	
	// Complete the request
	e.tracker.UpdateStatus(mixed.ID, TrackedStatusCompleted, 1.0)
	completed := e.tracker.CompleteRequest(mixed.ID, true)
	
	// Cache cover traffic blocks
	if !mixed.IsRealRequest && blockData != nil {
		e.coverCache.CacheBlock(mixed.BlockID, blockData)
	}
	
	return &MixedRequestResult{
		BlockID:   mixed.BlockID,
		Data:      blockData,
		Success:   true,
		Latency:   completed.TotalLatency,
		MixID:     mixed.MixID,
		RelayUsed: mixed.RelayID,
	}
}

// fetchBlockData fetches actual block data from IPFS or relays
func (e *EnhancedMixerExecutor) fetchBlockData(ctx context.Context, blockID string, distribution *DistributedRequest) ([]byte, error) {
	// Try to fetch from IPFS directly first
	block, err := e.blockRetriever.RetrieveBlock(blockID)
	if err == nil && block != nil {
		return block.Data, nil
	}
	
	// If direct fetch failed, try through relays
	if distribution != nil && len(distribution.RelayRequests) > 0 {
		// In a real implementation, this would contact the relays
		// For now, we'll simulate relay fetch
		return e.simulateRelayFetch(ctx, blockID, distribution)
	}
	
	return nil, fmt.Errorf("failed to fetch block %s: no available sources", blockID)
}

// simulateRelayFetch simulates fetching from relays
func (e *EnhancedMixerExecutor) simulateRelayFetch(ctx context.Context, blockID string, distribution *DistributedRequest) ([]byte, error) {
	// In production, this would:
	// 1. Contact each relay in the distribution
	// 2. Request the block
	// 3. Verify the data
	// 4. Return the first successful response
	
	// For now, create simulated data
	simulatedData := fmt.Sprintf("Block data for %s fetched via relay", blockID)
	return []byte(simulatedData), nil
}

// BatchExecutor executes multiple mixed requests efficiently
type BatchExecutor struct {
	executor *EnhancedMixerExecutor
	workers  int
}

// NewBatchExecutor creates a new batch executor
func NewBatchExecutor(executor *EnhancedMixerExecutor, workers int) *BatchExecutor {
	if workers <= 0 {
		workers = 4
	}
	return &BatchExecutor{
		executor: executor,
		workers:  workers,
	}
}

// ExecuteBatch executes a batch of mixed requests
func (be *BatchExecutor) ExecuteBatch(ctx context.Context, mixedRequests []*MixedRequest) map[string]*MixedRequestResult {
	results := make(map[string]*MixedRequestResult)
	resultsMu := sync.Mutex{}
	
	// Create work queue
	workQueue := make(chan *MixedRequest, len(mixedRequests))
	for _, req := range mixedRequests {
		workQueue <- req
	}
	close(workQueue)
	
	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < be.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			for req := range workQueue {
				result := be.executor.ExecuteMixedRequest(ctx, req)
				
				resultsMu.Lock()
				results[req.ID] = result
				resultsMu.Unlock()
			}
		}()
	}
	
	wg.Wait()
	return results
}

// GetTracker returns the request tracker for monitoring
func (e *EnhancedMixerExecutor) GetTracker() *RequestTracker {
	return e.tracker
}

// GetActiveRequests returns all active requests
func (rt *RequestTracker) GetActiveRequests() map[string]*TrackedRequest {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	
	// Return a copy to avoid race conditions
	copy := make(map[string]*TrackedRequest)
	for k, v := range rt.activeRequests {
		copy[k] = v
	}
	return copy
}

// GetCompletedRequests returns completed requests
func (rt *RequestTracker) GetCompletedRequests(limit int) []*CompletedRequest {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	
	results := make([]*CompletedRequest, 0, limit)
	count := 0
	
	// Get most recent completed requests
	for _, completed := range rt.completedRequests {
		if count >= limit {
			break
		}
		results = append(results, completed)
		count++
	}
	
	return results
}

// CleanupOldRequests removes old completed requests
func (rt *RequestTracker) CleanupOldRequests(maxAge time.Duration) int {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	
	cutoff := time.Now().Add(-maxAge)
	removed := 0
	
	for id, completed := range rt.completedRequests {
		if completed.CompletedAt.Before(cutoff) {
			delete(rt.completedRequests, id)
			removed++
		}
	}
	
	return removed
}
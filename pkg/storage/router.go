package storage

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
)

// Router manages block distribution across multiple storage backends
type Router struct {
	manager      *Manager
	config       *DistributionConfig
	strategies   map[string]DistributionStrategy
	loadBalancer *LoadBalancer
}

// NewRouter creates a new storage router
func NewRouter(manager *Manager, config *DistributionConfig) *Router {
	router := &Router{
		manager:    manager,
		config:     config,
		strategies: make(map[string]DistributionStrategy),
	}

	// Register built-in distribution strategies
	router.RegisterStrategy("single", &SingleBackendStrategy{})
	router.RegisterStrategy("replicate", &ReplicationStrategy{})
	router.RegisterStrategy("stripe", &StripingStrategy{})
	router.RegisterStrategy("smart", &SmartDistributionStrategy{})

	// Initialize load balancer
	router.loadBalancer = NewLoadBalancer(config.LoadBalancing)

	return router
}

// RegisterStrategy registers a new distribution strategy
func (r *Router) RegisterStrategy(name string, strategy DistributionStrategy) {
	r.strategies[name] = strategy
}

// Put stores a block using the configured distribution strategy
func (r *Router) Put(ctx context.Context, block *blocks.Block) (*BlockAddress, error) {
	strategy, exists := r.strategies[r.config.Strategy]
	if !exists {
		return nil, fmt.Errorf("unknown distribution strategy: %s", r.config.Strategy)
	}

	return strategy.Put(ctx, r, block)
}

// Get retrieves a block using intelligent backend selection
func (r *Router) Get(ctx context.Context, address *BlockAddress) (*blocks.Block, error) {
	// Try to get from the backend specified in the address first
	if address.BackendType != "" {
		if backend, exists := r.manager.GetBackend(address.BackendType); exists && backend.IsConnected() {
			block, err := backend.Get(ctx, address)
			if err == nil {
				return block, nil
			}
			// If specific backend fails, continue to try others
		}
	}

	// Try other available backends
	backends := r.manager.GetBackendsByPriority()

	var lastErr error
	for _, backend := range backends {
		// Create a copy of the address for this backend
		backendAddress := *address
		backendAddress.BackendType = backend.GetBackendInfo().Type

		block, err := backend.Get(ctx, &backendAddress)
		if err == nil {
			return block, nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return nil, fmt.Errorf("failed to retrieve block from any backend: %w", lastErr)
	}

	return nil, NewNotFoundError("all", address)
}

// Has checks if a block exists in any backend
func (r *Router) Has(ctx context.Context, address *BlockAddress) (bool, error) {
	// Check specific backend first if specified
	if address.BackendType != "" {
		if backend, exists := r.manager.GetBackend(address.BackendType); exists && backend.IsConnected() {
			exists, err := backend.Has(ctx, address)
			if err == nil && exists {
				return true, nil
			}
		}
	}

	// Check other backends
	backends := r.manager.GetAvailableBackends()

	for _, backend := range backends {
		backendAddress := *address
		backendAddress.BackendType = backend.GetBackendInfo().Type

		exists, err := backend.Has(ctx, &backendAddress)
		if err == nil && exists {
			return true, nil
		}
	}

	return false, nil
}

// Delete removes a block from all backends where it exists
func (r *Router) Delete(ctx context.Context, address *BlockAddress) error {
	backends := r.manager.GetAvailableBackends()

	var errors ErrorAggregator
	var deletedCount int

	for _, backend := range backends {
		backendAddress := *address
		backendAddress.BackendType = backend.GetBackendInfo().Type

		// Check if block exists before trying to delete
		exists, err := backend.Has(ctx, &backendAddress)
		if err != nil {
			errors.Add(err)
			continue
		}

		if exists {
			if err := backend.Delete(ctx, &backendAddress); err != nil {
				errors.Add(err)
			} else {
				deletedCount++
			}
		}
	}

	if deletedCount == 0 && errors.HasErrors() {
		return errors.CreateAggregateError()
	}

	return nil
}

// PutMany stores multiple blocks efficiently
func (r *Router) PutMany(ctx context.Context, blocks []*blocks.Block) ([]*BlockAddress, error) {
	strategy, exists := r.strategies[r.config.Strategy]
	if !exists {
		return nil, fmt.Errorf("unknown distribution strategy: %s", r.config.Strategy)
	}

	// Check if strategy supports batch operations
	if batchStrategy, ok := strategy.(BatchDistributionStrategy); ok {
		return batchStrategy.PutMany(ctx, r, blocks)
	}

	// Fallback to individual puts
	addresses := make([]*BlockAddress, len(blocks))
	for i, block := range blocks {
		address, err := strategy.Put(ctx, r, block)
		if err != nil {
			return nil, fmt.Errorf("failed to store block %d: %w", i, err)
		}
		addresses[i] = address
	}

	return addresses, nil
}

// GetMany retrieves multiple blocks efficiently
func (r *Router) GetMany(ctx context.Context, addresses []*BlockAddress) ([]*blocks.Block, error) {
	// Group addresses by backend type for efficient batch retrieval
	backendGroups := make(map[string][]*BlockAddress)

	for _, address := range addresses {
		backendType := address.BackendType
		if backendType == "" {
			// Try to determine backend from available backends
			for _, backend := range r.manager.GetAvailableBackends() {
				backendType = backend.GetBackendInfo().Type
				break
			}
		}

		backendGroups[backendType] = append(backendGroups[backendType], address)
	}

	// Retrieve blocks from each backend group
	allBlocks := make([]*blocks.Block, len(addresses))
	addressToIndex := make(map[*BlockAddress]int)

	for i, addr := range addresses {
		addressToIndex[addr] = i
	}

	var errors ErrorAggregator

	for backendType, groupAddresses := range backendGroups {
		backend, exists := r.manager.GetBackend(backendType)
		if !exists || !backend.IsConnected() {
			// Try to retrieve from other backends
			for _, addr := range groupAddresses {
				block, err := r.Get(ctx, addr)
				if err != nil {
					errors.Add(err)
					continue
				}
				allBlocks[addressToIndex[addr]] = block
			}
			continue
		}

		groupBlocks, err := backend.GetMany(ctx, groupAddresses)
		if err != nil {
			// Fallback to individual gets
			for _, addr := range groupAddresses {
				block, err := r.Get(ctx, addr)
				if err != nil {
					errors.Add(err)
					continue
				}
				allBlocks[addressToIndex[addr]] = block
			}
			continue
		}

		// Map blocks back to their positions in the result array
		for i, addr := range groupAddresses {
			allBlocks[addressToIndex[addr]] = groupBlocks[i]
		}
	}

	// Check for any missing blocks
	for i, block := range allBlocks {
		if block == nil {
			errors.Add(fmt.Errorf("failed to retrieve block at index %d", i))
		}
	}

	if errors.HasErrors() {
		return nil, errors.CreateAggregateError()
	}

	return allBlocks, nil
}

// Pin pins a block in appropriate backends
func (r *Router) Pin(ctx context.Context, address *BlockAddress) error {
	backends := r.getBackendsForAddress(address)

	var errors ErrorAggregator
	var pinnedCount int

	for _, backend := range backends {
		backendAddress := *address
		backendAddress.BackendType = backend.GetBackendInfo().Type

		if err := backend.Pin(ctx, &backendAddress); err != nil {
			errors.Add(err)
		} else {
			pinnedCount++
		}
	}

	if pinnedCount == 0 && errors.HasErrors() {
		return errors.CreateAggregateError()
	}

	return nil
}

// Unpin unpins a block from all backends
func (r *Router) Unpin(ctx context.Context, address *BlockAddress) error {
	backends := r.getBackendsForAddress(address)

	var errors ErrorAggregator

	for _, backend := range backends {
		backendAddress := *address
		backendAddress.BackendType = backend.GetBackendInfo().Type

		if err := backend.Unpin(ctx, &backendAddress); err != nil {
			errors.Add(err)
		}
	}

	if errors.HasErrors() {
		return errors.CreateAggregateError()
	}

	return nil
}

// SelectBackend selects the best backend for an operation based on criteria
func (r *Router) SelectBackend(ctx context.Context, criteria SelectionCriteria) (Backend, error) {
	backends := r.manager.GetAvailableBackends()

	// Filter by required capabilities
	if len(criteria.RequiredCapabilities) > 0 {
		filtered := make(map[string]Backend)
		for name, backend := range backends {
			info := backend.GetBackendInfo()
			hasAllCaps := true

			for _, requiredCap := range criteria.RequiredCapabilities {
				found := false
				for _, cap := range info.Capabilities {
					if cap == requiredCap {
						found = true
						break
					}
				}
				if !found {
					hasAllCaps = false
					break
				}
			}

			if hasAllCaps {
				filtered[name] = backend
			}
		}
		backends = filtered
	}

	if len(backends) == 0 {
		return nil, fmt.Errorf("no backends available matching criteria")
	}

	// Use load balancer to select from filtered backends
	return r.loadBalancer.SelectBackend(backends, criteria)
}

// Helper methods

func (r *Router) getBackendsForAddress(address *BlockAddress) []Backend {
	if address.BackendType != "" {
		if backend, exists := r.manager.GetBackend(address.BackendType); exists {
			return []Backend{backend}
		}
	}

	// Return all available backends
	backends := r.manager.GetAvailableBackends()
	result := make([]Backend, 0, len(backends))
	for _, backend := range backends {
		result = append(result, backend)
	}
	return result
}

// Distribution strategies

// DistributionStrategy defines how blocks are distributed across backends
type DistributionStrategy interface {
	Put(ctx context.Context, router *Router, block *blocks.Block) (*BlockAddress, error)
}

// BatchDistributionStrategy extends DistributionStrategy for batch operations
type BatchDistributionStrategy interface {
	DistributionStrategy
	PutMany(ctx context.Context, router *Router, blocks []*blocks.Block) ([]*BlockAddress, error)
}

// SingleBackendStrategy stores blocks in a single backend
type SingleBackendStrategy struct{}

func (s *SingleBackendStrategy) Put(ctx context.Context, router *Router, block *blocks.Block) (*BlockAddress, error) {
	criteria := SelectionCriteria{
		RequiredCapabilities: []string{CapabilityContentAddress},
	}

	backend, err := router.SelectBackend(ctx, criteria)
	if err != nil {
		return nil, err
	}

	return backend.Put(ctx, block)
}

// ReplicationStrategy replicates blocks across multiple backends
type ReplicationStrategy struct{}

func (s *ReplicationStrategy) Put(ctx context.Context, router *Router, block *blocks.Block) (*BlockAddress, error) {
	config := router.config.Replication
	if config == nil {
		return nil, fmt.Errorf("replication configuration not found")
	}

	backends := router.manager.GetHealthyBackends()
	if len(backends) < config.MinReplicas {
		return nil, fmt.Errorf("insufficient healthy backends for replication: need %d, have %d",
			config.MinReplicas, len(backends))
	}

	// Select backends for replication
	selectedBackends := s.selectBackendsForReplication(backends, config)

	var primaryAddress *BlockAddress
	var errors ErrorAggregator
	successCount := 0

	for _, backend := range selectedBackends {
		address, err := backend.Put(ctx, block)
		if err != nil {
			errors.Add(err)
			continue
		}

		if primaryAddress == nil {
			primaryAddress = address
		}
		successCount++

		if successCount >= config.MinReplicas {
			break
		}
	}

	if successCount < config.MinReplicas {
		return nil, fmt.Errorf("failed to achieve minimum replication: succeeded %d, required %d",
			successCount, config.MinReplicas)
	}

	return primaryAddress, nil
}

func (s *ReplicationStrategy) selectBackendsForReplication(backends map[string]Backend, config *ReplicationConfig) []Backend {
	var result []Backend

	// Convert map to slice
	backendList := make([]Backend, 0, len(backends))
	for _, backend := range backends {
		backendList = append(backendList, backend)
	}

	// Shuffle for random selection
	rand.Shuffle(len(backendList), func(i, j int) {
		backendList[i], backendList[j] = backendList[j], backendList[i]
	})

	// Select up to MaxReplicas
	maxSelect := config.MaxReplicas
	if maxSelect > len(backendList) {
		maxSelect = len(backendList)
	}

	for i := 0; i < maxSelect; i++ {
		result = append(result, backendList[i])
	}

	return result
}

// StripingStrategy stripes blocks across backends (not implemented for security reasons)
type StripingStrategy struct{}

func (s *StripingStrategy) Put(ctx context.Context, router *Router, block *blocks.Block) (*BlockAddress, error) {
	// Note: Striping is not recommended for NoiseFS as it reduces privacy
	// This is a placeholder implementation that falls back to single backend
	single := &SingleBackendStrategy{}
	return single.Put(ctx, router, block)
}

// SmartDistributionStrategy uses intelligent backend selection
type SmartDistributionStrategy struct{}

func (s *SmartDistributionStrategy) Put(ctx context.Context, router *Router, block *blocks.Block) (*BlockAddress, error) {
	// Use replication if multiple backends are available
	backends := router.manager.GetHealthyBackends()

	if len(backends) >= 2 {
		replication := &ReplicationStrategy{}
		return replication.Put(ctx, router, block)
	}

	// Fall back to single backend
	single := &SingleBackendStrategy{}
	return single.Put(ctx, router, block)
}

// LoadBalancer handles backend selection for optimal performance
type LoadBalancer struct {
	config  *LoadBalancingConfig
	metrics map[string]*BackendMetrics
	mutex   sync.RWMutex
}

type BackendMetrics struct {
	RequestCount   int64
	SuccessRate    float64
	AverageLatency time.Duration
	LastUsed       time.Time
}

func NewLoadBalancer(config *LoadBalancingConfig) *LoadBalancer {
	return &LoadBalancer{
		config:  config,
		metrics: make(map[string]*BackendMetrics),
	}
}

func (lb *LoadBalancer) SelectBackend(backends map[string]Backend, criteria SelectionCriteria) (Backend, error) {
	if len(backends) == 0 {
		return nil, fmt.Errorf("no backends available")
	}

	if len(backends) == 1 {
		for _, backend := range backends {
			return backend, nil
		}
	}

	switch lb.config.Algorithm {
	case "round_robin":
		return lb.selectRoundRobin(backends)
	case "weighted":
		return lb.selectWeighted(backends)
	case "least_connections":
		return lb.selectLeastConnections(backends)
	case "performance":
		return lb.selectByPerformance(backends, criteria)
	default:
		// Default to performance-based selection
		return lb.selectByPerformance(backends, criteria)
	}
}

func (lb *LoadBalancer) selectRoundRobin(backends map[string]Backend) (Backend, error) {
	// Simple round-robin based on timestamps
	var selected Backend
	var oldestTime time.Time = time.Now()

	lb.mutex.RLock()
	defer lb.mutex.RUnlock()

	for name, backend := range backends {
		metrics, exists := lb.metrics[name]
		if !exists || metrics.LastUsed.Before(oldestTime) {
			oldestTime = metrics.LastUsed
			selected = backend
		}
	}

	if selected == nil {
		// Fallback to first available
		for _, backend := range backends {
			return backend, nil
		}
	}

	return selected, nil
}

func (lb *LoadBalancer) selectWeighted(backends map[string]Backend) (Backend, error) {
	// Weight based on success rate and latency
	return lb.selectByPerformance(backends, SelectionCriteria{})
}

func (lb *LoadBalancer) selectLeastConnections(backends map[string]Backend) (Backend, error) {
	var selected Backend
	var minConnections int64 = 999999999

	lb.mutex.RLock()
	defer lb.mutex.RUnlock()

	for name, backend := range backends {
		metrics, exists := lb.metrics[name]
		if !exists {
			return backend, nil // New backend, use it
		}

		if metrics.RequestCount < minConnections {
			minConnections = metrics.RequestCount
			selected = backend
		}
	}

	if selected == nil {
		for _, backend := range backends {
			return backend, nil
		}
	}

	return selected, nil
}

func (lb *LoadBalancer) selectByPerformance(backends map[string]Backend, criteria SelectionCriteria) (Backend, error) {
	var bestBackend Backend
	var bestScore float64

	lb.mutex.RLock()
	defer lb.mutex.RUnlock()

	for name, backend := range backends {
		if lb.config.RequireHealthy {
			status := backend.HealthCheck(context.Background())
			if !status.Healthy {
				continue
			}
		}

		metrics, exists := lb.metrics[name]
		if !exists {
			// New backend, give it priority
			return backend, nil
		}

		// Calculate performance score
		score := lb.calculatePerformanceScore(metrics, criteria)

		if bestBackend == nil || score > bestScore {
			bestBackend = backend
			bestScore = score
		}
	}

	if bestBackend == nil {
		// No healthy backends found, return first available
		for _, backend := range backends {
			return backend, nil
		}
		return nil, fmt.Errorf("no suitable backends found")
	}

	return bestBackend, nil
}

func (lb *LoadBalancer) calculatePerformanceScore(metrics *BackendMetrics, criteria SelectionCriteria) float64 {
	score := 0.0

	// Success rate component (0-0.5)
	score += metrics.SuccessRate * 0.5

	// Latency component (0-0.3)
	if criteria.MaxLatency > 0 {
		maxLatencyDuration := time.Duration(criteria.MaxLatency * float64(time.Millisecond))
		if metrics.AverageLatency <= maxLatencyDuration {
			latencyScore := 1.0 - float64(metrics.AverageLatency)/float64(maxLatencyDuration)
			score += latencyScore * 0.3
		}
	}

	// Recency component (0-0.2) - prefer less recently used backends
	timeSinceUse := time.Since(metrics.LastUsed)
	recencyScore := float64(timeSinceUse) / float64(time.Hour)
	if recencyScore > 1.0 {
		recencyScore = 1.0
	}
	score += recencyScore * 0.2

	return score
}

func (lb *LoadBalancer) UpdateMetrics(backendName string, success bool, latency time.Duration) {
	lb.mutex.Lock()
	defer lb.mutex.Unlock()

	metrics, exists := lb.metrics[backendName]
	if !exists {
		metrics = &BackendMetrics{}
		lb.metrics[backendName] = metrics
	}

	metrics.RequestCount++
	metrics.LastUsed = time.Now()

	// Update success rate with exponential moving average
	if success {
		if metrics.SuccessRate == 0 {
			metrics.SuccessRate = 1.0
		} else {
			alpha := 0.1
			metrics.SuccessRate = metrics.SuccessRate*(1-alpha) + alpha
		}
	} else {
		alpha := 0.1
		metrics.SuccessRate = metrics.SuccessRate * (1 - alpha)
	}

	// Update average latency
	if metrics.AverageLatency == 0 {
		metrics.AverageLatency = latency
	} else {
		alpha := 0.1
		metrics.AverageLatency = time.Duration(
			float64(metrics.AverageLatency)*(1-alpha) + float64(latency)*alpha,
		)
	}
}

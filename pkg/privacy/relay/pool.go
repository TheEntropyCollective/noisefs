package relay

import (
	"context"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// RelayPool manages a pool of relay nodes for privacy-preserving request distribution
type RelayPool struct {
	mu sync.RWMutex
	
	// Pool of available relay nodes
	relays map[peer.ID]*RelayNode
	
	// Configuration
	config *PoolConfig
	
	// Selection strategy
	selector RelaySelector
	
	// Health monitoring
	healthMonitor *RelayHealthMonitor
	
	// Metrics
	metrics *PoolMetrics
}

// RelayNode represents a single relay node in the pool
type RelayNode struct {
	ID          peer.ID
	Addresses   []string
	Capabilities []Capability
	Health      *HealthStatus
	Performance *PerformanceMetrics
	LastUsed    time.Time
	CreatedAt   time.Time
}

// PoolConfig contains configuration for the relay pool
type PoolConfig struct {
	MaxRelays        int           // Maximum number of relays to maintain
	MinRelays        int           // Minimum number of relays to maintain
	HealthCheckInterval time.Duration // How often to check relay health
	MaxRelayAge      time.Duration // Maximum age for a relay before refresh
	LoadBalanceStrategy string     // "round_robin", "least_loaded", "random"
	PrivacyLevel     int           // 1-3 hops for privacy routing
}

// Capability represents what a relay node can do
type Capability string

const (
	CapabilityBlockRetrieval Capability = "block_retrieval"
	CapabilityCoverTraffic   Capability = "cover_traffic"
	CapabilityRequestMixing  Capability = "request_mixing"
	CapabilityEncryption     Capability = "encryption"
)

// HealthStatus tracks the health of a relay node
type HealthStatus struct {
	IsHealthy    bool
	LastCheck    time.Time
	FailureCount int
	Latency      time.Duration
	Bandwidth    float64 // MB/s
	Reliability  float64 // 0-1 success rate
}

// PerformanceMetrics tracks performance statistics
type PerformanceMetrics struct {
	TotalRequests   int64
	SuccessfulRequests int64
	FailedRequests  int64
	AverageLatency  time.Duration
	TotalBandwidth  float64
	LastUpdate      time.Time
}

// RelaySelector interface for different relay selection strategies
type RelaySelector interface {
	SelectRelays(ctx context.Context, pool *RelayPool, count int) ([]*RelayNode, error)
}


// PoolMetrics tracks overall pool statistics
type PoolMetrics struct {
	TotalRelays     int
	HealthyRelays   int
	UnhealthyRelays int
	TotalRequests   int64
	SuccessRate     float64
	AverageLatency  time.Duration
	LastUpdate      time.Time
}

// NewRelayPool creates a new relay pool with the given configuration
func NewRelayPool(config *PoolConfig) *RelayPool {
	pool := &RelayPool{
		relays:  make(map[peer.ID]*RelayNode),
		config:  config,
		metrics: &PoolMetrics{},
	}
	
	// Initialize selector based on config
	switch config.LoadBalanceStrategy {
	case "round_robin":
		pool.selector = NewRoundRobinSelector()
	case "least_loaded":
		pool.selector = NewLeastLoadedSelector()
	case "random":
		pool.selector = NewRandomSelector()
	default:
		pool.selector = NewRandomSelector()
	}
	
	// Initialize health monitor
	pool.healthMonitor = NewRelayHealthMonitor(pool, config.HealthCheckInterval)
	
	return pool
}

// AddRelay adds a new relay node to the pool
func (p *RelayPool) AddRelay(ctx context.Context, peerID peer.ID, addresses []string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	// Check if relay already exists
	if _, exists := p.relays[peerID]; exists {
		return nil // Already exists
	}
	
	// Check pool capacity
	if len(p.relays) >= p.config.MaxRelays {
		// Remove oldest relay to make room
		p.removeOldestRelay()
	}
	
	// Create new relay node
	relay := &RelayNode{
		ID:          peerID,
		Addresses:   addresses,
		Capabilities: []Capability{CapabilityBlockRetrieval}, // Default capability
		Health:      &HealthStatus{IsHealthy: true, LastCheck: time.Now()},
		Performance: &PerformanceMetrics{LastUpdate: time.Now()},
		CreatedAt:   time.Now(),
	}
	
	p.relays[peerID] = relay
	p.updateMetrics()
	
	return nil
}

// RemoveRelay removes a relay node from the pool
func (p *RelayPool) RemoveRelay(peerID peer.ID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	delete(p.relays, peerID)
	p.updateMetrics()
}

// GetHealthyRelays returns all healthy relays
func (p *RelayPool) GetHealthyRelays() []*RelayNode {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	var healthy []*RelayNode
	for _, relay := range p.relays {
		if relay.Health.IsHealthy {
			healthy = append(healthy, relay)
		}
	}
	
	return healthy
}

// SelectRelays selects the specified number of relays using the configured strategy
func (p *RelayPool) SelectRelays(ctx context.Context, count int) ([]*RelayNode, error) {
	return p.selector.SelectRelays(ctx, p, count)
}

// UpdateRelayHealth updates the health status of a relay
func (p *RelayPool) UpdateRelayHealth(peerID peer.ID, health *HealthStatus) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if relay, exists := p.relays[peerID]; exists {
		relay.Health = health
		relay.LastUsed = time.Now()
		p.updateMetrics()
	}
}

// UpdateRelayPerformance updates the performance metrics of a relay
func (p *RelayPool) UpdateRelayPerformance(peerID peer.ID, metrics *PerformanceMetrics) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if relay, exists := p.relays[peerID]; exists {
		relay.Performance = metrics
		relay.LastUsed = time.Now()
		p.updateMetrics() // Update pool-level metrics when relay performance is updated
	}
}

// GetMetrics returns current pool metrics
func (p *RelayPool) GetMetrics() *PoolMetrics {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	return p.metrics
}

// Start starts the relay pool and health monitoring
func (p *RelayPool) Start(ctx context.Context) error {
	return p.healthMonitor.Start(ctx)
}

// Stop stops the relay pool and health monitoring
func (p *RelayPool) Stop() error {
	return p.healthMonitor.Stop()
}

// removeOldestRelay removes the oldest relay to make room for new ones
func (p *RelayPool) removeOldestRelay() {
	var oldest *RelayNode
	var oldestID peer.ID
	
	for id, relay := range p.relays {
		if oldest == nil || relay.CreatedAt.Before(oldest.CreatedAt) {
			oldest = relay
			oldestID = id
		}
	}
	
	if oldest != nil {
		delete(p.relays, oldestID)
	}
}

// updateMetrics updates the pool metrics
func (p *RelayPool) updateMetrics() {
	p.metrics.TotalRelays = len(p.relays)
	p.metrics.HealthyRelays = 0
	p.metrics.UnhealthyRelays = 0
	
	var totalLatency time.Duration
	var totalSuccess int64
	var totalRequests int64
	
	for _, relay := range p.relays {
		if relay.Health.IsHealthy {
			p.metrics.HealthyRelays++
		} else {
			p.metrics.UnhealthyRelays++
		}
		
		totalLatency += relay.Performance.AverageLatency
		totalSuccess += relay.Performance.SuccessfulRequests
		totalRequests += relay.Performance.TotalRequests
	}
	
	if len(p.relays) > 0 {
		p.metrics.AverageLatency = totalLatency / time.Duration(len(p.relays))
	}
	
	if totalRequests > 0 {
		p.metrics.SuccessRate = float64(totalSuccess) / float64(totalRequests)
	}
	
	p.metrics.TotalRequests = totalRequests
	p.metrics.LastUpdate = time.Now()
}
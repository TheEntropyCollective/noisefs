package relay

import (
	"context"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// HealthMonitor monitors the health of relay nodes
type HealthMonitor struct {
	pool     *RelayPool
	interval time.Duration
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// RelayHealthMonitor is an alias for compatibility
type RelayHealthMonitor = HealthMonitor

// HealthCheck represents a health check operation
type HealthCheck struct {
	PeerID    peer.ID
	Timestamp time.Time
	Success   bool
	Latency   time.Duration
	Error     error
}

// HealthChecker interface for different health check strategies
type HealthChecker interface {
	CheckHealth(ctx context.Context, relay *RelayNode) (*HealthStatus, error)
}

// PingHealthChecker implements health checking via ping
type PingHealthChecker struct {
	timeout time.Duration
}

func NewPingHealthChecker(timeout time.Duration) *PingHealthChecker {
	return &PingHealthChecker{timeout: timeout}
}

func (p *PingHealthChecker) CheckHealth(ctx context.Context, relay *RelayNode) (*HealthStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	
	start := time.Now()
	
	// Simulate ping - in real implementation, this would ping the relay
	// For now, we'll simulate with a short delay
	select {
	case <-time.After(10 * time.Millisecond):
		// Success
		latency := time.Since(start)
		return &HealthStatus{
			IsHealthy:    true,
			LastCheck:    time.Now(),
			FailureCount: 0,
			Latency:      latency,
			Bandwidth:    relay.Health.Bandwidth, // Keep previous value
			Reliability:  0.95,                   // Simulate good reliability
		}, nil
	case <-ctx.Done():
		// Timeout
		return &HealthStatus{
			IsHealthy:    false,
			LastCheck:    time.Now(),
			FailureCount: relay.Health.FailureCount + 1,
			Latency:      p.timeout,
			Bandwidth:    0,
			Reliability:  0.0,
		}, ctx.Err()
	}
}

// BlockRetrievalHealthChecker implements health checking via block retrieval
type BlockRetrievalHealthChecker struct {
	timeout   time.Duration
	testBlock string // Test block to retrieve
}

func NewBlockRetrievalHealthChecker(timeout time.Duration, testBlock string) *BlockRetrievalHealthChecker {
	return &BlockRetrievalHealthChecker{
		timeout:   timeout,
		testBlock: testBlock,
	}
}

func (b *BlockRetrievalHealthChecker) CheckHealth(ctx context.Context, relay *RelayNode) (*HealthStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()
	
	start := time.Now()
	
	// Simulate block retrieval - in real implementation, this would retrieve a test block
	// For now, we'll simulate with a delay based on relay performance
	delay := relay.Performance.AverageLatency
	if delay == 0 {
		delay = 50 * time.Millisecond
	}
	
	select {
	case <-time.After(delay):
		// Success
		latency := time.Since(start)
		return &HealthStatus{
			IsHealthy:    true,
			LastCheck:    time.Now(),
			FailureCount: 0,
			Latency:      latency,
			Bandwidth:    100.0, // Simulate 100 MB/s
			Reliability:  0.9,
		}, nil
	case <-ctx.Done():
		// Timeout
		return &HealthStatus{
			IsHealthy:    false,
			LastCheck:    time.Now(),
			FailureCount: relay.Health.FailureCount + 1,
			Latency:      b.timeout,
			Bandwidth:    0,
			Reliability:  0.0,
		}, ctx.Err()
	}
}

// NewRelayHealthMonitor creates a new relay health monitor
func NewRelayHealthMonitor(pool *RelayPool, interval time.Duration) *RelayHealthMonitor {
	return &RelayHealthMonitor{
		pool:     pool,
		interval: interval,
	}
}

// Start starts the health monitoring
func (h *RelayHealthMonitor) Start(ctx context.Context) error {
	h.ctx, h.cancel = context.WithCancel(ctx)
	
	h.wg.Add(1)
	go h.monitorLoop()
	
	return nil
}

// Stop stops the health monitoring
func (h *RelayHealthMonitor) Stop() error {
	if h.cancel != nil {
		h.cancel()
	}
	h.wg.Wait()
	return nil
}

// monitorLoop runs the health monitoring loop
func (h *RelayHealthMonitor) monitorLoop() {
	defer h.wg.Done()
	
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()
	
	// Create health checker
	checker := NewPingHealthChecker(5 * time.Second)
	
	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			h.checkAllRelays(checker)
		}
	}
}

// checkAllRelays checks the health of all relays
func (h *RelayHealthMonitor) checkAllRelays(checker HealthChecker) {
	h.pool.mu.RLock()
	relays := make([]*RelayNode, 0, len(h.pool.relays))
	for _, relay := range h.pool.relays {
		relays = append(relays, relay)
	}
	h.pool.mu.RUnlock()
	
	// Check relays in parallel
	var wg sync.WaitGroup
	for _, relay := range relays {
		wg.Add(1)
		go func(relay *RelayNode) {
			defer wg.Done()
			h.checkRelay(checker, relay)
		}(relay)
	}
	wg.Wait()
}

// checkRelay checks the health of a single relay
func (h *RelayHealthMonitor) checkRelay(checker HealthChecker, relay *RelayNode) {
	ctx, cancel := context.WithTimeout(h.ctx, 10*time.Second)
	defer cancel()
	
	health, err := checker.CheckHealth(ctx, relay)
	if err != nil {
		// Health check failed
		health = &HealthStatus{
			IsHealthy:    false,
			LastCheck:    time.Now(),
			FailureCount: relay.Health.FailureCount + 1,
			Latency:      0,
			Bandwidth:    0,
			Reliability:  0.0,
		}
	}
	
	// Update relay health
	h.pool.UpdateRelayHealth(relay.ID, health)
	
	// If relay has failed too many times, remove it
	if health.FailureCount >= 3 {
		h.pool.RemoveRelay(relay.ID)
	}
}

// GetHealthReport returns a health report for all relays
func (h *RelayHealthMonitor) GetHealthReport() *HealthReport {
	h.pool.mu.RLock()
	defer h.pool.mu.RUnlock()
	
	report := &HealthReport{
		Timestamp:    time.Now(),
		TotalRelays:  len(h.pool.relays),
		HealthyCount: 0,
		UnhealthyCount: 0,
		Relays:       make([]*RelayHealthInfo, 0, len(h.pool.relays)),
	}
	
	for _, relay := range h.pool.relays {
		info := &RelayHealthInfo{
			PeerID:      relay.ID,
			IsHealthy:   relay.Health.IsHealthy,
			LastCheck:   relay.Health.LastCheck,
			Latency:     relay.Health.Latency,
			Bandwidth:   relay.Health.Bandwidth,
			Reliability: relay.Health.Reliability,
			FailureCount: relay.Health.FailureCount,
		}
		
		report.Relays = append(report.Relays, info)
		
		if relay.Health.IsHealthy {
			report.HealthyCount++
		} else {
			report.UnhealthyCount++
		}
	}
	
	return report
}

// HealthReport contains health information for all relays
type HealthReport struct {
	Timestamp      time.Time
	TotalRelays    int
	HealthyCount   int
	UnhealthyCount int
	Relays         []*RelayHealthInfo
}

// RelayHealthInfo contains health information for a single relay
type RelayHealthInfo struct {
	PeerID       peer.ID
	IsHealthy    bool
	LastCheck    time.Time
	Latency      time.Duration
	Bandwidth    float64
	Reliability  float64
	FailureCount int
}
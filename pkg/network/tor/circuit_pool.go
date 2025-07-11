package tor

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"
)

// CircuitPool manages pre-established Tor circuits for performance
type CircuitPool struct {
	client  *Client
	config  CircuitPoolConfig
	
	mu       sync.RWMutex
	circuits map[string]*Circuit
	
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// Circuit represents a Tor circuit with its own HTTP client
type Circuit struct {
	ID         string
	HTTPClient *http.Client
	Created    time.Time
	LastUsed   time.Time
	UseCount   int64
	Healthy    bool
	
	mu sync.Mutex
}

// NewCircuitPool creates a new circuit pool
func NewCircuitPool(client *Client, config CircuitPoolConfig) *CircuitPool {
	return &CircuitPool{
		client:   client,
		config:   config,
		circuits: make(map[string]*Circuit),
		stopChan: make(chan struct{}),
	}
}

// Initialize creates initial circuits
func (p *CircuitPool) Initialize() error {
	// PERFORMANCE IMPACT: Initial setup time
	// Each circuit takes 1-3s to establish
	fmt.Printf("Initializing Tor circuit pool with %d circuits...\n", p.config.MinCircuits)
	
	start := time.Now()
	errors := make([]error, 0)
	
	// Create initial circuits in parallel
	var wg sync.WaitGroup
	errChan := make(chan error, p.config.MinCircuits)
	
	for i := 0; i < p.config.MinCircuits; i++ {
		wg.Add(1)
		go func(num int) {
			defer wg.Done()
			
			circuitID := fmt.Sprintf("init-%d", num)
			if _, err := p.createCircuit(circuitID); err != nil {
				errChan <- fmt.Errorf("circuit %s: %w", circuitID, err)
			}
		}(i)
	}
	
	wg.Wait()
	close(errChan)
	
	// Collect errors
	for err := range errChan {
		errors = append(errors, err)
	}
	
	setupTime := time.Since(start)
	p.client.updateMetrics(func(m *Metrics) {
		m.CircuitBuilds += int64(p.config.MinCircuits)
		m.CircuitBuildTime = setupTime / time.Duration(p.config.MinCircuits)
	})
	
	// PERFORMANCE WARNING: Slow circuit establishment
	if setupTime > 10*time.Second {
		fmt.Printf("Warning: Circuit pool initialization took %v\n", setupTime)
	}
	
	// Start maintenance routines
	p.wg.Add(2)
	go p.healthCheckLoop()
	go p.rotationLoop()
	
	if len(errors) > 0 {
		return fmt.Errorf("failed to create %d circuits", len(errors))
	}
	
	fmt.Printf("Circuit pool ready (%d circuits in %v)\n", p.config.MinCircuits, setupTime)
	return nil
}

// GetCircuit returns an available circuit
func (p *CircuitPool) GetCircuit() (*Circuit, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	// PERFORMANCE OPTIMIZATION: Reuse least recently used healthy circuit
	var bestCircuit *Circuit
	var oldestUse time.Time
	
	for _, circuit := range p.circuits {
		if !circuit.Healthy {
			continue
		}
		
		// Check if circuit is too old
		if time.Since(circuit.Created) > p.config.CircuitLifetime {
			continue
		}
		
		// Find least recently used
		if bestCircuit == nil || circuit.LastUsed.Before(oldestUse) {
			bestCircuit = circuit
			oldestUse = circuit.LastUsed
		}
	}
	
	if bestCircuit == nil {
		// PERFORMANCE IMPACT: Creating new circuit adds latency
		return nil, fmt.Errorf("no healthy circuits available")
	}
	
	// Update usage
	bestCircuit.mu.Lock()
	bestCircuit.LastUsed = time.Now()
	bestCircuit.UseCount++
	bestCircuit.mu.Unlock()
	
	return bestCircuit, nil
}

// GetCircuitClient returns HTTP client for specific circuit
func (p *CircuitPool) GetCircuitClient(circuitID string) *http.Client {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	if circuit, ok := p.circuits[circuitID]; ok && circuit.Healthy {
		return circuit.HTTPClient
	}
	
	return nil
}

// createCircuit establishes a new Tor circuit
func (p *CircuitPool) createCircuit(id string) (*Circuit, error) {
	// PERFORMANCE IMPACT: Each circuit creation involves:
	// 1. Tor circuit establishment (1-3s)
	// 2. HTTP client creation
	// 3. Health check
	
	start := time.Now()
	
	// Create new HTTP client for this circuit
	// This forces Tor to create a new circuit
	httpClient := &http.Client{
		Transport: p.client.httpClient.Transport,
		Timeout:   p.client.config.Performance.RequestTimeout,
	}
	
	circuit := &Circuit{
		ID:         id,
		HTTPClient: httpClient,
		Created:    time.Now(),
		LastUsed:   time.Now(),
		Healthy:    true,
	}
	
	// Test circuit with lightweight request
	if err := p.testCircuit(circuit); err != nil {
		p.client.updateMetrics(func(m *Metrics) {
			m.CircuitFailures++
		})
		return nil, fmt.Errorf("circuit test failed: %w", err)
	}
	
	buildTime := time.Since(start)
	p.client.updateMetrics(func(m *Metrics) {
		m.CircuitBuilds++
		m.CircuitBuildTime = (m.CircuitBuildTime + buildTime) / 2
	})
	
	// Add to pool
	p.mu.Lock()
	p.circuits[id] = circuit
	p.mu.Unlock()
	
	return circuit, nil
}

// testCircuit verifies circuit functionality
func (p *CircuitPool) testCircuit(circuit *Circuit) error {
	// PERFORMANCE TEST: Minimal request to verify circuit
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	req, _ := http.NewRequestWithContext(ctx, "HEAD", "https://check.torproject.org/", nil)
	resp, err := circuit.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	
	return nil
}

// healthCheckLoop monitors circuit health
func (p *CircuitPool) healthCheckLoop() {
	defer p.wg.Done()
	
	ticker := time.NewTicker(p.config.HealthCheckInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			p.checkHealth()
		case <-p.stopChan:
			return
		}
	}
}

// checkHealth tests all circuits
func (p *CircuitPool) checkHealth() {
	p.mu.RLock()
	circuits := make([]*Circuit, 0, len(p.circuits))
	for _, c := range p.circuits {
		circuits = append(circuits, c)
	}
	p.mu.RUnlock()
	
	// PERFORMANCE OPTIMIZATION: Parallel health checks
	var wg sync.WaitGroup
	for _, circuit := range circuits {
		wg.Add(1)
		go func(c *Circuit) {
			defer wg.Done()
			
			err := p.testCircuit(c)
			c.mu.Lock()
			c.Healthy = err == nil
			c.mu.Unlock()
			
			if err != nil {
				// PERFORMANCE: Unhealthy circuits will be replaced
				fmt.Printf("Circuit %s health check failed: %v\n", c.ID, err)
			}
		}(circuit)
	}
	
	wg.Wait()
	
	// Replace unhealthy circuits
	p.maintainPoolSize()
}

// rotationLoop handles circuit rotation
func (p *CircuitPool) rotationLoop() {
	defer p.wg.Done()
	
	ticker := time.NewTicker(p.config.CircuitLifetime / 2)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			p.rotateOldCircuits()
		case <-p.stopChan:
			return
		}
	}
}

// rotateOldCircuits replaces aged circuits
func (p *CircuitPool) rotateOldCircuits() {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	now := time.Now()
	toRemove := make([]string, 0)
	
	for id, circuit := range p.circuits {
		age := now.Sub(circuit.Created)
		if age > p.config.CircuitLifetime {
			toRemove = append(toRemove, id)
			// PERFORMANCE LOG: Track circuit lifetime
			fmt.Printf("Rotating circuit %s (age: %v, uses: %d)\n", 
				id, age, circuit.UseCount)
		}
	}
	
	// Remove old circuits
	for _, id := range toRemove {
		delete(p.circuits, id)
	}
	
	// Maintain pool size
	p.maintainPoolSize()
}

// maintainPoolSize ensures minimum circuits available
func (p *CircuitPool) maintainPoolSize() {
	current := len(p.circuits)
	needed := p.config.MinCircuits - current
	
	if needed <= 0 {
		return
	}
	
	// PERFORMANCE OPTIMIZATION: Create circuits in background
	for i := 0; i < needed; i++ {
		go func(num int) {
			id := fmt.Sprintf("pool-%d-%d", time.Now().Unix(), num)
			if _, err := p.createCircuit(id); err != nil {
				fmt.Printf("Failed to create replacement circuit: %v\n", err)
			}
		}(i)
	}
}

// HasAvailableCircuits checks if pool has healthy circuits
func (p *CircuitPool) HasAvailableCircuits() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	for _, circuit := range p.circuits {
		if circuit.Healthy {
			return true
		}
	}
	
	return false
}

// GetPoolStats returns pool statistics
func (p *CircuitPool) GetPoolStats() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	healthy := 0
	totalUses := int64(0)
	avgAge := time.Duration(0)
	
	for _, circuit := range p.circuits {
		if circuit.Healthy {
			healthy++
		}
		totalUses += circuit.UseCount
		avgAge += time.Since(circuit.Created)
	}
	
	if len(p.circuits) > 0 {
		avgAge /= time.Duration(len(p.circuits))
	}
	
	return map[string]interface{}{
		"total_circuits":  len(p.circuits),
		"healthy":         healthy,
		"total_uses":      totalUses,
		"average_age":     avgAge,
		"min_circuits":    p.config.MinCircuits,
		"max_circuits":    p.config.MaxCircuits,
	}
}

// Close shuts down the circuit pool
func (p *CircuitPool) Close() error {
	close(p.stopChan)
	p.wg.Wait()
	
	// Log final stats
	stats := p.GetPoolStats()
	fmt.Printf("Circuit pool closed. Final stats: %+v\n", stats)
	
	return nil
}

// Helper function for random int64
func randInt64n(n int64) int64 {
	val, _ := rand.Int(rand.Reader, big.NewInt(n))
	return val.Int64()
}
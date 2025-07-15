package testing

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

// MockBackendAdapter provides compatibility between MockIPFSClient and existing storage backends
// This adapter ensures that tests can seamlessly switch between mock and real implementations
type MockBackendAdapter struct {
	mu sync.RWMutex

	// Core components
	mockClient    *MockIPFSClient
	networkSim    *NetworkSimulator
	conditionSim  *ConditionSimulator
	
	// Compatibility state
	backendType   string
	
	// Environment configuration
	testMode      string // "unit", "integration", "e2e"
	isolationLevel string // "strict", "moderate", "loose"
	
	// Adapter metrics
	adapterCalls  map[string]int64
	errors        map[string]int64
	
	// Event handling
	eventHandlers map[string]func(AdapterEvent)
}

// AdapterEvent represents events from the adapter
type AdapterEvent struct {
	Timestamp   time.Time
	Type        string
	Method      string
	Success     bool
	Duration    time.Duration
	Details     map[string]interface{}
	Error       string
}

// TestEnvironmentConfig configures the test environment
type TestEnvironmentConfig struct {
	TestMode          string
	IsolationLevel    string
	EnableNetworkSim  bool
	EnableConditionSim bool
	DefaultLatency    time.Duration
	DefaultPeers      []string
	StorageQuota      int64
	BandwidthLimit    int64
	
	// Compatibility settings
	BackendType       string
	
	// Advanced options
	EnableMetrics     bool
	EnableEventLog    bool
	MaxEventHistory   int
}

// NewMockBackendAdapter creates a new adapter
func NewMockBackendAdapter(config *TestEnvironmentConfig) *MockBackendAdapter {
	if config == nil {
		config = &TestEnvironmentConfig{
			TestMode:           "unit",
			IsolationLevel:     "strict",
			EnableNetworkSim:   true,
			EnableConditionSim: true,
			DefaultLatency:     time.Millisecond * 50,
			DefaultPeers:       []string{"peer1", "peer2", "peer3"},
			StorageQuota:       1000000000, // 1GB
			BandwidthLimit:     1000000,    // 1MB/s
			BackendType:        storage.BackendTypeIPFS,
			EnableMetrics:      true,
			EnableEventLog:     true,
			MaxEventHistory:    1000,
		}
	}

	// Create mock components
	mockClient := NewMockIPFSClient()
	networkSim := NewNetworkSimulator()
	conditionSim := NewConditionSimulator(mockClient, networkSim)

	// Configure mock client
	mockClient.SetLatency(config.DefaultLatency)
	mockClient.SetStorageQuota(config.StorageQuota)
	mockClient.SetBandwidthLimit(config.BandwidthLimit)

	// Configure network simulation
	if config.EnableNetworkSim {
		networkSim.SetNetworkLatency(config.DefaultLatency, config.DefaultLatency/4)
		networkSim.SetBandwidthLimit(config.BandwidthLimit)
		
		// Add default peers
		for _, peerID := range config.DefaultPeers {
			networkSim.AddPeer(peerID)
		}
	}

	adapter := &MockBackendAdapter{
		mockClient:     mockClient,
		networkSim:     networkSim,
		conditionSim:   conditionSim,
		backendType:    config.BackendType,
		testMode:       config.TestMode,
		isolationLevel: config.IsolationLevel,
		adapterCalls:   make(map[string]int64),
		errors:         make(map[string]int64),
		eventHandlers:  make(map[string]func(AdapterEvent)),
	}

	// Start simulators based on configuration
	if config.EnableNetworkSim || config.EnableConditionSim {
		adapter.Start()
	}

	return adapter
}

// Storage Backend Interface Implementation

// Put stores a block and returns its address
func (a *MockBackendAdapter) Put(ctx context.Context, block *blocks.Block) (*storage.BlockAddress, error) {
	start := time.Now()
	defer func() {
		a.recordAdapterEvent("Put", start, nil)
	}()

	return a.mockClient.Put(ctx, block)
}

// Get retrieves a block by address
func (a *MockBackendAdapter) Get(ctx context.Context, address *storage.BlockAddress) (*blocks.Block, error) {
	start := time.Now()
	defer func() {
		a.recordAdapterEvent("Get", start, nil)
	}()

	return a.mockClient.Get(ctx, address)
}

// Has checks if a block exists
func (a *MockBackendAdapter) Has(ctx context.Context, address *storage.BlockAddress) (bool, error) {
	start := time.Now()
	defer func() {
		a.recordAdapterEvent("Has", start, nil)
	}()

	return a.mockClient.Has(ctx, address)
}

// Delete removes a block
func (a *MockBackendAdapter) Delete(ctx context.Context, address *storage.BlockAddress) error {
	start := time.Now()
	defer func() {
		a.recordAdapterEvent("Delete", start, nil)
	}()

	return a.mockClient.Delete(ctx, address)
}

// PutMany stores multiple blocks
func (a *MockBackendAdapter) PutMany(ctx context.Context, blocks []*blocks.Block) ([]*storage.BlockAddress, error) {
	start := time.Now()
	defer func() {
		a.recordAdapterEvent("PutMany", start, map[string]interface{}{
			"block_count": len(blocks),
		})
	}()

	return a.mockClient.PutMany(ctx, blocks)
}

// GetMany retrieves multiple blocks
func (a *MockBackendAdapter) GetMany(ctx context.Context, addresses []*storage.BlockAddress) ([]*blocks.Block, error) {
	start := time.Now()
	defer func() {
		a.recordAdapterEvent("GetMany", start, map[string]interface{}{
			"address_count": len(addresses),
		})
	}()

	return a.mockClient.GetMany(ctx, addresses)
}

// Pin marks a block as pinned
func (a *MockBackendAdapter) Pin(ctx context.Context, address *storage.BlockAddress) error {
	start := time.Now()
	defer func() {
		a.recordAdapterEvent("Pin", start, nil)
	}()

	return a.mockClient.Pin(ctx, address)
}

// Unpin removes pinning from a block
func (a *MockBackendAdapter) Unpin(ctx context.Context, address *storage.BlockAddress) error {
	start := time.Now()
	defer func() {
		a.recordAdapterEvent("Unpin", start, nil)
	}()

	return a.mockClient.Unpin(ctx, address)
}

// PeerAwareBackend Interface Implementation

// GetWithPeerHint retrieves a block with preferred peers
func (a *MockBackendAdapter) GetWithPeerHint(ctx context.Context, address *storage.BlockAddress, peers []string) (*blocks.Block, error) {
	start := time.Now()
	defer func() {
		a.recordAdapterEvent("GetWithPeerHint", start, map[string]interface{}{
			"peer_hints": len(peers),
		})
	}()

	// Use network simulation if enabled
	if a.networkSim != nil {
		return a.networkSim.SimulateBlockRetrieval(ctx, address.ID, "requester", peers)
	}

	return a.mockClient.GetWithPeerHint(ctx, address, peers)
}

// BroadcastToNetwork broadcasts a block to the network
func (a *MockBackendAdapter) BroadcastToNetwork(ctx context.Context, address *storage.BlockAddress, block *blocks.Block) error {
	start := time.Now()
	defer func() {
		a.recordAdapterEvent("BroadcastToNetwork", start, nil)
	}()

	// Use network simulation if enabled
	if a.networkSim != nil {
		return a.networkSim.SimulateBlockBroadcast(ctx, address.ID, block, "broadcaster")
	}

	return a.mockClient.BroadcastToNetwork(ctx, address, block)
}

// GetConnectedPeers returns list of connected peers
func (a *MockBackendAdapter) GetConnectedPeers() []string {
	start := time.Now()
	defer func() {
		a.recordAdapterEvent("GetConnectedPeers", start, nil)
	}()

	return a.mockClient.GetConnectedPeers()
}

// Backend Metadata Implementation

// GetBackendInfo returns backend information
func (a *MockBackendAdapter) GetBackendInfo() *storage.BackendInfo {
	info := a.mockClient.GetBackendInfo()
	
	// Add adapter-specific information
	info.Config["adapter"] = true
	info.Config["test_mode"] = true
	info.Config["isolation_level"] = a.isolationLevel
	
	return info
}

// HealthCheck returns current health status
func (a *MockBackendAdapter) HealthCheck(ctx context.Context) *storage.HealthStatus {
	start := time.Now()
	defer func() {
		a.recordAdapterEvent("HealthCheck", start, nil)
	}()

	return a.mockClient.HealthCheck(ctx)
}

// Connection Management

// Connect simulates connecting to the backend
func (a *MockBackendAdapter) Connect(ctx context.Context) error {
	start := time.Now()
	defer func() {
		a.recordAdapterEvent("Connect", start, nil)
	}()

	return a.mockClient.Connect(ctx)
}

// Disconnect simulates disconnecting from the backend
func (a *MockBackendAdapter) Disconnect(ctx context.Context) error {
	start := time.Now()
	defer func() {
		a.recordAdapterEvent("Disconnect", start, nil)
	}()

	return a.mockClient.Disconnect(ctx)
}

// IsConnected returns connection status
func (a *MockBackendAdapter) IsConnected() bool {
	return a.mockClient.IsConnected()
}


// Test Environment Management

// SetTestMode configures the test mode
func (a *MockBackendAdapter) SetTestMode(mode string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.testMode = mode
}

// SetIsolationLevel configures the isolation level
func (a *MockBackendAdapter) SetIsolationLevel(level string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.isolationLevel = level
}


// GetMockClient returns the underlying mock client for direct access
func (a *MockBackendAdapter) GetMockClient() *MockIPFSClient {
	return a.mockClient
}

// GetNetworkSimulator returns the network simulator for direct access
func (a *MockBackendAdapter) GetNetworkSimulator() *NetworkSimulator {
	return a.networkSim
}

// GetConditionSimulator returns the condition simulator for direct access
func (a *MockBackendAdapter) GetConditionSimulator() *ConditionSimulator {
	return a.conditionSim
}

// Test Control Methods

// ApplyTestCondition applies a test condition
func (a *MockBackendAdapter) ApplyTestCondition(conditionID string) error {
	if a.conditionSim == nil {
		return fmt.Errorf("condition simulator not enabled")
	}
	return a.conditionSim.ApplyCondition(conditionID)
}

// RemoveTestCondition removes a test condition
func (a *MockBackendAdapter) RemoveTestCondition(conditionID string) error {
	if a.conditionSim == nil {
		return fmt.Errorf("condition simulator not enabled")
	}
	return a.conditionSim.RemoveCondition(conditionID)
}

// ApplyTestScenario applies a test scenario
func (a *MockBackendAdapter) ApplyTestScenario(scenarioName string) error {
	if a.conditionSim == nil {
		return fmt.Errorf("condition simulator not enabled")
	}
	return a.conditionSim.ApplyScenario(scenarioName)
}

// SimulateNetworkPartition creates a network partition
func (a *MockBackendAdapter) SimulateNetworkPartition(peerIDs []string) {
	if a.networkSim != nil {
		a.networkSim.CreateNetworkPartition(peerIDs)
	}
}

// HealNetworkPartition heals network partitions
func (a *MockBackendAdapter) HealNetworkPartition() {
	if a.networkSim != nil {
		a.networkSim.HealNetworkPartition()
	}
}

// Environment Variable Integration

// ApplyEnvironmentConfig applies configuration from environment variables
func (a *MockBackendAdapter) ApplyEnvironmentConfig() {
	// This would typically read from environment variables like:
	// NOISEFS_TEST_MODE, NOISEFS_MOCK_LATENCY, etc.
	// For now, we'll use default configuration
	
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// Example environment configuration
	a.testMode = "unit" // Could be read from NOISEFS_TEST_MODE
	a.isolationLevel = "strict" // Could be read from NOISEFS_ISOLATION_LEVEL
}

// Lifecycle Management

// Start starts all simulators
func (a *MockBackendAdapter) Start() {
	if a.conditionSim != nil {
		a.conditionSim.Start()
	}
}

// Stop stops all simulators
func (a *MockBackendAdapter) Stop() {
	if a.conditionSim != nil {
		a.conditionSim.Stop()
	}
}

// Reset resets all components to initial state
func (a *MockBackendAdapter) Reset() {
	a.mockClient.Reset()
	if a.networkSim != nil {
		a.networkSim.Reset()
	}
	
	a.mu.Lock()
	defer a.mu.Unlock()
	a.adapterCalls = make(map[string]int64)
	a.errors = make(map[string]int64)
}

// Monitoring and Metrics

// GetAdapterStats returns adapter statistics
func (a *MockBackendAdapter) GetAdapterStats() map[string]interface{} {
	a.mu.RLock()
	defer a.mu.RUnlock()

	stats := map[string]interface{}{
		"test_mode":       a.testMode,
		"isolation_level": a.isolationLevel,
		"backend_type":    a.backendType,
		"adapter_calls":   a.copyIntMap(a.adapterCalls),
		"errors":          a.copyIntMap(a.errors),
	}

	// Add component stats
	if a.mockClient != nil {
		stats["mock_client"] = a.mockClient.GetStorageStats()
	}
	if a.networkSim != nil {
		stats["network_sim"] = a.networkSim.GetNetworkStats()
	}
	if a.conditionSim != nil {
		stats["condition_sim"] = a.conditionSim.GetConditionStats()
	}

	return stats
}

// RegisterEventHandler registers an event handler
func (a *MockBackendAdapter) RegisterEventHandler(eventType string, handler func(AdapterEvent)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.eventHandlers[eventType] = handler
}

// Private helper methods

func (a *MockBackendAdapter) recordAdapterEvent(method string, start time.Time, details map[string]interface{}) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.adapterCalls[method]++
	
	event := AdapterEvent{
		Timestamp: start,
		Type:      "adapter_call",
		Method:    method,
		Success:   true, // Would be set based on actual result
		Duration:  time.Since(start),
		Details:   details,
	}

	// Call registered handlers
	if handler, exists := a.eventHandlers["adapter_call"]; exists {
		go handler(event)
	}
}

func (a *MockBackendAdapter) copyIntMap(source map[string]int64) map[string]int64 {
	copy := make(map[string]int64)
	for k, v := range source {
		copy[k] = v
	}
	return copy
}

// Factory Functions for Easy Testing

// NewUnitTestAdapter creates an adapter configured for unit testing
func NewUnitTestAdapter() *MockBackendAdapter {
	config := &TestEnvironmentConfig{
		TestMode:           "unit",
		IsolationLevel:     "strict",
		EnableNetworkSim:   false,
		EnableConditionSim: false,
		DefaultLatency:     time.Microsecond * 100, // Very fast for unit tests
		StorageQuota:       100000000, // 100MB
		BandwidthLimit:     10000000,  // 10MB/s
		BackendType:        storage.BackendTypeIPFS,
	}
	return NewMockBackendAdapter(config)
}

// NewIntegrationTestAdapter creates an adapter configured for integration testing
func NewIntegrationTestAdapter() *MockBackendAdapter {
	config := &TestEnvironmentConfig{
		TestMode:           "integration",
		IsolationLevel:     "moderate",
		EnableNetworkSim:   true,
		EnableConditionSim: true,
		DefaultLatency:     time.Millisecond * 10,
		DefaultPeers:       []string{"peer1", "peer2", "peer3", "peer4", "peer5"},
		StorageQuota:       1000000000, // 1GB
		BandwidthLimit:     1000000,    // 1MB/s
		BackendType:        storage.BackendTypeIPFS,
	}
	return NewMockBackendAdapter(config)
}

// NewE2ETestAdapter creates an adapter configured for end-to-end testing
func NewE2ETestAdapter() *MockBackendAdapter {
	config := &TestEnvironmentConfig{
		TestMode:           "e2e",
		IsolationLevel:     "loose",
		EnableNetworkSim:   true,
		EnableConditionSim: true,
		DefaultLatency:     time.Millisecond * 50, // Realistic latency
		DefaultPeers:       []string{"peer1", "peer2", "peer3", "peer4", "peer5", "peer6", "peer7", "peer8"},
		StorageQuota:       5000000000, // 5GB
		BandwidthLimit:     1000000,    // 1MB/s
		BackendType:        storage.BackendTypeIPFS,
		EnableMetrics:      true,
		EnableEventLog:     true,
	}
	return NewMockBackendAdapter(config)
}
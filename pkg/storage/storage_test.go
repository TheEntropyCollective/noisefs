package storage

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
)

// MockBackend implements the Backend interface for testing
type MockBackend struct {
	name         string
	connected    bool
	healthy      bool
	blocks       map[string]*blocks.Block
	capabilities []string
	latency      time.Duration
	errorRate    float64
	shouldFail   bool
}

func NewMockBackend(name string) *MockBackend {
	return &MockBackend{
		name:         name,
		connected:    false,
		healthy:      true,
		blocks:       make(map[string]*blocks.Block),
		capabilities: []string{CapabilityContentAddress, CapabilityPinning},
		latency:      10 * time.Millisecond,
		errorRate:    0.0,
		shouldFail:   false,
	}
}

func (m *MockBackend) Connect(ctx context.Context) error {
	if m.shouldFail {
		return NewConnectionError("mock", fmt.Errorf("mock connection failure"))
	}
	m.connected = true
	return nil
}

func (m *MockBackend) Disconnect(ctx context.Context) error {
	m.connected = false
	return nil
}

func (m *MockBackend) IsConnected() bool {
	return m.connected
}

func (m *MockBackend) Put(ctx context.Context, block *blocks.Block) (*BlockAddress, error) {
	if !m.connected {
		return nil, NewConnectionError("mock", fmt.Errorf("not connected"))
	}

	if m.shouldFail {
		return nil, NewStorageError("MOCK_ERROR", "mock put failure", "mock", nil)
	}

	// Simulate latency
	time.Sleep(m.latency)

	// Generate a mock address
	address := &BlockAddress{
		ID:          fmt.Sprintf("mock-%d", len(m.blocks)),
		BackendType: "mock",
		Size:        int64(len(block.Data)),
		CreatedAt:   time.Now(),
	}

	m.blocks[address.ID] = block

	return address, nil
}

func (m *MockBackend) Get(ctx context.Context, address *BlockAddress) (*blocks.Block, error) {
	if !m.connected {
		return nil, NewConnectionError("mock", fmt.Errorf("not connected"))
	}

	if m.shouldFail {
		return nil, NewStorageError("MOCK_ERROR", "mock get failure", "mock", nil)
	}

	// Simulate latency
	time.Sleep(m.latency)

	block, exists := m.blocks[address.ID]
	if !exists {
		return nil, NewNotFoundError("mock", address)
	}

	return block, nil
}

func (m *MockBackend) Has(ctx context.Context, address *BlockAddress) (bool, error) {
	if !m.connected {
		return false, NewConnectionError("mock", fmt.Errorf("not connected"))
	}

	if m.shouldFail {
		return false, NewStorageError("MOCK_ERROR", "mock has failure", "mock", nil)
	}

	_, exists := m.blocks[address.ID]
	return exists, nil
}

func (m *MockBackend) Delete(ctx context.Context, address *BlockAddress) error {
	if !m.connected {
		return NewConnectionError("mock", fmt.Errorf("not connected"))
	}

	if m.shouldFail {
		return NewStorageError("MOCK_ERROR", "mock delete failure", "mock", nil)
	}

	delete(m.blocks, address.ID)
	return nil
}

func (m *MockBackend) PutMany(ctx context.Context, blocks []*blocks.Block) ([]*BlockAddress, error) {
	addresses := make([]*BlockAddress, len(blocks))
	for i, block := range blocks {
		addr, err := m.Put(ctx, block)
		if err != nil {
			return nil, err
		}
		addresses[i] = addr
	}
	return addresses, nil
}

func (m *MockBackend) GetMany(ctx context.Context, addresses []*BlockAddress) ([]*blocks.Block, error) {
	blocks := make([]*blocks.Block, len(addresses))
	for i, addr := range addresses {
		block, err := m.Get(ctx, addr)
		if err != nil {
			return nil, err
		}
		blocks[i] = block
	}
	return blocks, nil
}

func (m *MockBackend) Pin(ctx context.Context, address *BlockAddress) error {
	if !m.connected {
		return NewConnectionError("mock", fmt.Errorf("not connected"))
	}
	// Mock implementation - just check if block exists
	_, exists := m.blocks[address.ID]
	if !exists {
		return NewNotFoundError("mock", address)
	}
	return nil
}

func (m *MockBackend) Unpin(ctx context.Context, address *BlockAddress) error {
	if !m.connected {
		return NewConnectionError("mock", fmt.Errorf("not connected"))
	}
	// Mock implementation - just check if block exists
	_, exists := m.blocks[address.ID]
	if !exists {
		return NewNotFoundError("mock", address)
	}
	return nil
}

func (m *MockBackend) GetBackendInfo() *BackendInfo {
	return &BackendInfo{
		Name:         m.name,
		Type:         "mock",
		Version:      "1.0.0",
		Capabilities: m.capabilities,
		Config: map[string]interface{}{
			"name": m.name,
		},
	}
}

func (m *MockBackend) HealthCheck(ctx context.Context) *HealthStatus {
	status := "healthy"
	if !m.healthy {
		status = "unhealthy"
	}

	return &HealthStatus{
		Healthy:   m.healthy,
		Status:    status,
		Latency:   m.latency,
		ErrorRate: m.errorRate,
		LastCheck: time.Now(),
	}
}

// Test functions

func TestBackendInterface(t *testing.T) {
	ctx := context.Background()
	backend := NewMockBackend("test")

	// Test connection
	err := backend.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	if !backend.IsConnected() {
		t.Fatal("Backend should be connected")
	}

	// Test putting a block
	block, err := blocks.NewBlock([]byte("test data"))
	if err != nil {
		t.Fatalf("Failed to create block: %v", err)
	}

	address, err := backend.Put(ctx, block)
	if err != nil {
		t.Fatalf("Failed to put block: %v", err)
	}

	if address.ID == "" {
		t.Fatal("Address ID should not be empty")
	}

	// Test checking block existence
	exists, err := backend.Has(ctx, address)
	if err != nil {
		t.Fatalf("Failed to check block existence: %v", err)
	}

	if !exists {
		t.Fatal("Block should exist")
	}

	// Test getting the block
	retrievedBlock, err := backend.Get(ctx, address)
	if err != nil {
		t.Fatalf("Failed to get block: %v", err)
	}

	if string(retrievedBlock.Data) != string(block.Data) {
		t.Fatal("Retrieved block data doesn't match original")
	}

	// Test pinning
	err = backend.Pin(ctx, address)
	if err != nil {
		t.Fatalf("Failed to pin block: %v", err)
	}

	// Test unpinning
	err = backend.Unpin(ctx, address)
	if err != nil {
		t.Fatalf("Failed to unpin block: %v", err)
	}

	// Test deleting the block
	err = backend.Delete(ctx, address)
	if err != nil {
		t.Fatalf("Failed to delete block: %v", err)
	}

	// Verify block is deleted
	exists, err = backend.Has(ctx, address)
	if err != nil {
		t.Fatalf("Failed to check block existence after deletion: %v", err)
	}

	if exists {
		t.Fatal("Block should not exist after deletion")
	}

	// Test disconnection
	err = backend.Disconnect(ctx)
	if err != nil {
		t.Fatalf("Failed to disconnect: %v", err)
	}

	if backend.IsConnected() {
		t.Fatal("Backend should not be connected")
	}
}

func TestBackendBatchOperations(t *testing.T) {
	ctx := context.Background()
	backend := NewMockBackend("test")

	err := backend.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// Create test blocks
	testData := []string{"block1", "block2", "block3"}
	testBlocks := make([]*blocks.Block, len(testData))

	for i, data := range testData {
		block, err := blocks.NewBlock([]byte(data))
		if err != nil {
			t.Fatalf("Failed to create block %d: %v", i, err)
		}
		testBlocks[i] = block
	}

	// Test batch put
	addresses, err := backend.PutMany(ctx, testBlocks)
	if err != nil {
		t.Fatalf("Failed to put blocks: %v", err)
	}

	if len(addresses) != len(testBlocks) {
		t.Fatalf("Expected %d addresses, got %d", len(testBlocks), len(addresses))
	}

	// Test batch get
	retrievedBlocks, err := backend.GetMany(ctx, addresses)
	if err != nil {
		t.Fatalf("Failed to get blocks: %v", err)
	}

	if len(retrievedBlocks) != len(testBlocks) {
		t.Fatalf("Expected %d blocks, got %d", len(testBlocks), len(retrievedBlocks))
	}

	// Verify data integrity
	for i, block := range retrievedBlocks {
		if string(block.Data) != testData[i] {
			t.Fatalf("Block %d data mismatch: expected %s, got %s",
				i, testData[i], string(block.Data))
		}
	}
}

func TestStorageManager(t *testing.T) {
	// Create manager with mock backends
	manager := createMockManager(t)

	// Start manager
	ctx := context.Background()
	err := manager.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start manager: %v", err)
	}
	defer manager.Stop(ctx)

	// Test basic operations through manager
	block, err := blocks.NewBlock([]byte("test data"))
	if err != nil {
		t.Fatalf("Failed to create block: %v", err)
	}

	address, err := manager.Put(ctx, block)
	if err != nil {
		t.Fatalf("Failed to put block through manager: %v", err)
	}

	retrievedBlock, err := manager.Get(ctx, address)
	if err != nil {
		t.Fatalf("Failed to get block through manager: %v", err)
	}

	if string(retrievedBlock.Data) != string(block.Data) {
		t.Fatal("Retrieved block data doesn't match original")
	}

	// Test manager status
	status := manager.GetManagerStatus()
	if status.TotalBackends != 2 {
		t.Fatalf("Expected 2 total backends, got %d", status.TotalBackends)
	}

	if status.ActiveBackends != 2 {
		t.Fatalf("Expected 2 active backends, got %d", status.ActiveBackends)
	}
}

func TestErrorHandling(t *testing.T) {
	// Test error classification
	classifier := NewErrorClassifier("test")

	// Test different error types
	testErrors := map[error]string{
		fmt.Errorf("connection refused"): ErrCodeConnectionFailed,
		fmt.Errorf("not found"):          ErrCodeNotFound,
		fmt.Errorf("timeout"):            ErrCodeTimeout,
		fmt.Errorf("quota exceeded"):     ErrCodeQuotaExceeded,
		fmt.Errorf("unauthorized"):       ErrCodeUnauthorized,
		fmt.Errorf("checksum mismatch"):  ErrCodeIntegrityFailure,
	}

	for err, expectedCode := range testErrors {
		storageErr := classifier.ClassifyError(err, "test_operation", nil)
		if storageErr.Code != expectedCode {
			t.Errorf("Expected error code %s for error '%v', got %s",
				expectedCode, err, storageErr.Code)
		}
	}

	// Test error aggregation
	aggregator := NewErrorAggregator("test_operation")

	for err := range testErrors {
		aggregator.Add(err)
	}

	if !aggregator.HasErrors() {
		t.Fatal("Aggregator should have errors")
	}

	aggregateErr := aggregator.CreateAggregateError()
	if aggregateErr == nil {
		t.Fatal("Aggregate error should not be nil")
	}

	storageErr, ok := aggregateErr.(*StorageError)
	if !ok {
		t.Fatal("Aggregate error should be a StorageError")
	}

	if storageErr.Code != "AGGREGATE_ERROR" {
		t.Fatalf("Expected aggregate error code, got %s", storageErr.Code)
	}
}

func TestConfigValidation(t *testing.T) {
	// Test valid configuration
	validConfig := DefaultConfig()
	err := validConfig.Validate()
	if err != nil {
		t.Fatalf("Default config should be valid: %v", err)
	}

	// Test invalid configurations
	invalidConfigs := []*Config{
		// Empty default backend
		{
			DefaultBackend: "",
			Backends: map[string]*BackendConfig{
				"test": {Type: "mock", Enabled: true},
			},
		},
		// No backends
		{
			DefaultBackend: "test",
			Backends:       map[string]*BackendConfig{},
		},
		// Default backend not found
		{
			DefaultBackend: "nonexistent",
			Backends: map[string]*BackendConfig{
				"test": {Type: "mock", Enabled: true},
			},
		},
	}

	for i, config := range invalidConfigs {
		err := config.Validate()
		if err == nil {
			t.Fatalf("Invalid config %d should have failed validation", i)
		}
	}
}

func TestHealthMonitoring(t *testing.T) {
	// Create a manager with health monitoring enabled
	config := DefaultConfig()
	config.HealthCheck.Enabled = true
	config.HealthCheck.Interval = 100 * time.Millisecond
	config.HealthCheck.Timeout = 50 * time.Millisecond

	manager, err := NewManager(config)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Add mock backends
	backend1 := NewMockBackend("test1")
	backend2 := NewMockBackend("test2")
	backend2.healthy = false // Make second backend unhealthy

	// Connect the backends so they're available in the registry
	ctx := context.Background()
	backend1.Connect(ctx)
	backend2.Connect(ctx)

	// Add backends to the manager's registry
	registry := manager.GetRegistry()
	registry.AddBackend("test1", backend1)
	registry.AddBackend("test2", backend2)

	// Start health monitoring
	monitor := manager.GetHealthMonitor()
	err = monitor.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start health monitor: %v", err)
	}
	defer monitor.Stop()

	// Connect backends for monitoring
	backend1.Connect(ctx)
	backend2.Connect(ctx)

	// Wait for some health checks
	time.Sleep(300 * time.Millisecond)

	// Check health summary
	summary := monitor.GetHealthSummary()
	if summary.TotalBackends != 2 {
		t.Fatalf("Expected 2 total backends, got %d", summary.TotalBackends)
	}

	if summary.HealthyBackends != 1 {
		t.Fatalf("Expected 1 healthy backend, got %d", summary.HealthyBackends)
	}

	if summary.OverallHealth != "degraded" {
		t.Fatalf("Expected degraded health, got %s", summary.OverallHealth)
	}

	// Verify degraded health is properly detected
	// The HealthSummary already confirms we have 1 healthy + 1 unhealthy backend
	// which results in "degraded" status - this validates the core monitoring functionality
}

// MockBackendFactory for testing - create a wrapper around the real factory
func createMockManager(t *testing.T) *Manager {
	// Create test configuration
	config := &Config{
		DefaultBackend: "mock1",
		Backends: map[string]*BackendConfig{
			"mock1": {
				Type:     "mock",
				Enabled:  true,
				Priority: 100,
				Connection: &ConnectionConfig{
					Endpoint: "mock://mock1",
				},
				Retry: &RetryConfig{
					MaxAttempts: 3,
					BaseDelay:   100 * time.Millisecond,
					MaxDelay:    1 * time.Second,
					Multiplier:  2.0,
				},
				Timeouts: &TimeoutConfig{
					Connect:   5 * time.Second,
					Operation: 30 * time.Second,
				},
			},
			"mock2": {
				Type:     "mock",
				Enabled:  true,
				Priority: 90,
				Connection: &ConnectionConfig{
					Endpoint: "mock://mock2",
				},
				Retry: &RetryConfig{
					MaxAttempts: 3,
					BaseDelay:   100 * time.Millisecond,
					MaxDelay:    1 * time.Second,
					Multiplier:  2.0,
				},
				Timeouts: &TimeoutConfig{
					Connect:   5 * time.Second,
					Operation: 30 * time.Second,
				},
			},
		},
		Distribution: &DistributionConfig{
			Strategy: "single",
			Selection: &SelectionConfig{
				RequiredCapabilities: []string{CapabilityContentAddress},
			},
			LoadBalancing: &LoadBalancingConfig{
				Algorithm:      "performance",
				RequireHealthy: true,
			},
		},
		HealthCheck: &HealthCheckConfig{
			Enabled:  false, // Disable for testing
			Interval: 30 * time.Second,
			Timeout:  10 * time.Second,
		},
		Performance: &PerformanceConfig{
			MaxConcurrentOperations: 10,
			MaxConcurrentPerBackend: 5,
		},
	}

	// Register mock backend constructor
	RegisterBackend("mock", func(config *BackendConfig) (Backend, error) {
		// Extract name from endpoint
		name := "mock1"
		if config.Connection.Endpoint == "mock://mock2" {
			name = "mock2"
		}
		return NewMockBackend(name), nil
	})

	manager, err := NewManager(config)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	return manager
}

// Benchmark tests

func BenchmarkBackendPut(b *testing.B) {
	ctx := context.Background()
	backend := NewMockBackend("bench")
	backend.Connect(ctx)

	block, _ := blocks.NewBlock([]byte("benchmark data"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := backend.Put(ctx, block)
		if err != nil {
			b.Fatalf("Put failed: %v", err)
		}
	}
}

func BenchmarkBackendGet(b *testing.B) {
	ctx := context.Background()
	backend := NewMockBackend("bench")
	backend.Connect(ctx)

	// Pre-populate with blocks
	block, _ := blocks.NewBlock([]byte("benchmark data"))
	address, _ := backend.Put(ctx, block)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := backend.Get(ctx, address)
		if err != nil {
			b.Fatalf("Get failed: %v", err)
		}
	}
}

func BenchmarkManagerOperations(b *testing.B) {
	// Create simple mock manager for benchmarking
	config := DefaultConfig()
	config.HealthCheck.Enabled = false

	// Use mock backend
	RegisterBackend("ipfs", func(config *BackendConfig) (Backend, error) {
		return NewMockBackend("ipfs"), nil
	})

	manager, _ := NewManager(config)

	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop(ctx)

	block, _ := blocks.NewBlock([]byte("benchmark data"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		address, err := manager.Put(ctx, block)
		if err != nil {
			b.Fatalf("Put failed: %v", err)
		}

		_, err = manager.Get(ctx, address)
		if err != nil {
			b.Fatalf("Get failed: %v", err)
		}
	}
}

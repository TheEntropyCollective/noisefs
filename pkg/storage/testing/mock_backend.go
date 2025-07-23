package testing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

// MockBackend provides a mock implementation of storage.Backend for testing
type MockBackend struct {
	mu          sync.RWMutex
	blocks      map[string]*blocks.Block
	backendType string
	isConnected bool

	// Test control
	storeError    error
	retrieveError error
	hasError      error
	latency       time.Duration
	storeDelay    time.Duration
	retrieveDelay time.Duration

	// Metrics
	storeCount    int64
	retrieveCount int64
	hasCount      int64
}

// NewMockBackend creates a new mock backend
func NewMockBackend(backendType string) *MockBackend {
	return &MockBackend{
		blocks:      make(map[string]*blocks.Block),
		backendType: backendType,
		isConnected: true,
	}
}

// Put stores a block in the mock backend
func (m *MockBackend) Put(ctx context.Context, block *blocks.Block) (*storage.BlockAddress, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.storeCount++

	if m.storeDelay > 0 {
		time.Sleep(m.storeDelay)
	}
	if m.latency > 0 {
		time.Sleep(m.latency)
	}

	if m.storeError != nil {
		return nil, m.storeError
	}

	// Generate a deterministic CID based on block data
	hash := sha256.Sum256(block.Data)
	cid := hex.EncodeToString(hash[:16]) // Use first 16 bytes as CID

	m.blocks[cid] = block
	address := &storage.BlockAddress{
		ID:          cid,
		BackendType: m.backendType,
		Size:        int64(len(block.Data)),
		CreatedAt:   time.Now(),
	}
	return address, nil
}

// Get retrieves a block from the mock backend
func (m *MockBackend) Get(ctx context.Context, address *storage.BlockAddress) (*blocks.Block, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.retrieveCount++

	if m.retrieveDelay > 0 {
		time.Sleep(m.retrieveDelay)
	}
	if m.latency > 0 {
		time.Sleep(m.latency)
	}

	if m.retrieveError != nil {
		return nil, m.retrieveError
	}

	block, exists := m.blocks[address.ID]
	if !exists {
		return nil, fmt.Errorf("block not found: %s", address.ID)
	}

	return block, nil
}

// Has checks if a block exists in the mock backend
func (m *MockBackend) Has(ctx context.Context, address *storage.BlockAddress) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.hasCount++

	if m.latency > 0 {
		time.Sleep(m.latency)
	}

	if m.hasError != nil {
		return false, m.hasError
	}

	_, exists := m.blocks[address.ID]
	return exists, nil
}

// Delete removes a block from the mock backend
func (m *MockBackend) Delete(ctx context.Context, address *storage.BlockAddress) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.blocks, address.ID)
	return nil
}

// GetStats returns backend statistics
func (m *MockBackend) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"backend_type":   m.backendType,
		"blocks_stored":  len(m.blocks),
		"store_count":    m.storeCount,
		"retrieve_count": m.retrieveCount,
		"has_count":      m.hasCount,
		"connected":      m.isConnected,
	}
}

// GetBackendInfo returns backend information
func (m *MockBackend) GetBackendInfo() *storage.BackendInfo {
	return &storage.BackendInfo{
		Name:    "mock-" + m.backendType,
		Type:    m.backendType,
		Version: "test-1.0",
		Capabilities: []string{
			storage.CapabilityBatch,
			storage.CapabilityContentAddress,
		},
		Config: map[string]interface{}{
			"mock":    true,
			"testing": true,
		},
	}
}

// HealthCheck returns the health status of the mock backend
func (m *MockBackend) HealthCheck(ctx context.Context) *storage.HealthStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := "healthy"
	if !m.isConnected {
		status = "offline"
	}

	return &storage.HealthStatus{
		Healthy:   m.isConnected,
		Status:    status,
		LastCheck: time.Now(),
	}
}

// PutMany stores multiple blocks (batch operation)
func (m *MockBackend) PutMany(ctx context.Context, blocks []*blocks.Block) ([]*storage.BlockAddress, error) {
	addresses := make([]*storage.BlockAddress, len(blocks))
	for i, block := range blocks {
		address, err := m.Put(ctx, block)
		if err != nil {
			return nil, err
		}
		addresses[i] = address
	}
	return addresses, nil
}

// GetMany retrieves multiple blocks (batch operation)
func (m *MockBackend) GetMany(ctx context.Context, addresses []*storage.BlockAddress) ([]*blocks.Block, error) {
	blocks := make([]*blocks.Block, len(addresses))
	for i, address := range addresses {
		block, err := m.Get(ctx, address)
		if err != nil {
			return nil, err
		}
		blocks[i] = block
	}
	return blocks, nil
}

// Pin marks a block as pinned (no-op for mock)
func (m *MockBackend) Pin(ctx context.Context, address *storage.BlockAddress) error {
	return nil
}

// Unpin removes pinning from a block (no-op for mock)
func (m *MockBackend) Unpin(ctx context.Context, address *storage.BlockAddress) error {
	return nil
}

// IsConnected returns whether the backend is connected
func (m *MockBackend) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isConnected
}

// Connect simulates connecting to the backend
func (m *MockBackend) Connect(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.isConnected = true
	return nil
}

// Disconnect simulates disconnecting from the backend
func (m *MockBackend) Disconnect(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.isConnected = false
	return nil
}

// Test control methods

// SetStoreError sets an error to return on store operations
func (m *MockBackend) SetStoreError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.storeError = err
}

// SetRetrieveError sets an error to return on retrieve operations
func (m *MockBackend) SetRetrieveError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.retrieveError = err
}

// SetHasError sets an error to return on has operations
func (m *MockBackend) SetHasError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hasError = err
}

// SetLatency sets artificial latency for all operations
func (m *MockBackend) SetLatency(latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latency = latency
}

// SetStoreDelay sets specific delay for store operations
func (m *MockBackend) SetStoreDelay(delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.storeDelay = delay
}

// SetRetrieveDelay sets specific delay for retrieve operations
func (m *MockBackend) SetRetrieveDelay(delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.retrieveDelay = delay
}

// SetConnectionState sets the connection state
func (m *MockBackend) SetConnectionState(connected bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.isConnected = connected
}

// Store convenience method for compatibility
func (m *MockBackend) Store(ctx context.Context, block *blocks.Block) (string, error) {
	address, err := m.Put(ctx, block)
	if err != nil {
		return "", err
	}
	return address.ID, nil
}

// AddBlock directly adds a block to the mock backend (for test setup)
func (m *MockBackend) AddBlock(cid string, block *blocks.Block) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blocks[cid] = block
}

// GetBlockCount returns the number of stored blocks
func (m *MockBackend) GetBlockCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.blocks)
}

// Clear removes all blocks from the mock backend
func (m *MockBackend) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blocks = make(map[string]*blocks.Block)
	m.storeCount = 0
	m.retrieveCount = 0
	m.hasCount = 0
}

// GetMetrics returns detailed metrics
func (m *MockBackend) GetMetrics() map[string]int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]int64{
		"stores":    m.storeCount,
		"retrieves": m.retrieveCount,
		"has_ops":   m.hasCount,
		"blocks":    int64(len(m.blocks)),
	}
}

// ResetMetrics resets operation counters
func (m *MockBackend) ResetMetrics() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.storeCount = 0
	m.retrieveCount = 0
	m.hasCount = 0
}

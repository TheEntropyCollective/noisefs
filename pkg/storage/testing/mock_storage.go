package testing

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

// MockStorageManager provides a mock implementation of storage.Manager for testing
type MockStorageManager struct {
	mu             sync.RWMutex
	blocks         map[string]*blocks.Block
	backends       map[string]*MockBackend
	defaultBackend string
	isConnected    bool
	isStarted      bool

	// Test control
	storeError      error
	retrieveError   error
	latencySimulate time.Duration
}

// NewMockStorageManager creates a new mock storage manager
func NewMockStorageManager() *MockStorageManager {
	return &MockStorageManager{
		blocks:         make(map[string]*blocks.Block),
		backends:       make(map[string]*MockBackend),
		defaultBackend: "mock",
		isConnected:    true,
		isStarted:      false,
	}
}

// Start starts the mock storage manager
func (m *MockStorageManager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.isStarted = true
	return nil
}

// Stop stops the mock storage manager
func (m *MockStorageManager) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.isStarted = false
	return nil
}

// IsConnected returns whether the mock manager is connected
func (m *MockStorageManager) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isConnected
}

// GetDefaultBackend returns the default backend
func (m *MockStorageManager) GetDefaultBackend() (storage.Backend, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if backend, exists := m.backends[m.defaultBackend]; exists {
		return backend, nil
	}

	// Create a default mock backend
	backend := NewMockBackend("mock")
	m.backends[m.defaultBackend] = backend
	return backend, nil
}

// GetBackend returns a specific backend
func (m *MockStorageManager) GetBackend(backendType string) (storage.Backend, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if backend, exists := m.backends[backendType]; exists {
		return backend, nil
	}

	return nil, fmt.Errorf("backend %s not found", backendType)
}

// Store stores a block using the default backend
func (m *MockStorageManager) Store(ctx context.Context, block *blocks.Block) (string, error) {
	if m.latencySimulate > 0 {
		time.Sleep(m.latencySimulate)
	}

	if m.storeError != nil {
		return "", m.storeError
	}

	backend, err := m.GetDefaultBackend()
	if err != nil {
		return "", err
	}

	// Use the Store method we added to MockBackend for compatibility
	if mockBackend, ok := backend.(*MockBackend); ok {
		return mockBackend.Store(ctx, block)
	}

	// Fallback to Put and extract ID
	address, err := backend.Put(ctx, block)
	if err != nil {
		return "", err
	}
	return address.ID, nil
}

// Retrieve retrieves a block using the default backend
func (m *MockStorageManager) Retrieve(ctx context.Context, cid string) (*blocks.Block, error) {
	if m.latencySimulate > 0 {
		time.Sleep(m.latencySimulate)
	}

	if m.retrieveError != nil {
		return nil, m.retrieveError
	}

	backend, err := m.GetDefaultBackend()
	if err != nil {
		return nil, err
	}

	address := &storage.BlockAddress{
		ID:          cid,
		BackendType: storage.BackendTypeIPFS,
	}

	return backend.Get(ctx, address)
}

// Has checks if a block exists
func (m *MockStorageManager) Has(ctx context.Context, cid string) (bool, error) {
	backend, err := m.GetDefaultBackend()
	if err != nil {
		return false, err
	}

	address := &storage.BlockAddress{
		ID:          cid,
		BackendType: storage.BackendTypeIPFS,
	}

	return backend.Has(ctx, address)
}

// GetStats returns mock statistics
func (m *MockStorageManager) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"blocks_stored":   len(m.blocks),
		"backends_active": len(m.backends),
		"connected":       m.isConnected,
		"started":         m.isStarted,
	}
}

// Test control methods

// SetStoreError sets an error to return on store operations
func (m *MockStorageManager) SetStoreError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.storeError = err
}

// SetRetrieveError sets an error to return on retrieve operations
func (m *MockStorageManager) SetRetrieveError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.retrieveError = err
}

// SetLatencySimulation sets artificial latency for operations
func (m *MockStorageManager) SetLatencySimulation(latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latencySimulate = latency
}

// SetConnectionState sets the connection state
func (m *MockStorageManager) SetConnectionState(connected bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.isConnected = connected
}

// AddBlock directly adds a block to the mock storage (for test setup)
func (m *MockStorageManager) AddBlock(cid string, block *blocks.Block) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blocks[cid] = block

	// Also add to default backend
	if backend, exists := m.backends[m.defaultBackend]; exists {
		backend.AddBlock(cid, block)
	}
}

// GetBlockCount returns the number of stored blocks
func (m *MockStorageManager) GetBlockCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.blocks)
}

// Clear removes all blocks from the mock storage
func (m *MockStorageManager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blocks = make(map[string]*blocks.Block)
	for _, backend := range m.backends {
		backend.Clear()
	}
}

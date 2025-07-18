package backends

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

func init() {
	storage.RegisterBackend("mock", func(config *storage.BackendConfig) (storage.Backend, error) {
		return NewMockBackend("mock", config)
	})
}

// MockBackend is a mock storage backend for testing
type MockBackend struct {
	id      string
	config  *storage.BackendConfig
	data    map[string]*blocks.Block
	mutex   sync.RWMutex
	connected bool
}

// NewMockBackend creates a new mock backend
func NewMockBackend(id string, config *storage.BackendConfig) (storage.Backend, error) {
	return &MockBackend{
		id:     id,
		config: config,
		data:   make(map[string]*blocks.Block),
		connected: true,
	}, nil
}

// Put stores a block
func (m *MockBackend) Put(ctx context.Context, block *blocks.Block) (*storage.BlockAddress, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Check if block already exists
	_, alreadyExists := m.data[block.ID]
	
	address := &storage.BlockAddress{
		ID:             block.ID,
		BackendType:    "mock",
		Metadata:       map[string]interface{}{"backend": m.id},
		WasNewlyStored: !alreadyExists, // true if this is a new block, false if it already existed
		Size:           int64(len(block.Data)),
	}
	
	m.data[block.ID] = block
	return address, nil
}

// Get retrieves a block
func (m *MockBackend) Get(ctx context.Context, address *storage.BlockAddress) (*blocks.Block, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	block, exists := m.data[address.ID]
	if !exists {
		return nil, fmt.Errorf("block not found: %s", address.ID)
	}
	
	return block, nil
}

// Has checks if a block exists
func (m *MockBackend) Has(ctx context.Context, address *storage.BlockAddress) (bool, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	_, exists := m.data[address.ID]
	return exists, nil
}

// Delete removes a block
func (m *MockBackend) Delete(ctx context.Context, address *storage.BlockAddress) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.data, address.ID)
	return nil
}

// PutMany stores multiple blocks
func (m *MockBackend) PutMany(ctx context.Context, blocks []*blocks.Block) ([]*storage.BlockAddress, error) {
	addresses := make([]*storage.BlockAddress, len(blocks))
	for i, block := range blocks {
		addr, err := m.Put(ctx, block)
		if err != nil {
			return nil, err
		}
		addresses[i] = addr
	}
	return addresses, nil
}

// GetMany retrieves multiple blocks
func (m *MockBackend) GetMany(ctx context.Context, addresses []*storage.BlockAddress) ([]*blocks.Block, error) {
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

// Pin marks a block as pinned
func (m *MockBackend) Pin(ctx context.Context, address *storage.BlockAddress) error {
	// Mock implementation - just verify block exists
	_, err := m.Has(ctx, address)
	return err
}

// Unpin removes pin from a block
func (m *MockBackend) Unpin(ctx context.Context, address *storage.BlockAddress) error {
	// Mock implementation - just verify block exists
	_, err := m.Has(ctx, address)
	return err
}

// GetBackendInfo returns information about the backend
func (m *MockBackend) GetBackendInfo() *storage.BackendInfo {
	return &storage.BackendInfo{
		Name:        m.id,
		Type:        "mock",
		Version:     "1.0.0",
		Capabilities: []string{
			storage.CapabilityBatch,
			storage.CapabilityContentAddress,
		},
		Config: map[string]interface{}{
			"connected": m.connected,
			"blocks":    len(m.data),
		},
	}
}

// HealthCheck performs a health check
func (m *MockBackend) HealthCheck(ctx context.Context) *storage.HealthStatus {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	status := "healthy"
	if !m.connected {
		status = "offline"
	}

	return &storage.HealthStatus{
		Healthy:          m.connected,
		Status:           status,
		Latency:          time.Millisecond,
		Throughput:       1000000, // 1MB/s
		ErrorRate:        0,
		UsedStorage:      int64(len(m.data) * 1024),
		AvailableStorage: 1024 * 1024 * 1024, // 1GB
		ConnectedPeers:   1,
		NetworkHealth:    "good",
		LastCheck:        time.Now(),
	}
}

// Connect establishes connection
func (m *MockBackend) Connect(ctx context.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	m.connected = true
	return nil
}

// Disconnect closes connection
func (m *MockBackend) Disconnect(ctx context.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	m.connected = false
	return nil
}

// IsConnected returns connection status
func (m *MockBackend) IsConnected() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	return m.connected
}
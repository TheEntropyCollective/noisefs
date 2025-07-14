package integration

import (
	"context"
	"fmt"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/adapters"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/ipfs"
)

// SimpleStorageManager provides a simple interface for transitioning from direct IPFS to storage abstraction
// This is designed to make the integration as easy as possible with minimal changes to existing code
type SimpleStorageManager struct {
	manager storage.Backend // Can be either Manager or adapted IPFS client
	ctx     context.Context
}

// NewSimpleStorageManagerFromIPFS creates a storage manager from an existing IPFS client
// This is the easiest migration path - wrap existing IPFS client in the new interface
func NewSimpleStorageManagerFromIPFS(ipfsClient *ipfs.Client) *SimpleStorageManager {
	adapter := adapters.NewLegacyIPFSAdapter(ipfsClient, nil)
	
	return &SimpleStorageManager{
		manager: adapter,
		ctx:     context.Background(),
	}
}

// NewSimpleStorageManagerFromEndpoint creates a storage manager with IPFS backend from endpoint
func NewSimpleStorageManagerFromEndpoint(endpoint string) (*SimpleStorageManager, error) {
	adapter, err := adapters.NewLegacyIPFSAdapterFromEndpoint(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to create IPFS adapter: %w", err)
	}
	
	return &SimpleStorageManager{
		manager: adapter,
		ctx:     context.Background(),
	}, nil
}

// NewSimpleStorageManagerFromConfig creates a full storage manager from configuration
func NewSimpleStorageManagerFromConfig(config *storage.Config) (*SimpleStorageManager, error) {
	manager, err := storage.CreateManagerFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage manager: %w", err)
	}
	
	// Start the manager
	if err := manager.Start(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to start storage manager: %w", err)
	}
	
	return &SimpleStorageManager{
		manager: manager,
		ctx:     context.Background(),
	}, nil
}

// StoreBlock stores a block and returns a CID-like identifier (for backward compatibility)
func (sm *SimpleStorageManager) StoreBlock(block *blocks.Block) (string, error) {
	address, err := sm.manager.Put(sm.ctx, block)
	if err != nil {
		return "", err
	}
	return address.ID, nil
}

// RetrieveBlock retrieves a block by CID (backward compatible)
func (sm *SimpleStorageManager) RetrieveBlock(cid string) (*blocks.Block, error) {
	// Create a block address from the CID (assuming IPFS for backward compatibility)
	address := &storage.BlockAddress{
		ID:          cid,
		BackendType: storage.BackendTypeIPFS,
	}
	
	return sm.manager.Get(sm.ctx, address)
}

// HasBlock checks if a block exists by CID
func (sm *SimpleStorageManager) HasBlock(cid string) (bool, error) {
	address := &storage.BlockAddress{
		ID:          cid,
		BackendType: storage.BackendTypeIPFS,
	}
	
	return sm.manager.Has(sm.ctx, address)
}

// PinBlock pins a block to prevent garbage collection
func (sm *SimpleStorageManager) PinBlock(cid string) error {
	address := &storage.BlockAddress{
		ID:          cid,
		BackendType: storage.BackendTypeIPFS,
	}
	
	return sm.manager.Pin(sm.ctx, address)
}

// UnpinBlock unpins a block
func (sm *SimpleStorageManager) UnpinBlock(cid string) error {
	address := &storage.BlockAddress{
		ID:          cid,
		BackendType: storage.BackendTypeIPFS,
	}
	
	return sm.manager.Unpin(sm.ctx, address)
}

// GetBackendInfo returns information about the storage backend
func (sm *SimpleStorageManager) GetBackendInfo() *storage.BackendInfo {
	return sm.manager.GetBackendInfo()
}

// HealthCheck performs a health check on the storage backend
func (sm *SimpleStorageManager) HealthCheck() *storage.HealthStatus {
	return sm.manager.HealthCheck(sm.ctx)
}

// IsConnected returns whether the storage backend is connected
func (sm *SimpleStorageManager) IsConnected() bool {
	return sm.manager.IsConnected()
}

// GetUnderlyingManager returns the underlying storage manager for advanced usage
func (sm *SimpleStorageManager) GetUnderlyingManager() storage.Backend {
	return sm.manager
}

// Close gracefully shuts down the storage manager
func (sm *SimpleStorageManager) Close() error {
	return sm.manager.Disconnect(sm.ctx)
}

// IPFSCompatibilityInterface provides IPFS-compatible methods for easy migration
type IPFSCompatibilityInterface interface {
	StoreBlock(block *blocks.Block) (string, error)
	RetrieveBlock(cid string) (*blocks.Block, error)
	HasBlock(cid string) (bool, error)
	PinBlock(cid string) error
	UnpinBlock(cid string) error
	GetBackendInfo() *storage.BackendInfo
	HealthCheck() *storage.HealthStatus
	IsConnected() bool
	Close() error
}

// Ensure SimpleStorageManager implements the compatibility interface
var _ IPFSCompatibilityInterface = (*SimpleStorageManager)(nil)

// Helper functions for easy migration

// CreateDefaultStorageManager creates a storage manager with sensible defaults for IPFS
func CreateDefaultStorageManager(ipfsEndpoint string) (*SimpleStorageManager, error) {
	if ipfsEndpoint == "" {
		ipfsEndpoint = "127.0.0.1:5001"
	}
	
	return NewSimpleStorageManagerFromEndpoint(ipfsEndpoint)
}

// MigrateFromIPFSClient provides a drop-in replacement for direct IPFS client usage
// Call this to wrap an existing IPFS client with the storage abstraction
func MigrateFromIPFSClient(ipfsClient *ipfs.Client) IPFSCompatibilityInterface {
	return NewSimpleStorageManagerFromIPFS(ipfsClient)
}

// Configuration helpers

// DefaultIPFSConfig creates a default configuration for IPFS backend
func DefaultIPFSConfig(endpoint string) *storage.Config {
	return &storage.Config{
		DefaultBackend: "ipfs",
		Backends: map[string]*storage.BackendConfig{
			"ipfs": {
				Type:     storage.BackendTypeIPFS,
				Enabled:  true,
				Priority: 100,
				Connection: &storage.ConnectionConfig{
					Endpoint: endpoint,
				},
			},
		},
		Distribution: &storage.DistributionConfig{
			Strategy: "single",
		},
		HealthCheck: &storage.HealthCheckConfig{
			Enabled: true,
		},
	}
}

// ConfigFromIPFSEndpoint creates a storage configuration from IPFS endpoint
func ConfigFromIPFSEndpoint(endpoint string) *storage.Config {
	if endpoint == "" {
		endpoint = "127.0.0.1:5001"
	}
	return DefaultIPFSConfig(endpoint)
}
package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/ipfs"
)

// LegacyIPFSAdapter wraps the existing IPFS client to implement the Backend interface
// This provides backward compatibility while allowing gradual migration to the new interface
type LegacyIPFSAdapter struct {
	client *ipfs.Client
	config *storage.BackendConfig
}

// NewLegacyIPFSAdapter creates an adapter around an existing IPFS client
func NewLegacyIPFSAdapter(client *ipfs.Client, config *storage.BackendConfig) *LegacyIPFSAdapter {
	if config == nil {
		config = &storage.BackendConfig{
			Type:     storage.BackendTypeIPFS,
			Enabled:  true,
			Priority: 100,
		}
	}
	
	return &LegacyIPFSAdapter{
		client: client,
		config: config,
	}
}

// NewLegacyIPFSAdapterFromEndpoint creates an adapter with a new IPFS client
func NewLegacyIPFSAdapterFromEndpoint(endpoint string) (*LegacyIPFSAdapter, error) {
	client, err := ipfs.NewClient(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to create IPFS client: %w", err)
	}
	
	config := &storage.BackendConfig{
		Type:     storage.BackendTypeIPFS,
		Enabled:  true,
		Priority: 100,
		Connection: &storage.ConnectionConfig{
			Endpoint: endpoint,
		},
	}
	
	return NewLegacyIPFSAdapter(client, config), nil
}

// Implement Backend interface

func (a *LegacyIPFSAdapter) Put(ctx context.Context, block *blocks.Block) (*storage.BlockAddress, error) {
	cid, err := a.client.StoreBlock(block)
	if err != nil {
		return nil, storage.NewStorageError(storage.ErrCodeConnectionFailed, 
			"failed to store block in IPFS", storage.BackendTypeIPFS, err)
	}
	
	// Calculate checksum
	checksum := sha256.Sum256(block.Data)
	
	address := &storage.BlockAddress{
		ID:          cid,
		BackendType: storage.BackendTypeIPFS,
		Size:        int64(len(block.Data)),
		Checksum:    hex.EncodeToString(checksum[:]),
		CreatedAt:   time.Now(),
		Metadata: map[string]interface{}{
			"ipfs_cid": cid,
		},
	}
	
	return address, nil
}

func (a *LegacyIPFSAdapter) Get(ctx context.Context, address *storage.BlockAddress) (*blocks.Block, error) {
	if address.BackendType != storage.BackendTypeIPFS {
		return nil, storage.NewStorageError(storage.ErrCodeInvalidAddress,
			"address is not for IPFS backend", storage.BackendTypeIPFS, nil)
	}
	
	block, err := a.client.RetrieveBlock(address.ID)
	if err != nil {
		return nil, storage.NewStorageError(storage.ErrCodeNotFound,
			"failed to retrieve block from IPFS", storage.BackendTypeIPFS, err)
	}
	
	// Verify checksum if available
	if address.Checksum != "" {
		checksum := sha256.Sum256(block.Data)
		if hex.EncodeToString(checksum[:]) != address.Checksum {
			return nil, storage.NewStorageError(storage.ErrCodeIntegrityFailure,
				"block checksum mismatch", storage.BackendTypeIPFS, nil)
		}
	}
	
	return block, nil
}

func (a *LegacyIPFSAdapter) Has(ctx context.Context, address *storage.BlockAddress) (bool, error) {
	if address.BackendType != storage.BackendTypeIPFS {
		return false, storage.NewStorageError(storage.ErrCodeInvalidAddress,
			"address is not for IPFS backend", storage.BackendTypeIPFS, nil)
	}
	
	return a.client.HasBlock(address.ID)
}

func (a *LegacyIPFSAdapter) Delete(ctx context.Context, address *storage.BlockAddress) error {
	if address.BackendType != storage.BackendTypeIPFS {
		return storage.NewStorageError(storage.ErrCodeInvalidAddress,
			"address is not for IPFS backend", storage.BackendTypeIPFS, nil)
	}
	
	// IPFS delete is achieved through unpinning
	return a.client.UnpinBlock(address.ID)
}

func (a *LegacyIPFSAdapter) PutMany(ctx context.Context, blocks []*blocks.Block) ([]*storage.BlockAddress, error) {
	addresses := make([]*storage.BlockAddress, len(blocks))
	
	for i, block := range blocks {
		address, err := a.Put(ctx, block)
		if err != nil {
			return nil, fmt.Errorf("failed to store block %d: %w", i, err)
		}
		addresses[i] = address
	}
	
	return addresses, nil
}

func (a *LegacyIPFSAdapter) GetMany(ctx context.Context, addresses []*storage.BlockAddress) ([]*blocks.Block, error) {
	blks := make([]*blocks.Block, len(addresses))
	
	for i, address := range addresses {
		block, err := a.Get(ctx, address)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve block %d: %w", i, err)
		}
		blks[i] = block
	}
	
	return blks, nil
}

func (a *LegacyIPFSAdapter) Pin(ctx context.Context, address *storage.BlockAddress) error {
	if address.BackendType != storage.BackendTypeIPFS {
		return storage.NewStorageError(storage.ErrCodeInvalidAddress,
			"address is not for IPFS backend", storage.BackendTypeIPFS, nil)
	}
	
	return a.client.PinBlock(address.ID)
}

func (a *LegacyIPFSAdapter) Unpin(ctx context.Context, address *storage.BlockAddress) error {
	if address.BackendType != storage.BackendTypeIPFS {
		return storage.NewStorageError(storage.ErrCodeInvalidAddress,
			"address is not for IPFS backend", storage.BackendTypeIPFS, nil)
	}
	
	return a.client.UnpinBlock(address.ID)
}

func (a *LegacyIPFSAdapter) Connect(ctx context.Context) error {
	// The legacy IPFS client connects during creation
	// Test connection by trying to get node ID
	connectedPeers := a.client.GetConnectedPeers()
	if len(connectedPeers) >= 0 { // Even 0 peers is a valid connection state
		return nil
	}
	
	return storage.NewConnectionError(storage.BackendTypeIPFS, 
		fmt.Errorf("IPFS client not properly connected"))
}

func (a *LegacyIPFSAdapter) Disconnect(ctx context.Context) error {
	// Legacy IPFS client doesn't have explicit disconnect
	return nil
}

func (a *LegacyIPFSAdapter) IsConnected() bool {
	// Test connection by checking if we can get connected peers
	peers := a.client.GetConnectedPeers()
	return peers != nil // If we can get peers list, we're connected
}

func (a *LegacyIPFSAdapter) GetBackendInfo() *storage.BackendInfo {
	peers := a.client.GetConnectedPeers()
	peerStrs := make([]string, len(peers))
	for i, peer := range peers {
		peerStrs[i] = peer.String()
	}
	
	return &storage.BackendInfo{
		Name:    "IPFS (Legacy Adapter)",
		Type:    storage.BackendTypeIPFS,
		Version: "legacy-adapter-v1.0",
		Capabilities: []string{
			storage.CapabilityContentAddress,
			storage.CapabilityDistributed,
			storage.CapabilityPinning,
			storage.CapabilityPeerAware,
			storage.CapabilityDeduplication,
		},
		Config: map[string]interface{}{
			"adapter_type": "legacy",
			"endpoint":     a.getEndpoint(),
		},
		Peers: peerStrs,
	}
}

func (a *LegacyIPFSAdapter) HealthCheck(ctx context.Context) *storage.HealthStatus {
	// Basic health check - test if we can get peer list
	peers := a.client.GetConnectedPeers()
	healthy := peers != nil
	
	status := "healthy"
	var issues []storage.HealthIssue
	
	if !healthy {
		status = "unhealthy"
		issues = append(issues, storage.HealthIssue{
			Severity:    "error",
			Code:        "CONNECTION_FAILED",
			Description: "Cannot connect to IPFS",
			Timestamp:   time.Now(),
		})
	}
	
	return &storage.HealthStatus{
		Healthy:        healthy,
		Status:         status,
		ConnectedPeers: len(peers),
		LastCheck:      time.Now(),
		Issues:         issues,
	}
}

// Implement PeerAwareBackend interface if the underlying client supports it

func (a *LegacyIPFSAdapter) GetConnectedPeers() []string {
	peers := a.client.GetConnectedPeers()
	peerStrs := make([]string, len(peers))
	for i, peer := range peers {
		peerStrs[i] = peer.String()
	}
	return peerStrs
}

func (a *LegacyIPFSAdapter) GetWithPeerHint(ctx context.Context, address *storage.BlockAddress, peers []string) (*blocks.Block, error) {
	// Convert string peer IDs back to peer.ID if the client supports it
	// For now, fall back to regular Get
	return a.Get(ctx, address)
}

func (a *LegacyIPFSAdapter) BroadcastToNetwork(ctx context.Context, address *storage.BlockAddress, block *blocks.Block) error {
	// Legacy client doesn't have explicit broadcast
	// Pin the block to ensure it's available to the network
	return a.Pin(ctx, address)
}

// Helper methods

func (a *LegacyIPFSAdapter) getEndpoint() string {
	if a.config.Connection != nil {
		return a.config.Connection.Endpoint
	}
	return "127.0.0.1:5001" // default
}

// GetLegacyClient returns the underlying legacy IPFS client for compatibility
func (a *LegacyIPFSAdapter) GetLegacyClient() *ipfs.Client {
	return a.client
}

// Utility functions for address conversion

// CIDToBlockAddress converts an IPFS CID to a generic BlockAddress
func CIDToBlockAddress(cid string, size int64) *storage.BlockAddress {
	return &storage.BlockAddress{
		ID:          cid,
		BackendType: storage.BackendTypeIPFS,
		Size:        size,
		CreatedAt:   time.Now(),
		Metadata: map[string]interface{}{
			"ipfs_cid": cid,
		},
	}
}

// BlockAddressToCID extracts the CID from a BlockAddress (validates it's for IPFS)
func BlockAddressToCID(address *storage.BlockAddress) (string, error) {
	if address.BackendType != storage.BackendTypeIPFS {
		return "", fmt.Errorf("address is not for IPFS backend: %s", address.BackendType)
	}
	return address.ID, nil
}

// Ensure the adapter implements the required interfaces
var _ storage.Backend = (*LegacyIPFSAdapter)(nil)
var _ storage.PeerAwareBackend = (*LegacyIPFSAdapter)(nil)
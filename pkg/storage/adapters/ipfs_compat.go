package adapters

import (
	"context"
	"fmt"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/ipfs"
	"github.com/TheEntropyCollective/noisefs/pkg/privacy/p2p"
	storageipfs "github.com/TheEntropyCollective/noisefs/pkg/storage/ipfs"
	"github.com/libp2p/go-libp2p/core/peer"
)

// IPFSCompatibilityAdapter bridges the simple ipfs.Client to PeerAwareIPFSClient interface
// This solves the interface compatibility crisis between pkg/ipfs and pkg/storage/ipfs
type IPFSCompatibilityAdapter struct {
	simpleClient ipfs.Client
	peerManager  *p2p.PeerManager
}

// NewIPFSCompatibilityAdapter creates an adapter that makes simple IPFS client compatible with PeerAwareIPFSClient
func NewIPFSCompatibilityAdapter(simpleClient ipfs.Client) *IPFSCompatibilityAdapter {
	return &IPFSCompatibilityAdapter{
		simpleClient: simpleClient,
	}
}

// StoreBlock implements BlockStore interface by converting block to bytes and calling Add
func (a *IPFSCompatibilityAdapter) StoreBlock(block *blocks.Block) (string, error) {
	ctx := context.Background()
	return a.simpleClient.Add(ctx, block.Data)
}

// RetrieveBlock implements BlockStore interface by calling Get and creating a Block
func (a *IPFSCompatibilityAdapter) RetrieveBlock(cid string) (*blocks.Block, error) {
	ctx := context.Background()
	data, err := a.simpleClient.Get(ctx, cid)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve block %s: %w", cid, err)
	}
	
	return blocks.NewBlock(data)
}

// RetrieveBlockWithPeerHint implements BlockStore with peer hint (fallback to normal retrieval)
func (a *IPFSCompatibilityAdapter) RetrieveBlockWithPeerHint(cid string, preferredPeers []peer.ID) (*blocks.Block, error) {
	// For now, ignore peer hints and use normal retrieval
	// TODO: Implement peer-specific retrieval when underlying client supports it
	return a.RetrieveBlock(cid)
}

// StoreBlockWithStrategy implements BlockStore with strategy (fallback to normal storage)
func (a *IPFSCompatibilityAdapter) StoreBlockWithStrategy(block *blocks.Block, strategy string) (string, error) {
	// For now, ignore strategy and use normal storage
	// TODO: Implement strategy-specific storage when underlying client supports it
	return a.StoreBlock(block)
}

// HasBlock implements BlockStore interface by attempting to retrieve the block
func (a *IPFSCompatibilityAdapter) HasBlock(cid string) (bool, error) {
	ctx := context.Background()
	_, err := a.simpleClient.Get(ctx, cid)
	if err != nil {
		// If we can't get it, assume it doesn't exist
		return false, nil
	}
	return true, nil
}

// SetPeerManager implements PeerAwareIPFSClient interface
func (a *IPFSCompatibilityAdapter) SetPeerManager(manager *p2p.PeerManager) {
	a.peerManager = manager
}

// GetConnectedPeers implements PeerAwareIPFSClient interface (returns empty for simple client)
func (a *IPFSCompatibilityAdapter) GetConnectedPeers() []peer.ID {
	// Simple client doesn't have peer awareness, return empty slice
	return []peer.ID{}
}

// RequestFromPeer implements PeerAwareIPFSClient interface (fallback to normal retrieval)
func (a *IPFSCompatibilityAdapter) RequestFromPeer(ctx context.Context, cid string, peerID peer.ID) (*blocks.Block, error) {
	// Simple client can't request from specific peers, use normal retrieval
	return a.RetrieveBlock(cid)
}

// BroadcastBlock implements PeerAwareIPFSClient interface (no-op for simple client)
func (a *IPFSCompatibilityAdapter) BroadcastBlock(ctx context.Context, cid string, block *blocks.Block) error {
	// Simple client doesn't support broadcasting, this is a no-op
	return nil
}

// Verify interface compliance at compile time
var _ storageipfs.PeerAwareIPFSClient = (*IPFSCompatibilityAdapter)(nil)
var _ storageipfs.BlockStore = (*IPFSCompatibilityAdapter)(nil)
package ipfs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	shell "github.com/ipfs/go-ipfs-api"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/TheEntropyCollective/noisefs/pkg/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/p2p"
)

// BlockStore defines the interface for IPFS block operations
type BlockStore interface {
	StoreBlock(block *blocks.Block) (string, error)
	RetrieveBlock(cid string) (*blocks.Block, error)
	RetrieveBlockWithPeerHint(cid string, preferredPeers []peer.ID) (*blocks.Block, error)
	StoreBlockWithStrategy(block *blocks.Block, strategy string) (string, error)
}

// PeerAwareIPFSClient extends the basic IPFS client with peer selection capabilities
type PeerAwareIPFSClient interface {
	BlockStore
	SetPeerManager(manager *p2p.PeerManager)
	GetConnectedPeers() []peer.ID
	RequestFromPeer(ctx context.Context, cid string, peerID peer.ID) (*blocks.Block, error)
	BroadcastBlock(ctx context.Context, cid string, block *blocks.Block) error
}

// Client handles interaction with IPFS with peer selection integration
type Client struct {
	shell          *shell.Shell
	peerManager    *p2p.PeerManager
	requestMetrics map[peer.ID]*RequestMetrics
	metricsLock    sync.RWMutex
}

// RequestMetrics tracks request performance to individual peers
type RequestMetrics struct {
	TotalRequests  int64
	SuccessfulRequests int64
	FailedRequests int64
	AverageLatency time.Duration
	LastRequest    time.Time
	Bandwidth      float64 // bytes per second
}

// NewClient creates a new IPFS client
func NewClient(apiURL string) (*Client, error) {
	if apiURL == "" {
		apiURL = "localhost:5001" // Default IPFS API endpoint
	}
	
	sh := shell.NewShell(apiURL)
	
	// Test connection
	if _, err := sh.ID(); err != nil {
		return nil, fmt.Errorf("failed to connect to IPFS: %w", err)
	}
	
	return &Client{
		shell:          sh,
		requestMetrics: make(map[peer.ID]*RequestMetrics),
	}, nil
}

// SetPeerManager sets the peer manager for intelligent peer selection
func (c *Client) SetPeerManager(manager *p2p.PeerManager) {
	c.peerManager = manager
}

// GetConnectedPeers returns a list of currently connected IPFS peers
func (c *Client) GetConnectedPeers() []peer.ID {
	// Use IPFS shell to get connected peers
	ctx := context.Background()
	peers, err := c.shell.SwarmPeers(ctx)
	if err != nil {
		return []peer.ID{}
	}
	
	peerIDs := make([]peer.ID, 0, len(peers.Peers))
	for _, p := range peers.Peers {
		if peerID, err := peer.Decode(p.Peer); err == nil {
			peerIDs = append(peerIDs, peerID)
		}
	}
	
	return peerIDs
}

// StoreBlock stores a block in IPFS and returns its CID
func (c *Client) StoreBlock(block *blocks.Block) (string, error) {
	if block == nil {
		return "", errors.New("block cannot be nil")
	}
	
	reader := bytes.NewReader(block.Data)
	cid, err := c.shell.Add(reader)
	if err != nil {
		return "", fmt.Errorf("failed to store block: %w", err)
	}
	
	return cid, nil
}

// RetrieveBlock retrieves a block from IPFS by its CID
func (c *Client) RetrieveBlock(cid string) (*blocks.Block, error) {
	return c.RetrieveBlockWithPeerHint(cid, nil)
}

// RetrieveBlockWithPeerHint retrieves a block with preferred peer hints
func (c *Client) RetrieveBlockWithPeerHint(cid string, preferredPeers []peer.ID) (*blocks.Block, error) {
	if cid == "" {
		return nil, errors.New("CID cannot be empty")
	}
	
	ctx := context.Background()
	
	// If we have a peer manager and preferred peers, try them first
	if c.peerManager != nil && len(preferredPeers) > 0 {
		for _, peerID := range preferredPeers {
			if block, err := c.RequestFromPeer(ctx, cid, peerID); err == nil {
				return block, nil
			}
		}
	}
	
	// If peer selection is available, use intelligent peer selection
	if c.peerManager != nil {
		return c.retrieveBlockWithPeerSelection(ctx, cid)
	}
	
	// Fallback to standard IPFS retrieval
	return c.retrieveBlockStandard(cid)
}

// retrieveBlockWithPeerSelection uses peer selection strategies for block retrieval
func (c *Client) retrieveBlockWithPeerSelection(ctx context.Context, cid string) (*blocks.Block, error) {
	// Use the performance strategy by default for block retrieval
	criteria := p2p.SelectionCriteria{
		Count:          3,
		RequiredBlocks: []string{cid},
	}
	selectedPeers, err := c.peerManager.SelectPeers(ctx, "performance", criteria)
	if err != nil {
		// If peer selection fails, fall back to standard retrieval
		return c.retrieveBlockStandard(cid)
	}
	
	// Try to retrieve from selected peers in parallel
	type result struct {
		block *blocks.Block
		err   error
	}
	
	resultChan := make(chan result, len(selectedPeers))
	
	for _, peerID := range selectedPeers {
		go func(pid peer.ID) {
			block, err := c.RequestFromPeer(ctx, cid, pid)
			resultChan <- result{block: block, err: err}
		}(peerID)
	}
	
	// Return the first successful result
	for i := 0; i < len(selectedPeers); i++ {
		select {
		case res := <-resultChan:
			if res.err == nil {
				return res.block, nil
			}
		case <-time.After(5 * time.Second):
			// Timeout after 5 seconds
			break
		}
	}
	
	// If all peer requests fail, fall back to standard retrieval
	return c.retrieveBlockStandard(cid)
}

// retrieveBlockStandard performs standard IPFS block retrieval
func (c *Client) retrieveBlockStandard(cid string) (*blocks.Block, error) {
	reader, err := c.shell.Cat(cid)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve block: %w", err)
	}
	defer reader.Close()
	
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read block data: %w", err)
	}
	
	return blocks.NewBlock(data)
}

// StoreBlocks stores multiple blocks in IPFS and returns their CIDs
func (c *Client) StoreBlocks(blks []*blocks.Block) ([]string, error) {
	if len(blks) == 0 {
		return nil, errors.New("no blocks to store")
	}
	
	cids := make([]string, len(blks))
	
	for i, block := range blks {
		cid, err := c.StoreBlock(block)
		if err != nil {
			return nil, fmt.Errorf("failed to store block %d: %w", i, err)
		}
		cids[i] = cid
	}
	
	return cids, nil
}

// RetrieveBlocks retrieves multiple blocks from IPFS by their CIDs
func (c *Client) RetrieveBlocks(cids []string) ([]*blocks.Block, error) {
	if len(cids) == 0 {
		return nil, errors.New("no CIDs provided")
	}
	
	blks := make([]*blocks.Block, len(cids))
	
	for i, cid := range cids {
		block, err := c.RetrieveBlock(cid)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve block %d: %w", i, err)
		}
		blks[i] = block
	}
	
	return blks, nil
}

// PinBlock pins a block in IPFS to prevent garbage collection
func (c *Client) PinBlock(cid string) error {
	if cid == "" {
		return errors.New("CID cannot be empty")
	}
	
	return c.shell.Pin(cid)
}

// UnpinBlock unpins a block in IPFS
func (c *Client) UnpinBlock(cid string) error {
	if cid == "" {
		return errors.New("CID cannot be empty")
	}
	
	return c.shell.Unpin(cid)
}

// Add stores data in IPFS and returns its CID
func (c *Client) Add(reader io.Reader) (string, error) {
	if reader == nil {
		return "", errors.New("reader cannot be nil")
	}
	
	return c.shell.Add(reader)
}

// Cat retrieves data from IPFS by its CID
func (c *Client) Cat(cid string) (io.ReadCloser, error) {
	if cid == "" {
		return nil, errors.New("CID cannot be empty")
	}
	
	return c.shell.Cat(cid)
}

// StoreBlockWithStrategy stores a block using a specific peer selection strategy
func (c *Client) StoreBlockWithStrategy(block *blocks.Block, strategy string) (string, error) {
	if block == nil {
		return "", errors.New("block cannot be nil")
	}
	
	// For now, we store locally and then broadcast to selected peers
	cid, err := c.StoreBlock(block)
	if err != nil {
		return "", err
	}
	
	// If peer manager is available, broadcast to strategically selected peers
	if c.peerManager != nil {
		ctx := context.Background()
		go c.BroadcastBlock(ctx, cid, block)
	}
	
	return cid, nil
}

// RequestFromPeer requests a specific block from a specific peer
func (c *Client) RequestFromPeer(ctx context.Context, cid string, peerID peer.ID) (*blocks.Block, error) {
	startTime := time.Now()
	
	// Update metrics
	defer func() {
		duration := time.Since(startTime)
		c.updateRequestMetrics(peerID, duration, true) // We'll set success/failure later
	}()
	
	// For IPFS shell, we can't directly request from a specific peer,
	// but we can try to connect to the peer first
	peerAddr := "/p2p/" + peerID.String()
	
	// Try to connect to the specific peer
	if err := c.shell.SwarmConnect(ctx, peerAddr); err != nil {
		c.updateRequestMetrics(peerID, time.Since(startTime), false)
		return nil, fmt.Errorf("failed to connect to peer %s: %w", peerID, err)
	}
	
	// Now try to retrieve the block
	block, err := c.retrieveBlockStandard(cid)
	if err != nil {
		c.updateRequestMetrics(peerID, time.Since(startTime), false)
		return nil, err
	}
	
	c.updateRequestMetrics(peerID, time.Since(startTime), true)
	return block, nil
}

// BroadcastBlock broadcasts a block to selected peers for redundancy
func (c *Client) BroadcastBlock(ctx context.Context, cid string, block *blocks.Block) error {
	if c.peerManager == nil {
		return nil // No peer manager, skip broadcast
	}
	
	// Select peers for broadcasting using randomizer-aware strategy
	criteria := p2p.SelectionCriteria{
		Count:             5,
		PreferRandomizers: true,
		RequiredBlocks:    []string{cid},
	}
	selectedPeers, err := c.peerManager.SelectPeers(ctx, "randomizer", criteria)
	if err != nil {
		return fmt.Errorf("failed to select peers for broadcast: %w", err)
	}
	
	// Broadcast to selected peers in parallel
	var wg sync.WaitGroup
	for _, peerID := range selectedPeers {
		wg.Add(1)
		go func(pid peer.ID) {
			defer wg.Done()
			
			// Connect to peer and ensure they have the block
			peerAddr := "/p2p/" + pid.String()
			if err := c.shell.SwarmConnect(ctx, peerAddr); err != nil {
				return // Skip if we can't connect
			}
			
			// Pin the block to ensure it's replicated
			c.shell.Pin(cid)
		}(peerID)
	}
	
	wg.Wait()
	return nil
}

// updateRequestMetrics updates performance metrics for a peer
func (c *Client) updateRequestMetrics(peerID peer.ID, latency time.Duration, success bool) {
	c.metricsLock.Lock()
	defer c.metricsLock.Unlock()
	
	metrics, exists := c.requestMetrics[peerID]
	if !exists {
		metrics = &RequestMetrics{}
		c.requestMetrics[peerID] = metrics
	}
	
	metrics.TotalRequests++
	metrics.LastRequest = time.Now()
	
	if success {
		metrics.SuccessfulRequests++
		
		// Update average latency with exponential moving average
		if metrics.AverageLatency == 0 {
			metrics.AverageLatency = latency
		} else {
			alpha := 0.1 // Smoothing factor
			metrics.AverageLatency = time.Duration(
				float64(metrics.AverageLatency)*(1-alpha) + float64(latency)*alpha,
			)
		}
	} else {
		metrics.FailedRequests++
	}
	
	// Update peer manager with latest metrics if available
	if c.peerManager != nil {
		successRate := float64(metrics.SuccessfulRequests) / float64(metrics.TotalRequests)
		c.peerManager.UpdatePeerMetrics(peerID, success, latency, int64(successRate*100))
	}
}

// GetPeerMetrics returns current performance metrics for all peers
func (c *Client) GetPeerMetrics() map[peer.ID]*RequestMetrics {
	c.metricsLock.RLock()
	defer c.metricsLock.RUnlock()
	
	// Create a copy to avoid race conditions
	copy := make(map[peer.ID]*RequestMetrics)
	for peerID, metrics := range c.requestMetrics {
		metricsCopy := *metrics
		copy[peerID] = &metricsCopy
	}
	
	return copy
}
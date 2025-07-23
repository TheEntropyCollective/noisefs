package backends

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	shell "github.com/ipfs/go-ipfs-api"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/privacy/p2p"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

// IPFSBackend implements the storage.Backend interface for IPFS
type IPFSBackend struct {
	config          *storage.BackendConfig
	shell           *shell.Shell
	peerManager     *p2p.PeerManager
	errorClassifier *storage.ErrorClassifier
	errorReporter   storage.ErrorReporter

	// Connection state
	connected   bool
	connectedAt time.Time

	// Performance tracking
	requestMetrics map[peer.ID]*RequestMetrics
	metricsLock    sync.RWMutex

	// Health monitoring
	lastHealthCheck time.Time
	healthStatus    *storage.HealthStatus
	healthLock      sync.RWMutex
}

// RequestMetrics tracks request performance to individual peers
type RequestMetrics struct {
	TotalRequests      int64
	SuccessfulRequests int64
	FailedRequests     int64
	AverageLatency     time.Duration
	LastRequest        time.Time
	Bandwidth          float64 // bytes per second
}

// NewIPFSBackend creates a new IPFS storage backend
func NewIPFSBackend(config *storage.BackendConfig) (*IPFSBackend, error) {
	if config.Type != storage.BackendTypeIPFS {
		return nil, fmt.Errorf("invalid backend type: expected %s, got %s", storage.BackendTypeIPFS, config.Type)
	}

	backend := &IPFSBackend{
		config:          config,
		errorClassifier: storage.NewErrorClassifier(storage.BackendTypeIPFS),
		errorReporter:   storage.NewDefaultErrorReporter(),
		requestMetrics:  make(map[peer.ID]*RequestMetrics),
		healthStatus: &storage.HealthStatus{
			Healthy:   false,
			Status:    "disconnected",
			LastCheck: time.Now(),
		},
	}

	return backend, nil
}

// Connect establishes connection to IPFS node
func (ipfs *IPFSBackend) Connect(ctx context.Context) error {
	endpoint := ipfs.config.Connection.Endpoint
	if endpoint == "" {
		endpoint = "127.0.0.1:5001"
	}

	ipfs.shell = shell.NewShell(endpoint)

	// Test connection
	if _, err := ipfs.shell.ID(); err != nil {
		storageErr := ipfs.errorClassifier.ClassifyError(err, "connect", nil)
		ipfs.errorReporter.ReportError(storageErr)
		return storageErr
	}

	ipfs.connected = true
	ipfs.connectedAt = time.Now()

	// Update health status
	ipfs.updateHealthStatus()

	return nil
}

// Disconnect closes connection to IPFS node
func (ipfs *IPFSBackend) Disconnect(ctx context.Context) error {
	ipfs.connected = false
	ipfs.shell = nil

	ipfs.healthLock.Lock()
	ipfs.healthStatus.Healthy = false
	ipfs.healthStatus.Status = "disconnected"
	ipfs.healthLock.Unlock()

	return nil
}

// IsConnected returns true if connected to IPFS
func (ipfs *IPFSBackend) IsConnected() bool {
	return ipfs.connected && ipfs.shell != nil
}

// Put stores a block in IPFS and returns its address
func (ipfs *IPFSBackend) Put(ctx context.Context, block *blocks.Block) (*storage.BlockAddress, error) {
	if !ipfs.IsConnected() {
		err := storage.NewConnectionError(storage.BackendTypeIPFS, fmt.Errorf("not connected to IPFS"))
		ipfs.errorReporter.ReportError(err)
		return nil, err
	}

	reader := bytes.NewReader(block.Data)

	cid, err := ipfs.shell.Add(reader)
	if err != nil {
		storageErr := ipfs.errorClassifier.ClassifyError(err, "put", nil)
		ipfs.errorReporter.ReportError(storageErr)
		return nil, storageErr
	}


	address := &storage.BlockAddress{
		ID:          cid,
		BackendType: storage.BackendTypeIPFS,
		Size:        int64(len(block.Data)),
		CreatedAt:   time.Now(),
	}

	return address, nil
}

// Get retrieves a block from IPFS by its address
func (ipfs *IPFSBackend) Get(ctx context.Context, address *storage.BlockAddress) (*blocks.Block, error) {
	if !ipfs.IsConnected() {
		err := storage.NewConnectionError(storage.BackendTypeIPFS, fmt.Errorf("not connected to IPFS"))
		ipfs.errorReporter.ReportError(err)
		return nil, err
	}

	if address.BackendType != storage.BackendTypeIPFS {
		err := storage.NewInvalidRequestError(storage.BackendTypeIPFS,
			"address is not for IPFS backend", nil)
		err.Address = address
		ipfs.errorReporter.ReportError(err)
		return nil, err
	}


	// Try intelligent peer selection if available
	if ipfs.peerManager != nil {
		if block, err := ipfs.getWithPeerSelection(ctx, address); err == nil {
			return block, nil
		}
	}

	// Fallback to standard IPFS retrieval
	block, err := ipfs.getStandard(address.ID)
	if err != nil {
		storageErr := ipfs.errorClassifier.ClassifyError(err, "get", address)
		ipfs.errorReporter.ReportError(storageErr)
		return nil, storageErr
	}


	return block, nil
}

// Has checks if a block exists in IPFS
func (ipfs *IPFSBackend) Has(ctx context.Context, address *storage.BlockAddress) (bool, error) {
	if !ipfs.IsConnected() {
		err := storage.NewConnectionError(storage.BackendTypeIPFS, fmt.Errorf("not connected to IPFS"))
		ipfs.errorReporter.ReportError(err)
		return false, err
	}

	if address.BackendType != storage.BackendTypeIPFS {
		return false, storage.NewInvalidRequestError(storage.BackendTypeIPFS,
			"address is not for IPFS backend", nil)
	}

	// Try to stat the object (faster than full retrieval)
	_, err := ipfs.shell.ObjectStat(address.ID)
	if err != nil {
		if ipfs.errorClassifier.ClassifyError(err, "has", address).Code == storage.ErrCodeNotFound {
			return false, nil
		}
		storageErr := ipfs.errorClassifier.ClassifyError(err, "has", address)
		ipfs.errorReporter.ReportError(storageErr)
		return false, storageErr
	}

	return true, nil
}

// Delete removes a block from IPFS (unpins it)
func (ipfs *IPFSBackend) Delete(ctx context.Context, address *storage.BlockAddress) error {
	if !ipfs.IsConnected() {
		err := storage.NewConnectionError(storage.BackendTypeIPFS, fmt.Errorf("not connected to IPFS"))
		ipfs.errorReporter.ReportError(err)
		return err
	}

	if address.BackendType != storage.BackendTypeIPFS {
		err := storage.NewInvalidRequestError(storage.BackendTypeIPFS,
			"address is not for IPFS backend", nil)
		err.Address = address
		ipfs.errorReporter.ReportError(err)
		return err
	}

	// In IPFS, delete means unpin (actual deletion happens during GC)
	err := ipfs.shell.Unpin(address.ID)
	if err != nil {
		storageErr := ipfs.errorClassifier.ClassifyError(err, "delete", address)
		ipfs.errorReporter.ReportError(storageErr)
		return storageErr
	}

	return nil
}

// PutMany stores multiple blocks in IPFS
func (ipfs *IPFSBackend) PutMany(ctx context.Context, blocks []*blocks.Block) ([]*storage.BlockAddress, error) {
	if len(blocks) == 0 {
		return []*storage.BlockAddress{}, nil
	}

	addresses := make([]*storage.BlockAddress, len(blocks))

	for i, block := range blocks {
		address, err := ipfs.Put(ctx, block)
		if err != nil {
			return nil, fmt.Errorf("failed to store block %d: %w", i, err)
		}
		addresses[i] = address
	}

	return addresses, nil
}

// GetMany retrieves multiple blocks from IPFS
func (ipfs *IPFSBackend) GetMany(ctx context.Context, addresses []*storage.BlockAddress) ([]*blocks.Block, error) {
	if len(addresses) == 0 {
		return []*blocks.Block{}, nil
	}

	blocks := make([]*blocks.Block, len(addresses))

	for i, address := range addresses {
		block, err := ipfs.Get(ctx, address)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve block %d: %w", i, err)
		}
		blocks[i] = block
	}

	return blocks, nil
}

// Pin pins a block in IPFS to prevent garbage collection
func (ipfs *IPFSBackend) Pin(ctx context.Context, address *storage.BlockAddress) error {
	if !ipfs.IsConnected() {
		err := storage.NewConnectionError(storage.BackendTypeIPFS, fmt.Errorf("not connected to IPFS"))
		ipfs.errorReporter.ReportError(err)
		return err
	}

	if address.BackendType != storage.BackendTypeIPFS {
		err := storage.NewInvalidRequestError(storage.BackendTypeIPFS,
			"address is not for IPFS backend", nil)
		err.Address = address
		ipfs.errorReporter.ReportError(err)
		return err
	}

	err := ipfs.shell.Pin(address.ID)
	if err != nil {
		storageErr := ipfs.errorClassifier.ClassifyError(err, "pin", address)
		ipfs.errorReporter.ReportError(storageErr)
		return storageErr
	}

	return nil
}

// Unpin unpins a block in IPFS
func (ipfs *IPFSBackend) Unpin(ctx context.Context, address *storage.BlockAddress) error {
	if !ipfs.IsConnected() {
		err := storage.NewConnectionError(storage.BackendTypeIPFS, fmt.Errorf("not connected to IPFS"))
		ipfs.errorReporter.ReportError(err)
		return err
	}

	if address.BackendType != storage.BackendTypeIPFS {
		err := storage.NewInvalidRequestError(storage.BackendTypeIPFS,
			"address is not for IPFS backend", nil)
		err.Address = address
		ipfs.errorReporter.ReportError(err)
		return err
	}

	err := ipfs.shell.Unpin(address.ID)
	if err != nil {
		storageErr := ipfs.errorClassifier.ClassifyError(err, "unpin", address)
		ipfs.errorReporter.ReportError(storageErr)
		return storageErr
	}

	return nil
}

// GetBackendInfo returns information about the IPFS backend
func (ipfs *IPFSBackend) GetBackendInfo() *storage.BackendInfo {
	info := &storage.BackendInfo{
		Name:    "IPFS",
		Type:    storage.BackendTypeIPFS,
		Version: "go-ipfs-api",
		Capabilities: []string{
			storage.CapabilityContentAddress,
			storage.CapabilityDistributed,
			storage.CapabilityPinning,
			storage.CapabilityPeerAware,
			storage.CapabilityDeduplication,
		},
		Config: map[string]interface{}{
			"endpoint": ipfs.config.Connection.Endpoint,
			"enabled":  ipfs.config.Enabled,
			"priority": ipfs.config.Priority,
		},
	}

	if ipfs.IsConnected() {
		// Get network ID and peers
		if id, err := ipfs.shell.ID(); err == nil {
			info.NetworkID = id.ID
		}

		if peers := ipfs.getConnectedPeers(); len(peers) > 0 {
			info.Peers = peers
		}
	}

	return info
}

// HealthCheck performs a health check on the IPFS backend
func (ipfs *IPFSBackend) HealthCheck(ctx context.Context) *storage.HealthStatus {
	ipfs.healthLock.Lock()
	defer ipfs.healthLock.Unlock()

	ipfs.lastHealthCheck = time.Now()

	if !ipfs.IsConnected() {
		ipfs.healthStatus = &storage.HealthStatus{
			Healthy:   false,
			Status:    "offline",
			LastCheck: ipfs.lastHealthCheck,
		}
		return ipfs.healthStatus
	}

	// Perform basic connectivity test
	_, err := ipfs.shell.ID()

	if err != nil {
		ipfs.healthStatus = &storage.HealthStatus{
			Healthy:   false,
			Status:    "offline",
			LastCheck: ipfs.lastHealthCheck,
		}
		return ipfs.healthStatus
	}

	ipfs.healthStatus = &storage.HealthStatus{
		Healthy:   true,
		Status:    "healthy",
		LastCheck: ipfs.lastHealthCheck,
	}

	return ipfs.healthStatus
}

// SetPeerManager sets the peer manager for intelligent peer selection
func (ipfs *IPFSBackend) SetPeerManager(manager interface{}) error {
	if peerMgr, ok := manager.(*p2p.PeerManager); ok {
		ipfs.peerManager = peerMgr
		return nil
	}
	return fmt.Errorf("invalid peer manager type: expected *p2p.PeerManager")
}

// GetConnectedPeers returns connected peer IDs (implements PeerAwareBackend)
func (ipfs *IPFSBackend) GetConnectedPeers() []string {
	return ipfs.getConnectedPeers()
}

// GetWithPeerHint retrieves block with peer hints (implements PeerAwareBackend)
func (ipfs *IPFSBackend) GetWithPeerHint(ctx context.Context, address *storage.BlockAddress, peers []string) (*blocks.Block, error) {
	// Convert string peer IDs to peer.ID
	peerIDs := make([]peer.ID, 0, len(peers))
	for _, peerStr := range peers {
		if peerID, err := peer.Decode(peerStr); err == nil {
			peerIDs = append(peerIDs, peerID)
		}
	}

	// Try to retrieve from preferred peers first
	for _, peerID := range peerIDs {
		if block, err := ipfs.requestFromPeer(ctx, address.ID, peerID); err == nil {
			return block, nil
		}
	}

	// Fallback to standard retrieval
	return ipfs.Get(ctx, address)
}

// BroadcastToNetwork broadcasts a block to the network (implements PeerAwareBackend)
func (ipfs *IPFSBackend) BroadcastToNetwork(ctx context.Context, address *storage.BlockAddress, block *blocks.Block) error {
	if ipfs.peerManager == nil {
		return nil // No peer manager, skip broadcast
	}

	// Select peers for broadcasting
	criteria := p2p.SelectionCriteria{
		Count:             5,
		PreferRandomizers: true,
		RequiredBlocks:    []string{address.ID},
	}
	selectedPeers, err := ipfs.peerManager.SelectPeers(ctx, "randomizer", criteria)
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
			if err := ipfs.shell.SwarmConnect(ctx, peerAddr); err != nil {
				return // Skip if we can't connect
			}

			// Pin the block to ensure it's replicated
			ipfs.shell.Pin(address.ID)
		}(peerID)
	}

	wg.Wait()
	return nil
}

// Helper methods

func (ipfs *IPFSBackend) getStandard(cid string) (*blocks.Block, error) {
	reader, err := ipfs.shell.Cat(cid)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	return blocks.NewBlock(data)
}

func (ipfs *IPFSBackend) getWithPeerSelection(ctx context.Context, address *storage.BlockAddress) (*blocks.Block, error) {
	criteria := p2p.SelectionCriteria{
		Count:          3,
		RequiredBlocks: []string{address.ID},
	}

	selectedPeers, err := ipfs.peerManager.SelectPeers(ctx, "performance", criteria)
	if err != nil {
		return nil, err
	}

	// Try to retrieve from selected peers in parallel
	type result struct {
		block *blocks.Block
		err   error
	}

	resultChan := make(chan result, len(selectedPeers))

	for _, peerID := range selectedPeers {
		go func(pid peer.ID) {
			block, err := ipfs.requestFromPeer(ctx, address.ID, pid)
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
			break
		}
	}

	return nil, fmt.Errorf("failed to retrieve from all selected peers")
}

func (ipfs *IPFSBackend) requestFromPeer(ctx context.Context, cid string, peerID peer.ID) (*blocks.Block, error) {
	start := time.Now()
	
	// Connect to the specific peer
	peerAddr := "/p2p/" + peerID.String()
	if err := ipfs.shell.SwarmConnect(ctx, peerAddr); err != nil {
		ipfs.updateRequestMetrics(peerID, time.Since(start), false)
		return nil, err
	}

	// Retrieve the block
	block, err := ipfs.getStandard(cid)
	if err != nil {
		ipfs.updateRequestMetrics(peerID, time.Since(start), false)
		return nil, err
	}

	ipfs.updateRequestMetrics(peerID, time.Since(start), true)
	return block, nil
}

func (ipfs *IPFSBackend) getConnectedPeers() []string {
	if !ipfs.IsConnected() {
		return []string{}
	}

	ctx := context.Background()
	peers, err := ipfs.shell.SwarmPeers(ctx)
	if err != nil {
		return []string{}
	}

	peerStrs := make([]string, 0, len(peers.Peers))
	for _, p := range peers.Peers {
		peerStrs = append(peerStrs, p.Peer)
	}

	return peerStrs
}

func (ipfs *IPFSBackend) updateRequestMetrics(peerID peer.ID, latency time.Duration, success bool) {
	ipfs.metricsLock.Lock()
	defer ipfs.metricsLock.Unlock()

	metrics, exists := ipfs.requestMetrics[peerID]
	if !exists {
		metrics = &RequestMetrics{}
		ipfs.requestMetrics[peerID] = metrics
	}

	metrics.TotalRequests++
	metrics.LastRequest = time.Now()

	if success {
		metrics.SuccessfulRequests++

		// Update average latency with exponential moving average
		if metrics.AverageLatency == 0 {
			metrics.AverageLatency = latency
		} else {
			alpha := 0.1
			metrics.AverageLatency = time.Duration(
				float64(metrics.AverageLatency)*(1-alpha) + float64(latency)*alpha,
			)
		}
	} else {
		metrics.FailedRequests++
	}

	// Update peer manager with latest metrics if available
	if ipfs.peerManager != nil {
		successRate := float64(metrics.SuccessfulRequests) / float64(metrics.TotalRequests)
		ipfs.peerManager.UpdatePeerMetrics(peerID, success, latency, int64(successRate*100))
	}
}

func (ipfs *IPFSBackend) updateHealthStatus() {
	ipfs.healthLock.Lock()
	defer ipfs.healthLock.Unlock()

	if ipfs.IsConnected() {
		ipfs.healthStatus.Healthy = true
		ipfs.healthStatus.Status = "healthy"
	} else {
		ipfs.healthStatus.Healthy = false
		ipfs.healthStatus.Status = "disconnected"
	}

	ipfs.healthStatus.LastCheck = time.Now()
}

// Ensure IPFSBackend implements all required interfaces
var _ storage.Backend = (*IPFSBackend)(nil)
var _ storage.PeerAwareBackend = (*IPFSBackend)(nil)

// init registers the IPFS backend constructor
func init() {
	storage.RegisterBackend(storage.BackendTypeIPFS, func(config *storage.BackendConfig) (storage.Backend, error) {
		return NewIPFSBackend(config)
	})
}

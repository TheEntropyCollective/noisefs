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

// MockIPFSClient provides a comprehensive mock implementation of PeerAwareBackend
// with full IPFS simulation capabilities for unit testing
type MockIPFSClient struct {
	mu sync.RWMutex

	// Storage
	blocks       map[string]*blocks.Block
	pinnedBlocks map[string]bool

	// Network simulation
	peers          []string
	connectedPeers map[string]bool
	networkHealth  string

	// Test controls
	isConnected    bool
	errorMode      map[string]error // operation -> error mapping
	latency        time.Duration
	operationDelay map[string]time.Duration

	// Metrics and tracking
	operationCount   map[string]int64
	operationHistory []OperationRecord

	// Advanced features
	bandwidthLimit   int64 // bytes per second
	storageQuota     int64 // max storage bytes
	currentStorage   int64
	requestRateLimit int // requests per second

	// Failure simulation
	failureRate     float64 // 0.0 to 1.0
	failureCount    int
	totalOperations int

	// State persistence (for complex testing scenarios)
	statePersistence bool
	stateFilePath    string

	// Network simulator integration
	networkSim *NetworkSimulator
}

// OperationRecord tracks individual operations for debugging
type OperationRecord struct {
	Timestamp time.Time
	Operation string
	BlockID   string
	PeerHints []string
	Success   bool
	Error     string
	Duration  time.Duration
	Metadata  map[string]interface{}
}

// SetNetworkSimulator links this client with a network simulator for block sync
func (m *MockIPFSClient) SetNetworkSimulator(networkSim *NetworkSimulator) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.networkSim = networkSim
}

// NewMockIPFSClient creates a new mock IPFS client with default settings
func NewMockIPFSClient() *MockIPFSClient {
	return &MockIPFSClient{
		blocks:           make(map[string]*blocks.Block),
		pinnedBlocks:     make(map[string]bool),
		peers:            []string{"peer1", "peer2", "peer3"},
		connectedPeers:   map[string]bool{"peer1": true, "peer2": true, "peer3": true},
		networkHealth:    "good",
		isConnected:      true,
		errorMode:        make(map[string]error),
		operationDelay:   make(map[string]time.Duration),
		operationCount:   make(map[string]int64),
		operationHistory: make([]OperationRecord, 0),
		latency:          0,
		bandwidthLimit:   1000000,    // 1MB/s default
		storageQuota:     1000000000, // 1GB default
		requestRateLimit: 100,        // 100 req/s default
		failureRate:      0.0,
	}
}

// Core Backend interface implementation

// Put stores a block and returns its address
func (m *MockIPFSClient) Put(ctx context.Context, block *blocks.Block) (*storage.BlockAddress, error) {
	start := time.Now()
	defer func() {
		m.recordOperation("put", "", nil, start, nil)
	}()

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.simulateOperation("put"); err != nil {
		return nil, err
	}

	// Check storage quota
	blockSize := int64(len(block.Data))
	if m.currentStorage+blockSize > m.storageQuota {
		return nil, storage.NewStorageError(storage.ErrCodeQuotaExceeded, "storage quota exceeded", storage.BackendTypeIPFS, nil)
	}

	// Generate deterministic CID
	hash := sha256.Sum256(block.Data)
	cid := "Qm" + hex.EncodeToString(hash[:16]) // IPFS-like CID prefix

	m.blocks[cid] = block
	m.currentStorage += blockSize
	m.operationCount["put"]++

	// Sync with network simulator if available
	if m.networkSim != nil {
		m.networkSim.SyncBlockFromClient(cid, block)
	}

	address := &storage.BlockAddress{
		ID:          cid,
		BackendType: storage.BackendTypeIPFS,
		Size:        blockSize,
		CreatedAt:   time.Now(),
		Providers:   m.getConnectedPeersList(),
		Metadata: map[string]interface{}{
			"ipfs_version": "mock-1.0",
			"block_type":   "raw",
		},
	}

	return address, nil
}

// Get retrieves a block by address
func (m *MockIPFSClient) Get(ctx context.Context, address *storage.BlockAddress) (*blocks.Block, error) {
	start := time.Now()
	defer func() {
		m.recordOperation("get", address.ID, nil, start, nil)
	}()

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.simulateOperation("get"); err != nil {
		return nil, err
	}

	block, exists := m.blocks[address.ID]
	if !exists {
		return nil, storage.NewNotFoundError(storage.BackendTypeIPFS, address)
	}

	m.operationCount["get"]++

	// Update access time
	address.AccessedAt = time.Now()

	return block, nil
}

// Has checks if a block exists
func (m *MockIPFSClient) Has(ctx context.Context, address *storage.BlockAddress) (bool, error) {
	start := time.Now()
	defer func() {
		m.recordOperation("has", address.ID, nil, start, nil)
	}()

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.simulateOperation("has"); err != nil {
		return false, err
	}

	_, exists := m.blocks[address.ID]
	m.operationCount["has"]++

	return exists, nil
}

// Delete removes a block
func (m *MockIPFSClient) Delete(ctx context.Context, address *storage.BlockAddress) error {
	start := time.Now()
	defer func() {
		m.recordOperation("delete", address.ID, nil, start, nil)
	}()

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.simulateOperation("delete"); err != nil {
		return err
	}

	if block, exists := m.blocks[address.ID]; exists {
		m.currentStorage -= int64(len(block.Data))
		delete(m.blocks, address.ID)
		delete(m.pinnedBlocks, address.ID)
		m.operationCount["delete"]++
	}

	return nil
}

// PutMany stores multiple blocks
func (m *MockIPFSClient) PutMany(ctx context.Context, blocks []*blocks.Block) ([]*storage.BlockAddress, error) {
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

// GetMany retrieves multiple blocks
func (m *MockIPFSClient) GetMany(ctx context.Context, addresses []*storage.BlockAddress) ([]*blocks.Block, error) {
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

// Pin marks a block as pinned
func (m *MockIPFSClient) Pin(ctx context.Context, address *storage.BlockAddress) error {
	start := time.Now()
	defer func() {
		m.recordOperation("pin", address.ID, nil, start, nil)
	}()

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.simulateOperation("pin"); err != nil {
		return err
	}

	if _, exists := m.blocks[address.ID]; !exists {
		return storage.NewNotFoundError(storage.BackendTypeIPFS, address)
	}

	m.pinnedBlocks[address.ID] = true
	m.operationCount["pin"]++

	return nil
}

// Unpin removes pinning from a block
func (m *MockIPFSClient) Unpin(ctx context.Context, address *storage.BlockAddress) error {
	start := time.Now()
	defer func() {
		m.recordOperation("unpin", address.ID, nil, start, nil)
	}()

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.simulateOperation("unpin"); err != nil {
		return err
	}

	delete(m.pinnedBlocks, address.ID)
	m.operationCount["unpin"]++

	return nil
}

// PeerAwareBackend interface implementation

// GetWithPeerHint retrieves a block with preferred peers
func (m *MockIPFSClient) GetWithPeerHint(ctx context.Context, address *storage.BlockAddress, peers []string) (*blocks.Block, error) {
	start := time.Now()
	defer func() {
		m.recordOperation("get_with_peer_hint", address.ID, peers, start, nil)
	}()

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.simulateOperation("get_peer_hint"); err != nil {
		return nil, err
	}

	// Simulate peer preference by adding slight delay if preferred peers are offline
	validPeers := 0
	for _, peer := range peers {
		if m.connectedPeers[peer] {
			validPeers++
		}
	}

	if validPeers == 0 && len(peers) > 0 {
		// Simulate slower retrieval from non-preferred peers
		time.Sleep(m.latency * 2)
	}

	block, exists := m.blocks[address.ID]
	if !exists {
		return nil, storage.NewNotFoundError(storage.BackendTypeIPFS, address)
	}

	m.operationCount["get_peer_hint"]++
	return block, nil
}

// BroadcastToNetwork broadcasts a block to the network
func (m *MockIPFSClient) BroadcastToNetwork(ctx context.Context, address *storage.BlockAddress, block *blocks.Block) error {
	start := time.Now()
	defer func() {
		m.recordOperation("broadcast", address.ID, m.getConnectedPeersList(), start, nil)
	}()

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.simulateOperation("broadcast"); err != nil {
		return err
	}

	// Simulate network broadcast delay
	time.Sleep(time.Duration(len(m.getConnectedPeersList())) * time.Millisecond * 10)

	m.operationCount["broadcast"]++
	return nil
}

// GetConnectedPeers returns list of connected peers
func (m *MockIPFSClient) GetConnectedPeers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.getConnectedPeersList()
}

// Backend metadata implementation

// GetBackendInfo returns backend information
func (m *MockIPFSClient) GetBackendInfo() *storage.BackendInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return &storage.BackendInfo{
		Name:    "mock-ipfs-client",
		Type:    storage.BackendTypeIPFS,
		Version: "mock-1.0.0",
		Capabilities: []string{
			storage.CapabilityPinning,
			storage.CapabilityPeerAware,
			storage.CapabilityBatch,
			storage.CapabilityContentAddress,
			storage.CapabilityDistributed,
		},
		Config: map[string]interface{}{
			"mock":            true,
			"bandwidth_limit": m.bandwidthLimit,
			"storage_quota":   m.storageQuota,
			"failure_rate":    m.failureRate,
		},
		NetworkID: "mock-ipfs-network",
		Peers:     m.getConnectedPeersList(),
	}
}

// HealthCheck returns current health status
func (m *MockIPFSClient) HealthCheck(ctx context.Context) *storage.HealthStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	healthy := m.isConnected && len(m.getConnectedPeersList()) > 0
	status := "healthy"
	if !m.isConnected {
		status = "offline"
	}

	return &storage.HealthStatus{
		Healthy:   healthy,
		Status:    status,
		LastCheck: time.Now(),
	}
}

// Connection management

// Connect simulates connecting to IPFS
func (m *MockIPFSClient) Connect(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.errorMode["connect"] != nil {
		return m.errorMode["connect"]
	}

	m.isConnected = true
	// Simulate connection establishment delay
	time.Sleep(time.Millisecond * 100)

	return nil
}

// Disconnect simulates disconnecting from IPFS
func (m *MockIPFSClient) Disconnect(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.isConnected = false
	return nil
}

// IsConnected returns connection status
func (m *MockIPFSClient) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isConnected
}

// Test control methods

// SetErrorMode configures error simulation for specific operations
func (m *MockIPFSClient) SetErrorMode(operation string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errorMode[operation] = err
}

// ClearErrorMode removes error simulation for an operation
func (m *MockIPFSClient) ClearErrorMode(operation string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.errorMode, operation)
}

// SetLatency configures artificial latency
func (m *MockIPFSClient) SetLatency(latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latency = latency
}

// SetOperationDelay configures delay for specific operations
func (m *MockIPFSClient) SetOperationDelay(operation string, delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.operationDelay[operation] = delay
}

// SetFailureRate configures random failure rate (0.0 to 1.0)
func (m *MockIPFSClient) SetFailureRate(rate float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failureRate = rate
}

// SetStorageQuota configures storage limits
func (m *MockIPFSClient) SetStorageQuota(quota int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.storageQuota = quota
}

// SetBandwidthLimit configures bandwidth limits
func (m *MockIPFSClient) SetBandwidthLimit(limit int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bandwidthLimit = limit
}

// Peer management for testing

// AddPeer adds a simulated peer
func (m *MockIPFSClient) AddPeer(peerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.peers = append(m.peers, peerID)
	m.connectedPeers[peerID] = true
}

// RemovePeer removes a simulated peer
func (m *MockIPFSClient) RemovePeer(peerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, peer := range m.peers {
		if peer == peerID {
			m.peers = append(m.peers[:i], m.peers[i+1:]...)
			break
		}
	}
	delete(m.connectedPeers, peerID)
}

// SetPeerConnectionState controls peer connectivity
func (m *MockIPFSClient) SetPeerConnectionState(peerID string, connected bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connectedPeers[peerID] = connected
}

// SetNetworkHealth configures network health status
func (m *MockIPFSClient) SetNetworkHealth(health string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.networkHealth = health
}

// Metrics and debugging

// GetOperationCounts returns operation statistics
func (m *MockIPFSClient) GetOperationCounts() map[string]int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	counts := make(map[string]int64)
	for op, count := range m.operationCount {
		counts[op] = count
	}
	return counts
}

// GetOperationHistory returns detailed operation history
func (m *MockIPFSClient) GetOperationHistory() []OperationRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()

	history := make([]OperationRecord, len(m.operationHistory))
	copy(history, m.operationHistory)
	return history
}

// ClearHistory clears operation history
func (m *MockIPFSClient) ClearHistory() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.operationHistory = make([]OperationRecord, 0)
}

// GetStorageStats returns current storage statistics
func (m *MockIPFSClient) GetStorageStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"blocks_stored":     len(m.blocks),
		"blocks_pinned":     len(m.pinnedBlocks),
		"current_storage":   m.currentStorage,
		"storage_quota":     m.storageQuota,
		"storage_usage_pct": float64(m.currentStorage) / float64(m.storageQuota) * 100,
		"connected_peers":   len(m.getConnectedPeersList()),
		"total_peers":       len(m.peers),
	}
}

// Reset clears all data and resets to initial state
func (m *MockIPFSClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.blocks = make(map[string]*blocks.Block)
	m.pinnedBlocks = make(map[string]bool)
	m.operationCount = make(map[string]int64)
	m.operationHistory = make([]OperationRecord, 0)
	m.currentStorage = 0
	m.failureCount = 0
	m.totalOperations = 0
}

// Private helper methods

func (m *MockIPFSClient) getConnectedPeersList() []string {
	var connected []string
	for peer, isConnected := range m.connectedPeers {
		if isConnected {
			connected = append(connected, peer)
		}
	}
	return connected
}

func (m *MockIPFSClient) simulateOperation(operation string) error {
	// Check connection
	if !m.isConnected {
		return storage.NewConnectionError(storage.BackendTypeIPFS, fmt.Errorf("client not connected"))
	}

	// Simulate latency
	if m.latency > 0 {
		time.Sleep(m.latency)
	}

	// Simulate operation-specific delay
	if delay, exists := m.operationDelay[operation]; exists {
		time.Sleep(delay)
	}

	// Check for configured errors
	if err, exists := m.errorMode[operation]; exists {
		return err
	}

	// Simulate random failures
	m.totalOperations++
	if m.failureRate > 0 && float64(m.failureCount)/float64(m.totalOperations) < m.failureRate {
		m.failureCount++
		return storage.NewStorageError("SIMULATED_FAILURE", "simulated operation failure", storage.BackendTypeIPFS, nil)
	}

	return nil
}

func (m *MockIPFSClient) recordOperation(operation, blockID string, peerHints []string, start time.Time, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	record := OperationRecord{
		Timestamp: start,
		Operation: operation,
		BlockID:   blockID,
		PeerHints: peerHints,
		Success:   err == nil,
		Duration:  time.Since(start),
		Metadata:  make(map[string]interface{}),
	}

	if err != nil {
		record.Error = err.Error()
	}

	// Add operation-specific metadata
	record.Metadata["connected_peers"] = len(m.getConnectedPeersList())
	record.Metadata["network_health"] = m.networkHealth

	m.operationHistory = append(m.operationHistory, record)

	// Limit history size to prevent memory issues in long-running tests
	if len(m.operationHistory) > 1000 {
		m.operationHistory = m.operationHistory[100:] // Keep most recent 900 records
	}
}

func (m *MockIPFSClient) calculateErrorRate() float64 {
	if m.totalOperations == 0 {
		return 0.0
	}
	return float64(m.failureCount) / float64(m.totalOperations)
}

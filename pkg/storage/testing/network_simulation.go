package testing

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

// NetworkSimulator provides realistic network behavior simulation
type NetworkSimulator struct {
	mu sync.RWMutex

	// Network topology
	peers       map[string]*SimulatedPeer
	connections map[string]map[string]*Connection

	// Network conditions
	baseLatency       time.Duration
	latencyVariance   time.Duration
	packetLossRate    float64
	bandwidthLimit    int64 // bytes per second
	networkPartitions map[string]bool

	// DHT simulation
	dhtEnabled   bool
	dhtNodes     map[string]*DHTNode
	routingTable map[string][]string

	// Gossip protocol simulation
	gossipEnabled  bool
	gossipFanout   int
	gossipInterval time.Duration
	gossipMessages []GossipMessage

	// Events and monitoring
	eventHistory   []NetworkEvent
	eventCallbacks map[string]func(NetworkEvent)

	// Advanced features
	byzantine      bool // Enable byzantine fault simulation
	byzantineRatio float64
	churnRate      float64 // Peer join/leave rate
	lastChurnTime  time.Time
}

// SimulatedPeer represents a peer in the network
type SimulatedPeer struct {
	ID           string
	IP           string
	Port         int
	Online       bool
	Capabilities []string
	StoredBlocks map[string]*blocks.Block
	Latency      time.Duration
	Bandwidth    int64
	Reliability  float64 // 0.0 to 1.0
	LastSeen     time.Time

	// Behavior simulation
	Byzantine     bool
	ResponseDelay time.Duration
	ErrorRate     float64

	// Resource constraints
	StorageLimit int64
	StorageUsed  int64
	CPULoad      float64
	MemoryUsage  float64
}

// Connection represents a connection between two peers
type Connection struct {
	From         string
	To           string
	Latency      time.Duration
	Bandwidth    int64
	PacketLoss   float64
	Established  time.Time
	LastActivity time.Time
	Quality      string // "excellent", "good", "poor", "unstable"
}

// DHTNode represents a node in the DHT simulation
type DHTNode struct {
	PeerID       string
	RoutingTable map[string][]string
	StoredKeys   map[string]string // key -> peer mapping
	Queries      int64
	Responses    int64
}

// GossipMessage represents a gossip protocol message
type GossipMessage struct {
	ID        string
	Type      string
	Payload   interface{}
	Origin    string
	TTL       int
	Timestamp time.Time
	Path      []string
}

// NetworkEvent represents network activity
type NetworkEvent struct {
	Timestamp   time.Time
	Type        string
	Source      string
	Destination string
	Details     map[string]interface{}
	Success     bool
	Error       string
}

// NewNetworkSimulator creates a new network simulator
func NewNetworkSimulator() *NetworkSimulator {
	return &NetworkSimulator{
		peers:             make(map[string]*SimulatedPeer),
		connections:       make(map[string]map[string]*Connection),
		networkPartitions: make(map[string]bool),
		dhtNodes:          make(map[string]*DHTNode),
		routingTable:      make(map[string][]string),
		gossipMessages:    make([]GossipMessage, 0),
		eventHistory:      make([]NetworkEvent, 0),
		eventCallbacks:    make(map[string]func(NetworkEvent)),

		// Default network conditions
		baseLatency:     time.Millisecond * 50,
		latencyVariance: time.Millisecond * 20,
		packetLossRate:  0.01,    // 1%
		bandwidthLimit:  1000000, // 1MB/s

		// DHT settings
		dhtEnabled: true,

		// Gossip settings
		gossipEnabled:  true,
		gossipFanout:   6,
		gossipInterval: time.Second * 30,

		// Byzantine settings
		byzantine:      false,
		byzantineRatio: 0.1,  // 10% byzantine nodes
		churnRate:      0.05, // 5% churn rate
	}
}

// Peer Management

// AddPeer adds a new peer to the network
func (n *NetworkSimulator) AddPeer(peerID string) *SimulatedPeer {
	n.mu.Lock()
	defer n.mu.Unlock()

	peer := &SimulatedPeer{
		ID:           peerID,
		IP:           fmt.Sprintf("192.168.%d.%d", rand.Intn(255), rand.Intn(255)),
		Port:         4001 + rand.Intn(1000),
		Online:       true,
		Capabilities: []string{"bitswap", "dht", "pubsub"},
		StoredBlocks: make(map[string]*blocks.Block),
		Latency:      n.generateRandomLatency(),
		Bandwidth:    n.bandwidthLimit,
		Reliability:  0.95 + rand.Float64()*0.05, // 95-100%
		LastSeen:     time.Now(),
		StorageLimit: 1000000000,            // 1GB
		ErrorRate:    rand.Float64() * 0.02, // 0-2%
	}

	// Byzantine behavior simulation
	if n.byzantine && rand.Float64() < n.byzantineRatio {
		peer.Byzantine = true
		peer.Reliability = 0.3 + rand.Float64()*0.4 // 30-70%
		peer.ErrorRate = 0.1 + rand.Float64()*0.2   // 10-30%
	}

	n.peers[peerID] = peer
	n.connections[peerID] = make(map[string]*Connection)

	// Initialize DHT node
	if n.dhtEnabled {
		n.dhtNodes[peerID] = &DHTNode{
			PeerID:       peerID,
			RoutingTable: make(map[string][]string),
			StoredKeys:   make(map[string]string),
		}
	}

	n.recordEvent("peer_join", peerID, "", map[string]interface{}{
		"peer_ip":   peer.IP,
		"peer_port": peer.Port,
		"byzantine": peer.Byzantine,
	}, true, "")

	return peer
}

// RemovePeer removes a peer from the network
func (n *NetworkSimulator) RemovePeer(peerID string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if peer, exists := n.peers[peerID]; exists {
		peer.Online = false
		delete(n.peers, peerID)
		delete(n.connections, peerID)
		delete(n.dhtNodes, peerID)

		// Remove connections to this peer
		for otherPeerID := range n.connections {
			delete(n.connections[otherPeerID], peerID)
		}

		n.recordEvent("peer_leave", peerID, "", map[string]interface{}{
			"reason": "removed",
		}, true, "")
	}
}

// SetPeerOnline controls peer availability
func (n *NetworkSimulator) SetPeerOnline(peerID string, online bool) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if peer, exists := n.peers[peerID]; exists {
		peer.Online = online
		peer.LastSeen = time.Now()

		eventType := "peer_online"
		if !online {
			eventType = "peer_offline"
		}

		n.recordEvent(eventType, peerID, "", map[string]interface{}{
			"online": online,
		}, true, "")
	}
}

// Connection Management

// EstablishConnection creates a connection between two peers
func (n *NetworkSimulator) EstablishConnection(fromPeerID, toPeerID string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if !n.isPeerOnline(fromPeerID) || !n.isPeerOnline(toPeerID) {
		return fmt.Errorf("one or both peers are offline")
	}

	if n.networkPartitions[fromPeerID] || n.networkPartitions[toPeerID] {
		return fmt.Errorf("peers are in different network partitions")
	}

	connection := &Connection{
		From:         fromPeerID,
		To:           toPeerID,
		Latency:      n.generateRandomLatency(),
		Bandwidth:    n.bandwidthLimit,
		PacketLoss:   n.packetLossRate,
		Established:  time.Now(),
		LastActivity: time.Now(),
		Quality:      n.determineConnectionQuality(),
	}

	if n.connections[fromPeerID] == nil {
		n.connections[fromPeerID] = make(map[string]*Connection)
	}
	n.connections[fromPeerID][toPeerID] = connection

	n.recordEvent("connection_established", fromPeerID, toPeerID, map[string]interface{}{
		"latency":     connection.Latency,
		"quality":     connection.Quality,
		"packet_loss": connection.PacketLoss,
	}, true, "")

	return nil
}

// DropConnection removes a connection between peers
func (n *NetworkSimulator) DropConnection(fromPeerID, toPeerID string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if connections, exists := n.connections[fromPeerID]; exists {
		delete(connections, toPeerID)
	}

	n.recordEvent("connection_dropped", fromPeerID, toPeerID, map[string]interface{}{
		"reason": "manual",
	}, true, "")
}

// Network Conditions

// SetNetworkLatency configures base network latency
func (n *NetworkSimulator) SetNetworkLatency(base, variance time.Duration) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.baseLatency = base
	n.latencyVariance = variance
}

// SetPacketLossRate configures packet loss simulation
func (n *NetworkSimulator) SetPacketLossRate(rate float64) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.packetLossRate = rate
}

// SetBandwidthLimit configures bandwidth constraints
func (n *NetworkSimulator) SetBandwidthLimit(limit int64) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.bandwidthLimit = limit
}

// CreateNetworkPartition simulates network partitions
func (n *NetworkSimulator) CreateNetworkPartition(peerIDs []string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	for _, peerID := range peerIDs {
		n.networkPartitions[peerID] = true
	}

	n.recordEvent("network_partition", "", "", map[string]interface{}{
		"affected_peers": peerIDs,
		"partition_size": len(peerIDs),
	}, true, "")
}

// HealNetworkPartition removes network partition
func (n *NetworkSimulator) HealNetworkPartition() {
	n.mu.Lock()
	defer n.mu.Unlock()

	partitionedPeers := make([]string, 0)
	for peerID := range n.networkPartitions {
		partitionedPeers = append(partitionedPeers, peerID)
	}

	n.networkPartitions = make(map[string]bool)

	n.recordEvent("partition_healed", "", "", map[string]interface{}{
		"restored_peers": partitionedPeers,
	}, true, "")
}

// Block Operations with Network Simulation

// SimulateBlockRetrieval simulates retrieving a block from the network
func (n *NetworkSimulator) SimulateBlockRetrieval(ctx context.Context, blockID string, requesterID string, peerHints []string) (*blocks.Block, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Find peers with the block
	candidatePeers := n.findPeersWithBlock(blockID, peerHints)
	if len(candidatePeers) == 0 {
		return nil, storage.NewNotFoundError(storage.BackendTypeIPFS, &storage.BlockAddress{ID: blockID})
	}

	// Select best peer based on network conditions
	selectedPeer := n.selectBestPeer(requesterID, candidatePeers)

	// Simulate network retrieval
	return n.simulateBlockTransfer(ctx, blockID, selectedPeer, requesterID)
}

// SimulateBlockBroadcast simulates broadcasting a block to the network
func (n *NetworkSimulator) SimulateBlockBroadcast(ctx context.Context, blockID string, block *blocks.Block, broadcasterID string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	connectedPeers := n.getConnectedPeers(broadcasterID)
	successCount := 0

	for _, peerID := range connectedPeers {
		if n.simulatePacketLoss() {
			continue // Packet lost
		}

		// Simulate propagation delay
		time.Sleep(n.generateRandomLatency() / 10) // Scale down for simulation

		if peer, exists := n.peers[peerID]; exists && peer.Online {
			peer.StoredBlocks[blockID] = block
			successCount++
		}
	}

	n.recordEvent("block_broadcast", broadcasterID, "", map[string]interface{}{
		"block_id":         blockID,
		"target_peers":     len(connectedPeers),
		"successful_sends": successCount,
		"success_rate":     float64(successCount) / float64(len(connectedPeers)),
	}, successCount > 0, "")

	if successCount == 0 {
		return fmt.Errorf("broadcast failed - no peers reachable")
	}

	return nil
}

// DHT Simulation

// EnableDHT enables DHT simulation
func (n *NetworkSimulator) EnableDHT(enabled bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.dhtEnabled = enabled
}

// SimulateDHTLookup simulates DHT key lookup
func (n *NetworkSimulator) SimulateDHTLookup(key string, requesterID string) ([]string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if !n.dhtEnabled {
		return nil, fmt.Errorf("DHT not enabled")
	}

	// Simulate DHT routing
	hops := 0
	maxHops := 8
	currentPeer := requesterID
	visited := make(map[string]bool)

	for hops < maxHops {
		if visited[currentPeer] {
			break // Routing loop detected
		}
		visited[currentPeer] = true

		dhtNode, exists := n.dhtNodes[currentPeer]
		if !exists {
			break
		}

		// Check if this node has the key
		if providerPeer, hasKey := dhtNode.StoredKeys[key]; hasKey {
			return []string{providerPeer}, nil
		}

		// Find next hop
		nextPeers, hasNextHop := n.routingTable[currentPeer]
		if !hasNextHop || len(nextPeers) == 0 {
			break
		}

		// Select random next hop
		currentPeer = nextPeers[rand.Intn(len(nextPeers))]
		hops++
	}

	return nil, fmt.Errorf("DHT lookup failed after %d hops", hops)
}

// Gossip Protocol Simulation

// EnableGossip enables gossip protocol simulation
func (n *NetworkSimulator) EnableGossip(enabled bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.gossipEnabled = enabled
}

// SimulateGossipBroadcast simulates gossip message propagation
func (n *NetworkSimulator) SimulateGossipBroadcast(message GossipMessage, originPeer string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if !n.gossipEnabled {
		return
	}

	message.Origin = originPeer
	message.Timestamp = time.Now()
	message.TTL = 10 // Default TTL
	message.Path = []string{originPeer}

	n.gossipMessages = append(n.gossipMessages, message)
	n.propagateGossipMessage(message)
}

// Byzantine Behavior Simulation

// EnableByzantine enables byzantine fault simulation
func (n *NetworkSimulator) EnableByzantine(enabled bool, ratio float64) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.byzantine = enabled
	n.byzantineRatio = ratio
}

// Monitoring and Events

// RegisterEventCallback registers a callback for network events
func (n *NetworkSimulator) RegisterEventCallback(eventType string, callback func(NetworkEvent)) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.eventCallbacks[eventType] = callback
}

// GetNetworkStats returns current network statistics
func (n *NetworkSimulator) GetNetworkStats() map[string]interface{} {
	n.mu.RLock()
	defer n.mu.RUnlock()

	onlinePeers := 0
	totalConnections := 0
	byzantinePeers := 0

	for _, peer := range n.peers {
		if peer.Online {
			onlinePeers++
		}
		if peer.Byzantine {
			byzantinePeers++
		}
	}

	for _, connections := range n.connections {
		totalConnections += len(connections)
	}

	return map[string]interface{}{
		"total_peers":        len(n.peers),
		"online_peers":       onlinePeers,
		"total_connections":  totalConnections,
		"byzantine_peers":    byzantinePeers,
		"network_partitions": len(n.networkPartitions),
		"base_latency":       n.baseLatency,
		"packet_loss_rate":   n.packetLossRate,
		"bandwidth_limit":    n.bandwidthLimit,
		"dht_enabled":        n.dhtEnabled,
		"gossip_enabled":     n.gossipEnabled,
		"event_count":        len(n.eventHistory),
	}
}

// GetEventHistory returns network event history
func (n *NetworkSimulator) GetEventHistory() []NetworkEvent {
	n.mu.RLock()
	defer n.mu.RUnlock()

	history := make([]NetworkEvent, len(n.eventHistory))
	copy(history, n.eventHistory)
	return history
}

// ClearEventHistory clears the event history
func (n *NetworkSimulator) ClearEventHistory() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.eventHistory = make([]NetworkEvent, 0)
}

// SyncBlockFromClient adds a block from MockIPFSClient to all online peers
func (n *NetworkSimulator) SyncBlockFromClient(blockID string, block *blocks.Block) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Add block to all online peers for realism
	for _, peer := range n.peers {
		if peer.Online {
			peer.StoredBlocks[blockID] = block
		}
	}

	n.recordEvent("block_sync", "mock_client", "", map[string]interface{}{
		"block_id":     blockID,
		"synced_peers": len(n.peers),
	}, true, "")
}

// Reset resets the network simulation to initial state
func (n *NetworkSimulator) Reset() {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.peers = make(map[string]*SimulatedPeer)
	n.connections = make(map[string]map[string]*Connection)
	n.networkPartitions = make(map[string]bool)
	n.dhtNodes = make(map[string]*DHTNode)
	n.routingTable = make(map[string][]string)
	n.gossipMessages = make([]GossipMessage, 0)
	n.eventHistory = make([]NetworkEvent, 0)
}

// Private helper methods

func (n *NetworkSimulator) isPeerOnline(peerID string) bool {
	if peer, exists := n.peers[peerID]; exists {
		return peer.Online
	}
	return false
}

func (n *NetworkSimulator) generateRandomLatency() time.Duration {
	variance := time.Duration(rand.Int63n(int64(n.latencyVariance)))
	if rand.Float32() < 0.5 {
		variance = -variance
	}
	return n.baseLatency + variance
}

func (n *NetworkSimulator) determineConnectionQuality() string {
	rand := rand.Float64()
	switch {
	case rand < 0.7:
		return "excellent"
	case rand < 0.9:
		return "good"
	case rand < 0.98:
		return "poor"
	default:
		return "unstable"
	}
}

func (n *NetworkSimulator) simulatePacketLoss() bool {
	return rand.Float64() < n.packetLossRate
}

func (n *NetworkSimulator) findPeersWithBlock(blockID string, peerHints []string) []string {
	var candidates []string

	// Check hinted peers first
	for _, peerID := range peerHints {
		if peer, exists := n.peers[peerID]; exists && peer.Online {
			if _, hasBlock := peer.StoredBlocks[blockID]; hasBlock {
				candidates = append(candidates, peerID)
			}
		}
	}

	// If no hints or hints don't have the block, check all peers
	if len(candidates) == 0 {
		for peerID, peer := range n.peers {
			if peer.Online {
				if _, hasBlock := peer.StoredBlocks[blockID]; hasBlock {
					candidates = append(candidates, peerID)
				}
			}
		}
	}

	return candidates
}

func (n *NetworkSimulator) selectBestPeer(requesterID string, candidates []string) string {
	if len(candidates) == 0 {
		return ""
	}

	bestPeer := candidates[0]
	bestScore := 0.0

	for _, peerID := range candidates {
		score := n.calculatePeerScore(requesterID, peerID)
		if score > bestScore {
			bestScore = score
			bestPeer = peerID
		}
	}

	return bestPeer
}

func (n *NetworkSimulator) calculatePeerScore(requesterID, candidateID string) float64 {
	peer, exists := n.peers[candidateID]
	if !exists {
		return 0.0
	}

	score := peer.Reliability

	// Factor in connection quality
	if connections, hasConns := n.connections[requesterID]; hasConns {
		if conn, hasConn := connections[candidateID]; hasConn {
			switch conn.Quality {
			case "excellent":
				score += 0.3
			case "good":
				score += 0.1
			case "poor":
				score -= 0.1
			case "unstable":
				score -= 0.3
			}
		}
	}

	// Factor in latency
	latencyPenalty := float64(peer.Latency) / float64(time.Second)
	score -= latencyPenalty * 0.1

	return score
}

func (n *NetworkSimulator) simulateBlockTransfer(ctx context.Context, blockID, fromPeer, toPeer string) (*blocks.Block, error) {
	peer, exists := n.peers[fromPeer]
	if !exists {
		return nil, fmt.Errorf("peer %s not found", fromPeer)
	}

	block, hasBlock := peer.StoredBlocks[blockID]
	if !hasBlock {
		return nil, storage.NewNotFoundError(storage.BackendTypeIPFS, &storage.BlockAddress{ID: blockID})
	}

	// Simulate transfer time based on block size and bandwidth
	transferTime := time.Duration(len(block.Data)) * time.Second / time.Duration(peer.Bandwidth)
	time.Sleep(transferTime / 1000) // Scale down for simulation

	// Simulate network conditions
	if n.simulatePacketLoss() {
		return nil, fmt.Errorf("transfer failed due to packet loss")
	}

	// Byzantine behavior
	if peer.Byzantine && rand.Float64() < 0.3 {
		return nil, fmt.Errorf("byzantine peer returned corrupted data")
	}

	n.recordEvent("block_transfer", fromPeer, toPeer, map[string]interface{}{
		"block_id":      blockID,
		"block_size":    len(block.Data),
		"transfer_time": transferTime,
	}, true, "")

	return block, nil
}

func (n *NetworkSimulator) getConnectedPeers(peerID string) []string {
	var connected []string
	if connections, exists := n.connections[peerID]; exists {
		for otherPeerID := range connections {
			if n.isPeerOnline(otherPeerID) {
				connected = append(connected, otherPeerID)
			}
		}
	}
	return connected
}

func (n *NetworkSimulator) propagateGossipMessage(message GossipMessage) {
	if message.TTL <= 0 {
		return
	}

	originConnections := n.connections[message.Origin]
	fanoutPeers := make([]string, 0)

	// Select random peers for fanout
	for peerID := range originConnections {
		if len(fanoutPeers) >= n.gossipFanout {
			break
		}
		if n.isPeerOnline(peerID) && !n.peerInPath(peerID, message.Path) {
			fanoutPeers = append(fanoutPeers, peerID)
		}
	}

	// Propagate to selected peers
	for _, peerID := range fanoutPeers {
		newMessage := message
		newMessage.TTL--
		newMessage.Path = append(newMessage.Path, peerID)

		// Continue propagation from this peer
		go func(msg GossipMessage, peer string) {
			time.Sleep(n.generateRandomLatency() / 10)
			n.propagateGossipMessage(msg)
		}(newMessage, peerID)
	}
}

func (n *NetworkSimulator) peerInPath(peerID string, path []string) bool {
	for _, p := range path {
		if p == peerID {
			return true
		}
	}
	return false
}

func (n *NetworkSimulator) recordEvent(eventType, source, destination string, details map[string]interface{}, success bool, errorMsg string) {
	event := NetworkEvent{
		Timestamp:   time.Now(),
		Type:        eventType,
		Source:      source,
		Destination: destination,
		Details:     details,
		Success:     success,
		Error:       errorMsg,
	}

	n.eventHistory = append(n.eventHistory, event)

	// Limit history size
	if len(n.eventHistory) > 5000 {
		n.eventHistory = n.eventHistory[1000:] // Keep most recent 4000 events
	}

	// Call registered callbacks
	if callback, exists := n.eventCallbacks[eventType]; exists {
		go callback(event)
	}
}

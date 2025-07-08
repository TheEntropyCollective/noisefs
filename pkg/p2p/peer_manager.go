package p2p

import (
	"context"
	"crypto/rand"
	"fmt"
	"math"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
)

// PeerInfo tracks comprehensive metadata about a peer
type PeerInfo struct {
	ID              peer.ID           `json:"id"`
	LastSeen        time.Time         `json:"last_seen"`
	Latency         time.Duration     `json:"latency"`
	Bandwidth       float64           `json:"bandwidth_mbps"`
	SuccessRate     float64           `json:"success_rate"`
	BlockInventory  *BloomFilter      `json:"-"`
	RandomizerScore float64           `json:"randomizer_score"`
	Reputation      float64           `json:"reputation"`
	ConnectedAt     time.Time         `json:"connected_at"`
	
	// Performance tracking
	TotalRequests   int64             `json:"total_requests"`
	SuccessRequests int64             `json:"success_requests"`
	TotalBytes      int64             `json:"total_bytes"`
	TotalTime       time.Duration     `json:"total_time"`
	
	// Connection state
	IsConnected     bool              `json:"is_connected"`
	ConnectionCount int               `json:"connection_count"`
	
	// Lock for thread-safe updates
	mu              sync.RWMutex
}

// UpdateMetrics updates peer performance metrics
func (p *PeerInfo) UpdateMetrics(success bool, latency time.Duration, bytes int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.TotalRequests++
	if success {
		p.SuccessRequests++
	}
	p.TotalBytes += bytes
	p.TotalTime += latency
	p.LastSeen = time.Now()
	
	// Calculate rolling averages
	if p.TotalRequests > 0 {
		p.SuccessRate = float64(p.SuccessRequests) / float64(p.TotalRequests)
	}
	
	if p.TotalTime > 0 && p.TotalRequests > 0 {
		avgLatency := p.TotalTime / time.Duration(p.TotalRequests)
		// Use exponential moving average for latency
		alpha := 0.1
		p.Latency = time.Duration(float64(p.Latency)*(1-alpha) + float64(avgLatency)*alpha)
	}
	
	if p.TotalTime > 0 && p.TotalBytes > 0 {
		// Calculate bandwidth in MB/s
		seconds := p.TotalTime.Seconds()
		p.Bandwidth = float64(p.TotalBytes) / (1024 * 1024) / seconds
	}
}

// GetPerformanceScore calculates a composite performance score
func (p *PeerInfo) GetPerformanceScore() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	if p.TotalRequests < 5 {
		return 0.5 // Neutral score for new peers
	}
	
	// Latency score (lower is better)
	latencyScore := 1.0 / (1.0 + p.Latency.Seconds())
	
	// Bandwidth score (higher is better, normalize to 10MB/s)
	bandwidthScore := math.Min(p.Bandwidth/10.0, 1.0)
	
	// Reliability score
	reliabilityScore := p.SuccessRate
	
	// Composite score with weights
	return latencyScore*0.4 + bandwidthScore*0.3 + reliabilityScore*0.3
}

// IsHealthy checks if peer is considered healthy
func (p *PeerInfo) IsHealthy() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	// Consider unhealthy if:
	// - Not seen in last 5 minutes
	// - Success rate below 50%
	// - Latency above 10 seconds
	return time.Since(p.LastSeen) < 5*time.Minute &&
		p.SuccessRate >= 0.5 &&
		p.Latency < 10*time.Second
}

// PeerSelectionStrategy defines different peer selection algorithms
type PeerSelectionStrategy interface {
	SelectPeers(ctx context.Context, criteria SelectionCriteria) ([]peer.ID, error)
	UpdateMetrics(peerID peer.ID, success bool, latency time.Duration, bytes int64)
	GetPeerInfo(peerID peer.ID) (*PeerInfo, bool)
}

// SelectionCriteria defines parameters for peer selection
type SelectionCriteria struct {
	Count               int           `json:"count"`
	MinBandwidth        float64       `json:"min_bandwidth_mbps"`
	MaxLatency          time.Duration `json:"max_latency"`
	RequiredBlocks      []string      `json:"required_blocks"`
	ExcludePeers        []peer.ID     `json:"exclude_peers"`
	PreferRandomizers   bool          `json:"prefer_randomizers"`
	RequirePrivacy      bool          `json:"require_privacy"`
	LoadBalancing       bool          `json:"load_balancing"`
}

// PeerManager manages peer discovery, selection, and performance tracking
type PeerManager struct {
	host               host.Host
	peers              map[peer.ID]*PeerInfo
	strategies         map[string]PeerSelectionStrategy
	defaultStrategy    string
	
	// Configuration
	maxPeers           int
	healthCheckInterval time.Duration
	metricRetention     time.Duration
	
	// Synchronization
	mu                 sync.RWMutex
	ctx                context.Context
	cancel             context.CancelFunc
	
	// Metrics
	totalSelections    int64
	totalConnections   int64
	totalDisconnections int64
}

// NewPeerManager creates a new peer manager
func NewPeerManager(h host.Host, maxPeers int) *PeerManager {
	ctx, cancel := context.WithCancel(context.Background())
	
	pm := &PeerManager{
		host:                h,
		peers:               make(map[peer.ID]*PeerInfo),
		strategies:          make(map[string]PeerSelectionStrategy),
		defaultStrategy:     "performance",
		maxPeers:            maxPeers,
		healthCheckInterval: 30 * time.Second,
		metricRetention:     24 * time.Hour,
		ctx:                 ctx,
		cancel:              cancel,
	}
	
	// Register default strategies
	pm.RegisterStrategy("performance", NewPerformanceStrategy(pm))
	pm.RegisterStrategy("randomizer", NewRandomizerStrategy(pm))
	pm.RegisterStrategy("privacy", NewPrivacyStrategy(pm))
	pm.RegisterStrategy("hybrid", NewHybridStrategy(pm))
	
	// Start background tasks
	go pm.healthCheckLoop()
	go pm.metricCleanupLoop()
	
	// Set up connection event handlers
	h.Network().Notify(&network.NotifyBundle{
		ConnectedF:    pm.onPeerConnected,
		DisconnectedF: pm.onPeerDisconnected,
	})
	
	return pm
}

// RegisterStrategy registers a new peer selection strategy
func (pm *PeerManager) RegisterStrategy(name string, strategy PeerSelectionStrategy) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.strategies[name] = strategy
}

// SelectPeers selects peers using the specified strategy
func (pm *PeerManager) SelectPeers(ctx context.Context, strategy string, criteria SelectionCriteria) ([]peer.ID, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	if strategy == "" {
		strategy = pm.defaultStrategy
	}
	
	s, exists := pm.strategies[strategy]
	if !exists {
		return nil, fmt.Errorf("unknown selection strategy: %s", strategy)
	}
	
	pm.totalSelections++
	return s.SelectPeers(ctx, criteria)
}

// UpdatePeerMetrics updates performance metrics for a peer
func (pm *PeerManager) UpdatePeerMetrics(peerID peer.ID, success bool, latency time.Duration, bytes int64) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	peerInfo, exists := pm.peers[peerID]
	if !exists {
		peerInfo = &PeerInfo{
			ID:          peerID,
			ConnectedAt: time.Now(),
		}
		pm.peers[peerID] = peerInfo
	}
	
	peerInfo.UpdateMetrics(success, latency, bytes)
	
	// Update all strategies
	for _, strategy := range pm.strategies {
		strategy.UpdateMetrics(peerID, success, latency, bytes)
	}
}

// GetPeerInfo returns information about a specific peer
func (pm *PeerManager) GetPeerInfo(peerID peer.ID) (*PeerInfo, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	info, exists := pm.peers[peerID]
	if !exists {
		return nil, false
	}
	
	// Return a copy to avoid race conditions
	infoCopy := *info
	return &infoCopy, true
}

// GetHealthyPeers returns all currently healthy peers
func (pm *PeerManager) GetHealthyPeers() []peer.ID {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	var healthy []peer.ID
	for id, info := range pm.peers {
		if info.IsHealthy() && info.IsConnected {
			healthy = append(healthy, id)
		}
	}
	
	return healthy
}

// GetPeersByPerformance returns peers sorted by performance score
func (pm *PeerManager) GetPeersByPerformance(limit int) []peer.ID {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	type peerScore struct {
		ID    peer.ID
		Score float64
	}
	
	var scores []peerScore
	for id, info := range pm.peers {
		if info.IsHealthy() && info.IsConnected {
			scores = append(scores, peerScore{
				ID:    id,
				Score: info.GetPerformanceScore(),
			})
		}
	}
	
	// Sort by score (descending)
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})
	
	// Extract peer IDs
	var result []peer.ID
	for i, score := range scores {
		if i >= limit {
			break
		}
		result = append(result, score.ID)
	}
	
	return result
}

// GetRandomPeers returns a random selection of healthy peers
func (pm *PeerManager) GetRandomPeers(count int) []peer.ID {
	healthy := pm.GetHealthyPeers()
	if len(healthy) <= count {
		return healthy
	}
	
	// Fisher-Yates shuffle
	result := make([]peer.ID, len(healthy))
	copy(result, healthy)
	
	for i := len(result) - 1; i > 0; i-- {
		j, _ := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		result[i], result[int(j.Int64())] = result[int(j.Int64())], result[i]
	}
	
	return result[:count]
}

// GetStats returns statistics about the peer manager
func (pm *PeerManager) GetStats() map[string]interface{} {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	
	stats := map[string]interface{}{
		"total_peers":         len(pm.peers),
		"connected_peers":     0,
		"healthy_peers":       0,
		"total_selections":    pm.totalSelections,
		"total_connections":   pm.totalConnections,
		"total_disconnections": pm.totalDisconnections,
		"strategies":          make([]string, 0, len(pm.strategies)),
	}
	
	for _, info := range pm.peers {
		if info.IsConnected {
			stats["connected_peers"] = stats["connected_peers"].(int) + 1
		}
		if info.IsHealthy() {
			stats["healthy_peers"] = stats["healthy_peers"].(int) + 1
		}
	}
	
	for name := range pm.strategies {
		stats["strategies"] = append(stats["strategies"].([]string), name)
	}
	
	return stats
}

// onPeerConnected handles peer connection events
func (pm *PeerManager) onPeerConnected(n network.Network, conn network.Conn) {
	peerID := conn.RemotePeer()
	
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	pm.totalConnections++
	
	peerInfo, exists := pm.peers[peerID]
	if !exists {
		peerInfo = &PeerInfo{
			ID:          peerID,
			ConnectedAt: time.Now(),
		}
		pm.peers[peerID] = peerInfo
	}
	
	peerInfo.mu.Lock()
	peerInfo.IsConnected = true
	peerInfo.ConnectionCount++
	peerInfo.LastSeen = time.Now()
	peerInfo.mu.Unlock()
}

// onPeerDisconnected handles peer disconnection events
func (pm *PeerManager) onPeerDisconnected(n network.Network, conn network.Conn) {
	peerID := conn.RemotePeer()
	
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	pm.totalDisconnections++
	
	if peerInfo, exists := pm.peers[peerID]; exists {
		peerInfo.mu.Lock()
		peerInfo.IsConnected = false
		peerInfo.mu.Unlock()
	}
}

// healthCheckLoop periodically checks peer health
func (pm *PeerManager) healthCheckLoop() {
	ticker := time.NewTicker(pm.healthCheckInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			pm.performHealthCheck()
		case <-pm.ctx.Done():
			return
		}
	}
}

// performHealthCheck checks health of all peers
func (pm *PeerManager) performHealthCheck() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	for peerID, info := range pm.peers {
		if !info.IsHealthy() {
			// Disconnect unhealthy peers
			if info.IsConnected {
				pm.host.Network().ClosePeer(peerID)
			}
		}
	}
}

// metricCleanupLoop periodically cleans up old metrics
func (pm *PeerManager) metricCleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			pm.cleanupOldMetrics()
		case <-pm.ctx.Done():
			return
		}
	}
}

// cleanupOldMetrics removes metrics for peers not seen recently
func (pm *PeerManager) cleanupOldMetrics() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	
	cutoff := time.Now().Add(-pm.metricRetention)
	
	for peerID, info := range pm.peers {
		if info.LastSeen.Before(cutoff) && !info.IsConnected {
			delete(pm.peers, peerID)
		}
	}
}

// Close shuts down the peer manager
func (pm *PeerManager) Close() error {
	pm.cancel()
	return nil
}
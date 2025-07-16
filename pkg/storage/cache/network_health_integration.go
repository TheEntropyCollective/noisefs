package cache

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	shell "github.com/ipfs/go-ipfs-api"
)

// NetworkHealthManager integrates all network health components
type NetworkHealthManager struct {
	cache         *AltruisticCache
	healthTracker *BlockHealthTracker
	gossiper      *HealthGossiper
	exchanger     *BloomExchanger
	shell         *shell.Shell
	
	// Configuration
	config *NetworkHealthConfig
	
	// State
	started bool
	mu      sync.RWMutex
}

// NetworkHealthConfig configures the network health system
type NetworkHealthConfig struct {
	EnableGossip         bool                  `json:"enable_gossip"`
	EnableBloomExchange  bool                  `json:"enable_bloom_exchange"`
	GossipConfig         *HealthGossipConfig   `json:"gossip_config,omitempty"`
	BloomExchangeConfig  *BloomExchangeConfig  `json:"bloom_exchange_config,omitempty"`
}

// DefaultNetworkHealthConfig returns default configuration
func DefaultNetworkHealthConfig() *NetworkHealthConfig {
	return &NetworkHealthConfig{
		EnableGossip:        true,
		EnableBloomExchange: true,
		GossipConfig:        DefaultHealthGossipConfig(),
		BloomExchangeConfig: DefaultBloomExchangeConfig(),
	}
}

// NewNetworkHealthManager creates a new network health manager
func NewNetworkHealthManager(
	cache *AltruisticCache,
	shell *shell.Shell,
	config *NetworkHealthConfig,
) (*NetworkHealthManager, error) {
	if config == nil {
		config = DefaultNetworkHealthConfig()
	}
	
	nhm := &NetworkHealthManager{
		cache:         cache,
		healthTracker: cache.GetHealthTracker(),
		shell:         shell,
		config:        config,
	}
	
	// Initialize gossiper if enabled
	if config.EnableGossip {
		gossiper, err := NewHealthGossiper(config.GossipConfig, nhm.healthTracker, shell)
		if err != nil {
			return nil, fmt.Errorf("failed to create gossiper: %w", err)
		}
		nhm.gossiper = gossiper
	}
	
	// Initialize bloom exchanger if enabled
	if config.EnableBloomExchange {
		exchanger, err := NewBloomExchanger(config.BloomExchangeConfig, cache, shell)
		if err != nil {
			return nil, fmt.Errorf("failed to create bloom exchanger: %w", err)
		}
		nhm.exchanger = exchanger
	}
	
	return nhm, nil
}

// Start starts all network health components
func (nhm *NetworkHealthManager) Start() error {
	nhm.mu.Lock()
	defer nhm.mu.Unlock()
	
	if nhm.started {
		return fmt.Errorf("network health manager already started")
	}
	
	// Start gossiper
	if nhm.gossiper != nil {
		if err := nhm.gossiper.Start(); err != nil {
			return fmt.Errorf("failed to start gossiper: %w", err)
		}
	}
	
	// Start bloom exchanger
	if nhm.exchanger != nil {
		if err := nhm.exchanger.Start(); err != nil {
			// Stop gossiper if it was started
			if nhm.gossiper != nil {
				nhm.gossiper.Stop()
			}
			return fmt.Errorf("failed to start bloom exchanger: %w", err)
		}
	}
	
	nhm.started = true
	return nil
}

// Stop stops all network health components
func (nhm *NetworkHealthManager) Stop() {
	nhm.mu.Lock()
	defer nhm.mu.Unlock()
	
	if !nhm.started {
		return
	}
	
	// Stop components in reverse order
	if nhm.exchanger != nil {
		nhm.exchanger.Stop()
	}
	
	if nhm.gossiper != nil {
		nhm.gossiper.Stop()
	}
	
	nhm.started = false
}

// GetNetworkHealth returns comprehensive network health information
func (nhm *NetworkHealthManager) GetNetworkHealth() *NetworkHealthReport {
	nhm.mu.RLock()
	defer nhm.mu.RUnlock()
	
	report := &NetworkHealthReport{
		Timestamp: time.Now(),
		Enabled:   nhm.started,
	}
	
	// Get gossip statistics
	if nhm.gossiper != nil {
		report.GossipStats = nhm.gossiper.GetNetworkHealthEstimate()
	}
	
	// Get coordination statistics
	if nhm.exchanger != nil {
		report.CoordinationStats = nhm.exchanger.GetPeerCoordination()
	}
	
	// Get local health statistics
	report.LocalHealth = nhm.healthTracker.GetHealthSummary()
	
	// Calculate overall network health score
	report.OverallScore = nhm.calculateOverallScore(report)
	
	return report
}

// NetworkHealthReport represents comprehensive network health information
type NetworkHealthReport struct {
	Timestamp         time.Time
	Enabled           bool
	GossipStats       *NetworkHealthEstimate
	CoordinationStats *PeerCoordinationStats
	LocalHealth       *HealthSummary
	OverallScore      float64
}

// HealthSummary represents local health tracking summary
type HealthSummary struct {
	TrackedBlocks      int
	LowReplication     int
	HighEntropy        int
	AverageValue       float64
	OpportunisticQueue int
}

// GetHealthSummary returns a summary of local health tracking
func (bht *BlockHealthTracker) GetHealthSummary() *HealthSummary {
	bht.mu.RLock()
	defer bht.mu.RUnlock()
	
	summary := &HealthSummary{
		TrackedBlocks: len(bht.blocks),
	}
	
	totalValue := 0.0
	for _, health := range bht.blocks {
		hint := health.Hint
		if hint.ReplicationBucket == ReplicationLow {
			summary.LowReplication++
		}
		if hint.HighEntropy {
			summary.HighEntropy++
		}
		totalValue += bht.calculateBlockValueInternal(hint)
	}
	
	if summary.TrackedBlocks > 0 {
		summary.AverageValue = totalValue / float64(summary.TrackedBlocks)
	}
	
	// TODO: Integrate with opportunistic fetcher when available
	summary.OpportunisticQueue = 0
	
	return summary
}

// calculateOverallScore calculates an overall network health score
func (nhm *NetworkHealthManager) calculateOverallScore(report *NetworkHealthReport) float64 {
	score := 0.0
	components := 0
	
	// Local health component (40% weight)
	if report.LocalHealth != nil && report.LocalHealth.TrackedBlocks > 0 {
		// Normalize AverageValue to 0-1 range (assuming max value of ~10)
		localScore := math.Min(report.LocalHealth.AverageValue / 10.0, 1.0)
		score += localScore * 0.4
		components++
	}
	
	// Network gossip component (30% weight)
	if report.GossipStats != nil && report.GossipStats.PeerCount > 0 {
		// Score based on peer participation
		peerScore := float64(report.GossipStats.PeerCount) / 20.0 // Normalize to 20 peers
		if peerScore > 1.0 {
			peerScore = 1.0
		}
		score += peerScore * 0.3
		components++
	}
	
	// Coordination component (30% weight)
	if report.CoordinationStats != nil && report.CoordinationStats.ActivePeers > 0 {
		// Use average category overlap as coordination metric
		coordScore := 0.0
		for _, catStats := range report.CoordinationStats.Categories {
			// Optimal overlap is around 30-50%
			overlap := catStats.AverageOverlap
			if overlap > 0.3 && overlap < 0.5 {
				coordScore = 1.0
			} else if overlap < 0.3 {
				coordScore = overlap / 0.3
			} else {
				coordScore = (1.0 - overlap) / 0.5
			}
		}
		score += coordScore * 0.3
		components++
	}
	
	// Normalize by number of active components
	if components > 0 {
		return score
	}
	
	return 0.0
}

// UpdateFromNetworkHealth updates local decisions based on network health
func (nhm *NetworkHealthManager) UpdateFromNetworkHealth() {
	nhm.mu.RLock()
	defer nhm.mu.RUnlock()
	
	if !nhm.started {
		return
	}
	
	// Get network health estimate
	if nhm.gossiper != nil {
		networkEstimate := nhm.gossiper.GetNetworkHealthEstimate()
		if networkEstimate.PeerCount >= 3 {
			// Update opportunistic fetching based on network needs
			nhm.updateOpportunisticTargets(networkEstimate)
		}
	}
	
	// Update coordination
	if nhm.exchanger != nil {
		coordination := nhm.exchanger.GetPeerCoordination()
		if coordination.ActivePeers >= nhm.config.BloomExchangeConfig.MinPeersForCoordination {
			// Adjust eviction strategy based on coordination
			nhm.adjustEvictionStrategy(coordination)
		}
	}
}

// updateOpportunisticTargets updates blocks to fetch based on network needs
func (nhm *NetworkHealthManager) updateOpportunisticTargets(estimate *NetworkHealthEstimate) {
	fetcher := nhm.healthTracker.GetOpportunisticFetcher()
	if fetcher == nil {
		return
	}
	
	// If network has many low-replication blocks, prioritize fetching them
	if estimate.LowReplicationBlocks > estimate.TotalNetworkBlocks/10 {
		// Increase priority for low-replication blocks
		// This would be implemented in the opportunistic fetcher
	}
}

// adjustEvictionStrategy adjusts eviction based on coordination
func (nhm *NetworkHealthManager) adjustEvictionStrategy(coordination *PeerCoordinationStats) {
	// If coordination is good, we can be more aggressive with eviction
	// If coordination is poor, be more conservative
	for category, stats := range coordination.Categories {
		if category == "valuable_blocks" && stats.AverageOverlap < 0.2 {
			// Low overlap means peers aren't coordinating well
			// Be more conservative with evicting valuable blocks
			nhm.cache.SetEvictionStrategy("ValueBased")
			return
		}
	}
}

// GetNetworkHealthManager returns the network health manager for a cache
func (ac *AltruisticCache) GetNetworkHealthManager(shell *shell.Shell) (*NetworkHealthManager, error) {
	// This would typically be initialized once and stored
	// For now, create a new one
	config := &NetworkHealthConfig{
		EnableGossip:        ac.config.EnableAltruistic,
		EnableBloomExchange: ac.config.EnableAltruistic,
	}
	
	return NewNetworkHealthManager(ac, shell, config)
}

// IntegrateWithPeerDiscovery integrates with existing peer discovery
func (nhm *NetworkHealthManager) IntegrateWithPeerDiscovery(ctx context.Context) error {
	// This would integrate with the existing P2P peer manager
	// For now, we'll use a simple integration approach
	
	// Monitor for new peers and update health tracking
	go nhm.peerDiscoveryLoop(ctx)
	
	return nil
}

// peerDiscoveryLoop monitors peer discovery events
func (nhm *NetworkHealthManager) peerDiscoveryLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Update network health based on discovered peers
			nhm.UpdateFromNetworkHealth()
		}
	}
}
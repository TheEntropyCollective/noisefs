package cache

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"sync"
	"time"

	"github.com/bits-and-blooms/bloom/v3"
	shell "github.com/ipfs/go-ipfs-api"
)

// HealthGossipConfig configures the block health gossip protocol
type HealthGossipConfig struct {
	// Gossip interval for periodic broadcasts
	GossipInterval time.Duration `json:"gossip_interval"`
	
	// Maximum peers to gossip with per round
	MaxGossipPeers int `json:"max_gossip_peers"`
	
	// Bloom filter parameters
	BloomFilterSize       uint    `json:"bloom_filter_size"`
	BloomFilterHashFuncs  uint    `json:"bloom_filter_hash_funcs"`
	BloomFalsePositive    float64 `json:"bloom_false_positive"`
	
	// Privacy parameters
	EnableDifferentialPrivacy bool    `json:"enable_differential_privacy"`
	PrivacyEpsilon           float64 `json:"privacy_epsilon"`
	
	// Aggregation parameters
	AggregationWindow        time.Duration `json:"aggregation_window"`
	MinBlocksForGossip       int          `json:"min_blocks_for_gossip"`
}

// DefaultHealthGossipConfig returns default configuration
func DefaultHealthGossipConfig() *HealthGossipConfig {
	return &HealthGossipConfig{
		GossipInterval:           5 * time.Minute,
		MaxGossipPeers:          5,
		BloomFilterSize:         10000,
		BloomFilterHashFuncs:    5,
		BloomFalsePositive:      0.01,
		EnableDifferentialPrivacy: true,
		PrivacyEpsilon:          1.0,
		AggregationWindow:       15 * time.Minute,
		MinBlocksForGossip:      10,
	}
}

// HealthGossipMessage represents a gossip message about block health
type HealthGossipMessage struct {
	// Timestamp of the message
	Timestamp time.Time `json:"timestamp"`
	
	// Bloom filter of low-replication blocks
	LowReplicationFilter []byte `json:"low_replication_filter"`
	
	// Bloom filter of high-entropy blocks
	HighEntropyFilter []byte `json:"high_entropy_filter"`
	
	// Aggregate statistics (with noise for privacy)
	AggregateStats *AggregateHealthStats `json:"aggregate_stats"`
	
	// Peer ID (anonymized)
	PeerID string `json:"peer_id"`
	
	// Message version for compatibility
	Version int `json:"version"`
}

// AggregateHealthStats contains aggregated health statistics
type AggregateHealthStats struct {
	// Total blocks tracked (with noise)
	TotalBlocks int64 `json:"total_blocks"`
	
	// Replication buckets (with noise)
	LowReplicationCount  int64 `json:"low_replication_count"`
	MediumReplicationCount int64 `json:"medium_replication_count"`
	HighReplicationCount int64 `json:"high_replication_count"`
	
	// Average metrics (with noise)
	AveragePopularity float64 `json:"average_popularity"`
	AverageEntropy    float64 `json:"average_entropy"`
	
	// Geographic diversity (anonymized regions)
	RegionCounts map[string]int64 `json:"region_counts"`
}

// HealthGossiper implements privacy-preserving gossip for block health
type HealthGossiper struct {
	config        *HealthGossipConfig
	healthTracker *BlockHealthTracker
	shell         *shell.Shell
	
	// Gossip state
	localBloomFilter    *bloom.BloomFilter
	peerHealthEstimates map[string]*PeerHealthEstimate
	lastGossipTime      time.Time
	
	// Privacy state
	noiseGenerator *LaplaceNoise
	
	// Metrics
	gossipsSent     int64
	gossipsReceived int64
	
	// Control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
}

// PeerHealthEstimate tracks estimated health info from a peer
type PeerHealthEstimate struct {
	LastUpdate           time.Time
	LowReplicationBlocks *bloom.BloomFilter
	HighEntropyBlocks    *bloom.BloomFilter
	AggregateStats       *AggregateHealthStats
}

// NewHealthGossiper creates a new health gossip protocol instance
func NewHealthGossiper(config *HealthGossipConfig, healthTracker *BlockHealthTracker, shell *shell.Shell) (*HealthGossiper, error) {
	if config == nil {
		config = DefaultHealthGossipConfig()
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	hg := &HealthGossiper{
		config:              config,
		healthTracker:       healthTracker,
		shell:              shell,
		peerHealthEstimates: make(map[string]*PeerHealthEstimate),
		ctx:                ctx,
		cancel:             cancel,
	}
	
	// Initialize noise generator for differential privacy
	if config.EnableDifferentialPrivacy {
		hg.noiseGenerator = NewLaplaceNoise(config.PrivacyEpsilon)
	}
	
	// Initialize local bloom filter
	hg.localBloomFilter = bloom.NewWithEstimates(
		uint(config.BloomFilterSize),
		config.BloomFalsePositive,
	)
	
	return hg, nil
}

// Start begins the gossip protocol
func (hg *HealthGossiper) Start() error {
	hg.mu.Lock()
	defer hg.mu.Unlock()
	
	// Subscribe to gossip topic
	hg.wg.Add(1)
	go hg.gossipLoop()
	
	// Subscribe to incoming gossip
	hg.wg.Add(1)
	go hg.receiveLoop()
	
	return nil
}

// Stop stops the gossip protocol
func (hg *HealthGossiper) Stop() {
	hg.cancel()
	hg.wg.Wait()
}

// gossipLoop periodically sends health gossip
func (hg *HealthGossiper) gossipLoop() {
	defer hg.wg.Done()
	
	ticker := time.NewTicker(hg.config.GossipInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			if err := hg.sendGossip(); err != nil {
				// Log error but continue
				fmt.Printf("Gossip error: %v\n", err)
			}
		case <-hg.ctx.Done():
			return
		}
	}
}

// sendGossip creates and sends a gossip message
func (hg *HealthGossiper) sendGossip() error {
	hg.mu.Lock()
	defer hg.mu.Unlock()
	
	// Get current block health data
	blockHints := hg.healthTracker.GetAllBlockHints()
	
	// Check if we have enough blocks to gossip
	if len(blockHints) < hg.config.MinBlocksForGossip {
		return nil
	}
	
	// Create bloom filters
	lowRepFilter := bloom.NewWithEstimates(
		uint(hg.config.BloomFilterSize),
		hg.config.BloomFalsePositive,
	)
	highEntropyFilter := bloom.NewWithEstimates(
		uint(hg.config.BloomFilterSize),
		hg.config.BloomFalsePositive,
	)
	
	// Populate filters and calculate stats
	stats := hg.calculateAggregateStats(blockHints, lowRepFilter, highEntropyFilter)
	
	// Marshal bloom filters
	lowRepData, err := lowRepFilter.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to marshal low rep filter: %w", err)
	}
	
	highEntropyData, err := highEntropyFilter.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to marshal high entropy filter: %w", err)
	}
	
	// Create gossip message
	msg := &HealthGossipMessage{
		Timestamp:            time.Now(),
		LowReplicationFilter: lowRepData,
		HighEntropyFilter:    highEntropyData,
		AggregateStats:       stats,
		PeerID:              hg.anonymizePeerID(),
		Version:             1,
	}
	
	// Serialize message
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal gossip message: %w", err)
	}
	
	// Publish to gossip topic
	topic := "noisefs-health-gossip"
	if err := hg.shell.PubSubPublish(topic, string(data)); err != nil {
		return fmt.Errorf("failed to publish gossip: %w", err)
	}
	
	hg.gossipsSent++
	hg.lastGossipTime = time.Now()
	
	return nil
}

// calculateAggregateStats computes privacy-preserving aggregate statistics
func (hg *HealthGossiper) calculateAggregateStats(
	hints map[string]BlockHint,
	lowRepFilter *bloom.BloomFilter,
	highEntropyFilter *bloom.BloomFilter,
) *AggregateHealthStats {
	stats := &AggregateHealthStats{
		RegionCounts: make(map[string]int64),
	}
	
	var totalPopularity float64
	var totalEntropy float64
	
	for blockID, hint := range hints {
		// Add to appropriate bloom filters
		switch hint.ReplicationBucket {
		case ReplicationLow:
			lowRepFilter.AddString(blockID)
			stats.LowReplicationCount++
		case ReplicationMedium:
			stats.MediumReplicationCount++
		default:
			stats.HighReplicationCount++
		}
		
		if hint.HighEntropy {
			highEntropyFilter.AddString(blockID)
		}
		
		// Accumulate metrics
		totalPopularity += hint.NoisyRequestRate
		if hint.HighEntropy {
			totalEntropy += 1.0
		}
		
		// Count regions (anonymized)
		// MissingRegions is an int representing count, not a slice
		if hint.MissingRegions > 0 {
			// Group by region count buckets for privacy
			regionBucket := "unknown"
			if hint.MissingRegions <= 2 {
				regionBucket = "low"
			} else if hint.MissingRegions <= 5 {
				regionBucket = "medium"
			} else {
				regionBucket = "high"
			}
			stats.RegionCounts[regionBucket]++
		}
	}
	
	stats.TotalBlocks = int64(len(hints))
	
	// Calculate averages
	if stats.TotalBlocks > 0 {
		stats.AveragePopularity = totalPopularity / float64(stats.TotalBlocks)
		stats.AverageEntropy = totalEntropy / float64(stats.TotalBlocks)
	}
	
	// Add differential privacy noise if enabled
	if hg.config.EnableDifferentialPrivacy && hg.noiseGenerator != nil {
		stats.TotalBlocks = hg.noiseGenerator.AddNoiseInt64(stats.TotalBlocks)
		stats.LowReplicationCount = hg.noiseGenerator.AddNoiseInt64(stats.LowReplicationCount)
		stats.MediumReplicationCount = hg.noiseGenerator.AddNoiseInt64(stats.MediumReplicationCount)
		stats.HighReplicationCount = hg.noiseGenerator.AddNoiseInt64(stats.HighReplicationCount)
		stats.AveragePopularity = hg.noiseGenerator.AddNoiseFloat64(stats.AveragePopularity)
		stats.AverageEntropy = hg.noiseGenerator.AddNoiseFloat64(stats.AverageEntropy)
		
		// Add noise to region counts
		for region := range stats.RegionCounts {
			stats.RegionCounts[region] = hg.noiseGenerator.AddNoiseInt64(stats.RegionCounts[region])
		}
	}
	
	return stats
}

// receiveLoop handles incoming gossip messages
func (hg *HealthGossiper) receiveLoop() {
	defer hg.wg.Done()
	
	topic := "noisefs-health-gossip"
	sub, err := hg.shell.PubSubSubscribe(topic)
	if err != nil {
		fmt.Printf("Failed to subscribe to gossip: %v\n", err)
		return
	}
	defer sub.Cancel()
	
	for {
		select {
		case <-hg.ctx.Done():
			return
		default:
			msg, err := sub.Next()
			if err != nil {
				if hg.ctx.Err() != nil {
					return
				}
				continue
			}
			
			// Process gossip message
			if err := hg.processGossip(msg.Data); err != nil {
				fmt.Printf("Failed to process gossip: %v\n", err)
			}
		}
	}
}

// processGossip handles an incoming gossip message
func (hg *HealthGossiper) processGossip(data []byte) error {
	var msg HealthGossipMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return fmt.Errorf("failed to unmarshal gossip: %w", err)
	}
	
	// Validate message
	if msg.Version != 1 {
		return fmt.Errorf("unsupported gossip version: %d", msg.Version)
	}
	
	if time.Since(msg.Timestamp) > 10*time.Minute {
		return fmt.Errorf("gossip message too old")
	}
	
	hg.mu.Lock()
	defer hg.mu.Unlock()
	
	// Update peer health estimate
	estimate := &PeerHealthEstimate{
		LastUpdate:     msg.Timestamp,
		AggregateStats: msg.AggregateStats,
	}
	
	// Unmarshal bloom filters
	if len(msg.LowReplicationFilter) > 0 {
		filter := &bloom.BloomFilter{}
		if err := filter.UnmarshalBinary(msg.LowReplicationFilter); err == nil {
			estimate.LowReplicationBlocks = filter
		}
	}
	
	if len(msg.HighEntropyFilter) > 0 {
		filter := &bloom.BloomFilter{}
		if err := filter.UnmarshalBinary(msg.HighEntropyFilter); err == nil {
			estimate.HighEntropyBlocks = filter
		}
	}
	
	hg.peerHealthEstimates[msg.PeerID] = estimate
	hg.gossipsReceived++
	
	// Update local health tracker with network-wide information
	hg.updateHealthFromGossip(estimate)
	
	return nil
}

// updateHealthFromGossip updates local health tracking based on gossip
func (hg *HealthGossiper) updateHealthFromGossip(estimate *PeerHealthEstimate) {
	// This is where we would update our local health tracker
	// with aggregated network-wide information
	// For now, we just track the estimates
}

// GetNetworkHealthEstimate returns aggregated network health information
func (hg *HealthGossiper) GetNetworkHealthEstimate() *NetworkHealthEstimate {
	hg.mu.RLock()
	defer hg.mu.RUnlock()
	
	estimate := &NetworkHealthEstimate{
		Timestamp:       time.Now(),
		PeerCount:       len(hg.peerHealthEstimates),
		GossipsSent:     hg.gossipsSent,
		GossipsReceived: hg.gossipsReceived,
	}
	
	// Aggregate stats from all peers
	for _, peerEstimate := range hg.peerHealthEstimates {
		if time.Since(peerEstimate.LastUpdate) > hg.config.AggregationWindow {
			continue // Skip old estimates
		}
		
		if peerEstimate.AggregateStats != nil {
			estimate.TotalNetworkBlocks += peerEstimate.AggregateStats.TotalBlocks
			estimate.LowReplicationBlocks += peerEstimate.AggregateStats.LowReplicationCount
			estimate.HighEntropyBlocks += peerEstimate.AggregateStats.HighReplicationCount
		}
	}
	
	return estimate
}

// NetworkHealthEstimate represents network-wide health information
type NetworkHealthEstimate struct {
	Timestamp            time.Time
	PeerCount            int
	TotalNetworkBlocks   int64
	LowReplicationBlocks int64
	HighEntropyBlocks    int64
	GossipsSent          int64
	GossipsReceived      int64
}

// anonymizePeerID creates an anonymized peer identifier
func (hg *HealthGossiper) anonymizePeerID() string {
	// Generate random ID for privacy
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return fmt.Sprintf("peer-%x", bytes)
}

// anonymizeRegion maps regions to coarse buckets for privacy
func (hg *HealthGossiper) anonymizeRegion(region int) string {
	// Map to continental regions for privacy
	switch region {
	case 0, 1, 2:
		return "americas"
	case 3, 4, 5:
		return "europe"
	case 6, 7, 8:
		return "asia"
	default:
		return "other"
	}
}

// LaplaceNoise generates Laplace noise for differential privacy
type LaplaceNoise struct {
	epsilon float64
}

// NewLaplaceNoise creates a new Laplace noise generator
func NewLaplaceNoise(epsilon float64) *LaplaceNoise {
	return &LaplaceNoise{epsilon: epsilon}
}

// AddNoiseInt64 adds Laplace noise to an int64 value
func (ln *LaplaceNoise) AddNoiseInt64(value int64) int64 {
	noise := ln.generateLaplace()
	return value + int64(noise)
}

// AddNoiseFloat64 adds Laplace noise to a float64 value
func (ln *LaplaceNoise) AddNoiseFloat64(value float64) float64 {
	return value + ln.generateLaplace()
}

// generateLaplace generates a sample from Laplace distribution
func (ln *LaplaceNoise) generateLaplace() float64 {
	// Generate uniform random in (0, 1)
	max := big.NewInt(1 << 53)
	n, _ := rand.Int(rand.Reader, max)
	u := float64(n.Int64()) / float64(max.Int64())
	
	// Transform to Laplace distribution
	// scale = sensitivity / epsilon
	scale := 1.0 / ln.epsilon
	
	// Laplace CDF inverse
	if u < 0.5 {
		return scale * math.Log(2*u)
	}
	return -scale * math.Log(2*(1-u))
}
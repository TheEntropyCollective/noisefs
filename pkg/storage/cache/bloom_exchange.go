package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/bits-and-blooms/bloom/v3"
	shell "github.com/ipfs/go-ipfs-api"
)

// BloomExchangeConfig configures the Bloom filter exchange protocol
type BloomExchangeConfig struct {
	// Exchange interval
	ExchangeInterval time.Duration `json:"exchange_interval"`
	
	// Bloom filter parameters per category
	FilterCategories map[string]*BloomFilterParams `json:"filter_categories"`
	
	// Maximum filters to maintain
	MaxPeerFilters int `json:"max_peer_filters"`
	
	// Filter expiry time
	FilterExpiry time.Duration `json:"filter_expiry"`
	
	// Coordination parameters
	MinPeersForCoordination int `json:"min_peers_for_coordination"`
	CoordinationThreshold   float64 `json:"coordination_threshold"`
}

// BloomFilterParams defines parameters for a Bloom filter category
type BloomFilterParams struct {
	Size            uint    `json:"size"`
	HashFunctions   uint    `json:"hash_functions"`
	FalsePositive   float64 `json:"false_positive"`
}

// DefaultBloomExchangeConfig returns default configuration
func DefaultBloomExchangeConfig() *BloomExchangeConfig {
	return &BloomExchangeConfig{
		ExchangeInterval: 10 * time.Minute,
		FilterCategories: map[string]*BloomFilterParams{
			"valuable_blocks": {
				Size:          50000,
				HashFunctions: 5,
				FalsePositive: 0.01,
			},
			"personal_blocks": {
				Size:          10000,
				HashFunctions: 4,
				FalsePositive: 0.05,
			},
			"popular_randomizers": {
				Size:          20000,
				HashFunctions: 5,
				FalsePositive: 0.02,
			},
		},
		MaxPeerFilters:          100,
		FilterExpiry:            30 * time.Minute,
		MinPeersForCoordination: 3,
		CoordinationThreshold:   0.7,
	}
}

// BloomExchangeMessage represents a Bloom filter exchange message
type BloomExchangeMessage struct {
	// Message metadata
	Timestamp   time.Time `json:"timestamp"`
	PeerID      string    `json:"peer_id"`
	Version     int       `json:"version"`
	
	// Bloom filters by category
	Filters map[string]*SerializedBloomFilter `json:"filters"`
	
	// Coordination hints
	CoordinationHints *CoordinationHints `json:"coordination_hints,omitempty"`
}

// SerializedBloomFilter represents a serialized Bloom filter
type SerializedBloomFilter struct {
	Data         []byte  `json:"data"`
	ElementCount uint    `json:"element_count"`
	Capacity     uint    `json:"capacity"`
	FillRatio    float64 `json:"fill_ratio"`
}

// CoordinationHints provides hints for coordinated caching
type CoordinationHints struct {
	// Blocks that multiple peers are interested in
	HighDemandBlocks []string `json:"high_demand_blocks,omitempty"`
	
	// Suggested blocks for this peer to cache
	SuggestedBlocks []string `json:"suggested_blocks,omitempty"`
	
	// Coordination score (0-1)
	CoordinationScore float64 `json:"coordination_score"`
}

// BloomExchanger handles Bloom filter exchange between peers
type BloomExchanger struct {
	config        *BloomExchangeConfig
	cache         *AltruisticCache
	shell         *shell.Shell
	
	// Local filters
	localFilters map[string]*bloom.BloomFilter
	
	// Peer filters
	peerFilters map[string]*PeerFilterSet
	
	// Coordination state
	coordinationEngine *CoordinationEngine
	
	// Metrics
	exchangesSent     int64
	exchangesReceived int64
	
	// Control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
}

// PeerFilterSet tracks filters from a peer
type PeerFilterSet struct {
	PeerID      string
	LastUpdate  time.Time
	Filters     map[string]*bloom.BloomFilter
	Hints       *CoordinationHints
}

// NewBloomExchanger creates a new Bloom filter exchanger
func NewBloomExchanger(config *BloomExchangeConfig, cache *AltruisticCache, shell *shell.Shell) (*BloomExchanger, error) {
	if config == nil {
		config = DefaultBloomExchangeConfig()
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	be := &BloomExchanger{
		config:       config,
		cache:        cache,
		shell:        shell,
		localFilters: make(map[string]*bloom.BloomFilter),
		peerFilters:  make(map[string]*PeerFilterSet),
		ctx:          ctx,
		cancel:       cancel,
	}
	
	// Initialize local filters
	for category, params := range config.FilterCategories {
		be.localFilters[category] = bloom.NewWithEstimates(
			params.Size,
			params.FalsePositive,
		)
	}
	
	// Initialize coordination engine
	be.coordinationEngine = NewCoordinationEngine(config)
	
	return be, nil
}

// Start begins the Bloom filter exchange protocol
func (be *BloomExchanger) Start() error {
	be.wg.Add(1)
	go be.exchangeLoop()
	
	be.wg.Add(1)
	go be.receiveLoop()
	
	be.wg.Add(1)
	go be.cleanupLoop()
	
	return nil
}

// Stop stops the exchange protocol
func (be *BloomExchanger) Stop() {
	be.cancel()
	be.wg.Wait()
}

// UpdateLocalFilters updates local Bloom filters based on cache state
func (be *BloomExchanger) UpdateLocalFilters() {
	be.mu.Lock()
	defer be.mu.Unlock()
	
	// Get cache statistics
	stats := be.cache.GetAltruisticStats()
	
	// Clear and rebuild filters
	for category := range be.localFilters {
		be.localFilters[category].ClearAll()
	}
	
	// Update valuable blocks filter
	if filter, exists := be.localFilters["valuable_blocks"]; exists {
		be.cache.mu.RLock()
		for blockID, metadata := range be.cache.altruisticBlocks {
			// Check if block is valuable based on health
			healthScore := be.cache.healthTracker.GetBlockValue(blockID)
			if healthScore > 0.7 {
				filter.AddString(blockID)
			}
		}
		be.cache.mu.RUnlock()
	}
	
	// Update personal blocks filter (privacy-preserving)
	if filter, exists := be.localFilters["personal_blocks"]; exists {
		be.cache.mu.RLock()
		// Only add a sample of personal blocks for privacy
		sampleCount := 0
		maxSample := len(be.cache.personalBlocks) / 10 // 10% sample
		for blockID := range be.cache.personalBlocks {
			if sampleCount >= maxSample {
				break
			}
			filter.AddString(blockID)
			sampleCount++
		}
		be.cache.mu.RUnlock()
	}
	
	// Update popular randomizers filter
	if filter, exists := be.localFilters["popular_randomizers"]; exists {
		randomizers, _ := be.cache.GetRandomizers(100)
		for _, info := range randomizers {
			filter.AddString(info.CID)
		}
	}
}

// exchangeLoop periodically exchanges Bloom filters
func (be *BloomExchanger) exchangeLoop() {
	defer be.wg.Done()
	
	ticker := time.NewTicker(be.config.ExchangeInterval)
	defer ticker.Stop()
	
	// Initial update
	be.UpdateLocalFilters()
	
	for {
		select {
		case <-ticker.C:
			// Update filters
			be.UpdateLocalFilters()
			
			// Send exchange message
			if err := be.sendExchange(); err != nil {
				fmt.Printf("Exchange error: %v\n", err)
			}
		case <-be.ctx.Done():
			return
		}
	}
}

// sendExchange creates and sends an exchange message
func (be *BloomExchanger) sendExchange() error {
	be.mu.RLock()
	defer be.mu.RUnlock()
	
	// Create exchange message
	msg := &BloomExchangeMessage{
		Timestamp: time.Now(),
		PeerID:    be.generatePeerID(),
		Version:   1,
		Filters:   make(map[string]*SerializedBloomFilter),
	}
	
	// Serialize local filters
	for category, filter := range be.localFilters {
		data, err := filter.MarshalBinary()
		if err != nil {
			continue
		}
		
		msg.Filters[category] = &SerializedBloomFilter{
			Data:         data,
			ElementCount: filter.ApproximatedSize(),
			Capacity:     filter.Cap(),
			FillRatio:    filter.FillRatio(),
		}
	}
	
	// Add coordination hints if we have enough peers
	if len(be.peerFilters) >= be.config.MinPeersForCoordination {
		msg.CoordinationHints = be.coordinationEngine.GenerateHints(
			be.localFilters,
			be.peerFilters,
		)
	}
	
	// Serialize and publish
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal exchange message: %w", err)
	}
	
	topic := "noisefs-bloom-exchange"
	if err := be.shell.PubSubPublish(topic, string(data)); err != nil {
		return fmt.Errorf("failed to publish exchange: %w", err)
	}
	
	be.exchangesSent++
	return nil
}

// receiveLoop handles incoming exchange messages
func (be *BloomExchanger) receiveLoop() {
	defer be.wg.Done()
	
	topic := "noisefs-bloom-exchange"
	sub, err := be.shell.PubSubSubscribe(topic)
	if err != nil {
		fmt.Printf("Failed to subscribe to exchange: %v\n", err)
		return
	}
	defer sub.Cancel()
	
	for {
		select {
		case <-be.ctx.Done():
			return
		default:
			msg, err := sub.Next()
			if err != nil {
				if be.ctx.Err() != nil {
					return
				}
				continue
			}
			
			// Process exchange message
			if err := be.processExchange(msg.Data); err != nil {
				fmt.Printf("Failed to process exchange: %v\n", err)
			}
		}
	}
}

// processExchange handles an incoming exchange message
func (be *BloomExchanger) processExchange(data []byte) error {
	var msg BloomExchangeMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return fmt.Errorf("failed to unmarshal exchange: %w", err)
	}
	
	// Validate message
	if msg.Version != 1 {
		return fmt.Errorf("unsupported exchange version: %d", msg.Version)
	}
	
	if time.Since(msg.Timestamp) > be.config.FilterExpiry {
		return fmt.Errorf("exchange message too old")
	}
	
	be.mu.Lock()
	defer be.mu.Unlock()
	
	// Create peer filter set
	peerSet := &PeerFilterSet{
		PeerID:     msg.PeerID,
		LastUpdate: msg.Timestamp,
		Filters:    make(map[string]*bloom.BloomFilter),
		Hints:      msg.CoordinationHints,
	}
	
	// Unmarshal filters
	for category, serialized := range msg.Filters {
		filter := &bloom.BloomFilter{}
		if err := filter.UnmarshalBinary(serialized.Data); err == nil {
			peerSet.Filters[category] = filter
		}
	}
	
	// Store peer filters
	be.peerFilters[msg.PeerID] = peerSet
	be.exchangesReceived++
	
	// Process coordination hints
	if msg.CoordinationHints != nil {
		be.processCoordinationHints(msg.CoordinationHints)
	}
	
	// Limit peer filters
	if len(be.peerFilters) > be.config.MaxPeerFilters {
		be.evictOldestPeerFilter()
	}
	
	return nil
}

// processCoordinationHints processes coordination hints from peers
func (be *BloomExchanger) processCoordinationHints(hints *CoordinationHints) {
	if hints.CoordinationScore < be.config.CoordinationThreshold {
		return
	}
	
	// Queue suggested blocks for opportunistic fetching
	if be.cache.config.EnableAltruistic {
		fetcher := be.cache.GetHealthTracker().GetOpportunisticFetcher()
		if fetcher != nil {
			for _, blockID := range hints.SuggestedBlocks {
				// Only queue if we don't already have it
				if !be.cache.Has(blockID) {
					fetcher.QueueBlock(blockID, BlockHint{
						ReplicationBucket: ReplicationLow,
						HighEntropy:       true,
					})
				}
			}
		}
	}
}

// cleanupLoop removes expired peer filters
func (be *BloomExchanger) cleanupLoop() {
	defer be.wg.Done()
	
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			be.cleanupExpiredFilters()
		case <-be.ctx.Done():
			return
		}
	}
}

// cleanupExpiredFilters removes expired peer filters
func (be *BloomExchanger) cleanupExpiredFilters() {
	be.mu.Lock()
	defer be.mu.Unlock()
	
	now := time.Now()
	for peerID, filterSet := range be.peerFilters {
		if now.Sub(filterSet.LastUpdate) > be.config.FilterExpiry {
			delete(be.peerFilters, peerID)
		}
	}
}

// evictOldestPeerFilter removes the oldest peer filter
func (be *BloomExchanger) evictOldestPeerFilter() {
	var oldestPeer string
	var oldestTime time.Time
	
	for peerID, filterSet := range be.peerFilters {
		if oldestPeer == "" || filterSet.LastUpdate.Before(oldestTime) {
			oldestPeer = peerID
			oldestTime = filterSet.LastUpdate
		}
	}
	
	if oldestPeer != "" {
		delete(be.peerFilters, oldestPeer)
	}
}

// GetPeerCoordination returns coordination statistics
func (be *BloomExchanger) GetPeerCoordination() *PeerCoordinationStats {
	be.mu.RLock()
	defer be.mu.RUnlock()
	
	stats := &PeerCoordinationStats{
		ActivePeers:       len(be.peerFilters),
		ExchangesSent:     be.exchangesSent,
		ExchangesReceived: be.exchangesReceived,
		Categories:        make(map[string]*CategoryStats),
	}
	
	// Calculate overlap statistics by category
	for category, localFilter := range be.localFilters {
		catStats := &CategoryStats{
			LocalSize: localFilter.ApproximatedSize(),
		}
		
		// Calculate average overlap with peers
		totalOverlap := uint(0)
		peerCount := 0
		
		for _, peerSet := range be.peerFilters {
			if peerFilter, exists := peerSet.Filters[category]; exists {
				// Estimate overlap (this is approximate)
				overlap := estimateBloomOverlap(localFilter, peerFilter)
				totalOverlap += overlap
				peerCount++
			}
		}
		
		if peerCount > 0 {
			catStats.AverageOverlap = float64(totalOverlap) / float64(peerCount)
		}
		
		stats.Categories[category] = catStats
	}
	
	return stats
}

// PeerCoordinationStats represents coordination statistics
type PeerCoordinationStats struct {
	ActivePeers       int
	ExchangesSent     int64
	ExchangesReceived int64
	Categories        map[string]*CategoryStats
}

// CategoryStats represents statistics for a filter category
type CategoryStats struct {
	LocalSize      uint
	AverageOverlap float64
}

// generatePeerID creates an anonymized peer ID
func (be *BloomExchanger) generatePeerID() string {
	// Use cache's peer ID generation for consistency
	return fmt.Sprintf("bloom-peer-%d", time.Now().UnixNano())
}

// estimateBloomOverlap estimates the overlap between two Bloom filters
func estimateBloomOverlap(f1, f2 *bloom.BloomFilter) uint {
	// This is a rough estimate based on fill ratios
	// In practice, you'd need to implement proper overlap estimation
	fillRatio1 := f1.FillRatio()
	fillRatio2 := f2.FillRatio()
	
	// Estimate overlap based on fill ratios
	overlapRatio := fillRatio1 * fillRatio2
	estimatedOverlap := uint(overlapRatio * float64(f1.Cap()))
	
	return estimatedOverlap
}
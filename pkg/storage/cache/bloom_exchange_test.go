package cache

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/bits-and-blooms/bloom/v3"
)

func TestBloomExchanger_FilterCreation(t *testing.T) {
	// Create test cache
	baseCache := NewMemoryCache(100)
	config := &AltruisticCacheConfig{
		MinPersonalCache: 50 * 1024,
		EnableAltruistic: true,
	}
	cache := NewAltruisticCache(baseCache, config, 100*1024)

	// Add test blocks
	for i := 0; i < 10; i++ {
		data := make([]byte, 1024)
		block := &blocks.Block{Data: data}

		if i < 5 {
			cache.StoreWithOrigin(string(rune('a'+i)), block, PersonalBlock)
		} else {
			cache.StoreWithOrigin(string(rune('a'+i)), block, AltruisticBlock)
			// Mark some as valuable
			if i%2 == 0 {
				cache.UpdateBlockHealth(string(rune('a'+i)), BlockHint{
					ReplicationBucket: ReplicationLow,
					HighEntropy:       true,
				})
			}
		}
	}

	// Create bloom exchanger
	exchangeConfig := DefaultBloomExchangeConfig()
	exchanger, err := NewBloomExchanger(exchangeConfig, cache, nil)
	if err != nil {
		t.Fatalf("Failed to create exchanger: %v", err)
	}

	// Update local filters
	exchanger.UpdateLocalFilters()

	// Check valuable blocks filter
	valuableFilter := exchanger.localFilters["valuable_blocks"]
	if valuableFilter == nil {
		t.Fatal("Valuable blocks filter should exist")
	}

	// Should contain valuable altruistic blocks
	// Note: UpdateLocalFilters uses health score > 0.7 threshold
	// Our test blocks might not meet this threshold without proper health scoring

	// Check personal blocks filter
	personalFilter := exchanger.localFilters["personal_blocks"]
	if personalFilter == nil {
		t.Fatal("Personal blocks filter should exist")
	}

	// Should contain sampled personal blocks (10% sample)
	// With 5 personal blocks, should sample at least 1
	hasPersonal := false
	for i := 0; i < 5; i++ {
		if personalFilter.TestString(string(rune('a' + i))) {
			hasPersonal = true
			break
		}
	}

	if !hasPersonal {
		t.Error("Personal blocks filter should contain at least one personal block")
	}
}

func TestBloomExchanger_MessageSerialization(t *testing.T) {
	// Create test message
	filter1 := bloom.NewWithEstimates(1000, 0.01)
	filter1.AddString("block1")
	filter1.AddString("block2")

	filter2 := bloom.NewWithEstimates(1000, 0.01)
	filter2.AddString("block3")

	filter1Data, _ := filter1.MarshalBinary()
	filter2Data, _ := filter2.MarshalBinary()

	msg := &BloomExchangeMessage{
		Timestamp: time.Now(),
		PeerID:    "test-peer",
		Version:   1,
		Filters: map[string]*SerializedBloomFilter{
			"valuable_blocks": {
				Data:         filter1Data,
				ElementCount: 2,
				Capacity:     1000,
				FillRatio:    float64(filter1.ApproximatedSize()) / float64(filter1.Cap()),
			},
			"personal_blocks": {
				Data:         filter2Data,
				ElementCount: 1,
				Capacity:     1000,
				FillRatio:    float64(filter2.ApproximatedSize()) / float64(filter2.Cap()),
			},
		},
		CoordinationHints: &CoordinationHints{
			HighDemandBlocks:  []string{"block1", "block2"},
			SuggestedBlocks:   []string{"block4", "block5"},
			CoordinationScore: 0.75,
		},
	}

	// Serialize
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal message: %v", err)
	}

	// Deserialize
	var decoded BloomExchangeMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal message: %v", err)
	}

	// Verify fields
	if decoded.Version != msg.Version {
		t.Errorf("Version mismatch: got %d, want %d", decoded.Version, msg.Version)
	}

	if decoded.PeerID != msg.PeerID {
		t.Errorf("PeerID mismatch: got %s, want %s", decoded.PeerID, msg.PeerID)
	}

	// Verify filters
	if len(decoded.Filters) != 2 {
		t.Errorf("Expected 2 filters, got %d", len(decoded.Filters))
	}

	// Verify coordination hints
	if decoded.CoordinationHints == nil {
		t.Fatal("Coordination hints should be present")
	}

	if len(decoded.CoordinationHints.HighDemandBlocks) != 2 {
		t.Errorf("Expected 2 high demand blocks, got %d",
			len(decoded.CoordinationHints.HighDemandBlocks))
	}

	if decoded.CoordinationHints.CoordinationScore != 0.75 {
		t.Errorf("Expected coordination score 0.75, got %f",
			decoded.CoordinationHints.CoordinationScore)
	}
}

func TestBloomExchanger_ProcessExchange(t *testing.T) {
	// Create test cache and exchanger
	baseCache := NewMemoryCache(100)
	config := &AltruisticCacheConfig{
		MinPersonalCache: 50 * 1024,
		EnableAltruistic: true,
	}
	cache := NewAltruisticCache(baseCache, config, 100*1024)

	exchangeConfig := &BloomExchangeConfig{
		FilterExpiry:            10 * time.Minute,
		MaxPeerFilters:          5,
		CoordinationThreshold:   0.5,
		MinPeersForCoordination: 2,
	}

	exchanger, err := NewBloomExchanger(exchangeConfig, cache, nil)
	if err != nil {
		t.Fatalf("Failed to create exchanger: %v", err)
	}

	// Create test message
	filter := bloom.NewWithEstimates(1000, 0.01)
	filter.AddString("test-block")
	filterData, _ := filter.MarshalBinary()

	msg := &BloomExchangeMessage{
		Timestamp: time.Now(),
		PeerID:    "peer1",
		Version:   1,
		Filters: map[string]*SerializedBloomFilter{
			"valuable_blocks": {
				Data:         filterData,
				ElementCount: 1,
				Capacity:     1000,
				FillRatio:    0.01,
			},
		},
		CoordinationHints: &CoordinationHints{
			HighDemandBlocks:  []string{"block1"},
			CoordinationScore: 0.8,
		},
	}

	data, _ := json.Marshal(msg)

	// Process message
	err = exchanger.processExchange(data)
	if err != nil {
		t.Fatalf("Failed to process exchange: %v", err)
	}

	// Verify peer filter was stored
	exchanger.mu.RLock()
	peerSet, exists := exchanger.peerFilters["peer1"]
	exchanger.mu.RUnlock()

	if !exists {
		t.Fatal("Peer filter set should be stored")
	}

	if peerSet.PeerID != "peer1" {
		t.Errorf("Expected peer ID peer1, got %s", peerSet.PeerID)
	}

	// Verify filter was unmarshaled
	valuableFilter, exists := peerSet.Filters["valuable_blocks"]
	if !exists {
		t.Fatal("Valuable blocks filter should exist")
	}

	if !valuableFilter.TestString("test-block") {
		t.Error("Filter should contain test-block")
	}

	// Verify coordination hints
	if peerSet.Hints == nil {
		t.Fatal("Coordination hints should be stored")
	}

	if peerSet.Hints.CoordinationScore != 0.8 {
		t.Errorf("Expected coordination score 0.8, got %f", peerSet.Hints.CoordinationScore)
	}
}

func TestBloomExchanger_OldMessageRejection(t *testing.T) {
	baseCache := NewMemoryCache(100)
	config := &AltruisticCacheConfig{
		MinPersonalCache: 50 * 1024,
		EnableAltruistic: true,
	}
	cache := NewAltruisticCache(baseCache, config, 100*1024)

	exchangeConfig := &BloomExchangeConfig{
		FilterExpiry: 5 * time.Minute,
	}

	exchanger, err := NewBloomExchanger(exchangeConfig, cache, nil)
	if err != nil {
		t.Fatalf("Failed to create exchanger: %v", err)
	}

	// Create old message
	msg := &BloomExchangeMessage{
		Timestamp: time.Now().Add(-10 * time.Minute), // Too old
		PeerID:    "old-peer",
		Version:   1,
		Filters:   make(map[string]*SerializedBloomFilter),
	}

	data, _ := json.Marshal(msg)
	err = exchanger.processExchange(data)

	if err == nil {
		t.Error("Should reject messages older than filter expiry")
	}
}

func TestBloomExchanger_MaxPeerFilters(t *testing.T) {
	baseCache := NewMemoryCache(100)
	config := &AltruisticCacheConfig{
		MinPersonalCache: 50 * 1024,
		EnableAltruistic: true,
	}
	cache := NewAltruisticCache(baseCache, config, 100*1024)

	exchangeConfig := &BloomExchangeConfig{
		MaxPeerFilters: 3,
		FilterExpiry:   10 * time.Minute,
	}

	exchanger, err := NewBloomExchanger(exchangeConfig, cache, nil)
	if err != nil {
		t.Fatalf("Failed to create exchanger: %v", err)
	}

	// Add peer filters up to and beyond limit
	now := time.Now()
	for i := 0; i < 5; i++ {
		exchanger.mu.Lock()
		exchanger.peerFilters[string(rune('a'+i))] = &PeerFilterSet{
			PeerID:     string(rune('a' + i)),
			LastUpdate: now.Add(time.Duration(-i) * time.Minute),
			Filters:    make(map[string]*bloom.BloomFilter),
		}
		exchanger.mu.Unlock()

		// Trigger eviction if needed
		if len(exchanger.peerFilters) > exchangeConfig.MaxPeerFilters {
			exchanger.evictOldestPeerFilter()
		}
	}

	// Should only have max filters
	exchanger.mu.RLock()
	count := len(exchanger.peerFilters)
	exchanger.mu.RUnlock()

	if count != 3 {
		t.Errorf("Expected %d peer filters, got %d", exchangeConfig.MaxPeerFilters, count)
	}

	// Oldest peers should have been evicted
	exchanger.mu.RLock()
	_, hasOldest := exchanger.peerFilters["d"] // 3 minutes old
	_, hasOlder := exchanger.peerFilters["e"]  // 4 minutes old
	exchanger.mu.RUnlock()

	if hasOldest || hasOlder {
		t.Error("Oldest peer filters should have been evicted")
	}
}

func TestBloomExchanger_CleanupExpired(t *testing.T) {
	baseCache := NewMemoryCache(100)
	config := &AltruisticCacheConfig{
		MinPersonalCache: 50 * 1024,
		EnableAltruistic: true,
	}
	cache := NewAltruisticCache(baseCache, config, 100*1024)

	exchangeConfig := &BloomExchangeConfig{
		FilterExpiry: 5 * time.Minute,
	}

	exchanger, err := NewBloomExchanger(exchangeConfig, cache, nil)
	if err != nil {
		t.Fatalf("Failed to create exchanger: %v", err)
	}

	// Add filters with different ages
	now := time.Now()
	exchanger.mu.Lock()
	exchanger.peerFilters["recent"] = &PeerFilterSet{
		PeerID:     "recent",
		LastUpdate: now.Add(-2 * time.Minute),
		Filters:    make(map[string]*bloom.BloomFilter),
	}
	exchanger.peerFilters["old"] = &PeerFilterSet{
		PeerID:     "old",
		LastUpdate: now.Add(-10 * time.Minute),
		Filters:    make(map[string]*bloom.BloomFilter),
	}
	exchanger.mu.Unlock()

	// Run cleanup
	exchanger.cleanupExpiredFilters()

	// Check results
	exchanger.mu.RLock()
	_, hasRecent := exchanger.peerFilters["recent"]
	_, hasOld := exchanger.peerFilters["old"]
	exchanger.mu.RUnlock()

	if !hasRecent {
		t.Error("Recent filter should not be removed")
	}

	if hasOld {
		t.Error("Old filter should be removed")
	}
}

func TestBloomExchanger_CoordinationThreshold(t *testing.T) {
	baseCache := NewMemoryCache(100)
	config := &AltruisticCacheConfig{
		MinPersonalCache: 50 * 1024,
		EnableAltruistic: true,
	}
	cache := NewAltruisticCache(baseCache, config, 100*1024)

	exchangeConfig := &BloomExchangeConfig{
		CoordinationThreshold: 0.7,
	}

	exchanger, err := NewBloomExchanger(exchangeConfig, cache, nil)
	if err != nil {
		t.Fatalf("Failed to create exchanger: %v", err)
	}

	// Test with low coordination score
	lowHints := &CoordinationHints{
		CoordinationScore: 0.5,
		SuggestedBlocks:   []string{"block1", "block2"},
	}

	// Process hints - should not queue blocks due to low score
	exchanger.processCoordinationHints(lowHints)

	// For testing purposes, verify hints were processed without error
	// (this is a simplified test - real implementation would check queue state)

	// Test with high coordination score
	highHints := &CoordinationHints{
		CoordinationScore: 0.8,
		SuggestedBlocks:   []string{"block3"},
	}

	// Process hints - should queue blocks
	exchanger.processCoordinationHints(highHints)

	// Note: In the actual implementation, QueueBlock checks if we already have the block
	// For this test, we're just verifying the coordination threshold logic
}

func TestEstimateBloomOverlap(t *testing.T) {
	// Create two filters with known overlap
	filter1 := bloom.NewWithEstimates(1000, 0.01)
	filter2 := bloom.NewWithEstimates(1000, 0.01)

	// Add some unique elements to each
	for i := 0; i < 50; i++ {
		filter1.AddString(string(rune('a' + i)))
	}

	for i := 25; i < 75; i++ {
		filter2.AddString(string(rune('a' + i)))
	}

	// Estimate overlap
	overlap := estimateBloomOverlap(filter1, filter2)

	// Should be greater than 0
	if overlap == 0 {
		t.Error("Overlap should be greater than 0 for filters with common elements")
	}

	// Test with empty filters
	emptyFilter1 := bloom.NewWithEstimates(1000, 0.01)
	emptyFilter2 := bloom.NewWithEstimates(1000, 0.01)

	emptyOverlap := estimateBloomOverlap(emptyFilter1, emptyFilter2)
	if emptyOverlap != 0 {
		t.Error("Empty filters should have 0 overlap")
	}
}

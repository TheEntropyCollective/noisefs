package cache

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// AdaptiveCacheItem represents a cached block with metadata for adaptive caching
type AdaptiveCacheItem struct {
	CID             string            `json:"cid"`
	Data            []byte            `json:"-"`
	Size            int64             `json:"size"`
	CreatedAt       time.Time         `json:"created_at"`
	LastAccessed    time.Time         `json:"last_accessed"`
	AccessCount     int64             `json:"access_count"`
	PopularityScore float64           `json:"popularity_score"`
	PredictedValue  float64           `json:"predicted_value"`
	Tier            AdaptiveCacheTier `json:"tier"`
	RandomizerUse   int64             `json:"randomizer_use"`
	
	// Metadata for decision making
	IsRandomizer    bool              `json:"is_randomizer"`
	BlockType       string            `json:"block_type"`
	SourcePeer      peer.ID           `json:"source_peer"`
	
	mutex           sync.RWMutex
}

// AdaptiveAccessPattern tracks access patterns for ML prediction
type AdaptiveAccessPattern struct {
	CID                string              `json:"cid"`
	AccessTimes        []time.Time         `json:"access_times"`
	AccessIntervals    []time.Duration     `json:"access_intervals"`
	AccessCount        int64               `json:"access_count"`
	FirstAccess        time.Time           `json:"first_access"`
	LastAccess         time.Time           `json:"last_access"`
	DailyPattern       [24]int             `json:"daily_pattern"`
	WeeklyPattern      [7]int              `json:"weekly_pattern"`
	TrendDirection     float64             `json:"trend_direction"`
	Seasonality        float64             `json:"seasonality"`
	LastPrediction     time.Time           `json:"last_prediction"`
	PredictionAccuracy float64             `json:"prediction_accuracy"`
}

// AdaptiveCacheTier represents different cache tiers
type AdaptiveCacheTier int

const (
	AdaptiveHotTier AdaptiveCacheTier = iota
	AdaptiveWarmTier
	AdaptiveColdTier
)

// AdaptiveEvictionPolicy defines different eviction strategies
type AdaptiveEvictionPolicy interface {
	ShouldEvict(item *AdaptiveCacheItem, cache *AdaptiveCache) bool
	SelectEvictionCandidates(cache *AdaptiveCache, spaceNeeded int64) []*AdaptiveCacheItem
	GetPriority(item *AdaptiveCacheItem) float64
}

// AdaptiveCacheStats tracks cache performance metrics
type AdaptiveCacheStats struct {
	Hits            int64         `json:"hits"`
	Misses          int64         `json:"misses"`
	Evictions       int64         `json:"evictions"`
	Insertions      int64         `json:"insertions"`
	TotalRequests   int64         `json:"total_requests"`
	HitRate         float64       `json:"hit_rate"`
	AvgAccessTime   time.Duration `json:"avg_access_time"`
	
	// Tier statistics
	HotTierHits     int64         `json:"hot_tier_hits"`
	WarmTierHits    int64         `json:"warm_tier_hits"`
	ColdTierHits    int64         `json:"cold_tier_hits"`
	
	// Prediction accuracy
	PredictionHits  int64         `json:"prediction_hits"`
	PredictionTotal int64         `json:"prediction_total"`
	PredictionAccuracy float64    `json:"prediction_accuracy"`
	
	mutex           sync.RWMutex
}

// AdaptiveCacheConfig holds configuration for the adaptive cache
type AdaptiveCacheConfig struct {
	MaxSize            int64         `json:"max_size_bytes"`
	MaxItems           int           `json:"max_items"`
	HotTierRatio       float64       `json:"hot_tier_ratio"`
	WarmTierRatio      float64       `json:"warm_tier_ratio"`
	PredictionWindow   time.Duration `json:"prediction_window"`
	EvictionBatchSize  int           `json:"eviction_batch_size"`
	ExchangeInterval   time.Duration `json:"exchange_interval"`
	PredictionInterval time.Duration `json:"prediction_interval"`
	
	// Privacy settings
	PrivacyEpsilon     float64       `json:"privacy_epsilon"`     // Differential privacy parameter
	TemporalQuantum    time.Duration `json:"temporal_quantum"`    // Time quantization interval
	DummyAccessRate    float64       `json:"dummy_access_rate"`   // Rate of dummy accesses (0.0-1.0)
}

// PeerCacheInfo holds information about peer cache state
type PeerCacheInfo struct {
	PeerID           peer.ID              `json:"peer_id"`
	PopularBlocks    []string             `json:"popular_blocks"`
	CacheUtilization float64              `json:"cache_utilization"`
	LastSync         time.Time            `json:"last_sync"`
	ConnectionQuality float64             `json:"connection_quality"`
}

// CacheExchangeProtocol manages cache state exchange between peers
type CacheExchangeProtocol struct {
	exchangeRate    float64   `json:"exchange_rate"`
	lastExchange    time.Time `json:"last_exchange"`
	mutex           sync.RWMutex
}


// AdaptiveAccessPredictor predicts access patterns using ML
type AdaptiveAccessPredictor struct {
	engine          *AccessPredictor
	predictionCache map[string]float64
	cacheHitRate    float64
	lastUpdate      time.Time
	mutex           sync.RWMutex
}


// AdaptiveCache implements ML-based caching with intelligent eviction policies
type AdaptiveCache struct {
	// Core cache data
	items           map[string]*AdaptiveCacheItem
	accessHistory   map[string]*AdaptiveAccessPattern
	evictionPolicy  AdaptiveEvictionPolicy
	
	// Cache configuration
	maxSize         int64
	currentSize     int64
	maxItems        int
	
	// ML prediction model
	predictor       *AdaptiveAccessPredictor
	
	// Peer coordination
	peerCache       map[peer.ID]*PeerCacheInfo
	cacheExchange   *CacheExchangeProtocol
	
	// Privacy components
	privacyNoise    *rand.Rand
	lastDummyAccess time.Time
	
	// Synchronization
	mutex           sync.RWMutex
	
	// Statistics
	stats           *AdaptiveCacheStats
	
	// Configuration
	config          *AdaptiveCacheConfig
}

// basicAdaptiveEvictionPolicy provides a basic implementation of the AdaptiveEvictionPolicy interface
type basicAdaptiveEvictionPolicy struct{}

func (p *basicAdaptiveEvictionPolicy) ShouldEvict(item *AdaptiveCacheItem, cache *AdaptiveCache) bool {
	// Simple LRU-based eviction: evict if item hasn't been accessed recently
	return time.Since(item.LastAccessed) > time.Hour
}

func (p *basicAdaptiveEvictionPolicy) SelectEvictionCandidates(cache *AdaptiveCache, spaceNeeded int64) []*AdaptiveCacheItem {
	var candidates []*AdaptiveCacheItem
	var freedSpace int64
	
	// Sort items by last access time (oldest first)
	for _, item := range cache.items {
		if freedSpace >= spaceNeeded {
			break
		}
		candidates = append(candidates, item)
		freedSpace += item.Size
	}
	
	return candidates
}

func (p *basicAdaptiveEvictionPolicy) GetPriority(item *AdaptiveCacheItem) float64 {
	// Higher priority = less likely to be evicted
	// Base priority on access frequency and recency
	recency := time.Since(item.LastAccessed).Hours()
	frequency := float64(item.AccessCount)
	
	return frequency / (1.0 + recency)
}

// NewAdaptiveCache creates a new adaptive cache instance
func NewAdaptiveCache(config *AdaptiveCacheConfig) *AdaptiveCache {
	cache := &AdaptiveCache{
		items:         make(map[string]*AdaptiveCacheItem),
		accessHistory: make(map[string]*AdaptiveAccessPattern),
		peerCache:     make(map[peer.ID]*PeerCacheInfo),
		maxSize:       config.MaxSize,
		maxItems:      config.MaxItems,
		config:        config,
		stats:         &AdaptiveCacheStats{},
	}
	
	// Initialize ML predictor
	cache.predictor = &AdaptiveAccessPredictor{
		engine:          NewAccessPredictor(),
		predictionCache: make(map[string]float64),
		lastUpdate:      time.Now(),
	}
	
	// Initialize eviction policy with basic implementation
	cache.evictionPolicy = &basicAdaptiveEvictionPolicy{}
	
	// Set default intervals if not configured
	if config.PredictionInterval == 0 {
		config.PredictionInterval = time.Minute * 5
	}
	if config.ExchangeInterval == 0 {
		config.ExchangeInterval = time.Minute * 10
	}
	
	// Initialize cache exchange protocol
	cache.cacheExchange = &CacheExchangeProtocol{
		exchangeRate: 0.1, // 10% of cache state exchanged per sync
	}
	
	// Initialize privacy components
	cache.privacyNoise = rand.New(rand.NewSource(time.Now().UnixNano()))
	cache.lastDummyAccess = time.Now()
	
	// Start background tasks
	go cache.predictionLoop()
	go cache.evictionLoop()
	go cache.cacheExchangeLoop()
	go cache.privacyMaintenanceLoop()
	
	return cache
}

// Get retrieves an item from the cache
func (ac *AdaptiveCache) Get(cid string) ([]byte, bool) {
	ac.mutex.RLock()
	defer ac.mutex.RUnlock()
	
	item, exists := ac.items[cid]
	if !exists {
		ac.recordMiss(cid)
		return nil, false
	}
	
	// Update access metadata
	item.mutex.Lock()
	item.LastAccessed = time.Now()
	item.AccessCount++
	item.mutex.Unlock()
	
	// Update access pattern with privacy protections
	ac.updateAccessPatternPrivate(cid)
	
	// Record hit
	ac.recordHit(cid, item.Tier)
	
	// Promote tier if needed
	ac.promoteIfNeeded(item)
	
	return item.Data, true
}

// Put adds an item to the cache
func (ac *AdaptiveCache) Put(cid string, data []byte, metadata map[string]interface{}) error {
	ac.mutex.Lock()
	defer ac.mutex.Unlock()
	
	// Check if already exists
	if _, exists := ac.items[cid]; exists {
		return nil // Already cached
	}
	
	size := int64(len(data))
	
	// Check if we need to make space
	if ac.currentSize+size > ac.maxSize || len(ac.items) >= ac.maxItems {
		if err := ac.makeSpace(size); err != nil {
			return fmt.Errorf("failed to make space: %w", err)
		}
	}
	
	// Determine initial tier based on prediction
	tier := ac.predictInitialTier(cid, metadata)
	
	// Create cache item
	now := time.Now()
	item := &AdaptiveCacheItem{
		CID:          cid,
		Data:         data,
		Size:         size,
		CreatedAt:    now,
		LastAccessed: now,
		AccessCount:  1,
		Tier:         tier,
	}
	
	// Extract metadata
	if isRandomizer, ok := metadata["is_randomizer"].(bool); ok {
		item.IsRandomizer = isRandomizer
	}
	if blockType, ok := metadata["block_type"].(string); ok {
		item.BlockType = blockType
	}
	if sourcePeer, ok := metadata["source_peer"].(peer.ID); ok {
		item.SourcePeer = sourcePeer
	}
	
	// Add to cache
	ac.items[cid] = item
	ac.currentSize += size
	
	// Initialize access pattern
	ac.initializeAccessPattern(cid)
	
	// Update statistics
	ac.stats.mutex.Lock()
	ac.stats.Insertions++
	ac.stats.mutex.Unlock()
	
	return nil
}

// makeSpace evicts items to make room for new data
func (ac *AdaptiveCache) makeSpace(spaceNeeded int64) error {
	candidates := ac.evictionPolicy.SelectEvictionCandidates(ac, spaceNeeded)
	
	if len(candidates) == 0 {
		return fmt.Errorf("no eviction candidates found")
	}
	
	spaceFreed := int64(0)
	for _, item := range candidates {
		if spaceFreed >= spaceNeeded {
			break
		}
		
		ac.evictItem(item)
		spaceFreed += item.Size
	}
	
	if spaceFreed < spaceNeeded {
		return fmt.Errorf("insufficient space freed: need %d, freed %d", spaceNeeded, spaceFreed)
	}
	
	return nil
}

// evictItem removes an item from the cache
func (ac *AdaptiveCache) evictItem(item *AdaptiveCacheItem) {
	delete(ac.items, item.CID)
	ac.currentSize -= item.Size
	
	// Update statistics
	ac.stats.mutex.Lock()
	ac.stats.Evictions++
	ac.stats.mutex.Unlock()
}

// predictInitialTier predicts the initial tier for a new cache item
func (ac *AdaptiveCache) predictInitialTier(cid string, metadata map[string]interface{}) AdaptiveCacheTier {
	// Check if it's a randomizer block (likely to be accessed frequently)
	if isRandomizer, ok := metadata["is_randomizer"].(bool); ok && isRandomizer {
		return AdaptiveHotTier
	}
	
	// Use ML predictor if available
	if ac.predictor != nil && ac.predictor.engine != nil {
		// Get prediction score from ML model
		predictionScore := ac.predictor.engine.PredictAccess(cid, metadata)
		
		// Convert prediction score to tier assignment
		// Higher scores indicate higher likelihood of access
		if predictionScore >= 0.8 {
			return AdaptiveHotTier
		} else if predictionScore >= 0.5 {
			return AdaptiveWarmTier
		}
		// Scores below 0.5 continue to default tier assignment
	}
	
	return AdaptiveColdTier
}

// promoteIfNeeded promotes an item to a higher tier based on access patterns
func (ac *AdaptiveCache) promoteIfNeeded(item *AdaptiveCacheItem) {
	item.mutex.Lock()
	defer item.mutex.Unlock()
	
	// Calculate promotion score based on access frequency and recency
	timeSinceCreation := time.Since(item.CreatedAt)
	accessRate := float64(item.AccessCount) / timeSinceCreation.Hours()
	recencyScore := 1.0 / (1.0 + time.Since(item.LastAccessed).Hours())
	
	promotionScore := accessRate * recencyScore
	
	// Promotion thresholds
	if item.Tier == AdaptiveColdTier && promotionScore > 0.5 {
		item.Tier = AdaptiveWarmTier
	} else if item.Tier == AdaptiveWarmTier && promotionScore > 1.0 {
		item.Tier = AdaptiveHotTier
	}
}

// updateAccessPattern updates the access pattern for prediction
func (ac *AdaptiveCache) updateAccessPattern(cid string) {
	pattern, exists := ac.accessHistory[cid]
	if !exists {
		ac.initializeAccessPattern(cid)
		pattern = ac.accessHistory[cid]
	}
	
	now := time.Now()
	pattern.AccessTimes = append(pattern.AccessTimes, now)
	pattern.AccessCount++
	pattern.LastAccess = now
	
	// Calculate interval if we have previous access
	if len(pattern.AccessTimes) > 1 {
		lastAccess := pattern.AccessTimes[len(pattern.AccessTimes)-2]
		interval := now.Sub(lastAccess)
		pattern.AccessIntervals = append(pattern.AccessIntervals, interval)
		
		// Add training example for ML
		if ac.predictor != nil && ac.predictor.engine != nil {
			if item, exists := ac.items[cid]; exists {
				metadata := map[string]interface{}{
					"is_randomizer": item.IsRandomizer,
					"block_type":    item.BlockType,
				}
				// This block was accessed, so it's a positive example
				ac.predictor.engine.AddTrainingExample(pattern, true, metadata)
			}
		}
	}
	
	// Update daily and weekly patterns
	hour := now.Hour()
	weekday := int(now.Weekday())
	pattern.DailyPattern[hour]++
	pattern.WeeklyPattern[weekday]++
	
	// Limit history size
	if len(pattern.AccessTimes) > 1000 {
		pattern.AccessTimes = pattern.AccessTimes[100:] // Keep recent 900 entries
		pattern.AccessIntervals = pattern.AccessIntervals[100:]
	}
}

// initializeAccessPattern creates a new access pattern for a CID
func (ac *AdaptiveCache) initializeAccessPattern(cid string) {
	now := time.Now()
	pattern := &AdaptiveAccessPattern{
		CID:         cid,
		AccessTimes: make([]time.Time, 0),
		AccessIntervals: make([]time.Duration, 0),
		DailyPattern: [24]int{},
		WeeklyPattern: [7]int{},
		FirstAccess: now,
		LastAccess:  now,
		AccessCount: 0,
	}
	ac.accessHistory[cid] = pattern
}

// recordHit records a cache hit
func (ac *AdaptiveCache) recordHit(cid string, tier AdaptiveCacheTier) {
	ac.stats.mutex.Lock()
	defer ac.stats.mutex.Unlock()
	
	ac.stats.Hits++
	ac.stats.TotalRequests++
	
	switch tier {
	case AdaptiveHotTier:
		ac.stats.HotTierHits++
	case AdaptiveWarmTier:
		ac.stats.WarmTierHits++
	case AdaptiveColdTier:
		ac.stats.ColdTierHits++
	}
	
	ac.updateHitRate()
}

// recordMiss records a cache miss
func (ac *AdaptiveCache) recordMiss(cid string) {
	ac.stats.mutex.Lock()
	defer ac.stats.mutex.Unlock()
	
	ac.stats.Misses++
	ac.stats.TotalRequests++
	ac.updateHitRate()
}

// updateHitRate recalculates the hit rate
func (ac *AdaptiveCache) updateHitRate() {
	if ac.stats.TotalRequests > 0 {
		ac.stats.HitRate = float64(ac.stats.Hits) / float64(ac.stats.TotalRequests)
	}
}

// predictionLoop runs periodic prediction updates
func (ac *AdaptiveCache) predictionLoop() {
	ticker := time.NewTicker(ac.config.PredictionInterval)
	defer ticker.Stop()
	
	for range ticker.C {
		ac.updatePredictions()
		
		// Trigger ML model training periodically
		if ac.predictor != nil && ac.predictor.engine != nil {
			go ac.predictor.engine.Train()
		}
	}
}

// updatePredictions updates ML predictions for all cached items
func (ac *AdaptiveCache) updatePredictions() {
	ac.mutex.RLock()
	defer ac.mutex.RUnlock()

	if ac.predictor == nil || ac.predictor.engine == nil {
		return
	}

	for cid, item := range ac.items {
		pattern := ac.accessHistory[cid]
		if pattern != nil {
			// Update access pattern data
			pattern.AccessCount = item.AccessCount
			pattern.LastAccess = item.LastAccessed
			if len(pattern.AccessTimes) == 0 && !item.CreatedAt.IsZero() {
				pattern.FirstAccess = item.CreatedAt
			}
			
			// Get metadata for prediction
			metadata := map[string]interface{}{
				"is_randomizer": item.IsRandomizer,
				"block_type":    item.BlockType,
			}
			
			// Get ML prediction
			_ = metadata // Mark as used
			prediction := ac.predictor.engine.PredictNextAccess(pattern)
			
			item.mutex.Lock()
			item.PredictedValue = prediction
			item.mutex.Unlock()
			
			// Update last prediction time
			pattern.LastPrediction = time.Now()
			pattern.PredictionAccuracy = prediction
		}
	}
}

// evictionLoop runs periodic eviction checks
func (ac *AdaptiveCache) evictionLoop() {
	ticker := time.NewTicker(time.Minute * 5) // Check every 5 minutes
	defer ticker.Stop()
	
	for range ticker.C {
		ac.performMaintenance()
	}
}

// performMaintenance performs cache maintenance tasks
func (ac *AdaptiveCache) performMaintenance() {
	ac.mutex.Lock()
	defer ac.mutex.Unlock()
	
	// Check if proactive eviction is needed
	utilizationRatio := float64(ac.currentSize) / float64(ac.maxSize)
	if utilizationRatio > 0.8 { // 80% threshold
		spaceToFree := int64(float64(ac.maxSize) * 0.1) // Free 10%
		ac.makeSpace(spaceToFree)
	}
	
	// Update popularity scores with privacy protection
	ac.updatePopularityScoresPrivate()
	
	// Clean old access patterns
	ac.cleanOldAccessPatterns()
}

// updatePopularityScores recalculates popularity scores for all items
func (ac *AdaptiveCache) updatePopularityScores() {
	now := time.Now()
	
	for _, item := range ac.items {
		item.mutex.Lock()
		
		// Calculate popularity based on access frequency and recency
		timeSinceCreation := now.Sub(item.CreatedAt)
		timeSinceLastAccess := now.Sub(item.LastAccessed)
		
		accessRate := float64(item.AccessCount) / math.Max(timeSinceCreation.Hours(), 1.0)
		recencyFactor := 1.0 / (1.0 + timeSinceLastAccess.Hours())
		
		// Bonus for randomizer blocks
		randomizerBonus := 1.0
		if item.IsRandomizer {
			randomizerBonus = 1.5
		}
		
		item.PopularityScore = accessRate * recencyFactor * randomizerBonus
		
		item.mutex.Unlock()
	}
}

// cleanOldAccessPatterns removes old access pattern data
func (ac *AdaptiveCache) cleanOldAccessPatterns() {
	cutoff := time.Now().Add(-24 * time.Hour)
	
	for cid, pattern := range ac.accessHistory {
		if len(pattern.AccessTimes) == 0 {
			continue
		}
		
		lastAccess := pattern.AccessTimes[len(pattern.AccessTimes)-1]
		if lastAccess.Before(cutoff) {
			delete(ac.accessHistory, cid)
		}
	}
}

// cacheExchangeLoop handles cache state exchange with peers
func (ac *AdaptiveCache) cacheExchangeLoop() {
	ticker := time.NewTicker(ac.config.ExchangeInterval)
	defer ticker.Stop()
	
	for range ticker.C {
		ac.exchangeCacheState()
	}
}

// exchangeCacheState exchanges cache state with other peers
func (ac *AdaptiveCache) exchangeCacheState() {
	// This would integrate with the peer manager to exchange cache information
	// For now, we'll implement a placeholder
	
	ac.cacheExchange.mutex.Lock()
	defer ac.cacheExchange.mutex.Unlock()
	
	// Update last exchange time
	ac.cacheExchange.lastExchange = time.Now()
	
	// In a real implementation, this would:
	// 1. Get list of connected peers
	// 2. Exchange cache state summaries
	// 3. Coordinate cache placement for popular blocks
	// 4. Implement cache warming based on peer recommendations
}

// GetStats returns current cache statistics
func (ac *AdaptiveCache) GetStats() *AdaptiveCacheStats {
	ac.stats.mutex.RLock()
	defer ac.stats.mutex.RUnlock()
	
	// Create a copy to avoid race conditions
	statsCopy := *ac.stats
	return &statsCopy
}

// GetTierStats returns statistics by cache tier
func (ac *AdaptiveCache) GetTierStats() map[AdaptiveCacheTier]map[string]interface{} {
	ac.mutex.RLock()
	defer ac.mutex.RUnlock()
	
	tierStats := make(map[AdaptiveCacheTier]map[string]interface{})
	
	for tier := AdaptiveHotTier; tier <= AdaptiveColdTier; tier++ {
		count := 0
		totalSize := int64(0)
		totalAccesses := int64(0)
		
		for _, item := range ac.items {
			item.mutex.RLock()
			if item.Tier == tier {
				count++
				totalSize += item.Size
				totalAccesses += item.AccessCount
			}
			item.mutex.RUnlock()
		}
		
		tierStats[tier] = map[string]interface{}{
			"item_count":    count,
			"total_size":    totalSize,
			"total_accesses": totalAccesses,
		}
	}
	
	return tierStats
}

// SetEvictionPolicy changes the eviction policy
func (ac *AdaptiveCache) SetEvictionPolicy(policy AdaptiveEvictionPolicy) {
	ac.mutex.Lock()
	defer ac.mutex.Unlock()
	ac.evictionPolicy = policy
}

// GetCacheUtilization returns current cache utilization
func (ac *AdaptiveCache) GetCacheUtilization() map[string]interface{} {
	ac.mutex.RLock()
	defer ac.mutex.RUnlock()
	
	return map[string]interface{}{
		"current_size":    ac.currentSize,
		"max_size":        ac.maxSize,
		"utilization":     float64(ac.currentSize) / float64(ac.maxSize),
		"item_count":      len(ac.items),
		"max_items":       ac.maxItems,
		"item_utilization": float64(len(ac.items)) / float64(ac.maxItems),
	}
}

// Preload attempts to preload predicted blocks based on access patterns
func (ac *AdaptiveCache) Preload(ctx context.Context, blockFetcher func(string) ([]byte, error)) error {
	// Get predictions from ML model
	predictions := ac.getPredictedBlocks(10) // Get top 10 predictions
	
	if len(predictions) == 0 {
		return nil // No predictions available
	}

	fmt.Printf("Preloading %d predicted blocks based on ML analysis...\n", len(predictions))
	
	// Track preload performance
	preloaded := 0
	errors := 0

	for _, prediction := range predictions {
		// Check if already cached
		if _, exists := ac.items[prediction.CID]; exists {
			continue
		}
		
		// Check if we have space (use 80% threshold for preloading)
		if float64(ac.currentSize)/float64(ac.maxSize) > 0.8 {
			fmt.Printf("Cache utilization at %.1f%%, stopping preload\n", 
				float64(ac.currentSize)/float64(ac.maxSize)*100)
			break
		}
		
		// Only preload high-confidence predictions
		if prediction.Score < 0.7 {
			continue
		}
		
		// Fetch and cache the block asynchronously
		go func(pred *PredictionResult) {
			data, err := blockFetcher(pred.CID)
			if err == nil {
				metadata := map[string]interface{}{
					"preloaded": true,
					"prediction_score": pred.Score,
				}
				ac.Put(pred.CID, data, metadata)
				preloaded++
			} else {
				errors++
			}
		}(prediction)
	}
	
	// Update prediction accuracy tracking
	go ac.updatePredictionAccuracy()
	
	return nil
}

// addDifferentialPrivacyNoise adds calibrated noise for differential privacy
func (ac *AdaptiveCache) addDifferentialPrivacyNoise(trueValue float64) float64 {
	if ac.config.PrivacyEpsilon <= 0 {
		return trueValue // No privacy protection
	}
	
	// Laplace mechanism for differential privacy
	sensitivity := 1.0 // Sensitivity of popularity counting
	scale := sensitivity / ac.config.PrivacyEpsilon
	
	// Generate Laplace noise: Lap(0, scale)
	u := ac.privacyNoise.Float64() - 0.5
	noise := -scale * math.Copysign(math.Log(1-2*math.Abs(u)), u)
	
	return math.Max(0, trueValue + noise) // Ensure non-negative
}

// quantizeTimestamp rounds timestamp to privacy quantum intervals
func (ac *AdaptiveCache) quantizeTimestamp(t time.Time) time.Time {
	if ac.config.TemporalQuantum <= 0 {
		return t // No quantization
	}
	
	// Round down to nearest quantum boundary
	quantumNanos := ac.config.TemporalQuantum.Nanoseconds()
	quantized := (t.UnixNano() / quantumNanos) * quantumNanos
	return time.Unix(0, quantized)
}

// updateAccessPatternPrivate updates the access pattern with privacy protections
func (ac *AdaptiveCache) updateAccessPatternPrivate(cid string) {
	pattern, exists := ac.accessHistory[cid]
	if !exists {
		ac.initializeAccessPattern(cid)
		pattern = ac.accessHistory[cid]
	}
	
	// Use quantized timestamp for privacy
	now := ac.quantizeTimestamp(time.Now())
	pattern.AccessTimes = append(pattern.AccessTimes, now)
	
	// Calculate interval with quantized times
	if len(pattern.AccessTimes) > 1 {
		lastAccess := pattern.AccessTimes[len(pattern.AccessTimes)-2]
		interval := now.Sub(lastAccess)
		pattern.AccessIntervals = append(pattern.AccessIntervals, interval)
	}
	
	// Update daily and weekly patterns
	hour := now.Hour()
	weekday := int(now.Weekday())
	pattern.DailyPattern[hour]++
	pattern.WeeklyPattern[weekday]++
	
	// Limit history size
	if len(pattern.AccessTimes) > 1000 {
		pattern.AccessTimes = pattern.AccessTimes[100:]
		pattern.AccessIntervals = pattern.AccessIntervals[100:]
	}
}

// updatePopularityScoresPrivate recalculates popularity with differential privacy
func (ac *AdaptiveCache) updatePopularityScoresPrivate() {
	now := time.Now()
	
	for _, item := range ac.items {
		item.mutex.Lock()
		
		// Calculate base popularity
		timeSinceCreation := now.Sub(item.CreatedAt)
		timeSinceLastAccess := now.Sub(item.LastAccessed)
		
		accessRate := float64(item.AccessCount) / math.Max(timeSinceCreation.Hours(), 1.0)
		recencyFactor := 1.0 / (1.0 + timeSinceLastAccess.Hours())
		
		randomizerBonus := 1.0
		if item.IsRandomizer {
			randomizerBonus = 1.5
		}
		
		basePopularity := accessRate * recencyFactor * randomizerBonus
		
		// Apply differential privacy noise
		item.PopularityScore = ac.addDifferentialPrivacyNoise(basePopularity)
		
		item.mutex.Unlock()
	}
}

// injectDummyAccess creates fake cache accesses for privacy
func (ac *AdaptiveCache) injectDummyAccess() {
	if ac.config.DummyAccessRate <= 0 || len(ac.items) == 0 {
		return
	}
	
	// Check if it's time for a dummy access
	timeSinceLastDummy := time.Since(ac.lastDummyAccess)
	averageInterval := time.Duration(float64(time.Hour) / ac.config.DummyAccessRate)
	
	if timeSinceLastDummy < averageInterval {
		return
	}
	
	// Select a random cached item for dummy access
	var items []*AdaptiveCacheItem
	ac.mutex.RLock()
	for _, item := range ac.items {
		items = append(items, item)
	}
	ac.mutex.RUnlock()
	
	if len(items) > 0 {
		randomItem := items[ac.privacyNoise.Intn(len(items))]
		
		// Perform dummy access without returning data
		randomItem.mutex.Lock()
		randomItem.LastAccessed = ac.quantizeTimestamp(time.Now())
		randomItem.AccessCount++
		randomItem.mutex.Unlock()
		
		// Update access pattern for the dummy access
		ac.updateAccessPatternPrivate(randomItem.CID)
		
		ac.lastDummyAccess = time.Now()
	}
}

// privacyMaintenanceLoop handles privacy-related maintenance tasks
func (ac *AdaptiveCache) privacyMaintenanceLoop() {
	ticker := time.NewTicker(time.Minute) // Check every minute
	defer ticker.Stop()
	
	for range ticker.C {
		ac.injectDummyAccess()
		
		// Periodically update popularity with privacy noise
		if time.Now().Minute()%5 == 0 { // Every 5 minutes
			ac.mutex.Lock()
			ac.updatePopularityScoresPrivate()
			ac.mutex.Unlock()
		}
	}
}

// getPredictedBlocks returns top predicted blocks for preloading
func (ac *AdaptiveCache) getPredictedBlocks(count int) []*PredictionResult {
	ac.mutex.RLock()
	defer ac.mutex.RUnlock()
	
	if ac.predictor == nil || ac.predictor.engine == nil {
		return nil
	}
	
	// Get predictions from ML engine
	predictions := ac.predictor.engine.GetTopPredictions(count)
	
	// Filter out already cached blocks
	filtered := make([]*PredictionResult, 0, len(predictions))
	for _, pred := range predictions {
		if _, exists := ac.items[pred.CID]; !exists {
			filtered = append(filtered, pred)
		}
	}
	
	return filtered
}

// updatePredictionAccuracy tracks how well predictions performed
func (ac *AdaptiveCache) updatePredictionAccuracy() {
	ac.mutex.Lock()
	defer ac.mutex.Unlock()
	
	if ac.predictor == nil || ac.predictor.engine == nil {
		return
	}
	
	// Track prediction hits
	predictionHits := 0
	totalPredictions := 0
	
	// Check recent predictions
	for _, pattern := range ac.accessHistory {
		if pattern.LastPrediction.IsZero() {
			continue
		}
		
		totalPredictions++
		
		// Check if the block was accessed within prediction window
		if len(pattern.AccessTimes) > 0 {
			lastAccess := pattern.AccessTimes[len(pattern.AccessTimes)-1]
			if lastAccess.After(pattern.LastPrediction) && 
			   lastAccess.Before(pattern.LastPrediction.Add(ac.config.PredictionWindow)) {
				predictionHits++
			}
		}
	}
	
	// Update statistics
	ac.stats.mutex.Lock()
	ac.stats.PredictionHits += int64(predictionHits)
	ac.stats.PredictionTotal += int64(totalPredictions)
	if ac.stats.PredictionTotal > 0 {
		ac.stats.PredictionAccuracy = float64(ac.stats.PredictionHits) / float64(ac.stats.PredictionTotal)
	}
	ac.stats.mutex.Unlock()
	
	// Train ML model with new data
	for cid, pattern := range ac.accessHistory {
		if item, exists := ac.items[cid]; exists {
			// Add training example
			wasAccessed := item.AccessCount > 0
			metadata := map[string]interface{}{
				"is_randomizer": item.IsRandomizer,
				"block_type":    item.BlockType,
			}
			ac.predictor.engine.AddTrainingExample(pattern, wasAccessed, metadata)
		}
	}
}
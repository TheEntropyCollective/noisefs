package cache

import (
	"math"
	"sort"
	"time"
)

// MLEvictionPolicy implements ML-based eviction decisions
type MLEvictionPolicy struct {
	cache *AdaptiveCache
}

// LRUEvictionPolicy implements Least Recently Used eviction
type LRUEvictionPolicy struct{}

// LFUEvictionPolicy implements Least Frequently Used eviction
type LFUEvictionPolicy struct{}

// RandomizerAwareEvictionPolicy prioritizes keeping randomizer blocks
type RandomizerAwareEvictionPolicy struct{}

// NewMLEvictionPolicy creates a new ML-based eviction policy
func NewMLEvictionPolicy(cache *AdaptiveCache) *MLEvictionPolicy {
	return &MLEvictionPolicy{cache: cache}
}

// ShouldEvict determines if an item should be evicted
func (mep *MLEvictionPolicy) ShouldEvict(item *AdaptiveCacheItem, cache *AdaptiveCache) bool {
	item.mutex.RLock()
	defer item.mutex.RUnlock()
	
	// Never evict hot tier items unless absolutely necessary
	if item.Tier == AdaptiveHotTier {
		return false
	}
	
	// Consider ML prediction score
	if item.PredictedValue > 0.7 {
		return false // High probability of future access
	}
	
	// Consider if it's a randomizer block
	if item.IsRandomizer && item.RandomizerUse > 5 {
		return false // Frequently used randomizer
	}
	
	// Check recency and frequency
	timeSinceAccess := time.Since(item.LastAccessed)
	if timeSinceAccess < time.Hour && item.AccessCount > 3 {
		return false // Recently accessed and frequently used
	}
	
	// Cold tier items with low scores are candidates
	return item.Tier == AdaptiveColdTier && item.PopularityScore < 0.1
}

// SelectEvictionCandidates selects items for eviction
func (mep *MLEvictionPolicy) SelectEvictionCandidates(cache *AdaptiveCache, spaceNeeded int64) []*AdaptiveCacheItem {
	cache.mutex.RLock()
	defer cache.mutex.RUnlock()
	
	var candidates []*AdaptiveCacheItem
	
	// Collect eviction candidates with their priorities
	type candidateScore struct {
		item     *AdaptiveCacheItem
		priority float64
	}
	
	var scoredCandidates []candidateScore
	
	for _, item := range cache.items {
		if mep.ShouldEvict(item, cache) {
			priority := mep.GetPriority(item)
			scoredCandidates = append(scoredCandidates, candidateScore{
				item:     item,
				priority: priority,
			})
		}
	}
	
	// Sort by priority (lower = higher priority for eviction)
	sort.Slice(scoredCandidates, func(i, j int) bool {
		return scoredCandidates[i].priority < scoredCandidates[j].priority
	})
	
	// Select candidates until we have enough space
	spaceToFree := int64(0)
	for _, scored := range scoredCandidates {
		candidates = append(candidates, scored.item)
		spaceToFree += scored.item.Size
		
		if spaceToFree >= spaceNeeded {
			break
		}
	}
	
	return candidates
}

// GetPriority calculates eviction priority (lower = higher priority for eviction)
func (mep *MLEvictionPolicy) GetPriority(item *AdaptiveCacheItem) float64 {
	item.mutex.RLock()
	defer item.mutex.RUnlock()
	
	// Base priority
	priority := 1.0
	
	// ML prediction weight (higher prediction = lower eviction priority)
	priority *= (1.0 - item.PredictedValue)
	
	// Popularity weight
	priority *= (1.0 - item.PopularityScore)
	
	// Recency weight
	timeSinceAccess := time.Since(item.LastAccessed)
	recencyFactor := math.Min(timeSinceAccess.Hours()/24.0, 1.0) // Normalize to days
	priority *= (1.0 + recencyFactor)
	
	// Tier weight (cold tier has higher eviction priority)
	switch item.Tier {
	case AdaptiveHotTier:
		priority *= 0.1 // Very low eviction priority
	case AdaptiveWarmTier:
		priority *= 0.5
	case AdaptiveColdTier:
		priority *= 1.0
	}
	
	// Randomizer bonus (lower eviction priority)
	if item.IsRandomizer {
		priority *= 0.3
	}
	
	// Size factor (larger items have slightly higher eviction priority)
	sizeFactor := 1.0 + (float64(item.Size)/(1024*1024))*0.01 // Small bonus for large items
	priority *= sizeFactor
	
	return priority
}

// NewLRUEvictionPolicy creates a new LRU eviction policy
func NewLRUEvictionPolicy() *LRUEvictionPolicy {
	return &LRUEvictionPolicy{}
}

// ShouldEvict for LRU policy
func (lru *LRUEvictionPolicy) ShouldEvict(item *AdaptiveCacheItem, cache *AdaptiveCache) bool {
	item.mutex.RLock()
	defer item.mutex.RUnlock()
	
	// Simple time-based eviction
	return time.Since(item.LastAccessed) > time.Hour
}

// SelectEvictionCandidates for LRU policy
func (lru *LRUEvictionPolicy) SelectEvictionCandidates(cache *AdaptiveCache, spaceNeeded int64) []*AdaptiveCacheItem {
	cache.mutex.RLock()
	defer cache.mutex.RUnlock()
	
	// Collect all items with their last access times
	type itemTime struct {
		item       *AdaptiveCacheItem
		lastAccess time.Time
	}
	
	var items []itemTime
	for _, item := range cache.items {
		item.mutex.RLock()
		items = append(items, itemTime{
			item:       item,
			lastAccess: item.LastAccessed,
		})
		item.mutex.RUnlock()
	}
	
	// Sort by last access time (oldest first)
	sort.Slice(items, func(i, j int) bool {
		return items[i].lastAccess.Before(items[j].lastAccess)
	})
	
	// Select oldest items until we have enough space
	var candidates []*AdaptiveCacheItem
	spaceToFree := int64(0)
	
	for _, item := range items {
		candidates = append(candidates, item.item)
		spaceToFree += item.item.Size
		
		if spaceToFree >= spaceNeeded {
			break
		}
	}
	
	return candidates
}

// GetPriority for LRU policy
func (lru *LRUEvictionPolicy) GetPriority(item *AdaptiveCacheItem) float64 {
	item.mutex.RLock()
	defer item.mutex.RUnlock()
	
	// Priority based on last access time (older = higher priority for eviction)
	timeSinceAccess := time.Since(item.LastAccessed)
	return timeSinceAccess.Hours()
}

// NewLFUEvictionPolicy creates a new LFU eviction policy
func NewLFUEvictionPolicy() *LFUEvictionPolicy {
	return &LFUEvictionPolicy{}
}

// ShouldEvict for LFU policy
func (lfu *LFUEvictionPolicy) ShouldEvict(item *AdaptiveCacheItem, cache *AdaptiveCache) bool {
	item.mutex.RLock()
	defer item.mutex.RUnlock()
	
	// Evict items with low access count relative to their age
	timeSinceCreation := time.Since(item.CreatedAt)
	accessRate := float64(item.AccessCount) / math.Max(timeSinceCreation.Hours(), 1.0)
	
	return accessRate < 0.1 // Less than 0.1 accesses per hour
}

// SelectEvictionCandidates for LFU policy
func (lfu *LFUEvictionPolicy) SelectEvictionCandidates(cache *AdaptiveCache, spaceNeeded int64) []*AdaptiveCacheItem {
	cache.mutex.RLock()
	defer cache.mutex.RUnlock()
	
	// Collect all items with their access frequencies
	type itemFreq struct {
		item      *AdaptiveCacheItem
		frequency float64
	}
	
	var items []itemFreq
	for _, item := range cache.items {
		item.mutex.RLock()
		timeSinceCreation := time.Since(item.CreatedAt)
		frequency := float64(item.AccessCount) / math.Max(timeSinceCreation.Hours(), 1.0)
		items = append(items, itemFreq{
			item:      item,
			frequency: frequency,
		})
		item.mutex.RUnlock()
	}
	
	// Sort by frequency (lowest first)
	sort.Slice(items, func(i, j int) bool {
		return items[i].frequency < items[j].frequency
	})
	
	// Select least frequent items until we have enough space
	var candidates []*AdaptiveCacheItem
	spaceToFree := int64(0)
	
	for _, item := range items {
		candidates = append(candidates, item.item)
		spaceToFree += item.item.Size
		
		if spaceToFree >= spaceNeeded {
			break
		}
	}
	
	return candidates
}

// GetPriority for LFU policy
func (lfu *LFUEvictionPolicy) GetPriority(item *AdaptiveCacheItem) float64 {
	item.mutex.RLock()
	defer item.mutex.RUnlock()
	
	// Priority based on access frequency (lower frequency = higher priority for eviction)
	timeSinceCreation := time.Since(item.CreatedAt)
	frequency := float64(item.AccessCount) / math.Max(timeSinceCreation.Hours(), 1.0)
	
	return 1.0 / (1.0 + frequency) // Invert so lower frequency = higher priority
}

// NewRandomizerAwareEvictionPolicy creates a new randomizer-aware eviction policy
func NewRandomizerAwareEvictionPolicy() *RandomizerAwareEvictionPolicy {
	return &RandomizerAwareEvictionPolicy{}
}

// ShouldEvict for randomizer-aware policy
func (rae *RandomizerAwareEvictionPolicy) ShouldEvict(item *AdaptiveCacheItem, cache *AdaptiveCache) bool {
	item.mutex.RLock()
	defer item.mutex.RUnlock()
	
	// Never evict frequently used randomizer blocks
	if item.IsRandomizer && item.RandomizerUse > 3 {
		return false
	}
	
	// Standard eviction criteria for non-randomizers
	if !item.IsRandomizer {
		timeSinceAccess := time.Since(item.LastAccessed)
		return timeSinceAccess > time.Hour && item.AccessCount < 5
	}
	
	// For randomizers, be more conservative
	timeSinceAccess := time.Since(item.LastAccessed)
	return timeSinceAccess > 6*time.Hour && item.RandomizerUse < 2
}

// SelectEvictionCandidates for randomizer-aware policy
func (rae *RandomizerAwareEvictionPolicy) SelectEvictionCandidates(cache *AdaptiveCache, spaceNeeded int64) []*AdaptiveCacheItem {
	cache.mutex.RLock()
	defer cache.mutex.RUnlock()
	
	// Separate randomizers and non-randomizers
	var nonRandomizers []*AdaptiveCacheItem
	var randomizers []*AdaptiveCacheItem
	
	for _, item := range cache.items {
		item.mutex.RLock()
		if item.IsRandomizer {
			randomizers = append(randomizers, item)
		} else {
			nonRandomizers = append(nonRandomizers, item)
		}
		item.mutex.RUnlock()
	}
	
	// Sort non-randomizers by last access (oldest first)
	sort.Slice(nonRandomizers, func(i, j int) bool {
		nonRandomizers[i].mutex.RLock()
		nonRandomizers[j].mutex.RLock()
		defer nonRandomizers[i].mutex.RUnlock()
		defer nonRandomizers[j].mutex.RUnlock()
		
		return nonRandomizers[i].LastAccessed.Before(nonRandomizers[j].LastAccessed)
	})
	
	// Sort randomizers by usage (least used first)
	sort.Slice(randomizers, func(i, j int) bool {
		randomizers[i].mutex.RLock()
		randomizers[j].mutex.RLock()
		defer randomizers[i].mutex.RUnlock()
		defer randomizers[j].mutex.RUnlock()
		
		return randomizers[i].RandomizerUse < randomizers[j].RandomizerUse
	})
	
	// Select candidates: prefer non-randomizers first
	var candidates []*AdaptiveCacheItem
	spaceToFree := int64(0)
	
	// First, try non-randomizers
	for _, item := range nonRandomizers {
		if rae.ShouldEvict(item, cache) {
			candidates = append(candidates, item)
			spaceToFree += item.Size
			
			if spaceToFree >= spaceNeeded {
				return candidates
			}
		}
	}
	
	// If not enough space, consider randomizers
	for _, item := range randomizers {
		if rae.ShouldEvict(item, cache) {
			candidates = append(candidates, item)
			spaceToFree += item.Size
			
			if spaceToFree >= spaceNeeded {
				break
			}
		}
	}
	
	return candidates
}

// GetPriority for randomizer-aware policy
func (rae *RandomizerAwareEvictionPolicy) GetPriority(item *AdaptiveCacheItem) float64 {
	item.mutex.RLock()
	defer item.mutex.RUnlock()
	
	// Base priority
	priority := 1.0
	
	// Randomizer bonus (much lower eviction priority)
	if item.IsRandomizer {
		priority *= 0.1
		
		// Further reduce priority based on randomizer usage
		usageBonus := 1.0 / (1.0 + float64(item.RandomizerUse))
		priority *= usageBonus
	}
	
	// Standard recency factor
	timeSinceAccess := time.Since(item.LastAccessed)
	recencyFactor := math.Min(timeSinceAccess.Hours()/24.0, 1.0)
	priority *= (1.0 + recencyFactor)
	
	// Access frequency factor
	timeSinceCreation := time.Since(item.CreatedAt)
	accessRate := float64(item.AccessCount) / math.Max(timeSinceCreation.Hours(), 1.0)
	priority *= (1.0 / (1.0 + accessRate))
	
	return priority
}

// HybridEvictionPolicy combines multiple eviction strategies
type HybridEvictionPolicy struct {
	policies []AdaptiveEvictionPolicy
	weights  []float64
}

// NewHybridEvictionPolicy creates a new hybrid eviction policy
func NewHybridEvictionPolicy(policies []AdaptiveEvictionPolicy, weights []float64) *HybridEvictionPolicy {
	if len(policies) != len(weights) {
		panic("policies and weights must have the same length")
	}
	
	// Normalize weights
	totalWeight := 0.0
	for _, weight := range weights {
		totalWeight += weight
	}
	
	normalizedWeights := make([]float64, len(weights))
	for i, weight := range weights {
		normalizedWeights[i] = weight / totalWeight
	}
	
	return &HybridEvictionPolicy{
		policies: policies,
		weights:  normalizedWeights,
	}
}

// ShouldEvict for hybrid policy
func (hep *HybridEvictionPolicy) ShouldEvict(item *AdaptiveCacheItem, cache *AdaptiveCache) bool {
	// Use majority voting
	votes := 0
	for _, policy := range hep.policies {
		if policy.ShouldEvict(item, cache) {
			votes++
		}
	}
	
	return votes > len(hep.policies)/2
}

// SelectEvictionCandidates for hybrid policy
func (hep *HybridEvictionPolicy) SelectEvictionCandidates(cache *AdaptiveCache, spaceNeeded int64) []*AdaptiveCacheItem {
	cache.mutex.RLock()
	defer cache.mutex.RUnlock()
	
	// Collect weighted priorities from all policies
	type itemScore struct {
		item  *AdaptiveCacheItem
		score float64
	}
	
	var scoredItems []itemScore
	
	for _, item := range cache.items {
		if hep.ShouldEvict(item, cache) {
			// Calculate weighted average priority
			totalScore := 0.0
			for i, policy := range hep.policies {
				priority := policy.GetPriority(item)
				totalScore += priority * hep.weights[i]
			}
			
			scoredItems = append(scoredItems, itemScore{
				item:  item,
				score: totalScore,
			})
		}
	}
	
	// Sort by score (higher score = higher priority for eviction)
	sort.Slice(scoredItems, func(i, j int) bool {
		return scoredItems[i].score > scoredItems[j].score
	})
	
	// Select candidates until we have enough space
	var candidates []*AdaptiveCacheItem
	spaceToFree := int64(0)
	
	for _, scored := range scoredItems {
		candidates = append(candidates, scored.item)
		spaceToFree += scored.item.Size
		
		if spaceToFree >= spaceNeeded {
			break
		}
	}
	
	return candidates
}

// GetPriority for hybrid policy
func (hep *HybridEvictionPolicy) GetPriority(item *AdaptiveCacheItem) float64 {
	// Calculate weighted average priority
	totalScore := 0.0
	for i, policy := range hep.policies {
		priority := policy.GetPriority(item)
		totalScore += priority * hep.weights[i]
	}
	
	return totalScore
}
package cache

import (
	"sort"
	"time"
)

// RandomizerAwareEvictionPolicy implements an eviction policy that prioritizes
// keeping randomizer blocks in cache due to their high reuse potential
type RandomizerAwareEvictionPolicy struct {
	// Weight factors for scoring
	randomizerWeight   float64
	accessWeight       float64
	recencyWeight      float64
	predictionWeight   float64
}

// NewRandomizerAwareEvictionPolicy creates a new randomizer-aware eviction policy
func NewRandomizerAwareEvictionPolicy() *RandomizerAwareEvictionPolicy {
	return &RandomizerAwareEvictionPolicy{
		randomizerWeight: 3.0,  // Strong preference for keeping randomizers
		accessWeight:     2.0,  // Frequency of access
		recencyWeight:    1.0,  // Recent access
		predictionWeight: 1.5,  // ML prediction value
	}
}

// ShouldEvict determines if a specific item should be evicted
func (p *RandomizerAwareEvictionPolicy) ShouldEvict(item *AdaptiveCacheItem, cache *AdaptiveCache) bool {
	// Never evict hot tier randomizers
	if item.IsRandomizer && item.Tier == AdaptiveHotTier {
		return false
	}
	
	// Calculate retention score
	score := p.GetPriority(item)
	
	// Items with very low scores should be evicted
	// Threshold depends on cache pressure
	utilizationRatio := float64(cache.currentSize) / float64(cache.maxSize)
	threshold := 0.2 * (1.0 + utilizationRatio) // Dynamic threshold
	
	return score < threshold
}

// SelectEvictionCandidates selects items to evict to free up space
func (p *RandomizerAwareEvictionPolicy) SelectEvictionCandidates(cache *AdaptiveCache, spaceNeeded int64) []*AdaptiveCacheItem {
	// Create a scored list of all items
	type scoredItem struct {
		item  *AdaptiveCacheItem
		score float64
	}
	
	scoredItems := make([]scoredItem, 0, len(cache.items))
	
	// Score all items
	for _, item := range cache.items {
		score := p.GetPriority(item)
		scoredItems = append(scoredItems, scoredItem{
			item:  item,
			score: score,
		})
	}
	
	// Sort by score (lowest first - these are eviction candidates)
	sort.Slice(scoredItems, func(i, j int) bool {
		return scoredItems[i].score < scoredItems[j].score
	})
	
	// Select items until we have enough space
	candidates := make([]*AdaptiveCacheItem, 0)
	freedSpace := int64(0)
	
	for _, scored := range scoredItems {
		if freedSpace >= spaceNeeded {
			break
		}
		
		// Skip hot tier randomizers completely
		if scored.item.IsRandomizer && scored.item.Tier == AdaptiveHotTier {
			continue
		}
		
		candidates = append(candidates, scored.item)
		freedSpace += scored.item.Size
	}
	
	return candidates
}

// GetPriority calculates the retention priority for an item
// Higher score = less likely to be evicted
func (p *RandomizerAwareEvictionPolicy) GetPriority(item *AdaptiveCacheItem) float64 {
	now := time.Now()
	
	// Base score components
	var score float64
	
	// 1. Randomizer bonus - heavily weighted
	if item.IsRandomizer {
		score += p.randomizerWeight
		
		// Additional bonus based on randomizer usage
		if item.RandomizerUse > 0 {
			usageBonus := float64(item.RandomizerUse) / 100.0 // Normalize
			if usageBonus > 1.0 {
				usageBonus = 1.0
			}
			score += p.randomizerWeight * usageBonus
		}
	}
	
	// 2. Access frequency score
	timeSinceCreation := now.Sub(item.CreatedAt).Hours()
	if timeSinceCreation > 0 {
		accessRate := float64(item.AccessCount) / timeSinceCreation
		score += p.accessWeight * (accessRate / 10.0) // Normalize by 10 accesses/hour
	}
	
	// 3. Recency score
	timeSinceAccess := now.Sub(item.LastAccessed).Hours()
	recencyScore := 1.0 / (1.0 + timeSinceAccess/24.0) // Decay over days
	score += p.recencyWeight * recencyScore
	
	// 4. ML prediction score
	if item.PredictedValue > 0 {
		score += p.predictionWeight * item.PredictedValue
	}
	
	// 5. Tier multiplier
	switch item.Tier {
	case AdaptiveHotTier:
		score *= 2.0
	case AdaptiveWarmTier:
		score *= 1.5
	case AdaptiveColdTier:
		score *= 1.0
	}
	
	// 6. Special block type bonuses
	switch item.BlockType {
	case "descriptor":
		score *= 1.2 // Descriptors are important for file reconstruction
	case "index":
		score *= 1.1 // Index blocks help with navigation
	}
	
	return score
}

// SetWeights allows customization of scoring weights
func (p *RandomizerAwareEvictionPolicy) SetWeights(randomizer, access, recency, prediction float64) {
	p.randomizerWeight = randomizer
	p.accessWeight = access
	p.recencyWeight = recency
	p.predictionWeight = prediction
}

// GetStatistics returns eviction policy statistics
func (p *RandomizerAwareEvictionPolicy) GetStatistics(cache *AdaptiveCache) map[string]interface{} {
	stats := make(map[string]interface{})
	
	randomizerCount := 0
	totalRandomizerUse := int64(0)
	avgScore := 0.0
	
	for _, item := range cache.items {
		if item.IsRandomizer {
			randomizerCount++
			totalRandomizerUse += item.RandomizerUse
		}
		avgScore += p.GetPriority(item)
	}
	
	if len(cache.items) > 0 {
		avgScore /= float64(len(cache.items))
	}
	
	stats["randomizer_count"] = randomizerCount
	stats["total_randomizer_use"] = totalRandomizerUse
	stats["average_retention_score"] = avgScore
	stats["weights"] = map[string]float64{
		"randomizer": p.randomizerWeight,
		"access":     p.accessWeight,
		"recency":    p.recencyWeight,
		"prediction": p.predictionWeight,
	}
	
	return stats
}
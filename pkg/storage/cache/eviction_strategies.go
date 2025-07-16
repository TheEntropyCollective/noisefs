package cache

import (
	"math"
	"sort"
	"time"
)

// EvictionStrategy defines how to select blocks for eviction
type EvictionStrategy interface {
	// SelectEvictionCandidates returns blocks to evict to free the requested space
	SelectEvictionCandidates(blocks map[string]*BlockMetadata, spaceNeeded int64, healthTracker *BlockHealthTracker) []*BlockMetadata
	
	// Score calculates an eviction score for a block (higher = more likely to evict)
	Score(block *BlockMetadata, healthTracker *BlockHealthTracker) float64
	
	// Name returns the strategy name
	Name() string
}

// LRUEvictionStrategy evicts least recently used blocks
type LRUEvictionStrategy struct{}

func (s *LRUEvictionStrategy) Name() string {
	return "LRU"
}

func (s *LRUEvictionStrategy) Score(block *BlockMetadata, healthTracker *BlockHealthTracker) float64 {
	// Higher score = more likely to evict
	// Use hours since last access as score
	return time.Since(block.LastAccessed).Hours()
}

func (s *LRUEvictionStrategy) SelectEvictionCandidates(blocks map[string]*BlockMetadata, spaceNeeded int64, healthTracker *BlockHealthTracker) []*BlockMetadata {
	// Convert to slice for sorting
	candidates := make([]*BlockMetadata, 0, len(blocks))
	for _, block := range blocks {
		candidates = append(candidates, block)
	}
	
	// Sort by score (descending - oldest first)
	sort.Slice(candidates, func(i, j int) bool {
		return s.Score(candidates[i], healthTracker) > s.Score(candidates[j], healthTracker)
	})
	
	// Select blocks until we have enough space
	selected := make([]*BlockMetadata, 0)
	freedSpace := int64(0)
	
	for _, block := range candidates {
		if freedSpace >= spaceNeeded {
			break
		}
		selected = append(selected, block)
		freedSpace += int64(block.Size)
	}
	
	return selected
}

// LFUEvictionStrategy evicts least frequently used blocks
type LFUEvictionStrategy struct{}

func (s *LFUEvictionStrategy) Name() string {
	return "LFU"
}

func (s *LFUEvictionStrategy) Score(block *BlockMetadata, healthTracker *BlockHealthTracker) float64 {
	strategyName := s.Name()
	
	// Check for cached score first
	if cachedScore, valid := block.GetCachedScore(strategyName); valid {
		return cachedScore
	}
	
	// Calculate access frequency
	age := time.Since(block.CachedAt).Hours()
	if age < 1 {
		age = 1
	}
	
	// Inverse of access rate (lower frequency = higher score)
	accessRate := float64(block.Popularity) / age
	var score float64
	if accessRate == 0 {
		score = 1000.0 // Never accessed
	} else {
		score = 1.0 / accessRate
	}
	
	// Cache the calculated score
	block.SetCachedScore(strategyName, score)
	
	return score
}

func (s *LFUEvictionStrategy) SelectEvictionCandidates(blocks map[string]*BlockMetadata, spaceNeeded int64, healthTracker *BlockHealthTracker) []*BlockMetadata {
	candidates := make([]*BlockMetadata, 0, len(blocks))
	for _, block := range blocks {
		candidates = append(candidates, block)
	}
	
	sort.Slice(candidates, func(i, j int) bool {
		return s.Score(candidates[i], healthTracker) > s.Score(candidates[j], healthTracker)
	})
	
	selected := make([]*BlockMetadata, 0)
	freedSpace := int64(0)
	
	for _, block := range candidates {
		if freedSpace >= spaceNeeded {
			break
		}
		selected = append(selected, block)
		freedSpace += int64(block.Size)
	}
	
	return selected
}

// ValueBasedEvictionStrategy evicts blocks based on their network value
type ValueBasedEvictionStrategy struct {
	// Weight factors for different criteria
	AgeWeight        float64
	FrequencyWeight  float64
	HealthWeight     float64
	RandomizerWeight float64
}

func NewValueBasedEvictionStrategy() *ValueBasedEvictionStrategy {
	return &ValueBasedEvictionStrategy{
		AgeWeight:        0.2,
		FrequencyWeight:  0.3,
		HealthWeight:     0.4,
		RandomizerWeight: 0.1,
	}
}

func (s *ValueBasedEvictionStrategy) Name() string {
	return "ValueBased"
}

func (s *ValueBasedEvictionStrategy) Score(block *BlockMetadata, healthTracker *BlockHealthTracker) float64 {
	strategyName := s.Name()
	
	// Check for cached score first
	if cachedScore, valid := block.GetCachedScore(strategyName); valid {
		return cachedScore
	}
	
	score := 0.0
	
	// Age component (older = higher score)
	ageHours := time.Since(block.LastAccessed).Hours()
	ageScore := ageHours / 24.0 // Normalize to days
	score += ageScore * s.AgeWeight
	
	// Frequency component (less frequent = higher score)
	age := time.Since(block.CachedAt).Hours()
	if age < 1 {
		age = 1
	}
	accessRate := float64(block.Popularity) / age
	frequencyScore := 1.0 / (1.0 + accessRate)
	score += frequencyScore * s.FrequencyWeight
	
	// Health component (lower value = more likely to evict)
	if healthTracker != nil {
		// Use the stored hint from the health tracker
		hint := healthTracker.GetBlockHint(block.CID)
		blockValue := healthTracker.CalculateBlockValue(block.CID, hint)
		// Invert so that high-value blocks have low eviction scores
		// Normalize blockValue to 0-1 range (assuming max value of ~10)
		normalizedValue := math.Min(blockValue / 10.0, 1.0)
		healthScore := 1.0 - normalizedValue
		score += healthScore * s.HealthWeight
	}
	
	// Randomizer penalty (randomizers are valuable)
	if block.Origin == AltruisticBlock {
		// Check if it's a high-entropy block (likely randomizer)
		// This is a simplified check - in practice, you'd analyze the block data
		score *= (1.0 - s.RandomizerWeight)
	}
	
	// Cache the calculated score
	block.SetCachedScore(strategyName, score)
	
	return score
}

func (s *ValueBasedEvictionStrategy) SelectEvictionCandidates(blocks map[string]*BlockMetadata, spaceNeeded int64, healthTracker *BlockHealthTracker) []*BlockMetadata {
	candidates := make([]*BlockMetadata, 0, len(blocks))
	for _, block := range blocks {
		candidates = append(candidates, block)
	}
	
	sort.Slice(candidates, func(i, j int) bool {
		return s.Score(candidates[i], healthTracker) > s.Score(candidates[j], healthTracker)
	})
	
	selected := make([]*BlockMetadata, 0)
	freedSpace := int64(0)
	
	for _, block := range candidates {
		if freedSpace >= spaceNeeded {
			break
		}
		selected = append(selected, block)
		freedSpace += int64(block.Size)
	}
	
	return selected
}

// AdaptiveEvictionStrategy combines multiple strategies based on cache state
type AdaptiveEvictionStrategy struct {
	lru        *LRUEvictionStrategy
	lfu        *LFUEvictionStrategy
	valueBased *ValueBasedEvictionStrategy
	
	// Thresholds for strategy selection
	HighPressureThreshold float64 // Use aggressive eviction above this
	LowPressureThreshold  float64 // Use gentle eviction below this
}

func NewAdaptiveEvictionStrategy() *AdaptiveEvictionStrategy {
	return &AdaptiveEvictionStrategy{
		lru:                   &LRUEvictionStrategy{},
		lfu:                   &LFUEvictionStrategy{},
		valueBased:            NewValueBasedEvictionStrategy(),
		HighPressureThreshold: 0.9,  // 90% full
		LowPressureThreshold:  0.7,  // 70% full
	}
}

func (s *AdaptiveEvictionStrategy) Name() string {
	return "Adaptive"
}

func (s *AdaptiveEvictionStrategy) Score(block *BlockMetadata, healthTracker *BlockHealthTracker) float64 {
	// Use value-based scoring as default
	return s.valueBased.Score(block, healthTracker)
}

func (s *AdaptiveEvictionStrategy) SelectEvictionCandidates(blocks map[string]*BlockMetadata, spaceNeeded int64, healthTracker *BlockHealthTracker) []*BlockMetadata {
	// Calculate current utilization
	totalSize := int64(0)
	for _, block := range blocks {
		totalSize += int64(block.Size)
	}
	
	// This is a simplified calculation - in practice, you'd get total capacity from cache
	estimatedCapacity := totalSize + spaceNeeded
	utilization := float64(totalSize) / float64(estimatedCapacity)
	
	// Select strategy based on pressure
	var strategy EvictionStrategy
	if utilization >= s.HighPressureThreshold {
		// High pressure: use LRU for fast eviction
		strategy = s.lru
	} else if utilization <= s.LowPressureThreshold {
		// Low pressure: use value-based for optimal selection
		strategy = s.valueBased
	} else {
		// Medium pressure: use LFU
		strategy = s.lfu
	}
	
	return strategy.SelectEvictionCandidates(blocks, spaceNeeded, healthTracker)
}

// GradualEvictionStrategy evicts blocks gradually to prevent thrashing
type GradualEvictionStrategy struct {
	baseStrategy   EvictionStrategy
	maxEvictRatio  float64       // Max % of cache to evict at once
	minEvictSize   int64         // Minimum size to evict
	evictionBuffer float64       // Extra space to free (1.2 = 20% extra)
}

func NewGradualEvictionStrategy(base EvictionStrategy) *GradualEvictionStrategy {
	return &GradualEvictionStrategy{
		baseStrategy:   base,
		maxEvictRatio:  0.1,        // Max 10% at once
		minEvictSize:   1024,       // 1KB minimum (more reasonable for tests)
		evictionBuffer: 1.2,         // Free 20% extra space
	}
}

func (s *GradualEvictionStrategy) Name() string {
	return "Gradual-" + s.baseStrategy.Name()
}

func (s *GradualEvictionStrategy) Score(block *BlockMetadata, healthTracker *BlockHealthTracker) float64 {
	return s.baseStrategy.Score(block, healthTracker)
}

func (s *GradualEvictionStrategy) SelectEvictionCandidates(blocks map[string]*BlockMetadata, spaceNeeded int64, healthTracker *BlockHealthTracker) []*BlockMetadata {
	// Calculate total cache size
	totalSize := int64(0)
	for _, block := range blocks {
		totalSize += int64(block.Size)
	}
	
	// Apply gradual eviction limits
	maxEvictSize := int64(float64(totalSize) * s.maxEvictRatio)
	targetSize := int64(float64(spaceNeeded) * s.evictionBuffer)
	
	// Ensure we evict at least the minimum
	if targetSize < s.minEvictSize {
		targetSize = s.minEvictSize
	}
	
	// Cap at maximum eviction size
	if targetSize > maxEvictSize {
		targetSize = maxEvictSize
	}
	
	// Use base strategy with adjusted target
	return s.baseStrategy.SelectEvictionCandidates(blocks, targetSize, healthTracker)
}
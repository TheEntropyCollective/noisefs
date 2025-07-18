package cache

import (
	"math"
	"sync"
	"time"
)

// CacheHealthMonitor tracks and scores cache health metrics
type CacheHealthMonitor struct {
	mu                    sync.RWMutex
	metrics               *HealthMetrics
	coordinationEngine    *CoordinationEngine
	healthTracker         *BlockHealthTracker
	availabilityIntegration *AvailabilityIntegration
	startTime             time.Time
}

// HealthMetrics holds raw health data
type HealthMetrics struct {
	TotalBlocks       int64
	EvictionCount     int64
	LastEvictionTime  time.Time
	MemoryUsageBytes  int64
	TotalMemoryBytes  int64
	BlockAgeSum       float64 // Sum of all block ages in hours
	RandomizerCount   int64
	UniqueRandomizers int64
}

// HealthScore represents the computed health assessment
type HealthScore struct {
	Overall            float64 `json:"overall"`             // 0-1 overall health
	RandomizerDiversity float64 `json:"randomizer_diversity"` // 0-1 entropy measure
	AverageBlockAge    float64 `json:"average_block_age"`    // Hours since last access
	MemoryPressure     float64 `json:"memory_pressure"`      // 0-1 memory usage
	EvictionRate       float64 `json:"eviction_rate"`        // Evictions per hour
	CoordinationHealth float64 `json:"coordination_health"`  // 0-1 network coordination
	AvailabilityHealth float64 `json:"availability_health"`  // 0-1 randomizer availability
}

// NewCacheHealthMonitor creates a new health monitor
func NewCacheHealthMonitor() *CacheHealthMonitor {
	return &CacheHealthMonitor{
		metrics:   &HealthMetrics{},
		startTime: time.Now(),
	}
}

// SetCoordinationEngine sets the coordination engine for health scoring
func (chm *CacheHealthMonitor) SetCoordinationEngine(ce *CoordinationEngine) {
	chm.mu.Lock()
	defer chm.mu.Unlock()
	chm.coordinationEngine = ce
}

// SetHealthTracker sets the block health tracker
func (chm *CacheHealthMonitor) SetHealthTracker(ht *BlockHealthTracker) {
	chm.mu.Lock()
	defer chm.mu.Unlock()
	chm.healthTracker = ht
}

// SetAvailabilityIntegration sets the availability integration for health scoring
func (chm *CacheHealthMonitor) SetAvailabilityIntegration(ai *AvailabilityIntegration) {
	chm.mu.Lock()
	defer chm.mu.Unlock()
	chm.availabilityIntegration = ai
}

// UpdateMetrics updates the raw health metrics
func (chm *CacheHealthMonitor) UpdateMetrics(metrics *HealthMetrics) {
	chm.mu.Lock()
	defer chm.mu.Unlock()
	chm.metrics = metrics
}

// RecordEviction records a cache eviction event
func (chm *CacheHealthMonitor) RecordEviction() {
	chm.mu.Lock()
	defer chm.mu.Unlock()
	chm.metrics.EvictionCount++
	chm.metrics.LastEvictionTime = time.Now()
}

// CalculateHealthScore computes the overall health score
func (chm *CacheHealthMonitor) CalculateHealthScore() *HealthScore {
	chm.mu.RLock()
	defer chm.mu.RUnlock()

	score := &HealthScore{}
	
	// Calculate individual components
	score.RandomizerDiversity = chm.calculateRandomizerDiversity()
	score.AverageBlockAge = chm.calculateAverageBlockAge()
	score.MemoryPressure = chm.calculateMemoryPressure()
	score.EvictionRate = chm.calculateEvictionRate()
	score.CoordinationHealth = chm.calculateCoordinationHealth()
	score.AvailabilityHealth = chm.calculateAvailabilityHealth()
	
	// Calculate weighted overall score
	score.Overall = chm.calculateOverallScore(score)
	
	return score
}

// calculateRandomizerDiversity measures entropy in randomizer selection
func (chm *CacheHealthMonitor) calculateRandomizerDiversity() float64 {
	if chm.metrics.RandomizerCount == 0 {
		return 0.0
	}
	
	// Simple diversity measure: unique/total ratio
	diversity := float64(chm.metrics.UniqueRandomizers) / float64(chm.metrics.RandomizerCount)
	
	// Cap at 1.0
	if diversity > 1.0 {
		diversity = 1.0
	}
	
	return diversity
}

// calculateAverageBlockAge computes average block age in hours
func (chm *CacheHealthMonitor) calculateAverageBlockAge() float64 {
	if chm.metrics.TotalBlocks == 0 {
		return 0.0
	}
	
	return chm.metrics.BlockAgeSum / float64(chm.metrics.TotalBlocks)
}

// calculateMemoryPressure measures memory usage as percentage
func (chm *CacheHealthMonitor) calculateMemoryPressure() float64 {
	if chm.metrics.TotalMemoryBytes == 0 {
		return 0.0
	}
	
	pressure := float64(chm.metrics.MemoryUsageBytes) / float64(chm.metrics.TotalMemoryBytes)
	
	// Cap at 1.0
	if pressure > 1.0 {
		pressure = 1.0
	}
	
	return pressure
}

// calculateEvictionRate computes evictions per hour
func (chm *CacheHealthMonitor) calculateEvictionRate() float64 {
	elapsed := time.Since(chm.startTime).Hours()
	if elapsed < 0.1 { // Avoid division by very small numbers
		return 0.0
	}
	
	return float64(chm.metrics.EvictionCount) / elapsed
}

// calculateCoordinationHealth measures network coordination effectiveness
func (chm *CacheHealthMonitor) calculateCoordinationHealth() float64 {
	if chm.coordinationEngine == nil {
		return 0.5 // Neutral score when coordination is unavailable
	}
	
	metrics := chm.coordinationEngine.GetCoordinationMetrics()
	return metrics.CoordinationScore
}

// calculateAvailabilityHealth measures randomizer availability effectiveness
func (chm *CacheHealthMonitor) calculateAvailabilityHealth() float64 {
	if chm.availabilityIntegration == nil {
		return 0.8 // Good default score when availability checking is unavailable
	}
	
	return chm.availabilityIntegration.GetAvailabilityScore()
}

// calculateOverallScore computes weighted average of all health components
func (chm *CacheHealthMonitor) calculateOverallScore(score *HealthScore) float64 {
	// Weights for different health aspects
	weights := map[string]float64{
		"diversity":    0.20, // Randomizer diversity is critical for privacy
		"age":          0.10, // Block age affects access performance
		"memory":       0.15, // Memory pressure affects system stability
		"eviction":     0.10, // Eviction rate indicates cache churn
		"coordination": 0.25, // Network coordination is critical for efficiency
		"availability": 0.20, // Randomizer availability is critical for functionality
	}
	
	// Convert age to 0-1 score (lower age is better)
	ageScore := 1.0
	if score.AverageBlockAge > 0 {
		// Ideal age is < 1 hour, score decreases as age increases
		ageScore = math.Max(0.0, 1.0 - (score.AverageBlockAge / 24.0)) // 24 hour max
	}
	
	// Convert eviction rate to 0-1 score (lower rate is better)
	evictionScore := 1.0
	if score.EvictionRate > 0 {
		// Ideal rate is < 1 eviction/hour, score decreases as rate increases
		evictionScore = math.Max(0.0, 1.0 - (score.EvictionRate / 10.0)) // 10 evictions/hour max
	}
	
	// Convert memory pressure to 0-1 score (lower pressure is better)
	memoryScore := 1.0 - score.MemoryPressure
	
	// Calculate weighted sum
	overall := weights["diversity"]*score.RandomizerDiversity +
		weights["age"]*ageScore +
		weights["memory"]*memoryScore +
		weights["eviction"]*evictionScore +
		weights["coordination"]*score.CoordinationHealth +
		weights["availability"]*score.AvailabilityHealth
	
	return math.Max(0.0, math.Min(1.0, overall))
}

// GetHealthSummary returns a human-readable health summary
func (chm *CacheHealthMonitor) GetHealthSummary() string {
	score := chm.CalculateHealthScore()
	
	switch {
	case score.Overall >= 0.8:
		return "Excellent"
	case score.Overall >= 0.6:
		return "Good"
	case score.Overall >= 0.4:
		return "Fair"
	case score.Overall >= 0.2:
		return "Poor"
	default:
		return "Critical"
	}
}
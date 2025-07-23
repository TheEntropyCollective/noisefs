package cache

import (
	"math"
	"sort"
	"sync"
	"time"
)

// AccessPattern represents the access pattern for prediction
type AccessPattern struct {
	BlockID       string
	AccessTimes   []time.Time
	AccessCounts  []int // Hourly access counts
	LastPredicted time.Time
}

// PredictiveEvictor predicts future access patterns and pre-emptively evicts blocks
type PredictiveEvictor struct {
	// Access patterns for prediction
	patterns map[string]*AccessPattern

	// Configuration
	predictionWindow  time.Duration
	updateInterval    time.Duration
	preEvictThreshold float64 // Utilization threshold to start pre-eviction

	// Prediction model (simplified)
	hourlyAverages [24]float64 // Average access rate by hour

	// State
	lastUpdate time.Time
	mu         sync.RWMutex
}

// PredictiveEvictorConfig configures the predictive evictor
type PredictiveEvictorConfig struct {
	PredictionWindow  time.Duration
	UpdateInterval    time.Duration
	PreEvictThreshold float64
}

// DefaultPredictiveEvictorConfig returns default configuration
func DefaultPredictiveEvictorConfig() *PredictiveEvictorConfig {
	return &PredictiveEvictorConfig{
		PredictionWindow:  24 * time.Hour,
		UpdateInterval:    15 * time.Minute,
		PreEvictThreshold: 0.85, // Start pre-evicting at 85% full
	}
}

// NewPredictiveEvictor creates a new predictive evictor
func NewPredictiveEvictor(config *PredictiveEvictorConfig) *PredictiveEvictor {
	if config == nil {
		config = DefaultPredictiveEvictorConfig()
	}

	return &PredictiveEvictor{
		patterns:          make(map[string]*AccessPattern),
		predictionWindow:  config.PredictionWindow,
		updateInterval:    config.UpdateInterval,
		preEvictThreshold: config.PreEvictThreshold,
		lastUpdate:        time.Now(),
	}
}

// RecordAccess records a block access for prediction
func (pe *PredictiveEvictor) RecordAccess(blockID string, accessTime time.Time) {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	pattern, exists := pe.patterns[blockID]
	if !exists {
		pattern = &AccessPattern{
			BlockID:      blockID,
			AccessTimes:  make([]time.Time, 0),
			AccessCounts: make([]int, 24), // 24 hours
		}
		pe.patterns[blockID] = pattern
	}

	pattern.AccessTimes = append(pattern.AccessTimes, accessTime)

	// Sort access times to ensure chronological order
	sort.Slice(pattern.AccessTimes, func(i, j int) bool {
		return pattern.AccessTimes[i].Before(pattern.AccessTimes[j])
	})

	hour := accessTime.Hour()
	pattern.AccessCounts[hour]++

	// Update hourly averages
	pe.updateHourlyAverages()

	// Clean old access times
	cutoff := time.Now().Add(-pe.predictionWindow)
	newTimes := make([]time.Time, 0)
	for _, t := range pattern.AccessTimes {
		if t.After(cutoff) {
			newTimes = append(newTimes, t)
		}
	}
	pattern.AccessTimes = newTimes
}

// PredictNextAccess predicts when a block will be accessed next
func (pe *PredictiveEvictor) PredictNextAccess(blockID string) (time.Time, float64) {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	pattern, exists := pe.patterns[blockID]
	if !exists || len(pattern.AccessTimes) == 0 {
		// No history, use average prediction
		return pe.predictFromAverage(), 0.1
	}

	// Simple prediction based on access intervals
	intervals := pe.calculateIntervals(pattern.AccessTimes)
	if len(intervals) == 0 {
		return pe.predictFromAverage(), 0.2
	}

	// Calculate average interval
	avgInterval := pe.averageInterval(intervals)

	// Predict next access
	lastAccess := pattern.AccessTimes[len(pattern.AccessTimes)-1]
	now := time.Now()

	// If last access was recent, predict avgInterval from last access
	// Otherwise, predict from now
	var predictedTime time.Time
	timeSinceLastAccess := now.Sub(lastAccess)

	if timeSinceLastAccess < avgInterval {
		// Next access is avgInterval from last access
		predictedTime = lastAccess.Add(avgInterval)
	} else {
		// We've already passed the expected time, predict soon
		predictedTime = now.Add(avgInterval / 4)
	}

	// Calculate confidence based on interval variance
	variance := pe.calculateVariance(intervals, avgInterval)
	// Use exponential decay for confidence based on variance
	// Higher variance = much lower confidence
	confidence := math.Exp(-variance)
	confidence = math.Min(1.0, math.Max(0.0, confidence))

	return predictedTime, confidence
}

// GetEvictionCandidates returns blocks unlikely to be accessed soon
func (pe *PredictiveEvictor) GetEvictionCandidates(blocks map[string]*BlockMetadata, count int) []string {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	type prediction struct {
		blockID    string
		nextAccess time.Time
		confidence float64
		score      float64
	}

	predictions := make([]prediction, 0, len(blocks))
	now := time.Now()

	for blockID := range blocks {
		nextAccess, confidence := pe.PredictNextAccess(blockID)

		// Calculate score (higher = more likely to evict)
		timeUntilAccess := nextAccess.Sub(now).Hours()
		if timeUntilAccess < 0 {
			timeUntilAccess = 0
		}

		// Score based on time until access and confidence
		// Higher score = more likely to evict
		// Blocks with longer time until access should have higher scores
		// But if confidence is low, we should be more conservative
		score := timeUntilAccess
		if confidence < 0.3 {
			// Low confidence - reduce score to avoid premature eviction
			score *= 0.5
		}

		predictions = append(predictions, prediction{
			blockID:    blockID,
			nextAccess: nextAccess,
			confidence: confidence,
			score:      score,
		})
	}

	// Sort by score (descending - blocks with longest time until access first)
	for i := 0; i < len(predictions); i++ {
		for j := i + 1; j < len(predictions); j++ {
			if predictions[j].score > predictions[i].score {
				predictions[i], predictions[j] = predictions[j], predictions[i]
			}
		}
	}

	// Return top candidates
	candidates := make([]string, 0, count)
	for i := 0; i < len(predictions) && i < count; i++ {
		candidates = append(candidates, predictions[i].blockID)
	}

	return candidates
}

// ShouldPreEvict determines if pre-emptive eviction should occur
func (pe *PredictiveEvictor) ShouldPreEvict(utilization float64) bool {
	return utilization >= pe.preEvictThreshold
}

// GetPreEvictionSize calculates how much to pre-evict based on predicted usage
func (pe *PredictiveEvictor) GetPreEvictionSize(currentUtilization float64, totalCapacity int64) int64 {
	if currentUtilization < pe.preEvictThreshold {
		return 0
	}

	// Pre-evict to bring utilization to 75%
	targetUtilization := 0.75
	currentSize := int64(currentUtilization * float64(totalCapacity))
	targetSize := int64(targetUtilization * float64(totalCapacity))

	return currentSize - targetSize
}

// Helper methods

func (pe *PredictiveEvictor) updateHourlyAverages() {
	// Reset averages
	for i := range pe.hourlyAverages {
		pe.hourlyAverages[i] = 0
	}

	// Calculate new averages
	totalByHour := make(map[int]int)
	patternCount := 0

	for _, pattern := range pe.patterns {
		if len(pattern.AccessTimes) > 0 {
			patternCount++
			for hour, count := range pattern.AccessCounts {
				totalByHour[hour] += count
			}
		}
	}

	if patternCount > 0 {
		for hour, total := range totalByHour {
			pe.hourlyAverages[hour] = float64(total) / float64(patternCount)
		}
	}
}

func (pe *PredictiveEvictor) calculateIntervals(times []time.Time) []time.Duration {
	if len(times) < 2 {
		return nil
	}

	intervals := make([]time.Duration, len(times)-1)
	for i := 1; i < len(times); i++ {
		intervals[i-1] = times[i].Sub(times[i-1])
	}

	return intervals
}

func (pe *PredictiveEvictor) averageInterval(intervals []time.Duration) time.Duration {
	if len(intervals) == 0 {
		return time.Hour // Default
	}

	total := time.Duration(0)
	for _, interval := range intervals {
		total += interval
	}

	return total / time.Duration(len(intervals))
}

func (pe *PredictiveEvictor) calculateVariance(intervals []time.Duration, mean time.Duration) float64 {
	if len(intervals) < 2 {
		return 10.0 // High variance for insufficient data
	}

	variance := 0.0
	meanSeconds := mean.Seconds()

	// Prevent division by zero
	if meanSeconds == 0 {
		return 10.0
	}

	for _, interval := range intervals {
		diff := interval.Seconds() - meanSeconds
		variance += diff * diff
	}

	variance /= float64(len(intervals))
	coeffVariation := math.Sqrt(variance) / meanSeconds

	// For highly irregular patterns, return high variance
	if coeffVariation > 1.0 {
		return coeffVariation * 10.0
	}

	return coeffVariation
}

func (pe *PredictiveEvictor) predictFromAverage() time.Time {
	now := time.Now()
	currentHour := now.Hour()

	// Find next hour with high average access
	maxAvg := 0.0
	nextHour := (currentHour + 1) % 24

	for i := 1; i <= 24; i++ {
		hour := (currentHour + i) % 24
		if pe.hourlyAverages[hour] > maxAvg {
			maxAvg = pe.hourlyAverages[hour]
			nextHour = hour
		}
	}

	// If no activity, predict 2 hours from now
	if maxAvg == 0 {
		return now.Add(2 * time.Hour)
	}

	// Calculate time until that hour
	hoursUntil := nextHour - currentHour
	if hoursUntil <= 0 {
		hoursUntil += 24
	}

	// Return the beginning of that hour
	nextTime := now.Add(time.Duration(hoursUntil) * time.Hour)
	// Round to the start of the hour
	return time.Date(nextTime.Year(), nextTime.Month(), nextTime.Day(), nextTime.Hour(), 0, 0, 0, nextTime.Location())
}

// PredictiveEvictionIntegration integrates predictive eviction with the cache
type PredictiveEvictionIntegration struct {
	cache     *AltruisticCache
	predictor *PredictiveEvictor
	enabled   bool
	mu        sync.RWMutex
}

// NewPredictiveEvictionIntegration creates integration between cache and predictor
func NewPredictiveEvictionIntegration(cache *AltruisticCache, config *PredictiveEvictorConfig) *PredictiveEvictionIntegration {
	return &PredictiveEvictionIntegration{
		cache:     cache,
		predictor: NewPredictiveEvictor(config),
		enabled:   true,
	}
}

// RecordBlockAccess records access for prediction
func (pei *PredictiveEvictionIntegration) RecordBlockAccess(blockID string) {
	if !pei.enabled {
		return
	}

	pei.predictor.RecordAccess(blockID, time.Now())
}

// PerformPreEviction performs pre-emptive eviction if needed
func (pei *PredictiveEvictionIntegration) PerformPreEviction() error {
	pei.mu.Lock()
	defer pei.mu.Unlock()

	if !pei.enabled {
		return nil
	}

	stats := pei.cache.GetAltruisticStats()
	utilization := float64(stats.PersonalSize+stats.AltruisticSize) / float64(stats.TotalCapacity)

	if !pei.predictor.ShouldPreEvict(utilization) {
		return nil
	}

	// Get pre-eviction size
	evictSize := pei.predictor.GetPreEvictionSize(utilization, stats.TotalCapacity)
	if evictSize <= 0 {
		return nil
	}

	// Get eviction candidates
	pei.cache.mu.RLock()
	candidates := pei.predictor.GetEvictionCandidates(pei.cache.altruisticBlocks, 20)
	pei.cache.mu.RUnlock()

	// Evict predicted blocks
	evicted := int64(0)
	for _, blockID := range candidates {
		if evicted >= evictSize {
			break
		}

		pei.cache.mu.RLock()
		metadata, exists := pei.cache.altruisticBlocks[blockID]
		pei.cache.mu.RUnlock()

		if exists {
			err := pei.cache.Remove(blockID)
			if err == nil {
				evicted += int64(metadata.Size)
			}
		}
	}

	return nil
}

// Enable enables predictive eviction
func (pei *PredictiveEvictionIntegration) Enable() {
	pei.mu.Lock()
	defer pei.mu.Unlock()
	pei.enabled = true
}

// Disable disables predictive eviction
func (pei *PredictiveEvictionIntegration) Disable() {
	pei.mu.Lock()
	defer pei.mu.Unlock()
	pei.enabled = false
}

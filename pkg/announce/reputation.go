package announce

import (
	"math"
	"sync"
	"time"
)

// ReputationSystem tracks reputation of announcement sources
type ReputationSystem struct {
	// Configuration
	initialScore      float64
	maxScore          float64
	minScore          float64
	decayRate         float64
	positiveWeight    float64
	negativeWeight    float64
	requiredHistory   int
	
	// Tracking
	sources map[string]*SourceReputation
	mu      sync.RWMutex
	
	// Cleanup
	stopCleanup chan struct{}
	wg          sync.WaitGroup
}

// SourceReputation tracks reputation for a single source
type SourceReputation struct {
	Score           float64
	TotalEvents     int
	PositiveEvents  int
	NegativeEvents  int
	LastActivity    time.Time
	FirstSeen       time.Time
	Flags           []string
}

// ReputationConfig holds reputation system configuration
type ReputationConfig struct {
	InitialScore    float64       // Starting reputation score
	MaxScore        float64       // Maximum possible score
	MinScore        float64       // Minimum possible score
	DecayRate       float64       // Score decay per day of inactivity
	PositiveWeight  float64       // Weight for positive events
	NegativeWeight  float64       // Weight for negative events
	RequiredHistory int           // Events needed for trusted status
	CleanupInterval time.Duration // How often to run cleanup
}

// DefaultReputationConfig returns default reputation configuration
func DefaultReputationConfig() *ReputationConfig {
	return &ReputationConfig{
		InitialScore:    50.0,  // Start neutral
		MaxScore:        100.0, // Maximum reputation
		MinScore:        0.0,   // Minimum reputation
		DecayRate:       0.1,   // Lose 0.1 points per day inactive
		PositiveWeight:  1.0,   // +1 for good behavior
		NegativeWeight:  5.0,   // -5 for bad behavior
		RequiredHistory: 10,    // Need 10 events to be trusted
		CleanupInterval: 24 * time.Hour,
	}
}

// NewReputationSystem creates a new reputation system
func NewReputationSystem(config *ReputationConfig) *ReputationSystem {
	if config == nil {
		config = DefaultReputationConfig()
	}
	
	rs := &ReputationSystem{
		initialScore:    config.InitialScore,
		maxScore:        config.MaxScore,
		minScore:        config.MinScore,
		decayRate:       config.DecayRate,
		positiveWeight:  config.PositiveWeight,
		negativeWeight:  config.NegativeWeight,
		requiredHistory: config.RequiredHistory,
		sources:         make(map[string]*SourceReputation),
		stopCleanup:     make(chan struct{}),
	}
	
	// Start cleanup routine
	rs.wg.Add(1)
	go rs.cleanupLoop(config.CleanupInterval)
	
	return rs
}

// RecordPositive records a positive event for a source
func (rs *ReputationSystem) RecordPositive(sourceID string, reason string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	
	rep := rs.getOrCreateReputation(sourceID)
	
	// Update score
	rep.Score += rs.positiveWeight
	if rep.Score > rs.maxScore {
		rep.Score = rs.maxScore
	}
	
	// Update counters
	rep.PositiveEvents++
	rep.TotalEvents++
	rep.LastActivity = time.Now()
}

// RecordNegative records a negative event for a source
func (rs *ReputationSystem) RecordNegative(sourceID string, reason string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	
	rep := rs.getOrCreateReputation(sourceID)
	
	// Update score
	rep.Score -= rs.negativeWeight
	if rep.Score < rs.minScore {
		rep.Score = rs.minScore
	}
	
	// Update counters
	rep.NegativeEvents++
	rep.TotalEvents++
	rep.LastActivity = time.Now()
	
	// Add flag if significant
	if reason != "" && !contains(rep.Flags, reason) {
		rep.Flags = append(rep.Flags, reason)
	}
}

// GetReputation returns the reputation for a source
func (rs *ReputationSystem) GetReputation(sourceID string) (*SourceReputation, bool) {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	
	rep, exists := rs.sources[sourceID]
	if !exists {
		return nil, false
	}
	
	// Apply decay
	decayedScore := rs.applyDecay(rep)
	
	// Return copy with decayed score
	result := &SourceReputation{
		Score:          decayedScore,
		TotalEvents:    rep.TotalEvents,
		PositiveEvents: rep.PositiveEvents,
		NegativeEvents: rep.NegativeEvents,
		LastActivity:   rep.LastActivity,
		FirstSeen:      rep.FirstSeen,
		Flags:          append([]string{}, rep.Flags...), // Copy slice
	}
	
	return result, true
}

// GetScore returns just the reputation score for a source
func (rs *ReputationSystem) GetScore(sourceID string) float64 {
	rep, exists := rs.GetReputation(sourceID)
	if !exists {
		return rs.initialScore
	}
	return rep.Score
}

// IsTrusted checks if a source is trusted
func (rs *ReputationSystem) IsTrusted(sourceID string) bool {
	rep, exists := rs.GetReputation(sourceID)
	if !exists {
		return false
	}
	
	// Require minimum history
	if rep.TotalEvents < rs.requiredHistory {
		return false
	}
	
	// Require good score
	threshold := (rs.maxScore + rs.initialScore) / 2 // Above average
	return rep.Score >= threshold
}

// IsBlacklisted checks if a source is blacklisted
func (rs *ReputationSystem) IsBlacklisted(sourceID string) bool {
	rep, exists := rs.GetReputation(sourceID)
	if !exists {
		return false
	}
	
	// Very low score indicates blacklist
	threshold := rs.minScore + (rs.initialScore-rs.minScore)*0.2 // Bottom 20%
	return rep.Score <= threshold
}

// GetTrustLevel returns a trust level string
func (rs *ReputationSystem) GetTrustLevel(sourceID string) string {
	rep, exists := rs.GetReputation(sourceID)
	if !exists {
		return "unknown"
	}
	
	if rep.TotalEvents < rs.requiredHistory {
		return "new"
	}
	
	scoreRange := rs.maxScore - rs.minScore
	normalizedScore := (rep.Score - rs.minScore) / scoreRange
	
	switch {
	case normalizedScore >= 0.8:
		return "trusted"
	case normalizedScore >= 0.6:
		return "good"
	case normalizedScore >= 0.4:
		return "neutral"
	case normalizedScore >= 0.2:
		return "suspicious"
	default:
		return "untrusted"
	}
}

// GetStats returns reputation system statistics
func (rs *ReputationSystem) GetStats() ReputationStats {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	
	stats := ReputationStats{
		TotalSources: len(rs.sources),
	}
	
	for _, rep := range rs.sources {
		score := rs.applyDecay(rep)
		
		// Count by trust level
		if rep.TotalEvents < rs.requiredHistory {
			stats.NewSources++
		} else if score >= (rs.maxScore+rs.initialScore)/2 {
			stats.TrustedSources++
		} else if score <= rs.minScore+(rs.initialScore-rs.minScore)*0.2 {
			stats.BlacklistedSources++
		}
		
		// Aggregate events
		stats.TotalPositiveEvents += rep.PositiveEvents
		stats.TotalNegativeEvents += rep.NegativeEvents
		stats.AverageScore += score
	}
	
	if stats.TotalSources > 0 {
		stats.AverageScore /= float64(stats.TotalSources)
	}
	
	return stats
}

// Close stops the reputation system
func (rs *ReputationSystem) Close() {
	close(rs.stopCleanup)
	rs.wg.Wait()
}

// Helper methods

// getOrCreateReputation gets or creates a reputation record
func (rs *ReputationSystem) getOrCreateReputation(sourceID string) *SourceReputation {
	rep, exists := rs.sources[sourceID]
	if !exists {
		now := time.Now()
		rep = &SourceReputation{
			Score:        rs.initialScore,
			FirstSeen:    now,
			LastActivity: now,
			Flags:        []string{},
		}
		rs.sources[sourceID] = rep
	}
	return rep
}

// applyDecay applies time-based score decay
func (rs *ReputationSystem) applyDecay(rep *SourceReputation) float64 {
	daysSinceActivity := time.Since(rep.LastActivity).Hours() / 24
	decay := rs.decayRate * daysSinceActivity
	
	decayedScore := rep.Score - decay
	if decayedScore < rs.minScore {
		decayedScore = rs.minScore
	}
	
	return decayedScore
}

// cleanupLoop periodically removes old records
func (rs *ReputationSystem) cleanupLoop(interval time.Duration) {
	defer rs.wg.Done()
	
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	
	for {
		select {
		case <-rs.stopCleanup:
			return
		case <-ticker.C:
			rs.cleanup()
		}
	}
}

// cleanup removes very old inactive records
func (rs *ReputationSystem) cleanup() {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	
	cutoff := time.Now().Add(-365 * 24 * time.Hour) // 1 year
	
	for sourceID, rep := range rs.sources {
		if rep.LastActivity.Before(cutoff) && rep.TotalEvents < 10 {
			// Remove old sources with minimal activity
			delete(rs.sources, sourceID)
		}
	}
}

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// ReputationStats holds reputation system statistics
type ReputationStats struct {
	TotalSources        int
	TrustedSources      int
	BlacklistedSources  int
	NewSources          int
	TotalPositiveEvents int
	TotalNegativeEvents int
	AverageScore        float64
}

// ScoreAdjustment calculates score adjustment based on behavior
func (rs *ReputationSystem) ScoreAdjustment(positive, negative int) float64 {
	return float64(positive)*rs.positiveWeight - float64(negative)*rs.negativeWeight
}

// DecayScore calculates decayed score for a given time period
func (rs *ReputationSystem) DecayScore(currentScore float64, daysSinceActivity float64) float64 {
	decay := rs.decayRate * daysSinceActivity
	decayedScore := currentScore - decay
	
	return math.Max(decayedScore, rs.minScore)
}
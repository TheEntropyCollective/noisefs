package cache

import (
	"math"
	"sync"
	"time"
)

// RandomizerDiversityControls manages diversity in randomizer selection
// to prevent concentration attacks and ensure strong privacy guarantees
type RandomizerDiversityControls struct {
	mu                    sync.RWMutex
	config                *DiversityControlsConfig
	usageTracker          map[string]*RandomizerUsage
	concentrationTracker  *ConcentrationTracker
	entropyCalculator     *EntropyCalculator
	diversityEnforcer     *DiversityEnforcer
	lastCleanup          time.Time
}

// DiversityControlsConfig holds configuration for diversity controls
type DiversityControlsConfig struct {
	// Concentration thresholds
	MaxUsageRatio           float64       `json:"max_usage_ratio"`           // 0.15 = max 15% usage
	ConcentrationThreshold  float64       `json:"concentration_threshold"`   // 0.3 = alert if >30% concentration
	CriticalThreshold       float64       `json:"critical_threshold"`        // 0.5 = critical if >50% concentration
	
	// Diversity requirements  
	MinEntropyBits          float64       `json:"min_entropy_bits"`          // 4.0 = minimum 4 bits entropy
	TargetUniqueRatio       float64       `json:"target_unique_ratio"`       // 0.8 = target 80% unique randomizers
	
	// Enforcement policies
	EnableDiversityBoost    bool          `json:"enable_diversity_boost"`    // Boost under-used randomizers
	EnableConcentrationPenalty bool       `json:"enable_concentration_penalty"` // Penalize over-used randomizers
	
	// Cleanup and maintenance
	CleanupInterval         time.Duration `json:"cleanup_interval"`          // 1 hour
	UsageHistoryWindow      time.Duration `json:"usage_history_window"`      // 24 hours
	
	// Emergency controls
	EmergencyDiversityMode  bool          `json:"emergency_diversity_mode"`  // Force maximum diversity
	BlockConcentratedRandomizers bool     `json:"block_concentrated_randomizers"` // Block highly concentrated randomizers
}

// RandomizerUsage tracks usage statistics for a single randomizer
type RandomizerUsage struct {
	CID                 string
	UsageCount          int64
	LastUsed            time.Time
	FirstUsed           time.Time
	UsageHistory        []time.Time // Recent usage timestamps
	ConcentrationScore  float64     // Current concentration score
	DiversityScore      float64     // Current diversity score
}

// ConcentrationTracker identifies concentration patterns
type ConcentrationTracker struct {
	mu                  sync.RWMutex
	totalSelections     int64
	recentSelections    map[string]int64 // CID -> recent count
	concentrationAlerts map[string]time.Time // CID -> last alert time
	criticalRandomizers map[string]bool  // CIDs with critical concentration
}

// EntropyCalculator computes entropy metrics for randomizer selection
type EntropyCalculator struct {
	mu           sync.RWMutex
	sampleWindow int
	recentSelections []string // Rolling window of recent selections
	entropyCache     float64
	cacheTime        time.Time
	cacheTTL         time.Duration
}

// DiversityEnforcer implements diversity enforcement policies
type DiversityEnforcer struct {
	mu                sync.RWMutex
	config            *DiversityControlsConfig
	penaltyMultipliers map[string]float64 // CID -> penalty multiplier
	boostMultipliers   map[string]float64 // CID -> boost multiplier
	lastUpdate         time.Time
}

// NewRandomizerDiversityControls creates a new diversity control system
func NewRandomizerDiversityControls(config *DiversityControlsConfig) *RandomizerDiversityControls {
	if config == nil {
		config = DefaultDiversityControlsConfig()
	}
	
	return &RandomizerDiversityControls{
		config:               config,
		usageTracker:         make(map[string]*RandomizerUsage),
		concentrationTracker: NewConcentrationTracker(),
		entropyCalculator:    NewEntropyCalculator(1000), // 1000 sample window
		diversityEnforcer:    NewDiversityEnforcer(config),
		lastCleanup:          time.Now(),
	}
}

// DefaultDiversityControlsConfig returns default configuration
func DefaultDiversityControlsConfig() *DiversityControlsConfig {
	return &DiversityControlsConfig{
		MaxUsageRatio:           0.15,  // Max 15% usage for any single randomizer
		ConcentrationThreshold:  0.3,   // Alert at 30% concentration
		CriticalThreshold:       0.5,   // Critical at 50% concentration
		MinEntropyBits:          4.0,   // Minimum 4 bits of entropy
		TargetUniqueRatio:       0.8,   // Target 80% unique selections
		EnableDiversityBoost:    true,
		EnableConcentrationPenalty: true,
		CleanupInterval:         time.Hour,
		UsageHistoryWindow:      24 * time.Hour,
		EmergencyDiversityMode:  false,
		BlockConcentratedRandomizers: false,
	}
}

// RecordRandomizerSelection records a randomizer selection for diversity tracking
func (rdc *RandomizerDiversityControls) RecordRandomizerSelection(cid string) {
	rdc.mu.Lock()
	defer rdc.mu.Unlock()
	
	now := time.Now()
	
	// Update usage tracker
	if usage, exists := rdc.usageTracker[cid]; exists {
		usage.UsageCount++
		usage.LastUsed = now
		usage.UsageHistory = append(usage.UsageHistory, now)
		
		// Maintain history window
		rdc.trimUsageHistory(usage)
	} else {
		rdc.usageTracker[cid] = &RandomizerUsage{
			CID:          cid,
			UsageCount:   1,
			LastUsed:     now,
			FirstUsed:    now,
			UsageHistory: []time.Time{now},
		}
	}
	
	// Update concentration tracker
	rdc.concentrationTracker.RecordSelection(cid)
	
	// Update entropy calculator
	rdc.entropyCalculator.RecordSelection(cid)
	
	// Update diversity scores
	rdc.updateDiversityScores()
	
	// Check for cleanup
	if time.Since(rdc.lastCleanup) > rdc.config.CleanupInterval {
		rdc.performCleanup()
		rdc.lastCleanup = now
	}
}

// CalculateRandomizerScore calculates diversity-adjusted score for randomizer selection
func (rdc *RandomizerDiversityControls) CalculateRandomizerScore(cid string, baseScore float64) float64 {
	rdc.mu.RLock()
	defer rdc.mu.RUnlock()
	
	// Start with base score
	adjustedScore := baseScore
	
	// New randomizers get a boost
	_, exists := rdc.usageTracker[cid]
	if !exists {
		adjustedScore *= 2.0 // Boost new randomizers
	}
	
	// Apply diversity enforcement
	if rdc.config.EnableDiversityBoost {
		if boost, exists := rdc.diversityEnforcer.boostMultipliers[cid]; exists {
			adjustedScore *= boost
		}
	}
	
	if rdc.config.EnableConcentrationPenalty {
		if penalty, exists := rdc.diversityEnforcer.penaltyMultipliers[cid]; exists {
			adjustedScore *= penalty
		}
	}
	
	// Check for blocked randomizers
	if rdc.config.BlockConcentratedRandomizers {
		if rdc.concentrationTracker.criticalRandomizers[cid] {
			return 0.0 // Block critical concentration randomizers
		}
	}
	
	// Emergency diversity mode
	if rdc.config.EmergencyDiversityMode {
		adjustedScore *= rdc.calculateEmergencyDiversityBoost(cid)
	}
	
	return math.Max(0.0, adjustedScore)
}

// GetDiversityMetrics returns current diversity metrics
func (rdc *RandomizerDiversityControls) GetDiversityMetrics() *DiversityMetrics {
	rdc.mu.RLock()
	defer rdc.mu.RUnlock()
	
	return &DiversityMetrics{
		Entropy:              rdc.entropyCalculator.GetCurrentEntropy(),
		UniqueRatio:          rdc.calculateUniqueRatio(),
		ConcentrationScore:   rdc.concentrationTracker.GetConcentrationScore(),
		MaxUsageRatio:        rdc.calculateMaxUsageRatio(),
		TotalRandomizers:     int64(len(rdc.usageTracker)),
		ActiveRandomizers:    rdc.countActiveRandomizers(),
		ConcentrationAlerts:  int64(len(rdc.concentrationTracker.concentrationAlerts)),
		CriticalRandomizers:  int64(len(rdc.concentrationTracker.criticalRandomizers)),
		EmergencyMode:        rdc.config.EmergencyDiversityMode,
		HealthStatus:         rdc.calculateHealthStatus(),
	}
}

// DiversityMetrics holds diversity analysis results
type DiversityMetrics struct {
	Entropy              float64 `json:"entropy"`               // Shannon entropy in bits
	UniqueRatio          float64 `json:"unique_ratio"`          // Ratio of unique selections
	ConcentrationScore   float64 `json:"concentration_score"`   // Overall concentration score
	MaxUsageRatio        float64 `json:"max_usage_ratio"`       // Highest individual usage ratio
	TotalRandomizers     int64   `json:"total_randomizers"`     // Total tracked randomizers
	ActiveRandomizers    int64   `json:"active_randomizers"`    // Recently used randomizers
	ConcentrationAlerts  int64   `json:"concentration_alerts"`  // Number of concentration alerts
	CriticalRandomizers  int64   `json:"critical_randomizers"`  // Number of critical randomizers
	EmergencyMode        bool    `json:"emergency_mode"`        // Emergency diversity mode active
	HealthStatus         string  `json:"health_status"`         // Overall health assessment
}

// Helper methods

func (rdc *RandomizerDiversityControls) trimUsageHistory(usage *RandomizerUsage) {
	cutoff := time.Now().Add(-rdc.config.UsageHistoryWindow)
	newHistory := make([]time.Time, 0, len(usage.UsageHistory))
	
	for _, timestamp := range usage.UsageHistory {
		if timestamp.After(cutoff) {
			newHistory = append(newHistory, timestamp)
		}
	}
	
	usage.UsageHistory = newHistory
}

func (rdc *RandomizerDiversityControls) updateDiversityScores() {
	totalSelections := rdc.concentrationTracker.totalSelections
	if totalSelections == 0 {
		return
	}
	
	for cid, usage := range rdc.usageTracker {
		// Calculate concentration score
		usage.ConcentrationScore = float64(usage.UsageCount) / float64(totalSelections)
		
		// Calculate diversity score (inverse of concentration)
		usage.DiversityScore = 1.0 - usage.ConcentrationScore
		
		// Check for critical concentration
		if usage.ConcentrationScore >= rdc.config.CriticalThreshold {
			rdc.concentrationTracker.criticalRandomizers[cid] = true
		} else {
			delete(rdc.concentrationTracker.criticalRandomizers, cid)
		}
		
		// Check for concentration alerts
		if usage.ConcentrationScore >= rdc.config.ConcentrationThreshold {
			rdc.concentrationTracker.concentrationAlerts[cid] = time.Now()
		} else {
			delete(rdc.concentrationTracker.concentrationAlerts, cid)
		}
		
		// Update enforcer multipliers
		rdc.diversityEnforcer.UpdateMultipliers(cid, usage.ConcentrationScore, usage.DiversityScore)
	}
}

func (rdc *RandomizerDiversityControls) performCleanup() {
	cutoff := time.Now().Add(-rdc.config.UsageHistoryWindow)
	
	// Clean old usage records
	for cid, usage := range rdc.usageTracker {
		// Trim history first
		rdc.trimUsageHistory(usage)
		
		// Remove if last used before cutoff and no recent history
		if usage.LastUsed.Before(cutoff) && len(usage.UsageHistory) == 0 {
			delete(rdc.usageTracker, cid)
			// Also clean from concentration tracker
			delete(rdc.concentrationTracker.recentSelections, cid)
			delete(rdc.concentrationTracker.concentrationAlerts, cid)
			delete(rdc.concentrationTracker.criticalRandomizers, cid)
		}
	}
	
	// Clean concentration tracker
	rdc.concentrationTracker.Cleanup(cutoff)
	
	// Update entropy calculator
	rdc.entropyCalculator.Cleanup()
}

func (rdc *RandomizerDiversityControls) calculateUniqueRatio() float64 {
	if len(rdc.usageTracker) == 0 {
		return 1.0
	}
	
	recentSelections := rdc.concentrationTracker.totalSelections
	if recentSelections == 0 {
		return 1.0
	}
	
	return float64(len(rdc.usageTracker)) / float64(recentSelections)
}

func (rdc *RandomizerDiversityControls) calculateMaxUsageRatio() float64 {
	totalSelections := rdc.concentrationTracker.totalSelections
	if totalSelections == 0 {
		return 0.0
	}
	
	maxUsage := int64(0)
	for _, usage := range rdc.usageTracker {
		if usage.UsageCount > maxUsage {
			maxUsage = usage.UsageCount
		}
	}
	
	return float64(maxUsage) / float64(totalSelections)
}

func (rdc *RandomizerDiversityControls) countActiveRandomizers() int64 {
	cutoff := time.Now().Add(-time.Hour) // Active in last hour
	count := int64(0)
	
	for _, usage := range rdc.usageTracker {
		if usage.LastUsed.After(cutoff) {
			count++
		}
	}
	
	return count
}

func (rdc *RandomizerDiversityControls) calculateHealthStatus() string {
	if rdc.config.EmergencyDiversityMode {
		return "Emergency"
	}
	
	if len(rdc.concentrationTracker.criticalRandomizers) > 0 {
		return "Critical"
	}
	
	if len(rdc.concentrationTracker.concentrationAlerts) > 0 {
		return "Warning"
	}
	
	// If no randomizers tracked yet, consider healthy
	if len(rdc.usageTracker) == 0 {
		return "Healthy"
	}
	
	entropy := rdc.entropyCalculator.GetCurrentEntropy()
	uniqueRatio := rdc.calculateUniqueRatio()
	
	if entropy >= rdc.config.MinEntropyBits && 
	   uniqueRatio >= rdc.config.TargetUniqueRatio {
		return "Healthy"
	}
	
	return "Fair"
}

func (rdc *RandomizerDiversityControls) calculateEmergencyDiversityBoost(cid string) float64 {
	// In emergency mode, heavily favor least-used randomizers
	usage, exists := rdc.usageTracker[cid]
	if !exists {
		return 2.0 // Heavily boost new randomizers
	}
	
	// Calculate boost based on inverse usage
	totalSelections := rdc.concentrationTracker.totalSelections
	if totalSelections == 0 {
		return 1.0
	}
	
	usageRatio := float64(usage.UsageCount) / float64(totalSelections)
	return math.Max(0.1, 2.0 * (1.0 - usageRatio))
}

// NewConcentrationTracker creates a new concentration tracker
func NewConcentrationTracker() *ConcentrationTracker {
	return &ConcentrationTracker{
		recentSelections:    make(map[string]int64),
		concentrationAlerts: make(map[string]time.Time),
		criticalRandomizers: make(map[string]bool),
	}
}

// RecordSelection records a randomizer selection
func (ct *ConcentrationTracker) RecordSelection(cid string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	
	ct.totalSelections++
	ct.recentSelections[cid]++
}

// GetConcentrationScore calculates overall concentration score
func (ct *ConcentrationTracker) GetConcentrationScore() float64 {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	
	if ct.totalSelections == 0 {
		return 0.0
	}
	
	// Calculate Herfindahl-Hirschman Index for concentration
	hhi := 0.0
	for _, count := range ct.recentSelections {
		ratio := float64(count) / float64(ct.totalSelections)
		hhi += ratio * ratio
	}
	
	return hhi
}

// Cleanup removes old concentration data
func (ct *ConcentrationTracker) Cleanup(cutoff time.Time) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	
	// Clean old alerts
	for cid, alertTime := range ct.concentrationAlerts {
		if alertTime.Before(cutoff) {
			delete(ct.concentrationAlerts, cid)
		}
	}
}

// NewEntropyCalculator creates a new entropy calculator
func NewEntropyCalculator(sampleWindow int) *EntropyCalculator {
	return &EntropyCalculator{
		sampleWindow:     sampleWindow,
		recentSelections: make([]string, 0, sampleWindow),
		cacheTTL:         5 * time.Minute,
	}
}

// RecordSelection records a selection for entropy calculation
func (ec *EntropyCalculator) RecordSelection(cid string) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	
	ec.recentSelections = append(ec.recentSelections, cid)
	
	// Maintain window size
	if len(ec.recentSelections) > ec.sampleWindow {
		ec.recentSelections = ec.recentSelections[1:]
	}
	
	// Invalidate cache
	ec.cacheTime = time.Time{}
}

// GetCurrentEntropy calculates Shannon entropy of recent selections
func (ec *EntropyCalculator) GetCurrentEntropy() float64 {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	
	// Check cache
	if time.Since(ec.cacheTime) < ec.cacheTTL {
		return ec.entropyCache
	}
	
	// Calculate fresh entropy
	entropy := ec.calculateShannonsEntropy()
	
	// Update cache
	ec.mu.RUnlock()
	ec.mu.Lock()
	ec.entropyCache = entropy
	ec.cacheTime = time.Now()
	ec.mu.Unlock()
	ec.mu.RLock()
	
	return entropy
}

func (ec *EntropyCalculator) calculateShannonsEntropy() float64 {
	if len(ec.recentSelections) == 0 {
		return 0.0
	}
	
	// Count frequencies
	frequencies := make(map[string]int)
	for _, cid := range ec.recentSelections {
		frequencies[cid]++
	}
	
	// Calculate Shannon entropy
	total := float64(len(ec.recentSelections))
	entropy := 0.0
	
	for _, count := range frequencies {
		probability := float64(count) / total
		if probability > 0 {
			entropy -= probability * math.Log2(probability)
		}
	}
	
	return entropy
}

// Cleanup removes old selections
func (ec *EntropyCalculator) Cleanup() {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	
	// Clear cache to force recalculation
	ec.cacheTime = time.Time{}
}

// NewDiversityEnforcer creates a new diversity enforcer
func NewDiversityEnforcer(config *DiversityControlsConfig) *DiversityEnforcer {
	return &DiversityEnforcer{
		config:             config,
		penaltyMultipliers: make(map[string]float64),
		boostMultipliers:   make(map[string]float64),
		lastUpdate:         time.Now(),
	}
}

// UpdateMultipliers updates penalty and boost multipliers for a randomizer
func (de *DiversityEnforcer) UpdateMultipliers(cid string, concentrationScore, diversityScore float64) {
	de.mu.Lock()
	defer de.mu.Unlock()
	
	// Calculate penalty multiplier
	if concentrationScore > de.config.ConcentrationThreshold {
		penalty := math.Max(0.1, 1.0 - (concentrationScore - de.config.ConcentrationThreshold))
		de.penaltyMultipliers[cid] = penalty
	} else {
		delete(de.penaltyMultipliers, cid)
	}
	
	// Calculate boost multiplier - favor low-usage randomizers
	if concentrationScore < 0.1 { // Low concentration gets boost
		boost := 1.0 + (0.1 - concentrationScore) * 10.0 // Stronger boost for less used
		de.boostMultipliers[cid] = boost
	} else {
		delete(de.boostMultipliers, cid)
	}
	
	de.lastUpdate = time.Now()
}
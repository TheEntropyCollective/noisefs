package cache

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"sync"
	"time"
)

// ReplicationBucket represents bucketed replication counts for privacy
type ReplicationBucket int

const (
	ReplicationUnknown ReplicationBucket = iota
	ReplicationLow     // 0-3 replicas
	ReplicationMedium  // 4-10 replicas
	ReplicationHigh    // 10+ replicas
)

// BlockHint provides privacy-safe hints about a block's value
type BlockHint struct {
	// Bucketed replication count
	ReplicationBucket ReplicationBucket
	
	// Request rate with differential privacy noise
	NoisyRequestRate float64
	
	// Whether block has high entropy (good randomizer)
	HighEntropy bool
	
	// Number of geographic regions missing this block
	MissingRegions int
	
	// Last seen timestamp (quantized to hours)
	LastSeen time.Time
	
	// Block size for matching
	Size int
}

// BlockHealth tracks health metrics for a specific block
type BlockHealth struct {
	CID              string
	Hint             BlockHint
	Value            float64   // Calculated value score
	LastUpdated      time.Time
	RequestCount     int64     // Local request count
	LastRequested    time.Time
}

// BlockHealthTracker maintains privacy-safe network health metrics
type BlockHealthTracker struct {
	// Block health data
	blocks map[string]*BlockHealth
	
	// Configuration
	privacyEpsilon   float64       // Differential privacy parameter
	temporalQuantum  time.Duration // Time quantization
	valueCacheTime   time.Duration // How long to cache value calculations
	
	// Metrics
	totalRequests    int64
	lastCleanup      time.Time
	
	// Synchronization
	mu sync.RWMutex
}

// BlockHealthConfig configures the health tracker
type BlockHealthConfig struct {
	PrivacyEpsilon  float64       // Differential privacy (default: 1.0)
	TemporalQuantum time.Duration // Time rounding (default: 1 hour)
	ValueCacheTime  time.Duration // Value cache duration (default: 5 minutes)
	CleanupInterval time.Duration // Cleanup old entries (default: 1 hour)
}

// DefaultBlockHealthConfig returns sensible defaults
func DefaultBlockHealthConfig() *BlockHealthConfig {
	return &BlockHealthConfig{
		PrivacyEpsilon:  1.0,
		TemporalQuantum: time.Hour,
		ValueCacheTime:  5 * time.Minute,
		CleanupInterval: time.Hour,
	}
}

// NewBlockHealthTracker creates a new health tracker
func NewBlockHealthTracker(config *BlockHealthConfig) *BlockHealthTracker {
	if config == nil {
		config = DefaultBlockHealthConfig()
	}
	
	// Validate config values to prevent panics
	if config.CleanupInterval <= 0 {
		config.CleanupInterval = time.Hour
	}
	if config.ValueCacheTime <= 0 {
		config.ValueCacheTime = 5 * time.Minute
	}
	if config.TemporalQuantum <= 0 {
		config.TemporalQuantum = time.Hour
	}
	
	tracker := &BlockHealthTracker{
		blocks:          make(map[string]*BlockHealth),
		privacyEpsilon:  config.PrivacyEpsilon,
		temporalQuantum: config.TemporalQuantum,
		valueCacheTime:  config.ValueCacheTime,
	}
	
	// Start cleanup routine
	go tracker.cleanupLoop(config.CleanupInterval)
	
	return tracker
}

// UpdateBlockHealth updates health metrics for a block
func (bht *BlockHealthTracker) UpdateBlockHealth(cid string, hint BlockHint) {
	bht.mu.Lock()
	defer bht.mu.Unlock()
	
	health, exists := bht.blocks[cid]
	if !exists {
		health = &BlockHealth{
			CID: cid,
		}
		bht.blocks[cid] = health
	}
	
	// Update hint with temporal quantization
	hint.LastSeen = bht.quantizeTime(hint.LastSeen)
	health.Hint = hint
	
	// Recalculate value
	health.Value = bht.calculateBlockValueInternal(hint)
	health.LastUpdated = time.Now()
}

// RecordRequest tracks a local request for a block
func (bht *BlockHealthTracker) RecordRequest(cid string) {
	bht.mu.Lock()
	defer bht.mu.Unlock()
	
	bht.totalRequests++
	
	health, exists := bht.blocks[cid]
	if !exists {
		health = &BlockHealth{
			CID: cid,
			Hint: BlockHint{
				ReplicationBucket: ReplicationUnknown,
			},
		}
		bht.blocks[cid] = health
	}
	
	health.RequestCount++
	health.LastRequested = time.Now()
}

// CalculateBlockValue calculates the value of caching a block
func (bht *BlockHealthTracker) CalculateBlockValue(cid string, hint BlockHint) float64 {
	bht.mu.RLock()
	defer bht.mu.RUnlock()
	
	// Check if we have cached value
	if health, exists := bht.blocks[cid]; exists {
		if time.Since(health.LastUpdated) < bht.valueCacheTime {
			return health.Value
		}
	}
	
	return bht.calculateBlockValueInternal(hint)
}

// GetBlockHint returns the stored hint for a block
func (bht *BlockHealthTracker) GetBlockHint(cid string) BlockHint {
	bht.mu.RLock()
	defer bht.mu.RUnlock()
	
	if health, exists := bht.blocks[cid]; exists {
		return health.Hint
	}
	
	// Return empty hint if not found
	return BlockHint{
		ReplicationBucket: ReplicationUnknown,
	}
}

// calculateBlockValueInternal computes block value from hints
func (bht *BlockHealthTracker) calculateBlockValueInternal(hint BlockHint) float64 {
	value := 0.0
	
	// 1. Replication score (favor under-replicated blocks)
	switch hint.ReplicationBucket {
	case ReplicationLow:
		value += 3.0
	case ReplicationMedium:
		value += 1.0
	case ReplicationHigh:
		value += 0.1
	default:
		value += 0.5 // Unknown replication
	}
	
	// 2. Request frequency (with privacy noise)
	if hint.NoisyRequestRate > 0 {
		// Log scale for request rate
		value += math.Log1p(hint.NoisyRequestRate) * 2.0
	}
	
	// 3. Randomizer potential
	if hint.HighEntropy {
		value += 2.0
	}
	
	// 4. Geographic diversity
	if hint.MissingRegions > 0 {
		value += float64(hint.MissingRegions) * 0.3
	}
	
	// 5. Recency bonus
	if !hint.LastSeen.IsZero() {
		hoursSinceLastSeen := time.Since(hint.LastSeen).Hours()
		if hoursSinceLastSeen < 24 {
			value += 1.0
		} else if hoursSinceLastSeen < 168 { // 1 week
			value += 0.5
		}
	}
	
	return value
}

// GetMostValuableBlocks returns the most valuable blocks to cache
func (bht *BlockHealthTracker) GetMostValuableBlocks(count int, sizeLimit int) []string {
	bht.mu.RLock()
	defer bht.mu.RUnlock()
	
	// Create slice of blocks with values
	type blockValue struct {
		cid   string
		value float64
		size  int
	}
	
	candidates := make([]blockValue, 0, len(bht.blocks))
	for cid, health := range bht.blocks {
		// Skip if size doesn't match requirements
		if sizeLimit > 0 && health.Hint.Size > sizeLimit {
			continue
		}
		
		candidates = append(candidates, blockValue{
			cid:   cid,
			value: health.Value,
			size:  health.Hint.Size,
		})
	}
	
	// Sort by value (descending)
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].value > candidates[i].value {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}
	
	// Return top N
	result := make([]string, 0, count)
	for i := 0; i < len(candidates) && i < count; i++ {
		result = append(result, candidates[i].cid)
	}
	
	return result
}

// AddDifferentialPrivacyNoise adds calibrated noise for privacy
func (bht *BlockHealthTracker) AddDifferentialPrivacyNoise(trueValue float64) float64 {
	if bht.privacyEpsilon <= 0 {
		return trueValue
	}
	
	// Laplace mechanism
	sensitivity := 1.0
	scale := sensitivity / bht.privacyEpsilon
	
	// Simple Laplace noise generation
	// In production, use proper random source
	noise := bht.generateLaplaceNoise(scale)
	
	return math.Max(0, trueValue + noise)
}

// generateLaplaceNoise generates Laplace-distributed noise
func (bht *BlockHealthTracker) generateLaplaceNoise(scale float64) float64 {
	// Use crypto/rand for cryptographically secure randomness
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		// If crypto/rand fails, this is a critical system issue
		panic(fmt.Sprintf("crypto/rand failed for differential privacy: %v", err))
	}
	
	// Convert bytes to uniform float in [-1, 1]
	val := binary.LittleEndian.Uint64(buf)
	uniform := float64(val)/float64(^uint64(0)) // [0, 1]
	uniform = uniform*2 - 1 // [-1, 1]
	
	// Laplace distribution
	if uniform > 0 {
		return -scale * math.Log(1-uniform)
	}
	return scale * math.Log(1+uniform)
}

// quantizeTime rounds time to privacy quantum
func (bht *BlockHealthTracker) quantizeTime(t time.Time) time.Time {
	if bht.temporalQuantum <= 0 {
		return t
	}
	
	quantum := int64(bht.temporalQuantum)
	quantized := (t.UnixNano() / quantum) * quantum
	return time.Unix(0, quantized)
}

// GetBlockRequestRate returns the request rate with privacy noise
func (bht *BlockHealthTracker) GetBlockRequestRate(cid string) float64 {
	bht.mu.RLock()
	defer bht.mu.RUnlock()
	
	health, exists := bht.blocks[cid]
	if !exists {
		return 0
	}
	
	// Calculate rate per hour
	timeSinceFirst := time.Since(health.LastUpdated)
	if timeSinceFirst < time.Hour {
		timeSinceFirst = time.Hour
	}
	
	rate := float64(health.RequestCount) / timeSinceFirst.Hours()
	
	// Add differential privacy noise
	return bht.AddDifferentialPrivacyNoise(rate)
}

// AnalyzeBlockEntropy determines if a block has high entropy
func AnalyzeBlockEntropy(data []byte) bool {
	if len(data) < 256 {
		return false
	}
	
	// Simple entropy estimation using byte frequency
	freq := make(map[byte]int)
	for _, b := range data[:256] { // Sample first 256 bytes
		freq[b]++
	}
	
	// High entropy if bytes are well distributed
	uniqueBytes := len(freq)
	return uniqueBytes > 200 // 200+ unique bytes in 256 = high entropy
}

// GetReplicationBucket converts exact count to privacy bucket
func GetReplicationBucket(exactCount int) ReplicationBucket {
	switch {
	case exactCount <= 3:
		return ReplicationLow
	case exactCount <= 10:
		return ReplicationMedium
	default:
		return ReplicationHigh
	}
}

// cleanupLoop periodically removes old entries
func (bht *BlockHealthTracker) cleanupLoop(interval time.Duration) {
	// Validate interval to prevent NewTicker panic
	if interval <= 0 {
		interval = time.Hour // Fallback to safe default
	}
	
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	
	for range ticker.C {
		bht.cleanup()
	}
}

// cleanup removes stale entries
func (bht *BlockHealthTracker) cleanup() {
	bht.mu.Lock()
	defer bht.mu.Unlock()
	
	cutoff := time.Now().Add(-24 * time.Hour)
	
	for cid, health := range bht.blocks {
		// Remove if not updated or requested recently
		if health.LastUpdated.Before(cutoff) && health.LastRequested.Before(cutoff) {
			delete(bht.blocks, cid)
		}
	}
	
	bht.lastCleanup = time.Now()
}

// GetStats returns tracker statistics
func (bht *BlockHealthTracker) GetStats() map[string]interface{} {
	bht.mu.RLock()
	defer bht.mu.RUnlock()
	
	lowRep := 0
	medRep := 0
	highRep := 0
	
	for _, health := range bht.blocks {
		switch health.Hint.ReplicationBucket {
		case ReplicationLow:
			lowRep++
		case ReplicationMedium:
			medRep++
		case ReplicationHigh:
			highRep++
		}
	}
	
	return map[string]interface{}{
		"total_blocks":     len(bht.blocks),
		"total_requests":   bht.totalRequests,
		"low_replication":  lowRep,
		"med_replication":  medRep,
		"high_replication": highRep,
		"last_cleanup":     bht.lastCleanup,
	}
}

// CreateBlockID generates a privacy-safe block identifier
func CreateBlockID(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:16]) // First 16 bytes
}

// GetBlockValue returns the calculated value score for a block
func (bht *BlockHealthTracker) GetBlockValue(cid string) float64 {
	bht.mu.RLock()
	defer bht.mu.RUnlock()
	
	health, exists := bht.blocks[cid]
	if !exists {
		return 0
	}
	
	// Return cached value if still valid
	if time.Since(health.LastUpdated) < bht.valueCacheTime {
		return health.Value
	}
	
	// Recalculate value
	value := bht.CalculateBlockValue(cid, health.Hint)
	health.Value = value
	health.LastUpdated = time.Now()
	
	return value
}

// GetOpportunisticFetcher returns the opportunistic fetcher instance
// This is a placeholder - the actual implementation should return the real fetcher
func (bht *BlockHealthTracker) GetOpportunisticFetcher() interface{} {
	// TODO: Implement proper opportunistic fetcher integration
	return nil
}

// GetAllBlockHints returns all block hints for gossip protocol
func (bht *BlockHealthTracker) GetAllBlockHints() map[string]BlockHint {
	bht.mu.RLock()
	defer bht.mu.RUnlock()
	
	hints := make(map[string]BlockHint)
	for cid, health := range bht.blocks {
		hints[cid] = health.Hint
	}
	
	return hints
}
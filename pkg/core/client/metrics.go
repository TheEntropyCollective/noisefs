// Package noisefs provides performance metrics and monitoring functionality.
// This file handles tracking of upload/download statistics, cache efficiency,
// availability integration, and system performance metrics.
package noisefs

import (
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
)

// Metrics tracks NoiseFS performance and efficiency metrics
type Metrics struct {
	mu                    sync.RWMutex
	BlocksReused          int64 // Number of blocks reused from cache
	BlocksGenerated       int64 // Number of new blocks generated
	CacheHits             int64 // Number of cache hits
	CacheMisses           int64 // Number of cache misses
	TotalUploads          int64 // Total files uploaded
	TotalDownloads        int64 // Total files downloaded
	BytesUploadedOriginal int64 // Original bytes uploaded
	BytesStoredIPFS       int64 // Actual bytes stored in IPFS

	// Health monitoring (Week 1 implementation)
	healthMonitor *cache.CacheHealthMonitor
	startTime     time.Time
}

// NewMetrics creates a new metrics tracker
func NewMetrics() *Metrics {
	return &Metrics{
		healthMonitor: cache.NewCacheHealthMonitor(),
		startTime:     time.Now(),
	}
}

// RecordBlockReuse increments the block reuse counter
func (m *Metrics) RecordBlockReuse() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.BlocksReused++
}

// RecordBlockGeneration increments the block generation counter
func (m *Metrics) RecordBlockGeneration() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.BlocksGenerated++
}

// RecordCacheHit increments the cache hit counter
func (m *Metrics) RecordCacheHit() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CacheHits++
}

// RecordCacheMiss increments the cache miss counter
func (m *Metrics) RecordCacheMiss() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CacheMisses++
}

// RecordUpload records a file upload
func (m *Metrics) RecordUpload(originalBytes, storedBytes int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TotalUploads++
	m.BytesUploadedOriginal += originalBytes
	m.BytesStoredIPFS += storedBytes
}

// RecordDownload increments the download counter
func (m *Metrics) RecordDownload() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TotalDownloads++
}

// GetStats returns a snapshot of current metrics
func (m *Metrics) GetStats() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Get health scores if monitor is available
	var healthScore *cache.HealthScore
	if m.healthMonitor != nil {
		healthScore = m.healthMonitor.CalculateHealthScore()
	}

	snapshot := MetricsSnapshot{
		BlocksReused:          m.BlocksReused,
		BlocksGenerated:       m.BlocksGenerated,
		CacheHits:             m.CacheHits,
		CacheMisses:           m.CacheMisses,
		TotalUploads:          m.TotalUploads,
		TotalDownloads:        m.TotalDownloads,
		BytesUploadedOriginal: m.BytesUploadedOriginal,
		BytesStoredIPFS:       m.BytesStoredIPFS,
		BlockReuseRate:        m.calculateBlockReuseRate(),
		CacheHitRate:          m.calculateCacheHitRate(),
		StorageEfficiency:     m.calculateStorageEfficiency(),
	}

	// Add health metrics if available
	if healthScore != nil {
		snapshot.CacheHealthScore = healthScore.Overall
		snapshot.RandomizerDiversity = healthScore.RandomizerDiversity
		snapshot.AverageBlockAge = healthScore.AverageBlockAge
		snapshot.MemoryPressure = healthScore.MemoryPressure
		snapshot.EvictionRate = healthScore.EvictionRate
		snapshot.CoordinationHealth = healthScore.CoordinationHealth
		snapshot.AvailabilityHealth = healthScore.AvailabilityHealth
	}

	return snapshot
}

// MetricsSnapshot represents a point-in-time view of metrics
type MetricsSnapshot struct {
	BlocksReused          int64   `json:"blocks_reused"`
	BlocksGenerated       int64   `json:"blocks_generated"`
	CacheHits             int64   `json:"cache_hits"`
	CacheMisses           int64   `json:"cache_misses"`
	TotalUploads          int64   `json:"total_uploads"`
	TotalDownloads        int64   `json:"total_downloads"`
	BytesUploadedOriginal int64   `json:"bytes_uploaded_original"`
	BytesStoredIPFS       int64   `json:"bytes_stored_ipfs"`
	BlockReuseRate        float64 `json:"block_reuse_rate"`
	CacheHitRate          float64 `json:"cache_hit_rate"`
	StorageEfficiency     float64 `json:"storage_efficiency"`

	// Cache Health Metrics (Week 1 implementation)
	CacheHealthScore    float64 `json:"cache_health_score"`   // Overall health 0-1
	RandomizerDiversity float64 `json:"randomizer_diversity"` // Entropy measure 0-1
	AverageBlockAge     float64 `json:"average_block_age"`    // Hours since last access
	MemoryPressure      float64 `json:"memory_pressure"`      // Memory usage 0-1
	EvictionRate        float64 `json:"eviction_rate"`        // Evictions per hour
	CoordinationHealth  float64 `json:"coordination_health"`  // Network coordination 0-1
	AvailabilityHealth  float64 `json:"availability_health"`  // Randomizer availability 0-1
}

// calculateBlockReuseRate returns the percentage of blocks that were reused
func (m *Metrics) calculateBlockReuseRate() float64 {
	total := m.BlocksReused + m.BlocksGenerated
	if total == 0 {
		return 0.0
	}
	return float64(m.BlocksReused) / float64(total) * 100.0
}

// calculateCacheHitRate returns the cache hit percentage
func (m *Metrics) calculateCacheHitRate() float64 {
	total := m.CacheHits + m.CacheMisses
	if total == 0 {
		return 0.0
	}
	return float64(m.CacheHits) / float64(total) * 100.0
}

// calculateStorageEfficiency returns the storage overhead percentage
func (m *Metrics) calculateStorageEfficiency() float64 {
	if m.BytesUploadedOriginal == 0 {
		return 0.0
	}
	overhead := float64(m.BytesStoredIPFS) / float64(m.BytesUploadedOriginal) * 100.0
	return overhead
}

// SetHealthMonitorComponents configures health monitor with cache components
func (m *Metrics) SetHealthMonitorComponents(ce *cache.CoordinationEngine, ht *cache.BlockHealthTracker) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.healthMonitor != nil {
		if ce != nil {
			m.healthMonitor.SetCoordinationEngine(ce)
		}
		if ht != nil {
			m.healthMonitor.SetHealthTracker(ht)
		}
	}
}

// SetAvailabilityIntegration configures health monitor with availability integration
func (m *Metrics) SetAvailabilityIntegration(ai *cache.AvailabilityIntegration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.healthMonitor != nil && ai != nil {
		m.healthMonitor.SetAvailabilityIntegration(ai)
	}
}

// UpdateHealthMetrics updates the health monitor with current cache state
func (m *Metrics) UpdateHealthMetrics(metrics *cache.HealthMetrics) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.healthMonitor != nil {
		m.healthMonitor.UpdateMetrics(metrics)
	}
}

// RecordEviction records a cache eviction for health tracking
func (m *Metrics) RecordEviction() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.healthMonitor != nil {
		m.healthMonitor.RecordEviction()
	}
}

// GetHealthSummary returns a human-readable health summary
func (m *Metrics) GetHealthSummary() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.healthMonitor != nil {
		return m.healthMonitor.GetHealthSummary()
	}
	return "Unknown"
}

package subsystems

import (
	"sync"

	noisefs "github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/TheEntropyCollective/noisefs/pkg/privacy/relay"
	"github.com/TheEntropyCollective/noisefs/pkg/privacy/reuse"
)

// SystemMetrics tracks overall system performance
type SystemMetrics struct {
	TotalUploads      int64
	TotalDownloads    int64
	TotalBlocks       int64
	ReuseRatio        float64
	CoverTrafficRatio float64
	StorageEfficiency float64
	PrivacyScore      float64
}

// MetricsSubsystem manages all metrics-related functionality
type MetricsSubsystem struct {
	systemMetrics *SystemMetrics
	mu            sync.RWMutex
}

// NewMetricsSubsystem creates a new metrics subsystem
func NewMetricsSubsystem() *MetricsSubsystem {
	return &MetricsSubsystem{
		systemMetrics: &SystemMetrics{},
	}
}

// GetSystemMetrics returns current system metrics
func (m *MetricsSubsystem) GetSystemMetrics(
	reuseClient *reuse.ReuseAwareClient,
	coverTraffic *relay.CoverTrafficGenerator,
	noisefsClient *noisefs.Client,
) *SystemMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Calculate current metrics
	metrics := *m.systemMetrics

	// Get reuse statistics
	if reuseClient != nil {
		reuseStats := reuseClient.GetReuseStatistics()
		if poolStats, ok := reuseStats["pool"].(map[string]interface{}); ok {
			if reuseRatio, ok := poolStats["average_reuse_count"].(float64); ok {
				metrics.ReuseRatio = reuseRatio
			}
		}
	}

	// Get cover traffic statistics
	if coverTraffic != nil {
		coverStats := coverTraffic.GetMetrics()
		if coverStats.TotalCoverRequests > 0 {
			metrics.CoverTrafficRatio = coverStats.NoiseRatioAchieved
		}
	}

	// Calculate storage efficiency from real metrics
	if noisefsClient != nil {
		clientMetrics := noisefsClient.GetMetrics()
		if clientMetrics.BytesStoredIPFS > 0 {
			// Calculate efficiency as ratio of original bytes to stored bytes
			// 1.0 = perfect efficiency, <1.0 = storage overhead exists
			metrics.StorageEfficiency = float64(clientMetrics.BytesUploadedOriginal) / float64(clientMetrics.BytesStoredIPFS)
		} else {
			// No data uploaded yet
			metrics.StorageEfficiency = 0.0
		}
	}

	return &metrics
}

// IncrementUploads increments the upload counter
func (m *MetricsSubsystem) IncrementUploads() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.systemMetrics.TotalUploads++
}

// IncrementDownloads increments the download counter
func (m *MetricsSubsystem) IncrementDownloads() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.systemMetrics.TotalDownloads++
}

// UpdateMetricsFromUpload updates metrics based on upload result
func (m *MetricsSubsystem) UpdateMetricsFromUpload(result *reuse.UploadResult, coverTraffic *relay.CoverTrafficGenerator) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if result.MixingPlan != nil {
		m.systemMetrics.TotalBlocks += int64(result.MixingPlan.TotalBlocks)
	}

	m.systemMetrics.PrivacyScore = m.calculatePrivacyScore(result, coverTraffic)
}

// calculatePrivacyScore calculates the privacy score for an upload
func (m *MetricsSubsystem) calculatePrivacyScore(result *reuse.UploadResult, coverTraffic *relay.CoverTrafficGenerator) float64 {
	score := 0.7 // Base score

	// Add points for reuse
	if result.ReuseProof != nil {
		score += 0.1
	}

	// Add points for public domain mixing
	if result.MixingPlan != nil && result.MixingPlan.PublicDomainBlocks > 0 {
		score += 0.1
	}

	// Add points for cover traffic
	if coverTraffic != nil && coverTraffic.GetMetrics().TotalCoverRequests > 0 {
		score += 0.1
	}

	return score
}

// Shutdown gracefully shuts down the metrics subsystem
func (m *MetricsSubsystem) Shutdown() error {
	// Metrics components cleanup would go here
	return nil
}
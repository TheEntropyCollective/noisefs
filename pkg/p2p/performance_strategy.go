package p2p

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// PerformanceStrategy selects peers based on performance metrics
type PerformanceStrategy struct {
	peerManager *PeerManager
	metrics     map[peer.ID]*PerformanceMetrics
	mutex       sync.RWMutex
	
	// Configuration
	minSuccessRate    float64
	maxLatency        time.Duration
	bandwidthWeight   float64
	latencyWeight     float64
	reliabilityWeight float64
}

// PerformanceMetrics tracks detailed performance data for a peer
type PerformanceMetrics struct {
	PeerID              peer.ID           `json:"peer_id"`
	LastUpdate          time.Time         `json:"last_update"`
	
	// Request tracking
	TotalRequests       int64             `json:"total_requests"`
	SuccessfulRequests  int64             `json:"successful_requests"`
	FailedRequests      int64             `json:"failed_requests"`
	
	// Timing metrics
	TotalLatency        time.Duration     `json:"total_latency"`
	MinLatency          time.Duration     `json:"min_latency"`
	MaxLatency          time.Duration     `json:"max_latency"`
	AverageLatency      time.Duration     `json:"average_latency"`
	
	// Bandwidth metrics
	TotalBytes          int64             `json:"total_bytes"`
	TotalTime           time.Duration     `json:"total_time"`
	AverageBandwidth    float64           `json:"average_bandwidth_mbps"`
	PeakBandwidth       float64           `json:"peak_bandwidth_mbps"`
	
	// Reliability metrics
	SuccessRate         float64           `json:"success_rate"`
	ConsecutiveFailures int               `json:"consecutive_failures"`
	LastSuccess         time.Time         `json:"last_success"`
	LastFailure         time.Time         `json:"last_failure"`
	
	// Performance score
	PerformanceScore    float64           `json:"performance_score"`
	
	mutex               sync.RWMutex
}

// NewPerformanceStrategy creates a new performance-based peer selection strategy
func NewPerformanceStrategy(pm *PeerManager) *PerformanceStrategy {
	return &PerformanceStrategy{
		peerManager:       pm,
		metrics:           make(map[peer.ID]*PerformanceMetrics),
		minSuccessRate:    0.7,  // 70% minimum success rate
		maxLatency:        5 * time.Second,
		bandwidthWeight:   0.3,
		latencyWeight:     0.4,
		reliabilityWeight: 0.3,
	}
}

// SelectPeers selects peers based on performance criteria
func (ps *PerformanceStrategy) SelectPeers(ctx context.Context, criteria SelectionCriteria) ([]peer.ID, error) {
	ps.mutex.RLock()
	defer ps.mutex.RUnlock()
	
	// Get all healthy peers from peer manager
	healthyPeers := ps.peerManager.GetHealthyPeers()
	if len(healthyPeers) == 0 {
		return nil, fmt.Errorf("no healthy peers available")
	}
	
	// Filter and score peers based on criteria
	candidates := ps.filterAndScorePeers(healthyPeers, criteria)
	
	// Sort by performance score (descending)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})
	
	// Select top performers
	count := criteria.Count
	if count > len(candidates) {
		count = len(candidates)
	}
	
	result := make([]peer.ID, count)
	for i := 0; i < count; i++ {
		result[i] = candidates[i].PeerID
	}
	
	return result, nil
}

// PeerCandidate represents a peer with its performance score
type PeerCandidate struct {
	PeerID peer.ID
	Score  float64
}

// filterAndScorePeers filters peers based on criteria and calculates performance scores
func (ps *PerformanceStrategy) filterAndScorePeers(peers []peer.ID, criteria SelectionCriteria) []PeerCandidate {
	var candidates []PeerCandidate
	
	for _, peerID := range peers {
		// Skip excluded peers
		if ps.isPeerExcluded(peerID, criteria.ExcludePeers) {
			continue
		}
		
		metrics := ps.getOrCreateMetrics(peerID)
		metrics.mutex.RLock()
		
		// Apply filters
		if !ps.meetsCriteria(metrics, criteria) {
			metrics.mutex.RUnlock()
			continue
		}
		
		// Calculate performance score
		score := ps.calculatePerformanceScore(metrics)
		metrics.mutex.RUnlock()
		
		candidates = append(candidates, PeerCandidate{
			PeerID: peerID,
			Score:  score,
		})
	}
	
	return candidates
}

// isPeerExcluded checks if a peer is in the exclusion list
func (ps *PerformanceStrategy) isPeerExcluded(peerID peer.ID, excludeList []peer.ID) bool {
	for _, excluded := range excludeList {
		if peerID == excluded {
			return true
		}
	}
	return false
}

// meetsCriteria checks if a peer meets the selection criteria
func (ps *PerformanceStrategy) meetsCriteria(metrics *PerformanceMetrics, criteria SelectionCriteria) bool {
	// Check minimum bandwidth requirement
	if criteria.MinBandwidth > 0 && metrics.AverageBandwidth < criteria.MinBandwidth {
		return false
	}
	
	// Check maximum latency requirement
	if criteria.MaxLatency > 0 && metrics.AverageLatency > criteria.MaxLatency {
		return false
	}
	
	// Check minimum success rate
	if metrics.SuccessRate < ps.minSuccessRate {
		return false
	}
	
	// Check if peer has too many consecutive failures
	if metrics.ConsecutiveFailures > 5 {
		return false
	}
	
	return true
}

// calculatePerformanceScore calculates a composite performance score for a peer
func (ps *PerformanceStrategy) calculatePerformanceScore(metrics *PerformanceMetrics) float64 {
	if metrics.TotalRequests < 3 {
		return 0.5 // Neutral score for new peers
	}
	
	// Latency score (lower is better)
	latencyScore := 1.0
	if metrics.AverageLatency > 0 {
		latencyScore = 1.0 / (1.0 + metrics.AverageLatency.Seconds())
	}
	
	// Bandwidth score (higher is better, normalize to 10MB/s)
	bandwidthScore := 0.0
	if metrics.AverageBandwidth > 0 {
		bandwidthScore = metrics.AverageBandwidth / 10.0
		if bandwidthScore > 1.0 {
			bandwidthScore = 1.0
		}
	}
	
	// Reliability score
	reliabilityScore := metrics.SuccessRate
	
	// Penalty for consecutive failures
	failurePenalty := 1.0
	if metrics.ConsecutiveFailures > 0 {
		failurePenalty = 1.0 / (1.0 + float64(metrics.ConsecutiveFailures)*0.1)
	}
	
	// Recency bonus (prefer recently active peers)
	recencyBonus := 1.0
	timeSinceLastSuccess := time.Since(metrics.LastSuccess)
	if timeSinceLastSuccess < time.Hour {
		recencyBonus = 1.1
	} else if timeSinceLastSuccess > 24*time.Hour {
		recencyBonus = 0.9
	}
	
	// Composite score
	score := (latencyScore*ps.latencyWeight +
		bandwidthScore*ps.bandwidthWeight +
		reliabilityScore*ps.reliabilityWeight) *
		failurePenalty * recencyBonus
	
	return score
}

// UpdateMetrics updates performance metrics for a peer
func (ps *PerformanceStrategy) UpdateMetrics(peerID peer.ID, success bool, latency time.Duration, bytes int64) {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	
	metrics := ps.getOrCreateMetrics(peerID)
	metrics.mutex.Lock()
	defer metrics.mutex.Unlock()
	
	now := time.Now()
	metrics.LastUpdate = now
	metrics.TotalRequests++
	
	if success {
		metrics.SuccessfulRequests++
		metrics.ConsecutiveFailures = 0
		metrics.LastSuccess = now
	} else {
		metrics.FailedRequests++
		metrics.ConsecutiveFailures++
		metrics.LastFailure = now
	}
	
	// Update latency metrics
	metrics.TotalLatency += latency
	if metrics.MinLatency == 0 || latency < metrics.MinLatency {
		metrics.MinLatency = latency
	}
	if latency > metrics.MaxLatency {
		metrics.MaxLatency = latency
	}
	metrics.AverageLatency = metrics.TotalLatency / time.Duration(metrics.TotalRequests)
	
	// Update bandwidth metrics
	if success && bytes > 0 && latency > 0 {
		metrics.TotalBytes += bytes
		metrics.TotalTime += latency
		
		// Calculate average bandwidth in MB/s
		if metrics.TotalTime > 0 {
			metrics.AverageBandwidth = float64(metrics.TotalBytes) / (1024 * 1024) / metrics.TotalTime.Seconds()
		}
		
		// Calculate instantaneous bandwidth
		instantBandwidth := float64(bytes) / (1024 * 1024) / latency.Seconds()
		if instantBandwidth > metrics.PeakBandwidth {
			metrics.PeakBandwidth = instantBandwidth
		}
	}
	
	// Update success rate
	if metrics.TotalRequests > 0 {
		metrics.SuccessRate = float64(metrics.SuccessfulRequests) / float64(metrics.TotalRequests)
	}
	
	// Update performance score
	metrics.PerformanceScore = ps.calculatePerformanceScore(metrics)
}

// GetPeerInfo returns performance information for a specific peer
func (ps *PerformanceStrategy) GetPeerInfo(peerID peer.ID) (*PeerInfo, bool) {
	ps.mutex.RLock()
	defer ps.mutex.RUnlock()
	
	if metrics, exists := ps.metrics[peerID]; exists {
		metrics.mutex.RLock()
		defer metrics.mutex.RUnlock()
		
		// Convert to PeerInfo format
		peerInfo := &PeerInfo{
			ID:              peerID,
			LastSeen:        metrics.LastUpdate,
			Latency:         metrics.AverageLatency,
			Bandwidth:       metrics.AverageBandwidth,
			SuccessRate:     metrics.SuccessRate,
			TotalRequests:   metrics.TotalRequests,
			SuccessRequests: metrics.SuccessfulRequests,
			TotalBytes:      metrics.TotalBytes,
			TotalTime:       metrics.TotalTime,
		}
		
		return peerInfo, true
	}
	
	return nil, false
}

// getOrCreateMetrics gets existing metrics or creates new ones for a peer
func (ps *PerformanceStrategy) getOrCreateMetrics(peerID peer.ID) *PerformanceMetrics {
	if metrics, exists := ps.metrics[peerID]; exists {
		return metrics
	}
	
	metrics := &PerformanceMetrics{
		PeerID:     peerID,
		LastUpdate: time.Now(),
	}
	ps.metrics[peerID] = metrics
	
	return metrics
}

// GetTopPerformers returns the top N performing peers
func (ps *PerformanceStrategy) GetTopPerformers(count int) []peer.ID {
	ps.mutex.RLock()
	defer ps.mutex.RUnlock()
	
	type peerScore struct {
		PeerID peer.ID
		Score  float64
	}
	
	var scores []peerScore
	for peerID, metrics := range ps.metrics {
		metrics.mutex.RLock()
		if metrics.TotalRequests >= 3 { // Only consider peers with enough data
			scores = append(scores, peerScore{
				PeerID: peerID,
				Score:  metrics.PerformanceScore,
			})
		}
		metrics.mutex.RUnlock()
	}
	
	// Sort by score (descending)
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})
	
	// Return top performers
	result := make([]peer.ID, 0, count)
	for i, score := range scores {
		if i >= count {
			break
		}
		result = append(result, score.PeerID)
	}
	
	return result
}

// GetWorstPerformers returns the worst N performing peers
func (ps *PerformanceStrategy) GetWorstPerformers(count int) []peer.ID {
	ps.mutex.RLock()
	defer ps.mutex.RUnlock()
	
	type peerScore struct {
		PeerID peer.ID
		Score  float64
	}
	
	var scores []peerScore
	for peerID, metrics := range ps.metrics {
		metrics.mutex.RLock()
		if metrics.TotalRequests >= 3 {
			scores = append(scores, peerScore{
				PeerID: peerID,
				Score:  metrics.PerformanceScore,
			})
		}
		metrics.mutex.RUnlock()
	}
	
	// Sort by score (ascending)
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score < scores[j].Score
	})
	
	// Return worst performers
	result := make([]peer.ID, 0, count)
	for i, score := range scores {
		if i >= count {
			break
		}
		result = append(result, score.PeerID)
	}
	
	return result
}

// GetMetricsStats returns statistics about the performance metrics
func (ps *PerformanceStrategy) GetMetricsStats() map[string]interface{} {
	ps.mutex.RLock()
	defer ps.mutex.RUnlock()
	
	totalPeers := len(ps.metrics)
	totalRequests := int64(0)
	totalSuccesses := int64(0)
	totalBandwidth := 0.0
	avgLatency := time.Duration(0)
	
	validPeers := 0
	for _, metrics := range ps.metrics {
		metrics.mutex.RLock()
		if metrics.TotalRequests > 0 {
			validPeers++
			totalRequests += metrics.TotalRequests
			totalSuccesses += metrics.SuccessfulRequests
			totalBandwidth += metrics.AverageBandwidth
			avgLatency += metrics.AverageLatency
		}
		metrics.mutex.RUnlock()
	}
	
	stats := map[string]interface{}{
		"total_peers":    totalPeers,
		"valid_peers":    validPeers,
		"total_requests": totalRequests,
		"overall_success_rate": 0.0,
		"avg_bandwidth": 0.0,
		"avg_latency": "0s",
	}
	
	if totalRequests > 0 {
		stats["overall_success_rate"] = float64(totalSuccesses) / float64(totalRequests)
	}
	
	if validPeers > 0 {
		stats["avg_bandwidth"] = totalBandwidth / float64(validPeers)
		stats["avg_latency"] = (avgLatency / time.Duration(validPeers)).String()
	}
	
	return stats
}

// CleanupOldMetrics removes metrics for peers that haven't been seen recently
func (ps *PerformanceStrategy) CleanupOldMetrics(maxAge time.Duration) {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()
	
	cutoff := time.Now().Add(-maxAge)
	
	for peerID, metrics := range ps.metrics {
		metrics.mutex.RLock()
		lastUpdate := metrics.LastUpdate
		metrics.mutex.RUnlock()
		
		if lastUpdate.Before(cutoff) {
			delete(ps.metrics, peerID)
		}
	}
}
package p2p

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// HybridStrategy combines performance, randomizer, and privacy strategies
type HybridStrategy struct {
	peerManager         *PeerManager
	performanceStrategy *PerformanceStrategy
	randomizerStrategy  *RandomizerStrategy
	privacyStrategy     *PrivacyStrategy
	
	// Strategy weights
	performanceWeight   float64
	randomizerWeight    float64
	privacyWeight       float64
	
	// Configuration
	adaptiveWeights     bool
	contextAware        bool
	mutex               sync.RWMutex
}

// HybridPeerScore represents a peer with composite scoring from all strategies
type HybridPeerScore struct {
	PeerID            peer.ID `json:"peer_id"`
	PerformanceScore  float64 `json:"performance_score"`
	RandomizerScore   float64 `json:"randomizer_score"`
	PrivacyScore      float64 `json:"privacy_score"`
	CompositeScore    float64 `json:"composite_score"`
	SelectedStrategy  string  `json:"selected_strategy"`
}

// NewHybridStrategy creates a new hybrid peer selection strategy
func NewHybridStrategy(pm *PeerManager) *HybridStrategy {
	return &HybridStrategy{
		peerManager:         pm,
		performanceStrategy: NewPerformanceStrategy(pm),
		randomizerStrategy:  NewRandomizerStrategy(pm),
		privacyStrategy:     NewPrivacyStrategy(pm),
		performanceWeight:   0.4,
		randomizerWeight:    0.3,
		privacyWeight:       0.3,
		adaptiveWeights:     true,
		contextAware:        true,
	}
}

// SelectPeers selects peers using hybrid approach
func (hs *HybridStrategy) SelectPeers(ctx context.Context, criteria SelectionCriteria) ([]peer.ID, error) {
	hs.mutex.RLock()
	defer hs.mutex.RUnlock()
	
	// Adapt strategy weights based on criteria
	weights := hs.calculateAdaptiveWeights(criteria)
	
	// Get healthy peers
	healthyPeers := hs.peerManager.GetHealthyPeers()
	if len(healthyPeers) == 0 {
		return nil, fmt.Errorf("no healthy peers available")
	}
	
	// Score peers using all strategies
	hybridScores := hs.calculateHybridScores(ctx, healthyPeers, criteria, weights)
	
	// Sort by composite score
	sort.Slice(hybridScores, func(i, j int) bool {
		return hybridScores[i].CompositeScore > hybridScores[j].CompositeScore
	})
	
	// Apply diversity selection if needed
	if criteria.RequirePrivacy {
		hybridScores = hs.applyDiversitySelection(hybridScores)
	}
	
	// Select top peers
	count := criteria.Count
	if count > len(hybridScores) {
		count = len(hybridScores)
	}
	
	result := make([]peer.ID, count)
	for i := 0; i < count; i++ {
		result[i] = hybridScores[i].PeerID
	}
	
	return result, nil
}

// calculateAdaptiveWeights adjusts strategy weights based on selection criteria
func (hs *HybridStrategy) calculateAdaptiveWeights(criteria SelectionCriteria) map[string]float64 {
	weights := map[string]float64{
		"performance": hs.performanceWeight,
		"randomizer":  hs.randomizerWeight,
		"privacy":     hs.privacyWeight,
	}
	
	if !hs.adaptiveWeights {
		return weights
	}
	
	// Adjust weights based on criteria
	if criteria.MinBandwidth > 0 || criteria.MaxLatency > 0 {
		// Performance is critical
		weights["performance"] += 0.2
		weights["randomizer"] -= 0.1
		weights["privacy"] -= 0.1
	}
	
	if criteria.PreferRandomizers || len(criteria.RequiredBlocks) > 0 {
		// Randomizer selection is important
		weights["randomizer"] += 0.2
		weights["performance"] -= 0.1
		weights["privacy"] -= 0.1
	}
	
	if criteria.RequirePrivacy {
		// Privacy is paramount
		weights["privacy"] += 0.3
		weights["performance"] -= 0.15
		weights["randomizer"] -= 0.15
	}
	
	// Normalize weights to sum to 1.0
	total := weights["performance"] + weights["randomizer"] + weights["privacy"]
	if total > 0 {
		weights["performance"] /= total
		weights["randomizer"] /= total
		weights["privacy"] /= total
	}
	
	return weights
}

// calculateHybridScores calculates composite scores from all strategies
func (hs *HybridStrategy) calculateHybridScores(ctx context.Context, peers []peer.ID, criteria SelectionCriteria, weights map[string]float64) []HybridPeerScore {
	var scores []HybridPeerScore
	
	for _, peerID := range peers {
		// Skip excluded peers
		if hs.isPeerExcluded(peerID, criteria.ExcludePeers) {
			continue
		}
		
		hybridScore := HybridPeerScore{
			PeerID: peerID,
		}
		
		// Get performance score
		if perfInfo, exists := hs.performanceStrategy.GetPeerInfo(peerID); exists {
			hybridScore.PerformanceScore = perfInfo.GetPerformanceScore()
		}
		
		// Get randomizer score  
		if randInfo, exists := hs.randomizerStrategy.GetPeerInfo(peerID); exists {
			hybridScore.RandomizerScore = randInfo.RandomizerScore
		}
		
		// Get privacy score
		if privInfo, exists := hs.privacyStrategy.GetPeerInfo(peerID); exists {
			hybridScore.PrivacyScore = privInfo.Reputation
		}
		
		// Calculate composite score
		hybridScore.CompositeScore = 
			hybridScore.PerformanceScore * weights["performance"] +
			hybridScore.RandomizerScore * weights["randomizer"] +
			hybridScore.PrivacyScore * weights["privacy"]
		
		// Determine which strategy contributed most
		hybridScore.SelectedStrategy = hs.getDominantStrategy(hybridScore, weights)
		
		scores = append(scores, hybridScore)
	}
	
	return scores
}

// getDominantStrategy determines which strategy contributed most to the score
func (hs *HybridStrategy) getDominantStrategy(score HybridPeerScore, weights map[string]float64) string {
	performanceContribution := score.PerformanceScore * weights["performance"]
	randomizerContribution := score.RandomizerScore * weights["randomizer"]
	privacyContribution := score.PrivacyScore * weights["privacy"]
	
	if performanceContribution >= randomizerContribution && performanceContribution >= privacyContribution {
		return "performance"
	} else if randomizerContribution >= privacyContribution {
		return "randomizer"
	} else {
		return "privacy"
	}
}

// applyDiversitySelection ensures diversity in selected peers for privacy
func (hs *HybridStrategy) applyDiversitySelection(scores []HybridPeerScore) []HybridPeerScore {
	// Group by dominant strategy
	strategyGroups := make(map[string][]HybridPeerScore)
	for _, score := range scores {
		strategy := score.SelectedStrategy
		strategyGroups[strategy] = append(strategyGroups[strategy], score)
	}
	
	// Select from each strategy group to ensure diversity
	var diverseScores []HybridPeerScore
	maxPerStrategy := len(scores) / 3 // Roughly equal distribution
	
	for _, group := range strategyGroups {
		// Sort group by score
		sort.Slice(group, func(i, j int) bool {
			return group[i].CompositeScore > group[j].CompositeScore
		})
		
		// Take top peers from this strategy
		count := maxPerStrategy
		if count > len(group) {
			count = len(group)
		}
		
		diverseScores = append(diverseScores, group[:count]...)
	}
	
	// Sort the diverse selection by composite score
	sort.Slice(diverseScores, func(i, j int) bool {
		return diverseScores[i].CompositeScore > diverseScores[j].CompositeScore
	})
	
	return diverseScores
}

// UpdateMetrics updates metrics across all strategies
func (hs *HybridStrategy) UpdateMetrics(peerID peer.ID, success bool, latency time.Duration, bytes int64) {
	hs.performanceStrategy.UpdateMetrics(peerID, success, latency, bytes)
	hs.randomizerStrategy.UpdateMetrics(peerID, success, latency, bytes)
	hs.privacyStrategy.UpdateMetrics(peerID, success, latency, bytes)
}

// GetPeerInfo returns comprehensive peer information from all strategies
func (hs *HybridStrategy) GetPeerInfo(peerID peer.ID) (*PeerInfo, bool) {
	// Combine information from all strategies
	combinedInfo := &PeerInfo{
		ID: peerID,
	}
	
	found := false
	
	// Get performance info
	if perfInfo, exists := hs.performanceStrategy.GetPeerInfo(peerID); exists {
		combinedInfo.Latency = perfInfo.Latency
		combinedInfo.Bandwidth = perfInfo.Bandwidth
		combinedInfo.SuccessRate = perfInfo.SuccessRate
		combinedInfo.TotalRequests = perfInfo.TotalRequests
		combinedInfo.SuccessRequests = perfInfo.SuccessRequests
		combinedInfo.TotalBytes = perfInfo.TotalBytes
		combinedInfo.TotalTime = perfInfo.TotalTime
		combinedInfo.LastSeen = perfInfo.LastSeen
		found = true
	}
	
	// Get randomizer info
	if randInfo, exists := hs.randomizerStrategy.GetPeerInfo(peerID); exists {
		combinedInfo.RandomizerScore = randInfo.RandomizerScore
		if randInfo.LastSeen.After(combinedInfo.LastSeen) {
			combinedInfo.LastSeen = randInfo.LastSeen
		}
		found = true
	}
	
	// Get privacy info
	if privInfo, exists := hs.privacyStrategy.GetPeerInfo(peerID); exists {
		combinedInfo.Reputation = privInfo.Reputation
		if privInfo.LastSeen.After(combinedInfo.LastSeen) {
			combinedInfo.LastSeen = privInfo.LastSeen
		}
		found = true
	}
	
	return combinedInfo, found
}

// GetStrategyStats returns statistics from all strategies
func (hs *HybridStrategy) GetStrategyStats() map[string]interface{} {
	hs.mutex.RLock()
	defer hs.mutex.RUnlock()
	
	stats := map[string]interface{}{
		"strategy_type": "hybrid",
		"weights": map[string]float64{
			"performance": hs.performanceWeight,
			"randomizer":  hs.randomizerWeight,
			"privacy":     hs.privacyWeight,
		},
		"adaptive_weights": hs.adaptiveWeights,
		"context_aware":    hs.contextAware,
		"performance_stats": hs.performanceStrategy.GetMetricsStats(),
		"randomizer_stats":  hs.randomizerStrategy.GetRandomizerStats(),
	}
	
	return stats
}

// SetStrategyWeights allows manual adjustment of strategy weights
func (hs *HybridStrategy) SetStrategyWeights(performance, randomizer, privacy float64) {
	hs.mutex.Lock()
	defer hs.mutex.Unlock()
	
	// Normalize weights
	total := performance + randomizer + privacy
	if total > 0 {
		hs.performanceWeight = performance / total
		hs.randomizerWeight = randomizer / total
		hs.privacyWeight = privacy / total
	}
}

// SetAdaptiveWeights enables or disables adaptive weight adjustment
func (hs *HybridStrategy) SetAdaptiveWeights(enabled bool) {
	hs.mutex.Lock()
	defer hs.mutex.Unlock()
	hs.adaptiveWeights = enabled
}

// SelectOptimalStrategy selects the best single strategy for given criteria
func (hs *HybridStrategy) SelectOptimalStrategy(criteria SelectionCriteria) string {
	weights := hs.calculateAdaptiveWeights(criteria)
	
	maxWeight := 0.0
	optimalStrategy := "performance"
	
	for strategy, weight := range weights {
		if weight > maxWeight {
			maxWeight = weight
			optimalStrategy = strategy
		}
	}
	
	return optimalStrategy
}

// SelectPeersWithStrategy selects peers using a specific strategy
func (hs *HybridStrategy) SelectPeersWithStrategy(ctx context.Context, strategy string, criteria SelectionCriteria) ([]peer.ID, error) {
	switch strategy {
	case "performance":
		return hs.performanceStrategy.SelectPeers(ctx, criteria)
	case "randomizer":
		return hs.randomizerStrategy.SelectPeers(ctx, criteria)
	case "privacy":
		return hs.privacyStrategy.SelectPeers(ctx, criteria)
	case "hybrid":
		return hs.SelectPeers(ctx, criteria)
	default:
		return nil, fmt.Errorf("unknown strategy: %s", strategy)
	}
}

// CompareStrategies compares the effectiveness of different strategies
func (hs *HybridStrategy) CompareStrategies(ctx context.Context, criteria SelectionCriteria) (map[string][]peer.ID, error) {
	results := make(map[string][]peer.ID)
	
	strategies := []string{"performance", "randomizer", "privacy", "hybrid"}
	
	for _, strategy := range strategies {
		peers, err := hs.SelectPeersWithStrategy(ctx, strategy, criteria)
		if err != nil {
			results[strategy] = nil
		} else {
			results[strategy] = peers
		}
	}
	
	return results, nil
}

// AnalyzeStrategies provides analysis of strategy performance
func (hs *HybridStrategy) AnalyzeStrategies() map[string]interface{} {
	analysis := map[string]interface{}{
		"total_peers_tracked": map[string]int{
			"performance": len(hs.performanceStrategy.metrics),
			"randomizer":  len(hs.randomizerStrategy.randomizerMetrics),
			"privacy":     len(hs.privacyStrategy.privacyMetrics),
		},
		"strategy_effectiveness": hs.calculateStrategyEffectiveness(),
		"weight_recommendations": hs.recommendWeights(),
	}
	
	return analysis
}

// calculateStrategyEffectiveness calculates how effective each strategy is
func (hs *HybridStrategy) calculateStrategyEffectiveness() map[string]float64 {
	// This is a simplified effectiveness calculation
	// In practice, this would analyze historical performance
	
	effectiveness := map[string]float64{
		"performance": 0.8, // Generally reliable
		"randomizer":  0.7, // Good for specific use cases
		"privacy":     0.6, // Important but may sacrifice performance
	}
	
	// Adjust based on current metrics
	perfStats := hs.performanceStrategy.GetMetricsStats()
	if successRate, ok := perfStats["overall_success_rate"].(float64); ok {
		effectiveness["performance"] = successRate
	}
	
	return effectiveness
}

// recommendWeights recommends optimal weights based on current network state
func (hs *HybridStrategy) recommendWeights() map[string]float64 {
	effectiveness := hs.calculateStrategyEffectiveness()
	
	// Calculate recommended weights based on effectiveness
	total := 0.0
	for _, eff := range effectiveness {
		total += eff
	}
	
	recommendations := make(map[string]float64)
	for strategy, eff := range effectiveness {
		recommendations[strategy] = eff / total
	}
	
	return recommendations
}

// isPeerExcluded checks if a peer is in the exclusion list
func (hs *HybridStrategy) isPeerExcluded(peerID peer.ID, excludeList []peer.ID) bool {
	for _, excluded := range excludeList {
		if peerID == excluded {
			return true
		}
	}
	return false
}
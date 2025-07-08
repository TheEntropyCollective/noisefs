package p2p

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// RandomizerStrategy selects peers based on their randomizer block availability
type RandomizerStrategy struct {
	peerManager          *PeerManager
	blockTracker         *BlockAvailabilityTracker
	randomizerMetrics    map[peer.ID]*RandomizerMetrics
	popularBlocks        map[string]*BlockPopularity
	mutex                sync.RWMutex
	
	// Configuration
	minPopularityScore   float64
	maxBlockAge          time.Duration
	diversityWeight      float64
	popularityWeight     float64
	availabilityWeight   float64
}

// RandomizerMetrics tracks randomizer-specific metrics for a peer
type RandomizerMetrics struct {
	PeerID                peer.ID              `json:"peer_id"`
	LastUpdate            time.Time            `json:"last_update"`
	
	// Block inventory
	TotalBlocks           int                  `json:"total_blocks"`
	PopularBlocks         int                  `json:"popular_blocks"`
	UniqueBlocks          int                  `json:"unique_blocks"`
	BlockDiversity        float64              `json:"block_diversity"`
	
	// Randomizer usage
	RandomizerRequests    int64                `json:"randomizer_requests"`
	RandomizerHits        int64                `json:"randomizer_hits"`
	RandomizerHitRate     float64              `json:"randomizer_hit_rate"`
	
	// Block categories
	HotBlocks             map[string]time.Time `json:"hot_blocks"`       // Recently accessed
	WarmBlocks            map[string]time.Time `json:"warm_blocks"`      // Moderately accessed
	ColdBlocks            map[string]time.Time `json:"cold_blocks"`      // Rarely accessed
	
	// Scoring
	RandomizerScore       float64              `json:"randomizer_score"`
	
	mutex                 sync.RWMutex
}

// BlockPopularity tracks how popular a block is across the network
type BlockPopularity struct {
	BlockCID            string        `json:"block_cid"`
	PeerCount           int           `json:"peer_count"`
	RequestCount        int64         `json:"request_count"`
	LastRequested       time.Time     `json:"last_requested"`
	PopularityScore     float64       `json:"popularity_score"`
	
	// Usage as randomizer
	RandomizerUsage     int64         `json:"randomizer_usage"`
	EfficiencyScore     float64       `json:"efficiency_score"`
	
	mutex               sync.RWMutex
}

// NewRandomizerStrategy creates a new randomizer-aware peer selection strategy
func NewRandomizerStrategy(pm *PeerManager) *RandomizerStrategy {
	return &RandomizerStrategy{
		peerManager:        pm,
		blockTracker:       NewBlockAvailabilityTracker(),
		randomizerMetrics:  make(map[peer.ID]*RandomizerMetrics),
		popularBlocks:      make(map[string]*BlockPopularity),
		minPopularityScore: 0.1,
		maxBlockAge:        24 * time.Hour,
		diversityWeight:    0.3,
		popularityWeight:   0.4,
		availabilityWeight: 0.3,
	}
}

// SelectPeers selects peers based on randomizer block availability
func (rs *RandomizerStrategy) SelectPeers(ctx context.Context, criteria SelectionCriteria) ([]peer.ID, error) {
	rs.mutex.RLock()
	defer rs.mutex.RUnlock()
	
	// Get healthy peers
	healthyPeers := rs.peerManager.GetHealthyPeers()
	if len(healthyPeers) == 0 {
		return nil, fmt.Errorf("no healthy peers available")
	}
	
	// If specific blocks are required, filter by availability
	if len(criteria.RequiredBlocks) > 0 {
		return rs.selectPeersForBlocks(healthyPeers, criteria)
	}
	
	// Otherwise, select based on general randomizer quality
	return rs.selectBestRandomizers(healthyPeers, criteria)
}

// selectPeersForBlocks selects peers that have specific required blocks
func (rs *RandomizerStrategy) selectPeersForBlocks(peers []peer.ID, criteria SelectionCriteria) ([]peer.ID, error) {
	type peerMatch struct {
		PeerID     peer.ID
		MatchCount int
		Score      float64
	}
	
	var candidates []peerMatch
	
	for _, peerID := range peers {
		// Skip excluded peers
		if rs.isPeerExcluded(peerID, criteria.ExcludePeers) {
			continue
		}
		
		metrics := rs.getOrCreateMetrics(peerID)
		metrics.mutex.RLock()
		
		// Count matching blocks
		matchCount := 0
		for _, blockCID := range criteria.RequiredBlocks {
			if rs.peerHasBlock(peerID, blockCID) {
				matchCount++
			}
		}
		
		if matchCount > 0 {
			score := rs.calculateRandomizerScore(metrics, float64(matchCount)/float64(len(criteria.RequiredBlocks)))
			candidates = append(candidates, peerMatch{
				PeerID:     peerID,
				MatchCount: matchCount,
				Score:      score,
			})
		}
		
		metrics.mutex.RUnlock()
	}
	
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no peers have required blocks")
	}
	
	// Sort by match count first, then by score
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].MatchCount == candidates[j].MatchCount {
			return candidates[i].Score > candidates[j].Score
		}
		return candidates[i].MatchCount > candidates[j].MatchCount
	})
	
	// Select top candidates
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

// selectBestRandomizers selects peers with the best randomizer characteristics
func (rs *RandomizerStrategy) selectBestRandomizers(peers []peer.ID, criteria SelectionCriteria) ([]peer.ID, error) {
	type peerCandidate struct {
		PeerID peer.ID
		Score  float64
	}
	
	var candidates []peerCandidate
	
	for _, peerID := range peers {
		// Skip excluded peers
		if rs.isPeerExcluded(peerID, criteria.ExcludePeers) {
			continue
		}
		
		metrics := rs.getOrCreateMetrics(peerID)
		metrics.mutex.RLock()
		
		score := rs.calculateRandomizerScore(metrics, 1.0)
		
		// Apply additional criteria
		if criteria.PreferRandomizers && metrics.PopularBlocks > 0 {
			score *= 1.2 // 20% bonus for peers with popular blocks
		}
		
		metrics.mutex.RUnlock()
		
		candidates = append(candidates, peerCandidate{
			PeerID: peerID,
			Score:  score,
		})
	}
	
	// Sort by score (descending)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})
	
	// Select top candidates
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

// isPeerExcluded checks if a peer is in the exclusion list
func (rs *RandomizerStrategy) isPeerExcluded(peerID peer.ID, excludeList []peer.ID) bool {
	for _, excluded := range excludeList {
		if peerID == excluded {
			return true
		}
	}
	return false
}

// peerHasBlock checks if a peer likely has a specific block
func (rs *RandomizerStrategy) peerHasBlock(peerID peer.ID, blockCID string) bool {
	// Check with block availability tracker
	peersWithBlock := rs.blockTracker.GetPeersWithBlock(blockCID)
	for _, pid := range peersWithBlock {
		if pid == peerID.String() {
			return true
		}
	}
	return false
}

// calculateRandomizerScore calculates a composite randomizer score for a peer
func (rs *RandomizerStrategy) calculateRandomizerScore(metrics *RandomizerMetrics, blockMatch float64) float64 {
	if metrics.TotalBlocks < 10 {
		return 0.1 // Low score for peers with few blocks
	}
	
	// Diversity score (variety of blocks)
	diversityScore := metrics.BlockDiversity
	
	// Popularity score (has popular blocks)
	popularityScore := 0.0
	if metrics.TotalBlocks > 0 {
		popularityScore = float64(metrics.PopularBlocks) / float64(metrics.TotalBlocks)
	}
	
	// Availability score (randomizer hit rate)
	availabilityScore := metrics.RandomizerHitRate
	
	// Composite score
	score := diversityScore*rs.diversityWeight +
		popularityScore*rs.popularityWeight +
		availabilityScore*rs.availabilityWeight
	
	// Apply block match bonus
	score *= blockMatch
	
	return score
}

// UpdateMetrics updates randomizer metrics for a peer
func (rs *RandomizerStrategy) UpdateMetrics(peerID peer.ID, success bool, latency time.Duration, bytes int64) {
	rs.mutex.Lock()
	defer rs.mutex.Unlock()
	
	metrics := rs.getOrCreateMetrics(peerID)
	metrics.mutex.Lock()
	defer metrics.mutex.Unlock()
	
	now := time.Now()
	metrics.LastUpdate = now
	metrics.RandomizerRequests++
	
	if success {
		metrics.RandomizerHits++
	}
	
	// Update hit rate
	if metrics.RandomizerRequests > 0 {
		metrics.RandomizerHitRate = float64(metrics.RandomizerHits) / float64(metrics.RandomizerRequests)
	}
	
	// Update randomizer score
	metrics.RandomizerScore = rs.calculateRandomizerScore(metrics, 1.0)
}

// UpdatePeerInventory updates the block inventory for a peer
func (rs *RandomizerStrategy) UpdatePeerInventory(peerID peer.ID, blocks []string) {
	rs.mutex.Lock()
	defer rs.mutex.Unlock()
	
	// Update block tracker
	rs.blockTracker.UpdatePeerInventory(peerID.String(), blocks)
	
	// Update peer metrics
	metrics := rs.getOrCreateMetrics(peerID)
	metrics.mutex.Lock()
	defer metrics.mutex.Unlock()
	
	now := time.Now()
	metrics.LastUpdate = now
	metrics.TotalBlocks = len(blocks)
	
	// Initialize block categories if needed
	if metrics.HotBlocks == nil {
		metrics.HotBlocks = make(map[string]time.Time)
		metrics.WarmBlocks = make(map[string]time.Time)
		metrics.ColdBlocks = make(map[string]time.Time)
	}
	
	// Categorize blocks and update popularity
	popularCount := 0
	uniqueCount := 0
	
	for _, blockCID := range blocks {
		// Update block popularity
		popularity := rs.getOrCreateBlockPopularity(blockCID)
		popularity.mutex.Lock()
		
		if !rs.blockKnown(blockCID) {
			uniqueCount++
		}
		
		popularity.PeerCount++
		popularity.LastRequested = now
		popularity.PopularityScore = rs.calculateBlockPopularity(popularity)
		
		// Check if block is popular
		if popularity.PopularityScore > rs.minPopularityScore {
			popularCount++
			metrics.HotBlocks[blockCID] = now
		} else {
			metrics.ColdBlocks[blockCID] = now
		}
		
		popularity.mutex.Unlock()
	}
	
	metrics.PopularBlocks = popularCount
	metrics.UniqueBlocks = uniqueCount
	
	// Calculate block diversity (Shannon entropy)
	metrics.BlockDiversity = rs.calculateBlockDiversity(blocks)
	
	// Update randomizer score
	metrics.RandomizerScore = rs.calculateRandomizerScore(metrics, 1.0)
}

// calculateBlockPopularity calculates popularity score for a block
func (rs *RandomizerStrategy) calculateBlockPopularity(popularity *BlockPopularity) float64 {
	// Base score on peer count and request frequency
	peerScore := math.Log(float64(popularity.PeerCount + 1))
	requestScore := math.Log(float64(popularity.RequestCount + 1))
	
	// Recency bonus
	recencyBonus := 1.0
	timeSinceRequest := time.Since(popularity.LastRequested)
	if timeSinceRequest < time.Hour {
		recencyBonus = 1.5
	} else if timeSinceRequest > 24*time.Hour {
		recencyBonus = 0.5
	}
	
	return (peerScore + requestScore) * recencyBonus
}

// calculateBlockDiversity calculates Shannon entropy for block diversity
func (rs *RandomizerStrategy) calculateBlockDiversity(blocks []string) float64 {
	if len(blocks) <= 1 {
		return 0.0
	}
	
	// For simplicity, use a basic diversity measure
	// In practice, this could be more sophisticated
	uniqueBlocks := make(map[string]bool)
	for _, block := range blocks {
		uniqueBlocks[block] = true
	}
	
	diversity := float64(len(uniqueBlocks)) / float64(len(blocks))
	return diversity
}

// blockKnown checks if a block is already known in the network
func (rs *RandomizerStrategy) blockKnown(blockCID string) bool {
	_, exists := rs.popularBlocks[blockCID]
	return exists
}

// GetPeerInfo returns randomizer information for a specific peer
func (rs *RandomizerStrategy) GetPeerInfo(peerID peer.ID) (*PeerInfo, bool) {
	rs.mutex.RLock()
	defer rs.mutex.RUnlock()
	
	if metrics, exists := rs.randomizerMetrics[peerID]; exists {
		metrics.mutex.RLock()
		defer metrics.mutex.RUnlock()
		
		peerInfo := &PeerInfo{
			ID:              peerID,
			LastSeen:        metrics.LastUpdate,
			RandomizerScore: metrics.RandomizerScore,
			TotalRequests:   metrics.RandomizerRequests,
			SuccessRequests: metrics.RandomizerHits,
		}
		
		return peerInfo, true
	}
	
	return nil, false
}

// getOrCreateMetrics gets existing metrics or creates new ones for a peer
func (rs *RandomizerStrategy) getOrCreateMetrics(peerID peer.ID) *RandomizerMetrics {
	if metrics, exists := rs.randomizerMetrics[peerID]; exists {
		return metrics
	}
	
	metrics := &RandomizerMetrics{
		PeerID:     peerID,
		LastUpdate: time.Now(),
		HotBlocks:  make(map[string]time.Time),
		WarmBlocks: make(map[string]time.Time),
		ColdBlocks: make(map[string]time.Time),
	}
	rs.randomizerMetrics[peerID] = metrics
	
	return metrics
}

// getOrCreateBlockPopularity gets existing popularity or creates new for a block
func (rs *RandomizerStrategy) getOrCreateBlockPopularity(blockCID string) *BlockPopularity {
	if popularity, exists := rs.popularBlocks[blockCID]; exists {
		return popularity
	}
	
	popularity := &BlockPopularity{
		BlockCID:      blockCID,
		LastRequested: time.Now(),
	}
	rs.popularBlocks[blockCID] = popularity
	
	return popularity
}

// GetPopularBlocks returns the most popular blocks for use as randomizers
func (rs *RandomizerStrategy) GetPopularBlocks(count int) []string {
	rs.mutex.RLock()
	defer rs.mutex.RUnlock()
	
	type blockScore struct {
		BlockCID string
		Score    float64
	}
	
	var scores []blockScore
	for blockCID, popularity := range rs.popularBlocks {
		popularity.mutex.RLock()
		scores = append(scores, blockScore{
			BlockCID: blockCID,
			Score:    popularity.PopularityScore,
		})
		popularity.mutex.RUnlock()
	}
	
	// Sort by popularity score (descending)
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})
	
	// Return top blocks
	result := make([]string, 0, count)
	for i, score := range scores {
		if i >= count {
			break
		}
		result = append(result, score.BlockCID)
	}
	
	return result
}

// GetPeersWithPopularBlocks returns peers that have popular randomizer blocks
func (rs *RandomizerStrategy) GetPeersWithPopularBlocks(minPopularity float64) []peer.ID {
	rs.mutex.RLock()
	defer rs.mutex.RUnlock()
	
	var result []peer.ID
	for peerID, metrics := range rs.randomizerMetrics {
		metrics.mutex.RLock()
		
		hasPopularBlocks := false
		for blockCID := range metrics.HotBlocks {
			if popularity, exists := rs.popularBlocks[blockCID]; exists {
				popularity.mutex.RLock()
				if popularity.PopularityScore >= minPopularity {
					hasPopularBlocks = true
				}
				popularity.mutex.RUnlock()
				
				if hasPopularBlocks {
					break
				}
			}
		}
		
		if hasPopularBlocks {
			result = append(result, peerID)
		}
		
		metrics.mutex.RUnlock()
	}
	
	return result
}

// GetRandomizerStats returns statistics about randomizer usage
func (rs *RandomizerStrategy) GetRandomizerStats() map[string]interface{} {
	rs.mutex.RLock()
	defer rs.mutex.RUnlock()
	
	totalPeers := len(rs.randomizerMetrics)
	totalBlocks := len(rs.popularBlocks)
	totalRequests := int64(0)
	totalHits := int64(0)
	avgDiversity := 0.0
	
	validPeers := 0
	for _, metrics := range rs.randomizerMetrics {
		metrics.mutex.RLock()
		if metrics.TotalBlocks > 0 {
			validPeers++
			totalRequests += metrics.RandomizerRequests
			totalHits += metrics.RandomizerHits
			avgDiversity += metrics.BlockDiversity
		}
		metrics.mutex.RUnlock()
	}
	
	stats := map[string]interface{}{
		"total_peers":         totalPeers,
		"valid_peers":         validPeers,
		"total_blocks":        totalBlocks,
		"total_requests":      totalRequests,
		"total_hits":          totalHits,
		"overall_hit_rate":    0.0,
		"avg_block_diversity": 0.0,
	}
	
	if totalRequests > 0 {
		stats["overall_hit_rate"] = float64(totalHits) / float64(totalRequests)
	}
	
	if validPeers > 0 {
		stats["avg_block_diversity"] = avgDiversity / float64(validPeers)
	}
	
	return stats
}

// CleanupOldBlocks removes old block popularity data
func (rs *RandomizerStrategy) CleanupOldBlocks() {
	rs.mutex.Lock()
	defer rs.mutex.Unlock()
	
	cutoff := time.Now().Add(-rs.maxBlockAge)
	
	for blockCID, popularity := range rs.popularBlocks {
		popularity.mutex.RLock()
		lastRequested := popularity.LastRequested
		popularity.mutex.RUnlock()
		
		if lastRequested.Before(cutoff) {
			delete(rs.popularBlocks, blockCID)
		}
	}
}
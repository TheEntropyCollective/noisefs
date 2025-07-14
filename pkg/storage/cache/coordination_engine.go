package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"sort"
	"sync"

	"github.com/bits-and-blooms/bloom/v3"
)

// CoordinationEngine helps peers coordinate their caching decisions
type CoordinationEngine struct {
	config            *BloomExchangeConfig
	blockAssignments  map[string][]string // block -> assigned peers
	peerAssignments   map[string][]string // peer -> assigned blocks
	coordinationScore float64
	mu                sync.RWMutex
}

// NewCoordinationEngine creates a new coordination engine
func NewCoordinationEngine(config *BloomExchangeConfig) *CoordinationEngine {
	return &CoordinationEngine{
		config:           config,
		blockAssignments: make(map[string][]string),
		peerAssignments:  make(map[string][]string),
	}
}

// GenerateHints generates coordination hints based on peer filters
func (ce *CoordinationEngine) GenerateHints(
	localFilters map[string]*bloom.BloomFilter,
	peerFilters map[string]*PeerFilterSet,
) *CoordinationHints {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	
	hints := &CoordinationHints{
		HighDemandBlocks:  make([]string, 0),
		SuggestedBlocks:   make([]string, 0),
		CoordinationScore: 0.0,
	}
	
	if len(peerFilters) < ce.config.MinPeersForCoordination {
		return hints
	}
	
	// Analyze valuable blocks across peers
	valuableBlocks := ce.analyzeValuableBlocks(peerFilters)
	
	// Find high-demand blocks (blocks wanted by multiple peers)
	highDemand := ce.findHighDemandBlocks(valuableBlocks, len(peerFilters))
	hints.HighDemandBlocks = highDemand
	
	// Generate suggestions based on coordination algorithm
	suggestions := ce.generateBlockSuggestions(localFilters, peerFilters, valuableBlocks)
	hints.SuggestedBlocks = suggestions
	
	// Calculate coordination score
	hints.CoordinationScore = ce.calculateCoordinationScore(peerFilters)
	ce.coordinationScore = hints.CoordinationScore
	
	return hints
}

// analyzeValuableBlocks identifies valuable blocks across peers
func (ce *CoordinationEngine) analyzeValuableBlocks(
	peerFilters map[string]*PeerFilterSet,
) map[string][]string {
	blockPeers := make(map[string][]string)
	
	// Collect all blocks that peers consider valuable
	for peerID, filterSet := range peerFilters {
		if _, exists := filterSet.Filters["valuable_blocks"]; exists {
			// Since we can't iterate Bloom filter contents,
			// we use coordination hints from peers
			if filterSet.Hints != nil && filterSet.Hints.HighDemandBlocks != nil {
				for _, blockID := range filterSet.Hints.HighDemandBlocks {
					blockPeers[blockID] = append(blockPeers[blockID], peerID)
				}
			}
		}
	}
	
	return blockPeers
}

// findHighDemandBlocks identifies blocks wanted by multiple peers
func (ce *CoordinationEngine) findHighDemandBlocks(
	blockPeers map[string][]string,
	totalPeers int,
) []string {
	type blockDemand struct {
		blockID string
		demand  int
	}
	
	demands := make([]blockDemand, 0, len(blockPeers))
	
	// Calculate demand for each block
	for blockID, peers := range blockPeers {
		demandRatio := float64(len(peers)) / float64(totalPeers)
		if demandRatio >= 0.3 { // At least 30% of peers want it
			demands = append(demands, blockDemand{
				blockID: blockID,
				demand:  len(peers),
			})
		}
	}
	
	// Sort by demand (descending)
	sort.Slice(demands, func(i, j int) bool {
		return demands[i].demand > demands[j].demand
	})
	
	// Return top high-demand blocks
	result := make([]string, 0, 10)
	for i, bd := range demands {
		if i >= 10 { // Limit to top 10
			break
		}
		result = append(result, bd.blockID)
	}
	
	return result
}

// generateBlockSuggestions suggests blocks for a peer to cache
func (ce *CoordinationEngine) generateBlockSuggestions(
	localFilters map[string]*bloom.BloomFilter,
	peerFilters map[string]*PeerFilterSet,
	valuableBlocks map[string][]string,
) []string {
	suggestions := make([]string, 0)
	
	// Use consistent hashing to assign blocks to peers
	myPeerID := ce.generateLocalPeerID()
	
	// Score each valuable block for this peer
	type blockScore struct {
		blockID string
		score   float64
	}
	
	scores := make([]blockScore, 0)
	
	for blockID, interestedPeers := range valuableBlocks {
		// Skip if too many peers already have it
		if len(interestedPeers) > len(peerFilters)/2 {
			continue
		}
		
		// Calculate affinity score using consistent hashing
		affinityScore := ce.calculateBlockAffinity(blockID, myPeerID)
		
		// Adjust score based on current coverage
		coverageScore := 1.0 - (float64(len(interestedPeers)) / float64(len(peerFilters)))
		
		// Combined score
		totalScore := affinityScore * 0.6 + coverageScore * 0.4
		
		scores = append(scores, blockScore{
			blockID: blockID,
			score:   totalScore,
		})
	}
	
	// Sort by score (descending)
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})
	
	// Return top suggestions
	for i, bs := range scores {
		if i >= 20 { // Limit suggestions
			break
		}
		suggestions = append(suggestions, bs.blockID)
	}
	
	return suggestions
}

// calculateBlockAffinity calculates affinity between a block and peer using consistent hashing
func (ce *CoordinationEngine) calculateBlockAffinity(blockID, peerID string) float64 {
	// Hash both IDs
	blockHash := sha256.Sum256([]byte(blockID))
	peerHash := sha256.Sum256([]byte(peerID))
	
	// Calculate distance in hash space
	distance := 0
	for i := 0; i < 32; i++ {
		diff := int(blockHash[i]) - int(peerHash[i])
		distance += diff * diff
	}
	
	// Normalize to 0-1 (lower distance = higher affinity)
	maxDistance := 255 * 255 * 32
	affinity := 1.0 - (float64(distance) / float64(maxDistance))
	
	return affinity
}

// calculateCoordinationScore calculates how well peers are coordinating
func (ce *CoordinationEngine) calculateCoordinationScore(
	peerFilters map[string]*PeerFilterSet,
) float64 {
	if len(peerFilters) < ce.config.MinPeersForCoordination {
		return 0.0
	}
	
	// Calculate overlap ratios between peers
	totalOverlap := 0.0
	comparisons := 0
	
	peerList := make([]string, 0, len(peerFilters))
	for peerID := range peerFilters {
		peerList = append(peerList, peerID)
	}
	
	// Compare each pair of peers
	for i := 0; i < len(peerList); i++ {
		for j := i + 1; j < len(peerList); j++ {
			peer1 := peerFilters[peerList[i]]
			peer2 := peerFilters[peerList[j]]
			
			// Calculate overlap for valuable blocks
			if f1, ok1 := peer1.Filters["valuable_blocks"]; ok1 {
				if f2, ok2 := peer2.Filters["valuable_blocks"]; ok2 {
					overlap := ce.estimateFilterOverlap(f1, f2)
					totalOverlap += overlap
					comparisons++
				}
			}
		}
	}
	
	if comparisons == 0 {
		return 0.0
	}
	
	// Average overlap ratio
	avgOverlap := totalOverlap / float64(comparisons)
	
	// Good coordination means moderate overlap (not too high, not too low)
	// Optimal overlap is around 30-50%
	optimalOverlap := 0.4
	deviation := math.Abs(avgOverlap - optimalOverlap)
	
	// Score peaks at optimal overlap
	score := 1.0 - (deviation / optimalOverlap)
	if score < 0 {
		score = 0
	}
	
	return score
}

// estimateFilterOverlap estimates overlap between two Bloom filters
func (ce *CoordinationEngine) estimateFilterOverlap(f1, f2 *bloom.BloomFilter) float64 {
	// Get fill ratios
	fill1 := float64(f1.ApproximatedSize()) / float64(f1.Cap())
	fill2 := float64(f2.ApproximatedSize()) / float64(f2.Cap())
	
	// Estimate overlap using probability theory
	// P(bit set in both) ≈ P(bit set in f1) * P(bit set in f2)
	// This is an approximation assuming independence
	expectedOverlap := fill1 * fill2
	
	// Adjust for filter sizes
	size1 := float64(f1.Cap())
	size2 := float64(f2.Cap())
	sizeRatio := math.Min(size1, size2) / math.Max(size1, size2)
	
	// Final overlap estimate
	overlap := expectedOverlap * sizeRatio
	
	return overlap
}

// generateLocalPeerID generates a consistent peer ID for this node
func (ce *CoordinationEngine) generateLocalPeerID() string {
	// In practice, this would use a persistent node ID
	// For now, we'll use a hash of some stable identifier
	hash := sha256.Sum256([]byte("local-peer"))
	return hex.EncodeToString(hash[:8])
}

// GetCoordinationMetrics returns current coordination metrics
func (ce *CoordinationEngine) GetCoordinationMetrics() *CoordinationMetrics {
	ce.mu.RLock()
	defer ce.mu.RUnlock()
	
	metrics := &CoordinationMetrics{
		CoordinationScore:     ce.coordinationScore,
		AssignedBlocks:        len(ce.peerAssignments[ce.generateLocalPeerID()]),
		TotalTrackedBlocks:    len(ce.blockAssignments),
		AverageBlockCoverage:  0.0,
	}
	
	// Calculate average coverage
	if len(ce.blockAssignments) > 0 {
		totalCoverage := 0
		for _, peers := range ce.blockAssignments {
			totalCoverage += len(peers)
		}
		metrics.AverageBlockCoverage = float64(totalCoverage) / float64(len(ce.blockAssignments))
	}
	
	return metrics
}

// CoordinationMetrics represents coordination metrics
type CoordinationMetrics struct {
	CoordinationScore    float64
	AssignedBlocks       int
	TotalTrackedBlocks   int
	AverageBlockCoverage float64
}

// UpdateAssignments updates block assignments based on peer coordination
func (ce *CoordinationEngine) UpdateAssignments(
	peerID string,
	assignedBlocks []string,
) {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	
	// Clear old assignments for this peer
	if oldBlocks, exists := ce.peerAssignments[peerID]; exists {
		for _, blockID := range oldBlocks {
			ce.removeAssignment(blockID, peerID)
		}
	}
	
	// Add new assignments
	ce.peerAssignments[peerID] = assignedBlocks
	for _, blockID := range assignedBlocks {
		ce.blockAssignments[blockID] = append(ce.blockAssignments[blockID], peerID)
	}
}

// removeAssignment removes a peer from a block's assignment list
func (ce *CoordinationEngine) removeAssignment(blockID, peerID string) {
	peers := ce.blockAssignments[blockID]
	newPeers := make([]string, 0, len(peers))
	
	for _, p := range peers {
		if p != peerID {
			newPeers = append(newPeers, p)
		}
	}
	
	if len(newPeers) > 0 {
		ce.blockAssignments[blockID] = newPeers
	} else {
		delete(ce.blockAssignments, blockID)
	}
}

// GetBlockAssignments returns current block assignments
func (ce *CoordinationEngine) GetBlockAssignments(blockID string) []string {
	ce.mu.RLock()
	defer ce.mu.RUnlock()
	
	assignments := ce.blockAssignments[blockID]
	result := make([]string, len(assignments))
	copy(result, assignments)
	return result
}

// GetPeerAssignments returns blocks assigned to a peer
func (ce *CoordinationEngine) GetPeerAssignments(peerID string) []string {
	ce.mu.RLock()
	defer ce.mu.RUnlock()
	
	assignments := ce.peerAssignments[peerID]
	result := make([]string, len(assignments))
	copy(result, assignments)
	return result
}
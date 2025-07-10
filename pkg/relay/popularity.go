package relay

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/noisefs/noisefs/pkg/cache"
)

// PopularBlockTracker tracks and identifies popular blocks for cover traffic
type PopularBlockTracker struct {
	cache           cache.Cache
	config          *PopularityConfig
	popularBlocks   map[string]*PopularityInfo
	globalStats     *GlobalPopularityStats
	refreshTicker   *time.Ticker
	mu              sync.RWMutex
	ctx             context.Context
	cancel          context.CancelFunc
}

// PopularityConfig contains configuration for popularity tracking
type PopularityConfig struct {
	RefreshInterval     time.Duration // How often to refresh popularity data
	MinAccessCount      int64         // Minimum accesses to be considered popular
	PopularityThreshold float64       // Threshold for popularity score (0-1)
	MaxPopularBlocks    int           // Maximum number of popular blocks to track
	DecayFactor         float64       // Decay factor for time-based popularity
	CategoryWeights     map[string]float64 // Weights for different block categories
}

// PopularityInfo contains detailed information about a block's popularity
type PopularityInfo struct {
	BlockID         string
	AccessCount     int64
	LastAccessed    time.Time
	PopularityScore float64
	TrendScore      float64     // Trending up/down indicator
	Category        BlockCategory
	RandomizerUsage int64       // Times used as randomizer
	PeerReports     map[string]int64 // Popularity reports from peers
	FirstSeen       time.Time
	Updated         time.Time
}

// BlockCategory represents different types of blocks
type BlockCategory string

const (
	CategoryUnknown     BlockCategory = "unknown"
	CategoryPublicDomain BlockCategory = "public_domain"
	CategoryMedia       BlockCategory = "media"
	CategoryDocument    BlockCategory = "document"
	CategoryArchive     BlockCategory = "archive"
	CategoryCode        BlockCategory = "code"
)

// GlobalPopularityStats tracks overall popularity statistics
type GlobalPopularityStats struct {
	TotalBlocks      int64
	PopularBlocks    int64
	AverageScore     float64
	TopCategories    map[BlockCategory]int64
	TrendingUp       []string
	TrendingDown     []string
	LastUpdate       time.Time
}

// PopularBlockSet represents a set of popular blocks for cover traffic
type PopularBlockSet struct {
	Blocks      []*PopularityInfo
	GeneratedAt time.Time
	Category    BlockCategory
	TotalScore  float64
	Diversity   float64 // Measure of diversity in the set
}

// NewPopularBlockTracker creates a new popular block tracker
func NewPopularBlockTracker(cache cache.Cache, config *PopularityConfig) *PopularBlockTracker {
	ctx, cancel := context.WithCancel(context.Background())
	
	tracker := &PopularBlockTracker{
		cache:         cache,
		config:        config,
		popularBlocks: make(map[string]*PopularityInfo),
		globalStats:   &GlobalPopularityStats{TopCategories: make(map[BlockCategory]int64)},
		ctx:           ctx,
		cancel:        cancel,
	}
	
	// Start refresh routine
	tracker.refreshTicker = time.NewTicker(config.RefreshInterval)
	go tracker.refreshLoop()
	
	return tracker
}

// GetPopularBlocks returns the most popular blocks for cover traffic
func (pbt *PopularBlockTracker) GetPopularBlocks(count int, category BlockCategory) (*PopularBlockSet, error) {
	pbt.mu.RLock()
	defer pbt.mu.RUnlock()
	
	// Filter blocks by category if specified
	var candidates []*PopularityInfo
	for _, info := range pbt.popularBlocks {
		if category == CategoryUnknown || info.Category == category {
			// Only include blocks that meet minimum criteria
			if info.AccessCount >= pbt.config.MinAccessCount &&
			   info.PopularityScore >= pbt.config.PopularityThreshold {
				candidates = append(candidates, info)
			}
		}
	}
	
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no popular blocks found for category %s", category)
	}
	
	// Sort by popularity score
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].PopularityScore > candidates[j].PopularityScore
	})
	
	// Limit to requested count
	if count > len(candidates) {
		count = len(candidates)
	}
	
	selected := candidates[:count]
	
	// Calculate diversity score
	diversity := pbt.calculateDiversity(selected)
	
	// Calculate total score
	totalScore := 0.0
	for _, info := range selected {
		totalScore += info.PopularityScore
	}
	
	return &PopularBlockSet{
		Blocks:      selected,
		GeneratedAt: time.Now(),
		Category:    category,
		TotalScore:  totalScore,
		Diversity:   diversity,
	}, nil
}

// GetTrendingBlocks returns blocks that are trending up in popularity
func (pbt *PopularBlockTracker) GetTrendingBlocks(count int) ([]*PopularityInfo, error) {
	pbt.mu.RLock()
	defer pbt.mu.RUnlock()
	
	var trending []*PopularityInfo
	for _, info := range pbt.popularBlocks {
		if info.TrendScore > 0.1 { // Positive trend threshold
			trending = append(trending, info)
		}
	}
	
	// Sort by trend score
	sort.Slice(trending, func(i, j int) bool {
		return trending[i].TrendScore > trending[j].TrendScore
	})
	
	if count > len(trending) {
		count = len(trending)
	}
	
	return trending[:count], nil
}

// UpdateBlockPopularity updates popularity information for a block
func (pbt *PopularBlockTracker) UpdateBlockPopularity(blockID string, accessCount int64, category BlockCategory) {
	pbt.mu.Lock()
	defer pbt.mu.Unlock()
	
	now := time.Now()
	
	info, exists := pbt.popularBlocks[blockID]
	if !exists {
		info = &PopularityInfo{
			BlockID:      blockID,
			Category:     category,
			PeerReports:  make(map[string]int64),
			FirstSeen:    now,
		}
		pbt.popularBlocks[blockID] = info
	}
	
	// Calculate trend score based on access count change
	oldCount := info.AccessCount
	info.AccessCount = accessCount
	info.LastAccessed = now
	info.Updated = now
	
	// Calculate popularity score
	info.PopularityScore = pbt.calculatePopularityScore(info)
	
	// Calculate trend score
	if oldCount > 0 {
		info.TrendScore = float64(accessCount-oldCount) / float64(oldCount)
	}
}

// ReportPeerPopularity updates popularity based on peer reports
func (pbt *PopularBlockTracker) ReportPeerPopularity(peerID string, blockID string, accessCount int64) {
	pbt.mu.Lock()
	defer pbt.mu.Unlock()
	
	info, exists := pbt.popularBlocks[blockID]
	if !exists {
		info = &PopularityInfo{
			BlockID:     blockID,
			Category:    CategoryUnknown,
			PeerReports: make(map[string]int64),
			FirstSeen:   time.Now(),
		}
		pbt.popularBlocks[blockID] = info
	}
	
	info.PeerReports[peerID] = accessCount
	info.Updated = time.Now()
	
	// Recalculate popularity score including peer reports
	info.PopularityScore = pbt.calculatePopularityScore(info)
}

// GetRandomizedBlocks returns popular blocks suitable for use as randomizers
func (pbt *PopularBlockTracker) GetRandomizedBlocks(count int) ([]*PopularityInfo, error) {
	pbt.mu.RLock()
	defer pbt.mu.RUnlock()
	
	// Get blocks that are suitable for randomizer use
	var candidates []*PopularityInfo
	for _, info := range pbt.popularBlocks {
		// Prefer public domain blocks for randomizers
		if info.Category == CategoryPublicDomain || info.RandomizerUsage > 0 {
			candidates = append(candidates, info)
		}
	}
	
	if len(candidates) == 0 {
		// Fall back to general popular blocks
		for _, info := range pbt.popularBlocks {
			if info.PopularityScore >= pbt.config.PopularityThreshold {
				candidates = append(candidates, info)
			}
		}
	}
	
	// Sort by combination of popularity and randomizer usage
	sort.Slice(candidates, func(i, j int) bool {
		scoreI := candidates[i].PopularityScore + float64(candidates[i].RandomizerUsage)*0.1
		scoreJ := candidates[j].PopularityScore + float64(candidates[j].RandomizerUsage)*0.1
		return scoreI > scoreJ
	})
	
	if count > len(candidates) {
		count = len(candidates)
	}
	
	return candidates[:count], nil
}

// refreshLoop periodically refreshes popularity data from the cache
func (pbt *PopularBlockTracker) refreshLoop() {
	defer pbt.refreshTicker.Stop()
	
	for {
		select {
		case <-pbt.ctx.Done():
			return
		case <-pbt.refreshTicker.C:
			pbt.refreshFromCache()
		}
	}
}

// refreshFromCache updates popularity data from the cache system
func (pbt *PopularBlockTracker) refreshFromCache() {
	// Get cache statistics
	stats := pbt.cache.GetStats()
	
	pbt.mu.Lock()
	defer pbt.mu.Unlock()
	
	// Update global statistics
	pbt.globalStats.TotalBlocks = int64(len(pbt.popularBlocks))
	pbt.globalStats.PopularBlocks = 0
	pbt.globalStats.AverageScore = 0
	pbt.globalStats.LastUpdate = time.Now()
	
	totalScore := 0.0
	categoryCount := make(map[BlockCategory]int64)
	
	// Get popular blocks from cache
	if randomizers := pbt.cache.GetRandomizers(pbt.config.MaxPopularBlocks); randomizers != nil {
		for i, cid := range randomizers {
			// Get or create popularity info
			info, exists := pbt.popularBlocks[cid]
			if !exists {
				info = &PopularityInfo{
					BlockID:     cid,
					Category:    pbt.inferCategory(cid),
					PeerReports: make(map[string]int64),
					FirstSeen:   time.Now(),
				}
				pbt.popularBlocks[cid] = info
			}
			
			// Update from cache stats if available
			if stats != nil {
				if count, exists := stats.PopularBlocks[cid]; exists {
					info.AccessCount = count
				}
			}
			
			// Update popularity score
			info.PopularityScore = pbt.calculatePopularityScore(info)
			
			// Give higher scores to more popular randomizers
			info.PopularityScore += float64(len(randomizers)-i) / float64(len(randomizers)) * 0.2
			
			if info.PopularityScore >= pbt.config.PopularityThreshold {
				pbt.globalStats.PopularBlocks++
			}
			
			totalScore += info.PopularityScore
			categoryCount[info.Category]++
		}
	}
	
	// Calculate average score
	if pbt.globalStats.TotalBlocks > 0 {
		pbt.globalStats.AverageScore = totalScore / float64(pbt.globalStats.TotalBlocks)
	}
	
	// Update category statistics
	pbt.globalStats.TopCategories = categoryCount
}

// calculatePopularityScore calculates a comprehensive popularity score
func (pbt *PopularBlockTracker) calculatePopularityScore(info *PopularityInfo) float64 {
	score := 0.0
	
	// Base score from access count
	if info.AccessCount > 0 {
		score += float64(info.AccessCount) / 1000.0 // Normalize
	}
	
	// Recency bonus (decay over time)
	if !info.LastAccessed.IsZero() {
		recency := time.Since(info.LastAccessed)
		recencyScore := 1.0 / (1.0 + recency.Hours()/24.0*pbt.config.DecayFactor)
		score += recencyScore * 0.3
	}
	
	// Category weight
	if weight, exists := pbt.config.CategoryWeights[string(info.Category)]; exists {
		score *= weight
	}
	
	// Randomizer usage bonus
	if info.RandomizerUsage > 0 {
		score += float64(info.RandomizerUsage) / 100.0 * 0.2
	}
	
	// Peer consensus bonus
	if len(info.PeerReports) > 1 {
		peerAvg := 0.0
		for _, count := range info.PeerReports {
			peerAvg += float64(count)
		}
		peerAvg /= float64(len(info.PeerReports))
		score += peerAvg / 1000.0 * 0.1
	}
	
	// Normalize to 0-1 range
	if score > 1.0 {
		score = 1.0
	}
	
	return score
}

// calculateDiversity calculates diversity score for a block set
func (pbt *PopularBlockTracker) calculateDiversity(blocks []*PopularityInfo) float64 {
	if len(blocks) <= 1 {
		return 0.0
	}
	
	// Count categories
	categories := make(map[BlockCategory]int)
	for _, block := range blocks {
		categories[block.Category]++
	}
	
	// Calculate Shannon diversity index
	diversity := 0.0
	total := float64(len(blocks))
	
	for _, count := range categories {
		p := float64(count) / total
		if p > 0 {
			diversity -= p * (p / 2.0) // Simplified calculation
		}
	}
	
	return diversity
}

// inferCategory attempts to infer block category from block ID
func (pbt *PopularBlockTracker) inferCategory(blockID string) BlockCategory {
	// In a real implementation, this would analyze the block content
	// or metadata to determine the category
	// For now, we'll return unknown
	return CategoryUnknown
}

// GetStats returns current popularity statistics
func (pbt *PopularBlockTracker) GetStats() *GlobalPopularityStats {
	pbt.mu.RLock()
	defer pbt.mu.RUnlock()
	
	return pbt.globalStats
}

// Stop stops the popularity tracker
func (pbt *PopularBlockTracker) Stop() {
	pbt.cancel()
}
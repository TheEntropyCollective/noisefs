package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/TheEntropyCollective/noisefs/pkg/privacy/p2p"
	"github.com/libp2p/go-libp2p/core/peer"
)

// MetricsConfig holds configuration for bounded metrics collection
type MetricsConfig struct {
	MaxOperations   int           `json:"max_operations"`
	MaxCacheMetrics int           `json:"max_cache_metrics"`
	MaxPeerMetrics  int           `json:"max_peer_metrics"`
	RetentionPeriod time.Duration `json:"retention_period"`
}

// DefaultMetricsConfig returns default configuration for metrics collection
func DefaultMetricsConfig() *MetricsConfig {
	return &MetricsConfig{
		MaxOperations:   10000,          // 10k operations
		MaxCacheMetrics: 1000,           // 1k cache snapshots
		MaxPeerMetrics:  5000,           // 5k peer measurements
		RetentionPeriod: 24 * time.Hour, // 24 hours
	}
}

// PerformanceAnalyzer provides comprehensive performance analysis for NoiseFS
type PerformanceAnalyzer struct {
	mu           sync.RWMutex
	clients      []*noisefs.Client
	peerManagers []*p2p.PeerManager
	startTime    time.Time
	config       *MetricsConfig

	// Bounded metrics with circular buffer logic
	operations    []OperationMetric
	cacheMetrics  []CacheMetric
	peerMetrics   []PeerMetric
	systemMetrics SystemMetric

	// Circular buffer indices
	operationIdx int
	cacheIdx     int
	peerIdx      int

	// Buffer full flags
	operationFull bool
	cacheFull     bool
	peerFull      bool
}

// OperationMetric tracks individual operation performance
type OperationMetric struct {
	Type      string        `json:"type"` // "store", "retrieve", "randomizer_select"
	Duration  time.Duration `json:"duration"`
	BlockSize int           `json:"block_size"`
	Success   bool          `json:"success"`
	Strategy  string        `json:"strategy"`
	CacheHit  bool          `json:"cache_hit"`
	Timestamp time.Time     `json:"timestamp"`
}

// CacheMetric tracks cache performance over time
type CacheMetric struct {
	Timestamp   time.Time `json:"timestamp"`
	HitRate     float64   `json:"hit_rate"`
	TotalBlocks int       `json:"total_blocks"`
	HotTier     int       `json:"hot_tier"`
	WarmTier    int       `json:"warm_tier"`
	ColdTier    int       `json:"cold_tier"`
	Evictions   int64     `json:"evictions"`
}

// PeerMetric tracks peer performance and selection effectiveness
type PeerMetric struct {
	PeerID         peer.ID       `json:"peer_id"`
	Timestamp      time.Time     `json:"timestamp"`
	Latency        time.Duration `json:"latency"`
	Bandwidth      float64       `json:"bandwidth"`
	SuccessRate    float64       `json:"success_rate"`
	SelectionCount int           `json:"selection_count"`
	Strategy       string        `json:"strategy"`
}

// SystemMetric tracks overall system performance
type SystemMetric struct {
	TotalOperations      int64         `json:"total_operations"`
	SuccessfulOps        int64         `json:"successful_ops"`
	TotalBlocksStored    int64         `json:"total_blocks_stored"`
	TotalBlocksRetrieved int64         `json:"total_blocks_retrieved"`
	AverageLatency       time.Duration `json:"average_latency"`
	ThroughputMBps       float64       `json:"throughput_mbps"`
	StorageOverhead      float64       `json:"storage_overhead"`
	CacheEfficiency      float64       `json:"cache_efficiency"`
	PeerEfficiency       float64       `json:"peer_efficiency"`
	StartTime            time.Time     `json:"start_time"`
	EndTime              time.Time     `json:"end_time"`
}

// AnalysisResult contains the complete performance analysis
type AnalysisResult struct {
	SystemMetrics    SystemMetric      `json:"system_metrics"`
	OperationMetrics []OperationMetric `json:"operation_metrics"`
	CacheAnalysis    CacheAnalysis     `json:"cache_analysis"`
	PeerAnalysis     PeerAnalysis      `json:"peer_analysis"`
	Recommendations  []string          `json:"recommendations"`
}

// CacheAnalysis provides detailed cache performance analysis
type CacheAnalysis struct {
	OverallHitRate     float64            `json:"overall_hit_rate"`
	HitRateByBlockSize map[int]float64    `json:"hit_rate_by_block_size"`
	TierDistribution   map[string]float64 `json:"tier_distribution"`
	EvictionEfficiency float64            `json:"eviction_efficiency"`
	PredictionAccuracy float64            `json:"prediction_accuracy"`
	TierTransitions    map[string]int     `json:"tier_transitions"`
	PopularBlocks      []string           `json:"popular_blocks"`
}

// PeerAnalysis provides detailed peer selection analysis
type PeerAnalysis struct {
	PeerPerformance       map[peer.ID]PeerPerformanceStats `json:"peer_performance"`
	StrategyEffectiveness map[string]float64               `json:"strategy_effectiveness"`
	LoadDistribution      map[peer.ID]int                  `json:"load_distribution"`
	NetworkEfficiency     float64                          `json:"network_efficiency"`
	OptimalPeerSet        []peer.ID                        `json:"optimal_peer_set"`
}

// PeerPerformanceStats contains detailed stats for individual peers
type PeerPerformanceStats struct {
	AverageLatency   time.Duration `json:"average_latency"`
	TotalRequests    int64         `json:"total_requests"`
	SuccessRate      float64       `json:"success_rate"`
	Bandwidth        float64       `json:"bandwidth"`
	ReliabilityScore float64       `json:"reliability_score"`
	LastSeen         time.Time     `json:"last_seen"`
}

// NewPerformanceAnalyzer creates a new performance analyzer with default configuration
func NewPerformanceAnalyzer() *PerformanceAnalyzer {
	return NewPerformanceAnalyzerWithConfig(DefaultMetricsConfig())
}

// NewPerformanceAnalyzerWithConfig creates a new performance analyzer with custom configuration
func NewPerformanceAnalyzerWithConfig(config *MetricsConfig) *PerformanceAnalyzer {
	pa := &PerformanceAnalyzer{
		startTime:    time.Now(),
		config:       config,
		operations:   make([]OperationMetric, 0, config.MaxOperations),
		cacheMetrics: make([]CacheMetric, 0, config.MaxCacheMetrics),
		peerMetrics:  make([]PeerMetric, 0, config.MaxPeerMetrics),
	}
	return pa
}

// AddClient adds a NoiseFS client to the analyzer
func (pa *PerformanceAnalyzer) AddClient(client *noisefs.Client) {
	pa.mu.Lock()
	defer pa.mu.Unlock()
	pa.clients = append(pa.clients, client)
}

// AddPeerManager adds a peer manager to the analyzer
func (pa *PerformanceAnalyzer) AddPeerManager(pm *p2p.PeerManager) {
	pa.mu.Lock()
	defer pa.mu.Unlock()
	pa.peerManagers = append(pa.peerManagers, pm)
}

// RecordOperation records an operation metric with bounded collection
func (pa *PerformanceAnalyzer) RecordOperation(opType string, duration time.Duration, blockSize int, success bool, strategy string, cacheHit bool) {
	metric := OperationMetric{
		Type:      opType,
		Duration:  duration,
		BlockSize: blockSize,
		Success:   success,
		Strategy:  strategy,
		CacheHit:  cacheHit,
		Timestamp: time.Now(),
	}
	pa.mu.Lock()
	defer pa.mu.Unlock()
	pa.addOperationMetric(metric)
}

// addOperationMetric implements circular buffer logic for operations
func (pa *PerformanceAnalyzer) addOperationMetric(metric OperationMetric) {
	if len(pa.operations) < pa.config.MaxOperations {
		pa.operations = append(pa.operations, metric)
	} else {
		pa.operations[pa.operationIdx] = metric
		pa.operationIdx = (pa.operationIdx + 1) % pa.config.MaxOperations
		pa.operationFull = true
	}
}

// addCacheMetric implements circular buffer logic for cache metrics
func (pa *PerformanceAnalyzer) addCacheMetric(metric CacheMetric) {
	if len(pa.cacheMetrics) < pa.config.MaxCacheMetrics {
		pa.cacheMetrics = append(pa.cacheMetrics, metric)
	} else {
		pa.cacheMetrics[pa.cacheIdx] = metric
		pa.cacheIdx = (pa.cacheIdx + 1) % pa.config.MaxCacheMetrics
		pa.cacheFull = true
	}
}

// addPeerMetric implements circular buffer logic for peer metrics
func (pa *PerformanceAnalyzer) addPeerMetric(metric PeerMetric) {
	if len(pa.peerMetrics) < pa.config.MaxPeerMetrics {
		pa.peerMetrics = append(pa.peerMetrics, metric)
	} else {
		pa.peerMetrics[pa.peerIdx] = metric
		pa.peerIdx = (pa.peerIdx + 1) % pa.config.MaxPeerMetrics
		pa.peerFull = true
	}
}

// CollectMetrics collects current metrics from all clients and peer managers with bounded collection
func (pa *PerformanceAnalyzer) CollectMetrics() {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	timestamp := time.Now()

	// Collect cache metrics
	for _, client := range pa.clients {
		if stats := client.GetAdaptiveCacheStats(); stats != nil {
			metric := CacheMetric{
				Timestamp:   timestamp,
				HitRate:     stats.HitRate,
				TotalBlocks: int(stats.TotalRequests),
				HotTier:     int(stats.HotTierHits),
				WarmTier:    int(stats.WarmTierHits),
				ColdTier:    int(stats.ColdTierHits),
				Evictions:   stats.Evictions,
			}
			pa.addCacheMetric(metric)
		}

		// Collect peer metrics (TODO: implement when storage manager provides peer stats)
		if peerStats := client.GetPeerStats(); peerStats != nil {
			for peerID, statsInterface := range peerStats {
				// Type assert to a stats struct (when implemented)
				if statsMap, ok := statsInterface.(map[string]interface{}); ok {
					// Extract metrics with safe type assertions
					latency := time.Duration(0)
					bandwidth := float64(0)
					successfulReqs := int64(0)
					totalReqs := int64(1) // Avoid division by zero

					if l, ok := statsMap["average_latency"]; ok {
						if dur, ok := l.(time.Duration); ok {
							latency = dur
						}
					}
					if b, ok := statsMap["bandwidth"]; ok {
						if bw, ok := b.(float64); ok {
							bandwidth = bw
						}
					}
					if s, ok := statsMap["successful_requests"]; ok {
						if sr, ok := s.(int64); ok {
							successfulReqs = sr
						}
					}
					if t, ok := statsMap["total_requests"]; ok {
						if tr, ok := t.(int64); ok && tr > 0 {
							totalReqs = tr
						}
					}

					metric := PeerMetric{
						PeerID:         peerID,
						Timestamp:      timestamp,
						Latency:        latency,
						Bandwidth:      bandwidth,
						SuccessRate:    float64(successfulReqs) / float64(totalReqs),
						SelectionCount: int(totalReqs),
						Strategy:       "mixed", // Would need to track per strategy
					}
					pa.addPeerMetric(metric)
				}
			}
		}
	}
}

// StartContinuousCollection starts continuous metric collection
func (pa *PerformanceAnalyzer) StartContinuousCollection(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pa.CollectMetrics()
		case <-ctx.Done():
			return
		}
	}
}

// Analyze performs comprehensive performance analysis
func (pa *PerformanceAnalyzer) Analyze() *AnalysisResult {
	pa.mu.RLock()
	defer pa.mu.RUnlock()

	pa.systemMetrics.EndTime = time.Now()

	// Calculate system metrics
	pa.calculateSystemMetrics()

	// Analyze cache performance
	cacheAnalysis := pa.analyzeCachePerformance()

	// Analyze peer performance
	peerAnalysis := pa.analyzePeerPerformance()

	// Generate recommendations
	recommendations := pa.generateRecommendations(cacheAnalysis, peerAnalysis)

	// Create copy of operations for safe external access
	operationsCopy := make([]OperationMetric, len(pa.operations))
	copy(operationsCopy, pa.operations)

	return &AnalysisResult{
		SystemMetrics:    pa.systemMetrics,
		OperationMetrics: operationsCopy,
		CacheAnalysis:    cacheAnalysis,
		PeerAnalysis:     peerAnalysis,
		Recommendations:  recommendations,
	}
}

// UpdateConfig updates the metrics configuration
func (pa *PerformanceAnalyzer) UpdateConfig(config *MetricsConfig) {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	pa.config = config

	// Resize slices if needed
	if len(pa.operations) > config.MaxOperations {
		pa.operations = pa.operations[:config.MaxOperations]
		pa.operationIdx = 0
		pa.operationFull = true
	}
	if len(pa.cacheMetrics) > config.MaxCacheMetrics {
		pa.cacheMetrics = pa.cacheMetrics[:config.MaxCacheMetrics]
		pa.cacheIdx = 0
		pa.cacheFull = true
	}
	if len(pa.peerMetrics) > config.MaxPeerMetrics {
		pa.peerMetrics = pa.peerMetrics[:config.MaxPeerMetrics]
		pa.peerIdx = 0
		pa.peerFull = true
	}
}

// GetConfig returns the current metrics configuration
func (pa *PerformanceAnalyzer) GetConfig() *MetricsConfig {
	pa.mu.RLock()
	defer pa.mu.RUnlock()

	// Return a copy to prevent external modification
	return &MetricsConfig{
		MaxOperations:   pa.config.MaxOperations,
		MaxCacheMetrics: pa.config.MaxCacheMetrics,
		MaxPeerMetrics:  pa.config.MaxPeerMetrics,
		RetentionPeriod: pa.config.RetentionPeriod,
	}
}

// CleanupOldMetrics removes metrics older than the retention period
func (pa *PerformanceAnalyzer) CleanupOldMetrics() {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	cutoff := time.Now().Add(-pa.config.RetentionPeriod)

	// Clean operations
	validOps := pa.operations[:0]
	for _, op := range pa.operations {
		if op.Timestamp.After(cutoff) {
			validOps = append(validOps, op)
		}
	}
	pa.operations = validOps
	pa.operationIdx = len(validOps) % pa.config.MaxOperations
	pa.operationFull = len(validOps) >= pa.config.MaxOperations

	// Clean cache metrics
	validCache := pa.cacheMetrics[:0]
	for _, cache := range pa.cacheMetrics {
		if cache.Timestamp.After(cutoff) {
			validCache = append(validCache, cache)
		}
	}
	pa.cacheMetrics = validCache
	pa.cacheIdx = len(validCache) % pa.config.MaxCacheMetrics
	pa.cacheFull = len(validCache) >= pa.config.MaxCacheMetrics

	// Clean peer metrics
	validPeer := pa.peerMetrics[:0]
	for _, peer := range pa.peerMetrics {
		if peer.Timestamp.After(cutoff) {
			validPeer = append(validPeer, peer)
		}
	}
	pa.peerMetrics = validPeer
	pa.peerIdx = len(validPeer) % pa.config.MaxPeerMetrics
	pa.peerFull = len(validPeer) >= pa.config.MaxPeerMetrics
}

// GetMetricsUsage returns information about current metrics buffer usage
func (pa *PerformanceAnalyzer) GetMetricsUsage() map[string]interface{} {
	pa.mu.RLock()
	defer pa.mu.RUnlock()

	return map[string]interface{}{
		"operations": map[string]interface{}{
			"count": len(pa.operations),
			"max":   pa.config.MaxOperations,
			"full":  pa.operationFull,
			"usage": float64(len(pa.operations)) / float64(pa.config.MaxOperations),
		},
		"cache_metrics": map[string]interface{}{
			"count": len(pa.cacheMetrics),
			"max":   pa.config.MaxCacheMetrics,
			"full":  pa.cacheFull,
			"usage": float64(len(pa.cacheMetrics)) / float64(pa.config.MaxCacheMetrics),
		},
		"peer_metrics": map[string]interface{}{
			"count": len(pa.peerMetrics),
			"max":   pa.config.MaxPeerMetrics,
			"full":  pa.peerFull,
			"usage": float64(len(pa.peerMetrics)) / float64(pa.config.MaxPeerMetrics),
		},
	}
}

// calculateSystemMetrics calculates overall system performance metrics
// Note: This method expects to be called from within a locked context
func (pa *PerformanceAnalyzer) calculateSystemMetrics() {
	pa.systemMetrics.StartTime = pa.startTime
	pa.systemMetrics.TotalOperations = int64(len(pa.operations))

	var totalLatency time.Duration
	var totalBytes int64
	var successOps int64
	var storeOps, retrieveOps int64

	for _, op := range pa.operations {
		totalLatency += op.Duration
		totalBytes += int64(op.BlockSize)

		if op.Success {
			successOps++
		}

		switch op.Type {
		case "store":
			storeOps++
		case "retrieve":
			retrieveOps++
		}
	}

	pa.systemMetrics.SuccessfulOps = successOps
	pa.systemMetrics.TotalBlocksStored = storeOps
	pa.systemMetrics.TotalBlocksRetrieved = retrieveOps

	if len(pa.operations) > 0 {
		pa.systemMetrics.AverageLatency = totalLatency / time.Duration(len(pa.operations))

		duration := pa.systemMetrics.EndTime.Sub(pa.systemMetrics.StartTime)
		if duration > 0 {
			pa.systemMetrics.ThroughputMBps = float64(totalBytes) / (1024 * 1024) / duration.Seconds()
		}
	}

	// Calculate cache efficiency (hit rate)
	if len(pa.cacheMetrics) > 0 {
		totalHitRate := 0.0
		for _, metric := range pa.cacheMetrics {
			totalHitRate += metric.HitRate
		}
		pa.systemMetrics.CacheEfficiency = totalHitRate / float64(len(pa.cacheMetrics))
	}

	// Calculate peer efficiency (average success rate)
	if len(pa.peerMetrics) > 0 {
		totalSuccessRate := 0.0
		for _, metric := range pa.peerMetrics {
			totalSuccessRate += metric.SuccessRate
		}
		pa.systemMetrics.PeerEfficiency = totalSuccessRate / float64(len(pa.peerMetrics))
	}
}

// analyzeCachePerformance provides detailed cache analysis
func (pa *PerformanceAnalyzer) analyzeCachePerformance() CacheAnalysis {
	analysis := CacheAnalysis{
		HitRateByBlockSize: make(map[int]float64),
		TierDistribution:   make(map[string]float64),
		TierTransitions:    make(map[string]int),
		PopularBlocks:      make([]string, 0),
	}

	// Calculate overall hit rate
	cacheHits := 0
	totalOps := 0

	for _, op := range pa.operations {
		if op.Type == "retrieve" {
			totalOps++
			if op.CacheHit {
				cacheHits++
			}
		}
	}

	if totalOps > 0 {
		analysis.OverallHitRate = float64(cacheHits) / float64(totalOps)
	}

	// Analyze hit rate by block size
	blockSizeHits := make(map[int]int)
	blockSizeTotal := make(map[int]int)

	for _, op := range pa.operations {
		if op.Type == "retrieve" {
			blockSizeTotal[op.BlockSize]++
			if op.CacheHit {
				blockSizeHits[op.BlockSize]++
			}
		}
	}

	for size, total := range blockSizeTotal {
		if total > 0 {
			analysis.HitRateByBlockSize[size] = float64(blockSizeHits[size]) / float64(total)
		}
	}

	// Analyze tier distribution
	if len(pa.cacheMetrics) > 0 {
		lastMetric := pa.cacheMetrics[len(pa.cacheMetrics)-1]
		total := lastMetric.HotTier + lastMetric.WarmTier + lastMetric.ColdTier
		if total > 0 {
			analysis.TierDistribution["hot"] = float64(lastMetric.HotTier) / float64(total)
			analysis.TierDistribution["warm"] = float64(lastMetric.WarmTier) / float64(total)
			analysis.TierDistribution["cold"] = float64(lastMetric.ColdTier) / float64(total)
		}
	}

	// Calculate prediction accuracy (simplified)
	analysis.PredictionAccuracy = analysis.OverallHitRate * 100

	return analysis
}

// analyzePeerPerformance provides detailed peer analysis
func (pa *PerformanceAnalyzer) analyzePeerPerformance() PeerAnalysis {
	analysis := PeerAnalysis{
		PeerPerformance:       make(map[peer.ID]PeerPerformanceStats),
		StrategyEffectiveness: make(map[string]float64),
		LoadDistribution:      make(map[peer.ID]int),
		OptimalPeerSet:        make([]peer.ID, 0),
	}

	// Aggregate peer performance
	peerData := make(map[peer.ID][]PeerMetric)
	for _, metric := range pa.peerMetrics {
		peerData[metric.PeerID] = append(peerData[metric.PeerID], metric)
	}

	var totalNetworkLatency time.Duration
	var totalNetworkSuccess float64
	peerCount := 0

	for peerID, metrics := range peerData {
		if len(metrics) == 0 {
			continue
		}

		var avgLatency time.Duration
		var totalSuccess float64
		var totalBandwidth float64
		var totalRequests int64

		for _, metric := range metrics {
			avgLatency += metric.Latency
			totalSuccess += metric.SuccessRate
			totalBandwidth += metric.Bandwidth
			totalRequests += int64(metric.SelectionCount)
		}

		avgLatency /= time.Duration(len(metrics))
		avgSuccess := totalSuccess / float64(len(metrics))
		avgBandwidth := totalBandwidth / float64(len(metrics))

		analysis.PeerPerformance[peerID] = PeerPerformanceStats{
			AverageLatency:   avgLatency,
			TotalRequests:    totalRequests,
			SuccessRate:      avgSuccess,
			Bandwidth:        avgBandwidth,
			ReliabilityScore: avgSuccess * (1.0 / (1.0 + avgLatency.Seconds())),
			LastSeen:         metrics[len(metrics)-1].Timestamp,
		}

		analysis.LoadDistribution[peerID] = int(totalRequests)

		totalNetworkLatency += avgLatency
		totalNetworkSuccess += avgSuccess
		peerCount++

		// Add to optimal peer set if performance is good
		if avgSuccess > 0.8 && avgLatency < time.Millisecond*100 {
			analysis.OptimalPeerSet = append(analysis.OptimalPeerSet, peerID)
		}
	}

	// Calculate network efficiency
	if peerCount > 0 {
		avgNetworkLatency := totalNetworkLatency / time.Duration(peerCount)
		avgNetworkSuccess := totalNetworkSuccess / float64(peerCount)
		analysis.NetworkEfficiency = avgNetworkSuccess * (1.0 / (1.0 + avgNetworkLatency.Seconds()))
	}

	// Analyze strategy effectiveness
	strategyMetrics := make(map[string][]float64)
	for _, op := range pa.operations {
		if op.Success {
			strategyMetrics[op.Strategy] = append(strategyMetrics[op.Strategy], 1.0)
		} else {
			strategyMetrics[op.Strategy] = append(strategyMetrics[op.Strategy], 0.0)
		}
	}

	for strategy, results := range strategyMetrics {
		if len(results) > 0 {
			sum := 0.0
			for _, result := range results {
				sum += result
			}
			analysis.StrategyEffectiveness[strategy] = sum / float64(len(results))
		}
	}

	return analysis
}

// generateRecommendations generates optimization recommendations
func (pa *PerformanceAnalyzer) generateRecommendations(cache CacheAnalysis, peer PeerAnalysis) []string {
	recommendations := make([]string, 0)

	// Cache recommendations
	if cache.OverallHitRate < 0.7 {
		recommendations = append(recommendations, "Consider increasing cache size - hit rate is below 70%")
	}

	if cache.TierDistribution["hot"] > 0.5 {
		recommendations = append(recommendations, "Hot tier usage is high - consider adjusting tier ratios")
	}

	if cache.PredictionAccuracy < 60 {
		recommendations = append(recommendations, "ML prediction accuracy is low - more training data needed")
	}

	// Peer recommendations
	if peer.NetworkEfficiency < 0.8 {
		recommendations = append(recommendations, "Network efficiency is low - consider optimizing peer selection")
	}

	if len(peer.OptimalPeerSet) < 3 {
		recommendations = append(recommendations, "Limited optimal peers available - consider expanding peer network")
	}

	// Strategy recommendations
	bestStrategy := ""
	bestRate := 0.0
	for strategy, rate := range peer.StrategyEffectiveness {
		if rate > bestRate {
			bestRate = rate
			bestStrategy = strategy
		}
	}

	if bestStrategy != "" && bestRate > 0.9 {
		recommendations = append(recommendations,
			fmt.Sprintf("Consider using '%s' strategy more frequently (%.1f%% success rate)",
				bestStrategy, bestRate*100))
	}

	// Performance recommendations
	if pa.systemMetrics.ThroughputMBps < 10 {
		recommendations = append(recommendations, "Throughput is low - consider parallel operations or larger block sizes")
	}

	if pa.systemMetrics.StorageOverhead > 200 {
		recommendations = append(recommendations, "Storage overhead is high - optimize randomizer reuse")
	}

	return recommendations
}

// SaveReport saves the analysis report to a file
func (pa *PerformanceAnalyzer) SaveReport(filename string) error {
	result := pa.Analyze()

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}

	return os.WriteFile(filename, data, 0644)
}

// PrintSummary prints a summary of the performance analysis
func (pa *PerformanceAnalyzer) PrintSummary() {
	result := pa.Analyze()

	fmt.Println("=== NoiseFS Performance Analysis Summary ===")
	fmt.Printf("Duration: %v\n", result.SystemMetrics.EndTime.Sub(result.SystemMetrics.StartTime))
	fmt.Printf("Total Operations: %d\n", result.SystemMetrics.TotalOperations)
	fmt.Printf("Success Rate: %.1f%%\n", float64(result.SystemMetrics.SuccessfulOps)/float64(result.SystemMetrics.TotalOperations)*100)
	fmt.Printf("Average Latency: %v\n", result.SystemMetrics.AverageLatency)
	fmt.Printf("Throughput: %.2f MB/s\n", result.SystemMetrics.ThroughputMBps)
	fmt.Printf("Cache Hit Rate: %.1f%%\n", result.CacheAnalysis.OverallHitRate*100)
	fmt.Printf("Network Efficiency: %.1f%%\n", result.PeerAnalysis.NetworkEfficiency*100)

	fmt.Println("\n=== Recommendations ===")
	for i, rec := range result.Recommendations {
		fmt.Printf("%d. %s\n", i+1, rec)
	}

	fmt.Println("\n=== Top Performing Peers ===")
	// Sort peers by reliability score
	type peerScore struct {
		id    peer.ID
		score float64
	}

	var peerScores []peerScore
	for peerID, stats := range result.PeerAnalysis.PeerPerformance {
		peerScores = append(peerScores, peerScore{id: peerID, score: stats.ReliabilityScore})
	}

	sort.Slice(peerScores, func(i, j int) bool {
		return peerScores[i].score > peerScores[j].score
	})

	for i, ps := range peerScores {
		if i >= 5 { // Top 5 peers
			break
		}
		stats := result.PeerAnalysis.PeerPerformance[ps.id]
		fmt.Printf("%d. %s: %.1f%% success, %v latency\n",
			i+1, ps.id, stats.SuccessRate*100, stats.AverageLatency)
	}
}

// RunLiveAnalysis runs live performance analysis with periodic updates
func (pa *PerformanceAnalyzer) RunLiveAnalysis(ctx context.Context, updateInterval time.Duration) {
	ticker := time.NewTicker(updateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pa.CollectMetrics()
			pa.PrintSummary()
			fmt.Println("\n" + strings.Repeat("=", 50) + "\n")
		case <-ctx.Done():
			log.Println("Stopping live analysis")
			return
		}
	}
}

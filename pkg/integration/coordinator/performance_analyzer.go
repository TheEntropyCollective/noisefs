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

// MetricsConfig configures metrics collection bounds
type MetricsConfig struct {
	MaxOperations   int           `json:"max_operations"`   // Maximum operation metrics to retain
	MaxCacheMetrics int           `json:"max_cache_metrics"` // Maximum cache metrics to retain
	MaxPeerMetrics  int           `json:"max_peer_metrics"`  // Maximum peer metrics to retain
	RetentionPeriod time.Duration `json:"retention_period"` // How long to retain metrics
}

// DefaultMetricsConfig returns sensible defaults for metrics collection
func DefaultMetricsConfig() MetricsConfig {
	return MetricsConfig{
		MaxOperations:   10000, // ~10k operations (reasonable for most workloads)
		MaxCacheMetrics: 1000,  // ~1k cache snapshots (at 1 per minute = ~16 hours)
		MaxPeerMetrics:  5000,  // ~5k peer metrics (varies by peer count)
		RetentionPeriod: 24 * time.Hour, // 24 hours of data
	}
}

// PerformanceAnalyzer provides comprehensive performance analysis for NoiseFS
type PerformanceAnalyzer struct {
	mu           sync.RWMutex
	clients      []*noisefs.Client
	peerManagers []*p2p.PeerManager
	startTime    time.Time
	config       MetricsConfig

	// Bounded circular buffers for metrics
	operations    []OperationMetric
	cacheMetrics  []CacheMetric
	peerMetrics   []PeerMetric
	systemMetrics SystemMetric

	// Circular buffer tracking
	operationIdx   int
	cacheIdx       int
	peerIdx        int
	operationFull  bool
	cacheFull      bool
	peerFull       bool
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
func NewPerformanceAnalyzerWithConfig(config MetricsConfig) *PerformanceAnalyzer {
	return &PerformanceAnalyzer{
		startTime:    time.Now(),
		config:       config,
		operations:   make([]OperationMetric, 0, config.MaxOperations),
		cacheMetrics: make([]CacheMetric, 0, config.MaxCacheMetrics),
		peerMetrics:  make([]PeerMetric, 0, config.MaxPeerMetrics),
	}
}

// AddClient adds a NoiseFS client to the analyzer
func (pa *PerformanceAnalyzer) AddClient(client *noisefs.Client) {
	pa.clients = append(pa.clients, client)
}

// AddPeerManager adds a peer manager to the analyzer
func (pa *PerformanceAnalyzer) AddPeerManager(pm *p2p.PeerManager) {
	pa.peerManagers = append(pa.peerManagers, pm)
}

// RecordOperation records an operation metric using bounded circular buffer
func (pa *PerformanceAnalyzer) RecordOperation(opType string, duration time.Duration, blockSize int, success bool, strategy string, cacheHit bool) {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	metric := OperationMetric{
		Type:      opType,
		Duration:  duration,
		BlockSize: blockSize,
		Success:   success,
		Strategy:  strategy,
		CacheHit:  cacheHit,
		Timestamp: time.Now(),
	}

	pa.addOperationMetric(metric)
}

// addOperationMetric adds a metric to the circular buffer
func (pa *PerformanceAnalyzer) addOperationMetric(metric OperationMetric) {
	if len(pa.operations) < pa.config.MaxOperations {
		// Still growing the slice
		pa.operations = append(pa.operations, metric)
	} else {
		// Use circular buffer
		pa.operations[pa.operationIdx] = metric
		pa.operationIdx = (pa.operationIdx + 1) % pa.config.MaxOperations
		pa.operationFull = true
	}
}

// addCacheMetric adds a cache metric to the circular buffer
func (pa *PerformanceAnalyzer) addCacheMetric(metric CacheMetric) {
	if len(pa.cacheMetrics) < pa.config.MaxCacheMetrics {
		// Still growing the slice
		pa.cacheMetrics = append(pa.cacheMetrics, metric)
	} else {
		// Use circular buffer
		pa.cacheMetrics[pa.cacheIdx] = metric
		pa.cacheIdx = (pa.cacheIdx + 1) % pa.config.MaxCacheMetrics
		pa.cacheFull = true
	}
}

// addPeerMetric adds a peer metric to the circular buffer
func (pa *PerformanceAnalyzer) addPeerMetric(metric PeerMetric) {
	if len(pa.peerMetrics) < pa.config.MaxPeerMetrics {
		// Still growing the slice
		pa.peerMetrics = append(pa.peerMetrics, metric)
	} else {
		// Use circular buffer
		pa.peerMetrics[pa.peerIdx] = metric
		pa.peerIdx = (pa.peerIdx + 1) % pa.config.MaxPeerMetrics
		pa.peerFull = true
	}
}

// CollectMetrics collects current metrics from all clients and peer managers
func (pa *PerformanceAnalyzer) CollectMetrics() {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	timestamp := time.Now()

	// Clean up old metrics if retention period is configured
	pa.cleanupOldMetrics(timestamp)

	// Collect cache metrics
	for _, client := range pa.clients {
		if client == nil {
			continue // Skip nil clients
		}

		if stats := client.GetAdaptiveCacheStats(); stats != nil {
			// Add defensive programming for stats field access
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
				// Skip empty peer IDs
				if peerID == "" {
					continue
				}

				// Skip nil stats interface
				if statsInterface == nil {
					continue
				}

				// Type assert to a stats struct (when implemented)
				if statsMap, ok := statsInterface.(map[string]interface{}); ok && statsMap != nil {
					// Extract metrics with safe type assertions and validation
					latency := time.Duration(0)
					bandwidth := float64(0)
					successfulReqs := int64(0)
					totalReqs := int64(1) // Avoid division by zero

					// Safely extract latency with additional validation
					if l, exists := statsMap["average_latency"]; exists && l != nil {
						if dur, ok := l.(time.Duration); ok && dur >= 0 {
							latency = dur
						}
					}

					// Safely extract bandwidth with validation
					if b, exists := statsMap["bandwidth"]; exists && b != nil {
						if bw, ok := b.(float64); ok && bw >= 0 {
							bandwidth = bw
						}
					}

					// Safely extract successful requests with validation
					if s, exists := statsMap["successful_requests"]; exists && s != nil {
						if sr, ok := s.(int64); ok && sr >= 0 {
							successfulReqs = sr
						}
					}

					// Safely extract total requests with validation
					if t, exists := statsMap["total_requests"]; exists && t != nil {
						if tr, ok := t.(int64); ok && tr > 0 {
							totalReqs = tr
						}
					}

					// Validate that successfulReqs doesn't exceed totalReqs
					if successfulReqs > totalReqs {
						successfulReqs = totalReqs
					}

					// Create metric with validated data
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

// cleanupOldMetrics removes metrics older than the retention period
func (pa *PerformanceAnalyzer) cleanupOldMetrics(currentTime time.Time) {
	if pa.config.RetentionPeriod == 0 {
		return // No retention limit
	}

	cutoffTime := currentTime.Add(-pa.config.RetentionPeriod)

	// Clean up operation metrics
	pa.operations = pa.filterMetricsByTime(pa.operations, cutoffTime)

	// Clean up cache metrics
	pa.cacheMetrics = pa.filterCacheMetricsByTime(pa.cacheMetrics, cutoffTime)

	// Clean up peer metrics
	pa.peerMetrics = pa.filterPeerMetricsByTime(pa.peerMetrics, cutoffTime)
}

// filterMetricsByTime filters operation metrics by timestamp
func (pa *PerformanceAnalyzer) filterMetricsByTime(metrics []OperationMetric, cutoffTime time.Time) []OperationMetric {
	filtered := make([]OperationMetric, 0, len(metrics))
	for _, metric := range metrics {
		if metric.Timestamp.After(cutoffTime) {
			filtered = append(filtered, metric)
		}
	}
	return filtered
}

// filterCacheMetricsByTime filters cache metrics by timestamp
func (pa *PerformanceAnalyzer) filterCacheMetricsByTime(metrics []CacheMetric, cutoffTime time.Time) []CacheMetric {
	filtered := make([]CacheMetric, 0, len(metrics))
	for _, metric := range metrics {
		if metric.Timestamp.After(cutoffTime) {
			filtered = append(filtered, metric)
		}
	}
	return filtered
}

// filterPeerMetricsByTime filters peer metrics by timestamp
func (pa *PerformanceAnalyzer) filterPeerMetricsByTime(metrics []PeerMetric, cutoffTime time.Time) []PeerMetric {
	filtered := make([]PeerMetric, 0, len(metrics))
	for _, metric := range metrics {
		if metric.Timestamp.After(cutoffTime) {
			filtered = append(filtered, metric)
		}
	}
	return filtered
}

// GetCurrentMetrics returns current metrics counts for monitoring
func (pa *PerformanceAnalyzer) GetCurrentMetrics() (operationCount, cacheCount, peerCount int) {
	pa.mu.RLock()
	defer pa.mu.RUnlock()
	return len(pa.operations), len(pa.cacheMetrics), len(pa.peerMetrics)
}

// GetMetricsConfig returns the current metrics configuration
func (pa *PerformanceAnalyzer) GetMetricsConfig() MetricsConfig {
	pa.mu.RLock()
	defer pa.mu.RUnlock()
	return pa.config
}

// UpdateMetricsConfig updates the metrics configuration
// Note: This will not resize existing slices, only affects new metrics
func (pa *PerformanceAnalyzer) UpdateMetricsConfig(config MetricsConfig) {
	pa.mu.Lock()
	defer pa.mu.Unlock()
	pa.config = config
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

	// Create a copy of operations for the result to avoid data races
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

// calculateSystemMetrics calculates overall system performance metrics
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

	// Also print metrics buffer status
	operationCount, cacheCount, peerCount := pa.GetCurrentMetrics()
	config := pa.GetMetricsConfig()

	fmt.Println("=== NoiseFS Performance Analysis Summary ===")
	fmt.Printf("Duration: %v\n", result.SystemMetrics.EndTime.Sub(result.SystemMetrics.StartTime))
	fmt.Printf("Total Operations: %d\n", result.SystemMetrics.TotalOperations)
	fmt.Printf("Success Rate: %.1f%%\n", float64(result.SystemMetrics.SuccessfulOps)/float64(result.SystemMetrics.TotalOperations)*100)
	fmt.Printf("Average Latency: %v\n", result.SystemMetrics.AverageLatency)
	fmt.Printf("Throughput: %.2f MB/s\n", result.SystemMetrics.ThroughputMBps)
	fmt.Printf("Cache Hit Rate: %.1f%%\n", result.CacheAnalysis.OverallHitRate*100)
	fmt.Printf("Network Efficiency: %.1f%%\n", result.PeerAnalysis.NetworkEfficiency*100)
	fmt.Printf("Metrics Buffer Status: Operations %d/%d, Cache %d/%d, Peers %d/%d\n",
		operationCount, config.MaxOperations,
		cacheCount, config.MaxCacheMetrics,
		peerCount, config.MaxPeerMetrics)

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

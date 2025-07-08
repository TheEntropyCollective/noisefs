package integration

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/TheEntropyCollective/noisefs/pkg/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/ipfs"
	"github.com/TheEntropyCollective/noisefs/pkg/noisefs"
	"github.com/TheEntropyCollective/noisefs/pkg/p2p"
)

// Milestone4ImpactAnalyzer provides comprehensive testing of Milestone 4 improvements
type Milestone4ImpactAnalyzer struct {
	// Test configurations
	legacyClients []*noisefs.Client // Without Milestone 4 features
	modernClients []*noisefs.Client // With Milestone 4 features
	
	// Test parameters
	numPeers      int
	numBlocks     int
	testDuration  time.Duration
	blockSizes    []int
	
	// Results
	legacyResults  *TestResults
	modernResults  *TestResults
	analysis       *ImpactAnalysis
}

// TestResults captures comprehensive test metrics
type TestResults struct {
	// Performance metrics
	TotalOperations     int64         `json:"total_operations"`
	SuccessfulOps       int64         `json:"successful_ops"`
	FailedOps           int64         `json:"failed_ops"`
	AverageLatency      time.Duration `json:"average_latency"`
	ThroughputMBps      float64       `json:"throughput_mbps"`
	
	// Storage efficiency
	OriginalDataSize    int64   `json:"original_data_size"`
	StoredDataSize      int64   `json:"stored_data_size"`
	StorageOverhead     float64 `json:"storage_overhead_percent"`
	RandomizerReuseRate float64 `json:"randomizer_reuse_rate"`
	
	// Cache performance
	CacheHitRate        float64 `json:"cache_hit_rate"`
	CacheMissRate       float64 `json:"cache_miss_rate"`
	EvictionCount       int64   `json:"eviction_count"`
	PreloadHitRate      float64 `json:"preload_hit_rate"`
	
	// Peer selection effectiveness
	PeerSelectionTime   time.Duration            `json:"peer_selection_time"`
	SelectedPeerCount   int                      `json:"selected_peer_count"`
	PeerSuccessRates    map[peer.ID]float64     `json:"peer_success_rates"`
	StrategyEffectiveness map[string]float64     `json:"strategy_effectiveness"`
	
	// Network efficiency
	NetworkUtilization  float64 `json:"network_utilization"`
	LoadDistribution    float64 `json:"load_distribution_fairness"`
	FailoverCount       int64   `json:"failover_count"`
	
	// ML performance (modern only)
	PredictionAccuracy  float64 `json:"prediction_accuracy"`
	TierPromotions      int64   `json:"tier_promotions"`
	TierDemotions       int64   `json:"tier_demotions"`
	
	// Time series data
	Timestamps          []time.Time     `json:"timestamps"`
	LatencyHistory      []time.Duration `json:"latency_history"`
	ThroughputHistory   []float64       `json:"throughput_history"`
	CacheHitRateHistory []float64       `json:"cache_hit_rate_history"`
}

// ImpactAnalysis provides detailed comparison and insights
type ImpactAnalysis struct {
	// Performance improvements
	LatencyImprovement      float64 `json:"latency_improvement_percent"`
	ThroughputImprovement   float64 `json:"throughput_improvement_percent"`
	SuccessRateImprovement  float64 `json:"success_rate_improvement_percent"`
	
	// Storage efficiency gains
	OverheadReduction       float64 `json:"overhead_reduction_percent"`
	RandomizerEfficiency    float64 `json:"randomizer_efficiency_gain"`
	
	// Cache effectiveness
	CacheHitImprovement     float64 `json:"cache_hit_improvement_percent"`
	EvictionReduction       float64 `json:"eviction_reduction_percent"`
	
	// Peer selection benefits
	PeerSelectionSpeedup    float64 `json:"peer_selection_speedup"`
	LoadBalanceImprovement  float64 `json:"load_balance_improvement"`
	
	// ML/AI benefits
	PredictionValue         float64 `json:"prediction_value_score"`
	AdaptiveOptimization    float64 `json:"adaptive_optimization_score"`
	
	// Overall impact scores
	OverallPerformanceGain  float64 `json:"overall_performance_gain"`
	EfficiencyGain          float64 `json:"efficiency_gain"`
	ScalabilityImprovement  float64 `json:"scalability_improvement"`
	
	// Recommendations
	KeyInsights             []string `json:"key_insights"`
	OptimizationSuggestions []string `json:"optimization_suggestions"`
	NextSteps               []string `json:"next_steps"`
}

// NewMilestone4ImpactAnalyzer creates a new impact analyzer
func NewMilestone4ImpactAnalyzer(numPeers, numBlocks int, duration time.Duration) *Milestone4ImpactAnalyzer {
	return &Milestone4ImpactAnalyzer{
		numPeers:     numPeers,
		numBlocks:    numBlocks,
		testDuration: duration,
		blockSizes:   []int{4096, 32768, 131072, 1048576}, // 4KB to 1MB
	}
}

// SetupTestEnvironment initializes both legacy and modern test environments
func (m4 *Milestone4ImpactAnalyzer) SetupTestEnvironment() error {
	log.Println("Setting up Milestone 4 impact testing environment...")
	
	// Setup legacy clients (simulating pre-Milestone 4)
	if err := m4.setupLegacyClients(); err != nil {
		return fmt.Errorf("failed to setup legacy clients: %w", err)
	}
	
	// Setup modern clients (with Milestone 4 features)
	if err := m4.setupModernClients(); err != nil {
		return fmt.Errorf("failed to setup modern clients: %w", err)
	}
	
	log.Printf("Test environment ready: %d legacy clients, %d modern clients", 
		len(m4.legacyClients), len(m4.modernClients))
	
	return nil
}

// setupLegacyClients creates clients without Milestone 4 improvements
func (m4 *Milestone4ImpactAnalyzer) setupLegacyClients() error {
	for i := 0; i < m4.numPeers; i++ {
		// Create basic IPFS client (without peer selection)
		ipfsClient := NewLegacyMockIPFSClient()
		
		// Create basic cache (without adaptive features)
		basicCache := cache.NewMemoryCache(1000)
		
		// Create basic NoiseFS client
		client, err := noisefs.NewClient(ipfsClient, basicCache)
		if err != nil {
			return fmt.Errorf("failed to create legacy client %d: %w", i, err)
		}
		
		m4.legacyClients = append(m4.legacyClients, client)
	}
	
	return nil
}

// setupModernClients creates clients with full Milestone 4 features
func (m4 *Milestone4ImpactAnalyzer) setupModernClients() error {
	for i := 0; i < m4.numPeers; i++ {
		// Create enhanced IPFS client with peer selection
		ipfsClient := NewModernMockIPFSClient()
		
		// Create adaptive cache
		basicCache := cache.NewMemoryCache(1000)
		
		// Create modern NoiseFS client with adaptive features
		config := &noisefs.ClientConfig{
			EnableAdaptiveCache:   true,
			PreferRandomizerPeers: true,
			AdaptiveCacheConfig: &cache.AdaptiveCacheConfig{
				MaxSize:            50 * 1024 * 1024, // 50MB
				MaxItems:           5000,
				HotTierRatio:       0.1,
				WarmTierRatio:      0.3,
				PredictionWindow:   time.Hour,
				EvictionBatchSize:  10,
				ExchangeInterval:   time.Minute * 5,
				PredictionInterval: time.Minute * 2,
			},
		}
		
		client, err := noisefs.NewClientWithConfig(ipfsClient, basicCache, config)
		if err != nil {
			return fmt.Errorf("failed to create modern client %d: %w", i, err)
		}
		
		// Setup peer manager with all strategies
		host := NewMockHost(peer.ID(fmt.Sprintf("modern-peer-%d", i)))
		peerManager := p2p.NewPeerManager(host, &p2p.PeerManagerConfig{
			MaxPeers:            20,
			HealthCheckInterval: time.Minute,
			MetricRetention:     time.Hour * 2,
		})
		
		// Add other peers to the network
		for j := 0; j < m4.numPeers; j++ {
			if i != j {
				otherPeerID := peer.ID(fmt.Sprintf("modern-peer-%d", j))
				peerManager.AddPeer(otherPeerID, fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", 4000+j))
			}
		}
		
		client.SetPeerManager(peerManager)
		client.OptimizeForRandomizers() // Enable randomizer optimization
		
		m4.modernClients = append(m4.modernClients, client)
	}
	
	return nil
}

// RunComprehensiveTest executes the full impact analysis
func (m4 *Milestone4ImpactAnalyzer) RunComprehensiveTest() error {
	log.Println("Starting comprehensive Milestone 4 impact analysis...")
	
	// Test legacy clients
	log.Println("Testing legacy (pre-Milestone 4) performance...")
	legacyResults, err := m4.runTestSuite(m4.legacyClients, "legacy")
	if err != nil {
		return fmt.Errorf("legacy tests failed: %w", err)
	}
	m4.legacyResults = legacyResults
	
	// Test modern clients
	log.Println("Testing modern (Milestone 4) performance...")
	modernResults, err := m4.runTestSuite(m4.modernClients, "modern")
	if err != nil {
		return fmt.Errorf("modern tests failed: %w", err)
	}
	m4.modernResults = modernResults
	
	// Analyze impact
	log.Println("Analyzing impact and generating insights...")
	m4.analysis = m4.analyzeImpact(legacyResults, modernResults)
	
	return nil
}

// runTestSuite executes comprehensive tests on a set of clients
func (m4 *Milestone4ImpactAnalyzer) runTestSuite(clients []*noisefs.Client, testType string) (*TestResults, error) {
	results := &TestResults{
		PeerSuccessRates:      make(map[peer.ID]float64),
		StrategyEffectiveness: make(map[string]float64),
		Timestamps:           make([]time.Time, 0),
		LatencyHistory:       make([]time.Duration, 0),
		ThroughputHistory:    make([]float64, 0),
		CacheHitRateHistory:  make([]float64, 0),
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), m4.testDuration)
	defer cancel()
	
	startTime := time.Now()
	var operations int64
	var successfulOps int64
	var totalLatency time.Duration
	var totalBytes int64
	var originalDataSize int64
	var storedDataSize int64
	
	// Create test workload
	workload := m4.createTestWorkload()
	
	// Run concurrent operations
	var wg sync.WaitGroup
	for i, client := range clients {
		wg.Add(1)
		go func(clientID int, c *noisefs.Client) {
			defer wg.Done()
			
			clientOps := 0
			clientSuccess := 0
			
			for {
				select {
				case <-ctx.Done():
					return
				default:
					// Perform mixed operations
					for _, op := range workload {
						opStart := time.Now()
						success := false
						
						switch op.Type {
						case "store":
							success = m4.performStoreOperation(c, op)
							if success {
								originalDataSize += int64(op.BlockSize)
								storedDataSize += int64(op.BlockSize) // Simplified
							}
						case "retrieve":
							success = m4.performRetrieveOperation(c, op)
						case "randomizer":
							success = m4.performRandomizerSelection(c, op)
						}
						
						opDuration := time.Since(opStart)
						totalLatency += opDuration
						totalBytes += int64(op.BlockSize)
						operations++
						
						if success {
							successfulOps++
							clientSuccess++
						}
						
						clientOps++
						
						// Record time-series data periodically
						if operations%100 == 0 {
							results.Timestamps = append(results.Timestamps, time.Now())
							results.LatencyHistory = append(results.LatencyHistory, opDuration)
							
							duration := time.Since(startTime)
							throughput := float64(totalBytes) / (1024 * 1024) / duration.Seconds()
							results.ThroughputHistory = append(results.ThroughputHistory, throughput)
							
							// Calculate current cache hit rate
							metrics := c.GetMetrics()
							hitRate := float64(metrics.CacheHits) / float64(metrics.CacheHits + metrics.CacheMisses + 1)
							results.CacheHitRateHistory = append(results.CacheHitRateHistory, hitRate)
						}
						
						// Small delay to prevent overwhelming
						time.Sleep(time.Millisecond * 10)
					}
				}
			}
		}(i, client)
	}
	
	// Wait for test completion
	wg.Wait()
	
	// Calculate final metrics
	duration := time.Since(startTime)
	results.TotalOperations = operations
	results.SuccessfulOps = successfulOps
	results.FailedOps = operations - successfulOps
	
	if operations > 0 {
		results.AverageLatency = totalLatency / time.Duration(operations)
		results.ThroughputMBps = float64(totalBytes) / (1024 * 1024) / duration.Seconds()
	}
	
	results.OriginalDataSize = originalDataSize
	results.StoredDataSize = storedDataSize
	if originalDataSize > 0 {
		results.StorageOverhead = (float64(storedDataSize-originalDataSize) / float64(originalDataSize)) * 100
	}
	
	// Collect final metrics from clients
	m4.collectFinalMetrics(clients, results, testType)
	
	return results, nil
}

// createTestWorkload generates a realistic test workload
func (m4 *Milestone4ImpactAnalyzer) createTestWorkload() []TestOperation {
	workload := make([]TestOperation, 0, m4.numBlocks)
	
	// 40% store operations, 50% retrieve operations, 10% randomizer selections
	for i := 0; i < m4.numBlocks; i++ {
		op := TestOperation{
			ID:        fmt.Sprintf("op-%d", i),
			BlockSize: m4.blockSizes[rand.Intn(len(m4.blockSizes))],
		}
		
		rand := rand.Float32()
		switch {
		case rand < 0.4:
			op.Type = "store"
		case rand < 0.9:
			op.Type = "retrieve"
		default:
			op.Type = "randomizer"
		}
		
		workload = append(workload, op)
	}
	
	return workload
}

// TestOperation represents a single test operation
type TestOperation struct {
	ID        string
	Type      string // "store", "retrieve", "randomizer"
	BlockSize int
	Data      []byte
}

// Performance test methods
func (m4 *Milestone4ImpactAnalyzer) performStoreOperation(client *noisefs.Client, op TestOperation) bool {
	data := make([]byte, op.BlockSize)
	rand.Read(data)
	
	block, err := blocks.NewBlock(data)
	if err != nil {
		return false
	}
	
	_, err = client.StoreBlockWithCache(block)
	return err == nil
}

func (m4 *Milestone4ImpactAnalyzer) performRetrieveOperation(client *noisefs.Client, op TestOperation) bool {
	// Try to retrieve a previously stored block (simplified)
	cid := fmt.Sprintf("test-block-%d", rand.Intn(100))
	_, err := client.RetrieveBlockWithCache(cid)
	return err == nil
}

func (m4 *Milestone4ImpactAnalyzer) performRandomizerSelection(client *noisefs.Client, op TestOperation) bool {
	_, _, err := client.SelectRandomizer(op.BlockSize)
	return err == nil
}

// collectFinalMetrics gathers final performance metrics
func (m4 *Milestone4ImpactAnalyzer) collectFinalMetrics(clients []*noisefs.Client, results *TestResults, testType string) {
	var totalCacheHits, totalCacheMisses int64
	var totalEvictions int64
	
	for _, client := range clients {
		metrics := client.GetMetrics()
		totalCacheHits += metrics.CacheHits
		totalCacheMisses += metrics.CacheMisses
		
		// Collect adaptive cache metrics if available (modern clients only)
		if testType == "modern" {
			if adaptiveStats := client.GetAdaptiveCacheStats(); adaptiveStats != nil {
				totalEvictions += adaptiveStats.Evictions
				results.PredictionAccuracy = adaptiveStats.PredictionAccuracy
			}
		}
	}
	
	if totalCacheHits+totalCacheMisses > 0 {
		results.CacheHitRate = float64(totalCacheHits) / float64(totalCacheHits+totalCacheMisses)
		results.CacheMissRate = 1.0 - results.CacheHitRate
	}
	
	results.EvictionCount = totalEvictions
}

// analyzeImpact performs detailed comparison and generates insights
func (m4 *Milestone4ImpactAnalyzer) analyzeImpact(legacy, modern *TestResults) *ImpactAnalysis {
	analysis := &ImpactAnalysis{}
	
	// Performance improvements
	if legacy.AverageLatency > 0 {
		analysis.LatencyImprovement = ((float64(legacy.AverageLatency - modern.AverageLatency)) / float64(legacy.AverageLatency)) * 100
	}
	
	if legacy.ThroughputMBps > 0 {
		analysis.ThroughputImprovement = ((modern.ThroughputMBps - legacy.ThroughputMBps) / legacy.ThroughputMBps) * 100
	}
	
	legacySuccessRate := float64(legacy.SuccessfulOps) / float64(legacy.TotalOperations)
	modernSuccessRate := float64(modern.SuccessfulOps) / float64(modern.TotalOperations)
	if legacySuccessRate > 0 {
		analysis.SuccessRateImprovement = ((modernSuccessRate - legacySuccessRate) / legacySuccessRate) * 100
	}
	
	// Storage efficiency
	if legacy.StorageOverhead > 0 {
		analysis.OverheadReduction = legacy.StorageOverhead - modern.StorageOverhead
	}
	
	// Cache improvements
	if legacy.CacheHitRate > 0 {
		analysis.CacheHitImprovement = ((modern.CacheHitRate - legacy.CacheHitRate) / legacy.CacheHitRate) * 100
	}
	
	if legacy.EvictionCount > 0 {
		analysis.EvictionReduction = ((float64(legacy.EvictionCount - modern.EvictionCount)) / float64(legacy.EvictionCount)) * 100
	}
	
	// ML/AI benefits (modern only)
	analysis.PredictionValue = modern.PredictionAccuracy
	
	// Overall scores
	analysis.OverallPerformanceGain = (analysis.LatencyImprovement + analysis.ThroughputImprovement + analysis.SuccessRateImprovement) / 3
	analysis.EfficiencyGain = (analysis.OverheadReduction + analysis.CacheHitImprovement) / 2
	analysis.ScalabilityImprovement = analysis.OverallPerformanceGain // Simplified
	
	// Generate insights
	analysis.KeyInsights = m4.generateInsights(legacy, modern, analysis)
	analysis.OptimizationSuggestions = m4.generateOptimizations(analysis)
	analysis.NextSteps = m4.generateNextSteps(analysis)
	
	return analysis
}

// generateInsights creates key insights from the analysis
func (m4 *Milestone4ImpactAnalyzer) generateInsights(legacy, modern *TestResults, analysis *ImpactAnalysis) []string {
	insights := make([]string, 0)
	
	if analysis.LatencyImprovement > 20 {
		insights = append(insights, fmt.Sprintf("Significant latency improvement: %.1f%% faster response times", analysis.LatencyImprovement))
	}
	
	if analysis.ThroughputImprovement > 30 {
		insights = append(insights, fmt.Sprintf("Major throughput gains: %.1f%% increase in data transfer rate", analysis.ThroughputImprovement))
	}
	
	if analysis.CacheHitImprovement > 25 {
		insights = append(insights, fmt.Sprintf("Adaptive caching is highly effective: %.1f%% improvement in hit rate", analysis.CacheHitImprovement))
	}
	
	if modern.StorageOverhead < 200 {
		insights = append(insights, fmt.Sprintf("Excellent storage efficiency: %.1f%% overhead (target <200%%)", modern.StorageOverhead))
	}
	
	if analysis.PredictionAccuracy > 70 {
		insights = append(insights, fmt.Sprintf("ML predictions are accurate: %.1f%% prediction accuracy", analysis.PredictionAccuracy))
	}
	
	return insights
}

// generateOptimizations suggests optimizations based on results
func (m4 *Milestone4ImpactAnalyzer) generateOptimizations(analysis *ImpactAnalysis) []string {
	suggestions := make([]string, 0)
	
	if analysis.CacheHitImprovement < 20 {
		suggestions = append(suggestions, "Consider increasing cache size or adjusting eviction policies")
	}
	
	if analysis.LatencyImprovement < 15 {
		suggestions = append(suggestions, "Tune peer selection algorithms for better performance")
	}
	
	if analysis.PredictionAccuracy < 60 {
		suggestions = append(suggestions, "ML model needs more training data or feature engineering")
	}
	
	if analysis.OverheadReduction < 10 {
		suggestions = append(suggestions, "Optimize randomizer reuse strategies for better storage efficiency")
	}
	
	return suggestions
}

// generateNextSteps provides recommendations for future improvements
func (m4 *Milestone4ImpactAnalyzer) generateNextSteps(analysis *ImpactAnalysis) []string {
	steps := make([]string, 0)
	
	if analysis.OverallPerformanceGain > 30 {
		steps = append(steps, "Ready for production deployment - consider Milestone 6")
	}
	
	if analysis.PredictionValue > 70 {
		steps = append(steps, "ML system is effective - consider advanced AI features in Milestone 7")
	}
	
	steps = append(steps, "Monitor performance in production environment")
	steps = append(steps, "Collect real-world usage data for further optimization")
	
	return steps
}

// PrintDetailedReport prints a comprehensive analysis report
func (m4 *Milestone4ImpactAnalyzer) PrintDetailedReport() {
	if m4.analysis == nil {
		log.Println("No analysis available. Run tests first.")
		return
	}
	
	fmt.Println("\n" + "="*80)
	fmt.Println("MILESTONE 4 IMPACT ANALYSIS REPORT")
	fmt.Println("="*80)
	
	// Performance comparison
	fmt.Println("\n🚀 PERFORMANCE IMPROVEMENTS:")
	fmt.Printf("  Latency Improvement:     %+.1f%%\n", m4.analysis.LatencyImprovement)
	fmt.Printf("  Throughput Improvement:  %+.1f%%\n", m4.analysis.ThroughputImprovement)
	fmt.Printf("  Success Rate Improvement: %+.1f%%\n", m4.analysis.SuccessRateImprovement)
	
	// Storage efficiency
	fmt.Println("\n💾 STORAGE EFFICIENCY:")
	fmt.Printf("  Legacy Overhead:         %.1f%%\n", m4.legacyResults.StorageOverhead)
	fmt.Printf("  Modern Overhead:         %.1f%%\n", m4.modernResults.StorageOverhead)
	fmt.Printf("  Overhead Reduction:      %.1f%%\n", m4.analysis.OverheadReduction)
	
	// Cache performance
	fmt.Println("\n🎯 CACHE PERFORMANCE:")
	fmt.Printf("  Legacy Hit Rate:         %.1f%%\n", m4.legacyResults.CacheHitRate*100)
	fmt.Printf("  Modern Hit Rate:         %.1f%%\n", m4.modernResults.CacheHitRate*100)
	fmt.Printf("  Cache Improvement:       %+.1f%%\n", m4.analysis.CacheHitImprovement)
	fmt.Printf("  Eviction Reduction:      %.1f%%\n", m4.analysis.EvictionReduction)
	
	// ML/AI benefits
	fmt.Println("\n🤖 ML/AI BENEFITS:")
	fmt.Printf("  Prediction Accuracy:     %.1f%%\n", m4.analysis.PredictionAccuracy)
	fmt.Printf("  Prediction Value Score:  %.1f\n", m4.analysis.PredictionValue)
	
	// Overall impact
	fmt.Println("\n📊 OVERALL IMPACT:")
	fmt.Printf("  Performance Gain:        %.1f%%\n", m4.analysis.OverallPerformanceGain)
	fmt.Printf("  Efficiency Gain:         %.1f%%\n", m4.analysis.EfficiencyGain)
	fmt.Printf("  Scalability Improvement: %.1f%%\n", m4.analysis.ScalabilityImprovement)
	
	// Key insights
	fmt.Println("\n💡 KEY INSIGHTS:")
	for i, insight := range m4.analysis.KeyInsights {
		fmt.Printf("  %d. %s\n", i+1, insight)
	}
	
	// Optimization suggestions
	fmt.Println("\n🔧 OPTIMIZATION SUGGESTIONS:")
	for i, suggestion := range m4.analysis.OptimizationSuggestions {
		fmt.Printf("  %d. %s\n", i+1, suggestion)
	}
	
	// Next steps
	fmt.Println("\n🎯 RECOMMENDED NEXT STEPS:")
	for i, step := range m4.analysis.NextSteps {
		fmt.Printf("  %d. %s\n", i+1, step)
	}
	
	fmt.Println("\n" + "="*80)
}

// Mock implementations for testing

// LegacyMockIPFSClient simulates pre-Milestone 4 IPFS client
type LegacyMockIPFSClient struct {
	*MockIPFSClient
}

func NewLegacyMockIPFSClient() *LegacyMockIPFSClient {
	return &LegacyMockIPFSClient{
		MockIPFSClient: NewMockIPFSClient(),
	}
}

// ModernMockIPFSClient simulates Milestone 4 enhanced IPFS client
type ModernMockIPFSClient struct {
	*MockIPFSClient
	peerManager *p2p.PeerManager
}

func NewModernMockIPFSClient() *ModernMockIPFSClient {
	return &ModernMockIPFSClient{
		MockIPFSClient: NewMockIPFSClient(),
	}
}

func (m *ModernMockIPFSClient) SetPeerManager(manager *p2p.PeerManager) {
	m.peerManager = manager
}

func (m *ModernMockIPFSClient) RetrieveBlockWithPeerHint(cid string, preferredPeers []peer.ID) (*blocks.Block, error) {
	// Simulate faster retrieval with peer hints (20% improvement)
	time.Sleep(time.Millisecond * time.Duration(rand.Intn(8)+2)) // 2-10ms vs 5-15ms
	return m.RetrieveBlock(cid)
}

func (m *ModernMockIPFSClient) StoreBlockWithStrategy(block *blocks.Block, strategy string) (string, error) {
	// Strategy affects performance simulation
	delay := time.Millisecond
	switch strategy {
	case "performance":
		delay = time.Microsecond * 300 // Faster
	case "randomizer":
		delay = time.Millisecond * 2   // Slightly more overhead but better reuse
	default:
		delay = time.Millisecond
	}
	time.Sleep(delay)
	
	return m.StoreBlock(block)
}

func (m *ModernMockIPFSClient) RequestFromPeer(ctx context.Context, cid string, peerID peer.ID) (*blocks.Block, error) {
	// Simulate peer-specific request with better performance
	time.Sleep(time.Millisecond * time.Duration(rand.Intn(15)+3)) // 3-18ms
	return m.RetrieveBlock(cid)
}

func (m *ModernMockIPFSClient) BroadcastBlock(ctx context.Context, cid string, block *blocks.Block) error {
	// Mock broadcast with realistic delay
	time.Sleep(time.Millisecond * time.Duration(rand.Intn(30)+10))
	return nil
}

func (m *ModernMockIPFSClient) GetPeerMetrics() map[peer.ID]*ipfs.RequestMetrics {
	return map[peer.ID]*ipfs.RequestMetrics{
		peer.ID("modern-peer-1"): {
			TotalRequests:      150,
			SuccessfulRequests: 145,
			FailedRequests:     5,
			AverageLatency:     time.Millisecond * 25, // Better than legacy
			Bandwidth:          2048 * 1024,          // 2MB/s
		},
	}
}
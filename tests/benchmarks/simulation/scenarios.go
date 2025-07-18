package simulation

import (
	"fmt"
	"time"
)

// ScenarioType defines different types of network scenarios
type ScenarioType int

const (
	SmallNetwork ScenarioType = iota
	MediumNetwork
	LargeNetwork
	MassiveNetwork
	PopularContent
	UniformDistribution
	ContentTypeVaried
)

// Scenario represents a specific test scenario
type Scenario struct {
	Name        string
	Type        ScenarioType
	Config      *SimulationConfig
	Description string
}

// ScenarioRunner manages and executes multiple scenarios
type ScenarioRunner struct {
	scenarios []*Scenario
	results   map[string]*ScenarioResult
}

// ScenarioResult contains the results of running a scenario
type ScenarioResult struct {
	Scenario      *Scenario
	Metrics       *GlobalMetrics
	Duration      time.Duration
	Summary       string
	Efficiency    EfficiencyMetrics
	Performance   PerformanceMetrics
	Scalability   ScalabilityMetrics
}

// EfficiencyMetrics tracks storage and block reuse efficiency
type EfficiencyMetrics struct {
	StorageOverhead       float64 // Multiplier over original size
	BlockReuseRate        float64 // Percentage of blocks reused
	CacheEfficiency       float64 // Average cache hit rate
	DeduplicationSavings  float64 // Percentage of storage saved
}

// PerformanceMetrics tracks system performance
type PerformanceMetrics struct {
	AverageUploadTime     time.Duration
	AverageDownloadTime   time.Duration
	ThroughputMBps        float64
	MemoryUsagePerNode    int64
	CacheHitRatio         float64
}

// ScalabilityMetrics tracks how well the system scales
type ScalabilityMetrics struct {
	NodesPerSecond        float64 // Operations per second per node
	StorageScalingFactor  float64 // How storage grows with network size
	NetworkEfficiencyLoss float64 // Efficiency loss as network grows
}

// NewScenarioRunner creates a new scenario runner
func NewScenarioRunner() *ScenarioRunner {
	return &ScenarioRunner{
		scenarios: make([]*Scenario, 0),
		results:   make(map[string]*ScenarioResult),
	}
}

// AddScenario adds a scenario to the runner
func (sr *ScenarioRunner) AddScenario(scenario *Scenario) {
	sr.scenarios = append(sr.scenarios, scenario)
}

// CreateDefaultScenarios creates a set of default scaling scenarios
func (sr *ScenarioRunner) CreateDefaultScenarios() {
	// Small network scenario (10 nodes)
	sr.AddScenario(&Scenario{
		Name: "Small Network",
		Type: SmallNetwork,
		Config: &SimulationConfig{
			NumNodes:           10,
			CacheSize:          100,
			NumFiles:           50,
			FileSizeRange:      [2]int{1024, 10 * 1024 * 1024}, // 1KB to 10MB
			PopularityFactor:   0.8,
			SimulationDuration: 30 * time.Second,
			UploadRate:         0.5,
			DownloadRate:       2.0,
		},
		Description: "Small network with 10 nodes to establish baseline performance",
	})

	// Medium network scenario (100 nodes)
	sr.AddScenario(&Scenario{
		Name: "Medium Network",
		Type: MediumNetwork,
		Config: &SimulationConfig{
			NumNodes:           100,
			CacheSize:          200,
			NumFiles:           500,
			FileSizeRange:      [2]int{1024, 10 * 1024 * 1024},
			PopularityFactor:   0.8,
			SimulationDuration: 45 * time.Second,
			UploadRate:         0.3,
			DownloadRate:       1.5,
		},
		Description: "Medium network with 100 nodes to test moderate scaling",
	})

	// Large network scenario (1000 nodes)
	sr.AddScenario(&Scenario{
		Name: "Large Network",
		Type: LargeNetwork,
		Config: &SimulationConfig{
			NumNodes:           1000,
			CacheSize:          500,
			NumFiles:           5000,
			FileSizeRange:      [2]int{1024, 10 * 1024 * 1024},
			PopularityFactor:   0.8,
			SimulationDuration: 60 * time.Second,
			UploadRate:         0.2,
			DownloadRate:       1.0,
		},
		Description: "Large network with 1000 nodes to test significant scaling",
	})

	// Massive network scenario (10000 nodes)
	sr.AddScenario(&Scenario{
		Name: "Massive Network",
		Type: MassiveNetwork,
		Config: &SimulationConfig{
			NumNodes:           10000,
			CacheSize:          1000,
			NumFiles:           50000,
			FileSizeRange:      [2]int{1024, 10 * 1024 * 1024},
			PopularityFactor:   0.8,
			SimulationDuration: 90 * time.Second,
			UploadRate:         0.1,
			DownloadRate:       0.5,
		},
		Description: "Massive network with 10000 nodes to test extreme scaling",
	})

	// Popular content scenario (focus on high reuse)
	sr.AddScenario(&Scenario{
		Name: "Popular Content",
		Type: PopularContent,
		Config: &SimulationConfig{
			NumNodes:           500,
			CacheSize:          300,
			NumFiles:           1000,
			FileSizeRange:      [2]int{1024, 5 * 1024 * 1024},
			PopularityFactor:   0.3, // Higher concentration of popular files
			SimulationDuration: 45 * time.Second,
			UploadRate:         0.2,
			DownloadRate:       2.0, // Higher download rate
		},
		Description: "Network with highly popular content to maximize block reuse",
	})

	// Uniform distribution scenario (minimal reuse)
	sr.AddScenario(&Scenario{
		Name: "Uniform Distribution",
		Type: UniformDistribution,
		Config: &SimulationConfig{
			NumNodes:           500,
			CacheSize:          300,
			NumFiles:           1000,
			FileSizeRange:      [2]int{1024, 5 * 1024 * 1024},
			PopularityFactor:   2.0, // More uniform distribution
			SimulationDuration: 45 * time.Second,
			UploadRate:         0.4,
			DownloadRate:       0.8,
		},
		Description: "Network with uniform content distribution to test minimal reuse",
	})

	// Content type varied scenario (different file sizes)
	sr.AddScenario(&Scenario{
		Name: "Content Type Varied",
		Type: ContentTypeVaried,
		Config: &SimulationConfig{
			NumNodes:           500,
			CacheSize:          400,
			NumFiles:           2000,
			FileSizeRange:      [2]int{512, 100 * 1024 * 1024}, // 512B to 100MB
			PopularityFactor:   0.8,
			SimulationDuration: 60 * time.Second,
			UploadRate:         0.3,
			DownloadRate:       1.2,
		},
		Description: "Network with varied content types and sizes",
	})
}

// RunAllScenarios executes all configured scenarios
func (sr *ScenarioRunner) RunAllScenarios() error {
	fmt.Printf("Running %d scenarios...\n", len(sr.scenarios))

	for i, scenario := range sr.scenarios {
		fmt.Printf("\n[%d/%d] Running scenario: %s\n", i+1, len(sr.scenarios), scenario.Name)
		fmt.Printf("Description: %s\n", scenario.Description)
		
		result, err := sr.runScenario(scenario)
		if err != nil {
			return fmt.Errorf("scenario %s failed: %w", scenario.Name, err)
		}
		
		sr.results[scenario.Name] = result
		sr.printScenarioResults(result)
	}

	return nil
}

// runScenario executes a single scenario
func (sr *ScenarioRunner) runScenario(scenario *Scenario) (*ScenarioResult, error) {
	start := time.Now()
	
	// Create and run simulation
	sim := NewNetworkSimulation(scenario.Config)
	err := sim.Run()
	if err != nil {
		return nil, err
	}
	
	duration := time.Since(start)
	metrics := sim.GetGlobalMetrics()
	
	// Calculate efficiency metrics
	efficiency := sr.calculateEfficiencyMetrics(metrics)
	performance := sr.calculatePerformanceMetrics(metrics, duration)
	scalability := sr.calculateScalabilityMetrics(metrics, scenario.Config)
	
	result := &ScenarioResult{
		Scenario:    scenario,
		Metrics:     metrics,
		Duration:    duration,
		Efficiency:  efficiency,
		Performance: performance,
		Scalability: scalability,
	}
	
	result.Summary = sr.generateSummary(result)
	
	return result, nil
}

// calculateEfficiencyMetrics computes storage and reuse efficiency
func (sr *ScenarioRunner) calculateEfficiencyMetrics(metrics *GlobalMetrics) EfficiencyMetrics {
	storageOverhead := 0.0
	if metrics.TotalBytesOriginal > 0 {
		storageOverhead = ((float64(metrics.TotalBytesStored) - float64(metrics.TotalBytesOriginal)) / float64(metrics.TotalBytesOriginal)) * 100.0
	}
	
	deduplicationSavings := 0.0
	if metrics.TotalBlocks > 0 {
		deduplicationSavings = float64(metrics.TotalBlocks-metrics.UniqueBlocks) / float64(metrics.TotalBlocks) * 100.0
	}
	
	// Calculate average cache efficiency across nodes
	avgCacheHitRate := 0.0
	if len(metrics.NodeMetrics) > 0 {
		totalHitRate := 0.0
		for _, nodeMetrics := range metrics.NodeMetrics {
			totalHitRate += nodeMetrics.CacheHitRate
		}
		avgCacheHitRate = totalHitRate / float64(len(metrics.NodeMetrics))
	}
	
	return EfficiencyMetrics{
		StorageOverhead:      storageOverhead,
		BlockReuseRate:       metrics.NetworkBlockReuseRate,
		CacheEfficiency:      avgCacheHitRate,
		DeduplicationSavings: deduplicationSavings,
	}
}

// calculatePerformanceMetrics computes system performance metrics
func (sr *ScenarioRunner) calculatePerformanceMetrics(metrics *GlobalMetrics, duration time.Duration) PerformanceMetrics {
	avgUploadTime := duration / time.Duration(metrics.TotalUploads)
	avgDownloadTime := duration / time.Duration(metrics.TotalDownloads)
	
	throughputMBps := float64(metrics.TotalBytesOriginal) / (1024 * 1024) / duration.Seconds()
	
	// Estimate memory usage per node
	memoryPerNode := int64(0)
	if metrics.TotalNodes > 0 {
		// Rough estimate: cache size + overhead
		memoryPerNode = (metrics.UniqueBlocks * 128 * 1024) / int64(metrics.TotalNodes)
	}
	
	return PerformanceMetrics{
		AverageUploadTime:   avgUploadTime,
		AverageDownloadTime: avgDownloadTime,
		ThroughputMBps:      throughputMBps,
		MemoryUsagePerNode:  memoryPerNode,
		CacheHitRatio:       metrics.NodeMetrics[fmt.Sprintf("node-0")].CacheHitRate,
	}
}

// calculateScalabilityMetrics computes how well the system scales
func (sr *ScenarioRunner) calculateScalabilityMetrics(metrics *GlobalMetrics, config *SimulationConfig) ScalabilityMetrics {
	nodesPerSecond := float64(metrics.TotalUploads+metrics.TotalDownloads) / float64(config.NumNodes) / config.SimulationDuration.Seconds()
	
	// Storage scaling factor (how storage grows with network size)
	storageScalingFactor := float64(metrics.TotalBytesStored) / float64(config.NumNodes)
	
	// Efficiency loss as network grows (placeholder - would need baseline)
	efficiencyLoss := 0.0
	
	return ScalabilityMetrics{
		NodesPerSecond:        nodesPerSecond,
		StorageScalingFactor:  storageScalingFactor,
		NetworkEfficiencyLoss: efficiencyLoss,
	}
}

// generateSummary creates a human-readable summary of results
func (sr *ScenarioRunner) generateSummary(result *ScenarioResult) string {
	return fmt.Sprintf(
		"Scenario completed in %.2fs with %.1f%% block reuse, %.2fx storage overhead, %.1f%% cache hit rate",
		result.Duration.Seconds(),
		result.Efficiency.BlockReuseRate,
		result.Efficiency.StorageOverhead,
		result.Efficiency.CacheEfficiency,
	)
}

// printScenarioResults prints detailed results for a scenario
func (sr *ScenarioRunner) printScenarioResults(result *ScenarioResult) {
	fmt.Printf("\n=== Results for %s ===\n", result.Scenario.Name)
	fmt.Printf("Duration: %.2fs\n", result.Duration.Seconds())
	fmt.Printf("Summary: %s\n", result.Summary)
	
	fmt.Printf("\nNetwork Stats:\n")
	fmt.Printf("  Nodes: %d\n", result.Metrics.TotalNodes)
	fmt.Printf("  Files: %d\n", result.Metrics.TotalFiles)
	fmt.Printf("  Total Blocks: %d\n", result.Metrics.TotalBlocks)
	fmt.Printf("  Unique Blocks: %d\n", result.Metrics.UniqueBlocks)
	fmt.Printf("  Uploads: %d\n", result.Metrics.TotalUploads)
	fmt.Printf("  Downloads: %d\n", result.Metrics.TotalDownloads)
	
	fmt.Printf("\nEfficiency Metrics:\n")
	fmt.Printf("  Storage Overhead: %.2fx\n", result.Efficiency.StorageOverhead)
	fmt.Printf("  Block Reuse Rate: %.1f%%\n", result.Efficiency.BlockReuseRate)
	fmt.Printf("  Cache Efficiency: %.1f%%\n", result.Efficiency.CacheEfficiency)
	fmt.Printf("  Deduplication Savings: %.1f%%\n", result.Efficiency.DeduplicationSavings)
	
	fmt.Printf("\nPerformance Metrics:\n")
	fmt.Printf("  Throughput: %.2f MB/s\n", result.Performance.ThroughputMBps)
	fmt.Printf("  Memory/Node: %.2f MB\n", float64(result.Performance.MemoryUsagePerNode)/(1024*1024))
	fmt.Printf("  Cache Hit Ratio: %.1f%%\n", result.Performance.CacheHitRatio)
	
	fmt.Printf("\nScalability Metrics:\n")
	fmt.Printf("  Operations/Node/Second: %.2f\n", result.Scalability.NodesPerSecond)
	fmt.Printf("  Storage Scaling Factor: %.2f bytes/node\n", result.Scalability.StorageScalingFactor)
}

// GetResults returns all scenario results
func (sr *ScenarioRunner) GetResults() map[string]*ScenarioResult {
	return sr.results
}

// GenerateComparisonReport generates a comparison report across all scenarios
func (sr *ScenarioRunner) GenerateComparisonReport() string {
	report := "\n=== SCALING COMPARISON REPORT ===\n\n"
	
	report += fmt.Sprintf("%-20s %-8s %-10s %-12s %-10s %-10s\n", 
		"Scenario", "Nodes", "Block Reuse", "Storage OH", "Cache Hit", "Throughput")
	report += fmt.Sprintf("%-20s %-8s %-10s %-12s %-10s %-10s\n", 
		"--------", "-----", "----------", "----------", "---------", "----------")
	
	for _, scenario := range sr.scenarios {
		if result, exists := sr.results[scenario.Name]; exists {
			report += fmt.Sprintf("%-20s %-8d %-10.1f%% %-12.2fx %-10.1f%% %-10.2f MB/s\n",
				scenario.Name,
				result.Metrics.TotalNodes,
				result.Efficiency.BlockReuseRate,
				result.Efficiency.StorageOverhead,
				result.Efficiency.CacheEfficiency,
				result.Performance.ThroughputMBps,
			)
		}
	}
	
	return report
}
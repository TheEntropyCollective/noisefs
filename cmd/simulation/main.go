package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/simulation"
)

func main() {
	var (
		scenario     = flag.String("scenario", "all", "Scenario to run (all, small, medium, large, massive, popular, uniform, varied)")
		duration     = flag.Duration("duration", 30*time.Second, "Simulation duration")
		nodes        = flag.Int("nodes", 0, "Number of nodes (0 = use scenario default)")
		files        = flag.Int("files", 0, "Number of files (0 = use scenario default)")
		cacheSize    = flag.Int("cache", 0, "Cache size per node (0 = use scenario default)")
		_ = flag.Bool("verbose", false, "Enable verbose output")
		output       = flag.String("output", "", "Output file for results (optional)")
		comparison   = flag.Bool("comparison", true, "Generate comparison report")
		maxProcs     = flag.Int("maxprocs", 0, "Maximum number of CPU cores to use (0 = use all)")
	)
	flag.Parse()

	// Set CPU limits if specified
	if *maxProcs > 0 {
		runtime.GOMAXPROCS(*maxProcs)
	}

	fmt.Printf("NoiseFS Network Scaling Simulation\n")
	fmt.Printf("===================================\n")
	fmt.Printf("CPU Cores: %d\n", runtime.NumCPU())
	fmt.Printf("Max Procs: %d\n", runtime.GOMAXPROCS(0))
	fmt.Printf("Scenario: %s\n", *scenario)
	fmt.Printf("Duration: %s\n", *duration)
	fmt.Printf("\n")

	// Create scenario runner
	runner := simulation.NewScenarioRunner()
	
	// Add scenarios based on command line arguments
	switch *scenario {
	case "all":
		runner.CreateDefaultScenarios()
	case "small":
		runner.AddScenario(createCustomScenario("Small Network", 10, 50, 100, *duration, *nodes, *files, *cacheSize))
	case "medium":
		runner.AddScenario(createCustomScenario("Medium Network", 100, 500, 200, *duration, *nodes, *files, *cacheSize))
	case "large":
		runner.AddScenario(createCustomScenario("Large Network", 1000, 5000, 500, *duration, *nodes, *files, *cacheSize))
	case "massive":
		runner.AddScenario(createCustomScenario("Massive Network", 10000, 50000, 1000, *duration, *nodes, *files, *cacheSize))
	case "popular":
		runner.AddScenario(createPopularContentScenario(*duration, *nodes, *files, *cacheSize))
	case "uniform":
		runner.AddScenario(createUniformDistributionScenario(*duration, *nodes, *files, *cacheSize))
	case "varied":
		runner.AddScenario(createContentTypeVariedScenario(*duration, *nodes, *files, *cacheSize))
	default:
		fmt.Printf("Unknown scenario: %s\n", *scenario)
		fmt.Printf("Available scenarios: all, small, medium, large, massive, popular, uniform, varied\n")
		os.Exit(1)
	}

	// Run scenarios
	start := time.Now()
	err := runner.RunAllScenarios()
	if err != nil {
		log.Fatalf("Simulation failed: %v", err)
	}
	totalTime := time.Since(start)

	// Generate reports
	if *comparison {
		fmt.Printf("\n")
		fmt.Printf("%s", runner.GenerateComparisonReport())
	}

	// Print summary
	fmt.Printf("\n=== SIMULATION SUMMARY ===\n")
	fmt.Printf("Total simulation time: %.2fs\n", totalTime.Seconds())
	fmt.Printf("Scenarios completed: %d\n", len(runner.GetResults()))
	
	// Analyze scaling trends
	analyzeScalingTrends(runner.GetResults())

	// Save results to file if specified
	if *output != "" {
		err := saveResultsToFile(runner.GetResults(), *output)
		if err != nil {
			log.Printf("Failed to save results to file: %v", err)
		} else {
			fmt.Printf("Results saved to: %s\n", *output)
		}
	}

	fmt.Printf("\nSimulation completed successfully!\n")
}

// createCustomScenario creates a custom scenario with optional overrides
func createCustomScenario(name string, defaultNodes, defaultFiles, defaultCache int, duration time.Duration, nodeOverride, fileOverride, cacheOverride int) *simulation.Scenario {
	nodes := defaultNodes
	files := defaultFiles
	cache := defaultCache
	
	if nodeOverride > 0 {
		nodes = nodeOverride
	}
	if fileOverride > 0 {
		files = fileOverride
	}
	if cacheOverride > 0 {
		cache = cacheOverride
	}
	
	return &simulation.Scenario{
		Name: name,
		Config: &simulation.SimulationConfig{
			NumNodes:           nodes,
			CacheSize:          cache,
			NumFiles:           files,
			FileSizeRange:      [2]int{1024, 10 * 1024 * 1024}, // 1KB to 10MB
			PopularityFactor:   0.8,
			SimulationDuration: duration,
			UploadRate:         0.3,
			DownloadRate:       1.0,
		},
		Description: fmt.Sprintf("Custom %s scenario with %d nodes, %d files, %d cache size", name, nodes, files, cache),
	}
}

// createPopularContentScenario creates a scenario focused on popular content
func createPopularContentScenario(duration time.Duration, nodeOverride, fileOverride, cacheOverride int) *simulation.Scenario {
	nodes := 500
	files := 1000
	cache := 300
	
	if nodeOverride > 0 {
		nodes = nodeOverride
	}
	if fileOverride > 0 {
		files = fileOverride
	}
	if cacheOverride > 0 {
		cache = cacheOverride
	}
	
	return &simulation.Scenario{
		Name: "Popular Content",
		Config: &simulation.SimulationConfig{
			NumNodes:           nodes,
			CacheSize:          cache,
			NumFiles:           files,
			FileSizeRange:      [2]int{1024, 5 * 1024 * 1024}, // 1KB to 5MB
			PopularityFactor:   0.3, // More concentrated popularity
			SimulationDuration: duration,
			UploadRate:         0.2,
			DownloadRate:       2.0, // Higher download rate
		},
		Description: "Scenario with highly concentrated popular content",
	}
}

// createUniformDistributionScenario creates a scenario with uniform content distribution
func createUniformDistributionScenario(duration time.Duration, nodeOverride, fileOverride, cacheOverride int) *simulation.Scenario {
	nodes := 500
	files := 1000
	cache := 300
	
	if nodeOverride > 0 {
		nodes = nodeOverride
	}
	if fileOverride > 0 {
		files = fileOverride
	}
	if cacheOverride > 0 {
		cache = cacheOverride
	}
	
	return &simulation.Scenario{
		Name: "Uniform Distribution",
		Config: &simulation.SimulationConfig{
			NumNodes:           nodes,
			CacheSize:          cache,
			NumFiles:           files,
			FileSizeRange:      [2]int{1024, 5 * 1024 * 1024}, // 1KB to 5MB
			PopularityFactor:   2.0, // More uniform distribution
			SimulationDuration: duration,
			UploadRate:         0.4,
			DownloadRate:       0.8,
		},
		Description: "Scenario with uniform content distribution (minimal reuse)",
	}
}

// createContentTypeVariedScenario creates a scenario with varied content types
func createContentTypeVariedScenario(duration time.Duration, nodeOverride, fileOverride, cacheOverride int) *simulation.Scenario {
	nodes := 500
	files := 2000
	cache := 400
	
	if nodeOverride > 0 {
		nodes = nodeOverride
	}
	if fileOverride > 0 {
		files = fileOverride
	}
	if cacheOverride > 0 {
		cache = cacheOverride
	}
	
	return &simulation.Scenario{
		Name: "Content Type Varied",
		Config: &simulation.SimulationConfig{
			NumNodes:           nodes,
			CacheSize:          cache,
			NumFiles:           files,
			FileSizeRange:      [2]int{512, 100 * 1024 * 1024}, // 512B to 100MB
			PopularityFactor:   0.8,
			SimulationDuration: duration,
			UploadRate:         0.3,
			DownloadRate:       1.2,
		},
		Description: "Scenario with varied content types and file sizes",
	}
}

// analyzeScalingTrends analyzes how metrics change as network size increases
func analyzeScalingTrends(results map[string]*simulation.ScenarioResult) {
	fmt.Printf("\n=== SCALING TRENDS ANALYSIS ===\n")
	
	// Extract network scaling scenarios
	networkScenarios := []string{"Small Network", "Medium Network", "Large Network", "Massive Network"}
	
	fmt.Printf("\nStorage Efficiency vs Network Size:\n")
	for _, scenarioName := range networkScenarios {
		if result, exists := results[scenarioName]; exists {
			fmt.Printf("  %d nodes: %.2fx overhead, %.1f%% reuse\n", 
				result.Metrics.TotalNodes, 
				result.Efficiency.StorageOverhead,
				result.Efficiency.BlockReuseRate)
		}
	}
	
	fmt.Printf("\nPerformance vs Network Size:\n")
	for _, scenarioName := range networkScenarios {
		if result, exists := results[scenarioName]; exists {
			fmt.Printf("  %d nodes: %.2f MB/s throughput, %.1f%% cache hit rate\n", 
				result.Metrics.TotalNodes, 
				result.Performance.ThroughputMBps,
				result.Performance.CacheHitRatio)
		}
	}
	
	// Analyze content distribution effects
	fmt.Printf("\nContent Distribution Effects:\n")
	contentScenarios := []string{"Popular Content", "Uniform Distribution", "Content Type Varied"}
	for _, scenarioName := range contentScenarios {
		if result, exists := results[scenarioName]; exists {
			fmt.Printf("  %s: %.1f%% reuse, %.2fx overhead\n", 
				scenarioName,
				result.Efficiency.BlockReuseRate,
				result.Efficiency.StorageOverhead)
		}
	}
}

// saveResultsToFile saves simulation results to a file
func saveResultsToFile(results map[string]*simulation.ScenarioResult, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	
	fmt.Fprintf(file, "NoiseFS Network Scaling Simulation Results\n")
	fmt.Fprintf(file, "==========================================\n")
	fmt.Fprintf(file, "Generated: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))
	
	for scenarioName, result := range results {
		fmt.Fprintf(file, "Scenario: %s\n", scenarioName)
		fmt.Fprintf(file, "Duration: %.2fs\n", result.Duration.Seconds())
		fmt.Fprintf(file, "Nodes: %d\n", result.Metrics.TotalNodes)
		fmt.Fprintf(file, "Files: %d\n", result.Metrics.TotalFiles)
		fmt.Fprintf(file, "Block Reuse Rate: %.1f%%\n", result.Efficiency.BlockReuseRate)
		fmt.Fprintf(file, "Storage Overhead: %.2fx\n", result.Efficiency.StorageOverhead)
		fmt.Fprintf(file, "Cache Hit Rate: %.1f%%\n", result.Efficiency.CacheEfficiency)
		fmt.Fprintf(file, "Throughput: %.2f MB/s\n", result.Performance.ThroughputMBps)
		fmt.Fprintf(file, "\n")
	}
	
	return nil
}
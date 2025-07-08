package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/integration"
)

// Milestone4TestRunner executes comprehensive impact analysis
func main() {
	var (
		numPeers     = flag.Int("peers", 5, "Number of peers to simulate")
		numBlocks    = flag.Int("blocks", 1000, "Number of test operations")
		duration     = flag.Duration("duration", time.Minute*2, "Test duration")
		outputFile   = flag.String("output", "milestone4_analysis.json", "Output file for results")
		verbose      = flag.Bool("verbose", true, "Enable verbose output")
	)
	flag.Parse()

	if *verbose {
		log.SetOutput(os.Stdout)
	}

	log.Printf("Starting Milestone 4 Impact Analysis")
	log.Printf("Configuration: %d peers, %d operations, %v duration", *numPeers, *numBlocks, *duration)

	// Create analyzer
	analyzer := integration.NewMilestone4ImpactAnalyzer(*numPeers, *numBlocks, *duration)

	// Setup test environment
	log.Println("Setting up test environment...")
	if err := analyzer.SetupTestEnvironment(); err != nil {
		log.Fatalf("Failed to setup test environment: %v", err)
	}

	// Run comprehensive tests
	log.Println("Running comprehensive impact analysis...")
	if err := analyzer.RunComprehensiveTest(); err != nil {
		log.Fatalf("Tests failed: %v", err)
	}

	// Print detailed report
	analyzer.PrintDetailedReport()

	// Save results to file
	if err := saveResults(analyzer, *outputFile); err != nil {
		log.Printf("Warning: Failed to save results to %s: %v", *outputFile, err)
	} else {
		log.Printf("Results saved to %s", *outputFile)
	}

	log.Println("Milestone 4 Impact Analysis completed successfully!")
}

// saveResults saves the analysis results to a JSON file
func saveResults(analyzer *integration.Milestone4ImpactAnalyzer, filename string) error {
	// This would need to be implemented with proper getters from the analyzer
	// For now, we'll create a simplified output structure
	
	results := map[string]interface{}{
		"timestamp": time.Now(),
		"summary": map[string]interface{}{
			"test_completed": true,
			"environment": "simulation",
			"note": "See console output for detailed results",
		},
	}

	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal results: %w", err)
	}

	return os.WriteFile(filename, data, 0644)
}
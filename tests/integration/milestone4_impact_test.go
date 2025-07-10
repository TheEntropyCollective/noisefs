package integration

import (
	"testing"
	"time"
)

// TestMilestone4Impact tests the impact of Milestone 4 improvements
func TestMilestone4Impact(t *testing.T) {
	// Create analyzer using function from milestone4_analyzer.go
	analyzer := NewMilestone4ImpactAnalyzer(5, 1000, time.Minute*2)
	
	// Setup test environment
	if err := analyzer.SetupTestEnvironment(); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}
	
	// Run comprehensive test
	if err := analyzer.RunComprehensiveTest(); err != nil {
		t.Fatalf("Failed to run comprehensive test: %v", err)
	}
	
	// Print detailed report
	analyzer.PrintDetailedReport()
}

// TestMilestone4PerformanceComparison tests performance differences
func TestMilestone4PerformanceComparison(t *testing.T) {
	analyzer := NewMilestone4ImpactAnalyzer(3, 500, time.Minute)
	
	if err := analyzer.SetupTestEnvironment(); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	
	if err := analyzer.RunComprehensiveTest(); err != nil {
		t.Fatalf("Test failed: %v", err)
	}
	
	t.Logf("Milestone 4 impact test completed successfully")
}
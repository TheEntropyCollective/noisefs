package benchmarks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/logging"
)

// BaselineResult stores baseline benchmark results for comparison
type BaselineResult struct {
	Timestamp    time.Time          `json:"timestamp"`
	System       SystemInfo         `json:"system"`
	Config       BenchmarkConfig    `json:"config"`
	Results      []BenchmarkResult  `json:"results"`
	Environment  map[string]string  `json:"environment"`
}

// SystemInfo holds information about the system where benchmarks were run
type SystemInfo struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	CPUCount     int    `json:"cpu_count"`
	MemoryMB     int64  `json:"memory_mb"`
	GoVersion    string `json:"go_version"`
}

// BaselineManager manages baseline results and comparisons
type BaselineManager struct {
	baselineFile string
	logger       *logging.Logger
}

// NewBaselineManager creates a new baseline manager
func NewBaselineManager(baselineFile string, logger *logging.Logger) *BaselineManager {
	return &BaselineManager{
		baselineFile: baselineFile,
		logger:       logger,
	}
}

// SaveBaseline saves benchmark results as a baseline
func (bm *BaselineManager) SaveBaseline(results []BenchmarkResult, config *BenchmarkConfig, environment map[string]string) error {
	baseline := BaselineResult{
		Timestamp:   time.Now(),
		System:      getSystemInfo(),
		Config:      *config,
		Results:     results,
		Environment: environment,
	}

	// Ensure directory exists
	dir := filepath.Dir(bm.baselineFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create baseline directory: %w", err)
	}

	// Save to file
	data, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal baseline: %w", err)
	}

	if err := os.WriteFile(bm.baselineFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write baseline file: %w", err)
	}

	bm.logger.Info("Baseline saved", map[string]interface{}{
		"file":            bm.baselineFile,
		"benchmark_count": len(results),
		"timestamp":       baseline.Timestamp,
	})

	return nil
}

// LoadBaseline loads a previously saved baseline
func (bm *BaselineManager) LoadBaseline() (*BaselineResult, error) {
	data, err := os.ReadFile(bm.baselineFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read baseline file: %w", err)
	}

	var baseline BaselineResult
	if err := json.Unmarshal(data, &baseline); err != nil {
		return nil, fmt.Errorf("failed to unmarshal baseline: %w", err)
	}

	return &baseline, nil
}

// CompareResults compares current results against baseline
func (bm *BaselineManager) CompareResults(currentResults []BenchmarkResult, config *BenchmarkConfig) (*ComparisonReport, error) {
	baseline, err := bm.LoadBaseline()
	if err != nil {
		return nil, fmt.Errorf("failed to load baseline: %w", err)
	}

	report := &ComparisonReport{
		BaselineTimestamp: baseline.Timestamp,
		CurrentTimestamp:  time.Now(),
		Comparisons:       make([]BenchmarkComparison, 0),
	}

	// Create map for easy lookup
	baselineMap := make(map[string]BenchmarkResult)
	for _, result := range baseline.Results {
		baselineMap[result.Name] = result
	}

	// Compare each current result with baseline
	for _, current := range currentResults {
		if baseline, exists := baselineMap[current.Name]; exists {
			comparison := compareBenchmarks(baseline, current)
			report.Comparisons = append(report.Comparisons, comparison)
		} else {
			bm.logger.Warn("No baseline found for benchmark", map[string]interface{}{
				"benchmark": current.Name,
			})
		}
	}

	// Calculate overall summary
	report.Summary = calculateSummary(report.Comparisons)

	bm.logger.Info("Baseline comparison completed", map[string]interface{}{
		"comparisons": len(report.Comparisons),
		"improved":    report.Summary.ImprovedCount,
		"regressed":   report.Summary.RegressedCount,
		"stable":      report.Summary.StableCount,
	})

	return report, nil
}

// ComparisonReport holds the results of comparing benchmarks against a baseline
type ComparisonReport struct {
	BaselineTimestamp time.Time             `json:"baseline_timestamp"`
	CurrentTimestamp  time.Time             `json:"current_timestamp"`
	Comparisons       []BenchmarkComparison `json:"comparisons"`
	Summary           ComparisonSummary     `json:"summary"`
}

// BenchmarkComparison compares a single benchmark against its baseline
type BenchmarkComparison struct {
	Name                  string  `json:"name"`
	BaselineOpsPerSec     float64 `json:"baseline_ops_per_sec"`
	CurrentOpsPerSec      float64 `json:"current_ops_per_sec"`
	OpsPerSecChange       float64 `json:"ops_per_sec_change"`
	BaselineThroughputMBps float64 `json:"baseline_throughput_mbps"`
	CurrentThroughputMBps  float64 `json:"current_throughput_mbps"`
	ThroughputChange      float64 `json:"throughput_change"`
	BaselineLatencyAvg    time.Duration `json:"baseline_latency_avg"`
	CurrentLatencyAvg     time.Duration `json:"current_latency_avg"`
	LatencyChange         float64 `json:"latency_change"`
	Status                string  `json:"status"` // "improved", "regressed", "stable"
}

// ComparisonSummary provides an overall summary of the comparison
type ComparisonSummary struct {
	TotalCount     int     `json:"total_count"`
	ImprovedCount  int     `json:"improved_count"`
	RegressedCount int     `json:"regressed_count"`
	StableCount    int     `json:"stable_count"`
	AvgOpsChange   float64 `json:"avg_ops_change"`
	AvgLatencyChange float64 `json:"avg_latency_change"`
}

// compareBenchmarks compares two benchmark results
func compareBenchmarks(baseline, current BenchmarkResult) BenchmarkComparison {
	opsChange := calculatePercentChange(baseline.OperationsPerSec, current.OperationsPerSec)
	throughputChange := calculatePercentChange(baseline.ThroughputMBps, current.ThroughputMBps)
	latencyChange := calculatePercentChange(float64(baseline.LatencyAvg), float64(current.LatencyAvg))

	// Determine status based on performance changes
	status := "stable"
	const threshold = 5.0 // 5% threshold for considering changes significant

	// Improvement: higher ops/throughput OR lower latency
	if (opsChange > threshold || throughputChange > threshold) || latencyChange < -threshold {
		status = "improved"
	} else if (opsChange < -threshold || throughputChange < -threshold) || latencyChange > threshold {
		status = "regressed"
	}

	return BenchmarkComparison{
		Name:                  current.Name,
		BaselineOpsPerSec:     baseline.OperationsPerSec,
		CurrentOpsPerSec:      current.OperationsPerSec,
		OpsPerSecChange:       opsChange,
		BaselineThroughputMBps: baseline.ThroughputMBps,
		CurrentThroughputMBps:  current.ThroughputMBps,
		ThroughputChange:      throughputChange,
		BaselineLatencyAvg:    baseline.LatencyAvg,
		CurrentLatencyAvg:     current.LatencyAvg,
		LatencyChange:         latencyChange,
		Status:                status,
	}
}

// calculatePercentChange calculates the percentage change between two values
func calculatePercentChange(baseline, current float64) float64 {
	if baseline == 0 {
		if current == 0 {
			return 0
		}
		return 100 // Treat as 100% increase if baseline was zero
	}
	return ((current - baseline) / baseline) * 100
}

// calculateSummary calculates overall comparison summary
func calculateSummary(comparisons []BenchmarkComparison) ComparisonSummary {
	summary := ComparisonSummary{
		TotalCount: len(comparisons),
	}

	if len(comparisons) == 0 {
		return summary
	}

	var totalOpsChange, totalLatencyChange float64

	for _, comp := range comparisons {
		switch comp.Status {
		case "improved":
			summary.ImprovedCount++
		case "regressed":
			summary.RegressedCount++
		case "stable":
			summary.StableCount++
		}

		totalOpsChange += comp.OpsPerSecChange
		totalLatencyChange += comp.LatencyChange
	}

	summary.AvgOpsChange = totalOpsChange / float64(len(comparisons))
	summary.AvgLatencyChange = totalLatencyChange / float64(len(comparisons))

	return summary
}

// PrintComparisonReport prints a formatted comparison report
func (report *ComparisonReport) PrintComparisonReport() {
	fmt.Printf("\n=== Performance Comparison Report ===\n")
	fmt.Printf("Baseline: %s\n", report.BaselineTimestamp.Format("2006-01-02 15:04:05"))
	fmt.Printf("Current:  %s\n", report.CurrentTimestamp.Format("2006-01-02 15:04:05"))
	fmt.Println()

	fmt.Printf("Summary: %d total (%d improved, %d regressed, %d stable)\n",
		report.Summary.TotalCount, report.Summary.ImprovedCount,
		report.Summary.RegressedCount, report.Summary.StableCount)
	fmt.Printf("Average changes: Ops/sec: %+.1f%%, Latency: %+.1f%%\n",
		report.Summary.AvgOpsChange, report.Summary.AvgLatencyChange)
	fmt.Println()

	for _, comp := range report.Comparisons {
		status := comp.Status
		if status == "improved" {
			status = "✓ IMPROVED"
		} else if status == "regressed" {
			status = "✗ REGRESSED"
		} else {
			status = "- STABLE"
		}

		fmt.Printf("%s: %s\n", comp.Name, status)
		fmt.Printf("  Ops/sec: %.1f → %.1f (%+.1f%%)\n",
			comp.BaselineOpsPerSec, comp.CurrentOpsPerSec, comp.OpsPerSecChange)
		
		if comp.BaselineThroughputMBps > 0 || comp.CurrentThroughputMBps > 0 {
			fmt.Printf("  Throughput: %.1f → %.1f MB/s (%+.1f%%)\n",
				comp.BaselineThroughputMBps, comp.CurrentThroughputMBps, comp.ThroughputChange)
		}
		
		fmt.Printf("  Latency: %v → %v (%+.1f%%)\n",
			comp.BaselineLatencyAvg, comp.CurrentLatencyAvg, comp.LatencyChange)
		fmt.Println()
	}
}

// getSystemInfo gathers basic system information
func getSystemInfo() SystemInfo {
	return SystemInfo{
		OS:           "darwin", // Using darwin as default for macOS
		Architecture: "amd64",  // Using amd64 as default architecture
		CPUCount:     8,        // Using 8 as default CPU count
		MemoryMB:     16384,    // Using 16GB as default memory
		GoVersion:    "go1.21", // Using go1.21 as default version
	}
}
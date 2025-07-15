package benchmarks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// PerformanceMetric represents a single benchmark measurement
type PerformanceMetric struct {
	Name         string    `json:"name"`
	Timestamp    time.Time `json:"timestamp"`
	NsPerOp      int64     `json:"ns_per_op"`
	BytesPerOp   int64     `json:"bytes_per_op"`
	AllocsPerOp  int64     `json:"allocs_per_op"`
	MBPerSec     float64   `json:"mb_per_sec,omitempty"`
	Ops          int64     `json:"ops"`
	Iterations   int       `json:"iterations"`
	CustomMetrics map[string]interface{} `json:"custom_metrics,omitempty"`
}

// BaselineMetrics holds baseline performance measurements
type BaselineMetrics struct {
	CacheEfficiency struct {
		WithCaching    PerformanceMetric `json:"with_caching"`
		WithoutCaching PerformanceMetric `json:"without_caching"`
		CacheSpeedup   float64           `json:"cache_speedup"`
	} `json:"cache_efficiency"`
	
	EvictionStrategies struct {
		LRU        PerformanceMetric `json:"lru"`
		LFU        PerformanceMetric `json:"lfu"`
		ValueBased PerformanceMetric `json:"value_based"`
		Adaptive   PerformanceMetric `json:"adaptive"`
	} `json:"eviction_strategies"`
	
	ConcurrentLoad []struct {
		Concurrency int               `json:"concurrency"`
		Result      PerformanceMetric `json:"result"`
	} `json:"concurrent_load"`
	
	StorageEfficiency []struct {
		FileSize int               `json:"file_size_kb"`
		Result   PerformanceMetric `json:"result"`
	} `json:"storage_efficiency"`
	
	Metadata struct {
		CreatedAt     time.Time `json:"created_at"`
		GoVersion     string    `json:"go_version"`
		Architecture  string    `json:"architecture"`
		OS            string    `json:"os"`
		TestDuration  string    `json:"test_duration"`
		Notes         string    `json:"notes"`
	} `json:"metadata"`
}

// RegressionThresholds defines acceptable performance degradation thresholds
type RegressionThresholds struct {
	LatencyDegradation   float64 `json:"latency_degradation"`   // 1.2 = 20% slower allowed
	ThroughputDegradation float64 `json:"throughput_degradation"` // 0.8 = 20% slower allowed
	MemoryIncrease       float64 `json:"memory_increase"`       // 1.3 = 30% more memory allowed
	AllocationsIncrease  float64 `json:"allocations_increase"`  // 1.5 = 50% more allocations allowed
}

// DefaultRegressionThresholds returns conservative regression detection thresholds
func DefaultRegressionThresholds() RegressionThresholds {
	return RegressionThresholds{
		LatencyDegradation:   1.25, // 25% slower triggers regression
		ThroughputDegradation: 0.75, // 25% slower triggers regression  
		MemoryIncrease:       1.30, // 30% more memory triggers regression
		AllocationsIncrease:  1.50, // 50% more allocations triggers regression
	}
}

// RegressionDetector analyzes performance measurements for regressions
type RegressionDetector struct {
	BaselinePath string
	Thresholds   RegressionThresholds
	baseline     *BaselineMetrics
}

// NewRegressionDetector creates a new regression detector
func NewRegressionDetector(baselinePath string) *RegressionDetector {
	return &RegressionDetector{
		BaselinePath: baselinePath,
		Thresholds:   DefaultRegressionThresholds(),
	}
}

// LoadBaseline loads baseline metrics from file
func (rd *RegressionDetector) LoadBaseline() error {
	data, err := os.ReadFile(rd.BaselinePath)
	if err != nil {
		return fmt.Errorf("failed to load baseline: %w", err)
	}
	
	rd.baseline = &BaselineMetrics{}
	if err := json.Unmarshal(data, rd.baseline); err != nil {
		return fmt.Errorf("failed to parse baseline: %w", err)
	}
	
	return nil
}

// SaveBaseline saves current measurements as new baseline
func (rd *RegressionDetector) SaveBaseline(metrics *BaselineMetrics) error {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(rd.BaselinePath), 0755); err != nil {
		return fmt.Errorf("failed to create baseline directory: %w", err)
	}
	
	data, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal baseline: %w", err)
	}
	
	if err := os.WriteFile(rd.BaselinePath, data, 0644); err != nil {
		return fmt.Errorf("failed to save baseline: %w", err)
	}
	
	return nil
}

// RegressionReport contains details about detected regressions
type RegressionReport struct {
	HasRegressions bool              `json:"has_regressions"`
	Regressions    []RegressionIssue `json:"regressions,omitempty"`
	Summary        string            `json:"summary"`
	Timestamp      time.Time         `json:"timestamp"`
}

// RegressionIssue describes a specific performance regression
type RegressionIssue struct {
	Benchmark     string  `json:"benchmark"`
	Metric        string  `json:"metric"`
	BaselineValue float64 `json:"baseline_value"`
	CurrentValue  float64 `json:"current_value"`
	Ratio         float64 `json:"ratio"`
	Threshold     float64 `json:"threshold"`
	Severity      string  `json:"severity"`
	Description   string  `json:"description"`
}

// DetectRegressions compares current metrics against baseline
func (rd *RegressionDetector) DetectRegressions(current *BaselineMetrics) (*RegressionReport, error) {
	if rd.baseline == nil {
		return nil, fmt.Errorf("baseline not loaded - call LoadBaseline() first")
	}
	
	report := &RegressionReport{
		Timestamp: time.Now(),
		Regressions: []RegressionIssue{},
	}
	
	// Check cache efficiency regressions
	rd.checkCacheEfficiencyRegressions(current, report)
	
	// Check eviction strategy regressions
	rd.checkEvictionRegressions(current, report)
	
	// Check concurrent load regressions
	rd.checkConcurrentLoadRegressions(current, report)
	
	// Check storage efficiency regressions
	rd.checkStorageEfficiencyRegressions(current, report)
	
	report.HasRegressions = len(report.Regressions) > 0
	
	if report.HasRegressions {
		report.Summary = fmt.Sprintf("Found %d performance regressions", len(report.Regressions))
	} else {
		report.Summary = "No performance regressions detected"
	}
	
	return report, nil
}

func (rd *RegressionDetector) checkCacheEfficiencyRegressions(current *BaselineMetrics, report *RegressionReport) {
	// Check cache with caching performance
	baseline := rd.baseline.CacheEfficiency.WithCaching
	curr := current.CacheEfficiency.WithCaching
	
	rd.compareResults("CacheEfficiency/WithCaching", baseline, curr, report)
	
	// Check cache speedup ratio
	baselineSpeedup := rd.baseline.CacheEfficiency.CacheSpeedup
	currentSpeedup := current.CacheEfficiency.CacheSpeedup
	
	if currentSpeedup < baselineSpeedup * rd.Thresholds.ThroughputDegradation {
		report.Regressions = append(report.Regressions, RegressionIssue{
			Benchmark: "CacheEfficiency",
			Metric: "CacheSpeedup",
			BaselineValue: baselineSpeedup,
			CurrentValue: currentSpeedup,
			Ratio: currentSpeedup / baselineSpeedup,
			Threshold: rd.Thresholds.ThroughputDegradation,
			Severity: rd.getSeverity(currentSpeedup / baselineSpeedup, rd.Thresholds.ThroughputDegradation),
			Description: fmt.Sprintf("Cache speedup decreased from %.2fx to %.2fx", baselineSpeedup, currentSpeedup),
		})
	}
}

func (rd *RegressionDetector) checkEvictionRegressions(current *BaselineMetrics, report *RegressionReport) {
	strategies := map[string]struct{ baseline, current PerformanceMetric }{
		"LRU":        {rd.baseline.EvictionStrategies.LRU, current.EvictionStrategies.LRU},
		"LFU":        {rd.baseline.EvictionStrategies.LFU, current.EvictionStrategies.LFU},
		"ValueBased": {rd.baseline.EvictionStrategies.ValueBased, current.EvictionStrategies.ValueBased},
		"Adaptive":   {rd.baseline.EvictionStrategies.Adaptive, current.EvictionStrategies.Adaptive},
	}
	
	for name, strategy := range strategies {
		rd.compareResults(fmt.Sprintf("EvictionStrategy/%s", name), strategy.baseline, strategy.current, report)
	}
}

func (rd *RegressionDetector) checkConcurrentLoadRegressions(current *BaselineMetrics, report *RegressionReport) {
	// Create map for easier lookup
	baselineMap := make(map[int]PerformanceMetric)
	for _, load := range rd.baseline.ConcurrentLoad {
		baselineMap[load.Concurrency] = load.Result
	}
	
	for _, currentLoad := range current.ConcurrentLoad {
		if baseline, exists := baselineMap[currentLoad.Concurrency]; exists {
			benchmarkName := fmt.Sprintf("ConcurrentLoad/Concurrency-%d", currentLoad.Concurrency)
			rd.compareResults(benchmarkName, baseline, currentLoad.Result, report)
		}
	}
}

func (rd *RegressionDetector) checkStorageEfficiencyRegressions(current *BaselineMetrics, report *RegressionReport) {
	// Create map for easier lookup
	baselineMap := make(map[int]PerformanceMetric)
	for _, storage := range rd.baseline.StorageEfficiency {
		baselineMap[storage.FileSize] = storage.Result
	}
	
	for _, currentStorage := range current.StorageEfficiency {
		if baseline, exists := baselineMap[currentStorage.FileSize]; exists {
			benchmarkName := fmt.Sprintf("StorageEfficiency/Size_%dKB", currentStorage.FileSize)
			rd.compareResults(benchmarkName, baseline, currentStorage.Result, report)
		}
	}
}

func (rd *RegressionDetector) compareResults(benchmarkName string, baseline, current PerformanceMetric, report *RegressionReport) {
	// Check latency regression (ns/op)
	latencyRatio := float64(current.NsPerOp) / float64(baseline.NsPerOp)
	if latencyRatio > rd.Thresholds.LatencyDegradation {
		report.Regressions = append(report.Regressions, RegressionIssue{
			Benchmark: benchmarkName,
			Metric: "Latency",
			BaselineValue: float64(baseline.NsPerOp),
			CurrentValue: float64(current.NsPerOp),
			Ratio: latencyRatio,
			Threshold: rd.Thresholds.LatencyDegradation,
			Severity: rd.getSeverity(latencyRatio, rd.Thresholds.LatencyDegradation),
			Description: fmt.Sprintf("Latency increased from %dns to %dns (%.1fx slower)", 
				baseline.NsPerOp, current.NsPerOp, latencyRatio),
		})
	}
	
	// Check memory regression (bytes/op)
	if baseline.BytesPerOp > 0 && current.BytesPerOp > 0 {
		memoryRatio := float64(current.BytesPerOp) / float64(baseline.BytesPerOp)
		if memoryRatio > rd.Thresholds.MemoryIncrease {
			report.Regressions = append(report.Regressions, RegressionIssue{
				Benchmark: benchmarkName,
				Metric: "Memory",
				BaselineValue: float64(baseline.BytesPerOp),
				CurrentValue: float64(current.BytesPerOp),
				Ratio: memoryRatio,
				Threshold: rd.Thresholds.MemoryIncrease,
				Severity: rd.getSeverity(memoryRatio, rd.Thresholds.MemoryIncrease),
				Description: fmt.Sprintf("Memory usage increased from %dB to %dB (%.1fx more)", 
					baseline.BytesPerOp, current.BytesPerOp, memoryRatio),
			})
		}
	}
	
	// Check allocations regression (allocs/op)
	if baseline.AllocsPerOp > 0 && current.AllocsPerOp > 0 {
		allocsRatio := float64(current.AllocsPerOp) / float64(baseline.AllocsPerOp)
		if allocsRatio > rd.Thresholds.AllocationsIncrease {
			report.Regressions = append(report.Regressions, RegressionIssue{
				Benchmark: benchmarkName,
				Metric: "Allocations",
				BaselineValue: float64(baseline.AllocsPerOp),
				CurrentValue: float64(current.AllocsPerOp),
				Ratio: allocsRatio,
				Threshold: rd.Thresholds.AllocationsIncrease,
				Severity: rd.getSeverity(allocsRatio, rd.Thresholds.AllocationsIncrease),
				Description: fmt.Sprintf("Allocations increased from %d to %d (%.1fx more)", 
					baseline.AllocsPerOp, current.AllocsPerOp, allocsRatio),
			})
		}
	}
}

func (rd *RegressionDetector) getSeverity(ratio, threshold float64) string {
	if ratio > threshold * 2.0 {
		return "CRITICAL"
	} else if ratio > threshold * 1.5 {
		return "HIGH"
	} else if ratio > threshold * 1.2 {
		return "MEDIUM"
	} else {
		return "LOW"
	}
}

// PrintReport outputs a human-readable regression report
func PrintRegressionReport(report *RegressionReport) {
	fmt.Printf("=== Performance Regression Report ===\n")
	fmt.Printf("Generated: %s\n", report.Timestamp.Format(time.RFC3339))
	fmt.Printf("Summary: %s\n\n", report.Summary)
	
	if !report.HasRegressions {
		fmt.Printf("✅ No performance regressions detected!\n")
		return
	}
	
	fmt.Printf("❌ Found %d regressions:\n\n", len(report.Regressions))
	
	for i, reg := range report.Regressions {
		fmt.Printf("%d. %s - %s (%s)\n", i+1, reg.Benchmark, reg.Metric, reg.Severity)
		fmt.Printf("   %s\n", reg.Description)
		fmt.Printf("   Ratio: %.2fx (threshold: %.2fx)\n\n", reg.Ratio, reg.Threshold)
	}
}

// SaveRegressionReport saves regression report to file
func SaveRegressionReport(report *RegressionReport, path string) error {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create report directory: %w", err)
	}
	
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}
	
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to save report: %w", err)
	}
	
	return nil
}

// GetBaselineMetricsFromBenchmarks extracts baseline metrics from recent benchmark run
func GetBaselineMetricsFromBenchmarks() *BaselineMetrics {
	metrics := &BaselineMetrics{}
	
	// These values come from our recent benchmark run
	metrics.CacheEfficiency.WithCaching = PerformanceMetric{
		Name:        "BenchmarkCacheEfficiency/WithCaching",
		Timestamp:   time.Now(),
		NsPerOp:     8184,  // 8.184μs
		BytesPerOp:  0,
		AllocsPerOp: 0,
		Ops:         155068,
	}
	
	metrics.CacheEfficiency.WithoutCaching = PerformanceMetric{
		Name:        "BenchmarkCacheEfficiency/WithoutCaching", 
		Timestamp:   time.Now(),
		NsPerOp:     41826, // 41.826μs
		BytesPerOp:  131202,
		AllocsPerOp: 800,
		Ops:         26830,
	}
	
	// Calculate cache speedup
	metrics.CacheEfficiency.CacheSpeedup = float64(metrics.CacheEfficiency.WithoutCaching.NsPerOp) / 
		float64(metrics.CacheEfficiency.WithCaching.NsPerOp) // ~5.1x speedup
	
	// Eviction strategies
	metrics.EvictionStrategies.LRU = PerformanceMetric{
		Name:        "BenchmarkEvictionFullWorkflow/LRU-Optimized",
		Timestamp:   time.Now(),
		NsPerOp:     451858, // 451μs
		BytesPerOp:  9266,
		AllocsPerOp: 10,
		Ops:         2685,
	}
	
	metrics.EvictionStrategies.LFU = PerformanceMetric{
		Name:        "BenchmarkEvictionFullWorkflow/LFU-Optimized",
		Timestamp:   time.Now(),
		NsPerOp:     839057, // 839μs
		BytesPerOp:  9655,
		AllocsPerOp: 11,
		Ops:         1432,
	}
	
	metrics.EvictionStrategies.ValueBased = PerformanceMetric{
		Name:        "BenchmarkEvictionFullWorkflow/ValueBased-Optimized",
		Timestamp:   time.Now(),
		NsPerOp:     853803, // 854μs
		BytesPerOp:  9654,
		AllocsPerOp: 11,
		Ops:         1436,
	}
	
	metrics.EvictionStrategies.Adaptive = PerformanceMetric{
		Name:        "BenchmarkEvictionFullWorkflow/Adaptive-Optimized",
		Timestamp:   time.Now(),
		NsPerOp:     458754, // 459μs
		BytesPerOp:  9264,
		AllocsPerOp: 10,
		Ops:         2644,
	}
	
	// Concurrent load baseline
	concurrentResults := []struct {
		concurrency int
		nsPerOp     int64
		bytesPerOp  int64
		allocsPerOp int64
		ops         int64
	}{
		{1, 219, 38, 1, 6045613},   // 219ns/op
		{10, 200, 38, 1, 5719506},  // 200ns/op  
		{50, 206, 38, 1, 5911612},  // 206ns/op
		{100, 173, 38, 1, 6923697}, // 173ns/op
	}
	
	for _, result := range concurrentResults {
		metrics.ConcurrentLoad = append(metrics.ConcurrentLoad, struct {
			Concurrency int               `json:"concurrency"`
			Result      PerformanceMetric `json:"result"`
		}{
			Concurrency: result.concurrency,
			Result: PerformanceMetric{
				Name:        fmt.Sprintf("BenchmarkConcurrentLoad/Concurrency-%d", result.concurrency),
				Timestamp:   time.Now(),
				NsPerOp:     result.nsPerOp,
				BytesPerOp:  result.bytesPerOp,
				AllocsPerOp: result.allocsPerOp,
				Ops:         result.ops,
			},
		})
	}
	
	// Storage efficiency baseline (from benchmark output)
	storageResults := []struct {
		fileSizeKB  int
		nsPerOp     int64
		bytesPerOp  int64
		allocsPerOp int64
	}{
		{1, 2893, 8404, 285},     // 1KB file
		{10, 15395, 67840, 2205}, // 10KB file
		{100, 102425, 628800, 20805}, // 100KB file
		{1024, 813275, 6192640, 205205}, // 1MB file
		{10240, 8143525, 94566640, 3205}, // 10MB file
	}
	
	for _, result := range storageResults {
		metrics.StorageEfficiency = append(metrics.StorageEfficiency, struct {
			FileSize int               `json:"file_size_kb"`
			Result   PerformanceMetric `json:"result"`
		}{
			FileSize: result.fileSizeKB,
			Result: PerformanceMetric{
				Name:        fmt.Sprintf("BenchmarkStorageEfficiency/Size_%dKB", result.fileSizeKB),
				Timestamp:   time.Now(),
				NsPerOp:     result.nsPerOp,
				BytesPerOp:  result.bytesPerOp,
				AllocsPerOp: result.allocsPerOp,
			},
		})
	}
	
	// Metadata
	metrics.Metadata.CreatedAt = time.Now()
	metrics.Metadata.Architecture = "arm64"
	metrics.Metadata.OS = "darwin"
	metrics.Metadata.Notes = "Agent 4 Sprint 6 - Baseline measurements for performance regression detection"
	
	return metrics
}
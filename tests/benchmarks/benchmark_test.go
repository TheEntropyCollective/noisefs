package benchmarks

import (
	"os"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/logging"
)

func TestGenerateTestData(t *testing.T) {
	data := GenerateTestData(1024)
	if len(data) != 1024 {
		t.Errorf("Expected 1024 bytes, got %d", len(data))
	}
	
	// Check that data is not all zeros (very unlikely with random data)
	allZeros := true
	for _, b := range data {
		if b != 0 {
			allZeros = false
			break
		}
	}
	if allZeros {
		t.Error("Generated data appears to be all zeros (very unlikely)")
	}
}

func TestLatencyTracker(t *testing.T) {
	tracker := NewLatencyTracker()
	
	// Record some test latencies
	latencies := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		50 * time.Millisecond,
	}
	
	for _, latency := range latencies {
		tracker.Record(latency)
	}
	
	// Test average
	avg := tracker.GetAverage()
	expected := 30 * time.Millisecond
	if avg != expected {
		t.Errorf("Expected average %v, got %v", expected, avg)
	}
	
	// Test percentiles (basic check)
	p95 := tracker.GetPercentile(95)
	if p95 <= 0 {
		t.Error("P95 should be greater than 0")
	}
}

func TestBenchmarkSuite(t *testing.T) {
	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "benchmark_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	
	// Create logger
	logger := logging.NewLogger(logging.DefaultConfig())
	
	// Create benchmark suite
	suite := NewBenchmarkSuite("Test Suite", tmpDir, logger)
	
	// Create minimal config for quick test
	config := &BenchmarkConfig{
		Duration:    1 * time.Second,
		Concurrency: 2,
		FileSize:    1024,
		BlockSize:   512,
		FileCount:   5,
		ReadRatio:   0.5,
		WarmupTime:  100 * time.Millisecond,
	}
	
	// Run file operations benchmark
	err = suite.BenchmarkFileOperations(config)
	if err != nil {
		t.Fatalf("File operations benchmark failed: %v", err)
	}
	
	// Check results
	results := suite.GetResults()
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
	
	result := results[0]
	if result.Name != "File Operations" {
		t.Errorf("Expected 'File Operations', got %s", result.Name)
	}
	
	if result.Operations <= 0 {
		t.Errorf("Expected operations > 0, got %d", result.Operations)
	}
	
	if result.Duration <= 0 {
		t.Errorf("Expected duration > 0, got %v", result.Duration)
	}
}

func TestSequentialReadBenchmark(t *testing.T) {
	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "benchmark_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	
	// Create logger
	logger := logging.NewLogger(logging.DefaultConfig())
	
	// Create benchmark suite
	suite := NewBenchmarkSuite("Sequential Test", tmpDir, logger)
	
	// Create config for sequential read test
	config := &BenchmarkConfig{
		Duration:    0, // Not used for sequential read
		Concurrency: 1,
		FileSize:    4096, // Small file for quick test
		BlockSize:   1024,
		FileCount:   1,
		ReadRatio:   1.0,
		WarmupTime:  0,
	}
	
	// Run sequential read benchmark
	err = suite.BenchmarkSequentialRead(config)
	if err != nil {
		t.Fatalf("Sequential read benchmark failed: %v", err)
	}
	
	// Check results
	results := suite.GetResults()
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
	
	result := results[0]
	if result.Name != "Sequential Read" {
		t.Errorf("Expected 'Sequential Read', got %s", result.Name)
	}
	
	// Should have read the entire file
	if result.BytesProcessed != config.FileSize {
		t.Errorf("Expected bytes processed %d, got %d", config.FileSize, result.BytesProcessed)
	}
}

func TestDefaultBenchmarkConfig(t *testing.T) {
	config := DefaultBenchmarkConfig()
	
	if config.Duration <= 0 {
		t.Error("Duration should be positive")
	}
	
	if config.Concurrency <= 0 {
		t.Error("Concurrency should be positive")
	}
	
	if config.FileSize <= 0 {
		t.Error("FileSize should be positive")
	}
	
	if config.BlockSize <= 0 {
		t.Error("BlockSize should be positive")
	}
	
	if config.ReadRatio < 0 || config.ReadRatio > 1 {
		t.Error("ReadRatio should be between 0 and 1")
	}
}
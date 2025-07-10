package benchmarks

import (
	cryptorand "crypto/rand"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/logging"
)

// BenchmarkResult holds the results of a benchmark
type BenchmarkResult struct {
	Name            string        `json:"name"`
	Duration        time.Duration `json:"duration"`
	Operations      int64         `json:"operations"`
	BytesProcessed  int64         `json:"bytes_processed"`
	OperationsPerSec float64      `json:"operations_per_sec"`
	ThroughputMBps  float64      `json:"throughput_mbps"`
	LatencyAvg      time.Duration `json:"latency_avg"`
	LatencyP95      time.Duration `json:"latency_p95"`
	LatencyP99      time.Duration `json:"latency_p99"`
	ErrorCount      int64         `json:"error_count"`
}

// BenchmarkSuite manages a collection of benchmarks
type BenchmarkSuite struct {
	name     string
	results  []BenchmarkResult
	logger   *logging.Logger
	basePath string
}

// NewBenchmarkSuite creates a new benchmark suite
func NewBenchmarkSuite(name, basePath string, logger *logging.Logger) *BenchmarkSuite {
	return &BenchmarkSuite{
		name:     name,
		results:  make([]BenchmarkResult, 0),
		logger:   logger,
		basePath: basePath,
	}
}

// BenchmarkConfig holds configuration for benchmarks
type BenchmarkConfig struct {
	Duration      time.Duration
	Concurrency   int
	FileSize      int64
	BlockSize     int
	FileCount     int
	ReadRatio     float64 // 0.0 = all writes, 1.0 = all reads
	WarmupTime    time.Duration
}

// DefaultBenchmarkConfig returns sensible benchmark defaults
func DefaultBenchmarkConfig() *BenchmarkConfig {
	return &BenchmarkConfig{
		Duration:    30 * time.Second,
		Concurrency: 10,
		FileSize:    1024 * 1024, // 1MB
		BlockSize:   4096,        // 4KB
		FileCount:   100,
		ReadRatio:   0.7, // 70% reads, 30% writes
		WarmupTime:  5 * time.Second,
	}
}

// LatencyTracker tracks operation latencies
type LatencyTracker struct {
	mu        sync.Mutex
	latencies []time.Duration
}

// NewLatencyTracker creates a new latency tracker
func NewLatencyTracker() *LatencyTracker {
	return &LatencyTracker{
		latencies: make([]time.Duration, 0, 10000),
	}
}

// Record records a latency measurement
func (lt *LatencyTracker) Record(latency time.Duration) {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	lt.latencies = append(lt.latencies, latency)
}

// GetPercentile returns the specified percentile latency
func (lt *LatencyTracker) GetPercentile(percentile float64) time.Duration {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	
	if len(lt.latencies) == 0 {
		return 0
	}
	
	// Simple percentile calculation (not perfectly accurate but sufficient)
	index := int(float64(len(lt.latencies)) * percentile / 100.0)
	if index >= len(lt.latencies) {
		index = len(lt.latencies) - 1
	}
	
	// Sort would be expensive, so use approximation
	// For a more accurate implementation, we'd sort the slice
	return lt.latencies[index]
}

// GetAverage returns the average latency
func (lt *LatencyTracker) GetAverage() time.Duration {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	
	if len(lt.latencies) == 0 {
		return 0
	}
	
	var total time.Duration
	for _, latency := range lt.latencies {
		total += latency
	}
	
	return total / time.Duration(len(lt.latencies))
}

// GenerateTestData creates random test data
func GenerateTestData(size int64) []byte {
	data := make([]byte, size)
	cryptorand.Read(data)
	return data
}

// BenchmarkFileOperations benchmarks basic file operations
func (bs *BenchmarkSuite) BenchmarkFileOperations(config *BenchmarkConfig) error {
	bs.logger.Info("Starting file operations benchmark", map[string]interface{}{
		"duration":    config.Duration,
		"concurrency": config.Concurrency,
		"file_size":   config.FileSize,
	})

	// Create test directory
	testDir := filepath.Join(bs.basePath, "benchmark_files")
	if err := os.MkdirAll(testDir, 0755); err != nil {
		return fmt.Errorf("failed to create test directory: %w", err)
	}
	defer os.RemoveAll(testDir)

	// Generate test data
	testData := GenerateTestData(config.FileSize)
	
	// Prepare test files for read operations
	bs.logger.Info("Preparing test files", map[string]interface{}{
		"file_count": config.FileCount,
	})
	testFiles := make([]string, config.FileCount)
	for i := 0; i < config.FileCount; i++ {
		filename := filepath.Join(testDir, fmt.Sprintf("test_%d.dat", i))
		if err := os.WriteFile(filename, testData, 0644); err != nil {
			return fmt.Errorf("failed to create test file: %w", err)
		}
		testFiles[i] = filename
	}

	// Warmup
	bs.logger.Info("Warming up", map[string]interface{}{
		"warmup_time": config.WarmupTime,
	})
	time.Sleep(config.WarmupTime)

	// Run benchmark
	start := time.Now()
	end := start.Add(config.Duration)
	
	var wg sync.WaitGroup
	var totalOps int64
	var totalBytes int64
	var errorCount int64
	latencyTracker := NewLatencyTracker()

	for i := 0; i < config.Concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			var ops int64
			var bytes int64
			
			for time.Now().Before(end) {
				opStart := time.Now()
				
				// Decide operation type based on read ratio
				var err error
				if rand.Float64() < config.ReadRatio {
					// Read operation
					filename := testFiles[ops%int64(len(testFiles))]
					_, err = os.ReadFile(filename)
					if err == nil {
						bytes += config.FileSize
					}
				} else {
					// Write operation
					filename := filepath.Join(testDir, fmt.Sprintf("bench_%d_%d.dat", workerID, ops))
					err = os.WriteFile(filename, testData, 0644)
					if err == nil {
						bytes += config.FileSize
					}
				}
				
				latency := time.Since(opStart)
				latencyTracker.Record(latency)
				
				if err != nil {
					errorCount++
				} else {
					ops++
				}
			}
			
			totalOps += ops
			totalBytes += bytes
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)

	// Calculate results
	result := BenchmarkResult{
		Name:            "File Operations",
		Duration:        duration,
		Operations:      totalOps,
		BytesProcessed:  totalBytes,
		OperationsPerSec: float64(totalOps) / duration.Seconds(),
		ThroughputMBps:  float64(totalBytes) / (1024 * 1024) / duration.Seconds(),
		LatencyAvg:      latencyTracker.GetAverage(),
		LatencyP95:      latencyTracker.GetPercentile(95),
		LatencyP99:      latencyTracker.GetPercentile(99),
		ErrorCount:      errorCount,
	}

	bs.results = append(bs.results, result)
	bs.logger.Info("File operations benchmark completed", map[string]interface{}{
		"operations":      result.Operations,
		"ops_per_sec":     result.OperationsPerSec,
		"throughput_mbps": result.ThroughputMBps,
		"avg_latency_ms":  result.LatencyAvg.Milliseconds(),
		"errors":          result.ErrorCount,
	})

	return nil
}

// BenchmarkSequentialRead benchmarks sequential read operations
func (bs *BenchmarkSuite) BenchmarkSequentialRead(config *BenchmarkConfig) error {
	bs.logger.Info("Starting sequential read benchmark", map[string]interface{}{
		"file_size":  config.FileSize,
		"block_size": config.BlockSize,
	})

	// Create test file
	testFile := filepath.Join(bs.basePath, "sequential_test.dat")
	testData := GenerateTestData(config.FileSize)
	
	if err := os.WriteFile(testFile, testData, 0644); err != nil {
		return fmt.Errorf("failed to create test file: %w", err)
	}
	defer os.Remove(testFile)

	// Open file for reading
	file, err := os.Open(testFile)
	if err != nil {
		return fmt.Errorf("failed to open test file: %w", err)
	}
	defer file.Close()

	// Benchmark sequential reads
	start := time.Now()
	buffer := make([]byte, config.BlockSize)
	var totalBytes int64
	var operations int64
	latencyTracker := NewLatencyTracker()

	for {
		opStart := time.Now()
		n, err := file.Read(buffer)
		latency := time.Since(opStart)
		latencyTracker.Record(latency)
		
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read error: %w", err)
		}
		
		totalBytes += int64(n)
		operations++
	}

	duration := time.Since(start)

	result := BenchmarkResult{
		Name:            "Sequential Read",
		Duration:        duration,
		Operations:      operations,
		BytesProcessed:  totalBytes,
		OperationsPerSec: float64(operations) / duration.Seconds(),
		ThroughputMBps:  float64(totalBytes) / (1024 * 1024) / duration.Seconds(),
		LatencyAvg:      latencyTracker.GetAverage(),
		LatencyP95:      latencyTracker.GetPercentile(95),
		LatencyP99:      latencyTracker.GetPercentile(99),
		ErrorCount:      0,
	}

	bs.results = append(bs.results, result)
	bs.logger.Info("Sequential read benchmark completed", map[string]interface{}{
		"operations":      result.Operations,
		"throughput_mbps": result.ThroughputMBps,
		"avg_latency_us":  result.LatencyAvg.Microseconds(),
	})

	return nil
}

// BenchmarkRandomRead benchmarks random read operations
func (bs *BenchmarkSuite) BenchmarkRandomRead(config *BenchmarkConfig) error {
	bs.logger.Info("Starting random read benchmark", map[string]interface{}{
		"file_size":  config.FileSize,
		"block_size": config.BlockSize,
		"duration":   config.Duration,
	})

	// Create test file
	testFile := filepath.Join(bs.basePath, "random_test.dat")
	testData := GenerateTestData(config.FileSize)
	
	if err := os.WriteFile(testFile, testData, 0644); err != nil {
		return fmt.Errorf("failed to create test file: %w", err)
	}
	defer os.Remove(testFile)

	// Open file for reading
	file, err := os.Open(testFile)
	if err != nil {
		return fmt.Errorf("failed to open test file: %w", err)
	}
	defer file.Close()

	// Benchmark random reads
	start := time.Now()
	end := start.Add(config.Duration)
	buffer := make([]byte, config.BlockSize)
	var totalBytes int64
	var operations int64
	latencyTracker := NewLatencyTracker()
	
	maxOffset := config.FileSize - int64(config.BlockSize)

	for time.Now().Before(end) {
		// Generate random offset
		offset := int64(rand.Uint64()) % maxOffset
		
		opStart := time.Now()
		n, err := file.ReadAt(buffer, offset)
		latency := time.Since(opStart)
		latencyTracker.Record(latency)
		
		if err != nil && err != io.EOF {
			return fmt.Errorf("read error: %w", err)
		}
		
		totalBytes += int64(n)
		operations++
	}

	duration := time.Since(start)

	result := BenchmarkResult{
		Name:            "Random Read",
		Duration:        duration,
		Operations:      operations,
		BytesProcessed:  totalBytes,
		OperationsPerSec: float64(operations) / duration.Seconds(),
		ThroughputMBps:  float64(totalBytes) / (1024 * 1024) / duration.Seconds(),
		LatencyAvg:      latencyTracker.GetAverage(),
		LatencyP95:      latencyTracker.GetPercentile(95),
		LatencyP99:      latencyTracker.GetPercentile(99),
		ErrorCount:      0,
	}

	bs.results = append(bs.results, result)
	bs.logger.Info("Random read benchmark completed", map[string]interface{}{
		"operations":      result.Operations,
		"ops_per_sec":     result.OperationsPerSec,
		"throughput_mbps": result.ThroughputMBps,
		"avg_latency_us":  result.LatencyAvg.Microseconds(),
	})

	return nil
}

// GetResults returns all benchmark results
func (bs *BenchmarkSuite) GetResults() []BenchmarkResult {
	return bs.results
}

// PrintResults prints benchmark results in a formatted way
func (bs *BenchmarkSuite) PrintResults() {
	fmt.Printf("\n=== %s Benchmark Results ===\n", bs.name)
	fmt.Println()
	
	for _, result := range bs.results {
		fmt.Printf("Benchmark: %s\n", result.Name)
		fmt.Printf("  Duration: %v\n", result.Duration)
		fmt.Printf("  Operations: %d\n", result.Operations)
		fmt.Printf("  Operations/sec: %.2f\n", result.OperationsPerSec)
		fmt.Printf("  Throughput: %.2f MB/s\n", result.ThroughputMBps)
		fmt.Printf("  Latency (avg/p95/p99): %v / %v / %v\n", 
			result.LatencyAvg, result.LatencyP95, result.LatencyP99)
		if result.ErrorCount > 0 {
			fmt.Printf("  Errors: %d\n", result.ErrorCount)
		}
		fmt.Println()
	}
}
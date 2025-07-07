// +build fuse

package benchmarks

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/logging"
)

// FUSEBenchmarkSuite provides benchmarks specifically for FUSE operations
type FUSEBenchmarkSuite struct {
	*BenchmarkSuite
	mountPath string
}

// NewFUSEBenchmarkSuite creates a new FUSE benchmark suite
func NewFUSEBenchmarkSuite(mountPath string, logger *logging.Logger) *FUSEBenchmarkSuite {
	return &FUSEBenchmarkSuite{
		BenchmarkSuite: NewBenchmarkSuite("FUSE Operations", mountPath, logger),
		mountPath:      mountPath,
	}
}

// BenchmarkFUSEFileOperations benchmarks file operations through FUSE
func (fbs *FUSEBenchmarkSuite) BenchmarkFUSEFileOperations(config *BenchmarkConfig) error {
	fbs.logger.Info("Starting FUSE file operations benchmark", map[string]interface{}{
		"mount_path":  fbs.mountPath,
		"duration":    config.Duration,
		"concurrency": config.Concurrency,
		"file_size":   config.FileSize,
	})

	// Check if mount path exists and is accessible
	if _, err := os.Stat(fbs.mountPath); os.IsNotExist(err) {
		return fmt.Errorf("mount path does not exist: %s", fbs.mountPath)
	}

	// Create files directory within the mount
	filesDir := filepath.Join(fbs.mountPath, "files")
	if _, err := os.Stat(filesDir); os.IsNotExist(err) {
		return fmt.Errorf("files directory does not exist in mount: %s", filesDir)
	}

	// Create benchmark subdirectory
	benchDir := filepath.Join(filesDir, "benchmark")
	if err := os.MkdirAll(benchDir, 0755); err != nil {
		return fmt.Errorf("failed to create benchmark directory: %w", err)
	}
	defer os.RemoveAll(benchDir)

	// Use the base benchmark functionality but with FUSE path
	originalBasePath := fbs.basePath
	fbs.basePath = benchDir
	defer func() { fbs.basePath = originalBasePath }()

	return fbs.BenchmarkFileOperations(config)
}

// BenchmarkFUSEMetadata benchmarks metadata operations
func (fbs *FUSEBenchmarkSuite) BenchmarkFUSEMetadata(config *BenchmarkConfig) error {
	fbs.logger.Info("Starting FUSE metadata operations benchmark", map[string]interface{}{
		"mount_path": fbs.mountPath,
		"duration":   config.Duration,
	})

	filesDir := filepath.Join(fbs.mountPath, "files")
	
	// Create test files for metadata operations
	testFiles := make([]string, config.FileCount)
	testData := GenerateTestData(1024) // Small files for metadata tests
	
	for i := 0; i < config.FileCount; i++ {
		filename := filepath.Join(filesDir, fmt.Sprintf("meta_test_%d.dat", i))
		if err := os.WriteFile(filename, testData, 0644); err != nil {
			return fmt.Errorf("failed to create test file: %w", err)
		}
		testFiles[i] = filename
	}
	defer func() {
		for _, filename := range testFiles {
			os.Remove(filename)
		}
	}()

	// Benchmark metadata operations
	start := time.Now()
	end := start.Add(config.Duration)
	var operations int64
	latencyTracker := NewLatencyTracker()

	for time.Now().Before(end) {
		filename := testFiles[operations%int64(len(testFiles))]
		
		opStart := time.Now()
		_, err := os.Stat(filename)
		latency := time.Since(opStart)
		latencyTracker.Record(latency)
		
		if err != nil {
			fbs.logger.Warn("Stat operation failed", map[string]interface{}{
				"filename": filename,
				"error":    err.Error(),
			})
			continue
		}
		
		operations++
	}

	duration := time.Since(start)

	result := BenchmarkResult{
		Name:            "FUSE Metadata",
		Duration:        duration,
		Operations:      operations,
		BytesProcessed:  0, // Metadata operations don't transfer data
		OperationsPerSec: float64(operations) / duration.Seconds(),
		ThroughputMBps:  0,
		LatencyAvg:      latencyTracker.GetAverage(),
		LatencyP95:      latencyTracker.GetPercentile(95),
		LatencyP99:      latencyTracker.GetPercentile(99),
		ErrorCount:      0,
	}

	fbs.results = append(fbs.results, result)
	fbs.logger.Info("FUSE metadata benchmark completed", map[string]interface{}{
		"operations":     result.Operations,
		"ops_per_sec":    result.OperationsPerSec,
		"avg_latency_ms": result.LatencyAvg.Milliseconds(),
	})

	return nil
}

// BenchmarkFUSEDirectoryOps benchmarks directory operations
func (fbs *FUSEBenchmarkSuite) BenchmarkFUSEDirectoryOps(config *BenchmarkConfig) error {
	fbs.logger.Info("Starting FUSE directory operations benchmark", map[string]interface{}{
		"mount_path": fbs.mountPath,
		"duration":   config.Duration,
	})

	filesDir := filepath.Join(fbs.mountPath, "files")
	
	// Benchmark directory operations
	start := time.Now()
	end := start.Add(config.Duration)
	var operations int64
	latencyTracker := NewLatencyTracker()

	for time.Now().Before(end) {
		// Create directory
		dirName := filepath.Join(filesDir, fmt.Sprintf("dir_%d", operations))
		
		opStart := time.Now()
		err := os.Mkdir(dirName, 0755)
		createLatency := time.Since(opStart)
		latencyTracker.Record(createLatency)
		
		if err != nil {
			fbs.logger.Warn("Mkdir operation failed", map[string]interface{}{
				"dirname": dirName,
				"error":   err.Error(),
			})
			continue
		}
		
		// List directory (readdir)
		opStart = time.Now()
		entries, err := os.ReadDir(filesDir)
		listLatency := time.Since(opStart)
		latencyTracker.Record(listLatency)
		
		if err != nil {
			fbs.logger.Warn("ReadDir operation failed", map[string]interface{}{
				"dirname": filesDir,
				"error":   err.Error(),
			})
		} else {
			fbs.logger.Debug("Directory listing", map[string]interface{}{
				"entry_count": len(entries),
			})
		}
		
		// Remove directory
		opStart = time.Now()
		err = os.Remove(dirName)
		removeLatency := time.Since(opStart)
		latencyTracker.Record(removeLatency)
		
		if err != nil {
			fbs.logger.Warn("Remove operation failed", map[string]interface{}{
				"dirname": dirName,
				"error":   err.Error(),
			})
		}
		
		operations += 3 // mkdir, readdir, rmdir
	}

	duration := time.Since(start)

	result := BenchmarkResult{
		Name:            "FUSE Directory Ops",
		Duration:        duration,
		Operations:      operations,
		BytesProcessed:  0,
		OperationsPerSec: float64(operations) / duration.Seconds(),
		ThroughputMBps:  0,
		LatencyAvg:      latencyTracker.GetAverage(),
		LatencyP95:      latencyTracker.GetPercentile(95),
		LatencyP99:      latencyTracker.GetPercentile(99),
		ErrorCount:      0,
	}

	fbs.results = append(fbs.results, result)
	fbs.logger.Info("FUSE directory operations benchmark completed", map[string]interface{}{
		"operations":     result.Operations,
		"ops_per_sec":    result.OperationsPerSec,
		"avg_latency_ms": result.LatencyAvg.Milliseconds(),
	})

	return nil
}

// RunFullFUSEBenchmarkSuite runs all FUSE benchmarks
func (fbs *FUSEBenchmarkSuite) RunFullFUSEBenchmarkSuite(config *BenchmarkConfig) error {
	fbs.logger.Info("Starting full FUSE benchmark suite", map[string]interface{}{
		"mount_path": fbs.mountPath,
	})

	benchmarks := []struct {
		name string
		fn   func(*BenchmarkConfig) error
	}{
		{"FUSE File Operations", fbs.BenchmarkFUSEFileOperations},
		{"FUSE Metadata Operations", fbs.BenchmarkFUSEMetadata},
		{"FUSE Directory Operations", fbs.BenchmarkFUSEDirectoryOps},
	}

	for _, benchmark := range benchmarks {
		fbs.logger.Info("Running benchmark", map[string]interface{}{
			"benchmark": benchmark.name,
		})
		
		if err := benchmark.fn(config); err != nil {
			fbs.logger.Error("Benchmark failed", map[string]interface{}{
				"benchmark": benchmark.name,
				"error":     err.Error(),
			})
			return fmt.Errorf("benchmark %s failed: %w", benchmark.name, err)
		}
	}

	fbs.logger.Info("Full FUSE benchmark suite completed", map[string]interface{}{
		"total_benchmarks": len(benchmarks),
	})

	return nil
}
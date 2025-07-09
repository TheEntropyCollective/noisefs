package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/benchmarks"
	"github.com/TheEntropyCollective/noisefs/pkg/config"
	"github.com/TheEntropyCollective/noisefs/pkg/logging"
)

func main() {
	var (
		configFile    = flag.String("config", "", "Configuration file path")
		mountPath     = flag.String("mount", "", "Mount point for FUSE benchmarks")
		outputFormat  = flag.String("format", "text", "Output format: text or json")
		outputFile    = flag.String("output", "", "Output file (default: stdout)")
		duration      = flag.Duration("duration", 30*time.Second, "Benchmark duration")
		concurrency   = flag.Int("concurrency", 10, "Number of concurrent workers")
		fileSize      = flag.Int64("file-size", 1024*1024, "Test file size in bytes")
		blockSize     = flag.Int("block-size", 4096, "Block size for operations")
		fileCount     = flag.Int("file-count", 100, "Number of test files")
		readRatio     = flag.Float64("read-ratio", 0.7, "Ratio of read operations (0.0-1.0)")
		warmupTime    = flag.Duration("warmup", 5*time.Second, "Warmup duration")
		benchmarkType = flag.String("type", "all", "Benchmark type: all, basic, fuse, sequential, random, metadata")
		basePath      = flag.String("base-path", "/tmp/noisefs-benchmark", "Base path for benchmark files")
	)

	flag.Parse()

	// Load configuration
	cfg, err := loadConfig(*configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logging
	if err := logging.InitFromConfig(cfg.Logging.Level, cfg.Logging.Format, cfg.Logging.Output, cfg.Logging.File); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logging: %v\n", err)
		os.Exit(1)
	}

	logger := logging.GetGlobalLogger().WithComponent("noisefs-benchmark")

	// Create benchmark configuration
	benchConfig := &benchmarks.BenchmarkConfig{
		Duration:    *duration,
		Concurrency: *concurrency,
		FileSize:    *fileSize,
		BlockSize:   *blockSize,
		FileCount:   *fileCount,
		ReadRatio:   *readRatio,
		WarmupTime:  *warmupTime,
	}

	logger.Info("Starting NoiseFS benchmarks", map[string]interface{}{
		"type":         *benchmarkType,
		"duration":     *duration,
		"concurrency":  *concurrency,
		"file_size":    *fileSize,
		"output_format": *outputFormat,
	})

	// Create base path
	if err := os.MkdirAll(*basePath, 0755); err != nil {
		logger.Error("Failed to create base path", map[string]interface{}{
			"path":  *basePath,
			"error": err.Error(),
		})
		os.Exit(1)
	}
	defer os.RemoveAll(*basePath)

	// Run benchmarks based on type
	var results []benchmarks.BenchmarkResult

	switch *benchmarkType {
	case "basic":
		results, err = runBasicBenchmarks(*basePath, benchConfig, logger)
	case "fuse":
		if *mountPath == "" {
			logger.Error("Mount path required for FUSE benchmarks", nil)
			os.Exit(1)
		}
		results, err = runFUSEBenchmarks(*mountPath, benchConfig, logger)
	case "sequential":
		results, err = runSequentialBenchmarks(*basePath, benchConfig, logger)
	case "random":
		results, err = runRandomBenchmarks(*basePath, benchConfig, logger)
	case "metadata":
		if *mountPath == "" {
			logger.Error("Mount path required for metadata benchmarks", nil)
			os.Exit(1)
		}
		results, err = runMetadataBenchmarks(*mountPath, benchConfig, logger)
	case "all":
		results, err = runAllBenchmarks(*basePath, *mountPath, benchConfig, logger)
	default:
		logger.Error("Invalid benchmark type", map[string]interface{}{
			"type": *benchmarkType,
		})
		os.Exit(1)
	}

	if err != nil {
		logger.Error("Benchmark failed", map[string]interface{}{
			"error": err.Error(),
		})
		os.Exit(1)
	}

	// Output results
	if err := outputResults(results, *outputFormat, *outputFile, logger); err != nil {
		logger.Error("Failed to output results", map[string]interface{}{
			"error": err.Error(),
		})
		os.Exit(1)
	}

	logger.Info("Benchmarks completed successfully", map[string]interface{}{
		"total_benchmarks": len(results),
	})
}

func loadConfig(configPath string) (*config.Config, error) {
	if configPath == "" {
		// Try default config path
		defaultPath, err := config.GetDefaultConfigPath()
		if err == nil {
			configPath = defaultPath
		}
	}
	
	return config.LoadConfig(configPath)
}

func runBasicBenchmarks(basePath string, config *benchmarks.BenchmarkConfig, logger *logging.Logger) ([]benchmarks.BenchmarkResult, error) {
	suite := benchmarks.NewBenchmarkSuite("Basic File Operations", basePath, logger)
	
	if err := suite.BenchmarkFileOperations(config); err != nil {
		return nil, err
	}
	
	return suite.GetResults(), nil
}

func runFUSEBenchmarks(mountPath string, config *benchmarks.BenchmarkConfig, logger *logging.Logger) ([]benchmarks.BenchmarkResult, error) {
	suite := benchmarks.NewFUSEBenchmarkSuite(mountPath, logger)
	
	if err := suite.RunFullFUSEBenchmarkSuite(config); err != nil {
		return nil, err
	}
	
	return suite.GetResults(), nil
}

func runSequentialBenchmarks(basePath string, config *benchmarks.BenchmarkConfig, logger *logging.Logger) ([]benchmarks.BenchmarkResult, error) {
	suite := benchmarks.NewBenchmarkSuite("Sequential Operations", basePath, logger)
	
	if err := suite.BenchmarkSequentialRead(config); err != nil {
		return nil, err
	}
	
	return suite.GetResults(), nil
}

func runRandomBenchmarks(basePath string, config *benchmarks.BenchmarkConfig, logger *logging.Logger) ([]benchmarks.BenchmarkResult, error) {
	suite := benchmarks.NewBenchmarkSuite("Random Operations", basePath, logger)
	
	if err := suite.BenchmarkRandomRead(config); err != nil {
		return nil, err
	}
	
	return suite.GetResults(), nil
}

func runMetadataBenchmarks(mountPath string, config *benchmarks.BenchmarkConfig, logger *logging.Logger) ([]benchmarks.BenchmarkResult, error) {
	suite := benchmarks.NewFUSEBenchmarkSuite(mountPath, logger)
	
	if err := suite.BenchmarkFUSEMetadata(config); err != nil {
		return nil, err
	}
	
	return suite.GetResults(), nil
}

func runAllBenchmarks(basePath, mountPath string, config *benchmarks.BenchmarkConfig, logger *logging.Logger) ([]benchmarks.BenchmarkResult, error) {
	var allResults []benchmarks.BenchmarkResult

	// Basic benchmarks
	logger.Info("Running basic benchmarks", nil)
	basicResults, err := runBasicBenchmarks(basePath, config, logger)
	if err != nil {
		return nil, fmt.Errorf("basic benchmarks failed: %w", err)
	}
	allResults = append(allResults, basicResults...)

	// Sequential benchmarks
	logger.Info("Running sequential benchmarks", nil)
	seqResults, err := runSequentialBenchmarks(basePath, config, logger)
	if err != nil {
		return nil, fmt.Errorf("sequential benchmarks failed: %w", err)
	}
	allResults = append(allResults, seqResults...)

	// Random benchmarks
	logger.Info("Running random benchmarks", nil)
	randResults, err := runRandomBenchmarks(basePath, config, logger)
	if err != nil {
		return nil, fmt.Errorf("random benchmarks failed: %w", err)
	}
	allResults = append(allResults, randResults...)

	// FUSE benchmarks (if mount path provided)
	if mountPath != "" {
		logger.Info("Running FUSE benchmarks", nil)
		fuseResults, err := runFUSEBenchmarks(mountPath, config, logger)
		if err != nil {
			logger.Warn("FUSE benchmarks failed", map[string]interface{}{
				"error": err.Error(),
			})
			// Don't fail completely if FUSE benchmarks fail
		} else {
			allResults = append(allResults, fuseResults...)
		}
	}

	return allResults, nil
}

func outputResults(results []benchmarks.BenchmarkResult, format, outputFile string, logger *logging.Logger) error {
	var output string
	var err error

	switch format {
	case "json":
		data, marshalErr := json.MarshalIndent(results, "", "  ")
		if marshalErr != nil {
			return fmt.Errorf("failed to marshal JSON: %w", marshalErr)
		}
		output = string(data)
	case "text":
		output = formatTextResults(results)
	default:
		return fmt.Errorf("invalid output format: %s", format)
	}

	if outputFile != "" {
		err = os.WriteFile(outputFile, []byte(output), 0644)
		if err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
		logger.Info("Results written to file", map[string]interface{}{
			"file": outputFile,
		})
	} else {
		fmt.Print(output)
	}

	return nil
}

func formatTextResults(results []benchmarks.BenchmarkResult) string {
	output := "\n=== NoiseFS Benchmark Results ===\n\n"
	
	for _, result := range results {
		output += fmt.Sprintf("Benchmark: %s\n", result.Name)
		output += fmt.Sprintf("  Duration: %v\n", result.Duration)
		output += fmt.Sprintf("  Operations: %d\n", result.Operations)
		output += fmt.Sprintf("  Operations/sec: %.2f\n", result.OperationsPerSec)
		
		if result.BytesProcessed > 0 {
			output += fmt.Sprintf("  Throughput: %.2f MB/s\n", result.ThroughputMBps)
		}
		
		output += fmt.Sprintf("  Latency (avg/p95/p99): %v / %v / %v\n", 
			result.LatencyAvg, result.LatencyP95, result.LatencyP99)
		
		if result.ErrorCount > 0 {
			output += fmt.Sprintf("  Errors: %d\n", result.ErrorCount)
		}
		
		output += "\n"
	}
	
	return output
}
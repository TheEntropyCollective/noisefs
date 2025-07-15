package integration

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/config"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	"github.com/TheEntropyCollective/noisefs/tests/benchmarks"
)

// TestCrossAgentValidation validates all agent work from Sprint 6
func TestCrossAgentValidation(t *testing.T) {
	t.Run("Agent2_ConfigurationPresets", testAgent2ConfigurationPresets)
	t.Run("Agent1_AtomicOperations", testAgent1AtomicOperations)
	t.Run("BaselinePerformance", testBaselinePerformanceMeasurements)
	t.Run("RegressionDetection", testRegressionDetectionFramework)
}

// testAgent2ConfigurationPresets validates Agent 2's configuration preset work
func testAgent2ConfigurationPresets(t *testing.T) {
	presets := []string{"quickstart", "security", "performance"}
	
	for _, preset := range presets {
		t.Run(fmt.Sprintf("Preset_%s", preset), func(t *testing.T) {
			// Test preset configuration generation
			cfg, err := config.GetPresetConfig(preset)
			if err != nil {
				t.Fatalf("Failed to get %s preset config: %v", preset, err)
			}
			
			// Validate configuration
			if err := cfg.Validate(); err != nil {
				t.Fatalf("%s preset configuration validation failed: %v", preset, err)
			}
			
			// Test preset-specific settings
			switch preset {
			case "quickstart":
				validateQuickStartPreset(t, cfg)
			case "security":
				validateSecurityPreset(t, cfg)
			case "performance":
				validatePerformancePreset(t, cfg)
			}
			
			t.Logf("✅ %s preset validated successfully", preset)
		})
	}
}

func validateQuickStartPreset(t *testing.T, cfg *config.Config) {
	// QuickStart should have conservative settings
	if cfg.Cache.MemoryLimit != 256 {
		t.Errorf("QuickStart should have 256MB memory limit, got %d", cfg.Cache.MemoryLimit)
	}
	
	if cfg.Performance.MaxConcurrentOps != 5 {
		t.Errorf("QuickStart should have 5 max concurrent ops, got %d", cfg.Performance.MaxConcurrentOps)
	}
	
	if cfg.Tor.Enabled {
		t.Error("QuickStart should have Tor disabled for simplicity")
	}
	
	if cfg.Logging.Level != "warn" {
		t.Errorf("QuickStart should have warn logging, got %s", cfg.Logging.Level)
	}
}

func validateSecurityPreset(t *testing.T, cfg *config.Config) {
	// Security should have maximum security features
	if cfg.Cache.MemoryLimit != 1024 {
		t.Errorf("Security should have 1024MB memory limit, got %d", cfg.Cache.MemoryLimit)
	}
	
	if !cfg.Tor.Enabled {
		t.Error("Security preset should have Tor enabled")
	}
	
	if !cfg.Security.EnableEncryption {
		t.Error("Security preset should have encryption enabled")
	}
	
	if !cfg.Security.AntiForensics {
		t.Error("Security preset should have anti-forensics enabled")
	}
	
	if cfg.WebUI.Host != "127.0.0.1" {
		t.Errorf("Security preset should restrict WebUI to localhost, got %s", cfg.WebUI.Host)
	}
	
	if cfg.WebUI.TLSMinVersion != "1.3" {
		t.Errorf("Security preset should require TLS 1.3, got %s", cfg.WebUI.TLSMinVersion)
	}
}

func validatePerformancePreset(t *testing.T, cfg *config.Config) {
	// Performance should have high-performance settings
	if cfg.Cache.MemoryLimit != 2048 {
		t.Errorf("Performance should have 2048MB memory limit, got %d", cfg.Cache.MemoryLimit)
	}
	
	if cfg.Performance.MaxConcurrentOps != 50 {
		t.Errorf("Performance should have 50 max concurrent ops, got %d", cfg.Performance.MaxConcurrentOps)
	}
	
	if !cfg.Performance.ReadAhead {
		t.Error("Performance preset should have read-ahead enabled")
	}
	
	if !cfg.Performance.WriteBack {
		t.Error("Performance preset should have write-back enabled")
	}
	
	if cfg.Tor.Enabled {
		t.Error("Performance preset should have Tor disabled for speed")
	}
	
	if cfg.Logging.Level != "warn" {
		t.Errorf("Performance preset should have minimal logging, got %s", cfg.Logging.Level)
	}
}

// testAgent1AtomicOperations validates Agent 1's atomic operations fixes
func testAgent1AtomicOperations(t *testing.T) {
	// Create simple memory cache for testing atomic operations
	memCache := cache.NewMemoryCache(100 * 1024 * 1024) // 100MB
	
	t.Run("ConcurrentMetricsUpdates", func(t *testing.T) {
		// Test concurrent metrics updates don't cause race conditions
		const numGoroutines = 50
		const operationsPerGoroutine = 100
		
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		
		done := make(chan error, numGoroutines)
		
		// Start concurrent operations
		for i := 0; i < numGoroutines; i++ {
			go func(workerID int) {
				defer func() {
					if r := recover(); r != nil {
						done <- fmt.Errorf("worker %d panicked: %v", workerID, r)
						return
					}
					done <- nil
				}()
				
				for j := 0; j < operationsPerGoroutine; j++ {
					// Simulate block operations
					cid := fmt.Sprintf("test-block-%d-%d", workerID, j)
					block, err := blocks.NewBlock([]byte("test data"))
					if err != nil {
						done <- fmt.Errorf("failed to create block: %v", err)
						return
					}
					
					// Store and retrieve block (tests concurrent operations)
					memCache.Store(cid, block)
					memCache.Get(cid)
					
					// Check for context cancellation
					select {
					case <-ctx.Done():
						return
					default:
					}
				}
			}(i)
		}
		
		// Wait for all goroutines to complete
		var errors []error
		for i := 0; i < numGoroutines; i++ {
			select {
			case err := <-done:
				if err != nil {
					errors = append(errors, err)
				}
			case <-ctx.Done():
				t.Fatal("Test timed out - possible deadlock in atomic operations")
			}
		}
		
		// Check for errors (race conditions, panics)
		if len(errors) > 0 {
			t.Fatalf("Concurrent operations failed with %d errors. First error: %v", 
				len(errors), errors[0])
		}
		
		// Verify cache is still functional
		stats := memCache.GetStats()
		if stats.Hits == 0 && stats.Misses == 0 {
			t.Error("Expected some cache operations to be recorded")
		}
		
		t.Logf("✅ Atomic operations validated - %d concurrent workers completed without race conditions", 
			numGoroutines)
	})
	
	t.Run("MetricsConsistency", func(t *testing.T) {
		// Verify that metrics remain consistent under concurrent load
		stats1 := memCache.GetStats()
		
		// Small delay to allow any pending operations
		time.Sleep(10 * time.Millisecond)
		
		stats2 := memCache.GetStats()
		
		// Metrics should be stable (no race conditions in reading)
		if stats1.Hits != stats2.Hits {
			t.Errorf("Metrics inconsistent: Hits changed from %d to %d without operations", 
				stats1.Hits, stats2.Hits)
		}
		
		if stats1.Misses != stats2.Misses {
			t.Errorf("Metrics inconsistent: Misses changed from %d to %d without operations", 
				stats1.Misses, stats2.Misses)
		}
		
		t.Log("✅ Metrics consistency validated")
	})
}

// testBaselinePerformanceMeasurements validates baseline measurements are captured
func testBaselinePerformanceMeasurements(t *testing.T) {
	// Create baseline metrics from our benchmark run
	baseline := benchmarks.GetBaselineMetricsFromBenchmarks()
	
	// Validate baseline has required measurements
	if baseline.CacheEfficiency.WithCaching.NsPerOp == 0 {
		t.Error("Baseline missing cache efficiency measurements")
	}
	
	if baseline.CacheEfficiency.CacheSpeedup < 2.0 {
		t.Errorf("Cache speedup too low: %.2fx (expected > 2x)", baseline.CacheEfficiency.CacheSpeedup)
	}
	
	if len(baseline.ConcurrentLoad) == 0 {
		t.Error("Baseline missing concurrent load measurements")
	}
	
	if len(baseline.StorageEfficiency) == 0 {
		t.Error("Baseline missing storage efficiency measurements")
	}
	
	// Validate eviction strategies
	if baseline.EvictionStrategies.LRU.NsPerOp == 0 {
		t.Error("Baseline missing LRU eviction measurements")
	}
	
	if baseline.EvictionStrategies.Adaptive.NsPerOp == 0 {
		t.Error("Baseline missing Adaptive eviction measurements")
	}
	
	// Save baseline for future regression detection
	baselinePath := "/tmp/noisefs_performance_baseline.json"
	detector := benchmarks.NewRegressionDetector(baselinePath)
	
	if err := detector.SaveBaseline(baseline); err != nil {
		t.Fatalf("Failed to save baseline: %v", err)
	}
	
	// Verify we can load it back
	if err := detector.LoadBaseline(); err != nil {
		t.Fatalf("Failed to load saved baseline: %v", err)
	}
	
	t.Logf("✅ Baseline measurements validated and saved to %s", baselinePath)
	t.Logf("   Cache speedup: %.2fx", baseline.CacheEfficiency.CacheSpeedup)
	t.Logf("   Concurrent load tests: %d", len(baseline.ConcurrentLoad))
	t.Logf("   Storage efficiency tests: %d", len(baseline.StorageEfficiency))
}

// testRegressionDetectionFramework validates the regression detection system
func testRegressionDetectionFramework(t *testing.T) {
	// Create test baseline
	baseline := benchmarks.GetBaselineMetricsFromBenchmarks()
	
	// Create detector
	baselinePath := "/tmp/noisefs_test_baseline.json"
	detector := benchmarks.NewRegressionDetector(baselinePath)
	
	// Save baseline
	if err := detector.SaveBaseline(baseline); err != nil {
		t.Fatalf("Failed to save test baseline: %v", err)
	}
	
	// Load baseline
	if err := detector.LoadBaseline(); err != nil {
		t.Fatalf("Failed to load test baseline: %v", err)
	}
	
	t.Run("NoRegressionDetection", func(t *testing.T) {
		// Test with same metrics (should detect no regressions)
		report, err := detector.DetectRegressions(baseline)
		if err != nil {
			t.Fatalf("Failed to detect regressions: %v", err)
		}
		
		if report.HasRegressions {
			t.Errorf("False positive: detected %d regressions when none should exist", 
				len(report.Regressions))
		}
		
		t.Log("✅ No false positives in regression detection")
	})
	
	t.Run("RegressionDetection", func(t *testing.T) {
		// Create degraded metrics to test regression detection
		degraded := *baseline
		
		// Make cache efficiency 50% slower (should trigger regression)
		degraded.CacheEfficiency.WithCaching.NsPerOp *= 2 // 2x slower
		
		// Make LRU eviction 30% slower (should trigger regression)
		degraded.EvictionStrategies.LRU.NsPerOp = int64(float64(baseline.EvictionStrategies.LRU.NsPerOp) * 1.3)
		
		// Increase memory usage by 40% (should trigger regression)
		degraded.EvictionStrategies.LRU.BytesPerOp = int64(float64(baseline.EvictionStrategies.LRU.BytesPerOp) * 1.4)
		
		report, err := detector.DetectRegressions(&degraded)
		if err != nil {
			t.Fatalf("Failed to detect regressions: %v", err)
		}
		
		if !report.HasRegressions {
			t.Error("Failed to detect intentional regressions")
		}
		
		if len(report.Regressions) < 2 {
			t.Errorf("Expected at least 2 regressions, got %d", len(report.Regressions))
		}
		
		// Verify specific regressions were detected
		hasLatencyRegression := false
		hasMemoryRegression := false
		
		for _, reg := range report.Regressions {
			if reg.Metric == "Latency" && reg.Ratio > 1.25 {
				hasLatencyRegression = true
			}
			if reg.Metric == "Memory" && reg.Ratio > 1.30 {
				hasMemoryRegression = true
			}
		}
		
		if !hasLatencyRegression {
			t.Error("Failed to detect latency regression")
		}
		
		if !hasMemoryRegression {
			t.Error("Failed to detect memory regression")
		}
		
		// Save regression report
		reportPath := "/tmp/noisefs_test_regression_report.json"
		if err := benchmarks.SaveRegressionReport(report, reportPath); err != nil {
			t.Fatalf("Failed to save regression report: %v", err)
		}
		
		t.Logf("✅ Regression detection validated - found %d regressions", len(report.Regressions))
		t.Logf("   Report saved to %s", reportPath)
	})
	
	// Cleanup
	os.Remove(baselinePath)
	os.Remove("/tmp/noisefs_test_regression_report.json")
}

// BenchmarkCrossAgentValidation provides performance measurements for the validation suite
func BenchmarkCrossAgentValidation(b *testing.B) {
	b.Run("ConfigPresetCreation", func(b *testing.B) {
		presets := []string{"quickstart", "security", "performance"}
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			preset := presets[i%len(presets)]
			_, err := config.GetPresetConfig(preset)
			if err != nil {
				b.Fatalf("Failed to create %s preset: %v", preset, err)
			}
		}
	})
	
	b.Run("AtomicOperationsPerformance", func(b *testing.B) {
		memCache := cache.NewMemoryCache(100 * 1024 * 1024) // 100MB
		
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				cid := fmt.Sprintf("bench-block-%d", i)
				block, _ := blocks.NewBlock([]byte("test data"))
				
				// Store and retrieve block (exercises atomic operations)
				memCache.Store(cid, block)
				memCache.Get(cid)
				
				i++
			}
		})
	})
	
	b.Run("RegressionDetectionPerformance", func(b *testing.B) {
		baseline := benchmarks.GetBaselineMetricsFromBenchmarks()
		
		baselinePath := "/tmp/noisefs_bench_baseline.json" 
		detector := benchmarks.NewRegressionDetector(baselinePath)
		detector.SaveBaseline(baseline)
		detector.LoadBaseline()
		
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := detector.DetectRegressions(baseline)
			if err != nil {
				b.Fatalf("Regression detection failed: %v", err)
			}
		}
		
		b.Cleanup(func() {
			os.Remove(baselinePath)
		})
	})
}

// TestSystemIntegration validates the complete system works together
func TestSystemIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping system integration test in short mode")
	}
	
	t.Run("ConfigurationToExecution", func(t *testing.T) {
		// Test that configuration presets lead to working system
		presets := []string{"quickstart", "performance"}
		
		for _, preset := range presets {
			t.Run(fmt.Sprintf("Preset_%s", preset), func(t *testing.T) {
				// Get preset configuration
				cfg, err := config.GetPresetConfig(preset)
				if err != nil {
					t.Fatalf("Failed to get %s preset: %v", preset, err)
				}
				
				// Create cache with preset configuration
				memCache := cache.NewMemoryCache(int(cfg.Cache.MemoryLimit) * 1024 * 1024)
				
				// Test basic operations work
				cid := fmt.Sprintf("integration-test-%s", preset)
				block, err := blocks.NewBlock([]byte("test data"))
				if err != nil {
					t.Fatalf("Failed to create block: %v", err)
				}
				
				// Store block
				if err := memCache.Store(cid, block); err != nil {
					t.Errorf("Failed to store block with %s preset configuration: %v", preset, err)
				}
				
				// Retrieve block
				retrieved, err := memCache.Get(cid)
				if err != nil || retrieved == nil {
					t.Errorf("Failed to retrieve block with %s preset configuration: %v", preset, err)
				}
				
				// Verify metrics work
				stats := memCache.GetStats()
				if stats.Hits == 0 && stats.Misses == 0 {
					t.Errorf("Metrics not working with %s preset configuration", preset)
				}
				
				t.Logf("✅ %s preset configuration working correctly", preset)
			})
		}
	})
}

// Helper function to print system information
func init() {
	fmt.Printf("=== Cross-Agent Validation Test Suite ===\n")
	fmt.Printf("Runtime: %s %s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("Go version: %s\n", runtime.Version())
	fmt.Printf("CPU cores: %d\n", runtime.NumCPU())
	fmt.Printf("==========================================\n\n")
}
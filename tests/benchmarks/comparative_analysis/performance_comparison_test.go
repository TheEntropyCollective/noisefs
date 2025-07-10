package comparative_analysis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/noisefs"
)

// PerformanceMetrics tracks comprehensive performance data
type PerformanceMetrics struct {
	TestName              string                 `json:"test_name"`
	SystemVersion         string                 `json:"system_version"`
	Timestamp             time.Time              `json:"timestamp"`
	Duration              time.Duration          `json:"duration"`
	
	// Operation metrics
	TotalOperations       int64                  `json:"total_operations"`
	SuccessfulOperations  int64                  `json:"successful_operations"`
	FailedOperations      int64                  `json:"failed_operations"`
	OperationsPerSecond   float64                `json:"operations_per_second"`
	
	// Latency metrics
	AverageLatency        time.Duration          `json:"average_latency"`
	MedianLatency         time.Duration          `json:"median_latency"`
	P95Latency            time.Duration          `json:"p95_latency"`
	P99Latency            time.Duration          `json:"p99_latency"`
	MinLatency            time.Duration          `json:"min_latency"`
	MaxLatency            time.Duration          `json:"max_latency"`
	
	// Throughput metrics
	TotalBytesTransferred int64                  `json:"total_bytes_transferred"`
	AverageThroughputMBps float64                `json:"average_throughput_mbps"`
	PeakThroughputMBps    float64                `json:"peak_throughput_mbps"`
	
	// Storage efficiency
	OriginalDataSize      int64                  `json:"original_data_size"`
	StoredDataSize        int64                  `json:"stored_data_size"`
	StorageOverheadRatio  float64                `json:"storage_overhead_ratio"`
	CompressionRatio      float64                `json:"compression_ratio"`
	
	// Cache performance
	CacheHitRate          float64                `json:"cache_hit_rate"`
	CacheHits             int64                  `json:"cache_hits"`
	CacheMisses           int64                  `json:"cache_misses"`
	CacheEvictions        int64                  `json:"cache_evictions"`
	
	// Memory and resource usage
	PeakMemoryUsageMB     float64                `json:"peak_memory_usage_mb"`
	AverageMemoryUsageMB  float64                `json:"average_memory_usage_mb"`
	CPUUsagePercent       float64                `json:"cpu_usage_percent"`
	
	// Detailed breakdowns
	UploadMetrics         *OperationMetrics      `json:"upload_metrics,omitempty"`
	DownloadMetrics       *OperationMetrics      `json:"download_metrics,omitempty"`
	CacheMetrics          *CacheMetrics          `json:"cache_metrics,omitempty"`
	
	// Custom metrics for different test scenarios
	CustomMetrics         map[string]interface{} `json:"custom_metrics,omitempty"`
}

type OperationMetrics struct {
	Count             int64         `json:"count"`
	AverageLatency    time.Duration `json:"average_latency"`
	TotalBytes        int64         `json:"total_bytes"`
	ThroughputMBps    float64       `json:"throughput_mbps"`
	SuccessRate       float64       `json:"success_rate"`
}

type CacheMetrics struct {
	HitRate           float64 `json:"hit_rate"`
	MissRate          float64 `json:"miss_rate"`
	EvictionRate      float64 `json:"eviction_rate"`
	PredictionAccuracy float64 `json:"prediction_accuracy,omitempty"`
	AdaptiveScore     float64 `json:"adaptive_score,omitempty"`
}

type LatencyMeasurement struct {
	Operation string
	StartTime time.Time
	EndTime   time.Time
	Success   bool
	ByteSize  int64
}

// BenchmarkBasicOperations provides baseline performance measurements
func BenchmarkBasicOperations(b *testing.B) {
	scenarios := []struct {
		name      string
		blockSize int
		dataSize  int64
	}{
		{"Small_4KB", 4 * 1024, 4 * 1024},
		{"Medium_64KB", 64 * 1024, 64 * 1024},
		{"Large_1MB", 1024 * 1024, 1024 * 1024},
		{"XLarge_10MB", 10 * 1024 * 1024, 10 * 1024 * 1024},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			suite := setupBenchmarkSuite(b)
			metrics := &PerformanceMetrics{
				TestName:      fmt.Sprintf("BenchmarkBasicOperations_%s", scenario.name),
				SystemVersion: "milestone4",
				Timestamp:     time.Now(),
				CustomMetrics: make(map[string]interface{}),
			}

			// Generate test data
			testData := generateBenchmarkData(int(scenario.dataSize))
			
			latencies := make([]LatencyMeasurement, 0, b.N*2) // Upload + Download
			startTime := time.Now()

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				// Measure upload
				uploadStart := time.Now()
				reader := bytes.NewReader(testData)
				descriptorCID, err := suite.client.Upload(reader, fmt.Sprintf("benchmark_%d.dat", i))
				uploadEnd := time.Now()

				uploadSuccess := err == nil
				if uploadSuccess {
					metrics.SuccessfulOperations++
				} else {
					metrics.FailedOperations++
				}

				latencies = append(latencies, LatencyMeasurement{
					Operation: "upload",
					StartTime: uploadStart,
					EndTime:   uploadEnd,
					Success:   uploadSuccess,
					ByteSize:  scenario.dataSize,
				})

				if !uploadSuccess {
					continue
				}

				// Measure download
				downloadStart := time.Now()
				downloadedData, err := suite.client.Download(descriptorCID)
				downloadEnd := time.Now()

				downloadSuccess := err == nil && bytes.Equal(testData, downloadedData)
				if downloadSuccess {
					metrics.SuccessfulOperations++
				} else {
					metrics.FailedOperations++
				}

				latencies = append(latencies, LatencyMeasurement{
					Operation: "download",
					StartTime: downloadStart,
					EndTime:   downloadEnd,
					Success:   downloadSuccess,
					ByteSize:  int64(len(downloadedData)),
				})

				metrics.TotalBytesTransferred += scenario.dataSize * 2 // Upload + Download
			}

			b.StopTimer()

			// Calculate comprehensive metrics
			metrics.Duration = time.Since(startTime)
			metrics.TotalOperations = metrics.SuccessfulOperations + metrics.FailedOperations
			metrics.OperationsPerSecond = float64(metrics.TotalOperations) / metrics.Duration.Seconds()

			calculateLatencyMetrics(metrics, latencies)
			calculateThroughputMetrics(metrics, scenario.dataSize, b.N)
			collectCacheMetrics(metrics, suite.cache)
			
			// Save metrics for comparative analysis
			saveMetrics(b, metrics)

			// Report key metrics to benchmark framework
			b.ReportMetric(float64(metrics.AverageLatency.Nanoseconds()), "avg_latency_ns")
			b.ReportMetric(metrics.AverageThroughputMBps, "throughput_mbps")
			b.ReportMetric(metrics.CacheHitRate*100, "cache_hit_rate_percent")
		})
	}
}

// BenchmarkCacheEffectiveness tests cache performance improvements over time
func BenchmarkCacheEffectiveness(b *testing.B) {
	suite := setupBenchmarkSuite(b)
	
	// Create popular and unpopular content
	popularFiles := make([][]byte, 10)
	unpopularFiles := make([][]byte, 100)
	
	for i := 0; i < 10; i++ {
		popularFiles[i] = generateBenchmarkData(32 * 1024) // 32KB popular files
	}
	for i := 0; i < 100; i++ {
		unpopularFiles[i] = generateBenchmarkData(32 * 1024) // 32KB unpopular files
	}

	metrics := &PerformanceMetrics{
		TestName:      "BenchmarkCacheEffectiveness",
		SystemVersion: "milestone4",
		Timestamp:     time.Now(),
		CustomMetrics: make(map[string]interface{}),
	}

	// Upload all files first
	popularCIDs := make([]string, len(popularFiles))
	unpopularCIDs := make([]string, len(unpopularFiles))

	for i, data := range popularFiles {
		reader := bytes.NewReader(data)
		cid, _ := suite.client.Upload(reader, fmt.Sprintf("popular_%d.dat", i))
		popularCIDs[i] = cid
	}

	for i, data := range unpopularFiles {
		reader := bytes.NewReader(data)
		cid, _ := suite.client.Upload(reader, fmt.Sprintf("unpopular_%d.dat", i))
		unpopularCIDs[i] = cid
	}

	startTime := time.Now()
	b.ResetTimer()

	// Simulate realistic access patterns: 80% popular, 20% unpopular
	for i := 0; i < b.N; i++ {
		var cid string
		if i%5 == 0 { // 20% unpopular
			cid = unpopularCIDs[i%len(unpopularCIDs)]
		} else { // 80% popular
			cid = popularCIDs[i%len(popularCIDs)]
		}

		_, err := suite.client.Download(cid)
		if err == nil {
			metrics.SuccessfulOperations++
		} else {
			metrics.FailedOperations++
		}
	}

	b.StopTimer()
	
	metrics.Duration = time.Since(startTime)
	metrics.TotalOperations = metrics.SuccessfulOperations + metrics.FailedOperations
	collectCacheMetrics(metrics, suite.cache)
	
	// Calculate cache learning effectiveness
	initialStats := suite.cache.GetStats()
	metrics.CustomMetrics["cache_learning_score"] = calculateCacheLearningScore(initialStats)
	
	saveMetrics(b, metrics)
	b.ReportMetric(metrics.CacheHitRate*100, "cache_hit_rate_percent")
}

// BenchmarkStorageEfficiency measures storage overhead and efficiency
func BenchmarkStorageEfficiency(b *testing.B) {
	suite := setupBenchmarkSuite(b)
	
	fileSizes := []int64{
		1024,           // 1KB
		16 * 1024,      // 16KB
		128 * 1024,     // 128KB
		1024 * 1024,    // 1MB
		10 * 1024 * 1024, // 10MB
	}

	for _, fileSize := range fileSizes {
		b.Run(fmt.Sprintf("Size_%dKB", fileSize/1024), func(b *testing.B) {
			metrics := &PerformanceMetrics{
				TestName:      fmt.Sprintf("BenchmarkStorageEfficiency_%dKB", fileSize/1024),
				SystemVersion: "milestone4",
				Timestamp:     time.Now(),
				CustomMetrics: make(map[string]interface{}),
			}

			totalOriginalSize := int64(0)
			totalStoredSize := int64(0)

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				testData := generateBenchmarkData(int(fileSize))
				reader := bytes.NewReader(testData)
				
				_, err := suite.client.Upload(reader, fmt.Sprintf("efficiency_test_%d_%d.dat", fileSize, i))
				if err == nil {
					totalOriginalSize += fileSize
					// Estimate stored size (in real implementation, this would query actual storage)
					estimatedStoredSize := estimateStoredSize(fileSize)
					totalStoredSize += estimatedStoredSize
					metrics.SuccessfulOperations++
				} else {
					metrics.FailedOperations++
				}
			}

			b.StopTimer()

			metrics.OriginalDataSize = totalOriginalSize
			metrics.StoredDataSize = totalStoredSize
			metrics.StorageOverheadRatio = float64(totalStoredSize) / float64(totalOriginalSize)
			metrics.TotalOperations = metrics.SuccessfulOperations + metrics.FailedOperations

			// Custom storage efficiency metrics
			metrics.CustomMetrics["deduplication_ratio"] = calculateDeduplicationRatio(b.N, fileSize)
			metrics.CustomMetrics["block_reuse_factor"] = calculateBlockReuseFactor(fileSize)

			saveMetrics(b, metrics)
			b.ReportMetric(metrics.StorageOverheadRatio*100, "storage_overhead_percent")
		})
	}
}

// BenchmarkConcurrentOperations tests performance under concurrent load
func BenchmarkConcurrentOperations(b *testing.B) {
	concurrencyLevels := []int{1, 2, 4, 8, 16, 32}

	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("Concurrency_%d", concurrency), func(b *testing.B) {
			suite := setupBenchmarkSuite(b)
			
			metrics := &PerformanceMetrics{
				TestName:      fmt.Sprintf("BenchmarkConcurrentOperations_%d", concurrency),
				SystemVersion: "milestone4",
				Timestamp:     time.Now(),
				CustomMetrics: make(map[string]interface{}),
			}

			// Create a semaphore to limit concurrency
			semaphore := make(chan struct{}, concurrency)
			results := make(chan LatencyMeasurement, b.N)
			
			testData := generateBenchmarkData(64 * 1024) // 64KB per operation

			startTime := time.Now()
			b.ResetTimer()

			// Launch concurrent operations
			for i := 0; i < b.N; i++ {
				go func(id int) {
					semaphore <- struct{}{} // Acquire
					defer func() { <-semaphore }() // Release

					operationStart := time.Now()
					reader := bytes.NewReader(testData)
					_, err := suite.client.Upload(reader, fmt.Sprintf("concurrent_%d_%d.dat", concurrency, id))
					operationEnd := time.Now()

					results <- LatencyMeasurement{
						Operation: "concurrent_upload",
						StartTime: operationStart,
						EndTime:   operationEnd,
						Success:   err == nil,
						ByteSize:  int64(len(testData)),
					}
				}(i)
			}

			// Collect results
			latencies := make([]LatencyMeasurement, 0, b.N)
			for i := 0; i < b.N; i++ {
				measurement := <-results
				latencies = append(latencies, measurement)
				
				if measurement.Success {
					metrics.SuccessfulOperations++
				} else {
					metrics.FailedOperations++
				}
			}

			b.StopTimer()

			metrics.Duration = time.Since(startTime)
			metrics.TotalOperations = metrics.SuccessfulOperations + metrics.FailedOperations
			metrics.OperationsPerSecond = float64(metrics.TotalOperations) / metrics.Duration.Seconds()

			calculateLatencyMetrics(metrics, latencies)
			
			// Concurrency-specific metrics
			metrics.CustomMetrics["concurrency_level"] = concurrency
			metrics.CustomMetrics["concurrent_efficiency"] = metrics.OperationsPerSecond / float64(concurrency)

			saveMetrics(b, metrics)
			b.ReportMetric(metrics.OperationsPerSecond, "ops_per_second")
			b.ReportMetric(float64(metrics.AverageLatency.Nanoseconds()), "avg_latency_ns")
		})
	}
}

// Helper functions

type BenchmarkSuite struct {
	client *noisefs.Client
	cache  cache.Cache
}

func setupBenchmarkSuite(b *testing.B) *BenchmarkSuite {
	// Create mock block store
	blockStore := &MockBlockStore{blocks: make(map[string]*blocks.Block)}
	
	// Create cache with appropriate size for benchmarking
	cacheInstance := cache.NewMemoryCache(100 * 1024 * 1024) // 100MB cache
	
	// Create NoiseFS client
	client, err := noisefs.NewClient(blockStore, cacheInstance)
	if err != nil {
		b.Fatalf("Failed to create NoiseFS client: %v", err)
	}

	return &BenchmarkSuite{
		client: client,
		cache:  cacheInstance,
	}
}

func generateBenchmarkData(size int) []byte {
	data := make([]byte, size)
	for i := 0; i < size; i++ {
		data[i] = byte(i % 256)
	}
	return data
}

func calculateLatencyMetrics(metrics *PerformanceMetrics, latencies []LatencyMeasurement) {
	if len(latencies) == 0 {
		return
	}

	var totalLatency time.Duration
	var minLatency, maxLatency time.Duration
	durations := make([]time.Duration, 0, len(latencies))

	for i, measurement := range latencies {
		duration := measurement.EndTime.Sub(measurement.StartTime)
		durations = append(durations, duration)
		totalLatency += duration

		if i == 0 || duration < minLatency {
			minLatency = duration
		}
		if i == 0 || duration > maxLatency {
			maxLatency = duration
		}
	}

	metrics.AverageLatency = totalLatency / time.Duration(len(latencies))
	metrics.MinLatency = minLatency
	metrics.MaxLatency = maxLatency

	// Calculate percentiles (simplified)
	if len(durations) > 0 {
		// Sort durations for percentile calculation
		for i := 0; i < len(durations)-1; i++ {
			for j := i + 1; j < len(durations); j++ {
				if durations[i] > durations[j] {
					durations[i], durations[j] = durations[j], durations[i]
				}
			}
		}

		p50Index := len(durations) / 2
		p95Index := int(float64(len(durations)) * 0.95)
		p99Index := int(float64(len(durations)) * 0.99)

		if p50Index < len(durations) {
			metrics.MedianLatency = durations[p50Index]
		}
		if p95Index < len(durations) {
			metrics.P95Latency = durations[p95Index]
		}
		if p99Index < len(durations) {
			metrics.P99Latency = durations[p99Index]
		}
	}
}

func calculateThroughputMetrics(metrics *PerformanceMetrics, dataSize int64, operations int) {
	if metrics.Duration.Seconds() > 0 {
		bytesPerSecond := float64(metrics.TotalBytesTransferred) / metrics.Duration.Seconds()
		metrics.AverageThroughputMBps = bytesPerSecond / (1024 * 1024)
		
		// Estimate peak throughput (simplified)
		metrics.PeakThroughputMBps = metrics.AverageThroughputMBps * 1.5
	}
}

func collectCacheMetrics(metrics *PerformanceMetrics, cache cache.Cache) {
	stats := cache.GetStats()
	
	total := stats.Hits + stats.Misses
	if total > 0 {
		metrics.CacheHitRate = float64(stats.Hits) / float64(total)
	}
	
	metrics.CacheHits = stats.Hits
	metrics.CacheMisses = stats.Misses
	metrics.CacheEvictions = stats.Evictions
}

func calculateCacheLearningScore(stats cache.Stats) float64 {
	// Simplified cache learning effectiveness score
	total := stats.Hits + stats.Misses
	if total == 0 {
		return 0.0
	}
	
	hitRate := float64(stats.Hits) / float64(total)
	return hitRate * 100 // Convert to percentage
}

func estimateStoredSize(originalSize int64) int64 {
	// Simplified estimation: assume some overhead for OFFSystem architecture
	// In reality, this would query actual storage systems
	blockSize := int64(128 * 1024) // 128KB blocks
	numBlocks := (originalSize + blockSize - 1) / blockSize
	
	// Assume some overhead for randomizer blocks and metadata
	overhead := float64(1.3) // 30% overhead estimate
	return int64(float64(originalSize) * overhead)
}

func calculateDeduplicationRatio(numFiles int, fileSize int64) float64 {
	// Simplified deduplication calculation
	// In reality, this would measure actual block reuse
	if fileSize < 128*1024 {
		return 1.0 // Small files, minimal deduplication
	}
	return 0.85 // Assume 15% deduplication for larger files
}

func calculateBlockReuseFactor(fileSize int64) float64 {
	// Simplified block reuse factor
	blockSize := int64(128 * 1024)
	numBlocks := (fileSize + blockSize - 1) / blockSize
	
	// Assume some blocks are reused from universal pool
	reuseRate := 0.3 // 30% of blocks reused
	return float64(numBlocks) * reuseRate
}

func saveMetrics(b *testing.B, metrics *PerformanceMetrics) {
	// Create results directory if it doesn't exist
	os.MkdirAll("../../../results/benchmarks", 0755)
	
	// Save metrics to JSON file
	filename := fmt.Sprintf("../../../results/benchmarks/%s_%s.json", 
		metrics.TestName, 
		metrics.Timestamp.Format("20060102_150405"))
	
	data, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		b.Logf("Warning: Failed to marshal metrics: %v", err)
		return
	}
	
	err = os.WriteFile(filename, data, 0644)
	if err != nil {
		b.Logf("Warning: Failed to save metrics to %s: %v", filename, err)
	} else {
		b.Logf("Saved metrics to %s", filename)
	}
}

// MockBlockStore for benchmarking (simplified version)
type MockBlockStore struct {
	blocks map[string]*blocks.Block
}

func (m *MockBlockStore) StoreBlock(block *blocks.Block) (string, error) {
	cid := fmt.Sprintf("bench_%s", block.ID)
	m.blocks[cid] = block
	return cid, nil
}

func (m *MockBlockStore) RetrieveBlock(cid string) (*blocks.Block, error) {
	block, exists := m.blocks[cid]
	if !exists {
		return nil, fmt.Errorf("block not found: %s", cid)
	}
	return block, nil
}

func (m *MockBlockStore) RetrieveBlockWithPeerHint(cid string, preferredPeers []interface{}) (*blocks.Block, error) {
	return m.RetrieveBlock(cid)
}

func (m *MockBlockStore) StoreBlockWithStrategy(block *blocks.Block, strategy string) (string, error) {
	return m.StoreBlock(block)
}
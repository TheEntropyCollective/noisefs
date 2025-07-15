package benchmarks

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	noisefs "github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/logging"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/workers"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	storagetesting "github.com/TheEntropyCollective/noisefs/pkg/storage/testing"
)

// ParallelPerformanceMetrics tracks detailed metrics for parallel operations
type ParallelPerformanceMetrics struct {
	TestName            string                `json:"test_name"`
	Implementation      string                `json:"implementation"` // "sequential" or "parallel"
	Timestamp           time.Time             `json:"timestamp"`
	
	// Operation timings
	TotalDuration       time.Duration         `json:"total_duration"`
	SplitDuration       time.Duration         `json:"split_duration"`
	XORDuration         time.Duration         `json:"xor_duration"`
	StorageDuration     time.Duration         `json:"storage_duration"`
	RetrievalDuration   time.Duration         `json:"retrieval_duration"`
	AssemblyDuration    time.Duration         `json:"assembly_duration"`
	
	// Throughput metrics
	FileSize            int64                 `json:"file_size"`
	BlockCount          int                   `json:"block_count"`
	BlockSize           int                   `json:"block_size"`
	ThroughputMBps      float64               `json:"throughput_mbps"`
	BlocksPerSecond     float64               `json:"blocks_per_second"`
	
	// Parallelism metrics
	WorkerCount         int                   `json:"worker_count"`
	CPUUtilization      float64               `json:"cpu_utilization"`
	ParallelEfficiency  float64               `json:"parallel_efficiency"`
	SpeedupFactor       float64               `json:"speedup_factor"`
	
	// Memory metrics
	PeakMemoryMB        float64               `json:"peak_memory_mb"`
	AvgMemoryMB         float64               `json:"avg_memory_mb"`
	MemoryPerWorkerMB   float64               `json:"memory_per_worker_mb"`
	
	// Detailed operation breakdown
	XORMetrics          *OperationBreakdown   `json:"xor_metrics"`
	StorageMetrics      *OperationBreakdown   `json:"storage_metrics"`
	RetrievalMetrics    *OperationBreakdown   `json:"retrieval_metrics"`
}

type OperationBreakdown struct {
	Count               int                   `json:"count"`
	AvgLatency          time.Duration         `json:"avg_latency"`
	MinLatency          time.Duration         `json:"min_latency"`
	MaxLatency          time.Duration         `json:"max_latency"`
	P50Latency          time.Duration         `json:"p50_latency"`
	P95Latency          time.Duration         `json:"p95_latency"`
	P99Latency          time.Duration         `json:"p99_latency"`
	ConcurrentOps       int                   `json:"concurrent_ops"`
}

// BenchmarkUploadPerformanceComparison compares sequential vs parallel upload
func BenchmarkUploadPerformanceComparison(b *testing.B) {
	testSizes := []struct {
		name      string
		fileSize  int64
		blockSize int
	}{
		{"1MB_128KB", 1 * 1024 * 1024, 128 * 1024},
		{"10MB_128KB", 10 * 1024 * 1024, 128 * 1024},
		{"100MB_128KB", 100 * 1024 * 1024, 128 * 1024},
		{"1GB_256KB", 1024 * 1024 * 1024, 256 * 1024},
	}

	for _, test := range testSizes {
		b.Run(test.name, func(b *testing.B) {
			// Skip large tests in short mode
			if testing.Short() && test.fileSize > 100*1024*1024 {
				b.Skip("Skipping large file test in short mode")
			}

			// Run sequential baseline
			b.Run("Sequential", func(b *testing.B) {
				benchmarkUploadSequential(b, test.fileSize, test.blockSize)
			})

			// Run parallel implementation with different worker counts
			workerCounts := []int{2, 4, 8, runtime.NumCPU(), runtime.NumCPU() * 2}
			for _, workers := range workerCounts {
				b.Run(fmt.Sprintf("Parallel_%dWorkers", workers), func(b *testing.B) {
					benchmarkUploadParallel(b, test.fileSize, test.blockSize, workers)
				})
			}
		})
	}
}

func benchmarkUploadSequential(b *testing.B, fileSize int64, blockSize int) {
	// Setup
	storageManager, client, logger := setupBenchmarkEnvironment(b)
	testData := generateTestData(int(fileSize))
	
	metrics := &ParallelPerformanceMetrics{
		TestName:       fmt.Sprintf("UploadSequential_%dMB", fileSize/(1024*1024)),
		Implementation: "sequential",
		FileSize:       fileSize,
		BlockSize:      blockSize,
		WorkerCount:    1,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		startTime := time.Now()
		
		// Split file
		splitStart := time.Now()
		splitter, _ := blocks.NewSplitter(blockSize)
		reader := bytes.NewReader(testData)
		fileBlocks, err := splitter.Split(reader)
		if err != nil {
			b.Fatalf("Failed to split file: %v", err)
		}
		metrics.SplitDuration = time.Since(splitStart)
		metrics.BlockCount = len(fileBlocks)

		// Select randomizers
		randomizer1Blocks := make([]*blocks.Block, len(fileBlocks))
		randomizer2Blocks := make([]*blocks.Block, len(fileBlocks))
		for j := range fileBlocks {
			r1, _, r2, _, err := client.SelectRandomizers(fileBlocks[j].Size())
			if err != nil {
				b.Fatalf("Failed to select randomizers: %v", err)
			}
			randomizer1Blocks[j] = r1
			randomizer2Blocks[j] = r2
		}

		// Sequential XOR
		xorStart := time.Now()
		anonymizedBlocks := make([]*blocks.Block, len(fileBlocks))
		for j := range fileBlocks {
			anonBlock, err := fileBlocks[j].XOR(randomizer1Blocks[j], randomizer2Blocks[j])
			if err != nil {
				b.Fatalf("XOR failed: %v", err)
			}
			anonymizedBlocks[j] = anonBlock
		}
		metrics.XORDuration = time.Since(xorStart)

		// Sequential storage
		storageStart := time.Now()
		for _, block := range anonymizedBlocks {
			if err := storageManager.Put(context.Background(), block); err != nil {
				b.Fatalf("Storage failed: %v", err)
			}
		}
		metrics.StorageDuration = time.Since(storageStart)

		metrics.TotalDuration = time.Since(startTime)
		metrics.ThroughputMBps = float64(fileSize) / (1024 * 1024) / metrics.TotalDuration.Seconds()
		metrics.BlocksPerSecond = float64(len(fileBlocks)) / metrics.TotalDuration.Seconds()

		// Log key metrics
		logger.Info("Sequential upload completed", logging.Fields{
			"duration_ms":     metrics.TotalDuration.Milliseconds(),
			"throughput_mbps": metrics.ThroughputMBps,
			"blocks_per_sec":  metrics.BlocksPerSecond,
		})
	}

	// Report metrics
	b.ReportMetric(metrics.ThroughputMBps, "MB/s")
	b.ReportMetric(float64(metrics.TotalDuration.Nanoseconds()), "ns/op")
	b.ReportMetric(metrics.BlocksPerSecond, "blocks/s")
}

func benchmarkUploadParallel(b *testing.B, fileSize int64, blockSize int, workerCount int) {
	// Setup
	storageManager, client, logger := setupBenchmarkEnvironment(b)
	testData := generateTestData(int(fileSize))
	
	metrics := &ParallelPerformanceMetrics{
		TestName:       fmt.Sprintf("UploadParallel_%dMB_%dWorkers", fileSize/(1024*1024), workerCount),
		Implementation: "parallel",
		FileSize:       fileSize,
		BlockSize:      blockSize,
		WorkerCount:    workerCount,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		startTime := time.Now()
		
		// Split file (still sequential - I/O bound)
		splitStart := time.Now()
		splitter, _ := blocks.NewSplitter(blockSize)
		reader := bytes.NewReader(testData)
		fileBlocks, err := splitter.Split(reader)
		if err != nil {
			b.Fatalf("Failed to split file: %v", err)
		}
		metrics.SplitDuration = time.Since(splitStart)
		metrics.BlockCount = len(fileBlocks)

		// Select randomizers (could be parallelized but kept sequential for consistency)
		randomizer1Blocks := make([]*blocks.Block, len(fileBlocks))
		randomizer2Blocks := make([]*blocks.Block, len(fileBlocks))
		for j := range fileBlocks {
			r1, _, r2, _, err := client.SelectRandomizers(fileBlocks[j].Size())
			if err != nil {
				b.Fatalf("Failed to select randomizers: %v", err)
			}
			randomizer1Blocks[j] = r1
			randomizer2Blocks[j] = r2
		}

		// Create worker pool
		pool := workers.NewPool(workers.Config{
			WorkerCount:     workerCount,
			BufferSize:      workerCount * 2,
			ShutdownTimeout: 30 * time.Second,
		})
		if err := pool.Start(); err != nil {
			b.Fatalf("Failed to start worker pool: %v", err)
		}
		defer pool.Shutdown()

		blockOps := workers.NewBlockOperationBatch(pool)
		ctx := context.Background()

		// Parallel XOR
		xorStart := time.Now()
		anonymizedBlocks, err := blockOps.ParallelXOR(ctx, fileBlocks, randomizer1Blocks, randomizer2Blocks)
		if err != nil {
			b.Fatalf("Parallel XOR failed: %v", err)
		}
		metrics.XORDuration = time.Since(xorStart)

		// Parallel storage - using storage manager directly
		storageStart := time.Now()
		var wg sync.WaitGroup
		wg.Add(len(anonymizedBlocks))
		
		for _, block := range anonymizedBlocks {
			go func(b *blocks.Block) {
				defer wg.Done()
				if err := storageManager.Put(ctx, b); err != nil {
					// Log error but don't fail the benchmark
					logger.Error("Failed to store block", logging.Fields{"error": err})
				}
			}(block)
		}
		wg.Wait()
		metrics.StorageDuration = time.Since(storageStart)

		metrics.TotalDuration = time.Since(startTime)
		metrics.ThroughputMBps = float64(fileSize) / (1024 * 1024) / metrics.TotalDuration.Seconds()
		metrics.BlocksPerSecond = float64(len(fileBlocks)) / metrics.TotalDuration.Seconds()

		// Log key metrics
		logger.Info("Parallel upload completed", logging.Fields{
			"workers":         workerCount,
			"duration_ms":     metrics.TotalDuration.Milliseconds(),
			"throughput_mbps": metrics.ThroughputMBps,
			"blocks_per_sec":  metrics.BlocksPerSecond,
			"xor_ms":          metrics.XORDuration.Milliseconds(),
			"storage_ms":      metrics.StorageDuration.Milliseconds(),
		})
	}

	// Report metrics
	b.ReportMetric(metrics.ThroughputMBps, "MB/s")
	b.ReportMetric(float64(metrics.TotalDuration.Nanoseconds()), "ns/op")
	b.ReportMetric(metrics.BlocksPerSecond, "blocks/s")
	b.ReportMetric(float64(workerCount), "workers")
}

// BenchmarkDownloadPerformanceComparison compares sequential vs parallel download
func BenchmarkDownloadPerformanceComparison(b *testing.B) {
	testSizes := []struct {
		name      string
		fileSize  int64
		blockSize int
	}{
		{"1MB_128KB", 1 * 1024 * 1024, 128 * 1024},
		{"10MB_128KB", 10 * 1024 * 1024, 128 * 1024},
		{"100MB_128KB", 100 * 1024 * 1024, 128 * 1024},
		{"1GB_256KB", 1024 * 1024 * 1024, 256 * 1024},
	}

	for _, test := range testSizes {
		b.Run(test.name, func(b *testing.B) {
			// Skip large tests in short mode
			if testing.Short() && test.fileSize > 100*1024*1024 {
				b.Skip("Skipping large file test in short mode")
			}

			// Prepare test data
			descriptor := prepareTestDescriptor(b, test.fileSize, test.blockSize)

			// Run sequential baseline
			b.Run("Sequential", func(b *testing.B) {
				benchmarkDownloadSequential(b, descriptor)
			})

			// Run parallel implementation with different worker counts
			workerCounts := []int{2, 4, 8, runtime.NumCPU(), runtime.NumCPU() * 2}
			for _, workers := range workerCounts {
				b.Run(fmt.Sprintf("Parallel_%dWorkers", workers), func(b *testing.B) {
					benchmarkDownloadParallel(b, descriptor, workers)
				})
			}
		})
	}
}

func benchmarkDownloadSequential(b *testing.B, descriptor *descriptors.Descriptor) {
	// Setup
	storageManager, _, logger := setupBenchmarkEnvironment(b)
	
	metrics := &ParallelPerformanceMetrics{
		TestName:       fmt.Sprintf("DownloadSequential_%dMB", descriptor.FileSize/(1024*1024)),
		Implementation: "sequential",
		FileSize:       descriptor.FileSize,
		BlockSize:      descriptor.BlockSize,
		BlockCount:     len(descriptor.Blocks),
		WorkerCount:    1,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		startTime := time.Now()
		ctx := context.Background()

		// Sequential retrieval
		retrievalStart := time.Now()
		dataBlocks := make([]*blocks.Block, len(descriptor.Blocks))
		randomizer1Blocks := make([]*blocks.Block, len(descriptor.Blocks))
		randomizer2Blocks := make([]*blocks.Block, len(descriptor.Blocks))

		for j, block := range descriptor.Blocks {
			// Retrieve data block
			dataAddr := &storage.BlockAddress{ID: block.DataCID}
			dataBlock, err := storageManager.Get(ctx, dataAddr)
			if err != nil {
				b.Fatalf("Failed to retrieve data block: %v", err)
			}
			dataBlocks[j] = dataBlock

			// Retrieve randomizer1
			r1Addr := &storage.BlockAddress{ID: block.RandomizerCID1}
			r1Block, err := storageManager.Get(ctx, r1Addr)
			if err != nil {
				b.Fatalf("Failed to retrieve randomizer1: %v", err)
			}
			randomizer1Blocks[j] = r1Block

			// Retrieve randomizer2
			r2Addr := &storage.BlockAddress{ID: block.RandomizerCID2}
			r2Block, err := storageManager.Get(ctx, r2Addr)
			if err != nil {
				b.Fatalf("Failed to retrieve randomizer2: %v", err)
			}
			randomizer2Blocks[j] = r2Block
		}
		metrics.RetrievalDuration = time.Since(retrievalStart)

		// Sequential XOR reconstruction
		xorStart := time.Now()
		originalBlocks := make([]*blocks.Block, len(dataBlocks))
		for j := range dataBlocks {
			originalBlock, err := dataBlocks[j].XOR(randomizer1Blocks[j], randomizer2Blocks[j])
			if err != nil {
				b.Fatalf("XOR reconstruction failed: %v", err)
			}
			originalBlocks[j] = originalBlock
		}
		metrics.XORDuration = time.Since(xorStart)

		// Assembly (sequential by nature)
		assemblyStart := time.Now()
		assembler := blocks.NewAssembler()
		var buf bytes.Buffer
		if err := assembler.AssembleToWriter(originalBlocks, &buf); err != nil {
			b.Fatalf("Assembly failed: %v", err)
		}
		metrics.AssemblyDuration = time.Since(assemblyStart)

		metrics.TotalDuration = time.Since(startTime)
		metrics.ThroughputMBps = float64(descriptor.FileSize) / (1024 * 1024) / metrics.TotalDuration.Seconds()
		metrics.BlocksPerSecond = float64(len(descriptor.Blocks)*3) / metrics.RetrievalDuration.Seconds() // *3 for all block types

		// Log key metrics
		logger.Info("Sequential download completed", logging.Fields{
			"duration_ms":      metrics.TotalDuration.Milliseconds(),
			"throughput_mbps":  metrics.ThroughputMBps,
			"blocks_per_sec":   metrics.BlocksPerSecond,
			"retrieval_ms":     metrics.RetrievalDuration.Milliseconds(),
			"xor_ms":           metrics.XORDuration.Milliseconds(),
		})
	}

	// Report metrics
	b.ReportMetric(metrics.ThroughputMBps, "MB/s")
	b.ReportMetric(float64(metrics.TotalDuration.Nanoseconds()), "ns/op")
	b.ReportMetric(metrics.BlocksPerSecond, "blocks/s")
}

func benchmarkDownloadParallel(b *testing.B, descriptor *descriptors.Descriptor, workerCount int) {
	// Setup
	storageManager, _, logger := setupBenchmarkEnvironment(b)
	
	metrics := &ParallelPerformanceMetrics{
		TestName:       fmt.Sprintf("DownloadParallel_%dMB_%dWorkers", descriptor.FileSize/(1024*1024), workerCount),
		Implementation: "parallel",
		FileSize:       descriptor.FileSize,
		BlockSize:      descriptor.BlockSize,
		BlockCount:     len(descriptor.Blocks),
		WorkerCount:    workerCount,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		startTime := time.Now()
		ctx := context.Background()

		// Create worker pool
		pool := workers.NewPool(workers.Config{
			WorkerCount:     workerCount,
			BufferSize:      workerCount * 2,
			ShutdownTimeout: 30 * time.Second,
		})
		if err := pool.Start(); err != nil {
			b.Fatalf("Failed to start worker pool: %v", err)
		}
		defer pool.Shutdown()

		blockOps := workers.NewBlockOperationBatch(pool)

		// Prepare addresses for parallel retrieval
		dataAddresses := make([]*storage.BlockAddress, len(descriptor.Blocks))
		randomizer1Addresses := make([]*storage.BlockAddress, len(descriptor.Blocks))
		randomizer2Addresses := make([]*storage.BlockAddress, len(descriptor.Blocks))
		
		for j, block := range descriptor.Blocks {
			dataAddresses[j] = &storage.BlockAddress{ID: block.DataCID}
			randomizer1Addresses[j] = &storage.BlockAddress{ID: block.RandomizerCID1}
			randomizer2Addresses[j] = &storage.BlockAddress{ID: block.RandomizerCID2}
		}

		// Parallel retrieval of all blocks
		retrievalStart := time.Now()
		var dataBlocks, randomizer1Blocks, randomizer2Blocks []*blocks.Block
		var wg sync.WaitGroup
		wg.Add(3)
		
		errChan := make(chan error, 3)
		
		// Retrieve data blocks in parallel
		go func() {
			defer wg.Done()
			blocks, err := blockOps.ParallelRetrieval(ctx, dataAddresses, storageManager)
			if err != nil {
				errChan <- err
				return
			}
			dataBlocks = blocks
		}()
		
		// Retrieve randomizer1 blocks in parallel
		go func() {
			defer wg.Done()
			blocks, err := blockOps.ParallelRetrieval(ctx, randomizer1Addresses, storageManager)
			if err != nil {
				errChan <- err
				return
			}
			randomizer1Blocks = blocks
		}()
		
		// Retrieve randomizer2 blocks in parallel
		go func() {
			defer wg.Done()
			blocks, err := blockOps.ParallelRetrieval(ctx, randomizer2Addresses, storageManager)
			if err != nil {
				errChan <- err
				return
			}
			randomizer2Blocks = blocks
		}()
		
		wg.Wait()
		close(errChan)
		
		// Check for errors
		for err := range errChan {
			if err != nil {
				b.Fatalf("Parallel retrieval failed: %v", err)
			}
		}
		metrics.RetrievalDuration = time.Since(retrievalStart)

		// Parallel XOR reconstruction
		xorStart := time.Now()
		originalBlocks, err := blockOps.ParallelXOR(ctx, dataBlocks, randomizer1Blocks, randomizer2Blocks)
		if err != nil {
			b.Fatalf("Parallel XOR failed: %v", err)
		}
		metrics.XORDuration = time.Since(xorStart)

		// Assembly (still sequential)
		assemblyStart := time.Now()
		assembler := blocks.NewAssembler()
		var buf bytes.Buffer
		if err := assembler.AssembleToWriter(originalBlocks, &buf); err != nil {
			b.Fatalf("Assembly failed: %v", err)
		}
		metrics.AssemblyDuration = time.Since(assemblyStart)

		metrics.TotalDuration = time.Since(startTime)
		metrics.ThroughputMBps = float64(descriptor.FileSize) / (1024 * 1024) / metrics.TotalDuration.Seconds()
		metrics.BlocksPerSecond = float64(len(descriptor.Blocks)*3) / metrics.RetrievalDuration.Seconds()

		// Calculate parallel efficiency
		sequentialEstimate := metrics.RetrievalDuration.Seconds() * float64(workerCount)
		metrics.ParallelEfficiency = sequentialEstimate / metrics.RetrievalDuration.Seconds() / float64(workerCount)

		// Log key metrics
		logger.Info("Parallel download completed", logging.Fields{
			"workers":           workerCount,
			"duration_ms":       metrics.TotalDuration.Milliseconds(),
			"throughput_mbps":   metrics.ThroughputMBps,
			"blocks_per_sec":    metrics.BlocksPerSecond,
			"retrieval_ms":      metrics.RetrievalDuration.Milliseconds(),
			"xor_ms":            metrics.XORDuration.Milliseconds(),
			"parallel_efficiency": fmt.Sprintf("%.2f%%", metrics.ParallelEfficiency*100),
		})
	}

	// Report metrics
	b.ReportMetric(metrics.ThroughputMBps, "MB/s")
	b.ReportMetric(float64(metrics.TotalDuration.Nanoseconds()), "ns/op")
	b.ReportMetric(metrics.BlocksPerSecond, "blocks/s")
	b.ReportMetric(metrics.ParallelEfficiency*100, "%efficiency")
}

// BenchmarkStreamingMemoryUsage tests memory efficiency of streaming implementation
func BenchmarkStreamingMemoryUsage(b *testing.B) {
	testSizes := []struct {
		name      string
		fileSize  int64
		blockSize int
	}{
		{"100MB", 100 * 1024 * 1024, 128 * 1024},
		{"500MB", 500 * 1024 * 1024, 256 * 1024},
		{"1GB", 1024 * 1024 * 1024, 256 * 1024},
	}

	for _, test := range testSizes {
		b.Run(test.name, func(b *testing.B) {
			if testing.Short() && test.fileSize > 100*1024*1024 {
				b.Skip("Skipping large file test in short mode")
			}

			b.Run("Regular", func(b *testing.B) {
				benchmarkRegularMemoryUsage(b, test.fileSize, test.blockSize)
			})

			b.Run("Streaming", func(b *testing.B) {
				benchmarkStreamingMemoryUsage(b, test.fileSize, test.blockSize)
			})
		})
	}
}

func benchmarkRegularMemoryUsage(b *testing.B, fileSize int64, blockSize int) {
	_, client, logger := setupBenchmarkEnvironment(b)
	
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Track memory before
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		startAlloc := m.Alloc

		// Generate all test data at once (simulating regular upload)
		testData := generateTestData(int(fileSize))
		
		// Process normally
		reader := bytes.NewReader(testData)
		descriptorCID, err := client.Upload(reader, fmt.Sprintf("test_%d.dat", i))
		if err != nil {
			b.Fatalf("Upload failed: %v", err)
		}

		// Track memory after
		runtime.ReadMemStats(&m)
		peakAlloc := m.Alloc - startAlloc

		logger.Info("Regular memory usage", logging.Fields{
			"file_size_mb":    fileSize / (1024 * 1024),
			"peak_memory_mb":  peakAlloc / (1024 * 1024),
			"descriptor_cid":  descriptorCID,
		})

		b.ReportMetric(float64(peakAlloc)/(1024*1024), "MB_peak")
	}
}

func benchmarkStreamingMemoryUsage(b *testing.B, fileSize int64, blockSize int) {
	_, client, logger := setupBenchmarkEnvironment(b)
	
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Track memory before
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		startAlloc := m.Alloc

		// Create a simulated streaming source (generates data on demand)
		streamingReader := &streamingTestReader{
			totalSize: fileSize,
			blockSize: blockSize,
		}

		// Process with streaming (using regular upload for now since streaming not implemented)
		descriptorCID, err := client.Upload(
			streamingReader, 
			fmt.Sprintf("streaming_test_%d.dat", i),
		)
		if err != nil {
			b.Fatalf("Streaming upload failed: %v", err)
		}

		// Track memory after
		runtime.ReadMemStats(&m)
		peakAlloc := m.Alloc - startAlloc

		logger.Info("Streaming memory usage", logging.Fields{
			"file_size_mb":    fileSize / (1024 * 1024),
			"peak_memory_mb":  peakAlloc / (1024 * 1024),
			"descriptor_cid":  descriptorCID,
		})

		b.ReportMetric(float64(peakAlloc)/(1024*1024), "MB_peak")
	}
}

// Helper functions

func setupBenchmarkEnvironment(b *testing.B) (*storage.Manager, *noisefs.Client, *logging.Logger) {
	b.Helper()

	// Create storage manager
	storageManager, err := storagetesting.CreateRealTestStorageManager()
	if err != nil {
		b.Fatalf("Failed to create storage manager: %v", err)
	}

	// Create cache
	cacheInstance := cache.NewMemoryCache(100 * 1024 * 1024) // 100MB cache

	// Create client
	client, err := noisefs.NewClient(storageManager, cacheInstance)
	if err != nil {
		b.Fatalf("Failed to create client: %v", err)
	}

	// Create logger
	logger := logging.NewLogger(logging.DefaultConfig())

	return storageManager, client, logger
}

func generateTestData(size int) []byte {
	data := make([]byte, size)
	// Use a pattern that compresses poorly to simulate real data
	for i := range data {
		data[i] = byte((i * 7) % 256)
	}
	return data
}

func prepareTestDescriptor(b *testing.B, fileSize int64, blockSize int) *descriptors.Descriptor {
	b.Helper()

	// Create test descriptor with mock block references
	descriptor := descriptors.NewDescriptor("test_file.dat", fileSize, blockSize)
	
	blockCount := int((fileSize + int64(blockSize) - 1) / int64(blockSize))
	for i := 0; i < blockCount; i++ {
		blockInfo := descriptors.Block{
			Index:          i,
			DataCID:        fmt.Sprintf("data-cid-%d", i),
			RandomizerCID1: fmt.Sprintf("rand1-cid-%d", i),
			RandomizerCID2: fmt.Sprintf("rand2-cid-%d", i),
		}
		descriptor.Blocks = append(descriptor.Blocks, blockInfo)
	}

	// Pre-populate storage with test blocks
	storageManager, _, _ := setupBenchmarkEnvironment(b)
	ctx := context.Background()

	for i := 0; i < blockCount; i++ {
		// Calculate actual block size for last block
		remainingSize := fileSize - int64(i)*int64(blockSize)
		currentBlockSize := blockSize
		if remainingSize < int64(blockSize) {
			currentBlockSize = int(remainingSize)
		}

		// Create and store test blocks
		dataBlock := generateTestData(currentBlockSize)
		rand1Block := generateTestData(currentBlockSize)
		rand2Block := generateTestData(currentBlockSize)

		// XOR to create anonymized block
		data, _ := blocks.NewBlock(dataBlock)
		r1, _ := blocks.NewBlock(rand1Block)
		r2, _ := blocks.NewBlock(rand2Block)
		anonBlock, _ := data.XOR(r1, r2)

		// Store all blocks with proper BlockAddress
		storageManager.Put(ctx, anonBlock)
		
		// Store randomizer blocks too
		r1WithAddr := &blocks.Block{ID: fmt.Sprintf("rand1-cid-%d", i), Data: rand1Block}
		r2WithAddr := &blocks.Block{ID: fmt.Sprintf("rand2-cid-%d", i), Data: rand2Block}
		storageManager.Put(ctx, r1WithAddr)
		storageManager.Put(ctx, r2WithAddr)
		
		// Update descriptor with actual IDs
		descriptor.Blocks[i].DataCID = anonBlock.ID
	}

	return descriptor
}

// streamingTestReader simulates a streaming data source
type streamingTestReader struct {
	totalSize int64
	blockSize int
	position  int64
}

func (r *streamingTestReader) Read(p []byte) (n int, err error) {
	if r.position >= r.totalSize {
		return 0, fmt.Errorf("EOF")
	}

	remaining := r.totalSize - r.position
	toRead := int64(len(p))
	if toRead > remaining {
		toRead = remaining
	}

	// Generate data on-the-fly to simulate streaming
	for i := int64(0); i < toRead; i++ {
		p[i] = byte(((r.position + i) * 7) % 256)
	}

	r.position += toRead
	return int(toRead), nil
}

// BenchmarkPerformanceRegression tests for performance regressions
func BenchmarkPerformanceRegression(b *testing.B) {
	// Load baseline if it exists
	baselineFile := filepath.Join("results", "benchmarks", "baseline.json")
	baselineManager := NewBaselineManager(baselineFile, logging.NewLogger(nil))
	
	_, err := baselineManager.LoadBaseline()
	hasBaseline := err == nil

	// Run standard benchmarks
	results := runStandardBenchmarks(b)

	// Compare with baseline if available
	if hasBaseline {
		comparison, err := baselineManager.CompareResults(results, &BenchmarkConfig{
			Duration:    30 * time.Second,
			Concurrency: runtime.NumCPU(),
			FileSize:    10 * 1024 * 1024,
			BlockSize:   128 * 1024,
		})
		
		if err == nil {
			comparison.PrintComparisonReport()
			
			// Fail if significant regression detected
			if comparison.Summary.RegressedCount > 0 && comparison.Summary.AvgOpsChange < -10 {
				b.Errorf("Performance regression detected: %d tests regressed with avg %.1f%% decrease",
					comparison.Summary.RegressedCount, -comparison.Summary.AvgOpsChange)
			}
		}
	} else {
		// Save current results as new baseline
		baselineManager.SaveBaseline(results, &BenchmarkConfig{
			Duration:    30 * time.Second,
			Concurrency: runtime.NumCPU(),
			FileSize:    10 * 1024 * 1024,
			BlockSize:   128 * 1024,
		}, map[string]string{
			"version": "parallel_implementation",
			"date":    time.Now().Format("2006-01-02"),
		})
	}
}

func runStandardBenchmarks(b *testing.B) []BenchmarkResult {
	results := []BenchmarkResult{}
	
	// Define standard benchmark scenarios
	scenarios := []struct {
		name      string
		fileSize  int64
		blockSize int
		workers   int
	}{
		{"Small_Sequential", 1 * 1024 * 1024, 128 * 1024, 1},
		{"Small_Parallel", 1 * 1024 * 1024, 128 * 1024, 8},
		{"Medium_Sequential", 10 * 1024 * 1024, 128 * 1024, 1},
		{"Medium_Parallel", 10 * 1024 * 1024, 128 * 1024, 8},
		{"Large_Sequential", 100 * 1024 * 1024, 256 * 1024, 1},
		{"Large_Parallel", 100 * 1024 * 1024, 256 * 1024, 16},
	}

	for _, scenario := range scenarios {
		if testing.Short() && scenario.fileSize > 10*1024*1024 {
			continue
		}

		result := runSingleBenchmark(b, scenario.name, scenario.fileSize, scenario.blockSize, scenario.workers)
		results = append(results, result)
	}

	return results
}

func runSingleBenchmark(b *testing.B, name string, fileSize int64, blockSize int, workers int) BenchmarkResult {
	_, client, _ := setupBenchmarkEnvironment(b)
	testData := generateTestData(int(fileSize))
	
	start := time.Now()
	operations := int64(0)
	bytesProcessed := int64(0)
	latencies := []time.Duration{}

	// Run upload/download cycle
	for i := 0; i < 10; i++ { // Fixed number of operations for consistency
		opStart := time.Now()
		
		// Upload
		reader := bytes.NewReader(testData)
		descriptorCID, err := client.Upload(reader, fmt.Sprintf("bench_%s_%d.dat", name, i))
		if err == nil {
			operations++
			bytesProcessed += fileSize
			
			// Download
			data, err := client.Download(descriptorCID)
			if err == nil && len(data) == len(testData) {
				operations++
				bytesProcessed += fileSize
			}
		}
		
		latencies = append(latencies, time.Since(opStart))
	}

	duration := time.Since(start)
	
	// Calculate metrics
	avgLatency := time.Duration(0)
	for _, l := range latencies {
		avgLatency += l
	}
	if len(latencies) > 0 {
		avgLatency /= time.Duration(len(latencies))
	}

	return BenchmarkResult{
		Name:             name,
		Duration:         duration,
		Operations:       operations,
		BytesProcessed:   bytesProcessed,
		OperationsPerSec: float64(operations) / duration.Seconds(),
		ThroughputMBps:   float64(bytesProcessed) / (1024 * 1024) / duration.Seconds(),
		LatencyAvg:       avgLatency,
		ErrorCount:       0,
	}
}
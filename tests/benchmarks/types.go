package benchmarks

import "time"

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
package cache

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/logging"
)

// PerformanceMetrics tracks detailed cache performance characteristics
type PerformanceMetrics struct {
	mu sync.RWMutex
	
	// Operation timing
	GetLatencies    []time.Duration
	StoreLatencies  []time.Duration
	EvictLatencies  []time.Duration
	
	// Throughput tracking
	OperationsPerSecond float64
	PeakThroughput      float64
	LastMeasurement     time.Time
	TotalOperations     int64
	
	// Memory usage
	HeapBytes       uint64
	StackBytes      uint64
	GCPauses        []time.Duration
	NumGoroutines   int
	
	// Cache effectiveness
	TemporalHitRate map[time.Duration]float64 // Hit rate over different time windows
	SizeEfficiency  float64                   // Actual/theoretical optimal cache size
	
	// Performance regression detection
	BaselineMetrics *PerformanceSnapshot
	RegressionAlert bool
	RegressionRatio float64
}

// PerformanceSnapshot captures cache performance at a point in time
type PerformanceSnapshot struct {
	Timestamp           time.Time     `json:"timestamp"`
	AvgGetLatency       time.Duration `json:"avg_get_latency"`
	AvgStoreLatency     time.Duration `json:"avg_store_latency"`
	AvgEvictLatency     time.Duration `json:"avg_evict_latency"`
	OperationsPerSecond float64       `json:"operations_per_second"`
	HitRate             float64       `json:"hit_rate"`
	MemoryUsageMB       float64       `json:"memory_usage_mb"`
	NumGoroutines       int           `json:"num_goroutines"`
	CacheUtilization    float64       `json:"cache_utilization"`
}

// PerformanceMonitor wraps a cache with detailed performance monitoring
type PerformanceMonitor struct {
	underlying Cache
	metrics    *PerformanceMetrics
	logger     *logging.Logger
	
	// Monitoring configuration
	sampleSize       int           // Number of latency samples to keep
	measureInterval  time.Duration // How often to calculate throughput
	regressionFactor float64       // Threshold for performance regression (e.g., 1.2 = 20% worse)
	
	// Atomic counters for high-frequency operations
	opCounter    int64
	lastOpTime   int64 // Unix nano timestamp
}

// NewPerformanceMonitor creates a new performance monitoring wrapper
func NewPerformanceMonitor(underlying Cache, logger *logging.Logger) *PerformanceMonitor {
	pm := &PerformanceMonitor{
		underlying:      underlying,
		logger:          logger,
		sampleSize:      1000,
		measureInterval: 10 * time.Second,
		regressionFactor: 1.2, // 20% performance degradation threshold
		metrics: &PerformanceMetrics{
			TemporalHitRate: make(map[time.Duration]float64),
			LastMeasurement: time.Now(),
		},
	}
	
	// Start monitoring goroutines
	go pm.monitorThroughput()
	go pm.monitorMemory()
	
	return pm
}

// Store adds a block to the cache with performance monitoring
func (pm *PerformanceMonitor) Store(cid string, block *blocks.Block) error {
	start := time.Now()
	
	err := pm.underlying.Store(cid, block)
	latency := time.Since(start)
	
	pm.recordLatency("store", latency)
	pm.recordOperation()
	
	return err
}

// Get retrieves a block from the cache with performance monitoring  
func (pm *PerformanceMonitor) Get(cid string) (*blocks.Block, error) {
	start := time.Now()
	
	block, err := pm.underlying.Get(cid)
	latency := time.Since(start)
	
	pm.recordLatency("get", latency)
	pm.recordOperation()
	
	return block, err
}

// Has checks if a block exists in the cache
func (pm *PerformanceMonitor) Has(cid string) bool {
	return pm.underlying.Has(cid)
}

// Remove removes a block from the cache
func (pm *PerformanceMonitor) Remove(cid string) error {
	start := time.Now()
	
	err := pm.underlying.Remove(cid)
	latency := time.Since(start)
	
	pm.recordLatency("evict", latency)
	pm.recordOperation()
	
	return err
}

// GetRandomizers returns popular blocks suitable as randomizers
func (pm *PerformanceMonitor) GetRandomizers(count int) ([]*BlockInfo, error) {
	return pm.underlying.GetRandomizers(count)
}

// IncrementPopularity increases the popularity score of a block
func (pm *PerformanceMonitor) IncrementPopularity(cid string) error {
	return pm.underlying.IncrementPopularity(cid)
}

// Size returns the number of blocks in the cache
func (pm *PerformanceMonitor) Size() int {
	return pm.underlying.Size()
}

// Clear removes all blocks from the cache
func (pm *PerformanceMonitor) Clear() {
	pm.underlying.Clear()
	pm.resetMetrics()
}

// GetStats returns basic cache statistics
func (pm *PerformanceMonitor) GetStats() *Stats {
	return pm.underlying.GetStats()
}

// recordLatency records operation latency
func (pm *PerformanceMonitor) recordLatency(opType string, latency time.Duration) {
	pm.metrics.mu.Lock()
	defer pm.metrics.mu.Unlock()
	
	switch opType {
	case "get":
		pm.metrics.GetLatencies = append(pm.metrics.GetLatencies, latency)
		if len(pm.metrics.GetLatencies) > pm.sampleSize {
			pm.metrics.GetLatencies = pm.metrics.GetLatencies[1:]
		}
	case "store":
		pm.metrics.StoreLatencies = append(pm.metrics.StoreLatencies, latency)
		if len(pm.metrics.StoreLatencies) > pm.sampleSize {
			pm.metrics.StoreLatencies = pm.metrics.StoreLatencies[1:]
		}
	case "evict":
		pm.metrics.EvictLatencies = append(pm.metrics.EvictLatencies, latency)
		if len(pm.metrics.EvictLatencies) > pm.sampleSize {
			pm.metrics.EvictLatencies = pm.metrics.EvictLatencies[1:]
		}
	}
}

// recordOperation increments operation counter
func (pm *PerformanceMonitor) recordOperation() {
	atomic.AddInt64(&pm.opCounter, 1)
	atomic.StoreInt64(&pm.lastOpTime, time.Now().UnixNano())
}

// monitorThroughput calculates operations per second
func (pm *PerformanceMonitor) monitorThroughput() {
	ticker := time.NewTicker(pm.measureInterval)
	defer ticker.Stop()
	
	lastOps := int64(0)
	lastTime := time.Now()
	
	for range ticker.C {
		currentOps := atomic.LoadInt64(&pm.opCounter)
		currentTime := time.Now()
		
		elapsed := currentTime.Sub(lastTime).Seconds()
		if elapsed > 0 {
			currentThroughput := float64(currentOps-lastOps) / elapsed
			
			pm.metrics.mu.Lock()
			pm.metrics.OperationsPerSecond = currentThroughput
			if currentThroughput > pm.metrics.PeakThroughput {
				pm.metrics.PeakThroughput = currentThroughput
			}
			pm.metrics.LastMeasurement = currentTime
			pm.metrics.TotalOperations = currentOps
			pm.metrics.mu.Unlock()
			
			// Check for performance regression
			pm.checkRegression()
		}
		
		lastOps = currentOps
		lastTime = currentTime
	}
}

// monitorMemory tracks memory usage
func (pm *PerformanceMonitor) monitorMemory() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)
		
		pm.metrics.mu.Lock()
		pm.metrics.HeapBytes = memStats.HeapAlloc
		pm.metrics.StackBytes = memStats.StackInuse
		pm.metrics.NumGoroutines = runtime.NumGoroutine()
		
		// Track GC pauses
		if len(memStats.PauseNs) > 0 {
			recentPause := time.Duration(memStats.PauseNs[(memStats.NumGC+255)%256])
			pm.metrics.GCPauses = append(pm.metrics.GCPauses, recentPause)
			if len(pm.metrics.GCPauses) > 100 { // Keep last 100 GC pauses
				pm.metrics.GCPauses = pm.metrics.GCPauses[1:]
			}
		}
		pm.metrics.mu.Unlock()
	}
}

// checkRegression detects performance regressions
func (pm *PerformanceMonitor) checkRegression() {
	pm.metrics.mu.Lock()
	defer pm.metrics.mu.Unlock()
	
	if pm.metrics.BaselineMetrics == nil {
		// Set initial baseline
		pm.metrics.BaselineMetrics = pm.getSnapshotUnsafe()
		return
	}
	
	current := pm.getSnapshotUnsafe()
	baseline := pm.metrics.BaselineMetrics
	
	// Calculate regression ratio for key metrics
	var regressionRatio float64
	
	if baseline.AvgGetLatency > 0 {
		latencyRatio := float64(current.AvgGetLatency) / float64(baseline.AvgGetLatency)
		regressionRatio = max(regressionRatio, latencyRatio)
	}
	
	if baseline.OperationsPerSecond > 0 {
		throughputRatio := baseline.OperationsPerSecond / current.OperationsPerSecond
		regressionRatio = max(regressionRatio, throughputRatio)
	}
	
	pm.metrics.RegressionRatio = regressionRatio
	pm.metrics.RegressionAlert = regressionRatio > pm.regressionFactor
	
	if pm.metrics.RegressionAlert {
		pm.logger.Warn("Performance regression detected", map[string]interface{}{
			"regression_ratio":      regressionRatio,
			"current_get_latency":   current.AvgGetLatency.String(),
			"baseline_get_latency":  baseline.AvgGetLatency.String(),
			"current_throughput":    current.OperationsPerSecond,
			"baseline_throughput":   baseline.OperationsPerSecond,
		})
	}
}

// GetPerformanceSnapshot returns current performance metrics
func (pm *PerformanceMonitor) GetPerformanceSnapshot() *PerformanceSnapshot {
	pm.metrics.mu.RLock()
	defer pm.metrics.mu.RUnlock()
	
	return pm.getSnapshotUnsafe()
}

// getSnapshotUnsafe returns performance snapshot without locking (must be called with lock held)
func (pm *PerformanceMonitor) getSnapshotUnsafe() *PerformanceSnapshot {
	stats := pm.underlying.GetStats()
	
	var avgGetLatency, avgStoreLatency, avgEvictLatency time.Duration
	
	if len(pm.metrics.GetLatencies) > 0 {
		var total time.Duration
		for _, latency := range pm.metrics.GetLatencies {
			total += latency
		}
		avgGetLatency = total / time.Duration(len(pm.metrics.GetLatencies))
	}
	
	if len(pm.metrics.StoreLatencies) > 0 {
		var total time.Duration
		for _, latency := range pm.metrics.StoreLatencies {
			total += latency
		}
		avgStoreLatency = total / time.Duration(len(pm.metrics.StoreLatencies))
	}
	
	if len(pm.metrics.EvictLatencies) > 0 {
		var total time.Duration
		for _, latency := range pm.metrics.EvictLatencies {
			total += latency
		}
		avgEvictLatency = total / time.Duration(len(pm.metrics.EvictLatencies))
	}
	
	var hitRate float64
	if stats.Hits+stats.Misses > 0 {
		hitRate = float64(stats.Hits) / float64(stats.Hits+stats.Misses)
	}
	
	return &PerformanceSnapshot{
		Timestamp:           time.Now(),
		AvgGetLatency:       avgGetLatency,
		AvgStoreLatency:     avgStoreLatency,
		AvgEvictLatency:     avgEvictLatency,
		OperationsPerSecond: pm.metrics.OperationsPerSecond,
		HitRate:             hitRate,
		MemoryUsageMB:       float64(pm.metrics.HeapBytes) / (1024 * 1024),
		NumGoroutines:       pm.metrics.NumGoroutines,
		CacheUtilization:    float64(stats.Size) / float64(stats.Size+1), // Avoid division by zero
	}
}

// SetBaseline sets a new performance baseline
func (pm *PerformanceMonitor) SetBaseline() {
	pm.metrics.mu.Lock()
	defer pm.metrics.mu.Unlock()
	
	pm.metrics.BaselineMetrics = pm.getSnapshotUnsafe()
	pm.metrics.RegressionAlert = false
	pm.metrics.RegressionRatio = 1.0
	
	pm.logger.Info("Performance baseline set", map[string]interface{}{
		"avg_get_latency":    pm.metrics.BaselineMetrics.AvgGetLatency.String(),
		"operations_per_sec": pm.metrics.BaselineMetrics.OperationsPerSecond,
		"hit_rate":           pm.metrics.BaselineMetrics.HitRate,
	})
}

// GetRegressionStatus returns current regression detection status
func (pm *PerformanceMonitor) GetRegressionStatus() (bool, float64, string) {
	pm.metrics.mu.RLock()
	defer pm.metrics.mu.RUnlock()
	
	message := "Performance within normal parameters"
	if pm.metrics.RegressionAlert {
		message = fmt.Sprintf("Performance regression detected: %.1fx worse than baseline", 
			pm.metrics.RegressionRatio)
	}
	
	return pm.metrics.RegressionAlert, pm.metrics.RegressionRatio, message
}

// resetMetrics resets all performance metrics
func (pm *PerformanceMonitor) resetMetrics() {
	pm.metrics.mu.Lock()
	defer pm.metrics.mu.Unlock()
	
	pm.metrics.GetLatencies = nil
	pm.metrics.StoreLatencies = nil
	pm.metrics.EvictLatencies = nil
	pm.metrics.OperationsPerSecond = 0
	pm.metrics.PeakThroughput = 0
	pm.metrics.TotalOperations = 0
	pm.metrics.BaselineMetrics = nil
	pm.metrics.RegressionAlert = false
	pm.metrics.RegressionRatio = 1.0
	
	atomic.StoreInt64(&pm.opCounter, 0)
}

// LogPerformanceReport logs detailed performance information
func (pm *PerformanceMonitor) LogPerformanceReport() {
	snapshot := pm.GetPerformanceSnapshot()
	alert, ratio, message := pm.GetRegressionStatus()
	
	pm.logger.Info("Cache performance report", map[string]interface{}{
		"avg_get_latency":       snapshot.AvgGetLatency.String(),
		"avg_store_latency":     snapshot.AvgStoreLatency.String(),
		"avg_evict_latency":     snapshot.AvgEvictLatency.String(),
		"operations_per_second": snapshot.OperationsPerSecond,
		"hit_rate":              snapshot.HitRate,
		"memory_usage_mb":       snapshot.MemoryUsageMB,
		"num_goroutines":        snapshot.NumGoroutines,
		"cache_utilization":     snapshot.CacheUtilization,
		"regression_alert":      alert,
		"regression_ratio":      ratio,
		"regression_message":    message,
	})
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
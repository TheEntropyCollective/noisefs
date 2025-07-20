// Package noisefs provides comprehensive performance metrics and health monitoring functionality.
// This file handles tracking of upload/download statistics, cache efficiency, storage overhead,
// availability integration, and system health metrics for NoiseFS operations.
//
// The metrics system provides multi-dimensional monitoring:
//   - Operational metrics: Upload/download counts and throughput
//   - Storage efficiency: Block reuse rates and storage overhead analysis
//   - Cache performance: Hit/miss ratios and cache health monitoring
//   - System health: Memory pressure, eviction rates, and coordination health
//   - Availability metrics: Randomizer diversity and availability scores
//   - Performance trends: Historical analysis and optimization insights
//
// Key Features:
//   - Thread-safe metrics collection with reader-writer locks
//   - Real-time health monitoring with cache health scores
//   - Storage efficiency analysis for overhead optimization
//   - Integration with availability and coordination systems
//   - JSON-serializable snapshots for external monitoring
//   - Human-readable health summaries for debugging
package noisefs

import (
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
)

// Metrics tracks comprehensive NoiseFS performance, efficiency, and health metrics.
// This structure provides thread-safe collection and analysis of system performance data,
// enabling monitoring, optimization, and troubleshooting of NoiseFS operations.
//
// The metrics system tracks multiple performance dimensions:
//   - Block management: Reuse vs generation for storage efficiency
//   - Cache performance: Hit/miss ratios for access optimization
//   - Upload/download throughput: Operational volume and efficiency
//   - Storage efficiency: Overhead analysis and optimization insights
//   - System health: Cache health, memory pressure, and coordination status
//
// Thread Safety:
//   All metrics operations are protected by a reader-writer mutex, enabling
//   concurrent read access while ensuring consistent updates during writes.
//
// Health Monitoring:
//   Integrated cache health monitor provides real-time assessment of system
//   health including memory pressure, eviction rates, and availability metrics.
//
// Time Complexity: O(1) for all metric operations
// Space Complexity: O(1) - constant memory overhead for counters and health monitoring
type Metrics struct {
	mu                    sync.RWMutex // Reader-writer mutex for thread-safe metrics access
	BlocksReused          int64        // Number of blocks reused from cache for efficiency tracking
	BlocksGenerated       int64        // Number of new blocks generated for overhead analysis
	CacheHits             int64        // Number of cache hits for performance monitoring
	CacheMisses           int64        // Number of cache misses for optimization insights
	TotalUploads          int64        // Total files uploaded for throughput analysis
	TotalDownloads        int64        // Total files downloaded for usage tracking
	BytesUploadedOriginal int64        // Original bytes uploaded for efficiency calculation
	BytesStoredIPFS       int64        // Actual bytes stored in distributed storage for overhead tracking

	// Health monitoring integration for system health assessment
	healthMonitor *cache.CacheHealthMonitor // Cache health monitor for real-time health assessment
	startTime     time.Time                 // Metrics collection start time for duration calculations
}

// NewMetrics creates a new comprehensive metrics tracker with health monitoring.
// This constructor initializes a complete metrics collection system with integrated
// health monitoring, providing immediate tracking of NoiseFS performance and system health.
//
// Initialization Features:
//   - Thread-safe metrics collection with zero initial values
//   - Integrated cache health monitor for real-time health assessment
//   - Start time tracking for duration-based calculations
//   - Ready for immediate metric collection and analysis
//
// The metrics tracker is designed for long-running operation and provides
// comprehensive monitoring capabilities from the moment of creation.
//
// Returns:
//   - *Metrics: Fully initialized metrics tracker ready for operation
//
// Call Flow:
//   - Called by: Client initialization, metrics system setup
//   - Calls: Cache health monitor constructor for health tracking integration
//
// Time Complexity: O(1) - simple initialization
// Space Complexity: O(1) - constant memory allocation for metrics tracking
func NewMetrics() *Metrics {
	return &Metrics{
		healthMonitor: cache.NewCacheHealthMonitor(), // Initialize health monitoring system
		startTime:     time.Now(),                    // Record metrics collection start time
	}
}

// RecordBlockReuse increments the block reuse counter for storage efficiency tracking.
// This method records when a block is successfully reused from cache instead of being
// generated or retrieved from network storage, providing key insights into storage efficiency.
//
// Block reuse is a critical metric for NoiseFS performance because:
//   - Higher reuse rates indicate better cache efficiency
//   - Reduced network traffic through intelligent block management
//   - Lower storage overhead through optimal randomizer selection
//   - Improved system performance through cache optimization
//
// The metric is used for calculating block reuse rates and storage efficiency
// ratios, enabling optimization of caching strategies and randomizer selection.
//
// Call Flow:
//   - Called by: Cache hit operations, randomizer selection, block retrieval
//   - Updates: BlocksReused counter for efficiency calculations
//
// Time Complexity: O(1) - simple atomic counter increment
// Space Complexity: O(1) - no additional memory allocation
func (m *Metrics) RecordBlockReuse() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.BlocksReused++
}

// RecordBlockGeneration increments the block generation counter for overhead analysis.
// This method records when a new block is generated rather than reused from cache,
// providing insights into storage overhead and cache miss patterns.
//
// Block generation tracking is important for:
//   - Storage overhead analysis and optimization
//   - Cache effectiveness evaluation
//   - Network traffic pattern analysis
//   - System capacity planning and resource allocation
//
// Higher generation rates may indicate:
//   - Cache capacity limitations requiring adjustment
//   - Suboptimal randomizer selection strategies
//   - Opportunities for cache policy optimization
//   - Need for adaptive caching improvements
//
// Call Flow:
//   - Called by: Block creation operations, randomizer generation, cache miss handling
//   - Updates: BlocksGenerated counter for efficiency calculations
//
// Time Complexity: O(1) - simple atomic counter increment
// Space Complexity: O(1) - no additional memory allocation
func (m *Metrics) RecordBlockGeneration() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.BlocksGenerated++
}

// RecordCacheHit increments the cache hit counter for performance monitoring.
// This method records successful cache retrievals, providing crucial insights into
// cache effectiveness and system performance optimization opportunities.
//
// Cache hits are fundamental performance indicators for:
//   - System responsiveness through reduced retrieval latency
//   - Network efficiency by avoiding redundant data transfers
//   - Storage system load reduction through intelligent caching
//   - User experience improvement through faster access patterns
//
// Cache hit metrics enable:
//   - Cache hit ratio calculations for performance analysis
//   - Cache policy effectiveness evaluation
//   - System optimization and tuning decisions
//   - Performance trend analysis and capacity planning
//
// Call Flow:
//   - Called by: Cache retrieval operations, block access functions, storage management
//   - Updates: CacheHits counter for hit rate calculations
//
// Time Complexity: O(1) - simple atomic counter increment
// Space Complexity: O(1) - no additional memory allocation
func (m *Metrics) RecordCacheHit() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CacheHits++
}

// RecordCacheMiss increments the cache miss counter for optimization insights.
// This method records cache retrieval failures, providing critical data for
// cache optimization, capacity planning, and performance troubleshooting.
//
// Cache misses indicate opportunities for:
//   - Cache capacity expansion to improve hit rates
//   - Cache policy optimization for better retention
//   - Preloading strategies to anticipate future needs
//   - Network optimization to handle miss scenarios efficiently
//
// Cache miss analysis enables:
//   - Cache hit ratio calculations for performance assessment
//   - Cache effectiveness evaluation and policy tuning
//   - Capacity planning and resource allocation decisions
//   - Performance bottleneck identification and resolution
//
// High cache miss rates may indicate:
//   - Insufficient cache capacity for workload patterns
//   - Suboptimal eviction policies requiring adjustment
//   - Access patterns not well-suited to current cache strategy
//   - Need for adaptive or predictive caching improvements
//
// Call Flow:
//   - Called by: Cache retrieval operations, storage fallback, network retrieval
//   - Updates: CacheMisses counter for hit rate calculations
//
// Time Complexity: O(1) - simple atomic counter increment
// Space Complexity: O(1) - no additional memory allocation
func (m *Metrics) RecordCacheMiss() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CacheMisses++
}

// RecordUpload records a file upload with comprehensive storage efficiency tracking.
// This method captures both operational metrics and storage overhead data,
// enabling detailed analysis of upload performance and storage efficiency.
//
// Upload Metrics Captured:
//   - Upload operation count for throughput analysis
//   - Original file size for baseline efficiency calculations
//   - Actual storage used including anonymization overhead
//   - Storage efficiency ratios for optimization insights
//
// Storage Efficiency Analysis:
//   - Tracks overhead from 3-tuple XOR anonymization
//   - Measures impact of block padding and randomizer storage
//   - Enables optimization of block size and caching strategies
//   - Provides data for capacity planning and cost analysis
//
// The metrics enable calculation of:
//   - Upload throughput and performance trends
//   - Storage overhead percentages and optimization opportunities
//   - System efficiency and resource utilization patterns
//   - Cost analysis and capacity planning insights
//
// Parameters:
//   - originalBytes: Original file size before processing (baseline for efficiency)
//   - storedBytes: Actual bytes stored including overhead (for efficiency calculation)
//
// Call Flow:
//   - Called by: Upload operations, file processing, storage workflows
//   - Updates: Upload counters and byte tracking for efficiency analysis
//
// Time Complexity: O(1) - simple counter and accumulator updates
// Space Complexity: O(1) - no additional memory allocation
func (m *Metrics) RecordUpload(originalBytes, storedBytes int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TotalUploads++
	m.BytesUploadedOriginal += originalBytes
	m.BytesStoredIPFS += storedBytes
}

// RecordDownload increments the download counter for throughput monitoring.
// This method tracks download operation frequency, providing insights into
// system usage patterns and throughput characteristics.
//
// Download metrics are essential for:
//   - System usage analysis and pattern identification
//   - Throughput monitoring and performance assessment
//   - Capacity planning and resource allocation
//   - User experience analysis and optimization
//
// Download tracking enables:
//   - Upload/download ratio analysis for system balance
//   - Usage pattern identification for optimization
//   - Performance trend analysis and forecasting
//   - System health monitoring and alerting
//
// The metric provides foundation for:
//   - System load analysis and capacity planning
//   - Performance benchmarking and optimization
//   - Usage pattern analysis for cache optimization
//   - Network traffic analysis and optimization
//
// Call Flow:
//   - Called by: Download operations, file retrieval, streaming downloads
//   - Updates: TotalDownloads counter for throughput analysis
//
// Time Complexity: O(1) - simple atomic counter increment
// Space Complexity: O(1) - no additional memory allocation
func (m *Metrics) RecordDownload() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TotalDownloads++
}

// GetStats returns a comprehensive snapshot of current metrics with calculated ratios and health data.
// This method provides a complete point-in-time view of system performance, efficiency, and health,
// enabling monitoring, analysis, and optimization of NoiseFS operations.
//
// Metrics Snapshot Features:
//   - Thread-safe snapshot creation with consistent data
//   - Calculated efficiency ratios and performance indicators
//   - Integrated health monitoring data and scores
//   - JSON-serializable format for external monitoring systems
//   - Comprehensive coverage of all performance dimensions
//
// Calculated Metrics Include:
//   - Block reuse rate for storage efficiency analysis
//   - Cache hit rate for performance optimization
//   - Storage efficiency ratio for overhead analysis
//   - Health scores for system status assessment
//
// Health Integration:
//   - Cache health scores for system health assessment
//   - Randomizer diversity metrics for security analysis
//   - Memory pressure indicators for capacity planning
//   - Availability health for reliability monitoring
//
// The snapshot provides a consistent view of all metrics at a single point in time,
// ensuring accurate analysis and preventing race conditions during data collection.
//
// Returns:
//   - MetricsSnapshot: Complete metrics snapshot with calculated ratios and health data
//
// Call Flow:
//   - Called by: Monitoring systems, performance analysis, health dashboards
//   - Calls: Health monitor for current health scores, internal calculation methods
//
// Time Complexity: O(1) - simple data copying and ratio calculations
// Space Complexity: O(1) - fixed-size snapshot structure
func (m *Metrics) GetStats() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Get current health scores from integrated health monitor
	var healthScore *cache.HealthScore
	if m.healthMonitor != nil {
		healthScore = m.healthMonitor.CalculateHealthScore()
	}

	// Create comprehensive metrics snapshot with raw counters and calculated ratios
	snapshot := MetricsSnapshot{
		BlocksReused:          m.BlocksReused,                   // Raw count of reused blocks
		BlocksGenerated:       m.BlocksGenerated,               // Raw count of generated blocks
		CacheHits:             m.CacheHits,                     // Raw count of cache hits
		CacheMisses:           m.CacheMisses,                   // Raw count of cache misses
		TotalUploads:          m.TotalUploads,                  // Total upload operations
		TotalDownloads:        m.TotalDownloads,                // Total download operations
		BytesUploadedOriginal: m.BytesUploadedOriginal,         // Original bytes before processing
		BytesStoredIPFS:       m.BytesStoredIPFS,               // Actual bytes stored with overhead
		BlockReuseRate:        m.calculateBlockReuseRate(),     // Calculated reuse efficiency percentage
		CacheHitRate:          m.calculateCacheHitRate(),       // Calculated cache performance percentage
		StorageEfficiency:     m.calculateStorageEfficiency(),  // Calculated storage overhead percentage
	}

	// Integrate health monitoring data for comprehensive system status
	if healthScore != nil {
		snapshot.CacheHealthScore = healthScore.Overall              // Overall system health score (0-1)
		snapshot.RandomizerDiversity = healthScore.RandomizerDiversity // Randomizer entropy for security (0-1)
		snapshot.AverageBlockAge = healthScore.AverageBlockAge        // Block freshness metric (hours)
		snapshot.MemoryPressure = healthScore.MemoryPressure         // Memory utilization pressure (0-1)
		snapshot.EvictionRate = healthScore.EvictionRate             // Cache eviction frequency (per hour)
		snapshot.CoordinationHealth = healthScore.CoordinationHealth // Network coordination health (0-1)
		snapshot.AvailabilityHealth = healthScore.AvailabilityHealth // Randomizer availability health (0-1)
	}

	return snapshot
}

// MetricsSnapshot represents a comprehensive point-in-time view of NoiseFS system metrics.
// This structure provides a complete snapshot of system performance, efficiency, and health
// in a JSON-serializable format suitable for monitoring, analysis, and external integration.
//
// The snapshot includes multiple metric categories:
//   - Operational metrics: Upload/download counts and throughput data
//   - Storage efficiency: Block reuse rates and storage overhead analysis
//   - Cache performance: Hit/miss ratios and cache effectiveness
//   - System health: Memory pressure, eviction rates, and coordination status
//   - Security metrics: Randomizer diversity and availability scores
//
// Calculated Ratios:
//   - Block reuse rate: Percentage of blocks reused vs generated for efficiency
//   - Cache hit rate: Percentage of successful cache retrievals for performance
//   - Storage efficiency: Storage overhead percentage for cost analysis
//
// Health Metrics:
//   - Overall health scores (0-1 scale) for system status assessment
//   - Component-specific health indicators for targeted optimization
//   - Performance trend indicators for predictive analysis
//
// JSON Serialization:
//   All fields are tagged for JSON serialization, enabling integration with
//   external monitoring systems, dashboards, and analytics platforms.
//
// Time Complexity: O(1) for snapshot creation and serialization
// Space Complexity: O(1) - fixed-size structure with numeric fields
type MetricsSnapshot struct {
	// Operational Metrics - Core system operation counters
	BlocksReused          int64   `json:"blocks_reused"`          // Number of blocks reused from cache for efficiency
	BlocksGenerated       int64   `json:"blocks_generated"`       // Number of new blocks generated for overhead tracking
	CacheHits             int64   `json:"cache_hits"`             // Number of successful cache retrievals
	CacheMisses           int64   `json:"cache_misses"`           // Number of cache misses requiring network retrieval
	TotalUploads          int64   `json:"total_uploads"`          // Total file upload operations completed
	TotalDownloads        int64   `json:"total_downloads"`        // Total file download operations completed
	BytesUploadedOriginal int64   `json:"bytes_uploaded_original"` // Original file bytes before processing
	BytesStoredIPFS       int64   `json:"bytes_stored_ipfs"`       // Actual bytes stored including overhead

	// Calculated Efficiency Ratios - Derived performance indicators
	BlockReuseRate        float64 `json:"block_reuse_rate"`       // Percentage of blocks reused (0-100)
	CacheHitRate          float64 `json:"cache_hit_rate"`         // Percentage of cache hits (0-100)
	StorageEfficiency     float64 `json:"storage_efficiency"`     // Storage overhead percentage (100+ indicates overhead)

	// System Health Metrics - Real-time health assessment indicators
	CacheHealthScore    float64 `json:"cache_health_score"`   // Overall cache system health score (0-1)
	RandomizerDiversity float64 `json:"randomizer_diversity"` // Randomizer entropy for security assessment (0-1)
	AverageBlockAge     float64 `json:"average_block_age"`    // Average hours since last block access
	MemoryPressure      float64 `json:"memory_pressure"`      // Memory utilization pressure indicator (0-1)
	EvictionRate        float64 `json:"eviction_rate"`        // Cache evictions per hour for capacity analysis
	CoordinationHealth  float64 `json:"coordination_health"`  // Network coordination health score (0-1)
	AvailabilityHealth  float64 `json:"availability_health"`  // Randomizer availability health score (0-1)
}

// calculateBlockReuseRate returns the percentage of blocks that were reused for storage efficiency analysis.
// This internal calculation method determines how effectively the system reuses existing blocks
// instead of generating new ones, providing a key indicator of storage efficiency and cache performance.
//
// Block Reuse Rate Significance:
//   - Higher rates indicate better storage efficiency and cache utilization
//   - Lower rates may suggest need for cache capacity expansion or policy optimization
//   - Optimal rates depend on workload patterns and randomizer availability
//   - Enables comparison across different caching strategies and configurations
//
// Calculation Method:
//   Rate = (BlocksReused / (BlocksReused + BlocksGenerated)) * 100
//   Returns 0.0 if no blocks have been processed to avoid division by zero
//
// Usage Applications:
//   - Storage efficiency optimization and policy tuning
//   - Cache performance assessment and capacity planning
//   - System health monitoring and alerting thresholds
//   - Comparative analysis across different configurations
//
// Returns:
//   - float64: Block reuse percentage (0.0-100.0), or 0.0 if no blocks processed
//
// Time Complexity: O(1) - simple arithmetic calculation
// Space Complexity: O(1) - no additional memory allocation
func (m *Metrics) calculateBlockReuseRate() float64 {
	total := m.BlocksReused + m.BlocksGenerated
	if total == 0 {
		return 0.0 // Avoid division by zero when no blocks processed
	}
	return float64(m.BlocksReused) / float64(total) * 100.0
}

// calculateCacheHitRate returns the cache hit percentage for performance optimization analysis.
// This internal calculation method determines cache effectiveness by measuring successful
// cache retrievals against total cache access attempts, providing critical performance insights.
//
// Cache Hit Rate Significance:
//   - Higher rates indicate effective caching and better system performance
//   - Lower rates suggest need for cache optimization or capacity expansion
//   - Industry benchmarks typically target 80%+ hit rates for optimal performance
//   - Enables identification of performance bottlenecks and optimization opportunities
//
// Calculation Method:
//   Rate = (CacheHits / (CacheHits + CacheMisses)) * 100
//   Returns 0.0 if no cache operations have occurred to avoid division by zero
//
// Performance Implications:
//   - High hit rates reduce network traffic and improve response times
//   - Low hit rates increase storage backend load and user-perceived latency
//   - Optimal rates depend on cache size, eviction policies, and access patterns
//   - Critical metric for cache tuning and capacity planning decisions
//
// Returns:
//   - float64: Cache hit percentage (0.0-100.0), or 0.0 if no cache operations
//
// Time Complexity: O(1) - simple arithmetic calculation
// Space Complexity: O(1) - no additional memory allocation
func (m *Metrics) calculateCacheHitRate() float64 {
	total := m.CacheHits + m.CacheMisses
	if total == 0 {
		return 0.0 // Avoid division by zero when no cache operations
	}
	return float64(m.CacheHits) / float64(total) * 100.0
}

// calculateStorageEfficiency returns the storage overhead percentage for cost and efficiency analysis.
// This internal calculation method determines storage efficiency by comparing actual storage used
// against original file sizes, providing insights into anonymization overhead and optimization opportunities.
//
// Storage Efficiency Analysis:
//   - Values near 100% indicate minimal overhead and high efficiency
//   - Values significantly above 100% indicate overhead from anonymization and padding
//   - NoiseFS typically achieves ~300% overhead due to 3-tuple XOR anonymization
//   - Lower overhead indicates better randomizer reuse and cache efficiency
//
// Overhead Sources in NoiseFS:
//   - 3-tuple XOR anonymization requires storing data + 2 randomizer blocks
//   - Block padding to fixed sizes for privacy protection
//   - Storage backend metadata and protocol overhead
//   - Network redundancy and replication factors
//
// Calculation Method:
//   Efficiency = (BytesStoredIPFS / BytesUploadedOriginal) * 100
//   Returns 0.0 if no uploads have occurred to avoid division by zero
//
// Optimization Applications:
//   - Storage cost analysis and capacity planning
//   - Randomizer reuse strategy effectiveness assessment
//   - Block size optimization for different file types
//   - Cache policy tuning for better efficiency
//
// Returns:
//   - float64: Storage overhead percentage (100+ indicates overhead), or 0.0 if no uploads
//
// Time Complexity: O(1) - simple arithmetic calculation
// Space Complexity: O(1) - no additional memory allocation
func (m *Metrics) calculateStorageEfficiency() float64 {
	if m.BytesUploadedOriginal == 0 {
		return 0.0 // Avoid division by zero when no uploads processed
	}
	overhead := float64(m.BytesStoredIPFS) / float64(m.BytesUploadedOriginal) * 100.0
	return overhead
}

// SetHealthMonitorComponents configures health monitor with cache coordination and tracking components.
// This configuration method integrates cache coordination and health tracking systems with the metrics
// health monitor, enabling comprehensive system health assessment and monitoring.
//
// Component Integration:
//   - Coordination Engine: Provides network coordination health metrics
//   - Block Health Tracker: Supplies block-level health and aging information
//   - Combined integration enables comprehensive health assessment
//
// Health Monitoring Benefits:
//   - Real-time system health assessment and alerting
//   - Predictive health analysis for proactive maintenance
//   - Component-specific health tracking for targeted optimization
//   - Integration with external monitoring and alerting systems
//
// Configuration Safety:
//   - Thread-safe configuration with proper locking
//   - Null-safe component checking to prevent errors
//   - Graceful handling of partial component availability
//
// Parameters:
//   - ce: Coordination engine for network health metrics (nil-safe)
//   - ht: Block health tracker for block-level health data (nil-safe)
//
// Call Flow:
//   - Called by: Client initialization, health system configuration
//   - Calls: Health monitor component configuration methods
//
// Time Complexity: O(1) - simple component assignment
// Space Complexity: O(1) - no additional memory allocation
func (m *Metrics) SetHealthMonitorComponents(ce *cache.CoordinationEngine, ht *cache.BlockHealthTracker) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.healthMonitor != nil {
		// Configure coordination engine for network health tracking
		if ce != nil {
			m.healthMonitor.SetCoordinationEngine(ce)
		}
		// Configure health tracker for block-level health monitoring
		if ht != nil {
			m.healthMonitor.SetHealthTracker(ht)
		}
	}
}

// SetAvailabilityIntegration configures health monitor with availability integration for reliability tracking.
// This configuration method integrates availability monitoring with the health assessment system,
// enabling comprehensive reliability analysis and predictive health scoring.
//
// Availability Integration Benefits:
//   - Real-time randomizer availability monitoring for system reliability
//   - Predictive availability analysis for proactive optimization
//   - Integration with fallback mechanisms and redundancy systems
//   - Health score calculation including availability factors
//
// Reliability Metrics:
//   - Randomizer block availability rates for anonymization reliability
//   - Network availability patterns for system planning
//   - Fallback mechanism effectiveness for resilience assessment
//   - Availability trend analysis for predictive maintenance
//
// Health Assessment Integration:
//   - Availability scores contribute to overall health assessment
//   - Availability trends inform health predictions and alerts
//   - Integration with coordination health for comprehensive analysis
//
// Parameters:
//   - ai: Availability integration system for reliability monitoring (nil-safe)
//
// Call Flow:
//   - Called by: Client initialization, availability system configuration
//   - Calls: Health monitor availability integration configuration
//
// Time Complexity: O(1) - simple component assignment
// Space Complexity: O(1) - no additional memory allocation
func (m *Metrics) SetAvailabilityIntegration(ai *cache.AvailabilityIntegration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Configure availability integration for reliability health tracking
	if m.healthMonitor != nil && ai != nil {
		m.healthMonitor.SetAvailabilityIntegration(ai)
	}
}

// UpdateHealthMetrics updates the health monitor with current cache state for real-time health assessment.
// This method provides real-time health data updates to the health monitoring system,
// enabling continuous health assessment and proactive alerting based on current system state.
//
// Health Data Integration:
//   - Real-time cache state updates for accurate health assessment
//   - Memory pressure and utilization metrics for capacity analysis
//   - Performance indicators for trend analysis and optimization
//   - Component health data for targeted health monitoring
//
// Real-time Health Benefits:
//   - Immediate health assessment based on current system state
//   - Proactive alerting and intervention capability
//   - Trend analysis for predictive health assessment
//   - Health-based optimization and adaptation decisions
//
// Health Metrics Integration:
//   - Cache utilization and memory pressure indicators
//   - Performance metrics and efficiency measurements
//   - Component-specific health and status information
//   - Historical health data for trend analysis
//
// Parameters:
//   - metrics: Current cache health metrics for health assessment (nil-safe)
//
// Call Flow:
//   - Called by: Cache systems, health monitoring loops, periodic health updates
//   - Calls: Health monitor metrics update methods
//
// Time Complexity: O(1) - simple metrics update
// Space Complexity: O(1) - no additional memory allocation
func (m *Metrics) UpdateHealthMetrics(metrics *cache.HealthMetrics) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Update health monitor with current cache state for real-time assessment
	if m.healthMonitor != nil {
		m.healthMonitor.UpdateMetrics(metrics)
	}
}

// RecordEviction records a cache eviction event for health tracking and capacity analysis.
// This method tracks cache eviction events to enable analysis of memory pressure,
// cache efficiency, and capacity planning for optimal system performance.
//
// Eviction Tracking Benefits:
//   - Memory pressure analysis for capacity planning decisions
//   - Cache efficiency assessment and optimization opportunities
//   - Eviction rate monitoring for performance impact analysis
//   - Health scoring based on eviction patterns and frequency
//
// Eviction Analysis Applications:
//   - Cache capacity planning and optimization
//   - Eviction policy effectiveness assessment
//   - Memory pressure alerting and intervention
//   - Performance impact analysis and mitigation
//
// Health Impact:
//   - High eviction rates may indicate memory pressure or suboptimal cache sizing
//   - Eviction patterns inform health scores and alerting thresholds
//   - Trend analysis enables predictive capacity planning
//   - Integration with overall system health assessment
//
// Call Flow:
//   - Called by: Cache eviction operations, memory management, cache policy enforcement
//   - Calls: Health monitor eviction recording for health assessment
//
// Time Complexity: O(1) - simple event recording
// Space Complexity: O(1) - no additional memory allocation
func (m *Metrics) RecordEviction() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Record eviction event for health monitoring and capacity analysis
	if m.healthMonitor != nil {
		m.healthMonitor.RecordEviction()
	}
}

// GetHealthSummary returns a human-readable health summary for debugging and monitoring.
// This method provides a comprehensive, human-readable assessment of system health,
// suitable for debugging, monitoring dashboards, and operational status reporting.
//
// Health Summary Features:
//   - Human-readable health status and assessment
//   - Component-specific health information and diagnostics
//   - Performance indicators and optimization recommendations
//   - Alert-worthy conditions and intervention suggestions
//
// Summary Applications:
//   - System administration and operational monitoring
//   - Debugging and troubleshooting health-related issues
//   - Dashboard displays and status reporting
//   - Documentation and audit trail for health status
//
// Health Assessment Coverage:
//   - Overall system health status and score
//   - Cache performance and efficiency indicators
//   - Memory pressure and capacity utilization
//   - Network coordination and availability status
//   - Recommendations for optimization and intervention
//
// The summary provides actionable insights for system administrators and enables
// quick assessment of system health status without requiring detailed metric analysis.
//
// Returns:
//   - string: Human-readable health summary, or "Unknown" if health monitor unavailable
//
// Call Flow:
//   - Called by: Monitoring systems, debugging tools, administrative interfaces
//   - Calls: Health monitor summary generation methods
//
// Time Complexity: O(1) - summary generation from cached health data
// Space Complexity: O(k) where k is the summary text length
func (m *Metrics) GetHealthSummary() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Generate human-readable health summary for monitoring and debugging
	if m.healthMonitor != nil {
		return m.healthMonitor.GetHealthSummary()
	}
	return "Unknown" // Health monitor not available
}

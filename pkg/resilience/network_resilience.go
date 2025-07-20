package resilience

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// OperationType represents different types of network operations
type OperationType int

const (
	OperationRead OperationType = iota
	OperationWrite
	OperationDelete
	OperationList
	OperationSync
	OperationQuery
)

// String returns the string representation of OperationType
func (ot OperationType) String() string {
	switch ot {
	case OperationRead:
		return "Read"
	case OperationWrite:
		return "Write"
	case OperationDelete:
		return "Delete"
	case OperationList:
		return "List"
	case OperationSync:
		return "Sync"
	case OperationQuery:
		return "Query"
	default:
		return "Unknown"
	}
}

// OperationConfig holds configuration for specific operation types
type OperationConfig struct {
	Timeout     time.Duration
	RetryPolicy *RetryConfig
	Enabled     bool
}

// NetworkResilienceConfig holds configuration for the network resilience wrapper
type NetworkResilienceConfig struct {
	// DefaultTimeout is the default timeout for operations
	DefaultTimeout time.Duration
	// DefaultRetryPolicy is the default retry policy
	DefaultRetryPolicy *RetryConfig
	// OperationConfigs holds per-operation-type configurations
	OperationConfigs map[OperationType]*OperationConfig
	// EnableCircuitBreaker enables circuit breaker protection
	EnableCircuitBreaker bool
	// CircuitBreakerConfig holds circuit breaker configuration
	CircuitBreakerConfig *CircuitBreakerConfig
	// EnableHealthMonitoring enables health monitoring
	EnableHealthMonitoring bool
	// HealthMonitorConfig holds health monitor configuration
	HealthMonitorConfig *HealthMonitorConfig
}

// DefaultNetworkResilienceConfig returns a sensible default configuration
func DefaultNetworkResilienceConfig() *NetworkResilienceConfig {
	defaultRetry := DefaultRetryConfig()
	
	operationConfigs := map[OperationType]*OperationConfig{
		OperationRead: {
			Timeout:     10 * time.Second,
			RetryPolicy: defaultRetry,
			Enabled:     true,
		},
		OperationWrite: {
			Timeout:     30 * time.Second,
			RetryPolicy: &RetryConfig{
				MaxRetries:        5,
				InitialDelay:      200 * time.Millisecond,
				MaxDelay:          10 * time.Second,
				BackoffMultiplier: 2.0,
				Jitter:            true,
			},
			Enabled: true,
		},
		OperationDelete: {
			Timeout:     15 * time.Second,
			RetryPolicy: defaultRetry,
			Enabled:     true,
		},
		OperationList: {
			Timeout:     20 * time.Second,
			RetryPolicy: defaultRetry,
			Enabled:     true,
		},
		OperationSync: {
			Timeout:     60 * time.Second,
			RetryPolicy: &RetryConfig{
				MaxRetries:        3,
				InitialDelay:      500 * time.Millisecond,
				MaxDelay:          30 * time.Second,
				BackoffMultiplier: 2.0,
				Jitter:            true,
			},
			Enabled: true,
		},
		OperationQuery: {
			Timeout:     10 * time.Second,
			RetryPolicy: defaultRetry,
			Enabled:     true,
		},
	}

	return &NetworkResilienceConfig{
		DefaultTimeout:         15 * time.Second,
		DefaultRetryPolicy:     defaultRetry,
		OperationConfigs:       operationConfigs,
		EnableCircuitBreaker:   true,
		CircuitBreakerConfig:   DefaultCircuitBreakerConfig("network"),
		EnableHealthMonitoring: true,
		HealthMonitorConfig:    DefaultHealthMonitorConfig(),
	}
}

// NetworkResilience provides a resilient wrapper for network operations
type NetworkResilience struct {
	config            *NetworkResilienceConfig
	circuitBreaker    *CircuitBreaker
	healthMonitor     *HealthMonitor
	connectionManager *ConnectionManager
	mu                sync.RWMutex

	// Statistics
	stats map[OperationType]*OperationStats
}

// OperationStats holds statistics for operation types
type OperationStats struct {
	TotalOperations   int64         `json:"total_operations"`
	SuccessfulOps     int64         `json:"successful_operations"`
	FailedOps         int64         `json:"failed_operations"`
	TotalDuration     time.Duration `json:"total_duration"`
	AverageDuration   time.Duration `json:"average_duration"`
	LastOperationTime time.Time     `json:"last_operation_time"`
	mu                sync.RWMutex
}

// NewNetworkResilience creates a new network resilience wrapper
func NewNetworkResilience(config *NetworkResilienceConfig, connectionManager *ConnectionManager) *NetworkResilience {
	if config == nil {
		config = DefaultNetworkResilienceConfig()
	}

	nr := &NetworkResilience{
		config:            config,
		connectionManager: connectionManager,
		stats:             make(map[OperationType]*OperationStats),
	}

	// Initialize circuit breaker if enabled
	if config.EnableCircuitBreaker {
		nr.circuitBreaker = NewCircuitBreaker(config.CircuitBreakerConfig)
	}

	// Initialize health monitor if enabled
	if config.EnableHealthMonitoring {
		nr.healthMonitor = NewHealthMonitor(config.HealthMonitorConfig)
	}

	// Initialize stats for all operation types
	for opType := range config.OperationConfigs {
		nr.stats[opType] = &OperationStats{}
	}

	return nr
}

// ExecuteOperation executes a network operation with full resilience protection
func (nr *NetworkResilience) ExecuteOperation(ctx context.Context, opType OperationType, fn func(context.Context) error) error {
	startTime := time.Now()
	
	// Get operation configuration
	opConfig := nr.getOperationConfig(opType)
	if !opConfig.Enabled {
		return fmt.Errorf("operation type %s is disabled", opType.String())
	}

	// Create context with timeout
	if opConfig.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opConfig.Timeout)
		defer cancel()
	}

	// Execute with resilience patterns
	var err error
	if nr.circuitBreaker != nil {
		// Execute with circuit breaker
		err = nr.circuitBreaker.Execute(ctx, func(ctx context.Context) error {
			// Execute with retry
			return RetryWithConfig(ctx, fn, opConfig.RetryPolicy)
		})
	} else {
		// Execute with retry only
		err = RetryWithConfig(ctx, fn, opConfig.RetryPolicy)
	}

	// Record statistics
	nr.recordOperationStats(opType, time.Since(startTime), err == nil)

	return err
}

// ExecuteOperationWithBackend executes an operation with backend selection and failover
func (nr *NetworkResilience) ExecuteOperationWithBackend(ctx context.Context, opType OperationType, fn func(context.Context, *Backend) error) error {
	if nr.connectionManager == nil {
		return fmt.Errorf("no connection manager configured")
	}

	return nr.ExecuteOperation(ctx, opType, func(ctx context.Context) error {
		return nr.connectionManager.ExecuteWithFailover(ctx, fn)
	})
}

// ExecuteRead executes a read operation with resilience
func (nr *NetworkResilience) ExecuteRead(ctx context.Context, fn func(context.Context) error) error {
	return nr.ExecuteOperation(ctx, OperationRead, fn)
}

// ExecuteWrite executes a write operation with resilience
func (nr *NetworkResilience) ExecuteWrite(ctx context.Context, fn func(context.Context) error) error {
	return nr.ExecuteOperation(ctx, OperationWrite, fn)
}

// ExecuteDelete executes a delete operation with resilience
func (nr *NetworkResilience) ExecuteDelete(ctx context.Context, fn func(context.Context) error) error {
	return nr.ExecuteOperation(ctx, OperationDelete, fn)
}

// ExecuteList executes a list operation with resilience
func (nr *NetworkResilience) ExecuteList(ctx context.Context, fn func(context.Context) error) error {
	return nr.ExecuteOperation(ctx, OperationList, fn)
}

// ExecuteSync executes a sync operation with resilience
func (nr *NetworkResilience) ExecuteSync(ctx context.Context, fn func(context.Context) error) error {
	return nr.ExecuteOperation(ctx, OperationSync, fn)
}

// ExecuteQuery executes a query operation with resilience
func (nr *NetworkResilience) ExecuteQuery(ctx context.Context, fn func(context.Context) error) error {
	return nr.ExecuteOperation(ctx, OperationQuery, fn)
}

// GetOperationStats returns statistics for a specific operation type
func (nr *NetworkResilience) GetOperationStats(opType OperationType) *OperationStats {
	nr.mu.RLock()
	defer nr.mu.RUnlock()

	stats, exists := nr.stats[opType]
	if !exists {
		return &OperationStats{}
	}

	// Return a copy to avoid race conditions
	stats.mu.RLock()
	defer stats.mu.RUnlock()

	return &OperationStats{
		TotalOperations:   stats.TotalOperations,
		SuccessfulOps:     stats.SuccessfulOps,
		FailedOps:         stats.FailedOps,
		TotalDuration:     stats.TotalDuration,
		AverageDuration:   stats.AverageDuration,
		LastOperationTime: stats.LastOperationTime,
	}
}

// GetAllOperationStats returns statistics for all operation types
func (nr *NetworkResilience) GetAllOperationStats() map[OperationType]*OperationStats {
	nr.mu.RLock()
	defer nr.mu.RUnlock()

	result := make(map[OperationType]*OperationStats)
	for opType, stats := range nr.stats {
		stats.mu.RLock()
		result[opType] = &OperationStats{
			TotalOperations:   stats.TotalOperations,
			SuccessfulOps:     stats.SuccessfulOps,
			FailedOps:         stats.FailedOps,
			TotalDuration:     stats.TotalDuration,
			AverageDuration:   stats.AverageDuration,
			LastOperationTime: stats.LastOperationTime,
		}
		stats.mu.RUnlock()
	}

	return result
}

// GetCircuitBreakerStats returns circuit breaker statistics
func (nr *NetworkResilience) GetCircuitBreakerStats() *CircuitBreakerStats {
	if nr.circuitBreaker == nil {
		return nil
	}

	stats := nr.circuitBreaker.GetStats()
	return &stats
}

// ResetStats resets all operation statistics
func (nr *NetworkResilience) ResetStats() {
	nr.mu.Lock()
	defer nr.mu.Unlock()

	for _, stats := range nr.stats {
		stats.mu.Lock()
		stats.TotalOperations = 0
		stats.SuccessfulOps = 0
		stats.FailedOps = 0
		stats.TotalDuration = 0
		stats.AverageDuration = 0
		stats.LastOperationTime = time.Time{}
		stats.mu.Unlock()
	}

	// Reset circuit breaker if present
	if nr.circuitBreaker != nil {
		nr.circuitBreaker.Reset()
	}
}

// UpdateOperationConfig updates configuration for a specific operation type
func (nr *NetworkResilience) UpdateOperationConfig(opType OperationType, config *OperationConfig) {
	nr.mu.Lock()
	defer nr.mu.Unlock()

	nr.config.OperationConfigs[opType] = config

	// Initialize stats if new operation type
	if _, exists := nr.stats[opType]; !exists {
		nr.stats[opType] = &OperationStats{}
	}
}

// IsOperationEnabled checks if an operation type is enabled
func (nr *NetworkResilience) IsOperationEnabled(opType OperationType) bool {
	nr.mu.RLock()
	defer nr.mu.RUnlock()

	config := nr.getOperationConfig(opType)
	return config.Enabled
}

// Start starts the network resilience wrapper
func (nr *NetworkResilience) Start() {
	if nr.healthMonitor != nil {
		nr.healthMonitor.Start()
	}
}

// Stop stops the network resilience wrapper
func (nr *NetworkResilience) Stop() {
	if nr.healthMonitor != nil {
		nr.healthMonitor.Stop()
	}
}

// getOperationConfig returns the configuration for an operation type
func (nr *NetworkResilience) getOperationConfig(opType OperationType) *OperationConfig {
	if config, exists := nr.config.OperationConfigs[opType]; exists {
		return config
	}

	// Return default configuration
	return &OperationConfig{
		Timeout:     nr.config.DefaultTimeout,
		RetryPolicy: nr.config.DefaultRetryPolicy,
		Enabled:     true,
	}
}

// recordOperationStats records statistics for an operation
func (nr *NetworkResilience) recordOperationStats(opType OperationType, duration time.Duration, success bool) {
	nr.mu.RLock()
	stats, exists := nr.stats[opType]
	nr.mu.RUnlock()

	if !exists {
		return
	}

	stats.mu.Lock()
	defer stats.mu.Unlock()

	stats.TotalOperations++
	stats.TotalDuration += duration
	stats.LastOperationTime = time.Now()

	if success {
		stats.SuccessfulOps++
	} else {
		stats.FailedOps++
	}

	// Update average duration
	if stats.TotalOperations > 0 {
		stats.AverageDuration = time.Duration(int64(stats.TotalDuration) / stats.TotalOperations)
	}
}

// GetSuccessRate returns the success rate for an operation type
func (stats *OperationStats) GetSuccessRate() float64 {
	stats.mu.RLock()
	defer stats.mu.RUnlock()

	if stats.TotalOperations == 0 {
		return 0.0
	}

	return float64(stats.SuccessfulOps) / float64(stats.TotalOperations)
}

// GetFailureRate returns the failure rate for an operation type
func (stats *OperationStats) GetFailureRate() float64 {
	stats.mu.RLock()
	defer stats.mu.RUnlock()

	if stats.TotalOperations == 0 {
		return 0.0
	}

	return float64(stats.FailedOps) / float64(stats.TotalOperations)
}
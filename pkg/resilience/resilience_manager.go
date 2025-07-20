package resilience

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ResilienceManagerConfig holds configuration for the resilience manager
type ResilienceManagerConfig struct {
	// Error handling configuration
	EnableErrorClassification bool
	
	// Circuit breaker configuration
	EnableCircuitBreaker bool
	CircuitBreakerConfig *CircuitBreakerConfig
	
	// Health monitoring configuration
	EnableHealthMonitoring bool
	HealthMonitorConfig    *HealthMonitorConfig
	
	// Connection management configuration
	EnableConnectionManager bool
	ConnectionManagerConfig *ConnectionManagerConfig
	
	// Network resilience configuration
	EnableNetworkResilience bool
	NetworkResilienceConfig *NetworkResilienceConfig
	
	// Recovery management configuration
	EnableRecoveryManager bool
	
	// Global settings
	DefaultTimeout time.Duration
	MetricsEnabled bool
}

// DefaultResilienceManagerConfig returns a sensible default configuration
func DefaultResilienceManagerConfig() *ResilienceManagerConfig {
	return &ResilienceManagerConfig{
		EnableErrorClassification: true,
		EnableCircuitBreaker:      true,
		CircuitBreakerConfig:      DefaultCircuitBreakerConfig("resilience-manager"),
		EnableHealthMonitoring:    true,
		HealthMonitorConfig:       DefaultHealthMonitorConfig(),
		EnableConnectionManager:   true,
		ConnectionManagerConfig:   DefaultConnectionManagerConfig(),
		EnableNetworkResilience:   true,
		NetworkResilienceConfig:   DefaultNetworkResilienceConfig(),
		EnableRecoveryManager:     true,
		DefaultTimeout:            30 * time.Second,
		MetricsEnabled:            true,
	}
}

// ResilienceManager integrates all resilience components into a unified system
type ResilienceManager struct {
	config *ResilienceManagerConfig
	
	// Core components
	circuitBreaker    *CircuitBreaker
	healthMonitor     *HealthMonitor
	connectionManager *ConnectionManager
	networkResilience *NetworkResilience
	recoveryManager   *SyncRecoveryManager
	
	// State
	started bool
	mu      sync.RWMutex
	
	// Metrics
	totalOperations   int64
	successfulOps     int64
	failedOps         int64
	totalRecoveries   int64
	lastOperationTime time.Time
	metricsLock       sync.RWMutex
}

// NewResilienceManager creates a new resilience manager
func NewResilienceManager(config *ResilienceManagerConfig) *ResilienceManager {
	if config == nil {
		config = DefaultResilienceManagerConfig()
	}
	
	rm := &ResilienceManager{
		config: config,
	}
	
	// Initialize components based on configuration
	if config.EnableCircuitBreaker {
		rm.circuitBreaker = NewCircuitBreaker(config.CircuitBreakerConfig)
	}
	
	if config.EnableHealthMonitoring {
		rm.healthMonitor = NewHealthMonitor(config.HealthMonitorConfig)
	}
	
	if config.EnableConnectionManager {
		rm.connectionManager = NewConnectionManager(config.ConnectionManagerConfig)
	}
	
	if config.EnableNetworkResilience {
		rm.networkResilience = NewNetworkResilience(config.NetworkResilienceConfig, rm.connectionManager)
	}
	
	if config.EnableRecoveryManager {
		rm.recoveryManager = NewSyncRecoveryManager()
	}
	
	return rm
}

// Start starts all resilience components
func (rm *ResilienceManager) Start() error {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	
	if rm.started {
		return fmt.Errorf("resilience manager already started")
	}
	
	// Start health monitor
	if rm.healthMonitor != nil {
		rm.healthMonitor.Start()
	}
	
	// Start connection manager
	if rm.connectionManager != nil {
		rm.connectionManager.Start()
	}
	
	// Start network resilience
	if rm.networkResilience != nil {
		rm.networkResilience.Start()
	}
	
	rm.started = true
	return nil
}

// Stop stops all resilience components
func (rm *ResilienceManager) Stop() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	
	if !rm.started {
		return
	}
	
	// Stop network resilience
	if rm.networkResilience != nil {
		rm.networkResilience.Stop()
	}
	
	// Stop connection manager
	if rm.connectionManager != nil {
		rm.connectionManager.Stop()
	}
	
	// Stop health monitor
	if rm.healthMonitor != nil {
		rm.healthMonitor.Stop()
	}
	
	rm.started = false
}

// ExecuteResilientOperation executes an operation with full resilience protection
func (rm *ResilienceManager) ExecuteResilientOperation(ctx context.Context, opType OperationType, fn func(context.Context) error) error {
	start := time.Now()
	
	// Apply timeout if configured
	if rm.config.DefaultTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, rm.config.DefaultTimeout)
		defer cancel()
	}
	
	var err error
	
	// Execute with network resilience if available
	if rm.networkResilience != nil {
		err = rm.networkResilience.ExecuteOperation(ctx, opType, fn)
	} else if rm.circuitBreaker != nil {
		// Fall back to circuit breaker only
		err = rm.circuitBreaker.Execute(ctx, fn)
	} else {
		// Execute directly
		err = fn(ctx)
	}
	
	// Update metrics
	rm.updateMetrics(time.Since(start), err == nil)
	
	// Classify error if error classification is enabled
	if err != nil && rm.config.EnableErrorClassification {
		classified := ClassifyError(err, "resilience-manager")
		if classified != nil {
			return classified
		}
	}
	
	return err
}

// ExecuteResilientOperationWithBackend executes an operation with backend selection and full resilience
func (rm *ResilienceManager) ExecuteResilientOperationWithBackend(ctx context.Context, opType OperationType, fn func(context.Context, *Backend) error) error {
	if rm.networkResilience == nil {
		return fmt.Errorf("network resilience not enabled")
	}
	
	start := time.Now()
	
	// Apply timeout if configured
	if rm.config.DefaultTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, rm.config.DefaultTimeout)
		defer cancel()
	}
	
	err := rm.networkResilience.ExecuteOperationWithBackend(ctx, opType, fn)
	
	// Update metrics
	rm.updateMetrics(time.Since(start), err == nil)
	
	// Classify error if error classification is enabled
	if err != nil && rm.config.EnableErrorClassification {
		classified := ClassifyError(err, "resilience-manager")
		if classified != nil {
			return classified
		}
	}
	
	return err
}

// CreateRecoveryWorkflow creates a new recovery workflow
func (rm *ResilienceManager) CreateRecoveryWorkflow(id, description string) (*RecoveryWorkflow, error) {
	if rm.recoveryManager == nil {
		return nil, fmt.Errorf("recovery manager not enabled")
	}
	
	workflow := rm.recoveryManager.CreateWorkflow(id, description)
	
	// Set up callback to track recoveries
	workflow.SetWorkflowCompleteCallback(func(w *RecoveryWorkflow, success bool) {
		if !success {
			rm.incrementRecoveries()
		}
	})
	
	return workflow, nil
}

// AddBackend adds a storage backend with health monitoring
func (rm *ResilienceManager) AddBackend(backend *Backend, healthCheck HealthCheck) error {
	if rm.connectionManager == nil {
		return fmt.Errorf("connection manager not enabled")
	}
	
	return rm.connectionManager.AddBackend(backend, healthCheck)
}

// RemoveBackend removes a storage backend
func (rm *ResilienceManager) RemoveBackend(backendID string) error {
	if rm.connectionManager == nil {
		return fmt.Errorf("connection manager not enabled")
	}
	
	return rm.connectionManager.RemoveBackend(backendID)
}

// RegisterHealthComponent registers a component for health monitoring
func (rm *ResilienceManager) RegisterHealthComponent(name string, healthCheck HealthCheck) error {
	if rm.healthMonitor == nil {
		return fmt.Errorf("health monitor not enabled")
	}
	
	rm.healthMonitor.RegisterComponent(name, healthCheck)
	return nil
}

// GetSystemHealth returns overall system health status
func (rm *ResilienceManager) GetSystemHealth() (*SystemHealthReport, error) {
	report := &SystemHealthReport{
		Timestamp: time.Now(),
		Overall:   HealthHealthy,
	}
	
	// Get health monitor status
	if rm.healthMonitor != nil {
		report.HealthMonitor = &HealthMonitorReport{
			OverallHealth: rm.healthMonitor.GetOverallHealth(),
			Summary:       rm.healthMonitor.GetHealthSummary(),
		}
		
		if report.HealthMonitor.OverallHealth > report.Overall {
			report.Overall = report.HealthMonitor.OverallHealth
		}
	}
	
	// Get circuit breaker status
	if rm.circuitBreaker != nil {
		stats := rm.circuitBreaker.GetStats()
		report.CircuitBreaker = &CircuitBreakerReport{
			State: stats.State,
			Stats: &stats,
		}
		
		if stats.State == StateOpen {
			report.Overall = HealthCritical
		}
	}
	
	// Get connection manager status
	if rm.connectionManager != nil {
		statuses := rm.connectionManager.GetAllBackendStatuses()
		report.ConnectionManager = &ConnectionManagerReport{
			BackendStatuses: statuses,
			PrimaryBackend:  rm.connectionManager.GetPrimaryBackend(),
		}
		
		// Check if any critical backends are down
		for _, status := range statuses {
			if status == ConnectionFailed {
				if report.Overall < HealthUnhealthy {
					report.Overall = HealthUnhealthy
				}
			}
		}
	}
	
	// Get network resilience stats
	if rm.networkResilience != nil {
		allStats := rm.networkResilience.GetAllOperationStats()
		report.NetworkResilience = &NetworkResilienceReport{
			OperationStats: allStats,
		}
	}
	
	// Get recovery manager stats
	if rm.recoveryManager != nil {
		stats := rm.recoveryManager.GetStatistics()
		report.RecoveryManager = &RecoveryManagerReport{
			Statistics: stats,
		}
	}
	
	// Get overall metrics
	report.Metrics = rm.getMetrics()
	
	return report, nil
}

// ValidateSystemState validates the consistency of the entire system
func (rm *ResilienceManager) ValidateSystemState(ctx context.Context, state interface{}) error {
	if rm.recoveryManager == nil {
		return fmt.Errorf("recovery manager not enabled for state validation")
	}
	
	return rm.recoveryManager.ValidateState(ctx, state)
}

// GetMetrics returns operational metrics
func (rm *ResilienceManager) GetMetrics() *ResilienceMetrics {
	return rm.getMetrics()
}

// ResetMetrics resets all operational metrics
func (rm *ResilienceManager) ResetMetrics() {
	rm.metricsLock.Lock()
	defer rm.metricsLock.Unlock()
	
	rm.totalOperations = 0
	rm.successfulOps = 0
	rm.failedOps = 0
	rm.totalRecoveries = 0
	rm.lastOperationTime = time.Time{}
	
	// Reset component metrics
	if rm.networkResilience != nil {
		rm.networkResilience.ResetStats()
	}
	
	if rm.circuitBreaker != nil {
		rm.circuitBreaker.Reset()
	}
}

// IsHealthy returns true if the system is healthy
func (rm *ResilienceManager) IsHealthy() bool {
	report, err := rm.GetSystemHealth()
	if err != nil {
		return false
	}
	
	return report.Overall == HealthHealthy || report.Overall == HealthDegraded
}

// updateMetrics updates operational metrics
func (rm *ResilienceManager) updateMetrics(duration time.Duration, success bool) {
	if !rm.config.MetricsEnabled {
		return
	}
	
	rm.metricsLock.Lock()
	defer rm.metricsLock.Unlock()
	
	rm.totalOperations++
	rm.lastOperationTime = time.Now()
	
	if success {
		rm.successfulOps++
	} else {
		rm.failedOps++
	}
}

// incrementRecoveries increments the recovery counter
func (rm *ResilienceManager) incrementRecoveries() {
	if !rm.config.MetricsEnabled {
		return
	}
	
	rm.metricsLock.Lock()
	defer rm.metricsLock.Unlock()
	
	rm.totalRecoveries++
}

// getMetrics returns current metrics
func (rm *ResilienceManager) getMetrics() *ResilienceMetrics {
	rm.metricsLock.RLock()
	defer rm.metricsLock.RUnlock()
	
	successRate := 0.0
	if rm.totalOperations > 0 {
		successRate = float64(rm.successfulOps) / float64(rm.totalOperations)
	}
	
	return &ResilienceMetrics{
		TotalOperations:   rm.totalOperations,
		SuccessfulOps:     rm.successfulOps,
		FailedOps:         rm.failedOps,
		TotalRecoveries:   rm.totalRecoveries,
		SuccessRate:       successRate,
		LastOperationTime: rm.lastOperationTime,
	}
}

// Health report structures
type SystemHealthReport struct {
	Timestamp         time.Time                  `json:"timestamp"`
	Overall           HealthStatus               `json:"overall_status"`
	HealthMonitor     *HealthMonitorReport       `json:"health_monitor,omitempty"`
	CircuitBreaker    *CircuitBreakerReport      `json:"circuit_breaker,omitempty"`
	ConnectionManager *ConnectionManagerReport   `json:"connection_manager,omitempty"`
	NetworkResilience *NetworkResilienceReport   `json:"network_resilience,omitempty"`
	RecoveryManager   *RecoveryManagerReport     `json:"recovery_manager,omitempty"`
	Metrics           *ResilienceMetrics         `json:"metrics"`
}

type HealthMonitorReport struct {
	OverallHealth HealthStatus     `json:"overall_health"`
	Summary       *HealthSummary   `json:"summary"`
}

type CircuitBreakerReport struct {
	State CircuitBreakerState  `json:"state"`
	Stats *CircuitBreakerStats `json:"stats"`
}

type ConnectionManagerReport struct {
	BackendStatuses map[string]ConnectionStatus `json:"backend_statuses"`
	PrimaryBackend  *Backend                    `json:"primary_backend,omitempty"`
}

type NetworkResilienceReport struct {
	OperationStats map[OperationType]*OperationStats `json:"operation_stats"`
}

type RecoveryManagerReport struct {
	Statistics map[string]int64 `json:"statistics"`
}

type ResilienceMetrics struct {
	TotalOperations   int64     `json:"total_operations"`
	SuccessfulOps     int64     `json:"successful_operations"`
	FailedOps         int64     `json:"failed_operations"`
	TotalRecoveries   int64     `json:"total_recoveries"`
	SuccessRate       float64   `json:"success_rate"`
	LastOperationTime time.Time `json:"last_operation_time"`
}
package resilience

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ConnectionStatus represents the status of a connection
type ConnectionStatus int

const (
	ConnectionUnknown ConnectionStatus = iota
	ConnectionActive
	ConnectionDegraded
	ConnectionInactive
	ConnectionFailed
)

// String returns the string representation of ConnectionStatus
func (cs ConnectionStatus) String() string {
	switch cs {
	case ConnectionActive:
		return "Active"
	case ConnectionDegraded:
		return "Degraded"
	case ConnectionInactive:
		return "Inactive"
	case ConnectionFailed:
		return "Failed"
	default:
		return "Unknown"
	}
}

// Backend represents a storage backend connection
type Backend struct {
	ID       string
	Name     string
	Address  string
	Priority int  // Lower numbers = higher priority
	Primary  bool // Is this the primary backend?
}

// BackendConnection tracks the connection state for a backend
type BackendConnection struct {
	Backend    *Backend
	Status     ConnectionStatus
	LastCheck  time.Time
	FailCount  int64
	CircuitBreaker *CircuitBreaker
	HealthMonitor  *ComponentHealth
	mu         sync.RWMutex
}

// ConnectionManagerConfig holds configuration for the connection manager
type ConnectionManagerConfig struct {
	// HealthCheckInterval is how often to check backend health
	HealthCheckInterval time.Duration
	// FailoverTimeout is how long to wait before failing over
	FailoverTimeout time.Duration
	// MaxFailures is the number of failures before marking as failed
	MaxFailures int64
	// RetryBackoffBase is the base delay for exponential backoff
	RetryBackoffBase time.Duration
	// MaxRetryDelay is the maximum retry delay
	MaxRetryDelay time.Duration
	// ConnectionTimeout is the timeout for individual connections
	ConnectionTimeout time.Duration
}

// DefaultConnectionManagerConfig returns a sensible default configuration
func DefaultConnectionManagerConfig() *ConnectionManagerConfig {
	return &ConnectionManagerConfig{
		HealthCheckInterval: 30 * time.Second,
		FailoverTimeout:     5 * time.Second,
		MaxFailures:         3,
		RetryBackoffBase:    100 * time.Millisecond,
		MaxRetryDelay:       30 * time.Second,
		ConnectionTimeout:   10 * time.Second,
	}
}

// ConnectionManager manages connections to multiple storage backends
type ConnectionManager struct {
	config      *ConnectionManagerConfig
	backends    map[string]*BackendConnection
	primary     *BackendConnection
	secondary   *BackendConnection
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	healthMonitor *HealthMonitor

	// Callbacks
	onFailover func(from, to *Backend)
	onBackendStatusChange func(backend *Backend, oldStatus, newStatus ConnectionStatus)
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager(config *ConnectionManagerConfig) *ConnectionManager {
	if config == nil {
		config = DefaultConnectionManagerConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create health monitor for backend monitoring
	healthConfig := &HealthMonitorConfig{
		CheckInterval:      config.HealthCheckInterval,
		CheckTimeout:       config.ConnectionTimeout,
		MaxRecentResults:   20,
		DegradedThreshold:  2,
		UnhealthyThreshold: config.MaxFailures,
		CriticalThreshold:  config.MaxFailures * 2,
		RecoveryThreshold:  1,
	}

	return &ConnectionManager{
		config:        config,
		backends:      make(map[string]*BackendConnection),
		ctx:           ctx,
		cancel:        cancel,
		healthMonitor: NewHealthMonitor(healthConfig),
	}
}

// AddBackend adds a new backend to the connection manager
func (cm *ConnectionManager) AddBackend(backend *Backend, healthCheck HealthCheck) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if _, exists := cm.backends[backend.ID]; exists {
		return fmt.Errorf("backend '%s' already exists", backend.ID)
	}

	// Create circuit breaker for this backend
	cbConfig := DefaultCircuitBreakerConfig(backend.Name)
	cbConfig.FailureThreshold = cm.config.MaxFailures
	cbConfig.Timeout = cm.config.ConnectionTimeout
	circuitBreaker := NewCircuitBreaker(cbConfig)

	connection := &BackendConnection{
		Backend:        backend,
		Status:         ConnectionUnknown,
		CircuitBreaker: circuitBreaker,
	}

	cm.backends[backend.ID] = connection

	// Register with health monitor
	cm.healthMonitor.RegisterComponent(backend.ID, healthCheck)

	// Set as primary/secondary based on priority
	if backend.Primary || cm.primary == nil {
		cm.primary = connection
	} else if cm.secondary == nil || backend.Priority < cm.secondary.Backend.Priority {
		cm.secondary = connection
	}

	return nil
}

// RemoveBackend removes a backend from the connection manager
func (cm *ConnectionManager) RemoveBackend(backendID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	connection, exists := cm.backends[backendID]
	if !exists {
		return fmt.Errorf("backend '%s' not found", backendID)
	}

	delete(cm.backends, backendID)
	cm.healthMonitor.UnregisterComponent(backendID)

	// Update primary/secondary if necessary
	if cm.primary == connection {
		cm.primary = cm.findBestBackend(true)
	}
	if cm.secondary == connection {
		cm.secondary = cm.findBestBackend(false)
	}

	return nil
}

// GetPrimaryBackend returns the current primary backend
func (cm *ConnectionManager) GetPrimaryBackend() *Backend {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if cm.primary != nil {
		return cm.primary.Backend
	}
	return nil
}

// GetSecondaryBackend returns the current secondary backend
func (cm *ConnectionManager) GetSecondaryBackend() *Backend {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if cm.secondary != nil {
		return cm.secondary.Backend
	}
	return nil
}

// GetActiveBackend returns the best available backend for operations
func (cm *ConnectionManager) GetActiveBackend() *Backend {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Check primary first
	if cm.primary != nil && cm.isBackendAvailable(cm.primary) {
		return cm.primary.Backend
	}

	// Fallback to secondary
	if cm.secondary != nil && cm.isBackendAvailable(cm.secondary) {
		return cm.secondary.Backend
	}

	// Try to find any available backend
	for _, conn := range cm.backends {
		if cm.isBackendAvailable(conn) {
			return conn.Backend
		}
	}

	return nil
}

// ExecuteWithBackend executes a function with the best available backend
func (cm *ConnectionManager) ExecuteWithBackend(ctx context.Context, fn func(context.Context, *Backend) error) error {
	backend := cm.GetActiveBackend()
	if backend == nil {
		return fmt.Errorf("no available backends")
	}

	cm.mu.RLock()
	connection := cm.backends[backend.ID]
	cm.mu.RUnlock()

	// Execute with circuit breaker protection
	return connection.CircuitBreaker.Execute(ctx, func(ctx context.Context) error {
		return fn(ctx, backend)
	})
}

// ExecuteWithFailover executes a function with automatic failover to secondary backend
func (cm *ConnectionManager) ExecuteWithFailover(ctx context.Context, fn func(context.Context, *Backend) error) error {
	// Try primary backend first
	primary := cm.GetPrimaryBackend()
	if primary != nil {
		cm.mu.RLock()
		connection := cm.backends[primary.ID]
		cm.mu.RUnlock()

		err := connection.CircuitBreaker.Execute(ctx, func(ctx context.Context) error {
			return fn(ctx, primary)
		})

		// If successful or circuit is open, return
		if err == nil || IsCircuitOpenError(err) {
			return err
		}

		// Try failover if we have a secondary and primary failed
		secondary := cm.GetSecondaryBackend()
		if secondary != nil && secondary.ID != primary.ID {
			cm.triggerFailover(primary, secondary)
			
			cm.mu.RLock()
			secondaryConnection := cm.backends[secondary.ID]
			cm.mu.RUnlock()

			return secondaryConnection.CircuitBreaker.Execute(ctx, func(ctx context.Context) error {
				return fn(ctx, secondary)
			})
		}

		return err
	}

	// No primary, try any available backend
	return cm.ExecuteWithBackend(ctx, fn)
}

// GetBackendStatus returns the status of a specific backend
func (cm *ConnectionManager) GetBackendStatus(backendID string) (ConnectionStatus, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	connection, exists := cm.backends[backendID]
	if !exists {
		return ConnectionUnknown, fmt.Errorf("backend '%s' not found", backendID)
	}

	connection.mu.RLock()
	defer connection.mu.RUnlock()
	return connection.Status, nil
}

// GetAllBackendStatuses returns the status of all backends
func (cm *ConnectionManager) GetAllBackendStatuses() map[string]ConnectionStatus {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	statuses := make(map[string]ConnectionStatus)
	for id, connection := range cm.backends {
		connection.mu.RLock()
		statuses[id] = connection.Status
		connection.mu.RUnlock()
	}

	return statuses
}

// SetFailoverCallback sets a callback for when failover occurs
func (cm *ConnectionManager) SetFailoverCallback(callback func(from, to *Backend)) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.onFailover = callback
}

// SetBackendStatusChangeCallback sets a callback for backend status changes
func (cm *ConnectionManager) SetBackendStatusChangeCallback(callback func(backend *Backend, oldStatus, newStatus ConnectionStatus)) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.onBackendStatusChange = callback
}

// Start begins monitoring backend health
func (cm *ConnectionManager) Start() {
	// Set up health monitor callback to update backend status
	cm.healthMonitor.SetStatusChangeCallback(cm.handleHealthStatusChange)
	
	// Start monitoring
	cm.wg.Add(1)
	go cm.monitorBackends()
}

// Stop stops the connection manager
func (cm *ConnectionManager) Stop() {
	cm.cancel()
	cm.healthMonitor.Stop()
	cm.wg.Wait()
}

// isBackendAvailable checks if a backend is available for operations
func (cm *ConnectionManager) isBackendAvailable(connection *BackendConnection) bool {
	connection.mu.RLock()
	defer connection.mu.RUnlock()

	switch connection.Status {
	case ConnectionActive, ConnectionDegraded:
		return connection.CircuitBreaker.GetState() != StateOpen
	default:
		return false
	}
}

// findBestBackend finds the best available backend (excluding current primary if excludePrimary is true)
func (cm *ConnectionManager) findBestBackend(excludePrimary bool) *BackendConnection {
	var best *BackendConnection
	bestPriority := int(^uint(0) >> 1) // Max int

	for _, connection := range cm.backends {
		if excludePrimary && cm.primary == connection {
			continue
		}

		if cm.isBackendAvailable(connection) && connection.Backend.Priority < bestPriority {
			best = connection
			bestPriority = connection.Backend.Priority
		}
	}

	return best
}

// triggerFailover triggers a failover from one backend to another
func (cm *ConnectionManager) triggerFailover(from, to *Backend) {
	cm.mu.Lock()
	
	// Update primary/secondary assignments
	if cm.primary != nil && cm.primary.Backend.ID == from.ID {
		cm.primary = cm.backends[to.ID]
	}
	
	callback := cm.onFailover
	cm.mu.Unlock()

	// Call failover callback
	if callback != nil {
		go callback(from, to)
	}
}

// handleHealthStatusChange handles health status changes from the health monitor
func (cm *ConnectionManager) handleHealthStatusChange(componentName string, oldStatus, newStatus HealthStatus) {
	cm.mu.RLock()
	connection, exists := cm.backends[componentName]
	cm.mu.RUnlock()

	if !exists {
		return
	}

	connection.mu.Lock()
	oldConnectionStatus := connection.Status

	// Map health status to connection status
	switch newStatus {
	case HealthHealthy:
		connection.Status = ConnectionActive
	case HealthDegraded:
		connection.Status = ConnectionDegraded
	case HealthUnhealthy, HealthCritical:
		connection.Status = ConnectionInactive
	default:
		connection.Status = ConnectionUnknown
	}

	newConnectionStatus := connection.Status
	connection.mu.Unlock()

	// Call status change callback
	cm.mu.RLock()
	callback := cm.onBackendStatusChange
	cm.mu.RUnlock()

	if callback != nil && oldConnectionStatus != newConnectionStatus {
		go callback(connection.Backend, oldConnectionStatus, newConnectionStatus)
	}
}

// monitorBackends continuously monitors backend health and manages failover
func (cm *ConnectionManager) monitorBackends() {
	defer cm.wg.Done()

	ticker := time.NewTicker(cm.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-cm.ctx.Done():
			return
		case <-ticker.C:
			cm.performHealthChecks()
		}
	}
}

// performHealthChecks performs health checks on all backends
func (cm *ConnectionManager) performHealthChecks() {
	// Health checks are handled by the embedded HealthMonitor
	// This method can be extended for additional monitoring logic
	results := cm.healthMonitor.CheckAllNow()
	
	// Log or handle health check results as needed
	_ = results
}
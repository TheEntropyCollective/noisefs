package resilience

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// HealthStatus represents the health state of a component
type HealthStatus int

const (
	// HealthUnknown - health status is not yet determined
	HealthUnknown HealthStatus = iota
	// HealthHealthy - component is functioning normally
	HealthHealthy
	// HealthDegraded - component is functioning but with reduced performance
	HealthDegraded
	// HealthUnhealthy - component is not functioning properly
	HealthUnhealthy
	// HealthCritical - component is in critical failure state
	HealthCritical
)

// String returns the string representation of HealthStatus
func (hs HealthStatus) String() string {
	switch hs {
	case HealthHealthy:
		return "Healthy"
	case HealthDegraded:
		return "Degraded"
	case HealthUnhealthy:
		return "Unhealthy"
	case HealthCritical:
		return "Critical"
	default:
		return "Unknown"
	}
}

// HealthCheck represents a health check function
type HealthCheck func(ctx context.Context) error

// HealthCheckResult contains the result of a health check
type HealthCheckResult struct {
	Name        string        `json:"name"`
	Status      HealthStatus  `json:"status"`
	Error       string        `json:"error,omitempty"`
	Duration    time.Duration `json:"duration"`
	Timestamp   time.Time     `json:"timestamp"`
	Message     string        `json:"message,omitempty"`
	Details     interface{}   `json:"details,omitempty"`
}

// ComponentHealth tracks the health state of a component over time
type ComponentHealth struct {
	Name             string                `json:"name"`
	Status           HealthStatus          `json:"status"`
	LastCheck        time.Time             `json:"last_check"`
	LastHealthy      time.Time             `json:"last_healthy"`
	ConsecutiveFailures int64              `json:"consecutive_failures"`
	TotalChecks      int64                 `json:"total_checks"`
	TotalFailures    int64                 `json:"total_failures"`
	AvgDuration      time.Duration         `json:"avg_duration"`
	RecentResults    []HealthCheckResult   `json:"recent_results"`
	mu               sync.RWMutex
}

// HealthMonitorConfig holds configuration for the health monitor
type HealthMonitorConfig struct {
	// CheckInterval is how often to run health checks
	CheckInterval time.Duration
	// CheckTimeout is the timeout for individual health checks
	CheckTimeout time.Duration
	// MaxRecentResults is the number of recent results to keep
	MaxRecentResults int
	// DegradedThreshold is consecutive failures before marking as degraded
	DegradedThreshold int64
	// UnhealthyThreshold is consecutive failures before marking as unhealthy
	UnhealthyThreshold int64
	// CriticalThreshold is consecutive failures before marking as critical
	CriticalThreshold int64
	// RecoveryThreshold is consecutive successes needed to mark as healthy
	RecoveryThreshold int64
}

// DefaultHealthMonitorConfig returns a sensible default configuration
func DefaultHealthMonitorConfig() *HealthMonitorConfig {
	return &HealthMonitorConfig{
		CheckInterval:      30 * time.Second,
		CheckTimeout:       10 * time.Second,
		MaxRecentResults:   50,
		DegradedThreshold:  2,
		UnhealthyThreshold: 5,
		CriticalThreshold:  10,
		RecoveryThreshold:  3,
	}
}

// HealthMonitor manages health checks for multiple components
type HealthMonitor struct {
	config     *HealthMonitorConfig
	components map[string]*ComponentHealth
	checks     map[string]HealthCheck
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup

	// Callbacks
	onStatusChange func(componentName string, oldStatus, newStatus HealthStatus)
}

// NewHealthMonitor creates a new health monitor
func NewHealthMonitor(config *HealthMonitorConfig) *HealthMonitor {
	if config == nil {
		config = DefaultHealthMonitorConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &HealthMonitor{
		config:     config,
		components: make(map[string]*ComponentHealth),
		checks:     make(map[string]HealthCheck),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// RegisterComponent registers a component for health monitoring
func (hm *HealthMonitor) RegisterComponent(name string, healthCheck HealthCheck) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	hm.components[name] = &ComponentHealth{
		Name:          name,
		Status:        HealthUnknown,
		RecentResults: make([]HealthCheckResult, 0, hm.config.MaxRecentResults),
	}
	hm.checks[name] = healthCheck

	// Start monitoring this component
	hm.wg.Add(1)
	go hm.monitorComponent(name)
}

// UnregisterComponent removes a component from health monitoring
func (hm *HealthMonitor) UnregisterComponent(name string) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	delete(hm.components, name)
	delete(hm.checks, name)
}

// GetComponentHealth returns the current health status of a component
func (hm *HealthMonitor) GetComponentHealth(name string) (*ComponentHealth, bool) {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	component, exists := hm.components[name]
	if !exists {
		return nil, false
	}

	// Return a copy to avoid race conditions
	component.mu.RLock()
	defer component.mu.RUnlock()

	copy := *component
	copy.RecentResults = make([]HealthCheckResult, len(component.RecentResults))
	for i, result := range component.RecentResults {
		copy.RecentResults[i] = result
	}

	return &copy, true
}

// GetOverallHealth returns the overall health status across all components
func (hm *HealthMonitor) GetOverallHealth() HealthStatus {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	if len(hm.components) == 0 {
		return HealthUnknown
	}

	worstStatus := HealthHealthy
	for _, component := range hm.components {
		component.mu.RLock()
		status := component.Status
		component.mu.RUnlock()

		if status > worstStatus {
			worstStatus = status
		}
	}

	return worstStatus
}

// GetAllComponentsHealth returns health status for all components
func (hm *HealthMonitor) GetAllComponentsHealth() map[string]*ComponentHealth {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	result := make(map[string]*ComponentHealth)
	for name, component := range hm.components {
		component.mu.RLock()
		copy := *component
		copy.RecentResults = make([]HealthCheckResult, len(component.RecentResults))
		for i, res := range component.RecentResults {
			copy.RecentResults[i] = res
		}
		component.mu.RUnlock()
		result[name] = &copy
	}

	return result
}

// SetStatusChangeCallback sets a callback for when component status changes
func (hm *HealthMonitor) SetStatusChangeCallback(callback func(componentName string, oldStatus, newStatus HealthStatus)) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.onStatusChange = callback
}

// CheckNow performs an immediate health check for a specific component
func (hm *HealthMonitor) CheckNow(componentName string) (*HealthCheckResult, error) {
	hm.mu.RLock()
	check, exists := hm.checks[componentName]
	component, componentExists := hm.components[componentName]
	hm.mu.RUnlock()

	if !exists || !componentExists {
		return nil, fmt.Errorf("component '%s' not registered", componentName)
	}

	return hm.performHealthCheck(componentName, check, component)
}

// CheckAllNow performs immediate health checks for all components
func (hm *HealthMonitor) CheckAllNow() map[string]*HealthCheckResult {
	hm.mu.RLock()
	components := make(map[string]*ComponentHealth)
	checks := make(map[string]HealthCheck)
	for name, component := range hm.components {
		components[name] = component
		checks[name] = hm.checks[name]
	}
	hm.mu.RUnlock()

	results := make(map[string]*HealthCheckResult)
	for name, check := range checks {
		if result, err := hm.performHealthCheck(name, check, components[name]); err == nil {
			results[name] = result
		}
	}

	return results
}

// Start begins the health monitoring process
func (hm *HealthMonitor) Start() {
	// Health monitoring starts automatically when components are registered
	// This method exists for interface compatibility
}

// Stop stops the health monitoring process
func (hm *HealthMonitor) Stop() {
	hm.cancel()
	hm.wg.Wait()
}

// monitorComponent runs periodic health checks for a component
func (hm *HealthMonitor) monitorComponent(componentName string) {
	defer hm.wg.Done()

	ticker := time.NewTicker(hm.config.CheckInterval)
	defer ticker.Stop()

	// Perform initial check
	hm.performComponentCheck(componentName)

	for {
		select {
		case <-hm.ctx.Done():
			return
		case <-ticker.C:
			hm.performComponentCheck(componentName)
		}
	}
}

// performComponentCheck performs a health check for a component
func (hm *HealthMonitor) performComponentCheck(componentName string) {
	hm.mu.RLock()
	check, exists := hm.checks[componentName]
	component, componentExists := hm.components[componentName]
	hm.mu.RUnlock()

	if !exists || !componentExists {
		return
	}

	hm.performHealthCheck(componentName, check, component)
}

// performHealthCheck executes a health check and updates component status
func (hm *HealthMonitor) performHealthCheck(componentName string, check HealthCheck, component *ComponentHealth) (*HealthCheckResult, error) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(hm.ctx, hm.config.CheckTimeout)
	defer cancel()

	startTime := time.Now()
	err := check(ctx)
	duration := time.Since(startTime)

	result := HealthCheckResult{
		Name:      componentName,
		Duration:  duration,
		Timestamp: startTime,
	}

	if err != nil {
		result.Status = HealthUnhealthy
		result.Error = err.Error()
	} else {
		result.Status = HealthHealthy
	}

	// Update component health
	hm.updateComponentHealth(component, result)

	return &result, nil
}

// updateComponentHealth updates the health status of a component based on check result
func (hm *HealthMonitor) updateComponentHealth(component *ComponentHealth, result HealthCheckResult) {
	component.mu.Lock()
	defer component.mu.Unlock()

	oldStatus := component.Status
	component.LastCheck = result.Timestamp
	component.TotalChecks++

	// Update average duration
	if component.TotalChecks == 1 {
		component.AvgDuration = result.Duration
	} else {
		component.AvgDuration = time.Duration(
			(int64(component.AvgDuration)*int64(component.TotalChecks-1) + int64(result.Duration)) / int64(component.TotalChecks),
		)
	}

	if result.Status == HealthHealthy {
		component.LastHealthy = result.Timestamp
		component.ConsecutiveFailures = 0
		component.Status = HealthHealthy
	} else {
		component.ConsecutiveFailures++
		component.TotalFailures++

		// Determine new status based on consecutive failures
		if component.ConsecutiveFailures >= hm.config.CriticalThreshold {
			component.Status = HealthCritical
		} else if component.ConsecutiveFailures >= hm.config.UnhealthyThreshold {
			component.Status = HealthUnhealthy
		} else if component.ConsecutiveFailures >= hm.config.DegradedThreshold {
			component.Status = HealthDegraded
		}
	}

	// Add to recent results
	component.RecentResults = append(component.RecentResults, result)
	if len(component.RecentResults) > hm.config.MaxRecentResults {
		component.RecentResults = component.RecentResults[1:]
	}

	// Call status change callback if status changed
	if oldStatus != component.Status && hm.onStatusChange != nil {
		go hm.onStatusChange(component.Name, oldStatus, component.Status)
	}
}

// IsHealthy returns true if the component is healthy
func (ch *ComponentHealth) IsHealthy() bool {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return ch.Status == HealthHealthy
}

// IsDegraded returns true if the component is degraded or worse
func (ch *ComponentHealth) IsDegraded() bool {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return ch.Status >= HealthDegraded
}

// IsUnhealthy returns true if the component is unhealthy or worse
func (ch *ComponentHealth) IsUnhealthy() bool {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return ch.Status >= HealthUnhealthy
}

// IsCritical returns true if the component is in critical state
func (ch *ComponentHealth) IsCritical() bool {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return ch.Status == HealthCritical
}

// GetSuccessRate returns the success rate over recent checks
func (ch *ComponentHealth) GetSuccessRate() float64 {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	if len(ch.RecentResults) == 0 {
		return 0.0
	}

	successes := 0
	for _, result := range ch.RecentResults {
		if result.Status == HealthHealthy {
			successes++
		}
	}

	return float64(successes) / float64(len(ch.RecentResults))
}

// HealthSummary provides a summary of system health
type HealthSummary struct {
	OverallStatus    HealthStatus                   `json:"overall_status"`
	TotalComponents  int                            `json:"total_components"`
	HealthyCount     int                            `json:"healthy_count"`
	DegradedCount    int                            `json:"degraded_count"`
	UnhealthyCount   int                            `json:"unhealthy_count"`
	CriticalCount    int                            `json:"critical_count"`
	Components       map[string]*ComponentHealth    `json:"components"`
	Timestamp        time.Time                      `json:"timestamp"`
}

// GetHealthSummary returns a comprehensive health summary
func (hm *HealthMonitor) GetHealthSummary() *HealthSummary {
	components := hm.GetAllComponentsHealth()
	
	summary := &HealthSummary{
		TotalComponents: len(components),
		Components:      components,
		Timestamp:       time.Now(),
	}

	for _, component := range components {
		switch component.Status {
		case HealthHealthy:
			summary.HealthyCount++
		case HealthDegraded:
			summary.DegradedCount++
		case HealthUnhealthy:
			summary.UnhealthyCount++
		case HealthCritical:
			summary.CriticalCount++
		}
	}

	summary.OverallStatus = hm.GetOverallHealth()
	return summary
}
package storage

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// HealthMonitor continuously monitors the health of storage backends
type HealthMonitor struct {
	manager *Manager
	config  *HealthCheckConfig

	// State management
	running  bool
	stopChan chan struct{}
	mutex    sync.RWMutex

	// Health tracking
	healthHistory map[string][]*HealthStatus
	alerts        []HealthAlert
	lastCheck     time.Time
}

// NewHealthMonitor creates a new health monitor
func NewHealthMonitor(manager *Manager, config *HealthCheckConfig) *HealthMonitor {
	return &HealthMonitor{
		manager:       manager,
		config:        config,
		stopChan:      make(chan struct{}),
		healthHistory: make(map[string][]*HealthStatus),
		alerts:        make([]HealthAlert, 0),
	}
}

// Start begins health monitoring
func (hm *HealthMonitor) Start(ctx context.Context) error {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()

	if hm.running {
		return fmt.Errorf("health monitor already running")
	}

	if !hm.config.Enabled {
		return nil // Health monitoring disabled
	}

	hm.running = true

	// Start monitoring goroutine
	go hm.monitorLoop(ctx)

	return nil
}

// Stop stops health monitoring
func (hm *HealthMonitor) Stop() {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()

	if !hm.running {
		return
	}

	hm.running = false
	close(hm.stopChan)
}

// GetHealthHistory returns health history for a backend
func (hm *HealthMonitor) GetHealthHistory(backendName string) []*HealthStatus {
	hm.mutex.RLock()
	defer hm.mutex.RUnlock()

	history, exists := hm.healthHistory[backendName]
	if !exists {
		return []*HealthStatus{}
	}

	// Return a copy to prevent concurrent access issues
	result := make([]*HealthStatus, len(history))
	copy(result, history)
	return result
}

// GetAlerts returns current health alerts
func (hm *HealthMonitor) GetAlerts() []HealthAlert {
	hm.mutex.RLock()
	defer hm.mutex.RUnlock()

	// Return a copy
	result := make([]HealthAlert, len(hm.alerts))
	copy(result, hm.alerts)
	return result
}

// GetActiveAlerts returns only active (unresolved) alerts
func (hm *HealthMonitor) GetActiveAlerts() []HealthAlert {
	hm.mutex.RLock()
	defer hm.mutex.RUnlock()

	var active []HealthAlert
	for _, alert := range hm.alerts {
		if !alert.Resolved {
			active = append(active, alert)
		}
	}
	return active
}

// Main monitoring loop
func (hm *HealthMonitor) monitorLoop(ctx context.Context) {
	ticker := time.NewTicker(hm.config.Interval)
	defer ticker.Stop()

	// Perform initial health check
	hm.performHealthCheck(ctx)

	for {
		select {
		case <-ticker.C:
			hm.performHealthCheck(ctx)
		case <-hm.stopChan:
			return
		case <-ctx.Done():
			return
		}
	}
}

// performHealthCheck checks health of all backends
func (hm *HealthMonitor) performHealthCheck(ctx context.Context) {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()

	hm.lastCheck = time.Now()
	backends := hm.manager.GetAvailableBackends()

	for name, backend := range backends {
		// Create context with timeout for health check
		checkCtx, cancel := context.WithTimeout(ctx, hm.config.Timeout)

		status := backend.HealthCheck(checkCtx)
		cancel()

		// Store health status in history
		hm.recordHealthStatus(name, status)

		// Check for health state changes and generate alerts
		hm.checkHealthAlerts(name, status)

		// Take actions based on health status
		hm.takeHealthActions(name, backend, status)
	}

	// Clean up old health records
	hm.cleanupHealthHistory()
}

// recordHealthStatus stores a health status in the history
func (hm *HealthMonitor) recordHealthStatus(backendName string, status *HealthStatus) {
	history, exists := hm.healthHistory[backendName]
	if !exists {
		history = make([]*HealthStatus, 0)
	}

	// Add new status
	history = append(history, status)

	// Keep only last 100 records
	if len(history) > 100 {
		history = history[1:]
	}

	hm.healthHistory[backendName] = history
}

// checkHealthAlerts generates alerts based on health status changes
func (hm *HealthMonitor) checkHealthAlerts(backendName string, status *HealthStatus) {
	// Get previous status
	history := hm.healthHistory[backendName]
	var previousStatus *HealthStatus
	if len(history) > 1 {
		previousStatus = history[len(history)-2]
	}

	// Check for health state changes
	if previousStatus != nil {
		if previousStatus.Healthy && !status.Healthy {
			// Backend became unhealthy
			alert := HealthAlert{
				ID:          hm.generateAlertID(),
				BackendName: backendName,
				Type:        "backend_unhealthy",
				Severity:    "warning",
				Message:     fmt.Sprintf("Backend '%s' became unhealthy: %s", backendName, status.Status),
				Timestamp:   time.Now(),
				Resolved:    false,
				Metadata: map[string]interface{}{
					"previous_status": previousStatus.Status,
					"current_status":  status.Status,
					"error_rate":      status.ErrorRate,
					"latency":         status.Latency,
				},
			}
			hm.alerts = append(hm.alerts, alert)
		} else if !previousStatus.Healthy && status.Healthy {
			// Backend recovered
			alert := HealthAlert{
				ID:          hm.generateAlertID(),
				BackendName: backendName,
				Type:        "backend_recovered",
				Severity:    "info",
				Message:     fmt.Sprintf("Backend '%s' recovered and is now healthy", backendName),
				Timestamp:   time.Now(),
				Resolved:    true,
				Metadata: map[string]interface{}{
					"previous_status": previousStatus.Status,
					"current_status":  status.Status,
				},
			}
			hm.alerts = append(hm.alerts, alert)

			// Resolve previous unhealthy alerts for this backend
			hm.resolveAlertsForBackend(backendName, "backend_unhealthy")
		}
	}

	// Check specific thresholds
	if hm.config.Thresholds != nil {
		hm.checkLatencyThreshold(backendName, status)
		hm.checkErrorRateThreshold(backendName, status)
		hm.checkConsecutiveFailures(backendName, status)
	}
}

// checkLatencyThreshold checks if latency exceeds threshold
func (hm *HealthMonitor) checkLatencyThreshold(backendName string, status *HealthStatus) {
	if hm.config.Thresholds.MaxLatency > 0 && status.Latency > hm.config.Thresholds.MaxLatency {
		// Check if we already have an active latency alert
		if !hm.hasActiveAlert(backendName, "high_latency") {
			alert := HealthAlert{
				ID:          hm.generateAlertID(),
				BackendName: backendName,
				Type:        "high_latency",
				Severity:    "warning",
				Message: fmt.Sprintf("Backend '%s' latency exceeds threshold: %v > %v",
					backendName, status.Latency, hm.config.Thresholds.MaxLatency),
				Timestamp: time.Now(),
				Resolved:  false,
				Metadata: map[string]interface{}{
					"current_latency": status.Latency,
					"threshold":       hm.config.Thresholds.MaxLatency,
				},
			}
			hm.alerts = append(hm.alerts, alert)
		}
	} else {
		// Resolve latency alerts if latency is now acceptable
		hm.resolveAlertsForBackend(backendName, "high_latency")
	}
}

// checkErrorRateThreshold checks if error rate exceeds threshold
func (hm *HealthMonitor) checkErrorRateThreshold(backendName string, status *HealthStatus) {
	if hm.config.Thresholds.MaxErrorRate > 0 && status.ErrorRate > hm.config.Thresholds.MaxErrorRate {
		if !hm.hasActiveAlert(backendName, "high_error_rate") {
			alert := HealthAlert{
				ID:          hm.generateAlertID(),
				BackendName: backendName,
				Type:        "high_error_rate",
				Severity:    "warning",
				Message: fmt.Sprintf("Backend '%s' error rate exceeds threshold: %.2f%% > %.2f%%",
					backendName, status.ErrorRate*100, hm.config.Thresholds.MaxErrorRate*100),
				Timestamp: time.Now(),
				Resolved:  false,
				Metadata: map[string]interface{}{
					"current_error_rate": status.ErrorRate,
					"threshold":          hm.config.Thresholds.MaxErrorRate,
				},
			}
			hm.alerts = append(hm.alerts, alert)
		}
	} else {
		hm.resolveAlertsForBackend(backendName, "high_error_rate")
	}
}

// checkConsecutiveFailures checks for consecutive health check failures
func (hm *HealthMonitor) checkConsecutiveFailures(backendName string, status *HealthStatus) {
	if hm.config.Thresholds.ConsecutiveFailures <= 0 {
		return
	}

	history := hm.healthHistory[backendName]
	if len(history) < hm.config.Thresholds.ConsecutiveFailures {
		return
	}

	// Check last N statuses for consecutive failures
	consecutiveFailures := 0
	for i := len(history) - 1; i >= 0 && consecutiveFailures < hm.config.Thresholds.ConsecutiveFailures; i-- {
		if !history[i].Healthy {
			consecutiveFailures++
		} else {
			break
		}
	}

	if consecutiveFailures >= hm.config.Thresholds.ConsecutiveFailures {
		if !hm.hasActiveAlert(backendName, "consecutive_failures") {
			alert := HealthAlert{
				ID:          hm.generateAlertID(),
				BackendName: backendName,
				Type:        "consecutive_failures",
				Severity:    "error",
				Message: fmt.Sprintf("Backend '%s' has %d consecutive health check failures",
					backendName, consecutiveFailures),
				Timestamp: time.Now(),
				Resolved:  false,
				Metadata: map[string]interface{}{
					"consecutive_failures": consecutiveFailures,
					"threshold":            hm.config.Thresholds.ConsecutiveFailures,
				},
			}
			hm.alerts = append(hm.alerts, alert)
		}
	} else {
		hm.resolveAlertsForBackend(backendName, "consecutive_failures")
	}
}

// takeHealthActions performs actions based on health status
func (hm *HealthMonitor) takeHealthActions(backendName string, backend Backend, status *HealthStatus) {
	if hm.config.Actions == nil {
		return
	}

	if !status.Healthy {
		switch hm.config.Actions.OnUnhealthy {
		case "disable":
			// Would need to implement backend disabling in manager
			// For now, just log the action
		case "deprioritize":
			// Would need to implement priority adjustment
			// For now, just log the action
		case "quarantine":
			// Would need to implement quarantine functionality
			// For now, just log the action
		}
	} else {
		switch hm.config.Actions.OnRecovered {
		case "enable":
			// Re-enable previously disabled backend
		case "restore_priority":
			// Restore original priority
		}
	}
}

// Helper methods

func (hm *HealthMonitor) hasActiveAlert(backendName, alertType string) bool {
	for _, alert := range hm.alerts {
		if alert.BackendName == backendName && alert.Type == alertType && !alert.Resolved {
			return true
		}
	}
	return false
}

func (hm *HealthMonitor) resolveAlertsForBackend(backendName, alertType string) {
	for i := range hm.alerts {
		if hm.alerts[i].BackendName == backendName && hm.alerts[i].Type == alertType && !hm.alerts[i].Resolved {
			hm.alerts[i].Resolved = true
			hm.alerts[i].ResolvedAt = time.Now()
		}
	}
}

func (hm *HealthMonitor) generateAlertID() string {
	return fmt.Sprintf("alert-%d", time.Now().UnixNano())
}

func (hm *HealthMonitor) cleanupHealthHistory() {
	// Clean up health history older than retention period
	retentionCutoff := time.Now().Add(-24 * time.Hour) // Keep 24 hours of history

	for backendName, history := range hm.healthHistory {
		var cleaned []*HealthStatus
		for _, status := range history {
			if status.LastCheck.After(retentionCutoff) {
				cleaned = append(cleaned, status)
			}
		}
		hm.healthHistory[backendName] = cleaned
	}

	// Clean up old alerts (keep last 1000)
	if len(hm.alerts) > 1000 {
		hm.alerts = hm.alerts[len(hm.alerts)-1000:]
	}
}

// HealthAlert represents a health-related alert
type HealthAlert struct {
	ID          string                 `json:"id"`
	BackendName string                 `json:"backend_name"`
	Type        string                 `json:"type"`
	Severity    string                 `json:"severity"`
	Message     string                 `json:"message"`
	Timestamp   time.Time              `json:"timestamp"`
	Resolved    bool                   `json:"resolved"`
	ResolvedAt  time.Time              `json:"resolved_at,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// HealthSummary provides a summary of overall system health
type HealthSummary struct {
	OverallHealth   string            `json:"overall_health"`
	TotalBackends   int               `json:"total_backends"`
	HealthyBackends int               `json:"healthy_backends"`
	ActiveAlerts    int               `json:"active_alerts"`
	LastCheck       time.Time         `json:"last_check"`
	BackendHealth   map[string]string `json:"backend_health"`
	RecentAlerts    []HealthAlert     `json:"recent_alerts"`
}

// GetHealthSummary returns a summary of overall system health
func (hm *HealthMonitor) GetHealthSummary() *HealthSummary {
	hm.mutex.RLock()
	defer hm.mutex.RUnlock()

	backends := hm.manager.GetAvailableBackends()
	healthyCount := 0
	backendHealth := make(map[string]string)

	for name, backend := range backends {
		status := backend.HealthCheck(context.Background())
		backendHealth[name] = status.Status
		if status.Healthy {
			healthyCount++
		}
	}

	// Determine overall health
	overallHealth := "healthy"
	if healthyCount == 0 {
		overallHealth = "critical"
	} else if healthyCount < len(backends) {
		overallHealth = "degraded"
	}

	// Get recent alerts (last 10)
	var recentAlerts []HealthAlert
	startIdx := len(hm.alerts) - 10
	if startIdx < 0 {
		startIdx = 0
	}
	for i := startIdx; i < len(hm.alerts); i++ {
		recentAlerts = append(recentAlerts, hm.alerts[i])
	}

	return &HealthSummary{
		OverallHealth:   overallHealth,
		TotalBackends:   len(backends),
		HealthyBackends: healthyCount,
		ActiveAlerts:    len(hm.GetActiveAlerts()),
		LastCheck:       hm.lastCheck,
		BackendHealth:   backendHealth,
		RecentAlerts:    recentAlerts,
	}
}

// GetHealthTrends analyzes health trends over time
func (hm *HealthMonitor) GetHealthTrends(backendName string, duration time.Duration) *HealthTrends {
	hm.mutex.RLock()
	defer hm.mutex.RUnlock()

	history := hm.healthHistory[backendName]
	if len(history) == 0 {
		return &HealthTrends{}
	}

	cutoff := time.Now().Add(-duration)
	var recentHistory []*HealthStatus

	for _, status := range history {
		if status.LastCheck.After(cutoff) {
			recentHistory = append(recentHistory, status)
		}
	}

	if len(recentHistory) == 0 {
		return &HealthTrends{}
	}

	// Calculate trends
	healthyCount := 0
	var totalLatency time.Duration
	var maxLatency time.Duration
	var totalErrorRate float64

	for _, status := range recentHistory {
		if status.Healthy {
			healthyCount++
		}
		totalLatency += status.Latency
		if status.Latency > maxLatency {
			maxLatency = status.Latency
		}
		totalErrorRate += status.ErrorRate
	}

	trends := &HealthTrends{
		BackendName:      backendName,
		Period:           duration,
		SampleCount:      len(recentHistory),
		HealthyPercent:   float64(healthyCount) / float64(len(recentHistory)) * 100,
		AverageLatency:   totalLatency / time.Duration(len(recentHistory)),
		MaxLatency:       maxLatency,
		AverageErrorRate: totalErrorRate / float64(len(recentHistory)),
	}

	// Calculate availability (uptime percentage)
	trends.Availability = trends.HealthyPercent

	return trends
}

// HealthTrends represents health trends over a period
type HealthTrends struct {
	BackendName      string        `json:"backend_name"`
	Period           time.Duration `json:"period"`
	SampleCount      int           `json:"sample_count"`
	HealthyPercent   float64       `json:"healthy_percent"`
	Availability     float64       `json:"availability"`
	AverageLatency   time.Duration `json:"average_latency"`
	MaxLatency       time.Duration `json:"max_latency"`
	AverageErrorRate float64       `json:"average_error_rate"`
}

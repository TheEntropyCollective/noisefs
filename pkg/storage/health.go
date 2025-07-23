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
}

// NewHealthMonitor creates a new health monitor
func NewHealthMonitor(manager *Manager, config *HealthCheckConfig) *HealthMonitor {
	return &HealthMonitor{
		manager:  manager,
		config:   config,
		stopChan: make(chan struct{}),
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
	backends := hm.manager.GetAvailableBackends()

	for _, backend := range backends {
		// Create context with timeout for health check
		checkCtx, cancel := context.WithTimeout(ctx, hm.config.Timeout)

		// Perform simple connectivity check
		backend.HealthCheck(checkCtx)
		cancel()
	}
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

	return &HealthSummary{
		OverallHealth:   overallHealth,
		TotalBackends:   len(backends),
		HealthyBackends: healthyCount,
		LastCheck:       time.Now(),
		BackendHealth:   backendHealth,
	}
}

// HealthSummary provides a summary of overall system health
type HealthSummary struct {
	OverallHealth   string            `json:"overall_health"`
	TotalBackends   int               `json:"total_backends"`
	HealthyBackends int               `json:"healthy_backends"`
	LastCheck       time.Time         `json:"last_check"`
	BackendHealth   map[string]string `json:"backend_health"`
}

package storage

import (
	"context"
	"fmt"
	"time"
)

// defaultStatusAggregator implements the StatusAggregator interface
type defaultStatusAggregator struct {
	registry BackendRegistry
	config   *Config
}

// NewStatusAggregator creates a new status aggregator
func NewStatusAggregator(registry BackendRegistry, config *Config) StatusAggregator {
	return &defaultStatusAggregator{
		registry: registry,
		config:   config,
	}
}

// GetManagerStatus returns the current status of the manager
func (a *defaultStatusAggregator) GetManagerStatus() *ManagerStatus {
	allBackends := a.registry.GetAllBackends()
	availableBackends := a.registry.GetAvailableBackends()

	status := &ManagerStatus{
		Started:         len(availableBackends) > 0, // Manager is considered started if any backends are available
		TotalBackends:   len(a.config.Backends),
		ActiveBackends:  len(availableBackends),
		HealthyBackends: 0,
		BackendStatus:   make(map[string]*BackendStatus),
		LastCheck:       time.Now(),
	}

	for name, backend := range allBackends {
		backendStatus := a.GetBackendStatus(name, backend)
		if backendStatus.Healthy {
			status.HealthyBackends++
		}
		status.BackendStatus[name] = backendStatus
	}

	return status
}

// GetBackendStatuses returns the status of all backends
func (a *defaultStatusAggregator) GetBackendStatuses() map[string]*BackendStatus {
	allBackends := a.registry.GetAllBackends()
	statuses := make(map[string]*BackendStatus)

	for name, backend := range allBackends {
		statuses[name] = a.GetBackendStatus(name, backend)
	}

	return statuses
}

// GetHealthStatus returns the overall health status
func (a *defaultStatusAggregator) GetHealthStatus(ctx context.Context) *HealthStatus {
	status := a.GetManagerStatus()

	healthy := status.HealthyBackends > 0
	healthStr := "healthy"
	var issues []HealthIssue

	if status.HealthyBackends == 0 {
		healthy = false
		healthStr = "critical"
		issues = append(issues, HealthIssue{
			Severity:    "critical",
			Code:        "NO_HEALTHY_BACKENDS",
			Description: "No healthy backends available",
			Timestamp:   time.Now(),
		})
	} else if status.HealthyBackends < status.ActiveBackends {
		healthStr = "degraded"
		issues = append(issues, HealthIssue{
			Severity: "warning",
			Code:     "SOME_BACKENDS_UNHEALTHY",
			Description: fmt.Sprintf("%d of %d backends unhealthy",
				status.ActiveBackends-status.HealthyBackends, status.ActiveBackends),
			Timestamp: time.Now(),
		})
	}

	return &HealthStatus{
		Healthy:   healthy,
		Status:    healthStr,
		LastCheck: time.Now(),
		Issues:    issues,
	}
}

// GetTotalBackends returns the total number of configured backends
func (a *defaultStatusAggregator) GetTotalBackends() int {
	return len(a.config.Backends)
}

// GetActiveBackends returns the number of currently active (available) backends
func (a *defaultStatusAggregator) GetActiveBackends() int {
	return len(a.registry.GetAvailableBackends())
}

// GetHealthyBackends returns the number of currently healthy backends
func (a *defaultStatusAggregator) GetHealthyBackends() int {
	return len(a.registry.GetHealthyBackends())
}

// GetConnectedPeerCount returns the total number of connected peers from peer-aware backends
func (a *defaultStatusAggregator) GetConnectedPeerCount() int {
	allBackends := a.registry.GetAllBackends()
	totalPeers := 0

	for _, backend := range allBackends {
		if peerAware, ok := backend.(PeerAwareBackend); ok {
			peers := peerAware.GetConnectedPeers()
			totalPeers += len(peers)
		}
	}

	return totalPeers
}

// GetBackendStatus returns the status of a specific backend
func (a *defaultStatusAggregator) GetBackendStatus(name string, backend Backend) *BackendStatus {
	backendHealth := backend.HealthCheck(context.Background())
	backendInfo := backend.GetBackendInfo()

	return &BackendStatus{
		Name:         name,
		Type:         backendInfo.Type,
		Connected:    backend.IsConnected(),
		Healthy:      backendHealth.Healthy,
		Status:       backendHealth.Status,
		Latency:      backendHealth.Latency,
		ErrorRate:    backendHealth.ErrorRate,
		LastCheck:    backendHealth.LastCheck,
		Capabilities: backendInfo.Capabilities,
	}
}

// GetBackendHealth returns the health status of a specific backend
func (a *defaultStatusAggregator) GetBackendHealth(name string, backend Backend) *HealthStatus {
	return backend.HealthCheck(context.Background())
}

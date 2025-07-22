package storage

import (
	"context"
	"fmt"
	"sort"
)

// defaultBackendSelector implements the BackendSelector interface
type defaultBackendSelector struct {
	registry BackendRegistry
	config   *Config
}

// NewBackendSelector creates a new backend selector
func NewBackendSelector(registry BackendRegistry, config *Config) BackendSelector {
	return &defaultBackendSelector{
		registry: registry,
		config:   config,
	}
}

// SelectBackend selects a backend based on the given criteria
func (s *defaultBackendSelector) SelectBackend(ctx context.Context, criteria SelectionCriteria) (Backend, error) {
	backends := s.getEligibleBackends(criteria)
	if len(backends) == 0 {
		return nil, NewStorageError(ErrCodeNoBackends, "no backends match selection criteria", "selector", nil)
	}
	
	// Apply selection strategy based on criteria
	return s.applySelectionStrategy(backends, criteria)
}

// GetBackendsByPriority returns backends sorted by priority and health
func (s *defaultBackendSelector) GetBackendsByPriority() []Backend {
	type backendInfo struct {
		backend  Backend
		name     string
		priority int
		healthy  bool
	}
	
	var infos []backendInfo
	allBackends := s.registry.GetAllBackends()
	
	for name, backend := range allBackends {
		if !backend.IsConnected() {
			continue
		}
		
		config, exists := s.config.Backends[name]
		if !exists {
			continue
		}
		
		status := backend.HealthCheck(context.Background())
		
		infos = append(infos, backendInfo{
			backend:  backend,
			name:     name,
			priority: config.Priority,
			healthy:  status.Healthy,
		})
	}
	
	// Sort by health first, then priority
	sort.Slice(infos, func(i, j int) bool {
		if infos[i].healthy != infos[j].healthy {
			return infos[i].healthy // Healthy backends first
		}
		return infos[i].priority > infos[j].priority // Higher priority first
	})
	
	backends := make([]Backend, len(infos))
	for i, info := range infos {
		backends[i] = info.backend
	}
	
	return backends
}

// GetDefaultBackend returns the default backend instance
func (s *defaultBackendSelector) GetDefaultBackend() (Backend, error) {
	defaultName := s.config.DefaultBackend
	backend, exists := s.registry.GetBackend(defaultName)
	if !exists {
		return nil, NewStorageError(ErrCodeNotFound, fmt.Sprintf("default backend '%s' not available", defaultName), defaultName, nil)
	}
	return backend, nil
}

// SelectBestBackend selects the best available backend for an operation
func (s *defaultBackendSelector) SelectBestBackend(ctx context.Context, criteria SelectionCriteria) (Backend, error) {
	// For now, this is identical to SelectBackend, but could be enhanced with more sophisticated logic
	return s.SelectBackend(ctx, criteria)
}

// SelectHealthyBackends returns the specified number of healthy backends
func (s *defaultBackendSelector) SelectHealthyBackends(count int) []Backend {
	healthyBackends := s.registry.GetHealthyBackends()
	
	// Convert map to slice for prioritization
	backends := make([]Backend, 0, len(healthyBackends))
	for _, backend := range healthyBackends {
		backends = append(backends, backend)
	}
	
	// Sort by priority
	prioritizedBackends := s.GetBackendsByPriority()
	
	// Filter to only include healthy ones and limit count
	var result []Backend
	for _, backend := range prioritizedBackends {
		status := backend.HealthCheck(context.Background())
		if status.Healthy {
			result = append(result, backend)
			if len(result) >= count {
				break
			}
		}
	}
	
	return result
}

// SelectBackendByCapability selects a backend that supports the specified capability
func (s *defaultBackendSelector) SelectBackendByCapability(capability string) (Backend, error) {
	backends := s.registry.GetBackendsWithCapability(capability)
	if len(backends) == 0 {
		return nil, NewStorageError(ErrCodeNotFound, fmt.Sprintf("no backends support capability '%s'", capability), "selector", nil)
	}
	
	// Return the first healthy backend with the capability
	for _, backend := range backends {
		status := backend.HealthCheck(context.Background())
		if status.Healthy {
			return backend, nil
		}
	}
	
	// If no healthy backends, return the first available one
	return backends[0], nil
}

// getEligibleBackends filters backends based on selection criteria
func (s *defaultBackendSelector) getEligibleBackends(criteria SelectionCriteria) []Backend {
	var eligible []Backend
	
	// Start with available backends
	available := s.registry.GetAvailableBackends()
	
	for name, backend := range available {
		// Check exclusions
		if s.isExcluded(name, criteria.ExcludeBackends) {
			continue
		}
		
		// Check capabilities
		if !s.hasRequiredCapabilities(backend, criteria.RequiredCapabilities) {
			continue
		}
		
		// Check health if preferred
		if criteria.PreferHealthy {
			status := backend.HealthCheck(context.Background())
			if !status.Healthy {
				continue
			}
		}
		
		// Check storage requirements
		if criteria.MinAvailableStorage > 0 {
			status := backend.HealthCheck(context.Background())
			if status.AvailableStorage < criteria.MinAvailableStorage {
				continue
			}
		}
		
		eligible = append(eligible, backend)
	}
	
	return eligible
}

// applySelectionStrategy applies the selection strategy to choose from eligible backends
func (s *defaultBackendSelector) applySelectionStrategy(backends []Backend, criteria SelectionCriteria) (Backend, error) {
	if len(backends) == 1 {
		return backends[0], nil
	}
	
	// Apply priority-based selection if preferred
	if criteria.PreferHighPriority {
		prioritized := s.GetBackendsByPriority()
		// Find the first backend from prioritized list that's in our eligible set
		for _, prioritizedBackend := range prioritized {
			for _, eligibleBackend := range backends {
				if prioritizedBackend == eligibleBackend {
					return prioritizedBackend, nil
				}
			}
		}
	}
	
	// Apply latency-based selection if preferred
	if criteria.PreferLowLatency {
		return s.selectByLatency(backends)
	}
	
	// Default: return the first backend
	return backends[0], nil
}

// selectByLatency selects the backend with the lowest latency
func (s *defaultBackendSelector) selectByLatency(backends []Backend) (Backend, error) {
	if len(backends) == 0 {
		return nil, NewStorageError(ErrCodeNoBackends, "no backends available for latency selection", "selector", nil)
	}
	
	bestBackend := backends[0]
	bestLatency := bestBackend.HealthCheck(context.Background()).Latency
	
	for _, backend := range backends[1:] {
		status := backend.HealthCheck(context.Background())
		if status.Latency < bestLatency {
			bestBackend = backend
			bestLatency = status.Latency
		}
	}
	
	return bestBackend, nil
}

// isExcluded checks if a backend name is in the exclusion list
func (s *defaultBackendSelector) isExcluded(name string, exclusions []string) bool {
	for _, excluded := range exclusions {
		if name == excluded {
			return true
		}
	}
	return false
}

// hasRequiredCapabilities checks if a backend has all required capabilities
func (s *defaultBackendSelector) hasRequiredCapabilities(backend Backend, required []string) bool {
	if len(required) == 0 {
		return true
	}
	
	info := backend.GetBackendInfo()
	backendCaps := make(map[string]bool)
	for _, cap := range info.Capabilities {
		backendCaps[cap] = true
	}
	
	for _, req := range required {
		if !backendCaps[req] {
			return false
		}
	}
	
	return true
}
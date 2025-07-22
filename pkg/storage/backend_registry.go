package storage

import (
	"context"
	"sync"
)

// defaultBackendRegistry implements the BackendRegistry interface
type defaultBackendRegistry struct {
	backends map[string]Backend
	mutex    sync.RWMutex
}

// NewBackendRegistry creates a new backend registry
func NewBackendRegistry() BackendRegistry {
	return &defaultBackendRegistry{
		backends: make(map[string]Backend),
	}
}

// GetBackend returns a specific backend by name
func (r *defaultBackendRegistry) GetBackend(name string) (Backend, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	backend, exists := r.backends[name]
	return backend, exists
}

// GetAvailableBackends returns all currently available (connected) backends
func (r *defaultBackendRegistry) GetAvailableBackends() map[string]Backend {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	available := make(map[string]Backend)
	for name, backend := range r.backends {
		if backend.IsConnected() {
			available[name] = backend
		}
	}
	
	return available
}

// GetHealthyBackends returns backends that are currently healthy
func (r *defaultBackendRegistry) GetHealthyBackends() map[string]Backend {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	healthy := make(map[string]Backend)
	for name, backend := range r.backends {
		if backend.IsConnected() {
			status := backend.HealthCheck(context.Background())
			if status.Healthy {
				healthy[name] = backend
			}
		}
	}
	
	return healthy
}

// GetBackendsWithCapability returns backends that support a specific capability
func (r *defaultBackendRegistry) GetBackendsWithCapability(capability string) []Backend {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	var result []Backend
	
	for _, backend := range r.backends {
		if !backend.IsConnected() {
			continue
		}
		
		info := backend.GetBackendInfo()
		for _, cap := range info.Capabilities {
			if cap == capability {
				result = append(result, backend)
				break
			}
		}
	}
	
	return result
}

// AddBackend adds a backend to the registry
func (r *defaultBackendRegistry) AddBackend(name string, backend Backend) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	r.backends[name] = backend
}

// RemoveBackend removes a backend from the registry
func (r *defaultBackendRegistry) RemoveBackend(name string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	delete(r.backends, name)
}

// GetAllBackends returns all backends (including disconnected ones)
func (r *defaultBackendRegistry) GetAllBackends() map[string]Backend {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	// Return a copy to prevent concurrent access issues
	all := make(map[string]Backend)
	for name, backend := range r.backends {
		all[name] = backend
	}
	
	return all
}

// GetBackendNames returns the names of all registered backends
func (r *defaultBackendRegistry) GetBackendNames() []string {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	names := make([]string, 0, len(r.backends))
	for name := range r.backends {
		names = append(names, name)
	}
	
	return names
}
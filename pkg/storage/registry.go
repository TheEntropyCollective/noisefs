package storage

import (
	"fmt"
	"sync"
)

// BackendConstructor is a function that creates a new backend instance
type BackendConstructor func(config *BackendConfig) (Backend, error)

// backendRegistry holds registered backend constructors
var backendRegistry = struct {
	sync.RWMutex
	constructors map[string]BackendConstructor
}{
	constructors: make(map[string]BackendConstructor),
}

// RegisterBackend registers a backend constructor
func RegisterBackend(backendType string, constructor BackendConstructor) {
	backendRegistry.Lock()
	defer backendRegistry.Unlock()

	backendRegistry.constructors[backendType] = constructor
}

// CreateBackend creates a backend instance using the registered constructor
func CreateBackend(config *BackendConfig) (Backend, error) {
	backendRegistry.RLock()
	constructor, exists := backendRegistry.constructors[config.Type]
	backendRegistry.RUnlock()

	if !exists {
		return nil, fmt.Errorf("backend type %s not registered", config.Type)
	}

	return constructor(config)
}

// GetRegisteredBackends returns a list of registered backend types
func GetRegisteredBackends() []string {
	backendRegistry.RLock()
	defer backendRegistry.RUnlock()

	types := make([]string, 0, len(backendRegistry.constructors))
	for backendType := range backendRegistry.constructors {
		types = append(types, backendType)
	}

	return types
}

// BackendFactory creates storage backends based on configuration
type BackendFactory struct {
	config *Config
}

// NewBackendFactory creates a new backend factory
func NewBackendFactory(config *Config) *BackendFactory {
	return &BackendFactory{config: config}
}

// CreateBackend creates a backend instance of the specified type
func (factory *BackendFactory) CreateBackend(backendName string) (Backend, error) {
	backendConfig, exists := factory.config.Backends[backendName]
	if !exists {
		return nil, fmt.Errorf("backend '%s' not found in configuration", backendName)
	}

	if !backendConfig.Enabled {
		return nil, fmt.Errorf("backend '%s' is disabled", backendName)
	}

	// Use the registry to create the backend
	return CreateBackend(backendConfig)
}

// CreateAllBackends creates all enabled backends
func (factory *BackendFactory) CreateAllBackends() (map[string]Backend, error) {
	backends := make(map[string]Backend)

	for name, config := range factory.config.Backends {
		if !config.Enabled {
			continue
		}

		backend, err := CreateBackend(config)
		if err != nil {
			return nil, fmt.Errorf("failed to create backend '%s': %w", name, err)
		}

		backends[name] = backend
	}

	return backends, nil
}

// SelectionCriteria defines criteria for backend selection
type SelectionCriteria struct {
	// Required capabilities
	RequiredCapabilities []string

	// Preferred capabilities (nice to have)
	PreferredCapabilities []string

	// Performance requirements
	MaxLatency    float64 // milliseconds
	MinThroughput float64 // bytes per second
	MaxErrorRate  float64 // percentage (0.0-1.0)

	// Backend type restrictions
	AllowedTypes    []string
	DisallowedTypes []string

	// Health requirements
	RequireHealthy bool

	// Priority weighting
	PreferHighPriority bool

	// Load balancing
	LoadBalance bool
}

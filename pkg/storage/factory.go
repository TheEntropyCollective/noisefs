package storage

import (
	"fmt"
	"sync"
)

// BackendFactory creates backend instances
type BackendFactory struct {
	config *Config
	mutex  sync.RWMutex
}

// NewBackendFactory creates a new backend factory
func NewBackendFactory(config *Config) *BackendFactory {
	return &BackendFactory{
		config: config,
	}
}

// CreateBackend creates a single backend by name
func (f *BackendFactory) CreateBackend(name string) (Backend, error) {
	f.mutex.RLock()
	config, exists := f.config.Backends[name]
	f.mutex.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("backend '%s' not found in configuration", name)
	}
	
	if !config.Enabled {
		return nil, fmt.Errorf("backend '%s' is disabled", name)
	}
	
	return CreateBackend(config)
}

// CreateAllBackends creates all enabled backends
func (f *BackendFactory) CreateAllBackends() (map[string]Backend, error) {
	f.mutex.RLock()
	defer f.mutex.RUnlock()
	
	backends := make(map[string]Backend)
	var errors ErrorAggregator
	
	for name, config := range f.config.Backends {
		if !config.Enabled {
			continue
		}
		
		backend, err := CreateBackend(config)
		if err != nil {
			errors.Add(fmt.Errorf("failed to create backend '%s': %w", name, err))
			continue
		}
		
		backends[name] = backend
	}
	
	if len(backends) == 0 && errors.HasErrors() {
		return nil, errors.CreateAggregateError()
	}
	
	return backends, nil
}

// GetSupportedBackendTypes returns all registered backend types
func (f *BackendFactory) GetSupportedBackendTypes() []string {
	return GetRegisteredBackends()
}

// ValidateConfig validates that all configured backend types are supported
func (f *BackendFactory) ValidateConfig() error {
	supported := GetRegisteredBackends()
	supportedMap := make(map[string]bool)
	for _, backendType := range supported {
		supportedMap[backendType] = true
	}
	
	for name, config := range f.config.Backends {
		if !supportedMap[config.Type] {
			return fmt.Errorf("backend '%s' uses unsupported type '%s'. Supported types: %v", 
				name, config.Type, supported)
		}
	}
	
	return nil
}

// UpdateConfig updates the factory configuration
func (f *BackendFactory) UpdateConfig(newConfig *Config) error {
	if err := newConfig.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}
	
	f.mutex.Lock()
	defer f.mutex.Unlock()
	
	f.config = newConfig
	return nil
}

// CreateManagerFromConfig creates a complete storage manager from configuration
func CreateManagerFromConfig(config *Config) (*Manager, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	
	factory := NewBackendFactory(config)
	if err := factory.ValidateConfig(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}
	
	manager, err := NewManager(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create manager: %w", err)
	}
	
	return manager, nil
}

// CreateManagerWithIPFS creates a storage manager configured for IPFS
func CreateManagerWithIPFS(endpoint string) (*Manager, error) {
	config := &Config{
		DefaultBackend: "ipfs",
		Backends: map[string]*BackendConfig{
			"ipfs": {
				Type:     BackendTypeIPFS,
				Enabled:  true,
				Priority: 100,
				Connection: &ConnectionConfig{
					Endpoint: endpoint,
				},
			},
		},
		Distribution: &DistributionConfig{
			Strategy: "single",
		},
		HealthCheck: &HealthCheckConfig{
			Enabled: true,
		},
	}
	
	return CreateManagerFromConfig(config)
}

// SelectionCriteria defines criteria for backend selection
type SelectionCriteria struct {
	// Required capabilities
	RequiredCapabilities []string
	
	// Preferred capabilities (nice to have)
	PreferredCapabilities []string
	
	// Performance requirements
	MaxLatency   float64 // milliseconds
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

// DefaultSelectionCriteria returns sensible default selection criteria
func DefaultSelectionCriteria() SelectionCriteria {
	return SelectionCriteria{
		RequiredCapabilities: []string{CapabilityContentAddress},
		MaxLatency:          5000, // 5 seconds
		MaxErrorRate:        0.1,  // 10%
		RequireHealthy:      true,
		PreferHighPriority:  true,
		LoadBalance:         false,
	}
}

// ErrorAggregator collects multiple errors
type ErrorAggregator struct {
	errors []error
	mutex  sync.Mutex
}

// Add adds an error to the aggregator
func (ea *ErrorAggregator) Add(err error) {
	if err == nil {
		return
	}
	
	ea.mutex.Lock()
	defer ea.mutex.Unlock()
	
	ea.errors = append(ea.errors, err)
}

// HasErrors returns true if any errors were collected
func (ea *ErrorAggregator) HasErrors() bool {
	ea.mutex.Lock()
	defer ea.mutex.Unlock()
	
	return len(ea.errors) > 0
}

// GetAllErrors returns all collected errors
func (ea *ErrorAggregator) GetAllErrors() []error {
	ea.mutex.Lock()
	defer ea.mutex.Unlock()
	
	// Return a copy to prevent race conditions
	result := make([]error, len(ea.errors))
	copy(result, ea.errors)
	return result
}

// CreateAggregateError creates a single error from all collected errors
func (ea *ErrorAggregator) CreateAggregateError() error {
	ea.mutex.Lock()
	defer ea.mutex.Unlock()
	
	if len(ea.errors) == 0 {
		return nil
	}
	
	if len(ea.errors) == 1 {
		return ea.errors[0]
	}
	
	var message string
	for i, err := range ea.errors {
		if i > 0 {
			message += "; "
		}
		message += err.Error()
	}
	
	return fmt.Errorf("multiple errors: %s", message)
}

// Clear removes all collected errors
func (ea *ErrorAggregator) Clear() {
	ea.mutex.Lock()
	defer ea.mutex.Unlock()
	
	ea.errors = ea.errors[:0]
}
package storage

import (
	"fmt"
)

// Config represents the storage configuration for NoiseFS
type Config struct {
	// Default backend to use
	DefaultBackend string `json:"default_backend" yaml:"default_backend"`

	// Backend configurations
	Backends map[string]*BackendConfig `json:"backends" yaml:"backends"`

	// Distribution strategy
	Distribution *DistributionConfig `json:"distribution" yaml:"distribution"`

	// Health monitoring
	HealthCheck *HealthCheckConfig `json:"health_check" yaml:"health_check"`

	// Performance tuning
	Performance *PerformanceConfig `json:"performance" yaml:"performance"`
}

// BackendConfig represents configuration for a specific storage backend
type BackendConfig struct {
	// Backend type (ipfs, mock)
	Type string `json:"type" yaml:"type"`

	// Enabled status
	Enabled bool `json:"enabled" yaml:"enabled"`

	// Priority for backend selection (higher = preferred)
	Priority int `json:"priority" yaml:"priority"`

	// Connection settings
	Connection *ConnectionConfig `json:"connection" yaml:"connection"`

	// Retry configuration
	Retry *RetryConfig `json:"retry" yaml:"retry"`

	// Timeouts
	Timeouts *TimeoutConfig `json:"timeouts" yaml:"timeouts"`
}

// Validate validates the storage configuration
func (c *Config) Validate() error {
	if c.DefaultBackend == "" {
		return NewConfigError("storage", "default_backend cannot be empty", nil)
	}

	if len(c.Backends) == 0 {
		return NewConfigError("storage", "at least one backend must be configured", nil)
	}

	// Check that default backend exists
	if _, exists := c.Backends[c.DefaultBackend]; !exists {
		return NewConfigError("storage", fmt.Sprintf("default backend '%s' not found in backends configuration", c.DefaultBackend), nil)
	}

	// Check that default backend is enabled
	if defaultBackend := c.Backends[c.DefaultBackend]; !defaultBackend.Enabled {
		return NewConfigError("storage", fmt.Sprintf("default backend '%s' is disabled", c.DefaultBackend), nil)
	}

	// Ensure at least one backend is enabled
	hasEnabledBackend := false
	for _, backend := range c.Backends {
		if backend.Enabled {
			hasEnabledBackend = true
			break
		}
	}
	if !hasEnabledBackend {
		return NewConfigError("storage", "at least one backend must be enabled", nil)
	}

	// Validate distribution configuration
	if c.Distribution != nil {
		if err := c.Distribution.Validate(); err != nil {
			return NewConfigError("storage", "distribution configuration invalid", err)
		}
	}

	// Validate health check configuration
	if c.HealthCheck != nil {
		if err := c.HealthCheck.Validate(); err != nil {
			return NewConfigError("storage", "health check configuration invalid", err)
		}
	}

	// Validate performance configuration
	if c.Performance != nil {
		if err := c.Performance.Validate(); err != nil {
			return NewConfigError("storage", "performance configuration invalid", err)
		}
	}

	// Validate each backend configuration
	for name, backend := range c.Backends {
		if err := backend.Validate(); err != nil {
			return NewConfigError("storage", fmt.Sprintf("backend '%s' configuration invalid", name), err)
		}
	}

	return nil
}

// Validate validates a backend configuration
func (bc *BackendConfig) Validate() error {
	if bc.Type == "" {
		return NewConfigError(bc.Type, "backend type cannot be empty", nil)
	}

	// Validate supported backend types
	validTypes := map[string]bool{
		"ipfs": true, "mock": true,
	}
	if !validTypes[bc.Type] {
		return NewConfigError(bc.Type, fmt.Sprintf("unsupported backend type '%s'", bc.Type), nil)
	}

	if bc.Connection == nil {
		return NewConfigError(bc.Type, "connection configuration is required", nil)
	}

	if err := bc.Connection.Validate(); err != nil {
		return NewConfigError(bc.Type, "connection configuration invalid", err)
	}

	if bc.Priority < 0 {
		return NewConfigError(bc.Type, "priority cannot be negative", nil)
	}

	// Validate retry configuration if present
	if bc.Retry != nil {
		if err := bc.Retry.Validate(); err != nil {
			return NewConfigError(bc.Type, "retry configuration invalid", err)
		}
	}

	// Validate timeout configuration if present
	if bc.Timeouts != nil {
		if err := bc.Timeouts.Validate(); err != nil {
			return NewConfigError(bc.Type, "timeout configuration invalid", err)
		}
	}

	return nil
}

// GetEnabledBackends returns a list of enabled backend names
func (c *Config) GetEnabledBackends() []string {
	var enabled []string
	for name, backend := range c.Backends {
		if backend.Enabled {
			enabled = append(enabled, name)
		}
	}
	return enabled
}

// GetBackendsByPriority returns backends sorted by priority (highest first)
func (c *Config) GetBackendsByPriority() []*BackendConfig {
	var backends []*BackendConfig
	for _, backend := range c.Backends {
		if backend.Enabled {
			backends = append(backends, backend)
		}
	}

	// Sort by priority (highest first)
	for i := 0; i < len(backends)-1; i++ {
		for j := i + 1; j < len(backends); j++ {
			if backends[i].Priority < backends[j].Priority {
				backends[i], backends[j] = backends[j], backends[i]
			}
		}
	}

	return backends
}

// GetBackendConfig returns the configuration for a specific backend
func (c *Config) GetBackendConfig(name string) (*BackendConfig, bool) {
	config, exists := c.Backends[name]
	return config, exists
}

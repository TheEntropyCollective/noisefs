package storage

import (
	"fmt"
	"time"
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
	// Backend type (ipfs, filecoin, arweave, etc.)
	Type string `json:"type" yaml:"type"`

	// Enabled status
	Enabled bool `json:"enabled" yaml:"enabled"`

	// Priority for backend selection (higher = preferred)
	Priority int `json:"priority" yaml:"priority"`

	// Connection settings
	Connection *ConnectionConfig `json:"connection" yaml:"connection"`

	// Backend-specific settings
	Settings map[string]interface{} `json:"settings" yaml:"settings"`

	// Retry configuration
	Retry *RetryConfig `json:"retry" yaml:"retry"`

	// Timeouts
	Timeouts *TimeoutConfig `json:"timeouts" yaml:"timeouts"`
}

// ConnectionConfig represents connection settings for a backend
type ConnectionConfig struct {
	// Endpoint/URL for the backend
	Endpoint string `json:"endpoint" yaml:"endpoint"`

	// Authentication
	Auth *AuthConfig `json:"auth,omitempty" yaml:"auth,omitempty"`

	// Connection pool settings
	MaxConnections int           `json:"max_connections" yaml:"max_connections"`
	IdleTimeout    time.Duration `json:"idle_timeout" yaml:"idle_timeout"`
	ConnectTimeout time.Duration `json:"connect_timeout" yaml:"connect_timeout"`

	// TLS/Security settings
	TLS *TLSConfig `json:"tls,omitempty" yaml:"tls,omitempty"`
}

// AuthConfig represents authentication configuration
type AuthConfig struct {
	Type     string            `json:"type" yaml:"type"` // "none", "basic", "api_key", "oauth"
	Username string            `json:"username,omitempty" yaml:"username,omitempty"`
	Password string            `json:"password,omitempty" yaml:"password,omitempty"`
	APIKey   string            `json:"api_key,omitempty" yaml:"api_key,omitempty"`
	Token    string            `json:"token,omitempty" yaml:"token,omitempty"`
	Headers  map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
}

// TLSConfig represents TLS configuration
type TLSConfig struct {
	Enabled            bool   `json:"enabled" yaml:"enabled"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify" yaml:"insecure_skip_verify"`
	CertFile           string `json:"cert_file,omitempty" yaml:"cert_file,omitempty"`
	KeyFile            string `json:"key_file,omitempty" yaml:"key_file,omitempty"`
	CAFile             string `json:"ca_file,omitempty" yaml:"ca_file,omitempty"`
}

// RetryConfig represents retry configuration
type RetryConfig struct {
	MaxAttempts int           `json:"max_attempts" yaml:"max_attempts"`
	BaseDelay   time.Duration `json:"base_delay" yaml:"base_delay"`
	MaxDelay    time.Duration `json:"max_delay" yaml:"max_delay"`
	Multiplier  float64       `json:"multiplier" yaml:"multiplier"`
	Jitter      bool          `json:"jitter" yaml:"jitter"`
}

// TimeoutConfig represents timeout configuration
type TimeoutConfig struct {
	Connect   time.Duration `json:"connect" yaml:"connect"`
	Read      time.Duration `json:"read" yaml:"read"`
	Write     time.Duration `json:"write" yaml:"write"`
	Operation time.Duration `json:"operation" yaml:"operation"`
}

// DistributionConfig represents block distribution configuration
type DistributionConfig struct {
	// Strategy for distributing blocks across backends
	Strategy string `json:"strategy" yaml:"strategy"` // "single"

	// Backend selection criteria
	Selection *SelectionConfig `json:"selection,omitempty" yaml:"selection,omitempty"`

	// Load balancing
	LoadBalancing *LoadBalancingConfig `json:"load_balancing,omitempty" yaml:"load_balancing,omitempty"`
}

// SelectionConfig represents backend selection criteria
type SelectionConfig struct {
	// Prefer backends with these capabilities
	PreferredCapabilities []string `json:"preferred_capabilities" yaml:"preferred_capabilities"`

	// Required capabilities
	RequiredCapabilities []string `json:"required_capabilities" yaml:"required_capabilities"`

	// Performance criteria
	Performance *PerformanceCriteria `json:"performance,omitempty" yaml:"performance,omitempty"`

	// Cost considerations
	CostWeight float64 `json:"cost_weight" yaml:"cost_weight"`
}

// PerformanceCriteria represents performance-based selection criteria
type PerformanceCriteria struct {
	MaxLatency        time.Duration `json:"max_latency" yaml:"max_latency"`
	MinThroughput     float64       `json:"min_throughput" yaml:"min_throughput"`
	MaxErrorRate      float64       `json:"max_error_rate" yaml:"max_error_rate"`
	LatencyWeight     float64       `json:"latency_weight" yaml:"latency_weight"`
	ReliabilityWeight float64       `json:"reliability_weight" yaml:"reliability_weight"`
}

// LoadBalancingConfig represents load balancing configuration
type LoadBalancingConfig struct {
	Algorithm string `json:"algorithm" yaml:"algorithm"` // "performance"

	// Health check requirements
	RequireHealthy bool `json:"require_healthy" yaml:"require_healthy"`
}

// HealthCheckConfig represents health monitoring configuration
type HealthCheckConfig struct {
	// Enable health checking
	Enabled bool `json:"enabled" yaml:"enabled"`

	// Check interval
	Interval time.Duration `json:"interval" yaml:"interval"`

	// Health check timeout
	Timeout time.Duration `json:"timeout" yaml:"timeout"`
}

// PerformanceConfig represents performance tuning configuration
type PerformanceConfig struct {
	// Concurrency limits
	MaxConcurrentOperations int `json:"max_concurrent_operations" yaml:"max_concurrent_operations"`
	MaxConcurrentPerBackend int `json:"max_concurrent_per_backend" yaml:"max_concurrent_per_backend"`

	// Caching
	Cache *CacheConfig `json:"cache,omitempty" yaml:"cache,omitempty"`

	// Batching
	Batch *BatchConfig `json:"batch,omitempty" yaml:"batch,omitempty"`

	// Compression
	Compression *CompressionConfig `json:"compression,omitempty" yaml:"compression,omitempty"`
}

// CacheConfig represents caching configuration
type CacheConfig struct {
	Enabled   bool          `json:"enabled" yaml:"enabled"`
	MaxSize   int64         `json:"max_size" yaml:"max_size"`
	TTL       time.Duration `json:"ttl" yaml:"ttl"`
	Algorithm string        `json:"algorithm" yaml:"algorithm"` // "lru", "lfu", "arc"
}

// BatchConfig represents batching configuration
type BatchConfig struct {
	Enabled bool          `json:"enabled" yaml:"enabled"`
	MaxSize int           `json:"max_size" yaml:"max_size"`
	MaxWait time.Duration `json:"max_wait" yaml:"max_wait"`
	MinSize int           `json:"min_size" yaml:"min_size"`
}

// CompressionConfig represents compression configuration
type CompressionConfig struct {
	Enabled   bool   `json:"enabled" yaml:"enabled"`
	Algorithm string `json:"algorithm" yaml:"algorithm"` // "gzip", "lz4", "zstd"
	Level     int    `json:"level" yaml:"level"`
}

// DefaultConfig returns a default storage configuration
func DefaultConfig() *Config {
	return &Config{
		DefaultBackend: "ipfs",
		Backends: map[string]*BackendConfig{
			"ipfs": {
				Type:     BackendTypeIPFS,
				Enabled:  true,
				Priority: 100,
				Connection: &ConnectionConfig{
					Endpoint:       "127.0.0.1:5001",
					MaxConnections: 10,
					IdleTimeout:    30 * time.Second,
					ConnectTimeout: 10 * time.Second,
				},
				Settings: map[string]interface{}{
					"api_version": "v0",
				},
				Retry: &RetryConfig{
					MaxAttempts: 3,
					BaseDelay:   100 * time.Millisecond,
					MaxDelay:    5 * time.Second,
					Multiplier:  2.0,
					Jitter:      true,
				},
				Timeouts: &TimeoutConfig{
					Connect:   10 * time.Second,
					Read:      30 * time.Second,
					Write:     30 * time.Second,
					Operation: 60 * time.Second,
				},
			},
		},
		Distribution: &DistributionConfig{
			Strategy: "single",
			Selection: &SelectionConfig{
				RequiredCapabilities: []string{CapabilityContentAddress},
			},
			LoadBalancing: &LoadBalancingConfig{
				Algorithm:      "performance",
				RequireHealthy: true,
			},
		},
		HealthCheck: &HealthCheckConfig{
			Enabled:  true,
			Interval: 30 * time.Second,
			Timeout:  10 * time.Second,
		},
		Performance: &PerformanceConfig{
			MaxConcurrentOperations: 100,
			MaxConcurrentPerBackend: 20,
			Cache: &CacheConfig{
				Enabled:   true,
				MaxSize:   100 * 1024 * 1024, // 100MB
				TTL:       1 * time.Hour,
				Algorithm: "lru",
			},
			Batch: &BatchConfig{
				Enabled: true,
				MaxSize: 50,
				MaxWait: 100 * time.Millisecond,
				MinSize: 5,
			},
			Compression: &CompressionConfig{
				Enabled:   false,
				Algorithm: "gzip",
				Level:     6,
			},
		},
	}
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

// Validate validates connection configuration
func (cc *ConnectionConfig) Validate() error {
	if cc.Endpoint == "" {
		return NewConfigError("connection", "endpoint cannot be empty", nil)
	}

	if cc.MaxConnections < 0 {
		return NewConfigError("connection", "max_connections cannot be negative", nil)
	}
	if cc.MaxConnections == 0 {
		cc.MaxConnections = 10 // Set default
	}

	if cc.IdleTimeout < 0 {
		return NewConfigError("connection", "idle_timeout cannot be negative", nil)
	}

	if cc.ConnectTimeout < 0 {
		return NewConfigError("connection", "connect_timeout cannot be negative", nil)
	}

	// Validate auth configuration if present
	if cc.Auth != nil {
		if err := cc.Auth.Validate(); err != nil {
			return NewConfigError("connection", "auth configuration invalid", err)
		}
	}

	// Validate TLS configuration if present
	if cc.TLS != nil {
		if err := cc.TLS.Validate(); err != nil {
			return NewConfigError("connection", "TLS configuration invalid", err)
		}
	}

	return nil
}

// Validate validates authentication configuration
func (ac *AuthConfig) Validate() error {
	validTypes := map[string]bool{
		"none": true, "basic": true, "api_key": true, "oauth": true, "bearer": true,
	}
	if !validTypes[ac.Type] {
		return NewConfigError("auth", fmt.Sprintf("unsupported auth type '%s'", ac.Type), nil)
	}

	switch ac.Type {
	case "basic":
		if ac.Username == "" {
			return NewConfigError("auth", "username required for basic auth", nil)
		}
		if ac.Password == "" {
			return NewConfigError("auth", "password required for basic auth", nil)
		}
	case "api_key":
		if ac.APIKey == "" {
			return NewConfigError("auth", "api_key required for api_key auth", nil)
		}
	case "oauth", "bearer":
		if ac.Token == "" {
			return NewConfigError("auth", "token required for oauth/bearer auth", nil)
		}
	}

	return nil
}

// Validate validates TLS configuration
func (tc *TLSConfig) Validate() error {
	if !tc.Enabled {
		return nil // Skip validation if TLS is disabled
	}

	// If cert/key files are specified, both must be present
	if tc.CertFile != "" && tc.KeyFile == "" {
		return NewConfigError("tls", "key_file required when cert_file is specified", nil)
	}
	if tc.KeyFile != "" && tc.CertFile == "" {
		return NewConfigError("tls", "cert_file required when key_file is specified", nil)
	}

	return nil
}

// Validate validates retry configuration
func (rc *RetryConfig) Validate() error {
	if rc.MaxAttempts < 0 {
		return NewConfigError("retry", "max_attempts cannot be negative", nil)
	}
	if rc.MaxAttempts == 0 {
		rc.MaxAttempts = 3 // Set default
	}

	if rc.BaseDelay < 0 {
		return NewConfigError("retry", "base_delay cannot be negative", nil)
	}

	if rc.MaxDelay < 0 {
		return NewConfigError("retry", "max_delay cannot be negative", nil)
	}

	if rc.MaxDelay > 0 && rc.BaseDelay > rc.MaxDelay {
		return NewConfigError("retry", "base_delay cannot be greater than max_delay", nil)
	}

	if rc.Multiplier < 1.0 {
		return NewConfigError("retry", "multiplier must be >= 1.0", nil)
	}

	return nil
}

// Validate validates timeout configuration
func (tc *TimeoutConfig) Validate() error {
	if tc.Connect < 0 {
		return NewConfigError("timeout", "connect timeout cannot be negative", nil)
	}

	if tc.Read < 0 {
		return NewConfigError("timeout", "read timeout cannot be negative", nil)
	}

	if tc.Write < 0 {
		return NewConfigError("timeout", "write timeout cannot be negative", nil)
	}

	if tc.Operation < 0 {
		return NewConfigError("timeout", "operation timeout cannot be negative", nil)
	}

	return nil
}

// Validate validates distribution configuration
func (dc *DistributionConfig) Validate() error {
	if dc.Strategy != "single" {
		return NewConfigError("distribution", fmt.Sprintf("unsupported strategy '%s', only 'single' is supported", dc.Strategy), nil)
	}

	// Validate selection config if present
	if dc.Selection != nil {
		if err := dc.Selection.Validate(); err != nil {
			return NewConfigError("distribution", "selection configuration invalid", err)
		}
	}

	// Validate load balancing config if present
	if dc.LoadBalancing != nil {
		if err := dc.LoadBalancing.Validate(); err != nil {
			return NewConfigError("distribution", "load balancing configuration invalid", err)
		}
	}

	return nil
}

// Validate validates selection configuration
func (sc *SelectionConfig) Validate() error {
	if sc.CostWeight < 0 || sc.CostWeight > 1 {
		return NewConfigError("selection", "cost_weight must be between 0 and 1", nil)
	}

	// Validate performance criteria if present
	if sc.Performance != nil {
		if err := sc.Performance.Validate(); err != nil {
			return NewConfigError("selection", "performance criteria invalid", err)
		}
	}

	return nil
}

// Validate validates performance criteria
func (pc *PerformanceCriteria) Validate() error {
	if pc.MaxLatency < 0 {
		return NewConfigError("performance", "max_latency cannot be negative", nil)
	}

	if pc.MinThroughput < 0 {
		return NewConfigError("performance", "min_throughput cannot be negative", nil)
	}

	if pc.MaxErrorRate < 0 || pc.MaxErrorRate > 1 {
		return NewConfigError("performance", "max_error_rate must be between 0 and 1", nil)
	}

	if pc.LatencyWeight < 0 || pc.LatencyWeight > 1 {
		return NewConfigError("performance", "latency_weight must be between 0 and 1", nil)
	}

	if pc.ReliabilityWeight < 0 || pc.ReliabilityWeight > 1 {
		return NewConfigError("performance", "reliability_weight must be between 0 and 1", nil)
	}

	return nil
}

// Validate validates load balancing configuration
func (lbc *LoadBalancingConfig) Validate() error {
	if lbc.Algorithm != "performance" {
		return NewConfigError("load_balancing", fmt.Sprintf("unsupported algorithm '%s', only 'performance' is supported", lbc.Algorithm), nil)
	}

	return nil
}

// Validate validates health check configuration
func (hcc *HealthCheckConfig) Validate() error {
	if !hcc.Enabled {
		return nil // Skip validation if health checks are disabled
	}

	if hcc.Interval <= 0 {
		return NewConfigError("health_check", "interval must be positive", nil)
	}

	if hcc.Timeout <= 0 {
		return NewConfigError("health_check", "timeout must be positive", nil)
	}

	if hcc.Timeout >= hcc.Interval {
		return NewConfigError("health_check", "timeout must be less than interval", nil)
	}

	return nil
}

// Validate validates performance configuration
func (pc *PerformanceConfig) Validate() error {
	if pc.MaxConcurrentOperations < 0 {
		return NewConfigError("performance", "max_concurrent_operations cannot be negative", nil)
	}

	if pc.MaxConcurrentPerBackend < 0 {
		return NewConfigError("performance", "max_concurrent_per_backend cannot be negative", nil)
	}

	// Validate cache config if present - add placeholder methods if needed
	if pc.Cache != nil {
		// Cache validation will depend on cache config structure
	}

	// Validate batch config if present - add placeholder methods if needed
	if pc.Batch != nil {
		// Batch validation will depend on batch config structure
	}

	// Validate compression config if present - add placeholder methods if needed
	if pc.Compression != nil {
		// Compression validation will depend on compression config structure
	}

	return nil
}

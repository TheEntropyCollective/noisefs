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
	Strategy string `json:"strategy" yaml:"strategy"` // "single", "replicate", "stripe", "smart"

	// Replication settings
	Replication *ReplicationConfig `json:"replication,omitempty" yaml:"replication,omitempty"`

	// Backend selection criteria
	Selection *SelectionConfig `json:"selection,omitempty" yaml:"selection,omitempty"`

	// Load balancing
	LoadBalancing *LoadBalancingConfig `json:"load_balancing,omitempty" yaml:"load_balancing,omitempty"`
}

// ReplicationConfig represents replication settings
type ReplicationConfig struct {
	// Minimum number of replicas
	MinReplicas int `json:"min_replicas" yaml:"min_replicas"`

	// Maximum number of replicas
	MaxReplicas int `json:"max_replicas" yaml:"max_replicas"`

	// Backend diversity requirements
	RequireDiverseBackends bool `json:"require_diverse_backends" yaml:"require_diverse_backends"`

	// Geographic diversity
	RequireGeoDiversity bool `json:"require_geo_diversity" yaml:"require_geo_diversity"`
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
	Algorithm string `json:"algorithm" yaml:"algorithm"` // "round_robin", "weighted", "least_connections", "performance"

	// Health check requirements
	RequireHealthy bool `json:"require_healthy" yaml:"require_healthy"`

	// Sticky sessions
	StickyBlocks bool `json:"sticky_blocks" yaml:"sticky_blocks"`
}

// HealthCheckConfig represents health monitoring configuration
type HealthCheckConfig struct {
	// Enable health checking
	Enabled bool `json:"enabled" yaml:"enabled"`

	// Check interval
	Interval time.Duration `json:"interval" yaml:"interval"`

	// Health check timeout
	Timeout time.Duration `json:"timeout" yaml:"timeout"`

	// Thresholds for marking backends unhealthy
	Thresholds *HealthThresholds `json:"thresholds" yaml:"thresholds"`

	// Actions to take when backends become unhealthy
	Actions *HealthActions `json:"actions" yaml:"actions"`
}

// HealthThresholds represents health check thresholds
type HealthThresholds struct {
	MaxLatency          time.Duration `json:"max_latency" yaml:"max_latency"`
	MaxErrorRate        float64       `json:"max_error_rate" yaml:"max_error_rate"`
	MinSuccessRate      float64       `json:"min_success_rate" yaml:"min_success_rate"`
	ConsecutiveFailures int           `json:"consecutive_failures" yaml:"consecutive_failures"`
}

// HealthActions represents actions to take on health events
type HealthActions struct {
	OnUnhealthy string `json:"on_unhealthy" yaml:"on_unhealthy"` // "disable", "deprioritize", "quarantine"
	OnRecovered string `json:"on_recovered" yaml:"on_recovered"` // "enable", "restore_priority"

	// Notification settings
	NotifyOnStateChange bool `json:"notify_on_state_change" yaml:"notify_on_state_change"`
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
				Performance: &PerformanceCriteria{
					MaxLatency:        5 * time.Second,
					MaxErrorRate:      0.1,
					LatencyWeight:     0.6,
					ReliabilityWeight: 0.4,
				},
			},
			LoadBalancing: &LoadBalancingConfig{
				Algorithm:      "performance",
				RequireHealthy: true,
				StickyBlocks:   false,
			},
		},
		HealthCheck: &HealthCheckConfig{
			Enabled:  true,
			Interval: 30 * time.Second,
			Timeout:  10 * time.Second,
			Thresholds: &HealthThresholds{
				MaxLatency:          10 * time.Second,
				MaxErrorRate:        0.2,
				MinSuccessRate:      0.8,
				ConsecutiveFailures: 3,
			},
			Actions: &HealthActions{
				OnUnhealthy:         "deprioritize",
				OnRecovered:         "restore_priority",
				NotifyOnStateChange: true,
			},
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
		return fmt.Errorf("default_backend cannot be empty")
	}

	if len(c.Backends) == 0 {
		return fmt.Errorf("at least one backend must be configured")
	}

	// Check that default backend exists
	if _, exists := c.Backends[c.DefaultBackend]; !exists {
		return fmt.Errorf("default backend '%s' not found in backends configuration", c.DefaultBackend)
	}

	// Validate each backend configuration
	for name, backend := range c.Backends {
		if err := backend.Validate(); err != nil {
			return fmt.Errorf("backend '%s' configuration invalid: %w", name, err)
		}
	}

	return nil
}

// Validate validates a backend configuration
func (bc *BackendConfig) Validate() error {
	if bc.Type == "" {
		return fmt.Errorf("backend type cannot be empty")
	}

	if bc.Connection == nil {
		return fmt.Errorf("connection configuration is required")
	}

	if bc.Connection.Endpoint == "" {
		return fmt.Errorf("connection endpoint cannot be empty")
	}

	if bc.Priority < 0 {
		return fmt.Errorf("priority cannot be negative")
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

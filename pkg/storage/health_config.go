package storage

import (
	"fmt"
	"time"
)

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
}

// SelectionConfig represents backend selection criteria
type SelectionConfig struct {
	// Required capabilities
	RequiredCapabilities []string `json:"required_capabilities" yaml:"required_capabilities"`

	// Performance criteria
	Performance *PerformanceCriteria `json:"performance,omitempty" yaml:"performance,omitempty"`
}

// PerformanceCriteria represents performance-based selection criteria
type PerformanceCriteria struct {
	MaxLatency   time.Duration `json:"max_latency" yaml:"max_latency"`
	MaxErrorRate float64       `json:"max_error_rate" yaml:"max_error_rate"`
}

// LoadBalancingConfig represents load balancing configuration
type LoadBalancingConfig struct {
	Algorithm string `json:"algorithm" yaml:"algorithm"` // "round_robin", "weighted", "least_connections", "performance"

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
}

// Validate validates distribution configuration
func (dc *DistributionConfig) Validate() error {
	validStrategies := map[string]bool{
		"single": true, "replicate": true, "stripe": true, "smart": true,
	}
	if !validStrategies[dc.Strategy] {
		return NewConfigError("distribution", fmt.Sprintf("unsupported strategy '%s'", dc.Strategy), nil)
	}

	// Validate replication config if present
	if dc.Replication != nil {
		if err := dc.Replication.Validate(); err != nil {
			return NewConfigError("distribution", "replication configuration invalid", err)
		}
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

// Validate validates replication configuration
func (rc *ReplicationConfig) Validate() error {
	if rc.MinReplicas < 1 {
		return NewConfigError("replication", "min_replicas must be at least 1", nil)
	}

	if rc.MaxReplicas < rc.MinReplicas {
		return NewConfigError("replication", "max_replicas cannot be less than min_replicas", nil)
	}

	return nil
}

// Validate validates selection configuration
func (sc *SelectionConfig) Validate() error {
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

	if pc.MaxErrorRate < 0 || pc.MaxErrorRate > 1 {
		return NewConfigError("performance", "max_error_rate must be between 0 and 1", nil)
	}

	return nil
}

// Validate validates load balancing configuration
func (lbc *LoadBalancingConfig) Validate() error {
	validAlgorithms := map[string]bool{
		"round_robin": true, "weighted": true, "least_connections": true, "performance": true,
	}
	if !validAlgorithms[lbc.Algorithm] {
		return NewConfigError("load_balancing", fmt.Sprintf("unsupported algorithm '%s'", lbc.Algorithm), nil)
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

	// Validate thresholds if present
	if hcc.Thresholds != nil {
		if err := hcc.Thresholds.Validate(); err != nil {
			return NewConfigError("health_check", "thresholds configuration invalid", err)
		}
	}

	// Validate actions if present
	if hcc.Actions != nil {
		if err := hcc.Actions.Validate(); err != nil {
			return NewConfigError("health_check", "actions configuration invalid", err)
		}
	}

	return nil
}

// Validate validates health thresholds
func (ht *HealthThresholds) Validate() error {
	if ht.MaxLatency < 0 {
		return NewConfigError("health_thresholds", "max_latency cannot be negative", nil)
	}

	if ht.MaxErrorRate < 0 || ht.MaxErrorRate > 1 {
		return NewConfigError("health_thresholds", "max_error_rate must be between 0 and 1", nil)
	}

	if ht.MinSuccessRate < 0 || ht.MinSuccessRate > 1 {
		return NewConfigError("health_thresholds", "min_success_rate must be between 0 and 1", nil)
	}

	if ht.ConsecutiveFailures < 0 {
		return NewConfigError("health_thresholds", "consecutive_failures cannot be negative", nil)
	}

	return nil
}

// Validate validates health actions
func (ha *HealthActions) Validate() error {
	validUnhealthyActions := map[string]bool{
		"disable": true, "deprioritize": true, "quarantine": true,
	}
	if ha.OnUnhealthy != "" && !validUnhealthyActions[ha.OnUnhealthy] {
		return NewConfigError("health_actions", fmt.Sprintf("unsupported on_unhealthy action '%s'", ha.OnUnhealthy), nil)
	}

	validRecoveredActions := map[string]bool{
		"enable": true, "restore_priority": true,
	}
	if ha.OnRecovered != "" && !validRecoveredActions[ha.OnRecovered] {
		return NewConfigError("health_actions", fmt.Sprintf("unsupported on_recovered action '%s'", ha.OnRecovered), nil)
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

	return nil
}

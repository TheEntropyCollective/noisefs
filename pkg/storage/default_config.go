package storage

import (
	"time"
)

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
					ConnectTimeout: 10 * time.Second,
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
					MaxLatency:   5 * time.Second,
					MaxErrorRate: 0.1,
				},
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
		},
	}
}

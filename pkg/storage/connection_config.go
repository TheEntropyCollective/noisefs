package storage

import (
	"time"
)

// ConnectionConfig represents connection settings for a backend
type ConnectionConfig struct {
	// Endpoint/URL for the backend
	Endpoint string `json:"endpoint" yaml:"endpoint"`

	// Connect timeout
	ConnectTimeout time.Duration `json:"connect_timeout" yaml:"connect_timeout"`
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

// Validate validates connection configuration
func (cc *ConnectionConfig) Validate() error {
	if cc.Endpoint == "" {
		return NewConfigError("connection", "endpoint cannot be empty", nil)
	}

	if cc.ConnectTimeout < 0 {
		return NewConfigError("connection", "connect_timeout cannot be negative", nil)
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

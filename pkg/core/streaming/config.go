// Package streaming provides configuration interfaces and builder patterns for streaming operations.
// This file implements type-safe configuration with validation, defaults, and builder patterns
// for creating and managing streaming configurations.
package streaming

import (
	"fmt"
	"runtime"
	"time"
)

// ConfigBuilder provides a fluent interface for building streaming configurations.
// Enables type-safe configuration construction with validation and defaults.
//
// Example usage:
//   config := NewConfigBuilder().
//       WithBlockSize(256 * 1024).
//       WithMaxConcurrency(8).
//       WithTimeout(30 * time.Minute).
//       WithProgressReporter(myReporter).
//       Build()
type ConfigBuilder interface {
	// WithBlockSize sets the block size for file splitting operations.
	// Block size affects memory usage, network efficiency, and cache performance.
	//
	// Parameters:
	//   - size: Block size in bytes (must be positive)
	//
	// Returns:
	//   - ConfigBuilder: Builder instance for method chaining
	//
	// Validation:
	//   - Size must be positive
	//   - Size should be a power of 2 for optimal performance
	//   - Recommended range: 64 KiB to 1 MiB
	WithBlockSize(size int) ConfigBuilder

	// WithMaxConcurrency sets the maximum number of concurrent operations.
	// Controls resource usage and prevents overwhelming storage backends.
	//
	// Parameters:
	//   - concurrency: Maximum concurrent operations (must be positive)
	//
	// Returns:
	//   - ConfigBuilder: Builder instance for method chaining
	//
	// Validation:
	//   - Concurrency must be positive
	//   - Recommended maximum: 2-4x number of CPU cores
	WithMaxConcurrency(concurrency int) ConfigBuilder

	// WithProgressReporter sets the progress reporter for operation monitoring.
	// Enables real-time progress updates and user interface integration.
	//
	// Parameters:
	//   - reporter: Progress reporter implementation (may be nil)
	//
	// Returns:
	//   - ConfigBuilder: Builder instance for method chaining
	WithProgressReporter(reporter ProgressReporter) ConfigBuilder

	// WithTimeout sets the maximum duration for streaming operations.
	// Operations exceeding this duration will be cancelled automatically.
	//
	// Parameters:
	//   - timeout: Maximum operation duration (0 = no timeout)
	//
	// Returns:
	//   - ConfigBuilder: Builder instance for method chaining
	//
	// Validation:
	//   - Timeout must be non-negative
	//   - Recommended minimum: 30 seconds for small files
	WithTimeout(timeout time.Duration) ConfigBuilder

	// WithBufferSize sets the internal buffer size for streaming operations.
	// Affects memory usage and I/O efficiency during data processing.
	//
	// Parameters:
	//   - size: Buffer size in bytes (must be positive)
	//
	// Returns:
	//   - ConfigBuilder: Builder instance for method chaining
	//
	// Validation:
	//   - Size must be positive
	//   - Recommended range: 16 KiB to 256 KiB
	WithBufferSize(size int) ConfigBuilder

	// WithEncryption enables descriptor encryption with the specified password.
	// When enabled, file descriptors are encrypted before storage.
	//
	// Parameters:
	//   - enabled: Whether to enable encryption
	//   - password: Encryption password (required if enabled is true)
	//
	// Returns:
	//   - ConfigBuilder: Builder instance for method chaining
	//
	// Validation:
	//   - Password must be non-empty if encryption is enabled
	//   - Password should meet minimum security requirements
	WithEncryption(enabled bool, password string) ConfigBuilder

	// WithRetryPolicy sets the retry policy for failed operations.
	// Configures retry attempts, backoff strategy, and retry conditions.
	//
	// Parameters:
	//   - policy: Retry policy configuration (may be nil to disable retries)
	//
	// Returns:
	//   - ConfigBuilder: Builder instance for method chaining
	WithRetryPolicy(policy *RetryPolicy) ConfigBuilder

	// WithValidationLevel sets the data validation level for operations.
	// Higher levels provide more integrity checking at the cost of performance.
	//
	// Parameters:
	//   - level: Validation level (none, basic, standard, strict)
	//
	// Returns:
	//   - ConfigBuilder: Builder instance for method chaining
	WithValidationLevel(level ValidationLevel) ConfigBuilder

	// WithTags sets metadata tags for the streaming operation.
	// Used for categorization, search, and operational tracking.
	//
	// Parameters:
	//   - tags: Key-value pairs for operation metadata
	//
	// Returns:
	//   - ConfigBuilder: Builder instance for method chaining
	WithTags(tags map[string]string) ConfigBuilder

	// Build creates an immutable streaming configuration from the builder state.
	// Performs comprehensive validation and returns a validated configuration.
	//
	// Returns:
	//   - StreamingConfig: Immutable configuration instance
	//   - error: Validation error if configuration is invalid
	Build() (StreamingConfig, error)

	// BuildWithDefaults creates a configuration with default values for unset parameters.
	// Automatically applies sensible defaults for any unspecified configuration.
	//
	// Returns:
	//   - StreamingConfig: Configuration with defaults applied
	//   - error: Validation error if any explicit values are invalid
	BuildWithDefaults() (StreamingConfig, error)

	// Reset clears all configuration values and returns to initial state.
	// Enables reuse of the builder for creating multiple configurations.
	//
	// Returns:
	//   - ConfigBuilder: Reset builder instance for method chaining
	Reset() ConfigBuilder

	// Clone creates a copy of the current builder state.
	// Enables creating variations of a base configuration.
	//
	// Returns:
	//   - ConfigBuilder: Independent copy of the builder
	Clone() ConfigBuilder
}

// streamingConfig implements the StreamingConfig interface with immutable state.
// Provides thread-safe access to validated configuration parameters.
type streamingConfig struct {
	blockSize        int
	maxConcurrency   int
	progressReporter ProgressReporter
	timeout          time.Duration
	bufferSize       int
	encryptionEnabled bool
	encryptionPassword string
	retryPolicy      *RetryPolicy
	validationLevel  ValidationLevel
	tags             map[string]string
}

// GetBlockSize returns the configured block size in bytes.
func (c *streamingConfig) GetBlockSize() int {
	return c.blockSize
}

// GetMaxConcurrency returns the maximum number of concurrent operations.
func (c *streamingConfig) GetMaxConcurrency() int {
	return c.maxConcurrency
}

// GetProgressReporter returns the configured progress reporter.
func (c *streamingConfig) GetProgressReporter() ProgressReporter {
	return c.progressReporter
}

// GetTimeout returns the operation timeout duration.
func (c *streamingConfig) GetTimeout() time.Duration {
	return c.timeout
}

// GetBufferSize returns the internal buffer size for streaming operations.
func (c *streamingConfig) GetBufferSize() int {
	return c.bufferSize
}

// IsEncryptionEnabled returns whether descriptor encryption is enabled.
func (c *streamingConfig) IsEncryptionEnabled() bool {
	return c.encryptionEnabled
}

// GetEncryptionPassword returns the encryption password.
// Note: This method should be used carefully to avoid password exposure.
func (c *streamingConfig) GetEncryptionPassword() string {
	return c.encryptionPassword
}

// GetRetryPolicy returns the configured retry policy.
func (c *streamingConfig) GetRetryPolicy() *RetryPolicy {
	return c.retryPolicy
}

// GetValidationLevel returns the configured validation level.
func (c *streamingConfig) GetValidationLevel() ValidationLevel {
	return c.validationLevel
}

// GetTags returns the configured metadata tags.
func (c *streamingConfig) GetTags() map[string]string {
	// Return a copy to maintain immutability
	if c.tags == nil {
		return nil
	}
	tags := make(map[string]string, len(c.tags))
	for k, v := range c.tags {
		tags[k] = v
	}
	return tags
}

// Validate performs comprehensive configuration validation.
func (c *streamingConfig) Validate() error {
	if c.blockSize <= 0 {
		return fmt.Errorf("%w: block size must be positive, got %d", ErrInvalidOptions, c.blockSize)
	}

	if c.maxConcurrency <= 0 {
		return fmt.Errorf("%w: max concurrency must be positive, got %d", ErrInvalidOptions, c.maxConcurrency)
	}

	if c.timeout < 0 {
		return fmt.Errorf("%w: timeout must be non-negative, got %v", ErrInvalidOptions, c.timeout)
	}

	if c.bufferSize <= 0 {
		return fmt.Errorf("%w: buffer size must be positive, got %d", ErrInvalidOptions, c.bufferSize)
	}

	if c.encryptionEnabled && c.encryptionPassword == "" {
		return fmt.Errorf("%w: encryption password required when encryption is enabled", ErrInvalidOptions)
	}

	if c.retryPolicy != nil {
		if err := validateRetryPolicy(c.retryPolicy); err != nil {
			return fmt.Errorf("%w: invalid retry policy: %v", ErrInvalidOptions, err)
		}
	}

	return nil
}

// Clone creates a deep copy of the configuration.
func (c *streamingConfig) Clone() StreamingConfig {
	clone := &streamingConfig{
		blockSize:          c.blockSize,
		maxConcurrency:     c.maxConcurrency,
		progressReporter:   c.progressReporter,
		timeout:            c.timeout,
		bufferSize:         c.bufferSize,
		encryptionEnabled:  c.encryptionEnabled,
		encryptionPassword: c.encryptionPassword,
		retryPolicy:        c.retryPolicy,
		validationLevel:    c.validationLevel,
	}

	// Deep copy tags map
	if c.tags != nil {
		clone.tags = make(map[string]string, len(c.tags))
		for k, v := range c.tags {
			clone.tags[k] = v
		}
	}

	// Deep copy retry policy if present
	if c.retryPolicy != nil {
		retryPolicyCopy := *c.retryPolicy
		if c.retryPolicy.RetryableErrors != nil {
			retryPolicyCopy.RetryableErrors = make([]error, len(c.retryPolicy.RetryableErrors))
			copy(retryPolicyCopy.RetryableErrors, c.retryPolicy.RetryableErrors)
		}
		clone.retryPolicy = &retryPolicyCopy
	}

	return clone
}

// configBuilder implements the ConfigBuilder interface.
type configBuilder struct {
	config streamingConfig
}

// NewConfigBuilder creates a new configuration builder with empty state.
func NewConfigBuilder() ConfigBuilder {
	return &configBuilder{}
}

// WithBlockSize sets the block size for file splitting operations.
func (b *configBuilder) WithBlockSize(size int) ConfigBuilder {
	b.config.blockSize = size
	return b
}

// WithMaxConcurrency sets the maximum number of concurrent operations.
func (b *configBuilder) WithMaxConcurrency(concurrency int) ConfigBuilder {
	b.config.maxConcurrency = concurrency
	return b
}

// WithProgressReporter sets the progress reporter for operation monitoring.
func (b *configBuilder) WithProgressReporter(reporter ProgressReporter) ConfigBuilder {
	b.config.progressReporter = reporter
	return b
}

// WithTimeout sets the maximum duration for streaming operations.
func (b *configBuilder) WithTimeout(timeout time.Duration) ConfigBuilder {
	b.config.timeout = timeout
	return b
}

// WithBufferSize sets the internal buffer size for streaming operations.
func (b *configBuilder) WithBufferSize(size int) ConfigBuilder {
	b.config.bufferSize = size
	return b
}

// WithEncryption enables descriptor encryption with the specified password.
func (b *configBuilder) WithEncryption(enabled bool, password string) ConfigBuilder {
	b.config.encryptionEnabled = enabled
	b.config.encryptionPassword = password
	return b
}

// WithRetryPolicy sets the retry policy for failed operations.
func (b *configBuilder) WithRetryPolicy(policy *RetryPolicy) ConfigBuilder {
	b.config.retryPolicy = policy
	return b
}

// WithValidationLevel sets the data validation level for operations.
func (b *configBuilder) WithValidationLevel(level ValidationLevel) ConfigBuilder {
	b.config.validationLevel = level
	return b
}

// WithTags sets metadata tags for the streaming operation.
func (b *configBuilder) WithTags(tags map[string]string) ConfigBuilder {
	if tags == nil {
		b.config.tags = nil
	} else {
		b.config.tags = make(map[string]string, len(tags))
		for k, v := range tags {
			b.config.tags[k] = v
		}
	}
	return b
}

// Build creates an immutable streaming configuration from the builder state.
func (b *configBuilder) Build() (StreamingConfig, error) {
	config := b.config.Clone().(*streamingConfig)
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return config, nil
}

// BuildWithDefaults creates a configuration with default values for unset parameters.
func (b *configBuilder) BuildWithDefaults() (StreamingConfig, error) {
	// Apply defaults for unset values
	config := b.config

	if config.blockSize <= 0 {
		config.blockSize = DefaultBlockSize
	}

	if config.maxConcurrency <= 0 {
		config.maxConcurrency = runtime.NumCPU()
	}

	if config.bufferSize <= 0 {
		config.bufferSize = DefaultBufferSize
	}

	if config.validationLevel < ValidationNone || config.validationLevel > ValidationStrict {
		config.validationLevel = ValidationStandard
	}

	// Create immutable config and validate
	finalConfig := config.Clone().(*streamingConfig)
	if err := finalConfig.Validate(); err != nil {
		return nil, err
	}

	return finalConfig, nil
}

// Reset clears all configuration values and returns to initial state.
func (b *configBuilder) Reset() ConfigBuilder {
	b.config = streamingConfig{}
	return b
}

// Clone creates a copy of the current builder state.
func (b *configBuilder) Clone() ConfigBuilder {
	clone := &configBuilder{
		config: streamingConfig{
			blockSize:          b.config.blockSize,
			maxConcurrency:     b.config.maxConcurrency,
			progressReporter:   b.config.progressReporter,
			timeout:            b.config.timeout,
			bufferSize:         b.config.bufferSize,
			encryptionEnabled:  b.config.encryptionEnabled,
			encryptionPassword: b.config.encryptionPassword,
			retryPolicy:        b.config.retryPolicy,
			validationLevel:    b.config.validationLevel,
		},
	}

	// Deep copy tags
	if b.config.tags != nil {
		clone.config.tags = make(map[string]string, len(b.config.tags))
		for k, v := range b.config.tags {
			clone.config.tags[k] = v
		}
	}

	return clone
}

// Default configuration values
const (
	// DefaultBlockSize is the recommended block size for most use cases (128 KiB).
	DefaultBlockSize = 128 * 1024

	// DefaultBufferSize is the recommended buffer size for streaming operations (64 KiB).
	DefaultBufferSize = 64 * 1024

	// DefaultTimeout is the recommended timeout for streaming operations (no timeout).
	DefaultTimeout = 0

	// MinBlockSize is the minimum allowed block size (4 KiB).
	MinBlockSize = 4 * 1024

	// MaxBlockSize is the maximum allowed block size (16 MiB).
	MaxBlockSize = 16 * 1024 * 1024

	// MinBufferSize is the minimum allowed buffer size (4 KiB).
	MinBufferSize = 4 * 1024

	// MaxBufferSize is the maximum allowed buffer size (1 MiB).
	MaxBufferSize = 1024 * 1024
)

// validateRetryPolicy validates retry policy configuration.
func validateRetryPolicy(policy *RetryPolicy) error {
	if policy.MaxAttempts < 0 {
		return fmt.Errorf("max attempts must be non-negative, got %d", policy.MaxAttempts)
	}

	if policy.InitialDelay < 0 {
		return fmt.Errorf("initial delay must be non-negative, got %v", policy.InitialDelay)
	}

	if policy.MaxDelay < 0 {
		return fmt.Errorf("max delay must be non-negative, got %v", policy.MaxDelay)
	}

	if policy.MaxDelay > 0 && policy.InitialDelay > policy.MaxDelay {
		return fmt.Errorf("initial delay (%v) cannot exceed max delay (%v)", policy.InitialDelay, policy.MaxDelay)
	}

	if policy.BackoffMultiplier < 1.0 {
		return fmt.Errorf("backoff multiplier must be >= 1.0, got %f", policy.BackoffMultiplier)
	}

	return nil
}

// DefaultConfig creates a streaming configuration with recommended default values.
func DefaultConfig() StreamingConfig {
	config, _ := NewConfigBuilder().BuildWithDefaults()
	return config
}
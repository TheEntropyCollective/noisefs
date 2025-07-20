package resilience

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// CircuitBreakerState represents the current state of the circuit breaker
type CircuitBreakerState int

const (
	// StateClosed - circuit breaker allows requests through
	StateClosed CircuitBreakerState = iota
	// StateOpen - circuit breaker blocks requests, failing fast
	StateOpen
	// StateHalfOpen - circuit breaker allows limited requests to test recovery
	StateHalfOpen
)

// String returns the string representation of CircuitBreakerState
func (s CircuitBreakerState) String() string {
	switch s {
	case StateClosed:
		return "Closed"
	case StateOpen:
		return "Open"
	case StateHalfOpen:
		return "HalfOpen"
	default:
		return "Unknown"
	}
}

// CircuitBreakerConfig holds configuration for the circuit breaker
type CircuitBreakerConfig struct {
	// FailureThreshold is the number of failures that triggers the circuit to open
	FailureThreshold int64
	// RecoveryTimeout is how long to wait before transitioning from Open to HalfOpen
	RecoveryTimeout time.Duration
	// SuccessThreshold is the number of successes needed in HalfOpen to close the circuit
	SuccessThreshold int64
	// MaxRequests is the maximum number of requests allowed in HalfOpen state
	MaxRequests int64
	// Timeout is the timeout for individual requests
	Timeout time.Duration
	// Name is a human-readable name for this circuit breaker
	Name string
}

// DefaultCircuitBreakerConfig returns a sensible default configuration
func DefaultCircuitBreakerConfig(name string) *CircuitBreakerConfig {
	return &CircuitBreakerConfig{
		FailureThreshold: 5,
		RecoveryTimeout:  30 * time.Second,
		SuccessThreshold: 3,
		MaxRequests:      10,
		Timeout:          10 * time.Second,
		Name:             name,
	}
}

// CircuitBreakerStats holds statistics about circuit breaker operation
type CircuitBreakerStats struct {
	State            CircuitBreakerState `json:"state"`
	Failures         int64               `json:"failures"`
	Successes        int64               `json:"successes"`
	Requests         int64               `json:"requests"`
	LastFailureTime  time.Time           `json:"last_failure_time"`
	LastSuccessTime  time.Time           `json:"last_success_time"`
	StateChangedTime time.Time           `json:"state_changed_time"`
	TotalRequests    int64               `json:"total_requests"`
	TotalFailures    int64               `json:"total_failures"`
	TotalSuccesses   int64               `json:"total_successes"`
}

// CircuitBreaker implements the circuit breaker pattern for resilience
type CircuitBreaker struct {
	config *CircuitBreakerConfig
	state  CircuitBreakerState
	mu     sync.RWMutex

	// Counters (using atomic operations)
	failures        int64
	successes       int64
	requests        int64
	totalRequests   int64
	totalFailures   int64
	totalSuccesses  int64

	// Timestamps
	lastFailureTime  time.Time
	lastSuccessTime  time.Time
	stateChangedTime time.Time

	// Callbacks
	onStateChange func(from, to CircuitBreakerState)
}

// NewCircuitBreaker creates a new circuit breaker with the given configuration
func NewCircuitBreaker(config *CircuitBreakerConfig) *CircuitBreaker {
	if config == nil {
		config = DefaultCircuitBreakerConfig("default")
	}

	return &CircuitBreaker{
		config:           config,
		state:            StateClosed,
		stateChangedTime: time.Now(),
	}
}

// Execute runs the given function with circuit breaker protection
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func(context.Context) error) error {
	// Check if request is allowed
	if !cb.allowRequest() {
		return cb.createCircuitOpenError()
	}

	atomic.AddInt64(&cb.requests, 1)
	atomic.AddInt64(&cb.totalRequests, 1)

	// Create context with timeout
	if cb.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cb.config.Timeout)
		defer cancel()
	}

	// Execute the function
	err := fn(ctx)

	// Record result
	if err != nil {
		cb.recordFailure(err)
		return err
	}

	cb.recordSuccess()
	return nil
}

// allowRequest determines if a request should be allowed through
func (cb *CircuitBreaker) allowRequest() bool {
	cb.mu.RLock()
	state := cb.state
	cb.mu.RUnlock()

	switch state {
	case StateClosed:
		return true
	case StateOpen:
		// Check if it's time to transition to half-open
		cb.mu.Lock()
		if time.Since(cb.stateChangedTime) >= cb.config.RecoveryTimeout {
			cb.setState(StateHalfOpen)
			cb.mu.Unlock()
			return true
		}
		cb.mu.Unlock()
		return false
	case StateHalfOpen:
		// Allow limited requests in half-open state
		return atomic.LoadInt64(&cb.requests) < cb.config.MaxRequests
	default:
		return false
	}
}

// recordSuccess records a successful request
func (cb *CircuitBreaker) recordSuccess() {
	atomic.AddInt64(&cb.successes, 1)
	atomic.AddInt64(&cb.totalSuccesses, 1)

	cb.mu.Lock()
	cb.lastSuccessTime = time.Now()

	switch cb.state {
	case StateHalfOpen:
		// Check if we have enough successes to close the circuit
		if atomic.LoadInt64(&cb.successes) >= cb.config.SuccessThreshold {
			cb.setState(StateClosed)
		}
	}
	cb.mu.Unlock()
}

// recordFailure records a failed request
func (cb *CircuitBreaker) recordFailure(err error) {
	atomic.AddInt64(&cb.failures, 1)
	atomic.AddInt64(&cb.totalFailures, 1)

	cb.mu.Lock()
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case StateClosed:
		// Check if we should open the circuit
		if atomic.LoadInt64(&cb.failures) >= cb.config.FailureThreshold {
			cb.setState(StateOpen)
		}
	case StateHalfOpen:
		// Any failure in half-open state opens the circuit immediately
		cb.setState(StateOpen)
	}
	cb.mu.Unlock()
}

// setState changes the circuit breaker state and resets counters
func (cb *CircuitBreaker) setState(newState CircuitBreakerState) {
	oldState := cb.state
	cb.state = newState
	cb.stateChangedTime = time.Now()

	// Reset counters based on state transition
	switch newState {
	case StateClosed:
		atomic.StoreInt64(&cb.failures, 0)
		atomic.StoreInt64(&cb.successes, 0)
		atomic.StoreInt64(&cb.requests, 0)
	case StateOpen:
		atomic.StoreInt64(&cb.failures, 0)
		atomic.StoreInt64(&cb.successes, 0)
		atomic.StoreInt64(&cb.requests, 0)
	case StateHalfOpen:
		atomic.StoreInt64(&cb.failures, 0)
		atomic.StoreInt64(&cb.successes, 0)
		atomic.StoreInt64(&cb.requests, 0)
	}

	// Call state change callback if set
	if cb.onStateChange != nil {
		go cb.onStateChange(oldState, newState)
	}
}

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() CircuitBreakerState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetStats returns current statistics about the circuit breaker
func (cb *CircuitBreaker) GetStats() CircuitBreakerStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return CircuitBreakerStats{
		State:            cb.state,
		Failures:         atomic.LoadInt64(&cb.failures),
		Successes:        atomic.LoadInt64(&cb.successes),
		Requests:         atomic.LoadInt64(&cb.requests),
		LastFailureTime:  cb.lastFailureTime,
		LastSuccessTime:  cb.lastSuccessTime,
		StateChangedTime: cb.stateChangedTime,
		TotalRequests:    atomic.LoadInt64(&cb.totalRequests),
		TotalFailures:    atomic.LoadInt64(&cb.totalFailures),
		TotalSuccesses:   atomic.LoadInt64(&cb.totalSuccesses),
	}
}

// SetStateChangeCallback sets a callback function to be called when state changes
func (cb *CircuitBreaker) SetStateChangeCallback(callback func(from, to CircuitBreakerState)) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.onStateChange = callback
}

// Reset resets the circuit breaker to closed state with zero counters
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.setState(StateClosed)
}

// ForceOpen forces the circuit breaker to open state
func (cb *CircuitBreaker) ForceOpen() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.setState(StateOpen)
}

// Name returns the name of this circuit breaker
func (cb *CircuitBreaker) Name() string {
	return cb.config.Name
}

// createCircuitOpenError creates an error to return when circuit is open
func (cb *CircuitBreaker) createCircuitOpenError() error {
	return fmt.Errorf("circuit breaker '%s' is open", cb.config.Name)
}

// IsCircuitOpenError checks if an error is due to circuit breaker being open
func IsCircuitOpenError(err error) bool {
	if err == nil {
		return false
	}
	// Simple string check - in production this might use error types
	errStr := err.Error()
	return contains(errStr, "circuit breaker") && contains(errStr, "is open")
}

// CircuitBreakerWrapper provides a convenient way to wrap functions with circuit breaker
type CircuitBreakerWrapper struct {
	circuitBreaker *CircuitBreaker
}

// NewCircuitBreakerWrapper creates a new wrapper with the given circuit breaker
func NewCircuitBreakerWrapper(cb *CircuitBreaker) *CircuitBreakerWrapper {
	return &CircuitBreakerWrapper{
		circuitBreaker: cb,
	}
}

// Wrap wraps a function with circuit breaker protection
func (cbw *CircuitBreakerWrapper) Wrap(fn func(context.Context) error) func(context.Context) error {
	return func(ctx context.Context) error {
		return cbw.circuitBreaker.Execute(ctx, fn)
	}
}

// WrapWithRetry wraps a function with both circuit breaker and retry logic
func (cbw *CircuitBreakerWrapper) WrapWithRetry(fn func(context.Context) error, retryConfig *RetryConfig) func(context.Context) error {
	return func(ctx context.Context) error {
		return cbw.circuitBreaker.Execute(ctx, func(ctx context.Context) error {
			return RetryWithConfig(ctx, fn, retryConfig)
		})
	}
}

// RetryConfig holds configuration for retry logic
type RetryConfig struct {
	MaxRetries      int
	InitialDelay    time.Duration
	MaxDelay        time.Duration
	BackoffMultiplier float64
	Jitter          bool
}

// DefaultRetryConfig returns a sensible default retry configuration
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxRetries:      3,
		InitialDelay:    100 * time.Millisecond,
		MaxDelay:        5 * time.Second,
		BackoffMultiplier: 2.0,
		Jitter:          true,
	}
}

// RetryWithConfig executes a function with retry logic
func RetryWithConfig(ctx context.Context, fn func(context.Context) error, config *RetryConfig) error {
	if config == nil {
		config = DefaultRetryConfig()
	}

	var lastErr error
	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Calculate delay with exponential backoff
			delay := time.Duration(float64(config.InitialDelay) * 
				pow(config.BackoffMultiplier, float64(attempt-1)))
			
			if delay > config.MaxDelay {
				delay = config.MaxDelay
			}

			// Add jitter if enabled
			if config.Jitter {
				jitter := time.Duration(float64(delay) * 0.1 * (2.0*rand() - 1.0))
				delay += jitter
			}

			// Wait with context cancellation support
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		err := fn(ctx)
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if classified := ClassifyError(err, "retry"); classified != nil && !classified.IsRetryable() {
			break
		}

		// Check context cancellation
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	return lastErr
}

// Simple power function for exponential backoff
func pow(base, exp float64) float64 {
	result := 1.0
	for i := 0; i < int(exp); i++ {
		result *= base
	}
	return result
}

// Simple random function for jitter (0.0 to 1.0)
func rand() float64 {
	// Simple linear congruential generator
	seed := time.Now().UnixNano()
	seed = (seed*1103515245 + 12345) & 0x7fffffff
	return float64(seed) / float64(0x7fffffff)
}
package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCircuitBreaker_Basic(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold: 3,
		RecoveryTimeout:  100 * time.Millisecond,
		SuccessThreshold: 2,
		MaxRequests:      5,
		Timeout:          time.Second,
		Name:             "test",
	}

	cb := NewCircuitBreaker(config)
	
	// Initially should be closed
	if cb.GetState() != StateClosed {
		t.Errorf("Expected initial state to be Closed, got %v", cb.GetState())
	}

	// Test successful execution
	err := cb.Execute(context.Background(), func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Errorf("Expected no error for successful execution, got %v", err)
	}

	stats := cb.GetStats()
	if stats.TotalSuccesses != 1 {
		t.Errorf("Expected 1 success, got %d", stats.TotalSuccesses)
	}
}

func TestCircuitBreaker_FailureThreshold(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold: 2,
		RecoveryTimeout:  100 * time.Millisecond,
		SuccessThreshold: 1,
		MaxRequests:      5,
		Timeout:          time.Second,
		Name:             "test",
	}

	cb := NewCircuitBreaker(config)
	
	// Cause failures to reach threshold
	for i := 0; i < 2; i++ {
		err := cb.Execute(context.Background(), func(ctx context.Context) error {
			return errors.New("test failure")
		})
		if err == nil {
			t.Errorf("Expected error for failing function")
		}
	}

	// Circuit should now be open
	if cb.GetState() != StateOpen {
		t.Errorf("Expected state to be Open after failures, got %v", cb.GetState())
	}

	// Next request should fail fast
	err := cb.Execute(context.Background(), func(ctx context.Context) error {
		return nil
	})
	if err == nil || !IsCircuitOpenError(err) {
		t.Errorf("Expected circuit open error, got %v", err)
	}
}

func TestCircuitBreaker_Recovery(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold: 2,
		RecoveryTimeout:  50 * time.Millisecond,
		SuccessThreshold: 1,
		MaxRequests:      5,
		Timeout:          time.Second,
		Name:             "test",
	}

	cb := NewCircuitBreaker(config)
	
	// Cause failures to open circuit
	for i := 0; i < 2; i++ {
		cb.Execute(context.Background(), func(ctx context.Context) error {
			return errors.New("test failure")
		})
	}

	if cb.GetState() != StateOpen {
		t.Errorf("Expected state to be Open, got %v", cb.GetState())
	}

	// Wait for recovery timeout
	time.Sleep(100 * time.Millisecond)

	// Next request should transition to half-open
	err := cb.Execute(context.Background(), func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Errorf("Expected successful execution in half-open state, got %v", err)
	}

	// Circuit should now be closed again
	if cb.GetState() != StateClosed {
		t.Errorf("Expected state to be Closed after successful recovery, got %v", cb.GetState())
	}
}

func TestCircuitBreaker_HalfOpenFailure(t *testing.T) {
	config := &CircuitBreakerConfig{
		FailureThreshold: 1,
		RecoveryTimeout:  50 * time.Millisecond,
		SuccessThreshold: 2,
		MaxRequests:      5,
		Timeout:          time.Second,
		Name:             "test",
	}

	cb := NewCircuitBreaker(config)
	
	// Cause failure to open circuit
	cb.Execute(context.Background(), func(ctx context.Context) error {
		return errors.New("test failure")
	})

	// Wait for recovery timeout
	time.Sleep(100 * time.Millisecond)

	// Fail in half-open state
	err := cb.Execute(context.Background(), func(ctx context.Context) error {
		return errors.New("test failure")
	})
	if err == nil {
		t.Errorf("Expected error for failing function")
	}

	// Circuit should be open again
	if cb.GetState() != StateOpen {
		t.Errorf("Expected state to be Open after half-open failure, got %v", cb.GetState())
	}
}

func TestCircuitBreaker_StateChangeCallback(t *testing.T) {
	config := DefaultCircuitBreakerConfig("test")
	config.FailureThreshold = 1

	cb := NewCircuitBreaker(config)
	
	var stateChanges []CircuitBreakerState
	cb.SetStateChangeCallback(func(from, to CircuitBreakerState) {
		stateChanges = append(stateChanges, to)
	})

	// Trigger state change to open
	cb.Execute(context.Background(), func(ctx context.Context) error {
		return errors.New("test failure")
	})

	// Give callback time to execute
	time.Sleep(10 * time.Millisecond)

	if len(stateChanges) != 1 || stateChanges[0] != StateOpen {
		t.Errorf("Expected one state change to Open, got %v", stateChanges)
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	config := DefaultCircuitBreakerConfig("test")
	config.FailureThreshold = 1

	cb := NewCircuitBreaker(config)
	
	// Open the circuit
	cb.Execute(context.Background(), func(ctx context.Context) error {
		return errors.New("test failure")
	})

	if cb.GetState() != StateOpen {
		t.Errorf("Expected state to be Open, got %v", cb.GetState())
	}

	// Reset should close the circuit
	cb.Reset()

	if cb.GetState() != StateClosed {
		t.Errorf("Expected state to be Closed after reset, got %v", cb.GetState())
	}

	stats := cb.GetStats()
	if stats.Failures != 0 || stats.TotalFailures == 0 {
		t.Errorf("Expected current failures to be 0 but total failures preserved")
	}
}

func TestErrorClassification(t *testing.T) {
	tests := []struct {
		err      error
		expected ErrorType
		retryable bool
	}{
		{errors.New("connection refused"), NetworkError, true},
		{errors.New("timeout occurred"), TimeoutError, true},
		{errors.New("file not found"), PermanentError, false},
		{errors.New("rate limit exceeded"), RateLimitError, true},
		{errors.New("unauthorized access"), AuthenticationError, false},
		{errors.New("service unavailable"), TransientError, true},
		{errors.New("unknown error"), UnknownError, true},
	}

	for _, test := range tests {
		classified := ClassifyError(test.err, "test")
		if classified.Type != test.expected {
			t.Errorf("Expected error type %v for '%v', got %v", 
				test.expected, test.err, classified.Type)
		}
		if classified.Retryable != test.retryable {
			t.Errorf("Expected retryable %v for '%v', got %v", 
				test.retryable, test.err, classified.Retryable)
		}
	}
}

func TestRetryWithConfig(t *testing.T) {
	config := &RetryConfig{
		MaxRetries:      2,
		InitialDelay:    10 * time.Millisecond,
		MaxDelay:        100 * time.Millisecond,
		BackoffMultiplier: 2.0,
		Jitter:          false,
	}

	attempts := 0
	err := RetryWithConfig(context.Background(), func(ctx context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary failure")
		}
		return nil
	}, config)

	if err != nil {
		t.Errorf("Expected no error after retries, got %v", err)
	}

	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}
}

func TestRetryWithConfig_PermanentError(t *testing.T) {
	config := DefaultRetryConfig()

	attempts := 0
	err := RetryWithConfig(context.Background(), func(ctx context.Context) error {
		attempts++
		return errors.New("file not found") // Should be classified as permanent
	}, config)

	if err == nil {
		t.Errorf("Expected error for permanent failure")
	}

	if attempts != 1 {
		t.Errorf("Expected 1 attempt for permanent error, got %d", attempts)
	}
}
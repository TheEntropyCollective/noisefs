package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNetworkResilience_BasicOperation(t *testing.T) {
	nr := NewNetworkResilience(nil, nil)
	defer nr.Stop()

	// Execute a successful read operation
	executed := false
	err := nr.ExecuteRead(context.Background(), func(ctx context.Context) error {
		executed = true
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error for successful operation, got %v", err)
	}

	if !executed {
		t.Errorf("Expected operation to be executed")
	}

	// Check statistics
	stats := nr.GetOperationStats(OperationRead)
	if stats.TotalOperations != 1 {
		t.Errorf("Expected 1 total operation, got %d", stats.TotalOperations)
	}

	if stats.SuccessfulOps != 1 {
		t.Errorf("Expected 1 successful operation, got %d", stats.SuccessfulOps)
	}

	if stats.FailedOps != 0 {
		t.Errorf("Expected 0 failed operations, got %d", stats.FailedOps)
	}
}

func TestNetworkResilience_OperationTypes(t *testing.T) {
	nr := NewNetworkResilience(nil, nil)
	defer nr.Stop()

	operations := []struct {
		name     string
		opType   OperationType
		executor func(context.Context, func(context.Context) error) error
	}{
		{"Read", OperationRead, nr.ExecuteRead},
		{"Write", OperationWrite, nr.ExecuteWrite},
		{"Delete", OperationDelete, nr.ExecuteDelete},
		{"List", OperationList, nr.ExecuteList},
		{"Sync", OperationSync, nr.ExecuteSync},
		{"Query", OperationQuery, nr.ExecuteQuery},
	}

	for _, op := range operations {
		t.Run(op.name, func(t *testing.T) {
			executed := false
			err := op.executor(context.Background(), func(ctx context.Context) error {
				executed = true
				return nil
			})

			if err != nil {
				t.Errorf("Expected no error for %s operation, got %v", op.name, err)
			}

			if !executed {
				t.Errorf("Expected %s operation to be executed", op.name)
			}

			stats := nr.GetOperationStats(op.opType)
			if stats.TotalOperations == 0 {
				t.Errorf("Expected at least 1 operation for %s", op.name)
			}
		})
	}
}

func TestNetworkResilience_RetryOnFailure(t *testing.T) {
	config := DefaultNetworkResilienceConfig()
	config.OperationConfigs[OperationRead].RetryPolicy = &RetryConfig{
		MaxRetries:        2,
		InitialDelay:      1 * time.Millisecond,
		MaxDelay:          10 * time.Millisecond,
		BackoffMultiplier: 2.0,
		Jitter:            false,
	}

	nr := NewNetworkResilience(config, nil)
	defer nr.Stop()

	attempts := 0
	err := nr.ExecuteRead(context.Background(), func(ctx context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary failure")
		}
		return nil // Success on third attempt
	})

	if err != nil {
		t.Errorf("Expected no error after retries, got %v", err)
	}

	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}

	stats := nr.GetOperationStats(OperationRead)
	if stats.SuccessfulOps != 1 {
		t.Errorf("Expected 1 successful operation, got %d", stats.SuccessfulOps)
	}
}

func TestNetworkResilience_PermanentFailureNoRetry(t *testing.T) {
	config := DefaultNetworkResilienceConfig()
	config.OperationConfigs[OperationRead].RetryPolicy = DefaultRetryConfig()

	nr := NewNetworkResilience(config, nil)
	defer nr.Stop()

	attempts := 0
	err := nr.ExecuteRead(context.Background(), func(ctx context.Context) error {
		attempts++
		return errors.New("file not found") // Permanent error
	})

	if err == nil {
		t.Errorf("Expected error for permanent failure")
	}

	// Should only attempt once for permanent errors
	if attempts != 1 {
		t.Errorf("Expected 1 attempt for permanent error, got %d", attempts)
	}

	stats := nr.GetOperationStats(OperationRead)
	if stats.FailedOps != 1 {
		t.Errorf("Expected 1 failed operation, got %d", stats.FailedOps)
	}
}

func TestNetworkResilience_CircuitBreakerIntegration(t *testing.T) {
	config := DefaultNetworkResilienceConfig()
	config.CircuitBreakerConfig.FailureThreshold = 2
	config.CircuitBreakerConfig.RecoveryTimeout = 10 * time.Millisecond

	nr := NewNetworkResilience(config, nil)
	defer nr.Stop()

	// Cause failures to open circuit
	for i := 0; i < 2; i++ {
		nr.ExecuteRead(context.Background(), func(ctx context.Context) error {
			return errors.New("service failure")
		})
	}

	// Next request should fail fast due to open circuit
	err := nr.ExecuteRead(context.Background(), func(ctx context.Context) error {
		return nil // This shouldn't execute
	})

	if err == nil || !IsCircuitOpenError(err) {
		t.Errorf("Expected circuit open error, got %v", err)
	}

	// Check circuit breaker stats
	cbStats := nr.GetCircuitBreakerStats()
	if cbStats == nil {
		t.Errorf("Expected circuit breaker stats")
	}

	if cbStats.State != StateOpen {
		t.Errorf("Expected circuit to be open, got %v", cbStats.State)
	}
}

func TestNetworkResilience_OperationTimeout(t *testing.T) {
	config := DefaultNetworkResilienceConfig()
	config.OperationConfigs[OperationRead].Timeout = 10 * time.Millisecond
	config.OperationConfigs[OperationRead].RetryPolicy.MaxRetries = 0 // No retries for cleaner test

	nr := NewNetworkResilience(config, nil)
	defer nr.Stop()

	err := nr.ExecuteRead(context.Background(), func(ctx context.Context) error {
		// Sleep longer than timeout
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
			return nil
		}
	})

	if err == nil {
		t.Errorf("Expected timeout error")
	}

	if err != context.DeadlineExceeded {
		t.Errorf("Expected context deadline exceeded error, got %v", err)
	}
}

func TestNetworkResilience_DisabledOperation(t *testing.T) {
	config := DefaultNetworkResilienceConfig()
	config.OperationConfigs[OperationRead].Enabled = false

	nr := NewNetworkResilience(config, nil)
	defer nr.Stop()

	err := nr.ExecuteRead(context.Background(), func(ctx context.Context) error {
		t.Errorf("Function should not be executed for disabled operation")
		return nil
	})

	if err == nil {
		t.Errorf("Expected error for disabled operation")
	}

	if err.Error() != "operation type Read is disabled" {
		t.Errorf("Expected disabled operation error, got %v", err)
	}
}

func TestNetworkResilience_WithConnectionManager(t *testing.T) {
	cm := NewConnectionManager(nil)
	defer cm.Stop()

	// Add a backend
	backend := &Backend{
		ID:       "test",
		Name:     "Test Backend",
		Address:  "localhost:8080",
		Priority: 1,
		Primary:  true,
	}

	err := cm.AddBackend(backend, func(ctx context.Context) error {
		return nil // Always healthy
	})
	if err != nil {
		t.Errorf("Expected no error adding backend, got %v", err)
	}

	cm.Start()
	time.Sleep(10 * time.Millisecond) // Let health check complete

	nr := NewNetworkResilience(nil, cm)
	defer nr.Stop()

	// Execute operation with backend
	executed := false
	err = nr.ExecuteOperationWithBackend(context.Background(), OperationRead, func(ctx context.Context, b *Backend) error {
		executed = true
		if b.ID != "test" {
			t.Errorf("Expected backend ID 'test', got %s", b.ID)
		}
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error with backend operation, got %v", err)
	}

	if !executed {
		t.Errorf("Expected operation to be executed")
	}
}

func TestNetworkResilience_StatisticsTracking(t *testing.T) {
	nr := NewNetworkResilience(nil, nil)
	defer nr.Stop()

	// Execute multiple operations
	for i := 0; i < 5; i++ {
		nr.ExecuteRead(context.Background(), func(ctx context.Context) error {
			if i%2 == 0 {
				return nil // Success
			}
			return errors.New("failure") // Failure
		})
	}

	stats := nr.GetOperationStats(OperationRead)
	if stats.TotalOperations != 5 {
		t.Errorf("Expected 5 total operations, got %d", stats.TotalOperations)
	}

	if stats.SuccessfulOps != 3 {
		t.Errorf("Expected 3 successful operations, got %d", stats.SuccessfulOps)
	}

	if stats.FailedOps != 2 {
		t.Errorf("Expected 2 failed operations, got %d", stats.FailedOps)
	}

	successRate := stats.GetSuccessRate()
	expectedRate := 3.0 / 5.0
	if successRate != expectedRate {
		t.Errorf("Expected success rate of %f, got %f", expectedRate, successRate)
	}

	failureRate := stats.GetFailureRate()
	expectedFailureRate := 2.0 / 5.0
	if failureRate != expectedFailureRate {
		t.Errorf("Expected failure rate of %f, got %f", expectedFailureRate, failureRate)
	}
}

func TestNetworkResilience_GetAllOperationStats(t *testing.T) {
	nr := NewNetworkResilience(nil, nil)
	defer nr.Stop()

	// Execute different operations
	nr.ExecuteRead(context.Background(), func(ctx context.Context) error { return nil })
	nr.ExecuteWrite(context.Background(), func(ctx context.Context) error { return nil })

	allStats := nr.GetAllOperationStats()

	if len(allStats) == 0 {
		t.Errorf("Expected operation stats to be available")
	}

	if readStats, exists := allStats[OperationRead]; exists {
		if readStats.TotalOperations != 1 {
			t.Errorf("Expected 1 read operation, got %d", readStats.TotalOperations)
		}
	} else {
		t.Errorf("Expected read operation stats")
	}

	if writeStats, exists := allStats[OperationWrite]; exists {
		if writeStats.TotalOperations != 1 {
			t.Errorf("Expected 1 write operation, got %d", writeStats.TotalOperations)
		}
	} else {
		t.Errorf("Expected write operation stats")
	}
}

func TestNetworkResilience_ResetStats(t *testing.T) {
	nr := NewNetworkResilience(nil, nil)
	defer nr.Stop()

	// Execute some operations
	nr.ExecuteRead(context.Background(), func(ctx context.Context) error { return nil })
	nr.ExecuteRead(context.Background(), func(ctx context.Context) error { return errors.New("failure") })

	// Verify stats exist
	stats := nr.GetOperationStats(OperationRead)
	if stats.TotalOperations != 2 {
		t.Errorf("Expected 2 operations before reset, got %d", stats.TotalOperations)
	}

	// Reset stats
	nr.ResetStats()

	// Verify stats are reset
	stats = nr.GetOperationStats(OperationRead)
	if stats.TotalOperations != 0 {
		t.Errorf("Expected 0 operations after reset, got %d", stats.TotalOperations)
	}

	if stats.SuccessfulOps != 0 {
		t.Errorf("Expected 0 successful operations after reset, got %d", stats.SuccessfulOps)
	}

	if stats.FailedOps != 0 {
		t.Errorf("Expected 0 failed operations after reset, got %d", stats.FailedOps)
	}
}

func TestNetworkResilience_UpdateOperationConfig(t *testing.T) {
	nr := NewNetworkResilience(nil, nil)
	defer nr.Stop()

	// Update configuration for read operations
	newConfig := &OperationConfig{
		Timeout:     5 * time.Second,
		RetryPolicy: &RetryConfig{MaxRetries: 1},
		Enabled:     false,
	}

	nr.UpdateOperationConfig(OperationRead, newConfig)

	// Check that operation is now disabled
	if nr.IsOperationEnabled(OperationRead) {
		t.Errorf("Expected read operation to be disabled")
	}

	// Try to execute disabled operation
	err := nr.ExecuteRead(context.Background(), func(ctx context.Context) error {
		return nil
	})

	if err == nil {
		t.Errorf("Expected error for disabled operation")
	}
}

func TestOperationType_String(t *testing.T) {
	tests := []struct {
		opType   OperationType
		expected string
	}{
		{OperationRead, "Read"},
		{OperationWrite, "Write"},
		{OperationDelete, "Delete"},
		{OperationList, "List"},
		{OperationSync, "Sync"},
		{OperationQuery, "Query"},
	}

	for _, test := range tests {
		if test.opType.String() != test.expected {
			t.Errorf("Expected %s for operation type %d, got %s", test.expected, test.opType, test.opType.String())
		}
	}
}

func TestNetworkResilience_NoConnectionManager(t *testing.T) {
	nr := NewNetworkResilience(nil, nil)
	defer nr.Stop()

	err := nr.ExecuteOperationWithBackend(context.Background(), OperationRead, func(ctx context.Context, b *Backend) error {
		return nil
	})

	if err == nil {
		t.Errorf("Expected error when no connection manager configured")
	}

	if err.Error() != "no connection manager configured" {
		t.Errorf("Expected no connection manager error, got %v", err)
	}
}
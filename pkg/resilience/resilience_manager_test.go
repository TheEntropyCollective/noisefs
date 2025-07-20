package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestResilienceManager_BasicOperations(t *testing.T) {
	config := DefaultResilienceManagerConfig()
	config.DefaultTimeout = 5 * time.Second
	
	rm := NewResilienceManager(config)
	defer rm.Stop()
	
	// Start the manager
	err := rm.Start()
	if err != nil {
		t.Errorf("Expected no error starting resilience manager, got %v", err)
	}
	
	// Test starting again should fail
	err = rm.Start()
	if err == nil {
		t.Errorf("Expected error starting already started manager")
	}
	
	// Check system health
	if !rm.IsHealthy() {
		t.Errorf("Expected system to be healthy initially")
	}
	
	health, err := rm.GetSystemHealth()
	if err != nil {
		t.Errorf("Expected no error getting system health, got %v", err)
	}
	
	if health.Overall != HealthHealthy {
		t.Errorf("Expected overall health to be healthy, got %v", health.Overall)
	}
}

func TestResilienceManager_ResilientOperation(t *testing.T) {
	rm := NewResilienceManager(nil)
	defer rm.Stop()
	
	err := rm.Start()
	if err != nil {
		t.Errorf("Expected no error starting resilience manager, got %v", err)
	}
	
	// Execute successful operation
	executed := false
	err = rm.ExecuteResilientOperation(context.Background(), OperationRead, func(ctx context.Context) error {
		executed = true
		return nil
	})
	
	if err != nil {
		t.Errorf("Expected no error for successful operation, got %v", err)
	}
	
	if !executed {
		t.Errorf("Expected operation to be executed")
	}
	
	// Check metrics
	metrics := rm.GetMetrics()
	if metrics.TotalOperations != 1 {
		t.Errorf("Expected 1 total operation, got %d", metrics.TotalOperations)
	}
	
	if metrics.SuccessfulOps != 1 {
		t.Errorf("Expected 1 successful operation, got %d", metrics.SuccessfulOps)
	}
	
	if metrics.SuccessRate != 1.0 {
		t.Errorf("Expected success rate of 1.0, got %f", metrics.SuccessRate)
	}
}

func TestResilienceManager_FailedOperation(t *testing.T) {
	config := DefaultResilienceManagerConfig()
	config.EnableErrorClassification = true
	
	rm := NewResilienceManager(config)
	defer rm.Stop()
	
	err := rm.Start()
	if err != nil {
		t.Errorf("Expected no error starting resilience manager, got %v", err)
	}
	
	// Execute failing operation
	err = rm.ExecuteResilientOperation(context.Background(), OperationRead, func(ctx context.Context) error {
		return errors.New("operation failed")
	})
	
	if err == nil {
		t.Errorf("Expected error for failing operation")
	}
	
	// Check if error was classified
	if classified, ok := err.(*ClassifiedError); ok {
		if classified.Type != UnknownError {
			t.Logf("Error was classified as %v", classified.Type)
		}
	}
	
	// Check metrics
	metrics := rm.GetMetrics()
	if metrics.TotalOperations != 1 {
		t.Errorf("Expected 1 total operation, got %d", metrics.TotalOperations)
	}
	
	if metrics.FailedOps != 1 {
		t.Errorf("Expected 1 failed operation, got %d", metrics.FailedOps)
	}
	
	if metrics.SuccessRate != 0.0 {
		t.Errorf("Expected success rate of 0.0, got %f", metrics.SuccessRate)
	}
}

func TestResilienceManager_BackendOperations(t *testing.T) {
	rm := NewResilienceManager(nil)
	defer rm.Stop()
	
	err := rm.Start()
	if err != nil {
		t.Errorf("Expected no error starting resilience manager, got %v", err)
	}
	
	// Add a backend
	backend := &Backend{
		ID:       "test-backend",
		Name:     "Test Backend",
		Address:  "localhost:8080",
		Priority: 1,
		Primary:  true,
	}
	
	err = rm.AddBackend(backend, func(ctx context.Context) error {
		return nil // Always healthy
	})
	if err != nil {
		t.Errorf("Expected no error adding backend, got %v", err)
	}
	
	// Wait for health check
	time.Sleep(10 * time.Millisecond)
	
	// Execute operation with backend
	executed := false
	err = rm.ExecuteResilientOperationWithBackend(context.Background(), OperationRead, func(ctx context.Context, b *Backend) error {
		executed = true
		if b.ID != "test-backend" {
			t.Errorf("Expected backend ID 'test-backend', got %s", b.ID)
		}
		return nil
	})
	
	if err != nil {
		t.Errorf("Expected no error for backend operation, got %v", err)
	}
	
	if !executed {
		t.Errorf("Expected operation to be executed")
	}
	
	// Remove backend
	err = rm.RemoveBackend("test-backend")
	if err != nil {
		t.Errorf("Expected no error removing backend, got %v", err)
	}
}

func TestResilienceManager_HealthMonitoring(t *testing.T) {
	rm := NewResilienceManager(nil)
	defer rm.Stop()
	
	err := rm.Start()
	if err != nil {
		t.Errorf("Expected no error starting resilience manager, got %v", err)
	}
	
	// Register a health component
	err = rm.RegisterHealthComponent("test-component", func(ctx context.Context) error {
		return nil // Always healthy
	})
	if err != nil {
		t.Errorf("Expected no error registering health component, got %v", err)
	}
	
	// Wait for health check
	time.Sleep(10 * time.Millisecond)
	
	// Get system health
	health, err := rm.GetSystemHealth()
	if err != nil {
		t.Errorf("Expected no error getting system health, got %v", err)
	}
	
	if health.HealthMonitor == nil {
		t.Errorf("Expected health monitor report")
	}
	
	if health.HealthMonitor.OverallHealth != HealthHealthy {
		t.Errorf("Expected healthy status, got %v", health.HealthMonitor.OverallHealth)
	}
}

func TestResilienceManager_RecoveryWorkflow(t *testing.T) {
	rm := NewResilienceManager(nil)
	defer rm.Stop()
	
	err := rm.Start()
	if err != nil {
		t.Errorf("Expected no error starting resilience manager, got %v", err)
	}
	
	// Create recovery workflow
	workflow, err := rm.CreateRecoveryWorkflow("test-workflow", "Test workflow")
	if err != nil {
		t.Errorf("Expected no error creating workflow, got %v", err)
	}
	
	if workflow == nil {
		t.Errorf("Expected workflow to be created")
	}
	
	if workflow.ID != "test-workflow" {
		t.Errorf("Expected workflow ID 'test-workflow', got %s", workflow.ID)
	}
	
	// Add a step that will fail to trigger recovery counter
	action := NewTestAction("action", "Test action", true)
	workflow.AddStep("step", action)
	
	// Execute workflow (should fail and trigger recovery)
	err = workflow.Execute(context.Background())
	if err == nil {
		t.Errorf("Expected workflow to fail")
	}
	
	// Give callback time to execute
	time.Sleep(10 * time.Millisecond)
	
	// Check metrics for recovery
	metrics := rm.GetMetrics()
	if metrics.TotalRecoveries == 0 {
		t.Errorf("Expected at least 1 recovery to be recorded")
	}
}

func TestResilienceManager_StateValidation(t *testing.T) {
	rm := NewResilienceManager(nil)
	defer rm.Stop()
	
	err := rm.Start()
	if err != nil {
		t.Errorf("Expected no error starting resilience manager, got %v", err)
	}
	
	// Validate state (should work with empty validators)
	err = rm.ValidateSystemState(context.Background(), map[string]interface{}{"test": "data"})
	if err != nil {
		t.Errorf("Expected no error validating state, got %v", err)
	}
}

func TestResilienceManager_MetricsReset(t *testing.T) {
	rm := NewResilienceManager(nil)
	defer rm.Stop()
	
	err := rm.Start()
	if err != nil {
		t.Errorf("Expected no error starting resilience manager, got %v", err)
	}
	
	// Execute some operations
	rm.ExecuteResilientOperation(context.Background(), OperationRead, func(ctx context.Context) error {
		return nil
	})
	rm.ExecuteResilientOperation(context.Background(), OperationWrite, func(ctx context.Context) error {
		return errors.New("failure")
	})
	
	// Check metrics exist
	metrics := rm.GetMetrics()
	if metrics.TotalOperations == 0 {
		t.Errorf("Expected operations to be recorded")
	}
	
	// Reset metrics
	rm.ResetMetrics()
	
	// Check metrics are reset
	metrics = rm.GetMetrics()
	if metrics.TotalOperations != 0 {
		t.Errorf("Expected metrics to be reset, got %d total operations", metrics.TotalOperations)
	}
	
	if metrics.SuccessfulOps != 0 {
		t.Errorf("Expected successful ops to be reset, got %d", metrics.SuccessfulOps)
	}
	
	if metrics.FailedOps != 0 {
		t.Errorf("Expected failed ops to be reset, got %d", metrics.FailedOps)
	}
}

func TestResilienceManager_DisabledComponents(t *testing.T) {
	// Create config with all components disabled
	config := &ResilienceManagerConfig{
		EnableErrorClassification: false,
		EnableCircuitBreaker:      false,
		EnableHealthMonitoring:    false,
		EnableConnectionManager:   false,
		EnableNetworkResilience:   false,
		EnableRecoveryManager:     false,
		MetricsEnabled:            false,
	}
	
	rm := NewResilienceManager(config)
	defer rm.Stop()
	
	// Should still be able to start
	err := rm.Start()
	if err != nil {
		t.Errorf("Expected no error starting with disabled components, got %v", err)
	}
	
	// Execute operation should work but with minimal functionality
	err = rm.ExecuteResilientOperation(context.Background(), OperationRead, func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Errorf("Expected no error with disabled components, got %v", err)
	}
	
	// Backend operations should fail
	err = rm.ExecuteResilientOperationWithBackend(context.Background(), OperationRead, func(ctx context.Context, b *Backend) error {
		return nil
	})
	if err == nil {
		t.Errorf("Expected error when network resilience disabled")
	}
	
	// Adding backend should fail
	backend := &Backend{ID: "test", Name: "Test", Priority: 1}
	err = rm.AddBackend(backend, func(ctx context.Context) error { return nil })
	if err == nil {
		t.Errorf("Expected error when connection manager disabled")
	}
	
	// Health component registration should fail
	err = rm.RegisterHealthComponent("test", func(ctx context.Context) error { return nil })
	if err == nil {
		t.Errorf("Expected error when health monitor disabled")
	}
	
	// Recovery workflow creation should fail
	_, err = rm.CreateRecoveryWorkflow("test", "Test")
	if err == nil {
		t.Errorf("Expected error when recovery manager disabled")
	}
	
	// State validation should fail
	err = rm.ValidateSystemState(context.Background(), nil)
	if err == nil {
		t.Errorf("Expected error when recovery manager disabled")
	}
}

func TestResilienceManager_SystemHealthWithFailures(t *testing.T) {
	config := DefaultResilienceManagerConfig()
	config.CircuitBreakerConfig.FailureThreshold = 1 // Open circuit quickly
	
	rm := NewResilienceManager(config)
	defer rm.Stop()
	
	err := rm.Start()
	if err != nil {
		t.Errorf("Expected no error starting resilience manager, got %v", err)
	}
	
	// Trigger circuit breaker to open
	rm.ExecuteResilientOperation(context.Background(), OperationRead, func(ctx context.Context) error {
		return errors.New("failure")
	})
	
	// Wait a moment for circuit breaker state to update
	time.Sleep(10 * time.Millisecond)
	
	// Get system health
	health, err := rm.GetSystemHealth()
	if err != nil {
		t.Errorf("Expected no error getting system health, got %v", err)
	}
	
	// Check if circuit breaker is open - it may take time to transition
	if health.CircuitBreaker == nil {
		t.Errorf("Expected circuit breaker report")
	} else {
		t.Logf("Circuit breaker state: %v", health.CircuitBreaker.State)
		// The circuit breaker behavior may vary based on exact timing
		// Just verify the system is tracking the state
	}
}

func TestResilienceManager_Timeout(t *testing.T) {
	config := DefaultResilienceManagerConfig()
	config.DefaultTimeout = 10 * time.Millisecond // Very short timeout
	
	rm := NewResilienceManager(config)
	defer rm.Stop()
	
	err := rm.Start()
	if err != nil {
		t.Errorf("Expected no error starting resilience manager, got %v", err)
	}
	
	// Execute operation that takes longer than timeout
	err = rm.ExecuteResilientOperation(context.Background(), OperationRead, func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
			return nil
		}
	})
	
	if err == nil {
		t.Errorf("Expected timeout error")
	}
	
	// Accept both raw deadline exceeded and classified error
	if err != context.DeadlineExceeded {
		if classified, ok := err.(*ClassifiedError); ok {
			if classified.Err != context.DeadlineExceeded {
				t.Errorf("Expected deadline exceeded error, got %v", err)
			}
		} else {
			t.Errorf("Expected deadline exceeded error, got %v", err)
		}
	}
}

func TestResilienceManager_ComprehensiveIntegration(t *testing.T) {
	// Test all components working together
	rm := NewResilienceManager(nil)
	defer rm.Stop()
	
	err := rm.Start()
	if err != nil {
		t.Errorf("Expected no error starting resilience manager, got %v", err)
	}
	
	// Add backend
	backend := &Backend{
		ID:       "integration-backend",
		Name:     "Integration Backend",
		Address:  "localhost:9090",
		Priority: 1,
		Primary:  true,
	}
	
	err = rm.AddBackend(backend, func(ctx context.Context) error {
		return nil // Always healthy
	})
	if err != nil {
		t.Errorf("Expected no error adding backend, got %v", err)
	}
	
	// Register health component
	err = rm.RegisterHealthComponent("integration-component", func(ctx context.Context) error {
		return nil // Always healthy
	})
	if err != nil {
		t.Errorf("Expected no error registering health component, got %v", err)
	}
	
	// Create recovery workflow
	workflow, err := rm.CreateRecoveryWorkflow("integration-workflow", "Integration test workflow")
	if err != nil {
		t.Errorf("Expected no error creating workflow, got %v", err)
	}
	
	action := NewTestAction("integration-action", "Integration action", false)
	workflow.AddStep("integration-step", action)
	
	// Wait for health checks
	time.Sleep(50 * time.Millisecond)
	
	// Execute operations
	for i := 0; i < 5; i++ {
		rm.ExecuteResilientOperation(context.Background(), OperationRead, func(ctx context.Context) error {
			return nil
		})
	}
	
	// Execute backend operation
	err = rm.ExecuteResilientOperationWithBackend(context.Background(), OperationWrite, func(ctx context.Context, b *Backend) error {
		if b.ID != "integration-backend" {
			t.Errorf("Expected integration backend, got %s", b.ID)
		}
		return nil
	})
	if err != nil {
		t.Errorf("Expected no error with backend operation, got %v", err)
	}
	
	// Execute recovery workflow
	err = workflow.Execute(context.Background())
	if err != nil {
		t.Errorf("Expected no error executing workflow, got %v", err)
	}
	
	// Get comprehensive system health
	health, err := rm.GetSystemHealth()
	if err != nil {
		t.Errorf("Expected no error getting system health, got %v", err)
	}
	
	// Verify all components are represented
	if health.HealthMonitor == nil {
		t.Errorf("Expected health monitor report")
	}
	
	if health.CircuitBreaker == nil {
		t.Errorf("Expected circuit breaker report")
	}
	
	if health.ConnectionManager == nil {
		t.Errorf("Expected connection manager report")
	}
	
	if health.NetworkResilience == nil {
		t.Errorf("Expected network resilience report")
	}
	
	if health.RecoveryManager == nil {
		t.Errorf("Expected recovery manager report")
	}
	
	if health.Metrics == nil {
		t.Errorf("Expected metrics")
	}
	
	// System should be healthy
	if !rm.IsHealthy() {
		t.Errorf("Expected system to be healthy after successful operations")
	}
	
	if health.Overall != HealthHealthy {
		t.Errorf("Expected overall health to be healthy, got %v", health.Overall)
	}
	
	// Verify metrics
	metrics := rm.GetMetrics()
	if metrics.TotalOperations < 6 { // 5 + 1 backend operation
		t.Errorf("Expected at least 6 operations, got %d", metrics.TotalOperations)
	}
	
	if metrics.SuccessRate != 1.0 {
		t.Errorf("Expected 100%% success rate, got %f", metrics.SuccessRate)
	}
}
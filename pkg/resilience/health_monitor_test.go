package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestHealthMonitor_BasicRegistration(t *testing.T) {
	hm := NewHealthMonitor(nil)
	defer hm.Stop()

	// Register a healthy component
	hm.RegisterComponent("test-component", func(ctx context.Context) error {
		return nil
	})

	// Check immediate health
	result, err := hm.CheckNow("test-component")
	if err != nil {
		t.Errorf("Expected no error for health check, got %v", err)
	}

	if result.Status != HealthHealthy {
		t.Errorf("Expected healthy status, got %v", result.Status)
	}

	// Get component health
	health, exists := hm.GetComponentHealth("test-component")
	if !exists {
		t.Errorf("Expected component to exist")
	}

	if !health.IsHealthy() {
		t.Errorf("Expected component to be healthy")
	}
}

func TestHealthMonitor_FailureDetection(t *testing.T) {
	config := &HealthMonitorConfig{
		CheckInterval:      10 * time.Millisecond,
		CheckTimeout:       time.Second,
		MaxRecentResults:   10,
		DegradedThreshold:  2,
		UnhealthyThreshold: 3,
		CriticalThreshold:  5,
		RecoveryThreshold:  1,
	}

	hm := NewHealthMonitor(config)
	defer hm.Stop()

	failureCount := 0
	hm.RegisterComponent("failing-component", func(ctx context.Context) error {
		failureCount++
		return errors.New("component failure")
	})

	// Wait for enough checks to mark as degraded
	time.Sleep(50 * time.Millisecond)

	health, exists := hm.GetComponentHealth("failing-component")
	if !exists {
		t.Errorf("Expected component to exist")
	}

	if !health.IsDegraded() {
		t.Errorf("Expected component to be degraded, got status %v", health.Status)
	}

	// Wait longer for unhealthy
	time.Sleep(50 * time.Millisecond)

	health, exists = hm.GetComponentHealth("failing-component")
	if !exists {
		t.Errorf("Expected component to exist")
	}

	if !health.IsUnhealthy() {
		t.Errorf("Expected component to be unhealthy, got status %v", health.Status)
	}
}

func TestHealthMonitor_Recovery(t *testing.T) {
	config := &HealthMonitorConfig{
		CheckInterval:      10 * time.Millisecond,
		CheckTimeout:       time.Second,
		MaxRecentResults:   10,
		DegradedThreshold:  1,
		UnhealthyThreshold: 2,
		CriticalThreshold:  3,
		RecoveryThreshold:  1,
	}

	hm := NewHealthMonitor(config)
	defer hm.Stop()

	shouldFail := true
	hm.RegisterComponent("recovery-component", func(ctx context.Context) error {
		if shouldFail {
			return errors.New("component failure")
		}
		return nil
	})

	// Wait for component to become unhealthy
	time.Sleep(50 * time.Millisecond)

	health, _ := hm.GetComponentHealth("recovery-component")
	if !health.IsUnhealthy() {
		t.Errorf("Expected component to be unhealthy, got status %v", health.Status)
	}

	// Fix the component
	shouldFail = false

	// Wait for recovery
	time.Sleep(50 * time.Millisecond)

	health, _ = hm.GetComponentHealth("recovery-component")
	if !health.IsHealthy() {
		t.Errorf("Expected component to recover to healthy, got status %v", health.Status)
	}
}

func TestHealthMonitor_StatusChangeCallback(t *testing.T) {
	config := DefaultHealthMonitorConfig()
	config.CheckInterval = 10 * time.Millisecond
	config.DegradedThreshold = 1

	hm := NewHealthMonitor(config)
	defer hm.Stop()

	statusChanges := make(chan HealthStatus, 10)
	hm.SetStatusChangeCallback(func(componentName string, oldStatus, newStatus HealthStatus) {
		statusChanges <- newStatus
	})

	hm.RegisterComponent("callback-component", func(ctx context.Context) error {
		return errors.New("failure")
	})

	// Wait for status change
	select {
	case status := <-statusChanges:
		if status != HealthDegraded {
			t.Errorf("Expected degraded status change, got %v", status)
		}
	case <-time.After(100 * time.Millisecond):
		t.Errorf("Expected status change callback to be called")
	}
}

func TestHealthMonitor_OverallHealth(t *testing.T) {
	hm := NewHealthMonitor(nil)
	defer hm.Stop()

	// No components - should be unknown
	if hm.GetOverallHealth() != HealthUnknown {
		t.Errorf("Expected unknown health with no components")
	}

	// Add healthy component
	hm.RegisterComponent("healthy", func(ctx context.Context) error {
		return nil
	})

	hm.CheckNow("healthy")

	if hm.GetOverallHealth() != HealthHealthy {
		t.Errorf("Expected overall health to be healthy")
	}

	// Add unhealthy component - trigger multiple failures to reach unhealthy threshold
	hm.RegisterComponent("unhealthy", func(ctx context.Context) error {
		return errors.New("failure")
	})

	// Trigger enough failures to reach unhealthy status (default threshold is 5)
	for i := 0; i < 5; i++ {
		hm.CheckNow("unhealthy")
	}

	// Overall health should be worst status
	if hm.GetOverallHealth() != HealthUnhealthy {
		t.Errorf("Expected overall health to be unhealthy")
	}
}

func TestHealthMonitor_HealthSummary(t *testing.T) {
	hm := NewHealthMonitor(nil)
	defer hm.Stop()

	// Add components with different health states
	hm.RegisterComponent("healthy", func(ctx context.Context) error {
		return nil
	})

	hm.RegisterComponent("unhealthy", func(ctx context.Context) error {
		return errors.New("failure")
	})

	// Perform checks
	hm.CheckNow("healthy")
	// Trigger enough failures to reach unhealthy status (default threshold is 5)
	for i := 0; i < 5; i++ {
		hm.CheckNow("unhealthy")
	}

	summary := hm.GetHealthSummary()

	if summary.TotalComponents != 2 {
		t.Errorf("Expected 2 total components, got %d", summary.TotalComponents)
	}

	if summary.HealthyCount != 1 {
		t.Errorf("Expected 1 healthy component, got %d", summary.HealthyCount)
	}

	if summary.UnhealthyCount != 1 {
		t.Errorf("Expected 1 unhealthy component, got %d", summary.UnhealthyCount)
	}

	if summary.OverallStatus != HealthUnhealthy {
		t.Errorf("Expected overall status to be unhealthy, got %v", summary.OverallStatus)
	}
}

func TestHealthMonitor_CheckTimeout(t *testing.T) {
	config := &HealthMonitorConfig{
		CheckInterval:      time.Hour, // Don't auto-check
		CheckTimeout:       10 * time.Millisecond,
		MaxRecentResults:   10,
		DegradedThreshold:  1,
		UnhealthyThreshold: 2,
		CriticalThreshold:  3,
		RecoveryThreshold:  1,
	}

	hm := NewHealthMonitor(config)
	defer hm.Stop()

	hm.RegisterComponent("slow-component", func(ctx context.Context) error {
		// Sleep longer than timeout
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
			return nil
		}
	})

	result, err := hm.CheckNow("slow-component")
	if err != nil {
		t.Errorf("Expected no error from CheckNow, got %v", err)
	}

	// Should be unhealthy due to timeout
	if result.Status != HealthUnhealthy {
		t.Errorf("Expected unhealthy status due to timeout, got %v", result.Status)
	}

	if result.Error == "" {
		t.Errorf("Expected error message for timeout")
	}
}

func TestHealthMonitor_Unregister(t *testing.T) {
	hm := NewHealthMonitor(nil)
	defer hm.Stop()

	hm.RegisterComponent("temp-component", func(ctx context.Context) error {
		return nil
	})

	// Component should exist
	_, exists := hm.GetComponentHealth("temp-component")
	if !exists {
		t.Errorf("Expected component to exist after registration")
	}

	// Unregister component
	hm.UnregisterComponent("temp-component")

	// Component should no longer exist
	_, exists = hm.GetComponentHealth("temp-component")
	if exists {
		t.Errorf("Expected component to not exist after unregistration")
	}

	// CheckNow should fail
	_, err := hm.CheckNow("temp-component")
	if err == nil {
		t.Errorf("Expected error when checking unregistered component")
	}
}

func TestHealthMonitor_SuccessRate(t *testing.T) {
	config := DefaultHealthMonitorConfig()
	config.MaxRecentResults = 5

	hm := NewHealthMonitor(config)
	defer hm.Stop()

	checkCount := 0
	hm.RegisterComponent("variable-component", func(ctx context.Context) error {
		checkCount++
		// Fail every other check starting with success
		if checkCount%2 == 1 {
			return nil // Success on odd counts (1,3,5...)
		}
		return errors.New("failure") // Fail on even counts (2,4,6...)
	})

	// Perform several checks: S,F,S,F,S,F (6 total)
	for i := 0; i < 6; i++ {
		hm.CheckNow("variable-component")
	}

	health, _ := hm.GetComponentHealth("variable-component")
	successRate := health.GetSuccessRate()

	// With MaxRecentResults=5, we keep the last 5 results: [F,S,F,S,F] = 2/5 = 0.4
	expectedRate := 0.4
	if successRate != expectedRate {
		t.Errorf("Expected success rate of %f, got %f", expectedRate, successRate)
	}
}

func TestComponentHealth_StatusChecks(t *testing.T) {
	component := &ComponentHealth{
		Name:   "test",
		Status: HealthDegraded,
	}

	if component.IsHealthy() {
		t.Errorf("Expected component to not be healthy")
	}

	if !component.IsDegraded() {
		t.Errorf("Expected component to be degraded")
	}

	if component.IsUnhealthy() {
		t.Errorf("Expected component to not be unhealthy")
	}

	if component.IsCritical() {
		t.Errorf("Expected component to not be critical")
	}
}

func TestHealthStatus_String(t *testing.T) {
	tests := []struct {
		status   HealthStatus
		expected string
	}{
		{HealthUnknown, "Unknown"},
		{HealthHealthy, "Healthy"},
		{HealthDegraded, "Degraded"},
		{HealthUnhealthy, "Unhealthy"},
		{HealthCritical, "Critical"},
	}

	for _, test := range tests {
		if test.status.String() != test.expected {
			t.Errorf("Expected %s for status %d, got %s", test.expected, test.status, test.status.String())
		}
	}
}
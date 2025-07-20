package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestConnectionManager_BasicOperations(t *testing.T) {
	cm := NewConnectionManager(nil)
	defer cm.Stop()

	// Add a primary backend
	primary := &Backend{
		ID:       "primary",
		Name:     "Primary Storage",
		Address:  "localhost:8080",
		Priority: 1,
		Primary:  true,
	}

	err := cm.AddBackend(primary, func(ctx context.Context) error {
		return nil // Always healthy
	})
	if err != nil {
		t.Errorf("Expected no error adding backend, got %v", err)
	}

	// Check primary backend
	if cm.GetPrimaryBackend() == nil {
		t.Errorf("Expected primary backend to be set")
	}

	if cm.GetPrimaryBackend().ID != "primary" {
		t.Errorf("Expected primary backend ID to be 'primary', got %s", cm.GetPrimaryBackend().ID)
	}

	cm.Start()
	time.Sleep(10 * time.Millisecond) // Let health check complete

	// Check active backend
	active := cm.GetActiveBackend()
	if active == nil {
		t.Errorf("Expected active backend to be available")
	}

	// Add a secondary backend
	secondary := &Backend{
		ID:       "secondary",
		Name:     "Secondary Storage",
		Address:  "localhost:8081",
		Priority: 2,
		Primary:  false,
	}

	err = cm.AddBackend(secondary, func(ctx context.Context) error {
		return nil // Always healthy
	})
	if err != nil {
		t.Errorf("Expected no error adding secondary backend, got %v", err)
	}

	// Check secondary backend
	if cm.GetSecondaryBackend() == nil {
		t.Errorf("Expected secondary backend to be set")
	}

	if cm.GetSecondaryBackend().ID != "secondary" {
		t.Errorf("Expected secondary backend ID to be 'secondary', got %s", cm.GetSecondaryBackend().ID)
	}
}

func TestConnectionManager_ExecuteWithBackend(t *testing.T) {
	cm := NewConnectionManager(nil)
	defer cm.Stop()

	// Add a backend
	backend := &Backend{
		ID:       "test",
		Name:     "Test Storage",
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

	// Start health monitoring to make backend available
	cm.Start()
	time.Sleep(10 * time.Millisecond) // Let health check complete

	// Execute operation
	executed := false
	err = cm.ExecuteWithBackend(context.Background(), func(ctx context.Context, b *Backend) error {
		executed = true
		if b.ID != "test" {
			t.Errorf("Expected backend ID 'test', got %s", b.ID)
		}
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error executing with backend, got %v", err)
	}

	if !executed {
		t.Errorf("Expected function to be executed")
	}
}

func TestConnectionManager_Failover(t *testing.T) {
	config := DefaultConnectionManagerConfig()
	config.HealthCheckInterval = 10 * time.Millisecond // Fast health checks for testing
	cm := NewConnectionManager(config)
	defer cm.Stop()

	// Track failover events
	cm.SetFailoverCallback(func(from, to *Backend) {
		if from.ID != "primary" {
			t.Errorf("Expected failover from 'primary', got %s", from.ID)
		}
		if to.ID != "secondary" {
			t.Errorf("Expected failover to 'secondary', got %s", to.ID)
		}
	})

	// Add primary backend that will fail
	primaryFailures := 0
	primary := &Backend{
		ID:       "primary",
		Name:     "Primary Storage",
		Address:  "localhost:8080",
		Priority: 1,
		Primary:  true,
	}

	err := cm.AddBackend(primary, func(ctx context.Context) error {
		primaryFailures++
		if primaryFailures > 2 {
			return errors.New("primary backend failure")
		}
		return nil
	})
	if err != nil {
		t.Errorf("Expected no error adding primary backend, got %v", err)
	}

	// Add secondary backend that's always healthy
	secondary := &Backend{
		ID:       "secondary",
		Name:     "Secondary Storage",
		Address:  "localhost:8081",
		Priority: 2,
		Primary:  false,
	}

	err = cm.AddBackend(secondary, func(ctx context.Context) error {
		return nil // Always healthy
	})
	if err != nil {
		t.Errorf("Expected no error adding secondary backend, got %v", err)
	}

	cm.Start()

	// Wait for primary to become unhealthy and failover to occur
	time.Sleep(100 * time.Millisecond)

	// Try failover execution
	executed := false
	err = cm.ExecuteWithFailover(context.Background(), func(ctx context.Context, b *Backend) error {
		executed = true
		// Should execute with secondary due to primary failure
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error with failover execution, got %v", err)
	}

	if !executed {
		t.Errorf("Expected function to be executed")
	}
}

func TestConnectionManager_BackendStatusTracking(t *testing.T) {
	config := DefaultConnectionManagerConfig()
	config.HealthCheckInterval = 10 * time.Millisecond
	cm := NewConnectionManager(config)
	defer cm.Stop()

	// Track status changes
	statusChanges := make(chan ConnectionStatus, 10)
	cm.SetBackendStatusChangeCallback(func(backend *Backend, oldStatus, newStatus ConnectionStatus) {
		statusChanges <- newStatus
	})

	// Add a backend that will become unhealthy
	checkCount := 0
	backend := &Backend{
		ID:       "test",
		Name:     "Test Storage",
		Address:  "localhost:8080",
		Priority: 1,
		Primary:  true,
	}

	err := cm.AddBackend(backend, func(ctx context.Context) error {
		checkCount++
		if checkCount > 3 {
			return errors.New("backend failure")
		}
		return nil
	})
	if err != nil {
		t.Errorf("Expected no error adding backend, got %v", err)
	}

	cm.Start()

	// Wait for status change to active
	select {
	case status := <-statusChanges:
		if status != ConnectionActive {
			t.Errorf("Expected first status change to Active, got %v", status)
		}
	case <-time.After(100 * time.Millisecond):
		t.Errorf("Expected status change to active")
	}

	// Wait for status change to degraded or inactive (due to failures)
	select {
	case status := <-statusChanges:
		if status != ConnectionDegraded && status != ConnectionInactive {
			t.Errorf("Expected status change to Degraded or Inactive, got %v", status)
		}
	case <-time.After(200 * time.Millisecond):
		t.Errorf("Expected status change to degraded/inactive")
	}

	// Verify status can be queried
	finalStatus, err := cm.GetBackendStatus("test")
	if err != nil {
		t.Errorf("Expected no error getting backend status, got %v", err)
	}

	if finalStatus != ConnectionDegraded && finalStatus != ConnectionInactive {
		t.Errorf("Expected final status to be Degraded or Inactive, got %v", finalStatus)
	}
}

func TestConnectionManager_NoAvailableBackends(t *testing.T) {
	cm := NewConnectionManager(nil)
	defer cm.Stop()

	// Try to execute without any backends
	err := cm.ExecuteWithBackend(context.Background(), func(ctx context.Context, b *Backend) error {
		return nil
	})

	if err == nil {
		t.Errorf("Expected error when no backends available")
	}

	if err.Error() != "no available backends" {
		t.Errorf("Expected 'no available backends' error, got %v", err)
	}
}

func TestConnectionManager_RemoveBackend(t *testing.T) {
	cm := NewConnectionManager(nil)
	defer cm.Stop()

	// Add a backend
	backend := &Backend{
		ID:       "test",
		Name:     "Test Storage",
		Address:  "localhost:8080",
		Priority: 1,
		Primary:  true,
	}

	err := cm.AddBackend(backend, func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Errorf("Expected no error adding backend, got %v", err)
	}

	// Verify backend exists
	if cm.GetPrimaryBackend() == nil {
		t.Errorf("Expected primary backend to exist")
	}

	// Remove backend
	err = cm.RemoveBackend("test")
	if err != nil {
		t.Errorf("Expected no error removing backend, got %v", err)
	}

	// Verify backend is gone
	if cm.GetPrimaryBackend() != nil {
		t.Errorf("Expected primary backend to be nil after removal")
	}

	// Try to remove non-existent backend
	err = cm.RemoveBackend("nonexistent")
	if err == nil {
		t.Errorf("Expected error removing non-existent backend")
	}
}

func TestConnectionManager_GetAllBackendStatuses(t *testing.T) {
	cm := NewConnectionManager(nil)
	defer cm.Stop()

	// Add multiple backends
	backends := []*Backend{
		{ID: "backend1", Name: "Backend 1", Priority: 1, Primary: true},
		{ID: "backend2", Name: "Backend 2", Priority: 2, Primary: false},
		{ID: "backend3", Name: "Backend 3", Priority: 3, Primary: false},
	}

	for _, backend := range backends {
		err := cm.AddBackend(backend, func(ctx context.Context) error {
			return nil // All healthy
		})
		if err != nil {
			t.Errorf("Expected no error adding backend %s, got %v", backend.ID, err)
		}
	}

	statuses := cm.GetAllBackendStatuses()

	if len(statuses) != 3 {
		t.Errorf("Expected 3 backend statuses, got %d", len(statuses))
	}

	for _, backend := range backends {
		if _, exists := statuses[backend.ID]; !exists {
			t.Errorf("Expected status for backend %s", backend.ID)
		}
	}
}

func TestConnectionStatus_String(t *testing.T) {
	tests := []struct {
		status   ConnectionStatus
		expected string
	}{
		{ConnectionActive, "Active"},
		{ConnectionDegraded, "Degraded"},
		{ConnectionInactive, "Inactive"},
		{ConnectionFailed, "Failed"},
		{ConnectionUnknown, "Unknown"},
	}

	for _, test := range tests {
		if test.status.String() != test.expected {
			t.Errorf("Expected %s for status %d, got %s", test.expected, test.status, test.status.String())
		}
	}
}

func TestConnectionManager_AddDuplicateBackend(t *testing.T) {
	cm := NewConnectionManager(nil)
	defer cm.Stop()

	backend := &Backend{
		ID:       "test",
		Name:     "Test Storage",
		Address:  "localhost:8080",
		Priority: 1,
		Primary:  true,
	}

	// Add backend
	err := cm.AddBackend(backend, func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Errorf("Expected no error adding backend, got %v", err)
	}

	// Try to add same backend again
	err = cm.AddBackend(backend, func(ctx context.Context) error {
		return nil
	})
	if err == nil {
		t.Errorf("Expected error adding duplicate backend")
	}

	if err.Error() != "backend 'test' already exists" {
		t.Errorf("Expected duplicate backend error, got %v", err)
	}
}
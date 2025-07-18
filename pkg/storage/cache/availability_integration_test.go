package cache

import (
	"testing"
	"time"
)

func TestAvailabilityIntegration_Config(t *testing.T) {
	// Test default config
	config := DefaultAvailabilityConfig()
	
	if config.CacheTTL != 5*time.Minute {
		t.Errorf("Expected CacheTTL 5m, got %v", config.CacheTTL)
	}
	
	if config.CheckTimeout != 10*time.Second {
		t.Errorf("Expected CheckTimeout 10s, got %v", config.CheckTimeout)
	}
	
	if config.MaxConcurrentChecks != 10 {
		t.Errorf("Expected MaxConcurrentChecks 10, got %d", config.MaxConcurrentChecks)
	}
	
	if config.MinAvailabilityThreshold != 0.5 {
		t.Errorf("Expected MinAvailabilityThreshold 0.5, got %f", config.MinAvailabilityThreshold)
	}
}

func TestAvailabilityCache_BasicOperations(t *testing.T) {
	cache := NewAvailabilityCache(5 * time.Minute)
	
	// Test empty cache
	status := cache.Get("test-cid")
	if status != nil {
		t.Error("Expected nil from empty cache")
	}
	
	// Test set and get
	testStatus := &AvailabilityStatus{
		CID:         "test-cid",
		Available:   true,
		LastChecked: time.Now(),
	}
	
	cache.Set("test-cid", testStatus)
	
	retrieved := cache.Get("test-cid")
	if retrieved == nil {
		t.Fatal("Expected status from cache")
	}
	
	if retrieved.CID != testStatus.CID {
		t.Errorf("Expected CID %s, got %s", testStatus.CID, retrieved.CID)
	}
	
	if retrieved.Available != testStatus.Available {
		t.Errorf("Expected Available %v, got %v", testStatus.Available, retrieved.Available)
	}
}

func TestAvailabilityCache_TTLExpiration(t *testing.T) {
	cache := NewAvailabilityCache(50 * time.Millisecond)
	
	// Add entry
	testStatus := &AvailabilityStatus{
		CID:         "test-cid",
		Available:   true,
		LastChecked: time.Now(),
	}
	
	cache.Set("test-cid", testStatus)
	
	// Should be available immediately
	retrieved := cache.Get("test-cid")
	if retrieved == nil {
		t.Fatal("Expected status from cache")
	}
	
	// Wait for expiration
	time.Sleep(60 * time.Millisecond)
	
	// Should be expired now
	retrieved = cache.Get("test-cid")
	if retrieved != nil {
		t.Error("Expected nil from expired cache entry")
	}
}

func TestAvailabilityCache_GetAllEntries(t *testing.T) {
	cache := NewAvailabilityCache(5 * time.Minute)
	
	// Add multiple entries
	for i := 0; i < 3; i++ {
		status := &AvailabilityStatus{
			CID:         "test-cid-" + string(rune(i)),
			Available:   true,
			LastChecked: time.Now(),
		}
		cache.Set(status.CID, status)
	}
	
	// Get all entries
	entries := cache.GetAllEntries()
	if len(entries) != 3 {
		t.Errorf("Expected 3 entries, got %d", len(entries))
	}
	
	// Verify all expected CIDs are present
	for i := 0; i < 3; i++ {
		expectedCID := "test-cid-" + string(rune(i))
		if _, exists := entries[expectedCID]; !exists {
			t.Errorf("Expected CID %s not found in entries", expectedCID)
		}
	}
}

func TestAvailabilityCache_Cleanup(t *testing.T) {
	cache := NewAvailabilityCache(50 * time.Millisecond)
	
	// Add entries
	for i := 0; i < 3; i++ {
		status := &AvailabilityStatus{
			CID:         "test-cid-" + string(rune(i)),
			Available:   true,
			LastChecked: time.Now(),
		}
		cache.Set(status.CID, status)
	}
	
	// Verify all entries are there
	entries := cache.GetAllEntries()
	if len(entries) != 3 {
		t.Errorf("Expected 3 entries before cleanup, got %d", len(entries))
	}
	
	// Wait for expiration
	time.Sleep(60 * time.Millisecond)
	
	// Cleanup
	cache.Cleanup()
	
	// Verify entries are cleaned up
	entries = cache.GetAllEntries()
	if len(entries) != 0 {
		t.Errorf("Expected 0 entries after cleanup, got %d", len(entries))
	}
}

func TestAvailabilityStatus_BasicFields(t *testing.T) {
	status := &AvailabilityStatus{
		CID:               "test-cid",
		Available:         true,
		LastChecked:       time.Now(),
		CheckDuration:     100 * time.Millisecond,
		HealthyBackends:   2,
		TotalBackends:     3,
		AvailabilityScore: 0.75,
		Error:             "",
	}
	
	if status.CID != "test-cid" {
		t.Errorf("Expected CID 'test-cid', got %s", status.CID)
	}
	
	if !status.Available {
		t.Error("Expected Available to be true")
	}
	
	if status.AvailabilityScore != 0.75 {
		t.Errorf("Expected AvailabilityScore 0.75, got %f", status.AvailabilityScore)
	}
	
	if status.HealthyBackends != 2 {
		t.Errorf("Expected HealthyBackends 2, got %d", status.HealthyBackends)
	}
	
	if status.TotalBackends != 3 {
		t.Errorf("Expected TotalBackends 3, got %d", status.TotalBackends)
	}
}
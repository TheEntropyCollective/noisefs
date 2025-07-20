package noisefs

import (
	"context"
	"testing"

	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
)

// TODO: Rewrite tests to work with storage.Manager instead of legacy IPFS client
// These tests need to be updated to use mock storage backends

func TestNewClient_Basic(t *testing.T) {
	// Basic test to ensure the package compiles
	cache := cache.NewMemoryCache(10)
	if cache == nil {
		t.Error("Failed to create cache")
	}
}

func TestNewClient_WithMockStorage(t *testing.T) {
	// Create mock storage manager directly to avoid import cycles
	config := storage.DefaultConfig()
	config.Backends = make(map[string]*storage.BackendConfig)

	// Create a simple in-memory backend configuration
	config.Backends["memory"] = &storage.BackendConfig{
		Type:    storage.BackendTypeLocal,
		Enabled: true,
		Connection: &storage.ConnectionConfig{
			Endpoint: "memory://test",
		},
	}
	config.DefaultBackend = "memory"

	manager, err := storage.NewManager(config)
	if err != nil {
		t.Fatalf("Failed to create storage manager: %v", err)
	}

	cache := cache.NewMemoryCache(10)
	client, err := NewClient(manager, cache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	if client == nil {
		t.Error("Client should not be nil")
		return
	}

	// Test that client has required components
	if client.cache == nil {
		t.Error("Client cache should not be nil")
	}

	if client.storageManager == nil {
		t.Error("Client storage manager should not be nil")
	}
}

func TestClient_BasicFunctionality(t *testing.T) {
	// Create basic client for testing core functionality
	config := storage.DefaultConfig()
	config.Backends = make(map[string]*storage.BackendConfig)

	config.Backends["memory"] = &storage.BackendConfig{
		Type:    storage.BackendTypeLocal,
		Enabled: true,
		Connection: &storage.ConnectionConfig{
			Endpoint: "memory://test",
		},
	}
	config.DefaultBackend = "memory"

	manager, err := storage.NewManager(config)
	if err != nil {
		t.Fatalf("Failed to create storage manager: %v", err)
	}
	defer manager.Stop(context.Background())

	// Note: We don't need to start the manager for basic functionality testing

	cache := cache.NewMemoryCache(10)
	client, err := NewClient(manager, cache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Test metrics initialization
	metrics := client.GetMetrics()
	if metrics.TotalUploads != 0 {
		t.Error("Initial upload count should be 0")
	}
	if metrics.TotalDownloads != 0 {
		t.Error("Initial download count should be 0")
	}
}

func TestClient_CacheIntegration(t *testing.T) {
	// Test that client properly integrates with cache
	config := storage.DefaultConfig()
	config.Backends = make(map[string]*storage.BackendConfig)

	config.Backends["memory"] = &storage.BackendConfig{
		Type:    storage.BackendTypeLocal,
		Enabled: true,
		Connection: &storage.ConnectionConfig{
			Endpoint: "memory://test",
		},
	}
	config.DefaultBackend = "memory"

	manager, err := storage.NewManager(config)
	if err != nil {
		t.Fatalf("Failed to create storage manager: %v", err)
	}
	defer manager.Stop(context.Background())

	testCache := cache.NewMemoryCache(10)
	client, err := NewClient(manager, testCache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Verify cache is properly set
	if client.cache != testCache {
		t.Error("Client should use the provided cache instance")
	}

	// Test cache statistics
	stats := testCache.GetStats()
	if stats.Size != 0 {
		t.Error("Cache should start empty")
	}
}

// TODO: Add comprehensive tests for:
// - NewClient with storage manager
// - StoreBlockWithCache functionality
// - RetrieveBlockWithCache functionality
// - Block generation with randomizers
// - Cache integration
// - Error handling

// Package noisefs provides client configuration and initialization functionality.
// This file handles client construction, configuration management, and dependency injection
// for the NoiseFS anonymous file storage system.
package noisefs

import (
	"errors"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
)

// ClientConfig holds configuration for NoiseFS client
type ClientConfig struct {
	EnableAdaptiveCache     bool
	PreferRandomizerPeers   bool
	AdaptiveCacheConfig     *cache.AdaptiveCacheConfig
	DiversityControlsConfig *cache.DiversityControlsConfig
	AvailabilityConfig      *cache.AvailabilityConfig
}

// NewClient creates a new NoiseFS client using storage manager
func NewClient(storageManager *storage.Manager, blockCache cache.Cache) (*Client, error) {
	config := &ClientConfig{
		EnableAdaptiveCache:   true,
		PreferRandomizerPeers: true,
		AdaptiveCacheConfig: &cache.AdaptiveCacheConfig{
			MaxSize:            100 * 1024 * 1024, // 100MB
			MaxItems:           10000,
			HotTierRatio:       0.1, // 10% hot tier
			WarmTierRatio:      0.3, // 30% warm tier
			PredictionWindow:   time.Hour * 24,
			EvictionBatchSize:  10,
			ExchangeInterval:   time.Minute * 15,
			PredictionInterval: time.Minute * 10,
		},
		DiversityControlsConfig: cache.DefaultDiversityControlsConfig(),
		AvailabilityConfig:      cache.DefaultAvailabilityConfig(),
	}

	return NewClientWithConfig(storageManager, blockCache, config)
}

// NewClientWithStorageManager creates a NoiseFS client using the storage abstraction layer
func NewClientWithStorageManager(storageManager *storage.Manager, blockCache cache.Cache) (*Client, error) {
	return NewClient(storageManager, blockCache)
}

// NewClientWithStorageManagerAndConfig creates a NoiseFS client with storage manager and custom config
func NewClientWithStorageManagerAndConfig(storageManager *storage.Manager, blockCache cache.Cache, config *ClientConfig) (*Client, error) {
	return NewClientWithConfig(storageManager, blockCache, config)
}

// NewClientWithConfig creates a new NoiseFS client with custom configuration
func NewClientWithConfig(storageManager *storage.Manager, blockCache cache.Cache, config *ClientConfig) (*Client, error) {
	if storageManager == nil {
		return nil, errors.New("storage manager is required")
	}

	if blockCache == nil {
		return nil, errors.New("cache is required")
	}

	client := &Client{
		storageManager:        storageManager,
		cache:                 blockCache,
		metrics:               NewMetrics(),
		preferRandomizerPeers: config.PreferRandomizerPeers,
		adaptiveCacheEnabled:  config.EnableAdaptiveCache,
	}

	// Initialize adaptive cache if enabled
	if config.EnableAdaptiveCache && config.AdaptiveCacheConfig != nil {
		adaptiveCache := cache.NewAdaptiveCache(config.AdaptiveCacheConfig)
		client.adaptiveCache = adaptiveCache
	}

	// Initialize diversity controls if configured
	if config.DiversityControlsConfig != nil {
		diversityControls := cache.NewRandomizerDiversityControls(config.DiversityControlsConfig)
		client.diversityControls = diversityControls
	}

	// Initialize availability integration if configured
	if config.AvailabilityConfig != nil {
		availabilityIntegration := cache.NewAvailabilityIntegration(storageManager, config.AvailabilityConfig)
		client.availabilityIntegration = availabilityIntegration

		// Set availability integration in metrics for health monitoring
		client.metrics.SetAvailabilityIntegration(availabilityIntegration)
	}

	return client, nil
}

// GetAdaptiveCacheStats returns adaptive cache statistics if enabled
func (c *Client) GetAdaptiveCacheStats() *cache.AdaptiveCacheStatsSnapshot {
	if c.adaptiveCacheEnabled && c.adaptiveCache != nil {
		return c.adaptiveCache.GetAdaptiveStats()
	}
	return nil
}

// GetAltruisticCacheStats returns altruistic cache statistics if available
func (c *Client) GetAltruisticCacheStats() *cache.AltruisticStats {
	// Check if the cache is an altruistic cache
	if altruisticCache, ok := c.cache.(*cache.AltruisticCache); ok {
		return altruisticCache.GetAltruisticStats()
	}
	return nil
}

// IsAltruisticCacheEnabled returns whether altruistic caching is enabled
func (c *Client) IsAltruisticCacheEnabled() bool {
	if altruisticCache, ok := c.cache.(*cache.AltruisticCache); ok {
		return altruisticCache.GetConfig().EnableAltruistic
	}
	return false
}

// GetCacheConfig returns the cache configuration
func (c *Client) GetCacheConfig() *cache.AltruisticCacheConfig {
	if altruisticCache, ok := c.cache.(*cache.AltruisticCache); ok {
		return altruisticCache.GetConfig()
	}
	return nil
}

// GetAvailabilityMetrics returns availability metrics if availability integration is enabled
func (c *Client) GetAvailabilityMetrics() *cache.AvailabilityMetrics {
	if c.availabilityIntegration == nil {
		return nil
	}
	return c.availabilityIntegration.GetAvailabilityMetrics()
}

// GetAvailabilityScore returns the current availability score for health monitoring
func (c *Client) GetAvailabilityScore() float64 {
	if c.availabilityIntegration == nil {
		return 1.0 // Default to good score when not available
	}
	return c.availabilityIntegration.GetAvailabilityScore()
}

// IsAvailabilityIntegrationEnabled returns whether availability integration is enabled
func (c *Client) IsAvailabilityIntegrationEnabled() bool {
	return c.availabilityIntegration != nil
}

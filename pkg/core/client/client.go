// Package noisefs provides the core NoiseFS client structure and interface.
// This file defines the main Client struct that coordinates all NoiseFS operations
// including storage management, caching, metrics, and peer selection.
package noisefs

import (
	"github.com/TheEntropyCollective/noisefs/pkg/privacy/p2p"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
)

// Client provides high-level NoiseFS operations with caching and peer selection
type Client struct {
	// Storage abstraction
	storageManager *storage.Manager

	// Common components
	cache         cache.Cache
	adaptiveCache *cache.AdaptiveCache
	peerManager   *p2p.PeerManager
	metrics       *Metrics

	// Configuration for intelligent operations
	preferRandomizerPeers bool
	adaptiveCacheEnabled  bool

	// Diversity controls for anti-concentration
	diversityControls *cache.RandomizerDiversityControls

	// Availability integration for randomizer availability checking
	availabilityIntegration *cache.AvailabilityIntegration
}

// GetMetrics returns current metrics
func (c *Client) GetMetrics() MetricsSnapshot {
	return c.metrics.GetStats()
}

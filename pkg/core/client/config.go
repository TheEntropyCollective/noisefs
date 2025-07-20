// Package noisefs provides client configuration and initialization functionality.
// This file handles client construction, configuration management, and dependency injection
// for the NoiseFS anonymous file storage system with support for adaptive caching,
// peer management, diversity controls, and availability integration.
//
// The configuration system provides multiple factory functions for different use cases
// and comprehensive configuration options for all client subsystems.
package noisefs

import (
	"errors"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
)

// ClientConfig holds comprehensive configuration options for NoiseFS client initialization.
// This configuration structure defines all client behavior including caching policies,
// peer preferences, diversity controls, availability integration, and encryption settings.
//
// The configuration enables fine-tuning of client behavior for different deployment
// scenarios including development, testing, production, and specialized environments
// with specific performance or security requirements.
//
// Configuration Categories:
//   - Caching: Adaptive cache enablement and detailed cache configuration
//   - Networking: Peer selection preferences and connection strategies
//   - Security: Diversity controls for anti-concentration protection
//   - Reliability: Availability integration for robust operations
//   - Encryption: Default encryption behavior and password provider configuration
//
// Call Flow:
//   - Created by: Application initialization code, configuration loaders
//   - Used by: Client factory functions for initialization
//   - Validates: All configuration options during client creation
//
// Time Complexity: O(1) for configuration access
// Space Complexity: O(1) - fixed configuration structure size
type ClientConfig struct {
	EnableAdaptiveCache     bool                              // Enable intelligent adaptive caching with popularity tracking
	PreferRandomizerPeers   bool                              // Prefer peers that have desired randomizer blocks for efficiency
	AdaptiveCacheConfig     *cache.AdaptiveCacheConfig        // Detailed configuration for adaptive cache behavior
	DiversityControlsConfig *cache.DiversityControlsConfig    // Anti-concentration controls for security
	AvailabilityConfig      *cache.AvailabilityConfig         // Availability checking and fallback configuration
	EncryptionConfig        *EncryptionConfig                 // Default encryption behavior and password provider configuration
}


// NewClient creates a new NoiseFS client with default configuration using storage manager.
// This convenience factory function initializes a client with recommended default settings
// suitable for most use cases, including adaptive caching, peer preferences, and
// comprehensive integration features.
//
// Default Configuration:
//   - Adaptive caching enabled with 100MB cache, 10K items, tiered storage
//   - Randomizer peer preference enabled for optimal performance
//   - Diversity controls configured with default anti-concentration settings
//   - Availability integration enabled with default reliability settings
//   - 24-hour prediction window with 15-minute exchange intervals
//
// This function is recommended for applications that want optimal performance
// without custom configuration requirements.
//
// Parameters:
//   - storageManager: Storage abstraction managing multiple backend implementations
//   - blockCache: Base cache implementation for randomizer block storage
//
// Returns:
//   - *Client: Fully initialized client with default configuration
//   - error: Non-nil if storage manager/cache validation fails or initialization fails
//
// Call Flow:
//   - Called by: Application initialization, simple client setup code
//   - Calls: NewClientWithConfig with default configuration
//
// Time Complexity: O(1) - constant time initialization plus configuration setup
// Space Complexity: O(k) where k is cache size and connection pool sizes
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
		EncryptionConfig:        nil, // No default encryption behavior
	}

	return NewClientWithConfig(storageManager, blockCache, config)
}

// NewClientWithStorageManager creates a NoiseFS client using the storage abstraction layer.
// This factory function is an alias for NewClient, providing an alternative name that
// emphasizes the use of the storage manager abstraction layer for backend management.
//
// The function provides the same default configuration as NewClient, suitable for
// applications that want to emphasize the storage manager pattern in their code.
//
// Parameters:
//   - storageManager: Storage abstraction managing multiple backend implementations
//   - blockCache: Base cache implementation for randomizer block storage
//
// Returns:
//   - *Client: Fully initialized client with default configuration
//   - error: Non-nil if storage manager/cache validation fails or initialization fails
//
// Call Flow:
//   - Called by: Code emphasizing storage manager abstraction pattern
//   - Calls: NewClient for actual implementation
//
// Time Complexity: O(1) - delegates to NewClient
// Space Complexity: O(k) where k is cache size and connection pool sizes
func NewClientWithStorageManager(storageManager *storage.Manager, blockCache cache.Cache) (*Client, error) {
	return NewClient(storageManager, blockCache)
}

// NewClientWithStorageManagerAndConfig creates a NoiseFS client with storage manager and custom configuration.
// This factory function provides an alternative name for NewClientWithConfig that emphasizes
// the storage manager pattern while allowing custom configuration for specialized use cases.
//
// This function is useful for applications that want custom configuration while emphasizing
// the storage manager abstraction in their code structure and naming conventions.
//
// Parameters:
//   - storageManager: Storage abstraction managing multiple backend implementations
//   - blockCache: Base cache implementation for randomizer block storage
//   - config: Custom client configuration specifying cache, networking, and integration settings
//
// Returns:
//   - *Client: Fully initialized client with custom configuration
//   - error: Non-nil if validation fails or initialization fails
//
// Call Flow:
//   - Called by: Code emphasizing storage manager with custom configuration needs
//   - Calls: NewClientWithConfig for actual implementation
//
// Time Complexity: O(1) - delegates to NewClientWithConfig
// Space Complexity: O(k) where k is configured cache size and connection pool sizes
func NewClientWithStorageManagerAndConfig(storageManager *storage.Manager, blockCache cache.Cache, config *ClientConfig) (*Client, error) {
	return NewClientWithConfig(storageManager, blockCache, config)
}

// NewClientWithConfig creates a new NoiseFS client with custom configuration and full validation.
// This is the primary factory function that performs comprehensive initialization of all
// client subsystems with validation, dependency injection, and integration setup.
//
// The function validates all required dependencies and initializes optional subsystems
// based on configuration settings:
//   - Validates storage manager and cache requirements
//   - Initializes metrics collection system
//   - Sets up adaptive caching if enabled and configured
//   - Configures diversity controls for anti-concentration
//   - Integrates availability checking and fallback mechanisms
//   - Establishes metrics integration for health monitoring
//
// Initialization Process:
//   1. Validate required dependencies (storage manager, cache)
//   2. Create base client with core configuration
//   3. Initialize adaptive cache subsystem if enabled
//   4. Set up diversity controls for security
//   5. Configure availability integration for reliability
//   6. Establish metrics integration for monitoring
//
// Parameters:
//   - storageManager: Storage abstraction managing multiple backend implementations (required)
//   - blockCache: Base cache implementation for randomizer block storage (required)
//   - config: Client configuration specifying all subsystem behaviors (required)
//
// Returns:
//   - *Client: Fully initialized and configured client ready for operations
//   - error: Non-nil if validation fails or any subsystem initialization fails
//
// Call Flow:
//   - Called by: All other factory functions, custom initialization code
//   - Calls: cache constructors, metrics initialization, integration setup
//
// Time Complexity: O(1) - constant time initialization plus subsystem setup
// Space Complexity: O(k) where k is configured cache size and connection pool sizes
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
		encryptionConfig:      config.EncryptionConfig,
	}

	// Initialize adaptive cache subsystem if enabled for intelligent caching
	if config.EnableAdaptiveCache && config.AdaptiveCacheConfig != nil {
		adaptiveCache := cache.NewAdaptiveCache(config.AdaptiveCacheConfig)
		client.adaptiveCache = adaptiveCache
	}

	// Initialize diversity controls for anti-concentration security
	if config.DiversityControlsConfig != nil {
		diversityControls := cache.NewRandomizerDiversityControls(config.DiversityControlsConfig)
		client.diversityControls = diversityControls
	}

	// Initialize availability integration for reliability and fallback mechanisms
	if config.AvailabilityConfig != nil {
		availabilityIntegration := cache.NewAvailabilityIntegration(storageManager, config.AvailabilityConfig)
		client.availabilityIntegration = availabilityIntegration

		// Integrate availability monitoring with metrics for comprehensive health tracking
		client.metrics.SetAvailabilityIntegration(availabilityIntegration)
	}

	return client, nil
}

// NewClientWithEncryption creates a new NoiseFS client with default configuration and encryption enabled.
// This convenience factory function initializes a client with encryption enabled by default,
// using the provided password provider for all upload operations that don't explicitly specify encryption.
//
// The client will automatically encrypt all uploads unless explicitly disabled, providing
// a simplified API for applications requiring default encryption behavior.
//
// Parameters:
//   - storageManager: Storage abstraction managing multiple backend implementations
//   - blockCache: Base cache implementation for randomizer block storage
//   - passwordProvider: Provider function for encryption passwords
//
// Returns:
//   - *Client: Fully initialized client with default encryption enabled
//   - error: Non-nil if storage manager/cache validation fails or initialization fails
//
// Call Flow:
//   - Called by: Applications requiring default encryption behavior
//   - Calls: NewClientWithConfig with encryption configuration
//
// Time Complexity: O(1) - constant time initialization plus configuration setup
// Space Complexity: O(k) where k is cache size and connection pool sizes
func NewClientWithEncryption(storageManager *storage.Manager, blockCache cache.Cache, passwordProvider descriptors.PasswordProvider) (*Client, error) {
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
		EncryptionConfig: &EncryptionConfig{
			EnableDefaultEncryption: true,
			DefaultPasswordProvider: passwordProvider,
			RequireEncryption:       false, // Allow fallback to unencrypted if needed
			AllowUnencrypted:        true,  // Backward compatibility
		},
	}

	return NewClientWithConfig(storageManager, blockCache, config)
}

// NewClientWithMandatoryEncryption creates a new NoiseFS client with mandatory encryption policy.
// This factory function initializes a client that enforces encryption for ALL operations,
// preventing any unencrypted uploads and providing enterprise-grade security compliance.
//
// Use this constructor for applications with strict security requirements where
// unencrypted storage is not permitted under any circumstances.
//
// Parameters:
//   - storageManager: Storage abstraction managing multiple backend implementations
//   - blockCache: Base cache implementation for randomizer block storage
//   - passwordProvider: Provider function for encryption passwords
//
// Returns:
//   - *Client: Fully initialized client with mandatory encryption policy
//   - error: Non-nil if storage manager/cache validation fails or initialization fails
//
// Call Flow:
//   - Called by: Enterprise applications with mandatory encryption policies
//   - Calls: NewClientWithConfig with strict encryption configuration
//
// Time Complexity: O(1) - constant time initialization plus configuration setup
// Space Complexity: O(k) where k is cache size and connection pool sizes
func NewClientWithMandatoryEncryption(storageManager *storage.Manager, blockCache cache.Cache, passwordProvider descriptors.PasswordProvider) (*Client, error) {
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
		EncryptionConfig: &EncryptionConfig{
			EnableDefaultEncryption: true,
			DefaultPasswordProvider: passwordProvider,
			RequireEncryption:       true,  // Enforce encryption for all operations
			AllowUnencrypted:        false, // No fallback to unencrypted
		},
	}

	return NewClientWithConfig(storageManager, blockCache, config)
}

// GetAdaptiveCacheStats returns comprehensive adaptive cache statistics if enabled.
// This method provides detailed statistics about adaptive cache performance including
// hit/miss ratios, tier distributions, prediction accuracy, and optimization metrics.
//
// The statistics include:
//   - Cache hit/miss ratios for performance analysis
//   - Hot/warm/cold tier distribution and efficiency
//   - Prediction accuracy and learning effectiveness
//   - Eviction statistics and memory utilization
//   - Exchange intervals and adaptation metrics
//
// Returns nil if adaptive caching is disabled or not configured.
//
// Returns:
//   - *cache.AdaptiveCacheStatsSnapshot: Comprehensive adaptive cache statistics, or nil if disabled
//
// Call Flow:
//   - Called by: Monitoring systems, performance analysis tools, debugging utilities
//   - Calls: adaptiveCache.GetAdaptiveStats for detailed statistics
//
// Time Complexity: O(1) - statistics are pre-aggregated and cached
// Space Complexity: O(1) - returns snapshot copy with fixed size
func (c *Client) GetAdaptiveCacheStats() *cache.AdaptiveCacheStatsSnapshot {
	if c.adaptiveCacheEnabled && c.adaptiveCache != nil {
		return c.adaptiveCache.GetAdaptiveStats()
	}
	return nil
}

// GetAltruisticCacheStats returns altruistic cache statistics if available.
// This method provides statistics about altruistic caching behavior including
// blocks shared with other peers, download assistance provided, and community
// contribution metrics for distributed scenarios.
//
// The method performs type checking to determine if the underlying cache
// implements altruistic caching functionality before attempting to retrieve
// statistics, ensuring safe operation with different cache implementations.
//
// Altruistic statistics include:
//   - Blocks provided to other peers for community benefit
//   - Download assistance and bandwidth sharing metrics
//   - Community contribution ratios and efficiency
//   - Network utilization and peer cooperation statistics
//
// Returns:
//   - *cache.AltruisticStats: Altruistic caching statistics, or nil if not available
//
// Call Flow:
//   - Called by: Monitoring systems, community metrics analysis, peer evaluation
//   - Calls: altruisticCache.GetAltruisticStats for detailed statistics
//
// Time Complexity: O(1) - type check plus pre-aggregated statistics
// Space Complexity: O(1) - returns statistics snapshot
func (c *Client) GetAltruisticCacheStats() *cache.AltruisticStats {
	// Safely check if the cache implements altruistic functionality
	if altruisticCache, ok := c.cache.(*cache.AltruisticCache); ok {
		return altruisticCache.GetAltruisticStats()
	}
	return nil
}

// IsAltruisticCacheEnabled returns whether altruistic caching is enabled and active.
// This method determines if the client is configured to provide altruistic caching
// services, sharing randomizer blocks with other peers for community benefit
// and distributed system efficiency.
//
// The method performs type checking to verify altruistic cache implementation
// and then checks the configuration to determine if altruistic features are
// actively enabled, providing accurate status for monitoring and decision making.
//
// Returns:
//   - bool: True if altruistic caching is enabled and active, false otherwise
//
// Call Flow:
//   - Called by: Configuration validation, feature detection, monitoring systems
//   - Calls: altruisticCache.GetConfig for configuration access
//
// Time Complexity: O(1) - type check plus configuration access
// Space Complexity: O(1) - no memory allocation
func (c *Client) IsAltruisticCacheEnabled() bool {
	if altruisticCache, ok := c.cache.(*cache.AltruisticCache); ok {
		return altruisticCache.GetConfig().EnableAltruistic
	}
	return false
}

// GetCacheConfig returns the current cache configuration if available.
// This method provides access to the complete cache configuration including
// altruistic settings, size limits, behavior policies, and performance tuning
// parameters for analysis and debugging purposes.
//
// The method performs type checking to ensure the cache supports configuration
// access before attempting retrieval, providing safe operation across different
// cache implementations and preventing runtime errors.
//
// Configuration includes:
//   - Altruistic caching enablement and policies
//   - Cache size limits and memory management
//   - Eviction policies and performance tuning
//   - Network sharing and community parameters
//
// Returns:
//   - *cache.AltruisticCacheConfig: Complete cache configuration, or nil if not available
//
// Call Flow:
//   - Called by: Configuration inspection, debugging tools, system analysis
//   - Calls: altruisticCache.GetConfig for configuration access
//
// Time Complexity: O(1) - type check plus configuration access
// Space Complexity: O(1) - returns reference to existing configuration
func (c *Client) GetCacheConfig() *cache.AltruisticCacheConfig {
	if altruisticCache, ok := c.cache.(*cache.AltruisticCache); ok {
		return altruisticCache.GetConfig()
	}
	return nil
}

// GetAvailabilityMetrics returns comprehensive availability metrics if integration is enabled.
// This method provides detailed metrics about randomizer block availability including
// success rates, failure patterns, fallback utilization, and reliability statistics
// for monitoring system health and optimization.
//
// Availability metrics include:
//   - Block retrieval success/failure rates and patterns
//   - Network availability and peer connectivity statistics
//   - Fallback mechanism utilization and effectiveness
//   - Response time distributions and performance trends
//   - Reliability scores and availability predictions
//
// Returns nil if availability integration is not configured or disabled.
//
// Returns:
//   - *cache.AvailabilityMetrics: Comprehensive availability statistics, or nil if disabled
//
// Call Flow:
//   - Called by: Monitoring systems, reliability analysis, health dashboards
//   - Calls: availabilityIntegration.GetAvailabilityMetrics for detailed metrics
//
// Time Complexity: O(1) - metrics are pre-aggregated and cached
// Space Complexity: O(1) - returns metrics snapshot with fixed size
func (c *Client) GetAvailabilityMetrics() *cache.AvailabilityMetrics {
	if c.availabilityIntegration == nil {
		return nil
	}
	return c.availabilityIntegration.GetAvailabilityMetrics()
}

// GetAvailabilityScore returns the current availability score for health monitoring and decision making.
// This method provides a normalized score (0.0 to 1.0) representing overall system
// availability and reliability, useful for health monitoring, alerting, and automatic
// decision making in adaptive systems.
//
// The availability score aggregates multiple factors:
//   - Recent block retrieval success rates
//   - Network connectivity and peer availability
//   - Fallback mechanism effectiveness
//   - Historical reliability trends
//   - Performance consistency metrics
//
// A score of 1.0 indicates optimal availability, while lower scores indicate
// degraded performance requiring attention or automatic adaptation.
//
// Returns:
//   - float64: Availability score from 0.0 (poor) to 1.0 (excellent), defaults to 1.0 if not configured
//
// Call Flow:
//   - Called by: Health monitoring, alerting systems, adaptive algorithms
//   - Calls: availabilityIntegration.GetAvailabilityScore for current score
//
// Time Complexity: O(1) - score is continuously updated and cached
// Space Complexity: O(1) - simple scalar value return
func (c *Client) GetAvailabilityScore() float64 {
	if c.availabilityIntegration == nil {
		return 1.0 // Default to optimal score when availability integration is not configured
	}
	return c.availabilityIntegration.GetAvailabilityScore()
}

// IsAvailabilityIntegrationEnabled returns whether availability integration is enabled and active.
// This method indicates if the client has availability integration configured,
// enabling availability monitoring, fallback mechanisms, and reliability tracking
// for robust operation in distributed environments.
//
// Availability integration provides:
//   - Continuous monitoring of block availability
//   - Automatic fallback mechanisms for failed retrievals
//   - Reliability metrics and health scoring
//   - Predictive availability analysis
//   - Integration with metrics for comprehensive monitoring
//
// Returns:
//   - bool: True if availability integration is configured and active, false otherwise
//
// Call Flow:
//   - Called by: Feature detection, configuration validation, monitoring setup
//   - Checks: availabilityIntegration field for nil status
//
// Time Complexity: O(1) - simple nil check
// Space Complexity: O(1) - no memory allocation
func (c *Client) IsAvailabilityIntegrationEnabled() bool {
	return c.availabilityIntegration != nil
}

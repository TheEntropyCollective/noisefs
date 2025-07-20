// Package noisefs provides the core NoiseFS client structure and interface.
// This file defines the main Client struct that coordinates all NoiseFS operations
// including storage management, caching, metrics, and peer selection for
// privacy-preserving distributed storage with 3-tuple XOR anonymization.
//
// The client serves as the primary entry point for all NoiseFS operations,
// coordinating between storage backends, caching systems, peer networks,
// and metrics collection to provide a unified interface for applications.
package noisefs

import (
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/privacy/p2p"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
)

// EncryptionConfig holds configuration options for client-level encryption behavior.
// This configuration enables default encryption policies, password provider patterns,
// and automated encryption workflows for applications requiring enhanced privacy.
//
// The configuration supports multiple password provider patterns including static passwords,
// interactive prompting, environment variables, and custom callback functions.
//
// Key Features:
//   - Default encryption behavior for all uploads
//   - Flexible password provider patterns (static, callback, environment, interactive)
//   - Policy enforcement for mandatory encryption scenarios
//   - Backward compatibility with explicit encryption calls
//
// Security Considerations:
//   - DefaultPasswordProvider should implement secure memory handling
//   - Static passwords in configuration should be avoided in production
//   - Interactive providers should validate terminal availability
//   - Environment variable providers should validate variable presence
//
// Use Cases:
//   - Enterprise deployments with mandatory encryption policies
//   - Applications requiring automated encryption workflows
//   - Development environments with simplified encryption setup
//   - Interactive applications with user-driven encryption decisions
//
// Time Complexity: O(1) for configuration access
// Space Complexity: O(1) - minimal configuration overhead
type EncryptionConfig struct {
	EnableDefaultEncryption bool                         // Enable encryption by default for all uploads
	DefaultPasswordProvider descriptors.PasswordProvider // Default password provider for automatic encryption
	RequireEncryption       bool                         // Enforce encryption for all operations (policy mode)
	AllowUnencrypted        bool                         // Allow fallback to unencrypted uploads when encryption fails
}

// Client provides high-level NoiseFS operations with integrated caching and peer selection.
// This is the main client structure that coordinates all NoiseFS functionality including
// storage management, randomizer caching, peer networking, and metrics collection.
//
// The client acts as a central coordinator for:
//   - Storage operations through pluggable storage backends
//   - Randomizer caching for optimal storage efficiency
//   - Peer management for distributed operations
//   - Metrics collection for performance monitoring
//   - Diversity controls for anti-concentration attacks
//   - Availability checking for reliable operations
//
// Key Features:
//   - Multi-backend storage abstraction with seamless switching
//   - Adaptive caching with intelligent randomizer selection
//   - Peer-aware operations for distributed scenarios
//   - Comprehensive metrics collection and reporting
//   - Anti-concentration controls for enhanced security
//   - Availability integration for robust operations
//
// Thread Safety:
//   The client is designed to be thread-safe for concurrent operations,
//   with internal synchronization handled by component subsystems.
//
// Call Flow:
//   - Created by: Client factory functions and configuration loaders
//   - Used by: Application code, CLI commands, API handlers
//   - Coordinates: Storage, caching, networking, and metrics subsystems
//
// Time Complexity: Varies by operation (O(1) for cached, O(log n) for storage)
// Space Complexity: O(k) where k is cache size plus connection pools
type Client struct {
	// Storage abstraction providing unified access to multiple storage backends
	// Handles IPFS, local storage, and other backend implementations
	storageManager *storage.Manager

	// Caching components for randomizer block management and performance optimization
	cache         cache.Cache         // Base cache interface for randomizer block storage
	adaptiveCache *cache.AdaptiveCache // Intelligent cache with adaptive policies and popularity tracking
	peerManager   *p2p.PeerManager    // P2P network management for distributed operations
	metrics       *Metrics            // Performance and operational metrics collection

	// Configuration flags controlling intelligent operation modes
	preferRandomizerPeers bool // Enable preference for peers with desired randomizer blocks
	adaptiveCacheEnabled  bool // Enable adaptive caching policies based on usage patterns

	// Security and reliability controls for robust operation
	diversityControls *cache.RandomizerDiversityControls // Anti-concentration controls preventing randomizer clustering

	// Integration components for enhanced reliability and availability
	availabilityIntegration *cache.AvailabilityIntegration // Randomizer availability checking and fallback mechanisms

	// Encryption configuration for default encryption behavior and password provider support
	encryptionConfig        *EncryptionConfig              // Default encryption policies and password provider configuration
}

// GetMetrics returns current performance and operational metrics snapshot.
// This method provides access to comprehensive metrics collected during NoiseFS
// operations, including storage performance, cache efficiency, network statistics,
// and operation counts for monitoring and optimization purposes.
//
// The returned snapshot captures metrics at the time of the call and provides
// a consistent view of system performance across all subsystems. Metrics include:
//   - Storage operation counts and latencies
//   - Cache hit/miss ratios and efficiency statistics
//   - Network operation statistics and peer connectivity
//   - Error rates and failure analysis data
//   - Resource utilization and performance trends
//
// Returns:
//   - MetricsSnapshot: Comprehensive snapshot of current system metrics
//
// Call Flow:
//   - Called by: Monitoring systems, performance analysis tools, debugging utilities
//   - Calls: metrics.GetStats for aggregated statistics collection
//
// Time Complexity: O(1) - metrics are pre-aggregated and cached
// Space Complexity: O(1) - returns snapshot copy with fixed size
func (c *Client) GetMetrics() MetricsSnapshot {
	return c.metrics.GetStats()
}

// GetEncryptionConfig returns the current encryption configuration if set.
// This method provides access to the client's encryption policy settings including
// default encryption behavior, password provider configuration, and policy enforcement
// settings for monitoring and configuration validation purposes.
//
// The returned configuration includes:
//   - Default encryption enablement status
//   - Password provider function reference
//   - Encryption policy enforcement settings
//   - Fallback behavior configuration
//
// Returns nil if no encryption configuration is set (default behavior).
//
// Returns:
//   - *EncryptionConfig: Current encryption configuration, or nil if not configured
//
// Call Flow:
//   - Called by: Configuration validation, policy checking, administrative tools
//   - Returns: Direct reference to internal encryption configuration
//
// Time Complexity: O(1) - simple field access
// Space Complexity: O(1) - returns reference to existing configuration
func (c *Client) GetEncryptionConfig() *EncryptionConfig {
	return c.encryptionConfig
}

// IsDefaultEncryptionEnabled returns whether default encryption is enabled for this client.
// This method provides a convenient way to check if the client will automatically
// encrypt uploads by default, useful for applications that need to understand
// the client's encryption behavior without accessing the full configuration.
//
// Returns:
//   - bool: True if default encryption is enabled, false otherwise
//
// Call Flow:
//   - Called by: Upload logic, configuration validation, policy checking
//   - Checks: encryptionConfig field and EnableDefaultEncryption setting
//
// Time Complexity: O(1) - simple field access with nil check
// Space Complexity: O(1) - no memory allocation
func (c *Client) IsDefaultEncryptionEnabled() bool {
	return c.encryptionConfig != nil && c.encryptionConfig.EnableDefaultEncryption
}

// IsEncryptionRequired returns whether encryption is mandatorily required for this client.
// This method indicates if the client enforces encryption policies that prevent
// any unencrypted uploads, useful for compliance validation and policy enforcement.
//
// Returns:
//   - bool: True if encryption is required for all operations, false otherwise
//
// Call Flow:
//   - Called by: Upload validation, policy enforcement, compliance checking
//   - Checks: encryptionConfig field and RequireEncryption setting
//
// Time Complexity: O(1) - simple field access with nil check
// Space Complexity: O(1) - no memory allocation
func (c *Client) IsEncryptionRequired() bool {
	return c.encryptionConfig != nil && c.encryptionConfig.RequireEncryption
}

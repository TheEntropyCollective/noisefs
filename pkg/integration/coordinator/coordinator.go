package integration

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/compliance"
	"github.com/TheEntropyCollective/noisefs/pkg/common/config"
	"github.com/TheEntropyCollective/noisefs/pkg/common/logging"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
	client "github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/TheEntropyCollective/noisefs/pkg/privacy/p2p"
	"github.com/TheEntropyCollective/noisefs/pkg/privacy/relay"
	"github.com/TheEntropyCollective/noisefs/pkg/privacy/reuse"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

// SystemCoordinator orchestrates all NoiseFS components
type SystemCoordinator struct {
	// Core components
	config           *config.Config
	storageManager   *storage.Manager
	// blockManager is not needed - blocks are managed directly
	noisefsClient    *client.Client
	
	// P2P components
	libp2pHost       host.Host
	
	// Privacy components
	relayPool        *relay.RelayPool
	coverTraffic     *relay.CoverTrafficGenerator
	requestMixer     *relay.RequestMixer
	peerManager      *p2p.PeerManager
	
	// Reuse components
	reuseClient      *reuse.ReuseAwareClient
	universalPool    *reuse.UniversalBlockPool
	reuseEnforcer    *reuse.ReuseEnforcer
	publicMixer      *reuse.PublicDomainMixer
	
	// Legal components
	complianceAudit  *compliance.ComplianceAuditSystem
	
	// Cache components
	blockCache       cache.Cache
	adaptiveCache    *cache.AdaptiveCache
	
	// Descriptor storage
	descriptorStore  *descriptors.Store
	
	// Metrics
	systemMetrics    *SystemMetrics
	
	// Logging
	logger           *logging.Logger
	
	mu               sync.RWMutex
}

// SystemMetrics tracks overall system performance
type SystemMetrics struct {
	TotalUploads      int64
	TotalDownloads    int64
	TotalBlocks       int64
	ReuseRatio        float64
	CoverTrafficRatio float64
	StorageEfficiency float64
	PrivacyScore      float64
}

// NewSystemCoordinator creates a new system coordinator with all components
func NewSystemCoordinator(cfg *config.Config) (*SystemCoordinator, error) {
	coordinator := &SystemCoordinator{
		config:        cfg,
		systemMetrics: &SystemMetrics{},
		logger:        logging.GetGlobalLogger().WithComponent("coordinator"),
	}
	
	// Initialize components in dependency order
	if err := coordinator.initializeStorage(); err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}
	
	if err := coordinator.initializeCache(); err != nil {
		return nil, fmt.Errorf("failed to initialize cache: %w", err)
	}
	
	if err := coordinator.initializeP2P(); err != nil {
		return nil, fmt.Errorf("failed to initialize P2P: %w", err)
	}
	
	if err := coordinator.initializePrivacy(); err != nil {
		return nil, fmt.Errorf("failed to initialize privacy: %w", err)
	}
	
	if err := coordinator.initializeReuse(); err != nil {
		return nil, fmt.Errorf("failed to initialize reuse: %w", err)
	}
	
	if err := coordinator.initializeCompliance(); err != nil {
		return nil, fmt.Errorf("failed to initialize compliance: %w", err)
	}
	
	if err := coordinator.initializeCore(); err != nil {
		return nil, fmt.Errorf("failed to initialize core: %w", err)
	}
	
	// Wire components together
	if err := coordinator.wireComponents(); err != nil {
		return nil, fmt.Errorf("failed to wire components: %w", err)
	}
	
	return coordinator, nil
}

// initializeStorage sets up the storage manager
func (sc *SystemCoordinator) initializeStorage() error {
	// Create storage manager with IPFS backend
	storageConfig := storage.DefaultConfig()
	if ipfsBackend, exists := storageConfig.Backends["ipfs"]; exists {
		ipfsBackend.Connection.Endpoint = sc.config.IPFS.APIEndpoint
	}
	
	manager, err := storage.NewManager(storageConfig)
	if err != nil {
		return err
	}
	sc.storageManager = manager
	return nil
}

// initializeCache sets up the caching layer
func (sc *SystemCoordinator) initializeCache() error {
	// Create adaptive cache if enabled in configuration
	if sc.config.Cache.EnableAdaptiveCache {
		adaptiveCfg := &cache.AdaptiveCacheConfig{
			MaxSize:            int64(sc.config.Cache.MemoryLimit) * 1024 * 1024, // Convert MB to bytes
			MaxItems:           sc.config.Cache.BlockCacheSize,
			HotTierRatio:       0.1,
			WarmTierRatio:      0.3,
			PredictionWindow:   time.Hour * 24,
			EvictionBatchSize:  10,
			ExchangeInterval:   time.Minute * 15,
			PredictionInterval: time.Minute * 10,
		}
		sc.adaptiveCache = cache.NewAdaptiveCache(adaptiveCfg)
		
		// Wrap with altruistic cache if enabled
		if sc.config.Cache.EnableAltruistic {
			altruisticConfig := &cache.AltruisticCacheConfig{
				MinPersonalCache: int64(sc.config.Cache.MinPersonalCacheMB) * 1024 * 1024,
				EnableAltruistic: true,
				EvictionCooldown: 5 * time.Minute,
			}
			
			// Use adaptive cache as the base
			sc.blockCache = cache.NewAltruisticCache(
				sc.adaptiveCache,
				altruisticConfig,
				int64(sc.config.Cache.MemoryLimit) * 1024 * 1024,
			)
		} else {
			// Use adaptive cache directly
			sc.blockCache = sc.adaptiveCache
		}
	} else {
		// Create simple memory cache
		sc.blockCache = cache.NewMemoryCache(sc.config.Cache.BlockCacheSize)
	}
	
	return nil
}

// initializeP2P sets up the libp2p host and peer management
func (sc *SystemCoordinator) initializeP2P() error {
	// Create libp2p host with reasonable defaults
	host, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"), // Listen on random port
		libp2p.Ping(false),  // Disable ping service for privacy
		libp2p.DisableRelay(), // Disable relay for now
	)
	if err != nil {
		return fmt.Errorf("failed to create libp2p host: %w", err)
	}
	
	sc.libp2pHost = host
	
	// Create peer manager with the host
	maxPeers := 50  // Default to 50 peers for reasonable network size
	
	sc.peerManager = p2p.NewPeerManager(host, maxPeers)
	
	logging.GetGlobalLogger().Info("P2P host initialized", map[string]interface{}{
		"peer_id":   host.ID().String(),
		"addresses": host.Addrs(),
	})
	
	return nil
}

// initializePrivacy sets up privacy components
func (sc *SystemCoordinator) initializePrivacy() error {
	// Peer manager is now initialized in initializeP2P()
	
	// Create relay pool with default config
	poolConfig := &relay.PoolConfig{
		MaxRelays:           10,
		MinRelays:           3,
		HealthCheckInterval: time.Minute * 5,
		MaxRelayAge:         time.Hour * 24,
		LoadBalanceStrategy: "random",
		PrivacyLevel:        3, // 3 hops for high privacy
	}
	sc.relayPool = relay.NewRelayPool(poolConfig)
	
	// Create connection pool with config
	connPoolConfig := &relay.ConnectionPoolConfig{
		MaxConnections:      100,
		MaxIdleTime:         time.Minute * 10,
		ConnectionTimeout:   time.Second * 30,
		KeepAliveInterval:   time.Minute,
		ReconnectAttempts:   3,
		ReconnectDelay:      time.Second * 5,
		MaxRequestsPerConn:  10,
	}
	connectionPool := relay.NewConnectionPool(connPoolConfig)
	
	// Create popularity tracker with cache and config
	popConfig := &relay.PopularityConfig{
		RefreshInterval:     time.Minute,
		MinAccessCount:      5,
		PopularityThreshold: 0.7,
		MaxPopularBlocks:    1000,
		DecayFactor:         0.95,
		CategoryWeights:     map[string]float64{"default": 1.0},
	}
	popularityTracker := relay.NewPopularBlockTracker(sc.blockCache, popConfig)
	
	// Create cover traffic generator with defaults
	coverConfig := &relay.CoverTrafficConfig{
		NoiseRatio:         0.3,
		MinCoverRequests:   5,
		MaxCoverRequests:   20,
		CoverInterval:      time.Second * 30,
		RandomDelay:        time.Second * 5,
		BandwidthLimit:     1024 * 1024, // 1MB/s
		PopularityBias:     0.7,
		BatchSize:          10,
		MaxConcurrent:      50,
	}
	sc.coverTraffic = relay.NewCoverTrafficGenerator(coverConfig, popularityTracker, sc.relayPool, connectionPool)
	
	// Create request distributor with default config
	distConfig := &relay.DistributorConfig{
		MaxConcurrentRequests: 50,
		RequestTimeout:        time.Second * 30,
		RetryAttempts:         3,
		LoadBalanceStrategy:   "round_robin",
		FailoverEnabled:       true,
	}
	distributor := relay.NewRequestDistributor(sc.relayPool, distConfig)
	
	// Create request mixer with defaults
	mixerConfig := &relay.MixerConfig{
		MixingDelay:       time.Millisecond * 500,
		MinMixSize:        5,
		MaxMixSize:        20,
		CoverRatio:        0.3,
		RelayDistribution: 0.8,
		TemporalJitter:    time.Millisecond * 100,
		PriorityMixing:    true,
		BatchTimeout:      time.Second * 2,
	}
	sc.requestMixer = relay.NewRequestMixer(mixerConfig, sc.coverTraffic, popularityTracker, distributor)
	
	return nil
}

// initializeReuse sets up the reuse system
func (sc *SystemCoordinator) initializeReuse() error {
	// Create universal block pool with defaults
	poolConfig := reuse.DefaultPoolConfig()
	poolConfig.PublicDomainRatio = 0.3
	poolConfig.MinReuseCount = 3
	sc.universalPool = reuse.NewUniversalBlockPool(poolConfig, sc.storageManager)
	
	// Initialize the pool
	if err := sc.universalPool.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize universal pool: %w", err)
	}
	
	// Create reuse enforcer with defaults
	reusePolicy := reuse.DefaultReusePolicy()
	// Note: ReusePolicy fields might be different
	sc.reuseEnforcer = reuse.NewReuseEnforcer(sc.universalPool, reusePolicy)
	
	// Create public domain mixer with defaults
	mixerConfig := reuse.DefaultMixerConfig()
	mixerConfig.MinPublicDomainRatio = 0.3
	sc.publicMixer = reuse.NewPublicDomainMixer(sc.universalPool, mixerConfig, sc.storageManager)
	
	// Create reuse-aware client
	reuseClient, err := reuse.NewReuseAwareClient(sc.storageManager, sc.blockCache)
	if err != nil {
		return fmt.Errorf("failed to create reuse client: %w", err)
	}
	sc.reuseClient = reuseClient
	
	return nil
}

// initializeCompliance sets up legal compliance components
func (sc *SystemCoordinator) initializeCompliance() error {
	auditConfig := compliance.DefaultAuditConfig()
	// Use default database path and retention period
	
	sc.complianceAudit = compliance.NewComplianceAuditSystem(auditConfig)
	
	// Compliance audit system is ready to use
	
	return nil
}

// initializeCore sets up core NoiseFS components
func (sc *SystemCoordinator) initializeCore() error {
	// Block management is handled by the blocks package directly
	
	// Create NoiseFS client
	noisefsClient, err := client.NewClient(sc.storageManager, sc.blockCache)
	if err != nil {
		return fmt.Errorf("failed to create NoiseFS client: %w", err)
	}
	sc.noisefsClient = noisefsClient
	
	// Create descriptor store
	sc.descriptorStore, err = descriptors.NewStore(sc.storageManager)
	if err != nil {
		return fmt.Errorf("failed to create descriptor store: %w", err)
	}
	
	return nil
}

// wireComponents connects all components together
func (sc *SystemCoordinator) wireComponents() error {
	// Validate that critical privacy components are initialized
	if sc.peerManager == nil {
		return fmt.Errorf("peer manager not initialized - privacy features unavailable")
	}
	if sc.libp2pHost == nil {
		return fmt.Errorf("libp2p host not initialized - P2P features unavailable")
	}
	
	// Start privacy components
	ctx := context.Background()
	if err := sc.relayPool.Start(ctx); err != nil {
		return fmt.Errorf("failed to start relay pool: %w", err)
	}
	
	
	// Log privacy status for transparency
	sc.logPrivacyStatus()
	
	return nil
}

// logPrivacyStatus provides clear information about privacy features
func (sc *SystemCoordinator) logPrivacyStatus() {
	logger := logging.GetGlobalLogger().WithComponent("coordinator")
	
	// Check P2P components
	if sc.libp2pHost != nil && sc.peerManager != nil {
		peerStats := sc.peerManager.GetStats()
		logger.Info("P2P privacy layer active", map[string]interface{}{
			"peer_id":         sc.libp2pHost.ID().String(),
			"connected_peers": peerStats["connected_peers"],
			"healthy_peers":   peerStats["healthy_peers"],
		})
	} else {
		logger.Warn("P2P privacy layer inactive - reduced anonymity", map[string]interface{}{})
	}
	
	// Check relay components
	if sc.relayPool != nil && sc.requestMixer != nil {
		logger.Info("Relay privacy layer active", map[string]interface{}{
			"relay_pool_initialized":    true,
			"request_mixer_initialized": true,
		})
	} else {
		logger.Warn("Relay privacy layer inactive - no request mixing", map[string]interface{}{})
	}
	
	// Check cover traffic
	if sc.coverTraffic != nil {
		coverStats := sc.coverTraffic.GetMetrics()
		logger.Info("Cover traffic active", map[string]interface{}{
			"total_cover_requests": coverStats.TotalCoverRequests,
			"noise_ratio":          coverStats.NoiseRatioAchieved,
		})
	} else {
		logger.Warn("Cover traffic inactive - traffic analysis possible", map[string]interface{}{})
	}
	
	// Check reuse components
	if sc.universalPool != nil && sc.reuseEnforcer != nil {
		logger.Info("Block reuse privacy active", map[string]interface{}{
			"universal_pool_initialized":  true,
			"reuse_enforcer_initialized": true,
		})
	} else {
		logger.Warn("Block reuse privacy inactive - reduced plausible deniability", map[string]interface{}{})
	}
}

// GetPrivacyStatus returns detailed privacy feature status
func (sc *SystemCoordinator) GetPrivacyStatus() map[string]interface{} {
	status := map[string]interface{}{
		"p2p_enabled":           sc.libp2pHost != nil && sc.peerManager != nil,
		"relay_enabled":         sc.relayPool != nil && sc.requestMixer != nil,
		"cover_traffic_enabled": sc.coverTraffic != nil,
		"block_reuse_enabled":   sc.universalPool != nil && sc.reuseEnforcer != nil,
		"overall_privacy_score": 0.0,
	}
	
	// Calculate overall privacy score
	score := 0.0
	if status["p2p_enabled"].(bool) {
		score += 0.3
	}
	if status["relay_enabled"].(bool) {
		score += 0.3
	}
	if status["cover_traffic_enabled"].(bool) {
		score += 0.2
	}
	if status["block_reuse_enabled"].(bool) {
		score += 0.2
	}
	
	status["overall_privacy_score"] = score
	
	// Add detailed stats if available
	if sc.peerManager != nil {
		status["peer_stats"] = sc.peerManager.GetStats()
	}
	
	if sc.coverTraffic != nil {
		status["cover_traffic_stats"] = sc.coverTraffic.GetMetrics()
	}
	
	return status
}

// UploadFile performs a complete file upload with all protections
func (sc *SystemCoordinator) UploadFile(reader io.Reader, filename string) (string, error) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	
	// Update metrics
	sc.systemMetrics.TotalUploads++
	
	// Use reuse-aware client for upload with default block size
	result, err := sc.reuseClient.UploadFile(reader, filename, blocks.DefaultBlockSize)
	if err != nil {
		return "", fmt.Errorf("upload failed: %w", err)
	}
	
	// Log compliance event
	err = sc.complianceAudit.LogComplianceEvent(
		"file_upload",
		"system",
		result.DescriptorCID,
		"upload_completed",
		map[string]interface{}{
			"filename":     filename,
			"reuse_proof":  result.ReuseProof,
			"mixing_plan":  result.MixingPlan,
			"privacy_score": sc.calculatePrivacyScore(result),
		},
	)
	if err != nil {
		// Log compliance logging failure securely without exposing sensitive data
		sc.logger.Warn("Failed to log compliance event", map[string]interface{}{
			"operation": "compliance_audit_logging",
			"status":    "failed",
			"context":   "file_upload_completion",
		})
	}
	
	// Update system metrics
	sc.updateMetricsFromUpload(result)
	
	return result.DescriptorCID, nil
}

// DownloadFile performs a complete file download with privacy
func (sc *SystemCoordinator) DownloadFile(descriptorCID string) (io.Reader, error) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	
	// Update metrics
	sc.systemMetrics.TotalDownloads++
	
	// Mix download request with cover traffic
	ctx := context.Background()
	mixedResult, err := sc.requestMixer.SubmitRequest(ctx, descriptorCID, 1)
	if err != nil {
		// Fallback to direct download if mixing fails
		data, err := sc.reuseClient.DownloadFile(descriptorCID)
		if err != nil {
			return nil, err
		}
		return bytes.NewReader(data), nil
	}
	
	// Use mixed result for download
	if mixedResult.Success && mixedResult.Data != nil {
		// Parse descriptor and download file
		data, err := sc.reuseClient.DownloadFile(descriptorCID)
		if err != nil {
			return nil, err
		}
		return bytes.NewReader(data), nil
	}
	
	return nil, fmt.Errorf("download failed")
}

// GetSystemMetrics returns current system metrics
func (sc *SystemCoordinator) GetSystemMetrics() *SystemMetrics {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	
	// Calculate current metrics
	metrics := *sc.systemMetrics
	
	// Get reuse statistics
	if sc.reuseClient != nil {
		reuseStats := sc.reuseClient.GetReuseStatistics()
		if poolStats, ok := reuseStats["pool"].(map[string]interface{}); ok {
			if reuseRatio, ok := poolStats["average_reuse_count"].(float64); ok {
				metrics.ReuseRatio = reuseRatio
			}
		}
	}
	
	// Get cover traffic statistics
	if sc.coverTraffic != nil {
		coverStats := sc.coverTraffic.GetMetrics()
		if coverStats.TotalCoverRequests > 0 {
			metrics.CoverTrafficRatio = coverStats.NoiseRatioAchieved
		}
	}
	
	// Calculate storage efficiency
	if sc.noisefsClient != nil {
		// Storage efficiency would be calculated from actual metrics
		// For now, use a placeholder
		metrics.StorageEfficiency = 0.85
	}
	
	return &metrics
}

// Shutdown gracefully shuts down all components
func (sc *SystemCoordinator) Shutdown() error {
	logger := sc.logger.WithComponent("shutdown")
	var shutdownErrors []error
	
	logger.Info("Starting system shutdown", map[string]interface{}{})
	
	// Stop P2P components first
	if sc.peerManager != nil {
		if err := sc.peerManager.Close(); err != nil {
			shutdownErrors = append(shutdownErrors, fmt.Errorf("failed to close peer manager: %w", err))
			logger.Error("failed to close peer manager", map[string]interface{}{
				"error": err,
			})
		} else {
			logger.Info("peer manager closed successfully", map[string]interface{}{})
		}
	}
	
	if sc.libp2pHost != nil {
		if err := sc.libp2pHost.Close(); err != nil {
			shutdownErrors = append(shutdownErrors, fmt.Errorf("failed to close libp2p host: %w", err))
			logger.Error("failed to close libp2p host", map[string]interface{}{
				"error": err,
			})
		} else {
			logger.Info("libp2p host closed successfully", map[string]interface{}{})
		}
	}
	
	// Stop privacy components
	if sc.relayPool != nil {
		sc.relayPool.Stop()
		logger.Info("relay pool stopped", map[string]interface{}{})
	}
	
	if sc.coverTraffic != nil {
		sc.coverTraffic.Stop()
		logger.Info("cover traffic stopped", map[string]interface{}{})
	}
	
	if sc.requestMixer != nil {
		sc.requestMixer.Stop()
		logger.Info("request mixer stopped", map[string]interface{}{})
	}
	
	// Stop cache components
	if sc.adaptiveCache != nil {
		// AdaptiveCache doesn't have a cleanup method, but clear it for safety
		sc.adaptiveCache.Clear()
		logger.Info("adaptive cache cleared", map[string]interface{}{})
	}
	
	if sc.blockCache != nil {
		// Clear the cache to free memory
		sc.blockCache.Clear()
		logger.Info("block cache cleared", map[string]interface{}{})
	}
	
	// Stop reuse components
	// Note: UniversalBlockPool doesn't have a Stop() method yet
	// The reuse components are mostly stateless and don't require explicit cleanup
	logger.Info("reuse components cleanup completed", map[string]interface{}{})
	
	// Stop storage manager with timeout
	if sc.storageManager != nil {
		// Create context with timeout for storage manager shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		
		if err := sc.storageManager.Stop(ctx); err != nil {
			shutdownErrors = append(shutdownErrors, fmt.Errorf("failed to stop storage manager: %w", err))
			logger.Error("failed to stop storage manager", map[string]interface{}{
				"error": err,
				"timeout": "30s",
			})
		} else {
			logger.Info("storage manager stopped successfully", map[string]interface{}{
				"timeout": "30s",
			})
		}
	}
	
	// Stop compliance audit system
	if sc.complianceAudit != nil {
		// ComplianceAuditSystem doesn't have a cleanup method in the current implementation
		// but we log that it's being shut down for tracking
		logger.Info("compliance audit system shutdown", map[string]interface{}{})
	}
	
	// Final cleanup and state reset
	sc.mu.Lock()
	sc.storageManager = nil
	sc.blockCache = nil
	sc.adaptiveCache = nil
	sc.noisefsClient = nil
	sc.descriptorStore = nil
	sc.libp2pHost = nil
	sc.peerManager = nil
	sc.relayPool = nil
	sc.coverTraffic = nil
	sc.requestMixer = nil
	sc.reuseClient = nil
	sc.universalPool = nil
	sc.reuseEnforcer = nil
	sc.publicMixer = nil
	sc.complianceAudit = nil
	sc.mu.Unlock()
	
	if len(shutdownErrors) > 0 {
		logger.Error("shutdown completed with errors", map[string]interface{}{
			"error_count": len(shutdownErrors),
		})
		
		// Return the first error but log all
		for i, err := range shutdownErrors {
			logger.Error("shutdown error", map[string]interface{}{
				"error_index": i,
				"error": err.Error(),
			})
		}
		return shutdownErrors[0]
	}
	
	logger.Info("system shutdown completed successfully", map[string]interface{}{})
	return nil
}

// Helper methods

func (sc *SystemCoordinator) calculatePrivacyScore(result *reuse.UploadResult) float64 {
	score := 0.7 // Base score
	
	// Add points for reuse
	if result.ReuseProof != nil {
		score += 0.1
	}
	
	// Add points for public domain mixing
	if result.MixingPlan != nil && result.MixingPlan.PublicDomainBlocks > 0 {
		score += 0.1
	}
	
	// Add points for cover traffic
	if sc.coverTraffic != nil && sc.coverTraffic.GetMetrics().TotalCoverRequests > 0 {
		score += 0.1
	}
	
	return score
}

func (sc *SystemCoordinator) updateMetricsFromUpload(result *reuse.UploadResult) {
	if result.MixingPlan != nil {
		sc.systemMetrics.TotalBlocks += int64(result.MixingPlan.TotalBlocks)
	}
	
	sc.systemMetrics.PrivacyScore = sc.calculatePrivacyScore(result)
}
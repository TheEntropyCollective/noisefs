package integration

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/TheEntropyCollective/noisefs/pkg/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/cache"
	"github.com/TheEntropyCollective/noisefs/pkg/compliance"
	"github.com/TheEntropyCollective/noisefs/pkg/config"
	"github.com/TheEntropyCollective/noisefs/pkg/descriptors"
	"github.com/TheEntropyCollective/noisefs/pkg/ipfs"
	"github.com/TheEntropyCollective/noisefs/pkg/noisefs"
	"github.com/TheEntropyCollective/noisefs/pkg/p2p"
	"github.com/TheEntropyCollective/noisefs/pkg/relay"
	"github.com/TheEntropyCollective/noisefs/pkg/reuse"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

// SystemCoordinator orchestrates all NoiseFS components
type SystemCoordinator struct {
	// Core components
	config           *config.Config
	ipfsClient       *ipfs.Client
	storageManager   *storage.Manager
	blockManager     *blocks.BlockManager
	noisefsClient    *noisefs.Client
	
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
	}
	
	// Initialize components in dependency order
	if err := coordinator.initializeIPFS(); err != nil {
		return nil, fmt.Errorf("failed to initialize IPFS: %w", err)
	}
	
	if err := coordinator.initializeStorage(); err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}
	
	if err := coordinator.initializeCache(); err != nil {
		return nil, fmt.Errorf("failed to initialize cache: %w", err)
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

// initializeIPFS sets up the IPFS client
func (sc *SystemCoordinator) initializeIPFS() error {
	ipfsClient, err := ipfs.NewClient(sc.config.IPFS.URL)
	if err != nil {
		return err
	}
	sc.ipfsClient = ipfsClient
	return nil
}

// initializeStorage sets up the storage manager
func (sc *SystemCoordinator) initializeStorage() error {
	// Create storage manager with IPFS backend
	storageConfig := &storage.Config{
		Backends: []storage.BackendConfig{
			{
				Name:   "ipfs",
				Type:   "ipfs",
				Config: map[string]interface{}{"client": sc.ipfsClient},
			},
		},
		DefaultBackend: "ipfs",
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
	// Create main block cache
	sc.blockCache = cache.NewMemoryCache(sc.config.Cache.MaxBlocks)
	
	// Create adaptive cache if enabled
	if sc.config.Cache.EnableAdaptive {
		adaptiveCfg := &cache.AdaptiveCacheConfig{
			MaxSize:            sc.config.Cache.MaxSize,
			MaxItems:           sc.config.Cache.MaxBlocks,
			HotTierRatio:       0.1,
			WarmTierRatio:      0.3,
			PredictionWindow:   sc.config.Cache.PredictionWindow,
			EvictionBatchSize:  10,
			ExchangeInterval:   sc.config.Cache.ExchangeInterval,
			PredictionInterval: sc.config.Cache.PredictionInterval,
		}
		sc.adaptiveCache = cache.NewAdaptiveCache(adaptiveCfg)
	}
	
	return nil
}

// initializePrivacy sets up privacy components
func (sc *SystemCoordinator) initializePrivacy() error {
	// Create peer manager
	sc.peerManager = p2p.NewPeerManager()
	
	// Create relay pool
	poolConfig := &relay.PoolConfig{
		MaxRelays:           sc.config.Privacy.MaxRelays,
		MinRelays:           sc.config.Privacy.MinRelays,
		HealthCheckInterval: sc.config.Privacy.HealthCheckInterval,
		MaxRelayAge:         sc.config.Privacy.MaxRelayAge,
		LoadBalanceStrategy: "random",
		PrivacyLevel:        sc.config.Privacy.PrivacyLevel,
	}
	sc.relayPool = relay.NewRelayPool(poolConfig)
	
	// Create connection pool
	connectionPool := relay.NewConnectionPool(sc.config.Privacy.MaxConnections)
	
	// Create popularity tracker
	popularityTracker := relay.NewPopularBlockTracker(sc.config.Privacy.PopularityWindow)
	
	// Create cover traffic generator
	coverConfig := &relay.CoverTrafficConfig{
		NoiseRatio:         sc.config.Privacy.CoverTrafficRatio,
		MinCoverRequests:   sc.config.Privacy.MinCoverRequests,
		MaxCoverRequests:   sc.config.Privacy.MaxCoverRequests,
		CoverInterval:      sc.config.Privacy.CoverInterval,
		RandomDelay:        sc.config.Privacy.RandomDelay,
		BandwidthLimit:     sc.config.Privacy.BandwidthLimit,
		PopularityBias:     0.7,
		BatchSize:          10,
		MaxConcurrent:      sc.config.Privacy.MaxConcurrent,
	}
	sc.coverTraffic = relay.NewCoverTrafficGenerator(coverConfig, popularityTracker, sc.relayPool, connectionPool)
	
	// Create request distributor
	distributor := relay.NewRequestDistributor(sc.relayPool, connectionPool, relay.DefaultDistributorConfig())
	
	// Create request mixer
	mixerConfig := &relay.MixerConfig{
		MixingDelay:       sc.config.Privacy.MixingDelay,
		MinMixSize:        sc.config.Privacy.MinMixSize,
		MaxMixSize:        sc.config.Privacy.MaxMixSize,
		CoverRatio:        sc.config.Privacy.CoverTrafficRatio,
		RelayDistribution: 0.8,
		TemporalJitter:    sc.config.Privacy.TemporalJitter,
		PriorityMixing:    true,
		BatchTimeout:      sc.config.Privacy.BatchTimeout,
	}
	sc.requestMixer = relay.NewRequestMixer(mixerConfig, sc.coverTraffic, popularityTracker, distributor)
	
	return nil
}

// initializeReuse sets up the reuse system
func (sc *SystemCoordinator) initializeReuse() error {
	// Create universal block pool
	poolConfig := reuse.DefaultPoolConfig()
	poolConfig.PublicDomainRatio = sc.config.Reuse.PublicDomainRatio
	poolConfig.MinReuseCount = sc.config.Reuse.MinReuseCount
	sc.universalPool = reuse.NewUniversalBlockPool(poolConfig, sc.ipfsClient)
	
	// Initialize the pool
	if err := sc.universalPool.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize universal pool: %w", err)
	}
	
	// Create reuse enforcer
	reusePolicy := reuse.DefaultReusePolicy()
	reusePolicy.MinReuseCount = sc.config.Reuse.MinReuseCount
	reusePolicy.RequirePublicDomain = sc.config.Reuse.RequirePublicDomain
	sc.reuseEnforcer = reuse.NewReuseEnforcer(sc.universalPool, reusePolicy)
	
	// Create public domain mixer
	mixerConfig := reuse.DefaultMixerConfig()
	mixerConfig.MinPublicDomainRatio = sc.config.Reuse.PublicDomainRatio
	sc.publicMixer = reuse.NewPublicDomainMixer(sc.universalPool, mixerConfig)
	
	// Create reuse-aware client
	reuseClient, err := reuse.NewReuseAwareClient(sc.ipfsClient, sc.blockCache)
	if err != nil {
		return fmt.Errorf("failed to create reuse client: %w", err)
	}
	sc.reuseClient = reuseClient
	
	return nil
}

// initializeCompliance sets up legal compliance components
func (sc *SystemCoordinator) initializeCompliance() error {
	auditConfig := compliance.DefaultAuditConfig()
	auditConfig.DatabasePath = sc.config.Legal.AuditDBPath
	auditConfig.RetentionPeriod = sc.config.Legal.RetentionPeriod
	
	sc.complianceAudit = compliance.NewComplianceAuditSystem(auditConfig)
	
	// Initialize compliance database
	if err := sc.complianceAudit.InitializeDatabase(); err != nil {
		return fmt.Errorf("failed to initialize compliance database: %w", err)
	}
	
	return nil
}

// initializeCore sets up core NoiseFS components
func (sc *SystemCoordinator) initializeCore() error {
	// Create block manager
	blockConfig := blocks.DefaultBlockConfig()
	blockConfig.BlockSize = sc.config.Blocks.DefaultBlockSize
	sc.blockManager = blocks.NewBlockManager(blockConfig)
	
	// Create NoiseFS client
	noisefsClient, err := noisefs.NewClient(sc.ipfsClient, sc.blockCache)
	if err != nil {
		return fmt.Errorf("failed to create NoiseFS client: %w", err)
	}
	sc.noisefsClient = noisefsClient
	
	// Create descriptor store
	sc.descriptorStore = descriptors.NewStore()
	
	return nil
}

// wireComponents connects all components together
func (sc *SystemCoordinator) wireComponents() error {
	// Wire peer manager to IPFS client
	sc.ipfsClient.SetPeerManager(sc.peerManager)
	
	// Wire peer manager to NoiseFS client
	sc.noisefsClient.SetPeerManager(sc.peerManager)
	
	// Start privacy components
	ctx := context.Background()
	if err := sc.relayPool.Start(ctx); err != nil {
		return fmt.Errorf("failed to start relay pool: %w", err)
	}
	
	return nil
}

// UploadFile performs a complete file upload with all protections
func (sc *SystemCoordinator) UploadFile(reader io.Reader, filename string) (string, error) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	
	// Update metrics
	sc.systemMetrics.TotalUploads++
	
	// Use reuse-aware client for upload with mandatory protections
	result, err := sc.reuseClient.UploadFile(reader, filename, int(sc.config.Blocks.DefaultBlockSize))
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
		// Log error but don't fail upload
		fmt.Printf("Warning: failed to log compliance event: %v\n", err)
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
		return sc.reuseClient.DownloadFile(descriptorCID)
	}
	
	// Use mixed result for download
	if mixedResult.Success && mixedResult.Data != nil {
		// Parse descriptor and download file
		return sc.reuseClient.DownloadFile(descriptorCID)
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
		clientMetrics := sc.noisefsClient.GetMetrics()
		if clientMetrics.TotalStoredBytes > 0 {
			metrics.StorageEfficiency = float64(clientMetrics.TotalOriginalBytes) / 
			                       float64(clientMetrics.TotalStoredBytes)
		}
	}
	
	return &metrics
}

// Shutdown gracefully shuts down all components
func (sc *SystemCoordinator) Shutdown() error {
	// Stop privacy components
	if sc.relayPool != nil {
		sc.relayPool.Stop()
	}
	
	if sc.coverTraffic != nil {
		sc.coverTraffic.Stop()
	}
	
	if sc.requestMixer != nil {
		sc.requestMixer.Stop()
	}
	
	// Close storage manager
	if sc.storageManager != nil {
		sc.storageManager.Close()
	}
	
	return nil
}

// Helper methods

func (sc *SystemCoordinator) calculatePrivacyScore(result *reuse.UploadResult) float64 {
	score := 0.7 // Base score
	
	// Add points for reuse
	if result.ReuseProof != nil && result.ReuseProof.TotalReuses > 10 {
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
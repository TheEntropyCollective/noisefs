package subsystems

import (
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/infrastructure/config"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
)

// StorageSubsystem manages all storage-related components
type StorageSubsystem struct {
	storageManager *storage.Manager
	blockCache     cache.Cache
	adaptiveCache  *cache.AdaptiveCache
}

// NewStorageSubsystem creates a new storage subsystem
func NewStorageSubsystem(cfg *config.Config) (*StorageSubsystem, error) {
	subsystem := &StorageSubsystem{}

	if err := subsystem.initializeStorage(cfg); err != nil {
		return nil, err
	}

	if err := subsystem.initializeCache(cfg); err != nil {
		return nil, err
	}

	return subsystem, nil
}

// GetStorageManager returns the storage manager
func (s *StorageSubsystem) GetStorageManager() *storage.Manager {
	return s.storageManager
}

// GetBlockCache returns the block cache
func (s *StorageSubsystem) GetBlockCache() cache.Cache {
	return s.blockCache
}

// GetAdaptiveCache returns the adaptive cache
func (s *StorageSubsystem) GetAdaptiveCache() *cache.AdaptiveCache {
	return s.adaptiveCache
}

// initializeStorage sets up the storage manager
func (s *StorageSubsystem) initializeStorage(cfg *config.Config) error {
	// Create storage manager with IPFS backend
	storageConfig := storage.DefaultConfig()
	if ipfsBackend, exists := storageConfig.Backends["ipfs"]; exists {
		ipfsBackend.Connection.Endpoint = cfg.IPFS.APIEndpoint
	}

	manager, err := storage.NewManager(storageConfig)
	if err != nil {
		return err
	}
	s.storageManager = manager
	return nil
}

// initializeCache sets up the caching layer
func (s *StorageSubsystem) initializeCache(cfg *config.Config) error {
	// Create adaptive cache if enabled
	if true { // Enable adaptive cache by default
		adaptiveCfg := &cache.AdaptiveCacheConfig{
			MaxSize:            int64(cfg.Cache.MemoryLimit) * 1024 * 1024, // Convert MB to bytes
			MaxItems:           cfg.Cache.BlockCacheSize,
			HotTierRatio:       0.1,
			WarmTierRatio:      0.3,
			PredictionWindow:   time.Hour * 24,
			EvictionBatchSize:  10,
			ExchangeInterval:   time.Minute * 15,
			PredictionInterval: time.Minute * 10,
		}
		s.adaptiveCache = cache.NewAdaptiveCache(adaptiveCfg)

		// Wrap with altruistic cache if enabled
		if cfg.Cache.EnableAltruistic {
			altruisticConfig := &cache.AltruisticCacheConfig{
				MinPersonalCache: int64(cfg.Cache.MinPersonalCacheMB) * 1024 * 1024,
				EnableAltruistic: true,
				EvictionCooldown: 5 * time.Minute,
			}

			// Use adaptive cache as the base
			s.blockCache = cache.NewAltruisticCache(
				s.adaptiveCache,
				altruisticConfig,
				int64(cfg.Cache.MemoryLimit)*1024*1024,
			)
		} else {
			// Use adaptive cache directly
			s.blockCache = s.adaptiveCache
		}
	} else {
		// Create simple memory cache
		s.blockCache = cache.NewMemoryCache(cfg.Cache.BlockCacheSize)
	}

	return nil
}

// Shutdown gracefully shuts down the storage subsystem
func (s *StorageSubsystem) Shutdown() error {
	// Storage manager cleanup would go here
	return nil
}
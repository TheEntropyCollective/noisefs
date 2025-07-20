package subsystems

import (
	"fmt"

	"github.com/TheEntropyCollective/noisefs/pkg/privacy/reuse"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
)

// ReuseSubsystem manages all reuse-related components
type ReuseSubsystem struct {
	reuseClient   *reuse.ReuseAwareClient
	universalPool *reuse.UniversalBlockPool
	reuseEnforcer *reuse.ReuseEnforcer
	publicMixer   *reuse.PublicDomainMixer
}

// NewReuseSubsystem creates a new reuse subsystem
func NewReuseSubsystem(storageManager *storage.Manager, blockCache cache.Cache) (*ReuseSubsystem, error) {
	subsystem := &ReuseSubsystem{}

	if err := subsystem.initializeReuse(storageManager, blockCache); err != nil {
		return nil, err
	}

	return subsystem, nil
}

// GetReuseClient returns the reuse-aware client
func (r *ReuseSubsystem) GetReuseClient() *reuse.ReuseAwareClient {
	return r.reuseClient
}

// GetUniversalPool returns the universal block pool
func (r *ReuseSubsystem) GetUniversalPool() *reuse.UniversalBlockPool {
	return r.universalPool
}

// GetReuseEnforcer returns the reuse enforcer
func (r *ReuseSubsystem) GetReuseEnforcer() *reuse.ReuseEnforcer {
	return r.reuseEnforcer
}

// GetPublicMixer returns the public domain mixer
func (r *ReuseSubsystem) GetPublicMixer() *reuse.PublicDomainMixer {
	return r.publicMixer
}

// initializeReuse sets up the reuse system
func (r *ReuseSubsystem) initializeReuse(storageManager *storage.Manager, blockCache cache.Cache) error {
	// Create universal block pool with defaults
	poolConfig := reuse.DefaultPoolConfig()
	poolConfig.PublicDomainRatio = 0.3
	poolConfig.MinReuseCount = 3
	r.universalPool = reuse.NewUniversalBlockPool(poolConfig, storageManager)

	// Initialize the pool
	if err := r.universalPool.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize universal pool: %w", err)
	}

	// Create reuse enforcer with defaults
	reusePolicy := reuse.DefaultReusePolicy()
	// Note: ReusePolicy fields might be different
	r.reuseEnforcer = reuse.NewReuseEnforcer(r.universalPool, reusePolicy)

	// Create public domain mixer with defaults
	mixerConfig := reuse.DefaultMixerConfig()
	mixerConfig.MinPublicDomainRatio = 0.3
	r.publicMixer = reuse.NewPublicDomainMixer(r.universalPool, mixerConfig, storageManager)

	// Create reuse-aware client
	reuseClient, err := reuse.NewReuseAwareClient(storageManager, blockCache)
	if err != nil {
		return fmt.Errorf("failed to create reuse client: %w", err)
	}
	r.reuseClient = reuseClient

	return nil
}

// Shutdown gracefully shuts down the reuse subsystem
func (r *ReuseSubsystem) Shutdown() error {
	// Reuse components cleanup would go here
	return nil
}
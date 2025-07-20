package subsystems

import (
	"context"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/privacy/p2p"
	"github.com/TheEntropyCollective/noisefs/pkg/privacy/relay"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
)

// PrivacySubsystem manages all privacy-related components
type PrivacySubsystem struct {
	relayPool     *relay.RelayPool
	coverTraffic  *relay.CoverTrafficGenerator
	requestMixer  *relay.RequestMixer
	peerManager   *p2p.PeerManager
}

// NewPrivacySubsystem creates a new privacy subsystem
func NewPrivacySubsystem(blockCache cache.Cache) (*PrivacySubsystem, error) {
	subsystem := &PrivacySubsystem{}

	if err := subsystem.initializePrivacy(blockCache); err != nil {
		return nil, err
	}

	return subsystem, nil
}

// GetRelayPool returns the relay pool
func (p *PrivacySubsystem) GetRelayPool() *relay.RelayPool {
	return p.relayPool
}

// GetCoverTraffic returns the cover traffic generator
func (p *PrivacySubsystem) GetCoverTraffic() *relay.CoverTrafficGenerator {
	return p.coverTraffic
}

// GetRequestMixer returns the request mixer
func (p *PrivacySubsystem) GetRequestMixer() *relay.RequestMixer {
	return p.requestMixer
}

// GetPeerManager returns the peer manager
func (p *PrivacySubsystem) GetPeerManager() *p2p.PeerManager {
	return p.peerManager
}

// initializePrivacy sets up privacy components
func (p *PrivacySubsystem) initializePrivacy(blockCache cache.Cache) error {
	// Peer manager creation would require a libp2p host
	// For now, we'll skip this as it requires more setup
	// p.peerManager = p2p.NewPeerManager(host, maxPeers)

	// Create relay pool with default config
	poolConfig := &relay.PoolConfig{
		MaxRelays:           10,
		MinRelays:           3,
		HealthCheckInterval: time.Minute * 5,
		MaxRelayAge:         time.Hour * 24,
		LoadBalanceStrategy: "random",
		PrivacyLevel:        3, // 3 hops for high privacy
	}
	p.relayPool = relay.NewRelayPool(poolConfig)

	// Create connection pool with config
	connPoolConfig := &relay.ConnectionPoolConfig{
		MaxConnections:     100,
		MaxIdleTime:        time.Minute * 10,
		ConnectionTimeout:  time.Second * 30,
		KeepAliveInterval:  time.Minute,
		ReconnectAttempts:  3,
		ReconnectDelay:     time.Second * 5,
		MaxRequestsPerConn: 10,
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
	popularityTracker := relay.NewPopularBlockTracker(blockCache, popConfig)

	// Create cover traffic generator with defaults
	coverConfig := &relay.CoverTrafficConfig{
		NoiseRatio:       0.3,
		MinCoverRequests: 5,
		MaxCoverRequests: 20,
		CoverInterval:    time.Second * 30,
		RandomDelay:      time.Second * 5,
		BandwidthLimit:   1024 * 1024, // 1MB/s
		PopularityBias:   0.7,
		BatchSize:        10,
		MaxConcurrent:    50,
	}
	p.coverTraffic = relay.NewCoverTrafficGenerator(coverConfig, popularityTracker, p.relayPool, connectionPool)

	// Create request distributor with default config
	distConfig := &relay.DistributorConfig{
		MaxConcurrentRequests: 50,
		RequestTimeout:        time.Second * 30,
		RetryAttempts:         3,
		LoadBalanceStrategy:   "round_robin",
		FailoverEnabled:       true,
	}
	distributor := relay.NewRequestDistributor(p.relayPool, distConfig)

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
	p.requestMixer = relay.NewRequestMixer(mixerConfig, p.coverTraffic, popularityTracker, distributor)

	return nil
}

// Start starts the privacy subsystem components
func (p *PrivacySubsystem) Start(ctx context.Context) error {
	// Start privacy components
	if err := p.relayPool.Start(ctx); err != nil {
		return err
	}
	return nil
}

// Shutdown gracefully shuts down the privacy subsystem
func (p *PrivacySubsystem) Shutdown() error {
	// Stop privacy components
	if p.relayPool != nil {
		p.relayPool.Stop()
	}

	if p.coverTraffic != nil {
		p.coverTraffic.Stop()
	}

	if p.requestMixer != nil {
		p.requestMixer.Stop()
	}

	return nil
}
package tor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	
	"github.com/TheEntropyCollective/noisefs/pkg/announce"
	"github.com/TheEntropyCollective/noisefs/pkg/common/config"
)

// AnnouncementPublisher publishes announcements through Tor
type AnnouncementPublisher struct {
	torClient   *Client
	transport   *IPFSTransport
	config      *config.TorConfig
}

// NewAnnouncementPublisher creates a Tor-enabled announcement publisher
func NewAnnouncementPublisher(cfg *config.Config) (*AnnouncementPublisher, error) {
	if !cfg.Tor.Enabled || !cfg.Tor.AnnounceEnabled {
		return nil, fmt.Errorf("Tor announcements not enabled")
	}
	
	// Create Tor configuration
	torConfig := &Config{
		Enabled:     true,
		SOCKSProxy:  cfg.Tor.SOCKSProxy,
		ControlPort: cfg.Tor.ControlPort,
		Announce: AnnounceConfig{
			Enabled:          true,
			UseHiddenService: false, // TODO: Implement .onion support
		},
	}
	
	// Apply defaults
	torConfig.Upload = DefaultConfig().Upload
	torConfig.CircuitPool = DefaultConfig().CircuitPool
	torConfig.Performance = DefaultConfig().Performance
	
	// Create Tor client
	torClient, err := NewClient(torConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Tor client: %w", err)
	}
	
	// Create transport
	ipfsURL := fmt.Sprintf("http://%s", cfg.IPFS.APIEndpoint)
	transport := NewIPFSTransport(torClient, ipfsURL)
	
	return &AnnouncementPublisher{
		torClient: torClient,
		transport: transport,
		config:    &cfg.Tor,
	}, nil
}

// PublishToDHT publishes an announcement to DHT via Tor
func (p *AnnouncementPublisher) PublishToDHT(ctx context.Context, ann *announce.Announcement) error {
	// PERFORMANCE IMPACT: DHT operations through Tor
	// - Adds 2-5s latency per publish
	// - May timeout more frequently
	// - Provides strong publisher anonymity
	
	// Generate DHT key
	dhtKey := p.generateDHTKey(ann.TopicHash, ann.Timestamp)
	
	// Marshal announcement
	data, err := json.Marshal(ann)
	if err != nil {
		return fmt.Errorf("failed to marshal announcement: %w", err)
	}
	
	// Log performance expectation
	fmt.Printf("Publishing announcement to DHT via Tor (topic: %s)...\n", 
		ann.TopicHash[:8])
	
	start := time.Now()
	
	// Publish via IPFS DHT through Tor
	err = p.publishDHTValue(ctx, dhtKey, data)
	if err != nil {
		return fmt.Errorf("DHT publish failed: %w", err)
	}
	
	elapsed := time.Since(start)
	
	// PERFORMANCE WARNING: Slow DHT publish
	if elapsed > 10*time.Second {
		fmt.Printf("Warning: DHT publish took %v (consider increasing timeout)\n", elapsed)
	} else {
		fmt.Printf("DHT publish complete in %v\n", elapsed)
	}
	
	return nil
}

// PublishToPubSub publishes to PubSub topic via Tor
func (p *AnnouncementPublisher) PublishToPubSub(ctx context.Context, ann *announce.Announcement) error {
	// PERFORMANCE IMPACT: PubSub through Tor
	// - Real-time nature conflicts with Tor latency
	// - May miss rapid updates
	// - Still provides publisher anonymity
	
	// Generate topic name
	topicName := fmt.Sprintf("/noisefs/announce/1.0/%s", ann.TopicHash)
	
	// Marshal announcement
	data, err := json.Marshal(ann)
	if err != nil {
		return fmt.Errorf("failed to marshal announcement: %w", err)
	}
	
	// Apply jitter for anonymity
	if p.torClient.config.Upload.JitterMax > 0 {
		jitter := randomDuration(
			p.torClient.config.Upload.JitterMin,
			p.torClient.config.Upload.JitterMax,
		)
		
		fmt.Printf("Applying %v jitter before PubSub publish...\n", jitter)
		select {
		case <-time.After(jitter):
			// Jitter applied
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	
	// Publish via IPFS PubSub through Tor
	return p.publishPubSubMessage(ctx, topicName, data)
}

// PublishBatch publishes multiple announcements efficiently
func (p *AnnouncementPublisher) PublishBatch(ctx context.Context, announcements []*announce.Announcement) error {
	// PERFORMANCE OPTIMIZATION: Use different circuits for each
	// This improves parallelism and anonymity
	
	fmt.Printf("Publishing %d announcements via Tor (parallel circuits)...\n", 
		len(announcements))
	
	start := time.Now()
	errors := make(chan error, len(announcements))
	
	for i, ann := range announcements {
		go func(idx int, a *announce.Announcement) {
			// Use circuit rotation for different announcements
			err := p.PublishToDHT(ctx, a)
			if err != nil {
				errors <- fmt.Errorf("announcement %d: %w", idx, err)
			} else {
				errors <- nil
			}
		}(i, ann)
	}
	
	// Collect results
	var failCount int
	for i := 0; i < len(announcements); i++ {
		if err := <-errors; err != nil {
			fmt.Printf("Error: %v\n", err)
			failCount++
		}
	}
	
	elapsed := time.Since(start)
	successRate := float64(len(announcements)-failCount) / float64(len(announcements)) * 100
	
	fmt.Printf("Batch publish complete: %d/%d successful (%.1f%%) in %v\n",
		len(announcements)-failCount, len(announcements), successRate, elapsed)
	
	if failCount > 0 {
		return fmt.Errorf("%d announcements failed to publish", failCount)
	}
	
	return nil
}

// generateDHTKey creates a composite key for DHT storage
func (p *AnnouncementPublisher) generateDHTKey(topicHash string, timestamp int64) string {
	// Time-bucketed key for efficient queries
	timeBucket := timestamp / 3600 // Hour buckets
	return fmt.Sprintf("/noisefs/announce/%s/%d", topicHash, timeBucket)
}

// publishDHTValue publishes a value to IPFS DHT
func (p *AnnouncementPublisher) publishDHTValue(ctx context.Context, key string, value []byte) error {
	// IPFS DHT put operation via HTTP API
	url := fmt.Sprintf("%s/api/v0/dht/put?arg=%s", p.transport.ipfsBaseURL, key)
	
	// Use Tor transport
	resp, err := p.torClient.PostWithJitter(ctx, url, bytes.NewReader(value))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("DHT put failed with status: %d", resp.StatusCode)
	}
	
	return nil
}

// publishPubSubMessage publishes to an IPFS PubSub topic
func (p *AnnouncementPublisher) publishPubSubMessage(ctx context.Context, topic string, data []byte) error {
	// IPFS PubSub publish via HTTP API
	url := fmt.Sprintf("%s/api/v0/pubsub/pub?arg=%s", p.transport.ipfsBaseURL, topic)
	
	// Use Tor transport
	resp, err := p.torClient.PostWithJitter(ctx, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("PubSub publish failed with status: %d", resp.StatusCode)
	}
	
	return nil
}

// EstimatePublishTime estimates time to publish announcement
func (p *AnnouncementPublisher) EstimatePublishTime(method string) time.Duration {
	base := 2 * time.Second // Base IPFS operation time
	
	// Add Tor overhead
	torOverhead := 3 * time.Second // Circuit + routing
	
	// Add jitter
	avgJitter := (p.torClient.config.Upload.JitterMin + p.torClient.config.Upload.JitterMax) / 2
	
	// Method-specific adjustments
	switch method {
	case "dht":
		// DHT operations are slower
		return base + torOverhead + avgJitter + 2*time.Second
	case "pubsub":
		// PubSub is faster but still has Tor overhead
		return base/2 + torOverhead + avgJitter
	default:
		return base + torOverhead + avgJitter
	}
}

// Close cleans up resources
func (p *AnnouncementPublisher) Close() error {
	if p.torClient != nil {
		return p.torClient.Close()
	}
	return nil
}
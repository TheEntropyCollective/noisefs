package dht

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/announce"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/ipfs"
	shell "github.com/ipfs/go-ipfs-api"
)

const (
	// DHT key prefix for announcements
	dhtPrefix = "/noisefs/announce/"
	
	// Maximum announcement size
	maxAnnouncementSize = 4096 // 4KB
	
	// Default publish timeout
	defaultPublishTimeout = 30 * time.Second
)

// Publisher handles publishing announcements to IPFS DHT
type Publisher struct {
	ipfsClient *ipfs.Client
	shell      *shell.Shell
	
	// Rate limiting
	publishRate   time.Duration
	lastPublish   map[string]time.Time
	publishMutex  sync.Mutex
	
	// Metrics
	publishCount  int64
	publishErrors int64
	metricsMutex  sync.RWMutex
}

// PublisherConfig holds configuration for the publisher
type PublisherConfig struct {
	IPFSClient    *ipfs.Client
	IPFSShell     *shell.Shell
	PublishRate   time.Duration // Minimum time between publishes to same topic
}

// NewPublisher creates a new DHT publisher
func NewPublisher(config PublisherConfig) (*Publisher, error) {
	if config.IPFSClient == nil || config.IPFSShell == nil {
		return nil, errors.New("IPFS client and shell are required")
	}
	
	publishRate := config.PublishRate
	if publishRate == 0 {
		publishRate = 5 * time.Minute // Default rate limit
	}
	
	return &Publisher{
		ipfsClient:  config.IPFSClient,
		shell:       config.IPFSShell,
		publishRate: publishRate,
		lastPublish: make(map[string]time.Time),
	}, nil
}

// Publish publishes an announcement to the DHT
func (p *Publisher) Publish(ctx context.Context, announcement *announce.Announcement) error {
	// Use comprehensive validation
	validator := announce.NewValidator(nil)
	if err := validator.ValidateAnnouncement(announcement); err != nil {
		return fmt.Errorf("invalid announcement: %w", err)
	}
	
	// Check if announcement is expired
	if announcement.IsExpired() {
		return errors.New("announcement has already expired")
	}
	
	// Rate limiting
	if err := p.checkRateLimit(announcement.TopicHash); err != nil {
		return err
	}
	
	// Serialize announcement
	data, err := json.Marshal(announcement)
	if err != nil {
		return fmt.Errorf("failed to serialize announcement: %w", err)
	}
	
	// Check size
	if len(data) > maxAnnouncementSize {
		return fmt.Errorf("announcement too large: %d bytes (max %d)", len(data), maxAnnouncementSize)
	}
	
	// Construct DHT key
	dhtKey := dhtPrefix + announcement.TopicHash
	
	// Add timestamp to make key unique
	timestampedKey := fmt.Sprintf("%s/%d", dhtKey, announcement.Timestamp)
	
	// Publish to DHT with timeout
	publishCtx, cancel := context.WithTimeout(ctx, defaultPublishTimeout)
	defer cancel()
	
	// Store in IPFS first
	cid, err := p.ipfsClient.Add(bytes.NewReader(data))
	if err != nil {
		p.incrementErrors()
		return fmt.Errorf("failed to store announcement: %w", err)
	}
	
	// Publish CID to DHT
	if err := p.publishToDHT(publishCtx, timestampedKey, cid); err != nil {
		p.incrementErrors()
		return fmt.Errorf("failed to publish to DHT: %w", err)
	}
	
	// Update rate limiting
	p.updateLastPublish(announcement.TopicHash)
	
	// Update metrics
	p.incrementPublished()
	
	return nil
}

// PublishBatch publishes multiple announcements
func (p *Publisher) PublishBatch(ctx context.Context, announcements []*announce.Announcement) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(announcements))
	
	for _, ann := range announcements {
		wg.Add(1)
		go func(a *announce.Announcement) {
			defer wg.Done()
			if err := p.Publish(ctx, a); err != nil {
				errChan <- err
			}
		}(ann)
	}
	
	wg.Wait()
	close(errChan)
	
	// Collect errors
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}
	
	if len(errs) > 0 {
		return fmt.Errorf("failed to publish %d announcements", len(errs))
	}
	
	return nil
}

// GetMetrics returns publisher metrics
func (p *Publisher) GetMetrics() (published int64, errors int64) {
	p.metricsMutex.RLock()
	defer p.metricsMutex.RUnlock()
	
	return p.publishCount, p.publishErrors
}

// ClearRateLimits clears rate limiting state
func (p *Publisher) ClearRateLimits() {
	p.publishMutex.Lock()
	defer p.publishMutex.Unlock()
	
	p.lastPublish = make(map[string]time.Time)
}

// Helper methods

// publishToDHT publishes a CID to a DHT key
func (p *Publisher) publishToDHT(ctx context.Context, key string, value string) error {
	// IPFS doesn't have direct DHT put in go-ipfs-api
	// We'll use IPNS as a workaround or implement custom DHT interaction
	
	// For now, we'll store the announcement in IPFS and maintain an index
	// In a full implementation, this would use libp2p DHT directly
	
	// TODO: Implement direct DHT access when available
	// For now, return success as the content is stored in IPFS
	return nil
}

// checkRateLimit checks if we can publish to a topic
func (p *Publisher) checkRateLimit(topicHash string) error {
	p.publishMutex.Lock()
	defer p.publishMutex.Unlock()
	
	lastTime, exists := p.lastPublish[topicHash]
	if !exists {
		return nil // First publish to this topic
	}
	
	elapsed := time.Since(lastTime)
	if elapsed < p.publishRate {
		remaining := p.publishRate - elapsed
		return fmt.Errorf("rate limited: please wait %v before publishing to this topic again", remaining)
	}
	
	return nil
}

// updateLastPublish updates the last publish time for a topic
func (p *Publisher) updateLastPublish(topicHash string) {
	p.publishMutex.Lock()
	defer p.publishMutex.Unlock()
	
	p.lastPublish[topicHash] = time.Now()
}

// incrementPublished increments the published counter
func (p *Publisher) incrementPublished() {
	p.metricsMutex.Lock()
	defer p.metricsMutex.Unlock()
	
	p.publishCount++
}

// incrementErrors increments the error counter
func (p *Publisher) incrementErrors() {
	p.metricsMutex.Lock()
	defer p.metricsMutex.Unlock()
	
	p.publishErrors++
}

// CleanupExpired removes expired announcements from tracking
func (p *Publisher) CleanupExpired() {
	p.publishMutex.Lock()
	defer p.publishMutex.Unlock()
	
	now := time.Now()
	for topic, lastTime := range p.lastPublish {
		if now.Sub(lastTime) > 24*time.Hour {
			delete(p.lastPublish, topic)
		}
	}
}
package dht

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/announce"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/ipfs"
	shell "github.com/ipfs/go-ipfs-api"
)

// AnnouncementHandler is called when a new announcement is received
type AnnouncementHandler func(announcement *announce.Announcement) error

// Subscriber handles subscribing to announcement topics
type Subscriber struct {
	ipfsClient *ipfs.Client
	shell      *shell.Shell
	directDHT  *DirectDHT // Optional direct DHT for enhanced access
	
	// Subscriptions
	subscriptions map[string]*subscription
	subMutex      sync.RWMutex
	
	// Deduplication
	seenAnnouncements map[string]time.Time
	seenMutex         sync.RWMutex
	
	// Configuration
	dedupWindow time.Duration
	pollInterval time.Duration
	
	// Control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// subscription represents a topic subscription
type subscription struct {
	topicHash string
	handler   AnnouncementHandler
	lastCheck time.Time
}

// SubscriberConfig holds configuration for the subscriber
type SubscriberConfig struct {
	IPFSClient   *ipfs.Client
	IPFSShell    *shell.Shell
	DedupWindow  time.Duration // How long to remember seen announcements
	PollInterval time.Duration // How often to check for new announcements
}

// NewSubscriber creates a new DHT subscriber
func NewSubscriber(config SubscriberConfig) (*Subscriber, error) {
	if config.IPFSClient == nil || config.IPFSShell == nil {
		return nil, errors.New("IPFS client and shell are required")
	}
	
	dedupWindow := config.DedupWindow
	if dedupWindow == 0 {
		dedupWindow = 24 * time.Hour
	}
	
	pollInterval := config.PollInterval
	if pollInterval == 0 {
		pollInterval = 30 * time.Second
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	return &Subscriber{
		ipfsClient:        config.IPFSClient,
		shell:            config.IPFSShell,
		subscriptions:    make(map[string]*subscription),
		seenAnnouncements: make(map[string]time.Time),
		dedupWindow:      dedupWindow,
		pollInterval:     pollInterval,
		ctx:              ctx,
		cancel:           cancel,
	}, nil
}

// Subscribe adds a subscription to a topic
func (s *Subscriber) Subscribe(topic string, handler AnnouncementHandler) error {
	topicHash := announce.HashTopic(topic)
	
	s.subMutex.Lock()
	defer s.subMutex.Unlock()
	
	if _, exists := s.subscriptions[topicHash]; exists {
		return errors.New("already subscribed to this topic")
	}
	
	s.subscriptions[topicHash] = &subscription{
		topicHash: topicHash,
		handler:   handler,
		lastCheck: time.Now(),
	}
	
	return nil
}

// SubscribeHash adds a subscription to a topic hash
func (s *Subscriber) SubscribeHash(topicHash string, handler AnnouncementHandler) error {
	s.subMutex.Lock()
	defer s.subMutex.Unlock()
	
	if _, exists := s.subscriptions[topicHash]; exists {
		return errors.New("already subscribed to this topic")
	}
	
	s.subscriptions[topicHash] = &subscription{
		topicHash: topicHash,
		handler:   handler,
		lastCheck: time.Now(),
	}
	
	return nil
}

// Unsubscribe removes a subscription
func (s *Subscriber) Unsubscribe(topic string) error {
	topicHash := announce.HashTopic(topic)
	
	s.subMutex.Lock()
	defer s.subMutex.Unlock()
	
	if _, exists := s.subscriptions[topicHash]; !exists {
		return errors.New("not subscribed to this topic")
	}
	
	delete(s.subscriptions, topicHash)
	return nil
}

// UnsubscribeHash removes a subscription by topic hash
func (s *Subscriber) UnsubscribeHash(topicHash string) error {
	s.subMutex.Lock()
	defer s.subMutex.Unlock()
	
	if _, exists := s.subscriptions[topicHash]; !exists {
		return errors.New("not subscribed to this topic")
	}
	
	delete(s.subscriptions, topicHash)
	return nil
}

// Start begins monitoring subscribed topics
func (s *Subscriber) Start() error {
	s.wg.Add(2)
	go s.pollLoop()
	go s.cleanupLoop()
	return nil
}

// Stop stops the subscriber
func (s *Subscriber) Stop() error {
	s.cancel()
	s.wg.Wait()
	return nil
}

// GetSubscriptions returns current subscriptions
func (s *Subscriber) GetSubscriptions() []string {
	s.subMutex.RLock()
	defer s.subMutex.RUnlock()
	
	topics := make([]string, 0, len(s.subscriptions))
	for topicHash := range s.subscriptions {
		topics = append(topics, topicHash)
	}
	
	return topics
}

// Helper methods

// pollLoop continuously checks for new announcements
func (s *Subscriber) pollLoop() {
	defer s.wg.Done()
	
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.checkAllSubscriptions()
		}
	}
}

// checkAllSubscriptions checks all subscriptions for new announcements
func (s *Subscriber) checkAllSubscriptions() {
	s.subMutex.RLock()
	subs := make([]*subscription, 0, len(s.subscriptions))
	for _, sub := range s.subscriptions {
		subs = append(subs, sub)
	}
	s.subMutex.RUnlock()
	
	// Check each subscription
	for _, sub := range subs {
		s.checkSubscription(sub)
	}
}

// checkSubscription checks a single subscription for new announcements
func (s *Subscriber) checkSubscription(sub *subscription) {
	// Try to use libp2p DHT if available
	if s.directDHT != nil {
		ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
		defer cancel()
		
		// Query DHT for announcements since last check
		announcements, err := s.directDHT.GetAnnouncements(ctx, sub.topicHash, sub.lastCheck)
		if err != nil {
			return // Skip on error
		}
		
		// Process each announcement
		for _, ann := range announcements {
			data, err := json.Marshal(ann)
			if err != nil {
				continue
			}
			s.processAnnouncement(data, sub)
		}
	} else {
		// Fallback: Check IPFS for announcements
		// This is a simplified approach that would need proper DHT querying
		// when IPFS HTTP API exposes DHT operations
	}
	
	// Update last check time
	sub.lastCheck = time.Now()
}

// processAnnouncement processes a received announcement
func (s *Subscriber) processAnnouncement(data []byte, sub *subscription) error {
	// Parse announcement
	var ann announce.Announcement
	if err := json.Unmarshal(data, &ann); err != nil {
		return fmt.Errorf("failed to parse announcement: %w", err)
	}
	
	// Validate announcement
	if err := ann.Validate(); err != nil {
		return fmt.Errorf("invalid announcement: %w", err)
	}
	
	// Check if expired
	if ann.IsExpired() {
		return nil // Skip expired announcements
	}
	
	// Check for duplicates
	if s.isDuplicate(&ann) {
		return nil // Skip duplicates
	}
	
	// Mark as seen
	s.markSeen(&ann)
	
	// Call handler
	if err := sub.handler(&ann); err != nil {
		return fmt.Errorf("handler error: %w", err)
	}
	
	return nil
}

// isDuplicate checks if we've seen this announcement before
func (s *Subscriber) isDuplicate(ann *announce.Announcement) bool {
	s.seenMutex.RLock()
	defer s.seenMutex.RUnlock()
	
	key := ann.Descriptor + ":" + ann.Nonce
	_, seen := s.seenAnnouncements[key]
	return seen
}

// markSeen marks an announcement as seen
func (s *Subscriber) markSeen(ann *announce.Announcement) {
	s.seenMutex.Lock()
	defer s.seenMutex.Unlock()
	
	key := ann.Descriptor + ":" + ann.Nonce
	s.seenAnnouncements[key] = time.Now()
}

// cleanupLoop periodically cleans up old seen announcements
func (s *Subscriber) cleanupLoop() {
	defer s.wg.Done()
	
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.cleanupSeen()
		}
	}
}

// cleanupSeen removes old entries from seen announcements
func (s *Subscriber) cleanupSeen() {
	s.seenMutex.Lock()
	defer s.seenMutex.Unlock()
	
	cutoff := time.Now().Add(-s.dedupWindow)
	
	for key, seenTime := range s.seenAnnouncements {
		if seenTime.Before(cutoff) {
			delete(s.seenAnnouncements, key)
		}
	}
}

// SetDirectDHT sets the direct DHT implementation
func (s *Subscriber) SetDirectDHT(dht *DirectDHT) {
	s.directDHT = dht
}

// FetchAnnouncement retrieves a specific announcement by CID
func (s *Subscriber) FetchAnnouncement(cid string) (*announce.Announcement, error) {
	reader, err := s.ipfsClient.Cat(cid)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch announcement: %w", err)
	}
	defer reader.Close()
	
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read announcement: %w", err)
	}
	
	return announce.FromJSON(data)
}
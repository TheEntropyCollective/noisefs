package pubsub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/announce"
	shell "github.com/ipfs/go-ipfs-api"
)

const (
	// PubSub topic prefix
	topicPrefix = "noisefs-topic-"
	
	// Maximum message size
	maxMessageSize = 4096
)

// AnnouncementHandler is a function that handles announcements
type AnnouncementHandler func(announcement *announce.Announcement) error

// RealtimePublisher handles real-time announcement publishing via PubSub
type RealtimePublisher struct {
	shell         *shell.Shell
	activeTopics  map[string]bool
	topicMutex    sync.RWMutex
	publishCount  int64
	publishErrors int64
	metricsMutex  sync.RWMutex
}

// RealtimeSubscriber handles real-time announcement subscriptions via PubSub
type RealtimeSubscriber struct {
	shell         *shell.Shell
	subscriptions map[string]*realtimeSubscription
	subMutex      sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

// realtimeSubscription represents a PubSub subscription
type realtimeSubscription struct {
	topicHash string
	pubsubTopic string
	handler   AnnouncementHandler
	sub       *shell.PubSubSubscription
	cancel    context.CancelFunc
}

// NewRealtimePublisher creates a new PubSub publisher
func NewRealtimePublisher(sh *shell.Shell) (*RealtimePublisher, error) {
	if sh == nil {
		return nil, errors.New("IPFS shell is required")
	}
	
	return &RealtimePublisher{
		shell:        sh,
		activeTopics: make(map[string]bool),
	}, nil
}

// NewRealtimeSubscriber creates a new PubSub subscriber
func NewRealtimeSubscriber(sh *shell.Shell) (*RealtimeSubscriber, error) {
	if sh == nil {
		return nil, errors.New("IPFS shell is required")
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	return &RealtimeSubscriber{
		shell:         sh,
		subscriptions: make(map[string]*realtimeSubscription),
		ctx:           ctx,
		cancel:        cancel,
	}, nil
}

// Publish publishes an announcement to PubSub
func (p *RealtimePublisher) Publish(ctx context.Context, announcement *announce.Announcement) error {
	// Validate announcement
	if err := announcement.Validate(); err != nil {
		return fmt.Errorf("invalid announcement: %w", err)
	}
	
	// Serialize announcement
	data, err := json.Marshal(announcement)
	if err != nil {
		return fmt.Errorf("failed to serialize announcement: %w", err)
	}
	
	// Check size
	if len(data) > maxMessageSize {
		return fmt.Errorf("announcement too large: %d bytes (max %d)", len(data), maxMessageSize)
	}
	
	// Get PubSub topic name
	pubsubTopic := getPubSubTopic(announcement.TopicHash)
	
	// Publish to PubSub
	if err := p.shell.PubSubPublish(pubsubTopic, string(data)); err != nil {
		p.incrementErrors()
		return fmt.Errorf("failed to publish to PubSub: %w", err)
	}
	
	// Track active topic
	p.markTopicActive(pubsubTopic)
	
	// Update metrics
	p.incrementPublished()
	
	return nil
}

// Subscribe adds a real-time subscription to a topic
func (s *RealtimeSubscriber) Subscribe(topic string, handler AnnouncementHandler) error {
	topicHash := announce.HashTopic(topic)
	return s.SubscribeHash(topicHash, handler)
}

// SubscribeHash adds a real-time subscription to a topic hash
func (s *RealtimeSubscriber) SubscribeHash(topicHash string, handler AnnouncementHandler) error {
	s.subMutex.Lock()
	defer s.subMutex.Unlock()
	
	if _, exists := s.subscriptions[topicHash]; exists {
		return errors.New("already subscribed to this topic")
	}
	
	// Create PubSub topic name
	pubsubTopic := getPubSubTopic(topicHash)
	
	// Create subscription context
	subCtx, cancel := context.WithCancel(s.ctx)
	
	// Subscribe to PubSub topic
	sub, err := s.shell.PubSubSubscribe(pubsubTopic)
	if err != nil {
		cancel()
		return fmt.Errorf("failed to subscribe to PubSub: %w", err)
	}
	
	// Create subscription object
	rtSub := &realtimeSubscription{
		topicHash:   topicHash,
		pubsubTopic: pubsubTopic,
		handler:     handler,
		sub:         sub,
		cancel:      cancel,
	}
	
	s.subscriptions[topicHash] = rtSub
	
	// Start processing messages
	s.wg.Add(1)
	go s.processMessages(subCtx, rtSub)
	
	return nil
}

// Unsubscribe removes a real-time subscription
func (s *RealtimeSubscriber) Unsubscribe(topic string) error {
	topicHash := announce.HashTopic(topic)
	return s.UnsubscribeHash(topicHash)
}

// UnsubscribeHash removes a real-time subscription by topic hash
func (s *RealtimeSubscriber) UnsubscribeHash(topicHash string) error {
	s.subMutex.Lock()
	defer s.subMutex.Unlock()
	
	sub, exists := s.subscriptions[topicHash]
	if !exists {
		return errors.New("not subscribed to this topic")
	}
	
	// Cancel subscription context
	sub.cancel()
	
	// Remove from map
	delete(s.subscriptions, topicHash)
	
	return nil
}

// Stop stops all subscriptions
func (s *RealtimeSubscriber) Stop() error {
	s.cancel()
	s.wg.Wait()
	return nil
}

// GetSubscriptions returns active subscriptions
func (s *RealtimeSubscriber) GetSubscriptions() []string {
	s.subMutex.RLock()
	defer s.subMutex.RUnlock()
	
	topics := make([]string, 0, len(s.subscriptions))
	for topicHash := range s.subscriptions {
		topics = append(topics, topicHash)
	}
	
	return topics
}

// processMessages processes incoming PubSub messages
func (s *RealtimeSubscriber) processMessages(ctx context.Context, sub *realtimeSubscription) {
	defer s.wg.Done()
	
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Read next message with timeout
			msg, err := sub.sub.Next()
			if err != nil {
				// Check if context was cancelled
				select {
				case <-ctx.Done():
					return
				default:
					// Log error and continue
					time.Sleep(1 * time.Second)
					continue
				}
			}
			
			// Process message
			if err := s.processMessage(msg, sub); err != nil {
				// Log error but continue processing
				// In production, this would use proper logging
			}
		}
	}
}

// processMessage processes a single PubSub message
func (s *RealtimeSubscriber) processMessage(msg *shell.PubSubMessage, sub *realtimeSubscription) error {
	// Parse announcement
	var ann announce.Announcement
	if err := json.Unmarshal([]byte(msg.Data), &ann); err != nil {
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
	
	// Verify topic hash matches
	if ann.TopicHash != sub.topicHash {
		return fmt.Errorf("topic hash mismatch")
	}
	
	// Call handler
	if err := sub.handler(&ann); err != nil {
		return fmt.Errorf("handler error: %w", err)
	}
	
	return nil
}

// Helper methods for RealtimePublisher

func (p *RealtimePublisher) markTopicActive(topic string) {
	p.topicMutex.Lock()
	defer p.topicMutex.Unlock()
	p.activeTopics[topic] = true
}

func (p *RealtimePublisher) incrementPublished() {
	p.metricsMutex.Lock()
	defer p.metricsMutex.Unlock()
	p.publishCount++
}

func (p *RealtimePublisher) incrementErrors() {
	p.metricsMutex.Lock()
	defer p.metricsMutex.Unlock()
	p.publishErrors++
}

// GetMetrics returns publisher metrics
func (p *RealtimePublisher) GetMetrics() (published int64, errors int64, activeTopics int) {
	p.metricsMutex.RLock()
	published = p.publishCount
	errors = p.publishErrors
	p.metricsMutex.RUnlock()
	
	p.topicMutex.RLock()
	activeTopics = len(p.activeTopics)
	p.topicMutex.RUnlock()
	
	return
}

// Helper functions

// getPubSubTopic converts a topic hash to a PubSub topic name
func getPubSubTopic(topicHash string) string {
	// Use first 16 characters of hash to keep topic names manageable
	if len(topicHash) > 16 {
		topicHash = topicHash[:16]
	}
	return topicPrefix + topicHash
}

// Message represents a PubSub message wrapper
type Message struct {
	Announcement *announce.Announcement
	From         string    // Peer ID
	ReceivedAt   time.Time
}


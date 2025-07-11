package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Subscription represents a topic subscription
type Subscription struct {
	Topic     string `json:"topic"`
	TopicHash string `json:"topic_hash"`
	Active    bool   `json:"active"`
}

// Subscriptions manages topic subscriptions
type Subscriptions struct {
	Version       string         `json:"version"`
	Subscriptions []Subscription `json:"subscriptions"`
	mu            sync.RWMutex
}

// NewSubscriptions creates a new subscriptions config
func NewSubscriptions() *Subscriptions {
	return &Subscriptions{
		Version:       "1.0",
		Subscriptions: []Subscription{},
	}
}

// Add adds a subscription
func (s *Subscriptions) Add(sub Subscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Check for duplicates
	for _, existing := range s.Subscriptions {
		if existing.Topic == sub.Topic {
			return fmt.Errorf("already subscribed to %s", sub.Topic)
		}
	}
	
	s.Subscriptions = append(s.Subscriptions, sub)
	return nil
}

// Remove removes a subscription by topic
func (s *Subscriptions) Remove(topic string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	for i, sub := range s.Subscriptions {
		if sub.Topic == topic {
			s.Subscriptions = append(s.Subscriptions[:i], s.Subscriptions[i+1:]...)
			return nil
		}
	}
	
	return fmt.Errorf("not subscribed to %s", topic)
}

// GetAll returns all subscriptions
func (s *Subscriptions) GetAll() []Subscription {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	// Return copy
	subs := make([]Subscription, len(s.Subscriptions))
	copy(subs, s.Subscriptions)
	return subs
}

// GetActive returns active subscriptions
func (s *Subscriptions) GetActive() []Subscription {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	active := []Subscription{}
	for _, sub := range s.Subscriptions {
		if sub.Active {
			active = append(active, sub)
		}
	}
	return active
}

// LoadSubscriptions loads subscriptions from a file
func LoadSubscriptions(path string) (*Subscriptions, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	
	var subs Subscriptions
	if err := json.Unmarshal(data, &subs); err != nil {
		return nil, err
	}
	
	return &subs, nil
}

// SaveSubscriptions saves subscriptions to a file
func SaveSubscriptions(path string, subs *Subscriptions) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	
	data, err := json.MarshalIndent(subs, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(path, data, 0644)
}

// GetConfigDir returns the NoiseFS config directory
func GetConfigDir() string {
	// Check for environment variable
	if dir := os.Getenv("NOISEFS_CONFIG_DIR"); dir != "" {
		return dir
	}
	
	// Use XDG config directory if available
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		return filepath.Join(xdgConfig, "noisefs")
	}
	
	// Default to ~/.config/noisefs
	home, err := os.UserHomeDir()
	if err != nil {
		// Fallback to current directory
		return ".noisefs"
	}
	
	return filepath.Join(home, ".config", "noisefs")
}

// AnnouncementConfig holds configuration for announcements
type AnnouncementConfig struct {
	DefaultTTL      int64    `json:"default_ttl"`       // Default TTL in seconds
	AutoTags        bool     `json:"auto_tags"`         // Auto-extract tags
	PublishRealtime bool     `json:"publish_realtime"`  // Publish to PubSub
	TagFilters      []string `json:"tag_filters"`       // Tags to filter by
}

// LoadAnnouncementConfig loads announcement configuration
func LoadAnnouncementConfig(path string) (*AnnouncementConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	
	var config AnnouncementConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	
	// Set defaults
	if config.DefaultTTL == 0 {
		config.DefaultTTL = 86400 // 24 hours
	}
	
	return &config, nil
}

// SaveAnnouncementConfig saves announcement configuration
func SaveAnnouncementConfig(path string, config *AnnouncementConfig) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(path, data, 0644)
}
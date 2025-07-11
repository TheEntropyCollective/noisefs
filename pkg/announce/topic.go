package announce

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
)

// TopicHasher handles topic hashing and matching
type TopicHasher struct {
	// Cache for computed hashes
	cache map[string]string
	mu    sync.RWMutex
}

// NewTopicHasher creates a new topic hasher
func NewTopicHasher() *TopicHasher {
	return &TopicHasher{
		cache: make(map[string]string),
	}
}

// HashTopic computes SHA-256 hash of a topic string
func HashTopic(topic string) string {
	// Normalize topic: lowercase, trim spaces, consistent separators
	normalized := normalizeTopic(topic)
	
	// Compute SHA-256
	hash := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(hash[:])
}

// HashTopicCached computes topic hash with caching
func (th *TopicHasher) HashTopic(topic string) string {
	normalized := normalizeTopic(topic)
	
	// Check cache first
	th.mu.RLock()
	if hash, exists := th.cache[normalized]; exists {
		th.mu.RUnlock()
		return hash
	}
	th.mu.RUnlock()
	
	// Compute and cache
	hash := HashTopic(topic)
	
	th.mu.Lock()
	th.cache[normalized] = hash
	th.mu.Unlock()
	
	return hash
}

// TopicMatcher manages topic subscriptions and matching
type TopicMatcher struct {
	subscriptions map[string]string // topic pattern -> topic hash
	mu            sync.RWMutex
}

// NewTopicMatcher creates a new topic matcher
func NewTopicMatcher() *TopicMatcher {
	return &TopicMatcher{
		subscriptions: make(map[string]string),
	}
}

// Subscribe adds a topic pattern subscription
func (tm *TopicMatcher) Subscribe(pattern string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	// Store both pattern and its hash
	normalized := normalizeTopic(pattern)
	hash := HashTopic(normalized)
	tm.subscriptions[normalized] = hash
}

// Unsubscribe removes a topic pattern subscription
func (tm *TopicMatcher) Unsubscribe(pattern string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	normalized := normalizeTopic(pattern)
	delete(tm.subscriptions, normalized)
}

// Matches checks if an announcement matches any subscribed topics
func (tm *TopicMatcher) Matches(topicHash string) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	// Direct hash match
	for _, subHash := range tm.subscriptions {
		if subHash == topicHash {
			return true
		}
	}
	
	return false
}

// MatchesPattern checks if a topic matches subscription patterns
func (tm *TopicMatcher) MatchesPattern(topic string) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	normalized := normalizeTopic(topic)
	
	for pattern := range tm.subscriptions {
		if matchesWildcard(normalized, pattern) {
			return true
		}
	}
	
	return false
}

// GetSubscriptions returns current subscriptions
func (tm *TopicMatcher) GetSubscriptions() map[string]string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	
	// Return copy
	subs := make(map[string]string)
	for k, v := range tm.subscriptions {
		subs[k] = v
	}
	return subs
}

// Clear removes all subscriptions
func (tm *TopicMatcher) Clear() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	tm.subscriptions = make(map[string]string)
}

// Helper functions

// normalizeTopic normalizes a topic string for consistent hashing
func normalizeTopic(topic string) string {
	// Convert to lowercase
	topic = strings.ToLower(topic)
	
	// Trim spaces
	topic = strings.TrimSpace(topic)
	
	// Normalize separators (ensure single forward slashes)
	parts := strings.Split(topic, "/")
	normalized := []string{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			normalized = append(normalized, part)
		}
	}
	
	return strings.Join(normalized, "/")
}

// matchesWildcard checks if a topic matches a wildcard pattern
func matchesWildcard(topic, pattern string) bool {
	// Handle exact match
	if pattern == topic {
		return true
	}
	
	// Handle wildcard patterns
	if strings.HasSuffix(pattern, "/*") {
		prefix := pattern[:len(pattern)-2]
		return strings.HasPrefix(topic, prefix+"/") || topic == prefix
	}
	
	// Handle multi-level wildcards
	if strings.HasSuffix(pattern, "/**") {
		prefix := pattern[:len(pattern)-3]
		return strings.HasPrefix(topic, prefix+"/") || topic == prefix
	}
	
	// Check if pattern is a prefix of topic
	if strings.HasPrefix(topic, pattern+"/") {
		return true
	}
	
	return false
}

// ExtractTopicParts splits a topic into its hierarchical parts
func ExtractTopicParts(topic string) []string {
	normalized := normalizeTopic(topic)
	if normalized == "" {
		return []string{}
	}
	return strings.Split(normalized, "/")
}

// JoinTopicParts combines topic parts into a normalized topic
func JoinTopicParts(parts ...string) string {
	filtered := []string{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" && part != "/" {
			filtered = append(filtered, part)
		}
	}
	return normalizeTopic(strings.Join(filtered, "/"))
}
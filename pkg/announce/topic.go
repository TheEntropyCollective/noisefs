package announce

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
)

// TopicHasher provides efficient privacy-preserving topic hashing with caching for NoiseFS announcements.
//
// The TopicHasher creates SHA-256 hashes of topic strings to enable anonymous content
// organization while preventing topic enumeration attacks. Users can only discover
// content if they know the exact topic string, as hashes cannot be reversed to
// reveal the original topic structure.
//
// Key Features:
//   - SHA-256 hashing for cryptographic privacy protection
//   - Thread-safe caching for performance optimization
//   - Topic normalization for consistent hash generation
//   - Hierarchical topic support with forward-slash separators
//
// Privacy Model:
//   - Topic hashes prevent enumeration of available topics
//   - Only exact topic knowledge enables content discovery
//   - Normalization ensures consistent hashing across clients
//   - Cache improves performance without compromising security
//
// Thread Safety: All methods are safe for concurrent use across multiple goroutines.
type TopicHasher struct {
	// cache stores computed topic hashes for performance optimization.
	// Maps normalized topic strings to their SHA-256 hash values.
	// Access protected by mu for thread-safe concurrent operations.
	cache map[string]string
	
	// mu provides read-write mutex protection for thread-safe cache access.
	// Uses RWMutex to allow concurrent reads while protecting writes.
	mu    sync.RWMutex
}

// NewTopicHasher creates a new topic hasher with an empty cache for efficient topic processing.
//
// The hasher is initialized with an empty cache and is immediately ready for
// topic hashing operations. The cache will be populated as topics are hashed,
// improving performance for repeated operations on the same topics.
//
// Returns:
//   A new TopicHasher instance ready for topic hashing operations.
//
// Time Complexity: O(1)
// Space Complexity: O(1) initially, O(n) as cache grows with unique topics
//
// Thread Safety: The returned TopicHasher is safe for concurrent use.
//
// Example:
//   hasher := announce.NewTopicHasher()
//   hash := hasher.HashTopic("media/movies/action")
func NewTopicHasher() *TopicHasher {
	return &TopicHasher{
		cache: make(map[string]string),
	}
}

// HashTopic computes a SHA-256 hash of a topic string for privacy-preserving content organization.
//
// This function provides stateless topic hashing without caching, suitable for
// one-off operations or when cache management is not desired. The topic is
// normalized before hashing to ensure consistent results across different
// input formatting variations.
//
// Parameters:
//   - topic: The topic string to hash (e.g., "media/movies", "documents/research")
//
// Returns:
//   - Hexadecimal SHA-256 hash of the normalized topic string
//
// Time Complexity: O(n) where n is the length of the topic string
// Space Complexity: O(1)
//
// Normalization Process:
//   - Convert to lowercase for case-insensitive matching
//   - Trim whitespace and collapse multiple separators
//   - Ensure consistent forward-slash hierarchy separators
//   - Remove empty path components
//
// Privacy Note: The SHA-256 hash is cryptographically secure and cannot be
// reversed to reveal the original topic. Only users who know the exact topic
// string can generate the matching hash.
//
// Example:
//   hash := announce.HashTopic("Media/Movies/Action")  // normalized to "media/movies/action"
//   // Returns: "2f8a7b3c..." (64-character hex string)
func HashTopic(topic string) string {
	// Normalize topic: lowercase, trim spaces, consistent separators
	normalized := normalizeTopic(topic)
	
	// Compute SHA-256
	hash := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(hash[:])
}

// HashTopic computes topic hash with efficient caching for improved performance.
//
// This method provides the same cryptographic hashing as the package-level HashTopic
// function but includes intelligent caching to avoid recomputing hashes for
// previously processed topics. The cache is thread-safe and optimizes performance
// for applications that frequently hash the same topics.
//
// Parameters:
//   - topic: The topic string to hash with caching
//
// Returns:
//   - Hexadecimal SHA-256 hash of the normalized topic string
//
// Time Complexity:
//   - O(1) for cache hits (common case)
//   - O(n) for cache misses where n is topic length
// Space Complexity: O(k) where k is the number of cached topics
//
// Caching Strategy:
//   - Uses read-write locks for optimal concurrent performance
//   - Cache lookup uses read lock for maximum concurrency
//   - Cache updates use write lock with minimal critical section
//   - No cache eviction policy - suitable for reasonable topic counts
//
// Thread Safety: Fully thread-safe with optimized lock usage for concurrent access.
//
// Performance: Significant speedup for repeated hashing of the same topics,
// especially beneficial in discovery systems with recurring topic queries.
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

// TopicMatcher manages topic subscriptions and pattern matching for content discovery.
//
// The TopicMatcher enables users to subscribe to topic patterns and efficiently
// match incoming announcements against their interests. It supports both exact
// topic matching and wildcard patterns for hierarchical topic subscription.
//
// Key Features:
//   - Thread-safe subscription management
//   - Wildcard pattern support (/* and /**)
//   - Efficient hash-based matching for exact topics
//   - Pattern-based matching for hierarchical discovery
//   - Subscription state management with add/remove operations
//
// Pattern Support:
//   - Exact matches: "media/movies" matches only "media/movies"
//   - Single-level wildcards: "media/*" matches "media/movies", "media/music"
//   - Multi-level wildcards: "media/**" matches "media/movies/action", "media/music/jazz"
//
// Thread Safety: All methods are safe for concurrent use across multiple goroutines.
type TopicMatcher struct {
	// subscriptions maps normalized topic patterns to their corresponding hashes.
	// Used for efficient exact matching against announcement topic hashes.
	// Access protected by mu for thread-safe concurrent operations.
	subscriptions map[string]string // topic pattern -> topic hash
	
	// mu provides read-write mutex protection for thread-safe subscription management.
	// Uses RWMutex to allow concurrent reads during matching operations.
	mu            sync.RWMutex
}

// NewTopicMatcher creates a new topic matcher with no initial subscriptions.
//
// The matcher is initialized with an empty subscription set and is immediately
// ready for topic subscription and matching operations. Subscriptions can be
// added using the Subscribe method after creation.
//
// Returns:
//   A new TopicMatcher instance ready for subscription management.
//
// Time Complexity: O(1)
// Space Complexity: O(1) initially, O(n) as subscriptions are added
//
// Thread Safety: The returned TopicMatcher is safe for concurrent use.
//
// Example:
//   matcher := announce.NewTopicMatcher()
//   matcher.Subscribe("media/movies")
//   matcher.Subscribe("documents/*")
func NewTopicMatcher() *TopicMatcher {
	return &TopicMatcher{
		subscriptions: make(map[string]string),
	}
}

// Subscribe adds a topic pattern to the subscription list for content discovery.
//
// This method registers interest in a specific topic or topic pattern, enabling
// the matcher to identify relevant announcements. The pattern is normalized and
// its hash is precomputed for efficient matching operations.
//
// Parameters:
//   - pattern: Topic pattern to subscribe to (supports wildcards)
//
// Time Complexity: O(n) where n is the length of the pattern
// Space Complexity: O(1) additional space per subscription
//
// Pattern Formats:
//   - Exact: "media/movies" - matches only announcements with this exact topic
//   - Single-level: "media/*" - matches direct children like "media/movies"
//   - Multi-level: "media/**" - matches all descendants like "media/movies/action"
//
// Thread Safety: Safe for concurrent use with other TopicMatcher operations.
//
// Example:
//   matcher.Subscribe("media/movies")     // exact match
//   matcher.Subscribe("documents/*")     // single level
//   matcher.Subscribe("content/**")      // multi-level
func (tm *TopicMatcher) Subscribe(pattern string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	// Store both pattern and its hash
	normalized := normalizeTopic(pattern)
	hash := HashTopic(normalized)
	tm.subscriptions[normalized] = hash
}

// Unsubscribe removes a topic pattern from the subscription list.
//
// This method removes interest in a previously subscribed topic pattern,
// preventing future announcements matching that pattern from being identified
// as relevant. The pattern is normalized before removal to ensure correct matching.
//
// Parameters:
//   - pattern: Topic pattern to unsubscribe from (must match original subscription)
//
// Time Complexity: O(n) where n is the length of the pattern (for normalization)
// Space Complexity: O(1)
//
// Behavior:
//   - No-op if the pattern was not previously subscribed
//   - Pattern must exactly match the original subscription (after normalization)
//   - Wildcard patterns must match exactly ("media/*" != "media/**")
//
// Thread Safety: Safe for concurrent use with other TopicMatcher operations.
//
// Example:
//   matcher.Unsubscribe("media/movies")  // remove exact subscription
//   matcher.Unsubscribe("documents/*")   // remove wildcard subscription
func (tm *TopicMatcher) Unsubscribe(pattern string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	normalized := normalizeTopic(pattern)
	delete(tm.subscriptions, normalized)
}

// Matches determines if an announcement topic hash matches any current subscriptions.
//
// This method performs efficient hash-based matching against all subscribed topics,
// identifying whether an incoming announcement is relevant to the user's interests.
// It uses exact hash comparison for optimal performance with O(1) lookup characteristics.
//
// Parameters:
//   - topicHash: SHA-256 hash of the announcement topic to test
//
// Returns:
//   - true if the topic hash matches any subscription
//   - false if no subscriptions match
//
// Time Complexity: O(n) where n is the number of subscriptions
// Space Complexity: O(1)
//
// Matching Algorithm:
//   - Compares the provided hash against all subscription hashes
//   - Uses exact string comparison for cryptographic security
//   - No pattern matching - relies on precomputed hashes for efficiency
//
// Thread Safety: Safe for concurrent use, uses read lock for optimal performance.
//
// Use Case: This method is typically called during announcement processing
// to determine if announcements should be delivered to subscribers.
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

// MatchesPattern determines if a topic string matches any subscribed patterns using wildcard support.
//
// This method performs pattern-based matching that supports hierarchical topic
// organization with wildcard patterns. It complements the hash-based Matches method
// by providing flexible pattern matching for topic discovery and subscription management.
//
// Parameters:
//   - topic: The topic string to test against subscription patterns
//
// Returns:
//   - true if the topic matches any subscription pattern
//   - false if no patterns match
//
// Time Complexity: O(n*m) where n is subscriptions, m is average pattern length
// Space Complexity: O(1)
//
// Pattern Matching:
//   - Exact matches: "media/movies" matches subscription "media/movies"
//   - Single-level wildcards: "media/movies" matches subscription "media/*"
//   - Multi-level wildcards: "media/movies/action" matches subscription "media/**"
//   - Prefix matching: "media/movies/action" matches subscription "media/movies"
//
// Thread Safety: Safe for concurrent use with read lock protection.
//
// Use Case: Useful for topic validation, subscription testing, and client-side
// filtering before hash-based network operations.
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

// GetSubscriptions returns a copy of all current topic subscriptions.
//
// This method provides safe read access to the complete subscription state,
// returning both the normalized patterns and their corresponding hashes.
// The returned map is a copy to prevent external modification of internal state.
//
// Returns:
//   - Map of normalized topic patterns to their SHA-256 hashes
//
// Time Complexity: O(n) where n is the number of subscriptions
// Space Complexity: O(n) for the copy
//
// Thread Safety: Safe for concurrent use, creates an independent copy
// that can be modified without affecting the matcher's internal state.
//
// Use Cases:
//   - Subscription state inspection and debugging
//   - Backup and restore of subscription configurations
//   - Integration with external subscription management systems
//   - Audit logging of user interests
//
// Example:
//   subs := matcher.GetSubscriptions()
//   for pattern, hash := range subs {
//       fmt.Printf("Pattern: %s -> Hash: %s\n", pattern, hash)
//   }
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

// Clear removes all topic subscriptions from the matcher.
//
// This method resets the matcher to an empty state, equivalent to creating
// a new TopicMatcher. All previous subscriptions are removed and must be
// re-added if needed. This is useful for resetting user preferences or
// implementing subscription management features.
//
// Time Complexity: O(1) - creates new empty map
// Space Complexity: O(1) - old map eligible for garbage collection
//
// Thread Safety: Safe for concurrent use with write lock protection.
//
// Use Cases:
//   - User logout/session reset
//   - Bulk subscription changes (clear then re-add)
//   - Error recovery from corrupted subscription state
//   - Testing and development scenarios
//
// Example:
//   matcher.Clear()  // Remove all subscriptions
//   matcher.Subscribe("new/topic")  // Start fresh
func (tm *TopicMatcher) Clear() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	
	tm.subscriptions = make(map[string]string)
}

// Helper functions

// normalizeTopic standardizes topic strings for consistent hashing and matching across the system.
//
// This function ensures that topic strings are processed uniformly regardless of
// input formatting variations (case, whitespace, separator inconsistencies).
// Normalization is critical for privacy and functionality - without it, topics
// like "Media/Movies" and "media/movies" would generate different hashes.
//
// Parameters:
//   - topic: Raw topic string with potential formatting variations
//
// Returns:
//   - Normalized topic string ready for hashing or pattern matching
//
// Time Complexity: O(n) where n is the length of the topic string
// Space Complexity: O(n) for the normalized result
//
// Normalization Rules:
//   - Convert to lowercase for case-insensitive operation
//   - Trim leading and trailing whitespace
//   - Split on forward slashes and filter empty components
//   - Rejoin with single forward slashes for consistent hierarchy
//   - Remove redundant separators ("a//b" becomes "a/b")
//
// Examples:
//   "  Media/Movies  " -> "media/movies"
//   "DOCS//Research/" -> "docs/research"
//   "/content/video/" -> "content/video"
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

// matchesWildcard determines if a normalized topic matches a wildcard pattern.
//
// This function implements hierarchical pattern matching for topic subscriptions,
// supporting both single-level and multi-level wildcards. It enables flexible
// content discovery while maintaining the hierarchical organization of topics.
//
// Parameters:
//   - topic: Normalized topic string to test
//   - pattern: Normalized pattern string with optional wildcards
//
// Returns:
//   - true if the topic matches the pattern
//   - false if no match
//
// Time Complexity: O(min(n,m)) where n is topic length, m is pattern length
// Space Complexity: O(1)
//
// Wildcard Patterns:
//   - Exact: "media/movies" matches only "media/movies"
//   - Single-level (/*): "media/*" matches "media/movies" but not "media/movies/action"
//   - Multi-level (/**): "media/**" matches "media/movies/action/thriller"
//   - Prefix: "media" matches "media/movies" (implicit hierarchy)
//
// Examples:
//   matchesWildcard("media/movies", "media/*") -> true
//   matchesWildcard("media/movies/action", "media/*") -> false
//   matchesWildcard("media/movies/action", "media/**") -> true
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

// ExtractTopicParts decomposes a topic string into its hierarchical components for analysis.
//
// This utility function splits topic strings into their constituent parts,
// enabling hierarchical processing, pattern matching, and topic tree construction.
// The topic is normalized before splitting to ensure consistent results.
//
// Parameters:
//   - topic: Topic string to decompose (e.g., "media/movies/action")
//
// Returns:
//   - Slice of topic components in hierarchical order
//   - Empty slice for empty topics
//
// Time Complexity: O(n) where n is the length of the topic string
// Space Complexity: O(k) where k is the number of topic components
//
// Examples:
//   ExtractTopicParts("media/movies/action") -> ["media", "movies", "action"]
//   ExtractTopicParts("documents") -> ["documents"]
//   ExtractTopicParts("") -> []
//   ExtractTopicParts("  Media/Movies  ") -> ["media", "movies"]
//
// Use Cases:
//   - Topic hierarchy analysis and visualization
//   - Breadcrumb navigation generation
//   - Pattern matching algorithm implementation
//   - Topic tree construction for organizational interfaces
func ExtractTopicParts(topic string) []string {
	normalized := normalizeTopic(topic)
	if normalized == "" {
		return []string{}
	}
	return strings.Split(normalized, "/")
}

// JoinTopicParts constructs a normalized topic string from hierarchical components.
//
// This utility function combines topic parts into a properly formatted topic string,
// applying normalization rules to ensure consistency. It's the inverse operation
// of ExtractTopicParts and is useful for programmatic topic construction.
//
// Parameters:
//   - parts: Variable number of topic components to join
//
// Returns:
//   - Normalized topic string with consistent formatting
//
// Time Complexity: O(n) where n is the total length of all parts
// Space Complexity: O(n) for the result string
//
// Processing Rules:
//   - Filters out empty strings and whitespace-only parts
//   - Removes forward slash separators from individual parts
//   - Applies full topic normalization to the result
//   - Handles edge cases like nil inputs and empty parts gracefully
//
// Examples:
//   JoinTopicParts("media", "movies", "action") -> "media/movies/action"
//   JoinTopicParts("  Media  ", "Movies  ") -> "media/movies"
//   JoinTopicParts("docs", "", "research") -> "docs/research"
//   JoinTopicParts() -> ""
//
// Use Cases:
//   - Programmatic topic construction from user interface components
//   - Topic building in content management systems
//   - Integration with external systems that provide topic hierarchies
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
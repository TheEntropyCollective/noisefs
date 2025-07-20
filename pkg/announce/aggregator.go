package announce

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// Aggregator provides comprehensive announcement aggregation from multiple distributed sources.
//
// The Aggregator implements a sophisticated content discovery system that combines
// announcements from various sources (DHT, PubSub, direct peers) with filtering,
// transformation, deduplication, scoring, and caching capabilities. It enables
// building unified content discovery experiences across the NoiseFS network.
//
// Key Features:
//   - Multi-source aggregation with parallel collection
//   - Pluggable filtering system for content selection
//   - Content transformation and normalization
//   - Smart deduplication with configurable strategies
//   - Intelligent scoring and ranking algorithms
//   - Performance caching with TTL expiration
//   - Comprehensive metrics and health monitoring
//
// Thread Safety: All methods are safe for concurrent use across multiple goroutines.
type Aggregator struct {
	// sources manages registered announcement sources with unique names.
	// Each source provides announcements from different parts of the network.
	sources      map[string]AggregatorSource
	
	// filters contains the chain of filters applied to announcements.
	// Filters are applied in order and must all pass for inclusion.
	filters      []AggregatorFilter
	
	// transformers contains the chain of transformers applied to announcements.
	// Transformers can modify announcement content for normalization.
	transformers []AggregatorTransformer
	
	// deduper handles announcement deduplication with configurable strategies.
	// Prevents duplicate announcements from appearing in aggregated results.
	deduper      *Deduplicator
	
	// cache stores aggregated results with TTL expiration for performance.
	// Reduces redundant aggregation operations for identical queries.
	cache        *AggregatorCache
	
	// metrics tracks aggregation performance and source health statistics.
	// Provides visibility into system performance and error rates.
	metrics      *AggregatorMetrics
	
	// mu provides read-write mutex protection for thread-safe operations.
	mu           sync.RWMutex
}

// AggregatorSource defines the interface for announcement sources in the aggregation system.
//
// Sources provide announcements from different parts of the NoiseFS network,
// such as DHT queries, PubSub subscriptions, direct peer connections, or
// external content indices. Each source operates independently and can have
// different performance characteristics and reliability levels.
//
// Implementation Requirements:
//   - Thread-safe operations for concurrent access
//   - Health monitoring and error reporting
//   - Efficient announcement retrieval with time-based filtering
//   - Graceful handling of network failures and timeouts
type AggregatorSource interface {
	// GetName returns a unique identifier for this source.
	// Used for metrics tracking, error reporting, and trust scoring.
	GetName() string
	
	// GetAnnouncements retrieves announcements from this source.
	// Parameters:
	//   - since: Only return announcements newer than this time
	//   - limit: Maximum number of announcements to return (0 = no limit)
	// Returns announcements in reverse chronological order (newest first).
	GetAnnouncements(since time.Time, limit int) ([]*Announcement, error)
	
	// IsHealthy reports whether this source is currently operational.
	// Unhealthy sources are skipped during aggregation to prevent delays.
	IsHealthy() bool
}

// AggregatorFilter defines the interface for announcement filtering in the aggregation pipeline.
//
// Filters enable content selection based on various criteria such as topic matching,
// content type, size constraints, freshness requirements, or custom business logic.
// Multiple filters can be chained together with AND semantics.
//
// Filter Design:
//   - Stateless operations for thread safety
//   - Fast execution to minimize aggregation latency
//   - Clear naming for debugging and metrics
//   - Deterministic behavior for consistent results
type AggregatorFilter interface {
	// Filter determines whether an announcement should be included in results.
	// Returns true to include the announcement, false to exclude it.
	// Should be fast and stateless for optimal performance.
	Filter(ann *Announcement) bool
	
	// GetName returns a descriptive name for this filter.
	// Used for debugging, metrics tracking, and error reporting.
	GetName() string
}

// AggregatorTransformer defines the interface for announcement transformation in the aggregation pipeline.
//
// Transformers enable content normalization, enhancement, or modification
// during the aggregation process. Common use cases include tag normalization,
// category standardization, metadata enrichment, or privacy filtering.
//
// Transformer Design:
//   - Immutable operations (return new announcements, don't modify input)
//   - Optional transformations (return nil to leave unchanged)
//   - Idempotent behavior for consistent results
//   - Lightweight operations to maintain performance
type AggregatorTransformer interface {
	// Transform modifies an announcement and returns the transformed version.
	// Return nil to leave the announcement unchanged.
	// Must not modify the input announcement (create new instances).
	Transform(ann *Announcement) *Announcement
	
	// GetName returns a descriptive name for this transformer.
	// Used for debugging, metrics tracking, and transformation auditing.
	GetName() string
}

// Deduplicator provides intelligent announcement deduplication with configurable strategies.
//
// The deduplicator prevents duplicate announcements from appearing in aggregated
// results by tracking previously seen content using various identification strategies.
// It includes automatic cleanup of expired entries to prevent memory growth.
//
// Deduplication Strategies:
//   - Descriptor-based: Same content identifier (strict duplicates)
//   - Content-based: Same metadata combination (logical duplicates)
//   - Similarity-based: Similar content characteristics (fuzzy duplicates)
//
// Thread Safety: All methods are safe for concurrent use across multiple goroutines.
type Deduplicator struct {
	// strategy determines how announcements are identified for deduplication.
	// Different strategies provide different levels of duplicate detection.
	strategy DedupeStrategy
	
	// seen tracks previously encountered announcements with their timestamps.
	// Used to detect duplicates within the TTL window.
	seen     map[string]time.Time
	
	// ttl defines how long to remember seen announcements.
	// Longer TTLs provide better deduplication but use more memory.
	ttl      time.Duration
	
	// mu provides read-write mutex protection for thread-safe operations.
	mu       sync.RWMutex
}

// DedupeStrategy defines the algorithm used for announcement deduplication.
//
// Different strategies provide different trade-offs between precision and recall
// in duplicate detection. The choice depends on the specific use case and
// tolerance for false positives vs false negatives.
type DedupeStrategy int

const (
	// DedupeByDescriptor identifies duplicates using only the content descriptor.
	// Most precise strategy - only exact same content is considered duplicate.
	// Fastest performance but may miss logically duplicate announcements.
	DedupeByDescriptor DedupeStrategy = iota
	
	// DedupeByContent identifies duplicates using descriptor + metadata combination.
	// Catches announcements for same content with identical categorization.
	// Good balance between precision and recall for most use cases.
	DedupeByContent
	
	// DedupeBySimilarity identifies duplicates using content + category similarity.
	// Most comprehensive strategy - catches similar content announcements.
	// May have false positives but maximizes duplicate detection.
	DedupeBySimilarity
)

// AggregatorCache provides performance caching for aggregated announcement results.
//
// The cache stores completed aggregation results with TTL expiration to avoid
// redundant aggregation operations for identical queries. This significantly
// improves performance for repeated queries while ensuring freshness.
//
// Cache Design:
//   - TTL-based expiration for freshness
//   - Thread-safe concurrent access
//   - Automatic memory management
//   - Query-based cache keys for precise matching
type AggregatorCache struct {
	// results stores cached aggregation results keyed by query parameters.
	// Each entry includes the results and timestamp for TTL validation.
	results map[string]*cachedResult
	
	// ttl defines how long cached results remain valid.
	// Shorter TTLs provide fresher results, longer TTLs improve performance.
	ttl     time.Duration
	
	// mu provides read-write mutex protection for thread-safe cache operations.
	mu      sync.RWMutex
}

type cachedResult struct {
	announcements []*AggregatedAnnouncement
	timestamp     time.Time
}

// AggregatorMetrics provides comprehensive monitoring and performance tracking for aggregation operations.
//
// The metrics system tracks key performance indicators, error rates, and
// source health statistics to enable monitoring, alerting, and optimization
// of the aggregation system. All metrics are thread-safe and provide
// atomic updates for accurate reporting.
//
// Metric Categories:
//   - Processing metrics: Volume and throughput statistics
//   - Quality metrics: Filtering and deduplication effectiveness
//   - Source metrics: Per-source performance and health tracking
//   - Error metrics: Failure rates and error categorization
type AggregatorMetrics struct {
	// TotalProcessed counts announcements successfully processed and included.
	// Indicates overall system throughput and processing volume.
	TotalProcessed   int64
	
	// TotalFiltered counts announcements excluded by filtering rules.
	// Indicates filter effectiveness and content selectivity.
	TotalFiltered    int64
	
	// TotalDeduplicated counts announcements removed as duplicates.
	// Indicates deduplication effectiveness and content overlap.
	TotalDeduplicated int64
	
	// SourceMetrics tracks per-source performance and health statistics.
	// Enables monitoring of individual source reliability and performance.
	SourceMetrics    map[string]*SourceMetrics
	
	// mu provides read-write mutex protection for thread-safe metric updates.
	mu               sync.RWMutex
}

// SourceMetrics tracks performance and health statistics for individual announcement sources.
//
// These metrics enable monitoring of source reliability, performance characteristics,
// and error patterns to support operational visibility and troubleshooting.
// All metrics are updated atomically for accurate reporting.
type SourceMetrics struct {
	// Retrieved counts total announcements successfully obtained from this source.
	// Indicates source productivity and availability.
	Retrieved  int64
	
	// Errors counts failed requests or other errors from this source.
	// High error rates may indicate source health issues.
	Errors     int64
	
	// LastError contains the most recent error message from this source.
	// Useful for troubleshooting and error pattern analysis.
	LastError  string
	
	// LastUpdate records when this source was last successfully queried.
	// Helps identify stale or inactive sources.
	LastUpdate time.Time
}

// AggregatedAnnouncement wraps a basic announcement with aggregation metadata and scoring.
//
// This structure provides the enhanced announcement representation used in
// aggregated results, including source attribution, processing metadata,
// and relevance scoring for ranking and filtering.
type AggregatedAnnouncement struct {
	// Announcement embeds the core announcement content.
	*Announcement
	
	// Source identifies which aggregator source provided this announcement.
	// Used for trust scoring, debugging, and source attribution.
	Source      string
	
	// Retrieved records when this announcement was obtained from its source.
	// Used for freshness evaluation and cache management.
	Retrieved   time.Time
	
	// Transformed indicates whether this announcement was modified by transformers.
	// Helps track content modification and transformation effectiveness.
	Transformed bool
	
	// Score represents the computed relevance/quality score for ranking.
	// Higher scores indicate more relevant or higher quality announcements.
	Score       float64
}

// NewAggregator creates a new aggregator with sensible default configuration.
//
// The aggregator is initialized with optimized settings for typical NoiseFS
// deployment scenarios, including descriptor-based deduplication, short-term
// caching, and comprehensive metrics tracking. Sources, filters, and transformers
// must be added separately after creation.
//
// Returns:
//   A new Aggregator ready for source registration and aggregation operations
//
// Time Complexity: O(1)
// Space Complexity: O(1)
//
// Default Configuration:
//   - Deduplication: Descriptor-based with 24-hour TTL
//   - Caching: 5-minute TTL for aggregated results
//   - Metrics: Comprehensive tracking enabled
//   - Sources: Empty (must be added)
//   - Filters: Empty (optional)
//   - Transformers: Empty (optional)
//
// Example:
//   aggregator := announce.NewAggregator()
//   aggregator.AddSource("dht", dhtSource)
//   results, err := aggregator.Aggregate(options)
func NewAggregator() *Aggregator {
	return &Aggregator{
		sources:      make(map[string]AggregatorSource),
		filters:      []AggregatorFilter{},
		transformers: []AggregatorTransformer{},
		deduper:      NewDeduplicator(DedupeByDescriptor, 24*time.Hour),
		cache:        NewAggregatorCache(5 * time.Minute),
		metrics:      NewAggregatorMetrics(),
	}
}

// AddSource registers a new announcement source with the aggregator.
//
// Sources provide announcements from different parts of the NoiseFS network
// and are queried in parallel during aggregation operations. Each source
// must have a unique name for identification and metrics tracking.
//
// Parameters:
//   - name: Unique identifier for this source (used in metrics and trust scoring)
//   - source: Implementation of AggregatorSource interface
//
// Time Complexity: O(1)
// Space Complexity: O(1)
//
// Thread Safety: Safe for concurrent use with other aggregator operations.
//
// Source Registration:
//   - Overwrites any existing source with the same name
//   - Initializes metrics tracking for the new source
//   - Source becomes available for immediate use in aggregation
//
// Example:
//   aggregator.AddSource("dht", dhtSource)
//   aggregator.AddSource("pubsub", pubsubSource)
func (a *Aggregator) AddSource(name string, source AggregatorSource) {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	a.sources[name] = source
	a.metrics.SourceMetrics[name] = &SourceMetrics{}
}

// AddFilter adds a filter to the aggregation pipeline.
//
// Filters are applied in the order they were added, with AND semantics
// (all filters must pass for an announcement to be included). Filters
// enable content selection based on topic, category, size, or custom criteria.
//
// Parameters:
//   - filter: Implementation of AggregatorFilter interface
//
// Time Complexity: O(1)
// Space Complexity: O(1)
//
// Thread Safety: Safe for concurrent use with other aggregator operations.
//
// Filter Pipeline:
//   - Filters are applied after source collection but before deduplication
//   - Order matters: filters are applied sequentially
//   - Failed filters increment the filtered metrics counter
//
// Example:
//   aggregator.AddFilter(NewCategoryFilter("video"))
//   aggregator.AddFilter(NewSizeFilter("large", "huge"))
func (a *Aggregator) AddFilter(filter AggregatorFilter) {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	a.filters = append(a.filters, filter)
}

// AddTransformer adds a transformer to the aggregation pipeline.
//
// Transformers modify announcements during aggregation, enabling content
// normalization, enhancement, or filtering. They are applied in the order
// they were added, after filtering but before deduplication.
//
// Parameters:
//   - transformer: Implementation of AggregatorTransformer interface
//
// Time Complexity: O(1)
// Space Complexity: O(1)
//
// Thread Safety: Safe for concurrent use with other aggregator operations.
//
// Transformation Pipeline:
//   - Applied after filtering but before deduplication
//   - Order matters: transformers are applied sequentially
//   - Transformers mark announcements as "Transformed" when modified
//
// Example:
//   aggregator.AddTransformer(NewTagNormalizer())
//   aggregator.AddTransformer(NewCategoryStandardizer())
func (a *Aggregator) AddTransformer(transformer AggregatorTransformer) {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	a.transformers = append(a.transformers, transformer)
}

// Aggregate performs comprehensive announcement aggregation from all configured sources.
//
// This method orchestrates the complete aggregation pipeline: parallel source
// collection, filtering, transformation, deduplication, scoring, ranking, and
// caching. It provides a unified view of content across the distributed NoiseFS network.
//
// Parameters:
//   - options: Configuration for aggregation behavior and constraints
//
// Returns:
//   - Ranked slice of aggregated announcements
//   - error if aggregation fails
//
// Time Complexity: O(n*log(n)) where n is total announcements (due to sorting)
// Space Complexity: O(n) for collected announcements and processing
//
// Aggregation Pipeline:
//   1. Cache lookup for identical queries
//   2. Parallel collection from all healthy sources
//   3. Filter application with metrics tracking
//   4. Content transformation with modification tracking
//   5. Deduplication using configured strategy
//   6. Relevance scoring with multiple factors
//   7. Result ranking and limit application
//   8. Cache storage for future queries
//
// Performance Features:
//   - Parallel source collection for optimal latency
//   - Result caching for repeated queries
//   - Unhealthy source skipping to prevent delays
//   - Comprehensive metrics for monitoring and optimization
func (a *Aggregator) Aggregate(options AggregationOptions) ([]*AggregatedAnnouncement, error) {
	// Check cache
	cacheKey := fmt.Sprintf("%+v", options)
	if cached := a.cache.Get(cacheKey); cached != nil {
		return cached, nil
	}
	
	a.mu.RLock()
	sources := a.sources
	filters := a.filters
	transformers := a.transformers
	a.mu.RUnlock()
	
	// Collect from all sources in parallel
	resultChan := make(chan *sourceResult, len(sources))
	
	for name, source := range sources {
		go a.collectFromSource(name, source, options, resultChan)
	}
	
	// Gather results
	allAnnouncements := []*AggregatedAnnouncement{}
	
	for i := 0; i < len(sources); i++ {
		result := <-resultChan
		if result.err != nil {
			a.metrics.RecordError(result.source, result.err)
			continue
		}
		
		for _, ann := range result.announcements {
			// Create aggregated announcement
			aggAnn := &AggregatedAnnouncement{
				Announcement: ann,
				Source:       result.source,
				Retrieved:    time.Now(),
			}
			
			// Apply filters
			if !a.passesFilters(aggAnn, filters) {
				a.metrics.IncrementFiltered()
				continue
			}
			
			// Apply transformers
			for _, transformer := range transformers {
				transformed := transformer.Transform(aggAnn.Announcement)
				if transformed != nil {
					aggAnn.Announcement = transformed
					aggAnn.Transformed = true
				}
			}
			
			// Deduplicate
			if a.deduper.IsDuplicate(aggAnn.Announcement) {
				a.metrics.IncrementDeduplicated()
				continue
			}
			
			allAnnouncements = append(allAnnouncements, aggAnn)
			a.metrics.IncrementProcessed()
		}
	}
	
	// Score and sort
	a.scoreAnnouncements(allAnnouncements, options)
	a.sortAnnouncements(allAnnouncements, options)
	
	// Apply limit
	if options.Limit > 0 && len(allAnnouncements) > options.Limit {
		allAnnouncements = allAnnouncements[:options.Limit]
	}
	
	// Cache results
	a.cache.Set(cacheKey, allAnnouncements)
	
	return allAnnouncements, nil
}

// AggregateByTopic performs topic-specific aggregation with temporary topic filtering.
//
// This convenience method adds temporary topic filtering to the aggregation
// pipeline, collects results matching the specified topics, then removes
// the temporary filter. It provides an efficient way to aggregate content
// for specific topics without permanent filter modification.
//
// Parameters:
//   - topics: List of topic strings to match (will be hashed for comparison)
//   - options: Standard aggregation configuration
//
// Returns:
//   - Ranked slice of topic-matching aggregated announcements
//   - error if aggregation fails
//
// Time Complexity: O(n*log(n)) where n is matching announcements
// Space Complexity: O(n) for collected and filtered announcements
//
// Topic Matching:
//   - Topics are SHA-256 hashed for privacy-preserving comparison
//   - Uses temporary filter that is automatically removed after aggregation
//   - Supports multiple topics with OR semantics (any topic matches)
//
// Example:
//   topics := []string{"media/movies", "media/tv"}
//   results, err := aggregator.AggregateByTopic(topics, options)
func (a *Aggregator) AggregateByTopic(topics []string, options AggregationOptions) ([]*AggregatedAnnouncement, error) {
	// Add topic filter
	topicFilter := NewTopicFilter(topics)
	
	a.mu.Lock()
	a.filters = append(a.filters, topicFilter)
	a.mu.Unlock()
	
	// Aggregate
	results, err := a.Aggregate(options)
	
	// Remove temporary filter
	a.mu.Lock()
	a.filters = a.filters[:len(a.filters)-1]
	a.mu.Unlock()
	
	return results, err
}

// GetMetrics returns a snapshot of current aggregator performance metrics.
//
// This method provides comprehensive metrics including processing statistics,
// source health information, and performance indicators. The returned metrics
// are a defensive copy to prevent external modification of internal state.
//
// Returns:
//   Complete AggregatorMetrics snapshot with all current statistics
//
// Time Complexity: O(n) where n is the number of sources (for copying)
// Space Complexity: O(n) for the metrics copy
//
// Thread Safety: Returns an independent copy safe for concurrent access.
//
// Metrics Included:
//   - Total processed, filtered, and deduplicated counts
//   - Per-source retrieval and error statistics
//   - Source health and last update information
//   - Error details for troubleshooting
//
// Use Cases:
//   - Performance monitoring and alerting
//   - Source health assessment
//   - System optimization and tuning
//   - Debugging aggregation issues
func (a *Aggregator) GetMetrics() AggregatorMetrics {
	return *a.metrics.Copy()
}

// Helper methods

func (a *Aggregator) collectFromSource(name string, source AggregatorSource, options AggregationOptions, resultChan chan<- *sourceResult) {
	result := &sourceResult{source: name}
	
	// Check health
	if !source.IsHealthy() {
		result.err = fmt.Errorf("source unhealthy")
		resultChan <- result
		return
	}
	
	// Get announcements
	since := time.Now().Add(-options.TimeWindow)
	announcements, err := source.GetAnnouncements(since, options.MaxPerSource)
	if err != nil {
		result.err = err
		resultChan <- result
		return
	}
	
	result.announcements = announcements
	a.metrics.UpdateSource(name, len(announcements))
	resultChan <- result
}

func (a *Aggregator) passesFilters(ann *AggregatedAnnouncement, filters []AggregatorFilter) bool {
	for _, filter := range filters {
		if !filter.Filter(ann.Announcement) {
			return false
		}
	}
	return true
}

func (a *Aggregator) scoreAnnouncements(announcements []*AggregatedAnnouncement, options AggregationOptions) {
	for _, ann := range announcements {
		score := 1.0
		
		// Recency score
		age := time.Since(time.Unix(ann.Timestamp, 0))
		if age < 1*time.Hour {
			score *= 2.0
		} else if age < 24*time.Hour {
			score *= 1.5
		} else if age > 7*24*time.Hour {
			score *= 0.5
		}
		
		// Source trust score
		if trust, ok := options.SourceTrust[ann.Source]; ok {
			score *= trust
		}
		
		// Size preference
		if options.PreferLargeFiles && ann.SizeClass == "large" || ann.SizeClass == "huge" {
			score *= 1.2
		}
		
		ann.Score = score
	}
}

func (a *Aggregator) sortAnnouncements(announcements []*AggregatedAnnouncement, options AggregationOptions) {
	sort.Slice(announcements, func(i, j int) bool {
		switch options.SortBy {
		case "time":
			return announcements[i].Timestamp > announcements[j].Timestamp
		case "source":
			return announcements[i].Source < announcements[j].Source
		default: // score
			return announcements[i].Score > announcements[j].Score
		}
	})
}

// Deduplicator implementation

// NewDeduplicator creates a new deduplicator with the specified strategy and TTL.
//
// The deduplicator automatically starts a background cleanup goroutine to
// prevent memory growth by removing expired entries. Different strategies
// provide different levels of duplicate detection precision.
//
// Parameters:
//   - strategy: Deduplication algorithm (descriptor/content/similarity-based)
//   - ttl: How long to remember seen announcements
//
// Returns:
//   A new Deduplicator with automatic cleanup enabled
//
// Time Complexity: O(1)
// Space Complexity: O(1) initially, grows with unique announcements
//
// Cleanup Behavior:
//   - Background goroutine runs hourly cleanup
//   - Removes entries older than TTL to prevent memory leaks
//   - Cleanup continues until deduplicator is garbage collected
func NewDeduplicator(strategy DedupeStrategy, ttl time.Duration) *Deduplicator {
	d := &Deduplicator{
		strategy: strategy,
		seen:     make(map[string]time.Time),
		ttl:      ttl,
	}
	
	// Start cleanup
	go d.cleanup()
	
	return d
}

// IsDuplicate determines if an announcement is a duplicate based on the configured strategy.
//
// This method checks if the announcement has been seen recently (within TTL)
// and records it for future duplicate detection. The specific criteria for
// "duplicate" depends on the deduplication strategy.
//
// Parameters:
//   - ann: Announcement to check for duplication
//
// Returns:
//   - true if this announcement is a duplicate
//   - false if this is the first occurrence or outside TTL window
//
// Time Complexity: O(1) average case for map operations
// Space Complexity: O(1) per new announcement
//
// Thread Safety: Safe for concurrent use across multiple goroutines.
//
// Side Effects:
//   - Records the announcement for future duplicate detection
//   - Updates the "last seen" timestamp for existing announcements
func (d *Deduplicator) IsDuplicate(ann *Announcement) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	
	key := d.getKey(ann)
	
	if lastSeen, exists := d.seen[key]; exists {
		if time.Since(lastSeen) < d.ttl {
			return true
		}
	}
	
	d.seen[key] = time.Now()
	return false
}

func (d *Deduplicator) getKey(ann *Announcement) string {
	switch d.strategy {
	case DedupeByContent:
		// Hash of content fields
		return fmt.Sprintf("%s:%s:%s:%s", ann.Descriptor, ann.TopicHash, ann.Category, ann.SizeClass)
	case DedupeBySimilarity:
		// Simplified similarity key
		return fmt.Sprintf("%s:%s", ann.Descriptor, ann.Category)
	default: // DedupeByDescriptor
		return ann.Descriptor
	}
}

func (d *Deduplicator) cleanup() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	
	for range ticker.C {
		d.mu.Lock()
		now := time.Now()
		for key, lastSeen := range d.seen {
			if now.Sub(lastSeen) > d.ttl {
				delete(d.seen, key)
			}
		}
		d.mu.Unlock()
	}
}

// Cache implementation

// NewAggregatorCache creates a new cache with the specified TTL.
//
// The cache provides performance optimization by storing aggregated results
// for frequently repeated queries. Results expire after the TTL to ensure
// freshness while providing significant performance benefits.
//
// Parameters:
//   - ttl: How long cached results remain valid
//
// Returns:
//   A new AggregatorCache ready for use
//
// Time Complexity: O(1)
// Space Complexity: O(1) initially, grows with cached queries
//
// Cache Behavior:
//   - Automatic expiration based on TTL
//   - Thread-safe concurrent access
//   - Query-specific cache keys for precise matching
func NewAggregatorCache(ttl time.Duration) *AggregatorCache {
	return &AggregatorCache{
		results: make(map[string]*cachedResult),
		ttl:     ttl,
	}
}

// Get retrieves cached aggregation results if available and not expired.
//
// This method performs cache lookup with automatic expiration checking.
// Expired entries are treated as cache misses but remain in memory until
// manual cleanup (future enhancement opportunity).
//
// Parameters:
//   - key: Cache key generated from aggregation options
//
// Returns:
//   - Cached results if available and fresh
//   - nil if cache miss or expired
//
// Time Complexity: O(1) for cache lookup
// Space Complexity: O(1)
//
// Thread Safety: Safe for concurrent use with writes.
func (c *AggregatorCache) Get(key string) []*AggregatedAnnouncement {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	if cached, exists := c.results[key]; exists {
		if time.Since(cached.timestamp) < c.ttl {
			return cached.announcements
		}
	}
	
	return nil
}

// Set stores aggregation results in the cache with current timestamp.
//
// This method stores the provided results with the current timestamp
// for future TTL-based expiration checking. The cache grows unbounded
// until manual cleanup (future enhancement opportunity).
//
// Parameters:
//   - key: Cache key generated from aggregation options
//   - announcements: Aggregated results to cache
//
// Time Complexity: O(1) for cache storage
// Space Complexity: O(n) where n is the number of announcements
//
// Thread Safety: Safe for concurrent use with reads.
func (c *AggregatorCache) Set(key string, announcements []*AggregatedAnnouncement) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.results[key] = &cachedResult{
		announcements: announcements,
		timestamp:     time.Now(),
	}
}

// Metrics implementation

// NewAggregatorMetrics creates a new metrics tracker with initialized counters.
//
// The metrics system provides comprehensive monitoring of aggregation
// performance, source health, and system behavior. All counters start
// at zero and source metrics are created on-demand.
//
// Returns:
//   A new AggregatorMetrics ready for tracking
//
// Time Complexity: O(1)
// Space Complexity: O(1) initially, grows with sources
//
// Metrics Tracking:
//   - Processing volume and throughput
//   - Filter effectiveness and content selection
//   - Deduplication effectiveness and overlap
//   - Per-source performance and error rates
func NewAggregatorMetrics() *AggregatorMetrics {
	return &AggregatorMetrics{
		SourceMetrics: make(map[string]*SourceMetrics),
	}
}

func (m *AggregatorMetrics) IncrementProcessed() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TotalProcessed++
}

func (m *AggregatorMetrics) IncrementFiltered() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TotalFiltered++
}

func (m *AggregatorMetrics) IncrementDeduplicated() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TotalDeduplicated++
}

func (m *AggregatorMetrics) UpdateSource(name string, count int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, exists := m.SourceMetrics[name]; !exists {
		m.SourceMetrics[name] = &SourceMetrics{}
	}
	
	m.SourceMetrics[name].Retrieved += int64(count)
	m.SourceMetrics[name].LastUpdate = time.Now()
}

func (m *AggregatorMetrics) RecordError(source string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, exists := m.SourceMetrics[source]; !exists {
		m.SourceMetrics[source] = &SourceMetrics{}
	}
	
	m.SourceMetrics[source].Errors++
	m.SourceMetrics[source].LastError = err.Error()
}

func (m *AggregatorMetrics) Copy() *AggregatorMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	copy := &AggregatorMetrics{
		TotalProcessed:    m.TotalProcessed,
		TotalFiltered:     m.TotalFiltered,
		TotalDeduplicated: m.TotalDeduplicated,
		SourceMetrics:     make(map[string]*SourceMetrics),
	}
	
	for name, metrics := range m.SourceMetrics {
		copy.SourceMetrics[name] = &SourceMetrics{
			Retrieved:  metrics.Retrieved,
			Errors:     metrics.Errors,
			LastError:  metrics.LastError,
			LastUpdate: metrics.LastUpdate,
		}
	}
	
	return copy
}

// Built-in filters

// TopicFilter provides efficient topic-based announcement filtering.
//
// This filter matches announcements against a set of allowed topics using
// SHA-256 topic hashes for privacy-preserving comparison. It provides O(1)
// lookup performance for topic matching in large-scale aggregation scenarios.
type TopicFilter struct {
	// topics maps SHA-256 topic hashes to boolean inclusion flags.
	// Pre-computed hashes enable efficient announcement filtering.
	topics map[string]bool
}

// NewTopicFilter creates a topic filter for the specified topic list.
//
// This constructor pre-computes SHA-256 hashes for all provided topics
// to enable efficient O(1) filtering during aggregation. Topics are
// normalized and hashed using the same algorithm as announcements.
//
// Parameters:
//   - topics: List of topic strings to allow through the filter
//
// Returns:
//   A new TopicFilter ready for use in aggregation pipelines
//
// Time Complexity: O(n) where n is the number of topics
// Space Complexity: O(n) for the hash map
//
// Topic Processing:
//   - Topics are normalized using standard topic normalization
//   - SHA-256 hashes computed for privacy-preserving comparison
//   - Hash map provides O(1) lookup during filtering
func NewTopicFilter(topics []string) *TopicFilter {
	topicMap := make(map[string]bool)
	for _, topic := range topics {
		topicMap[HashTopic(topic)] = true
	}
	return &TopicFilter{topics: topicMap}
}

func (f *TopicFilter) Filter(ann *Announcement) bool {
	return f.topics[ann.TopicHash]
}

func (f *TopicFilter) GetName() string {
	return "topic_filter"
}

// Types

type sourceResult struct {
	source        string
	announcements []*Announcement
	err           error
}

// AggregationOptions configures aggregation behavior and performance constraints.
//
// These options control various aspects of the aggregation process including
// time windows, limits, scoring preferences, and source trust relationships.
// They enable fine-tuning of aggregation behavior for different use cases.
type AggregationOptions struct {
	// TimeWindow limits how far back to look for announcements.
	// Only announcements newer than (now - TimeWindow) are included.
	TimeWindow       time.Duration
	
	// MaxPerSource limits announcements retrieved from each source.
	// Prevents any single source from dominating results (0 = no limit).
	MaxPerSource     int
	
	// Limit sets maximum announcements to return in final results.
	// Applied after scoring and sorting (0 = no limit).
	Limit            int
	
	// SortBy determines result ordering: "score" (default), "time", or "source".
	// Score-based sorting provides relevance ranking.
	SortBy           string
	
	// PreferLargeFiles boosts scores for large and huge size classes.
	// Useful for discovery scenarios favoring substantial content.
	PreferLargeFiles bool
	
	// SourceTrust maps source names to trust multipliers (0.0-∞).
	// Higher values boost scores from trusted sources.
	SourceTrust      map[string]float64
}
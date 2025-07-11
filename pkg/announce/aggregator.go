package announce

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// Aggregator aggregates announcements from multiple sources
type Aggregator struct {
	sources      map[string]AggregatorSource
	filters      []AggregatorFilter
	transformers []AggregatorTransformer
	
	// Deduplication
	deduper      *Deduplicator
	
	// Caching
	cache        *AggregatorCache
	
	// Metrics
	metrics      *AggregatorMetrics
	
	mu           sync.RWMutex
}

// AggregatorSource represents a source of announcements
type AggregatorSource interface {
	GetName() string
	GetAnnouncements(since time.Time, limit int) ([]*Announcement, error)
	IsHealthy() bool
}

// AggregatorFilter filters announcements
type AggregatorFilter interface {
	Filter(ann *Announcement) bool
	GetName() string
}

// AggregatorTransformer transforms announcements
type AggregatorTransformer interface {
	Transform(ann *Announcement) *Announcement
	GetName() string
}

// Deduplicator handles announcement deduplication
type Deduplicator struct {
	strategy DedupeStrategy
	seen     map[string]time.Time
	ttl      time.Duration
	mu       sync.RWMutex
}

// DedupeStrategy defines how to deduplicate
type DedupeStrategy int

const (
	DedupeByDescriptor DedupeStrategy = iota
	DedupeByContent
	DedupeBySimilarity
)

// AggregatorCache caches aggregated results
type AggregatorCache struct {
	results map[string]*cachedResult
	ttl     time.Duration
	mu      sync.RWMutex
}

type cachedResult struct {
	announcements []*AggregatedAnnouncement
	timestamp     time.Time
}

// AggregatorMetrics tracks aggregation metrics
type AggregatorMetrics struct {
	TotalProcessed   int64
	TotalFiltered    int64
	TotalDeduplicated int64
	SourceMetrics    map[string]*SourceMetrics
	mu               sync.RWMutex
}

// SourceMetrics tracks per-source metrics
type SourceMetrics struct {
	Retrieved  int64
	Errors     int64
	LastError  string
	LastUpdate time.Time
}

// AggregatedAnnouncement wraps an announcement with metadata
type AggregatedAnnouncement struct {
	*Announcement
	Source      string
	Retrieved   time.Time
	Transformed bool
	Score       float64
}

// NewAggregator creates a new aggregator
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

// AddSource adds an announcement source
func (a *Aggregator) AddSource(name string, source AggregatorSource) {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	a.sources[name] = source
	a.metrics.SourceMetrics[name] = &SourceMetrics{}
}

// AddFilter adds a filter
func (a *Aggregator) AddFilter(filter AggregatorFilter) {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	a.filters = append(a.filters, filter)
}

// AddTransformer adds a transformer
func (a *Aggregator) AddTransformer(transformer AggregatorTransformer) {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	a.transformers = append(a.transformers, transformer)
}

// Aggregate aggregates announcements from all sources
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

// AggregateByTopic aggregates announcements for specific topics
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

// GetMetrics returns aggregator metrics
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

func NewAggregatorCache(ttl time.Duration) *AggregatorCache {
	return &AggregatorCache{
		results: make(map[string]*cachedResult),
		ttl:     ttl,
	}
}

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

func (c *AggregatorCache) Set(key string, announcements []*AggregatedAnnouncement) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.results[key] = &cachedResult{
		announcements: announcements,
		timestamp:     time.Now(),
	}
}

// Metrics implementation

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

// TopicFilter filters by topic
type TopicFilter struct {
	topics map[string]bool
}

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

// AggregationOptions configures aggregation
type AggregationOptions struct {
	TimeWindow       time.Duration
	MaxPerSource     int
	Limit            int
	SortBy           string // "score", "time", "source"
	PreferLargeFiles bool
	SourceTrust      map[string]float64 // source -> trust score
}
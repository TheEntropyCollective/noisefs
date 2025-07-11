package announce

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// CrossTopicDiscovery enables discovery across multiple related topics
type CrossTopicDiscovery struct {
	hierarchy   *TopicHierarchy
	subscribers map[string]TopicSubscriber // topic hash -> subscriber
	
	// Discovery rules
	rules       []DiscoveryRule
	
	// Caching
	cache       map[string]*discoveryCache
	cacheTTL    time.Duration
	
	mu          sync.RWMutex
}

// TopicSubscriber interface for topic-specific subscribers
type TopicSubscriber interface {
	GetAnnouncements(since time.Time, limit int) ([]*Announcement, error)
	GetTopicHash() string
}

// DiscoveryRule defines how to discover across topics
type DiscoveryRule struct {
	Name        string
	Description string
	Matcher     func(sourceTopic string) []string // Returns related topic paths
	Weight      float64 // Relevance weight
}

// discoveryCache caches discovery results
type discoveryCache struct {
	results    []*DiscoveryResult
	timestamp  time.Time
}

// DiscoveryResult represents a cross-topic discovery result
type DiscoveryResult struct {
	Announcement *Announcement
	SourceTopic  string   // Original topic path
	MatchedVia   []string // How it was discovered
	Relevance    float64  // Relevance score
}

// NewCrossTopicDiscovery creates a new cross-topic discovery system
func NewCrossTopicDiscovery(hierarchy *TopicHierarchy, cacheTTL time.Duration) *CrossTopicDiscovery {
	return &CrossTopicDiscovery{
		hierarchy:   hierarchy,
		subscribers: make(map[string]TopicSubscriber),
		rules:       getDefaultDiscoveryRules(),
		cache:       make(map[string]*discoveryCache),
		cacheTTL:    cacheTTL,
	}
}

// RegisterSubscriber registers a topic subscriber
func (ctd *CrossTopicDiscovery) RegisterSubscriber(topicPath string, subscriber TopicSubscriber) error {
	ctd.mu.Lock()
	defer ctd.mu.Unlock()
	
	node, exists := ctd.hierarchy.GetTopic(topicPath)
	if !exists {
		return fmt.Errorf("topic not found: %s", topicPath)
	}
	
	ctd.subscribers[node.Hash] = subscriber
	return nil
}

// DiscoverRelated discovers announcements from related topics
func (ctd *CrossTopicDiscovery) DiscoverRelated(topicPath string, options DiscoveryOptions) ([]*DiscoveryResult, error) {
	// Check cache first
	cacheKey := fmt.Sprintf("%s:%+v", topicPath, options)
	if cached := ctd.getFromCache(cacheKey); cached != nil {
		return cached, nil
	}
	
	ctd.mu.RLock()
	defer ctd.mu.RUnlock()
	
	// Get source topic
	sourceTopic, exists := ctd.hierarchy.GetTopic(topicPath)
	if !exists {
		return nil, fmt.Errorf("topic not found: %s", topicPath)
	}
	
	// Collect related topics based on rules
	relatedTopics := ctd.collectRelatedTopics(topicPath, options)
	
	// Gather announcements from related topics
	results := []*DiscoveryResult{}
	seen := make(map[string]bool) // Dedup by descriptor
	
	for _, related := range relatedTopics {
		topicNode, exists := ctd.hierarchy.GetTopic(related.Path)
		if !exists {
			continue
		}
		
		subscriber, exists := ctd.subscribers[topicNode.Hash]
		if !exists {
			continue
		}
		
		// Get announcements from this topic
		announcements, err := subscriber.GetAnnouncements(
			time.Now().Add(-options.TimeWindow),
			options.MaxPerTopic,
		)
		if err != nil {
			continue // Skip failed topics
		}
		
		// Process announcements
		for _, ann := range announcements {
			if seen[ann.Descriptor] {
				continue
			}
			seen[ann.Descriptor] = true
			
			// Apply filters
			if !ctd.matchesFilters(ann, options) {
				continue
			}
			
			// Calculate relevance
			relevance := ctd.calculateRelevance(ann, sourceTopic, topicNode, related.Weight)
			
			result := &DiscoveryResult{
				Announcement: ann,
				SourceTopic:  related.Path,
				MatchedVia:   related.MatchedVia,
				Relevance:    relevance,
			}
			
			results = append(results, result)
		}
	}
	
	// Sort by relevance
	sort.Slice(results, func(i, j int) bool {
		return results[i].Relevance > results[j].Relevance
	})
	
	// Apply limit
	if options.MaxResults > 0 && len(results) > options.MaxResults {
		results = results[:options.MaxResults]
	}
	
	// Cache results
	ctd.saveToCache(cacheKey, results)
	
	return results, nil
}

// DiscoverAcrossHierarchy discovers announcements across the hierarchy
func (ctd *CrossTopicDiscovery) DiscoverAcrossHierarchy(startTopic string, depth int, options DiscoveryOptions) ([]*DiscoveryResult, error) {
	ctd.mu.RLock()
	defer ctd.mu.RUnlock()
	
	// Get starting topic
	startNode, exists := ctd.hierarchy.GetTopic(startTopic)
	if !exists {
		return nil, fmt.Errorf("topic not found: %s", startTopic)
	}
	
	// Collect topics to search
	topicsToSearch := []*TopicNode{startNode}
	
	// Add ancestors up to depth
	ancestors, _ := ctd.hierarchy.GetAncestors(startTopic)
	for i, ancestor := range ancestors {
		if i >= depth {
			break
		}
		topicsToSearch = append(topicsToSearch, ancestor)
	}
	
	// Add descendants up to depth
	descendants, _ := ctd.hierarchy.GetDescendants(startTopic)
	currentDepth := GetTopicDepth(startTopic)
	for _, desc := range descendants {
		if GetTopicDepth(desc.Path) - currentDepth <= depth {
			topicsToSearch = append(topicsToSearch, desc)
		}
	}
	
	// Gather announcements
	results := []*DiscoveryResult{}
	seen := make(map[string]bool)
	
	for _, topic := range topicsToSearch {
		subscriber, exists := ctd.subscribers[topic.Hash]
		if !exists {
			continue
		}
		
		announcements, err := subscriber.GetAnnouncements(
			time.Now().Add(-options.TimeWindow),
			options.MaxPerTopic,
		)
		if err != nil {
			continue
		}
		
		for _, ann := range announcements {
			if seen[ann.Descriptor] {
				continue
			}
			seen[ann.Descriptor] = true
			
			if !ctd.matchesFilters(ann, options) {
				continue
			}
			
			// Calculate hierarchical distance
			distance := ctd.calculateHierarchicalDistance(startNode, topic)
			relevance := 1.0 / (1.0 + float64(distance))
			
			result := &DiscoveryResult{
				Announcement: ann,
				SourceTopic:  topic.Path,
				MatchedVia:   []string{"hierarchy"},
				Relevance:    relevance,
			}
			
			results = append(results, result)
		}
	}
	
	// Sort by relevance
	sort.Slice(results, func(i, j int) bool {
		return results[i].Relevance > results[j].Relevance
	})
	
	// Apply limit
	if options.MaxResults > 0 && len(results) > options.MaxResults {
		results = results[:options.MaxResults]
	}
	
	return results, nil
}

// Helper methods

func (ctd *CrossTopicDiscovery) collectRelatedTopics(topicPath string, options DiscoveryOptions) []relatedTopic {
	related := []relatedTopic{}
	seen := make(map[string]bool)
	
	// Apply discovery rules
	for _, rule := range ctd.rules {
		if !options.shouldUseRule(rule.Name) {
			continue
		}
		
		relatedPaths := rule.Matcher(topicPath)
		for _, path := range relatedPaths {
			if seen[path] {
				continue
			}
			seen[path] = true
			
			related = append(related, relatedTopic{
				Path:       path,
				Weight:     rule.Weight,
				MatchedVia: []string{rule.Name},
			})
		}
	}
	
	return related
}

func (ctd *CrossTopicDiscovery) matchesFilters(ann *Announcement, options DiscoveryOptions) bool {
	// Category filter
	if len(options.Categories) > 0 {
		found := false
		for _, cat := range options.Categories {
			if ann.Category == cat {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	
	// Tag filter
	if len(options.RequiredTags) > 0 && ann.TagBloom != "" {
		matches, _, err := MatchesTags(ann.TagBloom, options.RequiredTags)
		if err != nil || !matches {
			return false
		}
	}
	
	// Size filter
	if len(options.SizeClasses) > 0 {
		found := false
		for _, size := range options.SizeClasses {
			if ann.SizeClass == size {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	
	return true
}

func (ctd *CrossTopicDiscovery) calculateRelevance(ann *Announcement, source, target *TopicNode, baseWeight float64) float64 {
	relevance := baseWeight
	
	// Boost if categories match
	if ann.Category == source.Metadata["type"] {
		relevance *= 1.2
	}
	
	// Distance penalty
	distance := ctd.calculateHierarchicalDistance(source, target)
	relevance *= (1.0 / (1.0 + float64(distance)*0.1))
	
	// Recency bonus
	age := time.Since(time.Unix(ann.Timestamp, 0))
	if age < 24*time.Hour {
		relevance *= 1.5
	} else if age < 7*24*time.Hour {
		relevance *= 1.2
	}
	
	return relevance
}

func (ctd *CrossTopicDiscovery) calculateHierarchicalDistance(a, b *TopicNode) int {
	// Find common ancestor
	aAncestors := map[string]int{}
	current := a
	depth := 0
	
	for current != nil {
		aAncestors[current.Path] = depth
		current = current.Parent
		depth++
	}
	
	// Find first common ancestor from b
	current = b
	depth = 0
	
	for current != nil {
		if aDepth, found := aAncestors[current.Path]; found {
			return aDepth + depth
		}
		current = current.Parent
		depth++
	}
	
	return 999 // No common ancestor
}

// Cache management

func (ctd *CrossTopicDiscovery) getFromCache(key string) []*DiscoveryResult {
	ctd.mu.RLock()
	defer ctd.mu.RUnlock()
	
	cached, exists := ctd.cache[key]
	if !exists {
		return nil
	}
	
	if time.Since(cached.timestamp) > ctd.cacheTTL {
		delete(ctd.cache, key)
		return nil
	}
	
	return cached.results
}

func (ctd *CrossTopicDiscovery) saveToCache(key string, results []*DiscoveryResult) {
	ctd.mu.Lock()
	defer ctd.mu.Unlock()
	
	ctd.cache[key] = &discoveryCache{
		results:   results,
		timestamp: time.Now(),
	}
}

// Types

type relatedTopic struct {
	Path       string
	Weight     float64
	MatchedVia []string
}

// DiscoveryOptions configures cross-topic discovery
type DiscoveryOptions struct {
	TimeWindow    time.Duration
	MaxResults    int
	MaxPerTopic   int
	Categories    []string
	SizeClasses   []string
	RequiredTags  []string
	EnabledRules  []string // Empty means all rules
}

func (do DiscoveryOptions) shouldUseRule(ruleName string) bool {
	if len(do.EnabledRules) == 0 {
		return true
	}
	
	for _, enabled := range do.EnabledRules {
		if enabled == ruleName {
			return true
		}
	}
	return false
}

// Default discovery rules

func getDefaultDiscoveryRules() []DiscoveryRule {
	return []DiscoveryRule{
		{
			Name:        "siblings",
			Description: "Topics with same parent",
			Weight:      0.8,
			Matcher: func(topic string) []string {
				parent := GetTopicParent(topic)
				if parent == "" {
					return []string{}
				}
				// In real implementation, would get siblings from hierarchy
				return []string{}
			},
		},
		{
			Name:        "parent",
			Description: "Parent topic",
			Weight:      0.7,
			Matcher: func(topic string) []string {
				parent := GetTopicParent(topic)
				if parent == "" {
					return []string{}
				}
				return []string{parent}
			},
		},
		{
			Name:        "children",
			Description: "Child topics",
			Weight:      0.6,
			Matcher: func(topic string) []string {
				// In real implementation, would get children from hierarchy
				return []string{}
			},
		},
		{
			Name:        "semantic",
			Description: "Semantically related topics",
			Weight:      0.5,
			Matcher: func(topic string) []string {
				// Hardcoded semantic relationships
				related := map[string][]string{
					"media/movies/scifi": {"media/tv/scifi", "media/books/scifi"},
					"media/movies/action": {"media/games/action"},
					"media/music/rock": {"media/music/metal", "media/music/punk"},
				}
				
				if rels, ok := related[topic]; ok {
					return rels
				}
				return []string{}
			},
		},
	}
}
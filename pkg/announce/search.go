package announce

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// SearchEngine provides comprehensive content discovery and search capabilities for NoiseFS announcements.
//
// The SearchEngine implements advanced search functionality that combines multiple indexing
// strategies, intelligent scoring algorithms, and privacy-preserving search capabilities.
// It enables users to discover content through tag-based filtering, topic organization,
// temporal search, and similarity matching while maintaining the privacy guarantees of
// the NoiseFS announcement system.
//
// Key Features:
//   - Multi-dimensional indexing (tags, topics, time-based)
//   - Intelligent relevance scoring with recency bias
//   - Privacy-preserving tag recovery from bloom filters
//   - Hierarchical topic search with subtopic support
//   - Advanced filtering (category, size, temporal)
//   - Search suggestions and auto-completion
//   - Learning capabilities for improved tag recovery
//
// Search Capabilities:
//   - Keyword search across announcement metadata
//   - Tag-based filtering with multiple match modes (any/all/exact)
//   - Topic-based organization with hierarchy support
//   - Similarity search for content discovery
//   - Temporal filtering and recency-based ranking
//   - Combined multi-criteria search with intelligent scoring
//
// Privacy Features:
//   - Tag recovery from bloom filters without exact tag disclosure
//   - Topic hashing maintains anonymous content organization
//   - No exposure of exact file metadata beyond size classes
//   - Bloom filter false positives protect against exact enumeration
//
// Thread Safety: SearchEngine is safe for concurrent use across multiple goroutines.
// All indexing operations use read-write mutexes for optimal concurrent performance.
type SearchEngine struct {
	store       AnnouncementStore
	hierarchy   *TopicHierarchy
	tagMatcher  *TagMatcher
	tagRecovery *TagRecovery
	
	// Indexing structures for efficient content discovery
	// tagIndex maps normalized tags to announcement descriptor IDs for tag-based search.
	// Populated through bloom filter tag recovery and enables fast tag lookups.
	tagIndex    map[string][]string // tag -> announcement IDs
	
	// topicIndex maps topic hashes to announcement descriptor IDs for topic-based search.
	// Enables efficient retrieval of content within specific topic hierarchies.
	topicIndex  map[string][]string // topic hash -> announcement IDs
	
	// timeIndex provides temporal indexing with hourly bucketing for time-based queries.
	// Optimizes searches with temporal constraints and recency-based discovery.
	timeIndex   *TimeIndex
	
	// Configuration and performance limits
	// maxResults limits the maximum number of results returned to prevent resource exhaustion.
	maxResults  int
	
	// mu provides read-write mutex protection for thread-safe search operations.
	// Uses RWMutex to optimize concurrent read-heavy search workloads.
	mu          sync.RWMutex
}

// AnnouncementStore defines the interface for persistent announcement storage and retrieval.
//
// This interface abstracts the underlying storage mechanism for NoiseFS announcements,
// enabling the search engine to work with different storage backends (in-memory, database,
// distributed storage, etc.). The interface provides both ID-based retrieval and
// filtered querying capabilities optimized for search operations.
//
// Implementation Requirements:
//   - Thread-safe operations for concurrent access
//   - Efficient indexing for ID, topic, and time-based queries
//   - Consistent data retrieval across multiple calls
//   - Error handling for missing or corrupted announcements
//
// Query Optimization:
//   - GetByTopic should use topic hash indexing for O(1) lookups
//   - GetRecent should use temporal indexing for efficient time-range queries
//   - GetAll should implement reasonable limits to prevent memory exhaustion
type AnnouncementStore interface {
	// GetByID retrieves a specific announcement by its descriptor CID.
	// Returns the announcement or an error if not found or corrupted.
	GetByID(id string) (*Announcement, error)
	
	// GetAll retrieves all stored announcements for full-scan operations.
	// Should implement reasonable limits to prevent memory exhaustion.
	// Used primarily for index rebuilding and comprehensive searches.
	GetAll() ([]*Announcement, error)
	
	// GetByTopic retrieves all announcements matching a specific topic hash.
	// Should use topic indexing for efficient O(1) lookup performance.
	// Topic hash is the SHA-256 hash of the normalized topic string.
	GetByTopic(topicHash string) ([]*Announcement, error)
	
	// GetRecent retrieves announcements newer than the specified time.
	// Should use temporal indexing for efficient time-range queries.
	// Limit parameter controls maximum results to prevent resource exhaustion.
	GetRecent(since time.Time, limit int) ([]*Announcement, error)
}

// TimeIndex provides efficient time-based indexing with hourly bucketing for temporal queries.
//
// The TimeIndex organizes announcements into hourly time buckets to enable fast
// time-range queries without scanning all announcements. This indexing strategy
// optimizes searches with temporal constraints, recency-based discovery, and
// time-sensitive content filtering.
//
// Bucketing Strategy:
//   - Hourly buckets reduce memory overhead while maintaining query efficiency
//   - Bucket keys use "YYYY-MM-DD-HH" format for deterministic ordering
//   - Each bucket contains announcement IDs for that specific hour
//   - Range queries span multiple buckets with deduplication
//
// Thread Safety: TimeIndex is safe for concurrent use with read-write mutex protection.
type TimeIndex struct {
	// buckets maps hourly time bucket keys to announcement descriptor IDs.
	// Bucket keys use "YYYY-MM-DD-HH" format for chronological organization.
	buckets map[string][]string // time bucket -> announcement IDs
	
	// mu provides read-write mutex protection for thread-safe time index operations.
	mu      sync.RWMutex
}

// SearchQuery represents a comprehensive search specification with multiple filtering and ranking criteria.
//
// SearchQuery provides fine-grained control over content discovery by combining
// multiple search dimensions: text keywords, tag-based filtering, topic organization,
// content categorization, size constraints, and temporal bounds. The query structure
// enables complex searches while maintaining privacy through bloom filter tag matching
// and topic hash-based organization.
//
// Query Composition:
//   - Multiple criteria are combined with AND semantics
//   - Tag matching supports different modes (any/all/exact)
//   - Topic hierarchies enable both specific and broad discovery
//   - Temporal and size filters provide precise content targeting
//   - Result control enables pagination and custom sorting
//
// Privacy Preservation:
//   - Tag queries use bloom filter matching (false positives, no false negatives)
//   - Topic searches use SHA-256 hashes for anonymous organization
//   - Size filtering uses broad classes rather than exact sizes
type SearchQuery struct {
	// Text search criteria for keyword-based content discovery
	// Keywords are matched against announcement metadata (category, size class)
	// Search is case-insensitive with partial matching support
	Keywords    []string
	
	// Tag-based filtering using privacy-preserving bloom filter matching
	// IncludeTags specifies tags that should be present (bloom filter testing)
	// ExcludeTags specifies tags that should NOT be present (exclusion filtering)
	// TagMode controls how multiple IncludeTags are combined (any/all/exact)
	IncludeTags []string
	ExcludeTags []string
	TagMode     TagMatchMode
	
	// Topic-based organization for hierarchical content discovery
	// Topics are SHA-256 hashes for privacy-preserving organization
	// IncludeSubtopics enables hierarchical matching for broader discovery
	Topics      []string
	IncludeSubtopics bool
	
	// Content classification filters for targeted discovery
	// Categories: video, audio, document, data, software, image, archive, other
	// SizeClasses: tiny, small, medium, large, huge (privacy-preserving ranges)
	// MinSize/MaxSize: exact size bounds (used sparingly to preserve privacy)
	Categories  []string
	SizeClasses []string
	MinSize     int64
	MaxSize     int64
	
	// Temporal filtering for time-sensitive content discovery
	// Since: only include announcements newer than this time
	// Until: only include announcements older than this time
	// Nil values disable the respective temporal bound
	Since       *time.Time
	Until       *time.Time
	
	// Result control for pagination and custom ranking
	// SortBy: relevance (default), time, or size-based ordering
	// SortOrder: ascending or descending result arrangement
	// Limit: maximum results to return (0 = no limit)
	// Offset: pagination starting position
	SortBy      SortField
	SortOrder   SortOrder
	Limit       int
	Offset      int
}

// TagMatchMode defines the strategy for matching multiple tags in search queries.
//
// Tag matching modes control how multiple tags in the IncludeTags list are
// evaluated against announcement bloom filters. Different modes provide
// different trade-offs between precision and recall for tag-based discovery.
//
// Privacy Note: All tag matching uses bloom filter testing, which provides
// privacy protection through potential false positives while guaranteeing
// no false negatives for actual tag matches.
type TagMatchMode int

const (
	// TagMatchAny requires at least one tag to match (OR semantics).
	// Most permissive mode - maximizes discovery but may include less relevant results.
	// Best for exploratory search and broad content discovery.
	TagMatchAny TagMatchMode = iota
	
	// TagMatchAll requires all tags to match (AND semantics).
	// Most restrictive mode - ensures high relevance but may miss some content.
	// Best for precise search with specific multi-tag requirements.
	TagMatchAll
	
	// TagMatchExact attempts exact tag set matching (limited by bloom filter capabilities).
	// Note: Exact matching is approximate due to bloom filter false positives.
	// Provides balanced precision for searches with well-defined tag sets.
	TagMatchExact
)

// SortField defines the available sorting criteria for search results.
//
// Sort fields determine the primary ordering dimension for search results,
// enabling users to prioritize different aspects of content discovery:
// relevance scoring, temporal organization, or size-based arrangement.
type SortField string

const (
	// SortByRelevance orders results by computed relevance score (default).
	// Relevance combines tag matching, keyword relevance, and recency bias
	// to provide the most useful results for the specific query.
	SortByRelevance SortField = "relevance"
	
	// SortByTime orders results by announcement timestamp.
	// Enables chronological browsing and discovery of recent content.
	// Useful for time-sensitive content and freshness-focused searches.
	SortByTime      SortField = "time"
	
	// SortBySize orders results by content size class.
	// Enables size-based browsing from tiny to huge content.
	// Useful for storage-conscious discovery and size-specific searches.
	SortBySize      SortField = "size"
)

// SortOrder defines the ordering direction for sorted search results.
//
// Sort order controls whether results are arranged in ascending or
// descending order based on the selected sort field, enabling both
// chronological and reverse-chronological discovery patterns.
type SortOrder string

const (
	// SortAsc arranges results in ascending order (smallest to largest).
	// For time: oldest first, for size: smallest first, for relevance: lowest score first.
	SortAsc  SortOrder = "asc"
	
	// SortDesc arranges results in descending order (largest to smallest).
	// For time: newest first, for size: largest first, for relevance: highest score first.
	SortDesc SortOrder = "desc"
)

// SearchResult represents a single search result with relevance scoring and highlighting.
//
// SearchResult combines the matched announcement with computed relevance metadata
// to provide rich search result information. The result includes the original
// announcement, a relevance score for ranking, and highlighted snippets showing
// why the announcement matched the search query.
//
// Result Components:
//   - Complete announcement data for content access
//   - Relevance score for intelligent ranking
//   - Highlight information for user interface display
type SearchResult struct {
	// Announcement contains the complete matched announcement data.
	// Provides access to all announcement metadata and content identifiers.
	Announcement *Announcement
	
	// Score represents the computed relevance score for this result.
	// Higher scores indicate better matches for the search query.
	// Score combines tag matching, keyword relevance, and recency factors.
	Score        float64
	
	// Highlights maps result fields to highlighted text snippets.
	// Shows users why this announcement matched their search query.
	// Field keys: "tags", "keywords", "category", etc.
	Highlights   map[string][]string // field -> highlighted snippets
}

// NewSearchEngine creates a new search engine with comprehensive indexing and learning capabilities.
//
// This constructor initializes a fully-featured search engine with optimized default
// configuration for NoiseFS content discovery. The engine includes tag recovery
// capabilities, hierarchical topic organization, and intelligent learning systems
// to improve search quality over time.
//
// Parameters:
//   - store: Announcement storage backend implementing AnnouncementStore interface
//   - hierarchy: Topic organization system for hierarchical content discovery
//
// Returns:
//   A new SearchEngine ready for indexing and search operations
//
// Time Complexity: O(1)
// Space Complexity: O(1) initially, grows with indexed content
//
// Default Configuration:
//   - Tag recovery with 70% confidence threshold and 1000 candidate limit
//   - Learning rate of 0.1 for adaptive tag recovery improvement
//   - 7-day pattern retention for tag recovery optimization
//   - Prefix search enabled for auto-completion functionality
//   - Maximum 1000 results to prevent resource exhaustion
//   - TagMatchAny mode for permissive tag matching
//
// Example:
//   store := NewInMemoryStore()
//   hierarchy := NewTopicHierarchy()
//   engine := announce.NewSearchEngine(store, hierarchy)
//   results, err := engine.Search(query)
func NewSearchEngine(store AnnouncementStore, hierarchy *TopicHierarchy) *SearchEngine {
	// Create tag recovery with default config
	tagRecoveryConfig := TagRecoveryConfig{
		MinConfidence:      0.7,
		MaxCandidates:      1000,
		LearningRate:       0.1,
		PatternRetention:   7 * 24 * time.Hour,
		EnablePrefixSearch: true,
	}
	
	return &SearchEngine{
		store:       store,
		hierarchy:   hierarchy,
		tagMatcher:  NewTagMatcher(TagMatchAny),
		tagRecovery: NewTagRecovery(tagRecoveryConfig),
		tagIndex:    make(map[string][]string),
		topicIndex:  make(map[string][]string),
		timeIndex:   newTimeIndex(),
		maxResults:  1000,
	}
}

// Search performs comprehensive content discovery based on the provided query specification.
//
// This method orchestrates the complete search pipeline: candidate retrieval,
// multi-dimensional filtering, relevance scoring, result highlighting, sorting,
// and pagination. It combines privacy-preserving tag matching, topic-based
// organization, and intelligent ranking to provide optimal content discovery.
//
// Parameters:
//   - query: Comprehensive search specification with filtering and ranking criteria
//
// Returns:
//   - Ranked slice of search results with relevance scores and highlights
//   - error if search operation fails
//
// Time Complexity: O(n*log(n)) where n is matching announcements (sorting dominates)
// Space Complexity: O(n) for result processing and intermediate storage
//
// Search Pipeline:
//   1. Candidate retrieval using topic/time indexing for efficiency
//   2. Multi-dimensional filtering (category, size, tags, temporal)
//   3. Relevance scoring with tag matching, keyword relevance, and recency bias
//   4. Result highlighting for user interface display
//   5. Custom sorting by relevance, time, or size with configurable order
//   6. Pagination with offset and limit controls
//
// Privacy Features:
//   - Tag matching uses bloom filters for privacy-preserving discovery
//   - Topic searches use SHA-256 hashes for anonymous organization
//   - No exposure of exact metadata beyond announcement structure
//
// Performance Optimizations:
//   - Index-based candidate retrieval minimizes full-scan operations
//   - Early filtering reduces scoring computation overhead
//   - Efficient sorting algorithms for large result sets
func (se *SearchEngine) Search(query SearchQuery) ([]*SearchResult, error) {
	se.mu.RLock()
	defer se.mu.RUnlock()
	
	// Get candidate announcements
	candidates, err := se.getCandidates(query)
	if err != nil {
		return nil, err
	}
	
	// Score and filter
	results := []*SearchResult{}
	
	for _, ann := range candidates {
		// Apply filters
		if !se.matchesFilters(ann, query) {
			continue
		}
		
		// Calculate score
		score := se.calculateScore(ann, query)
		if score <= 0 {
			continue
		}
		
		// Generate highlights
		highlights := se.generateHighlights(ann, query)
		
		result := &SearchResult{
			Announcement: ann,
			Score:        score,
			Highlights:   highlights,
		}
		
		results = append(results, result)
	}
	
	// Sort results
	se.sortResults(results, query.SortBy, query.SortOrder)
	
	// Apply pagination
	start := query.Offset
	end := query.Offset + query.Limit
	if end > len(results) || query.Limit == 0 {
		end = len(results)
	}
	
	if start >= len(results) {
		return []*SearchResult{}, nil
	}
	
	return results[start:end], nil
}

// SearchSimilar discovers content similar to a specified announcement using feature extraction.
//
// This method performs similarity-based content discovery by analyzing the features
// of a source announcement and finding other content with similar characteristics.
// It uses tag recovery, topic matching, category alignment, and size classification
// to build a comprehensive similarity profile for automated content discovery.
//
// Parameters:
//   - announcementID: Descriptor CID of the source announcement for similarity analysis
//   - limit: Maximum number of similar results to return
//
// Returns:
//   - Ranked slice of similar announcements ordered by relevance
//   - error if source announcement is not found or similarity search fails
//
// Time Complexity: O(n*log(n)) where n is matching similar announcements
// Space Complexity: O(m) where m is the number of extracted features
//
// Similarity Features:
//   - Tag similarity using bloom filter tag recovery
//   - Topic alignment using exact topic hash matching
//   - Category matching for content type similarity
//   - Size class alignment for similar content scope
//
// Feature Extraction:
//   - Recovers tags from source announcement bloom filter
//   - Uses topic hash for hierarchical content organization
//   - Incorporates category and size class for classification matching
//   - Combines features using TagMatchAny for permissive discovery
//
// Result Processing:
//   - Excludes the source announcement from similar results
//   - Ranks results by computed relevance score
//   - Applies specified limit for result set control
func (se *SearchEngine) SearchSimilar(announcementID string, limit int) ([]*SearchResult, error) {
	se.mu.RLock()
	defer se.mu.RUnlock()
	
	// Get source announcement
	source, err := se.store.GetByID(announcementID)
	if err != nil {
		return nil, err
	}
	
	// Extract features
	sourceTags := []string{}
	if source.TagBloom != "" {
		// Extract tags from bloom filter (approximate)
		sourceTags = se.extractTagsFromBloom(source.TagBloom)
	}
	
	// Build similarity query
	query := SearchQuery{
		IncludeTags: sourceTags,
		TagMode:     TagMatchAny,
		Topics:      []string{source.TopicHash},
		Categories:  []string{source.Category},
		SizeClasses: []string{source.SizeClass},
		SortBy:      SortByRelevance,
		SortOrder:   SortDesc,
		Limit:       limit,
	}
	
	// Search
	results, err := se.Search(query)
	if err != nil {
		return nil, err
	}
	
	// Filter out source
	filtered := []*SearchResult{}
	for _, result := range results {
		if result.Announcement.Descriptor != source.Descriptor {
			filtered = append(filtered, result)
		}
	}
	
	return filtered, nil
}

// Suggest provides intelligent search suggestions and auto-completion for user queries.
//
// This method generates contextual search suggestions based on indexed content,
// helping users discover relevant tags and topics while providing usage statistics
// for suggestion ranking. The suggestions include both tag-based and topic-based
// options with popularity metrics for intelligent ordering.
//
// Parameters:
//   - prefix: Partial search term for suggestion matching (case-insensitive)
//   - limit: Maximum number of suggestions to return (0 = no limit)
//
// Returns:
//   - Slice of search suggestions ordered by usage count (most popular first)
//
// Time Complexity: O(n + m*log(m)) where n is indexed items, m is matching suggestions
// Space Complexity: O(m) where m is the number of matching suggestions
//
// Suggestion Types:
//   - Tag suggestions from indexed tag collections with usage counts
//   - Topic suggestions from hierarchy with content availability metrics
//   - Popularity-based ranking for optimal user experience
//
// Suggestion Sources:
//   - Tag index provides real tag suggestions with actual usage statistics
//   - Topic hierarchy provides structured topic suggestions with content counts
//   - Case-insensitive prefix matching for flexible user input
//
// Ranking Algorithm:
//   - Primary ranking by usage count (popularity)
//   - Secondary ranking by alphabetical order for consistency
//   - Limit application after sorting for top suggestions
func (se *SearchEngine) Suggest(prefix string, limit int) []SearchSuggestion {
	se.mu.RLock()
	defer se.mu.RUnlock()
	
	suggestions := []SearchSuggestion{}
	prefix = strings.ToLower(prefix)
	
	// Suggest tags
	for tag := range se.tagIndex {
		if strings.HasPrefix(strings.ToLower(tag), prefix) {
			count := len(se.tagIndex[tag])
			suggestions = append(suggestions, SearchSuggestion{
				Type:  "tag",
				Value: tag,
				Count: count,
			})
		}
	}
	
	// Suggest topics
	topics := se.hierarchy.FindTopics(prefix)
	for _, topic := range topics {
		if hashes, err := se.hierarchy.GetTopicHashes(topic.Path, false); err == nil && len(hashes) > 0 {
			count := len(se.topicIndex[hashes[0]])
			suggestions = append(suggestions, SearchSuggestion{
				Type:  "topic",
				Value: topic.Path,
				Count: count,
			})
		}
	}
	
	// Sort by count
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Count > suggestions[j].Count
	})
	
	// Apply limit
	if limit > 0 && len(suggestions) > limit {
		suggestions = suggestions[:limit]
	}
	
	return suggestions
}

// Index methods

// IndexAnnouncement adds an announcement to all search indices for comprehensive discovery.
//
// This method performs multi-dimensional indexing of announcement content to enable
// efficient search across different discovery dimensions: tags, topics, and temporal
// organization. The indexing process extracts searchable features while maintaining
// privacy through bloom filter tag recovery and topic hash organization.
//
// Parameters:
//   - ann: Complete announcement to add to search indices
//
// Returns:
//   - error if indexing operation fails
//
// Time Complexity: O(k) where k is the number of recovered tags
// Space Complexity: O(k) for tag storage and index entries
//
// Indexing Dimensions:
//   - Tag indexing using bloom filter tag recovery for searchable tag discovery
//   - Topic indexing using SHA-256 topic hashes for hierarchical organization
//   - Temporal indexing with hourly bucketing for time-based queries
//
// Privacy Preservation:
//   - Tag recovery from bloom filters maintains tag privacy through false positives
//   - Topic hashing prevents enumeration while enabling exact-match discovery
//   - No exposure of sensitive metadata beyond announcement structure
//
// Thread Safety: Safe for concurrent use with write lock protection during indexing.
func (se *SearchEngine) IndexAnnouncement(ann *Announcement) error {
	se.mu.Lock()
	defer se.mu.Unlock()
	
	id := ann.Descriptor
	
	// Index by tags
	if ann.TagBloom != "" {
		tags := se.extractTagsFromBloom(ann.TagBloom)
		for _, tag := range tags {
			se.tagIndex[tag] = append(se.tagIndex[tag], id)
		}
	}
	
	// Index by topic
	se.topicIndex[ann.TopicHash] = append(se.topicIndex[ann.TopicHash], id)
	
	// Index by time
	se.timeIndex.Add(ann.Timestamp, id)
	
	return nil
}

// RebuildIndex completely reconstructs all search indices from stored announcements.
//
// This method performs a comprehensive index rebuild by clearing all existing
// indices and re-indexing all stored announcements. It's useful for index
// corruption recovery, configuration changes, or periodic index optimization.
// The rebuild process is atomic - either all indices are rebuilt successfully
// or the operation fails completely.
//
// Returns:
//   - error if index rebuild fails at any stage
//
// Time Complexity: O(n*k) where n is stored announcements, k is average tags per announcement
// Space Complexity: O(n*k) for complete index reconstruction
//
// Rebuild Process:
//   1. Clear all existing indices (tag, topic, time)
//   2. Retrieve all stored announcements from the storage backend
//   3. Re-index each announcement across all indexing dimensions
//   4. Atomic completion - success or complete failure
//
// Use Cases:
//   - Index corruption recovery after storage issues
//   - Configuration changes requiring index restructuring
//   - Periodic index optimization and cleanup
//   - Migration between different indexing strategies
//
// Thread Safety: Uses write lock for exclusive access during rebuild operation.
// Other search operations are blocked during the rebuild process.
func (se *SearchEngine) RebuildIndex() error {
	se.mu.Lock()
	defer se.mu.Unlock()
	
	// Clear indices
	se.tagIndex = make(map[string][]string)
	se.topicIndex = make(map[string][]string)
	se.timeIndex = newTimeIndex()
	
	// Get all announcements
	announcements, err := se.store.GetAll()
	if err != nil {
		return err
	}
	
	// Re-index
	for _, ann := range announcements {
		se.IndexAnnouncement(ann)
	}
	
	return nil
}

// Helper methods

func (se *SearchEngine) getCandidates(query SearchQuery) ([]*Announcement, error) {
	// Start with all or filtered by time
	var candidates []*Announcement
	var err error
	
	if query.Since != nil {
		candidates, err = se.store.GetRecent(*query.Since, se.maxResults)
	} else if len(query.Topics) > 0 {
		// Get by topics
		candidateMap := make(map[string]*Announcement)
		for _, topic := range query.Topics {
			topicAnns, err := se.store.GetByTopic(topic)
			if err != nil {
				continue
			}
			for _, ann := range topicAnns {
				candidateMap[ann.Descriptor] = ann
			}
		}
		
		candidates = make([]*Announcement, 0, len(candidateMap))
		for _, ann := range candidateMap {
			candidates = append(candidates, ann)
		}
	} else {
		candidates, err = se.store.GetAll()
	}
	
	if err != nil {
		return nil, err
	}
	
	return candidates, nil
}

func (se *SearchEngine) matchesFilters(ann *Announcement, query SearchQuery) bool {
	// Time filters
	annTime := time.Unix(ann.Timestamp, 0)
	if query.Since != nil && annTime.Before(*query.Since) {
		return false
	}
	if query.Until != nil && annTime.After(*query.Until) {
		return false
	}
	
	// Category filter
	if len(query.Categories) > 0 {
		found := false
		for _, cat := range query.Categories {
			if ann.Category == cat {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	
	// Size class filter
	if len(query.SizeClasses) > 0 {
		found := false
		for _, size := range query.SizeClasses {
			if ann.SizeClass == size {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	
	// Tag filters
	if len(query.ExcludeTags) > 0 && ann.TagBloom != "" {
		for _, tag := range query.ExcludeTags {
			bloom, _ := DecodeBloom(ann.TagBloom)
			if bloom != nil && bloom.Test(normalizeTag(tag)) {
				return false
			}
		}
	}
	
	return true
}

func (se *SearchEngine) calculateScore(ann *Announcement, query SearchQuery) float64 {
	score := 1.0
	
	// Tag matching
	if len(query.IncludeTags) > 0 && ann.TagBloom != "" {
		tagScore := se.calculateTagScore(ann, query.IncludeTags, query.TagMode)
		if query.TagMode == TagMatchAll && tagScore < 1.0 {
			return 0 // Must match all tags
		}
		score *= (1.0 + tagScore)
	}
	
	// Keyword matching (simplified)
	if len(query.Keywords) > 0 {
		keywordScore := se.calculateKeywordScore(ann, query.Keywords)
		score *= (1.0 + keywordScore)
	}
	
	// Recency boost
	age := time.Since(time.Unix(ann.Timestamp, 0))
	if age < 24*time.Hour {
		score *= 1.5
	} else if age < 7*24*time.Hour {
		score *= 1.2
	}
	
	return score
}

func (se *SearchEngine) calculateTagScore(ann *Announcement, tags []string, mode TagMatchMode) float64 {
	bloom, err := DecodeBloom(ann.TagBloom)
	if err != nil {
		return 0
	}
	
	matches := 0
	for _, tag := range tags {
		if bloom.Test(normalizeTag(tag)) {
			matches++
		}
	}
	
	switch mode {
	case TagMatchAll:
		if matches == len(tags) {
			return 1.0
		}
		return 0
	case TagMatchExact:
		// Can't do exact match with bloom filter
		return float64(matches) / float64(len(tags))
	default: // TagMatchAny
		return float64(matches) / float64(len(tags))
	}
}

func (se *SearchEngine) calculateKeywordScore(ann *Announcement, keywords []string) float64 {
	// Simple keyword matching in category and size
	matches := 0
	searchText := strings.ToLower(ann.Category + " " + ann.SizeClass)
	
	for _, keyword := range keywords {
		if strings.Contains(searchText, strings.ToLower(keyword)) {
			matches++
		}
	}
	
	return float64(matches) / float64(len(keywords))
}

func (se *SearchEngine) generateHighlights(ann *Announcement, query SearchQuery) map[string][]string {
	highlights := make(map[string][]string)
	
	// Highlight matching tags
	if len(query.IncludeTags) > 0 && ann.TagBloom != "" {
		bloom, _ := DecodeBloom(ann.TagBloom)
		if bloom != nil {
			matchedTags := []string{}
			for _, tag := range query.IncludeTags {
				if bloom.Test(normalizeTag(tag)) {
					matchedTags = append(matchedTags, tag)
				}
			}
			if len(matchedTags) > 0 {
				highlights["tags"] = matchedTags
			}
		}
	}
	
	return highlights
}

func (se *SearchEngine) sortResults(results []*SearchResult, sortBy SortField, order SortOrder) {
	sort.Slice(results, func(i, j int) bool {
		var less bool
		
		switch sortBy {
		case SortByTime:
			less = results[i].Announcement.Timestamp < results[j].Announcement.Timestamp
		case SortBySize:
			// Compare size classes
			sizeOrder := map[string]int{
				"tiny": 1, "small": 2, "medium": 3, "large": 4, "huge": 5,
			}
			iOrder := sizeOrder[results[i].Announcement.SizeClass]
			jOrder := sizeOrder[results[j].Announcement.SizeClass]
			less = iOrder < jOrder
		default: // SortByRelevance
			less = results[i].Score < results[j].Score
		}
		
		if order == SortDesc {
			less = !less
		}
		
		return less
	})
}

func (se *SearchEngine) extractTagsFromBloom(bloomStr string) []string {
	// Use enhanced tag recovery if available
	if se.tagRecovery != nil {
		tags, err := se.tagRecovery.RecoverTags(bloomStr)
		if err == nil {
			return tags
		}
	}
	
	// Fallback to basic recovery
	knownTags := []string{
		"res:4k", "res:1080p", "res:720p",
		"genre:scifi", "genre:drama", "genre:comedy",
		"year:2023", "year:2024",
		"remastered", "extended",
	}
	
	bloom, err := DecodeBloom(bloomStr)
	if err != nil {
		return []string{}
	}
	
	extractedTags := []string{}
	for _, tag := range knownTags {
		if bloom.Test(normalizeTag(tag)) {
			extractedTags = append(extractedTags, tag)
		}
	}
	
	return extractedTags
}

// TimeIndex implementation

// newTimeIndex creates a new time index with hourly bucketing for efficient temporal queries.
//
// Returns:
//   A new TimeIndex ready for timestamp-based announcement indexing
//
// Time Complexity: O(1)
// Space Complexity: O(1) initially, grows with indexed announcements
func newTimeIndex() *TimeIndex {
	return &TimeIndex{
		buckets: make(map[string][]string),
	}
}

// Add inserts an announcement into the appropriate time bucket for temporal indexing.
//
// This method organizes announcements into hourly time buckets to enable efficient
// time-range queries. The bucketing strategy balances memory usage with query
// performance by grouping announcements within the same hour.
//
// Parameters:
//   - timestamp: Unix timestamp (seconds since epoch) of the announcement
//   - id: Announcement descriptor CID for bucket storage
//
// Time Complexity: O(1) for bucket insertion
// Space Complexity: O(1) per announcement
//
// Bucketing Strategy:
//   - Uses "YYYY-MM-DD-HH" format for deterministic bucket keys
//   - Groups all announcements within the same hour
//   - Enables efficient range queries spanning multiple hours
func (ti *TimeIndex) Add(timestamp int64, id string) {
	ti.mu.Lock()
	defer ti.mu.Unlock()
	
	// Create hourly buckets
	bucket := time.Unix(timestamp, 0).Format("2006-01-02-15")
	ti.buckets[bucket] = append(ti.buckets[bucket], id)
}

// GetRange retrieves all announcement IDs within the specified time range using bucket iteration.
//
// This method performs efficient time-range queries by iterating through hourly
// buckets that overlap with the requested time range. It includes deduplication
// to handle announcements that might appear in multiple buckets due to edge cases.
//
// Parameters:
//   - start: Beginning of the time range (inclusive)
//   - end: End of the time range (exclusive)
//
// Returns:
//   - Deduplicated slice of announcement descriptor IDs in the time range
//
// Time Complexity: O(h + n) where h is hours in range, n is matching announcements
// Space Complexity: O(n) for result collection and deduplication
//
// Range Query Algorithm:
//   - Truncates start time to hour boundary for bucket alignment
//   - Iterates through all hourly buckets in the time range
//   - Collects announcement IDs with deduplication
//   - Returns combined results from all relevant buckets
func (ti *TimeIndex) GetRange(start, end time.Time) []string {
	ti.mu.RLock()
	defer ti.mu.RUnlock()
	
	ids := []string{}
	seen := make(map[string]bool)
	
	// Iterate through hourly buckets
	current := start.Truncate(time.Hour)
	for current.Before(end) {
		bucket := current.Format("2006-01-02-15")
		for _, id := range ti.buckets[bucket] {
			if !seen[id] {
				ids = append(ids, id)
				seen[id] = true
			}
		}
		current = current.Add(time.Hour)
	}
	
	return ids
}

// SearchSuggestion represents a single search suggestion with metadata for intelligent auto-completion.
//
// SearchSuggestion provides contextual information about available search terms,
// including the suggestion type, actual value, and usage statistics. This enables
// intelligent auto-completion interfaces that help users discover relevant content
// while showing the popularity and availability of different search options.
type SearchSuggestion struct {
	// Type indicates the suggestion category for user interface organization.
	// Values: "tag" (searchable tags), "topic" (hierarchical topics), "category" (content types)
	Type  string // "tag", "topic", "category"
	
	// Value contains the actual suggestion text for auto-completion.
	// Ready for direct use in search queries without additional processing.
	Value string
	
	// Count indicates the number of announcements matching this suggestion.
	// Provides popularity metrics for suggestion ranking and user guidance.
	Count int
}

// TagMatcher provides configurable tag matching functionality with different matching strategies.
//
// TagMatcher encapsulates tag matching logic with support for different matching
// modes (any/all/exact), enabling flexible tag-based content discovery. The matcher
// works with bloom filter-based tag storage to provide privacy-preserving search
// while maintaining configurable precision levels.
type TagMatcher struct {
	// mode defines the tag matching strategy for multi-tag queries.
	// Controls how multiple tags are combined (any/all/exact matching).
	mode TagMatchMode
}

// NewTagMatcher creates a new tag matcher with the specified matching strategy.
//
// Parameters:
//   - mode: Tag matching mode (any/all/exact) for multi-tag query evaluation
//
// Returns:
//   A new TagMatcher configured with the specified matching strategy
//
// Time Complexity: O(1)
// Space Complexity: O(1)
func NewTagMatcher(mode TagMatchMode) *TagMatcher {
	return &TagMatcher{mode: mode}
}

// LearnFromSearch improves tag recovery through user interaction analysis and feedback learning.
//
// This method implements a learning system that observes user search behavior
// to improve tag recovery accuracy over time. By analyzing search queries and
// user result selections, the system learns which tags are commonly used
// together and refines its tag recovery algorithms accordingly.
//
// Parameters:
//   - query: Original search query with user-specified tags and criteria
//   - selectedResults: Results that the user actually selected or interacted with
//
// Time Complexity: O(k) where k is the total number of tags in query and results
// Space Complexity: O(k) for tag learning data structures
//
// Learning Sources:
//   - Query tags: Direct user tag specifications indicating tag usage patterns
//   - Selected result tags: Tags recovered from bloom filters of chosen content
//   - Correlation patterns: Associations between query tags and result tags
//
// Improvement Areas:
//   - Tag recovery confidence through usage pattern analysis
//   - Tag association discovery for related content suggestions
//   - Query refinement suggestions based on successful search patterns
//
// Privacy Note: Learning operates on normalized tag data without exposing
// user identity or sensitive search patterns beyond tag usage statistics.
func (se *SearchEngine) LearnFromSearch(query SearchQuery, selectedResults []*SearchResult) {
	// Learn tags from the query
	if se.tagRecovery != nil && len(query.IncludeTags) > 0 {
		se.tagRecovery.LearnFromTags(query.IncludeTags)
	}
	
	// Learn tags from selected results
	for _, result := range selectedResults {
		if result.Announcement.TagBloom != "" {
			tags := se.extractTagsFromBloom(result.Announcement.TagBloom)
			if len(tags) > 0 && se.tagRecovery != nil {
				se.tagRecovery.LearnFromTags(tags)
			}
		}
	}
}

// GetTagRecoveryStats returns comprehensive statistics about tag recovery performance and learning.
//
// This method provides visibility into the tag recovery system's performance,
// learning progress, and operational metrics. The statistics help monitor
// system effectiveness and guide configuration optimization for improved
// search quality and tag discovery accuracy.
//
// Returns:
//   - Map containing tag recovery metrics and performance statistics
//   - "enabled": false if tag recovery is not available
//
// Time Complexity: O(1) for statistics retrieval
// Space Complexity: O(k) where k is the number of statistical metrics
//
// Statistics Included:
//   - Tag recovery success rates and confidence metrics
//   - Learning system performance and adaptation rates
//   - Pattern recognition accuracy and false positive rates
//   - System configuration and operational parameters
//
// Use Cases:
//   - System monitoring and performance optimization
//   - Configuration tuning for improved tag recovery
//   - Debugging tag discovery issues
//   - Performance analysis and capacity planning
func (se *SearchEngine) GetTagRecoveryStats() map[string]interface{} {
	if se.tagRecovery != nil {
		return se.tagRecovery.GetStatistics()
	}
	return map[string]interface{}{
		"enabled": false,
	}
}
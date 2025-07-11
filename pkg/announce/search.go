package announce

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// SearchEngine provides advanced search capabilities for announcements
type SearchEngine struct {
	store       AnnouncementStore
	hierarchy   *TopicHierarchy
	tagMatcher  *TagMatcher
	
	// Indexing
	tagIndex    map[string][]string // tag -> announcement IDs
	topicIndex  map[string][]string // topic hash -> announcement IDs
	timeIndex   *TimeIndex
	
	// Configuration
	maxResults  int
	
	mu          sync.RWMutex
}

// AnnouncementStore interface for announcement storage
type AnnouncementStore interface {
	GetByID(id string) (*Announcement, error)
	GetAll() ([]*Announcement, error)
	GetByTopic(topicHash string) ([]*Announcement, error)
	GetRecent(since time.Time, limit int) ([]*Announcement, error)
}

// TimeIndex provides time-based indexing
type TimeIndex struct {
	buckets map[string][]string // time bucket -> announcement IDs
	mu      sync.RWMutex
}

// SearchQuery represents a search query
type SearchQuery struct {
	// Text search
	Keywords    []string
	
	// Tag search
	IncludeTags []string
	ExcludeTags []string
	TagMode     TagMatchMode
	
	// Topic search
	Topics      []string
	IncludeSubtopics bool
	
	// Filters
	Categories  []string
	SizeClasses []string
	MinSize     int64
	MaxSize     int64
	
	// Time filters
	Since       *time.Time
	Until       *time.Time
	
	// Result control
	SortBy      SortField
	SortOrder   SortOrder
	Limit       int
	Offset      int
}

// TagMatchMode defines how tags are matched
type TagMatchMode int

const (
	TagMatchAny TagMatchMode = iota
	TagMatchAll
	TagMatchExact
)

// SortField defines sort fields
type SortField string

const (
	SortByRelevance SortField = "relevance"
	SortByTime      SortField = "time"
	SortBySize      SortField = "size"
)

// SortOrder defines sort order
type SortOrder string

const (
	SortAsc  SortOrder = "asc"
	SortDesc SortOrder = "desc"
)

// SearchResult represents a search result
type SearchResult struct {
	Announcement *Announcement
	Score        float64
	Highlights   map[string][]string // field -> highlighted snippets
}

// NewSearchEngine creates a new search engine
func NewSearchEngine(store AnnouncementStore, hierarchy *TopicHierarchy) *SearchEngine {
	return &SearchEngine{
		store:      store,
		hierarchy:  hierarchy,
		tagMatcher: NewTagMatcher(TagMatchAny),
		tagIndex:   make(map[string][]string),
		topicIndex: make(map[string][]string),
		timeIndex:  newTimeIndex(),
		maxResults: 1000,
	}
}

// Search performs a search based on the query
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

// SearchSimilar finds similar announcements
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

// Suggest provides search suggestions
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

// IndexAnnouncement adds an announcement to the search index
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

// RebuildIndex rebuilds the search index
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
	// This is approximate - bloom filters don't allow extraction
	// In practice, we'd test known tags against the bloom filter
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

func newTimeIndex() *TimeIndex {
	return &TimeIndex{
		buckets: make(map[string][]string),
	}
}

func (ti *TimeIndex) Add(timestamp int64, id string) {
	ti.mu.Lock()
	defer ti.mu.Unlock()
	
	// Create hourly buckets
	bucket := time.Unix(timestamp, 0).Format("2006-01-02-15")
	ti.buckets[bucket] = append(ti.buckets[bucket], id)
}

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

// SearchSuggestion represents a search suggestion
type SearchSuggestion struct {
	Type  string // "tag", "topic", "category"
	Value string
	Count int
}

// TagMatcher wraps tag matching functionality
type TagMatcher struct {
	mode TagMatchMode
}

func NewTagMatcher(mode TagMatchMode) *TagMatcher {
	return &TagMatcher{mode: mode}
}
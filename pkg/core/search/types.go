package search

import (
	"time"
)

// SearchQueryType defines the type of search query
type SearchQueryType int

const (
	FilenameSearch SearchQueryType = iota
	ContentSearch
	MetadataSearch
	SimilaritySearch
	ComplexSearch
)

// String returns the string representation of SearchQueryType
func (sqt SearchQueryType) String() string {
	switch sqt {
	case FilenameSearch:
		return "filename"
	case ContentSearch:
		return "content"
	case MetadataSearch:
		return "metadata"
	case SimilaritySearch:
		return "similarity"
	case ComplexSearch:
		return "complex"
	default:
		return "unknown"
	}
}

// SearchQuery represents a privacy-preserving search query
type SearchQuery struct {
	// Core query information
	Query           string          `json:"query"`
	Type            SearchQueryType `json:"type"`
	MaxResults      int             `json:"max_results"`
	
	// Privacy settings
	PrivacyLevel    int             `json:"privacy_level"`
	SessionID       string          `json:"session_id"`
	
	// Privacy transformations
	ObfuscatedQuery string          `json:"-"`
	DummyQueries    []string        `json:"-"`
	
	// Metadata
	RequestTime     time.Time       `json:"request_time"`
	UserID          string          `json:"user_id,omitempty"`
}

// SearchResult represents a search result with privacy protection
type SearchResult struct {
	// File identification
	FileID          string                 `json:"file_id"`
	Filename        string                 `json:"filename,omitempty"`
	Path            string                 `json:"path,omitempty"`
	
	// Relevance and matching
	Relevance       float64                `json:"relevance"`
	MatchType       string                 `json:"match_type"`
	Similarity      float64                `json:"similarity"`
	
	// Source information
	Sources         []string               `json:"sources"`
	IndexSource     string                 `json:"index_source"`
	
	// Privacy information
	PrivacyLevel    int                    `json:"privacy_level"`
	NoiseLevel      float64                `json:"noise_level"`
	
	// Metadata
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
	
	// Timestamps
	LastModified    time.Time              `json:"last_modified,omitempty"`
	IndexedAt       time.Time              `json:"indexed_at,omitempty"`
}

// SearchResponse represents the response from a privacy search
type SearchResponse struct {
	// Query information
	Query           *SearchQuery           `json:"query"`
	SearchID        string                 `json:"search_id"`
	
	// Results
	Results         []SearchResult         `json:"results"`
	TotalResults    int                    `json:"total_results"`
	
	// Privacy information
	PrivacyLevel    int                    `json:"privacy_level"`
	DummyQueries    int                    `json:"dummy_queries"`
	NoiseApplied    bool                   `json:"noise_applied"`
	
	// Performance metrics
	Duration        time.Duration          `json:"duration"`
	CacheHit        bool                   `json:"cache_hit"`
	
	// Privacy budget
	BudgetUsed      float64                `json:"budget_used"`
	BudgetRemaining float64                `json:"budget_remaining"`
	
	// Metadata
	Timestamp       time.Time              `json:"timestamp"`
	Version         string                 `json:"version"`
}

// PrivacySearchConfig configures the privacy search engine
type PrivacySearchConfig struct {
	// Core settings
	MaxResults              int           `json:"max_results"`
	DefaultPrivacyLevel     int           `json:"default_privacy_level"`
	MaxPrivacyLevel         int           `json:"max_privacy_level"`
	
	// Performance settings
	QueryTimeout            time.Duration `json:"query_timeout"`
	ParallelSearchEnabled   bool          `json:"parallel_search_enabled"`
	MaxConcurrentQueries    int           `json:"max_concurrent_queries"`
	
	// Cache settings
	CacheEnabled            bool          `json:"cache_enabled"`
	CacheTTL                time.Duration `json:"cache_ttl"`
	MaxCacheEntries         int           `json:"max_cache_entries"`
	
	// Privacy settings
	EnableDummyQueries      bool          `json:"enable_dummy_queries"`
	EnableTimingObfuscation bool          `json:"enable_timing_obfuscation"`
	EnableResultObfuscation bool          `json:"enable_result_obfuscation"`
	
	// Session settings
	SessionTimeout          time.Duration `json:"session_timeout"`
	MaxSessionQueries       int           `json:"max_session_queries"`
	
	// Rate limiting
	RateLimitEnabled        bool          `json:"rate_limit_enabled"`
	MaxQueriesPerMinute     int           `json:"max_queries_per_minute"`
	MaxQueriesPerHour       int           `json:"max_queries_per_hour"`
}

// DefaultPrivacySearchConfig returns default configuration
func DefaultPrivacySearchConfig() *PrivacySearchConfig {
	return &PrivacySearchConfig{
		MaxResults:              100,
		DefaultPrivacyLevel:     2,
		MaxPrivacyLevel:         5,
		QueryTimeout:            time.Second * 30,
		ParallelSearchEnabled:   true,
		MaxConcurrentQueries:    10,
		CacheEnabled:            true,
		CacheTTL:                time.Hour,
		MaxCacheEntries:         1000,
		EnableDummyQueries:      true,
		EnableTimingObfuscation: true,
		EnableResultObfuscation: true,
		SessionTimeout:          time.Hour * 4,
		MaxSessionQueries:       1000,
		RateLimitEnabled:        true,
		MaxQueriesPerMinute:     60,
		MaxQueriesPerHour:       1000,
	}
}

// SearchStats represents search engine statistics
type SearchStats struct {
	// Query statistics
	TotalQueries        uint64            `json:"total_queries"`
	SuccessfulQueries   uint64            `json:"successful_queries"`
	FailedQueries       uint64            `json:"failed_queries"`
	CachedQueries       uint64            `json:"cached_queries"`
	
	// Performance statistics
	AverageResponseTime time.Duration     `json:"average_response_time"`
	P95ResponseTime     time.Duration     `json:"p95_response_time"`
	P99ResponseTime     time.Duration     `json:"p99_response_time"`
	
	// Privacy statistics
	QueriesByPrivacyLevel map[int]uint64   `json:"queries_by_privacy_level"`
	DummyQueriesGenerated uint64           `json:"dummy_queries_generated"`
	NoiseApplications     uint64           `json:"noise_applications"`
	
	// Cache statistics
	CacheHits           uint64            `json:"cache_hits"`
	CacheMisses         uint64            `json:"cache_misses"`
	CacheEvictions      uint64            `json:"cache_evictions"`
	
	// Error statistics
	ErrorsByType        map[string]uint64 `json:"errors_by_type"`
	SecurityViolations  uint64            `json:"security_violations"`
	PrivacyViolations   uint64            `json:"privacy_violations"`
	
	// Timestamps
	StartTime           time.Time         `json:"start_time"`
	LastUpdated         time.Time         `json:"last_updated"`
	UptimeDuration      time.Duration     `json:"uptime_duration"`
}

// ToIndexQuery converts SearchQuery to an index manager query (simplified interface)
func (sq *SearchQuery) ToIndexQuery() *UnifiedSearchQuery {
	return &UnifiedSearchQuery{
		FilenameQuery: &FilenameQuery{
			Pattern:    sq.ObfuscatedQuery,
			CaseSensitive: false,
		},
		ContentQuery: &ContentQuery{
			Text:       sq.ObfuscatedQuery,
			Fuzzy:      true,
		},
		DirectoryQuery: &DirectoryQuery{
			Path:       sq.ObfuscatedQuery,
			Recursive:  true,
		},
		MaxResults: sq.MaxResults,
	}
}

// Simplified index query types (will be replaced with actual index package imports)
type UnifiedSearchQuery struct {
	FilenameQuery  *FilenameQuery
	ContentQuery   *ContentQuery
	DirectoryQuery *DirectoryQuery
	MaxResults     int
}

type FilenameQuery struct {
	Pattern       string
	CaseSensitive bool
}

type ContentQuery struct {
	Text  string
	Fuzzy bool
}

type DirectoryQuery struct {
	Path      string
	Recursive bool
}

// UnifiedSearchResult represents unified search results from index manager
type UnifiedSearchResult struct {
	Matches []SearchMatch `json:"matches"`
	Total   int           `json:"total"`
}

// SearchMatch represents a single search match
type SearchMatch struct {
	FileID     string   `json:"file_id"`
	Relevance  float64  `json:"relevance"`
	MatchType  string   `json:"match_type"`
	Similarity float64  `json:"similarity"`
	Sources    []string `json:"sources"`
	Source     string   `json:"source"`
}

// Error types for search operations
type SearchError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Code    int    `json:"code"`
}

func (e *SearchError) Error() string {
	return e.Message
}

// Common error types
const (
	ErrorTypeInvalidQuery    = "invalid_query"
	ErrorTypePrivacyViolation = "privacy_violation"
	ErrorTypeRateLimit       = "rate_limit"
	ErrorTypeTimeout         = "timeout"
	ErrorTypeInternal        = "internal_error"
)
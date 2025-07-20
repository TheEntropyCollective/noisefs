// Package types defines core data structures for privacy-preserving search operations.
package types

import (
	"context"
	"time"
)

// SearchQuery represents a privacy-preserving search request
type SearchQuery struct {
	// Query terms with privacy protection
	Terms []string `json:"terms"`
	
	// Search scope and filters
	Scope SearchScope `json:"scope"`
	
	// Privacy settings
	Privacy PrivacyLevel `json:"privacy"`
	
	// Pagination and limits
	Limit  int `json:"limit,omitempty"`
	Offset int `json:"offset,omitempty"`
	
	// Timeout for search operation
	Timeout time.Duration `json:"timeout,omitempty"`
}

// SearchScope defines the scope of search operations
type SearchScope struct {
	// Include file content search
	Content bool `json:"content"`
	
	// Include metadata search
	Metadata bool `json:"metadata"`
	
	// Include filename search
	Filenames bool `json:"filenames"`
	
	// File type filters
	FileTypes []string `json:"file_types,omitempty"`
	
	// Size filters
	MinSize int64 `json:"min_size,omitempty"`
	MaxSize int64 `json:"max_size,omitempty"`
	
	// Time filters
	ModifiedAfter  *time.Time `json:"modified_after,omitempty"`
	ModifiedBefore *time.Time `json:"modified_before,omitempty"`
}

// PrivacyLevel defines the level of privacy protection for search
type PrivacyLevel int

const (
	// PrivacyMinimal provides basic query obfuscation
	PrivacyMinimal PrivacyLevel = iota
	
	// PrivacyStandard provides moderate privacy protection with dummy queries
	PrivacyStandard
	
	// PrivacyMaximum provides maximum privacy with extensive query mixing
	PrivacyMaximum
)

// SearchResult represents a single search result with privacy considerations
type SearchResult struct {
	// File identifier (anonymized)
	FileID string `json:"file_id"`
	
	// Match information
	Matches []Match `json:"matches"`
	
	// Relevance score (privacy-adjusted)
	Score float64 `json:"score"`
	
	// Metadata (filtered for privacy)
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// Match represents a specific match within a file
type Match struct {
	// Type of match (content, filename, metadata)
	Type MatchType `json:"type"`
	
	// Context around the match (privacy-filtered)
	Context string `json:"context"`
	
	// Position information (if available)
	Position *MatchPosition `json:"position,omitempty"`
	
	// Confidence score
	Confidence float64 `json:"confidence"`
}

// MatchType defines the type of search match
type MatchType string

const (
	MatchTypeContent  MatchType = "content"
	MatchTypeFilename MatchType = "filename"
	MatchTypeMetadata MatchType = "metadata"
)

// MatchPosition provides position information for a match
type MatchPosition struct {
	Line   int `json:"line,omitempty"`
	Column int `json:"column,omitempty"`
	Offset int `json:"offset,omitempty"`
}

// SearchResponse represents the complete response to a search query
type SearchResponse struct {
	// Search results
	Results []SearchResult `json:"results"`
	
	// Total number of results (may be approximate for privacy)
	Total int `json:"total"`
	
	// Search metadata
	SearchTime time.Duration `json:"search_time"`
	
	// Privacy information
	PrivacyLevel PrivacyLevel `json:"privacy_level"`
	
	// Pagination information
	HasMore bool `json:"has_more"`
	
	// Error information (if any)
	Error string `json:"error,omitempty"`
}

// SearchEngine defines the interface for privacy-preserving search operations
type SearchEngine interface {
	// Search performs a privacy-preserving search
	Search(ctx context.Context, query *SearchQuery) (*SearchResponse, error)
	
	// Index adds content to the search index
	Index(ctx context.Context, fileID string, content []byte, metadata map[string]interface{}) error
	
	// Remove removes content from the search index
	Remove(ctx context.Context, fileID string) error
	
	// UpdatePrivacySettings updates privacy settings for the search engine
	UpdatePrivacySettings(settings *PrivacySettings) error
	
	// GetStats returns search engine statistics
	GetStats() (*SearchStats, error)
}

// PrivacySettings configures privacy protection for search operations
type PrivacySettings struct {
	// Default privacy level
	DefaultPrivacy PrivacyLevel `json:"default_privacy"`
	
	// Query obfuscation settings
	QueryObfuscation bool `json:"query_obfuscation"`
	
	// Result filtering settings
	ResultFiltering bool `json:"result_filtering"`
	
	// Timing obfuscation
	TimingObfuscation bool `json:"timing_obfuscation"`
	
	// Maximum number of dummy queries per real query
	MaxDummyQueries int `json:"max_dummy_queries"`
	
	// Context window size for result snippets
	ContextWindow int `json:"context_window"`
}

// SearchStats provides statistics about search operations
type SearchStats struct {
	// Index statistics
	IndexedFiles int64 `json:"indexed_files"`
	IndexSize    int64 `json:"index_size"`
	
	// Query statistics
	TotalQueries   int64         `json:"total_queries"`
	AverageLatency time.Duration `json:"average_latency"`
	
	// Privacy statistics
	PrivacyQueries map[PrivacyLevel]int64 `json:"privacy_queries"`
	
	// Performance metrics
	CacheHitRate float64 `json:"cache_hit_rate"`
	
	// Last update time
	LastUpdated time.Time `json:"last_updated"`
}
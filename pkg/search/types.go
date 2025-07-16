package search

import (
	"time"
)

// SearchService defines the interface for search operations
type SearchService interface {
	// Search operations
	Search(query string, options SearchOptions) (*SearchResults, error)
	SearchMetadata(filters MetadataFilters) (*SearchResults, error)
	
	// Index management
	UpdateIndex(path string, metadata FileMetadata) error
	RemoveFromIndex(path string) error
	RebuildIndex() error
	
	// Status and maintenance
	GetIndexStats() (*IndexStats, error)
	OptimizeIndex() error
	Close() error
}

// SearchOptions configures search behavior
type SearchOptions struct {
	// Result configuration
	MaxResults   int               `json:"max_results,omitempty"`
	Offset       int               `json:"offset,omitempty"`
	SortBy       SortField         `json:"sort_by,omitempty"`
	SortOrder    SortOrder         `json:"sort_order,omitempty"`
	
	// Content options
	Highlight    bool              `json:"highlight,omitempty"`
	IncludeBody  bool              `json:"include_body,omitempty"`
	PreviewSize  int               `json:"preview_size,omitempty"`
	
	// Filtering
	Filters      map[string]interface{} `json:"filters,omitempty"`
	TimeRange    *TimeRange        `json:"time_range,omitempty"`
	SizeRange    *SizeRange        `json:"size_range,omitempty"`
	FileTypes    []string          `json:"file_types,omitempty"`
	
	// Directory scoping
	Directory    string            `json:"directory,omitempty"`
	Recursive    bool              `json:"recursive,omitempty"`
	
	// Advanced options
	Facets       []string          `json:"facets,omitempty"`
	MinScore     float64           `json:"min_score,omitempty"`
	Timeout      time.Duration     `json:"timeout,omitempty"`
}

// MetadataFilters for metadata-only searches
type MetadataFilters struct {
	// File properties
	NamePattern  string            `json:"name_pattern,omitempty"`
	PathPattern  string            `json:"path_pattern,omitempty"`
	SizeRange    *SizeRange        `json:"size_range,omitempty"`
	TimeRange    *TimeRange        `json:"time_range,omitempty"`
	
	// Content properties
	MimeTypes    []string          `json:"mime_types,omitempty"`
	FileTypes    []string          `json:"file_types,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
	
	// Directory scoping
	Directory    string            `json:"directory,omitempty"`
	Recursive    bool              `json:"recursive,omitempty"`
	
	// Advanced filters
	HasContent   *bool             `json:"has_content,omitempty"`
	IsEncrypted  *bool             `json:"is_encrypted,omitempty"`
	MinSize      *int64            `json:"min_size,omitempty"`
	MaxSize      *int64            `json:"max_size,omitempty"`
}

// SearchResults contains search results and metadata
type SearchResults struct {
	// Query information
	Query        string            `json:"query"`
	QueryType    string            `json:"query_type"`
	Options      SearchOptions     `json:"options"`
	
	// Results
	Results      []SearchResult    `json:"results"`
	Total        int               `json:"total"`
	MaxScore     float64           `json:"max_score"`
	
	// Performance metrics
	TimeTaken    time.Duration     `json:"time_taken"`
	TimeTakenMS  int64             `json:"time_taken_ms"`
	
	// Facets and aggregations
	Facets       map[string]FacetResult `json:"facets,omitempty"`
	
	// Pagination
	Offset       int               `json:"offset"`
	Limit        int               `json:"limit"`
	HasMore      bool              `json:"has_more"`
}

// SearchResult represents a single search result
type SearchResult struct {
	// File identification
	Path            string            `json:"path"`
	DescriptorCID   string            `json:"descriptor_cid"`
	
	// Content information
	Score           float64           `json:"score"`
	Preview         string            `json:"preview,omitempty"`
	Highlights      []string          `json:"highlights,omitempty"`
	
	// File metadata
	Size            int64             `json:"size"`
	ModifiedAt      time.Time         `json:"modified_at"`
	CreatedAt       time.Time         `json:"created_at"`
	MimeType        string            `json:"mime_type"`
	FileType        string            `json:"file_type"`
	
	// NoiseFS specific
	IsEncrypted     bool              `json:"is_encrypted"`
	EncryptionKeyID string            `json:"encryption_key_id,omitempty"`
	Tags            []string          `json:"tags,omitempty"`
	
	// Directory information
	Directory       string            `json:"directory"`
	IsDirectory     bool              `json:"is_directory"`
	
	// Content analysis
	ContentHash     string            `json:"content_hash,omitempty"`
	IndexedAt       time.Time         `json:"indexed_at"`
	Language        string            `json:"language,omitempty"`
	
	// Extended metadata
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// FileMetadata contains file information for indexing
type FileMetadata struct {
	// Basic file info
	Path            string            `json:"path"`
	Size            int64             `json:"size"`
	ModifiedAt      time.Time         `json:"modified_at"`
	CreatedAt       time.Time         `json:"created_at"`
	
	// Content information
	MimeType        string            `json:"mime_type"`
	FileType        string            `json:"file_type"`
	ContentHash     string            `json:"content_hash"`
	Content         string            `json:"content,omitempty"`
	ContentPreview  string            `json:"content_preview,omitempty"`
	Language        string            `json:"language,omitempty"`
	
	// NoiseFS specific
	DescriptorCID   string            `json:"descriptor_cid"`
	IsEncrypted     bool              `json:"is_encrypted"`
	EncryptionKeyID string            `json:"encryption_key_id,omitempty"`
	Tags            []string          `json:"tags,omitempty"`
	
	// Directory information
	Directory       string            `json:"directory"`
	IsDirectory     bool              `json:"is_directory"`
	
	// Extended metadata
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// IndexStats provides information about the search index
type IndexStats struct {
	// Index size and counts
	DocumentCount   int64             `json:"document_count"`
	IndexSize       int64             `json:"index_size"`
	IndexSizeHuman  string            `json:"index_size_human"`
	
	// Performance metrics
	LastIndexTime   time.Time         `json:"last_index_time"`
	IndexDuration   time.Duration     `json:"index_duration"`
	SearchCount     int64             `json:"search_count"`
	AvgSearchTime   time.Duration     `json:"avg_search_time"`
	
	// Content analysis
	FileTypes       map[string]int64  `json:"file_types"`
	Languages       map[string]int64  `json:"languages"`
	MimeTypes       map[string]int64  `json:"mime_types"`
	
	// Directory distribution
	DirectoryCount  int64             `json:"directory_count"`
	FileCount       int64             `json:"file_count"`
	TotalSize       int64             `json:"total_size"`
	
	// Index health
	LastOptimized   time.Time         `json:"last_optimized"`
	NeedsOptimization bool            `json:"needs_optimization"`
	ErrorCount      int64             `json:"error_count"`
	LastError       string            `json:"last_error,omitempty"`
	
	// Configuration
	MaxIndexSize    int64             `json:"max_index_size"`
	Workers         int               `json:"workers"`
	BatchSize       int               `json:"batch_size"`
	
	// Queue status
	QueueSize       int               `json:"queue_size"`
	ProcessingFiles int               `json:"processing_files"`
	BackgroundTasks int               `json:"background_tasks"`
	
	// Cache statistics
	CacheStats      *CacheStats       `json:"cache_stats,omitempty"`
}

// Supporting types

// SortField defines available sort fields
type SortField string

const (
	SortByScore      SortField = "score"
	SortByModified   SortField = "modified"
	SortByCreated    SortField = "created"
	SortBySize       SortField = "size"
	SortByName       SortField = "name"
	SortByPath       SortField = "path"
	SortByType       SortField = "type"
)

// SortOrder defines sort direction
type SortOrder string

const (
	SortAsc  SortOrder = "asc"
	SortDesc SortOrder = "desc"
)

// TimeRange defines a time range filter
type TimeRange struct {
	Start time.Time `json:"start,omitempty"`
	End   time.Time `json:"end,omitempty"`
}

// SizeRange defines a size range filter
type SizeRange struct {
	Min int64 `json:"min,omitempty"`
	Max int64 `json:"max,omitempty"`
}

// FacetResult contains facet aggregation results
type FacetResult struct {
	Field       string                 `json:"field"`
	Values      []FacetValue          `json:"values"`
	Total       int                   `json:"total"`
	Other       int                   `json:"other"`
	Missing     int                   `json:"missing"`
}

// FacetValue represents a single facet value
type FacetValue struct {
	Value       string                `json:"value"`
	Count       int                   `json:"count"`
	Percentage  float64               `json:"percentage"`
}

// SearchConfig configures the search service
type SearchConfig struct {
	// Index configuration
	IndexPath       string            `json:"index_path"`
	MaxIndexSize    int64             `json:"max_index_size"`
	
	// Performance configuration
	Workers         int               `json:"workers"`
	BatchSize       int               `json:"batch_size"`
	ContentPreview  int               `json:"content_preview"`
	
	// Feature configuration
	SupportedTypes  []string          `json:"supported_types"`
	MaxFileSize     int64             `json:"max_file_size"`
	
	// Maintenance configuration
	ReindexInterval time.Duration     `json:"reindex_interval"`
	OptimizeInterval time.Duration    `json:"optimize_interval"`
	CleanupInterval time.Duration     `json:"cleanup_interval"`
	
	// Search configuration
	DefaultResults  int               `json:"default_results"`
	MaxResults      int               `json:"max_results"`
	DefaultTimeout  time.Duration     `json:"default_timeout"`
	
	// Cache configuration
	CacheSize       int               `json:"cache_size"`
	CacheTTL        time.Duration     `json:"cache_ttl"`
	
	// Encryption configuration
	EncryptIndex    bool              `json:"encrypt_index"`
	MasterKeyPath   string            `json:"master_key_path"`
}

// DefaultSearchConfig returns default search configuration
func DefaultSearchConfig() SearchConfig {
	return SearchConfig{
		IndexPath:       "~/.noisefs/search",
		MaxIndexSize:    1024 * 1024 * 1024, // 1GB
		Workers:         4,
		BatchSize:       100,
		ContentPreview:  500,
		SupportedTypes:  []string{"txt", "md", "pdf", "docx", "html", "json", "xml", "csv"},
		MaxFileSize:     100 * 1024 * 1024, // 100MB
		ReindexInterval: 24 * time.Hour,
		OptimizeInterval: 6 * time.Hour,
		CleanupInterval: 1 * time.Hour,
		DefaultResults:  20,
		MaxResults:      1000,
		DefaultTimeout:  30 * time.Second,
		CacheSize:       1000,
		CacheTTL:        15 * time.Minute,
		EncryptIndex:    true,
		MasterKeyPath:   "~/.noisefs/master.key",
	}
}

// SearchError represents a search-related error
type SearchError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

func (e SearchError) Error() string {
	if e.Details != "" {
		return e.Message + ": " + e.Details
	}
	return e.Message
}

// Common search errors
var (
	ErrInvalidQuery     = SearchError{Code: "invalid_query", Message: "Invalid search query"}
	ErrIndexNotFound    = SearchError{Code: "index_not_found", Message: "Search index not found"}
	ErrIndexCorrupt     = SearchError{Code: "index_corrupt", Message: "Search index is corrupted"}
	ErrIndexLocked      = SearchError{Code: "index_locked", Message: "Search index is locked"}
	ErrTimeout          = SearchError{Code: "timeout", Message: "Search operation timed out"}
	ErrPermissionDenied = SearchError{Code: "permission_denied", Message: "Permission denied"}
	ErrTooManyResults   = SearchError{Code: "too_many_results", Message: "Too many results"}
	ErrInvalidFilter    = SearchError{Code: "invalid_filter", Message: "Invalid filter"}
)
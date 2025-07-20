package search

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/index"
)

// SearchQueryType represents different types of search operations
type SearchQueryType int

const (
	FilenameSearch SearchQueryType = iota
	ContentSearch
	MetadataSearch
	SimilaritySearch
	ComplexSearch
)

// SearchQuery represents a user's search request with privacy context
type SearchQuery struct {
	// Core query parameters
	Query       string          `json:"query"`
	Type        SearchQueryType `json:"type"`
	MaxResults  int             `json:"max_results"`
	
	// Search options
	ExactMatch  bool     `json:"exact_match"`
	CaseSensitive bool   `json:"case_sensitive"`
	FileTypes   []string `json:"file_types,omitempty"`
	
	// Similarity search parameters
	SimilarityThreshold float64 `json:"similarity_threshold,omitempty"`
	ContentSample       []byte  `json:"content_sample,omitempty"`
	
	// Metadata filters
	SizeRange    *SizeRange    `json:"size_range,omitempty"`
	TimeRange    *TimeRange    `json:"time_range,omitempty"`
	CustomFilter map[string]interface{} `json:"custom_filter,omitempty"`
	
	// Privacy configuration
	PrivacyLevel    int           `json:"privacy_level"`
	SessionID       string        `json:"session_id"`
	RequestTime     time.Time     `json:"request_time"`
	PrivacyBudget   float64       `json:"privacy_budget"`
	
	// Internal processing fields
	ProcessedAt     time.Time     `json:"-"`
	ObfuscatedQuery string        `json:"-"`
	DummyQueries    []string      `json:"-"`
}

// SizeRange represents a file size range filter
type SizeRange struct {
	MinSize int64 `json:"min_size"`
	MaxSize int64 `json:"max_size"`
}

// TimeRange represents a time range filter
type TimeRange struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
}

// SearchQueryParser handles parsing and validation of search queries
type SearchQueryParser struct {
	maxQueryLength     int
	allowedFileTypes   map[string]bool
	sensitivePatterns  []*regexp.Regexp
	privacyLevels      map[int]PrivacyQueryConfig
}

// PrivacyQueryConfig defines privacy settings for query processing
type PrivacyQueryConfig struct {
	AddDummyQueries    bool    `json:"add_dummy_queries"`
	DummyQueryCount    int     `json:"dummy_query_count"`
	ObfuscateTerms     bool    `json:"obfuscate_terms"`
	RandomizeDelay     bool    `json:"randomize_delay"`
	MaxDelayMs         int     `json:"max_delay_ms"`
	PrivacyBudgetCost  float64 `json:"privacy_budget_cost"`
}

// QueryValidationError represents query validation errors
type QueryValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    string `json:"code"`
}

func (e *QueryValidationError) Error() string {
	return fmt.Sprintf("query validation error [%s]: %s (%s)", e.Field, e.Message, e.Code)
}

// NewSearchQueryParser creates a new query parser with privacy-aware configuration
func NewSearchQueryParser() *SearchQueryParser {
	// Define sensitive patterns that require special handling
	sensitivePatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(password|secret|key|token)\b`),
		regexp.MustCompile(`(?i)\b(private|confidential|classified)\b`),
		regexp.MustCompile(`(?i)\b(ssn|social|security)\b`),
	}

	// Define privacy levels with different protection strategies
	privacyLevels := map[int]PrivacyQueryConfig{
		1: { // Minimal privacy
			AddDummyQueries:   false,
			DummyQueryCount:   0,
			ObfuscateTerms:    false,
			RandomizeDelay:    false,
			MaxDelayMs:        0,
			PrivacyBudgetCost: 0.01,
		},
		2: { // Low privacy
			AddDummyQueries:   true,
			DummyQueryCount:   2,
			ObfuscateTerms:    false,
			RandomizeDelay:    true,
			MaxDelayMs:        100,
			PrivacyBudgetCost: 0.02,
		},
		3: { // Standard privacy
			AddDummyQueries:   true,
			DummyQueryCount:   5,
			ObfuscateTerms:    true,
			RandomizeDelay:    true,
			MaxDelayMs:        300,
			PrivacyBudgetCost: 0.05,
		},
		4: { // High privacy
			AddDummyQueries:   true,
			DummyQueryCount:   10,
			ObfuscateTerms:    true,
			RandomizeDelay:    true,
			MaxDelayMs:        500,
			PrivacyBudgetCost: 0.10,
		},
		5: { // Maximum privacy
			AddDummyQueries:   true,
			DummyQueryCount:   20,
			ObfuscateTerms:    true,
			RandomizeDelay:    true,
			MaxDelayMs:        1000,
			PrivacyBudgetCost: 0.20,
		},
	}

	// Common file types for validation
	allowedFileTypes := map[string]bool{
		"txt": true, "doc": true, "docx": true, "pdf": true,
		"jpg": true, "jpeg": true, "png": true, "gif": true,
		"mp3": true, "mp4": true, "avi": true, "mov": true,
		"zip": true, "tar": true, "gz": true, "rar": true,
		"exe": true, "dmg": true, "app": true, "deb": true,
	}

	return &SearchQueryParser{
		maxQueryLength:    1000,
		allowedFileTypes:  allowedFileTypes,
		sensitivePatterns: sensitivePatterns,
		privacyLevels:     privacyLevels,
	}
}

// ParseQuery parses and validates a search query string into a SearchQuery structure
func (p *SearchQueryParser) ParseQuery(queryStr string, options map[string]interface{}) (*SearchQuery, error) {
	// Basic validation
	if err := p.validateBasicQuery(queryStr); err != nil {
		return nil, err
	}

	// Create base query
	query := &SearchQuery{
		Query:        strings.TrimSpace(queryStr),
		Type:         FilenameSearch, // Default
		MaxResults:   100,           // Default
		PrivacyLevel: 3,             // Standard privacy by default
		RequestTime:  time.Now(),
		PrivacyBudget: 1.0,         // Full budget by default
	}

	// Parse options
	if err := p.parseOptions(query, options); err != nil {
		return nil, err
	}

	// Determine query type
	p.determineQueryType(query)

	// Apply privacy transformations
	if err := p.applyPrivacyTransforms(query); err != nil {
		return nil, err
	}

	return query, nil
}

// validateBasicQuery performs basic query validation
func (p *SearchQueryParser) validateBasicQuery(query string) error {
	if query == "" {
		return &QueryValidationError{
			Field:   "query",
			Message: "Query cannot be empty",
			Code:    "EMPTY_QUERY",
		}
	}

	if len(query) > p.maxQueryLength {
		return &QueryValidationError{
			Field:   "query",
			Message: fmt.Sprintf("Query too long (max %d characters)", p.maxQueryLength),
			Code:    "QUERY_TOO_LONG",
		}
	}

	// Check for potentially malicious patterns
	if strings.Contains(query, "..") || strings.Contains(query, "//") {
		return &QueryValidationError{
			Field:   "query",
			Message: "Query contains invalid path patterns",
			Code:    "INVALID_PATH_PATTERN",
		}
	}

	return nil
}

// parseOptions parses search options from the provided map
func (p *SearchQueryParser) parseOptions(query *SearchQuery, options map[string]interface{}) error {
	if options == nil {
		return nil
	}

	// Parse max results
	if maxResults, ok := options["max_results"].(int); ok {
		if maxResults <= 0 || maxResults > 10000 {
			return &QueryValidationError{
				Field:   "max_results",
				Message: "max_results must be between 1 and 10000",
				Code:    "INVALID_MAX_RESULTS",
			}
		}
		query.MaxResults = maxResults
	}

	// Parse privacy level
	if privacyLevel, ok := options["privacy_level"].(int); ok {
		if privacyLevel < 1 || privacyLevel > 5 {
			return &QueryValidationError{
				Field:   "privacy_level",
				Message: "privacy_level must be between 1 and 5",
				Code:    "INVALID_PRIVACY_LEVEL",
			}
		}
		query.PrivacyLevel = privacyLevel
	}

	// Parse session ID
	if sessionID, ok := options["session_id"].(string); ok {
		query.SessionID = sessionID
	}

	// Parse boolean options
	if exactMatch, ok := options["exact_match"].(bool); ok {
		query.ExactMatch = exactMatch
	}

	if caseSensitive, ok := options["case_sensitive"].(bool); ok {
		query.CaseSensitive = caseSensitive
	}

	// Parse file types
	if fileTypes, ok := options["file_types"].([]string); ok {
		validTypes := make([]string, 0, len(fileTypes))
		for _, ft := range fileTypes {
			if p.allowedFileTypes[strings.ToLower(ft)] {
				validTypes = append(validTypes, strings.ToLower(ft))
			}
		}
		query.FileTypes = validTypes
	}

	// Parse similarity threshold
	if threshold, ok := options["similarity_threshold"].(float64); ok {
		if threshold < 0.0 || threshold > 1.0 {
			return &QueryValidationError{
				Field:   "similarity_threshold",
				Message: "similarity_threshold must be between 0.0 and 1.0",
				Code:    "INVALID_SIMILARITY_THRESHOLD",
			}
		}
		query.SimilarityThreshold = threshold
	}

	// Parse size range
	if sizeRange, ok := options["size_range"].(map[string]interface{}); ok {
		sr := &SizeRange{}
		if minSize, ok := sizeRange["min_size"].(int64); ok {
			sr.MinSize = minSize
		}
		if maxSize, ok := sizeRange["max_size"].(int64); ok {
			sr.MaxSize = maxSize
		}
		if sr.MinSize < 0 || (sr.MaxSize > 0 && sr.MinSize > sr.MaxSize) {
			return &QueryValidationError{
				Field:   "size_range",
				Message: "Invalid size range",
				Code:    "INVALID_SIZE_RANGE",
			}
		}
		query.SizeRange = sr
	}

	return nil
}

// determineQueryType analyzes the query to determine the most appropriate search type
func (p *SearchQueryParser) determineQueryType(query *SearchQuery) {
	queryLower := strings.ToLower(query.Query)

	// Check for similarity search indicators
	if query.SimilarityThreshold > 0 || query.ContentSample != nil {
		query.Type = SimilaritySearch
		return
	}

	// Check for metadata search indicators
	if query.SizeRange != nil || query.TimeRange != nil || len(query.CustomFilter) > 0 {
		query.Type = MetadataSearch
		return
	}

	// Check for content search indicators
	contentIndicators := []string{"content:", "contains:", "text:", "body:"}
	for _, indicator := range contentIndicators {
		if strings.HasPrefix(queryLower, indicator) {
			query.Type = ContentSearch
			query.Query = strings.TrimPrefix(queryLower, indicator)
			return
		}
	}

	// Check for complex search indicators
	if strings.Contains(queryLower, " and ") || strings.Contains(queryLower, " or ") ||
		strings.Contains(queryLower, " not ") {
		query.Type = ComplexSearch
		return
	}

	// Default to filename search
	query.Type = FilenameSearch
}

// applyPrivacyTransforms applies privacy transformations based on the query's privacy level
func (p *SearchQueryParser) applyPrivacyTransforms(query *SearchQuery) error {
	config, exists := p.privacyLevels[query.PrivacyLevel]
	if !exists {
		config = p.privacyLevels[3] // Default to standard privacy
	}

	// Check privacy budget
	if query.PrivacyBudget < config.PrivacyBudgetCost {
		return &QueryValidationError{
			Field:   "privacy_budget",
			Message: "Insufficient privacy budget for this query",
			Code:    "INSUFFICIENT_PRIVACY_BUDGET",
		}
	}

	// Apply query obfuscation if enabled
	if config.ObfuscateTerms {
		query.ObfuscatedQuery = p.obfuscateQuery(query.Query)
	} else {
		query.ObfuscatedQuery = query.Query
	}

	// Generate dummy queries if enabled
	if config.AddDummyQueries {
		query.DummyQueries = p.generateDummyQueries(query.Query, config.DummyQueryCount)
	}

	// Check for sensitive patterns
	for _, pattern := range p.sensitivePatterns {
		if pattern.MatchString(query.Query) {
			// Automatically upgrade privacy level for sensitive queries
			if query.PrivacyLevel < 4 {
				query.PrivacyLevel = 4
				return p.applyPrivacyTransforms(query) // Reapply with higher level
			}
		}
	}

	query.ProcessedAt = time.Now()
	return nil
}

// obfuscateQuery applies term obfuscation to the query string
func (p *SearchQueryParser) obfuscateQuery(query string) string {
	// Simple obfuscation by adding noise characters
	words := strings.Fields(query)
	obfuscated := make([]string, len(words))

	for i, word := range words {
		if len(word) > 3 {
			// Insert noise characters in longer words
			mid := len(word) / 2
			obfuscated[i] = word[:mid] + "*" + word[mid:]
		} else {
			obfuscated[i] = word
		}
	}

	return strings.Join(obfuscated, " ")
}

// generateDummyQueries creates dummy queries to obfuscate search patterns
func (p *SearchQueryParser) generateDummyQueries(realQuery string, count int) []string {
	dummyTerms := []string{
		"document", "file", "report", "data", "image", "photo", "video", "music",
		"archive", "backup", "config", "log", "temp", "cache", "system", "user",
		"project", "test", "sample", "example", "template", "draft", "final",
		"notes", "readme", "license", "install", "setup", "script", "source",
	}

	dummy := make([]string, count)
	for i := 0; i < count; i++ {
		// Generate dummy query by combining random terms
		term1 := dummyTerms[i%len(dummyTerms)]
		term2 := dummyTerms[(i+7)%len(dummyTerms)]
		dummy[i] = fmt.Sprintf("%s_%s", term1, term2)
	}

	return dummy
}

// ToIndexQuery converts a SearchQuery to the format expected by the index system
func (query *SearchQuery) ToIndexQuery() *index.UnifiedSearchQuery {
	indexQuery := &index.UnifiedSearchQuery{
		MaxResults: query.MaxResults,
	}

	switch query.Type {
	case FilenameSearch:
		indexQuery.FilenameQuery = &index.FilenameQuery{
			Filename: query.ObfuscatedQuery,
		}

	case ContentSearch:
		indexQuery.ContentQuery = &index.ContentQuery{
			ContentSimilarity: &index.SimilarityQuery{
				Content:       []byte(query.ObfuscatedQuery),
				Threshold:     query.SimilarityThreshold,
				MaxCandidates: query.MaxResults,
			},
		}

	case MetadataSearch:
		metadataQuery := &index.MetadataQuery{}
		
		if query.SizeRange != nil {
			metadataQuery.SizeRange = &index.SizeRange{
				MinSize: query.SizeRange.MinSize,
				MaxSize: query.SizeRange.MaxSize,
			}
		}
		
		if query.TimeRange != nil {
			metadataQuery.TimeRange = &index.TimeRange{
				StartTime: query.TimeRange.StartTime,
				EndTime:   query.TimeRange.EndTime,
			}
		}
		
		if len(query.FileTypes) > 0 {
			metadataQuery.ContentTypes = query.FileTypes
		}
		
		metadataQuery.CustomAttrs = query.CustomFilter
		
		indexQuery.ContentQuery = &index.ContentQuery{
			MetadataFilter: metadataQuery,
		}
	}

	return indexQuery
}

// String returns a string representation of the search query for logging
func (query *SearchQuery) String() string {
	return fmt.Sprintf("SearchQuery{Type: %v, Query: %s, PrivacyLevel: %d, MaxResults: %d}",
		query.Type, query.ObfuscatedQuery, query.PrivacyLevel, query.MaxResults)
}
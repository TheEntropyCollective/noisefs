package search

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/index"
)

// PrivacySearchEngine provides privacy-preserving search capabilities
// by integrating Phase 1's privacy infrastructure with Day 2's indexing system
type PrivacySearchEngine struct {
	// Day 2 indexing integration
	indexManager *index.IndexManager
	
	// Phase 1 privacy components
	queryParser     *SearchQueryParser
	privacyTransformer *PrivacyQueryTransformer
	queryValidator  *QueryValidator
	
	// Core search execution
	searchExecutor   *SearchExecutor
	searchCoordinator *SearchCoordinator
	
	// Configuration and state
	config          *PrivacySearchConfig
	sessionManager  *SearchSessionManager
	
	// Statistics and monitoring
	stats           *SearchEngineStats
	
	// Thread safety
	mu              sync.RWMutex
}

// PrivacySearchConfig configures privacy-preserving search behavior
type PrivacySearchConfig struct {
	// Privacy settings
	DefaultPrivacyLevel     int           `json:"default_privacy_level"`
	MaxConcurrentQueries    int           `json:"max_concurrent_queries"`
	EnableQueryObfuscation  bool          `json:"enable_query_obfuscation"`
	EnableTimingObfuscation bool          `json:"enable_timing_obfuscation"`
	EnableResultNoise       bool          `json:"enable_result_noise"`
	
	// Performance settings
	MaxResults              int           `json:"max_results"`
	QueryTimeout            time.Duration `json:"query_timeout"`
	CacheEnabled            bool          `json:"cache_enabled"`
	CacheTTL                time.Duration `json:"cache_ttl"`
	
	// Integration settings
	UseIndexOptimizations   bool          `json:"use_index_optimizations"`
	EnableCrossIndexSearch  bool          `json:"enable_cross_index_search"`
	ParallelSearchEnabled   bool          `json:"parallel_search_enabled"`
}

// SearchEngineStats tracks comprehensive search engine statistics
type SearchEngineStats struct {
	// Query statistics
	TotalQueries         uint64        `json:"total_queries"`
	SuccessfulQueries    uint64        `json:"successful_queries"`
	FailedQueries        uint64        `json:"failed_queries"`
	AverageQueryTime     time.Duration `json:"average_query_time"`
	
	// Privacy statistics
	QueriesByPrivacyLevel map[int]uint64 `json:"queries_by_privacy_level"`
	DummyQueriesGenerated uint64         `json:"dummy_queries_generated"`
	TimingObfuscationCount uint64        `json:"timing_obfuscation_count"`
	
	// Performance statistics
	CacheHitRate         float64       `json:"cache_hit_rate"`
	IndexSearchTime      time.Duration `json:"index_search_time"`
	PrivacyProcessingTime time.Duration `json:"privacy_processing_time"`
	
	// System health
	SessionCount         int           `json:"active_sessions"`
	MemoryUsage          int64         `json:"memory_usage_bytes"`
	ErrorRate            float64       `json:"error_rate"`
	
	LastUpdated          time.Time     `json:"last_updated"`
}

// SearchResult represents a privacy-processed search result
type SearchResult struct {
	// Core result data
	FileID      string                 `json:"file_id"`
	Filename    string                 `json:"filename,omitempty"`
	Directory   string                 `json:"directory,omitempty"`
	ContentType string                 `json:"content_type,omitempty"`
	
	// Relevance and matching
	Relevance   float64                `json:"relevance"`
	MatchType   string                 `json:"match_type"`
	Similarity  float64                `json:"similarity,omitempty"`
	
	// Privacy-filtered metadata
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	
	// Privacy information
	PrivacyLevel int                   `json:"privacy_level"`
	NoiseLevel   float64               `json:"noise_level"`
	
	// Source information
	Sources     []string               `json:"sources"`
	IndexSource string                 `json:"index_source"`
}

// SearchResponse represents the complete response to a search query
type SearchResponse struct {
	// Results
	Results      []SearchResult `json:"results"`
	TotalResults int           `json:"total_results"`
	HasMore      bool          `json:"has_more"`
	
	// Query information
	Query        *SearchQuery  `json:"query"`
	QueryTime    time.Duration `json:"query_time"`
	
	// Privacy information
	PrivacyLevel     int     `json:"privacy_level"`
	DummyQueries     int     `json:"dummy_queries"`
	TimingDelay      time.Duration `json:"timing_delay"`
	NoiseInjected    bool    `json:"noise_injected"`
	
	// Pagination
	Offset       int     `json:"offset"`
	Limit        int     `json:"limit"`
	
	// Metadata
	SearchID     string    `json:"search_id"`
	Timestamp    time.Time `json:"timestamp"`
}

// NewPrivacySearchEngine creates a new privacy-preserving search engine
func NewPrivacySearchEngine(indexManager *index.IndexManager, config *PrivacySearchConfig) (*PrivacySearchEngine, error) {
	if indexManager == nil {
		return nil, fmt.Errorf("index manager cannot be nil")
	}
	
	if config == nil {
		config = DefaultPrivacySearchConfig()
	}
	
	if err := validatePrivacySearchConfig(config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	
	// Initialize Phase 1 privacy components
	queryParser := NewSearchQueryParser()
	privacyTransformer := NewPrivacyQueryTransformer()
	queryValidator := NewQueryValidator()
	
	// Initialize search execution components
	searchExecutor := NewSearchExecutor(indexManager, config)
	searchCoordinator := NewSearchCoordinator(config)
	
	// Initialize session manager
	sessionManager := NewSearchSessionManager()
	
	// Initialize statistics
	stats := &SearchEngineStats{
		QueriesByPrivacyLevel: make(map[int]uint64),
		LastUpdated:          time.Now(),
	}
	
	return &PrivacySearchEngine{
		indexManager:       indexManager,
		queryParser:        queryParser,
		privacyTransformer: privacyTransformer,
		queryValidator:     queryValidator,
		searchExecutor:     searchExecutor,
		searchCoordinator:  searchCoordinator,
		config:            config,
		sessionManager:    sessionManager,
		stats:             stats,
	}, nil
}

// Search performs a privacy-preserving search operation
func (pse *PrivacySearchEngine) Search(ctx context.Context, queryStr string, options map[string]interface{}) (*SearchResponse, error) {
	startTime := time.Now()
	
	// Generate search ID for tracking
	searchID := pse.generateSearchID()
	
	// Update statistics
	pse.updateQueryStats()
	
	// Parse and validate query
	query, err := pse.queryParser.ParseQuery(queryStr, options)
	if err != nil {
		pse.updateFailureStats()
		return nil, fmt.Errorf("query parsing failed: %w", err)
	}
	
	// Validate query with security checks
	validationResult, err := pse.queryValidator.ValidateQuery(query, pse.getClientIP(ctx))
	if err != nil {
		pse.updateFailureStats()
		return nil, fmt.Errorf("query validation failed: %w", err)
	}
	
	if !validationResult.Valid || validationResult.Blocked {
		pse.updateFailureStats()
		return &SearchResponse{
			Query:        query,
			Results:      []SearchResult{},
			TotalResults: 0,
			QueryTime:    time.Since(startTime),
			PrivacyLevel: query.PrivacyLevel,
			SearchID:     searchID,
			Timestamp:    time.Now(),
		}, nil
	}
	
	// Apply privacy transformations
	transformResult, err := pse.privacyTransformer.Transform(query)
	if err != nil {
		pse.updateFailureStats()
		return nil, fmt.Errorf("privacy transformation failed: %w", err)
	}
	
	// Update session tracking
	if err := pse.sessionManager.UpdateSession(query.SessionID, query); err != nil {
		// Log but don't fail the search
		// In full implementation, this would use proper logging
	}
	
	// Execute privacy-preserving search
	searchResults, err := pse.executePrivacySearch(ctx, transformResult)
	if err != nil {
		pse.updateFailureStats()
		return nil, fmt.Errorf("search execution failed: %w", err)
	}
	
	// Apply timing obfuscation if needed
	if transformResult.TimingDelay > 0 {
		select {
		case <-time.After(transformResult.TimingDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		pse.stats.TimingObfuscationCount++
	}
	
	// Create response
	response := &SearchResponse{
		Results:       searchResults,
		TotalResults:  len(searchResults),
		HasMore:       len(searchResults) >= query.MaxResults,
		Query:         query,
		QueryTime:     time.Since(startTime),
		PrivacyLevel:  query.PrivacyLevel,
		DummyQueries:  len(transformResult.DummyQueries),
		TimingDelay:   transformResult.TimingDelay,
		NoiseInjected: transformResult.NoiseLevel > 0,
		SearchID:      searchID,
		Timestamp:     time.Now(),
	}
	
	// Apply pagination
	pse.applyPagination(response, query)
	
	// Update success statistics
	pse.updateSuccessStats(time.Since(startTime), query.PrivacyLevel)
	
	return response, nil
}

// executePrivacySearch executes the search with privacy protection
func (pse *PrivacySearchEngine) executePrivacySearch(ctx context.Context, transformResult *TransformResult) ([]SearchResult, error) {
	// Create search context with privacy information
	searchCtx := &PrivacySearchContext{
		OriginalQuery:    transformResult.TransformedQuery,
		DummyQueries:     transformResult.DummyQueries,
		KAnonymityGroup:  transformResult.KAnonymityGroup,
		NoiseLevel:       transformResult.NoiseLevel,
		PrivacyCost:      transformResult.PrivacyCost,
	}
	
	// Execute search through coordinator for privacy protection
	return pse.searchCoordinator.ExecuteSearch(ctx, searchCtx, pse.searchExecutor)
}

// GetStats returns comprehensive search engine statistics
func (pse *PrivacySearchEngine) GetStats() *SearchEngineStats {
	pse.mu.RLock()
	defer pse.mu.RUnlock()
	
	// Create a copy for thread safety
	statsCopy := *pse.stats
	statsCopy.LastUpdated = time.Now()
	statsCopy.SessionCount = pse.sessionManager.GetActiveSessionCount()
	
	return &statsCopy
}

// UpdateConfig updates the search engine configuration
func (pse *PrivacySearchEngine) UpdateConfig(config *PrivacySearchConfig) error {
	if err := validatePrivacySearchConfig(config); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}
	
	pse.mu.Lock()
	defer pse.mu.Unlock()
	
	pse.config = config
	
	// Update component configurations
	pse.searchExecutor.UpdateConfig(config)
	pse.searchCoordinator.UpdateConfig(config)
	
	return nil
}

// ClearSessions clears all active search sessions
func (pse *PrivacySearchEngine) ClearSessions() error {
	return pse.sessionManager.ClearAllSessions()
}

// Helper methods

// generateSearchID generates a unique search identifier
func (pse *PrivacySearchEngine) generateSearchID() string {
	return fmt.Sprintf("search_%d_%d", time.Now().UnixNano(), pse.stats.TotalQueries)
}

// getClientIP extracts client IP from context (simplified)
func (pse *PrivacySearchEngine) getClientIP(ctx context.Context) string {
	if ip, ok := ctx.Value("client_ip").(string); ok {
		return ip
	}
	return "unknown"
}

// applyPagination applies pagination to search results
func (pse *PrivacySearchEngine) applyPagination(response *SearchResponse, query *SearchQuery) {
	// This would implement proper pagination
	// For now, just set basic pagination info
	response.Offset = 0
	response.Limit = query.MaxResults
}

// Statistics update methods

func (pse *PrivacySearchEngine) updateQueryStats() {
	pse.mu.Lock()
	defer pse.mu.Unlock()
	pse.stats.TotalQueries++
}

func (pse *PrivacySearchEngine) updateSuccessStats(queryTime time.Duration, privacyLevel int) {
	pse.mu.Lock()
	defer pse.mu.Unlock()
	
	pse.stats.SuccessfulQueries++
	pse.stats.QueriesByPrivacyLevel[privacyLevel]++
	
	// Update average query time (exponential moving average)
	alpha := 0.1
	pse.stats.AverageQueryTime = time.Duration(
		alpha*float64(queryTime) + (1-alpha)*float64(pse.stats.AverageQueryTime),
	)
}

func (pse *PrivacySearchEngine) updateFailureStats() {
	pse.mu.Lock()
	defer pse.mu.Unlock()
	pse.stats.FailedQueries++
}

// Configuration validation and defaults

func validatePrivacySearchConfig(config *PrivacySearchConfig) error {
	if config.DefaultPrivacyLevel < 1 || config.DefaultPrivacyLevel > 5 {
		return fmt.Errorf("default privacy level must be between 1 and 5")
	}
	
	if config.MaxConcurrentQueries < 1 {
		return fmt.Errorf("max concurrent queries must be positive")
	}
	
	if config.QueryTimeout < time.Millisecond {
		return fmt.Errorf("query timeout must be positive")
	}
	
	return nil
}

// DefaultPrivacySearchConfig returns default configuration
func DefaultPrivacySearchConfig() *PrivacySearchConfig {
	return &PrivacySearchConfig{
		DefaultPrivacyLevel:     3,
		MaxConcurrentQueries:    10,
		EnableQueryObfuscation:  true,
		EnableTimingObfuscation: true,
		EnableResultNoise:       true,
		MaxResults:              100,
		QueryTimeout:            time.Second * 30,
		CacheEnabled:            true,
		CacheTTL:                time.Minute * 15,
		UseIndexOptimizations:   true,
		EnableCrossIndexSearch:  true,
		ParallelSearchEnabled:   true,
	}
}
package search

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/fuse"
)

// PrivacySearchEngine provides privacy-preserving search capabilities
type PrivacySearchEngine struct {
	// Core components (from Phase 2)
	fileIndex         *fuse.FileIndex
	queryParser       *SearchQueryParser
	privacyTransformer *PrivacyQueryTransformer
	queryValidator    *QueryValidator
	searchExecutor    *SearchExecutor
	searchCoordinator *SearchCoordinator
	
	// Phase 3 components
	resultProcessor   *AdvancedResultProcessor
	cacheManager     *PrivacyCacheManager
	sessionManager   *EnhancedSessionManager
	analytics        *PrivacySearchAnalytics
	
	// Configuration
	config           *PrivacySearchConfig
	
	// Statistics
	stats            *SearchStats
	
	// Thread safety
	mu               sync.RWMutex
}

// SearchQueryParser parses and validates search queries
type SearchQueryParser struct {
	maxQueryLength   int
	allowedTypes     map[SearchQueryType]bool
	mu               sync.RWMutex
}

// NewPrivacySearchEngine creates a new privacy search engine
func NewPrivacySearchEngine(fileIndex *fuse.FileIndex, config *PrivacySearchConfig) (*PrivacySearchEngine, error) {
	if fileIndex == nil {
		return nil, fmt.Errorf("file index is required")
	}
	
	if config == nil {
		config = DefaultPrivacySearchConfig()
	}
	
	engine := &PrivacySearchEngine{
		fileIndex: fileIndex,
		config:       config,
		stats: &SearchStats{
			QueriesByPrivacyLevel: make(map[int]uint64),
			ErrorsByType:          make(map[string]uint64),
			StartTime:             time.Now(),
			LastUpdated:           time.Now(),
		},
	}
	
	// Initialize core components (Phase 1 & 2)
	engine.queryParser = NewSearchQueryParser()
	engine.privacyTransformer = NewPrivacyQueryTransformer()
	engine.queryValidator = NewQueryValidator()
	engine.searchExecutor = NewSearchExecutor(fileIndex, config)
	engine.searchCoordinator = NewSearchCoordinator(config)
	
	// Initialize Phase 3 components
	engine.resultProcessor = NewAdvancedResultProcessor(DefaultResultPrivacyConfig())
	engine.cacheManager = NewPrivacyCacheManager(DefaultCachePrivacyConfig())
	engine.sessionManager = NewEnhancedSessionManager(DefaultSessionConfig())
	engine.analytics = NewPrivacySearchAnalytics(DefaultAnalyticsConfig())
	
	return engine, nil
}

// Search performs a privacy-preserving search
func (pse *PrivacySearchEngine) Search(ctx context.Context, queryText string, options map[string]interface{}) (*SearchResponse, error) {
	startTime := time.Now()
	
	pse.mu.Lock()
	defer pse.mu.Unlock()
	
	// Update statistics
	pse.stats.TotalQueries++
	pse.stats.LastUpdated = time.Now()
	pse.stats.UptimeDuration = time.Since(pse.stats.StartTime)
	
	// Parse search options
	searchOptions, err := pse.parseSearchOptions(options)
	if err != nil {
		pse.stats.FailedQueries++
		pse.stats.ErrorsByType[ErrorTypeInvalidQuery]++
		return nil, fmt.Errorf("invalid search options: %w", err)
	}
	
	// Create search query
	query := &SearchQuery{
		Query:           queryText,
		ObfuscatedQuery: queryText,
		Type:            searchOptions.QueryType,
		MaxResults:      searchOptions.MaxResults,
		PrivacyLevel:    searchOptions.PrivacyLevel,
		SessionID:       searchOptions.SessionID,
		RequestTime:     startTime,
		UserID:          searchOptions.UserID,
	}
	
	// Validate query
	validationResult, err := pse.queryValidator.ValidateQuery(query, searchOptions.ClientIP)
	if err != nil {
		pse.stats.FailedQueries++
		pse.stats.ErrorsByType[ErrorTypeInternal]++
		return nil, fmt.Errorf("query validation failed: %w", err)
	}
	
	if !validationResult.Valid || validationResult.Blocked {
		pse.stats.FailedQueries++
		if validationResult.RateLimited {
			pse.stats.ErrorsByType[ErrorTypeRateLimit]++
		} else {
			pse.stats.SecurityViolations++
		}
		return nil, fmt.Errorf("query blocked: validation failed")
	}
	
	// Check cache first
	if pse.config.CacheEnabled {
		cachedResults, found := pse.cacheManager.GetCachedResults(query)
		if found {
			pse.stats.CachedQueries++
			pse.stats.CacheHits++
			
			response := &SearchResponse{
				Query:           query,
				SearchID:        fmt.Sprintf("search_%d", time.Now().UnixNano()),
				Results:         cachedResults,
				TotalResults:    len(cachedResults),
				PrivacyLevel:    query.PrivacyLevel,
				Duration:        time.Since(startTime),
				CacheHit:        true,
				Timestamp:       time.Now(),
				Version:         "1.0",
			}
			
			// Update session
			if pse.sessionManager != nil {
				pse.sessionManager.UpdateSession(query.SessionID, query)
			}
			
			return response, nil
		}
		pse.stats.CacheMisses++
	}
	
	// Apply privacy transformations
	transformResult, err := pse.privacyTransformer.Transform(query)
	if err != nil {
		pse.stats.FailedQueries++
		pse.stats.ErrorsByType[ErrorTypePrivacyViolation]++
		return nil, fmt.Errorf("privacy transformation failed: %w", err)
	}
	
	// Update query with transformation results
	query = transformResult.TransformedQuery
	dummyQueryCount := len(transformResult.DummyQueries)
	pse.stats.DummyQueriesGenerated += uint64(dummyQueryCount)
	
	// Update session before search
	if pse.sessionManager != nil {
		err = pse.sessionManager.UpdateSession(query.SessionID, query)
		if err != nil {
			// Log but don't fail the search
			pse.stats.ErrorsByType[ErrorTypeInternal]++
		}
	}
	
	// Execute search
	searchResults, err := pse.searchExecutor.ExecuteSearch(ctx, query)
	if err != nil {
		pse.stats.FailedQueries++
		pse.stats.ErrorsByType[ErrorTypeInternal]++
		return nil, fmt.Errorf("search execution failed: %w", err)
	}
	
	// Apply advanced result processing (Phase 3)
	processedResults, err := pse.resultProcessor.ProcessResults(ctx, searchResults, query)
	if err != nil {
		pse.stats.FailedQueries++
		pse.stats.ErrorsByType[ErrorTypeInternal]++
		return nil, fmt.Errorf("result processing failed: %w", err)
	}
	
	// Cache results if enabled
	if pse.config.CacheEnabled && len(processedResults) > 0 {
		err = pse.cacheManager.CacheResults(query, processedResults)
		if err != nil {
			// Log but don't fail the search
			pse.stats.ErrorsByType[ErrorTypeInternal]++
		}
	}
	
	// Record analytics
	if pse.analytics != nil {
		metric := &SearchMetric{
			MetricType:        "search_execution",
			QueryType:         query.Type,
			PrivacyLevel:      query.PrivacyLevel,
			ResponseTime:      time.Since(startTime),
			ResultCount:       len(processedResults),
			CacheHit:          false,
			NoiseLevel:        transformResult.NoiseLevel,
			PrivacyBudgetUsed: transformResult.PrivacyCost,
			Timestamp:         time.Now(),
			SessionID:         query.SessionID,
		}
		
		err = pse.analytics.RecordSearchMetric(metric)
		if err != nil {
			// Log but don't fail the search
			pse.stats.ErrorsByType[ErrorTypeInternal]++
		}
	}
	
	// Record query result in session
	if pse.sessionManager != nil {
		queryResult := &QueryResult{
			ResultCount:       len(processedResults),
			PrivacyBudgetUsed: transformResult.PrivacyCost,
			CacheHit:          false,
			Duration:          time.Since(startTime),
		}
		
		err = pse.sessionManager.RecordQueryResult(query.SessionID, queryResult)
		if err != nil {
			// Log but don't fail the search
			pse.stats.ErrorsByType[ErrorTypeInternal]++
		}
	}
	
	// Update statistics
	pse.stats.SuccessfulQueries++
	pse.stats.QueriesByPrivacyLevel[query.PrivacyLevel]++
	if transformResult.NoiseLevel > 0 {
		pse.stats.NoiseApplications++
	}
	
	// Update average response time (exponential moving average)
	alpha := 0.1
	currentDuration := time.Since(startTime)
	pse.stats.AverageResponseTime = time.Duration(
		alpha*float64(currentDuration) + (1-alpha)*float64(pse.stats.AverageResponseTime),
	)
	
	// Create response
	response := &SearchResponse{
		Query:           query,
		SearchID:        fmt.Sprintf("search_%d", time.Now().UnixNano()),
		Results:         processedResults,
		TotalResults:    len(processedResults),
		PrivacyLevel:    query.PrivacyLevel,
		DummyQueries:    dummyQueryCount,
		NoiseApplied:    transformResult.NoiseLevel > 0,
		Duration:        currentDuration,
		CacheHit:        false,
		BudgetUsed:      transformResult.PrivacyCost,
		Timestamp:       time.Now(),
		Version:         "1.0",
	}
	
	return response, nil
}

// parseSearchOptions parses search options from the options map
func (pse *PrivacySearchEngine) parseSearchOptions(options map[string]interface{}) (*ParsedSearchOptions, error) {
	opts := &ParsedSearchOptions{
		QueryType:    FilenameSearch,
		MaxResults:   pse.config.MaxResults,
		PrivacyLevel: pse.config.DefaultPrivacyLevel,
		ClientIP:     "127.0.0.1",
	}
	
	if val, exists := options["privacy_level"]; exists {
		if level, ok := val.(int); ok && level >= 1 && level <= pse.config.MaxPrivacyLevel {
			opts.PrivacyLevel = level
		}
	}
	
	if val, exists := options["session_id"]; exists {
		if sessionID, ok := val.(string); ok {
			opts.SessionID = sessionID
		}
	}
	
	if val, exists := options["max_results"]; exists {
		if maxResults, ok := val.(int); ok && maxResults > 0 && maxResults <= pse.config.MaxResults {
			opts.MaxResults = maxResults
		}
	}
	
	if val, exists := options["user_id"]; exists {
		if userID, ok := val.(string); ok {
			opts.UserID = userID
		}
	}
	
	if val, exists := options["client_ip"]; exists {
		if clientIP, ok := val.(string); ok {
			opts.ClientIP = clientIP
		}
	}
	
	return opts, nil
}

// ParsedSearchOptions represents parsed search options
type ParsedSearchOptions struct {
	QueryType    SearchQueryType
	MaxResults   int
	PrivacyLevel int
	SessionID    string
	UserID       string
	ClientIP     string
}

// GetStats returns search engine statistics
func (pse *PrivacySearchEngine) GetStats() *SearchStats {
	pse.mu.RLock()
	defer pse.mu.RUnlock()
	
	// Return a copy to prevent external modification
	statsCopy := *pse.stats
	statsCopy.QueriesByPrivacyLevel = make(map[int]uint64)
	for k, v := range pse.stats.QueriesByPrivacyLevel {
		statsCopy.QueriesByPrivacyLevel[k] = v
	}
	statsCopy.ErrorsByType = make(map[string]uint64)
	for k, v := range pse.stats.ErrorsByType {
		statsCopy.ErrorsByType[k] = v
	}
	
	return &statsCopy
}

// UpdateConfig updates the search engine configuration
func (pse *PrivacySearchEngine) UpdateConfig(config *PrivacySearchConfig) error {
	pse.mu.Lock()
	defer pse.mu.Unlock()
	
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}
	
	pse.config = config
	
	// Update component configurations
	if pse.searchExecutor != nil {
		pse.searchExecutor.UpdateConfig(config)
	}
	
	return nil
}

// Close shuts down the search engine
func (pse *PrivacySearchEngine) Close() error {
	pse.mu.Lock()
	defer pse.mu.Unlock()
	
	// Cleanup components
	if pse.sessionManager != nil {
		err := pse.sessionManager.CleanupExpiredSessions()
		if err != nil {
			return fmt.Errorf("failed to cleanup sessions: %w", err)
		}
	}
	
	return nil
}

// Helper function implementations

// NewSearchQueryParser creates a new search query parser
func NewSearchQueryParser() *SearchQueryParser {
	return &SearchQueryParser{
		maxQueryLength: 1000,
		allowedTypes: map[SearchQueryType]bool{
			FilenameSearch:   true,
			ContentSearch:    true,
			MetadataSearch:   true,
			SimilaritySearch: true,
			ComplexSearch:    true,
		},
	}
}

// NewSearchCoordinator creates a new search coordinator
func NewSearchCoordinator(config *PrivacySearchConfig) *SearchCoordinator {
	return &SearchCoordinator{
		config:         config,
		sessionManager: &SessionManager{
			sessions: make(map[string]*SearchSession),
		},
	}
}

// SearchCoordinator coordinates search operations with privacy protection
type SearchCoordinator struct {
	config         *PrivacySearchConfig
	sessionManager *SessionManager
	mu             sync.RWMutex
}

// SessionManager manages search sessions for coordination
type SessionManager struct {
	sessions map[string]*SearchSession
	mu       sync.RWMutex
}

// SearchSession represents an active search session
type SearchSession struct {
	SessionID    string
	CreatedAt    time.Time
	LastActivity time.Time
	QueryCount   int
}

// GetActiveSessionCount returns the number of active sessions
func (sm *SessionManager) GetActiveSessionCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.sessions)
}

// UpdateSession updates session activity
func (sm *SessionManager) UpdateSession(sessionID string, query *SearchQuery) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	session, exists := sm.sessions[sessionID]
	if !exists {
		session = &SearchSession{
			SessionID: sessionID,
			CreatedAt: time.Now(),
		}
		sm.sessions[sessionID] = session
	}
	
	session.LastActivity = time.Now()
	session.QueryCount++
	
	return nil
}
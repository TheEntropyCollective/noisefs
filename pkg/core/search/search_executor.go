package search

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/index"
)

// SearchExecutor handles core search execution against Day 2's indexing infrastructure
type SearchExecutor struct {
	// Day 2 indexing integration
	indexManager *index.IndexManager
	
	// Configuration
	config          *PrivacySearchConfig
	
	// Performance optimization
	resultCache     *SearchResultCache
	queryOptimizer  *QueryOptimizer
	
	// Thread safety and coordination
	mu              sync.RWMutex
	activeSearches  map[string]*ActiveSearch
}

// ActiveSearch tracks an ongoing search operation
type ActiveSearch struct {
	SearchID    string
	Query       *SearchQuery
	StartTime   time.Time
	Context     context.Context
	Cancel      context.CancelFunc
}

// SearchResultCache caches search results for performance
type SearchResultCache struct {
	cache     map[string]*CachedResult
	ttl       time.Duration
	mu        sync.RWMutex
}

// CachedResult represents a cached search result
type CachedResult struct {
	Results   []SearchResult
	Timestamp time.Time
	QueryHash string
}

// QueryOptimizer optimizes queries for better performance
type QueryOptimizer struct {
	// Query pattern analysis
	commonPatterns   map[string]*QueryPattern
	optimizations    map[string]QueryOptimization
	
	// Performance metrics
	executionTimes   map[string][]time.Duration
	mu               sync.RWMutex
}

// QueryOptimization represents an optimization for a query pattern
type QueryOptimization struct {
	Pattern        string
	IndexPreference []string  // Preferred order of index types
	ParallelSafe   bool       // Whether query can be parallelized
	CacheEnabled   bool       // Whether result should be cached
	EstimatedTime  time.Duration
}

// NewSearchExecutor creates a new search executor
func NewSearchExecutor(indexManager *index.IndexManager, config *PrivacySearchConfig) *SearchExecutor {
	cache := &SearchResultCache{
		cache: make(map[string]*CachedResult),
		ttl:   config.CacheTTL,
	}
	
	optimizer := &QueryOptimizer{
		commonPatterns:  make(map[string]*QueryPattern),
		optimizations:   make(map[string]QueryOptimization),
		executionTimes:  make(map[string][]time.Duration),
	}
	
	return &SearchExecutor{
		indexManager:   indexManager,
		config:        config,
		resultCache:   cache,
		queryOptimizer: optimizer,
		activeSearches: make(map[string]*ActiveSearch),
	}
}

// ExecuteSearch executes a search query against the index system
func (se *SearchExecutor) ExecuteSearch(ctx context.Context, query *SearchQuery) ([]SearchResult, error) {
	searchID := fmt.Sprintf("exec_%d", time.Now().UnixNano())
	
	// Register active search
	se.registerActiveSearch(searchID, query, ctx)
	defer se.unregisterActiveSearch(searchID)
	
	// Check cache first if enabled
	if se.config.CacheEnabled {
		if cached := se.checkCache(query); cached != nil {
			return cached.Results, nil
		}
	}
	
	// Optimize query for execution
	optimizedQuery, optimization := se.queryOptimizer.OptimizeQuery(query)
	
	// Execute search based on query type and optimization
	var results []SearchResult
	var err error
	
	if optimization.ParallelSafe && se.config.ParallelSearchEnabled {
		results, err = se.executeParallelSearch(ctx, optimizedQuery)
	} else {
		results, err = se.executeSequentialSearch(ctx, optimizedQuery)
	}
	
	if err != nil {
		return nil, fmt.Errorf("search execution failed: %w", err)
	}
	
	// Post-process results
	results = se.postProcessResults(results, query)
	
	// Cache results if enabled
	if se.config.CacheEnabled && optimization.CacheEnabled {
		se.cacheResults(query, results)
	}
	
	// Update optimizer with execution metrics
	se.queryOptimizer.UpdateMetrics(query, optimization, time.Since(time.Now()))
	
	return results, nil
}

// executeParallelSearch executes search using parallel operations
func (se *SearchExecutor) executeParallelSearch(ctx context.Context, query *SearchQuery) ([]SearchResult, error) {
	var wg sync.WaitGroup
	resultsChan := make(chan []SearchResult, 3) // Max 3 index types
	errorsChan := make(chan error, 3)
	
	// Convert to unified search query
	unifiedQuery := query.ToIndexQuery()
	
	// Execute privacy index search
	if unifiedQuery.FilenameQuery != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results, err := se.executePrivacyIndexSearch(ctx, unifiedQuery.FilenameQuery)
			if err != nil {
				errorsChan <- err
				return
			}
			resultsChan <- results
		}()
	}
	
	// Execute manifest index search
	if unifiedQuery.DirectoryQuery != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results, err := se.executeManifestIndexSearch(ctx, unifiedQuery.DirectoryQuery)
			if err != nil {
				errorsChan <- err
				return
			}
			resultsChan <- results
		}()
	}
	
	// Execute content index search
	if unifiedQuery.ContentQuery != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results, err := se.executeContentIndexSearch(ctx, unifiedQuery.ContentQuery)
			if err != nil {
				errorsChan <- err
				return
			}
			resultsChan <- results
		}()
	}
	
	// Wait for completion
	go func() {
		wg.Wait()
		close(resultsChan)
		close(errorsChan)
	}()
	
	// Collect results
	var allResults []SearchResult
	var errors []error
	
	for {
		select {
		case results, ok := <-resultsChan:
			if !ok {
				resultsChan = nil
				break
			}
			allResults = append(allResults, results...)
		case err, ok := <-errorsChan:
			if !ok {
				errorsChan = nil
				break
			}
			errors = append(errors, err)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		
		if resultsChan == nil && errorsChan == nil {
			break
		}
	}
	
	// Handle errors
	if len(errors) > 0 {
		return nil, fmt.Errorf("parallel search had %d errors: %v", len(errors), errors[0])
	}
	
	return se.mergeAndRankResults(allResults), nil
}

// executeSequentialSearch executes search using sequential operations
func (se *SearchExecutor) executeSequentialSearch(ctx context.Context, query *SearchQuery) ([]SearchResult, error) {
	// Convert to unified search query for index manager
	unifiedQuery := query.ToIndexQuery()
	
	// Execute search through index manager
	unifiedResult, err := se.indexManager.SearchFiles(unifiedQuery)
	if err != nil {
		return nil, fmt.Errorf("index search failed: %w", err)
	}
	
	// Convert unified results to search results
	return se.convertUnifiedResults(unifiedResult, query), nil
}

// Individual index search methods

func (se *SearchExecutor) executePrivacyIndexSearch(ctx context.Context, filenameQuery *index.FilenameQuery) ([]SearchResult, error) {
	// Create a simplified unified query for just filename search
	unifiedQuery := &index.UnifiedSearchQuery{
		FilenameQuery: filenameQuery,
		MaxResults:   se.config.MaxResults,
	}
	
	result, err := se.indexManager.SearchFiles(unifiedQuery)
	if err != nil {
		return nil, err
	}
	
	return se.convertUnifiedResultsWithSource(result, "privacy"), nil
}

func (se *SearchExecutor) executeManifestIndexSearch(ctx context.Context, directoryQuery *index.DirectoryQuery) ([]SearchResult, error) {
	// Create a simplified unified query for just directory search
	unifiedQuery := &index.UnifiedSearchQuery{
		DirectoryQuery: directoryQuery,
		MaxResults:     se.config.MaxResults,
	}
	
	result, err := se.indexManager.SearchFiles(unifiedQuery)
	if err != nil {
		return nil, err
	}
	
	return se.convertUnifiedResultsWithSource(result, "manifest"), nil
}

func (se *SearchExecutor) executeContentIndexSearch(ctx context.Context, contentQuery *index.ContentQuery) ([]SearchResult, error) {
	// Create a simplified unified query for just content search
	unifiedQuery := &index.UnifiedSearchQuery{
		ContentQuery: contentQuery,
		MaxResults:   se.config.MaxResults,
	}
	
	result, err := se.indexManager.SearchFiles(unifiedQuery)
	if err != nil {
		return nil, err
	}
	
	return se.convertUnifiedResultsWithSource(result, "content"), nil
}

// Result processing methods

func (se *SearchExecutor) convertUnifiedResults(unifiedResult *index.UnifiedSearchResult, query *SearchQuery) []SearchResult {
	results := make([]SearchResult, 0, len(unifiedResult.Matches))
	
	for _, match := range unifiedResult.Matches {
		result := SearchResult{
			FileID:       match.FileID,
			Relevance:    match.Relevance,
			MatchType:    match.MatchType,
			Similarity:   match.Similarity,
			Sources:      match.Sources,
			IndexSource:  match.Source,
			PrivacyLevel: query.PrivacyLevel,
		}
		
		// If we have multiple sources, use the primary one
		if len(match.Sources) > 0 {
			result.IndexSource = match.Sources[0]
		}
		
		results = append(results, result)
	}
	
	return results
}

func (se *SearchExecutor) convertUnifiedResultsWithSource(unifiedResult *index.UnifiedSearchResult, source string) []SearchResult {
	results := make([]SearchResult, 0, len(unifiedResult.Matches))
	
	for _, match := range unifiedResult.Matches {
		result := SearchResult{
			FileID:      match.FileID,
			Relevance:   match.Relevance,
			MatchType:   match.MatchType,
			Similarity:  match.Similarity,
			Sources:     []string{source},
			IndexSource: source,
		}
		
		results = append(results, result)
	}
	
	return results
}

func (se *SearchExecutor) mergeAndRankResults(results []SearchResult) []SearchResult {
	// Group results by FileID
	fileGroups := make(map[string][]SearchResult)
	for _, result := range results {
		fileGroups[result.FileID] = append(fileGroups[result.FileID], result)
	}
	
	// Merge grouped results
	mergedResults := make([]SearchResult, 0, len(fileGroups))
	for fileID, group := range fileGroups {
		merged := se.mergeResultGroup(fileID, group)
		mergedResults = append(mergedResults, merged)
	}
	
	// Sort by relevance (descending)
	sort.Slice(mergedResults, func(i, j int) bool {
		return mergedResults[i].Relevance > mergedResults[j].Relevance
	})
	
	return mergedResults
}

func (se *SearchExecutor) mergeResultGroup(fileID string, group []SearchResult) SearchResult {
	if len(group) == 1 {
		return group[0]
	}
	
	merged := SearchResult{
		FileID:      fileID,
		MatchType:   "combined",
		Sources:     make([]string, 0),
		Relevance:   0.0,
	}
	
	// Combine sources and calculate weighted relevance
	totalWeight := 0.0
	for _, result := range group {
		merged.Sources = append(merged.Sources, result.Sources...)
		
		// Weight different sources
		weight := 1.0
		switch result.IndexSource {
		case "privacy":
			weight = 0.7
		case "manifest":
			weight = 0.9
		case "content":
			weight = 1.0
		}
		
		merged.Relevance += result.Relevance * weight
		totalWeight += weight
		
		// Preserve highest similarity and privacy level
		if result.Similarity > merged.Similarity {
			merged.Similarity = result.Similarity
		}
		if result.PrivacyLevel > merged.PrivacyLevel {
			merged.PrivacyLevel = result.PrivacyLevel
		}
	}
	
	// Normalize relevance
	if totalWeight > 0 {
		merged.Relevance /= totalWeight
	}
	
	return merged
}

func (se *SearchExecutor) postProcessResults(results []SearchResult, query *SearchQuery) []SearchResult {
	// Apply result limit
	if len(results) > query.MaxResults {
		results = results[:query.MaxResults]
	}
	
	// Apply privacy-specific post-processing
	for i := range results {
		results[i].PrivacyLevel = query.PrivacyLevel
		
		// Apply noise if required
		if query.PrivacyLevel >= 4 {
			results[i].NoiseLevel = se.calculateNoiseLevel(query.PrivacyLevel)
			results[i].Relevance = se.applyRelevanceNoise(results[i].Relevance, results[i].NoiseLevel)
		}
	}
	
	return results
}

// Helper methods

func (se *SearchExecutor) calculateNoiseLevel(privacyLevel int) float64 {
	// Increase noise with privacy level
	baseNoise := 0.01
	return baseNoise * float64(privacyLevel) * 0.02
}

func (se *SearchExecutor) applyRelevanceNoise(relevance, noiseLevel float64) float64 {
	// Add small amount of noise to relevance scores
	noise := (math.Mod(float64(time.Now().UnixNano()), 2.0) - 1.0) * noiseLevel
	noised := relevance + noise
	
	// Keep within valid range
	if noised < 0 {
		noised = 0
	}
	if noised > 1 {
		noised = 1
	}
	
	return noised
}

// Active search management

func (se *SearchExecutor) registerActiveSearch(searchID string, query *SearchQuery, ctx context.Context) {
	searchCtx, cancel := context.WithTimeout(ctx, se.config.QueryTimeout)
	
	se.mu.Lock()
	defer se.mu.Unlock()
	
	se.activeSearches[searchID] = &ActiveSearch{
		SearchID:  searchID,
		Query:     query,
		StartTime: time.Now(),
		Context:   searchCtx,
		Cancel:    cancel,
	}
}

func (se *SearchExecutor) unregisterActiveSearch(searchID string) {
	se.mu.Lock()
	defer se.mu.Unlock()
	
	if search, exists := se.activeSearches[searchID]; exists {
		search.Cancel()
		delete(se.activeSearches, searchID)
	}
}

// Cache management

func (se *SearchExecutor) checkCache(query *SearchQuery) *CachedResult {
	if !se.config.CacheEnabled {
		return nil
	}
	
	queryHash := se.calculateQueryHash(query)
	
	se.resultCache.mu.RLock()
	defer se.resultCache.mu.RUnlock()
	
	if cached, exists := se.resultCache.cache[queryHash]; exists {
		// Check if cache entry is still valid
		if time.Since(cached.Timestamp) < se.resultCache.ttl {
			return cached
		}
	}
	
	return nil
}

func (se *SearchExecutor) cacheResults(query *SearchQuery, results []SearchResult) {
	if !se.config.CacheEnabled {
		return
	}
	
	queryHash := se.calculateQueryHash(query)
	
	se.resultCache.mu.Lock()
	defer se.resultCache.mu.Unlock()
	
	se.resultCache.cache[queryHash] = &CachedResult{
		Results:   results,
		Timestamp: time.Now(),
		QueryHash: queryHash,
	}
}

func (se *SearchExecutor) calculateQueryHash(query *SearchQuery) string {
	// Simple hash based on query content
	return fmt.Sprintf("%s_%d_%d_%v", 
		query.ObfuscatedQuery, 
		query.Type, 
		query.PrivacyLevel,
		query.MaxResults)
}

// Configuration update

func (se *SearchExecutor) UpdateConfig(config *PrivacySearchConfig) {
	se.mu.Lock()
	defer se.mu.Unlock()
	
	se.config = config
	se.resultCache.ttl = config.CacheTTL
}

// Query optimizer methods

func (qo *QueryOptimizer) OptimizeQuery(query *SearchQuery) (*SearchQuery, QueryOptimization) {
	qo.mu.RLock()
	defer qo.mu.RUnlock()
	
	// For now, return the original query with basic optimization
	pattern := fmt.Sprintf("type_%d", int(query.Type))
	optimization := QueryOptimization{
		Pattern:         pattern,
		IndexPreference: []string{"content", "manifest", "privacy"},
		ParallelSafe:    true,
		CacheEnabled:    true,
		EstimatedTime:   time.Millisecond * 100,
	}
	
	return query, optimization
}

func (qo *QueryOptimizer) UpdateMetrics(query *SearchQuery, optimization QueryOptimization, duration time.Duration) {
	qo.mu.Lock()
	defer qo.mu.Unlock()
	
	pattern := optimization.Pattern
	if qo.executionTimes[pattern] == nil {
		qo.executionTimes[pattern] = make([]time.Duration, 0)
	}
	
	qo.executionTimes[pattern] = append(qo.executionTimes[pattern], duration)
	
	// Keep only last 100 measurements
	if len(qo.executionTimes[pattern]) > 100 {
		qo.executionTimes[pattern] = qo.executionTimes[pattern][1:]
	}
}
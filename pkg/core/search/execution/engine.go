// Package execution provides the core search execution engine for privacy-preserving operations.
package execution

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/search/query"
	"github.com/TheEntropyCollective/noisefs/pkg/core/search/types"
)

// Engine implements privacy-preserving search execution
type Engine struct {
	mu              sync.RWMutex
	index           SearchIndex
	privacyManager  *PrivacyManager
	queryProcessor  *query.Processor
	resultFilter    *ResultFilter
	stats          *EngineStats
}

// NewEngine creates a new search engine with privacy protection
func NewEngine(index SearchIndex, settings *types.PrivacySettings) *Engine {
	return &Engine{
		index:          index,
		privacyManager: NewPrivacyManager(settings),
		queryProcessor: query.NewProcessor(settings),
		resultFilter:   NewResultFilter(settings),
		stats:         NewEngineStats(),
	}
}

// Search performs a privacy-preserving search operation
func (e *Engine) Search(ctx context.Context, searchQuery *types.SearchQuery) (*types.SearchResponse, error) {
	startTime := time.Now()
	
	// Update statistics
	e.stats.IncrementQuery(searchQuery.Privacy)

	// Process query with privacy protection
	processed, err := e.queryProcessor.ProcessQuery(ctx, searchQuery)
	if err != nil {
		return nil, fmt.Errorf("query processing failed: %w", err)
	}

	// Execute search with privacy protection
	rawResults, err := e.executePrivacySearch(ctx, processed)
	if err != nil {
		return nil, fmt.Errorf("search execution failed: %w", err)
	}

	// Filter and rank results
	filteredResults := e.resultFilter.FilterResults(rawResults, searchQuery)
	rankedResults := e.rankResults(filteredResults, processed)

	// Apply pagination
	paginatedResults, hasMore := e.applyPagination(rankedResults, searchQuery)

	// Create response
	response := &types.SearchResponse{
		Results:      paginatedResults,
		Total:        len(filteredResults),
		SearchTime:   time.Since(startTime),
		PrivacyLevel: searchQuery.Privacy,
		HasMore:      hasMore,
	}

	// Update performance statistics
	e.stats.UpdateLatency(response.SearchTime)

	return response, nil
}

// Index adds content to the search index
func (e *Engine) Index(ctx context.Context, fileID string, content []byte, metadata map[string]interface{}) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Add to index with privacy considerations
	if err := e.index.AddDocument(ctx, fileID, content, metadata); err != nil {
		return fmt.Errorf("indexing failed: %w", err)
	}

	// Update statistics
	e.stats.IncrementIndexedFiles()

	return nil
}

// Remove removes content from the search index
func (e *Engine) Remove(ctx context.Context, fileID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.index.RemoveDocument(ctx, fileID); err != nil {
		return fmt.Errorf("removal failed: %w", err)
	}

	// Update statistics
	e.stats.DecrementIndexedFiles()

	return nil
}

// UpdatePrivacySettings updates privacy settings for the search engine
func (e *Engine) UpdatePrivacySettings(settings *types.PrivacySettings) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.privacyManager.UpdateSettings(settings)
	e.queryProcessor.UpdatePrivacySettings(settings)
	e.resultFilter.UpdateSettings(settings)

	return nil
}

// GetStats returns search engine statistics
func (e *Engine) GetStats() (*types.SearchStats, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	indexStats, err := e.index.GetStats()
	if err != nil {
		return nil, fmt.Errorf("failed to get index stats: %w", err)
	}

	return &types.SearchStats{
		IndexedFiles:   indexStats.DocumentCount,
		IndexSize:      indexStats.IndexSize,
		TotalQueries:   e.stats.TotalQueries(),
		AverageLatency: e.stats.AverageLatency(),
		PrivacyQueries: e.stats.PrivacyQueries(),
		CacheHitRate:   indexStats.CacheHitRate,
		LastUpdated:    time.Now(),
	}, nil
}

// executePrivacySearch executes search with privacy protection
func (e *Engine) executePrivacySearch(ctx context.Context, processed *query.ProcessedQuery) ([]types.SearchResult, error) {
	// Execute searches for all terms (real and dummy)
	allResults := make([]types.SearchResult, 0)
	
	for _, privacyTerm := range processed.PrivacyTerms {
		// Execute individual term search
		termResults, err := e.index.Search(ctx, privacyTerm.Term, processed.Original.Scope)
		if err != nil {
			return nil, fmt.Errorf("term search failed for %q: %w", privacyTerm.Term, err)
		}

		// Only include results from real terms
		if privacyTerm.IsReal {
			// Apply term weight to results
			for i := range termResults {
				termResults[i].Score *= privacyTerm.Weight
			}
			allResults = append(allResults, termResults...)
		}
	}

	// Apply privacy timing obfuscation
	if e.privacyManager.ShouldObfuscateTiming() {
		e.privacyManager.ApplyTimingObfuscation(ctx)
	}

	return allResults, nil
}

// rankResults ranks search results by relevance
func (e *Engine) rankResults(results []types.SearchResult, processed *query.ProcessedQuery) []types.SearchResult {
	// Combine results by file ID and calculate final scores
	fileResults := make(map[string]*types.SearchResult)
	
	for _, result := range results {
		if existing, exists := fileResults[result.FileID]; exists {
			// Combine scores and matches
			existing.Score += result.Score
			existing.Matches = append(existing.Matches, result.Matches...)
		} else {
			// Create copy to avoid modifying original
			combined := result
			fileResults[result.FileID] = &combined
		}
	}

	// Convert map back to slice
	rankedResults := make([]types.SearchResult, 0, len(fileResults))
	for _, result := range fileResults {
		rankedResults = append(rankedResults, *result)
	}

	// Sort by score (descending)
	sort.Slice(rankedResults, func(i, j int) bool {
		return rankedResults[i].Score > rankedResults[j].Score
	})

	return rankedResults
}

// applyPagination applies pagination to search results
func (e *Engine) applyPagination(results []types.SearchResult, query *types.SearchQuery) ([]types.SearchResult, bool) {
	start := query.Offset
	end := start + query.Limit

	if start >= len(results) {
		return []types.SearchResult{}, false
	}

	if end > len(results) {
		end = len(results)
	}

	hasMore := end < len(results)
	return results[start:end], hasMore
}

// SearchIndex defines the interface for the underlying search index
type SearchIndex interface {
	// Search performs a search for a single term
	Search(ctx context.Context, term string, scope types.SearchScope) ([]types.SearchResult, error)
	
	// AddDocument adds a document to the index
	AddDocument(ctx context.Context, fileID string, content []byte, metadata map[string]interface{}) error
	
	// RemoveDocument removes a document from the index
	RemoveDocument(ctx context.Context, fileID string) error
	
	// GetStats returns index statistics
	GetStats() (*IndexStats, error)
}

// IndexStats provides statistics about the search index
type IndexStats struct {
	DocumentCount int64
	IndexSize     int64
	CacheHitRate  float64
}
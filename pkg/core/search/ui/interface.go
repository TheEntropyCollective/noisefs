// Package ui provides user interface components for privacy-preserving search.
package ui

import (
	"context"

	"github.com/TheEntropyCollective/noisefs/pkg/core/search/types"
)

// SearchInterface defines the interface for search user interactions
type SearchInterface interface {
	// ExecuteSearch performs a search with user-provided query
	ExecuteSearch(ctx context.Context, query *types.SearchQuery) (*types.SearchResponse, error)
	
	// GetSearchHistory returns user's search history (privacy-filtered)
	GetSearchHistory(ctx context.Context, limit int) ([]SearchHistoryEntry, error)
	
	// ClearSearchHistory clears user's search history
	ClearSearchHistory(ctx context.Context) error
	
	// GetPrivacySettings returns current privacy settings
	GetPrivacySettings() *types.PrivacySettings
	
	// UpdatePrivacySettings updates privacy settings
	UpdatePrivacySettings(settings *types.PrivacySettings) error
	
	// GetSearchSuggestions provides search suggestions based on input
	GetSearchSuggestions(ctx context.Context, partial string) ([]string, error)
}

// SearchHistoryEntry represents an entry in the search history
type SearchHistoryEntry struct {
	// Query that was executed
	Query *types.SearchQuery `json:"query"`
	
	// When the search was performed
	Timestamp int64 `json:"timestamp"`
	
	// Number of results returned
	ResultCount int `json:"result_count"`
	
	// Privacy level used
	PrivacyLevel types.PrivacyLevel `json:"privacy_level"`
}

// SearchController implements the search interface with privacy protection
type SearchController struct {
	engine types.SearchEngine
	history *SearchHistory
	suggestions *SuggestionProvider
}

// NewSearchController creates a new search controller
func NewSearchController(engine types.SearchEngine) *SearchController {
	return &SearchController{
		engine: engine,
		history: NewSearchHistory(),
		suggestions: NewSuggestionProvider(),
	}
}

// ExecuteSearch performs a search with user-provided query
func (sc *SearchController) ExecuteSearch(ctx context.Context, query *types.SearchQuery) (*types.SearchResponse, error) {
	// Execute search
	response, err := sc.engine.Search(ctx, query)
	if err != nil {
		return nil, err
	}

	// Add to search history
	sc.history.AddEntry(&SearchHistoryEntry{
		Query:        query,
		Timestamp:    ctx.Value("timestamp").(int64),
		ResultCount:  len(response.Results),
		PrivacyLevel: query.Privacy,
	})

	return response, nil
}

// GetSearchHistory returns user's search history (privacy-filtered)
func (sc *SearchController) GetSearchHistory(ctx context.Context, limit int) ([]SearchHistoryEntry, error) {
	return sc.history.GetEntries(limit), nil
}

// ClearSearchHistory clears user's search history
func (sc *SearchController) ClearSearchHistory(ctx context.Context) error {
	sc.history.Clear()
	return nil
}

// GetPrivacySettings returns current privacy settings
func (sc *SearchController) GetPrivacySettings() *types.PrivacySettings {
	// This would typically get settings from configuration
	return &types.PrivacySettings{
		DefaultPrivacy:     types.PrivacyStandard,
		QueryObfuscation:   true,
		ResultFiltering:    true,
		TimingObfuscation:  true,
		MaxDummyQueries:    4,
		ContextWindow:      200,
	}
}

// UpdatePrivacySettings updates privacy settings
func (sc *SearchController) UpdatePrivacySettings(settings *types.PrivacySettings) error {
	return sc.engine.UpdatePrivacySettings(settings)
}

// GetSearchSuggestions provides search suggestions based on input
func (sc *SearchController) GetSearchSuggestions(ctx context.Context, partial string) ([]string, error) {
	return sc.suggestions.GetSuggestions(partial), nil
}
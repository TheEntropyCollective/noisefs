// Package execution provides result filtering for privacy-preserving search.
package execution

import (
	"github.com/TheEntropyCollective/noisefs/pkg/core/search/privacy"
	"github.com/TheEntropyCollective/noisefs/pkg/core/search/types"
)

// ResultFilter filters and processes search results with privacy protection
type ResultFilter struct {
	privacyManager *privacy.Manager
	settings       *types.PrivacySettings
}

// NewResultFilter creates a new result filter
func NewResultFilter(settings *types.PrivacySettings) *ResultFilter {
	return &ResultFilter{
		privacyManager: privacy.NewManager(settings),
		settings:       settings,
	}
}

// FilterResults applies privacy filtering to search results
func (f *ResultFilter) FilterResults(results []types.SearchResult, query *types.SearchQuery) []types.SearchResult {
	if len(results) == 0 {
		return results
	}

	filtered := make([]types.SearchResult, 0, len(results))

	for _, result := range results {
		// Apply privacy filtering to individual result
		filteredResult := f.filterResult(result, query.Privacy)
		
		// Apply scope-based filtering
		if f.matchesScope(filteredResult, query.Scope) {
			filtered = append(filtered, filteredResult)
		}
	}

	return filtered
}

// filterResult applies privacy filtering to a single search result
func (f *ResultFilter) filterResult(result types.SearchResult, privacyLevel types.PrivacyLevel) types.SearchResult {
	// Create a copy to avoid modifying the original
	filtered := types.SearchResult{
		FileID: result.FileID,
		Score:  f.privacyManager.ObfuscateScore(result.Score, privacyLevel),
	}

	// Filter matches
	filtered.Matches = f.filterMatches(result.Matches, privacyLevel)

	// Filter metadata
	filtered.Metadata = f.privacyManager.FilterMetadata(result.Metadata, privacyLevel)

	return filtered
}

// filterMatches applies privacy filtering to search matches
func (f *ResultFilter) filterMatches(matches []types.Match, privacyLevel types.PrivacyLevel) []types.Match {
	filtered := make([]types.Match, 0, len(matches))

	for _, match := range matches {
		filteredMatch := types.Match{
			Type:       match.Type,
			Context:    f.privacyManager.FilterResultContext(match.Context, privacyLevel),
			Position:   match.Position,
			Confidence: f.privacyManager.ObfuscateScore(match.Confidence, privacyLevel),
		}

		// Filter position information based on privacy level
		if privacyLevel == types.PrivacyMaximum {
			filteredMatch.Position = nil // Remove precise position for maximum privacy
		}

		filtered = append(filtered, filteredMatch)
	}

	return filtered
}

// matchesScope checks if a result matches the search scope
func (f *ResultFilter) matchesScope(result types.SearchResult, scope types.SearchScope) bool {
	// Check file type filters
	if len(scope.FileTypes) > 0 {
		if !f.matchesFileType(result, scope.FileTypes) {
			return false
		}
	}

	// Check size filters
	if !f.matchesSizeFilter(result, scope) {
		return false
	}

	// Check time filters
	if !f.matchesTimeFilter(result, scope) {
		return false
	}

	// Check match type filters
	if !f.matchesContentScope(result, scope) {
		return false
	}

	return true
}

// matchesFileType checks if result matches file type filters
func (f *ResultFilter) matchesFileType(result types.SearchResult, fileTypes []string) bool {
	if result.Metadata == nil {
		return false
	}

	fileType, exists := result.Metadata["file_type"]
	if !exists {
		return false
	}

	fileTypeStr, ok := fileType.(string)
	if !ok {
		return false
	}

	for _, allowedType := range fileTypes {
		if fileTypeStr == allowedType {
			return true
		}
	}

	return false
}

// matchesSizeFilter checks if result matches size filters
func (f *ResultFilter) matchesSizeFilter(result types.SearchResult, scope types.SearchScope) bool {
	if result.Metadata == nil {
		return true // No metadata to filter on
	}

	sizeInterface, exists := result.Metadata["size"]
	if !exists {
		return true // No size information available
	}

	size, ok := sizeInterface.(int64)
	if !ok {
		return true // Invalid size format
	}

	// Check minimum size
	if scope.MinSize > 0 && size < scope.MinSize {
		return false
	}

	// Check maximum size
	if scope.MaxSize > 0 && size > scope.MaxSize {
		return false
	}

	return true
}

// matchesTimeFilter checks if result matches time filters
func (f *ResultFilter) matchesTimeFilter(result types.SearchResult, scope types.SearchScope) bool {
	if result.Metadata == nil {
		return true // No metadata to filter on
	}

	// Check modified_time if available
	modifiedInterface, exists := result.Metadata["modified_time"]
	if !exists {
		return true // No time information available
	}

	// This would need proper time parsing based on the actual metadata format
	// For now, we'll assume the time filtering is handled at the index level
	_ = modifiedInterface // Placeholder to avoid unused variable error

	return true
}

// matchesContentScope checks if result matches content scope filters
func (f *ResultFilter) matchesContentScope(result types.SearchResult, scope types.SearchScope) bool {
	hasContentMatch := false
	hasMetadataMatch := false
	hasFilenameMatch := false

	for _, match := range result.Matches {
		switch match.Type {
		case types.MatchTypeContent:
			hasContentMatch = true
		case types.MatchTypeMetadata:
			hasMetadataMatch = true
		case types.MatchTypeFilename:
			hasFilenameMatch = true
		}
	}

	// Check if any required match types are present
	if scope.Content && !hasContentMatch {
		return false
	}
	if scope.Metadata && !hasMetadataMatch {
		return false
	}
	if scope.Filenames && !hasFilenameMatch {
		return false
	}

	// If no specific scope is set, accept any match type
	if !scope.Content && !scope.Metadata && !scope.Filenames {
		return len(result.Matches) > 0
	}

	return true
}

// UpdateSettings updates the filter settings
func (f *ResultFilter) UpdateSettings(settings *types.PrivacySettings) {
	f.settings = settings
	f.privacyManager.UpdateSettings(settings)
}
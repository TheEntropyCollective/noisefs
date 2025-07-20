// Package search provides privacy-preserving search capabilities for NoiseFS.
//
// This package implements a comprehensive search infrastructure that maintains
// the privacy guarantees of the OFFSystem while providing efficient search
// functionality across anonymized content.
//
// Key components:
//
//   - types: Core data structures and interfaces
//   - query: Query processing and privacy protection
//   - execution: Search execution engine with privacy-aware ranking
//   - privacy: Privacy management and obfuscation
//   - ui: User interface and interaction components
//
// Privacy Features:
//
//   - Query obfuscation with dummy queries
//   - Result filtering and context limitation
//   - Timing obfuscation to prevent side-channel attacks
//   - Configurable privacy levels (Minimal, Standard, Maximum)
//   - Metadata filtering based on privacy settings
//
// Usage Example:
//
//	// Create search engine with privacy settings
//	settings := &types.PrivacySettings{
//		DefaultPrivacy:    types.PrivacyStandard,
//		QueryObfuscation:  true,
//		ResultFiltering:   true,
//		TimingObfuscation: true,
//		MaxDummyQueries:   4,
//		ContextWindow:     200,
//	}
//
//	engine := execution.NewEngine(searchIndex, settings)
//
//	// Perform a privacy-preserving search
//	query := &types.SearchQuery{
//		Terms:   []string{"document", "report"},
//		Privacy: types.PrivacyStandard,
//		Scope: types.SearchScope{
//			Content:  true,
//			Metadata: false,
//		},
//		Limit: 10,
//	}
//
//	response, err := engine.Search(ctx, query)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	for _, result := range response.Results {
//		fmt.Printf("File: %s, Score: %.2f\n", result.FileID, result.Score)
//	}
package search

import (
	"github.com/TheEntropyCollective/noisefs/pkg/core/search/execution"
	"github.com/TheEntropyCollective/noisefs/pkg/core/search/types"
	"github.com/TheEntropyCollective/noisefs/pkg/core/search/ui"
)

// DefaultPrivacySettings returns the default privacy settings for search operations
func DefaultPrivacySettings() *types.PrivacySettings {
	return &types.PrivacySettings{
		DefaultPrivacy:    types.PrivacyStandard,
		QueryObfuscation:  true,
		ResultFiltering:   true,
		TimingObfuscation: true,
		MaxDummyQueries:   4,
		ContextWindow:     200,
	}
}

// NewSearchEngine creates a new privacy-preserving search engine
func NewSearchEngine(index execution.SearchIndex, settings *types.PrivacySettings) types.SearchEngine {
	if settings == nil {
		settings = DefaultPrivacySettings()
	}
	return execution.NewEngine(index, settings)
}

// NewSearchController creates a new search controller with UI components
func NewSearchController(engine types.SearchEngine) ui.SearchInterface {
	return ui.NewSearchController(engine)
}

// Version returns the current version of the search package
const Version = "0.1.0"
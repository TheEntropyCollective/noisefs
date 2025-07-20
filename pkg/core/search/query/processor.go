// Package query provides privacy-preserving query processing capabilities.
package query

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/search/types"
)

// Processor handles query preprocessing and privacy protection
type Processor struct {
	privacySettings *types.PrivacySettings
	obfuscator      *QueryObfuscator
}

// NewProcessor creates a new query processor with specified privacy settings
func NewProcessor(settings *types.PrivacySettings) *Processor {
	return &Processor{
		privacySettings: settings,
		obfuscator:      NewQueryObfuscator(settings),
	}
}

// ProcessQuery preprocesses a search query for privacy-preserving execution
func (p *Processor) ProcessQuery(ctx context.Context, query *types.SearchQuery) (*ProcessedQuery, error) {
	// Validate query
	if err := p.validateQuery(query); err != nil {
		return nil, fmt.Errorf("query validation failed: %w", err)
	}

	// Normalize query terms
	normalizedTerms := p.normalizeTerms(query.Terms)

	// Apply privacy protection
	privacyTerms, err := p.applyPrivacyProtection(ctx, normalizedTerms, query.Privacy)
	if err != nil {
		return nil, fmt.Errorf("privacy protection failed: %w", err)
	}

	// Create processed query
	processed := &ProcessedQuery{
		Original:     query,
		Terms:        normalizedTerms,
		PrivacyTerms: privacyTerms,
		ProcessedAt:  time.Now(),
	}

	return processed, nil
}

// ProcessedQuery represents a query that has been processed for privacy-preserving search
type ProcessedQuery struct {
	// Original query
	Original *types.SearchQuery

	// Normalized search terms
	Terms []string

	// Privacy-protected terms (may include dummy queries)
	PrivacyTerms []PrivacyTerm

	// Processing timestamp
	ProcessedAt time.Time
}

// PrivacyTerm represents a search term with privacy protection information
type PrivacyTerm struct {
	// The term to search for
	Term string

	// Whether this is a real term or a dummy term
	IsReal bool

	// Weight for relevance scoring
	Weight float64

	// Privacy level applied to this term
	PrivacyLevel types.PrivacyLevel
}

// validateQuery validates the structure and content of a search query
func (p *Processor) validateQuery(query *types.SearchQuery) error {
	if query == nil {
		return fmt.Errorf("query cannot be nil")
	}

	if len(query.Terms) == 0 {
		return fmt.Errorf("query must contain at least one search term")
	}

	// Validate term length and content
	for i, term := range query.Terms {
		if strings.TrimSpace(term) == "" {
			return fmt.Errorf("term %d cannot be empty", i)
		}
		if len(term) > 1000 {
			return fmt.Errorf("term %d exceeds maximum length", i)
		}
	}

	// Validate limits
	if query.Limit < 0 {
		return fmt.Errorf("limit cannot be negative")
	}
	if query.Offset < 0 {
		return fmt.Errorf("offset cannot be negative")
	}

	return nil
}

// normalizeTerms normalizes search terms for consistent processing
func (p *Processor) normalizeTerms(terms []string) []string {
	normalized := make([]string, 0, len(terms))
	
	for _, term := range terms {
		// Trim whitespace
		term = strings.TrimSpace(term)
		
		// Convert to lowercase
		term = strings.ToLower(term)
		
		// Skip empty terms
		if term == "" {
			continue
		}
		
		normalized = append(normalized, term)
	}
	
	return normalized
}

// applyPrivacyProtection applies privacy protection to search terms
func (p *Processor) applyPrivacyProtection(ctx context.Context, terms []string, level types.PrivacyLevel) ([]PrivacyTerm, error) {
	privacyTerms := make([]PrivacyTerm, 0, len(terms))

	// Add real terms
	for _, term := range terms {
		privacyTerms = append(privacyTerms, PrivacyTerm{
			Term:         term,
			IsReal:       true,
			Weight:       1.0,
			PrivacyLevel: level,
		})
	}

	// Add dummy terms based on privacy level
	if p.privacySettings.QueryObfuscation {
		dummyTerms, err := p.obfuscator.GenerateDummyTerms(ctx, terms, level)
		if err != nil {
			return nil, fmt.Errorf("failed to generate dummy terms: %w", err)
		}

		for _, dummyTerm := range dummyTerms {
			privacyTerms = append(privacyTerms, PrivacyTerm{
				Term:         dummyTerm,
				IsReal:       false,
				Weight:       0.0,
				PrivacyLevel: level,
			})
		}
	}

	return privacyTerms, nil
}

// UpdatePrivacySettings updates the privacy settings for the processor
func (p *Processor) UpdatePrivacySettings(settings *types.PrivacySettings) {
	p.privacySettings = settings
	p.obfuscator.UpdateSettings(settings)
}
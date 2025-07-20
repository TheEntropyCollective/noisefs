// Package query provides query obfuscation for privacy protection.
package query

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/search/types"
)

// QueryObfuscator generates dummy queries to protect user privacy
type QueryObfuscator struct {
	settings    *types.PrivacySettings
	dummyTerms  []string
	rng         *rand.Rand
}

// NewQueryObfuscator creates a new query obfuscator
func NewQueryObfuscator(settings *types.PrivacySettings) *QueryObfuscator {
	return &QueryObfuscator{
		settings:   settings,
		dummyTerms: getCommonTerms(),
		rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// GenerateDummyTerms generates dummy search terms based on privacy level
func (o *QueryObfuscator) GenerateDummyTerms(ctx context.Context, realTerms []string, level types.PrivacyLevel) ([]string, error) {
	var numDummy int
	
	switch level {
	case types.PrivacyMinimal:
		numDummy = len(realTerms) // 1:1 ratio
	case types.PrivacyStandard:
		numDummy = len(realTerms) * 2 // 2:1 ratio
	case types.PrivacyMaximum:
		numDummy = len(realTerms) * 4 // 4:1 ratio
	default:
		return nil, fmt.Errorf("unknown privacy level: %v", level)
	}

	// Respect maximum dummy queries setting
	if o.settings.MaxDummyQueries > 0 && numDummy > o.settings.MaxDummyQueries {
		numDummy = o.settings.MaxDummyQueries
	}

	dummyTerms := make([]string, 0, numDummy)
	usedTerms := make(map[string]bool)

	// Mark real terms as used to avoid duplicates
	for _, term := range realTerms {
		usedTerms[strings.ToLower(term)] = true
	}

	// Generate dummy terms
	for len(dummyTerms) < numDummy {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		dummyTerm := o.generateDummyTerm(realTerms, level)
		
		// Avoid duplicates
		if !usedTerms[strings.ToLower(dummyTerm)] {
			dummyTerms = append(dummyTerms, dummyTerm)
			usedTerms[strings.ToLower(dummyTerm)] = true
		}
	}

	return dummyTerms, nil
}

// generateDummyTerm generates a single dummy term
func (o *QueryObfuscator) generateDummyTerm(realTerms []string, level types.PrivacyLevel) string {
	switch level {
	case types.PrivacyMinimal:
		// Use random common terms
		return o.getRandomCommonTerm()
		
	case types.PrivacyStandard:
		// Mix of common terms and variations of real terms
		if o.rng.Float32() < 0.5 {
			return o.getRandomCommonTerm()
		}
		return o.generateVariation(realTerms)
		
	case types.PrivacyMaximum:
		// More sophisticated obfuscation
		if o.rng.Float32() < 0.3 {
			return o.getRandomCommonTerm()
		} else if o.rng.Float32() < 0.6 {
			return o.generateVariation(realTerms)
		}
		return o.generateSynonym(realTerms)
		
	default:
		return o.getRandomCommonTerm()
	}
}

// getRandomCommonTerm returns a random common search term
func (o *QueryObfuscator) getRandomCommonTerm() string {
	if len(o.dummyTerms) == 0 {
		return "document"
	}
	return o.dummyTerms[o.rng.Intn(len(o.dummyTerms))]
}

// generateVariation generates a variation of real terms
func (o *QueryObfuscator) generateVariation(realTerms []string) string {
	if len(realTerms) == 0 {
		return o.getRandomCommonTerm()
	}
	
	baseTerm := realTerms[o.rng.Intn(len(realTerms))]
	
	// Apply simple variations
	variations := []string{
		baseTerm + "s",           // plural
		baseTerm + "ing",         // gerund
		baseTerm + "ed",          // past tense
		"un" + baseTerm,          // prefix
		baseTerm + "tion",        // suffix
	}
	
	return variations[o.rng.Intn(len(variations))]
}

// generateSynonym generates a synonym-like term
func (o *QueryObfuscator) generateSynonym(realTerms []string) string {
	// Simple synonym mapping for common terms
	synonyms := map[string][]string{
		"file":     {"document", "item", "record"},
		"document": {"file", "paper", "text"},
		"image":    {"picture", "photo", "graphic"},
		"text":     {"content", "document", "writing"},
		"data":     {"information", "content", "details"},
	}
	
	for _, term := range realTerms {
		if syns, exists := synonyms[strings.ToLower(term)]; exists {
			return syns[o.rng.Intn(len(syns))]
		}
	}
	
	return o.getRandomCommonTerm()
}

// UpdateSettings updates the obfuscator settings
func (o *QueryObfuscator) UpdateSettings(settings *types.PrivacySettings) {
	o.settings = settings
}

// getCommonTerms returns a list of common search terms for dummy queries
func getCommonTerms() []string {
	return []string{
		"document", "file", "text", "image", "photo", "video", "audio",
		"data", "information", "content", "report", "presentation",
		"spreadsheet", "database", "archive", "backup", "folder",
		"directory", "project", "work", "personal", "business",
		"meeting", "notes", "draft", "final", "version", "copy",
		"original", "temporary", "old", "new", "recent", "updated",
		"important", "urgent", "confidential", "public", "private",
		"shared", "local", "remote", "cloud", "storage", "sync",
		"email", "message", "attachment", "download", "upload",
		"configuration", "settings", "preferences", "options",
		"tutorial", "guide", "manual", "help", "documentation",
	}
}
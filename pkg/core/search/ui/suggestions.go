// Package ui provides search suggestions with privacy protection.
package ui

import (
	"strings"
	"sync"
)

// SuggestionProvider generates search suggestions while protecting privacy
type SuggestionProvider struct {
	mu          sync.RWMutex
	suggestions map[string][]string
	commonTerms []string
}

// NewSuggestionProvider creates a new suggestion provider
func NewSuggestionProvider() *SuggestionProvider {
	return &SuggestionProvider{
		suggestions: make(map[string][]string),
		commonTerms: getCommonSearchTerms(),
	}
}

// GetSuggestions provides search suggestions based on partial input
func (sp *SuggestionProvider) GetSuggestions(partial string) []string {
	sp.mu.RLock()
	defer sp.mu.RUnlock()

	if len(partial) < 2 {
		// Return common terms for short inputs
		return sp.getCommonSuggestions(5)
	}

	partial = strings.ToLower(strings.TrimSpace(partial))
	
	// Look for cached suggestions
	if cached, exists := sp.suggestions[partial]; exists {
		return cached
	}

	// Generate suggestions
	suggestions := sp.generateSuggestions(partial)
	
	// Cache the suggestions
	sp.suggestions[partial] = suggestions
	
	return suggestions
}

// generateSuggestions generates suggestions for a partial term
func (sp *SuggestionProvider) generateSuggestions(partial string) []string {
	suggestions := make([]string, 0, 10)
	
	// Find matching common terms
	for _, term := range sp.commonTerms {
		if strings.HasPrefix(strings.ToLower(term), partial) {
			suggestions = append(suggestions, term)
			if len(suggestions) >= 10 {
				break
			}
		}
	}
	
	// If we don't have enough suggestions, add related terms
	if len(suggestions) < 5 {
		related := sp.getRelatedTerms(partial)
		for _, term := range related {
			if len(suggestions) >= 10 {
				break
			}
			// Avoid duplicates
			if !sp.containsTerm(suggestions, term) {
				suggestions = append(suggestions, term)
			}
		}
	}
	
	return suggestions
}

// getRelatedTerms finds terms related to the partial input
func (sp *SuggestionProvider) getRelatedTerms(partial string) []string {
	// Simple related terms mapping for privacy-friendly suggestions
	relatedMap := map[string][]string{
		"doc":  {"document", "documentation", "docs"},
		"file": {"filename", "files", "filepath"},
		"img":  {"image", "images", "picture"},
		"pic":  {"picture", "pictures", "photo"},
		"vid":  {"video", "videos", "movie"},
		"aud":  {"audio", "sound", "music"},
		"txt":  {"text", "textual", "content"},
		"pdf":  {"document", "report", "manual"},
		"conf": {"configuration", "config", "settings"},
		"log":  {"logs", "logging", "debug"},
	}
	
	for prefix, related := range relatedMap {
		if strings.HasPrefix(partial, prefix) {
			return related
		}
	}
	
	return []string{}
}

// getCommonSuggestions returns common search suggestions
func (sp *SuggestionProvider) getCommonSuggestions(limit int) []string {
	if limit > len(sp.commonTerms) {
		limit = len(sp.commonTerms)
	}
	
	result := make([]string, limit)
	copy(result, sp.commonTerms[:limit])
	
	return result
}

// containsTerm checks if a term is already in the suggestions slice
func (sp *SuggestionProvider) containsTerm(suggestions []string, term string) bool {
	for _, suggestion := range suggestions {
		if strings.EqualFold(suggestion, term) {
			return true
		}
	}
	return false
}

// AddCustomSuggestion adds a custom suggestion (privacy-filtered)
func (sp *SuggestionProvider) AddCustomSuggestion(term string) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	
	// Only add if it's a reasonable term (privacy protection)
	if sp.isReasonableTerm(term) {
		sp.commonTerms = append(sp.commonTerms, term)
	}
}

// isReasonableTerm checks if a term is reasonable for suggestions
func (sp *SuggestionProvider) isReasonableTerm(term string) bool {
	// Basic privacy filtering for suggestions
	term = strings.TrimSpace(term)
	
	// Check length
	if len(term) < 2 || len(term) > 50 {
		return false
	}
	
	// Check for potentially sensitive patterns
	sensitivePatterns := []string{
		"password", "secret", "key", "token", "private",
		"confidential", "personal", "ssn", "credit",
	}
	
	lowerTerm := strings.ToLower(term)
	for _, pattern := range sensitivePatterns {
		if strings.Contains(lowerTerm, pattern) {
			return false
		}
	}
	
	return true
}

// getCommonSearchTerms returns a list of common, privacy-safe search terms
func getCommonSearchTerms() []string {
	return []string{
		"document", "file", "image", "photo", "video", "audio",
		"text", "content", "data", "information", "report",
		"presentation", "spreadsheet", "pdf", "word", "excel",
		"powerpoint", "notes", "meeting", "project", "work",
		"personal", "business", "draft", "final", "version",
		"backup", "archive", "folder", "directory", "recent",
		"old", "new", "updated", "modified", "created",
		"important", "urgent", "todo", "task", "calendar",
		"email", "message", "attachment", "download", "upload",
		"configuration", "settings", "preferences", "help",
		"manual", "guide", "tutorial", "documentation",
	}
}
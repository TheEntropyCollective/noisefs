// Package ui provides search history management with privacy protection.
package ui

import (
	"sync"
	"time"
)

// SearchHistory manages user search history with privacy considerations
type SearchHistory struct {
	mu      sync.RWMutex
	entries []SearchHistoryEntry
	maxSize int
}

// NewSearchHistory creates a new search history manager
func NewSearchHistory() *SearchHistory {
	return &SearchHistory{
		entries: make([]SearchHistoryEntry, 0),
		maxSize: 100, // Limit history size for privacy
	}
}

// AddEntry adds a new search history entry
func (sh *SearchHistory) AddEntry(entry *SearchHistoryEntry) {
	sh.mu.Lock()
	defer sh.mu.Unlock()

	// Set timestamp if not provided
	if entry.Timestamp == 0 {
		entry.Timestamp = time.Now().Unix()
	}

	// Add entry to the beginning of the slice
	sh.entries = append([]SearchHistoryEntry{*entry}, sh.entries...)

	// Trim to max size
	if len(sh.entries) > sh.maxSize {
		sh.entries = sh.entries[:sh.maxSize]
	}
}

// GetEntries returns recent search history entries
func (sh *SearchHistory) GetEntries(limit int) []SearchHistoryEntry {
	sh.mu.RLock()
	defer sh.mu.RUnlock()

	if limit <= 0 || limit > len(sh.entries) {
		limit = len(sh.entries)
	}

	// Return a copy to prevent external modification
	result := make([]SearchHistoryEntry, limit)
	copy(result, sh.entries[:limit])

	return result
}

// Clear removes all search history entries
func (sh *SearchHistory) Clear() {
	sh.mu.Lock()
	defer sh.mu.Unlock()

	sh.entries = sh.entries[:0]
}

// GetSize returns the current number of history entries
func (sh *SearchHistory) GetSize() int {
	sh.mu.RLock()
	defer sh.mu.RUnlock()

	return len(sh.entries)
}

// RemoveOlderThan removes entries older than the specified duration
func (sh *SearchHistory) RemoveOlderThan(duration time.Duration) {
	sh.mu.Lock()
	defer sh.mu.Unlock()

	cutoff := time.Now().Add(-duration).Unix()
	
	// Find the first entry that should be kept
	keepIndex := len(sh.entries)
	for i, entry := range sh.entries {
		if entry.Timestamp < cutoff {
			keepIndex = i
			break
		}
	}

	// Keep only recent entries
	if keepIndex < len(sh.entries) {
		sh.entries = sh.entries[:keepIndex]
	}
}
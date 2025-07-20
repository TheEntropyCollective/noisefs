// Package execution provides statistics tracking for search operations.
package execution

import (
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/search/types"
)

// EngineStats tracks statistics for search engine operations
type EngineStats struct {
	mu             sync.RWMutex
	totalQueries   int64
	indexedFiles   int64
	privacyQueries map[types.PrivacyLevel]int64
	latencies      []time.Duration
	maxLatencies   int
}

// NewEngineStats creates a new statistics tracker
func NewEngineStats() *EngineStats {
	return &EngineStats{
		privacyQueries: make(map[types.PrivacyLevel]int64),
		latencies:      make([]time.Duration, 0, 1000),
		maxLatencies:   1000, // Keep last 1000 latencies for average calculation
	}
}

// IncrementQuery increments the query counter for the specified privacy level
func (s *EngineStats) IncrementQuery(level types.PrivacyLevel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.totalQueries++
	s.privacyQueries[level]++
}

// IncrementIndexedFiles increments the indexed files counter
func (s *EngineStats) IncrementIndexedFiles() {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.indexedFiles++
}

// DecrementIndexedFiles decrements the indexed files counter
func (s *EngineStats) DecrementIndexedFiles() {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.indexedFiles > 0 {
		s.indexedFiles--
	}
}

// UpdateLatency adds a new latency measurement
func (s *EngineStats) UpdateLatency(latency time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.latencies = append(s.latencies, latency)
	
	// Keep only the most recent latencies to prevent unbounded growth
	if len(s.latencies) > s.maxLatencies {
		s.latencies = s.latencies[len(s.latencies)-s.maxLatencies:]
	}
}

// TotalQueries returns the total number of queries processed
func (s *EngineStats) TotalQueries() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	return s.totalQueries
}

// IndexedFiles returns the number of indexed files
func (s *EngineStats) IndexedFiles() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	return s.indexedFiles
}

// PrivacyQueries returns a copy of the privacy level query counts
func (s *EngineStats) PrivacyQueries() map[types.PrivacyLevel]int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	// Return a copy to prevent external modification
	result := make(map[types.PrivacyLevel]int64)
	for level, count := range s.privacyQueries {
		result[level] = count
	}
	
	return result
}

// AverageLatency calculates the average latency from recent measurements
func (s *EngineStats) AverageLatency() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	if len(s.latencies) == 0 {
		return 0
	}
	
	var total time.Duration
	for _, latency := range s.latencies {
		total += latency
	}
	
	return total / time.Duration(len(s.latencies))
}

// Reset resets all statistics
func (s *EngineStats) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.totalQueries = 0
	s.indexedFiles = 0
	s.privacyQueries = make(map[types.PrivacyLevel]int64)
	s.latencies = s.latencies[:0]
}
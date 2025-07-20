// Package privacy provides privacy protection mechanisms for search operations.
package privacy

import (
	"context"
	"math/rand"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/search/types"
)

// Manager handles privacy protection for search operations
type Manager struct {
	settings *types.PrivacySettings
	rng      *rand.Rand
}

// NewManager creates a new privacy manager
func NewManager(settings *types.PrivacySettings) *Manager {
	return &Manager{
		settings: settings,
		rng:      rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// ShouldObfuscateTiming determines if timing obfuscation should be applied
func (m *Manager) ShouldObfuscateTiming() bool {
	return m.settings.TimingObfuscation
}

// ApplyTimingObfuscation adds random delay to obfuscate search timing
func (m *Manager) ApplyTimingObfuscation(ctx context.Context) {
	if !m.settings.TimingObfuscation {
		return
	}

	// Generate random delay between 10ms and 500ms
	delay := time.Duration(m.rng.Intn(490)+10) * time.Millisecond

	select {
	case <-time.After(delay):
		// Timing obfuscation complete
	case <-ctx.Done():
		// Context cancelled, skip timing obfuscation
	}
}

// FilterResultContext filters result context based on privacy settings
func (m *Manager) FilterResultContext(context string, privacyLevel types.PrivacyLevel) string {
	if !m.settings.ResultFiltering {
		return context
	}

	// Calculate context window size based on privacy level
	windowSize := m.getContextWindowSize(privacyLevel)
	
	if len(context) <= windowSize {
		return context
	}

	// Truncate context to window size
	if windowSize > 0 {
		return context[:windowSize] + "..."
	}

	return "[Content filtered for privacy]"
}

// FilterMetadata filters metadata based on privacy settings
func (m *Manager) FilterMetadata(metadata map[string]interface{}, privacyLevel types.PrivacyLevel) map[string]interface{} {
	if !m.settings.ResultFiltering {
		return metadata
	}

	filtered := make(map[string]interface{})
	allowedFields := m.getAllowedMetadataFields(privacyLevel)

	for key, value := range metadata {
		if m.isFieldAllowed(key, allowedFields) {
			filtered[key] = value
		}
	}

	return filtered
}

// ObfuscateScore applies privacy-aware score obfuscation
func (m *Manager) ObfuscateScore(score float64, privacyLevel types.PrivacyLevel) float64 {
	switch privacyLevel {
	case types.PrivacyMinimal:
		// No score obfuscation
		return score
		
	case types.PrivacyStandard:
		// Add small random noise
		noise := (m.rng.Float64() - 0.5) * 0.1 // ±5% noise
		return score + noise
		
	case types.PrivacyMaximum:
		// Add larger random noise and round
		noise := (m.rng.Float64() - 0.5) * 0.2 // ±10% noise
		obfuscated := score + noise
		// Round to reduce precision
		return float64(int(obfuscated*10)) / 10
		
	default:
		return score
	}
}

// UpdateSettings updates privacy settings
func (m *Manager) UpdateSettings(settings *types.PrivacySettings) {
	m.settings = settings
}

// getContextWindowSize returns the context window size for the privacy level
func (m *Manager) getContextWindowSize(level types.PrivacyLevel) int {
	baseWindow := m.settings.ContextWindow
	if baseWindow == 0 {
		baseWindow = 200 // Default context window
	}

	switch level {
	case types.PrivacyMinimal:
		return baseWindow
	case types.PrivacyStandard:
		return baseWindow / 2
	case types.PrivacyMaximum:
		return baseWindow / 4
	default:
		return baseWindow
	}
}

// getAllowedMetadataFields returns allowed metadata fields for privacy level
func (m *Manager) getAllowedMetadataFields(level types.PrivacyLevel) []string {
	switch level {
	case types.PrivacyMinimal:
		return []string{
			"filename", "file_type", "size", "modified_time",
			"created_time", "permissions", "mime_type",
		}
	case types.PrivacyStandard:
		return []string{
			"file_type", "size", "mime_type",
		}
	case types.PrivacyMaximum:
		return []string{
			"file_type",
		}
	default:
		return []string{}
	}
}

// isFieldAllowed checks if a metadata field is allowed
func (m *Manager) isFieldAllowed(field string, allowedFields []string) bool {
	for _, allowed := range allowedFields {
		if field == allowed {
			return true
		}
	}
	return false
}

// GeneratePrivacyReport generates a privacy report for search operations
func (m *Manager) GeneratePrivacyReport(query *types.SearchQuery, results *types.SearchResponse) *PrivacyReport {
	return &PrivacyReport{
		QueryPrivacyLevel:    query.Privacy,
		ResultPrivacyLevel:   results.PrivacyLevel,
		TimingObfuscated:     m.settings.TimingObfuscation,
		QueryObfuscated:      m.settings.QueryObfuscation,
		ResultsFiltered:      m.settings.ResultFiltering,
		ContextWindowSize:    m.getContextWindowSize(query.Privacy),
		MetadataFieldsCount:  len(m.getAllowedMetadataFields(query.Privacy)),
		GeneratedAt:         time.Now(),
	}
}

// PrivacyReport provides information about privacy protections applied
type PrivacyReport struct {
	QueryPrivacyLevel    types.PrivacyLevel `json:"query_privacy_level"`
	ResultPrivacyLevel   types.PrivacyLevel `json:"result_privacy_level"`
	TimingObfuscated     bool               `json:"timing_obfuscated"`
	QueryObfuscated      bool               `json:"query_obfuscated"`
	ResultsFiltered      bool               `json:"results_filtered"`
	ContextWindowSize    int                `json:"context_window_size"`
	MetadataFieldsCount  int                `json:"metadata_fields_count"`
	GeneratedAt         time.Time          `json:"generated_at"`
}
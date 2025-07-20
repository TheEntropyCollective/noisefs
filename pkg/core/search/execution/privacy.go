// Package execution provides privacy management for search execution.
package execution

import (
	"context"
	"math/rand"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/search/types"
)

// PrivacyManager handles privacy protection during search execution
type PrivacyManager struct {
	settings *types.PrivacySettings
	rng      *rand.Rand
}

// NewPrivacyManager creates a new privacy manager for search execution
func NewPrivacyManager(settings *types.PrivacySettings) *PrivacyManager {
	return &PrivacyManager{
		settings: settings,
		rng:      rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// ShouldObfuscateTiming determines if timing obfuscation should be applied
func (pm *PrivacyManager) ShouldObfuscateTiming() bool {
	return pm.settings.TimingObfuscation
}

// ApplyTimingObfuscation adds random delay to obfuscate search timing
func (pm *PrivacyManager) ApplyTimingObfuscation(ctx context.Context) {
	if !pm.settings.TimingObfuscation {
		return
	}

	// Generate random delay between 50ms and 200ms for search operations
	delay := time.Duration(pm.rng.Intn(150)+50) * time.Millisecond

	select {
	case <-time.After(delay):
		// Timing obfuscation complete
	case <-ctx.Done():
		// Context cancelled, skip timing obfuscation
	}
}

// UpdateSettings updates the privacy settings
func (pm *PrivacyManager) UpdateSettings(settings *types.PrivacySettings) {
	pm.settings = settings
}
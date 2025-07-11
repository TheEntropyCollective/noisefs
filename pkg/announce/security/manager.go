package security

import (
	"fmt"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/announce"
)

// Manager coordinates security features for announcements
type Manager struct {
	validator    *announce.Validator
	rateLimiter  *announce.RateLimiter
	spamDetector *announce.SpamDetector
	reputation   *announce.ReputationSystem
	
	// Configuration
	spamThreshold int
	trustRequired bool
	
	// Metrics
	metrics *SecurityMetrics
	mu      sync.RWMutex
}

// SecurityMetrics tracks security-related metrics
type SecurityMetrics struct {
	TotalChecked       int64
	ValidationFailures int64
	RateLimitHits      int64
	SpamDetected       int64
	ReputationRejects  int64
	Allowed            int64
}

// Config holds security manager configuration
type Config struct {
	ValidationConfig  *announce.ValidationConfig
	RateLimitConfig   *announce.RateLimitConfig
	SpamConfig        *announce.SpamConfig
	ReputationConfig  *announce.ReputationConfig
	SpamThreshold     int  // Spam score threshold (0-100)
	TrustRequired     bool // Require trusted reputation
}

// DefaultConfig returns default security configuration
func DefaultConfig() *Config {
	return &Config{
		ValidationConfig:  announce.DefaultValidationConfig(),
		RateLimitConfig:   announce.DefaultRateLimitConfig(),
		SpamConfig:        announce.DefaultSpamConfig(),
		ReputationConfig:  announce.DefaultReputationConfig(),
		SpamThreshold:     70,   // Block if spam score > 70
		TrustRequired:     false, // Don't require trust by default
	}
}

// NewManager creates a new security manager
func NewManager(config *Config) *Manager {
	if config == nil {
		config = DefaultConfig()
	}
	
	return &Manager{
		validator:     announce.NewValidator(config.ValidationConfig),
		rateLimiter:   announce.NewRateLimiter(config.RateLimitConfig),
		spamDetector:  announce.NewSpamDetector(config.SpamConfig),
		reputation:    announce.NewReputationSystem(config.ReputationConfig),
		spamThreshold: config.SpamThreshold,
		trustRequired: config.TrustRequired,
		metrics:       &SecurityMetrics{},
	}
}

// CheckAnnouncement performs all security checks on an announcement
func (m *Manager) CheckAnnouncement(ann *announce.Announcement, sourceID string) error {
	m.mu.Lock()
	m.metrics.TotalChecked++
	m.mu.Unlock()
	
	// 1. Validate structure and content
	if err := m.validator.ValidateAnnouncement(ann); err != nil {
		m.incrementMetric(&m.metrics.ValidationFailures)
		return fmt.Errorf("validation failed: %w", err)
	}
	
	// 2. Check rate limits
	rateLimitKey := announce.RateLimitKey("announce", sourceID)
	if err := m.rateLimiter.CheckLimit(rateLimitKey); err != nil {
		m.incrementMetric(&m.metrics.RateLimitHits)
		// Record negative reputation event
		m.reputation.RecordNegative(sourceID, "rate_limit_exceeded")
		return fmt.Errorf("rate limit exceeded: %w", err)
	}
	
	// 3. Check for spam
	isSpam, spamReason := m.spamDetector.CheckSpam(ann)
	if isSpam {
		m.incrementMetric(&m.metrics.SpamDetected)
		// Record negative reputation event
		m.reputation.RecordNegative(sourceID, "spam_detected:"+spamReason)
		return fmt.Errorf("spam detected: %s", spamReason)
	}
	
	// 4. Check spam score
	spamScore := m.spamDetector.SpamScore(ann)
	if spamScore > m.spamThreshold {
		m.incrementMetric(&m.metrics.SpamDetected)
		// Record negative reputation event
		m.reputation.RecordNegative(sourceID, fmt.Sprintf("high_spam_score:%d", spamScore))
		return fmt.Errorf("spam score too high: %d > %d", spamScore, m.spamThreshold)
	}
	
	// 5. Check reputation
	if m.trustRequired && !m.reputation.IsTrusted(sourceID) {
		trustLevel := m.reputation.GetTrustLevel(sourceID)
		if trustLevel == "untrusted" || trustLevel == "suspicious" {
			m.incrementMetric(&m.metrics.ReputationRejects)
			return fmt.Errorf("untrusted source: %s", trustLevel)
		}
	}
	
	// 6. Check if blacklisted
	if m.reputation.IsBlacklisted(sourceID) {
		m.incrementMetric(&m.metrics.ReputationRejects)
		return fmt.Errorf("source is blacklisted")
	}
	
	// All checks passed - record positive event
	m.reputation.RecordPositive(sourceID, "valid_announcement")
	m.incrementMetric(&m.metrics.Allowed)
	
	return nil
}

// GetSourceInfo returns security information about a source
func (m *Manager) GetSourceInfo(sourceID string) SourceInfo {
	info := SourceInfo{
		SourceID: sourceID,
	}
	
	// Get rate limit status
	rateLimitKey := announce.RateLimitKey("announce", sourceID)
	info.RateLimit = m.rateLimiter.GetStatus(rateLimitKey)
	
	// Get reputation
	if rep, exists := m.reputation.GetReputation(sourceID); exists {
		info.Reputation = rep
		info.TrustLevel = m.reputation.GetTrustLevel(sourceID)
	} else {
		info.TrustLevel = "unknown"
	}
	
	return info
}

// GetMetrics returns security metrics
func (m *Manager) GetMetrics() SecurityMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	// Return copy
	return SecurityMetrics{
		TotalChecked:       m.metrics.TotalChecked,
		ValidationFailures: m.metrics.ValidationFailures,
		RateLimitHits:      m.metrics.RateLimitHits,
		SpamDetected:       m.metrics.SpamDetected,
		ReputationRejects:  m.metrics.ReputationRejects,
		Allowed:            m.metrics.Allowed,
	}
}

// ResetSource resets security state for a source
func (m *Manager) ResetSource(sourceID string) {
	// Reset rate limits
	rateLimitKey := announce.RateLimitKey("announce", sourceID)
	m.rateLimiter.Reset(rateLimitKey)
	
	// Note: We don't reset reputation as it should persist
}

// Close closes all security components
func (m *Manager) Close() {
	m.rateLimiter.Close()
	m.spamDetector.Close()
	m.reputation.Close()
}

// Helper methods

func (m *Manager) incrementMetric(metric *int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	*metric++
}

// SourceInfo contains security information about a source
type SourceInfo struct {
	SourceID   string
	RateLimit  announce.RateLimitStatus
	Reputation *announce.SourceReputation
	TrustLevel string
}

// SecurityReport generates a security report
func (m *Manager) SecurityReport() SecurityReport {
	metrics := m.GetMetrics()
	
	var successRate float64
	if metrics.TotalChecked > 0 {
		successRate = float64(metrics.Allowed) / float64(metrics.TotalChecked) * 100
	}
	
	return SecurityReport{
		Metrics:          metrics,
		SuccessRate:      successRate,
		SpamStats:        m.spamDetector.GetStats(),
		ReputationStats:  m.reputation.GetStats(),
		GeneratedAt:      time.Now(),
	}
}

// SecurityReport contains comprehensive security statistics
type SecurityReport struct {
	Metrics         SecurityMetrics
	SuccessRate     float64
	SpamStats       announce.SpamStats
	ReputationStats announce.ReputationStats
	GeneratedAt     time.Time
}
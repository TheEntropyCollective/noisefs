package search

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/fuse"
)

// PrivacyQueryTransformer applies privacy transformations to search queries
type PrivacyQueryTransformer struct {
	noiseGenerator *QueryNoiseGenerator
	kanonymizer    *KAnonymizer
	config         *TransformationConfig
	mu             sync.RWMutex
}

// TransformationConfig configures privacy transformations
type TransformationConfig struct {
	EnableDummyQueries    bool    `json:"enable_dummy_queries"`
	DummyQueryCount       int     `json:"dummy_query_count"`
	EnableNoiseInjection  bool    `json:"enable_noise_injection"`
	NoiseLevel           float64 `json:"noise_level"`
	EnableKAnonymity     bool    `json:"enable_k_anonymity"`
	KValue               int     `json:"k_value"`
}

// TransformationResult contains the result of privacy transformation
type TransformationResult struct {
	TransformedQuery *SearchQuery `json:"transformed_query"`
	DummyQueries     []string     `json:"dummy_queries"`
	NoiseLevel       float64      `json:"noise_level"`
	PrivacyCost      float64      `json:"privacy_cost"`
}

// QueryNoiseGenerator generates noise for query obfuscation
type QueryNoiseGenerator struct {
	noiseDictionary []string
	mu              sync.RWMutex
}

// KAnonymizer provides k-anonymity for search queries
type KAnonymizer struct {
	queryGroups map[string][]string
	mu          sync.RWMutex
}

// NewPrivacyQueryTransformer creates a new privacy query transformer
func NewPrivacyQueryTransformer() *PrivacyQueryTransformer {
	return &PrivacyQueryTransformer{
		noiseGenerator: &QueryNoiseGenerator{
			noiseDictionary: []string{"file", "document", "text", "data", "info", "content", "archive", "backup"},
		},
		kanonymizer: &KAnonymizer{
			queryGroups: make(map[string][]string),
		},
		config: &TransformationConfig{
			EnableDummyQueries:   true,
			DummyQueryCount:      3,
			EnableNoiseInjection: true,
			NoiseLevel:          0.1,
			EnableKAnonymity:    true,
			KValue:              5,
		},
	}
}

// Transform applies privacy transformations to a search query
func (pqt *PrivacyQueryTransformer) Transform(query *SearchQuery) (*TransformationResult, error) {
	pqt.mu.Lock()
	defer pqt.mu.Unlock()

	result := &TransformationResult{
		TransformedQuery: &SearchQuery{},
		DummyQueries:     make([]string, 0),
		NoiseLevel:       0.0,
		PrivacyCost:      0.0,
	}

	// Copy original query
	*result.TransformedQuery = *query

	// Apply query obfuscation based on privacy level
	if query.PrivacyLevel >= 2 {
		obfuscatedQuery, err := pqt.applyQueryObfuscation(query.Query, query.PrivacyLevel)
		if err != nil {
			return nil, fmt.Errorf("query obfuscation failed: %w", err)
		}
		result.TransformedQuery.ObfuscatedQuery = obfuscatedQuery
	}

	// Generate dummy queries for higher privacy levels
	if query.PrivacyLevel >= 3 && pqt.config.EnableDummyQueries {
		dummyQueries, err := pqt.generateDummyQueries(query, pqt.config.DummyQueryCount)
		if err != nil {
			return nil, fmt.Errorf("dummy query generation failed: %w", err)
		}
		result.DummyQueries = dummyQueries
		result.TransformedQuery.DummyQueries = dummyQueries
	}

	// Apply differential privacy noise
	if query.PrivacyLevel >= 4 && pqt.config.EnableNoiseInjection {
		noiseLevel := pqt.calculateNoiseLevel(query.PrivacyLevel)
		result.NoiseLevel = noiseLevel
	}

	// Calculate privacy budget cost
	result.PrivacyCost = pqt.calculatePrivacyCost(query, result)

	return result, nil
}

// applyQueryObfuscation obfuscates the search query
func (pqt *PrivacyQueryTransformer) applyQueryObfuscation(query string, privacyLevel int) (string, error) {
	// For higher privacy levels, apply more aggressive obfuscation
	if privacyLevel >= 4 {
		// Add noise terms
		noise := pqt.noiseGenerator.generateNoise(privacyLevel)
		return query + " " + noise, nil
	}
	
	// Basic obfuscation - normalize case and trim
	return strings.ToLower(strings.TrimSpace(query)), nil
}

// generateDummyQueries creates dummy queries for privacy protection
func (pqt *PrivacyQueryTransformer) generateDummyQueries(query *SearchQuery, count int) ([]string, error) {
	dummyQueries := make([]string, 0, count)
	
	for i := 0; i < count; i++ {
		dummy := pqt.noiseGenerator.generateDummyQuery(query.Query)
		dummyQueries = append(dummyQueries, dummy)
	}
	
	return dummyQueries, nil
}

// calculateNoiseLevel calculates the appropriate noise level
func (pqt *PrivacyQueryTransformer) calculateNoiseLevel(privacyLevel int) float64 {
	baseNoise := 0.01
	return baseNoise * float64(privacyLevel) * pqt.config.NoiseLevel
}

// calculatePrivacyCost calculates the privacy budget cost
func (pqt *PrivacyQueryTransformer) calculatePrivacyCost(query *SearchQuery, result *TransformationResult) float64 {
	baseCost := 0.001
	privacyMultiplier := float64(query.PrivacyLevel) * 0.01
	dummyQueryCost := float64(len(result.DummyQueries)) * 0.0005
	noiseCost := result.NoiseLevel * 0.01
	
	return baseCost + privacyMultiplier + dummyQueryCost + noiseCost
}

// generateNoise generates noise terms for query obfuscation
func (qng *QueryNoiseGenerator) generateNoise(privacyLevel int) string {
	qng.mu.RLock()
	defer qng.mu.RUnlock()
	
	if len(qng.noiseDictionary) == 0 {
		return ""
	}
	
	// Select random noise terms based on privacy level
	noiseCount := privacyLevel - 2
	if noiseCount <= 0 {
		return ""
	}
	
	noiseTerms := make([]string, 0, noiseCount)
	for i := 0; i < noiseCount && i < len(qng.noiseDictionary); i++ {
		term := qng.noiseDictionary[i%len(qng.noiseDictionary)]
		noiseTerms = append(noiseTerms, term)
	}
	
	return strings.Join(noiseTerms, " ")
}

// generateDummyQuery generates a dummy query based on the original
func (qng *QueryNoiseGenerator) generateDummyQuery(originalQuery string) string {
	qng.mu.RLock()
	defer qng.mu.RUnlock()
	
	if len(qng.noiseDictionary) == 0 {
		return originalQuery + "_dummy"
	}
	
	// Create a dummy query using noise dictionary
	noise := qng.noiseDictionary[len(originalQuery)%len(qng.noiseDictionary)]
	return noise + "_search"
}

// QueryValidator validates and rate-limits search queries
type QueryValidator struct {
	rateLimiter    *RateLimiter
	blockedQueries map[string]bool
	config         *ValidationConfig
	mu             sync.RWMutex
}

// ValidationConfig configures query validation
type ValidationConfig struct {
	MaxQueryLength     int           `json:"max_query_length"`
	MinQueryLength     int           `json:"min_query_length"`
	RateLimitEnabled   bool          `json:"rate_limit_enabled"`
	QueriesPerMinute   int           `json:"queries_per_minute"`
	BlockedPatterns    []string      `json:"blocked_patterns"`
	SessionTimeout     time.Duration `json:"session_timeout"`
}

// ValidationResult contains validation results
type ValidationResult struct {
	Valid         bool   `json:"valid"`
	Blocked       bool   `json:"blocked"`
	RateLimited   bool   `json:"rate_limited"`
	Reason        string `json:"reason"`
	RemainingQuota int   `json:"remaining_quota"`
}

// RateLimiter implements simple rate limiting
type RateLimiter struct {
	sessions map[string]*SessionLimits
	mu       sync.RWMutex
}

// SessionLimits tracks rate limits per session
type SessionLimits struct {
	QueryCount    int       `json:"query_count"`
	LastReset     time.Time `json:"last_reset"`
	LastQuery     time.Time `json:"last_query"`
}

// NewQueryValidator creates a new query validator
func NewQueryValidator() *QueryValidator {
	return &QueryValidator{
		rateLimiter: &RateLimiter{
			sessions: make(map[string]*SessionLimits),
		},
		blockedQueries: make(map[string]bool),
		config: &ValidationConfig{
			MaxQueryLength:   1000,
			MinQueryLength:   1,
			RateLimitEnabled: true,
			QueriesPerMinute: 60,
			BlockedPatterns:  []string{},
			SessionTimeout:   time.Hour,
		},
	}
}

// ValidateQuery validates a search query
func (qv *QueryValidator) ValidateQuery(query *SearchQuery, clientIP string) (*ValidationResult, error) {
	qv.mu.Lock()
	defer qv.mu.Unlock()

	result := &ValidationResult{
		Valid:   true,
		Blocked: false,
		RateLimited: false,
	}

	// Validate query length
	if len(query.Query) < qv.config.MinQueryLength {
		result.Valid = false
		result.Reason = "query too short"
		return result, nil
	}

	if len(query.Query) > qv.config.MaxQueryLength {
		result.Valid = false
		result.Reason = "query too long"
		return result, nil
	}

	// Check blocked patterns
	for _, pattern := range qv.config.BlockedPatterns {
		if strings.Contains(query.Query, pattern) {
			result.Valid = false
			result.Blocked = true
			result.Reason = "query contains blocked pattern"
			return result, nil
		}
	}

	// Check rate limits
	if qv.config.RateLimitEnabled {
		rateLimited, remaining := qv.rateLimiter.checkRateLimit(query.SessionID, qv.config.QueriesPerMinute)
		if rateLimited {
			result.Valid = false
			result.RateLimited = true
			result.Reason = "rate limit exceeded"
		}
		result.RemainingQuota = remaining
	}

	return result, nil
}

// checkRateLimit checks if a session has exceeded rate limits
func (rl *RateLimiter) checkRateLimit(sessionID string, maxPerMinute int) (bool, int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	
	limits, exists := rl.sessions[sessionID]
	if !exists {
		limits = &SessionLimits{
			QueryCount: 0,
			LastReset:  now,
			LastQuery:  now,
		}
		rl.sessions[sessionID] = limits
	}

	// Reset counter if a minute has passed
	if now.Sub(limits.LastReset) >= time.Minute {
		limits.QueryCount = 0
		limits.LastReset = now
	}

	// Check if limit exceeded
	if limits.QueryCount >= maxPerMinute {
		return true, 0
	}

	// Increment count and update timestamp
	limits.QueryCount++
	limits.LastQuery = now

	remaining := maxPerMinute - limits.QueryCount
	return false, remaining
}

// SearchExecutor executes search queries against the file index
type SearchExecutor struct {
	fileIndex     *fuse.FileIndex
	config        *PrivacySearchConfig
	queryCache    map[string]*CachedSearchResult
	mu            sync.RWMutex
}

// CachedSearchResult represents a cached search result
type CachedSearchResult struct {
	Results   []SearchResult `json:"results"`
	Timestamp time.Time      `json:"timestamp"`
	TTL       time.Duration  `json:"ttl"`
}

// NewSearchExecutor creates a new search executor
func NewSearchExecutor(fileIndex *fuse.FileIndex, config *PrivacySearchConfig) *SearchExecutor {
	return &SearchExecutor{
		fileIndex:  fileIndex,
		config:     config,
		queryCache: make(map[string]*CachedSearchResult),
	}
}

// ExecuteSearch executes a search query
func (se *SearchExecutor) ExecuteSearch(ctx context.Context, query *SearchQuery) ([]SearchResult, error) {
	se.mu.Lock()
	defer se.mu.Unlock()

	// Check cache first
	cacheKey := se.generateCacheKey(query)
	if cached, found := se.queryCache[cacheKey]; found {
		if time.Since(cached.Timestamp) < cached.TTL {
			return cached.Results, nil
		}
		// Remove expired cache entry
		delete(se.queryCache, cacheKey)
	}

	// Execute search against file index
	results, err := se.performSearch(query)
	if err != nil {
		return nil, err
	}

	// Cache results
	se.queryCache[cacheKey] = &CachedSearchResult{
		Results:   results,
		Timestamp: time.Now(),
		TTL:       time.Minute * 5,
	}

	return results, nil
}

// performSearch performs the actual search operation
func (se *SearchExecutor) performSearch(query *SearchQuery) ([]SearchResult, error) {
	// Get all files from the index
	allFiles := se.fileIndex.ListFiles()
	
	results := make([]SearchResult, 0)
	searchQuery := strings.ToLower(query.ObfuscatedQuery)

	for path, entry := range allFiles {
		// Simple filename matching
		filename := strings.ToLower(entry.Filename)
		if strings.Contains(filename, searchQuery) {
			result := SearchResult{
				FileID:       entry.DescriptorCID,
				Filename:     entry.Filename,
				Path:         path,
				Relevance:    se.calculateRelevance(filename, searchQuery),
				MatchType:    "filename",
				Similarity:   se.calculateSimilarity(filename, searchQuery),
				Sources:      []string{"file_index"},
				IndexSource:  "file_index",
				PrivacyLevel: query.PrivacyLevel,
				NoiseLevel:   0.0,
				LastModified: entry.ModifiedAt,
				IndexedAt:    entry.CreatedAt,
			}
			results = append(results, result)
		}

		// Limit results
		if len(results) >= query.MaxResults {
			break
		}
	}

	return results, nil
}

// calculateRelevance calculates search relevance score
func (se *SearchExecutor) calculateRelevance(filename, query string) float64 {
	if strings.Contains(filename, query) {
		if filename == query {
			return 1.0
		}
		return 0.8
	}
	return 0.1
}

// calculateSimilarity calculates similarity score
func (se *SearchExecutor) calculateSimilarity(filename, query string) float64 {
	if len(query) == 0 {
		return 0.0
	}
	
	matches := 0
	for _, char := range query {
		if strings.ContainsRune(filename, char) {
			matches++
		}
	}
	
	return float64(matches) / float64(len(query))
}

// generateCacheKey generates a cache key for the query
func (se *SearchExecutor) generateCacheKey(query *SearchQuery) string {
	return fmt.Sprintf("%s_%d_%d", query.ObfuscatedQuery, query.PrivacyLevel, query.MaxResults)
}

// UpdateConfig updates the search executor configuration
func (se *SearchExecutor) UpdateConfig(config *PrivacySearchConfig) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.config = config
}

// Default configuration functions

// DefaultResultPrivacyConfig returns default result privacy configuration
func DefaultResultPrivacyConfig() *ResultPrivacyConfig {
	return &ResultPrivacyConfig{
		EnableObfuscation:    true,
		EnableRanking:        true,
		EnableOptimization:   true,
		NoiseLevel:          0.05,
		MaxResults:          100,
		RankingAlgorithm:    "privacy_aware",
		ObfuscationLevel:    2,
	}
}

// Helper types for metrics and analytics (using existing types from other files)
package search

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"sync"
	"time"
)

// QueryValidator handles validation and rate limiting for search queries
type QueryValidator struct {
	// Rate limiting
	rateLimiter     *SearchRateLimiter
	
	// Security validation
	securityRules   []SecurityRule
	blockedPatterns []*regexp.Regexp
	allowedDomains  map[string]bool
	
	// Query analysis
	queryAnalyzer   *QueryAnalyzer
	
	// Configuration
	config          *ValidationConfig
	
	// Thread safety
	mu              sync.RWMutex
}

// ValidationConfig holds configuration for query validation
type ValidationConfig struct {
	// Rate limiting
	MaxQueriesPerMinute    int           `json:"max_queries_per_minute"`
	MaxQueriesPerHour      int           `json:"max_queries_per_hour"`
	MaxQueriesPerDay       int           `json:"max_queries_per_day"`
	BurstAllowance         int           `json:"burst_allowance"`
	
	// Query constraints
	MaxQueryLength         int           `json:"max_query_length"`
	MaxTermsPerQuery       int           `json:"max_terms_per_query"`
	MaxSimilarQueries      int           `json:"max_similar_queries"`
	SimilarityTimeWindow   time.Duration `json:"similarity_time_window"`
	
	// Security
	EnableInjectionDetection bool        `json:"enable_injection_detection"`
	EnablePatternBlocking    bool        `json:"enable_pattern_blocking"`
	EnableAnomalyDetection   bool        `json:"enable_anomaly_detection"`
	BlockSuspiciousQueries   bool        `json:"block_suspicious_queries"`
	
	// Privacy protection
	RequirePrivacyLevel      int         `json:"require_privacy_level"`
	EnforceSessionLimits     bool        `json:"enforce_session_limits"`
	MaxSessionDuration       time.Duration `json:"max_session_duration"`
}

// SearchRateLimiter implements rate limiting for search operations
type SearchRateLimiter struct {
	// Rate limiting buckets
	minuteLimits    map[string]*RateBucket
	hourLimits      map[string]*RateBucket
	dayLimits       map[string]*RateBucket
	
	// Configuration
	maxPerMinute    int
	maxPerHour      int
	maxPerDay       int
	burstAllowance  int
	
	// Cleanup
	lastCleanup     time.Time
	cleanupInterval time.Duration
	
	// Thread safety
	mu              sync.RWMutex
}

// RateBucket represents a rate limiting bucket
type RateBucket struct {
	Count     int       `json:"count"`
	ResetTime time.Time `json:"reset_time"`
	BurstUsed int       `json:"burst_used"`
}

// SecurityRule defines security validation rules
type SecurityRule struct {
	Name        string               `json:"name"`
	Pattern     *regexp.Regexp       `json:"pattern"`
	Action      SecurityAction       `json:"action"`
	Description string               `json:"description"`
	Severity    SecuritySeverity     `json:"severity"`
}

// SecurityAction defines what action to take when a rule matches
type SecurityAction int

const (
	ActionWarn SecurityAction = iota
	ActionBlock
	ActionEscalate
	ActionLog
)

// SecuritySeverity defines the severity level of security issues
type SecuritySeverity int

const (
	SeverityLow SecuritySeverity = iota
	SeverityMedium
	SeverityHigh
	SeverityCritical
)

// QueryAnalyzer analyzes queries for patterns and anomalies
type QueryAnalyzer struct {
	// Pattern tracking
	queryHistory    map[string][]QueryHistoryEntry
	patternCounts   map[string]int
	
	// Anomaly detection
	baselineMetrics *QueryBaselineMetrics
	anomalyThreshold float64
	
	// Thread safety
	mu              sync.RWMutex
}

// QueryHistoryEntry represents a historical query entry
type QueryHistoryEntry struct {
	Query       string        `json:"query"`
	Timestamp   time.Time     `json:"timestamp"`
	SourceIP    string        `json:"source_ip"`
	SessionID   string        `json:"session_id"`
	QueryType   SearchQueryType `json:"query_type"`
	Blocked     bool          `json:"blocked"`
	Reason      string        `json:"reason,omitempty"`
}

// QueryBaselineMetrics holds baseline metrics for anomaly detection
type QueryBaselineMetrics struct {
	AverageQueryLength    float64   `json:"average_query_length"`
	CommonTermFrequency   map[string]float64 `json:"common_term_frequency"`
	TypicalQueryPatterns  []string  `json:"typical_query_patterns"`
	NormalQueriesPerHour  float64   `json:"normal_queries_per_hour"`
	LastUpdated           time.Time `json:"last_updated"`
}

// ValidationResult represents the result of query validation
type ValidationResult struct {
	Valid         bool                `json:"valid"`
	Blocked       bool                `json:"blocked"`
	Warnings      []ValidationWarning `json:"warnings,omitempty"`
	RateLimited   bool                `json:"rate_limited"`
	SecurityIssues []SecurityIssue    `json:"security_issues,omitempty"`
	Recommendations []string          `json:"recommendations,omitempty"`
}

// ValidationWarning represents a validation warning
type ValidationWarning struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Severity    string `json:"severity"`
	Suggestion  string `json:"suggestion,omitempty"`
}

// SecurityIssue represents a detected security issue
type SecurityIssue struct {
	RuleName    string           `json:"rule_name"`
	Severity    SecuritySeverity `json:"severity"`
	Description string           `json:"description"`
	Action      SecurityAction   `json:"action"`
	Pattern     string           `json:"pattern,omitempty"`
}

// NewQueryValidator creates a new query validator with default configuration
func NewQueryValidator() *QueryValidator {
	config := DefaultValidationConfig()
	
	validator := &QueryValidator{
		config:        config,
		queryAnalyzer: NewQueryAnalyzer(),
	}
	
	// Initialize rate limiter
	validator.rateLimiter = NewSearchRateLimiter(
		config.MaxQueriesPerMinute,
		config.MaxQueriesPerHour,
		config.MaxQueriesPerDay,
		config.BurstAllowance,
	)
	
	// Initialize security rules
	validator.initializeSecurityRules()
	
	// Initialize blocked patterns
	validator.initializeBlockedPatterns()
	
	return validator
}

// DefaultValidationConfig returns default validation configuration
func DefaultValidationConfig() *ValidationConfig {
	return &ValidationConfig{
		MaxQueriesPerMinute:      60,
		MaxQueriesPerHour:        1000,
		MaxQueriesPerDay:         10000,
		BurstAllowance:           10,
		MaxQueryLength:           1000,
		MaxTermsPerQuery:         20,
		MaxSimilarQueries:        5,
		SimilarityTimeWindow:     time.Minute * 5,
		EnableInjectionDetection: true,
		EnablePatternBlocking:    true,
		EnableAnomalyDetection:   true,
		BlockSuspiciousQueries:   true,
		RequirePrivacyLevel:      1,
		EnforceSessionLimits:     true,
		MaxSessionDuration:       time.Hour * 4,
	}
}

// ValidateQuery validates a search query and applies rate limiting
func (qv *QueryValidator) ValidateQuery(query *SearchQuery, clientIP string) (*ValidationResult, error) {
	result := &ValidationResult{
		Valid:           true,
		Blocked:         false,
		Warnings:        make([]ValidationWarning, 0),
		SecurityIssues:  make([]SecurityIssue, 0),
		Recommendations: make([]string, 0),
	}
	
	// Rate limiting check
	if !qv.rateLimiter.AllowQuery(clientIP, query.SessionID) {
		result.Valid = false
		result.RateLimited = true
		result.Blocked = true
		return result, nil
	}
	
	// Basic validation
	if err := qv.validateBasicConstraints(query, result); err != nil {
		return result, err
	}
	
	// Security validation
	if qv.config.EnableInjectionDetection {
		qv.validateSecurity(query, result)
	}
	
	// Pattern analysis
	if qv.config.EnablePatternBlocking {
		qv.validatePatterns(query, result)
	}
	
	// Anomaly detection
	if qv.config.EnableAnomalyDetection {
		qv.detectAnomalies(query, clientIP, result)
	}
	
	// Privacy level validation
	qv.validatePrivacyLevel(query, result)
	
	// Record query for analysis
	qv.recordQuery(query, clientIP, result)
	
	// Determine if query should be blocked
	if qv.shouldBlockQuery(result) {
		result.Valid = false
		result.Blocked = true
	}
	
	return result, nil
}

// validateBasicConstraints validates basic query constraints
func (qv *QueryValidator) validateBasicConstraints(query *SearchQuery, result *ValidationResult) error {
	// Query length check
	if len(query.Query) > qv.config.MaxQueryLength {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Code:       "QUERY_TOO_LONG",
			Message:    fmt.Sprintf("Query length exceeds maximum of %d characters", qv.config.MaxQueryLength),
			Severity:   "high",
			Suggestion: "Shorten your query or break it into multiple searches",
		})
		result.Valid = false
		return nil
	}
	
	// Empty query check
	if strings.TrimSpace(query.Query) == "" {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Code:     "EMPTY_QUERY",
			Message:  "Query cannot be empty",
			Severity: "medium",
		})
		result.Valid = false
		return nil
	}
	
	// Terms count check
	terms := strings.Fields(query.Query)
	if len(terms) > qv.config.MaxTermsPerQuery {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Code:       "TOO_MANY_TERMS",
			Message:    fmt.Sprintf("Query has too many terms (max %d)", qv.config.MaxTermsPerQuery),
			Severity:   "medium",
			Suggestion: "Use fewer search terms or try exact phrase search",
		})
		result.Valid = false
		return nil
	}
	
	// Privacy level check
	if query.PrivacyLevel < qv.config.RequirePrivacyLevel {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Code:       "PRIVACY_LEVEL_TOO_LOW",
			Message:    fmt.Sprintf("Minimum privacy level is %d", qv.config.RequirePrivacyLevel),
			Severity:   "low",
			Suggestion: "Increase privacy level for better protection",
		})
	}
	
	return nil
}

// validateSecurity validates query for security issues
func (qv *QueryValidator) validateSecurity(query *SearchQuery, result *ValidationResult) {
	qv.mu.RLock()
	defer qv.mu.RUnlock()
	
	// Check against security rules
	for _, rule := range qv.securityRules {
		if rule.Pattern.MatchString(query.Query) {
			issue := SecurityIssue{
				RuleName:    rule.Name,
				Severity:    rule.Severity,
				Description: rule.Description,
				Action:      rule.Action,
				Pattern:     rule.Pattern.String(),
			}
			result.SecurityIssues = append(result.SecurityIssues, issue)
			
			// Apply action based on severity
			if rule.Action == ActionBlock || (rule.Severity >= SeverityHigh && qv.config.BlockSuspiciousQueries) {
				result.Valid = false
			}
		}
	}
	
	// Check blocked patterns
	for _, pattern := range qv.blockedPatterns {
		if pattern.MatchString(query.Query) {
			result.SecurityIssues = append(result.SecurityIssues, SecurityIssue{
				RuleName:    "BLOCKED_PATTERN",
				Severity:    SeverityHigh,
				Description: "Query contains blocked pattern",
				Action:      ActionBlock,
				Pattern:     pattern.String(),
			})
			result.Valid = false
		}
	}
}

// validatePatterns validates query patterns for suspicious behavior
func (qv *QueryValidator) validatePatterns(query *SearchQuery, result *ValidationResult) {
	// Check for similar recent queries
	similarCount := qv.queryAnalyzer.CountSimilarQueries(query.Query, query.SessionID, qv.config.SimilarityTimeWindow)
	if similarCount > qv.config.MaxSimilarQueries {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Code:       "TOO_MANY_SIMILAR_QUERIES",
			Message:    "Too many similar queries in short time",
			Severity:   "medium",
			Suggestion: "Vary your search terms or wait before searching again",
		})
	}
	
	// Check for automation patterns
	if qv.queryAnalyzer.DetectAutomationPattern(query.Query, query.SessionID) {
		result.SecurityIssues = append(result.SecurityIssues, SecurityIssue{
			RuleName:    "AUTOMATION_DETECTED",
			Severity:    SeverityMedium,
			Description: "Query pattern suggests automated searching",
			Action:      ActionWarn,
		})
	}
}

// detectAnomalies detects anomalous query behavior
func (qv *QueryValidator) detectAnomalies(query *SearchQuery, clientIP string, result *ValidationResult) {
	// Query length anomaly
	if qv.queryAnalyzer.IsQueryLengthAnomalous(query.Query) {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Code:     "QUERY_LENGTH_ANOMALY",
			Message:  "Query length is unusual compared to typical patterns",
			Severity: "low",
		})
	}
	
	// Query frequency anomaly
	if qv.queryAnalyzer.IsQueryFrequencyAnomalous(clientIP) {
		result.SecurityIssues = append(result.SecurityIssues, SecurityIssue{
			RuleName:    "FREQUENCY_ANOMALY",
			Severity:    SeverityMedium,
			Description: "Query frequency is unusually high",
			Action:      ActionWarn,
		})
	}
	
	// Content anomaly
	if qv.queryAnalyzer.IsQueryContentAnomalous(query.Query) {
		result.Warnings = append(result.Warnings, ValidationWarning{
			Code:     "CONTENT_ANOMALY",
			Message:  "Query content differs significantly from normal patterns",
			Severity: "low",
		})
	}
}

// validatePrivacyLevel validates privacy level requirements
func (qv *QueryValidator) validatePrivacyLevel(query *SearchQuery, result *ValidationResult) {
	if query.PrivacyLevel < qv.config.RequirePrivacyLevel {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Consider using privacy level %d or higher for better protection", qv.config.RequirePrivacyLevel))
	}
	
	// Recommend higher privacy for sensitive queries
	if qv.containsSensitiveTerms(query.Query) && query.PrivacyLevel < 4 {
		result.Recommendations = append(result.Recommendations,
			"Consider using higher privacy level for sensitive search terms")
	}
}

// recordQuery records query for analysis and history
func (qv *QueryValidator) recordQuery(query *SearchQuery, clientIP string, result *ValidationResult) {
	entry := QueryHistoryEntry{
		Query:     query.Query,
		Timestamp: time.Now(),
		SourceIP:  clientIP,
		SessionID: query.SessionID,
		QueryType: query.Type,
		Blocked:   result.Blocked,
	}
	
	if result.Blocked {
		reasons := make([]string, 0)
		for _, issue := range result.SecurityIssues {
			if issue.Action == ActionBlock {
				reasons = append(reasons, issue.RuleName)
			}
		}
		entry.Reason = strings.Join(reasons, ", ")
	}
	
	qv.queryAnalyzer.RecordQuery(entry)
}

// shouldBlockQuery determines if a query should be blocked based on validation results
func (qv *QueryValidator) shouldBlockQuery(result *ValidationResult) bool {
	if result.RateLimited {
		return true
	}
	
	// Block if any security issue requires blocking
	for _, issue := range result.SecurityIssues {
		if issue.Action == ActionBlock || issue.Severity >= SeverityCritical {
			return true
		}
	}
	
	// Count high-severity warnings
	highSeverityWarnings := 0
	for _, warning := range result.Warnings {
		if warning.Severity == "high" {
			highSeverityWarnings++
		}
	}
	
	// Block if too many high-severity warnings
	if highSeverityWarnings >= 2 {
		return true
	}
	
	return false
}

// initializeSecurityRules initializes security validation rules
func (qv *QueryValidator) initializeSecurityRules() {
	qv.securityRules = []SecurityRule{
		{
			Name:        "SQL_INJECTION",
			Pattern:     regexp.MustCompile(`(?i)(union|select|insert|update|delete|drop|create|alter)\s`),
			Action:      ActionBlock,
			Description: "Potential SQL injection attempt",
			Severity:    SeverityCritical,
		},
		{
			Name:        "PATH_TRAVERSAL",
			Pattern:     regexp.MustCompile(`\.\./|\.\.\\|%2e%2e/|%2e%2e\\`),
			Action:      ActionBlock,
			Description: "Path traversal attempt detected",
			Severity:    SeverityHigh,
		},
		{
			Name:        "SCRIPT_INJECTION",
			Pattern:     regexp.MustCompile(`(?i)<script|javascript:|eval\(|alert\(`),
			Action:      ActionBlock,
			Description: "Script injection attempt detected",
			Severity:    SeverityHigh,
		},
		{
			Name:        "EXCESSIVE_WILDCARDS",
			Pattern:     regexp.MustCompile(`\*{3,}|%{3,}`),
			Action:      ActionWarn,
			Description: "Excessive wildcard usage",
			Severity:    SeverityMedium,
		},
		{
			Name:        "SUSPICIOUS_CHARACTERS",
			Pattern:     regexp.MustCompile(`[<>&"'\\;]`),
			Action:      ActionWarn,
			Description: "Suspicious characters in query",
			Severity:    SeverityLow,
		},
	}
}

// initializeBlockedPatterns initializes blocked query patterns
func (qv *QueryValidator) initializeBlockedPatterns() {
	patterns := []string{
		`(?i)password\s*[:=]\s*\w+`,     // Password patterns
		`(?i)secret\s*[:=]\s*\w+`,       // Secret patterns
		`(?i)key\s*[:=]\s*\w+`,          // Key patterns
		`(?i)token\s*[:=]\s*\w+`,        // Token patterns
		`\b\d{4}-\d{4}-\d{4}-\d{4}\b`,   // Credit card patterns
		`\b\d{3}-\d{2}-\d{4}\b`,         // SSN patterns
	}
	
	qv.blockedPatterns = make([]*regexp.Regexp, len(patterns))
	for i, pattern := range patterns {
		qv.blockedPatterns[i] = regexp.MustCompile(pattern)
	}
}

// containsSensitiveTerms checks if query contains sensitive terms
func (qv *QueryValidator) containsSensitiveTerms(query string) bool {
	sensitiveTerms := []string{
		"password", "secret", "key", "token", "private", "confidential",
		"ssn", "social", "security", "credit", "card", "bank", "account",
	}
	
	queryLower := strings.ToLower(query)
	for _, term := range sensitiveTerms {
		if strings.Contains(queryLower, term) {
			return true
		}
	}
	
	return false
}

// NewSearchRateLimiter creates a new search rate limiter
func NewSearchRateLimiter(maxPerMinute, maxPerHour, maxPerDay, burstAllowance int) *SearchRateLimiter {
	return &SearchRateLimiter{
		minuteLimits:    make(map[string]*RateBucket),
		hourLimits:      make(map[string]*RateBucket),
		dayLimits:       make(map[string]*RateBucket),
		maxPerMinute:    maxPerMinute,
		maxPerHour:      maxPerHour,
		maxPerDay:       maxPerDay,
		burstAllowance:  burstAllowance,
		lastCleanup:     time.Now(),
		cleanupInterval: time.Hour,
	}
}

// AllowQuery checks if a query is allowed under rate limiting rules
func (srl *SearchRateLimiter) AllowQuery(clientIP, sessionID string) bool {
	srl.mu.Lock()
	defer srl.mu.Unlock()
	
	// Create a composite key for rate limiting
	key := fmt.Sprintf("%s:%s", clientIP, sessionID)
	
	// Clean up old entries periodically
	if time.Since(srl.lastCleanup) > srl.cleanupInterval {
		srl.cleanup()
		srl.lastCleanup = time.Now()
	}
	
	now := time.Now()
	
	// Check minute limit
	if !srl.checkBucket(srl.minuteLimits, key, srl.maxPerMinute, now, time.Minute) {
		return false
	}
	
	// Check hour limit
	if !srl.checkBucket(srl.hourLimits, key, srl.maxPerHour, now, time.Hour) {
		return false
	}
	
	// Check day limit
	if !srl.checkBucket(srl.dayLimits, key, srl.maxPerDay, now, 24*time.Hour) {
		return false
	}
	
	// All checks passed, increment counters
	srl.incrementBucket(srl.minuteLimits, key, now, time.Minute)
	srl.incrementBucket(srl.hourLimits, key, now, time.Hour)
	srl.incrementBucket(srl.dayLimits, key, now, 24*time.Hour)
	
	return true
}

// checkBucket checks if a rate limiting bucket allows the operation
func (srl *SearchRateLimiter) checkBucket(buckets map[string]*RateBucket, key string, limit int, now time.Time, window time.Duration) bool {
	bucket, exists := buckets[key]
	if !exists {
		return true // No bucket yet, allow
	}
	
	// Reset bucket if window has passed
	if now.After(bucket.ResetTime) {
		bucket.Count = 0
		bucket.BurstUsed = 0
		bucket.ResetTime = now.Add(window)
	}
	
	// Check if within limit
	if bucket.Count < limit {
		return true
	}
	
	// Check if burst allowance is available
	if bucket.BurstUsed < srl.burstAllowance {
		return true
	}
	
	return false
}

// incrementBucket increments a rate limiting bucket
func (srl *SearchRateLimiter) incrementBucket(buckets map[string]*RateBucket, key string, now time.Time, window time.Duration) {
	bucket, exists := buckets[key]
	if !exists {
		bucket = &RateBucket{
			Count:     0,
			ResetTime: now.Add(window),
			BurstUsed: 0,
		}
		buckets[key] = bucket
	}
	
	bucket.Count++
	
	// Use burst allowance if over normal limit
	limit := srl.getLimitForBucket(buckets)
	
	if bucket.Count > limit {
		bucket.BurstUsed++
	}
}

// getLimitForBucket determines the limit based on bucket type
func (srl *SearchRateLimiter) getLimitForBucket(buckets map[string]*RateBucket) int {
	// Use a simple approach to identify bucket type
	if len(buckets) == len(srl.minuteLimits) {
		return srl.maxPerMinute
	} else if len(buckets) == len(srl.hourLimits) {
		return srl.maxPerHour
	} else {
		return srl.maxPerDay
	}
}

// cleanup removes expired rate limiting entries
func (srl *SearchRateLimiter) cleanup() {
	now := time.Now()
	
	// Clean minute buckets
	for key, bucket := range srl.minuteLimits {
		if now.After(bucket.ResetTime.Add(time.Minute)) {
			delete(srl.minuteLimits, key)
		}
	}
	
	// Clean hour buckets
	for key, bucket := range srl.hourLimits {
		if now.After(bucket.ResetTime.Add(time.Hour)) {
			delete(srl.hourLimits, key)
		}
	}
	
	// Clean day buckets
	for key, bucket := range srl.dayLimits {
		if now.After(bucket.ResetTime.Add(24 * time.Hour)) {
			delete(srl.dayLimits, key)
		}
	}
}

// NewQueryAnalyzer creates a new query analyzer
func NewQueryAnalyzer() *QueryAnalyzer {
	return &QueryAnalyzer{
		queryHistory:     make(map[string][]QueryHistoryEntry),
		patternCounts:    make(map[string]int),
		anomalyThreshold: 2.0, // Standard deviations for anomaly detection
		baselineMetrics: &QueryBaselineMetrics{
			AverageQueryLength:   20.0,
			CommonTermFrequency:  make(map[string]float64),
			TypicalQueryPatterns: []string{},
			NormalQueriesPerHour: 10.0,
			LastUpdated:          time.Now(),
		},
	}
}

// RecordQuery records a query for analysis
func (qa *QueryAnalyzer) RecordQuery(entry QueryHistoryEntry) {
	qa.mu.Lock()
	defer qa.mu.Unlock()
	
	// Record in session history
	qa.queryHistory[entry.SessionID] = append(qa.queryHistory[entry.SessionID], entry)
	
	// Update pattern counts
	qa.patternCounts[entry.Query]++
	
	// Update baseline metrics periodically
	if time.Since(qa.baselineMetrics.LastUpdated) > time.Hour {
		qa.updateBaselineMetrics()
	}
}

// CountSimilarQueries counts similar queries in a time window
func (qa *QueryAnalyzer) CountSimilarQueries(query, sessionID string, window time.Duration) int {
	qa.mu.RLock()
	defer qa.mu.RUnlock()
	
	history, exists := qa.queryHistory[sessionID]
	if !exists {
		return 0
	}
	
	count := 0
	cutoff := time.Now().Add(-window)
	
	for _, entry := range history {
		if entry.Timestamp.After(cutoff) && qa.querySimilarity(query, entry.Query) > 0.8 {
			count++
		}
	}
	
	return count
}

// DetectAutomationPattern detects if queries show automation patterns
func (qa *QueryAnalyzer) DetectAutomationPattern(query, sessionID string) bool {
	qa.mu.RLock()
	defer qa.mu.RUnlock()
	
	history, exists := qa.queryHistory[sessionID]
	if !exists || len(history) < 5 {
		return false
	}
	
	// Check for regular timing patterns
	recentQueries := history[len(history)-5:]
	intervals := make([]time.Duration, len(recentQueries)-1)
	
	for i := 1; i < len(recentQueries); i++ {
		intervals[i-1] = recentQueries[i].Timestamp.Sub(recentQueries[i-1].Timestamp)
	}
	
	// Check if intervals are suspiciously regular
	avgInterval := intervals[0]
	for _, interval := range intervals[1:] {
		avgInterval = (avgInterval + interval) / 2
	}
	
	regularCount := 0
	for _, interval := range intervals {
		if absDuration(interval-avgInterval) < 100*time.Millisecond {
			regularCount++
		}
	}
	
	// If most intervals are very regular, suspect automation
	return float64(regularCount)/float64(len(intervals)) > 0.8
}

// IsQueryLengthAnomalous checks if query length is anomalous
func (qa *QueryAnalyzer) IsQueryLengthAnomalous(query string) bool {
	qa.mu.RLock()
	defer qa.mu.RUnlock()
	
	queryLength := float64(len(query))
	avgLength := qa.baselineMetrics.AverageQueryLength
	
	// Simple anomaly detection based on length
	return math.Abs(queryLength-avgLength) > avgLength*0.5
}

// IsQueryFrequencyAnomalous checks if query frequency is anomalous
func (qa *QueryAnalyzer) IsQueryFrequencyAnomalous(clientIP string) bool {
	qa.mu.RLock()
	defer qa.mu.RUnlock()
	
	// Count queries from this IP in the last hour
	count := 0
	cutoff := time.Now().Add(-time.Hour)
	
	for _, history := range qa.queryHistory {
		for _, entry := range history {
			if entry.SourceIP == clientIP && entry.Timestamp.After(cutoff) {
				count++
			}
		}
	}
	
	// Compare to normal frequency
	normalFreq := qa.baselineMetrics.NormalQueriesPerHour
	return float64(count) > normalFreq*qa.anomalyThreshold
}

// IsQueryContentAnomalous checks if query content is anomalous
func (qa *QueryAnalyzer) IsQueryContentAnomalous(query string) bool {
	qa.mu.RLock()
	defer qa.mu.RUnlock()
	
	// Simple check: query contains unusual characters or patterns
	unusualChars := []string{"{", "}", "[", "]", "<", ">", "|", "\\", "`"}
	for _, char := range unusualChars {
		if strings.Contains(query, char) {
			return true
		}
	}
	
	return false
}

// querySimilarity calculates similarity between two queries
func (qa *QueryAnalyzer) querySimilarity(query1, query2 string) float64 {
	if query1 == query2 {
		return 1.0
	}
	
	// Simple similarity based on common words
	words1 := strings.Fields(strings.ToLower(query1))
	words2 := strings.Fields(strings.ToLower(query2))
	
	if len(words1) == 0 || len(words2) == 0 {
		return 0.0
	}
	
	common := 0
	for _, word1 := range words1 {
		for _, word2 := range words2 {
			if word1 == word2 {
				common++
				break
			}
		}
	}
	
	return float64(common) / math.Max(float64(len(words1)), float64(len(words2)))
}

// updateBaselineMetrics updates baseline metrics for anomaly detection
func (qa *QueryAnalyzer) updateBaselineMetrics() {
	totalLength := 0
	queryCount := 0
	
	for _, history := range qa.queryHistory {
		for _, entry := range history {
			totalLength += len(entry.Query)
			queryCount++
		}
	}
	
	if queryCount > 0 {
		qa.baselineMetrics.AverageQueryLength = float64(totalLength) / float64(queryCount)
		qa.baselineMetrics.NormalQueriesPerHour = float64(queryCount) / 24.0 // Assume 24-hour period
	}
	
	qa.baselineMetrics.LastUpdated = time.Now()
}

// absDuration returns the absolute value of a duration
func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
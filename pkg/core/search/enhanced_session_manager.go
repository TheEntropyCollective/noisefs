package search

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

// EnhancedSessionManager provides advanced session management with privacy protection
type EnhancedSessionManager struct {
	// Session storage
	sessions         map[string]*PrivacySession
	sessionsByUser   map[string][]string
	
	// Session configuration
	config           *SessionConfig
	
	// Privacy protection
	privacyTracker   *SessionPrivacyTracker
	behaviorAnalyzer *SessionBehaviorAnalyzer
	securityMonitor  *SessionSecurityMonitor
	
	// Session analytics
	analytics        *SessionAnalytics
	
	// Cleanup management
	lastCleanup      time.Time
	cleanupInterval  time.Duration
	
	// Thread safety
	mu               sync.RWMutex
}

// SessionConfig configures session management behavior
type SessionConfig struct {
	// Session lifecycle
	DefaultTTL           time.Duration `json:"default_ttl"`
	MaxTTL               time.Duration `json:"max_ttl"`
	ExtendOnActivity     bool          `json:"extend_on_activity"`
	InactivityTimeout    time.Duration `json:"inactivity_timeout"`
	
	// Privacy settings
	EnablePrivacyTracking bool         `json:"enable_privacy_tracking"`
	PrivacyBudgetPerSession float64    `json:"privacy_budget_per_session"`
	MaxPrivacyLevel       int          `json:"max_privacy_level"`
	SessionIsolation      bool         `json:"session_isolation"`
	
	// Security settings
	EnableBehaviorAnalysis bool        `json:"enable_behavior_analysis"`
	EnableSecurityMonitoring bool      `json:"enable_security_monitoring"`
	MaxQueriesPerSession   int         `json:"max_queries_per_session"`
	SuspiciousActivityThreshold float64 `json:"suspicious_activity_threshold"`
	
	// Session limits
	MaxActiveSessions     int          `json:"max_active_sessions"`
	MaxSessionsPerUser    int          `json:"max_sessions_per_user"`
	EnableSessionSharing  bool         `json:"enable_session_sharing"`
	
	// Cleanup settings
	CleanupInterval       time.Duration `json:"cleanup_interval"`
	PreserveHistory       bool         `json:"preserve_history"`
	HistoryRetention      time.Duration `json:"history_retention"`
}

// PrivacySession represents an enhanced session with privacy tracking
type PrivacySession struct {
	// Basic session information
	SessionID        string            `json:"session_id"`
	UserID           string            `json:"user_id,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	LastActivity     time.Time         `json:"last_activity"`
	ExpiresAt        time.Time         `json:"expires_at"`
	
	// Privacy tracking
	PrivacyBudgetUsed    float64       `json:"privacy_budget_used"`
	PrivacyBudgetRemaining float64     `json:"privacy_budget_remaining"`
	MaxPrivacyLevelUsed  int           `json:"max_privacy_level_used"`
	
	// Query tracking
	TotalQueries         int           `json:"total_queries"`
	QueriesByType        map[SearchQueryType]int `json:"queries_by_type"`
	QueriesByPrivacyLevel map[int]int   `json:"queries_by_privacy_level"`
	RecentQueries        []*QueryRecord `json:"recent_queries"`
	
	// Behavior analysis
	BehaviorProfile      *SessionBehaviorProfile `json:"behavior_profile"`
	SuspiciousActivity   []SuspiciousActivityRecord `json:"suspicious_activity"`
	
	// Security tracking
	SecurityEvents       []SecurityEvent `json:"security_events"`
	ThreatLevel          ThreatLevel     `json:"threat_level"`
	
	// Session state
	IsActive             bool            `json:"is_active"`
	IsExpired            bool            `json:"is_expired"`
	IsSuspicious         bool            `json:"is_suspicious"`
	IsBlocked            bool            `json:"is_blocked"`
	
	// Metadata
	ClientInfo           *ClientInfo     `json:"client_info,omitempty"`
	SessionMetadata      map[string]interface{} `json:"session_metadata,omitempty"`
	
	// Thread safety
	mu                   sync.RWMutex
}

// QueryRecord tracks individual queries within a session
type QueryRecord struct {
	QueryID          string            `json:"query_id"`
	Query            string            `json:"query"`
	QueryType        SearchQueryType   `json:"query_type"`
	PrivacyLevel     int               `json:"privacy_level"`
	Timestamp        time.Time         `json:"timestamp"`
	Duration         time.Duration     `json:"duration"`
	ResultCount      int               `json:"result_count"`
	PrivacyBudgetUsed float64          `json:"privacy_budget_used"`
	CacheHit         bool              `json:"cache_hit"`
	SecurityFlags    []string          `json:"security_flags,omitempty"`
}

// SessionBehaviorProfile analyzes user behavior patterns
type SessionBehaviorProfile struct {
	// Query patterns
	AverageQueryLength   float64           `json:"average_query_length"`
	PreferredQueryTypes  map[SearchQueryType]float64 `json:"preferred_query_types"`
	PreferredPrivacyLevel int              `json:"preferred_privacy_level"`
	QueryFrequency       float64           `json:"query_frequency"`
	
	// Timing patterns
	TypicalSessionDuration time.Duration   `json:"typical_session_duration"`
	AverageQueryInterval   time.Duration   `json:"average_query_interval"`
	ActivityPeakHours      []int           `json:"activity_peak_hours"`
	
	// Privacy patterns
	PrivacyLevelDistribution map[int]float64 `json:"privacy_level_distribution"`
	PrivacyBudgetUsageRate   float64         `json:"privacy_budget_usage_rate"`
	
	// Behavioral indicators
	IsAutomatedPattern   bool              `json:"is_automated_pattern"`
	ConsistencyScore     float64           `json:"consistency_score"`
	AnomalyScore         float64           `json:"anomaly_score"`
	
	// Last analysis
	LastAnalyzed         time.Time         `json:"last_analyzed"`
}

// SuspiciousActivityRecord tracks suspicious activities
type SuspiciousActivityRecord struct {
	ActivityType     SuspiciousActivityType `json:"activity_type"`
	Description      string                 `json:"description"`
	Severity         SeverityLevel          `json:"severity"`
	Timestamp        time.Time              `json:"timestamp"`
	Evidence         map[string]interface{} `json:"evidence"`
	ActionTaken      string                 `json:"action_taken"`
	Resolved         bool                   `json:"resolved"`
}

// SuspiciousActivityType defines types of suspicious activities
type SuspiciousActivityType int

const (
	AutomatedQuerying SuspiciousActivityType = iota
	ExcessivePrivacyLevel
	RapidQueryPattern
	UnusualQueryContent
	PrivacyBudgetAbuse
	SessionAnomalies
	SecurityViolation
)

// SeverityLevel defines severity levels for activities
type SeverityLevel int

const (
	LowSeverity SeverityLevel = iota
	MediumSeverity
	HighSeverity
	CriticalSeverity
)

// SecurityEvent tracks security-related events
type SecurityEvent struct {
	EventType        SecurityEventType      `json:"event_type"`
	Description      string                 `json:"description"`
	Timestamp        time.Time              `json:"timestamp"`
	SourceIP         string                 `json:"source_ip,omitempty"`
	UserAgent        string                 `json:"user_agent,omitempty"`
	Evidence         map[string]interface{} `json:"evidence"`
	Severity         SeverityLevel          `json:"severity"`
	Response         string                 `json:"response"`
}

// SecurityEventType defines types of security events
type SecurityEventType int

const (
	InjectionAttempt SecurityEventType = iota
	UnauthorizedAccess
	PrivacyViolation
	SessionHijacking
	BruteForceAttempt
	MaliciousQuery
	DataExfiltration
)

// ThreatLevel defines session threat levels
type ThreatLevel int

const (
	NoThreat ThreatLevel = iota
	LowThreat
	MediumThreat
	HighThreat
	CriticalThreat
)

// ClientInfo tracks client information
type ClientInfo struct {
	IPAddress        string            `json:"ip_address"`
	UserAgent        string            `json:"user_agent"`
	Platform         string            `json:"platform"`
	Browser          string            `json:"browser"`
	Location         *LocationInfo     `json:"location,omitempty"`
	DeviceFingerprint string           `json:"device_fingerprint"`
}

// LocationInfo tracks approximate location (privacy-preserving)
type LocationInfo struct {
	Country          string            `json:"country"`
	Region           string            `json:"region"`
	Timezone         string            `json:"timezone"`
}

// SessionPrivacyTracker tracks privacy budget and compliance
type SessionPrivacyTracker struct {
	// Budget tracking
	totalBudgetAllocated map[string]float64
	budgetUsageHistory   map[string][]BudgetUsageRecord
	
	// Privacy compliance
	complianceViolations map[string][]ComplianceViolation
	privacyLevelLimits   map[int]float64
	
	// Configuration
	config               *SessionConfig
	
	// Thread safety
	mu                   sync.RWMutex
}

// BudgetUsageRecord tracks privacy budget usage
type BudgetUsageRecord struct {
	QueryID          string            `json:"query_id"`
	BudgetUsed       float64           `json:"budget_used"`
	Timestamp        time.Time         `json:"timestamp"`
	QueryType        SearchQueryType   `json:"query_type"`
	PrivacyLevel     int               `json:"privacy_level"`
}

// ComplianceViolation tracks privacy compliance violations
type ComplianceViolation struct {
	ViolationType    ComplianceViolationType `json:"violation_type"`
	Description      string                  `json:"description"`
	Timestamp        time.Time               `json:"timestamp"`
	Severity         SeverityLevel           `json:"severity"`
	Resolved         bool                    `json:"resolved"`
}

// ComplianceViolationType defines types of compliance violations
type ComplianceViolationType int

const (
	BudgetOverflow ComplianceViolationType = iota
	PrivacyLevelViolation
	SessionTimeoutViolation
	DataRetentionViolation
	AccessControlViolation
)

// SessionBehaviorAnalyzer analyzes session behavior patterns
type SessionBehaviorAnalyzer struct {
	// Behavior baselines
	behaviorBaselines    map[string]*BehaviorBaseline
	
	// Pattern detection
	patternDetectors     []*PatternDetector
	
	// Anomaly detection
	anomalyThreshold     float64
	
	// Configuration
	config               *SessionConfig
	
	// Thread safety
	mu                   sync.RWMutex
}

// BehaviorBaseline represents normal behavior patterns
type BehaviorBaseline struct {
	UserID               string            `json:"user_id"`
	NormalQueryRate      float64           `json:"normal_query_rate"`
	NormalSessionDuration time.Duration    `json:"normal_session_duration"`
	NormalPrivacyLevel   float64           `json:"normal_privacy_level"`
	TypicalQueryPatterns []string          `json:"typical_query_patterns"`
	LastUpdated          time.Time         `json:"last_updated"`
}

// PatternDetector detects specific behavior patterns
type PatternDetector struct {
	Name                 string            `json:"name"`
	Pattern              string            `json:"pattern"`
	DetectionFunction    func(*PrivacySession) bool `json:"-"`
	SeverityLevel        SeverityLevel     `json:"severity_level"`
	Enabled              bool              `json:"enabled"`
}

// SessionSecurityMonitor monitors session security
type SessionSecurityMonitor struct {
	// Security rules
	securityRules        []*SessionSecurityRule
	
	// Threat detection
	threatDetectors      []*ThreatDetector
	
	// Response actions
	responseActions      map[ThreatLevel][]SecurityAction
	
	// Configuration
	config               *SessionConfig
	
	// Thread safety
	mu                   sync.RWMutex
}

// SessionSecurityRule defines security rules for sessions
type SessionSecurityRule struct {
	RuleID               string            `json:"rule_id"`
	Name                 string            `json:"name"`
	Description          string            `json:"description"`
	Condition            func(*PrivacySession) bool `json:"-"`
	Action               SecurityAction    `json:"action"`
	Severity             SeverityLevel     `json:"severity"`
	Enabled              bool              `json:"enabled"`
}

// ThreatDetector detects security threats
type ThreatDetector struct {
	DetectorID           string            `json:"detector_id"`
	Name                 string            `json:"name"`
	ThreatType           SecurityEventType `json:"threat_type"`
	DetectionFunction    func(*PrivacySession, *QueryRecord) ThreatLevel `json:"-"`
	Enabled              bool              `json:"enabled"`
}

// SecurityAction defines security response actions
type SecurityAction int

const (
	LogEvent SecurityAction = iota
	WarnUser
	LimitPrivacyLevel
	ReduceSessionTTL
	BlockSession
	EscalateToAdmin
)

// SessionAnalytics provides session analytics
type SessionAnalytics struct {
	// Session statistics
	TotalSessions        uint64            `json:"total_sessions"`
	ActiveSessions       uint64            `json:"active_sessions"`
	ExpiredSessions      uint64            `json:"expired_sessions"`
	SuspiciousSessions   uint64            `json:"suspicious_sessions"`
	
	// Query statistics
	TotalQueries         uint64            `json:"total_queries"`
	QueriesByType        map[SearchQueryType]uint64 `json:"queries_by_type"`
	QueriesByPrivacyLevel map[int]uint64    `json:"queries_by_privacy_level"`
	
	// Privacy statistics
	TotalPrivacyBudgetUsed float64         `json:"total_privacy_budget_used"`
	AveragePrivacyLevel    float64         `json:"average_privacy_level"`
	PrivacyViolations      uint64          `json:"privacy_violations"`
	
	// Security statistics
	SecurityEvents       uint64            `json:"security_events"`
	ThreatsByLevel       map[ThreatLevel]uint64 `json:"threats_by_level"`
	BlockedSessions      uint64            `json:"blocked_sessions"`
	
	// Performance statistics
	AverageSessionDuration time.Duration   `json:"average_session_duration"`
	AverageQueryRate       float64         `json:"average_query_rate"`
	
	// Last updated
	LastUpdated          time.Time         `json:"last_updated"`
	
	// Thread safety
	mu                   sync.RWMutex
}

// NewEnhancedSessionManager creates a new enhanced session manager
func NewEnhancedSessionManager(config *SessionConfig) *EnhancedSessionManager {
	if config == nil {
		config = DefaultSessionConfig()
	}
	
	manager := &EnhancedSessionManager{
		sessions:        make(map[string]*PrivacySession),
		sessionsByUser:  make(map[string][]string),
		config:          config,
		lastCleanup:     time.Now(),
		cleanupInterval: config.CleanupInterval,
		analytics:       &SessionAnalytics{
			QueriesByType:         make(map[SearchQueryType]uint64),
			QueriesByPrivacyLevel: make(map[int]uint64),
			ThreatsByLevel:        make(map[ThreatLevel]uint64),
			LastUpdated:           time.Now(),
		},
	}
	
	// Initialize privacy tracker
	if config.EnablePrivacyTracking {
		manager.privacyTracker = NewSessionPrivacyTracker(config)
	}
	
	// Initialize behavior analyzer
	if config.EnableBehaviorAnalysis {
		manager.behaviorAnalyzer = NewSessionBehaviorAnalyzer(config)
	}
	
	// Initialize security monitor
	if config.EnableSecurityMonitoring {
		manager.securityMonitor = NewSessionSecurityMonitor(config)
	}
	
	return manager
}

// CreateSession creates a new privacy session
func (esm *EnhancedSessionManager) CreateSession(userID string, clientInfo *ClientInfo) (*PrivacySession, error) {
	esm.mu.Lock()
	defer esm.mu.Unlock()
	
	// Check session limits
	if len(esm.sessions) >= esm.config.MaxActiveSessions {
		if err := esm.evictOldestSession(); err != nil {
			return nil, fmt.Errorf("failed to evict session: %w", err)
		}
	}
	
	// Check user session limits
	if userID != "" {
		userSessions := esm.sessionsByUser[userID]
		if len(userSessions) >= esm.config.MaxSessionsPerUser {
			return nil, fmt.Errorf("maximum sessions per user exceeded")
		}
	}
	
	// Generate session ID
	sessionID, err := esm.generateSessionID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate session ID: %w", err)
	}
	
	// Create session
	now := time.Now()
	session := &PrivacySession{
		SessionID:                sessionID,
		UserID:                   userID,
		CreatedAt:                now,
		LastActivity:             now,
		ExpiresAt:                now.Add(esm.config.DefaultTTL),
		PrivacyBudgetRemaining:   esm.config.PrivacyBudgetPerSession,
		QueriesByType:            make(map[SearchQueryType]int),
		QueriesByPrivacyLevel:    make(map[int]int),
		RecentQueries:            make([]*QueryRecord, 0),
		SuspiciousActivity:       make([]SuspiciousActivityRecord, 0),
		SecurityEvents:           make([]SecurityEvent, 0),
		ThreatLevel:              NoThreat,
		IsActive:                 true,
		ClientInfo:               clientInfo,
		SessionMetadata:          make(map[string]interface{}),
		BehaviorProfile:          &SessionBehaviorProfile{
			PreferredQueryTypes:      make(map[SearchQueryType]float64),
			PrivacyLevelDistribution: make(map[int]float64),
			ActivityPeakHours:        make([]int, 0),
			LastAnalyzed:             now,
		},
	}
	
	// Store session
	esm.sessions[sessionID] = session
	if userID != "" {
		esm.sessionsByUser[userID] = append(esm.sessionsByUser[userID], sessionID)
	}
	
	// Initialize privacy tracking
	if esm.privacyTracker != nil {
		esm.privacyTracker.InitializeSession(sessionID, esm.config.PrivacyBudgetPerSession)
	}
	
	// Update analytics
	esm.analytics.mu.Lock()
	esm.analytics.TotalSessions++
	esm.analytics.ActiveSessions++
	esm.analytics.LastUpdated = time.Now()
	esm.analytics.mu.Unlock()
	
	return session, nil
}

// GetSession retrieves a session by ID
func (esm *EnhancedSessionManager) GetSession(sessionID string) (*PrivacySession, error) {
	esm.mu.RLock()
	defer esm.mu.RUnlock()
	
	session, exists := esm.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found")
	}
	
	// Check if session is expired
	if time.Now().After(session.ExpiresAt) {
		session.IsExpired = true
		session.IsActive = false
		return session, fmt.Errorf("session expired")
	}
	
	// Check if session is blocked
	if session.IsBlocked {
		return session, fmt.Errorf("session blocked")
	}
	
	return session, nil
}

// UpdateSession updates session with query activity
func (esm *EnhancedSessionManager) UpdateSession(sessionID string, query *SearchQuery) error {
	esm.mu.Lock()
	defer esm.mu.Unlock()
	
	session, exists := esm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found")
	}
	
	session.mu.Lock()
	defer session.mu.Unlock()
	
	now := time.Now()
	
	// Update activity time
	session.LastActivity = now
	
	// Extend session if configured
	if esm.config.ExtendOnActivity {
		session.ExpiresAt = now.Add(esm.config.DefaultTTL)
	}
	
	// Update query statistics
	session.TotalQueries++
	session.QueriesByType[query.Type]++
	session.QueriesByPrivacyLevel[query.PrivacyLevel]++
	
	// Track maximum privacy level used
	if query.PrivacyLevel > session.MaxPrivacyLevelUsed {
		session.MaxPrivacyLevelUsed = query.PrivacyLevel
	}
	
	// Create query record
	queryRecord := &QueryRecord{
		QueryID:      fmt.Sprintf("%s_%d", sessionID, session.TotalQueries),
		Query:        query.Query,
		QueryType:    query.Type,
		PrivacyLevel: query.PrivacyLevel,
		Timestamp:    now,
		Duration:     0, // Will be updated when query completes
	}
	
	// Add to recent queries (keep last 50)
	session.RecentQueries = append(session.RecentQueries, queryRecord)
	if len(session.RecentQueries) > 50 {
		session.RecentQueries = session.RecentQueries[1:]
	}
	
	// Run behavior analysis
	if esm.behaviorAnalyzer != nil {
		esm.behaviorAnalyzer.AnalyzeQuery(session, queryRecord)
	}
	
	// Run security monitoring
	if esm.securityMonitor != nil {
		threat := esm.securityMonitor.AssessThreat(session, queryRecord)
		if threat > session.ThreatLevel {
			session.ThreatLevel = threat
		}
	}
	
	// Update analytics
	esm.updateAnalytics(query)
	
	return nil
}

// RecordQueryResult records the result of a query execution
func (esm *EnhancedSessionManager) RecordQueryResult(sessionID string, queryResult *QueryResult) error {
	esm.mu.RLock()
	defer esm.mu.RUnlock()
	
	session, exists := esm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session not found")
	}
	
	session.mu.Lock()
	defer session.mu.Unlock()
	
	// Find the most recent query record and update it
	if len(session.RecentQueries) > 0 {
		lastQuery := session.RecentQueries[len(session.RecentQueries)-1]
		lastQuery.Duration = time.Since(lastQuery.Timestamp)
		lastQuery.ResultCount = queryResult.ResultCount
		lastQuery.PrivacyBudgetUsed = queryResult.PrivacyBudgetUsed
		lastQuery.CacheHit = queryResult.CacheHit
		
		// Update privacy budget
		session.PrivacyBudgetUsed += queryResult.PrivacyBudgetUsed
		session.PrivacyBudgetRemaining -= queryResult.PrivacyBudgetUsed
		
		// Track privacy budget usage
		if esm.privacyTracker != nil {
			esm.privacyTracker.RecordBudgetUsage(sessionID, queryResult.PrivacyBudgetUsed, lastQuery)
		}
	}
	
	return nil
}

// QueryResult represents the result of a query execution
type QueryResult struct {
	ResultCount       int     `json:"result_count"`
	PrivacyBudgetUsed float64 `json:"privacy_budget_used"`
	CacheHit          bool    `json:"cache_hit"`
	Duration          time.Duration `json:"duration"`
}

// GetActiveSessionCount returns the number of active sessions
func (esm *EnhancedSessionManager) GetActiveSessionCount() int {
	esm.mu.RLock()
	defer esm.mu.RUnlock()
	
	count := 0
	for _, session := range esm.sessions {
		if session.IsActive && !session.IsExpired {
			count++
		}
	}
	
	return count
}

// CleanupExpiredSessions removes expired sessions
func (esm *EnhancedSessionManager) CleanupExpiredSessions() error {
	esm.mu.Lock()
	defer esm.mu.Unlock()
	
	now := time.Now()
	expiredSessions := make([]string, 0)
	
	for sessionID, session := range esm.sessions {
		if now.After(session.ExpiresAt) || 
		   (session.LastActivity.Add(esm.config.InactivityTimeout).Before(now)) {
			expiredSessions = append(expiredSessions, sessionID)
		}
	}
	
	// Remove expired sessions
	for _, sessionID := range expiredSessions {
		session := esm.sessions[sessionID]
		session.IsExpired = true
		session.IsActive = false
		
		if !esm.config.PreserveHistory {
			delete(esm.sessions, sessionID)
			
			// Remove from user sessions
			if session.UserID != "" {
				userSessions := esm.sessionsByUser[session.UserID]
				for i, id := range userSessions {
					if id == sessionID {
						esm.sessionsByUser[session.UserID] = append(userSessions[:i], userSessions[i+1:]...)
						break
					}
				}
			}
		}
		
		// Update analytics
		esm.analytics.mu.Lock()
		esm.analytics.ExpiredSessions++
		esm.analytics.ActiveSessions--
		esm.analytics.mu.Unlock()
	}
	
	esm.lastCleanup = now
	return nil
}

// generateSessionID generates a secure session ID
func (esm *EnhancedSessionManager) generateSessionID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	
	hasher := sha256.New()
	hasher.Write(bytes)
	hasher.Write([]byte(time.Now().Format(time.RFC3339Nano)))
	
	return fmt.Sprintf("sess_%x", hasher.Sum(nil)[:16]), nil
}

// evictOldestSession evicts the oldest inactive session
func (esm *EnhancedSessionManager) evictOldestSession() error {
	oldestSessionID := ""
	oldestTime := time.Now()
	
	for sessionID, session := range esm.sessions {
		if !session.IsActive && session.LastActivity.Before(oldestTime) {
			oldestSessionID = sessionID
			oldestTime = session.LastActivity
		}
	}
	
	if oldestSessionID == "" {
		// If no inactive sessions, evict the oldest active session
		for sessionID, session := range esm.sessions {
			if session.LastActivity.Before(oldestTime) {
				oldestSessionID = sessionID
				oldestTime = session.LastActivity
			}
		}
	}
	
	if oldestSessionID != "" {
		session := esm.sessions[oldestSessionID]
		session.IsActive = false
		
		if !esm.config.PreserveHistory {
			delete(esm.sessions, oldestSessionID)
			
			// Remove from user sessions
			if session.UserID != "" {
				userSessions := esm.sessionsByUser[session.UserID]
				for i, id := range userSessions {
					if id == oldestSessionID {
						esm.sessionsByUser[session.UserID] = append(userSessions[:i], userSessions[i+1:]...)
						break
					}
				}
			}
		}
	}
	
	return nil
}

// updateAnalytics updates session analytics
func (esm *EnhancedSessionManager) updateAnalytics(query *SearchQuery) {
	esm.analytics.mu.Lock()
	defer esm.analytics.mu.Unlock()
	
	esm.analytics.TotalQueries++
	esm.analytics.QueriesByType[query.Type]++
	esm.analytics.QueriesByPrivacyLevel[query.PrivacyLevel]++
	esm.analytics.LastUpdated = time.Now()
}

// GetAnalytics returns session analytics
func (esm *EnhancedSessionManager) GetAnalytics() *SessionAnalytics {
	esm.analytics.mu.RLock()
	defer esm.analytics.mu.RUnlock()
	
	// Return a copy to prevent external modification
	analytics := *esm.analytics
	return &analytics
}

// DefaultSessionConfig returns default session configuration
func DefaultSessionConfig() *SessionConfig {
	return &SessionConfig{
		DefaultTTL:                  time.Hour * 4,
		MaxTTL:                      time.Hour * 24,
		ExtendOnActivity:            true,
		InactivityTimeout:           time.Hour,
		EnablePrivacyTracking:       true,
		PrivacyBudgetPerSession:     1.0,
		MaxPrivacyLevel:             5,
		SessionIsolation:            true,
		EnableBehaviorAnalysis:      true,
		EnableSecurityMonitoring:    true,
		MaxQueriesPerSession:        1000,
		SuspiciousActivityThreshold: 0.7,
		MaxActiveSessions:           10000,
		MaxSessionsPerUser:          5,
		EnableSessionSharing:        false,
		CleanupInterval:             time.Hour,
		PreserveHistory:             true,
		HistoryRetention:            time.Hour * 24 * 7, // 1 week
	}
}

// Helper functions for component creation

// NewSessionPrivacyTracker creates a new session privacy tracker
func NewSessionPrivacyTracker(config *SessionConfig) *SessionPrivacyTracker {
	return &SessionPrivacyTracker{
		totalBudgetAllocated: make(map[string]float64),
		budgetUsageHistory:   make(map[string][]BudgetUsageRecord),
		complianceViolations: make(map[string][]ComplianceViolation),
		privacyLevelLimits:   map[int]float64{
			1: 0.1, 2: 0.2, 3: 0.3, 4: 0.5, 5: 1.0,
		},
		config: config,
	}
}

// InitializeSession initializes privacy tracking for a session
func (spt *SessionPrivacyTracker) InitializeSession(sessionID string, budget float64) {
	spt.mu.Lock()
	defer spt.mu.Unlock()
	
	spt.totalBudgetAllocated[sessionID] = budget
	spt.budgetUsageHistory[sessionID] = make([]BudgetUsageRecord, 0)
	spt.complianceViolations[sessionID] = make([]ComplianceViolation, 0)
}

// RecordBudgetUsage records privacy budget usage
func (spt *SessionPrivacyTracker) RecordBudgetUsage(sessionID string, budgetUsed float64, queryRecord *QueryRecord) {
	spt.mu.Lock()
	defer spt.mu.Unlock()
	
	record := BudgetUsageRecord{
		QueryID:      queryRecord.QueryID,
		BudgetUsed:   budgetUsed,
		Timestamp:    time.Now(),
		QueryType:    queryRecord.QueryType,
		PrivacyLevel: queryRecord.PrivacyLevel,
	}
	
	spt.budgetUsageHistory[sessionID] = append(spt.budgetUsageHistory[sessionID], record)
}

// NewSessionBehaviorAnalyzer creates a new session behavior analyzer
func NewSessionBehaviorAnalyzer(config *SessionConfig) *SessionBehaviorAnalyzer {
	return &SessionBehaviorAnalyzer{
		behaviorBaselines: make(map[string]*BehaviorBaseline),
		patternDetectors:  make([]*PatternDetector, 0),
		anomalyThreshold:  2.0, // Standard deviations
		config:            config,
	}
}

// AnalyzeQuery analyzes a query for behavioral patterns
func (sba *SessionBehaviorAnalyzer) AnalyzeQuery(session *PrivacySession, queryRecord *QueryRecord) {
	sba.mu.RLock()
	defer sba.mu.RUnlock()
	
	// Update behavior profile
	profile := session.BehaviorProfile
	
	// Update query type preferences
	totalQueries := float64(session.TotalQueries)
	if totalQueries > 0 {
		for queryType, count := range session.QueriesByType {
			profile.PreferredQueryTypes[queryType] = float64(count) / totalQueries
		}
	}
	
	// Update privacy level distribution
	for privacyLevel, count := range session.QueriesByPrivacyLevel {
		profile.PrivacyLevelDistribution[privacyLevel] = float64(count) / totalQueries
	}
	
	// Calculate query frequency
	if len(session.RecentQueries) > 1 {
		sessionDuration := time.Since(session.CreatedAt)
		profile.QueryFrequency = float64(session.TotalQueries) / sessionDuration.Hours()
	}
	
	profile.LastAnalyzed = time.Now()
}

// NewSessionSecurityMonitor creates a new session security monitor
func NewSessionSecurityMonitor(config *SessionConfig) *SessionSecurityMonitor {
	monitor := &SessionSecurityMonitor{
		securityRules:   make([]*SessionSecurityRule, 0),
		threatDetectors: make([]*ThreatDetector, 0),
		responseActions: make(map[ThreatLevel][]SecurityAction),
		config:          config,
	}
	
	// Initialize default response actions
	monitor.responseActions[LowThreat] = []SecurityAction{LogEvent}
	monitor.responseActions[MediumThreat] = []SecurityAction{LogEvent, WarnUser}
	monitor.responseActions[HighThreat] = []SecurityAction{LogEvent, LimitPrivacyLevel, ReduceSessionTTL}
	monitor.responseActions[CriticalThreat] = []SecurityAction{LogEvent, BlockSession, EscalateToAdmin}
	
	return monitor
}

// AssessThreat assesses the threat level for a query
func (ssm *SessionSecurityMonitor) AssessThreat(session *PrivacySession, queryRecord *QueryRecord) ThreatLevel {
	ssm.mu.RLock()
	defer ssm.mu.RUnlock()
	
	maxThreat := NoThreat
	
	// Run threat detectors
	for _, detector := range ssm.threatDetectors {
		if detector.Enabled {
			threat := detector.DetectionFunction(session, queryRecord)
			if threat > maxThreat {
				maxThreat = threat
			}
		}
	}
	
	// Check for basic threat indicators
	if session.TotalQueries > ssm.config.MaxQueriesPerSession {
		maxThreat = MediumThreat
	}
	
	if queryRecord.PrivacyLevel > ssm.config.MaxPrivacyLevel {
		maxThreat = HighThreat
	}
	
	return maxThreat
}
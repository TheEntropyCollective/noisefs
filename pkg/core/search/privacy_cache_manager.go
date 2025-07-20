package search

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

// PrivacyCacheManager provides privacy-aware caching for search results
type PrivacyCacheManager struct {
	// Core caching components
	privacyCache     *PrivacyAwareCache
	sessionCache     *SessionCache
	queryCache       *QueryCache
	
	// Privacy configuration
	privacyConfig    *CachePrivacyConfig
	
	// Analytics and monitoring
	cacheAnalytics   *CacheAnalytics
	
	// Thread safety
	mu               sync.RWMutex
}

// CachePrivacyConfig configures privacy-preserving cache behavior
type CachePrivacyConfig struct {
	// Privacy settings
	EnablePrivacyCache     bool          `json:"enable_privacy_cache"`
	MaxPrivacyLevel        int           `json:"max_privacy_level"`
	PrivacyTTL             time.Duration `json:"privacy_ttl"`
	
	// Cache isolation
	EnableSessionIsolation bool          `json:"enable_session_isolation"`
	EnableLevelIsolation   bool          `json:"enable_level_isolation"`
	CrossSessionSharing    bool          `json:"cross_session_sharing"`
	
	// Cache obfuscation
	EnableCacheObfuscation bool          `json:"enable_cache_obfuscation"`
	ObfuscationSalt        []byte        `json:"-"`
	KeyRotationInterval    time.Duration `json:"key_rotation_interval"`
	
	// Performance
	MaxCacheSize           int           `json:"max_cache_size"`
	MaxSessionCaches       int           `json:"max_session_caches"`
	EvictionPolicy         EvictionPolicy `json:"eviction_policy"`
	
	// Privacy budget
	CacheBudgetEnabled     bool          `json:"cache_budget_enabled"`
	BudgetPerQuery         float64       `json:"budget_per_query"`
	MaxBudgetPerSession    float64       `json:"max_budget_per_session"`
}

// EvictionPolicy defines cache eviction strategies
type EvictionPolicy int

const (
	LRUEviction EvictionPolicy = iota
	LFUEviction
	PrivacyAwareEviction
	TimeBasedEviction
	BudgetBasedEviction
)

// PrivacyAwareCache implements privacy-preserving caching
type PrivacyAwareCache struct {
	// Cache storage organized by privacy level
	levelCaches      map[int]*LevelCache
	
	// Privacy protection
	encryptionKeys   map[int][]byte
	obfuscationKeys  map[int][]byte
	
	// Configuration
	config           *CachePrivacyConfig
	
	// Statistics
	hitStats         map[int]*CacheStats
	missStats        map[int]*CacheStats
	
	// Thread safety
	mu               sync.RWMutex
}

// LevelCache provides caching for a specific privacy level
type LevelCache struct {
	// Cache entries
	entries          map[string]*PrivacyCacheEntry
	
	// Access tracking
	accessOrder      []string
	accessFrequency  map[string]int
	lastAccess       map[string]time.Time
	
	// Privacy level
	privacyLevel     int
	
	// Configuration
	maxSize          int
	ttl              time.Duration
	
	// Thread safety
	mu               sync.RWMutex
}

// PrivacyCacheEntry represents a cached search result with privacy protection
type PrivacyCacheEntry struct {
	// Cached data
	Results          []SearchResult    `json:"results"`
	Query            *SearchQuery      `json:"query"`
	
	// Privacy metadata
	PrivacyLevel     int               `json:"privacy_level"`
	SessionID        string            `json:"session_id"`
	EncryptedKey     []byte            `json:"encrypted_key"`
	NoiseLevel       float64           `json:"noise_level"`
	
	// Cache metadata
	CreatedAt        time.Time         `json:"created_at"`
	LastAccessed     time.Time         `json:"last_accessed"`
	AccessCount      int               `json:"access_count"`
	TTL              time.Duration     `json:"ttl"`
	
	// Privacy budget tracking
	BudgetUsed       float64           `json:"budget_used"`
	BudgetRemaining  float64           `json:"budget_remaining"`
}

// SessionCache manages caches per session for privacy isolation
type SessionCache struct {
	// Session-specific caches
	sessionCaches    map[string]*PrivacyAwareCache
	
	// Session metadata
	sessionMetadata  map[string]*SessionCacheMetadata
	
	// Configuration
	config           *CachePrivacyConfig
	
	// Cleanup
	lastCleanup      time.Time
	cleanupInterval  time.Duration
	
	// Thread safety
	mu               sync.RWMutex
}

// SessionCacheMetadata tracks session cache information
type SessionCacheMetadata struct {
	SessionID        string            `json:"session_id"`
	CreatedAt        time.Time         `json:"created_at"`
	LastActivity     time.Time         `json:"last_activity"`
	TotalQueries     int               `json:"total_queries"`
	CacheHits        int               `json:"cache_hits"`
	CacheMisses      int               `json:"cache_misses"`
	PrivacyBudgetUsed float64          `json:"privacy_budget_used"`
	MaxPrivacyLevel  int               `json:"max_privacy_level"`
}

// QueryCache provides fast lookup for frequently accessed queries
type QueryCache struct {
	// Query cache organized by hash
	queryHashes      map[string]*QueryCacheEntry
	
	// Frequency tracking
	queryFrequency   map[string]*QueryFrequencyData
	
	// Configuration
	config           *CachePrivacyConfig
	
	// Thread safety
	mu               sync.RWMutex
}

// QueryCacheEntry represents a cached query pattern
type QueryCacheEntry struct {
	QueryHash        string            `json:"query_hash"`
	OriginalQuery    string            `json:"original_query"`
	ObfuscatedQuery  string            `json:"obfuscated_query"`
	QueryType        SearchQueryType   `json:"query_type"`
	PrivacyLevel     int               `json:"privacy_level"`
	CreatedAt        time.Time         `json:"created_at"`
	LastUsed         time.Time         `json:"last_used"`
	UseCount         int               `json:"use_count"`
}

// QueryFrequencyData tracks query frequency for privacy analysis
type QueryFrequencyData struct {
	QueryPattern     string            `json:"query_pattern"`
	TotalCount       int               `json:"total_count"`
	SessionCounts    map[string]int    `json:"session_counts"`
	LastSeen         time.Time         `json:"last_seen"`
	PrivacyRisk      float64           `json:"privacy_risk"`
}

// CacheAnalytics provides analytics for cache performance and privacy
type CacheAnalytics struct {
	// Performance metrics
	totalHits        uint64
	totalMisses      uint64
	totalEvictions   uint64
	
	// Privacy metrics
	privacyBudgetUsed map[int]float64
	crossSessionHits  uint64
	isolationViolations uint64
	
	// Timing metrics
	averageHitTime   time.Duration
	averageMissTime  time.Duration
	
	// Last updated
	lastUpdated      time.Time
	
	// Thread safety
	mu               sync.RWMutex
}

// CacheStats tracks cache statistics per privacy level
type CacheStats struct {
	Hits             uint64
	Misses           uint64
	Evictions        uint64
	BudgetUsed       float64
	LastUpdated      time.Time
}

// NewPrivacyCacheManager creates a new privacy-aware cache manager
func NewPrivacyCacheManager(config *CachePrivacyConfig) *PrivacyCacheManager {
	if config == nil {
		config = DefaultCachePrivacyConfig()
	}
	
	manager := &PrivacyCacheManager{
		privacyConfig:  config,
		cacheAnalytics: &CacheAnalytics{
			privacyBudgetUsed: make(map[int]float64),
			lastUpdated:       time.Now(),
		},
	}
	
	// Initialize privacy-aware cache
	manager.privacyCache = NewPrivacyAwareCache(config)
	
	// Initialize session cache if enabled
	if config.EnableSessionIsolation {
		manager.sessionCache = NewSessionCache(config)
	}
	
	// Initialize query cache
	manager.queryCache = NewQueryCache(config)
	
	return manager
}

// GetCachedResults retrieves cached results with privacy protection
func (pcm *PrivacyCacheManager) GetCachedResults(query *SearchQuery) ([]SearchResult, bool) {
	pcm.mu.RLock()
	defer pcm.mu.RUnlock()
	
	startTime := time.Now()
	
	// Check if caching is enabled for this privacy level
	if !pcm.shouldCache(query) {
		pcm.recordCacheMiss(query, "privacy_level_too_high")
		return nil, false
	}
	
	// Generate cache key
	cacheKey := pcm.generateCacheKey(query)
	
	var results []SearchResult
	var found bool
	
	// Try session cache first if enabled
	if pcm.privacyConfig.EnableSessionIsolation && pcm.sessionCache != nil {
		results, found = pcm.sessionCache.Get(query.SessionID, cacheKey, query.PrivacyLevel)
		if found {
			pcm.recordCacheHit(query, "session_cache", time.Since(startTime))
			return results, true
		}
	}
	
	// Try privacy-aware cache
	results, found = pcm.privacyCache.Get(cacheKey, query.PrivacyLevel)
	if found {
		// Check cross-session sharing policy
		if pcm.privacyConfig.CrossSessionSharing || pcm.isSameSession(cacheKey, query.SessionID) {
			pcm.recordCacheHit(query, "privacy_cache", time.Since(startTime))
			return results, true
		}
	}
	
	// Cache miss
	pcm.recordCacheMiss(query, "not_found")
	return nil, false
}

// CacheResults stores results in the cache with privacy protection
func (pcm *PrivacyCacheManager) CacheResults(query *SearchQuery, results []SearchResult) error {
	pcm.mu.Lock()
	defer pcm.mu.Unlock()
	
	// Check if caching is enabled for this privacy level
	if !pcm.shouldCache(query) {
		return nil // Silently skip caching for high privacy levels
	}
	
	// Generate cache key
	cacheKey := pcm.generateCacheKey(query)
	
	// Create cache entry
	entry := &PrivacyCacheEntry{
		Results:         results,
		Query:           query,
		PrivacyLevel:    query.PrivacyLevel,
		SessionID:       query.SessionID,
		CreatedAt:       time.Now(),
		LastAccessed:    time.Now(),
		AccessCount:     1,
		TTL:             pcm.calculateTTL(query),
		NoiseLevel:      pcm.calculateCacheNoiseLevel(query),
	}
	
	// Apply cache obfuscation if enabled
	if pcm.privacyConfig.EnableCacheObfuscation {
		if err := pcm.applyCacheObfuscation(entry); err != nil {
			return fmt.Errorf("cache obfuscation failed: %w", err)
		}
	}
	
	// Calculate privacy budget usage
	if pcm.privacyConfig.CacheBudgetEnabled {
		budgetUsed := pcm.calculateCacheBudgetUsage(query, results)
		entry.BudgetUsed = budgetUsed
		
		// Check budget constraints
		if !pcm.checkBudgetConstraints(query.SessionID, budgetUsed) {
			return fmt.Errorf("insufficient privacy budget for caching")
		}
	}
	
	// Store in session cache if enabled
	if pcm.privacyConfig.EnableSessionIsolation && pcm.sessionCache != nil {
		if err := pcm.sessionCache.Put(query.SessionID, cacheKey, entry); err != nil {
			return fmt.Errorf("session cache storage failed: %w", err)
		}
	}
	
	// Store in privacy-aware cache
	if err := pcm.privacyCache.Put(cacheKey, entry); err != nil {
		return fmt.Errorf("privacy cache storage failed: %w", err)
	}
	
	// Update query cache
	pcm.queryCache.UpdateQueryFrequency(query)
	
	return nil
}

// InvalidateCache invalidates cached entries based on privacy requirements
func (pcm *PrivacyCacheManager) InvalidateCache(sessionID string, privacyLevel int) error {
	pcm.mu.Lock()
	defer pcm.mu.Unlock()
	
	// Invalidate session cache if enabled
	if pcm.privacyConfig.EnableSessionIsolation && pcm.sessionCache != nil {
		pcm.sessionCache.InvalidateSession(sessionID)
	}
	
	// Invalidate privacy cache entries for the session
	pcm.privacyCache.InvalidateBySession(sessionID, privacyLevel)
	
	return nil
}

// shouldCache determines if a query should be cached based on privacy level
func (pcm *PrivacyCacheManager) shouldCache(query *SearchQuery) bool {
	// Don't cache queries above maximum privacy level
	if query.PrivacyLevel > pcm.privacyConfig.MaxPrivacyLevel {
		return false
	}
	
	// Don't cache if privacy caching is disabled
	if !pcm.privacyConfig.EnablePrivacyCache {
		return false
	}
	
	// Check query frequency to avoid caching too frequent queries
	frequency := pcm.queryCache.GetQueryFrequency(query)
	if frequency > 10 { // High frequency might indicate pattern tracking
		return false
	}
	
	return true
}

// generateCacheKey generates a privacy-aware cache key
func (pcm *PrivacyCacheManager) generateCacheKey(query *SearchQuery) string {
	hasher := sha256.New()
	
	// Include query content
	hasher.Write([]byte(query.ObfuscatedQuery))
	
	// Include query type
	hasher.Write([]byte(fmt.Sprintf("%d", int(query.Type))))
	
	// Include privacy level
	hasher.Write([]byte(fmt.Sprintf("%d", query.PrivacyLevel)))
	
	// Include obfuscation salt if enabled
	if pcm.privacyConfig.EnableCacheObfuscation {
		hasher.Write(pcm.privacyConfig.ObfuscationSalt)
	}
	
	// Include session ID for session isolation
	if pcm.privacyConfig.EnableSessionIsolation {
		hasher.Write([]byte(query.SessionID))
	}
	
	return fmt.Sprintf("%x", hasher.Sum(nil))
}

// calculateTTL calculates cache TTL based on privacy level
func (pcm *PrivacyCacheManager) calculateTTL(query *SearchQuery) time.Duration {
	baseTTL := pcm.privacyConfig.PrivacyTTL
	
	// Reduce TTL for higher privacy levels
	privacyFactor := 1.0 - (float64(query.PrivacyLevel-1) * 0.15)
	if privacyFactor < 0.1 {
		privacyFactor = 0.1
	}
	
	return time.Duration(float64(baseTTL) * privacyFactor)
}

// calculateCacheNoiseLevel calculates noise level for cached results
func (pcm *PrivacyCacheManager) calculateCacheNoiseLevel(query *SearchQuery) float64 {
	baseNoise := 0.005 // 0.5% base noise for cached results
	privacyNoise := float64(query.PrivacyLevel) * 0.01
	
	return baseNoise + privacyNoise
}

// applyCacheObfuscation applies obfuscation to cache entries
func (pcm *PrivacyCacheManager) applyCacheObfuscation(entry *PrivacyCacheEntry) error {
	if len(pcm.privacyConfig.ObfuscationSalt) == 0 {
		return fmt.Errorf("obfuscation salt not configured")
	}
	
	// Generate encryption key for this entry
	hasher := sha256.New()
	hasher.Write(pcm.privacyConfig.ObfuscationSalt)
	hasher.Write([]byte(entry.SessionID))
	hasher.Write([]byte(fmt.Sprintf("%d", entry.PrivacyLevel)))
	
	entry.EncryptedKey = hasher.Sum(nil)
	
	return nil
}

// calculateCacheBudgetUsage calculates privacy budget usage for caching
func (pcm *PrivacyCacheManager) calculateCacheBudgetUsage(query *SearchQuery, results []SearchResult) float64 {
	baseBudget := pcm.privacyConfig.BudgetPerQuery
	
	// Adjust based on result count
	resultFactor := float64(len(results)) * 0.001
	
	// Adjust based on privacy level
	privacyFactor := float64(query.PrivacyLevel) * 0.01
	
	return baseBudget + resultFactor + privacyFactor
}

// checkBudgetConstraints checks if caching is within budget constraints
func (pcm *PrivacyCacheManager) checkBudgetConstraints(sessionID string, budgetUsed float64) bool {
	if !pcm.privacyConfig.CacheBudgetEnabled {
		return true
	}
	
	// Check session-level budget if session cache is enabled
	if pcm.privacyConfig.EnableSessionIsolation && pcm.sessionCache != nil {
		sessionBudget := pcm.sessionCache.GetSessionBudgetUsed(sessionID)
		if sessionBudget+budgetUsed > pcm.privacyConfig.MaxBudgetPerSession {
			return false
		}
	}
	
	return true
}

// isSameSession checks if a cache entry belongs to the same session
func (pcm *PrivacyCacheManager) isSameSession(cacheKey, sessionID string) bool {
	// This is a simplified check - in a full implementation,
	// this would decode the session ID from the cache key
	return true
}

// recordCacheHit records a cache hit for analytics
func (pcm *PrivacyCacheManager) recordCacheHit(query *SearchQuery, source string, duration time.Duration) {
	pcm.cacheAnalytics.mu.Lock()
	defer pcm.cacheAnalytics.mu.Unlock()
	
	pcm.cacheAnalytics.totalHits++
	
	// Update average hit time (exponential moving average)
	alpha := 0.1
	pcm.cacheAnalytics.averageHitTime = time.Duration(
		alpha*float64(duration) + (1-alpha)*float64(pcm.cacheAnalytics.averageHitTime),
	)
	
	pcm.cacheAnalytics.lastUpdated = time.Now()
}

// recordCacheMiss records a cache miss for analytics
func (pcm *PrivacyCacheManager) recordCacheMiss(query *SearchQuery, reason string) {
	pcm.cacheAnalytics.mu.Lock()
	defer pcm.cacheAnalytics.mu.Unlock()
	
	pcm.cacheAnalytics.totalMisses++
	pcm.cacheAnalytics.lastUpdated = time.Now()
}

// GetAnalytics returns cache analytics
func (pcm *PrivacyCacheManager) GetAnalytics() *CacheAnalytics {
	pcm.cacheAnalytics.mu.RLock()
	defer pcm.cacheAnalytics.mu.RUnlock()
	
	// Return a copy to prevent external modification
	analytics := *pcm.cacheAnalytics
	return &analytics
}

// DefaultCachePrivacyConfig returns default cache privacy configuration
func DefaultCachePrivacyConfig() *CachePrivacyConfig {
	return &CachePrivacyConfig{
		EnablePrivacyCache:     true,
		MaxPrivacyLevel:        4,
		PrivacyTTL:             time.Hour,
		EnableSessionIsolation: true,
		EnableLevelIsolation:   true,
		CrossSessionSharing:    false,
		EnableCacheObfuscation: true,
		ObfuscationSalt:        []byte("noisefs_cache_salt_2024"),
		KeyRotationInterval:    time.Hour * 6,
		MaxCacheSize:           1000,
		MaxSessionCaches:       100,
		EvictionPolicy:         PrivacyAwareEviction,
		CacheBudgetEnabled:     true,
		BudgetPerQuery:         0.001,
		MaxBudgetPerSession:    1.0,
	}
}

// Helper functions for cache implementations (simplified for brevity)

// NewPrivacyAwareCache creates a new privacy-aware cache
func NewPrivacyAwareCache(config *CachePrivacyConfig) *PrivacyAwareCache {
	cache := &PrivacyAwareCache{
		levelCaches:     make(map[int]*LevelCache),
		encryptionKeys:  make(map[int][]byte),
		obfuscationKeys: make(map[int][]byte),
		config:          config,
		hitStats:        make(map[int]*CacheStats),
		missStats:       make(map[int]*CacheStats),
	}
	
	// Initialize level caches
	for level := 1; level <= config.MaxPrivacyLevel; level++ {
		cache.levelCaches[level] = NewLevelCache(level, config)
		cache.hitStats[level] = &CacheStats{}
		cache.missStats[level] = &CacheStats{}
	}
	
	return cache
}

// Get retrieves an entry from the privacy-aware cache
func (pac *PrivacyAwareCache) Get(key string, privacyLevel int) ([]SearchResult, bool) {
	pac.mu.RLock()
	defer pac.mu.RUnlock()
	
	levelCache, exists := pac.levelCaches[privacyLevel]
	if !exists {
		return nil, false
	}
	
	entry, found := levelCache.Get(key)
	if !found {
		pac.missStats[privacyLevel].Misses++
		return nil, false
	}
	
	pac.hitStats[privacyLevel].Hits++
	return entry.Results, true
}

// Put stores an entry in the privacy-aware cache
func (pac *PrivacyAwareCache) Put(key string, entry *PrivacyCacheEntry) error {
	pac.mu.Lock()
	defer pac.mu.Unlock()
	
	levelCache, exists := pac.levelCaches[entry.PrivacyLevel]
	if !exists {
		return fmt.Errorf("privacy level %d not supported", entry.PrivacyLevel)
	}
	
	return levelCache.Put(key, entry)
}

// InvalidateBySession invalidates entries by session
func (pac *PrivacyAwareCache) InvalidateBySession(sessionID string, privacyLevel int) {
	pac.mu.Lock()
	defer pac.mu.Unlock()
	
	levelCache, exists := pac.levelCaches[privacyLevel]
	if exists {
		levelCache.InvalidateBySession(sessionID)
	}
}

// NewLevelCache creates a new level cache
func NewLevelCache(privacyLevel int, config *CachePrivacyConfig) *LevelCache {
	return &LevelCache{
		entries:         make(map[string]*PrivacyCacheEntry),
		accessOrder:     make([]string, 0),
		accessFrequency: make(map[string]int),
		lastAccess:      make(map[string]time.Time),
		privacyLevel:    privacyLevel,
		maxSize:         config.MaxCacheSize,
		ttl:             config.PrivacyTTL,
	}
}

// Get retrieves an entry from the level cache
func (lc *LevelCache) Get(key string) (*PrivacyCacheEntry, bool) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	
	entry, exists := lc.entries[key]
	if !exists {
		return nil, false
	}
	
	// Check TTL
	if time.Since(entry.CreatedAt) > entry.TTL {
		delete(lc.entries, key)
		return nil, false
	}
	
	// Update access tracking
	entry.LastAccessed = time.Now()
	entry.AccessCount++
	lc.accessFrequency[key]++
	lc.lastAccess[key] = time.Now()
	
	return entry, true
}

// Put stores an entry in the level cache
func (lc *LevelCache) Put(key string, entry *PrivacyCacheEntry) error {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	
	// Check if cache is full and eviction is needed
	if len(lc.entries) >= lc.maxSize {
		lc.evictEntry()
	}
	
	lc.entries[key] = entry
	lc.accessFrequency[key] = 1
	lc.lastAccess[key] = time.Now()
	lc.accessOrder = append(lc.accessOrder, key)
	
	return nil
}

// evictEntry evicts an entry based on the eviction policy
func (lc *LevelCache) evictEntry() {
	if len(lc.entries) == 0 {
		return
	}
	
	// Simple LRU eviction for now
	oldestKey := lc.accessOrder[0]
	oldestTime := lc.lastAccess[oldestKey]
	
	for key, accessTime := range lc.lastAccess {
		if accessTime.Before(oldestTime) {
			oldestKey = key
			oldestTime = accessTime
		}
	}
	
	delete(lc.entries, oldestKey)
	delete(lc.accessFrequency, oldestKey)
	delete(lc.lastAccess, oldestKey)
	
	// Remove from access order
	for i, key := range lc.accessOrder {
		if key == oldestKey {
			lc.accessOrder = append(lc.accessOrder[:i], lc.accessOrder[i+1:]...)
			break
		}
	}
}

// InvalidateBySession invalidates entries belonging to a session
func (lc *LevelCache) InvalidateBySession(sessionID string) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	
	keysToDelete := make([]string, 0)
	
	for key, entry := range lc.entries {
		if entry.SessionID == sessionID {
			keysToDelete = append(keysToDelete, key)
		}
	}
	
	for _, key := range keysToDelete {
		delete(lc.entries, key)
		delete(lc.accessFrequency, key)
		delete(lc.lastAccess, key)
		
		// Remove from access order
		for i, orderKey := range lc.accessOrder {
			if orderKey == key {
				lc.accessOrder = append(lc.accessOrder[:i], lc.accessOrder[i+1:]...)
				break
			}
		}
	}
}

// NewSessionCache creates a new session cache
func NewSessionCache(config *CachePrivacyConfig) *SessionCache {
	return &SessionCache{
		sessionCaches:   make(map[string]*PrivacyAwareCache),
		sessionMetadata: make(map[string]*SessionCacheMetadata),
		config:          config,
		lastCleanup:     time.Now(),
		cleanupInterval: time.Hour,
	}
}

// Get retrieves an entry from the session cache
func (sc *SessionCache) Get(sessionID, key string, privacyLevel int) ([]SearchResult, bool) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	
	sessionCache, exists := sc.sessionCaches[sessionID]
	if !exists {
		return nil, false
	}
	
	results, found := sessionCache.Get(key, privacyLevel)
	if found {
		sc.updateSessionMetadata(sessionID, true)
	} else {
		sc.updateSessionMetadata(sessionID, false)
	}
	
	return results, found
}

// Put stores an entry in the session cache
func (sc *SessionCache) Put(sessionID, key string, entry *PrivacyCacheEntry) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	
	// Create session cache if it doesn't exist
	if _, exists := sc.sessionCaches[sessionID]; !exists {
		if len(sc.sessionCaches) >= sc.config.MaxSessionCaches {
			sc.evictOldestSession()
		}
		
		sc.sessionCaches[sessionID] = NewPrivacyAwareCache(sc.config)
		sc.sessionMetadata[sessionID] = &SessionCacheMetadata{
			SessionID:    sessionID,
			CreatedAt:    time.Now(),
			LastActivity: time.Now(),
		}
	}
	
	return sc.sessionCaches[sessionID].Put(key, entry)
}

// InvalidateSession invalidates all entries for a session
func (sc *SessionCache) InvalidateSession(sessionID string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	
	delete(sc.sessionCaches, sessionID)
	delete(sc.sessionMetadata, sessionID)
}

// GetSessionBudgetUsed returns the privacy budget used by a session
func (sc *SessionCache) GetSessionBudgetUsed(sessionID string) float64 {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	
	metadata, exists := sc.sessionMetadata[sessionID]
	if !exists {
		return 0.0
	}
	
	return metadata.PrivacyBudgetUsed
}

// updateSessionMetadata updates session metadata
func (sc *SessionCache) updateSessionMetadata(sessionID string, hit bool) {
	metadata, exists := sc.sessionMetadata[sessionID]
	if !exists {
		return
	}
	
	metadata.LastActivity = time.Now()
	if hit {
		metadata.CacheHits++
	} else {
		metadata.CacheMisses++
	}
}

// evictOldestSession evicts the oldest session cache
func (sc *SessionCache) evictOldestSession() {
	oldestSessionID := ""
	oldestTime := time.Now()
	
	for sessionID, metadata := range sc.sessionMetadata {
		if metadata.LastActivity.Before(oldestTime) {
			oldestSessionID = sessionID
			oldestTime = metadata.LastActivity
		}
	}
	
	if oldestSessionID != "" {
		delete(sc.sessionCaches, oldestSessionID)
		delete(sc.sessionMetadata, oldestSessionID)
	}
}

// NewQueryCache creates a new query cache
func NewQueryCache(config *CachePrivacyConfig) *QueryCache {
	return &QueryCache{
		queryHashes:    make(map[string]*QueryCacheEntry),
		queryFrequency: make(map[string]*QueryFrequencyData),
		config:         config,
	}
}

// GetQueryFrequency returns the frequency of a query
func (qc *QueryCache) GetQueryFrequency(query *SearchQuery) int {
	qc.mu.RLock()
	defer qc.mu.RUnlock()
	
	queryHash := qc.generateQueryHash(query)
	if freq, exists := qc.queryFrequency[queryHash]; exists {
		return freq.TotalCount
	}
	
	return 0
}

// UpdateQueryFrequency updates the frequency data for a query
func (qc *QueryCache) UpdateQueryFrequency(query *SearchQuery) {
	qc.mu.Lock()
	defer qc.mu.Unlock()
	
	queryHash := qc.generateQueryHash(query)
	
	freq, exists := qc.queryFrequency[queryHash]
	if !exists {
		freq = &QueryFrequencyData{
			QueryPattern:  query.Query,
			TotalCount:    0,
			SessionCounts: make(map[string]int),
			LastSeen:      time.Now(),
			PrivacyRisk:   0.0,
		}
		qc.queryFrequency[queryHash] = freq
	}
	
	freq.TotalCount++
	freq.SessionCounts[query.SessionID]++
	freq.LastSeen = time.Now()
	
	// Update privacy risk based on frequency
	freq.PrivacyRisk = float64(freq.TotalCount) / 100.0
	if freq.PrivacyRisk > 1.0 {
		freq.PrivacyRisk = 1.0
	}
}

// generateQueryHash generates a hash for query frequency tracking
func (qc *QueryCache) generateQueryHash(query *SearchQuery) string {
	hasher := sha256.New()
	hasher.Write([]byte(query.Query))
	hasher.Write([]byte(fmt.Sprintf("%d", int(query.Type))))
	return fmt.Sprintf("%x", hasher.Sum(nil)[:8])
}
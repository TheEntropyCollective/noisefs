package search

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	math_rand "math/rand"
	"strings"
	"sync"
	"time"
)

// PrivacyQueryTransformer handles privacy-preserving transformations for search queries
type PrivacyQueryTransformer struct {
	// Privacy configuration
	noiseGenerators map[int]*NoiseGenerator
	timingObfuscator *TimingObfuscator
	termObfuscator   *TermObfuscator
	
	// Query pattern tracking
	queryPatterns    map[string]*QueryPattern
	patternMutex     sync.RWMutex
	
	// Privacy budget management
	budgetTracker    *PrivacyBudgetTracker
	
	// Statistical protection
	kAnonymityMin    int
	diversityFactor  float64
}

// NoiseGenerator generates dummy queries and noise patterns
type NoiseGenerator struct {
	privacyLevel     int
	noiseDictionary  []string
	patternTemplates []string
	seedRotation     time.Duration
	lastSeedUpdate   time.Time
	currentSeed      int64
	mu               sync.Mutex
}

// TimingObfuscator handles timing-based privacy protection
type TimingObfuscator struct {
	baseDelay       time.Duration
	randomRange     time.Duration
	adaptiveDelay   bool
	trafficPattern  map[int]float64 // Traffic patterns by hour
	mu              sync.RWMutex
}

// TermObfuscator handles query term obfuscation and k-anonymity
type TermObfuscator struct {
	synonymGroups   map[string][]string
	commonTerms     []string
	obfuscationKeys [][]byte
	hashSalt        []byte
	mu              sync.RWMutex
}

// QueryPattern tracks patterns for privacy protection
type QueryPattern struct {
	OriginalQuery   string
	HashedPattern   []byte
	Frequency       int64
	LastSeen        time.Time
	SimilarQueries  []string
	PrivacyRisk     float64
}

// PrivacyBudgetTracker manages differential privacy budget allocation
type PrivacyBudgetTracker struct {
	totalBudget     float64
	remainingBudget float64
	budgetWindow    time.Duration
	allocations     map[string]*BudgetAllocation
	refreshTime     time.Time
	mu              sync.RWMutex
}

// BudgetAllocation tracks privacy budget allocation for specific operations
type BudgetAllocation struct {
	QueryType    SearchQueryType
	Amount       float64
	Timestamp    time.Time
	SessionID    string
	PrivacyLevel int
}

// TransformResult contains the result of privacy transformation
type TransformResult struct {
	TransformedQuery *SearchQuery
	DummyQueries     []*SearchQuery
	TimingDelay      time.Duration
	PrivacyCost      float64
	KAnonymityGroup  []string
	NoiseLevel       float64
}

// NewPrivacyQueryTransformer creates a new privacy query transformer
func NewPrivacyQueryTransformer() *PrivacyQueryTransformer {
	transformer := &PrivacyQueryTransformer{
		noiseGenerators:  make(map[int]*NoiseGenerator),
		queryPatterns:    make(map[string]*QueryPattern),
		kAnonymityMin:    5,
		diversityFactor:  0.3,
	}
	
	// Initialize noise generators for each privacy level
	for level := 1; level <= 5; level++ {
		transformer.noiseGenerators[level] = NewNoiseGenerator(level)
	}
	
	// Initialize timing obfuscator
	transformer.timingObfuscator = NewTimingObfuscator()
	
	// Initialize term obfuscator
	transformer.termObfuscator = NewTermObfuscator()
	
	// Initialize privacy budget tracker
	transformer.budgetTracker = NewPrivacyBudgetTracker(1.0, 24*time.Hour)
	
	return transformer
}

// Transform applies privacy transformations to a search query
func (pt *PrivacyQueryTransformer) Transform(query *SearchQuery) (*TransformResult, error) {
	result := &TransformResult{
		TransformedQuery: &SearchQuery{},
		DummyQueries:     make([]*SearchQuery, 0),
	}
	
	// Copy original query
	*result.TransformedQuery = *query
	
	// Calculate privacy cost
	result.PrivacyCost = pt.calculatePrivacyCost(query)
	
	// Check and allocate privacy budget
	if !pt.budgetTracker.AllocateBudget(query.SessionID, result.PrivacyCost, query.Type, query.PrivacyLevel) {
		return nil, fmt.Errorf("insufficient privacy budget for query")
	}
	
	// Track query pattern
	pt.trackQueryPattern(query)
	
	// Apply term obfuscation
	if err := pt.applyTermObfuscation(result.TransformedQuery); err != nil {
		return nil, fmt.Errorf("term obfuscation failed: %w", err)
	}
	
	// Generate k-anonymity group
	result.KAnonymityGroup = pt.generateKAnonymityGroup(query)
	
	// Generate dummy queries
	result.DummyQueries = pt.generatePrivacyDummyQueries(query)
	
	// Calculate timing delay
	result.TimingDelay = pt.timingObfuscator.CalculateDelay(query)
	
	// Calculate noise level
	result.NoiseLevel = pt.calculateNoiseLevel(query)
	
	return result, nil
}

// NewNoiseGenerator creates a noise generator for a specific privacy level
func NewNoiseGenerator(privacyLevel int) *NoiseGenerator {
	// Base noise dictionary
	noiseDictionary := []string{
		"system", "config", "data", "file", "document", "image", "video", "audio",
		"archive", "backup", "temp", "cache", "log", "script", "source", "binary",
		"text", "report", "spreadsheet", "presentation", "database", "index",
		"metadata", "header", "footer", "content", "sample", "example", "test",
		"production", "development", "staging", "release", "version", "patch",
		"update", "install", "setup", "configure", "deploy", "build", "compile",
		"debug", "trace", "profile", "benchmark", "monitor", "analyze", "process",
	}
	
	// Pattern templates for different query types
	patternTemplates := []string{
		"%s_%s",           // term1_term2
		"%s.%s",           // term1.term2
		"%s-%s",           // term1-term2
		"%s %s",           // term1 term2
		"%s%d",            // term + number
		"backup_%s",       // backup_term
		"temp_%s",         // temp_term
		"%s_copy",         // term_copy
		"%s_old",          // term_old
		"%s_new",          // term_new
		"%s_archive",      // term_archive
		"%s_v%d",          // term_v1
	}
	
	return &NoiseGenerator{
		privacyLevel:     privacyLevel,
		noiseDictionary:  noiseDictionary,
		patternTemplates: patternTemplates,
		seedRotation:     time.Hour,
		lastSeedUpdate:   time.Now(),
		currentSeed:      time.Now().UnixNano(),
	}
}

// NewTimingObfuscator creates a timing obfuscator
func NewTimingObfuscator() *TimingObfuscator {
	// Default traffic patterns (higher values = more traffic)
	trafficPattern := map[int]float64{
		0: 0.1, 1: 0.05, 2: 0.05, 3: 0.05, 4: 0.05, 5: 0.1,
		6: 0.3, 7: 0.5, 8: 0.8, 9: 1.0, 10: 1.0, 11: 0.9,
		12: 0.8, 13: 0.9, 14: 1.0, 15: 0.9, 16: 0.8, 17: 0.7,
		18: 0.6, 19: 0.5, 20: 0.4, 21: 0.3, 22: 0.2, 23: 0.15,
	}
	
	return &TimingObfuscator{
		baseDelay:      100 * time.Millisecond,
		randomRange:    500 * time.Millisecond,
		adaptiveDelay:  true,
		trafficPattern: trafficPattern,
	}
}

// NewTermObfuscator creates a term obfuscator
func NewTermObfuscator() *TermObfuscator {
	// Common synonyms for obfuscation
	synonymGroups := map[string][]string{
		"file":     {"document", "item", "object", "entry"},
		"image":    {"picture", "photo", "graphic", "visual"},
		"video":    {"movie", "clip", "recording", "media"},
		"audio":    {"sound", "music", "recording", "track"},
		"data":     {"information", "content", "records", "dataset"},
		"config":   {"settings", "preferences", "options", "parameters"},
		"backup":   {"copy", "archive", "snapshot", "duplicate"},
		"temp":     {"temporary", "cache", "buffer", "working"},
	}
	
	// Common terms for k-anonymity
	commonTerms := []string{
		"document", "file", "data", "image", "text", "report", "config",
		"backup", "temp", "archive", "log", "script", "source", "binary",
	}
	
	// Generate obfuscation keys
	obfuscationKeys := make([][]byte, 5)
	for i := 0; i < 5; i++ {
		key := make([]byte, 32)
		rand.Read(key)
		obfuscationKeys[i] = key
	}
	
	// Generate hash salt
	hashSalt := make([]byte, 16)
	rand.Read(hashSalt)
	
	return &TermObfuscator{
		synonymGroups:   synonymGroups,
		commonTerms:     commonTerms,
		obfuscationKeys: obfuscationKeys,
		hashSalt:        hashSalt,
	}
}

// NewPrivacyBudgetTracker creates a privacy budget tracker
func NewPrivacyBudgetTracker(totalBudget float64, window time.Duration) *PrivacyBudgetTracker {
	return &PrivacyBudgetTracker{
		totalBudget:     totalBudget,
		remainingBudget: totalBudget,
		budgetWindow:    window,
		allocations:     make(map[string]*BudgetAllocation),
		refreshTime:     time.Now().Add(window),
	}
}

// calculatePrivacyCost calculates the privacy budget cost for a query
func (pt *PrivacyQueryTransformer) calculatePrivacyCost(query *SearchQuery) float64 {
	baseCost := 0.01 // Base cost for any query
	
	// Privacy level multiplier
	levelMultiplier := float64(query.PrivacyLevel) * 0.02
	
	// Query complexity multiplier
	complexityMultiplier := 1.0
	switch query.Type {
	case FilenameSearch:
		complexityMultiplier = 1.0
	case ContentSearch:
		complexityMultiplier = 1.5
	case MetadataSearch:
		complexityMultiplier = 1.2
	case SimilaritySearch:
		complexityMultiplier = 2.0
	case ComplexSearch:
		complexityMultiplier = 2.5
	}
	
	// Pattern frequency adjustment
	pattern := pt.getQueryPattern(query.Query)
	if pattern != nil && pattern.Frequency > 10 {
		// Higher cost for frequently used patterns
		complexityMultiplier *= 1.2
	}
	
	return baseCost * (1.0 + levelMultiplier) * complexityMultiplier
}

// trackQueryPattern tracks query patterns for privacy analysis
func (pt *PrivacyQueryTransformer) trackQueryPattern(query *SearchQuery) {
	pt.patternMutex.Lock()
	defer pt.patternMutex.Unlock()
	
	// Create pattern hash
	hasher := sha256.New()
	hasher.Write([]byte(query.Query))
	hasher.Write(pt.termObfuscator.hashSalt)
	patternHash := hasher.Sum(nil)
	
	patternKey := fmt.Sprintf("%x", patternHash[:8]) // Use first 8 bytes as key
	
	pattern, exists := pt.queryPatterns[patternKey]
	if !exists {
		pattern = &QueryPattern{
			OriginalQuery:  query.Query,
			HashedPattern:  patternHash,
			Frequency:      0,
			SimilarQueries: make([]string, 0),
			PrivacyRisk:    0.0,
		}
		pt.queryPatterns[patternKey] = pattern
	}
	
	pattern.Frequency++
	pattern.LastSeen = time.Now()
	
	// Calculate privacy risk based on frequency
	pattern.PrivacyRisk = math.Min(float64(pattern.Frequency)/100.0, 1.0)
}

// getQueryPattern retrieves the pattern for a query
func (pt *PrivacyQueryTransformer) getQueryPattern(query string) *QueryPattern {
	pt.patternMutex.RLock()
	defer pt.patternMutex.RUnlock()
	
	hasher := sha256.New()
	hasher.Write([]byte(query))
	hasher.Write(pt.termObfuscator.hashSalt)
	patternHash := hasher.Sum(nil)
	
	patternKey := fmt.Sprintf("%x", patternHash[:8])
	return pt.queryPatterns[patternKey]
}

// applyTermObfuscation applies term-level obfuscation to the query
func (pt *PrivacyQueryTransformer) applyTermObfuscation(query *SearchQuery) error {
	if query.PrivacyLevel < 3 {
		return nil // No obfuscation for low privacy levels
	}
	
	pt.termObfuscator.mu.RLock()
	defer pt.termObfuscator.mu.RUnlock()
	
	words := strings.Fields(query.ObfuscatedQuery)
	obfuscated := make([]string, len(words))
	
	for i, word := range words {
		// Check if word has synonyms
		if synonyms, exists := pt.termObfuscator.synonymGroups[strings.ToLower(word)]; exists {
			// Use hash to deterministically select synonym
			hasher := sha256.New()
			hasher.Write([]byte(word))
			hasher.Write(pt.termObfuscator.hashSalt)
			hash := hasher.Sum(nil)
			
			synonymIndex := binary.BigEndian.Uint32(hash[:4]) % uint32(len(synonyms))
			obfuscated[i] = synonyms[synonymIndex]
		} else {
			obfuscated[i] = word
		}
	}
	
	query.ObfuscatedQuery = strings.Join(obfuscated, " ")
	return nil
}

// generateKAnonymityGroup generates a k-anonymity group for the query
func (pt *PrivacyQueryTransformer) generateKAnonymityGroup(query *SearchQuery) []string {
	pt.termObfuscator.mu.RLock()
	defer pt.termObfuscator.mu.RUnlock()
	
	groupSize := pt.kAnonymityMin
	if query.PrivacyLevel >= 4 {
		groupSize = pt.kAnonymityMin * 2
	}
	
	group := make([]string, groupSize)
	group[0] = query.ObfuscatedQuery // Include the actual query
	
	// Add similar queries based on common terms
	for i := 1; i < groupSize; i++ {
		termIndex := i % len(pt.termObfuscator.commonTerms)
		group[i] = pt.termObfuscator.commonTerms[termIndex]
	}
	
	return group
}

// generatePrivacyDummyQueries generates dummy queries for privacy protection
func (pt *PrivacyQueryTransformer) generatePrivacyDummyQueries(query *SearchQuery) []*SearchQuery {
	generator := pt.noiseGenerators[query.PrivacyLevel]
	if generator == nil {
		return nil
	}
	
	generator.mu.Lock()
	defer generator.mu.Unlock()
	
	// Refresh seed if needed
	if time.Since(generator.lastSeedUpdate) > generator.seedRotation {
		generator.currentSeed = time.Now().UnixNano()
		generator.lastSeedUpdate = time.Now()
	}
	
	dummyCount := query.PrivacyLevel * 2 // More dummies for higher privacy
	dummies := make([]*SearchQuery, dummyCount)
	
	for i := 0; i < dummyCount; i++ {
		dummy := &SearchQuery{
			Type:         query.Type,
			MaxResults:   query.MaxResults,
			PrivacyLevel: query.PrivacyLevel,
			SessionID:    query.SessionID + "_dummy_" + fmt.Sprintf("%d", i),
			RequestTime:  query.RequestTime,
		}
		
		// Generate dummy query text
		dummy.Query = generator.generateDummyQuery(i)
		dummy.ObfuscatedQuery = dummy.Query
		
		dummies[i] = dummy
	}
	
	return dummies
}

// generateDummyQuery generates a single dummy query
func (ng *NoiseGenerator) generateDummyQuery(seed int) string {
	// Use deterministic randomness based on seed
	rng := int64(seed) + ng.currentSeed
	
	dictSize := len(ng.noiseDictionary)
	templateSize := len(ng.patternTemplates)
	
	term1Index := int(rng) % dictSize
	term2Index := int(rng/2) % dictSize
	templateIndex := int(rng/3) % templateSize
	
	term1 := ng.noiseDictionary[term1Index]
	term2 := ng.noiseDictionary[term2Index]
	template := ng.patternTemplates[templateIndex]
	
	// Generate query based on template
	if strings.Contains(template, "%d") {
		number := int(rng % 100)
		return fmt.Sprintf(template, term1, number)
	} else if strings.Count(template, "%s") == 2 {
		return fmt.Sprintf(template, term1, term2)
	} else {
		return fmt.Sprintf(template, term1)
	}
}

// CalculateDelay calculates timing delay for privacy protection
func (to *TimingObfuscator) CalculateDelay(query *SearchQuery) time.Duration {
	to.mu.RLock()
	defer to.mu.RUnlock()
	
	if query.PrivacyLevel < 2 {
		return 0 // No delay for minimal privacy
	}
	
	baseDelay := to.baseDelay * time.Duration(query.PrivacyLevel)
	
	// Add random component
	randomComponent := time.Duration(math_rand.Int63n(int64(to.randomRange)))
	
	// Apply adaptive delay based on current traffic
	if to.adaptiveDelay {
		currentHour := time.Now().Hour()
		trafficFactor := to.trafficPattern[currentHour]
		
		// Increase delay during high traffic to blend in
		adaptiveFactor := 1.0 + (trafficFactor * 0.5)
		baseDelay = time.Duration(float64(baseDelay) * adaptiveFactor)
	}
	
	return baseDelay + randomComponent
}

// calculateNoiseLevel calculates the noise level for result obfuscation
func (pt *PrivacyQueryTransformer) calculateNoiseLevel(query *SearchQuery) float64 {
	baseNoise := 0.01 // 1% base noise
	privacyMultiplier := float64(query.PrivacyLevel) * 0.02
	
	// Increase noise for frequently used queries
	pattern := pt.getQueryPattern(query.Query)
	if pattern != nil {
		frequencyMultiplier := pattern.PrivacyRisk * 0.1
		return baseNoise + privacyMultiplier + frequencyMultiplier
	}
	
	return baseNoise + privacyMultiplier
}

// AllocateBudget allocates privacy budget for a query
func (pbt *PrivacyBudgetTracker) AllocateBudget(sessionID string, cost float64, queryType SearchQueryType, privacyLevel int) bool {
	pbt.mu.Lock()
	defer pbt.mu.Unlock()
	
	// Check if budget needs refresh
	if time.Now().After(pbt.refreshTime) {
		pbt.remainingBudget = pbt.totalBudget
		pbt.refreshTime = time.Now().Add(pbt.budgetWindow)
		// Clear old allocations
		pbt.allocations = make(map[string]*BudgetAllocation)
	}
	
	// Check if sufficient budget remains
	if pbt.remainingBudget < cost {
		return false
	}
	
	// Allocate budget
	pbt.remainingBudget -= cost
	
	// Record allocation
	allocationKey := fmt.Sprintf("%s_%d", sessionID, time.Now().UnixNano())
	pbt.allocations[allocationKey] = &BudgetAllocation{
		QueryType:    queryType,
		Amount:       cost,
		Timestamp:    time.Now(),
		SessionID:    sessionID,
		PrivacyLevel: privacyLevel,
	}
	
	return true
}

// GetRemainingBudget returns the remaining privacy budget
func (pbt *PrivacyBudgetTracker) GetRemainingBudget() float64 {
	pbt.mu.RLock()
	defer pbt.mu.RUnlock()
	return pbt.remainingBudget
}

// GetBudgetUtilization returns budget utilization statistics
func (pbt *PrivacyBudgetTracker) GetBudgetUtilization() map[string]interface{} {
	pbt.mu.RLock()
	defer pbt.mu.RUnlock()
	
	totalAllocated := 0.0
	allocationsByType := make(map[SearchQueryType]float64)
	
	for _, allocation := range pbt.allocations {
		totalAllocated += allocation.Amount
		allocationsByType[allocation.QueryType] += allocation.Amount
	}
	
	return map[string]interface{}{
		"total_budget":      pbt.totalBudget,
		"remaining_budget":  pbt.remainingBudget,
		"utilized_budget":   totalAllocated,
		"utilization_rate":  totalAllocated / pbt.totalBudget,
		"allocations_count": len(pbt.allocations),
		"by_query_type":     allocationsByType,
		"refresh_time":      pbt.refreshTime,
	}
}
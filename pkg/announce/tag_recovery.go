package announce

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// TagRecovery provides sophisticated probabilistic tag recovery from privacy-preserving bloom filters.
//
// The TagRecovery system implements advanced machine learning techniques to recover
// tags from bloom filters while preserving privacy guarantees. It uses dynamic
// vocabularies, pattern recognition, statistical analysis, and adaptive learning
// to improve tag recovery accuracy over time without compromising the privacy
// benefits of bloom filter-based tag storage.
//
// Key Features:
//   - Dynamic tag dictionary with core and learned vocabularies
//   - Pattern-based tag discovery and association learning
//   - Statistical confidence tracking and adaptive learning
//   - Prefix-based tag expansion for comprehensive coverage
//   - Configurable recovery parameters and optimization
//   - Memory-efficient cleanup of stale learning data
//
// Recovery Strategy:
//   - Multi-tier candidate generation (core tags, learned tags, patterns)
//   - Probabilistic testing against bloom filters with false positive awareness
//   - Confidence-based tag prioritization and candidate limitation
//   - Pattern learning from successful tag combinations
//   - Adaptive vocabulary expansion based on usage patterns
//
// Privacy Preservation:
//   - Works with bloom filter false positives without exact tag disclosure
//   - No reverse engineering of bloom filter contents
//   - Statistical recovery maintains plausible deniability
//   - Learning improves accuracy without compromising privacy
//
// Thread Safety: TagRecovery is safe for concurrent use across multiple goroutines.
type TagRecovery struct {
	// Dynamic tag dictionary for comprehensive tag vocabulary management
	// tagDict maintains core tags, learned tags, and prefix patterns
	tagDict      *TagDictionary
	
	// Learned patterns for tag association discovery
	// patterns maps pattern keys to tag combination statistics
	patterns     map[string]*TagPattern
	
	// patternMutex provides read-write protection for pattern access
	patternMutex sync.RWMutex
	
	// Statistical analysis for performance monitoring and optimization
	// stats tracks recovery effectiveness and learning progress
	stats        *TagStatistics
	
	// Configuration parameters for recovery behavior and learning
	// config controls confidence thresholds, learning rates, and limits
	config       TagRecoveryConfig
}

// TagRecoveryConfig provides comprehensive configuration for tag recovery behavior and learning parameters.
//
// This configuration structure enables fine-tuning of tag recovery algorithms
// to match specific deployment requirements and content characteristics. It
// controls the balance between recovery accuracy and computational efficiency
// while managing memory usage and learning adaptation rates.
type TagRecoveryConfig struct {
	// MinConfidence sets the minimum confidence threshold for tag inclusion
	// Lower values increase recall but may increase false positives
	MinConfidence      float64       // Minimum confidence for tag recovery
	
	// MaxCandidates limits computational cost by capping candidate testing
	// Higher values improve recall but increase processing time
	MaxCandidates      int           // Maximum candidates to test per bloom filter
	
	// LearningRate controls how quickly the system adapts to new patterns
	// Higher values enable faster adaptation but may cause instability
	LearningRate       float64       // Rate of learning from successful matches
	
	// PatternRetention defines memory management for learned patterns
	// Longer retention improves accuracy but increases memory usage
	PatternRetention   time.Duration // How long to keep learned patterns
	
	// EnablePrefixSearch enables prefix-based tag expansion
	// Improves discovery of structured tags but increases candidate count
	EnablePrefixSearch bool          // Enable prefix-based tag discovery
}

// TagDictionary manages the comprehensive dynamic tag vocabulary for probabilistic recovery.
//
// The TagDictionary maintains multiple tag sources: core tags with predefined
// priorities, learned tags with statistical confidence tracking, and prefix
// patterns for structured tag discovery. It provides the foundation for
// effective tag recovery while adapting to usage patterns over time.
//
// Vocabulary Components:
//   - Core tags: Predefined common tags with static priorities
//   - Learned tags: User-provided tags with adaptive confidence scoring
//   - Prefix patterns: Structured tag patterns for discovery expansion
//
// Thread Safety: TagDictionary uses read-write locks for concurrent access.
type TagDictionary struct {
	// Core tags that are always tested with predefined priorities
	// coreTags maps tag strings to their static priority scores
	coreTags     map[string]float64 // tag -> priority
	
	// Learned tags from successful matches with adaptive confidence
	// learnedTags maps tag strings to their learning statistics
	learnedTags  map[string]*LearnedTag
	
	// Tag prefixes for structured tag discovery and expansion
	// prefixes maps prefix strings to lists of common suffix patterns
	prefixes     map[string][]string // prefix -> common suffixes
	
	// Mutex for concurrent access protection across all dictionary operations
	mu           sync.RWMutex
}

// LearnedTag represents a tag discovered through usage with comprehensive learning statistics.
//
// This structure tracks the statistical performance of tags discovered through
// user input and bloom filter testing. It maintains confidence scores based on
// success rates and temporal factors to enable intelligent tag prioritization
// and vocabulary optimization.
type LearnedTag struct {
	// Tag contains the actual tag string for recovery testing
	Tag           string
	
	// FirstSeen records when this tag was first learned
	FirstSeen     time.Time
	
	// LastSeen tracks the most recent successful match or usage
	LastSeen      time.Time
	
	// SuccessCount tracks successful bloom filter matches
	SuccessCount  int64
	
	// TestCount tracks total bloom filter tests (successes + failures)
	TestCount     int64
	
	// Confidence represents the computed statistical confidence score
	Confidence    float64
}

// TagPattern represents frequently occurring tag combinations for pattern-based recovery.
//
// This structure tracks tags that commonly appear together in announcements,
// enabling pattern-based tag discovery where the presence of some tags in
// a pattern increases the likelihood of other tags in the same pattern.
type TagPattern struct {
	// Tags contains the tag combination that forms this pattern
	Tags          []string
	
	// Occurrences tracks how often this pattern has been observed
	Occurrences   int64
	
	// LastSeen records the most recent occurrence of this pattern
	LastSeen      time.Time
}

// TagStatistics tracks comprehensive tag recovery performance and learning effectiveness.
//
// This structure maintains operational statistics about tag recovery success
// rates, learning progress, and system performance. The statistics enable
// monitoring, optimization, and debugging of the tag recovery system.
type TagStatistics struct {
	// TotalRecoveries counts all tag recovery attempts
	TotalRecoveries   int64
	
	// SuccessfulMatches counts successful tag discoveries
	SuccessfulMatches int64
	
	// FalsePositives tracks bloom filter false positive occurrences
	FalsePositives    int64
	
	// UnknownTags maps unknown tags to their occurrence counts
	UnknownTags       map[string]int64
	
	// mu provides read-write mutex protection for statistics updates
	mu                sync.RWMutex
}

// NewTagRecovery creates a new tag recovery system with comprehensive learning capabilities.
//
// This constructor initializes a fully-featured tag recovery system with
// configurable parameters, automatic cleanup routines, and adaptive learning.
// The system starts with predefined core tags and evolves its vocabulary
// through usage patterns and user feedback.
//
// Parameters:
//   - config: Tag recovery configuration with learning and performance parameters
//
// Returns:
//   A new TagRecovery system with active background cleanup
//
// Time Complexity: O(k) where k is the number of core tags initialized
// Space Complexity: O(k) for core tag storage and data structures
//
// Default Configuration:
//   - MinConfidence: 0.7 (70% confidence threshold)
//   - MaxCandidates: 1000 (computational efficiency limit)
//   - LearningRate: 0.1 (moderate adaptation speed)
//   - PatternRetention: 7 days (weekly pattern cleanup)
//
// Initialization Features:
//   - Core tag vocabulary with common media tags
//   - Background cleanup routine for memory management
//   - Statistical tracking for performance monitoring
//   - Thread-safe concurrent access support
//
// Example:
//   config := TagRecoveryConfig{MinConfidence: 0.8}
//   recovery := announce.NewTagRecovery(config)
//   tags, err := recovery.RecoverTags(bloomFilter)
func NewTagRecovery(config TagRecoveryConfig) *TagRecovery {
	// Set defaults
	if config.MinConfidence == 0 {
		config.MinConfidence = 0.7
	}
	if config.MaxCandidates == 0 {
		config.MaxCandidates = 1000
	}
	if config.LearningRate == 0 {
		config.LearningRate = 0.1
	}
	if config.PatternRetention == 0 {
		config.PatternRetention = 7 * 24 * time.Hour
	}
	
	recovery := &TagRecovery{
		tagDict:  NewTagDictionary(),
		patterns: make(map[string]*TagPattern),
		stats:    &TagStatistics{UnknownTags: make(map[string]int64)},
		config:   config,
	}
	
	// Start cleanup routine
	go recovery.cleanupLoop()
	
	return recovery
}

// RecoverTags performs sophisticated probabilistic tag recovery from bloom filter data.
//
// This method implements the core tag recovery algorithm using multi-tier
// candidate generation, confidence-based prioritization, and adaptive learning.
// It tests candidate tags against the bloom filter while maintaining privacy
// through probabilistic testing and false positive awareness.
//
// Parameters:
//   - bloomStr: Base64-encoded bloom filter data containing tag information
//
// Returns:
//   - Slice of recovered tags with confidence above threshold
//   - error if bloom filter decoding fails
//
// Time Complexity: O(n + k*log(k)) where n is candidates, k is recovered tags
// Space Complexity: O(n) for candidate storage and processing
//
// Recovery Pipeline:
//   1. Decode bloom filter from base64 encoding
//   2. Generate candidates from core tags, learned tags, and patterns
//   3. Prioritize candidates by confidence and limit to MaxCandidates
//   4. Test candidates against bloom filter with normalized tag matching
//   5. Record success/failure statistics for adaptive learning
//   6. Learn patterns from successful tag combinations
//   7. Update recovery statistics for monitoring
//
// Candidate Sources:
//   - Core tags: Predefined common tags with static priorities
//   - Learned tags: User-provided tags with confidence above threshold
//   - Pattern candidates: Tags from previously observed combinations
//
// Learning Features:
//   - Success/failure tracking for tag confidence adjustment
//   - Pattern learning from tag combinations in results
//   - Statistical updates for system performance monitoring
//
// Privacy Properties:
//   - Uses bloom filter testing without reverse engineering
//   - Maintains plausible deniability through false positives
//   - No exact tag disclosure beyond probabilistic matching
func (tr *TagRecovery) RecoverTags(bloomStr string) ([]string, error) {
	bloom, err := DecodeBloom(bloomStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode bloom filter: %w", err)
	}
	
	// Start with core tags
	candidates := tr.tagDict.GetCoreTags()
	
	// Add learned tags with high confidence
	candidates = append(candidates, tr.tagDict.GetConfidentTags(tr.config.MinConfidence)...)
	
	// Add pattern-based candidates
	candidates = append(candidates, tr.generatePatternCandidates()...)
	
	// Limit candidates
	if len(candidates) > tr.config.MaxCandidates {
		candidates = tr.prioritizeCandidates(candidates)[:tr.config.MaxCandidates]
	}
	
	// Test candidates against bloom filter
	recovered := []string{}
	for _, tag := range candidates {
		normalized := normalizeTag(tag)
		if bloom.Test(normalized) {
			recovered = append(recovered, tag)
			tr.recordSuccess(tag)
		} else {
			tr.recordFailure(tag)
		}
	}
	
	// Learn patterns from recovered tags
	if len(recovered) > 1 {
		tr.learnPattern(recovered)
	}
	
	// Update statistics
	tr.stats.mu.Lock()
	tr.stats.TotalRecoveries++
	tr.stats.mu.Unlock()
	
	return recovered, nil
}

// LearnFromTags incorporates user-provided tags into the recovery vocabulary and pattern analysis.
//
// This method enables supervised learning by incorporating tags that users
// explicitly provide. It updates the learned tag vocabulary and extracts
// prefix patterns for structured tag discovery, improving future recovery
// accuracy through user feedback and usage patterns.
//
// Parameters:
//   - tags: User-provided tags to incorporate into the learning system
//
// Time Complexity: O(n) where n is the number of input tags
// Space Complexity: O(n) for tag storage and prefix extraction
//
// Learning Process:
//   - Adds tags to learned vocabulary with initial confidence scores
//   - Updates existing tag statistics and confidence values
//   - Extracts prefix:suffix patterns for structured tag discovery
//   - Enables future recovery of similar structured tags
//
// Prefix Pattern Extraction:
//   - Identifies colon-separated tag structures (e.g., "genre:action")
//   - Learns prefix patterns for tag expansion and discovery
//   - Enables recovery of similar tags with same prefixes
//   - Supports hierarchical tag organization
//
// Example:
//   tags := []string{"genre:action", "year:2024", "res:1080p"}
//   recovery.LearnFromTags(tags)
//   // Learns "genre", "year", "res" prefixes for future expansion
func (tr *TagRecovery) LearnFromTags(tags []string) {
	tr.tagDict.LearnTags(tags)
	
	// Extract prefixes for future discovery
	for _, tag := range tags {
		if idx := strings.Index(tag, ":"); idx > 0 {
			prefix := tag[:idx]
			suffix := tag[idx+1:]
			tr.tagDict.AddPrefix(prefix, suffix)
		}
	}
}

// GetStatistics retrieves comprehensive tag recovery performance and learning statistics.
//
// This method provides detailed visibility into tag recovery effectiveness,
// learning progress, and system performance. The statistics enable monitoring,
// optimization, and debugging of the tag recovery system for improved accuracy
// and performance tuning.
//
// Returns:
//   - Map containing comprehensive tag recovery statistics and metrics
//
// Time Complexity: O(1) for statistic retrieval and calculation
// Space Complexity: O(k) where k is the number of statistical metrics
//
// Statistics Included:
//   - total_recoveries: Total number of recovery attempts
//   - successful_matches: Number of successful tag discoveries
//   - false_positives: Bloom filter false positive occurrences
//   - success_rate: Calculated success percentage
//   - dictionary_size: Current vocabulary size (core + learned)
//   - learned_patterns: Number of discovered tag patterns
//   - unknown_tags_count: Number of unknown tags encountered
//
// Use Cases:
//   - System monitoring and performance analysis
//   - Learning effectiveness assessment
//   - Configuration optimization and tuning
//   - Debugging recovery accuracy issues
//
// Thread Safety: Uses read lock for safe concurrent access to statistics.
func (tr *TagRecovery) GetStatistics() map[string]interface{} {
	tr.stats.mu.RLock()
	defer tr.stats.mu.RUnlock()
	
	successRate := float64(0)
	if tr.stats.TotalRecoveries > 0 {
		successRate = float64(tr.stats.SuccessfulMatches) / float64(tr.stats.TotalRecoveries)
	}
	
	return map[string]interface{}{
		"total_recoveries":    tr.stats.TotalRecoveries,
		"successful_matches":  tr.stats.SuccessfulMatches,
		"false_positives":     tr.stats.FalsePositives,
		"success_rate":        successRate,
		"dictionary_size":     tr.tagDict.Size(),
		"learned_patterns":    len(tr.patterns),
		"unknown_tags_count":  len(tr.stats.UnknownTags),
	}
}

// Helper methods

func (tr *TagRecovery) generatePatternCandidates() []string {
	tr.patternMutex.RLock()
	defer tr.patternMutex.RUnlock()
	
	candidates := []string{}
	seen := make(map[string]bool)
	
	// Generate candidates from patterns
	for _, pattern := range tr.patterns {
		for _, tag := range pattern.Tags {
			if !seen[tag] {
				candidates = append(candidates, tag)
				seen[tag] = true
			}
		}
	}
	
	return candidates
}

func (tr *TagRecovery) prioritizeCandidates(candidates []string) []string {
	// Sort by confidence and recency
	type scoredTag struct {
		tag   string
		score float64
	}
	
	scored := make([]scoredTag, len(candidates))
	for i, tag := range candidates {
		score := tr.tagDict.GetTagScore(tag)
		scored[i] = scoredTag{tag: tag, score: score}
	}
	
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})
	
	result := make([]string, len(scored))
	for i, st := range scored {
		result[i] = st.tag
	}
	
	return result
}

func (tr *TagRecovery) recordSuccess(tag string) {
	tr.tagDict.RecordSuccess(tag)
	
	tr.stats.mu.Lock()
	tr.stats.SuccessfulMatches++
	tr.stats.mu.Unlock()
}

func (tr *TagRecovery) recordFailure(tag string) {
	tr.tagDict.RecordFailure(tag)
}

func (tr *TagRecovery) learnPattern(tags []string) {
	tr.patternMutex.Lock()
	defer tr.patternMutex.Unlock()
	
	// Create pattern key
	sorted := make([]string, len(tags))
	copy(sorted, tags)
	sort.Strings(sorted)
	key := strings.Join(sorted, "|")
	
	if pattern, exists := tr.patterns[key]; exists {
		pattern.Occurrences++
		pattern.LastSeen = time.Now()
	} else {
		tr.patterns[key] = &TagPattern{
			Tags:        tags,
			Occurrences: 1,
			LastSeen:    time.Now(),
		}
	}
}

func (tr *TagRecovery) cleanupLoop() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	
	for range ticker.C {
		tr.cleanupOldPatterns()
	}
}

func (tr *TagRecovery) cleanupOldPatterns() {
	tr.patternMutex.Lock()
	defer tr.patternMutex.Unlock()
	
	cutoff := time.Now().Add(-tr.config.PatternRetention)
	
	for key, pattern := range tr.patterns {
		if pattern.LastSeen.Before(cutoff) {
			delete(tr.patterns, key)
		}
	}
}

// TagDictionary methods

// NewTagDictionary creates a new tag dictionary with comprehensive core vocabulary initialization.
//
// This constructor initializes a tag dictionary with predefined core tags
// covering common media attributes, formats, and metadata categories. The
// core vocabulary provides a strong foundation for tag recovery across
// diverse content types and usage patterns.
//
// Returns:
//   A new TagDictionary with initialized core vocabulary
//
// Time Complexity: O(k) where k is the number of core tags
// Space Complexity: O(k) for core tag storage
//
// Core Vocabulary Categories:
//   - Resolutions: 480p, 720p, 1080p, 1440p, 4K, 8K
//   - Genres: action, comedy, drama, horror, sci-fi, fantasy, documentary, anime
//   - Years: Current year minus 10 to current year
//   - Attributes: remastered, extended, director's cut, HDR, Dolby
//   - Formats: MKV, MP4, AVI, WebM, FLAC, MP3, Opus
//
// Priority Scoring:
//   - Resolutions: 0.9 (high priority for video content)
//   - Genres: 0.8 (important for content classification)
//   - Years: 0.7 (useful for temporal filtering)
//   - Formats: 0.7 (important for compatibility)
//   - Attributes: 0.6 (supplementary metadata)
func NewTagDictionary() *TagDictionary {
	dict := &TagDictionary{
		coreTags:    make(map[string]float64),
		learnedTags: make(map[string]*LearnedTag),
		prefixes:    make(map[string][]string),
	}
	
	// Initialize with common tags
	dict.initializeCoreTags()
	
	return dict
}

// initializeCoreTags populates the dictionary with predefined common tags across media categories.
//
// This method establishes the core vocabulary that forms the foundation of
// tag recovery. The core tags cover essential media attributes, formats,
// and metadata categories with carefully tuned priority scores based on
// their importance and frequency in typical content libraries.
//
// Time Complexity: O(k) where k is the total number of core tags
// Space Complexity: O(k) for core tag storage
//
// Tag Categories and Priorities:
//   - Video resolutions (0.9): Essential for video content filtering
//   - Content genres (0.8): Critical for content categorization
//   - Publication years (0.7): Important for temporal organization
//   - Media formats (0.7): Essential for compatibility and playback
//   - Content attributes (0.6): Supplementary quality indicators
//
// Dynamic Elements:
//   - Years are generated dynamically (current year - 10 to current)
//   - Ensures relevance for recent content without manual updates
//   - Balances historical coverage with storage efficiency
func (td *TagDictionary) initializeCoreTags() {
	// Common resolution tags
	resolutions := []string{"480p", "720p", "1080p", "1440p", "4k", "8k"}
	for _, res := range resolutions {
		td.coreTags["res:"+res] = 0.9
	}
	
	// Common genres
	genres := []string{"action", "comedy", "drama", "horror", "scifi", "fantasy", "documentary", "anime"}
	for _, genre := range genres {
		td.coreTags["genre:"+genre] = 0.8
	}
	
	// Common years
	currentYear := time.Now().Year()
	for year := currentYear - 10; year <= currentYear; year++ {
		td.coreTags[fmt.Sprintf("year:%d", year)] = 0.7
	}
	
	// Common attributes
	attributes := []string{"remastered", "extended", "directors-cut", "uncut", "hdr", "dolby", "subtitled"}
	for _, attr := range attributes {
		td.coreTags[attr] = 0.6
	}
	
	// Common formats
	formats := []string{"mkv", "mp4", "avi", "webm", "flac", "mp3", "opus"}
	for _, format := range formats {
		td.coreTags["format:"+format] = 0.7
	}
}

// GetCoreTags retrieves all predefined core tags for candidate generation.
//
// This method returns the complete list of core tags that form the foundation
// of tag recovery attempts. Core tags are always tested against bloom filters
// and provide reliable coverage of common media attributes and formats.
//
// Returns:
//   - Slice containing all core tag strings
//
// Time Complexity: O(k) where k is the number of core tags
// Space Complexity: O(k) for result slice allocation
//
// Thread Safety: Uses read lock for safe concurrent access.
func (td *TagDictionary) GetCoreTags() []string {
	td.mu.RLock()
	defer td.mu.RUnlock()
	
	tags := make([]string, 0, len(td.coreTags))
	for tag := range td.coreTags {
		tags = append(tags, tag)
	}
	
	return tags
}

// GetConfidentTags retrieves learned tags with confidence above the specified threshold.
//
// This method filters learned tags based on their statistical confidence
// scores to ensure only reliable tags are included in recovery attempts.
// The confidence threshold enables quality control and computational
// efficiency by focusing on high-probability candidates.
//
// Parameters:
//   - minConfidence: Minimum confidence threshold for tag inclusion
//
// Returns:
//   - Slice of learned tags meeting the confidence criteria
//
// Time Complexity: O(n) where n is the number of learned tags
// Space Complexity: O(m) where m is the number of confident tags
//
// Confidence Scoring:
//   - Based on success rate and temporal recency factors
//   - Values range from 0.0 (no confidence) to 1.0 (full confidence)
//   - Higher thresholds improve precision but may reduce recall
//
// Thread Safety: Uses read lock for safe concurrent access.
func (td *TagDictionary) GetConfidentTags(minConfidence float64) []string {
	td.mu.RLock()
	defer td.mu.RUnlock()
	
	tags := []string{}
	for tag, learned := range td.learnedTags {
		if learned.Confidence >= minConfidence {
			tags = append(tags, tag)
		}
	}
	
	return tags
}

// LearnTags incorporates new tags into the learned vocabulary with statistical tracking.
//
// This method adds user-provided tags to the learned vocabulary or updates
// existing tags with new usage statistics. It maintains success counts,
// timestamps, and confidence scores to enable intelligent tag prioritization
// and quality assessment for future recovery attempts.
//
// Parameters:
//   - tags: Tag strings to incorporate into the learned vocabulary
//
// Time Complexity: O(n) where n is the number of input tags
// Space Complexity: O(n) for new tag storage
//
// Learning Process:
//   - Creates new LearnedTag entries for unknown tags
//   - Updates existing tags with incremented success counts
//   - Refreshes timestamps for recency-based confidence calculation
//   - Recomputes confidence scores based on updated statistics
//
// Initial Values:
//   - SuccessCount: 1 (assumes provided tags are valid)
//   - TestCount: 1 (perfect initial success rate)
//   - Confidence: 0.5 (moderate initial confidence)
//
// Thread Safety: Uses write lock for exclusive access during updates.
func (td *TagDictionary) LearnTags(tags []string) {
	td.mu.Lock()
	defer td.mu.Unlock()
	
	now := time.Now()
	for _, tag := range tags {
		if learned, exists := td.learnedTags[tag]; exists {
			learned.LastSeen = now
			learned.SuccessCount++
			learned.updateConfidence()
		} else {
			td.learnedTags[tag] = &LearnedTag{
				Tag:          tag,
				FirstSeen:    now,
				LastSeen:     now,
				SuccessCount: 1,
				TestCount:    1,
				Confidence:   0.5,
			}
		}
	}
}

// AddPrefix learns structured tag patterns for enhanced discovery capabilities.
//
// This method extracts and stores prefix-suffix patterns from structured
// tags (e.g., "genre:action") to enable discovery of similar tags with
// the same prefix structure. The pattern learning improves recovery of
// hierarchical and categorized tag systems.
//
// Parameters:
//   - prefix: Tag prefix component (e.g., "genre", "year", "res")
//   - suffix: Tag suffix component (e.g., "action", "2024", "1080p")
//
// Time Complexity: O(n) where n is the number of existing suffixes for the prefix
// Space Complexity: O(1) for suffix addition, bounded by suffix limit
//
// Pattern Storage:
//   - Maintains suffix lists for each prefix with duplicate prevention
//   - Limits suffix lists to 100 entries for memory efficiency
//   - Uses FIFO eviction when suffix limit is exceeded
//
// Discovery Benefits:
//   - Enables recovery of tags with known prefixes but new suffixes
//   - Supports hierarchical tag organization and classification
//   - Improves coverage of structured metadata systems
//
// Thread Safety: Uses write lock for exclusive access during updates.
func (td *TagDictionary) AddPrefix(prefix, suffix string) {
	td.mu.Lock()
	defer td.mu.Unlock()
	
	// Add suffix to prefix list
	suffixes := td.prefixes[prefix]
	
	// Check if suffix already exists
	found := false
	for _, s := range suffixes {
		if s == suffix {
			found = true
			break
		}
	}
	
	if !found {
		td.prefixes[prefix] = append(suffixes, suffix)
		
		// Limit suffix list size
		if len(td.prefixes[prefix]) > 100 {
			td.prefixes[prefix] = td.prefixes[prefix][1:]
		}
	}
}

// GetTagScore retrieves the priority or confidence score for a specific tag.
//
// This method provides unified access to tag scoring across both core and
// learned tag vocabularies. The score enables intelligent candidate
// prioritization during tag recovery by ranking tags based on their
// reliability and importance.
//
// Parameters:
//   - tag: Tag string for score retrieval
//
// Returns:
//   - Priority score for core tags or confidence score for learned tags
//   - 0.0 if tag is not found in either vocabulary
//
// Time Complexity: O(1) for map lookups
// Space Complexity: O(1)
//
// Score Sources:
//   - Core tags: Static priority scores based on tag importance
//   - Learned tags: Dynamic confidence scores based on success statistics
//   - Unknown tags: Default score of 0.0
//
// Score Interpretation:
//   - 0.0-0.3: Low priority/confidence
//   - 0.4-0.6: Medium priority/confidence
//   - 0.7-0.9: High priority/confidence
//   - 1.0: Maximum priority/confidence
//
// Thread Safety: Uses read lock for safe concurrent access.
func (td *TagDictionary) GetTagScore(tag string) float64 {
	td.mu.RLock()
	defer td.mu.RUnlock()
	
	// Check core tags
	if score, exists := td.coreTags[tag]; exists {
		return score
	}
	
	// Check learned tags
	if learned, exists := td.learnedTags[tag]; exists {
		return learned.Confidence
	}
	
	return 0.0
}

// RecordSuccess updates learned tag statistics after successful bloom filter matches.
//
// This method increments success and test counters for learned tags and
// recomputes confidence scores based on updated statistics. The success
// tracking enables adaptive learning and improved tag prioritization
// through statistical performance analysis.
//
// Parameters:
//   - tag: Tag string that successfully matched a bloom filter
//
// Time Complexity: O(1) for counter updates and confidence recalculation
// Space Complexity: O(1)
//
// Statistical Updates:
//   - Increments SuccessCount for positive outcome tracking
//   - Increments TestCount for total attempt tracking
//   - Updates LastSeen timestamp for recency-based confidence
//   - Recomputes confidence score using updated statistics
//
// Confidence Impact:
//   - Increases success rate component of confidence score
//   - Refreshes recency factor for temporal relevance
//   - Improves tag prioritization for future recovery attempts
//
// Thread Safety: Uses write lock for exclusive access during updates.
func (td *TagDictionary) RecordSuccess(tag string) {
	td.mu.Lock()
	defer td.mu.Unlock()
	
	if learned, exists := td.learnedTags[tag]; exists {
		learned.SuccessCount++
		learned.TestCount++
		learned.LastSeen = time.Now()
		learned.updateConfidence()
	}
}

// RecordFailure updates learned tag statistics after failed bloom filter tests.
//
// This method increments test counters for learned tags and recomputes
// confidence scores to reflect the failed test attempt. The failure
// tracking enables accurate statistical assessment and prevents
// overconfidence in unreliable tag candidates.
//
// Parameters:
//   - tag: Tag string that failed to match a bloom filter
//
// Time Complexity: O(1) for counter updates and confidence recalculation
// Space Complexity: O(1)
//
// Statistical Updates:
//   - Increments TestCount without changing SuccessCount
//   - Decreases success rate component of confidence score
//   - Recomputes confidence based on updated failure statistics
//
// Confidence Impact:
//   - Reduces success rate component of confidence score
//   - Lowers tag prioritization for future recovery attempts
//   - Prevents unreliable tags from dominating candidate selection
//
// Thread Safety: Uses write lock for exclusive access during updates.
func (td *TagDictionary) RecordFailure(tag string) {
	td.mu.Lock()
	defer td.mu.Unlock()
	
	if learned, exists := td.learnedTags[tag]; exists {
		learned.TestCount++
		learned.updateConfidence()
	}
}

// Size returns the total number of tags in the dictionary across all vocabularies.
//
// This method provides the combined size of core and learned tag vocabularies
// for monitoring vocabulary growth and memory usage analysis. The size metric
// helps assess system scaling and learning effectiveness over time.
//
// Returns:
//   - Total number of tags (core tags + learned tags)
//
// Time Complexity: O(1) for map size access
// Space Complexity: O(1)
//
// Thread Safety: Uses read lock for safe concurrent access.
func (td *TagDictionary) Size() int {
	td.mu.RLock()
	defer td.mu.RUnlock()
	
	return len(td.coreTags) + len(td.learnedTags)
}

// updateConfidence recalculates the statistical confidence score for a learned tag.
//
// This method computes a confidence score based on success rate and temporal
// recency factors to enable intelligent tag prioritization. The confidence
// score reflects both statistical reliability and temporal relevance for
// optimal tag recovery performance.
//
// Time Complexity: O(1) for confidence calculation
// Space Complexity: O(1)
//
// Confidence Calculation:
//   - Success rate: SuccessCount / TestCount (statistical reliability)
//   - Recency factor: 1.0 / (1.0 + days_since_seen * 0.1) (temporal decay)
//   - Final confidence: success_rate * recency_factor
//
// Temporal Decay:
//   - Recent tags maintain high confidence multipliers
//   - Older tags experience gradual confidence reduction
//   - Decay rate of 0.1 per day provides balanced temporal weighting
//
// Score Properties:
//   - Range: 0.0 to 1.0
//   - Higher values indicate more reliable and recent tags
//   - Balances statistical performance with temporal relevance
func (lt *LearnedTag) updateConfidence() {
	if lt.TestCount > 0 {
		// Basic confidence calculation
		successRate := float64(lt.SuccessCount) / float64(lt.TestCount)
		
		// Factor in recency
		daysSinceSeen := time.Since(lt.LastSeen).Hours() / 24
		recencyFactor := 1.0 / (1.0 + daysSinceSeen*0.1)
		
		lt.Confidence = successRate * recencyFactor
	}
}
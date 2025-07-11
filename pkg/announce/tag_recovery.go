package announce

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// TagRecovery provides probabilistic tag recovery from bloom filters
type TagRecovery struct {
	// Dynamic tag dictionary
	tagDict      *TagDictionary
	
	// Learned patterns
	patterns     map[string]*TagPattern
	patternMutex sync.RWMutex
	
	// Statistical analysis
	stats        *TagStatistics
	
	// Configuration
	config       TagRecoveryConfig
}

// TagRecoveryConfig holds configuration for tag recovery
type TagRecoveryConfig struct {
	MinConfidence      float64       // Minimum confidence for tag recovery
	MaxCandidates      int           // Maximum candidates to test per bloom filter
	LearningRate       float64       // Rate of learning from successful matches
	PatternRetention   time.Duration // How long to keep learned patterns
	EnablePrefixSearch bool          // Enable prefix-based tag discovery
}

// TagDictionary manages the dynamic tag vocabulary
type TagDictionary struct {
	// Core tags that are always tested
	coreTags     map[string]float64 // tag -> priority
	
	// Learned tags from successful matches
	learnedTags  map[string]*LearnedTag
	
	// Tag prefixes for discovery
	prefixes     map[string][]string // prefix -> common suffixes
	
	// Mutex for concurrent access
	mu           sync.RWMutex
}

// LearnedTag represents a tag discovered through usage
type LearnedTag struct {
	Tag           string
	FirstSeen     time.Time
	LastSeen      time.Time
	SuccessCount  int64
	TestCount     int64
	Confidence    float64
}

// TagPattern represents common tag combinations
type TagPattern struct {
	Tags          []string
	Occurrences   int64
	LastSeen      time.Time
}

// TagStatistics tracks tag recovery performance
type TagStatistics struct {
	TotalRecoveries   int64
	SuccessfulMatches int64
	FalsePositives    int64
	UnknownTags       map[string]int64
	mu                sync.RWMutex
}

// NewTagRecovery creates a new tag recovery system
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

// RecoverTags attempts to recover tags from a bloom filter
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

// LearnFromTags adds tags to the dictionary from user input
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

// GetStatistics returns tag recovery statistics
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

func (td *TagDictionary) GetCoreTags() []string {
	td.mu.RLock()
	defer td.mu.RUnlock()
	
	tags := make([]string, 0, len(td.coreTags))
	for tag := range td.coreTags {
		tags = append(tags, tag)
	}
	
	return tags
}

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

func (td *TagDictionary) RecordFailure(tag string) {
	td.mu.Lock()
	defer td.mu.Unlock()
	
	if learned, exists := td.learnedTags[tag]; exists {
		learned.TestCount++
		learned.updateConfidence()
	}
}

func (td *TagDictionary) Size() int {
	td.mu.RLock()
	defer td.mu.RUnlock()
	
	return len(td.coreTags) + len(td.learnedTags)
}

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
package announce

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

// SpamDetector detects and filters spam announcements
type SpamDetector struct {
	// Configuration
	duplicateWindow    time.Duration
	similarityWindow   time.Duration
	maxDuplicates      int
	suspiciousPatterns []string
	
	// Tracking
	recentHashes map[string]*hashRecord
	descriptors  map[string]*descriptorRecord
	mu           sync.RWMutex
	
	// Cleanup
	stopCleanup chan struct{}
	wg          sync.WaitGroup
}

// hashRecord tracks announcement hashes
type hashRecord struct {
	count     int
	firstSeen time.Time
	lastSeen  time.Time
}

// descriptorRecord tracks descriptor usage
type descriptorRecord struct {
	topics    map[string]int
	count     int
	firstSeen time.Time
	lastSeen  time.Time
}

// SpamConfig holds spam detector configuration
type SpamConfig struct {
	DuplicateWindow    time.Duration
	SimilarityWindow   time.Duration
	MaxDuplicates      int
	SuspiciousPatterns []string
	CleanupInterval    time.Duration
}

// DefaultSpamConfig returns default spam detection configuration
func DefaultSpamConfig() *SpamConfig {
	return &SpamConfig{
		DuplicateWindow:  1 * time.Hour,
		SimilarityWindow: 24 * time.Hour,
		MaxDuplicates:    3,
		SuspiciousPatterns: []string{
			"test", "spam", "xxx", "porn",
			"click here", "free money", "winner",
		},
		CleanupInterval: 1 * time.Hour,
	}
}

// NewSpamDetector creates a new spam detector
func NewSpamDetector(config *SpamConfig) *SpamDetector {
	if config == nil {
		config = DefaultSpamConfig()
	}
	
	sd := &SpamDetector{
		duplicateWindow:    config.DuplicateWindow,
		similarityWindow:   config.SimilarityWindow,
		maxDuplicates:      config.MaxDuplicates,
		suspiciousPatterns: config.SuspiciousPatterns,
		recentHashes:       make(map[string]*hashRecord),
		descriptors:        make(map[string]*descriptorRecord),
		stopCleanup:        make(chan struct{}),
	}
	
	// Start cleanup routine
	sd.wg.Add(1)
	go sd.cleanupLoop(config.CleanupInterval)
	
	return sd
}

// CheckSpam checks if an announcement is spam
func (sd *SpamDetector) CheckSpam(ann *Announcement) (bool, string) {
	// Generate content hash
	contentHash := sd.generateContentHash(ann)
	
	// Check for duplicates
	if isDupe, reason := sd.checkDuplicate(contentHash); isDupe {
		return true, reason
	}
	
	// Check for descriptor spam
	if isSpam, reason := sd.checkDescriptorSpam(ann); isSpam {
		return true, reason
	}
	
	// Check for suspicious patterns
	if isSpam, reason := sd.checkSuspiciousPatterns(ann); isSpam {
		return true, reason
	}
	
	// Check for anomalies
	if isSpam, reason := sd.checkAnomalies(ann); isSpam {
		return true, reason
	}
	
	// Record this announcement
	sd.recordAnnouncement(ann, contentHash)
	
	return false, ""
}

// generateContentHash creates a hash of announcement content
func (sd *SpamDetector) generateContentHash(ann *Announcement) string {
	// Hash key fields to detect duplicates
	h := sha256.New()
	h.Write([]byte(ann.Descriptor))
	h.Write([]byte(ann.TopicHash))
	h.Write([]byte(ann.Category))
	h.Write([]byte(ann.SizeClass))
	h.Write([]byte(ann.TagBloom))
	return hex.EncodeToString(h.Sum(nil))
}

// checkDuplicate checks for duplicate announcements
func (sd *SpamDetector) checkDuplicate(contentHash string) (bool, string) {
	sd.mu.RLock()
	defer sd.mu.RUnlock()
	
	record, exists := sd.recentHashes[contentHash]
	if !exists {
		return false, ""
	}
	
	now := time.Now()
	
	// Check if within duplicate window
	if now.Sub(record.firstSeen) <= sd.duplicateWindow {
		if record.count >= sd.maxDuplicates {
			return true, fmt.Sprintf("duplicate announcement (seen %d times)", record.count)
		}
	}
	
	return false, ""
}

// checkDescriptorSpam checks for descriptor-based spam
func (sd *SpamDetector) checkDescriptorSpam(ann *Announcement) (bool, string) {
	sd.mu.RLock()
	defer sd.mu.RUnlock()
	
	record, exists := sd.descriptors[ann.Descriptor]
	if !exists {
		return false, ""
	}
	
	// Check if same descriptor used across many topics
	if len(record.topics) > 10 {
		return true, "descriptor used across too many topics"
	}
	
	// Check rapid reannouncement
	if time.Since(record.lastSeen) < 5*time.Minute && record.count > 5 {
		return true, "rapid reannouncement of same descriptor"
	}
	
	return false, ""
}

// checkSuspiciousPatterns checks for known spam patterns
func (sd *SpamDetector) checkSuspiciousPatterns(ann *Announcement) (bool, string) {
	// Check tag bloom for suspicious patterns
	if ann.TagBloom != "" {
		// Decode bloom filter and check tags
		bloom, err := DecodeBloom(ann.TagBloom)
		if err == nil {
			for _, pattern := range sd.suspiciousPatterns {
				if bloom.Test(strings.ToLower(pattern)) {
					return true, fmt.Sprintf("suspicious pattern detected: %s", pattern)
				}
			}
		}
	}
	
	return false, ""
}

// checkAnomalies checks for anomalous behavior
func (sd *SpamDetector) checkAnomalies(ann *Announcement) (bool, string) {
	// Check for future timestamps
	if ann.Timestamp > time.Now().Unix()+300 { // More than 5 min in future
		return true, "announcement timestamp too far in future"
	}
	
	// Check for abnormally long TTL
	if ann.TTL > 7*24*3600 { // More than 1 week
		return true, "abnormally long TTL"
	}
	
	// Check for empty or invalid fields
	if ann.Descriptor == "" || ann.TopicHash == "" {
		return true, "missing required fields"
	}
	
	// Check for size/category mismatch
	if ann.Category == "document" && ann.SizeClass == "huge" {
		// Documents are rarely huge
		return true, "suspicious size/category combination"
	}
	
	return false, ""
}

// recordAnnouncement records an announcement for future spam detection
func (sd *SpamDetector) recordAnnouncement(ann *Announcement, contentHash string) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	
	now := time.Now()
	
	// Update hash record
	hRecord, exists := sd.recentHashes[contentHash]
	if !exists {
		hRecord = &hashRecord{
			count:     0,
			firstSeen: now,
		}
		sd.recentHashes[contentHash] = hRecord
	}
	hRecord.count++
	hRecord.lastSeen = now
	
	// Update descriptor record
	descRecord, exists := sd.descriptors[ann.Descriptor]
	if !exists {
		descRecord = &descriptorRecord{
			topics:    make(map[string]int),
			count:     0,
			firstSeen: now,
		}
		sd.descriptors[ann.Descriptor] = descRecord
	}
	descRecord.topics[ann.TopicHash]++
	descRecord.count++
	descRecord.lastSeen = now
}

// GetStats returns spam detector statistics
func (sd *SpamDetector) GetStats() SpamStats {
	sd.mu.RLock()
	defer sd.mu.RUnlock()
	
	return SpamStats{
		UniqueHashes:      len(sd.recentHashes),
		UniqueDescriptors: len(sd.descriptors),
		TotalAnnouncements: sd.countTotalAnnouncements(),
	}
}

// countTotalAnnouncements counts total announcements seen
func (sd *SpamDetector) countTotalAnnouncements() int {
	total := 0
	for _, record := range sd.recentHashes {
		total += record.count
	}
	return total
}

// Close stops the spam detector
func (sd *SpamDetector) Close() {
	close(sd.stopCleanup)
	sd.wg.Wait()
}

// cleanupLoop periodically removes old records
func (sd *SpamDetector) cleanupLoop(interval time.Duration) {
	defer sd.wg.Done()
	
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	
	for {
		select {
		case <-sd.stopCleanup:
			return
		case <-ticker.C:
			sd.cleanup()
		}
	}
}

// cleanup removes old records
func (sd *SpamDetector) cleanup() {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	
	now := time.Now()
	
	// Clean hash records
	for hash, record := range sd.recentHashes {
		if now.Sub(record.lastSeen) > sd.similarityWindow {
			delete(sd.recentHashes, hash)
		}
	}
	
	// Clean descriptor records
	for desc, record := range sd.descriptors {
		if now.Sub(record.lastSeen) > sd.similarityWindow {
			delete(sd.descriptors, desc)
		}
	}
}

// SpamStats holds spam detection statistics
type SpamStats struct {
	UniqueHashes       int
	UniqueDescriptors  int
	TotalAnnouncements int
}

// SpamScore calculates a spam probability score (0-100)
func (sd *SpamDetector) SpamScore(ann *Announcement) int {
	score := 0
	
	sd.mu.RLock()
	defer sd.mu.RUnlock()
	
	// Check duplicate count
	contentHash := sd.generateContentHash(ann)
	if record, exists := sd.recentHashes[contentHash]; exists {
		if record.count > 1 {
			score += record.count * 10 // Each duplicate adds 10 points
		}
	}
	
	// Check descriptor usage
	if record, exists := sd.descriptors[ann.Descriptor]; exists {
		if len(record.topics) > 5 {
			score += len(record.topics) * 5 // Each extra topic adds 5 points
		}
		if record.count > 10 {
			score += 20 // High usage adds 20 points
		}
	}
	
	// Check for suspicious patterns
	for _, pattern := range sd.suspiciousPatterns {
		if ann.TagBloom != "" {
			bloom, err := DecodeBloom(ann.TagBloom)
			if err == nil && bloom.Test(strings.ToLower(pattern)) {
				score += 30 // Suspicious pattern adds 30 points
			}
		}
	}
	
	// Cap at 100
	if score > 100 {
		score = 100
	}
	
	return score
}
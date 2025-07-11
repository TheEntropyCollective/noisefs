package announce

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// ValidationConfig holds configuration for announcement validation
type ValidationConfig struct {
	MaxDescriptorLength int           // Maximum descriptor CID length
	MaxTopicLength      int           // Maximum topic string length
	MaxTagCount         int           // Maximum number of tags
	MaxTTL              time.Duration // Maximum time-to-live
	MinTTL              time.Duration // Minimum time-to-live
	MaxFutureTime       time.Duration // Maximum timestamp in future
	RequiredFields      []string      // Required announcement fields
}

// DefaultValidationConfig returns default validation configuration
func DefaultValidationConfig() *ValidationConfig {
	return &ValidationConfig{
		MaxDescriptorLength: 100,        // CIDs are typically ~59 chars
		MaxTopicLength:      256,        // Reasonable topic length
		MaxTagCount:         50,         // Prevent tag spam
		MaxTTL:              7 * 24 * time.Hour,  // 1 week max
		MinTTL:              1 * time.Hour,       // 1 hour min
		MaxFutureTime:       5 * time.Minute,     // Allow 5 min clock skew
		RequiredFields:      []string{"v", "d", "t", "ts", "ttl"},
	}
}

// Validator validates announcements
type Validator struct {
	config *ValidationConfig
}

// NewValidator creates a new announcement validator
func NewValidator(config *ValidationConfig) *Validator {
	if config == nil {
		config = DefaultValidationConfig()
	}
	return &Validator{config: config}
}

// ValidateAnnouncement performs comprehensive validation
func (v *Validator) ValidateAnnouncement(ann *Announcement) error {
	// Check version
	if ann.Version == "" {
		return fmt.Errorf("missing version")
	}
	if ann.Version != "1.0" {
		return fmt.Errorf("unsupported version: %s", ann.Version)
	}
	
	// Validate descriptor
	if err := v.validateDescriptor(ann.Descriptor); err != nil {
		return fmt.Errorf("invalid descriptor: %w", err)
	}
	
	// Validate topic hash
	if err := v.validateTopicHash(ann.TopicHash); err != nil {
		return fmt.Errorf("invalid topic hash: %w", err)
	}
	
	// Validate timestamp
	if err := v.validateTimestamp(ann.Timestamp); err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}
	
	// Validate TTL
	if err := v.validateTTL(ann.TTL); err != nil {
		return fmt.Errorf("invalid TTL: %w", err)
	}
	
	// Validate category
	if ann.Category != "" {
		if err := v.validateCategory(ann.Category); err != nil {
			return fmt.Errorf("invalid category: %w", err)
		}
	}
	
	// Validate size class
	if ann.SizeClass != "" {
		if err := v.validateSizeClass(ann.SizeClass); err != nil {
			return fmt.Errorf("invalid size class: %w", err)
		}
	}
	
	// Validate bloom filter if present
	if ann.TagBloom != "" {
		if err := v.validateBloomFilter(ann.TagBloom); err != nil {
			return fmt.Errorf("invalid tag bloom filter: %w", err)
		}
	}
	
	// Validate nonce
	if ann.Nonce == "" {
		return fmt.Errorf("missing nonce")
	}
	if len(ann.Nonce) < 8 || len(ann.Nonce) > 32 {
		return fmt.Errorf("nonce length must be 8-32 characters")
	}
	
	return nil
}

// validateDescriptor validates a descriptor CID
func (v *Validator) validateDescriptor(descriptor string) error {
	if descriptor == "" {
		return fmt.Errorf("empty descriptor")
	}
	
	if len(descriptor) > v.config.MaxDescriptorLength {
		return fmt.Errorf("descriptor too long: %d > %d", len(descriptor), v.config.MaxDescriptorLength)
	}
	
	// Basic CID validation (should start with Qm or bafy)
	if !strings.HasPrefix(descriptor, "Qm") && !strings.HasPrefix(descriptor, "bafy") {
		return fmt.Errorf("invalid CID format")
	}
	
	// Check for valid base58/base32 characters
	if strings.HasPrefix(descriptor, "Qm") {
		// Base58 validation
		if !isValidBase58(descriptor) {
			return fmt.Errorf("invalid base58 encoding")
		}
	}
	
	return nil
}

// validateTopicHash validates a topic hash
func (v *Validator) validateTopicHash(topicHash string) error {
	if topicHash == "" {
		return fmt.Errorf("empty topic hash")
	}
	
	// Should be a hex-encoded SHA-256 hash (64 chars)
	if len(topicHash) != 64 {
		return fmt.Errorf("invalid hash length: expected 64, got %d", len(topicHash))
	}
	
	// Validate hex encoding
	if _, err := hex.DecodeString(topicHash); err != nil {
		return fmt.Errorf("invalid hex encoding: %w", err)
	}
	
	return nil
}

// validateTimestamp validates announcement timestamp
func (v *Validator) validateTimestamp(timestamp int64) error {
	if timestamp <= 0 {
		return fmt.Errorf("invalid timestamp: %d", timestamp)
	}
	
	now := time.Now().Unix()
	
	// Check if too far in the past (older than 1 year)
	if timestamp < now - 365*24*3600 {
		return fmt.Errorf("timestamp too old")
	}
	
	// Check if too far in the future
	maxFuture := now + int64(v.config.MaxFutureTime.Seconds())
	if timestamp > maxFuture {
		return fmt.Errorf("timestamp too far in future")
	}
	
	return nil
}

// validateTTL validates time-to-live
func (v *Validator) validateTTL(ttl int64) error {
	if ttl <= 0 {
		return fmt.Errorf("TTL must be positive")
	}
	
	ttlDuration := time.Duration(ttl) * time.Second
	
	if ttlDuration < v.config.MinTTL {
		return fmt.Errorf("TTL too short: %s < %s", ttlDuration, v.config.MinTTL)
	}
	
	if ttlDuration > v.config.MaxTTL {
		return fmt.Errorf("TTL too long: %s > %s", ttlDuration, v.config.MaxTTL)
	}
	
	return nil
}

// validateCategory validates content category
func (v *Validator) validateCategory(category string) error {
	validCategories := map[string]bool{
		"video":    true,
		"audio":    true,
		"document": true,
		"image":    true,
		"archive":  true,
		"other":    true,
	}
	
	if !validCategories[category] {
		return fmt.Errorf("unknown category: %s", category)
	}
	
	return nil
}

// validateSizeClass validates size classification
func (v *Validator) validateSizeClass(sizeClass string) error {
	validSizes := map[string]bool{
		"tiny":   true, // < 1MB
		"small":  true, // 1-10MB
		"medium": true, // 10-100MB
		"large":  true, // 100MB-1GB
		"huge":   true, // > 1GB
	}
	
	if !validSizes[sizeClass] {
		return fmt.Errorf("unknown size class: %s", sizeClass)
	}
	
	return nil
}

// validateBloomFilter validates bloom filter encoding
func (v *Validator) validateBloomFilter(bloomStr string) error {
	if bloomStr == "" {
		return nil // Bloom filter is optional
	}
	
	// Should be base64 encoded
	if len(bloomStr) < 4 {
		return fmt.Errorf("bloom filter too short")
	}
	
	// Try to decode
	_, err := DecodeBloom(bloomStr)
	if err != nil {
		return fmt.Errorf("failed to decode bloom filter: %w", err)
	}
	
	return nil
}

// isValidBase58 checks if a string contains only valid base58 characters
func isValidBase58(s string) bool {
	const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	for _, char := range s {
		if !strings.ContainsRune(base58Alphabet, char) {
			return false
		}
	}
	return true
}

// ValidateJSON validates announcement JSON before parsing
func ValidateJSON(data []byte) error {
	// Check size limits
	if len(data) > 10*1024 { // 10KB max
		return fmt.Errorf("announcement too large: %d bytes", len(data))
	}
	
	if len(data) < 50 { // Minimum viable announcement
		return fmt.Errorf("announcement too small: %d bytes", len(data))
	}
	
	return nil
}
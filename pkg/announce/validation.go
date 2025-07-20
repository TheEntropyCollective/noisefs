package announce

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Validation constants
const (
	// Length limits
	MaxDescriptorLength = 100  // CIDs are typically ~59 chars
	MaxTopicLength      = 256  // Reasonable topic length
	MaxTagCount         = 50   // Prevent tag spam
	
	// Time limits
	MaxTTL        = 7 * 24 * time.Hour // 1 week maximum
	MinTTL        = 1 * time.Hour      // 1 hour minimum
	MaxFutureTime = 5 * time.Minute    // Allow 5 min clock skew
	
	// Size limits
	MaxJSONSize      = 10 * 1024 // 10KB max announcement
	MinJSONSize      = 50        // Minimum viable announcement
	MaxTimestampAge  = 365 * 24 * 3600 // 1 year in seconds
	
	// Hash validation
	SHA256HashLength = 64 // SHA-256 hex string length
	MinNonceLength   = 8  // Minimum nonce length
	MaxNonceLength   = 32 // Maximum nonce length
	
	// Bloom filter validation
	MinBloomLength = 4 // Minimum bloom filter length
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
	RequireSignatures   bool          // Whether signatures are mandatory
}

// DefaultValidationConfig returns default validation configuration
func DefaultValidationConfig() *ValidationConfig {
	return &ValidationConfig{
		MaxDescriptorLength: MaxDescriptorLength,
		MaxTopicLength:      MaxTopicLength,
		MaxTagCount:         MaxTagCount,
		MaxTTL:              MaxTTL,
		MinTTL:              MinTTL,
		MaxFutureTime:       MaxFutureTime,
		RequiredFields:      []string{"v", "d", "t", "ts", "ttl"},
		RequireSignatures:   false,      // Signatures optional by default
	}
}

// Validator validates announcements
type Validator struct {
	config            *ValidationConfig
	signatureVerifier *SignatureVerifier
}

// NewValidator creates a new announcement validator
func NewValidator(config *ValidationConfig) *Validator {
	if config == nil {
		config = DefaultValidationConfig()
	}
	return &Validator{
		config:            config,
		signatureVerifier: NewSignatureVerifier(config.RequireSignatures),
	}
}

// ValidateAnnouncement performs comprehensive validation
func (v *Validator) ValidateAnnouncement(ann *Announcement) error {
	// Check version
	if ann.Version == "" {
		return fmt.Errorf("announcement missing required 'version' field - please set to '%s'", DefaultVersion)
	}
	if ann.Version != DefaultVersion {
		return fmt.Errorf("unsupported version %q - only version %q is currently supported", ann.Version, DefaultVersion)
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
		return fmt.Errorf("missing nonce - please provide a random string of %d-%d characters for replay protection", MinNonceLength, MaxNonceLength)
	}
	if len(ann.Nonce) < MinNonceLength || len(ann.Nonce) > MaxNonceLength {
		return fmt.Errorf("invalid nonce length (%d characters) - must be %d-%d characters for security (current: %q)", len(ann.Nonce), MinNonceLength, MaxNonceLength, ann.Nonce)
	}
	
	// Validate peer ID if present
	if ann.PeerID != "" {
		if err := v.signatureVerifier.ValidatePeerID(ann.PeerID); err != nil {
			return fmt.Errorf("invalid peer ID: %w", err)
		}
	}
	
	// Validate signature
	if err := v.signatureVerifier.VerifyAnnouncement(ann); err != nil {
		return fmt.Errorf("signature verification failed: %w", err)
	}
	
	return nil
}

// validateDescriptor validates a descriptor CID
func (v *Validator) validateDescriptor(descriptor string) error {
	if descriptor == "" {
		return fmt.Errorf("descriptor CID is required - please provide a valid IPFS content identifier")
	}
	
	if len(descriptor) > v.config.MaxDescriptorLength {
		return fmt.Errorf("descriptor CID too long (%d characters) - maximum allowed is %d characters", len(descriptor), v.config.MaxDescriptorLength)
	}
	
	// Basic CID validation (should start with Qm or bafy)
	if !strings.HasPrefix(descriptor, "Qm") && !strings.HasPrefix(descriptor, "bafy") {
		return fmt.Errorf("invalid CID format %q - must start with 'Qm' (CIDv0) or 'bafy' (CIDv1)", descriptor)
	}
	
	// Check for valid base58/base32 characters
	if strings.HasPrefix(descriptor, "Qm") {
		// Base58 validation
		if !isValidBase58(descriptor) {
			return fmt.Errorf("invalid base58 encoding in CID %q - contains invalid characters", descriptor)
		}
	}
	
	return nil
}

// validateTopicHash validates a topic hash
func (v *Validator) validateTopicHash(topicHash string) error {
	if topicHash == "" {
		return fmt.Errorf("topic hash is required - please provide a SHA-256 hash of the topic")
	}
	
	// Should be a hex-encoded SHA-256 hash (64 chars)
	if len(topicHash) != SHA256HashLength {
		return fmt.Errorf("invalid topic hash length (%d characters) - must be exactly %d characters (SHA-256 hex)", len(topicHash), SHA256HashLength)
	}
	
	// Validate hex encoding
	if _, err := hex.DecodeString(topicHash); err != nil {
		return fmt.Errorf("invalid topic hash %q - must contain only hexadecimal characters (0-9, a-f): %w", topicHash, err)
	}
	
	return nil
}

// validateTimestamp validates announcement timestamp
func (v *Validator) validateTimestamp(timestamp int64) error {
	if timestamp <= 0 {
		return fmt.Errorf("invalid timestamp %d - must be a positive Unix timestamp (seconds since epoch)", timestamp)
	}
	
	now := time.Now().Unix()
	
	// Check if too far in the past (older than 1 year)
	if timestamp < now - MaxTimestampAge {
		oldTime := time.Unix(timestamp, 0)
		return fmt.Errorf("timestamp too old (%s) - announcements cannot be older than 1 year", oldTime.Format("2006-01-02 15:04:05"))
	}
	
	// Check if too far in the future
	maxFuture := now + int64(v.config.MaxFutureTime.Seconds())
	if timestamp > maxFuture {
		futureTime := time.Unix(timestamp, 0)
		maxTime := time.Unix(maxFuture, 0)
		return fmt.Errorf("timestamp too far in future (%s) - cannot be more than %v ahead of current time (max: %s)", 
			futureTime.Format("2006-01-02 15:04:05"), v.config.MaxFutureTime, maxTime.Format("2006-01-02 15:04:05"))
	}
	
	return nil
}

// validateTTL validates time-to-live
func (v *Validator) validateTTL(ttl int64) error {
	if ttl <= 0 {
		return fmt.Errorf("invalid TTL %d - must be a positive number of seconds (minimum: %v)", ttl, v.config.MinTTL)
	}
	
	ttlDuration := time.Duration(ttl) * time.Second
	
	if ttlDuration < v.config.MinTTL {
		return fmt.Errorf("TTL too short (%v) - minimum allowed is %v (announcements need sufficient time to propagate)", ttlDuration, v.config.MinTTL)
	}
	
	if ttlDuration > v.config.MaxTTL {
		return fmt.Errorf("TTL too long (%v) - maximum allowed is %v (prevents resource exhaustion)", ttlDuration, v.config.MaxTTL)
	}
	
	return nil
}

// validateCategory validates content category
func (v *Validator) validateCategory(category string) error {
	validCategories := map[string]bool{
		"video":    true,
		"audio":    true,
		"document": true,
		"data":     true,
		"software": true,
		"image":    true,
		"archive":  true,
		"other":    true,
	}
	
	if !validCategories[category] {
		validList := []string{"video", "audio", "document", "data", "software", "image", "archive", "other"}
		return fmt.Errorf("invalid category %q - must be one of: %v (use 'other' if unsure)", category, validList)
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
		validList := []string{"tiny (<1MB)", "small (1-10MB)", "medium (10-100MB)", "large (100MB-1GB)", "huge (>1GB)"}
		return fmt.Errorf("invalid size class %q - must be one of: %v", sizeClass, validList)
	}
	
	return nil
}

// validateBloomFilter validates bloom filter encoding
func (v *Validator) validateBloomFilter(bloomStr string) error {
	if bloomStr == "" {
		return nil // Bloom filter is optional
	}
	
	// Should be base64 encoded
	if len(bloomStr) < MinBloomLength {
		return fmt.Errorf("bloom filter too short (%d characters) - minimum length is %d characters for valid base64 encoding", len(bloomStr), MinBloomLength)
	}
	
	// Try to decode
	_, err := DecodeBloom(bloomStr)
	if err != nil {
		return fmt.Errorf("invalid bloom filter encoding %q - must be valid base64 encoded bloom filter data: %w", bloomStr, err)
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
	if len(data) > MaxJSONSize {
		return fmt.Errorf("announcement too large: %d bytes", len(data))
	}
	
	if len(data) < MinJSONSize {
		return fmt.Errorf("announcement too small: %d bytes", len(data))
	}
	
	return nil
}
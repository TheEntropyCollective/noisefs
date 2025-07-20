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

// ValidationConfig provides comprehensive configuration for announcement validation and security policies.
//
// This configuration structure controls validation rules and security requirements
// for NoiseFS announcements, enabling fine-tuned control over content acceptance
// criteria. It balances security, performance, and usability by providing sensible
// defaults while allowing customization for specific deployment requirements.
//
// Security Considerations:
//   - Length limits prevent DoS attacks and resource exhaustion
//   - Time limits prevent temporal attacks and clock skew issues
//   - Field requirements ensure announcement completeness
//   - Signature policies control authentication requirements
type ValidationConfig struct {
	// MaxDescriptorLength sets the maximum allowed length for descriptor CIDs.
	// Prevents oversized identifiers that could indicate invalid or malicious content.
	// Default: 100 characters (IPFS CIDs are typically ~59 characters).
	MaxDescriptorLength int
	
	// MaxTopicLength limits topic string length to prevent abuse.
	// Reasonable limit that supports hierarchical organization without resource exhaustion.
	// Default: 256 characters.
	MaxTopicLength      int
	
	// MaxTagCount limits the number of tags per announcement.
	// Prevents tag spam while supporting rich content metadata.
	// Default: 50 tags.
	MaxTagCount         int
	
	// MaxTTL sets the maximum time-to-live for announcements.
	// Prevents extremely long-lived announcements that could exhaust network resources.
	// Default: 7 days (1 week).
	MaxTTL              time.Duration
	
	// MinTTL sets the minimum time-to-live for announcements.
	// Ensures announcements have sufficient time to propagate through the network.
	// Default: 1 hour.
	MinTTL              time.Duration
	
	// MaxFutureTime allows for reasonable clock skew between network participants.
	// Prevents announcements with timestamps too far in the future.
	// Default: 5 minutes.
	MaxFutureTime       time.Duration
	
	// RequiredFields specifies which JSON fields must be present in announcements.
	// Ensures announcements contain minimum required metadata for proper operation.
	// Default: ["v", "d", "t", "ts", "ttl"] (version, descriptor, topic, timestamp, TTL).
	RequiredFields      []string
	
	// RequireSignatures determines whether IPNS signatures are mandatory.
	// When true, all announcements must include valid cryptographic signatures.
	// Default: false (signatures optional for anonymous announcements).
	RequireSignatures   bool
}

// DefaultValidationConfig returns a sensible default validation configuration for NoiseFS announcements.
//
// This configuration provides a balanced approach to announcement validation,
// prioritizing security and network health while supporting diverse use cases.
// The defaults are designed for typical NoiseFS deployments and can be
// customized for specific requirements.
//
// Returns:
//   ValidationConfig with production-ready default values
//
// Time Complexity: O(1)
// Space Complexity: O(1)
//
// Default Values:
//   - MaxDescriptorLength: 100 chars (supports all IPFS CID formats)
//   - MaxTopicLength: 256 chars (reasonable hierarchical depth)
//   - MaxTagCount: 50 tags (rich metadata without spam)
//   - MaxTTL: 7 days (balanced availability vs resource usage)
//   - MinTTL: 1 hour (sufficient propagation time)
//   - MaxFutureTime: 5 minutes (reasonable clock skew tolerance)
//   - RequiredFields: ["v", "d", "t", "ts", "ttl"] (essential metadata)
//   - RequireSignatures: false (supports anonymous announcements)
//
// Security Properties:
//   - Prevents resource exhaustion attacks via length limits
//   - Mitigates temporal attacks through time boundaries
//   - Ensures announcement completeness via required fields
//   - Supports both signed and anonymous content models
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

// Validator provides comprehensive announcement validation and security verification for NoiseFS.
//
// The Validator performs multi-layered validation of announcements, ensuring they
// meet format requirements, security policies, and network compatibility standards.
// It integrates configurable validation rules with cryptographic signature verification
// to provide robust content validation.
//
// Validation Layers:
//   - Format validation: JSON structure, field presence, data types
//   - Content validation: CID format, hash validation, category checking
//   - Security validation: Length limits, time bounds, signature verification
//   - Policy validation: Custom rules and requirements
//
// Thread Safety: Validator is safe for concurrent use across multiple goroutines.
// All validation operations are stateless and deterministic.
type Validator struct {
	// config holds the validation rules and security policies.
	// Controls length limits, time bounds, required fields, and signature requirements.
	config            *ValidationConfig
	
	// signatureVerifier handles cryptographic signature validation.
	// Validates IPNS signatures when present or required by policy.
	signatureVerifier *SignatureVerifier
}

// NewValidator creates a new announcement validator with the specified configuration.
//
// The validator is initialized with validation rules and signature verification
// capabilities based on the provided configuration. If no configuration is provided,
// sensible defaults are used that work for most NoiseFS deployments.
//
// Parameters:
//   - config: Validation configuration, or nil for defaults
//
// Returns:
//   A new Validator ready for announcement validation
//
// Time Complexity: O(1)
// Space Complexity: O(1)
//
// Thread Safety: The returned Validator is safe for concurrent use.
//
// Configuration Impact:
//   - Validation rules determine acceptance criteria
//   - Signature requirements affect authentication validation
//   - Length and time limits control resource usage
//
// Example:
//   validator := announce.NewValidator(announce.DefaultValidationConfig())
//   err := validator.ValidateAnnouncement(announcement)
func NewValidator(config *ValidationConfig) *Validator {
	if config == nil {
		config = DefaultValidationConfig()
	}
	return &Validator{
		config:            config,
		signatureVerifier: NewSignatureVerifier(config.RequireSignatures),
	}
}

// ValidateAnnouncement performs comprehensive multi-layered validation of NoiseFS announcements.
//
// This method conducts thorough validation of all announcement components,
// ensuring format correctness, security compliance, and network compatibility.
// It validates structure, content, cryptographic elements, and policy compliance
// to provide robust protection against malformed or malicious announcements.
//
// Parameters:
//   - ann: The announcement to validate
//
// Returns:
//   - nil if the announcement passes all validation checks
//   - error with detailed description if validation fails
//
// Time Complexity: O(n) where n is the size of the announcement data
// Space Complexity: O(1)
//
// Validation Stages:
//   1. Version compatibility verification
//   2. Descriptor CID format and validity
//   3. Topic hash cryptographic validation
//   4. Timestamp bounds and clock skew checking
//   5. TTL policy compliance verification
//   6. Category and size class enumeration validation
//   7. Bloom filter format and decoding verification
//   8. Nonce presence and format validation
//   9. Peer ID format validation (if present)
//   10. Cryptographic signature verification (if present/required)
//
// Security Properties:
//   - Prevents injection attacks via input sanitization
//   - Mitigates DoS attacks through length and time limits
//   - Ensures cryptographic integrity via signature validation
//   - Validates all security-critical fields for completeness
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

// validateDescriptor validates NoiseFS descriptor CID format and properties.
//
// This method ensures that descriptor CIDs are properly formatted IPFS content
// identifiers that can be used for content retrieval. It validates both CIDv0
// and CIDv1 formats while checking length limits and encoding correctness.
//
// Parameters:
//   - descriptor: The descriptor CID string to validate
//
// Returns:
//   - nil if the descriptor is valid
//   - error with specific validation failure details
//
// Time Complexity: O(n) where n is the descriptor length
// Space Complexity: O(1)
//
// Validation Rules:
//   - Non-empty string required
//   - Length must not exceed configured maximum
//   - Must start with "Qm" (CIDv0) or "bafy" (CIDv1)
//   - CIDv0: Valid base58 character encoding
//   - CIDv1: Valid base32 character encoding (future enhancement)
//
// Supported Formats:
//   - CIDv0: "Qm..." with base58 encoding (legacy format)
//   - CIDv1: "bafy..." with base32 encoding (modern format)
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

// validateTopicHash validates SHA-256 topic hash format and encoding.
//
// This method ensures that topic hashes are properly formatted SHA-256 hashes
// that can be used for privacy-preserving topic organization. It validates
// both the length and hexadecimal encoding of the hash.
//
// Parameters:
//   - topicHash: The topic hash string to validate
//
// Returns:
//   - nil if the topic hash is valid
//   - error with specific validation failure details
//
// Time Complexity: O(n) where n is the hash length (typically O(1) for 64 chars)
// Space Complexity: O(1)
//
// Validation Rules:
//   - Non-empty string required
//   - Exactly 64 characters (SHA-256 hex length)
//   - Valid hexadecimal encoding (0-9, a-f)
//   - Case-insensitive hex validation
//
// Security Properties:
//   - Ensures cryptographic hash integrity
//   - Prevents malformed hashes from entering the system
//   - Validates encoding to prevent injection attacks
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

// validateTimestamp validates announcement timestamp for temporal security and network compatibility.
//
// This method ensures that announcement timestamps are reasonable and fall within
// acceptable bounds to prevent temporal attacks and clock skew issues. It validates
// against both past and future time limits to maintain network coherence.
//
// Parameters:
//   - timestamp: Unix timestamp (seconds since epoch) to validate
//
// Returns:
//   - nil if the timestamp is valid
//   - error with specific validation failure details
//
// Time Complexity: O(1)
// Space Complexity: O(1)
//
// Validation Rules:
//   - Must be positive (after Unix epoch)
//   - Cannot be older than 1 year (prevents ancient announcements)
//   - Cannot be more than MaxFutureTime ahead of current time
//   - Accounts for reasonable clock skew between network participants
//
// Security Properties:
//   - Prevents replay attacks with extremely old timestamps
//   - Mitigates clock skew attacks from future timestamps
//   - Ensures temporal consistency across distributed network
//   - Protects against timestamp manipulation attacks
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

// validateTTL validates announcement time-to-live for resource management and network health.
//
// This method ensures that TTL values are within reasonable bounds to prevent
// resource exhaustion while ensuring sufficient propagation time. It balances
// network performance with content availability requirements.
//
// Parameters:
//   - ttl: Time-to-live in seconds to validate
//
// Returns:
//   - nil if the TTL is valid
//   - error with specific validation failure details
//
// Time Complexity: O(1)
// Space Complexity: O(1)
//
// Validation Rules:
//   - Must be positive (greater than 0)
//   - Must be at least MinTTL (ensures propagation time)
//   - Must not exceed MaxTTL (prevents resource exhaustion)
//   - Configured limits balance availability vs resource usage
//
// Resource Management:
//   - Minimum TTL ensures announcements can propagate
//   - Maximum TTL prevents indefinite resource consumption
//   - Bounds are configurable based on network characteristics
//   - Default range: 1 hour to 7 days
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

// validateCategory validates content category against predefined enumeration.
//
// This method ensures that content categories are limited to predefined values
// to maintain consistency across the network and prevent arbitrary category
// creation that could be used for attacks or spam.
//
// Parameters:
//   - category: Content category string to validate
//
// Returns:
//   - nil if the category is valid
//   - error with specific validation failure and valid options
//
// Time Complexity: O(1) - map lookup
// Space Complexity: O(1)
//
// Valid Categories:
//   - "video": Video content (movies, clips, streams)
//   - "audio": Audio content (music, podcasts, audiobooks)
//   - "document": Text documents (PDFs, docs, ebooks)
//   - "data": Data files (CSV, JSON, databases)
//   - "software": Software packages and executables
//   - "image": Image files (photos, graphics, diagrams)
//   - "archive": Compressed archives and collections
//   - "other": Miscellaneous or unclassified content
//
// Security Benefits:
//   - Prevents category enumeration attacks
//   - Maintains consistent categorization across network
//   - Limits potential for category-based spam or abuse
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

// validateSizeClass validates size classification for privacy-preserving size categorization.
//
// This method ensures that size classes are limited to predefined ranges that
// provide useful filtering capabilities while protecting exact file size privacy.
// It validates against the standard NoiseFS size classification system.
//
// Parameters:
//   - sizeClass: Size classification string to validate
//
// Returns:
//   - nil if the size class is valid
//   - error with specific validation failure and valid options with size ranges
//
// Time Complexity: O(1) - map lookup
// Space Complexity: O(1)
//
// Valid Size Classes:
//   - "tiny": < 1MB (text files, small images)
//   - "small": 1-10MB (documents, music tracks)
//   - "medium": 10-100MB (software, short videos)
//   - "large": 100MB-1GB (large software, TV episodes)
//   - "huge": >= 1GB (movies, large datasets)
//
// Privacy Properties:
//   - Prevents exact file size disclosure
//   - Provides useful filtering without privacy loss
//   - Consistent classification across all NoiseFS clients
//   - Protects against size-based fingerprinting attacks
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

// validateBloomFilter validates bloom filter encoding and format for tag-based discovery.
//
// This method ensures that bloom filters are properly encoded and can be decoded
// for tag matching operations. It validates the base64 encoding and internal
// bloom filter structure to prevent malformed filters from entering the system.
//
// Parameters:
//   - bloomStr: Base64-encoded bloom filter string to validate
//
// Returns:
//   - nil if the bloom filter is valid or empty (optional field)
//   - error with specific validation failure details
//
// Time Complexity: O(n) where n is the bloom filter size
// Space Complexity: O(n) for decoding validation
//
// Validation Process:
//   - Checks minimum length for valid base64 encoding
//   - Attempts full decode using DecodeBloom function
//   - Validates internal bloom filter structure and parameters
//   - Ensures filter can be used for tag matching operations
//
// Empty String Handling:
//   - Empty bloom filters are valid (no tags case)
//   - Allows announcements without tag-based discovery
//   - Supports minimal announcements with topic-only organization
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

// isValidBase58 validates that a string contains only valid base58 characters used in CIDv0.
//
// This function checks each character against the base58 alphabet used by IPFS
// for CIDv0 encoding. Base58 encoding excludes visually similar characters
// (0, O, I, l) to reduce human transcription errors.
//
// Parameters:
//   - s: String to validate for base58 character compliance
//
// Returns:
//   - true if all characters are valid base58
//   - false if any character is outside the base58 alphabet
//
// Time Complexity: O(n) where n is the string length
// Space Complexity: O(1)
//
// Base58 Alphabet:
//   - Numbers: 1-9 (excludes 0 to avoid confusion with O)
//   - Uppercase: A-H, J-N, P-Z (excludes I, O for clarity)
//   - Lowercase: a-k, m-z (excludes l for clarity)
//   - Total: 58 characters for unambiguous encoding
//
// Use Case: Validates CIDv0 descriptors starting with "Qm" prefix.
func isValidBase58(s string) bool {
	const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	for _, char := range s {
		if !strings.ContainsRune(base58Alphabet, char) {
			return false
		}
	}
	return true
}

// ValidateJSON performs preliminary validation of announcement JSON data before parsing.
//
// This function provides fast, low-cost validation of JSON size constraints
// before expensive parsing operations. It prevents resource exhaustion attacks
// and ensures announcements fall within reasonable size bounds.
//
// Parameters:
//   - data: Raw JSON byte data to validate
//
// Returns:
//   - nil if the JSON data size is acceptable
//   - error if size constraints are violated
//
// Time Complexity: O(1) - only checks length
// Space Complexity: O(1)
//
// Size Constraints:
//   - Maximum: 10KB (prevents DoS attacks via oversized announcements)
//   - Minimum: 50 bytes (ensures viable announcement content)
//   - Balances functionality with resource protection
//
// Security Benefits:
//   - Prevents memory exhaustion from extremely large announcements
//   - Rejects obviously invalid tiny payloads before processing
//   - Fast validation with minimal resource consumption
//   - First line of defense against malformed input
//
// Use Case: Called before JSON unmarshaling in announcement processing pipelines.
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
// Package announce implements NoiseFS's privacy-preserving content discovery system.
//
// This package provides a distributed announcement system that allows users to
// publish and discover content while preserving privacy through bloom filters,
// topic hashing, and size classification. Announcements are distributed via
// IPFS DHT and PubSub for real-time discovery.
//
// Key Features:
//   - Privacy-preserving tag matching using bloom filters
//   - Topic-based content organization with hierarchical structure
//   - Size classification to prevent exact size leakage
//   - Optional IPNS signatures for authenticity
//   - TTL-based expiration for resource management
//   - Real-time discovery via IPFS PubSub
//
// Privacy Model:
// The announcement system is designed with privacy as a core principle:
//   - Tags are stored in bloom filters to prevent exact tag exposure
//   - Topics are SHA-256 hashed to prevent topic enumeration
//   - File sizes are classified into ranges rather than exact values
//   - Optional signatures allow verification without mandatory identity disclosure
//
// Usage Example:
//   creator := announce.NewCreator()
//   ann, err := creator.CreateFromFile(descriptorCID, filePath, announce.CreateOptions{
//       Topic: "documents/research",
//       Tags:  []string{"pdf", "science", "2024"},
//       TTL:   24 * time.Hour,
//   })
package announce

import (
	"encoding/json"
	"errors"
	"time"
)

// Protocol and system constants for the announcement system.
// These constants define the behavior and limits of the NoiseFS announcement protocol.
const (
	// DefaultVersion is the current announcement protocol version.
	// This version must match between publishers and subscribers for compatibility.
	DefaultVersion = "1.0"
	
	// DefaultTTL is the default time-to-live for announcements in seconds (24 hours).
	// After this time, announcements are considered expired and may be purged.
	// This balances network resource usage with content availability.
	DefaultTTL = 86400 // 24 hours in seconds
	
	// Size class boundaries define the thresholds for file size classification.
	// These boundaries prevent exact size leakage while maintaining useful categorization.
	// The classification helps users filter content by approximate size without
	// revealing precise file sizes for privacy protection.
	SizeClassTinyLimit   = 1024 * 1024      // 1MB - small documents, images
	SizeClassSmallLimit  = 10 * 1024 * 1024 // 10MB - large documents, music
	SizeClassMediumLimit = 100 * 1024 * 1024 // 100MB - software, video clips
	SizeClassLargeLimit  = 1024 * 1024 * 1024 // 1GB - large software, movies
)

// Version is an alias for DefaultVersion, providing backward compatibility.
// Use DefaultVersion in new code for clarity.
const Version = DefaultVersion

// Content category constants for broad content classification.
// These categories provide a coarse-grained content type system that helps
// users discover relevant content without revealing specific file types.
// The classification is intentionally broad to maintain privacy while
// providing useful filtering capabilities.
const (
	CategoryVideo    = "video"    // Video files (movies, clips, streams)
	CategoryAudio    = "audio"    // Audio files (music, podcasts, audiobooks)
	CategoryDocument = "document" // Text documents (PDFs, docs, ebooks)
	CategoryData     = "data"     // Data files (CSV, JSON, databases)
	CategorySoftware = "software" // Software packages and executables
	CategoryImage    = "image"    // Image files (photos, graphics, diagrams)
	CategoryArchive  = "archive"  // Compressed archives and collections
	CategoryOther    = "other"    // Miscellaneous or unclassified content
)

// Size classification constants for privacy-preserving file size categorization.
// Instead of revealing exact file sizes, content is classified into these
// broad ranges to maintain privacy while still providing useful filtering.
// This prevents fingerprinting attacks based on precise file sizes.
const (
	SizeClassTiny   = "tiny"   // < 1MB (text files, small images)
	SizeClassSmall  = "small"  // 1-10MB (documents, music tracks)
	SizeClassMedium = "medium" // 10-100MB (software, short videos)
	SizeClassLarge  = "large"  // 100MB-1GB (large software, TV episodes)
	SizeClassHuge   = "huge"   // >= 1GB (movies, large datasets)
)

// Announcement represents a privacy-preserving content announcement in the NoiseFS network.
//
// An announcement contains metadata about available content without revealing
// sensitive information. The structure is designed to enable discovery while
// maintaining user privacy through several techniques:
//
//   - Topic hashes prevent enumeration of topics
//   - Bloom filters hide exact tags while enabling matching
//   - Size classes prevent exact file size disclosure
//   - Optional signatures provide authenticity without mandatory identity
//
// Privacy Implications:
//   - All announcements are public and visible to the network
//   - Tags are probabilistically matchable via bloom filters (false positives possible)
//   - Topics are discoverable only if the exact topic string is known
//   - File sizes are revealed only within broad ranges
//   - Signatures are optional and reveal the signer's public key when used
//
// The announcement format is optimized for size and network efficiency while
// providing sufficient metadata for content discovery.
type Announcement struct {
	// Version specifies the announcement protocol version (currently "1.0").
	// This ensures compatibility between different NoiseFS implementations.
	Version    string `json:"v"`
	
	// Descriptor contains the NoiseFS descriptor CID that can be used to
	// retrieve and reconstruct the announced content. This is the primary
	// identifier for the content and should be a valid IPFS CID.
	Descriptor string `json:"d"`
	
	// TopicHash is a SHA-256 hash of the topic string, providing privacy-preserving
	// topic organization. Only users who know the exact topic can discover content
	// in that topic, preventing topic enumeration attacks.
	TopicHash  string `json:"t"`
	
	// TagBloom contains a base64-encoded bloom filter of content tags.
	// This enables privacy-preserving tag matching - users can search for tags
	// without revealing which specific tags are associated with the content.
	// May produce false positives but never false negatives.
	TagBloom   string `json:"tb,omitempty"`
	
	// Category provides broad content classification (video, audio, document, etc.).
	// This coarse-grained categorization helps discovery while maintaining privacy
	// by not revealing specific file types or formats.
	Category   string `json:"c"`
	
	// SizeClass indicates the approximate size range of the content
	// (tiny, small, medium, large, huge) without revealing exact file sizes.
	// This prevents size-based fingerprinting while enabling size-based filtering.
	SizeClass  string `json:"s"`
	
	// Timestamp indicates when the announcement was created (Unix timestamp).
	// Used for sorting, expiration calculation, and freshness determination.
	Timestamp  int64  `json:"ts"`
	
	// TTL (Time To Live) specifies how long the announcement should be considered
	// valid, in seconds. After Timestamp + TTL, the announcement expires.
	TTL        int64  `json:"ttl"`
	
	// Nonce provides uniqueness for announcements with otherwise identical content.
	// This prevents duplicate announcements and enables replay protection.
	Nonce      string `json:"n,omitempty"`
	
	// PeerID identifies the announcing peer when the announcement is signed.
	// This field is only present when the announcement includes a signature
	// for authenticity verification.
	PeerID     string `json:"pid,omitempty"`
	
	// Signature contains an optional IPNS signature for announcement authenticity.
	// When present, recipients can verify the announcement came from the claimed peer.
	// Signatures are optional to support anonymous announcements.
	Signature  string `json:"sig,omitempty"`
}

// NewAnnouncement creates a new announcement with sensible defaults.
//
// This constructor initializes a basic announcement with the current protocol version,
// current timestamp, and default TTL. Additional fields like tags, category, and
// size class should be set separately based on the content being announced.
//
// Parameters:
//   - descriptor: The NoiseFS descriptor CID for the content
//   - topicHash: SHA-256 hash of the topic string
//
// Returns:
//   A new Announcement with default values that can be customized before publishing.
//
// Time Complexity: O(1)
//
// Example:
//   ann := NewAnnouncement("QmExample...", "2f8a7...") 
//   ann.Category = CategoryDocument
//   ann.SizeClass = GetSizeClass(fileSize)
func NewAnnouncement(descriptor, topicHash string) *Announcement {
	return &Announcement{
		Version:    Version,
		Descriptor: descriptor,
		TopicHash:  topicHash,
		Timestamp:  time.Now().Unix(),
		TTL:        DefaultTTL,
	}
}

// Validate performs comprehensive validation of the announcement structure and content.
//
// This method checks all required fields, validates format constraints, and ensures
// the announcement meets protocol requirements. It should be called before
// publishing announcements to prevent invalid data from entering the network.
//
// Validation Rules:
//   - Version must match the current protocol version
//   - Descriptor and TopicHash cannot be empty
//   - Category and SizeClass must be valid predefined values
//   - Timestamp must be positive (Unix timestamp)
//   - TTL must be positive (seconds)
//
// Returns:
//   - nil if the announcement is valid
//   - error describing the first validation failure encountered
//
// Time Complexity: O(1) - constant time validation
//
// Note: This method does not validate cryptographic signatures or network
// connectivity requirements - those are handled by separate verification layers.
func (a *Announcement) Validate() error {
	// Validate protocol version compatibility
	if a.Version != Version {
		return errors.New("unsupported announcement version")
	}
	
	// Validate required identifier fields
	if a.Descriptor == "" {
		return errors.New("descriptor cannot be empty")
	}
	
	if a.TopicHash == "" {
		return errors.New("topic hash cannot be empty")
	}
	
	// Validate enumerated field values
	if !isValidCategory(a.Category) {
		return errors.New("invalid category")
	}
	
	if !isValidSizeClass(a.SizeClass) {
		return errors.New("invalid size class")
	}
	
	// Validate temporal fields
	if a.Timestamp <= 0 {
		return errors.New("invalid timestamp")
	}
	
	if a.TTL <= 0 {
		return errors.New("TTL must be positive")
	}
	
	return nil
}

// IsExpired determines if the announcement has exceeded its time-to-live.
//
// An announcement is considered expired when the current time exceeds
// the announcement's timestamp plus its TTL. Expired announcements
// should be ignored during discovery and may be purged from storage.
//
// Returns:
//   - true if the announcement has expired
//   - false if the announcement is still valid
//
// Time Complexity: O(1)
//
// Note: This method uses the system clock for time comparison. In distributed
// systems, clock skew between nodes may cause slight variations in expiration
// timing. The validation layer handles reasonable clock skew tolerance.
func (a *Announcement) IsExpired() bool {
	// Calculate expiration time: announcement timestamp + TTL
	expiryTime := time.Unix(a.Timestamp, 0).Add(time.Duration(a.TTL) * time.Second)
	// Compare against current system time
	return time.Now().After(expiryTime)
}

// ToJSON serializes the announcement to JSON format with validation.
//
// This method performs validation before serialization to ensure only
// valid announcements are converted to JSON. The resulting JSON is
// suitable for network transmission and storage.
//
// Returns:
//   - JSON byte array of the announcement
//   - error if validation fails or JSON marshaling fails
//
// Time Complexity: O(n) where n is the size of the announcement data
//
// The JSON format uses compact field names to minimize network overhead:
//   - "v" for Version
//   - "d" for Descriptor  
//   - "t" for TopicHash
//   - etc.
func (a *Announcement) ToJSON() ([]byte, error) {
	// Validate before serialization to prevent invalid data transmission
	if err := a.Validate(); err != nil {
		return nil, err
	}
	// Serialize to compact JSON format
	return json.Marshal(a)
}

// FromJSON deserializes an announcement from JSON data with validation.
//
// This function parses JSON data into an Announcement struct and validates
// the result to ensure it meets protocol requirements. It's the inverse
// operation of ToJSON and should be used when receiving announcements
// from the network or storage.
//
// Parameters:
//   - data: JSON byte array to deserialize
//
// Returns:
//   - Parsed and validated Announcement
//   - error if JSON parsing fails or validation fails
//
// Time Complexity: O(n) where n is the size of the JSON data
//
// Security Note: This function validates the parsed data to prevent
// processing of malformed or malicious announcements.
func FromJSON(data []byte) (*Announcement, error) {
	// Parse JSON into announcement structure
	var ann Announcement
	if err := json.Unmarshal(data, &ann); err != nil {
		return nil, err
	}
	
	// Validate the parsed announcement for protocol compliance
	if err := ann.Validate(); err != nil {
		return nil, err
	}
	
	return &ann, nil
}

// GetSizeClass determines the appropriate size classification for a given byte count.
//
// This function maps exact file sizes to privacy-preserving size classes,
// preventing exact size disclosure while maintaining useful categorization
// for discovery and filtering. The classification boundaries are designed
// to provide meaningful distinctions for different content types.
//
// Parameters:
//   - sizeBytes: The exact size in bytes to classify
//
// Returns:
//   - Size class string (tiny, small, medium, large, or huge)
//
// Time Complexity: O(1) - constant time classification
//
// Size Class Mappings:
//   - tiny:   < 1MB     (text files, small images)
//   - small:  1-10MB    (documents, music tracks)  
//   - medium: 10-100MB  (software, short videos)
//   - large:  100MB-1GB (large software, TV episodes)
//   - huge:   >= 1GB    (movies, large datasets)
//
// Privacy Note: This classification intentionally obscures exact file sizes
// to prevent fingerprinting while maintaining useful size-based filtering.
func GetSizeClass(sizeBytes int64) string {
	// Classify size using predefined boundaries
	switch {
	case sizeBytes < SizeClassTinyLimit:   // < 1MB
		return SizeClassTiny
	case sizeBytes < SizeClassSmallLimit:  // < 10MB  
		return SizeClassSmall
	case sizeBytes < SizeClassMediumLimit: // < 100MB
		return SizeClassMedium
	case sizeBytes < SizeClassLargeLimit:  // < 1GB
		return SizeClassLarge
	default:                               // >= 1GB
		return SizeClassHuge
	}
}

// Helper functions for validation

// isValidCategory checks if a category string is one of the predefined valid categories.
//
// This function validates that the category field contains only approved values,
// preventing arbitrary category strings that could be used for fingerprinting
// or attacks. The category system is intentionally limited to broad classifications.
//
// Parameters:
//   - category: The category string to validate
//
// Returns:
//   - true if the category is valid
//   - false if the category is invalid or empty
//
// Time Complexity: O(1) - constant time string comparison
func isValidCategory(category string) bool {
	// Check against predefined category constants
	switch category {
	case CategoryVideo, CategoryAudio, CategoryDocument, 
	     CategoryData, CategorySoftware, CategoryImage,
	     CategoryArchive, CategoryOther:
		return true
	default:
		return false
	}
}

// isValidSizeClass checks if a size class string is one of the predefined valid size classes.
//
// This function validates that the size class field contains only approved values,
// ensuring consistency in size classification across the network. Invalid size
// classes could indicate corrupted data or protocol violations.
//
// Parameters:
//   - sizeClass: The size class string to validate
//
// Returns:
//   - true if the size class is valid
//   - false if the size class is invalid or empty
//
// Time Complexity: O(1) - constant time string comparison
func isValidSizeClass(sizeClass string) bool {
	// Check against predefined size class constants
	switch sizeClass {
	case SizeClassTiny, SizeClassSmall, SizeClassMedium,
	     SizeClassLarge, SizeClassHuge:
		return true
	default:
		return false
	}
}
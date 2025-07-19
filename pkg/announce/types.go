package announce

import (
	"encoding/json"
	"errors"
	"time"
)

// Announcement constants
const (
	// Protocol version
	DefaultVersion = "1.0"
	
	// Default TTL
	DefaultTTL = 86400 // 24 hours in seconds
	
	// Size class boundaries
	SizeClassTinyLimit   = 1024 * 1024      // 1MB
	SizeClassSmallLimit  = 10 * 1024 * 1024 // 10MB
	SizeClassMediumLimit = 100 * 1024 * 1024 // 100MB
	SizeClassLargeLimit  = 1024 * 1024 * 1024 // 1GB
)

// Version of the announcement protocol
const Version = DefaultVersion

// Category constants for broad content classification
const (
	CategoryVideo    = "video"
	CategoryAudio    = "audio"
	CategoryDocument = "document"
	CategoryData     = "data"
	CategorySoftware = "software"
	CategoryOther    = "other"
)

// SizeClass constants for file size classification
const (
	SizeClassTiny   = "tiny"   // < 1MB
	SizeClassSmall  = "small"  // < 10MB
	SizeClassMedium = "medium" // < 100MB
	SizeClassLarge  = "large"  // < 1GB
	SizeClassHuge   = "huge"   // >= 1GB
)

// Announcement represents a content announcement in the NoiseFS network
type Announcement struct {
	Version    string `json:"v"`              // Protocol version
	Descriptor string `json:"d"`              // NoiseFS descriptor CID
	TopicHash  string `json:"t"`              // SHA-256 hash of the topic
	TagBloom   string `json:"tb,omitempty"`   // Bloom filter of tags (base64)
	Category   string `json:"c"`              // Broad category
	SizeClass  string `json:"s"`              // Size classification
	Timestamp  int64  `json:"ts"`             // Unix timestamp
	TTL        int64  `json:"ttl"`            // Time to live in seconds
	Nonce      string `json:"n,omitempty"`    // Random nonce for uniqueness
	PeerID     string `json:"pid,omitempty"`  // Peer ID of the announcer (when signed)
	Signature  string `json:"sig,omitempty"`  // Optional IPNS signature
}

// NewAnnouncement creates a new announcement with defaults
func NewAnnouncement(descriptor, topicHash string) *Announcement {
	return &Announcement{
		Version:    Version,
		Descriptor: descriptor,
		TopicHash:  topicHash,
		Timestamp:  time.Now().Unix(),
		TTL:        DefaultTTL,
	}
}

// Validate checks if the announcement is valid
func (a *Announcement) Validate() error {
	if a.Version != Version {
		return errors.New("unsupported announcement version")
	}
	
	if a.Descriptor == "" {
		return errors.New("descriptor cannot be empty")
	}
	
	if a.TopicHash == "" {
		return errors.New("topic hash cannot be empty")
	}
	
	if !isValidCategory(a.Category) {
		return errors.New("invalid category")
	}
	
	if !isValidSizeClass(a.SizeClass) {
		return errors.New("invalid size class")
	}
	
	if a.Timestamp <= 0 {
		return errors.New("invalid timestamp")
	}
	
	if a.TTL <= 0 {
		return errors.New("TTL must be positive")
	}
	
	return nil
}

// IsExpired checks if the announcement has expired
func (a *Announcement) IsExpired() bool {
	expiryTime := time.Unix(a.Timestamp, 0).Add(time.Duration(a.TTL) * time.Second)
	return time.Now().After(expiryTime)
}

// ToJSON serializes the announcement to JSON
func (a *Announcement) ToJSON() ([]byte, error) {
	if err := a.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(a)
}

// FromJSON deserializes an announcement from JSON
func FromJSON(data []byte) (*Announcement, error) {
	var ann Announcement
	if err := json.Unmarshal(data, &ann); err != nil {
		return nil, err
	}
	
	if err := ann.Validate(); err != nil {
		return nil, err
	}
	
	return &ann, nil
}

// GetSizeClass determines the size class from byte count
func GetSizeClass(sizeBytes int64) string {
	switch {
	case sizeBytes < SizeClassTinyLimit:
		return SizeClassTiny
	case sizeBytes < SizeClassSmallLimit:
		return SizeClassSmall
	case sizeBytes < SizeClassMediumLimit:
		return SizeClassMedium
	case sizeBytes < SizeClassLargeLimit:
		return SizeClassLarge
	default:
		return SizeClassHuge
	}
}

// Helper functions

func isValidCategory(category string) bool {
	switch category {
	case CategoryVideo, CategoryAudio, CategoryDocument, 
	     CategoryData, CategorySoftware, CategoryOther:
		return true
	default:
		return false
	}
}

func isValidSizeClass(sizeClass string) bool {
	switch sizeClass {
	case SizeClassTiny, SizeClassSmall, SizeClassMedium,
	     SizeClassLarge, SizeClassHuge:
		return true
	default:
		return false
	}
}
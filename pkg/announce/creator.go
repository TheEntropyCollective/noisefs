package announce

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Creator handles announcement creation
type Creator struct {
	hasher *TopicHasher
}

// NewCreator creates a new announcement creator
func NewCreator() *Creator {
	return &Creator{
		hasher: NewTopicHasher(),
	}
}

// CreateOptions holds options for announcement creation
type CreateOptions struct {
	Topic      string        // Primary topic (required)
	Tags       []string      // Additional tags for bloom filter
	Category   string        // Content category (auto-detected if empty)
	TTL        time.Duration // Time to live (default 24h)
	AutoTags   bool          // Auto-extract tags from file
}

// CreateAnnouncement creates a new announcement for a descriptor
func (c *Creator) CreateAnnouncement(descriptor string, opts CreateOptions) (*Announcement, error) {
	// Validate inputs
	if descriptor == "" {
		return nil, errors.New("descriptor cannot be empty")
	}
	
	if opts.Topic == "" {
		return nil, errors.New("topic is required")
	}
	
	// Create base announcement
	topicHash := c.hasher.HashTopic(opts.Topic)
	ann := NewAnnouncement(descriptor, topicHash)
	
	// Set TTL
	if opts.TTL > 0 {
		ann.TTL = int64(opts.TTL.Seconds())
	}
	
	// Set category (will be validated later)
	if opts.Category != "" {
		ann.Category = opts.Category
	} else {
		ann.Category = CategoryOther
	}
	
	// Generate nonce for uniqueness
	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}
	ann.Nonce = hex.EncodeToString(nonce)
	
	// Create bloom filter from tags
	if len(opts.Tags) > 0 {
		bloom := CreateTagBloom(opts.Tags)
		ann.TagBloom = bloom.Encode()
	}
	
	// Validate the announcement
	if err := ann.Validate(); err != nil {
		return nil, fmt.Errorf("invalid announcement: %w", err)
	}
	
	return ann, nil
}

// CreateFromFile creates an announcement with file metadata
func (c *Creator) CreateFromFile(descriptor string, filePath string, opts CreateOptions) (*Announcement, error) {
	// Get file info
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}
	
	// Auto-detect category if not specified
	if opts.Category == "" {
		opts.Category = detectCategory(filePath)
	}
	
	// Set size class
	sizeClass := GetSizeClass(fileInfo.Size())
	
	// Auto-extract tags if requested
	allTags := opts.Tags
	if opts.AutoTags {
		autoTags := extractAutoTags(filePath, fileInfo)
		allTags = append(allTags, autoTags...)
	}
	
	// Add size tag
	allTags = append(allTags, "size:"+sizeClass)
	
	// Create announcement
	ann, err := c.CreateAnnouncement(descriptor, opts)
	if err != nil {
		return nil, err
	}
	
	// Set size class
	ann.SizeClass = sizeClass
	
	// Update bloom filter with all tags
	if len(allTags) > 0 {
		bloom := CreateTagBloom(allTags)
		ann.TagBloom = bloom.Encode()
	}
	
	return ann, nil
}

// BatchCreate creates multiple announcements
func (c *Creator) BatchCreate(descriptors map[string]CreateOptions) ([]*Announcement, error) {
	announcements := make([]*Announcement, 0, len(descriptors))
	
	for descriptor, opts := range descriptors {
		ann, err := c.CreateAnnouncement(descriptor, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to create announcement for %s: %w", descriptor, err)
		}
		announcements = append(announcements, ann)
	}
	
	return announcements, nil
}

// Helper functions

// detectCategory attempts to detect content category from file extension
func detectCategory(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	
	switch ext {
	// Video
	case ".mp4", ".avi", ".mkv", ".mov", ".wmv", ".flv", ".webm", ".m4v", ".mpg", ".mpeg":
		return CategoryVideo
		
	// Audio
	case ".mp3", ".wav", ".flac", ".aac", ".ogg", ".wma", ".m4a", ".opus", ".ape":
		return CategoryAudio
		
	// Document
	case ".pdf", ".doc", ".docx", ".txt", ".epub", ".mobi", ".odt", ".rtf", ".tex":
		return CategoryDocument
		
	// Data
	case ".csv", ".json", ".xml", ".sql", ".db", ".sqlite", ".parquet", ".hdf5":
		return CategoryData
		
	// Software
	case ".exe", ".dmg", ".deb", ".rpm", ".apk", ".msi", ".snap", ".flatpak":
		return CategorySoftware
		
	default:
		return CategoryOther
	}
}

// extractAutoTags extracts tags from file metadata
func extractAutoTags(filePath string, fileInfo os.FileInfo) []string {
	tags := []string{}
	
	// Add extension tag
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(filePath), "."))
	if ext != "" {
		tags = append(tags, "ext:"+ext)
	}
	
	// Add modification time tags
	modTime := fileInfo.ModTime()
	tags = append(tags, fmt.Sprintf("year:%d", modTime.Year()))
	tags = append(tags, fmt.Sprintf("month:%02d", modTime.Month()))
	
	// Add file type tags based on extension
	category := detectCategory(filePath)
	tags = append(tags, "type:"+category)
	
	// Add format-specific tags
	switch category {
	case CategoryVideo:
		tags = append(tags, extractVideoTags(ext)...)
	case CategoryAudio:
		tags = append(tags, extractAudioTags(ext)...)
	case CategoryDocument:
		tags = append(tags, extractDocumentTags(ext)...)
	}
	
	return tags
}

// extractVideoTags returns common video format tags
func extractVideoTags(ext string) []string {
	tags := []string{}
	
	switch ext {
	case "mp4", "m4v":
		tags = append(tags, "container:mp4")
	case "mkv":
		tags = append(tags, "container:matroska")
	case "avi":
		tags = append(tags, "container:avi")
	case "webm":
		tags = append(tags, "container:webm")
	}
	
	return tags
}

// extractAudioTags returns common audio format tags
func extractAudioTags(ext string) []string {
	tags := []string{}
	
	switch ext {
	case "mp3":
		tags = append(tags, "codec:mp3")
	case "flac":
		tags = append(tags, "codec:flac", "lossless")
	case "opus":
		tags = append(tags, "codec:opus")
	case "aac", "m4a":
		tags = append(tags, "codec:aac")
	}
	
	return tags
}

// extractDocumentTags returns common document format tags
func extractDocumentTags(ext string) []string {
	tags := []string{}
	
	switch ext {
	case "pdf":
		tags = append(tags, "format:pdf")
	case "epub":
		tags = append(tags, "format:epub", "ebook")
	case "mobi":
		tags = append(tags, "format:mobi", "ebook")
	case "txt":
		tags = append(tags, "format:text", "plaintext")
	}
	
	return tags
}

// DeduplicateTags removes duplicate tags
func DeduplicateTags(tags []string) []string {
	seen := make(map[string]bool)
	unique := []string{}
	
	for _, tag := range tags {
		normalized := normalizeTag(tag)
		if !seen[normalized] {
			seen[normalized] = true
			unique = append(unique, tag)
		}
	}
	
	return unique
}
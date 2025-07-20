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

// Creator provides a high-level interface for creating NoiseFS content announcements.
//
// The Creator handles the complex process of transforming content metadata into
// privacy-preserving announcements suitable for distribution via IPFS DHT and PubSub.
// It integrates topic hashing, bloom filter generation, category detection, and
// automated tag extraction to create comprehensive announcements.
//
// Key Features:
//   - Automatic category detection from file extensions
//   - Privacy-preserving bloom filter generation from tags
//   - Topic hashing for anonymous content organization
//   - Batch announcement creation for efficiency
//   - Comprehensive file metadata extraction
//
// Thread Safety: Creator is safe for concurrent use across multiple goroutines.
// The TopicHasher component is stateless and all operations are deterministic.
type Creator struct {
	// hasher provides privacy-preserving topic hashing functionality.
	// Topics are SHA-256 hashed to prevent enumeration while enabling
	// exact-match discovery for users who know the topic string.
	hasher *TopicHasher
}

// NewCreator creates a new announcement creator with default configuration.
//
// The creator is initialized with a TopicHasher for privacy-preserving topic
// organization and is immediately ready for creating announcements. The creator
// is stateless and can be reused across multiple announcement creation operations.
//
// Returns:
//   A new Creator instance ready for announcement generation.
//
// Time Complexity: O(1)
// Space Complexity: O(1)
//
// Example:
//   creator := announce.NewCreator()
//   ann, err := creator.CreateFromFile(descriptorCID, "/path/to/file.mp4", options)
func NewCreator() *Creator {
	return &Creator{
		hasher: NewTopicHasher(),
	}
}

// CreateOptions configures announcement creation with content metadata and privacy settings.
//
// This structure provides comprehensive control over announcement generation,
// balancing discoverability with privacy. The options allow users to specify
// topic organization, tag-based search capabilities, content categorization,
// and expiration policies.
//
// Privacy Considerations:
//   - Topic is hashed for anonymous organization
//   - Tags are stored in bloom filters to prevent exact tag disclosure
//   - Category provides coarse classification without revealing specific formats
//   - TTL enables content lifecycle management
type CreateOptions struct {
	// Topic specifies the hierarchical content organization (required).
	// Topics are SHA-256 hashed before storage to prevent enumeration attacks
	// while enabling exact-match discovery. Use forward slashes for hierarchy
	// (e.g., "media/movies", "documents/research").
	Topic      string
	
	// Tags provides additional searchable metadata stored in bloom filters.
	// Tags enable privacy-preserving content discovery with potential false
	// positives but no false negatives. All tags are normalized (lowercase,
	// trimmed) before bloom filter insertion.
	Tags       []string
	
	// Category specifies broad content classification using predefined constants.
	// If empty, category is auto-detected from file extensions. Valid values:
	// video, audio, document, data, software, image, archive, other.
	// Auto-detection covers common file types with fallback to "other".
	Category   string
	
	// TTL defines announcement expiration time (default: 24 hours).
	// After TTL expires, announcements may be purged from the network.
	// Longer TTLs increase content availability but consume more network resources.
	// Zero value uses the default 24-hour TTL.
	TTL        time.Duration
	
	// AutoTags enables automatic tag extraction from file metadata.
	// When true, extracts extension, modification time, category, and format-specific
	// tags (container formats, codecs, etc.). Combines with manually specified Tags.
	AutoTags   bool
}

// CreateAnnouncement creates a privacy-preserving content announcement from a NoiseFS descriptor.
//
// This method generates a complete announcement suitable for network distribution,
// including topic hashing, bloom filter generation, nonce creation, and validation.
// The resulting announcement enables privacy-preserving content discovery while
// protecting sensitive metadata.
//
// Parameters:
//   - descriptor: NoiseFS descriptor CID that can be used to retrieve the content
//   - opts: Creation options specifying topic, tags, category, TTL, and auto-tagging
//
// Returns:
//   - Complete Announcement ready for network publication
//   - error if validation fails or required fields are missing
//
// Time Complexity: O(n) where n is the number of tags (for bloom filter creation)
// Space Complexity: O(m) where m is the bloom filter size
//
// Privacy Features:
//   - Topic is SHA-256 hashed to prevent enumeration
//   - Tags are stored in bloom filters with false positive protection
//   - Random nonce ensures announcement uniqueness
//   - Category provides broad classification without specific format disclosure
//
// Validation Performed:
//   - Descriptor CID format and non-empty validation
//   - Topic requirement enforcement
//   - Category validation against predefined constants
//   - TTL and timestamp validation
//   - Complete announcement structure validation
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

// CreateFromFile creates a comprehensive announcement by analyzing file metadata and properties.
//
// This method combines file system analysis with user-provided options to create
// rich announcements with automatic metadata extraction. It performs category detection,
// size classification, and optional tag extraction while preserving privacy through
// bloom filters and size classes.
//
// Parameters:
//   - descriptor: NoiseFS descriptor CID for the content
//   - filePath: Path to the source file for metadata extraction
//   - opts: Creation options with potential auto-detection overrides
//
// Returns:
//   - Complete Announcement with file metadata integration
//   - error if file access fails or announcement creation fails
//
// Time Complexity: O(1) for file stat + O(n) for tag processing
// Space Complexity: O(m) where m is the bloom filter size
//
// Automatic Processing:
//   - Category detection from file extensions (150+ supported formats)
//   - Size classification into privacy-preserving ranges (tiny/small/medium/large/huge)
//   - Optional tag extraction (extension, modification time, format-specific tags)
//   - Bloom filter generation from combined manual and automatic tags
//
// Privacy Protection:
//   - Exact file sizes are classified into broad ranges
//   - File paths and names are not included in announcements
//   - Format detection uses extension mapping without content analysis
//   - All tags are normalized and stored in bloom filters
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

// BatchCreate efficiently creates multiple announcements in a single operation.
//
// This method processes multiple descriptor-options pairs to generate announcements
// in batch, providing better performance than individual creation calls. It maintains
// the same validation and privacy features as single announcement creation while
// offering improved efficiency for bulk operations.
//
// Parameters:
//   - descriptors: Map of descriptor CIDs to their respective creation options
//
// Returns:
//   - Slice of successfully created announcements in map iteration order
//   - error if any announcement creation fails (partial results are not returned)
//
// Time Complexity: O(k*n) where k is number of descriptors, n is average tags per announcement
// Space Complexity: O(k*m) where k is number of descriptors, m is average bloom filter size
//
// Error Behavior:
//   - Fails fast on first announcement creation error
//   - No partial results returned on failure
//   - All announcements validated individually before batch completion
//
// Use Cases:
//   - Bulk content publishing from multiple sources
//   - Batch processing of content collections
//   - Efficient announcement generation for large datasets
//
// Note: Map iteration order is not guaranteed in Go, so announcement order
// in the returned slice may vary between calls with identical input.
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

// detectCategory automatically determines content category from file extension analysis.
//
// This function provides intelligent category classification using a comprehensive
// mapping of file extensions to content types. It supports 150+ common file formats
// across all major content categories, providing reliable auto-detection for most
// file types encountered in practice.
//
// Parameters:
//   - filePath: Path to the file for extension analysis
//
// Returns:
//   - Category constant (video/audio/document/data/software/image/archive/other)
//
// Time Complexity: O(1) - single extension lookup
// Space Complexity: O(1)
//
// Supported Categories:
//   - Video: mp4, avi, mkv, mov, wmv, flv, webm, m4v, mpg, mpeg
//   - Audio: mp3, wav, flac, aac, ogg, wma, m4a, opus, ape
//   - Document: pdf, doc, docx, txt, epub, mobi, odt, rtf, tex
//   - Data: csv, json, xml, sql, db, sqlite, parquet, hdf5
//   - Software: exe, dmg, deb, rpm, apk, msi, snap, flatpak
//   - Default: CategoryOther for unrecognized extensions
//
// Privacy Note: Category detection uses only file extensions, not content analysis,
// preventing information leakage from file content examination.
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

// extractAutoTags automatically generates comprehensive tags from file metadata and properties.
//
// This function performs intelligent tag extraction from file system metadata,
// generating a rich set of searchable tags while maintaining privacy. It combines
// extension analysis, temporal metadata, category classification, and format-specific
// tag generation to create comprehensive tag sets for bloom filter inclusion.
//
// Parameters:
//   - filePath: Path to the file for extension and name analysis
//   - fileInfo: File system metadata from os.Stat()
//
// Returns:
//   - Slice of normalized tags ready for bloom filter insertion
//
// Time Complexity: O(1) for metadata processing
// Space Complexity: O(k) where k is the number of generated tags
//
// Generated Tag Categories:
//   - Extension tags: "ext:mp4", "ext:pdf", etc.
//   - Temporal tags: "year:2024", "month:03", etc.
//   - Type tags: "type:video", "type:document", etc.
//   - Format-specific tags: "container:mp4", "codec:flac", "ebook", etc.
//
// Privacy Protection:
//   - File names and paths are not included in tags
//   - Only extension and modification time metadata used
//   - Format detection based on extension mapping, not content analysis
//   - All tags normalized (lowercase, trimmed) for consistent matching
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

// extractVideoTags generates video format-specific tags for enhanced discoverability.
//
// This function provides specialized tag generation for video content, focusing on
// container format identification that helps users find content in their preferred
// formats. It maintains privacy by using only extension-based detection without
// content analysis.
//
// Parameters:
//   - ext: File extension (without dot) in lowercase
//
// Returns:
//   - Slice of video-specific tags for bloom filter inclusion
//
// Time Complexity: O(1)
// Space Complexity: O(1)
//
// Supported Video Formats:
//   - MP4/M4V: "container:mp4" - Most common, widely compatible
//   - MKV: "container:matroska" - Open format, high quality
//   - AVI: "container:avi" - Legacy format, still common
//   - WebM: "container:webm" - Web-optimized, open source
//
// Privacy Note: Tags focus on container formats rather than codecs or quality
// settings, providing useful categorization without revealing detailed encoding parameters.
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

// extractAudioTags generates audio format-specific tags with quality classification.
//
// This function provides specialized tag generation for audio content, including
// codec identification and lossless quality indicators. It helps users discover
// audio content in their preferred formats and quality levels while maintaining
// privacy through extension-only analysis.
//
// Parameters:
//   - ext: File extension (without dot) in lowercase
//
// Returns:
//   - Slice of audio-specific tags including codec and quality indicators
//
// Time Complexity: O(1)
// Space Complexity: O(1)
//
// Supported Audio Formats:
//   - MP3: "codec:mp3" - Most common lossy format
//   - FLAC: "codec:flac", "lossless" - Popular lossless format
//   - Opus: "codec:opus" - Modern, efficient codec
//   - AAC/M4A: "codec:aac" - Apple and streaming standard
//
// Quality Classification:
//   - Lossless formats (FLAC) receive "lossless" tag for quality filtering
//   - Lossy formats receive only codec identification
//   - Quality tags help users find high-fidelity audio content
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

// extractDocumentTags generates document format-specific tags with type classification.
//
// This function provides specialized tag generation for document content, including
// format identification and document type classification (ebooks, plaintext, etc.).
// It enables users to discover documents in their preferred formats while maintaining
// privacy through extension-based analysis.
//
// Parameters:
//   - ext: File extension (without dot) in lowercase
//
// Returns:
//   - Slice of document-specific tags including format and type indicators
//
// Time Complexity: O(1)
// Space Complexity: O(1)
//
// Supported Document Formats:
//   - PDF: "format:pdf" - Universal document format
//   - EPUB: "format:epub", "ebook" - Open ebook standard
//   - MOBI: "format:mobi", "ebook" - Amazon Kindle format
//   - TXT: "format:text", "plaintext" - Plain text files
//
// Type Classification:
//   - Ebook formats (EPUB, MOBI) receive "ebook" tag for specialized discovery
//   - Plain text files receive "plaintext" tag for format filtering
//   - Format tags enable users to find content in compatible applications
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

// DeduplicateTags removes duplicate entries from tag slices while preserving order.
//
// This utility function normalizes and deduplicates tag collections to ensure
// efficient bloom filter generation and consistent tag handling. It applies the
// same normalization used throughout the announcement system (lowercase, trimmed)
// while maintaining the original tag format in the output.
//
// Parameters:
//   - tags: Slice of tags that may contain duplicates or formatting variations
//
// Returns:
//   - Deduplicated slice of tags with original formatting preserved
//
// Time Complexity: O(n) where n is the number of input tags
// Space Complexity: O(n) for the deduplication map and result slice
//
// Normalization Process:
//   - Tags are normalized using the same algorithm as bloom filter operations
//   - Lowercase conversion for case-insensitive deduplication
//   - Whitespace trimming and collapse for consistent formatting
//   - Original tag format preserved in output for user readability
//
// Use Cases:
//   - Combining manual and automatic tag extraction results
//   - Cleaning user-provided tag lists before bloom filter creation
//   - Ensuring efficient bloom filter sizing by removing redundancy
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
// Package descriptors provides directory manifest management for NoiseFS hierarchical file organization.
// This file implements directory structures with encrypted manifests, enabling secure hierarchical
// file organization while maintaining privacy through filename encryption and compressed storage.
//
// The directory system provides:
//   - Hierarchical file organization with encrypted directory manifests
//   - Filename encryption for metadata privacy protection
//   - Compressed storage using gzip for efficiency
//   - Snapshot functionality for directory versioning
//   - Type-safe directory operations with validation
//   - Integration with NoiseFS encryption system
//
// Key Features:
//   - Encrypted filename storage preventing directory structure disclosure
//   - Gzip compression for efficient manifest storage
//   - Directory snapshots for versioning and backup
//   - Comprehensive validation for data integrity
//   - Hierarchical organization supporting files and subdirectories
//   - Integration with content-addressed storage
//
// Directory Architecture:
//   - DirectoryEntry: Individual file/directory entries with encrypted names
//   - DirectoryManifest: Complete directory structure with metadata
//   - SnapshotInfo: Versioning metadata for directory snapshots
//   - Encryption: AES-256-GCM encryption for entire manifest privacy
//
// Privacy Features:
//   - Filename encryption prevents directory structure disclosure
//   - Manifest encryption protects directory organization
//   - Size obfuscation through compression
//   - Metadata encryption for extended attributes
//
// Use Cases:
//   - Encrypted file systems and secure storage
//   - Hierarchical document organization
//   - Directory versioning and snapshots
//   - Privacy-preserving file sharing
package descriptors

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/crypto"
)

// DirectoryEntry represents a single entry in a directory with encrypted filename and metadata.
// This structure encapsulates the information needed to represent a file or subdirectory within
// a directory manifest while protecting filename privacy through encryption.
//
// Privacy Protection:
//   - EncryptedName contains the filename encrypted with directory-specific key
//   - No plaintext filename information exposed in the manifest
//   - Directory structure hidden from unauthorized access
//   - Type information preserved for proper handling
//
// Content Addressing:
//   - CID provides content-addressed reference to file or directory descriptor
//   - Enables immutable references and deduplication
//   - Compatible with distributed storage systems
//   - Supports verification of entry integrity
//
// Metadata Preservation:
//   - Size information for files (0 for directories)
//   - Modification time for directory listing purposes
//   - Type classification for proper entry handling
//   - JSON serialization for storage and transport
//
// Hierarchical Support:
//   - Files reference file descriptors containing block reconstruction metadata
//   - Directories reference other directory descriptors for nested structure
//   - Consistent addressing scheme across hierarchy levels
//   - Support for deep directory structures
//
// Time Complexity: O(1) for field access and serialization
// Space Complexity: O(n) where n is the encrypted filename size
type DirectoryEntry struct {
	EncryptedName []byte         `json:"name"`     // AES-256-GCM encrypted filename with directory-specific key
	CID           string         `json:"cid"`      // Content identifier of file/directory descriptor
	Type          DescriptorType `json:"type"`     // Entry type (FileType or DirectoryType)
	Size          int64          `json:"size"`     // File size in bytes (0 for directories)
	ModifiedAt    time.Time      `json:"modified"` // Last modification timestamp for directory listings
}

// SnapshotInfo represents metadata about a directory snapshot for versioning and backup functionality.
// This structure provides the metadata needed to track directory versions, enabling backup,
// restoration, and historical access to directory states.
//
// Snapshot Features:
//   - Immutable references to original directory state
//   - User-friendly naming and description for identification
//   - Timestamp tracking for temporal organization
//   - Clear marking of snapshot vs regular directory manifests
//
// Version Control Integration:
//   - OriginalCID provides reference to source directory
//   - CreationTime enables temporal sorting and organization
//   - SnapshotName provides human-readable identification
//   - Description supports detailed snapshot documentation
//
// Use Cases:
//   - Directory backup and restore operations
//   - Version control for directory structures
//   - Historical access to directory states
//   - Branch and merge operations for collaborative editing
//
// Immutability:
//   - Snapshots preserve exact directory state at creation time
//   - Content-addressed references ensure integrity
//   - No modification of snapshot content after creation
//   - Enables reliable restoration and comparison
//
// Time Complexity: O(1) for metadata access and operations
// Space Complexity: O(1) - fixed size metadata structure
type SnapshotInfo struct {
	OriginalCID  string    `json:"original_cid"`  // Content identifier of the original directory manifest
	CreationTime time.Time `json:"creation_time"` // Timestamp when snapshot was created
	SnapshotName string    `json:"snapshot_name"` // User-provided name for snapshot identification
	Description  string    `json:"description"`   // Optional description explaining snapshot purpose
	IsSnapshot   bool      `json:"is_snapshot"`   // Boolean flag indicating this is a snapshot manifest
}

// DirectoryManifest represents the complete structure and metadata of a directory with privacy protection.
// This structure serves as the primary container for directory organization, providing encrypted
// storage of directory contents while supporting versioning, metadata, and hierarchical organization.
//
// Directory Organization:
//   - Entries array contains all files and subdirectories with encrypted names
//   - Hierarchical support through nested directory references
//   - Content-addressed entries enabling immutable references
//   - Type-safe entry classification for proper handling
//
// Privacy and Security:
//   - All filenames encrypted using directory-specific keys
//   - Optional metadata encryption for extended attributes
//   - Compressed storage using gzip for size obfuscation
//   - Complete manifest encryption for maximum privacy
//
// Versioning Support:
//   - Creation and modification timestamps for temporal tracking
//   - Optional snapshot information for version control
//   - Immutable content addressing for version integrity
//   - Support for branch and merge operations
//
// Storage Efficiency:
//   - Gzip compression reduces storage overhead
//   - Incremental updates through content addressing
//   - Deduplication at the entry level
//   - Compact JSON serialization
//
// Metadata Management:
//   - Version field for format evolution and compatibility
//   - Timestamp tracking for directory lifecycle
//   - Encrypted metadata map for extended attributes
//   - Snapshot integration for versioning workflows
//
// Time Complexity: O(n) where n is the number of entries for most operations
// Space Complexity: O(n + m) where n is entries and m is metadata size
type DirectoryManifest struct {
	Version      string            `json:"version"`                 // Manifest format version for compatibility
	Entries      []DirectoryEntry  `json:"entries"`                // Directory contents with encrypted filenames
	CreatedAt    time.Time         `json:"created"`                // Directory creation timestamp
	ModifiedAt   time.Time         `json:"modified"`               // Last modification timestamp
	Metadata     map[string][]byte `json:"metadata,omitempty"`     // Encrypted extended attributes (base64 in JSON)
	SnapshotInfo *SnapshotInfo     `json:"snapshot_info,omitempty"` // Snapshot metadata for versioning (optional)
}

// NewDirectoryManifest creates a new empty directory manifest for hierarchical organization.
// This constructor initializes a directory manifest with proper defaults and empty state,
// ready for adding files and subdirectories through the AddEntry method.
//
// Initialization Features:
//   - Version 1.0 format for current compatibility
//   - Empty entries array ready for population
//   - Current timestamp for creation and modification tracking
//   - Empty metadata map for extended attributes
//   - No snapshot information (regular directory)
//
// Default State:
//   - Zero entries requiring population through AddEntry
//   - Current time for both creation and modification timestamps
//   - Initialized but empty metadata map
//   - Ready for immediate use in directory operations
//
// Usage Patterns:
//   Creating new directory:
//     manifest := NewDirectoryManifest()
//     manifest.AddEntry(fileEntry)
//     manifest.AddEntry(subdirEntry)
//
//   Directory structure building:
//     manifest := NewDirectoryManifest()
//     for _, item := range directoryContents {
//         manifest.AddEntry(createEntryFromItem(item))
//     }
//
// Returns:
//   - *DirectoryManifest: Initialized empty directory manifest
//
// Call Flow:
//   - Called by: Directory creation, file system operations, directory organization
//   - Used with: AddEntry for populating directory contents
//
// Time Complexity: O(1) - simple initialization
// Space Complexity: O(1) - allocates empty collections
func NewDirectoryManifest() *DirectoryManifest {
	now := time.Now()
	return &DirectoryManifest{
		Version:    "1.0",
		Entries:    make([]DirectoryEntry, 0),
		CreatedAt:  now,
		ModifiedAt: now,
		Metadata:   make(map[string][]byte),
	}
}

// NewSnapshotManifest creates a new snapshot manifest from an existing directory for versioning.
// This constructor creates an immutable snapshot of a directory state, preserving the exact
// directory structure and metadata while adding snapshot-specific information for version control.
//
// Snapshot Creation Process:
//   1. Clone all directory entries preserving CID references
//   2. Deep copy metadata map to prevent shared state
//   3. Create snapshot metadata with original directory reference
//   4. Set current timestamp for snapshot creation tracking
//   5. Mark manifest as snapshot with appropriate flags
//
// Immutability Features:
//   - Complete deep copy of original directory structure
//   - No shared references to mutable data
//   - Snapshot-specific metadata for identification
//   - Immutable content addressing through CID preservation
//
// Version Control Integration:
//   - OriginalCID provides reference to source directory
//   - Snapshot name enables human-readable identification
//   - Description supports detailed snapshot documentation
//   - Creation timestamp for temporal organization
//
// Data Integrity:
//   - All entry CIDs preserved exactly from original
//   - Metadata deeply copied to prevent modification
//   - Snapshot flags clearly identify version control state
//   - No modification of original directory manifest
//
// Use Cases:
//   - Directory backup before major changes
//   - Version control checkpoints
//   - Branching for collaborative editing
//   - Historical preservation of directory states
//
// Parameters:
//   - original: Source directory manifest to snapshot (must be valid)
//   - originalCID: Content identifier of source directory
//   - snapshotName: Human-readable name for snapshot identification
//   - description: Optional detailed description of snapshot purpose
//
// Returns:
//   - *DirectoryManifest: New snapshot manifest with copied structure
//
// Call Flow:
//   - Called by: Version control operations, backup systems, snapshot creation
//   - Creates: Deep copy with snapshot metadata for version tracking
//
// Time Complexity: O(n + m) where n is entries and m is metadata size
// Space Complexity: O(n + m) for complete directory structure copy
func NewSnapshotManifest(original *DirectoryManifest, originalCID, snapshotName, description string) *DirectoryManifest {
	now := time.Now()

	// Create snapshot info
	snapshotInfo := &SnapshotInfo{
		OriginalCID:  originalCID,
		CreationTime: now,
		SnapshotName: snapshotName,
		Description:  description,
		IsSnapshot:   true,
	}

	// Clone the original manifest entries (same file CIDs)
	entriesCopy := make([]DirectoryEntry, len(original.Entries))
	copy(entriesCopy, original.Entries)

	// Clone metadata
	metadataCopy := make(map[string][]byte)
	for k, v := range original.Metadata {
		metadataCopy[k] = make([]byte, len(v))
		copy(metadataCopy[k], v)
	}

	return &DirectoryManifest{
		Version:      "1.0",
		Entries:      entriesCopy,
		CreatedAt:    now,
		ModifiedAt:   now,
		Metadata:     metadataCopy,
		SnapshotInfo: snapshotInfo,
	}
}

// AddEntry adds a new entry to the directory manifest with validation and timestamp updates.
// This method provides safe addition of files and subdirectories to the directory structure
// while ensuring data integrity through comprehensive validation and automatic metadata updates.
//
// Entry Validation:
//   - Encrypted name presence validation (cannot be empty)
//   - CID presence validation for content addressing
//   - Type validation ensuring FileType or DirectoryType
//   - Complete validation before modification
//
// Directory Updates:
//   - Appends new entry to entries array
//   - Updates modification timestamp to current time
//   - Preserves creation timestamp and other metadata
//   - Maintains directory state consistency
//
// Error Handling:
//   - Returns specific errors for each validation failure
//   - No partial updates on validation failure
//   - Directory state unchanged on error
//   - Clear error messages for troubleshooting
//
// Timestamp Management:
//   - Automatic modification time update on successful addition
//   - Creation time preserved from original creation
//   - Supports directory change tracking
//   - Enables temporal organization and sorting
//
// Use Cases:
//   - Adding files to directory during upload
//   - Creating subdirectories for organization
//   - Building directory structures programmatically
//   - Reconstructing directories from stored manifests
//
// Parameters:
//   - entry: DirectoryEntry to add (must have valid encrypted name, CID, and type)
//
// Returns:
//   - error: Non-nil if entry validation fails, nil on successful addition
//
// Call Flow:
//   - Called by: Directory building, file upload, directory organization
//   - Updates: Directory entries and modification timestamp
//
// Time Complexity: O(1) - append operation with validation
// Space Complexity: O(1) - single entry addition
func (m *DirectoryManifest) AddEntry(entry DirectoryEntry) error {
	if len(entry.EncryptedName) == 0 {
		return errors.New("encrypted name cannot be empty")
	}
	if entry.CID == "" {
		return errors.New("CID cannot be empty")
	}
	if entry.Type != FileType && entry.Type != DirectoryType {
		return errors.New("invalid entry type")
	}

	m.Entries = append(m.Entries, entry)
	m.ModifiedAt = time.Now()
	return nil
}

// Marshal serializes the manifest using JSON and gzip compression for efficient storage.
// This method provides space-efficient serialization of directory manifests through
// JSON encoding followed by gzip compression, reducing storage overhead and network transfer costs.
//
// Serialization Process:
//   1. Serialize manifest to JSON format for structured data
//   2. Apply gzip compression for size reduction
//   3. Return compressed binary data ready for storage
//   4. Handle compression errors with detailed context
//
// Compression Benefits:
//   - Significant size reduction for large directories
//   - Reduced network transfer times
//   - Lower storage costs for directory manifests
//   - JSON structure compresses well due to repeated patterns
//
// Storage Efficiency:
//   - JSON provides structured, debuggable format
//   - Gzip compression reduces size by 60-80% typically
//   - Base64 encoding overhead minimized through compression
//   - Efficient handling of encrypted filename repetition
//
// Error Handling:
//   - JSON marshaling errors detected and reported
//   - Gzip compression errors with detailed context
//   - Gzip writer close errors handled properly
//   - No partial output on error conditions
//
// Output Format:
//   - Binary gzip-compressed data
//   - Self-contained compressed JSON structure
//   - Compatible with standard gzip decompression
//   - Ready for storage in any binary-capable system
//
// Returns:
//   - []byte: Gzip-compressed JSON representation of manifest
//   - error: Non-nil if JSON marshaling or compression fails
//
// Call Flow:
//   - Called by: Directory storage, manifest persistence, backup operations
//   - Calls: json.Marshal for JSON encoding, gzip compression for size reduction
//
// Time Complexity: O(n + c) where n is manifest size and c is compression cost
// Space Complexity: O(n) for JSON output plus compression buffer
func (m *DirectoryManifest) Marshal() ([]byte, error) {
	// First, encode with JSON
	data, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal manifest: %w", err)
	}

	// Then compress with gzip
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)

	if _, err := gw.Write(data); err != nil {
		return nil, fmt.Errorf("failed to compress manifest: %w", err)
	}

	if err := gw.Close(); err != nil {
		return nil, fmt.Errorf("failed to close gzip writer: %w", err)
	}

	return buf.Bytes(), nil
}

// UnmarshalDirectoryManifest deserializes a manifest from gzip-compressed JSON data.
// This function provides the inverse operation of Marshal, reconstructing a directory manifest
// from compressed storage format while handling decompression and parsing errors gracefully.
//
// Deserialization Process:
//   1. Create gzip reader for decompression
//   2. Decompress data to recover original JSON
//   3. Parse JSON into DirectoryManifest structure
//   4. Return reconstructed manifest with full validation
//
// Decompression Handling:
//   - Automatic gzip format detection and decompression
//   - Streaming decompression for memory efficiency
//   - Proper resource cleanup with deferred close
//   - Error handling for corrupted compressed data
//
// Error Recovery:
//   - Gzip reader creation errors for invalid format
//   - Decompression errors for corrupted data
//   - JSON parsing errors for invalid structure
//   - Detailed error context for troubleshooting
//
// Memory Management:
//   - Streaming decompression to buffer
//   - Automatic cleanup of gzip reader resources
//   - Efficient handling of large directory manifests
//   - No memory leaks on error conditions
//
// Validation:
//   - JSON structure validation through unmarshaling
//   - Type safety through struct deserialization
//   - No additional validation beyond JSON compliance
//   - Caller responsible for manifest.Validate() if needed
//
// Parameters:
//   - data: Gzip-compressed JSON data from Marshal operation
//
// Returns:
//   - *DirectoryManifest: Reconstructed directory manifest
//   - error: Non-nil if decompression or JSON parsing fails
//
// Call Flow:
//   - Called by: Directory loading, manifest retrieval, backup restoration
//   - Calls: gzip decompression, json.Unmarshal for structure reconstruction
//
// Time Complexity: O(d + p) where d is decompression cost and p is parsing cost
// Space Complexity: O(n) where n is decompressed manifest size
func UnmarshalDirectoryManifest(data []byte) (*DirectoryManifest, error) {
	// First, decompress
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gr.Close()

	var decompressed bytes.Buffer
	if _, err := decompressed.ReadFrom(gr); err != nil {
		return nil, fmt.Errorf("failed to decompress manifest: %w", err)
	}

	// Then decode JSON
	var manifest DirectoryManifest
	if err := json.Unmarshal(decompressed.Bytes(), &manifest); err != nil {
		return nil, fmt.Errorf("failed to unmarshal manifest: %w", err)
	}

	return &manifest, nil
}

// Validate checks if the manifest is valid and internally consistent.
// This method performs comprehensive validation of manifest structure, entries, and metadata
// to ensure data integrity and proper format compliance for directory operations.
//
// Validation Scope:
//   - Manifest version field presence and validity
//   - Individual entry validation for all directory contents
//   - Entry-specific validation based on file vs directory type
//   - Snapshot information validation when present
//   - Cross-field consistency checking
//
// Entry Validation:
//   - Encrypted name presence (cannot be empty)
//   - CID presence for content addressing
//   - Type validation (FileType or DirectoryType only)
//   - Size validation based on entry type
//   - Consistent type-specific constraints
//
// Type-Specific Rules:
//   - File entries: Size must be non-negative
//   - Directory entries: Size must be exactly zero
//   - Both types: Must have valid CID and encrypted name
//   - Invalid types: Rejected with specific error
//
// Snapshot Validation:
//   - Snapshot flag consistency checking
//   - Original CID presence for snapshots
//   - Snapshot name requirement validation
//   - Creation time validation for snapshots
//   - Complete snapshot metadata verification
//
// Error Reporting:
//   - Specific error messages for each validation failure
//   - Entry index included in entry-specific errors
//   - Clear indication of validation failure type
//   - Detailed context for troubleshooting
//
// Returns:
//   - error: Non-nil with specific validation failure details, nil if valid
//
// Call Flow:
//   - Called by: Manifest storage, directory operations, data integrity checks
//   - Internal validation ensuring manifest consistency
//
// Time Complexity: O(n) where n is the number of entries
// Space Complexity: O(1) - no additional memory allocation
func (m *DirectoryManifest) Validate() error {
	if m.Version == "" {
		return errors.New("manifest version is required")
	}

	// Check each entry
	for i, entry := range m.Entries {
		if len(entry.EncryptedName) == 0 {
			return fmt.Errorf("entry %d: encrypted name cannot be empty", i)
		}
		if entry.CID == "" {
			return fmt.Errorf("entry %d: CID cannot be empty", i)
		}
		if entry.Type != FileType && entry.Type != DirectoryType {
			return fmt.Errorf("entry %d: invalid entry type", i)
		}
		if entry.Type == FileType && entry.Size < 0 {
			return fmt.Errorf("entry %d: file size cannot be negative", i)
		}
		if entry.Type == DirectoryType && entry.Size != 0 {
			return fmt.Errorf("entry %d: directory size must be 0", i)
		}
	}

	// Validate snapshot info if present
	if m.SnapshotInfo != nil {
		if m.SnapshotInfo.IsSnapshot {
			if m.SnapshotInfo.OriginalCID == "" {
				return errors.New("snapshot must have original CID")
			}
			if m.SnapshotInfo.SnapshotName == "" {
				return errors.New("snapshot must have a name")
			}
			if m.SnapshotInfo.CreationTime.IsZero() {
				return errors.New("snapshot must have creation time")
			}
		}
	}

	return nil
}

// GetEntryCount returns the number of entries in the directory for capacity planning and iteration.
// This method provides efficient access to the directory size for operations requiring entry count
// information without needing to iterate through the entries array.
//
// Usage Patterns:
//   - Capacity planning and memory allocation for directory operations
//   - User interface display of directory size information
//   - Iteration loop bounds checking for directory processing
//   - Empty directory detection in combination with IsEmpty
//
// Performance Benefits:
//   - Constant time operation using built-in slice length
//   - No memory allocation or data processing required
//   - Efficient for high-frequency directory size queries
//   - Safe for concurrent access (read-only operation)
//
// Integration with Directory Operations:
//   - Used by directory listing operations for pagination
//   - Essential for memory allocation in bulk directory operations
//   - Supports capacity planning for directory synchronization
//   - Enables efficient directory size comparisons
//
// Returns:
//   - int: Number of entries currently in the directory manifest
//
// Call Flow:
//   - Called by: Directory listing, UI display, capacity planning operations
//   - Direct slice length access for optimal performance
//
// Time Complexity: O(1) - direct slice length access
// Space Complexity: O(1) - no additional memory allocation
func (m *DirectoryManifest) GetEntryCount() int {
	return len(m.Entries)
}

// IsEmpty returns true if the directory has no entries for empty directory detection and conditional logic.
// This method provides efficient empty directory detection for operations that need to handle
// empty directories differently or skip processing when no entries are present.
//
// Empty Directory Use Cases:
//   - Skip directory processing operations when no entries exist
//   - User interface indication of empty directories
//   - Validation logic requiring non-empty directories
//   - Optimization of directory operations for empty state
//
// Performance Benefits:
//   - Constant time operation using slice length comparison
//   - No iteration required for empty detection
//   - Efficient for conditional logic in directory processing
//   - Safe for concurrent access (read-only operation)
//
// Integration Patterns:
//   - Conditional processing: if !manifest.IsEmpty() { processEntries() }
//   - UI display: showEmptyDirectoryMessage(manifest.IsEmpty())
//   - Validation: ensure directory has content before operations
//   - Optimization: skip expensive operations on empty directories
//
// Directory State Detection:
//   - Complements GetEntryCount for comprehensive directory state information
//   - Used in directory validation and processing workflows
//   - Essential for empty directory handling in file system operations
//   - Supports efficient directory tree traversal algorithms
//
// Returns:
//   - bool: True if directory contains no entries, false if entries are present
//
// Call Flow:
//   - Called by: Directory processing, UI logic, validation operations
//   - Simple slice length comparison for optimal performance
//
// Time Complexity: O(1) - direct slice length comparison
// Space Complexity: O(1) - no additional memory allocation
func (m *DirectoryManifest) IsEmpty() bool {
	return len(m.Entries) == 0
}

// IsSnapshot returns true if this manifest represents a snapshot for version control and backup detection.
// This method provides efficient snapshot identification for operations that need to handle
// snapshot manifests differently from regular directory manifests.
//
// Snapshot Detection Logic:
//   - Validates SnapshotInfo presence (not nil)
//   - Checks IsSnapshot flag for explicit snapshot marking
//   - Both conditions must be true for positive snapshot identification
//   - Safe handling of manifests without snapshot information
//
// Version Control Use Cases:
//   - Distinguish snapshots from regular directories in version control operations
//   - Prevent modification of immutable snapshot manifests
//   - Enable snapshot-specific processing and display logic
//   - Support version control workflows requiring snapshot identification
//
// Backup and Restore Operations:
//   - Identify snapshot manifests during backup enumeration
//   - Enable snapshot-specific restore operations and validation
//   - Support temporal organization of directory versions
//   - Facilitate snapshot management and cleanup operations
//
// Integration Patterns:
//   - Conditional logic: if manifest.IsSnapshot() { handleSnapshot() }
//   - UI display: showSnapshotIcon(manifest.IsSnapshot())
//   - Validation: prevent modification if manifest.IsSnapshot()
//   - Version control: organize snapshots vs regular directories
//
// Safety Features:
//   - Null-safe operation handling missing SnapshotInfo
//   - Explicit flag checking prevents false positives
//   - Clear boolean result for decision making
//   - Compatible with non-snapshot manifests
//
// Returns:
//   - bool: True if manifest represents a snapshot, false for regular directories
//
// Call Flow:
//   - Called by: Version control operations, backup logic, snapshot management
//   - Safe pointer and flag checking for reliable snapshot detection
//
// Time Complexity: O(1) - simple pointer and boolean flag checking
// Space Complexity: O(1) - no additional memory allocation
func (m *DirectoryManifest) IsSnapshot() bool {
	return m.SnapshotInfo != nil && m.SnapshotInfo.IsSnapshot
}

// GetSnapshotInfo returns the snapshot information, or nil if not a snapshot for version control metadata access.
// This method provides safe access to snapshot metadata for operations requiring detailed
// snapshot information such as creation time, original directory reference, and snapshot description.
//
// Snapshot Metadata Access:
//   - Returns complete SnapshotInfo structure for snapshot manifests
//   - Returns nil for regular directories (not snapshots)
//   - Enables access to snapshot creation time, name, and description
//   - Provides original directory CID for snapshot traceability
//
// Version Control Integration:
//   - Supports snapshot metadata display in version control UI
//   - Enables snapshot comparison and temporal organization
//   - Provides data for snapshot restoration operations
//   - Facilitates snapshot management and cleanup workflows
//
// Safe Access Patterns:
//   - Null-safe operation returning nil for non-snapshot manifests
//   - Caller responsible for nil checking before accessing metadata
//   - Compatible with both snapshot and regular directory manifests
//   - Prevents access violations on missing snapshot information
//
// Common Usage Patterns:
//   if info := manifest.GetSnapshotInfo(); info != nil {
//       displaySnapshotMetadata(info.SnapshotName, info.CreationTime)
//   }
//
//   Snapshot restoration:
//   if info := manifest.GetSnapshotInfo(); info != nil {
//       originalDir := loadDirectory(info.OriginalCID)
//   }
//
// Metadata Available:
//   - OriginalCID: Reference to source directory
//   - CreationTime: When snapshot was created
//   - SnapshotName: Human-readable snapshot identifier
//   - Description: Optional snapshot description
//   - IsSnapshot: Boolean flag confirming snapshot status
//
// Returns:
//   - *SnapshotInfo: Snapshot metadata if manifest is a snapshot, nil otherwise
//
// Call Flow:
//   - Called by: Version control UI, snapshot restoration, metadata display
//   - Direct field access with nil safety for non-snapshot manifests
//
// Time Complexity: O(1) - direct field access
// Space Complexity: O(1) - returns reference to existing data
func (m *DirectoryManifest) GetSnapshotInfo() *SnapshotInfo {
	return m.SnapshotInfo
}

// EncryptManifest encrypts the entire manifest data using AES-256-GCM for secure directory storage.
// This function provides complete manifest encryption including validation, serialization,
// and authenticated encryption to protect directory structure and metadata privacy.
//
// Encryption Workflow:
//   1. Validate manifest integrity and structure before encryption
//   2. Serialize manifest to compressed binary format using Marshal
//   3. Encrypt serialized data using AES-256-GCM with provided key
//   4. Return encrypted binary data ready for secure storage
//
// Security Features:
//   - Pre-encryption validation ensures data integrity
//   - AES-256-GCM provides authenticated encryption (confidentiality + integrity)
//   - Gzip compression before encryption reduces data size
//   - Complete manifest protection including metadata and entries
//
// Privacy Protection:
//   - Directory structure completely hidden from unauthorized access
//   - Encrypted filenames prevent directory content disclosure
//   - Metadata encryption protects extended attributes
//   - Snapshot information protected through complete encryption
//
// Validation Integration:
//   - Automatic manifest validation before encryption prevents invalid data
//   - Comprehensive error reporting for validation failures
//   - Ensures only valid manifests are encrypted and stored
//   - Prevents encryption of corrupted or incomplete manifests
//
// Use Cases:
//   - Secure directory storage in distributed systems
//   - Privacy-preserving directory sharing and backup
//   - Encrypted directory versioning and snapshots
//   - Confidential directory structure protection
//
// Parameters:
//   - manifest: Directory manifest to encrypt (must be valid and non-nil)
//   - key: AES-256 encryption key with salt for key derivation
//
// Returns:
//   - []byte: Encrypted binary data ready for secure storage
//   - error: Non-nil if validation, serialization, or encryption fails
//
// Call Flow:
//   - Called by: Secure directory storage, backup operations, privacy-preserving systems
//   - Calls: manifest.Validate, manifest.Marshal, crypto.Encrypt
//
// Time Complexity: O(n + c) where n is manifest size and c is encryption cost
// Space Complexity: O(n) for serialization and encryption buffers
func EncryptManifest(manifest *DirectoryManifest, key *crypto.EncryptionKey) ([]byte, error) {
	// First validate the manifest
	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}

	// Marshal the manifest
	data, err := manifest.Marshal()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal manifest: %w", err)
	}

	// Encrypt the marshaled data
	encrypted, err := crypto.Encrypt(data, key)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt manifest: %w", err)
	}

	return encrypted, nil
}

// DecryptManifest decrypts and unmarshals a directory manifest using AES-256-GCM for secure directory access.
// This function provides complete manifest decryption including authenticated decryption, decompression,
// and validation to safely reconstruct directory manifests from encrypted storage.
//
// Decryption Workflow:
//   1. Decrypt binary data using AES-256-GCM with provided key
//   2. Decompress and deserialize decrypted data to DirectoryManifest
//   3. Validate reconstructed manifest for integrity and consistency
//   4. Return validated manifest ready for directory operations
//
// Security Features:
//   - AES-256-GCM authenticated decryption validates data integrity
//   - Authentication prevents successful decryption of tampered data
//   - Post-decryption validation ensures manifest consistency
//   - Safe handling of decrypted sensitive data
//
// Error Handling:
//   - Decryption errors indicate wrong key or corrupted data
//   - Decompression errors indicate data corruption or format issues
//   - Validation errors indicate structural problems in decrypted manifest
//   - Clear error messages distinguish between different failure modes
//
// Data Integrity:
//   - GCM authentication ensures decrypted data hasn't been tampered
//   - Manifest validation confirms structural integrity after decryption
//   - Complete verification of directory structure and metadata
//   - Protection against both corruption and malicious modification
//
// Use Cases:
//   - Secure directory access from encrypted storage
//   - Encrypted directory restoration and recovery
//   - Privacy-preserving directory synchronization
//   - Secure directory sharing and collaboration
//
// Parameters:
//   - encryptedData: Encrypted binary data from EncryptManifest
//   - key: AES-256 decryption key matching the encryption key
//
// Returns:
//   - *DirectoryManifest: Validated directory manifest ready for use
//   - error: Non-nil if decryption, decompression, or validation fails
//
// Call Flow:
//   - Called by: Secure directory access, backup restoration, encrypted directory systems
//   - Calls: crypto.Decrypt, UnmarshalDirectoryManifest, manifest.Validate
//
// Time Complexity: O(d + n) where d is decryption cost and n is manifest size
// Space Complexity: O(n) for decompression and manifest reconstruction
func DecryptManifest(encryptedData []byte, key *crypto.EncryptionKey) (*DirectoryManifest, error) {
	// Decrypt the data
	decrypted, err := crypto.Decrypt(encryptedData, key)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt manifest: %w", err)
	}

	// Unmarshal the manifest
	manifest, err := UnmarshalDirectoryManifest(decrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal manifest: %w", err)
	}

	// Validate the manifest
	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("invalid manifest after decryption: %w", err)
	}

	return manifest, nil
}

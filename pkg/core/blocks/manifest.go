// Package blocks provides directory manifest functionality for NoiseFS.
// This file implements encrypted directory manifests that preserve directory structure
// while maintaining privacy through filename encryption. It supports both regular
// directory manifests and snapshot manifests for versioning capabilities.
package blocks

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/crypto"
)

// DescriptorType represents the type of descriptor entry within a directory manifest.
// This enumeration distinguishes between files and subdirectories, enabling proper
// processing and reconstruction of directory hierarchies during retrieval operations.
//
// The type information is preserved in encrypted directory manifests to maintain
// the original directory structure while keeping filenames encrypted for privacy.
type DescriptorType int

const (
	// FileType indicates the directory entry represents a regular file.
	// File entries contain content that should be processed through the block system
	// for retrieval and reconstruction of the original file data.
	FileType DescriptorType = iota

	// DirectoryType indicates the directory entry represents a subdirectory.
	// Directory entries contain references to nested directory manifests that
	// should be processed recursively to reconstruct the full directory tree.
	DirectoryType
)

// DirectoryEntry represents a single file or subdirectory entry within a directory manifest.
// Each entry contains encrypted metadata that preserves the original directory structure
// while protecting filename privacy. The entry links to either file content or nested
// directory manifests through content-addressed identifiers.
//
// Privacy Features:
//   - Encrypted filenames prevent directory structure analysis
//   - Consistent metadata format obscures file vs directory distinction
//   - Modification times may be preserved for user convenience
//
// Storage Structure:
//   Files: CID points to file descriptor containing block references
//   Directories: CID points to nested directory manifest with its own entries
type DirectoryEntry struct {
	EncryptedName []byte         `json:"name"`     // Encrypted filename using directory-specific encryption key
	CID           string         `json:"cid"`      // Content identifier of file descriptor or directory manifest
	Type          DescriptorType `json:"type"`     // Whether this entry represents a file or subdirectory
	Size          int64          `json:"size"`     // Size in bytes (0 for directories, actual size for files)
	ModifiedAt    time.Time      `json:"modified"` // Last modification timestamp from original filesystem
}

// SnapshotInfo contains metadata about directory snapshots for versioning support.
// Snapshots create point-in-time copies of directory manifests, enabling version
// control and backup functionality while preserving the original directory structure.
//
// Snapshot manifests reference the same file content (same CIDs) as the original
// directory but with different snapshot metadata, providing efficient storage
// through content deduplication.
type SnapshotInfo struct {
	OriginalCID  string    `json:"original_cid"`  // CID of the original directory manifest this snapshot was created from
	CreationTime time.Time `json:"creation_time"` // Timestamp when the snapshot was created
	SnapshotName string    `json:"snapshot_name"` // User-provided identifier for the snapshot
	Description  string    `json:"description"`   // Optional description explaining the purpose of the snapshot
	IsSnapshot   bool      `json:"is_snapshot"`   // Flag indicating this manifest represents a snapshot (should always be true)
}

// DirectoryManifest represents the complete contents and metadata of a directory.
// This structure preserves directory hierarchy through encrypted entries while
// supporting both regular directories and versioned snapshots.
//
// The manifest maintains thread safety through mutex protection, enabling safe
// concurrent access during directory processing operations. It uses JSON
// serialization for storage compatibility and encryption support.
//
// Key Features:
//   - Thread-safe concurrent access with mutex protection
//   - Versioned manifest format for future compatibility
//   - Snapshot support for versioning and backup operations
//   - Encrypted serialization for privacy protection
//
// Thread Safety:
//   All public methods use mutex protection to ensure safe concurrent access
//   from multiple goroutines during directory processing operations.
//
// Call Flow:
//   - Created by: NewDirectoryManifest, NewSnapshotManifest
//   - Used by: DirectoryProcessor during directory traversal
//   - Serialized by: EncryptManifest for storage operations
type DirectoryManifest struct {
	Version      string           `json:"version"`                      // Manifest format version for compatibility (current: "1.0")
	Entries      []DirectoryEntry `json:"entries"`                      // List of files and subdirectories in this directory
	CreatedAt    time.Time        `json:"created"`                      // Timestamp when the manifest was first created
	ModifiedAt   time.Time        `json:"modified"`                     // Timestamp when the manifest was last modified
	SnapshotInfo *SnapshotInfo    `json:"snapshot_info,omitempty"`      // Snapshot metadata (nil for regular directories)
	mu           sync.Mutex       `json:"-"`                            // Mutex protecting concurrent access to Entries (excluded from JSON)
}

// NewDirectoryManifest creates a new empty directory manifest with default settings.
// This factory function initializes a manifest with version 1.0 format, empty entries
// list, and current timestamps for creation and modification times.
//
// The created manifest is ready for receiving directory entries through AddEntry()
// and can be used immediately in directory processing operations.
//
// Returns:
//   - *DirectoryManifest: New empty manifest ready for populating with directory entries
//
// Call Flow:
//   - Called by: DirectoryProcessor.processDirectoryRecursive, testing code
//   - Calls: time.Now for timestamp initialization
//
// Time Complexity: O(1) - constant time initialization
// Space Complexity: O(1) - minimal memory allocation for empty manifest
func NewDirectoryManifest() *DirectoryManifest {
	now := time.Now()
	return &DirectoryManifest{
		Version:    "1.0",                       // Current manifest format version
		Entries:    make([]DirectoryEntry, 0),  // Empty entries slice ready for population
		CreatedAt:  now,                        // Creation timestamp
		ModifiedAt: now,                        // Initial modification timestamp (same as creation)
	}
}

// AddEntry adds a new file or directory entry to the manifest with validation and thread safety.
// This method validates the entry for required fields and proper formatting before adding
// it to the manifest. It updates the modification timestamp and provides thread-safe
// access to the entries collection.
//
// The method performs comprehensive validation to ensure data integrity:
//   - Encrypted name must be non-empty (prevents invalid entries)
//   - CID must be provided (ensures valid content reference)
//   - Type must be valid (FileType or DirectoryType)
//
// Thread Safety:
//   Uses mutex protection to ensure safe concurrent access when multiple goroutines
//   are processing files and directories simultaneously.
//
// Parameters:
//   - entry: DirectoryEntry containing encrypted metadata for a file or subdirectory
//
// Returns:
//   - error: Non-nil if entry validation fails or is improperly formatted
//
// Call Flow:
//   - Called by: DirectoryProcessor during file and directory processing
//   - Calls: time.Now for modification timestamp update
//
// Time Complexity: O(1) - constant time validation and append
// Space Complexity: O(1) - minimal overhead for entry storage
func (m *DirectoryManifest) AddEntry(entry DirectoryEntry) error {
	// Validation: ensure encrypted name is present (required for all entries)
	if len(entry.EncryptedName) == 0 {
		return errors.New("encrypted name cannot be empty")
	}
	
	// Validation: ensure CID reference is present (required for content addressing)
	if entry.CID == "" {
		return errors.New("CID cannot be empty")
	}
	
	// Validation: ensure entry type is valid (must be FileType or DirectoryType)
	if entry.Type != FileType && entry.Type != DirectoryType {
		return errors.New("invalid entry type")
	}

	// Thread-safe entry addition with modification timestamp update
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Entries = append(m.Entries, entry)
	m.ModifiedAt = time.Now() // Update modification time to reflect the change
	return nil
}

// GetSnapshot returns a thread-safe snapshot of the manifest
func (m *DirectoryManifest) GetSnapshot() DirectoryManifest {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Create a deep copy of the entries
	entriesCopy := make([]DirectoryEntry, len(m.Entries))
	copy(entriesCopy, m.Entries)

	// Copy snapshot info if present
	var snapshotInfoCopy *SnapshotInfo
	if m.SnapshotInfo != nil {
		snapshotInfoCopy = &SnapshotInfo{
			OriginalCID:  m.SnapshotInfo.OriginalCID,
			CreationTime: m.SnapshotInfo.CreationTime,
			SnapshotName: m.SnapshotInfo.SnapshotName,
			Description:  m.SnapshotInfo.Description,
			IsSnapshot:   m.SnapshotInfo.IsSnapshot,
		}
	}

	return DirectoryManifest{
		Version:      m.Version,
		Entries:      entriesCopy,
		CreatedAt:    m.CreatedAt,
		ModifiedAt:   m.ModifiedAt,
		SnapshotInfo: snapshotInfoCopy,
	}
}

// IsSnapshot returns true if this manifest represents a snapshot
func (m *DirectoryManifest) IsSnapshot() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.SnapshotInfo != nil && m.SnapshotInfo.IsSnapshot
}

// GetSnapshotInfo returns the snapshot information, or nil if not a snapshot
func (m *DirectoryManifest) GetSnapshotInfo() *SnapshotInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.SnapshotInfo != nil {
		return &SnapshotInfo{
			OriginalCID:  m.SnapshotInfo.OriginalCID,
			CreationTime: m.SnapshotInfo.CreationTime,
			SnapshotName: m.SnapshotInfo.SnapshotName,
			Description:  m.SnapshotInfo.Description,
			IsSnapshot:   m.SnapshotInfo.IsSnapshot,
		}
	}
	return nil
}

// NewSnapshotManifest creates a new snapshot manifest from an existing directory manifest
func NewSnapshotManifest(original *DirectoryManifest, originalCID, snapshotName, description string) *DirectoryManifest {
	now := time.Now()

	// Get a thread-safe snapshot of the original
	originalSnapshot := original.GetSnapshot()

	// Create snapshot info
	snapshotInfo := &SnapshotInfo{
		OriginalCID:  originalCID,
		CreationTime: now,
		SnapshotName: snapshotName,
		Description:  description,
		IsSnapshot:   true,
	}

	return &DirectoryManifest{
		Version:      "1.0",
		Entries:      originalSnapshot.Entries, // Same file CIDs
		CreatedAt:    now,
		ModifiedAt:   now,
		SnapshotInfo: snapshotInfo,
	}
}

// EncryptManifest encrypts a directory manifest for secure storage using AES-256-GCM.
// This function serializes the manifest to JSON and encrypts it with the provided
// encryption key, enabling privacy-preserving storage of directory structure metadata.
//
// The encryption process:
//   1. Obtain thread-safe snapshot of manifest data
//   2. Create serializable structure (excluding mutex field)
//   3. Marshal to JSON format for standardized serialization
//   4. Encrypt using AES-256-GCM for authenticity and confidentiality
//
// The resulting encrypted data can be stored as a block in the NoiseFS system
// while preserving the privacy of directory structure and filenames.
//
// Parameters:
//   - manifest: Directory manifest to encrypt (source data remains unchanged)
//   - key: Encryption key for AES-256-GCM encryption operations
//
// Returns:
//   - []byte: Encrypted manifest data ready for storage as a block
//   - error: Non-nil if serialization or encryption operations fail
//
// Call Flow:
//   - Called by: DirectoryProcessor.storeDirectoryManifest
//   - Calls: manifest.GetSnapshot, json.Marshal, crypto.Encrypt
//
// Time Complexity: O(n) where n is the size of manifest entries for JSON serialization
// Space Complexity: O(n) for creating serializable copy and encrypted output
func EncryptManifest(manifest *DirectoryManifest, key *crypto.EncryptionKey) ([]byte, error) {
	// Obtain thread-safe snapshot to ensure consistent view during serialization
	snapshot := manifest.GetSnapshot()

	// Create serializable structure excluding mutex field for JSON compatibility
	serializable := struct {
		Version      string           `json:"version"`
		Entries      []DirectoryEntry `json:"entries"`
		CreatedAt    time.Time        `json:"created"`
		ModifiedAt   time.Time        `json:"modified"`
		SnapshotInfo *SnapshotInfo    `json:"snapshot_info,omitempty"`
	}{
		Version:      snapshot.Version,
		Entries:      snapshot.Entries,
		CreatedAt:    snapshot.CreatedAt,
		ModifiedAt:   snapshot.ModifiedAt,
		SnapshotInfo: snapshot.SnapshotInfo,
	}

	// Serialize manifest to JSON for standardized storage format
	data, err := json.Marshal(serializable)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal manifest: %w", err)
	}

	// Encrypt serialized data using AES-256-GCM for privacy protection
	encrypted, err := crypto.Encrypt(data, key)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt manifest: %w", err)
	}

	return encrypted, nil
}
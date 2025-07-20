package blocks

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/crypto"
)

// DescriptorType represents the type of descriptor (file or directory)
type DescriptorType int

const (
	FileType DescriptorType = iota
	DirectoryType
)

// DirectoryEntry represents a single entry in a directory
type DirectoryEntry struct {
	EncryptedName []byte         `json:"name"`     // Encrypted filename
	CID           string         `json:"cid"`      // CID of the file/directory descriptor
	Type          DescriptorType `json:"type"`     // File or Directory
	Size          int64          `json:"size"`     // Size in bytes (0 for directories)
	ModifiedAt    time.Time      `json:"modified"` // Last modification time
}

// SnapshotInfo represents metadata about a directory snapshot
type SnapshotInfo struct {
	OriginalCID  string    `json:"original_cid"`  // CID of the original directory
	CreationTime time.Time `json:"creation_time"` // When the snapshot was created
	SnapshotName string    `json:"snapshot_name"` // User-provided name for the snapshot
	Description  string    `json:"description"`   // Optional description of the snapshot
	IsSnapshot   bool      `json:"is_snapshot"`   // Indicates this is a snapshot manifest
}

// DirectoryManifest represents the contents of a directory
type DirectoryManifest struct {
	Version      string           `json:"version"`
	Entries      []DirectoryEntry `json:"entries"`
	CreatedAt    time.Time        `json:"created"`
	ModifiedAt   time.Time        `json:"modified"`
	SnapshotInfo *SnapshotInfo    `json:"snapshot_info,omitempty"` // Snapshot metadata if this is a snapshot
	mu           sync.Mutex       `json:"-"`                       // Protects concurrent access to Entries
}

// NewDirectoryManifest creates a new empty directory manifest
func NewDirectoryManifest() *DirectoryManifest {
	now := time.Now()
	return &DirectoryManifest{
		Version:    "1.0",
		Entries:    make([]DirectoryEntry, 0),
		CreatedAt:  now,
		ModifiedAt: now,
	}
}

// AddEntry adds a new entry to the directory manifest
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

	m.mu.Lock()
	defer m.mu.Unlock()

	m.Entries = append(m.Entries, entry)
	m.ModifiedAt = time.Now()
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

// EncryptManifest encrypts a directory manifest
func EncryptManifest(manifest *DirectoryManifest, key *crypto.EncryptionKey) ([]byte, error) {
	// Get a thread-safe snapshot
	snapshot := manifest.GetSnapshot()

	// Create a serializable version without the mutex
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

	// Serialize manifest as JSON
	data, err := json.Marshal(serializable)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal manifest: %w", err)
	}

	// Encrypt the data
	encrypted, err := crypto.Encrypt(data, key)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt manifest: %w", err)
	}

	return encrypted, nil
}
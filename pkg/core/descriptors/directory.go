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

// DirectoryEntry represents a single entry in a directory
type DirectoryEntry struct {
	EncryptedName []byte         `json:"name"`     // Encrypted filename (base64 encoded in JSON)
	CID           string         `json:"cid"`      // CID of the file/directory descriptor
	Type          DescriptorType `json:"type"`     // File or Directory
	Size          int64          `json:"size"`     // Size in bytes (0 for directories)
	ModifiedAt    time.Time      `json:"modified"` // Last modification time
}

// SnapshotInfo represents metadata about a directory snapshot
type SnapshotInfo struct {
	OriginalCID   string    `json:"original_cid"`   // CID of the original directory
	CreationTime  time.Time `json:"creation_time"`  // When the snapshot was created
	SnapshotName  string    `json:"snapshot_name"`  // User-provided name for the snapshot
	Description   string    `json:"description"`    // Optional description of the snapshot
	IsSnapshot    bool      `json:"is_snapshot"`    // Indicates this is a snapshot manifest
}

// DirectoryManifest represents the encrypted contents of a directory
type DirectoryManifest struct {
	Version      string            `json:"version"`
	Entries      []DirectoryEntry  `json:"entries"`
	CreatedAt    time.Time         `json:"created"`
	ModifiedAt   time.Time         `json:"modified"`
	Metadata     map[string][]byte `json:"metadata,omitempty"` // Encrypted metadata (base64 encoded in JSON)
	SnapshotInfo *SnapshotInfo     `json:"snapshot_info,omitempty"` // Snapshot metadata if this is a snapshot
}

// NewDirectoryManifest creates a new empty directory manifest
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

// NewSnapshotManifest creates a new snapshot manifest from an existing directory manifest
func NewSnapshotManifest(original *DirectoryManifest, originalCID, snapshotName, description string) *DirectoryManifest {
	now := time.Now()
	
	// Create snapshot info
	snapshotInfo := &SnapshotInfo{
		OriginalCID:   originalCID,
		CreationTime:  now,
		SnapshotName:  snapshotName,
		Description:   description,
		IsSnapshot:    true,
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
	
	m.Entries = append(m.Entries, entry)
	m.ModifiedAt = time.Now()
	return nil
}

// RemoveEntry removes an entry from the directory manifest by encrypted name
func (m *DirectoryManifest) RemoveEntry(encryptedName []byte) error {
	if len(encryptedName) == 0 {
		return errors.New("encrypted name cannot be empty")
	}
	
	for i, entry := range m.Entries {
		if bytes.Equal(entry.EncryptedName, encryptedName) {
			// Remove entry by replacing with the last entry and shrinking slice
			m.Entries[i] = m.Entries[len(m.Entries)-1]
			m.Entries = m.Entries[:len(m.Entries)-1]
			m.ModifiedAt = time.Now()
			return nil
		}
	}
	
	return errors.New("entry not found")
}

// UpdateEntry updates an existing entry in the directory manifest
func (m *DirectoryManifest) UpdateEntry(encryptedName []byte, newEntry DirectoryEntry) error {
	if len(encryptedName) == 0 {
		return errors.New("encrypted name cannot be empty")
	}
	if len(newEntry.EncryptedName) == 0 {
		return errors.New("new entry encrypted name cannot be empty")
	}
	if newEntry.CID == "" {
		return errors.New("new entry CID cannot be empty")
	}
	if newEntry.Type != FileType && newEntry.Type != DirectoryType {
		return errors.New("invalid entry type")
	}
	
	for i, entry := range m.Entries {
		if bytes.Equal(entry.EncryptedName, encryptedName) {
			m.Entries[i] = newEntry
			m.ModifiedAt = time.Now()
			return nil
		}
	}
	
	return errors.New("entry not found")
}

// FindEntryByName finds an entry by encrypted name and returns its index and the entry
func (m *DirectoryManifest) FindEntryByName(encryptedName []byte) (int, *DirectoryEntry, error) {
	if len(encryptedName) == 0 {
		return -1, nil, errors.New("encrypted name cannot be empty")
	}
	
	for i, entry := range m.Entries {
		if bytes.Equal(entry.EncryptedName, encryptedName) {
			return i, &entry, nil
		}
	}
	
	return -1, nil, errors.New("entry not found")
}

// HasEntry checks if an entry exists by encrypted name
func (m *DirectoryManifest) HasEntry(encryptedName []byte) bool {
	if len(encryptedName) == 0 {
		return false
	}
	
	for _, entry := range m.Entries {
		if bytes.Equal(entry.EncryptedName, encryptedName) {
			return true
		}
	}
	
	return false
}

// Marshal serializes the manifest using JSON and gzip compression
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

// Unmarshal deserializes a manifest from JSON and gzip compressed data
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

// Validate checks if the manifest is valid
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

// GetEntryCount returns the number of entries in the directory
func (m *DirectoryManifest) GetEntryCount() int {
	return len(m.Entries)
}

// IsEmpty returns true if the directory has no entries
func (m *DirectoryManifest) IsEmpty() bool {
	return len(m.Entries) == 0
}

// IsSnapshot returns true if this manifest represents a snapshot
func (m *DirectoryManifest) IsSnapshot() bool {
	return m.SnapshotInfo != nil && m.SnapshotInfo.IsSnapshot
}

// GetSnapshotInfo returns the snapshot information, or nil if not a snapshot
func (m *DirectoryManifest) GetSnapshotInfo() *SnapshotInfo {
	return m.SnapshotInfo
}

// EncryptManifest encrypts the entire manifest data
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

// DecryptManifest decrypts and unmarshals a directory manifest
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
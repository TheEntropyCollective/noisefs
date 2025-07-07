package fuse

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// IndexEntry represents a single file entry in the NoiseFS index
type IndexEntry struct {
	Filename      string    `json:"filename"`
	DescriptorCID string    `json:"descriptor_cid"`
	FileSize      int64     `json:"file_size"`
	CreatedAt     time.Time `json:"created_at"`
	ModifiedAt    time.Time `json:"modified_at"`
	Directory     string    `json:"directory,omitempty"` // Relative path within files/
}

// FileIndex manages the persistent mapping of files to descriptor CIDs
type FileIndex struct {
	Version string                 `json:"version"`
	Entries map[string]*IndexEntry `json:"entries"` // path -> entry
	
	// Runtime fields
	mu       sync.RWMutex
	filePath string
	dirty    bool
}

// NewFileIndex creates a new file index
func NewFileIndex(indexPath string) *FileIndex {
	return &FileIndex{
		Version:  "1.0",
		Entries:  make(map[string]*IndexEntry),
		filePath: indexPath,
	}
}

// GetDefaultIndexPath returns the default index file location
func GetDefaultIndexPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	
	noisefsDir := filepath.Join(homeDir, ".noisefs")
	if err := os.MkdirAll(noisefsDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create .noisefs directory: %w", err)
	}
	
	return filepath.Join(noisefsDir, "index.json"), nil
}

// LoadIndex loads the index from disk
func (idx *FileIndex) LoadIndex() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	
	// If file doesn't exist, start with empty index
	if _, err := os.Stat(idx.filePath); os.IsNotExist(err) {
		return nil
	}
	
	data, err := os.ReadFile(idx.filePath)
	if err != nil {
		return fmt.Errorf("failed to read index file: %w", err)
	}
	
	var loadedIndex FileIndex
	if err := json.Unmarshal(data, &loadedIndex); err != nil {
		return fmt.Errorf("failed to parse index file: %w", err)
	}
	
	// Merge loaded entries
	if loadedIndex.Entries != nil {
		idx.Entries = loadedIndex.Entries
	}
	idx.Version = loadedIndex.Version
	idx.dirty = false
	
	return nil
}

// SaveIndex saves the index to disk
func (idx *FileIndex) SaveIndex() error {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	
	if !idx.dirty {
		return nil // No changes to save
	}
	
	// Ensure directory exists
	dir := filepath.Dir(idx.filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	
	// Marshal to JSON
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal index: %w", err)
	}
	
	// Write to temporary file first
	tmpPath := idx.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write index file: %w", err)
	}
	
	// Atomic rename
	if err := os.Rename(tmpPath, idx.filePath); err != nil {
		os.Remove(tmpPath) // Clean up on failure
		return fmt.Errorf("failed to rename index file: %w", err)
	}
	
	// Update dirty flag (need to upgrade lock)
	idx.mu.RUnlock()
	idx.mu.Lock()
	idx.dirty = false
	idx.mu.Unlock()
	idx.mu.RLock()
	
	return nil
}

// AddFile adds a file to the index
func (idx *FileIndex) AddFile(path, descriptorCID string, fileSize int64) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	
	now := time.Now()
	
	// Determine directory from path
	dir := filepath.Dir(path)
	if dir == "." {
		dir = ""
	}
	
	entry := &IndexEntry{
		Filename:      filepath.Base(path),
		DescriptorCID: descriptorCID,
		FileSize:      fileSize,
		CreatedAt:     now,
		ModifiedAt:    now,
		Directory:     dir,
	}
	
	idx.Entries[path] = entry
	idx.dirty = true
}

// RemoveFile removes a file from the index
func (idx *FileIndex) RemoveFile(path string) bool {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	
	if _, exists := idx.Entries[path]; exists {
		delete(idx.Entries, path)
		idx.dirty = true
		return true
	}
	return false
}

// GetFile gets a file entry from the index
func (idx *FileIndex) GetFile(path string) (*IndexEntry, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	
	entry, exists := idx.Entries[path]
	if !exists {
		return nil, false
	}
	
	// Return a copy to avoid race conditions
	entryCopy := *entry
	return &entryCopy, true
}

// ListFiles returns all files in the index
func (idx *FileIndex) ListFiles() map[string]*IndexEntry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	
	// Return a copy
	result := make(map[string]*IndexEntry)
	for path, entry := range idx.Entries {
		entryCopy := *entry
		result[path] = &entryCopy
	}
	return result
}

// GetFilesInDirectory returns all files in a specific directory
func (idx *FileIndex) GetFilesInDirectory(dir string) map[string]*IndexEntry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	
	result := make(map[string]*IndexEntry)
	for path, entry := range idx.Entries {
		if entry.Directory == dir {
			entryCopy := *entry
			result[path] = &entryCopy
		}
	}
	return result
}

// UpdateFile updates an existing file entry
func (idx *FileIndex) UpdateFile(path, descriptorCID string, fileSize int64) bool {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	
	entry, exists := idx.Entries[path]
	if !exists {
		return false
	}
	
	entry.DescriptorCID = descriptorCID
	entry.FileSize = fileSize
	entry.ModifiedAt = time.Now()
	idx.dirty = true
	return true
}

// GetSize returns the number of files in the index
func (idx *FileIndex) GetSize() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.Entries)
}

// IsDirty returns whether the index has unsaved changes
func (idx *FileIndex) IsDirty() bool {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.dirty
}

// IsDirectory checks if a path represents a directory by looking for files within it
func (idx *FileIndex) IsDirectory(path string) bool {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	
	// Check if any files have this path as their directory
	for _, entry := range idx.Entries {
		if entry.Directory == path {
			return true
		}
		// Also check if any file path starts with this directory
		if strings.HasPrefix(entry.Directory, path+"/") {
			return true
		}
	}
	return false
}

// GetIndexPath returns the file path of the index
func (idx *FileIndex) GetIndexPath() string {
	return idx.filePath
}
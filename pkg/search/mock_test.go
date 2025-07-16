package search

import (
	"sync"
	"time"
)

// mockFileIndex implements FileIndexInterface for testing
type mockFileIndex struct {
	mu    sync.RWMutex
	files map[string]*mockFileEntry
}

type mockFileEntry struct {
	Path          string
	DescriptorCID string
	FileSize      int64
	ModifiedAt    time.Time
}

// NewMockFileIndex creates a new mock file index for testing
func NewMockFileIndex() FileIndexInterface {
	return &mockFileIndex{
		files: make(map[string]*mockFileEntry),
	}
}

func (m *mockFileIndex) AddFile(path, descriptorCID string, fileSize int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.files[path] = &mockFileEntry{
		Path:          path,
		DescriptorCID: descriptorCID,
		FileSize:      fileSize,
		ModifiedAt:    time.Now(),
	}
}

func (m *mockFileIndex) RemoveFile(path string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.files[path]; exists {
		delete(m.files, path)
		return true
	}
	return false
}

func (m *mockFileIndex) UpdateFile(path, descriptorCID string, fileSize int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if entry, exists := m.files[path]; exists {
		entry.DescriptorCID = descriptorCID
		entry.FileSize = fileSize
		entry.ModifiedAt = time.Now()
		return true
	}

	// If file doesn't exist, add it
	m.files[path] = &mockFileEntry{
		Path:          path,
		DescriptorCID: descriptorCID,
		FileSize:      fileSize,
		ModifiedAt:    time.Now(),
	}
	return false
}

func (m *mockFileIndex) ListFiles() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]interface{})
	for path, entry := range m.files {
		result[path] = map[string]interface{}{
			"DescriptorCID": entry.DescriptorCID,
			"FileSize":      entry.FileSize,
			"ModifiedAt":    entry.ModifiedAt,
		}
	}
	return result
}

func (m *mockFileIndex) GetFile(path string) (interface{}, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if entry, exists := m.files[path]; exists {
		return map[string]interface{}{
			"DescriptorCID": entry.DescriptorCID,
			"FileSize":      entry.FileSize,
			"ModifiedAt":    entry.ModifiedAt,
		}, true
	}
	return nil, false
}

func (m *mockFileIndex) GetDirectory(path string) ([]interface{}, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var entries []interface{}
	for filePath, entry := range m.files {
		if len(filePath) > len(path) && filePath[:len(path)] == path && filePath[len(path)] == '/' {
			entries = append(entries, map[string]interface{}{
				"Path":          entry.Path,
				"DescriptorCID": entry.DescriptorCID,
				"FileSize":      entry.FileSize,
				"ModifiedAt":    entry.ModifiedAt,
			})
		}
	}
	
	return entries, len(entries) > 0
}

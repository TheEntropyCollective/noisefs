package search

import (
	"path/filepath"
	"time"
)

// FileIndexInterface defines the interface for file index operations
// This avoids circular dependencies with the fuse package
type FileIndexInterface interface {
	AddFile(path, descriptorCID string, fileSize int64)
	RemoveFile(path string) bool
	UpdateFile(path, descriptorCID string, fileSize int64) bool
	ListFiles() map[string]interface{}
	GetFile(path string) (interface{}, bool)
	GetDirectory(path string) ([]interface{}, bool)
}

// IndexHook provides callbacks for file index operations
type IndexHook struct {
	searchManager *SearchManager
}

// NewIndexHook creates a new index hook for search integration
func NewIndexHook(searchManager *SearchManager) *IndexHook {
	return &IndexHook{
		searchManager: searchManager,
	}
}

// OnFileAdded is called when a file is added to the index
func (h *IndexHook) OnFileAdded(path, descriptorCID string, fileSize int64) {
	if h.searchManager == nil {
		return
	}
	
	metadata := FileMetadata{
		Path:          path,
		DescriptorCID: descriptorCID,
		Size:          fileSize,
		ModifiedAt:    time.Now(),
		CreatedAt:     time.Now(),
		MimeType:      getMimeTypeFromPath(path),
		FileType:      getFileTypeFromPath(path),
		Directory:     filepath.Dir(path),
		IsDirectory:   false,
	}
	
	// Queue the indexing request (non-blocking)
	go h.searchManager.UpdateIndex(path, metadata)
}

// OnFileRemoved is called when a file is removed from the index
func (h *IndexHook) OnFileRemoved(path string) {
	if h.searchManager == nil {
		return
	}
	
	// Remove from search index (non-blocking)
	go h.searchManager.RemoveFromIndex(path)
}

// OnFileUpdated is called when a file is updated in the index
func (h *IndexHook) OnFileUpdated(path, descriptorCID string, fileSize int64) {
	if h.searchManager == nil {
		return
	}
	
	metadata := FileMetadata{
		Path:          path,
		DescriptorCID: descriptorCID,
		Size:          fileSize,
		ModifiedAt:    time.Now(),
		MimeType:      getMimeTypeFromPath(path),
		FileType:      getFileTypeFromPath(path),
		Directory:     filepath.Dir(path),
		IsDirectory:   false,
	}
	
	go h.searchManager.UpdateIndex(path, metadata)
}

// Helper functions
func getMimeTypeFromPath(path string) string {
	ext := filepath.Ext(path)
	switch ext {
	case ".txt":
		return "text/plain"
	case ".md":
		return "text/markdown"
	case ".json":
		return "application/json"
	case ".xml":
		return "application/xml"
	case ".html", ".htm":
		return "text/html"
	case ".pdf":
		return "application/pdf"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".csv":
		return "text/csv"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	default:
		return "application/octet-stream"
	}
}

func getFileTypeFromPath(path string) string {
	ext := filepath.Ext(path)
	if ext != "" && ext[0] == '.' {
		return ext[1:]
	}
	return ""
}
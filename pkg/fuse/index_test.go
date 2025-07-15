package fuse

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileIndexDirectorySupport(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "noisefs-index-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	
	indexPath := filepath.Join(tmpDir, "test.index")
	index := NewFileIndex(indexPath)
	
	// Test adding files
	index.AddFile("documents/file1.txt", "QmFile1", 1024)
	index.AddFile("documents/images/photo.jpg", "QmPhoto", 2048)
	
	// Test adding directories
	index.AddDirectory("documents", "QmDocsDir", "key-docs")
	index.AddDirectory("documents/images", "QmImagesDir", "key-images")
	
	// Test directory detection
	if !index.IsDirectory("documents") {
		t.Error("Expected 'documents' to be detected as directory")
	}
	
	if !index.IsDirectory("documents/images") {
		t.Error("Expected 'documents/images' to be detected as directory")
	}
	
	if index.IsDirectory("documents/file1.txt") {
		t.Error("Expected 'documents/file1.txt' to NOT be detected as directory")
	}
	
	// Test GetDirectory
	dirEntry, exists := index.GetDirectory("documents")
	if !exists {
		t.Fatal("Expected to find 'documents' directory entry")
	}
	
	if dirEntry.Type != DirectoryEntryType {
		t.Errorf("Expected directory type, got %s", dirEntry.Type)
	}
	
	if dirEntry.DirectoryDescriptorCID != "QmDocsDir" {
		t.Errorf("Expected directory CID 'QmDocsDir', got %s", dirEntry.DirectoryDescriptorCID)
	}
	
	if dirEntry.EncryptionKeyID != "key-docs" {
		t.Errorf("Expected encryption key ID 'key-docs', got %s", dirEntry.EncryptionKeyID)
	}
	
	// Test GetDirectoriesInDirectory
	dirs := index.GetDirectoriesInDirectory("documents")
	if len(dirs) != 1 {
		t.Errorf("Expected 1 directory in 'documents', got %d", len(dirs))
	}
	
	// Test HasDirectoryDescriptor
	if !index.HasDirectoryDescriptor("documents") {
		t.Error("Expected 'documents' to have directory descriptor")
	}
	
	if index.HasDirectoryDescriptor("documents/file1.txt") {
		t.Error("Expected 'documents/file1.txt' to NOT have directory descriptor")
	}
	
	// Test saving and loading
	if err := index.SaveIndex(); err != nil {
		t.Fatalf("Failed to save index: %v", err)
	}
	
	// Create new index and load
	index2 := NewFileIndex(indexPath)
	if err := index2.LoadIndex(); err != nil {
		t.Fatalf("Failed to load index: %v", err)
	}
	
	// Verify loaded data
	if index2.GetSize() != 4 { // 2 files + 2 directories
		t.Errorf("Expected 4 entries after load, got %d", index2.GetSize())
	}
	
	// Check backward compatibility
	fileEntry, exists := index2.GetFile("documents/file1.txt")
	if !exists {
		t.Fatal("Expected to find file after reload")
	}
	
	if fileEntry.Type != FileEntryType {
		t.Errorf("Expected file type after reload, got %s", fileEntry.Type)
	}
}

func TestFileIndexBackwardCompatibility(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "noisefs-index-compat-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	
	indexPath := filepath.Join(tmpDir, "test.index")
	
	// Create old-style index without type field
	oldIndexData := `{
		"version": "1.0",
		"entries": {
			"file1.txt": {
				"filename": "file1.txt",
				"descriptor_cid": "QmOldFile",
				"file_size": 512,
				"created_at": "2024-01-01T00:00:00Z",
				"modified_at": "2024-01-01T00:00:00Z",
				"directory": ""
			}
		}
	}`
	
	if err := os.WriteFile(indexPath, []byte(oldIndexData), 0600); err != nil {
		t.Fatalf("Failed to write old index: %v", err)
	}
	
	// Load with new index
	index := NewFileIndex(indexPath)
	if err := index.LoadIndex(); err != nil {
		t.Fatalf("Failed to load old index: %v", err)
	}
	
	// Check that type was set to file
	entry, exists := index.GetFile("file1.txt")
	if !exists {
		t.Fatal("Expected to find file1.txt")
	}
	
	if entry.Type != FileEntryType {
		t.Errorf("Expected type to be set to 'file' for backward compatibility, got %s", entry.Type)
	}
}

func TestFileIndexConcurrency(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "noisefs-index-concurrent-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	
	indexPath := filepath.Join(tmpDir, "test.index")
	index := NewFileIndex(indexPath)
	
	// Run concurrent operations
	done := make(chan bool)
	
	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			if i%2 == 0 {
				index.AddFile(filepath.Join("dir", fmt.Sprintf("file%d.txt", i)), fmt.Sprintf("Qm%d", i), int64(i*100))
			} else {
				index.AddDirectory(fmt.Sprintf("dir%d", i), fmt.Sprintf("QmDir%d", i), fmt.Sprintf("key%d", i))
			}
			time.Sleep(time.Microsecond)
		}
		done <- true
	}()
	
	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			_ = index.IsDirectory(fmt.Sprintf("dir%d", i))
			_ = index.GetSize()
			_ = index.ListFiles()
			time.Sleep(time.Microsecond)
		}
		done <- true
	}()
	
	// Wait for both to complete
	<-done
	<-done
	
	// Verify final state
	size := index.GetSize()
	if size != 100 {
		t.Errorf("Expected 100 entries after concurrent operations, got %d", size)
	}
}
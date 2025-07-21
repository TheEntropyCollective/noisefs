package sync

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileWatcher_Basic(t *testing.T) {
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "file_watcher_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create basic config
	config := &SyncConfig{
		WatchMode: true,
	}

	// Create file watcher
	fw, err := NewFileWatcher(config)
	if err != nil {
		t.Fatalf("Failed to create file watcher: %v", err)
	}
	defer fw.Stop()

	// Add path to watch
	err = fw.AddPath(tempDir)
	if err != nil {
		t.Fatalf("Failed to add path to watcher: %v", err)
	}

	// Create a test file
	testFile := filepath.Join(tempDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Wait for and verify file creation event
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	select {
	case event := <-fw.Events():
		if event.Type != EventTypeFileCreated {
			t.Errorf("Expected EventTypeFileCreated, got %s", event.Type)
		}
		if event.Path != testFile {
			t.Errorf("Expected path %s, got %s", testFile, event.Path)
		}
	case <-ctx.Done():
		t.Error("Timeout waiting for file creation event")
	}
}

func TestFileWatcher_FileModification(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "file_watcher_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	config := &SyncConfig{
		WatchMode: true,
	}

	fw, err := NewFileWatcher(config)
	if err != nil {
		t.Fatalf("Failed to create file watcher: %v", err)
	}
	defer fw.Stop()

	err = fw.AddPath(tempDir)
	if err != nil {
		t.Fatalf("Failed to add path to watcher: %v", err)
	}

	// Create and modify a file
	testFile := filepath.Join(tempDir, "modify_test.txt")
	err = os.WriteFile(testFile, []byte("initial content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Wait for creation event
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	select {
	case event := <-fw.Events():
		if event.Type != EventTypeFileCreated && event.Type != EventTypeFileModified {
			t.Errorf("Expected EventTypeFileCreated or EventTypeFileModified, got %s", event.Type)
		}
	case <-ctx.Done():
		t.Error("Timeout waiting for file creation event")
	}

	// Modify the file
	time.Sleep(200 * time.Millisecond) // Wait for debounce
	err = os.WriteFile(testFile, []byte("modified content"), 0644)
	if err != nil {
		t.Fatalf("Failed to modify test file: %v", err)
	}

	// Wait for modification event
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()

	select {
	case event := <-fw.Events():
		if event.Type != EventTypeFileModified {
			t.Errorf("Expected EventTypeFileModified, got %s", event.Type)
		}
		if event.Path != testFile {
			t.Errorf("Expected path %s, got %s", testFile, event.Path)
		}
	case <-ctx2.Done():
		t.Error("Timeout waiting for file modification event")
	}
}

func TestFileWatcher_FileDeletion(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "file_watcher_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	config := &SyncConfig{
		WatchMode: true,
	}

	fw, err := NewFileWatcher(config)
	if err != nil {
		t.Fatalf("Failed to create file watcher: %v", err)
	}
	defer fw.Stop()

	err = fw.AddPath(tempDir)
	if err != nil {
		t.Fatalf("Failed to add path to watcher: %v", err)
	}

	// Create a file
	testFile := filepath.Join(tempDir, "delete_test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Wait for creation event
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	select {
	case <-fw.Events():
		// Consume creation event
	case <-ctx.Done():
		t.Error("Timeout waiting for file creation event")
	}

	// Delete the file
	time.Sleep(200 * time.Millisecond) // Wait for debounce
	err = os.Remove(testFile)
	if err != nil {
		t.Fatalf("Failed to delete test file: %v", err)
	}

	// Wait for deletion event
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()

	select {
	case event := <-fw.Events():
		if event.Type != EventTypeFileDeleted {
			t.Errorf("Expected EventTypeFileDeleted, got %s", event.Type)
		}
		if event.Path != testFile {
			t.Errorf("Expected path %s, got %s", testFile, event.Path)
		}
	case <-ctx2.Done():
		t.Error("Timeout waiting for file deletion event")
	}
}

func TestFileWatcher_DirectoryOperations(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "file_watcher_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	config := &SyncConfig{
		WatchMode: true,
	}

	fw, err := NewFileWatcher(config)
	if err != nil {
		t.Fatalf("Failed to create file watcher: %v", err)
	}
	defer fw.Stop()

	err = fw.AddPath(tempDir)
	if err != nil {
		t.Fatalf("Failed to add path to watcher: %v", err)
	}

	// Create a subdirectory
	subDir := filepath.Join(tempDir, "subdir")
	err = os.Mkdir(subDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	// Wait for directory creation event
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	select {
	case event := <-fw.Events():
		if event.Type != EventTypeDirCreated {
			t.Errorf("Expected EventTypeDirCreated, got %s", event.Type)
		}
		if event.Path != subDir {
			t.Errorf("Expected path %s, got %s", subDir, event.Path)
		}
	case <-ctx.Done():
		t.Error("Timeout waiting for directory creation event")
	}

	// Verify the subdirectory is now being watched
	watchedPaths := fw.GetWatchedPaths()
	found := false
	for _, path := range watchedPaths {
		if path == subDir {
			found = true
			break
		}
	}
	if !found {
		t.Error("Subdirectory not added to watched paths")
	}
}

func TestFileWatcher_IncludePatterns(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "file_watcher_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	config := &SyncConfig{
		IncludePatterns: []string{"*.txt", "*.go"},
		WatchMode:       true,
	}

	fw, err := NewFileWatcher(config)
	if err != nil {
		t.Fatalf("Failed to create file watcher: %v", err)
	}
	defer fw.Stop()

	err = fw.AddPath(tempDir)
	if err != nil {
		t.Fatalf("Failed to add path to watcher: %v", err)
	}

	// Create a file that matches include pattern
	includedFile := filepath.Join(tempDir, "included.txt")
	err = os.WriteFile(includedFile, []byte("included content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create included file: %v", err)
	}

	// Wait for included file event
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	select {
	case event := <-fw.Events():
		if event.Type != EventTypeFileCreated {
			t.Errorf("Expected EventTypeFileCreated, got %s", event.Type)
		}
		if event.Path != includedFile {
			t.Errorf("Expected path %s, got %s", includedFile, event.Path)
		}
	case <-ctx.Done():
		t.Error("Timeout waiting for included file event")
	}

	// Create a file that doesn't match include pattern
	excludedFile := filepath.Join(tempDir, "excluded.log")
	err = os.WriteFile(excludedFile, []byte("excluded content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create excluded file: %v", err)
	}

	// Should not receive event for excluded file
	ctx2, cancel2 := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel2()

	select {
	case event := <-fw.Events():
		t.Errorf("Unexpected event for excluded file: %+v", event)
	case <-ctx2.Done():
		// Expected timeout - no event should be received
	}
}

func TestFileWatcher_ExcludePatterns(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "file_watcher_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	config := &SyncConfig{
		ExcludePatterns: []string{"*.log", "*.tmp"},
		WatchMode:       true,
	}

	fw, err := NewFileWatcher(config)
	if err != nil {
		t.Fatalf("Failed to create file watcher: %v", err)
	}
	defer fw.Stop()

	err = fw.AddPath(tempDir)
	if err != nil {
		t.Fatalf("Failed to add path to watcher: %v", err)
	}

	// Create a file that matches exclude pattern
	excludedFile := filepath.Join(tempDir, "excluded.log")
	err = os.WriteFile(excludedFile, []byte("excluded content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create excluded file: %v", err)
	}

	// Should not receive event for excluded file
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	select {
	case event := <-fw.Events():
		t.Errorf("Unexpected event for excluded file: %+v", event)
	case <-ctx.Done():
		// Expected timeout - no event should be received
	}

	// Create a file that doesn't match exclude pattern
	includedFile := filepath.Join(tempDir, "included.txt")
	err = os.WriteFile(includedFile, []byte("included content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create included file: %v", err)
	}

	// Should receive event for included file
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()

	select {
	case event := <-fw.Events():
		if event.Type != EventTypeFileCreated {
			t.Errorf("Expected EventTypeFileCreated, got %s", event.Type)
		}
		if event.Path != includedFile {
			t.Errorf("Expected path %s, got %s", includedFile, event.Path)
		}
	case <-ctx2.Done():
		t.Error("Timeout waiting for included file event")
	}
}

func TestFileWatcher_RemovePath(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "file_watcher_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	config := &SyncConfig{
		WatchMode: true,
	}

	fw, err := NewFileWatcher(config)
	if err != nil {
		t.Fatalf("Failed to create file watcher: %v", err)
	}
	defer fw.Stop()

	// Add and then remove path
	err = fw.AddPath(tempDir)
	if err != nil {
		t.Fatalf("Failed to add path to watcher: %v", err)
	}

	// Verify path is being watched
	watchedPaths := fw.GetWatchedPaths()
	if len(watchedPaths) == 0 {
		t.Error("No paths being watched")
	}

	// Remove the path
	err = fw.RemovePath(tempDir)
	if err != nil {
		t.Fatalf("Failed to remove path from watcher: %v", err)
	}

	// Verify path is no longer being watched
	watchedPaths = fw.GetWatchedPaths()
	for _, path := range watchedPaths {
		if path == tempDir {
			t.Error("Path still being watched after removal")
		}
	}
}

func TestFileWatcher_Stop(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "file_watcher_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	config := &SyncConfig{
		WatchMode: true,
	}

	fw, err := NewFileWatcher(config)
	if err != nil {
		t.Fatalf("Failed to create file watcher: %v", err)
	}

	err = fw.AddPath(tempDir)
	if err != nil {
		t.Fatalf("Failed to add path to watcher: %v", err)
	}

	// Stop the watcher
	err = fw.Stop()
	if err != nil {
		t.Fatalf("Failed to stop file watcher: %v", err)
	}

	// Verify channels are closed
	select {
	case _, ok := <-fw.Events():
		if ok {
			t.Error("Events channel not closed after stop")
		}
	default:
		t.Error("Events channel should be closed")
	}

	select {
	case _, ok := <-fw.Errors():
		if ok {
			t.Error("Errors channel not closed after stop")
		}
	default:
		t.Error("Errors channel should be closed")
	}
}

func TestFileWatcher_ErrorHandling(t *testing.T) {
	config := &SyncConfig{
		WatchMode: true,
	}

	fw, err := NewFileWatcher(config)
	if err != nil {
		t.Fatalf("Failed to create file watcher: %v", err)
	}
	defer fw.Stop()

	// Try to add a non-existent path
	err = fw.AddPath("/non/existent/path")
	if err == nil {
		t.Error("Expected error when adding non-existent path")
	}

	// Try to remove a path that wasn't added
	err = fw.RemovePath("/another/non/existent/path")
	if err != nil {
		t.Errorf("Unexpected error when removing non-watched path: %v", err)
	}
}

package sync

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInitialSyncIntegration(t *testing.T) {
	// Create temporary directories
	localDir, err := os.MkdirTemp("", "initial_sync_local")
	if err != nil {
		t.Fatalf("Failed to create local temp dir: %v", err)
	}
	defer os.RemoveAll(localDir)

	stateDir, err := os.MkdirTemp("", "initial_sync_state")
	if err != nil {
		t.Fatalf("Failed to create state temp dir: %v", err)
	}
	defer os.RemoveAll(stateDir)

	// Create test files in local directory
	testFiles := map[string]string{
		"document.txt":         "This is a test document",
		"data/numbers.csv":     "1,2,3\n4,5,6\n",
		"data/empty_file.txt":  "",
		"scripts/hello.sh":     "#!/bin/bash\necho 'Hello World'\n",
	}

	for relativePath, content := range testFiles {
		fullPath := filepath.Join(localDir, relativePath)
		
		// Create directory if needed
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		
		// Create file
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", relativePath, err)
		}
	}

	// Create state store
	stateStore, err := NewSyncStateStore(stateDir)
	if err != nil {
		t.Fatalf("Failed to create state store: %v", err)
	}

	// Create sync state for testing
	syncID := "test-initial-sync"
	remotePath := "/remote/test"

	if err := stateStore.CreateInitialState(syncID, localDir, remotePath); err != nil {
		t.Fatalf("Failed to create initial state: %v", err)
	}

	// Load state to simulate sync session
	state, err := stateStore.LoadState(syncID)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}

	// Test the scanner without remote directory manager (local-only initial scan)
	scanner := NewDirectoryScanner(nil)

	// Perform initial scan
	ctx := context.Background()
	scanResult, err := scanner.PerformInitialScan(ctx, localDir, remotePath, "", state)
	if err != nil {
		t.Fatalf("Initial scan failed: %v", err)
	}

	// Verify local snapshot contains all expected files
	expectedLocalFiles := len(testFiles) + 2 // +2 for data/ and scripts/ directories
	if len(scanResult.LocalSnapshot) != expectedLocalFiles {
		t.Errorf("Expected %d local files, got %d", expectedLocalFiles, len(scanResult.LocalSnapshot))
	}

	// Verify specific files
	for relativePath, content := range testFiles {
		metadata, exists := scanResult.LocalSnapshot[relativePath]
		if !exists {
			t.Errorf("File %s not found in local snapshot", relativePath)
			continue
		}

		if metadata.Size != int64(len(content)) {
			t.Errorf("File %s: expected size %d, got %d", relativePath, len(content), metadata.Size)
		}

		if !metadata.IsDir && content != "" && metadata.Checksum == "" {
			t.Errorf("File %s: missing checksum", relativePath)
		}
	}

	// Verify directories are captured
	for _, dirPath := range []string{"data", "scripts"} {
		metadata, exists := scanResult.LocalSnapshot[dirPath]
		if !exists {
			t.Errorf("Directory %s not found in local snapshot", dirPath)
			continue
		}

		if !metadata.IsDir {
			t.Errorf("Directory %s not marked as directory", dirPath)
		}
	}

	// Since no remote manifest was provided, remote snapshot should be empty
	if len(scanResult.RemoteSnapshot) != 0 {
		t.Errorf("Expected empty remote snapshot, got %d files", len(scanResult.RemoteSnapshot))
	}

	// All local files should generate upload changes (since no remote exists)
	if len(scanResult.Changes) != expectedLocalFiles {
		t.Errorf("Expected %d changes, got %d", expectedLocalFiles, len(scanResult.Changes))
	}

	// Verify all changes are local creates
	for _, change := range scanResult.Changes {
		if change.Type != ChangeTypeCreate {
			t.Errorf("Expected all changes to be creates, got %s for %s", change.Type, change.Path)
		}
		if !change.IsLocal {
			t.Errorf("Expected all changes to be local, got remote change for %s", change.Path)
		}
	}

	// Generate sync operations
	operations := scanner.GenerateSyncOperations(syncID, scanResult.Changes, localDir, remotePath)
	
	if len(operations) != expectedLocalFiles {
		t.Errorf("Expected %d operations, got %d", expectedLocalFiles, len(operations))
	}

	// Verify operation prioritization: directories should come first
	var dirOpCount int
	for i, op := range operations {
		if op.Type == OpTypeCreateDir {
			dirOpCount++
			// Directory operations should be at the beginning
			if i >= 2 { // We have 2 directories (data, scripts)
				t.Errorf("Directory operation found at position %d, expected at beginning", i)
			}
		}
	}

	if dirOpCount != 2 {
		t.Errorf("Expected 2 directory operations, got %d", dirOpCount)
	}

	// Verify operations have correct paths
	for _, op := range operations {
		if op.Status != OpStatusPending {
			t.Errorf("Expected operation status pending, got %s", op.Status)
		}

		if !filepath.IsAbs(op.LocalPath) {
			t.Errorf("Expected absolute local path, got %s", op.LocalPath)
		}

		if !filepath.IsAbs(op.RemotePath) {
			t.Errorf("Expected absolute remote path, got %s", op.RemotePath)
		}

		// Local path should be under localDir
		if !filepath.HasPrefix(op.LocalPath, localDir) {
			t.Errorf("Local path %s should be under %s", op.LocalPath, localDir)
		}

		// Remote path should be under remotePath
		if !filepath.HasPrefix(op.RemotePath, remotePath) {
			t.Errorf("Remote path %s should be under %s", op.RemotePath, remotePath)
		}
	}

	// Test scan duration
	if scanResult.ScanDuration <= 0 {
		t.Error("Scan duration should be positive")
	}

	t.Logf("Initial sync integration test completed successfully:")
	t.Logf("  - Scanned %d local files/directories", len(scanResult.LocalSnapshot))
	t.Logf("  - Generated %d changes", len(scanResult.Changes))
	t.Logf("  - Created %d sync operations", len(operations))
	t.Logf("  - Scan duration: %v", scanResult.ScanDuration)
}

func TestInitialSyncWithPreviousState(t *testing.T) {
	// Create temporary directory
	localDir, err := os.MkdirTemp("", "initial_sync_previous")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(localDir)

	// Create initial files
	initialFiles := map[string]string{
		"file1.txt": "original content",
		"file2.txt": "another file",
	}

	for name, content := range initialFiles {
		err := os.WriteFile(filepath.Join(localDir, name), []byte(content), 0644)
		if err != nil {
			t.Fatalf("Failed to create initial file: %v", err)
		}
	}

	// Create scanner and perform first scan
	scanner := NewDirectoryScanner(nil)
	ctx := context.Background()

	firstScan, err := scanner.PerformInitialScan(ctx, localDir, "/remote", "", nil)
	if err != nil {
		t.Fatalf("First scan failed: %v", err)
	}

	// Create previous state from first scan
	previousState := &SyncState{
		LocalPath:      localDir,
		RemotePath:     "/remote",
		LocalSnapshot:  firstScan.LocalSnapshot,
		RemoteSnapshot: firstScan.RemoteSnapshot,
		LastSync:       time.Now().Add(-1 * time.Hour),
	}

	// Modify files and add new ones
	if err := os.WriteFile(filepath.Join(localDir, "file1.txt"), []byte("modified content"), 0644); err != nil {
		t.Fatalf("Failed to modify file: %v", err)
	}

	if err := os.WriteFile(filepath.Join(localDir, "new_file.txt"), []byte("new content"), 0644); err != nil {
		t.Fatalf("Failed to create new file: %v", err)
	}

	if err := os.Remove(filepath.Join(localDir, "file2.txt")); err != nil {
		t.Fatalf("Failed to remove file: %v", err)
	}

	// Wait a bit to ensure different modification times
	time.Sleep(10 * time.Millisecond)

	// Perform second scan with previous state
	secondScan, err := scanner.PerformInitialScan(ctx, localDir, "/remote", "", previousState)
	if err != nil {
		t.Fatalf("Second scan failed: %v", err)
	}

	// Should detect changes
	if len(secondScan.Changes) == 0 {
		t.Error("Expected changes to be detected, got none")
	}

	// Verify specific changes
	changeMap := make(map[string]DetectedChange)
	for _, change := range secondScan.Changes {
		changeMap[change.Path] = change
	}

	// Should detect file1.txt as modified
	if change, exists := changeMap["file1.txt"]; exists {
		if change.Type != ChangeTypeModify {
			t.Errorf("Expected file1.txt to be modified, got %s", change.Type)
		}
	} else {
		t.Error("Expected change for file1.txt")
	}

	// Should detect new_file.txt as created
	if change, exists := changeMap["new_file.txt"]; exists {
		if change.Type != ChangeTypeCreate {
			t.Errorf("Expected new_file.txt to be created, got %s", change.Type)
		}
	} else {
		t.Error("Expected change for new_file.txt")
	}

	// Should detect file2.txt as deleted
	if change, exists := changeMap["file2.txt"]; exists {
		if change.Type != ChangeTypeDelete {
			t.Errorf("Expected file2.txt to be deleted, got %s", change.Type)
		}
	} else {
		t.Error("Expected change for file2.txt")
	}

	t.Logf("Previous state test completed with %d changes detected", len(secondScan.Changes))
}

func TestEmptyDirectoryHandling(t *testing.T) {
	// Create temporary directory with empty subdirectory
	localDir, err := os.MkdirTemp("", "empty_dir_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(localDir)

	// Create empty subdirectory
	emptyDir := filepath.Join(localDir, "empty_subdir")
	if err := os.MkdirAll(emptyDir, 0755); err != nil {
		t.Fatalf("Failed to create empty directory: %v", err)
	}

	// Create file in another subdirectory
	dataDir := filepath.Join(localDir, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatalf("Failed to create data directory: %v", err)
	}

	testFile := filepath.Join(dataDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Scan directory
	scanner := NewDirectoryScanner(nil)
	ctx := context.Background()

	scanResult, err := scanner.PerformInitialScan(ctx, localDir, "/remote", "", nil)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Should find 3 entries: empty_subdir, data, data/test.txt
	if len(scanResult.LocalSnapshot) != 3 {
		t.Errorf("Expected 3 entries, got %d", len(scanResult.LocalSnapshot))
	}

	// Verify empty directory is captured
	emptyDirMeta, exists := scanResult.LocalSnapshot["empty_subdir"]
	if !exists {
		t.Error("Empty directory not found in snapshot")
	} else {
		if !emptyDirMeta.IsDir {
			t.Error("Empty directory not marked as directory")
		}
		// Note: Directory size on filesystem may not be 0, that's normal
	}

	// Should generate create operations for empty directories
	operations := scanner.GenerateSyncOperations("test", scanResult.Changes, localDir, "/remote")
	
	var emptyDirOp *SyncOperation
	for i := range operations {
		if operations[i].Type == OpTypeCreateDir && filepath.Base(operations[i].LocalPath) == "empty_subdir" {
			emptyDirOp = &operations[i]
			break
		}
	}

	if emptyDirOp == nil {
		t.Error("No create directory operation found for empty directory")
	}

	t.Logf("Empty directory handling test completed successfully")
}
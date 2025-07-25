package sync

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDirectoryScanner_ScanLocalDirectory(t *testing.T) {
	// Create temporary directory structure
	tempDir, err := os.MkdirTemp("", "scanner_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test files and directories
	testFiles := map[string]string{
		"file1.txt":        "Hello, World!",
		"file2.txt":        "Another file content",
		"subdir/file3.txt": "File in subdirectory",
		"empty.txt":        "",
	}

	for relativePath, content := range testFiles {
		fullPath := filepath.Join(tempDir, relativePath)
		
		// Create directory if needed
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		
		// Create file
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", relativePath, err)
		}
	}

	// Create directory scanner
	scanner := NewDirectoryScanner(nil) // No directory manager needed for local scan

	// Scan the directory
	snapshot, err := scanner.ScanLocalDirectory(tempDir)
	if err != nil {
		t.Fatalf("Failed to scan directory: %v", err)
	}

	// Verify results
	expectedFiles := len(testFiles) + 1 // +1 for the subdirectory
	if len(snapshot) != expectedFiles {
		t.Errorf("Expected %d entries, got %d", expectedFiles, len(snapshot))
	}

	// Check individual files
	for relativePath, content := range testFiles {
		metadata, exists := snapshot[relativePath]
		if !exists {
			t.Errorf("File %s not found in snapshot", relativePath)
			continue
		}

		if metadata.Path != relativePath {
			t.Errorf("Expected path %s, got %s", relativePath, metadata.Path)
		}

		if metadata.Size != int64(len(content)) {
			t.Errorf("Expected size %d for %s, got %d", len(content), relativePath, metadata.Size)
		}

		if metadata.IsDir {
			t.Errorf("File %s incorrectly marked as directory", relativePath)
		}

		if content != "" && metadata.Checksum == "" {
			t.Errorf("Missing checksum for file %s", relativePath)
		}
	}

	// Check subdirectory
	subdirMeta, exists := snapshot["subdir"]
	if !exists {
		t.Error("Subdirectory not found in snapshot")
	} else {
		if !subdirMeta.IsDir {
			t.Error("Subdirectory not marked as directory")
		}
		if subdirMeta.Checksum != "" {
			t.Error("Directory should not have checksum")
		}
	}
}

func TestDirectoryScanner_ScanRemoteDirectory(t *testing.T) {
	// Skip test that requires complex mocking of storage components
	t.Skip("Remote directory scanning test requires complex storage mocking - tested in integration tests") 
}

func TestDirectoryScanner_GenerateInitialChanges(t *testing.T) {
	scanner := NewDirectoryScanner(nil)

	// Create test snapshots
	localSnapshot := map[string]FileMetadata{
		"file1.txt": {
			Path:     "file1.txt",
			Size:     100,
			ModTime:  time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
			IsDir:    false,
			Checksum: "abc123",
		},
		"file2.txt": {
			Path:     "file2.txt", 
			Size:     200,
			ModTime:  time.Date(2023, 1, 2, 12, 0, 0, 0, time.UTC),
			IsDir:    false,
			Checksum: "def456",
		},
		"local_only.txt": {
			Path:     "local_only.txt",
			Size:     50,
			ModTime:  time.Date(2023, 1, 3, 12, 0, 0, 0, time.UTC),
			IsDir:    false,
			Checksum: "local123",
		},
	}

	remoteSnapshot := map[string]RemoteMetadata{
		"file1.txt": {
			Path:          "file1.txt",
			DescriptorCID: "QmTest1",
			Size:          100,
			ModTime:       time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC), // Same as local
			IsDir:         false,
		},
		"file2.txt": {
			Path:          "file2.txt",
			DescriptorCID: "QmTest2", 
			Size:          200,
			ModTime:       time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC), // Older than local
			IsDir:         false,
		},
		"remote_only.txt": {
			Path:          "remote_only.txt",
			DescriptorCID: "QmTest3",
			Size:          75,
			ModTime:       time.Date(2023, 1, 4, 12, 0, 0, 0, time.UTC),
			IsDir:         false,
		},
	}

	// Generate changes
	changes := scanner.generateInitialChanges(localSnapshot, remoteSnapshot)

	// Should have changes for:
	// - local_only.txt (create from local)
	// - remote_only.txt (create from remote)  
	// - file2.txt (modify from local since it's newer)
	expectedChanges := 3
	if len(changes) != expectedChanges {
		t.Errorf("Expected %d changes, got %d", expectedChanges, len(changes))
	}

	// Verify specific changes
	changeMap := make(map[string]DetectedChange)
	for _, change := range changes {
		changeMap[change.Path] = change
	}

	// Check local_only.txt
	if change, exists := changeMap["local_only.txt"]; exists {
		if change.Type != ChangeTypeCreate || !change.IsLocal {
			t.Errorf("Expected local create for local_only.txt, got %s, isLocal=%t", change.Type, change.IsLocal)
		}
	} else {
		t.Error("Missing change for local_only.txt")
	}

	// Check remote_only.txt
	if change, exists := changeMap["remote_only.txt"]; exists {
		if change.Type != ChangeTypeCreate || change.IsLocal {
			t.Errorf("Expected remote create for remote_only.txt, got %s, isLocal=%t", change.Type, change.IsLocal)
		}
	} else {
		t.Error("Missing change for remote_only.txt")
	}

	// Check file2.txt (should be local modify since local is newer)
	if change, exists := changeMap["file2.txt"]; exists {
		if change.Type != ChangeTypeModify || !change.IsLocal {
			t.Errorf("Expected local modify for file2.txt, got %s, isLocal=%t", change.Type, change.IsLocal)
		}
	} else {
		t.Error("Missing change for file2.txt")
	}
}

func TestDirectoryScanner_GenerateSyncOperations(t *testing.T) {
	scanner := NewDirectoryScanner(nil)

	// Create test changes
	changes := []DetectedChange{
		{
			Type:    ChangeTypeCreate,
			Path:    "subdir",
			IsLocal: true,
			Metadata: FileMetadata{
				Path:  "subdir",
				IsDir: true,
			},
		},
		{
			Type:    ChangeTypeCreate,
			Path:    "file1.txt",
			IsLocal: true,
			Metadata: FileMetadata{
				Path:  "file1.txt",
				IsDir: false,
				Size:  100,
			},
		},
		{
			Type:    ChangeTypeCreate,
			Path:    "file2.txt",
			IsLocal: false,
			Metadata: RemoteMetadata{
				Path:  "file2.txt",
				IsDir: false,
				Size:  200,
			},
		},
		{
			Type:    ChangeTypeDelete,
			Path:    "old_file.txt",
			IsLocal: true,
		},
	}

	// Generate operations
	operations := scanner.GenerateSyncOperations("test-session", changes, "/local", "/remote")

	if len(operations) != 4 {
		t.Errorf("Expected 4 operations, got %d", len(operations))
	}

	// Check operation prioritization: directories first, then files, then deletes
	if operations[0].Type != OpTypeCreateDir {
		t.Errorf("Expected first operation to be CreateDir, got %s", operations[0].Type)
	}

	// Find upload and download operations
	var uploadOp, downloadOp, deleteOp *SyncOperation
	for i := range operations {
		switch operations[i].Type {
		case OpTypeUpload:
			uploadOp = &operations[i]
		case OpTypeDownload:
			downloadOp = &operations[i]
		case OpTypeDelete:
			deleteOp = &operations[i]
		}
	}

	if uploadOp == nil {
		t.Error("Missing upload operation")
	} else {
		if uploadOp.LocalPath != "/local/file1.txt" {
			t.Errorf("Expected local path /local/file1.txt, got %s", uploadOp.LocalPath)
		}
		if uploadOp.RemotePath != "/remote/file1.txt" {
			t.Errorf("Expected remote path /remote/file1.txt, got %s", uploadOp.RemotePath)
		}
	}

	if downloadOp == nil {
		t.Error("Missing download operation")
	} else {
		if downloadOp.LocalPath != "/local/file2.txt" {
			t.Errorf("Expected local path /local/file2.txt, got %s", downloadOp.LocalPath)
		}
	}

	if deleteOp == nil {
		t.Error("Missing delete operation")
	}

	// Check that delete operation comes last
	if operations[len(operations)-1].Type != OpTypeDelete {
		t.Error("Delete operation should be last")
	}
}

func TestDirectoryScanner_NeedsSync(t *testing.T) {
	scanner := NewDirectoryScanner(nil)

	testCases := []struct {
		name     string
		local    FileMetadata
		remote   RemoteMetadata
		expected bool
	}{
		{
			name: "identical files",
			local: FileMetadata{
				Size:    100,
				ModTime: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
				IsDir:   false,
			},
			remote: RemoteMetadata{
				Size:    100,
				ModTime: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
				IsDir:   false,
			},
			expected: false,
		},
		{
			name: "different sizes",
			local: FileMetadata{
				Size:    100,
				ModTime: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
				IsDir:   false,
			},
			remote: RemoteMetadata{
				Size:    200,
				ModTime: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
				IsDir:   false,
			},
			expected: true,
		},
		{
			name: "different modification times",
			local: FileMetadata{
				Size:    100,
				ModTime: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
				IsDir:   false,
			},
			remote: RemoteMetadata{
				Size:    100,
				ModTime: time.Date(2023, 1, 2, 12, 0, 0, 0, time.UTC),
				IsDir:   false,
			},
			expected: true,
		},
		{
			name: "type mismatch",
			local: FileMetadata{
				Size:    0,
				ModTime: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
				IsDir:   true,
			},
			remote: RemoteMetadata{
				Size:    100,
				ModTime: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
				IsDir:   false,
			},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := scanner.needsSync(tc.local, tc.remote)
			if result != tc.expected {
				t.Errorf("Expected %t, got %t", tc.expected, result)
			}
		})
	}
}

func TestDirectoryScanner_PerformInitialScan_Integration(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "integration_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test files
	testFiles := map[string]string{
		"file1.txt": "content1",
		"file2.txt": "content2",
	}

	for name, content := range testFiles {
		err := os.WriteFile(filepath.Join(tempDir, name), []byte(content), 0644)
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	// Create scanner without remote directory manager (for local-only test)
	scanner := NewDirectoryScanner(nil)

	// Perform scan
	ctx := context.Background()
	result, err := scanner.PerformInitialScan(ctx, tempDir, "/remote", "", nil)
	if err != nil {
		t.Fatalf("Initial scan failed: %v", err)
	}

	// Verify local snapshot
	if len(result.LocalSnapshot) != 2 {
		t.Errorf("Expected 2 local files, got %d", len(result.LocalSnapshot))
	}

	// Verify empty remote snapshot (no manifest CID provided)
	if len(result.RemoteSnapshot) != 0 {
		t.Errorf("Expected empty remote snapshot, got %d files", len(result.RemoteSnapshot))
	}

	// Verify changes (all local files should be creates since no previous state)
	if len(result.Changes) != 2 {
		t.Errorf("Expected 2 changes, got %d", len(result.Changes))
	}

	// Verify scan duration is reasonable
	if result.ScanDuration <= 0 {
		t.Error("Scan duration should be positive")
	}
}
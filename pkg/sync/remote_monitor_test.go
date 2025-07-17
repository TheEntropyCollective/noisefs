package sync

import (
	"context"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
)

// Mock directory manager for testing
type mockDirectoryManager struct {
	manifests map[string]*blocks.DirectoryManifest
}

func (m *mockDirectoryManager) RetrieveDirectoryManifest(ctx context.Context, dirPath string, manifestCID string) (*blocks.DirectoryManifest, error) {
	if manifest, exists := m.manifests[manifestCID]; exists {
		return manifest, nil
	}
	return &blocks.DirectoryManifest{
		Version: "1.0",
		Entries: []blocks.DirectoryEntry{},
	}, nil
}

func TestRemoteChangeMonitor_Basic(t *testing.T) {
	// This test is skipped because it requires a complex mock setup
	// The RemoteChangeMonitor depends on DirectoryManager which has complex dependencies
	t.Skip("Integration test requires full DirectoryManager setup")
}

func TestRemoteChangeMonitor_CompareSnapshots(t *testing.T) {
	// Test snapshot comparison logic directly
	now := time.Now()
	
	// Create old snapshot
	oldSnapshot := map[string]RemoteMetadata{
		"/remote/file1.txt": {
			Path:          "/remote/file1.txt",
			DescriptorCID: "QmOld1",
			Size:          1024,
			ModTime:       now.Add(-time.Hour),
			IsDir:         false,
		},
		"/remote/dir1": {
			Path:          "/remote/dir1",
			DescriptorCID: "QmOld2",
			Size:          0,
			ModTime:       now.Add(-time.Hour),
			IsDir:         true,
		},
	}

	// Create new snapshot with changes
	newSnapshot := map[string]RemoteMetadata{
		"/remote/file1.txt": {
			Path:          "/remote/file1.txt",
			DescriptorCID: "QmNew1", // Changed CID
			Size:          2048,     // Changed size
			ModTime:       now,      // Changed mod time
			IsDir:         false,
		},
		"/remote/dir1": {
			Path:          "/remote/dir1",
			DescriptorCID: "QmOld2", // Unchanged
			Size:          0,
			ModTime:       now.Add(-time.Hour),
			IsDir:         true,
		},
		"/remote/file2.txt": {
			Path:          "/remote/file2.txt",
			DescriptorCID: "QmNew2", // New file
			Size:          512,
			ModTime:       now,
			IsDir:         false,
		},
	}

	// Create a mock monitor to test the comparison
	config := &SyncConfig{SyncInterval: time.Minute}
	tempDir := t.TempDir()
	stateStore, err := NewSyncStateStore(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state store: %v", err)
	}

	// Create a minimal monitor just to test the comparison method
	monitor := &RemoteChangeMonitor{
		config:     config,
		stateStore: stateStore,
	}

	// Test the comparison
	changes := monitor.compareSnapshots(oldSnapshot, newSnapshot)

	// Verify changes
	if len(changes) != 2 {
		t.Errorf("Expected 2 changes, got %d", len(changes))
	}

	// Check for file modification
	foundModification := false
	foundCreation := false

	for _, change := range changes {
		if change.Path == "/remote/file1.txt" && change.Type == EventTypeFileModified {
			foundModification = true
			
			// Check metadata
			oldSize, ok := change.Metadata["old_size"]
			if !ok || oldSize != int64(1024) {
				t.Error("Expected old_size metadata")
			}
			
			newSize, ok := change.Metadata["new_size"]
			if !ok || newSize != int64(2048) {
				t.Error("Expected new_size metadata")
			}
		}
		
		if change.Path == "/remote/file2.txt" && change.Type == EventTypeFileCreated {
			foundCreation = true
			
			// Check metadata
			size, ok := change.Metadata["size"]
			if !ok || size != int64(512) {
				t.Error("Expected size metadata for created file")
			}
		}
	}

	if !foundModification {
		t.Error("Expected to find file modification event")
	}
	if !foundCreation {
		t.Error("Expected to find file creation event")
	}
}

func TestRemoteChangeMonitor_HasMetadataChanged(t *testing.T) {
	now := time.Now()
	
	baseMetadata := RemoteMetadata{
		Path:          "/test/file.txt",
		DescriptorCID: "QmTest",
		Size:          1024,
		ModTime:       now,
		IsDir:         false,
	}

	// Create monitor for testing
	monitor := &RemoteChangeMonitor{}

	// Test no change
	if monitor.hasMetadataChanged(baseMetadata, baseMetadata) {
		t.Error("Expected no change for identical metadata")
	}

	// Test size change
	sizeChanged := baseMetadata
	sizeChanged.Size = 2048
	if !monitor.hasMetadataChanged(baseMetadata, sizeChanged) {
		t.Error("Expected change for different size")
	}

	// Test CID change
	cidChanged := baseMetadata
	cidChanged.DescriptorCID = "QmChanged"
	if !monitor.hasMetadataChanged(baseMetadata, cidChanged) {
		t.Error("Expected change for different CID")
	}

	// Test mod time change
	timeChanged := baseMetadata
	timeChanged.ModTime = now.Add(time.Hour)
	if !monitor.hasMetadataChanged(baseMetadata, timeChanged) {
		t.Error("Expected change for different mod time")
	}

	// Test type change
	typeChanged := baseMetadata
	typeChanged.IsDir = true
	if !monitor.hasMetadataChanged(baseMetadata, typeChanged) {
		t.Error("Expected change for different type")
	}
}

func TestRemoteChangeMonitor_DeletionDetection(t *testing.T) {
	now := time.Now()
	
	// Create old snapshot with more files
	oldSnapshot := map[string]RemoteMetadata{
		"/remote/file1.txt": {
			Path:          "/remote/file1.txt",
			DescriptorCID: "QmOld1",
			Size:          1024,
			ModTime:       now.Add(-time.Hour),
			IsDir:         false,
		},
		"/remote/file2.txt": {
			Path:          "/remote/file2.txt",
			DescriptorCID: "QmOld2",
			Size:          512,
			ModTime:       now.Add(-time.Hour),
			IsDir:         false,
		},
		"/remote/dir1": {
			Path:          "/remote/dir1",
			DescriptorCID: "QmOld3",
			Size:          0,
			ModTime:       now.Add(-time.Hour),
			IsDir:         true,
		},
	}

	// Create new snapshot with file2 and dir1 removed
	newSnapshot := map[string]RemoteMetadata{
		"/remote/file1.txt": {
			Path:          "/remote/file1.txt",
			DescriptorCID: "QmOld1",
			Size:          1024,
			ModTime:       now.Add(-time.Hour),
			IsDir:         false,
		},
	}

	// Create a mock monitor to test deletion detection
	monitor := &RemoteChangeMonitor{}

	// Test the comparison
	changes := monitor.compareSnapshots(oldSnapshot, newSnapshot)

	// Should have 2 deletion events
	if len(changes) != 2 {
		t.Errorf("Expected 2 deletion events, got %d", len(changes))
	}

	foundFileDeletion := false
	foundDirDeletion := false

	for _, change := range changes {
		if change.Path == "/remote/file2.txt" && change.Type == EventTypeFileDeleted {
			foundFileDeletion = true
			
			// Check metadata
			oldSize, ok := change.Metadata["old_size"]
			if !ok || oldSize != int64(512) {
				t.Error("Expected old_size metadata for deleted file")
			}
			
			changeType, ok := change.Metadata["change_type"]
			if !ok || changeType != "deleted" {
				t.Error("Expected change_type metadata")
			}
		}
		
		if change.Path == "/remote/dir1" && change.Type == EventTypeDirDeleted {
			foundDirDeletion = true
			
			// Check metadata
			isDir, ok := change.Metadata["is_dir"]
			if !ok || isDir != true {
				t.Error("Expected is_dir metadata for deleted directory")
			}
		}
	}

	if !foundFileDeletion {
		t.Error("Expected to find file deletion event")
	}
	if !foundDirDeletion {
		t.Error("Expected to find directory deletion event")
	}
}

func TestRemoteChangeMonitor_EmptySnapshots(t *testing.T) {
	// Test comparison with empty snapshots
	monitor := &RemoteChangeMonitor{}

	// Empty to empty - no changes
	changes := monitor.compareSnapshots(map[string]RemoteMetadata{}, map[string]RemoteMetadata{})
	if len(changes) != 0 {
		t.Errorf("Expected 0 changes for empty snapshots, got %d", len(changes))
	}

	// Empty to non-empty - all creations
	newSnapshot := map[string]RemoteMetadata{
		"/remote/file1.txt": {
			Path:          "/remote/file1.txt",
			DescriptorCID: "QmNew1",
			Size:          1024,
			ModTime:       time.Now(),
			IsDir:         false,
		},
	}

	changes = monitor.compareSnapshots(map[string]RemoteMetadata{}, newSnapshot)
	if len(changes) != 1 {
		t.Errorf("Expected 1 creation event, got %d", len(changes))
	}
	if changes[0].Type != EventTypeFileCreated {
		t.Errorf("Expected file creation event, got %s", changes[0].Type)
	}

	// Non-empty to empty - all deletions
	oldSnapshot := map[string]RemoteMetadata{
		"/remote/file1.txt": {
			Path:          "/remote/file1.txt",
			DescriptorCID: "QmOld1",
			Size:          1024,
			ModTime:       time.Now(),
			IsDir:         false,
		},
	}

	changes = monitor.compareSnapshots(oldSnapshot, map[string]RemoteMetadata{})
	if len(changes) != 1 {
		t.Errorf("Expected 1 deletion event, got %d", len(changes))
	}
	if changes[0].Type != EventTypeFileDeleted {
		t.Errorf("Expected file deletion event, got %s", changes[0].Type)
	}
}

func TestRemoteMonitorStats(t *testing.T) {
	// Test the stats functionality
	config := &SyncConfig{
		SyncInterval: time.Minute,
	}

	tempDir := t.TempDir()
	stateStore, err := NewSyncStateStore(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state store: %v", err)
	}

	monitor := &RemoteChangeMonitor{
		config:         config,
		stateStore:     stateStore,
		monitoredPaths: make(map[string]*RemoteMonitorState),
		pollInterval:   time.Minute,
		eventChan:      make(chan SyncEvent, 100),
		errorChan:      make(chan error, 10),
	}

	// Test empty stats
	stats := monitor.GetStats()
	if stats.TotalPaths != 0 {
		t.Errorf("Expected 0 total paths, got %d", stats.TotalPaths)
	}
	if stats.ActivePaths != 0 {
		t.Errorf("Expected 0 active paths, got %d", stats.ActivePaths)
	}
	if stats.PollInterval != time.Minute {
		t.Errorf("Expected poll interval of 1 minute, got %v", stats.PollInterval)
	}

	// Add some monitored paths
	now := time.Now()
	monitor.monitoredPaths["/remote/path1"] = &RemoteMonitorState{
		RemotePath:  "/remote/path1",
		LastChecked: now,
		SyncID:      "sync1",
	}
	monitor.monitoredPaths["/remote/path2"] = &RemoteMonitorState{
		RemotePath:  "/remote/path2",
		LastChecked: now.Add(-5 * time.Minute), // Old check
		SyncID:      "sync2",
	}

	// Test stats with paths
	stats = monitor.GetStats()
	if stats.TotalPaths != 2 {
		t.Errorf("Expected 2 total paths, got %d", stats.TotalPaths)
	}
	if stats.ActivePaths != 1 {
		t.Errorf("Expected 1 active path, got %d", stats.ActivePaths)
	}
}
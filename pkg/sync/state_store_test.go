package sync

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSyncStateStore_CreateAndLoad(t *testing.T) {
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "sync_state_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store, err := NewSyncStateStore(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state store: %v", err)
	}

	// Test creating initial state
	syncID := "test-sync-1"
	localPath := "/local/test"
	remotePath := "/remote/test"

	err = store.CreateInitialState(syncID, localPath, remotePath)
	if err != nil {
		t.Fatalf("Failed to create initial state: %v", err)
	}

	// Test loading the state
	state, err := store.LoadState(syncID)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}

	if state.LocalPath != localPath {
		t.Errorf("Expected LocalPath %s, got %s", localPath, state.LocalPath)
	}
	if state.RemotePath != remotePath {
		t.Errorf("Expected RemotePath %s, got %s", remotePath, state.RemotePath)
	}
	if !state.SyncEnabled {
		t.Error("Expected SyncEnabled to be true")
	}
}

func TestSyncStateStore_SaveAndLoad(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sync_state_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store, err := NewSyncStateStore(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state store: %v", err)
	}

	syncID := "test-sync-2"
	now := time.Now()

	// Create a test state
	state := &SyncState{
		LocalPath:  "/local/test",
		RemotePath: "/remote/test",
		LocalSnapshot: map[string]FileMetadata{
			"file1.txt": {
				Path:    "file1.txt",
				Size:    100,
				ModTime: now,
				IsDir:   false,
			},
		},
		RemoteSnapshot: map[string]RemoteMetadata{
			"file2.txt": {
				Path:          "file2.txt",
				DescriptorCID: "test-cid",
				Size:          200,
				ModTime:       now,
				IsDir:         false,
			},
		},
		SyncHistory: []SyncOperation{
			{
				ID:        "op1",
				Type:      OpTypeUpload,
				LocalPath: "file1.txt",
				Timestamp: now,
				Status:    OpStatusCompleted,
			},
		},
		LastSync:    now,
		SyncEnabled: true,
	}

	// Save the state
	err = store.SaveState(syncID, state)
	if err != nil {
		t.Fatalf("Failed to save state: %v", err)
	}

	// Load and verify
	loadedState, err := store.LoadState(syncID)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}

	if len(loadedState.LocalSnapshot) != 1 {
		t.Errorf("Expected 1 local snapshot entry, got %d", len(loadedState.LocalSnapshot))
	}
	if len(loadedState.RemoteSnapshot) != 1 {
		t.Errorf("Expected 1 remote snapshot entry, got %d", len(loadedState.RemoteSnapshot))
	}
	if len(loadedState.SyncHistory) != 1 {
		t.Errorf("Expected 1 history entry, got %d", len(loadedState.SyncHistory))
	}
}

func TestSyncStateStore_PendingOperations(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sync_state_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store, err := NewSyncStateStore(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state store: %v", err)
	}

	syncID := "test-sync-3"
	err = store.CreateInitialState(syncID, "/local", "/remote")
	if err != nil {
		t.Fatalf("Failed to create initial state: %v", err)
	}

	// Add pending operation
	op := SyncOperation{
		ID:        "pending-op-1",
		Type:      OpTypeUpload,
		LocalPath: "test.txt",
		Timestamp: time.Now(),
		Status:    OpStatusPending,
	}

	err = store.AddPendingOperation(syncID, op)
	if err != nil {
		t.Fatalf("Failed to add pending operation: %v", err)
	}

	// Get pending operations
	pendingOps, err := store.GetPendingOperations(syncID)
	if err != nil {
		t.Fatalf("Failed to get pending operations: %v", err)
	}

	if len(pendingOps) != 1 {
		t.Errorf("Expected 1 pending operation, got %d", len(pendingOps))
	}
	if pendingOps[0].ID != "pending-op-1" {
		t.Errorf("Expected operation ID 'pending-op-1', got '%s'", pendingOps[0].ID)
	}

	// Remove pending operation
	err = store.RemovePendingOperation(syncID, "pending-op-1")
	if err != nil {
		t.Fatalf("Failed to remove pending operation: %v", err)
	}

	// Verify removal
	pendingOps, err = store.GetPendingOperations(syncID)
	if err != nil {
		t.Fatalf("Failed to get pending operations after removal: %v", err)
	}

	if len(pendingOps) != 0 {
		t.Errorf("Expected 0 pending operations after removal, got %d", len(pendingOps))
	}
}

func TestSyncStateStore_UpdateSnapshot(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sync_state_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store, err := NewSyncStateStore(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state store: %v", err)
	}

	syncID := "test-sync-4"
	err = store.CreateInitialState(syncID, "/local", "/remote")
	if err != nil {
		t.Fatalf("Failed to create initial state: %v", err)
	}

	// Update local snapshot
	localMeta := FileMetadata{
		Path:    "local-file.txt",
		Size:    150,
		ModTime: time.Now(),
		IsDir:   false,
	}

	err = store.UpdateSnapshot(syncID, true, "local-file.txt", localMeta)
	if err != nil {
		t.Fatalf("Failed to update local snapshot: %v", err)
	}

	// Update remote snapshot
	remoteMeta := RemoteMetadata{
		Path:          "remote-file.txt",
		DescriptorCID: "remote-cid",
		Size:          250,
		ModTime:       time.Now(),
		IsDir:         false,
	}

	err = store.UpdateSnapshot(syncID, false, "remote-file.txt", remoteMeta)
	if err != nil {
		t.Fatalf("Failed to update remote snapshot: %v", err)
	}

	// Verify updates
	state, err := store.LoadState(syncID)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}

	if len(state.LocalSnapshot) != 1 {
		t.Errorf("Expected 1 local snapshot entry, got %d", len(state.LocalSnapshot))
	}
	if len(state.RemoteSnapshot) != 1 {
		t.Errorf("Expected 1 remote snapshot entry, got %d", len(state.RemoteSnapshot))
	}

	if state.LocalSnapshot["local-file.txt"].Size != 150 {
		t.Errorf("Expected local file size 150, got %d", state.LocalSnapshot["local-file.txt"].Size)
	}
	if state.RemoteSnapshot["remote-file.txt"].Size != 250 {
		t.Errorf("Expected remote file size 250, got %d", state.RemoteSnapshot["remote-file.txt"].Size)
	}
}

func TestSyncStateStore_HistoryManagement(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sync_state_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store, err := NewSyncStateStore(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state store: %v", err)
	}

	syncID := "test-sync-5"
	err = store.CreateInitialState(syncID, "/local", "/remote")
	if err != nil {
		t.Fatalf("Failed to create initial state: %v", err)
	}

	// Add multiple history entries
	for i := 0; i < 5; i++ {
		op := SyncOperation{
			ID:        "history-op-" + string(rune('1'+i)),
			Type:      OpTypeUpload,
			LocalPath: "file" + string(rune('1'+i)) + ".txt",
			Timestamp: time.Now().Add(time.Duration(i) * time.Minute),
			Status:    OpStatusCompleted,
		}

		err = store.AddToHistory(syncID, op)
		if err != nil {
			t.Fatalf("Failed to add history operation %d: %v", i, err)
		}
	}

	// Verify history
	state, err := store.LoadState(syncID)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}

	if len(state.SyncHistory) != 5 {
		t.Errorf("Expected 5 history entries, got %d", len(state.SyncHistory))
	}
}

func TestSyncStateStore_ListStates(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sync_state_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store, err := NewSyncStateStore(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state store: %v", err)
	}

	// Create multiple sync states
	syncIDs := []string{"sync-1", "sync-2", "sync-3"}
	for _, syncID := range syncIDs {
		err = store.CreateInitialState(syncID, "/local/"+syncID, "/remote/"+syncID)
		if err != nil {
			t.Fatalf("Failed to create state for %s: %v", syncID, err)
		}
	}

	// List states
	listedIDs, err := store.ListStates()
	if err != nil {
		t.Fatalf("Failed to list states: %v", err)
	}

	if len(listedIDs) != 3 {
		t.Errorf("Expected 3 states, got %d", len(listedIDs))
	}

	// Verify all IDs are present
	for _, expectedID := range syncIDs {
		found := false
		for _, listedID := range listedIDs {
			if listedID == expectedID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected sync ID %s not found in list", expectedID)
		}
	}
}

func TestSyncStateStore_DeleteState(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sync_state_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store, err := NewSyncStateStore(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state store: %v", err)
	}

	syncID := "test-sync-delete"
	err = store.CreateInitialState(syncID, "/local", "/remote")
	if err != nil {
		t.Fatalf("Failed to create initial state: %v", err)
	}

	// Verify state exists
	_, err = store.LoadState(syncID)
	if err != nil {
		t.Fatalf("Failed to load state before deletion: %v", err)
	}

	// Delete state
	err = store.DeleteState(syncID)
	if err != nil {
		t.Fatalf("Failed to delete state: %v", err)
	}

	// Verify state no longer exists
	_, err = store.LoadState(syncID)
	if err == nil {
		t.Error("Expected error when loading deleted state, got nil")
	}

	// Verify file is removed
	stateFile := filepath.Join(tempDir, syncID+".json")
	if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
		t.Error("Expected state file to be deleted")
	}
}

func TestSyncStateStore_UpdateLastSync(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sync_state_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store, err := NewSyncStateStore(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state store: %v", err)
	}

	syncID := "test-sync-timestamp"
	err = store.CreateInitialState(syncID, "/local", "/remote")
	if err != nil {
		t.Fatalf("Failed to create initial state: %v", err)
	}

	// Update last sync timestamp
	now := time.Now()
	err = store.UpdateLastSync(syncID, now)
	if err != nil {
		t.Fatalf("Failed to update last sync: %v", err)
	}

	// Verify update
	state, err := store.LoadState(syncID)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}

	if !state.LastSync.Equal(now) {
		t.Errorf("Expected LastSync %v, got %v", now, state.LastSync)
	}
}
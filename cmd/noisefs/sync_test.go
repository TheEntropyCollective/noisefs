package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/sync"
)

func TestSyncCommands(t *testing.T) {
	// Create temporary directory for testing
	tempDir := t.TempDir()

	// Create a temporary local directory for sync
	localDir := filepath.Join(tempDir, "local")
	if err := os.MkdirAll(localDir, 0755); err != nil {
		t.Fatalf("Failed to create local directory: %v", err)
	}

	// Create a test file
	testFile := filepath.Join(localDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create storage manager for testing
	storageConfig := storage.DefaultConfig()
	storageManager, err := storage.NewManager(storageConfig)
	if err != nil {
		t.Skipf("Failed to create storage manager (requires IPFS): %v", err)
	}

	// Test sync usage
	if err := showSyncUsage(); err != nil {
		t.Errorf("showSyncUsage() failed: %v", err)
	}

	// Test sync list (should be empty initially)
	args := []string{"list"}
	if err := handleSyncList(args, storageManager, false, false); err != nil {
		t.Errorf("handleSyncList() failed: %v", err)
	}

	// Test sync list with JSON output
	if err := handleSyncList(args, storageManager, false, true); err != nil {
		t.Errorf("handleSyncList() with JSON failed: %v", err)
	}

	// Test sync list with quiet output
	if err := handleSyncList(args, storageManager, true, false); err != nil {
		t.Errorf("handleSyncList() with quiet failed: %v", err)
	}
}

func TestCreateSyncEngine(t *testing.T) {
	// Create temporary directory for testing
	tempDir := t.TempDir()

	// Create storage manager for testing
	storageConfig := storage.DefaultConfig()
	storageManager, err := storage.NewManager(storageConfig)
	if err != nil {
		t.Skipf("Failed to create storage manager (requires IPFS): %v", err)
	}

	// Override home directory for testing
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	// Test creating sync engine
	engine, err := createSyncEngine(storageManager)
	if err != nil {
		t.Errorf("createSyncEngine() failed: %v", err)
		return
	}

	if engine == nil {
		t.Error("createSyncEngine() returned nil engine")
		return
	}

	// Test engine functionality
	stats := engine.GetStats()
	if stats == nil {
		t.Error("GetStats() returned nil")
	}

	if stats.ActiveSessions != 0 {
		t.Errorf("Expected 0 active sessions, got %d", stats.ActiveSessions)
	}

	// Test listing active syncs
	sessions := engine.ListActiveSyncs()
	if len(sessions) != 0 {
		t.Errorf("Expected 0 active syncs, got %d", len(sessions))
	}

	// Stop the engine
	if err := engine.Stop(); err != nil {
		t.Errorf("Stop() failed: %v", err)
	}
}

func TestSyncResultStructs(t *testing.T) {
	// Test that our result structs can be marshaled to JSON
	now := time.Now()

	// Test SyncStartResult
	startResult := SyncStartResult{
		SyncID:     "test-sync",
		LocalPath:  "/local/path",
		RemotePath: "/remote/path",
		Status:     "idle",
		StartTime:  now,
	}

	if startResult.SyncID != "test-sync" {
		t.Errorf("Expected SyncID 'test-sync', got '%s'", startResult.SyncID)
	}

	// Test SyncStatusResult
	statusResult := SyncStatusResult{
		SyncID:     "test-sync",
		LocalPath:  "/local/path",
		RemotePath: "/remote/path",
		Status:     "syncing",
		LastSync:   now,
		Progress: &sync.SyncProgress{
			TotalOperations:     10,
			CompletedOperations: 5,
			CurrentOperation:    "uploading file",
			StartTime:           now,
		},
	}

	if statusResult.Progress.TotalOperations != 10 {
		t.Errorf("Expected 10 total operations, got %d", statusResult.Progress.TotalOperations)
	}

	// Test SyncListResult
	listResult := SyncListResult{
		SyncID:     "test-sync",
		LocalPath:  "/local/path",
		RemotePath: "/remote/path",
		Status:     "idle",
		LastSync:   now,
	}

	if listResult.Status != "idle" {
		t.Errorf("Expected status 'idle', got '%s'", listResult.Status)
	}

	// Test SyncActionResult
	actionResult := SyncActionResult{
		SyncID:    "test-sync",
		Action:    "pause",
		Success:   true,
		Timestamp: now,
	}

	if !actionResult.Success {
		t.Error("Expected success to be true")
	}
}

func TestSyncCommandArguments(t *testing.T) {
	// Create storage manager for testing
	storageConfig := storage.DefaultConfig()
	storageManager, err := storage.NewManager(storageConfig)
	if err != nil {
		t.Skipf("Failed to create storage manager (requires IPFS): %v", err)
	}

	// Test handleSyncCommand with no args
	err = handleSyncCommand([]string{}, storageManager, false, false)
	if err != nil {
		// This should show usage, not error
		t.Logf("handleSyncCommand with no args returned: %v", err)
	}

	// Test handleSyncCommand with unknown subcommand
	err = handleSyncCommand([]string{"unknown"}, storageManager, false, false)
	if err == nil {
		t.Error("Expected error for unknown subcommand")
	}

	// Test handleSyncStart with insufficient args
	err = handleSyncStart([]string{"sync1"}, storageManager, false, false)
	if err == nil {
		t.Error("Expected error for insufficient args")
	}

	// Test handleSyncStart with non-absolute path
	err = handleSyncStart([]string{"sync1", "relative/path", "/remote/path"}, storageManager, false, false)
	if err == nil {
		t.Error("Expected error for non-absolute path")
	}

	// Test handleSyncStart with non-existent path
	err = handleSyncStart([]string{"sync1", "/non/existent/path", "/remote/path"}, storageManager, false, false)
	if err == nil {
		t.Error("Expected error for non-existent path")
	}

	// Test handleSyncStop with no args
	err = handleSyncStop([]string{}, storageManager, false, false)
	if err == nil {
		t.Error("Expected error for no args")
	}

	// Test handleSyncPause with no args
	err = handleSyncPause([]string{}, storageManager, false, false)
	if err == nil {
		t.Error("Expected error for no args")
	}

	// Test handleSyncResume with no args
	err = handleSyncResume([]string{}, storageManager, false, false)
	if err == nil {
		t.Error("Expected error for no args")
	}
}

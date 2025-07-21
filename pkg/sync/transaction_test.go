package sync

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTransactionManagerBasicTransaction(t *testing.T) {
	// Setup test environment
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, "state")
	txDir := filepath.Join(tmpDir, "transactions")

	// Create state store
	stateStore, err := NewSyncStateStore(stateDir)
	if err != nil {
		t.Fatalf("Failed to create state store: %v", err)
	}

	// Create transaction manager
	txManager, err := NewTransactionManager(stateStore, txDir)
	if err != nil {
		t.Fatalf("Failed to create transaction manager: %v", err)
	}

	// Create initial sync state
	syncID := "test_sync"
	if err := stateStore.CreateInitialState(syncID, "/local", "/remote"); err != nil {
		t.Fatalf("Failed to create initial state: %v", err)
	}

	// Begin transaction
	tx, err := txManager.BeginTransaction(syncID)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	if tx.Status != TxStatusPending {
		t.Errorf("Expected pending status, got %s", tx.Status)
	}

	// Add operation
	testMetadata := FileMetadata{
		Path:     "test.txt",
		Size:     100,
		ModTime:  time.Now(),
		Checksum: "abc123",
	}

	err = txManager.AddOperation(tx, TxOpUpdateLocalSnapshot, "test.txt", nil, testMetadata)
	if err != nil {
		t.Fatalf("Failed to add operation: %v", err)
	}

	if len(tx.Operations) != 1 {
		t.Errorf("Expected 1 operation, got %d", len(tx.Operations))
	}

	// Commit transaction
	err = txManager.CommitTransaction(tx)
	if err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}

	if tx.Status != TxStatusCommitted {
		t.Errorf("Expected committed status, got %s", tx.Status)
	}

	// Verify state was updated
	state, err := stateStore.LoadState(syncID)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}

	if len(state.LocalSnapshot) != 1 {
		t.Errorf("Expected 1 file in local snapshot, got %d", len(state.LocalSnapshot))
	}

	if file, exists := state.LocalSnapshot["test.txt"]; !exists {
		t.Error("Expected test.txt in local snapshot")
	} else if file.Checksum != "abc123" {
		t.Errorf("Expected checksum abc123, got %s", file.Checksum)
	}
}

func TestTransactionRollback(t *testing.T) {
	// Setup test environment
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, "state")
	txDir := filepath.Join(tmpDir, "transactions")

	stateStore, err := NewSyncStateStore(stateDir)
	if err != nil {
		t.Fatalf("Failed to create state store: %v", err)
	}

	txManager, err := NewTransactionManager(stateStore, txDir)
	if err != nil {
		t.Fatalf("Failed to create transaction manager: %v", err)
	}

	syncID := "test_sync"
	if err := stateStore.CreateInitialState(syncID, "/local", "/remote"); err != nil {
		t.Fatalf("Failed to create initial state: %v", err)
	}

	// Begin transaction
	tx, err := txManager.BeginTransaction(syncID)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	// Add operation
	testMetadata := FileMetadata{
		Path:     "test.txt",
		Size:     100,
		ModTime:  time.Now(),
		Checksum: "abc123",
	}

	err = txManager.AddOperation(tx, TxOpUpdateLocalSnapshot, "test.txt", nil, testMetadata)
	if err != nil {
		t.Fatalf("Failed to add operation: %v", err)
	}

	// Rollback transaction
	err = txManager.RollbackTransaction(tx)
	if err != nil {
		t.Fatalf("Failed to rollback transaction: %v", err)
	}

	if tx.Status != TxStatusRolledBack {
		t.Errorf("Expected rolled back status, got %s", tx.Status)
	}

	// Verify state was not modified
	state, err := stateStore.LoadState(syncID)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}

	if len(state.LocalSnapshot) != 0 {
		t.Errorf("Expected 0 files in local snapshot after rollback, got %d", len(state.LocalSnapshot))
	}
}

func TestTransactionRecovery(t *testing.T) {
	// Setup test environment
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, "state")
	txDir := filepath.Join(tmpDir, "transactions")

	// Create incomplete transaction log manually
	txID := "tx_test_sync_12345"

	// Write incomplete transaction log
	if err := os.MkdirAll(txDir, 0755); err != nil {
		t.Fatalf("Failed to create tx dir: %v", err)
	}

	stateStore, err := NewSyncStateStore(stateDir)
	if err != nil {
		t.Fatalf("Failed to create state store: %v", err)
	}

	// Manually write incomplete transaction
	logFile := filepath.Join(txDir, txID+".json")
	txData := `{
		"id": "` + txID + `",
		"sync_id": "test_sync",
		"status": "pending",
		"start_time": "` + time.Now().Format(time.RFC3339) + `",
		"operations": []
	}`

	if err := os.WriteFile(logFile, []byte(txData), 0644); err != nil {
		t.Fatalf("Failed to write incomplete transaction: %v", err)
	}

	// Create transaction manager (should recover the incomplete transaction)
	txManager, err := NewTransactionManager(stateStore, txDir)
	if err != nil {
		t.Fatalf("Failed to create transaction manager: %v", err)
	}

	// Verify transaction log was cleaned up
	if _, err := os.Stat(logFile); !os.IsNotExist(err) {
		t.Error("Expected transaction log to be cleaned up after recovery")
	}

	// Verify no active transactions
	if len(txManager.activeTx) != 0 {
		t.Errorf("Expected no active transactions after recovery, got %d", len(txManager.activeTx))
	}
}

func TestAtomicBatchUpdate(t *testing.T) {
	// Setup test environment
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, "state")

	stateStore, err := NewSyncStateStore(stateDir)
	if err != nil {
		t.Fatalf("Failed to create state store: %v", err)
	}

	syncID := "test_sync"
	if err := stateStore.CreateInitialState(syncID, "/local", "/remote"); err != nil {
		t.Fatalf("Failed to create initial state: %v", err)
	}

	// Prepare batch operations
	operations := []BatchOperation{
		{
			Type: TxOpUpdateLocalSnapshot,
			Path: "file1.txt",
			NewData: FileMetadata{
				Path:     "file1.txt",
				Size:     100,
				Checksum: "abc123",
			},
		},
		{
			Type: TxOpUpdateLocalSnapshot,
			Path: "file2.txt",
			NewData: FileMetadata{
				Path:     "file2.txt",
				Size:     200,
				Checksum: "def456",
			},
		},
		{
			Type: TxOpUpdateRemoteSnapshot,
			Path: "remote.txt",
			NewData: RemoteMetadata{
				Path:          "remote.txt",
				DescriptorCID: "QmTest123",
				Size:          300,
			},
		},
	}

	// Execute batch update
	err = stateStore.AtomicBatchUpdate(syncID, operations)
	if err != nil {
		t.Fatalf("Failed to execute batch update: %v", err)
	}

	// Verify all updates were applied
	state, err := stateStore.LoadState(syncID)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}

	if len(state.LocalSnapshot) != 2 {
		t.Errorf("Expected 2 local files, got %d", len(state.LocalSnapshot))
	}

	if len(state.RemoteSnapshot) != 1 {
		t.Errorf("Expected 1 remote file, got %d", len(state.RemoteSnapshot))
	}

	// Verify specific files
	if file, exists := state.LocalSnapshot["file1.txt"]; !exists {
		t.Error("Expected file1.txt in local snapshot")
	} else if file.Checksum != "abc123" {
		t.Errorf("Expected checksum abc123, got %s", file.Checksum)
	}

	if file, exists := state.RemoteSnapshot["remote.txt"]; !exists {
		t.Error("Expected remote.txt in remote snapshot")
	} else if file.DescriptorCID != "QmTest123" {
		t.Errorf("Expected CID QmTest123, got %s", file.DescriptorCID)
	}
}

package sync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Transaction represents an atomic state update transaction
type Transaction struct {
	ID           string                 `json:"id"`
	SyncID       string                 `json:"sync_id"`
	Operations   []TransactionOperation `json:"operations"`
	Status       TransactionStatus      `json:"status"`
	StartTime    time.Time              `json:"start_time"`
	CommitTime   time.Time              `json:"commit_time,omitempty"`
	RollbackTime time.Time              `json:"rollback_time,omitempty"`
}

// TransactionOperation represents a single operation within a transaction
type TransactionOperation struct {
	Type    TransactionOpType `json:"type"`
	Path    string            `json:"path"`
	OldData interface{}       `json:"old_data,omitempty"`
	NewData interface{}       `json:"new_data,omitempty"`
}

// TransactionOpType represents the type of transaction operation
type TransactionOpType string

const (
	TxOpUpdateLocalSnapshot  TransactionOpType = "update_local_snapshot"
	TxOpUpdateRemoteSnapshot TransactionOpType = "update_remote_snapshot"
	TxOpDeleteLocalSnapshot  TransactionOpType = "delete_local_snapshot"
	TxOpDeleteRemoteSnapshot TransactionOpType = "delete_remote_snapshot"
	TxOpUpdateState          TransactionOpType = "update_state"
	TxOpAddPendingOp         TransactionOpType = "add_pending_op"
	TxOpRemovePendingOp      TransactionOpType = "remove_pending_op"
)

// TransactionStatus represents the status of a transaction
type TransactionStatus string

const (
	TxStatusPending    TransactionStatus = "pending"
	TxStatusCommitted  TransactionStatus = "committed"
	TxStatusRolledBack TransactionStatus = "rolled_back"
)

// TransactionManager manages atomic state updates with write-ahead logging
type TransactionManager struct {
	stateStore *SyncStateStore
	txDir      string
	mu         sync.RWMutex
	activeTx   map[string]*Transaction
}

// NewTransactionManager creates a new transaction manager
func NewTransactionManager(stateStore *SyncStateStore, txDir string) (*TransactionManager, error) {
	if err := os.MkdirAll(txDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create transaction directory: %w", err)
	}

	tm := &TransactionManager{
		stateStore: stateStore,
		txDir:      txDir,
		activeTx:   make(map[string]*Transaction),
	}

	// Recover any incomplete transactions on startup
	if err := tm.recoverTransactions(); err != nil {
		return nil, fmt.Errorf("failed to recover transactions: %w", err)
	}

	return tm, nil
}

// BeginTransaction starts a new atomic transaction
func (tm *TransactionManager) BeginTransaction(syncID string) (*Transaction, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	txID := generateTransactionID(syncID)
	tx := &Transaction{
		ID:         txID,
		SyncID:     syncID,
		Operations: make([]TransactionOperation, 0),
		Status:     TxStatusPending,
		StartTime:  time.Now(),
	}

	tm.activeTx[txID] = tx

	// Write transaction log
	if err := tm.writeTransactionLog(tx); err != nil {
		delete(tm.activeTx, txID)
		return nil, fmt.Errorf("failed to write transaction log: %w", err)
	}

	return tx, nil
}

// AddOperation adds an operation to the transaction
func (tm *TransactionManager) AddOperation(tx *Transaction, opType TransactionOpType, path string, oldData, newData interface{}) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	op := TransactionOperation{
		Type:    opType,
		Path:    path,
		OldData: oldData,
		NewData: newData,
	}

	tx.Operations = append(tx.Operations, op)

	// Update transaction log
	return tm.writeTransactionLog(tx)
}

// CommitTransaction atomically commits all operations in the transaction
func (tm *TransactionManager) CommitTransaction(tx *Transaction) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Load current state
	state, err := tm.stateStore.LoadState(tx.SyncID)
	if err != nil {
		return fmt.Errorf("failed to load state for commit: %w", err)
	}

	// Create backup of current state
	backup := tm.createStateBackup(state)

	// Apply all operations
	for _, op := range tx.Operations {
		if err := tm.applyOperation(state, op); err != nil {
			// Rollback on error
			tm.restoreStateBackup(state, backup)
			return fmt.Errorf("failed to apply operation %s: %w", op.Type, err)
		}
	}

	// Save updated state
	if err := tm.stateStore.SaveState(tx.SyncID, state); err != nil {
		// Rollback on error
		tm.restoreStateBackup(state, backup)
		return fmt.Errorf("failed to save state after commit: %w", err)
	}

	// Mark transaction as committed
	tx.Status = TxStatusCommitted
	tx.CommitTime = time.Now()

	// Update transaction log
	if err := tm.writeTransactionLog(tx); err != nil {
		// Log error but don't fail the commit since state is already saved
		fmt.Printf("Warning: failed to update transaction log after commit: %v\n", err)
	}

	// Remove from active transactions
	delete(tm.activeTx, tx.ID)

	// Clean up transaction log
	tm.cleanupTransactionLog(tx.ID)

	return nil
}

// RollbackTransaction rolls back all operations in the transaction
func (tm *TransactionManager) RollbackTransaction(tx *Transaction) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Mark transaction as rolled back
	tx.Status = TxStatusRolledBack
	tx.RollbackTime = time.Now()

	// Update transaction log
	if err := tm.writeTransactionLog(tx); err != nil {
		fmt.Printf("Warning: failed to update transaction log after rollback: %v\n", err)
	}

	// Remove from active transactions
	delete(tm.activeTx, tx.ID)

	// Clean up transaction log
	tm.cleanupTransactionLog(tx.ID)

	return nil
}

// applyOperation applies a single operation to the state
func (tm *TransactionManager) applyOperation(state *SyncState, op TransactionOperation) error {
	switch op.Type {
	case TxOpUpdateLocalSnapshot:
		if fileMeta, ok := op.NewData.(FileMetadata); ok {
			state.LocalSnapshot[op.Path] = fileMeta
		} else {
			return fmt.Errorf("invalid data type for local snapshot update")
		}

	case TxOpUpdateRemoteSnapshot:
		if remoteMeta, ok := op.NewData.(RemoteMetadata); ok {
			state.RemoteSnapshot[op.Path] = remoteMeta
		} else {
			return fmt.Errorf("invalid data type for remote snapshot update")
		}

	case TxOpDeleteLocalSnapshot:
		delete(state.LocalSnapshot, op.Path)

	case TxOpDeleteRemoteSnapshot:
		delete(state.RemoteSnapshot, op.Path)

	case TxOpAddPendingOp:
		if syncOp, ok := op.NewData.(SyncOperation); ok {
			state.PendingOps = append(state.PendingOps, syncOp)
		} else {
			return fmt.Errorf("invalid data type for pending operation")
		}

	case TxOpRemovePendingOp:
		// Remove pending operation by ID
		if opID, ok := op.NewData.(string); ok {
			for i, pendingOp := range state.PendingOps {
				if pendingOp.ID == opID {
					state.PendingOps = append(state.PendingOps[:i], state.PendingOps[i+1:]...)
					break
				}
			}
		} else {
			return fmt.Errorf("invalid data type for pending operation removal")
		}

	default:
		return fmt.Errorf("unsupported operation type: %s", op.Type)
	}

	return nil
}

// createStateBackup creates a backup copy of the state for rollback purposes
func (tm *TransactionManager) createStateBackup(state *SyncState) *SyncState {
	backup := &SyncState{
		LocalPath:      state.LocalPath,
		RemotePath:     state.RemotePath,
		LocalSnapshot:  make(map[string]FileMetadata),
		RemoteSnapshot: make(map[string]RemoteMetadata),
		SyncHistory:    make([]SyncOperation, len(state.SyncHistory)),
		PendingOps:     make([]SyncOperation, len(state.PendingOps)),
		LastSync:       state.LastSync,
		SyncEnabled:    state.SyncEnabled,
	}

	// Deep copy snapshots
	for path, meta := range state.LocalSnapshot {
		backup.LocalSnapshot[path] = meta
	}
	for path, meta := range state.RemoteSnapshot {
		backup.RemoteSnapshot[path] = meta
	}

	// Copy slices
	copy(backup.SyncHistory, state.SyncHistory)
	copy(backup.PendingOps, state.PendingOps)

	return backup
}

// restoreStateBackup restores state from backup
func (tm *TransactionManager) restoreStateBackup(state *SyncState, backup *SyncState) {
	state.LocalSnapshot = backup.LocalSnapshot
	state.RemoteSnapshot = backup.RemoteSnapshot
	state.SyncHistory = backup.SyncHistory
	state.PendingOps = backup.PendingOps
	state.LastSync = backup.LastSync
	state.SyncEnabled = backup.SyncEnabled
}

// writeTransactionLog writes transaction to the write-ahead log
func (tm *TransactionManager) writeTransactionLog(tx *Transaction) error {
	logFile := filepath.Join(tm.txDir, tx.ID+".json")
	data, err := json.MarshalIndent(tx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(logFile, data, 0644)
}

// cleanupTransactionLog removes the transaction log file
func (tm *TransactionManager) cleanupTransactionLog(txID string) {
	logFile := filepath.Join(tm.txDir, txID+".json")
	os.Remove(logFile) // Ignore errors
}

// recoverTransactions recovers incomplete transactions on startup
func (tm *TransactionManager) recoverTransactions() error {
	files, err := os.ReadDir(tm.txDir)
	if err != nil {
		return err
	}

	for _, file := range files {
		if filepath.Ext(file.Name()) == ".json" {
			if err := tm.recoverTransaction(file.Name()); err != nil {
				fmt.Printf("Warning: failed to recover transaction %s: %v\n", file.Name(), err)
			}
		}
	}

	return nil
}

// recoverTransaction recovers a single transaction
func (tm *TransactionManager) recoverTransaction(filename string) error {
	logFile := filepath.Join(tm.txDir, filename)
	data, err := os.ReadFile(logFile)
	if err != nil {
		return err
	}

	var tx Transaction
	if err := json.Unmarshal(data, &tx); err != nil {
		return err
	}

	// Only recover pending transactions
	if tx.Status == TxStatusPending {
		// Rollback incomplete transactions
		fmt.Printf("Rolling back incomplete transaction: %s\n", tx.ID)
		tx.Status = TxStatusRolledBack
		tx.RollbackTime = time.Now()
		tm.writeTransactionLog(&tx)
	}

	// Clean up the log file
	tm.cleanupTransactionLog(tx.ID)
	return nil
}

// generateTransactionID generates a unique transaction ID
func generateTransactionID(syncID string) string {
	return fmt.Sprintf("tx_%s_%d", syncID, time.Now().UnixNano())
}

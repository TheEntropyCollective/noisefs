package sync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SyncStateStore manages persistent storage of sync state
type SyncStateStore struct {
	stateDir  string
	mu        sync.RWMutex
	cache     map[string]*SyncState
	txManager *TransactionManager
}

// NewSyncStateStore creates a new SyncStateStore
func NewSyncStateStore(stateDir string) (*SyncStateStore, error) {
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create state directory: %w", err)
	}

	store := &SyncStateStore{
		stateDir: stateDir,
		cache:    make(map[string]*SyncState),
	}

	// Initialize transaction manager
	txDir := filepath.Join(stateDir, "transactions")
	txManager, err := NewTransactionManager(store, txDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction manager: %w", err)
	}
	store.txManager = txManager

	return store, nil
}

// SaveState saves sync state to persistent storage
func (s *SyncStateStore) SaveState(syncID string, state *SyncState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update cache
	s.cache[syncID] = state

	// Save to file
	stateFile := s.getStateFile(syncID)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal sync state: %w", err)
	}

	if err := os.WriteFile(stateFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write sync state file: %w", err)
	}

	return nil
}

// LoadState loads sync state from persistent storage
func (s *SyncStateStore) LoadState(syncID string) (*SyncState, error) {
	s.mu.RLock()

	// Check cache first
	if state, exists := s.cache[syncID]; exists {
		s.mu.RUnlock()
		return state, nil
	}
	s.mu.RUnlock()

	// Load from file
	stateFile := s.getStateFile(syncID)
	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("sync state not found: %s", syncID)
	}

	data, err := os.ReadFile(stateFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read sync state file: %w", err)
	}

	var state SyncState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal sync state: %w", err)
	}

	// Update cache
	s.mu.Lock()
	s.cache[syncID] = &state
	s.mu.Unlock()

	return &state, nil
}

// DeleteState removes sync state from persistent storage
func (s *SyncStateStore) DeleteState(syncID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove from cache
	delete(s.cache, syncID)

	// Remove file
	stateFile := s.getStateFile(syncID)
	if err := os.Remove(stateFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete sync state file: %w", err)
	}

	return nil
}

// ListStates returns a list of all sync state IDs
func (s *SyncStateStore) ListStates() ([]string, error) {
	files, err := os.ReadDir(s.stateDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read state directory: %w", err)
	}

	var syncIDs []string
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".json" {
			syncID := file.Name()[:len(file.Name())-5] // Remove .json extension
			syncIDs = append(syncIDs, syncID)
		}
	}

	return syncIDs, nil
}

// UpdateLastSync updates the last sync timestamp for a sync state
func (s *SyncStateStore) UpdateLastSync(syncID string, timestamp time.Time) error {
	state, err := s.LoadState(syncID)
	if err != nil {
		return err
	}

	state.LastSync = timestamp
	return s.SaveState(syncID, state)
}

// AddPendingOperation adds a pending operation to the sync state
func (s *SyncStateStore) AddPendingOperation(syncID string, op SyncOperation) error {
	state, err := s.LoadState(syncID)
	if err != nil {
		return err
	}

	state.PendingOps = append(state.PendingOps, op)
	return s.SaveState(syncID, state)
}

// RemovePendingOperation removes a pending operation from the sync state
func (s *SyncStateStore) RemovePendingOperation(syncID string, opID string) error {
	state, err := s.LoadState(syncID)
	if err != nil {
		return err
	}

	// Find and remove the operation
	for i, op := range state.PendingOps {
		if op.ID == opID {
			state.PendingOps = append(state.PendingOps[:i], state.PendingOps[i+1:]...)
			break
		}
	}

	return s.SaveState(syncID, state)
}

// AddToHistory adds a completed operation to the sync history
func (s *SyncStateStore) AddToHistory(syncID string, op SyncOperation) error {
	state, err := s.LoadState(syncID)
	if err != nil {
		return err
	}

	state.SyncHistory = append(state.SyncHistory, op)

	// Keep only recent history (last 1000 operations)
	if len(state.SyncHistory) > 1000 {
		state.SyncHistory = state.SyncHistory[len(state.SyncHistory)-1000:]
	}

	return s.SaveState(syncID, state)
}

// UpdateSnapshot updates the local or remote snapshot
func (s *SyncStateStore) UpdateSnapshot(syncID string, isLocal bool, path string, metadata interface{}) error {
	state, err := s.LoadState(syncID)
	if err != nil {
		return err
	}

	if isLocal {
		if fileMeta, ok := metadata.(FileMetadata); ok {
			state.LocalSnapshot[path] = fileMeta
		} else {
			return fmt.Errorf("invalid local metadata type")
		}
	} else {
		if remoteMeta, ok := metadata.(RemoteMetadata); ok {
			state.RemoteSnapshot[path] = remoteMeta
		} else {
			return fmt.Errorf("invalid remote metadata type")
		}
	}

	return s.SaveState(syncID, state)
}

// GetPendingOperations returns all pending operations for a sync state
func (s *SyncStateStore) GetPendingOperations(syncID string) ([]SyncOperation, error) {
	state, err := s.LoadState(syncID)
	if err != nil {
		return nil, err
	}

	return state.PendingOps, nil
}

// CreateInitialState creates an initial sync state
func (s *SyncStateStore) CreateInitialState(syncID, localPath, remotePath string) error {
	state := &SyncState{
		LocalPath:      localPath,
		RemotePath:     remotePath,
		LocalSnapshot:  make(map[string]FileMetadata),
		RemoteSnapshot: make(map[string]RemoteMetadata),
		SyncHistory:    make([]SyncOperation, 0),
		PendingOps:     make([]SyncOperation, 0),
		LastSync:       time.Time{},
		SyncEnabled:    true,
	}

	return s.SaveState(syncID, state)
}

// getStateFile returns the file path for a sync state
func (s *SyncStateStore) getStateFile(syncID string) string {
	return filepath.Join(s.stateDir, syncID+".json")
}

// AtomicUpdateSnapshot atomically updates either local or remote snapshot
func (s *SyncStateStore) AtomicUpdateSnapshot(syncID string, isLocal bool, updates map[string]interface{}) error {
	tx, err := s.txManager.BeginTransaction(syncID)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	for path, metadata := range updates {
		var opType TransactionOpType
		if isLocal {
			opType = TxOpUpdateLocalSnapshot
		} else {
			opType = TxOpUpdateRemoteSnapshot
		}

		if err := s.txManager.AddOperation(tx, opType, path, nil, metadata); err != nil {
			s.txManager.RollbackTransaction(tx)
			return fmt.Errorf("failed to add operation: %w", err)
		}
	}

	if err := s.txManager.CommitTransaction(tx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// AtomicBatchUpdate performs multiple atomic updates in a single transaction
func (s *SyncStateStore) AtomicBatchUpdate(syncID string, operations []BatchOperation) error {
	tx, err := s.txManager.BeginTransaction(syncID)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	for _, op := range operations {
		if err := s.txManager.AddOperation(tx, op.Type, op.Path, op.OldData, op.NewData); err != nil {
			s.txManager.RollbackTransaction(tx)
			return fmt.Errorf("failed to add operation: %w", err)
		}
	}

	if err := s.txManager.CommitTransaction(tx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// BatchOperation represents a single operation in a batch update
type BatchOperation struct {
	Type    TransactionOpType
	Path    string
	OldData interface{}
	NewData interface{}
}

// ValidateState validates the integrity of a sync state
func (s *SyncStateStore) ValidateState(syncID string) error {
	state, err := s.LoadState(syncID)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	// Validate local snapshots
	for path, metadata := range state.LocalSnapshot {
		if path == "" {
			return fmt.Errorf("empty path in local snapshot")
		}
		if metadata.Size < 0 {
			return fmt.Errorf("negative size for %s: %d", path, metadata.Size)
		}
		if !metadata.IsDir && metadata.Checksum == "" {
			return fmt.Errorf("missing checksum for file %s", path)
		}
	}

	// Validate remote snapshots
	for path, metadata := range state.RemoteSnapshot {
		if path == "" {
			return fmt.Errorf("empty path in remote snapshot")
		}
		if metadata.DescriptorCID == "" {
			return fmt.Errorf("missing descriptor CID for %s", path)
		}
		if metadata.Size < 0 {
			return fmt.Errorf("negative size for %s: %d", path, metadata.Size)
		}
	}

	// Validate pending operations
	for _, op := range state.PendingOps {
		if op.ID == "" {
			return fmt.Errorf("pending operation missing ID")
		}
		if op.LocalPath == "" && op.RemotePath == "" {
			return fmt.Errorf("pending operation missing paths")
		}
	}

	return nil
}

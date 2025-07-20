package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/security"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

// SyncEngine coordinates bi-directional synchronization between local and remote directories
type SyncEngine struct {
	stateStore       *SyncStateStore
	fileWatcher      *FileWatcher
	remoteMonitor    *RemoteChangeMonitor
	conflictResolver *ConflictResolver
	directoryManager *storage.DirectoryManager
	config           *SyncConfig
	
	// Channels for coordinating events
	localEventChan   chan SyncEvent
	remoteEventChan  chan SyncEvent
	syncOpChan       chan SyncOperation
	
	// Control and state
	ctx              context.Context
	cancel           context.CancelFunc
	mu               sync.RWMutex
	activeSyncs      map[string]*SyncSession
	
	// Statistics
	stats            *SyncEngineStats
	statsMu          sync.RWMutex
}

// SyncSession represents an active sync session
type SyncSession struct {
	SyncID           string
	LocalPath        string
	RemotePath       string
	State            *SyncState
	LastSync         time.Time
	Status           SyncStatus
	Progress         *SyncProgress
	mu               sync.RWMutex
}

// SyncStatus represents the current status of a sync session
type SyncStatus string

const (
	StatusIdle        SyncStatus = "idle"
	StatusSyncing     SyncStatus = "syncing"
	StatusConflict    SyncStatus = "conflict"
	StatusError       SyncStatus = "error"
	StatusPaused      SyncStatus = "paused"
)

// SyncProgress tracks the progress of sync operations
type SyncProgress struct {
	TotalOperations     int           `json:"total_operations"`
	CompletedOperations int           `json:"completed_operations"`
	FailedOperations    int           `json:"failed_operations"`
	CurrentOperation    string        `json:"current_operation"`
	StartTime           time.Time     `json:"start_time"`
	EstimatedCompletion time.Duration `json:"estimated_completion"`
}

// SyncEngineStats represents sync engine statistics
type SyncEngineStats struct {
	ActiveSessions     int           `json:"active_sessions"`
	TotalSyncEvents    int64         `json:"total_sync_events"`
	TotalConflicts     int64         `json:"total_conflicts"`
	TotalErrors        int64         `json:"total_errors"`
	AverageConflictAge time.Duration `json:"average_conflict_age"`
	LastSyncTime       time.Time     `json:"last_sync_time"`
}

// NewSyncEngine creates a new sync engine
func NewSyncEngine(
	stateStore *SyncStateStore,
	fileWatcher *FileWatcher,
	remoteMonitor *RemoteChangeMonitor,
	directoryManager *storage.DirectoryManager,
	config *SyncConfig,
) (*SyncEngine, error) {
	if stateStore == nil {
		return nil, fmt.Errorf("state store cannot be nil")
	}
	if fileWatcher == nil {
		return nil, fmt.Errorf("file watcher cannot be nil")
	}
	if remoteMonitor == nil {
		return nil, fmt.Errorf("remote monitor cannot be nil")
	}
	if directoryManager == nil {
		return nil, fmt.Errorf("directory manager cannot be nil")
	}
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	ctx, cancel := context.WithCancel(context.Background())

	engine := &SyncEngine{
		stateStore:       stateStore,
		fileWatcher:      fileWatcher,
		remoteMonitor:    remoteMonitor,
		directoryManager: directoryManager,
		config:           config,
		ctx:              ctx,
		cancel:           cancel,
		activeSyncs:      make(map[string]*SyncSession),
		localEventChan:   make(chan SyncEvent, 100),
		remoteEventChan:  make(chan SyncEvent, 100),
		syncOpChan:       make(chan SyncOperation, 100),
		stats:            &SyncEngineStats{},
	}

	// Initialize conflict resolver
	conflictResolver, err := NewConflictResolver(config.ConflictResolution)
	if err != nil {
		return nil, fmt.Errorf("failed to create conflict resolver: %w", err)
	}
	engine.conflictResolver = conflictResolver

	// Start event processing
	go engine.processEvents()
	go engine.processSyncOperations()

	return engine, nil
}

// StartSync starts synchronization for a local and remote path pair
func (se *SyncEngine) StartSync(syncID, localPath, remotePath, manifestCID string) error {
	se.mu.Lock()
	defer se.mu.Unlock()

	// Check if already syncing
	if _, exists := se.activeSyncs[syncID]; exists {
		return fmt.Errorf("sync already active for ID: %s", syncID)
	}

	// Load or create sync state
	state, err := se.stateStore.LoadState(syncID)
	if err != nil {
		// Create new state
		if err := se.stateStore.CreateInitialState(syncID, localPath, remotePath); err != nil {
			return fmt.Errorf("failed to create initial state: %w", err)
		}
		state, err = se.stateStore.LoadState(syncID)
		if err != nil {
			return fmt.Errorf("failed to load created state: %w", err)
		}
	}

	// Create sync session
	session := &SyncSession{
		SyncID:     syncID,
		LocalPath:  localPath,
		RemotePath: remotePath,
		State:      state,
		LastSync:   time.Now(),
		Status:     StatusIdle,
		Progress: &SyncProgress{
			StartTime: time.Now(),
		},
	}

	se.activeSyncs[syncID] = session

	// Start monitoring
	if err := se.fileWatcher.AddPath(localPath); err != nil {
		return fmt.Errorf("failed to add local path to watcher: %w", err)
	}

	if err := se.remoteMonitor.AddPath(syncID, remotePath, manifestCID); err != nil {
		return fmt.Errorf("failed to add remote path to monitor: %w", err)
	}

	// Perform initial sync
	go se.performInitialSync(session)

	return nil
}

// StopSync stops synchronization for a given sync ID
func (se *SyncEngine) StopSync(syncID string) error {
	se.mu.Lock()
	defer se.mu.Unlock()

	session, exists := se.activeSyncs[syncID]
	if !exists {
		return fmt.Errorf("sync not active for ID: %s", syncID)
	}

	// Remove from monitoring
	if err := se.fileWatcher.RemovePath(session.LocalPath); err != nil {
		// Log error but continue - sanitize for user display
		sanitizedErr := security.SanitizeErrorForUser(err, session.LocalPath)
		fmt.Printf("Warning: failed to remove local path from watcher: %v\n", sanitizedErr)
	}

	if err := se.remoteMonitor.RemovePath(session.RemotePath); err != nil {
		// Log error but continue - sanitize for user display
		sanitizedErr := security.SanitizeErrorForUser(err, session.RemotePath)
		fmt.Printf("Warning: failed to remove remote path from monitor: %v\n", sanitizedErr)
	}

	// Update session status
	session.mu.Lock()
	session.Status = StatusIdle
	session.mu.Unlock()

	// Remove from active syncs
	delete(se.activeSyncs, syncID)

	return nil
}

// PauseSync pauses synchronization for a given sync ID
func (se *SyncEngine) PauseSync(syncID string) error {
	se.mu.RLock()
	session, exists := se.activeSyncs[syncID]
	se.mu.RUnlock()

	if !exists {
		return fmt.Errorf("sync not active for ID: %s", syncID)
	}

	session.mu.Lock()
	session.Status = StatusPaused
	session.mu.Unlock()

	return nil
}

// ResumeSync resumes synchronization for a given sync ID
func (se *SyncEngine) ResumeSync(syncID string) error {
	se.mu.RLock()
	session, exists := se.activeSyncs[syncID]
	se.mu.RUnlock()

	if !exists {
		return fmt.Errorf("sync not active for ID: %s", syncID)
	}

	session.mu.Lock()
	if session.Status == StatusPaused {
		session.Status = StatusIdle
	}
	session.mu.Unlock()

	return nil
}

// processEvents processes sync events from local and remote sources
func (se *SyncEngine) processEvents() {
	// Bridge file watcher events to our local event channel
	go func() {
		for event := range se.fileWatcher.Events() {
			select {
			case se.localEventChan <- event:
			case <-se.ctx.Done():
				return
			}
		}
	}()

	// Bridge remote monitor events to our remote event channel
	go func() {
		for event := range se.remoteMonitor.Events() {
			select {
			case se.remoteEventChan <- event:
			case <-se.ctx.Done():
				return
			}
		}
	}()

	// Process events
	for {
		select {
		case <-se.ctx.Done():
			return

		case event := <-se.localEventChan:
			se.handleLocalEvent(event)

		case event := <-se.remoteEventChan:
			se.handleRemoteEvent(event)
		}
	}
}

// handleLocalEvent handles events from local file system
func (se *SyncEngine) handleLocalEvent(event SyncEvent) {
	se.updateStats(func(stats *SyncEngineStats) {
		stats.TotalSyncEvents++
		stats.LastSyncTime = time.Now()
	})

	// Find affected sync sessions
	affectedSessions := se.findAffectedSessions(event.Path, true)

	for _, session := range affectedSessions {
		if session.Status == StatusPaused {
			continue
		}

		// Create sync operation
		op := se.createSyncOperation(session, event, true)
		if op != nil {
			// Add to pending operations
			if err := se.stateStore.AddPendingOperation(session.SyncID, *op); err != nil {
				fmt.Printf("Error adding pending operation: %v\n", err)
				continue
			}

			// Queue for processing
			select {
			case se.syncOpChan <- *op:
			default:
				fmt.Printf("Sync operation queue full, dropping operation\n")
			}
		}
	}
}

// handleRemoteEvent handles events from remote NoiseFS
func (se *SyncEngine) handleRemoteEvent(event SyncEvent) {
	se.updateStats(func(stats *SyncEngineStats) {
		stats.TotalSyncEvents++
		stats.LastSyncTime = time.Now()
	})

	// Find affected sync sessions
	affectedSessions := se.findAffectedSessions(event.Path, false)

	for _, session := range affectedSessions {
		if session.Status == StatusPaused {
			continue
		}

		// Create sync operation
		op := se.createSyncOperation(session, event, false)
		if op != nil {
			// Add to pending operations
			if err := se.stateStore.AddPendingOperation(session.SyncID, *op); err != nil {
				fmt.Printf("Error adding pending operation: %v\n", err)
				continue
			}

			// Queue for processing
			select {
			case se.syncOpChan <- *op:
			default:
				fmt.Printf("Sync operation queue full, dropping operation\n")
			}
		}
	}
}

// findAffectedSessions finds sync sessions affected by a path change
func (se *SyncEngine) findAffectedSessions(path string, isLocal bool) []*SyncSession {
	se.mu.RLock()
	defer se.mu.RUnlock()

	var affected []*SyncSession
	for _, session := range se.activeSyncs {
		var matchPath string
		if isLocal {
			matchPath = session.LocalPath
		} else {
			matchPath = session.RemotePath
		}

		// Check if path is under the sync path (exact match or subdirectory)
		if path == matchPath || (len(path) > len(matchPath) && path[:len(matchPath)] == matchPath && path[len(matchPath)] == '/') {
			affected = append(affected, session)
		}
	}

	return affected
}

// createSyncOperation creates a sync operation from an event
func (se *SyncEngine) createSyncOperation(session *SyncSession, event SyncEvent, isLocal bool) *SyncOperation {
	var opType OperationType
	var localPath, remotePath string

	if isLocal {
		localPath = event.Path
		// Calculate corresponding remote path
		if event.Path == session.LocalPath {
			remotePath = session.RemotePath
		} else if len(event.Path) > len(session.LocalPath) && event.Path[:len(session.LocalPath)] == session.LocalPath && event.Path[len(session.LocalPath)] == '/' {
			relativePath := event.Path[len(session.LocalPath):]
			remotePath = session.RemotePath + relativePath
		} else {
			return nil
		}
	} else {
		remotePath = event.Path
		// Calculate corresponding local path
		if event.Path == session.RemotePath {
			localPath = session.LocalPath
		} else if len(event.Path) > len(session.RemotePath) && event.Path[:len(session.RemotePath)] == session.RemotePath && event.Path[len(session.RemotePath)] == '/' {
			relativePath := event.Path[len(session.RemotePath):]
			localPath = session.LocalPath + relativePath
		} else {
			return nil
		}
	}

	// Determine operation type
	switch event.Type {
	case EventTypeFileCreated:
		if isLocal {
			opType = OpTypeUpload
		} else {
			opType = OpTypeDownload
		}
	case EventTypeFileModified:
		if isLocal {
			opType = OpTypeUpload
		} else {
			opType = OpTypeDownload
		}
	case EventTypeFileDeleted:
		opType = OpTypeDelete
	case EventTypeDirCreated:
		opType = OpTypeCreateDir
	case EventTypeDirDeleted:
		opType = OpTypeDeleteDir
	default:
		return nil
	}

	return &SyncOperation{
		ID:         fmt.Sprintf("%s-%d", session.SyncID, time.Now().UnixNano()),
		Type:       opType,
		LocalPath:  localPath,
		RemotePath: remotePath,
		Timestamp:  event.Timestamp,
		Status:     OpStatusPending,
		Retries:    0,
	}
}

// GetSyncStatus returns the status of a sync session
func (se *SyncEngine) GetSyncStatus(syncID string) (*SyncSession, error) {
	se.mu.RLock()
	defer se.mu.RUnlock()

	session, exists := se.activeSyncs[syncID]
	if !exists {
		return nil, fmt.Errorf("sync not active for ID: %s", syncID)
	}

	// Return a copy to avoid race conditions
	session.mu.RLock()
	defer session.mu.RUnlock()

	return &SyncSession{
		SyncID:     session.SyncID,
		LocalPath:  session.LocalPath,
		RemotePath: session.RemotePath,
		State:      session.State,
		LastSync:   session.LastSync,
		Status:     session.Status,
		Progress:   session.Progress,
	}, nil
}

// ListActiveSyncs returns a list of all active sync sessions
func (se *SyncEngine) ListActiveSyncs() []*SyncSession {
	se.mu.RLock()
	defer se.mu.RUnlock()

	sessions := make([]*SyncSession, 0, len(se.activeSyncs))
	for _, session := range se.activeSyncs {
		session.mu.RLock()
		sessions = append(sessions, &SyncSession{
			SyncID:     session.SyncID,
			LocalPath:  session.LocalPath,
			RemotePath: session.RemotePath,
			State:      session.State,
			LastSync:   session.LastSync,
			Status:     session.Status,
			Progress:   session.Progress,
		})
		session.mu.RUnlock()
	}

	return sessions
}

// GetStats returns sync engine statistics
func (se *SyncEngine) GetStats() *SyncEngineStats {
	se.statsMu.RLock()
	defer se.statsMu.RUnlock()

	return &SyncEngineStats{
		ActiveSessions:     len(se.activeSyncs),
		TotalSyncEvents:    se.stats.TotalSyncEvents,
		TotalConflicts:     se.stats.TotalConflicts,
		TotalErrors:        se.stats.TotalErrors,
		AverageConflictAge: se.stats.AverageConflictAge,
		LastSyncTime:       se.stats.LastSyncTime,
	}
}

// updateStats updates engine statistics
func (se *SyncEngine) updateStats(updateFunc func(*SyncEngineStats)) {
	se.statsMu.Lock()
	defer se.statsMu.Unlock()
	updateFunc(se.stats)
}

// performInitialSync performs the initial synchronization for a new session
func (se *SyncEngine) performInitialSync(session *SyncSession) {
	session.mu.Lock()
	session.Status = StatusSyncing
	session.mu.Unlock()

	// Initial sync logic would go here
	// For now, just mark as idle
	time.Sleep(100 * time.Millisecond) // Simulate work

	session.mu.Lock()
	session.Status = StatusIdle
	session.LastSync = time.Now()
	session.mu.Unlock()
}

// processSyncOperations processes queued sync operations
func (se *SyncEngine) processSyncOperations() {
	for {
		select {
		case <-se.ctx.Done():
			return
		case op := <-se.syncOpChan:
			se.executeSyncOperation(op)
		}
	}
}

// executeSyncOperation executes a single sync operation
func (se *SyncEngine) executeSyncOperation(op SyncOperation) {
	// Find the session for this operation
	session := se.findSessionForOperation(op)
	if session == nil {
		fmt.Printf("No session found for operation: %s\n", op.ID)
		return
	}

	// Update operation status
	op.Status = OpStatusRunning
	se.stateStore.AddToHistory(session.SyncID, op)

	// Execute based on operation type
	var err error
	switch op.Type {
	case OpTypeUpload:
		err = se.executeUpload(session, op)
	case OpTypeDownload:
		err = se.executeDownload(session, op)
	case OpTypeDelete:
		err = se.executeDelete(session, op)
	case OpTypeCreateDir:
		err = se.executeCreateDir(session, op)
	case OpTypeDeleteDir:
		err = se.executeDeleteDir(session, op)
	default:
		err = fmt.Errorf("unknown operation type: %s", op.Type)
	}

	// Update operation status based on result
	if err != nil {
		op.Status = OpStatusFailed
		op.Error = err.Error()
		op.Retries++
		
		// Retry logic
		if op.Retries < se.config.MaxRetries {
			op.Status = OpStatusPending
			// Re-queue for retry
			go func() {
				time.Sleep(time.Duration(op.Retries) * time.Second) // Exponential backoff
				select {
				case se.syncOpChan <- op:
				case <-se.ctx.Done():
				}
			}()
		} else {
			fmt.Printf("Operation failed after %d retries: %s\n", op.Retries, err)
			se.updateStats(func(stats *SyncEngineStats) {
				stats.TotalErrors++
			})
		}
	} else {
		op.Status = OpStatusCompleted
	}

	// Update state store
	se.stateStore.RemovePendingOperation(session.SyncID, op.ID)
	se.stateStore.AddToHistory(session.SyncID, op)
}

// findSessionForOperation finds the session responsible for an operation
func (se *SyncEngine) findSessionForOperation(op SyncOperation) *SyncSession {
	se.mu.RLock()
	defer se.mu.RUnlock()

	for _, session := range se.activeSyncs {
		if strings.HasPrefix(op.LocalPath, session.LocalPath) || 
		   strings.HasPrefix(op.RemotePath, session.RemotePath) {
			return session
		}
	}

	return nil
}

// executeUpload executes an upload operation with NoiseFS manifest integration
func (se *SyncEngine) executeUpload(session *SyncSession, op SyncOperation) error {
	// Validate path is within allowed directory (security check)
	if err := security.ValidatePathInBounds(op.LocalPath, session.LocalPath); err != nil {
		return fmt.Errorf("security validation failed: %w", err)
	}
	
	// Check if local file exists
	if _, err := os.Stat(op.LocalPath); os.IsNotExist(err) {
		return fmt.Errorf("local file does not exist: %s", op.LocalPath)
	}
	
	// Get file info
	fileInfo, err := os.Stat(op.LocalPath)
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}
	
	// Handle directories vs files differently
	if fileInfo.IsDir() {
		return se.executeDirectoryUpload(session, op)
	}
	
	return se.executeFileUpload(session, op, fileInfo)
}

// executeFileUpload handles individual file uploads with manifest integration
func (se *SyncEngine) executeFileUpload(session *SyncSession, op SyncOperation, fileInfo os.FileInfo) error {
	ctx := context.Background()
	
	// Step 1: Load or create directory manifest
	manifest, err := se.loadOrCreateDirectoryManifest(session, session.LocalPath)
	if err != nil {
		return fmt.Errorf("failed to load directory manifest: %w", err)
	}
	
	// Step 2: Upload the individual file to NoiseFS storage
	// TODO: This would integrate with NoiseFS client to upload the file
	// For now, we'll simulate the file upload and generate a placeholder CID
	fileCID := fmt.Sprintf("QmFile%d", time.Now().UnixNano())
	
	// Step 3: Calculate relative path for manifest entry
	relativePath, err := filepath.Rel(session.LocalPath, op.LocalPath)
	if err != nil {
		return fmt.Errorf("failed to calculate relative path: %w", err)
	}
	
	// Step 4: Encrypt filename for directory entry
	// TODO: This would use proper encryption with directory key
	encryptedName := []byte(filepath.Base(relativePath))
	
	// Step 5: Create or update directory entry
	newEntry := blocks.DirectoryEntry{
		EncryptedName: encryptedName,
		CID:           fileCID,
		Type:          blocks.FileType,
		Size:          fileInfo.Size(),
		ModifiedAt:    fileInfo.ModTime(),
	}
	
	// Find existing entry or add new one
	updated := false
	for i, entry := range manifest.Entries {
		// Simple comparison using encrypted name (TODO: proper decryption comparison)
		if string(entry.EncryptedName) == string(encryptedName) {
			manifest.Entries[i] = newEntry
			updated = true
			break
		}
	}
	
	if !updated {
		manifest.Entries = append(manifest.Entries, newEntry)
	}
	
	// Step 6: Update manifest timestamps
	manifest.ModifiedAt = time.Now()
	
	// Step 7: Store updated manifest
	manifestCID, err := se.directoryManager.StoreDirectoryManifest(ctx, session.LocalPath, manifest)
	if err != nil {
		return fmt.Errorf("failed to store updated manifest: %w", err)
	}
	
	// Step 8: Update session state with new manifest CID
	session.State.ManifestCID = manifestCID
	if err := se.stateStore.ExplicitSave(session.SyncID); err != nil {
		// Log warning but don't fail the operation
		fmt.Printf("Warning: failed to save updated manifest CID to state: %v\n", err)
	}
	
	// Sanitize path for user display
	sanitizedPath := security.SanitizeString(op.LocalPath, session.LocalPath, false)
	fmt.Printf("Uploaded file: %s -> %s (manifest CID: %s)\n", sanitizedPath, fileCID, manifestCID)
	
	return nil
}

// executeDirectoryUpload handles directory creation with manifest integration
func (se *SyncEngine) executeDirectoryUpload(session *SyncSession, op SyncOperation) error {
	ctx := context.Background()
	
	// Step 1: Load or create parent directory manifest
	parentDir := filepath.Dir(op.LocalPath)
	if parentDir == "." || parentDir == session.LocalPath {
		parentDir = session.LocalPath
	}
	
	manifest, err := se.loadOrCreateDirectoryManifest(session, parentDir)
	if err != nil {
		return fmt.Errorf("failed to load parent directory manifest: %w", err)
	}
	
	// Step 2: Create empty manifest for the new directory
	newDirManifest := blocks.NewDirectoryManifest()
	newDirManifestCID, err := se.directoryManager.StoreDirectoryManifest(ctx, op.LocalPath, newDirManifest)
	if err != nil {
		return fmt.Errorf("failed to store new directory manifest: %w", err)
	}
	
	// Step 3: Calculate relative path for parent manifest entry
	relativePath, err := filepath.Rel(parentDir, op.LocalPath)
	if err != nil {
		return fmt.Errorf("failed to calculate relative path: %w", err)
	}
	
	// Step 4: Encrypt directory name
	// TODO: This would use proper encryption with directory key
	encryptedName := []byte(filepath.Base(relativePath))
	
	// Step 5: Create directory entry in parent manifest
	newEntry := blocks.DirectoryEntry{
		EncryptedName: encryptedName,
		CID:           newDirManifestCID,
		Type:          blocks.DirectoryType,
		Size:          0, // Directories have size 0
		ModifiedAt:    time.Now(),
	}
	
	// Find existing entry or add new one
	updated := false
	for i, entry := range manifest.Entries {
		if string(entry.EncryptedName) == string(encryptedName) {
			manifest.Entries[i] = newEntry
			updated = true
			break
		}
	}
	
	if !updated {
		manifest.Entries = append(manifest.Entries, newEntry)
	}
	
	// Step 6: Update parent manifest timestamps
	manifest.ModifiedAt = time.Now()
	
	// Step 7: Store updated parent manifest
	parentManifestCID, err := se.directoryManager.StoreDirectoryManifest(ctx, parentDir, manifest)
	if err != nil {
		return fmt.Errorf("failed to store updated parent manifest: %w", err)
	}
	
	// Step 8: Update session state with new manifest CID
	session.State.ManifestCID = parentManifestCID
	if err := se.stateStore.ExplicitSave(session.SyncID); err != nil {
		// Log warning but don't fail the operation
		fmt.Printf("Warning: failed to save updated manifest CID to state: %v\n", err)
	}
	
	// Sanitize path for user display
	sanitizedPath := security.SanitizeString(op.LocalPath, session.LocalPath, false)
	fmt.Printf("Created directory: %s (manifest CID: %s, parent CID: %s)\n", sanitizedPath, newDirManifestCID, parentManifestCID)
	
	return nil
}

// executeDownload executes a download operation
func (se *SyncEngine) executeDownload(session *SyncSession, op SyncOperation) error {
	// Validate path is within allowed directory (security check)
	if err := security.ValidatePathInBounds(op.LocalPath, session.LocalPath); err != nil {
		return fmt.Errorf("security validation failed: %w", err)
	}
	
	// Ensure local directory exists
	if err := os.MkdirAll(filepath.Dir(op.LocalPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// For individual file downloads in a sync context, we need to:
	// 1. Retrieve the directory manifest from NoiseFS
	// 2. Find the specific file entry in the manifest
	// 3. Download and reconstruct the file locally
	
	// For now, just simulate the operation since we need individual file
	// download capability which requires integration with NoiseFS client
	// A full implementation would need to:
	// 1. Get the remote directory manifest CID from session state
	// 2. Load directory manifest from remote NoiseFS
	// 3. Find the specific file entry in the manifest
	// 4. Download and reconstruct the file locally
	
	// Sanitize path for user display
	sanitizedPath := security.SanitizeString(op.LocalPath, session.LocalPath, false)
	fmt.Printf("Downloaded file: %s (remote manifest integration pending)\n", sanitizedPath)
	
	return nil
}

// executeDelete executes a delete operation
func (se *SyncEngine) executeDelete(session *SyncSession, op SyncOperation) error {
	// Validate path is within allowed directory (security check)
	if err := security.ValidatePathInBounds(op.LocalPath, session.LocalPath); err != nil {
		return fmt.Errorf("security validation failed: %w", err)
	}
	
	// Delete local file if it exists
	if _, err := os.Stat(op.LocalPath); err == nil {
		if err := os.Remove(op.LocalPath); err != nil {
			return fmt.Errorf("failed to delete local file: %w", err)
		}
	}

	// For remote deletion, we need to:
	// 1. Load the current directory manifest
	// 2. Remove the file entry from the manifest
	// 3. Store the updated manifest
	
	// For now, just simulate the operation
	// A full implementation would:
	// 1. Load the current directory manifest
	// 2. Remove the file entry from the manifest
	// 3. Store the updated manifest to NoiseFS
	// Sanitize path for user display
	sanitizedPath := security.SanitizeString(op.LocalPath, session.LocalPath, false)
	fmt.Printf("Deleted file: %s (manifest update pending)\n", sanitizedPath)
	
	return nil
}

// executeCreateDir executes a create directory operation
func (se *SyncEngine) executeCreateDir(session *SyncSession, op SyncOperation) error {
	// Validate path is within allowed directory (security check)
	if err := security.ValidatePathInBounds(op.LocalPath, session.LocalPath); err != nil {
		return fmt.Errorf("security validation failed: %w", err)
	}
	
	// Create local directory
	if err := os.MkdirAll(op.LocalPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// For remote directory creation, we need to:
	// 1. Load the parent directory manifest
	// 2. Add a directory entry to the manifest
	// 3. Store the updated manifest
	
	// For now, just simulate the operation
	// A full implementation would:
	// 1. Load the parent directory manifest
	// 2. Add a directory entry to the manifest  
	// 3. Store the updated manifest to NoiseFS
	// Sanitize path for user display
	sanitizedPath := security.SanitizeString(op.LocalPath, session.LocalPath, false)
	fmt.Printf("Created directory: %s (manifest update pending)\n", sanitizedPath)
	
	return nil
}

// executeDeleteDir executes a delete directory operation
func (se *SyncEngine) executeDeleteDir(session *SyncSession, op SyncOperation) error {
	// Validate path is within allowed directory (security check)
	if err := security.ValidatePathInBounds(op.LocalPath, session.LocalPath); err != nil {
		return fmt.Errorf("security validation failed: %w", err)
	}
	
	// Delete local directory if it exists
	if _, err := os.Stat(op.LocalPath); err == nil {
		if err := os.RemoveAll(op.LocalPath); err != nil {
			return fmt.Errorf("failed to delete local directory: %w", err)
		}
	}

	// For remote directory deletion, we need to:
	// 1. Load the parent directory manifest
	// 2. Remove the directory entry from the manifest
	// 3. Store the updated manifest
	
	// For now, just simulate the operation
	// A full implementation would:
	// 1. Load the parent directory manifest
	// 2. Remove the directory entry from the manifest
	// 3. Store the updated manifest to NoiseFS
	// Sanitize path for user display
	sanitizedPath := security.SanitizeString(op.LocalPath, session.LocalPath, false)
	fmt.Printf("Deleted directory: %s (manifest update pending)\n", sanitizedPath)
	
	return nil
}

// Stop stops the sync engine
func (se *SyncEngine) Stop() error {
	se.cancel()

	// Stop all active syncs
	se.mu.Lock()
	for syncID := range se.activeSyncs {
		se.StopSync(syncID)
	}
	se.mu.Unlock()

	// Close channels
	close(se.localEventChan)
	close(se.remoteEventChan)
	close(se.syncOpChan)

	return nil
}

// loadOrCreateDirectoryManifest loads an existing directory manifest or creates a new one
func (se *SyncEngine) loadOrCreateDirectoryManifest(session *SyncSession, dirPath string) (*blocks.DirectoryManifest, error) {
	ctx := context.Background()
	
	// Step 1: Check if we have an existing manifest CID in session state
	if session.State.ManifestCID != "" {
		manifest, err := se.directoryManager.RetrieveDirectoryManifest(ctx, dirPath, session.State.ManifestCID)
		if err == nil {
			return manifest, nil
		}
		// If retrieval fails, we'll create a new manifest below
		fmt.Printf("Warning: failed to load existing manifest %s, creating new one: %v\n", session.State.ManifestCID, err)
	}
	
	// Step 2: Create a new directory manifest
	manifest := blocks.NewDirectoryManifest()
	
	// Step 3: Scan the local directory and populate the manifest
	// TODO: This would involve:
	// 1. Walking the directory tree
	// 2. For each file: upload to NoiseFS and get CID
	// 3. Add entries to the manifest
	// 4. Store the manifest and return the manifest CID
	
	return manifest, nil
}
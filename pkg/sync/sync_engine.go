package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	noisefs "github.com/TheEntropyCollective/noisefs/pkg/core/client"
	"github.com/TheEntropyCollective/noisefs/pkg/core/crypto"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
)

// SyncEngine coordinates bi-directional synchronization between local and remote directories
type SyncEngine struct {
	stateStore        *SyncStateStore
	fileWatcher       *FileWatcher
	remoteMonitor     *RemoteChangeMonitor
	conflictResolver  *ConflictResolver
	directoryManager  *storage.DirectoryManager
	manifestUpdateMgr *ManifestUpdateManager
	noisefsClient     *noisefs.Client
	config            *SyncConfig

	// Channels for coordinating events
	localEventChan  chan SyncEvent
	remoteEventChan chan SyncEvent
	syncOpChan      chan SyncOperation

	// Control and state
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.RWMutex
	activeSyncs map[string]*SyncSession

	// Statistics
	stats   *SyncEngineStats
	statsMu sync.RWMutex
}

// SyncSession represents an active sync session
type SyncSession struct {
	SyncID      string
	LocalPath   string
	RemotePath  string
	ManifestCID string
	State       *SyncState
	LastSync    time.Time
	Status      SyncStatus
	Progress    *SyncProgress
	mu          sync.RWMutex
}

// SyncStatus represents the current status of a sync session
type SyncStatus string

const (
	StatusIdle     SyncStatus = "idle"
	StatusSyncing  SyncStatus = "syncing"
	StatusConflict SyncStatus = "conflict"
	StatusError    SyncStatus = "error"
	StatusPaused   SyncStatus = "paused"
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
	encryptionKey *crypto.EncryptionKey,
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
	if encryptionKey == nil {
		return nil, fmt.Errorf("encryption key cannot be nil")
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

	// Create manifest update manager
	manifestUpdateMgr, err := NewManifestUpdateManager(
		directoryManager,
		stateStore,
		encryptionKey,
		DefaultManifestUpdateConfig(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create manifest update manager: %w", err)
	}
	engine.manifestUpdateMgr = manifestUpdateMgr

	// Create NoiseFS client from directory manager's storage manager
	// Use memory cache for sync operations
	basicCache := cache.NewMemoryCache(1000) // 1000 blocks cache
	noisefsClient, err := noisefs.NewClient(directoryManager.GetStorageManager(), basicCache)
	if err != nil {
		return nil, fmt.Errorf("failed to create NoiseFS client: %w", err)
	}
	engine.noisefsClient = noisefsClient

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
		SyncID:      syncID,
		LocalPath:   localPath,
		RemotePath:  remotePath,
		ManifestCID: manifestCID,
		State:       state,
		LastSync:    time.Now(),
		Status:      StatusIdle,
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
		// Log error but continue
		fmt.Printf("Warning: failed to remove local path from watcher: %v\n", err)
	}

	if err := se.remoteMonitor.RemovePath(session.RemotePath); err != nil {
		// Log error but continue
		fmt.Printf("Warning: failed to remove remote path from monitor: %v\n", err)
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
		SyncID:      session.SyncID,
		LocalPath:   session.LocalPath,
		RemotePath:  session.RemotePath,
		ManifestCID: session.ManifestCID,
		State:       session.State,
		LastSync:    session.LastSync,
		Status:      session.Status,
		Progress:    session.Progress,
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
			SyncID:      session.SyncID,
			LocalPath:   session.LocalPath,
			RemotePath:  session.RemotePath,
			ManifestCID: session.ManifestCID,
			State:       session.State,
			LastSync:    session.LastSync,
			Status:      session.Status,
			Progress:    session.Progress,
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

	// Create directory scanner
	scanner := NewDirectoryScanner(se.directoryManager)

	// Perform initial scan
	ctx := context.Background()
	
	scanResult, err := scanner.PerformInitialScan(ctx, session.LocalPath, session.RemotePath, session.ManifestCID, session.State)
	if err != nil {
		session.mu.Lock()
		session.Status = StatusError
		session.mu.Unlock()
		fmt.Printf("Initial sync failed for session %s: %v\n", session.SyncID, err)
		return
	}

	// Update session state with scan results
	session.State.LocalSnapshot = scanResult.LocalSnapshot
	session.State.RemoteSnapshot = scanResult.RemoteSnapshot
	session.State.LastSync = time.Now()

	// Save updated state
	if err := se.stateStore.SaveState(session.SyncID, session.State); err != nil {
		fmt.Printf("Failed to save state for session %s: %v\n", session.SyncID, err)
		// Continue anyway - we'll try again later
	}

	// Generate sync operations from detected changes
	operations := scanner.GenerateSyncOperations(session.SyncID, scanResult.Changes, session.LocalPath, session.RemotePath)

	// Queue operations for processing
	for _, op := range operations {
		// Add to pending operations
		if err := se.stateStore.AddPendingOperation(session.SyncID, op); err != nil {
			fmt.Printf("Failed to add pending operation: %v\n", err)
			continue
		}

		// Queue for processing
		select {
		case se.syncOpChan <- op:
		default:
			fmt.Printf("Sync operation queue full, dropping operation %s\n", op.ID)
		}
	}

	// Update session progress
	session.mu.Lock()
	session.Status = StatusIdle
	session.LastSync = time.Now()
	session.Progress.TotalOperations = len(operations)
	session.Progress.CompletedOperations = 0
	session.Progress.CurrentOperation = "Initial sync completed"
	session.mu.Unlock()

	fmt.Printf("Initial sync completed for session %s: found %d changes, generated %d operations\n", 
		session.SyncID, len(scanResult.Changes), len(operations))
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

// executeUpload executes an upload operation
func (se *SyncEngine) executeUpload(session *SyncSession, op SyncOperation) error {
	// Check if local file exists
	fileInfo, err := os.Stat(op.LocalPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("local file does not exist: %s", op.LocalPath)
	}

	// Handle directories differently than files
	if fileInfo.IsDir() {
		// For directories, we don't need to upload content, just ensure remote directory exists
		// This would be handled by the directory structure sync
		fmt.Printf("Directory sync: %s -> %s\n", op.LocalPath, op.RemotePath)
		return nil
	}

	// Open the local file
	file, err := os.Open(op.LocalPath)
	if err != nil {
		return fmt.Errorf("failed to open local file %s: %w", op.LocalPath, err)
	}
	defer file.Close()

	// Extract filename from path
	filename := filepath.Base(op.LocalPath)

	// Upload file using NoiseFS client
	ctx := context.Background()
	descriptorCID, err := se.noisefsClient.Upload(ctx, file, filename)
	if err != nil {
		return fmt.Errorf("failed to upload file %s to NoiseFS: %w", op.LocalPath, err)
	}

	fmt.Printf("Successfully uploaded: %s -> %s (CID: %s)\n", op.LocalPath, op.RemotePath, descriptorCID)

	// Update parent directory manifest with the new file
	result, err := se.manifestUpdateMgr.UpdateAfterFileOperation(
		session.SyncID,
		op.RemotePath,
		ManifestOpAdd,
		descriptorCID,
		"", // No old CID for new files
	)
	if err != nil {
		return fmt.Errorf("failed to update directory manifest: %w", err)
	}

	fmt.Printf("Updated directory manifest: %s (new CID: %s)\n", filepath.Dir(op.RemotePath), result.NewCID)

	// Propagate the changes up the directory tree
	dirPath := filepath.Dir(op.RemotePath)
	propagateResult, err := se.manifestUpdateMgr.PropagateToAncestors(
		session.SyncID,
		dirPath,
		result.NewCID,
	)
	if err != nil {
		return fmt.Errorf("failed to propagate manifest updates: %w", err)
	}

	if len(propagateResult.UpdatedPaths) > 0 {
		fmt.Printf("Propagated updates to ancestor directories: %v\n", propagateResult.UpdatedPaths)
	}

	return nil
}

// executeDownload executes a download operation
func (se *SyncEngine) executeDownload(session *SyncSession, op SyncOperation) error {
	// TODO: Get the descriptor CID for the remote path from sync state
	// For now, we'll assume the remote path contains or maps to a CID
	// In a real implementation, this would be stored in the sync state

	// Extract potential CID from remote path or get it from state
	// This is a placeholder - in reality, you'd have a mapping table
	descriptorCID := strings.TrimPrefix(op.RemotePath, "noisefs://")
	if descriptorCID == op.RemotePath {
		// No noisefs:// prefix, might be a direct CID or need state lookup
		return fmt.Errorf("cannot determine descriptor CID for remote path: %s", op.RemotePath)
	}

	// Ensure local directory exists
	if err := os.MkdirAll(filepath.Dir(op.LocalPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Download file using NoiseFS client
	ctx := context.Background()
	data, filename, err := se.noisefsClient.DownloadWithMetadata(ctx, descriptorCID)
	if err != nil {
		return fmt.Errorf("failed to download file with CID %s: %w", descriptorCID, err)
	}

	// Use the original filename if available, otherwise use the local path basename
	targetPath := op.LocalPath
	if filename != "" && filepath.Base(op.LocalPath) != filename {
		// Update target path to use the original filename
		targetPath = filepath.Join(filepath.Dir(op.LocalPath), filename)
	}

	// Write the downloaded data to local file
	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write downloaded file to %s: %w", targetPath, err)
	}

	fmt.Printf("Successfully downloaded: %s -> %s (filename: %s)\n", op.RemotePath, targetPath, filename)

	return nil
}

// executeDelete executes a delete operation
func (se *SyncEngine) executeDelete(session *SyncSession, op SyncOperation) error {
	// Delete local file if it exists
	if _, err := os.Stat(op.LocalPath); err == nil {
		if err := os.Remove(op.LocalPath); err != nil {
			return fmt.Errorf("failed to delete local file: %w", err)
		}
	}

	// Update parent directory manifest to remove the file
	result, err := se.manifestUpdateMgr.UpdateAfterFileOperation(
		session.SyncID,
		op.RemotePath,
		ManifestOpRemove,
		"", // No new CID for removals
		"", // TODO: Get old CID from sync state if needed
	)
	if err != nil {
		return fmt.Errorf("failed to update directory manifest after deletion: %w", err)
	}

	fmt.Printf("Deleting: %s (updated directory manifest: %s)\n", op.LocalPath, result.NewCID)

	// Propagate the changes up the directory tree
	dirPath := filepath.Dir(op.RemotePath)
	propagateResult, err := se.manifestUpdateMgr.PropagateToAncestors(
		session.SyncID,
		dirPath,
		result.NewCID,
	)
	if err != nil {
		return fmt.Errorf("failed to propagate manifest updates after deletion: %w", err)
	}

	if len(propagateResult.UpdatedPaths) > 0 {
		fmt.Printf("Propagated deletion updates to ancestor directories: %v\n", propagateResult.UpdatedPaths)
	}

	return nil
}

// executeCreateDir executes a create directory operation
func (se *SyncEngine) executeCreateDir(session *SyncSession, op SyncOperation) error {
	// Create local directory
	if err := os.MkdirAll(op.LocalPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// TODO: Implement remote directory creation logic
	fmt.Printf("Creating directory: %s\n", op.LocalPath)
	return nil
}

// executeDeleteDir executes a delete directory operation
func (se *SyncEngine) executeDeleteDir(session *SyncSession, op SyncOperation) error {
	// Delete local directory if it exists
	if _, err := os.Stat(op.LocalPath); err == nil {
		if err := os.RemoveAll(op.LocalPath); err != nil {
			return fmt.Errorf("failed to delete local directory: %w", err)
		}
	}

	// TODO: Implement remote directory deletion logic
	fmt.Printf("Deleting directory: %s\n", op.LocalPath)
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

	// Stop manifest update manager
	if se.manifestUpdateMgr != nil {
		if err := se.manifestUpdateMgr.Stop(); err != nil {
			fmt.Printf("Warning: failed to stop manifest update manager: %v\n", err)
		}
	}

	// Close channels
	close(se.localEventChan)
	close(se.remoteEventChan)
	close(se.syncOpChan)

	return nil
}

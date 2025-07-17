package sync

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

// RemoteChangeMonitor monitors NoiseFS for remote directory changes
type RemoteChangeMonitor struct {
	directoryManager *storage.DirectoryManager
	stateStore       *SyncStateStore
	config           *SyncConfig
	eventChan        chan SyncEvent
	errorChan        chan error
	ctx              context.Context
	cancel           context.CancelFunc
	mu               sync.RWMutex
	monitoredPaths   map[string]*RemoteMonitorState
	pollInterval     time.Duration
}

// RemoteMonitorState tracks the state of a monitored remote path
type RemoteMonitorState struct {
	RemotePath    string
	ManifestCID   string
	LastSnapshot  map[string]RemoteMetadata
	LastChecked   time.Time
	LastModified  time.Time
	SyncID        string
}

// NewRemoteChangeMonitor creates a new remote change monitor
func NewRemoteChangeMonitor(directoryManager *storage.DirectoryManager, stateStore *SyncStateStore, config *SyncConfig) (*RemoteChangeMonitor, error) {
	if directoryManager == nil {
		return nil, fmt.Errorf("directory manager cannot be nil")
	}
	if stateStore == nil {
		return nil, fmt.Errorf("state store cannot be nil")
	}
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	ctx, cancel := context.WithCancel(context.Background())

	pollInterval := config.SyncInterval
	if pollInterval == 0 {
		pollInterval = 30 * time.Second // Default poll interval
	}

	monitor := &RemoteChangeMonitor{
		directoryManager: directoryManager,
		stateStore:       stateStore,
		config:           config,
		eventChan:        make(chan SyncEvent, 100),
		errorChan:        make(chan error, 10),
		ctx:              ctx,
		cancel:           cancel,
		monitoredPaths:   make(map[string]*RemoteMonitorState),
		pollInterval:     pollInterval,
	}

	go monitor.monitorLoop()

	return monitor, nil
}

// AddPath adds a remote path to monitor for changes
func (rm *RemoteChangeMonitor) AddPath(syncID, remotePath, manifestCID string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.monitoredPaths[remotePath] != nil {
		return nil // Already monitoring this path
	}

	// Create initial snapshot
	snapshot, err := rm.createSnapshot(remotePath, manifestCID)
	if err != nil {
		return fmt.Errorf("failed to create initial snapshot: %w", err)
	}

	rm.monitoredPaths[remotePath] = &RemoteMonitorState{
		RemotePath:   remotePath,
		ManifestCID:  manifestCID,
		LastSnapshot: snapshot,
		LastChecked:  time.Now(),
		LastModified: time.Now(),
		SyncID:       syncID,
	}

	return nil
}

// RemovePath removes a remote path from monitoring
func (rm *RemoteChangeMonitor) RemovePath(remotePath string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	delete(rm.monitoredPaths, remotePath)
	return nil
}

// UpdateManifestCID updates the manifest CID for a monitored path
func (rm *RemoteChangeMonitor) UpdateManifestCID(remotePath, newManifestCID string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	state, exists := rm.monitoredPaths[remotePath]
	if !exists {
		return fmt.Errorf("path not being monitored: %s", remotePath)
	}

	state.ManifestCID = newManifestCID
	state.LastModified = time.Now()

	return nil
}

// Events returns a channel that receives sync events
func (rm *RemoteChangeMonitor) Events() <-chan SyncEvent {
	return rm.eventChan
}

// Errors returns a channel that receives errors
func (rm *RemoteChangeMonitor) Errors() <-chan error {
	return rm.errorChan
}

// Stop stops the remote change monitor
func (rm *RemoteChangeMonitor) Stop() error {
	rm.cancel()
	close(rm.eventChan)
	close(rm.errorChan)
	return nil
}

// monitorLoop is the main monitoring loop
func (rm *RemoteChangeMonitor) monitorLoop() {
	ticker := time.NewTicker(rm.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-rm.ctx.Done():
			return
		case <-ticker.C:
			rm.checkForChanges()
		}
	}
}

// checkForChanges checks all monitored paths for changes
func (rm *RemoteChangeMonitor) checkForChanges() {
	rm.mu.RLock()
	paths := make(map[string]*RemoteMonitorState)
	for k, v := range rm.monitoredPaths {
		paths[k] = v
	}
	rm.mu.RUnlock()

	for remotePath, state := range paths {
		if err := rm.checkPathForChanges(remotePath, state); err != nil {
			select {
			case rm.errorChan <- fmt.Errorf("failed to check path %s: %w", remotePath, err):
			default:
			}
		}
	}
}

// checkPathForChanges checks a specific path for changes
func (rm *RemoteChangeMonitor) checkPathForChanges(remotePath string, state *RemoteMonitorState) error {
	// Create new snapshot
	newSnapshot, err := rm.createSnapshot(remotePath, state.ManifestCID)
	if err != nil {
		return fmt.Errorf("failed to create snapshot: %w", err)
	}

	// Compare with previous snapshot
	changes := rm.compareSnapshots(state.LastSnapshot, newSnapshot)

	// Update state
	rm.mu.Lock()
	if currentState, exists := rm.monitoredPaths[remotePath]; exists {
		currentState.LastSnapshot = newSnapshot
		currentState.LastChecked = time.Now()
		if len(changes) > 0 {
			currentState.LastModified = time.Now()
		}
	}
	rm.mu.Unlock()

	// Emit events for changes
	for _, change := range changes {
		select {
		case rm.eventChan <- change:
		case <-rm.ctx.Done():
			return nil
		default:
			// Channel is full, log error but don't block
			select {
			case rm.errorChan <- fmt.Errorf("event channel full, dropping remote change for %s", remotePath):
			default:
			}
		}
	}

	return nil
}

// createSnapshot creates a snapshot of the remote directory state
func (rm *RemoteChangeMonitor) createSnapshot(remotePath, manifestCID string) (map[string]RemoteMetadata, error) {
	snapshot := make(map[string]RemoteMetadata)

	// Retrieve the directory manifest
	manifest, err := rm.directoryManager.RetrieveDirectoryManifest(rm.ctx, remotePath, manifestCID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve directory manifest: %w", err)
	}

	// Process each entry in the manifest
	for _, entry := range manifest.Entries {
		// For now, we'll use the encrypted name as the key
		// In a real implementation, you might want to decrypt it
		entryPath := fmt.Sprintf("%s/%s", remotePath, string(entry.EncryptedName))
		
		metadata := RemoteMetadata{
			Path:          entryPath,
			DescriptorCID: entry.CID,
			Size:          entry.Size,
			ModTime:       entry.ModifiedAt,
			IsDir:         entry.Type == blocks.DirectoryType,
		}

		snapshot[entryPath] = metadata
	}

	return snapshot, nil
}

// compareSnapshots compares two snapshots and returns change events
func (rm *RemoteChangeMonitor) compareSnapshots(oldSnapshot, newSnapshot map[string]RemoteMetadata) []SyncEvent {
	var changes []SyncEvent
	now := time.Now()

	// Check for new or modified entries
	for path, newMeta := range newSnapshot {
		if oldMeta, exists := oldSnapshot[path]; exists {
			// Entry existed before, check if modified
			if rm.hasMetadataChanged(oldMeta, newMeta) {
				eventType := EventTypeFileModified
				if newMeta.IsDir {
					eventType = EventTypeDirCreated // Directory modifications are treated as recreations
				}

				changes = append(changes, SyncEvent{
					Type:      eventType,
					Path:      path,
					Timestamp: now,
					Metadata: map[string]interface{}{
						"old_size":    oldMeta.Size,
						"new_size":    newMeta.Size,
						"old_cid":     oldMeta.DescriptorCID,
						"new_cid":     newMeta.DescriptorCID,
						"old_modtime": oldMeta.ModTime,
						"new_modtime": newMeta.ModTime,
						"is_dir":      newMeta.IsDir,
					},
				})
			}
		} else {
			// New entry
			eventType := EventTypeFileCreated
			if newMeta.IsDir {
				eventType = EventTypeDirCreated
			}

			changes = append(changes, SyncEvent{
				Type:      eventType,
				Path:      path,
				Timestamp: now,
				Metadata: map[string]interface{}{
					"size":         newMeta.Size,
					"cid":          newMeta.DescriptorCID,
					"modtime":      newMeta.ModTime,
					"is_dir":       newMeta.IsDir,
					"change_type":  "created",
				},
			})
		}
	}

	// Check for deleted entries
	for path, oldMeta := range oldSnapshot {
		if _, exists := newSnapshot[path]; !exists {
			eventType := EventTypeFileDeleted
			if oldMeta.IsDir {
				eventType = EventTypeDirDeleted
			}

			changes = append(changes, SyncEvent{
				Type:      eventType,
				Path:      path,
				Timestamp: now,
				Metadata: map[string]interface{}{
					"old_size":    oldMeta.Size,
					"old_cid":     oldMeta.DescriptorCID,
					"old_modtime": oldMeta.ModTime,
					"is_dir":      oldMeta.IsDir,
					"change_type": "deleted",
				},
			})
		}
	}

	return changes
}

// hasMetadataChanged checks if remote metadata has changed
func (rm *RemoteChangeMonitor) hasMetadataChanged(old, new RemoteMetadata) bool {
	return old.Size != new.Size ||
		old.DescriptorCID != new.DescriptorCID ||
		!old.ModTime.Equal(new.ModTime) ||
		old.IsDir != new.IsDir
}

// GetMonitoredPaths returns a list of currently monitored paths
func (rm *RemoteChangeMonitor) GetMonitoredPaths() []string {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	paths := make([]string, 0, len(rm.monitoredPaths))
	for path := range rm.monitoredPaths {
		paths = append(paths, path)
	}

	return paths
}

// GetMonitorState returns the current state of a monitored path
func (rm *RemoteChangeMonitor) GetMonitorState(remotePath string) (*RemoteMonitorState, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	state, exists := rm.monitoredPaths[remotePath]
	if !exists {
		return nil, fmt.Errorf("path not being monitored: %s", remotePath)
	}

	// Return a copy to avoid race conditions
	return &RemoteMonitorState{
		RemotePath:    state.RemotePath,
		ManifestCID:   state.ManifestCID,
		LastSnapshot:  state.LastSnapshot,
		LastChecked:   state.LastChecked,
		LastModified:  state.LastModified,
		SyncID:        state.SyncID,
	}, nil
}

// ForceCheck forces an immediate check of all monitored paths
func (rm *RemoteChangeMonitor) ForceCheck() {
	go rm.checkForChanges()
}

// SetPollInterval updates the polling interval
func (rm *RemoteChangeMonitor) SetPollInterval(interval time.Duration) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.pollInterval = interval
	// Note: This won't affect the current ticker, but will be used
	// for the next restart or when creating a new monitor
}

// GetStats returns monitoring statistics
func (rm *RemoteChangeMonitor) GetStats() *RemoteMonitorStats {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	var totalPaths, activePaths int
	var lastCheck time.Time

	for _, state := range rm.monitoredPaths {
		totalPaths++
		if time.Since(state.LastChecked) < 2*rm.pollInterval {
			activePaths++
		}
		if state.LastChecked.After(lastCheck) {
			lastCheck = state.LastChecked
		}
	}

	return &RemoteMonitorStats{
		TotalPaths:    totalPaths,
		ActivePaths:   activePaths,
		PollInterval:  rm.pollInterval,
		LastCheck:     lastCheck,
		EventsQueued:  len(rm.eventChan),
		ErrorsQueued:  len(rm.errorChan),
	}
}

// RemoteMonitorStats represents remote monitoring statistics
type RemoteMonitorStats struct {
	TotalPaths   int           `json:"total_paths"`
	ActivePaths  int           `json:"active_paths"`
	PollInterval time.Duration `json:"poll_interval"`
	LastCheck    time.Time     `json:"last_check"`
	EventsQueued int           `json:"events_queued"`
	ErrorsQueued int           `json:"errors_queued"`
}
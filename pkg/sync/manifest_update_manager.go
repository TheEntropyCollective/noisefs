package sync

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/core/crypto"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

// ManifestUpdateManager handles directory manifest updates after file operations
type ManifestUpdateManager struct {
	directoryManager *storage.DirectoryManager
	stateStore       *SyncStateStore
	encryptionKey    *crypto.EncryptionKey
	config           *ManifestUpdateConfig

	// Concurrent update protection
	dirLocks map[string]*sync.Mutex
	lockMu   sync.RWMutex

	// Update queue for batching
	updateQueue chan *ManifestUpdateRequest
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup

	// Statistics
	stats   *ManifestUpdateStats
	statsMu sync.RWMutex
}

// ManifestUpdateConfig holds configuration for manifest updates
type ManifestUpdateConfig struct {
	BatchSize       int           // Maximum number of updates to batch together
	BatchTimeout    time.Duration // Maximum time to wait before processing a batch
	WorkerCount     int           // Number of worker goroutines
	RetryMaxCount   int           // Maximum number of retries for failed updates
	RetryBackoff    time.Duration // Base backoff duration for retries
	ConcurrentLimit int           // Maximum concurrent manifest updates
}

// ManifestUpdateRequest represents a request to update a directory manifest
type ManifestUpdateRequest struct {
	SyncID        string
	DirectoryPath string
	Operation     ManifestOperation
	FileEntry     *blocks.DirectoryEntry
	OldCID        string // For update operations
	NewCID        string // For add/update operations
	Timestamp     time.Time
	RetryCount    int
	resultChan    chan *ManifestUpdateResult
}

// ManifestOperation represents the type of manifest operation
type ManifestOperation string

const (
	ManifestOpAdd    ManifestOperation = "add"
	ManifestOpUpdate ManifestOperation = "update"
	ManifestOpRemove ManifestOperation = "remove"
)

// ManifestUpdateResult represents the result of a manifest update
type ManifestUpdateResult struct {
	Success      bool
	NewCID       string
	Error        error
	UpdatedPaths []string // Paths of all updated manifests (including ancestors)
}

// ManifestUpdateStats tracks statistics for manifest updates
type ManifestUpdateStats struct {
	TotalRequests     int64         `json:"total_requests"`
	SuccessfulUpdates int64         `json:"successful_updates"`
	FailedUpdates     int64         `json:"failed_updates"`
	RetryCount        int64         `json:"retry_count"`
	AverageUpdateTime time.Duration `json:"average_update_time"`
	LastUpdateTime    time.Time     `json:"last_update_time"`
}

// DefaultManifestUpdateConfig returns default configuration
func DefaultManifestUpdateConfig() *ManifestUpdateConfig {
	return &ManifestUpdateConfig{
		BatchSize:       10,
		BatchTimeout:    500 * time.Millisecond,
		WorkerCount:     3,
		RetryMaxCount:   3,
		RetryBackoff:    1 * time.Second,
		ConcurrentLimit: 5,
	}
}

// NewManifestUpdateManager creates a new manifest update manager
func NewManifestUpdateManager(
	directoryManager *storage.DirectoryManager,
	stateStore *SyncStateStore,
	encryptionKey *crypto.EncryptionKey,
	config *ManifestUpdateConfig,
) (*ManifestUpdateManager, error) {
	if directoryManager == nil {
		return nil, fmt.Errorf("directory manager cannot be nil")
	}
	if stateStore == nil {
		return nil, fmt.Errorf("state store cannot be nil")
	}
	if encryptionKey == nil {
		return nil, fmt.Errorf("encryption key cannot be nil")
	}
	if config == nil {
		config = DefaultManifestUpdateConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	manager := &ManifestUpdateManager{
		directoryManager: directoryManager,
		stateStore:       stateStore,
		encryptionKey:    encryptionKey,
		config:           config,
		dirLocks:         make(map[string]*sync.Mutex),
		updateQueue:      make(chan *ManifestUpdateRequest, config.BatchSize*2),
		ctx:              ctx,
		cancel:           cancel,
		stats:            &ManifestUpdateStats{},
	}

	// Start worker goroutines
	for i := 0; i < config.WorkerCount; i++ {
		manager.wg.Add(1)
		go manager.worker()
	}

	return manager, nil
}

// UpdateAfterFileOperation updates directory manifests after a file operation
func (m *ManifestUpdateManager) UpdateAfterFileOperation(
	syncID string,
	filePath string,
	operation ManifestOperation,
	newCID string,
	oldCID string,
) (*ManifestUpdateResult, error) {
	// Get the parent directory path
	dirPath := filepath.Dir(filePath)
	if dirPath == "." {
		dirPath = "/"
	}

	// Create directory entry for the file
	var fileEntry *blocks.DirectoryEntry
	if operation != ManifestOpRemove {
		filename := filepath.Base(filePath)

		// Encrypt the filename
		dirKey, err := crypto.DeriveDirectoryKey(m.encryptionKey, dirPath)
		if err != nil {
			return nil, fmt.Errorf("failed to derive directory key: %w", err)
		}

		encryptedName, err := crypto.EncryptFileName(filename, dirKey)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt filename: %w", err)
		}

		fileEntry = &blocks.DirectoryEntry{
			EncryptedName: encryptedName,
			CID:           newCID,
			Type:          blocks.FileType,
			Size:          0, // TODO: Get actual file size
			ModifiedAt:    time.Now(),
		}
	}

	// Create update request
	request := &ManifestUpdateRequest{
		SyncID:        syncID,
		DirectoryPath: dirPath,
		Operation:     operation,
		FileEntry:     fileEntry,
		OldCID:        oldCID,
		NewCID:        newCID,
		Timestamp:     time.Now(),
		resultChan:    make(chan *ManifestUpdateResult, 1),
	}

	// Queue the request
	select {
	case m.updateQueue <- request:
		// Wait for result
		select {
		case result := <-request.resultChan:
			return result, nil
		case <-m.ctx.Done():
			return nil, fmt.Errorf("manifest update manager stopped")
		}
	case <-m.ctx.Done():
		return nil, fmt.Errorf("manifest update manager stopped")
	default:
		return nil, fmt.Errorf("update queue full")
	}
}

// worker processes manifest update requests
func (m *ManifestUpdateManager) worker() {
	defer m.wg.Done()

	for {
		select {
		case <-m.ctx.Done():
			return
		case request := <-m.updateQueue:
			m.processUpdateRequest(request)
		}
	}
}

// processUpdateRequest processes a single manifest update request
func (m *ManifestUpdateManager) processUpdateRequest(request *ManifestUpdateRequest) {
	startTime := time.Now()

	m.updateStats(func(stats *ManifestUpdateStats) {
		stats.TotalRequests++
	})

	// Acquire directory lock
	dirLock := m.getDirLock(request.DirectoryPath)
	dirLock.Lock()
	defer dirLock.Unlock()

	result := m.executeManifestUpdate(request)

	// Update statistics
	updateDuration := time.Since(startTime)
	m.updateStats(func(stats *ManifestUpdateStats) {
		if result.Success {
			stats.SuccessfulUpdates++
		} else {
			stats.FailedUpdates++
		}
		stats.LastUpdateTime = time.Now()

		// Update average (simple moving average)
		if stats.AverageUpdateTime == 0 {
			stats.AverageUpdateTime = updateDuration
		} else {
			stats.AverageUpdateTime = (stats.AverageUpdateTime + updateDuration) / 2
		}
	})

	// Send result back
	select {
	case request.resultChan <- result:
	default:
		// Channel might be closed, ignore
	}
}

// executeManifestUpdate executes the actual manifest update with retry logic
func (m *ManifestUpdateManager) executeManifestUpdate(request *ManifestUpdateRequest) *ManifestUpdateResult {
	var result *ManifestUpdateResult
	var err error

	for attempt := 0; attempt <= m.config.RetryMaxCount; attempt++ {
		if attempt > 0 {
			// Wait before retry
			backoffDuration := time.Duration(attempt) * m.config.RetryBackoff
			time.Sleep(backoffDuration)

			m.updateStats(func(stats *ManifestUpdateStats) {
				stats.RetryCount++
			})
		}

		result = m.doManifestUpdate(request)
		if result.Success {
			return result
		}

		// Log retry attempt
		fmt.Printf("Manifest update failed (attempt %d/%d): %v\n",
			attempt+1, m.config.RetryMaxCount+1, result.Error)

		err = result.Error
		if attempt == m.config.RetryMaxCount {
			break
		}
	}

	// All retries failed
	return &ManifestUpdateResult{
		Success: false,
		Error:   fmt.Errorf("manifest update failed after %d attempts: %w", m.config.RetryMaxCount+1, err),
	}
}

// doManifestUpdate performs a single manifest update attempt
func (m *ManifestUpdateManager) doManifestUpdate(request *ManifestUpdateRequest) *ManifestUpdateResult {
	ctx := context.Background()

	// Get current manifest CID from sync state
	syncState, err := m.stateStore.LoadState(request.SyncID)
	if err != nil {
		return &ManifestUpdateResult{
			Success: false,
			Error:   fmt.Errorf("failed to load sync state: %w", err),
		}
	}

	// Get the current manifest CID for this directory from RemoteSnapshot
	var currentManifestCID string
	if remoteInfo, exists := syncState.RemoteSnapshot[request.DirectoryPath]; exists {
		currentManifestCID = remoteInfo.DescriptorCID
	}

	// Load or create the directory manifest
	var manifest *blocks.DirectoryManifest
	if currentManifestCID != "" {
		manifest, err = m.directoryManager.RetrieveDirectoryManifest(ctx, request.DirectoryPath, currentManifestCID)
		if err != nil {
			return &ManifestUpdateResult{
				Success: false,
				Error:   fmt.Errorf("failed to retrieve manifest: %w", err),
			}
		}
	} else {
		manifest = blocks.NewDirectoryManifest()
	}

	// Apply the operation to the manifest
	err = m.applyManifestOperation(manifest, request)
	if err != nil {
		return &ManifestUpdateResult{
			Success: false,
			Error:   fmt.Errorf("failed to apply operation: %w", err),
		}
	}

	// Store the updated manifest
	newManifestCID, err := m.directoryManager.StoreDirectoryManifest(ctx, request.DirectoryPath, manifest)
	if err != nil {
		return &ManifestUpdateResult{
			Success: false,
			Error:   fmt.Errorf("failed to store updated manifest: %w", err),
		}
	}

	// Update sync state with new manifest CID
	if syncState.RemoteSnapshot == nil {
		syncState.RemoteSnapshot = make(map[string]RemoteMetadata)
	}

	syncState.RemoteSnapshot[request.DirectoryPath] = RemoteMetadata{
		Path:          request.DirectoryPath,
		DescriptorCID: newManifestCID,
		Size:          0,
		ModTime:       time.Now(),
		IsDir:         true,
		LastSyncTime:  time.Now(),
		Version:       1,
	}

	if err := m.stateStore.SaveState(request.SyncID, syncState); err != nil {
		return &ManifestUpdateResult{
			Success: false,
			Error:   fmt.Errorf("failed to save sync state: %w", err),
		}
	}

	return &ManifestUpdateResult{
		Success:      true,
		NewCID:       newManifestCID,
		UpdatedPaths: []string{request.DirectoryPath},
	}
}

// applyManifestOperation applies the requested operation to the manifest
func (m *ManifestUpdateManager) applyManifestOperation(manifest *blocks.DirectoryManifest, request *ManifestUpdateRequest) error {
	switch request.Operation {
	case ManifestOpAdd:
		if request.FileEntry == nil {
			return fmt.Errorf("file entry required for add operation")
		}
		return manifest.AddEntry(*request.FileEntry)

	case ManifestOpUpdate:
		if request.FileEntry == nil {
			return fmt.Errorf("file entry required for update operation")
		}
		// Find existing entry and update it
		return manifest.UpdateEntry(request.FileEntry.EncryptedName, *request.FileEntry)

	case ManifestOpRemove:
		if request.FileEntry == nil {
			return fmt.Errorf("file entry required for remove operation")
		}
		return manifest.RemoveEntry(request.FileEntry.EncryptedName)

	default:
		return fmt.Errorf("unknown manifest operation: %s", request.Operation)
	}
}

// getDirLock gets or creates a mutex for the given directory path
func (m *ManifestUpdateManager) getDirLock(dirPath string) *sync.Mutex {
	m.lockMu.RLock()
	if lock, exists := m.dirLocks[dirPath]; exists {
		m.lockMu.RUnlock()
		return lock
	}
	m.lockMu.RUnlock()

	m.lockMu.Lock()
	defer m.lockMu.Unlock()

	// Double-check after acquiring write lock
	if lock, exists := m.dirLocks[dirPath]; exists {
		return lock
	}

	lock := &sync.Mutex{}
	m.dirLocks[dirPath] = lock
	return lock
}

// updateStats updates statistics with the given function
func (m *ManifestUpdateManager) updateStats(updateFunc func(*ManifestUpdateStats)) {
	m.statsMu.Lock()
	defer m.statsMu.Unlock()
	updateFunc(m.stats)
}

// GetStats returns current statistics
func (m *ManifestUpdateManager) GetStats() *ManifestUpdateStats {
	m.statsMu.RLock()
	defer m.statsMu.RUnlock()

	return &ManifestUpdateStats{
		TotalRequests:     m.stats.TotalRequests,
		SuccessfulUpdates: m.stats.SuccessfulUpdates,
		FailedUpdates:     m.stats.FailedUpdates,
		RetryCount:        m.stats.RetryCount,
		AverageUpdateTime: m.stats.AverageUpdateTime,
		LastUpdateTime:    m.stats.LastUpdateTime,
	}
}

// Stop stops the manifest update manager
func (m *ManifestUpdateManager) Stop() error {
	m.cancel()

	// Close update queue
	close(m.updateQueue)

	// Wait for workers to finish
	m.wg.Wait()

	return nil
}

// PropagateToAncestors propagates manifest updates up the directory tree
func (m *ManifestUpdateManager) PropagateToAncestors(
	syncID string,
	dirPath string,
	newManifestCID string,
) (*ManifestUpdateResult, error) {
	var updatedPaths []string
	currentPath := dirPath

	for {
		parentPath := filepath.Dir(currentPath)
		if parentPath == currentPath || parentPath == "." {
			// Reached root
			break
		}

		// Get the directory name for the current path
		dirName := filepath.Base(currentPath)

		// Update parent manifest with new CID for this directory
		result, err := m.UpdateAfterFileOperation(
			syncID,
			filepath.Join(parentPath, dirName),
			ManifestOpUpdate,
			newManifestCID,
			"", // oldCID not needed for directory updates
		)
		if err != nil {
			return &ManifestUpdateResult{
				Success:      false,
				Error:        fmt.Errorf("failed to update parent manifest %s: %w", parentPath, err),
				UpdatedPaths: updatedPaths,
			}, nil
		}

		updatedPaths = append(updatedPaths, parentPath)
		currentPath = parentPath
		newManifestCID = result.NewCID
	}

	return &ManifestUpdateResult{
		Success:      true,
		NewCID:       newManifestCID,
		UpdatedPaths: updatedPaths,
	}, nil
}

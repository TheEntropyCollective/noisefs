package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
)

// DirectoryScanner handles scanning of local and remote directories
type DirectoryScanner struct {
	directoryManager *storage.DirectoryManager
	stateComparator  *StateComparator
}

// NewDirectoryScanner creates a new directory scanner
func NewDirectoryScanner(directoryManager *storage.DirectoryManager) *DirectoryScanner {
	return &DirectoryScanner{
		directoryManager: directoryManager,
		stateComparator:  NewStateComparator(),
	}
}

// ScanResult contains the results of a directory scan
type ScanResult struct {
	LocalSnapshot  map[string]FileMetadata   `json:"local_snapshot"`
	RemoteSnapshot map[string]RemoteMetadata `json:"remote_snapshot"`
	Changes        []DetectedChange          `json:"changes"`
	ScanDuration   time.Duration             `json:"scan_duration"`
}

// PerformInitialScan performs initial scanning of both local and remote directories
func (ds *DirectoryScanner) PerformInitialScan(ctx context.Context, localPath, remotePath, manifestCID string, previousState *SyncState) (*ScanResult, error) {
	startTime := time.Now()

	// Create result structure
	result := &ScanResult{
		LocalSnapshot:  make(map[string]FileMetadata),
		RemoteSnapshot: make(map[string]RemoteMetadata),
		Changes:        make([]DetectedChange, 0),
	}

	// Scan local directory
	localSnapshot, err := ds.ScanLocalDirectory(localPath)
	if err != nil {
		return nil, fmt.Errorf("failed to scan local directory: %w", err)
	}
	result.LocalSnapshot = localSnapshot

	// Scan remote directory if manifest CID is provided
	if manifestCID != "" {
		remoteSnapshot, err := ds.ScanRemoteDirectory(ctx, remotePath, manifestCID)
		if err != nil {
			return nil, fmt.Errorf("failed to scan remote directory: %w", err)
		}
		result.RemoteSnapshot = remoteSnapshot
	}

	// Compare with previous state if available
	if previousState != nil {
		oldSnapshot := &StateSnapshot{
			LocalFiles:  previousState.LocalSnapshot,
			RemoteFiles: previousState.RemoteSnapshot,
			Timestamp:   previousState.LastSync,
		}

		newSnapshot := &StateSnapshot{
			LocalFiles:  result.LocalSnapshot,
			RemoteFiles: result.RemoteSnapshot,
			Timestamp:   time.Now(),
		}

		changes, err := ds.stateComparator.CompareStates(oldSnapshot, newSnapshot)
		if err != nil {
			return nil, fmt.Errorf("failed to compare states: %w", err)
		}
		result.Changes = changes
	} else {
		// For initial sync without previous state, generate changes for all files
		result.Changes = ds.generateInitialChanges(result.LocalSnapshot, result.RemoteSnapshot)
	}

	result.ScanDuration = time.Since(startTime)
	return result, nil
}

// ScanLocalDirectory recursively scans a local directory and returns file metadata
func (ds *DirectoryScanner) ScanLocalDirectory(rootPath string) (map[string]FileMetadata, error) {
	snapshot := make(map[string]FileMetadata)

	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Log error but continue scanning
			fmt.Printf("Warning: failed to access %s: %v\n", path, err)
			return nil
		}

		// Get relative path from root
		relativePath, err := filepath.Rel(rootPath, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		// Skip root directory itself
		if relativePath == "." {
			return nil
		}

		// Create file metadata
		metadata := FileMetadata{
			Path:        relativePath,
			Size:        info.Size(),
			ModTime:     info.ModTime(),
			IsDir:       info.IsDir(),
			Permissions: uint32(info.Mode().Perm()),
		}

		// Get system-specific metadata for move detection
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			metadata.Inode = stat.Ino
			metadata.Device = uint64(stat.Dev)
		}

		// Calculate checksum for files (not directories)
		if !info.IsDir() {
			checksum, err := CalculateFileChecksum(path)
			if err != nil {
				// Log error but continue
				fmt.Printf("Warning: failed to calculate checksum for %s: %v\n", path, err)
			} else {
				metadata.Checksum = checksum
			}
		}

		snapshot[relativePath] = metadata
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory %s: %w", rootPath, err)
	}

	return snapshot, nil
}

// ScanRemoteDirectory retrieves and parses a remote directory manifest
func (ds *DirectoryScanner) ScanRemoteDirectory(ctx context.Context, remotePath, manifestCID string) (map[string]RemoteMetadata, error) {
	// Retrieve directory manifest using the directory manager
	manifest, err := ds.directoryManager.RetrieveDirectoryManifest(ctx, remotePath, manifestCID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve directory manifest: %w", err)
	}

	// Convert manifest entries to remote metadata
	snapshot := make(map[string]RemoteMetadata)

	for _, entry := range manifest.Entries {
		// Decrypt filename if needed
		// For now, assuming entries have been decrypted by DirectoryManager
		// In a full implementation, this would need proper key management
		filename := string(entry.EncryptedName) // TODO: Implement proper decryption

		metadata := RemoteMetadata{
			Path:          filename,
			DescriptorCID: entry.CID,
			Size:          entry.Size,
			ModTime:       entry.ModifiedAt,
			IsDir:         entry.Type == blocks.DirectoryType,
			LastSyncTime:  time.Now(),
			Version:       1, // TODO: Implement proper versioning
		}

		snapshot[filename] = metadata
	}

	return snapshot, nil
}

// generateInitialChanges generates changes for initial sync when no previous state exists
func (ds *DirectoryScanner) generateInitialChanges(localSnapshot map[string]FileMetadata, remoteSnapshot map[string]RemoteMetadata) []DetectedChange {
	var changes []DetectedChange

	// All local files are potential uploads
	for path, metadata := range localSnapshot {
		// Check if file exists remotely
		if _, exists := remoteSnapshot[path]; !exists {
			changes = append(changes, DetectedChange{
				Type:      ChangeTypeCreate,
				Path:      path,
				IsLocal:   true,
				Metadata:  metadata,
				Timestamp: time.Now(),
			})
		}
	}

	// All remote files not in local are potential downloads
	for path, metadata := range remoteSnapshot {
		if _, exists := localSnapshot[path]; !exists {
			changes = append(changes, DetectedChange{
				Type:      ChangeTypeCreate,
				Path:      path,
				IsLocal:   false,
				Metadata:  metadata,
				Timestamp: time.Now(),
			})
		}
	}

	// Files existing in both may need sync based on modification time or checksums
	for path, localMeta := range localSnapshot {
		if remoteMeta, exists := remoteSnapshot[path]; exists {
			// Compare modification times and sizes
			if ds.needsSync(localMeta, remoteMeta) {
				// Determine which is newer
				if localMeta.ModTime.After(remoteMeta.ModTime) {
					changes = append(changes, DetectedChange{
						Type:      ChangeTypeModify,
						Path:      path,
						IsLocal:   true,
						Metadata:  localMeta,
						Timestamp: time.Now(),
					})
				} else if remoteMeta.ModTime.After(localMeta.ModTime) {
					changes = append(changes, DetectedChange{
						Type:      ChangeTypeModify,
						Path:      path,
						IsLocal:   false,
						Metadata:  remoteMeta,
						Timestamp: time.Now(),
					})
				}
			}
		}
	}

	return changes
}

// needsSync determines if a file needs synchronization based on metadata
func (ds *DirectoryScanner) needsSync(local FileMetadata, remote RemoteMetadata) bool {
	// Different sizes definitely need sync
	if local.Size != remote.Size {
		return true
	}

	// Different modification times might need sync
	if !local.ModTime.Equal(remote.ModTime) {
		return true
	}

	// Directory type mismatch
	if local.IsDir != remote.IsDir {
		return true
	}

	return false
}

// GenerateSyncOperations converts detected changes into sync operations
func (ds *DirectoryScanner) GenerateSyncOperations(sessionID string, changes []DetectedChange, localBasePath, remoteBasePath string) []SyncOperation {
	var operations []SyncOperation

	for _, change := range changes {
		var op SyncOperation
		op.ID = fmt.Sprintf("%s-%s-%d", sessionID, change.Path, time.Now().UnixNano())
		op.Timestamp = change.Timestamp
		op.Status = OpStatusPending
		op.Retries = 0

		// Calculate full paths
		op.LocalPath = filepath.Join(localBasePath, change.Path)
		op.RemotePath = filepath.Join(remoteBasePath, change.Path)

		// Determine operation type based on change
		switch change.Type {
		case ChangeTypeCreate:
			if change.IsLocal {
				if change.Metadata != nil {
					if fileMeta, ok := change.Metadata.(FileMetadata); ok && fileMeta.IsDir {
						op.Type = OpTypeCreateDir
					} else {
						op.Type = OpTypeUpload
					}
				} else {
					op.Type = OpTypeUpload
				}
			} else {
				if change.Metadata != nil {
					if remoteMeta, ok := change.Metadata.(RemoteMetadata); ok && remoteMeta.IsDir {
						op.Type = OpTypeCreateDir
					} else {
						op.Type = OpTypeDownload
					}
				} else {
					op.Type = OpTypeDownload
				}
			}

		case ChangeTypeModify:
			if change.IsLocal {
				op.Type = OpTypeUpload
			} else {
				op.Type = OpTypeDownload
			}

		case ChangeTypeDelete:
			if change.IsLocal {
				// Local file was deleted, remove from remote
				op.Type = OpTypeDelete
			} else {
				// Remote file was deleted, remove from local
				op.Type = OpTypeDelete
			}

		case ChangeTypeMove, ChangeTypeRename:
			// Handle move operations
			op.Type = OpTypeMove
			if change.OldPath != "" {
				op.LocalPath = filepath.Join(localBasePath, change.OldPath)
			}

		default:
			// Skip unknown change types
			continue
		}

		operations = append(operations, op)
	}

	// Sort operations by priority (directories first, then files, deletes last)
	return ds.prioritizeOperations(operations)
}

// prioritizeOperations sorts sync operations by priority
func (ds *DirectoryScanner) prioritizeOperations(operations []SyncOperation) []SyncOperation {
	// Create separate slices for different operation types
	var dirOps, fileOps, deleteOps []SyncOperation

	for _, op := range operations {
		switch op.Type {
		case OpTypeCreateDir:
			dirOps = append(dirOps, op)
		case OpTypeDelete, OpTypeDeleteDir:
			deleteOps = append(deleteOps, op)
		default:
			fileOps = append(fileOps, op)
		}
	}

	// Combine in priority order: directories first, then files, then deletes
	result := make([]SyncOperation, 0, len(operations))
	result = append(result, dirOps...)
	result = append(result, fileOps...)
	result = append(result, deleteOps...)

	return result
}
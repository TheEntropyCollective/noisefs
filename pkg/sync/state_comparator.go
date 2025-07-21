package sync

import (
	"path/filepath"
	"sort"
	"time"
)

// StateComparator provides functionality for comparing sync states and detecting changes
type StateComparator struct {
	moveDetector *MoveDetector
}

// NewStateComparator creates a new state comparator
func NewStateComparator() *StateComparator {
	return &StateComparator{
		moveDetector: NewMoveDetector(),
	}
}

// CompareStates compares two state snapshots and returns detected changes
func (sc *StateComparator) CompareStates(oldSnapshot, newSnapshot *StateSnapshot) ([]DetectedChange, error) {
	var changes []DetectedChange

	// Compare local files
	localChanges := sc.compareLocalFiles(oldSnapshot.LocalFiles, newSnapshot.LocalFiles)
	changes = append(changes, localChanges...)

	// Compare remote files
	remoteChanges := sc.compareRemoteFiles(oldSnapshot.RemoteFiles, newSnapshot.RemoteFiles)
	changes = append(changes, remoteChanges...)

	// Detect moves and renames
	moves := sc.moveDetector.DetectMoves(oldSnapshot, newSnapshot)
	for _, move := range moves {
		change := DetectedChange{
			Type:      ChangeTypeMove,
			Path:      move.NewPath,
			OldPath:   move.OldPath,
			IsLocal:   move.IsLocal,
			Metadata:  move.Metadata,
			Timestamp: time.Now(),
		}
		changes = append(changes, change)
	}

	// Sort changes by path for consistent ordering
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].Path < changes[j].Path
	})

	return changes, nil
}

// compareLocalFiles compares local file maps and detects changes
func (sc *StateComparator) compareLocalFiles(oldFiles, newFiles map[string]FileMetadata) []DetectedChange {
	var changes []DetectedChange

	// Find all paths in both snapshots
	allPaths := make(map[string]bool)
	for path := range oldFiles {
		allPaths[path] = true
	}
	for path := range newFiles {
		allPaths[path] = true
	}

	for path := range allPaths {
		oldFile, oldExists := oldFiles[path]
		newFile, newExists := newFiles[path]

		if !oldExists && newExists {
			// File created
			changes = append(changes, DetectedChange{
				Type:      ChangeTypeCreate,
				Path:      path,
				IsLocal:   true,
				Metadata:  newFile,
				Timestamp: time.Now(),
			})
		} else if oldExists && !newExists {
			// File deleted
			changes = append(changes, DetectedChange{
				Type:        ChangeTypeDelete,
				Path:        path,
				IsLocal:     true,
				OldMetadata: oldFile,
				Timestamp:   time.Now(),
			})
		} else if oldExists && newExists {
			// Check if file modified
			if sc.isFileModified(oldFile, newFile) {
				changes = append(changes, DetectedChange{
					Type:        ChangeTypeModify,
					Path:        path,
					IsLocal:     true,
					Metadata:    newFile,
					OldMetadata: oldFile,
					Timestamp:   time.Now(),
				})
			}
		}
	}

	return changes
}

// compareRemoteFiles compares remote file maps and detects changes
func (sc *StateComparator) compareRemoteFiles(oldFiles, newFiles map[string]RemoteMetadata) []DetectedChange {
	var changes []DetectedChange

	// Find all paths in both snapshots
	allPaths := make(map[string]bool)
	for path := range oldFiles {
		allPaths[path] = true
	}
	for path := range newFiles {
		allPaths[path] = true
	}

	for path := range allPaths {
		oldFile, oldExists := oldFiles[path]
		newFile, newExists := newFiles[path]

		if !oldExists && newExists {
			// File created
			changes = append(changes, DetectedChange{
				Type:      ChangeTypeCreate,
				Path:      path,
				IsLocal:   false,
				Metadata:  newFile,
				Timestamp: time.Now(),
			})
		} else if oldExists && !newExists {
			// File deleted
			changes = append(changes, DetectedChange{
				Type:        ChangeTypeDelete,
				Path:        path,
				IsLocal:     false,
				OldMetadata: oldFile,
				Timestamp:   time.Now(),
			})
		} else if oldExists && newExists {
			// Check if file modified
			if sc.isRemoteFileModified(oldFile, newFile) {
				changes = append(changes, DetectedChange{
					Type:        ChangeTypeModify,
					Path:        path,
					IsLocal:     false,
					Metadata:    newFile,
					OldMetadata: oldFile,
					Timestamp:   time.Now(),
				})
			}
		}
	}

	return changes
}

// isFileModified checks if a local file has been modified
func (sc *StateComparator) isFileModified(oldFile, newFile FileMetadata) bool {
	// Different modification times indicate potential changes
	if !oldFile.ModTime.Equal(newFile.ModTime) {
		return true
	}

	// Different sizes definitely indicate changes
	if oldFile.Size != newFile.Size {
		return true
	}

	// Different checksums indicate content changes
	if oldFile.Checksum != "" && newFile.Checksum != "" && oldFile.Checksum != newFile.Checksum {
		return true
	}

	// Different permissions
	if oldFile.Permissions != newFile.Permissions {
		return true
	}

	return false
}

// isRemoteFileModified checks if a remote file has been modified
func (sc *StateComparator) isRemoteFileModified(oldFile, newFile RemoteMetadata) bool {
	// Different descriptor CIDs indicate changes
	if oldFile.DescriptorCID != newFile.DescriptorCID {
		return true
	}

	// Different content CIDs indicate content changes
	if oldFile.ContentCID != newFile.ContentCID {
		return true
	}

	// Different sizes indicate changes
	if oldFile.Size != newFile.Size {
		return true
	}

	// Different modification times
	if !oldFile.ModTime.Equal(newFile.ModTime) {
		return true
	}

	// Different versions
	if oldFile.Version != newFile.Version {
		return true
	}

	return false
}

// DetectConflicts detects conflicts between local and remote changes
func (sc *StateComparator) DetectConflicts(localChanges, remoteChanges []DetectedChange) []Conflict {
	var conflicts []Conflict

	// Create maps for efficient lookup
	localChangeMap := make(map[string]DetectedChange)
	for _, change := range localChanges {
		localChangeMap[change.Path] = change
	}

	remoteChangeMap := make(map[string]DetectedChange)
	for _, change := range remoteChanges {
		remoteChangeMap[change.Path] = change
	}

	// Find conflicting paths
	allPaths := make(map[string]bool)
	for path := range localChangeMap {
		allPaths[path] = true
	}
	for path := range remoteChangeMap {
		allPaths[path] = true
	}

	for path := range allPaths {
		localChange, hasLocal := localChangeMap[path]
		remoteChange, hasRemote := remoteChangeMap[path]

		if hasLocal && hasRemote {
			conflictType := sc.determineConflictType(localChange, remoteChange)
			if conflictType != "" {
				conflict := Conflict{
					ID:           generateConflictID(path),
					LocalPath:    path,
					RemotePath:   path,
					ConflictType: conflictType,
					Timestamp:    time.Now(),
				}

				// Add metadata if available
				if localChange.Metadata != nil {
					if localMeta, ok := localChange.Metadata.(FileMetadata); ok {
						conflict.LocalMetadata = localMeta
					}
				}
				if remoteChange.Metadata != nil {
					if remoteMeta, ok := remoteChange.Metadata.(RemoteMetadata); ok {
						conflict.RemoteMetadata = remoteMeta
					}
				}

				conflicts = append(conflicts, conflict)
			}
		}
	}

	return conflicts
}

// determineConflictType determines the type of conflict based on local and remote changes
func (sc *StateComparator) determineConflictType(localChange, remoteChange DetectedChange) ConflictType {
	if localChange.Type == ChangeTypeModify && remoteChange.Type == ChangeTypeModify {
		return ConflictTypeBothModified
	}
	if localChange.Type == ChangeTypeDelete && remoteChange.Type == ChangeTypeModify {
		return ConflictTypeDeletedLocal
	}
	if localChange.Type == ChangeTypeModify && remoteChange.Type == ChangeTypeDelete {
		return ConflictTypeDeletedRemote
	}
	if (localChange.Type == ChangeTypeCreate && remoteChange.Type == ChangeTypeCreate) ||
		(localChange.Type == ChangeTypeModify && remoteChange.Type == ChangeTypeCreate) ||
		(localChange.Type == ChangeTypeCreate && remoteChange.Type == ChangeTypeModify) {
		// Check if types changed (file vs directory)
		return ConflictTypeTypeChanged
	}

	return ""
}

// generateConflictID generates a unique ID for a conflict
func generateConflictID(path string) string {
	return filepath.Base(path) + "_" + time.Now().Format("20060102_150405")
}

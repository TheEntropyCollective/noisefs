package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// GatherFileMetadata collects comprehensive metadata for a local file
func GatherFileMetadata(filePath string, includeChecksum bool) (*FileMetadata, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file %s: %w", filePath, err)
	}

	metadata := &FileMetadata{
		Path:        filePath,
		Size:        info.Size(),
		ModTime:     info.ModTime(),
		IsDir:       info.IsDir(),
		Permissions: uint32(info.Mode()),
	}

	// Get system-specific attributes for move detection
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		metadata.Inode = stat.Ino
		metadata.Device = uint64(stat.Dev)
	}

	// Calculate checksum if requested and not a directory
	if includeChecksum && !info.IsDir() {
		checksum, err := CalculateFileChecksum(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate checksum: %w", err)
		}
		metadata.Checksum = checksum
	}

	return metadata, nil
}

// GatherDirectoryMetadata recursively gathers metadata for all files in a directory
func GatherDirectoryMetadata(dirPath string, includeChecksum bool) (map[string]FileMetadata, error) {
	metadata := make(map[string]FileMetadata)

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get relative path from the base directory
		relPath, err := filepath.Rel(dirPath, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		fileMeta, err := GatherFileMetadata(path, includeChecksum)
		if err != nil {
			return err
		}

		// Use relative path as key
		fileMeta.Path = relPath
		metadata[relPath] = *fileMeta

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to gather directory metadata: %w", err)
	}

	return metadata, nil
}

// UpdateRemoteMetadata creates or updates remote metadata with current timestamp
func UpdateRemoteMetadata(path, descriptorCID, contentCID string, size int64, modTime time.Time, isDir bool) *RemoteMetadata {
	return &RemoteMetadata{
		Path:          path,
		DescriptorCID: descriptorCID,
		ContentCID:    contentCID,
		Size:          size,
		ModTime:       modTime,
		IsDir:         isDir,
		LastSyncTime:  time.Now(),
		Version:       time.Now().UnixNano(),
	}
}

// CreateSnapshot creates a complete snapshot of both local and remote file systems
func CreateSnapshot(localPath string, remoteFiles map[string]RemoteMetadata, includeChecksum bool) (*StateSnapshot, error) {
	localFiles, err := GatherDirectoryMetadata(localPath, includeChecksum)
	if err != nil {
		return nil, fmt.Errorf("failed to gather local metadata: %w", err)
	}

	return &StateSnapshot{
		LocalFiles:  localFiles,
		RemoteFiles: remoteFiles,
		Timestamp:   time.Now(),
		Version:     time.Now().UnixNano(),
	}, nil
}

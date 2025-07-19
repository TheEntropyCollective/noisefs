package security

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// ErrPathTraversal indicates a path traversal attack attempt
var ErrPathTraversal = fmt.Errorf("path traversal attempt detected")

// ValidatePathInBounds ensures path is within allowed directory
// This function prevents directory traversal attacks by validating that
// the requested path stays within the allowed root directory.
func ValidatePathInBounds(path, allowedRoot string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}
	if allowedRoot == "" {
		return fmt.Errorf("allowed root cannot be empty")
	}

	// Clean paths to remove . and .. elements
	cleanPath := filepath.Clean(path)
	cleanRoot := filepath.Clean(allowedRoot)
	
	// For relative paths, resolve them relative to the allowed root
	var absPath string
	if filepath.IsAbs(cleanPath) {
		absPath = cleanPath
	} else {
		// Join relative path with allowed root and then get absolute path
		absPath = filepath.Join(cleanRoot, cleanPath)
	}
	
	// Clean the final absolute path
	absPath = filepath.Clean(absPath)
	
	// Get absolute form of the root
	absRoot, err := filepath.Abs(cleanRoot)
	if err != nil {
		return fmt.Errorf("invalid root directory: %w", err)
	}
	
	// Clean the absolute root
	absRoot = filepath.Clean(absRoot)
	
	// Ensure absRoot ends with separator for precise prefix matching
	if !strings.HasSuffix(absRoot, string(filepath.Separator)) {
		absRoot += string(filepath.Separator)
	}
	
	// For the path comparison, add separator if it's not the root itself
	pathToCheck := absPath
	if absPath != strings.TrimSuffix(absRoot, string(filepath.Separator)) {
		pathToCheck = absPath + string(filepath.Separator)
	} else {
		// If the path equals the root directory, allow it
		return nil
	}
	
	// Check if path is within root directory
	if !strings.HasPrefix(pathToCheck, absRoot) {
		return ErrPathTraversal
	}
	
	return nil
}

// ValidateSyncID validates sync IDs to prevent path traversal in state files
// Sync IDs should be alphanumeric with hyphens and underscores only
func ValidateSyncID(syncID string) error {
	if syncID == "" {
		return fmt.Errorf("sync ID cannot be empty")
	}
	
	// Allow only safe characters: alphanumeric, hyphens, underscores
	// This prevents path traversal attempts like "../../../sensitive"
	validSyncID := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !validSyncID.MatchString(syncID) {
		return fmt.Errorf("invalid sync ID format: contains unsafe characters")
	}
	
	// Additional length check to prevent abuse
	if len(syncID) > 100 {
		return fmt.Errorf("sync ID too long: maximum 100 characters")
	}
	
	return nil
}

// ValidateFileName validates file names to prevent path traversal
func ValidateFileName(fileName string) error {
	if fileName == "" {
		return fmt.Errorf("file name cannot be empty")
	}
	
	// Check for path traversal patterns
	if strings.Contains(fileName, "..") {
		return fmt.Errorf("file name contains path traversal: %s", fileName)
	}
	
	// Check for absolute paths
	if filepath.IsAbs(fileName) {
		return fmt.Errorf("file name cannot be absolute path: %s", fileName)
	}
	
	return nil
}
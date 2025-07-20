package resilience

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// FileBackupAction creates a backup of a file before modification
type FileBackupAction struct {
	ID          string
	FilePath    string
	BackupPath  string
	Description string
}

// NewFileBackupAction creates a new file backup action
func NewFileBackupAction(id, filePath string) *FileBackupAction {
	backupPath := filePath + ".backup." + id
	return &FileBackupAction{
		ID:          id,
		FilePath:    filePath,
		BackupPath:  backupPath,
		Description: fmt.Sprintf("Backup file %s", filePath),
	}
}

// Execute creates a backup of the file
func (fba *FileBackupAction) Execute(ctx context.Context) error {
	// Check if original file exists
	if _, err := os.Stat(fba.FilePath); os.IsNotExist(err) {
		// File doesn't exist, nothing to backup
		return nil
	}

	// Read original file
	data, err := os.ReadFile(fba.FilePath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", fba.FilePath, err)
	}

	// Create backup directory if needed
	backupDir := filepath.Dir(fba.BackupPath)
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory %s: %w", backupDir, err)
	}

	// Write backup file
	if err := os.WriteFile(fba.BackupPath, data, 0644); err != nil {
		return fmt.Errorf("failed to create backup %s: %w", fba.BackupPath, err)
	}

	return nil
}

// Rollback removes the backup file
func (fba *FileBackupAction) Rollback(ctx context.Context) error {
	if _, err := os.Stat(fba.BackupPath); os.IsNotExist(err) {
		// Backup doesn't exist, nothing to rollback
		return nil
	}

	if err := os.Remove(fba.BackupPath); err != nil {
		return fmt.Errorf("failed to remove backup %s: %w", fba.BackupPath, err)
	}

	return nil
}

// GetID returns the action ID
func (fba *FileBackupAction) GetID() string {
	return fba.ID
}

// GetDescription returns the action description
func (fba *FileBackupAction) GetDescription() string {
	return fba.Description
}

// FileRestoreAction restores a file from its backup
type FileRestoreAction struct {
	ID          string
	FilePath    string
	BackupPath  string
	Description string
}

// NewFileRestoreAction creates a new file restore action
func NewFileRestoreAction(id, filePath, backupPath string) *FileRestoreAction {
	return &FileRestoreAction{
		ID:          id,
		FilePath:    filePath,
		BackupPath:  backupPath,
		Description: fmt.Sprintf("Restore file %s from backup", filePath),
	}
}

// Execute restores the file from backup
func (fra *FileRestoreAction) Execute(ctx context.Context) error {
	// Check if backup exists
	if _, err := os.Stat(fra.BackupPath); os.IsNotExist(err) {
		return fmt.Errorf("backup file %s does not exist", fra.BackupPath)
	}

	// Read backup file
	data, err := os.ReadFile(fra.BackupPath)
	if err != nil {
		return fmt.Errorf("failed to read backup %s: %w", fra.BackupPath, err)
	}

	// Create directory if needed
	dir := filepath.Dir(fra.FilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Restore file
	if err := os.WriteFile(fra.FilePath, data, 0644); err != nil {
		return fmt.Errorf("failed to restore file %s: %w", fra.FilePath, err)
	}

	return nil
}

// Rollback removes the restored file
func (fra *FileRestoreAction) Rollback(ctx context.Context) error {
	if _, err := os.Stat(fra.FilePath); os.IsNotExist(err) {
		// File doesn't exist, nothing to rollback
		return nil
	}

	if err := os.Remove(fra.FilePath); err != nil {
		return fmt.Errorf("failed to remove restored file %s: %w", fra.FilePath, err)
	}

	return nil
}

// GetID returns the action ID
func (fra *FileRestoreAction) GetID() string {
	return fra.ID
}

// GetDescription returns the action description
func (fra *FileRestoreAction) GetDescription() string {
	return fra.Description
}

// StateSnapshotAction captures a snapshot of application state
type StateSnapshotAction struct {
	ID          string
	StatePath   string
	StateData   interface{}
	Description string
	
	// Function to capture state
	CaptureFunc func(ctx context.Context) (interface{}, error)
	// Function to restore state
	RestoreFunc func(ctx context.Context, state interface{}) error
}

// NewStateSnapshotAction creates a new state snapshot action
func NewStateSnapshotAction(id, statePath string, captureFunc func(ctx context.Context) (interface{}, error), restoreFunc func(ctx context.Context, state interface{}) error) *StateSnapshotAction {
	return &StateSnapshotAction{
		ID:          id,
		StatePath:   statePath,
		Description: fmt.Sprintf("Snapshot state %s", statePath),
		CaptureFunc: captureFunc,
		RestoreFunc: restoreFunc,
	}
}

// Execute captures the current state
func (ssa *StateSnapshotAction) Execute(ctx context.Context) error {
	if ssa.CaptureFunc == nil {
		return fmt.Errorf("no capture function provided")
	}

	state, err := ssa.CaptureFunc(ctx)
	if err != nil {
		return fmt.Errorf("failed to capture state: %w", err)
	}

	ssa.StateData = state
	return nil
}

// Rollback restores the captured state
func (ssa *StateSnapshotAction) Rollback(ctx context.Context) error {
	if ssa.RestoreFunc == nil {
		return fmt.Errorf("no restore function provided")
	}

	if ssa.StateData == nil {
		return fmt.Errorf("no state data to restore")
	}

	if err := ssa.RestoreFunc(ctx, ssa.StateData); err != nil {
		return fmt.Errorf("failed to restore state: %w", err)
	}

	return nil
}

// GetID returns the action ID
func (ssa *StateSnapshotAction) GetID() string {
	return ssa.ID
}

// GetDescription returns the action description
func (ssa *StateSnapshotAction) GetDescription() string {
	return ssa.Description
}

// FunctionAction wraps arbitrary functions as recovery actions
type FunctionAction struct {
	ID          string
	Description string
	ExecuteFunc func(ctx context.Context) error
	RollbackFunc func(ctx context.Context) error
}

// NewFunctionAction creates a new function action
func NewFunctionAction(id, description string, executeFunc, rollbackFunc func(ctx context.Context) error) *FunctionAction {
	return &FunctionAction{
		ID:           id,
		Description:  description,
		ExecuteFunc:  executeFunc,
		RollbackFunc: rollbackFunc,
	}
}

// Execute runs the execute function
func (fa *FunctionAction) Execute(ctx context.Context) error {
	if fa.ExecuteFunc == nil {
		return fmt.Errorf("no execute function provided")
	}
	return fa.ExecuteFunc(ctx)
}

// Rollback runs the rollback function
func (fa *FunctionAction) Rollback(ctx context.Context) error {
	if fa.RollbackFunc == nil {
		return fmt.Errorf("no rollback function provided")
	}
	return fa.RollbackFunc(ctx)
}

// GetID returns the action ID
func (fa *FunctionAction) GetID() string {
	return fa.ID
}

// GetDescription returns the action description
func (fa *FunctionAction) GetDescription() string {
	return fa.Description
}

// DirectoryStateValidator validates directory structure and file consistency
type DirectoryStateValidator struct {
	Name      string
	Directory string
}

// NewDirectoryStateValidator creates a new directory state validator
func NewDirectoryStateValidator(name, directory string) *DirectoryStateValidator {
	return &DirectoryStateValidator{
		Name:      name,
		Directory: directory,
	}
}

// Validate checks directory consistency
func (dsv *DirectoryStateValidator) Validate(ctx context.Context, state interface{}) error {
	// Check if directory exists
	if _, err := os.Stat(dsv.Directory); os.IsNotExist(err) {
		return fmt.Errorf("directory %s does not exist", dsv.Directory)
	}

	// Validate based on provided state
	if expectedState, ok := state.(map[string]interface{}); ok {
		if expectedFiles, exists := expectedState["files"]; exists {
			if fileList, ok := expectedFiles.([]string); ok {
				for _, file := range fileList {
					fullPath := filepath.Join(dsv.Directory, file)
					if _, err := os.Stat(fullPath); os.IsNotExist(err) {
						return fmt.Errorf("expected file %s does not exist", fullPath)
					}
				}
			}
		}
	}

	return nil
}

// GetName returns the validator name
func (dsv *DirectoryStateValidator) GetName() string {
	return dsv.Name
}

// FileIntegrityValidator validates file checksums and integrity
type FileIntegrityValidator struct {
	Name     string
	FilePath string
	Expected string // Expected checksum or content hash
}

// NewFileIntegrityValidator creates a new file integrity validator
func NewFileIntegrityValidator(name, filePath, expectedChecksum string) *FileIntegrityValidator {
	return &FileIntegrityValidator{
		Name:     name,
		FilePath: filePath,
		Expected: expectedChecksum,
	}
}

// Validate checks file integrity
func (fiv *FileIntegrityValidator) Validate(ctx context.Context, state interface{}) error {
	// Check if file exists
	if _, err := os.Stat(fiv.FilePath); os.IsNotExist(err) {
		return fmt.Errorf("file %s does not exist", fiv.FilePath)
	}

	// Simple validation - in production you'd calculate actual checksums
	// For now, just validate the file is readable
	if _, err := os.ReadFile(fiv.FilePath); err != nil {
		return fmt.Errorf("failed to read file %s: %w", fiv.FilePath, err)
	}

	// If state provides expected checksum, validate it
	if stateMap, ok := state.(map[string]interface{}); ok {
		if expectedChecksum, exists := stateMap["checksum"]; exists {
			if checksum, ok := expectedChecksum.(string); ok {
				if checksum != fiv.Expected {
					return fmt.Errorf("file %s checksum mismatch: expected %s, got %s", fiv.FilePath, fiv.Expected, checksum)
				}
			}
		}
	}

	return nil
}

// GetName returns the validator name
func (fiv *FileIntegrityValidator) GetName() string {
	return fiv.Name
}
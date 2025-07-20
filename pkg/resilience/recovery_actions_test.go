package resilience

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFunctionAction(t *testing.T) {
	executed := false
	rolledBack := false

	executeFunc := func(ctx context.Context) error {
		executed = true
		return nil
	}

	rollbackFunc := func(ctx context.Context) error {
		rolledBack = true
		return nil
	}

	action := NewFunctionAction("test-func", "Test function action", executeFunc, rollbackFunc)

	// Test execution
	err := action.Execute(context.Background())
	if err != nil {
		t.Errorf("Expected no error executing function, got %v", err)
	}

	if !executed {
		t.Errorf("Expected function to be executed")
	}

	// Test rollback
	err = action.Rollback(context.Background())
	if err != nil {
		t.Errorf("Expected no error rolling back function, got %v", err)
	}

	if !rolledBack {
		t.Errorf("Expected function to be rolled back")
	}

	// Test action properties
	if action.GetID() != "test-func" {
		t.Errorf("Expected ID 'test-func', got %s", action.GetID())
	}

	if action.GetDescription() != "Test function action" {
		t.Errorf("Expected description 'Test function action', got %s", action.GetDescription())
	}
}

func TestFunctionAction_NoFunctions(t *testing.T) {
	action := NewFunctionAction("test", "Test", nil, nil)

	// Test execute with no function
	err := action.Execute(context.Background())
	if err == nil {
		t.Errorf("Expected error when no execute function provided")
	}

	// Test rollback with no function
	err = action.Rollback(context.Background())
	if err == nil {
		t.Errorf("Expected error when no rollback function provided")
	}
}

func TestFunctionAction_ExecuteFailure(t *testing.T) {
	expectedError := errors.New("execute failed")

	executeFunc := func(ctx context.Context) error {
		return expectedError
	}

	rollbackFunc := func(ctx context.Context) error {
		return nil
	}

	action := NewFunctionAction("test-func", "Test function action", executeFunc, rollbackFunc)

	// Test execution failure
	err := action.Execute(context.Background())
	if err != expectedError {
		t.Errorf("Expected execute error %v, got %v", expectedError, err)
	}
}

func TestFileBackupAction_NonExistentFile(t *testing.T) {
	// Create action for non-existent file
	action := NewFileBackupAction("test-backup", "/non/existent/file.txt")

	// Execute should succeed (nothing to backup)
	err := action.Execute(context.Background())
	if err != nil {
		t.Errorf("Expected no error for non-existent file backup, got %v", err)
	}

	// Rollback should succeed (no backup to remove)
	err = action.Rollback(context.Background())
	if err != nil {
		t.Errorf("Expected no error rolling back non-existent backup, got %v", err)
	}
}

func TestFileRestoreAction_NoBackup(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-restore-no-backup-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	targetFile := filepath.Join(tempDir, "target.txt")
	backupFile := filepath.Join(tempDir, "nonexistent-backup.txt")

	action := NewFileRestoreAction("test-restore", targetFile, backupFile)

	// Execute should fail (no backup to restore)
	err = action.Execute(context.Background())
	if err == nil {
		t.Errorf("Expected error when backup file doesn't exist")
	}
}

func TestStateSnapshotAction_NoFunctions(t *testing.T) {
	action := NewStateSnapshotAction("test", "test-state", nil, nil)

	// Execute should fail with no capture function
	err := action.Execute(context.Background())
	if err == nil {
		t.Errorf("Expected error when no capture function provided")
	}

	// Rollback should fail with no restore function
	err = action.Rollback(context.Background())
	if err == nil {
		t.Errorf("Expected error when no restore function provided")
	}
}

func TestStateSnapshotAction_CaptureFailure(t *testing.T) {
	expectedError := errors.New("capture failed")

	captureFunc := func(ctx context.Context) (interface{}, error) {
		return nil, expectedError
	}

	action := NewStateSnapshotAction("test", "test-state", captureFunc, nil)

	// Execute should fail
	err := action.Execute(context.Background())
	if err == nil {
		t.Errorf("Expected capture error")
	}
}

func TestStateSnapshotAction_RestoreFailure(t *testing.T) {
	expectedError := errors.New("restore failed")

	captureFunc := func(ctx context.Context) (interface{}, error) {
		return "test state", nil
	}

	restoreFunc := func(ctx context.Context, state interface{}) error {
		return expectedError
	}

	action := NewStateSnapshotAction("test", "test-state", captureFunc, restoreFunc)

	// Execute to capture state
	err := action.Execute(context.Background())
	if err != nil {
		t.Errorf("Expected no error capturing state, got %v", err)
	}

	// Rollback should fail
	err = action.Rollback(context.Background())
	if err == nil {
		t.Errorf("Expected restore error")
	}
}

func TestStateSnapshotAction_NoStateData(t *testing.T) {
	restoreFunc := func(ctx context.Context, state interface{}) error {
		return nil
	}

	action := NewStateSnapshotAction("test", "test-state", nil, restoreFunc)

	// Rollback without captured state should fail
	err := action.Rollback(context.Background())
	if err == nil {
		t.Errorf("Expected error when no state data to restore")
	}
}

func TestDirectoryStateValidator_NonExistentDirectory(t *testing.T) {
	validator := NewDirectoryStateValidator("test", "/non/existent/directory")

	err := validator.Validate(context.Background(), nil)
	if err == nil {
		t.Errorf("Expected error for non-existent directory")
	}
}

func TestDirectoryStateValidator_EmptyState(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-dir-validator-empty-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	validator := NewDirectoryStateValidator("test", tempDir)

	// Validate with empty state should succeed
	err = validator.Validate(context.Background(), nil)
	if err != nil {
		t.Errorf("Expected no error for empty state, got %v", err)
	}

	// Validate with non-map state should succeed
	err = validator.Validate(context.Background(), "not a map")
	if err != nil {
		t.Errorf("Expected no error for non-map state, got %v", err)
	}
}

func TestFileIntegrityValidator(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-integrity-validator-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test file
	testFile := filepath.Join(tempDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	validator := NewFileIntegrityValidator("test-integrity", testFile, "expected-checksum")

	// Validate should succeed (basic file existence check)
	err = validator.Validate(context.Background(), nil)
	if err != nil {
		t.Errorf("Expected no error for valid file, got %v", err)
	}

	// Test with state containing matching checksum
	state := map[string]interface{}{
		"checksum": "expected-checksum",
	}

	err = validator.Validate(context.Background(), state)
	if err != nil {
		t.Errorf("Expected no error for matching checksum, got %v", err)
	}

	// Test with state containing mismatched checksum
	invalidState := map[string]interface{}{
		"checksum": "wrong-checksum",
	}

	err = validator.Validate(context.Background(), invalidState)
	if err == nil {
		t.Errorf("Expected error for mismatched checksum")
	}
}

func TestFileIntegrityValidator_NonExistentFile(t *testing.T) {
	validator := NewFileIntegrityValidator("test", "/non/existent/file.txt", "checksum")

	err := validator.Validate(context.Background(), nil)
	if err == nil {
		t.Errorf("Expected error for non-existent file")
	}
}

func TestFileIntegrityValidator_GetName(t *testing.T) {
	validator := NewFileIntegrityValidator("test-name", "file.txt", "checksum")

	if validator.GetName() != "test-name" {
		t.Errorf("Expected name 'test-name', got %s", validator.GetName())
	}
}

func TestRecoveryWorkflow_GetSteps(t *testing.T) {
	workflow := NewRecoveryWorkflow("test", "Test workflow")

	action1 := NewTestAction("action1", "First action", false)
	action2 := NewTestAction("action2", "Second action", false)

	workflow.AddStep("step1", action1)
	workflow.AddStep("step2", action2)

	steps := workflow.GetSteps()

	if len(steps) != 2 {
		t.Errorf("Expected 2 steps, got %d", len(steps))
	}

	if steps[0].ID != "step1" {
		t.Errorf("Expected first step ID 'step1', got %s", steps[0].ID)
	}

	if steps[1].ID != "step2" {
		t.Errorf("Expected second step ID 'step2', got %s", steps[1].ID)
	}
}

func TestSyncRecoveryManager_RemoveWorkflow(t *testing.T) {
	manager := NewSyncRecoveryManager()

	// Try to remove non-existent workflow
	err := manager.RemoveWorkflow("non-existent")
	if err == nil {
		t.Errorf("Expected error removing non-existent workflow")
	}

	// Create and execute workflow
	workflow := manager.CreateWorkflow("test", "Test workflow")
	action := NewTestAction("action", "Test action", false)
	workflow.AddStep("step", action)

	err = workflow.Execute(context.Background())
	if err != nil {
		t.Errorf("Expected no error executing workflow, got %v", err)
	}

	// Remove completed workflow should succeed
	err = manager.RemoveWorkflow("test")
	if err != nil {
		t.Errorf("Expected no error removing completed workflow, got %v", err)
	}

	// Create workflow in progress
	workflow2 := manager.CreateWorkflow("in-progress", "In progress workflow")
	workflow2.AddStep("step", NewTestAction("action", "Test action", false))
	
	// Manually set state to in progress (simulate)
	workflow2.mu.Lock()
	workflow2.State = RecoveryStateInProgress
	workflow2.mu.Unlock()

	// Try to remove workflow in progress
	err = manager.RemoveWorkflow("in-progress")
	if err == nil {
		t.Errorf("Expected error removing workflow in progress")
	}
}

func TestSyncRecoveryManager_GetAllWorkflows(t *testing.T) {
	manager := NewSyncRecoveryManager()

	// Create multiple workflows
	workflow1 := manager.CreateWorkflow("workflow1", "Test 1")
	workflow2 := manager.CreateWorkflow("workflow2", "Test 2")

	allWorkflows := manager.GetAllWorkflows()

	if len(allWorkflows) != 2 {
		t.Errorf("Expected 2 workflows, got %d", len(allWorkflows))
	}

	if _, exists := allWorkflows["workflow1"]; !exists {
		t.Errorf("Expected workflow1 to exist")
	}

	if _, exists := allWorkflows["workflow2"]; !exists {
		t.Errorf("Expected workflow2 to exist")
	}

	// Verify workflows are correct
	if allWorkflows["workflow1"].Description != "Test 1" {
		t.Errorf("Expected workflow1 description 'Test 1', got %s", allWorkflows["workflow1"].Description)
	}

	_ = workflow1 // Use variables to avoid unused warnings
	_ = workflow2
}

func TestSyncRecoveryManager_Callbacks(t *testing.T) {
	manager := NewSyncRecoveryManager()

	workflowStarts := make(chan string, 10)
	workflowCompletions := make(chan bool, 10)

	manager.SetWorkflowStartCallback(func(workflow *RecoveryWorkflow) {
		workflowStarts <- workflow.ID
	})

	manager.SetWorkflowCompleteCallback(func(workflow *RecoveryWorkflow, success bool) {
		workflowCompletions <- success
	})

	// Create and execute workflow
	workflow := manager.CreateWorkflow("test", "Test workflow")
	action := NewTestAction("action", "Test action", false)
	workflow.AddStep("step", action)

	// Check start callback
	select {
	case workflowID := <-workflowStarts:
		if workflowID != "test" {
			t.Errorf("Expected workflow start for 'test', got %s", workflowID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Errorf("Expected workflow start callback")
	}

	// Execute workflow
	err := workflow.Execute(context.Background())
	if err != nil {
		t.Errorf("Expected no error executing workflow, got %v", err)
	}

	// Check completion callback
	select {
	case success := <-workflowCompletions:
		if !success {
			t.Errorf("Expected workflow to succeed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Errorf("Expected workflow completion callback")
	}
}
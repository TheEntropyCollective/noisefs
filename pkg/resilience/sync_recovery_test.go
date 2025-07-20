package resilience

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestAction is a simple test implementation of RecoveryAction
type TestAction struct {
	ID          string
	Description string
	ShouldFail  bool
	ExecuteCalled bool
	RollbackCalled bool
}

func NewTestAction(id, description string, shouldFail bool) *TestAction {
	return &TestAction{
		ID:          id,
		Description: description,
		ShouldFail:  shouldFail,
	}
}

func (ta *TestAction) Execute(ctx context.Context) error {
	ta.ExecuteCalled = true
	if ta.ShouldFail {
		return errors.New("test action failed")
	}
	return nil
}

func (ta *TestAction) Rollback(ctx context.Context) error {
	ta.RollbackCalled = true
	return nil
}

func (ta *TestAction) GetID() string {
	return ta.ID
}

func (ta *TestAction) GetDescription() string {
	return ta.Description
}

func TestRecoveryWorkflow_BasicExecution(t *testing.T) {
	workflow := NewRecoveryWorkflow("test-workflow", "Test workflow")

	// Add successful actions
	action1 := NewTestAction("action1", "First action", false)
	action2 := NewTestAction("action2", "Second action", false)

	workflow.AddStep("step1", action1)
	workflow.AddStep("step2", action2)

	// Execute workflow
	err := workflow.Execute(context.Background())
	if err != nil {
		t.Errorf("Expected no error for successful workflow, got %v", err)
	}

	// Check workflow state
	if workflow.GetState() != RecoveryStateCompleted {
		t.Errorf("Expected workflow state to be Completed, got %v", workflow.GetState())
	}

	// Check actions were executed
	if !action1.ExecuteCalled {
		t.Errorf("Expected action1 to be executed")
	}

	if !action2.ExecuteCalled {
		t.Errorf("Expected action2 to be executed")
	}

	// Check actions were not rolled back
	if action1.RollbackCalled {
		t.Errorf("Expected action1 not to be rolled back")
	}

	if action2.RollbackCalled {
		t.Errorf("Expected action2 not to be rolled back")
	}
}

func TestRecoveryWorkflow_FailureAndRollback(t *testing.T) {
	workflow := NewRecoveryWorkflow("test-workflow", "Test workflow")

	// Add actions where the second one fails
	action1 := NewTestAction("action1", "First action", false)
	action2 := NewTestAction("action2", "Second action", true) // This will fail
	action3 := NewTestAction("action3", "Third action", false)

	workflow.AddStep("step1", action1)
	workflow.AddStep("step2", action2)
	workflow.AddStep("step3", action3)

	// Execute workflow
	err := workflow.Execute(context.Background())
	if err == nil {
		t.Errorf("Expected error for failing workflow")
	}

	// Check workflow state
	if workflow.GetState() != RecoveryStateRolledBack {
		t.Errorf("Expected workflow state to be RolledBack, got %v", workflow.GetState())
	}

	// Check first action was executed and rolled back
	if !action1.ExecuteCalled {
		t.Errorf("Expected action1 to be executed")
	}

	if !action1.RollbackCalled {
		t.Errorf("Expected action1 to be rolled back")
	}

	// Check second action was executed but failed
	if !action2.ExecuteCalled {
		t.Errorf("Expected action2 to be executed")
	}

	// Check third action was not executed
	if action3.ExecuteCalled {
		t.Errorf("Expected action3 not to be executed")
	}
}

func TestRecoveryWorkflow_ManualRollback(t *testing.T) {
	workflow := NewRecoveryWorkflow("test-workflow", "Test workflow")

	action1 := NewTestAction("action1", "First action", false)
	action2 := NewTestAction("action2", "Second action", false)

	workflow.AddStep("step1", action1)
	workflow.AddStep("step2", action2)

	// Execute workflow successfully
	err := workflow.Execute(context.Background())
	if err != nil {
		t.Errorf("Expected no error for successful workflow, got %v", err)
	}

	// Manually rollback
	err = workflow.Rollback(context.Background())
	if err != nil {
		t.Errorf("Expected no error for manual rollback, got %v", err)
	}

	// Check actions were rolled back
	if !action1.RollbackCalled {
		t.Errorf("Expected action1 to be rolled back")
	}

	if !action2.RollbackCalled {
		t.Errorf("Expected action2 to be rolled back")
	}
}

func TestRecoveryWorkflow_StepCallbacks(t *testing.T) {
	workflow := NewRecoveryWorkflow("test-workflow", "Test workflow")

	stepCompletions := make(chan string, 10)
	workflow.SetStepCompleteCallback(func(step *RecoveryStep) {
		stepCompletions <- step.ID
	})

	workflowCompletions := make(chan bool, 1)
	workflow.SetWorkflowCompleteCallback(func(w *RecoveryWorkflow, success bool) {
		workflowCompletions <- success
	})

	action1 := NewTestAction("action1", "First action", false)
	workflow.AddStep("step1", action1)

	// Execute workflow
	err := workflow.Execute(context.Background())
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Check step completion callback
	select {
	case stepID := <-stepCompletions:
		if stepID != "step1" {
			t.Errorf("Expected step completion for 'step1', got %s", stepID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Errorf("Expected step completion callback")
	}

	// Check workflow completion callback
	select {
	case success := <-workflowCompletions:
		if !success {
			t.Errorf("Expected workflow to succeed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Errorf("Expected workflow completion callback")
	}
}

func TestSyncRecoveryManager_BasicOperations(t *testing.T) {
	manager := NewSyncRecoveryManager()

	// Create workflow
	workflow := manager.CreateWorkflow("test-workflow", "Test workflow")
	if workflow == nil {
		t.Errorf("Expected workflow to be created")
	}

	// Get workflow
	retrieved, exists := manager.GetWorkflow("test-workflow")
	if !exists {
		t.Errorf("Expected workflow to exist")
	}

	if retrieved.ID != "test-workflow" {
		t.Errorf("Expected workflow ID 'test-workflow', got %s", retrieved.ID)
	}

	// Check statistics
	stats := manager.GetStatistics()
	if stats["total_workflows"] != 1 {
		t.Errorf("Expected 1 total workflow, got %d", stats["total_workflows"])
	}

	if stats["active_workflows"] != 1 {
		t.Errorf("Expected 1 active workflow, got %d", stats["active_workflows"])
	}
}

func TestSyncRecoveryManager_WorkflowExecution(t *testing.T) {
	manager := NewSyncRecoveryManager()

	// Create workflow
	workflow := manager.CreateWorkflow("test-workflow", "Test workflow")
	action := NewTestAction("action1", "Test action", false)
	workflow.AddStep("step1", action)

	// Execute workflow
	err := workflow.Execute(context.Background())
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Give callback time to execute
	time.Sleep(10 * time.Millisecond)

	// Check statistics after completion
	stats := manager.GetStatistics()
	if stats["successful_workflows"] != 1 {
		t.Errorf("Expected 1 successful workflow, got %d", stats["successful_workflows"])
	}
}

func TestSyncRecoveryManager_StateValidation(t *testing.T) {
	manager := NewSyncRecoveryManager()

	// Add a validator that always passes
	validator := &TestStateValidator{
		Name:        "test-validator",
		ShouldFail:  false,
	}
	manager.AddStateValidator("test", validator)

	// Validate state
	err := manager.ValidateState(context.Background(), map[string]interface{}{"test": "data"})
	if err != nil {
		t.Errorf("Expected no error for valid state, got %v", err)
	}

	// Add a validator that fails
	failingValidator := &TestStateValidator{
		Name:       "failing-validator",
		ShouldFail: true,
	}
	manager.AddStateValidator("failing", failingValidator)

	// Validate state again - should fail
	err = manager.ValidateState(context.Background(), map[string]interface{}{"test": "data"})
	if err == nil {
		t.Errorf("Expected error for invalid state")
	}
}

func TestSyncRecoveryManager_CleanupCompletedWorkflows(t *testing.T) {
	manager := NewSyncRecoveryManager()

	// Create multiple workflows
	workflow1 := manager.CreateWorkflow("workflow1", "Test workflow 1")
	workflow2 := manager.CreateWorkflow("workflow2", "Test workflow 2")
	workflow3 := manager.CreateWorkflow("workflow3", "Test workflow 3")

	// Execute first workflow successfully
	action1 := NewTestAction("action1", "Test action", false)
	workflow1.AddStep("step1", action1)
	workflow1.Execute(context.Background())

	// Execute second workflow with failure
	action2 := NewTestAction("action2", "Test action", true)
	workflow2.AddStep("step1", action2)
	workflow2.Execute(context.Background())

	// Leave third workflow in progress (don't execute)
	_ = workflow3 // Use variable to avoid unused warning

	// Cleanup completed workflows
	cleaned := manager.CleanupCompletedWorkflows()
	if cleaned != 2 {
		t.Errorf("Expected 2 workflows to be cleaned up, got %d", cleaned)
	}

	// Check remaining workflows
	remaining := manager.GetAllWorkflows()
	if len(remaining) != 1 {
		t.Errorf("Expected 1 remaining workflow, got %d", len(remaining))
	}

	if _, exists := remaining["workflow3"]; !exists {
		t.Errorf("Expected workflow3 to remain")
	}
}

// TestStateValidator is a test implementation of StateValidator
type TestStateValidator struct {
	Name       string
	ShouldFail bool
}

func (tsv *TestStateValidator) Validate(ctx context.Context, state interface{}) error {
	if tsv.ShouldFail {
		return errors.New("validation failed")
	}
	return nil
}

func (tsv *TestStateValidator) GetName() string {
	return tsv.Name
}

func TestFileBackupAction(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "test-backup-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test file
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := "test content"
	err = os.WriteFile(testFile, []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create backup action
	action := NewFileBackupAction("test-backup", testFile)

	// Execute backup
	err = action.Execute(context.Background())
	if err != nil {
		t.Errorf("Expected no error creating backup, got %v", err)
	}

	// Check backup exists
	if _, err := os.Stat(action.BackupPath); os.IsNotExist(err) {
		t.Errorf("Expected backup file to exist at %s", action.BackupPath)
	}

	// Check backup content
	backupContent, err := os.ReadFile(action.BackupPath)
	if err != nil {
		t.Errorf("Failed to read backup file: %v", err)
	}

	if string(backupContent) != testContent {
		t.Errorf("Expected backup content '%s', got '%s'", testContent, string(backupContent))
	}

	// Rollback (remove backup)
	err = action.Rollback(context.Background())
	if err != nil {
		t.Errorf("Expected no error rolling back backup, got %v", err)
	}

	// Check backup is removed
	if _, err := os.Stat(action.BackupPath); !os.IsNotExist(err) {
		t.Errorf("Expected backup file to be removed")
	}
}

func TestFileRestoreAction(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "test-restore-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a backup file
	backupFile := filepath.Join(tempDir, "backup.txt")
	backupContent := "backup content"
	err = os.WriteFile(backupFile, []byte(backupContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create backup file: %v", err)
	}

	// Create restore action
	targetFile := filepath.Join(tempDir, "restored.txt")
	action := NewFileRestoreAction("test-restore", targetFile, backupFile)

	// Execute restore
	err = action.Execute(context.Background())
	if err != nil {
		t.Errorf("Expected no error restoring file, got %v", err)
	}

	// Check restored file exists
	if _, err := os.Stat(targetFile); os.IsNotExist(err) {
		t.Errorf("Expected restored file to exist at %s", targetFile)
	}

	// Check restored content
	restoredContent, err := os.ReadFile(targetFile)
	if err != nil {
		t.Errorf("Failed to read restored file: %v", err)
	}

	if string(restoredContent) != backupContent {
		t.Errorf("Expected restored content '%s', got '%s'", backupContent, string(restoredContent))
	}

	// Rollback (remove restored file)
	err = action.Rollback(context.Background())
	if err != nil {
		t.Errorf("Expected no error rolling back restore, got %v", err)
	}

	// Check restored file is removed
	if _, err := os.Stat(targetFile); !os.IsNotExist(err) {
		t.Errorf("Expected restored file to be removed")
	}
}

func TestStateSnapshotAction(t *testing.T) {
	// Create state snapshot action
	restoredState := ""

	captureFunc := func(ctx context.Context) (interface{}, error) {
		return "captured state", nil
	}

	restoreFunc := func(ctx context.Context, state interface{}) error {
		if s, ok := state.(string); ok {
			restoredState = s
		}
		return nil
	}

	action := NewStateSnapshotAction("test-snapshot", "test-state", captureFunc, restoreFunc)

	// Execute (capture state)
	err := action.Execute(context.Background())
	if err != nil {
		t.Errorf("Expected no error capturing state, got %v", err)
	}

	// Check state was captured
	if action.StateData != "captured state" {
		t.Errorf("Expected captured state 'captured state', got %v", action.StateData)
	}

	// Rollback (restore state)
	err = action.Rollback(context.Background())
	if err != nil {
		t.Errorf("Expected no error restoring state, got %v", err)
	}

	// Check state was restored
	if restoredState != "captured state" {
		t.Errorf("Expected restored state 'captured state', got %s", restoredState)
	}
}

func TestRecoveryState_String(t *testing.T) {
	tests := []struct {
		state    RecoveryState
		expected string
	}{
		{RecoveryStateIdle, "Idle"},
		{RecoveryStateInProgress, "InProgress"},
		{RecoveryStateCompleted, "Completed"},
		{RecoveryStateFailed, "Failed"},
		{RecoveryStateRolledBack, "RolledBack"},
	}

	for _, test := range tests {
		if test.state.String() != test.expected {
			t.Errorf("Expected %s for state %d, got %s", test.expected, test.state, test.state.String())
		}
	}
}

func TestDirectoryStateValidator(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "test-dir-validator-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create some test files
	testFile1 := filepath.Join(tempDir, "file1.txt")
	testFile2 := filepath.Join(tempDir, "file2.txt")
	
	os.WriteFile(testFile1, []byte("content1"), 0644)
	os.WriteFile(testFile2, []byte("content2"), 0644)

	validator := NewDirectoryStateValidator("test-dir", tempDir)

	// Test with valid state
	state := map[string]interface{}{
		"files": []string{"file1.txt", "file2.txt"},
	}

	err = validator.Validate(context.Background(), state)
	if err != nil {
		t.Errorf("Expected no error for valid directory state, got %v", err)
	}

	// Test with invalid state (missing file)
	invalidState := map[string]interface{}{
		"files": []string{"file1.txt", "missing.txt"},
	}

	err = validator.Validate(context.Background(), invalidState)
	if err == nil {
		t.Errorf("Expected error for missing file")
	}
}
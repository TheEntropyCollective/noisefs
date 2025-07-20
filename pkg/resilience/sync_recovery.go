package resilience

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RecoveryState represents the state of a recovery operation
type RecoveryState int

const (
	RecoveryStateIdle RecoveryState = iota
	RecoveryStateInProgress
	RecoveryStateCompleted
	RecoveryStateFailed
	RecoveryStateRolledBack
)

// String returns the string representation of RecoveryState
func (rs RecoveryState) String() string {
	switch rs {
	case RecoveryStateIdle:
		return "Idle"
	case RecoveryStateInProgress:
		return "InProgress"
	case RecoveryStateCompleted:
		return "Completed"
	case RecoveryStateFailed:
		return "Failed"
	case RecoveryStateRolledBack:
		return "RolledBack"
	default:
		return "Unknown"
	}
}

// RecoveryAction represents an action that can be executed and rolled back
type RecoveryAction interface {
	// Execute performs the action
	Execute(ctx context.Context) error
	// Rollback reverses the action
	Rollback(ctx context.Context) error
	// GetID returns a unique identifier for this action
	GetID() string
	// GetDescription returns a human-readable description
	GetDescription() string
}

// RecoveryStep represents a single step in a recovery workflow
type RecoveryStep struct {
	ID          string
	Action      RecoveryAction
	State       RecoveryState
	StartTime   time.Time
	EndTime     time.Time
	Error       error
	mu          sync.RWMutex
}

// RecoveryWorkflow represents a series of recovery steps that can be executed as a transaction
type RecoveryWorkflow struct {
	ID           string
	Description  string
	Steps        []*RecoveryStep
	State        RecoveryState
	StartTime    time.Time
	EndTime      time.Time
	Error        error
	mu           sync.RWMutex
	
	// Callbacks
	onStepComplete func(step *RecoveryStep)
	onWorkflowComplete func(workflow *RecoveryWorkflow, success bool)
}

// NewRecoveryWorkflow creates a new recovery workflow
func NewRecoveryWorkflow(id, description string) *RecoveryWorkflow {
	return &RecoveryWorkflow{
		ID:          id,
		Description: description,
		Steps:       make([]*RecoveryStep, 0),
		State:       RecoveryStateIdle,
	}
}

// AddStep adds a recovery step to the workflow
func (rw *RecoveryWorkflow) AddStep(id string, action RecoveryAction) {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	step := &RecoveryStep{
		ID:     id,
		Action: action,
		State:  RecoveryStateIdle,
	}

	rw.Steps = append(rw.Steps, step)
}

// Execute executes all steps in the workflow
func (rw *RecoveryWorkflow) Execute(ctx context.Context) error {
	rw.mu.Lock()
	rw.State = RecoveryStateInProgress
	rw.StartTime = time.Now()
	rw.mu.Unlock()

	var executedSteps []*RecoveryStep

	// Execute each step
	for _, step := range rw.Steps {
		step.mu.Lock()
		step.State = RecoveryStateInProgress
		step.StartTime = time.Now()
		step.mu.Unlock()

		err := step.Action.Execute(ctx)

		step.mu.Lock()
		step.EndTime = time.Now()
		if err != nil {
			step.State = RecoveryStateFailed
			step.Error = err
		} else {
			step.State = RecoveryStateCompleted
			executedSteps = append(executedSteps, step)
		}
		step.mu.Unlock()

		// Call step completion callback
		if rw.onStepComplete != nil {
			go rw.onStepComplete(step)
		}

		if err != nil {
			// Rollback executed steps in reverse order
			rw.rollbackSteps(ctx, executedSteps)
			
			rw.mu.Lock()
			rw.State = RecoveryStateRolledBack
			rw.EndTime = time.Now()
			rw.Error = err
			rw.mu.Unlock()

			if rw.onWorkflowComplete != nil {
				go rw.onWorkflowComplete(rw, false)
			}

			return fmt.Errorf("workflow step '%s' failed: %w", step.ID, err)
		}

		// Check for context cancellation
		if ctx.Err() != nil {
			rw.rollbackSteps(ctx, executedSteps)
			
			rw.mu.Lock()
			rw.State = RecoveryStateRolledBack
			rw.EndTime = time.Now()
			rw.Error = ctx.Err()
			rw.mu.Unlock()

			if rw.onWorkflowComplete != nil {
				go rw.onWorkflowComplete(rw, false)
			}

			return ctx.Err()
		}
	}

	rw.mu.Lock()
	rw.State = RecoveryStateCompleted
	rw.EndTime = time.Now()
	rw.mu.Unlock()

	if rw.onWorkflowComplete != nil {
		go rw.onWorkflowComplete(rw, true)
	}

	return nil
}

// Rollback rolls back all completed steps in reverse order
func (rw *RecoveryWorkflow) Rollback(ctx context.Context) error {
	rw.mu.RLock()
	steps := make([]*RecoveryStep, len(rw.Steps))
	copy(steps, rw.Steps)
	rw.mu.RUnlock()

	var completedSteps []*RecoveryStep
	for _, step := range steps {
		step.mu.RLock()
		if step.State == RecoveryStateCompleted {
			completedSteps = append(completedSteps, step)
		}
		step.mu.RUnlock()
	}

	return rw.rollbackSteps(ctx, completedSteps)
}

// rollbackSteps rolls back the given steps in reverse order
func (rw *RecoveryWorkflow) rollbackSteps(ctx context.Context, steps []*RecoveryStep) error {
	var lastErr error

	// Rollback in reverse order
	for i := len(steps) - 1; i >= 0; i-- {
		step := steps[i]
		
		step.mu.Lock()
		rollbackStartTime := time.Now()
		step.mu.Unlock()

		err := step.Action.Rollback(ctx)

		step.mu.Lock()
		if err != nil {
			step.Error = err
			lastErr = err
		} else {
			step.State = RecoveryStateRolledBack
		}
		step.mu.Unlock()

		// Call step completion callback for rollback
		if rw.onStepComplete != nil {
			go rw.onStepComplete(step)
		}

		// Continue rolling back even if one fails
		_ = rollbackStartTime // Use the variable to avoid unused warning
	}

	return lastErr
}

// GetState returns the current state of the workflow
func (rw *RecoveryWorkflow) GetState() RecoveryState {
	rw.mu.RLock()
	defer rw.mu.RUnlock()
	return rw.State
}

// GetSteps returns a copy of all steps
func (rw *RecoveryWorkflow) GetSteps() []*RecoveryStep {
	rw.mu.RLock()
	defer rw.mu.RUnlock()

	steps := make([]*RecoveryStep, len(rw.Steps))
	for i, step := range rw.Steps {
		step.mu.RLock()
		steps[i] = &RecoveryStep{
			ID:        step.ID,
			Action:    step.Action,
			State:     step.State,
			StartTime: step.StartTime,
			EndTime:   step.EndTime,
			Error:     step.Error,
		}
		step.mu.RUnlock()
	}

	return steps
}

// SetStepCompleteCallback sets a callback for step completion
func (rw *RecoveryWorkflow) SetStepCompleteCallback(callback func(step *RecoveryStep)) {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	rw.onStepComplete = callback
}

// SetWorkflowCompleteCallback sets a callback for workflow completion
func (rw *RecoveryWorkflow) SetWorkflowCompleteCallback(callback func(workflow *RecoveryWorkflow, success bool)) {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	rw.onWorkflowComplete = callback
}

// SyncRecoveryManager manages recovery workflows and state validation
type SyncRecoveryManager struct {
	workflows map[string]*RecoveryWorkflow
	mu        sync.RWMutex
	
	// State validation
	stateValidators map[string]StateValidator
	
	// Statistics
	totalWorkflows    int64
	successfulWorkflows int64
	failedWorkflows   int64
	rolledBackWorkflows int64
	
	// Callbacks
	onWorkflowStart    func(workflow *RecoveryWorkflow)
	onWorkflowComplete func(workflow *RecoveryWorkflow, success bool)
}

// StateValidator represents a validator for checking state consistency
type StateValidator interface {
	// Validate checks if the state is consistent
	Validate(ctx context.Context, state interface{}) error
	// GetName returns the name of this validator
	GetName() string
}

// NewSyncRecoveryManager creates a new sync recovery manager
func NewSyncRecoveryManager() *SyncRecoveryManager {
	return &SyncRecoveryManager{
		workflows:       make(map[string]*RecoveryWorkflow),
		stateValidators: make(map[string]StateValidator),
	}
}

// CreateWorkflow creates a new recovery workflow
func (srm *SyncRecoveryManager) CreateWorkflow(id, description string) *RecoveryWorkflow {
	srm.mu.Lock()
	defer srm.mu.Unlock()

	workflow := NewRecoveryWorkflow(id, description)
	
	// Set up callbacks to track statistics
	workflow.SetWorkflowCompleteCallback(func(w *RecoveryWorkflow, success bool) {
		srm.updateStatistics(w, success)
		if srm.onWorkflowComplete != nil {
			srm.onWorkflowComplete(w, success)
		}
	})

	srm.workflows[id] = workflow
	srm.totalWorkflows++

	if srm.onWorkflowStart != nil {
		go srm.onWorkflowStart(workflow)
	}

	return workflow
}

// GetWorkflow returns a workflow by ID
func (srm *SyncRecoveryManager) GetWorkflow(id string) (*RecoveryWorkflow, bool) {
	srm.mu.RLock()
	defer srm.mu.RUnlock()

	workflow, exists := srm.workflows[id]
	return workflow, exists
}

// RemoveWorkflow removes a completed workflow
func (srm *SyncRecoveryManager) RemoveWorkflow(id string) error {
	srm.mu.Lock()
	defer srm.mu.Unlock()

	workflow, exists := srm.workflows[id]
	if !exists {
		return fmt.Errorf("workflow '%s' not found", id)
	}

	workflow.mu.RLock()
	state := workflow.State
	workflow.mu.RUnlock()

	if state == RecoveryStateInProgress {
		return fmt.Errorf("cannot remove workflow '%s' in progress", id)
	}

	delete(srm.workflows, id)
	return nil
}

// AddStateValidator adds a state validator
func (srm *SyncRecoveryManager) AddStateValidator(name string, validator StateValidator) {
	srm.mu.Lock()
	defer srm.mu.Unlock()
	srm.stateValidators[name] = validator
}

// ValidateState validates state using all registered validators
func (srm *SyncRecoveryManager) ValidateState(ctx context.Context, state interface{}) error {
	srm.mu.RLock()
	validators := make(map[string]StateValidator)
	for name, validator := range srm.stateValidators {
		validators[name] = validator
	}
	srm.mu.RUnlock()

	for name, validator := range validators {
		if err := validator.Validate(ctx, state); err != nil {
			return fmt.Errorf("state validation failed for '%s': %w", name, err)
		}
	}

	return nil
}

// GetAllWorkflows returns all workflows
func (srm *SyncRecoveryManager) GetAllWorkflows() map[string]*RecoveryWorkflow {
	srm.mu.RLock()
	defer srm.mu.RUnlock()

	result := make(map[string]*RecoveryWorkflow)
	for id, workflow := range srm.workflows {
		result[id] = workflow
	}

	return result
}

// GetStatistics returns recovery statistics
func (srm *SyncRecoveryManager) GetStatistics() map[string]int64 {
	srm.mu.RLock()
	defer srm.mu.RUnlock()

	return map[string]int64{
		"total_workflows":     srm.totalWorkflows,
		"successful_workflows": srm.successfulWorkflows,
		"failed_workflows":    srm.failedWorkflows,
		"rolled_back_workflows": srm.rolledBackWorkflows,
		"active_workflows":    int64(len(srm.workflows)),
	}
}

// SetWorkflowStartCallback sets a callback for workflow start
func (srm *SyncRecoveryManager) SetWorkflowStartCallback(callback func(workflow *RecoveryWorkflow)) {
	srm.mu.Lock()
	defer srm.mu.Unlock()
	srm.onWorkflowStart = callback
}

// SetWorkflowCompleteCallback sets a callback for workflow completion
func (srm *SyncRecoveryManager) SetWorkflowCompleteCallback(callback func(workflow *RecoveryWorkflow, success bool)) {
	srm.mu.Lock()
	defer srm.mu.Unlock()
	srm.onWorkflowComplete = callback
}

// updateStatistics updates workflow statistics
func (srm *SyncRecoveryManager) updateStatistics(workflow *RecoveryWorkflow, success bool) {
	srm.mu.Lock()
	defer srm.mu.Unlock()

	if success {
		srm.successfulWorkflows++
	} else {
		workflow.mu.RLock()
		state := workflow.State
		workflow.mu.RUnlock()

		if state == RecoveryStateRolledBack {
			srm.rolledBackWorkflows++
		} else {
			srm.failedWorkflows++
		}
	}
}

// CleanupCompletedWorkflows removes all completed workflows
func (srm *SyncRecoveryManager) CleanupCompletedWorkflows() int {
	srm.mu.Lock()
	defer srm.mu.Unlock()

	cleaned := 0
	for id, workflow := range srm.workflows {
		workflow.mu.RLock()
		state := workflow.State
		workflow.mu.RUnlock()

		if state == RecoveryStateCompleted || state == RecoveryStateFailed || state == RecoveryStateRolledBack {
			delete(srm.workflows, id)
			cleaned++
		}
	}

	return cleaned
}
package sync

import (
	"testing"
	"time"
)

func TestSyncEngine_Basic(t *testing.T) {
	// This test is skipped because it requires complex mocks
	// The SyncEngine depends on FileWatcher, RemoteChangeMonitor, and DirectoryManager
	t.Skip("Integration test requires full component setup")
}

func TestSyncSession_StatusManagement(t *testing.T) {
	now := time.Now()
	session := &SyncSession{
		SyncID:     "test-sync-1",
		LocalPath:  "/local/test",
		RemotePath: "/remote/test",
		LastSync:   now,
		Status:     StatusIdle,
		Progress: &SyncProgress{
			TotalOperations:     10,
			CompletedOperations: 5,
			FailedOperations:    1,
			CurrentOperation:    "uploading file.txt",
			StartTime:           now,
			EstimatedCompletion: 30 * time.Second,
		},
	}

	// Test initial state
	if session.Status != StatusIdle {
		t.Errorf("Expected status %s, got %s", StatusIdle, session.Status)
	}

	// Test status changes
	session.Status = StatusSyncing
	if session.Status != StatusSyncing {
		t.Errorf("Expected status %s, got %s", StatusSyncing, session.Status)
	}

	session.Status = StatusConflict
	if session.Status != StatusConflict {
		t.Errorf("Expected status %s, got %s", StatusConflict, session.Status)
	}

	session.Status = StatusError
	if session.Status != StatusError {
		t.Errorf("Expected status %s, got %s", StatusError, session.Status)
	}

	session.Status = StatusPaused
	if session.Status != StatusPaused {
		t.Errorf("Expected status %s, got %s", StatusPaused, session.Status)
	}
}

func TestSyncProgress_Tracking(t *testing.T) {
	now := time.Now()
	progress := &SyncProgress{
		TotalOperations:     100,
		CompletedOperations: 25,
		FailedOperations:    5,
		CurrentOperation:    "syncing directory",
		StartTime:           now,
		EstimatedCompletion: 2 * time.Minute,
	}

	// Test progress calculations
	completionRate := float64(progress.CompletedOperations) / float64(progress.TotalOperations)
	expectedRate := 0.25
	if completionRate != expectedRate {
		t.Errorf("Expected completion rate %f, got %f", expectedRate, completionRate)
	}

	failureRate := float64(progress.FailedOperations) / float64(progress.TotalOperations)
	expectedFailureRate := 0.05
	if failureRate != expectedFailureRate {
		t.Errorf("Expected failure rate %f, got %f", expectedFailureRate, failureRate)
	}

	// Test progress updates
	progress.CompletedOperations = 50
	progress.FailedOperations = 2
	progress.CurrentOperation = "uploading large file"

	newCompletionRate := float64(progress.CompletedOperations) / float64(progress.TotalOperations)
	if newCompletionRate != 0.5 {
		t.Errorf("Expected updated completion rate 0.5, got %f", newCompletionRate)
	}
}

func TestSyncEngineStats_Tracking(t *testing.T) {
	stats := &SyncEngineStats{
		ActiveSessions:     3,
		TotalSyncEvents:    1500,
		TotalConflicts:     25,
		TotalErrors:        10,
		AverageConflictAge: 5 * time.Minute,
		LastSyncTime:       time.Now(),
	}

	// Test stats validation
	if stats.ActiveSessions != 3 {
		t.Errorf("Expected 3 active sessions, got %d", stats.ActiveSessions)
	}

	if stats.TotalSyncEvents != 1500 {
		t.Errorf("Expected 1500 total sync events, got %d", stats.TotalSyncEvents)
	}

	if stats.TotalConflicts != 25 {
		t.Errorf("Expected 25 total conflicts, got %d", stats.TotalConflicts)
	}

	if stats.TotalErrors != 10 {
		t.Errorf("Expected 10 total errors, got %d", stats.TotalErrors)
	}

	// Test conflict rate calculation
	conflictRate := float64(stats.TotalConflicts) / float64(stats.TotalSyncEvents)
	expectedConflictRate := 25.0 / 1500.0
	if conflictRate != expectedConflictRate {
		t.Errorf("Expected conflict rate %f, got %f", expectedConflictRate, conflictRate)
	}

	// Test error rate calculation
	errorRate := float64(stats.TotalErrors) / float64(stats.TotalSyncEvents)
	expectedErrorRate := 10.0 / 1500.0
	if errorRate != expectedErrorRate {
		t.Errorf("Expected error rate %f, got %f", expectedErrorRate, errorRate)
	}
}

func TestSyncEngine_CreateSyncOperation(t *testing.T) {
	engine := &SyncEngine{
		config: &SyncConfig{
			MaxRetries: 3,
		},
	}

	session := &SyncSession{
		SyncID:     "test-sync",
		LocalPath:  "/local/test",
		RemotePath: "/remote/test",
	}

	// Test file creation event from local
	event := SyncEvent{
		Type:      EventTypeFileCreated,
		Path:      "/local/test/file.txt",
		Timestamp: time.Now(),
	}

	op := engine.createSyncOperation(session, event, true)
	if op == nil {
		t.Fatal("Expected sync operation to be created")
	}

	if op.Type != OpTypeUpload {
		t.Errorf("Expected operation type %s, got %s", OpTypeUpload, op.Type)
	}

	if op.LocalPath != "/local/test/file.txt" {
		t.Errorf("Expected local path '/local/test/file.txt', got '%s'", op.LocalPath)
	}

	if op.RemotePath != "/remote/test/file.txt" {
		t.Errorf("Expected remote path '/remote/test/file.txt', got '%s'", op.RemotePath)
	}

	if op.Status != OpStatusPending {
		t.Errorf("Expected status %s, got %s", OpStatusPending, op.Status)
	}

	// Test file creation event from remote
	remoteEvent := SyncEvent{
		Type:      EventTypeFileCreated,
		Path:      "/remote/test/file2.txt",
		Timestamp: time.Now(),
	}

	op2 := engine.createSyncOperation(session, remoteEvent, false)
	if op2 == nil {
		t.Fatal("Expected sync operation to be created")
	}

	if op2.Type != OpTypeDownload {
		t.Errorf("Expected operation type %s, got %s", OpTypeDownload, op2.Type)
	}

	if op2.LocalPath != "/local/test/file2.txt" {
		t.Errorf("Expected local path '/local/test/file2.txt', got '%s'", op2.LocalPath)
	}

	if op2.RemotePath != "/remote/test/file2.txt" {
		t.Errorf("Expected remote path '/remote/test/file2.txt', got '%s'", op2.RemotePath)
	}

	// Test directory creation event
	dirEvent := SyncEvent{
		Type:      EventTypeDirCreated,
		Path:      "/local/test/subdir",
		Timestamp: time.Now(),
	}

	op3 := engine.createSyncOperation(session, dirEvent, true)
	if op3 == nil {
		t.Fatal("Expected sync operation to be created")
	}

	if op3.Type != OpTypeCreateDir {
		t.Errorf("Expected operation type %s, got %s", OpTypeCreateDir, op3.Type)
	}

	// Test deletion event
	deleteEvent := SyncEvent{
		Type:      EventTypeFileDeleted,
		Path:      "/local/test/deleted.txt",
		Timestamp: time.Now(),
	}

	op4 := engine.createSyncOperation(session, deleteEvent, true)
	if op4 == nil {
		t.Fatal("Expected sync operation to be created")
	}

	if op4.Type != OpTypeDelete {
		t.Errorf("Expected operation type %s, got %s", OpTypeDelete, op4.Type)
	}
}

func TestSyncEngine_FindAffectedSessions(t *testing.T) {
	engine := &SyncEngine{
		activeSyncs: map[string]*SyncSession{
			"sync1": {
				SyncID:     "sync1",
				LocalPath:  "/local/project1",
				RemotePath: "/remote/project1",
			},
			"sync2": {
				SyncID:     "sync2",
				LocalPath:  "/local/project2",
				RemotePath: "/remote/project2",
			},
			"sync3": {
				SyncID:     "sync3",
				LocalPath:  "/local/shared",
				RemotePath: "/remote/shared",
			},
		},
	}

	// Test finding sessions for local path
	sessions := engine.findAffectedSessions("/local/project1/file.txt", true)
	if len(sessions) != 1 {
		t.Errorf("Expected 1 affected session, got %d", len(sessions))
	}
	if sessions[0].SyncID != "sync1" {
		t.Errorf("Expected sync1, got %s", sessions[0].SyncID)
	}

	// Test finding sessions for remote path
	sessions = engine.findAffectedSessions("/remote/project2/subdir/file.txt", false)
	if len(sessions) != 1 {
		t.Errorf("Expected 1 affected session, got %d", len(sessions))
	}
	if sessions[0].SyncID != "sync2" {
		t.Errorf("Expected sync2, got %s", sessions[0].SyncID)
	}

	// Test path that doesn't match any session
	sessions = engine.findAffectedSessions("/local/untracked/file.txt", true)
	if len(sessions) != 0 {
		t.Errorf("Expected 0 affected sessions, got %d", len(sessions))
	}

	// Test exact path match
	sessions = engine.findAffectedSessions("/local/shared", true)
	if len(sessions) != 1 {
		t.Errorf("Expected 1 affected session, got %d", len(sessions))
	}
	if sessions[0].SyncID != "sync3" {
		t.Errorf("Expected sync3, got %s", sessions[0].SyncID)
	}
}

func TestSyncEngine_PathCalculations(t *testing.T) {
	engine := &SyncEngine{}

	session := &SyncSession{
		SyncID:     "test-sync",
		LocalPath:  "/local/project",
		RemotePath: "/remote/project",
	}

	// Test local event path calculation
	event := SyncEvent{
		Type: EventTypeFileCreated,
		Path: "/local/project/subdir/file.txt",
	}

	op := engine.createSyncOperation(session, event, true)
	if op == nil {
		t.Fatal("Expected sync operation to be created")
	}

	expectedRemotePath := "/remote/project/subdir/file.txt"
	if op.RemotePath != expectedRemotePath {
		t.Errorf("Expected remote path '%s', got '%s'", expectedRemotePath, op.RemotePath)
	}

	// Test remote event path calculation
	remoteEvent := SyncEvent{
		Type: EventTypeFileCreated,
		Path: "/remote/project/docs/readme.txt",
	}

	op2 := engine.createSyncOperation(session, remoteEvent, false)
	if op2 == nil {
		t.Fatal("Expected sync operation to be created")
	}

	expectedLocalPath := "/local/project/docs/readme.txt"
	if op2.LocalPath != expectedLocalPath {
		t.Errorf("Expected local path '%s', got '%s'", expectedLocalPath, op2.LocalPath)
	}

	// Test event path that doesn't match session path
	unmatchedEvent := SyncEvent{
		Type: EventTypeFileCreated,
		Path: "/different/path/file.txt",
	}

	op3 := engine.createSyncOperation(session, unmatchedEvent, true)
	if op3 != nil {
		t.Error("Expected no sync operation for unmatched path")
	}
}

func TestSyncEngine_OperationTypeMapping(t *testing.T) {
	engine := &SyncEngine{}

	session := &SyncSession{
		SyncID:     "test-sync",
		LocalPath:  "/local/test",
		RemotePath: "/remote/test",
	}

	testCases := []struct {
		eventType      EventType
		isLocal        bool
		expectedOpType OperationType
		description    string
	}{
		{EventTypeFileCreated, true, OpTypeUpload, "Local file creation should trigger upload"},
		{EventTypeFileCreated, false, OpTypeDownload, "Remote file creation should trigger download"},
		{EventTypeFileModified, true, OpTypeUpload, "Local file modification should trigger upload"},
		{EventTypeFileModified, false, OpTypeDownload, "Remote file modification should trigger download"},
		{EventTypeFileDeleted, true, OpTypeDelete, "Local file deletion should trigger delete"},
		{EventTypeFileDeleted, false, OpTypeDelete, "Remote file deletion should trigger delete"},
		{EventTypeDirCreated, true, OpTypeCreateDir, "Local directory creation should trigger create dir"},
		{EventTypeDirCreated, false, OpTypeCreateDir, "Remote directory creation should trigger create dir"},
		{EventTypeDirDeleted, true, OpTypeDeleteDir, "Local directory deletion should trigger delete dir"},
		{EventTypeDirDeleted, false, OpTypeDeleteDir, "Remote directory deletion should trigger delete dir"},
	}

	for _, tc := range testCases {
		event := SyncEvent{
			Type:      tc.eventType,
			Path:      "/local/test/file.txt",
			Timestamp: time.Now(),
		}

		if !tc.isLocal {
			event.Path = "/remote/test/file.txt"
		}

		op := engine.createSyncOperation(session, event, tc.isLocal)
		if op == nil {
			t.Errorf("Expected sync operation for %s", tc.description)
			continue
		}

		if op.Type != tc.expectedOpType {
			t.Errorf("%s: Expected operation type %s, got %s", tc.description, tc.expectedOpType, op.Type)
		}
	}
}

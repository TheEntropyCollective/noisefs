package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TheEntropyCollective/noisefs/pkg/security"
)

// TestSyncEnginePathTraversalPrevention tests that sync operations reject malicious paths
func TestSyncEnginePathTraversalPrevention(t *testing.T) {
	// Create temporary directories for testing
	tempDir, err := os.MkdirTemp("", "sync-security-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	localDir := filepath.Join(tempDir, "local")
	
	if err := os.MkdirAll(localDir, 0755); err != nil {
		t.Fatalf("Failed to create local directory: %v", err)
	}

	// Create a test sync session
	session := &SyncSession{
		SyncID:     "test-sync-123",
		LocalPath:  localDir,
		RemotePath: "/remote/test",
		State:      &SyncState{},
		Status:     StatusIdle,
	}

	// Create a minimal sync engine for testing (just for the execute functions)
	syncEngine := &SyncEngine{}

	// Test malicious paths for different operations
	maliciousPaths := []string{
		"../../../etc/passwd",          // Unix path traversal
		"/etc/passwd",                  // Absolute path
		"..\\..\\windows\\system32",    // Windows path traversal
		"valid/path/../../../etc/passwd", // Hidden path traversal
		localDir + "/../../../etc/passwd", // Path outside allowed root
	}

	for _, maliciousPath := range maliciousPaths {
		t.Run("PathTraversal_"+strings.ReplaceAll(maliciousPath, "/", "_"), func(t *testing.T) {
			// Test each operation type with malicious path
			operations := []struct {
				name string
				op   SyncOperation
			}{
				{
					name: "Upload",
					op: SyncOperation{
						ID:        "test-upload",
						Type:      OpTypeUpload,
						LocalPath: maliciousPath,
					},
				},
				{
					name: "Download", 
					op: SyncOperation{
						ID:        "test-download",
						Type:      OpTypeDownload,
						LocalPath: maliciousPath,
					},
				},
				{
					name: "Delete",
					op: SyncOperation{
						ID:        "test-delete",
						Type:      OpTypeDelete,
						LocalPath: maliciousPath,
					},
				},
				{
					name: "CreateDir",
					op: SyncOperation{
						ID:        "test-createdir",
						Type:      OpTypeCreateDir,
						LocalPath: maliciousPath,
					},
				},
				{
					name: "DeleteDir",
					op: SyncOperation{
						ID:        "test-deletedir",
						Type:      OpTypeDeleteDir,
						LocalPath: maliciousPath,
					},
				},
			}

			for _, operation := range operations {
				t.Run(operation.name, func(t *testing.T) {
					var err error
					
					switch operation.op.Type {
					case OpTypeUpload:
						err = syncEngine.executeUpload(session, operation.op)
					case OpTypeDownload:
						err = syncEngine.executeDownload(session, operation.op)
					case OpTypeDelete:
						err = syncEngine.executeDelete(session, operation.op)
					case OpTypeCreateDir:
						err = syncEngine.executeCreateDir(session, operation.op)
					case OpTypeDeleteDir:
						err = syncEngine.executeDeleteDir(session, operation.op)
					}

					// All malicious paths should be rejected
					if err == nil {
						t.Errorf("Expected security validation error for %s operation with path %s, but got none", 
							operation.name, maliciousPath)
					}

					// Check that the error mentions security validation
					if err != nil && !strings.Contains(err.Error(), "security validation failed") {
						t.Errorf("Expected security validation error, got: %v", err)
					}
				})
			}
		})
	}
}

// TestStateStorePathTraversalPrevention tests that state store rejects malicious sync IDs
func TestStateStorePathTraversalPrevention(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "state-security-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	stateStore, err := NewSyncStateStore(tempDir)
	if err != nil {
		t.Fatalf("Failed to create state store: %v", err)
	}

	// Test malicious sync IDs
	maliciousSyncIDs := []string{
		"../../../etc/passwd",
		"/etc/passwd",
		"..\\..\\windows\\system32",
		"sync/../../../sensitive",
		"sync@#$%",
		"", // empty sync ID
		strings.Repeat("a", 101), // too long sync ID
	}

	for _, maliciousSyncID := range maliciousSyncIDs {
		t.Run("SyncID_"+strings.ReplaceAll(maliciousSyncID, "/", "_"), func(t *testing.T) {
			// Test getStateFile with malicious sync ID
			stateFile := stateStore.getStateFile(maliciousSyncID)
			
			// Should return safe default path for invalid sync IDs
			if strings.Contains(stateFile, "..") || 
			   strings.Contains(stateFile, "/etc/") ||
			   strings.Contains(stateFile, "\\windows\\") {
				t.Errorf("State file path contains unsafe elements: %s", stateFile)
			}
			
			// Should default to invalid.json for malicious IDs
			if !strings.HasSuffix(stateFile, "invalid.json") {
				// Only valid sync IDs should not use invalid.json
				if err := security.ValidateSyncID(maliciousSyncID); err == nil {
					// This was actually a valid sync ID, so it's OK
					return
				}
				t.Errorf("Expected invalid.json for malicious sync ID %s, got: %s", 
					maliciousSyncID, stateFile)
			}
		})
	}
}

// TestValidSyncOperations ensures that legitimate operations still work
func TestValidSyncOperations(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sync-valid-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	localDir := filepath.Join(tempDir, "local")
	stateDir := filepath.Join(tempDir, "state")
	
	if err := os.MkdirAll(localDir, 0755); err != nil {
		t.Fatalf("Failed to create local directory: %v", err)
	}
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatalf("Failed to create state directory: %v", err)
	}

	// Create state store for sync ID test
	stateStore, err := NewSyncStateStore(stateDir)
	if err != nil {
		t.Fatalf("Failed to create state store: %v", err)
	}

	// Create minimal sync engine for testing
	syncEngine := &SyncEngine{}

	session := &SyncSession{
		SyncID:     "valid-sync-123",
		LocalPath:  localDir,
		RemotePath: "/remote/test",
		State:      &SyncState{},
		Status:     StatusIdle,
	}

	// Create a valid test file
	testFile := filepath.Join(localDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test valid operations
	validOperations := []struct {
		name string
		op   SyncOperation
	}{
		{
			name: "ValidUpload",
			op: SyncOperation{
				ID:        "test-upload",
				Type:      OpTypeUpload,
				LocalPath: testFile,
			},
		},
		{
			name: "ValidDownload",
			op: SyncOperation{
				ID:        "test-download",
				Type:      OpTypeDownload,
				LocalPath: filepath.Join(localDir, "download.txt"),
			},
		},
		{
			name: "ValidCreateDir",
			op: SyncOperation{
				ID:        "test-createdir",
				Type:      OpTypeCreateDir,
				LocalPath: filepath.Join(localDir, "newdir"),
			},
		},
	}

	for _, operation := range validOperations {
		t.Run(operation.name, func(t *testing.T) {
			var err error
			
			switch operation.op.Type {
			case OpTypeUpload:
				err = syncEngine.executeUpload(session, operation.op)
			case OpTypeDownload:
				err = syncEngine.executeDownload(session, operation.op)
			case OpTypeCreateDir:
				err = syncEngine.executeCreateDir(session, operation.op)
			}

			// Valid operations should not fail due to security validation
			if err != nil && strings.Contains(err.Error(), "security validation failed") {
				t.Errorf("Valid operation %s failed security validation: %v", operation.name, err)
			}
		})
	}

	// Test valid sync ID in state store
	validSyncID := "valid-sync-session-123"
	stateFile := stateStore.getStateFile(validSyncID)
	
	if !strings.HasSuffix(stateFile, validSyncID+".json") {
		t.Errorf("Valid sync ID should generate correct state file path, got: %s", stateFile)
	}
}
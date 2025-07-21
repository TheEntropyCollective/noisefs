package sync

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSyncEvent_JSON(t *testing.T) {
	now := time.Now()
	event := SyncEvent{
		Type:      EventTypeFileModified,
		Path:      "/test/file.txt",
		Timestamp: now,
		Metadata: map[string]interface{}{
			"size":     1024,
			"checksum": "abc123",
		},
	}

	// Test JSON marshaling
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal SyncEvent: %v", err)
	}

	// Test JSON unmarshaling
	var unmarshaled SyncEvent
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal SyncEvent: %v", err)
	}

	if unmarshaled.Type != EventTypeFileModified {
		t.Errorf("Expected Type %s, got %s", EventTypeFileModified, unmarshaled.Type)
	}
	if unmarshaled.Path != "/test/file.txt" {
		t.Errorf("Expected Path '/test/file.txt', got '%s'", unmarshaled.Path)
	}
	if !unmarshaled.Timestamp.Equal(now) {
		t.Errorf("Expected Timestamp %v, got %v", now, unmarshaled.Timestamp)
	}
}

func TestFileMetadata_JSON(t *testing.T) {
	now := time.Now()
	metadata := FileMetadata{
		Path:        "/test/file.txt",
		Size:        1024,
		ModTime:     now,
		IsDir:       false,
		Checksum:    "abc123",
		Permissions: 0644,
	}

	// Test JSON marshaling
	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("Failed to marshal FileMetadata: %v", err)
	}

	// Test JSON unmarshaling
	var unmarshaled FileMetadata
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal FileMetadata: %v", err)
	}

	if unmarshaled.Path != "/test/file.txt" {
		t.Errorf("Expected Path '/test/file.txt', got '%s'", unmarshaled.Path)
	}
	if unmarshaled.Size != 1024 {
		t.Errorf("Expected Size 1024, got %d", unmarshaled.Size)
	}
	if !unmarshaled.ModTime.Equal(now) {
		t.Errorf("Expected ModTime %v, got %v", now, unmarshaled.ModTime)
	}
	if unmarshaled.IsDir != false {
		t.Errorf("Expected IsDir false, got %v", unmarshaled.IsDir)
	}
	if unmarshaled.Checksum != "abc123" {
		t.Errorf("Expected Checksum 'abc123', got '%s'", unmarshaled.Checksum)
	}
	if unmarshaled.Permissions != 0644 {
		t.Errorf("Expected Permissions 0644, got %d", unmarshaled.Permissions)
	}
}

func TestRemoteMetadata_JSON(t *testing.T) {
	now := time.Now()
	metadata := RemoteMetadata{
		Path:          "/remote/file.txt",
		DescriptorCID: "QmTest123",
		Size:          2048,
		ModTime:       now,
		IsDir:         false,
		EncryptionKey: "key123",
	}

	// Test JSON marshaling
	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("Failed to marshal RemoteMetadata: %v", err)
	}

	// Test JSON unmarshaling
	var unmarshaled RemoteMetadata
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal RemoteMetadata: %v", err)
	}

	if unmarshaled.Path != "/remote/file.txt" {
		t.Errorf("Expected Path '/remote/file.txt', got '%s'", unmarshaled.Path)
	}
	if unmarshaled.DescriptorCID != "QmTest123" {
		t.Errorf("Expected DescriptorCID 'QmTest123', got '%s'", unmarshaled.DescriptorCID)
	}
	if unmarshaled.Size != 2048 {
		t.Errorf("Expected Size 2048, got %d", unmarshaled.Size)
	}
	if !unmarshaled.ModTime.Equal(now) {
		t.Errorf("Expected ModTime %v, got %v", now, unmarshaled.ModTime)
	}
	if unmarshaled.IsDir != false {
		t.Errorf("Expected IsDir false, got %v", unmarshaled.IsDir)
	}
	if unmarshaled.EncryptionKey != "key123" {
		t.Errorf("Expected EncryptionKey 'key123', got '%s'", unmarshaled.EncryptionKey)
	}
}

func TestSyncOperation_JSON(t *testing.T) {
	now := time.Now()
	operation := SyncOperation{
		ID:         "op-123",
		Type:       OpTypeUpload,
		LocalPath:  "/local/file.txt",
		RemotePath: "/remote/file.txt",
		Timestamp:  now,
		Status:     OpStatusCompleted,
		Retries:    2,
		Error:      "",
	}

	// Test JSON marshaling
	data, err := json.Marshal(operation)
	if err != nil {
		t.Fatalf("Failed to marshal SyncOperation: %v", err)
	}

	// Test JSON unmarshaling
	var unmarshaled SyncOperation
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal SyncOperation: %v", err)
	}

	if unmarshaled.ID != "op-123" {
		t.Errorf("Expected ID 'op-123', got '%s'", unmarshaled.ID)
	}
	if unmarshaled.Type != OpTypeUpload {
		t.Errorf("Expected Type %s, got %s", OpTypeUpload, unmarshaled.Type)
	}
	if unmarshaled.LocalPath != "/local/file.txt" {
		t.Errorf("Expected LocalPath '/local/file.txt', got '%s'", unmarshaled.LocalPath)
	}
	if unmarshaled.RemotePath != "/remote/file.txt" {
		t.Errorf("Expected RemotePath '/remote/file.txt', got '%s'", unmarshaled.RemotePath)
	}
	if !unmarshaled.Timestamp.Equal(now) {
		t.Errorf("Expected Timestamp %v, got %v", now, unmarshaled.Timestamp)
	}
	if unmarshaled.Status != OpStatusCompleted {
		t.Errorf("Expected Status %s, got %s", OpStatusCompleted, unmarshaled.Status)
	}
	if unmarshaled.Retries != 2 {
		t.Errorf("Expected Retries 2, got %d", unmarshaled.Retries)
	}
}

func TestConflict_JSON(t *testing.T) {
	now := time.Now()
	conflict := Conflict{
		ID:         "conflict-123",
		LocalPath:  "/local/file.txt",
		RemotePath: "/remote/file.txt",
		LocalMetadata: FileMetadata{
			Path:    "/local/file.txt",
			Size:    1024,
			ModTime: now,
			IsDir:   false,
		},
		RemoteMetadata: RemoteMetadata{
			Path:          "/remote/file.txt",
			DescriptorCID: "QmTest123",
			Size:          2048,
			ModTime:       now.Add(time.Hour),
			IsDir:         false,
		},
		ConflictType: ConflictTypeBothModified,
		Resolution:   ConflictResolveLocal,
		Timestamp:    now,
	}

	// Test JSON marshaling
	data, err := json.Marshal(conflict)
	if err != nil {
		t.Fatalf("Failed to marshal Conflict: %v", err)
	}

	// Test JSON unmarshaling
	var unmarshaled Conflict
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal Conflict: %v", err)
	}

	if unmarshaled.ID != "conflict-123" {
		t.Errorf("Expected ID 'conflict-123', got '%s'", unmarshaled.ID)
	}
	if unmarshaled.ConflictType != ConflictTypeBothModified {
		t.Errorf("Expected ConflictType %s, got %s", ConflictTypeBothModified, unmarshaled.ConflictType)
	}
	if unmarshaled.Resolution != ConflictResolveLocal {
		t.Errorf("Expected Resolution %s, got %s", ConflictResolveLocal, unmarshaled.Resolution)
	}
	if unmarshaled.LocalMetadata.Size != 1024 {
		t.Errorf("Expected LocalMetadata Size 1024, got %d", unmarshaled.LocalMetadata.Size)
	}
	if unmarshaled.RemoteMetadata.Size != 2048 {
		t.Errorf("Expected RemoteMetadata Size 2048, got %d", unmarshaled.RemoteMetadata.Size)
	}
}

func TestSyncConfig_JSON(t *testing.T) {
	config := SyncConfig{
		IncludePatterns:    []string{"*.txt", "*.go"},
		ExcludePatterns:    []string{"*.log", "*.tmp"},
		ConflictResolution: ConflictResolveTimestamp,
		SyncInterval:       30 * time.Second,
		MaxRetries:         3,
		WatchMode:          true,
	}

	// Test JSON marshaling
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Failed to marshal SyncConfig: %v", err)
	}

	// Test JSON unmarshaling
	var unmarshaled SyncConfig
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal SyncConfig: %v", err)
	}

	if len(unmarshaled.IncludePatterns) != 2 {
		t.Errorf("Expected 2 include patterns, got %d", len(unmarshaled.IncludePatterns))
	}
	if len(unmarshaled.ExcludePatterns) != 2 {
		t.Errorf("Expected 2 exclude patterns, got %d", len(unmarshaled.ExcludePatterns))
	}
	if unmarshaled.ConflictResolution != ConflictResolveTimestamp {
		t.Errorf("Expected ConflictResolution %s, got %s", ConflictResolveTimestamp, unmarshaled.ConflictResolution)
	}
	if unmarshaled.SyncInterval != 30*time.Second {
		t.Errorf("Expected SyncInterval 30s, got %v", unmarshaled.SyncInterval)
	}
	if unmarshaled.MaxRetries != 3 {
		t.Errorf("Expected MaxRetries 3, got %d", unmarshaled.MaxRetries)
	}
	if !unmarshaled.WatchMode {
		t.Errorf("Expected WatchMode true, got %v", unmarshaled.WatchMode)
	}
}

func TestEventTypes(t *testing.T) {
	eventTypes := []EventType{
		EventTypeFileCreated,
		EventTypeFileModified,
		EventTypeFileDeleted,
		EventTypeDirCreated,
		EventTypeDirDeleted,
	}

	expectedValues := []string{
		"file_created",
		"file_modified",
		"file_deleted",
		"dir_created",
		"dir_deleted",
	}

	for i, eventType := range eventTypes {
		if string(eventType) != expectedValues[i] {
			t.Errorf("Expected event type %s, got %s", expectedValues[i], string(eventType))
		}
	}
}

func TestOperationTypes(t *testing.T) {
	operationTypes := []OperationType{
		OpTypeUpload,
		OpTypeDownload,
		OpTypeDelete,
		OpTypeCreateDir,
		OpTypeDeleteDir,
	}

	expectedValues := []string{
		"upload",
		"download",
		"delete",
		"create_dir",
		"delete_dir",
	}

	for i, opType := range operationTypes {
		if string(opType) != expectedValues[i] {
			t.Errorf("Expected operation type %s, got %s", expectedValues[i], string(opType))
		}
	}
}

func TestConflictTypes(t *testing.T) {
	conflictTypes := []ConflictType{
		ConflictTypeBothModified,
		ConflictTypeDeletedLocal,
		ConflictTypeDeletedRemote,
		ConflictTypeTypeChanged,
	}

	expectedValues := []string{
		"both_modified",
		"deleted_local",
		"deleted_remote",
		"type_changed",
	}

	for i, conflictType := range conflictTypes {
		if string(conflictType) != expectedValues[i] {
			t.Errorf("Expected conflict type %s, got %s", expectedValues[i], string(conflictType))
		}
	}
}

func TestConflictResolutions(t *testing.T) {
	resolutions := []ConflictResolution{
		ConflictResolveLocal,
		ConflictResolveRemote,
		ConflictResolveTimestamp,
		ConflictResolvePrompt,
	}

	expectedValues := []string{
		"local",
		"remote",
		"timestamp",
		"prompt",
	}

	for i, resolution := range resolutions {
		if string(resolution) != expectedValues[i] {
			t.Errorf("Expected resolution %s, got %s", expectedValues[i], string(resolution))
		}
	}
}

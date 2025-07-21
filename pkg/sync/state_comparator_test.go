package sync

import (
	"testing"
	"time"
)

func TestCompareStatesNoChanges(t *testing.T) {
	comparator := NewStateComparator()

	// Create identical snapshots
	timestamp := time.Now()
	localFiles := map[string]FileMetadata{
		"file1.txt": {
			Path:     "file1.txt",
			Size:     100,
			ModTime:  timestamp,
			IsDir:    false,
			Checksum: "abc123",
		},
	}

	remoteFiles := map[string]RemoteMetadata{
		"file1.txt": {
			Path:          "file1.txt",
			DescriptorCID: "QmXYZ",
			ContentCID:    "QmABC",
			Size:          100,
			ModTime:       timestamp,
		},
	}

	oldSnapshot := &StateSnapshot{
		LocalFiles:  localFiles,
		RemoteFiles: remoteFiles,
		Timestamp:   timestamp,
	}

	newSnapshot := &StateSnapshot{
		LocalFiles:  localFiles,
		RemoteFiles: remoteFiles,
		Timestamp:   timestamp,
	}

	changes, err := comparator.CompareStates(oldSnapshot, newSnapshot)
	if err != nil {
		t.Fatalf("Failed to compare states: %v", err)
	}

	if len(changes) != 0 {
		t.Errorf("Expected no changes, got %d", len(changes))
	}
}

func TestCompareStatesFileCreated(t *testing.T) {
	comparator := NewStateComparator()

	// Old snapshot with no files
	oldSnapshot := &StateSnapshot{
		LocalFiles:  make(map[string]FileMetadata),
		RemoteFiles: make(map[string]RemoteMetadata),
		Timestamp:   time.Now(),
	}

	// New snapshot with a file
	newLocalFiles := map[string]FileMetadata{
		"new_file.txt": {
			Path:     "new_file.txt",
			Size:     50,
			ModTime:  time.Now(),
			IsDir:    false,
			Checksum: "def456",
		},
	}

	newSnapshot := &StateSnapshot{
		LocalFiles:  newLocalFiles,
		RemoteFiles: make(map[string]RemoteMetadata),
		Timestamp:   time.Now(),
	}

	changes, err := comparator.CompareStates(oldSnapshot, newSnapshot)
	if err != nil {
		t.Fatalf("Failed to compare states: %v", err)
	}

	if len(changes) != 1 {
		t.Fatalf("Expected 1 change, got %d", len(changes))
	}

	change := changes[0]
	if change.Type != ChangeTypeCreate {
		t.Errorf("Expected create change, got %s", change.Type)
	}
	if change.Path != "new_file.txt" {
		t.Errorf("Expected path new_file.txt, got %s", change.Path)
	}
	if !change.IsLocal {
		t.Error("Expected local change")
	}
}

func TestCompareStatesFileModified(t *testing.T) {
	comparator := NewStateComparator()

	// Old snapshot
	oldTime := time.Now().Add(-time.Hour)
	oldLocalFiles := map[string]FileMetadata{
		"file.txt": {
			Path:     "file.txt",
			Size:     100,
			ModTime:  oldTime,
			IsDir:    false,
			Checksum: "abc123",
		},
	}

	oldSnapshot := &StateSnapshot{
		LocalFiles:  oldLocalFiles,
		RemoteFiles: make(map[string]RemoteMetadata),
		Timestamp:   oldTime,
	}

	// New snapshot with modified file
	newTime := time.Now()
	newLocalFiles := map[string]FileMetadata{
		"file.txt": {
			Path:     "file.txt",
			Size:     150,
			ModTime:  newTime,
			IsDir:    false,
			Checksum: "def456",
		},
	}

	newSnapshot := &StateSnapshot{
		LocalFiles:  newLocalFiles,
		RemoteFiles: make(map[string]RemoteMetadata),
		Timestamp:   newTime,
	}

	changes, err := comparator.CompareStates(oldSnapshot, newSnapshot)
	if err != nil {
		t.Fatalf("Failed to compare states: %v", err)
	}

	if len(changes) != 1 {
		t.Fatalf("Expected 1 change, got %d", len(changes))
	}

	change := changes[0]
	if change.Type != ChangeTypeModify {
		t.Errorf("Expected modify change, got %s", change.Type)
	}
	if change.Path != "file.txt" {
		t.Errorf("Expected path file.txt, got %s", change.Path)
	}
}

func TestDetectConflictsBothModified(t *testing.T) {
	comparator := NewStateComparator()

	// Create local and remote changes for the same file
	localChanges := []DetectedChange{
		{
			Type:    ChangeTypeModify,
			Path:    "conflict.txt",
			IsLocal: true,
			Metadata: FileMetadata{
				Path:     "conflict.txt",
				Size:     200,
				Checksum: "local123",
			},
		},
	}

	remoteChanges := []DetectedChange{
		{
			Type:    ChangeTypeModify,
			Path:    "conflict.txt",
			IsLocal: false,
			Metadata: RemoteMetadata{
				Path:          "conflict.txt",
				DescriptorCID: "QmRemote",
				Size:          250,
			},
		},
	}

	conflicts := comparator.DetectConflicts(localChanges, remoteChanges)

	if len(conflicts) != 1 {
		t.Fatalf("Expected 1 conflict, got %d", len(conflicts))
	}

	conflict := conflicts[0]
	if conflict.ConflictType != ConflictTypeBothModified {
		t.Errorf("Expected both modified conflict, got %s", conflict.ConflictType)
	}
	if conflict.LocalPath != "conflict.txt" {
		t.Errorf("Expected path conflict.txt, got %s", conflict.LocalPath)
	}
}

func TestIsFileModified(t *testing.T) {
	comparator := NewStateComparator()

	baseTime := time.Now()

	baseFile := FileMetadata{
		Path:        "test.txt",
		Size:        100,
		ModTime:     baseTime,
		Checksum:    "abc123",
		Permissions: 0644,
	}

	// Same file - should not be modified
	sameFile := baseFile
	if comparator.isFileModified(baseFile, sameFile) {
		t.Error("Same file should not be detected as modified")
	}

	// Different modification time - should be modified
	differentTimeFile := baseFile
	differentTimeFile.ModTime = baseTime.Add(time.Hour)
	if !comparator.isFileModified(baseFile, differentTimeFile) {
		t.Error("File with different mod time should be detected as modified")
	}

	// Different size - should be modified
	differentSizeFile := baseFile
	differentSizeFile.Size = 200
	if !comparator.isFileModified(baseFile, differentSizeFile) {
		t.Error("File with different size should be detected as modified")
	}

	// Different checksum - should be modified
	differentChecksumFile := baseFile
	differentChecksumFile.Checksum = "def456"
	if !comparator.isFileModified(baseFile, differentChecksumFile) {
		t.Error("File with different checksum should be detected as modified")
	}

	// Different permissions - should be modified
	differentPermFile := baseFile
	differentPermFile.Permissions = 0755
	if !comparator.isFileModified(baseFile, differentPermFile) {
		t.Error("File with different permissions should be detected as modified")
	}
}

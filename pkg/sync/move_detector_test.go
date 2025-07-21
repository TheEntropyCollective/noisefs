package sync

import (
	"testing"
	"time"
)

func TestDetectMovesLocalBySameInode(t *testing.T) {
	detector := NewMoveDetector()

	timestamp := time.Now()

	// Old snapshot with file at original location
	oldFiles := map[string]FileMetadata{
		"old/file.txt": {
			Path:     "old/file.txt",
			Size:     100,
			ModTime:  timestamp,
			Checksum: "abc123",
			Inode:    12345,
			Device:   67890,
		},
	}

	// New snapshot with file at new location (same inode)
	newFiles := map[string]FileMetadata{
		"new/file.txt": {
			Path:     "new/file.txt",
			Size:     100,
			ModTime:  timestamp,
			Checksum: "abc123",
			Inode:    12345,
			Device:   67890,
		},
	}

	oldSnapshot := &StateSnapshot{LocalFiles: oldFiles, RemoteFiles: make(map[string]RemoteMetadata)}
	newSnapshot := &StateSnapshot{LocalFiles: newFiles, RemoteFiles: make(map[string]RemoteMetadata)}

	moves := detector.DetectMoves(oldSnapshot, newSnapshot)

	if len(moves) != 1 {
		t.Fatalf("Expected 1 move, got %d", len(moves))
	}

	move := moves[0]
	if move.OldPath != "old/file.txt" {
		t.Errorf("Expected old path old/file.txt, got %s", move.OldPath)
	}
	if move.NewPath != "new/file.txt" {
		t.Errorf("Expected new path new/file.txt, got %s", move.NewPath)
	}
	if move.Confidence != 1.0 {
		t.Errorf("Expected confidence 1.0 for same inode, got %f", move.Confidence)
	}
	if !move.IsLocal {
		t.Error("Expected local move")
	}
}

func TestDetectMovesLocalByChecksum(t *testing.T) {
	detector := NewMoveDetector()

	timestamp := time.Now()

	// Old snapshot
	oldFiles := map[string]FileMetadata{
		"old/document.txt": {
			Path:     "old/document.txt",
			Size:     1000,
			ModTime:  timestamp,
			Checksum: "unique_checksum_123",
		},
	}

	// New snapshot with same content, different path
	newFiles := map[string]FileMetadata{
		"new/document.txt": {
			Path:     "new/document.txt",
			Size:     1000,
			ModTime:  timestamp,
			Checksum: "unique_checksum_123",
		},
	}

	oldSnapshot := &StateSnapshot{LocalFiles: oldFiles, RemoteFiles: make(map[string]RemoteMetadata)}
	newSnapshot := &StateSnapshot{LocalFiles: newFiles, RemoteFiles: make(map[string]RemoteMetadata)}

	moves := detector.DetectMoves(oldSnapshot, newSnapshot)

	if len(moves) != 1 {
		t.Fatalf("Expected 1 move, got %d", len(moves))
	}

	move := moves[0]
	if move.Confidence < 0.8 {
		t.Errorf("Expected high confidence for same checksum, got %f", move.Confidence)
	}
	if move.OldPath != "old/document.txt" {
		t.Errorf("Expected old path old/document.txt, got %s", move.OldPath)
	}
	if move.NewPath != "new/document.txt" {
		t.Errorf("Expected new path new/document.txt, got %s", move.NewPath)
	}
}

func TestDetectMovesRemoteByCID(t *testing.T) {
	detector := NewMoveDetector()

	timestamp := time.Now()

	// Old snapshot
	oldFiles := map[string]RemoteMetadata{
		"old/remote.txt": {
			Path:          "old/remote.txt",
			DescriptorCID: "QmSameDescriptor123",
			ContentCID:    "QmSameContent456",
			Size:          500,
			ModTime:       timestamp,
		},
	}

	// New snapshot with same CIDs, different path
	newFiles := map[string]RemoteMetadata{
		"new/remote.txt": {
			Path:          "new/remote.txt",
			DescriptorCID: "QmSameDescriptor123",
			ContentCID:    "QmSameContent456",
			Size:          500,
			ModTime:       timestamp,
		},
	}

	oldSnapshot := &StateSnapshot{LocalFiles: make(map[string]FileMetadata), RemoteFiles: oldFiles}
	newSnapshot := &StateSnapshot{LocalFiles: make(map[string]FileMetadata), RemoteFiles: newFiles}

	moves := detector.DetectMoves(oldSnapshot, newSnapshot)

	if len(moves) != 1 {
		t.Fatalf("Expected 1 move, got %d", len(moves))
	}

	move := moves[0]
	if move.Confidence != 1.0 {
		t.Errorf("Expected confidence 1.0 for same descriptor CID, got %f", move.Confidence)
	}
	if move.IsLocal {
		t.Error("Expected remote move")
	}
}

func TestCalculateNameSimilarity(t *testing.T) {
	detector := NewMoveDetector()

	// Exact match
	similarity := detector.calculateNameSimilarity("file.txt", "file.txt")
	if similarity != 1.0 {
		t.Errorf("Expected 1.0 for exact match, got %f", similarity)
	}

	// Same filename, different directory
	similarity = detector.calculateNameSimilarity("old/file.txt", "new/file.txt")
	if similarity != 1.0 { // It's actually exact filename match
		t.Errorf("Expected 1.0 for same filename different directory, got %f", similarity)
	}

	// Completely different
	similarity = detector.calculateNameSimilarity("file1.txt", "document.pdf")
	if similarity >= 0.5 {
		t.Errorf("Expected low similarity for different files, got %f", similarity)
	}
}

func TestLevenshteinDistance(t *testing.T) {
	detector := NewMoveDetector()

	// Same strings
	distance := detector.levenshteinDistance("hello", "hello")
	if distance != 0 {
		t.Errorf("Expected distance 0 for same strings, got %d", distance)
	}

	// One character difference
	distance = detector.levenshteinDistance("hello", "hallo")
	if distance != 1 {
		t.Errorf("Expected distance 1 for one char diff, got %d", distance)
	}

	// Complete replacement
	distance = detector.levenshteinDistance("cat", "dog")
	if distance != 3 {
		t.Errorf("Expected distance 3 for complete replacement, got %d", distance)
	}

	// Empty string
	distance = detector.levenshteinDistance("", "hello")
	if distance != 5 {
		t.Errorf("Expected distance 5 for empty to hello, got %d", distance)
	}
}

func TestMoveDetectorConfidenceThreshold(t *testing.T) {
	detector := NewMoveDetector()
	detector.MinConfidence = 0.9 // Set high threshold

	// Files with only moderate similarity (below threshold)
	oldFiles := map[string]FileMetadata{
		"file.txt": {
			Path:    "file.txt",
			Size:    100,
			ModTime: time.Now(),
		},
	}

	newFiles := map[string]FileMetadata{
		"similar.txt": {
			Path:    "similar.txt",
			Size:    100,
			ModTime: time.Now().Add(time.Hour), // Different time
		},
	}

	oldSnapshot := &StateSnapshot{LocalFiles: oldFiles, RemoteFiles: make(map[string]RemoteMetadata)}
	newSnapshot := &StateSnapshot{LocalFiles: newFiles, RemoteFiles: make(map[string]RemoteMetadata)}

	moves := detector.DetectMoves(oldSnapshot, newSnapshot)

	// Should not detect move due to low confidence
	if len(moves) > 0 {
		t.Errorf("Expected no moves due to low confidence, got %d", len(moves))
	}
}

package blocks

import (
	"bytes"
	"fmt"
	"testing"
	"time"
)

func TestDirectoryManifest_RemoveEntry(t *testing.T) {
	manifest := NewDirectoryManifest()

	// Add test entries
	entry1 := DirectoryEntry{
		EncryptedName: []byte("file1.txt"),
		CID:           "QmTestCID1",
		Type:          FileType,
		Size:          100,
		ModifiedAt:    time.Now(),
	}
	entry2 := DirectoryEntry{
		EncryptedName: []byte("file2.txt"),
		CID:           "QmTestCID2",
		Type:          FileType,
		Size:          200,
		ModifiedAt:    time.Now(),
	}

	manifest.AddEntry(entry1)
	manifest.AddEntry(entry2)

	// Test removing existing entry
	err := manifest.RemoveEntry([]byte("file1.txt"))
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	entries := manifest.GetEntriesCopy()
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry after removal, got: %d", len(entries))
	}

	if !bytes.Equal(entries[0].EncryptedName, []byte("file2.txt")) {
		t.Fatalf("Expected remaining entry to be file2.txt, got: %s", string(entries[0].EncryptedName))
	}

	// Test removing non-existent entry
	err = manifest.RemoveEntry([]byte("nonexistent.txt"))
	if err == nil {
		t.Fatal("Expected error for non-existent entry")
	}

	// Test removing with empty name
	err = manifest.RemoveEntry([]byte{})
	if err == nil {
		t.Fatal("Expected error for empty encrypted name")
	}
}

func TestDirectoryManifest_UpdateEntry(t *testing.T) {
	manifest := NewDirectoryManifest()

	// Add test entry
	originalEntry := DirectoryEntry{
		EncryptedName: []byte("file1.txt"),
		CID:           "QmTestCID1",
		Type:          FileType,
		Size:          100,
		ModifiedAt:    time.Now(),
	}
	manifest.AddEntry(originalEntry)

	// Test updating existing entry
	updatedEntry := DirectoryEntry{
		EncryptedName: []byte("file1.txt"),
		CID:           "QmTestCID2",
		Type:          FileType,
		Size:          200,
		ModifiedAt:    time.Now(),
	}

	err := manifest.UpdateEntry([]byte("file1.txt"), updatedEntry)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	entries := manifest.GetEntriesCopy()
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry after update, got: %d", len(entries))
	}

	if entries[0].CID != "QmTestCID2" {
		t.Fatalf("Expected CID to be updated to QmTestCID2, got: %s", entries[0].CID)
	}

	if entries[0].Size != 200 {
		t.Fatalf("Expected size to be updated to 200, got: %d", entries[0].Size)
	}

	// Test updating non-existent entry
	err = manifest.UpdateEntry([]byte("nonexistent.txt"), updatedEntry)
	if err == nil {
		t.Fatal("Expected error for non-existent entry")
	}

	// Test updating with invalid entry
	invalidEntry := DirectoryEntry{
		EncryptedName: []byte{},
		CID:           "QmTestCID3",
		Type:          FileType,
		Size:          300,
		ModifiedAt:    time.Now(),
	}

	err = manifest.UpdateEntry([]byte("file1.txt"), invalidEntry)
	if err == nil {
		t.Fatal("Expected error for invalid new entry")
	}
}

func TestDirectoryManifest_FindEntryByName(t *testing.T) {
	manifest := NewDirectoryManifest()

	// Add test entries
	entry1 := DirectoryEntry{
		EncryptedName: []byte("file1.txt"),
		CID:           "QmTestCID1",
		Type:          FileType,
		Size:          100,
		ModifiedAt:    time.Now(),
	}
	entry2 := DirectoryEntry{
		EncryptedName: []byte("file2.txt"),
		CID:           "QmTestCID2",
		Type:          FileType,
		Size:          200,
		ModifiedAt:    time.Now(),
	}

	manifest.AddEntry(entry1)
	manifest.AddEntry(entry2)

	// Test finding existing entry
	index, entry, err := manifest.FindEntryByName([]byte("file1.txt"))
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if index != 0 {
		t.Fatalf("Expected index 0, got: %d", index)
	}

	if entry.CID != "QmTestCID1" {
		t.Fatalf("Expected CID QmTestCID1, got: %s", entry.CID)
	}

	// Test finding non-existent entry
	_, _, err = manifest.FindEntryByName([]byte("nonexistent.txt"))
	if err == nil {
		t.Fatal("Expected error for non-existent entry")
	}

	// Test finding with empty name
	_, _, err = manifest.FindEntryByName([]byte{})
	if err == nil {
		t.Fatal("Expected error for empty encrypted name")
	}
}

func TestDirectoryManifest_HasEntry(t *testing.T) {
	manifest := NewDirectoryManifest()

	// Add test entry
	entry := DirectoryEntry{
		EncryptedName: []byte("file1.txt"),
		CID:           "QmTestCID1",
		Type:          FileType,
		Size:          100,
		ModifiedAt:    time.Now(),
	}
	manifest.AddEntry(entry)

	// Test existing entry
	if !manifest.HasEntry([]byte("file1.txt")) {
		t.Fatal("Expected entry to exist")
	}

	// Test non-existent entry
	if manifest.HasEntry([]byte("nonexistent.txt")) {
		t.Fatal("Expected entry to not exist")
	}

	// Test empty name
	if manifest.HasEntry([]byte{}) {
		t.Fatal("Expected empty name to return false")
	}
}

func TestDirectoryManifest_ConcurrentAccess(t *testing.T) {
	manifest := NewDirectoryManifest()

	// Test concurrent add and read operations
	done := make(chan bool, 2)

	// Goroutine 1: Add entries
	go func() {
		for i := 0; i < 10; i++ {
			entry := DirectoryEntry{
				EncryptedName: []byte(fmt.Sprintf("file%d.txt", i)),
				CID:           fmt.Sprintf("QmTestCID%d", i),
				Type:          FileType,
				Size:          int64(i * 100),
				ModifiedAt:    time.Now(),
			}
			manifest.AddEntry(entry)
		}
		done <- true
	}()

	// Goroutine 2: Read entries
	go func() {
		for i := 0; i < 10; i++ {
			entries := manifest.GetEntriesCopy()
			_ = entries // Just reading, no validation needed for race test
		}
		done <- true
	}()

	// Wait for both goroutines to complete
	<-done
	<-done

	// Verify final state
	entries := manifest.GetEntriesCopy()
	if len(entries) != 10 {
		t.Fatalf("Expected 10 entries, got: %d", len(entries))
	}
}

func TestDirectoryManifest_EdgeCases(t *testing.T) {
	manifest := NewDirectoryManifest()

	// Test removing from empty manifest
	err := manifest.RemoveEntry([]byte("file.txt"))
	if err == nil {
		t.Fatal("Expected error when removing from empty manifest")
	}

	// Test updating empty manifest
	entry := DirectoryEntry{
		EncryptedName: []byte("file.txt"),
		CID:           "QmTestCID",
		Type:          FileType,
		Size:          100,
		ModifiedAt:    time.Now(),
	}

	err = manifest.UpdateEntry([]byte("file.txt"), entry)
	if err == nil {
		t.Fatal("Expected error when updating empty manifest")
	}

	// Test finding in empty manifest
	_, _, err = manifest.FindEntryByName([]byte("file.txt"))
	if err == nil {
		t.Fatal("Expected error when finding in empty manifest")
	}

	// Test has entry in empty manifest
	if manifest.HasEntry([]byte("file.txt")) {
		t.Fatal("Expected false for has entry in empty manifest")
	}
}

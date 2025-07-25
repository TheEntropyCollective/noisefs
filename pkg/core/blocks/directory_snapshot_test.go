package blocks

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/crypto"
)

// TestDirectoryManifestSnapshot tests the snapshot functionality of DirectoryManifest
func TestDirectoryManifestSnapshot(t *testing.T) {
	// Create a sample directory manifest
	manifest := NewDirectoryManifest()

	// Add some test entries
	encryptionKey, err := crypto.GenerateKey("test-key")
	if err != nil {
		t.Fatalf("Failed to generate encryption key: %v", err)
	}

	// Add test entries
	entry1 := DirectoryEntry{
		EncryptedName: []byte("encrypted_file1"),
		CID:           "QmTest1234567890",
		Type:          FileType,
		Size:          1024,
		ModifiedAt:    time.Now().UTC(),
	}

	entry2 := DirectoryEntry{
		EncryptedName: []byte("encrypted_file2"),
		CID:           "QmTest0987654321",
		Type:          FileType,
		Size:          2048,
		ModifiedAt:    time.Now().UTC(),
	}

	err = manifest.AddEntry(entry1)
	if err != nil {
		t.Fatalf("Failed to add entry1: %v", err)
	}

	err = manifest.AddEntry(entry2)
	if err != nil {
		t.Fatalf("Failed to add entry2: %v", err)
	}

	// Test that original manifest is not a snapshot
	if manifest.IsSnapshot() {
		t.Error("Original manifest should not be a snapshot")
	}

	// Create a snapshot
	originalCID := "QmOriginal123456"
	snapshotName := "test-snapshot"
	description := "Test snapshot description"

	snapshotManifest := NewSnapshotManifest(manifest, originalCID, snapshotName, description)

	// Test that snapshot manifest is properly created
	if !snapshotManifest.IsSnapshot() {
		t.Error("Snapshot manifest should be a snapshot")
	}

	// Test snapshot info
	snapshotInfo := snapshotManifest.GetSnapshotInfo()
	if snapshotInfo == nil {
		t.Fatal("Snapshot info should not be nil")
	}

	if snapshotInfo.OriginalCID != originalCID {
		t.Errorf("Expected original CID %s, got %s", originalCID, snapshotInfo.OriginalCID)
	}

	if snapshotInfo.SnapshotName != snapshotName {
		t.Errorf("Expected snapshot name %s, got %s", snapshotName, snapshotInfo.SnapshotName)
	}

	if snapshotInfo.Description != description {
		t.Errorf("Expected description %s, got %s", description, snapshotInfo.Description)
	}

	if !snapshotInfo.IsSnapshot {
		t.Error("Snapshot info should indicate this is a snapshot")
	}

	// Test that snapshot has the same entries as original
	if len(snapshotManifest.Entries) != len(manifest.Entries) {
		t.Errorf("Expected %d entries, got %d", len(manifest.Entries), len(snapshotManifest.Entries))
	}

	// Test that entries are properly copied
	for i, entry := range snapshotManifest.Entries {
		originalEntry := manifest.Entries[i]
		if entry.CID != originalEntry.CID {
			t.Errorf("Entry %d: expected CID %s, got %s", i, originalEntry.CID, entry.CID)
		}
		if entry.Type != originalEntry.Type {
			t.Errorf("Entry %d: expected type %d, got %d", i, originalEntry.Type, entry.Type)
		}
		if entry.Size != originalEntry.Size {
			t.Errorf("Entry %d: expected size %d, got %d", i, originalEntry.Size, entry.Size)
		}
	}

	// Test thread-safe snapshot creation
	snapshot := snapshotManifest.GetSnapshot()
	if snapshot.SnapshotInfo == nil {
		t.Error("Thread-safe snapshot should include snapshot info")
	}

	// Test that modifying original doesn't affect snapshot
	newEntry := DirectoryEntry{
		EncryptedName: []byte("encrypted_file3"),
		CID:           "QmTest1111111111",
		Type:          FileType,
		Size:          512,
		ModifiedAt:    time.Now().UTC(),
	}

	err = manifest.AddEntry(newEntry)
	if err != nil {
		t.Fatalf("Failed to add new entry: %v", err)
	}

	// Snapshot should still have original number of entries
	if len(snapshotManifest.Entries) != 2 {
		t.Errorf("Snapshot should still have 2 entries, got %d", len(snapshotManifest.Entries))
	}

	// Test encryption/decryption of snapshot manifest
	encryptedData, err := EncryptManifest(snapshotManifest, encryptionKey)
	if err != nil {
		t.Fatalf("Failed to encrypt snapshot manifest: %v", err)
	}

	// Decrypt and verify
	decryptedManifest, err := decryptManifest(encryptedData, encryptionKey)
	if err != nil {
		t.Fatalf("Failed to decrypt snapshot manifest: %v", err)
	}

	if !decryptedManifest.IsSnapshot() {
		t.Error("Decrypted manifest should be a snapshot")
	}

	decryptedInfo := decryptedManifest.GetSnapshotInfo()
	if decryptedInfo == nil {
		t.Fatal("Decrypted snapshot info should not be nil")
	}

	if decryptedInfo.SnapshotName != snapshotName {
		t.Errorf("Expected decrypted snapshot name %s, got %s", snapshotName, decryptedInfo.SnapshotName)
	}
}

// TestNewSnapshotManifest tests the NewSnapshotManifest function
func TestNewSnapshotManifest(t *testing.T) {
	// Create original manifest
	original := NewDirectoryManifest()

	// Add test entry
	entry := DirectoryEntry{
		EncryptedName: []byte("test_file"),
		CID:           "QmTestCID",
		Type:          FileType,
		Size:          1024,
		ModifiedAt:    time.Now().UTC(),
	}

	err := original.AddEntry(entry)
	if err != nil {
		t.Fatalf("Failed to add entry: %v", err)
	}

	// Create snapshot
	originalCID := "QmOriginal"
	snapshotName := "test-snapshot"
	description := "Test description"

	snapshot := NewSnapshotManifest(original, originalCID, snapshotName, description)

	// Test snapshot properties
	if !snapshot.IsSnapshot() {
		t.Error("Created manifest should be a snapshot")
	}

	info := snapshot.GetSnapshotInfo()
	if info == nil {
		t.Fatal("Snapshot info should not be nil")
	}

	if info.OriginalCID != originalCID {
		t.Errorf("Expected original CID %s, got %s", originalCID, info.OriginalCID)
	}

	if info.SnapshotName != snapshotName {
		t.Errorf("Expected snapshot name %s, got %s", snapshotName, info.SnapshotName)
	}

	if info.Description != description {
		t.Errorf("Expected description %s, got %s", description, info.Description)
	}

	// Test that entries are copied correctly
	if len(snapshot.Entries) != len(original.Entries) {
		t.Errorf("Expected %d entries, got %d", len(original.Entries), len(snapshot.Entries))
	}

	if snapshot.Entries[0].CID != entry.CID {
		t.Errorf("Expected CID %s, got %s", entry.CID, snapshot.Entries[0].CID)
	}
}

// TestSnapshotManifestThreadSafety tests thread-safety of snapshot operations
func TestSnapshotManifestThreadSafety(t *testing.T) {
	manifest := NewDirectoryManifest()

	// Add initial entry
	entry := DirectoryEntry{
		EncryptedName: []byte("test_file"),
		CID:           "QmTestCID",
		Type:          FileType,
		Size:          1024,
		ModifiedAt:    time.Now().UTC(),
	}

	err := manifest.AddEntry(entry)
	if err != nil {
		t.Fatalf("Failed to add entry: %v", err)
	}

	// Create snapshot
	snapshot := NewSnapshotManifest(manifest, "QmOriginal", "test-snapshot", "Test description")

	// Test concurrent access to snapshot info
	done := make(chan bool)

	// Goroutine 1: repeatedly get snapshot info
	go func() {
		for i := 0; i < 100; i++ {
			info := snapshot.GetSnapshotInfo()
			if info == nil {
				t.Error("Snapshot info should not be nil")
			}
		}
		done <- true
	}()

	// Goroutine 2: repeatedly check if it's a snapshot
	go func() {
		for i := 0; i < 100; i++ {
			if !snapshot.IsSnapshot() {
				t.Error("Should be a snapshot")
			}
		}
		done <- true
	}()

	// Goroutine 3: repeatedly get thread-safe snapshot
	go func() {
		for i := 0; i < 100; i++ {
			snap := snapshot.GetSnapshot()
			if snap.SnapshotInfo == nil {
				t.Error("Thread-safe snapshot should have snapshot info")
			}
		}
		done <- true
	}()

	// Wait for all goroutines to complete
	for i := 0; i < 3; i++ {
		<-done
	}
}

// decryptManifest is a helper function to decrypt a manifest (similar to blocks package)
func decryptManifest(encryptedData []byte, key *crypto.EncryptionKey) (*DirectoryManifest, error) {
	// Decrypt the data
	decryptedData, err := crypto.Decrypt(encryptedData, key)
	if err != nil {
		return nil, err
	}

	// Unmarshal the manifest from JSON
	var manifest DirectoryManifest
	if err := json.Unmarshal(decryptedData, &manifest); err != nil {
		return nil, err
	}

	return &manifest, nil
}

package descriptors

import (
	"bytes"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/crypto"
)

func TestDirectoryManifest(t *testing.T) {
	t.Run("NewDirectoryManifest", func(t *testing.T) {
		manifest := NewDirectoryManifest()

		if manifest == nil {
			t.Fatal("NewDirectoryManifest() returned nil")
		}

		if manifest.Version != "1.0" {
			t.Errorf("Version = %v, want 1.0", manifest.Version)
		}

		if len(manifest.Entries) != 0 {
			t.Errorf("Entries length = %v, want 0", len(manifest.Entries))
		}

		if manifest.IsEmpty() != true {
			t.Error("New manifest should be empty")
		}
	})

	t.Run("AddEntry", func(t *testing.T) {
		manifest := NewDirectoryManifest()

		entry := DirectoryEntry{
			EncryptedName: []byte("encrypted-name"),
			CID:           "QmTestCID",
			Type:          FileType,
			Size:          1024,
			ModifiedAt:    time.Now(),
		}

		if err := manifest.AddEntry(entry); err != nil {
			t.Errorf("AddEntry() failed: %v", err)
		}

		if manifest.GetEntryCount() != 1 {
			t.Errorf("Entry count = %v, want 1", manifest.GetEntryCount())
		}

		if manifest.IsEmpty() {
			t.Error("Manifest with entries should not be empty")
		}

		// Test invalid entries
		invalidEntry := DirectoryEntry{
			EncryptedName: []byte{},
			CID:           "QmTestCID",
			Type:          FileType,
		}
		if err := manifest.AddEntry(invalidEntry); err == nil {
			t.Error("Should fail with empty encrypted name")
		}

		invalidEntry = DirectoryEntry{
			EncryptedName: []byte("name"),
			CID:           "",
			Type:          FileType,
		}
		if err := manifest.AddEntry(invalidEntry); err == nil {
			t.Error("Should fail with empty CID")
		}
	})

	t.Run("MarshalUnmarshal", func(t *testing.T) {
		manifest := NewDirectoryManifest()

		// Add some entries
		for i := 0; i < 3; i++ {
			entry := DirectoryEntry{
				EncryptedName: []byte("encrypted-file-" + string(rune('0'+i))),
				CID:           "QmTestCID" + string(rune('0'+i)),
				Type:          FileType,
				Size:          int64(1024 * (i + 1)),
				ModifiedAt:    time.Now(),
			}
			manifest.AddEntry(entry)
		}

		// Add a directory entry
		dirEntry := DirectoryEntry{
			EncryptedName: []byte("encrypted-subdir"),
			CID:           "QmDirCID",
			Type:          DirectoryType,
			Size:          0,
			ModifiedAt:    time.Now(),
		}
		manifest.AddEntry(dirEntry)

		// Marshal
		data, err := manifest.Marshal()
		if err != nil {
			t.Fatalf("Marshal() failed: %v", err)
		}

		// Unmarshal
		loaded, err := UnmarshalDirectoryManifest(data)
		if err != nil {
			t.Fatalf("UnmarshalDirectoryManifest() failed: %v", err)
		}

		// Verify
		if loaded.Version != manifest.Version {
			t.Errorf("Version mismatch: got %v, want %v", loaded.Version, manifest.Version)
		}

		if loaded.GetEntryCount() != manifest.GetEntryCount() {
			t.Errorf("Entry count mismatch: got %v, want %v", loaded.GetEntryCount(), manifest.GetEntryCount())
		}

		// Check entries
		for i := 0; i < len(manifest.Entries); i++ {
			if !bytes.Equal(loaded.Entries[i].EncryptedName, manifest.Entries[i].EncryptedName) {
				t.Errorf("Entry %d: encrypted name mismatch", i)
			}
			if loaded.Entries[i].CID != manifest.Entries[i].CID {
				t.Errorf("Entry %d: CID mismatch", i)
			}
			if loaded.Entries[i].Type != manifest.Entries[i].Type {
				t.Errorf("Entry %d: Type mismatch", i)
			}
			if loaded.Entries[i].Size != manifest.Entries[i].Size {
				t.Errorf("Entry %d: Size mismatch", i)
			}
		}
	})

	t.Run("Validation", func(t *testing.T) {
		manifest := NewDirectoryManifest()

		// Valid manifest
		entry := DirectoryEntry{
			EncryptedName: []byte("name"),
			CID:           "QmCID",
			Type:          FileType,
			Size:          100,
			ModifiedAt:    time.Now(),
		}
		manifest.AddEntry(entry)

		if err := manifest.Validate(); err != nil {
			t.Errorf("Valid manifest failed validation: %v", err)
		}

		// Test various invalid cases
		manifest.Version = ""
		if err := manifest.Validate(); err == nil {
			t.Error("Manifest without version should fail validation")
		}
		manifest.Version = "1.0"

		// Invalid entry type
		manifest.Entries[0].Type = "invalid"
		if err := manifest.Validate(); err == nil {
			t.Error("Manifest with invalid entry type should fail validation")
		}
		manifest.Entries[0].Type = FileType

		// Negative file size
		manifest.Entries[0].Size = -1
		if err := manifest.Validate(); err == nil {
			t.Error("File entry with negative size should fail validation")
		}
		manifest.Entries[0].Size = 100

		// Directory with non-zero size
		dirEntry := DirectoryEntry{
			EncryptedName: []byte("dir"),
			CID:           "QmDirCID",
			Type:          DirectoryType,
			Size:          100, // Should be 0
			ModifiedAt:    time.Now(),
		}
		manifest.AddEntry(dirEntry)
		if err := manifest.Validate(); err == nil {
			t.Error("Directory entry with non-zero size should fail validation")
		}
	})

	t.Run("EncryptDecryptManifest", func(t *testing.T) {
		// Create encryption key
		key, err := crypto.GenerateKey("test-password")
		if err != nil {
			t.Fatalf("Failed to generate key: %v", err)
		}

		// Create manifest with entries
		manifest := NewDirectoryManifest()
		manifest.AddEntry(DirectoryEntry{
			EncryptedName: []byte("encrypted-file1"),
			CID:           "QmFile1",
			Type:          FileType,
			Size:          1024,
			ModifiedAt:    time.Now(),
		})
		manifest.AddEntry(DirectoryEntry{
			EncryptedName: []byte("encrypted-dir1"),
			CID:           "QmDir1",
			Type:          DirectoryType,
			Size:          0,
			ModifiedAt:    time.Now(),
		})

		// Encrypt
		encrypted, err := EncryptManifest(manifest, key)
		if err != nil {
			t.Fatalf("EncryptManifest() failed: %v", err)
		}

		// Decrypt
		decrypted, err := DecryptManifest(encrypted, key)
		if err != nil {
			t.Fatalf("DecryptManifest() failed: %v", err)
		}

		// Verify
		if decrypted.GetEntryCount() != manifest.GetEntryCount() {
			t.Errorf("Entry count mismatch after encrypt/decrypt")
		}

		// Test with wrong key
		wrongKey, _ := crypto.GenerateKey("wrong-password")
		if _, err := DecryptManifest(encrypted, wrongKey); err == nil {
			t.Error("Decryption with wrong key should fail")
		}
	})
}

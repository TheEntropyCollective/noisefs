package fuse

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEncryptedFileIndex_SecurePasswordHandling(t *testing.T) {
	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "noisefs-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	indexPath := filepath.Join(tmpDir, "test-index.json")
	password := "test-password-123"

	// Test creating encrypted index with string password
	t.Run("CreateWithStringPassword", func(t *testing.T) {
		eidx, err := NewEncryptedFileIndex(indexPath, password)
		if err != nil {
			t.Fatalf("Failed to create encrypted index: %v", err)
		}
		defer eidx.Cleanup()

		// Verify the index is configured for encryption
		if !eidx.encrypted {
			t.Error("Index should be encrypted")
		}

		// Verify password is stored as bytes
		if len(eidx.password) == 0 {
			t.Error("Password bytes should not be empty")
		}

		// Verify the password bytes match the original
		if string(eidx.password) != password {
			t.Error("Password bytes don't match original password")
		}

		// Verify encryption key was generated
		if eidx.encryptionKey == nil {
			t.Error("Encryption key should be generated")
		}
		if len(eidx.encryptionKey.Key) != 32 {
			t.Error("Encryption key should be 32 bytes")
		}
		if len(eidx.encryptionKey.Salt) != 32 {
			t.Error("Salt should be 32 bytes")
		}
	})

	// Test creating encrypted index with byte password
	t.Run("CreateWithBytePassword", func(t *testing.T) {
		passwordBytes := SecurePasswordBytes(password)
		defer SecureZeroMemory(passwordBytes)

		eidx, err := NewEncryptedFileIndexFromBytes(indexPath, passwordBytes)
		if err != nil {
			t.Fatalf("Failed to create encrypted index from bytes: %v", err)
		}
		defer eidx.Cleanup()

		// Verify the index is configured for encryption
		if !eidx.encrypted {
			t.Error("Index should be encrypted")
		}

		// Verify password is stored as bytes
		if len(eidx.password) == 0 {
			t.Error("Password bytes should not be empty")
		}

		// Verify the password bytes match the original
		if string(eidx.password) != password {
			t.Error("Password bytes don't match original password")
		}
	})

	// Test secure password cleanup
	t.Run("SecureCleanup", func(t *testing.T) {
		eidx, err := NewEncryptedFileIndex(indexPath, password)
		if err != nil {
			t.Fatalf("Failed to create encrypted index: %v", err)
		}

		// Verify password is stored
		if len(eidx.password) == 0 {
			t.Error("Password bytes should not be empty before cleanup")
		}

		// Call cleanup
		eidx.Cleanup()

		// Verify password bytes are cleared (should be all zeros or nil)
		if eidx.password != nil {
			for i, b := range eidx.password {
				if b != 0 {
					t.Errorf("Password byte at index %d should be zero after cleanup, got %d", i, b)
				}
			}
		}

		// Verify encryption key is cleared
		if eidx.encryptionKey != nil {
			for i, b := range eidx.encryptionKey.Key {
				if b != 0 {
					t.Errorf("Encryption key byte at index %d should be zero after cleanup, got %d", i, b)
				}
			}
			for i, b := range eidx.encryptionKey.Salt {
				if b != 0 {
					t.Errorf("Salt byte at index %d should be zero after cleanup, got %d", i, b)
				}
			}
		}
	})

	// Test empty password handling
	t.Run("EmptyPassword", func(t *testing.T) {
		eidx, err := NewEncryptedFileIndex(indexPath, "")
		if err != nil {
			t.Fatalf("Failed to create unencrypted index: %v", err)
		}
		defer eidx.Cleanup()

		// Verify the index is not encrypted
		if eidx.encrypted {
			t.Error("Index should not be encrypted with empty password")
		}

		// Verify no password is stored
		if len(eidx.password) != 0 {
			t.Error("Password bytes should be empty for unencrypted index")
		}

		// Verify no encryption key is generated
		if eidx.encryptionKey != nil {
			t.Error("Encryption key should not be generated for unencrypted index")
		}
	})
}

func TestEncryptedFileIndex_SaveAndLoad(t *testing.T) {
	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "noisefs-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	indexPath := filepath.Join(tmpDir, "test-index.json")
	password := "test-password-save-load"

	// Create and populate encrypted index
	eidx1, err := NewEncryptedFileIndex(indexPath, password)
	if err != nil {
		t.Fatalf("Failed to create encrypted index: %v", err)
	}

	// Add test data
	eidx1.AddFile("test-file.txt", "QmTestCID123", 1024)
	eidx1.AddDirectory("test-dir", "QmTestDirCID456", "test-key-id")

	// Save the index
	err = eidx1.SaveIndex()
	if err != nil {
		t.Fatalf("Failed to save encrypted index: %v", err)
	}
	eidx1.Cleanup()

	// Create new index instance and load
	eidx2, err := NewEncryptedFileIndex(indexPath, password)
	if err != nil {
		t.Fatalf("Failed to create new encrypted index: %v", err)
	}
	defer eidx2.Cleanup()

	err = eidx2.LoadIndex()
	if err != nil {
		t.Fatalf("Failed to load encrypted index: %v", err)
	}

	// Verify data was loaded correctly
	file, exists := eidx2.GetFile("test-file.txt")
	if !exists {
		t.Error("Test file should exist after loading")
	} else {
		if file.DescriptorCID != "QmTestCID123" {
			t.Errorf("Expected descriptor CID QmTestCID123, got %s", file.DescriptorCID)
		}
		if file.FileSize != 1024 {
			t.Errorf("Expected file size 1024, got %d", file.FileSize)
		}
	}

	dir, exists := eidx2.GetDirectory("test-dir")
	if !exists {
		t.Error("Test directory should exist after loading")
	} else {
		if dir.DirectoryDescriptorCID != "QmTestDirCID456" {
			t.Errorf("Expected directory CID QmTestDirCID456, got %s", dir.DirectoryDescriptorCID)
		}
	}
}

func TestEncryptedFileIndex_WrongPassword(t *testing.T) {
	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "noisefs-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	indexPath := filepath.Join(tmpDir, "test-index.json")
	correctPassword := "correct-password"
	wrongPassword := "wrong-password"

	// Create and save encrypted index with correct password
	eidx1, err := NewEncryptedFileIndex(indexPath, correctPassword)
	if err != nil {
		t.Fatalf("Failed to create encrypted index: %v", err)
	}

	eidx1.AddFile("test-file.txt", "QmTestCID123", 1024)
	err = eidx1.SaveIndex()
	if err != nil {
		t.Fatalf("Failed to save encrypted index: %v", err)
	}
	eidx1.Cleanup()

	// Read the file to see what was actually written
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("Failed to read saved file: %v", err)
	}
	t.Logf("Saved encrypted file content length: %d bytes", len(data))
	t.Logf("File starts with: %s", string(data[:min(100, len(data))]))

	// Try to load with wrong password
	eidx2, err := NewEncryptedFileIndex(indexPath, wrongPassword)
	if err != nil {
		t.Fatalf("Failed to create encrypted index: %v", err)
	}
	defer eidx2.Cleanup()

	err = eidx2.LoadIndex()
	if err == nil {
		// Check if any data was actually loaded
		files := eidx2.ListFiles()
		t.Errorf("Loading with wrong password should fail, but got %d files: %v", len(files), files)
	} else {
		t.Logf("Got expected error: %v", err)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestSecurePasswordBytes(t *testing.T) {
	password := "test-password"

	// Test normal password
	passwordBytes := SecurePasswordBytes(password)
	if passwordBytes == nil {
		t.Error("Password bytes should not be nil for non-empty password")
	}
	if string(passwordBytes) != password {
		t.Error("Password bytes should match original password")
	}

	// Test empty password
	emptyBytes := SecurePasswordBytes("")
	if emptyBytes != nil {
		t.Error("Password bytes should be nil for empty password")
	}

	// Test that we can securely clear the bytes
	SecureZeroMemory(passwordBytes)
	for i, b := range passwordBytes {
		if b != 0 {
			t.Errorf("Password byte at index %d should be zero after clearing, got %d", i, b)
		}
	}
}

func TestSecureZeroMemory(t *testing.T) {
	// Test with non-empty slice
	data := []byte("sensitive-data-123")
	original := make([]byte, len(data))
	copy(original, data)

	SecureZeroMemory(data)

	// Verify all bytes are zero
	for i, b := range data {
		if b != 0 {
			t.Errorf("Byte at index %d should be zero after SecureZeroMemory, got %d", i, b)
		}
	}

	// Verify original data was actually different
	allZero := true
	for _, b := range original {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("Original data should not be all zeros (test setup issue)")
	}

	// Test with empty slice
	SecureZeroMemory([]byte{})
	SecureZeroMemory(nil)
	// Should not panic
}

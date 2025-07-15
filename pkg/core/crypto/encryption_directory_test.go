package crypto

import (
	"bytes"
	"testing"
)

func TestDeriveDirectoryKey(t *testing.T) {
	// Generate master key
	masterKey, err := GenerateKey("master-password")
	if err != nil {
		t.Fatalf("Failed to generate master key: %v", err)
	}
	
	t.Run("BasicDerivation", func(t *testing.T) {
		dirPath := "/home/user/documents"
		
		derivedKey, err := DeriveDirectoryKey(masterKey, dirPath)
		if err != nil {
			t.Fatalf("DeriveDirectoryKey() failed: %v", err)
		}
		
		if derivedKey == nil {
			t.Fatal("Derived key is nil")
		}
		
		if len(derivedKey.Key) != 32 {
			t.Errorf("Derived key length = %v, want 32", len(derivedKey.Key))
		}
		
		// Should use same salt as master
		if !bytes.Equal(derivedKey.Salt, masterKey.Salt) {
			t.Error("Derived key should use master key's salt")
		}
		
		// Key should be different from master
		if bytes.Equal(derivedKey.Key, masterKey.Key) {
			t.Error("Derived key should be different from master key")
		}
	})
	
	t.Run("DifferentPaths", func(t *testing.T) {
		path1 := "/home/user/documents"
		path2 := "/home/user/pictures"
		
		key1, err := DeriveDirectoryKey(masterKey, path1)
		if err != nil {
			t.Fatalf("Failed to derive key for path1: %v", err)
		}
		
		key2, err := DeriveDirectoryKey(masterKey, path2)
		if err != nil {
			t.Fatalf("Failed to derive key for path2: %v", err)
		}
		
		// Keys should be different for different paths
		if bytes.Equal(key1.Key, key2.Key) {
			t.Error("Keys for different paths should be different")
		}
	})
	
	t.Run("Deterministic", func(t *testing.T) {
		dirPath := "/home/user/data"
		
		key1, err := DeriveDirectoryKey(masterKey, dirPath)
		if err != nil {
			t.Fatalf("First derivation failed: %v", err)
		}
		
		key2, err := DeriveDirectoryKey(masterKey, dirPath)
		if err != nil {
			t.Fatalf("Second derivation failed: %v", err)
		}
		
		// Same path should produce same key
		if !bytes.Equal(key1.Key, key2.Key) {
			t.Error("Same path should produce same derived key")
		}
	})
	
	t.Run("InvalidInput", func(t *testing.T) {
		// Nil master key
		if _, err := DeriveDirectoryKey(nil, "/path"); err == nil {
			t.Error("Should fail with nil master key")
		}
		
		// Empty master key
		emptyKey := &EncryptionKey{Key: []byte{}, Salt: []byte{}}
		if _, err := DeriveDirectoryKey(emptyKey, "/path"); err == nil {
			t.Error("Should fail with empty master key")
		}
	})
}

func TestFileNameEncryption(t *testing.T) {
	// Generate directory key
	masterKey, err := GenerateKey("test-password")
	if err != nil {
		t.Fatalf("Failed to generate master key: %v", err)
	}
	
	dirKey, err := DeriveDirectoryKey(masterKey, "/test/directory")
	if err != nil {
		t.Fatalf("Failed to derive directory key: %v", err)
	}
	
	t.Run("EncryptDecrypt", func(t *testing.T) {
		filename := "secret-document.txt"
		
		// Encrypt
		encrypted, err := EncryptFileName(filename, dirKey)
		if err != nil {
			t.Fatalf("EncryptFileName() failed: %v", err)
		}
		
		if len(encrypted) == 0 {
			t.Error("Encrypted filename is empty")
		}
		
		// Decrypt
		decrypted, err := DecryptFileName(encrypted, dirKey)
		if err != nil {
			t.Fatalf("DecryptFileName() failed: %v", err)
		}
		
		if decrypted != filename {
			t.Errorf("Decrypted filename = %v, want %v", decrypted, filename)
		}
	})
	
	t.Run("DifferentKeys", func(t *testing.T) {
		filename := "confidential.pdf"
		
		// Encrypt with one key
		encrypted, err := EncryptFileName(filename, dirKey)
		if err != nil {
			t.Fatalf("Encryption failed: %v", err)
		}
		
		// Try to decrypt with different key
		otherDirKey, _ := DeriveDirectoryKey(masterKey, "/other/directory")
		if _, err := DecryptFileName(encrypted, otherDirKey); err == nil {
			t.Error("Decryption with wrong key should fail")
		}
	})
	
	t.Run("UniqueEncryptions", func(t *testing.T) {
		filename := "test.txt"
		
		// Encrypt same filename twice
		encrypted1, err := EncryptFileName(filename, dirKey)
		if err != nil {
			t.Fatalf("First encryption failed: %v", err)
		}
		
		encrypted2, err := EncryptFileName(filename, dirKey)
		if err != nil {
			t.Fatalf("Second encryption failed: %v", err)
		}
		
		// Should produce different ciphertexts (due to random nonce)
		if bytes.Equal(encrypted1, encrypted2) {
			t.Error("Same filename should produce different ciphertexts")
		}
		
		// But both should decrypt to same filename
		decrypted1, _ := DecryptFileName(encrypted1, dirKey)
		decrypted2, _ := DecryptFileName(encrypted2, dirKey)
		
		if decrypted1 != decrypted2 || decrypted1 != filename {
			t.Error("Both ciphertexts should decrypt to same filename")
		}
	})
	
	t.Run("InvalidInput", func(t *testing.T) {
		// Empty filename
		if _, err := EncryptFileName("", dirKey); err == nil {
			t.Error("Should fail with empty filename")
		}
		
		// Empty encrypted name
		if _, err := DecryptFileName([]byte{}, dirKey); err == nil {
			t.Error("Should fail with empty encrypted name")
		}
		
		// Nil key
		if _, err := EncryptFileName("test", nil); err == nil {
			t.Error("Should fail with nil key")
		}
	})
	
	t.Run("SpecialCharacters", func(t *testing.T) {
		// Test with various special characters
		filenames := []string{
			"file with spaces.txt",
			"文件名.pdf",
			"file@#$%^&*.doc",
			"very/long/path/with/many/slashes.jpg",
			"émojis-😀🎉.txt",
		}
		
		for _, filename := range filenames {
			encrypted, err := EncryptFileName(filename, dirKey)
			if err != nil {
				t.Errorf("Failed to encrypt %q: %v", filename, err)
				continue
			}
			
			decrypted, err := DecryptFileName(encrypted, dirKey)
			if err != nil {
				t.Errorf("Failed to decrypt %q: %v", filename, err)
				continue
			}
			
			if decrypted != filename {
				t.Errorf("Filename mismatch: got %q, want %q", decrypted, filename)
			}
		}
	})
}
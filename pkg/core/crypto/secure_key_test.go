package crypto

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestGenerateSecureSyncKey(t *testing.T) {
	t.Run("BasicGeneration", func(t *testing.T) {
		sessionID := "test-session-123"
		userSalt := make([]byte, 16)
		rand.Read(userSalt)

		key, err := GenerateSecureSyncKey(sessionID, userSalt)
		if err != nil {
			t.Fatalf("GenerateSecureSyncKey() failed: %v", err)
		}

		if key == nil {
			t.Fatal("Generated key is nil")
		}

		if len(key.Key) != 32 {
			t.Errorf("Key length = %v, want 32", len(key.Key))
		}

		if len(key.Salt) != 32 {
			t.Errorf("Salt length = %v, want 32", len(key.Salt))
		}
	})

	t.Run("KeyRandomness", func(t *testing.T) {
		sessionID := "randomness-test"
		userSalt := make([]byte, 16)
		rand.Read(userSalt)

		// Generate multiple keys and verify they are different
		keys := make([]*EncryptionKey, 10)
		for i := 0; i < 10; i++ {
			key, err := GenerateSecureSyncKey(sessionID, userSalt)
			if err != nil {
				t.Fatalf("Key generation %d failed: %v", i, err)
			}
			keys[i] = key
		}

		// Compare all keys to ensure they are different
		for i := 0; i < len(keys); i++ {
			for j := i + 1; j < len(keys); j++ {
				if bytes.Equal(keys[i].Key, keys[j].Key) {
					t.Errorf("Keys %d and %d are identical (should be different due to entropy)", i, j)
				}
			}
		}
	})

	t.Run("SessionIDUniqueness", func(t *testing.T) {
		userSalt := make([]byte, 16)
		rand.Read(userSalt)

		sessionID1 := "session-001"
		sessionID2 := "session-002"

		key1, err := GenerateSecureSyncKey(sessionID1, userSalt)
		if err != nil {
			t.Fatalf("First key generation failed: %v", err)
		}

		key2, err := GenerateSecureSyncKey(sessionID2, userSalt)
		if err != nil {
			t.Fatalf("Second key generation failed: %v", err)
		}

		// Keys should be different for different session IDs
		if bytes.Equal(key1.Key, key2.Key) {
			t.Error("Keys for different session IDs should be different")
		}
	})

	t.Run("SaltVariations", func(t *testing.T) {
		sessionID := "salt-test"

		// Test with different salts
		salt1 := make([]byte, 16)
		salt2 := make([]byte, 16)
		rand.Read(salt1)
		rand.Read(salt2)

		key1, err := GenerateSecureSyncKey(sessionID, salt1)
		if err != nil {
			t.Fatalf("First key generation failed: %v", err)
		}

		key2, err := GenerateSecureSyncKey(sessionID, salt2)
		if err != nil {
			t.Fatalf("Second key generation failed: %v", err)
		}

		// Keys should be different for different salts
		if bytes.Equal(key1.Key, key2.Key) {
			t.Error("Keys for different salts should be different")
		}
	})

	t.Run("NoUserSalt", func(t *testing.T) {
		sessionID := "no-salt-test"

		key, err := GenerateSecureSyncKey(sessionID, nil)
		if err != nil {
			t.Fatalf("Key generation with nil salt failed: %v", err)
		}

		if key == nil {
			t.Fatal("Generated key is nil")
		}

		if len(key.Key) != 32 {
			t.Errorf("Key length = %v, want 32", len(key.Key))
		}
	})

	t.Run("InvalidInput", func(t *testing.T) {
		// Empty session ID should fail
		if _, err := GenerateSecureSyncKey("", nil); err == nil {
			t.Error("Should fail with empty session ID")
		}
	})

	t.Run("EntropyValidation", func(t *testing.T) {
		sessionID := "entropy-test"
		userSalt := make([]byte, 16)
		rand.Read(userSalt)

		// Generate multiple keys and check for entropy
		keyData := make([][]byte, 50)
		for i := 0; i < 50; i++ {
			key, err := GenerateSecureSyncKey(sessionID, userSalt)
			if err != nil {
				t.Fatalf("Key generation %d failed: %v", i, err)
			}
			keyData[i] = key.Key
		}

		// Basic entropy check: verify keys have varied byte distributions
		for _, keyBytes := range keyData {
			zeroCount := 0
			for _, b := range keyBytes {
				if b == 0 {
					zeroCount++
				}
			}

			// A truly random 32-byte key should not have more than ~75% zeros
			if zeroCount > 24 {
				t.Errorf("Key has too many zero bytes (%d/32), indicating poor entropy", zeroCount)
			}
		}
	})
}

func TestGenerateSecureSyncKeyWithRotation(t *testing.T) {
	t.Run("BasicRotation", func(t *testing.T) {
		sessionID := "rotation-test"
		userSalt := make([]byte, 16)
		rand.Read(userSalt)

		key1, err := GenerateSecureSyncKeyWithRotation(sessionID, userSalt, 0)
		if err != nil {
			t.Fatalf("First rotation key failed: %v", err)
		}

		key2, err := GenerateSecureSyncKeyWithRotation(sessionID, userSalt, 1)
		if err != nil {
			t.Fatalf("Second rotation key failed: %v", err)
		}

		// Keys should be different for different rotation counters
		if bytes.Equal(key1.Key, key2.Key) {
			t.Error("Rotation keys should be different")
		}
	})

	t.Run("RotationConsistency", func(t *testing.T) {
		sessionID := "consistency-test"
		userSalt := make([]byte, 16)
		rand.Read(userSalt)
		rotationCounter := uint32(5)

		// Generate same rotation key multiple times
		key1, err := GenerateSecureSyncKeyWithRotation(sessionID, userSalt, rotationCounter)
		if err != nil {
			t.Fatalf("First key generation failed: %v", err)
		}

		key2, err := GenerateSecureSyncKeyWithRotation(sessionID, userSalt, rotationCounter)
		if err != nil {
			t.Fatalf("Second key generation failed: %v", err)
		}

		// Keys should be different (due to timestamp and entropy)
		// but this tests the function doesn't crash with same params
		if key1 == nil || key2 == nil {
			t.Error("Rotation keys should not be nil")
		}
	})

	t.Run("InvalidRotationInput", func(t *testing.T) {
		userSalt := make([]byte, 16)
		rand.Read(userSalt)

		// Empty session ID should fail
		if _, err := GenerateSecureSyncKeyWithRotation("", userSalt, 0); err == nil {
			t.Error("Should fail with empty session ID")
		}
	})
}

func TestSecureKeyCompatibility(t *testing.T) {
	t.Run("EncryptionCompatibility", func(t *testing.T) {
		sessionID := "compat-test"
		userSalt := make([]byte, 16)
		rand.Read(userSalt)

		// Generate secure key
		key, err := GenerateSecureSyncKey(sessionID, userSalt)
		if err != nil {
			t.Fatalf("Key generation failed: %v", err)
		}

		// Test that the key works with existing encryption functions
		testData := []byte("test data for encryption compatibility")

		encrypted, err := Encrypt(testData, key)
		if err != nil {
			t.Fatalf("Encryption with secure key failed: %v", err)
		}

		decrypted, err := Decrypt(encrypted, key)
		if err != nil {
			t.Fatalf("Decryption with secure key failed: %v", err)
		}

		if !bytes.Equal(testData, decrypted) {
			t.Error("Decrypted data does not match original")
		}
	})

	t.Run("DirectoryKeyDerivation", func(t *testing.T) {
		sessionID := "dir-derive-test"
		userSalt := make([]byte, 16)
		rand.Read(userSalt)

		// Generate secure master key
		masterKey, err := GenerateSecureSyncKey(sessionID, userSalt)
		if err != nil {
			t.Fatalf("Master key generation failed: %v", err)
		}

		// Test directory key derivation
		dirPath := "/test/directory"
		dirKey, err := DeriveDirectoryKey(masterKey, dirPath)
		if err != nil {
			t.Fatalf("Directory key derivation failed: %v", err)
		}

		if dirKey == nil {
			t.Fatal("Directory key is nil")
		}

		if len(dirKey.Key) != 32 {
			t.Errorf("Directory key length = %v, want 32", len(dirKey.Key))
		}

		// Keys should be different
		if bytes.Equal(masterKey.Key, dirKey.Key) {
			t.Error("Directory key should be different from master key")
		}
	})
}

// BenchmarkSecureKeyGeneration benchmarks the performance of secure key generation
func BenchmarkSecureKeyGeneration(b *testing.B) {
	sessionID := "benchmark-session"
	userSalt := make([]byte, 16)
	rand.Read(userSalt)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := GenerateSecureSyncKey(sessionID, userSalt)
		if err != nil {
			b.Fatalf("Key generation failed: %v", err)
		}
	}
}

func BenchmarkSecureKeyWithRotation(b *testing.B) {
	sessionID := "rotation-benchmark"
	userSalt := make([]byte, 16)
	rand.Read(userSalt)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := GenerateSecureSyncKeyWithRotation(sessionID, userSalt, uint32(i))
		if err != nil {
			b.Fatalf("Rotation key generation failed: %v", err)
		}
	}
}

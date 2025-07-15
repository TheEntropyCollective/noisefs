package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/hkdf"
)

// EncryptionKey represents an encryption key with metadata
type EncryptionKey struct {
	Key  []byte
	Salt []byte
}

// GenerateKey generates a new encryption key from a password using Argon2id
func GenerateKey(password string) (*EncryptionKey, error) {
	// Generate random salt
	salt := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	// Derive key using Argon2id
	key := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)

	return &EncryptionKey{
		Key:  key,
		Salt: salt,
	}, nil
}

// DeriveKey derives an encryption key from a password and existing salt
func DeriveKey(password string, salt []byte) (*EncryptionKey, error) {
	if len(salt) != 32 {
		return nil, fmt.Errorf("salt must be 32 bytes")
	}

	// Derive key using Argon2id
	key := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)

	return &EncryptionKey{
		Key:  key,
		Salt: salt,
	}, nil
}

// Encrypt encrypts data using AES-256-GCM
func Encrypt(data []byte, key *EncryptionKey) ([]byte, error) {
	// Create AES cipher
	block, err := aes.NewCipher(key.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate random nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt data
	ciphertext := gcm.Seal(nonce, nonce, data, nil)

	return ciphertext, nil
}

// Decrypt decrypts data using AES-256-GCM
func Decrypt(ciphertext []byte, key *EncryptionKey) ([]byte, error) {
	// Create AES cipher
	block, err := aes.NewCipher(key.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Check minimum length
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	// Extract nonce and ciphertext
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	// Decrypt data
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

// HashPassword creates a bcrypt hash of a password for verification
// Uses a cost factor of 12 for good security vs performance balance
func HashPassword(password string) string {
	// Use cost factor of 12 as specified
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		// In case of error, return empty string to maintain interface
		// The caller should check for empty string
		return ""
	}
	return string(hash)
}

// VerifyPassword verifies a password against its bcrypt hash
func VerifyPassword(password, hash string) bool {
	// CompareHashAndPassword returns nil on success
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// SecureRandom generates cryptographically secure random bytes
func SecureRandom(size int) ([]byte, error) {
	bytes := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return bytes, nil
}

// SecureZero securely clears sensitive data from memory
func SecureZero(data []byte) {
	for i := range data {
		data[i] = 0
	}
}

// DeriveDirectoryKey derives a directory-specific encryption key using HKDF
func DeriveDirectoryKey(masterKey *EncryptionKey, directoryPath string) (*EncryptionKey, error) {
	if masterKey == nil || len(masterKey.Key) == 0 {
		return nil, fmt.Errorf("master key is required")
	}
	
	// Use HKDF to derive a directory-specific key
	info := []byte("noisefs-directory:" + directoryPath)
	hkdf := hkdf.New(sha256.New, masterKey.Key, masterKey.Salt, info)
	
	// Derive a 32-byte key
	derivedKey := make([]byte, 32)
	if _, err := io.ReadFull(hkdf, derivedKey); err != nil {
		return nil, fmt.Errorf("failed to derive directory key: %w", err)
	}
	
	return &EncryptionKey{
		Key:  derivedKey,
		Salt: masterKey.Salt, // Reuse the master key's salt
	}, nil
}

// EncryptFileName encrypts a filename using AES-256-GCM with a directory-specific key
func EncryptFileName(filename string, dirKey *EncryptionKey) ([]byte, error) {
	if filename == "" {
		return nil, fmt.Errorf("filename cannot be empty")
	}
	
	if dirKey == nil {
		return nil, fmt.Errorf("directory key cannot be nil")
	}
	
	return Encrypt([]byte(filename), dirKey)
}

// DecryptFileName decrypts a filename using AES-256-GCM with a directory-specific key
func DecryptFileName(encryptedName []byte, dirKey *EncryptionKey) (string, error) {
	if len(encryptedName) == 0 {
		return "", fmt.Errorf("encrypted name cannot be empty")
	}
	
	if dirKey == nil {
		return "", fmt.Errorf("directory key cannot be nil")
	}
	
	decrypted, err := Decrypt(encryptedName, dirKey)
	if err != nil {
		return "", err
	}
	
	return string(decrypted), nil
}
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/pbkdf2"
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
func HashPassword(password string) (string, error) {
	// Use cost factor of 12 as specified
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(hash), nil
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

// String returns a base64-encoded string representation of the encryption key
func (k *EncryptionKey) String() string {
	keyData := struct {
		Key  string `json:"key"`
		Salt string `json:"salt"`
	}{
		Key:  base64.StdEncoding.EncodeToString(k.Key),
		Salt: base64.StdEncoding.EncodeToString(k.Salt),
	}
	
	data, err := json.Marshal(keyData)
	if err != nil {
		return ""
	}
	
	return base64.StdEncoding.EncodeToString(data)
}

// MarshalText implements the encoding.TextMarshaler interface
func (k *EncryptionKey) MarshalText() ([]byte, error) {
	return []byte(k.String()), nil
}

// UnmarshalText implements the encoding.TextUnmarshaler interface
func (k *EncryptionKey) UnmarshalText(text []byte) error {
	parsed, err := ParseKeyFromString(string(text))
	if err != nil {
		return err
	}
	
	k.Key = parsed.Key
	k.Salt = parsed.Salt
	return nil
}

// ParseKeyFromString parses a base64-encoded string representation back to an EncryptionKey
func ParseKeyFromString(keyStr string) (*EncryptionKey, error) {
	if keyStr == "" {
		return nil, fmt.Errorf("key string cannot be empty")
	}
	
	// Decode base64 string
	data, err := base64.StdEncoding.DecodeString(keyStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode key string: %w", err)
	}
	
	// Parse JSON
	var keyData struct {
		Key  string `json:"key"`
		Salt string `json:"salt"`
	}
	
	if err := json.Unmarshal(data, &keyData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal key data: %w", err)
	}
	
	// Decode key and salt
	key, err := base64.StdEncoding.DecodeString(keyData.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to decode key: %w", err)
	}
	
	salt, err := base64.StdEncoding.DecodeString(keyData.Salt)
	if err != nil {
		return nil, fmt.Errorf("failed to decode salt: %w", err)
	}
	
	return &EncryptionKey{
		Key:  key,
		Salt: salt,
	}, nil
}

// GenerateSecureSyncKey generates a cryptographically secure key for sync operations
// using proper entropy from crypto/rand instead of hardcoded parameters
func GenerateSecureSyncKey(sessionID string, userSalt []byte) (*EncryptionKey, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session ID cannot be empty")
	}

	// Generate random entropy using crypto/rand
	entropy := make([]byte, 32)
	if _, err := rand.Read(entropy); err != nil {
		return nil, fmt.Errorf("failed to generate entropy: %w", err)
	}

	// Create session-specific salt by combining user salt with session ID and timestamp
	var salt []byte
	if len(userSalt) > 0 {
		salt = append(salt, userSalt...)
	} else {
		// Generate random salt if none provided
		randomSalt := make([]byte, 16)
		if _, err := rand.Read(randomSalt); err != nil {
			return nil, fmt.Errorf("failed to generate salt: %w", err)
		}
		salt = randomSalt
	}

	// Add session ID and timestamp for uniqueness
	salt = append(salt, []byte(sessionID)...)
	timestamp := time.Now().UnixNano()
	timestampBytes := make([]byte, 8)
	for i := 0; i < 8; i++ {
		timestampBytes[i] = byte(timestamp >> (8 * i))
	}
	salt = append(salt, timestampBytes...)

	// Derive key using PBKDF2 with secure parameters
	key := pbkdf2.Key(entropy, salt, 100000, 32, sha256.New)

	return &EncryptionKey{
		Key:  key,
		Salt: salt[:32], // Keep salt to standard 32-byte length
	}, nil
}

// GenerateSecureSyncKeyWithRotation generates a secure sync key with rotation support
func GenerateSecureSyncKeyWithRotation(sessionID string, userSalt []byte, rotationCounter uint32) (*EncryptionKey, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session ID cannot be empty")
	}

	// Add rotation counter to session ID for key versioning
	rotationID := fmt.Sprintf("%s-rotation-%d", sessionID, rotationCounter)
	
	return GenerateSecureSyncKey(rotationID, userSalt)
}
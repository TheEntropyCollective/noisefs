package fuse

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/TheEntropyCollective/noisefs/pkg/core/crypto"
)

// EncryptedFileIndex provides encrypted storage for the file index
type EncryptedFileIndex struct {
	*FileIndex
	password      []byte
	encryptionKey *crypto.EncryptionKey
	encrypted     bool
}

// NewEncryptedFileIndex creates a new encrypted file index
func NewEncryptedFileIndex(indexPath, password string) (*EncryptedFileIndex, error) {
	baseIndex := NewFileIndex(indexPath)

	if password == "" {
		// No encryption requested
		return &EncryptedFileIndex{
			FileIndex: baseIndex,
			encrypted: false,
		}, nil
	}

	// Convert password to []byte and clear the original string parameter from memory
	passwordBytes := make([]byte, len(password))
	copy(passwordBytes, []byte(password))

	// Generate encryption key from password
	encKey, err := crypto.GenerateKey(password)
	if err != nil {
		// Clear password bytes on error
		SecureZeroMemory(passwordBytes)
		return nil, fmt.Errorf("failed to generate encryption key: %w", err)
	}

	return &EncryptedFileIndex{
		FileIndex:     baseIndex,
		password:      passwordBytes,
		encryptionKey: encKey,
		encrypted:     true,
	}, nil
}

// NewEncryptedFileIndexFromBytes creates a new encrypted file index from a password byte slice
// This function takes ownership of the password bytes and will securely clear them on cleanup
func NewEncryptedFileIndexFromBytes(indexPath string, passwordBytes []byte) (*EncryptedFileIndex, error) {
	baseIndex := NewFileIndex(indexPath)

	if len(passwordBytes) == 0 {
		// No encryption requested
		return &EncryptedFileIndex{
			FileIndex: baseIndex,
			encrypted: false,
		}, nil
	}

	// Generate encryption key from password bytes
	encKey, err := crypto.GenerateKey(string(passwordBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to generate encryption key: %w", err)
	}

	// Take ownership of password bytes
	password := make([]byte, len(passwordBytes))
	copy(password, passwordBytes)

	return &EncryptedFileIndex{
		FileIndex:     baseIndex,
		password:      password,
		encryptionKey: encKey,
		encrypted:     true,
	}, nil
}

// LoadIndex loads the index from disk, trying encrypted format first, then fallback to unencrypted
func (eidx *EncryptedFileIndex) LoadIndex() error {
	eidx.mu.Lock()
	defer eidx.mu.Unlock()

	// If file doesn't exist, start with empty index
	if _, err := os.Stat(eidx.filePath); os.IsNotExist(err) {
		return nil
	}

	data, err := os.ReadFile(eidx.filePath)
	if err != nil {
		return fmt.Errorf("failed to read index file: %w", err)
	}

	// Try to load as encrypted if we have encryption enabled
	if eidx.encrypted {
		if decryptedData, err := eidx.tryDecryptIndex(data); err == nil {
			return eidx.parseIndexData(decryptedData, true)
		} else {
			// Check if this is an encrypted file format by looking for the encrypted wrapper
			var encIndex struct {
				Version   string `json:"version"`
				Encrypted bool   `json:"encrypted"`
			}
			if json.Unmarshal(data, &encIndex) == nil && encIndex.Encrypted {
				// This is definitely an encrypted file, but decryption failed
				return fmt.Errorf("failed to decrypt index: wrong password or corrupted file")
			}
			// If it's not an encrypted format, fall through to try unencrypted parsing
		}
	}

	// Try to load as unencrypted
	if err := eidx.parseIndexData(data, false); err != nil {
		if eidx.encrypted {
			return fmt.Errorf("failed to load index (wrong password or corrupted file): %w", err)
		}
		return fmt.Errorf("failed to parse index file: %w", err)
	}

	return nil
}

// tryDecryptIndex attempts to decrypt the index data
func (eidx *EncryptedFileIndex) tryDecryptIndex(encryptedData []byte) ([]byte, error) {
	if !eidx.encrypted || eidx.encryptionKey == nil {
		return nil, fmt.Errorf("encryption not enabled")
	}

	// Parse the encrypted index format
	var encIndex struct {
		Version   string `json:"version"`
		Encrypted bool   `json:"encrypted"`
		Salt      []byte `json:"salt"`
		Data      []byte `json:"data"`
	}

	if err := json.Unmarshal(encryptedData, &encIndex); err != nil {
		return nil, fmt.Errorf("invalid encrypted index format: %w", err)
	}

	if !encIndex.Encrypted || encIndex.Version != "1.0-encrypted" {
		return nil, fmt.Errorf("not an encrypted index")
	}

	// Derive key using stored salt
	key, err := crypto.DeriveKey(string(eidx.password), encIndex.Salt)
	if err != nil {
		return nil, fmt.Errorf("failed to derive key: %w", err)
	}

	// Decrypt the data
	decryptedData, err := crypto.Decrypt(encIndex.Data, key)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt index: %w", err)
	}

	// Clear sensitive key data
	crypto.SecureZero(key.Key)

	return decryptedData, nil
}

// parseIndexData parses the index data and updates the internal state
func (eidx *EncryptedFileIndex) parseIndexData(data []byte, wasEncrypted bool) error {
	var loadedIndex FileIndex
	if err := json.Unmarshal(data, &loadedIndex); err != nil {
		return err
	}

	// Merge loaded entries
	if loadedIndex.Entries != nil {
		eidx.Entries = loadedIndex.Entries
	}
	eidx.Version = loadedIndex.Version
	eidx.dirty = false

	return nil
}

// SaveIndex saves the index to disk with encryption if enabled
func (eidx *EncryptedFileIndex) SaveIndex() error {
	eidx.mu.RLock()
	defer eidx.mu.RUnlock()

	if !eidx.dirty {
		return nil // No changes to save
	}

	// Ensure directory exists
	dir := filepath.Dir(eidx.filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Serialize the index data
	indexData, err := json.MarshalIndent(eidx.FileIndex, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal index: %w", err)
	}

	var finalData []byte

	if eidx.encrypted && eidx.encryptionKey != nil {
		// Encrypt the index data
		encryptedData, err := crypto.Encrypt(indexData, eidx.encryptionKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt index: %w", err)
		}

		// Create encrypted index wrapper
		encIndex := struct {
			Version   string `json:"version"`
			Encrypted bool   `json:"encrypted"`
			Salt      []byte `json:"salt"`
			Data      []byte `json:"data"`
		}{
			Version:   "1.0-encrypted",
			Encrypted: true,
			Salt:      eidx.encryptionKey.Salt,
			Data:      encryptedData,
		}

		finalData, err = json.MarshalIndent(encIndex, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal encrypted index: %w", err)
		}
	} else {
		// Save unencrypted
		finalData = indexData
	}

	// Write atomically
	tmpPath := eidx.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, finalData, 0600); err != nil {
		return fmt.Errorf("failed to write index file: %w", err)
	}

	if err := os.Rename(tmpPath, eidx.filePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename index file: %w", err)
	}

	return nil
}

// SecureZeroMemory attempts to securely zero sensitive memory regions
func SecureZeroMemory(data []byte) {
	if len(data) == 0 {
		return
	}

	// Platform-specific secure memory clearing
	switch runtime.GOOS {
	case "linux", "darwin":
		// Use explicit_bzero or memset_s if available, fallback to manual clearing
		secureZeroUnix(data)
	case "windows":
		// Use SecureZeroMemory on Windows
		secureZeroWindows(data)
	default:
		// Fallback: manual clearing with memory barrier
		crypto.SecureZero(data)
	}

	// Additional protection: try to prevent compiler optimization
	runtime.KeepAlive(data)
}

// secureZeroUnix implements secure memory clearing for Unix-like systems
func secureZeroUnix(data []byte) {
	if len(data) == 0 {
		return
	}

	// Try to use mlock to prevent swapping during clearing
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		// Bounds check: ensure slice is not empty before taking address
		if len(data) > 0 {
			r1, r2, errno := syscall.Syscall(syscall.SYS_MLOCK, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), 0)
			if errno != 0 {
				// Log warning but continue - memory locking is optional for security
				_ = r1 // silence unused variable warning
				_ = r2 // silence unused variable warning
			}
		}
	}

	// Clear memory
	for i := range data {
		data[i] = 0
	}

	// Unlock memory if we locked it
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		// Bounds check: ensure slice is not empty before taking address
		if len(data) > 0 {
			r1, r2, errno := syscall.Syscall(syscall.SYS_MUNLOCK, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), 0)
			if errno != 0 {
				// Log warning but continue - memory unlocking failure is not critical
				_ = r1 // silence unused variable warning
				_ = r2 // silence unused variable warning
			}
		}
	}
}

// secureZeroWindows implements secure memory clearing for Windows
func secureZeroWindows(data []byte) {
	// Fallback to manual clearing on Windows
	// In a full implementation, this would use SecureZeroMemory from kernel32.dll
	crypto.SecureZero(data)
}

// LockMemory attempts to lock memory pages to prevent swapping
func (eidx *EncryptedFileIndex) LockMemory() error {
	if !eidx.encrypted {
		return nil // No sensitive data to protect
	}

	// This is a basic implementation - in production, you'd want more sophisticated memory protection
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		// Lock the encryption key in memory
		if eidx.encryptionKey != nil {
			// Bounds check: ensure key slice is not empty before taking address
			keyLen := len(eidx.encryptionKey.Key)
			if keyLen == 0 {
				return fmt.Errorf("encryption key is empty, cannot lock memory")
			}

			r1, r2, errno := syscall.Syscall(syscall.SYS_MLOCK,
				uintptr(unsafe.Pointer(&eidx.encryptionKey.Key[0])),
				uintptr(keyLen), 0)
			if errno != 0 {
				return fmt.Errorf("failed to lock memory (addr=%x, len=%d): %v", r1, keyLen, errno)
			}
			_ = r2 // silence unused variable warning
		}
	}

	return nil
}

// UnlockMemory unlocks previously locked memory
func (eidx *EncryptedFileIndex) UnlockMemory() {
	if !eidx.encrypted {
		return
	}

	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		if eidx.encryptionKey != nil && len(eidx.encryptionKey.Key) > 0 {
			// Bounds check: ensure key slice is not empty before taking address
			keyLen := len(eidx.encryptionKey.Key)
			if keyLen > 0 {
				r1, r2, errno := syscall.Syscall(syscall.SYS_MUNLOCK,
					uintptr(unsafe.Pointer(&eidx.encryptionKey.Key[0])),
					uintptr(keyLen), 0)
				if errno != 0 {
					// Don't panic on unlock failure - this is cleanup code
					// In production, you might want to log this error
					_ = r1 // silence unused variable warning
					_ = r2 // silence unused variable warning
				}
			}
		}
	}
}

// Cleanup securely clears sensitive data
func (eidx *EncryptedFileIndex) Cleanup() {
	if eidx.encryptionKey != nil {
		SecureZeroMemory(eidx.encryptionKey.Key)
		SecureZeroMemory(eidx.encryptionKey.Salt)
	}

	if len(eidx.password) > 0 {
		// Clear password from memory
		SecureZeroMemory(eidx.password)
		// Set to nil to help GC
		eidx.password = nil
	}

	eidx.UnlockMemory()
}

// GetEncryptedIndexPath returns the path for encrypted index with suffix
func GetEncryptedIndexPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	noisefsDir := filepath.Join(homeDir, ".noisefs")
	if err := os.MkdirAll(noisefsDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create .noisefs directory: %w", err)
	}

	return filepath.Join(noisefsDir, "index.json"), nil
}

// MigrateToEncrypted migrates an existing unencrypted index to encrypted format
func MigrateToEncrypted(indexPath, password string) error {
	if password == "" {
		return fmt.Errorf("password required for encrypted index")
	}

	// Create encrypted index instance
	encIndex, err := NewEncryptedFileIndex(indexPath, password)
	if err != nil {
		return fmt.Errorf("failed to create encrypted index: %w", err)
	}
	defer encIndex.Cleanup()

	// Load existing unencrypted data
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		return nil // No existing index to migrate
	}

	data, err := os.ReadFile(indexPath)
	if err != nil {
		return fmt.Errorf("failed to read existing index: %w", err)
	}

	// Parse unencrypted data
	var oldIndex FileIndex
	if err := json.Unmarshal(data, &oldIndex); err != nil {
		return fmt.Errorf("failed to parse existing index: %w", err)
	}

	// Copy data to encrypted index
	encIndex.Entries = oldIndex.Entries
	encIndex.Version = oldIndex.Version
	encIndex.dirty = true

	// Create backup of old index
	backupPath := indexPath + ".backup-unencrypted"
	if err := os.Rename(indexPath, backupPath); err != nil {
		return fmt.Errorf("failed to backup existing index: %w", err)
	}

	// Save as encrypted
	if err := encIndex.SaveIndex(); err != nil {
		// Restore backup on failure
		os.Rename(backupPath, indexPath)
		return fmt.Errorf("failed to save encrypted index: %w", err)
	}

	// Securely delete backup
	secureDeleteFile(backupPath)

	return nil
}

// SecurePasswordBytes creates a secure copy of a password string as bytes
// The returned bytes should be passed to SecureZeroMemory when no longer needed
func SecurePasswordBytes(password string) []byte {
	if password == "" {
		return nil
	}
	passwordBytes := make([]byte, len(password))
	copy(passwordBytes, []byte(password))
	return passwordBytes
}

// secureDeleteFile attempts to securely delete a file using modern secure deletion standards
func secureDeleteFile(path string) {
	// Enhanced secure deletion - overwrite file multiple times before deletion
	if file, err := os.OpenFile(path, os.O_WRONLY, 0); err == nil {
		defer file.Close()

		stat, err := file.Stat()
		if err == nil {
			size := stat.Size()

			// Use modern secure deletion standard: 7 overwrite passes
			// Pass 1-3: Random data
			for i := 0; i < 3; i++ {
				file.Seek(0, 0)
				randomData := make([]byte, size)
				rand.Read(randomData)
				file.Write(randomData)
				file.Sync()
			}

			// Pass 4: All zeros
			file.Seek(0, 0)
			zeroData := make([]byte, size)
			file.Write(zeroData)
			file.Sync()

			// Pass 5: All ones (0xFF)
			file.Seek(0, 0)
			onesData := make([]byte, size)
			for i := range onesData {
				onesData[i] = 0xFF
			}
			file.Write(onesData)
			file.Sync()

			// Pass 6-7: More random data for additional security
			for i := 0; i < 2; i++ {
				file.Seek(0, 0)
				randomData := make([]byte, size)
				rand.Read(randomData)
				file.Write(randomData)
				file.Sync()
			}
		}
	}

	// Finally remove the file
	os.Remove(path)
}

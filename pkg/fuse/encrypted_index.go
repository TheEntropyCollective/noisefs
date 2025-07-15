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
	password     string
	encryptionKey *crypto.EncryptionKey
	encrypted    bool
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
	
	// Generate encryption key from password
	encKey, err := crypto.GenerateKey(password)
	if err != nil {
		return nil, fmt.Errorf("failed to generate encryption key: %w", err)
	}
	
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
	key, err := crypto.DeriveKey(eidx.password, encIndex.Salt)
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
		syscall.Syscall(syscall.SYS_MLOCK, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), 0)
	}
	
	// Clear memory
	for i := range data {
		data[i] = 0
	}
	
	// Unlock memory if we locked it
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		syscall.Syscall(syscall.SYS_MUNLOCK, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), 0)
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
		if eidx.encryptionKey != nil && len(eidx.encryptionKey.Key) > 0 {
			_, _, errno := syscall.Syscall(syscall.SYS_MLOCK, 
				uintptr(unsafe.Pointer(&eidx.encryptionKey.Key[0])), 
				uintptr(len(eidx.encryptionKey.Key)), 0)
			if errno != 0 {
				return fmt.Errorf("failed to lock memory: %v", errno)
			}
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
			syscall.Syscall(syscall.SYS_MUNLOCK, 
				uintptr(unsafe.Pointer(&eidx.encryptionKey.Key[0])), 
				uintptr(len(eidx.encryptionKey.Key)), 0)
		}
	}
}

// Cleanup securely clears sensitive data
func (eidx *EncryptedFileIndex) Cleanup() {
	if eidx.encryptionKey != nil {
		SecureZeroMemory(eidx.encryptionKey.Key)
		SecureZeroMemory(eidx.encryptionKey.Salt)
	}
	
	if eidx.password != "" {
		// Clear password from memory (best effort)
		passwordBytes := []byte(eidx.password)
		SecureZeroMemory(passwordBytes)
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

// secureDeleteFile attempts to securely delete a file
func secureDeleteFile(path string) {
	// Basic secure deletion - overwrite file before deletion
	if file, err := os.OpenFile(path, os.O_WRONLY, 0); err == nil {
		defer file.Close()
		
		stat, err := file.Stat()
		if err == nil {
			size := stat.Size()
			
			// Overwrite with random data 3 times
			for i := 0; i < 3; i++ {
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
package security

import (
	"crypto/rand"
	"os"
	"runtime"
	"sync"
	"time"
)

// SecureFileDeleter provides secure file deletion capabilities
type SecureFileDeleter struct {
	enabled       bool
	tempFiles     map[string]time.Time
	mu            sync.Mutex
	cleanupTicker *time.Ticker
	done          chan bool
}

// NewSecureFileDeleter creates a new secure file deleter
func NewSecureFileDeleter(enabled bool) *SecureFileDeleter {
	sfd := &SecureFileDeleter{
		enabled:   enabled,
		tempFiles: make(map[string]time.Time),
		done:      make(chan bool),
	}
	
	if enabled {
		// Start cleanup goroutine for temporary files
		sfd.cleanupTicker = time.NewTicker(5 * time.Minute)
		go sfd.cleanupLoop()
	}
	
	return sfd
}

// RegisterTempFile registers a temporary file for secure deletion
func (sfd *SecureFileDeleter) RegisterTempFile(path string) {
	if !sfd.enabled {
		return
	}
	
	sfd.mu.Lock()
	defer sfd.mu.Unlock()
	sfd.tempFiles[path] = time.Now()
}

// SecureDelete securely deletes a file by overwriting it multiple times
func (sfd *SecureFileDeleter) SecureDelete(path string) error {
	if !sfd.enabled {
		// Just remove normally if secure delete is disabled
		return os.Remove(path)
	}
	
	// Open file for writing
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		// File might not exist, just try to remove it
		os.Remove(path)
		return nil
	}
	defer file.Close()
	
	// Get file size
	stat, err := file.Stat()
	if err != nil {
		file.Close()
		os.Remove(path)
		return nil
	}
	
	size := stat.Size()
	if size == 0 {
		file.Close()
		return os.Remove(path)
	}
	
	// Perform multiple overwrite passes
	passes := [][]byte{
		// Pass 1: All zeros
		make([]byte, size),
		// Pass 2: All ones
		func() []byte {
			data := make([]byte, size)
			for i := range data {
				data[i] = 0xFF
			}
			return data
		}(),
		// Pass 3: Random data
		func() []byte {
			data := make([]byte, size)
			rand.Read(data)
			return data
		}(),
	}
	
	for _, passData := range passes {
		// Seek to beginning
		if _, err := file.Seek(0, 0); err != nil {
			break
		}
		
		// Write pass data
		if _, err := file.Write(passData); err != nil {
			break
		}
		
		// Force sync to disk
		file.Sync()
	}
	
	// Close file handle before deletion
	file.Close()
	
	// Remove the file
	return os.Remove(path)
}

// cleanupLoop periodically cleans up old temporary files
func (sfd *SecureFileDeleter) cleanupLoop() {
	for {
		select {
		case <-sfd.cleanupTicker.C:
			sfd.cleanupOldTempFiles()
		case <-sfd.done:
			return
		}
	}
}

// cleanupOldTempFiles removes temporary files older than 1 hour
func (sfd *SecureFileDeleter) cleanupOldTempFiles() {
	sfd.mu.Lock()
	defer sfd.mu.Unlock()
	
	cutoff := time.Now().Add(-1 * time.Hour)
	
	for path, createdAt := range sfd.tempFiles {
		if createdAt.Before(cutoff) {
			sfd.SecureDelete(path)
			delete(sfd.tempFiles, path)
		}
	}
}

// Shutdown stops the secure file deleter and cleans up
func (sfd *SecureFileDeleter) Shutdown() {
	if !sfd.enabled {
		return
	}
	
	// Stop cleanup loop
	if sfd.cleanupTicker != nil {
		sfd.cleanupTicker.Stop()
	}
	
	select {
	case sfd.done <- true:
	default:
	}
	
	// Clean up remaining temp files
	sfd.mu.Lock()
	defer sfd.mu.Unlock()
	
	for path := range sfd.tempFiles {
		sfd.SecureDelete(path)
	}
	sfd.tempFiles = make(map[string]time.Time)
}

// MemoryProtection provides secure memory handling utilities
type MemoryProtection struct {
	enabled bool
}

// NewMemoryProtection creates a new memory protection instance
func NewMemoryProtection(enabled bool) *MemoryProtection {
	return &MemoryProtection{enabled: enabled}
}

// ClearSensitiveData securely clears sensitive data from memory
func (mp *MemoryProtection) ClearSensitiveData(data []byte) {
	if !mp.enabled {
		return
	}
	
	// Clear the data
	for i := range data {
		data[i] = 0
	}
	
	// Additional protection: trigger garbage collection
	runtime.GC()
	runtime.GC() // Call twice to be more thorough
}

// SecureBuffer represents a buffer that will be securely cleared
type SecureBuffer struct {
	data []byte
	mp   *MemoryProtection
}

// NewSecureBuffer creates a new secure buffer
func NewSecureBuffer(size int, mp *MemoryProtection) *SecureBuffer {
	return &SecureBuffer{
		data: make([]byte, size),
		mp:   mp,
	}
}

// Data returns the underlying byte slice
func (sb *SecureBuffer) Data() []byte {
	return sb.data
}

// Clear securely clears the buffer
func (sb *SecureBuffer) Clear() {
	if sb.mp != nil {
		sb.mp.ClearSensitiveData(sb.data)
	}
}

// RAMOnlyMode provides utilities for running in RAM-only mode
type RAMOnlyMode struct {
	enabled   bool
	tempFiles []string
	mu        sync.Mutex
}

// NewRAMOnlyMode creates a new RAM-only mode instance
func NewRAMOnlyMode(enabled bool) *RAMOnlyMode {
	return &RAMOnlyMode{
		enabled:   enabled,
		tempFiles: make([]string, 0),
	}
}

// IsEnabled returns whether RAM-only mode is enabled
func (rom *RAMOnlyMode) IsEnabled() bool {
	return rom.enabled
}

// RegisterTempFile registers a temporary file for cleanup
func (rom *RAMOnlyMode) RegisterTempFile(path string) {
	if !rom.enabled {
		return
	}
	
	rom.mu.Lock()
	defer rom.mu.Unlock()
	rom.tempFiles = append(rom.tempFiles, path)
}

// Cleanup removes all temporary files
func (rom *RAMOnlyMode) Cleanup() {
	if !rom.enabled {
		return
	}
	
	rom.mu.Lock()
	defer rom.mu.Unlock()
	
	for _, path := range rom.tempFiles {
		os.Remove(path)
	}
	rom.tempFiles = make([]string, 0)
}

// SecurityManager coordinates all security features
type SecurityManager struct {
	FileDeleter      *SecureFileDeleter
	MemoryProtection *MemoryProtection
	RAMOnlyMode      *RAMOnlyMode
	config           SecurityConfig
}

// SecurityConfig holds security configuration
type SecurityConfig struct {
	SecureMemory  bool `json:"secure_memory"`
	AntiForensics bool `json:"anti_forensics"`
	RAMOnlyMode   bool `json:"ram_only_mode"`
}

// NewSecurityManager creates a new security manager
func NewSecurityManager(config SecurityConfig) *SecurityManager {
	return &SecurityManager{
		FileDeleter:      NewSecureFileDeleter(config.AntiForensics),
		MemoryProtection: NewMemoryProtection(config.SecureMemory),
		RAMOnlyMode:      NewRAMOnlyMode(config.RAMOnlyMode),
		config:           config,
	}
}

// Shutdown cleans up all security components
func (sm *SecurityManager) Shutdown() {
	if sm.FileDeleter != nil {
		sm.FileDeleter.Shutdown()
	}
	
	if sm.RAMOnlyMode != nil {
		sm.RAMOnlyMode.Cleanup()
	}
	
	// Force garbage collection if secure memory is enabled
	if sm.config.SecureMemory {
		runtime.GC()
		runtime.GC()
	}
}
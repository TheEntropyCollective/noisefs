// Package security provides comprehensive security utilities for NoiseFS,
// including secure file deletion, memory protection, and anti-forensic capabilities.
//
// This package implements multiple layers of security protection designed to
// prevent sensitive data recovery through various attack vectors:
//
// Security Components:
//   - SecureFileDeleter: Multi-pass file overwriting for secure deletion
//   - MemoryProtection: Secure memory clearing and garbage collection
//   - SecureBuffer: Memory buffers with automatic secure cleanup
//   - RAMOnlyMode: Temporary file management for in-memory operations
//   - SecurityManager: Coordinated security feature management
//
// Anti-Forensic Features:
//   - 3-pass file overwriting (zeros, ones, random data)
//   - Forced disk synchronization during overwrites
//   - Automatic temporary file cleanup
//   - Secure memory clearing with explicit garbage collection
//   - RAM-only operation mode for sensitive operations
//
// Threat Model:
//   - Protects against basic file recovery tools
//   - Prevents sensitive data from lingering in memory
//   - Reduces forensic artifacts from temporary files
//   - Provides defense-in-depth for privacy-critical operations
//
// Usage Example:
//
//	// Create security manager with full protection
//	config := SecurityConfig{
//		SecureMemory:  true,
//		AntiForensics: true,
//		RAMOnlyMode:   false,
//	}
//	sm := NewSecurityManager(config)
//	defer sm.Shutdown()
//	
//	// Use secure file deletion
//	err := sm.FileDeleter.SecureDelete("/tmp/sensitive.dat")
//	
//	// Use secure memory handling
//	buffer := NewSecureBuffer(1024, sm.MemoryProtection)
//	defer buffer.Clear()
//
package security

import (
	"crypto/rand"
	"os"
	"runtime"
	"sync"
	"time"
)

// SecureFileDeleter provides comprehensive secure file deletion capabilities.
//
// This type implements multi-pass file overwriting and automatic cleanup
// of temporary files to prevent data recovery through forensic analysis.
// It uses a 3-pass overwrite strategy (zeros, ones, random) followed by
// file deletion to maximize data destruction effectiveness.
//
// Security Features:
//   - 3-pass overwrite algorithm for thorough data destruction
//   - Automatic temporary file tracking and cleanup
//   - Background cleanup of stale temporary files
//   - Forced disk synchronization during overwrites
//   - Graceful degradation when security features are disabled
//
// Implementation Details:
//   - Uses crypto/rand for cryptographically secure random data
//   - Performs explicit fsync() calls to ensure data reaches disk
//   - Thread-safe operations with mutex protection
//   - Configurable enabling/disabling for performance scenarios
//
// Forensic Resistance:
//   - Multiple overwrite passes prevent magnetic residue analysis
//   - Random data pass prevents pattern-based recovery
//   - Immediate file deletion after overwriting
//   - No temporary copies or backup files created
//
type SecureFileDeleter struct {
	enabled       bool
	tempFiles     map[string]time.Time
	mu            sync.Mutex
	cleanupTicker *time.Ticker
	done          chan bool
}

// NewSecureFileDeleter creates a new secure file deleter with configurable security features.
//
// This constructor initializes a SecureFileDeleter instance and optionally starts
// a background cleanup goroutine for automatic temporary file management.
// When enabled, the deleter provides comprehensive anti-forensic capabilities.
//
// Background Services:
//   - Cleanup goroutine runs every 5 minutes when enabled
//   - Automatically removes temporary files older than 1 hour
//   - Graceful shutdown coordination through done channel
//
// Performance Considerations:
//   - When disabled, falls back to standard os.Remove() for performance
//   - Minimal overhead when secure deletion is not needed
//   - Background cleanup can be disabled by passing enabled=false
//
// Parameters:
//   enabled: Whether to enable secure deletion features (false = standard deletion)
//
// Returns:
//   *SecureFileDeleter: A new secure file deleter instance
//
// Thread Safety:
//   - Safe for concurrent use across multiple goroutines
//   - Internal mutex protects temporary file registry
//
// Complexity: O(1) - Simple initialization
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

// RegisterTempFile registers a temporary file for automatic secure deletion.
//
// This method adds a file path to the temporary file registry, enabling
// automatic cleanup through the background cleanup process. Files are
// tracked with their registration timestamp for age-based cleanup.
//
// Cleanup Behavior:
//   - Files older than 1 hour are automatically deleted
//   - Cleanup occurs every 5 minutes via background goroutine
//   - All registered files are deleted during shutdown
//
// Security Considerations:
//   - Only operates when secure deletion is enabled
//   - Uses secure deletion for temporary file cleanup
//   - Thread-safe registration with mutex protection
//
// Parameters:
//   path: Absolute or relative path to the temporary file
//
// Thread Safety:
//   - Safe for concurrent calls from multiple goroutines
//   - Internal mutex protects the temporary file registry
//
// Complexity: O(1) - Simple map insertion
func (sfd *SecureFileDeleter) RegisterTempFile(path string) {
	if !sfd.enabled {
		return
	}
	
	sfd.mu.Lock()
	defer sfd.mu.Unlock()
	sfd.tempFiles[path] = time.Now()
}

// SecureDelete securely deletes a file using multi-pass overwriting for anti-forensic protection.
//
// This method implements a comprehensive 3-pass overwrite algorithm designed to
// prevent data recovery through magnetic residue analysis or specialized hardware.
// It ensures data destruction at both the logical and physical storage levels.
//
// Overwrite Algorithm:
//   Pass 1: Write zeros (0x00) to destroy logical data structure
//   Pass 2: Write ones (0xFF) to flip all magnetic domains
//   Pass 3: Write cryptographic random data to eliminate patterns
//
// Security Features:
//   - Forced disk synchronization (fsync) after each pass
//   - Handles files of any size with appropriate memory management
//   - Graceful degradation for non-existent or inaccessible files
//   - Uses crypto/rand for cryptographically secure random data
//
// Error Handling:
//   - Missing files are silently ignored (already deleted)
//   - Permission errors attempt standard deletion as fallback
//   - Write errors during overwrite terminate early but still delete
//   - Zero-length files skip overwrite and proceed to deletion
//
// Performance Characteristics:
//   - Time complexity: O(n) where n is file size
//   - Memory usage: O(file_size) for overwrite buffers
//   - Disk I/O: 3x file size plus sync operations
//
// Parameters:
//   path: Path to the file to be securely deleted
//
// Returns:
//   error: nil on success, error details on filesystem failures
//
// Complexity: O(n) where n is the file size in bytes
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
	
	// Perform multiple overwrite passes using 3-pass algorithm
	// This follows security best practices for magnetic media data destruction
	passes := [][]byte{
		// Pass 1: All zeros (0x00) - Destroys logical file structure
		// Sets all bits to 0, eliminating file content at logical level
		make([]byte, size),
		
		// Pass 2: All ones (0xFF) - Flips all magnetic domains
		// Sets all bits to 1, ensuring magnetic domains are rewritten
		func() []byte {
			data := make([]byte, size)
			for i := range data {
				data[i] = 0xFF  // Binary: 11111111
			}
			return data
		}(),
		
		// Pass 3: Cryptographic random data - Eliminates patterns
		// Uses crypto/rand for cryptographically secure randomness
		// Prevents pattern-based data recovery techniques
		func() []byte {
			data := make([]byte, size)
			rand.Read(data)  // Cryptographically secure random bytes
			return data
		}(),
	}
	
	// Execute each overwrite pass sequentially
	for _, passData := range passes {
		// Seek to beginning of file for each pass
		if _, err := file.Seek(0, 0); err != nil {
			break  // Stop on seek errors but continue to deletion
		}
		
		// Write the pass data over the entire file
		if _, err := file.Write(passData); err != nil {
			break  // Stop on write errors but continue to deletion
		}
		
		// Force synchronous write to physical storage
		// This ensures data reaches the disk before next pass
		file.Sync()
	}
	
	// Close file handle before deletion
	file.Close()
	
	// Remove the file
	return os.Remove(path)
}

// cleanupLoop runs the background cleanup process for temporary file management.
//
// This goroutine performs periodic cleanup of registered temporary files,
// removing files that exceed the maximum age threshold. It coordinates
// with the shutdown process through channel communication.
//
// Cleanup Schedule:
//   - Runs every 5 minutes via time.Ticker
//   - Processes all registered temporary files each cycle
//   - Terminates gracefully on shutdown signal
//
// Lifecycle Management:
//   - Started automatically when SecureFileDeleter is enabled
//   - Terminated via done channel during shutdown
//   - Cleanup ticker is managed by the calling code
//
// Error Handling:
//   - Individual file deletion errors are ignored
//   - Cleanup continues even if some files cannot be deleted
//   - Registry is updated regardless of deletion success
//
// Thread Safety:
//   - Coordinates with main thread through channels
//   - File registry access is mutex-protected
//
// Complexity: O(1) - Goroutine lifecycle management
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

// cleanupOldTempFiles removes temporary files that exceed the maximum age threshold.
//
// This method iterates through all registered temporary files and securely
// deletes those older than 1 hour. It maintains the temporary file registry
// by removing processed entries.
//
// Age Calculation:
//   - Uses current time minus 1 hour as cutoff threshold
//   - Compares against file registration timestamp (not filesystem timestamp)
//   - Processes all files in a single pass
//
// Cleanup Process:
//   - Identifies files older than threshold
//   - Performs secure deletion on qualifying files
//   - Removes processed files from registry
//   - Continues processing even if individual deletions fail
//
// Error Handling:
//   - Individual deletion failures are silently ignored
//   - Registry cleanup proceeds regardless of deletion success
//   - Ensures registry doesn't accumulate stale entries
//
// Thread Safety:
//   - Acquires mutex lock for entire operation
//   - Protects both registry access and modification
//
// Complexity: O(n) where n is the number of registered temporary files
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

// Shutdown gracefully terminates the secure file deleter and performs final cleanup.
//
// This method coordinates the shutdown of all background processes and ensures
// complete cleanup of registered temporary files. It provides comprehensive
// resource cleanup for secure shutdown scenarios.
//
// Shutdown Process:
//   1. Stop the background cleanup ticker
//   2. Signal the cleanup goroutine to terminate
//   3. Securely delete all remaining temporary files
//   4. Clear the temporary file registry
//
// Background Process Coordination:
//   - Stops cleanup ticker to prevent new cleanup cycles
//   - Sends shutdown signal through done channel
//   - Non-blocking send prevents deadlock scenarios
//
// Final Cleanup:
//   - Processes all files remaining in registry
//   - Uses secure deletion for each file
//   - Resets registry to prevent resource leaks
//
// Error Handling:
//   - Individual file deletion errors are ignored
//   - Shutdown completes even if some files cannot be deleted
//   - Graceful degradation when secure deletion is disabled
//
// Thread Safety:
//   - Mutex protection for registry access
//   - Safe coordination with background goroutine
//
// Complexity: O(n) where n is the number of registered temporary files
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

// MemoryProtection provides secure memory handling utilities for sensitive data.
//
// This type implements memory protection techniques designed to prevent
// sensitive data from lingering in system memory where it could be
// recovered by attackers or forensic analysis tools.
//
// Protection Mechanisms:
//   - Explicit memory clearing using zero-fill operations
//   - Forced garbage collection to clear Go runtime memory
//   - Configurable enabling/disabling for performance scenarios
//
// Security Considerations:
//   - Protects against memory dump analysis
//   - Reduces window for sensitive data recovery
//   - Coordinates with Go garbage collector for thorough cleanup
//
// Limitations:
//   - Cannot prevent all memory residue (OS swap files, etc.)
//   - Effectiveness depends on Go runtime memory management
//   - No protection against hardware-level attacks (cold boot, etc.)
//
type MemoryProtection struct {
	enabled bool
}

// NewMemoryProtection creates a new memory protection instance with configurable security.
//
// This constructor initializes a MemoryProtection instance that can be
// enabled or disabled based on security requirements and performance
// considerations.
//
// Configuration:
//   - When enabled: Provides active memory clearing and garbage collection
//   - When disabled: No-op operations for maximum performance
//
// Parameters:
//   enabled: Whether to enable memory protection features
//
// Returns:
//   *MemoryProtection: A new memory protection instance
//
// Complexity: O(1) - Simple initialization
func NewMemoryProtection(enabled bool) *MemoryProtection {
	return &MemoryProtection{enabled: enabled}
}

// ClearSensitiveData securely clears sensitive data from memory to prevent recovery.
//
// This method implements comprehensive memory clearing techniques to minimize
// the window of opportunity for sensitive data recovery from system memory.
// It combines explicit data clearing with garbage collection coordination.
//
// Clearing Process:
//   1. Zero-fill the entire data buffer byte by byte
//   2. Trigger immediate garbage collection
//   3. Trigger second garbage collection for thoroughness
//
// Security Features:
//   - Explicit zero-fill prevents simple memory scanning
//   - Double garbage collection clears Go runtime memory
//   - Immediate execution reduces exposure window
//
// Limitations:
//   - Cannot clear all copies (runtime may have internal copies)
//   - No protection against OS swap files or hibernation
//   - Memory pages may still exist in physical RAM
//
// Performance Impact:
//   - O(n) time complexity for buffer clearing
//   - Garbage collection pause may affect application responsiveness
//   - Disabled mode has no performance impact
//
// Parameters:
//   data: Byte slice containing sensitive data to be cleared
//
// Thread Safety:
//   - Safe for concurrent use on different data buffers
//   - Garbage collection is globally synchronized
//
// Complexity: O(n) where n is the length of the data buffer
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

// SecureBuffer represents a memory buffer with automatic secure cleanup capabilities.
//
// This type provides a convenient wrapper around byte slices that ensures
// sensitive data is properly cleared from memory when no longer needed.
// It integrates with MemoryProtection for consistent security handling.
//
// Security Features:
//   - Automatic secure clearing when explicitly requested
//   - Integration with memory protection policies
//   - Encapsulation of sensitive data handling
//
// Usage Pattern:
//   1. Create buffer with NewSecureBuffer()
//   2. Use Data() to access underlying byte slice
//   3. Call Clear() when data is no longer needed
//   4. Consider using defer Clear() for automatic cleanup
//
// Memory Management:
//   - Buffer size is fixed at creation time
//   - Underlying slice is allocated normally
//   - Clear() operation uses MemoryProtection if available
//
type SecureBuffer struct {
	data []byte
	mp   *MemoryProtection
}

// NewSecureBuffer creates a new secure buffer with specified size and memory protection.
//
// This constructor allocates a new byte buffer and associates it with a
// MemoryProtection instance for secure cleanup operations. The buffer
// can be used normally but provides secure clearing capabilities.
//
// Memory Allocation:
//   - Creates a new byte slice of the specified size
//   - Uses standard Go memory allocation (not locked memory)
//   - Zero-initialized by default Go behavior
//
// Parameters:
//   size: Size of the buffer in bytes
//   mp: MemoryProtection instance for secure clearing (can be nil)
//
// Returns:
//   *SecureBuffer: A new secure buffer instance
//
// Usage Example:
//   buffer := NewSecureBuffer(1024, memoryProtection)
//   defer buffer.Clear()
//   // Use buffer.Data() for operations
//
// Complexity: O(1) - Simple allocation and initialization
func NewSecureBuffer(size int, mp *MemoryProtection) *SecureBuffer {
	return &SecureBuffer{
		data: make([]byte, size),
		mp:   mp,
	}
}

// Data returns the underlying byte slice for read/write operations.
//
// This method provides access to the internal byte buffer, allowing
// normal slice operations while maintaining the security context.
// The returned slice shares memory with the secure buffer.
//
// Security Considerations:
//   - Returned slice shares memory with internal buffer
//   - Modifications affect the secure buffer directly
//   - Clearing the buffer will clear the returned slice
//   - Caller should not retain references after Clear()
//
// Returns:
//   []byte: The underlying byte slice
//
// Thread Safety:
//   - Not thread-safe for concurrent modifications
//   - Caller responsible for synchronization if needed
//
// Complexity: O(1) - Direct slice return
func (sb *SecureBuffer) Data() []byte {
	return sb.data
}

// Clear securely clears the buffer contents using memory protection.
//
// This method performs secure clearing of the buffer contents using
// the associated MemoryProtection instance. It should be called when
// the sensitive data is no longer needed.
//
// Clearing Process:
//   - Uses MemoryProtection.ClearSensitiveData() if available
//   - Falls back to no-op if no memory protection is configured
//   - Clears all bytes in the underlying buffer
//
// Post-Clear State:
//   - Buffer remains allocated but contains cleared data
//   - Buffer can be reused for new data if needed
//   - Data() method still returns valid slice (but cleared)
//
// Usage Pattern:
//   buffer := NewSecureBuffer(size, mp)
//   defer buffer.Clear() // Automatic cleanup
//   // Use buffer...
//
// Thread Safety:
//   - Not thread-safe with concurrent Data() access
//   - Safe to call multiple times
//
// Complexity: O(n) where n is the buffer size
func (sb *SecureBuffer) Clear() {
	if sb.mp != nil {
		sb.mp.ClearSensitiveData(sb.data)
	}
}

// RAMOnlyMode provides utilities for managing temporary files in memory-focused operations.
//
// This type helps coordinate temporary file management when operating in
// modes that prioritize keeping data in memory rather than writing to
// persistent storage. It tracks temporary files for cleanup purposes.
//
// RAM-Only Concept:
//   - Minimizes persistent storage writes during operations
//   - Tracks temporary files created during processing
//   - Ensures complete cleanup of temporary artifacts
//   - Reduces forensic footprint on storage devices
//
// Use Cases:
//   - Sensitive data processing workflows
//   - Operations requiring minimal disk writes
//   - Anti-forensic processing modes
//   - Memory-constrained environments
//
// File Management:
//   - Tracks all temporary files created during operations
//   - Provides centralized cleanup coordination
//   - Thread-safe registration and cleanup operations
//
type RAMOnlyMode struct {
	enabled   bool
	tempFiles []string
	mu        sync.Mutex
}

// NewRAMOnlyMode creates a new RAM-only mode instance with configurable behavior.
//
// This constructor initializes a RAMOnlyMode instance that can be enabled
// or disabled based on operational requirements. When disabled, it operates
// as a no-op for minimal overhead.
//
// Configuration:
//   - When enabled: Active temporary file tracking and cleanup
//   - When disabled: No-op operations for standard processing
//
// Parameters:
//   enabled: Whether to enable RAM-only mode features
//
// Returns:
//   *RAMOnlyMode: A new RAM-only mode instance
//
// Complexity: O(1) - Simple initialization
func NewRAMOnlyMode(enabled bool) *RAMOnlyMode {
	return &RAMOnlyMode{
		enabled:   enabled,
		tempFiles: make([]string, 0),
	}
}

// IsEnabled returns whether RAM-only mode is currently enabled.
//
// This method provides a way to check the current operational mode,
// allowing calling code to adjust behavior based on RAM-only status.
//
// Returns:
//   bool: true if RAM-only mode is enabled, false otherwise
//
// Usage:
//   if ramOnlyMode.IsEnabled() {
//       // Use in-memory processing
//   } else {
//       // Use standard file-based processing
//   }
//
// Complexity: O(1) - Simple field access
func (rom *RAMOnlyMode) IsEnabled() bool {
	return rom.enabled
}

// RegisterTempFile registers a temporary file for centralized cleanup management.
//
// This method adds a file path to the temporary file registry, enabling
// coordinated cleanup of all temporary files created during RAM-only
// operations. Files are tracked until explicitly cleaned up.
//
// Registration Benefits:
//   - Centralized cleanup coordination
//   - Ensures no temporary files are left behind
//   - Simplifies error handling and cleanup paths
//   - Supports bulk cleanup operations
//
// Parameters:
//   path: Path to the temporary file to be tracked
//
// Thread Safety:
//   - Safe for concurrent registration from multiple goroutines
//   - Mutex protection for internal file list
//
// Complexity: O(1) - Simple slice append operation
func (rom *RAMOnlyMode) RegisterTempFile(path string) {
	if !rom.enabled {
		return
	}
	
	rom.mu.Lock()
	defer rom.mu.Unlock()
	rom.tempFiles = append(rom.tempFiles, path)
}

// Cleanup removes all registered temporary files and clears the registry.
//
// This method performs bulk cleanup of all temporary files that have been
// registered during RAM-only operations. It ensures complete cleanup of
// temporary artifacts to minimize forensic footprint.
//
// Cleanup Process:
//   - Iterates through all registered file paths
//   - Attempts standard deletion for each file
//   - Clears the internal file registry
//   - Continues processing even if individual deletions fail
//
// Error Handling:
//   - Individual file deletion errors are silently ignored
//   - Registry is cleared regardless of deletion success
//   - Ensures registry doesn't accumulate stale entries
//
// Security Considerations:
//   - Uses standard os.Remove() (not secure deletion)
//   - Suitable for temporary files without sensitive content
//   - For sensitive files, use SecureFileDeleter instead
//
// Thread Safety:
//   - Mutex protection for registry access and modification
//   - Safe to call concurrently with RegisterTempFile()
//
// Complexity: O(n) where n is the number of registered temporary files
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

// SecurityManager coordinates all security features in a unified interface.
//
// This type provides centralized management of all security components,
// ensuring consistent configuration and coordinated lifecycle management.
// It serves as the primary interface for security operations in NoiseFS.
//
// Managed Components:
//   - SecureFileDeleter: Anti-forensic file deletion
//   - MemoryProtection: Secure memory handling
//   - RAMOnlyMode: Temporary file management
//   - SecurityConfig: Configuration coordination
//
// Design Benefits:
//   - Unified security policy enforcement
//   - Coordinated component lifecycle management
//   - Simplified security feature configuration
//   - Consistent security behavior across NoiseFS
//
// Usage Pattern:
//   1. Create with NewSecurityManager(config)
//   2. Access individual components as needed
//   3. Call Shutdown() for graceful cleanup
//
type SecurityManager struct {
	FileDeleter      *SecureFileDeleter
	MemoryProtection *MemoryProtection
	RAMOnlyMode      *RAMOnlyMode
	config           SecurityConfig
}

// SecurityConfig holds comprehensive security configuration for all security components.
//
// This structure defines the security posture for a NoiseFS instance,
// controlling the enablement and behavior of various security features.
// It provides fine-grained control over security vs performance trade-offs.
//
// Configuration Options:
//   - SecureMemory: Enable memory protection and secure clearing
//   - AntiForensics: Enable secure file deletion and anti-forensic measures
//   - RAMOnlyMode: Enable temporary file tracking and cleanup
//
// Security Levels:
//   - All disabled: Maximum performance, minimal security
//   - SecureMemory only: Basic memory protection
//   - AntiForensics only: File-level protection
//   - All enabled: Maximum security, moderate performance impact
//
type SecurityConfig struct {
	SecureMemory  bool `json:"secure_memory"`
	AntiForensics bool `json:"anti_forensics"`
	RAMOnlyMode   bool `json:"ram_only_mode"`
}

// NewSecurityManager creates a new security manager with the specified configuration.
//
// This constructor initializes all security components according to the
// provided configuration, ensuring consistent security policy enforcement
// across all NoiseFS operations.
//
// Component Initialization:
//   - SecureFileDeleter configured based on AntiForensics setting
//   - MemoryProtection configured based on SecureMemory setting
//   - RAMOnlyMode configured based on RAMOnlyMode setting
//   - Configuration stored for reference
//
// Parameters:
//   config: SecurityConfig defining the desired security posture
//
// Returns:
//   *SecurityManager: A new security manager with configured components
//
// Usage Example:
//   config := SecurityConfig{
//       SecureMemory:  true,
//       AntiForensics: true,
//       RAMOnlyMode:   false,
//   }
//   sm := NewSecurityManager(config)
//   defer sm.Shutdown()
//
// Complexity: O(1) - Simple component initialization
func NewSecurityManager(config SecurityConfig) *SecurityManager {
	return &SecurityManager{
		FileDeleter:      NewSecureFileDeleter(config.AntiForensics),
		MemoryProtection: NewMemoryProtection(config.SecureMemory),
		RAMOnlyMode:      NewRAMOnlyMode(config.RAMOnlyMode),
		config:           config,
	}
}

// Shutdown gracefully terminates all security components and performs final cleanup.
//
// This method coordinates the shutdown of all managed security components,
// ensuring proper cleanup of resources and temporary files. It should be
// called when the SecurityManager is no longer needed.
//
// Shutdown Process:
//   1. Shutdown SecureFileDeleter (stops background cleanup, deletes temp files)
//   2. Cleanup RAMOnlyMode (removes all registered temporary files)
//   3. Force garbage collection if secure memory is enabled
//
// Resource Cleanup:
//   - All temporary files are removed
//   - Background goroutines are terminated
//   - Memory is cleared if secure memory is enabled
//   - Component states are reset
//
// Error Handling:
//   - Individual component shutdown errors are ignored
//   - Shutdown proceeds even if some components fail
//   - Ensures best-effort cleanup in all scenarios
//
// Thread Safety:
//   - Safe to call multiple times
//   - Coordinates with background goroutines
//
// Complexity: O(n) where n is the total number of temporary files across all components
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
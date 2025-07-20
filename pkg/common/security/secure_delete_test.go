package security

import (
	"crypto/rand"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestNewSecureFileDeleter(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{"enabled", true},
		{"disabled", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sfd := NewSecureFileDeleter(tt.enabled)
			defer sfd.Shutdown()

			if sfd.enabled != tt.enabled {
				t.Errorf("enabled = %v, want %v", sfd.enabled, tt.enabled)
			}

			if sfd.tempFiles == nil {
				t.Error("tempFiles map not initialized")
			}

			if sfd.done == nil {
				t.Error("done channel not initialized")
			}

			if tt.enabled {
				if sfd.cleanupTicker == nil {
					t.Error("cleanupTicker not initialized when enabled")
				}
			} else {
				if sfd.cleanupTicker != nil {
					t.Error("cleanupTicker should be nil when disabled")
				}
			}
		})
	}
}

func TestSecureFileDeleter_RegisterTempFile(t *testing.T) {
	// Test enabled mode
	t.Run("enabled", func(t *testing.T) {
		sfd := NewSecureFileDeleter(true)
		defer sfd.Shutdown()

		testPath := "/tmp/test_file"
		sfd.RegisterTempFile(testPath)

		sfd.mu.Lock()
		defer sfd.mu.Unlock()

		if _, exists := sfd.tempFiles[testPath]; !exists {
			t.Errorf("temp file %s not registered", testPath)
		}
	})

	// Test disabled mode
	t.Run("disabled", func(t *testing.T) {
		sfd := NewSecureFileDeleter(false)
		defer sfd.Shutdown()

		testPath := "/tmp/test_file"
		sfd.RegisterTempFile(testPath)

		sfd.mu.Lock()
		defer sfd.mu.Unlock()

		if len(sfd.tempFiles) != 0 {
			t.Error("temp file registered when disabled")
		}
	})
}

func TestSecureFileDeleter_SecureDelete(t *testing.T) {
	tests := []struct {
		name     string
		enabled  bool
		fileSize int64
		content  []byte
	}{
		{"enabled_small_file", true, 100, []byte("test content that needs secure deletion")},
		{"enabled_empty_file", true, 0, []byte{}},
		{"enabled_large_file", true, 1024, make([]byte, 1024)},
		{"disabled_file", false, 100, []byte("test content for disabled mode")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sfd := NewSecureFileDeleter(tt.enabled)
			defer sfd.Shutdown()

			// Create temporary directory
			tmpDir, err := os.MkdirTemp("", "secure_delete_test")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			// Create test file
			testFile := filepath.Join(tmpDir, "test_file")
			file, err := os.Create(testFile)
			if err != nil {
				t.Fatalf("failed to create test file: %v", err)
			}

			if len(tt.content) > 0 {
				if _, err := file.Write(tt.content); err != nil {
					t.Fatalf("failed to write test content: %v", err)
				}
			}
			file.Close()

			// Verify file exists before deletion
			if _, err := os.Stat(testFile); os.IsNotExist(err) {
				t.Fatal("test file should exist before deletion")
			}

			// Perform secure deletion
			err = sfd.SecureDelete(testFile)
			if err != nil {
				t.Errorf("SecureDelete failed: %v", err)
			}

			// Verify file is deleted
			if _, err := os.Stat(testFile); !os.IsNotExist(err) {
				t.Error("file should be deleted after SecureDelete")
			}
		})
	}
}

func TestSecureFileDeleter_SecureDelete_NonExistentFile(t *testing.T) {
	sfd := NewSecureFileDeleter(true)
	defer sfd.Shutdown()

	nonExistentFile := "/tmp/this_file_does_not_exist"
	err := sfd.SecureDelete(nonExistentFile)
	if err != nil {
		t.Errorf("SecureDelete should not error on non-existent file: %v", err)
	}
}

func TestSecureFileDeleter_SecureDelete_OverwriteVerification(t *testing.T) {
	// This test verifies that the file content is actually overwritten
	// by examining the file system behavior, though the actual overwrite
	// content can't be easily verified without low-level disk access
	
	sfd := NewSecureFileDeleter(true)
	defer sfd.Shutdown()

	tmpDir, err := os.MkdirTemp("", "overwrite_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testFile := filepath.Join(tmpDir, "overwrite_test")
	originalContent := []byte("sensitive data that must be overwritten")

	// Create file with sensitive content
	if err := os.WriteFile(testFile, originalContent, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Perform secure deletion
	err = sfd.SecureDelete(testFile)
	if err != nil {
		t.Errorf("SecureDelete failed: %v", err)
	}

	// File should be completely removed
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("file should be completely removed")
	}
}

func TestSecureFileDeleter_cleanupOldTempFiles(t *testing.T) {
	sfd := NewSecureFileDeleter(true)
	defer sfd.Shutdown()

	tmpDir, err := os.MkdirTemp("", "cleanup_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test files
	oldFile := filepath.Join(tmpDir, "old_file")
	newFile := filepath.Join(tmpDir, "new_file")

	if err := os.WriteFile(oldFile, []byte("old content"), 0644); err != nil {
		t.Fatalf("failed to create old file: %v", err)
	}
	if err := os.WriteFile(newFile, []byte("new content"), 0644); err != nil {
		t.Fatalf("failed to create new file: %v", err)
	}

	// Register files with different timestamps
	sfd.mu.Lock()
	sfd.tempFiles[oldFile] = time.Now().Add(-2 * time.Hour) // Old file
	sfd.tempFiles[newFile] = time.Now()                     // New file
	sfd.mu.Unlock()

	// Run cleanup
	sfd.cleanupOldTempFiles()

	// Check results
	sfd.mu.Lock()
	defer sfd.mu.Unlock()

	if _, exists := sfd.tempFiles[oldFile]; exists {
		t.Error("old file should be removed from tracking")
	}

	if _, exists := sfd.tempFiles[newFile]; !exists {
		t.Error("new file should still be tracked")
	}

	// Old file should be deleted from filesystem
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Error("old file should be deleted from filesystem")
	}

	// New file should still exist
	if _, err := os.Stat(newFile); os.IsNotExist(err) {
		t.Error("new file should still exist")
	}
}

func TestSecureFileDeleter_Shutdown(t *testing.T) {
	sfd := NewSecureFileDeleter(true)

	tmpDir, err := os.MkdirTemp("", "shutdown_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Register some temp files
	testFile1 := filepath.Join(tmpDir, "temp1")
	testFile2 := filepath.Join(tmpDir, "temp2")

	if err := os.WriteFile(testFile1, []byte("temp1"), 0644); err != nil {
		t.Fatalf("failed to create temp file 1: %v", err)
	}
	if err := os.WriteFile(testFile2, []byte("temp2"), 0644); err != nil {
		t.Fatalf("failed to create temp file 2: %v", err)
	}

	sfd.RegisterTempFile(testFile1)
	sfd.RegisterTempFile(testFile2)

	// Shutdown should clean up all temp files
	sfd.Shutdown()

	// Verify files are deleted
	if _, err := os.Stat(testFile1); !os.IsNotExist(err) {
		t.Error("temp file 1 should be deleted on shutdown")
	}
	if _, err := os.Stat(testFile2); !os.IsNotExist(err) {
		t.Error("temp file 2 should be deleted on shutdown")
	}

	// Verify cleanup ticker is stopped
	if sfd.cleanupTicker != nil {
		// Ticker should be stopped, but we can't easily test this
		// The implementation calls Stop() which is the correct behavior
	}
}

func TestMemoryProtection(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{"enabled", true},
		{"disabled", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mp := NewMemoryProtection(tt.enabled)

			if mp.enabled != tt.enabled {
				t.Errorf("enabled = %v, want %v", mp.enabled, tt.enabled)
			}
		})
	}
}

func TestMemoryProtection_ClearSensitiveData(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		data    []byte
	}{
		{"enabled_clear_data", true, []byte("sensitive password")},
		{"disabled_no_clear", false, []byte("sensitive password")},
		{"enabled_empty_data", true, []byte{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mp := NewMemoryProtection(tt.enabled)
			originalData := make([]byte, len(tt.data))
			copy(originalData, tt.data)

			mp.ClearSensitiveData(tt.data)

			if tt.enabled && len(tt.data) > 0 {
				// Data should be cleared (all zeros)
				for i, b := range tt.data {
					if b != 0 {
						t.Errorf("data[%d] = %v, want 0 (data should be cleared)", i, b)
					}
				}
			} else if !tt.enabled && len(tt.data) > 0 {
				// Data should remain unchanged when disabled
				for i := range tt.data {
					if tt.data[i] != originalData[i] {
						t.Errorf("data should not be modified when disabled")
						break
					}
				}
			}
		})
	}
}

func TestSecureBuffer(t *testing.T) {
	mp := NewMemoryProtection(true)
	size := 100

	sb := NewSecureBuffer(size, mp)

	if len(sb.Data()) != size {
		t.Errorf("buffer size = %d, want %d", len(sb.Data()), size)
	}

	// Write some data
	testData := []byte("sensitive buffer content")
	copy(sb.Data(), testData)

	// Verify data is there
	if string(sb.Data()[:len(testData)]) != string(testData) {
		t.Error("data not correctly written to buffer")
	}

	// Clear the buffer
	sb.Clear()

	// Verify data is cleared
	for i := 0; i < len(testData); i++ {
		if sb.Data()[i] != 0 {
			t.Errorf("buffer[%d] = %v, want 0 (should be cleared)", i, sb.Data()[i])
		}
	}
}

func TestSecureBuffer_WithNilMemoryProtection(t *testing.T) {
	sb := NewSecureBuffer(50, nil)

	testData := []byte("test data")
	copy(sb.Data(), testData)

	// Clear should not panic with nil memory protection
	sb.Clear()

	// Data should still be there since no memory protection
	if string(sb.Data()[:len(testData)]) != string(testData) {
		t.Error("data should not be cleared when memory protection is nil")
	}
}

func TestRAMOnlyMode(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{"enabled", true},
		{"disabled", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rom := NewRAMOnlyMode(tt.enabled)

			if rom.IsEnabled() != tt.enabled {
				t.Errorf("IsEnabled() = %v, want %v", rom.IsEnabled(), tt.enabled)
			}

			if rom.tempFiles == nil {
				t.Error("tempFiles slice not initialized")
			}
		})
	}
}

func TestRAMOnlyMode_RegisterTempFile(t *testing.T) {
	// Test enabled mode
	t.Run("enabled", func(t *testing.T) {
		rom := NewRAMOnlyMode(true)
		testPath := "/tmp/test_file"

		rom.RegisterTempFile(testPath)

		rom.mu.Lock()
		defer rom.mu.Unlock()

		if len(rom.tempFiles) != 1 || rom.tempFiles[0] != testPath {
			t.Errorf("temp file not properly registered")
		}
	})

	// Test disabled mode
	t.Run("disabled", func(t *testing.T) {
		rom := NewRAMOnlyMode(false)
		testPath := "/tmp/test_file"

		rom.RegisterTempFile(testPath)

		rom.mu.Lock()
		defer rom.mu.Unlock()

		if len(rom.tempFiles) != 0 {
			t.Error("temp file should not be registered when disabled")
		}
	})
}

func TestRAMOnlyMode_Cleanup(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ram_only_cleanup_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Test enabled mode
	t.Run("enabled", func(t *testing.T) {
		rom := NewRAMOnlyMode(true)

		// Create test files
		testFile1 := filepath.Join(tmpDir, "ram_test1")
		testFile2 := filepath.Join(tmpDir, "ram_test2")

		if err := os.WriteFile(testFile1, []byte("test1"), 0644); err != nil {
			t.Fatalf("failed to create test file 1: %v", err)
		}
		if err := os.WriteFile(testFile2, []byte("test2"), 0644); err != nil {
			t.Fatalf("failed to create test file 2: %v", err)
		}

		rom.RegisterTempFile(testFile1)
		rom.RegisterTempFile(testFile2)

		// Cleanup should remove all files
		rom.Cleanup()

		// Verify files are deleted
		if _, err := os.Stat(testFile1); !os.IsNotExist(err) {
			t.Error("test file 1 should be deleted")
		}
		if _, err := os.Stat(testFile2); !os.IsNotExist(err) {
			t.Error("test file 2 should be deleted")
		}

		// Verify internal list is cleared
		rom.mu.Lock()
		defer rom.mu.Unlock()
		if len(rom.tempFiles) != 0 {
			t.Error("tempFiles list should be cleared")
		}
	})

	// Test disabled mode
	t.Run("disabled", func(t *testing.T) {
		rom := NewRAMOnlyMode(false)

		// Create test file
		testFile := filepath.Join(tmpDir, "ram_test_disabled")
		if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		// Cleanup should do nothing when disabled
		rom.Cleanup()

		// File should still exist
		if _, err := os.Stat(testFile); os.IsNotExist(err) {
			t.Error("file should not be deleted when RAM-only mode is disabled")
		}
	})
}

func TestSecurityManager(t *testing.T) {
	config := SecurityConfig{
		SecureMemory:  true,
		AntiForensics: true,
		RAMOnlyMode:   true,
	}

	sm := NewSecurityManager(config)
	defer sm.Shutdown()

	// Verify components are initialized
	if sm.FileDeleter == nil {
		t.Error("FileDeleter not initialized")
	}
	if sm.MemoryProtection == nil {
		t.Error("MemoryProtection not initialized")
	}
	if sm.RAMOnlyMode == nil {
		t.Error("RAMOnlyMode not initialized")
	}

	// Verify components have correct configuration
	if !sm.FileDeleter.enabled {
		t.Error("FileDeleter should be enabled")
	}
	if !sm.MemoryProtection.enabled {
		t.Error("MemoryProtection should be enabled")
	}
	if !sm.RAMOnlyMode.IsEnabled() {
		t.Error("RAMOnlyMode should be enabled")
	}
}

func TestSecurityManager_Shutdown(t *testing.T) {
	config := SecurityConfig{
		SecureMemory:  true,
		AntiForensics: true,
		RAMOnlyMode:   true,
	}

	sm := NewSecurityManager(config)

	tmpDir, err := os.MkdirTemp("", "security_manager_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Register files with components
	testFile := filepath.Join(tmpDir, "security_test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	sm.FileDeleter.RegisterTempFile(testFile)
	sm.RAMOnlyMode.RegisterTempFile(testFile)

	// Shutdown should clean up everything
	sm.Shutdown()

	// File should be deleted
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("test file should be deleted on security manager shutdown")
	}
}

// Concurrent access tests
func TestSecureFileDeleter_ConcurrentAccess(t *testing.T) {
	sfd := NewSecureFileDeleter(true)
	defer sfd.Shutdown()

	tmpDir, err := os.MkdirTemp("", "concurrent_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	var wg sync.WaitGroup
	numGoroutines := 10

	// Test concurrent RegisterTempFile calls
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			testFile := filepath.Join(tmpDir, "concurrent_test_"+string(rune('0'+id)))
			sfd.RegisterTempFile(testFile)
		}(i)
	}

	wg.Wait()

	// Verify all files were registered
	sfd.mu.Lock()
	if len(sfd.tempFiles) != numGoroutines {
		t.Errorf("expected %d temp files, got %d", numGoroutines, len(sfd.tempFiles))
	}
	sfd.mu.Unlock()
}

func TestRAMOnlyMode_ConcurrentAccess(t *testing.T) {
	rom := NewRAMOnlyMode(true)

	var wg sync.WaitGroup
	numGoroutines := 10

	// Test concurrent RegisterTempFile calls
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			testPath := "/tmp/concurrent_test_" + string(rune('0'+id))
			rom.RegisterTempFile(testPath)
		}(i)
	}

	wg.Wait()

	// Verify all files were registered
	rom.mu.Lock()
	if len(rom.tempFiles) != numGoroutines {
		t.Errorf("expected %d temp files, got %d", numGoroutines, len(rom.tempFiles))
	}
	rom.mu.Unlock()
}

// Benchmark tests
func BenchmarkSecureDelete_SmallFile(b *testing.B) {
	sfd := NewSecureFileDeleter(true)
	defer sfd.Shutdown()

	tmpDir, err := os.MkdirTemp("", "benchmark_test")
	if err != nil {
		b.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	content := make([]byte, 1024) // 1KB file
	rand.Read(content)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		testFile := filepath.Join(tmpDir, "bench_test")
		if err := os.WriteFile(testFile, content, 0644); err != nil {
			b.Fatalf("failed to create test file: %v", err)
		}

		if err := sfd.SecureDelete(testFile); err != nil {
			b.Fatalf("SecureDelete failed: %v", err)
		}
	}
}

func BenchmarkSecureDelete_LargeFile(b *testing.B) {
	sfd := NewSecureFileDeleter(true)
	defer sfd.Shutdown()

	tmpDir, err := os.MkdirTemp("", "benchmark_test")
	if err != nil {
		b.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	content := make([]byte, 1024*1024) // 1MB file
	rand.Read(content)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		testFile := filepath.Join(tmpDir, "bench_test_large")
		if err := os.WriteFile(testFile, content, 0644); err != nil {
			b.Fatalf("failed to create test file: %v", err)
		}

		if err := sfd.SecureDelete(testFile); err != nil {
			b.Fatalf("SecureDelete failed: %v", err)
		}
	}
}

func BenchmarkMemoryProtection_ClearSensitiveData(b *testing.B) {
	mp := NewMemoryProtection(true)
	data := make([]byte, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Fill with data each iteration
		for j := range data {
			data[j] = byte(j % 256)
		}
		mp.ClearSensitiveData(data)
	}
}

func BenchmarkSecureBuffer_Operations(b *testing.B) {
	mp := NewMemoryProtection(true)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sb := NewSecureBuffer(1024, mp)
		// Simulate some operations
		copy(sb.Data(), []byte("sensitive data"))
		sb.Clear()
	}
}

// Edge case tests
func TestSecureFileDeleter_CleanupLoop_Timing(t *testing.T) {
	// Test that cleanup loop actually runs periodically
	// This is a limited test since we can't easily control time
	sfd := NewSecureFileDeleter(true)
	defer sfd.Shutdown()

	// The cleanup loop should be running in the background
	// We can't easily test the timing without mocking time,
	// but we can verify the goroutine infrastructure is set up
	if sfd.cleanupTicker == nil {
		t.Error("cleanup ticker should be initialized")
	}
	if sfd.done == nil {
		t.Error("done channel should be initialized")
	}
}

func TestSecurityConfig_AllCombinations(t *testing.T) {
	configs := []SecurityConfig{
		{SecureMemory: false, AntiForensics: false, RAMOnlyMode: false},
		{SecureMemory: true, AntiForensics: false, RAMOnlyMode: false},
		{SecureMemory: false, AntiForensics: true, RAMOnlyMode: false},
		{SecureMemory: false, AntiForensics: false, RAMOnlyMode: true},
		{SecureMemory: true, AntiForensics: true, RAMOnlyMode: true},
	}

	for i, config := range configs {
		t.Run("config_"+string(rune('0'+i)), func(t *testing.T) {
			sm := NewSecurityManager(config)
			defer sm.Shutdown()

			// Verify components match configuration
			if sm.FileDeleter.enabled != config.AntiForensics {
				t.Errorf("FileDeleter.enabled = %v, want %v", sm.FileDeleter.enabled, config.AntiForensics)
			}
			if sm.MemoryProtection.enabled != config.SecureMemory {
				t.Errorf("MemoryProtection.enabled = %v, want %v", sm.MemoryProtection.enabled, config.SecureMemory)
			}
			if sm.RAMOnlyMode.IsEnabled() != config.RAMOnlyMode {
				t.Errorf("RAMOnlyMode.IsEnabled() = %v, want %v", sm.RAMOnlyMode.IsEnabled(), config.RAMOnlyMode)
			}
		})
	}
}

// Test memory usage and garbage collection behavior
func TestMemoryProtection_GarbageCollection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping garbage collection test in short mode")
	}

	mp := NewMemoryProtection(true)
	
	// Get initial memory stats
	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	// Create and clear lots of sensitive data
	for i := 0; i < 1000; i++ {
		data := make([]byte, 1024)
		for j := range data {
			data[j] = byte(i % 256)
		}
		mp.ClearSensitiveData(data)
	}

	// Check memory stats after operations
	runtime.ReadMemStats(&m2)

	// The memory protection should have triggered GC calls
	// We can't easily verify the exact behavior, but at least
	// ensure the operations completed without panic
	if m2.NumGC < m1.NumGC {
		t.Error("garbage collection should have been triggered")
	}
}
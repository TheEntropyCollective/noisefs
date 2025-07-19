package security

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestValidatePathInBounds(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := "/tmp/test-sync-root"
	
	tests := []struct {
		name        string
		path        string
		allowedRoot string
		expectError bool
		description string
	}{
		// Valid paths
		{
			name:        "ValidSubdirectory",
			path:        filepath.Join(tempDir, "subdir", "file.txt"),
			allowedRoot: tempDir,
			expectError: false,
			description: "Valid file in subdirectory",
		},
		{
			name:        "ValidSameDirectory",
			path:        filepath.Join(tempDir, "file.txt"),
			allowedRoot: tempDir,
			expectError: false,
			description: "Valid file in same directory",
		},
		{
			name:        "ValidRootDirectory",
			path:        tempDir,
			allowedRoot: tempDir,
			expectError: false,
			description: "Root directory itself",
		},
		
		// Path traversal attacks - Unix style
		{
			name:        "UnixTraversalParentDir",
			path:        filepath.Join(tempDir, "..", "etc", "passwd"),
			allowedRoot: tempDir,
			expectError: true,
			description: "Unix path traversal to parent directory",
		},
		{
			name:        "UnixTraversalMultipleParents",
			path:        filepath.Join(tempDir, "..", "..", "..", "etc", "passwd"),
			allowedRoot: tempDir,
			expectError: true,
			description: "Unix path traversal with multiple ../",
		},
		{
			name:        "UnixAbsolutePath",
			path:        "/etc/passwd",
			allowedRoot: tempDir,
			expectError: true,
			description: "Unix absolute path outside root",
		},
		
		// Edge cases
		{
			name:        "EmptyPath",
			path:        "",
			allowedRoot: tempDir,
			expectError: true,
			description: "Empty path should be rejected",
		},
		{
			name:        "EmptyRoot",
			path:        tempDir,
			allowedRoot: "",
			expectError: true,
			description: "Empty root should be rejected",
		},
		{
			name:        "DotPath",
			path:        ".",
			allowedRoot: tempDir,
			expectError: false,
			description: "Current directory reference",
		},
		{
			name:        "RelativePathValid",
			path:        "subdir/file.txt",
			allowedRoot: tempDir,
			expectError: false,
			description: "Valid relative path",
		},
	}
	
	// Add Windows-specific tests if running on Windows
	if runtime.GOOS == "windows" {
		windowsTests := []struct {
			name        string
			path        string
			allowedRoot string
			expectError bool
			description string
		}{
			{
				name:        "WindowsTraversalBackslash",
				path:        tempDir + "\\..\\..\\windows\\system32",
				allowedRoot: tempDir,
				expectError: true,
				description: "Windows path traversal with backslashes",
			},
			{
				name:        "WindowsAbsolutePath",
				path:        "C:\\windows\\system32",
				allowedRoot: tempDir,
				expectError: true,
				description: "Windows absolute path outside root",
			},
		}
		tests = append(tests, windowsTests...)
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePathInBounds(tt.path, tt.allowedRoot)
			
			if tt.expectError && err == nil {
				t.Errorf("Expected error for %s, but got none", tt.description)
			}
			
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error for %s: %v", tt.description, err)
			}
			
			// Check for specific path traversal error
			if tt.expectError && err != nil && strings.Contains(tt.description, "traversal") {
				if err != ErrPathTraversal && !strings.Contains(err.Error(), "path traversal") {
					t.Errorf("Expected path traversal error for %s, got: %v", tt.description, err)
				}
			}
		})
	}
}

func TestValidateSyncID(t *testing.T) {
	tests := []struct {
		name        string
		syncID      string
		expectError bool
		description string
	}{
		// Valid sync IDs
		{
			name:        "ValidAlphanumeric",
			syncID:      "sync123",
			expectError: false,
			description: "Valid alphanumeric sync ID",
		},
		{
			name:        "ValidWithHyphens",
			syncID:      "sync-session-123",
			expectError: false,
			description: "Valid sync ID with hyphens",
		},
		{
			name:        "ValidWithUnderscores",
			syncID:      "sync_session_123",
			expectError: false,
			description: "Valid sync ID with underscores",
		},
		{
			name:        "ValidMixed",
			syncID:      "sync-session_123-test",
			expectError: false,
			description: "Valid sync ID with mixed safe characters",
		},
		
		// Invalid sync IDs (path traversal attempts)
		{
			name:        "PathTraversalDots",
			syncID:      "../../../etc/passwd",
			expectError: true,
			description: "Path traversal with dots",
		},
		{
			name:        "PathTraversalSlashes",
			syncID:      "sync/../../etc/passwd",
			expectError: true,
			description: "Path traversal with slashes",
		},
		{
			name:        "AbsolutePath",
			syncID:      "/etc/passwd",
			expectError: true,
			description: "Absolute path",
		},
		{
			name:        "WindowsPath",
			syncID:      "..\\..\\windows\\system32",
			expectError: true,
			description: "Windows path traversal",
		},
		{
			name:        "SpecialCharacters",
			syncID:      "sync@!#$",
			expectError: true,
			description: "Special characters in sync ID",
		},
		{
			name:        "EmptySyncID",
			syncID:      "",
			expectError: true,
			description: "Empty sync ID",
		},
		{
			name:        "TooLongSyncID",
			syncID:      strings.Repeat("a", 101),
			expectError: true,
			description: "Sync ID too long (>100 chars)",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSyncID(tt.syncID)
			
			if tt.expectError && err == nil {
				t.Errorf("Expected error for %s, but got none", tt.description)
			}
			
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error for %s: %v", tt.description, err)
			}
		})
	}
}

func TestValidateFileName(t *testing.T) {
	tests := []struct {
		name        string
		fileName    string
		expectError bool
		description string
	}{
		// Valid file names
		{
			name:        "ValidSimpleFile",
			fileName:    "file.txt",
			expectError: false,
			description: "Valid simple file name",
		},
		{
			name:        "ValidFileWithSpaces",
			fileName:    "my file.txt",
			expectError: false,
			description: "Valid file name with spaces",
		},
		{
			name:        "ValidFileWithNumbers",
			fileName:    "file123.txt",
			expectError: false,
			description: "Valid file name with numbers",
		},
		
		// Invalid file names (path traversal attempts)
		{
			name:        "PathTraversalDots",
			fileName:    "../../../etc/passwd",
			expectError: true,
			description: "Path traversal with dots",
		},
		{
			name:        "PathTraversalInMiddle",
			fileName:    "file/../../../etc/passwd",
			expectError: true,
			description: "Path traversal in middle of filename",
		},
		{
			name:        "AbsolutePath",
			fileName:    "/etc/passwd",
			expectError: true,
			description: "Absolute path as filename",
		},
		{
			name:        "EmptyFileName",
			fileName:    "",
			expectError: true,
			description: "Empty file name",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFileName(tt.fileName)
			
			if tt.expectError && err == nil {
				t.Errorf("Expected error for %s, but got none", tt.description)
			}
			
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error for %s: %v", tt.description, err)
			}
		})
	}
}

// Benchmark tests for performance verification
func BenchmarkValidatePathInBounds(b *testing.B) {
	tempDir := "/tmp/test-sync-root"
	validPath := filepath.Join(tempDir, "subdir", "file.txt")
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ValidatePathInBounds(validPath, tempDir)
	}
}

func BenchmarkValidateSyncID(b *testing.B) {
	syncID := "valid-sync-id-123"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ValidateSyncID(syncID)
	}
}
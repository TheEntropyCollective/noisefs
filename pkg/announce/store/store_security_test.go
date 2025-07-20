package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/announce"
)

// TestFilenameSecurityValidation tests the security validation of filename components
func TestFilenameSecurityValidation(t *testing.T) {
	testCases := []struct {
		name       string
		descriptor string
		nonce      string
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "valid_components",
			descriptor: "QmTest123",
			nonce:      "random123",
			wantErr:    false,
		},
		{
			name:       "empty_descriptor",
			descriptor: "",
			nonce:      "random123",
			wantErr:    true,
			errMsg:     "descriptor cannot be empty",
		},
		{
			name:       "empty_nonce",
			descriptor: "QmTest123",
			nonce:      "",
			wantErr:    true,
			errMsg:     "nonce cannot be empty",
		},
		{
			name:       "path_traversal_descriptor",
			descriptor: "QmTest/../../../etc/passwd",
			nonce:      "random123",
			wantErr:    true,
			errMsg:     "path traversal attempt detected",
		},
		{
			name:       "path_traversal_nonce",
			descriptor: "QmTest123",
			nonce:      "../../../etc/passwd",
			wantErr:    true,
			errMsg:     "path traversal attempt detected",
		},
		{
			name:       "null_byte_descriptor",
			descriptor: "QmTest123\x00; rm -rf /",
			nonce:      "random123",
			wantErr:    true,
			errMsg:     "null bytes detected",
		},
		{
			name:       "null_byte_nonce",
			descriptor: "QmTest123",
			nonce:      "random123\x00; cat /etc/passwd",
			wantErr:    true,
			errMsg:     "null bytes detected",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateFilenameComponents(tc.descriptor, tc.nonce)
			
			if tc.wantErr {
				if err == nil {
					t.Errorf("validateFilenameComponents(%q, %q) expected error but got none", tc.descriptor, tc.nonce)
				} else if tc.errMsg != "" && !strings.Contains(err.Error(), tc.errMsg) {
					t.Errorf("validateFilenameComponents(%q, %q) error = %v, want error containing %q", tc.descriptor, tc.nonce, err, tc.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateFilenameComponents(%q, %q) unexpected error: %v", tc.descriptor, tc.nonce, err)
				}
			}
		})
	}
}

// TestSanitizeForFilename tests the filename sanitization function
func TestSanitizeForFilename(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "valid_input",
			input:    "QmTest123",
			expected: "QmTest123",
		},
		{
			name:     "path_separators",
			input:    "Qm/Test\\123",
			expected: "Qm_Test_123",
		},
		{
			name:     "path_traversal",
			input:    "QmTest../../etc",
			expected: "QmTest____etc",
		},
		{
			name:     "null_bytes",
			input:    "QmTest\x00123",
			expected: "QmTest_123",
		},
		{
			name:     "special_characters",
			input:    "QmTest@#$%^&*()",
			expected: "QmTest_________",
		},
		{
			name:     "empty_input",
			input:    "",
			expected: "unknown",
		},
		{
			name:     "too_long_input",
			input:    strings.Repeat("a", 100),
			expected: strings.Repeat("a", 50),
		},
		{
			name:     "mixed_dangerous_chars",
			input:    "QmTest/../..\\/..\\\\;rm -rf /",
			expected: "QmTest__________rm_-rf__",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := sanitizeForFilename(tc.input)
			if result != tc.expected {
				t.Errorf("sanitizeForFilename(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

// TestStorageSecurityIntegration tests the complete storage security with malicious inputs
func TestStorageSecurityIntegration(t *testing.T) {
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "store_security_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create store
	config := DefaultStoreConfig(tempDir)
	store, err := NewStore(config)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	t.Run("MaliciousDescriptorBlocked", func(t *testing.T) {
		// Create announcement with malicious descriptor
		ann := &announce.Announcement{
			Version:     "1.0",
			Descriptor:  "../../../etc/passwd", // Path traversal attempt
			TopicHash:   "a1b2c3d4e5f6",
			Timestamp:   time.Now().Unix(),
			TTL:         3600,
			Nonce:       "safe_nonce",
		}

		err := store.Add(ann, "test")
		if err == nil {
			t.Error("Expected error when adding announcement with malicious descriptor")
		}
		
		if !strings.Contains(err.Error(), "path traversal") {
			t.Errorf("Expected path traversal error, got: %v", err)
		}
	})

	t.Run("MaliciousNonceBlocked", func(t *testing.T) {
		// Create announcement with malicious nonce
		ann := &announce.Announcement{
			Version:     "1.0",
			Descriptor:  "QmSafeDescriptor123",
			TopicHash:   "a1b2c3d4e5f6",
			Timestamp:   time.Now().Unix(),
			TTL:         3600,
			Nonce:       "safe_nonce\x00; rm -rf /", // Null byte injection
		}

		err := store.Add(ann, "test")
		if err == nil {
			t.Error("Expected error when adding announcement with malicious nonce")
		}
		
		if !strings.Contains(err.Error(), "null bytes") {
			t.Errorf("Expected null byte error, got: %v", err)
		}
	})

	t.Run("ValidAnnouncementStored", func(t *testing.T) {
		// Create valid announcement
		ann := &announce.Announcement{
			Version:     "1.0",
			Descriptor:  "QmValidDescriptor123",
			TopicHash:   "a1b2c3d4e5f6789012345678901234567890123456789012345678901234",
			Timestamp:   time.Now().Unix(),
			TTL:         3600,
			Nonce:       "valid_nonce_123",
		}

		err := store.Add(ann, "test")
		if err != nil {
			t.Errorf("Expected valid announcement to be stored, got error: %v", err)
			return // Skip the rest of the test if storage failed
		}

		// Verify file was created safely
		files, err := os.ReadDir(tempDir)
		if err != nil {
			t.Fatalf("Failed to read temp dir: %v", err)
		}


		found := false
		for _, file := range files {
			if strings.Contains(file.Name(), "QmValidD") && strings.Contains(file.Name(), "valid_nonce_123") {
				found = true
				// Verify filename is safe (no path separators)
				if strings.Contains(file.Name(), "/") || strings.Contains(file.Name(), "\\") || strings.Contains(file.Name(), "..") {
					t.Errorf("Generated filename contains dangerous characters: %s", file.Name())
				}
				break
			}
		}

		if !found {
			t.Error("Valid announcement file was not created - check logs above for actual files")
		}
	})

	t.Run("NoPathEscapeAttempts", func(t *testing.T) {
		// Try various path escape attempts
		maliciousInputs := []struct {
			descriptor string
			nonce      string
		}{
			{"../../../root/.ssh/id_rsa", "normal_nonce"},
			{"QmNormal123", "../../etc/passwd"},
			{"..\\..\\windows\\system32", "normal_nonce"},
			{"QmNormal123", "..\\..\\boot.ini"},
			{"/etc/passwd", "normal_nonce"},
			{"QmNormal123", "/etc/shadow"},
			{"C:\\Windows\\System32\\cmd.exe", "normal_nonce"},
		}

		for i, input := range maliciousInputs {
			ann := &announce.Announcement{
				Version:     "1.0",
				Descriptor:  input.descriptor,
				TopicHash:   "a1b2c3d4e5f6789012345678901234567890123456789012345678901234",
				Timestamp:   time.Now().Unix(),
				TTL:         3600,
				Nonce:       input.nonce,
			}

			err := store.Add(ann, "test")
			if err == nil {
				t.Errorf("Test %d: Expected error for malicious input descriptor=%q nonce=%q", i, input.descriptor, input.nonce)
			}
		}

		// Verify no files were created outside the temp directory
		err := filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			// All files should be within tempDir
			relPath, err := filepath.Rel(tempDir, path)
			if err != nil {
				t.Errorf("Failed to get relative path for %s: %v", path, err)
				return nil
			}
			if strings.Contains(relPath, "..") {
				t.Errorf("File created outside temp directory: %s", path)
			}
			return nil
		})
		if err != nil {
			t.Errorf("Failed to walk temp directory: %v", err)
		}
	})
}
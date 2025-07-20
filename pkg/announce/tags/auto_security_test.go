package tags

import (
	"strings"
	"testing"
)

// TestValidateFilePath tests the security validation of file paths
func TestValidateFilePath(t *testing.T) {
	testCases := []struct {
		name     string
		filePath string
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "valid_absolute_path",
			filePath: "/home/user/file.mp4",
			wantErr:  false,
		},
		{
			name:     "valid_relative_path",
			filePath: "file.mp4",
			wantErr:  false,
		},
		{
			name:     "empty_path",
			filePath: "",
			wantErr:  true,
			errMsg:   "file path cannot be empty",
		},
		{
			name:     "null_byte_injection",
			filePath: "/home/user/file.mp4\x00; rm -rf /",
			wantErr:  true,
			errMsg:   "file path contains null bytes",
		},
		{
			name:     "command_injection_semicolon",
			filePath: "/home/user/file.mp4; rm -rf /",
			wantErr:  true,
			errMsg:   "file path contains dangerous character: ;",
		},
		{
			name:     "command_injection_pipe",
			filePath: "/home/user/file.mp4 | cat /etc/passwd",
			wantErr:  true,
			errMsg:   "file path contains dangerous character: |",
		},
		{
			name:     "command_injection_backtick",
			filePath: "/home/user/file.mp4`whoami`",
			wantErr:  true,
			errMsg:   "file path contains dangerous character: `",
		},
		{
			name:     "command_injection_dollar",
			filePath: "/home/user/file.mp4$(whoami)",
			wantErr:  true,
			errMsg:   "file path contains dangerous character: $",
		},
		{
			name:     "path_traversal",
			filePath: "/home/user/../../../etc/passwd",
			wantErr:  true,
			errMsg:   "file path contains path traversal sequence",
		},
		{
			name:     "path_traversal_relative",
			filePath: "../../../etc/passwd",
			wantErr:  true,
			errMsg:   "file path contains path traversal sequence",
		},
		{
			name:     "very_long_path",
			filePath: "/" + strings.Repeat("a", 5000), // Path longer than 4096 chars
			wantErr:  true,
			errMsg:   "file path too long",
		},
		{
			name:     "quote_injection",
			filePath: "/home/user/file.mp4' cat /etc/passwd",
			wantErr:  true,
			errMsg:   "file path contains dangerous character: '",
		},
		{
			name:     "double_quote_injection",
			filePath: "/home/user/file.mp4\" cat /etc/passwd",
			wantErr:  true,
			errMsg:   "file path contains dangerous character: \"",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateFilePath(tc.filePath)
			
			if tc.wantErr {
				if err == nil {
					t.Errorf("validateFilePath(%q) expected error but got none", tc.filePath)
				} else if tc.errMsg != "" && !contains(err.Error(), tc.errMsg) {
					t.Errorf("validateFilePath(%q) error = %v, want error containing %q", tc.filePath, err, tc.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateFilePath(%q) unexpected error: %v", tc.filePath, err)
				}
			}
		})
	}
}

// TestExtractMediaTagsSecurity tests that extractMediaTags is protected against injection
func TestExtractMediaTagsSecurity(t *testing.T) {
	at := NewAutoTagger()
	
	// Test malicious file paths are rejected
	maliciousPaths := []string{
		"/tmp/file.mp4; rm -rf /",
		"/tmp/file.mp4`whoami`",
		"/tmp/file.mp4$(id)",
		"/tmp/../../../etc/passwd",
		"/tmp/file.mp4\x00; cat /etc/passwd",
	}
	
	for _, path := range maliciousPaths {
		t.Run("malicious_path_"+path, func(t *testing.T) {
			_, err := at.extractMediaTags(path)
			if err == nil {
				t.Errorf("extractMediaTags(%q) should have rejected malicious path", path)
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
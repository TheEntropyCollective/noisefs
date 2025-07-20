package security

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeError(t *testing.T) {
	tests := []struct {
		name           string
		input          error
		publicPath     string
		preserveContext bool
		expectedContains []string
		shouldNotContain []string
	}{
		{
			name:           "nil error",
			input:          nil,
			publicPath:     "",
			preserveContext: false,
			expectedContains: nil,
			shouldNotContain: nil,
		},
		{
			name:           "unix path sanitization",
			input:          errors.New("failed to read /home/user/secret/documents/file.txt"),
			publicPath:     "",
			preserveContext: false,
			expectedContains: []string{"failed to read", "[PATH]"},
			shouldNotContain: []string{"/home/user", "secret", "documents"},
		},
		{
			name:           "windows path sanitization",
			input:          errors.New("cannot write to C:\\Users\\john\\Documents\\secret.docx"),
			publicPath:     "",
			preserveContext: false,
			expectedContains: []string{"cannot write to", "[PATH]"},
			shouldNotContain: []string{"C:\\Users", "john", "Documents"},
		},
		{
			name:           "ip address sanitization",
			input:          errors.New("connection failed to 192.168.1.100:5001"),
			publicPath:     "",
			preserveContext: false,
			expectedContains: []string{"connection failed to", "[IP_ADDRESS]"},
			shouldNotContain: []string{"192.168.1.100"},
		},
		{
			name:           "email sanitization",
			input:          errors.New("notification sent to user@company.com failed"),
			publicPath:     "",
			preserveContext: false,
			expectedContains: []string{"notification sent to", "[EMAIL]", "failed"},
			shouldNotContain: []string{"user@company.com"},
		},
		{
			name:           "public path preservation",
			input:          errors.New("failed to upload ./documents/report.pdf"),
			publicPath:     "./documents/report.pdf",
			preserveContext: false,
			expectedContains: []string{"failed to upload", "./documents/report.pdf"},
			shouldNotContain: []string{"[PATH]"},
		},
		{
			name:           "context preservation for file extensions",
			input:          errors.New("error processing /sensitive/path/document.pdf"),
			publicPath:     "",
			preserveContext: true,
			expectedContains: []string{"error processing", "[FILE.pdf]"},
			shouldNotContain: []string{"/sensitive/path", "document.pdf"},
		},
		{
			name:           "context preservation for directories",
			input:          errors.New("cannot access /var/log/sensitive/"),
			publicPath:     "",
			preserveContext: true,
			expectedContains: []string{"cannot access", "[DIRECTORY]"},
			shouldNotContain: []string{"/var/log/sensitive"},
		},
		{
			name:           "mixed sensitive data",
			input:          errors.New("sync failed: cannot reach admin@server.com at 10.0.0.5 for /home/admin/sync/data"),
			publicPath:     "",
			preserveContext: false,
			expectedContains: []string{"sync failed", "[EMAIL]", "[IP_ADDRESS]", "[PATH]"},
			shouldNotContain: []string{"admin@server.com", "10.0.0.5", "/home/admin"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeError(tt.input, tt.publicPath, tt.preserveContext)
			
			if tt.input == nil {
				if result != nil {
					t.Errorf("expected nil result for nil input, got %v", result)
				}
				return
			}

			resultStr := result.Error()
			
			for _, expected := range tt.expectedContains {
				if !strings.Contains(resultStr, expected) {
					t.Errorf("expected result to contain %q, got: %s", expected, resultStr)
				}
			}

			for _, shouldNotContain := range tt.shouldNotContain {
				if strings.Contains(resultStr, shouldNotContain) {
					t.Errorf("result should not contain %q, got: %s", shouldNotContain, resultStr)
				}
			}
		})
	}
}

func TestSanitizeString(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		publicPath     string
		preserveContext bool
		expected       string
	}{
		{
			name:           "empty string",
			input:          "",
			publicPath:     "",
			preserveContext: false,
			expected:       "",
		},
		{
			name:           "no sensitive data",
			input:          "operation completed successfully",
			publicPath:     "",
			preserveContext: false,
			expected:       "operation completed successfully",
		},
		{
			name:           "multiple paths",
			input:          "copying from /src/file.txt to /dst/file.txt",
			publicPath:     "",
			preserveContext: false,
			expected:       "copying from [PATH] to [PATH]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeString(tt.input, tt.publicPath, tt.preserveContext)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestPathSanitization(t *testing.T) {
	// Create a temporary directory for testing relative path logic
	tempDir, err := os.MkdirTemp("", "sanitizer_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Change to temp directory
	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(tempDir)

	// Create a test file
	testFile := filepath.Join(tempDir, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	tests := []struct {
		name           string
		input          string
		preserveContext bool
		shouldContain  string
	}{
		{
			name:           "relative path with context",
			input:          fmt.Sprintf("error reading %s", testFile),
			preserveContext: true,
			shouldContain:  "./test.txt",
		},
		{
			name:           "relative path without context",
			input:          fmt.Sprintf("error reading %s", testFile),
			preserveContext: false,
			shouldContain:  "[PATH]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeString(tt.input, "", tt.preserveContext)
			if !strings.Contains(result, tt.shouldContain) {
				t.Errorf("expected result to contain %q, got: %s", tt.shouldContain, result)
			}
		})
	}
}

func TestIPAddressSanitization(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		preserveContext bool
		expected       string
	}{
		{
			name:           "public IP without context",
			input:          "connecting to 8.8.8.8",
			preserveContext: false,
			expected:       "connecting to [IP_ADDRESS]",
		},
		{
			name:           "private IP with context",
			input:          "connecting to 192.168.1.1",
			preserveContext: true,
			expected:       "connecting to [LOCAL_IP]",
		},
		{
			name:           "localhost with context",
			input:          "connecting to 127.0.0.1",
			preserveContext: true,
			expected:       "connecting to [LOCAL_IP]",
		},
		{
			name:           "private IP without context",
			input:          "connecting to 10.0.0.1",
			preserveContext: false,
			expected:       "connecting to [IP_ADDRESS]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeString(tt.input, "", tt.preserveContext)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestConvenienceFunctions(t *testing.T) {
	sensitiveErr := errors.New("failed to access /home/user/secret.txt from 192.168.1.100")
	userPath := "./secret.txt"

	// Test SanitizeErrorForUser
	userResult := SanitizeErrorForUser(sensitiveErr, userPath)
	userStr := userResult.Error()
	
	if strings.Contains(userStr, "/home/user") || strings.Contains(userStr, "192.168.1.100") {
		t.Errorf("SanitizeErrorForUser should remove sensitive data, got: %s", userStr)
	}

	// Test SanitizeErrorForDebug
	debugResult := SanitizeErrorForDebug(sensitiveErr, userPath)
	debugStr := debugResult.Error()
	
	if strings.Contains(debugStr, "/home/user") || strings.Contains(debugStr, "192.168.1.100") {
		t.Errorf("SanitizeErrorForDebug should remove sensitive data, got: %s", debugStr)
	}

	// Test SanitizeForLogging
	logResult := SanitizeForLogging("error: /sensitive/path and user@domain.com")
	if strings.Contains(logResult, "/sensitive/path") || strings.Contains(logResult, "user@domain.com") {
		t.Errorf("SanitizeForLogging should remove all sensitive data, got: %s", logResult)
	}

	// Test SanitizeForVerbose
	verboseResult := SanitizeForVerbose("processing /path/to/file.txt", "./file.txt")
	if strings.Contains(verboseResult, "/path/to") {
		t.Errorf("SanitizeForVerbose should remove sensitive paths, got: %s", verboseResult)
	}
}

func TestRegexPatterns(t *testing.T) {
	patterns := getPatterns()

	// Test Unix path pattern
	unixTests := []struct {
		input    string
		shouldMatch bool
	}{
		{"/home/user/file.txt", true},
		{"/usr/local/bin", true},
		{"/var/log/app.log", true},
		{"./relative/path", false}, // Relative paths should not match
		{"filename.txt", false},
		{"/single", false}, // Should not match single-level paths
	}

	for _, tt := range unixTests {
		matches := patterns.UnixPaths.MatchString(tt.input)
		if matches != tt.shouldMatch {
			t.Errorf("Unix path pattern for %q: expected %v, got %v", tt.input, tt.shouldMatch, matches)
		}
	}

	// Test Windows path pattern
	windowsTests := []struct {
		input    string
		shouldMatch bool
	}{
		{"C:\\Windows\\System32", true},
		{"D:\\Users\\John\\Documents", true},
		{"\\\\server\\share\\file", true},
		{"C:\\file.txt", true},
		{"file.txt", false},
		{"./relative", false},
	}

	for _, tt := range windowsTests {
		matches := patterns.WindowsPaths.MatchString(tt.input)
		if matches != tt.shouldMatch {
			t.Errorf("Windows path pattern for %q: expected %v, got %v", tt.input, tt.shouldMatch, matches)
		}
	}

	// Test IP address pattern
	ipTests := []struct {
		input    string
		shouldMatch bool
	}{
		{"192.168.1.1", true},
		{"8.8.8.8", true},
		{"127.0.0.1", true},
		{"300.300.300.300", true}, // Invalid but matches pattern
		{"192.168.1", false},
		{"not.an.ip", false},
	}

	for _, tt := range ipTests {
		matches := patterns.IPAddresses.MatchString(tt.input)
		if matches != tt.shouldMatch {
			t.Errorf("IP address pattern for %q: expected %v, got %v", tt.input, tt.shouldMatch, matches)
		}
	}
}

func TestPerformance(t *testing.T) {
	// Test that regex compilation is cached
	patterns1 := getPatterns()
	patterns2 := getPatterns()
	
	if patterns1 != patterns2 {
		t.Error("patterns should be cached and return same instance")
	}

	// Benchmark sanitization performance
	testMessage := "error processing /home/user/documents/secret.txt from 192.168.1.100 for admin@company.com"
	
	// Warm up
	for i := 0; i < 10; i++ {
		SanitizeString(testMessage, "", false)
	}

	// This is more of a sanity check than a strict benchmark
	// In a real scenario, you'd use testing.B for proper benchmarking
	start := len(testMessage)
	result := SanitizeString(testMessage, "", false)
	end := len(result)
	
	if start == 0 || end == 0 {
		t.Error("sanitization should process the message")
	}
}

func TestCrossplatformPaths(t *testing.T) {
	tests := []struct {
		name  string
		input string
		shouldSanitize bool
	}{
		{
			name:  "unix absolute path",
			input: "/home/user/file.txt",
			shouldSanitize: true,
		},
		{
			name:  "windows absolute path",
			input: "C:\\Users\\file.txt",
			shouldSanitize: true,
		},
		{
			name:  "windows UNC path",
			input: "\\\\server\\share\\file.txt",
			shouldSanitize: true,
		},
		{
			name:  "relative path",
			input: "./relative/file.txt",
			shouldSanitize: false,
		},
		{
			name:  "filename only",
			input: "file.txt",
			shouldSanitize: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := fmt.Sprintf("error with %s", tt.input)
			sanitized := SanitizeString(original, "", false)
			
			containsOriginalPath := strings.Contains(sanitized, tt.input)
			
			if tt.shouldSanitize && containsOriginalPath {
				t.Errorf("expected %q to be sanitized, but found in result: %s", tt.input, sanitized)
			}
			
			if !tt.shouldSanitize && !containsOriginalPath {
				t.Errorf("expected %q to be preserved, but not found in result: %s", tt.input, sanitized)
			}
		})
	}
}
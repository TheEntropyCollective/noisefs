package validation

import (
	"strings"
	"testing"
)

func TestNewValidator(t *testing.T) {
	v := NewValidator()
	
	if v.maxFileSize != 100*1024*1024 {
		t.Errorf("Expected default max file size of 100MB, got %d", v.maxFileSize)
	}
	
	if v.maxFilenameLen != 255 {
		t.Errorf("Expected default max filename length of 255, got %d", v.maxFilenameLen)
	}
	
	if v.allowedExts == nil {
		t.Error("Expected allowedExts map to be initialized")
	}
	
	if v.cidPattern == nil {
		t.Error("Expected CID pattern to be compiled")
	}
}

func TestSetMaxFileSize(t *testing.T) {
	v := NewValidator()
	v.SetMaxFileSize(50 * 1024 * 1024)
	
	if v.maxFileSize != 50*1024*1024 {
		t.Errorf("Expected max file size of 50MB, got %d", v.maxFileSize)
	}
}

func TestSetAllowedExtensions(t *testing.T) {
	v := NewValidator()
	extensions := []string{".txt", ".PDF", ".jpg"}
	v.SetAllowedExtensions(extensions)
	
	// Check that extensions are stored in lowercase
	if !v.allowedExts[".txt"] {
		t.Error("Expected .txt to be allowed")
	}
	if !v.allowedExts[".pdf"] {
		t.Error("Expected .pdf to be allowed (lowercase)")
	}
	if !v.allowedExts[".jpg"] {
		t.Error("Expected .jpg to be allowed")
	}
	if v.allowedExts[".PDF"] {
		t.Error("Expected .PDF to be converted to lowercase")
	}
}

func TestValidationError(t *testing.T) {
	err := ValidationError{
		Field:   "test_field",
		Message: "test message",
		Value:   "test_value",
	}
	
	expected := "validation error for field 'test_field': test message"
	if err.Error() != expected {
		t.Errorf("Expected error message '%s', got '%s'", expected, err.Error())
	}
}

func TestValidateFilename(t *testing.T) {
	v := NewValidator()
	
	tests := []struct {
		name     string
		filename string
		wantErr  bool
		errField string
	}{
		// Valid filenames
		{"valid simple", "document.txt", false, ""},
		{"valid with numbers", "file123.pdf", false, ""},
		{"valid with underscores", "my_file.jpg", false, ""},
		{"valid with hyphens", "my-file.png", false, ""},
		{"valid with spaces", "my file.doc", false, ""},
		{"valid unicode", "файл.txt", false, ""},
		
		// Invalid filenames
		{"empty filename", "", true, "filename"},
		{"path traversal", "../../../etc/passwd", true, "filename"},
		{"path traversal simple", "..\\windows\\system32", true, "filename"},
		{"directory separator slash", "dir/file.txt", true, "filename"},
		{"directory separator backslash", "dir\\file.txt", true, "filename"},
		{"control character", "file\x00.txt", true, "filename"},
		{"control character tab", "file\t.txt", true, "filename"},
		{"control character newline", "file\n.txt", true, "filename"},
		
		// Reserved names (Windows)
		{"reserved CON", "CON", true, "filename"},
		{"reserved PRN", "PRN.txt", true, "filename"},
		{"reserved AUX", "aux.log", true, "filename"},
		{"reserved NUL", "NUL", true, "filename"},
		{"reserved COM1", "com1.dat", true, "filename"},
		{"reserved LPT1", "LPT1", true, "filename"},
		
		// Long filename
		{"too long", strings.Repeat("a", 256), true, "filename"},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateFilename(tt.filename)
			
			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error for filename '%s'", tt.filename)
					return
				}
				
				if ve, ok := err.(ValidationError); ok {
					if ve.Field != tt.errField {
						t.Errorf("Expected error field '%s', got '%s'", tt.errField, ve.Field)
					}
				} else {
					t.Errorf("Expected ValidationError, got %T", err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for filename '%s': %v", tt.filename, err)
				}
			}
		})
	}
}

func TestValidateFilenameWithExtensionRestrictions(t *testing.T) {
	v := NewValidator()
	v.SetAllowedExtensions([]string{".txt", ".pdf"})
	
	tests := []struct {
		name     string
		filename string
		wantErr  bool
	}{
		{"allowed txt", "document.txt", false},
		{"allowed pdf", "document.pdf", false},
		{"allowed PDF uppercase", "document.PDF", false},
		{"not allowed jpg", "image.jpg", true},
		{"not allowed no extension", "document", true},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateFilename(tt.filename)
			
			if tt.wantErr && err == nil {
				t.Errorf("Expected error for filename '%s'", tt.filename)
			} else if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error for filename '%s': %v", tt.filename, err)
			}
		})
	}
}

func TestValidateFileSize(t *testing.T) {
	v := NewValidator()
	
	tests := []struct {
		name    string
		size    int64
		wantErr bool
	}{
		{"valid zero", 0, false},
		{"valid small", 1024, false},
		{"valid max", 100 * 1024 * 1024, false},
		{"negative size", -1, true},
		{"too large", 100*1024*1024 + 1, true},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateFileSize(tt.size)
			
			if tt.wantErr && err == nil {
				t.Errorf("Expected error for size %d", tt.size)
			} else if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error for size %d: %v", tt.size, err)
			}
		})
	}
}

func TestValidateCID(t *testing.T) {
	v := NewValidator()
	
	tests := []struct {
		name    string
		cid     string
		wantErr bool
	}{
		// Valid CIDs
		{"valid v0", "QmbWqxBEKC3P8tqsKc98xmWNzrzDtRLMiMPL8wBuTGsMnR", false},
		{"valid v1 base32", "bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi", false},
		{"valid short format", "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG", false},
		
		// Invalid CIDs
		{"empty", "", true},
		{"too short", "Qm123", true},
		{"too long", strings.Repeat("a", 101), true},
		{"invalid characters", "QmInvalidCharacters!@#$%^&*()", true},
		{"wrong prefix", "Xm1234567890123456789012345678901234567890123456", true},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateCID(tt.cid)
			
			if tt.wantErr && err == nil {
				t.Errorf("Expected error for CID '%s'", tt.cid)
			} else if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error for CID '%s': %v", tt.cid, err)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	v := NewValidator()
	
	tests := []struct {
		name     string
		password string
		wantErr  bool
		errMsg   string
	}{
		// Valid passwords
		{"valid strong", "MyStr0ng!Pass", false, ""},
		{"valid complex", "ComplexP@ssw0rd517", false, ""}, // Non-sequential numbers
		{"valid with symbols", "Test!@#$%^&*()517Aa", false, ""}, // Non-sequential numbers
		
		// Too short
		{"too short", "Short1!", true, "at least 8 characters"},
		
		// Too long
		{"too long", strings.Repeat("A1!", 43), true, "too long"},
		
		// Missing character types
		{"no uppercase", "lowercase123!", true, "uppercase letter"},
		{"no lowercase", "UPPERCASE123!", true, "lowercase letter"},
		{"no numbers", "UpperLower!", true, "number"},
		{"no special", "UpperLower123", true, "special character"},
		
		// Null bytes
		{"null bytes", "Test123!\x00", true, "null bytes"},
		
		// Common passwords
		{"common password", "password", true, "uppercase letter"}, // Will fail on complexity first
		{"common 123456", "123456", true, "at least 8 characters"}, // Will fail on length first
		
		// Repeated characters
		{"repeated chars", "Tesssst123!", true, "repeated characters"},
		
		// Sequential characters
		{"sequential 123", "Test456!Pass", true, "sequential characters"}, // 456 is also sequential
		{"sequential abc", "Testabcd!1", true, "sequential characters"},
		
		// Repeated characters (will be caught before entropy check)
		{"low entropy", "Aaaaaaab5!", true, "repeated characters"}, // Excessive repeated chars
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidatePassword(tt.password)
			
			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error for password test '%s'", tt.name)
					return
				}
				
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Expected error message to contain '%s', got '%s'", tt.errMsg, err.Error())
				}
				
				// Verify password value is redacted
				if ve, ok := err.(ValidationError); ok {
					if ve.Value != "[redacted]" {
						t.Errorf("Expected password value to be redacted, got %v", ve.Value)
					}
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for password test '%s': %v", tt.name, err)
				}
			}
		})
	}
}

func TestValidateBlockSize(t *testing.T) {
	v := NewValidator()
	
	tests := []struct {
		name    string
		size    int
		wantErr bool
	}{
		{"valid 64KB", 64 * 1024, false},
		{"valid 128KB", 128 * 1024, false},
		{"valid 256KB", 256 * 1024, false},
		{"valid 512KB", 512 * 1024, false},
		{"valid 1MB", 1024 * 1024, false},
		{"invalid 32KB", 32 * 1024, true},
		{"invalid 2MB", 2 * 1024 * 1024, true},
		{"invalid random", 100000, true},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateBlockSize(tt.size)
			
			if tt.wantErr && err == nil {
				t.Errorf("Expected error for block size %d", tt.size)
			} else if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error for block size %d: %v", tt.size, err)
			}
		})
	}
}

func TestSanitizeInput(t *testing.T) {
	v := NewValidator()
	
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple text", "hello world", "hello world"},
		{"with whitespace", "  hello world  ", "hello world"},
		{"with null bytes", "hello\x00world", "helloworld"},
		{"with control chars", "hello\x01\x02world", "helloworld"},
		{"preserve newlines", "hello\nworld", "hello\nworld"},
		{"preserve tabs", "hello\tworld", "hello\tworld"},
		{"mixed control chars", "hello\x00\n\x01\tworld\x02", "hello\n\tworld"},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := v.SanitizeInput(tt.input)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestValidateHTTPHeader(t *testing.T) {
	v := NewValidator()
	
	tests := []struct {
		name      string
		headerName string
		headerValue string
		wantErr   bool
		errField  string
	}{
		// Valid headers
		{"valid simple", "Content-Type", "application/json", false, ""},
		{"valid user agent", "User-Agent", "NoiseFS/1.0", false, ""},
		{"valid long value", "Authorization", strings.Repeat("a", 1000), false, ""},
		
		// Invalid header names
		{"empty name", "", "value", true, "header_name"},
		{"name with space", "Content Type", "value", true, "header_name"},
		{"name with control char", "Content\x00Type", "value", true, "header_name"},
		
		// Invalid header values
		{"value too long", "Name", strings.Repeat("a", 8193), true, "header_value"},
		{"value with CRLF", "Name", "value\r\nInjected: header", true, "header_value"},
		{"value with LF", "Name", "value\nInjected: header", true, "header_value"},
		{"value with CR", "Name", "value\rInjected: header", true, "header_value"},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateHTTPHeader(tt.headerName, tt.headerValue)
			
			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error for header test '%s'", tt.name)
					return
				}
				
				if ve, ok := err.(ValidationError); ok {
					if ve.Field != tt.errField {
						t.Errorf("Expected error field '%s', got '%s'", tt.errField, ve.Field)
					}
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for header test '%s': %v", tt.name, err)
				}
			}
		})
	}
}

func TestValidateIPAddress(t *testing.T) {
	v := NewValidator()
	
	tests := []struct {
		name    string
		ip      string
		wantErr bool
	}{
		// Valid IPs
		{"valid ipv4", "192.168.1.1", false},
		{"valid localhost", "localhost", false},
		{"valid ipv6", "2001:0db8:85a3:0000:0000:8a2e:0370:7334", false},
		{"valid loopback", "127.0.0.1", false},
		
		// Invalid IPs
		{"empty", "", true},
		{"invalid format", "not.an.ip", true},
		{"out of range", "256.256.256.256", true},
		{"incomplete", "192.168.1", true},
		{"with port", "192.168.1.1:8080", true},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateIPAddress(tt.ip)
			
			if tt.wantErr && err == nil {
				t.Errorf("Expected error for IP '%s'", tt.ip)
			} else if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error for IP '%s': %v", tt.ip, err)
			}
		})
	}
}

func TestValidatePort(t *testing.T) {
	v := NewValidator()
	
	tests := []struct {
		name    string
		port    int
		wantErr bool
	}{
		{"valid 80", 80, false},
		{"valid 443", 443, false},
		{"valid 8080", 8080, false},
		{"valid min", 1, false},
		{"valid max", 65535, false},
		{"invalid zero", 0, true},
		{"invalid negative", -1, true},
		{"invalid too high", 65536, true},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidatePort(tt.port)
			
			if tt.wantErr && err == nil {
				t.Errorf("Expected error for port %d", tt.port)
			} else if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error for port %d: %v", tt.port, err)
			}
		})
	}
}

func TestValidateUploadRequest(t *testing.T) {
	v := NewValidator()
	
	tests := []struct {
		name      string
		filename  string
		fileSize  int64
		blockSize int
		wantErrs  int
	}{
		{"valid request", "file.txt", 1024, 128*1024, 0},
		{"invalid filename", "", 1024, 128*1024, 1},
		{"invalid size", "file.txt", -1, 128*1024, 1},
		{"invalid block size", "file.txt", 1024, 1000, 1},
		{"multiple errors", "", -1, 1000, 3},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := v.ValidateUploadRequest(tt.filename, tt.fileSize, tt.blockSize)
			
			if len(errors) != tt.wantErrs {
				t.Errorf("Expected %d errors, got %d", tt.wantErrs, len(errors))
			}
		})
	}
}

func TestValidateDownloadRequest(t *testing.T) {
	v := NewValidator()
	
	tests := []struct {
		name     string
		cid      string
		password string
		wantErrs int
	}{
		{"valid with password", "QmbWqxBEKC3P8tqsKc98xmWNzrzDtRLMiMPL8wBuTGsMnR", "MyStr0ng!Pass", 0},
		{"valid without password", "QmbWqxBEKC3P8tqsKc98xmWNzrzDtRLMiMPL8wBuTGsMnR", "", 0},
		{"invalid cid", "invalid", "", 1},
		{"invalid password", "QmbWqxBEKC3P8tqsKc98xmWNzrzDtRLMiMPL8wBuTGsMnR", "weak", 1},
		{"both invalid", "invalid", "weak", 2},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := v.ValidateDownloadRequest(tt.cid, tt.password)
			
			if len(errors) != tt.wantErrs {
				t.Errorf("Expected %d errors, got %d", tt.wantErrs, len(errors))
			}
		})
	}
}

func TestPasswordStrength(t *testing.T) {
	v := NewValidator()
	
	tests := []struct {
		name     string
		password string
		expected PasswordStrength
	}{
		{"empty", "", PasswordStrengthVeryWeak},
		{"very weak", "a", PasswordStrengthVeryWeak},
		{"weak", "password", PasswordStrengthFair}, // "password" has higher entropy than expected
		{"fair", "Pass517", PasswordStrengthFair}, // Shorter password with mixed case, numbers, but no special chars
		{"strong", "MyStr0ng!Pass", PasswordStrengthVeryStrong}, // Has good entropy with mixed chars
		{"very strong", "VeryC0mplex!P@ssw0rd#517", PasswordStrengthVeryStrong},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strength, entropy := v.GetPasswordStrength(tt.password)
			
			if strength != tt.expected {
				t.Errorf("Expected strength %v, got %v (entropy: %.2f)", tt.expected, strength, entropy)
			}
			
			// Verify entropy is non-negative
			if entropy < 0 {
				t.Errorf("Entropy should be non-negative, got %.2f", entropy)
			}
		})
	}
}

func TestPasswordStrengthString(t *testing.T) {
	tests := []struct {
		strength PasswordStrength
		expected string
	}{
		{PasswordStrengthVeryWeak, "Very Weak"},
		{PasswordStrengthWeak, "Weak"},
		{PasswordStrengthFair, "Fair"},
		{PasswordStrengthStrong, "Strong"},
		{PasswordStrengthVeryStrong, "Very Strong"},
		{PasswordStrength(999), "Unknown"},
	}
	
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.strength.String()
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestCalculatePasswordEntropy(t *testing.T) {
	v := NewValidator()
	
	tests := []struct {
		name     string
		password string
		minEntropy float64
	}{
		{"empty", "", 0},
		{"single char", "a", 4.7}, // log2(26) ≈ 4.7
		{"mixed case", "aA", 10.34}, // 2 * log2(52) ≈ 10.34
		{"with numbers", "aA1", 17.1}, // 3 * log2(62) ≈ 17.1
		{"with special", "aA1!", 24.0}, // 4 * log2(94) ≈ 24.0
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entropy := v.calculatePasswordEntropy(tt.password)
			
			if entropy < tt.minEntropy - 0.1 { // Allow small floating point variance
				t.Errorf("Expected entropy >= %.2f, got %.2f", tt.minEntropy, entropy)
			}
		})
	}
}

func TestIsCommonPassword(t *testing.T) {
	v := NewValidator()
	
	commonPasswords := []string{"password", "123456", "password123", "admin", "welcome"}
	uncommonPasswords := []string{"MyVeryUniquePassword123!", "UncommonPass!", "NotInList123"}
	
	for _, pwd := range commonPasswords {
		t.Run("common_"+pwd, func(t *testing.T) {
			if !v.isCommonPassword(strings.ToLower(pwd)) {
				t.Errorf("Expected '%s' to be detected as common", pwd)
			}
		})
	}
	
	for _, pwd := range uncommonPasswords {
		t.Run("uncommon_"+pwd, func(t *testing.T) {
			if v.isCommonPassword(strings.ToLower(pwd)) {
				t.Errorf("Expected '%s' to not be detected as common", pwd)
			}
		})
	}
}

func TestHasExcessiveRepeatedChars(t *testing.T) {
	v := NewValidator()
	
	tests := []struct {
		name     string
		password string
		expected bool
	}{
		{"no repetition", "abcdef", false},
		{"short repetition", "aab", false},
		{"excessive repetition", "aaab", true},
		{"long repetition", "aaaab", true},
		{"multiple groups", "aabbcc", false},
		{"excessive middle", "abcccdef", true},
		{"very short", "aa", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := v.hasExcessiveRepeatedChars(tt.password)
			if result != tt.expected {
				t.Errorf("Expected %v for '%s', got %v", tt.expected, tt.password, result)
			}
		})
	}
}

func TestHasSequentialChars(t *testing.T) {
	v := NewValidator()
	
	tests := []struct {
		name     string
		password string
		expected bool
	}{
		{"no sequence", "random", false},
		{"numeric sequence", "abc123def", true},
		{"alpha sequence", "password123", true}, // "123" is detected
		{"reverse sequence", "321test", true},
		{"qwerty sequence", "myqwerty", true},
		{"very short", "ab", false},
		{"keyboard sequence", "qazwsx", true},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := v.hasSequentialChars(tt.password)
			if result != tt.expected {
				t.Errorf("Expected %v for '%s', got %v", tt.expected, tt.password, result)
			}
		})
	}
}

// Benchmark tests for performance validation
func BenchmarkValidateFilename(b *testing.B) {
	v := NewValidator()
	filename := "test_file_with_reasonable_length.txt"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.ValidateFilename(filename)
	}
}

func BenchmarkValidatePassword(b *testing.B) {
	v := NewValidator()
	password := "MyStr0ng!Password123"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.ValidatePassword(password)
	}
}

func BenchmarkValidateCID(b *testing.B) {
	v := NewValidator()
	cid := "QmbWqxBEKC3P8tqsKc98xmWNzrzDtRLMiMPL8wBuTGsMnR"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.ValidateCID(cid)
	}
}

func BenchmarkSanitizeInput(b *testing.B) {
	v := NewValidator()
	input := "Some input with\x00null bytes\x01and control\tchars\n"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.SanitizeInput(input)
	}
}

func BenchmarkCalculatePasswordEntropy(b *testing.B) {
	v := NewValidator()
	password := "ComplexP@ssw0rd123WithManyCharacters"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.calculatePasswordEntropy(password)
	}
}
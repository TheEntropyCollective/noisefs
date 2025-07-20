package validation

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// Test helper functions and data

func createTestValidator() *InputValidator {
	return NewInputValidator()
}

func TestInputValidator_ValidateEmail(t *testing.T) {
	validator := createTestValidator()
	
	testCases := []struct {
		name        string
		email       string
		expectError bool
		errorType   ValidationErrorType
	}{
		// Valid emails
		{"Valid simple email", "user@example.com", false, ""},
		{"Valid with subdomain", "user@mail.example.com", false, ""},
		{"Valid with plus", "user+tag@example.com", false, ""},
		{"Valid with dots", "first.last@example.com", false, ""},
		{"Valid with numbers", "user123@example123.com", false, ""},
		{"Valid international domain", "user@example.co.uk", false, ""},
		
		// Invalid emails
		{"Missing @", "userexample.com", true, ErrInvalidEmail},
		{"Multiple @", "user@@example.com", true, ErrInvalidEmail},
		{"Missing domain", "user@", true, ErrInvalidEmail},
		{"Missing user", "@example.com", true, ErrInvalidEmail},
		{"Spaces", "user @example.com", true, ErrInvalidEmail},
		{"Empty string", "", true, ErrRequiredField},
		{"Too long local part", strings.Repeat("a", 65) + "@example.com", true, ErrInvalidEmail},
		{"Too long domain", "user@" + strings.Repeat("a", 255) + ".com", true, ErrInvalidEmail},
		
		// Security attacks via email
		{"XSS in email", "user+<script>alert('xss')</script>@evil.com", true, ErrXSSDetected},
		{"SQL injection in email", "user'; DROP TABLE users; --@evil.com", true, ErrSQLInjection},
		{"Null byte injection", "user\x00@example.com", true, ErrInvalidCharacter},
		{"Control character", "user\x07@example.com", true, ErrInvalidCharacter},
		{"Header injection", "user\r\nBcc: attacker@evil.com@example.com", true, ErrInvalidEmail},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.ValidateEmail(tc.email)
			
			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error for email %s", tc.email)
				} else {
					valErr, ok := err.(ValidationError)
					if !ok {
						t.Errorf("Expected ValidationError, got %T", err)
					} else if tc.errorType != "" && valErr.Type != tc.errorType {
						t.Errorf("Expected error type %s, got %s", tc.errorType, valErr.Type)
					}
				}
			} else {
				if err != nil {
					t.Errorf("Expected valid email %s to pass validation, got error: %v", tc.email, err)
				}
			}
		})
	}
}

func TestInputValidator_ValidateCID(t *testing.T) {
	validator := createTestValidator()
	
	testCases := []struct {
		name        string
		cid         string
		expectError bool
		errorType   ValidationErrorType
	}{
		// Valid CIDs
		{"Valid CIDv0", "QmT78zSuBmuS4z925WZfrqQ1qHaJ56DQaTfyMUF7F8ff5o", false, ""},
		{"Valid CIDv1 bafy", "bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi", false, ""},
		{"Valid CIDv1 bafk", "bafkreiem4twkqzsq2aj4shbycd4yvoj2cx72vezicletlhi7dijjciqpui", false, ""},
		
		// Invalid CIDs
		{"Empty CID", "", true, ErrRequiredField},
		{"Too short", "Qm", true, ErrInvalidCID},
		{"Too long", strings.Repeat("Q", 200), true, ErrInvalidCID},
		{"Invalid characters", "QmT78zSuBmuS4z925WZfrqQ1qHaJ56DQaTfyMUF7F8ff5o!", true, ErrInvalidCID},
		{"Wrong prefix", "Xm78zSuBmuS4z925WZfrqQ1qHaJ56DQaTfyMUF7F8ff5o", true, ErrInvalidCID},
		{"Contains spaces", "QmT78z SuBmuS4z925WZfrqQ1qHaJ56DQaTfyMUF7F8ff5o", true, ErrInvalidCID},
		
		// Security attacks via CID
		{"XSS in CID", "QmTest<script>alert('xss')</script>", true, ErrXSSDetected},
		{"SQL injection in CID", "QmTest'; DROP TABLE blocks; --", true, ErrSQLInjection},
		{"Path traversal in CID", "QmTest/../../../etc/passwd", true, ErrPathTraversal},
		{"Null byte injection", "QmTest\x00malicious", true, ErrInvalidCharacter},
		{"Control character", "QmTest\x07malicious", true, ErrInvalidCharacter},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.ValidateCID(tc.cid)
			
			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error for CID %s", tc.cid)
				} else {
					valErr, ok := err.(ValidationError)
					if !ok {
						t.Errorf("Expected ValidationError, got %T", err)
					} else if tc.errorType != "" && valErr.Type != tc.errorType {
						t.Errorf("Expected error type %s, got %s for CID %s", tc.errorType, valErr.Type, tc.cid)
					}
				}
			} else {
				if err != nil {
					t.Errorf("Expected valid CID %s to pass validation, got error: %v", tc.cid, err)
				}
			}
		})
	}
}

func TestInputValidator_ValidatePhoneNumber(t *testing.T) {
	validator := createTestValidator()
	
	testCases := []struct {
		name        string
		phone       string
		expectError bool
		errorType   ValidationErrorType
	}{
		// Valid phone numbers
		{"Empty phone (optional)", "", false, ""},
		{"US format with dashes", "555-123-4567", false, ""},
		{"International format", "+1-555-123-4567", false, ""},
		{"Numeric only", "5551234567", false, ""},
		{"International numeric", "+15551234567", false, ""},
		
		// Invalid phone numbers
		{"Invalid characters", "555-123-abcd", true, ErrInvalidPhoneNumber},
		{"Too short", "123", true, ErrInvalidPhoneNumber},
		{"Too long", "12345678901234567890", true, ErrInvalidPhoneNumber},
		
		// Security attacks
		{"XSS in phone", "555<script>alert('xss')</script>", true, ErrXSSDetected},
		{"SQL injection in phone", "555'; DROP TABLE users; --", true, ErrSQLInjection},
		{"Control character", "555\x00123", true, ErrInvalidCharacter},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.ValidatePhoneNumber(tc.phone)
			
			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error for phone %s", tc.phone)
				} else {
					valErr, ok := err.(ValidationError)
					if !ok {
						t.Errorf("Expected ValidationError, got %T", err)
					} else if tc.errorType != "" && valErr.Type != tc.errorType {
						t.Errorf("Expected error type %s, got %s", tc.errorType, valErr.Type)
					}
				}
			} else {
				if err != nil {
					t.Errorf("Expected valid phone %s to pass validation, got error: %v", tc.phone, err)
				}
			}
		})
	}
}

func TestInputValidator_ValidateSecurityInput(t *testing.T) {
	validator := createTestValidator()
	
	testCases := []struct {
		name        string
		input       string
		fieldName   string
		context     ValidationContext
		expectError bool
		errorType   ValidationErrorType
	}{
		// Valid inputs
		{"Valid text", "Hello World", "message", ValidationContext{RequiredField: true, MaxLength: 100}, false, ""},
		{"Empty optional field", "", "description", ValidationContext{RequiredField: false}, false, ""},
		{"Valid UTF-8", "José García 😀", "name", ValidationContext{RequiredField: true, MaxLength: 100}, false, ""},
		
		// Required field validation
		{"Missing required field", "", "name", ValidationContext{RequiredField: true}, true, ErrRequiredField},
		
		// Length validation
		{"Exceeds max length", strings.Repeat("A", 101), "description", ValidationContext{MaxLength: 100}, true, ErrExcessiveLength},
		
		// Security validation
		{"XSS content", "Hello <script>alert('xss')</script>", "message", ValidationContext{}, true, ErrXSSDetected},
		{"SQL injection", "'; DROP TABLE users; --", "query", ValidationContext{}, true, ErrSQLInjection},
		{"Path traversal", "../../../etc/passwd", "filename", ValidationContext{}, true, ErrPathTraversal},
		{"Invalid UTF-8", "Hello\xFF\xFE", "text", ValidationContext{}, true, ErrInvalidCharacter},
		{"Control characters", "Hello\x00World", "text", ValidationContext{}, true, ErrInvalidCharacter},
		{"Bidirectional override", "file\u202E.txt.exe", "filename", ValidationContext{}, true, ErrInvalidCharacter},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.ValidateSecurityInput(tc.input, tc.fieldName, tc.context)
			
			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error for input %s", tc.input)
				} else {
					valErr, ok := err.(ValidationError)
					if !ok {
						t.Errorf("Expected ValidationError, got %T", err)
					} else if tc.errorType != "" && valErr.Type != tc.errorType {
						t.Errorf("Expected error type %s, got %s", tc.errorType, valErr.Type)
					}
				}
			} else {
				if err != nil {
					t.Errorf("Expected valid input %s to pass validation, got error: %v", tc.input, err)
				}
			}
		})
	}
}

func TestInputValidator_ContainsXSS(t *testing.T) {
	validator := createTestValidator()
	
	xssPayloads := []string{
		"<script>alert('xss')</script>",
		"<img src=x onerror=alert('xss')>",
		"javascript:alert('xss')",
		"<svg onload=alert('xss')>",
		"<iframe src=javascript:alert('xss')>",
		"<body onload=alert('xss')>",
		"<div onclick=alert('xss')>",
		"<a href=\"javascript:alert('xss')\">link</a>",
		"<ScRiPt>alert('xss')</ScRiPt>", // Case insensitive
		"<SCRIPT SRC=http://evil.com/xss.js></SCRIPT>",
		"<IMG \"\"\"><SCRIPT>alert(\"XSS\")</SCRIPT>\">",
		"<TABLE BACKGROUND=\"javascript:alert('XSS')\">",
		"<DIV STYLE=\"background-image: url(javascript:alert('XSS'))\">",
		"vbscript:alert('xss')",
		"expression(alert('xss'))",
		"<svg><script>alert('xss')</script></svg>",
	}
	
	safeInputs := []string{
		"Hello World",
		"This is safe text",
		"email@example.com",
		"Some <safe> brackets",
		"Normal content with numbers 123",
	}
	
	// Test XSS detection
	for i, payload := range xssPayloads {
		t.Run(t.Name()+"_XSS_Payload_"+string(rune(i+1)), func(t *testing.T) {
			if !validator.ContainsXSS(payload) {
				t.Errorf("Expected XSS detection for payload: %s", payload)
			}
		})
	}
	
	// Test safe inputs
	for i, input := range safeInputs {
		t.Run(t.Name()+"_Safe_Input_"+string(rune(i+1)), func(t *testing.T) {
			if validator.ContainsXSS(input) {
				t.Errorf("False positive XSS detection for safe input: %s", input)
			}
		})
	}
}

func TestInputValidator_ContainsSQLInjection(t *testing.T) {
	validator := createTestValidator()
	
	sqlPayloads := []string{
		"'; DROP TABLE users; --",
		"' OR '1'='1",
		"' UNION SELECT * FROM users --",
		"'; INSERT INTO admin VALUES ('hacker', 'password'); --",
		"' OR 1=1 #",
		"'; EXEC xp_cmdshell('format c:'); --",
		"' AND (SELECT COUNT(*) FROM users) > 0 --",
		"'; WAITFOR DELAY '00:00:05'; --",
		"' OR (SELECT SUBSTRING(@@version,1,1))='M' --",
		"'; UPDATE users SET password='hacked' WHERE 1=1; --",
		"1'; SELECT * FROM information_schema.tables; --",
		"' OR SLEEP(5) --",
		"'; LOAD_FILE('/etc/passwd'); --",
		"' OR EXISTS(SELECT * FROM users WHERE username='admin') --",
		"' HAVING COUNT(*) > 1 --",
		"benchmark(10000,MD5(1))",
	}
	
	safeInputs := []string{
		"Hello World",
		"John O'Connor", // Apostrophe in name is safe
		"It's a nice day",
		"email@example.com",
		"Some text with -- dashes",
		"Normal content /* with comment style */",
	}
	
	// Test SQL injection detection
	for i, payload := range sqlPayloads {
		t.Run(t.Name()+"_SQL_Payload_"+string(rune(i+1)), func(t *testing.T) {
			if !validator.ContainsSQLInjection(payload) {
				t.Errorf("Expected SQL injection detection for payload: %s", payload)
			}
		})
	}
	
	// Test safe inputs
	for i, input := range safeInputs {
		t.Run(t.Name()+"_Safe_Input_"+string(rune(i+1)), func(t *testing.T) {
			if validator.ContainsSQLInjection(input) {
				t.Errorf("False positive SQL injection detection for safe input: %s", input)
			}
		})
	}
}

func TestInputValidator_ContainsPathTraversal(t *testing.T) {
	validator := createTestValidator()
	
	pathTraversalPayloads := []string{
		"../../../etc/passwd",
		"..\\..\\..\\windows\\system32\\config\\sam",
		"....//....//....//etc/passwd",
		"..%2F..%2F..%2Fetc%2Fpasswd",
		"..%5C..%5C..%5Cwindows%5Csystem32%5Cconfig%5Csam",
		"/%2e%2e/%2e%2e/%2e%2e/etc/passwd",
		"\\..\\..\\..\\etc\\passwd",
		"file:///etc/passwd",
		"file://c:/windows/system32/config/sam",
		"./../../../../root/.ssh/id_rsa",
		"~/../../../etc/shadow",
		"/proc/self/environ",
		"/sys/class/net",
		"/dev/sda",
		"\\windows\\system32\\config\\sam",
	}
	
	safeInputs := []string{
		"normal-filename.txt",
		"my_document.pdf",
		"folder/subfolder/file.txt",
		"Hello World",
		"email@example.com",
	}
	
	// Test path traversal detection
	for i, payload := range pathTraversalPayloads {
		t.Run(t.Name()+"_Path_Payload_"+string(rune(i+1)), func(t *testing.T) {
			if !validator.ContainsPathTraversal(payload) {
				t.Errorf("Expected path traversal detection for payload: %s", payload)
			}
		})
	}
	
	// Test safe inputs
	for i, input := range safeInputs {
		t.Run(t.Name()+"_Safe_Input_"+string(rune(i+1)), func(t *testing.T) {
			if validator.ContainsPathTraversal(input) {
				t.Errorf("False positive path traversal detection for safe input: %s", input)
			}
		})
	}
}

func TestInputValidator_IsValidUTF8(t *testing.T) {
	validator := createTestValidator()
	
	validUTF8 := []string{
		"Hello World",
		"José García",
		"李小明",
		"Александр",
		"Hello 😀 World",
		"",
		"Simple ASCII text",
	}
	
	invalidUTF8 := []string{
		"Hello\xFF\xFEWorld",
		"Test\xC0\x80",
		"Invalid\xED\xA0\x80",
		"\xF5\x80\x80\x80",
	}
	
	// Test valid UTF-8
	for _, input := range validUTF8 {
		t.Run("Valid_UTF8_"+input, func(t *testing.T) {
			if !validator.IsValidUTF8(input) {
				t.Errorf("Expected valid UTF-8 for input: %s", input)
			}
		})
	}
	
	// Test invalid UTF-8
	for i, input := range invalidUTF8 {
		t.Run(t.Name()+"_Invalid_UTF8_"+string(rune(i+1)), func(t *testing.T) {
			if validator.IsValidUTF8(input) {
				t.Errorf("Expected invalid UTF-8 detection for input: %x", input)
			}
		})
	}
}

func TestInputValidator_ContainsControlCharacters(t *testing.T) {
	validator := createTestValidator()
	
	// Valid inputs (allowed control characters)
	validInputs := []string{
		"Hello World",
		"Line 1\nLine 2",         // Newline allowed
		"Column 1\tColumn 2",     // Tab allowed  
		"Line 1\rLine 2",         // Carriage return allowed
		"José García",
		"Hello 😀 World",
	}
	
	// Invalid inputs (dangerous control characters)
	invalidInputs := []string{
		"Hello\x00World",         // Null byte
		"Hello\x07World",         // Bell
		"Hello\x08World",         // Backspace
		"Hello\x0BWorld",         // Vertical tab
		"Hello\x0CWorld",         // Form feed
		"Hello\x1BWorld",         // Escape
		"Hello\x7FWorld",         // Delete
		"file\u202E.txt.exe",     // Right-to-left override
		"file\u202D.txt.exe",     // Left-to-right override
		"file\u202C.txt.exe",     // Pop directional formatting
	}
	
	// Test valid inputs
	for _, input := range validInputs {
		t.Run("Valid_"+strings.ReplaceAll(input, "\n", "\\n"), func(t *testing.T) {
			if validator.ContainsControlCharacters(input) {
				t.Errorf("False positive control character detection for valid input: %q", input)
			}
		})
	}
	
	// Test invalid inputs
	for i, input := range invalidInputs {
		t.Run(t.Name()+"_Invalid_"+string(rune(i+1)), func(t *testing.T) {
			if !validator.ContainsControlCharacters(input) {
				t.Errorf("Expected control character detection for input: %q", input)
			}
		})
	}
}

// Performance benchmarks

func BenchmarkValidateEmail(b *testing.B) {
	validator := createTestValidator()
	email := "user@example.com"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.ValidateEmail(email)
	}
}

func BenchmarkValidateCID(b *testing.B) {
	validator := createTestValidator()
	cid := "QmT78zSuBmuS4z925WZfrqQ1qHaJ56DQaTfyMUF7F8ff5o"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.ValidateCID(cid)
	}
}

func BenchmarkSecurityValidation(b *testing.B) {
	validator := createTestValidator()
	input := "This is a normal text input with no security issues"
	context := ValidationContext{RequiredField: true, MaxLength: 1000}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.ValidateSecurityInput(input, "test_field", context)
	}
}

func BenchmarkXSSDetection(b *testing.B) {
	validator := createTestValidator()
	input := "This is a normal text input with no XSS content"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.ContainsXSS(input)
	}
}

func BenchmarkSQLInjectionDetection(b *testing.B) {
	validator := createTestValidator()
	input := "This is a normal text input with no SQL injection"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.ContainsSQLInjection(input)
	}
}

func BenchmarkPathTraversalDetection(b *testing.B) {
	validator := createTestValidator()
	input := "normal-filename.txt"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.ContainsPathTraversal(input)
	}
}

// Integration tests

func TestValidationEngine_Interface(t *testing.T) {
	// Test that InputValidator implements ValidationEngine interface
	var _ ValidationEngine = (*InputValidator)(nil)
	
	validator := NewInputValidator()
	if validator == nil {
		t.Fatal("NewInputValidator returned nil")
	}
}

func TestValidationError_Error(t *testing.T) {
	err := ValidationError{
		Field:   "test_field",
		Type:    ErrXSSDetected,
		Message: "Test error message",
		Value:   "test_value",
	}
	
	expected := "validation error in field 'test_field': Test error message (type: xss_detected)"
	if err.Error() != expected {
		t.Errorf("Expected error string %s, got %s", expected, err.Error())
	}
}

func TestValidationContext_FieldTypeAware(t *testing.T) {
	validator := createTestValidator()
	
	// Test different contexts for same input
	input := "test@example.com"
	
	// As email field - should pass
	emailContext := ValidationContext{
		RequiredField: true,
		MaxLength:     100,
		FieldType:     "email",
	}
	
	err := validator.ValidateSecurityInput(input, "email", emailContext)
	if err != nil {
		t.Errorf("Expected email input to pass security validation, got: %v", err)
	}
	
	// As general text field - should also pass
	textContext := ValidationContext{
		RequiredField: true,
		MaxLength:     100,
		FieldType:     "text",
	}
	
	err = validator.ValidateSecurityInput(input, "description", textContext)
	if err != nil {
		t.Errorf("Expected text input to pass security validation, got: %v", err)
	}
}

// Test concurrent validation
func TestValidationEngine_Concurrent(t *testing.T) {
	validator := createTestValidator()
	
	// Run multiple validations concurrently
	done := make(chan bool, 10)
	
	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() { done <- true }()
			
			// Test different validation methods concurrently
			email := fmt.Sprintf("user%d@example.com", id)
			err := validator.ValidateEmail(email)
			if err != nil {
				t.Errorf("Concurrent email validation failed: %v", err)
			}
			
			cid := "QmT78zSuBmuS4z925WZfrqQ1qHaJ56DQaTfyMUF7F8ff5o"
			err = validator.ValidateCID(cid)
			if err != nil {
				t.Errorf("Concurrent CID validation failed: %v", err)
			}
			
			input := fmt.Sprintf("Test input %d", id)
			context := ValidationContext{RequiredField: true, MaxLength: 100}
			err = validator.ValidateSecurityInput(input, "test", context)
			if err != nil {
				t.Errorf("Concurrent security validation failed: %v", err)
			}
		}(i)
	}
	
	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		select {
		case <-done:
			// Success
		case <-time.After(5 * time.Second):
			t.Fatal("Concurrent validation test timed out")
		}
	}
}
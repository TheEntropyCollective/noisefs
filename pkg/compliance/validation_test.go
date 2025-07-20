package compliance

import (
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// ValidationErrorType represents types of validation errors
type ValidationErrorType string

const (
	ErrXSSDetected       ValidationErrorType = "xss_detected"
	ErrSQLInjection      ValidationErrorType = "sql_injection"
	ErrPathTraversal     ValidationErrorType = "path_traversal"
	ErrInvalidEmail      ValidationErrorType = "invalid_email"
	ErrExcessiveLength   ValidationErrorType = "excessive_length"
	ErrInvalidCharacter  ValidationErrorType = "invalid_character"
	ErrRequiredField     ValidationErrorType = "required_field"
	ErrInvalidFormat     ValidationErrorType = "invalid_format"
	ErrInvalidCID        ValidationErrorType = "invalid_cid"
	ErrInvalidURL        ValidationErrorType = "invalid_url"
	ErrInvalidPhoneNumber ValidationErrorType = "invalid_phone"
)

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Type    ValidationErrorType
	Message string
	Value   string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("validation error in field '%s': %s (type: %s, value: '%s')", 
		e.Field, e.Message, e.Type, e.Value)
}

// InputValidator handles comprehensive input validation
type InputValidator struct {
	maxFieldLength    int
	allowedHTMLTags   []string
	sanitizeHTML      bool
	strictCIDValidation bool
}

// NewInputValidator creates a new input validator with default settings
func NewInputValidator() *InputValidator {
	return &InputValidator{
		maxFieldLength:      10000,  // 10KB max field length
		allowedHTMLTags:     []string{}, // No HTML allowed by default
		sanitizeHTML:        true,
		strictCIDValidation: true,
	}
}

// TestDMCANoticeValidation_XSSPrevention tests XSS prevention in DMCA notice fields
func TestDMCANoticeValidation_XSSPrevention(t *testing.T) {
	validator := NewInputValidator()
	
	xssPayloads := []string{
		"<script>alert('xss')</script>",
		"<img src=x onerror=alert('xss')>",
		"javascript:alert('xss')",
		"<svg onload=alert('xss')>",
		"<iframe src=javascript:alert('xss')>",
		"<body onload=alert('xss')>",
		"<div onclick=alert('xss')>",
		"<a href=\"javascript:alert('xss')\">link</a>",
		"&#60;script&#62;alert('xss')&#60;/script&#62;",
		"%3Cscript%3Ealert('xss')%3C/script%3E",
		"<ScRiPt>alert('xss')</ScRiPt>",
		"<SCRIPT SRC=http://evil.com/xss.js></SCRIPT>",
		"<IMG \"\"\"><SCRIPT>alert(\"XSS\")</SCRIPT>\">",
		"<TABLE BACKGROUND=\"javascript:alert('XSS')\">",
		"<DIV STYLE=\"background-image: url(javascript:alert('XSS'))\">",
	}
	
	fields := []string{
		"RequestorName",
		"RequestorEmail", 
		"RequestorAddress",
		"CopyrightWork",
		"CopyrightOwner",
		"Description",
		"SwornStatement",
		"GoodFaithBelief",
		"AccuracyStatement",
		"OriginalNotice",
		"ProcessingNotes",
	}
	
	for _, field := range fields {
		for i, payload := range xssPayloads {
			testName := fmt.Sprintf("%s_XSS_Payload_%d", field, i+1)
			t.Run(testName, func(t *testing.T) {
				// Create test notice with XSS payload
				notice := createTestDMCANotice()
				setNoticeField(notice, field, payload)
				
				// TDD: This will fail until implementation exists
				err := validator.ValidateDMCANotice(notice)
				
				if err == nil {
					t.Errorf("Expected XSS validation error for field %s with payload: %s", field, payload)
				} else {
					valErr, ok := err.(*ValidationError)
					if !ok {
						t.Errorf("Expected ValidationError, got %T", err)
					} else if valErr.Type != ErrXSSDetected {
						t.Errorf("Expected ErrXSSDetected, got %s", valErr.Type)
					}
				}
			})
		}
	}
}

// TestDMCANoticeValidation_SQLInjectionPrevention tests SQL injection prevention
func TestDMCANoticeValidation_SQLInjectionPrevention(t *testing.T) {
	validator := NewInputValidator()
	
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
		"'; CALL system('rm -rf /'); --",
	}
	
	testCases := []struct {
		field    string
		payloads []string
	}{
		{"RequestorName", sqlPayloads},
		{"RequestorEmail", sqlPayloads},
		{"CopyrightWork", sqlPayloads},
		{"Description", sqlPayloads},
		{"DescriptorCIDs", []string{"QmTest'; DROP TABLE blocks; --", "bafybeig'; UNION SELECT * FROM secrets; --"}},
	}
	
	for _, tc := range testCases {
		for i, payload := range tc.payloads {
			testName := fmt.Sprintf("%s_SQL_Injection_%d", tc.field, i+1)
			t.Run(testName, func(t *testing.T) {
				notice := createTestDMCANotice()
				setNoticeField(notice, tc.field, payload)
				
				// TDD: This will fail until implementation exists
				err := validator.ValidateDMCANotice(notice)
				
				if err == nil {
					t.Errorf("Expected SQL injection validation error for field %s with payload: %s", tc.field, payload)
				} else {
					valErr, ok := err.(*ValidationError)
					if !ok {
						t.Errorf("Expected ValidationError, got %T", err)
					} else if valErr.Type != ErrSQLInjection {
						t.Errorf("Expected ErrSQLInjection, got %s", valErr.Type)
					}
				}
			})
		}
	}
}

// TestDMCANoticeValidation_PathTraversalPrevention tests path traversal prevention in file identifiers
func TestDMCANoticeValidation_PathTraversalPrevention(t *testing.T) {
	validator := NewInputValidator()
	
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
		"QmTest/../../../sensitive.txt",
		"bafybei/../config/secrets.json",
		"descriptor://../../admin/private",
		"./../../../../root/.ssh/id_rsa",
		"~/../../../etc/shadow",
		"descriptor:///proc/self/environ",
	}
	
	for i, payload := range pathTraversalPayloads {
		testName := fmt.Sprintf("PathTraversal_InfringingURLs_%d", i+1)
		t.Run(testName, func(t *testing.T) {
			notice := createTestDMCANotice()
			notice.InfringingURLs = []string{payload}
			
			// TDD: This will fail until implementation exists
			err := validator.ValidateDMCANotice(notice)
			
			if err == nil {
				t.Errorf("Expected path traversal validation error for URL: %s", payload)
			} else {
				valErr, ok := err.(*ValidationError)
				if !ok {
					t.Errorf("Expected ValidationError, got %T", err)
				} else if valErr.Type != ErrPathTraversal {
					t.Errorf("Expected ErrPathTraversal, got %s", valErr.Type)
				}
			}
		})
	}
}

// TestEmailValidation tests comprehensive email validation and sanitization
func TestEmailValidation(t *testing.T) {
	validator := NewInputValidator()
	
	testCases := []struct {
		name    string
		email   string
		valid   bool
		errType ValidationErrorType
	}{
		// Valid emails
		{"Valid simple email", "user@example.com", true, ""},
		{"Valid with subdomain", "user@mail.example.com", true, ""},
		{"Valid with plus", "user+tag@example.com", true, ""},
		{"Valid with dots", "first.last@example.com", true, ""},
		{"Valid with numbers", "user123@example123.com", true, ""},
		{"Valid international domain", "user@example.co.uk", true, ""},
		
		// Invalid emails
		{"Missing @", "userexample.com", false, ErrInvalidEmail},
		{"Multiple @", "user@@example.com", false, ErrInvalidEmail},
		{"Missing domain", "user@", false, ErrInvalidEmail},
		{"Missing user", "@example.com", false, ErrInvalidEmail},
		{"Invalid characters", "user<script>@example.com", false, ErrInvalidEmail},
		{"Spaces", "user @example.com", false, ErrInvalidEmail},
		{"Empty string", "", false, ErrRequiredField},
		{"Too long local part", strings.Repeat("a", 65) + "@example.com", false, ErrExcessiveLength},
		{"Too long domain", "user@" + strings.Repeat("a", 255) + ".com", false, ErrExcessiveLength},
		
		// Security attacks via email
		{"XSS in email", "user+<script>alert('xss')</script>@evil.com", false, ErrXSSDetected},
		{"SQL injection in email", "user'; DROP TABLE users; --@evil.com", false, ErrSQLInjection},
		{"Command injection", "user`rm -rf /`@evil.com", false, ErrInvalidCharacter},
		{"Null byte injection", "user\x00@example.com", false, ErrInvalidCharacter},
		{"LDAP injection", "user*)(uid=*))(|(uid=*@evil.com", false, ErrInvalidCharacter},
		{"Header injection", "user\r\nBcc: attacker@evil.com@example.com", false, ErrInvalidCharacter},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test email validation in different contexts
			contexts := []struct {
				field  string
				setter func(*DMCANotice, string)
			}{
				{"RequestorEmail", func(n *DMCANotice, email string) { n.RequestorEmail = email }},
			}
			
			for _, ctx := range contexts {
				notice := createTestDMCANotice()
				ctx.setter(notice, tc.email)
				
				// TDD: This will fail until implementation exists
				err := validator.ValidateDMCANotice(notice)
				
				if tc.valid {
					if err != nil {
						t.Errorf("Expected valid email %s to pass validation, got error: %v", tc.email, err)
					}
				} else {
					if err == nil {
						t.Errorf("Expected invalid email %s to fail validation", tc.email)
					} else {
						valErr, ok := err.(*ValidationError)
						if !ok {
							t.Errorf("Expected ValidationError, got %T", err)
						} else if tc.errType != "" && valErr.Type != tc.errType {
							t.Errorf("Expected error type %s, got %s", tc.errType, valErr.Type)
						}
					}
				}
			}
		})
	}
}

// TestCIDValidation tests Content Identifier (CID) validation
func TestCIDValidation(t *testing.T) {
	validator := NewInputValidator()
	
	testCases := []struct {
		name    string
		cid     string
		valid   bool
		errType ValidationErrorType
	}{
		// Valid CIDs (simplified examples)
		{"Valid CIDv0", "QmT78zSuBmuS4z925WZfrqQ1qHaJ56DQaTfyMUF7F8ff5o", true, ""},
		{"Valid CIDv1", "bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi", true, ""},
		{"Valid CIDv1 base32", "bafkreiem4twkqzsq2aj4shbycd4yvoj2cx72vezicletlhi7dijjciqpui", true, ""},
		
		// Invalid CIDs
		{"Empty CID", "", false, ErrRequiredField},
		{"Too short", "Qm", false, ErrInvalidCID},
		{"Too long", strings.Repeat("Q", 200), false, ErrInvalidCID},
		{"Invalid characters", "QmT78zSuBmuS4z925WZfrqQ1qHaJ56DQaTfyMUF7F8ff5o!", false, ErrInvalidCID},
		{"Wrong prefix", "Xm78zSuBmuS4z925WZfrqQ1qHaJ56DQaTfyMUF7F8ff5o", false, ErrInvalidCID},
		{"Contains spaces", "QmT78z SuBmuS4z925WZfrqQ1qHaJ56DQaTfyMUF7F8ff5o", false, ErrInvalidCID},
		
		// Security attacks via CID
		{"XSS in CID", "QmTest<script>alert('xss')</script>", false, ErrXSSDetected},
		{"SQL injection in CID", "QmTest'; DROP TABLE blocks; --", false, ErrSQLInjection},
		{"Path traversal in CID", "QmTest/../../../etc/passwd", false, ErrPathTraversal},
		{"Null byte injection", "QmTest\x00malicious", false, ErrInvalidCharacter},
		{"Command injection", "QmTest`rm -rf /`", false, ErrInvalidCharacter},
		{"Binary data", "QmTest\xFF\xFE\xFD", false, ErrInvalidCharacter},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			notice := createTestDMCANotice()
			notice.DescriptorCIDs = []string{tc.cid}
			
			// TDD: This will fail until implementation exists
			err := validator.ValidateDMCANotice(notice)
			
			if tc.valid {
				if err != nil {
					t.Errorf("Expected valid CID %s to pass validation, got error: %v", tc.cid, err)
				}
			} else {
				if err == nil {
					t.Errorf("Expected invalid CID %s to fail validation", tc.cid)
				} else {
					valErr, ok := err.(*ValidationError)
					if !ok {
						t.Errorf("Expected ValidationError, got %T", err)
					} else if tc.errType != "" && valErr.Type != tc.errType {
						t.Errorf("Expected error type %s, got %s for CID %s", tc.errType, valErr.Type, tc.cid)
					}
				}
			}
		})
	}
}

// TestLengthLimitsAndSizeConstraints tests field length validation
func TestLengthLimitsAndSizeConstraints(t *testing.T) {
	validator := NewInputValidator()
	
	testCases := []struct {
		name      string
		field     string
		maxLength int
		testValue func(int) string
	}{
		{"RequestorName length", "RequestorName", 255, func(n int) string { return strings.Repeat("A", n) }},
		{"RequestorEmail length", "RequestorEmail", 320, func(n int) string { return strings.Repeat("a", n-11) + "@example.com" }},
		{"RequestorAddress length", "RequestorAddress", 1000, func(n int) string { return strings.Repeat("123 Main St ", n/10) }},
		{"CopyrightWork length", "CopyrightWork", 2000, func(n int) string { return strings.Repeat("A", n) }},
		{"Description length", "Description", 5000, func(n int) string { return strings.Repeat("A", n) }},
		{"SwornStatement length", "SwornStatement", 10000, func(n int) string { return strings.Repeat("A", n) }},
		{"OriginalNotice length", "OriginalNotice", 50000, func(n int) string { return strings.Repeat("A", n) }},
	}
	
	for _, tc := range testCases {
		// Test within limits
		t.Run(tc.name+"_WithinLimit", func(t *testing.T) {
			notice := createTestDMCANotice()
			value := tc.testValue(tc.maxLength)
			setNoticeField(notice, tc.field, value)
			
			// TDD: This will fail until implementation exists
			err := validator.ValidateDMCANotice(notice)
			
			if err != nil {
				valErr, ok := err.(*ValidationError)
				if ok && valErr.Type == ErrExcessiveLength {
					t.Errorf("Expected field %s with length %d to be within limits, got excessive length error", tc.field, tc.maxLength)
				}
			}
		})
		
		// Test exceeding limits
		t.Run(tc.name+"_ExceedsLimit", func(t *testing.T) {
			notice := createTestDMCANotice()
			value := tc.testValue(tc.maxLength + 1000) // Exceed by 1000 chars
			setNoticeField(notice, tc.field, value)
			
			// TDD: This will fail until implementation exists
			err := validator.ValidateDMCANotice(notice)
			
			if err == nil {
				t.Errorf("Expected field %s with excessive length to fail validation", tc.field)
			} else {
				valErr, ok := err.(*ValidationError)
				if !ok {
					t.Errorf("Expected ValidationError, got %T", err)
				} else if valErr.Type != ErrExcessiveLength {
					t.Errorf("Expected ErrExcessiveLength, got %s", valErr.Type)
				}
			}
		})
	}
}

// TestCharacterEncodingValidation tests UTF-8 and control character validation
func TestCharacterEncodingValidation(t *testing.T) {
	validator := NewInputValidator()
	
	testCases := []struct {
		name        string
		field       string
		value       string
		expectError bool
		errorType   ValidationErrorType
	}{
		// Valid UTF-8
		{"Valid ASCII", "RequestorName", "John Doe", false, ""},
		{"Valid UTF-8 accents", "RequestorName", "José García", false, ""},
		{"Valid UTF-8 emoji", "RequestorName", "John 😀 Doe", false, ""},
		{"Valid UTF-8 Chinese", "RequestorName", "李小明", false, ""},
		{"Valid UTF-8 Cyrillic", "RequestorName", "Александр", false, ""},
		
		// Invalid characters
		{"Null byte", "RequestorName", "John\x00Doe", true, ErrInvalidCharacter},
		{"Bell character", "RequestorName", "John\x07Doe", true, ErrInvalidCharacter},
		{"Backspace", "RequestorName", "John\x08Doe", true, ErrInvalidCharacter},
		{"Vertical tab", "RequestorName", "John\x0BDoe", true, ErrInvalidCharacter},
		{"Form feed", "RequestorName", "John\x0CDoe", true, ErrInvalidCharacter},
		{"Escape character", "RequestorName", "John\x1BDoe", true, ErrInvalidCharacter},
		{"Delete character", "RequestorName", "John\x7FDoe", true, ErrInvalidCharacter},
		
		// Invalid UTF-8 sequences
		{"Invalid UTF-8 byte sequence", "RequestorName", "John\xFF\xFEDoe", true, ErrInvalidCharacter},
		{"Incomplete UTF-8 sequence", "RequestorName", "John\xC0Doe", true, ErrInvalidCharacter},
		{"Overlong UTF-8 encoding", "RequestorName", "John\xC0\x80Doe", true, ErrInvalidCharacter},
		
		// Valid control characters (allowed ones)
		{"Newline", "Description", "Line 1\nLine 2", false, ""},
		{"Carriage return", "Description", "Line 1\rLine 2", false, ""},
		{"Tab", "Description", "Column 1\tColumn 2", false, ""},
		
		// Bidirectional text attacks
		{"RLO attack", "RequestorName", "John\u202E.txt.exe", true, ErrInvalidCharacter},
		{"LRO attack", "RequestorName", "John\u202D.txt.exe", true, ErrInvalidCharacter},
		{"PDF attack", "RequestorName", "John\u202C.txt.exe", true, ErrInvalidCharacter},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			notice := createTestDMCANotice()
			setNoticeField(notice, tc.field, tc.value)
			
			// TDD: This will fail until implementation exists
			err := validator.ValidateDMCANotice(notice)
			
			if tc.expectError {
				if err == nil {
					t.Errorf("Expected validation error for %s with value containing invalid characters", tc.field)
				} else {
					valErr, ok := err.(*ValidationError)
					if !ok {
						t.Errorf("Expected ValidationError, got %T", err)
					} else if valErr.Type != tc.errorType {
						t.Errorf("Expected error type %s, got %s", tc.errorType, valErr.Type)
					}
				}
			} else {
				if err != nil {
					valErr, ok := err.(*ValidationError)
					if ok && valErr.Type == ErrInvalidCharacter {
						t.Errorf("Expected valid UTF-8 text to pass validation, got invalid character error: %v", err)
					}
				}
			}
		})
	}
}

// TestBusinessLogicValidation tests required fields and format constraints
func TestBusinessLogicValidation(t *testing.T) {
	validator := NewInputValidator()
	
	testCases := []struct {
		name        string
		setupNotice func() *DMCANotice
		expectError bool
		errorType   ValidationErrorType
		errorField  string
	}{
		{
			name: "Complete valid notice",
			setupNotice: func() *DMCANotice {
				return createValidDMCANotice()
			},
			expectError: false,
		},
		{
			name: "Missing requestor name",
			setupNotice: func() *DMCANotice {
				notice := createValidDMCANotice()
				notice.RequestorName = ""
				return notice
			},
			expectError: true,
			errorType:   ErrRequiredField,
			errorField:  "RequestorName",
		},
		{
			name: "Missing requestor email",
			setupNotice: func() *DMCANotice {
				notice := createValidDMCANotice()
				notice.RequestorEmail = ""
				return notice
			},
			expectError: true,
			errorType:   ErrRequiredField,
			errorField:  "RequestorEmail",
		},
		{
			name: "Missing copyright work",
			setupNotice: func() *DMCANotice {
				notice := createValidDMCANotice()
				notice.CopyrightWork = ""
				return notice
			},
			expectError: true,
			errorType:   ErrRequiredField,
			errorField:  "CopyrightWork",
		},
		{
			name: "Missing descriptor CIDs and URLs",
			setupNotice: func() *DMCANotice {
				notice := createValidDMCANotice()
				notice.DescriptorCIDs = []string{}
				notice.InfringingURLs = []string{}
				return notice
			},
			expectError: true,
			errorType:   ErrRequiredField,
			errorField:  "DescriptorCIDs",
		},
		{
			name: "Missing sworn statement",
			setupNotice: func() *DMCANotice {
				notice := createValidDMCANotice()
				notice.SwornStatement = ""
				return notice
			},
			expectError: true,
			errorType:   ErrRequiredField,
			errorField:  "SwornStatement",
		},
		{
			name: "Missing signature",
			setupNotice: func() *DMCANotice {
				notice := createValidDMCANotice()
				notice.Signature = ""
				return notice
			},
			expectError: true,
			errorType:   ErrRequiredField,
			errorField:  "Signature",
		},
		{
			name: "Invalid phone number format",
			setupNotice: func() *DMCANotice {
				notice := createValidDMCANotice()
				notice.RequestorPhone = "invalid-phone-123-abc"
				return notice
			},
			expectError: true,
			errorType:   ErrInvalidPhoneNumber,
			errorField:  "RequestorPhone",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			notice := tc.setupNotice()
			
			// TDD: This will fail until implementation exists
			err := validator.ValidateDMCANotice(notice)
			
			if tc.expectError {
				if err == nil {
					t.Errorf("Expected validation error for %s", tc.name)
				} else {
					valErr, ok := err.(*ValidationError)
					if !ok {
						t.Errorf("Expected ValidationError, got %T", err)
					} else {
						if valErr.Type != tc.errorType {
							t.Errorf("Expected error type %s, got %s", tc.errorType, valErr.Type)
						}
						if tc.errorField != "" && valErr.Field != tc.errorField {
							t.Errorf("Expected error in field %s, got %s", tc.errorField, valErr.Field)
						}
					}
				}
			} else {
				if err != nil {
					t.Errorf("Expected valid notice to pass validation, got error: %v", err)
				}
			}
		})
	}
}

// TestCounterNoticeValidation tests validation of counter-notice specific fields
func TestCounterNoticeValidation(t *testing.T) {
	validator := NewInputValidator()
	
	testCases := []struct {
		name           string
		setupNotice    func() *CounterNotice
		expectError    bool
		errorType      ValidationErrorType
	}{
		{
			name: "Valid counter notice",
			setupNotice: func() *CounterNotice {
				return createValidCounterNotice()
			},
			expectError: false,
		},
		{
			name: "XSS in good faith belief",
			setupNotice: func() *CounterNotice {
				notice := createValidCounterNotice()
				notice.GoodFaithBelief = "I disagree <script>alert('xss')</script> with this claim"
				return notice
			},
			expectError: true,
			errorType:   ErrXSSDetected,
		},
		{
			name: "SQL injection in sworn statement",
			setupNotice: func() *CounterNotice {
				notice := createValidCounterNotice()
				notice.SwornStatement = "I swear'; DROP TABLE users; -- this is legitimate"
				return notice
			},
			expectError: true,
			errorType:   ErrSQLInjection,
		},
		{
			name: "Invalid user ID format",
			setupNotice: func() *CounterNotice {
				notice := createValidCounterNotice()
				notice.UserID = "../../admin/secrets"
				return notice
			},
			expectError: true,
			errorType:   ErrPathTraversal,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			notice := tc.setupNotice()
			
			// TDD: This will fail until implementation exists
			err := validator.ValidateCounterNotice(notice)
			
			if tc.expectError {
				if err == nil {
					t.Errorf("Expected validation error for %s", tc.name)
				} else {
					valErr, ok := err.(*ValidationError)
					if !ok {
						t.Errorf("Expected ValidationError, got %T", err)
					} else if valErr.Type != tc.errorType {
						t.Errorf("Expected error type %s, got %s", tc.errorType, valErr.Type)
					}
				}
			} else {
				if err != nil {
					t.Errorf("Expected valid counter notice to pass validation, got error: %v", err)
				}
			}
		})
	}
}

// TestNotificationValidation tests user notification content validation
func TestNotificationValidation(t *testing.T) {
	validator := NewInputValidator()
	
	testCases := []struct {
		name           string
		setupNotification func() *UserNotification
		expectError    bool
		errorType      ValidationErrorType
	}{
		{
			name: "Valid notification",
			setupNotification: func() *UserNotification {
				return createValidUserNotification()
			},
			expectError: false,
		},
		{
			name: "XSS in subject",
			setupNotification: func() *UserNotification {
				notif := createValidUserNotification()
				notif.Subject = "Alert <script>alert('xss')</script>"
				return notif
			},
			expectError: true,
			errorType:   ErrXSSDetected,
		},
		{
			name: "Malicious metadata",
			setupNotification: func() *UserNotification {
				notif := createValidUserNotification()
				notif.Metadata = map[string]interface{}{
					"eval": "javascript:alert('xss')",
					"script": "<script>malicious()</script>",
				}
				return notif
			},
			expectError: true,
			errorType:   ErrXSSDetected,
		},
		{
			name: "User response with SQL injection",
			setupNotification: func() *UserNotification {
				notif := createValidUserNotification()
				notif.UserResponse = "Yes'; DELETE FROM notifications; --"
				return notif
			},
			expectError: true,
			errorType:   ErrSQLInjection,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			notification := tc.setupNotification()
			
			// TDD: This will fail until implementation exists
			err := validator.ValidateUserNotification(notification)
			
			if tc.expectError {
				if err == nil {
					t.Errorf("Expected validation error for %s", tc.name)
				} else {
					valErr, ok := err.(*ValidationError)
					if !ok {
						t.Errorf("Expected ValidationError, got %T", err)
					} else if valErr.Type != tc.errorType {
						t.Errorf("Expected error type %s, got %s", tc.errorType, valErr.Type)
					}
				}
			} else {
				if err != nil {
					t.Errorf("Expected valid notification to pass validation, got error: %v", err)
				}
			}
		})
	}
}

// TestAuditLogValidation tests audit log entry validation
func TestAuditLogValidation(t *testing.T) {
	validator := NewInputValidator()
	
	testCases := []struct {
		name        string
		setupEntry  func() *AuditLogEntry
		expectError bool
		errorType   ValidationErrorType
	}{
		{
			name: "Valid audit entry",
			setupEntry: func() *AuditLogEntry {
				return createValidAuditLogEntry()
			},
			expectError: false,
		},
		{
			name: "XSS in user agent",
			setupEntry: func() *AuditLogEntry {
				entry := createValidAuditLogEntry()
				entry.UserAgent = "Mozilla/5.0 <script>alert('xss')</script>"
				return entry
			},
			expectError: true,
			errorType:   ErrXSSDetected,
		},
		{
			name: "Invalid IP address",
			setupEntry: func() *AuditLogEntry {
				entry := createValidAuditLogEntry()
				entry.IPAddress = "999.999.999.999"
				return entry
			},
			expectError: true,
			errorType:   ErrInvalidFormat,
		},
		{
			name: "Malicious details map",
			setupEntry: func() *AuditLogEntry {
				entry := createValidAuditLogEntry()
				entry.Details = map[string]interface{}{
					"command": "rm -rf /",
					"script":  "<script>alert('xss')</script>",
				}
				return entry
			},
			expectError: true,
			errorType:   ErrXSSDetected,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			entry := tc.setupEntry()
			
			// TDD: This will fail until implementation exists
			err := validator.ValidateAuditLogEntry(entry)
			
			if tc.expectError {
				if err == nil {
					t.Errorf("Expected validation error for %s", tc.name)
				} else {
					valErr, ok := err.(*ValidationError)
					if !ok {
						t.Errorf("Expected ValidationError, got %T", err)
					} else if valErr.Type != tc.errorType {
						t.Errorf("Expected error type %s, got %s", tc.errorType, valErr.Type)
					}
				}
			} else {
				if err != nil {
					t.Errorf("Expected valid audit entry to pass validation, got error: %v", err)
				}
			}
		})
	}
}

// TestPerformanceAndStressValidation tests validation performance under load
func TestPerformanceAndStressValidation(t *testing.T) {
	validator := NewInputValidator()
	
	// Test with realistic load
	t.Run("Performance_100_Notices", func(t *testing.T) {
		start := time.Now()
		
		for i := 0; i < 100; i++ {
			notice := createValidDMCANotice()
			notice.RequestorName = fmt.Sprintf("Test User %d", i)
			
			// TDD: This will fail until implementation exists
			err := validator.ValidateDMCANotice(notice)
			
			if err != nil {
				t.Errorf("Validation failed for notice %d: %v", i, err)
			}
		}
		
		elapsed := time.Since(start)
		t.Logf("Validated 100 notices in %v (%.2f notices/sec)", elapsed, 100.0/elapsed.Seconds())
		
		// Performance requirement: should handle at least 10 notices per second
		if elapsed > 10*time.Second {
			t.Errorf("Validation too slow: took %v for 100 notices (requirement: <10s)", elapsed)
		}
	})
	
	// Test with very large input
	t.Run("Large_Input_Handling", func(t *testing.T) {
		notice := createTestDMCANotice()
		// Create very large but valid description
		notice.Description = strings.Repeat("This is a very long description of the copyrighted work. ", 100) // ~5KB
		
		// TDD: This will fail until implementation exists
		err := validator.ValidateDMCANotice(notice)
		
		// Should handle large input gracefully (either accept if within limits or reject with proper error)
		if err != nil {
			valErr, ok := err.(*ValidationError)
			if !ok {
				t.Errorf("Expected ValidationError for large input, got %T", err)
			} else if valErr.Type != ErrExcessiveLength {
				t.Errorf("Expected ErrExcessiveLength for large input, got %s", valErr.Type)
			}
		}
	})
}

// Test helper functions

func createTestDMCANotice() *DMCANotice {
	return &DMCANotice{
		NoticeID:       "TEST-001",
		ReceivedDate:   time.Now(),
		RequestorName:  "Test Requestor",
		RequestorEmail: "test@example.com",
	}
}

func createValidDMCANotice() *DMCANotice {
	return &DMCANotice{
		NoticeID:             "VALID-001",
		ReceivedDate:         time.Now(),
		RequestorName:        "John Doe",
		RequestorEmail:       "john.doe@example.com",
		RequestorAddress:     "123 Main St, Anytown, USA",
		RequestorPhone:       "+1-555-123-4567",
		CopyrightWork:        "Test Copyrighted Work",
		CopyrightOwner:       "John Doe",
		RegistrationNumber:   "REG123456",
		InfringingURLs:       []string{"https://example.com/infringing"},
		DescriptorCIDs:       []string{"QmT78zSuBmuS4z925WZfrqQ1qHaJ56DQaTfyMUF7F8ff5o"},
		Description:          "Description of the infringement",
		SwornStatement:       "I swear under penalty of perjury that the information is accurate",
		GoodFaithBelief:      "I have a good faith belief that the use is not authorized",
		AccuracyStatement:    "The information in this notice is accurate",
		Signature:            "John Doe",
		OriginalNotice:       "Original DMCA notice text",
		ProcessingNotes:      "Internal processing notes",
	}
}

func createValidCounterNotice() *CounterNotice {
	return &CounterNotice{
		CounterNoticeID:        "CN-001",
		UserID:                 "user-123",
		UserName:               "Jane Smith",
		UserEmail:              "jane.smith@example.com",
		UserAddress:            "456 Oak Ave, Somewhere, USA",
		SwornStatement:         "I swear under penalty of perjury that I have a good faith belief",
		GoodFaithBelief:        "I believe this takedown was made in error",
		ConsentToJurisdiction:  true,
		Signature:              "Jane Smith",
		SubmissionDate:         time.Now(),
		Status:                 "pending",
		ProcessingNotes:        "",
	}
}

func createValidUserNotification() *UserNotification {
	return &UserNotification{
		NotificationID:   "NOTIF-001",
		UserID:           "user-123",
		NotificationType: "dmca_takedown",
		Priority:         "high",
		Subject:          "DMCA Takedown Notice",
		Content:          "Your content has been taken down due to a DMCA notice",
		CreatedAt:        time.Now(),
		Language:         "en-US",
		Metadata:         map[string]interface{}{"source": "automated"},
	}
}

func createValidAuditLogEntry() *AuditLogEntry {
	return &AuditLogEntry{
		EntryID:   "AUDIT-001",
		Timestamp: time.Now(),
		EventType: "dmca_notice_received",
		UserID:    "user-123",
		TargetID:  "QmT78zSuBmuS4z925WZfrqQ1qHaJ56DQaTfyMUF7F8ff5o",
		Action:    "notice_processed",
		Details:   map[string]interface{}{"requestor": "John Doe"},
		Result:    "success",
		IPAddress: "192.168.1.1",
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
	}
}

func setNoticeField(notice *DMCANotice, field, value string) {
	switch field {
	case "RequestorName":
		notice.RequestorName = value
	case "RequestorEmail":
		notice.RequestorEmail = value
	case "RequestorAddress":
		notice.RequestorAddress = value
	case "CopyrightWork":
		notice.CopyrightWork = value
	case "CopyrightOwner":
		notice.CopyrightOwner = value
	case "Description":
		notice.Description = value
	case "SwornStatement":
		notice.SwornStatement = value
	case "GoodFaithBelief":
		notice.GoodFaithBelief = value
	case "AccuracyStatement":
		notice.AccuracyStatement = value
	case "OriginalNotice":
		notice.OriginalNotice = value
	case "ProcessingNotes":
		notice.ProcessingNotes = value
	case "DescriptorCIDs":
		notice.DescriptorCIDs = []string{value}
	}
}

// Stub implementations to satisfy compilation - will fail all tests initially (TDD)

func (v *InputValidator) ValidateDMCANotice(notice *DMCANotice) error {
	return fmt.Errorf("DMCA notice validation not implemented")
}

func (v *InputValidator) ValidateCounterNotice(notice *CounterNotice) error {
	return fmt.Errorf("counter notice validation not implemented")
}

func (v *InputValidator) ValidateUserNotification(notification *UserNotification) error {
	return fmt.Errorf("user notification validation not implemented")
}

func (v *InputValidator) ValidateAuditLogEntry(entry *AuditLogEntry) error {
	return fmt.Errorf("audit log entry validation not implemented")
}

// Note: getScopesForRole function is defined in auth_test.go

// Additional validation helper functions that tests expect to exist

func isValidEmail(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}

func containsXSS(input string) bool {
	xssPatterns := []string{
		"<script",
		"javascript:",
		"<iframe",
		"<object",
		"<embed",
		"<form",
		"onerror=",
		"onload=",
		"onclick=",
	}
	
	lower := strings.ToLower(input)
	for _, pattern := range xssPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

func containsSQLInjection(input string) bool {
	sqlPatterns := []string{
		"' or ",
		"' union ",
		"'; drop ",
		"'; delete ",
		"'; insert ",
		"'; update ",
		"' and ",
		"-- ",
		"/*",
		"*/",
	}
	
	lower := strings.ToLower(input)
	for _, pattern := range sqlPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

func containsPathTraversal(input string) bool {
	pathPatterns := []string{
		"../",
		"..\\",
		"....//",
		"..%2F",
		"..%5C",
		"%2e%2e",
		"file://",
		"/proc/",
		"/etc/",
		"c:/windows",
	}
	
	lower := strings.ToLower(input)
	for _, pattern := range pathPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

func isValidUTF8(input string) bool {
	return utf8.ValidString(input)
}

func containsControlCharacters(input string) bool {
	for _, r := range input {
		// Allow tab, newline, carriage return
		if r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		// Check for other control characters
		if r < 32 || r == 127 {
			return true
		}
		// Check for dangerous Unicode characters
		if r == '\u202E' || r == '\u202D' || r == '\u202C' {
			return true
		}
	}
	return false
}

func isValidCID(cid string) bool {
	if len(cid) < 10 || len(cid) > 100 {
		return false
	}
	
	// Simple validation - real implementation would be more thorough
	matched, _ := regexp.MatchString(`^[A-Za-z0-9]+$`, cid)
	return matched && (strings.HasPrefix(cid, "Qm") || strings.HasPrefix(cid, "bafy") || strings.HasPrefix(cid, "bafk"))
}

func isValidPhoneNumber(phone string) bool {
	// Simple phone validation - real implementation would be more comprehensive
	matched, _ := regexp.MatchString(`^\+?[1-9]\d{1,14}$|^\+?[1-9]\d{0,3}[-.\s]?\d{3}[-.\s]?\d{3}[-.\s]?\d{4}$`, phone)
	return matched
}

func isValidIPAddress(ip string) bool {
	// Simple IP validation - real implementation would use net.ParseIP
	matched, _ := regexp.MatchString(`^(\d{1,3}\.){3}\d{1,3}$`, ip)
	if !matched {
		return false
	}
	
	parts := strings.Split(ip, ".")
	for _, part := range parts {
		if len(part) == 0 || len(part) > 3 {
			return false
		}
		// Simple range check
		if part > "255" || part < "0" {
			return false
		}
	}
	return true
}
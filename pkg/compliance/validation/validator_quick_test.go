package validation

import (
	"fmt"
	"testing"
)

// Quick test to verify enhanced security patterns work correctly
func TestEnhancedSecurityPatterns(t *testing.T) {
	validator := NewInputValidator()
	
	// Test enhanced XSS patterns
	xssTestCases := []string{
		"<script>alert('xss')</script>",
		"<img src=x onerror=alert('xss')>",
		"javascript:alert('xss')",
		"<svg onload=alert('xss')>",
		"&#60;script&#62;alert('xss')&#60;/script&#62;", // HTML entity encoding
		"%3cscript%3ealert('xss')%3c/script%3e",        // URL encoding
		"<ScRiPt>alert('xss')</ScRiPt>",               // Case variations
		"<script\x09>alert('xss')</script>",           // Tab evasion
	}
	
	for i, payload := range xssTestCases {
		t.Run(fmt.Sprintf("XSS_Test_%d", i+1), func(t *testing.T) {
			if !validator.ContainsXSS(payload) {
				t.Errorf("Failed to detect XSS in payload: %s", payload)
			}
		})
	}
	
	// Test enhanced SQL injection patterns
	sqlTestCases := []string{
		"' or 1=1--",
		"'; DROP TABLE users; --",
		"' UNION SELECT password FROM users--",
		"admin'/*",
		"1' and extractvalue(1,concat(char(126),version(),char(126)))--",
		"' AND (SELECT * FROM (SELECT COUNT(*),concat(version(),floor(rand(0)*2))x FROM information_schema.tables GROUP BY x)a)--",
		"' OR '1'='1",
		"') OR ('1'='1",
	}
	
	for i, payload := range sqlTestCases {
		t.Run(fmt.Sprintf("SQL_Test_%d", i+1), func(t *testing.T) {
			if !validator.ContainsSQLInjection(payload) {
				t.Errorf("Failed to detect SQL injection in payload: %s", payload)
			}
		})
	}
	
	// Test enhanced path traversal patterns
	pathTestCases := []string{
		"../../../etc/passwd",
		"..\\..\\..\\windows\\system32\\config\\sam",
		"..%2F..%2F..%2Fetc%2Fpasswd",
		"..%5c..%5c..%5cwindows%5csystem32%5cconfig%5csam",
		"%2e%2e%2f%2e%2e%2f%2e%2e%2fetc%2fpasswd",
		"file:///etc/passwd",
		"/proc/self/environ",
		"\\\\?\\c:\\windows\\system32\\config\\sam",
		"..%00/",
		"../\x00",
		"169.254.169.254/latest/meta-data/",
	}
	
	for i, payload := range pathTestCases {
		t.Run(fmt.Sprintf("Path_Test_%d", i+1), func(t *testing.T) {
			if !validator.ContainsPathTraversal(payload) {
				t.Errorf("Failed to detect path traversal in payload: %s", payload)
			}
		})
	}
	
	// Test document validation with security checks
	t.Run("DMCA_Notice_Security", func(t *testing.T) {
		notice := &DMCANotice{
			RequestorName:    "John Doe <script>alert('xss')</script>",
			RequestorEmail:   "john@example.com",
			RequestorAddress: "123 Main St",
			CopyrightWork:    "Test Work",
			CopyrightOwner:   "John Doe",
			Description:      "Test description",
			SwornStatement:   "I swear",
			GoodFaithBelief:  "Good faith",
			AccuracyStatement: "Accurate",
			Signature:        "John Doe",
			DescriptorCIDs:   []string{"QmTest123"},
		}
		
		err := validator.ValidateDMCANotice(notice)
		if err == nil {
			t.Error("Expected XSS detection in RequestorName field")
		}
		
		valErr, ok := err.(ValidationError)
		if !ok {
			t.Errorf("Expected ValidationError, got %T", err)
		} else if valErr.Type != ErrXSSDetected {
			t.Errorf("Expected ErrXSSDetected, got %s", valErr.Type)
		}
	})
	
	// Test valid input passes validation
	t.Run("Valid_Input", func(t *testing.T) {
		validInput := "This is a completely safe and valid input string"
		
		if validator.ContainsXSS(validInput) {
			t.Error("Valid input incorrectly flagged as XSS")
		}
		
		if validator.ContainsSQLInjection(validInput) {
			t.Error("Valid input incorrectly flagged as SQL injection")
		}
		
		if validator.ContainsPathTraversal(validInput) {
			t.Error("Valid input incorrectly flagged as path traversal")
		}
	})
}
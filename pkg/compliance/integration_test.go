package compliance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDMCAProcessorIntegration tests that the DMCA processor correctly integrates with the centralized validation engine
func TestDMCAProcessorIntegration(t *testing.T) {
	// Create processor with default config
	database := NewComplianceDatabase()
	processor := NewTakedownProcessor(database, nil)
	
	// Test XSS detection in DMCA notice
	maliciousNotice := &DMCANotice{
		RequestorName:    "Test User",
		RequestorEmail:   "test@example.com",
		CopyrightWork:    "Test Work",
		Description:      "<script>alert('xss')</script>Malicious description",
		DescriptorCIDs:   []string{"QmTestValidCID123456789"},
		SwornStatement:   "I swear this is true",
		GoodFaithBelief:  "I believe in good faith",
		AccuracyStatement: "This is accurate", 
		Signature:        "Test Signature",
	}
	
	result, err := processor.ProcessDMCANotice(maliciousNotice)
	require.NoError(t, err, "Processing should not fail")
	
	// Should be rejected due to XSS
	assert.Equal(t, "rejected", result.Status, "Notice with XSS should be rejected")
	assert.Contains(t, result.ValidationErrors[0], "XSS attack detected", "Should detect XSS attack")
	
	// Test valid notice
	validNotice := &DMCANotice{
		RequestorName:    "Test User",
		RequestorEmail:   "test@example.com", 
		CopyrightWork:    "Test Work",
		Description:      "Clean description of copyright work",
		DescriptorCIDs:   []string{"QmTestValidCID123456789"},
		SwornStatement:   "I swear this is true",
		GoodFaithBelief:  "I believe in good faith",
		AccuracyStatement: "This is accurate",
		Signature:        "Test Signature",
	}
	
	result, err = processor.ProcessDMCANotice(validNotice)
	require.NoError(t, err, "Processing should not fail")
	
	// Should be processed successfully
	assert.Equal(t, "processed", result.Status, "Valid notice should be processed")
	assert.Empty(t, result.ValidationErrors, "Should have no validation errors")
}

// TestSecurityValidationIntegration tests that all security validations are properly integrated
func TestSecurityValidationIntegration(t *testing.T) {
	database := NewComplianceDatabase()
	processor := NewTakedownProcessor(database, nil)
	validator := processor.GetValidator()
	
	testCases := []struct {
		name     string
		notice   *DMCANotice
		expectValid bool
		expectError string
	}{
		{
			name: "XSS in requestor name",
			notice: &DMCANotice{
				RequestorName:    "<script>alert('xss')</script>",
				RequestorEmail:   "test@example.com",
				CopyrightWork:    "Test Work",
				DescriptorCIDs:   []string{"QmTestValidCID123456789"},
				SwornStatement:   "I swear",
				GoodFaithBelief:  "I believe",
				Signature:        "Signature",
			},
			expectValid: false,
			expectError: "XSS attack detected in field 'RequestorName'",
		},
		{
			name: "SQL injection in email",
			notice: &DMCANotice{
				RequestorName:    "Test User",
				RequestorEmail:   "test@example.com'; DROP TABLE users; --",
				CopyrightWork:    "Test Work", 
				DescriptorCIDs:   []string{"QmTestValidCID123456789"},
				SwornStatement:   "I swear",
				GoodFaithBelief:  "I believe",
				Signature:        "Signature",
			},
			expectValid: false,
			expectError: "SQL injection attempt detected in field 'RequestorEmail'",
		},
		{
			name: "Path traversal in CID",
			notice: &DMCANotice{
				RequestorName:    "Test User",
				RequestorEmail:   "test@example.com",
				CopyrightWork:    "Test Work",
				DescriptorCIDs:   []string{"../../../etc/passwd"},
				SwornStatement:   "I swear",
				GoodFaithBelief:  "I believe", 
				Signature:        "Signature",
			},
			expectValid: false,
			expectError: "Path traversal attempt detected in DescriptorCID[0]",
		},
		{
			name: "Valid notice",
			notice: &DMCANotice{
				RequestorName:    "Test User",
				RequestorEmail:   "test@example.com",
				CopyrightWork:    "Test Work",
				Description:      "Clean description",
				DescriptorCIDs:   []string{"QmValidCID123"},
				SwornStatement:   "I swear this is true",
				GoodFaithBelief:  "I believe in good faith",
				AccuracyStatement: "This is accurate",
				Signature:        "Test Signature",
			},
			expectValid: true,
			expectError: "",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := validator.ValidateNotice(tc.notice)
			require.NoError(t, err, "Validation should not fail")
			
			assert.Equal(t, tc.expectValid, result.Valid, "Validation result should match expectation")
			
			if tc.expectError != "" {
				assert.Contains(t, result.Errors, tc.expectError, "Should contain expected error")
			}
		})
	}
}
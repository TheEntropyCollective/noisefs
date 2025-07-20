package compliance

import (
	"testing"
	"time"
)

func TestProcessorSecurityIntegration(t *testing.T) {
	// Create a simple test database
	database := &ComplianceDatabase{
		BlacklistedDescriptors: make(map[string]*TakedownRecord),
		CounterNotices:         make(map[string]*CounterNotice),
		ComplianceAuditSystem:  &ComplianceAuditSystem{},
		LegalFramework:         &LegalFramework{},
	}
	
	// Create processor with security validation
	processor := NewTakedownProcessor(database, nil)

	t.Run("SecurityValidationIntegration", func(t *testing.T) {
		// Test notice with XSS payload
		notice := &DMCANotice{
			RequestorName:      "John Doe <script>alert('xss')</script>",
			RequestorEmail:     "john@example.com",
			RequestorAddress:   "123 Main St",
			CopyrightWork:      "Test Work",
			CopyrightOwner:     "John Doe",
			Description:        "Test description",
			SwornStatement:     "I swear",
			GoodFaithBelief:    "Good faith",
			AccuracyStatement:  "Accurate",
			Signature:          "John Doe",
			DescriptorCIDs:     []string{"QmTest123"},
			ReceivedDate:       time.Now(),
		}

		result, err := processor.validator.ValidateNotice(notice)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Should fail validation due to XSS
		if result.Valid {
			t.Error("Expected validation to fail due to XSS attack")
		}

		// Should have security error
		hasSecurityError := false
		for _, errMsg := range result.Errors {
			if containsStr(errMsg, "XSS attack detected") {
				hasSecurityError = true
				break
			}
		}
		
		if !hasSecurityError {
			t.Errorf("Expected XSS security error, got errors: %v", result.Errors)
		}
		
		// Security issues should heavily impact score
		if result.Score > 0.5 {
			t.Errorf("Expected low score due to security issues, got: %f", result.Score)
		}
	})
	
	t.Run("ValidNoticePassesValidation", func(t *testing.T) {
		// Test completely valid notice
		notice := &DMCANotice{
			RequestorName:      "John Doe",
			RequestorEmail:     "john@example.com",
			RequestorAddress:   "123 Main Street",
			CopyrightWork:      "Original Novel",
			CopyrightOwner:     "John Doe",
			Description:        "Unauthorized reproduction",
			SwornStatement:     "I swear under penalty of perjury",
			GoodFaithBelief:    "I have a good faith belief",
			AccuracyStatement:  "I believe the information is accurate",
			Signature:          "John Doe",
			DescriptorCIDs:     []string{"QmValidCID123456789"},
			ReceivedDate:       time.Now(),
		}

		result, err := processor.validator.ValidateNotice(notice)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Should pass validation
		if !result.Valid {
			t.Errorf("Expected valid notice to pass, got errors: %v", result.Errors)
		}
		
		// Should have high score
		if result.Score < 0.8 {
			t.Errorf("Expected high score for valid notice, got: %f", result.Score)
		}
	})
}

// Simple helper function to avoid conflicts
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && findSubstr(s, substr)
}

func findSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
package compliance

import (
	"strings"
	"testing"
	"time"
)

func TestDMCANoticeSecurityValidation(t *testing.T) {
	// Create processor with default config
	database := NewComplianceDatabase()
	processor := NewTakedownProcessor(database, nil)

	t.Run("XSS_Attack_Detection", func(t *testing.T) {
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
		}

		result, err := processor.validator.ValidateNotice(notice)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if result.Valid {
			t.Error("Expected validation to fail due to XSS attack")
		}

		found := false
		for _, errMsg := range result.Errors {
			if strings.Contains(errMsg, "XSS attack detected in field 'RequestorName'") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected XSS detection error for RequestorName, got errors: %v", result.Errors)
		}
	})

	t.Run("SQL_Injection_Detection", func(t *testing.T) {
		notice := &DMCANotice{
			RequestorName:      "John Doe",
			RequestorEmail:     "john@example.com",
			RequestorAddress:   "123 Main St",
			CopyrightWork:      "'; DROP TABLE users; --",
			CopyrightOwner:     "John Doe",
			Description:        "Test description",
			SwornStatement:     "I swear",
			GoodFaithBelief:    "Good faith",
			AccuracyStatement:  "Accurate",
			Signature:          "John Doe",
			DescriptorCIDs:     []string{"QmTest123"},
		}

		result, err := processor.validator.ValidateNotice(notice)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if result.Valid {
			t.Error("Expected validation to fail due to SQL injection attack")
		}

		found := false
		for _, errMsg := range result.Errors {
			if strings.Contains(errMsg, "SQL injection attempt detected in field 'CopyrightWork'") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected SQL injection detection error for CopyrightWork, got errors: %v", result.Errors)
		}
	})

	t.Run("Path_Traversal_Detection", func(t *testing.T) {
		notice := &DMCANotice{
			RequestorName:      "John Doe",
			RequestorEmail:     "john@example.com",
			RequestorAddress:   "../../../etc/passwd",
			CopyrightWork:      "Test Work",
			CopyrightOwner:     "John Doe",
			Description:        "Test description",
			SwornStatement:     "I swear",
			GoodFaithBelief:    "Good faith",
			AccuracyStatement:  "Accurate",
			Signature:          "John Doe",
			DescriptorCIDs:     []string{"QmTest123"},
		}

		result, err := processor.validator.ValidateNotice(notice)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if result.Valid {
			t.Error("Expected validation to fail due to path traversal attack")
		}

		found := false
		for _, errMsg := range result.Errors {
			if strings.Contains(errMsg, "Path traversal attempt detected in field 'RequestorAddress'") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected path traversal detection error for RequestorAddress, got errors: %v", result.Errors)
		}
	})

	t.Run("InfringingURLs_Security_Check", func(t *testing.T) {
		notice := &DMCANotice{
			RequestorName:      "John Doe",
			RequestorEmail:     "john@example.com",
			RequestorAddress:   "123 Main St",
			CopyrightWork:      "Test Work",
			CopyrightOwner:     "John Doe",
			Description:        "Test description",
			SwornStatement:     "I swear",
			GoodFaithBelief:    "Good faith",
			AccuracyStatement:  "Accurate",
			Signature:          "John Doe",
			InfringingURLs:     []string{"http://example.com/<script>alert('xss')</script>"},
			DescriptorCIDs:     []string{"QmTest123"},
		}

		result, err := processor.validator.ValidateNotice(notice)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if result.Valid {
			t.Error("Expected validation to fail due to XSS in InfringingURLs")
		}

		found := false
		for _, errMsg := range result.Errors {
			if strings.Contains(errMsg, "XSS attack detected in InfringingURL[0]") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected XSS detection error for InfringingURL, got errors: %v", result.Errors)
		}
	})

	t.Run("DescriptorCID_Security_Check", func(t *testing.T) {
		notice := &DMCANotice{
			RequestorName:      "John Doe",
			RequestorEmail:     "john@example.com",
			RequestorAddress:   "123 Main St",
			CopyrightWork:      "Test Work",
			CopyrightOwner:     "John Doe",
			Description:        "Test description",
			SwornStatement:     "I swear",
			GoodFaithBelief:    "Good faith",
			AccuracyStatement:  "Accurate",
			Signature:          "John Doe",
			DescriptorCIDs:     []string{"../../../etc/passwd"},
		}

		result, err := processor.validator.ValidateNotice(notice)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if result.Valid {
			t.Error("Expected validation to fail due to path traversal in DescriptorCID")
		}

		found := false
		for _, errMsg := range result.Errors {
			if strings.Contains(errMsg, "Path traversal attempt detected in DescriptorCID[0]") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected path traversal detection error for DescriptorCID, got errors: %v", result.Errors)
		}
	})

	t.Run("Valid_Notice_Passes", func(t *testing.T) {
		notice := &DMCANotice{
			RequestorName:      "John Doe",
			RequestorEmail:     "john@example.com",
			RequestorAddress:   "123 Main Street, Anytown, ST 12345",
			RequestorPhone:     "+1-555-123-4567",
			CopyrightWork:      "Original Novel: The Adventure Chronicles",
			CopyrightOwner:     "John Doe",
			Description:        "Unauthorized reproduction of my copyrighted novel",
			SwornStatement:     "I swear under penalty of perjury that the information in this notice is accurate",
			GoodFaithBelief:    "I have a good faith belief that the use is not authorized",
			AccuracyStatement:  "I believe the information is accurate to the best of my knowledge",
			Signature:          "John Doe",
			DescriptorCIDs:     []string{"QmValidCID123456789"},
			InfringingURLs:     []string{"http://example.com/infringing-content"},
			ReceivedDate:       time.Now(),
		}

		result, err := processor.validator.ValidateNotice(notice)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if !result.Valid {
			t.Errorf("Expected valid notice to pass validation, got errors: %v", result.Errors)
		}

		if len(result.Errors) > 0 {
			t.Errorf("Expected no validation errors for valid notice, got: %v", result.Errors)
		}

		if result.Score < 0.8 {
			t.Errorf("Expected high validation score for valid notice, got: %f", result.Score)
		}
	})

	t.Run("Security_Score_Penalty", func(t *testing.T) {
		// Test that security violations get heavy score penalties
		notice := &DMCANotice{
			RequestorName:      "John Doe <script>alert('xss')</script>",
			RequestorEmail:     "john@example.com",
			RequestorAddress:   "123 Main St",
			CopyrightWork:      "'; DROP TABLE users; --",
			CopyrightOwner:     "John Doe",
			Description:        "../../../etc/passwd",
			SwornStatement:     "I swear",
			GoodFaithBelief:    "Good faith",
			AccuracyStatement:  "Accurate",
			Signature:          "John Doe",
			DescriptorCIDs:     []string{"QmTest123"},
		}

		result, err := processor.validator.ValidateNotice(notice)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Should have very low score due to multiple security violations
		if result.Score > 0.3 {
			t.Errorf("Expected very low score due to security violations, got: %f", result.Score)
		}

		// Should have multiple security errors
		if len(result.Errors) < 3 {
			t.Errorf("Expected multiple security errors, got: %v", result.Errors)
		}
	})
}

// Note: using strings.Contains from standard library
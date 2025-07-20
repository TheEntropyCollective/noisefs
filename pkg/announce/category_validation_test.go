package announce

import (
	"testing"
	"time"
)

func TestCategoryValidation_AllDefinedCategoriesValid(t *testing.T) {
	validator := NewValidator(nil)
	
	// Test all defined category constants
	categories := []struct {
		name     string
		constant string
	}{
		{"video", CategoryVideo},
		{"audio", CategoryAudio},
		{"document", CategoryDocument},
		{"data", CategoryData},
		{"software", CategorySoftware},
		{"image", CategoryImage},
		{"archive", CategoryArchive},
		{"other", CategoryOther},
	}
	
	for _, cat := range categories {
		t.Run(cat.name, func(t *testing.T) {
			// Create announcement with this category
			ann := &Announcement{
				Version:    "1.0",
				Descriptor: "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG",
				TopicHash:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
				Category:   cat.constant,
				SizeClass:  "medium",
				Timestamp:  time.Now().Unix(),
				TTL:        3600,
				Nonce:      "test-nonce-12345",
			}
			
			// Validate should pass for all defined categories
			err := validator.ValidateAnnouncement(ann)
			if err != nil {
				t.Errorf("Category '%s' (constant: %s) should be valid but got error: %v", cat.name, cat.constant, err)
			}
			
			// Also test the helper function
			if !isValidCategory(cat.constant) {
				t.Errorf("isValidCategory() should return true for '%s' but returned false", cat.constant)
			}
		})
	}
}

func TestCategoryValidation_InvalidCategories(t *testing.T) {
	validator := NewValidator(nil)
	
	invalidCategories := []string{
		"invalid",
		"text",
		"pdf",
		"music",
		"movie",
		"VIDEO", // case sensitive
	}
	
	for _, invalidCat := range invalidCategories {
		t.Run("invalid_"+invalidCat, func(t *testing.T) {
			ann := &Announcement{
				Version:    "1.0",
				Descriptor: "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG",
				TopicHash:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
				Category:   invalidCat,
				SizeClass:  "medium",
				Timestamp:  time.Now().Unix(),
				TTL:        3600,
				Nonce:      "test-nonce-12345",
			}
			
			// Validation should fail for invalid categories
			err := validator.ValidateAnnouncement(ann)
			if err == nil {
				t.Errorf("Category '%s' should be invalid but validation passed", invalidCat)
			}
			
			// Also test the helper function
			if isValidCategory(invalidCat) {
				t.Errorf("isValidCategory() should return false for '%s' but returned true", invalidCat)
			}
		})
	}
}

func TestCategoryValidation_EmptyCategory(t *testing.T) {
	validator := NewValidator(nil)
	
	// Empty category should be allowed (optional field)
	ann := &Announcement{
		Version:    "1.0",
		Descriptor: "QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG",
		TopicHash:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		Category:   "", // Empty category
		SizeClass:  "medium",
		Timestamp:  time.Now().Unix(),
		TTL:        3600,
		Nonce:      "test-nonce-12345",
	}
	
	err := validator.ValidateAnnouncement(ann)
	if err != nil {
		t.Errorf("Empty category should be valid (optional field) but got error: %v", err)
	}
}
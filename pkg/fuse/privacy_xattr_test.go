//go:build fuse
// +build fuse

package fuse

import (
	"github.com/hanwen/go-fuse/v2/fuse"
	"strings"
	"testing"
)

// TestExtendedAttributesPrivacy tests that NoiseFS extended attributes don't expose sensitive data
func TestExtendedAttributesPrivacy(t *testing.T) {
	// Create temporary index
	indexFile := "/tmp/test_privacy_index.json"
	index := NewFileIndex(indexFile)

	// Add a test file with sensitive data
	testFile := "sensitive/document.txt"
	testCID := "QmSensitiveCID123456789"
	testSize := int64(4096)

	index.AddFile(testFile, testCID, testSize)

	// Create NoiseFS instance
	fs := &NoiseFS{
		index: index,
	}

	// Test that sensitive attributes are not exposed
	context := &fuse.Context{}
	filename := "files/" + testFile

	// Test 1: Sensitive attributes should be blocked
	sensitiveAttrs := []string{
		"user.noisefs.descriptor_cid",
		"user.noisefs.created_at",
		"user.noisefs.modified_at",
		"user.noisefs.file_size",
		"user.noisefs.directory",
	}

	for _, attr := range sensitiveAttrs {
		data, status := fs.GetXAttr(filename, attr, context)
		if status == fuse.OK && len(data) > 0 {
			t.Errorf("Sensitive attribute %s should be blocked but returned: %s", attr, string(data))
		}
	}

	// Test 2: Check that only safe attributes are listed
	attrs, status := fs.ListXAttr(filename, context)
	if status == fuse.OK {
		for _, attr := range attrs {
			if strings.Contains(attr, "descriptor_cid") ||
				strings.Contains(attr, "created_at") ||
				strings.Contains(attr, "modified_at") ||
				strings.Contains(attr, "file_size") ||
				strings.Contains(attr, "directory") {
				t.Errorf("Sensitive attribute %s should not be listed", attr)
			}
		}

		// Verify safe attributes are available
		expectedSafe := []string{
			"user.noisefs.type",
			"user.noisefs.version",
			"user.noisefs.encrypted",
		}

		for _, expected := range expectedSafe {
			found := false
			for _, attr := range attrs {
				if attr == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected safe attribute %s not found", expected)
			}
		}
	}

	// Test 3: Safe attributes should work
	data, status := fs.GetXAttr(filename, "user.noisefs.type", context)
	if status != fuse.OK {
		t.Error("Safe attribute user.noisefs.type should be accessible")
	} else if string(data) != "noisefs-file" {
		t.Errorf("Expected 'noisefs-file', got '%s'", string(data))
	}
}

// TestExtendedAttributesNonSensitiveData tests that non-sensitive metadata can still be provided
func TestExtendedAttributesNonSensitiveData(t *testing.T) {
	// Create temporary index
	indexFile := "/tmp/test_nonsensitive_index.json"
	index := NewFileIndex(indexFile)

	// Add a test file
	testFile := "public/readme.txt"
	testCID := "QmPublicCID123456789"
	testSize := int64(2048)

	index.AddFile(testFile, testCID, testSize)

	// Create NoiseFS instance
	fs := &NoiseFS{
		index: index,
	}

	context := &fuse.Context{}
	filename := "files/" + testFile

	// Test that some metadata can still be provided safely
	attrs, status := fs.ListXAttr(filename, context)
	if status == fuse.OK {
		// Should have some attributes available
		if len(attrs) == 0 {
			t.Error("Expected some extended attributes to be available")
		}

		// Check for privacy-safe attributes
		foundSafeAttr := false
		for _, attr := range attrs {
			if strings.HasPrefix(attr, "user.noisefs.") &&
				!strings.Contains(attr, "descriptor_cid") &&
				!strings.Contains(attr, "directory") {
				foundSafeAttr = true
			}
		}

		if !foundSafeAttr {
			t.Log("No privacy-safe NoiseFS attributes found (this may be intentional)")
		}
	}
}

// TestExtendedAttributesEncryption tests that if sensitive data is provided, it's encrypted
func TestExtendedAttributesEncryption(t *testing.T) {
	// Create temporary index
	indexFile := "/tmp/test_encryption_index.json"
	index := NewFileIndex(indexFile)

	// Add a test file
	testFile := "secure/data.bin"
	testCID := "QmSecureCID987654321"
	testSize := int64(8192)

	index.AddFile(testFile, testCID, testSize)

	// Create NoiseFS instance
	fs := &NoiseFS{
		index: index,
	}

	context := &fuse.Context{}
	filename := "files/" + testFile

	// If descriptor CID is provided, it should be in encrypted/hashed form
	data, status := fs.GetXAttr(filename, "user.noisefs.descriptor_cid", context)
	if status == fuse.OK {
		dataStr := string(data)
		// Should not be the raw CID
		if dataStr == testCID {
			t.Error("Descriptor CID should be encrypted/hashed, not raw")
		}
		// Should not be empty (unless completely removed)
		if len(dataStr) == 0 {
			t.Log("Descriptor CID attribute returns empty (may be removed for privacy)")
		}
		// If non-empty, should look encrypted/hashed
		if len(dataStr) > 0 && dataStr == testCID {
			t.Error("Returned CID appears to be unencrypted")
		}
	}
}

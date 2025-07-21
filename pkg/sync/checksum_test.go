package sync

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCalculateFileChecksum(t *testing.T) {
	// Create temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	testContent := "Hello, World!"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Calculate checksum
	checksum, err := CalculateFileChecksum(testFile)
	if err != nil {
		t.Fatalf("Failed to calculate checksum: %v", err)
	}

	// Verify checksum is not empty
	if checksum == "" {
		t.Error("Checksum should not be empty")
	}

	// Verify checksum is consistent
	checksum2, err := CalculateFileChecksum(testFile)
	if err != nil {
		t.Fatalf("Failed to calculate checksum second time: %v", err)
	}

	if checksum != checksum2 {
		t.Errorf("Checksums should be identical: %s != %s", checksum, checksum2)
	}

	// Expected SHA-256 checksum for "Hello, World!"
	expectedChecksum := "dffd6021bb2bd5b0af676290809ec3a53191dd81c7f70a4b28688a362182986f"
	if checksum != expectedChecksum {
		t.Errorf("Unexpected checksum. Got %s, expected %s", checksum, expectedChecksum)
	}
}

func TestValidateChecksum(t *testing.T) {
	// Create temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	testContent := "Hello, World!"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Valid checksum
	expectedChecksum := "dffd6021bb2bd5b0af676290809ec3a53191dd81c7f70a4b28688a362182986f"
	valid, err := ValidateChecksum(testFile, expectedChecksum)
	if err != nil {
		t.Fatalf("Failed to validate checksum: %v", err)
	}
	if !valid {
		t.Error("Expected checksum to be valid")
	}

	// Invalid checksum
	invalidChecksum := "invalid_checksum"
	valid, err = ValidateChecksum(testFile, invalidChecksum)
	if err != nil {
		t.Fatalf("Failed to validate checksum: %v", err)
	}
	if valid {
		t.Error("Expected checksum to be invalid")
	}
}

func TestCalculateFileChecksumNonexistent(t *testing.T) {
	_, err := CalculateFileChecksum("/nonexistent/file")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestCalculateDirectoryChecksum(t *testing.T) {
	// Create temporary test directory with files
	tmpDir := t.TempDir()

	// Create some test files
	files := map[string]string{
		"file1.txt":        "Content 1",
		"file2.txt":        "Content 2",
		"subdir/file3.txt": "Content 3",
	}

	for relativePath, content := range files {
		fullPath := filepath.Join(tmpDir, relativePath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	// Calculate directory checksum
	checksum, err := CalculateDirectoryChecksum(tmpDir)
	if err != nil {
		t.Fatalf("Failed to calculate directory checksum: %v", err)
	}

	if checksum == "" {
		t.Error("Directory checksum should not be empty")
	}

	// Verify checksum is consistent
	checksum2, err := CalculateDirectoryChecksum(tmpDir)
	if err != nil {
		t.Fatalf("Failed to calculate directory checksum second time: %v", err)
	}

	if checksum != checksum2 {
		t.Errorf("Directory checksums should be identical: %s != %s", checksum, checksum2)
	}
}

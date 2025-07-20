package descriptors

import (
	"testing"
	"time"
)

func TestNewDescriptorWithPadding(t *testing.T) {
	filename := "test.txt"
	originalSize := int64(1000)
	paddedSize := int64(1024)
	blockSize := 128

	desc := NewDescriptor(filename, originalSize, paddedSize, blockSize)

	if desc.Filename != filename {
		t.Errorf("Expected filename %s, got %s", filename, desc.Filename)
	}

	if desc.FileSize != originalSize {
		t.Errorf("Expected original size %d, got %d", originalSize, desc.FileSize)
	}

	if desc.PaddedFileSize != paddedSize {
		t.Errorf("Expected padded size %d, got %d", paddedSize, desc.PaddedFileSize)
	}

	if desc.BlockSize != blockSize {
		t.Errorf("Expected block size %d, got %d", blockSize, desc.BlockSize)
	}

	if desc.Version != "4.0" {
		t.Errorf("Expected version 4.0, got %s", desc.Version)
	}
}

func TestIsPadded(t *testing.T) {
	tests := []struct {
		name         string
		originalSize int64
		paddedSize   int64
		expected     bool
	}{
		{
			name:         "padded file",
			originalSize: 1000,
			paddedSize:   1024,
			expected:     true,
		},
		{
			name:         "exact size",
			originalSize: 1024,
			paddedSize:   1024,
			expected:     false,
		},
		{
			name:         "no padding info",
			originalSize: 1000,
			paddedSize:   0,
			expected:     false,
		},
		{
			name:         "invalid padding",
			originalSize: 1024,
			paddedSize:   1000,
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc := NewDescriptor("test.txt", tt.originalSize, tt.paddedSize, 128)

			if desc.IsPadded() != tt.expected {
				t.Errorf("Expected IsPadded() = %v, got %v", tt.expected, desc.IsPadded())
			}
		})
	}
}

func TestGetOriginalFileSize(t *testing.T) {
	originalSize := int64(1000)
	paddedSize := int64(1024)

	desc := NewDescriptor("test.txt", originalSize, paddedSize, 128)

	if desc.GetOriginalFileSize() != originalSize {
		t.Errorf("Expected original size %d, got %d", originalSize, desc.GetOriginalFileSize())
	}
}

func TestGetPaddedFileSize(t *testing.T) {
	tests := []struct {
		name         string
		originalSize int64
		paddedSize   int64
		expected     int64
	}{
		{
			name:         "with padding info",
			originalSize: 1000,
			paddedSize:   1024,
			expected:     1024,
		},
		{
			name:         "no padding info",
			originalSize: 1000,
			paddedSize:   0,
			expected:     1000, // Falls back to original size
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc := NewDescriptor("test.txt", tt.originalSize, tt.paddedSize, 128)

			if desc.GetPaddedFileSize() != tt.expected {
				t.Errorf("Expected padded size %d, got %d", tt.expected, desc.GetPaddedFileSize())
			}
		})
	}
}

func TestNoBackwardCompatibility(t *testing.T) {
	// Test that all descriptors now require padding info
	desc := NewDescriptor("test.txt", 1000, 1000, 128)

	// Should not be considered padded if same size
	if desc.IsPadded() {
		t.Error("Descriptor with same original and padded size should not be considered padded")
	}

	// Should return correct values for both methods
	if desc.GetOriginalFileSize() != 1000 {
		t.Errorf("Expected original size 1000, got %d", desc.GetOriginalFileSize())
	}

	if desc.GetPaddedFileSize() != 1000 {
		t.Errorf("Expected padded size 1000, got %d", desc.GetPaddedFileSize())
	}
}

func TestPaddingSerializationRoundTrip(t *testing.T) {
	original := NewDescriptor("test.txt", 1000, 1024, 128)

	// Add some blocks to make it more realistic
	err := original.AddBlockTriple("data1", "rand1", "rand2")
	if err != nil {
		t.Fatalf("Failed to add block triple: %v", err)
	}

	// Serialize to JSON
	jsonData, err := original.ToJSON()
	if err != nil {
		t.Fatalf("Failed to serialize: %v", err)
	}

	// Deserialize from JSON
	deserialized, err := FromJSON(jsonData)
	if err != nil {
		t.Fatalf("Failed to deserialize: %v", err)
	}

	// Verify all fields are preserved
	if deserialized.Filename != original.Filename {
		t.Errorf("Filename mismatch: expected %s, got %s", original.Filename, deserialized.Filename)
	}

	if deserialized.FileSize != original.FileSize {
		t.Errorf("FileSize mismatch: expected %d, got %d", original.FileSize, deserialized.FileSize)
	}

	if deserialized.PaddedFileSize != original.PaddedFileSize {
		t.Errorf("PaddedFileSize mismatch: expected %d, got %d", original.PaddedFileSize, deserialized.PaddedFileSize)
	}

	if deserialized.BlockSize != original.BlockSize {
		t.Errorf("BlockSize mismatch: expected %d, got %d", original.BlockSize, deserialized.BlockSize)
	}

	if deserialized.Version != original.Version {
		t.Errorf("Version mismatch: expected %s, got %s", original.Version, deserialized.Version)
	}

	// Verify padding methods work the same
	if deserialized.IsPadded() != original.IsPadded() {
		t.Error("IsPadded() mismatch after serialization")
	}

	if deserialized.GetOriginalFileSize() != original.GetOriginalFileSize() {
		t.Error("GetOriginalFileSize() mismatch after serialization")
	}

	if deserialized.GetPaddedFileSize() != original.GetPaddedFileSize() {
		t.Error("GetPaddedFileSize() mismatch after serialization")
	}
}

func TestPaddingValidation(t *testing.T) {
	desc := NewDescriptor("test.txt", 1000, 1024, 128)

	// Add a block to make it valid
	err := desc.AddBlockTriple("data1", "rand1", "rand2")
	if err != nil {
		t.Fatalf("Failed to add block triple: %v", err)
	}

	// Should validate successfully
	if err := desc.Validate(); err != nil {
		t.Errorf("Padded descriptor should validate: %v", err)
	}
}

func TestPaddingTimestampPreservation(t *testing.T) {
	before := time.Now()
	desc := NewDescriptor("test.txt", 1000, 1024, 128)
	after := time.Now()

	if desc.CreatedAt.Before(before) || desc.CreatedAt.After(after) {
		t.Error("CreatedAt timestamp should be set during descriptor creation")
	}
}

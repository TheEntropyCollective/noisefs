package descriptors

import (
	"testing"
)

func TestDirectoryDescriptor(t *testing.T) {
	t.Run("NewDirectoryDescriptor", func(t *testing.T) {
		dirname := "test-directory"
		manifestCID := "QmTestManifestCID12345"

		desc := NewDirectoryDescriptor(dirname, manifestCID)

		if desc == nil {
			t.Fatal("NewDirectoryDescriptor() returned nil")
		}

		if desc.Version != "4.0" {
			t.Errorf("Version = %v, want 4.0", desc.Version)
		}

		if desc.Type != DirectoryType {
			t.Errorf("Type = %v, want %v", desc.Type, DirectoryType)
		}

		if desc.Filename != dirname {
			t.Errorf("Filename = %v, want %v", desc.Filename, dirname)
		}

		if desc.ManifestCID != manifestCID {
			t.Errorf("ManifestCID = %v, want %v", desc.ManifestCID, manifestCID)
		}

		if desc.FileSize != 0 {
			t.Errorf("FileSize = %v, want 0", desc.FileSize)
		}

		if desc.BlockSize != 0 {
			t.Errorf("BlockSize = %v, want 0", desc.BlockSize)
		}

		if len(desc.Blocks) != 0 {
			t.Errorf("Blocks length = %v, want 0", len(desc.Blocks))
		}
	})

	t.Run("DirectoryDescriptorValidation", func(t *testing.T) {
		desc := NewDirectoryDescriptor("test-dir", "QmTestCID")

		if err := desc.Validate(); err != nil {
			t.Errorf("Valid directory descriptor failed validation: %v", err)
		}

		// Test invalid cases
		desc.ManifestCID = ""
		if err := desc.Validate(); err == nil {
			t.Error("Directory descriptor without manifest CID should fail validation")
		}

		// Reset and test with blocks
		desc = NewDirectoryDescriptor("test-dir", "QmTestCID")
		desc.Blocks = []BlockPair{{DataCID: "test", RandomizerCID1: "r1", RandomizerCID2: "r2"}}
		if err := desc.Validate(); err == nil {
			t.Error("Directory descriptor with blocks should fail validation")
		}
	})

	t.Run("TypeHelpers", func(t *testing.T) {
		fileDesc := NewDescriptor("test.txt", 1024, 1024, 128)
		if !fileDesc.IsFile() {
			t.Error("File descriptor should return true for IsFile()")
		}
		if fileDesc.IsDirectory() {
			t.Error("File descriptor should return false for IsDirectory()")
		}

		dirDesc := NewDirectoryDescriptor("test-dir", "QmTestCID")
		if dirDesc.IsFile() {
			t.Error("Directory descriptor should return false for IsFile()")
		}
		if !dirDesc.IsDirectory() {
			t.Error("Directory descriptor should return true for IsDirectory()")
		}
	})
}

// TestBackwardCompatibility removed - no longer supporting v3 format without Type field

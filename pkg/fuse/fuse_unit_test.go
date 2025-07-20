package fuse

import (
	"testing"
)

// TestFileIndexOperations tests basic file index operations
func TestFileIndexOperations(t *testing.T) {
	// Create temporary index file
	indexFile := "/tmp/test_index.json"

	// Create new index
	index := NewFileIndex(indexFile)

	// Test adding files
	index.AddFile("test1.txt", "QmTest1", 1024)
	index.AddFile("test2.txt", "QmTest2", 2048)

	// Test file listing
	files := index.ListFiles()
	if len(files) != 2 {
		t.Errorf("Expected 2 files, got %d", len(files))
	}

	// Test file retrieval
	if entry, exists := files["test1.txt"]; !exists {
		t.Error("test1.txt should exist")
	} else {
		if entry.DescriptorCID != "QmTest1" {
			t.Errorf("Expected CID QmTest1, got %s", entry.DescriptorCID)
		}
		if entry.FileSize != 1024 {
			t.Errorf("Expected size 1024, got %d", entry.FileSize)
		}
	}

	// Test file removal
	if !index.RemoveFile("test1.txt") {
		t.Error("Should be able to remove test1.txt")
	}

	files = index.ListFiles()
	if len(files) != 1 {
		t.Errorf("Expected 1 file after removal, got %d", len(files))
	}

	// Test removing non-existent file
	if index.RemoveFile("nonexistent.txt") {
		t.Error("Should not be able to remove non-existent file")
	}
}

// TestMountOptions tests mount option validation
func TestMountOptions(t *testing.T) {
	// Test valid mount options
	opts := MountOptions{
		MountPath:  "/tmp/test_mount",
		VolumeName: "test_volume",
		ReadOnly:   false,
		AllowOther: false,
		Debug:      false,
	}

	if opts.MountPath != "/tmp/test_mount" {
		t.Errorf("Expected mount path /tmp/test_mount, got %s", opts.MountPath)
	}

	if opts.VolumeName != "test_volume" {
		t.Errorf("Expected volume name test_volume, got %s", opts.VolumeName)
	}

	if opts.ReadOnly != false {
		t.Errorf("Expected ReadOnly false, got %v", opts.ReadOnly)
	}
}

// TestDefaultIndexPath tests default index path generation
func TestDefaultIndexPath(t *testing.T) {
	path, err := GetDefaultIndexPath()
	if err != nil {
		t.Fatalf("Failed to get default index path: %v", err)
	}

	if path == "" {
		t.Error("Default index path should not be empty")
	}

	// Path should end with index.json
	if len(path) < 10 || path[len(path)-10:] != "index.json" {
		t.Errorf("Expected path to end with index.json, got %s", path)
	}
}

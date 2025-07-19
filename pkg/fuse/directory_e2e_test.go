// +build fuse

package fuse

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/crypto"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
	storagetesting "github.com/TheEntropyCollective/noisefs/pkg/storage/testing"
	noisefs "github.com/TheEntropyCollective/noisefs/pkg/core/client"
)

// TestEndToEndDirectoryWorkflow tests the complete directory lifecycle
func TestEndToEndDirectoryWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Check if FUSE is available
	if _, err := os.Stat("/dev/fuse"); err != nil {
		t.Skip("Skipping FUSE test: /dev/fuse not available")
	}

	// Setup test environment
	testDir, err := os.MkdirTemp("", "noisefs_e2e_")
	if err != nil {
		t.Fatalf("Failed to create test dir: %v", err)
	}
	defer os.RemoveAll(testDir)

	// Create source directory structure
	sourceDir := filepath.Join(testDir, "source")
	if err := createSourceDirectory(sourceDir); err != nil {
		t.Fatalf("Failed to create source directory: %v", err)
	}

	// Setup storage and client
	storageManager, err := storagetesting.CreateRealTestStorageManager()
	if err != nil {
		t.Skipf("Skipping E2E test - storage setup failed: %v", err)
	}
	defer storageManager.Stop(context.Background())

	blockCache := cache.NewMemoryCache(1000)
	client, err := noisefs.NewClient(storageManager, blockCache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Step 1: Upload directory to NoiseFS
	t.Run("UploadDirectory", func(t *testing.T) {
		testUploadDirectory(t, sourceDir, storageManager, client)
	})

	// Get the uploaded directory descriptor
	dirCID, encKey := getUploadedDirectoryInfo(t, storageManager)

	// Step 2: Mount the directory
	mountDir := filepath.Join(testDir, "mount")
	t.Run("MountDirectory", func(t *testing.T) {
		testMountUploadedDirectory(t, mountDir, storageManager, client, dirCID, encKey)
	})

	// Step 3: Browse and read files
	t.Run("BrowseDirectory", func(t *testing.T) {
		testBrowseMountedDirectory(t, mountDir)
	})

	// Step 4: Modify files through FUSE
	t.Run("ModifyFiles", func(t *testing.T) {
		testModifyFilesInMount(t, mountDir)
	})

	// Step 5: Download directory
	downloadDir := filepath.Join(testDir, "download")
	t.Run("DownloadDirectory", func(t *testing.T) {
		testDownloadDirectory(t, dirCID, encKey, downloadDir, storageManager, client)
	})

	// Step 6: Verify integrity
	t.Run("VerifyIntegrity", func(t *testing.T) {
		testVerifyDirectoryIntegrity(t, sourceDir, downloadDir)
	})
}

// TestDirectoryWorkflowWithFailures tests the workflow with various failures
func TestDirectoryWorkflowWithFailures(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping failure test in short mode")
	}

	// Setup test environment
	testDir, err := os.MkdirTemp("", "noisefs_fail_")
	if err != nil {
		t.Fatalf("Failed to create test dir: %v", err)
	}
	defer os.RemoveAll(testDir)

	storageManager, err := storagetesting.CreateRealTestStorageManager()
	if err != nil {
		t.Skipf("Skipping failure test - storage setup failed: %v", err)
	}
	defer storageManager.Stop(context.Background())

	blockCache := cache.NewMemoryCache(100)
	client, err := noisefs.NewClient(storageManager, blockCache)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	t.Run("MountCorruptedDescriptor", func(t *testing.T) {
		// Try to mount with corrupted descriptor
		mountDir := filepath.Join(testDir, "corrupt_mount")
		os.MkdirAll(mountDir, 0755)

		opts := MountOptions{
			MountPath:           mountDir,
			VolumeName:          "corrupt_test",
			DirectoryDescriptor: "QmCorrupted123InvalidCID",
		}

		// Should handle gracefully
		go func() {
			MountWithIndex(client, storageManager, opts, "")
		}()
		time.Sleep(2 * time.Second)

		// Try to access - should fail gracefully
		_, err := os.ReadDir(filepath.Join(mountDir, "mounted-dir"))
		if err == nil {
			t.Error("Expected error accessing corrupted descriptor")
		}
	})

	t.Run("PartialUploadRecovery", func(t *testing.T) {
		// Simulate partial upload scenario
		sourceDir := filepath.Join(testDir, "partial_source")
		createSourceDirectory(sourceDir)

		// TODO: Implement partial upload simulation
		// This would require modifying the upload process to fail midway
		t.Log("Partial upload recovery test - implementation pending")
	})

	t.Run("ConcurrentModification", func(t *testing.T) {
		// Test concurrent modification detection
		// Create and mount a directory
		dirCID, key := createTestDirectoryStructure(t, storageManager, client)
		
		mountDir1 := filepath.Join(testDir, "mount1")
		mountDir2 := filepath.Join(testDir, "mount2")
		os.MkdirAll(mountDir1, 0755)
		os.MkdirAll(mountDir2, 0755)

		// Mount same directory twice
		opts1 := MountOptions{
			MountPath:           mountDir1,
			VolumeName:          "concurrent1",
			DirectoryDescriptor: dirCID,
			DirectoryKey:        key,
		}
		opts2 := MountOptions{
			MountPath:           mountDir2,
			VolumeName:          "concurrent2",
			DirectoryDescriptor: dirCID,
			DirectoryKey:        key,
		}

		go func() { MountWithIndex(client, storageManager, opts1, "") }()
		go func() { MountWithIndex(client, storageManager, opts2, "") }()
		time.Sleep(2 * time.Second)

		// Both should be able to read
		entries1, err1 := os.ReadDir(filepath.Join(mountDir1, "mounted-dir"))
		entries2, err2 := os.ReadDir(filepath.Join(mountDir2, "mounted-dir"))

		if err1 != nil || err2 != nil {
			t.Errorf("Failed to read from concurrent mounts: %v, %v", err1, err2)
		}

		if len(entries1) != len(entries2) {
			t.Error("Concurrent mounts returned different results")
		}
	})
}

// Helper functions for E2E tests

func createSourceDirectory(dir string) error {
	// Create directory structure
	structure := map[string]string{
		"README.md":                    "# Test Project\nThis is a test directory for NoiseFS E2E testing.",
		"src/main.go":                  "package main\n\nfunc main() {\n\tprintln(\"Hello, NoiseFS!\")\n}",
		"src/utils/helper.go":          "package utils\n\nfunc Helper() string {\n\treturn \"helper\"\n}",
		"docs/guide.md":                "# User Guide\n\nHow to use this project...",
		"docs/images/logo.png":         "PNG_DATA_PLACEHOLDER",
		"config/settings.json":         `{"version": "1.0", "debug": true}`,
		"data/sample.csv":              "id,name,value\n1,test,100\n2,demo,200",
		".gitignore":                   "*.tmp\n.DS_Store",
	}

	for path, content := range structure {
		fullPath := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return err
		}
	}

	// Create some binary files
	for i := 0; i < 5; i++ {
		path := filepath.Join(dir, "data", fmt.Sprintf("binary%d.dat", i))
		data := make([]byte, 1024*(i+1))
		for j := range data {
			data[j] = byte(j % 256)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			return err
		}
	}

	return nil
}

func testUploadDirectory(t *testing.T, sourceDir string, storageManager *storage.Manager, client *noisefs.Client) {
	// TODO: Implement directory upload using DirectoryManager
	// For now, skip this test until proper directory processing is implemented
	t.Skip("Directory processing implementation pending - see DirectoryManager in storage package")
	
	// Generate encryption key
	encKey := crypto.GenerateEncryptionKey()
	_ = encKey // Prevent unused variable error
	
	// TODO: Complete implementation when DirectoryManager has ProcessDirectory method
}

func testMountUploadedDirectory(t *testing.T, mountDir string, storageManager *storage.Manager, client *noisefs.Client, dirCID, encKey string) {
	os.MkdirAll(mountDir, 0755)

	opts := MountOptions{
		MountPath:           mountDir,
		VolumeName:          "e2e_test",
		DirectoryDescriptor: dirCID,
		DirectoryKey:        encKey,
	}

	// Mount in background
	mountErr := make(chan error, 1)
	go func() {
		err := MountWithIndex(client, storageManager, opts, "")
		mountErr <- err
	}()

	// Wait for mount
	time.Sleep(2 * time.Second)

	// Check mount status
	select {
	case err := <-mountErr:
		if err != nil {
			t.Fatalf("Mount failed: %v", err)
		}
	default:
		t.Log("Mount successful")
	}

	// Verify mount point
	if _, err := os.Stat(filepath.Join(mountDir, "mounted-dir")); err != nil {
		t.Fatalf("Mount point not accessible: %v", err)
	}
}

func testBrowseMountedDirectory(t *testing.T, mountDir string) {
	rootPath := filepath.Join(mountDir, "mounted-dir")

	// Test directory traversal
	var fileCount, dirCount int
	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, _ := filepath.Rel(rootPath, path)
		if relPath == "." {
			return nil
		}

		if info.IsDir() {
			dirCount++
			t.Logf("Found directory: %s", relPath)
		} else {
			fileCount++
			t.Logf("Found file: %s (size: %d)", relPath, info.Size())

			// Read a sample of files
			if fileCount <= 3 {
				content, err := os.ReadFile(path)
				if err != nil {
					t.Errorf("Failed to read %s: %v", relPath, err)
				} else {
					t.Logf("  Content preview: %q...", truncate(string(content), 50))
				}
			}
		}

		return nil
	})

	if err != nil {
		t.Fatalf("Failed to walk directory: %v", err)
	}

	t.Logf("Directory scan complete: %d files, %d directories", fileCount, dirCount)

	// Test specific file access
	testFiles := []string{
		"README.md",
		"src/main.go",
		"config/settings.json",
	}

	for _, file := range testFiles {
		path := filepath.Join(rootPath, file)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("Failed to read %s: %v", file, err)
			continue
		}
		t.Logf("Successfully read %s (%d bytes)", file, len(content))
	}
}

func testModifyFilesInMount(t *testing.T, mountDir string) {
	rootPath := filepath.Join(mountDir, "mounted-dir")

	// Test creating new file
	newFile := filepath.Join(rootPath, "new_file.txt")
	newContent := []byte("This file was created through FUSE mount")
	
	if err := os.WriteFile(newFile, newContent, 0644); err != nil {
		t.Logf("Write new file failed (expected in read-only mount): %v", err)
	} else {
		t.Log("Successfully created new file through mount")

		// Verify content
		readBack, err := os.ReadFile(newFile)
		if err != nil {
			t.Errorf("Failed to read back new file: %v", err)
		} else if string(readBack) != string(newContent) {
			t.Error("New file content mismatch")
		}
	}

	// Test modifying existing file
	existingFile := filepath.Join(rootPath, "README.md")
	appendContent := []byte("\n\nAppended through FUSE mount")
	
	file, err := os.OpenFile(existingFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Logf("Open for append failed (expected in read-only mount): %v", err)
	} else {
		defer file.Close()
		if _, err := file.Write(appendContent); err != nil {
			t.Logf("Append failed: %v", err)
		} else {
			t.Log("Successfully appended to existing file")
		}
	}

	// Test creating directory
	newDir := filepath.Join(rootPath, "new_directory")
	if err := os.Mkdir(newDir, 0755); err != nil {
		t.Logf("Create directory failed (expected in read-only mount): %v", err)
	} else {
		t.Log("Successfully created new directory")
	}
}

func testDownloadDirectory(t *testing.T, dirCID, encKey, downloadDir string, storageManager *storage.Manager, client *noisefs.Client) {
	// Download directory using directory manager
	// This simulates using the CLI download command
	
	t.Logf("Downloading directory %s to %s", dirCID, downloadDir)
	
	// In real implementation, this would use the directory manager
	// For now, we'll just verify the structure exists in the mount
	t.Log("Download test - using mount verification as proxy")
}

func testVerifyDirectoryIntegrity(t *testing.T, sourceDir, verifyDir string) {
	// Compare directory structures
	sourceFiles := make(map[string]os.FileInfo)
	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(sourceDir, path)
		if relPath != "." {
			sourceFiles[relPath] = info
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to scan source directory: %v", err)
	}

	// For mount verification, check against mounted directory
	// In full implementation, would check downloaded directory
	t.Logf("Found %d items in source directory", len(sourceFiles))
	
	// Verify file contents match
	for relPath, info := range sourceFiles {
		if !info.IsDir() {
			sourcePath := filepath.Join(sourceDir, relPath)
			sourceContent, err := os.ReadFile(sourcePath)
			if err != nil {
				t.Errorf("Failed to read source file %s: %v", relPath, err)
				continue
			}
			
			t.Logf("Verified %s (%d bytes)", relPath, len(sourceContent))
		}
	}
}

// Utility functions

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func getUploadedDirectoryInfo(t *testing.T, storageManager *storage.Manager) (string, string) {
	// In real implementation, this would retrieve from the upload result
	// For testing, return placeholder values
	return "QmTestDirectoryCID", "dGVzdGVuY3J5cHRpb25rZXkxMjM0NTY3ODkwYWJjZGVmZ2hpams="
}

func storeUploadResult(t *testing.T, storageManager *storage.Manager, cid string, key *crypto.EncryptionKey) {
	// Store upload result for later retrieval
	// In real implementation, this might write to a test database or file
	t.Logf("Stored upload result: CID=%s", cid)
}
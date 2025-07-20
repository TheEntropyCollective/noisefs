//go:build fuse

package testutil

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestExampleUsage demonstrates how to use the testutil package
func TestExampleUsage(t *testing.T) {
	// Example 1: Basic test environment setup
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	t.Logf("Created test environment at: %s", env.TempDir)
	t.Logf("Mount directory: %s", env.MountDir)

	// Example 2: Create test directory structure
	dirCID, encKey := CreateTestDirectoryStructure(t, env)
	t.Logf("Created test directory with CID: %s", dirCID)
	t.Logf("Encryption key: %s", encKey[:20]+"...")

	// Example 3: Create mount options (for demonstration)
	_ = env.CreateDirectoryMountOptions(dirCID, encKey)
	t.Logf("Created mount options for directory mounting")

	// Example 4: Use mock client
	mockClient := NewMockNoisefsClient()
	
	// Store some test data
	testBlock := &MockBlock{data: []byte("test file content")}
	cid, err := mockClient.StoreBlockWithCache(testBlock)
	if err != nil {
		t.Fatalf("Failed to store test block: %v", err)
	}
	t.Logf("Stored test block with CID: %s", cid)

	// Retrieve the data
	retrievedBlock, err := mockClient.RetrieveBlockWithCache(cid)
	if err != nil {
		t.Fatalf("Failed to retrieve test block: %v", err)
	}
	t.Logf("Retrieved block data: %s", string(retrievedBlock.Data()))

	// Example 5: File creation helpers
	helper := NewFileCreationHelper(env.TempDir)
	
	// Create test files
	err = helper.CreateFile("example.txt", []byte("Hello, NoiseFS!"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	
	err = helper.CreateFileWithRandomContent("random_data.bin", 1024, 0644)
	if err != nil {
		t.Fatalf("Failed to create random file: %v", err)
	}

	// Example 6: Assertions
	AssertFileExists(t, filepath.Join(env.TempDir, "example.txt"))
	AssertFileContent(t, filepath.Join(env.TempDir, "example.txt"), []byte("Hello, NoiseFS!"))
	AssertFileSize(t, filepath.Join(env.TempDir, "random_data.bin"), 1024)

	// Example 7: Performance measurement
	duration := MeasureTime(func() {
		// Simulate some work
		time.Sleep(10 * time.Millisecond)
	})
	t.Logf("Operation took: %v", duration)
	AssertOperationTime(t, duration, 50*time.Millisecond, "test operation")

	// Example 8: Concurrent operations
	operations := []func() error{
		func() error { return helper.CreateFile("concurrent1.txt", []byte("data1"), 0644) },
		func() error { return helper.CreateFile("concurrent2.txt", []byte("data2"), 0644) },
		func() error { return helper.CreateFile("concurrent3.txt", []byte("data3"), 0644) },
	}
	
	errors := RunConcurrentOperations(operations)
	AssertNoConcurrentErrors(t, errors, "file creation")

	// Example 9: Directory structure creation
	structure := TestDirectory{
		Name: "test_hierarchy",
		Files: []TestFile{
			{Name: "file1.txt", Content: []byte("content1"), Mode: 0644},
			{Name: "file2.txt", Content: []byte("content2"), Mode: 0644},
		},
		Directories: []TestDirectory{
			{
				Name: "subdir",
				Files: []TestFile{
					{Name: "nested.txt", Content: []byte("nested content"), Mode: 0644},
				},
				Mode: 0755,
			},
		},
		Mode: 0755,
	}
	
	err = helper.CreateDirectoryStructure(structure)
	if err != nil {
		t.Fatalf("Failed to create directory structure: %v", err)
	}
	
	// Verify the structure
	AssertDirectoryExists(t, filepath.Join(env.TempDir, "test_hierarchy"))
	AssertDirectoryExists(t, filepath.Join(env.TempDir, "test_hierarchy", "subdir"))
	AssertFileExists(t, filepath.Join(env.TempDir, "test_hierarchy", "file1.txt"))
	AssertFileExists(t, filepath.Join(env.TempDir, "test_hierarchy", "subdir", "nested.txt"))

	t.Log("All testutil examples completed successfully!")
}

// TestFUSEIntegrationExample demonstrates FUSE-specific testing
func TestFUSEIntegrationExample(t *testing.T) {
	// Skip if FUSE not available
	CheckFUSEAvailable(t)
	SkipInShortMode(t, "FUSE integration test")

	// Setup test environment
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create test directory structure
	dirCID, encKey := CreateTestDirectoryStructure(t, env)

	// Create mount options
	opts := env.CreateDirectoryMountOptions(dirCID, encKey)

	// Start mount in background (this would work with real FUSE)
	mountErr := env.StartMountInBackground(opts)
	
	// In a real test, you would:
	// 1. Wait for mount to be ready
	// 2. Perform file operations
	// 3. Test directory listing
	// 4. Verify FUSE functionality
	
	// For this example, just check that mount was attempted
	select {
	case err := <-mountErr:
		// Mount failed immediately (expected in test environment)
		t.Logf("Mount failed as expected in test environment: %v", err)
	case <-time.After(100 * time.Millisecond):
		// Mount is running (would be success in real environment)
		t.Log("Mount appears to be running")
	}

	t.Log("FUSE integration example completed!")
}

// TestPerformanceTestingExample demonstrates performance testing utilities
func TestPerformanceTestingExample(t *testing.T) {
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Test file creation performance
	helper := NewFileCreationHelper(env.TempDir)
	
	// Measure file creation time
	duration, err := MeasureTimeWithError(func() error {
		return helper.CreateFileWithRandomContent("perf_test.bin", 10*1024, 0644)
	})
	
	if err != nil {
		t.Fatalf("Performance test failed: %v", err)
	}
	
	t.Logf("Created 10KB file in: %v", duration)
	AssertOperationTime(t, duration, 100*time.Millisecond, "file creation")

	// Test multiple file operations
	fileCount := 100
	totalDuration := MeasureTime(func() {
		for i := 0; i < fileCount; i++ {
			filename := generateTestFileName(i)
			content := GenerateTestContent(512) // 512 bytes each
			helper.CreateFile(filename, content, 0644)
		}
	})
	
	avgDuration := totalDuration / time.Duration(fileCount)
	t.Logf("Created %d files in %v (avg: %v per file)", fileCount, totalDuration, avgDuration)

	// Verify all files were created
	entries, err := os.ReadDir(env.TempDir)
	if err != nil {
		t.Fatalf("Failed to read temp directory: %v", err)
	}
	
	// Should have: perf_test.bin + 100 test files = 101 files minimum
	if len(entries) < 101 {
		t.Errorf("Expected at least 101 files, got %d", len(entries))
	}

	t.Log("Performance testing example completed!")
}

// TestLargeDirectoryExample demonstrates testing with large directories
func TestLargeDirectoryExample(t *testing.T) {
	SkipInShortMode(t, "large directory test")
	
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Create a large directory structure
	fileCount := 1000
	dirCID, _ := CreateLargeTestDirectory(t, env, fileCount)
	
	t.Logf("Created large directory with %d files, CID: %s", fileCount, dirCID)

	// Create nested directory structure
	depth := 5
	nestedCID := CreateNestedDirectoryStructure(t, env, depth)
	
	t.Logf("Created nested directory structure with depth %d, CID: %s", depth, nestedCID)

	t.Log("Large directory example completed!")
}
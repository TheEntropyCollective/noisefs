//go:build fuse

package testutil

import (
	"os"
	"testing"
	"time"
)

// TestSetupTestEnvironment tests the basic test environment setup
func TestSetupTestEnvironment(t *testing.T) {
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Verify environment components
	if env.TempDir == "" {
		t.Error("TempDir should not be empty")
	}
	if env.MountDir == "" {
		t.Error("MountDir should not be empty")
	}
	if env.StorageManager == nil {
		t.Error("StorageManager should not be nil")
	}
	if env.Client == nil {
		t.Error("Client should not be nil")
	}
	if env.Cache == nil {
		t.Error("Cache should not be nil")
	}

	// Verify directories exist
	if _, err := os.Stat(env.TempDir); os.IsNotExist(err) {
		t.Error("TempDir should exist")
	}
	if _, err := os.Stat(env.MountDir); os.IsNotExist(err) {
		t.Error("MountDir should exist")
	}
}

// TestMockNoisefsClient tests the mock client functionality
func TestMockNoisefsClient(t *testing.T) {
	client := NewMockNoisefsClient()

	// Test initial state
	if client.GetUploadCalls() != 0 {
		t.Error("Initial upload calls should be 0")
	}
	if client.GetDownloadCalls() != 0 {
		t.Error("Initial download calls should be 0")
	}

	// Test randomizer selection
	block1, cid1, block2, cid2, newStorage, err := client.SelectRandomizers(1024)
	if err != nil {
		t.Fatalf("SelectRandomizers failed: %v", err)
	}
	if block1 == nil || block2 == nil {
		t.Error("Randomizer blocks should not be nil")
	}
	if cid1 == "" || cid2 == "" {
		t.Error("Randomizer CIDs should not be empty")
	}
	if len(block1.Data()) != 1024 {
		t.Errorf("Block1 size mismatch: expected 1024, got %d", len(block1.Data()))
	}
	if len(block2.Data()) != 1024 {
		t.Errorf("Block2 size mismatch: expected 1024, got %d", len(block2.Data()))
	}
	if newStorage != 0 {
		t.Errorf("New storage should be 0 for mock, got %d", newStorage)
	}

	// Test block storage and retrieval
	testBlock := &MockBlock{data: []byte("test data")}
	cid, err := client.StoreBlockWithCache(testBlock)
	if err != nil {
		t.Fatalf("StoreBlockWithCache failed: %v", err)
	}
	if cid == "" {
		t.Error("Stored block CID should not be empty")
	}
	if client.GetUploadCalls() != 1 {
		t.Errorf("Upload calls should be 1, got %d", client.GetUploadCalls())
	}

	// Test block retrieval
	retrievedBlock, err := client.RetrieveBlockWithCache(cid)
	if err != nil {
		t.Fatalf("RetrieveBlockWithCache failed: %v", err)
	}
	if string(retrievedBlock.Data()) != "test data" {
		t.Errorf("Retrieved data mismatch: expected 'test data', got '%s'", string(retrievedBlock.Data()))
	}
	if client.GetDownloadCalls() != 1 {
		t.Errorf("Download calls should be 1, got %d", client.GetDownloadCalls())
	}
}

// TestFileCreationHelper tests the file creation utilities
func TestFileCreationHelper(t *testing.T) {
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "testutil_test_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	helper := NewFileCreationHelper(tempDir)

	// Test file creation
	testContent := []byte("test file content")
	err = helper.CreateFile("test.txt", testContent, 0644)
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}

	// Verify file was created
	AssertFileExists(t, tempDir+"/test.txt")
	AssertFileContent(t, tempDir+"/test.txt", testContent)
	AssertFileSize(t, tempDir+"/test.txt", int64(len(testContent)))

	// Test directory creation
	err = helper.CreateDirectory("subdir", 0755)
	if err != nil {
		t.Fatalf("CreateDirectory failed: %v", err)
	}
	AssertDirectoryExists(t, tempDir+"/subdir")

	// Test random content file creation
	err = helper.CreateFileWithRandomContent("random.txt", 100, 0644)
	if err != nil {
		t.Fatalf("CreateFileWithRandomContent failed: %v", err)
	}
	AssertFileExists(t, tempDir+"/random.txt")
	AssertFileSize(t, tempDir+"/random.txt", 100)
}

// TestGenerateTestData tests the test data generators
func TestGenerateTestData(t *testing.T) {
	// Test content generation
	content := GenerateTestContent(100)
	if len(content) != 100 {
		t.Errorf("Generated content size mismatch: expected 100, got %d", len(content))
	}

	// Test random content generation
	randomContent := GenerateRandomTestContent(50)
	if len(randomContent) != 50 {
		t.Errorf("Generated random content size mismatch: expected 50, got %d", len(randomContent))
	}

	// Test file generation
	files := GenerateTestFiles(5, 200)
	if len(files) != 5 {
		t.Errorf("Generated files count mismatch: expected 5, got %d", len(files))
	}
	for i, file := range files {
		if len(file.Content) != 200 {
			t.Errorf("File %d content size mismatch: expected 200, got %d", i, len(file.Content))
		}
		if file.Mode != 0644 {
			t.Errorf("File %d mode mismatch: expected 0644, got %o", i, file.Mode)
		}
	}
}

// TestMeasureTime tests the timing utilities
func TestMeasureTime(t *testing.T) {
	// Test basic time measurement
	duration := MeasureTime(func() {
		time.Sleep(10 * time.Millisecond)
	})

	if duration < 10*time.Millisecond {
		t.Errorf("Measured duration too short: %v", duration)
	}
	if duration > 50*time.Millisecond {
		t.Errorf("Measured duration too long: %v", duration)
	}

	// Test time measurement with error
	duration, err := MeasureTimeWithError(func() error {
		time.Sleep(5 * time.Millisecond)
		return nil
	})

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if duration < 5*time.Millisecond {
		t.Errorf("Measured duration too short: %v", duration)
	}
}

// TestCreateTestDirectoryStructure tests directory structure creation
func TestCreateTestDirectoryStructure(t *testing.T) {
	env := SetupTestEnvironment(t)
	defer env.Cleanup()

	// Test basic directory structure creation
	dirCID, encKey := CreateTestDirectoryStructure(t, env)
	if dirCID == "" {
		t.Error("Directory CID should not be empty")
	}
	if encKey == "" {
		t.Error("Encryption key should not be empty")
	}

	// Test large directory creation
	largeDirCID, _ := CreateLargeTestDirectory(t, env, 100)
	if largeDirCID == "" {
		t.Error("Large directory CID should not be empty")
	}

	// Test nested directory creation
	nestedDirCID := CreateNestedDirectoryStructure(t, env, 3)
	if nestedDirCID == "" {
		t.Error("Nested directory CID should not be empty")
	}
}

// TestMockStorageBackend tests the mock storage backend
func TestMockStorageBackend(t *testing.T) {
	backend := NewMockStorageBackend()

	// Test normal operation
	testData := []byte("test storage data")
	cid, err := backend.Put(nil, testData)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	data, err := backend.Get(nil, cid)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(data) != string(testData) {
		t.Errorf("Data mismatch: expected '%s', got '%s'", string(testData), string(data))
	}

	has, err := backend.Has(nil, cid)
	if err != nil {
		t.Fatalf("Has failed: %v", err)
	}
	if !has {
		t.Error("Backend should have the stored data")
	}

	// Test failure mode
	backend.SetFailMode(true, os.ErrNotExist)
	_, err = backend.Put(nil, testData)
	if err != os.ErrNotExist {
		t.Errorf("Expected ErrNotExist, got %v", err)
	}
}
//go:build fuse

package testutil

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
)

// TestFile represents a test file with content
type TestFile struct {
	Name    string
	Content []byte
	Mode    os.FileMode
}

// TestDirectory represents a test directory structure
type TestDirectory struct {
	Name        string
	Files       []TestFile
	Directories []TestDirectory
	Mode        os.FileMode
}

// FileCreationHelper provides utilities for creating test files
type FileCreationHelper struct {
	baseDir string
}

// NewFileCreationHelper creates a new file creation helper
func NewFileCreationHelper(baseDir string) *FileCreationHelper {
	return &FileCreationHelper{baseDir: baseDir}
}

// CreateFile creates a single test file
func (h *FileCreationHelper) CreateFile(relativePath string, content []byte, mode os.FileMode) error {
	fullPath := filepath.Join(h.baseDir, relativePath)
	
	// Ensure directory exists
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	
	return os.WriteFile(fullPath, content, mode)
}

// CreateFileWithRandomContent creates a file with random content
func (h *FileCreationHelper) CreateFileWithRandomContent(relativePath string, size int, mode os.FileMode) error {
	content := make([]byte, size)
	if _, err := rand.Read(content); err != nil {
		return fmt.Errorf("failed to generate random content: %w", err)
	}
	return h.CreateFile(relativePath, content, mode)
}

// CreateDirectory creates a directory structure
func (h *FileCreationHelper) CreateDirectory(relativePath string, mode os.FileMode) error {
	fullPath := filepath.Join(h.baseDir, relativePath)
	return os.MkdirAll(fullPath, mode)
}

// CreateDirectoryStructure creates a complete directory structure
func (h *FileCreationHelper) CreateDirectoryStructure(structure TestDirectory) error {
	// Create the directory itself
	if err := h.CreateDirectory(structure.Name, structure.Mode); err != nil {
		return err
	}
	
	// Create files in the directory
	for _, file := range structure.Files {
		filePath := filepath.Join(structure.Name, file.Name)
		if err := h.CreateFile(filePath, file.Content, file.Mode); err != nil {
			return err
		}
	}
	
	// Recursively create subdirectories
	for _, subdir := range structure.Directories {
		subdir.Name = filepath.Join(structure.Name, subdir.Name)
		if err := h.CreateDirectoryStructure(subdir); err != nil {
			return err
		}
	}
	
	return nil
}

// Test Data Generators

// GenerateTestContent creates test content of specified size
func GenerateTestContent(size int) []byte {
	content := make([]byte, size)
	for i := range content {
		content[i] = byte(i % 256)
	}
	return content
}

// GenerateRandomTestContent creates random test content of specified size
func GenerateRandomTestContent(size int) []byte {
	content := make([]byte, size)
	rand.Read(content)
	return content
}

// GenerateTestFiles creates multiple test files with incremental names
func GenerateTestFiles(count int, contentSize int) []TestFile {
	files := make([]TestFile, count)
	for i := 0; i < count; i++ {
		files[i] = TestFile{
			Name:    generateTestFileName(i),
			Content: GenerateTestContent(contentSize),
			Mode:    0644,
		}
	}
	return files
}

// GenerateTestDirectoryStructure creates a nested test directory structure
func GenerateTestDirectoryStructure(depth int, filesPerDir int, contentSize int) TestDirectory {
	if depth == 0 {
		return TestDirectory{
			Name:  "leaf",
			Files: GenerateTestFiles(filesPerDir, contentSize),
			Mode:  0755,
		}
	}
	
	return TestDirectory{
		Name:  fmt.Sprintf("level_%d", depth),
		Files: GenerateTestFiles(filesPerDir, contentSize),
		Directories: []TestDirectory{
			GenerateTestDirectoryStructure(depth-1, filesPerDir, contentSize),
		},
		Mode: 0755,
	}
}

// Assertion Utilities

// AssertFileExists checks if a file exists and fails the test if not
func AssertFileExists(t *testing.T, filePath string) {
	t.Helper()
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("File %s should exist but does not", filePath)
	}
}

// AssertFileNotExists checks if a file does not exist and fails the test if it does
func AssertFileNotExists(t *testing.T, filePath string) {
	t.Helper()
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Errorf("File %s should not exist but does", filePath)
	}
}

// AssertFileContent checks if file content matches expected content
func AssertFileContent(t *testing.T, filePath string, expected []byte) {
	t.Helper()
	actual, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", filePath, err)
	}
	
	if len(actual) != len(expected) {
		t.Errorf("File %s content length mismatch: expected %d, got %d", filePath, len(expected), len(actual))
		return
	}
	
	for i, b := range actual {
		if b != expected[i] {
			t.Errorf("File %s content mismatch at position %d: expected %d, got %d", filePath, i, expected[i], b)
			return
		}
	}
}

// AssertFileSize checks if file size matches expected size
func AssertFileSize(t *testing.T, filePath string, expectedSize int64) {
	t.Helper()
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("Failed to stat file %s: %v", filePath, err)
	}
	
	if info.Size() != expectedSize {
		t.Errorf("File %s size mismatch: expected %d, got %d", filePath, expectedSize, info.Size())
	}
}

// AssertDirectoryExists checks if a directory exists and fails the test if not
func AssertDirectoryExists(t *testing.T, dirPath string) {
	t.Helper()
	info, err := os.Stat(dirPath)
	if os.IsNotExist(err) {
		t.Errorf("Directory %s should exist but does not", dirPath)
		return
	}
	if err != nil {
		t.Errorf("Failed to stat directory %s: %v", dirPath, err)
		return
	}
	if !info.IsDir() {
		t.Errorf("Path %s exists but is not a directory", dirPath)
	}
}

// AssertDirectoryContainsFiles checks if directory contains expected files
func AssertDirectoryContainsFiles(t *testing.T, dirPath string, expectedFiles []string) {
	t.Helper()
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		t.Fatalf("Failed to read directory %s: %v", dirPath, err)
	}
	
	actualFiles := make(map[string]bool)
	for _, entry := range entries {
		actualFiles[entry.Name()] = true
	}
	
	for _, expectedFile := range expectedFiles {
		if !actualFiles[expectedFile] {
			t.Errorf("Directory %s should contain file %s but does not", dirPath, expectedFile)
		}
	}
}

// AssertDirectoryEntryCount checks if directory has expected number of entries
func AssertDirectoryEntryCount(t *testing.T, dirPath string, expectedCount int) {
	t.Helper()
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		t.Fatalf("Failed to read directory %s: %v", dirPath, err)
	}
	
	if len(entries) != expectedCount {
		t.Errorf("Directory %s entry count mismatch: expected %d, got %d", dirPath, expectedCount, len(entries))
	}
}

// Performance and Timing Utilities

// MeasureTime measures the time it takes to execute a function
func MeasureTime(fn func()) time.Duration {
	start := time.Now()
	fn()
	return time.Since(start)
}

// MeasureTimeWithError measures time and captures any error from function execution
func MeasureTimeWithError(fn func() error) (time.Duration, error) {
	start := time.Now()
	err := fn()
	return time.Since(start), err
}

// AssertOperationTime checks if operation completed within expected time
func AssertOperationTime(t *testing.T, duration time.Duration, maxExpected time.Duration, operation string) {
	t.Helper()
	if duration > maxExpected {
		t.Errorf("%s took too long: %v (expected ≤ %v)", operation, duration, maxExpected)
	}
}

// Concurrent Testing Utilities

// RunConcurrentOperations runs multiple operations concurrently and collects results
func RunConcurrentOperations(operations []func() error) []error {
	errors := make([]error, len(operations))
	done := make(chan int, len(operations))
	
	for i, op := range operations {
		go func(index int, operation func() error) {
			errors[index] = operation()
			done <- index
		}(i, op)
	}
	
	// Wait for all operations to complete
	for range operations {
		<-done
	}
	
	return errors
}

// AssertNoConcurrentErrors checks that no errors occurred in concurrent operations
func AssertNoConcurrentErrors(t *testing.T, errors []error, operation string) {
	t.Helper()
	for i, err := range errors {
		if err != nil {
			t.Errorf("Concurrent %s operation %d failed: %v", operation, i, err)
		}
	}
}

// Test Block Utilities

// CreateTestBlock creates a test block with specified content
func CreateTestBlock(t *testing.T, content []byte) *blocks.Block {
	t.Helper()
	if content == nil {
		content = GenerateTestContent(1024)
	}
	block, err := blocks.NewBlock(content)
	if err != nil {
		t.Fatalf("Failed to create test block: %v", err)
	}
	return block
}

// CreateTestBlocks creates multiple test blocks
func CreateTestBlocks(t *testing.T, count int, size int) []*blocks.Block {
	t.Helper()
	testBlocks := make([]*blocks.Block, count)
	
	for i := 0; i < count; i++ {
		content := GenerateTestContent(size)
		block := CreateTestBlock(t, content)
		testBlocks[i] = block
	}
	
	return testBlocks
}

// FUSE-specific Test Utilities

// WaitForMount waits for a FUSE mount to be ready for operations
func WaitForMount(mountPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(mountPath); err == nil {
			// Try to list the directory to ensure mount is responsive
			_, err := os.ReadDir(mountPath)
			if err == nil {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("mount at %s not ready after %v", mountPath, timeout)
}

// TestFileOperations performs basic file operations to test FUSE functionality
func TestFileOperations(t *testing.T, mountPath string) {
	t.Helper()
	
	// Test file creation
	testFile := filepath.Join(mountPath, "test_file.txt")
	testContent := []byte("Hello, NoiseFS FUSE!")
	
	if err := os.WriteFile(testFile, testContent, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
	
	// Test file reading
	AssertFileExists(t, testFile)
	AssertFileContent(t, testFile, testContent)
	AssertFileSize(t, testFile, int64(len(testContent)))
	
	// Test file deletion
	if err := os.Remove(testFile); err != nil {
		t.Fatalf("Failed to remove test file: %v", err)
	}
	AssertFileNotExists(t, testFile)
}

// Stream Testing Utilities

// CreateTestStream creates a test stream with specified content
func CreateTestStream(content []byte) io.Reader {
	return strings.NewReader(string(content))
}

// ReadStreamFully reads all content from a stream
func ReadStreamFully(t *testing.T, stream io.Reader) []byte {
	t.Helper()
	content, err := io.ReadAll(stream)
	if err != nil {
		t.Fatalf("Failed to read stream: %v", err)
	}
	return content
}

// Helper functions

// generateTestFileName generates a consistent test file name
func generateTestFileName(index int) string {
	return fmt.Sprintf("file_%04d.txt", index)
}

// generateTestDirName generates a consistent test directory name
func generateTestDirName(index int) string {
	return fmt.Sprintf("level%d", index)
}

// generateTestCID generates a test CID
func generateTestCID(index int) string {
	return fmt.Sprintf("QmTest%d", index)
}

// CleanupTempFiles removes temporary test files
func CleanupTempFiles(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if err := os.RemoveAll(path); err != nil {
			t.Logf("Warning: failed to cleanup %s: %v", path, err)
		}
	}
}

// WaitWithTimeout waits for a condition to be true with timeout
func WaitWithTimeout(condition func() bool, timeout time.Duration, checkInterval time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		time.Sleep(checkInterval)
	}
	return false
}
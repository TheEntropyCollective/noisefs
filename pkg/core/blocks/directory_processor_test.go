package blocks

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/TheEntropyCollective/noisefs/pkg/core/crypto"
)

// MockDirectoryBlockProcessor for testing
type MockDirectoryBlockProcessor struct {
	blocks     []*Block
	manifests  map[string]*Block
	mutex      sync.Mutex
	failOnFile string
	failOnDir  string
}

func NewMockDirectoryBlockProcessor() *MockDirectoryBlockProcessor {
	return &MockDirectoryBlockProcessor{
		blocks:    make([]*Block, 0),
		manifests: make(map[string]*Block),
	}
}

func (m *MockDirectoryBlockProcessor) ProcessDirectoryBlock(blockIndex int, block *Block) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	if m.failOnFile != "" {
		return fmt.Errorf("mock error for file: %s", m.failOnFile)
	}
	
	m.blocks = append(m.blocks, block)
	return nil
}

func (m *MockDirectoryBlockProcessor) ProcessDirectoryManifest(dirPath string, manifestBlock *Block) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	if m.failOnDir != "" && dirPath == m.failOnDir {
		return fmt.Errorf("mock error for directory: %s", dirPath)
	}
	
	m.manifests[dirPath] = manifestBlock
	return nil
}

func (m *MockDirectoryBlockProcessor) GetBlockCount() int {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return len(m.blocks)
}

func (m *MockDirectoryBlockProcessor) GetManifestCount() int {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return len(m.manifests)
}

func (m *MockDirectoryBlockProcessor) GetManifest(dirPath string) *Block {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.manifests[dirPath]
}

func (m *MockDirectoryBlockProcessor) SetFailOnFile(filePath string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.failOnFile = filePath
}

func (m *MockDirectoryBlockProcessor) SetFailOnDir(dirPath string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.failOnDir = dirPath
}

// Test helper functions
func createTestDirectoryHelper(t *testing.T, baseDir string) string {
	testDir := filepath.Join(baseDir, "test_dir")
	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	
	// Create test files
	testFiles := []struct {
		path    string
		content string
	}{
		{"file1.txt", "This is file 1 content"},
		{"file2.txt", "This is file 2 with more content"},
		{"subdir/file3.txt", "This is file 3 in subdirectory"},
		{"subdir/file4.txt", "This is file 4 in subdirectory"},
		{"subdir/nested/file5.txt", "This is file 5 in nested directory"},
	}
	
	for _, file := range testFiles {
		fullPath := filepath.Join(testDir, file.path)
		dir := filepath.Dir(fullPath)
		
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
		
		if err := os.WriteFile(fullPath, []byte(file.content), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", fullPath, err)
		}
	}
	
	return testDir
}

func createEncryptionKeyHelper(t *testing.T) *crypto.EncryptionKey {
	key, err := crypto.GenerateKey("test-password")
	if err != nil {
		t.Fatalf("Failed to generate encryption key: %v", err)
	}
	return key
}

func TestNewDirectoryProcessor(t *testing.T) {
	tests := []struct {
		name        string
		config      *ProcessorConfig
		expectError bool
	}{
		{
			name:        "nil config",
			config:      nil,
			expectError: true,
		},
		{
			name: "missing encryption key",
			config: &ProcessorConfig{
				BlockSize:  DefaultBlockSize,
				MaxWorkers: 5,
			},
			expectError: true,
		},
		{
			name: "valid config",
			config: &ProcessorConfig{
				BlockSize:     DefaultBlockSize,
				MaxWorkers:    5,
				EncryptionKey: createEncryptionKeyHelper(t),
			},
			expectError: false,
		},
		{
			name: "default values",
			config: &ProcessorConfig{
				EncryptionKey: createEncryptionKeyHelper(t),
			},
			expectError: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processor, err := NewDirectoryProcessor(tt.config)
			
			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got nil")
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			
			if processor == nil {
				t.Error("Expected non-nil processor")
				return
			}
			
			// Check default values
			if processor.blockSize <= 0 {
				t.Error("Block size should be positive")
			}
			
			if processor.maxWorkers <= 0 {
				t.Error("Max workers should be positive")
			}
		})
	}
}

func TestDirectoryProcessor_ProcessDirectory(t *testing.T) {
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "noisefs_test_")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir) // Explicitly ignore error in test cleanup
	}()
	
	// Create test directory structure
	testDir := createTestDirectoryHelper(t, tempDir)
	
	// Create processor
	config := &ProcessorConfig{
		BlockSize:     1024,
		MaxWorkers:    3,
		EncryptionKey: createEncryptionKeyHelper(t),
	}
	
	processor, err := NewDirectoryProcessor(config)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}
	
	// Create mock block processor
	mockProcessor := NewMockDirectoryBlockProcessor()
	
	// Process directory
	results, err := processor.ProcessDirectory(testDir, mockProcessor)
	if err != nil {
		t.Fatalf("Failed to process directory: %v", err)
	}
	
	// Verify results
	if len(results) == 0 {
		t.Error("Expected non-empty results")
	}
	
	// Check that blocks were processed
	if mockProcessor.GetBlockCount() == 0 {
		t.Error("Expected blocks to be processed")
	}
	
	// Check that manifests were created
	if mockProcessor.GetManifestCount() == 0 {
		t.Error("Expected manifests to be created")
	}
	
	// Verify result types
	fileCount := 0
	dirCount := 0
	
	for _, result := range results {
		switch result.Type {
		case FileType:
			fileCount++
			if result.CID == "" {
				t.Error("File result missing CID")
			}
			if result.Size <= 0 {
				t.Error("File result should have positive size")
			}
		case DirectoryType:
			dirCount++
			if result.ManifestCID == "" {
				t.Error("Directory result missing manifest CID")
			}
		}
	}
	
	if fileCount == 0 {
		t.Error("Expected file results")
	}
	
	if dirCount == 0 {
		t.Error("Expected directory results")
	}
}

func TestDirectoryProcessor_ProgressCallback(t *testing.T) {
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "noisefs_test_")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir) // Explicitly ignore error in test cleanup
	}()
	
	// Create test directory structure
	testDir := createTestDirectoryHelper(t, tempDir)
	
	// Track progress
	var progressCalls int64
	var lastFile string
	var progressMux sync.Mutex
	
	config := &ProcessorConfig{
		BlockSize:     1024,
		MaxWorkers:    3,
		EncryptionKey: createEncryptionKeyHelper(t),
		ProgressCallback: func(processed, total int64, currentFile string) {
			atomic.AddInt64(&progressCalls, 1)
			progressMux.Lock()
			lastFile = currentFile
			progressMux.Unlock()
		},
	}
	
	processor, err := NewDirectoryProcessor(config)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}
	
	mockProcessor := NewMockDirectoryBlockProcessor()
	
	// Process directory
	_, err = processor.ProcessDirectory(testDir, mockProcessor)
	if err != nil {
		t.Fatalf("Failed to process directory: %v", err)
	}
	
	// Verify progress was called
	if atomic.LoadInt64(&progressCalls) == 0 {
		t.Error("Expected progress callback to be called")
	}
	
	progressMux.Lock()
	finalLastFile := lastFile
	progressMux.Unlock()
	
	if finalLastFile == "" {
		t.Error("Expected current file to be set")
	}
}

func TestDirectoryProcessor_ErrorHandling(t *testing.T) {
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "noisefs_test_")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir) // Explicitly ignore error in test cleanup
	}()
	
	// Create test directory structure
	testDir := createTestDirectoryHelper(t, tempDir)
	
	// Track errors
	errorCalls := 0
	
	config := &ProcessorConfig{
		BlockSize:     1024,
		MaxWorkers:    3,
		EncryptionKey: createEncryptionKeyHelper(t),
		ErrorHandler: func(path string, err error) bool {
			errorCalls++
			return true // Continue processing
		},
	}
	
	processor, err := NewDirectoryProcessor(config)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}
	
	// Create mock processor that fails on specific file
	mockProcessor := NewMockDirectoryBlockProcessor()
	mockProcessor.SetFailOnFile("test error")
	
	// Process directory
	_, err = processor.ProcessDirectory(testDir, mockProcessor)
	
	// Should have error due to mock failure
	if err == nil {
		t.Error("Expected error due to mock failure")
	}
}

func TestDirectoryProcessor_Cancellation(t *testing.T) {
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "noisefs_test_")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir) // Explicitly ignore error in test cleanup
	}()
	
	// Create test directory structure
	testDir := createTestDirectoryHelper(t, tempDir)
	
	config := &ProcessorConfig{
		BlockSize:     1024,
		MaxWorkers:    3,
		EncryptionKey: createEncryptionKeyHelper(t),
	}
	
	processor, err := NewDirectoryProcessor(config)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}
	
	mockProcessor := NewMockDirectoryBlockProcessor()
	
	// Cancel immediately before processing
	processor.Cancel()
	
	// Now try to process (should fail quickly)
	_, err = processor.ProcessDirectory(testDir, mockProcessor)
	if err == nil {
		t.Error("Expected error due to cancellation")
	}
}

func TestDirectoryProcessor_GetProgress(t *testing.T) {
	config := &ProcessorConfig{
		BlockSize:     1024,
		MaxWorkers:    3,
		EncryptionKey: createEncryptionKeyHelper(t),
	}
	
	processor, err := NewDirectoryProcessor(config)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}
	
	// Initial progress should be 0
	processed, total := processor.GetProgress()
	if processed != 0 || total != 0 {
		t.Errorf("Initial progress should be 0,0 but got %d,%d", processed, total)
	}
}

func TestStreamingDirectoryProcessor(t *testing.T) {
	config := &ProcessorConfig{
		BlockSize:     1024,
		MaxWorkers:    3,
		EncryptionKey: createEncryptionKeyHelper(t),
	}
	
	processor, err := NewStreamingDirectoryProcessor(config, 100) // 100MB limit
	if err != nil {
		t.Fatalf("Failed to create streaming processor: %v", err)
	}
	
	if processor == nil {
		t.Fatal("Expected non-nil streaming processor")
	}
	
	if processor.maxMemoryUsage != 100*1024*1024 {
		t.Errorf("Expected memory limit of 100MB, got %d", processor.maxMemoryUsage)
	}
}

func TestFileBlockProcessor(t *testing.T) {
	// Create temporary file
	tempFile, err := os.CreateTemp("", "test_file_*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() {
		_ = os.Remove(tempFile.Name()) // Explicitly ignore error in test cleanup
	}()
	
	content := "This is test content for the file processor"
	if _, err := tempFile.Write([]byte(content)); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	if err := tempFile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}
	
	// Create file block processor
	mockProcessor := NewMockDirectoryBlockProcessor()
	
	fileProcessor := &FileBlockProcessor{
		FilePath:      tempFile.Name(),
		FileSize:      int64(len(content)),
		Processor:     mockProcessor,
		EncryptionKey: createEncryptionKeyHelper(t),
	}
	
	// Create test block
	block, err := NewBlock([]byte(content))
	if err != nil {
		t.Fatalf("Failed to create block: %v", err)
	}
	
	// Process block
	if err := fileProcessor.ProcessBlock(0, block); err != nil {
		t.Fatalf("Failed to process block: %v", err)
	}
	
	// Verify file CID was set
	if fileProcessor.GetFileCID() == "" {
		t.Error("Expected file CID to be set")
	}
	
	// Verify block was processed
	if mockProcessor.GetBlockCount() != 1 {
		t.Errorf("Expected 1 block, got %d", mockProcessor.GetBlockCount())
	}
}

func BenchmarkDirectoryProcessor(b *testing.B) {
	// Create temporary directory for benchmarking
	tempDir, err := os.MkdirTemp("", "noisefs_bench_")
	if err != nil {
		b.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir) // Explicitly ignore error in test cleanup
	}()
	
	// Create test directory structure
	testDir := createTestDirectoryBench(b, tempDir)
	
	config := &ProcessorConfig{
		BlockSize:     DefaultBlockSize,
		MaxWorkers:    10,
		EncryptionKey: createEncryptionKeyBench(b),
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		processor, err := NewDirectoryProcessor(config)
		if err != nil {
			b.Fatalf("Failed to create processor: %v", err)
		}
		
		mockProcessor := NewMockDirectoryBlockProcessor()
		
		_, err = processor.ProcessDirectory(testDir, mockProcessor)
		if err != nil {
			b.Fatalf("Failed to process directory: %v", err)
		}
	}
}

// createEncryptionKeyBench helper for benchmarks
func createEncryptionKeyBench(b *testing.B) *crypto.EncryptionKey {
	key, err := crypto.GenerateKey("test-password")
	if err != nil {
		b.Fatalf("Failed to generate encryption key: %v", err)
	}
	return key
}

// createTestDirectoryBench helper for benchmarks
func createTestDirectoryBench(b *testing.B, baseDir string) string {
	testDir := filepath.Join(baseDir, "test_dir")
	if err := os.MkdirAll(testDir, 0755); err != nil {
		b.Fatalf("Failed to create test directory: %v", err)
	}
	
	// Create test files
	testFiles := []struct {
		path    string
		content string
	}{
		{"file1.txt", "This is file 1 content"},
		{"file2.txt", "This is file 2 with more content"},
		{"subdir/file3.txt", "This is file 3 in subdirectory"},
		{"subdir/file4.txt", "This is file 4 in subdirectory"},
		{"subdir/nested/file5.txt", "This is file 5 in nested directory"},
	}
	
	for _, file := range testFiles {
		fullPath := filepath.Join(testDir, file.path)
		dir := filepath.Dir(fullPath)
		
		if err := os.MkdirAll(dir, 0755); err != nil {
			b.Fatalf("Failed to create directory %s: %v", dir, err)
		}
		
		if err := os.WriteFile(fullPath, []byte(file.content), 0644); err != nil {
			b.Fatalf("Failed to create file %s: %v", fullPath, err)
		}
	}
	
	return testDir
}
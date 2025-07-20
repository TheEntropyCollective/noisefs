package streaming

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/core/descriptors"
)

// Mock implementations for testing

type mockStorage struct {
	blocks      map[string]*blocks.Block
	descriptors map[string]*descriptors.Descriptor
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		blocks:      make(map[string]*blocks.Block),
		descriptors: make(map[string]*descriptors.Descriptor),
	}
}

func (m *mockStorage) StoreBlock(ctx context.Context, block *blocks.Block, hint string) (string, error) {
	// Create a more realistic CID based on block content
	cid := fmt.Sprintf("block-%s-%x", hint, block.Data[:min(8, len(block.Data))])
	m.blocks[cid] = block
	return cid, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (m *mockStorage) StoreBatch(ctx context.Context, blocks map[string]*blocks.Block, progress ProgressReporter) (map[string]string, error) {
	result := make(map[string]string)
	for hint, block := range blocks {
		cid, err := m.StoreBlock(ctx, block, hint)
		if err != nil {
			return nil, err
		}
		result[hint] = cid
	}
	return result, nil
}

func (m *mockStorage) RetrieveBlock(ctx context.Context, cid string, hint string) (*blocks.Block, error) {
	block, exists := m.blocks[cid]
	if !exists {
		return nil, ErrBlockRetrievalFailed
	}
	return block, nil
}

func (m *mockStorage) RetrieveBatch(ctx context.Context, cids map[string]string, progress ProgressReporter) (map[string]*blocks.Block, error) {
	result := make(map[string]*blocks.Block)
	for hint, cid := range cids {
		block, err := m.RetrieveBlock(ctx, cid, hint)
		if err != nil {
			return nil, err
		}
		result[hint] = block
	}
	return result, nil
}

func (m *mockStorage) StoreDescriptor(ctx context.Context, descriptor *descriptors.Descriptor, encrypted bool, password string) (string, error) {
	cid := "descriptor-" + descriptor.Filename
	m.descriptors[cid] = descriptor
	return cid, nil
}

func (m *mockStorage) RetrieveDescriptor(ctx context.Context, cid string, password string) (*descriptors.Descriptor, error) {
	descriptor, exists := m.descriptors[cid]
	if !exists {
		return nil, ErrDescriptorNotFound
	}
	return descriptor, nil
}

func (m *mockStorage) GetStorageMetrics() StorageMetrics {
	return StorageMetrics{}
}

func (m *mockStorage) Close() error {
	return nil
}

type mockRandomizerProvider struct {
	storage *mockStorage
}

func (m *mockRandomizerProvider) SelectRandomizers(ctx context.Context, blockSize int, hint string) (*blocks.Block, string, *blocks.Block, string, int64, error) {
	// Create simple randomizer blocks with different patterns
	rand1Data := make([]byte, blockSize)
	for i := range rand1Data {
		rand1Data[i] = 0xAA
	}
	
	rand2Data := make([]byte, blockSize)
	for i := range rand2Data {
		rand2Data[i] = 0x55
	}
	
	rand1, _ := blocks.NewBlock(rand1Data)
	rand2, _ := blocks.NewBlock(rand2Data)
	
	// Store randomizers in mock storage so they can be retrieved during download
	rand1CID, _ := m.storage.StoreBlock(ctx, rand1, "randomizer")
	rand2CID, _ := m.storage.StoreBlock(ctx, rand2, "randomizer")
	
	return rand1, rand1CID, rand2, rand2CID, 0, nil
}

func (m *mockRandomizerProvider) GenerateRandomizer(ctx context.Context, blockSize int, metadata map[string]string) (*blocks.Block, string, error) {
	data := make([]byte, blockSize)
	for i := range data {
		data[i] = 0xFF
	}
	block, _ := blocks.NewBlock(data)
	return block, "generated-rand-cid", nil
}

func (m *mockRandomizerProvider) CacheRandomizer(ctx context.Context, cid string, block *blocks.Block, metadata map[string]string) error {
	return nil
}

func (m *mockRandomizerProvider) GetRandomizerMetrics() RandomizerMetrics {
	return RandomizerMetrics{}
}

func (m *mockRandomizerProvider) SetStrategy(strategy string) error {
	return nil
}

type mockAssembler struct {
	writer           io.Writer
	complete         bool
	progress         AssemblyProgress
	originalFileSize int64
	bytesWritten     int64
}

func newMockAssembler() *mockAssembler {
	return &mockAssembler{}
}

func (m *mockAssembler) Initialize(ctx context.Context, descriptor *descriptors.Descriptor, writer io.Writer) error {
	m.writer = writer
	m.progress = AssemblyProgress{
		TotalBlocks: len(descriptor.Blocks),
		TotalBytes:  descriptor.FileSize,
	}
	// Store the original file size for proper trimming
	m.originalFileSize = descriptor.FileSize
	m.bytesWritten = 0
	return nil
}

func (m *mockAssembler) AddBlock(ctx context.Context, blockIndex int, anonymizedBlock *blocks.Block, randomizer1 *blocks.Block, randomizer2 *blocks.Block) (bool, error) {
	// Perform XOR de-anonymization
	original, err := anonymizedBlock.XOR(randomizer1, randomizer2)
	if err != nil {
		return false, err
	}
	
	// Determine how much data to write (handle padding for final block)
	dataToWrite := original.Data
	bytesToWrite := int64(len(original.Data))
	
	// If this would exceed the original file size, trim the padding
	if m.bytesWritten + bytesToWrite > m.originalFileSize {
		remainingBytes := m.originalFileSize - m.bytesWritten
		if remainingBytes > 0 {
			dataToWrite = original.Data[:remainingBytes]
			bytesToWrite = remainingBytes
		} else {
			bytesToWrite = 0
		}
	}
	
	
	// Write to output (only if there's data to write)
	if bytesToWrite > 0 {
		_, err = m.writer.Write(dataToWrite)
		if err != nil {
			return false, err
		}
		m.bytesWritten += bytesToWrite
	}
	
	// Update progress
	m.progress.ProcessedBlocks++
	m.progress.ProcessedBytes += int64(len(original.Data)) // Full block for progress tracking
	m.progress.WrittenBytes += bytesToWrite // Actual bytes written
	
	// Check if complete
	m.complete = m.progress.ProcessedBlocks >= m.progress.TotalBlocks
	return m.complete, nil
}

func (m *mockAssembler) GetProgress() AssemblyProgress {
	return m.progress
}

func (m *mockAssembler) IsComplete() bool {
	return m.complete
}

func (m *mockAssembler) GetMissingBlocks() []int {
	missing := make([]int, 0)
	for i := m.progress.ProcessedBlocks; i < m.progress.TotalBlocks; i++ {
		missing = append(missing, i)
	}
	return missing
}

func (m *mockAssembler) Cancel() error {
	return nil
}

func (m *mockAssembler) Close() error {
	return nil
}

// Test functions

func TestNewStreamer(t *testing.T) {
	storage := newMockStorage()
	randomizer := &mockRandomizerProvider{storage: storage}
	assembler := newMockAssembler()
	config := DefaultConfig()

	streamer, err := NewStreamer(storage, randomizer, assembler, config)
	if err != nil {
		t.Fatalf("Failed to create streamer: %v", err)
	}

	if streamer == nil {
		t.Fatal("Streamer should not be nil")
	}
}

func TestNewStreamerValidation(t *testing.T) {
	storage := newMockStorage()
	randomizer := &mockRandomizerProvider{storage: storage}
	assembler := newMockAssembler()

	// Test with nil storage
	_, err := NewStreamer(nil, randomizer, assembler, DefaultConfig())
	if err == nil {
		t.Error("Expected error with nil storage")
	}

	// Test with nil randomizer
	_, err = NewStreamer(storage, nil, assembler, DefaultConfig())
	if err == nil {
		t.Error("Expected error with nil randomizer")
	}

	// Test with nil assembler
	_, err = NewStreamer(storage, randomizer, nil, DefaultConfig())
	if err == nil {
		t.Error("Expected error with nil assembler")
	}

	// Test with nil config
	_, err = NewStreamer(storage, randomizer, assembler, nil)
	if err == nil {
		t.Error("Expected error with nil config")
	}
}

func TestStreamUpload(t *testing.T) {
	storage := newMockStorage()
	randomizer := &mockRandomizerProvider{storage: storage}
	assembler := newMockAssembler()
	config := DefaultConfig()

	streamer, err := NewStreamer(storage, randomizer, assembler, config)
	if err != nil {
		t.Fatalf("Failed to create streamer: %v", err)
	}

	// Test data
	testData := "Hello, NoiseFS streaming!"
	reader := strings.NewReader(testData)

	// Create upload options
	opts := UploadOptions{
		Filename:         "test.txt",
		BlockSize:        1024,
		MaxConcurrency:   2,
		ProgressReporter: NewNoOpProgressReporter(),
	}

	// Perform upload
	ctx := context.Background()
	descriptorCID, err := streamer.StreamUpload(ctx, reader, opts)
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	if descriptorCID == "" {
		t.Error("Descriptor CID should not be empty")
	}

	// Verify descriptor was stored
	descriptor, err := storage.RetrieveDescriptor(ctx, descriptorCID, "")
	if err != nil {
		t.Fatalf("Failed to retrieve descriptor: %v", err)
	}

	if descriptor.Filename != "test.txt" {
		t.Errorf("Expected filename 'test.txt', got '%s'", descriptor.Filename)
	}
}

func TestStreamDownload(t *testing.T) {
	storage := newMockStorage()
	randomizer := &mockRandomizerProvider{storage: storage}
	assembler := newMockAssembler()
	config := DefaultConfig()

	streamer, err := NewStreamer(storage, randomizer, assembler, config)
	if err != nil {
		t.Fatalf("Failed to create streamer: %v", err)
	}

	// First upload some data
	testData := "Hello, NoiseFS streaming download!"
	reader := strings.NewReader(testData)

	uploadOpts := UploadOptions{
		Filename:  "test.txt",
		BlockSize: 1024,
	}

	ctx := context.Background()
	descriptorCID, err := streamer.StreamUpload(ctx, reader, uploadOpts)
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	// Now download the data
	var output bytes.Buffer
	downloadOpts := DownloadOptions{
		MaxConcurrency:   2,
		ProgressReporter: NewNoOpProgressReporter(),
	}

	err = streamer.StreamDownload(ctx, descriptorCID, &output, downloadOpts)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	// Verify downloaded data matches original
	downloadedData := output.String()
	if downloadedData != testData {
		t.Errorf("Downloaded data doesn't match. Expected length %d, got length %d", len(testData), len(downloadedData))
		t.Errorf("Expected: '%s'", testData)
		t.Errorf("Got:      '%s'", downloadedData)
	}
}

func TestProgressReporting(t *testing.T) {
	// Test console progress reporter
	reporter := NewConsoleProgressReporter("test")
	
	startTime := time.Now()
	reporter.SetTotal(1000, 10)
	
	reporter.ReportProgress(ProgressInfo{
		Stage:           "Processing",
		BytesProcessed:  500,
		TotalBytes:      1000,
		BlocksProcessed: 5,
		TotalBlocks:     10,
		StartTime:       startTime,
		CurrentTime:     time.Now(),
		Throughput:      1024.0,
	})
	
	reporter.Complete(ProgressInfo{
		Stage:           "Complete",
		BytesProcessed:  1000,
		TotalBytes:      1000,
		BlocksProcessed: 10,
		TotalBlocks:     10,
		StartTime:       startTime,
		CurrentTime:     time.Now(),
		Throughput:      2048.0,
	})
}

func TestConfigBuilder(t *testing.T) {
	config, err := NewConfigBuilder().
		WithBlockSize(256 * 1024).
		WithMaxConcurrency(8).
		WithTimeout(30 * time.Minute).
		WithBufferSize(128 * 1024).
		WithValidationLevel(ValidationStandard).
		BuildWithDefaults()

	if err != nil {
		t.Fatalf("Failed to build config: %v", err)
	}

	if config.GetBlockSize() != 256*1024 {
		t.Errorf("Expected block size 256KB, got %d", config.GetBlockSize())
	}

	if config.GetMaxConcurrency() != 8 {
		t.Errorf("Expected max concurrency 8, got %d", config.GetMaxConcurrency())
	}

	if config.GetTimeout() != 30*time.Minute {
		t.Errorf("Expected timeout 30m, got %v", config.GetTimeout())
	}
}

func TestValidationLevels(t *testing.T) {
	levels := []ValidationLevel{
		ValidationNone,
		ValidationBasic,
		ValidationStandard,
		ValidationStrict,
	}

	expectedStrings := []string{"none", "basic", "standard", "strict"}

	for i, level := range levels {
		if level.String() != expectedStrings[i] {
			t.Errorf("Expected string '%s' for level %d, got '%s'", expectedStrings[i], level, level.String())
		}
	}
}

func TestStreamingErrors(t *testing.T) {
	err := &StreamingError{
		Operation:      "upload",
		Stage:          "initialization",
		Underlying:     ErrInvalidOptions,
		Retryable:      false,
		RecoveryAction: "Check configuration",
	}

	expectedMessage := "upload initialization failed: invalid streaming options"
	if err.Error() != expectedMessage {
		t.Errorf("Expected error message '%s', got '%s'", expectedMessage, err.Error())
	}

	if err.Unwrap() != ErrInvalidOptions {
		t.Error("Unwrap should return the underlying error")
	}

	if !err.Is(ErrInvalidOptions) {
		t.Error("Is should return true for the underlying error")
	}
}

func TestStreamUploadTimeout(t *testing.T) {
	storage := newMockStorage()
	randomizer := &mockRandomizerProvider{storage: storage}
	assembler := newMockAssembler()
	config := DefaultConfig()

	streamer, err := NewStreamer(storage, randomizer, assembler, config)
	if err != nil {
		t.Fatalf("Failed to create streamer: %v", err)
	}

	// Test data
	testData := "Hello, NoiseFS streaming with timeout!"
	reader := strings.NewReader(testData)

	// Create upload options with very short timeout
	opts := UploadOptions{
		Filename:         "test-timeout.txt",
		BlockSize:        1024,
		Timeout:          1 * time.Nanosecond, // Very short timeout
		ProgressReporter: NewNoOpProgressReporter(),
	}

	// Create context that will timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Perform upload - should timeout
	_, err = streamer.StreamUpload(ctx, reader, opts)
	if err == nil {
		t.Error("Expected timeout error but upload succeeded")
	}
}

func TestStreamUploadLargeFile(t *testing.T) {
	storage := newMockStorage()
	randomizer := &mockRandomizerProvider{storage: storage}
	assembler := newMockAssembler()
	config := DefaultConfig()

	streamer, err := NewStreamer(storage, randomizer, assembler, config)
	if err != nil {
		t.Fatalf("Failed to create streamer: %v", err)
	}

	// Create larger test data (multiple blocks)
	largeData := strings.Repeat("This is a test block of data. ", 100) // ~3KB
	reader := strings.NewReader(largeData)

	// Create upload options
	opts := UploadOptions{
		Filename:         "large-test.txt",
		BlockSize:        1024, // Will create multiple blocks
		MaxConcurrency:   2,
		ProgressReporter: NewNoOpProgressReporter(),
	}

	// Perform upload
	ctx := context.Background()
	descriptorCID, err := streamer.StreamUpload(ctx, reader, opts)
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	if descriptorCID == "" {
		t.Error("Descriptor CID should not be empty")
	}

	// Verify descriptor was stored with multiple blocks
	descriptor, err := storage.RetrieveDescriptor(ctx, descriptorCID, "")
	if err != nil {
		t.Fatalf("Failed to retrieve descriptor: %v", err)
	}

	if len(descriptor.Blocks) <= 1 {
		t.Errorf("Expected multiple blocks for large file, got %d", len(descriptor.Blocks))
	}
}

func TestStreamUploadWithChannelProgressReporter(t *testing.T) {
	storage := newMockStorage()
	randomizer := &mockRandomizerProvider{storage: storage}
	assembler := newMockAssembler()
	config := DefaultConfig()

	streamer, err := NewStreamer(storage, randomizer, assembler, config)
	if err != nil {
		t.Fatalf("Failed to create streamer: %v", err)
	}

	// Create channel progress reporter
	reporter := NewChannelProgressReporter("test-channel", 10)
	defer reporter.Close()

	// Test data
	testData := "Hello, NoiseFS streaming with channel progress!"
	reader := strings.NewReader(testData)

	// Create upload options
	opts := UploadOptions{
		Filename:         "test-channel-progress.txt",
		BlockSize:        1024,
		ProgressReporter: reporter,
	}

	// Start upload in goroutine
	ctx := context.Background()
	done := make(chan struct{})
	var uploadErr error
	var descriptorCID string

	go func() {
		defer close(done)
		descriptorCID, uploadErr = streamer.StreamUpload(ctx, reader, opts)
	}()

	// Monitor progress
	progressReceived := false
	for {
		select {
		case progress := <-reporter.Updates():
			progressReceived = true
			if progress.Stage == "" {
				t.Error("Progress stage should not be empty")
			}
		case <-reporter.Completed():
			// Upload completed
			if !progressReceived {
				t.Error("Should have received progress updates")
			}
			<-done
			if uploadErr != nil {
				t.Fatalf("Upload failed: %v", uploadErr)
			}
			if descriptorCID == "" {
				t.Error("Descriptor CID should not be empty")
			}
			return
		case reason := <-reporter.Cancelled():
			t.Fatalf("Upload was cancelled: %s", reason)
		case err := <-reporter.Errors():
			t.Fatalf("Progress reporter error: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatal("Test timed out waiting for progress")
		}
	}
}

func TestStreamMetrics(t *testing.T) {
	storage := newMockStorage()
	randomizer := &mockRandomizerProvider{storage: storage}
	assembler := newMockAssembler()
	config := DefaultConfig()

	streamer, err := NewStreamer(storage, randomizer, assembler, config)
	if err != nil {
		t.Fatalf("Failed to create streamer: %v", err)
	}

	// Check initial metrics
	metrics := streamer.GetMetrics()
	if metrics.TotalOperations != 0 {
		t.Errorf("Expected 0 initial operations, got %d", metrics.TotalOperations)
	}

	// Perform an upload
	testData := "Hello, NoiseFS metrics test!"
	reader := strings.NewReader(testData)

	opts := UploadOptions{
		Filename:         "metrics-test.txt",
		BlockSize:        1024,
		ProgressReporter: NewNoOpProgressReporter(),
	}

	ctx := context.Background()
	_, err = streamer.StreamUpload(ctx, reader, opts)
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	// Check updated metrics
	metrics = streamer.GetMetrics()
	if metrics.TotalOperations != 1 {
		t.Errorf("Expected 1 operation, got %d", metrics.TotalOperations)
	}
	if metrics.SuccessfulOperations != 1 {
		t.Errorf("Expected 1 successful operation, got %d", metrics.SuccessfulOperations)
	}
	if metrics.TotalBytesProcessed == 0 {
		t.Error("Expected some bytes processed")
	}
}

func TestStreamerClosed(t *testing.T) {
	storage := newMockStorage()
	randomizer := &mockRandomizerProvider{storage: storage}
	assembler := newMockAssembler()
	config := DefaultConfig()

	streamer, err := NewStreamer(storage, randomizer, assembler, config)
	if err != nil {
		t.Fatalf("Failed to create streamer: %v", err)
	}

	// Close the streamer
	err = streamer.Close()
	if err != nil {
		t.Fatalf("Failed to close streamer: %v", err)
	}

	// Try to use closed streamer
	testData := "Hello, NoiseFS closed streamer test!"
	reader := strings.NewReader(testData)

	opts := UploadOptions{
		Filename: "closed-test.txt",
		BlockSize: 1024,
	}

	ctx := context.Background()
	_, err = streamer.StreamUpload(ctx, reader, opts)
	if err != ErrStreamerClosed {
		t.Errorf("Expected ErrStreamerClosed, got %v", err)
	}
}

func TestConfigValidation(t *testing.T) {
	// Test invalid block size
	_, err := NewConfigBuilder().
		WithBlockSize(-1).
		Build()
	if err == nil {
		t.Error("Expected error for negative block size")
	}

	// Test invalid concurrency
	_, err = NewConfigBuilder().
		WithMaxConcurrency(0).
		Build()
	if err == nil {
		t.Error("Expected error for zero concurrency")
	}

	// Test invalid buffer size
	_, err = NewConfigBuilder().
		WithBufferSize(-1).
		Build()
	if err == nil {
		t.Error("Expected error for negative buffer size")
	}

	// Test valid configuration should work
	config, err := NewConfigBuilder().
		WithBlockSize(1024).
		WithMaxConcurrency(4).
		WithBufferSize(8192).
		Build()
	if err != nil {
		t.Errorf("Valid configuration should not error: %v", err)
	}
	if config == nil {
		t.Error("Valid configuration should not be nil")
	}
}
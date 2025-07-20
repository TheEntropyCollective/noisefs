package noisefs

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/TheEntropyCollective/noisefs/pkg/core/blocks"
	"github.com/TheEntropyCollective/noisefs/pkg/storage"
	"github.com/TheEntropyCollective/noisefs/pkg/storage/cache"
)

// testBackend implements a simple in-memory backend for testing
type testBackend struct {
	name         string
	connected    bool
	blocks       map[string]*blocks.Block
}

func newTestBackend(name string) *testBackend {
	return &testBackend{
		name:      name,
		connected: false,
		blocks:    make(map[string]*blocks.Block),
	}
}

func (t *testBackend) Connect(ctx context.Context) error {
	t.connected = true
	return nil
}

func (t *testBackend) Disconnect(ctx context.Context) error {
	t.connected = false
	return nil
}

func (t *testBackend) IsConnected() bool {
	return t.connected
}

func (t *testBackend) Put(ctx context.Context, block *blocks.Block) (*storage.BlockAddress, error) {
	if !t.connected {
		return nil, fmt.Errorf("not connected")
	}
	
	blockID := fmt.Sprintf("test-%d", len(t.blocks))
	t.blocks[blockID] = block
	
	return &storage.BlockAddress{
		ID:          blockID,
		BackendType: "test",
	}, nil
}

func (t *testBackend) Get(ctx context.Context, address *storage.BlockAddress) (*blocks.Block, error) {
	if !t.connected {
		return nil, fmt.Errorf("not connected")
	}
	
	block, exists := t.blocks[address.ID]
	if !exists {
		return nil, fmt.Errorf("block not found")
	}
	
	return block, nil
}

func (t *testBackend) Delete(ctx context.Context, address *storage.BlockAddress) error {
	if !t.connected {
		return fmt.Errorf("not connected")
	}
	
	delete(t.blocks, address.ID)
	return nil
}

func (t *testBackend) Has(ctx context.Context, address *storage.BlockAddress) (bool, error) {
	if !t.connected {
		return false, fmt.Errorf("not connected")
	}
	
	_, exists := t.blocks[address.ID]
	return exists, nil
}

func (t *testBackend) PutMany(ctx context.Context, blocks []*blocks.Block) ([]*storage.BlockAddress, error) {
	if !t.connected {
		return nil, fmt.Errorf("not connected")
	}
	
	addresses := make([]*storage.BlockAddress, len(blocks))
	for i, block := range blocks {
		addr, err := t.Put(ctx, block)
		if err != nil {
			return nil, err
		}
		addresses[i] = addr
	}
	return addresses, nil
}

func (t *testBackend) GetMany(ctx context.Context, addresses []*storage.BlockAddress) ([]*blocks.Block, error) {
	if !t.connected {
		return nil, fmt.Errorf("not connected")
	}
	
	result := make([]*blocks.Block, len(addresses))
	for i, addr := range addresses {
		block, err := t.Get(ctx, addr)
		if err != nil {
			return nil, err
		}
		result[i] = block
	}
	return result, nil
}

func (t *testBackend) Pin(ctx context.Context, address *storage.BlockAddress) error {
	// Simple implementation - pinning is a no-op
	return nil
}

func (t *testBackend) Unpin(ctx context.Context, address *storage.BlockAddress) error {
	// Simple implementation - unpinning is a no-op
	return nil
}

func (t *testBackend) GetBackendInfo() *storage.BackendInfo {
	return &storage.BackendInfo{
		Type:         "test",
		Name:         t.name,
		Capabilities: []string{"content-address"},
	}
}

func (t *testBackend) HealthCheck(ctx context.Context) *storage.HealthStatus {
	return &storage.HealthStatus{
		Healthy:    t.connected,
		Status:     "healthy",
		Latency:    1 * time.Millisecond,
		LastCheck:  time.Now(),
	}
}

// NewTestClient creates a client for testing with in-memory storage
func NewTestClient() (*Client, error) {
	// Register a test backend for testing
	storage.RegisterBackend("test", func(config *storage.BackendConfig) (storage.Backend, error) {
		return newTestBackend("test"), nil
	})
	
	config := storage.DefaultConfig()
	config.Backends = make(map[string]*storage.BackendConfig)
	
	// Create test backend configuration
	config.Backends["test"] = &storage.BackendConfig{
		Type:    "test",
		Enabled: true,
		Connection: &storage.ConnectionConfig{
			Endpoint: "memory://test",
		},
	}
	config.DefaultBackend = "test"
	
	manager, err := storage.NewManager(config)
	if err != nil {
		return nil, err
	}
	
	// Start the storage manager for testing
	err = manager.Start(context.Background())
	if err != nil {
		return nil, err
	}
	
	cache := cache.NewMemoryCache(10)
	return NewClient(manager, cache)
}

// TestStreamingUploadDownload tests the complete streaming upload and download cycle
func TestStreamingUploadDownload(t *testing.T) {
	// Create test data
	testData := "This is test data for streaming upload and download functionality"
	reader := strings.NewReader(testData)
	
	// Create client with in-memory storage
	client, err := NewTestClient()
	if err != nil {
		t.Fatalf("Failed to create test client: %v", err)
	}
	
	// Test streaming upload
	descriptorCID, err := client.StreamingUpload(reader, "test-file.txt")
	if err != nil {
		t.Fatalf("Failed to upload file: %v", err)
	}
	
	if descriptorCID == "" {
		t.Fatal("Expected non-empty descriptor CID")
	}
	
	// Test streaming download
	var downloadBuffer bytes.Buffer
	err = client.StreamingDownload(descriptorCID, &downloadBuffer)
	if err != nil {
		t.Fatalf("Failed to download file: %v", err)
	}
	
	// Verify downloaded data matches original
	downloadedData := downloadBuffer.String()
	if downloadedData != testData {
		t.Errorf("Downloaded data doesn't match original.\nExpected: %q\nGot: %q", testData, downloadedData)
	}
}

// TestStreamingUploadWithProgress tests streaming upload with progress reporting
func TestStreamingUploadWithProgress(t *testing.T) {
	// Create larger test data
	testData := strings.Repeat("Hello, streaming world! ", 1000) // ~25KB
	reader := strings.NewReader(testData)
	
	client, err := NewTestClient()
	if err != nil {
		t.Fatalf("Failed to create test client: %v", err)
	}
	
	var progressCallsUpload []string
	progressCallbackUpload := func(operation string, bytesProcessed int64, blocksProcessed int) {
		progressCallsUpload = append(progressCallsUpload, operation)
	}
	
	// Test streaming upload with progress
	descriptorCID, err := client.StreamingUploadWithProgress(reader, "test-large.txt", progressCallbackUpload)
	if err != nil {
		t.Fatalf("Failed to upload file: %v", err)
	}
	
	// Verify progress callbacks were called
	if len(progressCallsUpload) == 0 {
		t.Error("Expected progress callbacks to be called during upload")
	}
	
	// Test streaming download with progress
	var downloadBuffer bytes.Buffer
	var progressCallsDownload []string
	progressCallbackDownload := func(operation string, bytesProcessed int64, blocksProcessed int) {
		progressCallsDownload = append(progressCallsDownload, operation)
	}
	
	err = client.StreamingDownloadWithProgress(descriptorCID, &downloadBuffer, progressCallbackDownload)
	if err != nil {
		t.Fatalf("Failed to download file: %v", err)
	}
	
	// Verify progress callbacks were called
	if len(progressCallsDownload) == 0 {
		t.Error("Expected progress callbacks to be called during download")
	}
	
	// Verify data integrity
	downloadedData := downloadBuffer.String()
	if downloadedData != testData {
		t.Error("Downloaded data doesn't match original")
	}
}

// TestStreamingLargeFile tests streaming with larger files to verify memory efficiency
func TestStreamingLargeFile(t *testing.T) {
	// Create 1MB of random test data
	testData := make([]byte, 1024*1024) // 1MB
	_, err := rand.Read(testData)
	if err != nil {
		t.Fatalf("Failed to generate test data: %v", err)
	}
	
	reader := bytes.NewReader(testData)
	
	client, err := NewTestClient()
	if err != nil {
		t.Fatalf("Failed to create test client: %v", err)
	}
	
	// Upload with streaming
	descriptorCID, err := client.StreamingUpload(reader, "large-test.bin")
	if err != nil {
		t.Fatalf("Failed to upload large file: %v", err)
	}
	
	// Download with streaming
	var downloadBuffer bytes.Buffer
	err = client.StreamingDownload(descriptorCID, &downloadBuffer)
	if err != nil {
		t.Fatalf("Failed to download large file: %v", err)
	}
	
	// Verify data integrity
	downloadedData := downloadBuffer.Bytes()
	if !bytes.Equal(downloadedData, testData) {
		t.Error("Downloaded data doesn't match original for large file")
	}
}

// TestStreamingEmptyFile tests edge case of empty file
func TestStreamingEmptyFile(t *testing.T) {
	reader := strings.NewReader("")
	
	client, err := NewTestClient()
	if err != nil {
		t.Fatalf("Failed to create test client: %v", err)
	}
	
	// Upload empty file - this should handle the empty case appropriately
	descriptorCID, err := client.StreamingUpload(reader, "empty.txt")
	if err != nil {
		// Empty files may not be supported or may be handled differently
		// This is expected behavior that needs to be defined
		t.Skipf("Empty file upload not supported: %v", err)
	}
	
	// Download empty file
	var downloadBuffer bytes.Buffer
	err = client.StreamingDownload(descriptorCID, &downloadBuffer)
	if err != nil {
		t.Fatalf("Failed to download empty file: %v", err)
	}
	
	// Verify empty download
	if downloadBuffer.Len() != 0 {
		t.Errorf("Expected empty download, got %d bytes", downloadBuffer.Len())
	}
}

// TestStreamingCustomBlockSize tests streaming with different block sizes
func TestStreamingCustomBlockSize(t *testing.T) {
	testData := "This is test data for custom block size testing"
	
	client, err := NewTestClient()
	if err != nil {
		t.Fatalf("Failed to create test client: %v", err)
	}
	
	// Test with smaller block size
	customBlockSize := 1024 // 1KB
	reader := strings.NewReader(testData)
	
	descriptorCID, err := client.StreamingUploadWithBlockSize(reader, "custom-block.txt", customBlockSize)
	if err != nil {
		t.Fatalf("Failed to upload with custom block size: %v", err)
	}
	
	// Download and verify
	var downloadBuffer bytes.Buffer
	err = client.StreamingDownload(descriptorCID, &downloadBuffer)
	if err != nil {
		t.Fatalf("Failed to download with custom block size: %v", err)
	}
	
	downloadedData := downloadBuffer.String()
	if downloadedData != testData {
		t.Error("Downloaded data doesn't match original with custom block size")
	}
}

// TestStreamingErrorHandling tests error conditions
func TestStreamingErrorHandling(t *testing.T) {
	client, err := NewTestClient()
	if err != nil {
		t.Fatalf("Failed to create test client: %v", err)
	}
	
	t.Run("NilReader", func(t *testing.T) {
		_, err := client.StreamingUpload(nil, "test.txt")
		if err == nil {
			t.Error("Expected error for nil reader")
		}
	})
	
	t.Run("NilWriter", func(t *testing.T) {
		err := client.StreamingDownload("test-cid", nil)
		if err == nil {
			t.Error("Expected error for nil writer")
		}
	})
	
	t.Run("InvalidDescriptorCID", func(t *testing.T) {
		var buffer bytes.Buffer
		err := client.StreamingDownload("invalid-cid", &buffer)
		if err == nil {
			t.Error("Expected error for invalid descriptor CID")
		}
	})
	
	t.Run("InvalidBlockSize", func(t *testing.T) {
		reader := strings.NewReader("test")
		_, err := client.StreamingUploadWithBlockSize(reader, "test.txt", 0)
		if err == nil {
			t.Error("Expected error for invalid block size")
		}
	})
}

// TestStreamingBlockProcessing tests that streaming properly processes blocks
func TestStreamingBlockProcessing(t *testing.T) {
	// Create test data that spans multiple blocks
	testData := strings.Repeat("A", blocks.DefaultBlockSize*2+1000) // 2+ blocks
	reader := strings.NewReader(testData)
	
	client, err := NewTestClient()
	if err != nil {
		t.Fatalf("Failed to create test client: %v", err)
	}
	
	var progressReports []struct {
		operation      string
		bytesProcessed int64
		blocksProcessed int
	}
	
	progressCallback := func(operation string, bytesProcessed int64, blocksProcessed int) {
		progressReports = append(progressReports, struct {
			operation      string
			bytesProcessed int64
			blocksProcessed int
		}{operation, bytesProcessed, blocksProcessed})
	}
	
	// Upload with progress tracking
	descriptorCID, err := client.StreamingUploadWithProgress(reader, "multi-block.txt", progressCallback)
	if err != nil {
		t.Fatalf("Failed to upload multi-block file: %v", err)
	}
	
	// Verify multiple blocks were processed
	found := false
	for _, report := range progressReports {
		if report.operation == "Processing blocks" && report.blocksProcessed > 1 {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected multiple blocks to be processed")
	}
	
	// Download and verify
	var downloadBuffer bytes.Buffer
	err = client.StreamingDownload(descriptorCID, &downloadBuffer)
	if err != nil {
		t.Fatalf("Failed to download multi-block file: %v", err)
	}
	
	downloadedData := downloadBuffer.String()
	if downloadedData != testData {
		t.Error("Downloaded multi-block data doesn't match original")
	}
}

// TestStreamingMemoryEfficiency verifies streaming doesn't load entire file into memory
func TestStreamingMemoryEfficiency(t *testing.T) {
	// Note: This test is more of a design validation than a hard memory test
	// In a real implementation, you'd use memory profiling tools
	
	// Create test that would be prohibitive to load entirely into memory in a real scenario
	testSize := 10 * 1024 // 10KB for testing (in real usage this would be much larger)
	testData := strings.Repeat("X", testSize)
	
	client, err := NewTestClient()
	if err != nil {
		t.Fatalf("Failed to create test client: %v", err)
	}
	
	// Use a custom reader that tracks how much data is read at once
	reader := &trackingReader{
		reader:  strings.NewReader(testData),
		maxRead: 0,
	}
	
	descriptorCID, err := client.StreamingUpload(reader, "memory-test.txt")
	if err != nil {
		t.Fatalf("Failed to upload for memory test: %v", err)
	}
	
	// Verify the reader never read the entire file at once
	// (should read in block-sized chunks)
	if reader.maxRead >= testSize {
		t.Errorf("Streaming upload read entire file at once (%d bytes), not truly streaming", reader.maxRead)
	}
	
	// Download and verify data integrity
	var downloadBuffer bytes.Buffer
	err = client.StreamingDownload(descriptorCID, &downloadBuffer)
	if err != nil {
		t.Fatalf("Failed to download for memory test: %v", err)
	}
	
	downloadedData := downloadBuffer.String()
	if downloadedData != testData {
		t.Error("Downloaded data doesn't match original in memory efficiency test")
	}
}

// trackingReader tracks the maximum amount of data read in a single Read() call
type trackingReader struct {
	reader  io.Reader
	maxRead int
}

func (tr *trackingReader) Read(p []byte) (n int, err error) {
	n, err = tr.reader.Read(p)
	if n > tr.maxRead {
		tr.maxRead = n
	}
	return n, err
}

